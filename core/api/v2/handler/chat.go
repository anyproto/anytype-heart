package v2handler

// chat.go holds the Phase-6 chat handlers (APIV2.md §8.7). Message text
// is §8 inline markup in BOTH directions (the D′1 caveat applies verbatim:
// find/replace-style specials mint real marks). C7 etag/If-Match does NOT
// apply to chats — order ids and last_state_id are the stream's native
// concurrency vocabulary; this is a deliberate, documented exemption like
// search's C8/C9 one. All mutations honor Idempotency-Key (C8) and
// ?dry_run=true (C9).

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// maxChatRequestBody caps chat mutation bodies. A message is text (≤ the
// middleware's message-length cap) plus a bounded attachment list — 1 MiB
// is orders of magnitude above any legitimate body.
const maxChatRequestBody = 1 << 20 // 1 MiB

// decodeChatBody decodes a chat request body strictly (unknown fields are
// 400s with the field named — C13's spirit at the request layer, matching
// search). A false return means the error response was already written.
func decodeChatBody(c *gin.Context, into any, hint string) bool {
	return decodeStrictJSONBody(c, into, hint, maxChatRequestBody, "chat")
}

// respondChatMutation writes a chat mutation result: createdStatus on a real
// mutation, 200 on dry runs.
func respondChatMutation(c *gin.Context, dryRun bool, createdStatus int, payload any) {
	status := createdStatus
	if dryRun {
		status = http.StatusOK
	}
	c.JSON(status, payload)
}

// ListChatsHandler lists the space's chats as C5 rows
//
//	@Summary		List chats
//	@Description	C5 rows {id, name} via a store query — no chat opens, so the list is cheap at any size. Deliberately counter-free (Q3): per-chat unread state comes free on the messages read. No etag (C7 exemption: chats use order ids and last_state_id as their concurrency vocabulary).
//	@Id				list_chats
//	@Tags			Chat
//	@Produce		json
//	@Param			space_id	path		string									true	"Space id"
//	@Param			offset		query		int										false	"Rows to skip"		default(0)
//	@Param			limit		query		int										false	"Rows to return"	default(25)
//	@Success		200			{object}	v2model.ListResponse[v2model.ChatRow]	"Chat rows"
//	@Failure		404			{object}	v2model.Error							"Space not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/chats [get]
func ListChatsHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, err := s.ListChats(c.Request.Context(), c.Param("space_id"), offset, limit)
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, v2model.NewListResponse(rows, total, offset, limit, hasMore,
			"request the next offset"))
	}
}

// CreateChatHandler creates a chat
//
//	@Summary		Create chat
//	@Description	Creates a chat object: {name}. A thin create with the chat_derived type — messages live in the chat store, not blocks. Honors Idempotency-Key (C8) and ?dry_run=true (C9). No If-Match (C7 exemption).
//	@Id				create_chat
//	@Tags			Chat
//	@Accept			json
//	@Produce		json
//	@Param			space_id		path		string						true	"Space id"
//	@Param			dry_run			query		bool						false	"Validate and report without committing"
//	@Param			Idempotency-Key	header		string						false	"C8 replay guard: the same key with the same body replays the stored response"
//	@Param			request			body		v2model.CreateChatRequest	true	"The chat to create"
//	@Success		201				{object}	v2model.ChatResult			"Created chat row"
//	@Failure		400				{object}	v2model.Error				"Validation failure"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/chats [post]
func CreateChatHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req v2model.CreateChatRequest
		if !decodeChatBody(c, &req, "the chat body takes name") {
			return
		}
		result, err := s.CreateChat(c.Request.Context(), c.Param("space_id"), req, isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondChatMutation(c, result.DryRun, http.StatusCreated, result)
	}
}

