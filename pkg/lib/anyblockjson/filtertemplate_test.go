package anyblockjson

// Dynamic filter values (§6.2): the client substitutes these for a real
// object id before issuing the query (anytype-ts
// Dataview.valueTemplateMapper). They are stored verbatim, are opaque to the
// middleware, and are not object ids.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func filterDoc(props, filters string) string {
	return `{"version": 2, "id": "p1", "blocks": [{"type": "dataview",
		"object_id": "someSet", "properties": [` + props + `],
		"views": [{"name": "Mine", "filters": [` + filters + `]}]}]}`
}

// the token is not an id: nothing in either direction rewrites it. The
// fixture used to carry a refs legend to prove the token could not be
// swallowed into one; there is no legend to be swallowed into now (§9a), so
// what is left to pin is that the token survives the object-valued path
// verbatim in both directions.
func TestRoundtrip_FilterTemplateSurvives(t *testing.T) {
	for _, tok := range []string{"_filter_template_1_", "_filter_template_2_"} {
		t.Run(tok, func(t *testing.T) {
			doc := filterDoc(`{"property": "assignee", "format": "objects"}`,
				`{"property": "assignee", "condition": "in", "value": ["`+tok+`"]}`)

			_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
			require.NoError(t, err)

			var got []string
			for _, b := range snap.Blocks {
				if dv := b.GetDataview(); dv != nil && len(dv.Views) > 0 {
					for _, f := range dv.Views[0].Filters {
						for _, v := range f.Value.GetListValue().GetValues() {
							got = append(got, v.GetStringValue())
						}
					}
				}
			}
			assert.Equal(t, []string{tok}, got, "must reach the snapshot unrewritten")

			data, err := Marshal(model.SmartBlockType_Page, snap, testOptions())
			require.NoError(t, err)
			assert.Contains(t, string(data), `"`+tok+`"`)
			assert.NoError(t, Validate(data))
		})
	}
}

// Resolving to an object id, a template token can only match an object/file
// property — and saying so is a WARNING, never a refusal: a stored dataview
// filter really carries this pair, so the rule as an error was an I1 break —
// export wrote the document with zero warnings and the package's own
// Validate refused it, making the object unexportable over one stored
// filter. The same tension was settled the same way for the date-preset
// rule beside it.
//
// How this can fail: turn the warnIssue back into addIssue and the Marshal
// arm below fails on its own output; drop the warning entirely and a filter
// that matches nothing ships with a clean bill of health.
func TestValidate_FilterTemplateOnWrongFormat(t *testing.T) {
	for _, f := range []string{"select", "date", "text", "number"} {
		t.Run(f, func(t *testing.T) {
			doc := filterDoc(
				`{"property": "stage", "format": "`+f+`"}`,
				`{"property": "stage", "condition": "in", "value": ["_filter_template_2_"]}`)
			var warns []Issue
			require.NoError(t, ValidateWarn([]byte(doc), func(i Issue) { warns = append(warns, i) }),
				"a stored filter must not make an object unexportable")
			found := false
			for _, w := range warns {
				if strings.Contains(w.Message, "resolves to an object id") {
					found = true
				}
			}
			assert.True(t, found, "the mismatch is still named, as a warning")
		})
	}
	for _, f := range []string{"objects", "files"} {
		t.Run(f+" is fine", func(t *testing.T) {
			var warns []Issue
			require.NoError(t, ValidateWarn([]byte(filterDoc(
				`{"property": "assignee", "format": "`+f+`"}`,
				`{"property": "assignee", "condition": "in", "value": ["_filter_template_2_"]}`)),
				func(i Issue) { warns = append(warns, i) }))
			for _, w := range warns {
				assert.NotContains(t, w.Message, "resolves to an object id")
			}
		})
	}

	t.Run("the I1 arm: the stored pair exports, validates, and warns", func(t *testing.T) {
		// the exact shape the invariant break was found on: a stored filter
		// carrying the token on a property the same block declares as text
		doc := filterDoc(
			`{"property": "stage", "format": "text"}`,
			`{"property": "stage", "condition": "in", "value": ["_filter_template_2_"]}`)
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err, "the seam accepts what Validate accepts (I2)")

		data, err := Marshal(model.SmartBlockType_Page, snap, testOptions())
		require.NoError(t, err)
		assert.Contains(t, string(data), `"_filter_template_2_"`)
		require.NoError(t, Validate(data),
			"Marshal never emits what its own Validate rejects (I1)")
	})
}

func TestValidate_FilterTemplateNonTriggers(t *testing.T) {
	t.Run("a real id on a select is not a token", func(t *testing.T) {
		assert.NoError(t, Validate([]byte(filterDoc(
			`{"property": "stage", "format": "select"}`,
			`{"property": "stage", "condition": "in", "value": ["In progress"]}`))))
	})
	t.Run("undeclared format is not checked", func(t *testing.T) {
		assert.NoError(t, Validate([]byte(filterDoc(
			`{"property": "other", "format": "objects"}`,
			`{"property": "notDeclared", "condition": "in", "value": ["_filter_template_2_"]}`))))
	})
}
