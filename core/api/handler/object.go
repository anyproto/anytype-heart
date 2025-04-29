package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/apimodel"
	"github.com/anyproto/anytype-heart/core/api/internal"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// ListObjectsHandler retrieves a list of objects in a space
//
//	@Summary		List objects
//	@Description	Retrieves a paginated list of objects in the given space. The endpoint takes query parameters for pagination (offset and limit) and returns detailed data about each object including its ID, name, icon, type information, a snippet of the content (if applicable), layout, space ID, blocks and details. It is intended for building views where users can see all objects in a space at a glance.
//	@Id				listObjects
//	@Tags			Objects
//	@Produce		json
//	@Param			Anytype-Version	header		string											true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string											true	"Space ID"
//	@Param			offset			query		int												false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int												false	"The number of items to return"											default(100)	maximum(1000)
//	@Success		200				{object}	pagination.PaginatedResponse[apimodel.Object]	"List of objects"
//	@Failure		401				{object}	util.UnauthorizedError							"Unauthorized"
//	@Failure		500				{object}	util.ServerError								"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/objects [get]
func ListObjectsHandler(s *internal.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		objects, total, hasMore, err := s.ListObjects(c.Request.Context(), spaceId, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(internal.ErrFailedRetrieveObjects, http.StatusInternalServerError),
			util.ErrToCode(internal.ErrObjectNotFound, http.StatusInternalServerError),
			util.ErrToCode(internal.ErrFailedRetrieveObject, http.StatusInternalServerError),
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
//	@Id				getObject
//	@Tags			Objects
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			object_id		path		string					true	"Object ID"
//	@Success		200				{object}	apimodel.ObjectResponse	"The requested object"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Resource not found"
//	@Failure		410				{object}	util.GoneError			"Resource deleted"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/objects/{object_id} [get]
func GetObjectHandler(s *internal.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		objectId := c.Param("object_id")

		object, err := s.GetObject(c.Request.Context(), spaceId, objectId)
		code := util.MapErrorCode(err,
			util.ErrToCode(internal.ErrObjectNotFound, http.StatusNotFound),
			util.ErrToCode(internal.ErrObjectDeleted, http.StatusGone),
			util.ErrToCode(internal.ErrFailedRetrieveObject, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.ObjectResponse{Object: object})
	}
}

// DeleteObjectHandler deletes an object in a space
//
//	@Summary		Delete object
//	@Description	This endpoint “deletes” an object by marking it as archived. The deletion process is performed safely and is subject to rate limiting. It returns the object’s details after it has been archived. Proper error handling is in place for situations such as when the object isn’t found or the deletion cannot be performed because of permission issues.
//	@Id				deleteObject
//	@Tags			Objects
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			object_id		path		string					true	"Object ID"
//	@Success		200				{object}	apimodel.ObjectResponse	"The deleted object"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		403				{object}	util.ForbiddenError		"Forbidden"
//	@Failure		404				{object}	util.NotFoundError		"Resource not found"
//	@Failure		410				{object}	util.GoneError			"Resource deleted"
//	@Failure		423				{object}	util.RateLimitError		"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/objects/{object_id} [delete]
func DeleteObjectHandler(s *internal.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		objectId := c.Param("object_id")

		object, err := s.DeleteObject(c.Request.Context(), spaceId, objectId)
		code := util.MapErrorCode(err,
			util.ErrToCode(internal.ErrObjectNotFound, http.StatusNotFound),
			util.ErrToCode(internal.ErrObjectDeleted, http.StatusGone),
			util.ErrToCode(internal.ErrFailedDeleteObject, http.StatusForbidden),
			util.ErrToCode(internal.ErrFailedRetrieveObject, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.ObjectResponse{Object: object})
	}
}

// CreateObjectHandler creates a new object in a space
//
//	@Summary		Create object
//	@Description	Creates a new object in the specified space using a JSON payload. The creation process is subject to rate limiting. The payload must include key details such as the object name, icon, description, body content (which may support Markdown), source URL (required for bookmark objects), template identifier, and the type_key (which is the non-unique identifier of the type of object to create). Post-creation, additional operations (like setting featured properties or fetching bookmark metadata) may occur. The endpoint then returns the full object data, ready for further interactions.
//	@Id				createObject
//	@Tags			Objects
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string							true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string							true	"Space ID"
//	@Param			object			body		apimodel.CreateObjectRequest	true	"Object to create"
//	@Success		200				{object}	apimodel.ObjectResponse			"The created object"
//	@Failure		400				{object}	util.ValidationError			"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError			"Unauthorized"
//	@Failure		423				{object}	util.RateLimitError				"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError				"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/objects [post]
func CreateObjectHandler(s *internal.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")

		request := apimodel.CreateObjectRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		object, err := s.CreateObject(c.Request.Context(), spaceId, request)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(internal.ErrFailedCreateBookmark, http.StatusInternalServerError),
			util.ErrToCode(internal.ErrFailedCreateObject, http.StatusInternalServerError),
			util.ErrToCode(internal.ErrFailedSetPropertyFeatured, http.StatusInternalServerError),
			util.ErrToCode(internal.ErrFailedCreateBlock, http.StatusInternalServerError),
			util.ErrToCode(internal.ErrFailedPasteBody, http.StatusInternalServerError),
			util.ErrToCode(internal.ErrObjectNotFound, http.StatusInternalServerError),
			util.ErrToCode(internal.ErrFailedRetrieveObject, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.ObjectResponse{Object: object})
	}
}

// UpdateObjectHandler updates an existing object in a space
//
//	@Summary		Update object
//	@Description	This endpoint updates an existing object in the specified space using a JSON payload. The update process is subject to rate limiting. The payload must include the details to be updated. The endpoint then returns the full object data, ready for further interactions.
//	@Id				updateObject
//	@Tags			Objects
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string							true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string							true	"Space ID"
//	@Param			object_id		path		string							true	"Object ID"
//	@Param			object			body		apimodel.UpdateObjectRequest	true	"Object to update"
//	@Success		200				{object}	apimodel.ObjectResponse			"The updated object"
//	@Failure		400				{object}	util.ValidationError			"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError			"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError				"Resource not found"
//	@Failure		410				{object}	util.GoneError					"Resource deleted"
//	@Failure		500				{object}	util.ServerError				"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/objects/{object_id} [patch]
func UpdateObjectHandler(s *internal.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		objectId := c.Param("object_id")

		request := apimodel.UpdateObjectRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		object, err := s.UpdateObject(c.Request.Context(), spaceId, objectId, request)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(internal.ErrObjectNotFound, http.StatusNotFound),
			util.ErrToCode(internal.ErrObjectDeleted, http.StatusGone),
			util.ErrToCode(internal.ErrFailedUpdateObject, http.StatusInternalServerError),
			util.ErrToCode(internal.ErrFailedRetrieveObject, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.ObjectResponse{Object: object})
	}
}

