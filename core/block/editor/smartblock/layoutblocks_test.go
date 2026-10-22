package smartblock

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The lifecycle of a loaded object - Init, Apply - must never convert its blocks to the layout
// its type recommends: every device that loads the object would write that conversion, each
// from its own view of the type. Only the creation of an object may shape its blocks.

const lifecycleId = "obj"

// withTaskTypeLayout makes the object's type (Task) recommend layout, as this device sees it
func withTaskTypeLayout(t *testing.T, fx *fixture, layout model.ObjectTypeLayout) {
	fx.space.EXPECT().GetTypeIdByKey(mock.Anything, bundle.TypeKeyTask).Return(bundle.TypeKeyTask.URL(), nil).Maybe()
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:                domain.String(bundle.TypeKeyTask.URL()),
		bundle.RelationKeyRecommendedLayout: domain.Int64(layout),
	}})
}

// noteShapedDoc is an object in the note form: no title, the name lives in the first text block
func noteShapedDoc() *state.State {
	st := state.NewDoc(lifecycleId, map[string]simple.Block{
		lifecycleId: simple.New(&model.Block{Id: lifecycleId, ChildrenIds: []string{"text"}}),
		"text":      simple.New(&model.Block{Id: "text", Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "First line"}}}),
	}).(*state.State)
	st.SetObjectTypeKey(bundle.TypeKeyTask)
	return st
}

// titledDoc is an object in the titled form, named "Hello"
func titledDoc() *state.State {
	st := state.NewDoc(lifecycleId, map[string]simple.Block{
		lifecycleId:          simple.New(&model.Block{Id: lifecycleId, ChildrenIds: []string{state.HeaderLayoutID}}),
		state.HeaderLayoutID: simple.New(&model.Block{Id: state.HeaderLayoutID, ChildrenIds: []string{state.TitleBlockID}, Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Header}}}),
		state.TitleBlockID:   simple.New(&model.Block{Id: state.TitleBlockID, Content: &model.BlockContentOfText{Text: &model.BlockContentText{Style: model.BlockContentText_Title}}}),
	}).(*state.State)
	st.SetObjectTypeKey(bundle.TypeKeyTask)
	st.SetDetail(bundle.RelationKeyName, domain.String("Hello"))
	return st
}

func initLoaded(t *testing.T, fx *fixture, doc *state.State) *InitContext {
	fx.source.doc = doc
	initCtx := &InitContext{Ctx: context.Background(), SpaceID: testSpaceId, Source: fx.source, Doc: doc}
	require.NoError(t, fx.Init(initCtx))
	return initCtx
}

func blockChanges(pushed []source.PushChangeParams) (n int) {
	for _, p := range pushed {
		for _, ch := range p.Changes {
			if ch.GetBlockCreate() != nil || ch.GetBlockRemove() != nil || ch.GetBlockMove() != nil {
				n++
			}
		}
	}
	return n
}

