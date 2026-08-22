package anyblockjson

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// answeringNamer answers every id with one name, including a blank one — the
// shape the exported ParticipantResolver contract permits but the format's own
// rule forbids. The shipped storeresolver never answers blank; a third-party
// implementation may, because the interface only says a resolver that cannot
// answer returns false.
type answeringNamer struct{ name string }

func (a answeringNamer) ParticipantName(string) (string, bool) { return a.name, true }

func attributedSnapshot() *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{{Id: "o1",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
		Details: fields(map[string]*types.Value{
			"id": str("o1"), "name": str("Notes"),
			"creator": {Kind: &types.Value_ListValue{ListValue: &types.ListValue{
				Values: []*types.Value{str("_participant_a_b_C")}}}},
		}),
		ObjectTypes: []string{"ot-page"},
	}
}

// §3's rule is "a name or nothing, never a blank": an empty `creator` costs
// bytes, says less than absence, and reads to a model as an attribution that
// exists and is empty. Enforcing it inside the shipped resolver is not enough
// — the seam every resolver passes through has to hold it, or one third-party
// implementation puts a blank line on every object in an export.
//
// This can only fail if the seam stops filtering: it drives the real Marshal
// with a resolver that answers, so a rule enforced only in storeresolver would
// not save it.
func TestExport_AResolverThatAnswersBlankWritesNoAttribution(t *testing.T) {
	for name, answer := range map[string]string{
		"empty":           "",
		"a single space":  " ",
		"only whitespace": " \t\n ",
	} {
		t.Run(name, func(t *testing.T) {
			data, err := Marshal(model.SmartBlockType_Page, attributedSnapshot(),
				Options{ResolveParticipants: answeringNamer{name: answer}})
			require.NoError(t, err)
			assert.NotContains(t, string(data), `"creator"`,
				"a blank name is not a name: the property is omitted (§3)")
			require.NoError(t, Validate(data))
		})
	}

	// the control: a real name still lands, so the rule above cannot pass by
	// dropping attribution altogether
	t.Run("a real name still lands", func(t *testing.T) {
		data, err := Marshal(model.SmartBlockType_Page, attributedSnapshot(),
			Options{ResolveParticipants: answeringNamer{name: "Roman"}})
		require.NoError(t, err)
		assert.Contains(t, string(data), `"creator": "Roman"`)
	})
}

// MarshalPropertyValue and UnmarshalPropertyValue are twins: whatever one
// writes, the other reads back. Attribution breaks that on purpose — export
// writes a member NAME, and a name is not an address — so the read half must
// refuse rather than hand "Roman" back as though it were a participant id.
// Without this a value-level caller round-tripping one property puts a display
// name in an id slot, which nothing downstream can resolve.
func TestFragment_TheValueTwinsDisagreeOnAttribution(t *testing.T) {
	for _, key := range []string{"creator", "lastModifiedBy"} {
		assert.Nil(t, UnmarshalPropertyValue(key, "Roman", Options{}),
			"%s is dropped by whole-document import, so the value door drops it too", key)
	}

	// the control: an ordinary property still round-trips through the twins
	got := UnmarshalPropertyValue("assignee", "_participant_a_b_C", Options{})
	require.NotNil(t, got, "a user-chosen participant property is untouched")
	assert.Equal(t, "_participant_a_b_C",
		got.GetListValue().GetValues()[0].GetStringValue(),
		"and keeps its full id, resolvable")
}
