package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/apimodel"
	"github.com/anyproto/anytype-heart/core/api/internal"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// ListTagsHandler lists all tags for a given property id in a space
//
//	@Summary		List tags
//	@Description	This endpoint retrieves a paginated list of tags available for a specific property within a space. Each tag record includes its unique identifier, name, and color. This information is essential for clients to display select or multi-select options to users when they are creating or editing objects. The endpoint also supports pagination through offset and limit parameters.
//	@Id				listTags
//	@Tags			Tags
//	@Produce		json
//	@Param			Anytype-Version	header		string										true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string										true	"Space ID"
//	@Param			property_id		path		string										true	"Property ID"
//	@Success		200				{object}	pagination.PaginatedResponse[apimodel.Tag]	"List of tags"
//	@Failure		401				{object}	util.UnauthorizedError						"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError							"Property not found"
//	@Failure		500				{object}	util.ServerError							"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/properties/{property_id}/tags [get]
func ListTagsHandler(s *internal.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		tags, total, hasMore, err := s.ListTags(c.Request.Context(), spaceId, propertyId, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(internal.ErrInvalidPropertyId, http.StatusNotFound),
			util.ErrToCode(internal.ErrFailedRetrieveTags, http.StatusInternalServerError),
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
//	@Id				getTag
//	@Tags			Tags
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			property_id		path		string					true	"Property ID"
//	@Param			tag_id			path		string					true	"Tag ID"
//	@Success		200				{object}	apimodel.TagResponse	"The requested tag"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError		"Resource not found"
//	@Failure		410				{object}	util.GoneError			"Resource deleted"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/properties/{property_id}/tags/{tag_id} [get]
func GetTagHandler(s *internal.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")
		tagId := c.Param("tag_id")

		option, err := s.GetTag(c.Request.Context(), spaceId, propertyId, tagId)
		code := util.MapErrorCode(err,
			util.ErrToCode(internal.ErrTagNotFound, http.StatusNotFound),
			util.ErrToCode(internal.ErrTagDeleted, http.StatusGone),
			util.ErrToCode(internal.ErrFailedRetrieveTag, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.TagResponse{Tag: option})
	}
}

// CreateTagHandler creates a new tag for a given property id in a space
//
//	@Summary		Create tag
//	@Description	This endpoint creates a new tag for a given property id in a space. The creation process is subject to rate limiting. The tag is identified by its unique identifier within the specified space. The request must include the tag's name and color. The response includes the tag's details such as its ID, name, and color. This is useful for clients when users want to add new tag options to a property.
//	@Id				createTag
//	@Tags			Tags
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string						true	"The version of the API to use"	default(2025-04-22
//	@Param			space_id		path		string						true	"Space ID"
//	@Param			property_id		path		string						true	"Property ID"
//	@Param			tag				body		apimodel.CreateTagRequest	true	"Tag to create"
//	@Success		200				{object}	apimodel.TagResponse		"The created tag"
//	@Failure		400				{object}	util.ValidationError		"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError		"Unauthorized"
//	@Failure		500				{object}	util.ServerError			"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/properties/{property_id}/tags [post]
func CreateTagHandler(s *internal.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")

		request := apimodel.CreateTagRequest{}
		if err := c.BindJSON(&request); err != nil {
			apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
			c.JSON(http.StatusBadRequest, apiErr)
			return
		}

		option, err := s.CreateTag(c.Request.Context(), spaceId, propertyId, request)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(internal.ErrFailedCreateTag, http.StatusInternalServerError),
			util.ErrToCode(internal.ErrFailedRetrieveTag, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.TagResponse{Tag: option})
	}
}

// UpdateTagHandler updates a tag for a given property id in a space
//
//	@Summary		Update tag
//	@Description	This endpoint updates a tag for a given property id in a space. The update process is subject to rate limiting. The tag is identified by its unique identifier within the specified space. The request must include the tag's name and color. The response includes the tag's details such as its ID, name, and color. This is useful for clients when users want to edit existing tags for a property.
//	@Id				updateTag
//	@Tags			Tags
//	@Accept			json
//	@Produce		json
//	@Param			Anytype-Version	header		string						true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string						true	"Space ID"
//	@Param			property_id		path		string						true	"Property ID"
//	@Param			tag_id			path		string						true	"Tag ID"
//	@Param			tag				body		apimodel.UpdateTagRequest	true	"Tag to update"
//	@Success		200				{object}	apimodel.TagResponse		"The updated tag"
//	@Failure		400				{object}	util.ValidationError		"Bad request"
//	@Failure		401				{object}	util.UnauthorizedError		"Unauthorized"
//	@Failure		403				{object}	util.ForbiddenError			"Forbidden"
//	@Failure		404				{object}	util.NotFoundError			"Resource not found"
//	@Failure		410				{object}	util.GoneError				"Resource deleted"
//	@Failure		500				{object}	util.ServerError			"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/properties/{property_id}/tags/{tag_id} [patch]
func UpdateTagHandler(s *internal.Service) gin.HandlerFunc {
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

		option, err := s.UpdateTag(c.Request.Context(), spaceId, propertyId, tagId, request)
		code := util.MapErrorCode(err,
			util.ErrToCode(util.ErrBad, http.StatusBadRequest),
			util.ErrToCode(internal.ErrTagNotFound, http.StatusNotFound),
			util.ErrToCode(internal.ErrTagDeleted, http.StatusGone),
			util.ErrToCode(internal.ErrFailedUpdateTag, http.StatusForbidden),
			util.ErrToCode(internal.ErrFailedRetrieveTag, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.TagResponse{Tag: option})
	}
}

// DeleteTagHandler deletes a tag for a given property id in a space
//
//	@Summary		Delete tag
//	@Description	This endpoint “deletes” a tag by marking it as archived. The deletion process is performed safely and is subject to rate limiting. It returns the tag’s details after it has been archived. Proper error handling is in place for situations such as when the tag isn’t found or the deletion cannot be performed because of permission issues.
//	@Id				deleteTag
//	@Tags			Tags
//	@Produce		json
//	@Param			Anytype-Version	header		string					true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string					true	"Space ID"
//	@Param			property_id		path		string					true	"Property ID"
//	@Param			tag_id			path		string					true	"Tag ID"
//	@Success		200				{object}	apimodel.TagResponse	"The deleted tag"
//	@Failure		401				{object}	util.UnauthorizedError	"Unauthorized"
//	@Failure		403				{object}	util.ForbiddenError		"Forbidden"
//	@Failure		404				{object}	util.NotFoundError		"Resource not found"
//	@Failure		410				{object}	util.GoneError			"Resource deleted"
//	@Failure		423				{object}	util.RateLimitError		"Rate limit exceeded"
//	@Failure		500				{object}	util.ServerError		"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/properties/{property_id}/tags/{tag_id} [delete]
func DeleteTagHandler(s *internal.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		propertyId := c.Param("property_id")
		tagId := c.Param("tag_id")

		option, err := s.DeleteTag(c.Request.Context(), spaceId, propertyId, tagId)
		code := util.MapErrorCode(err,
			util.ErrToCode(internal.ErrTagNotFound, http.StatusNotFound),
			util.ErrToCode(internal.ErrTagDeleted, http.StatusGone),
			util.ErrToCode(internal.ErrFailedDeleteTag, http.StatusForbidden),
			util.ErrToCode(internal.ErrFailedRetrieveTag, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.TagResponse{Tag: option})
	}
}
