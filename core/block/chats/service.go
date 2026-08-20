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
	"cmp"
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
	"github.com/anyproto/anytype-heart/core/block/objectgc"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	subscriptionservice "github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
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

	ReadReaction(ctx context.Context, chatObjectId string) error
	ReadAll(ctx context.Context) error

	Search(ctx context.Context, req *pb.RpcChatSearchRequest) ([]*model.SearchMessageResult, error)

	PinMessages(ctx context.Context, chatObjectId string, messageIds []string, pinned bool) error
	GetPinnedMessages(ctx context.Context, chatObjectId string) ([]*chatmodel.Message, error)

	AddNotificationSubscriber(ctx context.Context, chatObjectId string, identity string) error
	RemoveNotificationSubscriber(ctx context.Context, chatObjectId string, identity string) error
	GetNotificationSubscribers(ctx context.Context, chatObjectId string) ([]string, error)

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
	objectGC                objectgc.ObjectGC
	ftSearch                ftsearch.FTSearch
	chatRepoService         chatrepository.Service

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
	s.objectGC = app.MustComponent[objectgc.ObjectGC](a)
	s.ftSearch = app.MustComponent[ftsearch.FTSearch](a)
	s.chatRepoService = app.MustComponent[chatrepository.Service](a)
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
	resp, err := s.onChatAdded(chatObjectId, subId)
	if err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}

	mngr, err := s.chatSubscriptionService.GetManager(spaceId, chatObjectId)
	if err != nil {
		return fmt.Errorf("get manager: %w", err)
	}
	mngr.Lock()
	chatState := mngr.GetChatState()
	mngr.Unlock()

	events := make([]*pb.EventMessage, 0, 2)
	if len(resp.Messages) > 0 {
		events = append(events, newChatAddEvent(spaceId, subId, resp))
	}
	// If the chat's message store is not loaded yet there is no last message to
	// preview right now — and that is fine, no polling needed. onChatAdded above
	// registered subId on the chat's subscription manager, so when the chat
	// object finishes loading it calls Flush() and the manager reactively emits
	// the last message (with dependencies) to subId. See
	// chatobject.onInit -> subscription.Flush and subscriptionManager.Flush.
	events = append(events, event.NewMessage(spaceId, &pb.EventMessageValueOfChatStateUpdate{ChatStateUpdate: &pb.EventChatUpdateState{
		State:  chatState,
		SubIds: []string{subId},
	}}))
	s.eventSender.Broadcast(&pb.Event{
		Messages:  events,
		ContextId: chatObjectId,
	})

	return nil
}

