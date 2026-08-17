package v2handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// GetObjectHandler reads one object as a flat AnyBlock document
//
//	@Summary		Read an object as an AnyBlock document
//	@Description	A `block` subtree comes back flagged as a subtree, and no write path accepts that partial body. `format=md` is read-only; markdown cannot be sent back.
//	@Id				get_object
//	@Tags			Objects
//	@Produce		json
//	@Param			space_id	path		string			true	"Space id"
//	@Param			object_id	path		string			true	"Object id"
//	@Param			include		query		string			false	"Subset of properties,blocks (default both)"
//	@Param			outline		query		bool			false	"Return the block skeleton instead of full blocks"
//	@Param			block		query		string			false	"Return only this block's subtree"
//	@Param			ids			query		string			false	"compact (default) is the edit shape, where minted block ids relabel to short suffixes; full is the export shape, with full ids everywhere, and the shape to send back. Object references are full and inline in both."
//	@Param			format		query		string			false	"anyblock (default) or md"
//	@Success		200			{object}	map[string]any	"The flat AnyBlock document + etag"
//	@Failure		400			{object}	v2model.Error	"Illegal parameter combination (ambiguous_input)"
//	@Failure		404			{object}	v2model.Error	"Object or space not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/objects/{object_id} [get]
func GetObjectHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		q := v2service.ObjectQuery{
			Include: c.Query("include"),
			Outline: c.Query("outline") == "true",
			Block:   c.Query("block"),
			Ids:     c.Query("ids"),
			Format:  c.Query("format"),
		}
		body, etag, err := s.GetObject(c.Request.Context(), c.Param("space_id"), c.Param("object_id"), q)
		if err != nil {
			RespondError(c, err)
			return
		}
		c.Header("ETag", v2service.QuoteEtag(etag))
		c.Data(http.StatusOK, "application/json", body)
	}
}

// ListObjectsHandler lists objects as minimal rows
//
//	@Summary	List the objects in a space
//	@Id			list_objects
//	@Tags		Objects
//	@Produce	json
//	@Param		space_id	path		string									true	"Space id"
//	@Param		fields		query		string									false	"Comma-separated property keys to include per row"
//	@Param		offset		query		int										false	"Items to skip"		default(0)
//	@Param		limit		query		int										false	"Items to return"	default(25)
//	@Success	200			{object}	v2model.ListResponse[v2model.ObjectRow]	"Minimal object rows"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/objects [get]
func ListObjectsHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)

		var fields []string
		if raw := c.Query("fields"); raw != "" {
			for _, f := range strings.Split(raw, ",") {
				if f = strings.TrimSpace(f); f != "" {
					fields = append(fields, f)
				}
			}
		}

		rows, total, hasMore, err := s.ListObjects(c.Request.Context(), c.Param("space_id"), fields, offset, limit)
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, v2model.NewListResponse(rows, total, offset, limit, hasMore,
			"narrow with search filters or request the next offset"))
	}
}
