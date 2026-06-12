package indexer

import (
	"context"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/headsync/headstorage"
	"github.com/anyproto/any-sync/commonspace/headsync/headstorage/mock_headstorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache/mock_objectcache"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/clientspace"
	mock_space "github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/anystorage/mock_anystorage"
)

func TestReindexMarketplaceSpace(t *testing.T) {
	spaceId := addr.AnytypeMarketplaceWorkspace
	getMockSpace := func(fx *fixture) *clientspace.VirtualSpace {
		virtualSpace := clientspace.NewVirtualSpace(spaceId, clientspace.VirtualSpaceDeps{
			Indexer: fx,
		})
		mockCache := mock_objectcache.NewMockCache(t)
		smartTest := smarttest.New(addr.MissingObject)
		smartTest.SetSpace(virtualSpace)

		smartTest.SetType(coresb.SmartBlockTypePage)
		smartTest.SetSpaceId("spaceId")
		mockCache.EXPECT().GetObject(context.Background(), addr.MissingObject).Return(editor.NewMissingObject(smartTest), nil)
		mockCache.EXPECT().GetObject(context.Background(), addr.AnytypeProfileId).Return(smartTest, nil)
		virtualSpace.Cache = mockCache

		return virtualSpace
	}

	t.Run("reindex missing object", func(t *testing.T) {
		// given
		indexerFx := newFixture(t)
		checksums := indexerFx.getLatestChecksums(true)
		err := indexerFx.store.SaveChecksums(spaceId, &checksums)
		assert.Nil(t, err)

		virtualSpace := getMockSpace(indexerFx)

		// when
		err = indexerFx.ReindexMarketplaceSpace(virtualSpace)

		// then
		details, err := indexerFx.store.SpaceIndex("space1").GetDetails(addr.MissingObject)
		assert.Nil(t, err)
		assert.NotNil(t, details)
	})

	t.Run("do not reindex links in marketplace", func(t *testing.T) {
		// given
		fx := newFixture(t)

		store := fx.store.SpaceIndex("space1")

		favs := []string{"fav1", "fav2"}
		trash := []string{"trash1", "trash2"}
		err := store.UpdateObjectLinks(ctx, "home", favs)
		require.NoError(t, err)
		err = store.UpdateObjectLinks(ctx, "bin", trash)
		require.NoError(t, err)

		homeLinks, err := store.GetOutboundLinksById("home")
		require.Equal(t, favs, homeLinks)

		archiveLinks, err := store.GetOutboundLinksById("bin")
		require.Equal(t, trash, archiveLinks)

		checksums := fx.getLatestChecksums(true)
		checksums.LinksErase = checksums.LinksErase - 1

		err = fx.objectStore.SaveChecksums(spaceId, &checksums)
		require.NoError(t, err)

		// when
		err = fx.ReindexMarketplaceSpace(getMockSpace(fx))
		assert.NoError(t, err)

		// then
		homeLinks, err = store.GetOutboundLinksById("home")
		assert.NoError(t, err)
		assert.Equal(t, favs, homeLinks)

		archiveLinks, err = store.GetOutboundLinksById("bin")
		assert.NoError(t, err)
		assert.Equal(t, trash, archiveLinks)

		storeChecksums, err := fx.store.GetChecksums(spaceId)
		assert.Equal(t, ForceLinksReindexCounter, storeChecksums.LinksErase)
	})

	t.Run("full marketplace reindex on force flag update", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:      domain.String("relationThatWillBeDeleted"),
			bundle.RelationKeyName:    domain.String("Relation-That-Will-Be-Deleted"),
			bundle.RelationKeySpaceId: domain.String(spaceId),
		}})

		checksums := fx.getLatestChecksums(true)
		checksums.MarketplaceForceReindexCounter = checksums.MarketplaceForceReindexCounter - 1

		err := fx.objectStore.SaveChecksums(spaceId, &checksums)
		require.NoError(t, err)

		fx.sourceFx.EXPECT().IDsListerBySmartblockType(mock.Anything, mock.Anything).Return(idsLister{Ids: []string{}}, nil).Maybe()

		// when
		err = fx.ReindexMarketplaceSpace(getMockSpace(fx))
		assert.NoError(t, err)

		// then
		det, err := fx.store.SpaceIndex("space1").GetDetails("relationThatWillBeDeleted")
		assert.NoError(t, err)
		assert.True(t, det.Len() == 0)
	})
}

