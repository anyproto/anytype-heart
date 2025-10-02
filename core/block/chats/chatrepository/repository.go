package chatrepository

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/anyproto/any-sync/app"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "chatrepository"

var log = logging.Logger(CName).Desugar()

const (
	descOrder   = "-_o.id"
	ascOrder    = "_o.id"
	descStateId = "-stateId"
)

type Service interface {
	app.ComponentRunnable

	Repository(chatObjectId string) (Repository, error)
}

type service struct {
	componentCtx       context.Context
	componentCtxCancel context.CancelFunc

	objectStore     objectstore.ObjectStore
	dbProvider      anystoreprovider.Provider
	spaceIdResolver idresolver.Resolver
	arenaPool       *anyenc.ArenaPool
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
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Repository(chatObjectId string) (Repository, error) {
	spaceId, err := s.spaceIdResolver.ResolveSpaceID(chatObjectId)
	if err != nil {
		return nil, fmt.Errorf("resolve space id: %w", err)
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

	return &repository{
		collection: collection,
		arenaPool:  s.arenaPool,
	}, nil
}

type Repository interface {
	WriteTx(ctx context.Context) (anystore.WriteTx, error)
	ReadTx(ctx context.Context) (anystore.ReadTx, error)
	AddTestMessage(ctx context.Context, msg *chatmodel.Message) error
	GetLastStateId(ctx context.Context) (string, error)
	GetPrevOrderId(ctx context.Context, orderId string) (string, error)
	LoadChatState(ctx context.Context) (*model.ChatState, error)
	GetOldestOrderId(ctx context.Context, counterType chatmodel.CounterType) (string, error)
	GetReadMessagesAfter(ctx context.Context, afterOrderId string, counterType chatmodel.CounterType) ([]string, error)
	GetUnreadMessageIdsInRange(ctx context.Context, afterOrderId, beforeOrderId string, lastStateId string, counterType chatmodel.CounterType) ([]string, error)
	GetAllUnreadMessages(ctx context.Context, counterType chatmodel.CounterType) ([]string, error)
	SetReadFlag(ctx context.Context, chatObjectId string, msgIds []string, counterType chatmodel.CounterType, value bool) []string
	GetMessages(ctx context.Context, req GetMessagesRequest) ([]*chatmodel.Message, error)
	HasMyReaction(ctx context.Context, myIdentity string, messageId string, emoji string) (bool, error)
	GetMessagesByIds(ctx context.Context, messageIds []string) ([]*chatmodel.Message, error)
	GetLastMessages(ctx context.Context, limit uint) ([]*chatmodel.Message, error)
	SetSyncedFlag(ctx context.Context, chatObjectId string, msgIds []string, value bool) []string
}

type repository struct {
	collection anystore.Collection
	arenaPool  *anyenc.ArenaPool
}

func (s *repository) AddTestMessage(ctx context.Context, msg *chatmodel.Message) error {
	arena := s.arenaPool.Get()
	arena.Reset()
	defer s.arenaPool.Put(arena)

	val := arena.NewObject()
	msg.MarshalAnyenc(val, arena)

	orderObj := arena.NewObject()
	orderObj.Set("id", arena.NewString(msg.OrderId))
	val.Set(chatmodel.OrderKey, orderObj)

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

	return &model.ChatState{
		Messages:    messagesState,
		Mentions:    mentionsState,
		LastStateId: lastStateId,
	}, nil
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
	unreadQuery := s.collection.Find(handler.getUnreadFilter()).Sort(ascOrder)

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
	unreadQuery := s.collection.Find(handler.getUnreadFilter())

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
		handler.getUnreadFilter(),
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
		handler.getUnreadFilter(),
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

func (r *repository) SetReadFlag(ctx context.Context, chatObjectId string, msgIds []string, counterType chatmodel.CounterType, value bool) []string {
	handler := newReadHandler(counterType)

	var idsModified []string
	for _, id := range msgIds {
		if id == chatObjectId {
			// skip tree root
			continue
		}
		res, err := r.collection.UpdateId(ctx, id, handler.readModifier(value))
		// Not all changes are messages, skip them
		if errors.Is(err, anystore.ErrDocNotFound) {
			continue
		}
		if err != nil {
			log.Error("markReadMessages: update message", zap.Error(err), zap.String("changeId", id), zap.String("chatObjectId", chatObjectId))
			continue
		}
		if res.Modified > 0 {
			idsModified = append(idsModified, id)
		}
	}
	return idsModified
}

func (r *repository) SetSyncedFlag(ctx context.Context, chatObjectId string, msgIds []string, value bool) []string {
	var idsModified []string
	for _, id := range msgIds {
		if id == chatObjectId {
			// skip tree root
			continue
		}
		res, err := r.collection.UpdateId(ctx, id, query.ModifyFunc(func(a *anyenc.Arena, v *anyenc.Value) (result *anyenc.Value, modified bool, err error) {
			oldValue := v.GetBool(chatmodel.SyncedKey)
			if oldValue != value {
				v.Set(chatmodel.SyncedKey, arenaNewBool(a, value))
				return v, true, nil
			}
			return v, false, nil
		}))
		// Not all changes are messages, skip them
		if errors.Is(err, anystore.ErrDocNotFound) {
			continue
		}
		if err != nil {
			log.Error("set synced flag: update message", zap.Error(err), zap.String("changeId", id), zap.String("chatObjectId", chatObjectId))
			continue
		}
		if res.Modified > 0 {
			idsModified = append(idsModified, id)
		}
	}
	return idsModified
}

type GetMessagesRequest struct {
	AfterOrderId    string
	BeforeOrderId   string
	Limit           int
	IncludeBoundary bool
}

func (s *repository) GetMessages(ctx context.Context, req GetMessagesRequest) ([]*chatmodel.Message, error) {
	var qry anystore.Query
	if req.AfterOrderId != "" {
		operator := query.CompOpGt
		if req.IncludeBoundary {
			operator = query.CompOpGte
		}
		qry = s.collection.Find(query.Key{Path: []string{chatmodel.OrderKey, "id"}, Filter: query.NewComp(operator, req.AfterOrderId)}).Sort(ascOrder).Limit(uint(req.Limit))
	} else if req.BeforeOrderId != "" {
		operator := query.CompOpLt
		if req.IncludeBoundary {
			operator = query.CompOpLte
		}
		qry = s.collection.Find(query.Key{Path: []string{chatmodel.OrderKey, "id"}, Filter: query.NewComp(operator, req.BeforeOrderId)}).Sort(descOrder).Limit(uint(req.Limit))
	} else {
		qry = s.collection.Find(nil).Sort(descOrder).Limit(uint(req.Limit))
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
		arena.Reset()
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
