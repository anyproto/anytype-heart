package anyblockjson

// relationformat.go implements §2d: the `property_settings` group of a
// `kind: "property"` document — one propertyDefinition (§2e), whose three
// travelling members are `format`, `include_time`, `object_types`. v0.31
// put the three at the document root; v0.32 regrouped them, because the
// dictionary entry and a type's property-definition entry are groups
// holding the same shape and two patterns for one idea is §15 #14 one
// level up; v0.38 renamed the group (and the kinds) off "relation": the
// product calls these things properties, and the format already did in
// every neighbouring name.
//
// A relation object IS a property definition, and until this lift it was the
// one document that could not state its own format in the format's own
// vocabulary: `properties` carried `relation_format: 100` — a raw enum
// number — while a `type_properties` entry three sections up spelled the
// same fact `format: "objects"`. One concept, two spellings, in one format
// (§15 #14). Worse, the raw spelling was a live trap: in a 198-run
// small-model eval, 9 of 9 attempts wrote `properties: {"format": "number"}`,
// which VALIDATED — inside `properties` every key is a property spelling, so
// that line means "a custom property named format" — and imported as exactly
// that, leaving the relation with no relationFormat at all: longtext forever,
// silently. The container was the problem, not the word.
//
// The precedent is §2b: stored keys lifted into typed envelope fields, with
// the flat spellings refused where they used to sit — including the refusal,
// because a format with two legal spellings for one thing, one of which a
// small model has seen far more of in training data, defeats the whole
// point. Like the nine §2b keys, all three relations are `hidden: true`, so
// no property row loses the presence §3 makes meaningful — but unlike §2b
// the envelope fields mirror stored presence EXACTLY (false, `[]` and null
// all travel): these are the definition of the property, not decoration, and
// the §15 #14 verdict was to fix the SPELLING and leave the emptiness
// collapse to its own change. Measured over 38,061 production documents
// (10,617 of them relation documents): every one carries `relationFormat`,
// so requiring `format` refuses nothing real; `include_time` is true only on
// dates (543 of 9,035 present) and null on 80; `object_types` is non-empty
// only on objects/files (1,089 + 167 of 10,159 present); and none of the
// three keys occurs on any other kind, so the unconditional refusal costs
// nothing.

