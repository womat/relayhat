// Package main is the entry point for the relayhat application.
//
// relayhat exposes a secured HTTPS REST API to control BC Robotics Relay HATs
// attached to a Raspberry Pi via GPIO. It supports both the 2-channel HAT
// (Pi Zero, GPIO 4/17) and the 4-channel HAT (Pi 3/4/5, GPIO 4/17/22/27).
//
// # Startup
//
// The application loads its configuration from a YAML file (default:
// /opt/relayhat/etc/config.yaml), initialises structured logging via slog,
// registers GPIO-backed relay handlers, and starts a TLS HTTP server.
//
// # Signals
//
//   - SIGHUP  – hot-reload: validates the config file first and keeps running
//     unchanged if it is broken; relays that stay configured keep their state
//   - SIGTERM – graceful shutdown, switches all relays off
//   - SIGINT  – graceful shutdown (Ctrl+C)
//
// # CLI Flags
//
//	-config  path to config file (default: /opt/relayhat/etc/config.yaml,
//	         or the CONFIG_FILE env variable if set; the flag wins over both)
//	-debug   force log level "debug" and output to stdout
//	-version print version and exit
//	-about   print build metadata and exit
//	-help    print README and exit
//
// # Build Tags
//
//	swagger  include Swagger UI at /swagger/
//
// Build-time variables injected via -ldflags:
//
//	main.buildDate    – UTC timestamp of the build
//	main.buildCommit  – short Git commit hash
//	app.VERSION       – version from the Git tag
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/womat/relayhat/app"
	"gopkg.in/yaml.v3"
)

// Readme embeds the README.md file for the --help flag
//
//go:embed README.md
var Readme string

// buildDate and buildCommit are injected at build time via -ldflags
var (
	buildDate   = "dev"
	buildCommit = "none"
)

// @securityDefinitions.apikey	ApiKeyAuth
// @in							header
// @name						X-API-Key
func main() {
	// Parse command line flags.
	flags := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flags.SetOutput(os.Stdout)

	about := flags.Bool("about", false, "Print app details and exit")
	help := flags.Bool("help", false, "Print a help message and exit")
	version := flags.Bool("version", false, "Print the app version and exit")
	debug := flags.Bool("debug", false, "Enable debug logging to stdout (overrides log settings from the config file)")
	configFile := flags.String("config", filepath.Join("/opt", app.MODULE, "etc", "config.yaml"), "Specify the path to the config file")

	if envCfg := os.Getenv("CONFIG_FILE"); envCfg != "" {
		*configFile = envCfg
	}

	if err := flags.Parse(os.Args[1:]); err != nil {
		fmt.Println("Error parsing flags:", err)
		flags.Usage()
		os.Exit(1)
	}

	switch {
	case *about:
		fmt.Println(About())
		os.Exit(0)
	case *version:
		fmt.Println(app.VERSION)
		os.Exit(0)
	case *help:
		fmt.Println(Readme)
		os.Exit(0)
	}

	// Run main application loop
	os.Exit(run(*configFile, *debug))
}

