package badgerstorage

import (
	"context"
	"sort"
	"strconv"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage/oldstorage"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/consensus/consensusproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

func spaceTestPayload() spacestorage.SpaceStorageCreatePayload {
	header := &spacesyncproto.RawSpaceHeaderWithId{
		RawHeader: []byte("header"),
		Id:        "headerId",
	}
	aclRoot := &consensusproto.RawRecordWithId{
		Payload: []byte("aclRoot"),
		Id:      "aclRootId",
	}
	settings := &treechangeproto.RawTreeChangeWithId{
		RawChange: []byte("settings"),
		Id:        "settingsId",
	}
	return spacestorage.SpaceStorageCreatePayload{
		AclWithId:           aclRoot,
		SpaceHeaderWithId:   header,
		SpaceSettingsWithId: settings,
	}
}

func testSpace(t *testing.T, store oldstorage.SpaceStorage, payload spacestorage.SpaceStorageCreatePayload) {
	header, err := store.SpaceHeader()
	require.NoError(t, err)
	require.Equal(t, payload.SpaceHeaderWithId, header)

	aclStorage, err := store.AclStorage()
	require.NoError(t, err)
	testList(t, aclStorage, payload.AclWithId, payload.AclWithId.Id)
}

func TestSpaceStorage_Create(t *testing.T) {
	fx := newFixture(t)
	fx.open(t)
	defer fx.stop(t)

	payload := spaceTestPayload()
	store, err := createSpaceStorage(fx.db, payload, &storageService{})
	require.NoError(t, err)

	testSpace(t, store, payload)
	require.NoError(t, store.Close(ctx))

	t.Run("create same storage returns error", func(t *testing.T) {
		_, err := createSpaceStorage(fx.db, payload, &storageService{})
		require.Error(t, err)
	})
}

func TestSpaceStorage_NewAndCreateTree(t *testing.T) {
	fx := newFixture(t)
	fx.open(t)

	payload := spaceTestPayload()
	store, err := createSpaceStorage(fx.db, payload, &storageService{})
	require.NoError(t, err)
	require.NoError(t, store.Close(ctx))
	fx.stop(t)

	fx.open(t)
	defer fx.stop(t)
	store, err = newSpaceStorage(fx.db, payload.SpaceHeaderWithId.Id, nil)
	require.NoError(t, err)
	testSpace(t, store, payload)

	t.Run("create tree, get tree and mark deleted", func(t *testing.T) {
		payload := treeTestPayload()
		treeStore, err := store.CreateTreeStorage(payload)
		require.NoError(t, err)
		testTreePayload(t, treeStore, payload)

		otherStore, err := store.TreeStorage(payload.RootRawChange.Id)
		require.NoError(t, err)
		testTreePayload(t, otherStore, payload)

		initialStatus := "deleted"
		err = store.SetTreeDeletedStatus(otherStore.Id(), initialStatus)
		require.NoError(t, err)

		status, err := store.TreeDeletedStatus(otherStore.Id())
		require.NoError(t, err)
		require.Equal(t, initialStatus, status)

		ids, err := store.(*spaceStorage).AllDeletedTreeIds()
		require.NoError(t, err)
		assert.Equal(t, []string{otherStore.Id()}, ids)
	})
}

func TestSpaceStorage_StoredIds_BigTxn(t *testing.T) {
	fx := newFixture(t)
	fx.open(t)
	defer fx.stop(t)

	payload := spaceTestPayload()
	store, err := createSpaceStorage(fx.db, payload, &storageService{})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, store.Close(ctx))
	}()

	n := 50000
	var ids []string
	for i := 0; i < n; i++ {
		treePayload := treeTestPayload()
		treePayload.RootRawChange.Id += strconv.Itoa(i)
		ids = append(ids, treePayload.RootRawChange.Id)
		_, err := store.CreateTreeStorage(treePayload)
		require.NoError(t, err)
	}
	ids = append(ids, payload.SpaceSettingsWithId.Id)
	sort.Strings(ids)

	storedIds, err := store.StoredIds()
	require.NoError(t, err)
	sort.Strings(storedIds)
	require.Equal(t, ids, storedIds)

	err = deleteSpace(store.Id(), fx.db)
	require.NoError(t, err)
	storedIds, err = store.StoredIds()
	require.NoError(t, err)
	require.Len(t, storedIds, 0)
}

func newServiceFixture(t *testing.T) *storageService {
	fx := newFixture(t)
	fx.open(t)

	t.Cleanup(func() {
		fx.stop(t)
	})

	s := &storageService{
		db:           fx.db,
		keys:         newStorageServiceKeys(),
		lockedSpaces: map[string]*lockSpace{},
	}
	return s
}

func TestStorageService_BindSpaceID(t *testing.T) {
	fx := newServiceFixture(t)

	err := fx.BindSpaceID("spaceId1", "objectId1")
	require.NoError(t, err)

	spaceId, err := fx.GetSpaceID("objectId1")
	require.NoError(t, err)

	require.Equal(t, spaceId, "spaceId1")
}

func TestStorageService_GetBoundObjectIds(t *testing.T) {
	t.Run("with no bindings", func(t *testing.T) {
		fx := newServiceFixture(t)

		ids, err := fx.GetBoundObjectIds("spaceId")
		require.NoError(t, err)
		assert.Empty(t, ids)
	})

	t.Run("ok", func(t *testing.T) {
		fx := newServiceFixture(t)

		spaceId := "spaceId1"
		err := fx.BindSpaceID(spaceId, "objectId1")
		require.NoError(t, err)

		err = fx.BindSpaceID(spaceId, "objectId2")
		require.NoError(t, err)

		ids, err := fx.GetBoundObjectIds(spaceId)
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"objectId1", "objectId2"}, ids)
	})

}
