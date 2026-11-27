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
//	@Description	The sync of the space's objects as the app shows it, what is not in sync, files not backed up for the storage limit, and the quota the space's files count against: the owner's, or in a one-to-one space the caller's. upgrade_needed is true only when a plan upgrade would fix a shortage the caller pays for. notes say what is wrong and what fixes it.
//	@Id				get_sync_status
//	@Tags			Spaces
//	@Produce		json
//	@Param			space_id	path		string				true	"Space id, from GET /v2/spaces"
//	@Param			ids			query		string				false	"compact (default) is the short space reference; full is the whole <cid>.<replicationKey> id, and the spelling to store outside this API"
//	@Param			count_only	query		bool				false	"Leave out the pending object list (default false)"
//	@Param			offset		query		int					false	"Pending objects to skip"	default(0)
//	@Param			limit		query		int					false	"Pending objects to return"	default(25)
//	@Success		200			{object}	v2model.SyncStatus	"The space's sync status"
//	@Failure		400			{object}	v2model.Error		"An invalid count_only or limit value"
//	@Failure		404			{object}	v2model.Error		"Space not found or unavailable"
//	@Failure		500			{object}	v2model.Error		"The sync counts could not be read"
//	@Failure		501			{object}	v2model.Error		"This server does not serve sync status"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/sync [get]
func GetSyncStatusHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var countOnly bool
		switch c.Query("count_only") {
		case "", "false":
		case "true":
			countOnly = true
		default:
			RespondError(c, v2model.ValidationFailed("invalid count_only value",
				v2model.Issue{Path: "count_only", Message: "allowed values: true, false"}))
			return
		}
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		status, err := s.GetSyncStatus(c.Request.Context(), c.Param("space_id"), countOnly, offset, limit)
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, status)
	}
}
