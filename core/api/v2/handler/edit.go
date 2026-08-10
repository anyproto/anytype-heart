package v2handler

// edit.go holds the Phase-3 edit handler (APIV2.md §2 Phase 3). PATCH is
// the whole edit surface — snapshots are for creates, edits are ops
// (§8.27) — and takes its concurrency precondition from the If-Match
// header (C7, advisory).

import (
	"net/http"

	"github.com/gin-gonic/gin"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// respondV2Edit writes an edit result with the ETag header when known.
func respondV2Edit(c *gin.Context, result *v2model.EditResult) {
	if result.Etag != "" {
		c.Header("ETag", v2service.QuoteEtag(result.Etag))
	}
	c.JSON(http.StatusOK, result)
}

// PatchObjectV2Handler applies a batch of edit ops atomically
//
//	@Summary		Edit object (batched ops)
//	@Description	Applies the closed, id-addressed op set — setProperties, updateBlock, replaceSubtree, insertBlocks, moveBlock, deleteBlock, replaceText, setCell, updateView, insertView, moveView, deleteView, addItems, removeItems — atomically (one change set). Block references accept full ids and unique suffixes. The post-op document must satisfy the AnyBlock semantic checks (SPEC §12); any violation rejects the whole PATCH with path-addressed errors. If-Match is advisory (C7): absent = last-write-wins, stale = 409 with the current etag. Responds with the new etag, the created-block id map keyed by payload position, and diffStats. Honors ?dry_run=true (C9). GET /v2/schemas/ops/{op} documents each op.
//	@Id				v2_patch_object
//	@Tags			V2
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string				true	"Space id"
//	@Param			object_id	path		string				true	"Object id"
//	@Param			If-Match	header		string				false	"Advisory etag precondition (C7)"
//	@Param			dry_run		query		bool				false	"Validate and report without committing"
//	@Success		200			{object}	v2model.EditResult	"New etag + created block ids + diffStats"
//	@Failure		400			{object}	v2model.Error		"Invalid ops or post-op document"
//	@Failure		404			{object}	v2model.Error		"Object, space, or referenced block not found"
//	@Failure		409			{object}	v2model.Error		"Stale If-Match (etag_mismatch)"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/objects/{object_id} [patch]
func PatchObjectV2Handler(s *v2service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		body := readV2Body(c)
		if body == nil {
			return
		}
		result, err := s.PatchObject(c.Request.Context(), c.Param("space_id"), c.Param("object_id"), body, c.GetHeader("If-Match"), isV2DryRun(c))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		respondV2Edit(c, result)
	}
}
