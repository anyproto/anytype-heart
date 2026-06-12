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

func TestLastReconciledLinksHash(t *testing.T) {
	ctx := context.Background()

	t.Run("absent marker is empty", func(t *testing.T) {
		s := NewStoreFixture(t)

		got, err := s.GetLastReconciledLinksHash(ctx, "id1")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("save and load", func(t *testing.T) {
		s := NewStoreFixture(t)
		want := HashLinksList([]string{"obj1", "obj2"})

		require.NoError(t, s.SaveLastReconciledLinksHash(ctx, "id1", want))

		got, err := s.GetLastReconciledLinksHash(ctx, "id1")
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("coexists with heads hash on the same doc", func(t *testing.T) {
		s := NewStoreFixture(t)
		want := HashLinksList([]string{"obj1"})

		require.NoError(t, s.SaveLastIndexedHeadsHash(ctx, "id1", "heads1"))
		require.NoError(t, s.SaveLastReconciledLinksHash(ctx, "id1", want))
		require.NoError(t, s.SaveLastIndexedHeadsHash(ctx, "id1", "heads2"))

		gotMarker, err := s.GetLastReconciledLinksHash(ctx, "id1")
		require.NoError(t, err)
		assert.Equal(t, want, gotMarker)

		gotHeads, err := s.GetLastIndexedHeadsHash(ctx, "id1")
		require.NoError(t, err)
		assert.Equal(t, "heads2", gotHeads)
	})
}

func TestHashLinksList(t *testing.T) {
	t.Run("insensitive to order and duplicates", func(t *testing.T) {
		want := HashLinksList([]string{"a", "b", "c"})

		assert.Equal(t, want, HashLinksList([]string{"c", "a", "b"}))
		assert.Equal(t, want, HashLinksList([]string{"b", "a", "c", "a", "b"}))
	})

	t.Run("empty list has a stable non-empty hash", func(t *testing.T) {
		assert.NotEmpty(t, HashLinksList(nil))
		assert.Equal(t, HashLinksList(nil), HashLinksList([]string{}))
	})

	t.Run("different sets differ", func(t *testing.T) {
		assert.NotEqual(t, HashLinksList([]string{"a"}), HashLinksList([]string{"b"}))
		assert.NotEqual(t, HashLinksList([]string{"a"}), HashLinksList([]string{"a", "b"}))
		assert.NotEqual(t, HashLinksList(nil), HashLinksList([]string{"a"}))
	})

	t.Run("does not mutate the input", func(t *testing.T) {
		ids := []string{"c", "a", "b"}
		HashLinksList(ids)
		assert.Equal(t, []string{"c", "a", "b"}, ids)
	})
}