func TestIndexer_ReindexSpace_RemoveParticipants(t *testing.T) {
	const (
		spaceId1 = "space1"
		spaceId2 = "space2"
	)
	fx := newFixture(t)

	fx.objectStore.AddObjects(t, spaceId1, []objectstore.TestObject{
		{
			bundle.RelationKeyId:             domain.String("_part1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_participant),
			bundle.RelationKeySpaceId:        domain.String(spaceId1),
		},
		{
			bundle.RelationKeyId:             domain.String("rand1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(model.SmartBlockType_Page),
			bundle.RelationKeySpaceId:        domain.String(spaceId1),
		},
	})
	fx.objectStore.AddObjects(t, spaceId2, []objectstore.TestObject{
		{
			bundle.RelationKeyId:             domain.String("_part2"),
			bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_participant),
			bundle.RelationKeySpaceId:        domain.String(spaceId2),
		},
		{
			bundle.RelationKeyId:             domain.String("_part21"),
			bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_participant),
			bundle.RelationKeySpaceId:        domain.String(spaceId2),
		},
		{
			bundle.RelationKeyId:             domain.String("rand2"),
			bundle.RelationKeyResolvedLayout: domain.Int64(model.SmartBlockType_Page),
			bundle.RelationKeySpaceId:        domain.String(spaceId1),
		},
	})

	checksums := fx.getLatestChecksums(false)
	checksums.ReindexParticipants = checksums.ReindexParticipants - 1

	err := fx.objectStore.SaveChecksums(spaceId1, &checksums)
	require.NoError(t, err)
	err = fx.objectStore.SaveChecksums(spaceId2, &checksums)
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	headStorage := mock_headstorage.NewMockHeadStorage(ctrl)
	storage := mock_anystorage.NewMockClientSpaceStorage(t)
	storage.EXPECT().HeadStorage().Return(headStorage)
	headStorage.EXPECT().IterateEntries(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(ctx context.Context, opts headstorage.IterOpts, entryIter headstorage.EntryIterator) error {
			return nil
		})

	for _, space := range []string{spaceId1, spaceId2} {
		t.Run("reindex - participants deleted - when flag doesn't match", func(t *testing.T) {
			// given
			store := fx.store.SpaceIndex(space)

			spc := mock_space.NewMockSpace(t)
			spc.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{}).Maybe() // reconcileLinkDerivedDetails: nothing to check
			spc.EXPECT().Id().Return(space)
			spc.EXPECT().Storage().Return(storage).Maybe()
			spc.EXPECT().FilterNotExists(mock.Anything).Return(nil).Maybe() // addSyncDetails: nothing to backfill in this test
			fx.sourceFx.EXPECT().IDsListerBySmartblockType(mock.Anything, mock.Anything).Return(idsLister{Ids: []string{}}, nil).Maybe()

			// when
			err = fx.ReindexSpace(spc)
			assert.NoError(t, err)

			// then
			ids, err := store.ListIds()
			assert.NoError(t, err)
			assert.Len(t, ids, 1)

			storeChecksums, err := fx.store.GetChecksums(space)
			assert.Equal(t, ForceReindexParticipantsCounter, storeChecksums.ReindexParticipants)
		})
	}

}

