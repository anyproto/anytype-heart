package syncsubscriptions

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestCount(t *testing.T) {
	spaceId := "space1"
	subService := subscription.NewInternalTestService(t)
	subService.AddObjects(t, spaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:   pbtypes.String("1"),
			bundle.RelationKeyName: pbtypes.String("1"),
		},
		{
			bundle.RelationKeyId:   pbtypes.String("2"),
			bundle.RelationKeyName: pbtypes.String("2"),
		},
		{
			bundle.RelationKeyId:   pbtypes.String("4"),
			bundle.RelationKeyName: pbtypes.String("4"),
		},
	})

	objSubscription := objectsubscription.NewIdSubscription(subService, subscription.SubscribeRequest{
		SpaceId: spaceId,
		Keys:    []string{bundle.RelationKeyId.String()},
	})
	err := objSubscription.Run()
	require.NoError(t, err)
	defer objSubscription.Close()

	syncing := &syncingObjects{
		objectSubscription: objSubscription,
	}
	cnt := syncing.SyncingObjectsCount([]string{"1", "2", "3"})
	require.Equal(t, 4, cnt)
}
