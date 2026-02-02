package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/service"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// ListMessagesHandler retrieves a list of chat messages
//
//	@Summary		List messages
//	@Description	Retrieves a paginated list of messages in the specified chat. Supports cursor-based pagination using before_order_id or after_order_id parameters for infinite scroll, or offset-based pagination for initial loads.
//	@Id				list_messages
//	@Tags			Chat
//	@Produce		json
//	@Param			Anytype-Version	header		string						true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path		string						true	"The ID of the space"
//	@Param			chat_id			path		string						true	"The ID of the chat object"
//	@Param			offset			query		int							false	"The number of items to skip (for offset-based pagination)"	default(0)
//	@Param			limit			query		int							false	"The number of items to return"								default(100)	maximum(1000)
//	@Param			before_order_id	query		string						false	"Return messages before this order ID (cursor-based pagination for older messages)"
//	@Param			after_order_id	query		string						false	"Return messages after this order ID (cursor-based pagination for newer messages)"
//	@Success		200				{object}	apimodel.MessagesResponse	"The list of chat messages"
//	@Failure		401				{object}	util.UnauthorizedError		"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError			"Chat not found"
//	@Failure		500				{object}	util.ServerError			"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/chats/{chat_id}/messages [get]
func ListMessagesHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		chatId := c.Param("chat_id")
		limit := c.GetInt(pagination.QueryParamLimit)
		beforeOrderId := c.Query("before_order_id")
		afterOrderId := c.Query("after_order_id")

		messages, chatState, err := s.GetMessages(c.Request.Context(), chatId, beforeOrderId, afterOrderId, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrChatNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrFailedRetrieveMessages, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.MessagesResponse{
			Messages: messages,
			State:    chatState,
		})
	}
}

// GetMessageHandler retrieves a single chat message
//
//	@Summary		Get message
//	@Description	Retrieves a single message by its ID from the specified chat.
//	@Id				get_message
//	@Tags			Chat
//	@Produce		json
//	@Param			Anytype-Version	header		string						true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path		string						true	"The ID of the space"
//	@Param			chat_id			path		string						true	"The ID of the chat object"
//	@Param			message_id		path		string						true	"The ID of the message to retrieve"
//	@Success		200				{object}	apimodel.MessageResponse	"The chat message"
//	@Failure		401				{object}	util.UnauthorizedError		"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError			"Message not found"
//	@Failure		500				{object}	util.ServerError			"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/chats/{chat_id}/messages/{message_id} [get]
func GetMessageHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		chatId := c.Param("chat_id")
		messageId := c.Param("message_id")

		message, err := s.GetMessageById(c.Request.Context(), chatId, messageId)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrMessageNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrFailedRetrieveMessages, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.MessageResponse{Message: *message})
	}
}

// CreateMessageHandler creates a new chat message
//
//	@Summary		Create message
//	@Description	Creates a new message in the specified chat. Rate limited.
//	@Id				create_message
//	@Tags			Chat
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string							true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path		string							true	"The ID of the space"
//	@Param			chat_id			path		string							true	"The ID of the chat object"
//	@Param			message			body		apimodel.CreateMessageRequest	true	"The message to create"
//	@Success		201				{object}	apimodel.MessageResponse		"The created message"
//	@Failure		400				{object}	util.ValidationError			"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError			"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError				"Chat not found"
//	@Failure		429				{object}	util.RateLimitError				"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError				"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/chats/{chat_id}/messages [post]
func CreateMessageHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		chatId := c.Param("chat_id")

		request := apimodel.CreateMessageRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToApiError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		message, err := s.CreateMessage(c.Request.Context(), chatId, request)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(service.ErrChatNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrFailedCreateMessage, http.StatusInternalServerError),
			util.ErrToCode(service.ErrMessageNotFound, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedRetrieveMessages, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusCreated, apimodel.MessageResponse{Message: *message})
	}
}

