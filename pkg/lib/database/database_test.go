package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/valyala/fastjson"

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

func newTestQueryBuilder(t *testing.T) queryBuilder {
	objectStore := NewMockObjectStore(t)
	objectStore.EXPECT().GetRelationFormatByKey(mock.Anything).RunAndReturn(func(key string) (model.RelationFormat, error) {
		rel, err := bundle.GetRelation(domain.RelationKey(key))
		if err != nil {
			return 0, nil
		}
		return rel.Format, nil
	}).Maybe()
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
