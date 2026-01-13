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

func makeDataviewRelation(key domain.RelationKey, isVisible bool) *model.BlockContentDataviewRelation {
	rel := bundle.MustGetRelation(key)

	return &model.BlockContentDataviewRelation{
		Key:       string(key),
		IsVisible: isVisible,
		Width:     propertyWidth(rel.Format),
	}
}

func makeRelationLinks(keys []domain.RelationKey) []*model.RelationLink {
	res := make([]*model.RelationLink, 0, len(keys))
	for _, key := range keys {
		rel := bundle.MustGetRelation(key)
		res = append(res, &model.RelationLink{
			Key:    rel.Key,
			Format: rel.Format,
		})
	}
	return res
}

func makeDataviewRelations(keys []domain.RelationKey, visible []domain.RelationKey) []*model.BlockContentDataviewRelation {
	res := make([]*model.BlockContentDataviewRelation, 0, len(keys))
	for _, key := range keys {
		res = append(res, makeDataviewRelation(key, slices.Contains(visible, key)))
	}
	return res
}

func TestMakeDataviewContentNew(t *testing.T) {
	for _, tc := range []struct {
		name         string
		isCollection bool
		ot           *model.ObjectType
		relLinks     []*model.RelationLink
		want         *model.BlockContentDataview
	}{
		{
			name:         "collection",
			isCollection: true,
			want: &model.BlockContentDataview{
				IsCollection: true,
				Views: []*model.BlockContentDataviewView{
					{
						Type: DefaultViewLayout,
						Name: defaultViewName,
						Sorts: []*model.BlockContentDataviewSort{
							{
								RelationKey: bundle.RelationKeyName.String(),
								Type:        model.BlockContentDataviewSort_Asc,
							},
						},
						Relations: makeDataviewRelations(defaultCollectionRelations, defaultVisibleRelations),
					},
				},
				RelationLinks: makeRelationLinks(defaultCollectionRelations),
			},
		},
		{
			name: "query by object type",
			ot: &model.ObjectType{
				RelationLinks: []*model.RelationLink{
					{Key: bundle.RelationKeyMentions.String()},
					{Key: bundle.RelationKeyLinkedProjects.String()},
					{Key: bundle.RelationKeyAssignee.String()},
				},
			},
			want: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{
						Type: DefaultViewLayout,
						Name: defaultViewName,
						Sorts: []*model.BlockContentDataviewSort{
							{
								RelationKey: bundle.RelationKeyLastModifiedDate.String(),
								Type:        model.BlockContentDataviewSort_Desc,
							},
						},
						Relations: makeDataviewRelations(append(defaultDataviewRelations, bundle.RelationKeyMentions, bundle.RelationKeyLinkedProjects, bundle.RelationKeyAssignee), defaultVisibleRelations),
					},
				},
				RelationLinks: makeRelationLinks(append(defaultDataviewRelations, bundle.RelationKeyMentions, bundle.RelationKeyLinkedProjects, bundle.RelationKeyAssignee)),
			},
		},
		{
			name: "query by object type: chats",
			ot: &model.ObjectType{
				Key: bundle.TypeKeyChatDerived.String(),
			},
			want: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{
						Type: DefaultViewLayout,
						Name: defaultViewName,
						Sorts: []*model.BlockContentDataviewSort{
							{
								RelationKey: bundle.RelationKeyLastMessageDate.String(),
								Type:        model.BlockContentDataviewSort_Desc,
								Format:      model.RelationFormat_date,
								IncludeTime: true,
							},
						},
						Relations: makeDataviewRelations(defaultDataviewRelations, defaultVisibleRelations),
					},
				},
				RelationLinks: makeRelationLinks(defaultDataviewRelations),
			},
		},
		{
			name: "query by relations",
			relLinks: []*model.RelationLink{
				{Key: bundle.RelationKeyAddedDate.String()},
				{Key: bundle.RelationKeyLastUsedDate.String()},
			},
			want: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{
						Type: DefaultViewLayout,
						Name: defaultViewName,
						Sorts: []*model.BlockContentDataviewSort{
							{
								RelationKey: bundle.RelationKeyLastModifiedDate.String(),
								Type:        model.BlockContentDataviewSort_Desc,
							},
						},
						Relations: makeDataviewRelations(
							append(defaultDataviewRelations, bundle.RelationKeyAddedDate, bundle.RelationKeyLastUsedDate),
							append(defaultVisibleRelations, bundle.RelationKeyAddedDate, bundle.RelationKeyLastUsedDate),
						),
					},
				},
				RelationLinks: makeRelationLinks(
					append(defaultDataviewRelations, bundle.RelationKeyAddedDate, bundle.RelationKeyLastUsedDate)),
			},
		},
		{
			name: "empty",
			want: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{
						Type: DefaultViewLayout,
						Name: defaultViewName,
						Sorts: []*model.BlockContentDataviewSort{
							{
								RelationKey: bundle.RelationKeyLastModifiedDate.String(),
								Type:        model.BlockContentDataviewSort_Desc,
							},
						},
						Relations: makeDataviewRelations(defaultDataviewRelations, defaultVisibleRelations),
					},
				},
				RelationLinks: makeRelationLinks(defaultDataviewRelations),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := MakeDataviewContent(tc.isCollection, tc.ot, tc.relLinks, nil)

			// normalize
			for _, view := range got.Dataview.Views {
				view.Id = ""
				for _, sort := range view.Sorts {
					sort.Id = ""
				}
			}

			want := &model.BlockContentOfDataview{Dataview: tc.want}

			assert.Equal(t, want, got)
		})
	}
}

