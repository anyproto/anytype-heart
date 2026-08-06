package service

// v2_chat.go implements the Phase-6 chat surface (APIV2.md §8.7,
// APIV2_SURFACES.md §5). The phase's finding: the middleware already
// returns chatState and messageCount on every messages read and v1 drops
// both — so a polling agent has no cheap peek and the ChatReadMessages
// lastStateId race guard was unreachable (no v1 response ever carried a
// state id). v2 passes both through and POST read forwards the guard.
//
// C7 etag/If-Match deliberately does NOT apply to chats: order ids and
// lastStateId are the stream's native concurrency vocabulary.

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// v2MarkupHint is the D′1 caveat, stated wherever message text fails to
// parse: text is §8 markup SOURCE on both read and write.
const v2MarkupHint = "message text is inline markup source (SPEC §8): *, [, ` and <mention> syntax mint real marks; escape literal specials with a backslash"

//
// ---- chat list + create ----
//

// ListChats returns C5 chat rows via a store query over the chat layouts —
// NO chat opens (opening every chat is the GO-7302 startup cost; Q3 keeps
// the list counter-free, per-chat state comes free on the messages read).
func (s *V2Service) ListChats(ctx context.Context, spaceId string, offset, limit int) ([]apimodel.V2ChatRow, int, bool, error) {
	if err := s.ensureSpace(spaceId); err != nil {
		return nil, 0, false, err
	}
	records, total, err := s.store.SpaceIndex(spaceId).QueryAndCount(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List(util.LayoutsToIntArgs(util.ChatLayouts)),
			},
			{
				RelationKey: bundle.RelationKeyIsHidden,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Bool(true),
			},
		},
		Sorts: []database.SortRequest{{
			RelationKey: bundle.RelationKeyLastModifiedDate,
			Type:        model.BlockContentDataviewSort_Desc,
			IncludeTime: true,
		}},
		Offset: offset,
		Limit:  limit + 1, // one extra record detects has_more without a second scan
	})
	if err != nil {
		return nil, 0, false, fmt.Errorf("query chats in space %s: %w", spaceId, err)
	}
	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}
	rows := make([]apimodel.V2ChatRow, 0, len(records))
	for _, record := range records {
		rows = append(rows, apimodel.V2ChatRow{
			Id:   record.Details.GetString(bundle.RelationKeyId),
			Name: record.Details.GetString(bundle.RelationKeyName),
		})
	}
	return rows, total, hasMore, nil
}

