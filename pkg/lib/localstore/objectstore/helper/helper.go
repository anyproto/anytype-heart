package helper

import (
	"context"
	"errors"
	"fmt"
	"os"

	anystore "github.com/anyproto/any-store"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("objectstore.spaceindex")

func OpenDatabaseWithLockCheck(ctx context.Context, path string, config *anystore.Config) (store anystore.DB, lockClose func() error, err error) {
	lockFilePath := path + ".LOCK"
	lockClose = func() error {
		return os.RemoveAll(lockFilePath)
	}
	lockCloseNoop := func() error {
		return nil
	}
	var runQuickCheck bool

	// Attempt to create the lock file atomically
	lockFile, err := os.OpenFile(lockFilePath, os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			runQuickCheck = true
		} else {
			return
		}
	} else {
		// todo: add pid check after a full badger deprecation
		_, _ = lockFile.Write([]byte(fmt.Sprintf("%d", os.Getpid())))
		_ = lockFile.Close()
	}

	store, err = anystore.Open(ctx, path, config)
	if errors.Is(err, anystore.ErrIncompatibleVersion) {
		runQuickCheck = false
		if err = os.RemoveAll(path); err != nil {
			return nil, lockCloseNoop, err
		}
		store, err = anystore.Open(ctx, path, config)
	}
	if err != nil {
		// todo: process some possible corrupted state errors here
		return nil, lockCloseNoop, err
	}
	if runQuickCheck {
		// means we have not closed properly
		err = store.QuickCheck(ctx)
		if err != nil {
			err = store.Close()
			log.With("closeErr", err).Errorf("quick check failed: %s; reinit store", err)
			if err = os.RemoveAll(path); err != nil {
				return nil, lockCloseNoop, err
			}
			store, err = anystore.Open(ctx, path, config)
			if err != nil {
				return nil, lockCloseNoop, err
			}
		}
	}

	return
}
