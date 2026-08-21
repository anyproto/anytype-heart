package anyblockjson

// optionrefs.go — the qualified option legend: a `refs` entry that carries a
// select option's ID beside the NAME the document spells (§3, §9a) — and,
// with it, the whole of option resolution. Export records an entry at the one
// site that substitutes a name for an id (recordOptionRef), and import
// resolves every select value through the one function below (resolveOption),
// so both halves of "which option does this name mean?" are answered in this
// file and nowhere else.
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
// ids keeps working exactly as it does without the legend.
//
// Two key shapes, one map. `refs` already holds plain compaction labels
// (§9a), whose charset has no `#`; a qualified key always has one, so the two
// populations are disjoint BY SHAPE, and each is reachable only from its own
// kind of position: `resolveId` reads plain labels and never a qualified key,
// option resolution reads qualified keys and never a plain label. Neither can
// be reached from the other's slot even by a hand-written document.
//
// The shape is the whole discriminator, and it can only be the shape: both
// directions have to sort a key into its population knowing nothing but the
// key. What a shape cannot say is that the right half MEANS something — the
// property census at the bottom of this file is that second, document-aware
// layer, a check on top of the admission rule rather than a replacement for
// it.

import (
	"sort"
	"strings"

	"github.com/anyproto/anytype-heart/core/domain"
)

// optionRefSeparator joins an option name to the property spelling that owns
// it. `#` is outside the plain-label charset (§9a), which is what makes the
// two key shapes tell themselves apart.
const optionRefSeparator = "#"

// optionRefKey builds the legend key for one option name under one property
// SPELLING — the slug the document writes, not the stored key, because the
// reader that resolves it reads the document.
//
// The pair (name, slug) is recoverable from the key by splitting on the LAST
// separator, which is what makes the shape a legend rather than a guess: the
// slug half is separator-free by admission (isQualifiedOptionRefKey), so a
// name carrying a `#` of its own — `C#`, `#1 priority` — still lands whole on
// the left of the split. That also makes the map from pairs to keys
// injective, so no two distinct (name, slug) pairs can contest one entry.
func optionRefKey(name, slug string) string {
	return name + optionRefSeparator + slug
}

// splitQualifiedOptionRefKey inverts optionRefKey: the (option name, property
// spelling) pair a qualified key stands for. It answers only for a key the
// shape rule admits, because an inadmissible key has no split to speak of —
// the last separator is where the halves meet only once both halves are legal
// (isQualifiedOptionRefKey pins the split by refusing a separator on the
// right).
func splitQualifiedOptionRefKey(s string) (name, slug string, ok bool) {
	if !isQualifiedOptionRefKey(s) {
		return "", "", false
	}
	i := strings.LastIndex(s, optionRefSeparator)
	return s[:i], s[i+1:], true
}

// isQualifiedRefsKey tells the two `refs` key shapes apart. It reads the key
// alone — no document context — because both directions have to agree about
// which population a key belongs to before they know anything else about it.
func isQualifiedRefsKey(s string) bool {
	return strings.Contains(s, optionRefSeparator)
}

// isQualifiedOptionRefKey admits a qualified key: <option name>#<property
// spelling>, split at the LAST separator, each half under the writable-key
// rule §3 puts on a property spelling — 1–128 characters, no control
// characters. The slug half carries that rule because it IS a property
// spelling; the name half takes the same bound rather than none, so a legend
// key is bounded like every other key slot in this format (257 characters,
// worst case). An option whose name is longer simply gets no entry and
// resolves by name, as it does today.
func isQualifiedOptionRefKey(s string) bool {
	i := strings.LastIndex(s, optionRefSeparator)
	if i < 0 {
		return false
	}
	// the right half cannot hold a separator (LastIndex), so admitting it
	// here is what pins the split — and what keeps the key invertible
	return isWritablePropertyKey(s[:i]) && isWritablePropertyKey(s[i+1:])
}

//
// ---- export ----
//

// optionRefPair is one entry before its property has a spelling: export
// records the STORED key, because the term ledger has not necessarily
// finished claiming spellings when a value is written, and renders the key at
// emission time (buildOptionRefs).
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

