package subscription

import (
	"context"
	"fmt"
	"runtime"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/kanban"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	benchParticipants = 10
	benchTags         = 5
	benchSubId        = "bench-sub"
)

type benchFixture struct {
	*service
	store *objectstore.StoreFixture
}

func newBenchFixture(b *testing.B) *benchFixture {
	b.Helper()
	a := &app.App{}
	store := objectstore.NewStoreFixture(b)
	a.Register(store)
	a.Register(kanban.New())
	a.Register(&collectionServiceMock{MockCollectionService: NewMockCollectionService(b)})
	sender := mock_event.NewMockSender(b)
	sender.EXPECT().Init(mock.Anything).Return(nil)
	sender.EXPECT().Name().Return(event.CName)
	sender.EXPECT().Broadcast(mock.Anything).Maybe()
	a.Register(sender)
	s := New().(*service)
	a.Register(s)
	require.NoError(b, a.Start(context.Background()))
	b.Cleanup(func() {
		_ = a.Close(context.Background())
	})
	return &benchFixture{service: s, store: store}
}

func benchObjectId(i int) string {
	return fmt.Sprintf("obj-%06d", i)
}

// genBenchObjects generates objects with a realistic number of details (~25 keys),
// referencing a small set of participant, tag and type objects via object-format relations.
func genBenchObjects(n int) []spaceindex.TestObject {
	objs := make([]spaceindex.TestObject, 0, n+benchParticipants+benchTags+1)
	for i := 0; i < n; i++ {
		creator := fmt.Sprintf("participant-%d", i%benchParticipants)
		objs = append(objs, spaceindex.TestObject{
			bundle.RelationKeyId:               domain.String(benchObjectId(i)),
			bundle.RelationKeySpaceId:          domain.String(testSpaceId),
			bundle.RelationKeyName:             domain.String(fmt.Sprintf("Object %06d", i)),
			bundle.RelationKeyDescription:      domain.String("description of the object, long enough to look like a real one"),
			bundle.RelationKeySnippet:          domain.String("snippet of the object body, long enough to look like a real one"),
			bundle.RelationKeyIconEmoji:        domain.String("📄"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyLayout:           domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyType:             domain.String("type-page"),
			bundle.RelationKeyCreator:          domain.StringList([]string{creator}),
			bundle.RelationKeyLastModifiedBy:   domain.StringList([]string{creator}),
			bundle.RelationKeyCreatedDate:      domain.Int64(int64(1700000000 + i)),
			bundle.RelationKeyLastModifiedDate: domain.Int64(int64(1700000000 + i*2)),
			bundle.RelationKeyLastOpenedDate:   domain.Int64(int64(1700000000 + i*3)),
			bundle.RelationKeyAddedDate:        domain.Int64(int64(1700000000 + i)),
			bundle.RelationKeyIsFavorite:       domain.Bool(i%10 == 0),
			bundle.RelationKeyIsArchived:       domain.Bool(false),
			bundle.RelationKeyIsDeleted:        domain.Bool(false),
			bundle.RelationKeyIsHidden:         domain.Bool(false),
			bundle.RelationKeyDone:             domain.Bool(i%2 == 0),
			bundle.RelationKeyDueDate:          domain.Int64(int64(1800000000 + i)),
			bundle.RelationKeyTag:              domain.StringList([]string{fmt.Sprintf("tag-%d", i%benchTags), fmt.Sprintf("tag-%d", (i+1)%benchTags)}),
			bundle.RelationKeyBacklinks:        domain.StringList([]string{benchObjectId((i + 1) % n)}),
			bundle.RelationKeyLinks:            domain.StringList([]string{benchObjectId((i + 2) % n)}),
			bundle.RelationKeySyncStatus:       domain.Int64(0),
			bundle.RelationKeySyncDate:         domain.Int64(int64(1700000000 + i)),
		})
	}
	for i := 0; i < benchParticipants; i++ {
		objs = append(objs, spaceindex.TestObject{
			bundle.RelationKeyId:             domain.String(fmt.Sprintf("participant-%d", i)),
			bundle.RelationKeyName:           domain.String(fmt.Sprintf("Participant %d", i)),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		})
	}
	for i := 0; i < benchTags; i++ {
		objs = append(objs, spaceindex.TestObject{
			bundle.RelationKeyId:             domain.String(fmt.Sprintf("tag-%d", i)),
			bundle.RelationKeyName:           domain.String(fmt.Sprintf("Tag %d", i)),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeyTag.String()),
		})
	}
	objs = append(objs, spaceindex.TestObject{
		bundle.RelationKeyId:             domain.String("type-page"),
		bundle.RelationKeyName:           domain.String("Page"),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
	})
	return objs
}

