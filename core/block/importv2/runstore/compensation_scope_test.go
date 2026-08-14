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
}
