package syncsubscriptions

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

func TestCount(t *testing.T) {
	spaceId := "space1"
	subService := subscription.NewInternalTestService(t)
	subService.AddObjects(t, spaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:   domain.String("1"),
			bundle.RelationKeyName: domain.String("1"),
		},
		{
			bundle.RelationKeyId:   domain.String("2"),
			bundle.RelationKeyName: domain.String("2"),
		},
		{
			bundle.RelationKeyId:   domain.String("4"),
			bundle.RelationKeyName: domain.String("4"),
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
