package anyblockbatch

// propertyterm.go — the one place the batch binds a PROPERTY term to a stored
// key. typeterm.go's twin, on the other namespace (SPEC.md §3: "one rule,
// stated once, covering both namespaces").
//
// The envelope `key` a property document carries is the raw STORED key and is
// never translated (SPEC.md §2). Every property SLOT is translated: the
// `properties` map's own keys, and `type_properties[].key`, carry a term that
// resolves through the §3 chain — the document's own `property_keys` legend,
// then the bundled derived table, then verbatim.
//
// The scans below build and compare tables the CONVERTER then reads by the
// resolved stored key: anyblockjson hands Options.ResolveFormat the output of
// importer.propertyKey, and Options.ResolveProperties a PropertyDefinition
// whose Key is likewise resolved. Keying an untranslated table and reading it
// translated fails both ways, and every failure here is silent:
//
//   - fail-open, the reason this matters: a bundle whose `property_keys`
//     legend backs a slug misses the format table entirely. The value passes
//     through as raw JSON — a date stays a string, a select mints no option,
//     an objects reference is never relinked — and NO Relation object is
//     minted for the property at all, so the space has a detail keyed to a
//     relation that does not exist. CheckPropertyFormats compared raw against
//     raw, so it agreed with itself and reported clean;
//   - fail-open again, one level down: newBatch pre-mints the declared select
//     vocabulary under the raw spelling, so the options land on a relation key
//     nothing ever asks for, and the values that DO arrive mint a second,
//     order-less set under the resolved key;
//   - fail-closed: `properties` spells bundled keys as their api slugs (§3),
//     and `bundle.GetRelationFormat("due_date")` does not know that spelling —
//     only `dueDate`. So a document written the canonical way was reported as
//     having no declared format, which anyblockconvert turns into a hard error
//     unless -lenient. Resolving first folds the slug arm and the stored-key
//     arm into one lookup;
//   - and CheckSharedSelects grouped by spelling, so two documents naming one
//     stored key two ways did not merge — exactly the collision the check
//     exists to warn about.
//
// So every scan has to run the codec's own chain before it keys or compares.

import "github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"

// propertyLegend is the `property_keys` envelope legend (§2), decoded
// alongside whatever slots a scan reads. Per-document, like typeLegend: the
// legend is the statement THIS document makes about ITS spellings, so it must
// be decoded from the same file as the slot it resolves.
type propertyLegend map[string]string

// resolvePropertyTerm binds one property term to the stored key it names,
// running the §3 chain in the property namespace exactly as the codec runs it
// here.
//
// "Here" is a package-only reader: anyblockconvert and anyblockvalidate pass
// no anyblockjson.Options.Keys, so anyblockjson resolves every property slot
// through the document's legend and then BundledKeyVocabulary — which is the
// same two steps below (importer.propertyKey). The legend lookup is this
// file's only original line; steps 3 and 4 are one call into the package's own
// exported vocabulary, which answers the bundled table when it knows the term
// and hands the term back untouched when it does not (that pass-through IS
// chain step 4).
//
// Unlike resolveTypeTerm this carries no reservation: `template` is a TYPE
// spelling, and the property namespace has no term whose meaning the envelope
// fixes.
//
// TestLintResolvesPropertyTermsLikeTheCodec pins the composition against what
// anyblockjson.Unmarshal actually stores, so the two cannot drift apart
// silently.
func resolvePropertyTerm(legend propertyLegend, term string) string {
	if term == "" {
		return ""
	}
	if key, ok := legend[term]; ok && key != "" {
		return key
	}
	key, _ := anyblockjson.BundledKeyVocabulary{}.PropertyKey(term)
	return key
}

// resolvedPropertyNote annotates a finding whose slot spelling differs from
// the stored key it resolves to, so the reported term stays the one the author
// can find in the file while the reason names what the converter will actually
// look for. resolvedNote's property-namespace twin.
// An empty `resolved` is not a resolution, it is an unset field on a value
// some other caller built, so it annotates nothing.
func resolvedPropertyNote(term, resolved string) string {
	if resolved == "" || resolved == term {
		return ""
	}
	return " (the converter looks for the stored key " + quote(resolved) + ")"
}
