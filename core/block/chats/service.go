package chats

/*
AI generated

Name: Chat API and Cross-Space Tracking
Scope: global

## Responsibility
- Provides RPC API for chat message operations: add, edit, delete, toggle reactions, read/unread
- Tracks all chat objects across spaces via cross-space subscription
- Manages cross-space message preview subscriptions (last message + state per chat)
- Sends push notifications on new messages with mentions routing

## Background Tasks
- monitorMessagePreviews: Processes cross-space subscription events to track chat object additions/removals and update preview subscriptions

## Documentation
Preview subscriptions work at two levels:
1. Cross-space subscription tracks all chatDerived objects across all spaces
2. For each active preview subscription, subscribes to last message of each chat via chatsubscription.Service
When chats are added/removed, preview subscriptions are automatically updated and events broadcasted.
*/

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
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
	"github.com/anyproto/anytype-heart/core/block/detailservice"
	"github.com/anyproto/anytype-heart/core/block/editor/chatobject"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/filegc"
	"github.com/anyproto/anytype-heart/core/session"
	subscriptionservice "github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
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

	Search(ctx context.Context, req *pb.RpcChatSearchRequest) ([]*model.SearchMessageResult, error)

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
	detailsService          detailservice.Service
	fileGC                  filegc.FileGC
	ftSearch                ftsearch.FTSearch

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
	s.detailsService = app.MustComponent[detailservice.Service](a)
	s.fileGC = app.MustComponent[filegc.FileGC](a)
	s.ftSearch = app.MustComponent[ftsearch.FTSearch](a)
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
			spaceId, chatId := rec.GetString(bundle.RelationKeySpaceId), rec.GetString(bundle.RelationKeyId)
			// todo: GO-6824 remove this hack after we do a proper recover of bind collection.
			_ = s.objectStore.BindSpaceId(s.componentCtx, spaceId, chatId)
			s.allChatObjectIds[chatId] = spaceId
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

			// todo: GO-6824 remove this hack after we do a proper recover of bind collection.
			_ = s.objectStore.BindSpaceId(s.componentCtx, spaceId, add.Id)
			s.allChatObjectIds[add.Id] = spaceId

			if len(s.subscriptionIds) == 0 {
				return
			}

			for subId := range s.subscriptionIds {
				err := s.onChatAddedAsync(spaceId, add.Id, subId)
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

func (s *service) onChatAddedAsync(spaceId string, chatObjectId string, subId string) error {
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
		chatName           string
	)

	err := s.chatObjectDo(ctx, chatObjectId, func(sb chatobject.StoreObject) error {
		var err error
		messageId, err = sb.AddMessage(ctx, sessionCtx, message)
		spaceId = sb.SpaceID()
		mentions, _ = message.MentionIdentities(ctx, sb)
		chatName = sb.Details().GetString(bundle.RelationKeyName)
		return err
	})
	if err == nil {
		// Update file attachments' CreatedInContextRef to the message ID
		if len(message.Attachments) > 0 {
			go s.updateAttachmentsContext(spaceId, chatObjectId, messageId, message.Attachments)
		}

		pushErr := s.sendPushNotification(ctx, pushNotificationRequest{
			spaceId:      spaceId,
			chatObjectId: chatObjectId,
			chatName:     chatName,
			messageId:    messageId,
			message:      message,
			mentions:     mentions,
		})
		if pushErr != nil {
			log.Error("sendPushNotification: ", zap.Error(pushErr))
		}
	}
	return messageId, err
}

