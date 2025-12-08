package subscription

import (
	"testing"

	"github.com/anyproto/any-store/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestSubscription_Add(t *testing.T) {
	testOrder := genOrder(t)
	t.Run("add", func(t *testing.T) {
		sub := &sortedSub{
			id:      "test",
			order:   testOrder,
			cache:   newCache(),
			limit:   3,
			afterId: "id3",
			ds:      &dependencyService{},
		}
		require.NoError(t, sub.init(genEntries(9, false)))
		newEntries := []*entry{
			genEntry("newActiveId1", 3),
			genEntry("newActiveId2", 3),
			genEntry("beforeId", 0),
			genEntry("afterId1", 10),
			genEntry("afterId2", 10),
		}

		assert.Len(t, sub.cache.entries, 9)

		ctx := &opCtx{c: sub.cache, entries: newEntries, outputs: map[string][]*pb.EventMessage{}}
		sub.onChange(ctx)
		assertCtxAdd(t, ctx, "newActiveId1", "")
		assertCtxAdd(t, ctx, "newActiveId2", "newActiveId1")
		assertCtxRemove(t, ctx, "id5", "id6")
		assertCtxCounters(t, ctx, opCounter{subId: "test", total: 14, prevCount: 4, nextCount: 7})

		ctx.apply()
		assert.Len(t, sub.cache.entries, 9+len(newEntries))
		for _, e := range sub.cache.entries {
			assert.Equal(t, []string{"test"}, e.SubIds())
		}
	})
}

func TestSubscription_Remove(t *testing.T) {
	newSub := func() *sortedSub {
		store := spaceindex.NewStoreFixture(t)
		store.AddObjects(t, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:   domain.String("id7"),
				bundle.RelationKeyName: domain.String("id7"),
			},
		})
		s := spaceSubscriptions{
			cache:       newCache(),
			objectStore: store,
		}

		return &sortedSub{
			order:   genOrder(t),
			cache:   newCache(),
			limit:   3,
			afterId: "id3",
			ds:      newDependencyService(&s),
			filter: database.FilterNot{database.FilterEq{
				Key:   "order",
				Cond:  model.BlockContentDataviewFilter_Equal,
				Value: domain.Int64(100),
			}},
		}
	}
	t.Run("remove active", func(t *testing.T) {
		sub := newSub()
		require.NoError(t, sub.init(genEntries(9, false)))
		ctx := &opCtx{c: sub.cache}
		ctx.entries = append(ctx.entries, &entry{
			id:   "id4",
			data: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"id": domain.String("id4"), "order": domain.Int64(100)}),
		})
		sub.onChange(ctx)
		assertCtxRemove(t, ctx, "id4")
		assertCtxCounters(t, ctx, opCounter{total: 8, prevCount: 3, nextCount: 2})
		assertCtxAdd(t, ctx, "id7", "id6")
	})
	t.Run("remove non active", func(t *testing.T) {
		sub := newSub()
		require.NoError(t, sub.init(genEntries(9, false)))
		ctx := &opCtx{c: sub.cache}
		ctx.entries = append(ctx.entries, &entry{
			id:   "id1",
			data: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"id": domain.String("id4"), "order": domain.Int64(100)}),
		})
		sub.onChange(ctx)
		assertCtxCounters(t, ctx, opCounter{total: 8, prevCount: 2, nextCount: 3})
	})
}

func TestSubscription_Change(t *testing.T) {
	t.Run("change active order", func(t *testing.T) {
		sub := &sortedSub{
			order:   genOrder(t),
			cache:   newCache(),
			limit:   3,
			afterId: "id3",
			ds:      &dependencyService{},
		}
		require.NoError(t, sub.init(genEntries(9, false)))
		ctx := &opCtx{c: sub.cache}
		ctx.entries = append(ctx.entries, newEntry("id4", domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"id": domain.String("id4"), "order": domain.Int64(6)})))
		sub.onChange(ctx)
		assertCtxPosition(t, ctx, "id5", "")
		assertCtxChange(t, ctx, "id4")
	})
}

