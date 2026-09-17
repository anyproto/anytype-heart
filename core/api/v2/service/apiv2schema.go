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
	"encoding/json"
	"fmt"
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
