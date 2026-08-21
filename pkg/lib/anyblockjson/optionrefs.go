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
	id := imp.doc.Refs[optionRefKey(name, slug)]
	if id == "" {
		return "", false
	}
	if _, live := imp.opts.ResolveOptions.OptionName(domain.RelationKey(key), id); !live {
		return "", false
	}
	return id, true
}
