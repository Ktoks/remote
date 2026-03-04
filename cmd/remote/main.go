package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

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
		daemon.Start(*flagDaemon, *flagDaemon, home)
		return
	}

	// 2. Client Mode
	linkName := filepath.Base(os.Args[0])

	if err := client.Run(linkName, linkName, *flagBatch, flag.Args()); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
