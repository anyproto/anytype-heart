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
//	@Tags			Objects
//	@Produce		json
//	@Param			Anytype-Version	header		string									true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string									true	"Space ID"
//	@Param			offset			query		int										false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int										false	"The number of items to return"											default(100)	maximum(1000)
//	@Success		200				{object}	pagination.PaginatedResponse[Object]	"List of objects"
//	@Failure		401				{object}	util.UnauthorizedError					"Unauthorized"
//	@Failure		500				{object}	util.ServerError						"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/objects [get]
func GetObjectsHandler(s Service) gin.HandlerFunc {
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
//	@Description	Fetches the full details of a single object identified by the object ID within the specified space. The response includes not only basic metadata (ID, name, icon, type) but also the complete set of blocks (which may include text, files, properties and dataviews) and extra details (such as timestamps and linked member information). This endpoint is essential when a client needs to render or edit the full object view.
//	@Tags			Objects
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			object_id		path		string					true	"Object ID"
//	@Success		200				{object}	ObjectResponse			"The requested object"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Resource not found"
//	@Failure		410				{object}	util.GoneError			"Resource deleted"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/objects/{object_id} [get]
func GetObjectHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		objectId := c.Param("object_id")

		object, err := s.GetObject(c.Request.Context(), spaceId, objectId)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrObjectNotFound, http.StatusNotFound),
			util.ErrToCode(ErrObjectDeleted, http.StatusGone),
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
//	@Tags			Objects
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			object_id		path		string					true	"Object ID"
//	@Success		200				{object}	ObjectResponse			"The deleted object"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		403				{object}	util.ForbiddenError		"Forbidden"
//	@Failure		404				{object}	util.NotFoundError		"Resource not found"
//	@Failure		410				{object}	util.GoneError			"Resource deleted"
//	@Failure		423				{object}	util.RateLimitError		"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/objects/{object_id} [delete]
func DeleteObjectHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		objectId := c.Param("object_id")

		object, err := s.DeleteObject(c.Request.Context(), spaceId, objectId)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrObjectNotFound, http.StatusNotFound),
			util.ErrToCode(ErrObjectDeleted, http.StatusGone),
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
//	@Description	Creates a new object in the specified space using a JSON payload. The creation process is subject to rate limiting. The payload must include key details such as the object name, icon, description, body content (which may support Markdown), source URL (required for bookmark objects), template identifier, and the type_key (which is the non-unique identifier of the type of object to create). Post-creation, additional operations (like setting featured properties or fetching bookmark metadata) may occur. The endpoint then returns the full object data, ready for further interactions.
//	@Tags			Objects
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			object			body		CreateObjectRequest		true	"Object to create"
//	@Success		200				{object}	ObjectResponse			"The created object"
//	@Failure		400				{object}	util.ValidationError	"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		423				{object}	util.RateLimitError		"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/objects [post]
func CreateObjectHandler(s Service) gin.HandlerFunc {
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
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(ErrFailedCreateBookmark, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedCreateObject, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedSetPropertyFeatured, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedCreateBlock, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedPasteBody, http.StatusInternalServerError),
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

// GetObjectExportHandler exports an object in specified format
//
//	@Summary		Export object
//	@Description	This endpoint exports a single object from the specified space into a desired format. The export format is provided as a path parameter (currently supporting “markdown” only). The endpoint calls the export service which converts the object’s content into the requested format. It is useful for sharing, or displaying the markdown representation of the objecte externally.
//	@Tags			Objects
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			object_id		path		string					true	"Object ID"
//	@Param			format			path		string					true	"Export format"	Enums(markdown)
//	@Success		200				{object}	ObjectExportResponse	"Object exported successfully"
//	@Failure		400				{object}	util.ValidationError	"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/objects/{object_id}/{format} [get]
func GetObjectExportHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		objectId := c.Param("object_id")
		format := c.Param("format")

		markdown, err := s.GetObjectExport(c.Request.Context(), spaceId, objectId, format)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrInvalidExportFormat, http.StatusInternalServerError))

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, ObjectExportResponse{Markdown: markdown})
	}
}

