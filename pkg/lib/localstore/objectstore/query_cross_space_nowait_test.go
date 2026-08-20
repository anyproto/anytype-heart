package objectstore

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestQueryCrossSpaceNoWait(t *testing.T) {
	newLoadedFixture := func(t *testing.T) *StoreFixture {
		fx := NewStoreFixture(t)
		fx.AddObjects(t, "space1", []TestObject{
			{bundle.RelationKeyId: domain.String("obj-a"), bundle.RelationKeyName: domain.String("a")},
			{bundle.RelationKeyId: domain.String("obj-c"), bundle.RelationKeyName: domain.String("c")},
		})
		fx.AddObjects(t, "space2", []TestObject{
			{bundle.RelationKeyId: domain.String("obj-b"), bundle.RelationKeyName: domain.String("b")},
			{bundle.RelationKeyId: domain.String("obj-d"), bundle.RelationKeyName: domain.String("d")},
		})
		require.NoError(t, fx.WaitStoresLoaded(context.Background()))
		return fx
	}

	t.Run("merges spaces with a global sort and paging", func(t *testing.T) {
		// given
		fx := newLoadedFixture(t)

		// when: global order d,c,b,a — offset 1 limit 2 must slice the merged set
		records, allLoaded, err := fx.QueryCrossSpaceNoWait(context.Background(), database.Query{
			Sorts: []database.SortRequest{
				{RelationKey: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Desc},
			},
			Offset: 1,
			Limit:  2,
		})

		// then
		require.NoError(t, err)
		assert.True(t, allLoaded)
		require.Len(t, records, 2)
		assert.Equal(t, "c", records[0].Details.GetString(bundle.RelationKeyName))
		assert.Equal(t, "b", records[1].Details.GetString(bundle.RelationKeyName))
	})

	t.Run("no paging returns the full merged set", func(t *testing.T) {
		// given
		fx := newLoadedFixture(t)

		// when
		records, allLoaded, err := fx.QueryCrossSpaceNoWait(context.Background(), database.Query{})

		// then
		require.NoError(t, err)
		assert.True(t, allLoaded)
		assert.Len(t, records, 4)
	})

	t.Run("offset beyond the merged set returns empty", func(t *testing.T) {
		// given
		fx := newLoadedFixture(t)

		// when
		records, allLoaded, err := fx.QueryCrossSpaceNoWait(context.Background(), database.Query{Offset: 10, Limit: 2})

		// then
		require.NoError(t, err)
		assert.True(t, allLoaded)
		assert.Empty(t, records)
	})

	t.Run("date sort keeps the time component across spaces", func(t *testing.T) {
		// review F3: NewKeyOrder alone cuts dates to day granularity; the
		// merge must reproduce the per-space includeTime resolution
		fx := NewStoreFixture(t)
		fx.AddObjects(t, "space1", []TestObject{
			{bundle.RelationKeyId: domain.String("obj-8"), bundle.RelationKeyName: domain.String("o8"), bundle.RelationKeyCreatedDate: domain.Int64(1_700_000_800)},
			{bundle.RelationKeyId: domain.String("obj-6"), bundle.RelationKeyName: domain.String("o6"), bundle.RelationKeyCreatedDate: domain.Int64(1_700_000_600)},
		})
		fx.AddObjects(t, "space2", []TestObject{
			{bundle.RelationKeyId: domain.String("obj-7"), bundle.RelationKeyName: domain.String("o7"), bundle.RelationKeyCreatedDate: domain.Int64(1_700_000_700)},
			{bundle.RelationKeyId: domain.String("obj-5"), bundle.RelationKeyName: domain.String("o5"), bundle.RelationKeyCreatedDate: domain.Int64(1_700_000_500)},
		})
		require.NoError(t, fx.WaitStoresLoaded(context.Background()))

		records, _, err := fx.QueryCrossSpaceNoWait(context.Background(), database.Query{
			Sorts: []database.SortRequest{
				{RelationKey: bundle.RelationKeyCreatedDate, Type: model.BlockContentDataviewSort_Desc, Format: model.RelationFormat_date, IncludeTime: true},
			},
		})
		require.NoError(t, err)
		got := make([]string, 0, len(records))
		for _, rec := range records {
			got = append(got, rec.Details.GetString(bundle.RelationKeyId))
		}
		// same-day timestamps must interleave across spaces, newest first
		assert.Equal(t, []string{"obj-8", "obj-7", "obj-6", "obj-5"}, got)
	})

	t.Run("order and paging are deterministic across identical calls", func(t *testing.T) {
		// review F2: the store snapshot iterates a map; ties must not break
		// on random space order or offset paging duplicates records
		fx := newLoadedFixture(t)

		var first []string
		for i := 0; i < 10; i++ {
			records, _, err := fx.QueryCrossSpaceNoWait(context.Background(), database.Query{})
			require.NoError(t, err)
			got := make([]string, 0, len(records))
			for _, rec := range records {
				got = append(got, rec.Details.GetString(bundle.RelationKeyId))
			}
			if first == nil {
				first = got
			} else {
				require.Equal(t, first, got, "call %d", i)
			}
		}

		// offset pages must partition the set with no duplicates
		seen := map[string]int{}
		for offset := 0; offset < 4; offset += 2 {
			records, _, err := fx.QueryCrossSpaceNoWait(context.Background(), database.Query{Offset: offset, Limit: 2})
			require.NoError(t, err)
			for _, rec := range records {
				seen[rec.Details.GetString(bundle.RelationKeyId)]++
			}
		}
		require.Len(t, seen, 4)
		for id, count := range seen {
			assert.Equal(t, 1, count, id)
		}
	})

	t.Run("tech space objects are excluded", func(t *testing.T) {
		// review F5/F7 (API lens F3): parity with iterateSpacesForFulltext
		// and the cross-space subscription
		fx := NewStoreFixture(t)
		fx.AddObjects(t, "space1", []TestObject{
			{bundle.RelationKeyId: domain.String("user-obj"), bundle.RelationKeyName: domain.String("user object")},
		})
		fx.AddObjects(t, TestTechSpaceId, []TestObject{
			{bundle.RelationKeyId: domain.String("spaceView1"), bundle.RelationKeyName: domain.String("space view")},
		})
		require.NoError(t, fx.WaitStoresLoaded(context.Background()))

		records, allLoaded, err := fx.QueryCrossSpaceNoWait(context.Background(), database.Query{})
		require.NoError(t, err)
		assert.True(t, allLoaded)
		require.Len(t, records, 1)
		assert.Equal(t, "user-obj", records[0].Details.GetString(bundle.RelationKeyId))
	})

	t.Run("negative paging is clamped, not unlimited", func(t *testing.T) {
		// review F7/F9: offset==-limit used to zero the per-space bound
		fx := newLoadedFixture(t)

		records, _, err := fx.QueryCrossSpaceNoWait(context.Background(), database.Query{Offset: -1, Limit: 1})
		require.NoError(t, err)
		assert.Len(t, records, 1)
	})

	t.Run("fulltext is scoped per space: a small space's match survives a noisy space", func(t *testing.T) {
		// review F1/F6: an unscoped query made every space run the same
		// global search — spaces whose matches don't rank in the global
		// candidate budget silently returned nothing
		fx := NewStoreFixture(t)
		for i := 0; i < 150; i++ {
			id := fmt.Sprintf("s1-obj-%03d", i)
			fx.AddObjects(t, "space1", []TestObject{
				{bundle.RelationKeyId: domain.String(id), bundle.RelationKeyName: domain.String("apple apple apple")},
			})
			require.NoError(t, fx.FullText.Index(ftsearch.SearchDoc{
				Id:      domain.NewObjectPathWithRelation(id, bundle.RelationKeyName.String()).String(),
				SpaceId: "space1",
				Title:   "apple apple apple",
			}))
		}
		fx.AddObjects(t, "space2", []TestObject{
			{bundle.RelationKeyId: domain.String("s2-lonely"), bundle.RelationKeyName: domain.String("one apple among other words")},
		})
		require.NoError(t, fx.FullText.Index(ftsearch.SearchDoc{
			Id:      domain.NewObjectPathWithRelation("s2-lonely", bundle.RelationKeyName.String()).String(),
			SpaceId: "space2",
			Title:   "one apple among other words",
		}))
		require.NoError(t, fx.WaitStoresLoaded(context.Background()))

		// no limit: before the fix even the unlimited query dropped the
		// space2 match — every space resolved the same global candidate page
		// and kept only its own docs, so a space whose matches don't rank in
		// the global page returned nothing at all
		records, allLoaded, err := fx.QueryCrossSpaceNoWait(context.Background(), database.Query{
			TextQuery: "apple",
		})
		require.NoError(t, err)
		assert.True(t, allLoaded)
		found := false
		for _, rec := range records {
			if rec.Details.GetString(bundle.RelationKeyId) == "s2-lonely" {
				found = true
			}
		}
		assert.True(t, found, "the weaker space2 match must not be starved by space1's candidates")
		// stronger space1 matches still outrank it: score-first merge order
		assert.NotEqual(t, "s2-lonely", records[0].Details.GetString(bundle.RelationKeyId))
	})

	t.Run("parallel per-space querying keeps the deterministic order", func(t *testing.T) {
		// per-space queries run concurrently (capped): completion order must
		// not leak into the result — slots are concatenated in sorted-space
		// order and the merge comparator is total
		fx := NewStoreFixture(t)
		want := make([]string, 0, 12)
		for spaceN := 1; spaceN <= 6; spaceN++ {
			spaceId := fmt.Sprintf("space%d", spaceN)
			for objN := 1; objN <= 2; objN++ {
				id := fmt.Sprintf("obj-%d-%d", spaceN, objN)
				fx.AddObjects(t, spaceId, []TestObject{
					{bundle.RelationKeyId: domain.String(id), bundle.RelationKeyName: domain.String("same name")},
				})
				want = append(want, id)
			}
		}
		require.NoError(t, fx.WaitStoresLoaded(context.Background()))

		for i := 0; i < 5; i++ {
			records, allLoaded, err := fx.QueryCrossSpaceNoWait(context.Background(), database.Query{
				Sorts: []database.SortRequest{
					{RelationKey: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Asc},
				},
			})
			require.NoError(t, err)
			assert.True(t, allLoaded)
			got := make([]string, 0, len(records))
			for _, rec := range records {
				got = append(got, rec.Details.GetString(bundle.RelationKeyId))
			}
			// all names tie: the id tiebreak yields one fixed global order
			require.Equal(t, want, got, "call %d", i)
		}
	})

	t.Run("parallel per-space querying keeps the deterministic order", func(t *testing.T) {
		// per-space queries run concurrently (capped): completion order must
		// not leak into the result — slots are concatenated in sorted-space
		// order and the merge comparator is total
		fx := NewStoreFixture(t)
		want := make([]string, 0, 12)
		for spaceN := 1; spaceN <= 6; spaceN++ {
			spaceId := fmt.Sprintf("space%d", spaceN)
			for objN := 1; objN <= 2; objN++ {
				id := fmt.Sprintf("obj-%d-%d", spaceN, objN)
				fx.AddObjects(t, spaceId, []TestObject{
					{bundle.RelationKeyId: domain.String(id), bundle.RelationKeyName: domain.String("same name")},
				})
				want = append(want, id)
			}
		}
		require.NoError(t, fx.WaitStoresLoaded(context.Background()))

		for i := 0; i < 5; i++ {
			records, allLoaded, err := fx.QueryCrossSpaceNoWait(context.Background(), database.Query{
				Sorts: []database.SortRequest{
					{RelationKey: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Asc},
				},
			})
			require.NoError(t, err)
			assert.True(t, allLoaded)
			got := make([]string, 0, len(records))
			for _, rec := range records {
				got = append(got, rec.Details.GetString(bundle.RelationKeyId))
			}
			// all names tie: the id tiebreak yields one fixed global order
			require.Equal(t, want, got, "call %d", i)
		}
	})

	t.Run("reports partial view while the warm-up has not finished", func(t *testing.T) {
		// given: a store whose warm-up never completed (loadedCh open)
		s := &dsObjectStore{loadedCh: make(chan struct{})}
		s.crossSpaceDrained = sync.NewCond(&s.lock)

		// when
		records, allLoaded, err := s.QueryCrossSpaceNoWait(context.Background(), database.Query{})

		// then: no error, no waiting — just an explicitly partial (empty) view
		require.NoError(t, err)
		assert.False(t, allLoaded)
		assert.Empty(t, records)
	})
}
