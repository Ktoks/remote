package config

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"time"

	_ "embed"
)

//go:embed config.json
var defaultConfigFile []byte

const (
	// SocketSubDir - where unix sockets will reside
	SocketSubDir = ".ssh/sockets"
	// IdleTimeout - how long the master will exist
	IdleTimeout = 5 * time.Minute
)

// HostConfig defines settings for a specific host
type HostConfig struct {
	Address         string   `json:"address"`
	Port            string   `json:"port"`
	User            string   `json:"user"`
	IgnoreHostKey   bool     `json:"ignore_host_key"`
	AllowedCommands []string `json:"allowed_commands"`
}

// IsCommandAllowed checks if a command is in the list of allowed commands
func (c *HostConfig) IsCommandAllowed(command string) bool {
	for _, allowed := range c.AllowedCommands {
		if command == allowed {
			return true
		}
	}
	return false
}

// Config holds the application configuration
type Config struct {
	Hosts    map[string]HostConfig `json:"hosts"`
	Defaults HostConfig            `json:"defaults"`
}

// GetHostConfig returns the configuration for a specific host, falling back
// to defaults for any unset values.
func (c *Config) GetHostConfig(host string) *HostConfig {
	hostCfg, ok := c.Hosts[host]
	if !ok {
		// No specific config for this host, return defaults but set address to host if default address is empty
		cfg := c.Defaults
		if cfg.Address == "" {
			cfg.Address = host
		}
		if cfg.Port == "" {
			cfg.Port = "22"
		}
		return &cfg
	}

	// Host config exists, but might be missing values. Fill in with defaults.
	// We make a local copy to modify and return a pointer to it (escapes to heap)
	newCfg := hostCfg

	if newCfg.Address == "" {
		if c.Defaults.Address != "" {
			newCfg.Address = c.Defaults.Address
		} else {
			newCfg.Address = host
		}
	}
	if newCfg.Port == "" {
		if c.Defaults.Port != "" {
			newCfg.Port = c.Defaults.Port
		} else {
			newCfg.Port = "22"
		}
	}
	if newCfg.User == "" {
		newCfg.User = c.Defaults.User
	}
	if !newCfg.IgnoreHostKey {
		newCfg.IgnoreHostKey = c.Defaults.IgnoreHostKey
	}
	if len(newCfg.AllowedCommands) == 0 {
		newCfg.AllowedCommands = c.Defaults.AllowedCommands
	}

	return &newCfg
}

// LoadConfig reads the configuration from a JSON file
func LoadConfig(path string) (*Config, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// LoadDefaultConfig loads the embedded configuration
func LoadDefaultConfig() (*Config, error) {
	var config Config
	if err := json.Unmarshal(defaultConfigFile, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// ResolveSocketPath calculates the absolute path for the unix socket.
func ResolveSocketPath(homeDir, identity string) string {
	return filepath.Join(homeDir, SocketSubDir, identity+".sock")
}
