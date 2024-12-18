package subscription

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
)

var testOrder = &database.KeyOrder{
	Key: "order",
}

func genEntries(n int, backord bool) (res []*entry) {
	for i := 1; i <= n; i++ {
		id := fmt.Sprintf("id%d", i)
		ord := i
		if backord {
			ord = n - i
		}
		res = append(res, genEntry(id, int64(ord)))
	}
	rand.Shuffle(len(res), func(i, j int) { res[i], res[j] = res[j], res[i] })
	return
}

func genEntry(id string, ord int64) *entry {
	return newEntry(id, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		"id":    domain.String(id),
		"order": domain.Int64(ord),
	}))
}

func assertCtxAdd(t *testing.T, ctx *opCtx, id, afterId string) {
	var found bool
	for _, add := range ctx.position {
		if add.isAdd && add.id == id {
			found = true
			assert.Equal(t, afterId, add.afterId, "add after id not equal")
			break
		}
	}
	assert.True(t, found, fmt.Sprintf("add id %v not found", id))
}

func assertCtxPosition(t *testing.T, ctx *opCtx, id, afterId string) {
	var found bool
	for _, pos := range ctx.position {
		if pos.id == id {
			found = true
			assert.Equal(t, afterId, pos.afterId, "pos after id not equal")
			break
		}
	}
	assert.True(t, found, fmt.Sprintf("pos id %v not found", id))
}

func assertCtxCounters(t *testing.T, ctx *opCtx, counter opCounter) {
	for _, c := range ctx.counters {
		assert.Equal(t, counter, c)
	}
}

func assertCtxRemove(t *testing.T, ctx *opCtx, ids ...string) {
	for _, id := range ids {
		var found bool
		for _, rem := range ctx.remove {
			if rem.id == id {
				found = true
				break
			}
		}
		assert.True(t, found, fmt.Sprintf("remove id %v not found", id))
	}
}

func assertCtxChange(t *testing.T, ctx *opCtx, ids ...string) {
	for _, id := range ids {
		var found bool
		for _, change := range ctx.change {
			if change.id == id {
				found = true
				break
			}
		}
		assert.True(t, found, fmt.Sprintf("change id %v not found", id))
	}
}

func assertCtxEmpty(t *testing.T, ctx *opCtx) {
	assert.Len(t, ctx.remove, 0, "remove not empty")
	assert.Len(t, ctx.counters, 0, "counters not empty")
	assert.Len(t, ctx.change, 0, "change not empty")
	assert.Len(t, ctx.position, 0, "position not empty")
}

func assertCtxGroup(t *testing.T, ctx *opCtx, added, removed int) {
	foundAdded := 0
	for _, g := range ctx.groups {
		if !g.remove {
			foundAdded++
		}
	}
	assert.Equal(t, foundAdded, added)

	foundRemoved := 0
	for _, g := range ctx.groups {
		if g.remove {
			foundRemoved++
		}
	}
	assert.Equal(t, foundRemoved, removed)
}
