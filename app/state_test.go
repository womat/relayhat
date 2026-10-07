package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/womat/relayhat/pkg/relay"
)

func TestRelayStatesRoundTrip(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.yaml")
	want := map[string]relay.State{"pump": relay.On, "light": relay.Off}

	if err := saveRelayStates(file, want); err != nil {
		t.Fatal(err)
	}
	got := readRelayStates(file)
	if len(got) != len(want) || got["pump"] != relay.On || got["light"] != relay.Off {
		t.Errorf("readRelayStates = %v, want %v", got, want)
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
	if err := os.WriteFile(file, []byte("pump: on\nlight: dimmed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := readRelayStates(file)
	if len(got) != 1 || got["pump"] != relay.On {
		t.Errorf("readRelayStates = %v, want only pump: on", got)
	}
}

func TestSaveRelayStatesMissingDirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "missing", "state.yaml")
	if err := saveRelayStates(file, map[string]relay.State{"pump": relay.On}); err == nil {
		t.Error("expected an error for a missing directory")
	}
}

func ptr(s string) *string { return &s }
