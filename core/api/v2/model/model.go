package v2model

// model.go holds the API v2 DTOs: the C6 error contract, the C10 list
// envelope, and the read shapes.

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// V2 error codes (APIV2.md C6). Error text is API surface; tests assert it.
const (
	CodeValidationFailed    = "validation_failed"
	CodeVersionUnsupported  = "version_unsupported"
	CodeIdempotencyConflict = "idempotency_conflict"
	CodeEtagMismatch        = "etag_mismatch"
	CodeAmbiguousInput      = "ambiguous_input"
	CodeNotFound            = "not_found"
	CodeForbidden           = "forbidden"
	CodeNotImplemented      = "not_implemented"
	CodeInternalError       = "internal_error"
	CodeRequestTooLarge     = "request_too_large"
	// CodeTooManyStreams is the 429 for a CONCURRENCY cap: too many
	// long-lived connections held at once. Deliberately not the shared
	// limiter's "rate_limit_exceeded" — that one means "back off and retry
	// the same request", this one means "retrying can never succeed, close
	// something" — and a client that cannot tell them apart retries forever.
	CodeTooManyStreams = "too_many_streams"
	// The space-grant codes. Messages NAME the actual grant: error-guided
	// self-correction is the v2 design language, and enumeration resistance
	// is a non-goal for a localhost single-user API.
	CodeSpaceNotGranted             = "space_not_granted"
	CodeWriteNotGranted             = "write_not_granted"
	CodeV1NotAvailableForScopedKeys = "v1_not_available_for_scoped_keys"
	// Object deletion is limited to objects the calling API key created, and the
	// message names what IS recorded so the repair is discoverable.
	CodeNotCreatedByThisKey = "not_created_by_this_key"
)

