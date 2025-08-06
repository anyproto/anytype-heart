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

// ListPropertiesHandler retrieves a list of properties in a space
//
//	@Summary		List properties
//	@Description	⚠ Warning: Properties are experimental and may change in the next update. ⚠ Retrieves a paginated list of properties available within a specific space. Each property record includes its unique identifier, name and format. This information is essential for clients to understand the available properties for filtering or creating objects.
//	@Description	Supports dynamic filtering via query parameters (e.g., ?format=text, ?name[contains]=date, ?key[ne]=custom_prop). See FilterCondition enum for available conditions.
//	@Id				list_properties
//	@Tags			Properties
//	@Produce		json
//	@Param			Anytype-Version	header		string											true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string											true	"The ID of the space to list properties for; must be retrieved from ListSpaces endpoint"
//	@Param			offset			query		int												false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int												false	"The number of items to return"											default(100)	maximum(1000)
//	@Success		200				{object}	pagination.PaginatedResponse[apimodel.Property]	"The list of properties in the specified space"
//	@Failure		401				{object}	util.UnauthorizedError							"Unauthorized"
//	@Failure		500				{object}	util.ServerError								"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/properties [get]
func ListPropertiesHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		filtersAny, _ := c.Get("filters")
		filters := filtersAny.([]*model.BlockContentDataviewFilter)

		properties, total, hasMore, err := s.ListProperties(c.Request.Context(), spaceId, filters, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedRetrieveProperties, http.StatusInternalServerError),
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
//	@Description	⚠ Warning: Properties are experimental and may change in the next update. ⚠ Fetches detailed information about one specific property by its ID. This includes the property’s unique identifier, name and format. This detailed view assists clients in showing property options to users and in guiding the user interface (such as displaying appropriate input fields or selection options).
//	@Id				get_property
//	@Tags			Properties
//	@Produce		json
//	@Param			Anytype-Version	header		string						true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string						true	"The ID of the space to which the property belongs; must be retrieved from ListSpaces endpoint"
//	@Param			property_id		path		string						true	"The ID of the property to retrieve; must be retrieved from ListProperties endpoint or obtained from response context"
//	@Success		200				{object}	apimodel.PropertyResponse	"The requested property"
//	@Failure		401				{object}	util.UnauthorizedError		"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError			"Resource not found"
//	@Failure		410				{object}	util.GoneError				"Resource deleted"
//	@Failure		500				{object}	util.ServerError			"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/properties/{property_id} [get]
func GetPropertyHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")

		property, err := s.GetProperty(c.Request.Context(), spaceId, propertyId)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrPropertyNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrPropertyDeleted, http.StatusGone),
			util.ErrToCode(service.ErrFailedRetrieveProperty, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.PropertyResponse{Property: *property})
	}
}

// CreatePropertyHandler creates a new property in a space
//
//	@Summary		Create property
//	@Description	⚠ Warning: Properties are experimental and may change in the next update. ⚠ Creates a new property in the specified space using a JSON payload. The creation process is subject to rate limiting. The payload must include property details such as the name and format. The endpoint then returns the full property data, ready for further interactions.
//	@Id				create_property
//	@Tags			Properties
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string							true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string							true	"The ID of the space to create the property in; must be retrieved from ListSpaces endpoint"
//	@Param			property		body		apimodel.CreatePropertyRequest	true	"The property to create"
//	@Success		201				{object}	apimodel.PropertyResponse		"The created property"
//	@Failure		400				{object}	util.ValidationError			"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError			"Unauthorized"
//	@Failure		429				{object}	util.RateLimitError				"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError				"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/properties [post]
func CreatePropertyHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")

		request := apimodel.CreatePropertyRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		property, err := s.CreateProperty(c.Request.Context(), spaceId, request)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(service.ErrFailedCreateProperty, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedRetrieveProperty, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusCreated, apimodel.PropertyResponse{Property: *property})
	}
}

// UpdatePropertyHandler updates a property in a space
//
//	@Summary		Update property
//	@Description	⚠ Warning: Properties are experimental and may change in the next update. ⚠ This endpoint updates an existing property in the specified space using a JSON payload. The update process is subject to rate limiting. The payload must include the name to be updated. The endpoint then returns the full property data, ready for further interactions.
//	@Id				update_property
//	@Tags			Properties
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string							true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string							true	"The ID of the space to which the property belongs; must be retrieved from ListSpaces endpoint"
//	@Param			property_id		path		string							true	"The ID of the property to update; must be retrieved from ListProperties endpoint or obtained from response context"
//	@Param			property		body		apimodel.UpdatePropertyRequest	true	"The property to update"
//	@Success		200				{object}	apimodel.PropertyResponse		"The updated property"
//	@Failure		400				{object}	util.ValidationError			"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError			"Unauthorized"
//	@Failure		403				{object}	util.ForbiddenError				"Forbidden"
//	@Failure		404				{object}	util.NotFoundError				"Resource not found"
//	@Failure		410				{object}	util.GoneError					"Resource deleted"
//	@Failure		429				{object}	util.RateLimitError				"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError				"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/properties/{property_id} [patch]
func UpdatePropertyHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")

		request := apimodel.UpdatePropertyRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		property, err := s.UpdateProperty(c.Request.Context(), spaceId, propertyId, request)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(service.ErrPropertyNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrPropertyDeleted, http.StatusGone),
			util.ErrToCode(service.ErrPropertyCannotBeUpdated, http.StatusForbidden),
			util.ErrToCode(service.ErrFailedUpdateProperty, http.StatusForbidden),
			util.ErrToCode(service.ErrFailedRetrieveProperty, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.PropertyResponse{Property: *property})
	}
}

// DeletePropertyHandler deletes a property in a space
//
//	@Summary		Delete property
//	@Description	⚠ Warning: Properties are experimental and may change in the next update. ⚠ This endpoint “deletes” a property by marking it as archived. The deletion process is performed safely and is subject to rate limiting. It returns the property’s details after it has been archived. Proper error handling is in place for situations such as when the property isn’t found or the deletion cannot be performed because of permission issues.
//	@Id				delete_property
//	@Tags			Properties
//	@Produce		json
//	@Param			Anytype-Version	header		string						true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string						true	"The ID of the space to which the property belongs; must be retrieved from ListSpaces endpoint"
//	@Param			property_id		path		string						true	"The ID of the property to delete; must be retrieved from ListProperties endpoint or obtained from response context"
//	@Success		200				{object}	apimodel.PropertyResponse	"The deleted property"
//	@Failure		401				{object}	util.UnauthorizedError		"Unauthorized"
//	@Failure		403				{object}	util.ForbiddenError			"Forbidden"
//	@Failure		404				{object}	util.NotFoundError			"Resource not found"
//	@Failure		410				{object}	util.GoneError				"Resource deleted"
//	@Failure		429				{object}	util.RateLimitError			"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError			"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/properties/{property_id} [delete]
func DeletePropertyHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")

		property, err := s.DeleteProperty(c.Request.Context(), spaceId, propertyId)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrPropertyNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrPropertyDeleted, http.StatusGone),
			util.ErrToCode(service.ErrFailedDeleteProperty, http.StatusForbidden),
			util.ErrToCode(service.ErrFailedRetrieveProperty, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.PropertyResponse{Property: *property})
	}
}
