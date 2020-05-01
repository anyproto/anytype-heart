package core

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const versionFileName = "anytype_version"

type migration func(a *Anytype) error

var skipMigration = func(a *Anytype) error {
	return nil
}

// ⚠️ NEVER REMOVE THE EXISTING MIGRATION FROM THE LIST, JUST REPLACE WITH skipMigration
var migrations = []migration{
	migration1,
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

func migration1(a *Anytype) error {
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

		log.Infof("migration1: %d pages indexed", migrated)
		return nil
	})
}
