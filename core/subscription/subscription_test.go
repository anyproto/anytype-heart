package subscription

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database/filter"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscription_Internal(t *testing.T) {
	t.Run("fill", func(t *testing.T) {
		t.Run("afterId err", func(t *testing.T) {
			sub := &subscription{
				order:   testOrder,
				cache:   newCache(),
				afterId: "id101",
			}
			require.Equal(t, ErrAfterId, sub.fill(genEntries(100, false)))
		})
		t.Run("beforeId err", func(t *testing.T) {
			sub := &subscription{
				order:    testOrder,
				cache:    newCache(),
				beforeId: "id101",
			}
			require.Equal(t, ErrBeforeId, sub.fill(genEntries(100, false)))
		})
	})
	t.Run("lookup", func(t *testing.T) {
		t.Run("no limits", func(t *testing.T) {
			sub := &subscription{
				order: testOrder,
				cache: newCache(),
			}
			require.NoError(t, sub.fill(genEntries(100, false)))
			inSet, inActive := sub.lookup(sub.cache.pick("id50"))
			assert.True(t, inSet, "inSet")
			assert.True(t, inActive, "inActive")
		})
		t.Run("with limit", func(t *testing.T) {
			sub := &subscription{
				order: testOrder,
				cache: newCache(),
				limit: 10,
			}
			require.NoError(t, sub.fill(genEntries(100, false)))
			inSet, inActive := sub.lookup(sub.cache.pick("id11"))
			assert.True(t, inSet, "inSet")
			assert.False(t, inActive, "inActive")
			inSet, inActive = sub.lookup(sub.cache.pick("id10"))
			assert.True(t, inSet, "inSet")
			assert.True(t, inActive, "inActive")
		})
		t.Run("afterId no limit", func(t *testing.T) {
			sub := &subscription{
				order:   testOrder,
				cache:   newCache(),
				afterId: "id50",
			}
			require.NoError(t, sub.fill(genEntries(100, false)))

			inSet, inActive := sub.lookup(sub.cache.pick("id49"))
			assert.True(t, inSet, "inSet")
			assert.False(t, inActive, "inActive")
			inSet, inActive = sub.lookup(sub.cache.pick("id50"))
			assert.True(t, inSet, "inSet")
			assert.False(t, inActive, "inActive")
			inSet, inActive = sub.lookup(sub.cache.pick("id51"))
			assert.True(t, inSet, "inSet")
			assert.True(t, inActive, "inActive")
		})
		t.Run("beforeId no limit", func(t *testing.T) {
			sub := &subscription{
				order:    testOrder,
				cache:    newCache(),
				beforeId: "id50",
			}
			require.NoError(t, sub.fill(genEntries(100, false)))
			inSet, inActive := sub.lookup(sub.cache.pick("id51"))
			assert.True(t, inSet, "inSet")
			assert.False(t, inActive, "inActive")

			inSet, inActive = sub.lookup(sub.cache.pick("id50"))
			assert.True(t, inSet, "inSet")
			assert.False(t, inActive, "inActive")

			inSet, inActive = sub.lookup(sub.cache.pick("id49"))
			assert.True(t, inSet, "inSet")
			assert.True(t, inActive, "inActive")
		})
		t.Run("afterId limit", func(t *testing.T) {
			sub := &subscription{
				order:   testOrder,
				cache:   newCache(),
				afterId: "id50",
				limit:   10,
			}
			require.NoError(t, sub.fill(genEntries(100, false)))

			inSet, inActive := sub.lookup(sub.cache.pick("id49"))
			assert.True(t, inSet, "inSet")
			assert.False(t, inActive, "inActive")

			inSet, inActive = sub.lookup(sub.cache.pick("id60"))
			assert.True(t, inSet, "inSet")
			assert.True(t, inActive, "inActive")

			inSet, inActive = sub.lookup(sub.cache.pick("id61"))
			assert.True(t, inSet, "inSet")
			assert.False(t, inActive, "inActive")
		})
		t.Run("beforeId limit", func(t *testing.T) {
			sub := &subscription{
				order:    testOrder,
				cache:    newCache(),
				beforeId: "id50",
				limit:    10,
			}
			require.NoError(t, sub.fill(genEntries(100, false)))

			inSet, inActive := sub.lookup(sub.cache.pick("id51"))
			assert.True(t, inSet, "inSet")
			assert.False(t, inActive, "inActive")

			inSet, inActive = sub.lookup(sub.cache.pick("id40"))
			assert.True(t, inSet, "inSet")
			assert.True(t, inActive, "inActive")

			inSet, inActive = sub.lookup(sub.cache.pick("id39"))
			assert.True(t, inSet, "inSet")
			assert.False(t, inActive, "inActive")
		})
	})
	t.Run("counters", func(t *testing.T) {
		t.Run("no limits", func(t *testing.T) {
			sub := &subscription{
				order: testOrder,
				cache: newCache(),
			}
			require.NoError(t, sub.fill(genEntries(6, false)))
			prev, next := sub.counters()
			assert.Equal(t, 0, prev, "prevCount")
			assert.Equal(t, 0, next, "nextCount")
		})
		t.Run("limit only", func(t *testing.T) {
			sub := &subscription{
				order: testOrder,
				cache: newCache(),
				limit: 2,
			}
			require.NoError(t, sub.fill(genEntries(6, false)))
			prev, next := sub.counters()
			assert.Equal(t, 0, prev, "prevCount")
			assert.Equal(t, 4, next, "nextCount")
		})
		t.Run("afterId no limit", func(t *testing.T) {
			sub := &subscription{
				order:   testOrder,
				cache:   newCache(),
				afterId: "id2",
			}
			require.NoError(t, sub.fill(genEntries(6, false)))
			prev, next := sub.counters()
			assert.Equal(t, 2, prev, "prevCount")
			assert.Equal(t, 0, next, "nextCount")
		})
		t.Run("beforeId no limit", func(t *testing.T) {
			sub := &subscription{
				order:    testOrder,
				cache:    newCache(),
				beforeId: "id3",
			}
			require.NoError(t, sub.fill(genEntries(6, false)))
			prev, next := sub.counters()
			assert.Equal(t, 0, prev, "prevCount")
			assert.Equal(t, 4, next, "nextCount")
		})
		t.Run("afterId with limit", func(t *testing.T) {
			sub := &subscription{
				order:   testOrder,
				cache:   newCache(),
				afterId: "id2",
				limit:   2,
			}
			require.NoError(t, sub.fill(genEntries(6, false)))
			prev, next := sub.counters()
			assert.Equal(t, 2, prev, "prevCount")
			assert.Equal(t, 2, next, "nextCount")
		})
		t.Run("beforeId with limit", func(t *testing.T) {
			sub := &subscription{
				order:    testOrder,
				cache:    newCache(),
				beforeId: "id5",
				limit:    2,
			}
			require.NoError(t, sub.fill(genEntries(6, false)))
			prev, next := sub.counters()
			assert.Equal(t, 2, prev, "prevCount")
			assert.Equal(t, 2, next, "nextCount")
		})
		t.Run("limit only - big limit", func(t *testing.T) {
			sub := &subscription{
				order: testOrder,
				cache: newCache(),
				limit: 20,
			}
			require.NoError(t, sub.fill(genEntries(6, false)))
			prev, next := sub.counters()
			assert.Equal(t, 0, prev, "prevCount")
			assert.Equal(t, 0, next, "nextCount")
		})
		t.Run("afterId - big limit", func(t *testing.T) {
			sub := &subscription{
				order:   testOrder,
				cache:   newCache(),
				limit:   20,
				afterId: "id2",
			}
			require.NoError(t, sub.fill(genEntries(6, false)))
			prev, next := sub.counters()
			assert.Equal(t, 2, prev, "prevCount")
			assert.Equal(t, 0, next, "nextCount")
		})
		t.Run("beforeId - big limit", func(t *testing.T) {
			sub := &subscription{
				order:    testOrder,
				cache:    newCache(),
				limit:    20,
				beforeId: "id5",
			}
			require.NoError(t, sub.fill(genEntries(6, false)))
			prev, next := sub.counters()
			assert.Equal(t, 0, prev, "prevCount")
			assert.Equal(t, 2, next, "nextCount")
		})
	})
}

