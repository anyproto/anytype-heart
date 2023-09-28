package indexer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/dgraph-io/badger/v3"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	smartblock2 "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (

	// ForceObjectsReindexCounter reindex thread-based objects
	ForceObjectsReindexCounter int32 = 8

	// ForceFilesReindexCounter reindex ipfs-file-based objects
	ForceFilesReindexCounter int32 = 11 //

	// ForceBundledObjectsReindexCounter reindex objects like anytypeProfile
	ForceBundledObjectsReindexCounter int32 = 5 // reindex objects like anytypeProfile

	// ForceIdxRebuildCounter erases localstore indexes and reindex all type of objects
	// (no need to increase ForceObjectsReindexCounter & ForceFilesReindexCounter)
	ForceIdxRebuildCounter int32 = 52

	// ForceFulltextIndexCounter  performs fulltext indexing for all type of objects (useful when we change fulltext config)
	ForceFulltextIndexCounter int32 = 5

	// ForceFilestoreKeysReindexCounter reindex filestore keys in all objects
	ForceFilestoreKeysReindexCounter int32 = 2
)

func (i *indexer) reindexIfNeeded() error {
	checksums, err := i.store.GetChecksums()
	if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
		return err
	}
	if checksums == nil {
		checksums = &model.ObjectStoreChecksums{
			// do no add bundled relations checksums, because we want to index them for new accounts
			ObjectsForceReindexCounter:       ForceObjectsReindexCounter,
			FilesForceReindexCounter:         ForceFilesReindexCounter,
			IdxRebuildCounter:                ForceIdxRebuildCounter,
			FilestoreKeysForceReindexCounter: ForceFilestoreKeysReindexCounter,
			FulltextRebuild:                  ForceFulltextIndexCounter,
			BundledObjects:                   ForceBundledObjectsReindexCounter,
		}
	}

	var flags reindexFlags
	if checksums.BundledRelations != bundle.RelationChecksum {
		flags.bundledRelations = true
	}
	if checksums.BundledObjectTypes != bundle.TypeChecksum {
		flags.bundledTypes = true
	}
	if checksums.ObjectsForceReindexCounter != ForceObjectsReindexCounter {
		flags.objects = true
	}
	if checksums.FilestoreKeysForceReindexCounter != ForceFilestoreKeysReindexCounter {
		flags.fileKeys = true
	}
	if checksums.FilesForceReindexCounter != ForceFilesReindexCounter {
		flags.fileObjects = true
	}
	if checksums.FulltextRebuild != ForceFulltextIndexCounter {
		flags.fulltext = true
	}
	if checksums.BundledTemplates != i.btHash.Hash() {
		flags.bundledTemplates = true
	}
	if checksums.BundledObjects != ForceBundledObjectsReindexCounter {
		flags.bundledObjects = true
	}
	if checksums.IdxRebuildCounter != ForceIdxRebuildCounter {
		flags.enableAll()
	}

	return i.reindex(flags)
}

