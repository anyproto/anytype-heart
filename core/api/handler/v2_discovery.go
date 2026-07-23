package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/service"
)

// ListSpacesV2Handler lists spaces as minimal rows
//
//	@Summary	List spaces (minimal rows)
//	@Id			v2_list_spaces
//	@Tags		V2
//	@Produce	json
//	@Success	200	{object}	apimodel.V2ListResponse[apimodel.V2SpaceRow]	"Minimal space rows"
//	@Security	bearerauth
//	@Router		/v2/spaces [get]
func ListSpacesV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, err := s.ListSpaces(c.Request.Context(), offset, limit)
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, apimodel.NewV2ListResponse(rows, total, offset, limit, hasMore,
			"request the next offset"))
	}
}

// ListMembersV2Handler lists space members as minimal rows
//
//	@Summary	List members (minimal rows)
//	@Id			v2_list_members
//	@Tags		V2
//	@Produce	json
//	@Param		space_id	path		string											true	"Space id"
//	@Success	200			{object}	apimodel.V2ListResponse[apimodel.V2MemberRow]	"Minimal member rows"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/members [get]
func ListMembersV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, err := s.ListMembers(c.Request.Context(), c.Param("space_id"), offset, limit)
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, apimodel.NewV2ListResponse(rows, total, offset, limit, hasMore,
			"request the next offset"))
	}
}

// ListTypesV2Handler lists type keys and names
//
//	@Summary	List types (keys + names)
//	@Id			v2_list_types
//	@Tags		V2
//	@Produce	json
//	@Param		space_id	path		string										true	"Space id"
//	@Success	200			{object}	apimodel.V2ListResponse[apimodel.V2TypeRow]	"Type rows"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/types [get]
func ListTypesV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, err := s.ListTypes(c.Request.Context(), c.Param("space_id"), offset, limit)
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, apimodel.NewV2ListResponse(rows, total, offset, limit, hasMore,
			"request the next offset"))
	}
}

// GetTypeV2Handler reads one type as its AnyBlock document
//
//	@Summary	Get type (AnyBlock document)
//	@Id			v2_get_type
//	@Tags		V2
//	@Produce	json
//	@Param		space_id	path		string				true	"Space id"
//	@Param		type		path		string				true	"Type key"
//	@Success	200			{object}	map[string]any		"The kind:objectType AnyBlock document + etag"
//	@Failure	404			{object}	apimodel.V2Error	"Type not found"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/types/{type} [get]
func GetTypeV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, etag, err := s.GetType(c.Request.Context(), c.Param("space_id"), c.Param("type"))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.Header("ETag", service.QuoteEtag(etag))
		c.Data(http.StatusOK, "application/json", body)
	}
}

// GetTypeSchemaV2Handler is the [build] GenerateSchema endpoint stub
//
//	@Summary	Get type schema (not implemented)
//	@Id			v2_get_type_schema
//	@Tags		V2
//	@Produce	json
//	@Param		space_id	path		string				true	"Space id"
//	@Param		type		path		string				true	"Type key"
//	@Failure	501			{object}	apimodel.V2Error	"Not implemented yet"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/types/{type}/schema [get]
func GetTypeSchemaV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		RespondV2Error(c, s.GetTypeSchema(c.Request.Context(), c.Param("space_id"), c.Param("type")))
	}
}

// ListPropertiesV2Handler lists properties as key/name/format rows
//
//	@Summary	List properties (key, name, format)
//	@Id			v2_list_properties
//	@Tags		V2
//	@Produce	json
//	@Param		space_id	path		string												true	"Space id"
//	@Success	200			{object}	apimodel.V2ListResponse[apimodel.V2PropertyRow]		"Property rows"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/properties [get]
func ListPropertiesV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, err := s.ListProperties(c.Request.Context(), c.Param("space_id"), offset, limit)
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, apimodel.NewV2ListResponse(rows, total, offset, limit, hasMore,
			"request the next offset"))
	}
}

// ListPropertyOptionsV2Handler lists option names of one property
//
//	@Summary	List property options (names + colors)
//	@Id			v2_list_property_options
//	@Tags		V2
//	@Produce	json
//	@Param		space_id	path		string											true	"Space id"
//	@Param		key			path		string											true	"Property key"
//	@Param		prefix		query		string											false	"Case-insensitive name prefix filter"
//	@Success	200			{object}	apimodel.V2ListResponse[apimodel.V2OptionRow]	"Option rows"
//	@Failure	404			{object}	apimodel.V2Error								"Property not found"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/properties/{key}/options [get]
func ListPropertyOptionsV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, err := s.ListPropertyOptions(c.Request.Context(), c.Param("space_id"), c.Param("key"), c.Query("prefix"), offset, limit)
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, apimodel.NewV2ListResponse(rows, total, offset, limit, hasMore,
			"narrow with prefix= or request the next offset"))
	}
}
