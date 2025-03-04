package badgerstorage

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage/oldstorage"
	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type oldTreeStorage interface {
	oldstorage.ChangesIterator
	oldstorage.TreeStorage
}

func treeTestPayload() treestorage.TreeStorageCreatePayload {
	rootRawChange := &treechangeproto.RawTreeChangeWithId{RawChange: []byte("some"), Id: "someRootId"}
	otherChange := &treechangeproto.RawTreeChangeWithId{RawChange: []byte("some other"), Id: "otherId"}
	changes := []*treechangeproto.RawTreeChangeWithId{rootRawChange, otherChange}
	return treestorage.TreeStorageCreatePayload{
		RootRawChange: rootRawChange,
		Changes:       changes,
		Heads:         []string{rootRawChange.Id},
	}
}

type fixture struct {
	dir string
	db  *badger.DB
}

func testTreePayload(t *testing.T, store oldstorage.TreeStorage, payload treestorage.TreeStorageCreatePayload) {
	require.Equal(t, payload.RootRawChange.Id, store.Id())

	root, err := store.Root()
	require.NoError(t, err)
	require.Equal(t, root, payload.RootRawChange)

	heads, err := store.Heads()
	require.NoError(t, err)
	require.Equal(t, payload.Heads, heads)

	for _, ch := range payload.Changes {
		dbCh, err := store.GetRawChange(context.Background(), ch.Id)
		require.NoError(t, err)
		require.Equal(t, ch, dbCh)
	}
	return
}

func newFixture(t *testing.T) *fixture {
	dir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	return &fixture{dir: dir}
}

func (fx *fixture) open(t *testing.T) {
	var err error
	fx.db, err = badger.Open(badger.DefaultOptions(fx.dir))
	require.NoError(t, err)
}

func (fx *fixture) stop(t *testing.T) {
	require.NoError(t, fx.db.Close())
}

func (fx *fixture) testNoKeysExist(t *testing.T, spaceId, treeId string) {
	treeKeys := newTreeKeys(spaceId, treeId)

	var keys [][]byte
	err := fx.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Prefix = treeKeys.RawChangePrefix()

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := item.Key()
			keyCopy := make([]byte, 0, len(key))
			keyCopy = item.KeyCopy(key)
			keys = append(keys, keyCopy)
		}
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, 0, len(keys))

	err = fx.db.View(func(txn *badger.Txn) error {
		_, err = getTxn(txn, treeKeys.RootIdKey())
		require.Equal(t, err, badger.ErrKeyNotFound)

		_, err = getTxn(txn, treeKeys.HeadsKey())
		require.Equal(t, err, badger.ErrKeyNotFound)

		return nil
	})
}

func TestTreeStorage_Create(t *testing.T) {
	fx := newFixture(t)
	fx.open(t)
	defer fx.stop(t)

	spaceId := "spaceId"
	payload := treeTestPayload()
	store, err := createTreeStorage(fx.db, spaceId, payload)
	require.NoError(t, err)
	testTreePayload(t, store, payload)

	t.Run("create same storage returns error", func(t *testing.T) {
		_, err := createTreeStorage(fx.db, spaceId, payload)
		require.Error(t, err)
	})
}

func TestTreeStorage_Methods(t *testing.T) {
	fx := newFixture(t)
	fx.open(t)
	payload := treeTestPayload()
	spaceId := "spaceId"
	_, err := createTreeStorage(fx.db, spaceId, payload)
	require.NoError(t, err)
	fx.stop(t)

	fx.open(t)
	defer fx.stop(t)
	treeStore, err := newTreeStorage(fx.db, spaceId, payload.RootRawChange.Id)
	require.NoError(t, err)
	store := treeStore.(oldTreeStorage)
	testTreePayload(t, store, payload)

	t.Run("update heads", func(t *testing.T) {
		newHeads := []string{"a", "b"}
		require.NoError(t, store.SetHeads(newHeads))
		heads, err := store.Heads()
		require.NoError(t, err)
		require.Equal(t, newHeads, heads)
	})

	t.Run("add raw change, get change and has change", func(t *testing.T) {
		newChange := &treechangeproto.RawTreeChangeWithId{RawChange: []byte("ab"), Id: "id10"}
		require.NoError(t, store.AddRawChange(newChange))
		rawCh, err := store.GetRawChange(context.Background(), newChange.Id)
		require.NoError(t, err)
		require.Equal(t, newChange, rawCh)
		has, err := store.HasChange(context.Background(), newChange.Id)
		require.NoError(t, err)
		require.True(t, has)
	})

	t.Run("get and has for unknown change", func(t *testing.T) {
		incorrectId := "incorrectId"
		_, err := store.GetRawChange(context.Background(), incorrectId)
		require.Error(t, err)
		has, err := store.HasChange(context.Background(), incorrectId)
		require.NoError(t, err)
		require.False(t, has)
	})

	t.Run("iterate changes", func(t *testing.T) {
		newChange := &treechangeproto.RawTreeChangeWithId{RawChange: []byte("foo"), Id: "id01"}
		require.NoError(t, store.AddRawChange(newChange))
		newChange = &treechangeproto.RawTreeChangeWithId{RawChange: []byte("bar"), Id: "id20"}
		require.NoError(t, store.AddRawChange(newChange))

		var collected []*treechangeproto.RawTreeChangeWithId
		require.NoError(t, store.IterateChanges(func(id string, rawChange []byte) error {
			collected = append(collected, &treechangeproto.RawTreeChangeWithId{
				Id:        id,
				RawChange: bytes.Clone(rawChange),
			})
			return nil
		}))

		want := []*treechangeproto.RawTreeChangeWithId{
			{Id: "id01", RawChange: []byte("foo")},
			{Id: "id10", RawChange: []byte("ab")},
			{Id: "id20", RawChange: []byte("bar")},
			{Id: "otherId", RawChange: []byte("some other")},
			{Id: "someRootId", RawChange: []byte("some")},
		}
		assert.Equal(t, want, collected)

		got, err := store.GetAllChanges()
		require.NoError(t, err)

		assert.Equal(t, want, got)
	})

	t.Run("get all change ids", func(t *testing.T) {
		got, err := store.GetAllChangeIds()
		require.NoError(t, err)

		want := []string{"id01",
			"id10",
			"id20",
			"otherId",
			"someRootId",
		}

		assert.Equal(t, want, got)
	})
}

func TestTreeStorage_Delete(t *testing.T) {
	fx := newFixture(t)
	fx.open(t)
	payload := treeTestPayload()
	spaceId := "spaceId"
	_, err := createTreeStorage(fx.db, spaceId, payload)
	require.NoError(t, err)
	fx.stop(t)

	fx.open(t)
	defer fx.stop(t)
	store, err := newTreeStorage(fx.db, spaceId, payload.RootRawChange.Id)
	require.NoError(t, err)
	testTreePayload(t, store, payload)

	t.Run("add raw change, get change and has change", func(t *testing.T) {
		newChange := &treechangeproto.RawTreeChangeWithId{RawChange: []byte("ab"), Id: "newId"}
		require.NoError(t, store.AddRawChange(newChange))

		err = store.Delete()
		require.NoError(t, err)

		_, err = newTreeStorage(fx.db, spaceId, payload.RootRawChange.Id)
		require.Equal(t, err, treestorage.ErrUnknownTreeId)

		fx.testNoKeysExist(t, spaceId, payload.RootRawChange.Id)
	})
}
