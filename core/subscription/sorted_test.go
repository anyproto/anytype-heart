package subscription

import (
	"testing"

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
	t.Run("add", func(t *testing.T) {
		sub := &sortedSub{
			id:      "test",
			order:   testOrder,
			cache:   newCache(),
			limit:   3,
			afterId: "id3",
			om:      newOrderManager(nil),
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
			order:   testOrder,
			cache:   newCache(),
			limit:   3,
			afterId: "id3",
			ds:      newDependencyService(&s),
			om:      newOrderManager(&s),
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
			order:   testOrder,
			cache:   newCache(),
			limit:   3,
			afterId: "id3",
			om:      newOrderManager(nil),
		}
		require.NoError(t, sub.init(genEntries(9, false)))
		ctx := &opCtx{c: sub.cache}
		ctx.entries = append(ctx.entries, newEntry("id4", domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"id": domain.String("id4"), "order": domain.Int64(6)})))
		sub.onChange(ctx)
		assertCtxPosition(t, ctx, "id5", "")
		assertCtxChange(t, ctx, "id4")
	})
}

func BenchmarkSubscription_fill(b *testing.B) {
	entries := genEntries(100000, true)
	c := newCache()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sub := &sortedSub{
			order: testOrder,
			cache: c,
		}
		sub.init(entries)
	}
}