func TestIndexer_ReindexSpace_EraseLinks(t *testing.T) {
	const (
		spaceId1 = "space1"
		spaceId2 = "space2"
	)
	fx := newFixture(t)

	fx.sourceFx.EXPECT().IDsListerBySmartblockType(mock.Anything, mock.Anything).RunAndReturn(
		func(_ source.Space, sbt coresb.SmartBlockType) (source.IDsLister, error) {
			switch sbt {
			case coresb.SmartBlockTypeHome:
				return idsLister{Ids: []string{"home"}}, nil
			case coresb.SmartBlockTypeArchive:
				return idsLister{Ids: []string{"bin"}}, nil
			default:
				return idsLister{Ids: []string{}}, nil
			}
		},
	)

	fx.objectStore.AddObjects(t, spaceId1, []objectstore.TestObject{
		{
			bundle.RelationKeyId:      domain.String("fav1"),
			bundle.RelationKeySpaceId: domain.String(spaceId1),
		},
		{
			bundle.RelationKeyId:      domain.String("fav2"),
			bundle.RelationKeySpaceId: domain.String(spaceId1),
		},
		{
			bundle.RelationKeyId:      domain.String("trash1"),
			bundle.RelationKeySpaceId: domain.String(spaceId1),
		},
		{
			bundle.RelationKeyId:      domain.String("trash2"),
			bundle.RelationKeySpaceId: domain.String(spaceId1),
		},
	})
	fx.objectStore.AddObjects(t, spaceId2, []objectstore.TestObject{
		{
			bundle.RelationKeyId:      domain.String("obj1"),
			bundle.RelationKeySpaceId: domain.String(spaceId2),
		},
		{
			bundle.RelationKeyId:      domain.String("obj2"),
			bundle.RelationKeySpaceId: domain.String(spaceId2),
		},
		{
			bundle.RelationKeyId:      domain.String("obj3"),
			bundle.RelationKeySpaceId: domain.String(spaceId2),
		},
	})

	checksums := fx.getLatestChecksums(false)
	checksums.LinksErase = checksums.LinksErase - 1

	err := fx.objectStore.SaveChecksums(spaceId1, &checksums)
	require.NoError(t, err)
	err = fx.objectStore.SaveChecksums(spaceId2, &checksums)
	require.NoError(t, err)
	ctrl := gomock.NewController(t)
	headStorage := mock_headstorage.NewMockHeadStorage(ctrl)
	storage := mock_anystorage.NewMockClientSpaceStorage(t)
	storage.EXPECT().HeadStorage().Return(headStorage).Maybe()
	headStorage.EXPECT().IterateEntries(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(ctx context.Context, opts headstorage.IterOpts, entryIter headstorage.EntryIterator) error {
			return nil
		})

	t.Run("links from archive and home are deleted", func(t *testing.T) {
		// given
		favs := []string{"fav1", "fav2"}
		trash := []string{"trash1", "trash2"}
		store := fx.store.SpaceIndex("space1")

		err = store.UpdateObjectLinks(ctx, "home", favs)
		require.NoError(t, err)
		err = store.UpdateObjectLinks(ctx, "bin", trash)
		require.NoError(t, err)

		homeLinks, err := store.GetOutboundLinksById("home")
		require.Equal(t, favs, homeLinks)

		archiveLinks, err := store.GetOutboundLinksById("bin")
		require.Equal(t, trash, archiveLinks)

		space1 := mock_space.NewMockSpace(t)
		space1.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{}).Maybe() // reconcileLinkDerivedDetails: nothing to check
		space1.EXPECT().Id().Return(spaceId1)
		space1.EXPECT().Storage().Return(storage).Maybe()
		space1.EXPECT().FilterNotExists(mock.Anything).Return(nil).Maybe() // addSyncDetails: nothing to backfill in this test

		// when
		err = fx.ReindexSpace(space1)
		assert.NoError(t, err)

		// then
		homeLinks, err = store.GetOutboundLinksById("home")
		assert.NoError(t, err)
		assert.Empty(t, homeLinks)

		archiveLinks, err = store.GetOutboundLinksById("bin")
		assert.NoError(t, err)
		assert.Empty(t, archiveLinks)

		storeChecksums, err := fx.store.GetChecksums(spaceId1)
		assert.Equal(t, ForceLinksReindexCounter, storeChecksums.LinksErase)
	})

	t.Run("links from plain objects are deleted as well", func(t *testing.T) {
		// given
		obj1links := []string{"obj2", "obj3"}
		obj2links := []string{"obj1"}
		obj3links := []string{"obj2"}
		store := fx.store.SpaceIndex(spaceId2)
		err = store.UpdateObjectLinks(ctx, "obj1", obj1links)
		require.NoError(t, err)
		err = store.UpdateObjectLinks(ctx, "obj2", obj2links)
		require.NoError(t, err)
		err = store.UpdateObjectLinks(ctx, "obj3", obj3links)
		require.NoError(t, err)

		storedObj1links, err := store.GetOutboundLinksById("obj1")
		require.Equal(t, obj1links, storedObj1links)
		storedObj2links, err := store.GetOutboundLinksById("obj2")
		require.Equal(t, obj2links, storedObj2links)
		storedObj3links, err := store.GetOutboundLinksById("obj3")
		require.Equal(t, obj3links, storedObj3links)

		space1 := mock_space.NewMockSpace(t)
		space1.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{}).Maybe() // reconcileLinkDerivedDetails: nothing to check
		space1.EXPECT().Id().Return(spaceId2)
		space1.EXPECT().Storage().Return(storage).Maybe()
		space1.EXPECT().FilterNotExists(mock.Anything).Return(nil).Maybe() // addSyncDetails: nothing to backfill in this test
		// when
		err = fx.ReindexSpace(space1)
		assert.NoError(t, err)

		// then
		storedObj1links, err = store.GetOutboundLinksById("obj1")
		assert.NoError(t, err)
		assert.Empty(t, storedObj1links)
		storedObj2links, err = store.GetOutboundLinksById("obj2")
		assert.NoError(t, err)
		assert.Empty(t, storedObj2links)
		storedObj3links, err = store.GetOutboundLinksById("obj3")
		assert.NoError(t, err)
		assert.Empty(t, storedObj3links)

		storeChecksums, err := fx.store.GetChecksums(spaceId2)
		assert.NoError(t, err)
		assert.Equal(t, ForceLinksReindexCounter, storeChecksums.LinksErase)
	})
}

