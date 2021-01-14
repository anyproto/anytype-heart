package localstore

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/ftsearch"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	"github.com/ipfs/go-datastore/sync"
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
	id1 := thread.NewIDV1(thread.Raw, 32).String()
	id2 := thread.NewIDV1(thread.Raw, 32).String()
	id3 := thread.NewIDV1(thread.Raw, 32).String()
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

func TestDsObjectStore_PrefixQuery(t *testing.T) {
	bds := sync.MutexWrap(ds.NewMapDatastore())
	err := bds.Put(ds.NewKey("/p1/abc/def/1"), []byte{})

	require.NoError(t, err)

	res, err := bds.Query(query.Query{Prefix: "/p1/abc", KeysOnly: true})
	require.NoError(t, err)

	entries, err := res.Rest()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "/p1/abc/def/1", entries[0].Key)

}
func TestDsObjectStore_RelationsIndex(t *testing.T) {
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
	id1 := thread.NewIDV1(thread.Raw, 32).String()
	id2 := thread.NewIDV1(thread.Raw, 32).String()
	id3 := thread.NewIDV1(thread.Raw, 32).String()
	require.NoError(t, ds.UpdateObject(id1, newDet("one"), &pbrelation.Relations{Relations: []*pbrelation.Relation{
		{
			Key:          "rel1",
			Format:       pbrelation.RelationFormat_status,
			Name:         "rel 1",
			DefaultValue: nil,
			SelectDict: []*pbrelation.RelationSelectOption{
				{"id1", "option1", "red"},
				{"id2", "option2", "red"},
				{"id3", "option3", "red"},
			},
		},
		{
			Key:          "rel2",
			Format:       pbrelation.RelationFormat_title,
			Name:         "rel 2",
			DefaultValue: nil,
		},
	}}, nil, "s1"))

	require.NoError(t, ds.UpdateObject(id2, newDet("two"), &pbrelation.Relations{Relations: []*pbrelation.Relation{
		{
			Key:          "rel1",
			Format:       pbrelation.RelationFormat_status,
			Name:         "rel 1",
			DefaultValue: nil,
			SelectDict: []*pbrelation.RelationSelectOption{
				{"id3", "option3", "yellow"},
				{"id4", "option4", "red"},
				{"id5", "option5", "red"},
			},
		},
		{
			Key:          "rel3",
			Format:       pbrelation.RelationFormat_status,
			Name:         "rel 3",
			DefaultValue: nil,
			SelectDict: []*pbrelation.RelationSelectOption{
				{"id5", "option5", "red"},
				{"id6", "option6", "red"},
			},
		},
		{
			Key:          "rel4",
			Format:       pbrelation.RelationFormat_tag,
			Name:         "rel 4",
			DefaultValue: nil,
			SelectDict: []*pbrelation.RelationSelectOption{
				{"id7", "option7", "red"},
			},
		},
	}}, nil, "s2"))
	require.NoError(t, ds.UpdateObject(id3, newDet("three"), nil, nil, "s3"))

	_, restOpts, err := ds.GetAggregatedOptionsForRelation("rel1", "htt")
	require.NoError(t, err)
	require.Len(t, restOpts, 5)

	options, err := ds.GetAggregatedOptionsForFormat(pbrelation.RelationFormat_status)
	require.NoError(t, err)
	require.Len(t, options, 6)

}
