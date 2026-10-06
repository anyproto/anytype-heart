package v2model

// chat.go holds the chat DTOs and the markup bridge: message text crosses
// the API as markdown source in BOTH directions (the anyblockjson inline
// codec — one vocabulary with block text, C2 — plus code fences for
// multi-line code marks); offset mark arrays never cross the API.
// Reactions are counts ({"👍":2}, Q4); ?reactions=full adds reacted_by
// (participant-id lists) in its own slot so neither field ever changes
// type.

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	textutil "github.com/anyproto/anytype-heart/util/text"
)

// Chat row kinds.
const (
	ChatKindChat       = "chat"
	ChatKindDiscussion = "discussion"
)

// list_chats ?unread= values.
const (
	ChatUnreadMessages = "messages"
	ChatUnreadMentions = "mentions"
)

// ChatRow is the C5 chat list row. The unread counters are the chat state
// manager's, read per row without opening the chat. For a discussion, Id is the
// discussion's own id (usable on every chat route) and ParentId the object it belongs to.
type ChatRow struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
	// True for the space's main chat (the space chat every member shares); false for every other chat and for every discussion.
	IsMain         bool   `json:"is_main"`
	ParentId       string `json:"parent_id,omitempty"`
	UnreadMessages int    `json:"unread_messages"`
	UnreadMentions int    `json:"unread_mentions"`
}

// ChatMessage is one chat message. Text is §8 inline markup rendered by
// the anyblockjson inline codec — the same serialization block text uses.
// AuthorId is the deterministic participant id; Author is the display name
// when the participant is known. At/EditedAt are RFC 3339 UTC — one date
// shape across v2 (C2), matching every AnyBlock date. Reactions is ALWAYS
// the counts map; ReactedBy (participant-id lists) appears only under
// ?reactions=full — two slots so neither ever changes type (C2).
// A message is stored either as content (a space chat) or as blocks (a
// discussion, or a desktop chat post with quotes); Text is what the message
// SAYS whichever store holds it: the content's text when there is one,
// otherwise the text-bearing blocks rendered and newline-joined. BlocksText
// carries the blocks rendering only when a message has BOTH — legacy
// desktop posts — so nothing is served twice and a blocks-only message,
// the API's own discussion posts included, reads back as text.
type ChatMessage struct {
	Id string `json:"id"`
	// Chat state id stamped once, when the message was stored locally; ids sort in that order. The message stream sends it as the event id, so it is the checkpoint a client resumes from (Last-Event-ID). An edit does not restamp it.
	StateId     string              `json:"state_id,omitempty"`
	Order       string              `json:"order"`
	Author      string              `json:"author,omitempty"`
	AuthorId    string              `json:"author_id,omitempty"`
	At          string              `json:"at,omitempty"`
	EditedAt    string              `json:"edited_at,omitempty"`
	Text        string              `json:"text"`
	BlocksText  string              `json:"blocks_text,omitempty"`
	ReplyTo     string              `json:"reply_to,omitempty"`
	Reactions   map[string]int      `json:"reactions,omitempty"`
	ReactedBy   map[string][]string `json:"reacted_by,omitempty"`
	Attachments []ChatAttachment    `json:"attachments,omitempty"`
	Pinned      bool                `json:"pinned,omitempty"`
}

// ChatAttachment is one message attachment: the target object id and its
// kind (file, image, link).
type ChatAttachment struct {
	Id   string `json:"id"`
	Type string `json:"type"`
}

// ChatState is the model.ChatState passthrough the v1 DTO dropped: the
// poll peek (unread counters) and the mark-read race guard (last_state_id —
// POST read forwards it).
type ChatState struct {
	UnreadMessages           int    `json:"unread_messages"`
	UnreadMentions           int    `json:"unread_mentions"`
	OldestUnreadOrder        string `json:"oldest_unread_order,omitempty"`
	OldestUnreadMentionOrder string `json:"oldest_unread_mention_order,omitempty"`
	UnreadReactionOrder      string `json:"unread_reaction_order,omitempty"`
	LastStateId              string `json:"last_state_id,omitempty"`
}

