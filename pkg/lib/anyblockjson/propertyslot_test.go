package anyblockjson

// propertyslot_test.go — the slot that names a property is spelled `property`,
// everywhere, and the vacated spelling `key` is refused with the repair named.
//
// Measured over 28,599 real exported documents, the slot had TWO member names
// twelve lines apart inside one dataview block, each a hard schema error in
// the other's position: `properties[]` required `key` (28,034 slots) while
// the columns, sorts and filters beside it required `property` (46,710), and
// the `property` block spelled `key` too (67,808). 2,504 real dataview blocks
// wrote the SAME spelling under both names. Generalising a member name across
// sibling structures inside one block is what generalisation IS, so a model
// that learned either spelling was rejected at the other slot — few-shot
// prompting on the corpus reproduced the split rather than resolving it.
//
// v0.41 collapses the slot onto `property`. No input alias: the pre-freeze
// rule (v0.37, v0.38) is that an old spelling is refused like any unknown
// member — a second legal spelling would keep the two-name confusion alive in
// every example an agent learns from — but refused WITH the repair named,
// because 95,842 corpus slots spell `key` and an agent prompted on an old
// export will write it.

import (
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The two documents of the measured defect, spelled the ONE way: the member
// name an author learned at any sibling slot works at every other.
func TestValidate_OnePropertySpellingAcrossTheDataview(t *testing.T) {
	for name, doc := range map[string]string{
		"properties entry": `{"version":2,"id":"o1","blocks":[{"id":"b1","type":"dataview","object_id":"t1",
			"properties":[{"property":"name","format":"text"}],"views":[{"id":"v"}]}]}`,
		"view column": `{"version":2,"id":"o1","blocks":[{"id":"b1","type":"dataview","object_id":"t1",
			"views":[{"id":"v","columns":[{"property":"name"}]}]}]}`,
		"property block": `{"version":2,"id":"o1","blocks":[{"id":"b1","type":"property","property":"name"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, Validate([]byte(doc)), doc)
			_, _, err := Unmarshal([]byte(doc), testOptions())
			require.NoError(t, err, "what Validate accepts, Unmarshal accepts (§12 I2)")
		})
	}
}

// The vacated spelling is refused at both slots that carried it, addressed at
// the member (§12), with the repair named — and Unmarshal refuses the same
// documents (I2). These scenarios pinned `key` as the REQUIRED member until
// v0.41; they now pin its refusal, because the corpus guarantees old-corpus
// agents will keep writing it.
func TestValidate_TheVacatedKeySpellingIsRefusedWithTheRepairNamed(t *testing.T) {
	for name, tc := range map[string]struct{ doc, path string }{
		"dataview properties entry": {
			`{"version":2,"id":"o1","blocks":[{"id":"b1","type":"dataview","object_id":"t1",
				"properties":[{"key":"name","format":"text"}],"views":[{"id":"v"}]}]}`,
			"/blocks/0/properties/0/key"},
		"property block": {
			`{"version":2,"id":"o1","blocks":[{"id":"b1","type":"property","key":"prio"}]}`,
			"/blocks/0/key"},
		"view column, where key never belonged": {
			`{"version":2,"id":"o1","blocks":[{"id":"b1","type":"dataview","object_id":"t1",
				"views":[{"id":"v","columns":[{"key":"name"}]}]}]}`,
			"/blocks/0/views/0/columns/0/key"},
	} {
		t.Run(name, func(t *testing.T) {
			err := Validate([]byte(tc.doc))
			require.Error(t, err, "accepted the vacated spelling:\n%s", tc.doc)
			assert.Contains(t, issuePaths(t, err), tc.path,
				"the refusal has to name the member it is about (§12): %v", err)
			assert.Contains(t, err.Error(), `spelled "property"`,
				"told only \"not allowed\", the obvious wrong repair is deleting the member: %v", err)

			_, _, err = Unmarshal([]byte(tc.doc), testOptions())
			require.Error(t, err, "Unmarshal must refuse what Validate refuses (§12 I2)")
		})
	}
}

// The I1 side of the collapse: export writes the one spelling at every slot
// that names a property, and no member spelled `key` anywhere — on a snapshot
// exercising all four slots of the measured defect at once (a dataview's
// properties list, a column, a sort, a filter, and a property block beside
// them). Validate then accepts exactly what export emits.
func TestExport_EmitsOnlyThePropertySpelling(t *testing.T) {
	snap := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{
			{Id: "root", ChildrenIds: []string{"rel", "dv"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "rel", Content: &model.BlockContentOfRelation{
				Relation: &model.BlockContentRelation{Key: "note"}}},
			{Id: "dv", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				RelationLinks: []*model.RelationLink{
					{Key: "note", Format: model.RelationFormat_longtext},
					{Key: "due_date", Format: model.RelationFormat_date},
				},
				Views: []*model.BlockContentDataviewView{{Id: "v1", Name: "All",
					Relations: []*model.BlockContentDataviewRelation{{Key: "note", Width: 120}},
					Sorts:     []*model.BlockContentDataviewSort{{RelationKey: "due_date"}},
					Filters: []*model.BlockContentDataviewFilter{{
						RelationKey: "note",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       str("x"),
					}},
				}},
			}}},
		},
		Details: fields(map[string]*types.Value{"id": str("root")}),
	}

	data, err := Marshal(model.SmartBlockType_Page, snap, testOptions())
	require.NoError(t, err)

	got := string(data)
	assert.NotContains(t, got, `"key"`,
		"no slot spells the vacated member — one concept, one spelling (§15 #14)")
	assert.Equal(t, 6, strings.Count(got, `"property":`),
		"the two properties entries, the column, the sort, the filter and the property block all spell it:\n%s", got)
	require.NoError(t, Validate(data), "Marshal never emits what Validate rejects (§11 I1)")
}
