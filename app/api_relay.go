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
//	@Router			/relays/{name} [get]
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
//	@Router			/relays [get]
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

// HandleRelaySet sets the state of a specific relay.
//
//	@Summary		Set relay state
//	@Description	Sets the state of a specific relay by name.
//	@Tags			relays
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			name	path		string			true	"Relay name"
//	@Param			state	header		string			true	"Relay state (on/off)"
//	@Success		200		{object}	HTTPResponse	"Relay state successfully set"
//	@Failure		400		{string}	string			"Bad request"
//	@Failure		401		{string}	string			"Unauthorized"
//	@Failure		404		{string}	string			"Relay not found"
//	@Router			/relays/{name} [put]
func (app *App) HandleRelaySet() http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {

			name := r.PathValue("name")
			state := r.Header.Get("state")

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
