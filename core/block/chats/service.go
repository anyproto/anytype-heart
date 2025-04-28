package chats

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/chatobject"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	subscriptionservice "github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "core.block.chats"

var log = logging.Logger(CName).Desugar()

type Service interface {
	AddMessage(ctx context.Context, sessionCtx session.Context, chatObjectId string, message *chatobject.Message) (string, error)
	EditMessage(ctx context.Context, chatObjectId string, messageId string, newMessage *chatobject.Message) error
	ToggleMessageReaction(ctx context.Context, chatObjectId string, messageId string, emoji string) error
	DeleteMessage(ctx context.Context, chatObjectId string, messageId string) error
	GetMessages(ctx context.Context, chatObjectId string, req chatobject.GetMessagesRequest) (*chatobject.GetMessagesResponse, error)
	GetMessagesByIds(ctx context.Context, chatObjectId string, messageIds []string) ([]*chatobject.Message, error)
	SubscribeLastMessages(ctx context.Context, chatObjectId string, limit int, subId string) (*chatobject.SubscribeLastMessagesResponse, error)
	ReadMessages(ctx context.Context, req ReadMessagesRequest) error
	UnreadMessages(ctx context.Context, chatObjectId string, afterOrderId string, counterType chatobject.CounterType) error
	Unsubscribe(chatObjectId string, subId string) error

	SubscribeToMessagePreviews(ctx context.Context, subId string) (*SubscribeToMessagePreviewsResponse, error)
	UnsubscribeFromMessagePreviews(subId string) error

	app.ComponentRunnable
}

var _ Service = (*service)(nil)

type service struct {
	objectGetter         cache.ObjectGetter
	crossSpaceSubService crossspacesub.Service

	componentCtx       context.Context
	componentCtxCancel context.CancelFunc

	chatObjectsSubQueue *mb.MB[*pb.EventMessage]

	lock sync.Mutex
	// chatObjectId => spaceId
	allChatObjectIds map[string]string

	// set of ids of subscriptions where to broadcast events
	subscriptionIds map[string]struct{}
}

func New() Service {
	return &service{
		allChatObjectIds:    make(map[string]string),
		subscriptionIds:     make(map[string]struct{}),
		chatObjectsSubQueue: mb.New[*pb.EventMessage](0),
	}
}

func (s *service) Name() string {
	return CName
}

func (s *service) Init(a *app.App) error {
	s.objectGetter = app.MustComponent[cache.ObjectGetter](a)
	s.crossSpaceSubService = app.MustComponent[crossspacesub.Service](a)
	s.componentCtx, s.componentCtxCancel = context.WithCancel(context.Background())

	return nil
}

const (
	// id for cross-space subscription
	allChatsSubscriptionId = "allChatObjects"
)

type ChatPreview struct {
	SpaceId      string
	ChatObjectId string
	State        *model.ChatState
	Message      *chatobject.Message
}

type SubscribeToMessagePreviewsResponse struct {
	Previews []*ChatPreview
}

func (s *service) SubscribeToMessagePreviews(ctx context.Context, subId string) (*SubscribeToMessagePreviewsResponse, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.hasPreviewSubscription(subId) {
		err := s.unsubscribeFromMessagePreviews(subId)
		if err != nil {
			return nil, fmt.Errorf("stop previous subscription: %w", err)
		}
	}

	s.subscriptionIds[subId] = struct{}{}

	result := &SubscribeToMessagePreviewsResponse{
		Previews: make([]*ChatPreview, 0, len(s.allChatObjectIds)),
	}
	for chatObjectId := range s.allChatObjectIds {
		chatAddResp, err := s.onChatAdded(chatObjectId, subId, false)
		if err != nil {
			log.Error("init lastMessage subscription", zap.Error(err))
			continue
		}
		var message *chatobject.Message
		if len(chatAddResp.Messages) > 0 {
			message = chatAddResp.Messages[0]
		}
		result.Previews = append(result.Previews, &ChatPreview{
			ChatObjectId: chatObjectId,
			State:        chatAddResp.ChatState,
			Message:      message,
		})
	}
	return result, nil
}

func (s *service) hasPreviewSubscription(subId string) bool {
	_, ok := s.subscriptionIds[subId]
	return ok
}

func (s *service) UnsubscribeFromMessagePreviews(subId string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.unsubscribeFromMessagePreviews(subId)
}

func (s *service) unsubscribeFromMessagePreviews(subId string) error {
	delete(s.subscriptionIds, subId)

	for chatObjectId := range s.allChatObjectIds {
		err := s.Unsubscribe(chatObjectId, subId)
		if err != nil {
			log.Error("unsubscribe from preview sub", zap.Error(err))
		}
	}
	return nil
}

