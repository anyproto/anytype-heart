package chatrepository

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	anystore "github.com/anyproto/any-store/v2"
	"github.com/anyproto/any-store/v2/anyenc"
	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver/mock_idresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

const (
	migrationSpaceId = "space1"
	migrationChatId  = "chat1"
	migrationMe      = "me"
)

// serviceFixture is the repository service as production builds it, over a crdt.db in dir.
type serviceFixture struct {
	*service
	app      *app.App
	provider anystoreprovider.Provider
}

func newServiceFixture(t *testing.T, dir string) *serviceFixture {
	t.Helper()
	ctx := context.Background()
	idResolver := mock_idresolver.NewMockResolver(t)
	idResolver.EXPECT().ResolveSpaceID(mock.Anything).Return(migrationSpaceId, nil).Maybe()
	provider, err := anystoreprovider.NewInPath(dir)
	require.NoError(t, err)
	a := &app.App{}
	a.Register(testutil.PrepareMock(ctx, a, idResolver))
	a.Register(provider)
	svc := New()
	a.Register(svc)
	require.NoError(t, a.Start(ctx))
	return &serviceFixture{service: svc.(*service), app: a, provider: provider}
}

func (fx *serviceFixture) close(t *testing.T) {
	t.Helper()
	require.NoError(t, fx.app.Close(context.Background()))
}

func (fx *serviceFixture) crdtDb(t *testing.T) anystore.DB {
	t.Helper()
	db, err := fx.provider.GetCrdtDb(migrationSpaceId).Wait()
	require.NoError(t, err)
	return db
}

func (fx *serviceFixture) repository(t *testing.T) *repository {
	t.Helper()
	repo, err := fx.Repository(migrationSpaceId, migrationChatId)
	require.NoError(t, err)
	return repo.(*repository)
}

// legacyMessage is a message as the layout before storage keys stored it: keyed by its id,
// ordered by _o.id.
type legacyMessage struct {
	msg     *chatmodel.Message
	orderId string // empty for a legacy stub
}

func legacyMessages() []legacyMessage {
	content := func(text string) *model.ChatMessageMessageContent {
		return &model.ChatMessageMessageContent{Text: text, Marks: []*model.BlockContentTextMark{}}
	}
	message := func(id, orderId, creator, text string, read bool) *chatmodel.Message {
		return &chatmodel.Message{ChatMessage: &model.ChatMessage{
			Id: id, OrderId: orderId, Creator: creator, Message: content(text),
			Read: read, MentionRead: true, Synced: true, StateId: "state-" + orderId,
			Attachments: []*model.ChatMessageAttachment{}, Reactions: &model.ChatMessageReactions{},
		}}
	}
	tail := strings.Repeat("x", 12)

	// an own message the user marked unread: a replay would make it read again
	unreadOverride := message(cid(1), "!!$1", migrationMe, "mine, marked unread", false)
	// changes made concurrently on one parent share an order id
	concurrentA := message(cid(2), "!!$2", "bob", "concurrent a", true)
	concurrentB := message(cid(3), "!!$2", "bob", "concurrent b", true)
	pinned := message(cid(4), "!!$3", "bob", "pinned", true)
	pinned.Pinned = true
	tombstone := message(cid(5), "!!$4", "bob", "deleted", true)
	tombstone.MakeTombstone()
	// as the store reads a tombstone back
	tombstone.Message = content("")
	tombstone.Attachments = []*model.ChatMessageAttachment{}
	reacted := message(cid(6), "!!$5", migrationMe, "reacted", true)
	reacted.Reactions = &model.ChatMessageReactions{Reactions: map[string]*model.ChatMessageReactionsIdentityList{"👍": {Ids: []string{"bob"}}}}
	reacted.UnreadReactionIds = map[string]map[string]chatmodel.ReactionChangeEntry{"👍": {"bob": {ChangeId: "reaction-change", OrderId: "!!$8"}}}
	reacted.UnreadReaction = true
	// a reply to the own message mentions me without a mention mark: a replay loses that
	reply := message(cid(7), "!!$6", "bob", "reply", false)
	reply.ReplyToMessageId = cid(1)
	reply.HasMention = true
	reply.MentionRead = false
	// two messages at one order whose ids end alike: one takes the collision key
	collisionFirst := message("first-change-"+tail, "!!$7", "bob", "first", true)
	collisionOther := message("other-change-"+tail, "!!$7", "bob", "other", true)

	stub := func(id string) *chatmodel.Message {
		return &chatmodel.Message{ChatMessage: &model.ChatMessage{
			Id: id, Message: content(""), Attachments: []*model.ChatMessageAttachment{},
			Reactions: &model.ChatMessageReactions{Reactions: map[string]*model.ChatMessageReactionsIdentityList{"🎉": {Ids: []string{"bob"}}}},
		}}
	}
	return []legacyMessage{
		{msg: unreadOverride, orderId: "!!$1"},
		{msg: concurrentA, orderId: "!!$2"},
		{msg: concurrentB, orderId: "!!$2"},
		{msg: pinned, orderId: "!!$3"},
		{msg: tombstone, orderId: "!!$4"},
		{msg: reacted, orderId: "!!$5"},
		{msg: reply, orderId: "!!$6"},
		{msg: collisionFirst, orderId: "!!$7"},
		{msg: collisionOther, orderId: "!!$7"},
		{msg: stub(cid(98))},
		{msg: stub(cid(99))},
	}
}

