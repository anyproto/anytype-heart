package smartblock

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver/mock_idresolver"
	"github.com/anyproto/anytype-heart/core/block/simple"
	_ "github.com/anyproto/anytype-heart/core/block/simple/base"
	_ "github.com/anyproto/anytype-heart/core/block/simple/link"
	_ "github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/internalflag"
)

func TestSmartBlock_Init(t *testing.T) {
	// given
	id := "one"
	fx := newFixture(id, t)

	// when
	initCtx := fx.init(t, []*model.Block{{Id: id}})

	require.NotNil(t, initCtx)
	require.NotNil(t, initCtx.State)
	links := initCtx.State.GetRelationLinks()
	for _, key := range bundle.RequiredInternalRelations {
		assert.Truef(t, links.Has(key.String()), "missing relation %s", key)
	}
	// then
	assert.Equal(t, id, fx.RootId())
}

func TestSmartBlock_Apply(t *testing.T) {
	t.Run("no flags", func(t *testing.T) {
		// given
		fx := newFixture("", t)

		fx.init(t, []*model.Block{{Id: "1"}})
		s := fx.NewState()
		s.Add(simple.New(&model.Block{Id: "2"}))
		require.NoError(t, s.InsertTo("1", model.Block_Inner, "2"))
		var event *pb.Event
		ctx := session.NewContext()
		fx.RegisterSession(ctx)
		fx.eventSender.EXPECT().SendToSession(mock.Anything, mock.Anything).Run(func(token string, e *pb.Event) {
			event = e
		})
		fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil)

		// when
		err := fx.Apply(s)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, fx.History().Len())
		assert.NotNil(t, event)
	})

}

func TestBasic_SetAlign(t *testing.T) {
	t.Run("with ids", func(t *testing.T) {
		// given
		fx := newFixture("", t)

		fx.init(t, []*model.Block{
			{Id: "test", ChildrenIds: []string{"title", "2"}},
			{Id: "title"},
			{Id: "2"},
		})
		st := fx.NewState()

		// when
		err := st.SetAlign(model.Block_AlignRight, "2", "3")

		// then
		require.NoError(t, err)
		assert.Equal(t, model.Block_AlignRight, st.NewState().Get("2").Model().Align)
	})

	t.Run("without ids", func(t *testing.T) {
		// given
		fx := newFixture("", t)

		fx.init(t, []*model.Block{
			{Id: "test", ChildrenIds: []string{"title", "2"}},
			{Id: "title"},
			{Id: "2"},
		})
		st := fx.NewState()

		// when
		err := st.SetAlign(model.Block_AlignRight)

		// then
		require.NoError(t, err)
		assert.Equal(t, model.Block_AlignRight, st.Get("title").Model().Align)
		assert.Equal(t, int64(model.Block_AlignRight), st.Details().GetInt64(bundle.RelationKeyLayoutAlign))
	})

}

func Test_removeInternalFlags(t *testing.T) {
	t.Run("no flags - no changes", func(t *testing.T) {
		// given
		st := state.NewDoc("test", nil).(*state.State)
		st.SetDetail(bundle.RelationKeyInternalFlags, domain.Int64List([]int64{}))

		// when
		removeInternalFlags(st)

		// then
		assert.Empty(t, st.CombinedDetails().GetInt64List(bundle.RelationKeyInternalFlags))
	})
	t.Run("EmptyDelete flag is not removed when state is empty", func(t *testing.T) {
		// given
		st := state.NewDoc("test", nil).(*state.State)
		flags := defaultInternalFlags()
		flags.AddToState(st)

		// when
		removeInternalFlags(st)

		// then
		assert.Len(t, st.CombinedDetails().GetInt64List(bundle.RelationKeyInternalFlags), 1)
	})
	t.Run("all flags are removed when title is not empty", func(t *testing.T) {
		// given
		st := state.NewDoc("test", map[string]simple.Block{
			"title": simple.New(&model.Block{Id: "title"}),
		}).(*state.State)
		st.SetDetail(bundle.RelationKeyName, domain.String("some name"))
		flags := defaultInternalFlags()
		flags.AddToState(st)

		// when
		removeInternalFlags(st)

		// then
		assert.Empty(t, st.CombinedDetails().GetInt64List(bundle.RelationKeyInternalFlags))
	})
	t.Run("all flags are removed when state has non-empty text blocks", func(t *testing.T) {
		// given
		st := state.NewDoc("test", map[string]simple.Block{
			"test": simple.New(&model.Block{Id: "test", ChildrenIds: []string{"text"}}),
			"text": simple.New(&model.Block{Id: "text", Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Text: "some text"},
			}}),
		}).(*state.State)
		flags := defaultInternalFlags()
		flags.AddToState(st)

		// when
		removeInternalFlags(st)

		// then
		assert.Empty(t, st.CombinedDetails().GetInt64List(bundle.RelationKeyInternalFlags))
	})
}

