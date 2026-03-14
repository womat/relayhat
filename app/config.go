package app

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"gopkg.in/yaml.v3"
)

const (
	ProdEnv = "prod"
	DevEnv  = "dev"
)

// Config holds the application's YAML configuration.
//
// Field names stay exported and use CamelCase so they can be targeted by future
// command-line overwrite support.
type Config struct {
	Env            string          `yaml:"env"`            // Application environment: dev | prod
	LogLevel       string          `yaml:"logLevel"`       // Log level: debug | info | warning | error
	LogDestination string          `yaml:"logDestination"` // Log output: stdout | stderr | /path/to/logfile
	Webserver      WebserverConfig `yaml:"webserver"`      // Webserver configuration

	Relays map[string]RelayConfig `yaml:"relay"`
}

// WebserverConfig holds HTTPS server settings.
type WebserverConfig struct {
	ListenHost string   `yaml:"listenHost"` // Host address for web server
	ListenPort int      `yaml:"listenPort"` // Port for web server
	ApiKey     string   `yaml:"apiKey"`     // API key for requests
	JwtSecret  string   `yaml:"jwtSecret"`  // Secret for JWT tokens
	JwtID      string   `yaml:"jwtID"`      // Unique JWT ID
	KeyFile    string   `yaml:"keyFile"`    // SSL private key file
	CertFile   string   `yaml:"certFile"`   // SSL certificate file
	BlockedIPs []string `yaml:"blockedIPs"` // Forbidden IP addresses or networks
	AllowedIPs []string `yaml:"allowedIPs"` // Allowed IP addresses or networks
}

// RelayConfig holds the configuration for a single relay from the YAML file.
type RelayConfig struct {
	GPIO        int    `yaml:"gpio"`
	Description string `yaml:"description"`
}

// NewConfig returns a Config initialized with default values.
func NewConfig() *Config {
	return &Config{
		Env:            DevEnv,
		LogLevel:       "info",
		LogDestination: "stdout",
		Webserver: WebserverConfig{
			ListenHost: "0.0.0.0",
			ListenPort: 8443,
			BlockedIPs: []string{},
			AllowedIPs: []string{},
		},
	}
}

// LoadConfig loads configuration from a YAML file and expands environment variables.
func LoadConfig(fileName string) (*Config, error) {
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

	// Replace environment variables in the YAML
	replaced := os.ExpandEnv(string(content))

	// Unmarshal YAML into the config struct
	if err = yaml.Unmarshal([]byte(replaced), cfg); err != nil {
		return cfg, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return cfg, nil
}

// IsDevEnv returns true if the environment is development.
func (c *Config) IsDevEnv() bool {
	return c.Env == DevEnv
}

// Validate checks the Config for invalid or missing values.
func (c *Config) Validate() error {

	if c.Env != ProdEnv && c.Env != DevEnv {
		return fmt.Errorf("invalid environment: %s, must be %s or %s", c.Env, ProdEnv, DevEnv)
	}
	if c.Webserver.ApiKey == "" {
		return errors.New("ApiKey is not configured")
	}

	validLogLevels := []string{"debug", "info", "warning", "warn", "error"}
	if !slices.Contains(validLogLevels, c.LogLevel) {
		return fmt.Errorf("invalid log level: %s, must be one of %v", c.LogLevel, validLogLevels)
	}

	if c.Webserver.ListenPort < 1 || c.Webserver.ListenPort > 65535 {
		return fmt.Errorf("invalid port: %d", c.Webserver.ListenPort)
	}

	for name, relay := range c.Relays {
		if name == "" {
			return errors.New("relay name must not be empty")
		}

		if relay.GPIO <= 0 {
			return fmt.Errorf("relay %q: gpio pin must be greater than 0, got %d", name, relay.GPIO)
		}

	}

	return nil
}
