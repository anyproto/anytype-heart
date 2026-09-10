// Package accountdirlock provides a process-wide lease for an account directory.
package accountdirlock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/gofrs/flock"
	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const (
	lockDirName       = ".anytype-locks"
	acquisitionWindow = time.Second
	retryDelay        = 50 * time.Millisecond
)

var (
	ErrLocked        = errors.New("account directory is locked by another process")
	ErrUnavailable   = errors.New("account directory lock is unavailable")
	processStartedAt = time.Now().UTC()
	log              = logging.Logger("account-directory-lock")
)

// Owner is diagnostic metadata. It must never be used to decide whether a lease
// is stale: only the operating-system lock is authoritative.
type Owner struct {
	LeaseID           string     `json:"leaseId"`
	AccountID         string     `json:"accountId"`
	PID               int        `json:"pid"`
	Executable        string     `json:"executable,omitempty"`
	MiddlewareVersion string     `json:"middlewareVersion,omitempty"`
	Hostname          string     `json:"hostname,omitempty"`
	ProcessStartedAt  *time.Time `json:"processStartedAt,omitempty"`
	AcquiredAt        time.Time  `json:"acquiredAt"`
	ReleasedAt        *time.Time `json:"releasedAt,omitempty"`
	State             string     `json:"state"`
}

// LockedError describes a contended lease and includes best-effort owner data.
type LockedError struct {
	LockPath string
	Owner    *Owner
}

func (e *LockedError) Error() string {
	if e.Owner == nil {
		return fmt.Sprintf("%s: %s", ErrLocked, e.LockPath)
	}
	return fmt.Sprintf("%s: %s (pid %d)", ErrLocked, e.LockPath, e.Owner.PID)
}

func (e *LockedError) Unwrap() error { return ErrLocked }

// Lease holds the OS lock until Release is called or the process exits.
type Lease struct {
	mu            sync.Mutex
	lock          *flock.Flock
	rootPath      string
	accountID     string
	lockPath      string
	ownerPath     string
	owner         Owner
	released      bool
	releaseFailed bool
}

// Acquire obtains an exclusive lease for accountID under rootPath.
func Acquire(ctx context.Context, rootPath, accountID, middlewareVersion string) (*Lease, error) {
	startedAt := time.Now()
	canonicalRoot, err := CanonicalRootPath(rootPath)
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	if err = ValidateAccountID(accountID); err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}

	lockDir := filepath.Join(canonicalRoot, lockDirName)
	if err = os.MkdirAll(lockDir, 0o700); err != nil {
		return nil, errors.Join(ErrUnavailable, fmt.Errorf("create lock directory: %w", err))
	}
	lockPath := filepath.Join(lockDir, accountID+".lock")
	ownerPath := filepath.Join(lockDir, accountID+".owner.json")
	osLock := flock.New(lockPath, flock.SetFlag(os.O_CREATE|os.O_RDWR), flock.SetPermissions(0o600))

	acquireCtx, cancel := context.WithTimeout(ctx, acquisitionWindow)
	defer cancel()
	locked, err := osLock.TryLockContext(acquireCtx, retryDelay)
	if err != nil {
		_ = osLock.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			owner := readOwner(ownerPath)
			log.Warnw("account directory lease contended",
				"accountId", accountID,
				"rootPath", canonicalRoot,
				"lockPath", lockPath,
				"owner", owner,
				"tookMs", time.Since(startedAt).Milliseconds())
			return nil, &LockedError{LockPath: lockPath, Owner: owner}
		}
		return nil, errors.Join(ErrUnavailable, fmt.Errorf("acquire %s: %w", lockPath, err))
	}
	if !locked {
		_ = osLock.Close()
		return nil, &LockedError{LockPath: lockPath, Owner: readOwner(ownerPath)}
	}

	now := time.Now().UTC()
	executable, _ := os.Executable()
	hostname, _ := os.Hostname()
	lease := &Lease{
		lock:      osLock,
		rootPath:  canonicalRoot,
		accountID: accountID,
		lockPath:  lockPath,
		ownerPath: ownerPath,
		owner: Owner{
			LeaseID:           uuid.NewString(),
			AccountID:         accountID,
			PID:               os.Getpid(),
			Executable:        executable,
			MiddlewareVersion: middlewareVersion,
			AcquiredAt:        now,
			Hostname:          hostname,
			ProcessStartedAt:  &processStartedAt,
			State:             "holding",
		},
	}
	if err = writeOwner(ownerPath, lease.owner); err != nil {
		// Metadata is diagnostic only. The OS lease remains authoritative.
		log.Warnf("failed to write account lock owner metadata: %v", err)
	}
	log.Infow("account directory lease acquired",
		"accountId", accountID,
		"rootPath", canonicalRoot,
		"lockPath", lockPath,
		"tookMs", time.Since(startedAt).Milliseconds())
	return lease, nil
}

