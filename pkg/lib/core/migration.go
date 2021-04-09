package core

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/vclock"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
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
	skipMigration,                     // 1
	alterThreadsDbSchema,              // 2
	skipMigration,                     // 3
	skipMigration,                     // 4
	snapshotToChanges,                 // 5
	skipMigration,                     // 6
	addFilesMetaHash,                  // 7
	skipMigration,                     // 8
	skipMigration,                     // 9
	skipMigration,                     // 10
	addMissingLayout,                  // 11
	addFilesToObjects,                 // 12
	removeBundleRelationsFromDs,       // 13
	skipMigration,                     // 14
	skipMigration,                     // 15
	skipMigration,                     // 16
	skipMigration,                     // 17
	skipMigration,                     // 18
	skipMigration,                     // 19
	skipMigration,                     // 20
	skipMigration,                     // 21
	skipMigration,                     // 22
	skipMigration,                     // 23
	reindexAll,                        // 24
	removeIncorrectlyIndexedRelations, // 25
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

	/*offlineWas := a.config.Offline
	defer func() {
		a.opts.Offline = offlineWas
	}()

	a.opts.Offline = offline*/
	var err error
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

		return fmt.Errorf("failed to get sb last snapshot: %s", err.Error())
	}

	if len(snapshotsPB) == 0 {
		return fmt.Errorf("no records found for the thread")
	}

	snap := snapshotsPB[0]
	var keys []*files.FileKeys
	for fileHash, fileKeys := range snap.KeysByHash {
		keys = append(keys, &files.FileKeys{
			Hash: fileHash,
			Keys: fileKeys.KeysByPath,
		})
	}
	var detailsFileFields = [...]string{"coverId", "iconImage"}

	if snap.Details != nil && snap.Details.Fields != nil {
		for _, fileField := range detailsFileFields {
			if v, exists := snap.Details.Fields[fileField]; exists {
				hash := v.GetStringValue()
				keysForFile, err := a.files.FileGetKeys(hash)
				if err != nil {
					log.With(zap.String("hash", hash)).Error("failed to get file key", err.Error())
				} else {
					keys = append(keys, keysForFile)
				}
			}
		}
	}

	record := a.opts.SnapshotMarshalerFunc(snap.Blocks, snap.Details, nil, nil, keys)
	sb, err := a.GetSmartBlock(id.String())

	log.With("thread", id.String()).Debugf("thread migrated")
	_, err = sb.PushRecord(record)
	return err
}

func runSnapshotToChangesMigration(a *Anytype) error {
	threadsIDs, err := a.threadService.Logstore().Threads()
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
	exists, err := db.Has(key)
	if !exists {
		log.Info("migration alterThreadsDbSchema skipped because schema not exists in the collections db")
		return nil
	}

	schemaBytes, err := json.Marshal(threads.ThreadInfoCollectionName)
	if err != nil {
		return err
	}
	if err := db.Put(key, schemaBytes); err != nil {
		return err
	}

	log.Infof("migration alterThreadsDbSchema: schema updated")

	return nil
}

func addFilesMetaHash(a *Anytype, lastMigration bool) error {
	// todo: better split into 2 migrations
	return doWithRunningNode(a, true, !lastMigration, func() error {
		files, err := a.fileStore.List()
		if err != nil {
			return err
		}
		var (
			ctx       context.Context
			cancel    context.CancelFunc
			toMigrate int
			migrated  int
		)
		for _, file := range files {
			if file.MetaHash == "" {
				toMigrate++
				for _, target := range file.Targets {
					ctx, cancel = context.WithTimeout(context.Background(), time.Second)
					// reindex file to add metaHash
					_, err = a.files.FileIndexInfo(ctx, target, true)
					if err != nil {
						log.Errorf("FileIndexInfo error: %s", err.Error())
					} else {
						migrated++
					}
					cancel()
				}
			}
		}
		if migrated != toMigrate {
			log.Errorf("addFilesMetaHash migration not completed for all files: %d/%d completed", migrated, toMigrate)
		} else {
			log.Debugf("addFilesMetaHash migration completed for %d files", migrated)
		}
		return nil
	})
}

