// Package app provides the main application wiring for relayhat.
//
// It opens the configured relays, serves the HTTPS API and handles the OS signals for
// graceful stops and configuration reloads. One App lives for one configuration;
// cmd/main.go builds a new one on every reload and hands the open relays over to it.
//
// Usage:
//
//	cfg, err := app.LoadConfig(file)          // then cfg.Validate()
//	signals := make(chan os.Signal, 1)
//	signal.Notify(signals, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)
//	a, err := app.New(cfg, signals, checkReload, inherited).Run()
//	select {
//	case <-a.Restart():  // inherited = a.Handover(), build the next App
//	case <-a.Shutdown(): // exit
//	}
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"os"
	"slices"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/womat/relayhat/pkg/relay"
)

// VERSION is the application version, following semantic versioning
// as described in https://semver.org/.
//
// It is not maintained in source: the Git tag is the single source of truth and
// the value is injected at build time via -ldflags (see Makefile and
// .goreleaser.yaml). The "dev" default applies to builds made without them.
var VERSION = "dev"

const (
	MODULE = "relayhat"

	ModeStop    = 0
	ModeRestart = 1
)

// App holds the application's runtime state and lifecycle dependencies.
type App struct {
	wg          sync.WaitGroup   // tracks the web server goroutine
	config      *Config          // app configuration
	web         *http.Server     // HTTP server
	signals     <-chan os.Signal // OS signals, subscribed once by the caller for all lifecycles
	checkReload func() error     // loads and validates the config file before a SIGHUP restart
	serverErr   chan error       // reports a web server that stopped on its own
	restart     chan struct{}    // signals application restart
	shutdown    chan struct{}    // signals application shutdown
	ctx         context.Context
	cancelFunc  context.CancelFunc

	now        func() time.Time                                         // time.Now; replaced in tests
	openRelay  func(gpio int) (*relay.Relay, error)                     // relay.New; replaced in tests
	lookupAddr func(ctx context.Context, addr string) ([]string, error) // reverse DNS; replaced in tests
	inherited  map[int]*Relay                                           // open relays of the previous App by GPIO, consumed by Init
	handover   map[int]*Relay                                           // open relays for the next App, set on restart

	mu     sync.RWMutex // protects app.relays
	relays map[string]*Relay

	stateMu sync.Mutex // serializes writes of the state file
}

// Relay is an open relay with its configuration and its last switch. It outlives the App
// that opened it when it is handed over on a reload.
type Relay struct {
	*relay.Relay
	Config RelayConfig // replaced by Init with the configuration of the App that uses it

	mu     sync.Mutex // serializes switching and protects change
	change Change
}

// errSwitchLocked is returned by switchTo while RelayConfig.MinSwitchInterval has not passed
// since the last switch.
type errSwitchLocked struct {
	remaining time.Duration
	interval  time.Duration
}

func (e errSwitchLocked) Error() string {
	return fmt.Sprintf("switching locked for %s (minSwitchInterval %s)", e.remaining, e.interval)
}

// LastChange returns the relay's last switch; its Time is zero when it is not known.
func (r *Relay) LastChange() Change {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.change
}

// LockRemaining returns how long the relay may not be switched at now, rounded up to whole
// seconds; 0 when it may.
func (r *Relay) LockRemaining(now time.Time) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lockRemaining(now)
}

// lockRemaining is LockRemaining for a caller holding r.mu. The lock runs from the last switch,
// whoever made it; without a known last switch there is none.
func (r *Relay) lockRemaining(now time.Time) time.Duration {
	interval := r.Config.MinSwitchInterval
	if interval <= 0 || r.change.Time.IsZero() {
		return 0
	}
	left := r.change.Time.Add(interval).Sub(now)
	if left <= 0 {
		return 0
	}
	if frac := left % time.Second; frac > 0 {
		left += time.Second - frac
	}
	return left
}

