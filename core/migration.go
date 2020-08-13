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

	"github.com/anytypeio/go-anytype-library/vclock"
	ds "github.com/ipfs/go-datastore"
	badger "github.com/ipfs/go-ds-badger"
	"github.com/textileio/go-threads/core/thread"
	"go.uber.org/zap"
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
}

func (a *Anytype) getRepoVersion() (int, error) {
	versionB, err := ioutil.ReadFile(filepath.Join(a.opts.Repo, versionFileName))
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}

	if versionB == nil {
		return 0, nil
	}

	return strconv.Atoi(strings.TrimSpace(string(versionB)))
}

func (a *Anytype) saveRepoVersion(version int) error {
	return ioutil.WriteFile(filepath.Join(a.opts.Repo, versionFileName), []byte(strconv.Itoa(version)), 0655)
}

func (a *Anytype) saveCurrentRepoVersion() error {
	return a.saveRepoVersion(len(migrations))
}

func (a *Anytype) runMigrationsUnsafe() error {
	if _, err := os.Stat(filepath.Join(a.opts.Repo, "ipfslite")); os.IsNotExist(err) {
		log.Debugf("repo is not inited, save all migrations as done")
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

	log.Debugf("migrating from %d to %d", version, len(migrations))

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
	offlineWas := a.opts.Offline
	defer func() {
		a.opts.Offline = offlineWas
	}()

	a.opts.Offline = offline
	err := a.start()
	if err != nil {
		return err
	}

	if stopAfter {
		defer func() {
			err = a.Stop()
			if err != nil {
				log.Errorf("migration failed to stop the running node: %s", err.Error())
			}
			a.lock.Lock()
			defer a.lock.Unlock()
			// @todo: possible race condition here. These chans not assumed to be replaced
			a.shutdownStartsCh = make(chan struct{})
			a.onlineCh = make(chan struct{})
		}()
	}

	err = f()
	if err != nil {
		return err
	}
	return nil
}

func (a *Anytype) migratePageToChanges(id thread.ID) error {
	snapshotsPB, err := a.snapshotTraverseLogs(context.TODO(), id, vclock.Undef, 1)
	if err != nil {
		if err == ErrFailedToDecodeSnapshot {
			// already migrated
			return ErrAlreadyMigrated
		}

		return fmt.Errorf("snapshotToChanges failed to get sb last snapshot %s: %s", id.String(), err.Error())
	}

	if len(snapshotsPB) == 0 {
		return fmt.Errorf("snapshotToChanges no snapshots found for %s", id.String())
	}

	snap := snapshotsPB[0]
	var keys []*FileKeys
	for fileHash, fileKeys := range snap.KeysByHash {
		keys = append(keys, &FileKeys{
			Hash: fileHash,
			Keys: fileKeys.KeysByPath,
		})
	}
	var detailsFileFields = [...]string{"coverId", "iconImage"}

	if snap.Details != nil && snap.Details.Fields != nil {
		for _, fileField := range detailsFileFields {
			if v, exists := snap.Details.Fields[fileField]; exists {
				hash := v.GetStringValue()
				keysForFile, err := a.FileGetKeys(hash)
				if err != nil {
					log.With(zap.String("hash", hash)).Error("failed to get file key", err.Error())
				} else {
					keys = append(keys, keysForFile)
				}
			}
		}
	}

	record := a.opts.SnapshotMarshalerFunc(snap.Blocks, snap.Details, keys)
	sb, _ := a.GetSmartBlock(id.String())

	log.Debugf("migratePageToChanges %s", id.String())
	_, err = sb.PushRecord(record)
	return err
}

func runSnapshotToChangesMigration(a *Anytype) error {
	threadsIDs, err := a.t.Logstore().Threads()
	if err != nil {
		return err
	}

	threadsIDs = append(threadsIDs)
	migrated := 0
	for _, threadID := range threadsIDs {
		err = a.migratePageToChanges(threadID)
		if err != nil {
			log.Errorf(err.Error())
		} else {
			migrated++
		}
	}

	log.Infof("migration snapshotToChanges: %d pages migrated", migrated)
	return nil
}

func snapshotToChanges(a *Anytype, lastMigration bool) error {
	return doWithRunningNode(a, false, !lastMigration, func() error {
		return runSnapshotToChangesMigration(a)
	})
}

func alterThreadsDbSchema(a *Anytype, _ bool) error {
	path := filepath.Join(a.opts.Repo, "collections", "eventstore")
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

	key := dsDBSchemas.ChildString(threadInfoCollection.Name)
	exists, err := db.Has(key)
	if !exists {
		log.Info("migration alterThreadsDbSchema skipped because schema not exists in the collections db")
		return nil
	}

	schemaBytes, err := json.Marshal(threadInfoCollection.Schema)
	if err != nil {
		return err
	}
	if err := db.Put(key, schemaBytes); err != nil {
		return err
	}

	log.Infof("migration alterThreadsDbSchema: schema updated")

	return nil
}