// seedLegacyChat writes the chat as the layout before storage keys left it: the messages keyed by
// id with its indexes, the message history and the replay watermark.
func seedLegacyChat(t *testing.T, db anystore.DB, messages []legacyMessage) {
	t.Helper()
	ctx := context.Background()
	coll, err := db.CreateCollection(ctx, messagesCollectionName(migrationChatId))
	require.NoError(t, err)
	require.NoError(t, coll.EnsureIndex(ctx,
		anystore.IndexInfo{Fields: []string{"_o.id"}},
		anystore.IndexInfo{Fields: []string{chatmodel.PinnedKey}, Sparse: true},
		anystore.IndexInfo{Fields: []string{chatmodel.ReactionUnreadOrderIdKey}, Sparse: true},
		anystore.IndexInfo{Fields: []string{chatmodel.CreatorKey, "_o.id"}},
		anystore.IndexInfo{Fields: []string{chatmodel.DeletedKey}, Sparse: true},
	))
	history, err := db.CreateCollection(ctx, messageHistoryCollectionName(migrationChatId))
	require.NoError(t, err)
	arena := &anyenc.Arena{}
	for _, m := range messages {
		arena.Reset()
		doc := arena.NewObject()
		m.msg.MarshalAnyenc(doc, arena)
		orders := arena.NewObject()
		if m.orderId != "" {
			orders.Set("id", arena.NewString(m.orderId))
		} else {
			orders.Set(chatmodel.ReactionsKey, arena.NewString("!!$3"))
		}
		doc.Set(chatmodel.OrderKey, orders)
		require.NoError(t, coll.Insert(ctx, doc))

		arena.Reset()
		historyDoc := arena.NewObject()
		historyDoc.Set("id", arena.NewString(m.msg.Id))
		require.NoError(t, history.Insert(ctx, historyDoc))
	}
	arena.Reset()
	marker := arena.NewObject()
	marker.Set("id", arena.NewString(messageHistorySchemaVersionId))
	require.NoError(t, history.Insert(ctx, marker))

	meta, err := db.Collection(ctx, storestate.CollMeta)
	require.NoError(t, err)
	arena.Reset()
	watermark := arena.NewObject()
	watermark.Set("id", arena.NewString(migrationChatId))
	watermark.Set("q", arena.NewNumberInt(42))
	watermark.Set("r", arena.NewTrue())
	require.NoError(t, meta.Insert(ctx, watermark))
}

