package object

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/cmd/api/pagination"
	"github.com/anyproto/anytype-heart/cmd/api/util"
)

// GetObjectsHandler retrieves objects in a specific space
//
//	@Summary	Retrieve objects in a specific space
//	@Tags		objects
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string					true	"The ID of the space"
//	@Param		offset		query		int						false	"The number of items to skip before starting to collect the result set"
//	@Param		limit		query		int						false	"The number of items to return"	default(100)
//	@Success	200			{object}	map[string][]Object		"List of objects"
//	@Failure	403			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure	404			{object}	util.NotFoundError		"Resource not found"
//	@Failure	502			{object}	util.ServerError		"Internal server error"
//	@Router		/spaces/{space_id}/objects [get]
func GetObjectsHandler(s *ObjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		objects, total, hasMore, err := s.ListObjects(c.Request.Context(), spaceId, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrorFailedRetrieveObjects, http.StatusInternalServerError),
			util.ErrToCode(ErrNoObjectsFound, http.StatusNotFound),
			util.ErrToCode(ErrObjectNotFound, http.StatusNotFound),
			util.ErrToCode(ErrFailedRetrieveObject, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		pagination.RespondWithPagination(c, http.StatusOK, objects, total, offset, limit, hasMore)
	}
}

// GetObjectHandler retrieves a specific object in a space
//
//	@Summary	Retrieve a specific object in a space
//	@Tags		objects
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string					true	"The ID of the space"
//	@Param		object_id	path		string					true	"The ID of the object"
//	@Success	200			{object}	ObjectResponse			"The requested object"
//	@Failure	403			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure	404			{object}	util.NotFoundError		"Resource not found"
//	@Failure	502			{object}	util.ServerError		"Internal server error"
//	@Router		/spaces/{space_id}/objects/{object_id} [get]
func GetObjectHandler(s *ObjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		objectId := c.Param("object_id")

		object, err := s.GetObject(c.Request.Context(), spaceId, objectId)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrObjectNotFound, http.StatusNotFound),
			util.ErrToCode(ErrFailedRetrieveObject, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, ObjectResponse{Object: object})
	}
}

// DeleteObjectHandler deletes a specific object in a space
//
//	@Summary	Delete a specific object in a space
//	@Tags		objects
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string					true	"The ID of the space"
//	@Param		object_id	path		string					true	"The ID of the object"
//	@Success	200			{object}	ObjectResponse			"The deleted object"
//	@Failure	403			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure	404			{object}	util.NotFoundError		"Resource not found"
//	@Failure	502			{object}	util.ServerError		"Internal server error"
//	@Router		/spaces/{space_id}/objects/{object_id} [delete]
func DeleteObjectHandler(s *ObjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		objectId := c.Param("object_id")

		object, err := s.DeleteObject(c.Request.Context(), spaceId, objectId)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrObjectNotFound, http.StatusNotFound),
			util.ErrToCode(ErrFailedRetrieveObject, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedDeleteObject, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, ObjectResponse{Object: object})
	}
}

// CreateObjectHandler creates a new object in a specific space
//
//	@Summary	Create a new object in a specific space
//	@Tags		objects
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string					true	"The ID of the space"
//	@Param		object		body		map[string]string		true	"Object details (e.g., name)"
//	@Success	200			{object}	ObjectResponse			"The created object"
//	@Failure	400			{object}	util.ValidationError	"Bad request"
//	@Failure	403			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure	502			{object}	util.ServerError		"Internal server error"
//	@Router		/spaces/{space_id}/objects [post]
func CreateObjectHandler(s *ObjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")

		request := CreateObjectRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		object, err := s.CreateObject(c.Request.Context(), spaceId, request)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrInputMissingSource, http.StatusBadRequest),
			util.ErrToCode(ErrFailedCreateObject, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedSetRelationFeatured, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedFetchBookmark, http.StatusInternalServerError),
			util.ErrToCode(ErrObjectNotFound, http.StatusNotFound),
			util.ErrToCode(ErrFailedRetrieveObject, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, ObjectResponse{Object: object})
	}
}

// UpdateObjectHandler updates an existing object in a specific space
//
//	@Summary	Update an existing object in a specific space
//	@Tags		objects
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string					true	"The ID of the space"
//	@Param		object_id	path		string					true	"The ID of the object"
//	@Param		object		body		Object					true	"The updated object details"
//	@Success	200			{object}	ObjectResponse			"The updated object"
//	@Failure	400			{object}	util.ValidationError	"Bad request"
//	@Failure	403			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure	404			{object}	util.NotFoundError		"Resource not found"
//	@Failure	502			{object}	util.ServerError		"Internal server error"
//	@Router		/spaces/{space_id}/objects/{object_id} [put]
func UpdateObjectHandler(s *ObjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		objectId := c.Param("object_id")

		request := UpdateObjectRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		object, err := s.UpdateObject(c.Request.Context(), spaceId, objectId, request)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrNotImplemented, http.StatusNotImplemented),
			util.ErrToCode(ErrFailedUpdateObject, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedRetrieveObject, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusNotImplemented, ObjectResponse{Object: object})
	}
}

// GetTypesHandler retrieves object types in a specific space
//
//	@Summary	Retrieve object types in a specific space
//	@Tags		objects
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string					true	"The ID of the space"
//	@Param		offset		query		int						false	"The number of items to skip before starting to collect the result set"
//	@Param		limit		query		int						false	"The number of items to return"	default(100)
//	@Success	200			{object}	map[string]ObjectType	"List of object types"
//	@Failure	403			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure	404			{object}	util.NotFoundError		"Resource not found"
//	@Failure	502			{object}	util.ServerError		"Internal server error"
//	@Router		/spaces/{space_id}/object_types [get]
func GetTypesHandler(s *ObjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		types, total, hasMore, err := s.ListTypes(c.Request.Context(), spaceId, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrFailedRetrieveTypes, http.StatusInternalServerError),
			util.ErrToCode(ErrNoTypesFound, http.StatusNotFound),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		pagination.RespondWithPagination(c, http.StatusOK, types, total, offset, limit, hasMore)
	}
}

// GetTemplatesHandler retrieves a list of templates for a specific object type in a space
//
//	@Summary	Retrieve a list of templates for a specific object type in a space
//	@Tags		objects
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string						true	"The ID of the space"
//	@Param		type_id		path		string						true	"The ID of the object type"
//	@Param		offset		query		int							false	"The number of items to skip before starting to collect the result set"
//	@Param		limit		query		int							false	"The number of items to return"	default(100)
//	@Success	200			{object}	map[string][]ObjectTemplate	"List of templates"
//	@Failure	403			{object}	util.UnauthorizedError		"Unauthorized"
//	@Failure	404			{object}	util.NotFoundError			"Resource not found"
//	@Failure	502			{object}	util.ServerError			"Internal server error"
//	@Router		/spaces/{space_id}/object_types/{type_id}/templates [get]
func GetTemplatesHandler(s *ObjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		typeId := c.Param("type_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		templates, total, hasMore, err := s.ListTemplates(c.Request.Context(), spaceId, typeId, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrFailedRetrieveTemplateType, http.StatusInternalServerError),
			util.ErrToCode(ErrTemplateTypeNotFound, http.StatusNotFound),
			util.ErrToCode(ErrFailedRetrieveTemplates, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedRetrieveTemplate, http.StatusInternalServerError),
			util.ErrToCode(ErrNoTemplatesFound, http.StatusNotFound),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		pagination.RespondWithPagination(c, http.StatusOK, templates, total, offset, limit, hasMore)
	}
}
