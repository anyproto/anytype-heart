package anyblockjson

import (
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type slugGuardOptions struct{}

func (slugGuardOptions) OptionName(key domain.RelationKey, id string) (string, bool) {
	if id == "optX" {
		return "High", true
	}
	return "", false
}
func (slugGuardOptions) OptionId(key domain.RelationKey, name string) (string, bool) {
	if name == "High" {
		return "optX", true
	}
	return "", false
}

func dataviewOnKey(key string) *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{
			{Id: "o1", ChildrenIds: []string{"dv"}, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "dv", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{{Id: "v1", Name: "V", Filters: []*model.BlockContentDataviewFilter{
					{RelationKey: key, Condition: model.BlockContentDataviewFilter_Equal,
						Value: &types.Value{Kind: &types.Value_StringValue{StringValue: "optX"}}},
				}}},
				RelationLinks: []*model.RelationLink{{Key: key, Format: model.RelationFormat_status}},
			}}},
		},
		Details:     fields(map[string]*types.Value{"id": str("o1")}),
		ObjectTypes: []string{"ot-page"},
	}
}

// propertySlug returns the STORED key verbatim when the vocabulary has no
// spelling for it, and a stored key need not be writable — §3's key rule is a
// deny rule precisely because real stores hold things like
// `completion_status_Not Started`. /properties filters unwritable keys before
// slugging; a dataview FILTER or SORT slot does not, so the option legend
// could take an outer key `propertyNameIssues` refuses, and Marshal emitted a
// document its own Validate and Unmarshal rejected — losing the whole object.
//
// This can only fail if export writes such a key into the legend again: the
// assertions are on Validate and Unmarshal accepting Marshal's own bytes (I1),
// not on the legend's contents, so a legend that silently stops being written
// at all would still have to keep I1 to pass.
func TestOptionIds_AnUnwritableSpellingNeverReachesTheLegend(t *testing.T) {
	for name, key := range map[string]string{
		"a control character in the stored key": "a\nb",
		"a stored key past the spelling bound":  strings.Repeat("y", 140),
	} {
		t.Run(name, func(t *testing.T) {
			data, err := Marshal(model.SmartBlockType_Page, dataviewOnKey(key), Options{ResolveOptions: slugGuardOptions{}})
			require.NoError(t, err)

			require.NoError(t, Validate(data),
				"Marshal must not emit a document its own Validate rejects (§11 I1): %s", string(data))
			_, back, err := Unmarshal(data, Options{})
			require.NoError(t, err, "…nor one its own Unmarshal rejects (§11 I1)")

			// the object itself survives — the point of the invariant
			require.NotNil(t, back)
			assert.NotEmpty(t, back.GetBlocks(), "the whole object was lost, not merely the legend")
		})
	}
}

// the positive control: a WRITABLE spelling in the same slot still earns its
// legend entry, so the guard above cannot pass by suppressing everything.
func TestOptionIds_AWritableSpellingInAFilterStillEarnsItsEntry(t *testing.T) {
	data, err := Marshal(model.SmartBlockType_Page, dataviewOnKey("severity"), Options{ResolveOptions: slugGuardOptions{}})
	require.NoError(t, err)
	require.NoError(t, Validate(data))
	assert.Contains(t, string(data), `"option_ids"`, "a filter-only property must still pin its option identity")
	assert.Contains(t, string(data), `"severity"`)
}