// GetChatMessagesHandler reads messages with the state passthrough
//
//	@Summary		Get chat messages
//	@Description	Cursor-paged messages (ascending order): ?after=/?before= are EXCLUSIVE order-id bounds, ?limit defaults to 25. A forward walk uses ?after alone and continues with the response's next_after; every OTHER query — ?before, no cursor, or BOTH bounds — is anchored at its NEWEST end (the newest N in range) and pages backward with next_before, so after+before does NOT walk forward through the window. has_more says more messages exist inside the requested bounds. The response carries state (unread counters, oldest unread orders, last_state_id — the mark-read race guard) and message_count (the chat's LIFETIME total, not the range size) at zero extra cost; a poll is a limit=1 read. Message text is §8 inline markup (blocks_text carries block-composed content read-only); reactions is always emoji counts ({"👍":2}); ?reactions=full adds reacted_by (participant-id lists) in its own slot. Offset pagination does not apply — page with the cursors.
//	@Id				get_chat_messages
//	@Tags			Chat
//	@Produce		json
//	@Param			space_id	path		string							true	"Space id"
//	@Param			chat_id		path		string							true	"Chat object id"
//	@Param			after		query		string							false	"Return messages after this order id (exclusive)"
//	@Param			before		query		string							false	"Return messages before this order id (exclusive)"
//	@Param			limit		query		int								false	"Messages to return"	default(25)
//	@Param			reactions	query		string							false	"counts (default) | full"
//	@Success		200			{object}	v2model.ChatMessagesResponse	"Messages + state + message_count"
//	@Failure		400			{object}	v2model.Error					"Not a chat, or invalid params"
//	@Failure		404			{object}	v2model.Error					"Chat not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/chats/{chat_id}/messages [get]
func GetChatMessagesHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Query("offset") != "" {
			RespondError(c, v2model.ValidationFailed("offset does not apply to the messages read",
				v2model.Issue{Path: "offset", Message: "messages are cursor-paged", Hint: "page with ?after= / ?before= order ids from a previous read"}))
			return
		}
		fullReactions := false
		switch c.Query("reactions") {
		case "", v2model.ReactionsCounts:
		case v2model.ReactionsFull:
			fullReactions = true
		default:
			RespondError(c, v2model.ValidationFailed("invalid reactions value",
				v2model.Issue{Path: "reactions", Message: fmt.Sprintf("unknown value %q", c.Query("reactions")), Hint: "allowed: counts, full"}))
			return
		}
		result, err := s.GetChatMessages(c.Request.Context(), c.Param("space_id"), c.Param("chat_id"), v2service.ChatMessagesQuery{
			After:         c.Query("after"),
			Before:        c.Query("before"),
			Limit:         c.GetInt(pagination.QueryParamLimit),
			FullReactions: fullReactions,
		})
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

// AddChatMessageHandler sends a message
//
//	@Summary		Send chat message
//	@Description	Sends one message: {text, reply_to?, attachments?}. Text is §8 markup SOURCE — *, [, ` and <mention objectId="…"> syntax mint real marks (the D′1 caveat; escape literal specials with a backslash) — capped at 8000 UTF-16 code units (an emoji counts 2+). Attachments are bare object ids, at most 32; the kind is inferred from each target's layout (image → image, other file layouts → file, anything else → link). Honors Idempotency-Key (C8 — a double-sent chat message is user-visible damage) and ?dry_run=true (C9, validate-only).
//	@Id				add_chat_message
//	@Tags			Chat
//	@Accept			json
//	@Produce		json
//	@Param			space_id		path		string							true	"Space id"
//	@Param			chat_id			path		string							true	"Chat object id"
//	@Param			dry_run			query		bool							false	"Validate and report without committing"
//	@Param			Idempotency-Key	header		string							false	"C8 replay guard: the same key with the same body replays the stored response"
//	@Param			request			body		v2model.AddChatMessageRequest	true	"The message to send"
//	@Success		201				{object}	v2model.ChatMessageResult		"Created message id"
//	@Failure		400				{object}	v2model.Error					"Validation failure"
//	@Failure		404				{object}	v2model.Error					"Chat not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/chats/{chat_id}/messages [post]
func AddChatMessageHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req v2model.AddChatMessageRequest
		if !decodeChatBody(c, &req, "the message body takes text, reply_to, attachments") {
			return
		}
		result, err := s.AddChatMessage(c.Request.Context(), c.Param("space_id"), c.Param("chat_id"), req, isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondChatMutation(c, result.DryRun, http.StatusCreated, result)
	}
}

// EditChatMessageHandler edits a message's text
//
//	@Summary		Edit chat message
//	@Description	Text-only MERGE: {text} replaces the message text (parsed as §8 markup source, capped at 8000 UTF-16 code units — the D′1 caveat: ALL marks are re-derived from the string, and an Emoji mark read back as its literal emoji stays literal); the message's attachments, reply target, style and blocks are preserved. Editing another member's message is a 403 forbidden. Honors Idempotency-Key (C8) and ?dry_run=true (C9).
//	@Id				edit_chat_message
//	@Tags			Chat
//	@Accept			json
//	@Produce		json
//	@Param			space_id		path		string							true	"Space id"
//	@Param			chat_id			path		string							true	"Chat object id"
//	@Param			message_id		path		string							true	"Message id"
//	@Param			dry_run			query		bool							false	"Validate and report without committing"
//	@Param			Idempotency-Key	header		string							false	"C8 replay guard: the same key with the same body replays the stored response"
//	@Param			request			body		v2model.EditChatMessageRequest	true	"The replacement text"
//	@Success		200				{object}	v2model.ChatMessageResult		"Edited message id"
//	@Failure		400				{object}	v2model.Error					"Validation failure"
//	@Failure		404				{object}	v2model.Error					"Chat or message not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/chats/{chat_id}/messages/{message_id} [patch]
func EditChatMessageHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req v2model.EditChatMessageRequest
		if !decodeChatBody(c, &req, "the edit body takes text") {
			return
		}
		result, err := s.EditChatMessage(c.Request.Context(), c.Param("space_id"), c.Param("chat_id"), c.Param("message_id"), req, isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondChatMutation(c, result.DryRun, http.StatusOK, result)
	}
}

