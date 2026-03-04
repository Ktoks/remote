package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mvdan.cc/sh/v3/syntax"

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
	Address         string              `json:"address"`
	Port            string              `json:"port"`
	User            string              `json:"user"`
	IgnoreHostKey   bool                `json:"ignore_host_key"`
	AllowedCommands []string            `json:"allowed_commands"`
	Constraints     []CommandConstraint `json:"constraints"`
	Security        *SecurityRules      `json:"security"`
}

// SecurityRules defines global or per-host shell feature restrictions
type SecurityRules struct {
	AllowPipes     bool `json:"allow_pipes"`
	AllowRedirects bool `json:"allow_redirects"`
	AllowChaining  bool `json:"allow_chaining"` // Allow ;, &&, ||
}

// CommandConstraint defines specific restrictions for an allowed command
type CommandConstraint struct {
	Command   string   `json:"command"`
	AllowArgs []string `json:"allow_args"` // Regex-like patterns (currently simple prefix)
	DenyArgs  []string `json:"deny_args"`
}

// IsCommandAllowed checks if a command is in the list of allowed commands
func (c *HostConfig) IsCommandAllowed(command string) bool {
	// 1. Check explicitly listed constraints first
	for _, constraint := range c.Constraints {
		if command == constraint.Command {
			return true
		}
	}

	// 2. Check simple allowed list
	for _, allowed := range c.AllowedCommands {
		if command == allowed {
			return true
		}
	}
	return false
}

// ValidateShellCommand parses and validates a shell string using mvdan/sh
func (c *HostConfig) ValidateShellCommand(cmdStr string) error {
	p := syntax.NewParser()
	f, err := p.Parse(strings.NewReader(cmdStr), "")
	if err != nil {
		return fmt.Errorf("invalid shell syntax: %w", err)
	}

	sec := c.Security
	if sec == nil {
		// Default strict security if not specified
		sec = &SecurityRules{
			AllowPipes:     false,
			AllowRedirects: false,
			AllowChaining:  false,
		}
	}

	// Check for multiple statements (semicolon or newline chaining)
	if !sec.AllowChaining && len(f.Stmts) > 1 {
		return fmt.Errorf("multiple commands (;) are disabled")
	}

	var validationErr error
	syntax.Walk(f, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.BinaryCmd:
			// Op can be syntax.AndStmt (&&), syntax.OrStmt (||), syntax.Pipe (|)
			if n.Op == syntax.AndStmt || n.Op == syntax.OrStmt {
				if !sec.AllowChaining {
					validationErr = fmt.Errorf("command chaining (&&, ||) is disabled")
					return false
				}
			}
			if n.Op == syntax.Pipe || n.Op == syntax.PipeAll {
				if !sec.AllowPipes {
					validationErr = fmt.Errorf("pipes (|) are disabled")
					return false
				}
			}
		case *syntax.Redirect:
			if !sec.AllowRedirects {
				validationErr = fmt.Errorf("I/O redirection is disabled")
				return false
			}
		case *syntax.CallExpr:
			// This is an actual command execution (e.g., "ls -l")
			if len(n.Args) == 0 {
				return true
			}
			// Get the command name (first argument)
			// For simplicity, we only handle literal names (no variables like $CMD)
			if len(n.Args[0].Parts) == 0 {
				return true
			}
			lit, ok := n.Args[0].Parts[0].(*syntax.Lit)
			if !ok {
				validationErr = fmt.Errorf("dynamic commands (variables/subshells) are disabled")
				return false
			}

			cmdName := lit.Value
			if !c.IsCommandAllowed(cmdName) {
				validationErr = fmt.Errorf("command not allowed: %s", cmdName)
				return false
			}

			// Check constraints if any
			for _, constraint := range c.Constraints {
				if constraint.Command == cmdName {
					// Check arguments
					for i := 1; i < len(n.Args); i++ {
						arg := ""
						for _, p := range n.Args[i].Parts {
							if l, ok := p.(*syntax.Lit); ok {
								arg += l.Value
							}
						}

						// Deny check
						for _, deny := range constraint.DenyArgs {
							if strings.Contains(arg, deny) {
								validationErr = fmt.Errorf("argument '%s' is forbidden for command '%s'", arg, cmdName)
								return false
							}
						}
						// If AllowArgs is present, it must match one of them
						if len(constraint.AllowArgs) > 0 {
							allowed := false
							for _, allow := range constraint.AllowArgs {
								if strings.HasPrefix(arg, allow) {
									allowed = true
									break
								}
							}
							if !allowed {
								validationErr = fmt.Errorf("argument '%s' is not in the allowed list for command '%s'", arg, cmdName)
								return false
							}
						}
					}
				}
			}
		}
		return true
	})

	return validationErr
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

	// Environment Overrides
	if envUser := os.Getenv("REMOTE_USER"); envUser != "" {
		newCfg.User = envUser
	}
	if envAddr := os.Getenv("REMOTE_ADDR"); envAddr != "" {
		newCfg.Address = envAddr
	}
	if envPort := os.Getenv("REMOTE_PORT"); envPort != "" {
		newCfg.Port = envPort
	}
	if envIgnore := os.Getenv("REMOTE_IGNORE_HOST_KEY"); envIgnore != "" {
		envIgnore = strings.ToLower(envIgnore)
		newCfg.IgnoreHostKey = (envIgnore == "true" || envIgnore == "1" || envIgnore == "yes")
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
