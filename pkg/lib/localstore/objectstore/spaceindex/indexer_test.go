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
}
