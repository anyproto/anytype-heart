package template

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
						// ot alone is the FALLBACK column source: its relations
						// become columns but stay switched off, because no
						// caller named them as the visible set. A type's own
						// view passes them explicitly too — the case below.
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
			// the objecttype.go calling convention (a type's own default "All"
			// view): the type AND its relation links are both passed, and the
			// explicitly passed links must come out VISIBLE. GO-5969 regressed
			// this to name-only visibility, which left every custom column of a
			// freshly created type's default view hidden (GO-7383).
			name: "type's own view: explicitly passed relation links become visible columns",
			ot: &model.ObjectType{
				Url:  "typeObjectId",
				Name: "Plant",
				Key:  "plant",
				RelationLinks: makeRelationLinks([]domain.RelationKey{
					bundle.RelationKeyMentions, bundle.RelationKeyLinkedProjects, bundle.RelationKeyAssignee}),
			},
			relLinks: makeRelationLinks([]domain.RelationKey{
				bundle.RelationKeyMentions, bundle.RelationKeyLinkedProjects, bundle.RelationKeyAssignee}),
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
							append(defaultDataviewRelations, bundle.RelationKeyMentions, bundle.RelationKeyLinkedProjects, bundle.RelationKeyAssignee),
							append(defaultVisibleRelations, bundle.RelationKeyMentions, bundle.RelationKeyLinkedProjects, bundle.RelationKeyAssignee),
						),
					},
				},
				RelationLinks: makeRelationLinks(
					append(defaultDataviewRelations, bundle.RelationKeyMentions, bundle.RelationKeyLinkedProjects, bundle.RelationKeyAssignee)),
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

