package dataviewservice

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/object/idresolver/mock_idresolver"
	dvblock "github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	spaceId    = "spc1"
	objectId   = "obj1"
	blockId    = "dataview"
	testViewId = "view1"
)

type fixture struct {
	store      *objectstore.StoreFixture
	idResolver *mock_idresolver.MockResolver
	*service
}

func newFixture(t *testing.T) *fixture {
	store := objectstore.NewStoreFixture(t)
	idResolver := mock_idresolver.NewMockResolver(t)
	return &fixture{
		store:      store,
		idResolver: idResolver,
		service: &service{
			objectStore: store,
			idResolver:  idResolver,
		},
	}
}

func TestService_syncViewRelationsAndRelationLinks(t *testing.T) {
	t.Run("relations are synced", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.idResolver.EXPECT().ResolveSpaceID(objectId).Return(spaceId, nil)
		dv := makeDataviewForViewRelationsTest([]*model.RelationLink{
			{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
			{Key: bundle.RelationKeyBacklinks.String(), Format: model.RelationFormat_object},
			// relation links do not include CreatedDate and Type, so they should be inserted using objectStore
		}, []*model.BlockContentDataviewRelation{
			{Key: bundle.RelationKeyName.String(), IsVisible: true, Width: dvblock.DefaultViewRelationWidth},
			{Key: bundle.RelationKeyCreatedDate.String(), IsVisible: true, Width: dvblock.DefaultViewRelationWidth},
			{Key: bundle.RelationKeyType.String(), IsVisible: false, Width: dvblock.DefaultViewRelationWidth},
			// view relations do not include Backlinks, so it should be inserted with default settings
		})

		want := makeDataviewForViewRelationsTest([]*model.RelationLink{
			{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
			{Key: bundle.RelationKeyBacklinks.String(), Format: model.RelationFormat_object},
			{Key: bundle.RelationKeyCreatedDate.String(), Format: model.RelationFormat_date},
			{Key: bundle.RelationKeyType.String(), Format: model.RelationFormat_object},
		}, []*model.BlockContentDataviewRelation{
			{Key: bundle.RelationKeyName.String(), IsVisible: true, Width: dvblock.DefaultViewRelationWidth},
			{Key: bundle.RelationKeyCreatedDate.String(), IsVisible: true, Width: dvblock.DefaultViewRelationWidth},
			{Key: bundle.RelationKeyType.String(), IsVisible: false, Width: dvblock.DefaultViewRelationWidth},
			{Key: bundle.RelationKeyBacklinks.String(), IsVisible: false, Width: dvblock.DefaultViewRelationWidth},
		})

		// when
		fx.syncViewRelationsAndRelationLinks(objectId, testViewId, dv)

		// then
		assert.Equal(t, want, dv)
	})
}

func makeDataviewForViewRelationsTest(relationLinks []*model.RelationLink, relations []*model.BlockContentDataviewRelation) dvblock.Block {
	return dvblock.NewDataview(&model.Block{
		Id: blockId,
		Content: &model.BlockContentOfDataview{
			Dataview: &model.BlockContentDataview{
				RelationLinks: relationLinks,
				Views: []*model.BlockContentDataviewView{
					{
						Id:        testViewId,
						Relations: relations,
					},
				},
			},
		},
	}).(dvblock.Block)
}
