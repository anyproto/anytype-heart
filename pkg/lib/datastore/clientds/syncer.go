package clientds

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// SyncDbAfterInactivity shows the minimum time after db was changed to call db.Sync
// regular Db.Sync will help to decrease the chance of data loss in case of power loss/bsod
// while this logic decrease the chance some db writer will need to wait for sync to finish
var SyncDbAfterInactivity = time.Second * 60

// SyncDbAfterVersions is the fallback mechanism in case there are some active write is happening so SyncDbAfterInactivity mechanism is not triggered
// usually change in the object produces 1 max in the localstore/spacestore db
// experimentally we found that 1 min of active writing produces about 150 versions in each of localstore and spacestore dbs
// so we set this value of 500 will make sure that in case of non-stop writing we will sync db at least once per 3 min
var SyncDbAfterVersions = 500

type dbSyncer struct {
	LastMaxVersion       uint64
	LastMaxVersionSynced uint64
	db                   *badger.DB
}

func (d *dbSyncer) name() string {
	return filepath.Base(d.db.Opts().Dir)
}

func (d *dbSyncer) info() string {
	return fmt.Sprintf("%s; lastMax: %d; lastSynced: %d;", d.name(), d.LastMaxVersion, d.LastMaxVersionSynced)
}

func (d *dbSyncer) sync(maxVersion uint64) {
	err := d.db.Sync()
	if err != nil {
		log.Errorf("failed to sync db %s at version %d: %s", d.info(), maxVersion, err)
	} else {
		log.Debugf("db synced %s at version %d", d.info(), maxVersion)
		d.LastMaxVersionSynced = maxVersion
	}
}

func newDbSyncer(db *badger.DB) *dbSyncer {
	d := &dbSyncer{
		db: db,
	}
	d.LastMaxVersion = d.db.MaxVersion()
	// hack to force sync on start after some inactivity
	d.LastMaxVersionSynced = d.LastMaxVersion - 1
	return d
}

func (r *clientds) syncer() error {
	var syncers []*dbSyncer
	if r.spaceDS != nil {
		syncers = append(syncers, newDbSyncer(r.spaceDS))
	}
	if r.localstoreDS != nil {
		syncers = append(syncers, newDbSyncer(r.localstoreDS))
	}

	for {
		select {
		case <-r.closed:
			return nil
		case <-time.After(SyncDbAfterInactivity):
			for _, syncer := range syncers {
				maxVersion := syncer.db.MaxVersion()
				if syncer.LastMaxVersionSynced == maxVersion {
					continue
				}

				var skip = true
				if syncer.LastMaxVersion == maxVersion {
					skip = false
				} else if syncer.LastMaxVersionSynced+uint64(SyncDbAfterVersions) < maxVersion {
					// todo: write local metrics on it to test in case of cold account recovery
					skip = false
				}

				syncer.LastMaxVersion = maxVersion
				if skip {
					continue
				}
				syncer.sync(maxVersion)
			}
		}
	}

}