func TestSubscription_Add(t *testing.T) {
	t.Run("add", func(t *testing.T) {
		sub := &subscription{
			order:   testOrder,
			cache:   newCache(),
			limit:   3,
			afterId: "id3",
		}
		require.NoError(t, sub.fill(genEntries(9, false)))
		newEntries := []*entry{
			genEntry("newActiveId1", 3),
			genEntry("newActiveId2", 3),
			genEntry("beforeId", 0),
			genEntry("afterId1", 10),
			genEntry("afterId2", 10),
		}

		ctx := newOpCtx()
		defer ctx.close()
		sub.onChangeBatch(ctx, newEntries...)
		assertCtxAdd(t, ctx, "newActiveId1", "id3")
		assertCtxAdd(t, ctx, "newActiveId2", "newActiveId1")
		assertCtxRemove(t, ctx, "id5", "id6")
		assertCtxCounters(t, ctx, opCounter{total: 14, prevCount: 4, nextCount: 7})
		t.Logf("%#v", ctx)
	})
}

func TestSubscription_Remove(t *testing.T) {
	newSub := func() *subscription {
		return &subscription{
			order:   testOrder,
			cache:   newCache(),
			limit:   3,
			afterId: "id3",
			filter: filter.Not{filter.Eq{
				Key:   "order",
				Cond:  model.BlockContentDataviewFilter_Equal,
				Value: pbtypes.Int64(100),
			}},
		}
	}
	t.Run("remove active", func(t *testing.T) {
		sub := newSub()
		require.NoError(t, sub.fill(genEntries(9, false)))
		ctx := newOpCtx()
		defer ctx.close()
		sub.onChangeBatch(ctx, &entry{
			id:   "id4",
			data: &types.Struct{Fields: map[string]*types.Value{"id": pbtypes.String("id4"), "order": pbtypes.Int64(100)}},
		})
		assertCtxRemove(t, ctx, "id4")
		assertCtxCounters(t, ctx, opCounter{total: 8, prevCount: 3, nextCount: 2})
		assertCtxAdd(t, ctx, "id7", "id6")
	})
	t.Run("remove non active", func(t *testing.T) {
		sub := newSub()
		require.NoError(t, sub.fill(genEntries(9, false)))
		ctx := newOpCtx()
		defer ctx.close()
		sub.onChangeBatch(ctx, &entry{
			id:   "id1",
			data: &types.Struct{Fields: map[string]*types.Value{"id": pbtypes.String("id4"), "order": pbtypes.Int64(100)}},
		})
		assertCtxCounters(t, ctx, opCounter{total: 8, prevCount: 2, nextCount: 3})
	})
}

func TestSubscription_Change(t *testing.T) {
	t.Run("change active order", func(t *testing.T) {
		sub := &subscription{
			order:   testOrder,
			cache:   newCache(),
			limit:   3,
			afterId: "id3",
		}
		require.NoError(t, sub.fill(genEntries(9, false)))
		ctx := newOpCtx()
		defer ctx.close()
		sub.onChangeBatch(ctx, &entry{
			id:   "id4",
			data: &types.Struct{Fields: map[string]*types.Value{"id": pbtypes.String("id4"), "order": pbtypes.Int64(6)}},
		})
		assertCtxPosition(t, ctx, "id4", "id5")
		assertCtxChange(t, ctx, "id4")
	})
}

func BenchmarkSubscription_fill(b *testing.B) {
	entries := genEntries(100000, true)
	c := newCache()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sub := &subscription{
			order: testOrder,
			cache: c,
		}
		sub.fill(entries)
	}
}
