package syncsubscriptions

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func genObject(syncStatus domain.ObjectSyncStatus, spaceId string) objectstore.TestObject {
	id := fmt.Sprintf("%d", rand.Int())
	return objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeySyncStatus:     domain.Int64(int64(syncStatus)),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		bundle.RelationKeyName:           domain.String("name" + id),
		bundle.RelationKeySpaceId:        domain.String(spaceId),
	}
}

func TestSyncSubscriptions(t *testing.T) {
	fx := newFixture(t)

	var objects []objectstore.TestObject
	objs := map[string]struct{}{}
	for i := 0; i < 10; i++ {
		obj := genObject(domain.ObjectSyncStatusSyncing, "spaceId")
		objects = append(objects, obj)
		objs[obj[bundle.RelationKeyId].String()] = struct{}{}
	}
	for i := 0; i < 10; i++ {
		objects = append(objects, genObject(domain.ObjectSyncStatusSynced, "spaceId"))
	}
	fx.subService.AddObjects(t, "spaceId", objects)

	err := fx.Run(context.Background())
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)
	spaceSub, err := fx.GetSubscription("spaceId")
	require.NoError(t, err)
	syncCnt := spaceSub.SyncingObjectsCount([]string{"1", "2"})
	require.Equal(t, 12, syncCnt)
	require.Len(t, objs, 10)
	spaceSub.GetObjectSubscription().Iterate(func(id string, data struct{}) bool {
		delete(objs, id)
		return true
	})
	require.Empty(t, objs)
	for i := 0; i < 10; i++ {
		objects[i][bundle.RelationKeySyncStatus] = domain.Int64(int64(domain.ObjectSyncStatusSynced))
		fx.subService.AddObjects(t, "spaceId", []objectstore.TestObject{objects[i]})
	}
	time.Sleep(500 * time.Millisecond)
	syncCnt = spaceSub.SyncingObjectsCount([]string{"1", "2"})
	require.Equal(t, 2, syncCnt)
	err = fx.Close(context.Background())
	require.NoError(t, err)
}
