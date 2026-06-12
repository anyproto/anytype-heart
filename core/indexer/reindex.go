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
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
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
	ForceLinksReindexCounter int32 = 3

	// ForceMarketplaceReindex forces to do reindex only for marketplace space
	ForceMarketplaceReindex int32 = 1

	ForceReindexDeletedObjectsCounter int32 = 1

	ForceReindexParticipantsCounter  int32 = 1
	ForceReindexChatsCounter         int32 = 7
	ForceReindexChatsFulltextCounter int32 = 1
	ForceReindexDiscussionsCounter   int32 = 1

	// ForceFTRecheckCounter triggers a lightweight FT consistency check
	// Aggregates the list of object ids that need to be indexed and verify their presence in the FT index.
	// Bumped to 1 for GO-7316: backfill objects missed by the FT queue and
	// garbage-collect orphaned FT docs accumulated by the old broken deletion
	// paths (bare-id deletes, no cleanup on space offload).
	ForceFTRecheckCounter int32 = 1

	// ForceInvalidateObjectsIndexCounter clears all indexed heads hashes, causing reindexOutdatedObjects
	// to reindex all objects. This is more efficient than ForceObjectsReindexCounter because it
	// reindexes objects asynchronously and continue reindex after app F
	// Bumped to 4 for GO-7237: collection members, inline dataview embeds, and Object-marks
	// are now indexed as outgoing links — existing objects need a one-shot reindex pass.
	ForceInvalidateObjectsIndexCounter int32 = 4
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
			BundledObjects:              ForceBundledObjectsReindexCounter,
			AreOldFilesRemoved:          true,
			ReindexDeletedObjects:       0, // Set to zero to force reindexing of deleted objects when objectstore was deleted
			ReindexParticipants:         ForceReindexParticipantsCounter,
			ReindexChats:                ForceReindexChatsCounter,
			ReindexDiscussions:          ForceReindexDiscussionsCounter,
			ReindexFulltextChatMessages: ForceReindexChatsFulltextCounter,
			InvalidateObjectsIndex:      ForceInvalidateObjectsIndexCounter,
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
	if checksums.ReindexDiscussions != ForceReindexDiscussionsCounter {
		flags.discussions = true
	}
	if checksums.ReindexFulltextChatMessages != ForceReindexChatsFulltextCounter {
		flags.messagesFulltext = true
	}
	if checksums.InvalidateObjectsIndex != ForceInvalidateObjectsIndexCounter {
		flags.invalidateObjectsIndex = true
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

		if flags.invalidateObjectsIndex {
			store := i.store.SpaceIndex(space.Id())
			if err := store.ClearHeadsState(ctx); err != nil {
				log.With(zap.String("space", space.Id())).Errorf("failed to clear heads state: %s", err)
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

	if flags.discussions {
		err = i.reindexDiscussions(ctx, space)
		if err != nil {
			log.Error("reindex discussions", zap.Error(err))
		}
	}

	if flags.messagesFulltext {
		err = i.reindexChatMessagesFulltext(ctx, space)
		if err != nil {
			log.Error("reindex chats fulltext", zap.Error(err))
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

func (i *indexer) cleanMetaEntry(ctx context.Context, db anystore.DB, objectId string) error {
	col, err := db.OpenCollection(ctx, storestate.CollMeta)
	if errors.Is(err, anystore.ErrCollectionNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open meta collection: %w", err)
	}
	err = col.DeleteId(ctx, objectId)
	if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
		return fmt.Errorf("delete meta entry: %w", err)
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
		// Clean addSeq entry from meta collection
		if err = i.cleanMetaEntry(txn.Context(), db, id); err != nil {
			return fmt.Errorf("clean meta entry: %w", err)
		}
	}

	err = txn.Commit()
	if err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	i.reindexIdsIgnoreErr(ctx, space, ids...)

	return nil
}

func (i *indexer) reindexDiscussions(ctx context.Context, space clientspace.Space) error {
	ids, err := i.getIdsForTypes(space, coresb.SmartBlockTypeDiscussionObject)
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
		err = i.cleanChatCollection(txn.Context(), db, id, chatobject.CollectionName)
		if err != nil {
			return fmt.Errorf("clean discussion messages collection: %w", err)
		}
		err = i.cleanChatCollection(txn.Context(), db, id, chatobject.EditorCollectionName)
		if err != nil {
			return fmt.Errorf("clean discussion editor collection: %w", err)
		}
		// Clean addSeq entry from meta collection
		if err = i.cleanMetaEntry(txn.Context(), db, id); err != nil {
			return fmt.Errorf("clean discussion meta entry: %w", err)
		}
	}

	err = txn.Commit()
	if err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	i.reindexIdsIgnoreErr(ctx, space, ids...)

	return nil
}

func (i *indexer) reindexChatMessagesFulltext(ctx context.Context, space clientspace.Space) error {
	ids, err := i.getIdsForTypes(space, coresb.SmartBlockTypeChatDerivedObject)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}

	for _, id := range ids {
		err = i.store.AddChatMessageToIndexQueue(ctx, domain.FullID{ObjectID: id, SpaceID: space.Id()}, objectstore.FtAllOrderId)
		if err != nil {
			log.With("chatId", id).Errorf("failed to add chat to fulltext queue: %v", err)
		}
	}

	return nil
}

// addSyncDetailsBatchSize bounds how many objects share a single write tx, so
// the (single) write connection is not held for the whole space at once. It is
// a var (not a const) only so tests can shrink it to exercise multi-batch
// re-filtering.
var addSyncDetailsBatchSize = 500

// addSyncDetails ensures every object that is missing the sync relations
// (SyncStatus/SyncDate/SyncError, see helper.InjectsSyncDetails) gets them.
//
// Steady state it is a single bulk query that returns nothing and writes
// nothing. On the first-ever launch it writes the missing objects in chunked
// shared write transactions. The whole pass runs under a non-cancelable
// context (context.WithoutCancel) on purpose:
//   - any-store arms a per-statement SQLite SetInterrupt goroutine/channel
//     handshake only when ctx.Done() != nil; that handshake dominates startup
//     scheduler-latency, and this op is bounded/internal so losing
//     interruptibility is acceptable;
//   - WriteTx on a non-tx ctx opens a real tx and threads the tx into
//     txn.Context(), so ModifyObjectDetailsCtx reuses it via the savepoint
//     path instead of a BEGIN IMMEDIATE per object.
//
// The not-in-cache check is done per batch via space.FilterNotExists, which
// acquires and releases the object-cache mutex before that batch's write tx is
// opened. It must NOT be done per-id inside the write tx (the old
// space.DoLockedIfNotExists path): that nests the cache mutex inside the
// any-store write connection, the reverse of the order taken by object loads
// (cache mutex -> write conn), and deadlocks on cold start (GO-7291).
// Re-filtering each batch keeps the stale window to a single batch instead of
// the whole run. The residual window — an id loaded after its batch filter but
// before its write — is benign: SyncStatus/SyncDate/SyncError are local-only
// relations the syncstatus service continuously republishes for loaded
// objects, so a stale baseline write is superseded.
func (i *indexer) addSyncDetails(space clientspace.Space) {
	syncStatus := domain.ObjectSyncStatusSynced
	syncError := domain.SyncErrorNull
	if i.config.IsLocalOnlyMode() {
		syncStatus = domain.ObjectSyncStatusError
		syncError = domain.SyncErrorNetworkError
	}
	store := i.store.SpaceIndex(space.Id())
	ctx := context.WithoutCancel(i.runCtx)

	ids, err := store.ListIdsWithoutSyncDetails(ctx)
	if err != nil {
		log.Error("add sync details: list ids without sync details", zap.Error(err))
		return
	}

	if len(ids) > 0 {
		fmt.Printf("### addSyncDetails: backfilling sync details for %d objects in space %s\n", len(ids), space.Id())
	}

	for start := 0; start < len(ids); start += addSyncDetailsBatchSize {
		end := start + addSyncDetailsBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		// Filter out ids loaded/loading in the object cache and release the
		// cache mutex before opening the write tx (see the function doc).
		// Re-done every batch so an id loaded while earlier batches were
		// committing is re-checked against the cache, not written stale.
		batch := space.FilterNotExists(ids[start:end])
		if len(batch) == 0 {
			continue
		}
		txn, err := store.WriteTx(ctx)
		if err != nil {
			log.Error("add sync details: start write tx", zap.Error(err))
			return
		}
		for _, id := range batch {
			modErr := store.ModifyObjectDetailsCtx(txn.Context(), id, func(details *domain.Details) (*domain.Details, bool, error) {
				return helper.InjectsSyncDetails(details, syncStatus, syncError), true, nil
			}, true)
			if modErr != nil {
				log.Debug("failed to add sync status relations", zap.Error(modErr))
			}
		}
		if err := txn.Commit(); err != nil {
			log.Error("add sync details: commit write tx", zap.Error(err))
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

	// FT queue consistency check: detect objects that were added to headsState
	// but the FT queue wasn't flushed before crash
	i.checkFTQueueConsistency(ctx, store, space.Id())

	var entries []headstorage.HeadsEntry

	start := time.Now()
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
		// todo: make it more effective
		id := entry.Id
		logErr := func(err error) {
			log.With("tree", entry.Id).Errorf("reindexOutdatedObjects failed to get tree to reindex: %s", err)
		}
		lastHash, err := store.GetLastIndexedHeadsHash(ctx, id)
		if err != nil {
			logErr(err)
			continue
		}
		if lastHash == "" || lastHash != headsHash(entry.Heads) {
			idsToReindex = append(idsToReindex, id)
		}
	}
	if len(idsToReindex) == 0 {
		return 0, 0, nil
	}
	log.Warn("reindexOutdatedObjects: found outdated objects to reindex", zap.Int("len", len(idsToReindex)), zap.Int64("durMs", time.Since(start).Milliseconds()))
	success = i.reindexIdsIgnoreErr(ctx, space, idsToReindex...)
	return len(idsToReindex), success, nil
}

// checkFTQueueConsistency checks for objects that may have been added to headsState
// but the FT queue wasn't flushed before crash. It compares the ftQueueCtr in headsState
// against the persisted FT queue counter (per-space) and re-adds any missing objects to the queue.
func (i *indexer) checkFTQueueConsistency(ctx context.Context, store spaceindex.Store, spaceId string) {
	// Read THIS space's counter from commonDB
	ftQueueCounter, err := i.store.GetFTQueueCounter(ctx, spaceId)
	if err != nil {
		log.With("space", spaceId).Warnf("get ft queue counter: %v", err)
		return
	}

	// If counter is 0, this is either first run for this space or no FT queue operations have happened yet
	if ftQueueCounter == 0 {
		return
	}

	entries, err := store.GetHeadsWithFtQueueCtrGreaterThan(ctx, ftQueueCounter)
	if err != nil {
		log.With("space", spaceId).Warnf("get heads with ftQueueCtr > counter: %v", err)
		return
	}

	if len(entries) == 0 {
		return
	}

	log.With("space", spaceId, "count", len(entries), "ftQueueCounter", ftQueueCounter).
		Warn("ft queue consistency: re-adding objects after crash recovery")

	var toRequeue []domain.FullID
	for _, e := range entries {
		toRequeue = append(toRequeue, domain.FullID{ObjectID: e.ObjectID, SpaceID: spaceId})
	}

	// Re-adding will update the per-space counter in commonDB
	_, _, err = i.store.AddToIndexQueue(ctx, toRequeue...)
	if err != nil {
		log.With("space", spaceId).Errorf("re-add to ft queue: %v", err)
	}
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
		ReindexDiscussions:               ForceReindexDiscussionsCounter,
		ReindexFulltextChatMessages:      ForceReindexChatsFulltextCounter,
		InvalidateObjectsIndex:           ForceInvalidateObjectsIndexCounter,
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
	i.lock.RLock()
	defer i.lock.RUnlock()
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
	var flags reindexFlags
	flags.enableAll()
	if err := i.removeCommonIndexes(spaceId, nil, flags); err != nil {
		log.Errorf("remove common indexes on space removal: %v", err)
	}
	// Remove the space's full-text documents and pending queue entries.
	// Failures are logged, not returned: leftover docs are only re-collected if
	// the same space is offloaded again (the consistency check's orphan GC
	// iterates store spaces, and this space's store is deleted right below).
	if err := i.store.ClearFullTextQueue([]string{spaceId}); err != nil {
		log.Errorf("clear fulltext queue on space removal: %v", err)
	}
	if err := i.removeFullTextIndexes(spaceId); err != nil {
		log.Errorf("remove fulltext docs on space removal: %v", err)
	}
	// Drop the per-space objectstore entirely: close the in-memory index and
	// remove the on-disk objectstore/CRDT databases for the space.
	if err := i.store.DeleteSpaceIndex(spaceId); err != nil {
		return fmt.Errorf("delete space index: %w", err)
	}
	return nil
}

// removeFullTextIndexes deletes all full-text documents belonging to the
// space. ListIdsBySpace returns one page at a time, so loop until empty; the
// iteration cap only guards against an unforeseen non-progressing loop.
func (i *indexer) removeFullTextIndexes(spaceId string) error {
	const maxIterations = 1000
	for iteration := 0; iteration < maxIterations; iteration++ {
		ids, err := i.ftsearch.ListIdsBySpace(spaceId, 0)
		if err != nil {
			return fmt.Errorf("list space doc ids: %w", err)
		}
		if len(ids) == 0 {
			return nil
		}
		batcher := i.ftsearch.NewAutoBatcher()
		for _, id := range ids {
			if err = batcher.DeleteDoc(id); err != nil {
				return fmt.Errorf("delete doc: %w", err)
			}
		}
		if _, err = batcher.Finish(); err != nil {
			return fmt.Errorf("finish delete batch: %w", err)
		}
	}
	return fmt.Errorf("space docs still present after %d delete iterations", maxIterations)
}
