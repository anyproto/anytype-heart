package anyblockjson

// omittedrelation.go — the §2f omission rule: a bundle does not carry a
// relation document whose definition restates the bundled table.
//
// Measured over the 38,061-document corpus: 9,675 of 10,617 relation
// documents are installed copies of the 194 bundled relations, and ~98% of
// them are field-identical to bundle/relations.json — each a ~967-byte
// restatement of `{key, name, format}` every reader already ships. The
// dictionary's `installed` list stands for them (§2f); the composition omits
// the documents; and a reader reconstructs each one from its own table,
// which is exactly what a restore does anyway.
//
// The predicate is FAIL-CLOSED in every direction: a detail key it cannot
// classify keeps the document, a stored value of an alien kind keeps the
// document, a block the format preserves keeps the document. Omission is an
// optimization; keeping a document is never wrong, and a predicate that
// omits one carrying real data would delete that data silently — the
// disqualifying failure for a backup format.

import (
	"math"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// relationDefinitionKeys are the stored keys that ARE the property's
// definition — what the bundled table states and what an omitted document
// must match, member for member. Everything a relation document carries is
// one of three things: a definition key (compared against the table), an
// install artifact (relationInstallArtifactKeys, any value), or an internal
// key the format never writes (strippedDetailKeys); a key that is none of
// them is real data and keeps the document.
var relationDefinitionKeys = map[string]bool{
	"name":                             true,
	"description":                      true,
	"isHidden":                         true,
	detailKeyRelationFormat:            true,
	"relationMaxCount":                 true,
	"relationReadonlyValue":            true,
	"relationDefaultValue":             true,
	detailKeyRelationFormatIncludeTime: true,
	detailKeyRelationFormatObjectTypes: true,
	// relationKey is definition-adjacent: it IS the identity the predicate
	// matched the table on, so it can never diverge on an omitted document
	// and the reconstruction re-states it exactly
	"relationKey": true,
}

// relationInstallArtifactKeys are the stored details of an installed copy
// that describe the INSTALL rather than the property — re-stamped by the
// next install, so omitting the document loses nothing a reader could act
// on. Every entry passed the §15 #12 admission test individually, against
// the 9,675 bundled-key relation documents in the corpus; the map value
// records the verdict. Keys that were candidates and FAILED the test — they
// keep the document, because they carry something a person did:
//
//   - `isUninstalled` (32 docs, all true): the user REMOVED this property
//     from the space; listing its key as installed would undo that.
//   - `isFavorite` / `isArchived`: user intent on the relation's own page,
//     same verdict §2a reached for a type's isHidden.
//   - `includeTime` — the BARE spelling, 7 docs: an orphan detail beside
//     relationFormatIncludeTime that no admission evidence explains; a key
//     the test cannot explain keeps the document, by the fail-closed rule.
var relationInstallArtifactKeys = map[string]string{
	// 10,617 of 10,617 docs, ONE distinct value each ("relation"): derivable
	// from the kind — the §2a layout verdict, on the other kind
	"layout":         "one distinct value, derivable from the kind",
	"resolvedLayout": "one distinct value, derivable from the kind",
	// how the INSTALL happened (builtin/usecase/api), not what the property
	// is — §2a's origin verdict
	"origin": "install provenance, not the property's definition",
	// an install timestamp at best — §2a's addedDate verdict
	"addedDate": "install timestamp",
	// the bundled url this copy was installed from — derivable from the key
	// (`_br<key>`)
	"sourceObject": "install artifact, derivable from the property key",
	// the bundled-table revision at install time; absent, the system re-runs
	// the bundled migrations and restamps it — §2a's revision verdict
	"revision": "bundled migration marker, restamped on install",
	// the moment the installed COPY was created — an install artifact, not
	// user data: nobody authored a bundled relation into the space (§2f)
	"createdDate": "the install moment of the copy, not user data",
	// restamped whenever the install machinery touches the copy; follows
	// createdDate
	"lastModifiedDate": "restamped by the install machinery",
	// derived from the bundled definition at install: measured, 154 bundled
	// keys carry one across 9,675 copies and NOT ONE key has a second
	// distinct value — a per-space fact would
	"apiObjectKey": "derived from the bundled definition: 0 of 154 keys carry a second value",
	// what the relation OBJECT's page features — an app-version stamp, not
	// the definition: 90 of 134 keys carry two different stamps for the SAME
	// key across spaces
	"featuredRelations": "the copy's page stamp: 90 of 134 keys carry two versions of it",
	// the deprecated pre-object-relations scope enum, written by legacy
	// installs; nothing reads it (330 docs)
	"scope": "deprecated legacy relation scope, unread",
	// which importer produced this copy — provenance of the machinery, the
	// same family as origin (32 docs)
	"importType": "import-machinery provenance",
	// a type-schema stamp on an object that defines no type: recommended
	// lists are read off TYPE objects only, and a relation is not one
	// (141 docs, all three lists together)
	"recommendedFeaturedRelations": "a type-schema stamp on an object that defines no type",
	"recommendedRelations":         "a type-schema stamp on an object that defines no type",
	"recommendedHiddenRelations":   "a type-schema stamp on an object that defines no type",
}

// RelationInstallArtifactKey reports a stored detail that describes the
// install of a bundled relation copy rather than the property it defines —
// the keys an omitted document (§2f) loses and the next install re-stamps.
// Exported for the round-trip comparator, which must skip exactly these on
// the way back and nothing else: the predicate is the format's own, not a
// copy, so the comparator and the composition cannot disagree (the miss
// that produced 1,344 false failures in one sweep).
func RelationInstallArtifactKey(key string) bool {
	_, ok := relationInstallArtifactKeys[key]
	return ok
}

// InstallStampedDefault reports a definition key carrying its empty default
// — what a reinstall stamps for a member the original copy never stored
// (`isHidden: false`, `object_types: []`). The comparator consults it for
// the added-details direction of an omitted-document round trip: absent and
// empty say the same thing for a definition member with a defined default,
// the same reading that lets the §2a settings follow the omit-empty canon.
// Scoped to definition keys and empty values only, so a reconstruction that
// invents a NON-empty member, or a key outside the definition, still
// reports.
func InstallStampedDefault(key string, v *types.Value) bool {
	return relationDefinitionKeys[key] && isEmptySystemValue(v)
}

// OmittedBundledRelation reports whether a relation snapshot is an installed
// copy whose definition is field-identical to the bundled table — the §2f
// omission rule: the bundle composition writes no document for it, lists its
// key in the dictionary's `installed`, and a reader reconstructs it from the
// table. The returned key is the bundled key the `installed` list carries.
//
// opts matters for one member: relationFormatObjectTypes stores type OBJECT
// ids (objectcreator rewrites bundled urls to derived ids at creation), and
// only the TypeResolver capability can turn them back into the keys the
// table speaks. Without one the comparison runs verbatim, which fails on
// every derived id — fewer omissions, never a wrong one, the same
// degradation every resolver-less path in this format takes.
func OmittedBundledRelation(sbType model.SmartBlockType, base *model.SmartBlockSnapshotBase, opts Options) (string, bool) {
	if !isRelationSmartBlock(sbType) || base == nil {
		return "", false
	}
	det := base.GetDetails().GetFields()
	key := stringDetail(det, "relationKey")
	if key == "" {
		return "", false
	}
	rel, err := bundle.GetRelation(domain.RelationKey(key))
	if err != nil {
		return "", false
	}
	if !relationBlocksCarryNothing(base) {
		// 19 corpus relation documents carry a dataview or free text on
		// their page; a document is the only place that survives
		return "", false
	}
	internal := strippedDetailKeys()
	for k := range det {
		switch {
		case isAttributionProperty(k):
			// `creator` and `lastModifiedBy` are in strippedDetailKeys, but
			// unlike the rest of that set they are NOT absent from a
			// document: export writes the §3 attribution spelling
			// `<id>#<name>` for both, so a KEPT copy of this relation would
			// have carried them and an omitted one does not. Every one of
			// the 10,617 corpus relation documents holds a `creator`.
			//
			// They are omitted anyway, on their own verdict rather than on
			// the internal set's: attribution on an installed copy of a
			// bundled relation records WHO RAN THE INSTALL, not who authored
			// the property — the bundled original is authored by nobody in
			// this space. Same class as createdDate, which
			// RelationInstallArtifactKey already covers.
		case internal[k]:
			// the raw stored value never travels in any document; nothing to
			// lose. (Attribution is the exception, handled above.)
		case RelationInstallArtifactKey(k):
			// re-stamped by the next install, any value
		case relationDefinitionKeys[k]:
			// compared below, table-side, so an ABSENT stored member is
			// judged too
		default:
			// unclassified is real data — fail closed
			return "", false
		}
	}
	if !bundledIdenticalDefinition(det, rel, opts) {
		return "", false
	}
	return key, true
}

// bundledIdenticalDefinition compares the stored definition members against
// the bundled table, absent-reads-as-zero on both sides — the same reading
// every consumer of these details applies. A stored value of an alien kind
// (a string where a bool belongs, a NULL that presence-mirroring §2d would
// carry) fails the comparison rather than coercing: the reconstruction
// writes the natural kind, so anything else is a difference by definition.
func bundledIdenticalDefinition(det map[string]*types.Value, rel *model.Relation, opts Options) bool {
	name, ok := stringDetailOK(det, "name")
	if !ok || name != rel.Name {
		return false
	}
	desc, ok := stringDetailOK(det, "description")
	if !ok || desc != rel.Description {
		return false
	}
	format, ok := numberDetailOK(det, detailKeyRelationFormat)
	if !ok || math.IsNaN(format) || math.IsInf(format, 0) ||
		format < 0 || format > math.MaxInt32 || model.RelationFormat(int32(format)) != rel.Format {
		return false
	}
	maxCount, ok := numberDetailOK(det, "relationMaxCount")
	if !ok || int32(maxCount) != rel.MaxCount || float64(int32(maxCount)) != maxCount {
		return false
	}
	for detailKey, table := range map[string]bool{
		"isHidden":                         rel.Hidden,
		"relationReadonlyValue":            rel.ReadOnly,
		detailKeyRelationFormatIncludeTime: rel.IncludeTime,
	} {
		b, ok := boolDetailOK(det, detailKey)
		if !ok || b != table {
			return false
		}
	}
	if v := det["relationDefaultValue"]; v != nil {
		if _, isNull := v.GetKind().(*types.Value_NullValue); !isNull {
			if rel.DefaultValue == nil || !proto.Equal(v, rel.DefaultValue) {
				return false
			}
		}
		// a stored null is the absence of a default — trimmedWhenEmpty's
		// own verdict for this key
	} else if rel.DefaultValue != nil {
		return false
	}
	stored := installedTargetKeys(valueStringList(det[detailKeyRelationFormatObjectTypes]), opts)
	table := make([]string, 0, len(rel.ObjectTypes))
	for _, u := range rel.ObjectTypes {
		if k, err := bundle.TypeKeyFromUrl(u); err == nil {
			table = append(table, string(k))
		} else {
			table = append(table, u)
		}
	}
	if len(stored) != len(table) {
		return false
	}
	for i := range stored {
		if stored[i] != table[i] {
			return false
		}
	}
	return true
}

// installedTargetKeys translates a stored relationFormatObjectTypes list to
// type KEYS: bundled urls directly, derived ids through the TypeResolver
// capability, anything else verbatim — relationTargetKeys' chain (§2d),
// restated here because this path has no exporter to memoize on.
func installedTargetKeys(entries []string, opts Options) []string {
	tr, _ := opts.ResolveProperties.(TypeResolver)
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if k, err := bundle.TypeKeyFromUrl(entry); err == nil {
			out = append(out, string(k))
			continue
		}
		if tr != nil {
			if k, ok := tr.TypeKeyById(entry); ok && k != "" {
				out = append(out, k)
				continue
			}
		}
		out = append(out, entry)
	}
	return out
}

