package sqlitestorage

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"slices"
	"sort"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage/oldstorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type oldTreeStorage interface {
	oldstorage.ChangesIterator
	oldstorage.TreeStorage
}

func TestTreeStorage_Create(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)

	spacePayload := spaceTestPayload()
	ss, err := createSpaceStorage(fx.storageService, spacePayload)
	require.NoError(t, err)
	payload := treeTestPayload()
	store, err := ss.CreateTreeStorage(payload)
	require.NoError(t, err)
	testTreePayload(t, store, payload)

	t.Run("create same storage returns error", func(t *testing.T) {
		_, err := ss.CreateTreeStorage(payload)
		require.Error(t, err)
	})
}

func TestTreeStorage_Methods(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)

	spacePayload := spaceTestPayload()
	ss, err := createSpaceStorage(fx.storageService, spacePayload)
	require.NoError(t, err)
	payload := treeTestPayload()
	var store oldTreeStorage
	treeStore, err := ss.CreateTreeStorage(payload)
	require.NoError(t, err)

	treeStore, err = ss.TreeStorage(payload.RootRawChange.Id)
	require.NoError(t, err)
	store = treeStore.(oldTreeStorage)
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
		rawCh, err := store.GetRawChange(ctx, newChange.Id)
		require.NoError(t, err)
		require.Equal(t, newChange, rawCh)
		has, err := store.HasChange(ctx, newChange.Id)
		require.NoError(t, err)
		require.True(t, has)
	})

	t.Run("get and has for unknown change", func(t *testing.T) {
		incorrectId := "incorrectId"
		_, err := store.GetRawChange(ctx, incorrectId)
		require.Error(t, err)
		has, err := store.HasChange(ctx, incorrectId)
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

		want := slices.Clone(payload.Changes)
		want = append(want, []*treechangeproto.RawTreeChangeWithId{
			{Id: "id01", RawChange: []byte("foo")},
			{Id: "id10", RawChange: []byte("ab")},
			{Id: "id20", RawChange: []byte("bar")},
		}...)

		sort.Slice(want, func(i, j int) bool {
			return want[i].Id < want[j].Id
		})
		assert.Equal(t, want, collected)

		got, err := store.GetAllChanges()
		require.NoError(t, err)

		assert.Equal(t, want, got)
	})

	t.Run("get all change ids", func(t *testing.T) {
		got, err := store.GetAllChangeIds()
		require.NoError(t, err)

		want := []string{
			payload.Changes[0].Id,
			payload.Changes[1].Id,
			"id01",
			"id10",
			"id20",
		}
		sort.Strings(want)

		assert.Equal(t, want, got)
	})
}

func TestTreeStorage_AddRawChangesSetHeads(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)

	spacePayload := spaceTestPayload()
	ss, err := createSpaceStorage(fx.storageService, spacePayload)
	require.NoError(t, err)
	payload := treeTestPayload()
	store, err := ss.CreateTreeStorage(payload)
	require.NoError(t, err)

	newChanges := []*treechangeproto.RawTreeChangeWithId{{RawChange: []byte("ab"), Id: "newId"}}

	hasChange, err := store.HasChange(ctx, newChanges[0].Id)
	require.NoError(t, err)
	require.False(t, hasChange)

	newHeads := []string{"a", "b"}
	require.NoError(t, store.AddRawChangesSetHeads(newChanges, newHeads))
	heads, err := store.Heads()
	require.NoError(t, err)
	require.Equal(t, newHeads, heads)

	hasChange, err = store.HasChange(ctx, newChanges[0].Id)
	require.NoError(t, err)
	require.True(t, hasChange)
}

func TestTreeStorage_Delete(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)

	spacePayload := spaceTestPayload()
	ss, err := createSpaceStorage(fx.storageService, spacePayload)
	require.NoError(t, err)
	payload := treeTestPayload()
	store, err := ss.CreateTreeStorage(payload)
	require.NoError(t, err)

	t.Run("add raw change, get change and has change", func(t *testing.T) {
		newChange := &treechangeproto.RawTreeChangeWithId{RawChange: []byte("ab"), Id: "newId"}
		require.NoError(t, store.AddRawChange(newChange))

		err = store.Delete()
		require.NoError(t, err)

		_, err = ss.TreeStorage(payload.RootRawChange.Id)
		require.ErrorIs(t, err, treestorage.ErrUnknownTreeId)

		t.Run("heads should be nulled", func(t *testing.T) {
			var heads sql.NullString
			err := fx.readDb.QueryRow("SELECT heads FROM trees WHERE id = ?", payload.RootRawChange.Id).Scan(&heads)
			require.NoError(t, err)
			assert.False(t, heads.Valid)
			assert.Empty(t, heads.String)
		})

		t.Run("tree is deleted: has tree should return false", func(t *testing.T) {
			ok, err := ss.HasTree(payload.RootRawChange.Id)
			require.NoError(t, err)
			assert.False(t, ok)
		})

		t.Run("tree deleted status updates independently, so should not be changed", func(t *testing.T) {
			status, err := ss.TreeDeletedStatus(payload.RootRawChange.Id)
			require.NoError(t, err)
			assert.Equal(t, "", status)
		})

		t.Run("stored ids still contains deleted tree id", func(t *testing.T) {
			ids, err := ss.StoredIds()
			require.NoError(t, err)
			assert.Contains(t, ids, payload.RootRawChange.Id)
		})
	})
}

func BenchmarkSpaceStorage_CreateTreeStorage(b *testing.B) {
	fx := newFixture(b)
	defer fx.finish(b)

	spacePayload := spaceTestPayload()
	ss, err := createSpaceStorage(fx.storageService, spacePayload)
	require.NoError(b, err)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		payload := treeTestPayload()
		_, err = ss.CreateTreeStorage(payload)
		require.NoError(b, err)
	}
}

func randBytes(n int) []byte {
	var b = make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

func randId() string {
	return hex.EncodeToString(randBytes(32))
}

func treeTestPayload() treestorage.TreeStorageCreatePayload {
	rootRawChange := &treechangeproto.RawTreeChangeWithId{RawChange: randBytes(100), Id: randId()}
	otherChange := &treechangeproto.RawTreeChangeWithId{RawChange: randBytes(100), Id: randId()}
	changes := []*treechangeproto.RawTreeChangeWithId{rootRawChange, otherChange}
	return treestorage.TreeStorageCreatePayload{
		RootRawChange: rootRawChange,
		Changes:       changes,
		Heads:         []string{otherChange.Id},
	}
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
		dbCh, err := store.GetRawChange(ctx, ch.Id)
		require.NoError(t, err)
		require.Equal(t, ch, dbCh)
	}
	return
}
