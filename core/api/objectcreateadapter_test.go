package api

import (
	"context"
	"errors"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator/mock_objectcreator"
	"github.com/anyproto/anytype-heart/core/block/template"
	"github.com/anyproto/anytype-heart/core/block/template/mock_template"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestSnapshotRootId(t *testing.T) {
	t.Run("finds the smartblock root among the blocks", func(t *testing.T) {
		// given
		snapshot := &model.SmartBlockSnapshotBase{Blocks: []*model.Block{
			{Id: "p1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "hi"}}},
			{Id: "root1", Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
		}}

		// then
		assert.Equal(t, "root1", snapshotRootId(snapshot))
	})

	t.Run("no root and nil snapshot yield empty", func(t *testing.T) {
		assert.Empty(t, snapshotRootId(nil))
		assert.Empty(t, snapshotRootId(&model.SmartBlockSnapshotBase{Blocks: []*model.Block{{Id: "p1"}}}))
	})
}

func TestBundledIdsToInstall(t *testing.T) {
	t.Run("bundled keys map to their source ids, custom keys are skipped", func(t *testing.T) {
		// given
		relationKeys := []domain.RelationKey{bundle.RelationKeyDueDate, "customKey"}
		typeKeys := []domain.TypeKey{bundle.TypeKeyTask, "customType"}
		want := []string{
			bundle.RelationKeyDueDate.BundledURL(),
			"_ot" + string(bundle.TypeKeyTask),
		}

		// when
		got := bundledIdsToInstall(relationKeys, typeKeys)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("nothing bundled yields an empty list", func(t *testing.T) {
		assert.Empty(t, bundledIdsToInstall([]domain.RelationKey{"x"}, []domain.TypeKey{"y"}))
	})
}

// docState builds the state a create document produces: a smartblock root
// with the given children, each a paragraph carrying its own id as text.
func docState(t *testing.T, rootId string, childIds ...string) *state.State {
	t.Helper()
	blocks := []*model.Block{{
		Id:          rootId,
		ChildrenIds: childIds,
		Content:     &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
	}}
	for _, id := range childIds {
		blocks = append(blocks, &model.Block{
			Id:      id,
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "doc " + id}},
		})
	}
	st, err := state.NewDocFromSnapshot(rootId, &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{Blocks: blocks}})
	require.NoError(t, err)
	return st
}

// templateState builds the state a template produces: a root whose children
// are the template's own blocks.
func templateState(t *testing.T, childIds ...string) *state.State {
	t.Helper()
	blocks := []*model.Block{{
		Id:          "tplRoot",
		ChildrenIds: childIds,
		Content:     &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
	}}
	for _, id := range childIds {
		blocks = append(blocks, &model.Block{
			Id:      id,
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "template " + id}},
		})
	}
	st, err := state.NewDocFromSnapshot("tplRoot", &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{Blocks: blocks}})
	require.NoError(t, err)
	return st
}

// blockText reads one block's text out of a state, for asserting WHICH block
// an id ended up naming.
func blockText(st *state.State, id string) string {
	b := st.Pick(id)
	if b == nil {
		return ""
	}
	return b.Model().GetText().GetText()
}

