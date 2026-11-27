package database

import (
	"bytes"
	"context"
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
	t.Run("a view's default sort gets the seconds the app runs it with", func(t *testing.T) {
		// given: what a new query view stores, with no format and no precision
		sorts := []SortRequest{{RelationKey: bundle.RelationKeyLastModifiedDate, Type: model.BlockContentDataviewSort_Desc}}
		want := []SortRequest{{
			RelationKey: bundle.RelationKeyLastModifiedDate, Type: model.BlockContentDataviewSort_Desc,
			Format: model.RelationFormat_date, IncludeTime: true,
		}}

		// when
		got := ImplyIncludeTime(&stubSpaceObjectStore{}, sorts)

		// then
		assert.Equal(t, want, got)
		assert.False(t, sorts[0].IncludeTime, "the caller's sort is not modified")
	})

	t.Run("a lone sort of another format is kept", func(t *testing.T) {
		// given
		sorts := []SortRequest{{RelationKey: bundle.RelationKeyName, Format: model.RelationFormat_date}}

		// when
		got := ImplyIncludeTime(&stubSpaceObjectStore{}, sorts)

		// then
		assert.Equal(t, sorts, got)
	})

	t.Run("a date sort among others is kept", func(t *testing.T) {
		// given
		sorts := []SortRequest{
			{RelationKey: bundle.RelationKeyDueDate, Format: model.RelationFormat_date},
			{RelationKey: bundle.RelationKeyName},
		}

		// when
		got := ImplyIncludeTime(&stubSpaceObjectStore{}, sorts)

		// then
		assert.Equal(t, sorts, got)
	})
}

