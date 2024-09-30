package syncsubscriptions

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func genObject(syncStatus domain.ObjectSyncStatus, spaceId string) objectstore.TestObject {
	id := fmt.Sprintf("%d", rand.Int())
	return objectstore.TestObject{
		bundle.RelationKeyId:         pbtypes.String(id),
		bundle.RelationKeySyncStatus: pbtypes.Int64(int64(syncStatus)),
		bundle.RelationKeyLayout:     pbtypes.Int64(int64(model.ObjectType_basic)),
		bundle.RelationKeyName:       pbtypes.String("name" + id),
		bundle.RelationKeySpaceId:    pbtypes.String(spaceId),
	}
}

func TestSyncSubscriptions(t *testing.T) {
	testSubs := subscription.NewInternalTestService(t)
	var objects []objectstore.TestObject
	objs := map[string]struct{}{}
	for i := 0; i < 10; i++ {
		obj := genObject(domain.ObjectSyncStatusSyncing, "spaceId")
		objects = append(objects, obj)
		objs[obj[bundle.RelationKeyId].GetStringValue()] = struct{}{}
	}
	for i := 0; i < 10; i++ {
		objects = append(objects, genObject(domain.ObjectSyncStatusSynced, "spaceId"))
	}
	testSubs.AddObjects(t, objects)
	subs := New()
	subs.(*syncSubscriptions).service = testSubs
	err := subs.Run(context.Background())
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)
	spaceSub, err := subs.GetSubscription("spaceId")
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
		objects[i][bundle.RelationKeySyncStatus] = pbtypes.Int64(int64(domain.ObjectSyncStatusSynced))
		testSubs.AddObjects(t, []objectstore.TestObject{objects[i]})
	}
	time.Sleep(100 * time.Millisecond)
	syncCnt = spaceSub.SyncingObjectsCount([]string{"1", "2"})
	require.Equal(t, 2, syncCnt)
	err = subs.Close(context.Background())
	require.NoError(t, err)
}
