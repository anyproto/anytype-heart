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

// PatchObjectHandler applies a batch of edit ops atomically
//
//	@Summary		Edit an object with a batch of ops
//	@Description	Ops apply in order as one change set. If one fails, or the result breaks the format's rules, none of them land. `update_block`, `delete_block` and `replace_text` can address a block by its exact text instead of an id; text matching zero or several blocks is refused, not guessed at. A later op sees the earlier ones' edits. Ops that only create take no id; the new ids come back in `created_blocks`.
//	@Id				patch_object
//	@Tags			Objects
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string				true	"Space id"
//	@Param			object_id	path		string				true	"Object id"
//	@Param			If-Match	header		string				false	"The etag the object must still carry"
//	@Param			dry_run		query		bool				false	"Validate and report without committing"
//	@Success		200			{object}	v2model.EditResult	"New etag + created block ids + diff_stats"
//	@Failure		400			{object}	v2model.Error		"Invalid ops or post-op document"
//	@Failure		404			{object}	v2model.Error		"Object, space, or referenced block not found"
//	@Failure		409			{object}	v2model.Error		"Stale If-Match (etag_mismatch)"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/objects/{object_id} [patch]
func PatchObjectHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		body := readV2Body(c)
		if body == nil {
			return
		}
		result, err := s.PatchObject(c.Request.Context(), c.Param("space_id"), c.Param("object_id"), body, c.GetHeader("If-Match"), isV2DryRun(c), mayCreateOptions(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondV2Edit(c, result)
	}
}

// DeleteObjectHandler archives an object the calling key created
//
//	@Summary		Delete an object this key created
//	@Description	Only objects this key created can be deleted. The creator is recorded at creation time and never added later, so objects made in the app, imported, made by another member, or made before this route shipped are refused for good. System objects are a 403 as well. A dry run reports the verdict without the checks that run at archive time, so a deletable verdict can still meet a 403.
//	@Id				delete_object
//	@Tags			Objects
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			object_id	path		string					true	"Object id"
//	@Param			dry_run		query		bool					false	"Probe deletability without writing"
//	@Success		200			{object}	v2model.CreateResult	"Archived object, or the dry-run verdict. Deleting again is a 200 carrying a warning."
//	@Failure		400			{object}	v2model.Error			"A type or a property: use their own delete routes"
//	@Failure		403			{object}	v2model.Error			"not_created_by_this_key, naming the recorded creator or its absence"
//	@Failure		404			{object}	v2model.Error			"Object or space not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/objects/{object_id} [delete]
func DeleteObjectHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := s.DeleteObject(c.Request.Context(), c.Param("space_id"), c.Param("object_id"), isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}
