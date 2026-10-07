package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/womat/golib/gpio"
	"github.com/womat/golib/gpio/rpiemu"
	"github.com/womat/golib/web"
	"github.com/womat/relayhat/pkg/relay"
)

const testKey = "test-key"

// emuRelays opens relays on emulated pins and remembers the pins, so a test can
// inspect the line level behind a relay.
type emuRelays map[int]rpiemu.Pin

func (e emuRelays) open(n int) (*relay.Relay, error) {
	p, err := rpiemu.NewPin(n, rpiemu.WithMode(gpio.Output))
	if err != nil {
		return nil, err
	}
	e[n] = p
	return relay.NewWithPin(p)
}

// testConfig returns a config with the given relays and a state file in a temporary directory.
func testConfig(t *testing.T, relays map[string]RelayConfig) *Config {
	t.Helper()
	cfg := NewConfig()
	cfg.Webserver.ApiKey = testKey
	cfg.StateFile = filepath.Join(t.TempDir(), "state.yaml")
	cfg.Relays = relays
	return cfg
}

// newTestApp returns an initialized App with the given relays on emulated pins.
func newTestApp(t *testing.T, relays map[string]RelayConfig, inherited map[int]*relay.Relay) (*App, emuRelays) {
	t.Helper()
	return startTestApp(t, testConfig(t, relays), inherited)
}

// startTestApp returns an initialized App for cfg with its relays on emulated pins.
func startTestApp(t *testing.T, cfg *Config, inherited map[int]*relay.Relay) (*App, emuRelays) {
	t.Helper()
	pins := emuRelays{}
	app := New(cfg, nil, nil, inherited)
	app.openRelay = pins.open
	if err := app.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Cleanup() })
	return app, pins
}

func serve(app *App, method, path, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "127.0.0.1:1234"
	if key != "" {
		req.Header.Set("X-API-Key", key)
	}
	rec := httptest.NewRecorder()
	app.web.Handler.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

func TestRoutesRequireApiKey(t *testing.T) {
	app, _ := newTestApp(t, map[string]RelayConfig{"r1": {GPIO: 4}}, nil)

	if rec := serve(app, http.MethodGet, "/version", ""); rec.Code != http.StatusOK {
		t.Errorf("GET /version without key = %d, want 200", rec.Code)
	}
	for _, rt := range []struct{ method, path string }{
		{http.MethodGet, "/health"},
		{http.MethodGet, "/relays"},
		{http.MethodGet, "/relays/r1"},
		{http.MethodPatch, "/relays/r1/on"},
	} {
		if rec := serve(app, rt.method, rt.path, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without key = %d, want 401", rt.method, rt.path, rec.Code)
		}
		if rec := serve(app, rt.method, rt.path, "wrong"); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with wrong key = %d, want 401", rt.method, rt.path, rec.Code)
		}
	}
}

func TestRelaySwitching(t *testing.T) {
	app, pins := newTestApp(t, map[string]RelayConfig{
		"r1": {GPIO: 4, Description: "pump"},
		"r2": {GPIO: 17},
	}, nil)

	rec := serve(app, http.MethodPatch, "/relays/r1/on", testKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH on = %d %s", rec.Code, rec.Body)
	}
	if got := decode[HTTPResponse](t, rec); got != (HTTPResponse{Name: "r1", State: "on", Description: "pump"}) {
		t.Errorf("PATCH on returned %+v", got)
	}
	if l, _ := pins[4].Value(); l != gpio.High {
		t.Errorf("gpio 4 = %v after PATCH on, want high", l)
	}

	rec = serve(app, http.MethodGet, "/relays", testKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /relays = %d", rec.Code)
	}
	all := decode[[]HTTPResponse](t, rec)
	if len(all) != 2 || all[0].Name != "r1" || all[0].State != "on" || all[1].Name != "r2" || all[1].State != "off" {
		t.Errorf("GET /relays = %+v, want r1 on, r2 off in name order", all)
	}

	if rec := serve(app, http.MethodPatch, "/relays/r1/off", testKey); rec.Code != http.StatusOK {
		t.Fatalf("PATCH off = %d", rec.Code)
	}
	if got := decode[HTTPResponse](t, serve(app, http.MethodGet, "/relays/r1", testKey)); got.State != "off" {
		t.Errorf("GET /relays/r1 state = %q after PATCH off, want off", got.State)
	}
}

func TestRelayErrors(t *testing.T) {
	app, pins := newTestApp(t, map[string]RelayConfig{"r1": {GPIO: 4}}, nil)

	for _, tc := range []struct {
		method, path string
		code         int
		msg          string
	}{
		{http.MethodGet, "/relays/nope", http.StatusNotFound, errRelayNotFound.Error()},
		{http.MethodPatch, "/relays/nope/on", http.StatusNotFound, errRelayNotFound.Error()},
		{http.MethodPatch, "/relays/r1/toggle", http.StatusBadRequest, errInvalidState.Error()},
	} {
		rec := serve(app, tc.method, tc.path, testKey)
		if rec.Code != tc.code {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, rec.Code, tc.code)
		}
		if got := decode[web.ApiError](t, rec); got.Error != tc.msg {
			t.Errorf("%s %s error = %q, want %q", tc.method, tc.path, got.Error, tc.msg)
		}
	}

	// A closed pin is an input, so switching fails: the client gets a generic 500 only.
	_ = pins[4].Close()
	rec := serve(app, http.MethodPatch, "/relays/r1/on", testKey)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("PATCH on a failing pin = %d, want 500", rec.Code)
	}
	if got := decode[web.ApiError](t, rec); got.Error != web.ErrInternal.Error() {
		t.Errorf("500 error = %q, want %q without internals", got.Error, web.ErrInternal.Error())
	}
}

func TestCORSAdvertisesUsedMethodsOnly(t *testing.T) {
	app, _ := newTestApp(t, nil, nil)

	rec := serve(app, http.MethodOptions, "/relays/r1/on", "")
	if got, want := rec.Header().Get("Access-Control-Allow-Methods"), "GET, PATCH, OPTIONS"; got != want {
		t.Errorf("Access-Control-Allow-Methods = %q, want %q", got, want)
	}
}