func addFilesToObjects(a *Anytype, lastMigration bool) error {
	// todo: better split into 2 migrations
	return doWithRunningNode(a, true, !lastMigration, func() error {
		files, err := a.fileStore.List()
		if err != nil {
			return err
		}
		targetsProceed := map[string]struct{}{}
		imgObjType := bundle.MustGetType(bundle.TypeKeyImage)
		fileObjType := bundle.MustGetType(bundle.TypeKeyFile)
		log.Debugf("migrating %d files", len(files))
		var (
			ctx      context.Context
			cancel   context.CancelFunc
			migrated int
		)

		for _, file := range files {
			if file.Mill == "/image/resize" {
				if len(file.Targets) == 0 {
					log.Errorf("addFilesToObjects migration: got image with empty targets list")
					continue
				}

				for _, target := range file.Targets {
					if _, exists := targetsProceed[target]; exists {
						continue
					}
					targetsProceed[target] = struct{}{}
					ctx, cancel = context.WithTimeout(context.Background(), time.Second*3)
					img, err := a.ImageByHash(ctx, target)
					if err != nil {
						log.Errorf("addFilesToObjects migration: ImageByHash failed: %s", err.Error())
						cancel()
						continue
					}

					details, err := img.Details()
					if err != nil {
						log.Errorf("addFilesToObjects migration: img.Details() failed: %s", err.Error())
						cancel()
						continue
					}

					err = a.objectStore.UpdateObjectDetails(img.Hash(), details, &pbrelation.Relations{Relations: imgObjType.Relations})
					if err != nil {
						// this shouldn't fail
						cancel()
						return err
					}
					migrated++
					cancel()
				}

			} else if file.Mill == "/blob" {
				if len(file.Targets) == 0 {
					return fmt.Errorf("got file with empty targets list")
				}
				for _, target := range file.Targets {
					if _, exists := targetsProceed[target]; exists {
						continue
					}
					targetsProceed[target] = struct{}{}
					ctx, cancel = context.WithTimeout(context.Background(), time.Second*3)

					file, err := a.FileByHash(ctx, target)
					if err != nil {
						log.Errorf("addFilesToObjects migration: FileByHash failed: %s", err.Error())
						cancel()
						continue
					}

					details, err := file.Details()
					if err != nil {
						log.Errorf("failed to fetch details for file %s: %w", file.Hash(), err)
						cancel()
						continue
					}

					err = a.objectStore.UpdateObjectDetails(file.Hash(), details, &pbrelation.Relations{Relations: fileObjType.Relations})
					if err != nil {
						cancel()
						return err
					}
					cancel()
					migrated++
				}
			}
		}
		if migrated != len(files) {
			log.Errorf("addFilesToObjects migration not completed for all files: %d/%d completed", migrated, len(files))
		} else {
			log.Debugf("addFilesToObjects migration completed for %d files", migrated)
		}
		return nil
	})
}

func removeBundleRelationsFromDs(a *Anytype, lastMigration bool) error {
	return doWithRunningNode(a, true, !lastMigration, func() error {
		keys := bundle.ListRelationsKeys()
		var migrated int
		for _, key := range keys {
			err := a.objectStore.RemoveRelationFromCache(key.String())
			if err != nil {
				continue
			}

			migrated++
		}

		log.Debugf("migration removeBundleRelationsFromDs completed for %d relations", migrated)

		return nil
	})
}

