package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/chats/chatrepository"
	"github.com/anyproto/anytype-heart/core/block/chats/chatsubscription"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver/mock_idresolver"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

const hubSpaceId = "space1"

type hubAccountStub struct{}

func (hubAccountStub) AccountID() string     { return "identity1" }
func (hubAccountStub) Name() string          { return "hubAccountStub" }
func (hubAccountStub) Init(_ *app.App) error { return nil }

// spaceChatHubFixture runs the hub against the real subscription engine, chat
// state managers and chat repository. Its GetManager wraps the real one, so a
// test can make a chat's manager refuse and can count observer registrations.
type spaceChatHubFixture struct {
	*spaceChatHubs
	store    *objectstore.StoreFixture
	subs     subscription.Service
	chatSubs chatsubscription.Service
	repos    chatrepository.Service

	mu        sync.Mutex
	failing   map[string]error
	observers map[string]int // live observer registrations by observer id
}

func newSpaceChatHubFixture(t *testing.T) *spaceChatHubFixture {
	ctx := context.Background()
	a := &app.App{}
	subs := subscription.RegisterSubscriptionService(t, a)

	idResolver := mock_idresolver.NewMockResolver(t)
	idResolver.EXPECT().ResolveSpaceID(mock.Anything).Return(hubSpaceId, nil).Maybe()
	idResolver.EXPECT().ResolveSpaceIdWithRetry(mock.Anything, mock.Anything).Return(hubSpaceId, nil).Maybe()
	objectGetter := mock_cache.NewMockObjectWaitGetterComponent(t)
	objectGetter.EXPECT().WaitAndGetObject(mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	provider, err := anystoreprovider.NewInPath(t.TempDir())
	require.NoError(t, err)
	repos := chatrepository.New()
	chatSubs := chatsubscription.New()

	a.Register(hubAccountStub{})
	a.Register(testutil.PrepareMock(ctx, a, idResolver))
	a.Register(testutil.PrepareMock(ctx, a, objectGetter))
	a.Register(provider)
	a.Register(repos)
	a.Register(chatSubs)
	require.NoError(t, a.Start(ctx))
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = a.Close(closeCtx)
	})

	fx := &spaceChatHubFixture{
		store:     subs.StoreFixture,
		subs:      subs,
		chatSubs:  chatSubs,
		repos:     repos,
		failing:   map[string]error{},
		observers: map[string]int{},
	}
	fx.spaceChatHubs = newSpaceChatHubs(fx, subs, repos)
	return fx
}

func (fx *spaceChatHubFixture) GetManager(spaceId, chatId string) (chatsubscription.Manager, error) {
	fx.mu.Lock()
	err := fx.failing[chatId]
	fx.mu.Unlock()
	if err != nil {
		return nil, err
	}
	mngr, err := fx.chatSubs.GetManager(spaceId, chatId)
	if err != nil {
		return nil, err
	}
	return &countingManager{Manager: mngr, fx: fx, chatId: chatId}, nil
}

func (fx *spaceChatHubFixture) failManager(chatId string, err error) {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	fx.failing[chatId] = err
}

func (fx *spaceChatHubFixture) liveObservers() map[string]int {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	out := map[string]int{}
	for id, n := range fx.observers {
		if n != 0 {
			out[id] = n
		}
	}
	return out
}

// countingManager counts observer registrations per chat and observer id.
type countingManager struct {
	chatsubscription.Manager
	fx     *spaceChatHubFixture
	chatId string
}

func (m *countingManager) AddObserver(id string, fn chatsubscription.ChatObserver) {
	m.Manager.AddObserver(id, fn)
	m.fx.mu.Lock()
	m.fx.observers[m.chatId+"/"+id] = 1
	m.fx.mu.Unlock()
}

func (m *countingManager) RemoveObserver(id string) {
	m.Manager.RemoveObserver(id)
	m.fx.mu.Lock()
	delete(m.fx.observers, m.chatId+"/"+id)
	m.fx.mu.Unlock()
}

