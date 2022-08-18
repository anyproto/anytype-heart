package smartblock

import (
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/util/testMock/mockDoc"
	"github.com/gogo/protobuf/types"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/anytypeio/go-anytype-middleware/util/testMock/mockSource"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		defer fx.tearDown()

		fx.init([]*model.Block{{Id: "1"}})
		s := fx.NewState()
		s.Add(simple.New(&model.Block{Id: "2"}))
		require.NoError(t, s.InsertTo("1", model.Block_Inner, "2"))
		fx.source.EXPECT().ReadOnly()
		fx.source.EXPECT().PushChange(gomock.Any())
		var event *pb.Event
		fx.SetEventFunc(func(e *pb.Event) {
			event = e
		})

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
		fx.source.EXPECT().ReadOnly().Return(false)
		fx.source.EXPECT().PushChange(gomock.Any())
		fx.init([]*model.Block{
			{Id: "test", ChildrenIds: []string{"title", "2"}},
			{Id: "title"},
			{Id: "2"},
		})
		require.NoError(t, fx.SetAlign(nil, model.Block_AlignRight, "2", "3"))
		assert.Equal(t, model.Block_AlignRight, fx.NewState().Get("2").Model().Align)
	})

	t.Run("without ids", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()
		fx.source.EXPECT().ReadOnly().Return(false)
		fx.source.EXPECT().PushChange(gomock.Any())
		fx.init([]*model.Block{
			{Id: "test", ChildrenIds: []string{"title", "2"}},
			{Id: "title"},
			{Id: "2"},
		})
		require.NoError(t, fx.SetAlign(nil, model.Block_AlignRight))
		assert.Equal(t, model.Block_AlignRight, fx.NewState().Get("title").Model().Align)
		assert.Equal(t, int64(model.Block_AlignRight), pbtypes.GetInt64(fx.NewState().Details(), bundle.RelationKeyLayoutAlign.String()))
	})

}

type fixture struct {
	t        *testing.T
	ctrl     *gomock.Controller
	app      *app.App
	source   *mockSource.MockSource
	snapshot *testMock.MockSmartBlockSnapshot
	store    *testMock.MockObjectStore
	md       *mockDoc.MockService
	SmartBlock
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)

	at := testMock.NewMockService(ctrl)
	at.EXPECT().ProfileID().Return("").AnyTimes()
	at.EXPECT().Account().Return("").AnyTimes()

	source := mockSource.NewMockSource(ctrl)
	source.EXPECT().Type().AnyTimes().Return(model.SmartBlockType_Page)
	source.EXPECT().Anytype().AnyTimes().Return(at)
	source.EXPECT().Virtual().AnyTimes().Return(false)
	store := testMock.NewMockObjectStore(ctrl)
	store.EXPECT().Name().Return(objectstore.CName).AnyTimes()
	a := testapp.New()
	a.Register(store).
		Register(restriction.New())
	md := mockDoc.RegisterMockDoc(ctrl, a)
	return &fixture{
		SmartBlock: New(),
		t:          t,
		ctrl:       ctrl,
		store:      store,
		app:        a.App,
		source:     source,
		md:         md,
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
	fx.source.EXPECT().LogHeads().Return(nil).AnyTimes()
	fx.store.EXPECT().GetDetails(id).Return(&model.ObjectDetails{
		Details: &types.Struct{Fields: map[string]*types.Value{}},
	}, nil)
	fx.md.EXPECT().ReportChange(gomock.Any(), gomock.Any()).AnyTimes()
	fx.store.EXPECT().GetPendingLocalDetails(id).Return(&model.ObjectDetails{
		Details: &types.Struct{Fields: map[string]*types.Value{}},
	}, nil)
	fx.store.EXPECT().UpdatePendingLocalDetails(id, nil).Return(nil)
	err := fx.Init(&InitContext{Source: fx.source, App: fx.app})
	require.NoError(fx.t, err)
}
