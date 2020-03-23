package localstore

import (
	"os"
	"path/filepath"
	"testing"

	ipfslite "github.com/hsanjuan/ipfs-lite"
	"github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/require"
)

func Test_AddIndex(t *testing.T) {
	ds, err := ipfslite.BadgerDatastore(filepath.Join(os.TempDir(), "anytypetestds"))
	require.NoError(t, err)

	type Item struct {
		PrimKey string
		Field1  string
		Field2  string
		Slice   []string
	}

	idxs := []Index{
		{
			Prefix: "items",
			Name:   "field1_field2",
			Keys: func(val interface{}) []IndexKeyParts {
				if v, ok := val.(Item); ok {
					return []IndexKeyParts{[]string{v.Field1, v.Field2}}
				}
				return nil
			},
			Unique: true,
			Hash:   false,
		},
		{
			Prefix: "items",
			Name:   "slice",
			Keys: func(val interface{}) []IndexKeyParts {
				if v, ok := val.(Item); ok {
					return []IndexKeyParts{[]string{v.Slice[0]}, []string{v.Slice[1]}}
				}
				return nil
			},
			Unique: true,
			Hash:   false,
		},
	}

	item := Item{
		Field1: "val1",
		Field2: "val2",
		Slice:  []string{"s1", "s2"},
	}

	for _, idx := range idxs {
		err = AddIndex(idx, ds.(datastore.TxnDatastore), item, "primkey1")
		require.NoError(t, err)
	}

	key, err := GetKeyByIndex(idxs[0], ds.(datastore.TxnDatastore), item)
	require.NoError(t, err)
	require.Equal(t, "primkey1", key)

	key, err = GetKeyByIndex(idxs[1], ds.(datastore.TxnDatastore), item)
	require.True(t, err != nil)

	results, err := GetKeysByIndexParts(ds.(datastore.TxnDatastore), idxs[1].Prefix, idxs[1].Name, []string{item.Slice[0]}, false, 1)
	require.NoError(t, err)

	res := <-results.Next()
	require.NotNil(t, res)

	require.NoError(t, res.Error)
	require.Equal(t, "/idx/items/slice/s1/primkey1", res.Key)
}
