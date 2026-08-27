package snapshotdiff

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// A participant document does not carry `created_date` (§3): the stored
// value is time.Now() stamped on every cold build — measured across a
// 1,164-document double-export, the ONLY kind that drifted (22/22) and the
// ONLY field (created_date) — so export omits it whatever it holds, and the
// comparator learns the rule in the same commit through the format's OWN
// predicate. This is the standing rule for every drop: the last comparator
// that learned about a drop late reported 1,344 false failures in one
// sweep, and this one would report one per corpus participant — 2,492.
//
// How this can fail: teach export the drop without wiring
// DroppedParticipantProvenanceKey here (the first case reports created_date
// lost on every participant); widen the suppression past the predicate's
// kind scope (the page case goes quiet on real loss); or suppress a got
// side that still CARRIES the key (the absent-only scoping is what keeps a
// wrong value reportable).
func TestParticipantProvenance_DropIsNormalizationOnParticipants(t *testing.T) {
	orig := &model.SmartBlockSnapshotBase{
		Details: &types.Struct{Fields: map[string]*types.Value{
			"id":               str2("AASdKiEGfcyhxX3ufr4auHRviACUXxkF68uZwtSb2AnyRoMA"),
			"name":             str2("Roman"),
			"createdDate":      num2(1756180000),
			"lastModifiedDate": num2(1700000000),
		}},
	}

	t.Run("the documented drop is silent, through the real round trip", func(t *testing.T) {
		data, err := anyblockjson.Marshal(model.SmartBlockType_Participant, orig, anyblockjson.Options{})
		require.NoError(t, err)
		_, got, err := anyblockjson.Unmarshal(data, anyblockjson.Options{})
		require.NoError(t, err)

		found := Compare(orig, got, model.SmartBlockType_Participant, anyblockjson.Options{})
		assert.Empty(t, found, "the drop is the format's decision, not loss")
	})

	t.Run("the same key vanishing from a page still reports", func(t *testing.T) {
		got := &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				"id": str2("bafypage"), "name": str2("Roman"), "lastModifiedDate": num2(1700000000),
			}},
		}
		found := Compare(orig, got, model.SmartBlockType_Page, anyblockjson.Options{})
		assert.NotEmpty(t, found, "off a participant, createdDate is real provenance")
	})
}
