package objectcache

import (
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/spacedomain"
)

func Test_Payloads(t *testing.T) {
	// doing some any-sync preparations
	changePayload := []byte("some")
	keys, err := accountdata.NewRandom()
	require.NoError(t, err)
	aclList, err := list.NewInMemoryDerivedAcl("spaceId", keys)
	require.NoError(t, err)
	timestamp := time.Now().Add(time.Hour).Unix()

	checkRoot := func(root *treechangeproto.RawTreeChangeWithId, changePayload []byte, changeType string, timestamp int64) {
		builder := objecttree.NewChangeBuilder(crypto.NewKeyStorage(), root)
		ch, err := builder.Unmarshall(root, true)
		require.NoError(t, err)
		rootModel := &treechangeproto.TreeChangeInfo{}
		err = rootModel.UnmarshalVT(ch.Data)
		require.NoError(t, err)

		require.Equal(t, rootModel.ChangePayload, changePayload)
		require.Equal(t, rootModel.ChangeType, spacedomain.ChangeType)
		require.Equal(t, ch.Timestamp, timestamp)
	}

	t.Run("test create payload", func(t *testing.T) {
		firstPayload, err := createPayload("spaceId", keys.SignKey, changePayload, timestamp)
		require.NoError(t, err)
		firstRoot, err := objecttree.CreateObjectTreeRoot(firstPayload, aclList)
		require.NoError(t, err)

		secondPayload, err := createPayload("spaceId", keys.SignKey, changePayload, timestamp)
		require.NoError(t, err)
		secondRoot, err := objecttree.CreateObjectTreeRoot(secondPayload, aclList)
		require.NoError(t, err)

		// checking that created roots are not equal
		require.NotEqual(t, firstRoot, secondRoot)

		checkRoot(firstRoot, changePayload, spacedomain.ChangeType, timestamp)
		checkRoot(secondRoot, changePayload, spacedomain.ChangeType, timestamp)
	})

	t.Run("test derive payload", func(t *testing.T) {
		firstPayload := derivePayload("spaceId", changePayload)
		firstRoot, err := objecttree.DeriveObjectTreeRoot(firstPayload, aclList)
		require.NoError(t, err)

		secondPayload := derivePayload("spaceId", changePayload)
		secondRoot, err := objecttree.DeriveObjectTreeRoot(secondPayload, aclList)
		require.NoError(t, err)

		// checking that derived roots are equal
		require.Equal(t, firstRoot, secondRoot)
		checkRoot(firstRoot, changePayload, spacedomain.ChangeType, 0)
	})
}
