package sqlitestorage

import (
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/acl/liststorage"
	"github.com/anyproto/any-sync/consensus/consensusproto"
	"github.com/stretchr/testify/require"
)

func testList(t *testing.T, store liststorage.ListStorage, root *consensusproto.RawRecordWithId, head string) {
	require.Equal(t, store.Id(), root.Id)

	aclRoot, err := store.Root()
	require.NoError(t, err)
	require.Equal(t, root, aclRoot)

	aclHead, err := store.Head()
	require.NoError(t, err)
	require.Equal(t, head, aclHead)
}