func TestMergeDocumentIntoTemplate(t *testing.T) {
	t.Run("the document's blocks land after the template's, in order", func(t *testing.T) {
		// given
		base := templateState(t, "header", "intro")
		doc := docState(t, "docRoot", "p1", "p2")

		// when
		mergeDocumentIntoTemplate(base, doc)

		// then
		assert.Equal(t, []string{"header", "intro", "p1", "p2"}, base.Pick(base.RootId()).Model().ChildrenIds)
		assert.Equal(t, "doc p1", blockText(base, "p1"))
		assert.Equal(t, "template intro", blockText(base, "intro"), "the template's own blocks survive")
	})

	t.Run("a document block whose id the template holds is reminted, references included", func(t *testing.T) {
		// given — `title` is an ordinary word for a caller and a real block id
		// in a template state; the collision must not overwrite the template's
		base := templateState(t, "title")
		doc := docState(t, "docRoot", "title", "after")
		// the caller's own block nests the colliding one
		doc.Get("after").Model().ChildrenIds = []string{"title"}
		doc.Get("docRoot").Model().ChildrenIds = []string{"after"}

		// when
		mergeDocumentIntoTemplate(base, doc)

		// then
		assert.Equal(t, "template title", blockText(base, "title"), "the template's block keeps the id")
		children := base.Pick(base.RootId()).Model().ChildrenIds
		require.Equal(t, []string{"title", "after"}, children)
		nested := base.Pick("after").Model().ChildrenIds
		require.Len(t, nested, 1)
		assert.NotEqual(t, "title", nested[0], "the caller's block was reminted")
		assert.Equal(t, "doc title", blockText(base, nested[0]), "and its parent points at the new id")
	})

	t.Run("relation links and collection items travel with the document", func(t *testing.T) {
		// given
		base := templateState(t, "intro")
		doc := docState(t, "docRoot", "p1")
		doc.AddRelationLinks(&model.RelationLink{Key: "severity", Format: model.RelationFormat_status})
		doc.UpdateStoreSlice("objects", []string{"obj1", "obj2"})

		// when
		mergeDocumentIntoTemplate(base, doc)

		// then
		assert.True(t, base.PickRelationLinks().Has("severity"), "a value whose link is missing is written as a removal")
		assert.Equal(t, []string{"obj1", "obj2"}, pbtypes.GetStringList(base.Store(), "objects"))
	})

	t.Run("a document with no blocks leaves the template untouched", func(t *testing.T) {
		// given
		base := templateState(t, "header", "intro")
		doc := docState(t, "docRoot")

		// when
		mergeDocumentIntoTemplate(base, doc)

		// then
		assert.Equal(t, []string{"header", "intro"}, base.Pick(base.RootId()).Model().ChildrenIds)
	})
}

// createAdapterFixture wires the create adapter over mocks, with one space
// holding a `memo` type whose recommended layout is note.
type createAdapterFixture struct {
	apicore.ObjectCreator
	creator   *mock_objectcreator.MockService
	templates *mock_template.MockService
	space     *mock_clientspace.MockSpace
	store     *objectstore.StoreFixture
}

func newCreateAdapterFixture(t *testing.T) *createAdapterFixture {
	creator := mock_objectcreator.NewMockService(t)
	templates := mock_template.NewMockService(t)
	spaces := mock_space.NewMockService(t)
	spc := mock_clientspace.NewMockSpace(t)
	store := objectstore.NewStoreFixture(t)
	require.NoError(t, store.WaitStoresLoaded(context.Background()))
	store.AddObjects(t, "space1", []objectstore.TestObject{{
		bundle.RelationKeyId:                domain.String("type-memo"),
		bundle.RelationKeyUniqueKey:         domain.String("ot-memo"),
		bundle.RelationKeyName:              domain.String("Memo"),
		bundle.RelationKeyRecommendedLayout: domain.Int64(int64(model.ObjectType_note)),
		bundle.RelationKeyResolvedLayout:    domain.Int64(int64(model.ObjectType_objectType)),
	}})
	spc.EXPECT().Id().Return("space1").Maybe()
	spaces.EXPECT().Get(mock.Anything, "space1").Return(spc, nil).Maybe()
	// the document names bundled relations (name), which the create path
	// installs before anything else
	creator.EXPECT().InstallBundledObjects(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil, nil).Maybe()
	return &createAdapterFixture{
		ObjectCreator: newObjectCreateAdapter(creator, spaces, templates, store),
		creator:       creator,
		templates:     templates,
		space:         spc,
		store:         store,
	}
}

// memoSnapshot is a create document of type memo with one paragraph.
func memoSnapshot() *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{
		Details: &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyName.String(): pbtypes.String("Monday"),
		}},
		ObjectTypes: []string{"memo"},
		Blocks: []*model.Block{
			{Id: "docRoot", ChildrenIds: []string{"p1"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "p1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "body"}}},
		},
	}
}