func (fx *spaceChatHubFixture) open(t *testing.T, includeDiscussions bool) apicore.SpaceChatSubscription {
	t.Helper()
	sub, err := fx.Open(context.Background(), apicore.SpaceChatOpen{SpaceId: hubSpaceId, IncludeDiscussions: includeDiscussions, Render: renderTestFrame})
	require.NoError(t, err)
	t.Cleanup(sub.Close)
	return sub
}

func (fx *spaceChatHubFixture) hub() *spaceChatHub {
	slot := fx.slot(hubSpaceId)
	slot.mu.Lock()
	defer slot.mu.Unlock()
	return slot.hub
}

func (fx *spaceChatHubFixture) manager(t *testing.T, chatId string) chatsubscription.Manager {
	t.Helper()
	mngr, err := fx.chatSubs.GetManager(hubSpaceId, chatId)
	require.NoError(t, err)
	return mngr
}

// apply runs fn under the chat manager's lock, the way the chat object does.
func (fx *spaceChatHubFixture) apply(t *testing.T, chatId string, fn func(m chatsubscription.Manager)) {
	t.Helper()
	mngr := fx.manager(t, chatId)
	mngr.Lock()
	defer mngr.Unlock()
	fn(mngr)
}

func (fx *spaceChatHubFixture) addMessages(t *testing.T, chatId string, msgs ...*chatmodel.Message) {
	t.Helper()
	fx.apply(t, chatId, func(m chatsubscription.Manager) {
		for _, msg := range msgs {
			m.Add("", msg)
		}
	})
}

func (fx *spaceChatHubFixture) ourSubscriptions() []string {
	var ours []string
	for _, id := range fx.subs.SubscriptionIDs() {
		if strings.HasPrefix(id, "api-space-chats-") {
			ours = append(ours, id)
		}
	}
	return ours
}

func givenHubChat(id, name string) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_chatDerived)),
		bundle.RelationKeyName:           domain.String(name),
	}
}

func givenHubDiscussion(id string) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_discussion)),
	}
}

func givenHubParent(id, name, discussionId string) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		bundle.RelationKeyName:           domain.String(name),
		bundle.RelationKeyDiscussionId:   domain.String(discussionId),
	}
}

func givenHubMessage(id, orderId, text string) *chatmodel.Message {
	return &chatmodel.Message{ChatMessage: &model.ChatMessage{
		Id:      id,
		OrderId: orderId,
		StateId: "state-" + id,
		Creator: "identity2",
		Message: &model.ChatMessageMessageContent{Text: text},
	}}
}

// testFrame is the test renderer's wire form: just enough of a change to
// assert on.
type testFrame struct {
	Type        string `json:"type"`
	ChatId      string `json:"chat_id,omitempty"`
	Name        string `json:"name,omitempty"`
	Discussion  bool   `json:"discussion,omitempty"`
	IsMain      bool   `json:"is_main,omitempty"`
	ParentId    string `json:"parent_id,omitempty"`
	MessageId   string `json:"message_id,omitempty"`
	Text        string `json:"text,omitempty"`
	Unread      int32  `json:"unread,omitempty"`
	LastStateId string `json:"last_state_id,omitempty"`
}

var testChangeNames = map[apicore.SpaceChatChangeType]string{
	apicore.SpaceChatAdded:            "chat_added",
	apicore.SpaceChatSnapshotComplete: "snapshot_complete",
	apicore.SpaceChatUpdated:          "chat_updated",
	apicore.SpaceChatRemoved:          "chat_removed",
	apicore.SpaceChatStateUpdated:     "state_updated",
	apicore.SpaceChatMessageAdded:     "message_added",
	apicore.SpaceChatMessageUpdated:   "message_updated",
	apicore.SpaceChatMessageDeleted:   "message_deleted",
	apicore.SpaceChatReactionsUpdated: "reactions_updated",
	apicore.SpaceChatPinnedUpdated:    "pinned_updated",
}

