package chatrepository

/*
AI generated

Name: Chat Message Storage
Scope: global

## Responsibility
- Provides per-chat Repository instances for message persistence
- Stores and queries chat messages with pagination support
- Tracks read/unread state for messages and mentions separately
- Tracks sync state for messages

## External State
- CRDT DB collections: one per chat object (`{chatObjectId}chats`)
*/

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"sync"

	anystore "github.com/anyproto/any-store/v2"
	"github.com/anyproto/any-store/v2/anyenc"
	"github.com/anyproto/any-store/v2/query"
	"github.com/anyproto/any-sync/app"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/anystorehelper"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "chatrepository"

const (
	// Messages are keyed by their storage key, which orders them by order id: sorting by it walks
	// the collection in its natural order, with no index and no sort.
	descOrder   = chatmodel.SortStorageKeyDesc
	ascOrder    = chatmodel.SortStorageKeyAsc
	descStateId = "-stateId"

	messageHistorySchemaVersionId = "$message-history-v1"
)

var log = logging.LoggerNotSugared(CName)

type Service interface {
	app.ComponentRunnable

	Repository(spaceId, chatObjectId string) (Repository, error)
}

type service struct {
	componentCtx       context.Context
	componentCtxCancel context.CancelFunc

	objectStore     objectstore.ObjectStore
	dbProvider      anystoreprovider.Provider
	spaceIdResolver idresolver.Resolver
	arenaPool       *anyenc.ArenaPool

	cache map[string]Repository
	lock  sync.RWMutex
}

func New() Service {
	return &service{
		arenaPool: &anyenc.ArenaPool{},
	}
}

func (s *service) Run(ctx context.Context) error {
	return nil
}

func (s *service) Close(ctx context.Context) error {
	if s.componentCtxCancel != nil {
		s.componentCtxCancel()
	}
	return nil
}

