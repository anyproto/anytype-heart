package anyblockjson

// authoredname_test.go — a property-definition entry that states only a
// NAME is spelled by that name, verbatim.
//
// This was the last derived identifier in a format that has none. The entry
// used to run the api-slug derivation over the name, which transliterates
// and then truncates: "Cooking Time" arrived as `cooking_time` — no longer
// the name a resolver holds — and "☕" arrived as the empty key, which the
// seam then refused. Each of the names below is now its own address.

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTypePropertyNameIsTheSpelling(t *testing.T) {
	// every one of these was mangled by the derivation the format no longer
	// runs; the third column is what it used to become
	names := []struct{ name, wasDerivedAs string }{
		{"Cooking Time", "cooking_time"},
		{"Due Date", "due_date"},
		{"Тоггл", "toggl"},
		{"作業内容", "zuo_ye_nei_rong"},
		{"C++", "c"},
		{"50% done", "50_done"},
		{"Дата выполнения", "data_vypolneniia"},
		{"#", ""},
		{"☕", ""},
	}

	for _, n := range names {
		t.Run(n.name, func(t *testing.T) {
			// given — an entry that identifies itself by name alone
			tp := TypeProperty{Name: n.name}

			// when
			term, isInternalKey := tp.authoredKey()

			// then
			assert.Equal(t, n.name, term, "the name IS the spelling")
			assert.False(t, isInternalKey, "a name derives a spelling, never a stored key")
			assert.NotEqual(t, n.wasDerivedAs, term,
				"the api-slug derivation is gone; %q used to arrive as %q", n.name, n.wasDerivedAs)
		})
	}
}

// The whole seam, not just the helper: a type document whose definition
// states only a name resolves that name as the property key, and the
// document round-trips through it.
func TestTypePropertyNameOnlyEntryResolvesThroughTheSeam(t *testing.T) {
	for _, name := range []string{"Cooking Time", "作業内容", "C++", "50% done"} {
		t.Run(name, func(t *testing.T) {
			// given
			defs, err := json.Marshal([]map[string]any{{"name": name, "format": "text"}})
			require.NoError(t, err)
			doc := []byte(fmt.Sprintf(`{"version":1,"kind":"object_type","id":"o1","internal_key":"recipe",`+
				`"type_settings":{"layout":"basic","property_definitions":%s},`+
				`"properties":{"Name":"Recipe"}}`, defs))
			require.NoError(t, Validate(doc), "I1/I2: the document is valid")

			// when
			_, snap, err := Unmarshal(doc, Options{GenerateId: seqIds("g")})

			// then — the name lands in the type's recommended list as the
			// key itself, with no resolver to map it to an id
			require.NoError(t, err)
			var got []string
			for _, v := range snap.Details.Fields["recommendedRelations"].GetListValue().GetValues() {
				got = append(got, v.GetStringValue())
			}
			assert.Equal(t, []string{name}, got,
				"the entry's own name is the key the list carries")
		})
	}
}

// A name that cannot BE a spelling derives none, and the seam says so at the
// entry's own pointer rather than truncating it into something else.
func TestTypePropertyUnwritableNameIsRefusedAtItsSlot(t *testing.T) {
	long := ""
	for i := 0; i <= maxPropertyKeyLen; i++ {
		long += "x"
	}
	for _, tc := range []struct{ name, why string }{
		{"", "a nameless entry identifies nothing"},
		{long, "a name longer than a spelling may be"},
		{"a\nb", "a name carrying a control character"},
	} {
		t.Run(tc.why, func(t *testing.T) {
			defs, err := json.Marshal([]map[string]any{{"name": tc.name, "format": "text"}})
			require.NoError(t, err)
			doc := []byte(fmt.Sprintf(`{"version":1,"kind":"object_type","id":"o1","internal_key":"recipe",`+
				`"type_settings":{"layout":"basic","property_definitions":%s},`+
				`"properties":{"Name":"Recipe"}}`, defs))

			_, _, err = Unmarshal(doc, Options{GenerateId: seqIds("g")})
			require.Error(t, err, tc.why)
			assert.Contains(t, err.Error(), typePropertyDefinitionsPath+"/0/"+memberProperty,
				"the refusal points at the entry that carries the name")
		})
	}
}
