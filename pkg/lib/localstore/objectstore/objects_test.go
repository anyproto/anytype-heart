package objectstore

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/schema"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore/clientds"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/ftsearch"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	"github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/textileio/go-threads/core/thread"
)

func TestDsObjectStore_UpdateLocalDetails(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(tmpDir)
	app := testapp.New()
	defer app.Close()

	ds := New()

	id, err := threads.ThreadCreateID(thread.AccessControlled, smartblock.SmartBlockTypePage)
	require.NoError(t, err)

	err = app.With(&config.DefaultConfig).With(wallet.NewWithRepoPathAndKeys(tmpDir, nil, nil)).With(clientds.New()).With(ds).Start()
	require.NoError(t, err)
	// bundle.RelationKeyLastOpenedDate is local relation (not stored in the changes tree)
	err = ds.CreateObject(id.String(), &types.Struct{
		Fields: map[string]*types.Value{bundle.RelationKeyLastOpenedDate.String(): pbtypes.Int64(4), "type": pbtypes.String("_otp1")},
	}, nil, "")
	require.NoError(t, err)

	ot := &model.ObjectType{Url: "_otp1", Name: "otp1"}
	recs, _, err := ds.Query(schema.NewByType(ot, nil), database.Query{})
	require.NoError(t, err)
	require.Len(t, recs, 1)
	require.Equal(t, pbtypes.Int64(4), pbtypes.Get(recs[0].Details, bundle.RelationKeyLastOpenedDate.String()))

	err = ds.UpdateObjectDetails(id.String(), &types.Struct{
		Fields: map[string]*types.Value{"k1": pbtypes.String("1"), "k2": pbtypes.String("2"), "type": pbtypes.String("_otp1")},
	}, true)
	require.NoError(t, err)

	recs, _, err = ds.Query(schema.NewByType(ot, nil), database.Query{})
	require.NoError(t, err)
	require.Len(t, recs, 1)
	require.Equal(t, pbtypes.Int64(4), pbtypes.Get(recs[0].Details, bundle.RelationKeyLastOpenedDate.String()))
	require.Equal(t, "2", pbtypes.GetString(recs[0].Details, "k2"))

	err = ds.UpdateObjectDetails(id.String(), &types.Struct{
		Fields: map[string]*types.Value{"k1": pbtypes.String("1"), "k2": pbtypes.String("2"), "type": pbtypes.String("_otp1")},
	}, false)
	require.NoError(t, err)

	recs, _, err = ds.Query(schema.NewByType(ot, nil), database.Query{})
	require.NoError(t, err)
	require.Len(t, recs, 1)
	require.Nil(t, pbtypes.Get(recs[0].Details, bundle.RelationKeyLastOpenedDate.String()))
	require.Equal(t, "2", pbtypes.GetString(recs[0].Details, "k2"))
}

func TestDsObjectStore_IndexQueue(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(tmpDir)

	app := testapp.New()
	defer app.Close()

	ds := New()
	err := app.With(&config.DefaultConfig).With(wallet.NewWithRepoPathAndKeys(tmpDir, nil, nil)).With(clientds.New()).With(ds).Start()
	require.NoError(t, err)

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
			// should be still removed from the queue
			return fmt.Errorf("test err")
		}
	}))
	assert.Equal(t, 2, count)
	count = 0
	require.NoError(t, ds.AddToIndexQueue("two"))
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

	app := testapp.New()
	defer app.Close()

	ds := New()
	err := app.With(&config.DefaultConfig).With(wallet.NewWithRepoPathAndKeys(tmpDir, nil, nil)).With(clientds.New()).With(ftsearch.New()).With(ds).Start()
	require.NoError(t, err)
	fts := app.MustComponent(ftsearch.CName).(ftsearch.FTSearch)

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
	id1 := tid1.String()
	id2 := tid2.String()
	id3 := tid3.String()
	require.NoError(t, ds.CreateObject(id1, newDet("one"), nil, "s1"))
	require.NoError(t, ds.CreateObject(id2, newDet("two"), nil, "s2"))
	require.NoError(t, ds.CreateObject(id3, newDet("three"), nil, "s3"))
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
func getId() string {
	thrdId, err := threads.ThreadCreateID(thread.AccessControlled, smartblock.SmartBlockTypePage)
	if err != nil {
		panic(err)
	}

	return thrdId.String()
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

func Test_removeByPrefix(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(tmpDir)

	logging.ApplyLevelsFromEnv()
	app := testapp.New()
	defer app.Close()
	ds := New()
	err := app.With(&config.DefaultConfig).With(wallet.NewWithRepoPathAndKeys(tmpDir, nil, nil)).With(clientds.New()).With(ftsearch.New()).With(ds).Start()
	require.NoError(t, err)

	ds2 := ds.(*dsObjectStore)
	var key = make([]byte, 32)
	for i := 0; i < 10; i++ {

		var links []string
		rand.Seed(time.Now().UnixNano())
		rand.Read(key)
		objId := fmt.Sprintf("%x", key)

		for j := 0; j < 8000; j++ {
			rand.Seed(time.Now().UnixNano())
			rand.Read(key)
			links = append(links, fmt.Sprintf("%x", key))
		}
		require.NoError(t, ds.CreateObject(objId, nil, links, ""))
	}
	tx, err := ds2.ds.NewTransaction(false)
	_, err = removeByPrefixInTx(tx, pagesInboundLinksBase.String())
	require.NotNil(t, err)
	tx.Discard()

	got, err := removeByPrefix(ds2.ds, pagesInboundLinksBase.String())
	require.NoError(t, err)
	require.Equal(t, 10*8000, got)

	got, err = removeByPrefix(ds2.ds, pagesOutboundLinksBase.String())
	require.NoError(t, err)
	require.Equal(t, 10*8000, got)
}
