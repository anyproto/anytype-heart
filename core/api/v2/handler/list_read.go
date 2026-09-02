package v2handler

// list_read.go holds the queries/collections read handlers. One service
// implementation branches on layout; a wrong-layout
// target is a 400 naming the other route.

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// listFieldsParam parses the shared fields= query param (C5 row expansion).
func listFieldsParam(c *gin.Context) []string {
	var fields []string
	if raw := c.Query("fields"); raw != "" {
		for _, f := range strings.Split(raw, ",") {
			if f = strings.TrimSpace(f); f != "" {
				fields = append(fields, f)
			}
		}
	}
	return fields
}

// GetQueryObjectsHandler lists the objects a query matches
//
//	@Summary		Run a query and list what it matches
//	@Description	A stored view's dynamic placeholders, such as the current date or the calling member, are resolved here. One that cannot be resolved becomes a warning rather than a silently empty result.
//	@Id				get_query_objects
//	@Tags			Lists
//	@Produce		json
//	@Param			space_id	path		string									true	"Space id"
//	@Param			query_id	path		string									true	"Query object id"
//	@Param			view		query		string									false	"Stored view id (exact or unique suffix)"
//	@Param			fields		query		string									false	"Comma-separated property keys to include per row"
//	@Param			offset		query		int										false	"Items to skip"		default(0)
//	@Param			limit		query		int										false	"Items to return"	default(25)
//	@Success		200			{object}	v2model.ListResponse[v2model.ObjectRow]	"Minimal object rows"
//	@Failure		400			{object}	v2model.Error							"Wrong-layout target or invalid params"
//	@Failure		404			{object}	v2model.Error							"Space, query or view not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/queries/{query_id}/objects [get]
func GetQueryObjectsHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, warnings, err := s.GetQueryObjects(c.Request.Context(),
			c.Param("space_id"), c.Param("query_id"), c.Query("view"), listFieldsParam(c), offset, limit)
		if err != nil {
			RespondError(c, err)
			return
		}
		resp := v2model.NewListResponse(rows, total, offset, limit, hasMore, v2service.SearchNarrowHint)
		resp.Warnings = warnings
		c.JSON(http.StatusOK, resp)
	}
}

// GetQueryViewsHandler lists a query's stored views
//
//	@Summary	List a query's views
//	@Id			get_query_views
//	@Tags		Lists
//	@Produce	json
//	@Param		space_id	path		string										true	"Space id"
//	@Param		query_id	path		string										true	"Query object id"
//	@Param		offset		query		int											false	"Items to skip"		default(0)
//	@Param		limit		query		int											false	"Items to return"	default(25)
//	@Success	200			{object}	v2model.ListResponse[v2model.ViewObject]	"The stored views, with their sorts, filters and columns"
//	@Failure	400			{object}	v2model.Error								"Wrong-layout target"
//	@Failure	404			{object}	v2model.Error								"Space or query not found"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/queries/{query_id}/views [get]
func GetQueryViewsHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		views, total, hasMore, err := s.GetQueryViews(c.Request.Context(),
			c.Param("space_id"), c.Param("query_id"), offset, limit)
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, v2model.NewListResponse(views, total, offset, limit, hasMore,
			"request the next offset"))
	}
}

// GetCollectionObjectsHandler lists a collection's members
//
//	@Summary		List a collection's objects
//	@Description	Members come back in the order the collection stores them, not sorted, unless a view is applied.
//	@Id				get_collection_objects
//	@Tags			Lists
//	@Produce		json
//	@Param			space_id		path		string									true	"Space id"
//	@Param			collection_id	path		string									true	"Collection object id"
//	@Param			view			query		string									false	"Stored view id (exact or unique suffix)"
//	@Param			fields			query		string									false	"Comma-separated property keys to include per row"
//	@Param			offset			query		int										false	"Items to skip"		default(0)
//	@Param			limit			query		int										false	"Items to return"	default(25)
//	@Success		200				{object}	v2model.ListResponse[v2model.ObjectRow]	"Minimal object rows"
//	@Failure		400				{object}	v2model.Error							"Wrong-layout target or invalid params"
//	@Failure		404				{object}	v2model.Error							"Space, collection or view not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/collections/{collection_id}/objects [get]
func GetCollectionObjectsHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, warnings, err := s.GetCollectionObjects(c.Request.Context(),
			c.Param("space_id"), c.Param("collection_id"), c.Query("view"), listFieldsParam(c), offset, limit)
		if err != nil {
			RespondError(c, err)
			return
		}
		resp := v2model.NewListResponse(rows, total, offset, limit, hasMore, v2service.SearchNarrowHint)
		resp.Warnings = warnings
		c.JSON(http.StatusOK, resp)
	}
}

// GetCollectionViewsHandler lists a collection's stored views
//
//	@Summary	List a collection's views
//	@Id			get_collection_views
//	@Tags		Lists
//	@Produce	json
//	@Param		space_id		path		string										true	"Space id"
//	@Param		collection_id	path		string										true	"Collection object id"
//	@Param		offset			query		int											false	"Items to skip"		default(0)
//	@Param		limit			query		int											false	"Items to return"	default(25)
//	@Success	200				{object}	v2model.ListResponse[v2model.ViewObject]	"The stored views, with their sorts, filters and columns"
//	@Failure	400				{object}	v2model.Error								"Wrong-layout target"
//	@Failure	404				{object}	v2model.Error								"Space or collection not found"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/collections/{collection_id}/views [get]
func GetCollectionViewsHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		views, total, hasMore, err := s.GetCollectionViews(c.Request.Context(),
			c.Param("space_id"), c.Param("collection_id"), offset, limit)
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, v2model.NewListResponse(views, total, offset, limit, hasMore,
			"request the next offset"))
	}
}
