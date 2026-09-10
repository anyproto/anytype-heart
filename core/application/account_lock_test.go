package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/application/accountdirlock"
	"github.com/anyproto/anytype-heart/pb"
)

func randomAccountID(t *testing.T) string {
	t.Helper()
	_, publicKey, err := crypto.GenerateRandomEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	return publicKey.Account()
}

func TestSwitchAccountLeaseKeepsCurrentLeaseWhenTargetIsLocked(t *testing.T) {
	root := t.TempDir()
	currentID := randomAccountID(t)
	targetID := randomAccountID(t)
	service := New()
	if err := service.acquireAccountLease(context.Background(), root, currentID); err != nil {
		t.Fatal(err)
	}
	defer service.releaseAccountLease()

	targetLease, err := accountdirlock.Acquire(context.Background(), root, targetID, "other-process")
	if err != nil {
		t.Fatal(err)
	}
	defer targetLease.Release()

	err = service.switchAccountLease(context.Background(), root, targetID)
	if !errors.Is(err, ErrAnotherProcessIsRunning) {
		t.Fatalf("expected target contention, got %v", err)
	}
	if service.accountLease == nil || !service.accountLease.Matches(root, currentID) {
		t.Fatal("current account lease was not retained")
	}
	if _, err = accountdirlock.Acquire(context.Background(), root, currentID, "contender"); !errors.Is(err, accountdirlock.ErrLocked) {
		t.Fatalf("current account lease is no longer held: %v", err)
	}
}

func TestStopReleasesAccountLease(t *testing.T) {
	root := t.TempDir()
	accountID := randomAccountID(t)
	service := New()
	if err := service.acquireAccountLease(context.Background(), root, accountID); err != nil {
		t.Fatal(err)
	}
	if err := service.stop(); err != nil {
		t.Fatal(err)
	}
	lease, err := accountdirlock.Acquire(context.Background(), root, accountID, "next-process")
	if err != nil {
		t.Fatalf("lease was not released by stop: %v", err)
	}
	if err = lease.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountSelectRejectsIDThatDoesNotMatchDerivedWallet(t *testing.T) {
	service := New()
	derivedPrivateKey, _, err := crypto.GenerateRandomEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	otherID := randomAccountID(t)
	service.derivedKeys = &crypto.DerivationResult{Identity: derivedPrivateKey}

	_, err = service.AccountSelect(context.Background(), &pb.RpcAccountSelectRequest{
		Id:       otherID,
		RootPath: t.TempDir(),
	})
	if !errors.Is(err, ErrBadInput) {
		t.Fatalf("expected mismatched ID to be bad input, got %v", err)
	}
}

func TestAccountSelectValidatesIDBeforeConstructingPaths(t *testing.T) {
	root := t.TempDir()
	unsafePath := filepath.Clean(filepath.Join(root, "..", "unsafe-account-dir"))
	if _, err := os.Stat(unsafePath); !os.IsNotExist(err) {
		t.Fatalf("unsafe path unexpectedly exists before test: %v", err)
	}
	service := New()
	_, err := service.AccountSelect(context.Background(), &pb.RpcAccountSelectRequest{
		Id:       "../unsafe-account-dir",
		RootPath: root,
	})
	if !errors.Is(err, ErrBadInput) {
		t.Fatalf("expected malformed ID to be bad input, got %v", err)
	}
	if _, statErr := os.Stat(unsafePath); !os.IsNotExist(statErr) {
		t.Fatalf("unsafe path was touched before ID validation: %v", statErr)
	}
}

func TestMigrationCacheIncludesCanonicalRoot(t *testing.T) {
	service := New()
	accountID := randomAccountID(t)
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	first, err := service.migrationManager.getOrCreateMigration(firstRoot, accountID, "en")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.migrationManager.getOrCreateMigration(secondRoot, accountID, "en")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("migration cache reused an entry from another root")
	}
	firstCanonical, err := accountdirlock.CanonicalRootPath(firstRoot)
	if err != nil {
		t.Fatal(err)
	}
	secondCanonical, err := accountdirlock.CanonicalRootPath(secondRoot)
	if err != nil {
		t.Fatal(err)
	}
	if first.rootPath != firstCanonical || second.rootPath != secondCanonical {
		t.Fatalf("migration roots not preserved: %q, %q", first.rootPath, second.rootPath)
	}
}

func TestAccountSelectDoesNotClassifyRootFailureAsBadInput(t *testing.T) {
	service := New()
	derivedPrivateKey, _, err := crypto.GenerateRandomEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	service.derivedKeys = &crypto.DerivationResult{Identity: derivedPrivateKey}
	_, err = service.AccountSelect(context.Background(), &pb.RpcAccountSelectRequest{
		Id: derivedPrivateKey.GetPublic().Account(),
	})
	if !errors.Is(err, accountdirlock.ErrUnavailable) || !errors.Is(err, ErrFailedToStartApplication) {
		t.Fatalf("expected unavailable/start failure, got %v", err)
	}
	if errors.Is(err, ErrBadInput) {
		t.Fatalf("root filesystem failure was misreported as bad input: %v", err)
	}
}
