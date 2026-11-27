package v2handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// GetSyncStatusHandler reads a space's sync status
//
//	@Summary		Get the sync status
//	@Description	App sync status and counts of pending objects, uploads and files over quota. Outside local_only, storage estimates the owner's quota (yours in one-to-one spaces) once the backup node reports it. notes give issues and fixes.
//	@Id				get_sync_status
//	@Tags			Spaces
//	@Produce		json
//	@Param			space_id	path		string				true	"Space id, from GET /v2/spaces"
//	@Param			objects		query		bool				false	"List out-of-sync objects (counts only by default)"
//	@Param			offset		query		int					false	"Objects to skip"	default(0)
//	@Param			limit		query		int					false	"Page size"			default(25)
//	@Success		200			{object}	v2model.SyncStatus	"Space sync status"
//	@Failure		400			{object}	v2model.Error		"Invalid objects or limit"
//	@Failure		404			{object}	v2model.Error		"Space missing or unavailable"
//	@Failure		500			{object}	v2model.Error		"Failed to read sync counts"
//	@Failure		501			{object}	v2model.Error		"Sync status unsupported"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/sync [get]
func GetSyncStatusHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var withObjects bool
		switch c.Query("objects") {
		case "", "false":
		case "true":
			withObjects = true
		default:
			RespondError(c, v2model.ValidationFailed("invalid objects value",
				v2model.Issue{Path: "objects", Message: "allowed values: true, false"}))
			return
		}
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		status, err := s.GetSyncStatus(c.Request.Context(), c.Param("space_id"), withObjects, offset, limit)
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, status)
	}
}
