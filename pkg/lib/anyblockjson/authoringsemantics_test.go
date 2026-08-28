package anyblockjson

// authoringsemantics_test.go — the two authoring-subset rules that had to
// leave the schema, because JSON Schema matches a member name literally and
// the format resolves a property key case- and separator-insensitively.
//
// A literal rule over a key the codec spells many ways is not a narrower
// rule; it is a rule with holes. Both rules below had one: the canonical
// `{"Name": "Habit"}` was REFUSED as a type document's name while the
// retired lowercase spelling was accepted, and nine derived keys lost their
// authoring refusal entirely the moment their canonical spelling became a
// display name.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A type document must name itself, and every spelling that RESOLVES to the
// name property satisfies it. The rule used to be `required: ["name"]` in
// the authoring schema, which accepted exactly one of these four and
// refused the canonical one.
func TestAuthoringTypeDocumentNamesItself(t *testing.T) {
	typeDoc := func(props string) []byte {
		return []byte(`{"version":1,"kind":"object_type","id":"habit","internal_key":"habit",` +
			`"type_settings":{"layout":"basic"},"properties":{` + props + `}}`)
	}

	t.Run("every spelling of the name property satisfies the rule", func(t *testing.T) {
		for _, spelling := range []string{"Name", "name", "NAME"} {
			doc := typeDoc(`"` + spelling + `":"Habit"`)
			require.NoError(t, Validate(doc), "the full format accepts it: %s", doc)
			assert.NoError(t, ValidateAuthoring(doc),
				"%q resolves to the name property, so the type names itself", spelling)
		}
	})

	t.Run("a type document with no name at all is still refused", func(t *testing.T) {
		doc := typeDoc(`"Description":"a habit"`)
		require.NoError(t, Validate(doc), "nameless is valid AnyBlock JSON — the subset is what refuses it")

		err := ValidateAuthoring(doc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "/properties")
		assert.Contains(t, err.Error(), "states its own display name")
	})

	t.Run("the rule is a type document's alone", func(t *testing.T) {
		doc := []byte(`{"version":1,"id":"page1","properties":{"Description":"no title"}}`)
		assert.NoError(t, ValidateAuthoring(doc),
			"an ordinary object may be untitled; only a type must name itself")
	})
}

// The nine keys the FULL format does not refuse — it DROPS them at import —
// are refused by the authoring subset under every spelling, not only the
// pre-raw-name one the schema's literal list happens to name.
func TestAuthoringDeniedKeysAreRefusedUnderEverySpelling(t *testing.T) {
	for key, why := range authoringDeniedPropertyKeys {
		name := BundledKeyVocabulary{}.PropertySlug(key)
		require.NotEqual(t, key, name, "%s has a display name to be spelled by", key)

		for _, spelling := range []string{key, name, strings.ToLower(name)} {
			t.Run(key+" as "+spelling, func(t *testing.T) {
				b, err := json.Marshal(map[string]any{
					"version": 1, "id": "o1",
					"properties": map[string]any{spelling: "whatever"},
				})
				require.NoError(t, err)

				err = ValidateAuthoring(b)
				require.Error(t, err, "an author does not write %q (%s)", key, why)
				assert.Contains(t, err.Error(), key,
					"the refusal names the resolved key, not just the spelling written")
			})
		}
	}
}

// The Go set is the authority and the schema's literal list is the
// documentation; this pins them together, in both directions, so neither
// can rot the way the list already did once.
func TestAuthoringDeniedKeysMatchTheSchema(t *testing.T) {
	var schema struct {
		Defs struct {
			PropertyMap struct {
				PropertyNames struct {
					Not struct {
						Enum []string `json:"enum"`
					} `json:"not"`
				} `json:"propertyNames"`
			} `json:"propertyMap"`
		} `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(AuthoringSchemaJSON(), &schema))
	listed := map[string]bool{}
	for _, e := range schema.Defs.PropertyMap.PropertyNames.Not.Enum {
		listed[e] = true
	}
	require.NotEmpty(t, listed)

	t.Run("every authoring-denied key is listed under both its spellings", func(t *testing.T) {
		for key := range authoringDeniedPropertyKeys {
			assert.True(t, listed[key], "the schema list is missing the stored key %q", key)
			name := BundledKeyVocabulary{}.PropertySlug(key)
			assert.True(t, listed[name],
				"the schema list is missing %q, which is how the format spells %q today", name, key)
		}
	})

	t.Run("no listed spelling has quietly gone toothless", func(t *testing.T) {
		// A listed spelling that RESOLVES to a stored key is a rule about
		// that key, and something has to enforce it for every OTHER
		// spelling too: either the full format's deny rule (import refuses
		// exactly what export strips) or the Go set above. An entry backed
		// by neither states a rule nothing enforces — which is precisely
		// what happened when the canonical spelling of these keys became a
		// display name and the list kept banning only the old one.
		//
		// Entries that resolve to NOTHING are skipped, and they are the
		// bulk of the list: a denied key's fold class deliberately answers
		// nothing, so `icon_emoji` is an ordinary custom property name in
		// the full format. Banning it here is the subset catching an author
		// who meant the envelope icon, and only the subset can.
		for _, spelling := range schema.Defs.PropertyMap.PropertyNames.Not.Enum {
			key, resolves := BundledKeyVocabulary{}.PropertyKey(spelling)
			if !resolves {
				continue
			}
			if _, denied := authoringDeniedPropertyKeys[key]; denied {
				continue
			}
			doc := []byte(fmt.Sprintf(`{"version":1,"id":"o1","properties":{%q:"x"}}`, key))
			assert.Error(t, Validate(doc),
				"the schema bans %q, which resolves to %q — but nothing refuses that key "+
					"under its own spelling, so the ban covers one spelling of many", spelling, key)
		}
	})
}

// The subset narrows a name-over-number key to the NAME, and that rule was
// keyed to the retired member name too.
func TestAuthoringNamedEnumTakesTheNameUnderItsCanonicalSpelling(t *testing.T) {
	for _, spelling := range []string{"Layout align", "layout_align", "layoutAlign"} {
		t.Run(spelling, func(t *testing.T) {
			named := []byte(fmt.Sprintf(`{"version":1,"id":"o1","properties":{%q:"center"}}`, spelling))
			assert.NoError(t, ValidateAuthoring(named), "the name is what an author writes")

			bare := []byte(fmt.Sprintf(`{"version":1,"id":"o1","properties":{%q:1}}`, spelling))
			require.NoError(t, Validate(bare), "the full format passes the stored number through")
			err := ValidateAuthoring(bare)
			require.Error(t, err, "the subset removes the stored-value pass-through")
			assert.Contains(t, err.Error(), "layoutAlign")
		})
	}
}
