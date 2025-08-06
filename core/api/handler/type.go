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

// ListTypesHandler retrieves a list of types in a space
//
//	@Summary		List types
//	@Description	This endpoint retrieves a paginated list of types (e.g. 'Page', 'Note', 'Task') available within the specified space. Each type's record includes its unique identifier, type key, display name, icon, and layout. While a type's id is truly unique, a type's key can be the same across spaces for known types, e.g. 'page' for 'Page'. Clients use this information when offering choices for object creation or for filtering objects by type through search.
//	@Description	Supports dynamic filtering via query parameters (e.g., ?key=page, ?name[contains]=task, ?layout[ne]=note). See FilterCondition enum for available conditions.
//	@Id				list_types
//	@Tags			Types
//	@Produce		json
//	@Param			Anytype-Version	header		string										true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string										true	"The ID of the space to retrieve types from; must be retrieved from ListSpaces endpoint"
//	@Param			offset			query		int											false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int											false	"The number of items to return"											default(100)	maximum(1000)
//	@Success		200				{object}	pagination.PaginatedResponse[apimodel.Type]	"The list of types"
//	@Failure		401				{object}	util.UnauthorizedError						"Unauthorized"
//	@Failure		500				{object}	util.ServerError							"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/types [get]
func ListTypesHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		filtersAny, _ := c.Get("filters")
		filters := filtersAny.([]*model.BlockContentDataviewFilter)

		types, total, hasMore, err := s.ListTypes(c.Request.Context(), spaceId, filters, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedRetrieveTypes, http.StatusInternalServerError),
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
//	@Description	Fetches detailed information about one specific type by its ID. This includes the type’s unique key, name, icon, and layout. This detailed view assists clients in understanding the expected structure and style for objects of that type and in guiding the user interface (such as displaying appropriate icons or layout hints).
//	@Id				get_type
//	@Tags			Types
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string					true	"The ID of the space from which to retrieve the type; must be retrieved from ListSpaces endpoint"
//	@Param			type_id			path		string					true	"The ID of the type to retrieve; must be retrieved from ListTypes endpoint or obtained from response context"
//	@Success		200				{object}	apimodel.TypeResponse	"The requested type"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Resource not found"
//	@Failure		410				{object}	util.GoneError			"Resource deleted"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/types/{type_id} [get]
func GetTypeHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		typeId := c.Param("type_id")

		object, err := s.GetType(c.Request.Context(), spaceId, typeId)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrTypeNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrTypeDeleted, http.StatusGone),
			util.ErrToCode(service.ErrFailedRetrieveType, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.TypeResponse{Type: *object})
	}
}

// CreateTypeHandler creates a new type in a space
//
//	@Summary		Create type
//	@Description	Creates a new type in the specified space using a JSON payload. The creation process is subject to rate limiting. The payload must include type details such as the name, icon, and layout. The endpoint then returns the full type data, ready to be used for creating objects.
//	@Id				create_type
//	@Tags			Types
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string						true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string						true	"The ID of the space in which to create the type; must be retrieved from ListSpaces endpoint"
//	@Param			type			body		apimodel.CreateTypeRequest	true	"The type to create"
//	@Success		201				{object}	apimodel.TypeResponse		"The created type"
//	@Failure		400				{object}	util.ValidationError		"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError		"Unauthorized"
//	@Failure		429				{object}	util.RateLimitError			"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError			"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/types [post]
func CreateTypeHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")

		request := apimodel.CreateTypeRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		object, err := s.CreateType(c.Request.Context(), spaceId, request)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(service.ErrFailedCreateType, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedRetrieveType, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusCreated, apimodel.TypeResponse{Type: *object})
	}
}

// UpdateTypeHandler updates a type in a space
//
//	@Summary		Update type
//	@Description	This endpoint updates an existing type in the specified space using a JSON payload. The update process is subject to rate limiting. The payload must include the name and properties to be updated. The endpoint then returns the full type data, ready for further interactions.
//	@Id				update_type
//	@Tags			Types
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string						true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string						true	"The ID of the space in which the type exists; must be retrieved from ListSpaces endpoint"
//	@Param			type_id			path		string						true	"The ID of the type to update; must be retrieved from ListTypes endpoint or obtained from response context"
//	@Param			type			body		apimodel.UpdateTypeRequest	true	"The type details to update"
//	@Success		200				{object}	apimodel.TypeResponse		"The updated type"
//	@Failure		400				{object}	util.ValidationError		"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError		"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError			"Resource not found"
//	@Failure		410				{object}	util.GoneError				"Resource deleted"
//	@Failure		429				{object}	util.RateLimitError			"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError			"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/types/{type_id} [patch]
func UpdateTypeHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		typeId := c.Param("type_id")

		request := apimodel.UpdateTypeRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		object, err := s.UpdateType(c.Request.Context(), spaceId, typeId, request)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(service.ErrTypeNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrTypeDeleted, http.StatusGone),
			util.ErrToCode(service.ErrFailedUpdateType, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedRetrieveType, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.TypeResponse{Type: *object})
	}
}

// DeleteTypeHandler deletes a type in a space
//
//	@Summary		Delete type
//	@Description	This endpoint “deletes” an type by marking it as archived. The deletion process is performed safely and is subject to rate limiting. It returns the type’s details after it has been archived. Proper error handling is in place for situations such as when the type isn’t found or the deletion cannot be performed because of permission issues.
//	@Id				delete_type
//	@Tags			Types
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string					true	"The ID of the space from which to delete the type; must be retrieved from ListSpaces endpoint"
//	@Param			type_id			path		string					true	"The ID of the type to delete; must be retrieved from ListTypes endpoint or obtained from response context"
//	@Success		200				{object}	apimodel.TypeResponse	"The deleted type"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		403				{object}	util.ForbiddenError		"Forbidden"
//	@Failure		404				{object}	util.NotFoundError		"Resource not found"
//	@Failure		410				{object}	util.GoneError			"Resource deleted"
//	@Failure		429				{object}	util.RateLimitError		"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/types/{type_id} [delete]
func DeleteTypeHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		typeId := c.Param("type_id")

		object, err := s.DeleteType(c.Request.Context(), spaceId, typeId)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrTypeNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrTypeDeleted, http.StatusGone),
			util.ErrToCode(service.ErrFailedDeleteType, http.StatusForbidden),
			util.ErrToCode(service.ErrFailedRetrieveType, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.TypeResponse{Type: *object})
	}
}
