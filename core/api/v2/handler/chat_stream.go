package v2handler

// chat_stream.go is the Server-Sent Events transport for the chat message
// stream. Everything it decides is transport: framing, the keepalive, and
// when the connection ends. Scope, bounds and what a reconnecting client is
// owed all belong to the service.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// Heartbeat bounds. The keepalive is an SSE COMMENT, which the spec says a
// client ignores, so it holds proxies and idle timeouts open without firing
// an event anyone has to filter.
const (
	defaultHeartbeatSeconds = 30
	minHeartbeatSeconds     = 1
	maxHeartbeatSeconds     = 60
)

// parseHeartbeatSeconds reads ?heartbeat. Unlike v1's header, this is a query
// parameter, which is how every other per-request knob on /v2 is spelled
// (?dry_run, ?ids, ?keys, ?limit). Out of range or unparsable falls back to
// the default rather than refusing: a keepalive cadence is not worth failing
// a stream over.
func parseHeartbeatSeconds(c *gin.Context) time.Duration {
	seconds := defaultHeartbeatSeconds
	if raw := c.Query("heartbeat"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v >= minHeartbeatSeconds && v <= maxHeartbeatSeconds {
			seconds = v
		}
	}
	return time.Duration(seconds) * time.Second
}

// ChatStreamHandler streams chat events over Server-Sent Events
//
//	@Summary		Stream a chat's messages
//	@Description	A resumed stream never replays deletions, so a message deleted while you were disconnected still appears in your copy until you read the messages again. Deleting removes the row, and nothing survives it to carry a state id.
//	@Id				stream_chat_messages
//	@Tags			Chat
//	@Produce		text/event-stream
//	@Param			space_id		path		string			true	"Space id"
//	@Param			chat_id			path		string			true	"Chat id"
//	@Param			limit			query		int				false	"Messages in the opening window"	default(50)	minimum(1)	maximum(1000)
//	@Param			heartbeat		query		int				false	"Keepalive cadence in seconds"		default(30)	minimum(1)	maximum(60)
//	@Param			Last-Event-ID	header		string			false	"Resume from this chat state id"
//	@Success		200				{string}	string			"Server-Sent Events stream"
//	@Failure		404				{object}	v2model.Error	"Space or chat not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/chats/{chat_id}/messages/stream [get]
func ChatStreamHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := 0
		if raw := c.Query("limit"); raw != "" {
			if v, err := strconv.Atoi(raw); err == nil {
				limit = v // the service owns the bound; this only parses
			}
		}

		// everything that can refuse happens BEFORE the first byte: once the
		// stream is open the status is committed and a failure can only be a
		// disconnect
		stream, err := s.OpenChatStream(c.Request.Context(), c.Param("space_id"), c.Param("chat_id"),
			v2service.ChatStreamQuery{Limit: limit, LastEventId: c.GetHeader("Last-Event-ID")})
		if err != nil {
			RespondError(c, err)
			return
		}
		defer stream.Close()

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no") // proxies must not buffer a stream
		c.Status(http.StatusOK)

		if stream.Resync {
			// first, so the client knows the window that follows is a fresh
			// read rather than a continuation of what it already held
			writeSSE(c, &v2model.ChatEvent{Type: v2model.ChatEventResyncRequired})
		}
		for i := range stream.Initial {
			writeSSE(c, &stream.Initial[i])
		}
		c.Writer.Flush()

		opts := s.ChatStreamMessageOptions(c.Request.Context(), c.Param("space_id"))
		heartbeat := time.NewTicker(parseHeartbeatSeconds(c))
		defer heartbeat.Stop()
		ctx := c.Request.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-stream.Events:
				if !ok {
					return
				}
				for _, msg := range event.Messages {
					if chatEvent := v2model.ChatEventFromProto(msg, opts); chatEvent != nil {
						writeSSE(c, chatEvent)
					}
				}
				c.Writer.Flush()
			case <-heartbeat.C:
				fmt.Fprint(c.Writer, ": keepalive\n\n")
				c.Writer.Flush()
			}
		}
	}
}

// writeSSE frames one event. The `id:` line is what a client sends back as
// Last-Event-ID, so it is omitted for events that carry no resumable state
// (a deletion has no surviving row to name one).
func writeSSE(c *gin.Context, event *v2model.ChatEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	if event.Id != "" {
		fmt.Fprintf(c.Writer, "id: %s\n", event.Id)
	}
	fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.Type, data)
}
