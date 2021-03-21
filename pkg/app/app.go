package app

import (
	"github.com/gofiber/fiber/v2"
	"github.com/womat/debug"
	"log"
	"net/url"
	"os"
	"relayhat/pkg/app/config"
	"relayhat/pkg/app/relay"
)

// App is the main application struct.
// App is where the application is wired up.
type App struct {
	// web is the fiber web framework instance
	web *fiber.App

	// config is the application configuration
	config *config.Config

	// urlParsed contains the parsed Config.Url parameter
	// and makes it easier to get params out of e.g.
	// url: https://0.0.0.0:7844/?minTls=1.2&bodyLimit=50MB
	urlParsed *url.URL

	relays *relay.Handler

	// restart signals application restart
	restart chan struct{}
	// shutdown signals application shutdown
	shutdown chan struct{}
	// power signals to check power consumption fpr forced activation
}

// New checks the Web server URL and initialize the main app structure
func New(config *config.Config) *App {
	u, err := url.Parse(config.Webserver.URL)
	if err != nil {
		log.Printf("Error parsing url %q: %s", config.Webserver.URL, err.Error())
		os.Exit(1)
	}

	return &App{
		config:    config,
		urlParsed: u,

		web:    fiber.New(),
		relays: func() *relay.Handler { rs, _ := relay.New(); return rs }(),

		restart:  make(chan struct{}),
		shutdown: make(chan struct{}),
	}
}

// Run starts the application.
func (app *App) Run() error {
	if err := app.init(); err != nil {
		return err
	}

	go app.runWebServer()

	return nil
}

// init initializes the application.
func (app *App) init() error {
	if err := app.relays.Connect(app.config.Relay); err != nil {
		return err
	}

	// initRoutes and initDefaultRoutes should be always called last because it may access things like app.api
	// which must be initialized before in initAPI()
	app.initDefaultRoutes()
	app.initRoutes()

	return nil
}

// Restart returns the read only restart channel.
// Restart is used to be able to react on application restart. (see cmd/main.go)
func (app *App) Restart() <-chan struct{} {
	debug.InfoLog.Println("call function restart()")
	return app.restart
}

// Shutdown returns the read only shutdown channel.
// Shutdown is used to be able to react on application shutdown. (see cmd/main.go)
func (app *App) Shutdown() <-chan struct{} {
	debug.InfoLog.Println("call function Shutdown()")
	return app.shutdown
}