func (s *service) updateAttachmentsContext(spaceId, chatObjectId, messageId string, attachments []*model.ChatMessageAttachment) {
	// Filter attachments
	var objectIds []string
	for _, attachment := range attachments {
		if attachment.Target != "" {
			objectIds = append(objectIds, attachment.Target)
		}
	}

	if len(objectIds) == 0 {
		return
	}

	var details []domain.Detail
	idx := s.objectStore.SpaceIndex(spaceId)
	if idx == nil {
		return
	}
	// Update CreatedInContextRef for all file attachments
	for _, fileId := range objectIds {
		details = details[:0]
		rec, err := idx.GetDetails(fileId)
		if err != nil {
			continue
		}
		current := rec.GetString(bundle.RelationKeyCreatedInContext)
		if current != chatObjectId {
			continue
		}
		// so we should have CreatedInContext, when creating the file/object in the context of chat
		// now we need to set the actual messageId
		if rec.GetString(bundle.RelationKeyCreatedInContextRef) != "" {
			continue
		}
		details = append(details, domain.Detail{
			Key:   bundle.RelationKeyCreatedInContextRef,
			Value: domain.String(messageId),
		})
		if len(details) == 0 {
			continue
		}
		// Use detail service to update the file object
		if err := s.detailsService.SetDetails(nil, fileId, details); err != nil {
			log.Error("failed to update attachment context",
				zap.String("fileId", fileId),
				zap.String("messageId", messageId),
				zap.Error(err))
		}
	}
}

type pushNotificationRequest struct {
	spaceId      string
	chatObjectId string
	chatName     string
	messageId    string
	message      *chatmodel.Message
	mentions     []string
}

func (s *service) buildPushPayload(req pushNotificationRequest) (*chatpush.Payload, error) {
	accountId := s.accountService.AccountID()
	spaceName := s.objectStore.GetSpaceName(req.spaceId)
	details, err := s.objectStore.SpaceIndex(req.spaceId).GetDetails(domain.NewParticipantId(req.spaceId, accountId))
	var senderName string
	if err != nil {
		log.Warn("buildPushPayload: failed to get profile name, details are empty", zap.Error(err))
	} else {
		senderName = details.GetString(bundle.RelationKeyName)
	}

	attachments, err := s.collectAttachmentPayloads(req.message, req.spaceId)
	if err != nil {
		return nil, fmt.Errorf("collect attachments: %w", err)
	}

	text := applyEmojiMarks(req.message.Message.Text, req.message.Message.Marks)

	return &chatpush.Payload{
		SpaceId:  req.spaceId,
		SenderId: accountId,
		Type:     chatpush.ChatMessage,
		NewMessagePayload: &chatpush.NewMessagePayload{
			ChatId:         req.chatObjectId,
			MsgId:          req.messageId,
			SpaceName:      spaceName,
			ChatName:       req.chatName,
			SenderName:     senderName,
			Text:           textUtil.Truncate(text, 1024, "..."),
			HasAttachments: len(req.message.Attachments) > 0,
			Attachments:    attachments,
		},
	}, nil
}

