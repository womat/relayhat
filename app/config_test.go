package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeConfig writes content to a temporary config file and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestLoadConfigExample(t *testing.T) {
	cfg, err := LoadConfig("../config/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("the shipped example config does not validate: %v", err)
	}
	if len(cfg.Relays) != 4 {
		t.Errorf("the example config defines %d relays, want the 4 of the 4-channel HAT", len(cfg.Relays))
	}
}

func TestLoadConfigRejectsUnknownKeys(t *testing.T) {
	for name, content := range map[string]string{
		"misspelled relay key": "relay:\n  r:\n    gpio: 4\n    descripton: x\n",
		"removed jwtSecret":    "webserver:\n  jwtSecret: x\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadConfig(writeConfig(t, content)); err == nil {
				t.Error("expected an error for the unknown key")
			}
		})
	}
}

func TestLoadConfigEmptyFileGivesDefaults(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Webserver.ListenPort != NewConfig().Webserver.ListenPort {
		t.Errorf("listenPort = %d, want the default", cfg.Webserver.ListenPort)
	}
}

func TestLoadConfigExpandsBracedVariablesOnly(t *testing.T) {
	t.Setenv("RELAYHAT_TEST_KEY", "from-env")
	cfg, err := LoadConfig(writeConfig(t, "webserver:\n  apiKey: ${RELAYHAT_TEST_KEY}\n  keyFile: /etc/pa$word.pem\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Webserver.ApiKey != "from-env" {
		t.Errorf("apiKey = %q, want from-env", cfg.Webserver.ApiKey)
	}
	if cfg.Webserver.KeyFile != "/etc/pa$word.pem" {
		t.Errorf("a bare $ was changed: %q", cfg.Webserver.KeyFile)
	}
}

func TestExpandEnvBraces(t *testing.T) {
	t.Setenv("RELAYHAT_TEST_KEY", "secret")
	for in, want := range map[string]string{
		"${RELAYHAT_TEST_KEY}":   "secret",
		"a${RELAYHAT_TEST_KEY}b": "asecretb",
		"$RELAYHAT_TEST_KEY":     "$RELAYHAT_TEST_KEY",
		"ab$cd":                  "ab$cd",
		"${RELAYHAT_UNSET_X1}":   "",
		"$$x${":                  "$$x${",
	} {
		if got := expandEnvBraces(in); got != want {
			t.Errorf("expandEnvBraces(%q) = %q, want %q", in, got, want)
		}
	}
}

// validConfig returns a config that passes Validate.
func validConfig() *Config {
	c := NewConfig()
	c.Webserver.ApiKey = "0123456789abcdefXYZ"
	c.Relays = map[string]RelayConfig{
		"a": {GPIO: 2},
		"b": {GPIO: 27},
	}
	return c
}

func TestValidate(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("valid config: %v", err)
	}

	withStates := validConfig()
	withStates.Relays["c"] = RelayConfig{GPIO: 3, StartState: StartOn}
	withStates.Relays["d"] = RelayConfig{GPIO: 4, StartState: StartLast}
	if err := withStates.Validate(); err != nil {
		t.Fatalf("valid start states: %v", err)
	}

	withDisplay := validConfig()
	withDisplay.Relays["c"] = RelayConfig{GPIO: 3, Label: "EVU", Color: ColorRed, OnText: "Gesperrt", OffText: "Freigegeben"}
	withDisplay.Relays["d"] = RelayConfig{GPIO: 4, Color: ColorAmber}
	if err := withDisplay.Validate(); err != nil {
		t.Fatalf("valid display settings: %v", err)
	}

	invalid := map[string]func(*Config){
		"missing apiKey":             func(c *Config) { c.Webserver.ApiKey = "" },
		"unknown env":                func(c *Config) { c.Env = "staging" },
		"unknown log level":          func(c *Config) { c.LogLevel = "trace" },
		"port out of range":          func(c *Config) { c.Webserver.ListenPort = 70000 },
		"empty relay name":           func(c *Config) { c.Relays[""] = RelayConfig{GPIO: 5} },
		"relay name with slash":      func(c *Config) { c.Relays["a/b"] = RelayConfig{GPIO: 5} },
		"relay name dot dot":         func(c *Config) { c.Relays[".."] = RelayConfig{GPIO: 5} },
		"prod without certFile": func(c *Config) {
			c.Env = ProdEnv
			c.Webserver.CertFile = "/nonexistent/cert.pem"
			c.Webserver.KeyFile = "/nonexistent/key.pem"
		},
		"gpio below range":           func(c *Config) { c.Relays["a"] = RelayConfig{GPIO: 1} },
		"gpio above range":           func(c *Config) { c.Relays["a"] = RelayConfig{GPIO: 28} },
		"missing gpio":               func(c *Config) { c.Relays["a"] = RelayConfig{} },
		"duplicate gpio":             func(c *Config) { c.Relays["a"] = RelayConfig{GPIO: 17}; c.Relays["b"] = RelayConfig{GPIO: 17} },
		"unknown startState":         func(c *Config) { c.Relays["a"] = RelayConfig{GPIO: 2, StartState: "toggle"} },
		"unknown color":              func(c *Config) { c.Relays["a"] = RelayConfig{GPIO: 2, Color: "blue"} },
		"negative minSwitchInterval": func(c *Config) { c.Relays["a"] = RelayConfig{GPIO: 2, MinSwitchInterval: -time.Second} },
		"minSwitchInterval too long": func(c *Config) { c.Relays["a"] = RelayConfig{GPIO: 2, MinSwitchInterval: 25 * time.Hour} },
		"last without stateFile": func(c *Config) {
			c.StateFile = ""
			c.Relays["a"] = RelayConfig{GPIO: 2, StartState: StartLast}
		},
	}
	for name, mutate := range invalid {
		t.Run(name, func(t *testing.T) {
			c := validConfig()
			mutate(c)
			if err := c.Validate(); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestWarnings(t *testing.T) {
	c := validConfig()
	if w := c.Warnings(); len(w) != 0 {
		t.Errorf("strong key: unexpected warnings %v", w)
	}

	for _, key := range []string{"Xq7z", "changeme!", "my-ChangeMe-key-that-is-long"} {
		c.Webserver.ApiKey = key
		w := c.Warnings()
		if len(w) != 1 {
			t.Errorf("key %q: warnings = %v, want one", key, w)
			continue
		}
		if strings.Contains(w[0], key) {
			t.Errorf("warning leaks the key: %q", w[0])
		}
	}
}

func TestLoadConfigMinSwitchInterval(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, "relay:\n  pump:\n    gpio: 4\n    minSwitchInterval: 5s\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Relays["pump"].MinSwitchInterval; got != 5*time.Second {
		t.Errorf("minSwitchInterval = %s, want 5s", got)
	}
}
