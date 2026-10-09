package v2handler

// search_stream.go is the Server-Sent Events transport of the search stream.
// It frames what the service renders and keeps the connection alive; it
// never ends the stream on its own. Only the client hanging up (or a write
// to a dead socket failing) ends it.

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// SearchStreamHandler streams a space search over Server-Sent Events
//
//	@Summary		Stream a space search
//	@Description	The body is the space search request without `query`; nothing pages. Opens with an object_added per matching object and snapshot_complete, then object_added, object_updated (a carried field or the discussion changed) and object_removed (left the set, not necessarily deleted). Events carry no id: a reconnect is a fresh snapshot.
//	@Id				stream_space_search
//	@Tags			Search
//	@Accept			json
//	@Produce		text/event-stream
//	@Param			space_id	path		string			true	"Space id"
//	@Param			request		body		object			true	"Search request"
//	@Param			heartbeat	query		int				false	"Keepalive cadence in seconds"	default(30)	minimum(1)	maximum(60)
//	@Success		200			{string}	string			"Server-Sent Events stream"
//	@Failure		400			{object}	v2model.Error	"Invalid request (validation_failed / ambiguous_input)"
//	@Failure		404			{object}	v2model.Error	"Space not found"
//	@Failure		429			{object}	v2model.Error	"Too many streams held at once"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/search/stream [post]
func SearchStreamHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		req, ok := decodeSearchRequest(c)
		if !ok {
			return
		}
		ctx := c.Request.Context()
		// every refusal happens here, before the first byte
		stream, err := s.OpenSearchStream(ctx, c.Param("space_id"), req)
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

		if err := writeStreamFrames(c, stream.Snapshot()); err != nil {
			return
		}
		c.Writer.Flush()

		heartbeat := time.NewTicker(parseHeartbeatSeconds(c))
		defer heartbeat.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stream.Ready():
				if err := writeStreamFrames(c, stream.Drain()); err != nil {
					return
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

// writeStreamFrames writes each frame as one SSE event with no id, as
// writeSpaceChatFrames does.
func writeStreamFrames(c *gin.Context, frames []v2service.StreamFrame) error {
	for _, frame := range frames {
		if err := writeSSEEvent(c, frame.Type, frame.Data); err != nil {
			return err
		}
	}
	return nil
}
