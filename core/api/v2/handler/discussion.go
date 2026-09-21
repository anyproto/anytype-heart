package v2handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// CreateDiscussionHandler mints an object's discussion
//
//	@Summary		Start an object's discussion
//	@Description	A discussion is the comment thread under an object, and it is a chat: the returned id is the `chat_id` for every chat operation, where `reply_to` makes a threaded reply. An object that already has one answers 200 with the same id and no `created`, so the call is safe to repeat. An object read serves the id as `discussion`. Chats, types, templates and system objects cannot hold one.
//	@Id				create_discussion
//	@Tags			Chat
//	@Produce		json
//	@Param			space_id		path		string						true	"Space id"
//	@Param			object_id		path		string						true	"Object id"
//	@Param			dry_run			query		bool						false	"Report whether the object has a discussion without creating one"
//	@Param			Idempotency-Key	header		string						false	"Replay guard: the same key with the same body replays the stored response"
//	@Success		201				{object}	v2model.DiscussionResult	"Discussion created"
//	@Success		200				{object}	v2model.DiscussionResult	"The object already has a discussion, or a dry run"
//	@Failure		400				{object}	v2model.Error				"A chat, or an object that cannot hold a discussion"
//	@Failure		404				{object}	v2model.Error				"Object or space not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/objects/{object_id}/discussion [post]
func CreateDiscussionHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := s.CreateDiscussion(c.Request.Context(), c.Param("space_id"), c.Param("object_id"), isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		status := http.StatusOK
		if result.Created && !result.DryRun {
			status = http.StatusCreated
		}
		c.JSON(status, result)
	}
}
