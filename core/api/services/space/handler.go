package space

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// GetSpacesHandler retrieves a list of spaces
//
//	@Summary	List spaces
//	@Tags		spaces
//	@Accept		json
//	@Produce	json
//	@Param		offset	query		int									false	"The number of items to skip before starting to collect the result set"
//	@Param		limit	query		int									false	"The number of items to return"	default(100)
//	@Success	200		{object}	pagination.PaginatedResponse[Space]	"List of spaces"
//	@Failure	401		{object}	util.UnauthorizedError				"Unauthorized"
//	@Failure	500		{object}	util.ServerError					"Internal server error"
//	@Router		/spaces [get]
func GetSpacesHandler(s *SpaceService) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		spaces, total, hasMore, err := s.ListSpaces(c.Request.Context(), offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrFailedListSpaces, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedOpenWorkspace, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		pagination.RespondWithPagination(c, http.StatusOK, spaces, total, offset, limit, hasMore)
	}
}

// CreateSpaceHandler creates a new space
//
//	@Summary	Create space
//	@Tags		spaces
//	@Accept		json
//	@Produce	json
//	@Param		name	body		string					true	"Space Name"
//	@Success	200		{object}	CreateSpaceResponse		"Space created successfully"
//	@Failure	400		{object}	util.ValidationError	"Bad request"
//	@Failure	401		{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure	500		{object}	util.ServerError		"Internal server error"
//	@Router		/spaces [post]
func CreateSpaceHandler(s *SpaceService) gin.HandlerFunc {
	return func(c *gin.Context) {
		nameRequest := CreateSpaceRequest{}
		if err := c.BindJSON(&nameRequest); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		space, err := s.CreateSpace(c.Request.Context(), nameRequest.Name)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrFailedCreateSpace, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, CreateSpaceResponse{Space: space})
	}
}

// GetMembersHandler retrieves a list of members in a space
//
//	@Summary	List members
//	@Tags		spaces
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string									true	"Space ID"
//	@Param		offset		query		int										false	"The number of items to skip before starting to collect the result set"
//	@Param		limit		query		int										false	"The number of items to return"	default(100)
//	@Success	200			{object}	pagination.PaginatedResponse[Member]	"List of members"
//	@Failure	401			{object}	util.UnauthorizedError					"Unauthorized"
//	@Failure	500			{object}	util.ServerError						"Internal server error"
//	@Router		/spaces/{space_id}/members [get]
func GetMembersHandler(s *SpaceService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		members, total, hasMore, err := s.ListMembers(c.Request.Context(), spaceId, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrFailedListMembers, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		pagination.RespondWithPagination(c, http.StatusOK, members, total, offset, limit, hasMore)
	}
}