func TestLayoutBlocksLifecycle(t *testing.T) {
	t.Run("loading a titled object whose type became note leaves its blocks and name", func(t *testing.T) {
		// given
		fx := newFixture(lifecycleId, t)
		withTaskTypeLayout(t, fx, model.ObjectType_note)

		// when
		initCtx := initLoaded(t, fx, titledDoc())

		// then
		assert.Equal(t, int64(model.ObjectType_note), initCtx.State.LocalDetails().GetInt64(bundle.RelationKeyResolvedLayout))
		assert.NotNil(t, initCtx.State.PickParentOf(state.TitleBlockID))
		assert.Equal(t, "Hello", initCtx.State.Details().GetString(bundle.RelationKeyName))
	})

	t.Run("loading a note-shaped object whose type left note leaves its blocks and name", func(t *testing.T) {
		// given
		fx := newFixture(lifecycleId, t)
		withTaskTypeLayout(t, fx, model.ObjectType_todo)

		// when
		initCtx := initLoaded(t, fx, noteShapedDoc())

		// then
		assert.Equal(t, int64(model.ObjectType_todo), initCtx.State.LocalDetails().GetInt64(bundle.RelationKeyResolvedLayout))
		assert.Nil(t, initCtx.State.Pick(state.TitleBlockID))
		assert.Empty(t, initCtx.State.Details().GetString(bundle.RelationKeyName))
	})

	// both directions: a titled object whose type became note, a note-shaped one whose type left it
	mismatches := []struct {
		name       string
		typeLayout model.ObjectTypeLayout
		doc        func() *state.State
		unchanged  func(t *testing.T, st *state.State)
	}{
		{"titled object, type became note", model.ObjectType_note, titledDoc, func(t *testing.T, st *state.State) {
			assert.NotNil(t, st.PickParentOf(state.TitleBlockID))
			assert.Equal(t, "Hello", st.Details().GetString(bundle.RelationKeyName))
		}},
		{"note-shaped object, type left note", model.ObjectType_todo, noteShapedDoc, func(t *testing.T, st *state.State) {
			assert.Nil(t, st.Pick(state.TitleBlockID))
			assert.Empty(t, st.Details().GetString(bundle.RelationKeyName))
		}},
	}
	for _, m := range mismatches {
		t.Run("an apply does not push a conversion: "+m.name, func(t *testing.T) {
			// given
			fx := newFixture(lifecycleId, t)
			withTaskTypeLayout(t, fx, m.typeLayout)
			fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil).Maybe()
			initLoaded(t, fx, m.doc())
			st := fx.NewState()
			st.SetDetail(bundle.RelationKeyDescription, domain.String("unrelated edit"))

			// when
			err := fx.Apply(st)

			// then
			require.NoError(t, err)
			require.NotEmpty(t, fx.source.pushed)
			assert.Zero(t, blockChanges(fx.source.pushed))
			m.unchanged(t, fx.NewState())
		})

		t.Run("a remote change appended leaves the blocks: "+m.name, func(t *testing.T) {
			// given
			fx := newFixture(lifecycleId, t)
			withTaskTypeLayout(t, fx, m.typeLayout)
			fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil).Maybe()
			initLoaded(t, fx, m.doc())

			// when
			err := fx.StateAppend(func(d state.Doc) (*state.State, []*pb.ChangeContent, error) {
				return d.NewState(), nil, nil
			})

			// then
			require.NoError(t, err)
			m.unchanged(t, fx.NewState())
		})

		t.Run("a rebuild leaves the blocks: "+m.name, func(t *testing.T) {
			// given
			fx := newFixture(lifecycleId, t)
			withTaskTypeLayout(t, fx, m.typeLayout)
			fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil).Maybe()
			initLoaded(t, fx, m.doc())

			// when
			err := fx.StateRebuild(m.doc())

			// then
			require.NoError(t, err)
			m.unchanged(t, fx.NewState())
		})
	}

	t.Run("an apply with ShapeNewObjectLayout shapes the blocks, as object creation does", func(t *testing.T) {
		// given
		fx := newFixture(lifecycleId, t)
		withTaskTypeLayout(t, fx, model.ObjectType_note)
		fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil).Maybe()
		initLoaded(t, fx, titledDoc())

		// when
		err := fx.Apply(fx.NewState(), NoRestrictions, ShapeNewObjectLayout)

		// then
		require.NoError(t, err)
		assert.Nil(t, fx.NewState().PickParentOf(state.TitleBlockID))
		assert.Empty(t, fx.Details().GetString(bundle.RelationKeyName))
	})

	t.Run("only a user change or its undo/redo lets the hooks cascade to other objects", func(t *testing.T) {
		for _, tc := range []struct {
			changeType domain.ChangeType
			want       bool
		}{
			{domain.ChangeTypeUserChange, true},
			{domain.ChangeTypeHistoryOperation, true},
			{domain.ChangeTypeSystemObjectReviserMigration, false},
			{domain.ChangeTypeObjectInit, false},
			{domain.ChangeTypeLayoutSync, false},
		} {
			t.Run(tc.changeType.String(), func(t *testing.T) {
				// given
				fx := newFixture(lifecycleId, t)
				withTaskTypeLayout(t, fx, model.ObjectType_todo)
				fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil).Maybe()
				initLoaded(t, fx, titledDoc())
				var got []bool
				fx.AddHook(func(info ApplyInfo) error {
					got = append(got, info.ApplyOtherObjects)
					return nil
				}, HookAfterApply)
				st := fx.NewState()
				st.SetDetail(bundle.RelationKeyDescription, domain.String("edit"))
				st.SetChangeType(tc.changeType)

				// when
				err := fx.Apply(st)

				// then
				require.NoError(t, err)
				assert.Equal(t, []bool{tc.want}, got)
			})
		}
	})

	t.Run("resetting to an imported version shapes its blocks to the layout it resolves to", func(t *testing.T) {
		// given: the object is a note (its own layout), the imported version drops that layout so
		// it follows the type, which says todo, and carries its name in details with no title
		fx := newFixture(lifecycleId, t)
		withTaskTypeLayout(t, fx, model.ObjectType_todo)
		fx.space.EXPECT().IsPersonal().Return(false).Maybe()
		fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil).Maybe()
		loaded := noteShapedDoc()
		loaded.SetDetail(bundle.RelationKeyLayout, domain.Int64(model.ObjectType_note))
		initLoaded(t, fx, loaded)
		imported := noteShapedDoc()
		imported.SetDetail(bundle.RelationKeyName, domain.String("Imported"))

		// when
		err := fx.ResetToVersion(imported)

		// then
		require.NoError(t, err)
		assert.NotNil(t, fx.NewState().PickParentOf(state.TitleBlockID))
		assert.Equal(t, "Imported", fx.Details().GetString(bundle.RelationKeyName))
	})

	t.Run("creating an object shapes its blocks to its layout", func(t *testing.T) {
		// given
		fx := newFixture(lifecycleId, t)
		withTaskTypeLayout(t, fx, model.ObjectType_todo)
		fx.space.EXPECT().IsPersonal().Return(false).Maybe()
		empty := state.NewDoc(lifecycleId, nil).(*state.State)
		fx.source.doc = empty
		initCtx := &InitContext{
			Ctx:         context.Background(),
			SpaceID:     testSpaceId,
			Source:      fx.source,
			Doc:         empty,
			State:       noteShapedDoc(),
			IsNewObject: true,
		}

		// when
		err := fx.Init(initCtx)

		// then
		require.NoError(t, err)
		assert.NotNil(t, initCtx.State.PickParentOf(state.TitleBlockID))
		assert.Equal(t, "First line", initCtx.State.Details().GetString(bundle.RelationKeyName))
	})
}
