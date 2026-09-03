package v2handler

// chat_stream.go is the Server-Sent Events transport for the chat message
// stream. Everything it decides is transport: framing, the keepalive, and
// when the connection ends. Scope, bounds and what a reconnecting client is
// owed all belong to the service.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/pagination"
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
//	@Description	Only additions are replayed on resume. A message deleted, edited, or reacted to while you were disconnected keeps its old form in your copy until you read the messages again, because none of those restamp the state id a resume is measured against.
//	@Id				stream_chat_messages
//	@Tags			Chat
//	@Produce		text/event-stream
//	@Param			space_id		path		string			true	"Space id"
//	@Param			chat_id			path		string			true	"Chat id"
//	@Param			limit			query		int				false	"Messages in the opening window"	default(25)	minimum(1)	maximum(1000)
//	@Param			heartbeat		query		int				false	"Keepalive cadence in seconds"		default(30)	minimum(1)	maximum(60)
//	@Param			Last-Event-ID	header		string			false	"Resume from this chat state id"
//	@Success		200				{string}	string			"Server-Sent Events stream"
//	@Failure		404				{object}	v2model.Error	"Space or chat not found"
//	@Failure		429				{object}	v2model.Error	"Too many streams held at once"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/chats/{chat_id}/messages/stream [get]
func ChatStreamHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		// the group's pagination middleware already parsed and BOUNDED limit
		// (it 400s anything outside 1..1000 before this runs), so reading its
		// result is what keeps one query parameter meaning one thing across
		// /v2 — a second Atoi here gave this route its own default
		limit := c.GetInt(pagination.QueryParamLimit)

		// every refusal this handler can make happens before the first byte.
		// Once the stream opens the status is committed, so a later failure
		// can only be a disconnect.
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
			if err := writeSSE(c, &v2model.ChatEvent{Type: v2model.ChatEventResyncRequired}); err != nil {
				return
			}
		}
		for i := range stream.Initial {
			if err := writeSSE(c, &stream.Initial[i]); err != nil {
				return
			}
		}
		// The window is emitted in order-id order, so the last id written is
		// not necessarily the highest state id in it. One trailing id-only
		// frame carrying the maximum leaves the client's cursor at the true
		// high-water mark. Per the SSE processing model the last-event-ID
		// buffer is set when the `id` field is parsed, and a frame with an
		// empty data buffer dispatches no event — so this moves the cursor
		// without inventing anything for the client to handle.
		if stream.Cursor != "" {
			if err := writeCursor(c, stream.Cursor); err != nil {
				return
			}
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
					// the producer dropped this subscription for reading too
					// slowly. Returning silently would look exactly like a
					// clean shutdown, so say what happened: reconnecting with
					// Last-Event-ID is the recovery, and resync_required is
					// already the word for "your copy is unverified".
					_ = writeSSE(c, &v2model.ChatEvent{Type: v2model.ChatEventResyncRequired})
					c.Writer.Flush()
					return
				}
				for _, msg := range event.Messages {
					chatEvent := v2model.ChatEventFromProto(msg, opts)
					if chatEvent == nil {
						continue
					}
					if err := writeSSE(c, chatEvent); err != nil {
						return
					}
				}
				c.Writer.Flush()
			case <-heartbeat.C:
				if _, err := fmt.Fprint(c.Writer, ": keepalive\n\n"); err != nil {
					return
				}
				c.Writer.Flush()
			}
		}
	}
}

// writeSSE frames one event. The `id:` line is what a client sends back as
// Last-Event-ID, so it is omitted for events that carry no resumable state:
// a deletion has no surviving row to name one, and an edit or a pin does not
// restamp the message's state id.
//
// The write error is RETURNED, not swallowed. Without it the loop keeps
// framing events into a half-closed socket until the request context happens
// to notice, and a marshal failure silently emits nothing, leaving the client
// a gap it has no way to detect.
func writeSSE(c *gin.Context, event *v2model.ChatEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal %s event: %w", event.Type, err)
	}
	// ONE write per frame: split across two, a failure in between leaves an
	// unterminated `id:` line as the tail of the stream. Type is a package
	// constant and Id is a locally minted state id, so neither can carry a
	// newline that would split the frame.
	frame := fmt.Sprintf("event: %s\ndata: %s\n\n", event.Type, data)
	if event.Id != "" {
		frame = fmt.Sprintf("id: %s\n", event.Id) + frame
	}
	if _, err := io.WriteString(c.Writer, frame); err != nil {
		return fmt.Errorf("write %s event: %w", event.Type, err)
	}
	return nil
}

// writeCursor emits a field-only frame that moves the client's resume point
// without dispatching an event. See the trailing-cursor note above.
func writeCursor(c *gin.Context, cursor string) error {
	if _, err := fmt.Fprintf(c.Writer, "id: %s\n\n", cursor); err != nil {
		return fmt.Errorf("write cursor: %w", err)
	}
	return nil
}
