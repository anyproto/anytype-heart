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

func TestWithIdTieBreak(t *testing.T) {
	asc, desc := model.BlockContentDataviewSort_Asc, model.BlockContentDataviewSort_Desc

	t.Run("no sorts get the id ascending", func(t *testing.T) {
		// given
		want := []SortRequest{IdTieBreak(asc)}

		// when
		got := WithIdTieBreak(nil)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("the id runs in the direction of the last sort", func(t *testing.T) {
		// given
		sorts := []SortRequest{
			{RelationKey: bundle.RelationKeyName, Type: asc},
			{RelationKey: bundle.RelationKeyLastModifiedDate, Type: desc, IncludeTime: true},
		}
		want := append(append([]SortRequest(nil), sorts...), IdTieBreak(desc))

		// when
		got := WithIdTieBreak(sorts)

		// then
		assert.Equal(t, want, got)
		assert.Len(t, sorts, 2, "the caller's slice is not extended")
	})

	t.Run("after a relevance sort the id runs ascending", func(t *testing.T) {
		// given: tied matches are grouped by id ascending
		sorts := []SortRequest{{RelationKey: bundle.RelationKey_final_score, Type: desc}}
		want := []SortRequest{sorts[0], IdTieBreak(asc)}

		// when
		got := WithIdTieBreak(sorts)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("an explicit day precision is kept", func(t *testing.T) {
		// given
		sorts := []SortRequest{{RelationKey: bundle.RelationKeyDueDate, Type: asc, Format: model.RelationFormat_date}}
		want := []SortRequest{sorts[0], IdTieBreak(asc)}

		// when
		got := WithIdTieBreak(sorts)

		// then
		assert.Equal(t, want, got)
		assert.False(t, extractOrder(&stubSpaceObjectStore{}, &anyenc.Arena{}, &collate.Buffer{}, got)[0].(*keyOrder).includeTime)
	})

	t.Run("a raw id already in the sorts is kept", func(t *testing.T) {
		// given
		sorts := []SortRequest{IdTieBreak(desc), {RelationKey: bundle.RelationKeyName, Type: asc}}

		// when
		got := WithIdTieBreak(sorts)

		// then
		assert.Equal(t, sorts, got)
	})

	t.Run("a caller's id sort still gets the raw id", func(t *testing.T) {
		// given: an app's id sort orders by name, so names can tie
		sorts := []SortRequest{{RelationKey: bundle.RelationKeyId, Type: asc}}
		want := []SortRequest{sorts[0], IdTieBreak(asc)}

		// when
		got := WithIdTieBreak(sorts)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("a manual order still gets the raw id", func(t *testing.T) {
		// given: the objects a manual order does not list can tie
		sorts := []SortRequest{{
			RelationKey: bundle.RelationKeyId, Type: model.BlockContentDataviewSort_Custom,
			CustomOrder: []domain.Value{domain.String("listed")},
		}}
		want := []SortRequest{sorts[0], IdTieBreak(asc)}

		// when
		got := WithIdTieBreak(sorts)

		// then
		assert.Equal(t, want, got)
	})
}

func TestImplyIncludeTime(t *testing.T) {
	t.Run("a lone date sort gets the seconds the store implies", func(t *testing.T) {
		// given
		sorts := []SortRequest{{RelationKey: bundle.RelationKeyDueDate, Format: model.RelationFormat_date}}
		want := []SortRequest{{RelationKey: bundle.RelationKeyDueDate, Format: model.RelationFormat_date, IncludeTime: true}}

		// when
		got := ImplyIncludeTime(sorts)

		// then
		assert.Equal(t, want, got)
		assert.False(t, sorts[0].IncludeTime, "the caller's sort is not modified")
	})

	t.Run("a date sort among others is kept", func(t *testing.T) {
		// given
		sorts := []SortRequest{
			{RelationKey: bundle.RelationKeyDueDate, Format: model.RelationFormat_date},
			{RelationKey: bundle.RelationKeyName},
		}

		// when
		got := ImplyIncludeTime(sorts)

		// then
		assert.Equal(t, sorts, got)
	})
}

func TestIdSortOrders(t *testing.T) {
	zulu := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId: domain.String("a"), bundle.RelationKeyName: domain.String("Zulu"),
	})
	alpha := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId: domain.String("b"), bundle.RelationKeyName: domain.String("Alpha"),
	})

	t.Run("the tie-break orders ids byte-wise without reading the store", func(t *testing.T) {
		// given
		store := &stubSpaceObjectStore{iterate: func(Query, func(*domain.Details)) error {
			t.Fatal("the id tie-break must not read the store")
			return nil
		}}
		upper := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyId: domain.String("B")})
		lower := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyId: domain.String("a")})

		// when
		asc := NewKeyOrder(store, &anyenc.Arena{}, &collate.Buffer{}, IdTieBreak(model.BlockContentDataviewSort_Asc))
		desc := NewKeyOrder(store, &anyenc.Arena{}, &collate.Buffer{}, IdTieBreak(model.BlockContentDataviewSort_Desc))

		// then: "B" < "a" in bytes; a case-folding collator would say the opposite
		assertCompare(t, asc, upper, lower, -1)
		assertCompare(t, desc, upper, lower, 1)
	})

	t.Run("an app's id sort orders by the objects' names", func(t *testing.T) {
		// given: a view sorted by id ascending
		store := &stubSpaceObjectStore{queryRawResult: []Record{{Details: zulu}, {Details: alpha}}}
		order := NewKeyOrder(store, &anyenc.Arena{}, &collate.Buffer{}, SortRequest{
			RelationKey: bundle.RelationKeyId, Type: model.BlockContentDataviewSort_Asc,
		})

		// when, then: Alpha (id b) before Zulu (id a)
		assert.Equal(t, 1, order.Compare(zulu, alpha))
	})

	t.Run("a manual order compares unlisted objects by name, then the next sort", func(t *testing.T) {
		// given: the desktop's manual order, then the view's own due date
		// sort, which ranks Zulu first
		monday := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC).Unix()
		tuesday := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC).Unix()
		zuluMonday := zulu.Copy()
		zuluMonday.SetInt64(bundle.RelationKeyDueDate, monday)
		alphaTuesday := alpha.Copy()
		alphaTuesday.SetInt64(bundle.RelationKeyDueDate, tuesday)
		sorts := []SortRequest{
			{RelationKey: bundle.RelationKeyId, Type: model.BlockContentDataviewSort_Custom, CustomOrder: []domain.Value{domain.String("listed")}},
			{RelationKey: bundle.RelationKeyDueDate, Type: model.BlockContentDataviewSort_Asc, IncludeTime: true},
		}
		store := &stubSpaceObjectStore{queryRawResult: []Record{{Details: zuluMonday}, {Details: alphaTuesday}}}

		// when
		got := extractOrder(store, &anyenc.Arena{}, &collate.Buffer{}, sorts).Compare(zuluMonday, alphaTuesday)

		// then: the names decide before the due date
		assert.Equal(t, 1, got)
	})

	t.Run("a manual order falls through to the next sort on equal names", func(t *testing.T) {
		// given
		monday := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC).Unix()
		tuesday := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC).Unix()
		early := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("z"), bundle.RelationKeyName: domain.String("Same"), bundle.RelationKeyDueDate: domain.Int64(monday),
		})
		late := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("a"), bundle.RelationKeyName: domain.String("Same"), bundle.RelationKeyDueDate: domain.Int64(tuesday),
		})
		sorts := []SortRequest{
			{RelationKey: bundle.RelationKeyId, Type: model.BlockContentDataviewSort_Custom, CustomOrder: []domain.Value{domain.String("listed")}},
			{RelationKey: bundle.RelationKeyDueDate, Type: model.BlockContentDataviewSort_Asc, IncludeTime: true},
		}
		store := &stubSpaceObjectStore{queryRawResult: []Record{{Details: early}, {Details: late}}}

		// when
		got := extractOrder(store, &anyenc.Arena{}, &collate.Buffer{}, sorts).Compare(early, late)

		// then
		assert.Equal(t, -1, got)
	})
}

func TestSortKey(t *testing.T) {
	t.Run("keys follow the store where Compare does not", func(t *testing.T) {
		// given: a day-only date sort; the store puts an undated row last,
		// Compare puts it first
		day := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC).Unix()
		dated := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("dated"), bundle.RelationKeyDueDate: domain.Int64(day),
		})
		undated := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyId: domain.String("undated")})
		order := NewSetOrder(&stubSpaceObjectStore{}, &anyenc.Arena{}, &collate.Buffer{}, []SortRequest{
			{RelationKey: bundle.RelationKeyDueDate, Type: model.BlockContentDataviewSort_Asc},
			IdTieBreak(model.BlockContentDataviewSort_Asc),
		})
		arena := &anyenc.Arena{}

		// when
		got := bytes.Compare(SortKey(order, dated, arena), SortKey(order, undated, arena))

		// then
		assert.Equal(t, -1, got)
		assert.Equal(t, 1, order.Compare(dated, undated), "the in-memory order disagrees with the store")
	})
}