// buildOptionRefs renders the recorded pairs into legend keys, in the
// document's own spelling. It runs at envelope-assembly time, when every key
// slot has already claimed its term, so the spelling here is the spelling the
// values were written under.
//
// The spelling rendered here is the spelling the slot itself wrote — the
// ledger's answer for a key is memoized — so every key this emits is in the
// document's own property census by construction (below), which
// TestInvariant_MarshalOutputValidates checks over the hostile corpus.
//
// A property whose spelling carries the separator is skipped: `#` is a legal
// character in an api key (`strcase.ToSnake("C#")` is `c#`), and a slug
// holding one would make the split ambiguous. Those options keep today's
// name-only behavior, which is correct, merely less faithful.
func (e *exporter) buildOptionRefs() map[string]string {
	if len(e.optionRefs) == 0 {
		return nil
	}
	pairs := make([]optionRefPair, 0, len(e.optionRefs))
	for pair := range e.optionRefs {
		pairs = append(pairs, pair)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].key != pairs[j].key {
			return pairs[i].key < pairs[j].key
		}
		return pairs[i].name < pairs[j].name
	})
	out := map[string]string{}
	for _, pair := range pairs {
		slug := e.propertySlug(pair.key)
		if slug == "" || isQualifiedRefsKey(slug) {
			continue
		}
		label := optionRefKey(pair.name, slug)
		if !isQualifiedOptionRefKey(label) {
			continue // an over-long or unwritable half; the name still stands
		}
		out[label] = e.optionRefs[pair]
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
//  1. the document's own qualified legend entry, honored only for an id the
//     target space still serves as an option of that relation
//     (optionIdFromRefs below);
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
	if id, ok := imp.optionIdFromRefs(key, slug, name); ok {
		return id
	}
	if imp.opts.ResolveOptions != nil {
		if id, ok := imp.opts.ResolveOptions.OptionId(domain.RelationKey(key), name); ok {
			return id
		}
	}
	return name
}