// Issue is one path-addressed problem (C6): path into the request
// document or the query-parameter name, a message naming allowed values,
// and an optional repair hint.
type Issue struct {
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// Error is the C6 error envelope, returned by every v2 endpoint.
//
// Issues carries no omitempty and NewError never leaves it nil, so `issues`
// is present on every v2 error — empty when the refusal has no path to
// address. A field whose PRESENCE is conditional is a branch, and the
// documented consumer form, err.issues.map(...), is the one that throws on
// the absent case (§8.53). The shared authentication, key-scope and
// rate-limit refusals are a different envelope entirely (§8.9) and are
// unaffected.
type Error struct {
	Status  int     `json:"status"`
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Issues  []Issue `json:"issues"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// MarshalJSON keeps `issues` an ARRAY even on a value this package did not
// build. NewError materializes the empty slice, but Issues is exported and
// Error is a plain struct, so one `&Error{Status: …}` literal in a future
// handler would emit `issues: null` — the same crash as the absence this
// replaced, wearing a different hat. The invariant belongs to the type, the
// way ListResponse.Data already does it (pagination.PaginatedResponse).
func (e Error) MarshalJSON() ([]byte, error) {
	type alias Error
	if e.Issues == nil {
		e.Issues = []Issue{}
	}
	return json.Marshal(alias(e))
}

// NewError builds a C6 error. It materializes the empty slice so a value
// read back in Go is never nil either; MarshalJSON is what guarantees the
// wire shape.
func NewError(status int, code, message string, issues ...Issue) *Error {
	if issues == nil {
		issues = []Issue{}
	}
	return &Error{Status: status, Code: code, Message: message, Issues: issues}
}

// AmbiguousInput is the 400 for illegal parameter combinations (C6),
// naming the conflicting params.
func AmbiguousInput(message string, issues ...Issue) *Error {
	return NewError(http.StatusBadRequest, CodeAmbiguousInput, message, issues...)
}

// ValidationFailed is the 400 for malformed parameters or bodies.
func ValidationFailed(message string, issues ...Issue) *Error {
	return NewError(http.StatusBadRequest, CodeValidationFailed, message, issues...)
}

// NotFound is the 404 for missing resources. The issues ARGUMENT is optional
// (the array itself is always on the wire, §8.53) — a 404 that has a repair
// loop to describe (which read actually lists the thing the caller could not
// find) carries it C6-shaped rather than in prose.
func NotFound(message string, issues ...Issue) *Error {
	return NewError(http.StatusNotFound, CodeNotFound, message, issues...)
}

// SpaceNotGranted is the 403 for a request outside the key's space grant —
// a space the grant does not cover, or a no-space route scoped keys cannot
// use. The message must name the actual grant.
func SpaceNotGranted(message string) *Error {
	return NewError(http.StatusForbidden, CodeSpaceNotGranted, message)
}

// WriteNotGranted is the 403 for a write-classified route reached with a
// read-only grant.
func WriteNotGranted(message string) *Error {
	return NewError(http.StatusForbidden, CodeWriteNotGranted, message)
}

// V1NotAvailableForScopedKeys is the 403 a granted key gets on every /v1
// route: the grant can only be honored on /v2 (a legacy nil-grant key is
// served on /v1 unchanged).
func V1NotAvailableForScopedKeys(message string) *Error {
	return NewError(http.StatusForbidden, CodeV1NotAvailableForScopedKeys, message)
}

// NotCreatedByThisKey is the 403 for an object DELETE outside the calling
// key's own output. Not a scope failure: the
// WWW-Authenticate insufficient_scope channel is deliberately NOT used.
func NotCreatedByThisKey(message string, issues ...Issue) *Error {
	return NewError(http.StatusForbidden, CodeNotCreatedByThisKey, message, issues...)
}

// RequestTooLarge is the 413 for an oversized request body (C3).
func RequestTooLarge(message string) *Error {
	return NewError(http.StatusRequestEntityTooLarge, CodeRequestTooLarge, message)
}

// EtagMismatch is the 409 for a stale If-Match (C7), carrying the current
// etag so the agent can re-read and retry.
func EtagMismatch(currentEtag string) *Error {
	return NewError(http.StatusConflict, CodeEtagMismatch,
		fmt.Sprintf("the object changed since the etag in If-Match was read — current etag is %q; re-read the object and retry", currentEtag))
}

// VersionUnsupported is the 400 for a document produced by a newer format
// version (C6: surfaces SPEC §10's wording, naming both versions).
func VersionUnsupported(documentVersion, supportedVersion string) *Error {
	return NewError(http.StatusBadRequest, CodeVersionUnsupported,
		fmt.Sprintf("the document was produced by a newer version of the AnyBlock format: document formatVersion %s is newer than the supported formatVersion %s", documentVersion, supportedVersion))
}

// ListResponse is the C10 paginated list envelope: default limit 25,
// has_more, and a steering message when the result is truncated. Warnings
// carry warning-grade C6 issues (C11 — e.g. the unguarded-date-comparison
// hazard on search, or spaces a global search skipped).
type ListResponse[T any] struct {
	Data     []T     `json:"data"`
	Total    int     `json:"total"`
	Offset   int     `json:"offset"`
	Limit    int     `json:"limit"`
	HasMore  bool    `json:"has_more"`
	Message  string  `json:"message,omitempty"`
	Warnings []Issue `json:"warnings,omitempty"`
}

// ViewObject is the OpenAPI stand-in for one §6.2 view object: the runtime
// rows are pre-serialized JSON (json.RawMessage), which swag cannot resolve
// as a generic argument, so the view-listing annotations name this untyped
// object instead. The wire shape is identical — one JSON object per row.
type ViewObject map[string]any

// NewListResponse assembles the envelope and, when truncated, the C10
// steering message.
func NewListResponse[T any](data []T, total, offset, limit int, hasMore bool, narrowHint string) ListResponse[T] {
	if data == nil {
		data = []T{}
	}
	resp := ListResponse[T]{Data: data, Total: total, Offset: offset, Limit: limit, HasMore: hasMore}
	if hasMore {
		resp.Message = fmt.Sprintf("%d matches — showing %d from offset %d; %s", total, len(data), offset, narrowHint)
	}
	return resp
}

// ObjectRow is the C5 minimal list row: id, name, type (a type key) plus
// requested property values. SpaceId is set only on global search rows —
// the addressing info a follow-up space-scoped read needs.
type ObjectRow struct {
	Id         string         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	SpaceId    string         `json:"space_id,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
}

// SpaceRow is a minimal space list row. Description rides the row because
// it is free (same tech-space record) and it is what disambiguates spaces on
// the canonical "list my spaces, pick one to write to" trace — withholding
// it would force a GET-one per space (the N+1 pushed onto the agent).
type SpaceRow struct {
	// Id is the space's short reference: the last six characters of the
	// first half of its id. It is the full id instead when that tail is
	// shared with another visible space, or when the request asked for
	// `?ids=full`. Either spelling is accepted back on every route that
	// takes a space.
	Id          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Space is the space shape shared by GET-one and the space mutations
// ({id, name, description}). gatewayUrl and
// networkId are client-infrastructure fields, deliberately absent from v2
// (they remain reachable via v1). On a dry run (C9) nothing is committed:
// Id stays empty and DryRun is true.
type Space struct {
	Id          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	DryRun      bool   `json:"dry_run,omitempty"`
}

// CreateSpaceRequest is the POST /v2/spaces body.
type CreateSpaceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// UpdateSpaceRequest is the PATCH /v2/spaces/{space_id} body: omitted
// fields stay unchanged (pointers distinguish absent from present-but-empty;
// at least one field is required).
type UpdateSpaceRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

//
// ---- P1c: whoami (GET /v2/auth/whoami) ----
//

// WhoamiResponse describes the authenticated CREDENTIAL — never the person;
// there is only ever one "who" on a single-account localhost API. KeyStatus
// and Notice repeat the Anytype-Key-Status / Anytype-Notice header signal in
// the body, because agents read bodies, not headers.
type WhoamiResponse struct {
	Key       WhoamiKey   `json:"key"`
	Scope     string      `json:"scope"` // "jsonApi" | "full" | "limited"
	Grant     WhoamiGrant `json:"grant"`
	Api       WhoamiApi   `json:"api"`
	KeyStatus string      `json:"key_status"`       // "legacy" | "scoped", always present
	Notice    string      `json:"notice,omitempty"` // the legacy sentence, verbatim printable
}

// WhoamiKey names the credential. CreatedAt/ExpiresAt are RFC 3339 UTC;
// null means unknown (CreatedAt) / never (ExpiresAt).
type WhoamiKey struct {
	Id        string  `json:"id"` // the app link's hash, which is the id the key list shows
	Name      string  `json:"name"`
	CreatedAt *string `json:"created_at"`
	ExpiresAt *string `json:"expires_at"`
}

// WhoamiGrant is the credential's space grant as enforced. Scoped is the
// REQUIRED explicit boolean and the load-bearing field: "legacy unscoped
// key" is NEVER encoded as spaces:null, because consumers get the
// null-vs-empty test backwards and that failure direction is fail-open (the
// agent concludes it may touch every space). When Scoped is false, Spaces
// is [] and Permission is null.
type WhoamiGrant struct {
	Scoped     bool               `json:"scoped"`
	Permission *string            `json:"permission"` // the compact form agents string-match on
	Spaces     []WhoamiGrantSpace `json:"spaces"`
}

// WhoamiGrantSpace is one granted space. Spaces are OBJECTS with a
// per-entry permission even though the grant's perms are uniform today —
// the shape that lets P2 add per-space permissions without a breaking wire
// change. Name comes from the same grant-intersected path GET /v2/spaces
// uses; a granted space absent from the live list keeps its entry with an
// empty name.
type WhoamiGrantSpace struct {
	Id         string `json:"id"`
	Name       string `json:"name"`
	Permission string `json:"permission"`
}

// WhoamiApi carries the serving API version — the same value as the
// Anytype-Version response header.
type WhoamiApi struct {
	Version string `json:"version"`
}

// MemberRow is a minimal member list row; agents need member ids for
// assignee/creator property values.
type MemberRow struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Identity string `json:"identity,omitempty"`
}

// TypeRow is a minimal type list row: keys + names.
type TypeRow struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// PropertyRow is a minimal property list row: key, name, format.
type PropertyRow struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Format string `json:"format"`
}