func (s *service) Init(a *app.App) (err error) {
	s.componentCtx, s.componentCtxCancel = context.WithCancel(context.Background())

	s.spaceIdResolver = app.MustComponent[idresolver.Resolver](a)
	s.dbProvider = app.MustComponent[anystoreprovider.Provider](a)

	s.cache = make(map[string]Repository)
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

// chatCollectionIndexes is package-level so tests can build a collection with exactly the indexes
// production has, and assert the queries below still reach them.
//
// The sparse ones are only usable by a filter that guarantees the field is present and non-null:
// since any-store v1.0.2 the planner refuses a sparse index for anything weaker, $exists included
// (it also matches an explicit null, which the index skips). See filterReactionUnread.
//
// There is no index on the order id: the primary key (the storage key) is order-prefixed, so every
// ordered walk and every order-id range is a walk of the collection itself.
var chatCollectionIndexes = []anystore.IndexInfo{
	// messages are named by id in changes and in the API: FindMessageDoc, GetMessagesByIds,
	// SetReadFlag. Unique, so that two documents never hold the same message.
	{Fields: []string{"id"}, Unique: true},
	{Fields: []string{chatmodel.PinnedKey}, Sparse: true},
	{Fields: []string{chatmodel.ReactionUnreadOrderIdKey}, Sparse: true},
	// serves GetLastMessagesByCreators: creator Eq + a walk in storage-key (order) order with early
	// exit (measured ~80µs vs a 25-45ms full-collection scan at 50k msgs). The trailing field is the
	// storage key, not _o.id, so the walk has the order the query sorts by and needs no sort.
	{Fields: []string{chatmodel.CreatorKey, chatmodel.StorageKeyField}},
	// only tombstones carry the field: counting them (CountMessages) reads just them
	{Fields: []string{chatmodel.DeletedKey}, Sparse: true},
}

// MessagesCollectionOptions are the options a chat messages collection is created with: it is
// keyed by the message storage key (chatmodel.StorageKey), not by the message id.
func MessagesCollectionOptions() anystore.CollectionOptions {
	return anystore.CollectionOptions{PrimaryKey: chatmodel.StorageKeyField}
}

func messagesCollectionName(chatObjectId string) string {
	return chatObjectId + "chats"
}

func messageHistoryCollectionName(chatObjectId string) string {
	return chatObjectId + "chatMessageHistory"
}

func (s *service) Repository(spaceId, chatObjectId string) (Repository, error) {
	s.lock.RLock()
	repo, ok := s.cache[chatObjectId]
	s.lock.RUnlock()

	if ok {
		return repo, nil
	}

	return s.getOrInitRepository(spaceId, chatObjectId)
}

func (s *service) getOrInitRepository(spaceId, chatObjectId string) (Repository, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if spaceId == "" {
		var err error
		spaceId, err = s.spaceIdResolver.ResolveSpaceID(chatObjectId)
		if err != nil {
			return nil, fmt.Errorf("resolve space id: %w", err)
		}
	}

	crdtDb, err := s.dbProvider.GetCrdtDb(spaceId).Wait()
	if err != nil {
		return nil, fmt.Errorf("get crdt db: %w", err)
	}

	collection, historyCollection, err := openMessageCollections(s.componentCtx, crdtDb, chatObjectId)
	if err != nil {
		return nil, fmt.Errorf("open message collections: %w", err)
	}

	repo := &repository{
		collection:        collection,
		historyCollection: historyCollection,
		arenaPool:         s.arenaPool,
	}
	if err = repo.ensureMessageHistory(s.componentCtx, crdtDb, chatObjectId); err != nil {
		return nil, fmt.Errorf("initialize message history: %w", err)
	}

	s.cache[chatObjectId] = repo
	return repo, nil
}

// openMessageCollections opens the chat's messages and message history collections, creating
// them if needed, and ensures the messages indexes.
//
// The primary key of a collection is fixed when it is created. A messages collection created
// before messages were keyed by their storage key is keyed by the message id; it is rebuilt: both
// collections are dropped, and the object's replay watermark is reset so that the next open of the
// chat replays its whole change tree into the new ones (see dropMessageCollections for what that
// keeps). Everything happens in one transaction, so a crash leaves either the old collections or
// the new ones, never a half: the rebuild cannot repeat once it committed.
func openMessageCollections(ctx context.Context, db anystore.DB, chatObjectId string) (messages, history anystore.Collection, err error) {
	tx, err := db.WriteTx(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("write tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	txCtx := tx.Context()

	messages, err = db.OpenCollection(txCtx, messagesCollectionName(chatObjectId))
	switch {
	case errors.Is(err, anystore.ErrCollectionNotFound):
		messages = nil
	case err != nil:
		return nil, nil, fmt.Errorf("open messages collection: %w", err)
	case messages.PrimaryKey() != chatmodel.StorageKeyField:
		if err = dropMessageCollections(txCtx, db, chatObjectId, messages); err != nil {
			return nil, nil, fmt.Errorf("rebuild messages collection keyed by %q: %w", messages.PrimaryKey(), err)
		}
		messages = nil
	}
	if messages == nil {
		messages, err = db.CreateCollection(txCtx, messagesCollectionName(chatObjectId), MessagesCollectionOptions())
		if err != nil {
			return nil, nil, fmt.Errorf("create messages collection: %w", err)
		}
	}
	history, err = db.Collection(txCtx, messageHistoryCollectionName(chatObjectId))
	if err != nil {
		return nil, nil, fmt.Errorf("open message history collection: %w", err)
	}
	if err = anystorehelper.AddIndexes(txCtx, messages, chatCollectionIndexes); err != nil {
		return nil, nil, fmt.Errorf("ensure indexes: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("commit: %w", err)
	}
	return messages, history, nil
}

// dropMessageCollections drops a chat's messages collection (and its history) whose layout can't
// be migrated in place, and makes the next open of the chat replay its whole change tree, which
// rebuilds both: the store is derived from the tree.
//
// The replay recreates what the tree holds: messages, edits, reactions, pins, tombstones, the
// message history (lifetime count) and the unread-reaction entries. It does not recreate the
// local-only fields; they come back as follows:
//   - read/mentionRead: recreated unread (own messages read), then marked read again by the
//     diff managers from the seen heads kept in the tech-space key-value store, as after a chat
//     reindex. Read state not yet stored as seen heads is lost.
//   - synced: own messages show unsynced until the next sync-status update of the chat.
//   - stateId: regenerated, so a client's older lastStateId excludes them from a read request
//     until it gets the new chat state.
//
// Until the chat is opened again its collection is empty, as for a chat that was never built.
func dropMessageCollections(ctx context.Context, db anystore.DB, chatObjectId string, messages anystore.Collection) error {
	log.Warn("rebuilding chat messages collection: keyed by another field, dropping it for a full replay",
		zap.String("chatId", chatObjectId), zap.String("primaryKey", messages.PrimaryKey()))
	if err := messages.Drop(ctx); err != nil {
		return fmt.Errorf("drop messages collection: %w", err)
	}
	history, err := db.OpenCollection(ctx, messageHistoryCollectionName(chatObjectId))
	switch {
	case errors.Is(err, anystore.ErrCollectionNotFound):
	case err != nil:
		return fmt.Errorf("open message history collection: %w", err)
	default:
		if err = history.Drop(ctx); err != nil {
			return fmt.Errorf("drop message history collection: %w", err)
		}
	}
	if err = storestate.ResetReplayState(ctx, db, chatObjectId); err != nil {
		return fmt.Errorf("reset replay state: %w", err)
	}
	return nil
}

// FindMessageDoc calls proc with the stored document of the message with this id; the value is
// valid only inside proc. Messages are keyed by their storage key, so the lookup goes through the
// unique id index. It returns an error wrapping anystore.ErrDocNotFound when the message is not
// stored, and proc's error as it is.
func FindMessageDoc(ctx context.Context, coll anystore.Collection, messageId string, proc func(doc *anyenc.Value) error) error {
	iter, err := coll.Find(query.Key{Path: []string{"id"}, Filter: query.NewComp(query.CompOpEq, messageId)}).Limit(1).Iter(ctx)
	if err != nil {
		return fmt.Errorf("find message by id: %w", err)
	}
	defer iter.Close()
	if !iter.Next() {
		if err = iter.Err(); err != nil {
			return fmt.Errorf("find message by id: %w", err)
		}
		return fmt.Errorf("message %s: %w", messageId, anystore.ErrDocNotFound)
	}
	doc, err := iter.Doc()
	if err != nil {
		return fmt.Errorf("read message: %w", err)
	}
	return proc(doc.Value())
}

// FindMessageKey returns the storage key of the stored message with this id, or an error wrapping
// anystore.ErrDocNotFound when it is not stored.
func FindMessageKey(ctx context.Context, coll anystore.Collection, messageId string) (string, error) {
	var key string
	err := FindMessageDoc(ctx, coll, messageId, func(doc *anyenc.Value) error {
		key = doc.GetString(chatmodel.StorageKeyField)
		return nil
	})
	return key, err
}

// MessageAttachmentInfo contains attachment metadata from a message
type MessageAttachmentInfo struct {
	MessageId string
	CreatedAt int64
	FileIds   []string // Target IDs of FILE and IMAGE attachments
}

type Repository interface {
	WriteTx(ctx context.Context) (anystore.WriteTx, error)
	ReadTx(ctx context.Context) (anystore.ReadTx, error)
	AddTestMessage(ctx context.Context, msg *chatmodel.Message) error
	GetLastStateId(ctx context.Context) (string, error)
	GetPrevOrderId(ctx context.Context, orderId string) (string, error)
	LoadChatState(ctx context.Context) (*model.ChatState, error)
	CountMessages(ctx context.Context) (int, error)
	CountMessagesLifetime(ctx context.Context) (int, error)
	RecordMessage(ctx context.Context, messageId string) (bool, error)
	GetOldestOrderId(ctx context.Context, counterType chatmodel.CounterType) (string, error)
	GetReadMessagesAfter(ctx context.Context, afterOrderId string, counterType chatmodel.CounterType) ([]string, error)
	GetUnreadMessageIdsInRange(ctx context.Context, afterOrderId, beforeOrderId string, lastStateId string, counterType chatmodel.CounterType) ([]string, error)
	GetAllUnreadMessages(ctx context.Context, counterType chatmodel.CounterType) ([]string, error)
	// IterateMessagesForIndexing streams messages created or edited at or after
	// afterOrderId to proc one at a time; an empty afterOrderId streams the whole
	// history. Nothing is materialized, so callers can index arbitrarily large
	// chats with bounded memory.
	IterateMessagesForIndexing(ctx context.Context, afterOrderId string, proc func(msg *chatmodel.Message) error) error
	SetReadFlag(ctx context.Context, chatObjectId string, msgIds []string, counterType chatmodel.CounterType, value bool) ([]string, error)
	GetMessages(ctx context.Context, req GetMessagesRequest) ([]*chatmodel.Message, error)
	HasMyReaction(ctx context.Context, myIdentity string, messageId string, emoji string) (bool, error)
	GetMessagesByIds(ctx context.Context, messageIds []string) ([]*chatmodel.Message, error)
	// GetLastMessages returns the newest messages, tombstones included: it seeds
	// subscription windows and stream backfills, which must hold a deleted
	// comment so its replies keep their thread.
	GetLastMessages(ctx context.Context, limit uint) ([]*chatmodel.Message, error)
	// GetLastLiveMessages is GetLastMessages without tombstones, for readers
	// that want messages only (search browse, the last-message date).
	GetLastLiveMessages(ctx context.Context, limit uint) ([]*chatmodel.Message, error)
	// GetLastMessagesByCreators returns the newest live messages authored by
	// any of the given identities (exact match), in ascending order like
	// GetLastMessages
	GetLastMessagesByCreators(ctx context.Context, creators []string, limit uint) ([]*chatmodel.Message, error)
	SetSyncedByMaxOrderId(ctx context.Context, maxOrderId string) ([]string, error)
	// GetAllMessageAttachments returns attachment info from all messages, optionally filtered by afterOrderId.
	GetAllMessageAttachments(ctx context.Context, afterOrderId string) ([]MessageAttachmentInfo, error)
	GetPinnedMessages(ctx context.Context) ([]*chatmodel.Message, error)
	GetAllUnreadReactionChangeIds(ctx context.Context) ([]string, error)
	ClearUnreadReactions(ctx context.Context, maxOrderId string) (modifiedMsgIds []string, err error)
	GetNewestUnreadReactionOrderId(ctx context.Context) (string, error)
	GetAllRawMessages(ctx context.Context) ([]json.RawMessage, error)
}

type repository struct {
	collection        anystore.Collection
	historyCollection anystore.Collection
	arenaPool         *anyenc.ArenaPool
}

func (s *repository) AddTestMessage(ctx context.Context, msg *chatmodel.Message) error {
	arena := s.arenaPool.Get()
	defer s.arenaPool.Put(arena)

	val := arena.NewObject()
	msg.MarshalAnyenc(val, arena)

	orderObj := arena.NewObject()
	orderObj.Set("id", arena.NewString(msg.OrderId))
	val.Set(chatmodel.OrderKey, orderObj)
	val.Set(chatmodel.StorageKeyField, arena.NewString(chatmodel.StorageKey(msg.OrderId, msg.Id)))

	if _, err := s.RecordMessage(ctx, msg.Id); err != nil {
		return err
	}
	if err := s.collection.Insert(ctx, val); err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	return nil
}

func (s *repository) WriteTx(ctx context.Context) (anystore.WriteTx, error) {
	return s.collection.WriteTx(ctx)
}

func (s *repository) ReadTx(ctx context.Context) (anystore.ReadTx, error) {
	return s.collection.ReadTx(ctx)
}

func (s *repository) GetLastStateId(ctx context.Context) (string, error) {
	lastAddedDate := s.collection.Find(nil).Sort(descStateId).Limit(1)
	iter, err := lastAddedDate.Iter(ctx)
	if err != nil {
		return "", fmt.Errorf("find last added date: %w", err)
	}
	defer iter.Close()

	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return "", fmt.Errorf("get doc: %w", err)
		}
		msg, err := chatmodel.UnmarshalMessage(doc.Value())
		if err != nil {
			return "", fmt.Errorf("unmarshal message: %w", err)
		}
		return msg.StateId, nil
	}
	return "", nil
}

func (s *repository) GetPrevOrderId(ctx context.Context, orderId string) (string, error) {
	iter, err := s.prevOrderIdQuery(orderId).Iter(ctx)
	if err != nil {
		return "", fmt.Errorf("init iterator: %w", err)
	}
	defer iter.Close()

	if iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return "", fmt.Errorf("read doc: %w", err)
		}
		prevOrderId := doc.Value().GetString(chatmodel.OrderKey, "id")
		return prevOrderId, nil
	}

	return "", nil
}