func renderTestFrame(change apicore.SpaceChatChange) (apicore.SpaceChatFrame, error) {
	frame := testFrame{
		Type:       testChangeNames[change.Type],
		ChatId:     change.Chat.Id,
		Name:       change.Chat.Name,
		Discussion: change.Chat.Discussion,
		IsMain:     change.Chat.IsMain,
		ParentId:   change.Chat.ParentId,
		MessageId:  change.MessageId,
	}
	if change.Message != nil {
		frame.Text = change.Message.GetMessage().GetText()
	}
	if change.State != nil {
		frame.Unread = change.State.GetMessages().GetCounter()
		frame.LastStateId = change.State.GetLastStateId()
	}
	data, err := json.Marshal(frame)
	if err != nil {
		return apicore.SpaceChatFrame{}, fmt.Errorf("marshal test frame: %w", err)
	}
	return apicore.SpaceChatFrame{Type: frame.Type, Data: data}, nil
}

func decodeFrames(t *testing.T, frames []apicore.SpaceChatFrame) []testFrame {
	t.Helper()
	out := make([]testFrame, 0, len(frames))
	for _, frame := range frames {
		var decoded testFrame
		require.NoError(t, json.Unmarshal(frame.Data, &decoded))
		require.Equal(t, frame.Type, decoded.Type)
		out = append(out, decoded)
	}
	return out
}

// waitFrames reads live frames until n have arrived.
func waitFrames(t *testing.T, sub apicore.SpaceChatSubscription, n int) []testFrame {
	t.Helper()
	deadline := time.After(3 * time.Second)
	var got []testFrame
	for len(got) < n {
		select {
		case <-sub.Ready():
			got = append(got, decodeFrames(t, sub.Drain())...)
		case <-deadline:
			t.Fatalf("got %d of %d frames: %+v", len(got), n, got)
		}
	}
	return got
}

// assertNoFrames waits a little and asserts nothing more arrived.
func assertNoFrames(t *testing.T, sub apicore.SpaceChatSubscription) {
	t.Helper()
	select {
	case <-sub.Ready():
		assert.Empty(t, decodeFrames(t, sub.Drain()))
	case <-time.After(150 * time.Millisecond):
	}
}

// settle waits until the hub worker has applied everything queued so far, by
// sending a probe chat change through it and waiting for its frame.
func settle(t *testing.T, fx *spaceChatHubFixture, sub apicore.SpaceChatSubscription, probeChatId string) []testFrame {
	t.Helper()
	probe := givenHubMessage(fmt.Sprintf("probe-%d", time.Now().UnixNano()), "zz", "probe")
	fx.addMessages(t, probeChatId, probe)
	var got []testFrame
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-sub.Ready():
			for _, frame := range decodeFrames(t, sub.Drain()) {
				if frame.MessageId == probe.Id {
					return got
				}
				got = append(got, frame)
			}
		case <-deadline:
			t.Fatalf("the probe never arrived; got %+v", got)
		}
	}
}

