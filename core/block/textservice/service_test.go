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
}
