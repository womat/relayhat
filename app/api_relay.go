package app

import (
	"errors"
	"log/slog"
	"net/http"
	"relayhat/pkg/relay"
	"sort"

	"github.com/womat/golib/web"
)

var (
	errRelayNotFound = errors.New("relay not found")
)

type HTTPResponse struct {
	Name        string `json:"name"`
	State       string `json:"state"`
	Description string `json:"description"`
}

// HandleRelayGetOne returns the state of a single relay by name.
//
//	@Summary		Get relay state
//	@Description	Returns name, state (on/off) and description for the given relay.
//	@Tags			relay
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			name	path		string			true	"Relay name (e.g. relay1)"
//	@Success		200		{object}	HTTPResponse	"Relay state"
//	@Failure		401		{string}	string			"Unauthorized"
//	@Failure		404		{string}	string			"Relay not found"
//	@Failure		500		{string}	string			"Internal server error"
//	@Router			/relay/{name} [get]
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

// HandleRelayGetAll returns the state of all configured relays.
//
//	@Summary		List all relays
//	@Description	Returns name, state (on/off) and description for every configured relay.
//	@Tags			relay
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Success		200	{array}		HTTPResponse	"List of all relays"
//	@Failure		401	{string}	string			"Unauthorized"
//	@Failure		500	{string}	string			"Internal server error"
//	@Router			/relay [get]
func (app *App) HandleRelayGetAll() http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			// sort relay names for consistent output
			names := make([]string, 0, len(app.relays))
			for n := range app.relays {
				names = append(names, n)
			}
			sort.Strings(names)

			res := make([]HTTPResponse, 0, len(names))
			for _, n := range names {
				resp, stat, err := app.relayGet(n)
				if err != nil {
					web.Encode(w, stat, err.Error())
					return
				}
				res = append(res, resp)
			}

			web.Encode(w, http.StatusOK, res)
		})
}

// HandleRelaySet sets the state of a relay to on or off.
//
//	@Summary		Set relay state
//	@Description	Switches the given relay to the requested state. Valid states: on, off.
//	@Tags			relay
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			name	path		string			true	"Relay name (e.g. relay1)"
//	@Param			state	path		string			true	"Target state"	Enums(on, off)
//	@Success		200		{object}	HTTPResponse	"Updated relay state"
//	@Failure		400		{string}	string			"Bad request – unknown state"
//	@Failure		401		{string}	string			"Unauthorized"
//	@Failure		404		{string}	string			"Relay not found"
//	@Failure		500		{string}	string			"Internal server error"
//	@Router			/relay/{name}/{state} [put]
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

			slog.Info("Relay set", "name", name, "state", state)
			web.Encode(w, http.StatusOK, res)
		})
}

func (app *App) relayGet(name string) (HTTPResponse, int, error) {
	r, ok := app.relays[name]
	if !ok {
		return HTTPResponse{}, http.StatusNotFound, errRelayNotFound
	}

	s, err := r.GetState()
	if err != nil {
		return HTTPResponse{}, http.StatusInternalServerError, err
	}

	return HTTPResponse{
		Name:        name,
		State:       s.String(),
		Description: r.Description,
	}, http.StatusOK, nil
}

func (app *App) relaySet(name, state string) (HTTPResponse, int, error) {
	r, ok := app.relays[name]
	if !ok {
		return HTTPResponse{}, http.StatusNotFound, errRelayNotFound
	}

	switch state {
	case "on":
		if err := r.TurnOn(); err != nil {
			return HTTPResponse{}, http.StatusInternalServerError, err

		}
	case "off":
		if err := r.TurnOff(); err != nil {
			return HTTPResponse{}, http.StatusInternalServerError, err
		}
	default:
		return HTTPResponse{}, http.StatusBadRequest, relay.ErrUnknownState
	}

	return app.relayGet(name)
}
