package v2handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// ListSpacesV2Handler lists spaces as minimal rows
//
//	@Summary		List spaces (minimal rows)
//	@Description	Returns {id, name, description} rows for the account's LIVE spaces — deleted, left and still-joining spaces are filtered out (the same predicate GET /v2/spaces/{space_id} and the global search use).
//	@Id				v2_list_spaces
//	@Tags			Spaces
//	@Produce		json
//	@Param			ids	query		string									false	"compact (default) = the short space reference; full = the full <cid>.<replicationKey> id — the export spelling, and the one to persist outside this API (a short reference is unique only against the spaces you can currently see)"
//	@Success		200	{object}	v2model.ListResponse[v2model.SpaceRow]	"Minimal space rows"
//	@Security		bearerauth
//	@Router			/v2/spaces [get]
func ListSpacesV2Handler(s *v2service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, err := s.ListSpaces(c.Request.Context(), offset, limit)
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, v2model.NewListResponse(rows, total, offset, limit, hasMore,
			"request the next offset"))
	}
}

// ListMembersV2Handler lists space members as minimal rows
//
//	@Summary	List members (minimal rows)
//	@Id			v2_list_members
//	@Tags		Members
//	@Produce	json
//	@Param		space_id	path		string									true	"Space id"
//	@Success	200			{object}	v2model.ListResponse[v2model.MemberRow]	"Minimal member rows"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/members [get]
func ListMembersV2Handler(s *v2service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, err := s.ListMembers(c.Request.Context(), c.Param("space_id"), offset, limit)
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, v2model.NewListResponse(rows, total, offset, limit, hasMore,
			"request the next offset"))
	}
}

// GetMemberMeV2Handler returns the caller's own member row
//
//	@Summary	Get the calling member (server-side identity, §7.3 @me)
//	@Id			v2_get_member_me
//	@Tags		Members
//	@Produce	json
//	@Param		space_id	path		string				true	"Space id"
//	@Success	200			{object}	v2model.MemberRow	"The caller's member row"
//	@Failure	404			{object}	v2model.Error		"Space not found, or no account identity"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/members/me [get]
func GetMemberMeV2Handler(s *v2service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		row, err := s.GetMemberMe(c.Request.Context(), c.Param("space_id"))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, row)
	}
}

// ListTypesV2Handler lists type keys and names
//
//	@Summary	List types (keys + names)
//	@Id			v2_list_types
//	@Tags		Types
//	@Produce	json
//	@Param		space_id	path		string									true	"Space id"
//	@Success	200			{object}	v2model.ListResponse[v2model.TypeRow]	"Type rows"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/types [get]
func ListTypesV2Handler(s *v2service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, err := s.ListTypes(c.Request.Context(), c.Param("space_id"), offset, limit)
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, v2model.NewListResponse(rows, total, offset, limit, hasMore,
			"request the next offset"))
	}
}

// GetTypeV2Handler reads one type as its AnyBlock document
//
//	@Summary	Get type (AnyBlock document)
//	@Id			v2_get_type
//	@Tags		Types
//	@Produce	json
//	@Param		space_id	path		string			true	"Space id"
//	@Param		type		path		string			true	"Type key"
//	@Param		ids			query		string			false	"compact (default) = the edit shape with short labels for minted view/block ids; full = the export shape with full ids"
//	@Success	200			{object}	map[string]any	"The kind:objectType AnyBlock document + etag"
//	@Failure	404			{object}	v2model.Error	"Type not found"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/types/{type} [get]
func GetTypeV2Handler(s *v2service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, etag, err := s.GetType(c.Request.Context(), c.Param("space_id"), c.Param("type"),
			v2service.V2ObjectQuery{Ids: c.Query("ids")})
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.Header("ETag", v2service.QuoteEtag(etag))
		c.Data(http.StatusOK, "application/json", body)
	}
}

// GetTypeSchemaV2Handler is the [build] GenerateSchema endpoint stub
//
//	@Summary	Get type schema (not implemented)
//	@Id			v2_get_type_schema
//	@Tags		Types
//	@Produce	json
//	@Param		space_id	path		string			true	"Space id"
//	@Param		type		path		string			true	"Type key"
//	@Failure	501			{object}	v2model.Error	"Not implemented yet"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/types/{type}/schema [get]
func GetTypeSchemaV2Handler(s *v2service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		RespondV2Error(c, s.GetTypeSchema(c.Request.Context(), c.Param("space_id"), c.Param("type")))
	}
}

// ListPropertiesV2Handler lists properties as key/name/format rows
//
//	@Summary	List properties (key, name, format)
//	@Id			v2_list_properties
//	@Tags		Properties
//	@Produce	json
//	@Param		space_id	path		string										true	"Space id"
//	@Success	200			{object}	v2model.ListResponse[v2model.PropertyRow]	"Property rows"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/properties [get]
func ListPropertiesV2Handler(s *v2service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, err := s.ListProperties(c.Request.Context(), c.Param("space_id"), offset, limit)
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, v2model.NewListResponse(rows, total, offset, limit, hasMore,
			"request the next offset"))
	}
}

// ListPropertyOptionsV2Handler lists option names of one property
//
//	@Summary	List property options (names + colors)
//	@Id			v2_list_property_options
//	@Tags		Properties
//	@Produce	json
//	@Param		space_id	path		string									true	"Space id"
//	@Param		key			path		string									true	"Property key"
//	@Param		prefix		query		string									false	"Case-insensitive name prefix filter"
//	@Success	200			{object}	v2model.ListResponse[v2model.OptionRow]	"Option rows"
//	@Failure	404			{object}	v2model.Error							"Property not found"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/properties/{key}/options [get]
func ListPropertyOptionsV2Handler(s *v2service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, err := s.ListPropertyOptions(c.Request.Context(), c.Param("space_id"), c.Param("key"), c.Query("prefix"), offset, limit)
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, v2model.NewListResponse(rows, total, offset, limit, hasMore,
			"narrow with prefix= or request the next offset"))
	}
}
