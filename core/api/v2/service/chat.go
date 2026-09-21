package v2service

// chat.go implements the chat surface. The middleware already returns
// chatState and message_count on every messages read and v1 drops
// both — so a polling agent has no cheap peek and the ChatReadMessages
// last_state_id race guard was unreachable (no v1 response ever carried a
// state id). v2 passes both through and POST read forwards the guard.
//
// C7 etag/If-Match deliberately does NOT apply to chats: order ids and
// last_state_id are the stream's native concurrency vocabulary.

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/anyproto/any-sync/commonspace/object/acl/list"
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
const v2MarkupHint = "message text is inline markup source: *, [, ` and <mention> syntax mint real marks; escape literal specials with a backslash"

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

// CreateChat implements POST /v2/spaces/{space_id}/chats: a thin ObjectCreate
// with the chatDerived type (NOT the snapshot-create path, which has never
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
// ---- messages read (state + message_count passthrough) ----
//

// ChatMessagesQuery carries the GET messages parameters: exclusive
// after/before order-id cursors and the Q4 reactions mode.
type ChatMessagesQuery struct {
	After         string
	Before        string
	Limit         int
	FullReactions bool
}

// GetChatMessages implements GET .../chats/{chat_id}/messages. The response
// carries the chatState and message_count the RPC already returns — the
// passthrough v1 dropped (zero extra RPC cost). Messages come back in
// ascending order-id order. The RPC is asked for limit+1 to detect
// has_more without guessing from len==limit (the C10 spirit): a forward
// walk (?after alone — the only ASC query in the repository) trims the
// newest extra and continues with next_after; every other query is anchored
// at its newest end (the repository sorts DESC), so the OLDEST extra is
// trimmed and paging continues backward with next_before.
func (s *Service) GetChatMessages(ctx context.Context, spaceId, chatId string, q ChatMessagesQuery) (*v2model.ChatMessagesResponse, error) {
	if _, err := s.ensureChat(ctx, spaceId, chatId); err != nil {
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
	// the served order is the contract, not the RPC's: ascending by order
	// id whichever way the range was walked
	sort.SliceStable(messages, func(i, j int) bool { return messages[i].Order < messages[j].Order })
	lifetimeCount := resp.LifetimeMessageCount
	if lifetimeCount < resp.MessageCount {
		// A lifetime total cannot be smaller than the current live set. This
		// also keeps reads honest while an upgraded chat's history replay is
		// being materialized for the first time.
		lifetimeCount = resp.MessageCount
	}
	// message_count is the messages the chat HOLDS (round-four eval R4-3: a
	// caller answering "how many messages" from it got the lifetime total,
	// which never decrements on delete); the lifetime total rides beside it
	out := &v2model.ChatMessagesResponse{
		Messages:             messages,
		State:                v2model.ChatStateFromProto(resp.ChatState),
		MessageCount:         int(resp.MessageCount),
		LifetimeMessageCount: int(lifetimeCount),
		HasMore:              hasMore,
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
// each target's layout. The parsed text is stored the way the chat's
// layout stores it (chatMessageBody: content for a space chat, one text
// block for a discussion). A dry run validates everything and sends nothing.
func (s *Service) AddChatMessage(ctx context.Context, spaceId, chatId string, req v2model.AddChatMessageRequest, dryRun bool) (*v2model.ChatMessageResult, error) {
	layout, err := s.ensureChatWrite(ctx, spaceId, chatId)
	if err != nil {
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
	links := s.newSpaceLinkExpander(ctx)
	links.Marks(marks)
	if err := v2ValidateChatTextLength(text); err != nil {
		return nil, err
	}
	attachments, err := s.resolveChatAttachments(spaceId, req.Attachments)
	if err != nil {
		return nil, err
	}
	content, blocks := chatMessageBody(layout, text, marks)
	if err := v2ValidateChatBody(layout, blocks, len(attachments)); err != nil {
		return nil, err
	}
	if dryRun {
		return &v2model.ChatMessageResult{DryRun: true, Warnings: links.Warnings("/text")}, nil
	}
	resp := s.mw.ChatAddMessage(ctx, &pb.RpcChatAddMessageRequest{
		ChatObjectId: chatId,
		Message: &model.ChatMessage{
			ReplyToMessageId: req.ReplyTo,
			Message:          content,
			Blocks:           blocks,
			Attachments:      attachments,
		},
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcChatAddMessageResponseError_NULL {
		return nil, v2ChatRpcError("add chat message", int32(resp.Error.Code), int32(pb.RpcChatAddMessageResponseError_BAD_INPUT), resp.Error.Description)
	}
	return &v2model.ChatMessageResult{Id: resp.MessageId, Warnings: links.Warnings("/text")}, nil
}

// EditChatMessage implements PATCH .../messages/{message_id} as a text-only
// MERGE: the middleware's edit replaces the whole message content
// (attachments included — chatmodel content = {message, attachments,
// blocks}), so the service reads the message first and carries its style
// and attachments through unchanged. In a content chat the blocks are
// carried through too and the text lands in the content; in a blocks chat
// (a discussion) the text REPLACES the blocks with one text block — the
// same "every mark is re-derived from the text you send" rule, one level
// up: a quote or link block the new text does not spell out is lost. A dry
// run stops after the existence check.
func (s *Service) EditChatMessage(ctx context.Context, spaceId, chatId, messageId string, req v2model.EditChatMessageRequest, dryRun bool) (*v2model.ChatMessageResult, error) {
	layout, err := s.ensureChatWrite(ctx, spaceId, chatId)
	if err != nil {
		return nil, err
	}
	text, marks, err := anyblockjson.ParseInlineText(req.Text)
	if err != nil {
		return nil, v2model.ValidationFailed("message text does not parse as inline markup",
			v2model.Issue{Path: "/text", Message: err.Error(), Hint: v2MarkupHint})
	}
	links := s.newSpaceLinkExpander(ctx)
	links.Marks(marks)
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
	warnings := links.Warnings("/text")
	if chatWritesBlocks(layout) {
		warnings = append(warnings, blocksEditWarnings(existing.Blocks)...)
	}
	content, blocks := chatMessageBody(layout, text, marks)
	if err := v2ValidateChatBody(layout, blocks, len(existing.Attachments)); err != nil {
		return nil, err
	}
	if dryRun {
		return &v2model.ChatMessageResult{Id: messageId, DryRun: true, Warnings: warnings}, nil
	}
	if !chatWritesBlocks(layout) {
		blocks = existing.Blocks
		if existing.Message != nil {
			content.Style = existing.Message.Style
		}
	}
	edited := &model.ChatMessage{
		Message:     content,
		Blocks:      blocks,
		Attachments: existing.Attachments,
	}
	resp := s.mw.ChatEditMessageContent(ctx, &pb.RpcChatEditMessageContentRequest{
		ChatObjectId:  chatId,
		MessageId:     messageId,
		EditedMessage: edited,
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcChatEditMessageContentResponseError_NULL {
		return nil, v2ChatRpcError("edit chat message", int32(resp.Error.Code), int32(pb.RpcChatEditMessageContentResponseError_BAD_INPUT), resp.Error.Description)
	}
	return &v2model.ChatMessageResult{Id: messageId, Warnings: warnings}, nil
}

// DeleteChatMessage implements DELETE .../messages/{message_id}. BOTH paths
// run the existence check: the store handler treats deleting a missing
// document as success, so without it the real call would answer 200 for a
// message that never existed while the dry run 404s — C9's contract is
// that the dry run predicts the real call. The read also feeds the file-GC
// warnings: the middleware permanently deletes (skipBin) attachment/link
// targets orphaned by the delete, asynchronously, after the API replied —
// the response names the ids at risk instead of hiding the irreversible
// part behind a 200.
func (s *Service) DeleteChatMessage(ctx context.Context, spaceId, chatId, messageId string, dryRun bool) (*v2model.ChatMessageResult, error) {
	if _, err := s.ensureChatWrite(ctx, spaceId, chatId); err != nil {
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

// ToggleChatReaction implements POST .../messages/{message_id}/reactions.
// Both paths read the message first: the RPC surfaces a missing message as
// an opaque UNKNOWN_ERROR (HasMyReaction's FindId), so the check turns it
// into a clean 404. A dry run reports the would-be outcome — added is true
// when the caller does not currently carry the reaction — unless the
// service has no account identity to predict with, in which case added is
// omitted with a warning instead of asserting a coin flip.
func (s *Service) ToggleChatReaction(ctx context.Context, spaceId, chatId, messageId string, req v2model.ChatReactionRequest, dryRun bool) (*v2model.ChatReactionResult, error) {
	if _, err := s.ensureChatWrite(ctx, spaceId, chatId); err != nil {
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

// ReadChat implements POST .../chats/{chat_id}/read, forwarding
// {up_to, last_state_id, scope} to ChatReadMessages / ChatReadReactions.
// up_to is INCLUSIVE and required for the messages/mentions scopes: the
// underlying range query is `orderId <= up_to`, so an empty bound would
// silently mark nothing (v1's read_all rides exactly that trap).
// last_state_id, the native race guard, is
// required for the SAME reason: the repository additionally ANDs
// `stateId <= last_state_id` and every stored message carries a non-empty
// bson state id, so `stateId <= ""` matches nothing and an omitted guard
// is the identical silent no-op one field over. Both values ride the same
// GET messages response (the newest order + state.last_state_id), so
// requiring them costs the agent no extra call. The reactions scope marks
// ALL unread reactions (the backend takes no bound) and therefore rejects
// up_to/last_state_id.
func (s *Service) ReadChat(ctx context.Context, spaceId, chatId string, req v2model.ChatReadRequest, dryRun bool) (*v2model.ChatReadResult, error) {
	if _, err := s.ensureChatWrite(ctx, spaceId, chatId); err != nil {
		return nil, err
	}
	switch req.Scope {
	case "", v2model.ChatReadScopeMessages, v2model.ChatReadScopeMentions:
		var missing []v2model.Issue
		if req.UpTo == "" {
			missing = append(missing, v2model.Issue{Path: "/up_to", Message: "the inclusive order id to mark read up to"}.
				Hintf("use the newest message's order from %s", v2model.RefGetChatMessages(spaceId, chatId).With("limit", "1")))
		}
		if req.LastStateId == "" {
			missing = append(missing, v2model.Issue{Path: "/last_state_id", Message: "the race guard from the same messages read"}.
				Hintf("use state.last_state_id from %s — an empty guard matches no message and would silently mark nothing", v2model.RefGetChatMessages(spaceId, chatId)))
		}
		if len(missing) > 0 {
			return nil, v2model.ValidationFailed("the read watermark needs up_to and last_state_id", missing...)
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
					v2model.Issue{Path: "/up_to", Message: "the chat is empty or up_to is not a valid order id"}.
						Hintf("read %s and use a returned order value", v2model.RefGetChatMessages(spaceId, chatId)))
			}
			return nil, v2ChatRpcError("mark chat read", int32(resp.Error.Code), int32(pb.RpcChatReadMessagesResponseError_BAD_INPUT), resp.Error.Description)
		}
		return &v2model.ChatReadResult{State: s.chatStateAfter(ctx, chatId)}, nil

	case v2model.ChatReadScopeReactions:
		var issues []v2model.Issue
		if req.UpTo != "" {
			issues = append(issues, v2model.Issue{Path: "/up_to", Message: "the reactions scope marks ALL unread reactions — it takes no up_to"})
		}
		if req.LastStateId != "" {
			issues = append(issues, v2model.Issue{Path: "/last_state_id", Message: "the reactions scope marks ALL unread reactions — it takes no last_state_id"})
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
		return &v2model.ChatReadResult{State: s.chatStateAfter(ctx, chatId)}, nil

	default:
		return nil, v2model.ValidationFailed("invalid scope value",
			v2model.Issue{Path: "/scope", Message: fmt.Sprintf("unknown value %q", req.Scope), Hint: "allowed: messages, mentions, reactions"})
	}
}

//
// ---- helpers ----
//

// chatMessageLayouts are the layouts a chat_id may name: a space chat and
// an object's discussion. Both are the same store-backed chat object, so
// every message operation serves both unchanged. util.ChatLayouts — the
// list_chats filter — stays chats-only on purpose: a discussion belongs to
// its object and is reached through it (the `discussion` member of the
// object read, or create_discussion), never listed beside the space chats,
// which is also how the desktop keeps the two apart.
var chatMessageLayouts = append(append([]model.ObjectTypeLayout{}, util.ChatLayouts...), model.ObjectType_discussion)

// ensureChat verifies chatId names a chat object in the space — a space
// chat or an object's discussion: a clean 404 for an unknown id and a
// targeted 400 for anything else, instead of the RPC's opaque failure. The
// 400 steers an object id to create_discussion, since the likeliest way to
// send a page id here is wanting its comment thread.
func (s *Service) ensureChat(ctx context.Context, spaceId, chatId string) (model.ObjectTypeLayout, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return 0, err
	}
	details, err := s.store.SpaceIndex(spaceId).GetDetails(chatId)
	if err != nil || details.Len() == 0 {
		return 0, v2model.NotFound(fmt.Sprintf("chat %q not found in space %q", chatId, spaceId),
			v2model.Issue{Path: "chat_id", Message: "no chat or discussion has this id"}.
				Hintf("list chats with %s; an object's discussion id is the discussion member of its read, or comes from %s", v2model.RefListChats(spaceId), v2model.RefCreateDiscussion(spaceId, "")))
	}
	layout := model.ObjectTypeLayout(details.GetInt64(bundle.RelationKeyResolvedLayout))
	for _, chatLayout := range chatMessageLayouts {
		if layout == chatLayout {
			return layout, nil
		}
	}
	issue := v2model.Issue{Path: "chat_id", Message: fmt.Sprintf("its layout is %q", layout.String())}.
		Hintf("chat ids come from %s", v2model.RefListChats(spaceId))
	if discussionHolderRow(details) {
		issue = issue.Hintf("chat ids come from %s; for this object's comment thread, use the id from %s as chat_id", v2model.RefListChats(spaceId), v2model.RefCreateDiscussion(spaceId, chatId))
	}
	return 0, v2model.ValidationFailed(fmt.Sprintf("object %q is not a chat", chatId), issue)
}

// discussionHolderRow says whether the object behind a store row can hold
// a discussion — the store-row view of CreateDiscussion's rules (a page or
// file, not a template, not archived), so a repair hint only recommends
// create_discussion where it can succeed. A template carries a page
// layout and is told apart by its target type.
func discussionHolderRow(details *domain.Details) bool {
	layout := model.ObjectTypeLayout(details.GetInt64(bundle.RelationKeyResolvedLayout))
	if layout == model.ObjectType_chatDerived || layout == model.ObjectType_discussion ||
		details.GetBool(bundle.RelationKeyIsArchived) || details.GetString(bundle.RelationKeyTargetObjectType) != "" {
		return false
	}
	for _, objectLayout := range util.ObjectLayouts {
		if layout == objectLayout {
			return true
		}
	}
	return util.IsFileLayout(layout)
}

// ensureChatWrite is ensureChat for the chat WRITE entry points (message
// create/edit/delete, reactions, and the read-watermark advance — a write:
// it mutates synced state). Route-gate precedence: grant space check, then
// the write-verb check, then the chat lookup — a read-only key is refused
// before anything resolves.
func (s *Service) ensureChatWrite(ctx context.Context, spaceId, chatId string) (model.ObjectTypeLayout, error) {
	if err := s.ensureSpaceGranted(ctx, spaceId); err != nil {
		return 0, err
	}
	if err := ensureWriteGranted(ctx); err != nil {
		return 0, err
	}
	return s.ensureChat(ctx, spaceId, chatId)
}

// chatWritesBlocks says whether a message written into a chat of this
// layout is stored as BLOCKS rather than as the legacy single content. The
// two stores are asymmetric today: a space chat's messages are content
// (`message`), a discussion's are blocks beside an EMPTY content object —
// what the desktop's discussion composer writes, and its renderer reads
// blocks first. The
// product plan is to move space chats to blocks as well; when the desktop
// chat UI follows, this becomes `return true` and nothing else changes.
func chatWritesBlocks(layout model.ObjectTypeLayout) bool {
	return layout == model.ObjectType_discussion
}

// chatMessageBody is the stored form of a parsed text: one paragraph
// content for a content chat, or paragraph text blocks for a blocks chat.
// Both are what the desktop renders for the respective store, and the
// read side serves either as `text`. A blocks chat still carries an EMPTY
// content object beside its blocks — the store serializer dereferences the
// content unconditionally (chatmodel.MarshalAnyenc), and the desktop's
// discussion composer writes the same empty object.
func chatMessageBody(layout model.ObjectTypeLayout, text string, marks []*model.BlockContentTextMark) (*model.ChatMessageMessageContent, []*model.ChatMessageMessageBlock) {
	if chatWritesBlocks(layout) {
		return &model.ChatMessageMessageContent{Style: model.BlockContentText_Paragraph}, splitTextBlocks(text, marks)
	}
	return &model.ChatMessageMessageContent{
		Text:  text,
		Style: model.BlockContentText_Paragraph,
		Marks: marks,
	}, nil
}

// splitTextBlocks turns a parsed inline text into one paragraph block per
// line, the shape the desktop's discussion composer writes (one part per
// paragraph) and its renderer expects — a newline INSIDE a discussion block
// is not rendered as a break there, unlike in a space chat. Empty lines
// produce no block, so a blank-line paragraph break reads back as a single
// newline; marks are clipped to the line they fall on, with their UTF-16
// ranges rebased. An empty text yields no blocks (an attachments-only
// message).
func splitTextBlocks(text string, marks []*model.BlockContentTextMark) []*model.ChatMessageMessageBlock {
	units := textutil.StrToUTF16(text)
	var blocks []*model.ChatMessageMessageBlock
	start := 0
	for i := 0; i <= len(units); i++ {
		if i < len(units) && units[i] != '\n' {
			continue
		}
		if i > start {
			blocks = append(blocks, &model.ChatMessageMessageBlock{
				Content: &model.ChatMessageMessageBlockContentOfText{Text: &model.ChatMessageMessageBlockText{
					Text:  textutil.UTF16ToStr(units[start:i]),
					Style: model.BlockContentText_Paragraph,
					Marks: clipMarks(marks, int32(start), int32(i)),
				}},
			})
		}
		start = i + 1
	}
	return blocks
}

// clipMarks keeps the part of every mark that falls inside [from, to) and
// rebases it to start at 0; a mark entirely outside, or left empty by the
// clip, is dropped.
func clipMarks(marks []*model.BlockContentTextMark, from, to int32) []*model.BlockContentTextMark {
	var out []*model.BlockContentTextMark
	for _, mark := range marks {
		if mark == nil || mark.Range == nil || mark.Range.To <= from || mark.Range.From >= to {
			continue
		}
		clipped := *mark
		clipped.Range = &model.Range{From: max(mark.Range.From, from) - from, To: min(mark.Range.To, to) - from}
		if clipped.Range.From >= clipped.Range.To {
			continue
		}
		out = append(out, &clipped)
	}
	return out
}

// blocksEditWarnings is the C6 warning for an edit that replaces a
// discussion message's blocks with plain text: the blocks the text cannot
// express — quotes, links, embeds, styled text — are dropped, and a caller
// who merely echoed the served `text` back would not know. Served on the
// dry run and the receipt alike.
func blocksEditWarnings(blocks []*model.ChatMessageMessageBlock) []v2model.Issue {
	var lost []string
	count := func(kind string, n int) {
		if n > 0 {
			lost = append(lost, fmt.Sprintf("%d %s", n, kind))
		}
	}
	var quotes, links, embeds, styled int
	for _, block := range blocks {
		switch {
		case block == nil:
		case block.GetEditorQuote() != nil, block.GetMessageQuote() != nil:
			quotes++
		case block.GetLink() != nil:
			links++
		case block.GetEmbed() != nil:
			embeds++
		case block.GetText() != nil && block.GetText().Style != model.BlockContentText_Paragraph:
			styled++
		}
	}
	count("quote block(s)", quotes)
	count("link block(s)", links)
	count("embed block(s)", embeds)
	count("styled text block(s)", styled)
	if len(lost) == 0 {
		return nil
	}
	return []v2model.Issue{{
		Path:    "/text",
		Message: fmt.Sprintf("this edit replaces the message's blocks with the text: %s the text cannot express are dropped", strings.Join(lost, ", ")),
		Hint:    "quotes, links and embeds cannot be written through this API; keep the original message if they matter",
	}}
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
				}.Hintf("upload files via %s first, or pass an existing object id", v2model.RefUploadFile(spaceId)))
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

// v2ValidateChatBody is the C9 guard for the one emptiness the store sees
// and the request does not: in a blocks chat a text of newlines only
// produces no block (splitTextBlocks), and a message with no block and no
// attachment is refused by chatmodel.Validate — so the dry run must refuse
// it too, rather than predict a 201 the real call turns into a 400.
func v2ValidateChatBody(layout model.ObjectTypeLayout, blocks []*model.ChatMessageMessageBlock, attachments int) error {
	if chatWritesBlocks(layout) && len(blocks) == 0 && attachments == 0 {
		return v2model.ValidationFailed("a message needs text or attachments",
			v2model.Issue{Path: "/text", Message: "the text has no non-empty line, and there are no attachments"})
	}
	return nil
}

// v2ChatDeleteWarnings surfaces the irreversible side effect of a message
// delete: the middleware garbage-collects attachment and link-block targets
// orphaned by the delete with skipBin=true — permanently deleted, not
// binned, asynchronously AFTER the API has replied. The dry run and the
// real receipt both carry the ids at risk (C6 warnings): the attachments,
// and the targets of link blocks (a desktop discussion post's files ride
// there rather than in attachments).
func v2ChatDeleteWarnings(msg *model.ChatMessage) []v2model.Issue {
	var ids []string
	for _, att := range msg.Attachments {
		if att != nil && att.Target != "" {
			ids = append(ids, att.Target)
		}
	}
	for _, block := range msg.Blocks {
		if link := block.GetLink(); link != nil && link.TargetObjectId != "" {
			ids = append(ids, link.TargetObjectId)
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
	case strings.Contains(description, list.ErrInsufficientPermissions.Error()):
		// the space ACL refused the write (the account is a reader there):
		// a permanent refusal for this key, not a server failure
		return v2model.NewError(http.StatusForbidden, v2model.CodeForbidden,
			fmt.Sprintf("%s: %s — this account cannot write in the space; ask the owner for edit rights", op, description))
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

// chatStateAfter is the chat's state once a watermark moved — the receipt
// of a write nothing else made observable (round-four eval R4-8: a read
// receipt of `{}` was indistinguishable from a no-op). Best effort: a read
// failure leaves it out rather than failing the write that succeeded.
func (s *Service) chatStateAfter(ctx context.Context, chatId string) *v2model.ChatState {
	resp := s.mw.ChatGetMessages(ctx, &pb.RpcChatGetMessagesRequest{ChatObjectId: chatId, Limit: 1})
	if resp == nil || (resp.Error != nil && resp.Error.Code != pb.RpcChatGetMessagesResponseError_NULL) {
		return nil
	}
	return v2model.ChatStateFromProto(resp.ChatState)
}
