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
//	@Summary		Edit object (batched ops)
//	@Description	Applies the closed, id-addressed op set — set_properties, update_block, replace_subtree, insert_blocks, move_block, delete_block, replace_text, set_cell, update_view, insert_view, move_view, delete_view, add_items, remove_items — atomically (one change set). EVERY id slot resolves the same way, in references and in payloads alike: a full id or a unique suffix, so a document echoed back from a read keeps its identity instead of being renamed. An id always names an EXISTING element: one that matches nothing is refused, and the ops that only ever create (insert_blocks) publish no id slot at all — omit it and the server mints one, reported in created_blocks under the payload path that produced it (nested row, column and cell-descendant slots included; minted view ids land in created_views). replace_text's id is itself optional: omitted, find doubles as the locator and must identify exactly ONE block — zero or several matching blocks refuse (the ambiguity refusal lists candidate ids), and resolution runs per-op under the object lock, so a later op resolves against the earlier ops' edits. update_block and delete_block take the same locator as `match` (exact text from the block) instead of an id — one of the two, never both, and the same one-match-or-refuse rule, which on delete_block is what stops a locator from removing the wrong subtree. The post-op document must satisfy the AnyBlock semantic checks (SPEC §12); any violation rejects the whole PATCH with path-addressed errors. If-Match is advisory (C7): absent = last-write-wins, stale = 409 with the current etag. Responds with the new etag, the minted-block id map keyed by payload position, and diff_stats. Honors ?dry_run=true (C9). GET /v2/schemas/ops/{op} documents each op.
//	@Id				patch_object
//	@Tags			Objects
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string				true	"Space id"
//	@Param			object_id	path		string				true	"Object id"
//	@Param			If-Match	header		string				false	"Advisory etag precondition (C7)"
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
		result, err := s.PatchObject(c.Request.Context(), c.Param("space_id"), c.Param("object_id"), body, c.GetHeader("If-Match"), isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondV2Edit(c, result)
	}
}

// DeleteObjectHandler archives an object the calling key created
//
//	@Summary		Delete object (archive, own output only)
//	@Description	Archives the object (moves it to Bin — reversible in the Anytype app; hard delete is a deferred ?permanent extension). Deletion is permitted ONLY for objects this API key created: provenance is recorded immutably on the object's creating change at creation time, so objects created before this route shipped, objects created in the Anytype app, imports and other members' objects all refuse with 403 not_created_by_this_key — for every key, permanently (fail-closed, no backfill). Only user content is deletable (pages, templates, files, chats); system objects refuse with 403, and types and properties refuse with a steer to their own DELETE routes. ?dry_run=true is the deletability probe: it verifies existence, grant and the provenance verdict without writing — archive-time restriction checks run only on the real call, so a deletable verdict can rarely still meet a 403. Honors Idempotency-Key; re-deleting an archived object is a 200 no-op with a warning.
//	@Id				delete_object
//	@Tags			Objects
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			object_id	path		string					true	"Object id"
//	@Param			dry_run		query		bool					false	"Probe deletability without writing"
//	@Success		200			{object}	v2model.CreateResult	"Archived object (or the dry-run verdict)"
//	@Failure		400			{object}	v2model.Error			"Type/property target — use their own DELETE routes"
//	@Failure		403			{object}	v2model.Error			"not_created_by_this_key: the recorded creator (or its absence) is named"
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