// GetPropertiesHandler retrieves a list of properties in a space
//
//	@Summary		List properties
//	@Description	This endpoint retrieves a paginated list of properties available within a specific space. Each property record includes its unique identifier, name and format. This information is essential for clients to understand the available properties for filtering or creating objects.
//	@Tags			Properties
//	@Produce		json
//	@Param			Anytype-Version	header		string									true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string									true	"Space ID"
//	@Param			offset			query		int										false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int										false	"The number of items to return"											default(100)	maximum(1000)
//	@Success		200				{object}	pagination.PaginatedResponse[Property]	"List of properties"
//	@Failure		401				{object}	util.UnauthorizedError					"Unauthorized"
//	@Failure		500				{object}	util.ServerError						"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/properties [get]
func GetPropertiesHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		properties, total, hasMore, err := s.ListProperties(c.Request.Context(), spaceId, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrFailedRetrieveProperties, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		pagination.RespondWithPagination(c, http.StatusOK, properties, total, offset, limit, hasMore)
	}
}

// GetPropertyHandler retrieves a property in a space
//
//	@Summary		Get property
//	@Description	Fetches detailed information about one specific property by its ID. This includes the property’s unique identifier, name and format. This detailed view assists clients in showing property options to users and in guiding the user interface (such as displaying appropriate input fields or selection options).
//	@Tags			Properties
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			property_id		path		string					true	"Property ID"
//	@Success		200				{object}	PropertyResponse		"The requested property"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Resource not found"
//	@Failure		410				{object}	util.GoneError			"Resource deleted"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/properties/{property_id} [get]
func GetPropertyHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")

		property, err := s.GetProperty(c.Request.Context(), spaceId, propertyId)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrPropertyNotFound, http.StatusNotFound),
			util.ErrToCode(ErrPropertyDeleted, http.StatusGone),
			util.ErrToCode(ErrFailedRetrieveProperty, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, PropertyResponse{Property: property})
	}
}

// CreatePropertyHandler creates a new property in a space
//
//	@Summary		Create property
//	@Description	Creates a new property in the specified space using a JSON payload. The creation process is subject to rate limiting. The payload must include property details such as the name and format. The endpoint then returns the full property data, ready for further interactions.
//	@Tags			Properties
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			property		body		CreatePropertyRequest	true	"Property to create"
//	@Success		200				{object}	PropertyResponse		"The created property"
//	@Failure		400				{object}	util.ValidationError	"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/properties [post]
func CreatePropertyHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")

		request := CreatePropertyRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		property, err := s.CreateProperty(c.Request.Context(), spaceId, request)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(ErrFailedCreateProperty, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedRetrieveProperty, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, PropertyResponse{Property: property})
	}
}

// UpdatePropertyHandler updates a property in a space
//
//	@Summary		Update property
//	@Description	This endpoint updates an existing property in the specified space using a JSON payload. The update process is subject to rate limiting. The payload must include the property ID and the name to be updated. The endpoint then returns the full property data, ready for further interactions.
//	@Tags			Properties
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			property_id		path		string					true	"Property ID"
//	@Param			property		body		UpdatePropertyRequest	true	"Property to update"
//	@Success		200				{object}	PropertyResponse		"The updated property"
//	@Failure		400				{object}	util.ValidationError	"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Resource not found"
//	@Failure		410				{object}	util.GoneError			"Resource deleted"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/properties/{property_id} [patch]
func UpdatePropertyHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")

		request := UpdatePropertyRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		property, err := s.UpdateProperty(c.Request.Context(), spaceId, propertyId, request)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(ErrPropertyNotFound, http.StatusNotFound),
			util.ErrToCode(ErrPropertyDeleted, http.StatusGone),
			util.ErrToCode(ErrFailedUpdateProperty, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedRetrieveProperty, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, PropertyResponse{Property: property})
	}
}

// DeletePropertyHandler deletes a property in a space
//
//	@Summary		Delete property
//	@Description	This endpoint “deletes” a property by marking it as archived. The deletion process is performed safely and is subject to rate limiting. It returns the property’s details after it has been archived. Proper error handling is in place for situations such as when the property isn’t found or the deletion cannot be performed because of permission issues.
//	@Tags			Properties
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			property_id		path		string					true	"Property ID"
//	@Success		200				{object}	PropertyResponse		"The deleted property"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		403				{object}	util.ForbiddenError		"Forbidden"
//	@Failure		404				{object}	util.NotFoundError		"Resource not found"
//	@Failure		410				{object}	util.GoneError			"Resource deleted"
//	@Failure		423				{object}	util.RateLimitError		"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/properties/{property_id} [delete]
func DeletePropertyHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")

		property, err := s.DeleteProperty(c.Request.Context(), spaceId, propertyId)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrPropertyNotFound, http.StatusNotFound),
			util.ErrToCode(ErrPropertyDeleted, http.StatusGone),
			util.ErrToCode(ErrFailedDeleteProperty, http.StatusForbidden),
			util.ErrToCode(ErrFailedRetrieveProperty, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, PropertyResponse{Property: property})
	}
}