// OptionRow is a select/multiSelect option list row: names are the
// option vocabulary (C2), color optional.
type OptionRow struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// ValidateResponse is the POST /v2/validate result: structural +
// format-semantic issues only (referential validation belongs to writes).
type ValidateResponse struct {
	Issues   []Issue `json:"issues"`
	Warnings []Issue `json:"warnings"`
}

// OutlineEntry is one row of the outline shape: every block's
// {indent, id, type} plus a text snippet capped at 80 runes — see
// buildOutlineEnvelope.
type OutlineEntry struct {
	Indent int    `json:"indent"`
	Id     string `json:"id"`
	Type   string `json:"type"`
	Text   string `json:"text,omitempty"`
}

//
// ---- create surface ----
//

// CreateResult is the response of every v2 create/update endpoint. C8:
// created ids are always returned. On a dry run (C9) nothing is committed:
// Id/Etag stay empty, DryRun is true, and Issues/Created report the would-be
// outcome.
type CreateResult struct {
	Id       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"` // type key of the created object
	Key      string       `json:"key,omitempty"`  // identity key (types, properties)
	Etag     string       `json:"etag,omitempty"` // etag of the created object
	DryRun   bool         `json:"dry_run,omitempty"`
	Created  *SideEffects `json:"created,omitempty"`
	Issues   []Issue      `json:"issues,omitempty"`
	Warnings []Issue      `json:"warnings,omitempty"`
}