// ExportObjectHandler exports an object in specified format
//
//	@Summary		Export object
//	@Description	This endpoint exports a single object from the specified space into a desired format. The export format is provided as a path parameter (currently supporting “markdown” only). The endpoint calls the export service which converts the object’s content into the requested format. It is useful for sharing, or displaying the markdown representation of the objecte externally.
//	@Id				exportObject
//	@Tags			Objects
//	@Produce		json
//	@Param			Anytype-Version	header		string							true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string							true	"Space ID"
//	@Param			object_id		path		string							true	"Object ID"
//	@Param			format			path		string							true	"Export format"	Enums(markdown)
//	@Success		200				{object}	apimodel.ObjectExportResponse	"Object exported successfully"
//	@Failure		400				{object}	util.ValidationError			"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError			"Unauthorized"
//	@Failure		500				{object}	util.ServerError				"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/objects/{object_id}/{format} [get]
func ExportObjectHandler(s *internal.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		objectId := c.Param("object_id")
		format := c.Param("format")

		markdown, err := s.GetObjectExport(c.Request.Context(), spaceId, objectId, format)
		code := util.MapErrorCode(err,
			util.ErrToCode(internal.ErrInvalidExportFormat, http.StatusInternalServerError))

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.ObjectExportResponse{Markdown: markdown})
	}
}
