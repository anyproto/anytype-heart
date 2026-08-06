package v2model

// chat.go holds the Phase-6 chat DTOs (APIV2.md §8.7, APIV2_SURFACES.md
// §5) and the inline-markup bridge: message text crosses the API as SPEC §8
// markup source in BOTH directions (the anyblockjson inline codec — one
// vocabulary with block text, C2); offset mark arrays never cross the API.
// Reactions are counts ({"👍":2}, Q4); ?reactions=full adds reactedBy
// (participant-id lists) in its own slot so neither field ever changes
// type. The SSE stream (Phase 8) reuses these DTOs.

import (
	"strings"
	"time"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// V2ChatRow is the C5 chat list row. Deliberately counter-free (Q3):
// per-chat unread state comes free on the messages read, while computing
// list-wide counters would open every chat — the GO-7302 startup cost.
type V2ChatRow struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

// V2ChatMessage is one chat message. Text is §8 inline markup rendered by
// the anyblockjson inline codec — the same serialization block text uses.
// AuthorId is the deterministic participant id; Author is the display name
// when the participant is known. At/EditedAt are RFC 3339 UTC — one date
// shape across v2 (C2), matching every AnyBlock date. Reactions is ALWAYS
// the counts map; ReactedBy (participant-id lists) appears only under
// ?reactions=full — two slots so neither ever changes type (C2).
// BlocksText is the read-only rendering of a block-composed message's
// text-bearing blocks (desktop quotes etc.) — without it a blocks-only
// message would read back as empty.
type V2ChatMessage struct {
	Id          string              `json:"id"`
	Order       string              `json:"order"`
	Author      string              `json:"author,omitempty"`
	AuthorId    string              `json:"authorId,omitempty"`
	At          string              `json:"at,omitempty"`
	EditedAt    string              `json:"editedAt,omitempty"`
	Text        string              `json:"text"`
	BlocksText  string              `json:"blocksText,omitempty"`
	ReplyTo     string              `json:"replyTo,omitempty"`
	Reactions   map[string]int      `json:"reactions,omitempty"`
	ReactedBy   map[string][]string `json:"reactedBy,omitempty"`
	Attachments []V2ChatAttachment  `json:"attachments,omitempty"`
	Pinned      bool                `json:"pinned,omitempty"`
}

// V2ChatAttachment is one message attachment: the target object id and its
// kind (file, image, link).
type V2ChatAttachment struct {
	Id   string `json:"id"`
	Type string `json:"type"`
}

// V2ChatState is the model.ChatState passthrough the v1 DTO dropped: the
// poll peek (unread counters) and the mark-read race guard (lastStateId —
// POST read forwards it).
type V2ChatState struct {
	UnreadMessages           int    `json:"unreadMessages"`
	UnreadMentions           int    `json:"unreadMentions"`
	OldestUnreadOrder        string `json:"oldestUnreadOrder,omitempty"`
	OldestUnreadMentionOrder string `json:"oldestUnreadMentionOrder,omitempty"`
	UnreadReactionOrder      string `json:"unreadReactionOrder,omitempty"`
	LastStateId              string `json:"lastStateId,omitempty"`
}

// V2ChatMessagesResponse is the GET messages payload: ascending-order
// messages plus the state+messageCount the underlying RPC already returns
// (zero extra cost — the Phase-6 finding). A poll is a limit=1 read of this
// shape. Cursor-paged (after/before order ids), not C10 offset pagination.
// MessageCount is the chat's LIFETIME total, not the size of the requested
// range; HasMore says more messages exist inside the requested bounds, and
// NextAfter/NextBefore carry the boundary order id to continue from —
// forward walks (?after alone) get NextAfter, everything else (newest-
// anchored) gets NextBefore.
type V2ChatMessagesResponse struct {
	Messages     []V2ChatMessage `json:"messages"`
	State        *V2ChatState    `json:"state,omitempty"`
	MessageCount int             `json:"messageCount"`
	HasMore      bool            `json:"has_more"`
	NextAfter    string          `json:"nextAfter,omitempty"`
	NextBefore   string          `json:"nextBefore,omitempty"`
}

// V2CreateChatRequest is the POST chats body.
type V2CreateChatRequest struct {
	Name string `json:"name"`
}

// V2ChatResult is the POST chats response: the created chat as a C5 row.
type V2ChatResult struct {
	Id     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	DryRun bool   `json:"dry_run,omitempty"`
}

// V2AddChatMessageRequest is the POST messages body. Text is §8 markup
// SOURCE (the D′1 caveat applies: *, [ and mention syntax mint real marks).
// Attachments are bare object ids — the attachment kind is inferred from
// each target's layout (image → image, other file layouts → file, anything
// else → link).
type V2AddChatMessageRequest struct {
	Text        string   `json:"text"`
	ReplyTo     string   `json:"replyTo,omitempty"`
	Attachments []string `json:"attachments,omitempty"`
}

// V2EditChatMessageRequest is the PATCH message body: a text-only merge —
// the message's attachments, reply target and style are preserved.
type V2EditChatMessageRequest struct {
	Text string `json:"text"`
}

// V2ChatMessageResult is the mutation response for message create/edit/
// delete. C8: the id is always returned on create.
type V2ChatMessageResult struct {
	Id       string    `json:"id,omitempty"`
	DryRun   bool      `json:"dry_run,omitempty"`
	Warnings []V2Issue `json:"warnings,omitempty"`
}

// V2ChatReactionRequest is the POST reactions body.
type V2ChatReactionRequest struct {
	Emoji string `json:"emoji"`
}

// V2ChatReactionResult reports the toggle outcome. On a dry run, Added is
// the would-be outcome (computed from the caller's current reaction) — and
// is OMITTED, with a warning, when the service has no account identity to
// predict with (asserting a coin-flip value would be wrong half the time).
type V2ChatReactionResult struct {
	Added    *bool     `json:"added,omitempty"`
	DryRun   bool      `json:"dry_run,omitempty"`
	Warnings []V2Issue `json:"warnings,omitempty"`
}

// V2ChatReadRequest is the POST read body. UpTo is an order id, INCLUSIVE,
// required for the messages/mentions scopes (an empty bound would silently
// mark nothing — the v1 read_all trap). LastStateId is EQUALLY required for
// those scopes: the repository ANDs `stateId <= lastStateId` and every
// stored message carries a non-empty state id, so an empty guard marks
// nothing just as silently. Both ride the same GET messages response
// (newest order + state.lastStateId). Scope defaults to "messages";
// "reactions" marks all unread reactions and takes no UpTo/LastStateId
// (the backend reads all).
type V2ChatReadRequest struct {
	UpTo        string `json:"upTo,omitempty"`
	LastStateId string `json:"lastStateId,omitempty"`
	Scope       string `json:"scope,omitempty"`
}

// V2ChatReadResult acknowledges a read watermark move.
type V2ChatReadResult struct {
	DryRun bool `json:"dry_run,omitempty"`
}

// Read scopes (V2ChatReadRequest.Scope).
const (
	V2ChatReadScopeMessages  = "messages"
	V2ChatReadScopeMentions  = "mentions"
	V2ChatReadScopeReactions = "reactions"
)

// Reactions render modes (?reactions= on the messages read).
const (
	V2ReactionsCounts = "counts"
	V2ReactionsFull   = "full"
)

//
// ---- proto → DTO conversion (the inline-markup bridge, read side) ----
//

// V2ChatMessageOptions parameterizes the proto→DTO conversion.
type V2ChatMessageOptions struct {
	SpaceId string
	// FullReactions switches reactions from counts to identity lists
	// (participant ids).
	FullReactions bool
	// ParticipantName resolves a participant id to a display name; nil or
	// an empty result leaves Author unset.
	ParticipantName func(participantId string) string
}

// V2ChatMessageFromProto converts one middleware message into the v2 DTO:
// marks render into the text via the anyblockjson inline codec (§8 markup,
// C2 — offset arrays never cross the API), the raw creator identity becomes
// the deterministic participant id, and reactions compact to counts unless
// FullReactions is set.
func V2ChatMessageFromProto(msg *model.ChatMessage, opts V2ChatMessageOptions) V2ChatMessage {
	if msg == nil {
		return V2ChatMessage{}
	}
	out := V2ChatMessage{
		Id:      msg.Id,
		Order:   msg.OrderId,
		At:      v2ChatTime(msg.CreatedAt),
		ReplyTo: msg.ReplyToMessageId,
		Pinned:  msg.Pinned,
	}
	if msg.ModifiedAt != 0 && msg.ModifiedAt != msg.CreatedAt {
		out.EditedAt = v2ChatTime(msg.ModifiedAt)
	}
	if msg.Creator != "" {
		out.AuthorId = domain.NewParticipantId(opts.SpaceId, msg.Creator)
		if opts.ParticipantName != nil {
			out.Author = opts.ParticipantName(out.AuthorId)
		}
	}
	if msg.Message != nil {
		out.Text = anyblockjson.RenderInlineText(msg.Message.Text, msg.Message.Marks)
	}
	out.BlocksText = v2BlocksText(msg.Blocks)
	for _, att := range msg.Attachments {
		if att == nil {
			continue
		}
		out.Attachments = append(out.Attachments, V2ChatAttachment{
			Id:   att.Target,
			Type: v2AttachmentTypeToString(att.Type),
		})
	}
	out.Reactions, out.ReactedBy = v2ReactionsFromProto(msg.Reactions, opts)
	return out
}

// v2ChatTime renders a unix-seconds timestamp as RFC 3339 UTC — the one
// date shape v2 uses everywhere (AnyBlock dates, search filters, file
// addedAt); an epoch int here would force agents into epoch arithmetic.
func v2ChatTime(sec int64) string {
	if sec == 0 {
		return ""
	}
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}

// v2BlocksText renders a message's text-bearing blocks (text blocks and the
// contents of editor/message quotes) as §8 markup, newline-joined. Chat
// messages composed of blocks (desktop quotes, rich pastes) are valid with
// empty text (chatmodel.Validate) — without this field they would read back
// as empty messages. Read-only: PATCH preserves blocks untouched.
func v2BlocksText(blocks []*model.ChatMessageMessageBlock) string {
	var parts []string
	appendText := func(tb *model.ChatMessageMessageBlockText) {
		if tb != nil && tb.Text != "" {
			parts = append(parts, anyblockjson.RenderInlineText(tb.Text, tb.Marks))
		}
	}
	for _, block := range blocks {
		if block == nil {
			continue
		}
		switch {
		case block.GetText() != nil:
			appendText(block.GetText())
		case block.GetEditorQuote() != nil:
			appendText(block.GetEditorQuote().Content)
		case block.GetMessageQuote() != nil:
			appendText(block.GetMessageQuote().Content)
		}
	}
	return strings.Join(parts, "\n")
}

// v2ReactionsFromProto compacts reactions to counts (always — the stable
// slot) and, in full mode, additionally maps the raw identities to
// participant ids for reactedBy (one vocabulary with AuthorId, C2). Two
// return slots so neither JSON field ever changes type.
func v2ReactionsFromProto(reactions *model.ChatMessageReactions, opts V2ChatMessageOptions) (map[string]int, map[string][]string) {
	if reactions == nil || len(reactions.Reactions) == 0 {
		return nil, nil
	}
	counts := make(map[string]int, len(reactions.Reactions))
	var full map[string][]string
	if opts.FullReactions {
		full = make(map[string][]string, len(reactions.Reactions))
	}
	for emoji, identityList := range reactions.Reactions {
		if identityList == nil {
			continue
		}
		counts[emoji] = len(identityList.Ids)
		if full != nil {
			ids := make([]string, 0, len(identityList.Ids))
			for _, identity := range identityList.Ids {
				ids = append(ids, domain.NewParticipantId(opts.SpaceId, identity))
			}
			full[emoji] = ids
		}
	}
	return counts, full
}

// V2ChatStateFromProto converts the middleware chat state — the passthrough
// v1 dropped.
func V2ChatStateFromProto(state *model.ChatState) *V2ChatState {
	if state == nil {
		return nil
	}
	out := &V2ChatState{
		LastStateId:         state.LastStateId,
		UnreadReactionOrder: state.UnreadReactionOrderId,
	}
	if state.Messages != nil {
		out.UnreadMessages = int(state.Messages.Counter)
		out.OldestUnreadOrder = state.Messages.OldestOrderId
	}
	if state.Mentions != nil {
		out.UnreadMentions = int(state.Mentions.Counter)
		out.OldestUnreadMentionOrder = state.Mentions.OldestOrderId
	}
	return out
}

var v2AttachmentTypeMap = map[model.ChatMessageAttachmentAttachmentType]string{
	model.ChatMessageAttachment_FILE:  "file",
	model.ChatMessageAttachment_IMAGE: "image",
	model.ChatMessageAttachment_LINK:  "link",
}

func v2AttachmentTypeToString(t model.ChatMessageAttachmentAttachmentType) string {
	if s, ok := v2AttachmentTypeMap[t]; ok {
		return s
	}
	return "file"
}