func TestSpaceChatHubSnapshot(t *testing.T) {
	t.Run("an empty space opens with snapshot_complete alone", func(t *testing.T) {
		// given
		fx := newSpaceChatHubFixture(t)
		want := []testFrame{{Type: "snapshot_complete"}}

		// when
		sub := fx.open(t, true)

		// then
		assert.Equal(t, want, decodeFrames(t, sub.Snapshot()))
	})

	t.Run("the snapshot lists every eligible chat with its counters, then snapshot_complete", func(t *testing.T) {
		// given
		fx := newSpaceChatHubFixture(t)
		hidden := givenHubChat("chat-hidden", "Hidden")
		hidden[bundle.RelationKeyIsHidden] = domain.Bool(true)
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{
			givenHubChat("chat1", "General"),
			hidden,
			givenHubDiscussion("disc1"),
			givenHubParent("page1", "Plan", "disc1"),
			// a discussion object whose parent is not here is not eligible
			givenHubDiscussion("disc-orphan"),
			// nor is a parent whose discussion has not synced
			givenHubParent("page2", "Draft", "disc-missing"),
		})
		fx.apply(t, "chat1", func(m chatsubscription.Manager) {
			m.UpdateChatState(func(state *model.ChatState) *model.ChatState {
				state.Messages.Counter = 2
				state.LastStateId = "state7"
				return state
			})
		})
		want := []testFrame{
			{Type: "chat_added", ChatId: "chat1", Name: "General", Unread: 2, LastStateId: "state7"},
			{Type: "chat_added", ChatId: "disc1", Name: "Plan", Discussion: true, ParentId: "page1"},
			{Type: "snapshot_complete"},
		}

		// when
		sub := fx.open(t, true)

		// then
		assert.Equal(t, want, decodeFrames(t, sub.Snapshot()))
	})

	t.Run("the space's main chat is marked, and no other chat", func(t *testing.T) {
		// given
		fx := newSpaceChatHubFixture(t)
		mainChat := givenHubChat("chat1", "General")
		mainChat[bundle.RelationKeyIsMainChat] = domain.Bool(true)
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{
			mainChat,
			givenHubChat("chat2", "Team"),
			givenHubDiscussion("disc1"),
			givenHubParent("page1", "Plan", "disc1"),
		})
		want := []testFrame{
			{Type: "chat_added", ChatId: "chat1", Name: "General", IsMain: true},
			{Type: "chat_added", ChatId: "chat2", Name: "Team"},
			{Type: "chat_added", ChatId: "disc1", Name: "Plan", Discussion: true, ParentId: "page1"},
			{Type: "snapshot_complete"},
		}

		// when
		sub := fx.open(t, true)

		// then
		assert.Equal(t, want, decodeFrames(t, sub.Snapshot()))
	})

	t.Run("a client without discussions gets chats only", func(t *testing.T) {
		// given
		fx := newSpaceChatHubFixture(t)
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{
			givenHubChat("chat1", "General"),
			givenHubDiscussion("disc1"),
			givenHubParent("page1", "Plan", "disc1"),
		})
		want := []testFrame{
			{Type: "chat_added", ChatId: "chat1", Name: "General"},
			{Type: "snapshot_complete"},
		}

		// when
		sub := fx.open(t, false)

		// then
		assert.Equal(t, want, decodeFrames(t, sub.Snapshot()))
	})
}