func TestRekeyLegacyMessagesCollection(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	seeded := legacyMessages()
	var (
		wantMessages []*chatmodel.Message // every message but the stubs, as stored before
		wantIds      []string
		wantOrderIds []string
		stubIds      []string
	)
	for _, m := range seeded {
		if m.orderId == "" {
			stubIds = append(stubIds, m.msg.Id)
			continue
		}
		wantMessages = append(wantMessages, m.msg)
		wantIds = append(wantIds, m.msg.Id)
		wantOrderIds = append(wantOrderIds, m.orderId)
	}
	sort.Strings(wantOrderIds)
	tail := strings.Repeat("x", 12)
	wantKeys := map[string]string{
		cid(2):                 chatmodel.StorageKey("!!$2", cid(2)),
		"first-change-" + tail: chatmodel.StorageKey("!!$7", "first-change-"+tail),
		"other-change-" + tail: chatmodel.CollisionStorageKey("!!$7", "other-change-"+tail),
	}
	wantChatState := &model.ChatState{
		Messages:              &model.ChatStateUnreadState{OldestOrderId: "!!$1", Counter: 2},
		Mentions:              &model.ChatStateUnreadState{OldestOrderId: "!!$6", Counter: 1},
		LastStateId:           "state-!!$7",
		UnreadReactionOrderId: "!!$5",
	}

	// given: a chat stored in the layout before storage keys
	fx := newServiceFixture(t, dir)
	seedLegacyChat(t, fx.crdtDb(t), seeded)

	assertMigrated := func(t *testing.T, fx *serviceFixture) {
		t.Helper()
		repo := fx.repository(t)
		assert.Equal(t, chatmodel.StorageKeyField, repo.collection.PrimaryKey())

		// every message is found by id, as it was stored
		got, err := repo.GetMessagesByIds(ctx, wantIds)
		require.NoError(t, err)
		assert.Equal(t, wantMessages, got)
		for id, wantKey := range wantKeys {
			key, err := FindMessageKey(ctx, repo.collection, id)
			require.NoError(t, err)
			assert.Equal(t, wantKey, key, id)
		}
		// the stubs are gone
		stubs, err := repo.GetMessagesByIds(ctx, stubIds)
		require.NoError(t, err)
		assert.Empty(t, stubs)

		// and read in order
		page, err := repo.GetMessages(ctx, GetMessagesRequest{Limit: 100})
		require.NoError(t, err)
		gotOrderIds := make([]string, 0, len(page))
		for _, msg := range page {
			gotOrderIds = append(gotOrderIds, msg.OrderId)
		}
		assert.Equal(t, wantOrderIds, gotOrderIds)
		older, err := repo.GetMessages(ctx, GetMessagesRequest{BeforeOrderId: "!!$3", Limit: 100})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{cid(1), cid(2), cid(3)}, messageIds(older))

		pinned, err := repo.GetPinnedMessages(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{cid(4)}, messageIds(pinned))
		live, err := repo.CountMessages(ctx)
		require.NoError(t, err)
		assert.Equal(t, len(wantMessages)-1, live, "the tombstone is not counted")

		// the local read state, mentions and unread reactions are as they were
		chatState, err := repo.LoadChatState(ctx)
		require.NoError(t, err)
		assert.Equal(t, wantChatState, chatState)

		// nothing is replayed: the watermark and the history stay
		meta, err := fx.crdtDb(t).Collection(ctx, storestate.CollMeta)
		require.NoError(t, err)
		watermark, err := meta.FindId(ctx, migrationChatId)
		require.NoError(t, err)
		assert.Equal(t, 42, watermark.Value().GetInt("q"))
		lifetime, err := repo.CountMessagesLifetime(ctx)
		require.NoError(t, err)
		assert.Equal(t, len(seeded), lifetime)

		_, err = fx.crdtDb(t).OpenCollection(ctx, messagesCollectionName(migrationChatId)+rekeyMessagesSuffix)
		assert.ErrorIs(t, err, anystore.ErrCollectionNotFound)
	}

	// when
	t.Run("the first open rekeys the messages", func(t *testing.T) {
		assertMigrated(t, fx)
	})
	fx.close(t)

	t.Run("a reopen finds them as they are", func(t *testing.T) {
		// when
		reopened := newServiceFixture(t, dir)
		defer reopened.close(t)

		// then
		assertMigrated(t, reopened)
	})
}

