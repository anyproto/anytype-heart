package v2handler

// chat.go holds the chat handlers. Message text
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
//	@Summary		List the chats in a space
//	@Description	A row carries no unread counters. Per-chat unread state comes back with the messages read instead.
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
//	@Summary		Create a chat
//	@Description	Messages are not blocks. Add them through the messages route; a document edit cannot reach them.
//	@Id				create_chat
//	@Tags			Chat
//	@Accept			json
//	@Produce		json
//	@Param			space_id		path		string						true	"Space id"
//	@Param			dry_run			query		bool						false	"Validate and report without committing"
//	@Param			Idempotency-Key	header		string						false	"Replay guard: the same key with the same body replays the stored response"
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
//	@Summary		List chat messages
//	@Description	`after` on its own walks forward, oldest first, continuing from `next_after`. Every other query, including `after` together with `before`, is anchored at the newest end of the range and walks backward from `next_before`. Both bounds are exclusive. `message_count` is the chat's total since it began, not the size of the range. Offset paging does not apply here, and `offset` is refused.
//	@Id				get_chat_messages
//	@Tags			Chat
//	@Produce		json
//	@Param			space_id	path		string							true	"Space id"
//	@Param			chat_id		path		string							true	"Chat object id"
//	@Param			after		query		string							false	"Return messages after this order id (exclusive)"
//	@Param			before		query		string							false	"Return messages before this order id (exclusive)"
//	@Param			limit		query		int								false	"Messages to return"	default(25)
//	@Param			reactions	query		string							false	"counts (default) returns the emoji counts; full adds the participant ids behind each count"
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
//	@Summary		Send a chat message
//	@Description	The text is markup source, so `*`, `[` and a mention tag mint real marks; escape a literal one with a backslash. The cap is 8000 UTF-16 code units, where one emoji can cost two or more. Attachments are object ids, at most 32, and each one's kind is taken from the target's layout.
//	@Id				add_chat_message
//	@Tags			Chat
//	@Accept			json
//	@Produce		json
//	@Param			space_id		path		string							true	"Space id"
//	@Param			chat_id			path		string							true	"Chat object id"
//	@Param			dry_run			query		bool							false	"Validate and report without committing"
//	@Param			Idempotency-Key	header		string							false	"Replay guard: the same key with the same body replays the stored response"
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
//	@Summary		Replace a chat message's text
//	@Description	Every mark is re-derived from the text you send, so a mark the old text carried and the new text does not spell out is lost. Attachments, the reply target and the style survive. Editing another member's message is a 403.
//	@Id				edit_chat_message
//	@Tags			Chat
//	@Accept			json
//	@Produce		json
//	@Param			space_id		path		string							true	"Space id"
//	@Param			chat_id			path		string							true	"Chat object id"
//	@Param			message_id		path		string							true	"Message id"
//	@Param			dry_run			query		bool							false	"Validate and report without committing"
//	@Param			Idempotency-Key	header		string							false	"Replay guard: the same key with the same body replays the stored response"
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
//	@Summary		Delete a chat message
//	@Description	An attachment whose only reference was this message is erased for good afterwards, not moved to Bin. The response names those ids in `warnings`, and a dry run reports the same list without deleting anything. A message that does not exist is a 404 on the dry run too.
//	@Id				delete_chat_message
//	@Tags			Chat
//	@Produce		json
//	@Param			space_id		path		string						true	"Space id"
//	@Param			chat_id			path		string						true	"Chat object id"
//	@Param			message_id		path		string						true	"Message id"
//	@Param			dry_run			query		bool						false	"Report what would be deleted, attachments included, without committing"
//	@Param			Idempotency-Key	header		string						false	"Replay guard: the same key with the same body replays the stored response"
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
//	@Summary		Toggle a reaction on a chat message
//	@Description	`added` says which way the toggle went. A dry run predicts it, but when there is no account identity to predict with it omits the field and says so in `warnings`.
//	@Id				toggle_chat_reaction
//	@Tags			Chat
//	@Accept			json
//	@Produce		json
//	@Param			space_id		path		string						true	"Space id"
//	@Param			chat_id			path		string						true	"Chat object id"
//	@Param			message_id		path		string						true	"Message id"
//	@Param			dry_run			query		bool						false	"Report the would-be outcome without committing"
//	@Param			Idempotency-Key	header		string						false	"Replay guard: the same key with the same body replays the stored response"
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
//	@Summary		Move a chat's read watermark
//	@Description	`up_to` is inclusive, and it and `last_state_id` both come from one messages read: the newest message's order, and the state's own id. An empty value for either would silently mark nothing, so it is refused. Messages that arrived after that state stay unread. The reactions scope marks every unread reaction and takes neither field.
//	@Id				read_chat
//	@Tags			Chat
//	@Accept			json
//	@Produce		json
//	@Param			space_id		path		string					true	"Space id"
//	@Param			chat_id			path		string					true	"Chat object id"
//	@Param			dry_run			query		bool					false	"Validate and report without committing"
//	@Param			Idempotency-Key	header		string					false	"Replay guard: the same key with the same body replays the stored response"
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
