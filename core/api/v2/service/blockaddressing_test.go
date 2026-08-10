package v2service

// blockaddressing_test.go pins what a block reference can address and what a
// read may promise about it (APIV2.md §8.29 F4, and the §8.28 property-1
// collision guard the audit found unpinned). Shared fixtures and helpers
// live in payloadids_test.go.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// TestBlockNotFoundHintsAreTrue is F4. Both 404s used to say "GET the object
// with ?outline=true to list block ids" — and cell descendants are served by
// a default read, resolve on NO channel, and are not in the outline. The
// caller was sent round a loop that could never terminate.
func TestBlockNotFoundHintsAreTrue(t *testing.T) {
	ctx := context.Background()

	t.Run("the outline lists exactly the ids a block reference resolves", func(t *testing.T) {
		// the property the hint promises. It holds for the top-level run…
		fx := newV2Fixture(t)
		outline := fx.readObject(t, editTableCellChildDoc, V2ObjectQuery{Outline: true})
		var entries []v2model.OutlineEntry
		require.NoError(t, json.Unmarshal(envelopeField(t, outline, "outline"), &entries))
		require.NotEmpty(t, entries)
		for _, entry := range entries {
			_, _, err := fx.GetObject(ctx, testSpaceId, "obj1", V2ObjectQuery{Block: entry.Id})
			assert.NoError(t, err, "outline entry %q must resolve as a block reference", entry.Id)
		}

		// …and the served-but-unaddressable id is NOT in it, which is why
		// the hint may not claim the outline lists every served id
		served := fx.readObject(t, editTableCellChildDoc, V2ObjectQuery{})
		require.Contains(t, string(served), `"id":"dddd1"`, "a default read serves the cell descendant")
		for _, entry := range entries {
			assert.NotEqual(t, "dddd1", entry.Id)
		}
	})

	t.Run("the read 404 names what is addressable instead of promising the outline", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").
			Return(editRead(t, editTableCellChildDoc), nil).Maybe()

		_, _, err := fx.GetObject(ctx, testSpaceId, "obj1", V2ObjectQuery{Block: "dddd1"})

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
		require.NotEmpty(t, apiErr.Issues)
		assertAddressabilityHint(t, apiErr.Issues[0])
	})

	t.Run("the PATCH 404 says the same thing", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editTableCellChildDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceText","id":"dddd1","find":"inside","replace":"x"}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "ops[0].id", apiErr.Issues[0].Path)
		assertAddressabilityHint(t, apiErr.Issues[0])
	})
}

// assertAddressabilityHint pins the honest half of the repair loop: the hint
// scopes the outline's promise to the blocks array and says outright that
// ids nested inside a block are not block references.
func assertAddressabilityHint(t *testing.T, issue v2model.Issue) {
	t.Helper()
	assert.Contains(t, issue.Message, "blocks array")
	assert.Contains(t, issue.Hint, "?outline=true")
	assert.Contains(t, issue.Hint, "not individually addressable",
		"the hint must admit the cell-descendant gap rather than send the caller round the outline again")
	assert.Contains(t, issue.Hint, "setCell", "and name the op that does reach it")
}

// TestServedLabelsAvoidTailCollisions pins §8.28 property 1 at the read:
// buildCompactIds censuses the WHOLE snapshot before any slicing, so two
// minted ids sharing a last-5 tail both keep their full spelling — a label
// is never served that could resolve to two blocks. It was the load-bearing
// claim of §8.28 and was unpinned.
func TestServedLabelsAvoidTailCollisions(t *testing.T) {
	t.Run("two minted ids sharing a tail both stay full", func(t *testing.T) {
		fx := newV2Fixture(t)
		served := fx.readObject(t, editTailCollisionDoc, V2ObjectQuery{})

		ids := blockIdsOf(docBlocks(mustDoc(t, served)))
		assert.Equal(t, []string{"1111111111111111117ffff9", "2222222222222222227ffff9"}, ids)
	})

	t.Run("the census covers a SUBSET read, not just the whole document", func(t *testing.T) {
		// the mechanism §8.28 property 1 rests on: the avoid-set is built
		// over the whole snapshot, so slicing to one block afterwards cannot
		// hand out a label the omitted blocks contest
		fx := newV2Fixture(t)
		served := fx.readObject(t, editTailCollisionDoc, V2ObjectQuery{Block: "1111111111111111117ffff9"})

		ids := blockIdsOf(docBlocks(mustDoc(t, served)))
		assert.Equal(t, []string{"1111111111111111117ffff9"}, ids,
			"a one-block read must not relabel to the tail its hidden twin also claims")
	})

	t.Run("an uncontested minted tail does relabel", func(t *testing.T) {
		// the control: without a collision the same read serves labels, so
		// the assertions above are about the guard and not about relabeling
		// being off
		fx := newV2Fixture(t)
		served := fx.readObject(t, editMintedDoc, V2ObjectQuery{})

		assert.Equal(t, []string{"aaaa1", "bbbb1"}, blockIdsOf(docBlocks(mustDoc(t, served))))
	})
}
