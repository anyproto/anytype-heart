package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fastjson"
	"golang.org/x/text/collate"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
}

func (s *stubSpaceObjectStore) SpaceId() string {
	return "space1"
}

func (s *stubSpaceObjectStore) Query(q Query) (records []Record, err error) {
	return nil, nil
}

func (s *stubSpaceObjectStore) QueryRaw(filters *Filters, limit int, offset int) ([]Record, error) {
	return s.queryRawResult, nil
}

func (s *stubSpaceObjectStore) GetRelationFormatByKey(key string) (model.RelationFormat, error) {
	rel, err := bundle.GetRelation(domain.RelationKey(key))
	if err != nil {
		return 0, nil
	}
	return rel.Format, nil
}

func (s *stubSpaceObjectStore) ListRelationOptions(relationKey string) (options []*model.RelationOption, err error) {
	return nil, nil
}

func newTestQueryBuilder(t *testing.T) queryBuilder {
	objectStore := &stubSpaceObjectStore{}

	return queryBuilder{
		objectStore: objectStore,
		arena:       &fastjson.Arena{},
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

func assertIncludeTime(t *testing.T, order SetOrder) {
	assert.IsType(t, order[0], &KeyOrder{})
	assert.Equal(t, order[0].(*KeyOrder).IncludeTime, true)
}

func assertNotIncludeTime(t *testing.T, order SetOrder) {
	assert.IsType(t, order[0], &KeyOrder{})
	assert.Equal(t, order[0].(*KeyOrder).IncludeTime, false)
}

func givenSingleDateSort() []*model.BlockContentDataviewSort {
	sorts := make([]*model.BlockContentDataviewSort, 1)
	sorts[0] = &model.BlockContentDataviewSort{
		Format: model.RelationFormat_date,
	}
	return sorts
}

func givenNotSingleDateSort() []*model.BlockContentDataviewSort {
	sorts := givenSingleDateSort()
	sorts = append(sorts, &model.BlockContentDataviewSort{
		Format: model.RelationFormat_shorttext,
	})
	return sorts
}

func givenSingleNotDateSort() []*model.BlockContentDataviewSort {
	sorts := make([]*model.BlockContentDataviewSort, 1)
	sorts[0] = &model.BlockContentDataviewSort{
		Format: model.RelationFormat_shorttext,
	}
	return sorts
}

func givenSingleIncludeTime() []*model.BlockContentDataviewSort {
	sorts := make([]*model.BlockContentDataviewSort, 1)
	sorts[0] = &model.BlockContentDataviewSort{
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
		filters, err := NewFilters(Query{}, mockStore, &fastjson.Arena{}, &collate.Buffer{})

		// then
		assert.Nil(t, err)
		assert.Len(t, filters.FilterObj, 3)
	})
	t.Run("and filter with 3 default", func(t *testing.T) {
		// given
		mockStore := &stubSpaceObjectStore{}
		filter := []*model.BlockContentDataviewFilter{
			{
				Operator: model.BlockContentDataviewFilter_And,
				NestedFilters: []*model.BlockContentDataviewFilter{
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: "relationKey",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String("option2"),
						Format:      model.RelationFormat_status,
					},
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: bundle.RelationKeyName.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String("Object 1"),
						Format:      model.RelationFormat_shorttext,
					},
				},
			},
		}

		// when
		filters, err := NewFilters(Query{Filters: filter}, mockStore, &fastjson.Arena{}, &collate.Buffer{})

		// when
		assert.Nil(t, err)
		assert.NotNil(t, filters.FilterObj)
		assert.NotNil(t, filters.FilterObj.(FiltersAnd))
		assert.Len(t, filters.FilterObj.(FiltersAnd), 5)
	})
	t.Run("deleted filter", func(t *testing.T) {
		// given
		mockStore := &stubSpaceObjectStore{}
		filter := []*model.BlockContentDataviewFilter{
			{
				Operator: model.BlockContentDataviewFilter_And,
				NestedFilters: []*model.BlockContentDataviewFilter{
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: "relationKey",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String("option2"),
						Format:      model.RelationFormat_status,
					},
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: bundle.RelationKeyName.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String("Object 1"),
						Format:      model.RelationFormat_shorttext,
					},
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: bundle.RelationKeyIsDeleted.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.Bool(true),
					},
				},
			},
		}

		// then
		filters, err := NewFilters(Query{Filters: filter}, mockStore, &fastjson.Arena{}, &collate.Buffer{})

		// when
		assert.Nil(t, err)
		assert.NotNil(t, filters.FilterObj)
		assert.NotNil(t, filters.FilterObj.(FiltersAnd))
		assert.Len(t, filters.FilterObj.(FiltersAnd), 5)
	})
	t.Run("archived filter", func(t *testing.T) {
		// given
		mockStore := &stubSpaceObjectStore{}
		filter := []*model.BlockContentDataviewFilter{
			{
				Operator: model.BlockContentDataviewFilter_And,
				NestedFilters: []*model.BlockContentDataviewFilter{
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: "relationKey",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String("option2"),
						Format:      model.RelationFormat_status,
					},
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: bundle.RelationKeyName.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String("Object 1"),
						Format:      model.RelationFormat_shorttext,
					},
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: bundle.RelationKeyIsArchived.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.Bool(true),
					},
				},
			},
		}

		// then
		filters, err := NewFilters(Query{Filters: filter}, mockStore, &fastjson.Arena{}, &collate.Buffer{})

		// when
		assert.Nil(t, err)
		assert.NotNil(t, filters.FilterObj)
		assert.NotNil(t, filters.FilterObj.(FiltersAnd))
		assert.Len(t, filters.FilterObj.(FiltersAnd), 5)
	})
	t.Run("type filter", func(t *testing.T) {
		// given
		mockStore := &stubSpaceObjectStore{}
		filter := []*model.BlockContentDataviewFilter{
			{
				Operator: model.BlockContentDataviewFilter_And,
				NestedFilters: []*model.BlockContentDataviewFilter{
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: "relationKey",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String("option2"),
						Format:      model.RelationFormat_status,
					},
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: bundle.RelationKeyName.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String("Object 1"),
						Format:      model.RelationFormat_shorttext,
					},
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: bundle.RelationKeyType.String(),
						Condition:   model.BlockContentDataviewFilter_In,
						Value:       pbtypes.Float64(float64(model.ObjectType_space)),
					},
				},
			},
		}

		// then
		filters, err := NewFilters(Query{Filters: filter}, mockStore, &fastjson.Arena{}, &collate.Buffer{})

		// when
		assert.Nil(t, err)
		assert.NotNil(t, filters.FilterObj)
		assert.NotNil(t, filters.FilterObj.(FiltersAnd))
		assert.Len(t, filters.FilterObj.(FiltersAnd), 6)
	})
	t.Run("or filter with 3 default", func(t *testing.T) {
		// given
		mockStore := &stubSpaceObjectStore{}
		filter := []*model.BlockContentDataviewFilter{
			{
				Operator: model.BlockContentDataviewFilter_Or,
				NestedFilters: []*model.BlockContentDataviewFilter{
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: "relationKey",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String("option2"),
						Format:      model.RelationFormat_status,
					},
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: bundle.RelationKeyName.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String("Object 1"),
						Format:      model.RelationFormat_shorttext,
					},
				},
			},
		}

		// then
		filters, err := NewFilters(Query{Filters: filter}, mockStore, &fastjson.Arena{}, &collate.Buffer{})

		// when
		assert.Nil(t, err)
		assert.NotNil(t, filters.FilterObj)
		assert.NotNil(t, filters.FilterObj.(FiltersAnd))
		assert.Len(t, filters.FilterObj.(FiltersAnd), 4)
		assert.NotNil(t, filters.FilterObj.(FiltersAnd)[0].(FiltersOr))
		assert.Len(t, filters.FilterObj.(FiltersAnd)[0].(FiltersOr), 2)
	})
}
