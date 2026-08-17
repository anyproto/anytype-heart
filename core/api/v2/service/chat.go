package v2service

// chat.go implements the Phase-6 chat surface (APIV2.md §8.7,
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
	"net/http"
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/editor/chatobject"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	textutil "github.com/anyproto/anytype-heart/util/text"
)

// v2MarkupHint is the D′1 caveat, stated wherever message text fails to
// parse: text is §8 markup SOURCE on both read and write.
const v2MarkupHint = "message text is inline markup source (SPEC §8): *, [, ` and <mention> syntax mint real marks; escape literal specials with a backslash"

// maxChatAttachments caps the attachment list per message — the bound the
// chatMessage discovery schema advertises (maxItems), enforced here so the
// strict schema stays true: an unbounded list means one store lookup per id
// and a permanently replicated CRDT change carrying every entry.
const maxChatAttachments = 32

// defaultChatMessagesLimit mirrors the C10 default page size for the
// cursor-paged messages read.
const defaultChatMessagesLimit = 25

//
// ---- chat list + create ----
//

// ListChats returns C5 chat rows via a store query over the chat layouts —
// NO chat opens (opening every chat is the GO-7302 startup cost; Q3 keeps
// the list counter-free, per-chat state comes free on the messages read).
func (s *Service) ListChats(ctx context.Context, spaceId string, offset, limit int) ([]v2model.ChatRow, int, bool, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
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
	rows := make([]v2model.ChatRow, 0, len(records))
	for _, record := range records {
		rows = append(rows, v2model.ChatRow{
			Id:   record.Details.GetString(bundle.RelationKeyId),
			Name: record.Details.GetString(bundle.RelationKeyName),
		})
	}
	return rows, total, hasMore, nil
}

