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

// ValidateHandler validates an AnyBlock document
//
//	@Summary		Validate an AnyBlock document
//	@Description	Structure and format rules only. Nothing is resolved against a space, so option names and a type's property keys are not checked here. Findings come back as data: an invalid document is still a 200, carrying the issues, and a valid one carries empty lists.
//	@Id				validate
//	@Tags			Schemas
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string						false	"Retry key; identical requests replay, different requests conflict"
//	@Success		200				{object}	v2model.ValidateResponse	"Issue and warning lists, empty when the document is valid"
//	@Failure		401				{object}	util.UnauthorizedError		"Missing or invalid key. This is the shared auth envelope, not this API's error shape."
//	@Failure		409				{object}	v2model.Error				"Idempotency-Key was already used with a different request"
//	@Security		bearerauth
//	@Router			/v2/validate [post]
func ValidateHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxValidateBodySize+1))
		if err != nil {
			RespondError(c, v2model.ValidationFailed("read request body: "+err.Error()))
			return
		}
		if len(body) > maxValidateBodySize {
			RespondError(c, v2model.ValidationFailed("request body exceeds the 10 MiB validation limit"))
			return
		}
		c.JSON(http.StatusOK, s.ValidateDocument(body))
	}
}