// prevOrderIdQuery walks back from orderId (excluded) to the newest older message.
func (s *repository) prevOrderIdQuery(orderId string) anystore.Query {
	return s.collection.Find(chatmodel.OrderIdBefore(orderId)).Sort(descOrder).Limit(1)
}

// initialChatState returns the initial chat state for the chat object from the DB
func (s *repository) LoadChatState(ctx context.Context) (*model.ChatState, error) {
	txn, err := s.ReadTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("start read tx: %w", err)
	}
	defer txn.Commit()

	messagesState, err := s.loadChatStateByType(txn.Context(), chatmodel.CounterTypeMessage)
	if err != nil {
		return nil, fmt.Errorf("get messages state: %w", err)
	}
	mentionsState, err := s.loadChatStateByType(txn.Context(), chatmodel.CounterTypeMention)
	if err != nil {
		return nil, fmt.Errorf("get mentions state: %w", err)
	}

	lastStateId, err := s.GetLastStateId(txn.Context())
	if err != nil {
		return nil, fmt.Errorf("get last added date: %w", err)
	}

	unreadReactionOrderId, err := s.GetNewestUnreadReactionOrderId(txn.Context())
	if err != nil {
		return nil, fmt.Errorf("get newest unread reaction order id: %w", err)
	}

	return &model.ChatState{
		Messages:              messagesState,
		Mentions:              mentionsState,
		LastStateId:           lastStateId,
		UnreadReactionOrderId: unreadReactionOrderId,
	}, nil
}