import (
	"fmt"
	"math"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The three stored detail keys the §2d envelope fields carry. Named off the
// bundle so a rename there is a compile error here rather than a silent
// un-lift (the §2b rule).
var (
	detailKeyRelationFormat            = bundle.RelationKeyRelationFormat.String()
	detailKeyRelationFormatIncludeTime = bundle.RelationKeyRelationFormatIncludeTime.String()
	detailKeyRelationFormatObjectTypes = bundle.RelationKeyRelationFormatObjectTypes.String()
)

// propertySettingsLiftedDetailKeys is the §2d lift list, and like liftedDetailKeys
// (§2b) it is the single source of truth for both directions: export writes
// these keys nowhere but the envelope, and import refuses them in
// `properties` (deniedPropertyKey reads this same set). The refusal is
// unconditional across kinds — there is no second way to write a relation's
// format — and export honours that everywhere: on a kind with no §2d fields
// to lift into, a present key is dropped with a warning rather than written
// as a property (never observed: 0 of 27,444 non-relation documents carry
// any of the three).
func propertySettingsLiftedDetailKeys() map[string]bool {
	return map[string]bool{
		detailKeyRelationFormat:            true,
		detailKeyRelationFormatIncludeTime: true,
		detailKeyRelationFormatObjectTypes: true,
	}
}

// propertySettingsLiftedKeyRepair names the property_settings member a refused flat
// spelling belongs in — liftedKeyRepair's rule (§2b): the refusal is worth
// twice as much said as a repair, because unlike an internal key there IS
// something to write instead.
func propertySettingsLiftedKeyRepair(key string) string {
	switch key {
	case detailKeyRelationFormat:
		return `"format": "<a §3 format name>"`
	case detailKeyRelationFormatIncludeTime:
		return `"include_time": true|false`
	case detailKeyRelationFormatObjectTypes:
		return `"object_types": ["<type key>", …]`
	}
	return ""
}

// TypeResolver translates between type OBJECT ids — what the store keeps in
// `relationFormatObjectTypes` (objectcreator.fillRelationFormatObjectTypes
// rewrites bundled urls to derived ids at creation) — and stored type KEYS,
// which are what every type-key slot in this format spells (§2a, §2d). It is
// an optional capability of Options.ResolveProperties, discovered by type
// assertion, rather than a fourth resolver field: the resolver that already
// answers PropertyById is the one with the space listing this mapping falls
// out of (storeresolver fills keyById from the same bounded query), and a
// caller without it keeps a well-defined degradation — entries pass through
// verbatim in both directions, each its own address (§3), so an offline
// round trip is byte-exact and only a resolver-wired one translates.
//
// Both directions exist so the translation is an inverse rather than a
// one-way normalization: export turns the stored ids into keys, import turns
// the keys back into this space's ids — the same policy applyTypeProperties
// applies to property definitions via PropertyId — and a key the reader's
// space does not serve stays a key, for the wiring to reconcile (§2a).
type TypeResolver interface {
	TypeKeyById(id string) (string, bool)
	TypeIdByKey(key string) (string, bool)
}

//
// ---- export ----
//

// isPropertyDoc reports whether this export carries the §2d envelope fields.
// It is the snapshot-side half of isPropertyKind and MUST list the same
// kinds, because the schema now requires `format` on all three: an export
// that lifted for fewer than it validates for would emit a document its own
// Validate rejects (§11 I1) — which is exactly what happened when only the
// document side was widened, on `bundled_property`.
//
// `sub_object` is NOT one of them. It is deprecated and out of the format's
// support surface by decision, not by measurement — 0 of 38,061 corpus
// documents carry it either way, so nothing observable turns on it; what
// turns on it is that a deprecated kind must not acquire a new obligation
// in a format about to freeze.
func (e *exporter) isPropertyDoc() bool {
	return isPropertySmartBlock(e.sbType)
}

// isPropertySmartBlock is the SNAPSHOT-side statement of which kinds are
// property documents, and isPropertyKind is the DOCUMENT-side one. All three lists —
// these two and the schema's `if` — must name the same kinds: the schema
// requires `format` on each of them, so a half that lifts for fewer than the
// schema validates for breaks §11 I1 in one direction and drops the
// definition in the other. Both breaks happened when only one list was
// widened.
func isPropertySmartBlock(sbType model.SmartBlockType) bool {
	return sbType == model.SmartBlockType_STRelation ||
		sbType == model.SmartBlockType_BundledRelation
}

// buildPropertySettings writes the `property_settings` group — one
// propertyDefinition, the three §2d members that travel today — or, on a
// kind that has no such group, reports any stored value the lift leaves
// nowhere to go. Member presence mirrors stored-key presence exactly, value
// included (false, `[]`, null): the §4 omit-empty canon stops at these three
// because they are the property's definition, and §15 #14 scoped the v0.31
// change to the SPELLING, deliberately leaving present-and-empty alone so
// the snapshot round-trips unchanged and the comparator needs no new rule.
// v0.32 regrouped the three off the root — churn on freshly shipped fields,
// accepted deliberately: the dictionary entry and the type's
// property-definition entry are groups holding the same shape, and two
// patterns for one idea is the §15 #14 disease again, one level up.
func (e *exporter) buildPropertySettings(doc *omap) error {
	if !e.isPropertyDoc() {
		for _, key := range []string{detailKeyRelationFormat,
			detailKeyRelationFormatIncludeTime, detailKeyRelationFormatObjectTypes} {
			if e.detail(key) != nil {
				e.warn("/properties", "%q describes a property definition and this is not a property document; "+
					"the value is dropped — it has no §2d member here and `properties` refuses the key", key)
			}
		}
		return nil
	}

	name, err := e.relationFormatName()
	if err != nil {
		return err
	}
	group := &omap{}
	group.set("format", name)

	if v := e.detail(detailKeyRelationFormatIncludeTime); v != nil {
		switch k := v.GetKind().(type) {
		case *types.Value_BoolValue:
			group.set("include_time", k.BoolValue)
		case *types.Value_NullValue:
			// a stored null is a value — the key was set (§3) — and 80
			// production relations hold exactly this, so dropping it would
			// change the snapshot on the way round
			group.set("include_time", nil)
		default:
			e.warn("/property_settings/include_time", "includeTime %v is neither a boolean nor null and is dropped — "+
				"there is no way to write it (§2d)", protoValueToJSON(v))
		}
	}

	if v := e.detail(detailKeyRelationFormatObjectTypes); v != nil {
		switch v.GetKind().(type) {
		case *types.Value_ListValue, *types.Value_StringValue:
			// present even when empty — an empty list is a cleared target
			// set, the same user-intent reading that kept
			// relationFormatObjectTypes off the §15 #12 trim whitelist
			group.set("object_types", stringsToAny(e.typeSlugs(e.relationTargetKeys())))
		case *types.Value_NullValue:
			group.set("object_types", nil)
		default:
			e.warn("/property_settings/object_types", "relationFormatObjectTypes %v is not a list and is dropped — "+
				"there is no way to write it (§2d)", protoValueToJSON(v))
		}
	}
	doc.set(memberPropertySettings, group)
	return nil
}

// relationFormatName renders the stored relationFormat as its §3 name. The
// reading mirrors what every consumer of this detail does — int32 of the
// number, absent and null both the proto zero, longtext — so the document
// states the format the system actually serves for this relation.
//
// A value that reading cannot name is an ERROR, not a fallback: `format` is
// required on a relation document, so there is nothing to omit, and writing
// "text" for a format that is not text would import as a permanent silent
// format rewrite — the exact disease the lift exists to kill. Failing the
// export instead follows buildDoc's own rule for a smartblock type with no
// kind mapping, and it is corrupt-data-only territory: formatNames is total
// over model.RelationFormat (pinned by TestFormatNames_TotalOverModelEnum),
// and all 10,617 production relation documents carry an in-enum integer.
func (e *exporter) relationFormatName() (string, error) {
	v := e.detail(detailKeyRelationFormat)
	var n float64
	switch k := v.GetKind().(type) {
	case nil, *types.Value_NullValue:
		n = 0
	case *types.Value_NumberValue:
		n = k.NumberValue
	default:
		return "", fmt.Errorf("relation format %v is not a number: this document cannot state "+
			"what it defines (§2d)", protoValueToJSON(v))
	}
	if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > math.MaxInt32 {
		return "", fmt.Errorf("relation format %v is outside the format enum: this document "+
			"cannot state what it defines (§2d)", n)
	}
	name := formatName(model.RelationFormat(int32(n)))
	if name == "" {
		return "", fmt.Errorf("relation format %v has no §3 name: this document cannot state "+
			"what it defines (§2d)", n)
	}
	return name, nil
}

// relationTargetKeys is the stored relationFormatObjectTypes list with each
// entry translated to the stored type KEY it names, memoized because the
// type-key census (seedTypeTermLedger) and buildPropertySettings both read
// it — the same one-build rule as iconField (§2b).
//
// Translation is per entry: a type object id inverts through the
// TypeResolver capability when the resolver carries it, and a bare type key
// the legacy import paths stored directly (21 production entries) passes
// through verbatim, its own address (§3) — a key is vocabulary, and a
// vocabulary miss is never evidence of nonexistence. What no longer passes
// (v0.44) is an entry the SPACE's own store disowns (§9): the
// `_missing_object` sentinel, and an object id the wired existence
// capability says names no row — 56 production properties carry one, type
// ids from the account where a shipped use case was AUTHORED, and an object
// id differs in every space while a key does not. Both drop, the real id
// with a warning naming it; `object_types` is a list, and a list expresses
// absence by being shorter. The predicate is DroppedMissingObjectRef,
// shared with snapshotdiff, so the comparator drops exactly what export
// drops. Without the capability — package-only, offline — everything still
// passes through verbatim and the round trip stays byte-exact: an id the
// store merely could not be asked about is still the stored value's
// meaning, and a backup format that deletes it on export is disqualifying.
func (e *exporter) relationTargetKeys() []string {
	if e.relTargetsBuilt {
		return e.relTargets
	}
	e.relTargetsBuilt = true
	entries := valueStringList(e.detail(detailKeyRelationFormatObjectTypes))
	tr, _ := e.opts.ResolveProperties.(TypeResolver)
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if tr != nil {
			if key, ok := tr.TypeKeyById(entry); ok && key != "" {
				out = append(out, key)
				continue
			}
		}
		if e.droppedMissingListEntry("/property_settings/object_types", entry) {
			continue
		}
		out = append(out, entry)
	}
	e.relTargets = out
	return out
}

