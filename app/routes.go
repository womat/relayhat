package app

import (
	"log/slog"
	"net/http"

	"github.com/womat/golib/web"
)

// SetupRoutes configures the application's routes and shared HTTP middleware.
func (app *App) SetupRoutes() {
	webCfg := web.Config{
		ApiKey:    app.config.Webserver.ApiKey,
		JwtSecret: app.config.Webserver.JwtSecret,
		JwtID:     app.config.Webserver.JwtID,
		AppName:   MODULE,
	}

	mux := http.NewServeMux()

	// Preflight CORS requests
	mux.Handle("OPTIONS /", web.HandlePreflight())

	// Dev-only Swagger documentation (only registered with -tags swagger)
	app.registerSwaggerRoute(mux)

	// Public routes
	mux.Handle("GET /version", app.HandleVersion())

	// Protected routes
	mux.Handle("GET /health", web.WithAuth(app.HandleHealth(), webCfg))
	mux.Handle("GET /relays", web.WithAuth(app.HandleRelayGetAll(), webCfg))
	mux.Handle("GET /relays/{name}", web.WithAuth(app.HandleRelayGetOne(), webCfg))
	mux.Handle("PATCH /relays/{name}/{state}", web.WithAuth(app.HandleRelaySet(), webCfg))

	// Apply global middleware: CORS + IP filter
	handler := web.WithCORS(mux)
	handler = web.WithIPFilter(handler, app.config.Webserver.AllowedIPs, app.config.Webserver.BlockedIPs)
	handler = WithLogging(handler)
	app.web.Handler = handler
}

// WithLogging logs basic request metadata before calling the next handler.
func WithLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("Incoming web request",
			"method", r.Method,
			"path", r.URL.Path,
			"client_ip", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}
