package v2service

// apiv2schema.go derives the document schema THIS API serves and accepts from
// the format's own published schema.
//
// The format has two published schemas and neither fits this surface. The full
// schema describes an EXPORTED document: it carries the legends, the store's
// minted keys and the export provenance that only a live space can produce, so
// serving it advertises members a caller cannot supply and this API does not
// return. The authoring subset is built for BUNDLE authoring — composing a
// whole space offline, where `internal_key` is the anchor every cross-reference
// resolves through — and it identifies a type by a member this API ignores
// while forbidding the one this API identifies a type by. Both are right for
// their own consumer; neither is right for this one.
//
// So this is the third artifact, and it is DERIVED rather than written. The
// full/authoring pair drifted into an inverted identity contract that nobody
// noticed until it was diffed; a third hand-maintained copy would drift the
// same way. Here the delta is one list, and the invariant that every document
// valid under it is valid AnyBlock JSON is a test rather than a promise.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
)

// apiV2ExcludedMember is one root member the format publishes and this API
// neither serves nor accepts, with the reason it does not travel here.
type apiV2ExcludedMember struct {
	member string
	why    string
}

// apiV2ExcludedMembers is the whole delta between the format's document and
// this API's. Every entry is a member an export writes and a caller of THIS
// surface can neither supply nor resolve.
var apiV2ExcludedMembers = []apiV2ExcludedMember{
	{"property_internal_keys", "an export legend: it rebinds every spelling in the document to the stored key it names, so a caller could aim a value at a property they never spelled in `properties`. Refused on write by rejectExportLegends."},
	{"option_ids", "the same legend, for select option names."},
	{"uninstalled", "a type the user removed. This API never serves one: the type listings filter isUninstalled and a direct read refuses a corpse outright, so the member cannot reach a caller."},
	{"root", "output-only in the format: an escape hatch for non-default root-block attributes a caller does not write."},
	{"store", "output-only in the format."},
	{"file_remote", "remote file metadata a caller cannot mint; the file endpoints own this."},
	{"property_settings", "the definition of a property document, which POST /v2/spaces/{space_id}/properties owns with its own body shape."},
	{"type_internal_key", "the STORED type key. This API spells a type by its minted api slug — that is what the `type` member already carries and what GET /v2/spaces/{space_id}/types/{type} accepts back — so the stored key is redundant here, and for a space's own type it is a bson id a caller cannot address anything with."},
}

// apiV2ExcludedSet is the same list as a set, for the response trim.
var apiV2ExcludedSet = func() map[string]bool {
	set := make(map[string]bool, len(apiV2ExcludedMembers))
	for _, e := range apiV2ExcludedMembers {
		set[e.member] = true
	}
	return set
}()

// apiV2DocumentSchema is the format's document schema with the excluded
// members removed, computed once. A failure returns the full schema unchanged
// rather than nothing: a schema that advertises too much is a documentation
// bug, an absent one breaks discovery.
var apiV2DocumentSchema = sync.OnceValue(func() []byte {
	trimmed, err := trimExcludedMembers(anyblockjson.SchemaJSON(), apiV2ExcludedSet)
	if err != nil {
		return anyblockjson.SchemaJSON()
	}
	return trimmed
})

