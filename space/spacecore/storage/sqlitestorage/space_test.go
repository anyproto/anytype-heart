package sqlitestorage

import (
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/consensus/consensusproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpaceStorage_Create(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)

	payload := spaceTestPayload()
	store, err := createSpaceStorage(fx.storageService, payload)
	require.NoError(t, err)

	testSpace(t, store, payload)
	require.NoError(t, store.Close(ctx))

	t.Run("create same storage returns error", func(t *testing.T) {
		_, err := createSpaceStorage(fx.storageService, payload)
		require.ErrorIs(t, err, spacestorage.ErrSpaceStorageExists)
	})
}

func TestSpaceStorage_Open(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)

	payload := spaceTestPayload()
	_, err := fx.WaitSpaceStorage(ctx, payload.SpaceHeaderWithId.Id)
	require.ErrorIs(t, err, spacestorage.ErrSpaceStorageMissing)

	store, err := fx.CreateSpaceStorage(payload)
	require.NoError(t, err)
	require.NoError(t, store.Close(ctx))

	store, err = fx.WaitSpaceStorage(ctx, payload.SpaceHeaderWithId.Id)
	require.NoError(t, err)

	testSpace(t, store, payload)
	require.NoError(t, store.Close(ctx))
}

func TestSpaceStorage_NewAndCreateTree(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)

	payload := spaceTestPayload()
	store, err := createSpaceStorage(fx.storageService, payload)
	require.NoError(t, err)
	require.NoError(t, store.Close(ctx))

	store, err = newSpaceStorage(fx.storageService, payload.SpaceHeaderWithId.Id)
	require.NoError(t, err)
	testSpace(t, store, payload)

	t.Run("create tree, get tree and mark deleted", func(t *testing.T) {
		treePayload := treeTestPayload()

		ex, err := store.HasTree(treePayload.RootRawChange.Id)
		require.NoError(t, err)
		assert.False(t, ex)

		treeStore, err := store.CreateTreeStorage(treePayload)
		require.NoError(t, err)
		testTreePayload(t, treeStore, treePayload)

		ex, err = store.HasTree(treePayload.RootRawChange.Id)
		require.NoError(t, err)
		assert.True(t, ex)

		otherStore, err := store.TreeStorage(treePayload.RootRawChange.Id)
		require.NoError(t, err)
		testTreePayload(t, otherStore, treePayload)

		initialStatus := "deleted"
		err = store.SetTreeDeletedStatus(otherStore.Id(), initialStatus)
		require.NoError(t, err)

		status, err := store.TreeDeletedStatus(otherStore.Id())
		require.NoError(t, err)
		require.Equal(t, initialStatus, status)

		treeIds, err := store.StoredIds()
		require.NoError(t, err)
		assert.Equal(t, []string{payload.SpaceSettingsWithId.Id, otherStore.Id()}, treeIds)
	})
}

func TestSpaceStorage_SetTreeDeletedStatus(t *testing.T) {
	t.Run("set status with absent tree row", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		payload := spaceTestPayload()
		store, err := createSpaceStorage(fx.storageService, payload)
		require.NoError(t, err)

		err = store.SetTreeDeletedStatus("treeId", spacestorage.TreeDeletedStatusDeleted)
		require.NoError(t, err)

		status, err := store.TreeDeletedStatus("treeId")
		require.NoError(t, err)
		require.Equal(t, spacestorage.TreeDeletedStatusDeleted, status)

		_, err = store.TreeStorage("treeId")
		require.ErrorIs(t, err, treestorage.ErrUnknownTreeId)

		ok, err := store.HasTree("treeId")
		require.NoError(t, err)
		assert.False(t, ok)
	})
}

func TestSpaceStorage_IsSpaceDeleted(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)

	payload := spaceTestPayload()
	ss, err := fx.CreateSpaceStorage(payload)
	require.NoError(t, err)

	isDeleted, err := ss.IsSpaceDeleted()
	require.NoError(t, err)
	assert.False(t, isDeleted)

	require.NoError(t, ss.SetSpaceDeleted())

	isDeleted, err = ss.IsSpaceDeleted()
	require.NoError(t, err)
	assert.True(t, isDeleted)

	require.NoError(t, ss.Close(ctx))

	ss, err = fx.WaitSpaceStorage(ctx, payload.SpaceHeaderWithId.Id)
	require.NoError(t, err)
	defer func() { _ = ss.Close(ctx) }()
	isDeleted, err = ss.IsSpaceDeleted()
	require.NoError(t, err)
	assert.True(t, isDeleted)
}

func TestSpaceStorage_SpaceSettingsId(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)

	payload := spaceTestPayload()
	ss, err := fx.CreateSpaceStorage(payload)
	require.NoError(t, err)

	assert.Equal(t, payload.SpaceSettingsWithId.Id, ss.SpaceSettingsId())
	require.NoError(t, ss.Close(ctx))

	ss, err = fx.WaitSpaceStorage(ctx, payload.SpaceHeaderWithId.Id)
	require.NoError(t, err)
	defer func() { _ = ss.Close(ctx) }()
	assert.Equal(t, payload.SpaceSettingsWithId.Id, ss.SpaceSettingsId())
}

func TestSpaceStorage_ReadSpaceHash(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)

	payload := spaceTestPayload()
	ss, err := fx.CreateSpaceStorage(payload)
	require.NoError(t, err)

	hash, err := ss.ReadSpaceHash()
	require.NoError(t, err)
	assert.Empty(t, hash)
	oldHash, err := ss.ReadOldSpaceHash()
	require.NoError(t, err)
	assert.Empty(t, oldHash)

	require.NoError(t, ss.WriteSpaceHash("hash"))
	require.NoError(t, ss.WriteOldSpaceHash("oldHash"))

	var checkHashes = func(ss spacestorage.SpaceStorage) {
		hash, err = ss.ReadSpaceHash()
		require.NoError(t, err)
		assert.Equal(t, "hash", hash)
		oldHash, err = ss.ReadOldSpaceHash()
		require.NoError(t, err)
		assert.Equal(t, "oldHash", oldHash)
	}

	checkHashes(ss)

	require.NoError(t, ss.Close(ctx))

	ss, err = fx.WaitSpaceStorage(ctx, payload.SpaceHeaderWithId.Id)
	require.NoError(t, err)
	defer func() { _ = ss.Close(ctx) }()
	checkHashes(ss)
}

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

func testSpace(t *testing.T, store spacestorage.SpaceStorage, payload spacestorage.SpaceStorageCreatePayload) {
	header, err := store.SpaceHeader()
	require.NoError(t, err)
	require.Equal(t, payload.SpaceHeaderWithId, header)

	aclStorage, err := store.AclStorage()
	require.NoError(t, err)
	testList(t, aclStorage, payload.AclWithId, payload.AclWithId.Id)
}
