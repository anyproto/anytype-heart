package anyblockjson

// typesettings.go implements the §2a `type_settings` group: everything that
// defines a TYPE, in one gated subtree — the five settings members lifted
// from `properties`, plus `property_definitions` (the array that lived at
// the root as `type_properties` until v0.32).
//
// Nesting is not tidiness. §2d already put one root `allOf` conditional on
// the schema; five more root fields would be five more, and the eval found
// that models put `type_properties` on non-type documents precisely BECAUSE
// the root had no conditionals — while many constrained decoders do not
// implement `if`/`then` at all. One group is one conditional, and a per-kind
// generated schema includes or omits it in one move.
//
// The same change DROPS the type object's own display and provenance from a
// type document's `properties`. Each key passed the §15 #12 admission test
// individually against a 38,061-document corpus (1,760 type documents);
// the verdicts live on typeProvenanceKeys and the keys that FAILED the test
// are recorded there too.

import (
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The five stored detail keys the type_settings members carry. Named off the
// bundle so a rename there is a compile error here rather than a silent
// un-lift (the §2b rule).
var (
	detailKeyRecommendedLayout = bundle.RelationKeyRecommendedLayout.String()
	detailKeyApiObjectKey      = bundle.RelationKeyApiObjectKey.String()
	detailKeyPluralName        = bundle.RelationKeyPluralName.String()
	detailKeyDefaultTemplateId = bundle.RelationKeyDefaultTemplateId.String()
	detailKeyDefaultViewType   = bundle.RelationKeyDefaultViewType.String()
)

// typeSettingsLiftedDetailKeys is the §2a settings lift list — the single
// source for both directions, like liftedDetailKeys (§2b) and
// relationLiftedDetailKeys (§2d): export writes these keys nowhere but the
// group, and on a TYPE document import refuses their flat spellings in
// `properties`. Unlike the §2d list the refusal is KIND-SCOPED, and that is
// measured, not stylistic: `apiObjectKey` is real data on 9,725 relation and
// 525 relation-option documents, where it stays an ordinary property — the
// group exists on type documents only, so only there is a flat spelling a
// second way to write one fact.
func typeSettingsLiftedDetailKeys() map[string]bool {
	return map[string]bool{
		detailKeyRecommendedLayout: true,
		detailKeyApiObjectKey:      true,
		detailKeyPluralName:        true,
		detailKeyDefaultTemplateId: true,
		detailKeyDefaultViewType:   true,
	}
}

// typeSettingsLiftedKeyRepair names the group member a refused flat spelling
// belongs in — liftedKeyRepair's rule (§2b).
func typeSettingsLiftedKeyRepair(key string) string {
	switch key {
	case detailKeyRecommendedLayout:
		return `"layout": "<a layout name>"`
	case detailKeyApiObjectKey:
		return `"api_key": "<the api key>"`
	case detailKeyPluralName:
		return `"plural_name": "<the plural display name>"`
	case detailKeyDefaultTemplateId:
		return `"default_template": "<a template object id>"`
	case detailKeyDefaultViewType:
		return `"default_view": "<a view type name>"`
	}
	return ""
}

// typeProvenanceKeys are the stored details a TYPE document does not carry:
// the type object's own display and provenance, which describe the install
// rather than the type. Export omits them on type documents and import
// drops them there (stale, not wrong — the transientProperties policy).
//
// Every entry passed the §15 #12 admission test individually, against 1,760
// corpus type documents; the map value records the verdict. Keys that were
// candidates and FAILED the test — they stay in `properties` because they
// carry something real:
//
//   - `isHidden` (626 docs, all true): cannot be proven install-only — an
//     integration can hide a type it minted, and §15 #12 requires proof,
//     not plausibility. Its EMPTY value is already trimmed (systemtrim.go).
//   - `orderId` (343 docs, 130 distinct lexids): the user's own ordering of
//     types in the library. User intent, kept.
//   - `layoutWidth` / `layoutAlign` (40/38 docs, 3/1 non-zero): the display
//     of the type object's OWN page, set by a person where non-zero. Kept.
//   - `featuredRelations` (400 docs): what this type OBJECT features —
//     which differs from `section: "featured"` (what objects OF this type
//     feature) in 361 of 400 corpus cases. Two things, not one. Kept.
//   - `headerRelationsLayout` (51 docs): a real per-type editor setting the
//     group does not model; kept in `properties` rather than half-lifted.
//   - `revision` (1,455 docs): admitted at first on the reasoning that the
//     system "re-runs the bundled migrations from zero and restamps it".
//     It does re-run, and the re-run is not a no-op. systemobjectreviser
//     guards on `bundleRevision <= localObject.GetInt64(revisionKey)`; an
//     absent revision reads 0, the guard stops short-circuiting, and
//     buildDiffDetails then copies the BUNDLED values over the local ones
//     for every key in systemObjectFilterKeys — name, pluralName,
//     recommendedLayout, isHidden, relationMaxCount among them. Measured
//     on the same corpus: of 1,599 installed bundled type documents, 40
//     carry a local `name` the reviser would overwrite (key `relation` is
//     locally "Relation", bundled "Property") and 36 a local plural name.
//     Dropping it silently reverts a user's rename on restore. KEPT.
var typeProvenanceKeys = map[string]string{
	// 1,758 of 1,760 docs, ONE distinct value ("object_type"): derivable
	// from the kind, information-free
	"layout": "one distinct value, derivable from the kind",
	// 1,760 of 1,760, ONE distinct value: same verdict
	"resolvedLayout": "one distinct value, derivable from the kind",
	// 1,608 docs, and every one also carries sourceObject — i.e. it occurs
	// only on installed copies of bundled types, where the bundled table
	// carries the same value
	"smartblockTypes": "install artifact, restated from the bundled table",
	// 1,623 docs: the bundled url this type was installed from — derivable
	// from the type's own key (`_ot<key>`)
	"sourceObject": "install artifact, derivable from the type key",
	// 1,693 docs, values builtin(7) 1,310 · usecase(6) 278 · import(3) 90 ·
	// api(9) 15 — how the INSTALL happened, not what the type is. (An
	// earlier draft read enum 3 as `dragAndDrop`; that is 2, and it occurs
	// zero times. The verdict stands; the value list did not.) On ordinary
	// objects origin is real provenance and stays; the drop is
	// type-documents-only.
	"origin": "install provenance, not the type's definition",
	// 1,627 docs, 1,600 of them the epoch zero (1970-01-01): an install
	// timestamp at best, garbage at median
	"addedDate": "install timestamp, epoch-zero on 98% of the corpus",
	// 1,757 docs, and 1,756 of them hold the document's OWN id — a
	// self-reference, not the pointer-to-nothing an earlier measurement
	// reported (it compared raw values against bare ids while the corpus
	// dump carried `#name` suffixes, so every comparison missed). The drop
	// is safe for a different reason than the one first written down:
	// objecttype.go:264 re-stamps it with WithForcedDetail from the
	// object's own id on every init, so it is a function of the id and
	// cannot survive into a new space anyway. On a SET document `setOf` is
	// the collection's meaning and stays; the drop is type-documents-only.
	"setOf": "the type's own id, re-stamped by WithForcedDetail on every init",
}

// DroppedTypeProvenanceKey reports a stored detail that export omits on a
// TYPE document because it describes the install rather than the type (§2a).
// It is the exported half of the rule, for the round-trip comparator — the
// predicate is the format's own, not a copy, so the comparator and the
// exporter cannot disagree (the miss that produced 1,344 false failures in
// one sweep).
func DroppedTypeProvenanceKey(sbType model.SmartBlockType, key string) bool {
	if !isTypeSmartBlock(sbType) {
		return false
	}
	_, dropped := typeProvenanceKeys[key]
	return dropped
}

// DroppedEmptyTypeSetting reports a stored detail that export omits on a
// TYPE document because it is one of the five lifted settings and its value
// is empty: the group follows the §4 omit-empty canon — a `pluralName` of ""
// (145 corpus docs) and a `defaultTemplateId` of [] (87) say nothing a
// reader could act on, unlike the §2d members, which are the property's
// definition and mirror presence exactly. The comparator consults this
// predicate for the absent-vs-dropped-empty step, like its three siblings.
func DroppedEmptyTypeSetting(sbType model.SmartBlockType, key string, v *types.Value) bool {
	if !isTypeSmartBlock(sbType) || !typeSettingsLiftedDetailKeys()[key] {
		return false
	}
	return isEmptySystemValue(v)
}

// isTypeSmartBlock is the SNAPSHOT-side statement of which kinds are type
// documents, and isTypeKind the DOCUMENT-side one — the same two-halves
// shape as isRelationSmartBlock/isRelationKind (§2d), with the same rule:
// both halves and the schema's gate must name the same kinds, or one side
// emits what the other refuses. `bundled_object_type` is in the set for the
// §2d side-door reason: 0 of 38,061 corpus documents carry it, but the
// schema's `kind` enum offers it beside `object_type` with nothing marking
// it non-authorable, and a kind nothing emits is exactly the kind nobody
// thought to guard.
func isTypeSmartBlock(sbType model.SmartBlockType) bool {
	return sbType == model.SmartBlockType_STType ||
		sbType == model.SmartBlockType_BundledObjectType
}

// isTypeKind reports the kinds whose document IS a type.
func isTypeKind(doc map[string]any) bool {
	kind, _ := doc["kind"].(string)
	return kind == kindNames.name(model.SmartBlockType_STType) ||
		kind == kindNames.name(model.SmartBlockType_BundledObjectType)
}

// typeSettingsOf reads the §2a group off a raw document, for the checks that
// run before it decodes — relationSettingsOf's twin.
func typeSettingsOf(doc map[string]any) (map[string]any, bool) {
	raw, has := doc["type_settings"]
	group, _ := raw.(map[string]any)
	return group, has
}

// typePropertyDefinitionsOf reads the property-definition list off a raw
// document. One reader for every raw-document pass, so none of them can
// keep looking at the pre-v0.32 root location.
func typePropertyDefinitionsOf(doc map[string]any) ([]any, bool) {
	group, _ := typeSettingsOf(doc)
	raw, has := group["property_definitions"]
	list, _ := raw.([]any)
	return list, has
}

// typePropertyDefinitionsPath is the JSON-pointer prefix of one
// property-definition entry — in one place because the document pass, the
// import seam and the PATCH channel must address the same slot identically.
const typePropertyDefinitionsPath = "/type_settings/property_definitions"

//
// ---- export ----
//

// isTypeDoc reports whether this export carries the §2a type_settings group.
func (e *exporter) isTypeDoc() bool {
	return isTypeSmartBlock(e.sbType)
}

// buildTypeSettings renders the §2a group, or nil off a type document. The
// five settings members follow the §4 omit-empty canon (see
// DroppedEmptyTypeSetting for why the §2d mirror does not apply);
// `property_definitions` is present even when empty exactly as the root
// array was — its presence is what tells import to rebuild the four lists.
// An empty group is omitted whole, which can only happen without a property
// resolver: with one, `property_definitions` is always present.
func (e *exporter) buildTypeSettings() *omap {
	if !e.isTypeDoc() {
		// off a type document the five stored keys are ORDINARY properties
		// and stay in `properties` — measured, not assumed: apiObjectKey is
		// real data on 9,725 relation documents. The kind-scoped lift is the
		// whole difference from §2d's unconditional one.
		return nil
	}
	g := &omap{}
	g.setNonEmpty("layout", e.typeSettingEnumValue(detailKeyRecommendedLayout, "/type_settings/layout",
		layoutNames.has,
		func(n float64) string { return layoutNames.name(model.ObjectTypeLayout(int32(n))) }))
	g.setNonEmpty("api_key", e.typeSettingString(detailKeyApiObjectKey, "/type_settings/api_key"))
	g.setNonEmpty("plural_name", e.typeSettingString(detailKeyPluralName, "/type_settings/plural_name"))
	g.setNonEmpty("default_template", e.typeSettingTemplate())
	g.setNonEmpty("default_view", e.typeSettingEnumValue(detailKeyDefaultViewType, "/type_settings/default_view",
		viewTypeNames.has,
		func(n float64) string { return viewTypeNames.name(model.BlockContentDataviewViewType(int32(n))) }))
	if tp := e.buildTypeProperties(); tp != nil {
		g.set("property_definitions", tp) // present even when empty (§2a)
	}
	return g
}

// typeSettingString reads a string-valued setting; a value of another shape
// is dropped with a warning — the §2d include_time policy: there is no way
// to write it in this member.
func (e *exporter) typeSettingString(detailKey, path string) string {
	v := e.detail(detailKey)
	if v == nil {
		return ""
	}
	switch k := v.GetKind().(type) {
	case *types.Value_StringValue:
		return k.StringValue
	case *types.Value_NullValue:
		return "" // a stored null carries nothing a string member could
	default:
		e.warn(path, "%s %v is not a string and is dropped — there is no way to write it (§2a)",
			detailKey, protoValueToJSON(v))
		return ""
	}
}

// typeSettingEnumValue reads a name-over-number setting: a stored number in
// the enum renders as its name, a number outside it passes through raw (the
// layout-key policy `properties` has always applied), a stored string the
// vocabulary knows passes as that name, and anything else drops with a
// warning — an unknown string may not be written, because the member's
// validation refuses unknown names (a typo silently landing on a
// number-format detail is the disease the layout rule exists for) and
// Marshal never emits what Validate rejects (§11 I1).
func (e *exporter) typeSettingEnumValue(detailKey, path string, known func(string) bool, name func(float64) string) any {
	v := e.detail(detailKey)
	if v == nil {
		return nil
	}
	switch k := v.GetKind().(type) {
	case *types.Value_NumberValue:
		if n := name(k.NumberValue); n != "" {
			return n
		}
		return k.NumberValue
	case *types.Value_StringValue:
		if known(k.StringValue) {
			return k.StringValue
		}
		e.warn(path, "%s %q is not a name this member can hold and is dropped — there is no way to write it (§2a)",
			detailKey, k.StringValue)
		return nil
	case *types.Value_NullValue:
		return nil
	default:
		e.warn(path, "%s %v is neither a name nor a number and is dropped — there is no way to write it (§2a)",
			detailKey, protoValueToJSON(v))
		return nil
	}
}

// typeSettingTemplate reads the stored defaultTemplateId — a LIST in every
// corpus document (142 of 142; 87 empty, 55 with one entry) — as the scalar
// object reference the member holds. A second entry has no written form and
// drops with a warning: 0 of 1,760 corpus type documents carry one, and a
// scalar member is what keeps the group readable as "the default template"
// rather than a list with a phantom order.
func (e *exporter) typeSettingTemplate() string {
	v := e.detail(detailKeyDefaultTemplateId)
	if v == nil {
		return ""
	}
	entries := valueStringList(v)
	if len(entries) == 0 {
		return ""
	}
	if len(entries) > 1 {
		e.warn("/type_settings/default_template",
			"%s holds %d entries; the member is the ONE default template, so only the first is written (§2a)",
			detailKeyDefaultTemplateId, len(entries))
	}
	return e.objectRef(entries[0])
}

//
// ---- import ----
//

// jsonTypeSettings is the decoded `type_settings` group (§2a). Layout and
// DefaultView decode as `any` because each member is a name over a stored
// number, with a raw number passing through for a value outside the enum —
// the layout-key policy `properties` has always applied.
type jsonTypeSettings struct {
	Layout          any                 `json:"layout"`
	ApiKey          string              `json:"api_key"`
	PluralName      string              `json:"plural_name"`
	DefaultTemplate string              `json:"default_template"`
	DefaultView     any                 `json:"default_view"`
	TypeProps       *[]jsonTypeProperty `json:"property_definitions"` // pointer: [] and absent differ (§2a)
}

// applyTypeSettings writes the stored keys the §2a group members stand for,
// and hands `property_definitions` to the list machinery. A member the
// document omits writes nothing.
func (imp *importer) applyTypeSettings(details *types.Struct, sbType model.SmartBlockType) error {
	ts := imp.doc.TypeSettings
	if ts == nil {
		return nil
	}
	if !isTypeSmartBlock(sbType) {
		// the schema keeps the group off every other kind (§2a); a caller
		// that skipped Validate gets the same silence as §2d
		return nil
	}
	if v := typeSettingDetailValue(ts.Layout, func(name string) (float64, bool) {
		if !layoutNames.has(name) {
			return 0, false
		}
		return float64(layoutNames.value(name)), true
	}); v != nil {
		details.Fields[detailKeyRecommendedLayout] = v
	}
	if ts.ApiKey != "" {
		details.Fields[detailKeyApiObjectKey] = &types.Value{
			Kind: &types.Value_StringValue{StringValue: ts.ApiKey}}
	}
	if ts.PluralName != "" {
		details.Fields[detailKeyPluralName] = &types.Value{
			Kind: &types.Value_StringValue{StringValue: ts.PluralName}}
	}
	if ts.DefaultTemplate != "" {
		// the stored shape is a list; the member is its one entry
		details.Fields[detailKeyDefaultTemplateId] = &types.Value{
			Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: []*types.Value{
				{Kind: &types.Value_StringValue{StringValue: imp.objectRef(ts.DefaultTemplate)}},
			}}}}
	}
	if v := typeSettingDetailValue(ts.DefaultView, func(name string) (float64, bool) {
		if !viewTypeNames.has(name) {
			return 0, false
		}
		return float64(viewTypeNames.value(name)), true
	}); v != nil {
		details.Fields[detailKeyDefaultViewType] = v
	}
	return imp.applyTypeProperties(details)
}

