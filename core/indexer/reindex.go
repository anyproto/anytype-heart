package indexer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anyproto/any-sync/util/slice"
	"github.com/dgraph-io/badger/v4"
	"github.com/globalsign/mgo/bson"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	smartblock2 "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	// ForceObjectsReindexCounter reindex thread-based objects
	ForceObjectsReindexCounter int32 = 14

	// ForceFilesReindexCounter reindex ipfs-file-based objects
	ForceFilesReindexCounter int32 = 11 //

	// ForceBundledObjectsReindexCounter reindex objects like anytypeProfile
	ForceBundledObjectsReindexCounter int32 = 5 // reindex objects like anytypeProfile

	// ForceIdxRebuildCounter erases localstore indexes and reindex all type of objects
	// (no need to increase ForceObjectsReindexCounter & ForceFilesReindexCounter)
	ForceIdxRebuildCounter int32 = 62

	// ForceFulltextIndexCounter  performs fulltext indexing for all type of objects (useful when we change fulltext config)
	ForceFulltextIndexCounter int32 = 5

	// ForceFilestoreKeysReindexCounter reindex filestore keys in all objects
	ForceFilestoreKeysReindexCounter int32 = 2
)

func (i *indexer) buildFlags(spaceID string) (reindexFlags, error) {
	checksums, err := i.store.GetChecksums(spaceID)
	if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
		return reindexFlags{}, err
	}
	if checksums == nil {
		checksums, err = i.store.GetGlobalChecksums()
		if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return reindexFlags{}, err
		}

		if checksums == nil {
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
	return flags, nil
}

func (i *indexer) ReindexSpace(space clientspace.Space) (err error) {
	flags, err := i.buildFlags(space.Id())
	if err != nil {
		return
	}
	err = i.removeCommonIndexes(space.Id(), flags)
	if err != nil {
		return fmt.Errorf("remove common indexes: %w", err)
	}
	ctx := objectcache.CacheOptsWithRemoteLoadDisabled(context.Background())
	// for all ids except home and archive setting cache timeout for reindexing
	// ctx = context.WithValue(ctx, ocache.CacheTimeout, cacheTimeout)
	if flags.objects {
		types := []smartblock2.SmartBlockType{
			// System types first
			smartblock2.SmartBlockTypeObjectType,
			smartblock2.SmartBlockTypeRelation,
			smartblock2.SmartBlockTypeRelationOption,
			smartblock2.SmartBlockTypeFileObject,

			smartblock2.SmartBlockTypePage,
			smartblock2.SmartBlockTypeTemplate,
			smartblock2.SmartBlockTypeArchive,
			smartblock2.SmartBlockTypeHome,
			smartblock2.SmartBlockTypeWorkspace,
			smartblock2.SmartBlockTypeSpaceView,
			smartblock2.SmartBlockTypeProfilePage,
		}
		ids, err := i.getIdsForTypes(
			space.Id(),
			types...,
		)
		if err != nil {
			return err
		}
		start := time.Now()
		successfullyReindexed := i.reindexIdsIgnoreErr(ctx, space, ids...)

		i.logFinishedReindexStat(metrics.ReindexTypeThreads, len(ids), successfullyReindexed, time.Since(start))
		l := log.With(zap.String("space", space.Id()), zap.Int("total", len(ids)), zap.Int("succeed", successfullyReindexed))
		if successfullyReindexed != len(ids) {
			l.Errorf("reindex partially failed")
		} else {
			l.Infof("reindex finished")
		}
	} else {
		// Index objects that updated, but not indexed yet
		// we can have objects which actual state is newer than the indexed one
		// this may happen e.g. if the app got closed in the middle of object updates processing
		// So here we reindexOutdatedObjects which compare the last indexed heads hash with the actual one
		go func() {
			start := time.Now()
			total, success, err := i.reindexOutdatedObjects(ctx, space)
			if err != nil {
				log.Errorf("reindex outdated failed: %s", err)
			}
			l := log.With(zap.String("space", space.Id()), zap.Int("total", total), zap.Int("succeed", success), zap.Int("spentMs", int(time.Since(start).Milliseconds())))
			if success != total {
				l.Errorf("reindex outdated partially failed")
			} else {
				l.Debugf("reindex outdated finished")
			}
			if total > 0 {
				i.logFinishedReindexStat(metrics.ReindexTypeOutdatedHeads, total, success, time.Since(start))
			}
		}()
	}

	if flags.fulltext {
		ids, err := i.getIdsForTypes(space.Id(),
			smartblock2.SmartBlockTypePage,
			smartblock2.SmartBlockTypeFileObject,
			smartblock2.SmartBlockTypeBundledRelation,
			smartblock2.SmartBlockTypeBundledObjectType,
			smartblock2.SmartBlockTypeAnytypeProfile,
			smartblock2.SmartBlockTypeObjectType,
			smartblock2.SmartBlockTypeRelation,
			smartblock2.SmartBlockTypeRelationOption,
		)
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

	return i.saveLatestChecksums(space.Id())
}

func (i *indexer) ReindexMarketplaceSpace(space clientspace.Space) error {
	flags, err := i.buildFlags(space.Id())
	if err != nil {
		return err
	}
	err = i.removeCommonIndexes(space.Id(), flags)
	if err != nil {
		return fmt.Errorf("remove common indexes: %w", err)
	}
	ctx := context.Background()

	if flags.bundledRelations {
		err := i.reindexIDsForSmartblockTypes(ctx, space, metrics.ReindexTypeBundledRelations, smartblock2.SmartBlockTypeBundledRelation)
		if err != nil {
			return fmt.Errorf("reindex bundled relations: %w", err)
		}
	}
	if flags.bundledTypes {
		err := i.reindexIDsForSmartblockTypes(ctx, space, metrics.ReindexTypeBundledTypes, smartblock2.SmartBlockTypeBundledObjectType, smartblock2.SmartBlockTypeAnytypeProfile)
		if err != nil {
			return fmt.Errorf("reindex bundled types: %w", err)
		}
	}
	if flags.bundledObjects {
		// hardcoded for now
		ids := []string{addr.AnytypeProfileId, addr.MissingObject}
		err := i.reindexIDs(ctx, space, metrics.ReindexTypeBundledObjects, ids)
		if err != nil {
			return fmt.Errorf("reindex profile and missing object: %w", err)
		}
	}

	if flags.bundledTemplates {
		existing, _, err := i.store.QueryObjectIDs(database.Query{
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyType.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String(bundle.TypeKeyTemplate.BundledURL()),
				},
			},
		})
		if err != nil {
			return fmt.Errorf("query bundled templates: %w", err)
		}
		for _, id := range existing {
			err = i.store.DeleteObject(id)
			if err != nil {
				log.Errorf("delete old bundled template %s: %s", id, err)
			}
		}

		err = i.reindexIDsForSmartblockTypes(ctx, space, metrics.ReindexTypeBundledTemplates, smartblock2.SmartBlockTypeBundledTemplate)
		if err != nil {
			return fmt.Errorf("reindex bundled templates: %w", err)
		}
	}

	return i.saveLatestChecksums(space.Id())
}

