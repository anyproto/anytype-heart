package chatrepository

import (
	"context"
	"fmt"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/anystorehelper"
)

// The cost of getting filterReactionUnread wrong, at the collection size a busy chat reaches.
//
// $exists cannot be served by the sparse reactionUnreadOrderId index (any-store v1.0.2 —
// GO-7510), so it scans the whole collection while returning the same rows. Measured here on
// 50k messages with 20 unread reactions, darwin/arm64:
//
//	NewestUnread_Exists    8,196 µs   full scan
//	NewestUnread_NotNull      24 µs   index seek, serves the sort too   (339x)
//	AllUnread_Exists       9,072 µs   full scan
//	AllUnread_NotNull        456 µs   index seek                         (20x)
//
// ClearUnreadReactions pays the NewestUnread cost once per 100-row batch until the batch is
// short, so ~1,000 unread reactions is ~10 full scans.
func seedMessages(tb testing.TB, total, withUnreadReaction int) *fixture {
	tb.Helper()
	fx := newFixture(tb)
	require.NoError(tb, anystorehelper.AddIndexes(context.Background(), fx.repo.collection, chatCollectionIndexes))

	arena := &anyenc.Arena{}
	ctx := context.Background()
	every := total / withUnreadReaction
	for i := 0; i < total; i++ {
		arena.Reset()
		doc := arena.NewObject()
		doc.Set("id", arena.NewString(fmt.Sprintf("msg%06d", i)))
		order := arena.NewObject()
		order.Set("id", arena.NewString(fmt.Sprintf("ord%06d", i)))
		doc.Set("_o", order)
		doc.Set(chatmodel.CreatorKey, arena.NewString("creator"))
		if i%every == 0 {
			doc.Set(chatmodel.ReactionUnreadOrderIdKey, arena.NewString(fmt.Sprintf("ord%06d", i)))
		}
		require.NoError(tb, fx.repo.collection.Insert(ctx, doc))
	}
	return fx
}

// filterReactionUnreadExists is the pre-v1.0.2 shape, kept only as the benchmark baseline.
var filterReactionUnreadExists = query.Key{
	Path:   []string{chatmodel.ReactionUnreadOrderIdKey},
	Filter: query.Exists{},
}

func benchmarkDrain(b *testing.B, q func(*fixture) anystore.Iterator) {
	fx := seedMessages(b, 50000, 20)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		iter := q(fx)
		for iter.Next() {
			_, _ = iter.Doc()
		}
		require.NoError(b, iter.Close())
	}
}

func benchmarkNewest(b *testing.B, f query.Filter) {
	ctx := context.Background()
	benchmarkDrain(b, func(fx *fixture) anystore.Iterator {
		iter, err := fx.repo.collection.Find(f).Sort("-" + chatmodel.ReactionUnreadOrderIdKey).Limit(1).Iter(ctx)
		require.NoError(b, err)
		return iter
	})
}

func benchmarkAll(b *testing.B, f query.Filter) {
	ctx := context.Background()
	benchmarkDrain(b, func(fx *fixture) anystore.Iterator {
		iter, err := fx.repo.collection.Find(f).Iter(ctx)
		require.NoError(b, err)
		return iter
	})
}

func BenchmarkNewestUnread_Exists(b *testing.B)  { benchmarkNewest(b, filterReactionUnreadExists) }
func BenchmarkNewestUnread_NotNull(b *testing.B) { benchmarkNewest(b, filterReactionUnread) }
func BenchmarkAllUnread_Exists(b *testing.B)     { benchmarkAll(b, filterReactionUnreadExists) }
func BenchmarkAllUnread_NotNull(b *testing.B)    { benchmarkAll(b, filterReactionUnread) }
