package apimodel

// v2.go holds the API v2 DTOs: the C6 error contract, the C10 list
// envelope, and the Phase-1 read shapes (APIV2.md).

import (
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
