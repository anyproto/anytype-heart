package indexer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/util/slice"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/detailsupdater/helper"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/taskmanager"
)

const (
	// ForceObjectsReindexCounter reindex thread-based objects
	ForceObjectsReindexCounter int32 = 16

	// ForceFilesReindexCounter reindex file objects
	ForceFilesReindexCounter int32 = 12 //

	// ForceBundledObjectsReindexCounter reindex objects like anytypeProfile
	ForceBundledObjectsReindexCounter int32 = 5 // reindex objects like anytypeProfile

	// ForceIdxRebuildCounter erases localstore indexes and reindex all type of objects
	// (no need to increase ForceObjectsReindexCounter & ForceFilesReindexCounter)
	ForceIdxRebuildCounter int32 = 62

	// ForceFilestoreKeysReindexCounter reindex filestore keys in all objects
	ForceFilestoreKeysReindexCounter int32 = 2

	// ForceLinksReindexCounter forces to erase links from store and reindex them
	ForceLinksReindexCounter int32 = 1

	// ForceMarketplaceReindex forces to do reindex only for marketplace space
	ForceMarketplaceReindex int32 = 1
)

func (i *indexer) buildFlags(spaceID string) (reindexFlags, error) {
	checksums, err := i.store.GetChecksums(spaceID)
	if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
		return reindexFlags{}, err
	}
	if checksums == nil {
		checksums, err = i.store.GetGlobalChecksums()
		if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
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
				LinksErase:                       ForceLinksReindexCounter,
				// global
				BundledObjects:             ForceBundledObjectsReindexCounter,
				AreOldFilesRemoved:         true,
				AreDeletedObjectsReindexed: true,
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
	if checksums.BundledTemplates != i.btHash.Hash() {
		flags.bundledTemplates = true
	}
	if checksums.BundledObjects != ForceBundledObjectsReindexCounter {
		flags.bundledObjects = true
	}
	if checksums.IdxRebuildCounter != ForceIdxRebuildCounter {
		flags.enableAll()
	}
	if !checksums.AreOldFilesRemoved {
		flags.removeOldFiles = true
	}
	if !checksums.AreDeletedObjectsReindexed {
		flags.deletedObjects = true
	}
	if checksums.LinksErase != ForceLinksReindexCounter {
		flags.eraseLinks = true
	}
	if spaceID == addr.AnytypeMarketplaceWorkspace && checksums.MarketplaceForceReindexCounter != ForceMarketplaceReindex {
		flags.enableAll()
	}
	return flags, nil
}

func (i *indexer) ReindexSpace(space clientspace.Space) (err error) {
	flags, err := i.buildFlags(space.Id())
	if err != nil {
		return
	}
	err = i.removeCommonIndexes(space.Id(), space, flags)
	if err != nil {
		return fmt.Errorf("remove common indexes: %w", err)
	}

	err = i.removeOldFiles(space.Id(), flags)
	if err != nil {
		return fmt.Errorf("remove old files: %w", err)
	}

	ctx := objectcache.CacheOptsWithRemoteLoadDisabled(context.Background())
	// for all ids except home and archive setting cache timeout for reindexing
	// ctx = context.WithValue(ctx, ocache.CacheTimeout, cacheTimeout)
	log.Warnf("reindex flags: %v", flags.String())

	if flags.objects {
		log.Warn("reindex all objects because of flags")

		types := []coresb.SmartBlockType{
			// System types first
			coresb.SmartBlockTypeObjectType,
			coresb.SmartBlockTypeRelation,
			coresb.SmartBlockTypeRelationOption,
			coresb.SmartBlockTypeFileObject,

			coresb.SmartBlockTypePage,
			coresb.SmartBlockTypeTemplate,
			coresb.SmartBlockTypeArchive,
			coresb.SmartBlockTypeHome,
			coresb.SmartBlockTypeWorkspace,
			coresb.SmartBlockTypeSpaceView,
			coresb.SmartBlockTypeProfilePage,
		}
		ids, err := i.getIdsForTypes(space, types...)
		if err != nil {
			return err
		}
		err = i.store.DeleteLastIndexedHeadHash(ids...)
		if err != nil {
			return fmt.Errorf("delete last indexed head hash: %w", err)
		}
	} else {

		if flags.fileObjects {
			err := i.reindexIDsForSmartblockTypes(ctx, space, metrics.ReindexTypeFiles, coresb.SmartBlockTypeFileObject)
			if err != nil {
				return fmt.Errorf("reindex file objects: %w", err)
			}
		}

		// Index objects that updated, but not indexed yet
		// we can have objects which actual state is newer than the indexed one
		// this may happen e.g. if the app got closed in the middle of object updates processing
		// So here we reindexOutdatedObjects which compare the last indexed heads hash with the actual one

		i.addToReindexQueue(space)
	}

	if flags.deletedObjects {
		err = i.reindexDeletedObjects(space)
		if err != nil {
			log.Error("reindex deleted objects", zap.Error(err))
		}
	}

	i.addSyncDetails(space)
	return i.saveLatestChecksums(space.Id())
}