func (i *indexer) reindex(flags reindexFlags) (err error) {
	if flags.any() {
		log.Infof("start store reindex (%s)", flags.String())
	}

	if flags.objects && flags.fileObjects {
		// files will be indexed within object indexing (see indexLinkedFiles)
		// because we need to do it in the background.
		// otherwise it will lead to the situation when files loading called from the reindex with DisableRemoteFlag
		// will be waiting for the linkedFiles background indexing without this flag
		flags.fileObjects = false
	}

	if flags.fileKeys {
		err = i.fileStore.RemoveEmptyFileKeys()
		if err != nil {
			log.Errorf("reindex failed to RemoveEmptyFileKeys: %v", err.Error())
		} else {
			log.Infof("RemoveEmptyFileKeys filekeys succeed")
		}
	}

	if flags.removeAllIndexedObjects {
		ids, err := i.store.ListIds()
		if err != nil {
			log.Errorf("reindex failed to get all ids(removeAllIndexedObjects): %v", err.Error())
		}
		for _, id := range ids {
			err = i.store.DeleteDetails(id)
			if err != nil {
				log.Errorf("reindex failed to delete details(removeAllIndexedObjects): %v", err.Error())
			}
		}
	}
	if flags.eraseIndexes {
		err = i.store.EraseIndexes()
		if err != nil {
			log.Errorf("reindex failed to erase indexes: %v", err.Error())
		} else {
			log.Infof("all store indexes successfully erased")
		}
	}

	err = i.reindexBundledObjects(flags)
	if err != nil {
		log.Errorf("failed to reindex bundled objects: %s", err)
	}

	// We derive or init predefined blocks here in order to ensure consistency of object store.
	// If we call this method before removing objects from store, we will end up with inconsistent state
	// because indexing of predefined objects will not run again
	predefinedObjectIDs, err := i.anytype.EnsurePredefinedBlocks(context.Background(), i.spaceService.AccountId())
	if err != nil {
		return fmt.Errorf("ensure predefined objects: %w", err)
	}
	spaceIDs := []string{i.spaceService.AccountId()}

	// spaceID => workspaceID
	spacesToInit := map[string]string{}
	err = block.Do(i.picker, predefinedObjectIDs.Workspace, func(sb smartblock.SmartBlock) error {
		st := sb.NewState()
		spaces := st.Store().GetFields()["spaces"]
		for k, v := range spaces.GetStructValue().GetFields() {
			spacesToInit[k] = v.GetStringValue()
		}
		return nil
	})
	for spaceID, _ := range spacesToInit {
		spaceIDs = append(spaceIDs, spaceID)
		_, err = i.anytype.EnsurePredefinedBlocks(context.Background(), spaceID)
		if err != nil {
			return fmt.Errorf("ensure predefined objects for child space %s: %w", spaceID, err)
		}
	}

	for _, spaceID := range spaceIDs {
		err = i.EnsurePreinstalledObjects(spaceID)
		if err != nil {
			return fmt.Errorf("ensure preinstalled objects: %w", err)
		}
	}

	// starting sync of all other objects later, because we don't want to have problems with loading of derived objects
	// due to parallel load which can overload the stream
	i.syncStarter.StartSync()

	for _, spaceID := range spaceIDs {
		err = i.reindexSpace(spaceID, flags)
		if err != nil {
			return fmt.Errorf("reindex space %s: %w", spaceID, err)
		}
	}

	err = i.saveLatestChecksums()
	if err != nil {
		return fmt.Errorf("save latest checksums: %w", err)
	}

	return nil
}

func (i *indexer) reindexSpace(spaceID string, flags reindexFlags) (err error) {
	ctx := objectcache.CacheOptsWithRemoteLoadDisabled(context.Background())
	// for all ids except home and archive setting cache timeout for reindexing
	// ctx = context.WithValue(ctx, ocache.CacheTimeout, cacheTimeout)
	if flags.objects {
		ids, err := i.getIdsForTypes(
			spaceID,
			smartblock2.SmartBlockTypePage,
			smartblock2.SmartBlockTypeProfilePage,
			smartblock2.SmartBlockTypeTemplate,
			smartblock2.SmartBlockTypeArchive,
			smartblock2.SmartBlockTypeHome,
			smartblock2.SmartBlockTypeWorkspace,
			smartblock2.SmartBlockTypeObjectType,
			smartblock2.SmartBlockTypeRelation,
		)
		if err != nil {
			return err
		}
		start := time.Now()
		successfullyReindexed := i.reindexIdsIgnoreErr(ctx, ids...)

		i.logFinishedReindexStat(metrics.ReindexTypeThreads, len(ids), successfullyReindexed, time.Since(start))

		log.Infof("%d/%d objects have been successfully reindexed", successfullyReindexed, len(ids))
	} else {
		// Index objects that updated, but not indexed yet
		// TODO Write more informative comment
		go func() {
			start := time.Now()
			total, success, err := i.reindexOutdatedObjects(ctx, spaceID)
			if err != nil {
				log.Infof("failed to reindex outdated objects: %s", err.Error())
			} else {
				log.Infof("%d/%d outdated objects have been successfully reindexed", success, total)
			}
			if total > 0 {
				i.logFinishedReindexStat(metrics.ReindexTypeOutdatedHeads, total, success, time.Since(start))
			}
		}()
	}

	if flags.fileObjects {
		err = i.reindexIDsForSmartblockTypes(ctx, spaceID, metrics.ReindexTypeFiles, smartblock2.SmartBlockTypeFile)
		if err != nil {
			return err
		}
	}

	if flags.fulltext {
		ids, err := i.getIdsForTypes(spaceID, smartblock2.SmartBlockTypePage, smartblock2.SmartBlockTypeFile, smartblock2.SmartBlockTypeBundledRelation, smartblock2.SmartBlockTypeBundledObjectType, smartblock2.SmartBlockTypeAnytypeProfile)
		if err != nil {
			return err
		}

		var addedToQueue int
		for _, id := range ids {
			if err := i.store.AddToIndexQueue(id); err != nil {
				log.Errorf("failed to add to index queue: %v", err)
			} else {
				addedToQueue++
			}
		}
		msg := fmt.Sprintf("%d/%d objects have been successfully added to the fulltext queue", addedToQueue, len(ids))
		if len(ids)-addedToQueue != 0 {
			log.Error(msg)
		} else {
			log.Info(msg)
		}
	}

	return nil
}

