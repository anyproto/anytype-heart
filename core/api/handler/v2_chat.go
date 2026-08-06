package handler

// v2_chat.go holds the Phase-6 chat handlers (APIV2.md §8.7). Message text
// is §8 inline markup in BOTH directions (the D′1 caveat applies verbatim:
// find/replace-style specials mint real marks). C7 etag/If-Match does NOT
// apply to chats — order ids and lastStateId are the stream's native
// concurrency vocabulary; this is a deliberate, documented exemption like
// search's C8/C9 one. All mutations honor Idempotency-Key (C8) and
// ?dry_run=true (C9).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/service"
)

// maxChatRequestBody caps chat mutation bodies. A message is text (≤ the
// middleware's message-length cap) plus a bounded attachment list — 1 MiB
// is orders of magnitude above any legitimate body.
const maxChatRequestBody = 1 << 20 // 1 MiB

// decodeChatBody decodes a chat request body strictly (unknown fields are
// 400s with the field named — C13's spirit at the request layer, matching
// search). A false return means the error response was already written.
func decodeChatBody(c *gin.Context, into any, hint string) bool {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxChatRequestBody+1))
	if err != nil {
		RespondV2Error(c, apimodel.V2ValidationFailed("read request body",
			apimodel.V2Issue{Message: err.Error()}))
		return false
	}
	if len(body) > maxChatRequestBody {
		RespondV2Error(c, apimodel.V2RequestTooLarge(
			fmt.Sprintf("chat request body exceeds the %d-byte limit", maxChatRequestBody)))
		return false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		RespondV2Error(c, apimodel.V2ValidationFailed("request body is required",
			apimodel.V2Issue{Message: hint}))
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		issue := apimodel.V2Issue{Message: err.Error(), Hint: hint}
		if field, ok := unknownFieldName(err); ok {
			issue.Path = "/" + field
		}
		RespondV2Error(c, apimodel.V2ValidationFailed("invalid request body", issue))
		return false
	}
	return true
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

// ListChatsV2Handler lists the space's chats as C5 rows
//
//	@Summary		List chats
//	@Description	C5 rows {id, name} via a store query — no chat opens, so the list is cheap at any size. Deliberately counter-free (Q3): per-chat unread state comes free on the messages read. No etag (C7 exemption: chats use order ids and lastStateId as their concurrency vocabulary).
//	@Id				v2_list_chats
//	@Tags			V2
//	@Produce		json
//	@Param			space_id	path		string										true	"Space id"
//	@Param			offset		query		int											false	"Rows to skip"	default(0)
//	@Param			limit		query		int											false	"Rows to return"	default(25)
//	@Success		200			{object}	apimodel.V2ListResponse[apimodel.V2ChatRow]	"Chat rows"
//	@Failure		404			{object}	apimodel.V2Error							"Space not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/chats [get]
func ListChatsV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, err := s.ListChats(c.Request.Context(), c.Param("space_id"), offset, limit)
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, apimodel.NewV2ListResponse(rows, total, offset, limit, hasMore,
			"request the next offset"))
	}
}

// CreateChatV2Handler creates a chat
//
//	@Summary		Create chat
//	@Description	Creates a chat object: {name}. A thin create with the chatDerived type — messages live in the chat store, not blocks. Honors Idempotency-Key (C8) and ?dry_run=true (C9). No If-Match (C7 exemption).
//	@Id				v2_create_chat
//	@Tags			V2
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			dry_run		query		bool					false	"Validate and report without committing"
//	@Success		201			{object}	apimodel.V2ChatResult	"Created chat row"
//	@Failure		400			{object}	apimodel.V2Error		"Validation failure"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/chats [post]
func CreateChatV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req apimodel.V2CreateChatRequest
		if !decodeChatBody(c, &req, "the chat body takes name") {
			return
		}
		result, err := s.CreateChat(c.Request.Context(), c.Param("space_id"), req, isV2DryRun(c))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		respondChatMutation(c, result.DryRun, http.StatusCreated, result)
	}
}