// switchTo switches the relay to want and records change as its last switch. A relay already
// in state want is left alone and its last switch kept, also while it is locked; otherwise a
// switch within MinSwitchInterval of the last one fails with errSwitchLocked. Checking and
// switching happen under r.mu, so two concurrent requests cannot both pass the lock.
func (r *Relay) switchTo(want relay.State, change Change) (from relay.State, switched bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if from, err = r.GetState(); err != nil {
		return relay.Unknown, false, err
	}
	if from == want {
		return from, false, nil
	}
	if left := r.lockRemaining(change.Time); left > 0 {
		return from, false, errSwitchLocked{remaining: left, interval: r.Config.MinSwitchInterval}
	}

	if want == relay.On {
		err = r.TurnOn()
	} else {
		err = r.TurnOff()
	}
	if err != nil {
		return from, false, err
	}
	r.change = change
	return from, true, nil
}

// setChange records the relay's last switch.
func (r *Relay) setChange(c Change) {
	r.mu.Lock()
	r.change = c
	r.mu.Unlock()
}

// setHost adds the client's host name to the last switch, unless the relay has been switched
// again since then, at.
func (r *Relay) setHost(at time.Time, host string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.change.Time.Equal(at) {
		return false
	}
	r.change.Host = host
	return true
}

// New initializes the App struct but does not start services.
//
// signals must already be subscribed (signal.Notify) to SIGHUP, SIGTERM and SIGINT, and stay
// subscribed across restarts: a signal arriving while one App is torn down and the next is
// built then waits in the channel for the next App, instead of hitting the default action,
// which would end the process without releasing the relays.
//
// checkReload is called on SIGHUP before anything is torn down. If it reports an error, the
// restart is refused and the App keeps running with its current configuration. Passing nil
// skips the check.
//
// inherited holds the still open relays of the previous App (see Handover), keyed by GPIO.
// Init takes over every relay whose GPIO is still configured, so it keeps its state and its
// last switch across the reload, and closes the others. The App owns the map from here on;
// nil is a cold start.
func New(config *Config, signals <-chan os.Signal, checkReload func() error, inherited map[int]*Relay) *App {
	ctx, cancel := context.WithCancel(context.Background())

	return &App{
		config:      config,
		signals:     signals,
		checkReload: checkReload,
		web: &http.Server{
			Addr: net.JoinHostPort(config.Webserver.ListenHost, strconv.Itoa(config.Webserver.ListenPort)),
		},
		serverErr:  make(chan error, 1),
		restart:    make(chan struct{}),
		shutdown:   make(chan struct{}),
		ctx:        ctx,
		cancelFunc: cancel,
		now:        time.Now,
		openRelay:  relay.New,
		lookupAddr: net.DefaultResolver.LookupAddr,
		inherited:  inherited,
		relays:     make(map[string]*Relay),
	}
}

// Run starts the application.
func (app *App) Run() (*App, error) {
	if err := app.Init(); err != nil {
		return app, err
	}

	// handle the OS signals
	app.HandleOSSignals()

	slog.Info("Starting web server", "url", app.web.Addr)
	if err := app.StartWebServer(); err != nil {
		slog.Error("Web server failed to start", "url", app.web.Addr, "error", err)
		// Stop the signal handler and switch the relays off, the caller exits.
		app.cancelFunc()
		if cerr := app.Cleanup(); cerr != nil {
			slog.Error("Cleanup failed", "error", cerr)
		}
		return app, err
	}

	slog.Info("Module started successfully",
		"module", MODULE,
		"version", VERSION,
		"pid", os.Getpid(),
	)
	return app, nil
}

