package objectstore

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
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
		records, allLoaded, err := fx.QueryCrossSpaceNoWait(database.Query{
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
		records, allLoaded, err := fx.QueryCrossSpaceNoWait(database.Query{})

		// then
		require.NoError(t, err)
		assert.True(t, allLoaded)
		assert.Len(t, records, 4)
	})

	t.Run("offset beyond the merged set returns empty", func(t *testing.T) {
		// given
		fx := newLoadedFixture(t)

		// when
		records, allLoaded, err := fx.QueryCrossSpaceNoWait(database.Query{Offset: 10, Limit: 2})

		// then
		require.NoError(t, err)
		assert.True(t, allLoaded)
		assert.Empty(t, records)
	})

	t.Run("reports partial view while the warm-up has not finished", func(t *testing.T) {
		// given: a store whose warm-up never completed (loadedCh open)
		s := &dsObjectStore{loadedCh: make(chan struct{})}
		s.crossSpaceDrained = sync.NewCond(&s.lock)

		// when
		records, allLoaded, err := s.QueryCrossSpaceNoWait(database.Query{})

		// then: no error, no waiting — just an explicitly partial (empty) view
		require.NoError(t, err)
		assert.False(t, allLoaded)
		assert.Empty(t, records)
	})
}
