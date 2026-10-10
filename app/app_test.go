package app

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/womat/golib/gpio"
	"github.com/womat/golib/web"
	"github.com/womat/relayhat/pkg/relay"
)

// restart runs a restart of app and returns the relays it hands over.
func restart(t *testing.T, app *App) map[int]*Relay {
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

// A failed Init hands every relay back instead of switching it off, so the caller can start the
// previous configuration with them; only when it exits are they closed.
func TestInitFailureHandsEverythingBack(t *testing.T) {
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
	if l, _ := pins[4].Value(); l != gpio.High {
		t.Error("the taken over relay was switched off after Init failed")
	}
	back := second.Handover()
	if _, ok := back[4]; !ok || len(back) != 1 {
		t.Fatalf("handover after the failed Init = %v, want gpio 4", back)
	}

	// The previous configuration takes the relay over again, still switched on.
	third, _ := newTestApp(t, map[string]RelayConfig{"old": {GPIO: 4}}, back)
	if got := decode[HTTPResponse](t, serve(third, "GET", "/relays/old", testKey)); got.State != "on" {
		t.Errorf("relay after falling back = %q, want on", got.State)
	}
	// The mutex must be free again. TryLock rather than Lock: a mutex still held fails the test
	// instead of hanging it.
	if !second.mu.TryLock() {
		t.Fatal("app.mu is still held after the failed Init")
	}
	second.mu.Unlock()
}

// After a shutdown has begun, a request still running must neither switch a relay nor start a
// host name lookup the shutdown no longer waits for.
func TestNoSwitchAfterShutdownStarted(t *testing.T) {
	app, pins := newTestApp(t, map[string]RelayConfig{"r1": {GPIO: 4}}, nil)
	app.stop()

	if rec := serve(app, "PATCH", "/relays/r1/on", testKey); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("PATCH after stop = %d, want 503", rec.Code)
	}
	if l, _ := pins[4].Value(); l != gpio.Low {
		t.Error("a request after stop switched the relay")
	}
	app.resolveClient(app.relays["r1"], time.Now(), "127.0.0.1") // must not add to app.wg
	app.wg.Wait()
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
	if err := saveRelayStates(cfg.StateFile, map[string]savedState{"lastOn": {State: relay.On}, "lastOff": {State: relay.Off}}); err != nil {
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
	if got["last"].State != relay.Off || got["on"].State != relay.On {
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
	if got := readRelayStates(cfg.StateFile); got["pump"].State != relay.On {
		t.Fatalf("state file after PATCH on = %v, want pump: on", got)
	}

	go app.shutdownProcedure(ModeStop)
	<-app.Shutdown()
	if got := readRelayStates(cfg.StateFile); got["pump"].State != relay.On {
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

func TestSwitchRecordsClientAndHost(t *testing.T) {
	cfg := testConfig(t, map[string]RelayConfig{"pump": {GPIO: 4}})
	app, _ := startTestApp(t, cfg, nil)
	app.lookupAddr = func(_ context.Context, addr string) ([]string, error) {
		if addr != "127.0.0.1" {
			t.Errorf("looked up %q, want the bare client IP", addr)
		}
		return []string{"nodered.fritz.box."}, nil
	}

	start := decode[HTTPResponse](t, serve(app, "GET", "/relays/pump", testKey)).LastChange
	if start == nil || start.Source != SourceStart || start.Client != "" {
		t.Fatalf("lastChange after the start = %+v, want source start", start)
	}

	got := decode[HTTPResponse](t, serve(app, "PATCH", "/relays/pump/on", testKey)).LastChange
	if got == nil || got.Source != SourceAPI || got.Client != "127.0.0.1" {
		t.Fatalf("lastChange after PATCH = %+v, want source api from 127.0.0.1", got)
	}

	app.wg.Wait() // the host name lookup
	got = decode[HTTPResponse](t, serve(app, "GET", "/relays/pump", testKey)).LastChange
	if got.Host != "nodered.fritz.box" {
		t.Errorf("lastChange host = %q, want nodered.fritz.box", got.Host)
	}
	if saved := readRelayStates(cfg.StateFile)["pump"].Change; saved.Host != "nodered.fritz.box" || saved.Client != "127.0.0.1" {
		t.Errorf("state file holds %+v, want client and host of the switch", saved)
	}
}

func TestLateHostNameDoesNotOverwriteNewerSwitch(t *testing.T) {
	r := &Relay{}
	first := time.Now()
	r.setChange(Change{Time: first, Source: SourceAPI, Client: "10.0.0.1"})
	r.setChange(Change{Time: first.Add(time.Second), Source: SourceAPI, Client: "10.0.0.2"})

	if r.setHost(first, "old-client") {
		t.Error("setHost changed a newer switch")
	}
	if got := r.LastChange(); got.Host != "" || got.Client != "10.0.0.2" {
		t.Errorf("LastChange = %+v, want the newer switch untouched", got)
	}
}

func TestLastChangeSurvivesRestartAndReload(t *testing.T) {
	changed := time.Date(2026, 10, 7, 14, 2, 13, 0, time.Local)
	saved := Change{Time: changed, Source: SourceAPI, Client: "192.168.65.20", Host: "nodered.fritz.box"}

	cfg := testConfig(t, map[string]RelayConfig{
		"keep":    {GPIO: 4, StartState: StartLast},
		"differs": {GPIO: 17, StartState: StartOff},
	})
	if err := saveRelayStates(cfg.StateFile, map[string]savedState{
		"keep":    {State: relay.On, Change: saved},
		"differs": {State: relay.On, Change: saved},
	}); err != nil {
		t.Fatal(err)
	}

	// Cold start: "keep" starts on, as the file says, so the saved switch is still its last one;
	// "differs" starts off although it was on, so the start is its last switch.
	first, _ := startTestApp(t, cfg, nil)
	if got := first.relays["keep"].LastChange(); !got.Time.Equal(changed) || got.Host != saved.Host {
		t.Errorf("keep: LastChange = %+v, want the saved switch", got)
	}
	if got := first.relays["differs"].LastChange(); got.Source != SourceStart {
		t.Errorf("differs: LastChange = %+v, want source start", got)
	}

	// Reload: a taken over relay keeps its last switch, also under a new name.
	handover := restart(t, first)
	second, _ := newTestApp(t, map[string]RelayConfig{"renamed": {GPIO: 4}}, handover)
	if got := second.relays["renamed"].LastChange(); !got.Time.Equal(changed) || got.Client != saved.Client {
		t.Errorf("after reload LastChange = %+v, want the saved switch", got)
	}
}

func TestStateFileWrittenWithoutLastStartState(t *testing.T) {
	cfg := testConfig(t, map[string]RelayConfig{"pump": {GPIO: 4}})
	app, _ := startTestApp(t, cfg, nil)

	serve(app, "PATCH", "/relays/pump/on", testKey)
	if got := readRelayStates(cfg.StateFile)["pump"]; got.State != relay.On || got.Change.Source != SourceAPI {
		t.Errorf("state file = %+v, want pump on, switched through the api", got)
	}
}

// clock is a settable time source for App.now.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func TestSwitchLock(t *testing.T) {
	app, pins := newTestApp(t, map[string]RelayConfig{"pump": {GPIO: 4, MinSwitchInterval: 10 * time.Second}}, nil)
	c := &clock{t: time.Now().Add(time.Hour)} // well past the start, which counts as a switch
	app.now = c.now

	if rec := serve(app, http.MethodPatch, "/relays/pump/on", testKey); rec.Code != http.StatusOK {
		t.Fatalf("first switch = %d, want 200", rec.Code)
	}
	switched := decode[HTTPResponse](t, serve(app, http.MethodGet, "/relays/pump", testKey)).LastChange.Time

	c.t = c.t.Add(3*time.Second + 200*time.Millisecond)
	rec := serve(app, http.MethodPatch, "/relays/pump/off", testKey)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("switch within the lock = %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "7" {
		t.Errorf("Retry-After = %q, want 7", got)
	}
	if msg := decode[web.ApiError](t, rec).Error; !strings.Contains(msg, "switching locked for 7s") {
		t.Errorf("error = %q, want the remaining lock", msg)
	}
	if l, _ := pins[4].Value(); l != gpio.High {
		t.Error("a refused switch changed the pin")
	}

	// The current state again is no switch: allowed while locked, and it keeps the last switch.
	rec = serve(app, http.MethodPatch, "/relays/pump/on", testKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH to the current state while locked = %d, want 200", rec.Code)
	}
	got := decode[HTTPResponse](t, rec)
	if got.LastChange.Time != switched {
		t.Errorf("PATCH to the current state moved the last switch from %s to %s", switched, got.LastChange.Time)
	}
	if got.Lock == nil || got.Lock.IntervalSeconds != 10 || got.Lock.RemainingSeconds != 7 {
		t.Errorf("lock = %+v, want interval 10, remaining 7", got.Lock)
	}

	c.t = c.t.Add(7 * time.Second)
	if rec := serve(app, http.MethodPatch, "/relays/pump/off", testKey); rec.Code != http.StatusOK {
		t.Errorf("switch after the lock = %d, want 200", rec.Code)
	}
}

func TestNoSwitchLockWithoutInterval(t *testing.T) {
	app, _ := newTestApp(t, map[string]RelayConfig{"pump": {GPIO: 4}}, nil)

	for _, state := range []string{"on", "off", "on"} {
		if rec := serve(app, http.MethodPatch, "/relays/pump/"+state, testKey); rec.Code != http.StatusOK {
			t.Errorf("PATCH %s = %d, want 200", state, rec.Code)
		}
	}
	if got := decode[HTTPResponse](t, serve(app, http.MethodGet, "/relays/pump", testKey)); got.Lock != nil {
		t.Errorf("lock = %+v, want none without minSwitchInterval", got.Lock)
	}
}

func TestSwitchLockSurvivesReload(t *testing.T) {
	cfgs := map[string]RelayConfig{"pump": {GPIO: 4, MinSwitchInterval: time.Minute}}
	first, _ := newTestApp(t, cfgs, nil)
	c := &clock{t: time.Now().Add(time.Hour)}
	first.now = c.now
	serve(first, http.MethodPatch, "/relays/pump/on", testKey)

	handover := restart(t, first)
	second, _ := newTestApp(t, cfgs, handover)
	second.now = c.now
	c.t = c.t.Add(10 * time.Second)
	if rec := serve(second, http.MethodPatch, "/relays/pump/off", testKey); rec.Code != http.StatusTooManyRequests {
		t.Errorf("switch 10 s after the last one, across a reload = %d, want 429", rec.Code)
	}
}
