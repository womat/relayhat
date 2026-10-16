package app

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/womat/golib/v2/web"
	"github.com/womat/relayhat/pkg/relay"
)

var (
	errRelayNotFound = errors.New("relay not found")
	errInvalidState  = errors.New("invalid state: must be \"on\" or \"off\"")
	errShuttingDown  = errors.New("shutting down, try again")
)

// HTTPResponse is the JSON representation of a relay.
type HTTPResponse struct {
	Name        string          `json:"name"`
	State       string          `json:"state"`
	Description string          `json:"description"`
	GPIO        int             `json:"gpio"`
	Display     Display         `json:"display"`
	LastChange  *LastChangeInfo `json:"lastChange,omitempty"` // absent when unknown
	Lock        *LockInfo       `json:"lock,omitempty"`       // absent without minSwitchInterval
}

// LockInfo is the switch lock of a relay, see RelayConfig.MinSwitchInterval.
type LockInfo struct {
	IntervalSeconds  float64 `json:"intervalSeconds"`  // minSwitchInterval
	RemainingSeconds float64 `json:"remainingSeconds"` // until it may be switched again, 0 when it may
}

// Display holds how the web UI shows a relay, from its configuration.
type Display struct {
	Label   string `json:"label"`             // the relay name unless configured
	Color   string `json:"color"`             // pilot light colour when on: green, red or amber
	OnText  string `json:"onText,omitempty"`  // word for the on state, ON when empty
	OffText string `json:"offText,omitempty"` // word for the off state, OFF when empty
}

// LastChangeInfo is the last switch of a relay.
type LastChangeInfo struct {
	Time       string  `json:"time"`             // RFC 3339, in the Pi's time zone
	AgeSeconds float64 `json:"ageSeconds"`       // seconds since Time, by the Pi's clock
	Source     string  `json:"source"`           // api, or start when relayhat switched it on start
	Client     string  `json:"client,omitempty"` // IP address of the API client
	User       string  `json:"user,omitempty"`   // name of the bearer token the client sent
}