//
// ---- import ----
//

// applyPropertySettings writes the stored keys the §2d group's members stand
// for. Presence mirrors presence: a member the document omits writes
// nothing, so the details that came out are the details that go back in.
func (imp *importer) applyPropertySettings(details *types.Struct, sbType model.SmartBlockType) error {
	if !isPropertySmartBlock(sbType) {
		// the schema keeps the group off every other kind (§2d), so there is
		// nothing here to read — Unmarshal validates before it decodes,
		// deliberately (§12 I2)
		return nil
	}
	rs := imp.doc.PropertySettings
	if rs == nil {
		// unreachable off a validated document — the schema requires the
		// group on both property-document kinds — but Unmarshal must not crash on a
		// snapshot rebuilt by a caller that skipped Validate
		return nil
	}
	if rs.Format != "" {
		// the name resolves per key, exactly as a type_properties entry's
		// format does (§3): "text" names both stored text formats, and the
		// relation's own envelope `key` is what disambiguates — a bundled
		// short-text relation (name, globalName, …) keeps its stored format
		// across a round trip even though the document never spells it
		f := declaredFormatWith(imp.opts, imp.doc.InternalKey, rs.Format)
		details.Fields[detailKeyRelationFormat] = &types.Value{
			Kind: &types.Value_NumberValue{NumberValue: float64(f)}}
	}
	if raw := rs.IncludeTime; len(raw) > 0 {
		if string(raw) == "null" {
			details.Fields[detailKeyRelationFormatIncludeTime] = &types.Value{
				Kind: &types.Value_NullValue{}}
		} else {
			var b bool
			if err := jsonUnmarshal(raw, &b); err != nil {
				return fmt.Errorf("decode include_time: %w", err)
			}
			details.Fields[detailKeyRelationFormatIncludeTime] = &types.Value{
				Kind: &types.Value_BoolValue{BoolValue: b}}
		}
	}
	if raw := rs.TargetTypes; len(raw) > 0 {
		if string(raw) == "null" {
			details.Fields[detailKeyRelationFormatObjectTypes] = &types.Value{
				Kind: &types.Value_NullValue{}}
			return nil
		}
		var slugs []string
		if err := jsonUnmarshal(raw, &slugs); err != nil {
			return fmt.Errorf("decode object_types: %w", err)
		}
		tr, _ := imp.opts.ResolveProperties.(TypeResolver)
		vals := make([]*types.Value, 0, len(slugs))
		for j, slug := range slugs {
			slotPath := fmt.Sprintf("/property_settings/object_types/%d", j)
			// a TYPE key slot (§2d): the document's own legend first, then
			// the vocabulary — and the seam refuses a resolution onto the
			// empty key, which has no written form (§3), the same refusal
			// applyTypeProperties makes for its object_types
			key := imp.typeKey(slug, slotPath)
			if key == "" {
				return &ValidationError{Issues: []Issue{{
					Path:    slotPath,
					Message: unwritableKeyReason("resolved type key", key),
				}}}
			}
			// the store speaks ids; the TypeResolver capability turns the
			// key back into this space's type object id, and a key the
			// space does not serve stays a key for the wiring to reconcile
			// — applyTypeProperties' degradation, on the type namespace
			id := key
			if tr != nil {
				if resolved, ok := tr.TypeIdByKey(key); ok && resolved != "" {
					id = resolved
				}
			}
			vals = append(vals, &types.Value{Kind: &types.Value_StringValue{StringValue: id}})
		}
		details.Fields[detailKeyRelationFormatObjectTypes] = &types.Value{
			Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: vals}}}
	}
	return nil
}

