package ipc

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// AcquireLock attempts to acquire an exclusive file lock and writes the PID to it.
func AcquireLock(lockPath string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0700); err != nil {
		return nil, fmt.Errorf("mkdir for lock failed: %w", err)
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if close_err := f.Close(); close_err != nil {
			fmt.Printf("Error occurred closing Unix Socket: %s", close_err)
		}
		return nil, fmt.Errorf("cannot acquire lock, another instance may be running: %w", err)
	}

	// Write PID to lock file
	pid := os.Getpid()
	if _, err := f.WriteString(strconv.Itoa(pid)); err != nil {
		if close_err := f.Close(); close_err != nil {
			fmt.Printf("Error occurred closing Unix Socket: %s", close_err)
		}
		return nil, fmt.Errorf("failed to write PID to lock file: %w", err)
	}

	return f, nil
}

// CheckAndCleanLock checks for and cleans up stale or zombie lock files.
func CheckAndCleanLock(lockPath, socketPath string) {
	// 1. Check if lock file exists. If not, nothing to do.
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		return
	}

	// 2. Read PID from lock file.
	pid, err := readPIDFromLock(lockPath)
	if err != nil {
		// Lock file is corrupt or unreadable, remove it.
		_ = os.Remove(lockPath)
		return
	}

	// 3. Check if the process is actually running.
	process, err := os.FindProcess(pid)
	if err != nil || process == nil {
		// Process not found, lock is stale.
		_ = os.Remove(lockPath)
		return
	}
	// On Unix, we send signal 0 to check for process existence.
	if err := process.Signal(syscall.Signal(0)); err != nil {
		// Process is not running, lock is stale.
		_ = os.Remove(lockPath)
		return
	}

	// 4. Process is running. Check if it's a zombie (socket is gone).
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		// Zombie process. Kill it and clean up the lock.
		_ = process.Kill()
		time.Sleep(100 * time.Millisecond) // Give it a moment to die.
		_ = os.Remove(lockPath)
	}
}

// readPIDFromLock reads a PID from a lock file.
func readPIDFromLock(lockPath string) (int, error) {
	pidBytes, err := ioutil.ReadFile(lockPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read PID from lock: %w", err)
	}
	pid, err := strconv.Atoi(string(pidBytes))
	if err != nil {
		return 0, fmt.Errorf("failed to parse PID from lock: %w", err)
	}
	return pid, nil
}


// ReleaseLock unlocks and closes the file.
func ReleaseLock(f *os.File) {
	if f == nil {
		return
	}
	// Truncate file before releasing lock
	if err := f.Truncate(0); err != nil {
		fmt.Printf("Error truncating lock file: %s", err)
	}
	if lock_err := unix.Flock(int(f.Fd()), unix.LOCK_UN); lock_err != nil {
		fmt.Printf("Error occurred unlocking Unix Socket: %s", lock_err)
	}
	if close_err := f.Close(); close_err != nil {
		fmt.Printf("Error occurred closing Unix Socket: %s", close_err)
	}
}
