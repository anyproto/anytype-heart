package list

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// GetListViewsHandler
//
//	@Summary		Get list views
//	@Description	Returns a paginated list of views defined for a specific list (query or collection) within a space. Each view includes configuration details such as layout, applied filters, and sorting options, enabling clients to render the list according to user preferences and context. This endpoint supports pagination parameters to control the number of views returned and the starting point of the result set.
//	@Tags			lists
//	@x-ai-omit		true
//	@Produce		json
//	@Param			Anytype-Version	header		string								false	"The version of the API to use"	default(2025-03-17)
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
func GetListViewsHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		listId := c.Param("list_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		views, total, hasMore, err := s.GetListViews(c, spaceId, listId, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrFailedGetList, http.StatusNotFound),
			util.ErrToCode(ErrFailedGetListDataview, http.StatusInternalServerError),
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
//	@Summary			Get objects in list
//	@Description		Returns a paginated list of objects associated with a specific list (query or collection) within a space. When a view ID is provided, the objects are filtered and sorted according to the view's configuration. If no view ID is specified, all list objects are returned without filtering and sorting. This endpoint helps clients to manage grouped objects (for example, tasks within a list) by returning information for each item of the list.
//	@x-ai-description	"Use this endpoint to retrieve all objects within a specific list (collection or query). You can specify a view ID to apply filters and sorting. The response includes detailed information about each object in the list. Use this when you need to display or manipulate the contents of a list, such as showing all items in a collection."
//	@Tags				lists
//	@Produce			json
//	@Param				Anytype-Version	header		string										false	"The version of the API to use"	default(2025-03-17)
//	@Param				space_id		path		string										true	"Space ID"
//	@Param				list_id			path		string										true	"List ID"
//	@Param				view_id			path		string										true	"View ID"
//	@Param				offset			query		int											false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param				limit			query		int											false	"The number of items to return"
//	@Success			200				{object}	pagination.PaginatedResponse[object.Object]	"List of objects"
//	@Failure			401				{object}	util.UnauthorizedError						"Unauthorized"
//	@Failure			404				{object}	util.NotFoundError							"Not found"
//	@Failure			500				{object}	util.ServerError							"Internal server error"
//	@Security			bearerauth
//	@Router				/spaces/{space_id}/lists/{list_id}/{view_id}/objects [get]
func GetObjectsInListHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		listId := c.Param("list_id")
		viewId := c.Param("view_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		objects, total, hasMore, err := s.GetObjectsInList(c, spaceId, listId, viewId, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrFailedGetList, http.StatusNotFound),
			util.ErrToCode(ErrFailedGetListDataview, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedGetListDataviewView, http.StatusInternalServerError),
			util.ErrToCode(ErrUnsupportedListType, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedGetObjectsInList, http.StatusInternalServerError),
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
//	@Summary			Add objects to list
//	@Description		Enables clients to add one or more objects to a specific list (collection only) by submitting a JSON array of object IDs. Upon success, the endpoint returns a confirmation message. This endpoint is vital for building user interfaces that allow drag‑and‑drop or multi‑select additions to collections.
//	@x-ai-description	"Use this endpoint to add multiple objects to a collection. You need to provide an array of object IDs in the request body. This is useful for organizing objects into collections, such as adding multiple tasks to a task list or multiple pages to a collection."
//	@Tags				lists
//	@Accept				json
//	@Produce			json
//	@Param				Anytype-Version	header		string					false	"The version of the API to use"	default(2025-03-17)
//	@Param				space_id		path		string					true	"Space ID"
//	@Param				list_id			path		string					true	"List ID"
//	@Param				objects			body		[]string				true	"List of object IDs"
//	@Success			200				{object}	string					"Objects added successfully"
//	@Failure			400				{object}	util.ValidationError	"Bad request"
//	@Failure			401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure			404				{object}	util.NotFoundError		"Not found"
//	@Failure			500				{object}	util.ServerError		"Internal server error"
//	@Security			bearerauth
//	@Router				/spaces/{space_id}/lists/{list_id}/objects [post]
func AddObjectsToListHandler(s Service) gin.HandlerFunc {
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
			util.ErrToCode(ErrFailedAddObjectsToList, http.StatusInternalServerError),
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
//	@Summary			Remove object from list
//	@Description		Removes a given object from the specified list (collection only) in a space. The endpoint takes the space, list, and object identifiers as path parameters. It's subject to rate limiting and returns a success message on completion. It is used for dynamically managing collections without affecting the underlying object data.
//	@x-ai-description	"Use this endpoint to remove an object from a collection. The object itself is not deleted, only its association with the collection is removed. This is useful when reorganizing collections or when an object should no longer be part of a specific collection."
//	@Tags				lists
//	@Produce			json
//	@Param				Anytype-Version	header		string					false	"The version of the API to use"	default(2025-03-17)
//	@Param				space_id		path		string					true	"Space ID"
//	@Param				list_id			path		string					true	"List ID"
//	@Param				object_id		path		string					true	"Object ID"
//	@Success			200				{object}	string					"Objects removed successfully"
//	@Failure			400				{object}	util.ValidationError	"Bad request"
//	@Failure			401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure			404				{object}	util.NotFoundError		"Not found"
//	@Failure			500				{object}	util.ServerError		"Internal server error"
//	@Security			bearerauth
//	@Router				/spaces/{space_id}/lists/{list_id}/objects/{object_id} [delete]
func RemoveObjectFromListHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		listId := c.Param("list_id")
		objectId := c.Param("object_id")

		err := s.RemoveObjectsFromList(c, spaceId, listId, []string{objectId})
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrFailedRemoveObjectsFromList, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, "Objects removed successfully")
	}
}

// CreateCollectionHandler
//
//	@Summary			Creates a collection with provided object IDs
//	@Description		Enables clients to create a new collection with a specified list of object IDs. The endpoint accepts a JSON array of object IDs in the request body. Upon successful creation, it returns a confirmation message. This is useful for creating collections of objects, such as tasks or pages, in a single operation.
//	@x-ai-description	"Use this endpoint to create a new collection and add multiple objects to it at once. You need to provide an array of object IDs in the request body. This is useful for creating collections of objects, such as tasks or pages, in a single operation."
//	@Tags				lists
//	@Accept				json
//	@Produce			json
//	@Param				Anytype-Version	header		string					false	"The version of the API to use"	default(2025-03-17)
//	@Param				space_id		path		string					true	"Space ID"
//	@Param				object			body		CreateCollectionRequest	true	"Collection to create"
//	@Success			200				{object}	ObjectResponse			"The created object"
//	@Failure			400				{object}	util.ValidationError	"Bad request"
//	@Failure			401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure			404				{object}	util.NotFoundError		"Not found"
//	@Failure			500				{object}	util.ServerError		"Internal server error"
//	@Security			bearerauth
//	@Router				/spaces/{space_id}/lists [post]
func CreateCollectionHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")

		request := CreateCollectionRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		object, err := s.CreateObjectsCollection(c, spaceId, request)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, ObjectResponse{
			Name: object.Name,
			Id:   object.Id,
		})
	}
}
