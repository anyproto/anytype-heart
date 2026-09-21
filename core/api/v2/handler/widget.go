package v2handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// widgetBodyHint steers a malformed widget body at the served schema.
func widgetBodyHint() v2model.Hint {
	return v2model.Hintf("the widget body is {target, scope, layout?, limit?, view_id?, after?, before?, position?} — %s for the schema", v2model.RefGetSchema("widget"))
}

// decodeWidgetBody is decodeStrictJSONBody with one more refusal: a member
// sent as an explicit null. The widget bodies tell absent from present by
// pointer, and the decoder reads a null into a nil pointer — the same as
// absent — while the served schema admits no null anywhere; a body that
// nulls view_id beside a valid limit would otherwise succeed and keep the
// view. A false return means the error response was already written.
func decodeWidgetBody(c *gin.Context, into any, hint v2model.Hint) bool {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxV2StructuredBodySize+1))
	if err != nil {
		RespondError(c, v2model.ValidationFailed("read request body", v2model.Issue{Message: err.Error()}))
		return false
	}
	var members map[string]json.RawMessage
	if int64(len(body)) <= maxV2StructuredBodySize && json.Unmarshal(body, &members) == nil {
		for name, raw := range members {
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				RespondError(c, v2model.ValidationFailed("invalid request body",
					v2model.Issue{Path: "/" + name, Message: "null is not a value here; omit the member to leave it alone, or send an empty string where one is accepted"}.WithHint(hint)))
				return false
			}
		}
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	return decodeStrictJSONBody(c, into, hint, maxV2StructuredBodySize, "widget")
}

// ListWidgetsHandler lists a space's sidebar widgets
//
//	@Summary		List widgets
//	@Description	The sidebar in its order: the space widgets every member sees, then this account's personal ones. `scope` narrows to one of the two. A built-in listing is a target spelled with a leading underscore, such as _favorite or _bin; an object id is served as it is.
//	@Id				list_widgets
//	@Tags			Widgets
//	@Produce		json
//	@Param			space_id	path		string									true	"Space id"
//	@Param			scope		query		string									false	"space or personal; omitted lists both"
//	@Param			offset		query		int										false	"Items to skip"		default(0)
//	@Param			limit		query		int										false	"Items to return"	default(25)
//	@Success		200			{object}	v2model.ListResponse[v2model.WidgetRow]	"Widget rows"
//	@Failure		400			{object}	v2model.Error							"Unknown scope"
//	@Failure		404			{object}	v2model.Error							"Space not found or unavailable"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/widgets [get]
func ListWidgetsHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, warnings, err := s.ListWidgets(c.Request.Context(), c.Param("space_id"), c.Query("scope"), offset, limit)
		if err != nil {
			RespondError(c, err)
			return
		}
		resp := v2model.NewListResponse(rows, total, offset, limit, hasMore, "narrow with scope= or request the next offset")
		resp.Warnings = warnings
		c.JSON(http.StatusOK, resp)
	}
}

// CreateWidgetHandler adds a widget to a sidebar
//
//	@Summary		Create a widget
//	@Description	Adds a widget for an object to the space sidebar (scope space, owner and admins) or to this account's own (scope personal, any member who can write). One widget per target per scope. The layout and limit follow the app's own rules: a layout the target cannot render, or a limit off the app's list, is replaced with the app's fallback and reported in warnings.
//	@Id				create_widget
//	@Tags			Widgets
//	@Accept			json
//	@Produce		json
//	@Param			space_id		path		string					true	"Space id"
//	@Param			dry_run			query		bool					false	"Validate and report without committing"
//	@Param			Idempotency-Key	header		string					false	"Replay guard: the same key with the same body replays the stored response"
//	@Param			body			body		object					true	"Widget to create"
//	@Success		201				{object}	v2model.WidgetResult	"The widget as stored"
//	@Failure		400				{object}	v2model.Error			"Validation failure: the target is missing, archived, a property, a template or a built-in listing, or the sidebar already holds it"
//	@Failure		403				{object}	v2model.Error			"This account cannot write the sidebar: the space sidebar takes the owner or an admin"
//	@Failure		413				{object}	v2model.Error			"Request body exceeds the 1 MiB cap"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/widgets [post]
func CreateWidgetHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req v2model.CreateWidgetRequest
		if !decodeWidgetBody(c, &req, widgetBodyHint()) {
			return
		}
		result, err := s.CreateWidget(c.Request.Context(), c.Param("space_id"), req, isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondWidget(c, result, http.StatusCreated)
	}
}

