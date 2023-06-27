package smartblock

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/testMock"
	"github.com/anyproto/anytype-heart/util/testMock/mockRelation"
	"github.com/anyproto/anytype-heart/util/testMock/mockSource"

	_ "github.com/anyproto/anytype-heart/core/block/simple/base"
	_ "github.com/anyproto/anytype-heart/core/block/simple/link"
	_ "github.com/anyproto/anytype-heart/core/block/simple/text"
)

func TestSmartBlock_Init(t *testing.T) {
	fx := newFixture(t)
	defer fx.tearDown()
	fx.init([]*model.Block{{Id: "one"}})
	assert.Equal(t, "one", fx.RootId())
}

func TestSmartBlock_Apply(t *testing.T) {
	t.Run("no flags", func(t *testing.T) {
		fx := newFixture(t)
		fx.at.EXPECT().PredefinedBlocks()
		defer fx.tearDown()

		fx.init([]*model.Block{{Id: "1"}})
		s := fx.NewState()
		s.Add(simple.New(&model.Block{Id: "2"}))
		require.NoError(t, s.InsertTo("1", model.Block_Inner, "2"))
		fx.source.EXPECT().ReadOnly()
		var event *pb.Event
		ctx := session.NewTestContext(t)
		fx.RegisterSession(ctx)
		// TODO Test Async context later
		ctx.Sender.EXPECT().SendToSession(mock.Anything, mock.Anything).Run(func(token string, e *pb.Event) {
			event = e
		})
		fx.source.EXPECT().Heads()
		fx.source.EXPECT().PushChange(gomock.Any()).Return("fake_change_id", nil)
		fx.indexer.EXPECT().Index(gomock.Any(), gomock.Any())
		err := fx.Apply(s)
		require.NoError(t, err)
		assert.Equal(t, 1, fx.History().Len())
		assert.NotNil(t, event)
	})

}

func TestBasic_SetAlign(t *testing.T) {
	t.Run("with ids", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()
		fx.init([]*model.Block{
			{Id: "test", ChildrenIds: []string{"title", "2"}},
			{Id: "title"},
			{Id: "2"},
		})

		st := fx.NewState()
		require.NoError(t, st.SetAlign(model.Block_AlignRight, "2", "3"))
		assert.Equal(t, model.Block_AlignRight, st.NewState().Get("2").Model().Align)
	})

	t.Run("without ids", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()
		fx.init([]*model.Block{
			{Id: "test", ChildrenIds: []string{"title", "2"}},
			{Id: "title"},
			{Id: "2"},
		})
		st := fx.NewState()
		require.NoError(t, st.SetAlign(model.Block_AlignRight))

		assert.Equal(t, model.Block_AlignRight, st.Get("title").Model().Align)
		assert.Equal(t, int64(model.Block_AlignRight), pbtypes.GetInt64(st.Details(), bundle.RelationKeyLayoutAlign.String()))
	})

}

type fixture struct {
	t       *testing.T
	ctrl    *gomock.Controller
	source  *mockSource.MockSource
	at      *testMock.MockService
	store   *testMock.MockObjectStore
	indexer *MockIndexer
	SmartBlock
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)

	coreService := testMock.NewMockService(ctrl)
	coreService.EXPECT().ProfileID().Return("").AnyTimes()
	coreService.EXPECT().GetWorkspaceIdForObject(gomock.Any()).AnyTimes()

	source := mockSource.NewMockSource(ctrl)
	source.EXPECT().Type().AnyTimes().Return(model.SmartBlockType_Page)

	objectStore := testMock.NewMockObjectStore(ctrl)
	objectStore.EXPECT().GetDetails(gomock.Any()).AnyTimes()
	objectStore.EXPECT().UpdatePendingLocalDetails(gomock.Any(), gomock.Any()).AnyTimes()

	objectStore.EXPECT().Name().Return(objectstore.CName).AnyTimes()

	indexer := NewMockIndexer(ctrl)
	indexer.EXPECT().Name().Return("indexer").AnyTimes()

	restrictionService := restriction.New(nil, nil)
	relationService := mockRelation.NewMockService(ctrl)

	fileService := testMock.NewMockFileService(ctrl)

	return &fixture{
		SmartBlock: New(coreService, fileService, restrictionService, objectStore, relationService, indexer),
		t:          t,
		at:         coreService,
		ctrl:       ctrl,
		store:      objectStore,
		source:     source,
		indexer:    indexer,
	}
}

func (fx *fixture) tearDown() {
	fx.ctrl.Finish()
}

func (fx *fixture) init(blocks []*model.Block) {
	id := blocks[0].Id
	bm := make(map[string]simple.Block)
	for _, b := range blocks {
		bm[b.Id] = simple.New(b)
	}
	doc := state.NewDoc(id, bm)
	fx.source.EXPECT().ReadDoc(context.Background(), gomock.Any(), false).Return(doc, nil)
	fx.source.EXPECT().Id().Return(id).AnyTimes()

	err := fx.Init(&InitContext{Source: fx.source})
	require.NoError(fx.t, err)
}