//
// ---- validation ----
//

// propertySettingsOf reads the §2d group off a raw document, for the checks
// that run before it decodes. One reader, so the slot issue and the semantic
// pass cannot disagree about where the group lives.
func propertySettingsOf(doc map[string]any) (map[string]any, bool) {
	raw, has := doc[memberPropertySettings]
	group, _ := raw.(map[string]any)
	return group, has
}

// propertyFormatSlotIssue words the missing-definition verdict on a property
// document — missingFormatIssue's trade (§2b) at the §2d slot: `required`
// can say a member is missing but not what the choices are, and the author
// most likely to hit it is holding an older document whose format lives at
// the root (pre-v0.32) or in `properties` as a raw number (pre-v0.31). The
// kind is read RAW, exactly as the schema's `if` reads it, so the two
// verdicts cannot disagree about which documents owe the group —
// isPropertyKind and the schema's `if` list the same kinds. The names are
// read out of the published schema (propertyFormatEnum), never restated.
func propertyFormatSlotIssue(doc map[string]any, r *keySlotReport) {
	if !isPropertyKind(doc) {
		return
	}
	group, hasGroup := propertySettingsOf(doc)
	if hasGroup {
		if _, has := group["format"]; has {
			return
		}
	}
	names := propertyFormatEnum()
	if len(names) == 0 {
		return
	}
	var msg string
	path := ""
	if hasGroup {
		path = "/property_settings"
		msg = fmt.Sprintf("missing property 'format': a property document states "+
			"the format of the property it defines — one of %s (§2d)", quotedList(names))
	} else {
		msg = fmt.Sprintf("missing property 'property_settings': a property document states "+
			"the definition of the property it IS — at least `format`, one of %s (§2d)", quotedList(names))
	}
	// the migration hints, on the same reasoning as `refs` (§10): each older
	// spelling is exactly one this verdict fires on, and told only that a
	// member is missing, the obvious wrong repair is to invent one while
	// leaving the old spelling where it sits. The hints keep the OLD member
	// names because they describe what the older document in hand SPELLS.
	if _, atRoot := doc["format"]; atRoot && !hasGroup {
		msg += `. This document spells "format" at the root — the pre-v0.32 form: ` +
			`the definition moved into the "property_settings" group, so move ` +
			`"format" (and "include_time"/"object_types" beside it) in there`
	}
	if _, was := doc["relation_settings"]; was && !hasGroup {
		// the pre-v0.38 group name, before the format stopped calling a
		// property a relation anywhere; the members inside are unchanged
		msg += `. This document spells the group "relation_settings" — the pre-v0.38 form: ` +
			`rename the group to "property_settings"`
	}
	if props, _ := doc["properties"].(map[string]any); props != nil {
		if _, legacy := props["relation_format"]; legacy {
			msg += `. This document spells "relation_format" inside properties — the pre-v0.31 ` +
				`form: replace that raw number with its name in property_settings`
		}
		// the OTHER wrong container, and the commoner one: 9 of 9
		// small-model attempts wrote `format` inside `properties`, where it
		// is a custom property named "format" and the relation ends up with
		// no format at all. Told only that a member is missing, the author
		// has no reason to connect it to the member they DID write — and
		// the warning that would say so lives in the semantic pass, which
		// a schema failure never reaches.
		if _, phantom := props["format"]; phantom {
			msg += `. This document spells "format" inside properties, where it names a ` +
				`CUSTOM property rather than the property's own format: move that member ` +
				`into property_settings`
		}
	}
	r.rejectValueAt(path, msg)
}