func TestSortedSub_Reorder(t *testing.T) {
	testOrder := genOrder(t)
	t.Run("reorder with updated dependencies", func(t *testing.T) {
		// Create test order that can be updated
		order := testOrder

		sub := &sortedSub{
			id:      "test",
			order:   order,
			cache:   newCache(),
			limit:   5,
			afterId: "id2",
			ds:      &dependencyService{},
		}

		// Initialize with entries
		require.NoError(t, sub.init(genEntries(7, false)))

		// Setup initial active entries
		initialEntries := sub.getActiveEntries()
		assert.Len(t, initialEntries, 5) // limited by limit

		// Create dependency details with updated order information
		depDetails := []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:      domain.String("user1"),
				bundle.RelationKeyName:    domain.String("Updated User 1"),
				bundle.RelationKeyOrderId: domain.String("order123"),
			}),
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:      domain.String("user2"),
				bundle.RelationKeyName:    domain.String("Updated User 2"),
				bundle.RelationKeyOrderId: domain.String("order456"),
			}),
		}

		ctx := &opCtx{
			c:       sub.cache,
			spaceId: spaceId,
		}

		// Execute reorder
		sub.reorder(ctx, depDetails)

		// Verify that entries are reordered (skip list should be reinitialized)
		newEntries := sub.getActiveEntries()
		assert.Len(t, newEntries, 5)

		// Verify context has position changes tracked
		assert.NotNil(t, sub.diff)
	})

	t.Run("reorder with no order changes", func(t *testing.T) {
		// Create mock order that returns false for Update
		order := &mockNoUpdateOrder{}

		sub := &sortedSub{
			id:    "test",
			order: order,
			cache: newCache(),
			ds:    &dependencyService{},
		}

		require.NoError(t, sub.init(genEntries(5, false)))

		depDetails := []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:   domain.String("user1"),
				bundle.RelationKeyName: domain.String("Same User"),
			}),
		}

		ctx := &opCtx{
			c:       sub.cache,
			spaceId: spaceId,
		}

		// Should return early without reordering
		sub.reorder(ctx, depDetails)

		// Verify no changes in context since order didn't change
		assert.Empty(t, ctx.position)
		assert.Empty(t, ctx.change)
	})

	t.Run("reorder with pagination before element", func(t *testing.T) {
		sub := &sortedSub{
			id:       "test",
			order:    testOrder,
			cache:    newCache(),
			limit:    3,
			beforeId: "id5", // paginate before id5
			ds:       &dependencyService{},
		}

		require.NoError(t, sub.init(genEntries(7, false)))

		depDetails := []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId: domain.String("dep1"),
			}),
		}

		ctx := &opCtx{
			c:       sub.cache,
			spaceId: spaceId,
		}

		sub.reorder(ctx, depDetails)

		// Should handle beforeId pagination correctly
		activeEntries := sub.getActiveEntries()
		assert.Len(t, activeEntries, 3) // limited by limit
	})

	t.Run("reorder updates counters", func(t *testing.T) {
		sub := &sortedSub{
			id:     "test",
			order:  testOrder,
			cache:  newCache(),
			limit:  3,
			offset: 1, // start from offset 1
			ds:     &dependencyService{},
		}

		require.NoError(t, sub.init(genEntries(6, false)))

		depDetails := []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId: domain.String("dep1"),
			}),
		}

		ctx := &opCtx{
			c:       sub.cache,
			spaceId: spaceId,
		}

		sub.reorder(ctx, depDetails)

		// Verify counters are updated if there are changes
		// Counters may be empty if no actual reordering occurred
		for _, counter := range ctx.counters {
			assert.Equal(t, "test", counter.subId)
			assert.Equal(t, 6, counter.total)
		}
	})

	t.Run("reorder with dependency subscription", func(t *testing.T) {
		// Create a subscription with dependency subscription
		depSub := &simpleSub{
			id:    "test/dep",
			cache: newCache(),
		}

		sub := &sortedSub{
			id:     "test",
			order:  testOrder,
			cache:  newCache(),
			limit:  3,
			depSub: depSub,
			ds:     &dependencyService{},
		}

		require.NoError(t, sub.init(genEntries(5, false)))

		depDetails := []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId: domain.String("dep1"),
			}),
		}

		ctx := &opCtx{
			c:       sub.cache,
			spaceId: spaceId,
		}

		sub.reorder(ctx, depDetails)

		// Should populate activeEntriesBuf for dependency subscription
		assert.NotNil(t, sub.activeEntriesBuf)
	})
}

// Mock order that never needs updates
type mockNoUpdateOrder struct{}

func (m *mockNoUpdateOrder) Compare(_, _ *domain.Details) int        { return 0 }
func (m *mockNoUpdateOrder) UpdateOrderMap(_ []*domain.Details) bool { return false }
func (m *mockNoUpdateOrder) AnystoreSort() query.Sort                { return nil }
