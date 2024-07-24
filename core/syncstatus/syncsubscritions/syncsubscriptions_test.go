package syncsubscritions

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func mapFileStatus(status filesyncstatus.Status) domain.ObjectSyncStatus {
	switch status {
	case filesyncstatus.Syncing:
		return domain.ObjectSyncStatusSyncing
	case filesyncstatus.Queued:
		return domain.ObjectSyncStatusSyncing
	case filesyncstatus.Limited:
		return domain.ObjectSyncStatusError
	default:
		return domain.ObjectSyncStatusSynced
	}
}

func genFileObject(fileStatus filesyncstatus.Status, spaceId string) objectstore.TestObject {
	id := fmt.Sprintf("%d", rand.Int())
	return objectstore.TestObject{
		bundle.RelationKeyId:               pbtypes.String(id),
		bundle.RelationKeySyncStatus:       pbtypes.Int64(int64(mapFileStatus(fileStatus))),
		bundle.RelationKeyFileBackupStatus: pbtypes.Int64(int64(fileStatus)),
		bundle.RelationKeyLayout:           pbtypes.Int64(int64(model.ObjectType_file)),
		bundle.RelationKeyName:             pbtypes.String("name" + id),
		bundle.RelationKeySpaceId:          pbtypes.String(spaceId),
	}
}

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
	fileObjs := map[string]struct{}{}
	objs := map[string]struct{}{}
	for i := 0; i < 10; i++ {
		obj := genObject(domain.ObjectSyncStatusSyncing, "spaceId")
		objects = append(objects, obj)
		objs[obj[bundle.RelationKeyId].GetStringValue()] = struct{}{}
	}
	for i := 0; i < 10; i++ {
		objects = append(objects, genObject(domain.ObjectSyncStatusSynced, "spaceId"))
	}
	for i := 0; i < 10; i++ {
		obj := genFileObject(filesyncstatus.Syncing, "spaceId")
		objects = append(objects, obj)
		fileObjs[obj[bundle.RelationKeyId].GetStringValue()] = struct{}{}
	}
	for i := 0; i < 10; i++ {
		obj := genFileObject(filesyncstatus.Queued, "spaceId")
		objects = append(objects, obj)
		fileObjs[obj[bundle.RelationKeyId].GetStringValue()] = struct{}{}
	}
	for i := 0; i < 10; i++ {
		objects = append(objects, genFileObject(filesyncstatus.Synced, "spaceId"))
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
	fileCnt := spaceSub.FileSyncingObjectsCount()
	require.Equal(t, 12, syncCnt)
	require.Equal(t, 20, fileCnt)
	require.Len(t, fileObjs, 20)
	require.Len(t, objs, 10)
	spaceSub.GetFileSubscription().Iterate(func(id string, data struct{}) bool {
		delete(fileObjs, id)
		return true
	})
	spaceSub.GetObjectSubscription().Iterate(func(id string, data struct{}) bool {
		delete(objs, id)
		return true
	})
	require.Empty(t, fileObjs)
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