// UpdateMessageHandler updates an existing chat message
//
//	@Summary		Update message
//	@Description	Updates an existing message in the specified chat. Rate limited.
//	@Id				update_message
//	@Tags			Chat
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string							true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path		string							true	"The ID of the space"
//	@Param			chat_id			path		string							true	"The ID of the chat object"
//	@Param			message_id		path		string							true	"The ID of the message to update"
//	@Param			message			body		apimodel.UpdateMessageRequest	true	"The message updates"
//	@Success		200				{object}	apimodel.MessageResponse		"The updated message"
//	@Failure		400				{object}	util.ValidationError			"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError			"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError				"Message not found"
//	@Failure		429				{object}	util.RateLimitError				"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError				"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/chats/{chat_id}/messages/{message_id} [patch]
func UpdateMessageHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		chatId := c.Param("chat_id")
		messageId := c.Param("message_id")

		request := apimodel.UpdateMessageRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToApiError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		err := s.UpdateMessage(c.Request.Context(), chatId, messageId, request)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(service.ErrMessageNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrFailedUpdateMessage, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		// Retrieve the updated message to return
		message, err := s.GetMessageById(c.Request.Context(), chatId, messageId)
		if err != nil {
			code := util.MapErrorCode(err,
				util.ErrToCode(service.ErrMessageNotFound, http.StatusNotFound),
				util.ErrToCode(service.ErrFailedRetrieveMessages, http.StatusInternalServerError),
			)
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.MessageResponse{Message: *message})
	}
}

// DeleteMessageHandler deletes a chat message
//
//	@Summary		Delete message
//	@Description	Deletes a message from the specified chat. Rate limited.
//	@Id				delete_message
//	@Tags			Chat
//	@Produce		json
//	@Param			Anytype-Version	header	string	true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path	string	true	"The ID of the space"
//	@Param			chat_id			path	string	true	"The ID of the chat object"
//	@Param			message_id		path	string	true	"The ID of the message to delete"
//	@Success		204				"Message deleted successfully"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Message not found"
//	@Failure		429				{object}	util.RateLimitError		"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/chats/{chat_id}/messages/{message_id} [delete]
func DeleteMessageHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		chatId := c.Param("chat_id")
		messageId := c.Param("message_id")

		err := s.DeleteMessage(c.Request.Context(), chatId, messageId)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrMessageNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrFailedDeleteMessage, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.Status(http.StatusNoContent)
	}
}

// ToggleReactionHandler toggles a reaction on a chat message
//
//	@Summary		Toggle reaction
//	@Description	Toggles a reaction (emoji) on a message. If the reaction already exists from the user, it is removed; otherwise, it is added. Rate limited.
//	@Id				toggle_reaction
//	@Tags			Chat
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string							true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path		string							true	"The ID of the space"
//	@Param			chat_id			path		string							true	"The ID of the chat object"
//	@Param			message_id		path		string							true	"The ID of the message"
//	@Param			reaction		body		apimodel.ToggleReactionRequest	true	"The reaction to toggle"
//	@Success		200				{object}	apimodel.ToggleReactionResponse	"The result of the toggle operation"
//	@Failure		400				{object}	util.ValidationError			"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError			"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError				"Message not found"
//	@Failure		429				{object}	util.RateLimitError				"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError				"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/chats/{chat_id}/messages/{message_id}/reactions [post]
func ToggleReactionHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		chatId := c.Param("chat_id")
		messageId := c.Param("message_id")

		request := apimodel.ToggleReactionRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToApiError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		added, err := s.ToggleReaction(c.Request.Context(), chatId, messageId, request.Emoji)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(service.ErrMessageNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrFailedToggleReaction, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.ToggleReactionResponse{Added: added})
	}
}

