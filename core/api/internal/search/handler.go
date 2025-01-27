package search

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// GlobalSearchHandler searches and retrieves objects across all spaces
//
//	@Summary	Search objects across all spaces
//	@Tags		search
//	@Accept		json
//	@Produce	json
//	@Param		offset	query		int							false	"The number of items to skip before starting to collect the result set"
//	@Param		limit	query		int							false	"The number of items to return"	default(100)
//	@Param		request	body		SearchRequest				true	"Search parameters"
//	@Success	200		{object}	map[string][]object.Object	"List of objects"
//	@Failure	401		{object}	util.UnauthorizedError		"Unauthorized"
//	@Failure	500		{object}	util.ServerError			"Internal server error"
//	@Router		/search [post]
func GlobalSearchHandler(s *SearchService) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		request := SearchRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		objects, total, hasMore, err := s.GlobalSearch(c, request, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrFailedSearchObjects, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		pagination.RespondWithPagination(c, http.StatusOK, objects, total, offset, limit, hasMore)
	}
}

// SearchHandler searches and retrieves objects within a space
//
//	@Summary	Search objects within a space
//	@Tags		search
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string						true	"Space ID"
//	@Param		offset		query		int							false	"The number of items to skip before starting to collect the result set"
//	@Param		limit		query		int							false	"The number of items to return"	default(100)
//	@Param		request		body		SearchRequest				true	"Search parameters"
//	@Success	200			{object}	map[string][]object.Object	"List of objects"
//	@Failure	401			{object}	util.UnauthorizedError		"Unauthorized"
//	@Failure	500			{object}	util.ServerError			"Internal server error"
//	@Router		/spaces/{space_id}/search [post]
func SearchHandler(s *SearchService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceID := c.Param("space_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		request := SearchRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		objects, total, hasMore, err := s.Search(c, spaceID, request, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrFailedSearchObjects, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		pagination.RespondWithPagination(c, http.StatusOK, objects, total, offset, limit, hasMore)
	}
}
