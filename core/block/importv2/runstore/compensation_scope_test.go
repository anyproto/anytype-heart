package runstore

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompensationScope(t *testing.T) {
	ctx := context.Background()

	t.Run("claims are NOT deletable before materialization began", func(t *testing.T) {
		// given — A1 (CONFIRMED): pass-1 claims are pure intent; before
		// pass 3 starts nothing exists in the space, so a suspended crawl
		// must sweep to ZERO deletes, not one per claim.
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.RecordClaims(ctx, []ClaimRecord{
			{SourceKey: "p1", ObjectId: "obj-1", PayloadRoot: []byte("r")},
			{SourceKey: "p2", ObjectId: "obj-2", PayloadRoot: []byte("r")},
		}))

		// when
		inputs, err := store.CompensationInputs(ctx)

		// then
		require.NoError(t, err)
		assert.Empty(t, inputs.Created, "intent-only claims must never be deleted")

		// and: once materialization began, a still-claimed row IS a
		// possible-create (the crash window) and joins the delete set
		require.NoError(t, store.SetState(ctx, StateMaterializing))
		inputs, err = store.CompensationInputs(ctx)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"obj-1", "obj-2"}, inputs.Created)

		// and: the marker is sticky through a later suspend
		require.NoError(t, store.SetState(ctx, StateSuspended))
		inputs, err = store.CompensationInputs(ctx)
		require.NoError(t, err)
		assert.Len(t, inputs.Created, 2)
	})

	t.Run("an own file deduped twice leaks WITH a trace, never silently", func(t *testing.T) {
		// given — two source files with identical bytes: the first upload is
		// fresh (the run created the object), the second dedups onto it and
		// classifies pre-existing only because the first upload indexed it.
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.RecordFile(ctx, "assets/a.png", "file-HASH", false))
		require.NoError(t, store.RecordFile(ctx, "assets/copy.png", "file-HASH", true))

		// when
		inputs, err := store.CompensationInputs(ctx)

		// then: the id-scoped protection stands (this exact ledger shape is
		// also what a crashed-then-resumed re-upload of a GENUINE user file
		// leaves, so deleting on it would be a guess) — but the run's own
		// fresh-upload row proves something was dropped, and a leak must
		// never leave the record without a word: the id joins Updated, the
		// derived-claimed skip's rule.
		require.NoError(t, err)
		assert.Empty(t, inputs.OwnedFiles,
			"id-scoped protection: never delete what any row says pre-existed")
		assert.Equal(t, []string{"file-HASH"}, inputs.Updated,
			"the dropped own-file id must leave a trace in the record")
	})

	t.Run("a purely pre-existing dedup stays silent", func(t *testing.T) {
		// given — the common case: the upload deduped onto the user's file;
		// no row claims ownership, nothing of the run's was dropped, so
		// there is nothing to say.
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.RecordFile(ctx, "assets/a.png", "file-USER", true))

		// when
		inputs, err := store.CompensationInputs(ctx)

		// then
		require.NoError(t, err)
		assert.Empty(t, inputs.OwnedFiles)
		assert.Empty(t, inputs.Updated, "an untouched pre-existing file needs no trace")
	})
}