// GetChatMessagesV2Handler reads messages with the state passthrough
//
//	@Summary		Get chat messages
//	@Description	Cursor-paged messages (ascending order): ?after=/?before= are EXCLUSIVE order-id bounds, ?limit defaults to 25. The response carries state (unread counters, oldest unread orders, lastStateId — the mark-read race guard) and messageCount at zero extra cost; a poll is a limit=1 read. Message text is §8 inline markup; reactions default to counts ({"👍":2}), ?reactions=full restores identity lists (participant ids). Offset pagination does not apply — page with the cursors.
//	@Id				v2_get_chat_messages
//	@Tags			V2
//	@Produce		json
//	@Param			space_id	path		string								true	"Space id"
//	@Param			chat_id		path		string								true	"Chat object id"
//	@Param			after		query		string								false	"Return messages after this order id (exclusive)"
//	@Param			before		query		string								false	"Return messages before this order id (exclusive)"
//	@Param			limit		query		int									false	"Messages to return"	default(25)
//	@Param			reactions	query		string								false	"counts (default) | full"
//	@Success		200			{object}	apimodel.V2ChatMessagesResponse		"Messages + state + messageCount"
//	@Failure		400			{object}	apimodel.V2Error					"Not a chat, or invalid params"
//	@Failure		404			{object}	apimodel.V2Error					"Chat not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/chats/{chat_id}/messages [get]
func GetChatMessagesV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Query("offset") != "" {
			RespondV2Error(c, apimodel.V2ValidationFailed("offset does not apply to the messages read",
				apimodel.V2Issue{Path: "offset", Message: "messages are cursor-paged", Hint: "page with ?after= / ?before= order ids from a previous read"}))
			return
		}
		fullReactions := false
		switch c.Query("reactions") {
		case "", apimodel.V2ReactionsCounts:
		case apimodel.V2ReactionsFull:
			fullReactions = true
		default:
			RespondV2Error(c, apimodel.V2ValidationFailed("invalid reactions value",
				apimodel.V2Issue{Path: "reactions", Message: fmt.Sprintf("unknown value %q", c.Query("reactions")), Hint: "allowed: counts, full"}))
			return
		}
		result, err := s.GetChatMessages(c.Request.Context(), c.Param("space_id"), c.Param("chat_id"), service.V2ChatMessagesQuery{
			After:         c.Query("after"),
			Before:        c.Query("before"),
			Limit:         c.GetInt(pagination.QueryParamLimit),
			FullReactions: fullReactions,
		})
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

// AddChatMessageV2Handler sends a message
//
//	@Summary		Send chat message
//	@Description	Sends one message: {text, replyTo?, attachments?}. Text is §8 markup SOURCE — *, [, ` and <mention objectId="…"> syntax mint real marks (the D′1 caveat; escape literal specials with a backslash). Attachments are bare object ids; the kind is inferred from each target's layout (image → image, other file layouts → file, anything else → link). Honors Idempotency-Key (C8 — a double-sent chat message is user-visible damage) and ?dry_run=true (C9, validate-only).
//	@Id				v2_add_chat_message
//	@Tags			V2
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string							true	"Space id"
//	@Param			chat_id		path		string							true	"Chat object id"
//	@Param			dry_run		query		bool							false	"Validate and report without committing"
//	@Success		201			{object}	apimodel.V2ChatMessageResult	"Created message id"
//	@Failure		400			{object}	apimodel.V2Error				"Validation failure"
//	@Failure		404			{object}	apimodel.V2Error				"Chat not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/chats/{chat_id}/messages [post]
func AddChatMessageV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req apimodel.V2AddChatMessageRequest
		if !decodeChatBody(c, &req, "the message body takes text, replyTo, attachments") {
			return
		}
		result, err := s.AddChatMessage(c.Request.Context(), c.Param("space_id"), c.Param("chat_id"), req, isV2DryRun(c))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		respondChatMutation(c, result.DryRun, http.StatusCreated, result)
	}
}

// EditChatMessageV2Handler edits a message's text
//
//	@Summary		Edit chat message
//	@Description	Text-only MERGE: {text} replaces the message text (parsed as §8 markup source — the D′1 caveat); the message's attachments, reply target and style are preserved. Honors Idempotency-Key (C8) and ?dry_run=true (C9).
//	@Id				v2_edit_chat_message
//	@Tags			V2
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string							true	"Space id"
//	@Param			chat_id		path		string							true	"Chat object id"
//	@Param			message_id	path		string							true	"Message id"
//	@Param			dry_run		query		bool							false	"Validate and report without committing"
//	@Success		200			{object}	apimodel.V2ChatMessageResult	"Edited message id"
//	@Failure		400			{object}	apimodel.V2Error				"Validation failure"
//	@Failure		404			{object}	apimodel.V2Error				"Chat or message not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/chats/{chat_id}/messages/{message_id} [patch]
func EditChatMessageV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req apimodel.V2EditChatMessageRequest
		if !decodeChatBody(c, &req, "the edit body takes text") {
			return
		}
		result, err := s.EditChatMessage(c.Request.Context(), c.Param("space_id"), c.Param("chat_id"), c.Param("message_id"), req, isV2DryRun(c))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		respondChatMutation(c, result.DryRun, http.StatusOK, result)
	}
}

