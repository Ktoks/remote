package daemon

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ktoks/remote/internal/config"
	"github.com/ktoks/remote/internal/ipc"
	"github.com/ktoks/remote/internal/protocol"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Start initiates the SSH master process.
func Start(host, linkName, homeDir string) {
	// 1. Setup Logging
	setupDaemonLogging(homeDir, linkName)
	log.Printf("Daemon starting for %s...", host)

	// 2. Lock
	socketPath := config.ResolveSocketPath(homeDir, linkName)
	lockPath := filepath.Join(filepath.Dir(socketPath), linkName+".lock")

	lockFile, err := ipc.AcquireLock(lockPath)
	if err != nil {
		log.Fatalf("Failed to acquire lock: %v", err)
	}
	defer ipc.ReleaseLock(lockFile)

	// 3. Establish SSH Connection
	client, err := createSSHClient(host, homeDir)
	if err != nil {
		log.Fatalf("%v", err)
	}
	defer client.Close()

	// 4. Setup Unix Socket Listener
	os.Remove(socketPath) // Clean up old socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("Failed to listen on socket: %v", err)
	}
	defer listener.Close()
	defer os.Remove(socketPath)

	log.Printf("Ready. Listening on %s", socketPath)

	// 5. Accept Loop
	serveLoop(listener, client)
}

func serveLoop(listener net.Listener, sshClient *ssh.Client) {
	var activeConns int32

	for {
		// Set deadline to kill daemon if idle
		listener.(*net.UnixListener).SetDeadline(time.Now().Add(config.IdleTimeout))

		conn, err := listener.Accept()
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
				if atomic.LoadInt32(&activeConns) > 0 {
					continue // Active connections exist, extend life
				}
				log.Println("Idle timeout reached. Shutting down.")
				return
			}
			log.Printf("Accept error: %v", err)
			return
		}

		atomic.AddInt32(&activeConns, 1)
		go func() {
			defer atomic.AddInt32(&activeConns, -1)
			handleConnection(conn, sshClient)
		}()
	}
}

func handleConnection(conn net.Conn, client *ssh.Client) {
	defer conn.Close()
	encoder := protocol.NewEncoder(conn)
	reader := bufio.NewReader(conn)

	// Limit concurrency per client connection
	sem := make(chan struct{}, 50)
	var wg sync.WaitGroup

	for {
		cmdStr, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		cmdStr = strings.TrimSpace(cmdStr)
		if cmdStr == "" {
			continue
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(cmd string) {
			defer wg.Done()
			defer func() { <-sem }()
			execRemote(client, cmd, encoder)
		}(cmdStr)
	}
	wg.Wait()
}

func execRemote(client *ssh.Client, cmd string, enc *protocol.Encoder) {
	session, err := client.NewSession()
	if err != nil {
		var buf []byte
		enc.Encode(protocol.TypeStderr, fmt.Appendf(buf, "SSH session error: %v\n", err))
		enc.Encode(protocol.TypeExit, intToBytes(255))
		return
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)

	// Send Output
	if len(output) > 0 {
		enc.Encode(protocol.TypeStdout, output)
	}

	// Determine Exit Code
	exitCode := 0
	if err != nil {
		exitErr, ok := err.(*ssh.ExitError)
		if ok {
			exitCode = exitErr.ExitStatus()
		} else {
			log.Println("session error (closing): ", exitErr)
			exitCode = 1
		}
		if exitErr == io.EOF {
			log.Println("completed processing: ", cmd)
		}
	}

	// Send Exit Packet
	enc.Encode(protocol.TypeExit, intToBytes(exitCode))
}

// Helpers

func intToBytes(n int) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(n))
	return b
}

func setupDaemonLogging(homeDir, identity string) {
	logPath := filepath.Join(homeDir, config.SocketSubDir, identity+".log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Printf("ERROR: Failed to open log file %s: %v", logPath, err)
		// Fallback to stderr if file fails
		return
	}
	// We do not close f explicitly; it remains open for the lifetime of the daemon
	// or until OS cleans up.
	log.SetOutput(f)
}

func createSSHClient(host, home string) (*ssh.Client, error) {
	// Enterprise Strictness: Always check known_hosts
	knownHostPath := filepath.Join(home, ".ssh", "known_hosts")
	hostKeyCallback, err := knownhosts.New(knownHostPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load known_hosts: %w", err)
	}

	// Auth: Agent + Key Files
	var methods []ssh.AuthMethod

	// 1. Agent
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			agentClient := agent.NewClient(conn)
			methods = append(methods, ssh.PublicKeysCallback(agentClient.Signers))
		}
	}

	// 2. Keys
	keyFiles := []string{"id_ed25519", "id_rsa"}
	for _, name := range keyFiles {
		keyPath := filepath.Join(home, ".ssh", name)
		keyBytes, err := os.ReadFile(keyPath)
		if err == nil {
			signer, err := ssh.ParsePrivateKey(keyBytes)
			if err == nil {
				methods = append(methods, ssh.PublicKeys(signer))
			}
		}
	}

	if len(methods) == 0 {
		return nil, errors.New("no valid authentication methods found (agent or keys)")
	}

	cfg := &ssh.ClientConfig{
		User:            os.Getenv("USER"),
		Auth:            methods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         5 * time.Second,
	}

	return ssh.Dial("tcp", net.JoinHostPort(host, config.RemotePort), cfg)
}
