package anyblockjson

// alias.go — the wire spellings of the bundled keys whose STORED key says
// "relation" where the format says "property" (§3, v0.38).
//
// The format's own vocabulary renamed outright (kinds, property_settings,
// layout names), but sixteen bundled keys spell "relation" INSIDE the stored
// key itself, and stored keys must not change: they are the app's addresses,
// identical in every space, and rewriting one would re-key live data. The
// format already separates a key's SPELLING from its stored key — that is
// what `internal_key` vs `property` means — so the same mechanism carries
// the rename: an explicit alias table that outranks the derived
// `bundle.ApiSlug` rule wherever a stored key becomes a wire spelling, and
// is inverted wherever a wire spelling becomes a stored key.
//
// Measured over a 77-space corpus export (38,081 documents), the aliased
// spellings are the format's most visible "relation"s: `featured_relations`
// on 12,609 documents, `relation_key` on 3,859, `relation_option_color` on
// 2,639 — and the sharpest incoherence sat inside ONE document, where the
// block type `featured_properties` stood beside the property key
// `featured_relations`. One concept, one spelling (§15 #14).
//
// What an alias changes, precisely:
//
//   - the WIRE spelling only. The stored key is untouched, resolves verbatim
//     at every slot (§3 chain step 2), and keeps naming its key — a document
//     spelling `featuredRelations` still lands on `featuredRelations`.
//   - the derived slug and its fold class stop binding. `featured_relations`
//     no longer names `featuredRelations` — pre-freeze, no back-compat — and
//     the forgiving fold layer answers for the ALIAS's fold class instead
//     (`Featured_Properties` resolves, `Featured_Relations` does not). The
//     fold cannot keep serving the old class: `bundle.FoldApiKey` drops case
//     and underscores, so the stored key's own fold IS the old slug's, and
//     keeping it would keep the old spelling alive one typo away.
//   - nothing else. The alias is a bundled-table fact, so it needs no legend
//     entry (recordPropertyKey's bundledBinds asks the alias-aware table),
//     ships with every reader, and resolves offline like every bundled slug.
//
// The guards in dictionarywarn_test.go extend over this table: an alias must
// not collide with any bundled stored key, any other key's slug, or any
// other key's fold — and every spelling must name its key back.

