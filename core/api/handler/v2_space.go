package handler

// v2_space.go holds the Phase-7 space handlers (APIV2_SURFACES.md §2):
// GET one space, create, update. The read is a tech-space store query (no
// WorkspaceOpen/ObjectShow RPC pair — the v1 N+1 shape). Both mutations
// honor Idempotency-Key (C8 — a retried space create without it duplicates
// an entire space) and ?dry_run=true (C9 — validate-only; a space create
// cannot be simulated).

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/service"
)

// maxSpaceRequestBody caps space mutation bodies: a space body is a name
// and a description.
const maxSpaceRequestBody = 1 << 20 // 1 MiB

// GetSpaceV2Handler reads one space
//
//	@Summary		Get space
//	@Description	Returns the space row {id, name, description} read from the tech space's space view — one store query, no workspace opens. gatewayUrl/networkId are client-infrastructure fields and are deliberately absent from v2.
//	@Id				v2_get_space
//	@Tags			V2
//	@Produce		json
//	@Param			space_id	path		string				true	"Space id"
//	@Success		200			{object}	apimodel.V2Space	"The space row"
//	@Failure		404			{object}	apimodel.V2Error	"Space not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id} [get]
func GetSpaceV2Handler(s *service.V2Service) gin.HandlerFunc {
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
//	@Description	Creates a space: {name, description?} → the same shape. Thin over WorkspaceCreate. Honors Idempotency-Key (C8) — an auto-retried space create without a key duplicates an entire space — and ?dry_run=true (C9): the dry run validates the body only, a space create cannot be simulated.
//	@Id				v2_create_space
//	@Tags			V2
//	@Accept			json
//	@Produce		json
//	@Param			dry_run			query		bool							false	"Validate the body without creating"
//	@Param			Idempotency-Key	header		string							false	"C8 replay guard: the same key with the same body replays the stored response"
//	@Param			request			body		apimodel.V2CreateSpaceRequest	true	"The space to create"
//	@Success		201				{object}	apimodel.V2Space				"Created space"
//	@Failure		400				{object}	apimodel.V2Error				"Validation failure"
//	@Security		bearerauth
//	@Router			/v2/spaces [post]
func CreateSpaceV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req apimodel.V2CreateSpaceRequest
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
//	@Description	Updates the space's name and/or description (omitted fields stay unchanged; at least one is required) → the resulting row. Thin over WorkspaceSetInfo. Honors Idempotency-Key (C8) and ?dry_run=true (C9).
//	@Id				v2_update_space
//	@Tags			V2
//	@Accept			json
//	@Produce		json
//	@Param			space_id		path		string							true	"Space id"
//	@Param			dry_run			query		bool							false	"Validate and report without committing"
//	@Param			Idempotency-Key	header		string							false	"C8 replay guard"
//	@Param			request			body		apimodel.V2UpdateSpaceRequest	true	"The fields to change"
//	@Success		200				{object}	apimodel.V2Space				"The updated space row"
//	@Failure		400				{object}	apimodel.V2Error				"Validation failure"
//	@Failure		404				{object}	apimodel.V2Error				"Space not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id} [patch]
func UpdateSpaceV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req apimodel.V2UpdateSpaceRequest
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
