package subscription

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database/filter"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testOrder = filter.KeyOrder{
	Key: "order",
}

func genEntries(n int, backord bool) (res []*entry) {
	for i := 1; i <= n; i++ {
		id := fmt.Sprintf("id%d", i)
		ord := i
		if backord {
			ord = n - i
		}
		res = append(res, &entry{
			id: id,
			data: &types.Struct{Fields: map[string]*types.Value{
				"id":    pbtypes.String(id),
				"order": pbtypes.Int64(int64(ord)),
			}},
		})
	}
	rand.Shuffle(len(res), func(i, j int) { res[i], res[j] = res[j], res[i] })
	return
}

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
			inSet, inActive := sub.lookup("id50")
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
			inSet, inActive := sub.lookup("id11")
			assert.True(t, inSet, "inSet")
			assert.False(t, inActive, "inActive")
			inSet, inActive = sub.lookup("id10")
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

			inSet, inActive := sub.lookup("id49")
			assert.True(t, inSet, "inSet")
			assert.False(t, inActive, "inActive")
			inSet, inActive = sub.lookup("id50")
			assert.True(t, inSet, "inSet")
			assert.False(t, inActive, "inActive")
			inSet, inActive = sub.lookup("id51")
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
			inSet, inActive := sub.lookup("id51")
			assert.True(t, inSet, "inSet")
			assert.False(t, inActive, "inActive")

			inSet, inActive = sub.lookup("id50")
			assert.True(t, inSet, "inSet")
			assert.False(t, inActive, "inActive")

			inSet, inActive = sub.lookup("id49")
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

			inSet, inActive := sub.lookup("id49")
			assert.True(t, inSet, "inSet")
			assert.False(t, inActive, "inActive")

			inSet, inActive = sub.lookup("id60")
			assert.True(t, inSet, "inSet")
			assert.True(t, inActive, "inActive")

			inSet, inActive = sub.lookup("id61")
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

			inSet, inActive := sub.lookup("id51")
			assert.True(t, inSet, "inSet")
			assert.False(t, inActive, "inActive")

			inSet, inActive = sub.lookup("id40")
			assert.True(t, inSet, "inSet")
			assert.True(t, inActive, "inActive")

			inSet, inActive = sub.lookup("id39")
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

func BenchmarkSubscription_fill(b *testing.B) {
	entries := genEntries(100000, true)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sub := &subscription{
			order: testOrder,
			cache: newCache(),
		}
		sub.fill(entries)
	}
}
