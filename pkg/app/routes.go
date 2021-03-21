package app

// initRoutes initializes the applications static routes.
// This are the custom business logic routes which normally are different between custom applications.
func (app *App) initRoutes() {
	// create api group
	api := app.web.Group("/api")

	grpSwitch := api.Group("/switch")
	grpSwitch.Patch("/:relay", app.HandleRelayUpdate())

	grpState := api.Group("/state")
	grpState.Get("", app.HandleRelayList())
	grpState.Get("/:relay", app.HandleRelayGet())
}

// initDefaultRoutes initializes the applications default routes.
//  This are the routes which always are the same in every application.
//  Things like user api, version, ...
func (app *App) initDefaultRoutes() {
	root := app.web.Group("/")
	if app.config.Webserver.Webservices["version"] {
		root.Get("/version", app.HandleVersion())
	}
	if app.config.Webserver.Webservices["health"] {
		root.Get("/health", app.HandleHealth())
	}
}
