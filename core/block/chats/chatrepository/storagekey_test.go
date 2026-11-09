package chatrepository

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	anystore "github.com/anyproto/any-store/v2"
	"github.com/anyproto/any-store/v2/anyenc"
	"github.com/anyproto/any-store/v2/query"
	"github.com/anyproto/lexid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// cid returns a message id shaped like a change CID (59 characters), unique for i and differing
// from the other ones only near the end, so the tests see the tail that the storage key keeps.
func cid(i int) string {
	return fmt.Sprintf("bafyreih%039d%012d", 0, i)
}

// newPopulatedFixture is a chat with enough messages that the planner prices a collection walk
// as it would in a real chat (on an empty collection every plan costs the same and a scan wins).
func newPopulatedFixture(t *testing.T) *fixture {
	t.Helper()
	fx := newFixture(t)
	ctx := context.Background()
	orders := lexid.Must(lexid.CharsAllNoEscape, 4, 100)
	var orderId string
	for i := range 1000 {
		orderId = orders.Next(orderId)
		msg := &chatmodel.Message{
			ChatMessage: &model.ChatMessage{
				Id:      cid(i),
				OrderId: orderId,
				Creator: fmt.Sprintf("creator%d", i%5),
				Message: &model.ChatMessageMessageContent{Text: "test"},
				Read:    i%2 == 0,
				Pinned:  i%100 == 0,
			},
		}
		if i%40 == 0 {
			msg.UnreadReactionIds = map[string]map[string]chatmodel.ReactionChangeEntry{
				"👍": {"user1": {ChangeId: fmt.Sprintf("ch%d", i), OrderId: orderId}},
			}
		}
		if i%25 == 0 {
			msg.MakeTombstone()
		}
		require.NoError(t, fx.repo.AddTestMessage(ctx, msg))
	}
	return fx
}

func explain(t *testing.T, q anystore.Query) anystore.Explain {
	t.Helper()
	ex, err := q.Explain(context.Background())
	require.NoError(t, err)
	return ex
}

func usedIndex(ex anystore.Explain) string {
	for _, idx := range ex.Indexes {
		if idx.Used {
			return idx.Name
		}
	}
	return ""
}

// Every ordered walk and every order-id range reads the collection itself, in its natural order
// (the storage key is order-prefixed): no secondary index, no sort stage. Order-id bounds become
// bounds of the walk (idBounds).
func TestOrderedQueriesWalkTheNaturalOrder(t *testing.T) {
	fx := newPopulatedFixture(t)
	const (
		x = "!!#5"
		y = "!!$5"
	)
	for _, tc := range []struct {
		name    string
		query   anystore.Query
		bounded bool
		reverse bool
	}{
		{name: "GetLastMessages", query: fx.repo.lastMessagesQuery(10), reverse: true},
		{name: "GetLastLiveMessages", query: fx.repo.lastLiveMessagesQuery(10), reverse: true},
		{name: "GetPrevOrderId", query: fx.repo.prevOrderIdQuery(x), bounded: true, reverse: true},
		{name: "GetOldestOrderId", query: fx.repo.oldestUnreadQuery(chatmodel.CounterTypeMessage)},
		{name: "GetMessages, no bounds", query: fx.repo.messagesPageQuery(GetMessagesRequest{Limit: 10}), reverse: true},
		{name: "GetMessages, after", query: fx.repo.messagesPageQuery(GetMessagesRequest{AfterOrderId: x, Limit: 10}), bounded: true},
		{name: "GetMessages, after, with boundary", query: fx.repo.messagesPageQuery(GetMessagesRequest{AfterOrderId: x, IncludeBoundary: true, Limit: 10}), bounded: true},
		{name: "GetMessages, before", query: fx.repo.messagesPageQuery(GetMessagesRequest{BeforeOrderId: x, Limit: 10}), bounded: true, reverse: true},
		{name: "GetMessages, range with boundary", query: fx.repo.messagesPageQuery(GetMessagesRequest{AfterOrderId: x, BeforeOrderId: y, IncludeBoundary: true, Limit: 10}), bounded: true, reverse: true},
		{name: "GetUnreadMessageIdsInRange", query: fx.repo.unreadInRangeQuery(x, y, "~", chatmodel.CounterTypeMessage), bounded: true},
		{name: "GetReadMessagesAfter", query: fx.repo.readAfterQuery(x, chatmodel.CounterTypeMessage), bounded: true},
		{name: "SetSyncedByMaxOrderId", query: fx.repo.unsyncedUpToQuery(x), bounded: true},
		{name: "GetAllMessageAttachments", query: fx.repo.attachmentsQuery(x), bounded: true},
		{name: "IterateMessagesForIndexing, all", query: fx.repo.messagesForIndexingQuery("")},
		// the content-edit order is not indexed: the Or gives no bounds, but the walk is still ordered
		{name: "IterateMessagesForIndexing, after", query: fx.repo.messagesForIndexingQuery(x)},
		{name: "GetAllRawMessages", query: fx.repo.allMessagesQuery()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// when
			ex := explain(t, tc.query)
			t.Logf("plan: %s", ex.Sql)

			// then
			assert.True(t, strings.HasPrefix(ex.Sql, "FullScan"), "a walk of the collection")
			assert.NotContains(t, ex.Sql, "Sort", "the natural order is the sort order")
			assert.NotContains(t, ex.Sql, "TopK", "the natural order is the sort order")
			assert.Empty(t, usedIndex(ex), "no secondary index")
			assert.Equal(t, tc.bounded, strings.Contains(ex.Sql, "idBounds"), "order-id bounds are bounds of the walk")
			assert.Equal(t, tc.reverse, strings.Contains(ex.Sql, "(reverse)"))
		})
	}
}