func (i *indexer) addSyncDetails(space clientspace.Space) {
	typesForSyncRelations := helper.SyncRelationsSmartblockTypes()
	syncStatus := domain.ObjectSyncStatusSynced
	syncError := domain.SyncErrorNull
	if i.config.IsLocalOnlyMode() {
		syncStatus = domain.ObjectSyncStatusError
		syncError = domain.SyncErrorNetworkError
	}
	ids, err := i.getIdsForTypes(space, typesForSyncRelations...)
	if err != nil {
		log.Debug("failed to add sync status relations", zap.Error(err))
	}
	for _, id := range ids {
		err := space.DoLockedIfNotExists(id, func() error {
			return i.store.ModifyObjectDetails(id, func(details *types.Struct) (*types.Struct, bool, error) {
				details = helper.InjectsSyncDetails(details, syncStatus, syncError)
				return details, true, nil
			})
		})
		if err != nil {
			log.Debug("failed to add sync status relations", zap.Error(err))
		}
	}
}

func (i *indexer) reindexDeletedObjects(space clientspace.Space) error {
	recs, err := i.store.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyIsDeleted.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Bool(true),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Empty,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("query deleted objects: %w", err)
	}
	for _, rec := range recs {
		objectId := pbtypes.GetString(rec.Details, bundle.RelationKeyId.String())
		status, err := space.Storage().TreeDeletedStatus(objectId)
		if err != nil {
			log.With("spaceId", space.Id(), "objectId", objectId).Warnf("failed to get tree deleted status: %s", err)
			continue
		}
		if status != "" {
			err = i.store.DeleteObject(domain.FullID{SpaceID: space.Id(), ObjectID: objectId})
			if err != nil {
				log.With("spaceId", space.Id(), "objectId", objectId).Errorf("failed to reindex deleted object: %s", err)
			}
		}
	}
	return nil
}

func (i *indexer) removeOldFiles(spaceId string, flags reindexFlags) error {
	if !flags.removeOldFiles {
		return nil
	}
	ids, _, err := i.store.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(spaceId),
			},
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value: pbtypes.IntList(
					int(model.ObjectType_file),
					int(model.ObjectType_image),
					int(model.ObjectType_video),
					int(model.ObjectType_audio),
				),
			},
			{
				RelationKey: bundle.RelationKeyFileId.String(),
				Condition:   model.BlockContentDataviewFilter_Empty,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("query old files: %w", err)
	}
	for _, id := range ids {
		if domain.IsFileId(id) {
			err = i.store.DeleteDetails(id)
			if err != nil {
				log.Errorf("delete old file %s: %s", id, err)
			}
		}
	}
	return nil
}

