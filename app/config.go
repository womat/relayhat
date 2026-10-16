package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/womat/golib/v2/web"
	"gopkg.in/yaml.v3"
)

const (
	ProdEnv = "prod"
	DevEnv  = "dev"
)

// Start states of a relay after a (re)start of the process, see RelayConfig.StartState.
const (
	StartOff  = "off"
	StartOn   = "on"
	StartLast = "last"
)

// Colours of a relay's pilot light in the web UI, see RelayConfig.Color.
const (
	ColorGreen = "green"
	ColorRed   = "red"
	ColorAmber = "amber"
)

// maxSwitchInterval is the longest RelayConfig.MinSwitchInterval Validate accepts.
const maxSwitchInterval = 24 * time.Hour

// Valid GPIO range of the 40-pin header (BCM numbering). GPIO 0 and 1 are reserved
// for the HAT EEPROM.
const (
	minGPIO = 2
	maxGPIO = 27
)

// Config holds the application's YAML configuration.
type Config struct {
	Env            string                 `yaml:"env"`            // Application environment: dev | prod
	LogLevel       string                 `yaml:"logLevel"`       // Log level: debug | info | warn | error
	LogDestination string                 `yaml:"logDestination"` // Log output: stdout | stderr | null | /path/to/logfile
	Webserver      WebserverConfig        `yaml:"webserver"`      // Webserver configuration
	StateFile      string                 `yaml:"stateFile"`      // Last state and switch of every relay; empty disables it
	Relays         map[string]RelayConfig `yaml:"relay"`          // Relays by name
}

// WebserverConfig holds HTTPS server settings.
type WebserverConfig struct {
	ListenHost string      `yaml:"listenHost"` // Host address for web server
	ListenPort int         `yaml:"listenPort"` // Port for web server
	ApiKey     string      `yaml:"apiKey"`     // Removed in v2; only read to name the replacement
	TokenFile  string      `yaml:"tokenFile"`  // File with the accepted bearer tokens as SHA-256 hashes
	Tokens     []web.Token `yaml:"-"`          // Read from TokenFile by LoadConfig
	KeyFile    string      `yaml:"keyFile"`    // SSL private key file
	CertFile   string      `yaml:"certFile"`   // SSL certificate file
	BlockedIPs []string    `yaml:"blockedIPs"` // Forbidden IP addresses or networks
	AllowedIPs []string    `yaml:"allowedIPs"` // Allowed IP addresses or networks
}

// RelayConfig holds the configuration for a single relay from the YAML file.
type RelayConfig struct {
	GPIO        int    `yaml:"gpio"`
	Description string `yaml:"description"`

	// StartState is the state a relay is switched to when it is opened: off (default), on, or
	// last, the state stored in Config.StateFile. It does not apply to a relay that is taken
	// over on a SIGHUP reload, which keeps its state.
	StartState string `yaml:"startState"`

	// MinSwitchInterval is the time after a switch during which the relay refuses to be switched
	// again, by any client (e.g. 5s, 10m); 0 disables it. Start states are never refused.
	MinSwitchInterval time.Duration `yaml:"minSwitchInterval"`

	// Display settings for the web UI; they change nothing in the API paths or the switching.
	Label   string `yaml:"label"`   // Name shown on the card, default the relay name
	Color   string `yaml:"color"`   // Pilot light colour when on: green (default), red or amber
	OnText  string `yaml:"onText"`  // Word for the on state, default ON
	OffText string `yaml:"offText"` // Word for the off state, default OFF
}

// color returns Color with the empty default resolved to ColorGreen.
func (r RelayConfig) color() string {
	if r.Color == "" {
		return ColorGreen
	}
	return r.Color
}

// startMode returns StartState with the empty default resolved to StartOff.
func (r RelayConfig) startMode() string {
	if r.StartState == "" {
		return StartOff
	}
	return r.StartState
}

// NewConfig returns a Config initialized with default values.
func NewConfig() *Config {
	return &Config{
		Env:            DevEnv,
		LogLevel:       "info",
		LogDestination: "stdout",
		StateFile:      filepath.Join("/opt", MODULE, "data", "state.yaml"),
		Webserver: WebserverConfig{
			ListenHost: "0.0.0.0",
			ListenPort: 8443,
			BlockedIPs: []string{},
			AllowedIPs: []string{},
		},
	}
}

// envBraces matches ${VAR} references; see expandEnvBraces.
var envBraces = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// expandEnvBraces replaces ${VAR} with the value of the environment variable VAR, or with an
// empty string when it is unset. Unlike os.ExpandEnv it leaves every other "$" alone, so an API
// key containing "$" is not silently cut short.
func expandEnvBraces(s string) string {
	return envBraces.ReplaceAllStringFunc(s, func(ref string) string {
		return os.Getenv(envBraces.FindStringSubmatch(ref)[1])
	})
}

