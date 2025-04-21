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
//	@Tags			Spaces
//	@Produce		json
//	@Param			Anytype-Version	header		string								true	"The version of the API to use"											default(2025-04-22)
//	@Param			offset			query		int									false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int									false	"The number of items to return"											default(100)	maximum(1000)
//	@Success		200				{object}	pagination.PaginatedResponse[Space]	"List of spaces"
//	@Failure		401				{object}	util.UnauthorizedError				"Unauthorized"
//	@Failure		500				{object}	util.ServerError					"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces [get]
func GetSpacesHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		spaces, total, hasMore, err := s.ListSpaces(c.Request.Context(), offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrFailedListSpaces, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedOpenWorkspace, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedOpenSpace, http.StatusInternalServerError),
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
//	@Tags			Spaces
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Success		200				{object}	SpaceResponse			"Space"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Space not found"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id} [get]
func GetSpaceHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")

		space, err := s.GetSpace(c.Request.Context(), spaceId)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrWorkspaceNotFound, http.StatusNotFound),
			util.ErrToCode(ErrFailedOpenWorkspace, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedOpenSpace, http.StatusInternalServerError),
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
//	@Tags			Spaces
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			name			body		CreateSpaceRequest		true	"Space to create"
//	@Success		200				{object}	SpaceResponse			"Space created successfully"
//	@Failure		400				{object}	util.ValidationError	"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		423				{object}	util.RateLimitError		"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces [post]
func CreateSpaceHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateSpaceRequest
		if err := c.BindJSON(&req); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		space, err := s.CreateSpace(c.Request.Context(), req)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrFailedCreateSpace, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedSetSpaceInfo, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedOpenWorkspace, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedOpenSpace, http.StatusInternalServerError),
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
//	@Description	Returns a paginated list of members belonging to the specified space. Each member record includes the member’s profile ID, name, icon (which may be derived from an emoji or image), network identity, global name, status (e.g. joining, active) and role (e.g. Viewer, Editor, Owner). This endpoint supports collaborative features by allowing clients to show who is in a space and manage access rights.
//	@Tags			Members
//	@Produce		json
//	@Param			Anytype-Version	header		string									true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string									true	"Space ID"
//	@Param			offset			query		int										false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int										false	"The number of items to return"											default(100)	maximum(1000)
//	@Success		200				{object}	pagination.PaginatedResponse[Member]	"List of members"
//	@Failure		401				{object}	util.UnauthorizedError					"Unauthorized"
//	@Failure		500				{object}	util.ServerError						"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/members [get]
func GetMembersHandler(s Service) gin.HandlerFunc {
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
//	@Description	Fetches detailed information about a single member within a space. The endpoint returns the member’s identifier, name, icon, identity, global name, status and role. The member_id path parameter can be provided as either the member's ID (starting  with `_participant`) or the member's identity. This is useful for user profile pages, permission management, and displaying member-specific information in collaborative environments.
//	@Tags			Members
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			member_id		path		string					true	"Member ID or Identity"
//	@Success		200				{object}	MemberResponse			"Member"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Member not found"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/members/{member_id} [get]
func GetMemberHandler(s Service) gin.HandlerFunc {
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

/*
// UpdateMemberHandler updates a member in a space
//
//	@Summary		Update member
//	@Description	Modifies a member's status and role in a space. Use this endpoint to approve a joining member by setting the status to `active` and specifying a role (`reader` or `writer`), reject a joining member by setting the status to `declined`, remove a member by setting the status to `removed`, or update an active member's role. This endpoint enables fine-grained control over member access and permissions.
//	@Tags			Members
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			member_id		path		string					true	"Member ID"
//	@Param			body			body		UpdateMemberRequest		true	"Member to update"
//	@Success		200				{object}	MemberResponse			"Member updated successfully"
//	@Failure		400				{object}	util.ValidationError	"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Member not found"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/members/{member_id} [patch]
func UpdateMemberHandler(s *SpaceService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		memberId := c.Param("member_id")

		var req UpdateMemberRequest
		if err := c.BindJSON(&req); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.AbortWithStatusJSON(http.StatusBadRequest, apiErr)
		}

		member, err := s.UpdateMember(c.Request.Context(), spaceId, memberId, req)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrInvalidApproveMemberStatus, http.StatusBadRequest),
			util.ErrToCode(ErrInvalidApproveMemberRole, http.StatusBadRequest),
			util.ErrToCode(ErrMemberNotFound, http.StatusNotFound),
			util.ErrToCode(ErrFailedUpdateMember, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, MemberResponse{Member: member})
	}
}
*/
