package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/service"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pb"
)

const (
	defaultSSELimit          = 50
	sseChannelBufSize        = 256
	heartbeatHeader          = "Anytype-Heartbeat-Seconds"
	defaultHeartbeatSeconds  = 30
	minHeartbeatSeconds      = 1
	maxHeartbeatSeconds      = 60
)

// parseHeartbeatSeconds reads the Anytype-Heartbeat-Seconds header. Missing,
// malformed, or out-of-range values fall back to the default. The accepted
// range is [minHeartbeatSeconds, maxHeartbeatSeconds].
func parseHeartbeatSeconds(c *gin.Context) int {
	raw := c.GetHeader(heartbeatHeader)
	if raw == "" {
		return defaultHeartbeatSeconds
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < minHeartbeatSeconds || v > maxHeartbeatSeconds {
		return defaultHeartbeatSeconds
	}
	return v
}

// ChatStreamHandler streams chat events via Server-Sent Events
//
//	@Summary		Subscribe to chat messages (SSE)
//	@Description	Opens a Server-Sent Events stream for real-time chat updates. On connect, the last N messages are sent, followed by live events (message_added, message_updated, message_deleted, reactions_updated). Periodic SSE comment lines (`: heartbeat`) keep the connection alive during idle periods; per the SSE spec these are invisible to EventSource clients. Clients can tune the cadence with the Anytype-Heartbeat-Seconds header (1-60s, default 30s; out-of-range or unparsable values fall back to the default).
//	@Id				chat_message_stream
//	@Tags			Chat
//	@Produce		text/event-stream
//	@Param			Anytype-Version				header	string	true	"The version of the API to use"						default(2025-11-08)
//	@Param			Anytype-Heartbeat-Seconds	header	int		false	"Heartbeat interval in seconds (1-60, default 30)"	default(30)	minimum(1)	maximum(60)
//	@Param			space_id					path	string	true	"The ID of the space"
//	@Param			chat_id						path	string	true	"The ID of the chat object"
//	@Param			limit						query	int		false	"Number of recent messages to send on connect"	default(50)	minimum(1)	maximum(1000)
//	@Success		200							"SSE stream of chat events"
//	@Failure		400							{object}	util.ValidationError	"Bad request"
//	@Failure		401							{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		500							{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/chats/{chat_id}/messages/stream [get]
func ChatStreamHandler(s *service.Service, chatSubSvc apicore.ChatSubscriptionService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		chatId := c.Param("chat_id")
		heartbeatPeriod := time.Duration(parseHeartbeatSeconds(c)) * time.Second

		limit := defaultSSELimit
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil {
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

		initial := make([]apimodel.ChatMessage, 0, len(messages))
		for _, msg := range messages {
			initial = append(initial, apimodel.ChatMessageFromProto(msg))
		}
		s.EnrichChatMessageCreators(c.Request.Context(), spaceId, initial)
		for i := range initial {
			ev := &apimodel.ChatEvent{
				Type: "message_added",
				Payload: apimodel.ChatEventMessageAdded{
					Message: initial[i],
				},
			}
			writeSSE(c, ev)
		}
		c.Writer.Flush()

		ctx := c.Request.Context()
		heartbeat := time.NewTicker(heartbeatPeriod)
		defer heartbeat.Stop()

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
					if chatEvent == nil {
						continue
					}
					enrichEventMessage(ctx, s, spaceId, chatEvent)
					writeSSE(c, chatEvent)
				}
				c.Writer.Flush()
			case <-heartbeat.C:
				writeSSEComment(c, "heartbeat")
				c.Writer.Flush()
			}
		}
	}
}

// enrichEventMessage applies creator enrichment to events whose payload
// carries a ChatMessage. Other event types (deletions, reaction updates) are
// untouched.
func enrichEventMessage(ctx context.Context, s *service.Service, spaceId string, ev *apimodel.ChatEvent) {
	switch p := ev.Payload.(type) {
	case apimodel.ChatEventMessageAdded:
		msgs := []apimodel.ChatMessage{p.Message}
		s.EnrichChatMessageCreators(ctx, spaceId, msgs)
		ev.Payload = apimodel.ChatEventMessageAdded{Message: msgs[0]}
	case apimodel.ChatEventMessageUpdated:
		msgs := []apimodel.ChatMessage{p.Message}
		s.EnrichChatMessageCreators(ctx, spaceId, msgs)
		ev.Payload = apimodel.ChatEventMessageUpdated{Message: msgs[0]}
	}
}

func writeSSE(c *gin.Context, event *apimodel.ChatEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.Type, data)
}

// writeSSEComment emits an SSE comment line. Per the SSE spec, lines starting
// with ":" are ignored by clients (EventSource fires no event), so this acts
// as an invisible keepalive that prevents proxies and idle timeouts from
// dropping the connection without surfacing noise to consumers.
func writeSSEComment(c *gin.Context, text string) {
	fmt.Fprintf(c.Writer, ": %s\n\n", text)
}
