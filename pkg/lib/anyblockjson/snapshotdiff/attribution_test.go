package snapshotdiff

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const participantId = "_participant_bafyreid62d5e6hny6mv6zass2zg73nxyhjzhjasx7imvzxvqz6rcnjqcgq_30afw2fe3tvff_AASdKiEGfcyhxX3ufr4auHRviACUXxkF68uZwtSb2AnyRoMA"

// Every object in a real account carries a `creator`, and after a round trip
// through this format none of them does: export writes the member's NAME and
// import drops the key (§3). A comparator that did not know would report data
// loss on all 36,966 of them — the same shape as the 1,344 false failures the
// recommended-list normalization produced.
//
// This is inherited rather than restated: Compare skips
// anyblockjson.InternalPropertyKeys(), which the attribution keys join. The
// point of pinning it here is that the inheritance is not obvious — the keys
// are exempt from the deny rule, so "internal" and "refused" have come apart
// — and a future edit that took them off the internal list would make the
// sweep unusable with nothing else failing.
//
// How this can fail: make the comparator compare the attribution keys —
// through InternalPropertyKeys(), which is what it reads — and both cases
// report a changed/lost detail.
//
// NOT via `derivedAttributionProperties` in strippedDetailKeys(): an audit
// deleted that loop and all four packages stayed green, because both keys are
// already in bundle.LocalAndDerivedRelationKeys and neither is in
// propertiesKeptOnExport, so the loop is inert today. It is defence for the
// day one of them moves onto the keep-list — not the mechanism this test
// pins, and naming it here sent a reader looking in the wrong place.
func TestCompare_AttributionKeysAreNotDataLoss(t *testing.T) {
	t.Run("creator lost to the round trip is not reported", func(t *testing.T) {
		// given
		orig := snapshot(map[string]*types.Value{
			"name":    str("Doc"),
			"creator": str(participantId),
		})
		got := snapshot(map[string]*types.Value{"name": str("Doc")})

		// when
		diff := Compare(orig, got, model.SmartBlockType_Page, anyblockjson.Options{})

		// then
		assert.Empty(t, diff)
	})

	t.Run("lastModifiedBy likewise, in both directions", func(t *testing.T) {
		// given
		orig := snapshot(map[string]*types.Value{"lastModifiedBy": str(participantId)})
		got := snapshot(map[string]*types.Value{"lastModifiedBy": str("someone-else")})

		// when
		diff := Compare(orig, got, model.SmartBlockType_Page, anyblockjson.Options{})

		// then
		assert.Empty(t, diff)
	})

	t.Run("a user-chosen participant reference is still compared", func(t *testing.T) {
		// given the control: assignee is source: details, and losing it IS loss
		orig := snapshot(map[string]*types.Value{"assignee": str(participantId)})
		got := snapshot(map[string]*types.Value{})

		// when
		diff := Compare(orig, got, model.SmartBlockType_Page, anyblockjson.Options{})

		// then
		require.Len(t, diff, 1)
		assert.Contains(t, diff[0], `detail "assignee" changed`)
	})
}