// CountMessages returns the number of live messages: tombstones (soft-deleted discussion
// comments) stay in the collection but are not counted.
func (s *repository) CountMessages(ctx context.Context) (int, error) {
	total, err := s.collection.Find(nil).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count all messages: %w", err)
	}
	deleted, err := s.deletedMessagesQuery().Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count deleted messages: %w", err)
	}
	return total - deleted, nil
}

// CountMessagesLifetime returns the number of distinct messages ever added to
// the chat. Deletion removes the live message document but deliberately keeps
// its history record, so API message_count does not move backwards.
func (s *repository) CountMessagesLifetime(ctx context.Context) (int, error) {
	return s.historyCollection.Find(query.Key{
		Path:   []string{"id"},
		Filter: query.NewComp(query.CompOpNe, messageHistorySchemaVersionId),
	}).Count(ctx)
}

// RecordMessage records one message identity in the append-only local history
// index. The index is rebuilt deterministically when chat changes are replayed;
// using the message id makes replays idempotent.
func (s *repository) RecordMessage(ctx context.Context, messageId string) (bool, error) {
	return s.recordMessageHistoryId(ctx, messageId)
}

func (s *repository) recordMessageHistoryId(ctx context.Context, messageId string) (bool, error) {
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()
	doc := arena.NewObject()
	doc.Set("id", arena.NewString(messageId))
	if err := s.historyCollection.Insert(ctx, doc); err != nil {
		if errors.Is(err, anystore.ErrDocExists) {
			return false, nil
		}
		return false, fmt.Errorf("record message history: %w", err)
	}
	return true, nil
}

// ensureMessageHistory seeds live messages, invalidates the incremental replay
// watermark exactly once, then writes its version marker. The next object open
// consequently replays the complete tree and recovers pre-migration messages
// that were already deleted. If initialization crashes before the marker, the
// whole idempotent migration runs again.
func (s *repository) ensureMessageHistory(ctx context.Context, db anystore.DB, chatObjectId string) error {
	if _, err := s.historyCollection.FindId(ctx, messageHistorySchemaVersionId); err == nil {
		return nil
	} else if !errors.Is(err, anystore.ErrDocNotFound) {
		return fmt.Errorf("read schema marker: %w", err)
	}
	if err := storestate.ResetReplayState(ctx, db, chatObjectId); err != nil {
		return fmt.Errorf("reset full replay marker: %w", err)
	}
	if err := s.seedMessageHistory(ctx); err != nil {
		return err
	}
	if _, err := s.recordMessageHistoryId(ctx, messageHistorySchemaVersionId); err != nil {
		return fmt.Errorf("write schema marker: %w", err)
	}
	return nil
}

func (s *repository) seedMessageHistory(ctx context.Context) error {
	iter, err := s.collection.Find(nil).Iter(ctx)
	if err != nil {
		return fmt.Errorf("iterate live messages: %w", err)
	}
	defer iter.Close()
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return fmt.Errorf("read live message: %w", err)
		}
		if _, err = s.RecordMessage(ctx, doc.Value().GetString("id")); err != nil {
			return err
		}
	}
	return iter.Err()
}

