package v2handler

// spaces_stream.go is the Server-Sent Events transport of the spaces stream.
// It frames what the service renders and keeps the connection alive. The
// stream ends when the client hangs up, when a write fails, or when the
// service says the server ended it (the key was revoked or expired, or the
// server is stopping).

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// SpacesStreamHandler streams the spaces a key can see over Server-Sent Events
//
//	@Summary		Stream your spaces
//	@Description	The spaces list, held open. Opens with a space_added per space and snapshot_complete, then space_added, space_updated, space_removed and not_granted_updated as the answer changes, a grant edit included. Space ids are always full. stream_closed is the last event when the key is revoked or expires. Events carry no id: a reconnect is a fresh snapshot.
//	@Id				stream_spaces
//	@Tags			Spaces
//	@Produce		text/event-stream
//	@Param			ids			query		string			false	"full, the only shape this stream serves; compact is refused"
//	@Param			heartbeat	query		int				false	"Keepalive cadence in seconds"	default(30)	minimum(1)	maximum(60)
//	@Success		200			{string}	string			"Server-Sent Events stream"
//	@Failure		400			{object}	v2model.Error	"ids=compact (validation_failed)"
//	@Failure		429			{object}	v2model.Error	"Too many streams held at once"
//	@Security		bearerauth
//	@Router			/v2/spaces/stream [get]
//
// watchKey builds the key watch of the request's own key; a false return
// means it refused the key and wrote the response. refuseKey writes that
// same refusal for a key found revoked, expired or unverifiable while the
// stream opens.
func SpacesStreamHandler(s *v2service.Service, watchKey func(*gin.Context) (v2service.KeyWatch, bool), refuseKey func(*gin.Context, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		// A short space reference is unique only against the visible set at
		// one instant: a space joined while the stream is open can retire one
		// the stream already sent, and no client tracks a re-spelling of an
		// id it holds. A shape the stream cannot serve is refused, not
		// ignored. Any other unknown value was refused by the group already.
		if c.Query("ids") == "compact" {
			RespondError(c, v2model.ValidationFailed("the spaces stream serves full space ids only",
				v2model.Issue{
					Path:    "ids",
					Message: "compact is not available here: a short reference can stop being unique while the stream is open",
					Hint:    "omit ids, or send ids=full",
				}))
			return
		}
		// the watch is registered before the subscription opens, so a grant
		// edit can land before the snapshot or as a signal after it, never
		// between; a refusal here has written its own response
		watch, ok := watchKey(c)
		if !ok {
			return
		}
		defer watch.Close()

		ctx := c.Request.Context()
		stream, err := s.OpenSpacesStream(ctx, watch)
		if err != nil {
			// an edit or a revocation landed between the registration and
			// the snapshot, and the re-read found the key gone: the
			// authentication middleware's own 401, as for any refused key
			if errors.Is(err, v2service.ErrKeyRevoked) || errors.Is(err, v2service.ErrKeyExpired) || errors.Is(err, v2service.ErrKeyUnverified) {
				refuseKey(c, err)
				return
			}
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
				frames, ended := stream.Drain()
				if err := writeStreamFrames(c, frames); err != nil {
					return
				}
				c.Writer.Flush()
				if ended {
					return
				}
			case <-heartbeat.C:
				if _, err := fmt.Fprint(c.Writer, ": keepalive\n\n"); err != nil {
					return
				}
				c.Writer.Flush()
			}
		}
	}
}