func TestQueriesUseTheirIndexes(t *testing.T) {
	fx := newPopulatedFixture(t)
	arena := &anyenc.Arena{}
	for _, tc := range []struct {
		name     string
		query    anystore.Query
		index    string
		wantSort bool
	}{
		// the trailing storage key gives the walk the sorted order: no sort
		{name: "GetLastMessagesByCreators", query: fx.repo.lastLiveMessagesByCreatorQuery("creator1", 10), index: chatmodel.CreatorKey + "," + chatmodel.StorageKeyField},
		{name: "message by id", query: fx.repo.collection.Find(query.Key{Path: []string{"id"}, Filter: query.NewComp(query.CompOpEq, cid(7))}).Limit(1), index: "id"},
		{name: "SetReadFlag, messages by id", query: fx.repo.collection.Find(query.And{
			filterReadTrue,
			query.Key{Path: []string{"id"}, Filter: query.NewInValue(arena.NewString(cid(7)), arena.NewString(cid(9)))},
		}), index: "id"},
		// every index key ends with the primary key, so the index is read backwards in the sort order: no sort
		{name: "GetPinnedMessages", query: fx.repo.pinnedMessagesQuery(), index: chatmodel.PinnedKey},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// when
			ex := explain(t, tc.query)
			t.Logf("plan: %s", ex.Sql)

			// then
			assert.Equal(t, tc.index, usedIndex(ex))
			// a bounded sort prints as TopK
			assert.Equal(t, tc.wantSort, strings.Contains(ex.Sql, "Sort") || strings.Contains(ex.Sql, "TopK"))
		})
	}
}

// storedMessages has order ids where one is a prefix of another and duplicate order ids (changes
// made concurrently on one parent).
var storedMessages = []struct {
	orderId string
	id      string
}{
	{"!!$", cid(1)},
	{"!!$5", cid(2)},
	{"!!$5", cid(3)},
	{"!!$5!", cid(4)},
	{"!!$5!!", cid(5)},
	{"!!$6", cid(6)},
	{"!!$6", cid(7)},
	{"!!$7", cid(8)},
}

var orderIdBounds = []string{"!", "!!$", "!!$4", "!!$5", "!!$5!", "!!$5!!", "!!$5!!!", "!!$6", "!!$7", "!!$8"}

func newStoredMessagesFixture(t *testing.T) *fixture {
	t.Helper()
	fx := newFixture(t)
	for _, m := range storedMessages {
		require.NoError(t, fx.repo.AddTestMessage(context.Background(), &chatmodel.Message{
			ChatMessage: &model.ChatMessage{
				Id:          m.id,
				OrderId:     m.orderId,
				Message:     &model.ChatMessageMessageContent{Text: "test"},
				Attachments: []*model.ChatMessageAttachment{{Target: "file", Type: model.ChatMessageAttachment_FILE}},
			},
		}))
	}
	return fx
}

