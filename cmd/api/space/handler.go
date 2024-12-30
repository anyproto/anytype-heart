package space

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/cmd/api/pagination"
)

// GetSpacesHandler retrieves a list of spaces
//
//	@Summary	Retrieve a list of spaces
//	@Tags		spaces
//	@Accept		json
//	@Produce	json
//	@Param		offset	query		int									false	"The number of items to skip before starting to collect the result set"
//	@Param		limit	query		int									false	"The number of items to return"	default(100)
//	@Success	200		{object}	pagination.PaginatedResponse[Space]	"List of spaces"
//	@Failure	403		{object}	api.UnauthorizedError				"Unauthorized"
//	@Failure	404		{object}	api.NotFoundError					"Resource not found"
//	@Failure	502		{object}	api.ServerError						"Internal server error"
//	@Router		/spaces [get]
func GetSpacesHandler(s *SpaceService) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		spaces, total, hasMore, err := s.ListSpaces(c.Request.Context(), offset, limit)
		if err != nil {
			switch {
			case errors.Is(err, ErrNoSpacesFound):
				c.JSON(http.StatusNotFound, gin.H{"message": "No spaces found."})
				return
			case errors.Is(err, ErrFailedListSpaces):
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve list of spaces."})
				return
			case errors.Is(err, ErrFailedOpenWorkspace):
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to open workspace."})
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}
		}

		pagination.RespondWithPagination(c, http.StatusOK, spaces, total, offset, limit, hasMore)
	}
}

// CreateSpaceHandler creates a new space
//
//	@Summary	Create a new Space
//	@Tags		spaces
//	@Accept		json
//	@Produce	json
//	@Param		name	body		string					true	"Space Name"
//	@Success	200		{object}	CreateSpaceResponse		"Space created successfully"
//	@Failure	403		{object}	api.UnauthorizedError	"Unauthorized"
//	@Failure	502		{object}	api.ServerError			"Internal server error"
//	@Router		/spaces [post]
func CreateSpaceHandler(s *SpaceService) gin.HandlerFunc {
	return func(c *gin.Context) {
		nameRequest := CreateSpaceRequest{}
		if err := c.BindJSON(&nameRequest); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid JSON"})
			return
		}
		name := nameRequest.Name

		space, err := s.CreateSpace(c.Request.Context(), name)
		if err != nil {
			switch {
			case errors.Is(err, ErrFailedCreateSpace):
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to create space."})
				return
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}
		}

		c.JSON(http.StatusOK, CreateSpaceResponse{Space: space})
	}
}

// GetMembersHandler retrieves a list of members for the specified space
//
//	@Summary	Retrieve a list of members for the specified Space
//	@Tags		spaces
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string									true	"The ID of the space"
//	@Param		offset		query		int										false	"The number of items to skip before starting to collect the result set"
//	@Param		limit		query		int										false	"The number of items to return"	default(100)
//	@Success	200			{object}	pagination.PaginatedResponse[Member]	"List of members"
//	@Failure	403			{object}	api.UnauthorizedError					"Unauthorized"
//	@Failure	404			{object}	api.NotFoundError						"Resource not found"
//	@Failure	502			{object}	api.ServerError							"Internal server error"
//	@Router		/spaces/{space_id}/members [get]
func GetMembersHandler(s *SpaceService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		members, total, hasMore, err := s.ListMembers(c.Request.Context(), spaceId, offset, limit)
		if err != nil {
			switch {
			case errors.Is(err, ErrNoMembersFound):
				c.JSON(http.StatusNotFound, gin.H{"message": "No members found."})
				return
			case errors.Is(err, ErrFailedListMembers):
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve list of members."})
				return
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}
		}

		pagination.RespondWithPagination(c, http.StatusOK, members, total, offset, limit, hasMore)
	}
}