// trimExcludedMembers removes each excluded member from a document schema's
// root, together with the conditional gates that exist only to forbid it.
//
// The gates are the subtle part. A root allOf branch says "this member is
// false unless `kind` is X"; once the member is undeclared that clause is
// merely redundant, so it is dropped. One branch also REQUIRES a member
// (property_settings, when kind is a property document) and that clause is
// not redundant — left in place beside an undeclared member it would make
// that kind unsatisfiable, so it is dropped too. A branch whose then/else
// empties out is dropped whole.
func trimExcludedMembers(raw []byte, excluded map[string]bool) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("decode document schema: %w", err)
	}

	if props, ok := root["properties"].(map[string]any); ok {
		for member := range excluded {
			delete(props, member)
		}
	}
	setOrDelete(root, "required", withoutExcluded(root["required"], excluded))

	if branches, ok := root["allOf"].([]any); ok {
		kept := make([]any, 0, len(branches))
		for _, raw := range branches {
			branch, ok := raw.(map[string]any)
			if !ok {
				kept = append(kept, raw)
				continue
			}
			for _, arm := range []string{"then", "else"} {
				body, ok := branch[arm].(map[string]any)
				if !ok {
					continue
				}
				if props, ok := body["properties"].(map[string]any); ok {
					for member := range excluded {
						delete(props, member)
					}
					if len(props) == 0 {
						delete(body, "properties")
					}
				}
				setOrDelete(body, "required", withoutExcluded(body["required"], excluded))
				if len(body) == 0 {
					delete(branch, arm)
				}
			}
			if _, hasThen := branch["then"]; !hasThen {
				if _, hasElse := branch["else"]; !hasElse {
					continue // the branch only ever gated an excluded member
				}
			}
			kept = append(kept, branch)
		}
		root["allOf"] = kept
	}

	return json.Marshal(root)
}

// setOrDelete assigns a value, or removes the key when there is nothing left:
// assigning a nil here would serialise as JSON null, which is not a valid
// `required` and fails the metaschema.
func setOrDelete(node map[string]any, key string, value any) {
	if value == nil {
		delete(node, key)
		return
	}
	node[key] = value
}

// withoutExcluded returns a required list with the excluded names removed, or
// nil when nothing is left (so the caller's assignment deletes the key).
func withoutExcluded(value any, excluded map[string]bool) any {
	names, ok := value.([]any)
	if !ok {
		return nil
	}
	kept := make([]any, 0, len(names))
	for _, n := range names {
		if name, ok := n.(string); ok && excluded[name] {
			continue
		}
		kept = append(kept, n)
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}

// trimAPIDocumentEnvelope drops the excluded members from a document this API
// is about to serve, so a response matches the schema the same API publishes.
// The read path can emit a legend for a spelling its vocabulary cannot invert;
// that is an export's business, and a caller of this surface addresses
// properties by the served slug instead.
func trimAPIDocumentEnvelope(fields map[string]json.RawMessage) {
	for member := range apiV2ExcludedSet {
		delete(fields, member)
	}
}

// ---- per-kind narrowing ----
//
// Three discovery kinds serve the document schema: object, template and
// type_document. Serving the one trimmed schema for all three was
// measured as a byte-identical 44 KB three times (surface audit SA-c), and
// it advertised members the operation refuses: a type document with
// `blocks` is refused on create, a template must name `template_for`, and
// POST objects takes kind page or template only. Each kind now serves the
// document schema narrowed to what its operation accepts — the delta is
// data (apiV2KindNarrowings), derived from the trimmed schema at first use,
// and the invariant that a document valid under a narrowed schema is valid
// under the document schema is a test.

// apiV2KindNarrowing is one kind's delta from the document schema.
type apiV2KindNarrowing struct {
	// kinds is the closed `kind` vocabulary; one value becomes a const
	kinds []string
	// drop are root members the operation refuses or ignores
	drop []string
	// require are root members the operation demands beyond formatVersion
	require []string
}

// apiV2KindNarrowings is the whole delta, per kind. A kind absent here
// serves the document schema unchanged.
var apiV2KindNarrowings = map[string]apiV2KindNarrowing{
	// POST objects: validateDocumentRefs takes kind "", page or template
	// and sends object_type to its own endpoint; type_settings is refused
	// by the format on every kind here
	"object": {
		kinds: []string{"page", "template"},
		drop:  []string{"type_settings", "uninstalled"},
	},
	// POST templates: kind and type default to template, the target type
	// is required (createFromDocument with requireTemplate)
	"template": {
		kinds:   []string{"template"},
		drop:    []string{"type_settings", "uninstalled", "query_source", "collection_items"},
		require: []string{"template_for"},
	},
	// POST types (document form): CreateType injects kind object_type and
	// refuses blocks ("a type gets its views generated for it"); the other
	// dropped members belong to instances, not to the type
	"type_document": {
		kinds: []string{"object_type"},
		drop:  []string{"blocks", "template_for", "collection_items", "query_source"},
	},
}

// apiV2KindSchemas caches each narrowed schema; the input is constant.
var apiV2KindSchemas sync.Map // kind → []byte

// apiV2KindSchema is the document schema narrowed for one discovery kind.
// A kind without a narrowing, or a narrowing that fails, serves the
// document schema: too wide is a documentation bug, absent breaks discovery.
func apiV2KindSchema(kind string) []byte {
	if cached, ok := apiV2KindSchemas.Load(kind); ok {
		return cached.([]byte)
	}
	narrowing, ok := apiV2KindNarrowings[kind]
	if !ok {
		return apiV2DocumentSchema()
	}
	narrowed, err := narrowDocumentSchema(apiV2DocumentSchema(), narrowing)
	if err != nil {
		narrowed = apiV2DocumentSchema()
	}
	apiV2KindSchemas.Store(kind, narrowed)
	return narrowed
}

// narrowDocumentSchema applies one narrowing: closes `kind`, drops the
// members and the conditional gates that mention them, adds the
// requirements, and prunes the definitions nothing references any more
// (the block family alone is a quarter of the document schema).
func narrowDocumentSchema(raw []byte, n apiV2KindNarrowing) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("decode document schema: %w", err)
	}
	props, ok := root["properties"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("document schema has no properties")
	}
	dropped := make(map[string]bool, len(n.drop))
	for _, member := range n.drop {
		dropped[member] = true
		delete(props, member)
	}
	switch len(n.kinds) {
	case 0:
	case 1:
		props["kind"] = map[string]any{"const": n.kinds[0]}
	default:
		props["kind"] = map[string]any{"enum": n.kinds}
	}
	setOrDelete(root, "required", withoutExcluded(root["required"], dropped))
	if len(n.require) > 0 {
		required, _ := root["required"].([]any)
		for _, member := range n.require {
			required = append(required, member)
		}
		root["required"] = required
	}
	if branches, ok := root["allOf"].([]any); ok {
		kept := make([]any, 0, len(branches))
		for _, raw := range branches {
			encoded, err := json.Marshal(raw)
			if err != nil {
				return nil, fmt.Errorf("encode gate: %w", err)
			}
			if gateMentions(encoded, dropped) {
				continue // it only ever gated a member this kind does not carry
			}
			kept = append(kept, raw)
		}
		setOrDelete(root, "allOf", nonEmpty(kept))
	}
	pruneUnreferencedDefs(root)
	return json.Marshal(root)
}

