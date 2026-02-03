package client

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ktoks/remote/internal/config"
	"github.com/ktoks/remote/internal/ipc"
	"github.com/ktoks/remote/internal/protocol"
)

// Run processes the client request (Single or Batch).
func Run(linkName, host string, batchMode bool, args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	socketPath := config.ResolveSocketPath(homeDir, linkName)
	conn, err := connectOrSpawn(socketPath, linkName)
	if err != nil {
		return err
	}
	defer func() {
		if close_err := conn.Close(); close_err != nil {
			fmt.Fprintf(os.Stderr, "client close error: %s", close_err)
		}
	}()

	if batchMode {
		return runBatch(conn)
	}

	if len(args) == 0 {
		return fmt.Errorf("no command provided")
	}

	cmd := strings.Join(args, " ")
	return runSingle(conn, cmd)
}

func runSingle(conn net.Conn, cmd string) error {
	// Send Command
	if _, err := fmt.Fprintf(conn, "%s\n", cmd); err != nil {
		return err
	}

	// Handle Response
	return protocol.DecodeLoop(conn,
		func(b []byte) {
			if _, os_err := os.Stdout.Write(b); os_err != nil {
				fmt.Fprintf(os.Stderr, "Error occurred writing to STDOUT: %v", os_err)
			}
		},
		func(b []byte) {
			if _, os_err := os.Stderr.Write(b); os_err != nil {
				fmt.Fprintf(os.Stderr, "Error occurred writing to STDERR: %v", os_err)
			}
		},
		func(code int) bool {
			os.Exit(code) // Hard exit on single command
			return true
		},
	)
}

func runBatch(conn net.Conn) error {
	// Async Sender
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			cmd := strings.TrimSpace(scanner.Text())
			if cmd == "" {
				continue
			}
			if _, err := fmt.Fprintf(conn, "%s\n", cmd); err != nil {
				fmt.Fprintf(os.Stderr, "Error occurred printing to connection: %s", err)
			}
		}
		// Close Write side to signal EOF to server
		if c, ok := conn.(*net.UnixConn); ok {
			if err := c.CloseWrite(); err != nil {
				fmt.Fprintf(os.Stderr, "Error occurred sending EOF signal to server: %s", err)
			}
		}
	}()

	// Sync Receiver
	return protocol.DecodeLoop(conn,
		func(b []byte) {
			if _, os_err := os.Stdout.Write(b); os_err != nil {
				fmt.Fprintf(os.Stderr, "Error occurred writing to STDOUT: %v", os_err)
			}
		},
		func(b []byte) {
			if _, os_err := os.Stderr.Write(b); os_err != nil {
				fmt.Fprintf(os.Stderr, "Error occurred writing to STDERR: %v", os_err)
			}
		},
		func(code int) bool {
			if code != 0 {
				fmt.Fprintf(os.Stderr, "[Exit %d]\n", code)
			}
			return false // Don't stop loop in batch mode
		},
	)
}

func connectOrSpawn(socketPath, linkName string) (net.Conn, error) {
	conn, err := net.Dial("unix", socketPath)
	if err == nil {
		return conn, nil
	}

	// --- Comprehensive Cleanup ---

	// 1. Clean up based on lock file status (stale or zombie with no socket)
	lockPath := filepath.Join(filepath.Dir(socketPath), linkName+".lock")
	ipc.CheckAndCleanLock(lockPath, socketPath)

	// 2. Clean up any remaining zombies that have no lock file.
	exe, _ := os.Executable()
	searchPattern := fmt.Sprintf("%s --daemon %s", exe, linkName)
	// Use pgrep to find processes by full command line.
	// -f: Interpret pattern as a full extended regular expression
	pgrepCmd := exec.Command("pgrep", "-f", searchPattern)
	if output, err := pgrepCmd.Output(); err == nil {
		pids := strings.Fields(string(output))
		for _, pidStr := range pids {
			if pid, convErr := strconv.Atoi(pidStr); convErr == nil {
				// Don't kill ourselves if pgrep finds the current client process
				if pid == os.Getpid() {
					continue
				}
				if process, findErr := os.FindProcess(pid); findErr == nil {
					_ = process.Kill()
					fmt.Fprintf(os.Stderr, "Killed zombie daemon process with PID %d\n", pid)
				}
			}
		}
		if len(pids) > 0 {
			time.Sleep(100 * time.Millisecond) // Wait for killed processes
		}
	}


	// --- Spawn New Daemon ---

	cmd := exec.Command(exe, "--daemon", linkName)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // Detach
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to spawn daemon: %w", err)
	}

	// Retry loop
	for range 20 {
		time.Sleep(100 * time.Millisecond)
		conn, err = net.Dial("unix", socketPath)
		if err == nil {
			return conn, nil
		}
	}
	return nil, fmt.Errorf("timeout waiting for daemon")
}