// HandleRelayGetOne returns the state of a single relay by name.
//
//	@Summary		Get relay state
//	@Description	Returns name, state (on/off) and description for the given relay.
//	@Tags			relay
//	@Produce		json
//	@Security		BearerAuth
//	@Param			name	path		string			true	"Relay name (e.g. relay1)"
//	@Success		200		{object}	HTTPResponse	"Relay state"
//	@Failure		401		{object}	web.ApiError	"Unauthorized"
//	@Failure		404		{object}	web.ApiError	"Relay not found"
//	@Failure		500		{object}	web.ApiError	"Internal server error"
//	@Router			/relays/{name} [get]
func (app *App) HandleRelayGetOne() http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			name := r.PathValue("name")

			res, stat, err := app.relayGet(name)
			if err != nil {
				web.WriteError(w, r, stat, err)
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
//	@Security		BearerAuth
//	@Success		200	{array}		HTTPResponse	"List of all relays"
//	@Failure		401	{object}	web.ApiError	"Unauthorized"
//	@Failure		500	{object}	web.ApiError	"Internal server error"
//	@Router			/relays [get]
func (app *App) HandleRelayGetAll() http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			// sort relay names for consistent output
			app.mu.RLock()
			names := make([]string, 0, len(app.relays))
			for n := range app.relays {
				names = append(names, n)
			}
			app.mu.RUnlock()
			sort.Strings(names)

			res := make([]HTTPResponse, 0, len(names))
			for _, n := range names {
				resp, stat, err := app.relayGet(n)
				if err != nil {
					web.WriteError(w, r, stat, err)
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
//	@Tags			relay
//	@Produce		json
//	@Security		BearerAuth
//	@Param			name	path		string			true	"Relay name"
//	@Param			state	path		string			true	"Relay state (on/off)"
//	@Success		200		{object}	HTTPResponse	"Relay state successfully set"
//	@Failure		400		{object}	web.ApiError	"Invalid state"
//	@Failure		401		{object}	web.ApiError	"Unauthorized"
//	@Failure		404		{object}	web.ApiError	"Relay not found"
//	@Failure		429		{object}	web.ApiError	"Switching locked by minSwitchInterval, see the Retry-After header"
//	@Failure		500		{object}	web.ApiError	"Internal server error"
//	@Failure		503		{object}	web.ApiError	"Shutting down or reloading, try again"
//	@Router			/relays/{name}/{state} [patch]
func (app *App) HandleRelaySet() http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {

			name := r.PathValue("name")
			state := r.PathValue("state")

			user, _ := web.UserFrom(r.Context())
			res, stat, err := app.relaySet(name, state, r.RemoteAddr, user)
			if err != nil {
				var locked errSwitchLocked
				if errors.As(err, &locked) {
					w.Header().Set("Retry-After", strconv.Itoa(int(locked.remaining/time.Second)))
				}
				web.WriteError(w, r, stat, err)
				return
			}

			web.Encode(w, http.StatusOK, res)
		})
}

// relayGet returns the API representation of a relay, with the HTTP status for an error.
func (app *App) relayGet(name string) (HTTPResponse, int, error) {
	app.mu.RLock()
	r, ok := app.relays[name]
	app.mu.RUnlock()
	if !ok {
		return HTTPResponse{}, http.StatusNotFound, errRelayNotFound
	}

	s, err := r.GetState()
	if err != nil {
		return HTTPResponse{}, http.StatusInternalServerError, err
	}

	label := r.Config.Label
	if label == "" {
		label = name
	}
	res := HTTPResponse{
		Name:        name,
		State:       s.String(),
		Description: r.Config.Description,
		GPIO:        r.GPIO(),
		Display: Display{
			Label:   label,
			Color:   r.Config.color(),
			OnText:  r.Config.OnText,
			OffText: r.Config.OffText,
		},
	}
	if interval := r.Config.MinSwitchInterval; interval > 0 {
		res.Lock = &LockInfo{
			IntervalSeconds:  interval.Seconds(),
			RemainingSeconds: r.LockRemaining(app.now()).Seconds(),
		}
	}
	if c := r.LastChange(); !c.Time.IsZero() {
		res.LastChange = &LastChangeInfo{
			Time:       c.Time.Format(time.RFC3339),
			AgeSeconds: app.now().Sub(c.Time).Seconds(),
			Source:     c.Source,
			Client:     c.Client,
			User:       c.User,
		}
	}
	return res, http.StatusOK, nil
}

// relaySet switches a relay, logs who switched it, records the switch and saves the states.
// client is the request's remote address, user the name of its bearer token. A relay already in state is not switched again: the request
// succeeds, but neither the last switch nor the switch lock change. Within minSwitchInterval
// of the last switch the request fails with 429.
func (app *App) relaySet(name, state, client, user string) (HTTPResponse, int, error) {
	var want relay.State
	switch state {
	case relay.On.String():
		want = relay.On
	case relay.Off.String():
		want = relay.Off
	default:
		return HTTPResponse{}, http.StatusBadRequest, errInvalidState
	}

	ip := clientIP(client)
	at := app.now()

	// Look up and switch under app.mu, which a shutdown takes before it hands the relays over
	// or closes them, so a request still running then cannot switch a relay it no longer owns.
	app.mu.RLock()
	if app.ctx.Err() != nil {
		app.mu.RUnlock()
		return HTTPResponse{}, http.StatusServiceUnavailable, errShuttingDown
	}
	r, ok := app.relays[name]
	if !ok {
		app.mu.RUnlock()
		return HTTPResponse{}, http.StatusNotFound, errRelayNotFound
	}
	from, switched, err := r.switchTo(want, Change{Time: at, Source: SourceAPI, Client: ip, User: user})
	app.mu.RUnlock()
	var locked errSwitchLocked
	switch {
	case errors.As(err, &locked):
		slog.Info("Relay switch refused, locked", "name", name, "to", state, "user", user, "client", ip, "remaining", locked.remaining)
		return HTTPResponse{}, http.StatusTooManyRequests, err
	case err != nil:
		return HTTPResponse{}, http.StatusInternalServerError, err
	case !switched:
		slog.Debug("Relay already in the requested state", "name", name, "state", state, "user", user, "client", ip)
		return app.relayGet(name)
	}

	slog.Info("Relay switched", "name", name, "gpio", r.GPIO(), "from", from, "to", state, "user", user, "client", ip)
	app.saveStates()
	return app.relayGet(name)
}

// clientIP returns the IP address of a request's remote address "ip:port".
func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