// newChatAddEvent builds the ChatAdd preview event for a chat backfilled into an
// existing previews subscription, carrying the message dependencies
// (creator/attachments) so it is as complete as the chats returned by
// SubscribeToMessagePreviews itself and the reactive Flush path.
func newChatAddEvent(spaceId string, subId string, resp *chatsubscription.SubscribeLastMessagesResponse) *pb.EventMessage {
	msg := resp.Messages[0]
	return event.NewMessage(spaceId, &pb.EventMessageValueOfChatAdd{
		ChatAdd: &pb.EventChatAdd{
			Id:           msg.Id,
			OrderId:      msg.OrderId,
			AfterOrderId: resp.PreviousOrderId,
			Message:      msg.ChatMessage,
			SubIds:       []string{subId},
			Dependencies: domain.DetailsListToProtos(resp.Dependencies[msg.Id]),
		},
	})
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
		if sb.Type() == smartblock.SmartBlockTypeDiscussionObject {
			if parentId := sb.Tree().Root().ParentId; parentId != "" {
				parentDetails, detailsErr := s.objectStore.SpaceIndex(sb.SpaceID()).GetDetails(parentId)
				if detailsErr == nil {
					chatName = parentDetails.GetString(bundle.RelationKeyName)
				}
			}
		}
		return err
	})
	if err == nil {
		// Update file attachments' CreatedInContextRef to the message ID
		linkTargets := message.LinkBlockTargetIds()
		if len(message.Attachments) > 0 || len(linkTargets) > 0 {
			go s.updateAttachmentsContext(spaceId, chatObjectId, messageId, message.Attachments, linkTargets)
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

func (s *service) updateAttachmentsContext(spaceId, chatObjectId, messageId string, attachments []*model.ChatMessageAttachment, linkTargets []string) {
	var objectIds []string
	for _, attachment := range attachments {
		if attachment.Target != "" {
			objectIds = append(objectIds, attachment.Target)
		}
	}
	objectIds = append(objectIds, linkTargets...)

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
	spaceViewDetails, err := s.objectStore.GetSpaceViewDetails(req.spaceId)
	if err != nil {
		log.Warn("buildPushPayload: failed to get space view details", zap.Error(err))
		spaceViewDetails = domain.NewDetails()
	}
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
	if blocksText := req.message.BlocksText(); blocksText != "" {
		if text != "" {
			text += "\n"
		}
		text += blocksText
	}

	hasAttachments := len(req.message.Attachments) > 0 || len(req.message.LinkBlockTargetIds()) > 0

	return &chatpush.Payload{
		SpaceId:     req.spaceId,
		SpaceUxType: int(spaceViewDetails.GetInt64(bundle.RelationKeySpaceUxType)), // TODO: GO-7102 remove
		SpaceType:   int(spaceViewDetails.GetInt64(bundle.RelationKeySpaceType)),
		SenderId:    accountId,
		Type:        chatpush.ChatMessage,
		NewMessagePayload: &chatpush.NewMessagePayload{
			ChatId:         req.chatObjectId,
			MsgId:          req.messageId,
			SpaceName:      spaceViewDetails.GetString(bundle.RelationKeyName),
			ChatName:       req.chatName,
			SenderName:     senderName,
			Text:           textUtil.Truncate(text, 1024, "..."),
			HasAttachments: hasAttachments,
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
	attachmentIds := make([]string, 0, len(message.Attachments))
	for _, attachment := range message.Attachments {
		attachmentIds = append(attachmentIds, attachment.Target)
	}
	attachmentIds = append(attachmentIds, message.LinkBlockTargetIds()...)

	if len(attachmentIds) == 0 {
		return nil, nil
	}

	attachmentDetails, err := s.objectStore.SpaceIndex(spaceId).QueryByIds(attachmentIds)
	if err != nil {
		return nil, fmt.Errorf("query attachments: %w", err)
	}
	attachments := make([]*chatpush.Attachment, 0, len(attachmentIds))
	for _, att := range attachmentDetails {
		attachments = append(attachments, &chatpush.Attachment{
			Layout: int(att.Details.GetInt64(bundle.RelationKeyResolvedLayout)),
		})
	}
	return attachments, nil
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
		spaceId       string
		attachments   []*model.ChatMessageAttachment
		linkTargetIds []string
	)

	// First get the message to extract attachments before deletion
	messages, err := s.GetMessagesByIds(ctx, chatObjectId, []string{messageId})
	if err == nil && len(messages) > 0 && messages[0] != nil {
		attachments = messages[0].Attachments
		linkTargetIds = messages[0].LinkBlockTargetIds()
	}

	err = s.chatObjectDo(ctx, chatObjectId, func(sb chatobject.StoreObject) error {
		spaceId = sb.SpaceID()
		return sb.DeleteMessage(ctx, messageId)
	})

	// If deletion was successful and there were attachments, run file GC
	if err == nil && (len(attachments) > 0 || len(linkTargetIds) > 0) {
		// Get file IDs from attachments and link blocks
		fileIds := make([]string, 0, len(attachments)+len(linkTargetIds))
		for _, attachment := range attachments {
			// do not filter by attachment type, because of bug on anytype-ts
			// we filter out files by layouts later in ArchiveOrphansOnLinksRemoval
			fileIds = append(fileIds, attachment.Target)
		}
		fileIds = append(fileIds, linkTargetIds...)

		if len(fileIds) > 0 {
			// Run file GC asynchronously with skipBin=true to permanently delete orphaned files
			// Pass messageId to only delete files created specifically for this message
			go func() {
				// Only orphaned *files* are handled here (archived/deleted internally). Any non-file
				// orphan objects come back in the returned OrphanCandidates but are intentionally
				// dropped on this path: chat attachments are files, and there is no session context
				// here to surface a CleanupSuggestion confirmation event to the user.
				if _, err := s.objectGC.ArchiveOrphansOnLinksRemoval(spaceId, chatObjectId, fileIds, true, []string{messageId}); err != nil {
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

func (s *service) AddNotificationSubscriber(ctx context.Context, chatObjectId string, identity string) error {
	return s.chatObjectDo(ctx, chatObjectId, func(sb chatobject.StoreObject) error {
		return sb.AddNotificationSubscriber(ctx, identity)
	})
}

func (s *service) RemoveNotificationSubscriber(ctx context.Context, chatObjectId string, identity string) error {
	return s.chatObjectDo(ctx, chatObjectId, func(sb chatobject.StoreObject) error {
		return sb.RemoveNotificationSubscriber(ctx, identity)
	})
}

func (s *service) GetNotificationSubscribers(ctx context.Context, chatObjectId string) ([]string, error) {
	var res []string
	err := s.chatObjectDo(ctx, chatObjectId, func(sb chatobject.StoreObject) error {
		var e error
		res, e = sb.GetNotificationSubscribers(ctx)
		return e
	})
	return res, err
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
	repo, err := s.chatRepository(chatObjectId)
	if err != nil {
		return nil, err
	}
	return repo.GetMessagesByIds(ctx, messageIds)
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

func (s *service) ReadReaction(ctx context.Context, chatObjectId string) error {
	return s.chatObjectDo(ctx, chatObjectId, func(sb chatobject.StoreObject) error {
		return sb.MarkReadReactions(ctx)
	})
}

// Search performs a fulltext search over chat messages. The scope narrows
// with the request: chatId set = that chat only; chatId empty = all chats in
// spaceId; both empty = all chats in all spaces (the fulltext index is a
// single cross-space index). See docs/fts/SpecChatSearchScopes.md.
func (s *service) Search(ctx context.Context, req *pb.RpcChatSearchRequest) ([]*model.SearchMessageResult, error) {
	if req.FullText == "" {
		// no query = browse: the latest messages in scope, straight from the
		// message stores (authoritative — no FT indexing lag), default sorted
		// by CREATED_AT desc. This is the search screen's entry state.
		return s.browseLastMessages(ctx, req)
	}
	// candidate budget: Limit == 0 falls back to the ftsearch default (one
	// 100-doc page); otherwise request at least the default page so sorting by
	// keys other than score stays consistent across shallow pages. All chats in
	// scope share this single budget. Known limitation: once offset+limit
	// crosses the shared candidate budget, pages sorted by non-score keys are
	// computed over different BM25-truncated sets and may overlap — deep
	// chat-search pagination needs a cursor to be exact.
	ftLimit := 0
	if req.Limit > 0 {
		ftLimit = int(req.Offset) + int(req.Limit)
		if ftLimit < 100 {
			ftLimit = 100
		}
		// same candidate ceiling as object search (ftCandidatesHardLimit):
		// the limit sizes tantivy's per-segment top-K heap
		if ftLimit > 2000 {
			ftLimit = 2000
		}
	}
	// the search is scoped to message docs so messages don't compete with the
	// rest of the space for the candidate limit
	ftResults, err := s.ftSearch.SearchChat(req.SpaceId, req.ChatId, req.FullText, req.Creators, ftLimit)
	if err != nil {
		return nil, fmt.Errorf("search ft: %w", err)
	}

	// the FT clause already scopes candidates to the requested authors; this
	// set backstops it at hydration (stale FT docs after an edit)
	var creatorSet map[string]struct{}
	if len(req.Creators) > 0 {
		creatorSet = make(map[string]struct{}, len(req.Creators))
		for _, creator := range req.Creators {
			creatorSet[creator] = struct{}{}
		}
	}

	// group message ids per chat, preserving the FT (score) order within groups.
	// The chat's space comes from the request when given, otherwise from the
	// hit's stored space field, so results are attributable in every scope.
	var chatIds []string
	messageIdsPerChat := make(map[string][]string)
	chatSpaceIds := make(map[string]string)
	ftResultsMap := make(map[string]*ftsearch.DocumentMatch, len(ftResults))
	for _, result := range ftResults {
		path, err := domain.NewFromPath(result.ID)
		if err != nil {
			log.Error("failed to parse ft result", zap.Error(err))
			continue
		}

		// HasMessage also guards the false positives of the message-doc marker
		// query (see ftsearch.SearchChat)
		if !path.HasMessage() {
			continue
		}
		if req.ChatId != "" && path.ObjectId != req.ChatId {
			continue
		}

		if _, ok := messageIdsPerChat[path.ObjectId]; !ok {
			chatIds = append(chatIds, path.ObjectId)
			spaceId := req.SpaceId
			if spaceId == "" {
				spaceId = result.SpaceId
			}
			chatSpaceIds[path.ObjectId] = spaceId
		}
		messageIdsPerChat[path.ObjectId] = append(messageIdsPerChat[path.ObjectId], path.MessageId)
		ftResultsMap[path.MessageId] = result
	}

	results := make([]*model.SearchMessageResult, 0, len(ftResultsMap))
	// keep the original float BM25 scores for sorting: the proto Score field is
	// an integer and loses sub-integer differences
	scores := make(map[string]float64, len(ftResultsMap))
	for _, chatId := range chatIds {
		spaceId := chatSpaceIds[chatId]
		// multi-chat scopes hydrate only chats the object index knows as live:
		// stale FT docs of deleted chats are skipped and the repository
		// accessor's create-collection side-effect is never triggered for them.
		// In the single-chat scope the caller named the chat, so it keeps the
		// permissive behavior of GetMessagesByIds.
		if req.ChatId == "" && !s.isLiveChat(spaceId, chatId) {
			continue
		}
		// hydrate from the anystore-backed repository (see chatRepository for
		// the consistency caveat): search must not pay the change-tree build
		// for every chat in scope. Messages the FT index still lists but the
		// store no longer has are skipped.
		repo, err := s.chatRepoService.Repository(spaceId, chatId)
		if err != nil {
			// a chat must not abort the whole multi-chat search: a space can
			// be mid-deletion or its store transiently unavailable
			if req.ChatId == "" {
				log.Error("chat search: get repository", zap.String("chatId", chatId), zap.Error(err))
				continue
			}
			return nil, fmt.Errorf("chat repository: %w", err)
		}
		messages, err := repo.GetMessagesByIds(ctx, messageIdsPerChat[chatId])
		if err != nil {
			if req.ChatId == "" {
				log.Error("chat search: get messages", zap.String("chatId", chatId), zap.Error(err))
				continue
			}
			return nil, fmt.Errorf("get messages: %w", err)
		}

		for _, message := range messages {
			docMatch := ftResultsMap[message.Id]
			if docMatch == nil {
				continue
			}
			if creatorSet != nil {
				if _, ok := creatorSet[message.Creator]; !ok {
					continue
				}
			}
			ftResult, err := database.FTDocumentMatchToFulltextResult(docMatch)
			if err != nil {
				return nil, fmt.Errorf("convert ft result: %w", err)
			}

			result := ftResult.MessageModel()
			result.Message = message.ChatMessage
			result.SpaceId = spaceId
			scores[result.MessageId] = ftResult.Score

			results = append(results, &result)
		}
	}

	sorts := req.Sorts
	if len(sorts) == 0 {
		// explicit default: hydration returns hits grouped per chat and the
		// sort below is unstable, so without a key the grouped order would
		// leak into the response
		sorts = []*model.SearchMessageSort{{Key: model.SearchMessageSort_SCORE, Type: model.SearchMessageSort_Desc}}
	}
	slices.SortFunc(results, getComparator(sorts, scores))

	if req.Offset > 0 {
		if int(req.Offset) >= len(results) {
			return nil, nil
		}
		results = results[req.Offset:]
	}

	if req.Limit > 0 && len(results) > int(req.Limit) {
		results = results[:req.Limit]
	}

	return results, nil
}

type scopedChat struct {
	chatId  string
	spaceId string
}

// chatsInScope enumerates the chats the request addresses: the named chat, or
// every live chat (space chats and object chats) of the space / all spaces,
// straight from the object index.
func (s *service) chatsInScope(ctx context.Context, req *pb.RpcChatSearchRequest) ([]scopedChat, error) {
	if req.ChatId != "" {
		spaceId := req.SpaceId
		if spaceId == "" {
			var err error
			spaceId, err = s.spaceIdResolver.ResolveSpaceID(req.ChatId)
			if err != nil {
				// unknown chat: same empty result the FT path produces
				log.Warn("chat browse: resolve space id", zap.String("chatId", req.ChatId), zap.Error(err))
				return nil, nil
			}
		}
		return []scopedChat{{chatId: req.ChatId, spaceId: spaceId}}, nil
	}

	query := database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List([]model.ObjectTypeLayout{model.ObjectType_chatDerived, model.ObjectType_discussion}),
			},
			{
				RelationKey: bundle.RelationKeyIsDeleted,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Bool(true),
			},
		},
	}
	var (
		records []database.Record
		err     error
	)
	if req.SpaceId != "" {
		records, err = s.objectStore.SpaceIndex(req.SpaceId).Query(query)
	} else {
		records, err = s.objectStore.QueryCrossSpace(ctx, query)
	}
	if err != nil {
		return nil, fmt.Errorf("query chats: %w", err)
	}
	chats := make([]scopedChat, 0, len(records))
	for _, rec := range records {
		spaceId := req.SpaceId
		if spaceId == "" {
			spaceId = rec.Details.GetString(bundle.RelationKeySpaceId)
		}
		chats = append(chats, scopedChat{
			chatId:  rec.Details.GetString(bundle.RelationKeyId),
			spaceId: spaceId,
		})
	}
	return chats, nil
}

// browseLastMessages serves the empty-query search: the newest messages of
// every chat in scope, merged. Candidates are each chat's latest offset+limit
// messages by orderId (the chat's canonical order, which tracks creation time
// closely — a late-arriving concurrent message can fall just outside the
// candidate window; accepted approximation), then the requested sorts order
// the merged set.
func (s *service) browseLastMessages(ctx context.Context, req *pb.RpcChatSearchRequest) ([]*model.SearchMessageResult, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		// parity with the FT default page
		limit = 100
	}
	candidatesPerChat := int(req.Offset) + limit
	if candidatesPerChat > 2000 {
		candidatesPerChat = 2000
	}

	chats, err := s.chatsInScope(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("chats in scope: %w", err)
	}

	var results []*model.SearchMessageResult
	for _, chat := range chats {
		repo, err := s.chatRepoService.Repository(chat.spaceId, chat.chatId)
		if err != nil {
			if req.ChatId == "" {
				log.Error("chat browse: get repository", zap.String("chatId", chat.chatId), zap.Error(err))
				continue
			}
			return nil, fmt.Errorf("chat repository: %w", err)
		}
		var messages []*chatmodel.Message
		if len(req.Creators) > 0 {
			messages, err = repo.GetLastMessagesByCreators(ctx, req.Creators, uint(candidatesPerChat))
		} else {
			messages, err = repo.GetLastMessages(ctx, uint(candidatesPerChat))
		}
		if err != nil {
			if req.ChatId == "" {
				log.Error("chat browse: get last messages", zap.String("chatId", chat.chatId), zap.Error(err))
				continue
			}
			return nil, fmt.Errorf("get last messages: %w", err)
		}
		for _, message := range messages {
			results = append(results, &model.SearchMessageResult{
				ChatId:    chat.chatId,
				SpaceId:   chat.spaceId,
				MessageId: message.Id,
				Message:   message.ChatMessage,
			})
		}
	}

	sorts := req.Sorts
	if len(sorts) == 0 {
		// scores don't exist without a query; newest-first is the natural
		// browse order
		sorts = []*model.SearchMessageSort{{Key: model.SearchMessageSort_CREATED_AT, Type: model.SearchMessageSort_Desc}}
	}
	slices.SortFunc(results, getComparator(sorts, nil))

	if req.Offset > 0 {
		if int(req.Offset) >= len(results) {
			return nil, nil
		}
		results = results[req.Offset:]
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// isLiveChat reports whether chatObjectId is a live chat object in spaceId —
// a space chat (chatDerived) or an object chat (discussion), both of which
// have their messages fulltext-indexed. The per-space object index is the
// source of truth: unlike the preview registry (allChatObjectIds, chatDerived
// only, filled asynchronously by the cross-space subscription) it covers
// object chats, needs no lock shared with the subscription bootstrap, and is
// current the moment the space index is readable. Missing and deleted objects
// fail the layout check (GetDetails returns empty details for unknown ids).
func (s *service) isLiveChat(spaceId, chatObjectId string) bool {
	details, err := s.objectStore.SpaceIndex(spaceId).GetDetails(chatObjectId)
	if err != nil {
		log.Warn("chat search: get chat details", zap.String("chatId", chatObjectId), zap.Error(err))
		return false
	}
	if details.GetBool(bundle.RelationKeyIsDeleted) {
		return false
	}
	layout := details.GetInt64(bundle.RelationKeyResolvedLayout)
	return layout == int64(model.ObjectType_chatDerived) || layout == int64(model.ObjectType_discussion)
}

func getComparator(sorts []*model.SearchMessageSort, scores map[string]float64) func(result *model.SearchMessageResult, result2 *model.SearchMessageResult) int {
	return func(a *model.SearchMessageResult, b *model.SearchMessageResult) (res int) {
		for _, sort := range sorts {
			switch sort.Key {
			case model.SearchMessageSort_ORDER_ID:
				res = strings.Compare(a.Message.OrderId, b.Message.OrderId)
			case model.SearchMessageSort_SCORE:
				res = cmp.Compare(scores[a.MessageId], scores[b.MessageId])
			case model.SearchMessageSort_CREATED_AT:
				res = cmp.Compare(a.Message.CreatedAt, b.Message.CreatedAt)
			case model.SearchMessageSort_MODIFIED_AT:
				res = cmp.Compare(a.Message.ModifiedAt, b.Message.ModifiedAt)
			}
			if sort.Type == model.SearchMessageSort_Desc {
				res = -res
			}
			if res != 0 {
				return
			}
		}
		// deterministic tiebreak: the sort is unstable and equal keys are
		// common (equal scores), so without this offset pagination could
		// shuffle results between requests
		if res = strings.Compare(a.ChatId, b.ChatId); res != 0 {
			return
		}
		return strings.Compare(a.MessageId, b.MessageId)
	}
}

func (s *service) chatObjectDo(ctx context.Context, chatObjectId string, proc func(sb chatobject.StoreObject) error) error {
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return cache.DoWait(s.objectGetter, waitCtx, chatObjectId, proc)
}

// chatRepository returns the anystore-backed repository for a chat, without opening the object's
// smartblock. Reads served from it (e.g. GetMessagesByIds, GetPinnedMessages) hit the persisted
// materialized view directly and do not pay the change-tree build, which is O(messages) and can
// take seconds for a large chat. The tree is only required for writes; it is warmed up lazily by
// the subscription path (ChatSubscribeLastMessages, GO-7302), so we do not build it here.
//
// Consistency: the materialized view is current for any chat that has been built at least once
// (the common reopen case) — materialization is incremental from a persisted watermark and
// survives restarts. On a cold start (a chat never built locally, or one with remote changes not
// yet head-synced) the view may be transiently stale/empty until the background warm-up
// materializes it. This is the same window ChatSubscribeLastMessages already accepts; the
// subscription self-corrects via its event stream, so a one-shot caller that needs cold-start
// freshness (notably pinned messages) should refresh once the chat warm-up settles.
func (s *service) chatRepository(chatObjectId string) (chatrepository.Repository, error) {
	spaceId, err := s.spaceIdResolver.ResolveSpaceID(chatObjectId)
	if err != nil {
		return nil, fmt.Errorf("resolve space id: %w", err)
	}
	return s.chatRepoService.Repository(spaceId, chatObjectId)
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
			if err := sb.MarkReadReactions(ctx); err != nil {
				return fmt.Errorf("reactions: %w", err)
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

func (s *service) PinMessages(ctx context.Context, chatObjectId string, messageIds []string, pinned bool) error {
	return s.chatObjectDo(ctx, chatObjectId, func(sb chatobject.StoreObject) error {
		for _, msgId := range messageIds {
			if err := sb.SetMessagePinned(ctx, msgId, pinned); err != nil {
				return fmt.Errorf("failed to set pinned status %v to message: %w", pinned, err)
			}
		}
		return nil
	})
}

func (s *service) GetPinnedMessages(ctx context.Context, chatObjectId string) ([]*chatmodel.Message, error) {
	repo, err := s.chatRepository(chatObjectId)
	if err != nil {
		return nil, err
	}
	return repo.GetPinnedMessages(ctx)
}

func pushGroupId(objectId string) string {
	hash := sha256.Sum256([]byte(objectId))
	return hex.EncodeToString(hash[:])
}