// optionIdFromRefs is step 1 of §3's option resolution: the qualified legend
// entry, honored only when the id it carries is a live option OF THAT
// RELATION in the target space. The liveness question is the resolver's
// OptionName — it answers for an id exactly when that id is an option of that
// key — which is why a reader with no resolver ignores these entries
// altogether: it has no space to ask, and an id it cannot check is not an
// answer it can give.
func (imp *importer) optionIdFromRefs(key, slug, name string) (string, bool) {
	if slug == "" || name == "" || imp.opts.ResolveOptions == nil {
		return "", false
	}
	if len(imp.doc.Refs) == 0 {
		return "", false // no legend, and nothing to take a census for
	}
	// The property half has to name a property this document uses. Import
	// BUILDS the key from the spelling the slot it is resolving just wrote,
	// so the spelling is in the census by construction: this cannot turn away
	// an entry that would otherwise be honored, which makes it defence in
	// depth rather than a behaviour change. What it defends against is the
	// shape check standing alone — `isQualifiedOptionRefKey` admits any `X#Y`
	// whose halves are writable strings, and it has to, since both directions
	// must sort a key into its population knowing nothing but the key (§9a).
	// This is the document-aware layer on top of that: the one that makes the
	// right half mean a property rather than a string. Export writes only
	// spellings it just used, so its keys pass by construction too, and both
	// constructions are pinned by test — a census that stopped covering a
	// position would otherwise disable the legend in silence, which is the
	// exact failure mode this whole file exists to close.
	if !imp.usesProperty(slug) {
		return "", false
	}
	id := imp.doc.Refs[optionRefKey(name, slug)]
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

// The right half of a qualified key is a PROPERTY SPELLING, and a spelling
// this document never uses qualifies nothing: the entry is unreachable, and
// the value it was written for resolves by name as if the legend were absent.
// Both directions therefore ask whether the document uses the property, and
// this is the census of where a document can spell one:
//
//   - `properties`            — member names (§3)
//   - `property_keys`         — member names, the slug→stored-key legend (§3)
//   - `type_properties[].key` — §2a
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
// Two readers, because the two directions hold the document in two shapes:
// the importer has decoded it (jsonDoc), Validate has not (map[string]any,
// and it must answer before anything is decoded). They must return the same
// set for the same document — one census wider than the other means Validate
// warning about an entry import honors, or staying quiet about one import
// steps over — which TestInvariant_ThePropertyCensusesAgree pins over the
// hostile corpus and the goldens, position by position in
// TestOptionRefs_ThePropertyCensusCoversEveryPosition.

// usesProperty reports whether the document spells this property in any of
// the census positions below. The census is taken once and kept: a document
// carrying no qualified legend entry never asks for it at all.
func (imp *importer) usesProperty(spelling string) bool {
	if imp.propertyVocab == nil {
		imp.propertyVocab = imp.doc.propertySpellings()
	}
	return imp.propertyVocab[spelling]
}

// propertySpellings is the census over a decoded document (import side).
func (d *jsonDoc) propertySpellings() map[string]bool {
	out := map[string]bool{}
	add := func(spelling string) {
		if spelling != "" {
			out[spelling] = true
		}
	}
	for slug := range d.Properties {
		add(slug)
	}
	for slug := range d.PropertyKeys {
		add(slug)
	}
	if d.TypeProps != nil {
		for _, tp := range *d.TypeProps {
			add(tp.Key)
		}
	}
	var walk func(jbs []*jsonBlock)
	walk = func(jbs []*jsonBlock) {
		for _, jb := range jbs {
			if jb == nil {
				continue
			}
			switch jb.Type {
			case "property":
				add(jb.Key)
			case "link":
				// `properties` is a list of spellings on a link block and a
				// list of objects on a dataview (§5, §6.2), so the shape is
				// read per type rather than guessed; a payload that does not
				// decode contributes nothing, exactly as it does on import
				var slugs []string
				if len(jb.Properties) > 0 {
					_ = jsonUnmarshal(jb.Properties, &slugs)
				}
				for _, slug := range slugs {
					add(slug)
				}
			case "dataview":
				var props []jsonDvProperty
				if len(jb.Properties) > 0 {
					_ = jsonUnmarshal(jb.Properties, &props)
				}
				for _, p := range props {
					add(p.Key)
				}
				for _, jv := range jb.Views {
					add(jv.GroupBy)
					add(jv.CoverProperty)
					add(jv.EndProperty)
					for _, jc := range jv.Columns {
						add(jc.Property)
					}
					for _, js := range jv.Sorts {
						add(js.Property)
					}
					var walkFilters func(nodes []jsonFilter)
					walkFilters = func(nodes []jsonFilter) {
						for _, jf := range nodes {
							add(jf.Property)
							walkFilters(jf.Filters)
						}
					}
					walkFilters(jv.Filters)
				}
			}
			// a cell holds any block but a table, so every position above can
			// appear inside one (§6.1) — the same descent claimAuthoredIds
			// makes for ids
			for _, row := range jb.Rows {
				for _, cell := range row.Cells {
					if cell.Block != nil {
						walk([]*jsonBlock{cell.Block})
					}
					walk(cell.Blocks)
				}
			}
		}
	}
	walk(d.Blocks)
	return out
}

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
	addMembers(doc["property_keys"])
	if list, _ := doc["type_properties"].([]any); list != nil {
		for _, raw := range list {
			tp, _ := raw.(map[string]any)
			addString(tp["key"])
		}
	}
	var walkFilters func(v any)
	walkFilters = func(v any) {
		nodes, _ := v.([]any)
		for _, raw := range nodes {
			node, _ := raw.(map[string]any)
			addString(node["property"])
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
				addString(block["key"])
			case "link":
				items, _ := block["properties"].([]any)
				for _, item := range items {
					addString(item)
				}
			case "dataview":
				items, _ := block["properties"].([]any)
				for _, item := range items {
					p, _ := item.(map[string]any)
					addString(p["key"])
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
						addString(column["property"])
					}
					sorts, _ := view["sorts"].([]any)
					for _, rawSort := range sorts {
						sortNode, _ := rawSort.(map[string]any)
						addString(sortNode["property"])
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
