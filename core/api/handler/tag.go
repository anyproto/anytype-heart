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

// ListTagsHandler lists all tags for a given property id in a space
//
//	@Summary		List tags
//	@Description	This endpoint retrieves a paginated list of tags available for a specific property within a space. Each tag record includes its unique identifier, name, and color. This information is essential for clients to display select or multi-select options to users when they are creating or editing objects. The endpoint also supports pagination through offset and limit parameters.
//	@Id				list_tags
//	@Tags			Tags
//	@Produce		json
//	@Param			Anytype-Version	header		string										true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string										true	"The ID of the space to list tags for; must be retrieved from ListSpaces endpoint"
//	@Param			property_id		path		string										true	"The ID of the property to list tags for; must be retrieved from ListProperties endpoint or obtained from response context"
//	@Success		200				{object}	pagination.PaginatedResponse[apimodel.Tag]	"The list of tags"
//	@Failure		401				{object}	util.UnauthorizedError						"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError							"Property not found"
//	@Failure		500				{object}	util.ServerError							"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/properties/{property_id}/tags [get]
func ListTagsHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		filtersAny, _ := c.Get("filters")
		filters := filtersAny.([]*model.BlockContentDataviewFilter)

		tags, total, hasMore, err := s.ListTags(c.Request.Context(), spaceId, propertyId, filters, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrInvalidPropertyId, http.StatusNotFound),
			util.ErrToCode(service.ErrFailedRetrieveTags, http.StatusInternalServerError),
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
//	@Id				get_tag
//	@Tags			Tags
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string					true	"The ID of the space to retrieve the tag from; must be retrieved from ListSpaces endpoint"
//	@Param			property_id		path		string					true	"The ID of the property to retrieve the tag for; must be retrieved from ListProperties endpoint or obtained from response context"
//	@Param			tag_id			path		string					true	"The ID of the tag to retrieve; must be retrieved from ListTags endpoint or obtained from response context"
//	@Success		200				{object}	apimodel.TagResponse	"The retrieved tag"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Resource not found"
//	@Failure		410				{object}	util.GoneError			"Resource deleted"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/properties/{property_id}/tags/{tag_id} [get]
func GetTagHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")
		tagId := c.Param("tag_id")

		tag, err := s.GetTag(c.Request.Context(), spaceId, propertyId, tagId)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrTagNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrTagDeleted, http.StatusGone),
			util.ErrToCode(service.ErrFailedRetrieveTag, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.TagResponse{Tag: *tag})
	}
}

// CreateTagHandler creates a new tag for a given property id in a space
//
//	@Summary		Create tag
//	@Description	This endpoint creates a new tag for a given property id in a space. The creation process is subject to rate limiting. The tag is identified by its unique identifier within the specified space. The request must include the tag's name and color. The response includes the tag's details such as its ID, name, and color. This is useful for clients when users want to add new tag options to a property.
//	@Id				create_tag
//	@Tags			Tags
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string						true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string						true	"The ID of the space to create the tag in; must be retrieved from ListSpaces endpoint"
//	@Param			property_id		path		string						true	"The ID of the property to create the tag for; must be retrieved from ListProperties endpoint or obtained from response context"
//	@Param			tag				body		apimodel.CreateTagRequest	true	"The tag to create"
//	@Success		201				{object}	apimodel.TagResponse		"The created tag"
//	@Failure		400				{object}	util.ValidationError		"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError		"Unauthorized"
//	@Failure		429				{object}	util.RateLimitError			"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError			"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/properties/{property_id}/tags [post]
func CreateTagHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")

		request := apimodel.CreateTagRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		tag, err := s.CreateTag(c.Request.Context(), spaceId, propertyId, request)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(service.ErrFailedCreateTag, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedRetrieveTag, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusCreated, apimodel.TagResponse{Tag: *tag})
	}
}

// UpdateTagHandler updates a tag for a given property id in a space
//
//	@Summary		Update tag
//	@Description	This endpoint updates a tag for a given property id in a space. The update process is subject to rate limiting. The tag is identified by its unique identifier within the specified space. The request must include the tag's name and color. The response includes the tag's details such as its ID, name, and color. This is useful for clients when users want to edit existing tags for a property.
//	@Id				update_tag
//	@Tags			Tags
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string						true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string						true	"The ID of the space to update the tag in; must be retrieved from ListSpaces endpoint"
//	@Param			property_id		path		string						true	"The ID of the property to update the tag for; must be retrieved from ListProperties endpoint or obtained from response context"
//	@Param			tag_id			path		string						true	"The ID of the tag to update; must be retrieved from ListTags endpoint or obtained from response context"
//	@Param			tag				body		apimodel.UpdateTagRequest	true	"The tag to update"
//	@Success		200				{object}	apimodel.TagResponse		"The updated tag"
//	@Failure		400				{object}	util.ValidationError		"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError		"Unauthorized"
//	@Failure		403				{object}	util.ForbiddenError			"Forbidden"
//	@Failure		404				{object}	util.NotFoundError			"Resource not found"
//	@Failure		410				{object}	util.GoneError				"Resource deleted"
//	@Failure		429				{object}	util.RateLimitError			"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError			"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/properties/{property_id}/tags/{tag_id} [patch]
func UpdateTagHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")
		tagId := c.Param("tag_id")

		request := apimodel.UpdateTagRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		tag, err := s.UpdateTag(c.Request.Context(), spaceId, propertyId, tagId, request)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(service.ErrTagNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrTagDeleted, http.StatusGone),
			util.ErrToCode(service.ErrFailedUpdateTag, http.StatusForbidden),
			util.ErrToCode(service.ErrFailedRetrieveTag, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.TagResponse{Tag: *tag})
	}
}

// DeleteTagHandler deletes a tag for a given property id in a space
//
//	@Summary		Delete tag
//	@Description	This endpoint “deletes” a tag by marking it as archived. The deletion process is performed safely and is subject to rate limiting. It returns the tag’s details after it has been archived. Proper error handling is in place for situations such as when the tag isn’t found or the deletion cannot be performed because of permission issues.
//	@Id				delete_tag
//	@Tags			Tags
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string					true	"The ID of the space to delete the tag from; must be retrieved from ListSpaces endpoint"
//	@Param			property_id		path		string					true	"The ID of the property to delete the tag for; must be retrieved from ListProperties endpoint or obtained from response context"
//	@Param			tag_id			path		string					true	"The ID of the tag to delete; must be retrieved from ListTags endpoint or obtained from response context"
//	@Success		200				{object}	apimodel.TagResponse	"The deleted tag"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		403				{object}	util.ForbiddenError		"Forbidden"
//	@Failure		404				{object}	util.NotFoundError		"Resource not found"
//	@Failure		410				{object}	util.GoneError			"Resource deleted"
//	@Failure		429				{object}	util.RateLimitError		"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/properties/{property_id}/tags/{tag_id} [delete]
func DeleteTagHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")
		tagId := c.Param("tag_id")

		tag, err := s.DeleteTag(c.Request.Context(), spaceId, propertyId, tagId)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrTagNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrTagDeleted, http.StatusGone),
			util.ErrToCode(service.ErrFailedDeleteTag, http.StatusForbidden),
			util.ErrToCode(service.ErrFailedRetrieveTag, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.TagResponse{Tag: *tag})
	}
}
