package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/service"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const defaultChatMessagesLimit = 50

// ListChatsHandler retrieves a list of chat objects in a space
//
//	@Summary		List chats
//	@Description	Retrieves a paginated list of chat objects in the given space. Chat objects are containers for chat messages; use the returned chat IDs with the GetChatMessages endpoint to fetch their messages.
//	@Description	Supports dynamic filtering via query parameters (see ListObjects for the full filter grammar).
//	@Id				list_chats
//	@Tags			Chat
//	@Produce		json
//	@Param			Anytype-Version	header		string											true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path		string											true	"The ID of the space in which to list chats; must be retrieved from ListSpaces endpoint"
//	@Param			offset			query		int												false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int												false	"The number of items to return"											default(100)	maximum(1000)
//	@Success		200				{object}	pagination.PaginatedResponse[apimodel.Object]	"The list of chats in the specified space"
//	@Failure		401				{object}	util.UnauthorizedError							"Unauthorized"
//	@Failure		500				{object}	util.ServerError								"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/chats [get]
func ListChatsHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)

		filtersAny, _ := c.Get("filters")
		filters := filtersAny.([]*model.BlockContentDataviewFilter)

		chats, total, hasMore, err := s.ListChats(c.Request.Context(), spaceId, filters, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedListChats, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		pagination.RespondWithPagination(c, http.StatusOK, chats, total, offset, limit, hasMore)
	}
}

// GetChatMessagesHandler returns messages for a chat
//
//	@Summary		Get chat messages
//	@Description	Retrieves a list of messages from a chat. Supports cursor-based pagination via before_order_id and after_order_id query parameters.
//	@Id				get_chat_messages
//	@Tags			Chat
//	@Produce		json
//	@Param			Anytype-Version	header		string							true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path		string							true	"The ID of the space"
//	@Param			chat_id			path		string							true	"The ID of the chat object"
//	@Param			before_order_id	query		string							false	"Return messages before this order ID"
//	@Param			after_order_id	query		string							false	"Return messages after this order ID"
//	@Param			limit			query		int								false	"Maximum number of messages to return"	default(50)
//	@Success		200				{object}	apimodel.ChatMessagesResponse	"The list of messages"
//	@Failure		401				{object}	util.UnauthorizedError			"Unauthorized"
//	@Failure		500				{object}	util.ServerError				"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/chats/{chat_id}/messages [get]
func GetChatMessagesHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		chatId := c.Param("chat_id")
		beforeOrderId := c.Query("before_order_id")
		afterOrderId := c.Query("after_order_id")

		limit := defaultChatMessagesLimit
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
				limit = parsed
			}
		}

		messages, err := s.GetChatMessages(c.Request.Context(), spaceId, chatId, beforeOrderId, afterOrderId, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedGetMessages, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.ChatMessagesResponse{Messages: messages})
	}
}

// AddChatMessageHandler adds a new message to a chat
//
//	@Summary		Add chat message
//	@Description	Adds a new message to the specified chat.
//	@Id				add_chat_message
//	@Tags			Chat
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string							true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path		string							true	"The ID of the space"
//	@Param			chat_id			path		string							true	"The ID of the chat object"
//	@Param			message			body		apimodel.AddChatMessageRequest	true	"The message to add"
//	@Success		201				{object}	apimodel.AddChatMessageResponse	"The created message ID"
//	@Failure		400				{object}	util.ValidationError			"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError			"Unauthorized"
//	@Failure		500				{object}	util.ServerError				"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/chats/{chat_id}/messages [post]
func AddChatMessageHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		chatId := c.Param("chat_id")

		var req apimodel.AddChatMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apiErr := util.CodeToApiError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		messageId, err := s.AddChatMessage(c.Request.Context(), chatId, req)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedAddMessage, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusCreated, apimodel.AddChatMessageResponse{MessageId: messageId})
	}
}

// EditChatMessageHandler edits an existing chat message
//
//	@Summary		Edit chat message
//	@Description	Edits the content of an existing message in the specified chat.
//	@Id				edit_chat_message
//	@Tags			Chat
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string							true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path		string							true	"The ID of the space"
//	@Param			chat_id			path		string							true	"The ID of the chat object"
//	@Param			message_id		path		string							true	"The ID of the message to edit"
//	@Param			message			body		apimodel.EditChatMessageRequest	true	"The updated message content"
//	@Success		200				{object}	nil								"Message updated successfully"
//	@Failure		400				{object}	util.ValidationError			"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError			"Unauthorized"
//	@Failure		500				{object}	util.ServerError				"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/chats/{chat_id}/messages/{message_id} [patch]
func EditChatMessageHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		chatId := c.Param("chat_id")
		messageId := c.Param("message_id")

		var req apimodel.EditChatMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apiErr := util.CodeToApiError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		err := s.EditChatMessage(c.Request.Context(), chatId, messageId, req)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedEditMessage, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.Status(http.StatusOK)
	}
}

// DeleteChatMessageHandler deletes a chat message
//
//	@Summary		Delete chat message
//	@Description	Deletes a message from the specified chat.
//	@Id				delete_chat_message
//	@Tags			Chat
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path		string					true	"The ID of the space"
//	@Param			chat_id			path		string					true	"The ID of the chat object"
//	@Param			message_id		path		string					true	"The ID of the message to delete"
//	@Success		200				{object}	nil						"Message deleted successfully"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/chats/{chat_id}/messages/{message_id} [delete]
func DeleteChatMessageHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		chatId := c.Param("chat_id")
		messageId := c.Param("message_id")

		err := s.DeleteChatMessage(c.Request.Context(), chatId, messageId)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedDeleteMessage, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.Status(http.StatusOK)
	}
}

// ToggleChatReactionHandler toggles a reaction on a chat message
//
//	@Summary		Toggle message reaction
//	@Description	Toggles an emoji reaction on a message. If the reaction already exists for the current user, it will be removed; otherwise, it will be added.
//	@Id				toggle_chat_reaction
//	@Tags			Chat
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string							true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path		string							true	"The ID of the space"
//	@Param			chat_id			path		string							true	"The ID of the chat object"
//	@Param			message_id		path		string							true	"The ID of the message"
//	@Param			reaction		body		apimodel.ToggleReactionRequest	true	"The emoji to toggle"
//	@Success		200				{object}	nil								"Reaction toggled"
//	@Failure		400				{object}	util.ValidationError			"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError			"Unauthorized"
//	@Failure		500				{object}	util.ServerError				"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/chats/{chat_id}/messages/{message_id}/reactions [post]
func ToggleChatReactionHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		chatId := c.Param("chat_id")
		messageId := c.Param("message_id")

		var req apimodel.ToggleReactionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apiErr := util.CodeToApiError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		_, err := s.ToggleChatReaction(c.Request.Context(), chatId, messageId, req.Emoji)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedToggleReaction, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.Status(http.StatusOK)
	}
}
