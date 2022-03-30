package subscription

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database/filter"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
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
		res = append(res, genEntry(id, int64(ord)))
	}
	rand.Shuffle(len(res), func(i, j int) { res[i], res[j] = res[j], res[i] })
	return
}

func genEntry(id string, ord int64) *entry {
	return &entry{
		id: id,
		data: &types.Struct{Fields: map[string]*types.Value{
			"id":    pbtypes.String(id),
			"order": pbtypes.Int64(ord),
		}},
	}
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
