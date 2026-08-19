package anyblockbatch

// typeterm.go — the one place the batch binds a TYPE term to a stored key.
//
// The envelope `key` a type document carries is the raw STORED key and is
// never translated (SPEC.md §2). Every other type slot IS translated: the
// envelope `type` and `template_for`, and `type_properties[].object_types`
// carry a term that resolves through the §3 chain — the document's own
// `type_keys` legend, then the bundled derived table, then verbatim.
//
// The lints below compare those slots against `TypeIds`, a map keyed by the
// untranslated envelope `key`. Comparing an untranslated map against a
// translated slot fails both ways:
//
//   - fail-closed: a bundle whose `template_for` is a slug its `type_keys`
//     legend binds to a stored key is rejected though the converter resolves
//     it perfectly well — and anyblockconvert turns the lint's finding into a
//     hard error, so a correct bundle cannot be converted;
//   - fail-open, and worse: when the term happens to equal some OTHER type's
//     stored key, the lint reports nothing while the converter resolves
//     elsewhere and falls through to `_ot<storedKey>` — a bundled url for a
//     type that is not bundled, matching nothing on import. That silent
//     dangling reference is precisely what these lints exist to catch.
//
// So the lint has to run the codec's own chain before it looks anything up.

import "github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"

// typeLegend is the `type_keys` envelope legend (§2), decoded alongside
// whatever slots a lint reads. It is per-document: the legend is the
// statement THIS document makes about ITS spellings, so it must be decoded
// from the same file as the slot it resolves.
type typeLegend map[string]string

// resolveTypeTerm binds one type term to the stored key it names, running the
// §3 chain in the type namespace exactly as the codec runs it here.
//
// "Here" is a package-only reader: anyblockconvert and anyblockvalidate pass
// no anyblockjson.Options.Keys, so anyblockjson resolves every type slot
// through the document's legend and then BundledKeyVocabulary — which is the
// same two steps below. The legend lookup is this file's only original line;
// steps 3 and 4 are one call into the package's own exported vocabulary,
// which answers the bundled table when it knows the term and hands the term
// back untouched when it does not (that pass-through IS chain step 4).
//
// TestLintResolvesTypeTermsLikeTheCodec pins the composition against what
// anyblockjson.Unmarshal actually stores, so the two cannot drift apart
// silently.
func resolveTypeTerm(legend typeLegend, term string) string {
	if term == "" {
		return ""
	}
	if key, ok := legend[term]; ok && key != "" {
		return key
	}
	key, _ := anyblockjson.BundledKeyVocabulary{}.TypeKey(term)
	return key
}

// templateTypeKey is the stored key of the template type. `template` is the
// reserved SPELLING (§3): validation gates `/template_for` on it, import
// derives the smartblock kind from it, and export refuses to move it in
// either direction — all through the document's own chain, which is why the
// gate below resolves the term rather than string-comparing it.
const templateTypeKey = "template"

// resolvedNote annotates a finding whose slot spelling differs from the
// stored key it resolves to, so the reported term stays the one the author
// can find in the file while the reason names what the converter will
// actually look for.
func resolvedNote(term, resolved string) string {
	if resolved == term {
		return ""
	}
	return " (this document's type_keys legend binds " + quote(term) + " to the stored key " + quote(resolved) + ")"
}

func quote(s string) string { return `"` + s + `"` }