func (s *service) sendPushNotification(ctx context.Context, req pushNotificationRequest) (err error) {
	payload, err := s.buildPushPayload(req)
	if err != nil {
		return fmt.Errorf("build push payload: %w", err)
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
	topics := make([]string, 0, (len(req.mentions)*2)+2)
	topics = append(topics, chatpush.ChatsTopicName)
	topics = append(topics, chatpush.ChatsTopicName+"/"+pushGroupId(req.chatObjectId))
	for _, mention := range req.mentions {
		topics = append(topics, mention)
		topics = append(topics, chatpush.ChatsTopicName+"/"+pushGroupId(req.chatObjectId)+"/"+mention)
	}
	err = s.pushService.Notify(s.componentCtx, req.spaceId, pushGroupId(req.chatObjectId), topics, jsonPayload)
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
	var (
		spaceId     string
		attachments []*model.ChatMessageAttachment
	)

	// First get the message to extract attachments before deletion
	messages, err := s.GetMessagesByIds(ctx, chatObjectId, []string{messageId})
	if err == nil && len(messages) > 0 && messages[0] != nil {
		attachments = messages[0].Attachments
	}

	err = s.chatObjectDo(ctx, chatObjectId, func(sb chatobject.StoreObject) error {
		spaceId = sb.SpaceID()
		return sb.DeleteMessage(ctx, messageId)
	})

	// If deletion was successful and there were attachments, run file GC
	if err == nil && len(attachments) > 0 {
		// Get file IDs from attachments
		fileIds := make([]string, 0, len(attachments))
		for _, attachment := range attachments {
			// do not filter by attachment type, because of bug on anytype-ts
			// we filter out files by layouts later in CheckFilesOnLinksRemoval
			fileIds = append(fileIds, attachment.Target)
		}

		if len(fileIds) > 0 {
			// Run file GC asynchronously with skipBin=true to permanently delete orphaned files
			// Pass messageId to only delete files created specifically for this message
			go func() {
				if err := s.fileGC.CheckFilesOnLinksRemoval(spaceId, chatObjectId, fileIds, true, []string{messageId}); err != nil {
					log.Error("file GC failed for deleted message",
						zap.String("messageId", messageId),
						zap.String("chatObjectId", chatObjectId),
						zap.Error(err))
				}
			}()
		}
	}

	return err
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

func (s *service) Search(ctx context.Context, req *pb.RpcChatSearchRequest) ([]*model.SearchMessageResult, error) {
	ftResults, err := s.ftSearch.Search(req.SpaceId, req.FullText)
	if err != nil {
		return nil, fmt.Errorf("search ft: %w", err)
	}

	messageIds := make([]string, 0, len(ftResults))
	ftResultsMap := make(map[string]*ftsearch.DocumentMatch, len(ftResults))
	for _, result := range ftResults {
		path, err := domain.NewFromPath(result.ID)
		if err != nil {
			log.Error("failed to parse ft result", zap.Error(err))
			continue
		}

		if path.MessageId == "" || path.ObjectId != req.ChatId {
			continue
		}

		messageIds = append(messageIds, path.MessageId)
		ftResultsMap[path.MessageId] = result
	}

	messages := make([]*chatmodel.Message, 0, len(messageIds))
	if err = s.chatObjectDo(ctx, req.ChatId, func(sb chatobject.StoreObject) error {
		messages, err = sb.GetMessagesByIds(ctx, messageIds)
		return err
	}); err != nil {
		return nil, err
	}

	results := make([]*model.SearchMessageResult, 0, len(messages))
	for _, message := range messages {
		docMatch := ftResultsMap[message.Id]
		ftResult, err := database.FTDocumentMatchToFulltextResult(docMatch)
		if err != nil {
			return nil, err
		}

		result := ftResult.MessageModel()
		result.Message = message.ChatMessage

		results = append(results, &result)
	}

	slices.SortFunc(results, getComparator(req.Sorts))

	if req.Offset > 0 && len(results) >= int(req.Offset) {
		results = results[req.Offset:]
	}

	if req.Limit > 0 && len(results) > int(req.Limit) {
		results = results[:req.Limit]
	}

	return results, nil
}

func getComparator(sorts []*model.SearchMessageSort) func(result *model.SearchMessageResult, result2 *model.SearchMessageResult) int {
	return func(a *model.SearchMessageResult, b *model.SearchMessageResult) (cmp int) {
		for _, sort := range sorts {
			switch sort.Key {
			case model.SearchMessageSort_ORDER_ID:
				cmp = strings.Compare(a.Message.OrderId, b.Message.OrderId)
			case model.SearchMessageSort_SCORE:
				cmp = int(a.Score - b.Score)
			case model.SearchMessageSort_CREATED_AT:
				cmp = int(a.Message.CreatedAt - b.Message.CreatedAt)
			case model.SearchMessageSort_MODIFIED_AT:
				cmp = int(a.Message.ModifiedAt - b.Message.ModifiedAt)
			}
			if sort.Type == model.SearchMessageSort_Desc {
				cmp = -cmp
			}
			if cmp != 0 {
				return
			}
		}
		return 0
	}
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
