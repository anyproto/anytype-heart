package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/apimodel"
	"github.com/anyproto/anytype-heart/core/api/internal/object"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// ListPropertiesHandler retrieves a list of properties in a space
//
//	@Summary		List properties
//	@Description	This endpoint retrieves a paginated list of properties available within a specific space. Each property record includes its unique identifier, name and format. This information is essential for clients to understand the available properties for filtering or creating objects.
//	@Id				listProperties
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
func ListPropertiesHandler(s object.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		properties, total, hasMore, err := s.ListProperties(c.Request.Context(), spaceId, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(object.ErrFailedRetrieveProperties, http.StatusInternalServerError),
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
//	@Id				getProperty
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
func GetPropertyHandler(s object.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")

		property, err := s.GetProperty(c.Request.Context(), spaceId, propertyId)
		code := util.MapErrorCode(err,
			util.ErrToCode(object.ErrPropertyNotFound, http.StatusNotFound),
			util.ErrToCode(object.ErrPropertyDeleted, http.StatusGone),
			util.ErrToCode(object.ErrFailedRetrieveProperty, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.PropertyResponse{Property: property})
	}
}

// CreatePropertyHandler creates a new property in a space
//
//	@Summary		Create property
//	@Description	Creates a new property in the specified space using a JSON payload. The creation process is subject to rate limiting. The payload must include property details such as the name and format. The endpoint then returns the full property data, ready for further interactions.
//	@Id				createProperty
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
func CreatePropertyHandler(s object.Service) gin.HandlerFunc {
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
			util.ErrToCode(object.ErrFailedCreateProperty, http.StatusInternalServerError),
			util.ErrToCode(object.ErrFailedRetrieveProperty, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.PropertyResponse{Property: property})
	}
}

// UpdatePropertyHandler updates a property in a space
//
//	@Summary		Update property
//	@Description	This endpoint updates an existing property in the specified space using a JSON payload. The update process is subject to rate limiting. The payload must include the name to be updated. The endpoint then returns the full property data, ready for further interactions.
//	@Id				updateProperty
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
func UpdatePropertyHandler(s object.Service) gin.HandlerFunc {
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
			util.ErrToCode(object.ErrPropertyNotFound, http.StatusNotFound),
			util.ErrToCode(object.ErrPropertyDeleted, http.StatusGone),
			util.ErrToCode(object.ErrFailedUpdateProperty, http.StatusInternalServerError),
			util.ErrToCode(object.ErrFailedRetrieveProperty, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.PropertyResponse{Property: property})
	}
}

// DeletePropertyHandler deletes a property in a space
//
//	@Summary		Delete property
//	@Description	This endpoint “deletes” a property by marking it as archived. The deletion process is performed safely and is subject to rate limiting. It returns the property’s details after it has been archived. Proper error handling is in place for situations such as when the property isn’t found or the deletion cannot be performed because of permission issues.
//	@Id				deleteProperty
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
func DeletePropertyHandler(s object.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")

		property, err := s.DeleteProperty(c.Request.Context(), spaceId, propertyId)
		code := util.MapErrorCode(err,
			util.ErrToCode(object.ErrPropertyNotFound, http.StatusNotFound),
			util.ErrToCode(object.ErrPropertyDeleted, http.StatusGone),
			util.ErrToCode(object.ErrFailedDeleteProperty, http.StatusForbidden),
			util.ErrToCode(object.ErrFailedRetrieveProperty, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.PropertyResponse{Property: property})
	}
}
