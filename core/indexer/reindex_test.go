package indexer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache/mock_objectcache"
	"github.com/anyproto/anytype-heart/core/indexer/mock_indexer"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/mock_storage"
)

func TestReindexMarketplaceSpace(t *testing.T) {
	t.Run("reindex missing object", func(t *testing.T) {
		// given
		indexerFx := NewIndexerFixture(t)
		mockHasher := mock_indexer.NewMockHasher(t)
		mockHasher.EXPECT().Hash().Return("")
		indexerFx.btHash = mockHasher
		checksums := &model.ObjectStoreChecksums{
			BundledObjectTypes:               bundle.TypeChecksum,
			BundledRelations:                 bundle.RelationChecksum,
			BundledTemplates:                 indexerFx.btHash.Hash(),
			ObjectsForceReindexCounter:       ForceObjectsReindexCounter,
			FilesForceReindexCounter:         ForceFilesReindexCounter,
			IdxRebuildCounter:                ForceIdxRebuildCounter,
			FilestoreKeysForceReindexCounter: ForceFilestoreKeysReindexCounter,
			AreOldFilesRemoved:               true,
		}
		err := indexerFx.store.SaveChecksums("spaceId", checksums)
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