// gateMentions reports whether a conditional names one of the members, as
// a property key or in a required list.
func gateMentions(encoded []byte, members map[string]bool) bool {
	for member := range members {
		if bytes.Contains(encoded, []byte(`"`+member+`"`)) {
			return true
		}
	}
	return false
}

// nonEmpty returns the slice, or nil when it is empty so the key is dropped.
func nonEmpty(items []any) any {
	if len(items) == 0 {
		return nil
	}
	return items
}

// pruneUnreferencedDefs keeps only the `$defs` reachable from the schema
// body, transitively through the definitions themselves.
func pruneUnreferencedDefs(root map[string]any) {
	defs, ok := root["$defs"].(map[string]any)
	if !ok {
		return
	}
	reachable := map[string]bool{}
	var walk func(node any)
	walk = func(node any) {
		switch v := node.(type) {
		case map[string]any:
			if ref, ok := v["$ref"].(string); ok && strings.HasPrefix(ref, "#/$defs/") {
				name := strings.TrimPrefix(ref, "#/$defs/")
				if !reachable[name] {
					reachable[name] = true
					walk(defs[name])
				}
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	for key, child := range root {
		if key != "$defs" {
			walk(child)
		}
	}
	for name := range defs {
		if !reachable[name] {
			delete(defs, name)
		}
	}
	if len(defs) == 0 {
		delete(root, "$defs")
	}
}
