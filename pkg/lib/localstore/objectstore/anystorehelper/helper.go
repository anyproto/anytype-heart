package anystorehelper

import (
	"context"
	"errors"
	"fmt"
	"os"

	anystore "github.com/anyproto/any-store"
	"zombiezen.com/go/sqlite"

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
	var lockFileAlreadyExists, runQuickCheck bool

	// Attempt to create the lock file atomically
	lockFile, err := os.OpenFile(lockFilePath, os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			lockFileAlreadyExists, runQuickCheck = true, true
		} else {
			return
		}
	} else {
		// todo: add pid check after a full badger deprecation
		_, _ = lockFile.Write([]byte(fmt.Sprintf("%d", os.Getpid())))
		_ = lockFile.Close()
	}

	store, err = anystore.Open(ctx, path, config)
	if err != nil {
		code := sqlite.ErrCode(err)
		l := log.With("error", err).With("code", code)
		if errors.Is(err, anystore.ErrIncompatibleVersion) || code == sqlite.ResultCorrupt || code == sqlite.ResultNotADB || code == sqlite.ResultCantOpen {
			runQuickCheck = false
			l.Errorf("failed to open anystore, reinit db")
			if err = os.RemoveAll(path); err != nil {
				return nil, lockCloseNoop, err
			}
			store, err = anystore.Open(ctx, path, config)
		} else {
			l.Errorf("failed to open anystore, non-recoverable error")
			// some other error
			return nil, lockCloseNoop, err
		}
	}
	if lockFileAlreadyExists {
		// overwrite existing lock file with the current pid
		err = os.WriteFile(lockFilePath, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
		if err != nil {
			log.Errorf("failed to write pid to existing lock file: %v", err.Error())
		}
	}

	if !runQuickCheck {
		return
	}
	// means we have not closed properly
	err = store.QuickCheck(ctx)
	if err != nil {
		// db is corrupted, close it and reinit
		err = store.Close()
		log.With("closeError", err).With("error", err).Error("quick check failed. reinit db")
		if err = os.RemoveAll(path); err != nil {
			return nil, lockCloseNoop, err
		}
		store, err = anystore.Open(ctx, path, config)
		if err != nil {
			return nil, lockCloseNoop, err
		}
	}

	return
}