// CanonicalRootPath resolves a stable identity for a root directory.
func CanonicalRootPath(rootPath string) (string, error) {
	if rootPath == "" {
		return "", errors.New("empty root path")
	}
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return "", fmt.Errorf("resolve root path: %w", err)
	}
	if err = os.MkdirAll(absRoot, 0o700); err != nil {
		return "", fmt.Errorf("create root path: %w", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", fmt.Errorf("canonicalize root path: %w", err)
	}
	return filepath.Clean(canonicalRoot), nil
}

// ValidateAccountID rejects malformed and non-canonical account addresses
// before they can be used as path components.
func ValidateAccountID(accountID string) error {
	if accountID == "" {
		return errors.New("empty account id")
	}
	key, err := crypto.DecodeAccountAddress(accountID)
	if err != nil {
		return fmt.Errorf("decode account id: %w", err)
	}
	if key.Account() != accountID {
		return errors.New("account id is not canonical")
	}
	return nil
}

func writeOwner(path string, owner Owner) error {
	data, err := json.MarshalIndent(owner, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(append(data, '\n'))
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func readOwner(path string) *Owner {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var owner Owner
	if json.Unmarshal(data, &owner) != nil {
		return nil
	}
	return &owner
}

func (l *Lease) AccountID() string { return l.accountID }
func (l *Lease) RootPath() string  { return l.rootPath }
func (l *Lease) AccountDir() string {
	return filepath.Join(l.rootPath, l.accountID)
}
func (l *Lease) LockPath() string { return l.lockPath }

func (l *Lease) Matches(rootPath, accountID string) bool {
	if l == nil || l.accountID != accountID {
		return false
	}
	canonicalRoot, err := CanonicalRootPath(rootPath)
	return err == nil && canonicalRoot == l.rootPath
}

// Usable reports whether the lease is still held without an ambiguous unlock
// failure. A failed Unlock must prevent the account from being reopened.
func (l *Lease) Usable() bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return !l.released && !l.releaseFailed
}

// Release marks the diagnostic record released and drops the OS lease. It is
// safe to call more than once.
func (l *Lease) Release() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return nil
	}
	now := time.Now().UTC()
	l.owner.ReleasedAt = &now
	l.owner.State = "released"
	if err := writeOwner(l.ownerPath, l.owner); err != nil {
		log.Warnf("failed to update account lock owner metadata: %v", err)
	}
	unlockErr := l.lock.Unlock()
	if unlockErr == nil {
		l.released = true
		log.Infow("account directory lease released",
			"accountId", l.accountID,
			"rootPath", l.rootPath,
			"lockPath", l.lockPath)
	} else {
		l.releaseFailed = true
		log.Errorw("account directory lease release failed",
			"accountId", l.accountID,
			"rootPath", l.rootPath,
			"lockPath", l.lockPath,
			"error", unlockErr)
	}
	return unlockErr
}