// SideEffects lists the schema entities a create materialized on the way
// (create-missing, SPEC §3/§2a) — or would materialize, on a dry run.
type SideEffects struct {
	Properties []PropertyRow   `json:"properties,omitempty"`
	Options    []CreatedOption `json:"options,omitempty"`
}

// CreatedOption names one select/multiSelect option created by
// create-missing resolution (SPEC §3: option names, never ids).
type CreatedOption struct {
	Property string `json:"property"` // property key
	Name     string `json:"name"`
}

// CreatePropertyRequest is the POST properties body:
// {key?, name, format, options?:[{name,color?}]}.
type CreatePropertyRequest struct {
	Key     string                `json:"key,omitempty"`
	Name    string                `json:"name"`
	Format  string                `json:"format"`
	Options []CreateOptionRequest `json:"options,omitempty"`
}

// CreateOptionRequest is one select option in a property create.
type CreateOptionRequest struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// UpdatePropertyRequest is the PATCH properties/{key} body.
type UpdatePropertyRequest struct {
	Name *string `json:"name,omitempty"`
}

// CreateQueryRequest is the POST queries body. filter (compact string) is
// reserved for the search parser; filters/sorts follow the AnyBlock §6.2
// shapes and are passed into the query's initial dataview verbatim. views,
// when given, replaces the default single view and is mutually exclusive with
// top-level filters/sorts (ambiguous_input otherwise).
type CreateQueryRequest struct {
	Name string `json:"name"`
	Type string `json:"type"`
	// The RawMessage fields carry pre-serialized §6.2 arrays. No OpenAPI
	// annotation references this type today; if one ever does, follow the
	// SearchRequestDoc pattern — swag cannot resolve json.RawMessage, and
	// its v3 parser panics on swaggertype:"array,…".
	Filter  string          `json:"filter,omitempty"`
	Filters json.RawMessage `json:"filters,omitempty"`
	Sorts   json.RawMessage `json:"sorts,omitempty"`
	Views   json.RawMessage `json:"views,omitempty"`
}

// CreateCollectionRequest is the POST collections body.
type CreateCollectionRequest struct {
	Name  string   `json:"name"`
	Items []string `json:"items,omitempty"`
}

// UploadFileRequest is the JSON form of POST files (URL upload); the
// multipart form is the byte-upload alternative.
type UploadFileRequest struct {
	Url  string `json:"url" binding:"required"`
	Name string `json:"name,omitempty"`
}

