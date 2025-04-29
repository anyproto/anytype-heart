package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/apimodel"
	"github.com/anyproto/anytype-heart/core/api/internal/search"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// GlobalSearchHandler searches and retrieves objects across all spaces
//
//	@Summary		Search objects across all spaces
//	@Description	This endpoint executes a global search over every space the user has access to. It accepts pagination parameters (offset and limit) and a JSON body containing search criteria. The criteria include a search query string, an optional list of object types, and sort options (e.g. ascending/descending by creation, modification, or last opened dates). Internally, the endpoint aggregates results from each space, merges and sorts them (after last modified date by default), and returns a unified, paginated list of objects that match the search parameters.
//	@Id				searchGlobal
//	@Tags			Search
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string										true	"The version of the API to use"											default(2025-04-22)
//	@Param			offset			query		int											false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int											false	"The number of items to return"											default(100)	maximum(1000)
//	@Param			request			body		SearchRequest								true	"Search parameters"
//	@Success		200				{object}	pagination.PaginatedResponse[object.Object]	"List of objects"
//	@Failure		401				{object}	util.UnauthorizedError						"Unauthorized"
//	@Failure		500				{object}	util.ServerError							"Internal server error"
//	@Security		bearerauth
//	@Router			/search [post]
func GlobalSearchHandler(s search.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		request := apimodel.SearchRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		objects, total, hasMore, err := s.GlobalSearch(c, request, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(search.ErrFailedSearchObjects, http.StatusInternalServerError),
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
//	@Summary		Search objects within a space
//	@Description	This endpoint performs a focused search within a single space (specified by the space_id path parameter). Like the global search, it accepts pagination parameters and a JSON payload containing the search query, object types, and sorting preferences. The search is limited to the provided space and returns a list of objects that match the query. This allows clients to implement space‑specific filtering without having to process extraneous results.
//	@Id				searchSpace
//	@Tags			Search
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string										true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string										true	"Space ID"
//	@Param			offset			query		int											false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int											false	"The number of items to return"											default(100)	maximum(1000)
//	@Param			request			body		SearchRequest								true	"Search parameters"
//	@Success		200				{object}	pagination.PaginatedResponse[object.Object]	"List of objects"
//	@Failure		401				{object}	util.UnauthorizedError						"Unauthorized"
//	@Failure		500				{object}	util.ServerError							"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/search [post]
func SearchHandler(s search.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceID := c.Param("space_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		request := apimodel.SearchRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		objects, total, hasMore, err := s.Search(c, spaceID, request, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(search.ErrFailedSearchObjects, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		pagination.RespondWithPagination(c, http.StatusOK, objects, total, offset, limit, hasMore)
	}
}