// CreateChat implements POST /v2/spaces/{spaceId}/chats: a thin ObjectCreate
// with the chatDerived type (NOT the Phase-2 snapshot path, which has never
// been exercised for store-backed smartblocks).
func (s *V2Service) CreateChat(ctx context.Context, spaceId string, req apimodel.V2CreateChatRequest, dryRun bool) (*apimodel.V2ChatResult, error) {
	if err := s.ensureSpace(spaceId); err != nil {
		return nil, err
	}
	if req.Name == "" {
		return nil, apimodel.V2ValidationFailed("chat name is required",
			apimodel.V2Issue{Path: "/name", Message: "the chat list row is {id, name} — an unnamed chat is unaddressable by name"})
	}
	if dryRun {
		return &apimodel.V2ChatResult{Name: req.Name, DryRun: true}, nil
	}
	resp := s.mw.ObjectCreate(ctx, &pb.RpcObjectCreateRequest{
		SpaceId:             spaceId,
		ObjectTypeUniqueKey: bundle.TypeKeyChatDerived.URL(),
		Details:             &types.Struct{Fields: map[string]*types.Value{bundle.RelationKeyName.String(): pbtypes.String(req.Name)}},
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcObjectCreateResponseError_NULL {
		return nil, v2ChatRpcError("create chat", int32(resp.Error.Code), int32(pb.RpcObjectCreateResponseError_BAD_INPUT), resp.Error.Description)
	}
	return &apimodel.V2ChatResult{Id: resp.ObjectId, Name: req.Name}, nil
}

//
// ---- messages read (state + messageCount passthrough) ----
//

// V2ChatMessagesQuery carries the GET messages parameters: exclusive
// after/before order-id cursors and the Q4 reactions mode.
type V2ChatMessagesQuery struct {
	After         string
	Before        string
	Limit         int
	FullReactions bool
}

// GetChatMessages implements GET .../chats/{chatId}/messages. The response
// carries the chatState and messageCount the RPC already returns — the
// passthrough v1 dropped (zero extra RPC cost). Messages come back in
// ascending order-id order.
func (s *V2Service) GetChatMessages(ctx context.Context, spaceId, chatId string, q V2ChatMessagesQuery) (*apimodel.V2ChatMessagesResponse, error) {
	if err := s.ensureChat(spaceId, chatId); err != nil {
		return nil, err
	}
	resp := s.mw.ChatGetMessages(ctx, &pb.RpcChatGetMessagesRequest{
		ChatObjectId:  chatId,
		AfterOrderId:  q.After,
		BeforeOrderId: q.Before,
		Limit:         int32(q.Limit),
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcChatGetMessagesResponseError_NULL {
		return nil, v2ChatRpcError("get chat messages", int32(resp.Error.Code), int32(pb.RpcChatGetMessagesResponseError_BAD_INPUT), resp.Error.Description)
	}
	opts := apimodel.V2ChatMessageOptions{
		SpaceId:         spaceId,
		FullReactions:   q.FullReactions,
		ParticipantName: s.participantNameLookup(spaceId),
	}
	messages := make([]apimodel.V2ChatMessage, 0, len(resp.Messages))
	for _, msg := range resp.Messages {
		messages = append(messages, apimodel.V2ChatMessageFromProto(msg, opts))
	}
	return &apimodel.V2ChatMessagesResponse{
		Messages:     messages,
		State:        apimodel.V2ChatStateFromProto(resp.ChatState),
		MessageCount: int(resp.MessageCount),
	}, nil
}

//
// ---- message mutations ----
//

// AddChatMessage implements POST .../messages: text is §8 markup source
// parsed by the anyblockjson inline codec (offset mark arrays never cross
// the API); attachments are bare object ids with the kind inferred from
// each target's layout. A dry run validates everything and sends nothing.
func (s *V2Service) AddChatMessage(ctx context.Context, spaceId, chatId string, req apimodel.V2AddChatMessageRequest, dryRun bool) (*apimodel.V2ChatMessageResult, error) {
	if err := s.ensureChat(spaceId, chatId); err != nil {
		return nil, err
	}
	if req.Text == "" && len(req.Attachments) == 0 {
		return nil, apimodel.V2ValidationFailed("a message needs text or attachments",
			apimodel.V2Issue{Path: "/text", Message: "text and attachments are both empty"})
	}
	text, marks, err := anyblockjson.ParseInlineText(req.Text)
	if err != nil {
		return nil, apimodel.V2ValidationFailed("message text does not parse as inline markup",
			apimodel.V2Issue{Path: "/text", Message: err.Error(), Hint: v2MarkupHint})
	}
	attachments, err := s.resolveChatAttachments(spaceId, req.Attachments)
	if err != nil {
		return nil, err
	}
	if dryRun {
		return &apimodel.V2ChatMessageResult{DryRun: true}, nil
	}
	resp := s.mw.ChatAddMessage(ctx, &pb.RpcChatAddMessageRequest{
		ChatObjectId: chatId,
		Message: &model.ChatMessage{
			ReplyToMessageId: req.ReplyTo,
			Message: &model.ChatMessageMessageContent{
				Text:  text,
				Style: model.BlockContentText_Paragraph,
				Marks: marks,
			},
			Attachments: attachments,
		},
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcChatAddMessageResponseError_NULL {
		return nil, v2ChatRpcError("add chat message", int32(resp.Error.Code), int32(pb.RpcChatAddMessageResponseError_BAD_INPUT), resp.Error.Description)
	}
	return &apimodel.V2ChatMessageResult{Id: resp.MessageId}, nil
}

// EditChatMessage implements PATCH .../messages/{messageId} as a text-only
// MERGE: the middleware's edit replaces the whole message content
// (attachments included — chatmodel content = {message, attachments,
// blocks}), so the service reads the message first and carries its style,
// attachments and blocks through unchanged. A dry run stops after the
// existence check.
func (s *V2Service) EditChatMessage(ctx context.Context, spaceId, chatId, messageId string, req apimodel.V2EditChatMessageRequest, dryRun bool) (*apimodel.V2ChatMessageResult, error) {
	if err := s.ensureChat(spaceId, chatId); err != nil {
		return nil, err
	}
	text, marks, err := anyblockjson.ParseInlineText(req.Text)
	if err != nil {
		return nil, apimodel.V2ValidationFailed("message text does not parse as inline markup",
			apimodel.V2Issue{Path: "/text", Message: err.Error(), Hint: v2MarkupHint})
	}
	existing, err := s.getChatMessageProto(ctx, chatId, messageId)
	if err != nil {
		return nil, err
	}
	if text == "" && len(existing.Attachments) == 0 {
		return nil, apimodel.V2ValidationFailed("a message needs text or attachments",
			apimodel.V2Issue{Path: "/text", Message: "the edited text is empty and the message has no attachments"})
	}
	if dryRun {
		return &apimodel.V2ChatMessageResult{Id: messageId, DryRun: true}, nil
	}
	edited := &model.ChatMessage{
		Message: &model.ChatMessageMessageContent{
			Text:  text,
			Marks: marks,
		},
		Attachments: existing.Attachments,
		Blocks:      existing.Blocks,
	}
	if existing.Message != nil {
		edited.Message.Style = existing.Message.Style
	}
	resp := s.mw.ChatEditMessageContent(ctx, &pb.RpcChatEditMessageContentRequest{
		ChatObjectId:  chatId,
		MessageId:     messageId,
		EditedMessage: edited,
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcChatEditMessageContentResponseError_NULL {
		return nil, v2ChatRpcError("edit chat message", int32(resp.Error.Code), int32(pb.RpcChatEditMessageContentResponseError_BAD_INPUT), resp.Error.Description)
	}
	return &apimodel.V2ChatMessageResult{Id: messageId}, nil
}

// DeleteChatMessage implements DELETE .../messages/{messageId}. A dry run
// verifies the message exists and deletes nothing.
func (s *V2Service) DeleteChatMessage(ctx context.Context, spaceId, chatId, messageId string, dryRun bool) (*apimodel.V2ChatMessageResult, error) {
	if err := s.ensureChat(spaceId, chatId); err != nil {
		return nil, err
	}
	if dryRun {
		if _, err := s.getChatMessageProto(ctx, chatId, messageId); err != nil {
			return nil, err
		}
		return &apimodel.V2ChatMessageResult{Id: messageId, DryRun: true}, nil
	}
	resp := s.mw.ChatDeleteMessage(ctx, &pb.RpcChatDeleteMessageRequest{
		ChatObjectId: chatId,
		MessageId:    messageId,
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcChatDeleteMessageResponseError_NULL {
		return nil, v2ChatRpcError("delete chat message", int32(resp.Error.Code), int32(pb.RpcChatDeleteMessageResponseError_BAD_INPUT), resp.Error.Description)
	}
	return &apimodel.V2ChatMessageResult{Id: messageId}, nil
}

// ToggleChatReaction implements POST .../messages/{messageId}/reactions.
// A dry run reads the message and reports the would-be outcome: added is
// true when the caller does not currently carry the reaction.
func (s *V2Service) ToggleChatReaction(ctx context.Context, spaceId, chatId, messageId string, req apimodel.V2ChatReactionRequest, dryRun bool) (*apimodel.V2ChatReactionResult, error) {
	if err := s.ensureChat(spaceId, chatId); err != nil {
		return nil, err
	}
	if req.Emoji == "" {
		return nil, apimodel.V2ValidationFailed("emoji is required",
			apimodel.V2Issue{Path: "/emoji", Message: "provide the reaction emoji, e.g. 👍"})
	}
	if dryRun {
		existing, err := s.getChatMessageProto(ctx, chatId, messageId)
		if err != nil {
			return nil, err
		}
		added := true
		if existing.Reactions != nil {
			if identityList, ok := existing.Reactions.Reactions[req.Emoji]; ok && identityList != nil {
				for _, identity := range identityList.Ids {
					if identity == s.accountId {
						added = false
						break
					}
				}
			}
		}
		return &apimodel.V2ChatReactionResult{Added: added, DryRun: true}, nil
	}
	resp := s.mw.ChatToggleMessageReaction(ctx, &pb.RpcChatToggleMessageReactionRequest{
		ChatObjectId: chatId,
		MessageId:    messageId,
		Emoji:        req.Emoji,
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcChatToggleMessageReactionResponseError_NULL {
		return nil, v2ChatRpcError("toggle chat reaction", int32(resp.Error.Code), int32(pb.RpcChatToggleMessageReactionResponseError_BAD_INPUT), resp.Error.Description)
	}
	return &apimodel.V2ChatReactionResult{Added: resp.Added}, nil
}

//
// ---- read watermark ----
//

// ReadChat implements POST .../chats/{chatId}/read, forwarding
// {upTo, lastStateId, scope} to ChatReadMessages / ChatReadReactions.
// upTo is INCLUSIVE and required for the messages/mentions scopes: the
// underlying range query is `orderId <= upTo`, so an empty bound would
// silently mark nothing (v1's read_all rides exactly that trap).
// lastStateId is the race guard this phase finally makes reachable — it
// comes from the messages read's state.lastStateId. The reactions scope
// marks ALL unread reactions (the backend takes no bound) and therefore
// rejects upTo/lastStateId.
func (s *V2Service) ReadChat(ctx context.Context, spaceId, chatId string, req apimodel.V2ChatReadRequest, dryRun bool) (*apimodel.V2ChatReadResult, error) {
	if err := s.ensureChat(spaceId, chatId); err != nil {
		return nil, err
	}
	switch req.Scope {
	case "", apimodel.V2ChatReadScopeMessages, apimodel.V2ChatReadScopeMentions:
		if req.UpTo == "" {
			return nil, apimodel.V2ValidationFailed("upTo is required",
				apimodel.V2Issue{Path: "/upTo", Message: "the inclusive order id to mark read up to",
					Hint: "use the newest message's order from GET .../messages (a limit=1 read returns it)"})
		}
		if dryRun {
			return &apimodel.V2ChatReadResult{DryRun: true}, nil
		}
		readType := pb.RpcChatReadMessages_Messages
		if req.Scope == apimodel.V2ChatReadScopeMentions {
			readType = pb.RpcChatReadMessages_Mentions
		}
		resp := s.mw.ChatReadMessages(ctx, &pb.RpcChatReadMessagesRequest{
			ChatObjectId:  chatId,
			Type:          readType,
			BeforeOrderId: req.UpTo,
			LastStateId:   req.LastStateId,
		})
		if resp.Error != nil && resp.Error.Code != pb.RpcChatReadMessagesResponseError_NULL {
			if resp.Error.Code == pb.RpcChatReadMessagesResponseError_MESSAGES_NOT_FOUND {
				return nil, apimodel.V2ValidationFailed("no messages matched the read range",
					apimodel.V2Issue{Path: "/upTo", Message: "the chat is empty or upTo is not a valid order id", Hint: "read GET .../messages and use a returned order value"})
			}
			return nil, v2ChatRpcError("mark chat read", int32(resp.Error.Code), int32(pb.RpcChatReadMessagesResponseError_BAD_INPUT), resp.Error.Description)
		}
		return &apimodel.V2ChatReadResult{}, nil

	case apimodel.V2ChatReadScopeReactions:
		var issues []apimodel.V2Issue
		if req.UpTo != "" {
			issues = append(issues, apimodel.V2Issue{Path: "/upTo", Message: "the reactions scope marks ALL unread reactions — it takes no upTo"})
		}
		if req.LastStateId != "" {
			issues = append(issues, apimodel.V2Issue{Path: "/lastStateId", Message: "the reactions scope marks ALL unread reactions — it takes no lastStateId"})
		}
		if len(issues) > 0 {
			return nil, apimodel.V2ValidationFailed("the reactions scope is all-or-nothing", issues...)
		}
		if dryRun {
			return &apimodel.V2ChatReadResult{DryRun: true}, nil
		}
		resp := s.mw.ChatReadReactions(ctx, &pb.RpcChatReadReactionsRequest{ChatObjectId: chatId})
		if resp.Error != nil && resp.Error.Code != pb.RpcChatReadReactionsResponseError_NULL {
			return nil, v2ChatRpcError("mark chat reactions read", int32(resp.Error.Code), int32(pb.RpcChatReadReactionsResponseError_BAD_INPUT), resp.Error.Description)
		}
		return &apimodel.V2ChatReadResult{}, nil

	default:
		return nil, apimodel.V2ValidationFailed("invalid scope value",
			apimodel.V2Issue{Path: "/scope", Message: fmt.Sprintf("unknown value %q", req.Scope), Hint: "allowed: messages, mentions, reactions"})
	}
}

//
// ---- helpers ----
//

// ensureChat verifies chatId names a chat object in the space: a clean 404
// for an unknown id and a targeted 400 for a non-chat object, instead of
// the RPC's opaque failure.
func (s *V2Service) ensureChat(spaceId, chatId string) error {
	if err := s.ensureSpace(spaceId); err != nil {
		return err
	}
	details, err := s.store.SpaceIndex(spaceId).GetDetails(chatId)
	if err != nil || details.Len() == 0 {
		return apimodel.V2NotFound(fmt.Sprintf("chat %q not found in space %q — list chats with GET /v2/spaces/%s/chats", chatId, spaceId, spaceId))
	}
	layout := model.ObjectTypeLayout(details.GetInt64(bundle.RelationKeyResolvedLayout))
	for _, chatLayout := range util.ChatLayouts {
		if layout == chatLayout {
			return nil
		}
	}
	return apimodel.V2ValidationFailed(fmt.Sprintf("object %q is not a chat", chatId),
		apimodel.V2Issue{Message: fmt.Sprintf("its layout is %q", layout.String()), Hint: fmt.Sprintf("chat ids come from GET /v2/spaces/%s/chats", spaceId)})
}

// participantNameLookup returns a memoized participant-id → display-name
// resolver over the space index. The v1 chat surface resolves names through
// its cross-space subscription cache; the v2 service is store-backed by
// design — participant objects are indexed under their deterministic ids,
// and an unknown participant degrades to an empty name exactly like a v1
// cache miss.
func (s *V2Service) participantNameLookup(spaceId string) func(participantId string) string {
	index := s.store.SpaceIndex(spaceId)
	memo := map[string]string{}
	return func(participantId string) string {
		if name, ok := memo[participantId]; ok {
			return name
		}
		name := ""
		if details, err := index.GetDetails(participantId); err == nil {
			name = details.GetString(bundle.RelationKeyName)
		}
		memo[participantId] = name
		return name
	}
}

// resolveChatAttachments turns bare object ids into typed attachments: the
// kind is inferred from each target's layout (image → image, other file
// layouts → file, anything else → link). An unknown id is a path-addressed
// 400 — attaching an object that does not exist would send a broken message.
func (s *V2Service) resolveChatAttachments(spaceId string, ids []string) ([]*model.ChatMessageAttachment, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	index := s.store.SpaceIndex(spaceId)
	attachments := make([]*model.ChatMessageAttachment, 0, len(ids))
	for i, id := range ids {
		details, err := index.GetDetails(id)
		if err != nil || details.Len() == 0 {
			return nil, apimodel.V2ValidationFailed("attachment target not found",
				apimodel.V2Issue{Path: fmt.Sprintf("/attachments/%d", i),
					Message: fmt.Sprintf("object %q not found in space %q", id, spaceId),
					Hint:    "upload files via POST /v2/spaces/{spaceId}/files first, or pass an existing object id"})
		}
		layout := model.ObjectTypeLayout(details.GetInt64(bundle.RelationKeyResolvedLayout))
		attachmentType := model.ChatMessageAttachment_LINK
		switch {
		case layout == model.ObjectType_image:
			attachmentType = model.ChatMessageAttachment_IMAGE
		case util.IsFileLayout(layout):
			attachmentType = model.ChatMessageAttachment_FILE
		}
		attachments = append(attachments, &model.ChatMessageAttachment{Target: id, Type: attachmentType})
	}
	return attachments, nil
}

// getChatMessageProto fetches one message by id (existence checks, the edit
// merge and the dry-run reaction probe).
func (s *V2Service) getChatMessageProto(ctx context.Context, chatId, messageId string) (*model.ChatMessage, error) {
	resp := s.mw.ChatGetMessagesByIds(ctx, &pb.RpcChatGetMessagesByIdsRequest{
		ChatObjectId: chatId,
		MessageIds:   []string{messageId},
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcChatGetMessagesByIdsResponseError_NULL {
		return nil, v2ChatRpcError("get chat message", int32(resp.Error.Code), int32(pb.RpcChatGetMessagesByIdsResponseError_BAD_INPUT), resp.Error.Description)
	}
	if len(resp.Messages) == 0 || resp.Messages[0] == nil {
		return nil, apimodel.V2NotFound(fmt.Sprintf("message %q not found in chat %q", messageId, chatId))
	}
	return resp.Messages[0], nil
}

// v2ChatRpcError maps a chat RPC failure to the C6 shape: BAD_INPUT (code 2
// across the chat RPC enums) becomes a 400 carrying the middleware's
// description; anything else is a 500 naming the failed operation.
func v2ChatRpcError(op string, code, badInputCode int32, description string) error {
	if code == badInputCode {
		return apimodel.V2ValidationFailed(fmt.Sprintf("%s: invalid input", op),
			apimodel.V2Issue{Message: description})
	}
	msg := op + " failed"
	if description != "" {
		msg += ": " + description
	}
	return apimodel.NewV2Error(500, apimodel.V2CodeInternalError, msg)
}
