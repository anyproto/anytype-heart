package keyvaluestore

import (
	"context"
	"path/filepath"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixture struct {
	Store[string]
}

func newFixture(t *testing.T) *fixture {
	db, err := anystore.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	kv, err := New(db, "test", func(v string) ([]byte, error) {
		return []byte(v), nil
	}, func(v []byte) (string, error) {
		return string(v), nil
	})
	require.NoError(t, err)

	return &fixture{kv}
}

func TestStore(t *testing.T) {
	kv := newFixture(t)

	key := "foo"

	t.Run("Set", func(t *testing.T) {
		ok, err := kv.Has(context.Background(), key)
		require.NoError(t, err)
		assert.False(t, ok)

		err = kv.Set(context.Background(), key, "bar")
		require.NoError(t, err)

		ok, err = kv.Has(context.Background(), key)
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("Get", func(t *testing.T) {
		t.Run("existing item", func(t *testing.T) {
			got, err := kv.Get(context.Background(), key)
			require.NoError(t, err)
			assert.Equal(t, "bar", got)
		})
		t.Run("non-existing item", func(t *testing.T) {
			_, err := kv.Get(context.Background(), "non-existing")
			require.Equal(t, anystore.ErrDocNotFound, err)
		})
	})

	t.Run("Delete", func(t *testing.T) {
		t.Run("non-existing item", func(t *testing.T) {
			err := kv.Delete(context.Background(), "non-existing")
			require.NoError(t, err)
		})

		t.Run("existing item", func(t *testing.T) {
			err := kv.Delete(context.Background(), key)
			require.NoError(t, err)

			ok, err := kv.Has(context.Background(), key)
			require.NoError(t, err)
			assert.False(t, ok)
		})
	})
}

func TestListAllValues(t *testing.T) {
	kv := newFixture(t)

	err := kv.Set(context.Background(), "a1", "1")
	require.NoError(t, err)

	err = kv.Set(context.Background(), "a2", "2")
	require.NoError(t, err)

	got, err := kv.ListAllValues(context.Background())
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"1", "2"}, got)
}
