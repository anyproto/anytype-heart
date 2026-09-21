package v2model

// ref.go — typed references to this API's own operations, the structured
// form of the route a repair hint names.
//
// Every hint used to be written in REST ("list keys with GET
// /v2/spaces/{space_id}/properties"). That is the right spelling on the HTTP
// surface and the wrong one on every other: an MCP caller has tools, not
// routes, and the six-actor benchmark in docs/evals/anytype-mcp-v2 showed
// that inferring `API-get-op-schema {"op":"set_properties"}` from `GET
// /v2/schemas/ops/set_properties` correlates with model strength — a
// capability gap the API manufactured. The fix is to emit the reference as
// data beside the prose, keyed by OpenAPI operationId, so each surface
// renders it in its own vocabulary: a REST reader gets the route (Ref.String
// is exactly what the prose carries), an MCP wrapper looks the op up in its
// own tool table and substitutes. The prose stays authoritative for humans;
// the reference is what makes the prose translatable mechanically.
//
// The op ids are the OpenAPI document's (core/api/docs/v2/openapi.json);
// TestOperationsMatchOpenAPI keeps the table and the document identical, so
// a renamed or added route surfaces here on the next test run.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Ref names one operation of this API, or — with no op — the request that
// produced the issue, to be resent with the query parameters given.
type Ref struct {
	// The operation's OpenAPI operationId, such as list_properties; absent means the request that produced this issue, resent with the query given, and the hint then spells only the query.
	Op string `json:"op,omitempty"`
	// Path parameters by their OpenAPI name, each substituted into the path once and verbatim (no percent-encoding); one left out keeps its {name} placeholder in the hint for the caller to fill.
	Params map[string]string `json:"params,omitempty"`
	// Query parameters to send with the operation, spelled in the hint as ?name=value pairs joined by & and sorted by name, values verbatim.
	Query map[string]string `json:"query,omitempty"`
}

// operation is one row of the OpenAPI document: method and path template.
type operation struct {
	method string
	path   string
}

