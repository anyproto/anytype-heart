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

type setTextTestCase struct {
	name            string
	initialState    *blockbuilder.Block
	setTextRequest  pb.RpcBlockTextSetTextRequest
	shouldWaitFlush bool
	expectedText    string
	expectedMarks   *model.BlockContentTextMarks
}

func TestService_SetText(t *testing.T) {
	tests := []setTextTestCase{
		{
			name: "flush after timeout",
			initialState: blockbuilder.Root(
				blockbuilder.ID("root"),
				blockbuilder.Children(
					blockbuilder.Text(
						"123",
						blockbuilder.ID("1"),
					),
				)),
			setTextRequest: pb.RpcBlockTextSetTextRequest{
				ContextId: "objectId",
				BlockId:   "1",
				Text:      "456",
			},
			shouldWaitFlush: true,
			expectedText:    "456",
		},
		{
			name: "title is changed",
			initialState: blockbuilder.Root(
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
				)),
			setTextRequest: pb.RpcBlockTextSetTextRequest{
				ContextId: "objectId",
				BlockId:   "1",
				Text:      "456",
			},
			shouldWaitFlush: false,
			expectedText:    "456",
		},
		{
			name: "mentions changed flushes immediately",
			initialState: blockbuilder.Root(
				blockbuilder.ID("root"),
				blockbuilder.Children(
					blockbuilder.Text(
						"Hello world",
						blockbuilder.ID("1"),
					),
				)),
			setTextRequest: pb.RpcBlockTextSetTextRequest{
				ContextId: "objectId",
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
			},
			shouldWaitFlush: false,
			expectedText:    "Hello world",
			expectedMarks: &model.BlockContentTextMarks{
				Marks: []*model.BlockContentTextMark{
					{
						Range: &model.Range{From: 0, To: 5},
						Type:  model.BlockContentTextMark_Mention,
						Param: "mentioned-object-id",
					},
				},
			},
		},
		{
			name: "mentions removed flushes immediately",
			initialState: blockbuilder.Root(
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
				)),
			setTextRequest: pb.RpcBlockTextSetTextRequest{
				ContextId: "objectId",
				BlockId:   "1",
				Text:      "Hello world",
			},
			shouldWaitFlush: false,
			expectedText:    "Hello world",
			expectedMarks:   nil,
		},
		{
			name: "mentions changed to different objects flushes immediately",
			initialState: blockbuilder.Root(
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
				)),
			setTextRequest: pb.RpcBlockTextSetTextRequest{
				ContextId: "objectId",
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
			},
			shouldWaitFlush: false,
			expectedText:    "Hello world",
			expectedMarks: &model.BlockContentTextMarks{
				Marks: []*model.BlockContentTextMark{
					{
						Range: &model.Range{From: 0, To: 5},
						Type:  model.BlockContentTextMark_Mention,
						Param: "new-object-id",
					},
				},
			},
		},
		{
			name: "no mention changes uses normal timeout",
			initialState: blockbuilder.Root(
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
				)),
			setTextRequest: pb.RpcBlockTextSetTextRequest{
				ContextId: "objectId",
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
			},
			shouldWaitFlush: true,
			expectedText:    "Hello there",
			expectedMarks: &model.BlockContentTextMarks{
				Marks: []*model.BlockContentTextMark{
					{
						Range: &model.Range{From: 0, To: 5},
						Type:  model.BlockContentTextMark_Mention,
						Param: "mentioned-object-id",
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFixture(t)

			sb := newPage(fx.app)
			sb.Doc = testutil.BuildStateFromAST(tc.initialState)

			// Get initial text for comparison if we should wait for flush
			var initialText string
			if tc.shouldWaitFlush {
				sb.Lock()
				st := sb.NewState()
				initialText = st.Pick("1").Model().GetText().Text
				sb.Unlock()
			}

			fx.objectGetter.EXPECT().GetObject(mock.Anything, tc.setTextRequest.ContextId).Return(sb, nil)

			ctx := session.NewContext()
			fx.eventSender.EXPECT().BroadcastToOtherSessions(ctx.ID(), mock.Anything)

			err := fx.Service.SetText(ctx, tc.setTextRequest)
			require.NoError(t, err)

			if tc.shouldWaitFlush {
				// Should not flush immediately
				sb.Lock()
				st := sb.NewState()
				assert.Equal(t, initialText, st.Pick("1").Model().GetText().Text)
				sb.Unlock()

				time.Sleep(testFlushTimeout * 2)
			}

			// Check final state
			sb.Lock()
			st := sb.NewState()
			textBlock := st.Pick("1").Model().GetText()
			assert.Equal(t, tc.expectedText, textBlock.Text)

			if tc.expectedMarks != nil {
				assert.Equal(t, tc.expectedMarks, textBlock.Marks)
			}
			sb.Unlock()
		})
	}
}