func TestSpaceChatHubLive(t *testing.T) {
	t.Run("a burst of messages yields one event per message, in order", func(t *testing.T) {
		// given
		fx := newSpaceChatHubFixture(t)
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{givenHubChat("chat1", "General")})
		sub := fx.open(t, true)
		var burst []*chatmodel.Message
		var want []testFrame
		for i := 0; i < 30; i++ {
			msg := givenHubMessage(fmt.Sprintf("m%02d", i), fmt.Sprintf("o%02d", i), fmt.Sprintf("text %d", i))
			burst = append(burst, msg)
			want = append(want, testFrame{Type: "message_added", ChatId: "chat1", Name: "General", MessageId: msg.Id, Text: msg.Message.Text})
		}

		// when: one lock hold, the way a synced batch applies
		fx.addMessages(t, "chat1", burst...)

		// then: nothing is coalesced or evicted, unlike the preview window
		assert.Equal(t, want, waitFrames(t, sub, len(want)))
	})

	t.Run("a late-synced older message is delivered", func(t *testing.T) {
		// given
		fx := newSpaceChatHubFixture(t)
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{givenHubChat("chat1", "General")})
		sub := fx.open(t, true)
		want := []testFrame{
			{Type: "message_added", ChatId: "chat1", Name: "General", MessageId: "new", Text: "new"},
			{Type: "message_added", ChatId: "chat1", Name: "General", MessageId: "old", Text: "old"},
		}

		// when: the older order id arrives after the newer one
		fx.addMessages(t, "chat1", givenHubMessage("new", "o9", "new"))
		fx.addMessages(t, "chat1", givenHubMessage("old", "o1", "old"))

		// then
		assert.Equal(t, want, waitFrames(t, sub, len(want)))
	})

	t.Run("every message change and every counter change reaches the client", func(t *testing.T) {
		// given
		fx := newSpaceChatHubFixture(t)
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{givenHubChat("chat1", "General")})
		sub := fx.open(t, true)
		chat := testFrame{ChatId: "chat1", Name: "General"}
		with := func(f testFrame) testFrame {
			f.ChatId, f.Name = chat.ChatId, chat.Name
			return f
		}
		want := []testFrame{
			with(testFrame{Type: "state_updated", Unread: 1, LastStateId: "state-m1"}),
			with(testFrame{Type: "message_added", MessageId: "m1", Text: "hello"}),
			with(testFrame{Type: "message_updated", MessageId: "m1", Text: "hello, edited"}),
			with(testFrame{Type: "reactions_updated", MessageId: "m1", Text: "hello, edited"}),
			with(testFrame{Type: "pinned_updated", MessageId: "m1", Text: "hello, edited"}),
			with(testFrame{Type: "message_deleted", MessageId: "m1"}),
		}

		// when
		fx.apply(t, "chat1", func(m chatsubscription.Manager) {
			m.UpdateChatState(func(state *model.ChatState) *model.ChatState {
				state.Messages.Counter = 1
				state.LastStateId = "state-m1"
				return state
			})
			// a watermark move leaves the counters as they are: not an event
			m.UpdateChatState(func(state *model.ChatState) *model.ChatState {
				state.Messages.OldestOrderId = "o1"
				return state
			})
			m.Add("", givenHubMessage("m1", "o1", "hello"))
			m.UpdateFull(givenHubMessage("m1", "o1", "hello, edited"))
			m.UpdateReactions(givenHubMessage("m1", "o1", "hello, edited"))
			m.UpdatePinned(givenHubMessage("m1", "o1", "hello, edited"))
			m.Delete("m1")
		})

		// then
		assert.Equal(t, want, waitFrames(t, sub, len(want)))
		assertNoFrames(t, sub)
	})

	t.Run("a chat appearing after open is announced, then backfilled from before its observer existed", func(t *testing.T) {
		// given: a client is already open
		fx := newSpaceChatHubFixture(t)
		sub := fx.open(t, true)
		// the discussion applies its first comment BEFORE discovery reaches the
		// hub: stored in the repository and passed through a manager nobody
		// observes yet
		repo, err := fx.repos.Repository(hubSpaceId, "disc1")
		require.NoError(t, err)
		first := givenHubMessage("c1", "o1", "first comment")
		require.NoError(t, repo.AddTestMessage(context.Background(), first))
		fx.addMessages(t, "disc1", first)
		want := []testFrame{
			// the counters are the manager's, loaded from the repository
			{Type: "chat_added", ChatId: "disc1", Name: "Agent output", Discussion: true, ParentId: "page1", Unread: 1, LastStateId: "state-c1"},
			{Type: "message_added", ChatId: "disc1", Name: "Agent output", Discussion: true, ParentId: "page1", MessageId: "c1", Text: "first comment"},
		}

		// when
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{
			givenHubDiscussion("disc1"),
			givenHubParent("page1", "Agent output", "disc1"),
		})

		// then
		assert.Equal(t, want, waitFrames(t, sub, len(want)))
	})

	t.Run("the backfill is the newest fifty messages", func(t *testing.T) {
		// given
		fx := newSpaceChatHubFixture(t)
		sub := fx.open(t, true)
		repo, err := fx.repos.Repository(hubSpaceId, "chat2")
		require.NoError(t, err)
		for i := 0; i < 60; i++ {
			require.NoError(t, repo.AddTestMessage(context.Background(), givenHubMessage(fmt.Sprintf("m%02d", i), fmt.Sprintf("o%02d", i), "x")))
		}

		// when
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{givenHubChat("chat2", "Late")})

		// then
		got := waitFrames(t, sub, 1+spaceChatBackfillLimit)
		assert.Equal(t, "chat_added", got[0].Type)
		assert.Equal(t, "m10", got[1].MessageId, "the oldest of the newest fifty")
		assert.Equal(t, "m59", got[len(got)-1].MessageId)
	})

	t.Run("eligibility follows the parent: renamed, then archived", func(t *testing.T) {
		// given
		fx := newSpaceChatHubFixture(t)
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{
			givenHubChat("chat1", "General"),
			givenHubDiscussion("disc1"),
			givenHubParent("page1", "Plan", "disc1"),
		})
		sub := fx.open(t, true)
		archived := givenHubParent("page1", "Plan v2", "disc1")
		archived[bundle.RelationKeyIsArchived] = domain.Bool(true)

		// when: the parent is renamed
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{givenHubParent("page1", "Plan v2", "disc1")})

		// then
		assert.Equal(t, []testFrame{
			{Type: "chat_updated", ChatId: "disc1", Name: "Plan v2", Discussion: true, ParentId: "page1"},
		}, waitFrames(t, sub, 1))

		// when: the parent is archived
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{archived})

		// then: the discussion leaves, and its later messages are not delivered
		assert.Equal(t, []testFrame{
			{Type: "chat_removed", ChatId: "disc1", Name: "Plan v2", Discussion: true, ParentId: "page1"},
		}, waitFrames(t, sub, 1))
		fx.addMessages(t, "disc1", givenHubMessage("late", "o5", "after removal"))
		assert.Empty(t, settle(t, fx, sub, "chat1"))
		assert.NotContains(t, fx.liveObservers(), "disc1/"+fx.hub().observerId, "a removed chat's observer is detached")
	})

	t.Run("a chat that becomes the main chat is updated", func(t *testing.T) {
		// given: the space chat of a space created before isMainChat gets it on its next open
		fx := newSpaceChatHubFixture(t)
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{givenHubChat("chat1", "General")})
		sub := fx.open(t, true)
		mainChat := givenHubChat("chat1", "General")
		mainChat[bundle.RelationKeyIsMainChat] = domain.Bool(true)

		// when
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{mainChat})

		// then
		assert.Equal(t, []testFrame{{Type: "chat_updated", ChatId: "chat1", Name: "General", IsMain: true}}, waitFrames(t, sub, 1))
	})

	t.Run("a chat that becomes hidden is removed", func(t *testing.T) {
		// given
		fx := newSpaceChatHubFixture(t)
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{givenHubChat("chat1", "General")})
		sub := fx.open(t, true)
		hidden := givenHubChat("chat1", "General")
		hidden[bundle.RelationKeyIsHidden] = domain.Bool(true)

		// when
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{hidden})

		// then
		assert.Equal(t, []testFrame{{Type: "chat_removed", ChatId: "chat1", Name: "General"}}, waitFrames(t, sub, 1))
	})

	t.Run("each client's include predicate filters discovery and messages", func(t *testing.T) {
		// given: two clients on one hub
		fx := newSpaceChatHubFixture(t)
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{
			givenHubChat("chat1", "General"),
			givenHubDiscussion("disc1"),
			givenHubParent("page1", "Plan", "disc1"),
		})
		withDiscussions := fx.open(t, true)
		chatsOnly := fx.open(t, false)
		require.Same(t, fx.hub(), fx.hub())

		// when
		fx.addMessages(t, "disc1", givenHubMessage("d1", "o1", "in a discussion"))
		fx.addMessages(t, "chat1", givenHubMessage("c1", "o1", "in a chat"))

		// then
		assert.Equal(t, []string{"d1", "c1"}, messageIds(waitFrames(t, withDiscussions, 2)))
		assert.Equal(t, []string{"c1"}, messageIds(waitFrames(t, chatsOnly, 1)))
		assertNoFrames(t, chatsOnly)
	})

	t.Run("a client that never reads does not block the manager", func(t *testing.T) {
		// given: a client that never drains its queue
		fx := newSpaceChatHubFixture(t)
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{givenHubChat("chat1", "General")})
		sub := fx.open(t, true)
		const n = 5000

		// when
		done := make(chan struct{})
		go func() {
			defer close(done)
			for i := 0; i < n; i++ {
				fx.addMessages(t, "chat1", givenHubMessage(fmt.Sprintf("m%d", i), fmt.Sprintf("o%06d", i), "x"))
			}
		}()

		// then: the chat keeps applying, and the client's queue just grows
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("the manager stalled behind a client that does not read")
		}
		client := sub.(*spaceChatClient)
		assert.Eventually(t, func() bool { return client.queue.len() == n }, 5*time.Second, 10*time.Millisecond)
	})
}

