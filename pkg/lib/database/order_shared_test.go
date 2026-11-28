package database

import (
	"bytes"
	"testing"
	"time"

	"github.com/anyproto/any-store/v2/anyenc"
	"github.com/stretchr/testify/assert"
	"golang.org/x/text/collate"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// countingOrderStore counts the store queries an order runs.
type countingOrderStore struct {
	*stubSpaceObjectStore
	queries int
}

func (s *countingOrderStore) Query(q Query) ([]Record, error) {
	s.queries++
	return s.stubSpaceObjectStore.Query(q)
}

// The app's views and subscriptions share these orders with API v2, which
// only appends its own id sort: each must order as develop does.
func TestSharedOrders(t *testing.T) {
	alice := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId: domain.String("alice"), bundle.RelationKeyName: domain.String("Alice"),
	})

	t.Run("a number view ascending with empty values last keeps zero first", func(t *testing.T) {
		// given
		zero := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeySizeInBytes: domain.Int64(0)})
		one := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeySizeInBytes: domain.Int64(1)})
		order := NewSetOrder(&stubSpaceObjectStore{}, &anyenc.Arena{}, &collate.Buffer{}, []SortRequest{{
			RelationKey: bundle.RelationKeySizeInBytes, Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_End,
		}})

		// when
		got := order.Compare(zero, one)

		// then
		assert.Equal(t, -1, got)
	})

	t.Run("an empty reference sorts with the empty values", func(t *testing.T) {
		// given: ascending, empty values last
		linked := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("linked"), bundle.RelationKeyAssignee: domain.StringList([]string{"alice"}),
		})
		empty := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("empty"), bundle.RelationKeyAssignee: domain.String(""),
		})
		store := &stubSpaceObjectStore{queryRawResult: []Record{{Details: alice}}}
		order := NewSetOrder(store, &anyenc.Arena{}, &collate.Buffer{}, []SortRequest{{
			RelationKey: bundle.RelationKeyAssignee, Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_End,
		}})

		// when, then: in memory and in the store's keys
		assertCompare(t, order, linked, empty, -1)
	})

	t.Run("a manual order leaves its unlisted objects to the next sort in the store", func(t *testing.T) {
		// given: the desktop's manual order, then the view's due date;
		// neither object is listed
		zuluMonday := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("a"), bundle.RelationKeyName: domain.String("Zulu"),
			bundle.RelationKeyDueDate: domain.Int64(time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC).Unix()),
		})
		alphaTuesday := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("b"), bundle.RelationKeyName: domain.String("Alpha"),
			bundle.RelationKeyDueDate: domain.Int64(time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC).Unix()),
		})
		store := &stubSpaceObjectStore{queryRawResult: []Record{{Details: zuluMonday}, {Details: alphaTuesday}}}
		order := NewSetOrder(store, &anyenc.Arena{}, &collate.Buffer{}, []SortRequest{
			{RelationKey: bundle.RelationKeyId, Type: model.BlockContentDataviewSort_Custom, CustomOrder: []domain.Value{domain.String("listed")}},
			{RelationKey: bundle.RelationKeyDueDate, Type: model.BlockContentDataviewSort_Asc, IncludeTime: true},
		})
		arena := &anyenc.Arena{}
		storeSort := order.AnystoreSort()

		// when
		got := bytes.Compare(storeSort.AppendKey(nil, zuluMonday.ToAnyEnc(arena)), storeSort.AppendKey(nil, alphaTuesday.ToAnyEnc(arena)))

		// then: Monday first
		assert.Equal(t, -1, got)
	})

	t.Run("an unresolved single reference costs no store query per row", func(t *testing.T) {
		// given: rows by a creator the store cannot resolve, next to a
		// resolvable object, so the order map is not empty
		store := &countingOrderStore{stubSpaceObjectStore: &stubSpaceObjectStore{queryRawResult: []Record{{Details: alice}}}}
		order := NewSetOrder(store, &anyenc.Arena{}, &collate.Buffer{}, []SortRequest{{
			RelationKey: bundle.RelationKeyCreator, Type: model.BlockContentDataviewSort_Asc,
		}})
		storeSort := order.AnystoreSort()
		arena := &anyenc.Arena{}
		row := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("row"), bundle.RelationKeyCreator: domain.String("missing"),
		})
		store.queries = 0

		// when
		for range 100 {
			storeSort.AppendKey(nil, row.ToAnyEnc(arena))
		}

		// then
		assert.Zero(t, store.queries)
	})
}
