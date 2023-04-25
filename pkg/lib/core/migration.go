package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	ds "github.com/ipfs/go-datastore"
	badger "github.com/ipfs/go-ds-badger"
	"github.com/textileio/go-threads/core/thread"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
)

const versionFileName = "anytype_version"

type migration func(a *Anytype, lastMigration bool) error

var skipMigration = func(a *Anytype, _ bool) error {
	return nil
}

var ErrAlreadyMigrated = fmt.Errorf("thread already migrated")

// ⚠️ NEVER REMOVE THE EXISTING MIGRATION FROM THE LIST, JUST REPLACE WITH skipMigration
var migrations = []migration{
	skipMigration,        // 1
	alterThreadsDbSchema, // 2
	skipMigration,        // 3
	skipMigration,        // 4
	snapshotToChanges,    // 5
	skipMigration,        // 6
	skipMigration,        // 7
	skipMigration,        // 8
	skipMigration,        // 9
	skipMigration,        // 10
	skipMigration,        // 11
	skipMigration,        // 12
	skipMigration,        // 13
	skipMigration,        // 14
	skipMigration,        // 15
	skipMigration,        // 16
	skipMigration,        // 17
	skipMigration,        // 18
	skipMigration,        // 19
	skipMigration,        // 20
	skipMigration,        // 21
	skipMigration,        // 22
	skipMigration,        // 23
	skipMigration,        // 24
	skipMigration,        // 25
}

func (a *Anytype) getRepoVersion() (int, error) {
	versionB, err := ioutil.ReadFile(filepath.Join(a.wallet.RepoPath(), versionFileName))
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}

	if versionB == nil {
		return 0, nil
	}

	return strconv.Atoi(strings.TrimSpace(string(versionB)))
}

func (a *Anytype) saveRepoVersion(version int) error {
	return ioutil.WriteFile(filepath.Join(a.wallet.RepoPath(), versionFileName), []byte(strconv.Itoa(version)), 0655)
}

func (a *Anytype) saveCurrentRepoVersion() error {
	return a.saveRepoVersion(len(migrations))
}

func (a *Anytype) runMigrationsUnsafe() error {
	// todo: FIXME refactoring
	if a.config.NewAccount {
		log.Debugf("new account")
		return a.saveCurrentRepoVersion()
	}

	version, err := a.getRepoVersion()
	if err != nil {
		return err
	}

	if len(migrations) == version {
		return nil
	} else if len(migrations) < version {
		log.Errorf("repo version(%d) is higher than the total migrations number(%d)", version, len(migrations))
		return nil
	}

	log.Errorf("migrating from %d to %d", version, len(migrations))

	for i := version; i < len(migrations); i++ {
		err := migrations[i](a, i == len(migrations)-1)
		if err != nil {
			return fmt.Errorf("failed to execute migration %d: %s", i+1, err.Error())
		}

		err = a.saveRepoVersion(i + 1)
		if err != nil {
			log.Errorf("failed to save migrated version to file: %s", err.Error())
			return err
		}
	}

	return nil
}

func (a *Anytype) RunMigrations() error {
	var err error
	a.migrationOnce.Do(func() {
		err = a.runMigrationsUnsafe()
	})

	return err
}

func doWithRunningNode(a *Anytype, offline bool, stopAfter bool, f func() error) error {
	// FIXME: refactor offline migration

	var err error

	err = f()
	if err != nil {
		return err
	}
	return nil
}

func (a *Anytype) migratePageToChanges(id thread.ID) error {
	return nil
}

func runSnapshotToChangesMigration(a *Anytype) error {
	return nil
}

func snapshotToChanges(a *Anytype, lastMigration bool) error {
	return doWithRunningNode(a, false, !lastMigration, func() error {
		return runSnapshotToChangesMigration(a)
	})
}

func alterThreadsDbSchema(a *Anytype, _ bool) error {
	// FIXME: refactor
	path := filepath.Join(a.wallet.RepoPath(), "collections", "eventstore")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Info("migration alterThreadsDbSchema skipped because collections db not yet created")
		return nil
	}

	db, err := badger.NewDatastore(path, &badger.DefaultOptions)
	if err != nil {
		return err
	}
	defer func() {
		err := db.Close()
		if err != nil {
			log.Errorf("failed to close db: %s", err.Error())
		}
	}()

	dsDBPrefix := ds.NewKey("/db")
	dsDBSchemas := dsDBPrefix.ChildString("schema")

	key := dsDBSchemas.ChildString(threads.ThreadInfoCollectionName)
	exists, err := db.Has(context.Background(), key)
	if !exists {
		log.Info("migration alterThreadsDbSchema skipped because schema not exists in the collections db")
		return nil
	}

	schemaBytes, err := json.Marshal(threads.ThreadInfoCollectionName)
	if err != nil {
		return err
	}
	if err := db.Put(context.Background(), key, schemaBytes); err != nil {
		return err
	}

	log.Infof("migration alterThreadsDbSchema: schema updated")

	return nil
}
