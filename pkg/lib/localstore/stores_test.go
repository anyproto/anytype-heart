package localstore

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	badger "github.com/ipfs/go-ds-badger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_AddIndex(t *testing.T) {
	ds, err := badger.NewDatastore(filepath.Join(os.TempDir(), "anytypetestds"), &badger.DefaultOptions)
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
		err = AddIndex(idx, ds, item, "primkey1")
		require.NoError(t, err)
	}

	txn, err := ds.NewTransaction(true)
	require.NoError(t, err)

	defer txn.Discard()

	key, err := GetKeyByIndex(idxs[0], txn, item)
	require.NoError(t, err)
	require.Equal(t, "primkey1", key)

	key, err = GetKeyByIndex(idxs[1], txn, item)
	require.True(t, err != nil)

	results, err := GetKeysByIndexParts(txn, idxs[1].Prefix, idxs[1].Name, []string{item.Slice[0]}, "", false, 1)
	require.NoError(t, err)

	res := <-results.Next()
	require.NotNil(t, res)

	require.NoError(t, res.Error)
	require.Equal(t, "/idx/items/slice/s1/primkey1", res.Key)
}

func TestCarveKeyParts(t *testing.T) {
	cases := []struct {
		key      string
		from, to int
		expected string
	}{
		{
			key:      "/a/b/c/d",
			from:     -1,
			to:       0,
			expected: "d",
		},
		{
			key:      "/a/b/c/d",
			from:     -2,
			to:       0,
			expected: "c/d",
		},
		{
			key:      "/a/b/c/d",
			from:     -2,
			to:       -1,
			expected: "c",
		},
		{
			key:      "/a/b/c/d",
			from:     1,
			to:       -1,
			expected: "b/c",
		},
	}

	for _, tt := range cases {
		result, err := CarveKeyParts(tt.key, tt.from, tt.to)
		assert.NoError(t, err)
		assert.Equal(t, tt.expected, result)
	}
}

func Test_RunLargeOperationWithRetries(t *testing.T) {
	tempDir, err := ioutil.TempDir(os.TempDir(), "anytypetestds*")
	require.NoError(t, err)

	ds, err := badger.NewDatastore(tempDir, &badger.DefaultOptions)
	require.NoError(t, err)

	index := Index{
		Prefix: "test1",
		Name:   "test1",
		Keys: func(val interface{}) []IndexKeyParts {
			if v, ok := val.(int); ok {
				return []IndexKeyParts{[]string{fmt.Sprintf("%d", v)}}
			}
			return nil
		},
		Unique:             false,
		Hash:               false,
		SplitIndexKeyParts: false,
	}

	for i := 0; i < 30000; i++ {
		err = AddIndex(index, ds, i, "2")
		require.NoError(t, err)
	}

	targetPrefix := IndexBase.ChildString(index.Prefix)

	tx, err := ds.NewTransaction(false)
	require.NoError(t, err)
	res, err := GetKeys(tx, targetPrefix.String(), 0)
	require.NoError(t, err)
	total, err := CountAllKeysFromResults(res)
	require.NoError(t, err)
	require.Equal(t, 30000, total)

	err = EraseIndexWithTxn(index, tx)
	require.Error(t, err, errTxnTooBig)
	tx.Discard()

	err = EraseIndex(index, ds)
	require.NoError(t, err)
	tx, err = ds.NewTransaction(false)
	require.NoError(t, err)
	res, err = GetKeys(tx, index.Prefix, 0)
	require.NoError(t, err)
	total, err = CountAllKeysFromResults(res)
	require.NoError(t, err)
	require.Equal(t, 0, total)

	err = ds.Close()
	require.NoError(t, err)
	err = os.RemoveAll(tempDir)
	require.NoError(t, err)

}
