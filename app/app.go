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
// TODO: increase version number
const (
	VERSION = "1.6.2+20260228"
	MODULE  = "relayhat"

	ModeStop    = 0
	ModeRestart = 1
)

// App is the main application struct.
// App is where the application is wired up.
type App struct {
	wg         sync.WaitGroup // wait group to track running webserver
	baseDir    string         // working directory
	config     *Config        // app configuration
	web        *http.Server   // HTTP server
	restart    chan struct{}  // signals application restart
	shutdown   chan struct{}  // signals application shutdown
	ctx        context.Context
	cancelFunc context.CancelFunc

	relays map[string]Relay
}

// Relay combines the runtime relay instance with its config metadata.
type Relay struct {
	*relay.Relay
	Description string
}

// New checks the Web server URL and initialize the main app structure
func New(config *Config, baseDir string) *App {
	ctx, cancel := context.WithCancel(context.Background())

	return &App{
		config: config,
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

// Init initializes the application.
func (app *App) Init() error {

	// register the relay
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

	// initRoutes should always be called at the end
	slog.Info("Initializing API routes")
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

		slog.Info("Starting signal handler")

		receivedSignal := <-sig
		slog.Info("Received OS signal", "signal", receivedSignal)

		switch receivedSignal {
		case syscall.SIGHUP:
			slog.Info("SIGHUP received, initiating restart")
			app.shutdownProcedure(ModeRestart)
			// reset the signal registration before the program restarts.
			// with program restarts, the HandleOSSignals function is called again and re-registers the signals.
			signal.Reset()

		case syscall.SIGTERM, syscall.SIGINT:
			slog.Info("SIGTERM/SIGINT received, stopping")
			app.shutdownProcedure(ModeStop)
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
	case ModeStop:
		slog.Info("Module stopped", "module", MODULE, "version", VERSION, "pid", os.Getpid())
		app.shutdown <- struct{}{}
		close(app.shutdown)
		close(app.restart)
	}
}

// Cleanup releases application resources.
// It's called when the application is shutdown or restarted.
// Should be used to free up resources.
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
