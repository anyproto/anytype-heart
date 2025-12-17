package filequeue

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testItem struct {
	id    string
	error bool
}

func TestSkipUnmarshalError(t *testing.T) {
	ctx := context.Background()
	db, err := anystore.Open(ctx, filepath.Join(t.TempDir(), "store.db"), nil)
	require.NoError(t, err)

	coll, err := db.Collection(ctx, "queue")
	require.NoError(t, err)

	store := NewStorage(coll, func(arena *anyenc.Arena, val testItem) *anyenc.Value {
		obj := arena.NewObject()
		obj.Set("id", arena.NewString(val.id))
		obj.Set("error", newBool(arena, val.error))
		return obj
	}, func(v *anyenc.Value) (testItem, error) {
		id := v.GetString("id")
		isError := v.GetBool("error")
		if isError {
			return testItem{}, fmt.Errorf("unexpected error")
		}
		return testItem{
			id: id,
		}, nil
	})

	err = store.set(ctx, "1", testItem{id: "1"})
	require.NoError(t, err)

	err = store.set(ctx, "2", testItem{id: "2", error: true})
	require.NoError(t, err)

	items, err := store.listAll(ctx)
	require.NoError(t, err)

	assert.Equal(t, []testItem{
		{id: "1"},
	}, items)
}