func (s *repository) loadChatStateByType(ctx context.Context, counterType chatmodel.CounterType) (*model.ChatStateUnreadState, error) {
	handler := newReadHandler(counterType)

	oldestOrderId, err := s.GetOldestOrderId(ctx, counterType)
	if err != nil {
		return nil, fmt.Errorf("get oldest order id: %w", err)
	}

	count, err := s.countUnreadMessages(ctx, handler)
	if err != nil {
		return nil, fmt.Errorf("update messages: %w", err)
	}

	return &model.ChatStateUnreadState{
		OldestOrderId: oldestOrderId,
		Counter:       int32(count),
	}, nil
}

func (s *repository) GetOldestOrderId(ctx context.Context, counterType chatmodel.CounterType) (string, error) {
	iter, err := s.oldestUnreadQuery(counterType).Iter(ctx)
	if err != nil {
		return "", fmt.Errorf("init iter: %w", err)
	}
	defer iter.Close()

	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return "", fmt.Errorf("get doc: %w", err)
		}
		orders := doc.Value().GetObject(chatmodel.OrderKey)
		if orders != nil {
			return orders.Get("id").GetString(), nil
		}
	}
	return "", nil
}

// oldestUnreadQuery walks from the oldest message to the first unread one.
func (s *repository) oldestUnreadQuery(counterType chatmodel.CounterType) anystore.Query {
	return s.collection.Find(newReadHandler(counterType).getReadFilter(false)).Sort(ascOrder).Limit(1)
}

func (s *repository) countUnreadMessages(ctx context.Context, handler readHandler) (int, error) {
	unreadQuery := s.collection.Find(handler.getReadFilter(false))

	return unreadQuery.Count(ctx)
}

func (s *repository) GetReadMessagesAfter(ctx context.Context, afterOrderId string, counterType chatmodel.CounterType) ([]string, error) {
	iter, err := s.readAfterQuery(afterOrderId, counterType).Iter(ctx)
	if err != nil {
		return nil, fmt.Errorf("init iterator: %w", err)
	}
	defer iter.Close()

	var msgIds []string
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("get doc: %w", err)
		}
		msgIds = append(msgIds, doc.Value().GetString("id"))
	}
	return msgIds, iter.Err()
}

// readAfterQuery walks the read messages with an order id >= afterOrderId.
func (s *repository) readAfterQuery(afterOrderId string, counterType chatmodel.CounterType) anystore.Query {
	handler := newReadHandler(counterType)
	filter := query.And{
		chatmodel.OrderIdAtLeast(afterOrderId),
		query.Key{Path: []string{handler.getReadKey()}, Filter: query.NewComp(query.CompOpEq, true)},
	}
	if handler.getMessagesFilter() != nil {
		filter = append(filter, handler.getMessagesFilter())
	}
	return s.collection.Find(filter)
}

func (s *repository) GetUnreadMessageIdsInRange(ctx context.Context, afterOrderId, beforeOrderId string, lastStateId string, counterType chatmodel.CounterType) ([]string, error) {
	iter, err := s.unreadInRangeQuery(afterOrderId, beforeOrderId, lastStateId, counterType).Iter(ctx)
	if err != nil {
		return nil, fmt.Errorf("find id: %w", err)
	}
	defer iter.Close()

	var msgIds []string
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("get doc: %w", err)
		}
		msgIds = append(msgIds, doc.Value().GetString("id"))
	}
	return msgIds, iter.Err()
}

// unreadInRangeQuery walks the unread messages with an order id in [afterOrderId, beforeOrderId].
func (s *repository) unreadInRangeQuery(afterOrderId, beforeOrderId string, lastStateId string, counterType chatmodel.CounterType) anystore.Query {
	return s.collection.Find(query.And{
		chatmodel.OrderIdAtLeast(afterOrderId),
		chatmodel.OrderIdAtMost(beforeOrderId),
		query.Or{
			query.Not{Filter: query.Key{Path: []string{chatmodel.StateIdKey}, Filter: query.Exists{}}},
			query.Key{Path: []string{chatmodel.StateIdKey}, Filter: query.NewComp(query.CompOpLte, lastStateId)},
		},
		newReadHandler(counterType).getReadFilter(false),
	})
}

func (s *repository) GetAllUnreadMessages(ctx context.Context, counterType chatmodel.CounterType) ([]string, error) {
	handler := newReadHandler(counterType)

	qry := query.And{
		handler.getReadFilter(false),
	}
	iter, err := s.collection.Find(qry).Iter(ctx)
	if err != nil {
		return nil, fmt.Errorf("find id: %w", err)
	}
	defer iter.Close()

	var msgIds []string
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("get doc: %w", err)
		}
		msgIds = append(msgIds, doc.Value().GetString("id"))
	}
	return msgIds, iter.Err()
}

func (s *repository) IterateMessagesForIndexing(ctx context.Context, afterOrderId string, proc func(msg *chatmodel.Message) error) error {
	iter, err := s.messagesForIndexingQuery(afterOrderId).Iter(ctx)
	if err != nil {
		return fmt.Errorf("init iterator: %w", err)
	}
	defer iter.Close()

	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return fmt.Errorf("get doc: %w", err)
		}
		msg, err := chatmodel.UnmarshalMessage(doc.Value())
		if err != nil {
			return fmt.Errorf("unmarshal message: %w", err)
		}
		if err = proc(msg); err != nil {
			return fmt.Errorf("process message: %w", err)
		}
	}
	return iter.Err()
}

