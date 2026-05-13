package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pb"
)

const (
	defaultSSELimit   = 50
	sseChannelBufSize = 256
)

// ChatStreamHandler streams chat events via Server-Sent Events
//
//	@Summary		Subscribe to chat messages (SSE)
//	@Description	Opens a Server-Sent Events stream for real-time chat updates. On connect, the last N messages are sent, followed by live events (message_added, message_updated, message_deleted, reactions_updated). Supports authentication via Authorization header or token query parameter.
//	@Id				chat_message_stream
//	@Tags			Chat
//	@Produce		text/event-stream
//	@Param			Anytype-Version	header	string	true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path	string	true	"The ID of the space"
//	@Param			chat_id			path	string	true	"The ID of the chat object"
//	@Param			limit			query	int		false	"Number of recent messages to send on connect"	default(50)
//	@Param			token			query	string	false	"API key for authentication (alternative to Authorization header, needed for browser EventSource)"
//	@Success		200				"SSE stream of chat events"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/chats/{chat_id}/messages/stream [get]
func ChatStreamHandler(chatSubSvc apicore.ChatSubscriptionService) gin.HandlerFunc {
	return func(c *gin.Context) {
		chatId := c.Param("chat_id")

		limit := defaultSSELimit
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
				limit = parsed
			}
		}

		sink := make(chan *pb.Event, sseChannelBufSize)
		subId := "sse-" + uuid.New().String()

		messages, err := chatSubSvc.SubscribeLastMessages(c.Request.Context(), chatId, limit, subId, sink)
		if err != nil {
			apiErr := util.CodeToApiError(http.StatusInternalServerError, err.Error())
			c.JSON(http.StatusInternalServerError, apiErr)
			return
		}
		defer chatSubSvc.Unsubscribe(chatId, subId)

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)

		for _, msg := range messages {
			ev := &apimodel.ChatEvent{
				Type: "message_added",
				Payload: apimodel.ChatEventMessageAdded{
					Message: apimodel.ChatMessageFromProto(msg),
				},
			}
			writeSSE(c, ev)
		}
		c.Writer.Flush()

		ctx := c.Request.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-sink:
				if !ok {
					return
				}
				for _, msg := range event.Messages {
					chatEvent := apimodel.ConvertEventMessage(msg)
					if chatEvent != nil {
						writeSSE(c, chatEvent)
					}
				}
				c.Writer.Flush()
			}
		}
	}
}

func writeSSE(c *gin.Context, event *apimodel.ChatEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.Type, data)
}
