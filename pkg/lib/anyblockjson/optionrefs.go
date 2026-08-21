package anyblockjson

// optionrefs.go — the qualified option legend: a `refs` entry that carries a
// select option's ID beside the NAME the document spells (§3, §9a).
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

import "strings"

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