// storedIdsWhere returns the ids of the stored messages whose order id passes ok.
func storedIdsWhere(ok func(orderId string) bool) []string {
	var ids []string
	for _, m := range storedMessages {
		if ok(m.orderId) {
			ids = append(ids, m.id)
		}
	}
	return ids
}

// The order-id comparisons of the queries, expressed on the storage key, against the same
// comparisons on the order ids: bounds equal to a stored order id, a prefix of one, one that a
// stored order id is a prefix of, and absent ones.
func TestOrderIdBoundsOnStoredMessages(t *testing.T) {
	ctx := context.Background()
	fx := newStoredMessagesFixture(t)
	msgIds := func(t *testing.T, req GetMessagesRequest) []string {
		t.Helper()
		req.Limit = 100
		msgs, err := fx.repo.GetMessages(ctx, req)
		require.NoError(t, err)
		return messageIds(msgs)
	}

	for _, x := range orderIdBounds {
		t.Run(x, func(t *testing.T) {
			t.Run("GetMessages after, with boundary: >=", func(t *testing.T) {
				want := storedIdsWhere(func(o string) bool { return o >= x })
				assert.ElementsMatch(t, want, msgIds(t, GetMessagesRequest{AfterOrderId: x, IncludeBoundary: true}))
			})
			t.Run("GetMessages after: >", func(t *testing.T) {
				want := storedIdsWhere(func(o string) bool { return o > x })
				assert.ElementsMatch(t, want, msgIds(t, GetMessagesRequest{AfterOrderId: x}))
			})
			t.Run("GetMessages before: <", func(t *testing.T) {
				want := storedIdsWhere(func(o string) bool { return o < x })
				assert.ElementsMatch(t, want, msgIds(t, GetMessagesRequest{BeforeOrderId: x}))
			})
			t.Run("GetMessages before, with boundary: <=", func(t *testing.T) {
				want := storedIdsWhere(func(o string) bool { return o <= x })
				assert.ElementsMatch(t, want, msgIds(t, GetMessagesRequest{BeforeOrderId: x, IncludeBoundary: true}))
			})
			t.Run("GetPrevOrderId: newest <", func(t *testing.T) {
				var want string
				for _, m := range storedMessages {
					if m.orderId < x && m.orderId > want {
						want = m.orderId
					}
				}
				got, err := fx.repo.GetPrevOrderId(ctx, x)
				require.NoError(t, err)
				assert.Equal(t, want, got)
			})
			t.Run("GetAllMessageAttachments: >", func(t *testing.T) {
				want := storedIdsWhere(func(o string) bool { return o > x })
				got, err := fx.repo.GetAllMessageAttachments(ctx, x)
				require.NoError(t, err)
				gotIds := make([]string, 0, len(got))
				for _, att := range got {
					gotIds = append(gotIds, att.MessageId)
				}
				assert.ElementsMatch(t, want, gotIds)
			})
			t.Run("GetUnreadMessageIdsInRange: [first stored, x]", func(t *testing.T) {
				want := storedIdsWhere(func(o string) bool { return o >= "!!$5" && o <= x })
				got, err := fx.repo.GetUnreadMessageIdsInRange(ctx, "!!$5", x, "~", chatmodel.CounterTypeMessage)
				require.NoError(t, err)
				assert.ElementsMatch(t, want, got)
			})
		})
	}

	t.Run("GetReadMessagesAfter: >=", func(t *testing.T) {
		_, err := fx.repo.SetReadFlag(ctx, "chat", storedIdsWhere(func(string) bool { return true }), chatmodel.CounterTypeMessage, true)
		require.NoError(t, err)
		for _, x := range orderIdBounds {
			want := storedIdsWhere(func(o string) bool { return o >= x })
			got, err := fx.repo.GetReadMessagesAfter(ctx, x, chatmodel.CounterTypeMessage)
			require.NoError(t, err)
			assert.ElementsMatch(t, want, got, x)
		}
	})

	t.Run("SetSyncedByMaxOrderId: <=", func(t *testing.T) {
		for _, x := range orderIdBounds {
			fx := newStoredMessagesFixture(t)
			want := storedIdsWhere(func(o string) bool { return o <= x })
			got, err := fx.repo.SetSyncedByMaxOrderId(ctx, x)
			require.NoError(t, err)
			assert.ElementsMatch(t, want, got, x)
		}
	})

	t.Run("a page cut inside duplicate order ids follows the storage key", func(t *testing.T) {
		// when: the newest two of the messages before "!!$7" are the two with order id "!!$6"
		msgs, err := fx.repo.GetMessages(ctx, GetMessagesRequest{BeforeOrderId: "!!$7", Limit: 2})
		require.NoError(t, err)
		// and the next page continues below them
		older, err := fx.repo.GetMessages(ctx, GetMessagesRequest{BeforeOrderId: "!!$6", Limit: 3})
		require.NoError(t, err)

		// then
		assert.ElementsMatch(t, []string{cid(6), cid(7)}, messageIds(msgs))
		assert.ElementsMatch(t, []string{cid(3), cid(4), cid(5)}, messageIds(older))
	})
}