// GetTagsHandler lists all tags for a given property id in a space
//
//	@Summary		List tags
//	@Description	This endpoint retrieves a paginated list of tags available for a specific property within a space. Each tag record includes its unique identifier, name, and color. This information is essential for clients to display select or multi-select options to users when they are creating or editing objects. The endpoint also supports pagination through offset and limit parameters.
//	@Tags			Tags
//	@Produce		json
//	@Param			Anytype-Version	header		string								true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string								true	"Space ID"
//	@Param			property_id		path		string								true	"Property ID"
//	@Success		200				{object}	pagination.PaginatedResponse[Tag]	"List of tags"
//	@Failure		401				{object}	util.UnauthorizedError				"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError					"Property not found"
//	@Failure		500				{object}	util.ServerError					"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/properties/{property_id}/tags [get]
func GetTagsHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		tags, total, hasMore, err := s.ListTags(c.Request.Context(), spaceId, propertyId, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrInvalidPropertyId, http.StatusNotFound),
			util.ErrToCode(ErrFailedRetrieveTags, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		pagination.RespondWithPagination(c, http.StatusOK, tags, total, offset, limit, hasMore)
	}
}

// GetTagHandler retrieves a tag for a given property id in a space.
//
//	@Summary		Get tag
//	@Description	This endpoint retrieves a tag for a given property id. The tag is identified by its unique identifier within the specified space. The response includes the tag's details such as its ID, name, and color. This is useful for clients to display or when editing a specific tag option.
//	@Tags			Tags
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			property_id		path		string					true	"Property ID"
//	@Param			tag_id			path		string					true	"Tag ID"
//	@Success		200				{object}	TagResponse				"The requested tag"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Resource not found"
//	@Failure		410				{object}	util.GoneError			"Resource deleted"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/properties/{property_id}/tags/{tag_id} [get]
func GetTagHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")
		tagId := c.Param("tag_id")

		option, err := s.GetTag(c.Request.Context(), spaceId, propertyId, tagId)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrTagNotFound, http.StatusNotFound),
			util.ErrToCode(ErrTagDeleted, http.StatusGone),
			util.ErrToCode(ErrFailedRetrieveTag, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, TagResponse{Tag: option})
	}
}

// CreateTagHandler creates a new tag for a given property id in a space
//
//	@Summary		Create tag
//	@Description	This endpoint creates a new tag for a given property id in a space. The tag is identified by its unique identifier within the specified space. The request must include the tag's name and color. The response includes the tag's details such as its ID, name, and color. This is useful for clients when users want to add new tag options to a property.
//	@Tags			Tags
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			property_id		path		string					true	"Property ID"
//	@Param			tag				body		CreateTagRequest		true	"Tag to create"
//	@Success		200				{object}	TagResponse				"The created tag"
//	@Failure		400				{object}	util.ValidationError	"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/properties/{property_id}/tags [post]
func CreateTagHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")

		request := CreateTagRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		option, err := s.CreateTag(c.Request.Context(), spaceId, propertyId, request)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(ErrFailedCreateTag, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedRetrieveTag, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, TagResponse{Tag: option})
	}
}

// UpdateTagHandler updates a tag for a given property id in a space
//
//	@Summary		Update tag
//	@Description	This endpoint updates a tag for a given property id in a space. The tag is identified by its unique identifier within the specified space. The request must include the tag's name and color. The response includes the tag's details such as its ID, name, and color. This is useful for clients when users want to edit existing tags for a property.
//	@Tags			Tags
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			property_id		path		string					true	"Property ID"
//	@Param			tag_id			path		string					true	"Tag ID"
//	@Param			tag				body		UpdateTagRequest		true	"Tag to update"
//	@Success		200				{object}	TagResponse				"The updated tag"
//	@Failure		400				{object}	util.ValidationError	"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Resource not found"
//	@Failure		410				{object}	util.GoneError			"Resource deleted"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/properties/{property_id}/tags/{tag_id} [patch]
func UpdateTagHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")
		tagId := c.Param("tag_id")

		request := UpdateTagRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		option, err := s.UpdateTag(c.Request.Context(), spaceId, propertyId, tagId, request)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(ErrTagNotFound, http.StatusNotFound),
			util.ErrToCode(ErrTagDeleted, http.StatusGone),
			util.ErrToCode(ErrFailedUpdateTag, http.StatusInternalServerError),
			util.ErrToCode(ErrFailedRetrieveTag, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, TagResponse{Tag: option})
	}
}

