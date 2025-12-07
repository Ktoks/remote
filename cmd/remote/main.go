package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ktoks/remote/internal/client"
	"github.com/ktoks/remote/internal/daemon"
)

var (
	flagDaemon = flag.String("daemon", "", "Internal: run as daemon for identity")
	flagBatch  = flag.Bool("batch", false, "Run in batch mode")
)

func main() {
	flag.Parse()

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Cannot get user home: %v", err)
	}

	// 1. Daemon Mode
	if *flagDaemon != "" {
		host := resolveHost(*flagDaemon)
		daemon.Start(host, *flagDaemon, home)
		return
	}

	// 2. Client Mode
	linkName := filepath.Base(os.Args[0])
	host := resolveHost(linkName)

	if err := client.Run(linkName, host, *flagBatch, flag.Args()); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// resolveHost maps symlink names to actual SSH hosts.
// Logic extracted from original code.
func resolveHost(name string) string {
	if strings.Contains(name, "mcpi") {
		return "mcpi"
	}
	if strings.Contains(name, "ftb") {
		return "ftb"
	}
	return name
}
