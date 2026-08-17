package v2handler

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// maxValidateBodySize bounds the /v2/validate request body.
const maxValidateBodySize = 10 << 20 // 10 MiB

// ValidateV2Handler validates an AnyBlock document
//
//	@Summary		Validate an AnyBlock document
//	@Description	Checks the request body against the AnyBlock JSON schema and the format's semantic rules. Structural and format-semantic only: referential checks against a space (option names, a type's property keys) are not performed here. Findings are returned as data — a valid document yields empty issue lists.
//	@Id				validate
//	@Tags			Schemas
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	v2model.ValidateResponse	"Issue and warning lists (empty when valid)"
//	@Failure		401	{object}	util.UnauthorizedError		"Missing or invalid key — the shared auth middleware's envelope (APIV2.md §8.9 seam), not the C6 shape"
//	@Security		bearerauth
//	@Router			/v2/validate [post]
func ValidateV2Handler(s *v2service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxValidateBodySize+1))
		if err != nil {
			RespondV2Error(c, v2model.ValidationFailed("read request body: "+err.Error()))
			return
		}
		if len(body) > maxValidateBodySize {
			RespondV2Error(c, v2model.ValidationFailed("request body exceeds the 10 MiB validation limit"))
			return
		}
		c.JSON(http.StatusOK, s.ValidateDocument(body))
	}
}
