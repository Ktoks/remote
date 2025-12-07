package config

import (
	"path/filepath"
	"time"
)

const (
	// RemotePort SSH default port
	RemotePort = "22"

	// SocketSubDir - where unix sockets will reside
	SocketSubDir = ".ssh/sockets"
	// IdleTimeout - how long the master will exist
	IdleTimeout = 5 * time.Minute
)

// ResolveSocketPath calculates the absolute path for the unix socket.
func ResolveSocketPath(homeDir, identity string) string {
	return filepath.Join(homeDir, SocketSubDir, identity+".sock")
}
