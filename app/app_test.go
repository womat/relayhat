package app

import (
	"errors"
	"path/filepath"
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

func TestStartStates(t *testing.T) {
	cfg := testConfig(t, map[string]RelayConfig{
		"default":  {GPIO: 4},
		"off":      {GPIO: 17, StartState: StartOff},
		"on":       {GPIO: 22, StartState: StartOn},
		"lastOn":   {GPIO: 27, StartState: StartLast},
		"lastOff":  {GPIO: 5, StartState: StartLast},
		"lastNone": {GPIO: 6, StartState: StartLast},
	})
	if err := saveRelayStates(cfg.StateFile, map[string]relay.State{"lastOn": relay.On, "lastOff": relay.Off}); err != nil {
		t.Fatal(err)
	}

	app, _ := startTestApp(t, cfg, nil)

	for name, want := range map[string]string{
		"default": "off", "off": "off", "on": "on", "lastOn": "on", "lastOff": "off", "lastNone": "off",
	} {
		if got := decode[HTTPResponse](t, serve(app, "GET", "/relays/"+name, testKey)); got.State != want {
			t.Errorf("relay %s starts %q, want %q", name, got.State, want)
		}
	}
}

func TestFirstStartCreatesStateFile(t *testing.T) {
	cfg := testConfig(t, map[string]RelayConfig{
		"last": {GPIO: 4, StartState: StartLast},
		"on":   {GPIO: 17, StartState: StartOn},
	})

	app, _ := startTestApp(t, cfg, nil)

	if got := decode[HTTPResponse](t, serve(app, "GET", "/relays/last", testKey)); got.State != "off" {
		t.Errorf("without a state file relay starts %q, want off", got.State)
	}
	got := readRelayStates(cfg.StateFile)
	if got["last"] != relay.Off || got["on"] != relay.On {
		t.Errorf("state file after the first start = %v, want last: off, on: on", got)
	}
}

func TestUnwritableStateFileDoesNotStopInit(t *testing.T) {
	cfg := testConfig(t, map[string]RelayConfig{"last": {GPIO: 4, StartState: StartLast}})
	cfg.StateFile = filepath.Join(t.TempDir(), "missing", "state.yaml")

	app, _ := startTestApp(t, cfg, nil) // fails the test if Init returns an error

	if rec := serve(app, "PATCH", "/relays/last/on", testKey); rec.Code != 200 {
		t.Errorf("PATCH with an unwritable state file = %d, want 200", rec.Code)
	}
}

func TestSwitchSavesStateAndStopKeepsIt(t *testing.T) {
	cfg := testConfig(t, map[string]RelayConfig{"pump": {GPIO: 4, StartState: StartLast}})
	app, _ := startTestApp(t, cfg, nil)

	serve(app, "PATCH", "/relays/pump/on", testKey)
	if got := readRelayStates(cfg.StateFile); got["pump"] != relay.On {
		t.Fatalf("state file after PATCH on = %v, want pump: on", got)
	}

	go app.shutdownProcedure(ModeStop)
	<-app.Shutdown()
	if got := readRelayStates(cfg.StateFile); got["pump"] != relay.On {
		t.Errorf("stop overwrote the state file: %v, want pump: on", got)
	}

	// The next start restores it.
	next, _ := startTestApp(t, cfg, nil)
	if got := decode[HTTPResponse](t, serve(next, "GET", "/relays/pump", testKey)); got.State != "on" {
		t.Errorf("after restart pump is %q, want on", got.State)
	}
}

func TestHandoverIgnoresStartState(t *testing.T) {
	first, _ := newTestApp(t, map[string]RelayConfig{"r": {GPIO: 4}}, nil)
	serve(first, "PATCH", "/relays/r/on", testKey)
	handover := restart(t, first)

	second, _ := newTestApp(t, map[string]RelayConfig{"r": {GPIO: 4, StartState: StartOff}}, handover)
	if got := decode[HTTPResponse](t, serve(second, "GET", "/relays/r", testKey)); got.State != "on" {
		t.Errorf("taken over relay is %q, want it to stay on despite startState off", got.State)
	}
}
