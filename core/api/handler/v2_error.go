package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
)

// RespondV2Error writes the C6 error envelope and aborts the request.
// Errors that are not *apimodel.V2Error become 500 internal_error.
func RespondV2Error(c *gin.Context, err error) {
	var v2Err *apimodel.V2Error
	if !errors.As(err, &v2Err) {
		v2Err = apimodel.NewV2Error(http.StatusInternalServerError, apimodel.V2CodeInternalError, err.Error())
	}
	c.AbortWithStatusJSON(v2Err.Status, v2Err)
}
