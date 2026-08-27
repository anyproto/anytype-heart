package anyblockjson

// participantprovenance_test.go pins the participant-scoped provenance drop
// (§3, participantprovenance.go): a participant document does not carry
// `created_date`, because the object is derived from the ACL, has no
// creation change, and the stored value is time.Now() stamped on every cold
// build. The measurement that admitted the drop: two exports of the same 7
// spaces, 1,164 documents compared field-by-field — the only drifting kind
// was participant (22 of 22) and the only drifting field `created_date`
// (22 of 22); on a full 155-space run, 2,322 drifts against 2,492
// participants, every other kind byte-stable.

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func participantSnapshot() *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{
		Details: fields(map[string]*types.Value{
			"id":               str("AASdKiEGfcyhxX3ufr4auHRviACUXxkF68uZwtSb2AnyRoMA"),
			"name":             str("Roman"),
			"createdDate":      num(1756180000), // the load timestamp wearing a fact's name
			"lastModifiedDate": num(1700000000),
		}),
	}
}

// Export's half: the key is omitted on a participant, and ONLY there — on a
// page createdDate is real provenance and stays. The kind scoping is the
// whole rule: widen it and every object loses its creation date; narrow it
// away and every eviction-separated pair of exports disagrees on 2,492
// documents again.
//
// How this can fail: gate on the key without the kind (the page case goes
// red); drop the export site but keep the predicate (the participant case
// finds the key in the document); or stamp the drop into strippedDetailKeys
// (kind-blind, same page regression).
func TestParticipantProvenance_CreatedDateNeverExported(t *testing.T) {
	// given / when
	doc, err := Marshal(model.SmartBlockType_Participant, participantSnapshot(), Options{})
	require.NoError(t, err)

	// then
	assert.NotContains(t, string(doc), "created_date",
		"a participant's created_date is a load timestamp, not a fact")
	assert.Contains(t, string(doc), "last_modified_date",
		"only the measured drifting key is dropped")

	t.Run("on a page the same key stays", func(t *testing.T) {
		snap := &model.SmartBlockSnapshotBase{
			Details: fields(map[string]*types.Value{
				"id": str("bafypage"), "name": str("A page"), "createdDate": num(1756180000),
			}),
		}
		doc, err := Marshal(model.SmartBlockType_Page, snap, Options{})
		require.NoError(t, err)
		assert.Contains(t, string(doc), "created_date",
			"on every other kind createdDate is real provenance")
	})
}

// Import's half: dropped, not refused — a document written before the rule
// (every pre-v0.47 dump carries the key on its participants) is stale
// rather than wrong, the transientProperties policy scoped by kind.
//
// How this can fail: refuse instead of drop (every existing dump's
// participants stop importing); or let the value through (the destination's
// derived detail is shadowed by the source's load timestamp).
func TestParticipantProvenance_DroppedNotRefusedOnImport(t *testing.T) {
	doc := `{"version": 1, "kind": "participant",
		"id": "AASdKiEGfcyhxX3ufr4auHRviACUXxkF68uZwtSb2AnyRoMA",
		"properties": {"name": "Roman", "created_date": "2026-08-26T21:42:26Z"}}`

	require.NoError(t, Validate([]byte(doc)), "stale, not wrong")
	sbType, snap, err := Unmarshal([]byte(doc), Options{})
	require.NoError(t, err, "Validate and Unmarshal agree (§11 I2)")
	require.Equal(t, model.SmartBlockType_Participant, sbType)
	assert.NotContains(t, snap.GetDetails().GetFields(), "createdDate",
		"the load timestamp must not reach the snapshot")
	assert.Equal(t, "Roman", snap.GetDetails().GetFields()["name"].GetStringValue())

	t.Run("on a page the same property still lands", func(t *testing.T) {
		doc := `{"version": 1, "properties": {"name": "A page", "created_date": "2026-08-26T21:42:26Z"}}`
		_, snap, err := Unmarshal([]byte(doc), Options{})
		require.NoError(t, err)
		assert.Contains(t, snap.GetDetails().GetFields(), "createdDate")
	})
}

// The predicate the comparator consults, pinned at both edges of its scope.
func TestDroppedParticipantProvenanceKey_Scope(t *testing.T) {
	assert.True(t, DroppedParticipantProvenanceKey(model.SmartBlockType_Participant, "createdDate"))
	assert.False(t, DroppedParticipantProvenanceKey(model.SmartBlockType_Page, "createdDate"),
		"kind-scoped: a page's creation date is real provenance")
	assert.False(t, DroppedParticipantProvenanceKey(model.SmartBlockType_Participant, "lastModifiedDate"),
		"key-scoped: only the measured drifting key")
}
