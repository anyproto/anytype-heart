package v2handler

// edit.go holds the edit handler. PATCH is
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
//	@Summary		Update an object
//	@Description	Edits in place, keeping the id and links; to change the type, use set_type rather than recreating the object. Applies ops in order as one atomic edit. A failed op or invalid result leaves the object unchanged. Text locators must match exactly one block. New block and view IDs are returned by payload position in `created_blocks` and `created_views`, compact by default or full with `ids=full`.
//	@Id				patch_object
//	@Tags			Objects
//	@Accept			json
//	@Produce		json
//	@Param			space_id				path		string				true	"Space id, from GET /v2/spaces"
//	@Param			object_id				path		string				true	"Object id, from POST /v2/spaces/{space_id}/search or GET /v2/spaces/{space_id}/objects"
//	@Param			If-Match				header		string				false	"The etag the object must still carry"
//	@Param			dry_run					query		bool				false	"Validate and report without committing"
//	@Param			ids						query		string				false	"ID spelling in created_blocks and created_views"	Enums(compact,full)	default(compact)
//	@Param			create_missing_options	query		bool				false	"Create select options for names the property does not hold yet (default false: an unmatched name is refused)"
//	@Param			body					body		object				true	"Ops envelope, an array of ops applied as one edit"
//	@Success		200						{object}	v2model.EditResult	"New etag + created block ids + diff_stats"
//	@Failure		400						{object}	v2model.Error		"Invalid ops or post-op document"
//	@Failure		404						{object}	v2model.Error		"Object, space, or referenced block not found"
//	@Failure		409						{object}	v2model.Error		"Stale If-Match (etag_mismatch)"
//	@Failure		422						{object}	v2model.Error		"Existing content cannot be represented losslessly for this edit"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/objects/{object_id} [patch]
func PatchObjectHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		body := readV2Body(c)
		if body == nil {
			return
		}
		result, err := s.PatchObject(c.Request.Context(), c.Param("space_id"), c.Param("object_id"), body, c.GetHeader("If-Match"), isV2DryRun(c), mayCreateMissingOptions(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondV2Edit(c, result)
	}
}

// DeleteObjectHandler archives an object, or with permanent=true deletes
// one the calling key created
//
//	@Summary		Delete an object
//	@Description	Moves the object to the Bin; restore_object or the app brings it back. Any user content the key can write can be archived; the receipt says if this key may delete it for good. permanent=true does that, archived or not, only for what this key created, with no other member's changes and no discussion; a file must be unused by other objects (chat attachments unchecked). Dry runs skip in-write checks.
//	@Id				delete_object
//	@Tags			Objects
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id, from GET /v2/spaces"
//	@Param			object_id	path		string					true	"Object id, from POST /v2/spaces/{space_id}/search or GET /v2/spaces/{space_id}/objects"
//	@Param			permanent	query		bool					false	"Delete for good instead of moving to the Bin (default false)"
//	@Param			dry_run		query		bool					false	"Probe deletability without writing"
//	@Success		200			{object}	v2model.CreateResult	"Archived or permanently deleted object, or the dry-run verdict. Archiving again is a 200 carrying a warning."
//	@Failure		400			{object}	v2model.Error			"A type or a property (use their own delete routes), skip_bin, or an invalid permanent value"
//	@Failure		403			{object}	v2model.Error			"A system object, or a permanent delete refused: not_created_by_this_key names the recorded creator or its absence"
//	@Failure		404			{object}	v2model.Error			"Object or space not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/objects/{object_id} [delete]
func DeleteObjectHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		// skip_bin is the spelling other APIs use for this; refused rather
		// than ignored, because ignoring it archived with a 200 that read as
		// the hard delete the caller asked for
		if _, ok := c.GetQuery("skip_bin"); ok {
			RespondError(c, v2model.ValidationFailed("unknown query parameter skip_bin",
				v2model.Issue{Path: "skip_bin", Message: "this route has no skip_bin parameter"}.
					Hintf("to delete for good, send %s instead", v2model.Resend("permanent", "true"))))
			return
		}
		var permanent bool
		switch c.Query("permanent") {
		case "", "false":
		case "true":
			permanent = true
		default:
			RespondError(c, v2model.ValidationFailed("invalid permanent value",
				v2model.Issue{Path: "permanent", Message: "allowed values: true, false"}))
			return
		}
		result, err := s.DeleteObject(c.Request.Context(), c.Param("space_id"), c.Param("object_id"), isV2DryRun(c), permanent)
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

// RestoreObjectHandler moves an archived object out of the Bin
//
//	@Summary		Restore an object
//	@Description	Moves an archived object out of the Bin. Any user content the key can write can be restored, whoever archived it. Restoring an object that is not archived is a 200 carrying a warning. Types and properties are restored in the Anytype app.
//	@Id				restore_object
//	@Tags			Objects
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id, from GET /v2/spaces"
//	@Param			object_id	path		string					true	"Object id, such as the one a delete_object receipt returned"
//	@Param			dry_run		query		bool					false	"Check the restore without writing"
//	@Success		200			{object}	v2model.CreateResult	"Restored object, or the dry-run verdict"
//	@Failure		400			{object}	v2model.Error			"A type, a property or a tag option"
//	@Failure		403			{object}	v2model.Error			"A system object"
//	@Failure		404			{object}	v2model.Error			"Object or space not found, or the object was deleted permanently"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/objects/{object_id}/restore [post]
func RestoreObjectHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := s.RestoreObject(c.Request.Context(), c.Param("space_id"), c.Param("object_id"), isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}
