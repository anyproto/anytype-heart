package list

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// GetObjectsInListHandler
//
//	@Summary		Get objects in list
//	@Description	Returns a paginated list of objects that are associated with a specific list (or collection) within a space. This endpoint helps clients to manage grouped objects (for example, tasks within a list) by returning detailed object information for each item of the list.
//	@Tags			lists
//	@Produce		json
//	@Param			space_id	path		string										true	"Space ID"
//	@Param			list_id		path		string										true	"List ID"
//	@Param			offset		query		int											false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit		query		int											false	"The number of items to return"
//	@Success		200			{object}	pagination.PaginatedResponse[object.Object]	"List of objects"
//	@Failure		401			{object}	util.UnauthorizedError						"Unauthorized"
//	@Failure		404			{object}	util.NotFoundError							"Not found"
//	@Failure		500			{object}	util.ServerError							"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/lists/{list_id}/objects [get]
func GetObjectsInListHandler(s *ListService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		listId := c.Param("list_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		objects, total, hasMore, err := s.GetObjectsInList(c, spaceId, listId, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrFailedGetObjectsInList, http.StatusInternalServerError),
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
//	@Description	Enables clients to add one or more objects to a specific list by submitting a JSON array of object IDs. Upon success, the endpoint returns a confirmation message. This endpoint is vital for building user interfaces that allow drag‑and‑drop or multi‑select additions to collections.
//	@Tags			lists
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space ID"
//	@Param			list_id		path		string					true	"List ID"
//	@Param			objects		body		[]string				true	"List of object IDs"
//	@Success		200			{object}	string					"Objects added successfully"
//	@Failure		400			{object}	util.ValidationError	"Bad request"
//	@Failure		401			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404			{object}	util.NotFoundError		"Not found"
//	@Failure		500			{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/lists/{list_id}/objects [post]
func AddObjectsToListHandler(s *ListService) gin.HandlerFunc {
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
//	@Summary		Remove object from list
//	@Description	Removes a given object from the specified list in a space. The endpoint takes the space, list, and object identifiers as path parameters. It's subject to rate limiting and returns a success message on completion. It is used for dynamically managing collections without affecting the underlying object data.
//	@Tags			lists
//	@Produce		json
//	@Param			space_id	path		string					true	"Space ID"
//	@Param			list_id		path		string					true	"List ID"
//	@Param			object_id	path		string					true	"Object ID"
//	@Success		200			{object}	string					"Objects removed successfully"
//	@Failure		400			{object}	util.ValidationError	"Bad request"
//	@Failure		401			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404			{object}	util.NotFoundError		"Not found"
//	@Failure		500			{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/lists/{list_id}/objects/{object_id} [delete]
func RemoveObjectFromListHandler(s *ListService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		listId := c.Param("list_id")
		objectId := c.Param("object_id")

		err := s.RemoveObjectFromList(c, spaceId, listId, objectId)
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
