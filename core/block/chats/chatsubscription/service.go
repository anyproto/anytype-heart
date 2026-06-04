package chatsubscription

/*
AI generated

Name: Chat Message Subscription Events
Scope: global

## Responsibility
- Manages subscriptions to chat messages and sends real-time events on changes
- Tracks message state: add, delete, update, reactions, read status, sync status
- Maintains chat state with unread counters for messages and mentions
- Resolves message dependencies (creator identity, attachment details)

## Documentation
Each chat object has one subscriptionManager, initialized lazily via futures for lock-free concurrent access.
Manager maintains a sliding window of messages (skiplist ordered by OrderId, capped by limit).
Changes accumulate in messagesState and flush as batched events after commit.
Supports sync events (via session context) and async events (via broadcast).
*/

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/hashicorp/golang-lru/v2/expirable"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/chats/chatrepository"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/futures"
)

const CName = "chatsubscription"

var log = logging.Logger(CName).Desugar()

type Manager interface {
	sync.Locker

	IsActive() bool
	GetChatState() *model.ChatState
	GetLastMessage() (*model.ChatMessage, bool, error)
	SetSessionContext(ctx session.Context)
	UpdateReactions(message *chatmodel.Message)
	UpdatePinned(message *chatmodel.Message)
	UpdateFull(message *chatmodel.Message)
	UpdateChatState(updater func(*model.ChatState) *model.ChatState)
	UpdateMessageCount(delta int32)
	GetMessageCount() int32
	Add(prevOrderId string, message *chatmodel.Message)
	Delete(messageId string)
	ForceSendingChatState()
	Flush(reloadStateIfNeeded bool)
	ReadMessages(newOldestOrderId string, idsModified []string, counterType chatmodel.CounterType)
	UnreadMessages(newOldestOrderId string, lastStateId string, msgIds []string, counterType chatmodel.CounterType)
	UpdateSyncStatus(messageIds []string, isSynced bool)
	UpdateReactionReadStatus(msgId string, unread bool)
	ReadReactions(newOrderId string, idsModified []string)
	ForceReloadReactionState()
}

type Service interface {
	app.ComponentRunnable
	GetManager(spaceId string, chatObjectId string) (Manager, error)

	SubscribeLastMessages(ctx context.Context, req SubscribeLastMessagesRequest) (*SubscribeLastMessagesResponse, error)
	Unsubscribe(chatObjectId string, subId string) error
}

type AccountService interface {
	AccountID() string
}

type service struct {
	componentCtx       context.Context
	componentCtxCancel context.CancelFunc

	spaceIdResolver   idresolver.Resolver
	objectStore       objectstore.ObjectStore
	eventSender       event.Sender
	repositoryService chatrepository.Service
	accountService    AccountService

	lock     sync.Mutex
	managers map[string]*futures.Future[*subscriptionManager]
}

func New() Service {
	return &service{
		managers: make(map[string]*futures.Future[*subscriptionManager]),
	}
}

