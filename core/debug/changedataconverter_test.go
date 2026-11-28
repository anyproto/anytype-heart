package debug

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
)

// A debug export of a store-backed tree (a chat) must not leak the name of
// the local-API app that authored a change when the export is anonymized.
func TestChangeDataConverter_StoreChangeIntegrationName(t *testing.T) {
	stamped := func() *pb.StoreChange {
		return &pb.StoreChange{ChangeSet: []*pb.StoreChangeContent{{}}, IntegrationName: "Raycast"}
	}
	exported := func(t *testing.T, converter *changeDataConverter, change *pb.StoreChange) *pb.StoreChange {
		data, dataType, err := converter.Marshall(change)
		require.NoError(t, err)
		res, err := converter.Unmarshall(dataType, data)
		require.NoError(t, err)
		return res.(*pb.StoreChange)
	}

	t.Run("an anonymized export does not carry the app name", func(t *testing.T) {
		// given
		change := stamped()

		// when
		got := exported(t, &changeDataConverter{anonymize: true, storeBacked: true}, change)

		// then: red if Marshall returns a store change before consulting the
		// anonymize flag
		assert.Empty(t, got.IntegrationName)
		assert.Equal(t, "Raycast", change.IntegrationName, "the tree's own model must not be modified")
	})

	t.Run("a plain export keeps it", func(t *testing.T) {
		// when
		got := exported(t, &changeDataConverter{storeBacked: true}, stamped())

		// then: red if the name is dropped unconditionally — a debug export
		// would then hide what the tree really holds
		assert.Equal(t, "Raycast", got.IntegrationName)
	})
}
