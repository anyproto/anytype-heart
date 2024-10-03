package indexer

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage/mock_spacestorage"
	"github.com/gogo/protobuf/types"
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
	"github.com/anyproto/anytype-heart/space/spacecore/storage/mock_storage"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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

		storage := mock_storage.NewMockClientStorage(t)
		storage.EXPECT().BindSpaceID(mock.Anything, mock.Anything).Return(nil)
		indexerFx.storageService = storage

		// when
		err = indexerFx.ReindexMarketplaceSpace(virtualSpace)

		// then
		details, err := indexerFx.store.GetDetails(addr.MissingObject)
		assert.Nil(t, err)
		assert.NotNil(t, details)
	})

	t.Run("do not reindex links in marketplace", func(t *testing.T) {
		// given
		fx := NewIndexerFixture(t)

		favs := []string{"fav1", "fav2"}
		trash := []string{"trash1", "trash2"}
		err := fx.store.UpdateObjectLinks("home", favs)
		require.NoError(t, err)
		err = fx.store.UpdateObjectLinks("bin", trash)
		require.NoError(t, err)

		homeLinks, err := fx.store.GetOutboundLinksByID("home")
		require.Equal(t, favs, homeLinks)

		archiveLinks, err := fx.store.GetOutboundLinksByID("bin")
		require.Equal(t, trash, archiveLinks)

		checksums := fx.getLatestChecksums(true)
		checksums.LinksErase = checksums.LinksErase - 1

		err = fx.objectStore.SaveChecksums(spaceId, &checksums)
		require.NoError(t, err)

		storage := mock_storage.NewMockClientStorage(t)
		storage.EXPECT().BindSpaceID(mock.Anything, mock.Anything).Return(nil)
		fx.storageService = storage

		// when
		err = fx.ReindexMarketplaceSpace(getMockSpace(fx))
		assert.NoError(t, err)

		// then
		homeLinks, err = fx.store.GetOutboundLinksByID("home")
		assert.NoError(t, err)
		assert.Equal(t, favs, homeLinks)

		archiveLinks, err = fx.store.GetOutboundLinksByID("bin")
		assert.NoError(t, err)
		assert.Equal(t, trash, archiveLinks)

		storeChecksums, err := fx.store.GetChecksums(spaceId)
		assert.Equal(t, ForceLinksReindexCounter, storeChecksums.LinksErase)
	})

	t.Run("full marketplace reindex on force flag update", func(t *testing.T) {
		// given
		fx := NewIndexerFixture(t)
		fx.objectStore.AddObjects(t, []objectstore.TestObject{map[domain.RelationKey]*types.Value{
			bundle.RelationKeyId:      pbtypes.String("relationThatWillBeDeleted"),
			bundle.RelationKeyName:    pbtypes.String("Relation-That-Will-Be-Deleted"),
			bundle.RelationKeySpaceId: pbtypes.String(spaceId),
		}})

		checksums := fx.getLatestChecksums(true)
		checksums.MarketplaceForceReindexCounter = checksums.MarketplaceForceReindexCounter - 1

		err := fx.objectStore.SaveChecksums(spaceId, &checksums)
		require.NoError(t, err)

		storage := mock_storage.NewMockClientStorage(t)
		storage.EXPECT().BindSpaceID(mock.Anything, mock.Anything).Return(nil)
		fx.storageService = storage

		fx.sourceFx.EXPECT().IDsListerBySmartblockType(mock.Anything, mock.Anything).Return(idsLister{Ids: []string{}}, nil).Maybe()

		// when
		err = fx.ReindexMarketplaceSpace(getMockSpace(fx))
		assert.NoError(t, err)

		// then
		det, err := fx.store.GetDetails("relationThatWillBeDeleted")
		assert.NoError(t, err)
		assert.Empty(t, det.Details.Fields)
	})
}

