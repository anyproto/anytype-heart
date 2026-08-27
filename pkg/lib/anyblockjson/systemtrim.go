package anyblockjson

// systemtrim.go — the seven system-stamped properties whose EMPTY value is
// not written (§3, §15 #12).
//
// §3's rule is that the presence of a property key is meaningful: it records
// that the property was set on this object, so values are written verbatim,
// empty ones included. That rule earns its keep for anything a person
// touched — an empty `tag` list is a person having cleared the tags — and it
// costs nothing on an ordinary page, which carries few system keys.
//
// It costs a great deal on the documents this format exists to be read:
// measured over 36,967 production documents, empty system-stamped values are
// 1.13% of all bytes but the distribution is bimodal — p50 1.21%, p90
// 13.55%, max 23.22% — because `relationDefaultValue`, `relationMaxCount`
// and their neighbours appear only on RELATION and TYPE documents. An agent
// reading a space's SCHEMA reads exactly the documents that pay ~20%.
//
// So the carve-out is a WHITELIST, not a rule over a category. The blanket
// form — every key in bundle.SystemRelations minus an exception list — was
// declined: it admits every system relation added in future sight-unseen,
// and it buys almost nothing, because the saving is top-heavy. These seven
// keys carry ~50% of it; the thirty-key tail carries 3.6% of 1.13%, which is
// 0.04% of all bytes. An explicit list gets nearly all of the benefit with
// every omission vetted.
//
// Admission test, applied to each key below: does anything distinguish
// present-and-empty from absent? For a system-stamped flag whose empty value
// IS the proto zero, nothing does — every reader reaches it through a typed
// getter that answers the same for both. Keys that failed the test and are
// deliberately absent from this list:
//
//   - `relationFormat` — 0 is `longtext`, a real format, not "unset" (§15 #14).
//   - `relationFormatObjectTypes` — list-valued and user-intent-bearing, the
//     same empty-vs-absent shape GO-7451 settled the other way for a type's
//     recommended lists: an empty list is how a cleared set is expressed, so
//     it has to survive.
//   - `featuredRelations` was excluded here for the same reason, and the
//     reason was wrong: an empty list there is not a cleared set but the
//     LAYOUT SYNCER's output (layout/syncer.go), since no UI sets a
//     per-object featured list. The key is now deprecated outright and never
//     reaches this rule.
//
// This is a state normalization, recorded in `N(S)` (§11): such a key comes
// back ABSENT. DroppedEmptySystemProperty exists so the round-trip
// comparator can suppress exactly that step — the rule lives here, in one
// place, so the comparator and the exporter cannot disagree about it.

import (
	"github.com/gogo/protobuf/types"
)

// trimmedWhenEmpty maps each admitted stored key to why its empty value says
// nothing. Every entry is a system relation whose empty value is the proto
// zero AND whose zero is the semantic default, so a reader that asks gets the
// same answer whether the key is absent or present-and-empty.
var trimmedWhenEmpty = map[string]string{
	"relationDefaultValue":  "no default value: empty IS the absence of one",
	"relationReadonlyValue": "false: the relation is writable, the default",
	"revision":              "0: no bundled revision recorded",
	"isHidden":              "false: the object is visible, the default",
	"isHiddenDiscovery":     "false: the object is discoverable, the default",
	"isArchived":            "false: the object is not archived, the default",
	"relationMaxCount":      "0: unlimited, the default",
}

// DroppedEmptySystemProperty reports a stored detail that export omits
// because it is one of the admitted system-stamped keys (§15 #12) and its
// value is empty. It is the exported half of the rule, for the round-trip
// comparator; the predicate is the format's own, not a copy of it.
//
// Scoped to empty-and-admitted and nothing else: a NON-empty value on the
// same key still reports as loss if it ever goes missing, and a key outside
// the list reports whatever it did before.
func DroppedEmptySystemProperty(key string, v *types.Value) bool {
	_, admitted := trimmedWhenEmpty[key]
	return admitted && isEmptySystemValue(v)
}

// isEmptySystemValue is the emptiness rule the export path applies, in one
// place so the comparator and the builder cannot disagree about which values
// are empty. A bool is spelled out rather than falling through, because
// `false` is precisely the value this rule is about — unlike the icon/cover
// rule beside it (liftedValueIsSource), where no source is a bool.
func isEmptySystemValue(v *types.Value) bool {
	switch k := v.GetKind().(type) {
	case *types.Value_StringValue:
		return k.StringValue == ""
	case *types.Value_NumberValue:
		return k.NumberValue == 0
	case *types.Value_BoolValue:
		return !k.BoolValue
	case *types.Value_ListValue:
		return len(k.ListValue.GetValues()) == 0
	case *types.Value_StructValue:
		return len(k.StructValue.GetFields()) == 0
	case *types.Value_NullValue:
		return true
	}
	return v.GetKind() == nil
}
