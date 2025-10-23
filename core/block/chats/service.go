package chats

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/samber/lo"

	"github.com/cheggaaa/mb/v3"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/chats/chatpush"
	"github.com/anyproto/anytype-heart/core/block/chats/chatrepository"
	"github.com/anyproto/anytype-heart/core/block/chats/chatsubscription"
	"github.com/anyproto/anytype-heart/core/block/editor/chatobject"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	subscriptionservice "github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	textUtil "github.com/anyproto/anytype-heart/util/text"
)

const CName = "core.block.chats"

var log = logging.Logger(CName).Desugar()

type Service interface {
	AddMessage(ctx context.Context, sessionCtx session.Context, chatObjectId string, message *chatmodel.Message) (string, error)
	EditMessage(ctx context.Context, chatObjectId string, messageId string, newMessage *chatmodel.Message) error
	ToggleMessageReaction(ctx context.Context, chatObjectId string, messageId string, emoji string) (bool, error)
	DeleteMessage(ctx context.Context, chatObjectId string, messageId string) error
	GetMessages(ctx context.Context, chatObjectId string, req chatrepository.GetMessagesRequest) (*chatobject.GetMessagesResponse, error)
	GetMessagesByIds(ctx context.Context, chatObjectId string, messageIds []string) ([]*chatmodel.Message, error)
	SubscribeLastMessages(ctx context.Context, chatObjectId string, limit int, subId string) (*chatsubscription.SubscribeLastMessagesResponse, error)
	ReadMessages(ctx context.Context, req ReadMessagesRequest) error
	UnreadMessages(ctx context.Context, chatObjectId string, afterOrderId string, counterType chatmodel.CounterType) error
	Unsubscribe(chatObjectId string, subId string) error

	SubscribeToMessagePreviews(ctx context.Context, subId string) (*SubscribeToMessagePreviewsResponse, error)
	UnsubscribeFromMessagePreviews(subId string) error

	ReadAll(ctx context.Context) error

	app.ComponentRunnable
}

var _ Service = (*service)(nil)

type pushService interface {
	Notify(ctx context.Context, spaceId, groupId string, topic []string, payload []byte) (err error)
	NotifyRead(ctx context.Context, spaceId, groupId string) (err error)
}

type accountService interface {
	AccountID() string
}

