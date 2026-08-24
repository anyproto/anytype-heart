package anyblockjson

// propertydefinition_test.go pins the ONE-SHAPE rule: a property is described
// by `$defs/propertyDefinition` wherever it is described, and every home
// REFERENCES that shape rather than restating it — the same discipline
// TestPropertyFormatEnum_MatchesFormatNames applies to the format vocabulary.
// A fourth spelling of "a property definition" is the §15 #14 disease this
// wave exists to end, and a restated member is how one starts: two lists that
// agree today and drift tomorrow.

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// sharedPropertyMembers is the decided propertyDefinition surface: the five
// members every home speaks today plus the five the dictionary lifts
// (description, include_time, max_count, readonly, default_value). The test
// restates it ON PURPOSE — the schema is the implementation and this list is
// the specification, so a member added to one and not the other fails here
// instead of shipping as a home-local extension.
var sharedPropertyMembers = []string{
	"key", "name", "format", "options", "object_types",
	"description", "include_time", "max_count", "readonly", "default_value",
}

// The published schema states the property-definition shape once —
// $defs/propertyDefinition — and each home layers over a $ref to it: its own
// `properties` may only NARROW a shared member (typeProperty pins `format` to
// authorableFormat and `object_types` to a real array) or add the one member
// that belongs to the home rather than the property (`section`). The shared
// shape itself stays open (no `required`, no unevaluated/additional gate), so
// homes can close themselves without the allOf-vs-additionalProperties trap.
//
// How this can fail: drop the $ref from typeProperty and restate the ten
// members locally (the shape validates identically today and drifts
// tomorrow); add an eleventh member to propertyDefinition without adding it
// here; close propertyDefinition itself, which would break every layered
// home at once; or reopen typeProperty by removing its unevaluatedProperties
// gate.
func TestPropertyDefinition_OneSharedShapeThreeHomes(t *testing.T) {
	type schemaNode struct {
		AllOf      []json.RawMessage          `json:"allOf"`
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
		Additional json.RawMessage            `json:"additionalProperties"`
		Uneval     json.RawMessage            `json:"unevaluatedProperties"`
	}
	var schema struct {
		Properties map[string]schemaNode `json:"properties"`
		Defs       map[string]schemaNode `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(SchemaJSON(), &schema))

	def, ok := schema.Defs["propertyDefinition"]
	require.True(t, ok, "the schema must publish $defs/propertyDefinition")

	want := map[string]bool{}
	for _, m := range sharedPropertyMembers {
		want[m] = true
	}
	got := map[string]bool{}
	for m := range def.Properties {
		got[m] = true
	}
	assert.Equal(t, want, got, "propertyDefinition carries the decided ten members, no more, no fewer")

	// the shared shape is the extension point, so it must stay open: each
	// home states its own `required` and closes itself
	assert.Empty(t, def.Required, "requiredness is home-specific; the shared shape demands nothing")
	assert.Empty(t, def.Additional, "the shared shape must stay open for its homes to layer over")
	assert.Empty(t, def.Uneval, "the shared shape must stay open for its homes to layer over")

	// each home: a $ref to the shared shape, a local layer of narrowings,
	// refusals (`false` members whose fact lives elsewhere) and home-owned
	// members ONLY, and its own closure. The map grows with the homes — the
	// dictionary entry joins it as it lands — and a home missing from it is
	// a fourth spelling.
	typeProperty, foundTypeProperty := schema.Defs["typeProperty"]
	relationSettings, foundRelationSettings := schema.Properties["relation_settings"]
	for home, tc := range map[string]struct {
		node         schemaNode
		found        bool
		localMembers []string // narrowings and home-owned members the layer may hold
	}{
		"typeProperty": {
			node: typeProperty, found: foundTypeProperty,
			localMembers: []string{"format", "object_types", "section"},
		},
		"relation_settings": {
			node: relationSettings, found: foundRelationSettings,
			localMembers: nil, // nothing to narrow; its layer is all refusals
		},
	} {
		require.Truef(t, tc.found, "home %s must exist", home)
		h := tc.node
		refFound := false
		for _, a := range h.AllOf {
			var ref struct {
				Ref string `json:"$ref"`
			}
			if json.Unmarshal(a, &ref) == nil && ref.Ref == "#/$defs/propertyDefinition" {
				refFound = true
			}
		}
		assert.Truef(t, refFound, "%s must reference propertyDefinition, not restate it", home)
		allowed := map[string]bool{}
		for _, m := range tc.localMembers {
			allowed[m] = true
		}
		for m, raw := range h.Properties {
			if string(raw) == "false" {
				// a refusal, not a restatement: the member's fact has a home
				// elsewhere on this document (§2d)
				continue
			}
			assert.Truef(t, allowed[m], "%s restates %q — a shared member may only be narrowed, and only where the home must", home, m)
		}
		assert.Equalf(t, "false", string(h.Uneval), "%s must close itself with unevaluatedProperties: false", home)
	}
}

// The layered closure has a classic failure mode: `additionalProperties:
// false` beside an allOf-$ref refuses EVERYTHING the ref admits, and
// swapping it for unevaluatedProperties without a working annotation flow
// silently admits every unknown member instead. Both ends are pinned through
// the real validator: an unknown member on a type_properties entry is still
// refused, and every shared member is still admitted.
//
// How this can fail: replace typeProperty's unevaluatedProperties with
// additionalProperties (every entry with a key fails, second case red), or
// delete the gate entirely (first case goes green on a member nothing reads).
func TestPropertyDefinition_LayeredClosureHoldsBothWays(t *testing.T) {
	t.Run("an unknown member is still refused through the layer", func(t *testing.T) {
		err := Validate([]byte(`{"version":1,"kind":"object_type","key":"task",
			"type_properties":[{"key":"due_date","sections":"featured"}]}`))
		require.Error(t, err, "`sections` names nothing; the closure must catch it")
	})
	t.Run("a null object_types stays a relation-only shape", func(t *testing.T) {
		// the shared shape admits null because a relation's STORED value can
		// hold one (§2d); a type declares targets or omits the member, so the
		// home narrows it back to an array
		err := Validate([]byte(`{"version":1,"kind":"object_type","key":"task",
			"type_properties":[{"key":"assignee","object_types":null}]}`))
		require.Error(t, err)
	})
	t.Run("every shared member is admitted on an entry", func(t *testing.T) {
		err := Validate([]byte(`{"version":1,"kind":"object_type","key":"task",
			"type_properties":[{"key":"budget","name":"Budget","format":"number",
				"description":"Planned spend","include_time":false,"max_count":1,
				"readonly":true,"default_value":100,"section":"featured"}]}`))
		require.NoError(t, err)
	})
}

// capturingPropertyResolver records the definitions PropertyId receives, so a
// test can see exactly what crossed the codec seam.
type capturingPropertyResolver struct {
	defs []PropertyDefinition
}

func (r *capturingPropertyResolver) PropertyById(id string) (PropertyDefinition, bool) {
	return PropertyDefinition{}, false
}

func (r *capturingPropertyResolver) PropertyId(def PropertyDefinition) (string, bool) {
	r.defs = append(r.defs, def)
	return "relid-" + string(def.Key), true
}

// A member the schema admits and the codec sheds is worse than one the schema
// refuses: the document validates, imports, and quietly means less than it
// says. So the whole decoded definition must reach the resolver's create
// path, through BOTH doors the §2a array arrives by — the document
// (applyTypeProperties) and the PATCH channel (BuildRecommendedLists) — which
// share TypeProperty.definition precisely so they cannot disagree.
//
// How this can fail: shed one of the five members in TypeProperty.definition,
// or rebuild the def by hand in one door and forget a member there.
func TestPropertyDefinition_SharedMembersReachTheResolver(t *testing.T) {
	doc := []byte(`{"version":1,"kind":"object_type","key":"task",
		"type_properties":[{"key":"budget","name":"Budget","format":"number",
			"description":"Planned spend","include_time":false,"max_count":1,
			"readonly":true,"default_value":100,"section":"featured"}]}`)

	check := func(t *testing.T, defs []PropertyDefinition) {
		require.Len(t, defs, 1)
		def := defs[0]
		assert.Equal(t, domain.RelationKey("budget"), def.Key)
		assert.Equal(t, model.RelationFormat_number, def.Format)
		assert.Equal(t, "Planned spend", def.Description)
		require.NotNil(t, def.IncludeTime, "include_time false is a declaration, not an absence")
		assert.False(t, *def.IncludeTime)
		assert.Equal(t, int64(1), def.MaxCount)
		assert.True(t, def.Readonly)
		assert.Equal(t, float64(100), def.DefaultValue)
	}

	t.Run("the document door", func(t *testing.T) {
		r := &capturingPropertyResolver{}
		_, _, err := Unmarshal(doc, Options{ResolveProperties: r})
		require.NoError(t, err)
		check(t, r.defs)
	})

	t.Run("the PATCH door", func(t *testing.T) {
		r := &capturingPropertyResolver{}
		var parsed struct {
			TypeProps []TypeProperty `json:"type_properties"`
		}
		require.NoError(t, json.Unmarshal(doc, &parsed))
		_, err := BuildRecommendedLists(parsed.TypeProps, Options{ResolveProperties: r})
		require.NoError(t, err)
		check(t, r.defs)
	})
}
