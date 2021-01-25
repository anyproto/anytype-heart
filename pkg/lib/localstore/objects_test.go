package localstore

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/threads"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/ftsearch"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/types"
	badger "github.com/ipfs/go-ds-badger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/textileio/go-threads/core/thread"
)

func TestDsObjectStore_IndexQueue(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(tmpDir)

	bds, err := badger.NewDatastore(tmpDir, nil)
	require.NoError(t, err)

	ds := NewObjectStore(bds, nil)

	require.NoError(t, ds.AddToIndexQueue("one"))
	require.NoError(t, ds.AddToIndexQueue("one"))
	require.NoError(t, ds.AddToIndexQueue("two"))
	var count int
	require.NoError(t, ds.IndexForEach(func(id string, tm time.Time) error {
		assert.NotEqual(t, -1, slice.FindPos([]string{"one", "two"}, id))
		assert.NotEmpty(t, tm)
		count++
		if id == "one" {
			return nil
		} else {
			return fmt.Errorf("test err")
		}
	}))
	assert.Equal(t, 2, count)
	count = 0
	require.NoError(t, ds.IndexForEach(func(id string, tm time.Time) error {
		assert.Equal(t, "two", id)
		assert.NotEmpty(t, tm)
		count++
		return nil
	}))
	assert.Equal(t, 1, count)

	count = 0
	require.NoError(t, ds.IndexForEach(func(id string, tm time.Time) error {
		count++
		return nil
	}))

	assert.Equal(t, 0, count)

	require.NoError(t, ds.AddToIndexQueue("one"))
	require.NoError(t, ds.AddToIndexQueue("one"))
	require.NoError(t, ds.AddToIndexQueue("two"))

	count = 0
	require.NoError(t, ds.IndexForEach(func(id string, tm time.Time) error {
		count++
		return nil
	}))
	assert.Equal(t, 2, count)
}

func TestDsObjectStore_Query(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(tmpDir)

	fts, err := ftsearch.NewFTSearch(filepath.Join(tmpDir, "fts"))
	require.NoError(t, err)

	bds, err := badger.NewDatastore(tmpDir, nil)
	require.NoError(t, err)

	ds := NewObjectStore(bds, fts)
	defer ds.Close()
	newDet := func(name string) *types.Struct {
		return &types.Struct{
			Fields: map[string]*types.Value{
				"name": pbtypes.String(name),
			},
		}
	}
	tid1, _ := threads.ThreadCreateID(thread.AccessControlled, smartblock.SmartBlockTypePage)
	tid2, _ := threads.ThreadCreateID(thread.AccessControlled, smartblock.SmartBlockTypePage)
	tid3, _ := threads.ThreadCreateID(thread.AccessControlled, smartblock.SmartBlockTypePage)
	id1 := string(tid1)
	id2 := string(tid2)
	id3 := string(tid3)
	require.NoError(t, ds.UpdateObject(id1, newDet("one"), nil, nil, "s1"))
	require.NoError(t, ds.UpdateObject(id2, newDet("two"), nil, nil, "s2"))
	require.NoError(t, ds.UpdateObject(id3, newDet("three"), nil, nil, "s3"))
	require.NoError(t, fts.Index(ftsearch.SearchDoc{
		Id:    id1,
		Title: "one",
		Text:  "text twoone uniqone",
	}))
	require.NoError(t, fts.Index(ftsearch.SearchDoc{
		Id:    id2,
		Title: "two",
		Text:  "twoone text twoone uniqtwo",
	}))
	require.NoError(t, fts.Index(ftsearch.SearchDoc{
		Id:    id3,
		Title: "three",
		Text:  "text uniqthree",
	}))

	// should return all records
	rec, tot, err := ds.Query(nil, database.Query{})
	require.NoError(t, err)
	assert.Equal(t, 3, tot)
	assert.Len(t, rec, 3)

	// filter
	rec, tot, err = ds.Query(nil, database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Operator:    model.BlockContentDataviewFilter_And,
				RelationKey: "name",
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String("two"),
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, tot)
	assert.Len(t, rec, 1)

	// fulltext
	rec, tot, err = ds.Query(nil, database.Query{
		FullText: "twoone",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, tot)
	assert.Len(t, rec, 2)
	var names []string
	for _, r := range rec {
		names = append(names, pbtypes.GetString(r.Details, "name"))
	}
	assert.Equal(t, []string{"two", "one"}, names)

	// fulltext + filter
	rec, tot, err = ds.Query(nil, database.Query{
		FullText: "twoone",
		Filters: []*model.BlockContentDataviewFilter{
			{
				Operator:    model.BlockContentDataviewFilter_And,
				RelationKey: "name",
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String("one"),
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, tot)
	assert.Len(t, rec, 1)
}