func TestReindexDeletedObjects(t *testing.T) {
	const (
		spaceId1 = "spaceId1"
		spaceId2 = "spaceId2"
		spaceId3 = "spaceId3"
	)
	fx := NewIndexerFixture(t)

	go fx.spaceReindexQueue.Run(context.Background())
	defer fx.spaceReindexQueue.WaitAndClose()
	fx.objectStore.AddObjects(t, []objectstore.TestObject{
		{
			bundle.RelationKeyId:        pbtypes.String("1"),
			bundle.RelationKeyIsDeleted: pbtypes.Bool(true),
		},
		{
			bundle.RelationKeyId:        pbtypes.String("2"),
			bundle.RelationKeyIsDeleted: pbtypes.Bool(true),
		},
		{
			bundle.RelationKeyId:        pbtypes.String("3"),
			bundle.RelationKeyIsDeleted: pbtypes.Bool(true),
			bundle.RelationKeySpaceId:   pbtypes.String(spaceId3),
		},
		{
			bundle.RelationKeyId: pbtypes.String("4"),
		},
	})

	checksums := fx.getLatestChecksums(false)
	checksums.AreDeletedObjectsReindexed = false

	err := fx.objectStore.SaveChecksums(spaceId1, &checksums)
	require.NoError(t, err)
	err = fx.objectStore.SaveChecksums(spaceId2, &checksums)
	require.NoError(t, err)

	t.Run("reindex first space", func(t *testing.T) {
		storage1 := mock_spacestorage.NewMockSpaceStorage(gomock.NewController(t))
		storage1.EXPECT().TreeDeletedStatus("1").Return(spacestorage.TreeDeletedStatusDeleted, nil)
		storage1.EXPECT().TreeDeletedStatus("2").Return("", nil)
		space1 := mock_space.NewMockSpace(t)
		space1.EXPECT().Id().Return(spaceId1)
		space1.EXPECT().Storage().Return(storage1)
		space1.EXPECT().StoredIds().Return([]string{}).Maybe()

		fx.sourceFx.EXPECT().IDsListerBySmartblockType(mock.Anything, mock.Anything).Return(idsLister{Ids: []string{}}, nil)

		err = fx.ReindexSpace(space1)
		require.NoError(t, err)

		sums, err := fx.objectStore.GetChecksums(spaceId1)
		require.NoError(t, err)

		assert.True(t, sums.AreDeletedObjectsReindexed)
	})

	t.Run("reindex second space", func(t *testing.T) {
		storage2 := mock_spacestorage.NewMockSpaceStorage(gomock.NewController(t))
		storage2.EXPECT().TreeDeletedStatus("2").Return(spacestorage.TreeDeletedStatusDeleted, nil)
		space2 := mock_space.NewMockSpace(t)
		space2.EXPECT().Id().Return(spaceId2)
		space2.EXPECT().Storage().Return(storage2)
		space2.EXPECT().StoredIds().Return([]string{}).Maybe()
		fx.sourceFx.EXPECT().IDsListerBySmartblockType(mock.Anything, mock.Anything).Return(idsLister{Ids: []string{}}, nil)

		err = fx.ReindexSpace(space2)
		fx.spaceReindexQueue.RefreshPriority()
		require.NoError(t, err)

		time.Sleep(4 * time.Second)
		sums, err := fx.objectStore.GetChecksums(spaceId2)
		require.NoError(t, err)

		assert.True(t, sums.AreDeletedObjectsReindexed)
	})

	got := fx.queryDeletedObjectIds(t, spaceId1)
	assert.Equal(t, []string{"1"}, got)

	got = fx.queryDeletedObjectIds(t, spaceId2)
	assert.Equal(t, []string{"2"}, got)

	got = fx.queryDeletedObjectIds(t, spaceId3)
	assert.Equal(t, []string{"3"}, got)
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

	fx.objectStore.AddObjects(t, []objectstore.TestObject{
		{
			bundle.RelationKeyId:      pbtypes.String("fav1"),
			bundle.RelationKeySpaceId: pbtypes.String(spaceId1),
		},
		{
			bundle.RelationKeyId:      pbtypes.String("fav2"),
			bundle.RelationKeySpaceId: pbtypes.String(spaceId1),
		},
		{
			bundle.RelationKeyId:      pbtypes.String("trash1"),
			bundle.RelationKeySpaceId: pbtypes.String(spaceId1),
		},
		{
			bundle.RelationKeyId:      pbtypes.String("trash2"),
			bundle.RelationKeySpaceId: pbtypes.String(spaceId1),
		},
		{
			bundle.RelationKeyId:      pbtypes.String("obj1"),
			bundle.RelationKeySpaceId: pbtypes.String(spaceId2),
		},
		{
			bundle.RelationKeyId:      pbtypes.String("obj2"),
			bundle.RelationKeySpaceId: pbtypes.String(spaceId2),
		},
		{
			bundle.RelationKeyId:      pbtypes.String("obj3"),
			bundle.RelationKeySpaceId: pbtypes.String(spaceId2),
		},
	})

	checksums := fx.getLatestChecksums(false)
	checksums.LinksErase = checksums.LinksErase - 1

	err := fx.objectStore.SaveChecksums(spaceId1, &checksums)
	require.NoError(t, err)
	err = fx.objectStore.SaveChecksums(spaceId2, &checksums)
	require.NoError(t, err)

	t.Run("links from archive and home are deleted", func(t *testing.T) {
		// given
		favs := []string{"fav1", "fav2"}
		trash := []string{"trash1", "trash2"}
		err = fx.store.UpdateObjectLinks("home", favs)
		require.NoError(t, err)
		err = fx.store.UpdateObjectLinks("bin", trash)
		require.NoError(t, err)

		homeLinks, err := fx.store.GetOutboundLinksByID("home")
		require.Equal(t, favs, homeLinks)

		archiveLinks, err := fx.store.GetOutboundLinksByID("bin")
		require.Equal(t, trash, archiveLinks)

		space1 := mock_space.NewMockSpace(t)
		space1.EXPECT().Id().Return(spaceId1)
		space1.EXPECT().StoredIds().Return([]string{}).Maybe()

		// when
		err = fx.ReindexSpace(space1)
		assert.NoError(t, err)

		// then
		homeLinks, err = fx.store.GetOutboundLinksByID("home")
		assert.NoError(t, err)
		assert.Empty(t, homeLinks)

		archiveLinks, err = fx.store.GetOutboundLinksByID("bin")
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
		err = fx.store.UpdateObjectLinks("obj1", obj1links)
		require.NoError(t, err)
		err = fx.store.UpdateObjectLinks("obj2", obj2links)
		require.NoError(t, err)
		err = fx.store.UpdateObjectLinks("obj3", obj3links)
		require.NoError(t, err)

		storedObj1links, err := fx.store.GetOutboundLinksByID("obj1")
		require.Equal(t, obj1links, storedObj1links)
		storedObj2links, err := fx.store.GetOutboundLinksByID("obj2")
		require.Equal(t, obj2links, storedObj2links)
		storedObj3links, err := fx.store.GetOutboundLinksByID("obj3")
		require.Equal(t, obj3links, storedObj3links)

		space1 := mock_space.NewMockSpace(t)
		space1.EXPECT().Id().Return(spaceId2)
		space1.EXPECT().StoredIds().Return([]string{}).Maybe()

		// when
		err = fx.ReindexSpace(space1)
		assert.NoError(t, err)

		// then
		storedObj1links, err = fx.store.GetOutboundLinksByID("obj1")
		assert.NoError(t, err)
		assert.Empty(t, storedObj1links)
		storedObj2links, err = fx.store.GetOutboundLinksByID("obj2")
		assert.NoError(t, err)
		assert.Empty(t, storedObj2links)
		storedObj3links, err = fx.store.GetOutboundLinksByID("obj3")
		assert.NoError(t, err)
		assert.Empty(t, storedObj3links)

		storeChecksums, err := fx.store.GetChecksums(spaceId2)
		assert.NoError(t, err)
		assert.Equal(t, ForceLinksReindexCounter, storeChecksums.LinksErase)
	})
}