func TestIdTieBreakMark(t *testing.T) {
	asc := model.BlockContentDataviewSort_Asc

	t.Run("no request can carry the mark", func(t *testing.T) {
		// given: the sort an existing subscription sends
		request := []*model.BlockContentDataviewSort{{RelationKey: bundle.RelationKeyId.String(), Type: asc, NoCollate: true}}

		// when
		got := SortsFromProto(request)

		// then
		assert.False(t, HasIdTieBreak(got))
	})

	t.Run("a marked context restores the trailing id sort", func(t *testing.T) {
		// given: what a cross-space search request carries
		sorts := SortsFromProto(SortsToProto(WithIdTieBreak([]SortRequest{{RelationKey: bundle.RelationKeyName, Type: asc}})))
		want := []SortRequest{sorts[0], IdTieBreak(asc)}

		// when
		got := RestoreIdTieBreak(ContextWithIdTieBreak(context.Background()), sorts)

		// then
		assert.Equal(t, want, got)
		assert.False(t, HasIdTieBreak(RestoreIdTieBreak(context.Background(), sorts)), "an unmarked context restores nothing")
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

	t.Run("an app's id sort orders by name in memory and by id in the store", func(t *testing.T) {
		// given: a view sorted by id ascending, with the tie-break appended
		store := &stubSpaceObjectStore{queryRawResult: []Record{{Details: zulu}, {Details: alpha}}}
		sorts := WithIdTieBreak([]SortRequest{{RelationKey: bundle.RelationKeyId, Type: model.BlockContentDataviewSort_Asc}})
		order := NewSetOrder(store, &anyenc.Arena{}, &collate.Buffer{}, sorts)
		arena := &anyenc.Arena{}

		// when
		inMemory := order.Compare(zulu, alpha)
		inStore := bytes.Compare(order.AnystoreSort().AppendKey(nil, zulu.ToAnyEnc(arena)), order.AnystoreSort().AppendKey(nil, alpha.ToAnyEnc(arena)))

		// then: a subscription puts Alpha (id b) first, as develop does; the
		// store reads an id as no reference, as develop does, so the raw id
		// decides and Zulu (id a) comes first
		assert.Equal(t, 1, inMemory)
		assert.Equal(t, -1, inStore)
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

		// when, then
		assertCompare(t, extractOrder(store, &anyenc.Arena{}, &collate.Buffer{}, sorts), early, late, -1)
	})
}

func TestSortKeyer(t *testing.T) {
	t.Run("keys follow the store where Compare does not", func(t *testing.T) {
		// given: a day-only date sort; the store puts an undated row last,
		// Compare puts it first
		day := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC).Unix()
		dated := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("dated"), bundle.RelationKeyDueDate: domain.Int64(day),
		})
		undated := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyId: domain.String("undated")})
		sorts := []SortRequest{
			{RelationKey: bundle.RelationKeyDueDate, Type: model.BlockContentDataviewSort_Asc},
			IdTieBreak(model.BlockContentDataviewSort_Asc),
		}
		keyer := NewSortKeyer(&stubSpaceObjectStore{}, sorts)

		// when
		got := bytes.Compare(keyer.Key(dated), keyer.Key(undated))

		// then
		assert.Equal(t, -1, got)
		order := NewSetOrder(&stubSpaceObjectStore{}, &anyenc.Arena{}, &collate.Buffer{}, sorts)
		assert.Equal(t, 1, order.Compare(dated, undated), "the in-memory order disagrees with the store")
	})

	t.Run("a key reads only what its sorts read", func(t *testing.T) {
		// given: two records that differ in a detail no sort reads
		keyer := NewSortKeyer(&stubSpaceObjectStore{}, []SortRequest{
			{RelationKey: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Asc},
			IdTieBreak(model.BlockContentDataviewSort_Asc),
		})
		narrow := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("a"), bundle.RelationKeyName: domain.String("Same"),
		})
		wide := narrow.Copy()
		wide.SetString(bundle.RelationKeyDescription, "not sorted on")

		// when, then
		assert.Equal(t, keyer.Key(narrow), keyer.Key(wide))
	})

	t.Run("an app's id sort keys as the store sorts it", func(t *testing.T) {
		// given: a view sorted by id ascending, as a request carries it,
		// with the tie-break appended
		zulu := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("a"), bundle.RelationKeyName: domain.String("Zulu"),
		})
		alpha := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("b"), bundle.RelationKeyName: domain.String("Alpha"),
		})
		store := &stubSpaceObjectStore{queryRawResult: []Record{{Details: zulu}, {Details: alpha}}}
		keyer := NewSortKeyer(store, WithIdTieBreak(SortsFromProto([]*model.BlockContentDataviewSort{
			{RelationKey: bundle.RelationKeyId.String(), Type: model.BlockContentDataviewSort_Asc, NoCollate: true},
		})))

		// when, then: the store reads an id as no reference, so the raw id
		// decides: Zulu (id a) before Alpha (id b)
		assert.Equal(t, -1, bytes.Compare(keyer.Key(zulu), keyer.Key(alpha)))
	})

	t.Run("a key compiles the store sort once", func(t *testing.T) {
		// given: a merge keys every row of every cut
		keyer := NewSortKeyer(&stubSpaceObjectStore{}, []SortRequest{
			{RelationKey: bundle.RelationKeyLastModifiedDate, Type: model.BlockContentDataviewSort_Desc, IncludeTime: true},
			IdTieBreak(model.BlockContentDataviewSort_Desc),
		})
		row := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("a"), bundle.RelationKeyLastModifiedDate: domain.Int64(1700000000),
		})

		// when
		allocs := testing.AllocsPerRun(100, func() { keyer.Key(row) })

		// then: the value and the key; rebuilding the sort per row cost 8
		assert.LessOrEqual(t, allocs, 2.0)
	})
}

func TestCustomOrderRanks(t *testing.T) {
	t.Run("a value listed twice keeps the unlisted values after the listed ones", func(t *testing.T) {
		// given: a number custom order that lists 1 twice, then the id
		two := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("listed"), bundle.RelationKeySizeInBytes: domain.Int64(2),
		})
		three := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("unlisted"), bundle.RelationKeySizeInBytes: domain.Int64(3),
		})
		order := NewSetOrder(&stubSpaceObjectStore{}, &anyenc.Arena{}, &collate.Buffer{}, WithIdTieBreak([]SortRequest{{
			RelationKey: bundle.RelationKeySizeInBytes, Type: model.BlockContentDataviewSort_Custom,
			CustomOrder: []domain.Value{domain.Int64(1), domain.Int64(1), domain.Int64(2)},
		}}))

		// when, then: the listed 2 before the unlisted 3, in memory and in
		// the store's keys
		assertCompare(t, order, two, three, -1)
	})
}
