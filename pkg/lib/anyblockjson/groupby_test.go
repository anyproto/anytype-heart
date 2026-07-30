package anyblockjson

// Only kanban (select/multiSelect/checkbox) and calendar (date) group. A
// groupBy anywhere else renders nothing at all, which authors reliably get
// wrong because the document looks entirely reasonable.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dataviewDoc(props, view string) string {
	return `{"version": 1, "id": "p1", "blocks": [{"type": "dataview",
		"objectId": "someSet", "properties": [` + props + `],
		"views": [` + view + `]}]}`
}

func TestValidate_GroupByImpossibleIsError(t *testing.T) {
	for _, tc := range []struct {
		name, props, view, wantMsg string
	}{
		{
			name:    "kanban on an object relation",
			props:   `{"key": "category", "format": "objects"}`,
			view:    `{"type": "kanban", "name": "By category", "groupBy": "category"}`,
			wantMsg: `cannot group by "category"`,
		},
		{
			name:    "kanban on a date",
			props:   `{"key": "due", "format": "date"}`,
			view:    `{"type": "kanban", "name": "By due", "groupBy": "due"}`,
			wantMsg: `cannot group by "due"`,
		},
		{
			name:    "calendar on a select",
			props:   `{"key": "status", "format": "select"}`,
			view:    `{"type": "calendar", "name": "Cal", "groupBy": "status"}`,
			wantMsg: `cannot group by "status"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate([]byte(dataviewDoc(tc.props, tc.view)))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

func TestValidate_GroupByValidCombinations(t *testing.T) {
	for _, tc := range []struct{ name, props, view string }{
		{"kanban + select", `{"key": "status", "format": "select"}`,
			`{"type": "kanban", "name": "K", "groupBy": "status"}`},
		{"kanban + multiSelect", `{"key": "tags", "format": "multiSelect"}`,
			`{"type": "kanban", "name": "K", "groupBy": "tags"}`},
		{"kanban + checkbox", `{"key": "done", "format": "checkbox"}`,
			`{"type": "kanban", "name": "K", "groupBy": "done"}`},
		{"calendar + date", `{"key": "due", "format": "date"}`,
			`{"type": "calendar", "name": "C", "groupBy": "due"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.NoError(t, Validate([]byte(dataviewDoc(tc.props, tc.view))))
		})
	}
}

// A stale groupBy on a non-grouping view is real exported data: switching a
// kanban to a table in the editor leaves groupRelationKey behind
// (insertGroupRelationKey's default branch is a no-op). It must warn, never
// reject, or round-tripping an account would fail.
func TestValidate_GroupByOnNonGroupingViewOnlyWarns(t *testing.T) {
	for _, viewType := range []string{"table", "list", "gallery", "graph"} {
		t.Run(viewType, func(t *testing.T) {
			view := `{"type": "` + viewType + `", "name": "V", "groupBy": "status"}`
			if viewType == "table" {
				view = `{"name": "V", "groupBy": "status"}` // table is the default type
			}
			doc := dataviewDoc(`{"key": "status", "format": "select"}`, view)

			require.NoError(t, Validate([]byte(doc)), "must not reject real data")

			var warnings []Issue
			_, _, err := Unmarshal([]byte(doc), Options{
				GenerateId: seqIds("g"),
				OnWarning:  func(i Issue) { warnings = append(warnings, i) },
			})
			require.NoError(t, err)
			require.Len(t, warnings, 1)
			assert.Contains(t, warnings[0].Message, "do not group")
			assert.Contains(t, warnings[0].Path, "groupBy")
		})
	}
}

// nothing to check against when the key carries no declared format
func TestValidate_GroupByUndeclaredKeyIsAccepted(t *testing.T) {
	doc := dataviewDoc(`{"key": "other", "format": "select"}`,
		`{"type": "kanban", "name": "K", "groupBy": "notInProperties"}`)
	assert.NoError(t, Validate([]byte(doc)))
}
