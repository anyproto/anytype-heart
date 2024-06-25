package database

import (
	"context"
	"path/filepath"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestAnystoreFilter(t *testing.T) {
	ctx := context.Background()
	db, err := anystore.Open(ctx, filepath.Join(t.TempDir(), "temp.db"), nil)
	require.NoError(t, err)

	objects, err := db.Collection(ctx, "objects")
	require.NoError(t, err)

	err = objects.Insert(ctx, map[string]any{
		"name": "John",
		"age":  30,
	})
	require.NoError(t, err)

	flt := FilterEq{
		Key:   "age",
		Value: pbtypes.Int64(29),
		Cond:  model.BlockContentDataviewFilter_Greater,
	}.Compile()

	iter, err := objects.Find(flt).Iter(ctx)
	require.NoError(t, err)

	var count int
	for iter.Next() {
		var obj map[string]any
		doc, err := iter.Doc()
		require.NoError(t, err)

		err = doc.Decode(&obj)
		require.NoError(t, err)
		require.Equal(t, "John", obj["name"])
		require.Equal(t, float64(30), obj["age"])
		count++
	}
	err = iter.Close()
	require.NoError(t, err)

	require.Equal(t, 1, count)
}
