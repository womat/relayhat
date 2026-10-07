package app

// The state file keeps the last state of every relay, so relays with startState: last can
// restore it after a restart of the process. It is a YAML map "relay name: on|off", written
// atomically after every switch through the API.
//
// A missing, empty or damaged file is never fatal: relayhat has to come up, and "off" is the
// safe fallback. The relays that wanted their last state then start off, and the log says why.

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/womat/relayhat/pkg/relay"
	"gopkg.in/yaml.v3"
)

// readRelayStates reads the state file. It returns an empty map, never an error, when the
// file is missing, empty or damaged; unknown values are skipped.
func readRelayStates(file string) map[string]relay.State {
	states := map[string]relay.State{}

	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		slog.Info("State file not found, relays with startState last start off", "file", file)
		return states
	}
	if err != nil {
		slog.Warn("State file not readable, relays with startState last start off", "file", file, "error", err)
		return states
	}
	if len(bytes.TrimSpace(data)) == 0 {
		slog.Warn("State file is empty, relays with startState last start off", "file", file)
		return states
	}

	var saved map[string]string
	if err = yaml.Unmarshal(data, &saved); err != nil {
		slog.Warn("State file is damaged, relays with startState last start off", "file", file, "error", err)
		return states
	}

	for name, value := range saved {
		switch value {
		case relay.On.String():
			states[name] = relay.On
		case relay.Off.String():
			states[name] = relay.Off
		default:
			slog.Warn("State file has an invalid value, relay starts off", "file", file, "relay", name, "value", value)
		}
	}
	return states
}

// saveRelayStates writes the states to file, atomically.
func saveRelayStates(file string, states map[string]relay.State) error {
	saved := make(map[string]string, len(states))
	for name, s := range states {
		saved[name] = s.String()
	}

	data, err := yaml.Marshal(saved)
	if err != nil {
		return fmt.Errorf("marshal relay states: %w", err)
	}
	return writeFileAtomic(file, data, 0o640)
}

// writeFileAtomic writes data to a temporary file next to file and renames it into place, so a
// power cut during the write leaves either the old or the new file, never a truncated one.
func writeFileAtomic(file string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(file)

	tmp, err := os.CreateTemp(dir, filepath.Base(file)+".tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}

	if err = os.Rename(tmp.Name(), file); err != nil {
		return err
	}

	// Persist the rename. Failing here no longer risks the data, only the durability of the
	// rename, so it is reported but the new file stays in place.
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err = d.Sync(); err != nil {
		_ = d.Close()
		return err
	}
	return d.Close()
}
