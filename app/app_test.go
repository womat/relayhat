package app

import (
	"errors"
	"testing"

	"github.com/womat/golib/gpio"
	"github.com/womat/relayhat/pkg/relay"
)

// restart runs a restart of app and returns the relays it hands over.
func restart(t *testing.T, app *App) map[int]*relay.Relay {
	t.Helper()
	go app.shutdownProcedure(ModeRestart)
	<-app.Restart()
	return app.Handover()
}

func TestRestartKeepsRelayState(t *testing.T) {
	first, pins := newTestApp(t, map[string]RelayConfig{
		"keep":   {GPIO: 4},
		"remove": {GPIO: 17},
	}, nil)
	if rec := serve(first, "PATCH", "/relays/keep/on", testKey); rec.Code != 200 {
		t.Fatalf("PATCH on = %d", rec.Code)
	}
	if rec := serve(first, "PATCH", "/relays/remove/on", testKey); rec.Code != 200 {
		t.Fatalf("PATCH on = %d", rec.Code)
	}

	handover := restart(t, first)
	if len(handover) != 2 {
		t.Fatalf("handover has %d relays, want 2", len(handover))
	}
	if l, _ := pins[4].Value(); l != gpio.High {
		t.Fatal("restart switched gpio 4 off before the next App took it over")
	}

	// gpio 4 is renamed, gpio 17 removed and gpio 22 added.
	second, newPins := newTestApp(t, map[string]RelayConfig{
		"renamed": {GPIO: 4},
		"added":   {GPIO: 22},
	}, handover)

	if got := decode[HTTPResponse](t, serve(second, "GET", "/relays/renamed", testKey)); got.State != "on" {
		t.Errorf("taken over relay state = %q, want on", got.State)
	}
	if _, reopened := newPins[4]; reopened {
		t.Error("gpio 4 was opened again instead of being taken over")
	}
	if l, _ := pins[17].Value(); l != gpio.Low {
		t.Error("gpio 17 is no longer configured but was not closed")
	}
	if got := decode[HTTPResponse](t, serve(second, "GET", "/relays/added", testKey)); got.State != "off" {
		t.Errorf("new relay state = %q, want off", got.State)
	}
}

func TestStopClosesRelays(t *testing.T) {
	app, pins := newTestApp(t, map[string]RelayConfig{"r1": {GPIO: 4}}, nil)
	serve(app, "PATCH", "/relays/r1/on", testKey)

	go app.shutdownProcedure(ModeStop)
	<-app.Shutdown()

	if len(app.Handover()) != 0 {
		t.Error("a stop must not hand over relays")
	}
	if l, _ := pins[4].Value(); l != gpio.Low {
		t.Error("stop left gpio 4 switched on")
	}
}

func TestInitFailureClosesEverything(t *testing.T) {
	first, pins := newTestApp(t, map[string]RelayConfig{"old": {GPIO: 4}}, nil)
	serve(first, "PATCH", "/relays/old/on", testKey)
	handover := restart(t, first)

	cfg := NewConfig()
	cfg.Webserver.ApiKey = testKey
	cfg.Relays = map[string]RelayConfig{"a": {GPIO: 4}, "b": {GPIO: 5}}
	second := New(cfg, nil, nil, handover)
	second.openRelay = func(int) (*relay.Relay, error) { return nil, errors.New("gpio busy") }

	if err := second.Init(); err == nil {
		t.Fatal("expected Init to fail")
	}
	if l, _ := pins[4].Value(); l != gpio.Low {
		t.Error("the taken over relay stayed on after Init failed")
	}
	// The mutex must be free again.
	second.mu.Lock()
	second.mu.Unlock()
}
