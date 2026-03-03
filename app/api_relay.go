package app

import (
	"errors"
	"net/http"

	"github.com/womat/golib/web"
)

var (
	errUnknownState  = errors.New("unknown state")
	errRelayNotFound = errors.New("relay not found")
)

type httpResponse struct {
	Name        string `json:"name"`
	State       string `json:"state"`
	Description string `json:"description"`
}

func (app *App) HandleRelayGetOne() http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			name := r.PathValue("name")

			res, stat, err := app.relayGet(name)
			if err != nil {
				web.Encode(w, stat, err.Error())
				return
			}

			web.Encode(w, http.StatusOK, res)
		})
}

func (app *App) HandleRelayGetAll() http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {

			res := make([]httpResponse, 0, len(app.relays))

			for n := range app.relays {
				r, stat, err := app.relayGet(n)
				if err != nil {
					web.Encode(w, stat, err.Error())
					return
				}

				res = append(res, r)
			}

			web.Encode(w, http.StatusOK, res)
		})
}

func (app *App) HandleRelaySet() http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {

			name := r.PathValue("name")
			state := r.PathValue("state")

			res, stat, err := app.relaySet(name, state)
			if err != nil {
				web.Encode(w, stat, err.Error())
				return
			}

			web.Encode(w, http.StatusOK, res)
		})
}

func (app *App) relayGet(name string) (httpResponse, int, error) {
	relay, ok := app.relays[name]
	if !ok {
		return httpResponse{}, http.StatusNotFound, errRelayNotFound
	}

	s, err := relay.GetState()
	if err != nil {
		return httpResponse{}, http.StatusInternalServerError, err
	}

	return httpResponse{
		Name:        name,
		State:       s.String(),
		Description: relay.Description,
	}, http.StatusOK, nil
}

func (app *App) relaySet(name, state string) (httpResponse, int, error) {
	relay, ok := app.relays[name]
	if !ok {
		return httpResponse{}, http.StatusNotFound, errRelayNotFound
	}

	switch state {
	case "on":
		if err := relay.TurnOn(); err != nil {
			return httpResponse{}, http.StatusInternalServerError, err

		}
	case "off":
		if err := relay.TurnOff(); err != nil {
			return httpResponse{}, http.StatusInternalServerError, err
		}
	default:
		return httpResponse{}, http.StatusBadRequest, errUnknownState
	}

	return app.relayGet(name)
}