func (i *indexer) reindexBundledObjects(flags reindexFlags) error {
	ctx := context.Background()
	spaceID := addr.AnytypeMarketplaceWorkspace

	if flags.bundledRelations {
		err := i.reindexIDsForSmartblockTypes(ctx, spaceID, metrics.ReindexTypeBundledRelations, smartblock2.SmartBlockTypeBundledRelation)
		if err != nil {
			return fmt.Errorf("reindex bundled relations: %w", err)
		}
	}
	if flags.bundledTypes {
		err := i.reindexIDsForSmartblockTypes(ctx, spaceID, metrics.ReindexTypeBundledTypes, smartblock2.SmartBlockTypeBundledObjectType, smartblock2.SmartBlockTypeAnytypeProfile)
		if err != nil {
			return fmt.Errorf("reindex bundled types: %w", err)
		}
	}

	if flags.bundledObjects {
		// hardcoded for now
		ids := []string{addr.AnytypeProfileId, addr.MissingObject}
		err := i.reindexIDs(ctx, metrics.ReindexTypeBundledObjects, ids)
		if err != nil {
			return fmt.Errorf("reindex profile and missing object: %w", err)
		}
	}

	if flags.bundledTemplates {
		existing, _, err := i.store.QueryObjectIDs(database.Query{}, []smartblock2.SmartBlockType{smartblock2.SmartBlockTypeBundledTemplate})
		if err != nil {
			return err
		}
		for _, id := range existing {
			err = i.store.DeleteObject(id)
			if err != nil {
				log.Errorf("delete old bundled template %s: %s", id, err)
			}
		}

		err = i.reindexIDsForSmartblockTypes(ctx, spaceID, metrics.ReindexTypeBundledTemplates, smartblock2.SmartBlockTypeBundledTemplate)
		if err != nil {
			return fmt.Errorf("reindex bundled templates: %w", err)
		}
	}

	return nil
}

func (i *indexer) reindexIDsForSmartblockTypes(ctx context.Context, spaceID string, reindexType metrics.ReindexType, sbTypes ...smartblock2.SmartBlockType) error {
	ids, err := i.getIdsForTypes(spaceID, sbTypes...)
	if err != nil {
		return err
	}
	return i.reindexIDs(ctx, reindexType, ids)
}

func (i *indexer) reindexIDs(ctx context.Context, reindexType metrics.ReindexType, ids []string) error {
	start := time.Now()
	successfullyReindexed := i.reindexIdsIgnoreErr(ctx, ids...)
	i.logFinishedReindexStat(reindexType, len(ids), successfullyReindexed, time.Since(start))
	return nil
}

func (i *indexer) reindexOutdatedObjects(ctx context.Context, spaceID string) (toReindex, success int, err error) {
	// reindex of subobject collection always leads to reindex of the all subobjects reindexing
	spc, err := i.spaceService.GetSpace(ctx, spaceID)
	if err != nil {
		return
	}

	tids := spc.StoredIds()
	var idsToReindex []string
	for _, tid := range tids {
		logErr := func(err error) {
			log.With("tree", tid).Errorf("reindexOutdatedObjects failed to get tree to reindex: %s", err.Error())
		}

		lastHash, err := i.store.GetLastIndexedHeadsHash(tid)
		if err != nil {
			logErr(err)
			continue
		}
		info, err := spc.Storage().TreeStorage(tid)
		if err != nil {
			logErr(err)
			continue
		}
		heads, err := info.Heads()
		if err != nil {
			logErr(err)
			continue
		}

		hh := headsHash(heads)
		if lastHash != hh {
			if lastHash != "" {
				log.With("tree", tid).Warnf("not equal indexed heads hash: %s!=%s (%d logs)", lastHash, hh, len(heads))
			}
			idsToReindex = append(idsToReindex, tid)
		}
	}

	success = i.reindexIdsIgnoreErr(ctx, idsToReindex...)
	return len(idsToReindex), success, nil
}

