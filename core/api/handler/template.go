package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/apimodel"
	"github.com/anyproto/anytype-heart/core/api/internal"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// ListTemplatesHandler retrieves a list of templates for a type in a space
//
//	@Summary		List templates
//	@Description	This endpoint returns a paginated list of templates that are associated with a specific object type within a space. Templates provide pre‑configured structures for creating new objects. Each template record contains its identifier, name, and icon, so that clients can offer users a selection of templates when creating objects.
//	@Id				listTemplates
//	@Tags			Templates
//	@Produce		json
//	@Param			Anytype-Version	header		string									true	"The version of the API to use"	default(2025-04-22)
//	@Param			space_id		path		string									true	"Space ID"
//	@Param			type_id			path		string									true	"Type ID"
//	@Param			offset			query		int										false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int										false	"The number of items to return"											default(100)	maximum(1000)
//	@Success		200				{object}	pagination.PaginatedResponse[Object]	"List of templates"
//	@Failure		401				{object}	util.UnauthorizedError					"Unauthorized"
//	@Failure		500				{object}	util.ServerError						"Internal server error"
//	@Security		bearerauth
//	@Router			/spaces/{space_id}/types/{type_id}/templates [get]
func ListTemplatesHandler(s *internal.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		typeId := c.Param("type_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		templates, total, hasMore, err := s.ListTemplates(c.Request.Context(), spaceId, typeId, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(internal.ErrFailedRetrieveTemplateType, http.StatusInternalServerError),
			util.ErrToCode(internal.ErrTemplateTypeNotFound, http.StatusInternalServerError),
			util.ErrToCode(internal.ErrFailedRetrieveTemplates, http.StatusInternalServerError),
			util.ErrToCode(internal.ErrFailedRetrieveTemplate, http.StatusInternalServerError),
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
//	@Id				getTemplate
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
func GetTemplateHandler(s *internal.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		typeId := c.Param("type_id")
		templateId := c.Param("template_id")

		template, err := s.GetTemplate(c.Request.Context(), spaceId, typeId, templateId)
		code := util.MapErrorCode(err,
			util.ErrToCode(internal.ErrTemplateNotFound, http.StatusNotFound),
			util.ErrToCode(internal.ErrTemplateDeleted, http.StatusGone),
			util.ErrToCode(internal.ErrFailedRetrieveTemplate, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.TemplateResponse{Template: template})
	}
}