func messageIds(frames []testFrame) []string {
	ids := make([]string, 0, len(frames))
	for _, frame := range frames {
		ids = append(ids, frame.MessageId)
	}
	return ids
}

func TestSpaceChatHubLifecycle(t *testing.T) {
	t.Run("the first client creates the hub and the last one tears it down", func(t *testing.T) {
		// given
		fx := newSpaceChatHubFixture(t)
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{givenHubChat("chat1", "General")})
		first := fx.open(t, true)
		hub := fx.hub()
		second := fx.open(t, false)
		require.Same(t, hub, fx.hub(), "the second client joins the running hub")
		require.Len(t, fx.ourSubscriptions(), 2)
		require.Len(t, fx.liveObservers(), 1)

		// when: one client leaves
		first.Close()

		// then: the hub keeps serving the other
		assert.Same(t, hub, fx.hub())
		fx.addMessages(t, "chat1", givenHubMessage("m1", "o1", "still here"))
		assert.Equal(t, []string{"m1"}, messageIds(waitFrames(t, second, 1)))

		// when: the last client leaves
		second.Close()

		// then: subscriptions, observers and queues are gone
		assert.Nil(t, fx.hub())
		assert.Empty(t, fx.ourSubscriptions())
		assert.Empty(t, fx.liveObservers())
		hub.queue.push(hubItem{chatId: "chat1"})
		assert.Zero(t, hub.queue.len(), "the hub queue is closed")
		assert.Error(t, hub.discovery.Add(context.Background(), nil), "the owned discovery queue is closed")
		assert.ErrorIs(t, hub.ctx.Err(), context.Canceled, "the workers were cancelled")
	})

	t.Run("closing twice releases once", func(t *testing.T) {
		// given
		fx := newSpaceChatHubFixture(t)
		first := fx.open(t, true)
		second := fx.open(t, true)
		hub := fx.hub()

		// when
		first.Close()
		first.Close()

		// then: the second client still holds the hub
		assert.Same(t, hub, fx.hub())
		second.Close()
		assert.Nil(t, fx.hub())
	})

	t.Run("a reopened space gets a new generation", func(t *testing.T) {
		// given
		fx := newSpaceChatHubFixture(t)
		first := fx.open(t, true)
		firstHub := fx.hub()
		first.Close()

		// when
		fx.open(t, true)

		// then
		assert.NotEqual(t, firstHub.generation, fx.hub().generation)
		assert.NotEqual(t, firstHub.observerId, fx.hub().observerId)
		assert.NotEqual(t, firstHub.chatsSubId, fx.hub().chatsSubId)
	})

	t.Run("an old hub's teardown cannot remove a newer hub's registrations", func(t *testing.T) {
		// given: generation 1 is detached from its slot but not torn down yet,
		// exactly the window leave opens, and generation 2 starts meanwhile
		fx := newSpaceChatHubFixture(t)
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{givenHubChat("chat1", "General")})
		fx.open(t, true)
		old := fx.hub()
		slot := fx.slot(hubSpaceId)
		slot.mu.Lock()
		slot.hub = nil
		slot.mu.Unlock()
		sub := fx.open(t, true)
		require.NotSame(t, old, fx.hub())

		// when
		old.close()

		// then: the new hub still observes chats and still discovers them
		fx.addMessages(t, "chat1", givenHubMessage("m1", "o1", "after the old teardown"))
		assert.Equal(t, []string{"m1"}, messageIds(waitFrames(t, sub, 1)))
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{givenHubChat("chat2", "New")})
		assert.Equal(t, "chat_added", waitFrames(t, sub, 1)[0].Type)
		assert.Len(t, fx.ourSubscriptions(), 2)
	})

	t.Run("concurrent opens and closes leave nothing behind", func(t *testing.T) {
		// given
		fx := newSpaceChatHubFixture(t)
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{
			givenHubChat("chat1", "General"),
			givenHubDiscussion("disc1"),
			givenHubParent("page1", "Plan", "disc1"),
		})

		// when
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					sub, err := fx.Open(context.Background(), apicore.SpaceChatOpen{SpaceId: hubSpaceId, IncludeDiscussions: i%2 == 0, Render: renderTestFrame})
					if !assert.NoError(t, err) {
						return
					}
					sub.Drain()
					sub.Close()
				}
			}(i)
		}
		wg.Wait()

		// then
		assert.Nil(t, fx.hub())
		assert.Empty(t, fx.ourSubscriptions())
		assert.Empty(t, fx.liveObservers())
	})

	t.Run("a client that leaves during the start leaves no hub behind", func(t *testing.T) {
		// given
		fx := newSpaceChatHubFixture(t)
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{givenHubChat("chat1", "General")})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// when
		_, err := fx.Open(ctx, apicore.SpaceChatOpen{SpaceId: hubSpaceId, Render: renderTestFrame})

		// then
		require.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, fx.hub())
		assert.Empty(t, fx.ourSubscriptions())
		assert.Empty(t, fx.liveObservers())
	})
}