func (i *indexer) ReindexMarketplaceSpace(space clientspace.Space) error {
	flags, err := i.buildFlags(space.Id())
	if err != nil {
		return err
	}

	if flags.removeAllIndexedObjects {
		err = i.removeDetails(space.Id())
		if err != nil {
			return fmt.Errorf("remove details for marketplace space: %w", err)
		}
	}

	ctx := context.Background()

	if flags.bundledRelations {
		err = i.reindexIDsForSmartblockTypes(ctx, space, metrics.ReindexTypeBundledRelations, coresb.SmartBlockTypeBundledRelation)
		if err != nil {
			return fmt.Errorf("reindex bundled relations: %w", err)
		}
	}
	if flags.bundledTypes {
		err = i.reindexIDsForSmartblockTypes(ctx, space, metrics.ReindexTypeBundledTypes, coresb.SmartBlockTypeBundledObjectType, coresb.SmartBlockTypeAnytypeProfile)
		if err != nil {
			return fmt.Errorf("reindex bundled types: %w", err)
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
			err = i.store.DeleteObject(domain.FullID{SpaceID: space.Id(), ObjectID: id})
			if err != nil {
				log.Errorf("delete old bundled template %s: %s", id, err)
			}
		}

		err = i.reindexIDsForSmartblockTypes(ctx, space, metrics.ReindexTypeBundledTemplates, coresb.SmartBlockTypeBundledTemplate)
		if err != nil {
			return fmt.Errorf("reindex bundled templates: %w", err)
		}
	}
	err = i.reindexIDs(ctx, space, metrics.ReindexTypeBundledObjects, []string{addr.AnytypeProfileId, addr.MissingObject})
	if err != nil {
		return fmt.Errorf("reindex profile and missing object: %w", err)
	}
	return i.saveLatestChecksums(space.Id())
}

