package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"ctx/internal/model"
)

const (
	lockName       = "ctx.json.lock"
	maxLockRetries = 50
	lockRetryMs    = 100
)

// Load reads ctx.json from path. If the file does not exist, it creates an empty
// ContextFile. It does not acquire a lock.
func Load(path string) (*model.ContextFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &model.ContextFile{Sessions: make(map[string]*model.Session)}, nil
		}
		return nil, fmt.Errorf("store: load: %w", err)
	}

	var cf model.ContextFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("store: load: invalid JSON: %w", err)
	}
	if cf.Sessions == nil {
		cf.Sessions = make(map[string]*model.Session)
	}
	return &cf, nil
}

// Save writes the ContextFile to path via a temp file + rename for atomicity.
func Save(path string, cf *model.ContextFile) error {
	data, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return fmt.Errorf("store: save: marshal: %w", err)
	}

	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, "ctx-*.json.tmp")
	if err != nil {
		return fmt.Errorf("store: save: create temp: %w", err)
	}
	tmpPath := tmpFile.Name()

	_, err = tmpFile.Write(data)
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("store: save: write temp: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("store: save: sync temp: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("store: save: close temp: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("store: save: rename: %w", err)
	}
	return nil
}

type flock struct {
	Type   int16
	Whence int16
	Start  int64
	Len    int64
	Pid    int32
}

// WithLock acquires an exclusive flock on path+lockName, calls fn, and releases the lock.
func WithLock(path string, fn func() error) error {
	dir := filepath.Dir(path)
	lockPath := filepath.Join(dir, lockName)

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("store: lock: open: %w", err)
	}
	defer f.Close()

	var lkErr error
	for i := 0; i < maxLockRetries; i++ {
		fd := f.Fd()
		flk := syscall.Flock_t{
			Type:  syscall.F_WRLCK,
			Whence: 0,
			Start:  0,
			Len:    0,
			Pid:    int32(os.Getpid()),
		}
		lkErr = syscall.FcntlFlock(uintptr(fd), syscall.F_SETLK, &flk)
		if lkErr == nil {
			break
		}
		if lkErr == syscall.EACCES || lkErr == syscall.EAGAIN {
			time.Sleep(time.Duration(lockRetryMs) * time.Millisecond)
			continue
		}
		return fmt.Errorf("store: lock: %w", lkErr)
	}
	if lkErr != nil {
		return fmt.Errorf("store: lock: could not acquire lock after %d retries: %w", maxLockRetries, lkErr)
	}

	var once sync.Once
	var fnErr error
	func() {
		defer func() {
			flk := syscall.Flock_t{
				Type:   syscall.F_UNLCK,
				Whence: 0,
				Start:  0,
				Len:    0,
				Pid:    int32(os.Getpid()),
			}
			syscall.FcntlFlock(uintptr(f.Fd()), syscall.F_SETLK, &flk)
			if r := recover(); r != nil {
				once.Do(func() {
					fnErr = fmt.Errorf("store: lock: panic: %v", r)
				})
			}
		}()
		fnErr = fn()
	}()
	return fnErr
}
