package anyblockjson

// relationformat.go implements §2d: the three relation-definition envelope
// fields of a `kind: "relation"` document — `format`, `include_time`,
// `object_types`.
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

// relationLiftedDetailKeys is the §2d lift list, and like liftedDetailKeys
// (§2b) it is the single source of truth for both directions: export writes
// these keys nowhere but the envelope, and import refuses them in
// `properties` (deniedPropertyKey reads this same set). The refusal is
// unconditional across kinds — there is no second way to write a relation's
// format — and export honours that everywhere: on a kind with no §2d fields
// to lift into, a present key is dropped with a warning rather than written
// as a property (never observed: 0 of 27,444 non-relation documents carry
// any of the three).
func relationLiftedDetailKeys() map[string]bool {
	return map[string]bool{
		detailKeyRelationFormat:            true,
		detailKeyRelationFormatIncludeTime: true,
		detailKeyRelationFormatObjectTypes: true,
	}
}

// relationLiftedKeyRepair names the envelope field a refused flat spelling
// belongs in — liftedKeyRepair's rule (§2b): the refusal is worth twice as
// much said as a repair, because unlike an internal key there IS something
// to write instead.
func relationLiftedKeyRepair(key string) string {
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

// isRelationDoc reports whether this export carries the §2d envelope fields.
// Only kind "relation": a bundled_relation snapshot is never exported (0 in
// the corpus) and widening the conditional would widen the schema for a
// population that does not exist.
func (e *exporter) isRelationDoc() bool {
	return e.sbType == model.SmartBlockType_STRelation
}

// buildRelationEnvelope writes the three §2d fields onto the envelope, or —
// on a kind that has no such fields — reports any stored value the lift
// leaves nowhere to go. Field presence mirrors stored-key presence exactly,
// value included (false, `[]`, null): the §4 omit-empty canon stops at these
// three because they are the property's definition, and §15 #14 scoped this
// change to the SPELLING, deliberately leaving present-and-empty alone so
// the snapshot round-trips unchanged and the comparator needs no new rule.
func (e *exporter) buildRelationEnvelope(doc *omap) error {
	if !e.isRelationDoc() {
		for _, key := range []string{detailKeyRelationFormat,
			detailKeyRelationFormatIncludeTime, detailKeyRelationFormatObjectTypes} {
			if e.detail(key) != nil {
				e.warn("/properties", "%q describes a relation and this is not a relation document; "+
					"the value is dropped — it has no §2d field here and `properties` refuses the key", key)
			}
		}
		return nil
	}

	name, err := e.relationFormatName()
	if err != nil {
		return err
	}
	doc.set("format", name)

	if v := e.detail(detailKeyRelationFormatIncludeTime); v != nil {
		switch k := v.GetKind().(type) {
		case *types.Value_BoolValue:
			doc.set("include_time", k.BoolValue)
		case *types.Value_NullValue:
			// a stored null is a value — the key was set (§3) — and 80
			// production relations hold exactly this, so dropping it would
			// change the snapshot on the way round
			doc.set("include_time", nil)
		default:
			e.warn("/include_time", "includeTime %v is neither a boolean nor null and is dropped — "+
				"there is no way to write it (§2d)", protoValueToJSON(v))
		}
	}

	if v := e.detail(detailKeyRelationFormatObjectTypes); v != nil {
		switch v.GetKind().(type) {
		case *types.Value_ListValue, *types.Value_StringValue:
			// present even when empty — an empty list is a cleared target
			// set, the same user-intent reading that kept
			// relationFormatObjectTypes off the §15 #12 trim whitelist
			doc.set("object_types", stringsToAny(e.typeSlugs(e.relationTargetKeys())))
		case *types.Value_NullValue:
			doc.set("object_types", nil)
		default:
			e.warn("/object_types", "relationFormatObjectTypes %v is not a list and is dropped — "+
				"there is no way to write it (§2d)", protoValueToJSON(v))
		}
	}
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
// type-key census (seedTypeTermLedger) and buildRelationEnvelope both read
// it — the same one-build rule as iconField (§2b).
//
// Translation is per entry and refuses nothing: a type object id inverts
// through the TypeResolver capability when the resolver carries it, and
// everything else — a bare type key the legacy import paths stored directly
// (21 production entries), an id the resolver no longer serves, the
// `_missing_object` sentinel — passes through verbatim, its own address
// (§3). Passing through rather than dropping is what makes the offline
// round trip byte-exact and keeps this a spelling change: §2a's export may
// drop a dangling recommended-list entry because the resolver is the only
// thing that knows what the id meant, but here the stored value IS the
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
		out = append(out, entry)
	}
	e.relTargets = out
	return out
}

//
// ---- import ----
//

