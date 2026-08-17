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
//	@Summary		List the account's spaces
//	@Description	Only live spaces are listed. A space that is deleted, left, or still joining does not appear.
//	@Id				list_spaces
//	@Tags			Spaces
//	@Produce		json
//	@Param			ids	query		string									false	"compact (default) is the short space reference; full is the whole <cid>.<replicationKey> id, and the spelling to store outside this API"
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
//	@Summary	List the members of a space
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
//	@Summary		Get the calling member
//	@Description	The identity is taken from the account this API runs against; there is no member id to send.
//	@Id				get_member_me
//	@Tags			Members
//	@Produce		json
//	@Param			space_id	path		string				true	"Space id"
//	@Success		200			{object}	v2model.MemberRow	"The caller's member row"
//	@Failure		404			{object}	v2model.Error		"Space not found, or no account identity"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/members/me [get]
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
//	@Summary	List the types in a space
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
//	@Summary	Read a type as an AnyBlock document
//	@Id			get_type
//	@Tags		Types
//	@Produce	json
//	@Param		space_id	path		string			true	"Space id"
//	@Param		type		path		string			true	"Type key"
//	@Param		ids			query		string			false	"compact (default) is the edit shape, with short labels for minted view and block ids; full is the export shape, with full ids"
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
//	@Summary		Get a JSON Schema for a type
//	@Description	Not implemented. Every request answers 501.
//	@Id				get_type_schema
//	@Tags			Types
//	@Produce		json
//	@Param			space_id	path		string			true	"Space id"
//	@Param			type		path		string			true	"Type key"
//	@Failure		501			{object}	v2model.Error	"Not implemented yet"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/types/{type}/schema [get]
func GetTypeSchemaHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		RespondError(c, s.GetTypeSchema(c.Request.Context(), c.Param("space_id"), c.Param("type")))
	}
}

// ListPropertiesHandler lists properties as key/name/format rows
//
//	@Summary	List the properties in a space
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
//	@Summary	List a property's options
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
