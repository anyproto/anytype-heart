package v2handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// RespondV2Error writes the C6 error envelope and aborts the request.
// Errors that are not *v2model.Error become 500 internal_error.
func RespondV2Error(c *gin.Context, err error) {
	var v2Err *v2model.Error
	if !errors.As(err, &v2Err) {
		v2Err = v2model.NewError(http.StatusInternalServerError, v2model.CodeInternalError, err.Error())
	}
	c.AbortWithStatusJSON(v2Err.Status, echoSpaceRef(c, v2Err))
}

// echoSpaceRef re-spells the resolved full space id back into the short
// reference the caller actually used (§8.35), across the message, every
// issue message and every hint. It runs HERE, at the one place a v2 error
// becomes bytes, rather than at the twenty Sprintf sites that interpolate a
// space id into a refusal or a repair URL — one hook cannot drift from
// nineteen others, and a message added tomorrow inherits it.
//
// It is a no-op unless the resolution middleware recorded an echo for this
// request, which it does only when the caller's spelling differed from the
// full id. Substituting a short reference into a repair URL keeps the URL
// valid: every /v2 route that takes a space id takes either spelling.
func echoSpaceRef(c *gin.Context, v2Err *v2model.Error) *v2model.Error {
	if c.Request == nil {
		return v2Err
	}
	full, ref, ok := v2service.SpaceEchoFromCtx(c.Request.Context())
	if !ok {
		return v2Err
	}
	echoed := *v2Err
	echoed.Message = strings.ReplaceAll(echoed.Message, full, ref)
	if len(v2Err.Issues) > 0 {
		echoed.Issues = make([]v2model.Issue, len(v2Err.Issues))
		for i, issue := range v2Err.Issues {
			issue.Message = strings.ReplaceAll(issue.Message, full, ref)
			issue.Hint = strings.ReplaceAll(issue.Hint, full, ref)
			echoed.Issues[i] = issue
		}
	}
	return &echoed
}

// decodeStrictJSONBody decodes a v2 request body strictly: unknown fields
// are 400s with the field named (C13's spirit at the request layer), an
// empty body 400s with the hint, and an oversized body 413s naming the
// surface. A false return means the error response was already written.
func decodeStrictJSONBody(c *gin.Context, into any, hint string, maxBody int64, surface string) bool {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBody+1))
	if err != nil {
		RespondV2Error(c, v2model.ValidationFailed("read request body",
			v2model.Issue{Message: err.Error()}))
		return false
	}
	if int64(len(body)) > maxBody {
		RespondV2Error(c, v2model.RequestTooLarge(
			fmt.Sprintf("%s request body exceeds the %d-byte limit", surface, maxBody)))
		return false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		RespondV2Error(c, v2model.ValidationFailed("request body is required",
			v2model.Issue{Message: hint}))
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		issue := v2model.Issue{Message: err.Error(), Hint: hint}
		if field, ok := unknownFieldName(err); ok {
			issue.Path = "/" + field
		}
		RespondV2Error(c, v2model.ValidationFailed("invalid request body", issue))
		return false
	}
	return true
}
