package v2handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// PublishChatStatusHandler publishes ephemeral chat activity.
//
//	@Summary		Publish chat status
//	@Description	Publishes encrypted ephemeral JSON on <chat_id>/status. Empty text is omitted so clients can show localized typing. data accepts any JSON value. The encoded payload is limited to 65,508 bytes. Status is not stored or replayed; receivers expire it. Success acknowledges acceptance, not delivery.
//	@Id				publish_chat_status
//	@Tags			Chat
//	@Accept			json
//	@Produce		json
//	@Param			space_id		path		string						true	"Space id"
//	@Param			chat_id			path		string						true	"Chat or discussion id"
//	@Param			status			body		object						false	"Optional text and arbitrary JSON data; clients localize the default typing label"
//	@Param			dry_run			query		bool						false	"Validate without publishing"
//	@Param			Idempotency-Key	header		string						false	"Unique key for this update; a reused key replays the response without publishing again"
//	@Success		200				{object}	v2model.ChatStatusResult	"Status accepted, or validated on a dry run"
//	@Failure		400				{object}	v2model.Error				"Invalid status"
//	@Failure		404				{object}	v2model.Error				"Space or chat not found"
//	@Failure		413				{object}	v2model.Error				"Request body or encoded pubsub payload too large"
//	@Failure		500				{object}	v2model.Error				"Publication failed"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/chats/{chat_id}/status [post]
func PublishChatStatusHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		req, err := decodeChatStatus(c.Request.Body)
		if err != nil {
			RespondError(c, err)
			return
		}
		result, err := s.PublishChatStatus(c.Request.Context(), c.Param("space_id"), c.Param("chat_id"), req, isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

// This body is optional, unlike stored chat messages. Only its data member is
// arbitrary JSON; the envelope remains strict and must be a single object.
func decodeChatStatus(r io.Reader) (req v2model.ChatStatusRequest, err error) {
	body, err := io.ReadAll(io.LimitReader(r, maxChatRequestBody+1))
	if err != nil {
		return req, v2model.ValidationFailed("read status request body", v2model.Issue{Message: err.Error()})
	}
	if len(body) > maxChatRequestBody {
		return req, v2model.RequestTooLarge(fmt.Sprintf("chat status request body exceeds the %d-byte limit", maxChatRequestBody))
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return req, nil
	}
	if body[0] != '{' {
		return req, v2model.ValidationFailed("status body must be a JSON object")
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		issue := v2model.Issue{Message: err.Error(), Hint: "send optional text and data; omit text for a localized typing label"}
		if field, ok := unknownFieldName(err); ok {
			issue.Path = "/" + field
		}
		return req, v2model.ValidationFailed("invalid status request body", issue)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return req, v2model.ValidationFailed("status body must contain exactly one JSON object")
	}
	return req, nil
}
