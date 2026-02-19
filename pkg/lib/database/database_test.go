package database

import (
	"testing"

	"github.com/anyproto/any-store/anyenc"
	"github.com/stretchr/testify/assert"
	"golang.org/x/text/collate"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestDatabase(t *testing.T) {

	t.Run("include time - when single date sort", func(t *testing.T) {
		testIncludeTimeWhenSingleDateSort(t)
	})

	t.Run("include time - when sort contains include time", func(t *testing.T) {
		testIncludeTimeWhenSortContainsIncludeTime(t)
	})

	t.Run("do not include time - when not single sort", func(t *testing.T) {
		testDoNotIncludeTimeWhenNotSingleSort(t)
	})

	t.Run("do not include time - when single not date sort", func(t *testing.T) {
		testDoNotIncludeTimeWhenSingleNotDateSort(t)
	})
}

type stubSpaceObjectStore struct {
	queryRawResult []Record
	options        []*model.RelationOption
	iterate        func(q Query, proc func(record *domain.Details)) error
}

func (s *stubSpaceObjectStore) SpaceId() string {
	return "space1"
}

func (s *stubSpaceObjectStore) Query(q Query) (records []Record, err error) {
	return s.queryRawResult, nil
}

func (s *stubSpaceObjectStore) QueryRaw(filters *Filters, limit int, offset int) ([]Record, error) {
	return s.queryRawResult, nil
}

func (s *stubSpaceObjectStore) GetRelationFormatByKey(key domain.RelationKey) (model.RelationFormat, error) {
	rel, err := bundle.GetRelation(key)
	if err != nil {
		return 0, nil
	}
	return rel.Format, nil
}

func (s *stubSpaceObjectStore) ListRelationOptions(relationKey domain.RelationKey) (options []*model.RelationOption, err error) {
	return s.options, nil
}

func (s *stubSpaceObjectStore) QueryIterate(q Query, proc func(record *domain.Details)) error {
	if s.iterate != nil {
		return s.iterate(q, proc)
	}
	for _, record := range s.queryRawResult {
		proc(record.Details)
	}
	return nil
}

func newTestQueryBuilder(t *testing.T) queryBuilder {
	objectStore := &stubSpaceObjectStore{}
	return queryBuilder{
		objectStore: objectStore,
		arena:       &anyenc.Arena{},
	}
}

func testIncludeTimeWhenSingleDateSort(t *testing.T) {
	// given
	sorts := givenSingleDateSort()
	qb := newTestQueryBuilder(t)

	// when
	order := qb.extractOrder(sorts)

	// then
	assertIncludeTime(t, order)
}

func testDoNotIncludeTimeWhenNotSingleSort(t *testing.T) {
	// given
	sorts := givenNotSingleDateSort()
	qb := newTestQueryBuilder(t)

	// when
	order := qb.extractOrder(sorts)

	// then
	assertNotIncludeTime(t, order)
}

func testIncludeTimeWhenSortContainsIncludeTime(t *testing.T) {
	// given
	sorts := givenSingleIncludeTime()
	qb := newTestQueryBuilder(t)

	// when
	order := qb.extractOrder(sorts)

	// then
	assertIncludeTime(t, order)
}

func testDoNotIncludeTimeWhenSingleNotDateSort(t *testing.T) {
	// given
	sorts := givenSingleNotDateSort()
	qb := newTestQueryBuilder(t)

	// when
	order := qb.extractOrder(sorts)

	// then
	assertNotIncludeTime(t, order)
}

func assertIncludeTime(t *testing.T, order setOrder) {
	assert.IsType(t, order[0], &keyOrder{})
	assert.Equal(t, order[0].(*keyOrder).includeTime, true)
}

func assertNotIncludeTime(t *testing.T, order setOrder) {
	assert.IsType(t, order[0], &keyOrder{})
	assert.Equal(t, order[0].(*keyOrder).includeTime, false)
}

func givenSingleDateSort() []SortRequest {
	sorts := make([]SortRequest, 1)
	sorts[0] = SortRequest{
		Format: model.RelationFormat_date,
	}
	return sorts
}

func givenNotSingleDateSort() []SortRequest {
	sorts := givenSingleDateSort()
	sorts = append(sorts, SortRequest{
		Format: model.RelationFormat_shorttext,
	})
	return sorts
}

func givenSingleNotDateSort() []SortRequest {
	sorts := make([]SortRequest, 1)
	sorts[0] = SortRequest{
		Format: model.RelationFormat_shorttext,
	}
	return sorts
}

func givenSingleIncludeTime() []SortRequest {
	sorts := make([]SortRequest, 1)
	sorts[0] = SortRequest{
		Format:      model.RelationFormat_shorttext,
		IncludeTime: true,
	}
	return sorts
}

func Test_NewFilters(t *testing.T) {
	t.Run("only default filters", func(t *testing.T) {
		// given
		mockStore := &stubSpaceObjectStore{}

		// when
		filters, err := NewFilters(Query{}, mockStore, &anyenc.Arena{}, &collate.Buffer{})

		// then
		assert.Nil(t, err)
		assert.Len(t, filters.FilterObj, 3)
	})
	t.Run("and filter with 3 default", func(t *testing.T) {
		// given
		mockStore := &stubSpaceObjectStore{}
		filter := []FilterRequest{
			{
				Operator: model.BlockContentDataviewFilter_And,
				NestedFilters: []FilterRequest{
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: "relationKey",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.String("option2"),
						Format:      model.RelationFormat_status,
					},
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: bundle.RelationKeyName,
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.String("Object 1"),
						Format:      model.RelationFormat_shorttext,
					},
				},
			},
		}

		// when
		filters, err := NewFilters(Query{Filters: filter}, mockStore, &anyenc.Arena{}, &collate.Buffer{})

		// when
		assert.Nil(t, err)
		assert.NotNil(t, filters.FilterObj)
		assert.NotNil(t, filters.FilterObj.(FiltersAnd))
		assert.Len(t, filters.FilterObj.(FiltersAnd), 5)
	})
	t.Run("deleted filter", func(t *testing.T) {
		// given
		mockStore := &stubSpaceObjectStore{}
		filter := []FilterRequest{
			{
				Operator: model.BlockContentDataviewFilter_And,
				NestedFilters: []FilterRequest{
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: "relationKey",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.String("option2"),
						Format:      model.RelationFormat_status,
					},
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: bundle.RelationKeyName,
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.String("Object 1"),
						Format:      model.RelationFormat_shorttext,
					},
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: bundle.RelationKeyIsDeleted,
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.Bool(true),
					},
				},
			},
		}

		// then
		filters, err := NewFilters(Query{Filters: filter}, mockStore, &anyenc.Arena{}, &collate.Buffer{})

		// when
		assert.Nil(t, err)
		assert.NotNil(t, filters.FilterObj)
		assert.NotNil(t, filters.FilterObj.(FiltersAnd))
		assert.Len(t, filters.FilterObj.(FiltersAnd), 5)
	})
	t.Run("archived filter", func(t *testing.T) {
		// given
		mockStore := &stubSpaceObjectStore{}
		filter := []FilterRequest{
			{
				Operator: model.BlockContentDataviewFilter_And,
				NestedFilters: []FilterRequest{
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: "relationKey",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.String("option2"),
						Format:      model.RelationFormat_status,
					},
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: bundle.RelationKeyName,
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.String("Object 1"),
						Format:      model.RelationFormat_shorttext,
					},
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: bundle.RelationKeyIsArchived,
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.Bool(true),
					},
				},
			},
		}

		// then
		filters, err := NewFilters(Query{Filters: filter}, mockStore, &anyenc.Arena{}, &collate.Buffer{})

		// when
		assert.Nil(t, err)
		assert.NotNil(t, filters.FilterObj)
		assert.NotNil(t, filters.FilterObj.(FiltersAnd))
		assert.Len(t, filters.FilterObj.(FiltersAnd), 5)
	})
	t.Run("type filter", func(t *testing.T) {
		// given
		mockStore := &stubSpaceObjectStore{}
		filter := []FilterRequest{
			{
				Operator: model.BlockContentDataviewFilter_And,
				NestedFilters: []FilterRequest{
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: "relationKey",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.String("option2"),
						Format:      model.RelationFormat_status,
					},
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: bundle.RelationKeyName,
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.String("Object 1"),
						Format:      model.RelationFormat_shorttext,
					},
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: bundle.RelationKeyType,
						Condition:   model.BlockContentDataviewFilter_In,
						Value:       domain.Int64(model.ObjectType_space),
					},
				},
			},
		}

		// then
		filters, err := NewFilters(Query{Filters: filter}, mockStore, &anyenc.Arena{}, &collate.Buffer{})

		// when
		assert.Nil(t, err)
		assert.NotNil(t, filters.FilterObj)
		assert.NotNil(t, filters.FilterObj.(FiltersAnd))
		assert.Len(t, filters.FilterObj.(FiltersAnd), 6)
	})
	t.Run("or filter with 3 default", func(t *testing.T) {
		// given
		mockStore := &stubSpaceObjectStore{}
		filter := []FilterRequest{
			{
				Operator: model.BlockContentDataviewFilter_Or,
				NestedFilters: []FilterRequest{
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: "relationKey",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.String("option2"),
						Format:      model.RelationFormat_status,
					},
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: bundle.RelationKeyName,
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.String("Object 1"),
						Format:      model.RelationFormat_shorttext,
					},
				},
			},
		}

		// then
		filters, err := NewFilters(Query{Filters: filter}, mockStore, &anyenc.Arena{}, &collate.Buffer{})

		// when
		assert.Nil(t, err)
		assert.NotNil(t, filters.FilterObj)
		assert.NotNil(t, filters.FilterObj.(FiltersAnd))
		assert.Len(t, filters.FilterObj.(FiltersAnd), 4)
		assert.NotNil(t, filters.FilterObj.(FiltersAnd)[0].(FiltersOr))
		assert.Len(t, filters.FilterObj.(FiltersAnd)[0].(FiltersOr), 2)
	})
}