// Init opens the configured relays, or takes them over from the previous App, switches the
// opened ones to their start state and initializes the HTTP routes.
func (app *App) Init() (err error) {
	relays := make(map[string]*Relay, len(app.config.Relays))

	defer func() {
		// Close what is not used: inherited relays whose GPIO is no longer configured and,
		// if Init fails, every relay it has already opened or taken over.
		if err != nil {
			app.inherited = mergeRelays(app.inherited, relays)
		}
		for gpio, r := range app.inherited {
			slog.Info("Closing relay that is no longer configured", "gpio", gpio)
			if cerr := r.Close(); cerr != nil {
				slog.Error("Failed to close relay", "gpio", gpio, "error", cerr)
			}
		}
		app.inherited = nil
	}()

	// Relays opened here, as opposed to taken over; only they get their start state.
	var opened []string

	// Sorted, so the log shows the relays in the same order on every start.
	for _, name := range slices.Sorted(maps.Keys(app.config.Relays)) {
		cfg := app.config.Relays[name]

		if r, ok := app.inherited[cfg.GPIO]; ok {
			delete(app.inherited, cfg.GPIO)
			state, _ := r.GetState()
			slog.Info("Take over relay", "name", name, "gpio", cfg.GPIO, "state", state)
			r.Config = cfg
			relays[name] = r
			continue
		}

		slog.Info("Register relay", "name", name, "gpio", cfg.GPIO)
		r, err := app.openRelay(cfg.GPIO)
		if err != nil {
			return fmt.Errorf("failed to register relay %q: %w", name, err)
		}
		relays[name] = &Relay{Relay: r, Config: cfg}
		opened = append(opened, name)
	}

	if err = app.applyStartStates(relays, opened); err != nil {
		return err
	}

	app.mu.Lock()
	app.relays = relays
	app.mu.Unlock()

	// Write the state file once, so it exists from the first start on, holds the start states
	// and a missing directory or missing write permission shows up now instead of at the first
	// switch.
	app.saveStates()

	// initRoutes should always be called at the end
	slog.Debug("Initializing API routes")
	app.SetupRoutes()
	return nil
}

// applyStartStates switches the opened relays to their configured start state. They come
// from openRelay switched off, so only "on" and a stored "on" for "last" need an action.
//
// A relay keeps the last switch from the state file when it starts in the state the file
// holds for it, so a restart does not hide when and by whom it was switched; otherwise the
// start is its last switch.
func (app *App) applyStartStates(relays map[string]*Relay, opened []string) error {
	var stored map[string]savedState
	if app.config.StateFile != "" {
		stored = readRelayStates(app.config.StateFile)
	}
	now := app.now()

	for _, name := range opened {
		mode := app.config.Relays[name].startMode()
		saved, hasSaved := stored[name]

		state := relay.Off
		switch mode {
		case StartOn:
			state = relay.On
		case StartLast:
			if hasSaved {
				state = saved.State
			}
		}

		if state == relay.On {
			if err := relays[name].TurnOn(); err != nil {
				return fmt.Errorf("failed to switch relay %q to its start state: %w", name, err)
			}
		}

		if hasSaved && saved.State == state && !saved.Change.Time.IsZero() {
			relays[name].setChange(saved.Change)
		} else {
			relays[name].setChange(Change{Time: now, Source: SourceStart})
		}
		slog.Info("Relay start state applied", "name", name, "startState", mode, "state", state)
	}
	return nil
}

// saveStates writes the current state and last switch of every relay to the state file. It
// does nothing without a stateFile, and a failure is logged only: the relay is switched
// anyway, just its state will not survive a restart.
func (app *App) saveStates() {
	if app.config.StateFile == "" {
		return
	}

	app.stateMu.Lock()
	defer app.stateMu.Unlock()

	// Read the states under stateMu, so the last write always holds the latest states, even
	// when two switches save at the same time.
	app.mu.RLock()
	states := make(map[string]savedState, len(app.relays))
	for name, r := range app.relays {
		if s, err := r.GetState(); err == nil {
			states[name] = savedState{State: s, Change: r.LastChange()}
		}
	}
	app.mu.RUnlock()

	if err := saveRelayStates(app.config.StateFile, states); err != nil {
		slog.Error("Failed to save relay states, they will not survive a restart", "file", app.config.StateFile, "error", err)
	}
}

// mergeRelays adds the relays of named to byGPIO and returns it.
func mergeRelays(byGPIO map[int]*Relay, named map[string]*Relay) map[int]*Relay {
	if byGPIO == nil {
		byGPIO = make(map[int]*Relay, len(named))
	}
	for _, r := range named {
		byGPIO[r.GPIO()] = r
	}
	return byGPIO
}

// Restart returns a read-only channel for restart signals.
func (app *App) Restart() <-chan struct{} {
	return app.restart
}

