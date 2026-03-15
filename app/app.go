package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"relayhat/pkg/relay"
	"strconv"
	"sync"
	"syscall"
)

// VERSION holds the version information with the following logic in mind
//
//	4 ... fixed
//	0 ... year 2020, 1->year 2021, etc.
//	7 ... month of year (7=July)
//	the date format after the + is always the first of the month
//
// VERSION differs from semantic versioning as described in https://semver.org/
// but we keep the correct syntax.
const (
	VERSION = "1.6.3+20260315"
	MODULE  = "relayhat"

	ModeStop    = 0
	ModeRestart = 1
)

// App holds the application's runtime state and lifecycle dependencies.
type App struct {
	wg         sync.WaitGroup // wait group to track running webserver
	baseDir    string         // working directory
	config     *Config        // app configuration
	web        *http.Server   // HTTP server
	restart    chan struct{}  // signals application restart
	shutdown   chan struct{}  // signals application shutdown
	ctx        context.Context
	cancelFunc context.CancelFunc

	mu     sync.RWMutex // protects app.relays
	relays map[string]Relay
}

// Relay combines the runtime relay instance with its config metadata.
type Relay struct {
	*relay.Relay
	Description string
}

// New constructs an App with the configured server address and lifecycle channels.
func New(config *Config, baseDir string) *App {
	ctx, cancel := context.WithCancel(context.Background())

	return &App{
		config:  config,
		baseDir: baseDir,
		web: &http.Server{
			Addr: net.JoinHostPort(config.Webserver.ListenHost, strconv.Itoa(config.Webserver.ListenPort)),
		},
		restart:    make(chan struct{}),
		shutdown:   make(chan struct{}),
		ctx:        ctx,
		cancelFunc: cancel,
		relays:     make(map[string]Relay),
	}
}

// Run starts the application.
func (app *App) Run() (*App, error) {
	if err := app.Init(); err != nil {
		return app, err
	}

	// here start your services

	// handle the OS signals
	app.HandleOSSignals()

	slog.Info("Starting web server", "url", app.web.Addr)
	err := app.StartWebServer()
	if err != nil {
		slog.Error("Web server failed to start", "url", app.web.Addr, "error", err)
		return app, err
	}

	slog.Info("Module started successfully",
		"module", MODULE,
		"version", VERSION,
		"pid", os.Getpid(),
	)
	return app, nil
}

// Init initializes relays and HTTP routes from the current configuration.
func (app *App) Init() error {

	// register the relay
	// Re-initialise the relay map on every call so that relay names removed
	// from the config do not survive a SIGHUP hot-reload into the next run.
	app.mu.Lock()
	app.relays = make(map[string]Relay)
	for name, config := range app.config.Relays {
		slog.Info("Register relay", "name", name, "gpio", config.GPIO)
		r, err := relay.New(config.GPIO)

		if err != nil {
			return fmt.Errorf("failed to register relay %q: %w", name, err)
		}

		app.relays[name] = Relay{
			Relay:       r,
			Description: config.Description,
		}
	}

	app.mu.Unlock()

	// initRoutes should always be called at the end
	slog.Debug("Initializing API routes")
	app.SetupRoutes()
	return nil
}

// Restart returns a read-only channel for restart signals.
func (app *App) Restart() <-chan struct{} {
	return app.restart
}

// Shutdown returns a read-only channel for shutdown signals.
func (app *App) Shutdown() <-chan struct{} {
	return app.shutdown
}

// HandleOSSignals listens for SIGHUP, SIGTERM, and SIGINT signals.
func (app *App) HandleOSSignals() {

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)
		defer signal.Stop(sig) // Cleanup: rollback signal.Notify

		slog.Debug("Starting signal handler")

		// Use select instead of a plain channel receive so the goroutine has
		// two exit paths and always terminates cleanly:
		//   - a signal is received and handled, or
		//   - the context is cancelled externally (e.g. from a concurrent shutdown).
		// Without this, the goroutine would block forever after signal.Reset()
		// on a SIGHUP restart, leaking one goroutine per reload cycle.
		select {
		case receivedSignal := <-sig:
			slog.Info("Received OS signal", "signal", receivedSignal)
			switch receivedSignal {
			case syscall.SIGHUP:
				slog.Info("SIGHUP received, initiating restart")
				app.shutdownProcedure(ModeRestart)
			case syscall.SIGTERM, syscall.SIGINT:
				slog.Info("SIGTERM/SIGINT received, stopping")
				app.shutdownProcedure(ModeStop)
			}
		case <-app.ctx.Done():
			// Context was cancelled externally – exit without triggering
			// a second shutdown procedure.
			slog.Debug("Signal handler: context cancelled, exiting goroutine")
		}
	}()
}

// shutdownProcedure gracefully stops or restarts the app based on mode.
//   - ModeStop: graceful shutdown the web server, Cleanup app resources and exit the application.
//   - ModeRestart: graceful shutdown the web server and Cleanup app resources and restart the application.
func (app *App) shutdownProcedure(mode int) {
	slog.Info("Initiating shutdown", "mode", mode)

	// cancel the application context to stop all running goroutines
	app.cancelFunc()
	app.wg.Wait() //wait for the web server to shutdown before cleaning up resources

	if err := app.Cleanup(); err != nil {
		slog.Error("Cleanup failed", "error", err)
	}

	switch mode {
	case ModeRestart:
		slog.Info("Shutdown complete, restarting")
		app.restart <- struct{}{}
		// Channels are intentionally left open: cmd/main.go receives the restart
		// signal and calls New(), which creates fresh channels for the next lifecycle.
	case ModeStop:
		slog.Info("Module stopped", "module", MODULE, "version", VERSION, "pid", os.Getpid())
		app.shutdown <- struct{}{}
		close(app.shutdown)
	}
}

// Cleanup releases application resources during shutdown or restart.
func (app *App) Cleanup() error {
	var errs error

	// here cleanup your service
	for name, r := range app.relays {
		slog.Info("Cleaning up relay", "name", name)
		if err := r.Close(); err != nil {
			slog.Error("Failed to cleanup relay", "name", name, "error", err)
			errs = errors.Join(errs, err)
		}
	}

	return errs
}
