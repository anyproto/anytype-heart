package chats

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"
	"go.uber.org/zap"
	"golang.org/x/exp/maps"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/chatobject"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
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
	AddMessage(ctx context.Context, sessionCtx session.Context, chatObjectId string, message *model.ChatMessage) (string, error)
	EditMessage(ctx context.Context, chatObjectId string, messageId string, newMessage *model.ChatMessage) error
	ToggleMessageReaction(ctx context.Context, chatObjectId string, messageId string, emoji string) error
	DeleteMessage(ctx context.Context, chatObjectId string, messageId string) error
	GetMessages(ctx context.Context, chatObjectId string, req chatobject.GetMessagesRequest) (*chatobject.GetMessagesResponse, error)
	GetMessagesByIds(ctx context.Context, chatObjectId string, messageIds []string) ([]*model.ChatMessage, error)
	SubscribeLastMessages(ctx context.Context, chatObjectId string, limit int, subId string) (*chatobject.SubscribeLastMessagesResponse, error)
	ReadMessages(ctx context.Context, chatObjectId string, afterOrderId string, beforeOrderId string, lastDbState int64) error
	UnreadMessages(ctx context.Context, chatObjectId string, afterOrderId string) error
	Unsubscribe(chatObjectId string, subId string) error

	SubscribeToMessagePreviews(ctx context.Context) (string, error)
	UnsubscribeFromMessagePreviews() error

	app.ComponentRunnable
}

var _ Service = (*service)(nil)

type service struct {
	objectGetter         cache.ObjectGetter
	crossSpaceSubService crossspacesub.Service

	componentCtx       context.Context
	componentCtxCancel context.CancelFunc

	eventSender event.Sender

	lock                sync.Mutex
	chatObjectsSubQueue *mb.MB[*pb.EventMessage]
	chatObjectIds       map[string]struct{}
}

func New() Service {
	return &service{}
}

func (s *service) Name() string {
	return CName
}

func (s *service) Init(a *app.App) error {
	s.objectGetter = app.MustComponent[cache.ObjectGetter](a)
	s.crossSpaceSubService = app.MustComponent[crossspacesub.Service](a)
	s.eventSender = app.MustComponent[event.Sender](a)
	s.componentCtx, s.componentCtxCancel = context.WithCancel(context.Background())

	return nil
}

const (
	allChatsSubscriptionId = "allChatObjects"
)

func (s *service) SubscribeToMessagePreviews(ctx context.Context) (string, error) {
	s.lock.Lock()
	if s.chatObjectsSubQueue != nil {
		s.lock.Unlock()
		return chatobject.LastMessageSubscriptionId, nil
	}
	s.chatObjectsSubQueue = mb.New[*pb.EventMessage](0)
	s.lock.Unlock()

	resp, err := s.crossSpaceSubService.Subscribe(subscriptionservice.SubscribeRequest{
		SubId:             allChatsSubscriptionId,
		InternalQueue:     s.chatObjectsSubQueue,
		Keys:              []string{bundle.RelationKeyId.String()},
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
		return "", fmt.Errorf("cross-space sub: %w", err)
	}
	for _, rec := range resp.Records {
		err := s.onChatAdded(rec.GetString(bundle.RelationKeyId))
		if err != nil {
			log.Error("init lastMessage subscription", zap.Error(err))
		}
	}
	go s.monitorChats()

	return chatobject.LastMessageSubscriptionId, nil
}

func (s *service) UnsubscribeFromMessagePreviews() error {
	err := s.crossSpaceSubService.Unsubscribe(allChatsSubscriptionId)
	if err != nil {
		return fmt.Errorf("unsubscribe from cross-space sub: %w", err)
	}

	s.lock.Lock()
	err = s.chatObjectsSubQueue.Close()
	if err != nil {
		log.Error("close cross-space chat objects queue", zap.Error(err))
	}
	s.chatObjectsSubQueue = nil
	chatIds := maps.Keys(s.chatObjectIds)
	s.lock.Unlock()

	for _, chatId := range chatIds {
		err := s.Unsubscribe(chatId, chatobject.LastMessageSubscriptionId)
		if err != nil {
			log.Error("unsubscribe from preview sub", zap.Error(err))
		}
	}
	return nil
}

func (s *service) Run(ctx context.Context) error {
	return nil
}

func (s *service) monitorChats() {
	matcher := subscriptionservice.EventMatcher{
		OnAdd: func(add *pb.EventObjectSubscriptionAdd) {
			err := s.onChatAdded(add.Id)
			if err != nil {
				log.Error("init last message subscription", zap.Error(err))
			}
		},
		OnRemove: func(remove *pb.EventObjectSubscriptionRemove) {
			err := s.onChatRemoved(remove.Id)
			if err != nil {
				log.Error("unsubscribe from the last message", zap.Error(err))
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

func (s *service) onChatAdded(chatObjectId string) error {
	s.lock.Lock()
	s.chatObjectIds[chatObjectId] = struct{}{}
	s.lock.Unlock()
	return cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		var err error
		_, err = sb.SubscribeLastMessages(s.componentCtx, chatobject.LastMessageSubscriptionId, 1, true)
		if err != nil {
			return err
		}
		return nil
	})
}

func (s *service) onChatRemoved(chatObjectId string) error {
	s.lock.Lock()
	delete(s.chatObjectIds, chatObjectId)
	s.lock.Unlock()

	err := s.Unsubscribe(chatObjectId, chatobject.LastMessageSubscriptionId)
	if err != nil && !errors.Is(err, domain.ErrObjectNotFound) {
		return err
	}
	return nil
}

func (s *service) Close(ctx context.Context) error {
	var err error
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.chatObjectsSubQueue != nil {
		err = s.chatObjectsSubQueue.Close()
	}

	s.componentCtxCancel()

	err = errors.Join(err,
		s.crossSpaceSubService.Unsubscribe(allChatsSubscriptionId),
	)
	return err
}

func (s *service) AddMessage(ctx context.Context, sessionCtx session.Context, chatObjectId string, message *model.ChatMessage) (string, error) {
	var messageId string
	err := cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		var err error
		messageId, err = sb.AddMessage(ctx, sessionCtx, message)
		return err
	})
	return messageId, err
}

func (s *service) EditMessage(ctx context.Context, chatObjectId string, messageId string, newMessage *model.ChatMessage) error {
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

func (s *service) GetMessagesByIds(ctx context.Context, chatObjectId string, messageIds []string) ([]*model.ChatMessage, error) {
	var res []*model.ChatMessage
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
		resp, err = sb.SubscribeLastMessages(ctx, subId, limit, false)
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

func (s *service) ReadMessages(ctx context.Context, chatObjectId string, afterOrderId string, beforeOrderId string, lastAddedMessageTimestamp int64) error {
	return cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		return sb.MarkReadMessages(ctx, afterOrderId, beforeOrderId, lastAddedMessageTimestamp)
	})
}

func (s *service) UnreadMessages(ctx context.Context, chatObjectId string, afterOrderId string) error {
	return cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		return sb.MarkMessagesAsUnread(ctx, afterOrderId)
	})
}