// Shutdown returns a read-only channel for shutdown signals.
func (app *App) Shutdown() <-chan struct{} {
	return app.shutdown
}

// Handover returns the relays a restart left open, keyed by GPIO, for the next App (see New).
// It is empty after a stop.
func (app *App) Handover() map[int]*Relay {
	return app.handover
}

// HandleOSSignals handles SIGHUP (restart), SIGTERM and SIGINT (stop) from app.signals, and
// restarts the App when the web server stopped on its own.
//
// The subscription itself belongs to the caller and outlives this App, so nothing here
// stops or resets it; one goroutine per App consumes at most one signal. Being the only
// caller of shutdownProcedure, it also rules out two shutdowns running at once.
func (app *App) HandleOSSignals() {

	go func() {
		slog.Debug("Starting signal handler")

		// Use select instead of a plain channel receive so the goroutine has
		// two exit paths and always terminates cleanly:
		//   - a signal or a server error is received and handled, or
		//   - the context is cancelled externally (e.g. from a failed start).
		// Without the second path the goroutine would outlive its App and take
		// the next signal away from the App that replaced it. The loop only
		// continues after a SIGHUP whose config was rejected.
		for {
			select {
			case receivedSignal := <-app.signals:
				slog.Info("Received OS signal", "signal", receivedSignal)
				switch receivedSignal {
				case syscall.SIGHUP:
					if app.checkReload != nil {
						if err := app.checkReload(); err != nil {
							slog.Error("Config reload rejected, keeping the running configuration", "error", err)
							continue
						}
					}
					slog.Info("SIGHUP received, initiating restart")
					app.shutdownProcedure(ModeRestart)
				case syscall.SIGTERM, syscall.SIGINT:
					slog.Info("SIGTERM/SIGINT received, stopping")
					app.shutdownProcedure(ModeStop)
				}
				return
			case err := <-app.serverErr:
				slog.Error("Web server stopped unexpectedly, initiating restart", "error", err)
				app.shutdownProcedure(ModeRestart)
				return
			case <-app.ctx.Done():
				// Context was cancelled externally – exit without triggering
				// a second shutdown procedure.
				slog.Debug("Signal handler: context cancelled, exiting goroutine")
				return
			}
		}
	}()
}

// shutdownProcedure gracefully stops or restarts the app based on mode.
//   - ModeStop: shut down the web server, switch the relays off and exit the application.
//   - ModeRestart: shut down the web server and hand the open relays over to the next App,
//     so a reload does not switch them.
func (app *App) shutdownProcedure(mode int) {
	slog.Info("Initiating shutdown", "mode", mode)

	// cancel the application context to stop all running goroutines
	app.cancelFunc()
	// Wait for the web server, so no request switches a relay that is handed over or closed.
	app.wg.Wait()

	if mode == ModeRestart {
		app.mu.Lock()
		app.handover = mergeRelays(nil, app.relays)
		app.relays = make(map[string]*Relay)
		app.mu.Unlock()
	}

	if err := app.Cleanup(); err != nil {
		slog.Error("Cleanup failed", "error", err)
	}

	switch mode {
	case ModeRestart:
		slog.Info("Shutdown complete, restarting", "relaysHandedOver", len(app.handover))
		app.restart <- struct{}{}
		// Channels are intentionally left open: cmd/main.go receives the restart
		// signal and calls New(), which creates fresh channels for the next lifecycle.
	case ModeStop:
		slog.Info("Module stopped", "module", MODULE, "version", VERSION, "pid", os.Getpid())
		app.shutdown <- struct{}{}
		close(app.shutdown)
	}
}

// Cleanup closes the relays the App still owns, which switches them off. Relays handed
// over for a restart are not among them.
func (app *App) Cleanup() error {
	var errs error

	app.mu.Lock()
	defer app.mu.Unlock()

	for name, r := range app.relays {
		slog.Info("Closing relay", "name", name)
		if err := r.Close(); err != nil {
			slog.Error("Failed to close relay", "name", name, "error", err)
			errs = errors.Join(errs, err)
		}
	}
	app.relays = make(map[string]*Relay)

	return errs
}
