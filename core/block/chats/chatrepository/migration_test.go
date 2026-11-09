package chatrepository

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	anystorev1 "github.com/anyproto/any-store"
	v1enc "github.com/anyproto/any-store/anyenc"
	anystore "github.com/anyproto/any-store/v2"
	"github.com/anyproto/any-store/v2/anyenc"
	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver/mock_idresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoremigrate"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

const (
	migrationSpaceId = "space1"
	migrationChatId  = "chat1"
	migrationMe      = "me"
)

// serviceFixture is the repository service as production builds it, over the object store in dir.
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

// v1Message is a message as a v1 build stored it: keyed by its id, ordered by _o.id.
type v1Message struct {
	msg     *chatmodel.Message
	orderId string // empty for a legacy stub
}

func v1Messages() []v1Message {
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

	// an own message the user marked unread
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
	// a reply to the own message mentions me without a mention mark
	reply := message(cid(7), "!!$6", "bob", "reply", false)
	reply.ReplyToMessageId = cid(1)
	reply.HasMention = true
	reply.MentionRead = false
	last := message(cid(8), "!!$7", "bob", "last", true)

	stub := func(id string) *chatmodel.Message {
		return &chatmodel.Message{ChatMessage: &model.ChatMessage{
			Id: id, Message: content(""), Attachments: []*model.ChatMessageAttachment{},
			Reactions: &model.ChatMessageReactions{Reactions: map[string]*model.ChatMessageReactionsIdentityList{"🎉": {Ids: []string{"bob"}}}},
		}}
	}
	return []v1Message{
		{msg: unreadOverride, orderId: "!!$1"},
		{msg: concurrentA, orderId: "!!$2"},
		{msg: concurrentB, orderId: "!!$2"},
		{msg: pinned, orderId: "!!$3"},
		{msg: tombstone, orderId: "!!$4"},
		{msg: reacted, orderId: "!!$5"},
		{msg: reply, orderId: "!!$6"},
		{msg: last, orderId: "!!$7"},
		{msg: stub(cid(98))},
		{msg: stub(cid(99))},
	}
}

// seedV1Chat writes the space's crdt.db as a v1 build left it: the chat's messages keyed by id with
// that build's indexes, the message history and the replay watermark.
func seedV1Chat(t *testing.T, root string, messages []v1Message) (crdtPath string) {
	t.Helper()
	ctx := context.Background()
	crdtPath = filepath.Join(root, "objectstore", migrationSpaceId, "crdt.db")
	require.NoError(t, os.MkdirAll(filepath.Dir(crdtPath), 0o700))
	db, err := anystorev1.Open(ctx, crdtPath, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()

	coll, err := db.CreateCollection(ctx, messagesCollectionName(migrationChatId))
	require.NoError(t, err)
	require.NoError(t, coll.EnsureIndex(ctx,
		anystorev1.IndexInfo{Fields: []string{"_o.id"}},
		anystorev1.IndexInfo{Fields: []string{chatmodel.PinnedKey}, Sparse: true},
		anystorev1.IndexInfo{Fields: []string{chatmodel.ReactionUnreadOrderIdKey}, Sparse: true},
		anystorev1.IndexInfo{Fields: []string{chatmodel.CreatorKey, "_o.id"}},
		anystorev1.IndexInfo{Fields: []string{chatmodel.DeletedKey}, Sparse: true},
	))
	history, err := db.CreateCollection(ctx, messageHistoryCollectionName(migrationChatId))
	require.NoError(t, err)
	meta, err := db.CreateCollection(ctx, storestate.CollMeta)
	require.NoError(t, err)

	// documents are built with the model's own marshaller (v2 anyenc) and stored as the same bytes
	// through v1: both versions encode these types alike
	var (
		arena  = &anyenc.Arena{}
		parser = &v1enc.Parser{}
	)
	insert := func(coll anystorev1.Collection, doc *anyenc.Value) {
		v1doc, err := parser.Parse(doc.MarshalTo(nil))
		require.NoError(t, err)
		require.NoError(t, coll.Insert(ctx, v1doc))
	}
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
		insert(coll, doc)

		arena.Reset()
		historyDoc := arena.NewObject()
		historyDoc.Set("id", arena.NewString(m.msg.Id))
		insert(history, historyDoc)
	}
	arena.Reset()
	marker := arena.NewObject()
	marker.Set("id", arena.NewString(messageHistorySchemaVersionId))
	insert(history, marker)
	arena.Reset()
	watermark := arena.NewObject()
	watermark.Set("id", arena.NewString(migrationChatId))
	watermark.Set("q", arena.NewNumberInt(42))
	watermark.Set("r", arena.NewTrue())
	insert(meta, watermark)
	return crdtPath
}

func TestV1ChatMigratesAtFirstOpen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	seeded := v1Messages()
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
	wantKeys := map[string]string{
		cid(2): chatmodel.StorageKey("!!$2", cid(2)),
		cid(3): chatmodel.StorageKey("!!$2", cid(3)),
		cid(8): chatmodel.StorageKey("!!$7", cid(8)),
	}
	wantChatState := &model.ChatState{
		Messages:              &model.ChatStateUnreadState{OldestOrderId: "!!$1", Counter: 2},
		Mentions:              &model.ChatStateUnreadState{OldestOrderId: "!!$6", Counter: 1},
		LastStateId:           "state-!!$7",
		UnreadReactionOrderId: "!!$5",
	}

	// given: the space's crdt.db as a v1 build left it
	crdtPath := seedV1Chat(t, root, seeded)
	fx := newServiceFixture(t, root)

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
	}

	// when
	t.Run("the first open migrates the space database", func(t *testing.T) {
		assertMigrated(t, fx)
		isV1, err := anystoremigrate.IsV1File(crdtPath + anystoremigrate.V1BackupSuffix)
		require.NoError(t, err)
		assert.True(t, isV1, "the v1 file is kept")
	})
	fx.close(t)

	t.Run("a reopen finds it as it is", func(t *testing.T) {
		// when
		reopened := newServiceFixture(t, root)
		defer reopened.close(t)

		// then
		assertMigrated(t, reopened)
	})
}

func TestMessagesCollectionKeyedOtherwiseIsRefused(t *testing.T) {
	// given: a v2 messages collection keyed by the message id
	ctx := context.Background()
	db, err := anystore.Open(ctx, filepath.Join(t.TempDir(), "crdt.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.CreateCollection(ctx, messagesCollectionName(migrationChatId))
	require.NoError(t, err)

	// when
	_, _, err = openMessageCollections(ctx, db, migrationChatId)

	// then
	assert.ErrorIs(t, err, anystore.ErrPrimaryKeyMismatch)
}