func TestBuildViewRelations(t *testing.T) {
	t.Run("empty parameters - dataview defaults", func(t *testing.T) {
		// when
		relations := BuildViewRelations(false, nil, nil)

		// then
		assert.Len(t, relations, len(defaultDataviewRelations))
		assert.Equal(t, bundle.RelationKeyName.String(), relations[0].Key)
		assert.True(t, relations[0].IsVisible)
		for i, expectedRel := range defaultDataviewRelations {
			assert.Equal(t, expectedRel.String(), relations[i].Key)
		}
	})

	t.Run("empty parameters - collection defaults", func(t *testing.T) {
		// when
		relations := BuildViewRelations(true, nil, nil)

		// then
		assert.Len(t, relations, len(defaultCollectionRelations))
		assert.Equal(t, bundle.RelationKeyName.String(), relations[0].Key)
		assert.True(t, relations[0].IsVisible)
		assert.Equal(t, bundle.RelationKeyType.String(), relations[1].Key)
		assert.True(t, relations[1].IsVisible)
		for i, expectedRel := range defaultCollectionRelations {
			assert.Equal(t, expectedRel.String(), relations[i].Key)
		}
	})

	t.Run("with additional relations - no duplicates", func(t *testing.T) {
		// given
		additionalRels := []*model.RelationLink{
			{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},   // Duplicate
			{Key: bundle.RelationKeyAssignee.String(), Format: model.RelationFormat_object}, // New
			{Key: bundle.RelationKeyDone.String(), Format: model.RelationFormat_checkbox},   // New
		}

		// when
		relations := BuildViewRelations(false, additionalRels, nil)

		// then
		expectedCount := len(defaultDataviewRelations) + 2
		assert.Len(t, relations, expectedCount)
		keys := make(map[string]bool)
		for _, rel := range relations {
			assert.False(t, keys[rel.Key], "Duplicate key found: %s", rel.Key)
			keys[rel.Key] = true
		}
		assert.True(t, keys[bundle.RelationKeyAssignee.String()])
		assert.True(t, keys[bundle.RelationKeyDone.String()])
	})

	t.Run("with visible relations specified", func(t *testing.T) {
		// given
		visibleRels := []domain.RelationKey{
			bundle.RelationKeyName,
			bundle.RelationKeyAssignee,
			bundle.RelationKeyDone,
		}

		additionalRels := []*model.RelationLink{
			{Key: bundle.RelationKeyAssignee.String(), Format: model.RelationFormat_object},
			{Key: bundle.RelationKeyDone.String(), Format: model.RelationFormat_checkbox},
		}

		// when
		relations := BuildViewRelations(false, additionalRels, visibleRels)

		// then
		for _, rel := range relations {
			expected := slices.Contains(visibleRels, domain.RelationKey(rel.Key))
			assert.Equal(t, expected, rel.IsVisible, "Relation %s visibility mismatch", rel.Key)
		}
	})

	t.Run("property width calculation", func(t *testing.T) {
		// given
		additionalRels := []*model.RelationLink{
			{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext}, // Long text -> 200
			{Key: bundle.RelationKeyDone.String(), Format: model.RelationFormat_checkbox}, // Checkbox -> 100
			{Key: bundle.RelationKeyTag.String(), Format: model.RelationFormat_tag},       // Tag -> 100
			{Key: bundle.RelationKeyPhone.String(), Format: model.RelationFormat_phone},   // Phone -> 100
		}

		// when
		relations := BuildViewRelations(false, additionalRels, nil)

		// then
		for _, rel := range relations {
			switch domain.RelationKey(rel.Key) {
			case bundle.RelationKeyName:
				assert.Equal(t, int32(200), rel.Width, "Name should have width 200")
			case bundle.RelationKeyDone:
				assert.Equal(t, int32(100), rel.Width, "Done should have width 100")
			case bundle.RelationKeyTag:
				assert.Equal(t, int32(100), rel.Width, "Tag should have width 100")
			case bundle.RelationKeyPhone:
				assert.Equal(t, int32(100), rel.Width, "Phone should have width 100")
			}
		}
	})
}

