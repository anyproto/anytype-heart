package template

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func assertDataviewBlock(
	t *testing.T,
	block *model.BlockContentOfDataview,
	isCollection bool,
	expectedRelations []domain.RelationKey,
	isVisible func(key domain.RelationKey) bool,
) {
	assert.Equal(t, isCollection, block.Dataview.IsCollection)
	assert.Len(t, block.Dataview.RelationLinks, len(expectedRelations))
	for i, link := range block.Dataview.RelationLinks {
		assert.Equal(t, expectedRelations[i], domain.RelationKey(link.Key))
	}
	assert.Len(t, block.Dataview.Views, 1)
	assert.Len(t, block.Dataview.Views[0].Relations, len(expectedRelations))
	for i, relation := range block.Dataview.Views[0].Relations {
		assert.Equal(t, expectedRelations[i], domain.RelationKey(relation.Key))
		assert.Equal(t, isVisible(domain.RelationKey(relation.Key)), relation.IsVisible)
	}
}

func TestMakeDataviewContent(t *testing.T) {
	for _, tc := range []struct {
		name              string
		isCollection      bool
		ot                *model.ObjectType
		relLinks          []*model.RelationLink
		expectedRelations []domain.RelationKey
		isVisible         func(key domain.RelationKey) bool
	}{
		{
			name:              "collection",
			isCollection:      true,
			expectedRelations: defaultCollectionRelations,
			isVisible: func(key domain.RelationKey) bool {
				return slices.Contains(defaultVisibleRelations, key)
			},
		},
		{
			name: "set by object type",
			ot: &model.ObjectType{
				RelationLinks: []*model.RelationLink{
					{Key: bundle.RelationKeyMentions.String()},
					{Key: bundle.RelationKeyLinkedProjects.String()},
					{Key: bundle.RelationKeyAssignee.String()},
				},
			},
			expectedRelations: append(defaultDataviewRelations, []domain.RelationKey{
				bundle.RelationKeyMentions,
				bundle.RelationKeyLinkedProjects,
				bundle.RelationKeyAssignee,
			}...),
			isVisible: func(key domain.RelationKey) bool {
				return slices.Contains(defaultVisibleRelations, key)
			},
		},
		{
			name: "set by relation",
			relLinks: []*model.RelationLink{
				{Key: bundle.RelationKeyAddedDate.String()},
				{Key: bundle.RelationKeyLastUsedDate.String()},
			},
			expectedRelations: append(defaultDataviewRelations, []domain.RelationKey{
				bundle.RelationKeyAddedDate,
				bundle.RelationKeyLastUsedDate,
			}...),
			isVisible: func(key domain.RelationKey) bool {
				return slices.Contains(append(defaultVisibleRelations, []domain.RelationKey{
					bundle.RelationKeyAddedDate,
					bundle.RelationKeyLastUsedDate,
				}...), key)
			},
		},
		{
			name:              "empty",
			expectedRelations: defaultDataviewRelations,
			isVisible: func(key domain.RelationKey) bool {
				return slices.Contains(defaultVisibleRelations, key)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			block := MakeDataviewContent(tc.isCollection, tc.ot, tc.relLinks, "")
			assertDataviewBlock(t, block, tc.isCollection, tc.expectedRelations, tc.isVisible)
		})
	}
}