func (s *service) Run(ctx context.Context) error {
	resp, err := s.crossSpaceSubService.Subscribe(subscriptionservice.SubscribeRequest{
		SubId:             allChatsSubscriptionId,
		InternalQueue:     s.chatObjectsSubQueue,
		Keys:              []string{bundle.RelationKeyId.String(), bundle.RelationKeySpaceId.String()},
		NoDepSubscription: true,
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(model.ObjectType_chatDerived),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("cross-space sub: %w", err)
	}

	for _, rec := range resp.Records {
		s.allChatObjectIds[rec.GetString(bundle.RelationKeyId)] = rec.GetString(bundle.RelationKeySpaceId)
	}

	go s.monitorMessagePreviews()
	return nil
}

func (s *service) monitorMessagePreviews() {
	matcher := subscriptionservice.EventMatcher{
		OnAdd: func(spaceId string, add *pb.EventObjectSubscriptionAdd) {
			s.lock.Lock()
			defer s.lock.Unlock()

			s.allChatObjectIds[add.Id] = spaceId

			if len(s.subscriptionIds) == 0 {
				return
			}

			for subId := range s.subscriptionIds {
				_, err := s.onChatAdded(add.Id, subId, true)
				if err != nil {
					log.Error("init last message subscription", zap.Error(err))
				}
			}
		},
		OnRemove: func(spaceId string, remove *pb.EventObjectSubscriptionRemove) {
			s.lock.Lock()
			defer s.lock.Unlock()

			delete(s.allChatObjectIds, remove.Id)
			if len(s.subscriptionIds) == 0 {
				return
			}

			for subId := range s.subscriptionIds {
				err := s.onChatRemoved(remove.Id, subId)
				if err != nil {
					log.Error("unsubscribe from the last message", zap.Error(err))
				}
			}
		},
	}
	for {
		msg, err := s.chatObjectsSubQueue.WaitOne(s.componentCtx)
		if errors.Is(err, mb.ErrClosed) {
			return
		}
		if err != nil {
			log.Error("wait message", zap.Error(err))
			return
		}
		matcher.Match(msg)
	}
}

func (s *service) onChatAdded(chatObjectId string, subId string, asyncInit bool) (*chatobject.SubscribeLastMessagesResponse, error) {
	var resp *chatobject.SubscribeLastMessagesResponse
	err := cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		var err error
		resp, err = sb.SubscribeLastMessages(s.componentCtx, chatobject.SubscribeLastMessagesRequest{
			SubId:            subId,
			Limit:            1,
			AsyncInit:        asyncInit,
			WithDependencies: true,
		})
		if err != nil {
			return err
		}
		return nil
	})
	return resp, err
}

func (s *service) onChatRemoved(chatObjectId string, subId string) error {
	err := s.Unsubscribe(chatObjectId, subId)
	if err != nil && !errors.Is(err, domain.ErrObjectNotFound) {
		return err
	}
	return nil
}

func (s *service) Close(ctx context.Context) error {
	var err error
	s.lock.Lock()
	defer s.lock.Unlock()

	err = s.chatObjectsSubQueue.Close()

	s.componentCtxCancel()

	unsubErr := s.crossSpaceSubService.Unsubscribe(allChatsSubscriptionId)
	if !errors.Is(err, crossspacesub.ErrSubscriptionNotFound) {
		err = errors.Join(err, unsubErr)
	}
	return err
}

func (s *service) AddMessage(ctx context.Context, sessionCtx session.Context, chatObjectId string, message *chatobject.Message) (string, error) {
	var messageId string
	err := cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		var err error
		messageId, err = sb.AddMessage(ctx, sessionCtx, message)
		return err
	})
	return messageId, err
}

func (s *service) EditMessage(ctx context.Context, chatObjectId string, messageId string, newMessage *chatobject.Message) error {
	return cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		return sb.EditMessage(ctx, messageId, newMessage)
	})
}

func (s *service) ToggleMessageReaction(ctx context.Context, chatObjectId string, messageId string, emoji string) error {
	return cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		return sb.ToggleMessageReaction(ctx, messageId, emoji)
	})
}

func (s *service) DeleteMessage(ctx context.Context, chatObjectId string, messageId string) error {
	return cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		return sb.DeleteMessage(ctx, messageId)
	})
}

func (s *service) GetMessages(ctx context.Context, chatObjectId string, req chatobject.GetMessagesRequest) (*chatobject.GetMessagesResponse, error) {
	var resp *chatobject.GetMessagesResponse
	err := cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		var err error
		resp, err = sb.GetMessages(ctx, req)
		if err != nil {
			return err
		}
		return nil
	})
	return resp, err
}

func (s *service) GetMessagesByIds(ctx context.Context, chatObjectId string, messageIds []string) ([]*chatobject.Message, error) {
	var res []*chatobject.Message
	err := cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		msg, err := sb.GetMessagesByIds(ctx, messageIds)
		if err != nil {
			return err
		}
		res = msg
		return nil
	})
	return res, err
}

func (s *service) SubscribeLastMessages(ctx context.Context, chatObjectId string, limit int, subId string) (*chatobject.SubscribeLastMessagesResponse, error) {
	var resp *chatobject.SubscribeLastMessagesResponse
	err := cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		var err error
		resp, err = sb.SubscribeLastMessages(ctx, chatobject.SubscribeLastMessagesRequest{
			SubId:            subId,
			Limit:            limit,
			AsyncInit:        false,
			WithDependencies: false,
		})
		if err != nil {
			return err
		}
		return nil
	})
	return resp, err
}

func (s *service) Unsubscribe(chatObjectId string, subId string) error {
	return cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		return sb.Unsubscribe(subId)
	})
}

type ReadMessagesRequest struct {
	ChatObjectId  string
	AfterOrderId  string
	BeforeOrderId string
	LastStateId   string
	CounterType   chatobject.CounterType
}

func (s *service) ReadMessages(ctx context.Context, req ReadMessagesRequest) error {
	return cache.Do(s.objectGetter, req.ChatObjectId, func(sb chatobject.StoreObject) error {
		return sb.MarkReadMessages(ctx, req.AfterOrderId, req.BeforeOrderId, req.LastStateId, req.CounterType)
	})
}

func (s *service) UnreadMessages(ctx context.Context, chatObjectId string, afterOrderId string, counterType chatobject.CounterType) error {
	return cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		return sb.MarkMessagesAsUnread(ctx, afterOrderId, counterType)
	})
}
