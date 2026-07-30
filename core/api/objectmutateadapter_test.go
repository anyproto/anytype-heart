package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// liveObject builds a smartblock whose state carries everything the AnyBlock
// format drops on export: structural blocks (header/title/featuredRelations),
// a custom relation link, a resolvedLayout local detail, and two object types.
func liveObject(t *testing.T) *smarttest.SmartTest {
	t.Helper()
	sb := smarttest.New("obj1")
	sb.AddBlock(simple.New(&model.Block{Id: "obj1", ChildrenIds: []string{"header", "p1"}}))
	sb.AddBlock(simple.New(&model.Block{Id: "header", ChildrenIds: []string{"title", "featuredRelations"},
		Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Header}}}))
	sb.AddBlock(simple.New(&model.Block{Id: "title",
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{Style: model.BlockContentText_Title}}}))
	sb.AddBlock(simple.New(&model.Block{Id: "featuredRelations",
		Content: &model.BlockContentOfFeaturedRelations{FeaturedRelations: &model.BlockContentFeaturedRelations{}}}))
	sb.AddBlock(simple.New(&model.Block{Id: "p1",
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "body"}}}))

	st := sb.Doc.(*state.State)
	st.AddRelationLinks(&model.RelationLink{Key: "customClient", Format: model.RelationFormat_shorttext})
	st.SetLocalDetail(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_note)))
	st.SetObjectTypeKeys([]domain.TypeKey{"note", "extraType"})
	return sb
}

// editedState is what the edit pipeline produces: the format's view only —
// no structural blocks, no relation links, no resolvedLayout, one type key.
func editedState(t *testing.T) *state.State {
	t.Helper()
	st := state.NewDoc("obj1", nil).(*state.State)
	st.Add(simple.New(&model.Block{Id: "obj1", ChildrenIds: []string{"p1"}}))
	st.Add(simple.New(&model.Block{Id: "p1",
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "edited body"}}}))
	st.SetObjectTypeKeys([]domain.TypeKey{"note"})
	return st
}

// TestPreserveEditorOwnedState covers review findings A1-A3 and the
// multi-type case: everything the format drops must survive an edit, or the
// reset turns each absence into a destructive CRDT change.
func TestPreserveEditorOwnedState(t *testing.T) {
	sb := liveObject(t)
	st := editedState(t)

	preserveEditorOwnedState(sb, st)

	t.Run("A1: custom relation links are carried over", func(t *testing.T) {
		// without this the diff emits RelationRemove, which deletes the
		// detail value on replay (GO-7217 class)
		var keys []string
		for _, l := range st.PickRelationLinks() {
			keys = append(keys, l.Key)
		}
		assert.Contains(t, keys, "customClient")
	})

	t.Run("A2: resolvedLayout is carried over", func(t *testing.T) {
		// without this resolveLayout sees unset->recommended as a change and
		// the note conversion eats the first paragraph into the title
		v := st.LocalDetails().Get(bundle.RelationKeyResolvedLayout)
		require.True(t, v.Ok(), "resolvedLayout must be present")
		assert.Equal(t, int64(model.ObjectType_note), v.Int64())
	})

	t.Run("A3: structural blocks are restored, leading the root", func(t *testing.T) {
		for _, id := range []string{"header", "title", "featuredRelations"} {
			assert.True(t, st.Exists(id), "structural block %q must survive the edit", id)
		}
		root := st.Pick(st.RootId())
		require.NotNil(t, root)
		assert.Equal(t, []string{"header", "p1"}, root.Model().ChildrenIds,
			"structural blocks lead the document, edited content follows")
	})

	t.Run("extra object types are kept", func(t *testing.T) {
		assert.Equal(t, []domain.TypeKey{"note", "extraType"}, st.ObjectTypeKeys())
	})

	t.Run("the edit itself is preserved", func(t *testing.T) {
		b := st.Pick("p1")
		require.NotNil(t, b)
		assert.Equal(t, "edited body", b.Model().GetText().Text)
	})
}

func TestPreserveEditorOwnedState_NoStructuralBlocks(t *testing.T) {
	// an object without structural blocks (e.g. a note-layout page) must not
	// gain any, and its root children must stay exactly as the ops left them
	sb := smarttest.New("obj1")
	sb.AddBlock(simple.New(&model.Block{Id: "obj1", ChildrenIds: []string{"p1"}}))
	sb.AddBlock(simple.New(&model.Block{Id: "p1",
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "body"}}}))

	st := editedState(t)
	preserveEditorOwnedState(sb, st)

	root := st.Pick(st.RootId())
	require.NotNil(t, root)
	assert.Equal(t, []string{"p1"}, root.Model().ChildrenIds)
}