// operations is the v2 OpenAPI document's operation table, keyed by
// operationId. It is hand-copied and test-pinned against the generated
// document rather than parsed at runtime: the served hint must not depend on
// an embedded JSON file being readable, and a mismatch is a build-time
// fact, not a request-time one.
var operations = map[string]operation{
	OpAuthWhoami:           {"GET", "/v2/auth/whoami"},
	OpCreateAuthChallenge:  {"POST", "/v2/auth/challenges"},
	OpCreateApiKey:         {"POST", "/v2/auth/api_keys"},
	OpListSchemas:          {"GET", "/v2/schemas"},
	OpGetOpSchema:          {"GET", "/v2/schemas/ops/{op}"},
	OpGetSchema:            {"GET", "/v2/schemas/{kind}"},
	OpSearchGlobal:         {"POST", "/v2/search"},
	OpListSpaces:           {"GET", "/v2/spaces"},
	OpCreateSpace:          {"POST", "/v2/spaces"},
	OpGetSpace:             {"GET", "/v2/spaces/{space_id}"},
	OpUpdateSpace:          {"PATCH", "/v2/spaces/{space_id}"},
	OpListChats:            {"GET", "/v2/spaces/{space_id}/chats"},
	OpCreateChat:           {"POST", "/v2/spaces/{space_id}/chats"},
	OpGetChatMessages:      {"GET", "/v2/spaces/{space_id}/chats/{chat_id}/messages"},
	OpAddChatMessage:       {"POST", "/v2/spaces/{space_id}/chats/{chat_id}/messages"},
	OpStreamChatMessages:   {"GET", "/v2/spaces/{space_id}/chats/{chat_id}/messages/stream"},
	OpDeleteChatMessage:    {"DELETE", "/v2/spaces/{space_id}/chats/{chat_id}/messages/{message_id}"},
	OpEditChatMessage:      {"PATCH", "/v2/spaces/{space_id}/chats/{chat_id}/messages/{message_id}"},
	OpToggleChatReaction:   {"POST", "/v2/spaces/{space_id}/chats/{chat_id}/messages/{message_id}/reactions"},
	OpReadChat:             {"POST", "/v2/spaces/{space_id}/chats/{chat_id}/read"},
	OpCreateCollection:     {"POST", "/v2/spaces/{space_id}/collections"},
	OpGetCollectionObjects: {"GET", "/v2/spaces/{space_id}/collections/{collection_id}/objects"},
	OpGetCollectionViews:   {"GET", "/v2/spaces/{space_id}/collections/{collection_id}/views"},
	OpUploadFile:           {"POST", "/v2/spaces/{space_id}/files"},
	OpDownloadFile:         {"GET", "/v2/spaces/{space_id}/files/{file_id}/content"},
	OpHeadFile:             {"HEAD", "/v2/spaces/{space_id}/files/{file_id}/content"},
	OpListMembers:          {"GET", "/v2/spaces/{space_id}/members"},
	OpGetMemberMe:          {"GET", "/v2/spaces/{space_id}/members/me"},
	OpListObjects:          {"GET", "/v2/spaces/{space_id}/objects"},
	OpCreateObject:         {"POST", "/v2/spaces/{space_id}/objects"},
	OpDeleteObject:         {"DELETE", "/v2/spaces/{space_id}/objects/{object_id}"},
	OpGetObject:            {"GET", "/v2/spaces/{space_id}/objects/{object_id}"},
	OpPatchObject:          {"PATCH", "/v2/spaces/{space_id}/objects/{object_id}"},
	OpCreateDiscussion:     {"POST", "/v2/spaces/{space_id}/objects/{object_id}/discussion"},
	OpListProperties:       {"GET", "/v2/spaces/{space_id}/properties"},
	OpCreateProperty:       {"POST", "/v2/spaces/{space_id}/properties"},
	OpDeleteProperty:       {"DELETE", "/v2/spaces/{space_id}/properties/{key}"},
	OpUpdateProperty:       {"PATCH", "/v2/spaces/{space_id}/properties/{key}"},
	OpListPropertyOptions:  {"GET", "/v2/spaces/{space_id}/properties/{key}/options"},
	OpCreateQuery:          {"POST", "/v2/spaces/{space_id}/queries"},
	OpGetQueryObjects:      {"GET", "/v2/spaces/{space_id}/queries/{query_id}/objects"},
	OpGetQueryViews:        {"GET", "/v2/spaces/{space_id}/queries/{query_id}/views"},
	OpSearchSpace:          {"POST", "/v2/spaces/{space_id}/search"},
	OpListTemplates:        {"GET", "/v2/spaces/{space_id}/templates"},
	OpCreateTemplate:       {"POST", "/v2/spaces/{space_id}/templates"},
	OpListTypes:            {"GET", "/v2/spaces/{space_id}/types"},
	OpCreateType:           {"POST", "/v2/spaces/{space_id}/types"},
	OpDeleteType:           {"DELETE", "/v2/spaces/{space_id}/types/{type}"},
	OpGetType:              {"GET", "/v2/spaces/{space_id}/types/{type}"},
	OpUpdateType:           {"PATCH", "/v2/spaces/{space_id}/types/{type}"},
	OpValidate:             {"POST", "/v2/validate"},
	OpListWidgets:          {"GET", "/v2/spaces/{space_id}/widgets"},
	OpCreateWidget:         {"POST", "/v2/spaces/{space_id}/widgets"},
	OpUpdateWidget:         {"PATCH", "/v2/spaces/{space_id}/widgets/{widget_id}"},
	OpDeleteWidget:         {"DELETE", "/v2/spaces/{space_id}/widgets/{widget_id}"},
}

// The operationIds, as constants so a hint site cannot misspell one.
const (
	OpAuthWhoami           = "auth_whoami"
	OpCreateAuthChallenge  = "create_auth_challenge"
	OpCreateApiKey         = "create_api_key"
	OpListSchemas          = "list_schemas"
	OpGetOpSchema          = "get_op_schema"
	OpGetSchema            = "get_schema"
	OpSearchGlobal         = "search_global"
	OpListSpaces           = "list_spaces"
	OpCreateSpace          = "create_space"
	OpGetSpace             = "get_space"
	OpUpdateSpace          = "update_space"
	OpListChats            = "list_chats"
	OpCreateChat           = "create_chat"
	OpGetChatMessages      = "get_chat_messages"
	OpAddChatMessage       = "add_chat_message"
	OpStreamChatMessages   = "stream_chat_messages"
	OpDeleteChatMessage    = "delete_chat_message"
	OpEditChatMessage      = "edit_chat_message"
	OpToggleChatReaction   = "toggle_chat_reaction"
	OpReadChat             = "read_chat"
	OpCreateCollection     = "create_collection"
	OpGetCollectionObjects = "get_collection_objects"
	OpGetCollectionViews   = "get_collection_views"
	OpUploadFile           = "upload_file"
	OpDownloadFile         = "download_file"
	OpHeadFile             = "head_file"
	OpListMembers          = "list_members"
	OpGetMemberMe          = "get_member_me"
	OpListObjects          = "list_objects"
	OpCreateObject         = "create_object"
	OpDeleteObject         = "delete_object"
	OpGetObject            = "get_object"
	OpPatchObject          = "patch_object"
	OpCreateDiscussion     = "create_discussion"
	OpListProperties       = "list_properties"
	OpCreateProperty       = "create_property"
	OpDeleteProperty       = "delete_property"
	OpUpdateProperty       = "update_property"
	OpListPropertyOptions  = "list_property_options"
	OpCreateQuery          = "create_query"
	OpGetQueryObjects      = "get_query_objects"
	OpGetQueryViews        = "get_query_views"
	OpSearchSpace          = "search_space"
	OpListTemplates        = "list_templates"
	OpCreateTemplate       = "create_template"
	OpListTypes            = "list_types"
	OpCreateType           = "create_type"
	OpDeleteType           = "delete_type"
	OpGetType              = "get_type"
	OpUpdateType           = "update_type"
	OpValidate             = "validate"
	OpListWidgets          = "list_widgets"
	OpCreateWidget         = "create_widget"
	OpUpdateWidget         = "update_widget"
	OpDeleteWidget         = "delete_widget"
)