// LoadConfig loads configuration from a YAML file and expands ${VAR} environment references,
// then reads the bearer tokens from webserver.tokenFile.
//
// Unknown keys are an error rather than ignored, so a misspelled or renamed key cannot silently
// leave its setting at the default. A token file that is missing or broken is an error too: the
// service would otherwise start and reject every request with 401. Since a SIGHUP reload goes
// through here, a token added with "relayhat token add" applies on reload.
func LoadConfig(fileName string) (*Config, error) {
	cfg, err := readConfig(fileName)
	if err != nil {
		return cfg, err
	}

	if cfg.Webserver.TokenFile != "" {
		if cfg.Webserver.Tokens, err = web.ReadTokenFile(cfg.Webserver.TokenFile); err != nil {
			return cfg, fmt.Errorf("token file: %w", err)
		}
	}

	return cfg, nil
}

// TokenFile returns webserver.tokenFile of the config file, without reading the token file
// itself - "relayhat token add" has to work before that file exists.
func TokenFile(fileName string) (string, error) {
	cfg, err := readConfig(fileName)
	if err != nil {
		return "", err
	}
	return cfg.Webserver.TokenFile, nil
}

// readConfig loads the config file alone. A relative tokenFile is resolved against the
// directory of the config file, so the service and "relayhat token" find the same file
// wherever they are started from.
func readConfig(fileName string) (*Config, error) {
	cfg := NewConfig()

	fileInfo, err := os.Stat(fileName)
	if err != nil {
		return cfg, err
	}
	if fileInfo.IsDir() {
		return cfg, errors.New("config path is a directory, not a file")
	}

	content, err := os.ReadFile(fileName)
	if err != nil {
		return cfg, err
	}

	dec := yaml.NewDecoder(bytes.NewReader([]byte(expandEnvBraces(string(content)))))
	dec.KnownFields(true)
	if err = dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return cfg, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if f := cfg.Webserver.TokenFile; f != "" && !filepath.IsAbs(f) {
		cfg.Webserver.TokenFile = filepath.Join(filepath.Dir(fileName), f)
	}

	return cfg, nil
}

// Validate checks the Config for invalid or missing values.
func (c *Config) Validate() error {

	if c.Env != ProdEnv && c.Env != DevEnv {
		return fmt.Errorf("invalid environment: %s, must be %s or %s", c.Env, ProdEnv, DevEnv)
	}

	if c.Webserver.ApiKey != "" {
		return errors.New(`webserver.apiKey is no longer supported; create a token with ` +
			`"relayhat token add <name>" and set webserver.tokenFile`)
	}
	if c.Webserver.TokenFile == "" {
		return errors.New(`webserver.tokenFile is not configured; create a token with "relayhat token add <name>"`)
	}

	validLogLevels := []string{"debug", "info", "warning", "warn", "error"}
	if !slices.Contains(validLogLevels, c.LogLevel) {
		return fmt.Errorf("invalid log level: %s, must be one of %v", c.LogLevel, validLogLevels)
	}

	if c.Webserver.ListenPort < 1 || c.Webserver.ListenPort > 65535 {
		return fmt.Errorf("invalid port: %d", c.Webserver.ListenPort)
	}

	// prod does not fall back to the embedded certificate. Checking here refuses a reload
	// with a missing file before the running App is torn down.
	if c.Env == ProdEnv {
		for key, file := range map[string]string{"certFile": c.Webserver.CertFile, "keyFile": c.Webserver.KeyFile} {
			if _, err := os.Stat(file); err != nil {
				return fmt.Errorf("webserver.%s is required with env %s: %w", key, ProdEnv, err)
			}
		}
	}

	// Visit the relays in a fixed order, so the reported duplicate does not depend on map order.
	names := make([]string, 0, len(c.Relays))
	for name := range c.Relays {
		names = append(names, name)
	}
	sort.Strings(names)

	gpioUsedBy := make(map[int]string, len(names))
	for _, name := range names {
		relay := c.Relays[name]
		if name == "" {
			return errors.New("relay name must not be empty")
		}
		// The name is a path segment of /relays/{name}; these could never be addressed.
		if strings.Contains(name, "/") || name == "." || name == ".." {
			return fmt.Errorf("relay %q: name must not contain \"/\" or be \".\" or \"..\"", name)
		}
		if relay.GPIO < minGPIO || relay.GPIO > maxGPIO {
			return fmt.Errorf("relay %q: gpio must be between %d and %d, got %d", name, minGPIO, maxGPIO, relay.GPIO)
		}
		if other, used := gpioUsedBy[relay.GPIO]; used {
			return fmt.Errorf("relays %q and %q both use gpio %d", other, name, relay.GPIO)
		}
		gpioUsedBy[relay.GPIO] = name

		switch relay.startMode() {
		case StartOff, StartOn:
		case StartLast:
			if c.StateFile == "" {
				return fmt.Errorf("relay %q: startState %s needs a stateFile", name, StartLast)
			}
		default:
			return fmt.Errorf("relay %q: invalid startState %q, must be %s, %s or %s", name, relay.StartState, StartOff, StartOn, StartLast)
		}

		if relay.MinSwitchInterval < 0 || relay.MinSwitchInterval > maxSwitchInterval {
			return fmt.Errorf("relay %q: minSwitchInterval must be between 0 and %s, got %s", name, maxSwitchInterval, relay.MinSwitchInterval)
		}

		switch relay.color() {
		case ColorGreen, ColorRed, ColorAmber:
		default:
			return fmt.Errorf("relay %q: invalid color %q, must be %s, %s or %s", name, relay.Color, ColorGreen, ColorRed, ColorAmber)
		}
	}

	return nil
}