// messagesForIndexingQuery walks, from the oldest, the live messages created or content-edited at
// or after afterOrderId (all live messages for an empty one).
func (s *repository) messagesForIndexingQuery(afterOrderId string) anystore.Query {
	// a tombstone has nothing to index; its search doc is removed through the delete queue
	var filter query.Filter = filterNotDeleted
	if afterOrderId != "" {
		// _o.id is the creation order (the storage key's prefix), _o.content the last content-edit
		// order: both new and edited messages need reindexing
		filter = query.And{
			query.Or{
				chatmodel.OrderIdAtLeast(afterOrderId),
				query.Key{Path: []string{chatmodel.OrderKey, "content"}, Filter: query.NewComp(query.CompOpGte, afterOrderId)},
			},
			filterNotDeleted,
		}
	}
	return s.collection.Find(filter).Sort(ascOrder)
}

func (r *repository) SetReadFlag(ctx context.Context, chatObjectId string, msgIds []string, counterType chatmodel.CounterType, value bool) ([]string, error) {
	handler := newReadHandler(counterType)

	arena := r.arenaPool.Get()
	defer func() {
		arena.Reset()
		r.arenaPool.Put(arena)
	}()

	var idsModified []string

	chunks := lo.Chunk(msgIds, 100)
	for _, chunk := range chunks {
		modified, err := r.setReadFlag(ctx, arena, handler, chunk, value)
		if err != nil {
			return nil, err
		}
		idsModified = append(idsModified, modified...)
	}

	return idsModified, nil
}

func (r *repository) setReadFlag(ctx context.Context, arena *anyenc.Arena, handler readHandler, msgIds []string, value bool) ([]string, error) {
	arena.Reset()
	encIds := make([]*anyenc.Value, 0, len(msgIds))
	for _, id := range msgIds {
		encIds = append(encIds, arena.NewString(id))
	}

	mod := handler.readModifier(value)
	_, err := r.collection.Find(query.And{
		handler.getReadFilter(!value),
		query.Key{
			Path:   []string{"id"},
			Filter: query.NewInValue(encIds...),
		},
	}).Update(ctx, mod)
	if err != nil {
		return nil, fmt.Errorf("update read flag: %w", err)
	}
	return mod.getModifiedIds(), nil
}

func (r *repository) SetSyncedByMaxOrderId(ctx context.Context, maxOrderId string) ([]string, error) {
	if maxOrderId == "" {
		return nil, nil
	}

	mod := &syncedModifier{value: true}
	_, err := r.unsyncedUpToQuery(maxOrderId).Update(ctx, mod)
	if err != nil {
		return nil, fmt.Errorf("set synced by max order id: %w", err)
	}
	return mod.getModifiedIds(), nil
}

// unsyncedUpToQuery walks the unsynced messages with an order id <= maxOrderId.
func (r *repository) unsyncedUpToQuery(maxOrderId string) anystore.Query {
	return r.collection.Find(query.And{
		filterSyncedFalse,
		chatmodel.OrderIdAtMost(maxOrderId),
	})
}

type GetMessagesRequest struct {
	AfterOrderId    string
	BeforeOrderId   string
	Limit           int
	IncludeBoundary bool
}

func (s *repository) GetMessages(ctx context.Context, req GetMessagesRequest) ([]*chatmodel.Message, error) {
	msgs, err := s.queryMessages(ctx, s.messagesPageQuery(req))
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	return msgs, nil
}

// messagesPageQuery walks the page of messages req asks for: a range of the collection in its
// natural order.
func (s *repository) messagesPageQuery(req GetMessagesRequest) anystore.Query {
	var filters query.And
	if req.AfterOrderId != "" {
		if req.IncludeBoundary {
			filters = append(filters, chatmodel.OrderIdAtLeast(req.AfterOrderId))
		} else {
			filters = append(filters, chatmodel.OrderIdAfter(req.AfterOrderId))
		}
	}
	if req.BeforeOrderId != "" {
		if req.IncludeBoundary {
			filters = append(filters, chatmodel.OrderIdAtMost(req.BeforeOrderId))
		} else {
			filters = append(filters, chatmodel.OrderIdBefore(req.BeforeOrderId))
		}
	}

	// Sort ASC only when paginating forward from AfterOrderId alone; otherwise
	// DESC returns the most recent N within the range / chat.
	sortOrder := descOrder
	if req.AfterOrderId != "" && req.BeforeOrderId == "" {
		sortOrder = ascOrder
	}

	if len(filters) == 0 {
		return s.collection.Find(nil).Sort(sortOrder).Limit(uint(req.Limit))
	}
	return s.collection.Find(filters).Sort(sortOrder).Limit(uint(req.Limit))
}

func (s *repository) queryMessages(ctx context.Context, query anystore.Query) ([]*chatmodel.Message, error) {
	arena := s.arenaPool.Get()
	defer func() {
		s.arenaPool.Put(arena)
	}()

	iter, err := query.Iter(ctx)
	if err != nil {
		return nil, fmt.Errorf("find iter: %w", err)
	}
	defer iter.Close()

	var res []*chatmodel.Message
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("get doc: %w", err)
		}

		msg, err := chatmodel.UnmarshalMessage(doc.Value())
		if err != nil {
			return nil, fmt.Errorf("unmarshal message: %w", err)
		}
		res = append(res, msg)
	}
	// reverse
	sort.Slice(res, func(i, j int) bool {
		return res[i].OrderId < res[j].OrderId
	})
	return res, nil
}

