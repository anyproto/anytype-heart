package subscription

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/kanban"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

// benchFixture wires the engine with a discarding event sender so delivery
// cost is excluded from engine-path measurements
type benchFixture struct {
	Service
	objectStore *objectstore.StoreFixture
	svc         *service
}

func newBenchFixture(tb testing.TB) *benchFixture {
	ctx := context.Background()
	a := &app.App{}

	eventSender := mock_event.NewMockSender(tb)
	eventSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()

	objectStore := objectstore.NewStoreFixture(tb)
	s := New()

	a.Register(objectStore)
	a.Register(kanban.New())
	a.Register(&collectionServiceMock{MockCollectionService: NewMockCollectionService(tb)})
	a.Register(testutil.PrepareMock(ctx, a, eventSender))
	a.Register(s)
	if err := a.Start(ctx); err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.Close(closeCtx)
	})
	return &benchFixture{
		Service:     s,
		objectStore: objectStore,
		svc:         s.(*service),
	}
}

func benchObject(i int, layout model.ObjectTypeLayout) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(fmt.Sprintf("obj-%06d", i)),
		bundle.RelationKeyName:           domain.String(fmt.Sprintf("name-%06d", i)),
		bundle.RelationKeyDescription:    domain.String("some description text"),
		bundle.RelationKeyCreatedDate:    domain.Int64(int64(1700000000 + i)),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(layout)),
	}
}

func benchDetails(i int, layout model.ObjectTypeLayout) *domain.Details {
	return benchObject(i, layout).Details()
}

func seedBenchObjects(tb testing.TB, fx *benchFixture, n int) {
	objs := make([]objectstore.TestObject, 0, n)
	for i := 0; i < n; i++ {
		objs = append(objs, benchObject(i, model.ObjectType_participant))
	}
	fx.objectStore.AddObjects(tb, testSpaceId, objs)
}

func benchRequest(subId string, internal bool) SubscribeRequest {
	return SubscribeRequest{
		SpaceId:           testSpaceId,
		SubId:             subId,
		Internal:          internal,
		NoDepSubscription: true,
		Keys:              []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String()},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_participant)),
			},
		},
	}
}

func (fx *benchFixture) spaceState(tb testing.TB) *spaceState {
	fx.svc.mu.Lock()
	defer fx.svc.mu.Unlock()
	st := fx.svc.spaces[testSpaceId]
	if st == nil {
		tb.Fatal("space state not created")
	}
	return st
}

// TestMissPathAllocations is the benchmark-enforced gate from the design's
// performance contract: an update matching no subscription must not allocate
// in the engine (one filter eval + one set lookup per sub). The injected
// default filters plus a scalar Equal stand in for the client's default
// filter shape.
func TestMissPathAllocations(t *testing.T) {
	fx := newBenchFixture(t)
	seedBenchObjects(t, fx, 100)
	for i := 0; i < 20; i++ {
		req := benchRequest(fmt.Sprintf("miss-sub-%d", i), true)
		_, err := fx.Search(req)
		require.NoError(t, err)
	}
	st := fx.spaceState(t)

	// a basic-layout object matches no participant subscription
	items := []feedItem{{id: "miss-object", details: benchDetails(0, model.ObjectType_basic)}}
	allocs := testing.AllocsPerRun(200, func() {
		st.processBatch(items)
	})
	assert.Zero(t, allocs, "miss path must not allocate")
}

func BenchmarkSubscribeSnapshot10k(b *testing.B) {
	fx := newBenchFixture(b)
	seedBenchObjects(b, fx, 10000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := benchRequest("bench-snapshot", true)
		resp, err := fx.Search(req)
		if err != nil {
			b.Fatal(err)
		}
		if len(resp.Records) != 10000 {
			b.Fatalf("unexpected snapshot size %d", len(resp.Records))
		}
	}
}

func BenchmarkSubscribeWindowed10k(b *testing.B) {
	fx := newBenchFixture(b)
	seedBenchObjects(b, fx, 10000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := benchRequest("bench-windowed", false)
		req.Limit = 100
		req.Sorts = []database.SortRequest{
			{RelationKey: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Asc},
		}
		resp, err := fx.Search(req)
		if err != nil {
			b.Fatal(err)
		}
		if len(resp.Records) != 100 {
			b.Fatalf("unexpected window size %d", len(resp.Records))
		}
	}
}