// CreateChat implements POST /v2/spaces/{spaceId}/chats: a thin ObjectCreate
// with the chatDerived type (NOT the Phase-2 snapshot path, which has never
// been exercised for store-backed smartblocks).
func (s *Service) CreateChat(ctx context.Context, spaceId string, req v2model.CreateChatRequest, dryRun bool) (*v2model.ChatResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	if req.Name == "" {
		return nil, v2model.ValidationFailed("chat name is required",
			v2model.Issue{Path: "/name", Message: "the chat list row is {id, name} — an unnamed chat is unaddressable by name"})
	}
	if dryRun {
		return &v2model.ChatResult{Name: req.Name, DryRun: true}, nil
	}
	resp := s.mw.ObjectCreate(ctx, &pb.RpcObjectCreateRequest{
		SpaceId:             spaceId,
		ObjectTypeUniqueKey: bundle.TypeKeyChatDerived.URL(),
		Details:             &types.Struct{Fields: map[string]*types.Value{bundle.RelationKeyName.String(): pbtypes.String(req.Name)}},
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcObjectCreateResponseError_NULL {
		return nil, v2ChatRpcError("create chat", int32(resp.Error.Code), int32(pb.RpcObjectCreateResponseError_BAD_INPUT), resp.Error.Description)
	}
	return &v2model.ChatResult{Id: resp.ObjectId, Name: req.Name}, nil
}

//
// ---- messages read (state + messageCount passthrough) ----
//

// ChatMessagesQuery carries the GET messages parameters: exclusive
// after/before order-id cursors and the Q4 reactions mode.
type ChatMessagesQuery struct {
	After         string
	Before        string
	Limit         int
	FullReactions bool
}

// GetChatMessages implements GET .../chats/{chatId}/messages. The response
// carries the chatState and messageCount the RPC already returns — the
// passthrough v1 dropped (zero extra RPC cost). Messages come back in
// ascending order-id order. The RPC is asked for limit+1 to detect
// has_more without guessing from len==limit (the C10 spirit): a forward
// walk (?after alone — the only ASC query in the repository) trims the
// newest extra and continues with nextAfter; every other query is anchored
// at its newest end (the repository sorts DESC), so the OLDEST extra is
// trimmed and paging continues backward with nextBefore.
func (s *Service) GetChatMessages(ctx context.Context, spaceId, chatId string, q ChatMessagesQuery) (*v2model.ChatMessagesResponse, error) {
	if err := s.ensureChat(ctx, spaceId, chatId); err != nil {
		return nil, err
	}
	limit := q.Limit
	if limit <= 0 {
		limit = defaultChatMessagesLimit
	}
	resp := s.mw.ChatGetMessages(ctx, &pb.RpcChatGetMessagesRequest{
		ChatObjectId:  chatId,
		AfterOrderId:  q.After,
		BeforeOrderId: q.Before,
		Limit:         int32(limit + 1), // one extra detects has_more
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcChatGetMessagesResponseError_NULL {
		return nil, v2ChatRpcError("get chat messages", int32(resp.Error.Code), int32(pb.RpcChatGetMessagesResponseError_BAD_INPUT), resp.Error.Description)
	}
	forward := q.After != "" && q.Before == ""
	protos := resp.Messages
	hasMore := len(protos) > limit
	if hasMore {
		if forward {
			protos = protos[:limit] // ascending: the extra is the newest
		} else {
			protos = protos[len(protos)-limit:] // newest-anchored: the extra is the oldest
		}
	}
	opts := v2model.ChatMessageOptions{
		SpaceId:         spaceId,
		FullReactions:   q.FullReactions,
		ParticipantName: s.participantNameLookup(spaceId),
	}
	messages := make([]v2model.ChatMessage, 0, len(protos))
	for _, msg := range protos {
		messages = append(messages, v2model.ChatMessageFromProto(msg, opts))
	}
	out := &v2model.ChatMessagesResponse{
		Messages:     messages,
		State:        v2model.ChatStateFromProto(resp.ChatState),
		MessageCount: int(resp.MessageCount),
		HasMore:      hasMore,
	}
	if hasMore && len(messages) > 0 {
		if forward {
			out.NextAfter = messages[len(messages)-1].Order
		} else {
			out.NextBefore = messages[0].Order
		}
	}
	return out, nil
}

//
// ---- message mutations ----
//

// AddChatMessage implements POST .../messages: text is §8 markup source
// parsed by the anyblockjson inline codec (offset mark arrays never cross
// the API); attachments are bare object ids with the kind inferred from
// each target's layout. A dry run validates everything and sends nothing.
func (s *Service) AddChatMessage(ctx context.Context, spaceId, chatId string, req v2model.AddChatMessageRequest, dryRun bool) (*v2model.ChatMessageResult, error) {
	if err := s.ensureChatWrite(ctx, spaceId, chatId); err != nil {
		return nil, err
	}
	if req.Text == "" && len(req.Attachments) == 0 {
		return nil, v2model.ValidationFailed("a message needs text or attachments",
			v2model.Issue{Path: "/text", Message: "text and attachments are both empty"})
	}
	text, marks, err := anyblockjson.ParseInlineText(req.Text)
	if err != nil {
		return nil, v2model.ValidationFailed("message text does not parse as inline markup",
			v2model.Issue{Path: "/text", Message: err.Error(), Hint: v2MarkupHint})
	}
	if err := v2ValidateChatTextLength(text); err != nil {
		return nil, err
	}
	attachments, err := s.resolveChatAttachments(spaceId, req.Attachments)
	if err != nil {
		return nil, err
	}
	if dryRun {
		return &v2model.ChatMessageResult{DryRun: true}, nil
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
	return &v2model.ChatMessageResult{Id: resp.MessageId}, nil
}

// EditChatMessage implements PATCH .../messages/{messageId} as a text-only
// MERGE: the middleware's edit replaces the whole message content
// (attachments included — chatmodel content = {message, attachments,
// blocks}), so the service reads the message first and carries its style,
// attachments and blocks through unchanged. A dry run stops after the
// existence check.
func (s *Service) EditChatMessage(ctx context.Context, spaceId, chatId, messageId string, req v2model.EditChatMessageRequest, dryRun bool) (*v2model.ChatMessageResult, error) {
	if err := s.ensureChatWrite(ctx, spaceId, chatId); err != nil {
		return nil, err
	}
	text, marks, err := anyblockjson.ParseInlineText(req.Text)
	if err != nil {
		return nil, v2model.ValidationFailed("message text does not parse as inline markup",
			v2model.Issue{Path: "/text", Message: err.Error(), Hint: v2MarkupHint})
	}
	if err := v2ValidateChatTextLength(text); err != nil {
		return nil, err
	}
	existing, err := s.getChatMessageProto(ctx, chatId, messageId)
	if err != nil {
		return nil, err
	}
	if text == "" && len(existing.Attachments) == 0 {
		return nil, v2model.ValidationFailed("a message needs text or attachments",
			v2model.Issue{Path: "/text", Message: "the edited text is empty and the message has no attachments"})
	}
	if dryRun {
		return &v2model.ChatMessageResult{Id: messageId, DryRun: true}, nil
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
	return &v2model.ChatMessageResult{Id: messageId}, nil
}

// DeleteChatMessage implements DELETE .../messages/{messageId}. BOTH paths
// run the existence check: the store handler treats deleting a missing
// document as success, so without it the real call would answer 200 for a
// message that never existed while the dry run 404s — C9's contract is
// that the dry run predicts the real call. The read also feeds the file-GC
// warnings: the middleware permanently deletes (skipBin) attachment/link
// targets orphaned by the delete, asynchronously, after the API replied —
// the response names the ids at risk instead of hiding the irreversible
// part behind a 200.
func (s *Service) DeleteChatMessage(ctx context.Context, spaceId, chatId, messageId string, dryRun bool) (*v2model.ChatMessageResult, error) {
	if err := s.ensureChatWrite(ctx, spaceId, chatId); err != nil {
		return nil, err
	}
	existing, err := s.getChatMessageProto(ctx, chatId, messageId)
	if err != nil {
		return nil, err
	}
	warnings := v2ChatDeleteWarnings(existing)
	if dryRun {
		return &v2model.ChatMessageResult{Id: messageId, DryRun: true, Warnings: warnings}, nil
	}
	resp := s.mw.ChatDeleteMessage(ctx, &pb.RpcChatDeleteMessageRequest{
		ChatObjectId: chatId,
		MessageId:    messageId,
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcChatDeleteMessageResponseError_NULL {
		return nil, v2ChatRpcError("delete chat message", int32(resp.Error.Code), int32(pb.RpcChatDeleteMessageResponseError_BAD_INPUT), resp.Error.Description)
	}
	return &v2model.ChatMessageResult{Id: messageId, Warnings: warnings}, nil
}

// ToggleChatReaction implements POST .../messages/{messageId}/reactions.
// Both paths read the message first: the RPC surfaces a missing message as
// an opaque UNKNOWN_ERROR (HasMyReaction's FindId), so the check turns it
// into a clean 404. A dry run reports the would-be outcome — added is true
// when the caller does not currently carry the reaction — unless the
// service has no account identity to predict with, in which case added is
// omitted with a warning instead of asserting a coin flip.
func (s *Service) ToggleChatReaction(ctx context.Context, spaceId, chatId, messageId string, req v2model.ChatReactionRequest, dryRun bool) (*v2model.ChatReactionResult, error) {
	if err := s.ensureChatWrite(ctx, spaceId, chatId); err != nil {
		return nil, err
	}
	if req.Emoji == "" {
		return nil, v2model.ValidationFailed("emoji is required",
			v2model.Issue{Path: "/emoji", Message: "provide the reaction emoji, e.g. 👍"})
	}
	existing, err := s.getChatMessageProto(ctx, chatId, messageId)
	if err != nil {
		return nil, err
	}
	if dryRun {
		if s.accountId == "" {
			return &v2model.ChatReactionResult{DryRun: true, Warnings: []v2model.Issue{{
				Path:    "/emoji",
				Message: "the would-be outcome could not be predicted: the service has no account identity",
				Hint:    "run without dry_run for the authoritative added value",
			}}}, nil
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
		return &v2model.ChatReactionResult{Added: &added, DryRun: true}, nil
	}
	resp := s.mw.ChatToggleMessageReaction(ctx, &pb.RpcChatToggleMessageReactionRequest{
		ChatObjectId: chatId,
		MessageId:    messageId,
		Emoji:        req.Emoji,
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcChatToggleMessageReactionResponseError_NULL {
		return nil, v2ChatRpcError("toggle chat reaction", int32(resp.Error.Code), int32(pb.RpcChatToggleMessageReactionResponseError_BAD_INPUT), resp.Error.Description)
	}
	added := resp.Added
	return &v2model.ChatReactionResult{Added: &added}, nil
}

//
// ---- read watermark ----
//

// ReadChat implements POST .../chats/{chatId}/read, forwarding
// {upTo, lastStateId, scope} to ChatReadMessages / ChatReadReactions.
// upTo is INCLUSIVE and required for the messages/mentions scopes: the
// underlying range query is `orderId <= upTo`, so an empty bound would
// silently mark nothing (v1's read_all rides exactly that trap).
// lastStateId — the race guard this phase finally makes reachable — is
// required for the SAME reason: the repository additionally ANDs
// `stateId <= lastStateId` and every stored message carries a non-empty
// bson state id, so `stateId <= ""` matches nothing and an omitted guard
// is the identical silent no-op one field over. Both values ride the same
// GET messages response (the newest order + state.lastStateId), so
// requiring them costs the agent no extra call. The reactions scope marks
// ALL unread reactions (the backend takes no bound) and therefore rejects
// upTo/lastStateId.
func (s *Service) ReadChat(ctx context.Context, spaceId, chatId string, req v2model.ChatReadRequest, dryRun bool) (*v2model.ChatReadResult, error) {
	if err := s.ensureChatWrite(ctx, spaceId, chatId); err != nil {
		return nil, err
	}
	switch req.Scope {
	case "", v2model.ChatReadScopeMessages, v2model.ChatReadScopeMentions:
		var missing []v2model.Issue
		if req.UpTo == "" {
			missing = append(missing, v2model.Issue{Path: "/upTo", Message: "the inclusive order id to mark read up to",
				Hint: "use the newest message's order from GET .../messages (a limit=1 read returns it)"})
		}
		if req.LastStateId == "" {
			missing = append(missing, v2model.Issue{Path: "/lastStateId", Message: "the race guard from the same messages read",
				Hint: "use state.lastStateId from GET .../messages — an empty guard matches no message and would silently mark nothing"})
		}
		if len(missing) > 0 {
			return nil, v2model.ValidationFailed("the read watermark needs upTo and lastStateId", missing...)
		}
		if dryRun {
			return &v2model.ChatReadResult{DryRun: true}, nil
		}
		readType := pb.RpcChatReadMessages_Messages
		if req.Scope == v2model.ChatReadScopeMentions {
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
				return nil, v2model.ValidationFailed("no messages matched the read range",
					v2model.Issue{Path: "/upTo", Message: "the chat is empty or upTo is not a valid order id", Hint: "read GET .../messages and use a returned order value"})
			}
			return nil, v2ChatRpcError("mark chat read", int32(resp.Error.Code), int32(pb.RpcChatReadMessagesResponseError_BAD_INPUT), resp.Error.Description)
		}
		return &v2model.ChatReadResult{}, nil

	case v2model.ChatReadScopeReactions:
		var issues []v2model.Issue
		if req.UpTo != "" {
			issues = append(issues, v2model.Issue{Path: "/upTo", Message: "the reactions scope marks ALL unread reactions — it takes no upTo"})
		}
		if req.LastStateId != "" {
			issues = append(issues, v2model.Issue{Path: "/lastStateId", Message: "the reactions scope marks ALL unread reactions — it takes no lastStateId"})
		}
		if len(issues) > 0 {
			return nil, v2model.ValidationFailed("the reactions scope is all-or-nothing", issues...)
		}
		if dryRun {
			return &v2model.ChatReadResult{DryRun: true}, nil
		}
		resp := s.mw.ChatReadReactions(ctx, &pb.RpcChatReadReactionsRequest{ChatObjectId: chatId})
		if resp.Error != nil && resp.Error.Code != pb.RpcChatReadReactionsResponseError_NULL {
			return nil, v2ChatRpcError("mark chat reactions read", int32(resp.Error.Code), int32(pb.RpcChatReadReactionsResponseError_BAD_INPUT), resp.Error.Description)
		}
		return &v2model.ChatReadResult{}, nil

	default:
		return nil, v2model.ValidationFailed("invalid scope value",
			v2model.Issue{Path: "/scope", Message: fmt.Sprintf("unknown value %q", req.Scope), Hint: "allowed: messages, mentions, reactions"})
	}
}

//
// ---- helpers ----
//

// ensureChat verifies chatId names a chat object in the space: a clean 404
// for an unknown id and a targeted 400 for a non-chat object, instead of
// the RPC's opaque failure.
func (s *Service) ensureChat(ctx context.Context, spaceId, chatId string) error {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return err
	}
	details, err := s.store.SpaceIndex(spaceId).GetDetails(chatId)
	if err != nil || details.Len() == 0 {
		return v2model.NotFound(fmt.Sprintf("chat %q not found in space %q — list chats with GET /v2/spaces/%s/chats", chatId, spaceId, spaceId))
	}
	layout := model.ObjectTypeLayout(details.GetInt64(bundle.RelationKeyResolvedLayout))
	for _, chatLayout := range util.ChatLayouts {
		if layout == chatLayout {
			return nil
		}
	}
	return v2model.ValidationFailed(fmt.Sprintf("object %q is not a chat", chatId),
		v2model.Issue{Message: fmt.Sprintf("its layout is %q", layout.String()), Hint: fmt.Sprintf("chat ids come from GET /v2/spaces/%s/chats", spaceId)})
}

// ensureChatWrite is ensureChat for the chat WRITE entry points (message
// create/edit/delete, reactions, and the read-watermark advance — a write:
// it mutates synced state). Route-gate precedence: grant space check, then
// the write-verb check, then the chat lookup — a read-only key is refused
// before anything resolves.
func (s *Service) ensureChatWrite(ctx context.Context, spaceId, chatId string) error {
	if err := ensureSpaceGranted(ctx, spaceId); err != nil {
		return err
	}
	if err := ensureWriteGranted(ctx); err != nil {
		return err
	}
	return s.ensureChat(ctx, spaceId, chatId)
}

// participantNameLookup returns a memoized participant-id → display-name
// resolver over the space index. The v1 chat surface resolves names through
// its cross-space subscription cache; the v2 service is store-backed by
// design — participant objects are indexed under their deterministic ids,
// and an unknown participant degrades to an empty name exactly like a v1
// cache miss.
func (s *Service) participantNameLookup(spaceId string) func(participantId string) string {
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
func (s *Service) resolveChatAttachments(spaceId string, ids []string) ([]*model.ChatMessageAttachment, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > maxChatAttachments {
		return nil, v2model.ValidationFailed("too many attachments",
			v2model.Issue{Path: "/attachments",
				Message: fmt.Sprintf("%d attachments — the cap is %d per message (the bound the chatMessage schema advertises)", len(ids), maxChatAttachments),
				Hint:    "split the message, or link a collection of the objects instead"})
	}
	index := s.store.SpaceIndex(spaceId)
	attachments := make([]*model.ChatMessageAttachment, 0, len(ids))
	for i, id := range ids {
		details, err := index.GetDetails(id)
		if err != nil || details.Len() == 0 {
			return nil, v2model.ValidationFailed("attachment target not found",
				v2model.Issue{Path: fmt.Sprintf("/attachments/%d", i),
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
func (s *Service) getChatMessageProto(ctx context.Context, chatId, messageId string) (*model.ChatMessage, error) {
	resp := s.mw.ChatGetMessagesByIds(ctx, &pb.RpcChatGetMessagesByIdsRequest{
		ChatObjectId: chatId,
		MessageIds:   []string{messageId},
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcChatGetMessagesByIdsResponseError_NULL {
		return nil, v2ChatRpcError("get chat message", int32(resp.Error.Code), int32(pb.RpcChatGetMessagesByIdsResponseError_BAD_INPUT), resp.Error.Description)
	}
	if len(resp.Messages) == 0 || resp.Messages[0] == nil {
		return nil, v2model.NotFound(fmt.Sprintf("message %q not found in chat %q", messageId, chatId))
	}
	return resp.Messages[0], nil
}

// v2ValidateChatTextLength enforces the store's message-text cap
// (chatmodel.MaxMessageLength, counted in UTF-16 code units) BEFORE the
// RPC. The chat RPC enums carry no usable error code, so without this
// pre-check an over-long message — the mistake the discovery schema's
// maxLength exists to prevent — would come back as a retry-looping 500.
// The cap applies to the PARSED text, matching what the store validates.
func v2ValidateChatTextLength(parsedText string) error {
	if length := len(textutil.StrToUTF16(parsedText)); length > chatmodel.MaxMessageLength {
		return v2model.ValidationFailed("message text is too long",
			v2model.Issue{Path: "/text",
				Message: fmt.Sprintf("the text is %d UTF-16 code units — the cap is %d", length, chatmodel.MaxMessageLength),
				Hint:    "the cap counts UTF-16 code units (an emoji counts 2+); split the message"})
	}
	return nil
}

// v2ChatDeleteWarnings surfaces the irreversible side effect of a message
// delete: the middleware garbage-collects attachment and link-block targets
// orphaned by the delete with skipBin=true — permanently deleted, not
// binned, asynchronously AFTER the API has replied. The dry run and the
// real receipt both carry the ids at risk (C6 warnings).
func v2ChatDeleteWarnings(msg *model.ChatMessage) []v2model.Issue {
	var ids []string
	for _, att := range msg.Attachments {
		if att != nil && att.Target != "" {
			ids = append(ids, att.Target)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return []v2model.Issue{{
		Path:    "/attachments",
		Message: fmt.Sprintf("deleting this message may PERMANENTLY delete its attached objects (not moved to the bin): %s", strings.Join(ids, ", ")),
		Hint:    "an attachment is garbage-collected asynchronously when this message was its only reference",
	}}
}

// v2ChatRpcError maps a chat RPC failure to the C6 shape. BAD_INPUT (code 2
// across the chat RPC enums) becomes a 400 carrying the middleware's
// description — but the chat RPCs never actually produce it (core
// mapErrorCode has no chat errToCode mappings and defaults everything to
// UNKNOWN_ERROR), so ordinary caller mistakes are classified on the
// description instead of defaulting the whole class to a retry-looping 500.
// The matched strings are pinned by the middleware: chatobject.go wraps
// store validation as "validate: …", any-store's ErrDocNotFound reads
// "document not found", and the two foreign-message refusals are matched
// through the chatobject sentinels themselves (ErrModifyForeignMessage for
// the EDIT path, ErrDeleteForeignMessage for DELETE — surface review M2b:
// matching the delete prose alone left the edit refusal a 500), so a
// rewording in the producer updates the matcher at compile time. The
// forbidden arms run FIRST: the edit refusal rides storestate.ErrValidation,
// and a future "validate:"-wrapped rendering must not downgrade a permanent
// 403 to a 400.
func v2ChatRpcError(op string, code, badInputCode int32, description string) error {
	if code == badInputCode {
		return v2model.ValidationFailed(fmt.Sprintf("%s: invalid input", op),
			v2model.Issue{Message: description})
	}
	switch {
	case strings.Contains(description, chatobject.ErrModifyForeignMessage.Error()),
		strings.Contains(description, chatobject.ErrDeleteForeignMessage.Error()):
		return v2model.NewError(http.StatusForbidden, v2model.CodeForbidden,
			fmt.Sprintf("%s: %s — only the author can edit or delete a message", op, description))
	case strings.Contains(description, "validate:"):
		return v2model.ValidationFailed(fmt.Sprintf("%s: the middleware rejected the message", op),
			v2model.Issue{Message: description})
	case strings.Contains(description, "not found"):
		return v2model.NotFound(fmt.Sprintf("%s: %s", op, description))
	}
	msg := op + " failed"
	if description != "" {
		msg += ": " + description
	}
	return v2model.NewError(http.StatusInternalServerError, v2model.CodeInternalError, msg)
}