// typeSettingDetailValue inverts a name-over-number member: a known name
// becomes its stored number, a number passes through raw, and nil stays
// nothing. An unknown string is unreachable off a validated document — the
// member's semantic check refuses it by name — and passes through verbatim
// only as the backstop for a caller that skipped Validate.
func typeSettingDetailValue(v any, value func(string) (float64, bool)) *types.Value {
	switch x := v.(type) {
	case string:
		if x == "" {
			return nil
		}
		if n, known := value(x); known {
			return &types.Value{Kind: &types.Value_NumberValue{NumberValue: n}}
		}
		return &types.Value{Kind: &types.Value_StringValue{StringValue: x}}
	case float64:
		return &types.Value{Kind: &types.Value_NumberValue{NumberValue: x}}
	}
	return nil
}

// definitionIdentityIssue reports a type or relation document that names no
// `key`. Such a document defines something and says nothing about WHAT: the
// key is the stored identity every other document addresses it by, and
// without one the definition lands on nothing an object can be typed by or a
// property value can resolve to.
//
// A WARNING and not a refusal, because §11 I1 forbids emitting what Validate
// rejects and a snapshot's stored key is untrusted — the hostile corpus
// builds a type whose stored key is the empty string precisely because a
// 36,808-object sweep falsified a closed charset over that slot. Export must
// stay able to write whatever a snapshot holds.
//
// It earns its place under §12's first test — it catches something silent.
// Four of four schema-only runs in the small-model authoring eval wrote a
// type document with `"type": "podcast_episode"` and no `key` at all; every
// one validated, imported and round-tripped, and the type came back with no
// identity. Nothing said a word.
func definitionIdentityIssue(doc map[string]any, warn func(path, format string, args ...any)) {
	kind, _ := doc["kind"].(string)
	var what string
	switch {
	case isRelationKind(doc):
		what = "relation"
	case kind == kindNames.name(model.SmartBlockType_STType) ||
		kind == kindNames.name(model.SmartBlockType_BundledObjectType):
		what = "type"
	default:
		return
	}
	if key, _ := doc["key"].(string); key != "" {
		return
	}
	warn("/key", "a %s document defines something and names no `key` — the stored "+
		"identity every other document addresses it by. Without one the definition "+
		"imports with no identity: nothing can be typed by it, and no property value "+
		"resolves to it (§2a, §2d)", what)
}