func BenchmarkChangeBatch(b *testing.B) {
	fx := newBenchFixture(b)
	seedBenchObjects(b, fx, 1000)
	for i := 0; i < 20; i++ {
		if _, err := fx.Search(benchRequest(fmt.Sprintf("bench-sub-%d", i), true)); err != nil {
			b.Fatal(err)
		}
	}
	st := fx.spaceState(b)

	// 100 member updates per batch: name changes → Amend per sub
	mkItems := func(gen int) []feedItem {
		items := make([]feedItem, 0, 100)
		for i := 0; i < 100; i++ {
			details := benchDetails(i, model.ObjectType_participant)
			details.SetString(bundle.RelationKeyName, fmt.Sprintf("renamed-%d-%d", gen, i))
			items = append(items, feedItem{id: details.GetString(bundle.RelationKeyId), details: details})
		}
		return items
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		items := mkItems(i)
		st.drainOutbox()
		b.StartTimer()
		st.processBatch(items)
	}
}

func BenchmarkLimitZeroSteadyState(b *testing.B) {
	fx := newBenchFixture(b)
	seedBenchObjects(b, fx, 10000)
	req := benchRequest("bench-limit0", false) // ordered, limit 0 = full window
	if _, err := fx.Search(req); err != nil {
		b.Fatal(err)
	}
	st := fx.spaceState(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		details := benchDetails(42, model.ObjectType_participant)
		details.SetString(bundle.RelationKeyDescription, fmt.Sprintf("changed %d", i))
		items := []feedItem{{id: details.GetString(bundle.RelationKeyId), details: details}}
		st.drainOutbox()
		b.StartTimer()
		st.processBatch(items)
	}
}

// benchOrderedSub subscribes name-sorted with the given limit and returns
// the space state for direct batch driving
func benchOrderedSub(b *testing.B, fx *benchFixture, limit int64) *spaceState {
	req := benchRequest("bench-ordered", false)
	req.Limit = limit
	req.Keys = append(req.Keys, bundle.RelationKeyDescription.String())
	req.Sorts = []database.SortRequest{
		{RelationKey: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Asc},
	}
	if _, err := fx.Search(req); err != nil {
		b.Fatal(err)
	}
	return fx.spaceState(b)
}

// BenchmarkOrderedAmend500 measures a requested non-sort key change on a
// window member: diff + Amend emission, no ordering work
func BenchmarkOrderedAmend500(b *testing.B) {
	fx := newBenchFixture(b)
	seedBenchObjects(b, fx, 10000)
	st := benchOrderedSub(b, fx, 500)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		details := benchDetails(250, model.ObjectType_participant) // window member
		details.SetString(bundle.RelationKeyDescription, fmt.Sprintf("changed %d", i))
		items := []feedItem{{id: details.GetString(bundle.RelationKeyId), details: details}}
		st.drainOutbox()
		b.StartTimer()
		st.processBatch(items)
	}
}

// BenchmarkOrderedReorder500 measures an in-window reposition: a sort-key
// change that moves a member within the window incrementally (binary-search
// remove/insert plus the window-diff script), no store re-query
func BenchmarkOrderedReorder500(b *testing.B) {
	fx := newBenchFixture(b)
	seedBenchObjects(b, fx, 10000)
	st := benchOrderedSub(b, fx, 500)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		details := benchDetails(250, model.ObjectType_participant)
		// alternate between two interior window ranks (window = names
		// 000000..000499): position ~100 and ~400, never first or last
		if i%2 == 0 {
			details.SetString(bundle.RelationKeyName, "name-000100a")
		} else {
			details.SetString(bundle.RelationKeyName, "name-000400a")
		}
		items := []feedItem{{id: details.GetString(bundle.RelationKeyId), details: details}}
		st.drainOutbox()
		b.StartTimer()
		st.processBatch(items)
	}
}

// BenchmarkOrderedReorder10kFullList is the same reposition on a limit-0
// subscription: the window is all 10k members, so this prices the slice
// surgery and the diff-script walk at full-list scale
func BenchmarkOrderedReorder10kFullList(b *testing.B) {
	fx := newBenchFixture(b)
	seedBenchObjects(b, fx, 10000)
	st := benchOrderedSub(b, fx, 0)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		details := benchDetails(5000, model.ObjectType_participant)
		if i%2 == 0 {
			details.SetString(bundle.RelationKeyName, "name-002000a")
		} else {
			details.SetString(bundle.RelationKeyName, "name-008000a")
		}
		items := []feedItem{{id: details.GetString(bundle.RelationKeyId), details: details}}
		st.drainOutbox()
		b.StartTimer()
		st.processBatch(items)
	}
}

