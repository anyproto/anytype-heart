package anyblockjson

// optionrefs.go — the `option_ids` legend: the id of the option each select
// value NAMES (§3, §9a) — and, with it, the whole of option resolution.
// Export records an entry at the one site that substitutes a name for an id
// (recordOptionRef), and import resolves every select value through the one
// function below (resolveOption), so both halves of "which option does this
// name mean?" are answered in this file and nowhere else.
//
// Select and multi_select values are spelled by name (§3) because a bundle
// carries no option objects — unlike a linked object, which the bundle
// carries and the importer relinks, an option id from another space would
// dangle. Names cost identity in two ways a live account shows, and both were
// measured on a 34 339-object sweep:
//
//  1. Duplicate names. A space may hold two distinct options with one name
//     under one relation; name resolution returns the FIRST
//     (storeresolver.OptionId scans a list), so 7 objects came back pointing
//     at an option they were never on.
//  2. Rename. Export writes the name; if the option is renamed before the
//     document is read back, nothing resolves, and the import wiring mints a
//     NEW option carrying the stale name — resurrecting the duplicate and
//     orphaning the object from the renamed option.
//
// The entry is a HINT, not an address. Import uses the id only when it is a
// live option OF THAT RELATION in the target space, and falls back to name
// resolution otherwise, so a bundle carried to a space that never saw those
// ids keeps working exactly as it does without the legend. That is the
// deliberate difference from `property_internal_keys`/`type_internal_keys`, whose values are
// taken at face value: a stored key IS the address, while an option id is a
// shortcut past a name that is already one (§3).
//
// NESTED, not joined by a separator. The legend is
// {property spelling: {option name: option id}} because a name in this format
// is arbitrary user text and no character can be reserved to join it to its
// scope. The flat spelling this replaced keyed entries `<name>#<property>`,
// and `strcase.ToSnake("C#")` is `c#` — a legal api slug — so an option of a
// property named `C#` had no representable entry at all: the escape hatch was
// unreachable exactly where it was needed. Nesting removes the separator and
// with it the split rule, the two-charset admission rule, and the joined
// key's length bound. The inner key carries no charset rule at all, on
// purpose: it is the same string the value slot already holds, and a legend
// that cannot name a value its own document carries is that same hole one
// level down.

import (
	"github.com/anyproto/anytype-heart/core/domain"
)

//
// ---- export ----
//

// optionRefPair is one entry before its property has a spelling: export
// records the STORED key, because the term ledger has not necessarily
// finished claiming spellings when a value is written, and renders the
// spelling at emission time (buildOptionIds).
type optionRefPair struct {
	key  string // stored property key
	name string // the option name written into the document
}

// recordOptionRef notes that a name written into the document stands for a
// particular option id. Called from exactly one place — optionName, the one
// site where export substitutes a name for an id — so the legend covers
// exactly the values that need it and nothing else: there is no pruning pass
// because there is nothing unused to prune (§9a).
//
// FIRST WRITING WINS. Two distinct options of one property sharing one name
// produce one key, and a JSON list of two identical strings has no way to say
// which entry means which option — the collapse §11 already documents for
// name resolution. Keeping the first makes the collapse deterministic, which
// is what export∘import byte-stability needs; dropping the entry instead
// would hand the choice back to the resolver's list order and make a second
// generation differ from the first.
func (e *exporter) recordOptionRef(key, name, id string) {
	if key == "" || name == "" || id == "" || name == id {
		return
	}
	if e.optionRefs == nil {
		e.optionRefs = map[optionRefPair]string{}
	}
	pair := optionRefPair{key: key, name: name}
	if _, seen := e.optionRefs[pair]; seen {
		return
	}
	e.optionRefs[pair] = id
}