func (fx *IndexerFixture) queryDeletedObjectIds(t *testing.T, spaceId string) []string {
	ids, _, err := fx.objectStore.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(spaceId),
			},
			{
				RelationKey: bundle.RelationKeyIsDeleted.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Bool(true),
			},
		},
	})
	require.NoError(t, err)
	return ids
}

func TestReindex_addSyncRelations(t *testing.T) {
	t.Run("addSyncRelations local only", func(t *testing.T) {
		// given
		const spaceId1 = "spaceId1"
		fx := NewIndexerFixture(t)

		fx.objectStore.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:        pbtypes.String("1"),
				bundle.RelationKeyIsDeleted: pbtypes.Bool(true),
			},
			{
				bundle.RelationKeyId:        pbtypes.String("2"),
				bundle.RelationKeyIsDeleted: pbtypes.Bool(true),
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

		space1.EXPECT().DoLockedIfNotExists("1", mock.AnythingOfType("func() error")).Return(nil)
		space1.EXPECT().DoLockedIfNotExists("2", mock.AnythingOfType("func() error")).Return(nil)

		// when
		err := fx.ReindexSpace(space1)

		// then
		require.NoError(t, err)
	})

	t.Run("addSyncRelations", func(t *testing.T) {
		// given
		const spaceId1 = "spaceId1"
		fx := NewIndexerFixture(t)

		fx.objectStore.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:        pbtypes.String("1"),
				bundle.RelationKeyIsDeleted: pbtypes.Bool(true),
			},
			{
				bundle.RelationKeyId:        pbtypes.String("2"),
				bundle.RelationKeyIsDeleted: pbtypes.Bool(true),
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

		space1.EXPECT().DoLockedIfNotExists("1", mock.AnythingOfType("func() error")).Return(nil)
		space1.EXPECT().DoLockedIfNotExists("2", mock.AnythingOfType("func() error")).Return(nil)

		fx.config.NetworkMode = pb.RpcAccount_DefaultConfig

		// when
		err := fx.ReindexSpace(space1)

		// then
		require.NoError(t, err)
	})
}

