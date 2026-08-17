package v2handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// ListSpacesHandler lists spaces as minimal rows
//
//	@Summary		List spaces (minimal rows)
//	@Description	Returns {id, name, description} rows for the account's LIVE spaces — deleted, left and still-joining spaces are filtered out (the same predicate GET /v2/spaces/{space_id} and the global search use).
//	@Id				list_spaces
//	@Tags			Spaces
//	@Produce		json
//	@Param			ids	query		string									false	"compact (default) = the short space reference; full = the full <cid>.<replicationKey> id — the export spelling, and the one to persist outside this API (a short reference is unique only against the spaces you can currently see)"
//	@Success		200	{object}	v2model.ListResponse[v2model.SpaceRow]	"Minimal space rows"
//	@Security		bearerauth
//	@Router			/v2/spaces [get]
func ListSpacesHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, err := s.ListSpaces(c.Request.Context(), offset, limit)
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, v2model.NewListResponse(rows, total, offset, limit, hasMore,
			"request the next offset"))
	}
}

// ListMembersHandler lists space members as minimal rows
//
//	@Summary	List members (minimal rows)
//	@Id			list_members
//	@Tags		Members
//	@Produce	json
//	@Param		space_id	path		string									true	"Space id"
//	@Success	200			{object}	v2model.ListResponse[v2model.MemberRow]	"Minimal member rows"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/members [get]
func ListMembersHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, err := s.ListMembers(c.Request.Context(), c.Param("space_id"), offset, limit)
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, v2model.NewListResponse(rows, total, offset, limit, hasMore,
			"request the next offset"))
	}
}

// GetMemberMeHandler returns the caller's own member row
//
//	@Summary	Get the calling member (server-side identity, §7.3 @me)
//	@Id			get_member_me
//	@Tags		Members
//	@Produce	json
//	@Param		space_id	path		string				true	"Space id"
//	@Success	200			{object}	v2model.MemberRow	"The caller's member row"
//	@Failure	404			{object}	v2model.Error		"Space not found, or no account identity"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/members/me [get]
func GetMemberMeHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		row, err := s.GetMemberMe(c.Request.Context(), c.Param("space_id"))
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, row)
	}
}

// ListTypesHandler lists type keys and names
//
//	@Summary	List types (keys + names)
//	@Id			list_types
//	@Tags		Types
//	@Produce	json
//	@Param		space_id	path		string									true	"Space id"
//	@Success	200			{object}	v2model.ListResponse[v2model.TypeRow]	"Type rows"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/types [get]
func ListTypesHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, err := s.ListTypes(c.Request.Context(), c.Param("space_id"), offset, limit)
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, v2model.NewListResponse(rows, total, offset, limit, hasMore,
			"request the next offset"))
	}
}

// GetTypeHandler reads one type as its AnyBlock document
//
//	@Summary	Get type (AnyBlock document)
//	@Id			get_type
//	@Tags		Types
//	@Produce	json
//	@Param		space_id	path		string			true	"Space id"
//	@Param		type		path		string			true	"Type key"
//	@Param		ids			query		string			false	"compact (default) = the edit shape with short labels for minted view/block ids; full = the export shape with full ids"
//	@Success	200			{object}	map[string]any	"The kind:objectType AnyBlock document + etag"
//	@Failure	404			{object}	v2model.Error	"Type not found"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/types/{type} [get]
func GetTypeHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, etag, err := s.GetType(c.Request.Context(), c.Param("space_id"), c.Param("type"),
			v2service.ObjectQuery{Ids: c.Query("ids")})
		if err != nil {
			RespondError(c, err)
			return
		}
		c.Header("ETag", v2service.QuoteEtag(etag))
		c.Data(http.StatusOK, "application/json", body)
	}
}

// GetTypeSchemaHandler is the [build] GenerateSchema endpoint stub
//
//	@Summary	Get type schema (not implemented)
//	@Id			get_type_schema
//	@Tags		Types
//	@Produce	json
//	@Param		space_id	path		string			true	"Space id"
//	@Param		type		path		string			true	"Type key"
//	@Failure	501			{object}	v2model.Error	"Not implemented yet"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/types/{type}/schema [get]
func GetTypeSchemaHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		RespondError(c, s.GetTypeSchema(c.Request.Context(), c.Param("space_id"), c.Param("type")))
	}
}

// ListPropertiesHandler lists properties as key/name/format rows
//
//	@Summary	List properties (key, name, format)
//	@Id			list_properties
//	@Tags		Properties
//	@Produce	json
//	@Param		space_id	path		string										true	"Space id"
//	@Success	200			{object}	v2model.ListResponse[v2model.PropertyRow]	"Property rows"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/properties [get]
func ListPropertiesHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, err := s.ListProperties(c.Request.Context(), c.Param("space_id"), offset, limit)
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, v2model.NewListResponse(rows, total, offset, limit, hasMore,
			"request the next offset"))
	}
}

// ListPropertyOptionsHandler lists option names of one property
//
//	@Summary	List property options (names + colors)
//	@Id			list_property_options
//	@Tags		Properties
//	@Produce	json
//	@Param		space_id	path		string									true	"Space id"
//	@Param		key			path		string									true	"Property key"
//	@Param		prefix		query		string									false	"Case-insensitive name prefix filter"
//	@Success	200			{object}	v2model.ListResponse[v2model.OptionRow]	"Option rows"
//	@Failure	404			{object}	v2model.Error							"Property not found"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/properties/{key}/options [get]
func ListPropertyOptionsHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, err := s.ListPropertyOptions(c.Request.Context(), c.Param("space_id"), c.Param("key"), c.Query("prefix"), offset, limit)
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, v2model.NewListResponse(rows, total, offset, limit, hasMore,
			"narrow with prefix= or request the next offset"))
	}
}
