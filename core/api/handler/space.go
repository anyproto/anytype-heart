package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/service"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// ListSpacesHandler retrieves a list of spaces
//
//	@Summary		List spaces
//	@Description	Retrieves a paginated list of all spaces that are accessible by the authenticated user. Each space record contains detailed information such as the space ID, name, icon (derived either from an emoji or image URL), and additional metadata. This endpoint is key to displaying a user's workspaces.
//	@Description	Supports dynamic filtering via query parameters (e.g., ?name[contains]=project, ?is_archived=false). See FilterCondition enum for available conditions.
//	@Id				list_spaces
//	@Tags			Spaces
//	@Produce		json
//	@Param			Anytype-Version	header		string											true	"The version of the API to use"											default(2025-05-20)
//	@Param			offset			query		int												false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int												false	"The number of items to return"											default(100)	maximum(1000)
//	@Success		200				{object}	pagination.PaginatedResponse[apimodel.Space]	"The list of spaces accessible by the authenticated user"
//	@Failure		401				{object}	util.UnauthorizedError							"Unauthorized"
//	@Failure		500				{object}	util.ServerError								"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces [get]
func ListSpacesHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		filtersAny, _ := c.Get("filters")
		filters := filtersAny.([]*model.BlockContentDataviewFilter)

		spaces, total, hasMore, err := s.ListSpaces(c.Request.Context(), filters, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedListSpaces, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedOpenWorkspace, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedOpenSpace, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		pagination.RespondWithPagination(c, http.StatusOK, spaces, total, offset, limit, hasMore)
	}
}

// GetSpaceHandler retrieves a space
//
//	@Summary		Get space
//	@Description	Fetches full details about a single space identified by its space ID. The response includes metadata such as the space name, icon, and various workspace IDs (home, archive, profile, etc.). This detailed view supports use cases such as displaying space-specific settings.
//	@Id				get_space
//	@Tags			Spaces
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string					true	"The ID of the space to retrieve; must be retrieved from ListSpaces endpoint"
//	@Success		200				{object}	apimodel.SpaceResponse	"The space details"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Space not found"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id} [get]
func GetSpaceHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")

		space, err := s.GetSpace(c.Request.Context(), spaceId)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrWorkspaceNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrFailedOpenWorkspace, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedOpenSpace, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.SpaceResponse{Space: space})
	}
}

// CreateSpaceHandler creates a new space
//
//	@Summary		Create space
//	@Description	Creates a new space based on a supplied name and description in the JSON request body. The endpoint is subject to rate limiting and automatically applies default configurations such as generating a random icon and initializing the workspace with default settings (for example, a default dashboard or home page). On success, the new space’s full metadata is returned, enabling the client to immediately switch context to the new internal.
//	@Id				create_space
//	@Tags			Spaces
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string						true	"The version of the API to use"	default(2025-05-20)
//	@Param			name			body		apimodel.CreateSpaceRequest	true	"The space to create"
//	@Success		201				{object}	apimodel.SpaceResponse		"The created space"
//	@Failure		400				{object}	util.ValidationError		"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError		"Unauthorized"
//	@Failure		429				{object}	util.RateLimitError			"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError			"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces [post]
func CreateSpaceHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req apimodel.CreateSpaceRequest
		if err := c.BindJSON(&req); err != nil {
			apiErr := util.CodeToApiError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		space, err := s.CreateSpace(c.Request.Context(), req)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedCreateSpace, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedSetSpaceInfo, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedOpenWorkspace, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedOpenSpace, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusCreated, apimodel.SpaceResponse{Space: space})
	}
}

// UpdateSpaceHandler updates a space
//
//	@Summary		Update space
//	@Description	Updates the name or description of an existing space. The request body should contain the new name and/or description in JSON format. This endpoint is useful for renaming or rebranding a workspace without needing to recreate it. The updated space’s metadata is returned in the response.
//	@Id				update_space
//	@Tags			Spaces
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string						true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string						true	"The ID of the space to update; must be retrieved from ListSpaces endpoint"
//	@Param			name			body		apimodel.UpdateSpaceRequest	true	"The space details to update"
//	@Success		200				{object}	apimodel.SpaceResponse		"The updated space"
//	@Failure		400				{object}	util.ValidationError		"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError		"Unauthorized"
//	@Failure		403				{object}	util.ForbiddenError			"Forbidden
//	@Failure		404				{object}	util.NotFoundError			"Space not found"
//	@Failure		429				{object}	util.RateLimitError			"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError			"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id} [patch]
func UpdateSpaceHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")

		var req apimodel.UpdateSpaceRequest
		if err := c.BindJSON(&req); err != nil {
			apiErr := util.CodeToApiError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		space, err := s.UpdateSpace(c.Request.Context(), spaceId, req)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedOpenWorkspace, http.StatusInternalServerError),
			util.ErrToCode(service.ErrWorkspaceNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrFailedSetSpaceInfo, http.StatusForbidden),
			util.ErrToCode(service.ErrFailedOpenSpace, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.SpaceResponse{Space: space})
	}
}