// ChatMessagesResponse is the GET messages payload: ascending-order
// messages plus the state+message_count the underlying RPC already returns
// at zero extra cost. A poll is a limit=1 read of this
// shape. Cursor-paged (after/before order ids), not C10 offset pagination.
// MessageCount is the number of messages the chat HOLDS (a deleted one is
// gone from it), not the size of the requested range; LifetimeMessageCount
// counts every message ever posted, deleted ones included. HasMore says
// more messages exist inside the requested bounds, and NextAfter/NextBefore
// carry the boundary order id to continue from — forward walks (?after
// alone) get NextAfter, everything else (newest-anchored) gets NextBefore.
type ChatMessagesResponse struct {
	Messages []ChatMessage `json:"messages"`
	State    *ChatState    `json:"state,omitempty"`
	// Messages the chat holds now; a deleted message leaves it
	MessageCount int `json:"message_count"`
	// Messages ever posted, deleted ones included
	LifetimeMessageCount int    `json:"lifetime_message_count"`
	HasMore              bool   `json:"has_more"` // more messages inside the requested bounds, not in the chat as a whole
	NextAfter            string `json:"next_after,omitempty"`
	NextBefore           string `json:"next_before,omitempty"`
}

// CreateChatRequest is the POST chats body.
type CreateChatRequest struct {
	Name string `json:"name"`
}

// ChatResult is the POST chats response: the created chat as a C5 row.
type ChatResult struct {
	Id     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	DryRun bool   `json:"dry_run,omitempty"`
}

// DiscussionResult is the POST objects/{object_id}/discussion response: the
// id of the object's discussion, which is a chat id for every chat
// operation. Created says whether THIS call minted it — false when the
// object already had one and the same id is returned — so a caller who
// cannot see the status code (201 vs 200) still knows. On a dry run Created
// is the would-be outcome and Id is set only when a discussion exists.
type DiscussionResult struct {
	Id      string `json:"id,omitempty"`
	Created bool   `json:"created,omitempty"`
	DryRun  bool   `json:"dry_run,omitempty"`
}

// AddChatMessageRequest is the POST messages body. Text is markdown
// SOURCE (the D′1 caveat applies: *, [ and mention syntax mint real marks,
// in a space chat so do heading lines and code fences, and in a discussion
// all block syntax becomes styled blocks).
// Attachments are bare object ids — the attachment kind is inferred from
// each target's layout (image → image, other file layouts → file, anything
// else → link).
type AddChatMessageRequest struct {
	Text        string   `json:"text"`
	ReplyTo     string   `json:"reply_to,omitempty"`
	Attachments []string `json:"attachments,omitempty"`
}

// EditChatMessageRequest is the PATCH message body: a text-only merge —
// the message's attachments, reply target and style are preserved.
type EditChatMessageRequest struct {
	Text string `json:"text"`
}

// ChatMessageResult is the mutation response for message create/edit/
// delete. C8: the id is always returned on create.
type ChatMessageResult struct {
	Id       string  `json:"id,omitempty"`
	DryRun   bool    `json:"dry_run,omitempty"`
	Warnings []Issue `json:"warnings,omitempty"`
}

// ChatReactionRequest is the POST reactions body.
type ChatReactionRequest struct {
	Emoji string `json:"emoji"`
}

// ChatReactionResult reports the toggle outcome. On a dry run, Added is
// the would-be outcome (computed from the caller's current reaction) — and
// is OMITTED, with a warning, when the service has no account identity to
// predict with (asserting a coin-flip value would be wrong half the time).
type ChatReactionResult struct {
	Added    *bool   `json:"added,omitempty"`
	DryRun   bool    `json:"dry_run,omitempty"`
	Warnings []Issue `json:"warnings,omitempty"`
}

// ChatReadRequest is the POST read body. UpTo is an order id, INCLUSIVE,
// required for the messages/mentions scopes (an empty bound would silently
// mark nothing — the v1 read_all trap). LastStateId is EQUALLY required for
// those scopes: the repository ANDs `stateId <= last_state_id` and every
// stored message carries a non-empty state id, so an empty guard marks
// nothing just as silently. Both ride the same GET messages response
// (newest order + state.last_state_id). Scope defaults to "messages";
// "reactions" marks all unread reactions and takes no UpTo/LastStateId
// (the backend reads all).
type ChatReadRequest struct {
	UpTo        string `json:"up_to,omitempty"`
	LastStateId string `json:"last_state_id,omitempty"`
	Scope       string `json:"scope,omitempty"`
}

