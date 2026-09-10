package accountdirlock

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/util/crypto"
)

const (
	helperEnv        = "ANYTYPE_ACCOUNT_LOCK_HELPER"
	helperRootEnv    = "ANYTYPE_ACCOUNT_LOCK_ROOT"
	helperAccountEnv = "ANYTYPE_ACCOUNT_LOCK_ACCOUNT"
	helperReadyEnv   = "ANYTYPE_ACCOUNT_LOCK_READY"
)

func randomAccountID(t *testing.T) string {
	t.Helper()
	_, publicKey, err := crypto.GenerateRandomEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	return publicKey.Account()
}

func TestLeasePersistsOutsideAccountDirectory(t *testing.T) {
	root := t.TempDir()
	accountID := randomAccountID(t)
	accountDir := filepath.Join(root, accountID)
	if err := os.MkdirAll(accountDir, 0o700); err != nil {
		t.Fatal(err)
	}

	lease, err := Acquire(context.Background(), root, accountID, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()

	if strings.HasPrefix(lease.LockPath(), accountDir+string(filepath.Separator)) {
		t.Fatalf("lock file must not be inside account directory: %s", lease.LockPath())
	}
	if !strings.Contains(filepath.Base(lease.LockPath()), accountID) {
		t.Fatalf("lock path does not contain full account id: %s", lease.LockPath())
	}
	if err = os.RemoveAll(accountDir); err != nil {
		t.Fatal(err)
	}
	if _, err = Acquire(context.Background(), root, accountID, "test"); !errors.Is(err, ErrLocked) {
		t.Fatalf("expected lock contention after deleting account directory, got %v", err)
	}
}

func TestLeaseReleaseUpdatesMetadataAndAllowsReacquire(t *testing.T) {
	root := t.TempDir()
	accountID := randomAccountID(t)
	lease, err := Acquire(context.Background(), root, accountID, "v-test")
	if err != nil {
		t.Fatal(err)
	}
	lockInfoBefore, err := os.Stat(lease.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	ownerPath := strings.TrimSuffix(lease.LockPath(), ".lock") + ".owner.json"
	if err = lease.Release(); err != nil {
		t.Fatal(err)
	}
	if err = lease.Release(); err != nil {
		t.Fatalf("release must be idempotent: %v", err)
	}

	data, err := os.ReadFile(ownerPath)
	if err != nil {
		t.Fatal(err)
	}
	var owner Owner
	if err = json.Unmarshal(data, &owner); err != nil {
		t.Fatal(err)
	}
	if owner.AccountID != accountID || owner.PID != os.Getpid() || owner.State != "released" || owner.ReleasedAt == nil {
		t.Fatalf("unexpected owner metadata: %+v", owner)
	}
	lockInfoAfter, err := os.Stat(lease.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(lockInfoBefore, lockInfoAfter) {
		t.Fatal("release replaced the persistent lock file")
	}

	second, err := Acquire(context.Background(), root, accountID, "v-test")
	if err != nil {
		t.Fatal(err)
	}
	if err = second.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestLeaseReleaseIsConcurrentAndIdempotent(t *testing.T) {
	lease, err := Acquire(context.Background(), t.TempDir(), randomAccountID(t), "test")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- lease.Release()
		}()
	}
	wg.Wait()
	close(errs)
	for err = range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestIndependentLeaseScopes(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	accountA := randomAccountID(t)
	accountB := randomAccountID(t)
	first, err := Acquire(context.Background(), rootA, accountA, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	secondAccount, err := Acquire(context.Background(), rootA, accountB, "test")
	if err != nil {
		t.Fatalf("different accounts in one root should not contend: %v", err)
	}
	defer secondAccount.Release()
	secondRoot, err := Acquire(context.Background(), rootB, accountA, "test")
	if err != nil {
		t.Fatalf("same account in different roots should not contend: %v", err)
	}
	defer secondRoot.Release()
}

func TestRootSymlinkAliasesContend(t *testing.T) {
	root := t.TempDir()
	aliasParent := t.TempDir()
	alias := filepath.Join(aliasParent, "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	accountID := randomAccountID(t)
	lease, err := Acquire(context.Background(), root, accountID, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	if _, err = Acquire(context.Background(), alias, accountID, "test"); !errors.Is(err, ErrLocked) {
		t.Fatalf("symlink alias did not contend: %v", err)
	}
}

func TestOwnerMetadataIsNotAuthoritative(t *testing.T) {
	root := t.TempDir()
	accountID := randomAccountID(t)
	lease, err := Acquire(context.Background(), root, accountID, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	ownerPath := strings.TrimSuffix(lease.LockPath(), ".lock") + ".owner.json"
	if err = os.WriteFile(ownerPath, []byte("{invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Acquire(context.Background(), root, accountID, "test")
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("malformed metadata bypassed OS lock: %v", err)
	}
	var lockedErr *LockedError
	if !errors.As(err, &lockedErr) || lockedErr.Owner != nil {
		t.Fatalf("malformed metadata should be ignored: %v", err)
	}
}

func TestAcquireHonorsCallerCancellation(t *testing.T) {
	root := t.TempDir()
	accountID := randomAccountID(t)
	lease, err := Acquire(context.Background(), root, accountID, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Acquire(ctx, root, accountID, "test")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected caller cancellation, got %v", err)
	}
	if errors.Is(err, ErrLocked) {
		t.Fatalf("caller cancellation was misreported as contention: %v", err)
	}
}

func TestAcquireRejectsUnsafeAccountID(t *testing.T) {
	_, err := Acquire(context.Background(), t.TempDir(), "../../account", "test")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected unavailable lock for unsafe account id, got %v", err)
	}
}

func TestLeaseMutualExclusionAcrossProcesses(t *testing.T) {
	root := t.TempDir()
	accountID := randomAccountID(t)
	readyPath := filepath.Join(root, "ready")
	cmd := exec.Command(os.Args[0], "-test.run=^TestAccountLockHelperProcess$")
	cmd.Env = append(os.Environ(),
		helperEnv+"=1",
		helperRootEnv+"="+root,
		helperAccountEnv+"="+accountID,
		helperReadyEnv+"="+readyPath,
	)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(readyPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not acquire lease")
		}
		time.Sleep(20 * time.Millisecond)
	}

	_, err := Acquire(context.Background(), root, accountID, "parent")
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("expected cross-process contention, got %v", err)
	}
	var lockedErr *LockedError
	if !errors.As(err, &lockedErr) || lockedErr.Owner == nil {
		t.Fatalf("expected diagnostic owner metadata, got %v", err)
	}
	if lockedErr.Owner.PID != cmd.Process.Pid || lockedErr.Owner.AccountID != accountID {
		t.Fatalf("unexpected lock owner: %+v, helper pid %s", lockedErr.Owner, strconv.Itoa(cmd.Process.Pid))
	}

	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err = cmd.Wait(); err == nil {
		t.Fatal("expected killed helper to exit unsuccessfully")
	}
	reacquired, err := Acquire(context.Background(), root, accountID, "parent")
	if err != nil {
		t.Fatalf("OS did not release lease after process death: %v", err)
	}
	if err = reacquired.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountLockHelperProcess(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		t.Skip("helper process")
	}
	lease, err := Acquire(
		context.Background(),
		os.Getenv(helperRootEnv),
		os.Getenv(helperAccountEnv),
		"helper",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	if err = os.WriteFile(os.Getenv(helperReadyEnv), []byte("ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	for {
		time.Sleep(time.Hour)
	}
}
