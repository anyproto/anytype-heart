package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/internal/list"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// GetListViewsHandler
//
//	@Summary		Get list views
//	@Description	Returns a paginated list of views defined for a specific list (query or collection) within a space. Each view includes configuration details such as layout, applied filters, and sorting options, enabling clients to render the list according to user preferences and context. This endpoint supports pagination parameters to control the number of views returned and the starting point of the result set.
//	@Id				getListViews
//	@Tags			Lists
//	@Produce		json
//	@Param			Anytype-Version	header		string								true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string								true	"Space ID"
//	@Param			list_id			path		string								true	"List ID"
//	@Param			offset			query		int									false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int									false	"The number of items to return"
//	@Success		200				{object}	pagination.PaginatedResponse[View]	"List of views"
//	@Failure		401				{object}	util.UnauthorizedError				"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError					"Not found"
//	@Failure		500				{object}	util.ServerError					"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/lists/{list_id}/views [get]
func GetListViewsHandler(s list.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		listId := c.Param("list_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		views, total, hasMore, err := s.GetListViews(c, spaceId, listId, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(list.ErrFailedGetList, http.StatusNotFound),
			util.ErrToCode(list.ErrFailedGetListDataview, http.StatusInternalServerError),
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
//	@Id				getListObjects
//	@Tags			Lists
//	@Produce		json
//	@Param			Anytype-Version	header		string										true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string										true	"Space ID"
//	@Param			list_id			path		string										true	"List ID"
//	@Param			view_id			path		string										true	"View ID"
//	@Param			offset			query		int											false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int											false	"The number of items to return"
//	@Success		200				{object}	pagination.PaginatedResponse[object.Object]	"List of objects"
//	@Failure		401				{object}	util.UnauthorizedError						"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError							"Not found"
//	@Failure		500				{object}	util.ServerError							"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/lists/{list_id}/{view_id}/objects [get]
func GetObjectsInListHandler(s list.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		listId := c.Param("list_id")
		viewId := c.Param("view_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		objects, total, hasMore, err := s.GetObjectsInList(c, spaceId, listId, viewId, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(list.ErrFailedGetList, http.StatusNotFound),
			util.ErrToCode(list.ErrFailedGetListDataview, http.StatusInternalServerError),
			util.ErrToCode(list.ErrFailedGetListDataviewView, http.StatusInternalServerError),
			util.ErrToCode(list.ErrUnsupportedListType, http.StatusInternalServerError),
			util.ErrToCode(list.ErrFailedGetObjectsInList, http.StatusInternalServerError),
			util.ErrToCode(util.ErrorResolveToUniqueKey, http.StatusInternalServerError),
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
//	@Description	Enables clients to add one or more objects to a specific list (collection only) by submitting a JSON array of object IDs. Upon success, the endpoint returns a confirmation message. This endpoint is vital for building user interfaces that allow drag‑and‑drop or multi‑select additions to collections.
//	@Id				addListObjects
//	@Tags			Lists
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			list_id			path		string					true	"List ID"
//	@Param			objects			body		[]string				true	"List of object IDs"
//	@Success		200				{object}	string					"Objects added successfully"
//	@Failure		400				{object}	util.ValidationError	"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Not found"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/lists/{list_id}/objects [post]
func AddObjectsToListHandler(s list.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		listId := c.Param("list_id")

		objects := []string{}
		if err := c.ShouldBindJSON(&objects); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		err := s.AddObjectsToList(c, spaceId, listId, objects)
		code := util.MapErrorCode(err,
			util.ErrToCode(list.ErrFailedAddObjectsToList, http.StatusInternalServerError),
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
//	@Description	Removes a given object from the specified list (collection only) in a space. The endpoint takes the space, list, and object identifiers as path parameters. It's subject to rate limiting and returns a success message on completion. It is used for dynamically managing collections without affecting the underlying object data.
//	@Id				removeListObject
//	@Tags			Lists
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			list_id			path		string					true	"List ID"
//	@Param			object_id		path		string					true	"Object ID"
//	@Success		200				{object}	string					"Objects removed successfully"
//	@Failure		400				{object}	util.ValidationError	"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Not found"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/lists/{list_id}/objects/{object_id} [delete]
func RemoveObjectFromListHandler(s list.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		listId := c.Param("list_id")
		objectId := c.Param("object_id")

		err := s.RemoveObjectsFromList(c, spaceId, listId, []string{objectId})
		code := util.MapErrorCode(err,
			util.ErrToCode(list.ErrFailedRemoveObjectsFromList, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, "Objects removed successfully")
	}
}