// ChatReadResult acknowledges a read watermark move.
// ChatReadResult is the read receipt: the chat's state after the move, when
// it could be read back (best effort — the write itself has succeeded).
// Deliberately no comment on the field: swag would lift it onto the shared
// ChatState schema, where "after the move" is false on an ordinary read.
type ChatReadResult struct {
	DryRun bool       `json:"dry_run,omitempty"`
	State  *ChatState `json:"state,omitempty"`
}

// Read scopes (ChatReadRequest.Scope).
const (
	ChatReadScopeMessages  = "messages"
	ChatReadScopeMentions  = "mentions"
	ChatReadScopeReactions = "reactions"
)

// Reactions render modes (?reactions= on the messages read).
const (
	ReactionsCounts = "counts"
	ReactionsFull   = "full"
)

//
// ---- proto → DTO conversion (the inline-markup bridge, read side) ----
//

// ChatMessageOptions parameterizes the proto→DTO conversion.
type ChatMessageOptions struct {
	SpaceId string
	// FullReactions switches reactions from counts to identity lists
	// (participant ids).
	FullReactions bool
	// ParticipantName resolves a participant id to a display name; nil or
	// an empty result leaves Author unset.
	ParticipantName func(participantId string) string
}

// ChatMessageFromProto converts one middleware message into the v2 DTO:
// marks render into the text via the anyblockjson inline codec (§8 markup,
// C2 — offset arrays never cross the API), the raw creator identity becomes
// the deterministic participant id, and reactions compact to counts unless
// FullReactions is set.
func ChatMessageFromProto(msg *model.ChatMessage, opts ChatMessageOptions) ChatMessage {
	if msg == nil {
		return ChatMessage{}
	}
	out := ChatMessage{
		Id:      msg.Id,
		StateId: msg.StateId,
		Order:   msg.OrderId,
		At:      chatTime(msg.CreatedAt),
		ReplyTo: msg.ReplyToMessageId,
		Pinned:  msg.Pinned,
	}
	if msg.ModifiedAt != 0 && msg.ModifiedAt != msg.CreatedAt {
		out.EditedAt = chatTime(msg.ModifiedAt)
	}
	if msg.Creator != "" {
		out.AuthorId = domain.NewParticipantId(opts.SpaceId, msg.Creator)
		if opts.ParticipantName != nil {
			out.Author = opts.ParticipantName(out.AuthorId)
		}
	}
	if msg.Message != nil {
		out.Text = renderChatContent(msg.Message.Text, msg.Message.Marks)
	}
	if rendered := blocksText(msg.Blocks); rendered != "" {
		if out.Text == "" {
			out.Text = rendered
		} else {
			out.BlocksText = rendered
		}
	}
	for _, att := range msg.Attachments {
		if att == nil {
			continue
		}
		out.Attachments = append(out.Attachments, ChatAttachment{
			Id:   att.Target,
			Type: attachmentTypeToString(att.Type),
		})
	}
	out.Reactions, out.ReactedBy = reactionsFromProto(msg.Reactions, opts)
	return out
}

// chatTime renders a unix-seconds timestamp as RFC 3339 UTC — the one
// date shape v2 uses everywhere (AnyBlock dates, search filters, file
// addedAt); an epoch int here would force agents into epoch arithmetic.
func chatTime(sec int64) string {
	if sec == 0 {
		return ""
	}
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}

// renderChatContent renders a message's content as markdown that posts back
// as the same content: §8 markup, except that a multi-line code mark over
// whole lines — how a ``` fence is stored, by the desktop composer and the
// API alike — reads back as a fence (the fence outgrows any backtick run in
// the code), and prose a post would read as a heading or fence is escaped.
func renderChatContent(text string, marks []*model.BlockContentTextMark) string {
	units := textutil.StrToUTF16(text)
	var out strings.Builder
	cursor := int32(0)
	for _, fence := range fencedCodeRanges(units, marks) {
		out.WriteString(renderProseRange(units, marks, cursor, fence.From))
		code := textutil.UTF16ToStr(units[fence.From:fence.To])
		marker := codeFence(code)
		out.WriteString(marker + "\n" + code + "\n" + marker)
		cursor = fence.To
	}
	out.WriteString(renderProseRange(units, marks, cursor, int32(len(units))))
	return out.String()
}