func TestCollectRelationLinksFromViews(t *testing.T) {
	t.Run("empty views", func(t *testing.T) {
		result := collectRelationLinksFromViews(nil)
		assert.Empty(t, result)
	})

	t.Run("single view with bundle relations", func(t *testing.T) {
		// given
		view := &model.BlockContentDataviewView{
			Relations: []*model.BlockContentDataviewRelation{
				{Key: bundle.RelationKeyName.String()},
				{Key: bundle.RelationKeyType.String()},
				{Key: bundle.RelationKeyCreatedDate.String()},
			},
		}

		// when
		result := collectRelationLinksFromViews(nil, view)

		// then
		assert.Len(t, result, 3)
		assert.Equal(t, bundle.RelationKeyName.String(), result[0].Key)
		assert.Equal(t, bundle.RelationKeyType.String(), result[1].Key)
		assert.Equal(t, bundle.RelationKeyCreatedDate.String(), result[2].Key)
	})

	t.Run("multiple views - no duplicates", func(t *testing.T) {
		// given
		view1 := &model.BlockContentDataviewView{
			Relations: []*model.BlockContentDataviewRelation{
				{Key: bundle.RelationKeyName.String()},
				{Key: bundle.RelationKeyType.String()},
				{Key: bundle.RelationKeyCreatedDate.String()},
			},
		}
		view2 := &model.BlockContentDataviewView{
			Relations: []*model.BlockContentDataviewRelation{
				{Key: bundle.RelationKeyName.String()},     // Duplicate
				{Key: bundle.RelationKeyAssignee.String()}, // New
				{Key: bundle.RelationKeyDone.String()},     // New
			},
		}

		// when
		result := collectRelationLinksFromViews(nil, view1, view2)

		// then
		assert.Len(t, result, 5)

		// Verify no duplicates
		keys := make(map[string]bool)
		for _, rel := range result {
			assert.False(t, keys[rel.Key], "Duplicate key found: %s", rel.Key)
			keys[rel.Key] = true
		}
	})

	t.Run("with custom relations from existing relLinks", func(t *testing.T) {
		// given
		customRelKey := "customRelation"
		existingRelLinks := []*model.RelationLink{
			{Key: customRelKey, Format: model.RelationFormat_longtext},
			{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_shorttext},
		}

		view := &model.BlockContentDataviewView{
			Relations: []*model.BlockContentDataviewRelation{
				{Key: customRelKey},                    // Custom relation
				{Key: bundle.RelationKeyType.String()}, // Bundle relation
			},
		}

		// when
		result := collectRelationLinksFromViews(existingRelLinks, view)

		// then
		assert.Len(t, result, 2)

		// Custom relation should preserve its format
		var foundCustom *model.RelationLink
		for _, rel := range result {
			if rel.Key == customRelKey {
				foundCustom = rel
				break
			}
		}
		assert.NotNil(t, foundCustom, "Custom relation not found")

		var foundType *model.RelationLink
		for _, rel := range result {
			if rel.Key == bundle.RelationKeyType.String() {
				foundType = rel
				break
			}
		}
		assert.NotNil(t, foundType, "Type relation not found")
	})

	t.Run("preserves order from views", func(t *testing.T) {
		// given
		view := &model.BlockContentDataviewView{
			Relations: []*model.BlockContentDataviewRelation{
				{Key: bundle.RelationKeyCreatedDate.String()},
				{Key: bundle.RelationKeyName.String()},
				{Key: bundle.RelationKeyType.String()},
			},
		}

		// when
		result := collectRelationLinksFromViews(nil, view)

		// then
		assert.Equal(t, bundle.RelationKeyCreatedDate.String(), result[0].Key)
		assert.Equal(t, bundle.RelationKeyName.String(), result[1].Key)
		assert.Equal(t, bundle.RelationKeyType.String(), result[2].Key)
	})
}

