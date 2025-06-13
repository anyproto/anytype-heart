package dataviewservice

import (
	"testing"

	"github.com/stretchr/testify/assert"

	dvblock "github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	blockId    = "dataview"
	testViewId = "view1"
)

func TestService_syncViewRelationsAndRelationLinks(t *testing.T) {
	t.Run("relations are synced", func(t *testing.T) {
		// given
		dv := makeDataviewForViewRelationsTest([]*model.RelationLink{
			{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
			{Key: bundle.RelationKeyBacklinks.String(), Format: model.RelationFormat_object},
			// relation links do not include CreatedDate and Type, but it is OK!
		}, []*model.BlockContentDataviewRelation{
			{Key: bundle.RelationKeyName.String(), IsVisible: true, Width: dvblock.DefaultViewRelationWidth},
			{Key: bundle.RelationKeyCreatedDate.String(), IsVisible: true, Width: dvblock.DefaultViewRelationWidth},
			{Key: bundle.RelationKeyType.String(), IsVisible: false, Width: dvblock.DefaultViewRelationWidth},
			// view relations do not include Backlinks, so it should be inserted with default settings
		})

		want := makeDataviewForViewRelationsTest([]*model.RelationLink{
			{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
			{Key: bundle.RelationKeyBacklinks.String(), Format: model.RelationFormat_object},
		}, []*model.BlockContentDataviewRelation{
			{Key: bundle.RelationKeyName.String(), IsVisible: true, Width: dvblock.DefaultViewRelationWidth},
			{Key: bundle.RelationKeyCreatedDate.String(), IsVisible: true, Width: dvblock.DefaultViewRelationWidth},
			{Key: bundle.RelationKeyType.String(), IsVisible: false, Width: dvblock.DefaultViewRelationWidth},
			{Key: bundle.RelationKeyBacklinks.String(), IsVisible: false, Width: dvblock.DefaultViewRelationWidth},
		})

		// when
		syncViewRelationsAndRelationLinks(testViewId, dv)

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
