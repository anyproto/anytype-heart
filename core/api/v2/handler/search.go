package v2handler

// search.go — the Phase-4 search handlers (APIV2.md §2 Phase 4). Search
// is a READ carried by POST only because the request needs a body: the
// routes attach no idempotency middleware, and a supplied ?dry_run=true is
// ignored (a read is its own dry run). Pagination is the C10 query params —
// a body `limit` is rejected by the strict request schema with a steering
// hint.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// maxSearchRequestBody caps the search request body. The search routes carry
// no idempotency middleware (and with it no body-size guard), so without a
// cap here io.ReadAll is attacker-sized — and the body feeds the recursive
// filter parser. A legitimate search body (filter ≤ 4096 bytes, bounded
// sorts/fields) is orders of magnitude smaller.
const maxSearchRequestBody = 1 << 20 // 1 MiB

// decodeSearchRequest decodes the search body strictly (C13): unknown
// fields are rejected, with C10 steering when the field is a pagination
// param that belongs in the query string.
func decodeSearchRequest(c *gin.Context) (v2model.SearchRequest, bool) {
	var req v2model.SearchRequest
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxSearchRequestBody+1))
	if err != nil {
		RespondError(c, v2model.ValidationFailed("read request body",
			v2model.Issue{Message: err.Error()}))
		return req, false
	}
	if len(body) > maxSearchRequestBody {
		RespondError(c, v2model.RequestTooLarge(
			fmt.Sprintf("search request body exceeds the %d-byte limit", maxSearchRequestBody)))
		return req, false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return req, true // an empty body is a match-everything search
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		issue := v2model.Issue{Message: err.Error(), Hint: "the search body takes query, type, filter, filters, sorts, fields"}
		if field, ok := unknownFieldName(err); ok {
			issue.Path = "/" + field
			if field == "limit" || field == "offset" {
				issue.Hint = fmt.Sprintf("pagination is the ?offset=&limit= query params (C10), not a body field — e.g. POST …/search?%s=25", field)
			}
		}
		RespondError(c, v2model.ValidationFailed("invalid search request", issue))
		return req, false
	}
	return req, true
}

// unknownFieldName extracts the field name from encoding/json's
// unknown-field error text (the decoder exposes no typed error for it).
func unknownFieldName(err error) (string, bool) {
	const marker = `unknown field "`
	msg := err.Error()
	idx := strings.Index(msg, marker)
	if idx < 0 {
		return "", false
	}
	rest := msg[idx+len(marker):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

// SearchObjectsHandler searches one space
//
//	@Summary		Search one space
//	@Description	`filter` and `filters` are two spellings of the same thing, the compact string and the structured array; sending both is refused. This is a read carried by POST because the query needs a body, so pagination stays in the query string and a `limit` or `offset` in the body is refused.
//	@Id				search_space
//	@Tags			Search
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string									true	"Space id"
//	@Param			request		body		v2model.SearchRequestDoc				true	"Search request"
//	@Param			offset		query		int										false	"Items to skip"		default(0)
//	@Param			limit		query		int										false	"Items to return"	default(25)
//	@Success		200			{object}	v2model.ListResponse[v2model.ObjectRow]	"Minimal object rows"
//	@Failure		400			{object}	v2model.Error							"Invalid request (validation_failed / ambiguous_input)"
//	@Failure		404			{object}	v2model.Error							"Space not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/search [post]
func SearchObjectsHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		req, ok := decodeSearchRequest(c)
		if !ok {
			return
		}
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, warnings, err := s.SearchObjects(c.Request.Context(), c.Param("space_id"), req, offset, limit)
		if err != nil {
			RespondError(c, err)
			return
		}
		resp := v2model.NewListResponse(rows, total, offset, limit, hasMore, v2service.SearchNarrowHint)
		resp.Warnings = warnings
		c.JSON(http.StatusOK, resp)
	}
}

// GlobalSearchObjectsHandler searches every space
//
//	@Summary		Search every space
//	@Description	Type keys and option names are resolved per space. A name that resolves in only some spaces searches those and warns about the rest. `total` is the sum of the per-space counts, and each row carries its `space_id`.
//	@Id				search_global
//	@Tags			Search
//	@Accept			json
//	@Produce		json
//	@Param			request	body		v2model.SearchRequestDoc				true	"Search request"
//	@Param			offset	query		int										false	"Items to skip"		default(0)
//	@Param			limit	query		int										false	"Items to return"	default(25)
//	@Param			ids		query		string									false	"How each row's space_id is spelled: compact (default) is the short space reference; full is the whole <cid>.<replicationKey> id, and the spelling to store outside this API"
//	@Success		200		{object}	v2model.ListResponse[v2model.ObjectRow]	"Minimal object rows with space_id"
//	@Failure		400		{object}	v2model.Error							"Invalid request"
//	@Security		bearerauth
//	@Router			/v2/search [post]
func GlobalSearchObjectsHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		req, ok := decodeSearchRequest(c)
		if !ok {
			return
		}
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, warnings, err := s.GlobalSearchObjects(c.Request.Context(), req, offset, limit)
		if err != nil {
			RespondError(c, err)
			return
		}
		resp := v2model.NewListResponse(rows, total, offset, limit, hasMore, v2service.SearchNarrowHint)
		resp.Warnings = warnings
		c.JSON(http.StatusOK, resp)
	}
}