// FileUploadResult is the POST files response: the file object id that
// file/image blocks and iconImage values need (R11).
//
// mime_type is v2's OWN response field and follows C2's snake_case. The
// query surface's `mimeType` field alias (object.go v2FieldAliases) is a
// different thing wearing the same word — the FORMAT's file-block field
// name — and is renamed on the anyblock branch, not here.
type FileUploadResult struct {
	Id       string `json:"id"`
	Name     string `json:"name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	Size     int64  `json:"size,omitempty"`
	DryRun   bool   `json:"dry_run,omitempty"`
}

//
// ---- query surface ----
//

// SearchRequest is the POST search body (space-scoped and global). filter
// (the compact string, SPEC §6.2.1) and filters (the structured §6.2 array)
// are mutually exclusive — both → 400 ambiguous_input; both land on one
// internal tree. Pagination is the C10 query params (offset/limit) — a body
// limit is rejected by the strict request schema. Search is a read: exempt
// from Idempotency-Key, and a supplied dry_run is ignored.
type SearchRequest struct {
	Query   string          `json:"query,omitempty"`
	Type    string          `json:"type,omitempty"`
	Filter  string          `json:"filter,omitempty"`
	Filters json.RawMessage `json:"filters,omitempty"`
	Sorts   json.RawMessage `json:"sorts,omitempty"`
	Fields  []string        `json:"fields,omitempty"`
}

// SearchRequestDoc mirrors SearchRequest for the OpenAPI document ONLY (the
// search annotations reference it): swag cannot resolve json.RawMessage and
// its v3 parser panics on swaggertype:"array,…", so the §6.2 array fields
// are documented through this twin instead. Wire shape is identical. A unit
// test pins the twin's JSON field set to SearchRequest's so the two cannot
// drift.
type SearchRequestDoc struct {
	Query   string           `json:"query,omitempty"`
	Type    string           `json:"type,omitempty"`
	Filter  string           `json:"filter,omitempty"`
	Filters []map[string]any `json:"filters,omitempty"`
	Sorts   []map[string]any `json:"sorts,omitempty"`
	Fields  []string         `json:"fields,omitempty"`
}

//
// ---- edit surface ----
//

// DiffStats summarizes what a mutation changed: the
// accidental-full-rewrite signal on PUT, the receipt on PATCH.
type DiffStats struct {
	BlocksAdded       int `json:"blocks_added"`
	BlocksRemoved     int `json:"blocks_removed"`
	BlocksChanged     int `json:"blocks_changed"`
	BlocksMoved       int `json:"blocks_moved"`
	PropertiesChanged int `json:"properties_changed"`
}

// EditResult is the PATCH response: the new etag, the created-block id map
// keyed by payload position, the schema side effects (created select
// options, like create), and the diff stats. On a dry run nothing
// is committed: Etag stays empty and DryRun is true; CreatedBlocks/Created/
// DiffStats report the would-be outcome.
//
// The nested slots CreatedBlocks reports are exactly the ones the id
// refusals tell a caller to leave empty, so leaving them unreported made the
// API withhold the answer it had promised. A payload position carrying an id
// is absent because since §8.29 that id resolves to an existing block whose
// identity the op keeps, and reporting a preserved block as created would be
// the same lie diff_stats used to tell. (This paragraph is deliberately on
// the type: swag publishes a FIELD comment as the schema description, and a
// reader outside this repository cannot follow either reference.)
type EditResult struct {
	Etag   string `json:"etag,omitempty"`
	DryRun bool   `json:"dry_run,omitempty"`
	// CreatedBlocks maps each payload position that created a block to the
	// id the server minted for it: the top-level run positions
	// ("ops[3].blocks[0]") and the nested slots alike, such as a table's
	// rows and columns ("ops[3].blocks[0].rows[1]") and the blocks inside a
	// cell run ("ops[3].value[1]"). A position that carried an id is
	// absent, because the block it names already existed.
	CreatedBlocks map[string]string `json:"created_blocks,omitempty"`
	// CreatedViews maps each payload position that created a dataview view
	// to the view id the server minted: an insert_view op ("ops[i]"), or a
	// view slot of an update_block set channel ("ops[i].set.views[2]").
	// View ids are always server-minted, and a view is not a block, so they
	// are reported here rather than in CreatedBlocks.
	CreatedViews map[string]string `json:"created_views,omitempty"`
	Created      *SideEffects      `json:"created,omitempty"`
	DiffStats    DiffStats         `json:"diff_stats"`
	Warnings     []Issue           `json:"warnings,omitempty"`
}

// SchemaEntry is one GET /v2/schemas/{kind} payload: the strict-mode
// generation schema (C13) plus one worked example (C12). The filters kind
// additionally carries the compact filter-string grammar (EBNF + examples) —
// one concept, one discovery slot (§5), the artifact the GBNF
// conversion consumes.
type SchemaEntry struct {
	Kind            string          `json:"kind"`
	Endpoint        string          `json:"endpoint"`
	Schema          json.RawMessage `json:"schema" swaggertype:"object"`
	Example         json.RawMessage `json:"example" swaggertype:"object"`
	Grammar         string          `json:"grammar,omitempty"`
	GrammarExamples []string        `json:"grammar_examples,omitempty"`
}

// SchemaIndex is the GET /v2/schemas payload. Ops lists the PATCH
// ops (per-op schemas at /v2/schemas/ops/{op}).
type SchemaIndex struct {
	Kinds []SchemaIndexEntry `json:"kinds"`
	Ops   []SchemaIndexEntry `json:"ops,omitempty"`
}

// SchemaIndexEntry is one row of the schema index.
type SchemaIndexEntry struct {
	Kind     string `json:"kind"`
	Endpoint string `json:"endpoint"`
	Url      string `json:"url"`
}

//
// ---- the output-only property contract (SPEC §4a) ----
//

// outputOnlyPropertyKeys are the SPEC §4a output-only property keys: an
// export writes them, a write must not. They live here, in the leaf model
// package, because two layers need the SAME answer — the service refuses a
// set_properties naming one (stateops.go), and the wrapper's describe must
// not advertise one as settable. A hand-copied second list is the drift
// class §8.31 was about.
//
// isFavorite is deliberately absent — SPEC §3 marks it authorable.
var outputOnlyPropertyKeys = map[string]bool{
	"coverId": true, "coverType": true, "createdDate": true,
	"lastModifiedDate": true, "creator": true, "isArchived": true,
	"resolvedLayout": true,
}

// IsUnwritableProperty reports whether set_properties can never change a
// property — the question describe must answer and the write path must
// enforce. It is the union of TWO sources that mean different things:
//
//   - SPEC §4a output-only (the hand list above): an export writes them, a
//     write must not. coverId/coverType/isArchived live only here — the
//     bundle does not mark them readonly.
//   - the bundle's own ReadOnly flag: 106 relations, including the DERIVED
//     ones (links, backlinks, mentions, snippet…). These are computed, so a
//     write to them is accepted and then does nothing.
//
// The second source is why this predicate exists. describe listed `Links`
// among a type's settable properties, gemma-4-e4b duly set it to point at
// another object, and the API answered "no changes" — a silent no-op, the
// worst outcome a write can have. Deriving the answer from the bundle
// instead of a second hand list is the same rule the §4a comment already
// states; that list simply never covered the derived relations.
func IsUnwritableProperty(key string) bool {
	if IsOutputOnlyProperty(key) {
		return true
	}
	stored, ok := bundle.RelationKeyByApiSlug(key)
	if !ok {
		// not a slug: the caller may already hold the stored spelling
		stored = domain.RelationKey(key)
	}
	rel, err := bundle.GetRelation(stored)
	return err == nil && rel != nil && rel.ReadOnly
}

// IsOutputOnlyProperty reports whether a property key is output-only. The
// keys above are STORED spellings; the wire spells slugs,
// so a served `created_date` has to answer here exactly as `createdDate`
// does — one predicate, both vocabularies.
func IsOutputOnlyProperty(key string) bool {
	if outputOnlyPropertyKeys[key] {
		return true
	}
	stored, ok := bundle.RelationKeyByApiSlug(key)
	return ok && outputOnlyPropertyKeys[string(stored)]
}
