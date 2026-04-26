//go:build unix

package portalloc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func acquireCatalogLock(timeout time.Duration) (*catalogLock, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, catalogLockWritePerms); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}

	lockPath := filepath.Join(dir, "catalog.json.lock")
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, catalogLockFileMode)
	if err != nil {
		return nil, fmt.Errorf("open catalog lock: %w", err)
	}

	deadline := time.Now().Add(timeout)
	for {
		if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			break
		} else if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			_ = file.Close()
			return nil, fmt.Errorf("acquire catalog lock: %w", err)
		}

		if time.Now().After(deadline) {
			holder := readLockHolder(lockPath)
			_ = file.Close()
			if holder != "" {
				return nil, fmt.Errorf("acquire catalog lock: timed out after %s (held by pid %s)", timeout, holder)
			}
			return nil, fmt.Errorf("acquire catalog lock: timed out after %s", timeout)
		}

		time.Sleep(catalogLockRetryDelay)
	}

	if err := file.Truncate(0); err != nil {
		_ = unlockCatalogFile(file)
		_ = file.Close()
		return nil, fmt.Errorf("record catalog lock holder: %w", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		_ = unlockCatalogFile(file)
		_ = file.Close()
		return nil, fmt.Errorf("record catalog lock holder: %w", err)
	}
	if _, err := fmt.Fprintf(file, "%d\n", os.Getpid()); err != nil {
		_ = unlockCatalogFile(file)
		_ = file.Close()
		return nil, fmt.Errorf("record catalog lock holder: %w", err)
	}

	return &catalogLock{file: file}, nil
}

func (l *catalogLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}

	unlockErr := unlockCatalogFile(l.file)
	closeErr := l.file.Close()
	if unlockErr != nil {
		return fmt.Errorf("release catalog lock: %w", unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close catalog lock: %w", closeErr)
	}

	return nil
}

func (l *catalogLock) holderPID() int {
	return os.Getpid()
}

func unlockCatalogFile(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
