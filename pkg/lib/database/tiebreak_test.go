package database

import (
	"testing"

	"github.com/anyproto/any-store/v2/anyenc"
	"github.com/stretchr/testify/assert"
	"golang.org/x/text/collate"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestWithIdTieBreak(t *testing.T) {
	idSort := func(direction model.BlockContentDataviewSortType) SortRequest {
		return SortRequest{RelationKey: bundle.RelationKeyId, Type: direction, NoCollate: true}
	}

	t.Run("no sorts get the id ascending", func(t *testing.T) {
		// given
		want := []SortRequest{idSort(model.BlockContentDataviewSort_Asc)}

		// when
		got := WithIdTieBreak(nil)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("the id runs in the direction of the last sort", func(t *testing.T) {
		// given
		sorts := []SortRequest{
			{RelationKey: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Asc},
			{RelationKey: bundle.RelationKeyLastModifiedDate, Type: model.BlockContentDataviewSort_Desc, IncludeTime: true},
		}
		want := append(append([]SortRequest(nil), sorts...), idSort(model.BlockContentDataviewSort_Desc))

		// when
		got := WithIdTieBreak(sorts)

		// then
		assert.Equal(t, want, got)
		assert.Len(t, sorts, 2, "the caller's slice is not extended")
	})

	t.Run("a single date sort keeps the includeTime it implied", func(t *testing.T) {
		// given
		sorts := []SortRequest{{RelationKey: bundle.RelationKeyDueDate, Type: model.BlockContentDataviewSort_Asc, Format: model.RelationFormat_date}}
		want := []SortRequest{
			{RelationKey: bundle.RelationKeyDueDate, Type: model.BlockContentDataviewSort_Asc, Format: model.RelationFormat_date, IncludeTime: true},
			idSort(model.BlockContentDataviewSort_Asc),
		}

		// when
		got := WithIdTieBreak(sorts)

		// then
		assert.Equal(t, want, got)
		assert.False(t, sorts[0].IncludeTime, "the caller's sort is not modified")
		assert.True(t, extractOrder(&stubSpaceObjectStore{}, &anyenc.Arena{}, &collate.Buffer{}, got)[0].(*keyOrder).includeTime)
	})

	t.Run("a custom id order still gets the id last", func(t *testing.T) {
		// given: a manual order leaves the objects it does not list tied
		sorts := []SortRequest{{
			RelationKey: bundle.RelationKeyId, Type: model.BlockContentDataviewSort_Custom,
			CustomOrder: []domain.Value{domain.String("listed")},
		}}
		want := append(append([]SortRequest(nil), sorts...), idSort(model.BlockContentDataviewSort_Asc))

		// when
		got := WithIdTieBreak(sorts)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("sorts that already sort by id are kept", func(t *testing.T) {
		// given
		sorts := []SortRequest{
			idSort(model.BlockContentDataviewSort_Desc),
			{RelationKey: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Asc},
		}

		// when
		got := WithIdTieBreak(sorts)

		// then
		assert.Equal(t, sorts, got)
	})
}

func TestIdOrder(t *testing.T) {
	t.Run("orders ids byte-wise without an order map", func(t *testing.T) {
		// given: the id relation is object-formatted; an object order would
		// iterate the store to build a name map
		store := &stubSpaceObjectStore{iterate: func(Query, func(*domain.Details)) error {
			t.Fatal("an id sort must not read the store")
			return nil
		}}
		upper := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyId: domain.String("B")})
		lower := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyId: domain.String("a")})

		// when
		asc := NewKeyOrder(store, &anyenc.Arena{}, &collate.Buffer{}, SortRequest{RelationKey: bundle.RelationKeyId, Type: model.BlockContentDataviewSort_Asc})
		desc := NewKeyOrder(store, &anyenc.Arena{}, &collate.Buffer{}, SortRequest{RelationKey: bundle.RelationKeyId, Type: model.BlockContentDataviewSort_Desc})

		// then: "B" < "a" in bytes; a case-folding collator would say the opposite
		assertCompare(t, asc, upper, lower, -1)
		assertCompare(t, desc, upper, lower, 1)
	})
}

func TestCustomIdOrderKeepsNextSort(t *testing.T) {
	t.Run("objects a manual order does not list fall through to the next sort", func(t *testing.T) {
		// given: the desktop's manual order, an id sort of type custom,
		// followed by the view's own sort
		order := extractOrder(&stubSpaceObjectStore{}, &anyenc.Arena{}, &collate.Buffer{}, []SortRequest{
			{RelationKey: bundle.RelationKeyId, Type: model.BlockContentDataviewSort_Custom, CustomOrder: []domain.Value{domain.String("listed")}},
			{RelationKey: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Asc},
		})
		first := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("b"), bundle.RelationKeyName: domain.String("y"),
		})
		second := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("a"), bundle.RelationKeyName: domain.String("z"),
		})

		// when, then: by name, not by id
		assertCompare(t, order, first, second, -1)
	})
}