// run initializes configuration, logging, and the application loop.
// It supports hot-reloading of the configuration and handles graceful shutdown.
func run(configFile string, debug bool) int {

	// closeLog releases the current log file, if logging goes to one.
	closeLog := func() error { return nil }
	defer func() { _ = closeLog() }()

	// inherited holds the relays a restart left open until the next App takes them over.
	// If run returns before that, they are closed here, which switches them off.
	var inherited map[int]*app.Relay
	defer func() {
		for _, r := range inherited {
			_ = r.Close()
		}
	}()

	fmt.Printf("Starting %s %s\n", app.MODULE, app.VERSION)
	fmt.Printf("Loading configuration from: %s\n", configFile)

	// Subscribe once for the whole process, not per App: between two lifecycles no App is
	// listening, and without a subscription a SIGTERM or a second SIGHUP in that gap would end
	// the process with the default action, before the relays are released. Here the signal
	// waits in the buffer and the next App handles it.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signals)

	for {
		// Reload configuration on every restart
		config, err := loadConfig(configFile, debug)
		if err != nil {
			fmt.Printf("Failed to load config file %s: %s\n", configFile, err.Error())
			return 1
		}

		// Switch to the new logger before closing the previous log file, so no line written
		// in between goes to a file that is already closed.
		logger, closeNew, err := newLogger(config.LogDestination, config.LogLevel)
		if err != nil {
			fmt.Printf("Failed to initialize logger: %s\n", err.Error())
			return 1
		}
		slog.SetDefault(logger)
		_ = closeLog()
		closeLog = closeNew
		slog.Info("Logging initialized/reloaded", "logLevel", config.LogLevel)

		for _, warning := range config.Warnings() {
			slog.Warn("Configuration warning", "warning", warning)
		}

		// A SIGHUP restart only goes ahead when the config file still loads and validates;
		// otherwise the running App keeps going with its current configuration.
		checkReload := func() error {
			_, err := loadConfig(configFile, debug)
			return err
		}

		// Create and run the application. From here on the App owns the inherited relays.
		a, err := app.New(config, signals, checkReload, inherited).Run()
		inherited = nil
		if err != nil {
			slog.Error("Critical error occurred, shutting down", "error", err)
			return 1
		}

		// Wait for restart or shutdown signals
		select {
		case <-a.Restart():
			slog.Info("Reloading configuration", "configFile", configFile)
			inherited = a.Handover()
			time.Sleep(time.Second) // prevent tight restart loops
		case <-a.Shutdown():
			slog.Info("Shutdown requested")
			return 0
		}
	}
}

// About returns build and runtime metadata as YAML.
func About() string {
	info := map[string]string{
		"Author":   "Wolfgang Mathe",
		"Binary":   filepath.Join("/opt", app.MODULE, "bin", app.MODULE),
		"Date":     buildDate,
		"Commit":   buildCommit,
		"Desc":     app.MODULE + " switches BC Robotics Relay HATs on a Raspberry Pi through an HTTPS REST API",
		"Help":     filepath.Join("/opt", app.MODULE, "bin", app.MODULE) + " --help",
		"Main":     filepath.Join("/opt/src", app.MODULE, "cmd", "main.go"),
		"ProgLang": runtime.Version(),
		"Repo":     "https://github.com/womat/" + app.MODULE + ".git",
		"Version":  app.VERSION,
	}

	b, err := yaml.Marshal(info)
	if err != nil {
		return fmt.Sprintf("Failed to marshal About info: %v", err)
	}
	return string(b)
}

// loadConfig loads the configuration from a YAML file, applies overrides and validates the configuration values.
//
// loadConfig forces debug logging to stdout when debug mode is enabled.
func loadConfig(configFile string, debug bool) (*app.Config, error) {

	config, err := app.LoadConfig(configFile)
	if err != nil {
		return nil, err
	}

	if debug {
		config.LogLevel = "debug"
		config.LogDestination = "stdout"
	}

	if err = config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}

// newLogger returns a text logger writing to dest - "stdout", "stderr", "null" or a file path,
// opened for appending - at the given level (debug, info, warn/warning, error; anything else
// is info). Source locations are added at debug level only. The returned function closes the
// log file and is a no-op for the other destinations.
func newLogger(dest, level string) (*slog.Logger, func() error, error) {
	var out io.Writer
	closeFn := func() error { return nil }

	switch strings.ToLower(strings.TrimSpace(dest)) {
	case "stdout":
		out = os.Stdout
	case "stderr":
		out = os.Stderr
	case "null":
		out = io.Discard
	default:
		file, err := os.OpenFile(dest, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
		if err != nil {
			return nil, nil, err
		}
		out, closeFn = file, file.Close
	}

	var lvl slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	handler := slog.NewTextHandler(out, &slog.HandlerOptions{AddSource: lvl == slog.LevelDebug, Level: lvl})
	return slog.New(handler), closeFn, nil
}
