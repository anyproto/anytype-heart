package handler

// v2_search.go — the Phase-4 search handlers (APIV2.md §2 Phase 4). Search
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

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/service"
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
func decodeSearchRequest(c *gin.Context) (apimodel.V2SearchRequest, bool) {
	var req apimodel.V2SearchRequest
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxSearchRequestBody+1))
	if err != nil {
		RespondV2Error(c, apimodel.V2ValidationFailed("read request body",
			apimodel.V2Issue{Message: err.Error()}))
		return req, false
	}
	if len(body) > maxSearchRequestBody {
		RespondV2Error(c, apimodel.V2RequestTooLarge(
			fmt.Sprintf("search request body exceeds the %d-byte limit", maxSearchRequestBody)))
		return req, false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return req, true // an empty body is a match-everything search
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		issue := apimodel.V2Issue{Message: err.Error(), Hint: "the search body takes query, type, filter, filters, sorts, fields"}
		if field, ok := unknownFieldName(err); ok {
			issue.Path = "/" + field
			if field == "limit" || field == "offset" {
				issue.Hint = fmt.Sprintf("pagination is the ?offset=&limit= query params (C10), not a body field — e.g. POST …/search?%s=25", field)
			}
		}
		RespondV2Error(c, apimodel.V2ValidationFailed("invalid search request", issue))
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

// SearchObjectsV2Handler searches one space
//
//	@Summary		Search objects (space)
//	@Description	Searches one space with full-text (query), a type scope, and either the compact filter string (filter) or the structured filter array (filters) — mutually exclusive, both landing on one internal tree. Sorts accept any property key. Rows are C5 minimal (id, name, type + requested fields). Search is a read: Idempotency-Key is not honored and dry_run is ignored. Pagination via ?offset=&limit= (C10).
//	@Id				v2_search_space
//	@Tags			V2
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string											true	"Space id"
//	@Param			request		body		apimodel.V2SearchRequest						true	"Search request"
//	@Param			offset		query		int												false	"Items to skip"		default(0)
//	@Param			limit		query		int												false	"Items to return"	default(25)
//	@Success		200			{object}	apimodel.V2ListResponse[apimodel.V2ObjectRow]	"Minimal object rows"
//	@Failure		400			{object}	apimodel.V2Error								"Invalid request (validation_failed / ambiguous_input)"
//	@Failure		404			{object}	apimodel.V2Error								"Space not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/search [post]
func SearchObjectsV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		req, ok := decodeSearchRequest(c)
		if !ok {
			return
		}
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, warnings, err := s.SearchObjects(c.Request.Context(), c.Param("space_id"), req, offset, limit)
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		resp := apimodel.NewV2ListResponse(rows, total, offset, limit, hasMore, service.V2SearchNarrowHint)
		resp.Warnings = warnings
		c.JSON(http.StatusOK, resp)
	}
}

// GlobalSearchObjectsV2Handler searches every space
//
//	@Summary		Search objects (global)
//	@Description	Searches all spaces: type keys and option names resolve per space (a reference that resolves in only some spaces queries those and warns about the rest), results merge by the requested sort, total is the sum of per-space store counts (honest totals). Rows carry spaceId. Same request shape as the space search.
//	@Id				v2_search_global
//	@Tags			V2
//	@Accept			json
//	@Produce		json
//	@Param			request	body		apimodel.V2SearchRequest						true	"Search request"
//	@Param			offset	query		int												false	"Items to skip"		default(0)
//	@Param			limit	query		int												false	"Items to return"	default(25)
//	@Success		200		{object}	apimodel.V2ListResponse[apimodel.V2ObjectRow]	"Minimal object rows with spaceId"
//	@Failure		400		{object}	apimodel.V2Error								"Invalid request"
//	@Security		bearerauth
//	@Router			/v2/search [post]
func GlobalSearchObjectsV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		req, ok := decodeSearchRequest(c)
		if !ok {
			return
		}
		offset := c.GetInt(pagination.QueryParamOffset)
		limit := c.GetInt(pagination.QueryParamLimit)
		rows, total, hasMore, warnings, err := s.GlobalSearchObjects(c.Request.Context(), req, offset, limit)
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		resp := apimodel.NewV2ListResponse(rows, total, offset, limit, hasMore, service.V2SearchNarrowHint)
		resp.Warnings = warnings
		c.JSON(http.StatusOK, resp)
	}
}