func (s *repository) HasMyReaction(ctx context.Context, myIdentity string, messageId string, emoji string) (bool, error) {
	var msg *chatmodel.Message
	err := FindMessageDoc(ctx, s.collection, messageId, func(doc *anyenc.Value) (err error) {
		msg, err = chatmodel.UnmarshalMessage(doc)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("find message: %w", err)
	}
	if v, ok := msg.GetReactions().GetReactions()[emoji]; ok {
		if slices.Contains(v.GetIds(), myIdentity) {
			return true, nil
		}
	}
	return false, nil
}

// GetMessagesByIds returns the stored messages with these ids, in the order asked; ids of messages
// not stored are skipped. The lookups go through the unique id index, 100 ids per query.
func (s *repository) GetMessagesByIds(ctx context.Context, messageIds []string) ([]*chatmodel.Message, error) {
	txn, err := s.ReadTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("start read tx: %w", err)
	}
	defer txn.Commit()

	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()
	byId := make(map[string]*chatmodel.Message, len(messageIds))
	for _, chunk := range lo.Chunk(messageIds, 100) {
		if err = s.collectMessagesById(txn.Context(), arena, chunk, byId); err != nil {
			return nil, errors.Join(txn.Commit(), err)
		}
	}

	messages := make([]*chatmodel.Message, 0, len(messageIds))
	returned := make(map[string]struct{}, len(byId))
	for _, messageId := range messageIds {
		msg, ok := byId[messageId]
		if !ok {
			continue
		}
		if _, again := returned[messageId]; again {
			// an id asked twice gets a message of its own, as each lookup returned one
			msg = msg.Clone()
		}
		returned[messageId] = struct{}{}
		messages = append(messages, msg)
	}
	return messages, nil
}

func (s *repository) collectMessagesById(ctx context.Context, arena *anyenc.Arena, messageIds []string, byId map[string]*chatmodel.Message) error {
	arena.Reset()
	ids := make([]*anyenc.Value, 0, len(messageIds))
	for _, id := range messageIds {
		ids = append(ids, arena.NewString(id))
	}
	iter, err := s.collection.Find(query.Key{Path: []string{"id"}, Filter: query.NewInValue(ids...)}).Iter(ctx)
	if err != nil {
		return fmt.Errorf("find messages by id: %w", err)
	}
	defer iter.Close()
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return fmt.Errorf("read message: %w", err)
		}
		msg, err := chatmodel.UnmarshalMessage(doc.Value())
		if err != nil {
			return fmt.Errorf("unmarshal message: %w", err)
		}
		byId[msg.Id] = msg
	}
	if err = iter.Err(); err != nil {
		return fmt.Errorf("find messages by id: %w", err)
	}
	return nil
}

func (s *repository) GetLastMessages(ctx context.Context, limit uint) ([]*chatmodel.Message, error) {
	return s.queryMessages(ctx, s.lastMessagesQuery(limit))
}

func (s *repository) GetLastLiveMessages(ctx context.Context, limit uint) ([]*chatmodel.Message, error) {
	return s.queryMessages(ctx, s.lastLiveMessagesQuery(limit))
}

// The queries below are built in one place so the tests explain exactly what runs: each must
// keep its index (repository_test / softdelete_test).

// lastMessagesQuery walks the collection from the newest end.
func (s *repository) lastMessagesQuery(limit uint) anystore.Query {
	return s.collection.Find(nil).Sort(descOrder).Limit(limit)
}

// deletedMessagesQuery reads the tombstones through the sparse `deleted` index.
func (s *repository) deletedMessagesQuery() anystore.Query {
	return s.collection.Find(filterDeleted)
}

// lastLiveMessagesQuery walks the collection from the newest end, skipping tombstones.
func (s *repository) lastLiveMessagesQuery(limit uint) anystore.Query {
	return s.collection.Find(filterNotDeleted).Sort(descOrder).Limit(limit)
}

// lastLiveMessagesByCreatorQuery walks the (creator, storage key) index from the newest end,
// skipping tombstones.
func (s *repository) lastLiveMessagesByCreatorQuery(creator string, limit uint) anystore.Query {
	return s.collection.Find(query.And{
		query.Key{
			Path:   []string{chatmodel.CreatorKey},
			Filter: query.NewComp(query.CompOpEq, creator),
		},
		filterNotDeleted,
	}).Sort(descOrder).Limit(limit)
}

func (s *repository) GetLastMessagesByCreators(ctx context.Context, creators []string, limit uint) ([]*chatmodel.Message, error) {
	// one indexed query per creator: an Eq filter walks the compound
	// (creator, storage key) index with early exit at the limit, while an In filter
	// makes the planner fall back to a full-collection scan (measured 35ms vs
	// 80µs per creator at 50k messages). The per-creator newest-limit sets
	// are a superset of the combined newest-limit set, merged below.
	unique := make(map[string]struct{}, len(creators))
	var all []*chatmodel.Message
	for _, creator := range creators {
		if creator == "" {
			continue
		}
		if _, ok := unique[creator]; ok {
			continue
		}
		unique[creator] = struct{}{}

		messages, err := s.queryMessages(ctx, s.lastLiveMessagesByCreatorQuery(creator, limit))
		if err != nil {
			return nil, fmt.Errorf("query creator messages: %w", err)
		}
		all = append(all, messages...)
	}
	if len(unique) > 1 {
		// keep the GetLastMessages contract: newest `limit` selected,
		// returned in ascending order
		sort.Slice(all, func(i, j int) bool {
			return all[i].OrderId < all[j].OrderId
		})
		if uint(len(all)) > limit {
			all = all[uint(len(all))-limit:]
		}
	}
	return all, nil
}

