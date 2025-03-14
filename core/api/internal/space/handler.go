package space

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// GetSpacesHandler retrieves a list of spaces
//
//	@Summary		List spaces
//	@Description	Retrieves a paginated list of all spaces that are accessible by the authenticated user. Each space record contains detailed information such as the space ID, name, icon (derived either from an emoji or image URL), and additional metadata. This endpoint is key to displaying a user’s workspaces.
//	@Tags			spaces
//	@Produce		json
//	@Param			offset	query		int									false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit	query		int									false	"The number of items to return"											default(100)	maximum(1000)
//	@Success		200		{object}	pagination.PaginatedResponse[Space]	"List of spaces"
//	@Failure		401		{object}	util.UnauthorizedError				"Unauthorized"
//	@Failure		500		{object}	util.ServerError					"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces [get]
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

// GetSpaceHandler retrieves a space
//
//	@Summary		Get space
//	@Description	Fetches full details about a single space identified by its space ID. The response includes metadata such as the space name, icon, and various workspace IDs (home, archive, profile, etc.). This detailed view supports use cases such as displaying space-specific settings.
//	@Tags			spaces
//	@Produce		json
//	@Param			space_id	path		string					true	"Space ID"
//	@Success		200			{object}	SpaceResponse			"Space"
//	@Failure		401			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404			{object}	util.NotFoundError		"Space not found"
//	@Failure		500			{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id} [get]
func GetSpaceHandler(s *SpaceService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")

		space, err := s.GetSpace(c.Request.Context(), spaceId)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrWorkspaceNotFound, http.StatusNotFound),
			util.ErrToCode(ErrFailedOpenWorkspace, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, SpaceResponse{Space: space})
	}
}

// CreateSpaceHandler creates a new space
//
//	@Summary		Create space
//	@Description	Creates a new workspace (or space) based on a supplied name in the JSON request body. The endpoint is subject to rate limiting and automatically applies default configurations such as generating a random icon and initializing the workspace with default settings (for example, a default dashboard or home page). On success, the new space’s full metadata is returned, enabling the client to immediately switch context to the new space.
//	@Tags			spaces
//	@Accept			json
//	@Produce		json
//	@Param			name	body		CreateSpaceRequest		true	"Space to create"
//	@Success		200		{object}	SpaceResponse			"Space created successfully"
//	@Failure		400		{object}	util.ValidationError	"Bad request"
//	@Failure		401		{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		423		{object}	util.RateLimitError		"Rate limit exceeded"
//	@Failure		500		{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces [post]
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

		c.JSON(http.StatusOK, SpaceResponse{Space: space})
	}
}

// GetMembersHandler retrieves a list of members in a space
//
//	@Summary		List members
//	@Description	Returns a paginated list of members belonging to the specified space. Each member record includes the member’s profile ID, name, icon (which may be derived from an emoji or image), network identity, global name, and role (e.g. Reader, Writer, Owner). This endpoint supports collaborative features by allowing clients to show who is in a space and manage access rights.
//	@Tags			members
//	@Produce		json
//	@Param			space_id	path		string									true	"Space ID"
//	@Param			offset		query		int										false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit		query		int										false	"The number of items to return"											default(100)	maximum(1000)
//	@Success		200			{object}	pagination.PaginatedResponse[Member]	"List of members"
//	@Failure		401			{object}	util.UnauthorizedError					"Unauthorized"
//	@Failure		500			{object}	util.ServerError						"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/members [get]
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

// GetMemberHandler retrieves a member in a space
//
//	@Summary		Get member
//	@Description	Fetches detailed information about a single member within a space. The endpoint returns the member’s identifier, name, icon, identity, global name, and role. This is useful for user profile pages, permission management, and displaying member-specific information in collaborative environments.
//	@Tags			members
//	@Produce		json
//	@Param			space_id	path		string					true	"Space ID"
//	@Param			member_id	path		string					true	"Member ID"
//	@Success		200			{object}	MemberResponse			"Member"
//	@Failure		401			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404			{object}	util.NotFoundError		"Member not found"
//	@Failure		500			{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/members/{member_id} [get]
func GetMemberHandler(s *SpaceService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		memberId := c.Param("member_id")

		member, err := s.GetMember(c.Request.Context(), spaceId, memberId)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrMemberNotFound, http.StatusNotFound),
			util.ErrToCode(ErrFailedGetMember, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, MemberResponse{Member: member})
	}
}
