package v2handler

// list_read.go — the Phase-4 sets/collections read handlers (APIV2.md §2
// Phase 4). One service implementation branches on layout; a wrong-layout
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

// GetSetObjectsV2Handler lists the objects a set's query matches
//
//	@Summary		Get set objects
//	@Description	Executes the set's stored query (its set_of source) directly against the store, optionally through one stored view's filters and sorts (?view=, exact id or unique suffix). Stored-view execution substitutes the SPEC §6.2 dynamic placeholders server-side; unresolvable placeholders degrade to warnings, never a silent no-match. Rows are C5 minimal; fields= expands.
//	@Id				get_set_objects
//	@Tags			Lists
//	@Produce		json
//	@Param			space_id	path		string									true	"Space id"
//	@Param			set_id		path		string									true	"Set object id"
//	@Param			view		query		string									false	"Stored view id (exact or unique suffix)"
//	@Param			fields		query		string									false	"Comma-separated property keys to include per row"
//	@Param			offset		query		int										false	"Items to skip"		default(0)
//	@Param			limit		query		int										false	"Items to return"	default(25)
//	@Success		200			{object}	v2model.ListResponse[v2model.ObjectRow]	"Minimal object rows"
//	@Failure		400			{object}	v2model.Error							"Wrong-layout target or invalid params"
//	@Failure		404			{object}	v2model.Error							"Space, set or view not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/sets/{set_id}/objects [get]
func GetSetObjectsV2Handler(s *v2service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, warnings, err := s.GetSetObjects(c.Request.Context(),
			c.Param("space_id"), c.Param("set_id"), c.Query("view"), listFieldsParam(c), offset, limit)
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		resp := v2model.NewListResponse(rows, total, offset, limit, hasMore, v2service.V2SearchNarrowHint)
		resp.Warnings = warnings
		c.JSON(http.StatusOK, resp)
	}
}

// GetSetViewsV2Handler lists a set's stored views
//
//	@Summary		Get set views
//	@Description	Returns the set's stored views as SPEC §6.2 view objects (sorts, filters, columns — option names resolved, the format vocabulary). Paginated (C10).
//	@Id				get_set_views
//	@Tags			Lists
//	@Produce		json
//	@Param			space_id	path		string										true	"Space id"
//	@Param			set_id		path		string										true	"Set object id"
//	@Param			offset		query		int											false	"Items to skip"		default(0)
//	@Param			limit		query		int											false	"Items to return"	default(25)
//	@Success		200			{object}	v2model.ListResponse[v2model.ViewObject]	"§6.2 view objects"
//	@Failure		400			{object}	v2model.Error								"Wrong-layout target"
//	@Failure		404			{object}	v2model.Error								"Space or set not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/sets/{set_id}/views [get]
func GetSetViewsV2Handler(s *v2service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		views, total, hasMore, err := s.GetSetViews(c.Request.Context(),
			c.Param("space_id"), c.Param("set_id"), offset, limit)
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, v2model.NewListResponse(views, total, offset, limit, hasMore,
			"request the next offset"))
	}
}

// GetCollectionObjectsV2Handler lists a collection's members
//
//	@Summary		Get collection objects
//	@Description	Reads the collection's curated membership (the store slice, in its order), optionally through one stored view's filters and sorts (?view=). Rows are C5 minimal; fields= expands.
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
func GetCollectionObjectsV2Handler(s *v2service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, warnings, err := s.GetCollectionObjects(c.Request.Context(),
			c.Param("space_id"), c.Param("collection_id"), c.Query("view"), listFieldsParam(c), offset, limit)
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		resp := v2model.NewListResponse(rows, total, offset, limit, hasMore, v2service.V2SearchNarrowHint)
		resp.Warnings = warnings
		c.JSON(http.StatusOK, resp)
	}
}

// GetCollectionViewsV2Handler lists a collection's stored views
//
//	@Summary		Get collection views
//	@Description	Returns the collection's stored views as SPEC §6.2 view objects. Paginated (C10).
//	@Id				get_collection_views
//	@Tags			Lists
//	@Produce		json
//	@Param			space_id		path		string										true	"Space id"
//	@Param			collection_id	path		string										true	"Collection object id"
//	@Param			offset			query		int											false	"Items to skip"		default(0)
//	@Param			limit			query		int											false	"Items to return"	default(25)
//	@Success		200				{object}	v2model.ListResponse[v2model.ViewObject]	"§6.2 view objects"
//	@Failure		400				{object}	v2model.Error								"Wrong-layout target"
//	@Failure		404				{object}	v2model.Error								"Space or collection not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/collections/{collection_id}/views [get]
func GetCollectionViewsV2Handler(s *v2service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		views, total, hasMore, err := s.GetCollectionViews(c.Request.Context(),
			c.Param("space_id"), c.Param("collection_id"), offset, limit)
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, v2model.NewListResponse(views, total, offset, limit, hasMore,
			"request the next offset"))
	}
}