func (s *repository) GetAllMessageAttachments(ctx context.Context, afterOrderId string) ([]MessageAttachmentInfo, error) {
	iter, err := s.attachmentsQuery(afterOrderId).Iter(ctx)
	if err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}
	defer iter.Close()

	var results []MessageAttachmentInfo
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			continue
		}

		msg, err := chatmodel.UnmarshalMessage(doc.Value())
		if err != nil {
			continue
		}

		var fileIds []string
		for _, att := range msg.Attachments {
			if att.Target != "" {
				fileIds = append(fileIds, att.Target)
			}
		}

		if len(fileIds) > 0 {
			results = append(results, MessageAttachmentInfo{
				MessageId: msg.Id,
				CreatedAt: msg.CreatedAt,
				FileIds:   fileIds,
			})
		}
	}
	return results, iter.Err()
}

// attachmentsQuery walks the messages that have attachments set, with an order id > afterOrderId
// when one is given.
func (s *repository) attachmentsQuery(afterOrderId string) anystore.Query {
	// This uses anystore's Exists filter - can be indexed in the future for better performance
	var filter query.Filter = query.Key{Path: []string{chatmodel.ContentKey, "attachments"}, Filter: query.Exists{}}
	if afterOrderId != "" {
		filter = query.And{
			filter,
			chatmodel.OrderIdAfter(afterOrderId),
		}
	}
	return s.collection.Find(filter)
}

func (s *repository) GetPinnedMessages(ctx context.Context) ([]*chatmodel.Message, error) {
	return s.queryMessages(ctx, s.pinnedMessagesQuery())
}

// pinnedMessagesQuery reads the pinned messages through the sparse `pinned` index and sorts them,
// newest first: there are few.
func (s *repository) pinnedMessagesQuery() anystore.Query {
	return s.collection.Find(query.Key{Path: []string{chatmodel.PinnedKey}, Filter: query.NewComp(query.CompOpEq, true)}).Sort(descOrder)
}

func (s *repository) GetAllUnreadReactionChangeIds(ctx context.Context) ([]string, error) {
	iter, err := s.collection.Find(filterReactionUnread).Iter(ctx)
	if err != nil {
		return nil, fmt.Errorf("find unread reactions: %w", err)
	}
	defer iter.Close()

	var changeIds []string
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("get doc: %w", err)
		}

		msg, err := chatmodel.UnmarshalMessage(doc.Value())
		if err != nil {
			return nil, fmt.Errorf("unmarshal message: %w", err)
		}

		for _, identities := range msg.UnreadReactionIds {
			for _, entry := range identities {
				if entry.ChangeId != "" {
					changeIds = append(changeIds, entry.ChangeId)
				}
			}
		}
	}
	return changeIds, iter.Err()
}

func (s *repository) ClearUnreadReactions(ctx context.Context, maxOrderId string) (modifiedMsgIds []string, err error) {
	filter := query.Filter(filterReactionUnread)
	if maxOrderId != "" {
		filter = query.And{
			filterReactionUnread,
			query.Key{Path: []string{chatmodel.ReactionUnreadOrderIdKey}, Filter: query.NewComp(query.CompOpLte, maxOrderId)},
		}
	}

	const batchSize = 100
	for {
		mod := &reactionReadModifier{maxOrderId: maxOrderId}
		_, err := s.collection.Find(filter).Limit(batchSize).Update(ctx, mod)
		if err != nil {
			return nil, fmt.Errorf("clear unread reactions: %w", err)
		}
		modifiedMsgIds = append(modifiedMsgIds, mod.modifiedIds...)
		if len(mod.modifiedIds) < batchSize {
			break
		}
	}
	return modifiedMsgIds, nil
}

func (s *repository) GetNewestUnreadReactionOrderId(ctx context.Context) (string, error) {
	iter, err := s.collection.Find(filterReactionUnread).Sort("-" + chatmodel.ReactionUnreadOrderIdKey).Limit(1).Iter(ctx)
	if err != nil {
		return "", fmt.Errorf("find newest unread reaction: %w", err)
	}
	defer iter.Close()

	if iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return "", fmt.Errorf("get doc: %w", err)
		}
		orders := doc.Value().GetObject(chatmodel.OrderKey)
		if orders != nil {
			return orders.Get("id").GetString(), nil
		}
	}
	return "", nil
}

func (s *repository) GetAllRawMessages(ctx context.Context) ([]json.RawMessage, error) {
	iter, err := s.allMessagesQuery().Iter(ctx)
	if err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}
	defer iter.Close()

	var result []json.RawMessage
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("get doc: %w", err)
		}
		result = append(result, json.RawMessage(doc.Value().String()))
	}
	return result, nil
}

// allMessagesQuery walks the whole collection from the oldest.
func (s *repository) allMessagesQuery() anystore.Query {
	return s.collection.Find(nil).Sort(ascOrder)
}
