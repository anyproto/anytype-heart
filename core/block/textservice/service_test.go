package textservice

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/stext"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/blockbuilder"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type fixture struct {
	Service

	app          *app.App
	objectGetter *mock_cache.MockObjectGetterComponent
	eventSender  *mock_event.MockSender
}

const testFlushTimeout = 10 * time.Millisecond

func newFixture(t *testing.T) *fixture {
	ctx := context.Background()
	a := new(app.App)

	objectGetter := mock_cache.NewMockObjectGetterComponent(t)
	eventSender := mock_event.NewMockSender(t)
	objectStore := objectstore.NewStoreFixture(t)

	a.Register(objectStore)
	a.Register(testutil.PrepareMock(ctx, a, objectGetter))
	a.Register(testutil.PrepareMock(ctx, a, eventSender))

	svc := New(testFlushTimeout)
	err := svc.Init(a)
	require.NoError(t, err)

	return &fixture{
		Service:      svc,
		objectGetter: objectGetter,
		eventSender:  eventSender,
		app:          a,
	}
}

func newPage(a *app.App) *smarttest.SmartTest {
	sb := smarttest.New("text")

	text := stext.NewText(sb, a)
	textFlusher := stext.NewFlusher(sb, a)

	sb.AddComponent(text)
	sb.AddComponent(textFlusher)

	return sb
}

func newPageWithText(a *app.App, text string) *smarttest.SmartTest {
	sb := newPage(a)
	sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
		blockbuilder.ID("root"),
		blockbuilder.Children(
			blockbuilder.Text(
				text,
				blockbuilder.ID("1"),
			),
		)))
	return sb
}

