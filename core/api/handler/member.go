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

// ListMembersHandler retrieves a list of members in a space
//
//	@Summary		List members
//	@Description	Returns a paginated list of members belonging to the specified space. Each member record includes the member's profile ID, name, icon (which may be derived from an emoji or image), network identity, global name, status (e.g. joining, active) and role (e.g. Viewer, Editor, Owner). This endpoint supports collaborative features by allowing clients to show who is in a space and manage access rights.
//	@Description	Supports dynamic filtering via query parameters (e.g., ?status=active, ?role[ne]=viewer, ?name[contains]=john). See FilterCondition enum for available conditions.
//	@Id				list_members
//	@Tags			Members
//	@Produce		json
//	@Param			Anytype-Version	header		string											true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string											true	"The ID of the space to list members for; must be retrieved from ListSpaces endpoint"
//	@Param			offset			query		int												false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int												false	"The number of items to return"											default(100)	maximum(1000)
//	@Success		200				{object}	pagination.PaginatedResponse[apimodel.Member]	"The list of members in the space"
//	@Failure		401				{object}	util.UnauthorizedError							"Unauthorized"
//	@Failure		500				{object}	util.ServerError								"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/members [get]
func ListMembersHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		filtersAny, _ := c.Get("filters")
		filters := filtersAny.([]*model.BlockContentDataviewFilter)

		members, total, hasMore, err := s.ListMembers(c.Request.Context(), spaceId, filters, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedListMembers, http.StatusInternalServerError),
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
//	@Id				get_member
//	@Tags			Members
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string					true	"The ID of the space to get the member from; must be retrieved from ListSpaces endpoint"
//	@Param			member_id		path		string					true	"Member ID or Identity; must be retrieved from ListMembers endpoint or obtained from response context"
//	@Success		200				{object}	apimodel.MemberResponse	"The member details"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Member not found"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/members/{member_id} [get]
func GetMemberHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		memberId := c.Param("member_id")

		member, err := s.GetMember(c.Request.Context(), spaceId, memberId)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrMemberNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrFailedGetMember, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.MemberResponse{Member: *member})
	}
}

/*
// UpdateMemberHandler updates a member in a space
//
//	@Summary		Update member
//	@Description	Modifies a member's status and role in a space. Use this endpoint to approve a joining member by setting the status to `active` and specifying a role (`reader` or `writer`), reject a joining member by setting the status to `declined`, remove a member by setting the status to `removed`, or update an active member's role. This endpoint enables fine-grained control over member access and permissions.
//	@Id				update_member
//	@Tags			Members
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string							true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string							true	"The ID of the space to update the member in; must be retrieved from ListSpaces endpoint"
//	@Param			member_id		path		string							true	"The ID or Identity of the member to update; must be retrieved from ListMembers endpoint or obtained from response context"
//	@Param			body			body		apimodel.UpdateMemberRequest	true	"The request body containing the member's new status and role"
//	@Success		200				{object}	apimodel.MemberResponse			"Member updated successfully"
//	@Failure		400				{object}	util.ValidationError			"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError			"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError				"Member not found"
//	@Failure		429				{object}	util.RateLimitError				"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError				"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/members/{member_id} [patch]
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
