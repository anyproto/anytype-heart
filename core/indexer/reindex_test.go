package indexer

import (
	"context"
	"testing"

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
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	mock_space "github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/anystorage/mock_anystorage"
)

func TestReindexMarketplaceSpace(t *testing.T) {
	spaceId := addr.AnytypeMarketplaceWorkspace
	getMockSpace := func(fx *IndexerFixture) *clientspace.VirtualSpace {
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
		indexerFx := NewIndexerFixture(t)
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
		fx := NewIndexerFixture(t)

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
		fx := NewIndexerFixture(t)
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
	fx := NewIndexerFixture(t)

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
			spc.EXPECT().Id().Return(space)
			spc.EXPECT().Storage().Return(storage).Maybe()
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
	fx := NewIndexerFixture(t)

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
		space1.EXPECT().Id().Return(spaceId1)
		space1.EXPECT().Storage().Return(storage).Maybe()

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
		space1.EXPECT().Id().Return(spaceId2)
		space1.EXPECT().Storage().Return(storage).Maybe()
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
	t.Run("addSyncRelations local only", func(t *testing.T) {
		// given
		const spaceId1 = "spaceId1"
		fx := NewIndexerFixture(t)

		fx.objectStore.AddObjects(t, spaceId1, []objectstore.TestObject{
			{
				bundle.RelationKeyId:        domain.String("1"),
				bundle.RelationKeyIsDeleted: domain.Bool(true),
			},
			{
				bundle.RelationKeyId:        domain.String("2"),
				bundle.RelationKeyIsDeleted: domain.Bool(true),
			},
		})

		space1 := mock_space.NewMockSpace(t)
		space1.EXPECT().Id().Return(spaceId1)
		space1.EXPECT().StoredIds().Return([]string{}).Maybe()

		fx.sourceFx.EXPECT().IDsListerBySmartblockType(space1, coresb.SmartBlockTypePage).Return(idsLister{Ids: []string{"1", "2"}}, nil)
		fx.sourceFx.EXPECT().IDsListerBySmartblockType(space1, coresb.SmartBlockTypeRelation).Return(idsLister{Ids: []string{}}, nil)
		fx.sourceFx.EXPECT().IDsListerBySmartblockType(space1, coresb.SmartBlockTypeRelationOption).Return(idsLister{Ids: []string{}}, nil)
		fx.sourceFx.EXPECT().IDsListerBySmartblockType(space1, coresb.SmartBlockTypeFileObject).Return(idsLister{Ids: []string{}}, nil)
		fx.sourceFx.EXPECT().IDsListerBySmartblockType(space1, coresb.SmartBlockTypeObjectType).Return(idsLister{Ids: []string{}}, nil)
		fx.sourceFx.EXPECT().IDsListerBySmartblockType(space1, coresb.SmartBlockTypeTemplate).Return(idsLister{Ids: []string{}}, nil)
		fx.sourceFx.EXPECT().IDsListerBySmartblockType(space1, coresb.SmartBlockTypeProfilePage).Return(idsLister{Ids: []string{}}, nil)
		fx.sourceFx.EXPECT().IDsListerBySmartblockType(space1, coresb.SmartBlockTypeChatDerivedObject).Return(idsLister{Ids: []string{}}, nil)
		fx.sourceFx.EXPECT().IDsListerBySmartblockType(space1, coresb.SmartBlockTypeChatObject).Return(idsLister{Ids: []string{}}, nil)
		fx.sourceFx.EXPECT().IDsListerBySmartblockType(space1, coresb.SmartBlockTypeSpaceView).Return(idsLister{Ids: []string{}}, nil)

		space1.EXPECT().DoLockedIfNotExists("1", mock.AnythingOfType("func() error")).Return(nil)
		space1.EXPECT().DoLockedIfNotExists("2", mock.AnythingOfType("func() error")).Return(nil)

		// when
		fx.addSyncDetails(space1)

		// then
	})

	t.Run("addSyncRelations", func(t *testing.T) {
		// given
		const spaceId1 = "spaceId1"
		fx := NewIndexerFixture(t)

		fx.objectStore.AddObjects(t, spaceId1, []objectstore.TestObject{
			{
				bundle.RelationKeyId:        domain.String("1"),
				bundle.RelationKeyIsDeleted: domain.Bool(true),
			},
			{
				bundle.RelationKeyId:        domain.String("2"),
				bundle.RelationKeyIsDeleted: domain.Bool(true),
			},
		})

		space1 := mock_space.NewMockSpace(t)
		space1.EXPECT().Id().Return(spaceId1)
		space1.EXPECT().StoredIds().Return([]string{}).Maybe()

		fx.sourceFx.EXPECT().IDsListerBySmartblockType(space1, coresb.SmartBlockTypePage).Return(idsLister{Ids: []string{"1", "2"}}, nil)
		fx.sourceFx.EXPECT().IDsListerBySmartblockType(space1, coresb.SmartBlockTypeRelation).Return(idsLister{Ids: []string{}}, nil)
		fx.sourceFx.EXPECT().IDsListerBySmartblockType(space1, coresb.SmartBlockTypeRelationOption).Return(idsLister{Ids: []string{}}, nil)
		fx.sourceFx.EXPECT().IDsListerBySmartblockType(space1, coresb.SmartBlockTypeFileObject).Return(idsLister{Ids: []string{}}, nil)
		fx.sourceFx.EXPECT().IDsListerBySmartblockType(space1, coresb.SmartBlockTypeObjectType).Return(idsLister{Ids: []string{}}, nil)
		fx.sourceFx.EXPECT().IDsListerBySmartblockType(space1, coresb.SmartBlockTypeTemplate).Return(idsLister{Ids: []string{}}, nil)
		fx.sourceFx.EXPECT().IDsListerBySmartblockType(space1, coresb.SmartBlockTypeProfilePage).Return(idsLister{Ids: []string{}}, nil)
		fx.sourceFx.EXPECT().IDsListerBySmartblockType(space1, coresb.SmartBlockTypeChatDerivedObject).Return(idsLister{Ids: []string{}}, nil)
		fx.sourceFx.EXPECT().IDsListerBySmartblockType(space1, coresb.SmartBlockTypeChatObject).Return(idsLister{Ids: []string{}}, nil)
		fx.sourceFx.EXPECT().IDsListerBySmartblockType(space1, coresb.SmartBlockTypeSpaceView).Return(idsLister{Ids: []string{}}, nil)

		space1.EXPECT().DoLockedIfNotExists("1", mock.AnythingOfType("func() error")).Return(nil)
		space1.EXPECT().DoLockedIfNotExists("2", mock.AnythingOfType("func() error")).Return(nil)

		fx.config.NetworkMode = pb.RpcAccount_DefaultConfig

		// when
		fx.addSyncDetails(space1)
	})
}

func (fx *IndexerFixture) queryDeletedObjectIds(t *testing.T, spaceId string) []string {
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