type service struct {
	spaceIdResolver         idresolver.Resolver
	objectGetter            cache.ObjectWaitGetter
	crossSpaceSubService    crossspacesub.Service
	pushService             pushService
	accountService          accountService
	objectStore             objectstore.ObjectStore
	chatSubscriptionService chatsubscription.Service
	eventSender             event.Sender

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
	s.crossSpaceSubService = app.MustComponent[crossspacesub.Service](a)
	s.componentCtx, s.componentCtxCancel = context.WithCancel(context.Background())
	s.pushService = app.MustComponent[pushService](a)
	s.accountService = app.MustComponent[accountService](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.objectGetter = app.MustComponent[cache.ObjectWaitGetter](a)
	s.chatSubscriptionService = app.MustComponent[chatsubscription.Service](a)
	s.spaceIdResolver = app.MustComponent[idresolver.Resolver](a)
	s.eventSender = app.MustComponent[event.Sender](a)
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
	Message      *chatmodel.Message
	Dependencies []*domain.Details
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

	lock := &sync.Mutex{}
	result := &SubscribeToMessagePreviewsResponse{
		Previews: make([]*ChatPreview, 0, len(s.allChatObjectIds)),
	}
	var wg sync.WaitGroup
	for chatObjectId, spaceId := range s.allChatObjectIds {
		wg.Add(1)
		go func() {
			defer wg.Done()

			chatAddResp, err := s.onChatAdded(chatObjectId, subId)
			if err != nil {
				log.Error("init lastMessage subscription", zap.Error(err))
				return
			}
			var (
				message      *chatmodel.Message
				dependencies []*domain.Details
			)
			if len(chatAddResp.Messages) > 0 {
				message = chatAddResp.Messages[0]
				dependencies = chatAddResp.Dependencies[message.Id]
			}

			lock.Lock()
			defer lock.Unlock()
			result.Previews = append(result.Previews, &ChatPreview{
				SpaceId:      spaceId,
				ChatObjectId: chatObjectId,
				State:        chatAddResp.ChatState,
				Message:      message,
				Dependencies: dependencies,
			})
		}()
	}
	wg.Wait()

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
	s.lock.Lock()
	go func() {
		defer s.lock.Unlock()
		resp, err := s.crossSpaceSubService.Subscribe(subscriptionservice.SubscribeRequest{
			SubId:             allChatsSubscriptionId,
			InternalQueue:     s.chatObjectsSubQueue,
			Keys:              []string{bundle.RelationKeyId.String(), bundle.RelationKeySpaceId.String()},
			NoDepSubscription: true,
			Filters: []database.FilterRequest{
				{
					RelationKey: bundle.RelationKeyResolvedLayout,
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       domain.Int64(model.ObjectType_chatDerived),
				},
			},
		}, crossspacesub.NoOpPredicate())
		if err != nil {
			log.Error("cross-space sub", zap.Error(err))
			return
		}

		for _, rec := range resp.Records {
			s.allChatObjectIds[rec.GetString(bundle.RelationKeyId)] = rec.GetString(bundle.RelationKeySpaceId)
		}
		go s.monitorMessagePreviews()
	}()

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
				err := s.onChatAddedAsync(add.Id, subId)
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

func (s *service) onChatAdded(chatObjectId string, subId string) (*chatsubscription.SubscribeLastMessagesResponse, error) {
	return s.chatSubscriptionService.SubscribeLastMessages(s.componentCtx, chatsubscription.SubscribeLastMessagesRequest{
		ChatObjectId:     chatObjectId,
		SubId:            subId,
		Limit:            1,
		WithDependencies: true,
		OnlyLastMessage:  true,
	})
}

func (s *service) onChatAddedAsync(chatObjectId string, subId string) error {
	resp, err := s.chatSubscriptionService.SubscribeLastMessages(s.componentCtx, chatsubscription.SubscribeLastMessagesRequest{
		ChatObjectId:     chatObjectId,
		SubId:            subId,
		Limit:            1,
		WithDependencies: true,
		OnlyLastMessage:  true,
	})
	if err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}

	spaceId, err := s.spaceIdResolver.ResolveSpaceID(chatObjectId)
	if err != nil {
		return fmt.Errorf("resolve space id: %w", err)
	}

	mngr, err := s.chatSubscriptionService.GetManager(spaceId, chatObjectId)
	if err != nil {
		return fmt.Errorf("get manager: %w", err)
	}
	mngr.Lock()
	defer mngr.Unlock()

	events := make([]*pb.EventMessage, 0, 2)
	if len(resp.Messages) > 0 {
		msg := resp.Messages[0]
		events = append(events, event.NewMessage(spaceId, &pb.EventMessageValueOfChatAdd{
			ChatAdd: &pb.EventChatAdd{
				Id:           msg.Id,
				OrderId:      msg.OrderId,
				AfterOrderId: resp.PreviousOrderId,
				Message:      msg.ChatMessage,
				SubIds:       []string{subId},
			},
		}))
	}
	events = append(events, event.NewMessage(spaceId, &pb.EventMessageValueOfChatStateUpdate{ChatStateUpdate: &pb.EventChatUpdateState{
		State:  mngr.GetChatState(),
		SubIds: []string{subId},
	}}))
	s.eventSender.Broadcast(&pb.Event{
		Messages:  events,
		ContextId: chatObjectId,
	})

	return nil
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

func (s *service) AddMessage(ctx context.Context, sessionCtx session.Context, chatObjectId string, message *chatmodel.Message) (string, error) {
	var (
		messageId, spaceId string
		mentions           []string
	)

	err := s.chatObjectDo(ctx, chatObjectId, func(sb chatobject.StoreObject) error {
		var err error
		messageId, err = sb.AddMessage(ctx, sessionCtx, message)
		spaceId = sb.SpaceID()
		mentions, _ = message.MentionIdentities(ctx, sb)
		return err
	})
	if err == nil {
		pushErr := s.sendPushNotification(ctx, spaceId, chatObjectId, messageId, message, mentions)
		if pushErr != nil {
			log.Error("sendPushNotification: ", zap.Error(pushErr))
		}

	}
	return messageId, err
}

func (s *service) sendPushNotification(ctx context.Context, spaceId, chatObjectId, messageId string, message *chatmodel.Message, mentions []string) (err error) {
	accountId := s.accountService.AccountID()
	spaceName := s.objectStore.GetSpaceName(spaceId)
	details, err := s.objectStore.SpaceIndex(spaceId).GetDetails(domain.NewParticipantId(spaceId, accountId))
	var senderName string
	if err != nil {
		log.Warn("sendPushNotification: failed to get profile name, details are empty", zap.Error(err))
	} else {
		senderName = details.GetString(bundle.RelationKeyName)
	}

	attachments, err := s.collectAttachmentPayloads(message, spaceId)
	if err != nil {
		return fmt.Errorf("collect attachments: %w", err)
	}

	text := applyEmojiMarks(message.Message.Text, message.Message.Marks)

	payload := &chatpush.Payload{
		SpaceId:  spaceId,
		SenderId: accountId,
		Type:     chatpush.ChatMessage,
		NewMessagePayload: &chatpush.NewMessagePayload{
			ChatId:         chatObjectId,
			MsgId:          messageId,
			SpaceName:      spaceName,
			SenderName:     senderName,
			Text:           textUtil.Truncate(text, 1024, "..."),
			HasAttachments: len(message.Attachments) > 0,
			Attachments:    attachments,
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		err = fmt.Errorf("marshal push payload: %w", err)
		return
	}

	// Expected topics:
	// 1. chats
	// 2. chats/sha256(<chatObjectId>)
	// 3. chats/sha256(<chatObjectId>)/<mentionIdentity>
	// 4. <mentionIdentity>
	topics := make([]string, 0, (len(mentions)*2)+2)
	topics = append(topics, chatpush.ChatsTopicName)
	topics = append(topics, chatpush.ChatsTopicName+"/"+pushGroupId(chatObjectId))
	for _, mention := range mentions {
		topics = append(topics, mention)
		topics = append(topics, chatpush.ChatsTopicName+"/"+pushGroupId(chatObjectId)+"/"+mention)
	}
	err = s.pushService.Notify(s.componentCtx, spaceId, pushGroupId(chatObjectId), topics, jsonPayload)
	if err != nil {
		err = fmt.Errorf("pushService.Notify: %w", err)
		return
	}

	return
}

func (s *service) collectAttachmentPayloads(message *chatmodel.Message, spaceId string) ([]*chatpush.Attachment, error) {
	if len(message.Attachments) > 0 {
		attachmentIds := make([]string, 0, len(message.Attachments))
		for _, attachment := range message.Attachments {
			attachmentIds = append(attachmentIds, attachment.Target)
		}

		attachmentDetails, err := s.objectStore.SpaceIndex(spaceId).QueryByIds(attachmentIds)
		if err != nil {
			return nil, fmt.Errorf("query attachments: %w", err)
		}
		attachments := make([]*chatpush.Attachment, 0, len(message.Attachments))
		for _, att := range attachmentDetails {
			attachments = append(attachments, &chatpush.Attachment{
				Layout: int(att.Details.GetInt64(bundle.RelationKeyResolvedLayout)),
			})
		}
		return attachments, nil
	}
	return nil, nil
}

func applyEmojiMarks(text string, marks []*model.BlockContentTextMark) string {
	utf16text := textUtil.StrToUTF16(text)
	res := make([]uint16, 0, len(text))

	toApply := lo.Filter(marks, func(mark *model.BlockContentTextMark, _ int) bool {
		return mark.Type == model.BlockContentTextMark_Emoji
	})
	sort.Slice(toApply, func(i, j int) bool {
		return toApply[i].Range.From < toApply[j].Range.From
	})
	var prev int
	var lastTo int
	for _, mark := range toApply {
		if mark.Range.From >= mark.Range.To {
			continue
		}
		if int(mark.Range.From) >= len(utf16text) {
			continue
		}
		res = append(res, utf16text[prev:mark.Range.From]...)
		res = append(res, textUtil.StrToUTF16(mark.Param)...)
		prev = int(mark.Range.To)
		lastTo = int(mark.Range.To)
	}
	if lastTo < len(text) {
		res = append(res, utf16text[lastTo:]...)
	}
	return textUtil.UTF16ToStr(res)
}

func (s *service) EditMessage(ctx context.Context, chatObjectId string, messageId string, newMessage *chatmodel.Message) error {
	return s.chatObjectDo(ctx, chatObjectId, func(sb chatobject.StoreObject) error {
		return sb.EditMessage(ctx, messageId, newMessage)
	})
}

func (s *service) ToggleMessageReaction(ctx context.Context, chatObjectId string, messageId string, emoji string) (bool, error) {
	var added bool
	err := s.chatObjectDo(ctx, chatObjectId, func(sb chatobject.StoreObject) error {
		var err error
		added, err = sb.ToggleMessageReaction(ctx, messageId, emoji)
		return err
	})
	return added, err
}

func (s *service) DeleteMessage(ctx context.Context, chatObjectId string, messageId string) error {
	return s.chatObjectDo(ctx, chatObjectId, func(sb chatobject.StoreObject) error {
		return sb.DeleteMessage(ctx, messageId)
	})
}

func (s *service) GetMessages(ctx context.Context, chatObjectId string, req chatrepository.GetMessagesRequest) (*chatobject.GetMessagesResponse, error) {
	var resp *chatobject.GetMessagesResponse
	err := s.chatObjectDo(ctx, chatObjectId, func(sb chatobject.StoreObject) error {
		var err error
		resp, err = sb.GetMessages(ctx, req)
		if err != nil {
			return err
		}
		return nil
	})
	return resp, err
}

func (s *service) GetMessagesByIds(ctx context.Context, chatObjectId string, messageIds []string) ([]*chatmodel.Message, error) {
	var res []*chatmodel.Message
	err := s.chatObjectDo(ctx, chatObjectId, func(sb chatobject.StoreObject) error {
		msg, err := sb.GetMessagesByIds(ctx, messageIds)
		if err != nil {
			return err
		}
		res = msg
		return nil
	})
	return res, err
}

func (s *service) SubscribeLastMessages(ctx context.Context, chatObjectId string, limit int, subId string) (*chatsubscription.SubscribeLastMessagesResponse, error) {
	return s.chatSubscriptionService.SubscribeLastMessages(s.componentCtx, chatsubscription.SubscribeLastMessagesRequest{
		ChatObjectId:           chatObjectId,
		SubId:                  subId,
		Limit:                  limit,
		WithDependencies:       false,
		CouldUseSessionContext: true,
	})
}

func (s *service) Unsubscribe(chatObjectId string, subId string) error {
	return s.chatSubscriptionService.Unsubscribe(chatObjectId, subId)
}

type ReadMessagesRequest struct {
	ChatObjectId  string
	AfterOrderId  string
	BeforeOrderId string
	LastStateId   string
	CounterType   chatmodel.CounterType
}

func (s *service) ReadMessages(ctx context.Context, req ReadMessagesRequest) error {
	return s.chatObjectDo(ctx, req.ChatObjectId, func(sb chatobject.StoreObject) error {
		markedCount, err := sb.MarkReadMessages(ctx, chatobject.ReadMessagesRequest{
			AfterOrderId:  req.AfterOrderId,
			BeforeOrderId: req.BeforeOrderId,
			LastStateId:   req.LastStateId,
			CounterType:   req.CounterType,
		})
		if err != nil {
			return err
		}
		if markedCount > 0 {
			if nErr := s.pushService.NotifyRead(ctx, sb.SpaceID(), pushGroupId(req.ChatObjectId)); nErr != nil {
				log.Error("notifyRead", zap.Error(nErr))
			}
		}
		return nil
	})
}

func (s *service) UnreadMessages(ctx context.Context, chatObjectId string, afterOrderId string, counterType chatmodel.CounterType) error {
	return s.chatObjectDo(ctx, chatObjectId, func(sb chatobject.StoreObject) error {
		return sb.MarkMessagesAsUnread(ctx, afterOrderId, counterType)
	})
}

func (s *service) chatObjectDo(ctx context.Context, chatObjectId string, proc func(sb chatobject.StoreObject) error) error {
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return cache.DoWait(s.objectGetter, waitCtx, chatObjectId, proc)
}

func (s *service) ReadAll(ctx context.Context) error {
	s.lock.Lock()
	chatIds := make([]string, 0, len(s.allChatObjectIds))
	for id := range s.allChatObjectIds {
		chatIds = append(chatIds, id)
	}
	s.lock.Unlock()

	for _, chatId := range chatIds {
		err := s.chatObjectDo(ctx, chatId, func(sb chatobject.StoreObject) error {
			markedMessages, err := sb.MarkReadMessages(ctx, chatobject.ReadMessagesRequest{
				All:         true,
				CounterType: chatmodel.CounterTypeMessage,
			})
			if err != nil {
				return fmt.Errorf("messages: %w", err)
			}
			markedMentions, err := sb.MarkReadMessages(ctx, chatobject.ReadMessagesRequest{
				All:         true,
				CounterType: chatmodel.CounterTypeMention,
			})
			if err != nil {
				return fmt.Errorf("mentions: %w", err)
			}
			if markedMessages+markedMentions > 0 {
				if nErr := s.pushService.NotifyRead(ctx, sb.SpaceID(), pushGroupId(chatId)); nErr != nil {
					log.Error("notifyRead", zap.Error(nErr))
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
	}

	return nil
}

func pushGroupId(objectId string) string {
	hash := sha256.Sum256([]byte(objectId))
	return hex.EncodeToString(hash[:])
}