type fixture struct {
	objectStore     *objectstore.StoreFixture
	store           spaceindex.Store
	indexer         *MockIndexer
	eventSender     *mock_event.MockSender
	source          *sourceStub
	spaceIdResolver *mock_idresolver.MockResolver
	space           *MockSpace

	*smartBlock
}

const testSpaceId = "space1"

func newFixture(id string, t *testing.T) *fixture {
	objectStore := objectstore.NewStoreFixture(t)
	spaceIndex := objectStore.SpaceIndex(testSpaceId)

	spaceIdResolver := mock_idresolver.NewMockResolver(t)

	space := NewMockSpace(t)

	indexer := NewMockIndexer(t)

	sender := mock_event.NewMockSender(t)

	sb := New(space, "", spaceIndex, objectStore, indexer, sender, spaceIdResolver).(*smartBlock)
	source := &sourceStub{
		id:      id,
		spaceId: "space1",
		sbType:  smartblock.SmartBlockTypePage,
	}
	sb.source = source

	return &fixture{
		source:          source,
		smartBlock:      sb,
		store:           spaceIndex,
		indexer:         indexer,
		eventSender:     sender,
		spaceIdResolver: spaceIdResolver,
		objectStore:     objectStore,
		space:           space,
	}
}

func (fx *fixture) init(t *testing.T, blocks []*model.Block) *InitContext {
	bm := make(map[string]simple.Block)
	for _, b := range blocks {
		bm[b.Id] = simple.New(b)
	}
	doc := state.NewDoc(fx.source.id, bm)
	fx.source.doc = doc

	initCtx := &InitContext{
		Ctx:     context.Background(),
		SpaceID: "space1",
		Source:  fx.source,
	}
	err := fx.Init(initCtx)
	require.NoError(t, err)
	return initCtx
}

type sourceStub struct {
	spaceId     string
	creator     string
	createdDate int64
	sbType      smartblock.SmartBlockType
	err         error
	doc         state.Doc
	id          string
}

func (s *sourceStub) GetCreationInfo() (creator string, createdDate int64, err error) {
	return s.creator, s.createdDate, s.err
}

func (s *sourceStub) Id() string                                { return s.id }
func (s *sourceStub) SpaceID() string                           { return s.spaceId }
func (s *sourceStub) Type() smartblock.SmartBlockType           { return s.sbType }
func (s *sourceStub) Heads() []string                           { return nil }
func (s *sourceStub) GetFileKeysSnapshot() []*pb.ChangeFileKeys { return nil }
func (s *sourceStub) ReadOnly() bool                            { return false }
func (s *sourceStub) Close() (err error)                        { return nil }
func (s *sourceStub) ReadDoc(_ context.Context, _ source.ChangeReceiver, _ bool) (doc state.Doc, err error) {
	return s.doc, nil
}
func (s *sourceStub) PushChange(_ source.PushChangeParams) (id string, err error) {
	return "", nil
}

func defaultInternalFlags() (flags internalflag.Set) {
	flags.Add(model.InternalFlag_editorDeleteEmpty)
	flags.Add(model.InternalFlag_editorSelectType)
	flags.Add(model.InternalFlag_editorSelectTemplate)
	return
}
