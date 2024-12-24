package sqlitestorage

import (
	"testing"

	"github.com/anyproto/any-sync/commonspace/spacestorage/oldstorage"
	"github.com/anyproto/any-sync/consensus/consensusproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListStorage_AddRawRecord(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)

	payload := spaceTestPayload()
	ss, err := fx.CreateSpaceStorage(payload)
	require.NoError(t, err)
	defer func() { _ = ss.Close(ctx) }()

	ls, err := ss.AclStorage()
	require.NoError(t, err)

	var rec = &consensusproto.RawRecordWithId{
		Id:      "id",
		Payload: []byte("data"),
	}

	require.NoError(t, ls.AddRawRecord(ctx, rec))

	res, err := ls.GetRawRecord(ctx, rec.Id)
	require.NoError(t, err)
	assert.Equal(t, rec, res)
}

func TestListStorage_SetHead(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)

	payload := spaceTestPayload()
	ss, err := fx.CreateSpaceStorage(payload)
	require.NoError(t, err)
	defer func() { _ = ss.Close(ctx) }()

	ls, err := ss.AclStorage()
	require.NoError(t, err)

	head, err := ls.Head()
	require.NoError(t, err)
	assert.Equal(t, "aclRootId", head)

	require.NoError(t, ls.SetHead("newHead"))
	head, err = ls.Head()
	require.NoError(t, err)
	assert.Equal(t, "newHead", head)

}

func testList(t *testing.T, store oldstorage.ListStorage, root *consensusproto.RawRecordWithId, head string) {
	require.Equal(t, store.Id(), root.Id)

	aclRoot, err := store.Root()
	require.NoError(t, err)
	require.Equal(t, root, aclRoot)

	aclHead, err := store.Head()
	require.NoError(t, err)
	require.Equal(t, head, aclHead)
}
