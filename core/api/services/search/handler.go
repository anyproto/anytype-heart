package search

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// SearchHandler searches and retrieves objects across all the spaces
//
//	@Summary	Search objects across all spaces
//	@Tags		search
//	@Accept		json
//	@Produce	json
//	@Param		query			query		string						false	"Search query"
//	@Param		object_types	query		[]string					false	"Types to filter objects by"
//	@Param		offset			query		int							false	"The number of items to skip before starting to collect the result set"
//	@Param		limit			query		int							false	"The number of items to return"	default(100)
//	@Success	200				{object}	map[string][]object.Object	"List of objects"
//	@Failure	401				{object}	util.UnauthorizedError		"Unauthorized"
//	@Failure	500				{object}	util.ServerError			"Internal server error"
//	@Router		/search [get]
func SearchHandler(s *SearchService) gin.HandlerFunc {
	return func(c *gin.Context) {
		searchQuery := c.Query("query")
		objectTypes := c.QueryArray("object_types")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		objects, total, hasMore, err := s.Search(c, searchQuery, objectTypes, offset, limit)
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
