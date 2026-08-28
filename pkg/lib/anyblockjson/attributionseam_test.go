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

// §3's rule is "a name or nothing after the `#`, never a blank": a dangling
// `#` costs bytes, says less than its absence, and reads to a model as a
// name that exists and is empty. Enforcing it inside the shipped resolver is
// not enough — the seam every resolver passes through has to hold it
// (refNameLabel), or one third-party implementation puts a dangling `#` on
// every object in an export. The id half is unaffected either way: it is
// the resolvable content and is written bare.
//
// This can only fail if the seam stops filtering: it drives the real Marshal
// with a resolver that answers, so a rule enforced only in storeresolver would
// not save it.
func TestExport_AResolverThatAnswersBlankWritesABareId(t *testing.T) {
	for name, answer := range map[string]string{
		"empty":           "",
		"a single space":  " ",
		"only whitespace": " \t\n ",
	} {
		t.Run(name, func(t *testing.T) {
			data, err := Marshal(model.SmartBlockType_Page, attributedSnapshot(),
				Options{ResolveParticipants: answeringNamer{name: answer}})
			require.NoError(t, err)
			assert.Contains(t, string(data), `"Created by": "_participant_a_b_C"`,
				"the bare id: resolvable, and blank-name-proof")
			assert.NotContains(t, string(data), "#", "a blank name is not a name — no dangling separator")
			require.NoError(t, Validate(data))
		})
	}

	// the control: a real name still lands as the suffix, so the rule above
	// cannot pass by dropping the suffix machinery altogether
	t.Run("a real name still lands", func(t *testing.T) {
		data, err := Marshal(model.SmartBlockType_Page, attributedSnapshot(),
			Options{ResolveParticipants: answeringNamer{name: "Roman"}})
		require.NoError(t, err)
		assert.Contains(t, string(data), `"Created by": "_participant_a_b_C#roman"`)
	})
}

// MarshalPropertyValue and UnmarshalPropertyValue are twins: whatever one
// writes, the other reads back. Attribution breaks that on purpose — the
// value is derived from the tree on every rebuild, and no write path could
// honour what a document carries — so the read half must drop it rather
// than hand it back as though it were settable.
func TestFragment_TheValueTwinsDisagreeOnAttribution(t *testing.T) {
	for _, key := range []string{"creator", "lastModifiedBy"} {
		assert.Nil(t, UnmarshalPropertyValue(key, "_participant_a_b_C#roman", Options{}),
			"%s is dropped by whole-document import, so the value door drops it too", key)
	}

	// the control: an ordinary property still round-trips through the twins
	got := UnmarshalPropertyValue("assignee", "_participant_a_b_C", Options{})
	require.NotNil(t, got, "a user-chosen participant property is untouched")
	assert.Equal(t, "_participant_a_b_C",
		got.GetListValue().GetValues()[0].GetStringValue(),
		"and keeps its full id, resolvable")
}