// SearchMessagesHandler searches for messages in a chat
//
//	@Summary		Search messages
//	@Description	Searches for messages in the specified chat using full-text search.
//	@Id				search_messages
//	@Tags			Chat
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string							true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path		string							true	"The ID of the space"
//	@Param			chat_id			path		string							true	"The ID of the chat object"
//	@Param			search			body		apimodel.SearchMessagesRequest	true	"The search query"
//	@Param			offset			query		int								false	"The number of items to skip"	default(0)
//	@Param			limit			query		int								false	"The number of items to return"	default(100)	maximum(1000)
//	@Success		200				{object}	apimodel.SearchMessagesResponse	"The search results"
//	@Failure		400				{object}	util.ValidationError			"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError			"Unauthorized"
//	@Failure		500				{object}	util.ServerError				"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/chats/{chat_id}/search [post]
func SearchMessagesHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		chatId := c.Param("chat_id")
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)

		request := apimodel.SearchMessagesRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToApiError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		results, err := s.SearchMessages(c.Request.Context(), spaceId, chatId, request.Query, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(service.ErrFailedSearchMessages, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.SearchMessagesResponse{Results: results})
	}
}

const (
	defaultSSELimit    = 50
	sseChannelSize     = 100
	ssePingInterval    = 30 * time.Second
	sseMaxInitialLimit = 1000
)

// SubscribeChatHandler establishes an SSE connection for real-time chat updates
//
//	@Summary		Subscribe to chat events
//	@Description	Establishes a Server-Sent Events (SSE) connection to receive real-time chat updates including new messages, edits, deletions, and reactions.
//	@Id				subscribe_chat
//	@Tags			Chat
//	@Produce		text/event-stream
//	@Param			Anytype-Version	header	string	true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path	string	true	"The ID of the space"
//	@Param			chat_id			path	string	true	"The ID of the chat object"
//	@Param			limit			query	int		false	"Number of initial messages to return"	default(50)	maximum(1000)
//	@Success		200				"SSE event stream"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Chat not found"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/chats/{chat_id}/subscribe [get]
func SubscribeChatHandler(s *service.Service, sseManager *service.SSESessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		chatId := c.Param("chat_id")
		limit := parseSSELimit(c.Query("limit"))
		subId := uuid.New().String()

		// Set SSE headers
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

		// Create subscription and get initial messages
		messages, state, err := s.SubscribeChat(c.Request.Context(), chatId, subId, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrChatNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrFailedSubscribeChat, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		// Register SSE session
		session := &service.SSESession{
			SubId:  subId,
			ChatId: chatId,
			Events: make(chan *apimodel.SSEChatEvent, sseChannelSize),
			Done:   make(chan struct{}),
		}
		sseManager.Register(session)
		defer func() {
			sseManager.Unregister(subId)
			_ = s.UnsubscribeChat(c.Request.Context(), chatId, subId) //nolint:errcheck // best effort cleanup
		}()

		// Send initial messages
		initialEvent := apimodel.SSEChatEvent{
			Type:     apimodel.SSEEventTypeInitial,
			Messages: messages,
			State:    state,
		}
		writeSSE(c.Writer, "initial", initialEvent)
		c.Writer.Flush()

		// Stream events until client disconnects
		ticker := time.NewTicker(ssePingInterval)
		defer ticker.Stop()

		for {
			select {
			case <-c.Request.Context().Done():
				return
			case event := <-session.Events:
				writeSSE(c.Writer, string(event.Type), event)
				c.Writer.Flush()
			case <-ticker.C:
				writeSSE(c.Writer, "ping", nil)
				c.Writer.Flush()
			case <-session.Done:
				return
			}
		}
	}
}

func parseSSELimit(limitStr string) int {
	if limitStr == "" {
		return defaultSSELimit
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		return defaultSSELimit
	}
	if limit > sseMaxInitialLimit {
		return sseMaxInitialLimit
	}
	return limit
}

func writeSSE(w io.Writer, eventType string, data interface{}) {
	fmt.Fprintf(w, "event: %s\n", eventType)
	if data != nil {
		jsonData, _ := json.Marshal(data) //nolint:errcheck // SSE best effort
		fmt.Fprintf(w, "data: %s\n\n", jsonData)
	} else {
		fmt.Fprintf(w, "data:\n\n")
	}
}