// applyRelationEnvelope writes the stored keys the §2d fields stand for.
// Presence mirrors presence: a field the document omits writes nothing, so
// the details that came out are the details that go back in.
func (imp *importer) applyRelationEnvelope(details *types.Struct, sbType model.SmartBlockType) error {
	if sbType != model.SmartBlockType_STRelation {
		// the schema keeps the three fields off every other kind (§2d), so
		// there is nothing here to read — Unmarshal validates before it
		// decodes, deliberately (§12 I2)
		return nil
	}
	doc := imp.doc
	if doc.Format != "" {
		// the name resolves per key, exactly as a type_properties entry's
		// format does (§3): "text" names both stored text formats, and the
		// relation's own envelope `key` is what disambiguates — a bundled
		// short-text relation (name, globalName, …) keeps its stored format
		// across a round trip even though the document never spells it
		f := declaredFormatWith(imp.opts, doc.Key, doc.Format)
		details.Fields[detailKeyRelationFormat] = &types.Value{
			Kind: &types.Value_NumberValue{NumberValue: float64(f)}}
	}
	if raw := doc.IncludeTime; len(raw) > 0 {
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
	if raw := doc.TargetTypes; len(raw) > 0 {
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
			slotPath := fmt.Sprintf("/object_types/%d", j)
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

// relationFormatSlotIssue words the missing-`format` verdict on a relation
// document — missingFormatIssue's trade (§2b) at the §2d slot: `required`
// can say a member is missing but not what the choices are, and the author
// most likely to hit it is holding a pre-v0.31 document whose format lives
// in `properties` as a raw number. The kind is read RAW, exactly as the
// schema's `if` reads it, so the two verdicts cannot disagree about which
// documents owe the field. The names are read out of the published schema
// (propertyFormatEnum), never restated.
func relationFormatSlotIssue(doc map[string]any, r *keySlotReport) {
	if kind, _ := doc["kind"].(string); kind != kindNames.name(model.SmartBlockType_STRelation) {
		return
	}
	if _, has := doc["format"]; has {
		return
	}
	names := propertyFormatEnum()
	if len(names) == 0 {
		return
	}
	msg := fmt.Sprintf("missing property 'format': a relation document states "+
		"the format of the property it defines — one of %s (§2d)", quotedList(names))
	// the migration hint, on the same reasoning as `refs` (§10): a pre-v0.31
	// document is exactly one that has no `format` and spells
	// `relation_format` in properties, and told only that a member is
	// missing, the obvious wrong repair is to invent one while leaving the
	// raw number where it sits
	if props, _ := doc["properties"].(map[string]any); props != nil {
		if _, legacy := props["relation_format"]; legacy {
			msg += `. This document spells "relation_format" inside properties — the pre-v0.31 ` +
				`form: replace that raw number with its name here, on the envelope`
		}
	}
	r.rejectValueAt("", msg)
}

// relationEnvelopeIssues runs the §2d checks the schema cannot express: a
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
func relationEnvelopeIssues(doc map[string]any, warn func(path, format string, args ...any)) {
	if kind, _ := doc["kind"].(string); kind != kindNames.name(model.SmartBlockType_STRelation) {
		return // the schema refuses the fields on every other kind
	}
	format, _ := doc["format"].(string)
	if format == "" {
		return // required and missing: the schema's error already says so
	}
	if v, has := doc["include_time"]; has && format != "date" {
		if b, isBool := v.(bool); isBool && b {
			warn("/include_time", "include_time is only meaningful on date, not %q — "+
				"it is carried but nothing reads it (§2d)", format)
		}
	}
	if v, has := doc["object_types"]; has && format != "objects" && format != "files" {
		if list, isList := v.([]any); isList && len(list) > 0 {
			warn("/object_types", "object_types is only meaningful on objects/files, not %q — "+
				"it is carried but nothing reads it (§2d)", format)
		}
	}
	// a `properties` member spelling one of the three FIELD names on a
	// relation document is almost certainly the envelope field written in the
	// wrong container — the exact shape the 9-of-9 eval failures wrote,
	// minus the missing envelope field the schema now catches. It stays a
	// WARNING, because the spelling is a legitimate custom property key (a
	// media space really can have a "Format" column) and a relation object
	// carrying one must stay exportable (I1); but silent it was the §2d bug
	// reborn, one confused generation later.
	if props, _ := doc["properties"].(map[string]any); props != nil {
		for _, member := range []string{"format", "include_time", "object_types"} {
			if _, has := props[member]; has {
				warn("/properties/"+member, "on a relation document %q names a CUSTOM property, "+
					"not the relation's own %s — that lives on the envelope (§2d); "+
					"drop this member unless a property literally named %q is meant",
					member, member, member)
			}
		}
	}
}