// propertySettingsIssues runs the §2d checks the schema cannot express: a
// meaningful value against a format that cannot use it. WARNINGS, not
// errors, and the grade is load-bearing (wrongShapeForFormat's reasoning):
// the stored details are not authored — a real relation may carry
// `includeTime` against any format, and 8,375 production relations carry a
// false one against a non-date format — so a refusal would make Marshal
// emit what Validate rejects (I1), and an export that dropped the value
// instead would silently delete stored state. §2a's array can afford its
// errors because it is authored, never lifted from a store.
//
// Only a MEANINGFUL value warns — `include_time: true`, a non-empty
// `object_types` — because a false or empty one against the wrong format
// says nothing the reader would act on, and warning on it would fire on
// most of the corpus (8,375 present-and-false alone), burying the case an
// author can actually fix.
func propertySettingsIssues(doc map[string]any, warn func(path, format string, args ...any)) {
	if !isPropertyKind(doc) {
		return // the schema refuses the group on every other kind
	}
	group, _ := propertySettingsOf(doc)
	format, _ := group["format"].(string)
	if format == "" {
		// required and missing: the schema's error already says so, and it
		// is the one that names the wrong container too
		// (propertyFormatSlotIssue) — this pass never runs on a document
		// the schema rejected, so it cannot be the place that says it.
		return
	}
	propertyPhantomIssues(doc, warn)
	if v, has := group["include_time"]; has && format != "date" {
		if b, isBool := v.(bool); isBool && b {
			warn("/property_settings/include_time", "include_time is only meaningful on date, not %q — "+
				"it is carried but nothing reads it (§2d)", format)
		}
	}
	if v, has := group["object_types"]; has && format != "objects" && format != "files" {
		if list, isList := v.([]any); isList && len(list) > 0 {
			warn("/property_settings/object_types", "object_types is only meaningful on objects/files, not %q — "+
				"it is carried but nothing reads it (§2d)", format)
		}
	}
}

