package syncsubscriptions

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
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

func TestSyncSubscriptionsConcurrentFirstUse(t *testing.T) {
	// given: no subscription for the space yet
	fx := newFixture(t)
	require.NoError(t, fx.Run(context.Background()))
	const callers = 16

	// when: several first calls race
	subs := make([]SyncSubscription, callers)
	var wg sync.WaitGroup
	for i := range subs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sub, err := fx.GetSubscription("spaceId")
			assert.NoError(t, err)
			subs[i] = sub
		}()
	}
	wg.Wait()

	// then: they share one subscription, and it is live: it sees an object
	// added after it was created
	for _, sub := range subs {
		require.Same(t, subs[0], sub)
	}
	fx.subService.AddObjects(t, "spaceId", []objectstore.TestObject{genObject(domain.ObjectSyncStatusSyncing, "spaceId")})
	require.Eventually(t, func() bool {
		return subs[0].SyncingObjectsCount(nil) == 1
	}, 5*time.Second, 10*time.Millisecond)
}

// blockingSubscription is a SyncSubscription whose Run waits for release.
type blockingSubscription struct {
	release <-chan struct{}
	runs    *atomic.Int32
}

func (b *blockingSubscription) Run() error {
	b.runs.Add(1)
	<-b.release
	return nil
}
func (b *blockingSubscription) Close() {}
func (b *blockingSubscription) GetObjectSubscription() *objectsubscription.ObjectSubscription[struct{}] {
	return nil
}
func (b *blockingSubscription) LimitedFilesCount() int           { return 0 }
func (b *blockingSubscription) SyncingObjectsCount([]string) int { return 0 }
func (b *blockingSubscription) UploadingFilesCount() int         { return 0 }

func TestSyncSubscriptionsBuildOneSpaceAtATime(t *testing.T) {
	// given: building space A's subscription hangs until released; B's does not
	fx := newFixture(t)
	// one real build first: the fixture expects the account id to be read
	_, err := fx.GetSubscription("spaceWarmup")
	require.NoError(t, err)
	releaseA := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseA) }) }
	t.Cleanup(release) // a failing test must not leave A's builders hung
	open := make(chan struct{})
	close(open)
	var runsA, runsB atomic.Int32
	fx.SyncSubscriptions.(*syncSubscriptions).newSubscription = func(spaceId string) SyncSubscription {
		if spaceId == "spaceA" {
			return &blockingSubscription{release: releaseA, runs: &runsA}
		}
		return &blockingSubscription{release: open, runs: &runsB}
	}
	gotA := make(chan SyncSubscription, 2)
	for range 2 {
		go func() {
			sub, err := fx.GetSubscription("spaceA")
			assert.NoError(t, err)
			gotA <- sub
		}()
	}
	require.Eventually(t, func() bool { return runsA.Load() == 1 }, 5*time.Second, time.Millisecond)

	// when: B is asked for while A is still building
	doneB := make(chan struct{})
	go func() {
		defer close(doneB)
		_, err := fx.GetSubscription("spaceB")
		assert.NoError(t, err)
	}()

	// then: B is not held up by A, and is built once
	select {
	case <-doneB:
	case <-time.After(10 * time.Second):
		t.Fatal("building space A held up space B")
	}
	assert.Equal(t, int32(1), runsB.Load())
	// and the two callers of A share one build
	release()
	first, second := <-gotA, <-gotA
	assert.Same(t, first, second)
	assert.Equal(t, int32(1), runsA.Load())
}
