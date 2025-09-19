package chatsubscription

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/chats/chatrepository"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
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
	SetSessionContext(ctx session.Context)
	UpdateReactions(message *chatmodel.Message)
	UpdateFull(message *chatmodel.Message)
	UpdateChatState(updater func(*model.ChatState) *model.ChatState)
	Add(prevOrderId string, message *chatmodel.Message)
	Delete(messageId string)
	ForceSendingChatState()
	Flush()
	ReadMessages(newOldestOrderId string, idsModified []string, counterType chatmodel.CounterType)
	UnreadMessages(newOldestOrderId string, lastStateId string, msgIds []string, counterType chatmodel.CounterType)
	UpdateSyncStatus(messageIds []string, isSynced bool)
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
	objectGetter      cache.ObjectWaitGetter

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
	s.objectGetter = app.MustComponent[cache.ObjectWaitGetter](a)
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

	repository, err := s.repositoryService.Repository(chatObjectId)
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
}

type SubscribeLastMessagesResponse struct {
	PreviousOrderId string
	Messages        []*chatmodel.Message
	ChatState       *model.ChatState
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
		return nil, fmt.Errorf("query messages: %w", err)
	}

	mngr.subscribe(req)

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

	// Warm up cache
	go func() {
		_, err = s.objectGetter.WaitAndGetObject(s.componentCtx, req.ChatObjectId)
		if err != nil {
			log.Error("load chat to cache", zap.String("chatObjectId", req.ChatObjectId), zap.Error(err))
		}
	}()

	return &SubscribeLastMessagesResponse{
		Messages:        messages,
		ChatState:       mngr.GetChatState(),
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