// TestCheckObjectEditable covers A4: the apply runs with NoRestrictions, so
// the adapter must enforce the object's own restrictions itself.
func TestCheckObjectEditable(t *testing.T) {
	t.Run("an unrestricted object is editable", func(t *testing.T) {
		require.NoError(t, checkObjectEditable(smarttest.New("obj1")))
	})

	t.Run("block-restricted objects are refused", func(t *testing.T) {
		sb := smarttest.New("obj1")
		sb.TestRestrictions = restriction.Restrictions{
			Object: restriction.ObjectRestrictions{model.Restrictions_Blocks: {}},
		}
		err := checkObjectEditable(sb)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "blocks cannot be edited")
	})

	t.Run("details-restricted objects are refused", func(t *testing.T) {
		sb := smarttest.New("obj1")
		sb.TestRestrictions = restriction.Restrictions{
			Object: restriction.ObjectRestrictions{model.Restrictions_Details: {}},
		}
		err := checkObjectEditable(sb)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "properties cannot be edited")
	})
}

// fakeGetter serves one smartblock to the adapter's DoContextFullID.
type fakeGetter struct {
	sb smartblock.SmartBlock
}

func (g fakeGetter) GetObject(_ context.Context, _ string) (smartblock.SmartBlock, error) {
	return g.sb, nil
}

func (g fakeGetter) GetObjectByFullID(_ context.Context, _ domain.FullID) (smartblock.SmartBlock, error) {
	return g.sb, nil
}

// TestMutateObject covers the PATCH commit path: apply receives a child
// state of the live doc, and a nil return commits it with one ordinary
// Apply.
func TestMutateObject(t *testing.T) {
	ctx := context.Background()

	newSb := func() *smarttest.SmartTest {
		sb := smarttest.New("obj1")
		sb.AddBlock(simple.New(&model.Block{Id: "obj1", ChildrenIds: []string{"p1"}}))
		sb.AddBlock(simple.New(&model.Block{Id: "p1",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "body"}}}))
		return sb
	}

	t.Run("apply on the child state commits", func(t *testing.T) {
		// given
		sb := newSb()
		adapter := newObjectMutateAdapter(fakeGetter{sb: sb})

		// when
		heads, err := adapter.MutateObject(ctx, "space1", "obj1", func(edit apicore.ObjectEdit) error {
			b := edit.State.Get("p1")
			require.NotNil(t, b)
			b.Model().GetText().Text = "edited"
			return nil
		})

		// then
		require.NoError(t, err)
		assert.NotEmpty(t, heads)
		assert.Equal(t, "edited", sb.Doc.(*state.State).Pick("p1").Model().GetText().Text,
			"the child state landed on the live doc")
	})

	t.Run("an apply error commits nothing", func(t *testing.T) {
		sb := newSb()
		adapter := newObjectMutateAdapter(fakeGetter{sb: sb})

		_, err := adapter.MutateObject(ctx, "space1", "obj1", func(edit apicore.ObjectEdit) error {
			edit.State.Get("p1").Model().GetText().Text = "edited"
			return assert.AnError
		})

		require.Error(t, err)
		assert.Equal(t, "body", sb.Doc.(*state.State).Pick("p1").Model().GetText().Text)
	})

	t.Run("object restrictions refuse the edit before apply runs", func(t *testing.T) {
		sb := newSb()
		sb.TestRestrictions = restriction.Restrictions{
			Object: restriction.ObjectRestrictions{model.Restrictions_Blocks: {}},
		}
		adapter := newObjectMutateAdapter(fakeGetter{sb: sb})

		called := false
		_, err := adapter.MutateObject(ctx, "space1", "obj1", func(apicore.ObjectEdit) error {
			called = true
			return nil
		})

		require.Error(t, err)
		assert.False(t, called)
	})

	t.Run("a revision downgrade is refused", func(t *testing.T) {
		sb := newSb()
		sb.Doc.(*state.State).SetDetail(bundle.RelationKeyRevision, domain.Int64(3))
		adapter := newObjectMutateAdapter(fakeGetter{sb: sb})

		_, err := adapter.MutateObject(ctx, "space1", "obj1", func(edit apicore.ObjectEdit) error {
			edit.State.SetDetail(bundle.RelationKeyRevision, domain.Int64(1))
			return nil
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "never downgraded")
	})
}

func TestIsStructuralBlock(t *testing.T) {
	tests := []struct {
		name  string
		block *model.Block
		want  bool
	}{
		{"header layout", &model.Block{Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Header}}}, true},
		{"title text", &model.Block{Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{Style: model.BlockContentText_Title}}}, true},
		{"description text", &model.Block{Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{Style: model.BlockContentText_Description}}}, true},
		{"featured relations", &model.Block{Content: &model.BlockContentOfFeaturedRelations{
			FeaturedRelations: &model.BlockContentFeaturedRelations{}}}, true},
		{"ordinary paragraph", &model.Block{Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{Text: "hi"}}}, false},
		{"row layout is not structural", &model.Block{Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Row}}}, false},
		{"nil layout content", &model.Block{Content: &model.BlockContentOfLayout{}}, false},
		{"nil text content", &model.Block{Content: &model.BlockContentOfText{}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isStructuralBlock(tt.block))
		})
	}
}
