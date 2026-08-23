package anyblockjson

// dataview_test.go — the dataview block's own round-trip rules (§6.2).

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// A filter's value is data, and a falsy one is the most ordinary data there
// is: `done = false` is how a task view spells "not finished yet". Eliding
// it leaves `done equal <nothing>` — a different query, in a document that
// still validates, and one no round-trip check can notice, because both
// generations lose it identically.
//
// Measured over 38,061 production objects when this was found: 151 of the
// 1,494 filters carrying a value carried a falsy one — 122 on `done` — in
// 70 documents.
//
// How this can fail: put setNonEmpty back at dataview.go's filter value and
// the false/0/"" rows come back nil.
func TestDataview_AFalsyFilterValueIsStillTheQuery(t *testing.T) {
	for name, tc := range map[string]struct {
		stored *types.Value
		want   string
	}{
		"false is a checkbox filter":   {boolVal(false), `"value": false`},
		"true still survives":          {boolVal(true), `"value": true`},
		"zero is a number filter":      {num(0), `"value": 0`},
		"one still survives":           {num(1), `"value": 1`},
		"the empty string is a filter": {str(""), `"value": ""`},
	} {
		t.Run(name, func(t *testing.T) {
			// given
			snap := filterValueSnapshot(tc.stored)

			// when
			data, err := Marshal(model.SmartBlockType_Page, snap, testOptions())
			require.NoError(t, err)
			require.NoError(t, Validate(data), "§11 I1")
			_, back, err := Unmarshal(data, testOptions())
			require.NoError(t, err)

			// then
			assert.Contains(t, string(data), tc.want)
			require.NotNil(t, back.GetBlocks(), "the filter value survives the round trip")
			var got *types.Value
			for _, b := range back.GetBlocks() {
				if dv := b.GetDataview(); dv != nil && len(dv.GetViews()) > 0 && len(dv.GetViews()[0].GetFilters()) > 0 {
					got = dv.GetViews()[0].GetFilters()[0].GetValue()
				}
			}
			require.NotNil(t, got, "the value came back nil — the query changed meaning")
			assert.Equal(t, tc.stored.String(), got.String())
		})
	}
}

func boolVal(b bool) *types.Value {
	return &types.Value{Kind: &types.Value_BoolValue{BoolValue: b}}
}

func filterValueSnapshot(v *types.Value) *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{
			{Id: "bafyreifiltroot", ChildrenIds: []string{"dv"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "dv", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{{
					Id: "view1", Name: "All",
					Filters: []*model.BlockContentDataviewFilter{{
						Id:          "f1",
						RelationKey: "done",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       v,
					}},
				}},
			}}},
		},
		Details: fields(map[string]*types.Value{"id": str("bafyreifiltroot")}),
	}
}
