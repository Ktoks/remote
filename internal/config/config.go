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
	// RemotePort SSH default port
	RemotePort = "22"

	// SocketSubDir - where unix sockets will reside
	SocketSubDir = ".ssh/sockets"
	// IdleTimeout - how long the master will exist
	IdleTimeout = 5 * time.Minute
)

// Config holds the application configuration
type Config struct {
	AllowedCommands []string `json:"allowed_commands"`
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


// IsCommandAllowed checks if a command is in the list of allowed commands
func (c *Config) IsCommandAllowed(command string) bool {
	for _, allowed := range c.AllowedCommands {
		if command == allowed {
			return true
		}
	}
	return false
}


// ResolveSocketPath calculates the absolute path for the unix socket.
func ResolveSocketPath(homeDir, identity string) string {
	return filepath.Join(homeDir, SocketSubDir, identity+".sock")
}
