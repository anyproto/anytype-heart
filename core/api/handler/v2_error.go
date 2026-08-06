package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// decodeStrictJSONBody decodes a v2 request body strictly: unknown fields
// are 400s with the field named (C13's spirit at the request layer), an
// empty body 400s with the hint, and an oversized body 413s naming the
// surface. A false return means the error response was already written.
func decodeStrictJSONBody(c *gin.Context, into any, hint string, maxBody int64, surface string) bool {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBody+1))
	if err != nil {
		RespondV2Error(c, apimodel.V2ValidationFailed("read request body",
			apimodel.V2Issue{Message: err.Error()}))
		return false
	}
	if int64(len(body)) > maxBody {
		RespondV2Error(c, apimodel.V2RequestTooLarge(
			fmt.Sprintf("%s request body exceeds the %d-byte limit", surface, maxBody)))
		return false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		RespondV2Error(c, apimodel.V2ValidationFailed("request body is required",
			apimodel.V2Issue{Message: hint}))
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		issue := apimodel.V2Issue{Message: err.Error(), Hint: hint}
		if field, ok := unknownFieldName(err); ok {
			issue.Path = "/" + field
		}
		RespondV2Error(c, apimodel.V2ValidationFailed("invalid request body", issue))
		return false
	}
	return true
}
