package v2handler

// chat_space_stream.go is the Server-Sent Events transport of the space-wide
// chat stream. It frames what the hub renders and keeps the connection alive;
// it never ends the stream on its own. Only the client hanging up (or a write
// to a dead socket failing) ends it.

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// SpaceChatStreamHandler streams every chat of a space over Server-Sent Events
//
//	@Summary		Stream a space's chats
//	@Description	Opens with a chat_added event per chat, then snapshot_complete, then live chat, state and message events for every chat. Events carry no id and there is no replay: keep the state_id of the newest message per chat and catch up a chat through its message stream with Last-Event-ID. A message event can arrive twice; dedupe on its id.
//	@Id				stream_space_chats
//	@Tags			Chat
//	@Produce		text/event-stream
//	@Param			space_id	path		string			true	"Space id"
//	@Param			include		query		string			false	"discussions adds object discussions; none streams chats only"	Enums(discussions, none)	default(discussions)
//	@Param			heartbeat	query		int				false	"Keepalive cadence in seconds"									default(30)					minimum(1)	maximum(60)
//	@Success		200			{string}	string			"Server-Sent Events stream"
//	@Failure		400			{object}	v2model.Error	"Unknown include value"
//	@Failure		404			{object}	v2model.Error	"Space not found"
//	@Failure		429			{object}	v2model.Error	"Too many streams held at once"
//	@Failure		500			{object}	v2model.Error	"A chat of the space cannot be attached"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/chats/stream [get]
func SpaceChatStreamHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, err := parseSpaceChatStreamQuery(c)
		if err != nil {
			RespondError(c, err)
			return
		}
		ctx := c.Request.Context()
		// every refusal happens here, before the first byte
		stream, err := s.OpenSpaceChatStream(ctx, c.Param("space_id"), query)
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

		if err := writeSpaceChatFrames(c, stream.Snapshot()); err != nil {
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
				if err := writeSpaceChatFrames(c, stream.Drain()); err != nil {
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

// parseSpaceChatStreamQuery reads ?include=. Discussions is the default on
// this route; any value but discussions or none is a 400 naming it.
func parseSpaceChatStreamQuery(c *gin.Context) (v2service.SpaceChatStreamQuery, error) {
	switch include := c.Query("include"); include {
	case "", v2model.ChatIncludeDiscussions:
		return v2service.SpaceChatStreamQuery{IncludeDiscussions: true}, nil
	case v2model.ChatIncludeNone:
		return v2service.SpaceChatStreamQuery{}, nil
	default:
		return v2service.SpaceChatStreamQuery{}, v2model.ValidationFailed("invalid include value",
			v2model.Issue{Path: "include", Message: fmt.Sprintf("unknown value %q", include),
				Hint: fmt.Sprintf("allowed: %s, %s", v2model.ChatIncludeDiscussions, v2model.ChatIncludeNone)})
	}
}

// writeSpaceChatFrames writes each frame as one SSE event with no id: the
// space stream has no replay. One write per frame, so a failure cannot leave
// half a frame as the tail of the stream; the error ends the stream.
func writeSpaceChatFrames(c *gin.Context, frames []apicore.SpaceChatFrame) error {
	for _, frame := range frames {
		if err := writeSSEEvent(c, frame.Type, frame.Data); err != nil {
			return err
		}
	}
	return nil
}

// writeSSEEvent writes one SSE event with no id in a single write.
func writeSSEEvent(c *gin.Context, eventType string, data []byte) error {
	if _, err := io.WriteString(c.Writer, "event: "+eventType+"\ndata: "+string(data)+"\n\n"); err != nil {
		return fmt.Errorf("write %s event: %w", eventType, err)
	}
	return nil
}
