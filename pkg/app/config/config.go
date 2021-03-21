package config

import (
	"fmt"
	"github.com/womat/debug"
	"gopkg.in/yaml.v2"
	"io"
	"os"
)

// Config holds the application configuration. Attention!
// To make it possible to overwrite fields with the -overwrite command
// line option each of the struct fields must be in the format
// first letter uppercase -> followed by CamelCase as in the config file.
// Config defines the struct of global config and the struct of the configuration file
type Config struct {
	Flag struct {
		Version    bool
		List       bool
		Debug      string
		ConfigFile string
	}

	Simulation bool                   `yaml:"simulation"`
	Relay      map[string]RelayConfig `yaml:"relay"`
	Webserver  WebserverConfig        `yaml:"webserver"`
	Debug      DebugConfig            `yaml:"debug"`
}

// DebugConfig defines the struct of the debug configuration and configuration file
type DebugConfig struct {
	File       io.WriteCloser `yaml:"-"`
	Flag       int            `yaml:"-"`
	FlagString string         `yaml:"flag"`
	FileString string         `yaml:"file"`
}

// WebserverConfig defines the struct of the webserver and webservice configuration and configuration file
type WebserverConfig struct {
	URL         string          `yaml:"url"`
	Webservices map[string]bool `yaml:"webservices"`
}

type RelayConfig struct {
	Description string `yaml:"description"`
	GPIO        int    `yaml:"gpio"`
}

func NewConfig() *Config {
	return &Config{
		Flag: struct {
			Version    bool
			List       bool
			Debug      string
			ConfigFile string
		}{},

		Webserver: WebserverConfig{
			URL: "http://0.0.0.0:8080",
			Webservices: map[string]bool{
				"version": true,
				"health":  true,
				"switch":  false,
				"state":   false,
			},
		},
		Debug: DebugConfig{
			FileString: "stderr",
			FlagString: "standard"},
	}
}

func (c *Config) LoadConfig() error {
	if err := c.readConfigFile(); err != nil {
		return fmt.Errorf("error reading config file %q: %w", c.Flag.ConfigFile, err)
	}

	if c.Flag.Debug != "" {
		c.Debug.FlagString = c.Flag.Debug
	}
	if err := c.setDebugConfig(); err != nil {
		return fmt.Errorf("unable to open debug file %q: %w", c.Debug, err)
	}

	return nil
}

func (c *Config) readConfigFile() error {
	file, err := os.Open(c.Flag.ConfigFile)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	decoder := yaml.NewDecoder(file)
	if err = decoder.Decode(c); err != nil {
		return err
	}

	return nil
}

func (c *Config) setDebugConfig() (err error) {
	// defines Debug section of global.Config
	switch c.Debug.FlagString {
	case "trace", "full":
		c.Debug.Flag = debug.Full
	case "debug":
		c.Debug.Flag = debug.Warning | debug.Info | debug.Error | debug.Fatal | debug.Debug
	case "standard":
		c.Debug.Flag = debug.Standard
	}

	switch c.Debug.FileString {
	case "stderr":
		c.Debug.File = os.Stderr
	case "stdout":
		c.Debug.File = os.Stdout
	default:
		if c.Debug.File, err = os.OpenFile(c.Debug.FileString, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666); err != nil {
			return
		}
	}

	return
}
