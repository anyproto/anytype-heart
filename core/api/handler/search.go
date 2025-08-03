package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/service"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// GlobalSearchHandler searches and retrieves objects across all spaces
//
//	@Summary		Search objects across all spaces
//	@Description	Executes a global search over all spaces accessible to the authenticated user. The request body must specify the `query` text (currently matching only name and snippet of an object), optional filters on types (e.g., "page", "task"), and sort directives (default: descending by last modified date). Pagination is controlled via `offset` and `limit` query parameters to facilitate lazy loading in client UIs. The response returns a unified list of matched objects with their metadata and properties.
//	@Id				search_global
//	@Tags			Search
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string											true	"The version of the API to use"											default(2025-05-20)
//	@Param			offset			query		int												false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int												false	"The number of items to return"											default(100)	maximum(1000)
//	@Param			request			body		apimodel.SearchRequest							true	"The search parameters used to filter and sort the results"
//	@Success		200				{object}	pagination.PaginatedResponse[apimodel.Object]	"The list of objects matching the search criteria"
//	@Failure		401				{object}	util.UnauthorizedError							"Unauthorized"
//	@Failure		500				{object}	util.ServerError								"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/search [post]
func GlobalSearchHandler(s *service.Service) gin.HandlerFunc {
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
			util.ErrToCode(service.ErrFailedSearchObjects, http.StatusInternalServerError),
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
//	@Description	Performs a search within a single space (specified by the `space_id` path parameter). Like the global search, it accepts pagination parameters and a JSON payload containing the search `query`, `types`, and sorting preferences. The search is limited to the provided space and returns a list of objects that match the query. This allows clients to implement space‑specific filtering without having to process extraneous results.
//	@Id				search_space
//	@Tags			Search
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string											true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string											true	"The ID of the space to search in; must be retrieved from ListSpaces endpoint"
//	@Param			offset			query		int												false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int												false	"The number of items to return"											default(100)	maximum(1000)
//	@Param			request			body		apimodel.SearchRequest							true	"The search parameters used to filter and sort the results"
//	@Success		200				{object}	pagination.PaginatedResponse[apimodel.Object]	"The list of objects matching the search criteria"
//	@Failure		401				{object}	util.UnauthorizedError							"Unauthorized"
//	@Failure		500				{object}	util.ServerError								"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/search [post]
func SearchHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		request := apimodel.SearchRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		objects, total, hasMore, err := s.Search(c, spaceId, request, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedSearchObjects, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		pagination.RespondWithPagination(c, http.StatusOK, objects, total, offset, limit, hasMore)
	}
}
