package chats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/chats/chatpush"
	"github.com/anyproto/anytype-heart/core/block/editor/chatobject"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/identity"
	"github.com/anyproto/anytype-heart/core/session"
	subscriptionservice "github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
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

	SubscribeToMessagePreviews(ctx context.Context) (string, error)
	UnsubscribeFromMessagePreviews() error

	app.ComponentRunnable
}

var _ Service = (*service)(nil)

type pushService interface {
	Notify(ctx context.Context, spaceId string, topic []string, payload []byte) (err error)
}

type accountService interface {
	AccountID() string
}

type service struct {
	objectGetter         cache.ObjectGetter
	crossSpaceSubService crossspacesub.Service
	pushService          pushService
	accountService       accountService

	componentCtx       context.Context
	componentCtxCancel context.CancelFunc

	chatObjectsSubQueue *mb.MB[*pb.EventMessage]

	lock                      sync.Mutex
	isMessagePreviewSubActive bool
	chatObjectIds             map[string]struct{}

	objectStore     objectstore.ObjectStore
	identityService identity.Service
}

func New() Service {
	return &service{
		chatObjectIds:       map[string]struct{}{},
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
	s.pushService = app.MustComponent[pushService](a)
	s.accountService = app.MustComponent[accountService](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.identityService = app.MustComponent[identity.Service](a)

	return nil
}

const (
	allChatsSubscriptionId = "allChatObjects"
)

func (s *service) SubscribeToMessagePreviews(ctx context.Context) (string, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.isMessagePreviewSubActive {
		err := s.unsubscribeFromMessagePreviews()
		if err != nil {
			return "", fmt.Errorf("stop previous subscription: %w", err)
		}
	}
	s.isMessagePreviewSubActive = true

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

	return chatobject.LastMessageSubscriptionId, nil
}

func (s *service) UnsubscribeFromMessagePreviews() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.unsubscribeFromMessagePreviews()
}

func (s *service) unsubscribeFromMessagePreviews() error {
	err := s.crossSpaceSubService.Unsubscribe(allChatsSubscriptionId)
	if err != nil {
		return fmt.Errorf("unsubscribe from cross-space sub: %w", err)
	}

	s.isMessagePreviewSubActive = false
	chatIds := lo.Keys(s.chatObjectIds)
	for key := range s.chatObjectIds {
		delete(s.chatObjectIds, key)
	}

	for _, chatId := range chatIds {
		err := s.Unsubscribe(chatId, chatobject.LastMessageSubscriptionId)
		if err != nil {
			log.Error("unsubscribe from preview sub", zap.Error(err))
		}
	}
	return nil
}

func (s *service) Run(ctx context.Context) error {
	go s.monitorMessagePreviews()
	return nil
}

func (s *service) monitorMessagePreviews() {
	matcher := subscriptionservice.EventMatcher{
		OnAdd: func(add *pb.EventObjectSubscriptionAdd) {
			s.lock.Lock()
			defer s.lock.Unlock()
			if !s.isMessagePreviewSubActive {
				return
			}

			err := s.onChatAdded(add.Id)
			if err != nil {
				log.Error("init last message subscription", zap.Error(err))
			}
		},
		OnRemove: func(remove *pb.EventObjectSubscriptionRemove) {
			s.lock.Lock()
			defer s.lock.Unlock()
			if !s.isMessagePreviewSubActive {
				return
			}

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
	if _, ok := s.chatObjectIds[chatObjectId]; ok {
		return nil
	}
	s.chatObjectIds[chatObjectId] = struct{}{}

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
	delete(s.chatObjectIds, chatObjectId)

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

	err = s.chatObjectsSubQueue.Close()

	s.componentCtxCancel()

	unsubErr := s.crossSpaceSubService.Unsubscribe(allChatsSubscriptionId)
	if !errors.Is(err, crossspacesub.ErrSubscriptionNotFound) {
		err = errors.Join(err, unsubErr)
	}
	return err
}

func (s *service) AddMessage(ctx context.Context, sessionCtx session.Context, chatObjectId string, message *chatobject.Message) (string, error) {
	var messageId, spaceId string
	err := cache.Do(s.objectGetter, chatObjectId, func(sb chatobject.StoreObject) error {
		var err error
		messageId, err = sb.AddMessage(ctx, sessionCtx, message)
		spaceId = sb.SpaceID()
		return err
	})
	if err == nil {
		go s.sendPushNotification(spaceId, chatObjectId, messageId, message.Message.Text)
	}
	return messageId, err
}

func (s *service) sendPushNotification(spaceId, chatObjectId string, messageId string, messageText string) {
	accountId := s.accountService.AccountID()
	spaceName := s.objectStore.GetSpaceName(spaceId)
	details, err := s.objectStore.SpaceIndex(spaceId).GetDetails(domain.NewParticipantId(spaceId, accountId))
	var senderName string
	if err != nil {
		log.Error("sendPushNotification: failed to get profile name, details are empty", zap.Error(err))
	} else {
		senderName = details.GetString(bundle.RelationKeyName)
	}

	payload := &chatpush.Payload{
		SpaceId:  spaceId,
		SenderId: accountId,
		Type:     chatpush.ChatMessage,
		NewMessagePayload: &chatpush.NewMessagePayload{
			ChatId:     chatObjectId,
			MsgId:      messageId,
			SpaceName:  spaceName,
			SenderName: senderName,
			Text:       messageText,
		},
	}

	jsonPayload, err := json.Marshal(payload)

	if err != nil {
		log.Error("marshal push payload", zap.Error(err))
	}
	err = s.pushService.Notify(s.componentCtx, spaceId, []string{chatpush.ChatsTopicName}, jsonPayload)
	if err != nil {
		log.Error("notify push message", zap.Error(err))
	}
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