func (s *service) Init(a *app.App) (err error) {
	s.componentCtx, s.componentCtxCancel = context.WithCancel(context.Background())

	s.spaceIdResolver = app.MustComponent[idresolver.Resolver](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.eventSender = app.MustComponent[event.Sender](a)
	s.repositoryService = app.MustComponent[chatrepository.Service](a)
	s.accountService = app.MustComponent[AccountService](a)
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run(ctx context.Context) (err error) {
	return nil
}

func (s *service) Close(ctx context.Context) (err error) {
	if s.componentCtxCancel != nil {
		s.componentCtxCancel()
	}
	return nil
}

func (s *service) GetManager(spaceId string, chatObjectId string) (Manager, error) {
	return s.getManager(spaceId, chatObjectId)
}

// getManagerFuture returns a future that should be resolved by the first who called this method.
// The idea behind using futures here is to initialize a manager once without blocking the whole service.
func (s *service) getManagerFuture(spaceId string, chatObjectId string) *futures.Future[*subscriptionManager] {
	s.lock.Lock()
	mngr, ok := s.managers[chatObjectId]
	if ok {
		s.lock.Unlock()
		return mngr
	}

	mngr = futures.New[*subscriptionManager]()
	s.managers[chatObjectId] = mngr
	s.lock.Unlock()

	mngr.Resolve(s.initManager(spaceId, chatObjectId))

	return mngr
}

func (s *service) getManager(spaceId string, chatObjectId string) (*subscriptionManager, error) {
	return s.getManagerFuture(spaceId, chatObjectId).Wait()
}

func (s *service) initManager(spaceId string, chatObjectId string) (*subscriptionManager, error) {
	currentIdentity := s.accountService.AccountID()
	currentParticipantId := domain.NewParticipantId(spaceId, currentIdentity)

	repository, err := s.repositoryService.Repository(spaceId, chatObjectId)
	if err != nil {
		return nil, fmt.Errorf("get repository: %w", err)
	}
	mngr := &subscriptionManager{
		componentCtx:    s.componentCtx,
		spaceId:         spaceId,
		chatId:          chatObjectId,
		myIdentity:      currentIdentity,
		myParticipantId: currentParticipantId,
		identityCache:   expirable.NewLRU[string, *domain.Details](50, nil, time.Minute),
		subscriptions:   make(map[string]*subscription),
		spaceIndex:      s.objectStore.SpaceIndex(spaceId),
		eventSender:     s.eventSender,
		repository:      repository,
	}

	err = mngr.loadChatState(s.componentCtx)
	if err != nil {
		err = fmt.Errorf("init chat state: %w", err)
		return nil, err
	}
	return mngr, nil
}

type SubscribeLastMessagesRequest struct {
	ChatObjectId           string
	SubId                  string
	Limit                  int
	WithDependencies       bool
	OnlyLastMessage        bool
	CouldUseSessionContext bool
	SseSink                chan<- *pb.Event
}

type SubscribeLastMessagesResponse struct {
	PreviousOrderId string
	Messages        []*chatmodel.Message
	ChatState       *model.ChatState
	MessageCount    int32
	// Dependencies per message id
	Dependencies map[string][]*domain.Details
}

func (s *service) SubscribeLastMessages(ctx context.Context, req SubscribeLastMessagesRequest) (*SubscribeLastMessagesResponse, error) {
	if req.ChatObjectId == "" {
		return nil, fmt.Errorf("empty chat object id")
	}

	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	spaceId, err := s.spaceIdResolver.ResolveSpaceIdWithRetry(ctx, req.ChatObjectId)
	if err != nil {
		return nil, fmt.Errorf("resolve space id: %w", err)
	}

	mngr, err := s.getManager(spaceId, req.ChatObjectId)
	if err != nil {
		return nil, fmt.Errorf("get manager: %w", err)
	}

	mngr.Lock()
	defer mngr.Unlock()

	txn, err := mngr.repository.ReadTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("init read transaction: %w", err)
	}
	defer txn.Commit()

	messages, err := mngr.repository.GetLastMessages(txn.Context(), uint(req.Limit))
	if err != nil {
		return nil, fmt.Errorf("query messagesMap: %w", err)
	}

	mngr.subscribe(req, messages)

	depsPerMessage := map[string][]*domain.Details{}
	if req.WithDependencies {
		for _, message := range messages {
			deps := mngr.collectMessageDependencies(message.ChatMessage)
			depsPerMessage[message.Id] = deps
		}
	}

	var previousOrderId string
	if len(messages) > 0 {
		previousOrderId, err = mngr.repository.GetPrevOrderId(txn.Context(), messages[0].OrderId)
		if err != nil {
			return nil, fmt.Errorf("get previous order id: %w", err)
		}
	}

	// No eager full-object warm-up here on purpose (GO-7302): SubscribeLastMessages
	// must not open chat trees on app start. Previews/last-messages and unread
	// counters are served from the durable CRDT store (the persisted repository
	// read above), and any-sync opens the chat object on demand — per-space
	// diffsync pulls a chat tree only when its heads diverge (head-syncing chats
	// first, see block.Service.GetPriorityIds), and peer push applies remote
	// changes.
	//
	// Force-opening every chat object here was costly: a cold open that
	// misses the object ocache runs any-sync BuildObjectTree, which reads
	// and decrypts the chat's whole change history (common snapshot -> head)
	// from SQLite (objecttreefactory rebuildFromStorage -> treebuilder
	// GetAfterOrder). Chat changes never set IsSnapshot so the common
	// snapshot rarely advances, making this effectively a full-history
	// rebuild per chat. SubscribeToMessagePreviews fans this across every
	// chat in every space, so it ran N concurrently at startup AND
	// force-built deferred spaces, defeating client-driven lazy
	// multi-space loading. (This is the base any-sync tree build, distinct
	// from and unaffected by the GO-7290 read-counter DiffManager rework,
	// which removed a separate BuildHistoryTree pass.)

	return &SubscribeLastMessagesResponse{
		Messages:        messages,
		ChatState:       mngr.GetChatState(),
		MessageCount:    mngr.GetMessageCount(),
		Dependencies:    depsPerMessage,
		PreviousOrderId: previousOrderId,
	}, nil
}

func (s *service) Unsubscribe(chatObjectId string, subId string) error {
	spaceId, err := s.spaceIdResolver.ResolveSpaceID(chatObjectId)
	if err != nil {
		return fmt.Errorf("resolve space id: %w", err)
	}

	mngr, err := s.getManager(spaceId, chatObjectId)
	if err != nil {
		return fmt.Errorf("get manager: %w", err)
	}
	mngr.lock.Lock()
	defer mngr.lock.Unlock()

	mngr.unsubscribe(subId)
	return nil
}
