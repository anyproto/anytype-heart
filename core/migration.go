package core

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	ds "github.com/ipfs/go-datastore"
	badger "github.com/ipfs/go-ds-badger"
)

const versionFileName = "anytype_version"

type migration func(a *Anytype) error

var skipMigration = func(a *Anytype) error {
	return nil
}

// ⚠️ NEVER REMOVE THE EXISTING MIGRATION FROM THE LIST, JUST REPLACE WITH skipMigration
var migrations = []migration{
	indexLinks,           // 1
	alterThreadsDbSchema, // 2
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
		err := migrations[i](a)
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

func doWithOfflineNode(a *Anytype, f func() error) error {
	offlineWas := a.opts.Offline
	defer func() {
		a.opts.Offline = offlineWas
	}()

	a.opts.Offline = true
	err := a.start()
	if err != nil {
		return err
	}

	defer func() {
		err = a.Stop()
		if err != nil {
			log.Errorf("migration failed to stop the offline node node: %s", err.Error())
		}
	}()

	err = f()
	if err != nil {
		return err
	}
	return nil
}

func indexLinks(a *Anytype) error {
	return doWithOfflineNode(a, func() error {
		threadsIDs, err := a.t.Logstore().Threads()
		if err != nil {
			return err
		}

		migrated := 0
		for _, threadID := range threadsIDs {
			block, err := a.GetSmartBlock(threadID.String())
			if err != nil {
				log.Errorf("failed to get smartblock %s: %s", threadID.String(), err.Error())
				continue
			}

			err = block.index()
			if err != nil {
				log.Errorf("failed to index page %s: %s", threadID.String(), err.Error())
				continue
			}
			migrated++
		}

		log.Infof("migration indexLinks: %d pages indexed", migrated)
		return nil
	})
}

func alterThreadsDbSchema(a *Anytype) error {
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