// BenchmarkOrderedOutOfWindowUpdate measures the bookkeeping tax for an
// update to a member beyond the window: a boundary probe (one sort-key
// projection + compare), no events
func BenchmarkOrderedOutOfWindowUpdate(b *testing.B) {
	fx := newBenchFixture(b)
	seedBenchObjects(b, fx, 10000)
	st := benchOrderedSub(b, fx, 500)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		details := benchDetails(9000, model.ObjectType_participant) // far beyond the window
		details.SetString(bundle.RelationKeyDescription, fmt.Sprintf("changed %d", i))
		items := []feedItem{{id: details.GetString(bundle.RelationKeyId), details: details}}
		b.StartTimer()
		st.processBatch(items)
	}
}

// BenchmarkRealisticSpaceUpdate models the contract's real shape — §3.10:
// every update is evaluated against every subscription of the space, dozens
// of them. 20k mixed objects; 4 limit-0 singletons, 15 typeCheck-style
// limit-1 client subs, 4 sorted 100-row views and one deps-enabled view.
// Measured: one sort-relevant update fanned out across all 24 subs.
func BenchmarkRealisticSpaceUpdate(b *testing.B) {
	fx := newBenchFixture(b)
	layouts := []model.ObjectTypeLayout{
		model.ObjectType_participant, model.ObjectType_basic,
		model.ObjectType_todo, model.ObjectType_note,
	}
	objs := make([]objectstore.TestObject, 0, 20101)
	objs = append(objs, givenAssigneeRelation())
	for i := 0; i < 100; i++ {
		objs = append(objs, givenPerson(fmt.Sprintf("person-%03d", i), fmt.Sprintf("Person %03d", i)))
	}
	for i := 0; i < 20000; i++ {
		obj := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String(fmt.Sprintf("obj-%06d", i)),
			bundle.RelationKeyName:           domain.String(fmt.Sprintf("name-%06d", i)),
			bundle.RelationKeyType:           domain.String(fmt.Sprintf("type-%02d", i%15)),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(layouts[i%4])),
		}
		if i%4 == 2 { // todos carry an assignee for the deps view
			obj[assigneeKey] = domain.StringList([]string{fmt.Sprintf("person-%03d", i%100)})
		}
		objs = append(objs, obj)
	}
	fx.objectStore.AddObjects(b, testSpaceId, objs)

	layoutFilter := func(l model.ObjectTypeLayout) []database.FilterRequest {
		return []database.FilterRequest{{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(l)),
		}}
	}
	subscribe := func(req SubscribeRequest) {
		if _, err := fx.Search(req); err != nil {
			b.Fatal(err)
		}
	}
	for i, l := range layouts { // limit-0 internal singletons
		subscribe(SubscribeRequest{
			SpaceId: testSpaceId, SubId: fmt.Sprintf("singleton-%d", i),
			Internal: true, NoDepSubscription: true,
			Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String()},
			Filters: layoutFilter(l),
		})
	}
	for i := 0; i < 15; i++ { // typeCheck-style limit-1 client subs
		subscribe(SubscribeRequest{
			SpaceId: testSpaceId, SubId: fmt.Sprintf("typecheck-%02d", i),
			NoDepSubscription: true, Limit: 1,
			Keys: []string{bundle.RelationKeyId.String()},
			Filters: []database.FilterRequest{{
				RelationKey: bundle.RelationKeyType,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(fmt.Sprintf("type-%02d", i)),
			}},
		})
	}
	for i := 0; i < 4; i++ { // sorted 100-row views
		subscribe(SubscribeRequest{
			SpaceId: testSpaceId, SubId: fmt.Sprintf("view-%d", i),
			NoDepSubscription: true, Limit: 100,
			Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String()},
			Sorts:   []database.SortRequest{{RelationKey: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Asc}},
			Filters: layoutFilter(model.ObjectType_participant),
		})
	}
	subscribe(SubscribeRequest{ // one deps-enabled view
		SpaceId: testSpaceId, SubId: "deps-view", Limit: 100,
		Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String(), assigneeKey},
		Sorts:   []database.SortRequest{{RelationKey: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Asc}},
		Filters: layoutFilter(model.ObjectType_todo),
	})
	st := fx.spaceState(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// a participant rename: amends in its singleton, boundary probes in
		// the sorted views, misses everywhere else
		details := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("obj-010000"),
			bundle.RelationKeyName:           domain.String(fmt.Sprintf("renamed-%06d", i)),
			bundle.RelationKeyType:           domain.String("type-10"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}.Details()
		items := []feedItem{{id: "obj-010000", details: details}}
		st.drainOutbox()
		b.StartTimer()
		st.processBatch(items)
	}
}

