package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/womat/relayhat/pkg/relay"
)

func TestRelayStatesRoundTrip(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.yaml")
	changed := time.Date(2026, 10, 7, 14, 2, 13, 0, time.FixedZone("CEST", 2*3600))
	want := map[string]savedState{
		"pump":  {State: relay.On, Change: Change{Time: changed, Source: SourceAPI, Client: "192.168.65.20", Host: "nodered.fritz.box"}},
		"light": {State: relay.Off},
	}

	if err := saveRelayStates(file, want); err != nil {
		t.Fatal(err)
	}
	got := readRelayStates(file)
	if len(got) != len(want) || got["light"] != want["light"] {
		t.Errorf("readRelayStates = %+v, want %+v", got, want)
	}
	if p := got["pump"]; p.State != relay.On || !p.Change.Time.Equal(changed) || p.Change.Source != SourceAPI ||
		p.Change.Client != "192.168.65.20" || p.Change.Host != "nodered.fritz.box" {
		t.Errorf("pump = %+v, want %+v", p, want["pump"])
	}

	// No temporary file is left behind.
	entries, err := os.ReadDir(filepath.Dir(file))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("directory holds %d entries, want only the state file", len(entries))
	}
}

func TestReadRelayStatesNeverFails(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]*string{
		"missing": nil,
		"empty":   ptr("  \n"),
		"damaged": ptr("pump: [on\n"),
		"no map":  ptr("- on\n"),
	} {
		t.Run(name, func(t *testing.T) {
			file := filepath.Join(dir, name+".yaml")
			if content != nil {
				if err := os.WriteFile(file, []byte(*content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if got := readRelayStates(file); len(got) != 0 {
				t.Errorf("readRelayStates = %v, want an empty map", got)
			}
		})
	}
}

func TestReadRelayStatesSkipsInvalidValues(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.yaml")
	if err := os.WriteFile(file, []byte("pump: on\nlight: dimmed\nfan:\n  state: toggled\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := readRelayStates(file)
	if len(got) != 1 || got["pump"].State != relay.On {
		t.Errorf("readRelayStates = %v, want only pump: on", got)
	}
}

func TestReadRelayStatesOldFormat(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.yaml")
	if err := os.WriteFile(file, []byte("relay1: on\nrelay2: off\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := readRelayStates(file)
	if got["relay1"].State != relay.On || got["relay2"].State != relay.Off || !got["relay1"].Change.Time.IsZero() {
		t.Errorf("readRelayStates of the 1.7 format = %+v, want relay1 on, relay2 off, no change time", got)
	}
}

func TestSaveRelayStatesMissingDirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "missing", "state.yaml")
	if err := saveRelayStates(file, map[string]savedState{"pump": {State: relay.On}}); err == nil {
		t.Error("expected an error for a missing directory")
	}
}

func ptr(s string) *string { return &s }