// DeleteTagHandler deletes a tag for a given property id in a space
//
//	@Summary		Delete tag
//	@Description	This endpoint “deletes” a tag by marking it as archived. The deletion process is performed safely and is subject to rate limiting. It returns the tag’s details after it has been archived. Proper error handling is in place for situations such as when the tag isn’t found or the deletion cannot be performed because of permission issues.
//	@Tags			Tags
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			property_id		path		string					true	"Property ID"
//	@Param			tag_id			path		string					true	"Tag ID"
//	@Success		200				{object}	TagResponse				"The deleted tag"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		403				{object}	util.ForbiddenError		"Forbidden"
//	@Failure		404				{object}	util.NotFoundError		"Resource not found"
//	@Failure		410				{object}	util.GoneError			"Resource deleted"
//	@Failure		423				{object}	util.RateLimitError		"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/properties/{property_id}/tags/{tag_id} [delete]
func DeleteTagHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")
		tagId := c.Param("tag_id")

		option, err := s.DeleteTag(c.Request.Context(), spaceId, propertyId, tagId)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrTagNotFound, http.StatusNotFound),
			util.ErrToCode(ErrTagDeleted, http.StatusGone),
			util.ErrToCode(ErrFailedDeleteTag, http.StatusForbidden),
			util.ErrToCode(ErrFailedRetrieveTag, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, TagResponse{Tag: option})
	}
}

// GetTypesHandler retrieves a list of types in a space
//
//	@Summary		List types
//	@Description	This endpoint retrieves a paginated list of object types (e.g. 'Page', 'Note', 'Task') available within the specified space. Each type’s record includes its unique identifier, type key, display name, icon, and a recommended layout. While a type's id is truly unique, a type's key can be the same across spaces for known types, e.g. 'ot-page' for 'Page'. Clients use this information when offering choices for object creation or for filtering objects by type through search.
//	@Tags			Types
//	@Produce		json
//	@Param			Anytype-Version	header		string								true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string								true	"Space ID"
//	@Param			offset			query		int									false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int									false	"The number of items to return"											default(100)	maximum(1000)
//	@Success		200				{object}	pagination.PaginatedResponse[Type]	"List of types"
//	@Failure		401				{object}	util.UnauthorizedError				"Unauthorized"
//	@Failure		500				{object}	util.ServerError					"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/types [get]
func GetTypesHandler(s Service) gin.HandlerFunc {
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
//	@Tags			Types
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			type_id			path		string					true	"Type ID"
//	@Success		200				{object}	TypeResponse			"The requested type"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Resource not found"
//	@Failure		410				{object}	util.GoneError			"Resource deleted"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/types/{type_id} [get]
func GetTypeHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		typeId := c.Param("type_id")

		object, err := s.GetType(c.Request.Context(), spaceId, typeId)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrTypeNotFound, http.StatusNotFound),
			util.ErrToCode(ErrTypeDeleted, http.StatusGone),
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
//	@Tags			Templates
//	@Produce		json
//	@Param			Anytype-Version	header		string									true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string									true	"Space ID"
//	@Param			type_id			path		string									true	"Type ID"
//	@Param			offset			query		int										false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int										false	"The number of items to return"											default(100)	maximum(1000)
//	@Success		200				{object}	pagination.PaginatedResponse[Template]	"List of templates"
//	@Failure		401				{object}	util.UnauthorizedError					"Unauthorized"
//	@Failure		500				{object}	util.ServerError						"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/types/{type_id}/templates [get]
func GetTemplatesHandler(s Service) gin.HandlerFunc {
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
//	@Tags			Templates
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			type_id			path		string					true	"Type ID"
//	@Param			template_id		path		string					true	"Template ID"
//	@Success		200				{object}	TemplateResponse		"The requested template"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Resource not found"
//	@Failure		410				{object}	util.GoneError			"Resource deleted"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/types/{type_id}/templates/{template_id} [get]
func GetTemplateHandler(s Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		typeId := c.Param("type_id")
		templateId := c.Param("template_id")

		object, err := s.GetTemplate(c.Request.Context(), spaceId, typeId, templateId)
		code := util.MapErrorCode(err,
			util.ErrToCode(ErrTemplateNotFound, http.StatusNotFound),
			util.ErrToCode(ErrTemplateDeleted, http.StatusGone),
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
