package indexer

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage/mock_spacestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache/mock_objectcache"
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
	t.Run("reindex missing object", func(t *testing.T) {
		// given
		indexerFx := NewIndexerFixture(t)
		checksums := indexerFx.getLatestChecksums()
		err := indexerFx.store.SaveChecksums("spaceId", &checksums)
		assert.Nil(t, err)

		virtualSpace := clientspace.NewVirtualSpace("spaceId", clientspace.VirtualSpaceDeps{
			Indexer: indexerFx,
		})
		mockCache := mock_objectcache.NewMockCache(t)
		smartTest := smarttest.New(addr.MissingObject)
		smartTest.SetSpace(virtualSpace)

		smartTest.SetType(coresb.SmartBlockTypePage)
		smartTest.SetSpaceId("spaceId")
		mockCache.EXPECT().GetObject(context.Background(), addr.MissingObject).Return(editor.NewMissingObject(smartTest), nil)
		mockCache.EXPECT().GetObject(context.Background(), addr.AnytypeProfileId).Return(smartTest, nil)
		virtualSpace.Cache = mockCache

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
}

func TestReindexDeletedObjects(t *testing.T) {
	const (
		spaceId1 = "spaceId1"
		spaceId2 = "spaceId2"
		spaceId3 = "spaceId3"
	)
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
		{
			bundle.RelationKeyId:        pbtypes.String("3"),
			bundle.RelationKeyIsDeleted: pbtypes.Bool(true),
			bundle.RelationKeySpaceId:   pbtypes.String(spaceId3),
		},
		{
			bundle.RelationKeyId: pbtypes.String("4"),
		},
	})

	checksums := fx.getLatestChecksums()
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
		space1.EXPECT().StoredIds().Return([]string{})

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
		space2.EXPECT().StoredIds().Return([]string{})

		err = fx.ReindexSpace(space2)
		require.NoError(t, err)

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
