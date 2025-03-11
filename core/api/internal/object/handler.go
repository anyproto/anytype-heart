package object

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// GetObjectsHandler retrieves a list of objects in a space
//
//	@Summary		List objects
//	@Description	Retrieves a paginated list of objects in the given space. The endpoint takes query parameters for pagination (offset and limit) and returns detailed data about each object including its ID, name, icon, type information, a snippet of the content (if applicable), layout, space ID, blocks and details. It is intended for building views where users can see all objects in a space at a glance.
//	@Tags			objects
//	@Produce		json
//	@Param			space_id	path		string									true	"Space ID"
//	@Param			offset		query		int										false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit		query		int										false	"The number of items to return"											default(100)	maximum(1000)
//	@Success		200			{object}	pagination.PaginatedResponse[Object]	"List of objects"
//	@Failure		401			{object}	util.UnauthorizedError					"Unauthorized"
//	@Failure		500			{object}	util.ServerError						"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/objects [get]
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
//	@Summary		Get object
//	@Description	Fetches the full details of a single object identified by the object ID within the specified space. The response includes not only basic metadata (ID, name, icon, type) but also the complete set of blocks (which may include text, files, and relations) and extra details (such as timestamps and linked member information). This endpoint is essential when a client needs to render or edit the full object view.
//	@Tags			objects
//	@Produce		json
//	@Param			space_id	path		string					true	"Space ID"
//	@Param			object_id	path		string					true	"Object ID"
//	@Success		200			{object}	ObjectResponse			"The requested object"
//	@Failure		401			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404			{object}	util.NotFoundError		"Resource not found"
//	@Failure		500			{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/objects/{object_id} [get]
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
//	@Summary		Delete object
//	@Description	This endpoint “deletes” an object by marking it as archived. The deletion process is performed safely and is subject to rate limiting. It returns the object’s details after it has been archived. Proper error handling is in place for situations such as when the object isn’t found or the deletion cannot be performed because of permission issues.
//	@Tags			objects
//	@Produce		json
//	@Param			space_id	path		string					true	"Space ID"
//	@Param			object_id	path		string					true	"Object ID"
//	@Success		200			{object}	ObjectResponse			"The deleted object"
//	@Failure		401			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		403			{object}	util.ForbiddenError		"Forbidden"
//	@Failure		404			{object}	util.NotFoundError		"Resource not found"
//	@Failure		423			{object}	util.RateLimitError		"Rate limit exceeded"
//	@Failure		500			{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/objects/{object_id} [delete]
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
//	@Summary		Create object
//	@Description	Creates a new object in the specified space using a JSON payload. The creation process is subject to rate limiting. The payload must include key details such as the object name, icon, description, body content (which may support Markdown), source URL (required for bookmark objects), template identifier, and the unique key for the object type. Post-creation, additional operations (like setting featured relations or fetching bookmark metadata) may occur. The endpoint then returns the full object data, ready for further interactions.
//	@Tags			objects
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space ID"
//	@Param			object		body		CreateObjectRequest		true	"Object to create"
//	@Success		200			{object}	ObjectResponse			"The created object"
//	@Failure		400			{object}	util.ValidationError	"Bad request"
//	@Failure		401			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		423			{object}	util.RateLimitError		"Rate limit exceeded"
//	@Failure		500			{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/objects [post]
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
//	@Summary		List types
//	@Description	This endpoint retrieves a paginated list of object types (e.g. 'Page', 'Note', 'Task') available within the specified space. Each type’s record includes its unique identifier, unique key, display name, icon, and a recommended layout. Clients use this information when offering choices for object creation or for filtering objects by type.
//	@Tags			types
//	@Produce		json
//	@Param			space_id	path		string								true	"Space ID"
//	@Param			offset		query		int									false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit		query		int									false	"The number of items to return"											default(100)	maximum(1000)
//	@Success		200			{object}	pagination.PaginatedResponse[Type]	"List of types"
//	@Failure		401			{object}	util.UnauthorizedError				"Unauthorized"
//	@Failure		500			{object}	util.ServerError					"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/types [get]
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
//	@Summary		Get type
//	@Description	Fetches detailed information about one specific object type by its ID. This includes the type’s unique key, name, icon, and recommended layout. This detailed view assists clients in understanding the expected structure and style for objects of that type and in guiding the user interface (such as displaying appropriate icons or layout hints).
//	@Tags			types
//	@Produce		json
//	@Param			space_id	path		string					true	"Space ID"
//	@Param			type_id		path		string					true	"Type ID"
//	@Success		200			{object}	TypeResponse			"The requested type"
//	@Failure		401			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404			{object}	util.NotFoundError		"Resource not found"
//	@Failure		500			{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/types/{type_id} [get]
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
//	@Summary		List templates
//	@Description	This endpoint returns a paginated list of templates that are associated with a specific object type within a space. Templates provide pre‑configured structures for creating new objects. Each template record contains its identifier, name, and icon, so that clients can offer users a selection of templates when creating objects.
//	@Tags			templates
//	@Produce		json
//	@Param			space_id	path		string									true	"Space ID"
//	@Param			type_id		path		string									true	"Type ID"
//	@Param			offset		query		int										false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit		query		int										false	"The number of items to return"											default(100)	maximum(1000)
//	@Success		200			{object}	pagination.PaginatedResponse[Template]	"List of templates"
//	@Failure		401			{object}	util.UnauthorizedError					"Unauthorized"
//	@Failure		500			{object}	util.ServerError						"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/types/{type_id}/templates [get]
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
//	@Summary		Get template
//	@Description	Fetches full details for one template associated with a particular object type in a space. The response provides the template’s identifier, name, icon, and any other relevant metadata. This endpoint is useful when a client needs to preview or apply a template to prefill object creation fields.
//	@Tags			templates
//	@Produce		json
//	@Param			space_id	path		string					true	"Space ID"
//	@Param			type_id		path		string					true	"Type ID"
//	@Param			template_id	path		string					true	"Template ID"
//	@Success		200			{object}	TemplateResponse		"The requested template"
//	@Failure		401			{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404			{object}	util.NotFoundError		"Resource not found"
//	@Failure		500			{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/types/{type_id}/templates/{template_id} [get]
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