// renderProseRange renders units[from:to] as §8 markup with the lines a
// space chat post would read as a heading or fence escaped.
func renderProseRange(units []uint16, marks []*model.BlockContentTextMark, from, to int32) string {
	rendered := renderInlineRange(units, marks, from, to)
	return escapeProse(rendered, proseLineStarts(units, marks, from, to), spaceChatSyntax)
}

// proseLineStarts says, for each line of units[from:to], whether it starts a
// line of prose: it starts a line of the whole text, and no code span covers
// or opens at its start — a backslash there would be literal code, or would
// break the span's delimiter.
func proseLineStarts(units []uint16, marks []*model.BlockContentTextMark, from, to int32) []bool {
	starts := []bool{isProseLineStart(units, marks, from)}
	for i := from; i < to; i++ {
		if units[i] == '\n' {
			starts = append(starts, isProseLineStart(units, marks, i+1))
		}
	}
	return starts
}

func isProseLineStart(units []uint16, marks []*model.BlockContentTextMark, at int32) bool {
	if at > 0 && units[at-1] != '\n' {
		return false
	}
	for _, mark := range marks {
		if mark != nil && mark.Range != nil && mark.Type == model.BlockContentTextMark_Keyboard &&
			mark.Range.From <= at && at < mark.Range.To {
			return false
		}
	}
	return true
}

// fencedCodeRanges picks the code marks that read back as fences: over
// whole lines, and either spanning a newline or holding a run of two
// backticks (inline, such code would need a ``` delimiter, which a post
// reads as a fence opener); in order, none overlapping.
func fencedCodeRanges(units []uint16, marks []*model.BlockContentTextMark) []model.Range {
	var ranges []model.Range
	for _, mark := range marks {
		if mark == nil || mark.Range == nil || mark.Type != model.BlockContentTextMark_Keyboard {
			continue
		}
		from, to := mark.Range.From, mark.Range.To
		if from < 0 || to > int32(len(units)) || from >= to {
			continue
		}
		startsLine := from == 0 || units[from-1] == '\n'
		endsLine := to == int32(len(units)) || units[to] == '\n'
		multiLine := slices.Contains(units[from:to], '\n')
		if startsLine && endsLine && (multiLine || longestRun(textutil.UTF16ToStr(units[from:to]), '`') >= 2) {
			ranges = append(ranges, model.Range{From: from, To: to})
		}
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].From < ranges[j].From })
	kept := ranges[:0]
	for _, r := range ranges {
		if len(kept) == 0 || r.From >= kept[len(kept)-1].To {
			kept = append(kept, r)
		}
	}
	return kept
}

// renderInlineRange renders units[from:to] with the marks clipped to it and
// rebased.
func renderInlineRange(units []uint16, marks []*model.BlockContentTextMark, from, to int32) string {
	if from >= to {
		return ""
	}
	var clipped []*model.BlockContentTextMark
	for _, mark := range marks {
		if mark == nil || mark.Range == nil || mark.Range.To <= from || mark.Range.From >= to {
			continue
		}
		part := *mark
		part.Range = &model.Range{From: max(mark.Range.From, from) - from, To: min(mark.Range.To, to) - from}
		if part.Range.From < part.Range.To {
			clipped = append(clipped, &part)
		}
	}
	return anyblockjson.RenderInlineText(textutil.UTF16ToStr(units[from:to]), clipped)
}

func longestRun(s string, c rune) int {
	longest, current := 0, 0
	for _, r := range s {
		if r == c {
			current++
			longest = max(longest, current)
		} else {
			current = 0
		}
	}
	return longest
}