func (i *indexer) reindexDoc(ctx context.Context, id string) error {
	err := block.DoContext(i.picker, ctx, id, func(sb smartblock.SmartBlock) error {
		d := sb.GetDocInfo()
		spaceId := sb.SpaceID()
		if v, ok := sb.(editor.SubObjectCollectionGetter); ok {
			// index all the subobjects
			v.GetAllDocInfoIterator(
				func(info smartblock.DocInfo) (contin bool) {
					details := info.Details
					uk, err := domain.UnmarshalUniqueKey(pbtypes.GetString(details, bundle.RelationKeyUniqueKey.String()))
					if err != nil {
						log.With("objectID", id).Errorf("failed to unmarshal unique key: %v", err)
						return true
					}

					id, err := i.objectCreator.MigrateSubObjects(ctx, &uk, details, info.Type, spaceId)
					if err != nil {
						log.Errorf("failed to index subobject %s: %s", info.Id, err)
						log.With("objectID", id).Errorf("failed to migrate subobject: %v", err)
						return true
					}
					log.With("objectID", id).Warnf("migrated subobject")
					return true
				},
			)
		}

		return i.Index(ctx, d)
	})
	return err
}

func (i *indexer) reindexIdsIgnoreErr(ctx context.Context, ids ...string) (successfullyReindexed int) {
	for _, id := range ids {
		err := i.reindexDoc(ctx, id)
		if err != nil {
			log.With("objectID", id).Errorf("failed to reindex: %v", err)
		} else {
			successfullyReindexed++
		}
	}
	return
}

func (i *indexer) EnsurePreinstalledObjects(spaceID string) error {
	start := time.Now()
	ids := make([]string, 0, len(bundle.SystemTypes)+len(bundle.SystemRelations))
	for _, ot := range bundle.SystemTypes {
		ids = append(ids, ot.BundledURL())
	}

	for _, rk := range bundle.SystemRelations {
		ids = append(ids, rk.BundledURL())
	}
	_, _, err := i.objectCreator.InstallBundledObjects(context.Background(), spaceID, ids)
	if err != nil {
		return err
	}

	i.logFinishedReindexStat(metrics.ReindexTypeSystem, len(ids), len(ids), time.Since(start))

	return nil
}

func (i *indexer) saveLatestChecksums() error {
	checksums := model.ObjectStoreChecksums{
		BundledObjectTypes:         bundle.TypeChecksum,
		BundledRelations:           bundle.RelationChecksum,
		BundledTemplates:           i.btHash.Hash(),
		ObjectsForceReindexCounter: ForceObjectsReindexCounter,
		FilesForceReindexCounter:   ForceFilesReindexCounter,

		IdxRebuildCounter:                ForceIdxRebuildCounter,
		FulltextRebuild:                  ForceFulltextIndexCounter,
		BundledObjects:                   ForceBundledObjectsReindexCounter,
		FilestoreKeysForceReindexCounter: ForceFilestoreKeysReindexCounter,
	}
	return i.store.SaveChecksums(&checksums)
}

func (i *indexer) getIdsForTypes(spaceID string, sbt ...smartblock2.SmartBlockType) ([]string, error) {
	var ids []string
	for _, t := range sbt {
		lister, err := i.source.IDsListerBySmartblockType(spaceID, t)
		if err != nil {
			return nil, err
		}
		idsT, err := lister.ListIds()
		if err != nil {
			return nil, err
		}
		ids = append(ids, idsT...)
	}
	return ids, nil
}

func (i *indexer) GetLogFields() []zap.Field {
	return i.reindexLogFields
}

func (i *indexer) logFinishedReindexStat(reindexType metrics.ReindexType, totalIds, succeedIds int, spent time.Duration) {
	i.reindexLogFields = append(i.reindexLogFields, zap.Int("r_"+reindexType.String(), totalIds))
	if succeedIds < totalIds {
		i.reindexLogFields = append(i.reindexLogFields, zap.Int("r_"+reindexType.String()+"_failed", totalIds-succeedIds))
	}
	i.reindexLogFields = append(i.reindexLogFields, zap.Int64("r_"+reindexType.String()+"_spent", spent.Milliseconds()))
	msg := fmt.Sprintf("%d/%d %s have been successfully reindexed", succeedIds, totalIds, reindexType)
	if totalIds-succeedIds != 0 {
		log.Error(msg)
	} else {
		log.Info(msg)
	}

	if metrics.Enabled {
		metrics.SharedClient.RecordEvent(metrics.ReindexEvent{
			ReindexType: reindexType,
			Total:       totalIds,
			Succeed:     succeedIds,
			SpentMs:     int(spent.Milliseconds()),
		})
	}
}
