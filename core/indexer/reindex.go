package indexer

import (
	"context"
	"errors"
	"fmt"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/headsync/headstorage"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/chatobject"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/detailsupdater/helper"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

const (
	// ForceObjectsReindexCounter reindex thread-based objects
	ForceObjectsReindexCounter int32 = 19

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

	ForceReindexDeletedObjectsCounter int32 = 1

	ForceReindexParticipantsCounter int32 = 1
	ForceReindexChatsCounter        int32 = 7
)

type allDeletedIdsProvider interface {
	AllDeletedTreeIds(ctx context.Context) (ids []string, err error)
}

func (i *indexer) buildFlags(spaceID string) (reindexFlags, error) {
	checksums, err := i.store.GetChecksums(spaceID)
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
			BundledObjects:        ForceBundledObjectsReindexCounter,
			AreOldFilesRemoved:    true,
			ReindexDeletedObjects: 0, // Set to zero to force reindexing of deleted objects when objectstore was deleted
			ReindexParticipants:   ForceReindexParticipantsCounter,
			ReindexChats:          ForceReindexChatsCounter,
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
	if checksums.ReindexDeletedObjects != ForceReindexDeletedObjectsCounter {
		flags.deletedObjects = true
	}
	if checksums.ReindexParticipants != ForceReindexParticipantsCounter {
		flags.removeParticipants = true
	}
	if checksums.LinksErase != ForceLinksReindexCounter {
		flags.eraseLinks = true
	}
	if checksums.ReindexChats != ForceReindexChatsCounter {
		flags.chats = true
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
	if flags.objects {
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
		go func() {
			start := time.Now()
			total, success, err := i.reindexOutdatedObjects(ctx, space)
			if err != nil {
				log.Errorf("reindex outdated failed: %s", err)
			}
			l := log.With(zap.String("space", space.Id()), zap.Int("total", total), zap.Int("succeed", success), zap.Int("spentMs", int(time.Since(start).Milliseconds())))
			if success != total {
				l.Errorf("reindex outdated partially failed")
			} else if total != 0 {
				l.Debugf("reindex outdated finished")
			}
			if total > 0 {
				i.logFinishedReindexStat(metrics.ReindexTypeOutdatedHeads, total, success, time.Since(start))
			}
		}()
	}

	if flags.chats {
		err = i.reindexChats(ctx, space)
		if err != nil {
			log.Error("reindex chats", zap.Error(err))
		}
	}

	if flags.deletedObjects {
		err = i.reindexDeletedObjects(space)
		if err != nil {
			log.Error("reindex deleted objects", zap.Error(err))
		}
	}

	if flags.removeParticipants {
		err = i.RemoveAclIndexes(space.Id())
		if err != nil {
			log.Error("reindex deleted objects", zap.Error(err))
		}
	}

	go i.addSyncDetails(space)

	return i.saveLatestChecksums(space.Id())
}

func (i *indexer) cleanChatCollection(ctx context.Context, db anystore.DB, chatId string, colName string) error {
	col, err := db.OpenCollection(ctx, chatId+colName)
	if errors.Is(err, anystore.ErrCollectionNotFound) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("open collection: %w", err)
	}
	var docIds []string
	it, err := col.Find(nil).Iter(ctx)
	if err != nil {
		return fmt.Errorf("create iterator: %w", err)
	}

	err = func() error {
		defer it.Close()

		for it.Next() {
			doc, err := it.Doc()
			if err != nil {
				return fmt.Errorf("get doc: %w", err)
			}
			id := doc.Value().Get("id").GetString()
			docIds = append(docIds, id)
		}
		return nil
	}()
	if err != nil {
		return fmt.Errorf("collect doc ids: %w", err)
	}

	for _, id := range docIds {
		err = col.DeleteId(ctx, id)
		if err != nil {
			return fmt.Errorf("delete doc id: %w", err)
		}
	}

	return nil
}

func (i *indexer) reindexChats(ctx context.Context, space clientspace.Space) error {
	ids, err := i.getIdsForTypes(space, coresb.SmartBlockTypeChatDerivedObject)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}

	db, err := i.dbProvider.GetCrdtDb(space.Id()).Wait()
	if err != nil {
		return fmt.Errorf("get crdt db: %w", err)
	}

	txn, err := db.WriteTx(ctx)
	if err != nil {
		return fmt.Errorf("write tx: %w", err)
	}
	defer func() {
		_ = txn.Rollback()
	}()

	for _, id := range ids {
		// Collection for messages
		err = i.cleanChatCollection(txn.Context(), db, id, chatobject.CollectionName)
		if err != nil {
			return fmt.Errorf("open collection: %w", err)
		}
		// Collection for details
		err = i.cleanChatCollection(txn.Context(), db, id, chatobject.EditorCollectionName)
		if err != nil {
			return fmt.Errorf("open collection: %w", err)
		}
		// Collection for orders
		err = i.cleanChatCollection(txn.Context(), db, id, storestate.CollChangeOrders)
		if err != nil {
			return fmt.Errorf("open collection: %w", err)
		}
	}

	err = txn.Commit()
	if err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	i.reindexIdsIgnoreErr(ctx, space, ids...)

	return nil
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
	store := i.store.SpaceIndex(space.Id())
	for _, id := range ids {
		err := space.DoLockedIfNotExists(id, func() error {
			return store.ModifyObjectDetails(id, func(details *domain.Details) (*domain.Details, bool, error) {
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
	store := i.store.SpaceIndex(space.Id())
	allIds, err := space.Storage().AllDeletedTreeIds(i.runCtx)
	if err != nil {
		return fmt.Errorf("get deleted tree ids: %w", err)
	}
	for _, objectId := range allIds {
		err = store.DeleteObject(objectId)
		if err != nil {
			log.With("spaceId", space.Id(), "objectId", objectId, "error", err).Errorf("failed to reindex deleted object")
		}
	}
	return nil
}

func (i *indexer) removeOldFiles(spaceId string, flags reindexFlags) error {
	if !flags.removeOldFiles {
		return nil
	}
	store := i.store.SpaceIndex(spaceId)
	// TODO: It seems we should also filter objects by Layout, because file objects should be re-indexed to receive resolvedLayout
	ids, _, err := store.QueryObjectIds(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value: domain.Int64List([]model.ObjectTypeLayout{
					model.ObjectType_file,
					model.ObjectType_image,
					model.ObjectType_video,
					model.ObjectType_audio,
					model.ObjectType_pdf,
				}),
			},
			{
				RelationKey: bundle.RelationKeyFileId,
				Condition:   model.BlockContentDataviewFilter_Empty,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("query old files: %w", err)
	}
	for _, id := range ids {
		if domain.IsFileId(id) {
			err = store.DeleteDetails(i.runCtx, []string{id})
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
		store := i.store.SpaceIndex(space.Id())
		existing, _, err := store.QueryObjectIds(database.Query{
			Filters: []database.FilterRequest{
				{
					RelationKey: bundle.RelationKeyType,
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       domain.String(bundle.TypeKeyTemplate.BundledURL()),
				},
			},
		})
		if err != nil {
			return fmt.Errorf("query bundled templates: %w", err)
		}
		for _, id := range existing {
			err = store.DeleteObject(id)
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
	store := i.store.SpaceIndex(spaceId)
	ids, err := store.ListIds()
	if err != nil {
		log.Errorf("reindex failed to get all ids(removeAllIndexedObjects): %v", err)
	}
	for _, id := range ids {
		if err = store.DeleteDetails(i.runCtx, []string{id}); err != nil {
			log.Errorf("reindex failed to delete details(removeAllIndexedObjects): %v", err)
		}
	}
	return err
}

func (i *indexer) removeCommonIndexes(spaceId string, space clientspace.Space, flags reindexFlags) (err error) {
	if flags.any() {
		log.Infof("start store reindex (%s)", flags.String())
	}

	if flags.eraseLinks {
		store := i.store.SpaceIndex(spaceId)
		ids, err := store.ListIds()
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
			if err = store.DeleteLinks([]string{id}); err != nil {
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

func (i *indexer) reindexOutdatedObjects(ctx context.Context, space clientspace.Space) (toReindex, success int, err error) {
	store := i.store.SpaceIndex(space.Id())
	var entries []headstorage.HeadsEntry

	err = space.Storage().HeadStorage().IterateEntries(ctx, headstorage.IterOpts{}, func(entry headstorage.HeadsEntry) (bool, error) {
		// skipping Acl
		if entry.CommonSnapshot != "" && entry.Id != space.Storage().StateStorage().SettingsId() {
			entries = append(entries, entry)
		}
		return true, nil
	})
	if err != nil {
		return
	}
	var idsToReindex []string
	for _, entry := range entries {
		id := entry.Id
		logErr := func(err error) {
			log.With("tree", entry.Id).Errorf("reindexOutdatedObjects failed to get tree to reindex: %s", err)
		}
		lastHash, err := store.GetLastIndexedHeadsHash(ctx, id)
		if err != nil {
			logErr(err)
			continue
		}
		hh := headsHash(entry.Heads)
		if lastHash != hh {
			if lastHash != "" {
				log.With("tree", id).Warnf("not equal indexed heads hash: %s!=%s (%d logs)", lastHash, hh, len(entry.Heads))
			}
			idsToReindex = append(idsToReindex, id)
		}
	}

	success = i.reindexIdsIgnoreErr(ctx, space, idsToReindex...)
	return len(idsToReindex), success, nil
}

func (i *indexer) reindexDoc(space smartblock.Space, id string) error {
	return space.Do(id, func(sb smartblock.SmartBlock) error {
		return i.Index(sb.GetDocInfo())
	})
}

func (i *indexer) reindexIdsIgnoreErr(ctx context.Context, space smartblock.Space, ids ...string) (successfullyReindexed int) {
	for _, id := range ids {
		select {
		case <-ctx.Done():
			return
		default:
		}
		err := i.reindexDoc(space, id)
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
		ReindexDeletedObjects:            ForceReindexDeletedObjectsCounter,
		ReindexParticipants:              ForceReindexParticipantsCounter,
		ReindexChats:                     ForceReindexChatsCounter,
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
}

func (i *indexer) RemoveIndexes(spaceId string) error {
	// Remove the spaceIndex from objectStore map and delete from filesystem
	return i.store.RemoveSpaceIndex(spaceId)
}