func ReindexAll(a *Anytype) (int, error) {
	ds, err := a.ds.LocalstoreDS()
	if err != nil {
		return 0, err
	}

	ids, err := a.objectStore.ListIds()
	if err != nil {
		return 0, err
	}
	total := len(ids)
	var migrated int
	for _, id := range ids {
		sbt, err := smartblock.SmartBlockTypeFromID(id)
		if err != nil {
			return 0, fmt.Errorf("migration reindexAll:failed to extract smartblock type: %w", err)
		}
		if sbt == smartblock.SmartBlockTypeArchive {
			// remove archive we have accidentally indexed
			err = a.objectStore.DeleteObject(id)
			if err != nil {
				log.Errorf("migration reindexAll: failed to delete archive from index: %s", err.Error())
			}
			total--
			continue
		}
		for _, idx := range a.objectStore.Indexes() {
			//if idx.Name == "objtype_relkey_setid" {
			// skip it because we can't reindex relations in sets for now
			//	continue
			//}

			err = localstore.EraseIndex(idx, ds)
			if err != nil {
				log.Errorf("migration reindexAll: failed to delete archive from index: %s", err.Error())
			}
		}
		oi, err := a.objectStore.GetByIDs(id)
		if err != nil {
			log.Errorf("migration reindexAll: failed to get objects by id: %s", err.Error())
			continue
		}

		if len(oi) < 1 {
			log.Errorf("migration reindexAll: failed to get objects: not found")
			continue
		}
		o := oi[0]
		var objType string
		if pbtypes.HasField(o.Details, bundle.RelationKeyType.String()) {
			objTypes := pbtypes.GetStringList(o.Details, bundle.RelationKeyType.String())
			if len(objTypes) > 0 {
				objType = objTypes[0]
			}
		}

		if strings.HasPrefix(objType, addr.OldCustomObjectTypeURLPrefix) {
			objType = strings.TrimPrefix(objType, addr.OldCustomObjectTypeURLPrefix)
		} else if strings.HasPrefix(objType, addr.OldBundledObjectTypeURLPrefix) {
			objType = addr.BundledObjectTypeURLPrefix + strings.TrimPrefix(objType, addr.OldBundledObjectTypeURLPrefix)
		} else if bundle.HasObjectType(objType) {
			objType = addr.BundledObjectTypeURLPrefix + objType
		}

		if sbt == smartblock.SmartBlockTypeIndexedRelation {
			err = a.objectStore.DeleteObject(id)
			if err != nil {
				log.Errorf("deletion of indexed relation failed: %s", err.Error())
			}
			// will be reindexed below
			continue
		}

		o.Details.Fields[bundle.RelationKeyType.String()] = pbtypes.String(objType)
		err = a.objectStore.CreateObject(id, o.Details, o.Relations, nil, o.Snippet)
		if err != nil {
			log.Errorf("migration reindexAll: createObject failed: %s", err.Error())
			continue
		}
		migrated++
	}
	relations, _ := a.objectStore.ListRelations("")
	for _, rel := range relations {
		if bundle.HasRelation(rel.Key) {
			rel.Creator = a.ProfileID()
		} else {
			rel.Creator = addr.AnytypeProfileId
		}
	}
	var indexedRelations int
	var divided [][]*pbrelation.Relation
	chunkSize := 30
	for i := 0; i < len(relations); i += chunkSize {
		end := i + chunkSize

		if end > len(relations) {
			end = len(relations)
		}

		divided = append(divided, relations[i:end])
	}
	for _, chunk := range divided {
		err = a.objectStore.StoreRelations(chunk)
		if err != nil {
			log.Errorf("reindex relations failed: %s", err.Error())
		} else {
			indexedRelations += len(chunk)
		}
	}

	log.Debugf("%d relations reindexed", indexedRelations)
	migrated += indexedRelations

	if migrated != total {
		log.Errorf("migration reindexAll: %d/%d completed", migrated, len(ids))
	} else {
		log.Debugf("migration reindexAll completed for %d objects", migrated)
	}
	return migrated, nil
}

func removeIncorrectlyIndexedRelations(a *Anytype, lastMigration bool) error {
	return doWithRunningNode(a, true, !lastMigration, func() error {
		var err error
		for _, rk := range bundle.ListRelationsKeys() {
			// remove accidentally indexed bundled relations with custom relation prefix
			err = a.objectStore.DeleteObject(addr.CustomRelationURLPrefix + rk.String())
			if err != nil {
				log.Errorf("migration reindexAll: failed to delete archive from index: %s", err.Error())
			}
		}
		return nil
	})
}

func reindexAll(a *Anytype, lastMigration bool) error {
	return doWithRunningNode(a, true, !lastMigration, func() error {
		_, err := ReindexAll(a)
		return err
	})
}