func (i *indexer) removeDetails(spaceId string) error {
	err := i.removeOldObjects()
	if err != nil {
		err = nil
		log.Errorf("reindex failed to removeOldObjects: %v", err)
	}
	ids, err := i.store.ListIdsBySpace(spaceId)
	if err != nil {
		log.Errorf("reindex failed to get all ids(removeAllIndexedObjects): %v", err)
	}
	for _, id := range ids {
		if err = i.store.DeleteDetails(id); err != nil {
			log.Errorf("reindex failed to delete details(removeAllIndexedObjects): %v", err)
		}
	}
	return err
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

func (i *indexer) removeCommonIndexes(spaceId string, space clientspace.Space, flags reindexFlags) (err error) {
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

	if flags.eraseLinks {
		ids, err := i.store.ListIdsBySpace(spaceId)
		if err != nil {
			log.Errorf("reindex failed to get all ids(eraseLinks): %v", err)
		}

		// we get ids of Home and Archive separately from other objects,
		// because we do not index its details, so it could not be fetched via store.Query
		if space != nil {
			homeAndArchive, err := i.getIdsForTypes(space, coresb.SmartBlockTypeHome, coresb.SmartBlockTypeArchive)
			if err != nil {
				log.Errorf("reindex: failed to get ids of home and archive (eraseLinks): %v", err)
			}
			ids = append(ids, homeAndArchive...)
		}

		for _, id := range ids {
			if err = i.store.DeleteLinks(id); err != nil {
				log.Errorf("reindex failed to delete links(eraseLinks): %v", err)
			}
		}
	}

	if flags.removeAllIndexedObjects {
		err = i.removeDetails(spaceId)
	}

	return
}

func (i *indexer) reindexIDsForSmartblockTypes(ctx context.Context, space smartblock.Space, reindexType metrics.ReindexType, sbTypes ...coresb.SmartBlockType) error {
	ids, err := i.getIdsForTypes(space, sbTypes...)
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

func (i *indexer) getLatestChecksums(isMarketplace bool) (checksums model.ObjectStoreChecksums) {
	checksums = model.ObjectStoreChecksums{
		BundledObjectTypes:               bundle.TypeChecksum,
		BundledRelations:                 bundle.RelationChecksum,
		BundledTemplates:                 i.btHash.Hash(),
		ObjectsForceReindexCounter:       ForceObjectsReindexCounter,
		FilesForceReindexCounter:         ForceFilesReindexCounter,
		IdxRebuildCounter:                ForceIdxRebuildCounter,
		BundledObjects:                   ForceBundledObjectsReindexCounter,
		FilestoreKeysForceReindexCounter: ForceFilestoreKeysReindexCounter,
		AreOldFilesRemoved:               true,
		AreDeletedObjectsReindexed:       true,
		LinksErase:                       ForceLinksReindexCounter,
	}
	if isMarketplace {
		checksums.MarketplaceForceReindexCounter = ForceMarketplaceReindex
	}
	return
}

func (i *indexer) saveLatestChecksums(spaceID string) error {
	checksums := i.getLatestChecksums(spaceID == addr.AnytypeMarketplaceWorkspace)
	return i.store.SaveChecksums(spaceID, &checksums)
}

func (i *indexer) getIdsForTypes(space smartblock.Space, sbt ...coresb.SmartBlockType) ([]string, error) {
	var ids []string
	for _, t := range sbt {
		lister, err := i.source.IDsListerBySmartblockType(space, t)
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
	i.lock.Lock()
	defer i.lock.Unlock()
	return i.reindexLogFields
}

func (i *indexer) logFinishedReindexStat(reindexType metrics.ReindexType, totalIds, succeedIds int, spent time.Duration) {
	i.lock.Lock()
	defer i.lock.Unlock()
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
		metrics.Service.Send(&metrics.ReindexEvent{
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
	return i.removeCommonIndexes(spaceId, nil, flags)
}

func (i *indexer) runReindexerQueue() {
	go func() {
		for {
			select {
			case <-i.quit:
			case priority := <-i.lastSpacesSubscriptionUpdateChan:
				i.updateSpacesPriority(pbtypes.ExtractString(priority, bundle.RelationKeyTargetSpaceId.String(), true))
			}
		}
	}()
	go i.spaceReindexQueue.Run(context.Background())
}

func (i *indexer) closeReindexerQueue() {
	i.spaceReindexQueue.WaitAndClose()
}

func (i *indexer) addToReindexQueue(space clientspace.Space) {
	log.Debug("reindexOutdatedObjects space %s added to queue", space.Id())

	task := i.newReIndexTask(space)
	i.spaceReindexQueue.AddTask(task)
}

func (i *indexer) newReIndexTask(space clientspace.Space) taskmanager.Task {
	return &reindexTask{
		TaskBase: taskmanager.NewTaskBase(space.Id()),
		space:    space,
		store:    i.store,
		indexer:  i,
	}
}

type reindexTask struct {
	taskmanager.TaskBase
	space   clientspace.Space
	store   objectstore.ObjectStore
	indexer *indexer
}

func (t *reindexTask) Run(ctx context.Context) error {
	start := time.Now()
	tids := t.space.StoredIds()
	var totalIndex, successIndex int
	var err error
	for _, tid := range tids {
		err = t.WaitIfPaused(ctx)
		if err != nil {
			return err
		}
		log.Debugf("reindexOutdatedObjects started for %s/%s", t.space.Id(), tid)
		logErr := func(err error) {
			log.With("tree", tid).Errorf("reindexOutdatedObjects failed to get tree to reindex: %s", err)
		}

		lastHash, err := t.store.GetLastIndexedHeadsHash(tid)
		if err != nil {
			logErr(err)
			continue
		}
		info, err := t.space.Storage().TreeStorage(tid)
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
		if lastHash == hh {
			continue
		}

		if lastHash != "" {
			log.With("tree", tid).Warnf("not equal indexed heads hash: %s!=%s (%d logs)", lastHash, hh, len(heads))
		}
		totalIndex++

		err = t.indexer.reindexDoc(ctx, t.space, tid)
		if err != nil {
			logErr(err)
			continue
		}
		successIndex++
	}

	log.Warn("reindexOutdatedObjects finished", zap.Int("total", totalIndex), zap.Int("succeed", successIndex), zap.Int("spentMs", int(time.Since(start).Milliseconds())))
	return nil
}
