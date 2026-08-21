package anyblockjson

// Dynamic filter values (§6.2): the client substitutes these for a real
// object id before issuing the query (anytype-ts
// Dataview.valueTemplateMapper). They are stored verbatim, are opaque to the
// middleware, and are not object ids.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func filterDoc(props, filters string) string {
	return `{"version": 1, "id": "p1", "blocks": [{"type": "dataview",
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
			doc := filterDoc(`{"key": "assignee", "format": "objects"}`,
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

// resolving to an object id, it can only match an object/file property
func TestValidate_FilterTemplateOnWrongFormat(t *testing.T) {
	for _, f := range []string{"select", "date", "text", "number"} {
		t.Run(f, func(t *testing.T) {
			err := Validate([]byte(filterDoc(
				`{"key": "stage", "format": "`+f+`"}`,
				`{"property": "stage", "condition": "in", "value": ["_filter_template_2_"]}`)))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "resolves to an object id")
		})
	}
	for _, f := range []string{"objects", "files"} {
		t.Run(f+" is fine", func(t *testing.T) {
			assert.NoError(t, Validate([]byte(filterDoc(
				`{"key": "assignee", "format": "`+f+`"}`,
				`{"property": "assignee", "condition": "in", "value": ["_filter_template_2_"]}`))))
		})
	}
}

func TestValidate_FilterTemplateNonTriggers(t *testing.T) {
	t.Run("a real id on a select is not a token", func(t *testing.T) {
		assert.NoError(t, Validate([]byte(filterDoc(
			`{"key": "stage", "format": "select"}`,
			`{"property": "stage", "condition": "in", "value": ["In progress"]}`))))
	})
	t.Run("undeclared format is not checked", func(t *testing.T) {
		assert.NoError(t, Validate([]byte(filterDoc(
			`{"key": "other", "format": "objects"}`,
			`{"property": "notDeclared", "condition": "in", "value": ["_filter_template_2_"]}`))))
	})
}