// blocksText renders a message's text-bearing blocks (text blocks and the
// contents of editor/message quotes) as markdown, newline-joined: a text
// block's style becomes the line syntax that posts it back (# headings,
// list items, checkboxes, > quotes, fenced code), and a run of numbered
// items is numbered from 1. Chat messages composed of blocks (a discussion
// post, desktop quotes, rich pastes) are valid with empty content
// (chatmodel.Validate) — without this they would read back as empty
// messages. Link and embed blocks have no text and stay invisible here.
func blocksText(blocks []*model.ChatMessageMessageBlock) string {
	var parts []string
	appendText := func(tb *model.ChatMessageMessageBlockText) {
		if tb != nil && tb.Text != "" {
			parts = append(parts, anyblockjson.RenderInlineText(tb.Text, tb.Marks))
		}
	}
	number := 0
	for _, block := range blocks {
		if block == nil {
			continue
		}
		if tb := block.GetText(); tb != nil && tb.Style == model.BlockContentText_Numbered {
			number++
		} else {
			number = 0
		}
		switch {
		case block.GetText() != nil:
			if line := renderTextBlock(block.GetText(), number); line != "" {
				parts = append(parts, line)
			}
		case block.GetEditorQuote() != nil:
			appendText(block.GetEditorQuote().Content)
		case block.GetMessageQuote() != nil:
			appendText(block.GetMessageQuote().Content)
		}
	}
	return strings.Join(parts, "\n")
}

// renderTextBlock renders one text block as markdown; number is its place
// in a run of numbered items.
func renderTextBlock(tb *model.ChatMessageMessageBlockText, number int) string {
	if tb.Text == "" {
		return ""
	}
	if tb.Style == model.BlockContentText_Code {
		marker := codeFence(tb.Text)
		return marker + tb.Lang + "\n" + tb.Text + "\n" + marker
	}
	if tb.Style == model.BlockContentText_Paragraph {
		return renderParagraph(tb)
	}
	text := anyblockjson.RenderInlineText(tb.Text, tb.Marks)
	switch tb.Style {
	case model.BlockContentText_Header1:
		return "# " + text
	case model.BlockContentText_Header2:
		return "## " + text
	case model.BlockContentText_Header3:
		return "### " + text
	case model.BlockContentText_Marked:
		return "- " + text
	case model.BlockContentText_Numbered:
		return fmt.Sprintf("%d. %s", number, text)
	case model.BlockContentText_Checkbox:
		if tb.Checked {
			return "- [x] " + text
		}
		return "- [ ] " + text
	case model.BlockContentText_Quote:
		return "> " + strings.ReplaceAll(text, "\n", "\n> ")
	}
	return text
}

// renderParagraph renders a paragraph block with every line a post would
// read as block syntax escaped — except the "---" paragraph, which is how
// the desktop stores a divider and how a divider posts.
func renderParagraph(tb *model.ChatMessageMessageBlockText) string {
	if tb.Text == "---" && len(tb.Marks) == 0 {
		return tb.Text
	}
	units := textutil.StrToUTF16(tb.Text)
	starts := proseLineStarts(units, tb.Marks, 0, int32(len(units)))
	return escapeProse(anyblockjson.RenderInlineText(tb.Text, tb.Marks), starts, func(string) bool { return true })
}

// codeFence is a backtick fence longer than any backtick run in the code,
// so the code cannot close it.
func codeFence(code string) string {
	return strings.Repeat("`", max(3, longestRun(code, '`')+1))
}

// reactionsFromProto compacts reactions to counts (always — the stable
// slot) and, in full mode, additionally maps the raw identities to
// participant ids for reacted_by (one vocabulary with AuthorId, C2). Two
// return slots so neither JSON field ever changes type.
func reactionsFromProto(reactions *model.ChatMessageReactions, opts ChatMessageOptions) (map[string]int, map[string][]string) {
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

// ChatStateFromProto converts the middleware chat state — the passthrough
// v1 dropped.
func ChatStateFromProto(state *model.ChatState) *ChatState {
	if state == nil {
		return nil
	}
	out := &ChatState{
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

var attachmentTypeMap = map[model.ChatMessageAttachmentAttachmentType]string{
	model.ChatMessageAttachment_FILE:  "file",
	model.ChatMessageAttachment_IMAGE: "image",
	model.ChatMessageAttachment_LINK:  "link",
}

func attachmentTypeToString(t model.ChatMessageAttachmentAttachmentType) string {
	if s, ok := attachmentTypeMap[t]; ok {
		return s
	}
	return "file"
}