func TestReindex_addSyncRelations(t *testing.T) {
	const spaceId1 = "spaceId1"

	// newSpace returns a space whose FilterNotExists treats every id as
	// not-in-cache (so writes hit the store) and which therefore must never
	// be asked to take the per-object cache lock: DoLockedIfNotExists is left
	// unexpected on purpose so a regression that re-nests the cache lock
	// inside the write tx (the GO-7291 ABBA deadlock) fails the test.
	newSpace := func(t *testing.T) *mock_space.MockSpace {
		space1 := mock_space.NewMockSpace(t)
		space1.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{}).Maybe() // reconcileLinkDerivedDetails: nothing to check
		space1.EXPECT().Id().Return(spaceId1)
		space1.EXPECT().StoredIds().Return([]string{}).Maybe()
		space1.EXPECT().FilterNotExists(mock.Anything).
			RunAndReturn(func(ids []string) []string { return ids }).Maybe()
		return space1
	}

	t.Run("first run writes sync details for objects missing them", func(t *testing.T) {
		fx := newFixture(t)
		fx.config.NetworkMode = pb.RpcAccount_DefaultConfig
		fx.objectStore.AddObjects(t, spaceId1, []objectstore.TestObject{
			{bundle.RelationKeyId: domain.String("1"), bundle.RelationKeyName: domain.String("a")},
			{bundle.RelationKeyId: domain.String("2"), bundle.RelationKeyName: domain.String("b")},
		})

		fx.addSyncDetails(newSpace(t))

		for _, id := range []string{"1", "2"} {
			got, err := fx.objectStore.GetDetails(spaceId1, id)
			require.NoError(t, err)
			assert.True(t, got.Has(bundle.RelationKeySyncStatus))
			assert.Equal(t, int64(domain.ObjectSyncStatusSynced), got.GetInt64(bundle.RelationKeySyncStatus))
			assert.Equal(t, int64(domain.SyncErrorNull), got.GetInt64(bundle.RelationKeySyncError))
			assert.NotZero(t, got.GetInt64(bundle.RelationKeySyncDate))
		}
	})

	t.Run("local only mode writes error status", func(t *testing.T) {
		fx := newFixture(t) // default fixture config is LocalOnly
		fx.objectStore.AddObjects(t, spaceId1, []objectstore.TestObject{
			{bundle.RelationKeyId: domain.String("1"), bundle.RelationKeyName: domain.String("a")},
		})

		fx.addSyncDetails(newSpace(t))

		got, err := fx.objectStore.GetDetails(spaceId1, "1")
		require.NoError(t, err)
		assert.Equal(t, int64(domain.ObjectSyncStatusError), got.GetInt64(bundle.RelationKeySyncStatus))
		assert.Equal(t, int64(domain.SyncErrorNetworkError), got.GetInt64(bundle.RelationKeySyncError))
	})

	t.Run("repeat run is a no-op when sync details already present", func(t *testing.T) {
		fx := newFixture(t)
		fx.config.NetworkMode = pb.RpcAccount_DefaultConfig
		fx.objectStore.AddObjects(t, spaceId1, []objectstore.TestObject{
			{
				bundle.RelationKeyId:         domain.String("1"),
				bundle.RelationKeyName:       domain.String("a"),
				bundle.RelationKeySyncStatus: domain.Int64(int64(domain.ObjectSyncStatusSynced)),
				bundle.RelationKeySyncDate:   domain.Int64(123), // sentinel; must not be bumped
				bundle.RelationKeySyncError:  domain.Int64(int64(domain.SyncErrorNull)),
			},
		})

		space1 := mock_space.NewMockSpace(t)
		space1.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{}).Maybe() // reconcileLinkDerivedDetails: nothing to check
		space1.EXPECT().Id().Return(spaceId1)
		space1.EXPECT().StoredIds().Return([]string{}).Maybe()
		// Nothing is missing, so the filtered set must be empty and no
		// object must be touched. DoLockedIfNotExists is left unexpected:
		// addSyncDetails must never take the per-object cache lock.
		space1.EXPECT().FilterNotExists(mock.Anything).
			RunAndReturn(func(ids []string) []string {
				assert.Empty(t, ids, "repeat run must find nothing missing")
				return ids
			}).Maybe()

		fx.addSyncDetails(space1)

		got, err := fx.objectStore.GetDetails(spaceId1, "1")
		require.NoError(t, err)
		assert.Equal(t, int64(123), got.GetInt64(bundle.RelationKeySyncDate))
	})

	t.Run("missing relation is added without overwriting existing ones", func(t *testing.T) {
		fx := newFixture(t)
		fx.config.NetworkMode = pb.RpcAccount_DefaultConfig
		fx.objectStore.AddObjects(t, spaceId1, []objectstore.TestObject{
			{
				bundle.RelationKeyId:         domain.String("1"),
				bundle.RelationKeyName:       domain.String("a"),
				bundle.RelationKeySyncStatus: domain.Int64(int64(domain.ObjectSyncStatusSyncing)),
				bundle.RelationKeySyncDate:   domain.Int64(777),
				// SyncError is missing
			},
		})

		fx.addSyncDetails(newSpace(t))

		got, err := fx.objectStore.GetDetails(spaceId1, "1")
		require.NoError(t, err)
		// Existing values preserved: InjectsSyncDetails is only-if-absent.
		assert.Equal(t, int64(domain.ObjectSyncStatusSyncing), got.GetInt64(bundle.RelationKeySyncStatus))
		assert.Equal(t, int64(777), got.GetInt64(bundle.RelationKeySyncDate))
		// Missing one is added.
		assert.True(t, got.Has(bundle.RelationKeySyncError))
		assert.Equal(t, int64(domain.SyncErrorNull), got.GetInt64(bundle.RelationKeySyncError))
	})

	t.Run("re-filters every batch so an id cached mid-run is skipped", func(t *testing.T) {
		// Two single-id batches. The first id is not cached and must be
		// written; the second becomes cached between batches (it gets
		// loaded by a concurrent object load) and must be skipped. This
		// only holds if FilterNotExists is called per batch, after the
		// previous batch's write tx has been committed and released.
		old := addSyncDetailsBatchSize
		addSyncDetailsBatchSize = 1
		t.Cleanup(func() { addSyncDetailsBatchSize = old })

		fx := newFixture(t)
		fx.config.NetworkMode = pb.RpcAccount_DefaultConfig
		fx.objectStore.AddObjects(t, spaceId1, []objectstore.TestObject{
			{bundle.RelationKeyId: domain.String("1"), bundle.RelationKeyName: domain.String("a")},
			{bundle.RelationKeyId: domain.String("2"), bundle.RelationKeyName: domain.String("b")},
		})

		space1 := mock_space.NewMockSpace(t)
		space1.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{}).Maybe() // reconcileLinkDerivedDetails: nothing to check
		space1.EXPECT().Id().Return(spaceId1)
		space1.EXPECT().StoredIds().Return([]string{}).Maybe()
		var calls int
		space1.EXPECT().FilterNotExists(mock.Anything).
			RunAndReturn(func(ids []string) []string {
				calls++
				require.Len(t, ids, 1, "must be filtered one batch at a time")
				if ids[0] == "2" {
					return nil // "2" got loaded into the cache before its batch
				}
				return ids
			})

		fx.addSyncDetails(space1)

		assert.Equal(t, 2, calls, "FilterNotExists must be called once per batch")

		got1, err := fx.objectStore.GetDetails(spaceId1, "1")
		require.NoError(t, err)
		assert.True(t, got1.Has(bundle.RelationKeySyncStatus), "uncached id is written")

		got2, err := fx.objectStore.GetDetails(spaceId1, "2")
		require.NoError(t, err)
		assert.False(t, got2.Has(bundle.RelationKeySyncStatus), "id cached mid-run is skipped")
	})
}