func reindexStoredRelations(a *Anytype, lastMigration bool) error {
	return doWithRunningNode(a, true, !lastMigration, func() error {
		rels, err := a.objectStore.ListRelations("")
		if err != nil {
			return err
		}
		migrate := func(old string) (new string, hasChanges bool) {
			if strings.HasPrefix(old, addr.OldCustomObjectTypeURLPrefix) {
				new = strings.TrimPrefix(old, addr.OldCustomObjectTypeURLPrefix)
				hasChanges = true
			} else if strings.HasPrefix(old, addr.OldBundledObjectTypeURLPrefix) {
				new = addr.BundledObjectTypeURLPrefix + strings.TrimPrefix(old, addr.OldBundledObjectTypeURLPrefix)
				hasChanges = true
			} else {
				new = old
			}
			return
		}

		for i, rel := range rels {
			if len(rel.ObjectTypes) == 0 {
				continue
			}
			var newOts []string
			var hasChanges2 bool
			for _, ot := range rel.ObjectTypes {
				newOt, hasChanges1 := migrate(ot)
				hasChanges2 = hasChanges2 || hasChanges1
				newOts = append(newOts, newOt)
			}

			if hasChanges2 {
				rels[i].ObjectTypes = newOts
			}
		}

		return a.objectStore.StoreRelations(rels)
	})
}

func addMissingLayout(a *Anytype, lastMigration bool) error {
	return doWithRunningNode(a, true, !lastMigration, func() error {
		ids, err := a.objectStore.ListIds()
		if err != nil {
			return err
		}
		total := len(ids)
		var migrated int
		for _, id := range ids {
			oi, err := a.objectStore.GetByIDs(id)
			if err != nil {
				log.Errorf("migration addMissingLayout: failed to get objects by id: %s", err.Error())
				continue
			}
			if len(oi) < 1 {
				log.Errorf("migration addMissingLayout: failed to get objects: not found")
				continue
			}
			o := oi[0]
			if o.Details == nil || o.Details.Fields == nil {
				o.Details = &types.Struct{Fields: make(map[string]*types.Value)}
			}

			if pbtypes.Exists(o.Details, bundle.RelationKeyLayout.String()) {
				continue
			}

			var ot []string
			if t, exists := o.Details.Fields[bundle.RelationKeyType.String()]; exists {
				ot = pbtypes.GetStringListValue(t)
			}
			if len(ot) == 0 {
				ot = []string{bundle.TypeKeyPage.URL()}
			}

			var layout pbrelation.ObjectTypeLayout
			otUrl := ot[len(ot)-1]
			if strings.HasPrefix(otUrl, bundle.TypePrefix) {
				t, err := bundle.GetTypeByUrl(otUrl)
				if err != nil {
					log.Errorf("migration addMissingLayout: failed to get bundled type '%s': %s", otUrl, err.Error())
					layout = pbrelation.ObjectType_basic
				} else {
					layout = t.Layout
				}
			} else {
				oi, err := a.objectStore.GetByIDs(otUrl)
				if err != nil {
					log.Errorf("migration addMissingLayout: failed to get objects type by id: %s", err.Error())
					continue
				} else if len(oi) == 0 {
					log.Errorf("migration addMissingLayout: failed to get custom type '%s'", otUrl)
					layout = pbrelation.ObjectType_basic
				} else {
					if exists := pbtypes.Exists(oi[0].Details, bundle.RelationKeyLayout.String()); exists {
						layout = pbrelation.ObjectTypeLayout(int32(pbtypes.GetFloat64(oi[0].Details, bundle.RelationKeyLayout.String())))
					} else {
						layout = pbrelation.ObjectType_basic
					}
				}
			}

			o.Details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(layout))
			err = a.objectStore.UpdateObjectDetails(id, o.Details, o.Relations)
			if err != nil {
				log.Errorf("migration addMissingLayout: failed to UpdateObject: %s", err.Error())
				continue
			}
			migrated++
		}

		if migrated != total {
			log.Errorf("migration addMissingLayout: %d/%d completed", migrated, len(ids))
		} else {
			log.Debugf("migration addMissingLayout completed for %d objects", migrated)
		}
		return nil
	})
}
