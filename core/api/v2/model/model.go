package v2model

// model.go holds the API v2 DTOs: the C6 error contract, the C10 list
// envelope, and the Phase-1 read shapes (APIV2.md).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

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
	// The space-grant codes. Messages NAME the actual grant: error-guided
	// self-correction is the v2 design language, and enumeration resistance
	// is a non-goal for a localhost single-user API.
	CodeSpaceNotGranted             = "space_not_granted"
	CodeWriteNotGranted             = "write_not_granted"
	CodeV1NotAvailableForScopedKeys = "v1_not_available_for_scoped_keys"
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
type Error struct {
	Status  int     `json:"status"`
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Issues  []Issue `json:"issues,omitempty"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NewError builds a C6 error.
func NewError(status int, code, message string, issues ...Issue) *Error {
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

// NotFound is the 404 for missing resources. Issues are optional — a 404
// that has a repair loop to describe (which read actually lists the thing
// the caller could not find) carries it C6-shaped rather than in prose.
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
func VersionUnsupported(documentVersion, supportedVersion int) *Error {
	return NewError(http.StatusBadRequest, CodeVersionUnsupported,
		fmt.Sprintf("the document was produced by a newer version of the AnyBlock format: document version %d is newer than the supported version %d", documentVersion, supportedVersion))
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
	SpaceId    string         `json:"spaceId,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
}

// SpaceRow is a minimal space list row. Description rides the row because
// it is free (same tech-space record) and it is what disambiguates spaces on
// the canonical "list my spaces, pick one to write to" trace — withholding
// it would force a GET-one per space (the N+1 pushed onto the agent).
type SpaceRow struct {
	// Id is the space's SHORT reference (§8.35) — the last six characters
	// of its CID half — or the full id when that tail is shared with
	// another visible space, or when the request asked for `?ids=full`
	// (§8.36: the spelling to persist, since a short reference is unique
	// only against the spaces you can currently see). Either spelling is
	// accepted back on every route that takes a space.
	Id          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Space is the space shape shared by GET-one and the space mutations
// (Phase 7, APIV2_SURFACES.md §2): {id, name, description}. gatewayUrl and
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

// UpdateSpaceRequest is the PATCH /v2/spaces/{spaceId} body: omitted
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
	KeyStatus string      `json:"keyStatus"`        //nolint:tagliatelle // C2: v2 bodies are camelCase; "legacy" | "scoped", always present
	Notice    string      `json:"notice,omitempty"` // the legacy sentence, verbatim printable
}