// buildOptionIds groups the recorded pairs under the document's own property
// spellings. It runs at envelope-assembly time, when every key slot has
// already claimed its term, so the spelling here is the spelling the values
// were written under.
//
// The spelling grouped on is the spelling the slot itself wrote — the
// ledger's answer for a key is memoized — so every outer key this emits is in
// the document's own property census by construction (below), which
// TestInvariant_MarshalOutputValidates checks over the hostile corpus.
//
// Nothing is skipped. Under the flat spelling this replaced, a property whose
// slug carried the separator and an option name past the joined key's bound
// both lost their entry silently; neither residue survives nesting (§11).
// The one thing that can still turn an entry away is a property that claimed
// no spelling at all, which no value in the document can be written under
// either.
//
// The intermediate map is what makes duplicate outer keys structurally
// impossible: the envelope's omap appends blindly, so two stored keys landing
// on one spelling would otherwise write the same JSON key twice.
func (e *exporter) buildOptionIds() map[string]map[string]string {
	if len(e.optionRefs) == 0 {
		return nil
	}
	out := map[string]map[string]string{}
	for pair, id := range e.optionRefs {
		slug := e.propertySlug(pair.key)
		// propertySlug hands back the STORED key verbatim when the vocabulary
		// has no spelling for it, and a stored key need not be a writable one
		// (§3 admits `a\nb`, a 140-character key, whatever the store holds).
		// /properties filters those before slugging; a dataview filter or sort
		// slot does not, so without this guard the legend takes an outer key
		// propertyNameIssues refuses and Marshal emits a document its own
		// Validate and Unmarshal reject — losing the whole object, not just
		// the legend. The `#` grammar this replaced bounded BOTH halves; only
		// dropping the name half was intended.
		if slug == "" || !isWritablePropertyKey(slug) {
			continue
		}
		if out[slug] == nil {
			out[slug] = map[string]string{}
		}
		out[slug][pair.name] = id
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// optionIdsFor is the value-level slice of buildOptionIds: the {name: id}
// map recorded for ONE stored key, ungrouped and unslugged, because a
// value-level caller holds the key rather than a document spelling.
func (e *exporter) optionIdsFor(key string) map[string]string {
	if key == "" || len(e.optionRefs) == 0 {
		return nil
	}
	out := map[string]string{}
	for pair, id := range e.optionRefs {
		if pair.key == key {
			out[pair.name] = id
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

//
// ---- import ----
//

// resolveOption resolves ONE select value — the whole of §3's three-step
// chain, and the only place any of it lives. Every option slot in the format
// arrives here: property values (import.go) and dataview filter values and
// sort custom orders (dataview.go) alike, which is what makes "how is an
// option value resolved?" a question this file answers by itself.
//
// First answer wins:
//
//  1. the document's own `option_ids` entry, honored only for an id the
//     target space still serves as an option of that relation
//     (optionIdFromLegend below);
//  2. name resolution through the wired resolver, which is what a bundle
//     carried to a space that never saw those ids falls back on;
//  3. the value unchanged, because creating a missing option is the wiring's
//     job (§3).
//
// `key` is the stored key the value lands on and `slug` the spelling the slot
// wrote: the resolver is asked with the former and the legend keyed by the
// latter, because the reader that resolves the legend is reading the
// document, not the store.
func (imp *importer) resolveOption(key, slug, name string) string {
	if id, ok := imp.optionIdFromLegend(key, slug, name); ok {
		return id
	}
	if imp.opts.ResolveOptions != nil {
		if id, ok := imp.opts.ResolveOptions.OptionId(domain.RelationKey(key), name); ok {
			return id
		}
	}
	return name
}

// optionIdFromLegend is step 1 of §3's option resolution: the `option_ids`
// entry, honored only when the id it carries is a live option OF THAT
// RELATION in the target space. The liveness question is the resolver's
// OptionName — it answers for an id exactly when that id is an option of that
// key — which is why a reader with no resolver ignores these entries
// altogether: it has no space to ask, and an id it cannot check is not an
// answer it can give.
//
// There is no reachability precondition left to state. The lookup is indexed
// by the spelling the slot in hand just wrote, so an entry filed under any
// other spelling is never consulted — where the flat spelling needed a census
// to make the key's right half MEAN a property rather than be a string, the
// nesting makes that structural. Validate still takes the census, to warn
// about an entry that can never be consulted (§12); import does not need it.
func (imp *importer) optionIdFromLegend(key, slug, name string) (string, bool) {
	if slug == "" || name == "" || imp.opts.ResolveOptions == nil {
		return "", false
	}
	id := imp.doc.OptionIds[slug][name]
	if id == "" {
		return "", false
	}
	if _, live := imp.opts.ResolveOptions.OptionName(domain.RelationKey(key), id); !live {
		return "", false
	}
	return id, true
}

//
// ---- the property vocabulary ----
//

// An `option_ids` outer key is a PROPERTY SPELLING, and a spelling this
// document never uses qualifies nothing: import indexes the legend by the
// spelling the slot it is resolving wrote, so such an entry is unreachable
// and the value it was written for resolves by name as if the legend were
// absent. Validate reports that (§12), and this is the census of where a
// document can spell a property:
//
//   - `properties`            — member names (§3)
//   - `property_internal_keys`         — member names, the spelling→stored-key legend (§3)
//   - `type_settings.property_definitions[].property` — §2a
//   - a `property` block's `key` (§5)
//   - a `link` block's `properties[]`, the shown-property list (§5)
//   - a `dataview` block's `properties[].key` (§6.2)
//   - a view's `group_by`, `cover_property`, `end_property` (§6.2)
//   - a view's `columns[].property` (§6.2)
//   - a view's `sorts[].property` (§6.2)
//   - a view's `filters[].property`, through nested `filters[]` (§6.2)
//   - every block position again inside a table cell, which holds any block
//     but a table (§6.1, schema `cellBlock`)
//
// The three that can reach an option value are `properties`, a sort's
// `property` and a filter's `property` (§3 says option values are names
// "everywhere": property values, filter values, custom orders). The rest are
// in the census because the vocabulary is a statement about the DOCUMENT, not
// about one slot — a property a document only groups by is still a property
// it uses — and because a census that tracked the reaching slots alone would
// silently narrow the moment a new slot started resolving options.
//
// A filter's `nested_property` is deliberately not in it: it names a property
// of the object the filter walks TO, not a key slot of this document — the
// importer passes it through without translating it (dataview.go) — and no
// option value is ever resolved under it.
//
// ONE reader, not two. The census used to exist in a decoded twin as well,
// because import took it too, and an agreement test stood between them. Import
// no longer takes a census at all (optionIdFromLegend), so the twin lost its
// only caller — and a function kept alive so a test can check it agrees with
// the one that is actually used proves nothing about behaviour. What the
// agreement test really guarded is that the census covers every position a
// property can be spelled in, and that is pinned directly, position by
// position, in TestOptionRefs_ThePropertyCensusCoversEveryPosition.

// rawPropertySpellings is the same census over an undecoded document
// (Validate's side). Every read is shape-tolerant: this runs after the schema
// has passed, but a census is not the place to have an opinion about a shape
// somebody else refuses.
func rawPropertySpellings(doc map[string]any) map[string]bool {
	out := map[string]bool{}
	add := func(spelling string) {
		if spelling != "" {
			out[spelling] = true
		}
	}
	addString := func(v any) {
		s, _ := v.(string)
		add(s)
	}
	addMembers := func(v any) {
		m, _ := v.(map[string]any)
		for term := range m {
			add(term)
		}
	}
	addMembers(doc["properties"])
	addMembers(doc[memberPropertyInternalKeys])
	if list, _ := typePropertyDefinitionsOf(doc); list != nil {
		for _, raw := range list {
			tp, _ := raw.(map[string]any)
			addString(tp[memberProperty])
		}
	}
	var walkFilters func(v any)
	walkFilters = func(v any) {
		nodes, _ := v.([]any)
		for _, raw := range nodes {
			node, _ := raw.(map[string]any)
			addString(node[memberProperty])
			walkFilters(node["filters"])
		}
	}
	var walk func(v any)
	walk = func(v any) {
		blocks, _ := v.([]any)
		for _, raw := range blocks {
			block, _ := raw.(map[string]any)
			switch typ, _ := block["type"].(string); typ {
			case "property":
				addString(block[memberProperty])
			case "link":
				items, _ := block["properties"].([]any)
				for _, item := range items {
					addString(item)
				}
			case "dataview":
				items, _ := block["properties"].([]any)
				for _, item := range items {
					p, _ := item.(map[string]any)
					addString(p[memberProperty])
				}
				views, _ := block["views"].([]any)
				for _, rawView := range views {
					view, _ := rawView.(map[string]any)
					addString(view["group_by"])
					addString(view["cover_property"])
					addString(view["end_property"])
					columns, _ := view["columns"].([]any)
					for _, rawColumn := range columns {
						column, _ := rawColumn.(map[string]any)
						addString(column[memberProperty])
					}
					sorts, _ := view["sorts"].([]any)
					for _, rawSort := range sorts {
						sortNode, _ := rawSort.(map[string]any)
						addString(sortNode[memberProperty])
					}
					walkFilters(view["filters"])
				}
			}
			rows, _ := block["rows"].([]any)
			for _, rawRow := range rows {
				row, _ := rawRow.(map[string]any)
				cells, _ := row["cells"].([]any)
				for _, cell := range cells {
					switch c := cell.(type) {
					case map[string]any:
						walk([]any{c})
					case []any:
						walk(c)
					}
				}
			}
		}
	}
	walk(doc["blocks"])
	return out
}