// DeleteChatMessageHandler deletes a message
//
//	@Summary		Delete chat message
//	@Description	Deletes one message. IRREVERSIBLE side effect: attachments whose ONLY reference was this message are permanently deleted afterwards (not moved to the bin, asynchronously) — the response's warnings name the attachment ids at risk, and the dry run reports the same warnings without deleting. A missing message is a 404 on both the real call and the dry run (C9). Honors Idempotency-Key (C8 — chat DELETE is keyed too, a Phase-6 widening) and ?dry_run=true.
//	@Id				delete_chat_message
//	@Tags			Chat
//	@Produce		json
//	@Param			space_id		path		string						true	"Space id"
//	@Param			chat_id			path		string						true	"Chat object id"
//	@Param			message_id		path		string						true	"Message id"
//	@Param			dry_run			query		bool						false	"Report the would-be deletion (incl. file-GC warnings) without committing"
//	@Param			Idempotency-Key	header		string						false	"C8 replay guard: the same key with the same body replays the stored response"
//	@Success		200				{object}	v2model.ChatMessageResult	"Deleted message id"
//	@Failure		404				{object}	v2model.Error				"Chat or message not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/chats/{chat_id}/messages/{message_id} [delete]
func DeleteChatMessageHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := s.DeleteChatMessage(c.Request.Context(), c.Param("space_id"), c.Param("chat_id"), c.Param("message_id"), isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

// ToggleChatReactionHandler toggles a reaction
//
//	@Summary		Toggle chat reaction
//	@Description	Toggles the caller's {emoji} reaction on a message → {added}. A missing message is a 404. Honors Idempotency-Key (C8) and ?dry_run=true (C9 — the dry run reads the message and reports the would-be outcome; when the service has no account identity to predict with, added is omitted and a warning says so).
//	@Id				toggle_chat_reaction
//	@Tags			Chat
//	@Accept			json
//	@Produce		json
//	@Param			space_id		path		string						true	"Space id"
//	@Param			chat_id			path		string						true	"Chat object id"
//	@Param			message_id		path		string						true	"Message id"
//	@Param			dry_run			query		bool						false	"Report the would-be outcome without committing"
//	@Param			Idempotency-Key	header		string						false	"C8 replay guard: the same key with the same body replays the stored response"
//	@Param			request			body		v2model.ChatReactionRequest	true	"The emoji to toggle"
//	@Success		200				{object}	v2model.ChatReactionResult	"Toggle outcome"
//	@Failure		400				{object}	v2model.Error				"Validation failure"
//	@Failure		404				{object}	v2model.Error				"Chat or message not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/chats/{chat_id}/messages/{message_id}/reactions [post]
func ToggleChatReactionHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req v2model.ChatReactionRequest
		if !decodeChatBody(c, &req, "the reaction body takes emoji") {
			return
		}
		result, err := s.ToggleChatReaction(c.Request.Context(), c.Param("space_id"), c.Param("chat_id"), c.Param("message_id"), req, isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

// ReadChatHandler moves the read watermark
//
//	@Summary		Mark chat read
//	@Description	Moves the read watermark: {up_to, last_state_id, scope?}. up_to is the INCLUSIVE order id to mark read up to and last_state_id is the race guard — BOTH are required for scopes messages/mentions and both ride the same GET messages response (the newest message's order + state.last_state_id); an empty value for either would silently mark nothing, so it is rejected instead. Messages that arrived after last_state_id's state stay unread. scope defaults to messages; mentions marks @-mentions; reactions marks ALL unread reactions and takes no up_to/last_state_id. Honors Idempotency-Key (C8) and ?dry_run=true (C9, validate-only).
//	@Id				read_chat
//	@Tags			Chat
//	@Accept			json
//	@Produce		json
//	@Param			space_id		path		string					true	"Space id"
//	@Param			chat_id			path		string					true	"Chat object id"
//	@Param			dry_run			query		bool					false	"Validate and report without committing"
//	@Param			Idempotency-Key	header		string					false	"C8 replay guard: the same key with the same body replays the stored response"
//	@Param			request			body		v2model.ChatReadRequest	true	"The watermark move"
//	@Success		200				{object}	v2model.ChatReadResult	"Watermark moved"
//	@Failure		400				{object}	v2model.Error			"Validation failure"
//	@Failure		404				{object}	v2model.Error			"Chat not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/chats/{chat_id}/read [post]
func ReadChatHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req v2model.ChatReadRequest
		if !decodeChatBody(c, &req, "the read body takes up_to, last_state_id, scope") {
			return
		}
		result, err := s.ReadChat(c.Request.Context(), c.Param("space_id"), c.Param("chat_id"), req, isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}