func TestSpaceChatHubFailures(t *testing.T) {
	t.Run("a manager that fails to initialize fails the open, naming the chat", func(t *testing.T) {
		// given
		fx := newSpaceChatHubFixture(t)
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{
			givenHubChat("chat1", "General"),
			givenHubChat("chat2", "Broken"),
		})
		fx.failManager("chat2", errors.New("init chat state: corrupted"))

		// when
		_, err := fx.Open(context.Background(), apicore.SpaceChatOpen{SpaceId: hubSpaceId, IncludeDiscussions: true, Render: renderTestFrame})

		// then
		var attachErr *apicore.SpaceChatAttachError
		require.ErrorAs(t, err, &attachErr)
		assert.Equal(t, "chat2", attachErr.ChatId)
		assert.Contains(t, err.Error(), "corrupted")
		assert.Nil(t, fx.hub(), "a refused first open leaves no hub")
		assert.Empty(t, fx.ourSubscriptions())
		assert.Empty(t, fx.liveObservers())
	})

	t.Run("a broken discussion does not refuse a client that does not receive discussions", func(t *testing.T) {
		// given
		fx := newSpaceChatHubFixture(t)
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{
			givenHubChat("chat1", "General"),
			givenHubDiscussion("disc1"),
			givenHubParent("page1", "Plan", "disc1"),
		})
		fx.failManager("disc1", errors.New("broken"))

		// when
		sub := fx.open(t, false)

		// then
		assert.Equal(t, []string{"chat_added", "snapshot_complete"}, frameTypes(decodeFrames(t, sub.Snapshot())))
	})

	t.Run("a chat that fails to attach after open is skipped, and later opens are refused", func(t *testing.T) {
		// given
		fx := newSpaceChatHubFixture(t)
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{givenHubChat("chat1", "General")})
		sub := fx.open(t, true)
		fx.failManager("chat2", errors.New("broken"))

		// when
		fx.store.AddObjects(t, hubSpaceId, []objectstore.TestObject{givenHubChat("chat2", "Broken"), givenHubChat("chat3", "Fine")})

		// then: the stream continues without it
		got := waitFrames(t, sub, 1)
		got = append(got, settle(t, fx, sub, "chat1")...)
		var added []string
		for _, frame := range got {
			if frame.Type == "chat_added" {
				added = append(added, frame.ChatId)
			}
		}
		sort.Strings(added)
		assert.Equal(t, []string{"chat3"}, added)
		_, err := fx.Open(context.Background(), apicore.SpaceChatOpen{SpaceId: hubSpaceId, Render: renderTestFrame})
		var attachErr *apicore.SpaceChatAttachError
		require.ErrorAs(t, err, &attachErr)
		assert.Equal(t, "chat2", attachErr.ChatId)
		assert.NotNil(t, fx.hub(), "the refused open does not tear down the running hub")
	})
}

func frameTypes(frames []testFrame) []string {
	types := make([]string, 0, len(frames))
	for _, frame := range frames {
		types = append(types, frame.Type)
	}
	return types
}
