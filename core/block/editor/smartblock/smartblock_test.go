package smartblock

import (
	"context"
	"testing"

	"github.com/anytypeio/any-sync/app"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/anytypeio/go-anytype-middleware/util/testMock/mockRelation"
	"github.com/anytypeio/go-anytype-middleware/util/testMock/mockSource"

	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
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
		fx.SetEventFunc(func(e *pb.Event) {
			event = e
		})
		fx.source.EXPECT().Heads()
		fx.source.EXPECT().PushChange(gomock.Any())
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
	app     *app.App
	source  *mockSource.MockSource
	at      *testMock.MockService
	store   *testMock.MockObjectStore
	indexer *MockIndexer
	SmartBlock
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)

	at := testMock.NewMockService(ctrl)
	at.EXPECT().ProfileID().Return("").AnyTimes()
	source := mockSource.NewMockSource(ctrl)
	source.EXPECT().Type().AnyTimes().Return(model.SmartBlockType_Page)
	source.EXPECT().Anytype().AnyTimes().Return(at)
	source.EXPECT().Virtual().AnyTimes().Return(false)
	store := testMock.NewMockObjectStore(ctrl)
	store.EXPECT().GetDetails(gomock.Any()).AnyTimes()
	store.EXPECT().UpdatePendingLocalDetails(gomock.Any(), gomock.Any()).AnyTimes()

	store.EXPECT().Name().Return(objectstore.CName).AnyTimes()

	indexer := NewMockIndexer(ctrl)
	indexer.EXPECT().Name().Return("indexer").AnyTimes()

	a := testapp.New()
	a.Register(store).
		Register(restriction.New(nil)).
		Register(indexer)

	mockRelation.RegisterMockRelation(ctrl, a)

	return &fixture{
		SmartBlock: New(),
		t:          t,
		at:         at,
		ctrl:       ctrl,
		store:      store,
		app:        a.App,
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

	err := fx.Init(&InitContext{Source: fx.source, App: fx.app})
	require.NoError(fx.t, err)
}
