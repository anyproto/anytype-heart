package apimodel

// v2.go holds the API v2 DTOs: the C6 error contract, the C10 list
// envelope, and the Phase-1 read shapes (APIV2.md).

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// V2 error codes (APIV2.md C6). Error text is API surface; tests assert it.
const (
	V2CodeValidationFailed    = "validation_failed"
	V2CodeVersionUnsupported  = "version_unsupported"
	V2CodeIdempotencyConflict = "idempotency_conflict"
	V2CodeEtagMismatch        = "etag_mismatch"
	V2CodeAmbiguousInput      = "ambiguous_input"
	V2CodeNotFound            = "not_found"
	V2CodeNotImplemented      = "not_implemented"
	V2CodeInternalError       = "internal_error"
	V2CodeRequestTooLarge     = "request_too_large"
)

// V2Issue is one path-addressed problem (C6): path into the request
// document or the query-parameter name, a message naming allowed values,
// and an optional repair hint.
type V2Issue struct {
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// V2Error is the C6 error envelope, returned by every v2 endpoint.
type V2Error struct {
	Status  int       `json:"status"`
	Code    string    `json:"code"`
	Message string    `json:"message"`
	Issues  []V2Issue `json:"issues,omitempty"`
}

func (e *V2Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NewV2Error builds a C6 error.
func NewV2Error(status int, code, message string, issues ...V2Issue) *V2Error {
	return &V2Error{Status: status, Code: code, Message: message, Issues: issues}
}

// V2AmbiguousInput is the 400 for illegal parameter combinations (C6),
// naming the conflicting params.
func V2AmbiguousInput(message string, issues ...V2Issue) *V2Error {
	return NewV2Error(http.StatusBadRequest, V2CodeAmbiguousInput, message, issues...)
}

// V2ValidationFailed is the 400 for malformed parameters or bodies.
func V2ValidationFailed(message string, issues ...V2Issue) *V2Error {
	return NewV2Error(http.StatusBadRequest, V2CodeValidationFailed, message, issues...)
}

// V2NotFound is the 404 for missing resources.
func V2NotFound(message string) *V2Error {
	return NewV2Error(http.StatusNotFound, V2CodeNotFound, message)
}

// V2RequestTooLarge is the 413 for an oversized request body (C3).
func V2RequestTooLarge(message string) *V2Error {
	return NewV2Error(http.StatusRequestEntityTooLarge, V2CodeRequestTooLarge, message)
}

// V2EtagMismatch is the 409 for a stale If-Match (C7), carrying the current
// etag so the agent can re-read and retry.
func V2EtagMismatch(currentEtag string) *V2Error {
	return NewV2Error(http.StatusConflict, V2CodeEtagMismatch,
		fmt.Sprintf("the object changed since the etag in If-Match was read — current etag is %q; re-read the object and retry", currentEtag))
}

// V2VersionUnsupported is the 400 for a document produced by a newer format
// version (C6: surfaces SPEC §10's wording, naming both versions).
func V2VersionUnsupported(documentVersion, supportedVersion int) *V2Error {
	return NewV2Error(http.StatusBadRequest, V2CodeVersionUnsupported,
		fmt.Sprintf("the document was produced by a newer version of the AnyBlock format: document version %d is newer than the supported version %d", documentVersion, supportedVersion))
}

// V2ListResponse is the C10 paginated list envelope: default limit 25,
// has_more, and a steering message when the result is truncated.
type V2ListResponse[T any] struct {
	Data    []T    `json:"data"`
	Total   int    `json:"total"`
	Offset  int    `json:"offset"`
	Limit   int    `json:"limit"`
	HasMore bool   `json:"has_more"`
	Message string `json:"message,omitempty"`
}

// NewV2ListResponse assembles the envelope and, when truncated, the C10
// steering message.
func NewV2ListResponse[T any](data []T, total, offset, limit int, hasMore bool, narrowHint string) V2ListResponse[T] {
	if data == nil {
		data = []T{}
	}
	resp := V2ListResponse[T]{Data: data, Total: total, Offset: offset, Limit: limit, HasMore: hasMore}
	if hasMore {
		resp.Message = fmt.Sprintf("%d matches — showing %d from offset %d; %s", total, len(data), offset, narrowHint)
	}
	return resp
}

// V2ObjectRow is the C5 minimal list row: id, name, type (a type key) plus
// requested property values.
type V2ObjectRow struct {
	Id         string         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
}

// V2SpaceRow is a minimal space list row.
type V2SpaceRow struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

// V2MemberRow is a minimal member list row; agents need member ids for
// assignee/creator property values.
type V2MemberRow struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Identity string `json:"identity,omitempty"`
}

// V2TypeRow is a minimal type list row: keys + names (Phase 1).
type V2TypeRow struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// V2PropertyRow is a minimal property list row: key, name, format (Phase 1).
type V2PropertyRow struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Format string `json:"format"`
}

// V2OptionRow is a select/multiSelect option list row: names are the
// option vocabulary (C2), color optional.
type V2OptionRow struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// V2ValidateResponse is the POST /v2/validate result: structural +
// format-semantic issues only (referential validation is Phase 2).
type V2ValidateResponse struct {
	Issues   []V2Issue `json:"issues"`
	Warnings []V2Issue `json:"warnings"`
}

// V2OutlineEntry is one row of the outline shape: every block's
// {indent, id, type}, text only on heading blocks (APIV2.md Phase 1).
type V2OutlineEntry struct {
	Indent int    `json:"indent"`
	Id     string `json:"id"`
	Type   string `json:"type"`
	Text   string `json:"text,omitempty"`
}

//
// ---- Phase 2: create surface ----
//

