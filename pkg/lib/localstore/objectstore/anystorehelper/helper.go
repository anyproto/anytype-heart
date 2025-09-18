package anystorehelper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

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
			if err != nil {
				l.Errorf("failed to open anystore again, %s", err)
				return nil, lockCloseNoop, err
			}
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
	start := time.Now()
	err = store.QuickCheck(ctx)
	if err != nil {
		// db is corrupted, close it and reinit
		closeErr := store.Close()
		log.With("closeError", closeErr).With("error", err).Error("quick check failed. reinit db")
		if err = os.RemoveAll(path); err != nil {
			return nil, lockCloseNoop, err
		}
		store, err = anystore.Open(ctx, path, config)
		if err != nil {
			return nil, lockCloseNoop, err
		}
	} else {
		spentMs := time.Since(start).Milliseconds()
		if spentMs < 1000 {
			log.With("spent", spentMs).Infof("quick check succeed")
		} else {
			log.With("spent", spentMs).Warn("quick check succeed")
		}
	}

	return
}

func AddIndexes(ctx context.Context, coll anystore.Collection, indexes []anystore.IndexInfo) error {
	gotIndexes := coll.GetIndexes()
	toCreate := indexes[:0]
	var toDrop []string
	for i, idx := range indexes {
		if idx.Name == "" {
			idx.Name = strings.Join(idx.Fields, ",")
			indexes[i].Name = idx.Name
		}
		if !slices.ContainsFunc(gotIndexes, func(i anystore.Index) bool {
			return i.Info().Name == idx.Name
		}) {
			toCreate = append(toCreate, idx)
		}
	}
	for _, idx := range gotIndexes {
		if !slices.ContainsFunc(indexes, func(i anystore.IndexInfo) bool {
			return i.Name == idx.Info().Name
		}) {
			toDrop = append(toDrop, idx.Info().Name)
		}
	}
	if len(toDrop) > 0 {
		for _, indexName := range toDrop {
			if err := coll.DropIndex(ctx, indexName); err != nil {
				return err
			}
		}
	}
	if len(toCreate) > 0 {
		coll.GetIndexes()
		return coll.EnsureIndex(ctx, toCreate...)
	}
	return nil
}