func TestCreateObjectFromSnapshotTemplate(t *testing.T) {
	t.Run("no template id leaves the template service out of it", func(t *testing.T) {
		// given
		fx := newCreateAdapterFixture(t)
		var created *state.State
		fx.creator.EXPECT().CreateSmartBlockFromStateInSpace(mock.Anything, mock.Anything, []domain.TypeKey{"memo"}, mock.Anything).
			RunAndReturn(func(_ context.Context, _ clientspace.Space, _ []domain.TypeKey, st *state.State) (string, *domain.Details, error) {
				created = st
				return "obj1", nil, nil
			})

		// when
		id, err := fx.CreateObjectFromSnapshot(context.Background(), "space1", memoSnapshot(), "")

		// then
		require.NoError(t, err)
		assert.Equal(t, "obj1", id)
		require.NotNil(t, created)
		assert.Equal(t, []string{"p1"}, created.Pick(created.RootId()).Model().ChildrenIds)
		assert.Equal(t, int64(model.ObjectOrigin_api), created.Details().GetInt64(bundle.RelationKeyOrigin))
	})

	t.Run("a template id builds the base state and the document lands on top", func(t *testing.T) {
		// given
		fx := newCreateAdapterFixture(t)
		fx.space.EXPECT().GetTypeIdByKey(mock.Anything, domain.TypeKey("memo")).Return("type-memo", nil)
		var request template.CreateTemplateRequest
		fx.templates.EXPECT().CreateTemplateStateWithDetails(mock.Anything).
			RunAndReturn(func(req template.CreateTemplateRequest) (*state.State, error) {
				request = req
				return templateState(t, "intro"), nil
			})
		var created *state.State
		fx.creator.EXPECT().CreateSmartBlockFromStateInSpace(mock.Anything, mock.Anything, []domain.TypeKey{"memo"}, mock.Anything).
			RunAndReturn(func(_ context.Context, _ clientspace.Space, _ []domain.TypeKey, st *state.State) (string, *domain.Details, error) {
				created = st
				return "obj1", nil, nil
			})

		// when
		id, err := fx.CreateObjectFromSnapshot(context.Background(), "space1", memoSnapshot(), "tpl-weekly")

		// then
		require.NoError(t, err)
		assert.Equal(t, "obj1", id)
		assert.Equal(t, "tpl-weekly", request.TemplateId)
		assert.Equal(t, "type-memo", request.TypeId)
		assert.Equal(t, "space1", request.SpaceId)
		assert.Equal(t, model.ObjectType_note, request.Layout, "the type's recommended layout, as a client create reads it")
		assert.False(t, request.WithTemplateValidation, "the id was validated before the response promised it")
		assert.Equal(t, "Monday", request.Details.GetString(bundle.RelationKeyName), "the caller's details are the template service's to merge")
		assert.Empty(t, request.Details.GetInt64(bundle.RelationKeyOrigin), "origin is this create's, not the document's")

		require.NotNil(t, created)
		assert.Equal(t, []string{"intro", "p1"}, created.Pick(created.RootId()).Model().ChildrenIds)
		assert.Equal(t, []domain.TypeKey{"memo"}, created.ObjectTypeKeys())
		assert.Equal(t, int64(model.ObjectOrigin_api), created.Details().GetInt64(bundle.RelationKeyOrigin))
	})

	t.Run("a template that cannot be built fails the create rather than silently emptying it", func(t *testing.T) {
		// given
		fx := newCreateAdapterFixture(t)
		fx.space.EXPECT().GetTypeIdByKey(mock.Anything, domain.TypeKey("memo")).Return("type-memo", nil)
		fx.templates.EXPECT().CreateTemplateStateWithDetails(mock.Anything).
			Return(nil, errors.New("template gone"))

		// when — no create expectation: nothing may be written
		_, err := fx.CreateObjectFromSnapshot(context.Background(), "space1", memoSnapshot(), "tpl-weekly")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tpl-weekly")
	})
}