func (fx *fixture) queryDeletedObjectIds(t *testing.T, spaceId string) []string {
	ids, _, err := fx.objectStore.SpaceIndex(spaceId).QueryObjectIds(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeySpaceId,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(spaceId),
			},
			{
				RelationKey: bundle.RelationKeyIsDeleted,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Bool(true),
			},
		},
	})
	require.NoError(t, err)
	return ids
}

type idsLister struct {
	Ids []string
}

func (l idsLister) ListIds() ([]string, error) {
	return l.Ids, nil
}

func TestReconcileLinkDerivedDetails(t *testing.T) {
	const (
		spaceId   = "space1"
		homeId    = "home1"
		archiveId = "archive1"
	)
	derivedIds := threads.DerivedSmartblockIds{Home: homeId, Archive: archiveId}
	headsEntries := map[string]headstorage.HeadsEntry{
		homeId:    {Id: homeId, Heads: []string{"homeHead1", "homeHead2"}, CommonSnapshot: "cs"},
		archiveId: {Id: archiveId, Heads: []string{"archiveHead1"}, CommonSnapshot: "cs"},
	}

	newSpaceMock := func(t *testing.T, entries map[string]headstorage.HeadsEntry) *mock_space.MockSpace {
		ctrl := gomock.NewController(t)
		headStorage := mock_headstorage.NewMockHeadStorage(ctrl)
		headStorage.EXPECT().GetEntry(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
			func(_ context.Context, id string) (headstorage.HeadsEntry, error) {
				entry, ok := entries[id]
				if !ok {
					return headstorage.HeadsEntry{}, anystore.ErrDocNotFound
				}
				return entry, nil
			})
		storage := mock_anystorage.NewMockClientSpaceStorage(t)
		storage.EXPECT().HeadStorage().Return(headStorage).Maybe()
		spc := mock_space.NewMockSpace(t)
		spc.EXPECT().Id().Return(spaceId).Maybe()
		spc.EXPECT().DerivedIDs().Return(derivedIds).Maybe()
		spc.EXPECT().Storage().Return(storage).Maybe()
		return spc
	}

	t.Run("in sync: no objects are opened", func(t *testing.T) {
		// given
		fx := newFixture(t)
		store := fx.store.SpaceIndex(spaceId)
		// marker is order-insensitive over heads
		require.NoError(t, store.SaveReconcileMarker(ctx, homeId, spaceindex.HashIds([]string{"homeHead2", "homeHead1"})))
		require.NoError(t, store.SaveReconcileMarker(ctx, archiveId, spaceindex.HashIds([]string{"archiveHead1"})))
		// no DoCtx expectation: any open fails the test
		spc := newSpaceMock(t, headsEntries)

		// when
		fx.reconcileLinkDerivedDetails(spc)
	})

	t.Run("stale marker triggers reconcile of that object only", func(t *testing.T) {
		// given
		fx := newFixture(t)
		store := fx.store.SpaceIndex(spaceId)
		require.NoError(t, store.SaveReconcileMarker(ctx, homeId, spaceindex.HashIds([]string{"oldHead"})))
		require.NoError(t, store.SaveReconcileMarker(ctx, archiveId, spaceindex.HashIds([]string{"archiveHead1"})))
		spc := newSpaceMock(t, headsEntries)
		spc.EXPECT().DoCtx(mock.Anything, homeId, mock.Anything).Return(nil).Once()

		// when
		fx.reconcileLinkDerivedDetails(spc)
	})

	t.Run("absent marker triggers reconcile", func(t *testing.T) {
		// given
		fx := newFixture(t)
		spc := newSpaceMock(t, headsEntries)
		spc.EXPECT().DoCtx(mock.Anything, homeId, mock.Anything).Return(nil).Once()
		spc.EXPECT().DoCtx(mock.Anything, archiveId, mock.Anything).Return(nil).Once()

		// when
		fx.reconcileLinkDerivedDetails(spc)
	})

	t.Run("tree not local: skipped, mandatory load path reconciles instead", func(t *testing.T) {
		// given
		fx := newFixture(t)
		spc := newSpaceMock(t, nil)

		// when
		fx.reconcileLinkDerivedDetails(spc)
	})

	t.Run("deleted tree: skipped", func(t *testing.T) {
		// given
		fx := newFixture(t)
		spc := newSpaceMock(t, map[string]headstorage.HeadsEntry{
			homeId:    {Id: homeId, Heads: []string{"homeHead1"}, DeletedStatus: headstorage.DeletedStatusDeleted},
			archiveId: {Id: archiveId, Heads: []string{"archiveHead1"}, DeletedStatus: headstorage.DeletedStatusQueued},
		})

		// when
		fx.reconcileLinkDerivedDetails(spc)
	})
}
