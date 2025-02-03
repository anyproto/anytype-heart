package object

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// GetObjectsHandler retrieves a list of objects in a space
//
//	@Summary	List objects
//	@Tags		objects
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string									true	"Space ID"
//	@Param		offset		query		int										false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param		limit		query		int										false	"The number of items to return"											default(100)	maximum(1000)
//	@Success	200			{object}	pagination.PaginatedResponse[Object]	"List of objects"
//	@Failure	401			{object}	util.UnauthorizedError					"Unauthorized"
//	@Failure	500			{object}	util.ServerError						"Internal server error"
//	@Router		/spaces/{space_id}/objects [get]
func GetObjectsHandler(s *ObjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		objects, total, hasMore, err := s.ListObjects(c.Request.Context(), spaceId, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrFailedRetrieveObjects, http.StatusInternalServerError),
			util.ErrToCode(ErrObjectNotFound, http.StatusInternalServerError),
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

// GetObjectHandler retrieves an object in a space
//
//	@Summary	Get object
//	@Tags		objects
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string					true	"Space ID"
//	@Param		object_id	path		string					true	"Object ID"
//	@Success	200			{object}	ObjectResponse			"The requested object"
//	@Failure	401			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure	404			{object}	util.NotFoundError		"Resource not found"
//	@Failure	500			{object}	util.ServerError		"Internal server error"
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

// DeleteObjectHandler deletes an object in a space
//
//	@Summary	Delete object
//	@Tags		objects
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string					true	"Space ID"
//	@Param		object_id	path		string					true	"Object ID"
//	@Success	200			{object}	ObjectResponse			"The deleted object"
//	@Failure	401			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure	403			{object}	util.ForbiddenError		"Forbidden"
//	@Failure	404			{object}	util.NotFoundError		"Resource not found"
//	@Failure	423			{object}	util.RateLimitError		"Rate limit exceeded"
//	@Failure	500			{object}	util.ServerError		"Internal server error"
//	@Router		/spaces/{space_id}/objects/{object_id} [delete]
func DeleteObjectHandler(s *ObjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		objectId := c.Param("object_id")

		object, err := s.DeleteObject(c.Request.Context(), spaceId, objectId)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrObjectNotFound, http.StatusNotFound),
			util.ErrToCode(ErrFailedDeleteObject, http.StatusForbidden),
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

// CreateObjectHandler creates a new object in a space
//
//	@Summary	Create object
//	@Tags		objects
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string					true	"Space ID"
//	@Param		object		body		CreateObjectRequest		true	"Object to create"
//	@Success	200			{object}	ObjectResponse			"The created object"
//	@Failure	400			{object}	util.ValidationError	"Bad request"
//	@Failure	401			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure	423			{object}	util.RateLimitError		"Rate limit exceeded"
//	@Failure	500			{object}	util.ServerError		"Internal server error"
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
			util.ErrToCode(ErrObjectNotFound, http.StatusInternalServerError),
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

// GetTypesHandler retrieves a list of types in a space
//
//	@Summary	List types
//	@Tags		types
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string								true	"Space ID"
//	@Param		offset		query		int									false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param		limit		query		int									false	"The number of items to return"											default(100)	maximum(1000)
//	@Success	200			{object}	pagination.PaginatedResponse[Type]	"List of types"
//	@Failure	401			{object}	util.UnauthorizedError				"Unauthorized"
//	@Failure	500			{object}	util.ServerError					"Internal server error"
//	@Router		/spaces/{space_id}/types [get]
func GetTypesHandler(s *ObjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		types, total, hasMore, err := s.ListTypes(c.Request.Context(), spaceId, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrFailedRetrieveTypes, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		pagination.RespondWithPagination(c, http.StatusOK, types, total, offset, limit, hasMore)
	}
}

// GetTypeHandler retrieves a type in a space
//
//	@Summary	Get type
//	@Tags		types
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string					true	"Space ID"
//	@Param		type_id		path		string					true	"Type ID"
//	@Success	200			{object}	TypeResponse			"The requested type"
//	@Failure	401			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure	404			{object}	util.NotFoundError		"Resource not found"
//	@Failure	500			{object}	util.ServerError		"Internal server error"
//	@Router		/spaces/{space_id}/types/{type_id} [get]
func GetTypeHandler(s *ObjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		typeId := c.Param("type_id")

		object, err := s.GetType(c.Request.Context(), spaceId, typeId)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrTypeNotFound, http.StatusNotFound),
			util.ErrToCode(ErrFailedRetrieveType, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, TypeResponse{Type: object})
	}
}

// GetTemplatesHandler retrieves a list of templates for a type in a space
//
//	@Summary	List templates
//	@Tags		types
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string									true	"Space ID"
//	@Param		type_id		path		string									true	"Type ID"
//	@Param		offset		query		int										false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param		limit		query		int										false	"The number of items to return"											default(100)	maximum(1000)
//	@Success	200			{object}	pagination.PaginatedResponse[Template]	"List of templates"
//	@Failure	401			{object}	util.UnauthorizedError					"Unauthorized"
//	@Failure	500			{object}	util.ServerError						"Internal server error"
//	@Router		/spaces/{space_id}/types/{type_id}/templates [get]
func GetTemplatesHandler(s *ObjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		typeId := c.Param("type_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		templates, total, hasMore, err := s.ListTemplates(c.Request.Context(), spaceId, typeId, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrFailedRetrieveTemplateType, http.StatusInternalServerError),
			util.ErrToCode(ErrTemplateTypeNotFound, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedRetrieveTemplates, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedRetrieveTemplate, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		pagination.RespondWithPagination(c, http.StatusOK, templates, total, offset, limit, hasMore)
	}
}

// GetTemplateHandler retrieves a template for a type in a space
//
//	@Summary	Get template
//	@Tags		types
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string					true	"Space ID"
//	@Param		type_id		path		string					true	"Type ID"
//	@Param		template_id	path		string					true	"Template ID"
//	@Success	200			{object}	TemplateResponse		"The requested template"
//	@Failure	401			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure	404			{object}	util.NotFoundError		"Resource not found"
//	@Failure	500			{object}	util.ServerError		"Internal server error"
//	@Router		/spaces/{space_id}/types/{type_id}/templates/{template_id} [get]
func GetTemplateHandler(s *ObjectService) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		typeId := c.Param("type_id")
		templateId := c.Param("template_id")

		object, err := s.GetTemplate(c.Request.Context(), spaceId, typeId, templateId)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrTemplateNotFound, http.StatusNotFound),
			util.ErrToCode(ErrFailedRetrieveTemplate, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, TemplateResponse{Template: object})
	}
}