// Operation returns the method and path template of an operationId, and
// whether the id is known.
func Operation(op string) (method, path string, ok bool) {
	o, ok := operations[op]
	return o.method, o.path, ok
}

// OperationIds lists every operationId, sorted.
func OperationIds() []string {
	ids := make([]string, 0, len(operations))
	for id := range operations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// NewRef builds a reference to op with path parameters given as name, value
// pairs. An empty value leaves the parameter unbound — the rendered route
// keeps its `{name}` placeholder for the caller to fill. The op and every
// parameter name must belong to the operation table; the tests over the hint
// sites (TestRefHelpersMatchOperations) are what catch a wrong one, since a
// hint is served text and must never panic a request.
func NewRef(op string, params ...string) Ref {
	r := Ref{Op: op}
	for i := 0; i+1 < len(params); i += 2 {
		if params[i+1] == "" {
			continue
		}
		if r.Params == nil {
			r.Params = map[string]string{}
		}
		r.Params[params[i]] = params[i+1]
	}
	return r
}

// With adds a query parameter to the reference and returns it.
func (r Ref) With(name, value string) Ref {
	q := make(map[string]string, len(r.Query)+1)
	for k, v := range r.Query {
		q[k] = v
	}
	q[name] = value
	r.Query = q
	return r
}

// Resend is the op-less reference: the request that produced the issue,
// sent again with this query parameter set.
func Resend(name, value string) Ref {
	return Ref{Query: map[string]string{name: value}}
}

// String renders the reference in REST — `GET /v2/spaces/{space_id}/types`
// with bound parameters substituted, `?create_missing_options=true` for a
// resend. This is the spelling every hint carries, verbatim, so a wrapper
// that knows the op can find and replace it mechanically.
func (r Ref) String() string {
	query := r.queryString()
	if r.Op == "" {
		return query
	}
	o, ok := operations[r.Op]
	if !ok {
		return r.Op + query
	}
	// one pass over the template: a bound value is opaque, so a value that
	// itself looks like a placeholder is never re-substituted, and the
	// rendering does not depend on map iteration order
	path := placeholder.ReplaceAllStringFunc(o.path, func(m string) string {
		if value, ok := r.Params[m[1:len(m)-1]]; ok {
			return value
		}
		return m
	})
	return o.method + " " + path + query
}

// placeholder matches one `{name}` path parameter in an operation's path.
var placeholder = regexp.MustCompile(`\{[a-z_]+\}`)

// queryString renders the query parameters, keys sorted, or "".
func (r Ref) queryString() string {
	if len(r.Query) == 0 {
		return ""
	}
	keys := make([]string, 0, len(r.Query))
	for k := range r.Query {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + r.Query[k]
	}
	return "?" + strings.Join(parts, "&")
}

// Hint is a repair sentence together with the operations it names, so the
// sentence can be re-spelled on a surface that has no routes.
type Hint struct {
	Text string
	Refs []Ref
}

// Hintf formats a repair sentence; every Ref among the arguments renders in
// REST (its %s) and is recorded typed, other arguments format as usual.
func Hintf(format string, args ...any) Hint {
	var refs []Ref
	for _, a := range args {
		if r, ok := a.(Ref); ok {
			refs = append(refs, r)
		}
	}
	return Hint{Text: fmt.Sprintf(format, args...), Refs: refs}
}

// Plain is a repair sentence that names no operation.
func Plain(text string) Hint {
	return Hint{Text: text}
}

// WithHint sets the issue's hint text and typed references from h.
func (i Issue) WithHint(h Hint) Issue {
	i.Hint = h.Text
	i.SeeAlso = h.Refs
	return i
}

// Hintf sets the issue's hint from a format whose Ref arguments render in
// REST and are recorded typed — WithHint(Hintf(...)) in one call.
func (i Issue) Hintf(format string, args ...any) Issue {
	return i.WithHint(Hintf(format, args...))
}

// The references the hint sites name, as helpers so the path-parameter names
// are written once, here, beside the operation table.

func RefListSpaces() Ref { return NewRef(OpListSpaces) }

func RefListTypes(spaceId string) Ref { return NewRef(OpListTypes, "space_id", spaceId) }
func RefGetType(spaceId, typeKey string) Ref {
	return NewRef(OpGetType, "space_id", spaceId, "type", typeKey)
}
func RefCreateType(spaceId string) Ref { return NewRef(OpCreateType, "space_id", spaceId) }
func RefUpdateType(spaceId, typeKey string) Ref {
	return NewRef(OpUpdateType, "space_id", spaceId, "type", typeKey)
}
func RefDeleteType(spaceId, typeKey string) Ref {
	return NewRef(OpDeleteType, "space_id", spaceId, "type", typeKey)
}

func RefListProperties(spaceId string) Ref { return NewRef(OpListProperties, "space_id", spaceId) }
func RefCreateProperty(spaceId string) Ref { return NewRef(OpCreateProperty, "space_id", spaceId) }
func RefUpdateProperty(spaceId, key string) Ref {
	return NewRef(OpUpdateProperty, "space_id", spaceId, "key", key)
}
func RefDeleteProperty(spaceId, key string) Ref {
	return NewRef(OpDeleteProperty, "space_id", spaceId, "key", key)
}
func RefListPropertyOptions(spaceId, key string) Ref {
	return NewRef(OpListPropertyOptions, "space_id", spaceId, "key", key)
}

func RefListMembers(spaceId string) Ref { return NewRef(OpListMembers, "space_id", spaceId) }
func RefListChats(spaceId string) Ref   { return NewRef(OpListChats, "space_id", spaceId) }
func RefGetChatMessages(spaceId, chatId string) Ref {
	return NewRef(OpGetChatMessages, "space_id", spaceId, "chat_id", chatId)
}

func RefListObjects(spaceId string) Ref   { return NewRef(OpListObjects, "space_id", spaceId) }
func RefListTemplates(spaceId string) Ref { return NewRef(OpListTemplates, "space_id", spaceId) }
func RefGetObject(spaceId, objectId string) Ref {
	return NewRef(OpGetObject, "space_id", spaceId, "object_id", objectId)
}
func RefPatchObject(spaceId, objectId string) Ref {
	return NewRef(OpPatchObject, "space_id", spaceId, "object_id", objectId)
}
func RefCreateDiscussion(spaceId, objectId string) Ref {
	return NewRef(OpCreateDiscussion, "space_id", spaceId, "object_id", objectId)
}
func RefSearchSpace(spaceId string) Ref      { return NewRef(OpSearchSpace, "space_id", spaceId) }
func RefCreateCollection(spaceId string) Ref { return NewRef(OpCreateCollection, "space_id", spaceId) }
func RefCreateQuery(spaceId string) Ref      { return NewRef(OpCreateQuery, "space_id", spaceId) }
func RefUploadFile(spaceId string) Ref       { return NewRef(OpUploadFile, "space_id", spaceId) }
func RefGetCollectionObjects(spaceId, collectionId string) Ref {
	return NewRef(OpGetCollectionObjects, "space_id", spaceId, "collection_id", collectionId)
}
func RefGetQueryObjects(spaceId, queryId string) Ref {
	return NewRef(OpGetQueryObjects, "space_id", spaceId, "query_id", queryId)
}

func RefListWidgets(spaceId string) Ref  { return NewRef(OpListWidgets, "space_id", spaceId) }
func RefCreateWidget(spaceId string) Ref { return NewRef(OpCreateWidget, "space_id", spaceId) }

func RefGetSchema(kind string) Ref { return NewRef(OpGetSchema, "kind", kind) }
func RefGetOpSchema(op string) Ref { return NewRef(OpGetOpSchema, "op", op) }
