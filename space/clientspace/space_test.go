package clientspace

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/commonspace/mock_commonspace"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache/mock_objectcache"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func Test_migrateRelationOptions(t *testing.T) {
	t.Run("no relation options", func(t *testing.T) {
		// given
		s := space{}

		mockSpace := mock_commonspace.NewMockSpace(gomock.NewController(t))
		mockSpace.EXPECT().Id().Return("spaceId")
		s.common = mockSpace
		// when
		err := s.migrateRelationOptions(objectstore.NewStoreFixture(t))

		// then
		assert.Nil(t, err)
	})
	t.Run("relation options with not tag relation", func(t *testing.T) {
		// given
		s := space{}
		mockSpace := mock_commonspace.NewMockSpace(gomock.NewController(t))
		mockSpace.EXPECT().Id().Return("spaceId")
		s.common = mockSpace

		// when
		storeFixture := objectstore.NewStoreFixture(t)
		storeFixture.AddObjects(t, "spaceId", []objectstore.TestObject{
			{
				bundle.RelationKeyId:          pbtypes.String("id1"),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relationOption)),
				bundle.RelationKeyRelationKey: pbtypes.String("key"),
				bundle.RelationKeySpaceId:     pbtypes.String("spaceId"),
			},
			{
				bundle.RelationKeyId:          pbtypes.String("id2"),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relationOption)),
				bundle.RelationKeyRelationKey: pbtypes.String("key"),
				bundle.RelationKeySpaceId:     pbtypes.String("spaceId"),
			},
		})
		err := s.migrateRelationOptions(storeFixture)

		// then
		assert.Nil(t, err)
	})
	t.Run("tag relation options", func(t *testing.T) {
		// given
		s := space{}
		mockSpace := mock_commonspace.NewMockSpace(gomock.NewController(t))
		mockSpace.EXPECT().Id().Return("spaceId")
		s.common = mockSpace

		// when
		storeFixture := objectstore.NewStoreFixture(t)
		storeFixture.AddObjects(t, "spaceId", []objectstore.TestObject{
			{
				bundle.RelationKeyId:          pbtypes.String("id1"),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relationOption)),
				bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyTag.String()),
				bundle.RelationKeySpaceId:     pbtypes.String("spaceId"),
			},
			{
				bundle.RelationKeyId:          pbtypes.String("id2"),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relationOption)),
				bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyTag.String()),
				bundle.RelationKeySpaceId:     pbtypes.String("spaceId"),
			},
		})

		cache := mock_objectcache.NewMockCache(t)
		cache.EXPECT().GetObject(context.Background(), "id1").Return(smarttest.New("id1"), nil)
		cache.EXPECT().GetObject(context.Background(), "id2").Return(smarttest.New("id1"), nil)
		s.Cache = cache

		err := s.migrateRelationOptions(storeFixture)

		// then
		assert.Nil(t, err)
	})
}

func Test_migrateTag(t *testing.T) {
	t.Run("no relations", func(t *testing.T) {
		// given
		s := space{}

		mockSpace := mock_commonspace.NewMockSpace(gomock.NewController(t))
		mockSpace.EXPECT().Id().Return("spaceId")
		s.common = mockSpace
		// when
		err := s.migrateTag(objectstore.NewStoreFixture(t))

		// then
		assert.Nil(t, err)
	})
	t.Run("not tag relation", func(t *testing.T) {
		// given
		s := space{}
		mockSpace := mock_commonspace.NewMockSpace(gomock.NewController(t))
		mockSpace.EXPECT().Id().Return("spaceId")
		s.common = mockSpace

		// when
		storeFixture := objectstore.NewStoreFixture(t)
		storeFixture.AddObjects(t, "spaceId", []objectstore.TestObject{
			{
				bundle.RelationKeyId:        pbtypes.String("id1"),
				bundle.RelationKeyLayout:    pbtypes.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyUniqueKey: pbtypes.String("key"),
				bundle.RelationKeySpaceId:   pbtypes.String("spaceId"),
			},
		})
		err := s.migrateTag(storeFixture)

		// then
		assert.Nil(t, err)
	})
	t.Run("migrate tag", func(t *testing.T) {
		// given
		s := space{}
		mockSpace := mock_commonspace.NewMockSpace(gomock.NewController(t))
		mockSpace.EXPECT().Id().Return("spaceId")
		s.common = mockSpace

		// when
		storeFixture := objectstore.NewStoreFixture(t)
		storeFixture.AddObjects(t, "spaceId", []objectstore.TestObject{
			{
				bundle.RelationKeyId:             pbtypes.String("id1"),
				bundle.RelationKeyLayout:         pbtypes.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyUniqueKey:      pbtypes.String(bundle.RelationKeyTag.URL()),
				bundle.RelationKeySpaceId:        pbtypes.String("spaceId"),
				bundle.RelationKeyRelationFormat: pbtypes.Int64(int64(model.RelationFormat_tag)),
			},
		})

		cache := mock_objectcache.NewMockCache(t)
		cache.EXPECT().GetObject(context.Background(), "id1").Return(smarttest.New("id1"), nil)
		s.Cache = cache

		err := s.migrateTag(storeFixture)

		// then
		assert.Nil(t, err)
	})
}