func TestMessagesByIdAfterRekeying(t *testing.T) {
	ctx := context.Background()

	t.Run("lookup by message id", func(t *testing.T) {
		// given
		fx := newStoredMessagesFixture(t)
		want := chatmodel.StorageKey("!!$5", cid(3))

		// when
		got, err := FindMessageKey(ctx, fx.repo.collection, cid(3))

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("lookup of a missing message", func(t *testing.T) {
		fx := newStoredMessagesFixture(t)
		_, err := FindMessageKey(ctx, fx.repo.collection, "missing")
		assert.ErrorIs(t, err, anystore.ErrDocNotFound)
	})

	t.Run("GetMessagesByIds keeps the asked order and skips missing ones", func(t *testing.T) {
		// given
		fx := newStoredMessagesFixture(t)
		want := []string{cid(7), cid(2), cid(3)}

		// when
		got, err := fx.repo.GetMessagesByIds(ctx, []string{cid(7), "missing", cid(2), cid(3)})

		// then
		require.NoError(t, err)
		assert.Equal(t, want, messageIds(got))
		assert.Equal(t, "!!$6", got[0].OrderId)
	})

	t.Run("GetMessagesByIds returns an id asked twice twice, as messages of their own", func(t *testing.T) {
		// given
		fx := newStoredMessagesFixture(t)

		// when
		got, err := fx.repo.GetMessagesByIds(ctx, []string{cid(2), cid(2)})

		// then
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, got[0], got[1])
		assert.NotSame(t, got[0], got[1])
	})

	t.Run("GetMessagesByIds over more than one query", func(t *testing.T) {
		// given
		fx := newFixture(t)
		var want []string
		for i := range 250 {
			id := cid(1000 - i) // asked in an order the index does not have
			require.NoError(t, fx.repo.AddTestMessage(ctx, &chatmodel.Message{ChatMessage: &model.ChatMessage{
				Id: id, OrderId: fmt.Sprintf("!!%04d", i), Message: &model.ChatMessageMessageContent{Text: "test"},
			}}))
			want = append(want, id)
		}

		// when
		got, err := fx.repo.GetMessagesByIds(ctx, want)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, messageIds(got))
	})

	t.Run("HasMyReaction", func(t *testing.T) {
		// given
		fx := newFixture(t)
		require.NoError(t, fx.repo.AddTestMessage(ctx, &chatmodel.Message{
			ChatMessage: &model.ChatMessage{
				Id:        cid(1),
				OrderId:   "!!$5",
				Message:   &model.ChatMessageMessageContent{Text: "test"},
				Reactions: &model.ChatMessageReactions{Reactions: map[string]*model.ChatMessageReactionsIdentityList{"👍": {Ids: []string{"me"}}}},
			},
		}))

		// when
		mine, err := fx.repo.HasMyReaction(ctx, "me", cid(1), "👍")
		require.NoError(t, err)
		other, err := fx.repo.HasMyReaction(ctx, "me", cid(1), "🎉")
		require.NoError(t, err)
		_, missingErr := fx.repo.HasMyReaction(ctx, "me", "missing", "👍")

		// then
		assert.True(t, mine)
		assert.False(t, other)
		assert.ErrorIs(t, missingErr, anystore.ErrDocNotFound)
	})

	t.Run("the id stays unique under the storage key", func(t *testing.T) {
		// given: a second document of cid(2) under another key
		fx := newStoredMessagesFixture(t)
		arena := &anyenc.Arena{}
		doc := arena.NewObject()
		doc.Set("id", arena.NewString(cid(2)))
		doc.Set(chatmodel.StorageKeyField, arena.NewString(chatmodel.StorageKey("!!$9", cid(2))))

		// when
		err := fx.repo.collection.Insert(ctx, doc)

		// then
		assert.ErrorIs(t, err, anystore.ErrUniqueConstraint)
	})
}