// benchSubscribeRequest mimics a typical client list subscription: a layout filter,
// a text sort, pagination and a dozen requested keys some of which are object relations.
func benchSubscribeRequest() SubscribeRequest {
	return SubscribeRequest{
		SpaceId: testSpaceId,
		SubId:   benchSubId,
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List([]int64{int64(model.ObjectType_basic)}),
			},
		},
		Sorts: []database.SortRequest{
			{
				RelationKey: bundle.RelationKeyName,
				Type:        model.BlockContentDataviewSort_Asc,
				Format:      model.RelationFormat_longtext,
			},
		},
		Limit: 100,
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyDescription.String(),
			bundle.RelationKeySnippet.String(),
			bundle.RelationKeyIconEmoji.String(),
			bundle.RelationKeyResolvedLayout.String(),
			bundle.RelationKeyType.String(),
			bundle.RelationKeyCreator.String(),
			bundle.RelationKeyTag.String(),
			bundle.RelationKeyDone.String(),
			bundle.RelationKeyLastModifiedDate.String(),
		},
	}
}

func BenchmarkSubscriptionSearch(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("objects=%d", n), func(b *testing.B) {
			fx := newBenchFixture(b)
			fx.store.AddObjects(b, testSpaceId, genBenchObjects(n))
			req := benchSubscribeRequest()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Search unsubscribes the previous subscription with the same id,
				// so each iteration measures full init (plus teardown of the previous one)
				resp, err := fx.Search(req)
				if err != nil {
					b.Fatal(err)
				}
				if len(resp.Records) == 0 {
					b.Fatal("no records")
				}
			}
			b.StopTimer()
			reportRetainedHeap(b, fx)
		})
	}
}

// reportRetainedHeap reports how much detail data the last subscription keeps
// in the entry cache: the average number of detail keys per cached entry and
// the process heap after GC
func reportRetainedHeap(b *testing.B, fx *benchFixture) {
	fx.lock.Lock()
	spaceSubs := fx.spaceSubs[testSpaceId]
	fx.lock.Unlock()
	spaceSubs.m.Lock()
	var totalKeys, entries int
	for _, e := range spaceSubs.cache.entries {
		entries++
		totalKeys += e.data.Len()
	}
	spaceSubs.m.Unlock()
	if entries > 0 {
		b.ReportMetric(float64(totalKeys)/float64(entries), "keys/entry")
	}
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	b.ReportMetric(float64(m.HeapAlloc)/(1<<20), "heap-MB")
}

// BenchmarkSubscriptionOnChange measures processing of a batch of changed records
// that belong to the active window of an existing subscription.
func BenchmarkSubscriptionOnChange(b *testing.B) {
	const batchSize = 10
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprintf("objects=%d", n), func(b *testing.B) {
			fx := newBenchFixture(b)
			fx.store.AddObjects(b, testSpaceId, genBenchObjects(n))
			_, err := fx.Search(benchSubscribeRequest())
			require.NoError(b, err)

			fx.lock.Lock()
			spaceSubs := fx.spaceSubs[testSpaceId]
			fx.lock.Unlock()

			ids := make([]string, batchSize)
			for i := range ids {
				ids[i] = benchObjectId(i)
			}
			records, err := spaceSubs.objectStore.QueryByIds(ids)
			require.NoError(b, err)
			require.Len(b, records, batchSize)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				entries := make([]*entry, 0, len(records))
				for _, r := range records {
					det := r.Details.Copy()
					det.SetString(bundle.RelationKeyName, fmt.Sprintf("Object renamed %06d", i))
					det.SetInt64(bundle.RelationKeyLastModifiedDate, int64(1700000000+i))
					entries = append(entries, newEntry(det.GetString(bundle.RelationKeyId), det))
				}
				spaceSubs.onChange(entries)
			}
		})
	}
}

// BenchmarkSubscriptionOnChangeMiss measures the cost of a batch of records
// that do not match any subscription filter: the common case of unrelated
// objects being indexed while subscriptions are active.
func BenchmarkSubscriptionOnChangeMiss(b *testing.B) {
	const batchSize = 10
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprintf("objects=%d", n), func(b *testing.B) {
			fx := newBenchFixture(b)
			fx.store.AddObjects(b, testSpaceId, genBenchObjects(n))
			_, err := fx.Search(benchSubscribeRequest())
			require.NoError(b, err)

			fx.lock.Lock()
			spaceSubs := fx.spaceSubs[testSpaceId]
			fx.lock.Unlock()

			records, err := spaceSubs.objectStore.QueryByIds([]string{benchObjectId(0)})
			require.NoError(b, err)
			require.Len(b, records, 1)
			base := records[0].Details

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				entries := make([]*entry, 0, batchSize)
				for j := 0; j < batchSize; j++ {
					det := base.Copy()
					det.SetString(bundle.RelationKeyId, fmt.Sprintf("miss-%06d-%06d", i, j))
					det.SetInt64(bundle.RelationKeyResolvedLayout, int64(model.ObjectType_relationOption))
					entries = append(entries, newEntry(det.GetString(bundle.RelationKeyId), det))
				}
				spaceSubs.onChange(entries)
			}
		})
	}
}