func TestReconcileTypeDataviewColumns(t *testing.T) {
	// A type's dataview as it was built before columns were made visible: the
	// type's own properties are listed among the view's relations, all off.
	brokenView := func() *model.BlockContentDataview {
		return &model.BlockContentDataview{
			RelationLinks: []*model.RelationLink{
				{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
				{Key: "task_priority", Format: model.RelationFormat_status},
			},
			Views: []*model.BlockContentDataviewView{{
				Id: "default",
				Relations: []*model.BlockContentDataviewRelation{
					{Key: bundle.RelationKeyName.String(), IsVisible: true},
					{Key: bundle.RelationKeyCreatedDate.String(), IsVisible: false},
					{Key: bundle.RelationKeyBacklinks.String(), IsVisible: false},
					{Key: "task_priority", IsVisible: false},
				},
			}},
		}
	}
	typeProperties := []*model.RelationLink{
		{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
		{Key: "task_priority", Format: model.RelationFormat_status},
		{Key: "task_assignee", Format: model.RelationFormat_shorttext},
	}

	visibleKeys := func(dv *model.BlockContentDataview) []string {
		var keys []string
		for _, rel := range dv.Views[0].Relations {
			if rel.IsVisible {
				keys = append(keys, rel.Key)
			}
		}
		return keys
	}
	linkKeys := func(dv *model.BlockContentDataview) []string {
		var keys []string
		for _, link := range dv.RelationLinks {
			keys = append(keys, link.Key)
		}
		return keys
	}

	t.Run("an untouched view gets the type's own properties back", func(t *testing.T) {
		// given
		dv := brokenView()

		// when
		changed := ReconcileTypeDataviewColumns(dv, typeProperties)

		// then — the type's properties, not the housekeeping relations
		assert.True(t, changed)
		assert.ElementsMatch(t, []string{bundle.RelationKeyName.String(), "task_priority", "task_assignee"}, visibleKeys(dv))
	})

	t.Run("a property the view never got is added as a column", func(t *testing.T) {
		// given — an import can create the type before the relation object is
		// indexed, and the property is then missing from the view entirely
		dv := brokenView()

		// when
		ReconcileTypeDataviewColumns(dv, typeProperties)

		// then — both the view relation and the link carrying its format
		assert.Contains(t, visibleKeys(dv), "task_assignee")
		assert.Contains(t, linkKeys(dv), "task_assignee")
		for _, link := range dv.RelationLinks {
			if link.Key == "task_assignee" {
				assert.Equal(t, model.RelationFormat_shorttext, link.Format)
			}
		}
	})

	t.Run("a property missing from a view the fix already filled is added visible", func(t *testing.T) {
		// given — the shape the import race leaves behind: the view was built
		// with the properties that resolved, and one never made it in
		dv := brokenView()
		dv.Views[0].Relations[3].IsVisible = true // task_priority, as built today

		// when
		changed := ReconcileTypeDataviewColumns(dv, typeProperties)

		// then — its own columns are not evidence that anyone arranged this
		assert.True(t, changed)
		assert.ElementsMatch(t, []string{bundle.RelationKeyName.String(), "task_priority", "task_assignee"}, visibleKeys(dv))
	})

	t.Run("a hidden property stays hidden once someone has arranged the view", func(t *testing.T) {
		// given — one of the type's properties on, another off: a selection
		dv := brokenView()
		dv.Views[0].Relations[3].IsVisible = true
		dv.Views[0].Relations = append(dv.Views[0].Relations,
			&model.BlockContentDataviewRelation{Key: "task_assignee", IsVisible: false})

		// when
		changed := ReconcileTypeDataviewColumns(dv, typeProperties)

		// then
		assert.False(t, changed)
		assert.Equal(t, []string{bundle.RelationKeyName.String(), "task_priority"}, visibleKeys(dv))
	})

	t.Run("a view the user has arranged keeps its columns, and gains the rest as available", func(t *testing.T) {
		// given — one column switched on by hand is the whole signal
		dv := brokenView()
		dv.Views[0].Relations[1].IsVisible = true

		// when
		changed := ReconcileTypeDataviewColumns(dv, typeProperties)

		// then — nothing switched on behind their back, but the missing
		// property is now offered in the column picker
		assert.True(t, changed)
		assert.Equal(t, []string{bundle.RelationKeyName.String(), bundle.RelationKeyCreatedDate.String()}, visibleKeys(dv))
		assert.Contains(t, linkKeys(dv), "task_assignee")
	})

	t.Run("a type with no properties of its own has nothing to reconcile", func(t *testing.T) {
		// given
		dv := &model.BlockContentDataview{Views: []*model.BlockContentDataviewView{{
			Relations: makeDataviewRelations(defaultDataviewRelations, defaultVisibleRelations),
		}}}

		// when
		changed := ReconcileTypeDataviewColumns(dv, makeRelationLinks(defaultDataviewRelations))

		// then
		assert.False(t, changed)
		assert.Equal(t, []string{bundle.RelationKeyName.String()}, visibleKeys(dv))
	})

	t.Run("a view built by the current code is already right", func(t *testing.T) {
		// given
		// the type editor's calling convention: the type AND its relation
		// links, which is what makes those columns visible (objecttype.go)
		links := []*model.RelationLink{{Key: bundle.RelationKeyAssignee.String()}}
		content := MakeDataviewContent(false, &model.ObjectType{RelationLinks: links}, links, nil)

		// when
		changed := ReconcileTypeDataviewColumns(content.Dataview, links)

		// then
		assert.False(t, changed)
	})

	t.Run("nothing to do without a dataview", func(t *testing.T) {
		assert.False(t, ReconcileTypeDataviewColumns(nil, typeProperties))
		assert.False(t, ReconcileTypeDataviewColumns(&model.BlockContentDataview{}, typeProperties))
	})
}

func TestPruneTypeDataviewColumns(t *testing.T) {
	dataview := func(views ...*model.BlockContentDataviewView) *model.BlockContentDataview {
		return &model.BlockContentDataview{
			RelationLinks: []*model.RelationLink{
				{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
				{Key: "sun_needs", Format: model.RelationFormat_status},
			},
			Views: views,
		}
	}
	view := func(id, name string, keys ...string) *model.BlockContentDataviewView {
		v := &model.BlockContentDataviewView{Id: id, Name: name}
		for _, key := range keys {
			v.Relations = append(v.Relations, &model.BlockContentDataviewRelation{Key: key, IsVisible: true})
		}
		return v
	}
	columnKeys := func(v *model.BlockContentDataviewView) []string {
		keys := make([]string, 0, len(v.Relations))
		for _, rel := range v.Relations {
			keys = append(keys, rel.Key)
		}
		return keys
	}

	t.Run("the column goes from every view that merely shows it", func(t *testing.T) {
		// given
		dv := dataview(
			view("all", "All", bundle.RelationKeyName.String(), "sun_needs"),
			view("grid", "Grid", "sun_needs"),
		)

		// when
		changed := PruneTypeDataviewColumns(dv, []string{"sun_needs"})

		// then
		assert.True(t, changed)
		assert.Equal(t, []string{bundle.RelationKeyName.String()}, columnKeys(dv.Views[0]))
		assert.Empty(t, columnKeys(dv.Views[1]))
	})

	// The three ways a view can be arranged by a property. Dropping the column
	// out from under any of them leaves a view organised by something nobody
	// can see, so the view is reported and left alone instead.
	t.Run("a view arranged by the property keeps it", func(t *testing.T) {
		for name, arrange := range map[string]func(*model.BlockContentDataviewView){
			"group": func(v *model.BlockContentDataviewView) { v.GroupRelationKey = "sun_needs" },
			"sort": func(v *model.BlockContentDataviewView) {
				v.Sorts = []*model.BlockContentDataviewSort{{RelationKey: "sun_needs"}}
			},
			"filter": func(v *model.BlockContentDataviewView) {
				v.Filters = []*model.BlockContentDataviewFilter{{RelationKey: "sun_needs"}}
			},
			"nested filter": func(v *model.BlockContentDataviewView) {
				v.Filters = []*model.BlockContentDataviewFilter{{NestedFilters: []*model.BlockContentDataviewFilter{
					{RelationKey: "sun_needs"},
				}}}
			},
		} {
			t.Run(name, func(t *testing.T) {
				// given
				board := view("board", "Board", bundle.RelationKeyName.String(), "sun_needs")
				arrange(board)
				dv := dataview(board)

				// when
				plan := PlanTypeDataviewColumnPrune(dv, []string{"sun_needs"})
				changed := PruneTypeDataviewColumns(dv, []string{"sun_needs"})

				// then
				assert.False(t, changed)
				assert.Equal(t, []string{bundle.RelationKeyName.String(), "sun_needs"}, columnKeys(dv.Views[0]))
				require.Len(t, plan.InUse, 1, "a view left alone must be named")
				assert.Equal(t, "Board", plan.InUse[0].ViewName)
				assert.Empty(t, plan.Pruned)
			})
		}
	})

	t.Run("a view arranged by one removed property keeps its other columns too", func(t *testing.T) {
		// given: pruning the rest would still be a rewrite of a view this
		// plan has decided not to touch
		board := view("board", "Board", "sun_needs", "water_needs")
		board.GroupRelationKey = "sun_needs"
		dv := dataview(board)

		// when
		changed := PruneTypeDataviewColumns(dv, []string{"sun_needs", "water_needs"})

		// then
		assert.False(t, changed)
		assert.Equal(t, []string{"sun_needs", "water_needs"}, columnKeys(dv.Views[0]))
	})

	t.Run("a property no view shows changes nothing", func(t *testing.T) {
		// given
		dv := dataview(view("all", "All", bundle.RelationKeyName.String()))

		// when
		plan := PlanTypeDataviewColumnPrune(dv, []string{"sun_needs"})

		// then
		assert.True(t, plan.Empty())
		assert.False(t, PruneTypeDataviewColumns(dv, []string{"sun_needs"}))
	})

	t.Run("nothing to work on is not a crash", func(t *testing.T) {
		assert.False(t, PruneTypeDataviewColumns(nil, []string{"sun_needs"}))
		assert.False(t, PruneTypeDataviewColumns(&model.BlockContentDataview{}, []string{"sun_needs"}))
		assert.False(t, PruneTypeDataviewColumns(dataview(view("all", "All", "sun_needs")), nil))
	})

	// The reconcile that runs on every type open only ever ADDS a column for a
	// property the type recommends, so a prune of a property the type no
	// longer lists is not undone by it.
	t.Run("the reconcile does not put a removed column back", func(t *testing.T) {
		// given
		dv := dataview(view("all", "All", bundle.RelationKeyName.String(), "sun_needs"))
		require.True(t, PruneTypeDataviewColumns(dv, []string{"sun_needs"}))

		// when: the type now recommends only name
		ReconcileTypeDataviewColumns(dv, []*model.RelationLink{
			{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_longtext},
		})

		// then
		assert.Equal(t, []string{bundle.RelationKeyName.String()}, columnKeys(dv.Views[0]))
	})
}

// TestPruneDropsUnreferencedRelationLinks covers the half a pruned column
// leaves behind. The served type document builds its `properties` array by
// walking RelationLinks, so a link left after its column is gone means GET
// still lists a property the type no longer recommends — the "checked and was
// reassured" reading this prune exists to end. It also feeds the reconcile,
// which re-adds a link with no column as a hidden column.
func TestPruneDropsUnreferencedRelationLinks(t *testing.T) {
	newDv := func() *model.BlockContentDataview {
		return &model.BlockContentDataview{
			RelationLinks: []*model.RelationLink{
				{Key: "name", Format: model.RelationFormat_longtext},
				{Key: "sun_needs", Format: model.RelationFormat_status},
				{Key: "water_needs", Format: model.RelationFormat_longtext},
			},
			Views: []*model.BlockContentDataviewView{
				{Id: "v1", Name: "All", Relations: []*model.BlockContentDataviewRelation{
					{Key: "name", IsVisible: true},
					{Key: "sun_needs", IsVisible: true},
				}},
				{Id: "v2", Name: "Grid", Relations: []*model.BlockContentDataviewRelation{
					{Key: "sun_needs", IsVisible: true},
					{Key: "water_needs", IsVisible: true},
				}},
			},
		}
	}
	linkKeys := func(dv *model.BlockContentDataview) []string {
		var keys []string
		for _, link := range dv.RelationLinks {
			keys = append(keys, link.Key)
		}
		return keys
	}

	t.Run("a link no view references any more goes", func(t *testing.T) {
		// given
		dv := newDv()

		// when: sun_needs leaves both views
		changed := PruneTypeDataviewColumns(dv, []string{"sun_needs"})

		// then
		require.True(t, changed)
		assert.Equal(t, []string{"name", "water_needs"}, linkKeys(dv),
			"a stale link keeps the property in the served document")
	})

	t.Run("a link a surviving view still uses stays", func(t *testing.T) {
		// given: a view that groups by the key is left whole, so it keeps the column
		dv := newDv()
		dv.Views[1].GroupRelationKey = "sun_needs"

		// when
		PruneTypeDataviewColumns(dv, []string{"sun_needs"})

		// then: v2 kept its column, so the format cache must keep the link
		assert.Contains(t, linkKeys(dv), "sun_needs",
			"a view still showing it needs its format")
	})

	t.Run("an untouched key keeps its link", func(t *testing.T) {
		// given
		dv := newDv()

		// when
		PruneTypeDataviewColumns(dv, []string{"sun_needs"})

		// then
		assert.Contains(t, linkKeys(dv), "name")
	})
}

// TestPruneKeepsViewsArrangedByCoverOrEnd covers the two arrangements beyond
// grouping. A gallery's cover and a calendar's date are the view doing
// something WITH the property, not merely showing it — dropping the column out
// from under either leaves the view arranged by something nobody can see, and
// a dangling key in its settings.
func TestPruneKeepsViewsArrangedByCoverOrEnd(t *testing.T) {
	newDv := func(arrange func(*model.BlockContentDataviewView)) *model.BlockContentDataview {
		view := &model.BlockContentDataviewView{
			Id: "v1", Name: "Gallery",
			Relations: []*model.BlockContentDataviewRelation{
				{Key: "name", IsVisible: true},
				{Key: "photo", IsVisible: true},
			},
		}
		arrange(view)
		return &model.BlockContentDataview{
			RelationLinks: []*model.RelationLink{{Key: "name"}, {Key: "photo"}},
			Views:         []*model.BlockContentDataviewView{view},
		}
	}

	t.Run("a view covering by the property keeps it", func(t *testing.T) {
		// given
		dv := newDv(func(v *model.BlockContentDataviewView) { v.CoverRelationKey = "photo" })

		// when
		plan := PlanTypeDataviewColumnPrune(dv, []string{"photo"})
		PruneTypeDataviewColumns(dv, []string{"photo"})

		// then
		require.Len(t, plan.InUse, 1, "the view is arranged by it, so it is reported in use")
		assert.Empty(t, plan.Pruned)
		assert.Equal(t, []string{"name", "photo"}, viewRelationKeys(dv.Views[0]))
	})

	t.Run("a view ending by the property keeps it", func(t *testing.T) {
		// given
		dv := newDv(func(v *model.BlockContentDataviewView) { v.EndRelationKey = "photo" })

		// when
		plan := PlanTypeDataviewColumnPrune(dv, []string{"photo"})
		PruneTypeDataviewColumns(dv, []string{"photo"})

		// then
		require.Len(t, plan.InUse, 1)
		assert.Equal(t, []string{"name", "photo"}, viewRelationKeys(dv.Views[0]))
	})

	t.Run("a view merely showing it loses the column", func(t *testing.T) {
		// given: the control — no arrangement
		dv := newDv(func(v *model.BlockContentDataviewView) {})

		// when
		PruneTypeDataviewColumns(dv, []string{"photo"})

		// then
		assert.Equal(t, []string{"name"}, viewRelationKeys(dv.Views[0]))
	})
}

func viewRelationKeys(view *model.BlockContentDataviewView) []string {
	var keys []string
	for _, rel := range view.Relations {
		if rel != nil {
			keys = append(keys, rel.Key)
		}
	}
	return keys
}