func TestFiltersFromProto(t *testing.T) {
	t.Run("no filters", func(t *testing.T) {
		// given
		var protoFilters []*model.BlockContentDataviewFilter

		// when
		result := FiltersFromProto(protoFilters)

		// then
		assert.NotNil(t, result)
		assert.Len(t, result, 0)
	})

	t.Run("single filter without nesting", func(t *testing.T) {
		// given
		protoFilters := []*model.BlockContentDataviewFilter{
			{
				Id:          "filter1",
				Operator:    model.BlockContentDataviewFilter_No,
				RelationKey: "relationKey1",
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String("value1").ToProto(),
				Format:      model.RelationFormat_shorttext,
			},
		}

		// when
		result := FiltersFromProto(protoFilters)

		// then
		assert.Len(t, result, 1)
		assert.Equal(t, "filter1", result[0].Id)
		assert.Equal(t, domain.RelationKey("relationKey1"), result[0].RelationKey)
		assert.Equal(t, model.BlockContentDataviewFilter_Equal, result[0].Condition)
		assert.Equal(t, domain.String("value1"), result[0].Value)
		assert.Equal(t, model.RelationFormat_shorttext, result[0].Format)
		assert.Empty(t, result[0].NestedFilters)
	})

	t.Run("nested filters", func(t *testing.T) {
		// given
		protoFilters := []*model.BlockContentDataviewFilter{
			{
				Id:       "filter1",
				Operator: model.BlockContentDataviewFilter_And,
				NestedFilters: []*model.BlockContentDataviewFilter{
					{
						Id:          "nestedFilter1",
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: "relationKey2",
						Condition:   model.BlockContentDataviewFilter_NotEqual,
						Value:       domain.String("value2").ToProto(),
						Format:      model.RelationFormat_date,
					},
					{
						Id:          "nestedFilter2",
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: "relationKey3",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.String("value3").ToProto(),
						Format:      model.RelationFormat_status,
					},
				},
			},
		}

		// when
		result := FiltersFromProto(protoFilters)

		// then
		assert.Len(t, result, 1)
		assert.Equal(t, "filter1", result[0].Id)
		assert.NotNil(t, result[0].NestedFilters)

		nested := result[0].NestedFilters
		assert.Len(t, nested, 2)
		assert.Equal(t, "nestedFilter1", nested[0].Id)
		assert.Equal(t, domain.RelationKey("relationKey2"), nested[0].RelationKey)
		assert.Equal(t, model.BlockContentDataviewFilter_NotEqual, nested[0].Condition)
		assert.Equal(t, domain.String("value2"), nested[0].Value)
		assert.Equal(t, model.RelationFormat_date, nested[0].Format)
		assert.Equal(t, "nestedFilter2", nested[1].Id)
		assert.Equal(t, domain.RelationKey("relationKey3"), nested[1].RelationKey)
		assert.Equal(t, model.BlockContentDataviewFilter_Equal, nested[1].Condition)
		assert.Equal(t, domain.String("value3"), nested[1].Value)
		assert.Equal(t, model.RelationFormat_status, nested[1].Format)
	})

	t.Run("deeply nested filters", func(t *testing.T) {
		// given
		protoFilters := []*model.BlockContentDataviewFilter{
			{
				Id:       "filter1",
				Operator: model.BlockContentDataviewFilter_And,
				NestedFilters: []*model.BlockContentDataviewFilter{
					{
						Id:       "nestedFilter1",
						Operator: model.BlockContentDataviewFilter_Or,
						NestedFilters: []*model.BlockContentDataviewFilter{
							{
								Id:          "deepNestedFilter1",
								Operator:    model.BlockContentDataviewFilter_No,
								RelationKey: "relationKey3",
								Condition:   model.BlockContentDataviewFilter_Equal,
								Value:       domain.String("value3").ToProto(),
								Format:      model.RelationFormat_status,
							},
							{
								Id:          "deepNestedFilter2",
								Operator:    model.BlockContentDataviewFilter_No,
								RelationKey: "relationKey4",
								Condition:   model.BlockContentDataviewFilter_NotEqual,
								Value:       domain.String("value4").ToProto(),
								Format:      model.RelationFormat_shorttext,
							},
						},
					},
				},
			},
		}

		// when
		result := FiltersFromProto(protoFilters)

		// then
		assert.Len(t, result, 1)
		assert.NotNil(t, result[0].NestedFilters)

		nested := result[0].NestedFilters
		assert.Len(t, nested, 1)

		deepNested := nested[0].NestedFilters
		assert.Len(t, deepNested, 2)
		assert.Equal(t, "deepNestedFilter1", deepNested[0].Id)
		assert.Equal(t, domain.RelationKey("relationKey3"), deepNested[0].RelationKey)
		assert.Equal(t, model.BlockContentDataviewFilter_Equal, deepNested[0].Condition)
		assert.Equal(t, domain.String("value3"), deepNested[0].Value)
		assert.Equal(t, model.RelationFormat_status, deepNested[0].Format)
		assert.Equal(t, "deepNestedFilter2", deepNested[1].Id)
		assert.Equal(t, domain.RelationKey("relationKey4"), deepNested[1].RelationKey)
		assert.Equal(t, model.BlockContentDataviewFilter_NotEqual, deepNested[1].Condition)
		assert.Equal(t, domain.String("value4"), deepNested[1].Value)
		assert.Equal(t, model.RelationFormat_shorttext, deepNested[1].Format)

	})
}
