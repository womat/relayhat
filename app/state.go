package app

// The state file keeps the last state of every relay and how it got there, so relays with
// startState: last can restore their state after a restart of the process and the web UI can
// show the last switch across one. It is a YAML map by relay name, written atomically after
// every switch through the API:
//
//	relay1:
//	  state: on
//	  changed: 2026-10-07T14:02:13+02:00
//	  source: api
//	  client: 192.168.65.20
//	  host: nodered.fritz.box
//
// The format of relayhat 1.7, "relay1: on", is still read.
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
	"time"

	"github.com/womat/relayhat/pkg/relay"
	"gopkg.in/yaml.v3"
)

// Sources of a relay switch, see Change.
const (
	SourceAPI   = "api"   // switched through the API
	SourceStart = "start" // switched to its start state by relayhat itself
)

// Change describes the last switch of a relay.
type Change struct {
	Time   time.Time
	Source string // SourceAPI or SourceStart
	Client string // IP address of the API client, empty for SourceStart
	Host   string // reverse DNS name of Client, empty when unknown
}

// savedRelay is one relay in the state file.
type savedRelay struct {
	State   string    `yaml:"state"`
	Changed time.Time `yaml:"changed,omitempty"`
	Source  string    `yaml:"source,omitempty"`
	Client  string    `yaml:"client,omitempty"`
	Host    string    `yaml:"host,omitempty"`
}

// UnmarshalYAML reads both the current map form and the plain "on"/"off" of relayhat 1.7.
func (s *savedRelay) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		return node.Decode(&s.State)
	}
	type plain savedRelay // without this method, so Decode does not recurse
	return node.Decode((*plain)(s))
}

// savedState is the state of a relay as read from the state file.
type savedState struct {
	State  relay.State
	Change Change // zero Time when the file does not say
}

// readRelayStates reads the state file. It returns an empty map, never an error, when the
// file is missing, empty or damaged; relays with an invalid state are skipped.
func readRelayStates(file string) map[string]savedState {
	states := map[string]savedState{}

	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		slog.Info("State file not found, starting without saved relay states", "file", file)
		return states
	}
	if err != nil {
		slog.Warn("State file not readable, starting without saved relay states", "file", file, "error", err)
		return states
	}
	if len(bytes.TrimSpace(data)) == 0 {
		slog.Warn("State file is empty, starting without saved relay states", "file", file)
		return states
	}

	var saved map[string]savedRelay
	if err = yaml.Unmarshal(data, &saved); err != nil {
		slog.Warn("State file is damaged, starting without saved relay states", "file", file, "error", err)
		return states
	}

	for name, v := range saved {
		var st relay.State
		switch v.State {
		case relay.On.String():
			st = relay.On
		case relay.Off.String():
			st = relay.Off
		default:
			slog.Warn("State file has an invalid state, relay ignored", "file", file, "relay", name, "state", v.State)
			continue
		}
		states[name] = savedState{
			State:  st,
			Change: Change{Time: v.Changed, Source: v.Source, Client: v.Client, Host: v.Host},
		}
	}
	return states
}

// saveRelayStates writes the states to file, atomically.
func saveRelayStates(file string, states map[string]savedState) error {
	saved := make(map[string]savedRelay, len(states))
	for name, s := range states {
		saved[name] = savedRelay{
			State:   s.State.String(),
			Changed: s.Change.Time,
			Source:  s.Change.Source,
			Client:  s.Change.Client,
			Host:    s.Change.Host,
		}
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
