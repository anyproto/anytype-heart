package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// typeStateWithColumns builds a type's state the way types were stored before
// the columns fix: the type's own property is a view relation, switched off.
func typeStateWithColumns(t *testing.T, propertyVisible bool, extra ...*model.BlockContentDataviewRelation) (*state.State, simple.Block) {
	t.Helper()
	dataview := simple.New(&model.Block{
		Id: state.DataviewBlockID,
		Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
			Views: []*model.BlockContentDataviewView{{
				Id: "default",
				Relations: []*model.BlockContentDataviewRelation{
					{Key: bundle.RelationKeyName.String(), IsVisible: true},
					{Key: bundle.RelationKeyBacklinks.String(), IsVisible: false},
					{Key: "task_priority", IsVisible: propertyVisible},
				},
			}},
		}},
	})
	view := dataview.Model().GetDataview().Views[0]
	view.Relations = append(view.Relations, extra...)
	doc := state.NewDoc("typeId", map[string]simple.Block{
		"typeId":              simple.New(&model.Block{Id: "typeId", ChildrenIds: []string{state.DataviewBlockID}}),
		state.DataviewBlockID: dataview,
	})
	s := doc.NewState()
	s.SetDetail(bundle.RelationKeyRecommendedFeaturedRelations, domain.StringList([]string{"rel-priority"}))
	s.SetDetail(bundle.RelationKeyRecommendedRelations, domain.StringList([]string{"rel-assignee"}))
	return s, dataview
}

func viewRelations(t *testing.T, s *state.State) []*model.BlockContentDataviewRelation {
	t.Helper()
	block := s.Pick(state.DataviewBlockID)
	require.NotNil(t, block)
	return block.Model().GetDataview().Views[0].Relations
}

func TestReconcileDataviewColumns(t *testing.T) {
	const spaceId = "space1"
	newObjectType := func(t *testing.T) *ObjectType {
		store := objectstore.NewStoreFixture(t)
		store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel-priority"),
				bundle.RelationKeyRelationKey:    domain.String("task_priority"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:             domain.String("rel-assignee"),
				bundle.RelationKeyRelationKey:    domain.String("task_assignee"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_shorttext)),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
			},
		})
		return &ObjectType{spaceIndex: store.SpaceIndex(spaceId)}
	}

	t.Run("a type stored with its columns off gets them back", func(t *testing.T) {
		// given
		s, _ := typeStateWithColumns(t, false)
		ot := newObjectType(t)

		// when
		ot.reconcileDataviewColumns(s)

		// then
		relations := viewRelations(t, s)
		assert.False(t, relations[1].IsVisible, "housekeeping relations stay off")
		assert.True(t, relations[2].IsVisible, "the type's own property is a column again")
	})

	t.Run("a view that already matches the type is not modified at all", func(t *testing.T) {
		// given — this runs on every open of every type, so a no-op must not
		// write a change to each one
		s, stored := typeStateWithColumns(t, true,
			&model.BlockContentDataviewRelation{Key: "task_assignee", IsVisible: true})
		ot := newObjectType(t)

		// when
		ot.reconcileDataviewColumns(s)

		// then — an untouched state still hands back the stored block itself,
		// a copied one would be a write on every type in every space
		assert.Same(t, stored, s.Pick(state.DataviewBlockID))
	})

	t.Run("a property the view never got becomes a column", func(t *testing.T) {
		// given — an import can build the view before a relation object is
		// indexed, and the property is then missing from the view entirely
		s, _ := typeStateWithColumns(t, false)
		ot := newObjectType(t)

		// when
		ot.reconcileDataviewColumns(s)

		// then
		var keys []string
		for _, rel := range viewRelations(t, s) {
			if rel.IsVisible {
				keys = append(keys, rel.Key)
			}
		}
		assert.Contains(t, keys, "task_assignee")
	})

	t.Run("a type with no dataview is left alone", func(t *testing.T) {
		// given
		s := state.NewDoc("typeId", nil).NewState()
		ot := newObjectType(t)

		// when / then — must not panic
		ot.reconcileDataviewColumns(s)
		assert.Nil(t, s.Pick(state.DataviewBlockID))
	})
}