// DeleteChatMessageV2Handler deletes a message
//
//	@Summary		Delete chat message
//	@Description	Deletes one message. Honors Idempotency-Key (C8 — chat DELETE is keyed too, a Phase-6 widening) and ?dry_run=true (C9, existence check only).
//	@Id				v2_delete_chat_message
//	@Tags			V2
//	@Produce		json
//	@Param			space_id	path		string							true	"Space id"
//	@Param			chat_id		path		string							true	"Chat object id"
//	@Param			message_id	path		string							true	"Message id"
//	@Param			dry_run		query		bool							false	"Validate and report without committing"
//	@Success		200			{object}	apimodel.V2ChatMessageResult	"Deleted message id"
//	@Failure		404			{object}	apimodel.V2Error				"Chat or message not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/chats/{chat_id}/messages/{message_id} [delete]
func DeleteChatMessageV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := s.DeleteChatMessage(c.Request.Context(), c.Param("space_id"), c.Param("chat_id"), c.Param("message_id"), isV2DryRun(c))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

// ToggleChatReactionV2Handler toggles a reaction
//
//	@Summary		Toggle chat reaction
//	@Description	Toggles the caller's {emoji} reaction on a message → {added}. Honors Idempotency-Key (C8) and ?dry_run=true (C9 — the dry run reads the message and reports the would-be outcome).
//	@Id				v2_toggle_chat_reaction
//	@Tags			V2
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string							true	"Space id"
//	@Param			chat_id		path		string							true	"Chat object id"
//	@Param			message_id	path		string							true	"Message id"
//	@Param			dry_run		query		bool							false	"Report the would-be outcome without committing"
//	@Success		200			{object}	apimodel.V2ChatReactionResult	"Toggle outcome"
//	@Failure		400			{object}	apimodel.V2Error				"Validation failure"
//	@Failure		404			{object}	apimodel.V2Error				"Chat or message not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/chats/{chat_id}/messages/{message_id}/reactions [post]
func ToggleChatReactionV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req apimodel.V2ChatReactionRequest
		if !decodeChatBody(c, &req, "the reaction body takes emoji") {
			return
		}
		result, err := s.ToggleChatReaction(c.Request.Context(), c.Param("space_id"), c.Param("chat_id"), c.Param("message_id"), req, isV2DryRun(c))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

// ReadChatV2Handler moves the read watermark
//
//	@Summary		Mark chat read
//	@Description	Moves the read watermark: {upTo, lastStateId?, scope?}. upTo is the INCLUSIVE order id to mark read up to (required for scopes messages/mentions — take it from the newest message of a GET messages read). lastStateId is the race guard from that read's state.lastStateId: messages that arrived after that state stay unread. scope defaults to messages; mentions marks @-mentions; reactions marks ALL unread reactions and takes no upTo. Honors Idempotency-Key (C8) and ?dry_run=true (C9, validate-only).
//	@Id				v2_read_chat
//	@Tags			V2
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string						true	"Space id"
//	@Param			chat_id		path		string						true	"Chat object id"
//	@Param			dry_run		query		bool						false	"Validate and report without committing"
//	@Success		200			{object}	apimodel.V2ChatReadResult	"Watermark moved"
//	@Failure		400			{object}	apimodel.V2Error			"Validation failure"
//	@Failure		404			{object}	apimodel.V2Error			"Chat not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/chats/{chat_id}/read [post]
func ReadChatV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req apimodel.V2ChatReadRequest
		if !decodeChatBody(c, &req, "the read body takes upTo, lastStateId, scope") {
			return
		}
		result, err := s.ReadChat(c.Request.Context(), c.Param("space_id"), c.Param("chat_id"), req, isV2DryRun(c))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}