func TestService_SetText(t *testing.T) {
	t.Run("flush after timeout", func(t *testing.T) {
		fx := newFixture(t)

		sb := newPageWithText(fx.app, "123")

		const objectId = "objectId"
		fx.objectGetter.EXPECT().GetObject(mock.Anything, objectId).Return(sb, nil)

		ctx := session.NewContext()
		fx.eventSender.EXPECT().BroadcastToOtherSessions(ctx.ID(), mock.Anything)

		err := fx.Service.SetText(ctx, pb.RpcBlockTextSetTextRequest{
			ContextId: objectId,
			BlockId:   "1",
			Text:      "456",
		})
		require.NoError(t, err)

		// Still not flushed
		sb.Lock()
		st := sb.NewState()
		assert.Equal(t, "123", st.Pick("1").Model().GetText().Text)
		sb.Unlock()

		time.Sleep(testFlushTimeout * 2)
		// Flushed
		sb.Lock()
		st = sb.NewState()
		assert.Equal(t, "456", st.Pick("1").Model().GetText().Text)
		sb.Unlock()
	})

	t.Run("title is changed", func(t *testing.T) {
		fx := newFixture(t)

		sb := newPage(fx.app)
		sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					"123",
					blockbuilder.ID("1"),
					blockbuilder.Fields(&types.Struct{
						Fields: map[string]*types.Value{
							text.DetailsKeyFieldName: pbtypes.StringList([]string{"name"}),
						},
					}),
				),
			)))

		const objectId = "objectId"
		fx.objectGetter.EXPECT().GetObject(mock.Anything, objectId).Return(sb, nil)

		ctx := session.NewContext()
		fx.eventSender.EXPECT().BroadcastToOtherSessions(ctx.ID(), mock.Anything)

		err := fx.Service.SetText(ctx, pb.RpcBlockTextSetTextRequest{
			ContextId: objectId,
			BlockId:   "1",
			Text:      "456",
		})
		require.NoError(t, err)

		// Flushed immediately as it's a block with relation
		sb.Lock()
		st := sb.NewState()
		assert.Equal(t, "456", st.Pick("1").Model().GetText().Text)
		sb.Unlock()
	})

	t.Run("mentions changed flushes immediately", func(t *testing.T) {
		fx := newFixture(t)

		sb := newPageWithText(fx.app, "Hello world")

		const objectId = "objectId"
		fx.objectGetter.EXPECT().GetObject(mock.Anything, objectId).Return(sb, nil)

		ctx := session.NewContext()
		fx.eventSender.EXPECT().BroadcastToOtherSessions(ctx.ID(), mock.Anything)

		// Add a mention to existing text
		err := fx.Service.SetText(ctx, pb.RpcBlockTextSetTextRequest{
			ContextId: objectId,
			BlockId:   "1",
			Text:      "Hello world",
			Marks: &model.BlockContentTextMarks{
				Marks: []*model.BlockContentTextMark{
					{
						Range: &model.Range{From: 0, To: 5},
						Type:  model.BlockContentTextMark_Mention,
						Param: "mentioned-object-id",
					},
				},
			},
		})
		require.NoError(t, err)

		// Should flush immediately due to mention change
		sb.Lock()
		st := sb.NewState()
		textBlock := st.Pick("1").Model().GetText()
		assert.Equal(t, "Hello world", textBlock.Text)
		assert.Len(t, textBlock.Marks.Marks, 1)
		assert.Equal(t, int32(0), textBlock.Marks.Marks[0].Range.From)
		assert.Equal(t, int32(5), textBlock.Marks.Marks[0].Range.To)
		assert.Equal(t, model.BlockContentTextMark_Mention, textBlock.Marks.Marks[0].Type)
		assert.Equal(t, "mentioned-object-id", textBlock.Marks.Marks[0].Param)
		sb.Unlock()
	})

	t.Run("mentions removed flushes immediately", func(t *testing.T) {
		fx := newFixture(t)

		// Start with text that has a mention
		sb := newPage(fx.app)
		sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					"Hello world",
					blockbuilder.ID("1"),
					blockbuilder.TextMarks(model.BlockContentTextMarks{
						Marks: []*model.BlockContentTextMark{
							{
								Range: &model.Range{From: 0, To: 5},
								Type:  model.BlockContentTextMark_Mention,
								Param: "mentioned-object-id",
							},
						},
					}),
				),
			)))

		const objectId = "objectId"
		fx.objectGetter.EXPECT().GetObject(mock.Anything, objectId).Return(sb, nil)

		ctx := session.NewContext()
		fx.eventSender.EXPECT().BroadcastToOtherSessions(ctx.ID(), mock.Anything)

		// Remove mention by setting text without marks
		err := fx.Service.SetText(ctx, pb.RpcBlockTextSetTextRequest{
			ContextId: objectId,
			BlockId:   "1",
			Text:      "Hello world",
		})
		require.NoError(t, err)

		// Should flush immediately due to mention removal
		sb.Lock()
		st := sb.NewState()
		textBlock := st.Pick("1").Model().GetText()
		assert.Equal(t, "Hello world", textBlock.Text)
		assert.Len(t, textBlock.Marks.Marks, 0)
		sb.Unlock()
	})

	t.Run("mentions changed to different objects flushes immediately", func(t *testing.T) {
		fx := newFixture(t)

		// Start with text that has a mention
		sb := newPage(fx.app)
		sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					"Hello world",
					blockbuilder.ID("1"),
					blockbuilder.TextMarks(model.BlockContentTextMarks{
						Marks: []*model.BlockContentTextMark{
							{
								Range: &model.Range{From: 0, To: 5},
								Type:  model.BlockContentTextMark_Mention,
								Param: "original-object-id",
							},
						},
					}),
				),
			)))

		const objectId = "objectId"
		fx.objectGetter.EXPECT().GetObject(mock.Anything, objectId).Return(sb, nil)

		ctx := session.NewContext()
		fx.eventSender.EXPECT().BroadcastToOtherSessions(ctx.ID(), mock.Anything)

		// Change mention to different object
		err := fx.Service.SetText(ctx, pb.RpcBlockTextSetTextRequest{
			ContextId: objectId,
			BlockId:   "1",
			Text:      "Hello world",
			Marks: &model.BlockContentTextMarks{
				Marks: []*model.BlockContentTextMark{
					{
						Range: &model.Range{From: 0, To: 5},
						Type:  model.BlockContentTextMark_Mention,
						Param: "new-object-id",
					},
				},
			},
		})
		require.NoError(t, err)

		// Should flush immediately due to mention change
		sb.Lock()
		st := sb.NewState()
		textBlock := st.Pick("1").Model().GetText()
		assert.Equal(t, "Hello world", textBlock.Text)
		assert.Len(t, textBlock.Marks.Marks, 1)
		assert.Equal(t, "new-object-id", textBlock.Marks.Marks[0].Param)
		sb.Unlock()
	})

	t.Run("no mention changes uses normal timeout", func(t *testing.T) {
		fx := newFixture(t)

		// Start with text that has a mention
		sb := newPage(fx.app)
		sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					"Hello world",
					blockbuilder.ID("1"),
					blockbuilder.TextMarks(model.BlockContentTextMarks{
						Marks: []*model.BlockContentTextMark{
							{
								Range: &model.Range{From: 0, To: 5},
								Type:  model.BlockContentTextMark_Mention,
								Param: "mentioned-object-id",
							},
						},
					}),
				),
			)))

		const objectId = "objectId"
		fx.objectGetter.EXPECT().GetObject(mock.Anything, objectId).Return(sb, nil)

		ctx := session.NewContext()
		fx.eventSender.EXPECT().BroadcastToOtherSessions(ctx.ID(), mock.Anything)

		// Change text but keep same mention
		err := fx.Service.SetText(ctx, pb.RpcBlockTextSetTextRequest{
			ContextId: objectId,
			BlockId:   "1",
			Text:      "Hello there",
			Marks: &model.BlockContentTextMarks{
				Marks: []*model.BlockContentTextMark{
					{
						Range: &model.Range{From: 0, To: 5},
						Type:  model.BlockContentTextMark_Mention,
						Param: "mentioned-object-id",
					},
				},
			},
		})
		require.NoError(t, err)

		// Should not flush immediately as mentions didn't change
		sb.Lock()
		st := sb.NewState()
		assert.Equal(t, "Hello world", st.Pick("1").Model().GetText().Text) // Old text still there
		sb.Unlock()

		time.Sleep(testFlushTimeout * 2)
		
		// Should flush after timeout
		sb.Lock()
		st = sb.NewState()
		textBlock := st.Pick("1").Model().GetText()
		assert.Equal(t, "Hello there", textBlock.Text)
		assert.Len(t, textBlock.Marks.Marks, 1)
		assert.Equal(t, "mentioned-object-id", textBlock.Marks.Marks[0].Param)
		sb.Unlock()
	})
}
