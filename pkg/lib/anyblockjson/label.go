package anyblockjson

// label.go — the spelling a document writes for a key the BUNDLED table does
// not speak for (§3): the entity's display name, NFC-normalized, otherwise
// verbatim.
//
// This used to be a ladder — the stored `apiObjectKey` when legal,
// re-spelled by the name inside one fold class, else the slug normalized,
// else the name normalized through an identifier grammar — and every rung
// existed to repair what normalization broke: `#`, `☕` and `C++` normalize
// to nothing or to `c`, so the ladder carried an empty-normalization
// fallback; `50% done` and `All` broke the grammar, so it carried a
// leading-`_` escape. Raw naming has no normalization step, so none of
// those faults can arise and none of the machinery survives: a name is a
// legal key exactly as written (the writable-key rule admits spaces,
// punctuation and every script), `api_object_key` is never read by the
// format at all, and the only inputs that still yield no label are the ones
// no rule could spell — an empty name, a name over the writable bound, a
// name carrying control characters. Each of those degrades to the stored
// key verbatim, which is always its own address.
//
// The name is carried exactly as the space holds it — edge whitespace and
// invisible characters included. The format warns about those (§12) but
// does not trim: a cleanup belongs where a user creates or renames the
// entity, applied once, not at the export seam on every write. The fold
// layer forgives the near-misses either way.

import (
	"golang.org/x/text/unicode/norm"
)

// PropertyLabel is the spelling a document writes for one space-minted
// PROPERTY key: NFC(name), else nothing. An empty answer means the key has
// no label but itself — the stored key is written verbatim, which is always
// its own address (§3 chain step 5). A name that merely repeats the stored
// key is no label either, for the same reason a spelling that repeated it
// never said anything: the verbatim key already says exactly that.
//
// `id` and `type` are refused: §2 refuses both SPELLINGS in `properties`
// before any resolution, so minting one would produce a label the exporter
// throws away with a warning. The type namespace does not share that
// reservation — its home surface is a value, not a member name — which is
// the one place the two namespaces differ and why TypeLabel is a separate
// function rather than a flag.
func PropertyLabel(key, name string) string {
	label := TypeLabel(key, name)
	if label == detailKeyId || label == detailKeyType {
		return ""
	}
	return label
}

// nfcTerm is §3's canonical form of one key spelling — NFC, otherwise
// verbatim. PropertyLabel/TypeLabel are the write half of the rule (a label
// is minted NFC); this is the read half's step: a slot's term resolves under
// its canonical form, so the precomposed and the decomposed bytes of one
// name land on one key instead of splitting into two visually
// indistinguishable properties. Stored keys — legend VALUES, the argument of
// every `…Slug` call — are never passed through it: a stored key's bytes are
// its address, whatever their normal form.
func nfcTerm(s string) string {
	return norm.NFC.String(s)
}

// TypeLabel is PropertyLabel for the type namespace.
func TypeLabel(key, name string) string {
	label := norm.NFC.String(name)
	// isWritablePropertyKey is the format's own shape rule (§3) — non-empty,
	// no control characters, inside the schema's 128-character bound. A name
	// longer than that is refused rather than truncated: a truncation
	// invents a spelling nobody chose, and the stored key is right there.
	if label == key || !isWritablePropertyKey(label) {
		return ""
	}
	return label
}