type idsLister struct {
	Ids []string
}

func (l idsLister) ListIds() ([]string, error) {
	return l.Ids, nil
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestReindexTaskIdParse(t *testing.T) {
	testCases := []struct {
		input            string
		expectedSpaceId  string
		expectedRetryNum int
	}{
		{"space1", "space1", 0},
		{"space1#1", "space1", 1},
		{"space2#2", "space2", 2},
		{"space3#abc", "space3", 0}, // Invalid retry number
		{"space4#", "space4", 0},    // Empty retry number
		{"", "", 0},
	}

	for _, tc := range testCases {
		spaceId, retryNum := reindexTaskId(tc.input).Parse()
		if spaceId != tc.expectedSpaceId || retryNum != tc.expectedRetryNum {
			t.Errorf("Parse(%q) = (%q, %d); want (%q, %d)", tc.input, spaceId, retryNum, tc.expectedSpaceId, tc.expectedRetryNum)
		}
	}
}

func TestTaskPrioritySorter(t *testing.T) {
	i := &indexer{
		spacesPriority: []string{"space1", "space2", "space3"},
	}

	testCases := []struct {
		name     string
		taskIds  []string
		expected []string
	}{
		{
			name: "Different retry attempts",
			taskIds: []string{
				"space1#2",
				"space2#1",
				"space3#3",
				"space4#0",
				"space5#1",
			},
			expected: []string{
				"space4#0", // try=0, space not in priority list
				"space2#1", // try=1, index=1
				"space5#1", // try=1, index=-1 (space5 not in priority)
				"space1#2", // try=2, index=0
				"space3#3", // try=3, index=2
			},
		},
		{
			name: "Same retry attempts, different priorities",
			taskIds: []string{
				"space3#1",
				"space1#1",
				"space4#1",
				"space2#1",
			},
			expected: []string{
				"space1#1", // index=0
				"space2#1", // index=1
				"space3#1", // index=2
				"space4#1", // index=-1
			},
		},
		{
			name: "Spaces not in priority list",
			taskIds: []string{
				"space4#0",
				"space5#0",
				"space6#0",
			},
			expected: []string{
				"space4#0",
				"space5#0",
				"space6#0",
			}, // Should be sorted alphabetically among themselves
		},
		{
			name: "Mixed retry attempts and priorities",
			taskIds: []string{
				"space2#0",
				"space4#0",
				"space1#1",
				"space5#1",
				"space3#0",
			},
			expected: []string{
				"space2#0", // try=0, index=1
				"space3#0", // try=0, index=2
				"space4#0", // try=0, index=-1
				"space1#1", // try=1, index=0
				"space5#1", // try=1, index=-1
			},
		},
		{
			name: "Tasks without retries",
			taskIds: []string{
				"space3",
				"space2",
				"space4",
				"space1",
			},
			expected: []string{
				"space1", // try=0, index=0
				"space2", // try=0, index=1
				"space3", // try=0, index=2
				"space4", // try=0, index=-1
			},
		},
		{
			name: "Equal tries and no priority",
			taskIds: []string{
				"space4#1",
				"space5#1",
				"space6#1",
			},
			expected: []string{
				"space4#1",
				"space5#1",
				"space6#1",
			}, // Should be sorted alphabetically among themselves
		},
		{
			name: "Complex mix",
			taskIds: []string{
				"space3#2",
				"space1#0",
				"space4#0",
				"space2#2",
				"space5#1",
				"space2#0",
				"space1#1",
			},
			expected: []string{
				"space1#0", // try=0, index=0
				"space2#0", // try=0, index=1
				"space4#0", // try=0, index=-1
				"space1#1", // try=1, index=0
				"space5#1", // try=1, index=-1
				"space2#2", // try=2, index=1
				"space3#2", // try=2, index=2
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			taskIdsCopy := make([]string, len(tc.taskIds))
			copy(taskIdsCopy, tc.taskIds)
			i.taskPrioritySorter(taskIdsCopy)
			if !slicesEqual(taskIdsCopy, tc.expected) {
				t.Errorf("taskPrioritySorter(%v) = %v; want %v", tc.taskIds, taskIdsCopy, tc.expected)
			}
		})
	}
}