// benchFeedStorm drives a write storm through the REAL pipeline — feed
// callback, coalescing intake (id-only degradation above 2048 pending),
// worker, outbox, queue delivery — and waits for a sentinel member's events
// to prove everything before it was delivered. ns/op is per storm.
func benchFeedStorm(b *testing.B, stormSize int) {
	fx := newBenchFixture(b)
	seedBenchObjects(b, fx, stormSize)
	req := benchRequest("storm-sub", true)
	resp, err := fx.Search(req)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < stormSize; j++ {
			details := benchDetails(j, model.ObjectType_participant)
			details.SetString(bundle.RelationKeyName, fmt.Sprintf("storm-%d-%06d", i, j))
			idx := fx.objectStore.SpaceIndex(testSpaceId)
			if err := idx.UpdateObjectDetails(context.Background(), details.GetString(bundle.RelationKeyId), details); err != nil {
				b.Fatal(err)
			}
		}
		// sentinel: a new member whose Add, by FIFO, proves the storm drained
		sentinel := givenParticipant(fmt.Sprintf("sentinel-%d", i))
		fx.objectStore.AddObjects(b, testSpaceId, []objectstore.TestObject{sentinel})
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		for {
			msg, err := resp.Output.WaitOne(ctx)
			if err != nil {
				cancel()
				b.Fatal(err)
			}
			if add := msg.GetSubscriptionAdd(); add != nil && add.Id == sentinel.Id() {
				break
			}
		}
		cancel()
	}
}

func BenchmarkFeedStorm1k(b *testing.B) { benchFeedStorm(b, 1000) }

// BenchmarkFeedStorm5kDegraded crosses the 2048-pending intake threshold:
// later entries degrade to id-only and the worker re-fetches them in
// batches on drain
func BenchmarkFeedStorm5kDegraded(b *testing.B) { benchFeedStorm(b, 5000) }

// seedBenchAssigneeWorld: m persons and n todos sorted by an object-format
// relation — the comparator that goes through an OrderMap (and may lazily
// query the store for unseen target ids inside Compare)
func seedBenchAssigneeWorld(tb testing.TB, fx *benchFixture, n, m int) {
	objs := make([]objectstore.TestObject, 0, n+m+1)
	objs = append(objs, givenAssigneeRelation())
	for i := 0; i < m; i++ {
		objs = append(objs, givenPerson(fmt.Sprintf("person-%03d", i), fmt.Sprintf("Person %03d", i)))
	}
	for i := 0; i < n; i++ {
		objs = append(objs, givenTask(fmt.Sprintf("task-%05d", i), fmt.Sprintf("task %05d", i), fmt.Sprintf("person-%03d", i%m)))
	}
	fx.objectStore.AddObjects(tb, testSpaceId, objs)
}

func benchAssigneeSortedSub(b *testing.B, fx *benchFixture) *spaceState {
	req := SubscribeRequest{
		SpaceId: testSpaceId, SubId: "assignee-sorted",
		NoDepSubscription: true, Limit: 100,
		Keys:  []string{bundle.RelationKeyId.String(), assigneeKey},
		Sorts: []database.SortRequest{{RelationKey: assigneeKey, Type: model.BlockContentDataviewSort_Asc}},
		Filters: []database.FilterRequest{{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectType_todo)),
		}},
	}
	if _, err := fx.Search(req); err != nil {
		b.Fatal(err)
	}
	return fx.spaceState(b)
}

// BenchmarkObjectSortReorder100 reassigns a window member under an
// object-format sort: the reposition compares through the OrderMap
// (collation of target names, lazy store lookups for unseen ids)
func BenchmarkObjectSortReorder100(b *testing.B) {
	fx := newBenchFixture(b)
	seedBenchAssigneeWorld(b, fx, 5000, 200)
	st := benchAssigneeSortedSub(b, fx)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		task := givenTask("task-00000", "task 00000", fmt.Sprintf("person-%03d", i%200))
		items := []feedItem{{id: "task-00000", details: task.Details()}}
		st.drainOutbox()
		b.StartTimer()
		st.processBatch(items)
	}
}

