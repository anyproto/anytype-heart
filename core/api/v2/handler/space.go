package v2handler

// space.go holds the Phase-7 space handlers (APIV2_SURFACES.md §2):
// GET one space, create, update. The read is a tech-space store query (no
// WorkspaceOpen/ObjectShow RPC pair — the v1 N+1 shape). Both mutations
// honor Idempotency-Key (C8 — a retried space create without it duplicates
// an entire space) and ?dry_run=true (C9 — validate-only; a space create
// cannot be simulated).

import (
	"net/http"

	"github.com/gin-gonic/gin"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// maxSpaceRequestBody caps space mutation bodies: a space body is a name
// and a description.
const maxSpaceRequestBody = 1 << 20 // 1 MiB

// GetSpaceHandler reads one space
//
//	@Summary		Get one space
//	@Description	Only live spaces are served. A space that is deleted, left, or still joining is a 404.
//	@Id				get_space
//	@Tags			Spaces
//	@Produce		json
//	@Param			space_id	path		string			true	"Space id"
//	@Param			ids			query		string			false	"compact (default) is the short space reference; full is the whole <cid>.<replicationKey> id, and the spelling to store outside this API"
//	@Success		200			{object}	v2model.Space	"The space row"
//	@Failure		404			{object}	v2model.Error	"Space not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id} [get]
func GetSpaceHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		space, err := s.GetSpace(c.Request.Context(), c.Param("space_id"))
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, space)
	}
}

// CreateSpaceHandler creates a space
//
//	@Summary		Create a space
//	@Description	A retry the server already handled makes a second space unless it carries the same idempotency key. A dry run validates the body and stops there; creating a space cannot be simulated.
//	@Id				create_space
//	@Tags			Spaces
//	@Accept			json
//	@Produce		json
//	@Param			dry_run			query		bool						false	"Validate the body without creating"
//	@Param			ids				query		string						false	"compact (default) is the short space reference; full is the whole <cid>.<replicationKey> id of the new space, and the spelling to store outside this API"
//	@Param			Idempotency-Key	header		string						false	"Replay guard: the same key with the same body replays the stored response"
//	@Param			request			body		v2model.CreateSpaceRequest	true	"The space to create"
//	@Success		201				{object}	v2model.Space				"Created space"
//	@Failure		400				{object}	v2model.Error				"Validation failure"
//	@Security		bearerauth
//	@Router			/v2/spaces [post]
func CreateSpaceHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req v2model.CreateSpaceRequest
		if !decodeStrictJSONBody(c, &req, "the space body takes name and an optional description", maxSpaceRequestBody, "space") {
			return
		}
		dryRun := isV2DryRun(c)
		space, err := s.CreateSpace(c.Request.Context(), req, dryRun)
		if err != nil {
			RespondError(c, err)
			return
		}
		status := http.StatusCreated
		if dryRun {
			status = http.StatusOK
		}
		c.JSON(status, space)
	}
}

// UpdateSpaceHandler updates a space
//
//	@Summary		Update a space
//	@Description	At least one of the two fields must be present; a field left out keeps its current value.
//	@Id				update_space
//	@Tags			Spaces
//	@Accept			json
//	@Produce		json
//	@Param			space_id		path		string						true	"Space id"
//	@Param			dry_run			query		bool						false	"Validate and report without committing"
//	@Param			ids				query		string						false	"compact (default) is the short space reference; full is the whole <cid>.<replicationKey> id, and the spelling to store outside this API"
//	@Param			Idempotency-Key	header		string						false	"Replay guard: the same key with the same body replays the stored response"
//	@Param			request			body		v2model.UpdateSpaceRequest	true	"The fields to change"
//	@Success		200				{object}	v2model.Space				"The updated space row"
//	@Failure		400				{object}	v2model.Error				"Validation failure"
//	@Failure		403				{object}	v2model.Error				"The caller's role cannot change the space info"
//	@Failure		404				{object}	v2model.Error				"Space not found or not live"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id} [patch]
func UpdateSpaceHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req v2model.UpdateSpaceRequest
		if !decodeStrictJSONBody(c, &req, "the update takes name and/or description — at least one", maxSpaceRequestBody, "space") {
			return
		}
		space, err := s.UpdateSpace(c.Request.Context(), c.Param("space_id"), req, isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, space)
	}
}