// relationBlocksCarryNothing reports whether the snapshot's blocks are the
// standard relation-page scaffolding — root, layout, featured-relations,
// title/description text — which the editor regenerates and the format
// already drops as structural (§7). Anything else (a dataview, free text) is
// content only a document can carry.
func relationBlocksCarryNothing(base *model.SmartBlockSnapshotBase) bool {
	for _, b := range base.Blocks {
		if b == nil {
			return false
		}
		switch c := b.Content.(type) {
		case *model.BlockContentOfSmartblock, *model.BlockContentOfLayout, *model.BlockContentOfFeaturedRelations:
		case *model.BlockContentOfText:
			if c.Text.GetStyle() != model.BlockContentText_Title &&
				c.Text.GetStyle() != model.BlockContentText_Description {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// InstalledRelationDetails is the import half of the `installed` list: the
// stored details a reader reconstructs for a bundled key, the shape a fresh
// install writes (relationutils.Relation.ToDetails, minus the ids and
// provenance the installer stamps itself). Definition members are written
// even when empty — an install states the whole definition — which is why
// the comparator's added-details direction reads InstallStampedDefault. The
// TypeResolver capability translates the table's bundled type urls into this
// space's derived ids, exactly as objectcreator does on a real install; a
// reader without one keeps the urls, each its own address (§3).
func InstalledRelationDetails(key string, opts Options) (*types.Struct, bool) {
	rel, err := bundle.GetRelation(domain.RelationKey(key))
	if err != nil {
		return nil, false
	}
	tr, _ := opts.ResolveProperties.(TypeResolver)
	targets := make([]*types.Value, 0, len(rel.ObjectTypes))
	for _, u := range rel.ObjectTypes {
		id := u
		if k, err := bundle.TypeKeyFromUrl(u); err == nil && tr != nil {
			if resolved, ok := tr.TypeIdByKey(string(k)); ok && resolved != "" {
				id = resolved
			}
		}
		targets = append(targets, &types.Value{Kind: &types.Value_StringValue{StringValue: id}})
	}
	fields := map[string]*types.Value{
		"name":        {Kind: &types.Value_StringValue{StringValue: rel.Name}},
		"relationKey": {Kind: &types.Value_StringValue{StringValue: rel.Key}},
		"description": {Kind: &types.Value_StringValue{StringValue: rel.Description}},
		detailKeyRelationFormat: {Kind: &types.Value_NumberValue{
			NumberValue: float64(rel.Format)}},
		"isHidden":              {Kind: &types.Value_BoolValue{BoolValue: rel.Hidden}},
		"relationReadonlyValue": {Kind: &types.Value_BoolValue{BoolValue: rel.ReadOnly}},
		"relationMaxCount":      {Kind: &types.Value_NumberValue{NumberValue: float64(rel.MaxCount)}},
		detailKeyRelationFormatIncludeTime: {Kind: &types.Value_BoolValue{
			BoolValue: rel.IncludeTime}},
		detailKeyRelationFormatObjectTypes: {Kind: &types.Value_ListValue{
			ListValue: &types.ListValue{Values: targets}}},
	}
	if rel.DefaultValue != nil {
		fields["relationDefaultValue"] = rel.DefaultValue
	}
	return &types.Struct{Fields: fields}, true
}

// typed detail readers: value-or-zero with a kind verdict, so an alien kind
// fails the identity comparison instead of coercing to a zero that happens
// to match the table.

func stringDetail(det map[string]*types.Value, key string) string {
	s, _ := stringDetailOK(det, key)
	return s
}

func stringDetailOK(det map[string]*types.Value, key string) (string, bool) {
	v := det[key]
	if v == nil {
		return "", true
	}
	k, isString := v.GetKind().(*types.Value_StringValue)
	if !isString {
		return "", false
	}
	return k.StringValue, true
}

func numberDetailOK(det map[string]*types.Value, key string) (float64, bool) {
	v := det[key]
	if v == nil {
		return 0, true
	}
	k, isNumber := v.GetKind().(*types.Value_NumberValue)
	if !isNumber {
		return 0, false
	}
	return k.NumberValue, true
}

func boolDetailOK(det map[string]*types.Value, key string) (bool, bool) {
	v := det[key]
	if v == nil {
		return false, true
	}
	k, isBool := v.GetKind().(*types.Value_BoolValue)
	if !isBool {
		return false, false
	}
	return k.BoolValue, true
}
