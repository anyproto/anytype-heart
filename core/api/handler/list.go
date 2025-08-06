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

// GetListViewsHandler
//
//	@Summary		Get list views
//	@Description	Returns a paginated list of views defined for a specific list (query or collection) within a space. Each view includes details such as layout, applied filters, and sorting options, enabling clients to render the list according to user preferences and context. This endpoint is essential for applications that need to display lists in various formats (e.g., grid, table) or with different sorting/filtering criteria.
//	@Id				get_list_views
//	@Tags			Lists
//	@Produce		json
//	@Param			Anytype-Version	header		string										true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string										true	"The ID of the space to which the list belongs; must be retrieved from ListSpaces endpoint"
//	@Param			list_id			path		string										true	"The ID of the list to retrieve views for; must be retrieved from SearchSpace endpoint with types: ['collection', 'set']"
//	@Param			offset			query		int											false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int											false	"The number of items to return"
//	@Success		200				{object}	pagination.PaginatedResponse[apimodel.View]	"The list of views associated with the specified list"
//	@Failure		401				{object}	util.UnauthorizedError						"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError							"Not found"
//	@Failure		500				{object}	util.ServerError							"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/lists/{list_id}/views [get]
func GetListViewsHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		listId := c.Param("list_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		views, total, hasMore, err := s.GetListViews(c, spaceId, listId, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedGetList, http.StatusNotFound),
			util.ErrToCode(service.ErrFailedGetListDataview, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		pagination.RespondWithPagination(c, http.StatusOK, views, total, offset, limit, hasMore)
	}
}

// GetObjectsInListHandler
//
//	@Summary		Get objects in list
//	@Description	Returns a paginated list of objects associated with a specific list (query or collection) within a space. When a view ID is provided, the objects are filtered and sorted according to the view's configuration. If no view ID is specified, all list objects are returned without filtering and sorting. This endpoint helps clients to manage grouped objects (for example, tasks within a list) by returning information for each item of the list.
//	@Description	Supports dynamic filtering via query parameters (e.g., ?type=page, ?done=false, ?created_date[gte]=2024-01-01, ?tags[in]=urgent,important). For select/tag properties use tag keys, for object properties use object IDs. See FilterCondition enum for available conditions.
//	@Id				get_list_objects
//	@Tags			Lists
//	@Produce		json
//	@Param			Anytype-Version	header		string											true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string											true	"The ID of the space to which the list belongs; must be retrieved from ListSpaces endpoint"
//	@Param			list_id			path		string											true	"The ID of the list to retrieve objects for; must be retrieved from SearchSpace endpoint with types: ['collection', 'set']"
//	@Param			view_id			path		string											true	"The ID of the view to retrieve objects for; must be retrieved from ListViews endpoint or omitted if you want to get all objects in the list"
//	@Param			offset			query		int												false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int												false	"The number of items to return"
//	@Success		200				{object}	pagination.PaginatedResponse[apimodel.Object]	"The list of objects associated with the specified list"
//	@Failure		401				{object}	util.UnauthorizedError							"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError								"Not found"
//	@Failure		500				{object}	util.ServerError								"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/lists/{list_id}/views/{view_id}/objects [get]
func GetObjectsInListHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		listId := c.Param("list_id")
		viewId := c.Param("view_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		filtersAny, _ := c.Get("filters")
		filters := filtersAny.([]*model.BlockContentDataviewFilter)

		objects, total, hasMore, err := s.GetObjectsInList(c, spaceId, listId, viewId, filters, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedGetList, http.StatusNotFound),
			util.ErrToCode(service.ErrFailedGetListDataview, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedGetListDataviewView, http.StatusInternalServerError),
			util.ErrToCode(service.ErrUnsupportedListType, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedGetObjectsInList, http.StatusInternalServerError),
			util.ErrToCode(util.ErrFailedResolveToUniqueKey, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		pagination.RespondWithPagination(c, http.StatusOK, objects, total, offset, limit, hasMore)
	}
}

// AddObjectsToListHandler
//
//	@Summary		Add objects to list
//	@Description	Adds one or more objects to a specific list (collection only) by submitting a JSON array of object IDs. Upon success, the endpoint returns a confirmation message. This endpoint is vital for building user interfaces that allow drag‑and‑drop or multi‑select additions to collections, enabling users to dynamically manage their collections without needing to modify the underlying object data.
//	@Id				add_list_objects
//	@Tags			Lists
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string								true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string								true	"The ID of the space to which the list belongs; must be retrieved from ListSpaces endpoint"
//	@Param			list_id			path		string								true	"The ID of the list to which objects will be added; must be retrieved from SearchSpace endpoint with types: ['collection', 'set']"
//	@Param			objects			body		apimodel.AddObjectsToListRequest	true	"The list of object IDs to add to the list; must be retrieved from SearchSpace or GlobalSearch endpoints or obtained from response context"
//	@Success		200				{object}	string								"Objects added successfully"
//	@Failure		400				{object}	util.ValidationError				"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError				"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError					"Not found"
//	@Failure		429				{object}	util.RateLimitError					"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError					"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/lists/{list_id}/objects [post]
func AddObjectsToListHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		listId := c.Param("list_id")

		var req apimodel.AddObjectsToListRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		err := s.AddObjectsToList(c, spaceId, listId, req)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedAddObjectsToList, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, "Objects added successfully")
	}
}

// RemoveObjectFromListHandler
//
//	@Summary		Remove object from list
//	@Description	Removes a given object from the specified list (collection only) in a space. The endpoint takes the space, list, and object identifiers as path parameters and is subject to rate limiting. It is used for dynamically managing collections without affecting the underlying object data.
//	@Id				remove_list_object
//	@Tags			Lists
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string					true	"The ID of the space to which the list belongs; must be retrieved from ListSpaces endpoint"
//	@Param			list_id			path		string					true	"The ID of the list from which the object will be removed; must be retrieved from SearchSpace endpoint with types: ['collection', 'set']"
//	@Param			object_id		path		string					true	"The ID of the object to remove from the list; must be retrieved from SearchSpace or GlobalSearch endpoints or obtained from response context"
//	@Success		200				{object}	string					"Objects removed successfully"
//	@Failure		400				{object}	util.ValidationError	"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Not found"
//	@Failure		429				{object}	util.RateLimitError		"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/lists/{list_id}/objects/{object_id} [delete]
func RemoveObjectFromListHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		listId := c.Param("list_id")
		objectId := c.Param("object_id")

		err := s.RemoveObjectsFromList(c, spaceId, listId, []string{objectId})
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedRemoveObjectsFromList, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, "Objects removed successfully")
	}
}
