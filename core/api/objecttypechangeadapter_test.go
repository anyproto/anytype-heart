package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// typeChangingSb is a smarttest object that also answers the editor's type
// change (basic.CommonOperations), recording what it was asked. Only
// SetObjectTypesInState is real; the rest of CommonOperations is the nil
// embed, and the methods both embeds spell are pinned to the smarttest one.
type typeChangingSb struct {
	*smarttest.SmartTest
	basic.CommonOperations
	keys               []domain.TypeKey
	ignoredRestriction bool
}

func (s *typeChangingSb) SetObjectTypesInState(st *state.State, keys []domain.TypeKey, ignoreRestrictions bool) error {
	s.keys = keys
	s.ignoredRestriction = ignoreRestrictions
	st.SetObjectTypeKeys(keys)
	return nil
}

func (s *typeChangingSb) SetDetails(ctx session.Context, details []domain.Detail, showEvent bool) error {
	return s.SmartTest.SetDetails(ctx, details, showEvent)
}

func (s *typeChangingSb) UpdateDetails(ctx session.Context, update func(current *domain.Details) (*domain.Details, error)) error {
	return s.SmartTest.UpdateDetails(ctx, update)
}

func (s *typeChangingSb) SetLayout(ctx session.Context, layout model.ObjectTypeLayout) error {
	return s.SmartTest.SetLayout(ctx, layout)
}

func (s *typeChangingSb) SetObjectTypes(_ session.Context, keys []domain.TypeKey, _ bool) error {
	s.SmartTest.SetObjectTypes(keys)
	return nil
}

func newTypeChangingSb() *typeChangingSb {
	sb := smarttest.New("obj1")
	sb.AddBlock(simple.New(&model.Block{Id: "obj1", ChildrenIds: []string{"p1"}}))
	sb.AddBlock(simple.New(&model.Block{Id: "p1",
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "body"}}}))
	sb.SetObjectTypes([]domain.TypeKey{bundle.TypeKeyPage})
	return &typeChangingSb{SmartTest: sb}
}

func TestMutateObjectTypeChange(t *testing.T) {
	ctx := context.Background()
	typeAxis := apicore.EditNeeds{TypeChange: true}

	t.Run("the hook reaches the editor's type change and the result commits", func(t *testing.T) {
		sb := newTypeChangingSb()
		adapter := newObjectMutateAdapter(fakeGetter{sb: sb})

		_, err := adapter.MutateObject(ctx, "space1", "obj1", typeAxis, func(edit apicore.ObjectEdit) error {
			require.NotNil(t, edit.SetObjectType, "an editor that can change types must hand the hook over")
			return edit.SetObjectType(edit.State, bundle.TypeKeyTask)
		})

		require.NoError(t, err)
		assert.Equal(t, []domain.TypeKey{bundle.TypeKeyTask}, sb.keys, "the editor path, not a bare key write")
		assert.False(t, sb.ignoredRestriction, "the editor's own restriction checks stay on under the lock")
		assert.Equal(t, []domain.TypeKey{bundle.TypeKeyTask}, sb.Doc.(*state.State).ObjectTypeKeys(), "committed with the batch")
	})

	t.Run("an object without the editor's type change gets no hook", func(t *testing.T) {
		sb := smarttest.New("obj1")
		sb.AddBlock(simple.New(&model.Block{Id: "obj1"}))
		adapter := newObjectMutateAdapter(fakeGetter{sb: sb})

		_, err := adapter.MutateObject(ctx, "space1", "obj1", apicore.EditNeeds{}, func(edit apicore.ObjectEdit) error {
			assert.Nil(t, edit.SetObjectType)
			return nil
		})

		require.NoError(t, err)
	})

	t.Run("a type-change-restricted object refuses the type axis and nothing else", func(t *testing.T) {
		sb := newTypeChangingSb()
		sb.TestRestrictions = restriction.Restrictions{
			Object: restriction.ObjectRestrictions{model.Restrictions_TypeChange: {}},
		}

		err := checkObjectEditable(sb, typeAxis)

		require.ErrorIs(t, err, restriction.ErrRestricted)
		assert.Contains(t, err.Error(), "type cannot be changed")
		require.NoError(t, checkObjectEditable(sb, apicore.EditNeeds{Blocks: true, Details: true}),
			"a set carries TypeChange but must still take property and block edits")
	})

	t.Run("the live read carries the type-change verdict", func(t *testing.T) {
		sb := newTypeChangingSb()
		sb.TestRestrictions = restriction.Restrictions{
			Object: restriction.ObjectRestrictions{model.Restrictions_TypeChange: {}},
		}

		read := readLiveState(sb)

		require.ErrorIs(t, read.TypeChangeRefused, restriction.ErrRestricted)
		assert.NoError(t, read.BlocksRefused)
		assert.NoError(t, read.DetailsRefused)
	})
}
