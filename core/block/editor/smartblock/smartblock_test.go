package smartblock

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/anytypeio/go-anytype-middleware/util/testMock/mockMeta"
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

func TestSmartBlock_Show(t *testing.T) {
	fx := newFixture(t)
	defer fx.tearDown()
	fx.init([]*model.Block{
		{Id: "1", ChildrenIds: []string{"2", "3"}},
		{Id: "2", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
			TargetBlockId: "22",
		}}},
		{Id: "3", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Marks: &model.BlockContentTextMarks{
				Marks: []*model.BlockContentTextMark{
					{Type: model.BlockContentTextMark_Mention, Param: "33"},
				},
			},
		}}},
	})

	fx.metaSubscriber.EXPECT().Callback(gomock.Any()).Return(fx.metaSubscriber)
	fx.metaSubscriber.EXPECT().Subscribe([]string{"1", "22", "33"})
	bm := meta.Meta{
		BlockId: "1",
		SmartBlockMeta: core.SmartBlockMeta{
			Details: fx.SmartBlock.(*smartBlock).Details(),
		},
	}
	fx.metaService.EXPECT().ReportChange(bm).Do(func(d meta.Meta) {
		go func() {
			fx.SmartBlock.(*smartBlock).onMetaChange(d)
			for _, id := range []string{"22", "33"} {
				fx.SmartBlock.(*smartBlock).onMetaChange(meta.Meta{
					BlockId:        id,
					SmartBlockMeta: core.SmartBlockMeta{},
				})
			}
		}()
	})

	ctx := state.NewContext(nil)
	err := fx.Show(ctx)
	require.NoError(t, err)

	msgs := ctx.GetMessages()
	require.Len(t, msgs, 1)
	msg := msgs[0].GetBlockShow()
	require.NotNil(t, msg)
	assert.Len(t, msg.Blocks, 3)
	assert.Len(t, msg.Details, 3)
	assert.Equal(t, "1", msg.RootId)
}

func TestSmartBlock_Apply(t *testing.T) {
	t.Run("no flags", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()
		fx.init([]*model.Block{{Id: "1"}})
		s := fx.NewState()
		s.Add(simple.New(&model.Block{Id: "2"}))
		require.NoError(t, s.InsertTo("1", model.Block_Inner, "2"))

		fx.source.EXPECT().PushChange(gomock.Any(), gomock.Any())
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

type fixture struct {
	t              *testing.T
	ctrl           *gomock.Controller
	source         *mockSource.MockSource
	metaSubscriber *mockMeta.MockSubscriber
	metaService    *mockMeta.MockService
	snapshot       *testMock.MockSmartBlockSnapshot
	SmartBlock
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	source := mockSource.NewMockSource(ctrl)
	source.EXPECT().Type().AnyTimes().Return(pb.SmartBlockType_Page)
	metaSubscriber := mockMeta.NewMockSubscriber(ctrl)
	metaPubSub := mockMeta.NewMockPubSub(ctrl)
	metaService := mockMeta.NewMockService(ctrl)
	metaService.EXPECT().PubSub().AnyTimes().Return(metaPubSub)
	metaPubSub.EXPECT().NewSubscriber().AnyTimes().Return(metaSubscriber)
	return &fixture{
		SmartBlock:     New(metaService),
		t:              t,
		ctrl:           ctrl,
		source:         source,
		metaSubscriber: metaSubscriber,
		metaService:    metaService,
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
	fx.source.EXPECT().ReadDoc(gomock.Any()).Return(doc, nil)
	fx.source.EXPECT().Id().Return(id).AnyTimes()

	err := fx.Init(fx.source, true)
	require.NoError(fx.t, err)
}
