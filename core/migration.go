package core

import (
	"fmt"
)

type migration func(a *Anytype) error

var skipMigration = func(a *Anytype) error {
	return nil
}

// ⚠️ NEVER REMOVE THE EXISTING MIGRATION FROM THE LIST, JUST REPLACE WITH skipMigration
var migrations = []migration{
	migration1,
}

func (a *Anytype) RunMigrations() error {
	version, err := a.localStore.Migrations.GetVersion()
	if err != nil {
		return err
	}

	if len(migrations) == version {
		return nil
	} else if len(migrations) < version {
		log.Errorf("repo version(%d) is higher than the total migrations number(%d)", version, len(migrations))
		return nil
	}

	for i := version; i < len(migrations); i++ {
		err := migrations[i](a)
		if err != nil {
			return fmt.Errorf("failed to execute migration %d: %s", i+1, err.Error())
		}

		err = a.localStore.Migrations.SaveVersion(i)
		if err != nil {
			log.Errorf("failed to save migrated version to db: %s", err.Error())
			return err
		}
	}

	return nil
}

func migration1(a *Anytype) error {
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

		if block.Type() != SmartBlockTypePage {
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
}
