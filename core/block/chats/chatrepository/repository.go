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

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/anyproto/any-sync/app"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/anystorehelper"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "chatrepository"

const (
	descOrder   = "-_o.id"
	ascOrder    = "_o.id"
	descStateId = "-stateId"

	messageHistorySchemaVersionId = "$message-history-v1"
)

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

	collectionName := chatObjectId + "chats"
	collection, err := crdtDb.OpenCollection(s.componentCtx, collectionName)
	if errors.Is(err, anystore.ErrCollectionNotFound) {
		collection, err = crdtDb.CreateCollection(s.componentCtx, collectionName)
		if err != nil {
			return nil, fmt.Errorf("create collection: %w", err)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}
	historyCollectionName := chatObjectId + "chatMessageHistory"
	historyCollection, err := crdtDb.OpenCollection(s.componentCtx, historyCollectionName)
	if errors.Is(err, anystore.ErrCollectionNotFound) {
		historyCollection, err = crdtDb.CreateCollection(s.componentCtx, historyCollectionName)
		if err != nil {
			return nil, fmt.Errorf("create message history collection: %w", err)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("get message history collection: %w", err)
	}

	if err = anystorehelper.AddIndexes(s.componentCtx, collection, []anystore.IndexInfo{
		{Fields: []string{"_o.id"}},
		{Fields: []string{chatmodel.PinnedKey}, Sparse: true},
		{Fields: []string{chatmodel.ReactionUnreadOrderIdKey}, Sparse: true},
		// serves GetLastMessagesByCreators: creator Eq + order walk with early
		// exit (measured ~80µs vs a 25-45ms full-collection scan at 50k msgs)
		{Fields: []string{chatmodel.CreatorKey, "_o.id"}},
	}); err != nil {
		return nil, fmt.Errorf("ensure indexes: %w", err)
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
	GetLastMessages(ctx context.Context, limit uint) ([]*chatmodel.Message, error)
	// GetLastMessagesByCreators returns the newest messages authored by any of
	// the given identities (exact match), in ascending order like
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

	if _, err := s.RecordMessage(ctx, msg.Id); err != nil {
		return err
	}
	return s.collection.Insert(ctx, val)
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
	iter, err := s.collection.Find(query.Key{Path: []string{chatmodel.OrderKey, "id"}, Filter: query.NewComp(query.CompOpLt, orderId)}).
		Sort(descOrder).
		Limit(1).
		Iter(ctx)
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

func (s *repository) CountMessages(ctx context.Context) (int, error) {
	return s.collection.Find(nil).Count(ctx)
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
	handler := newReadHandler(counterType)
	unreadQuery := s.collection.Find(handler.getReadFilter(false)).Sort(ascOrder)

	iter, err := unreadQuery.Limit(1).Iter(ctx)
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

func (s *repository) countUnreadMessages(ctx context.Context, handler readHandler) (int, error) {
	unreadQuery := s.collection.Find(handler.getReadFilter(false))

	return unreadQuery.Count(ctx)
}

func (s *repository) GetReadMessagesAfter(ctx context.Context, afterOrderId string, counterType chatmodel.CounterType) ([]string, error) {
	handler := newReadHandler(counterType)

	filter := query.And{
		query.Key{Path: []string{chatmodel.OrderKey, "id"}, Filter: query.NewComp(query.CompOpGte, afterOrderId)},
		query.Key{Path: []string{handler.getReadKey()}, Filter: query.NewComp(query.CompOpEq, true)},
	}
	if handler.getMessagesFilter() != nil {
		filter = append(filter, handler.getMessagesFilter())
	}

	iter, err := s.collection.Find(filter).Iter(ctx)
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

func (s *repository) GetUnreadMessageIdsInRange(ctx context.Context, afterOrderId, beforeOrderId string, lastStateId string, counterType chatmodel.CounterType) ([]string, error) {
	handler := newReadHandler(counterType)

	qry := query.And{
		query.Key{Path: []string{chatmodel.OrderKey, "id"}, Filter: query.NewComp(query.CompOpGte, afterOrderId)},
		query.Key{Path: []string{chatmodel.OrderKey, "id"}, Filter: query.NewComp(query.CompOpLte, beforeOrderId)},
		query.Or{
			query.Not{Filter: query.Key{Path: []string{chatmodel.StateIdKey}, Filter: query.Exists{}}},
			query.Key{Path: []string{chatmodel.StateIdKey}, Filter: query.NewComp(query.CompOpLte, lastStateId)},
		},
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
	var filter query.Filter
	if afterOrderId != "" {
		// _o.id is the creation order, _o.content the last content-edit order:
		// both new and edited messages need reindexing
		filter = query.Or{
			query.Key{Path: []string{chatmodel.OrderKey, "id"}, Filter: query.NewComp(query.CompOpGte, afterOrderId)},
			query.Key{Path: []string{chatmodel.OrderKey, "content"}, Filter: query.NewComp(query.CompOpGte, afterOrderId)},
		}
	}
	iter, err := s.collection.Find(filter).Sort(ascOrder).Iter(ctx)
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

	filter := query.And{
		filterSyncedFalse,
		query.Key{Path: []string{chatmodel.OrderKey, "id"}, Filter: query.NewComp(query.CompOpLte, maxOrderId)},
	}

	mod := &syncedModifier{value: true}
	_, err := r.collection.Find(filter).Update(ctx, mod)
	if err != nil {
		return nil, fmt.Errorf("set synced by max order id: %w", err)
	}
	return mod.getModifiedIds(), nil
}

type GetMessagesRequest struct {
	AfterOrderId    string
	BeforeOrderId   string
	Limit           int
	IncludeBoundary bool
}

func (s *repository) GetMessages(ctx context.Context, req GetMessagesRequest) ([]*chatmodel.Message, error) {
	var filters query.And
	if req.AfterOrderId != "" {
		operator := query.CompOpGt
		if req.IncludeBoundary {
			operator = query.CompOpGte
		}
		filters = append(filters, query.Key{Path: []string{chatmodel.OrderKey, "id"}, Filter: query.NewComp(operator, req.AfterOrderId)})
	}
	if req.BeforeOrderId != "" {
		operator := query.CompOpLt
		if req.IncludeBoundary {
			operator = query.CompOpLte
		}
		filters = append(filters, query.Key{Path: []string{chatmodel.OrderKey, "id"}, Filter: query.NewComp(operator, req.BeforeOrderId)})
	}

	// Sort ASC only when paginating forward from AfterOrderId alone; otherwise
	// DESC returns the most recent N within the range / chat.
	sortOrder := descOrder
	if req.AfterOrderId != "" && req.BeforeOrderId == "" {
		sortOrder = ascOrder
	}

	var qry anystore.Query
	if len(filters) == 0 {
		qry = s.collection.Find(nil).Sort(sortOrder).Limit(uint(req.Limit))
	} else {
		qry = s.collection.Find(filters).Sort(sortOrder).Limit(uint(req.Limit))
	}

	msgs, err := s.queryMessages(ctx, qry)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	return msgs, nil
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
	doc, err := s.collection.FindId(ctx, messageId)
	if err != nil {
		return false, fmt.Errorf("find message: %w", err)
	}

	msg, err := chatmodel.UnmarshalMessage(doc.Value())
	if err != nil {
		return false, fmt.Errorf("unmarshal message: %w", err)
	}
	if v, ok := msg.GetReactions().GetReactions()[emoji]; ok {
		if slices.Contains(v.GetIds(), myIdentity) {
			return true, nil
		}
	}
	return false, nil
}

func (s *repository) GetMessagesByIds(ctx context.Context, messageIds []string) ([]*chatmodel.Message, error) {
	txn, err := s.ReadTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("start read tx: %w", err)
	}
	defer txn.Commit()

	messages := make([]*chatmodel.Message, 0, len(messageIds))
	for _, messageId := range messageIds {
		obj, err := s.collection.FindId(txn.Context(), messageId)
		if errors.Is(err, anystore.ErrDocNotFound) {
			continue
		}
		if err != nil {
			return nil, errors.Join(txn.Commit(), fmt.Errorf("find id: %w", err))
		}
		msg, err := chatmodel.UnmarshalMessage(obj.Value())
		if err != nil {
			return nil, errors.Join(txn.Commit(), fmt.Errorf("unmarshal message: %w", err))
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

func (s *repository) GetLastMessages(ctx context.Context, limit uint) ([]*chatmodel.Message, error) {
	qry := s.collection.Find(nil).Sort(descOrder).Limit(limit)
	return s.queryMessages(ctx, qry)
}

func (s *repository) GetLastMessagesByCreators(ctx context.Context, creators []string, limit uint) ([]*chatmodel.Message, error) {
	// one indexed query per creator: an Eq filter walks the compound
	// (creator, _o.id) index with early exit at the limit, while an In filter
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

		qry := s.collection.Find(query.Key{
			Path:   []string{chatmodel.CreatorKey},
			Filter: query.NewComp(query.CompOpEq, creator),
		}).Sort(descOrder).Limit(limit)
		messages, err := s.queryMessages(ctx, qry)
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
	// Filter to only get messages that have attachments set
	// This uses anystore's Exists filter - can be indexed in the future for better performance
	var filter query.Filter = query.Key{Path: []string{chatmodel.ContentKey, "attachments"}, Filter: query.Exists{}}

	if afterOrderId != "" {
		filter = query.And{
			filter,
			query.Key{Path: []string{chatmodel.OrderKey, "id"}, Filter: query.NewComp(query.CompOpGt, afterOrderId)},
		}
	}

	iter, err := s.collection.Find(filter).Iter(ctx)
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

func (s *repository) GetPinnedMessages(ctx context.Context) ([]*chatmodel.Message, error) {
	qry := s.collection.Find(query.Key{Path: []string{chatmodel.PinnedKey}, Filter: query.NewComp(query.CompOpEq, true)}).Sort(descOrder)
	return s.queryMessages(ctx, qry)
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
	iter, err := s.collection.Find(nil).Sort(ascOrder).Iter(ctx)
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
