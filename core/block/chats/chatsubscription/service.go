package chatsubscription

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
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
	Flush()
	ReadMessages(newOldestOrderId string, idsModified []string, counterType chatmodel.CounterType)
	UnreadMessages(newOldestOrderId string, lastStateId string, msgIds []string, counterType chatmodel.CounterType)
}

type Service interface {
	app.ComponentRunnable
	GetManager(chatObjectId string) (Manager, error)

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

	identityCache *expirable.LRU[string, *domain.Details]
	lock          sync.Mutex
	managers      map[string]*subscriptionManager
}

func New() Service {
	return &service{
		managers: make(map[string]*subscriptionManager),
	}
}

func (s *service) Init(a *app.App) (err error) {
	s.componentCtx, s.componentCtxCancel = context.WithCancel(context.Background())

	s.spaceIdResolver = app.MustComponent[idresolver.Resolver](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.eventSender = app.MustComponent[event.Sender](a)
	s.repositoryService = app.MustComponent[chatrepository.Service](a)
	s.accountService = app.MustComponent[AccountService](a)
	s.identityCache = expirable.NewLRU[string, *domain.Details](50, nil, time.Minute)
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

func (s *service) GetManager(chatObjectId string) (Manager, error) {
	return s.getManager(chatObjectId)
}

func (s *service) getManager(chatObjectId string) (*subscriptionManager, error) {
	s.lock.Lock()
	mngr, ok := s.managers[chatObjectId]
	if ok {
		s.lock.Unlock()
		return mngr, nil
	}

	mngr = &subscriptionManager{}
	mngr.Lock()
	defer mngr.Unlock()
	s.managers[chatObjectId] = mngr
	s.lock.Unlock()

	err := s.initManager(chatObjectId, mngr)
	if err != nil {
		return nil, fmt.Errorf("init manager: %w", err)
	}

	return mngr, nil
}

func (s *service) initManager(chatObjectId string, mngr *subscriptionManager) error {
	spaceId, err := s.spaceIdResolver.ResolveSpaceID(chatObjectId)
	if err != nil {
		return fmt.Errorf("resolve space id: %w", err)
	}

	currentIdentity := s.accountService.AccountID()
	currentParticipantId := domain.NewParticipantId(spaceId, currentIdentity)

	repository, err := s.repositoryService.Repository(chatObjectId)
	if err != nil {
		return fmt.Errorf("get repository: %w", err)
	}
	mngr.componentCtx = s.componentCtx
	mngr.spaceId = spaceId
	mngr.chatId = chatObjectId
	mngr.myIdentity = currentIdentity
	mngr.myParticipantId = currentParticipantId
	mngr.identityCache = s.identityCache
	mngr.subscriptions = make(map[string]*subscription)
	mngr.spaceIndex = s.objectStore.SpaceIndex(spaceId)
	mngr.eventSender = s.eventSender
	mngr.repository = repository

	s.managers[chatObjectId] = mngr

	err = mngr.loadChatState(s.componentCtx)
	if err != nil {
		return fmt.Errorf("init chat state: %w", err)
	}

	return nil
}

type SubscribeLastMessagesRequest struct {
	ChatObjectId string
	SubId        string
	Limit        int
	// If AsyncInit is true, initial messages will be broadcast via events
	AsyncInit        bool
	WithDependencies bool
}

type SubscribeLastMessagesResponse struct {
	Messages  []*chatmodel.Message
	ChatState *model.ChatState
	// Dependencies per message id
	Dependencies map[string][]*domain.Details
}

func (s *service) SubscribeLastMessages(ctx context.Context, req SubscribeLastMessagesRequest) (*SubscribeLastMessagesResponse, error) {
	mngr, err := s.getManager(req.ChatObjectId)
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

	mngr.subscribe(req.SubId, req.WithDependencies)

	if req.AsyncInit {
		var previousOrderId string
		if len(messages) > 0 {
			previousOrderId, err = mngr.repository.GetPrevOrderId(txn.Context(), messages[0].OrderId)
			if err != nil {
				return nil, fmt.Errorf("get previous order id: %w", err)
			}
		}
		for _, message := range messages {
			mngr.Add(previousOrderId, message)
			previousOrderId = message.OrderId
		}

		// Force chatState to be sent
		mngr.chatStateUpdated = true
		mngr.Flush()
		return nil, nil
	} else {
		depsPerMessage := map[string][]*domain.Details{}
		if req.WithDependencies {
			for _, message := range messages {
				deps := mngr.collectMessageDependencies(message)
				depsPerMessage[message.Id] = deps
			}
		}
		return &SubscribeLastMessagesResponse{
			Messages:     messages,
			ChatState:    mngr.GetChatState(),
			Dependencies: depsPerMessage,
		}, nil
	}
}

func (s *service) Unsubscribe(chatObjectId string, subId string) error {
	mngr, err := s.getManager(chatObjectId)
	if err != nil {
		return fmt.Errorf("get manager: %w", err)
	}
	mngr.lock.Lock()
	defer mngr.lock.Unlock()

	mngr.unsubscribe(subId)
	return nil
}
