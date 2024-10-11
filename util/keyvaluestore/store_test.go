package keyvaluestore

import (
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore(t *testing.T) {
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	kv := New(db, []byte("test/"), func(v string) ([]byte, error) {
		return []byte(v), nil
	}, func(v []byte) (string, error) {
		return string(v), nil
	})

	key := "foo"

	t.Run("Set", func(t *testing.T) {
		ok, err := kv.Has(key)
		require.NoError(t, err)
		assert.False(t, ok)

		err = kv.Set(key, "bar")
		require.NoError(t, err)

		ok, err = kv.Has(key)
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("Get", func(t *testing.T) {
		t.Run("existing item", func(t *testing.T) {
			got, err := kv.Get(key)
			require.NoError(t, err)
			assert.Equal(t, "bar", got)
		})
		t.Run("non-existing item", func(t *testing.T) {
			_, err := kv.Get("non-existing")
			require.Equal(t, ErrNotFound, err)
		})
	})

	t.Run("Delete", func(t *testing.T) {
		t.Run("non-existing item", func(t *testing.T) {
			err = kv.Delete("non-existing")
			require.NoError(t, err)
		})

		t.Run("existing item", func(t *testing.T) {
			err = kv.Delete(key)
			require.NoError(t, err)

			ok, err := kv.Has(key)
			require.NoError(t, err)
			assert.False(t, ok)
		})
	})
}