// A legacy stub (made by an old reaction upsert) has no order id: its key has an empty order part.
func TestStubWithoutOrderId(t *testing.T) {
	// given
	ctx := context.Background()
	fx := newStoredMessagesFixture(t)
	arena := &anyenc.Arena{}
	stub := arena.NewObject()
	stub.Set("id", arena.NewString(cid(99)))
	reactions := arena.NewObject()
	reactions.Set("👍", arena.NewArray())
	stub.Set(chatmodel.ReactionsKey, reactions)
	orders := arena.NewObject()
	orders.Set(chatmodel.ReactionsKey, arena.NewString("!!$5"))
	stub.Set(chatmodel.OrderKey, orders)
	stub.Set(chatmodel.StorageKeyField, arena.NewString(chatmodel.StorageKey(stub.GetString(chatmodel.OrderKey, "id"), cid(99))))
	require.NoError(t, fx.repo.collection.Insert(ctx, stub))

	// when
	all, err := fx.repo.GetAllRawMessages(ctx)
	require.NoError(t, err)
	byId, err := fx.repo.GetMessagesByIds(ctx, []string{cid(99)})
	require.NoError(t, err)
	after, err := fx.repo.GetMessages(ctx, GetMessagesRequest{AfterOrderId: "!", Limit: 100})
	require.NoError(t, err)

	// then
	require.Len(t, all, len(storedMessages)+1)
	assert.Contains(t, string(all[0]), cid(99), "the stub sorts first")
	require.Len(t, byId, 1)
	assert.Equal(t, "", byId[0].OrderId)
	assert.NotContains(t, messageIds(after), cid(99), "the stub is below every order id")
}

// The on-disk effect of the key: the same synthetic history (creates in order-id order, ids random
// with respect to it) inserted into a collection keyed by the message id and one keyed by the
// storage key. Order-keyed inserts append to the last leaf, which any-store fills before it splits;
// id-keyed inserts land on random leaves, which split half full.
func TestStorageKeyInsertPatternOnDisk(t *testing.T) {
	// given
	ctx := context.Background()
	db, err := anystore.Open(ctx, filepath.Join(t.TempDir(), "store.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	byId, err := db.CreateCollection(ctx, "byId")
	require.NoError(t, err)
	byKey, err := db.CreateCollection(ctx, "byKey", MessagesCollectionOptions())
	require.NoError(t, err)
	rnd := rand.New(rand.NewSource(1))
	orders := lexid.Must(lexid.CharsAllNoEscape, 4, 100)
	text := strings.Repeat("message text ", 20)
	arena := &anyenc.Arena{}
	var orderId string

	// when
	for range 3000 {
		orderId = orders.Next(orderId)
		id := fmt.Sprintf("bafyrei%052x", rnd.Uint64())
		for _, coll := range []anystore.Collection{byId, byKey} {
			arena.Reset()
			doc := arena.NewObject()
			doc.Set("id", arena.NewString(id))
			doc.Set(chatmodel.StorageKeyField, arena.NewString(chatmodel.StorageKey(orderId, id)))
			doc.Set("text", arena.NewString(text))
			require.NoError(t, coll.Insert(ctx, doc))
		}
	}

	// then
	idStats, err := byId.Stats(ctx)
	require.NoError(t, err)
	keyStats, err := byKey.Stats(ctx)
	require.NoError(t, err)
	t.Logf("documents btree: keyed by id %d bytes, by storage key %d bytes", idStats.DocsSizeBytes, keyStats.DocsSizeBytes)
	assert.Less(t, keyStats.DocsSizeBytes, idStats.DocsSizeBytes)
}

func TestQueryMessagesOrder(t *testing.T) {
	// given
	fx := newStoredMessagesFixture(t)
	want := make([]string, 0, len(storedMessages))
	for _, m := range storedMessages {
		want = append(want, m.orderId)
	}
	sort.Strings(want)

	// when
	msgs, err := fx.repo.GetLastMessages(context.Background(), 100)

	// then
	require.NoError(t, err)
	got := make([]string, 0, len(msgs))
	for _, m := range msgs {
		got = append(got, m.OrderId)
	}
	assert.Equal(t, want, got)
}