// propertyPhantomIssues reports a `properties` member spelling one of the
// three FIELD names on a relation document. It is almost certainly the
// envelope field written in the wrong container — the exact shape 9 of 9
// small-model attempts wrote, which validated silently and imported as a
// custom property literally named `format`, leaving the relation with no
// relationFormat at all.
//
// It stays a WARNING, because the spelling is a legitimate custom property
// key (a media space really can have a "Format" column) and a relation
// object carrying one must stay exportable (§11 I1).
//
// The kind gate is the property-document SET, not `property` alone. Export
// writes `bundled_property` — 0 of 38,061 corpus
// documents — but the schema's `kind` enum offers it beside `property`
// with nothing marking it non-authorable, and an author who picks it
// walks straight back into the §2d bug with every gate silent. A kind
// nothing emits is exactly the kind nobody thought to guard.
func propertyPhantomIssues(doc map[string]any, warn func(path, format string, args ...any)) {
	props, _ := doc["properties"].(map[string]any)
	if props == nil {
		return
	}
	for _, member := range []string{"format", "include_time", "object_types"} {
		if _, has := props[member]; has {
			warn("/properties/"+member, "on a property document %q names a CUSTOM property, "+
				"not this property's own %s — that lives in property_settings (§2d); "+
				"drop this member unless a property literally named %q is meant",
				member, member, member)
		}
	}
}

// isPropertyKind reports the kinds whose document IS a property definition:
// the one export writes, plus the one the schema's enum offers beside it.
func isPropertyKind(doc map[string]any) bool {
	kind, _ := doc["kind"].(string)
	return kind == kindNames.name(model.SmartBlockType_STRelation) ||
		kind == kindNames.name(model.SmartBlockType_BundledRelation)
}