import (
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// propertyKeyAliases maps a bundled STORED relation key to the wire spelling
// this format writes for it, where that spelling is not the derived api slug.
// Keys are named off the bundle so a rename there is a compile error here
// (the §2b rule).
var propertyKeyAliases = map[string]string{
	bundle.RelationKeyFeaturedRelations.String():            "featured_properties",
	bundle.RelationKeyRelationKey.String():                  "property_key",
	bundle.RelationKeyRelationOptionColor.String():          "property_option_color",
	bundle.RelationKeyRelationMaxCount.String():             "property_max_count",
	bundle.RelationKeyRelationReadonlyValue.String():        "property_readonly_value",
	bundle.RelationKeyRelationFormat.String():               "property_format",
	bundle.RelationKeyRelationFormatIncludeTime.String():    "property_format_include_time",
	bundle.RelationKeyRelationFormatObjectTypes.String():    "property_format_object_types",
	bundle.RelationKeyRelationDefaultValue.String():         "property_default_value",
	bundle.RelationKeyHeaderRelationsLayout.String():        "header_properties_layout",
	bundle.RelationKeyRecommendedRelations.String():         "recommended_properties",
	bundle.RelationKeyRecommendedFeaturedRelations.String(): "recommended_featured_properties",
	bundle.RelationKeyRecommendedHiddenRelations.String():   "recommended_hidden_properties",
	bundle.RelationKeyRecommendedFileRelations.String():     "recommended_file_properties",
}

// typeKeyAliases is the same table for the TYPE namespace: the two bundled
// types whose stored key says relation.
var typeKeyAliases = map[string]string{
	bundle.TypeKeyRelation.String():       "property",
	bundle.TypeKeyRelationOption.String(): "property_option",
}

// The inverses, built once: wire spelling → stored key, and fold(spelling) →
// stored key for the forgiving layer.
var (
	propertyKeyByAlias     = invertAliases(propertyKeyAliases)
	propertyKeyByAliasFold = foldAliases(propertyKeyAliases)
	typeKeyByAlias         = invertAliases(typeKeyAliases)
	typeKeyByAliasFold     = foldAliases(typeKeyAliases)
)

func invertAliases(aliases map[string]string) map[string]string {
	out := make(map[string]string, len(aliases))
	for key, alias := range aliases {
		out[alias] = key
	}
	return out
}

func foldAliases(aliases map[string]string) map[string]string {
	out := make(map[string]string, len(aliases))
	for key, alias := range aliases {
		out[bundle.FoldApiKey(alias)] = key
	}
	return out
}

// bundledPropertySpelling is the wire spelling of a BUNDLED relation key:
// its alias when it has one, its derived api slug otherwise. The caller has
// already established the key is bundled — a non-bundled key must never
// reach ApiSlug (dictionaryKeySpelling's bson-id rule).
func bundledPropertySpelling(key string) string {
	if alias, ok := propertyKeyAliases[key]; ok {
		return alias
	}
	return bundle.ApiSlug(key)
}

// bundledPropertyKeyBySpelling inverts bundledPropertySpelling: the alias
// table first, then the derived slug table — REFUSING a slug whose key has
// an alias, because that key's wire spelling is the alias and the vacated
// slug must not keep resolving (a spelling with two answers is the disease,
// and pre-freeze there is no back-compat to owe the old one).
func bundledPropertyKeyBySpelling(spelling string) (string, bool) {
	if key, ok := propertyKeyByAlias[spelling]; ok {
		return key, true
	}
	if key, ok := bundle.RelationKeyByApiSlug(spelling); ok {
		if _, aliased := propertyKeyAliases[string(key)]; !aliased {
			return string(key), true
		}
	}
	return "", false
}

// bundledTypeSpelling / bundledTypeKeyBySpelling are the type namespace's
// halves of the same two rules.
func bundledTypeSpelling(key string) string {
	if alias, ok := typeKeyAliases[key]; ok {
		return alias
	}
	return bundle.ApiSlug(key)
}

func bundledTypeKeyBySpelling(spelling string) (string, bool) {
	if key, ok := typeKeyByAlias[spelling]; ok {
		return key, true
	}
	if key, ok := bundle.TypeKeyByApiSlug(spelling); ok {
		if _, aliased := typeKeyAliases[string(key)]; !aliased {
			return string(key), true
		}
	}
	return "", false
}

// BundledPropertyKeysByFold is the bundled arm of the fold layer (§3 chain
// step 4 — the near-miss forgiveness), alias-aware: an aliased key answers
// for its ALIAS's fold class and no longer for its stored key's, and every
// other key folds as before. Exported because storeresolver's chain runs the
// same bundled arm the package-only reader does, and the two must not
// disagree about which fold class a key answers to.
func BundledPropertyKeysByFold(term string) []string {
	var out []string
	if key, ok := propertyKeyByAliasFold[bundle.FoldApiKey(term)]; ok {
		out = append(out, key)
	}
	for _, k := range bundle.RelationKeysByApiFold(term) {
		if _, aliased := propertyKeyAliases[string(k)]; !aliased {
			out = append(out, string(k))
		}
	}
	return out
}

// BundledTypeKeysByFold is BundledPropertyKeysByFold on the type namespace.
func BundledTypeKeysByFold(term string) []string {
	var out []string
	if key, ok := typeKeyByAliasFold[bundle.FoldApiKey(term)]; ok {
		out = append(out, key)
	}
	for _, k := range bundle.TypeKeysByApiFold(term) {
		if _, aliased := typeKeyAliases[string(k)]; !aliased {
			out = append(out, string(k))
		}
	}
	return out
}
