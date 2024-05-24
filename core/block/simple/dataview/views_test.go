package dataview

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const testViewId = "viewId"

func makeDataviewForViewRelationsTest(relationLinks []*model.RelationLink, relations []*model.BlockContentDataviewRelation) Block {
	return NewDataview(&model.Block{
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
	}).(Block)
}

func TestReorderViewRelations(t *testing.T) {
	t.Run("reorder: add missing relation from relation links", func(t *testing.T) {
		dv := makeDataviewForViewRelationsTest(
			[]*model.RelationLink{
				{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
				{Key: bundle.RelationKeyCreator.String(), Format: model.RelationFormat_object},
				{Key: bundle.RelationKeyCreatedDate.String(), Format: model.RelationFormat_date},
			},
			[]*model.BlockContentDataviewRelation{
				{Key: bundle.RelationKeyName.String(), IsVisible: true, Width: DefaultViewRelationWidth},
			},
		)

		err := dv.ReorderViewRelations(testViewId, []string{bundle.RelationKeyCreator.String(), bundle.RelationKeyName.String()})
		require.NoError(t, err)

		want := makeDataviewForViewRelationsTest(
			[]*model.RelationLink{
				{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
				{Key: bundle.RelationKeyCreator.String(), Format: model.RelationFormat_object},
				{Key: bundle.RelationKeyCreatedDate.String(), Format: model.RelationFormat_date},
			},
			[]*model.BlockContentDataviewRelation{
				{Key: bundle.RelationKeyCreator.String(), IsVisible: false, Width: DefaultViewRelationWidth},
				{Key: bundle.RelationKeyName.String(), IsVisible: true, Width: DefaultViewRelationWidth},
				{Key: bundle.RelationKeyCreatedDate.String(), IsVisible: false, Width: DefaultViewRelationWidth},
			},
		)

		assert.Equal(t, want, dv)
	})

	t.Run("reorder: remove extra relation that don't exist in relation links", func(t *testing.T) {
		dv := makeDataviewForViewRelationsTest(
			[]*model.RelationLink{
				{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
				{Key: bundle.RelationKeyCreatedDate.String(), Format: model.RelationFormat_date},
			},
			[]*model.BlockContentDataviewRelation{
				{Key: bundle.RelationKeyCreator.String(), IsVisible: false, Width: DefaultViewRelationWidth},
				{Key: bundle.RelationKeyName.String(), IsVisible: true, Width: DefaultViewRelationWidth},
				{Key: bundle.RelationKeyCreatedDate.String(), IsVisible: false, Width: DefaultViewRelationWidth},
			},
		)

		err := dv.ReorderViewRelations(testViewId, []string{bundle.RelationKeyName.String(), bundle.RelationKeyCreator.String(), bundle.RelationKeyDescription.String()})
		require.NoError(t, err)

		want := makeDataviewForViewRelationsTest(
			[]*model.RelationLink{
				{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
				{Key: bundle.RelationKeyCreatedDate.String(), Format: model.RelationFormat_date},
			},
			[]*model.BlockContentDataviewRelation{
				{Key: bundle.RelationKeyName.String(), IsVisible: true, Width: DefaultViewRelationWidth},
				{Key: bundle.RelationKeyCreatedDate.String(), IsVisible: false, Width: DefaultViewRelationWidth},
			},
		)

		assert.Equal(t, want, dv)
	})
}

func TestReplaceViewRelation(t *testing.T) {
	t.Run("add new relation", func(t *testing.T) {
		dv := makeDataviewForViewRelationsTest(
			[]*model.RelationLink{
				{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
			},
			[]*model.BlockContentDataviewRelation{},
		)

		err := dv.ReplaceViewRelation(testViewId, bundle.RelationKeyDescription.String(), &model.BlockContentDataviewRelation{
			Key:       bundle.RelationKeyDescription.String(),
			Width:     DefaultViewRelationWidth,
			IsVisible: true,
		})
		require.NoError(t, err)

		want := makeDataviewForViewRelationsTest(
			[]*model.RelationLink{
				{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
			},
			[]*model.BlockContentDataviewRelation{
				// Added automatically from relation links
				{
					Key:       bundle.RelationKeyName.String(),
					Width:     DefaultViewRelationWidth,
					IsVisible: false,
				},
				{
					Key:       bundle.RelationKeyDescription.String(),
					Width:     DefaultViewRelationWidth,
					IsVisible: true,
				},
			},
		)

		assert.Equal(t, want, dv)
	})
	t.Run("replace existing", func(t *testing.T) {
		dv := makeDataviewForViewRelationsTest(
			[]*model.RelationLink{
				{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
			},
			[]*model.BlockContentDataviewRelation{
				// Added automatically from relation links
				{
					Key:       bundle.RelationKeyName.String(),
					Width:     DefaultViewRelationWidth,
					IsVisible: false,
				},
				{
					Key:       bundle.RelationKeyDescription.String(),
					Width:     DefaultViewRelationWidth,
					IsVisible: true,
				},
			},
		)

		err := dv.ReplaceViewRelation(testViewId, bundle.RelationKeyDescription.String(), &model.BlockContentDataviewRelation{
			Key:       bundle.RelationKeyAssignee.String(),
			Width:     DefaultViewRelationWidth,
			IsVisible: true,
		})
		require.NoError(t, err)

		want := makeDataviewForViewRelationsTest(
			[]*model.RelationLink{
				{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
			},
			[]*model.BlockContentDataviewRelation{
				// Added automatically from relation links
				{
					Key:       bundle.RelationKeyName.String(),
					Width:     DefaultViewRelationWidth,
					IsVisible: false,
				},
				{
					Key:       bundle.RelationKeyAssignee.String(),
					Width:     DefaultViewRelationWidth,
					IsVisible: true,
				},
			},
		)

		assert.Equal(t, want, dv)
	})
	t.Run("add relation that exist in relation links, but not in View", func(t *testing.T) {
		dv := makeDataviewForViewRelationsTest(
			[]*model.RelationLink{
				{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
			},
			[]*model.BlockContentDataviewRelation{},
		)

		err := dv.ReplaceViewRelation(testViewId, bundle.RelationKeyName.String(), &model.BlockContentDataviewRelation{
			Key:   bundle.RelationKeyName.String(),
			Width: DefaultViewRelationWidth,
		})
		require.NoError(t, err)

		want := makeDataviewForViewRelationsTest(
			[]*model.RelationLink{
				{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
			},
			[]*model.BlockContentDataviewRelation{
				{
					Key:   bundle.RelationKeyName.String(),
					Width: DefaultViewRelationWidth,
				},
			},
		)

		assert.Equal(t, want, dv)
	})
}
