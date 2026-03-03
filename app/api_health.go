// This package is designed to support Kubernetes-style health checks by exposing
// separate Liveness (/live) and Readiness (/ready) endpoints. Liveness checks
// verify that the application is running, whereas Readiness checks ensure that
// the application and its dependencies (DB, GPIO, etc.) are ready to serve traffic.
//
// The health data includes metrics such as memory usage, goroutine count, application
// version, hostname, Go runtime version, and operating system.

package app

import (
	"net/http"
	"relayhat/app/service/health"

	"github.com/womat/golib/web"
)

// HandleHealth returns the current health data of the application.
//
//	@Summary		Get health data
//	@Description	Retrieves memory usage, goroutine count, version, hostname, Go runtime version, and OS.
//	@Tags			info
//	@Success		200	{object}	health.Model	"Health data successfully retrieved"
//	@Router			/health [get]
func (app *App) HandleHealth() http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			resp := health.GetCurrentHealth(MODULE, VERSION)
			web.Encode(w, http.StatusOK, resp)
		},
	)
}
