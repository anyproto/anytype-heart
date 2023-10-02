package indexer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v3"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block"
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

func (i *indexer) buildFlags(spaceID string) (reindexFlags, error) {
	var (
		checksums *model.ObjectStoreChecksums
		flags     reindexFlags
		err       error
	)
	if spaceID == "" {
		checksums, err = i.store.GetGlobalChecksums()
	} else {
		checksums, err = i.store.GetChecksums(spaceID)
	}
	if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
		return reindexFlags{}, err
	}
	if checksums == nil {
		// TODO: [MR] split object store checksums for space and common?
		checksums = &model.ObjectStoreChecksums{
			// per space
			ObjectsForceReindexCounter: ForceObjectsReindexCounter,
			// ?
			FilesForceReindexCounter: ForceFilesReindexCounter,
			// global
			IdxRebuildCounter: ForceIdxRebuildCounter,
			// per space
			FilestoreKeysForceReindexCounter: ForceFilestoreKeysReindexCounter,
			// per space
			FulltextRebuild: ForceFulltextIndexCounter,
			// global
			BundledObjects: ForceBundledObjectsReindexCounter,
		}
	}

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
	return flags, nil
}

func (i *indexer) ReindexSpace(spaceID string) (err error) {
	flags, err := i.buildFlags(spaceID)
	if err != nil {
		return
	}
	ctx := objectcache.CacheOptsWithRemoteLoadDisabled(context.Background())
	// for all ids except home and archive setting cache timeout for reindexing
	// ctx = context.WithValue(ctx, ocache.CacheTimeout, cacheTimeout)
	if flags.objects {
		types := []smartblock2.SmartBlockType{
			smartblock2.SmartBlockTypePage,
			smartblock2.SmartBlockTypeTemplate,
			smartblock2.SmartBlockTypeArchive,
			smartblock2.SmartBlockTypeHome,
			smartblock2.SmartBlockTypeWorkspace,
			smartblock2.SmartBlockTypeObjectType,
			smartblock2.SmartBlockTypeRelation,
			smartblock2.SmartBlockTypeSpaceObject,
			smartblock2.SmartBlockTypeProfilePage,
		}
		ids, err := i.getIdsForTypes(
			spaceID,
			types...,
		)
		if err != nil {
			return err
		}
		start := time.Now()
		successfullyReindexed := i.reindexIdsIgnoreErr(ctx, spaceID, ids...)

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

	return i.saveLatestChecksums(spaceID)
}

func (i *indexer) ReindexCommonObjects() error {
	flags, err := i.buildFlags("")
	if err != nil {
		return err
	}
	err = i.removeGlobalIndexes(flags)
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
		err := i.reindexIDs(ctx, addr.AnytypeMarketplaceWorkspace, metrics.ReindexTypeBundledObjects, ids)
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

	return i.saveLatestChecksums("")
}

func (i *indexer) removeGlobalIndexes(flags reindexFlags) (err error) {
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
	return
}

func (i *indexer) reindexIDsForSmartblockTypes(ctx context.Context, spaceID string, reindexType metrics.ReindexType, sbTypes ...smartblock2.SmartBlockType) error {
	ids, err := i.getIdsForTypes(spaceID, sbTypes...)
	if err != nil {
		return err
	}
	return i.reindexIDs(ctx, spaceID, reindexType, ids)
}

func (i *indexer) reindexIDs(ctx context.Context, spaceID string, reindexType metrics.ReindexType, ids []string) error {
	start := time.Now()
	successfullyReindexed := i.reindexIdsIgnoreErr(ctx, spaceID, ids...)
	i.logFinishedReindexStat(reindexType, len(ids), successfullyReindexed, time.Since(start))
	return nil
}

func (i *indexer) reindexOutdatedObjects(ctx context.Context, spaceID string) (toReindex, success int, err error) {
	// reindex of subobject collection always leads to reindex of the all subobjects reindexing
	spc, err := i.spaceCore.Get(ctx, spaceID)
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

	success = i.reindexIdsIgnoreErr(ctx, spaceID, idsToReindex...)
	return len(idsToReindex), success, nil
}

func (i *indexer) reindexDoc(ctx context.Context, spaceID, id string) error {
	// TODO: use special method for getting with id instead of this hack
	err := i.storageService.BindSpaceID(spaceID, id)
	if err != nil {
		return err
	}
	err = block.DoContext(i.picker, ctx, id, func(sb smartblock.SmartBlock) error {
		return i.Index(ctx, sb.GetDocInfo())
	})
	return err
}

func (i *indexer) reindexIdsIgnoreErr(ctx context.Context, spaceID string, ids ...string) (successfullyReindexed int) {
	for _, id := range ids {
		err := i.reindexDoc(ctx, spaceID, id)
		if err != nil {
			log.With("objectID", id).Errorf("failed to reindex: %v", err)
		} else {
			successfullyReindexed++
		}
	}
	return
}

func (i *indexer) saveLatestChecksums(spaceID string) error {
	checksums := model.ObjectStoreChecksums{
		BundledObjectTypes:               bundle.TypeChecksum,
		BundledRelations:                 bundle.RelationChecksum,
		BundledTemplates:                 i.btHash.Hash(),
		ObjectsForceReindexCounter:       ForceObjectsReindexCounter,
		FilesForceReindexCounter:         ForceFilesReindexCounter,
		IdxRebuildCounter:                ForceIdxRebuildCounter,
		FulltextRebuild:                  ForceFulltextIndexCounter,
		BundledObjects:                   ForceBundledObjectsReindexCounter,
		FilestoreKeysForceReindexCounter: ForceFilestoreKeysReindexCounter,
	}
	if spaceID == "" {
		return i.store.SaveGlobalChecksums(&checksums)
	}
	return i.store.SaveChecksums(spaceID, &checksums)
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
