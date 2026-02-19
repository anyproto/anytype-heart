package persistentqueue

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStorage(t *testing.T) {
	db := newAnystore(t)
	defer db.Close()

	storage, err := NewAnystoreStorage(db, "test", makeTestItem)
	require.NoError(t, err)

	item1 := &testItem{
		Id:        "123",
		Timestamp: 123,
		Data:      "foo",
	}
	item2 := &testItem{
		Id:        "456",
		Timestamp: 456,
		Data:      "bar",
	}

	err = storage.Put(item1)
	require.NoError(t, err)

	err = storage.Put(item2)
	require.NoError(t, err)

	got, err := storage.List()
	require.NoError(t, err)

	require.ElementsMatch(t, []*testItem{item1, item2}, got)

	err = storage.Delete(item1.Key())
	require.NoError(t, err)

	got, err = storage.List()
	require.ElementsMatch(t, []*testItem{item2}, got)
}
