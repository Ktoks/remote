package ipc

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// AcquireLock attempts to acquire an exclusive file lock.
// It returns the file handle (which must be closed later) or an error.
func AcquireLock(lockPath string) (*os.File, error) {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(lockPath), 0700); err != nil {
		return nil, fmt.Errorf("mkdir failed: %w", err)
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	// Non-blocking lock attempt. Use LOCK_NB if you want to fail immediately,
	// but here we wait (LOCK_EX) as per original logic.
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}

	return f, nil
}

// ReleaseLock unlocks and closes the file.
func ReleaseLock(f *os.File) {
	if f == nil {
		return
	}
	unix.Flock(int(f.Fd()), unix.LOCK_UN)
	f.Close()
}