// BenchmarkOrderDepRenameRequery renames a person tasks are sorted by: the
// order-dep probe reports the order map shifted and the window rebuilds via
// the bounded re-query — the §3.5 "rename the assignee you sort by" cost
func BenchmarkOrderDepRenameRequery(b *testing.B) {
	fx := newBenchFixture(b)
	seedBenchAssigneeWorld(b, fx, 5000, 200)
	st := benchAssigneeSortedSub(b, fx)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		person := givenPerson("person-000", fmt.Sprintf("Renamed %06d", i))
		items := []feedItem{{id: "person-000", details: person.Details()}}
		st.drainOutbox()
		b.StartTimer()
		st.processBatch(items)
	}
}

const benchTagOptions = 20

// seedBenchTagWorld seeds a tag relation, benchTagOptions options and n
// tagged todo objects (one tag each, round-robin)
func seedBenchTagWorld(tb testing.TB, fx *benchFixture, n int) {
	objs := make([]objectstore.TestObject, 0, n+benchTagOptions+1)
	objs = append(objs, givenTagRelation())
	for i := 0; i < benchTagOptions; i++ {
		objs = append(objs, givenTagOption(fmt.Sprintf("t%02d", i)))
	}
	for i := 0; i < n; i++ {
		objs = append(objs, givenTaggedTask(fmt.Sprintf("task-%05d", i), fmt.Sprintf("t%02d", i%benchTagOptions)))
	}
	fx.objectStore.AddObjects(tb, testSpaceId, objs)
}

func benchGroupsRequest() SubscribeGroupsRequest {
	return SubscribeGroupsRequest{
		SpaceId:     testSpaceId,
		SubId:       "bench-groups",
		RelationKey: tagKey,
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_todo)),
			},
		},
	}
}

// BenchmarkSubscribeGroups1k measures the kanban groups subscribe path:
// grouper resolution, member + option relevance seeding, the grouper's own
// store query and the initial group computation. Same subId per iteration —
// replace-on-resubscribe is the client's real re-subscribe pattern.
func BenchmarkSubscribeGroups1k(b *testing.B) {
	fx := newBenchFixture(b)
	seedBenchTagWorld(b, fx, 1000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := fx.SubscribeGroups(benchGroupsRequest())
		if err != nil {
			b.Fatal(err)
		}
		if len(resp.Groups) != benchTagOptions+1 { // + the no-value column
			b.Fatalf("unexpected group count %d", len(resp.Groups))
		}
	}
}

// BenchmarkGroupsRecompute1k measures the full per-change cost of a live
// groups subscription: the relevance check marking it dirty plus the
// off-mutex recomputation (grouper store query + group diff) a member's
// grouped-value change triggers.
func BenchmarkGroupsRecompute1k(b *testing.B) {
	fx := newBenchFixture(b)
	seedBenchTagWorld(b, fx, 1000)
	if _, err := fx.SubscribeGroups(benchGroupsRequest()); err != nil {
		b.Fatal(err)
	}
	st := fx.spaceState(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// alternate the tag so the relevance check sees a real value change
		obj := givenTaggedTask("task-00000", fmt.Sprintf("t%02d", i%benchTagOptions))
		items := []feedItem{{id: "task-00000", details: obj.Details()}}
		b.StartTimer()
		st.processBatch(items)
	}
}

// BenchmarkGroupsMissPath measures the relevance check for feed items a
// groups subscription does not care about — the cost every irrelevant
// update in the space pays per groups sub
func BenchmarkGroupsMissPath(b *testing.B) {
	fx := newBenchFixture(b)
	seedBenchTagWorld(b, fx, 1000)
	if _, err := fx.SubscribeGroups(benchGroupsRequest()); err != nil {
		b.Fatal(err)
	}
	st := fx.spaceState(b)

	// a participant: wrong layout for the member filter, not an option
	items := []feedItem{{id: "miss-object", details: benchDetails(0, model.ObjectType_participant)}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		st.processBatch(items)
	}
}

func BenchmarkWindowShift500(b *testing.B) {
	fx := newBenchFixture(b)
	seedBenchObjects(b, fx, 5000)
	req := benchRequest("bench-window", false)
	req.Limit = 500
	req.Sorts = []database.SortRequest{
		{RelationKey: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Asc},
	}
	if _, err := fx.Search(req); err != nil {
		b.Fatal(err)
	}
	st := fx.spaceState(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// flip one object between the window front and beyond the window end
		details := benchDetails(4999, model.ObjectType_participant)
		if i%2 == 0 {
			details.SetString(bundle.RelationKeyName, "aaaa-front")
		}
		items := []feedItem{{id: details.GetString(bundle.RelationKeyId), details: details}}
		st.drainOutbox()
		b.StartTimer()
		st.processBatch(items)
	}
}