// The rekey runs inside the open's write transaction: a crash before the commit is a rollback,
// which leaves the old collection whole, and the next open starts over.
func TestRekeyIsAllOrNothing(t *testing.T) {
	// given
	ctx := context.Background()
	db, err := anystore.Open(ctx, filepath.Join(t.TempDir(), "crdt.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	seeded := legacyMessages()
	seedLegacyChat(t, db, seeded)

	// when: a rekey dies before its commit
	tx, err := db.WriteTx(ctx)
	require.NoError(t, err)
	old, err := db.OpenCollection(tx.Context(), messagesCollectionName(migrationChatId))
	require.NoError(t, err)
	_, err = rekeyMessages(tx.Context(), db, migrationChatId, old)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())

	// then: the old collection is whole
	untouched, err := db.OpenCollection(ctx, messagesCollectionName(migrationChatId))
	require.NoError(t, err)
	assert.Equal(t, "id", untouched.PrimaryKey())
	count, err := untouched.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, len(seeded), count)
	_, err = db.OpenCollection(ctx, messagesCollectionName(migrationChatId)+rekeyMessagesSuffix)
	assert.ErrorIs(t, err, anystore.ErrCollectionNotFound)

	t.Run("and the next open rekeys it", func(t *testing.T) {
		// when
		messages, _, err := openMessageCollections(ctx, db, migrationChatId)

		// then
		require.NoError(t, err)
		assert.Equal(t, chatmodel.StorageKeyField, messages.PrimaryKey())
		count, err := messages.Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, len(seeded)-2, count, "all but the two stubs")
	})
}

func TestRekeyDropsALeftoverRekeyCollection(t *testing.T) {
	// given: a collection under the rekey's working name
	ctx := context.Background()
	db, err := anystore.Open(ctx, filepath.Join(t.TempDir(), "crdt.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	seeded := legacyMessages()
	seedLegacyChat(t, db, seeded)
	leftover, err := db.CreateCollection(ctx, messagesCollectionName(migrationChatId)+rekeyMessagesSuffix, MessagesCollectionOptions())
	require.NoError(t, err)
	arena := &anyenc.Arena{}
	junk := arena.NewObject()
	junk.Set("id", arena.NewString("junk"))
	junk.Set(chatmodel.StorageKeyField, arena.NewString(chatmodel.StorageKey("!!$1", "junk")))
	require.NoError(t, leftover.Insert(ctx, junk))

	// when
	messages, _, err := openMessageCollections(ctx, db, migrationChatId)

	// then
	require.NoError(t, err)
	count, err := messages.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, len(seeded)-2, count)
	_, err = FindMessageKey(ctx, messages, "junk")
	assert.ErrorIs(t, err, anystore.ErrDocNotFound)
}

// The copy walks the legacy collection by its _o.id index, so the new collection gets its
// documents in key order: appends.
func TestRekeyWalksTheLegacyOrderIndex(t *testing.T) {
	// given
	ctx := context.Background()
	db, err := anystore.Open(ctx, filepath.Join(t.TempDir(), "crdt.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	seedLegacyChat(t, db, legacyMessages())
	old, err := db.OpenCollection(ctx, messagesCollectionName(migrationChatId))
	require.NoError(t, err)

	// when
	ex, err := old.Find(nil).Sort(legacyOrderIdSort).Explain(ctx)

	// then
	require.NoError(t, err)
	t.Logf("plan: %s", ex.Sql)
	assert.Equal(t, "_o.id", usedIndex(ex))
	assert.NotContains(t, ex.Sql, "Sort")
}
