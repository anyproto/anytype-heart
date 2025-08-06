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

// ListTemplatesHandler retrieves a list of templates for a type in a space
//
//	@Summary		List templates
//	@Description	This endpoint returns a paginated list of templates that are associated with a specific type within a space. Templates provide pre‑configured structures for creating new objects. Each template record contains its identifier, name, and icon, so that clients can offer users a selection of templates when creating objects.
//	@Description	Supports dynamic filtering via query parameters (e.g., ?name[contains]=invoice, ?is_default=true). See FilterCondition enum for available conditions.
//	@Id				list_templates
//	@Tags			Templates
//	@Produce		json
//	@Param			Anytype-Version	header		string											true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string											true	"The ID of the space to which the type belongs; must be retrieved from ListSpaces endpoint"
//	@Param			type_id			path		string											true	"The ID of the type to retrieve templates for; must be retrieved from ListTypes endpoint or obtained from response context"
//	@Param			offset			query		int												false	"The number of items to skip before starting to collect the result set"	default(0)
//	@Param			limit			query		int												false	"The number of items to return"											default(100)	maximum(1000)
//	@Success		200				{object}	pagination.PaginatedResponse[apimodel.Object]	"List of templates"
//	@Failure		401				{object}	util.UnauthorizedError							"Unauthorized"
//	@Failure		500				{object}	util.ServerError								"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/types/{type_id}/templates [get]
func ListTemplatesHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		typeId := c.Param("type_id")
		offset := c.GetInt("offset")
		limit := c.GetInt("limit")

		filtersAny, _ := c.Get("filters")
		filters := filtersAny.([]*model.BlockContentDataviewFilter)

		templates, total, hasMore, err := s.ListTemplates(c.Request.Context(), spaceId, typeId, filters, offset, limit)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrFailedRetrieveTemplateType, http.StatusInternalServerError),
			util.ErrToCode(service.ErrTemplateTypeNotFound, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedRetrieveTemplates, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedRetrieveTemplate, http.StatusInternalServerError),
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
//	@Description	Fetches full details for one template associated with a particular type in a space. The response provides the template’s identifier, name, icon, and any other relevant metadata. This endpoint is useful when a client needs to preview or apply a template to prefill object creation fields.
//	@Id				get_template
//	@Tags			Templates
//	@Produce		json
//	@Param			Anytype-Version	header		string						true	"The version of the API to use"	default(2025-05-20)
//	@Param			space_id		path		string						true	"The ID of the space to which the template belongs; must be retrieved from ListSpaces endpoint"
//	@Param			type_id			path		string						true	"The ID of the type to which the template belongs; must be retrieved from ListTypes endpoint or obtained from response context"
//	@Param			template_id		path		string						true	"The ID of the template to retrieve; must be retrieved from ListTemplates endpoint or obtained from response context"
//	@Success		200				{object}	apimodel.TemplateResponse	"The requested template"
//	@Failure		401				{object}	util.UnauthorizedError		"Unauthorized"
//	@Failure		404				{object}	util.NotFoundError			"Resource not found"
//	@Failure		410				{object}	util.GoneError				"Resource deleted"
//	@Failure		500				{object}	util.ServerError			"Internal server error"
//	@Security		bearerauth
//	@Router			/v1/spaces/{space_id}/types/{type_id}/templates/{template_id} [get]
func GetTemplateHandler(s *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		typeId := c.Param("type_id")
		templateId := c.Param("template_id")

		template, err := s.GetTemplate(c.Request.Context(), spaceId, typeId, templateId)
		code := util.MapErrorCode(err,
			util.ErrToCode(service.ErrTemplateNotFound, http.StatusNotFound),
			util.ErrToCode(service.ErrTemplateDeleted, http.StatusGone),
			util.ErrToCode(service.ErrFailedRetrieveTemplate, http.StatusInternalServerError),
			util.ErrToCode(service.ErrFailedExportMarkdown, http.StatusInternalServerError),
		)

		if code != http.StatusOK {
			apiErr := util.CodeToAPIError(code, err.Error())
			c.JSON(code, apiErr)
			return
		}

		c.JSON(http.StatusOK, apimodel.TemplateResponse{Template: *template})
	}
}