// WhoamiKey names the credential. CreatedAt/ExpiresAt are RFC 3339 UTC;
// null means unknown (CreatedAt) / never (ExpiresAt).
type WhoamiKey struct {
	Id        string  `json:"id"` // the app link's hash — the id the key list shows
	Name      string  `json:"name"`
	CreatedAt *string `json:"createdAt"` //nolint:tagliatelle // C2: v2 bodies are camelCase
	ExpiresAt *string `json:"expiresAt"` //nolint:tagliatelle // C2: v2 bodies are camelCase
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

// TypeRow is a minimal type list row: keys + names (Phase 1).
type TypeRow struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// PropertyRow is a minimal property list row: key, name, format (Phase 1).
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
// format-semantic issues only (referential validation is Phase 2).
type ValidateResponse struct {
	Issues   []Issue `json:"issues"`
	Warnings []Issue `json:"warnings"`
}

// OutlineEntry is one row of the outline shape: every block's
// {indent, id, type}, text only on heading blocks (APIV2.md Phase 1).
type OutlineEntry struct {
	Indent int    `json:"indent"`
	Id     string `json:"id"`
	Type   string `json:"type"`
	Text   string `json:"text,omitempty"`
}

//
// ---- Phase 2: create surface ----
//

// CreateResult is the response of every v2 create/update endpoint. C8:
// created ids are always returned. On a dry run (C9) nothing is committed:
// Id/Etag stay empty, DryRun is true, and Issues/Created report the would-be
// outcome.
type CreateResult struct {
	Id       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"` // type key of the created object (C2)
	Key      string       `json:"key,omitempty"`  // identity key (types, properties)
	Etag     string       `json:"etag,omitempty"` // C7 etag of the created object
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
// {key?, name, format, options?:[{name,color?}]} (APIV2.md Phase 2).
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

// CreateSetRequest is the POST sets body. filter (compact string) is
// reserved for the Phase-4 parser; filters/sorts follow the AnyBlock §6.2
// shapes and are passed into the set's initial dataview verbatim. views, when
// given, replaces the default single view and is mutually exclusive with
// top-level filters/sorts (ambiguous_input otherwise).
type CreateSetRequest struct {
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
	Url  string `json:"url"`
	Name string `json:"name,omitempty"`
}

// FileUploadResult is the POST files response: the file object id that
// file/image blocks and iconImage values need (R11).
type FileUploadResult struct {
	Id       string `json:"id"`
	Name     string `json:"name,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Size     int64  `json:"size,omitempty"`
	DryRun   bool   `json:"dry_run,omitempty"`
}

//
// ---- Phase 4: query surface ----
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
// ---- Phase 3: edit surface ----
//

// DiffStats summarizes what a mutation changed (APIV2.md Phase 3): the
// accidental-full-rewrite signal on PUT, the receipt on PATCH.
type DiffStats struct {
	BlocksAdded       int `json:"blocksAdded"`
	BlocksRemoved     int `json:"blocksRemoved"`
	BlocksChanged     int `json:"blocksChanged"`
	BlocksMoved       int `json:"blocksMoved"`
	PropertiesChanged int `json:"propertiesChanged"`
}

// EditResult is the PATCH response: the new etag, the created-block id map
// keyed by payload position, the schema side effects (created select
// options, like Phase 2's create), and the diff stats. On a dry run nothing
// is committed: Etag stays empty and DryRun is true; CreatedBlocks/Created/
// DiffStats report the would-be outcome.
type EditResult struct {
	Etag   string `json:"etag,omitempty"`
	DryRun bool   `json:"dry_run,omitempty"`
	// CreatedBlocks maps each payload position that CREATED a block to the id
	// the server minted for it — the top-level run positions
	// ("ops[3].blocks[0]") and the NESTED id slots alike: a table's rows and
	// columns ("ops[3].blocks[0].rows[1]") and the blocks inside a cell run
	// ("ops[3].value[1]"). Those nested slots are exactly the ones the id
	// refusals tell a caller to leave empty, so leaving them unreported made
	// the API withhold the answer it had promised. A payload position
	// carrying an id is absent: since §8.29 that id resolves to an existing
	// block whose identity the op keeps, and reporting a preserved block as
	// created would be the same lie diffStats used to tell.
	CreatedBlocks map[string]string `json:"createdBlocks,omitempty"`
	// CreatedViews maps each payload position that CREATED a dataview view to
	// the view id the server minted: an insertView op ("ops[i]") or a view
	// slot of an updateBlock set channel ("ops[i].set.views[2]"). View ids
	// are always server-minted; a view is not a block, so it is reported
	// here rather than in CreatedBlocks.
	CreatedViews map[string]string `json:"createdViews,omitempty"`
	Created      *SideEffects      `json:"created,omitempty"`
	DiffStats    DiffStats         `json:"diffStats"`
	Warnings     []Issue           `json:"warnings,omitempty"`
}

// SchemaEntry is one GET /v2/schemas/{kind} payload: the strict-mode
// generation schema (C13) plus one worked example (C12). The filters kind
// additionally carries the compact filter-string grammar (EBNF + examples) —
// one concept, one discovery slot (§5), the artifact the Phase-5 GBNF
// conversion consumes.
type SchemaEntry struct {
	Kind            string          `json:"kind"`
	Endpoint        string          `json:"endpoint"`
	Schema          json.RawMessage `json:"schema" swaggertype:"object"`
	Example         json.RawMessage `json:"example" swaggertype:"object"`
	Grammar         string          `json:"grammar,omitempty"`
	GrammarExamples []string        `json:"grammarExamples,omitempty"`
}

// SchemaIndex is the GET /v2/schemas payload. Ops lists the Phase-3 PATCH
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
// setProperties naming one (stateops.go), and the wrapper's describe must
// not advertise one as settable. A hand-copied second list is the drift
// class §8.31 was about.
//
// isFavorite is deliberately absent — SPEC §3 marks it authorable.
var outputOnlyPropertyKeys = map[string]bool{
	"coverId": true, "coverType": true, "createdDate": true,
	"lastModifiedDate": true, "creator": true, "isArchived": true,
	"resolvedLayout": true,
}

// IsOutputOnlyProperty reports whether a property key is output-only. The
// keys above are STORED spellings; the wire spells slugs (ADDRESSING §7.5a),
// so a served `created_date` has to answer here exactly as `createdDate`
// does — one predicate, both vocabularies.
func IsOutputOnlyProperty(key string) bool {
	if outputOnlyPropertyKeys[key] {
		return true
	}
	stored, ok := bundle.RelationKeyByApiSlug(key)
	return ok && outputOnlyPropertyKeys[string(stored)]
}

// OutputOnlyPropertyKeys returns the output-only keys, sorted — the form an
// agent-facing listing needs.
func OutputOnlyPropertyKeys() []string {
	keys := make([]string, 0, len(outputOnlyPropertyKeys))
	for key := range outputOnlyPropertyKeys {
		keys = append(keys, bundle.ApiSlug(key)) // advertised in the wire spelling
	}
	sort.Strings(keys)
	return keys
}
