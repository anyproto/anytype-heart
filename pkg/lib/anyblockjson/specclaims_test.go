package anyblockjson

// §15 records the designs this format rejected, and — since v0.22 — the
// evidence that rejects them. A rejected design comes back; a rejected design
// whose evidence has quietly stopped being true comes back and WINS.
//
// So the evidence is pinned here rather than left as prose. Each assertion is
// one fact §15 cites by name. A change that falsifies one is not necessarily
// wrong — it means a closed question is open again, and §15 has to be rewritten
// before the change lands.

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// §15.3, the separator design (`#`, deleted at v0.20): a joined key needs a
// byte that appears in neither half, and there is none — the sigil appears
// inside property slugs AND inside option names.
func TestSpecClaim_NoSeparatorSurvivesRealNames(t *testing.T) {
	assert.Equal(t, "c#", bundle.ApiSlug("C#"),
		"a property named C# slugs with the separator INSIDE it")
	assert.Equal(t, "#1_priority", bundle.ApiSlug("#1 priority"),
		"and one can begin with it")
	assert.Equal(t, "c/c++", bundle.ApiSlug("C/C++"),
		"`/` is no better: it survives slugging too")
}

// §15.3, the sigil design (`"@opt-high"` marking a handle in the value).
// Falsified twice over.
func TestSpecClaim_TheSigilIsNotDistinguishableFromData(t *testing.T) {
	// (a) a legal property slug can BEGIN with the sigil, so a marker made of
	// it marks nothing
	assert.Equal(t, "@home", bundle.ApiSlug("@home"))

	// (b) Validate takes bytes and returns an error — no resolver of any kind
	// (§13), so it cannot know whether a /properties value is a select value
	// or an object reference, and must either accept the sigil everywhere
	// (breaking I2) or refuse it where Marshal emits it (breaking I1)
	var _ func(data []byte) error = Validate

	// (c) and export's own deep links are not the counter-example they look
	// like: the id is percent-encoded, so a leading `@` never reaches the wire
	assert.Contains(t, objectLinkDest("@miovm"), "%40",
		"objectLinkDest percent-encodes, so its output never starts a value with the sigil")
}

// §15.3, the `{name, id}` value-pair design. It was believed to be a
// format-only change whose byte cost could be paid by the store already
// knowing each option's key. The store does not know it.
func TestSpecClaim_RelationOptionHasNoKeyField(t *testing.T) {
	var names []string
	rt := reflect.TypeOf(model.RelationOption{})
	for i := 0; i < rt.NumField(); i++ {
		names = append(names, rt.Field(i).Name)
	}
	assert.Equal(t, []string{"Id", "Text", "Color", "RelationKey", "OrderId"}, names,
		"a key field here would reopen §15.3's value-pair design")
}

// §15.11, both directions on the §3 chain's store step (3c).
func TestSpecClaim_TheStoreStepIsNeitherRemovableNorPromotable(t *testing.T) {
	// deleting it: the bundled fold knows nothing about a space's custom
	// keys, so a reader without a store resolves a spelling it was just
	// handed to nothing — and mints a duplicate relation beside it
	assert.Empty(t, bundle.RelationKeysByApiFold("Severity"),
		"a custom key's spelling folds to nothing in the bundled table")
	// while the fold DOES answer for a bundled key, which is why step 3b is
	// worth having at all
	assert.Equal(t, []domain.RelationKey{bundle.RelationKeyDueDate}, bundle.RelationKeysByApiFold("due-date"))

	// promoting it: a space may hold a live stored type key `Task` — this
	// format creates one, `{"kind": "object_type", "internal_key": "Task"}` is legal —
	// and a mandatory fold would overrule verbatim-first (§3 step 2) and
	// retype every reference to it onto the bundled Task type
	assert.Equal(t, []domain.TypeKey{bundle.TypeKeyTask}, bundle.TypeKeysByApiFold("Task"))
}

// §15.3's closing note: the sigil designs were largely defended as protecting
// `object_ids` against a dropped legend. There is no such member — object-ref
// compaction was deleted at v0.20 and object references print in full. The
// only `object_ids` in the format is the dataview's own field (§6.2).
func TestSpecClaim_TheOnlyObjectIdsIsTheDataviewField(t *testing.T) {
	var schema map[string]any
	require.NoError(t, json.Unmarshal(schemaJSON, &schema))

	// every `properties` map in the schema that declares an object_ids
	// member, reported by its sibling members — which is what identifies the
	// object it belongs to without depending on how $refs are reached
	var siblings [][]string
	var walk func(node any)
	walk = func(node any) {
		switch n := node.(type) {
		case map[string]any:
			for k, v := range n {
				if k == "properties" {
					if props, ok := v.(map[string]any); ok {
						if _, has := props["object_ids"]; has {
							var names []string
							for name := range props {
								names = append(names, name)
							}
							sort.Strings(names)
							siblings = append(siblings, names)
						}
						for _, sub := range props {
							walk(sub)
						}
						continue
					}
				}
				walk(v)
			}
		case []any:
			for _, v := range n {
				walk(v)
			}
		}
	}
	walk(schema)

	require.Len(t, siblings, 1, "object_ids appears in more than one place: %v", siblings)
	assert.Equal(t, []string{"group_id", "object_ids"}, siblings[0],
		"the only object_ids is the dataview's manual object order (§6.2)")
}

// §15.3's "additive within a version" half: the `{…}` container design argued
// that a legend member could be added later without a version bump. It could
// not — the envelope is closed, and §10 has no additive rule.
func TestSpecClaim_TheEnvelopeIsClosed(t *testing.T) {
	var schema map[string]any
	require.NoError(t, json.Unmarshal(schemaJSON, &schema))
	assert.Equal(t, false, schema["additionalProperties"],
		"an unknown envelope member is refused, so nothing can be added within a version")
}