func TestMakeDataviewContent_WithOldContent(t *testing.T) {
	t.Run("preserves object orders and group orders", func(t *testing.T) {
		// given
		oldContent := &model.BlockContentOfDataview{
			Dataview: &model.BlockContentDataview{
				ObjectOrders: []*model.BlockContentDataviewObjectOrder{
					{
						ViewId:    "view1",
						GroupId:   "group1",
						ObjectIds: []string{"obj1", "obj2"},
					},
				},
				GroupOrders: []*model.BlockContentDataviewGroupOrder{
					{
						ViewId: "view1",
						ViewGroups: []*model.BlockContentDataviewViewGroup{
							{GroupId: "group1"},
							{GroupId: "group2"},
						},
					},
				},
				Views: []*model.BlockContentDataviewView{
					{
						Id:   "view1",
						Name: "View 1",
						Relations: []*model.BlockContentDataviewRelation{
							{Key: bundle.RelationKeyName.String(), IsVisible: true},
						},
					},
				},
			},
		}

		// when
		result := MakeDataviewContent(false, nil, nil, oldContent)

		// then
		assert.NotNil(t, result.Dataview.ObjectOrders)
		assert.Len(t, result.Dataview.ObjectOrders, 1)
		assert.Equal(t, "view1", result.Dataview.ObjectOrders[0].ViewId)

		assert.NotNil(t, result.Dataview.GroupOrders)
		assert.Len(t, result.Dataview.GroupOrders, 1)
		assert.Equal(t, "view1", result.Dataview.GroupOrders[0].ViewId)
		assert.Len(t, result.Dataview.GroupOrders[0].ViewGroups, 2)
	})

	t.Run("adds default sorts when missing", func(t *testing.T) {
		// given
		oldContent := &model.BlockContentOfDataview{
			Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{
						Id:    "view1",
						Name:  "View 1",
						Sorts: nil, // No sorts
						Relations: []*model.BlockContentDataviewRelation{
							{Key: bundle.RelationKeyName.String(), IsVisible: true},
						},
					},
				},
			},
		}

		// when
		result := MakeDataviewContent(false, nil, nil, oldContent)

		// then
		assert.NotNil(t, result.Dataview.Views[0].Sorts)
		assert.Len(t, result.Dataview.Views[0].Sorts, 1)
		assert.Equal(t, bundle.RelationKeyLastModifiedDate.String(), result.Dataview.Views[0].Sorts[0].RelationKey)
		assert.Equal(t, model.BlockContentDataviewSort_Desc, result.Dataview.Views[0].Sorts[0].Type)
	})

	t.Run("clears default template and object type IDs", func(t *testing.T) {
		// given
		oldContent := &model.BlockContentOfDataview{
			Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{
						Id:                  "view1",
						Name:                "View 1",
						DefaultTemplateId:   "template123",
						DefaultObjectTypeId: "objectType456",
						Relations: []*model.BlockContentDataviewRelation{
							{Key: bundle.RelationKeyName.String(), IsVisible: true},
						},
					},
				},
			},
		}

		// when
		result := MakeDataviewContent(false, nil, nil, oldContent)

		// then
		assert.Empty(t, result.Dataview.Views[0].DefaultTemplateId)
		assert.Empty(t, result.Dataview.Views[0].DefaultObjectTypeId)
	})

	t.Run("merges new relations with existing", func(t *testing.T) {
		// given
		oldContent := &model.BlockContentOfDataview{
			Dataview: &model.BlockContentDataview{
				RelationLinks: []*model.RelationLink{
					{Key: bundle.RelationKeyName.String()},
					{Key: bundle.RelationKeyType.String()},
				},
				Views: []*model.BlockContentDataviewView{
					{
						Id:   "view1",
						Name: "View 1",
						Relations: []*model.BlockContentDataviewRelation{
							{Key: bundle.RelationKeyName.String(), IsVisible: true},
							{Key: bundle.RelationKeyType.String(), IsVisible: true},
						},
					},
				},
			},
		}

		newRelLinks := []*model.RelationLink{
			{Key: bundle.RelationKeyAssignee.String()},
			{Key: bundle.RelationKeyDone.String()},
		}

		// when
		result := MakeDataviewContent(false, nil, newRelLinks, oldContent)

		// then
		assert.True(t, len(result.Dataview.RelationLinks) >= 4)

		keys := make(map[string]bool)
		for _, rel := range result.Dataview.RelationLinks {
			keys[rel.Key] = true
		}

		// Verify all expected relations are present
		assert.True(t, keys[bundle.RelationKeyName.String()])
		assert.True(t, keys[bundle.RelationKeyType.String()])
		assert.True(t, keys[bundle.RelationKeyAssignee.String()])
		assert.True(t, keys[bundle.RelationKeyDone.String()])
	})
}
