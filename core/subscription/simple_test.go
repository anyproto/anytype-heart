package subscription

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
)

func TestSimpleSub_Changes(t *testing.T) {
	t.Run("add to set", func(t *testing.T) {
		sub := &simpleSub{
			keys:  []domain.RelationKey{"id", "order"},
			cache: newCache(),
			isDep: true,
			ds:    &dependencyService{sorts: sortsMap{}},
		}
		require.NoError(t, sub.init(genEntries(10, false)))
		ctx := &opCtx{c: sub.cache}
		ctx.entries = append(ctx.entries, genEntry("id5", 109))
		sub.onChange(ctx)
		assertCtxChange(t, ctx, "id5")
	})
	t.Run("miss set", func(t *testing.T) {
		sub := &simpleSub{
			keys:  []domain.RelationKey{"id", "order"},
			cache: newCache(),
			isDep: true,
			ds:    &dependencyService{sorts: sortsMap{}},
		}
		require.NoError(t, sub.init(genEntries(10, false)))
		ctx := &opCtx{c: sub.cache}
		ctx.entries = append(ctx.entries, genEntry("id50", 100))
		sub.onChange(ctx)
		assertCtxEmpty(t, ctx)
	})
}

func TestSimpleSub_Refill(t *testing.T) {
	sub := &simpleSub{
		keys:  []domain.RelationKey{"id", "order"},
		cache: newCache(),
		isDep: true,
	}
	require.NoError(t, sub.init(genEntries(3, false)))
	ctx := &opCtx{}
	sub.refill(ctx, []*entry{genEntry("id3", 100), genEntry("id20", 200)})
	assertCtxChange(t, ctx, "id3")
	assertCtxRemove(t, ctx, "id1", "id2")
	assertCtxAdd(t, ctx, "id20", "")
}