// removeOldObjects removes all objects that are not supported anymore (e.g. old subobjects) and no longer returned by the underlying source
func (i *indexer) removeOldObjects() (err error) {
	ids, err := i.store.ListIds()
	if err != nil {
		return err
	}
	ids = slice.Filter(ids, func(id string) bool {
		if strings.HasPrefix(id, addr.RelationKeyToIdPrefix) {
			return true
		}
		if strings.HasPrefix(id, addr.ObjectTypeKeyToIdPrefix) {
			return true
		}
		if bson.IsObjectIdHex(id) {
			return true
		}
		return false
	})

	if len(ids) == 0 {
		return
	}

	err = i.store.DeleteDetails(ids...)
	log.With(zap.Int("count", len(ids)), zap.Error(err)).Warnf("removeOldObjects")
	return err
}

func (i *indexer) removeCommonIndexes(spaceId string, flags reindexFlags) (err error) {
	if flags.any() {
		log.Infof("start store reindex (%s)", flags.String())
	}

	if flags.fileKeys {
		err = i.fileStore.RemoveEmptyFileKeys()
		if err != nil {
			log.Errorf("reindex failed to RemoveEmptyFileKeys: %v", err)
		} else {
			log.Infof("RemoveEmptyFileKeys filekeys succeed")
		}
	}

	if flags.removeAllIndexedObjects {
		err = i.removeOldObjects()
		if err != nil {
			err = nil
			log.Errorf("reindex failed to removeOldObjects: %v", err)
		}
		var ids []string
		ids, err = i.store.ListIdsBySpace(spaceId)
		if err != nil {
			log.Errorf("reindex failed to get all ids(removeAllIndexedObjects): %v", err)
		}
		for _, id := range ids {
			if err = i.store.DeleteLinks(id); err != nil {
				log.Errorf("reindex failed to delete links(removeAllIndexedObjects): %v", err)
			}
		}
		for _, id := range ids {
			if err = i.store.DeleteDetails(id); err != nil {
				log.Errorf("reindex failed to delete details(removeAllIndexedObjects): %v", err)
			}
		}
	}
	return
}

func (i *indexer) reindexIDsForSmartblockTypes(ctx context.Context, space smartblock.Space, reindexType metrics.ReindexType, sbTypes ...smartblock2.SmartBlockType) error {
	ids, err := i.getIdsForTypes(space.Id(), sbTypes...)
	if err != nil {
		return err
	}
	return i.reindexIDs(ctx, space, reindexType, ids)
}

func (i *indexer) reindexIDs(ctx context.Context, space smartblock.Space, reindexType metrics.ReindexType, ids []string) error {
	start := time.Now()
	successfullyReindexed := i.reindexIdsIgnoreErr(ctx, space, ids...)
	i.logFinishedReindexStat(reindexType, len(ids), successfullyReindexed, time.Since(start))
	return nil
}

func (i *indexer) reindexOutdatedObjects(ctx context.Context, space clientspace.Space) (toReindex, success int, err error) {
	tids := space.StoredIds()
	var idsToReindex []string
	for _, tid := range tids {
		logErr := func(err error) {
			log.With("tree", tid).Errorf("reindexOutdatedObjects failed to get tree to reindex: %s", err)
		}

		lastHash, err := i.store.GetLastIndexedHeadsHash(tid)
		if err != nil {
			logErr(err)
			continue
		}
		info, err := space.Storage().TreeStorage(tid)
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

	success = i.reindexIdsIgnoreErr(ctx, space, idsToReindex...)
	return len(idsToReindex), success, nil
}

func (i *indexer) reindexDoc(ctx context.Context, space smartblock.Space, id string) error {
	return space.Do(id, func(sb smartblock.SmartBlock) error {
		return i.Index(ctx, sb.GetDocInfo())
	})
}

func (i *indexer) reindexIdsIgnoreErr(ctx context.Context, space smartblock.Space, ids ...string) (successfullyReindexed int) {
	for _, id := range ids {
		err := i.reindexDoc(ctx, space, id)
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

func (i *indexer) RemoveIndexes(spaceId string) error {
	var flags reindexFlags
	flags.enableAll()
	return i.removeCommonIndexes(spaceId, flags)
}
