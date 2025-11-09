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

// ListObjectsHandler retrieves a list of objects in a space
//
//	@Summary		List objects
//	@Description	Retrieves a paginated list of objects in the given space. The endpoint takes query parameters for pagination (offset and limit) and returns detailed data about each object including its ID, name, icon, type information, a snippet of the content (if applicable), layout, space ID, blocks and details. It is intended for building views where users can see all objects in a space at a glance.
//	@Description	Supports dynamic filtering via query parameters (e.g., ?type=page, ?done=false, ?created_date[gte]=2024-01-01, ?tags[in]=urgent,important). For select/multi_select properties you can use either tag keys or tag IDs, for object properties use object IDs. See FilterCondition enum for available conditions.
//	@Id				list_objects
//	@Tags			Objects
//	@Produce		json
//	@Param			Anytype-Version	header		string											true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path		string											true	"The ID of the space in which to list objects; must be retrieved from ListSpaces endpoint"
//	@Param			offset			query		int												false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int												false	"The number of items to return"											default(100)	maximum(1000)
//	@Success		200				{object}	pagination.PaginatedResponse[apimodel.Object]	"The list of objects in the specified space"
//	@Failure		401				{object}	util.UnauthorizedError							"Unauthorized"
//	@Failure		500				{object}	util.ServerError								"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/objects [get]
func ListObjectsHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)

		filtersAny, _ := c.Get("filters")
		filters := filtersAny.([]*model.BlockContentDataviewFilter)

		objects, total, hasMore, err := s.ListObjects(c.Request.Context(), spaceId, filters, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedRetrieveObjects, http.StatusInternalServerError),
			util.ErrToCode(service.ErrObjectNotFound, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedRetrieveObject, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
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
//	@Id				get_object
//	@Tags			Objects
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path		string					true	"The ID of the space in which the object exists; must be retrieved from ListSpaces endpoint"
//	@Param			object_id		path		string					true	"The ID of the object to retrieve; must be retrieved from ListObjects, SearchSpace or GlobalSearch endpoints or obtained from response context"
//	@Param			format			query		apimodel.BodyFormat		false	"The format to return the object body in" default("md")
//	@Success		200				{object}	apimodel.ObjectResponse	"The retrieved object"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Resource not found"
//	@Failure		410				{object}	util.GoneError			"Resource deleted"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/objects/{object_id} [get]
func GetObjectHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		objectId := c.Param("object_id")
		// format := c.Query("format") // TODO: implement multiple formats

		object, err := s.GetObject(c.Request.Context(), spaceId, objectId)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrObjectNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrObjectDeleted, http.StatusGone),
			util.ErrToCode(service.ErrFailedRetrieveObject, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedExportMarkdown, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.ObjectResponse{Object: *object})
	}
}

// CreateObjectHandler creates a new object in a space
//
//	@Summary		Create object
//	@Description	Creates a new object in the specified space using a JSON payload. The creation process is subject to rate limiting. The payload must include key details such as the object name, icon, description, body content (which may support Markdown), source URL (required for bookmark objects), template identifier, and the type_key (which is the non-unique identifier of the type of object to create). Post-creation, additional operations (like setting featured properties or fetching bookmark metadata) may occur. The endpoint then returns the full object data, ready for further interactions.
//	@Id				create_object
//	@Tags			Objects
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string							true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path		string							true	"The ID of the space in which to create the object; must be retrieved from ListSpaces endpoint"
//	@Param			object			body		apimodel.CreateObjectRequest	true	"The object to create"
//	@Success		201				{object}	apimodel.ObjectResponse			"The created object"
//	@Failure		400				{object}	util.ValidationError			"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError			"Unauthorized"
//	@Failure		429				{object}	util.RateLimitError				"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError				"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/objects [post]
func CreateObjectHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")

		request := apimodel.CreateObjectRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToApiError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		object, err := s.CreateObject(c.Request.Context(), spaceId, request)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(service.ErrFailedCreateBookmark, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedCreateObject, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedSetPropertyFeatured, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedCreateBlock, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedPasteBody, http.StatusInternalServerError),
			util.ErrToCode(service.ErrObjectNotFound, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedRetrieveObject, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusCreated, apimodel.ObjectResponse{Object: *object})
	}
}

// UpdateObjectHandler updates an existing object in a space
//
//	@Summary		Update object
//	@Description	This endpoint updates an existing object in the specified space using a JSON payload. The update process is subject to rate limiting. The payload must include the details to be updated. The endpoint then returns the full object data, ready for further interactions.
//	@Id				update_object
//	@Tags			Objects
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string							true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path		string							true	"The ID of the space in which the object exists; must be retrieved from ListSpaces endpoint"
//	@Param			object_id		path		string							true	"The ID of the object to update; must be retrieved from ListObjects, SearchSpace or GlobalSearch endpoints or obtained from response context"
//	@Param			object			body		apimodel.UpdateObjectRequest	true	"The details of the object to update"
//	@Success		200				{object}	apimodel.ObjectResponse			"The updated object"
//	@Failure		400				{object}	util.ValidationError			"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError			"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError				"Resource not found"
//	@Failure		410				{object}	util.GoneError					"Resource deleted"
//	@Failure		429				{object}	util.RateLimitError				"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError				"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/objects/{object_id} [patch]
func UpdateObjectHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		objectId := c.Param("object_id")

		request := apimodel.UpdateObjectRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToApiError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		object, err := s.UpdateObject(c.Request.Context(), spaceId, objectId, request)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(service.ErrObjectNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrObjectDeleted, http.StatusGone),
			util.ErrToCode(service.ErrFailedUpdateObject, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedReplaceBlocks, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedRetrieveObject, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.ObjectResponse{Object: *object})
	}
}

// DeleteObjectHandler deletes an object in a space
//
//	@Summary		Delete object
//	@Description	This endpoint “deletes” an object by marking it as archived. The deletion process is performed safely and is subject to rate limiting. It returns the object’s details after it has been archived. Proper error handling is in place for situations such as when the object isn’t found or the deletion cannot be performed because of permission issues.
//	@Id				delete_object
//	@Tags			Objects
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-11-08)
//	@Param			space_id		path		string					true	"The ID of the space in which the object exists; must be retrieved from ListSpaces endpoint"
//	@Param			object_id		path		string					true	"The ID of the object to delete; must be retrieved from ListObjects, SearchSpace or GlobalSearch endpoints or obtained from response context"
//	@Success		200				{object}	apimodel.ObjectResponse	"The deleted object"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		403				{object}	util.ForbiddenError		"Forbidden"
//	@Failure		404				{object}	util.NotFoundError		"Resource not found"
//	@Failure		410				{object}	util.GoneError			"Resource deleted"
//	@Failure		429				{object}	util.RateLimitError		"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/objects/{object_id} [delete]
func DeleteObjectHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		objectId := c.Param("object_id")

		object, err := s.DeleteObject(c.Request.Context(), spaceId, objectId)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrObjectNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrObjectDeleted, http.StatusGone),
			util.ErrToCode(service.ErrFailedDeleteObject, http.StatusForbidden),
			util.ErrToCode(service.ErrFailedRetrieveObject, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToApiError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.ObjectResponse{Object: *object})
	}
}
