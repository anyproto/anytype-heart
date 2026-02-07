package spaceindex

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHeadsHash(t *testing.T) {
	ctx := context.Background()

	t.Run("previous hash is not found", func(t *testing.T) {
		s := NewStoreFixture(t)

		got, err := s.GetLastIndexedHeadsHash(ctx, "id1")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("save and load hash", func(t *testing.T) {
		s := NewStoreFixture(t)

		want := "hash1"

		require.NoError(t, s.SaveLastIndexedHeadsHash(ctx, "id1", want))

		got, err := s.GetLastIndexedHeadsHash(ctx, "id1")
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("clear heads state removes all hashes", func(t *testing.T) {
		s := NewStoreFixture(t)

		// Save multiple hashes
		require.NoError(t, s.SaveLastIndexedHeadsHash(ctx, "id1", "hash1"))
		require.NoError(t, s.SaveLastIndexedHeadsHash(ctx, "id2", "hash2"))
		require.NoError(t, s.SaveLastIndexedHeadsHash(ctx, "id3", "hash3"))

		// Verify they exist
		got, err := s.GetLastIndexedHeadsHash(ctx, "id1")
		require.NoError(t, err)
		assert.Equal(t, "hash1", got)

		// Clear all heads state
		require.NoError(t, s.ClearHeadsState(ctx))

		// Verify all hashes are gone
		got, err = s.GetLastIndexedHeadsHash(ctx, "id1")
		require.NoError(t, err)
		assert.Empty(t, got)

		got, err = s.GetLastIndexedHeadsHash(ctx, "id2")
		require.NoError(t, err)
		assert.Empty(t, got)

		got, err = s.GetLastIndexedHeadsHash(ctx, "id3")
		require.NoError(t, err)
		assert.Empty(t, got)

		// Verify we can save new hashes after clearing
		require.NoError(t, s.SaveLastIndexedHeadsHash(ctx, "id4", "hash4"))
		got, err = s.GetLastIndexedHeadsHash(ctx, "id4")
		require.NoError(t, err)
		assert.Equal(t, "hash4", got)
	})
}