// UpdateWidgetHandler changes or moves a widget
//
//	@Summary		Update a widget
//	@Description	Changes the layout, the limit or the view of a widget, or moves it with after, before or position. The path names the widget by its id or by its target; a target the sidebar holds in both scopes needs `scope`. The target itself does not change: delete the widget and create one for the other object.
//	@Id				update_widget
//	@Tags			Widgets
//	@Accept			json
//	@Produce		json
//	@Param			space_id		path		string					true	"Space id"
//	@Param			widget_id		path		string					true	"Widget id, or the id of the object the widget points at"
//	@Param			scope			query		string					false	"space or personal, when the target is in both"
//	@Param			dry_run			query		bool					false	"Validate and report without committing"
//	@Param			Idempotency-Key	header		string					false	"Replay guard: the same key with the same body replays the stored response"
//	@Param			body			body		object					true	"The members to change"
//	@Success		200				{object}	v2model.WidgetResult	"The widget as stored"
//	@Failure		400				{object}	v2model.Error			"Validation failure, or a target held in both scopes without scope"
//	@Failure		403				{object}	v2model.Error			"This account cannot write the sidebar the widget is in"
//	@Failure		404				{object}	v2model.Error			"No widget with this id or target"
//	@Failure		413				{object}	v2model.Error			"Request body exceeds the 1 MiB cap"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/widgets/{widget_id} [patch]
func UpdateWidgetHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req v2model.UpdateWidgetRequest
		if !decodeWidgetBody(c, &req, v2model.Plain("the widget patch takes layout, limit, view_id, after, before, position; the target is identity and cannot change")) {
			return
		}
		result, err := s.UpdateWidget(c.Request.Context(), c.Param("space_id"), c.Param("widget_id"), c.Query("scope"), req, isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondWidget(c, result, http.StatusOK)
	}
}

// DeleteWidgetHandler removes a widget from a sidebar
//
//	@Summary		Delete a widget
//	@Description	Removes the widget from its sidebar. The object it pointed at is untouched. The path names the widget by its id or by its target; a target the sidebar holds in both scopes needs `scope`.
//	@Id				delete_widget
//	@Tags			Widgets
//	@Produce		json
//	@Param			space_id		path		string					true	"Space id"
//	@Param			widget_id		path		string					true	"Widget id, or the id of the object the widget points at"
//	@Param			scope			query		string					false	"space or personal, when the target is in both"
//	@Param			dry_run			query		bool					false	"Report without removing"
//	@Param			Idempotency-Key	header		string					false	"Replay guard: the same key replays the stored response"
//	@Success		200				{object}	v2model.WidgetResult	"The removed widget"
//	@Failure		400				{object}	v2model.Error			"A target held in both scopes without scope"
//	@Failure		403				{object}	v2model.Error			"This account cannot write the sidebar the widget is in"
//	@Failure		404				{object}	v2model.Error			"No widget with this id or target"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/widgets/{widget_id} [delete]
func DeleteWidgetHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := s.DeleteWidget(c.Request.Context(), c.Param("space_id"), c.Param("widget_id"), c.Query("scope"), isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondWidget(c, result, http.StatusOK)
	}
}

// respondWidget writes a widget mutation result: createdStatus on a real
// write, 200 on a dry run.
func respondWidget(c *gin.Context, result *v2model.WidgetResult, createdStatus int) {
	status := createdStatus
	if result.DryRun {
		status = http.StatusOK
	}
	c.JSON(status, result)
}
