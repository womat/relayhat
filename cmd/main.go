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
//   - SIGHUP  – hot-reload: closes GPIO pins, reloads config, restarts server
//   - SIGTERM – graceful shutdown
//   - SIGINT  – graceful shutdown (Ctrl+C)
//
// # CLI Flags
//
//	-config  path to config file (default: /opt/relayhat/etc/config.yaml)
//	         overridden by CONFIG_FILE env variable
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
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"relayhat/app"
	"runtime"
	"time"

	"github.com/womat/golib/xlog"
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

	var logger *xlog.LoggerWrapper
	defer func() {
		if logger != nil {
			logger.Close()
		}
	}()

	fmt.Printf("Starting %s %s\n", app.MODULE, app.VERSION)
	fmt.Printf("Loading configuration from: %s\n", configFile)

	for {
		// Reload configuration on every restart
		config, err := loadConfig(configFile, debug)
		if err != nil {
			fmt.Printf("Failed to load config file %s: %s\n", configFile, err.Error())
			return 1
		}

		// Close previous logger if exists
		if logger != nil {
			logger.Close()
		}

		// Initialize logger
		if logger, err = xlog.Init(config.LogDestination, config.LogLevel); err != nil {
			fmt.Printf("Failed to initialize logger: %s\n", err.Error())
			return 1
		}

		slog.SetDefault(logger.Logger)
		slog.Info("Logging initialized/reloaded", "logLevel", config.LogLevel)

		// Create and run the application
		a, err := app.New(config, filepath.Join("/opt", app.MODULE)).Run()
		if err != nil {
			slog.Error("Critical error occurred, shutting down", "error", err)
			return 1
		}

		// Wait for restart or shutdown signals
		select {
		case <-a.Restart():
			slog.Info("Reloading configuration", "configFile", configFile)
			time.Sleep(time.Second) // prevent tight restart loops
		case <-a.Shutdown():
			slog.Debug("Shutdown requested")
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
		"Desc":     app.MODULE + " is demo app",
		"Help":     filepath.Join("/opt", app.MODULE, "bin", app.MODULE) + " --help",
		"Main":     filepath.Join("/opt/src", app.MODULE, "cmd", app.MODULE, "main.go"),
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
