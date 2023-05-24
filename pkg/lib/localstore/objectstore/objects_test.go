package objectstore

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"
	"time"

	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	"github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/app/testapp"
	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/clientds"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/util/slice"
)

func TestDsObjectStore_IndexQueue(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(tmpDir)

	app := testapp.New()
	defer app.Close(context.Background())

	ds := New(nil)
	err := app.With(&config.DefaultConfig).With(wallet.NewWithRepoDirAndRandomKeys(tmpDir)).With(clientds.New()).With(ds).Start(context.Background())
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

func TestDsObjectStore_PrefixQuery(t *testing.T) {
	bds := sync.MutexWrap(ds.NewMapDatastore())
	err := bds.Put(context.Background(), ds.NewKey("/p1/abc/def/1"), []byte{})

	require.NoError(t, err)

	res, err := bds.Query(context.Background(), query.Query{Prefix: "/p1/abc", KeysOnly: true})
	require.NoError(t, err)

	entries, err := res.Rest()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "/p1/abc/def/1", entries[0].Key)

}

func Test_removeByPrefix(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(tmpDir)

	app := testapp.New()
	defer app.Close(context.Background())
	ds := New(nil)
	err := app.With(&config.DefaultConfig).With(wallet.NewWithRepoDirAndRandomKeys(tmpDir)).With(clientds.New()).With(ftsearch.New()).With(ds).Start(context.Background())
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