// V2CreateResult is the response of every v2 create/update endpoint. C8:
// created ids are always returned. On a dry run (C9) nothing is committed:
// Id/Etag stay empty, DryRun is true, and Issues/Created report the would-be
// outcome.
type V2CreateResult struct {
	Id       string         `json:"id,omitempty"`
	Type     string         `json:"type,omitempty"` // type key of the created object (C2)
	Key      string         `json:"key,omitempty"`  // identity key (types, properties)
	Etag     string         `json:"etag,omitempty"` // C7 etag of the created object
	DryRun   bool           `json:"dry_run,omitempty"`
	Created  *V2SideEffects `json:"created,omitempty"`
	Issues   []V2Issue      `json:"issues,omitempty"`
	Warnings []V2Issue      `json:"warnings,omitempty"`
}

// V2SideEffects lists the schema entities a create materialized on the way
// (create-missing, SPEC §3/§2a) — or would materialize, on a dry run.
type V2SideEffects struct {
	Properties []V2PropertyRow   `json:"properties,omitempty"`
	Options    []V2CreatedOption `json:"options,omitempty"`
}

// V2CreatedOption names one select/multiSelect option created by
// create-missing resolution (SPEC §3: option names, never ids).
type V2CreatedOption struct {
	Property string `json:"property"` // property key
	Name     string `json:"name"`
}

// V2CreatePropertyRequest is the POST properties body:
// {key?, name, format, options?:[{name,color?}]} (APIV2.md Phase 2).
type V2CreatePropertyRequest struct {
	Key     string                  `json:"key,omitempty"`
	Name    string                  `json:"name"`
	Format  string                  `json:"format"`
	Options []V2CreateOptionRequest `json:"options,omitempty"`
}

// V2CreateOptionRequest is one select option in a property create.
type V2CreateOptionRequest struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// V2UpdatePropertyRequest is the PATCH properties/{key} body.
type V2UpdatePropertyRequest struct {
	Name *string `json:"name,omitempty"`
}

// V2CreateSetRequest is the POST sets body. filter (compact string) is
// reserved for the Phase-4 parser; filters/sorts follow the AnyBlock §6.2
// shapes and are passed into the set's initial dataview verbatim. views, when
// given, replaces the default single view and is mutually exclusive with
// top-level filters/sorts (ambiguous_input otherwise).
type V2CreateSetRequest struct {
	Name    string          `json:"name"`
	Type    string          `json:"type"`
	Filter  string          `json:"filter,omitempty"`
	Filters json.RawMessage `json:"filters,omitempty"`
	Sorts   json.RawMessage `json:"sorts,omitempty"`
	Views   json.RawMessage `json:"views,omitempty"`
}

// V2CreateCollectionRequest is the POST collections body.
type V2CreateCollectionRequest struct {
	Name  string   `json:"name"`
	Items []string `json:"items,omitempty"`
}

// V2UploadFileRequest is the JSON form of POST files (URL upload); the
// multipart form is the byte-upload alternative.
type V2UploadFileRequest struct {
	Url  string `json:"url"`
	Name string `json:"name,omitempty"`
}

// V2FileUploadResult is the POST files response: the file object id that
// file/image blocks and iconImage values need (R11).
type V2FileUploadResult struct {
	Id       string `json:"id"`
	Name     string `json:"name,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Size     int64  `json:"size,omitempty"`
	DryRun   bool   `json:"dry_run,omitempty"`
}

//
// ---- Phase 3: edit surface ----
//

// V2DiffStats summarizes what a mutation changed (APIV2.md Phase 3): the
// accidental-full-rewrite signal on PUT, the receipt on PATCH.
type V2DiffStats struct {
	BlocksAdded       int `json:"blocksAdded"`
	BlocksRemoved     int `json:"blocksRemoved"`
	BlocksChanged     int `json:"blocksChanged"`
	BlocksMoved       int `json:"blocksMoved"`
	PropertiesChanged int `json:"propertiesChanged"`
}

// V2EditResult is the PATCH/PUT response: the new etag, the created-block id
// map keyed by payload position (client-supplied ids echoed), the schema
// side effects (created select options, like Phase 2's create), and the
// diff stats. On a dry run nothing is committed: Etag stays empty and DryRun
// is true; CreatedBlocks/Created/DiffStats report the would-be outcome.
type V2EditResult struct {
	Etag          string            `json:"etag,omitempty"`
	DryRun        bool              `json:"dry_run,omitempty"`
	CreatedBlocks map[string]string `json:"createdBlocks,omitempty"`
	Created       *V2SideEffects    `json:"created,omitempty"`
	DiffStats     V2DiffStats       `json:"diffStats"`
	Warnings      []V2Issue         `json:"warnings,omitempty"`
}

// V2SchemaEntry is one GET /v2/schemas/{kind} payload: the strict-mode
// generation schema (C13) plus one worked example (C12).
type V2SchemaEntry struct {
	Kind     string          `json:"kind"`
	Endpoint string          `json:"endpoint"`
	Schema   json.RawMessage `json:"schema"`
	Example  json.RawMessage `json:"example"`
}

// V2SchemaIndex is the GET /v2/schemas payload. Ops lists the Phase-3 PATCH
// ops (per-op schemas at /v2/schemas/ops/{op}).
type V2SchemaIndex struct {
	Kinds []V2SchemaIndexEntry `json:"kinds"`
	Ops   []V2SchemaIndexEntry `json:"ops,omitempty"`
}

// V2SchemaIndexEntry is one row of the schema index.
type V2SchemaIndexEntry struct {
	Kind     string `json:"kind"`
	Endpoint string `json:"endpoint"`
	Url      string `json:"url"`
}
