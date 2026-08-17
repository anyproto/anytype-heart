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

// GetSpaceV2Handler reads one space
//
//	@Summary		Get space
//	@Description	Returns the space row {id, name, description} read from the tech space's space view — one store query, no workspace opens. Only LIVE spaces are served: a deleted, left or still-joining space 404s (the same predicate as the spaces list). gatewayUrl/networkId are client-infrastructure fields and are deliberately absent from v2.
//	@Id				get_space
//	@Tags			Spaces
//	@Produce		json
//	@Param			space_id	path		string			true	"Space id"
//	@Param			ids			query		string			false	"compact (default) = the short space reference; full = the full <cid>.<replicationKey> id — the export spelling, and the one to persist outside this API (a short reference is unique only against the spaces you can currently see)"
//	@Success		200			{object}	v2model.Space	"The space row"
//	@Failure		404			{object}	v2model.Error	"Space not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id} [get]
func GetSpaceV2Handler(s *v2service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		space, err := s.GetSpace(c.Request.Context(), c.Param("space_id"))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, space)
	}
}

// CreateSpaceV2Handler creates a space
//
//	@Summary		Create space
//	@Description	Creates a space: {name, description?} → the same shape. Thin over WorkspaceCreate. Both fields are capped at 4096 characters (the space kind's advertised maxLength — enforced). Honors Idempotency-Key (C8) — an auto-retried space create without a key duplicates an entire space — and ?dry_run=true (C9): the dry run validates the body only, a space create cannot be simulated.
//	@Id				create_space
//	@Tags			Spaces
//	@Accept			json
//	@Produce		json
//	@Param			dry_run			query		bool						false	"Validate the body without creating"
//	@Param			ids				query		string						false	"compact (default) = the short space reference; full = the full <cid>.<replicationKey> id of the created space — the spelling to persist outside this API"
//	@Param			Idempotency-Key	header		string						false	"C8 replay guard: the same key with the same body replays the stored response"
//	@Param			request			body		v2model.CreateSpaceRequest	true	"The space to create"
//	@Success		201				{object}	v2model.Space				"Created space"
//	@Failure		400				{object}	v2model.Error				"Validation failure"
//	@Security		bearerauth
//	@Router			/v2/spaces [post]
func CreateSpaceV2Handler(s *v2service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req v2model.CreateSpaceRequest
		if !decodeStrictJSONBody(c, &req, "the space body takes name and an optional description", maxSpaceRequestBody, "space") {
			return
		}
		dryRun := isV2DryRun(c)
		space, err := s.CreateSpace(c.Request.Context(), req, dryRun)
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		status := http.StatusCreated
		if dryRun {
			status = http.StatusOK
		}
		c.JSON(status, space)
	}
}

// UpdateSpaceV2Handler updates a space
//
//	@Summary		Update space
//	@Description	Updates the space's name and/or description (omitted fields stay unchanged; at least one is required; 4096-character cap) → the resulting row. Thin over WorkspaceSetInfo. A space that is deleted or gone answers 404; a role that may not change the space info answers 403. Honors Idempotency-Key (C8) and ?dry_run=true (C9).
//	@Id				update_space
//	@Tags			Spaces
//	@Accept			json
//	@Produce		json
//	@Param			space_id		path		string						true	"Space id"
//	@Param			dry_run			query		bool						false	"Validate and report without committing"
//	@Param			ids				query		string						false	"compact (default) = the short space reference; full = the full <cid>.<replicationKey> id — the spelling to persist outside this API"
//	@Param			Idempotency-Key	header		string						false	"C8 replay guard"
//	@Param			request			body		v2model.UpdateSpaceRequest	true	"The fields to change"
//	@Success		200				{object}	v2model.Space				"The updated space row"
//	@Failure		400				{object}	v2model.Error				"Validation failure"
//	@Failure		403				{object}	v2model.Error				"The caller's role cannot change the space info"
//	@Failure		404				{object}	v2model.Error				"Space not found or not live"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id} [patch]
func UpdateSpaceV2Handler(s *v2service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req v2model.UpdateSpaceRequest
		if !decodeStrictJSONBody(c, &req, "the update takes name and/or description — at least one", maxSpaceRequestBody, "space") {
			return
		}
		space, err := s.UpdateSpace(c.Request.Context(), c.Param("space_id"), req, isV2DryRun(c))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, space)
	}
}
