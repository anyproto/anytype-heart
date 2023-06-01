package database

import (
	"github.com/anyproto/anytype-heart/pkg/lib/database/filter"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
	"testing"
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

func testIncludeTimeWhenSingleDateSort(t *testing.T) {
	//given
	sorts := givenSingleDateSort()

	//when
	order := extractOrder(sorts, nil)

	//then
	assertIncludeTime(t, order)
}

func testDoNotIncludeTimeWhenNotSingleSort(t *testing.T) {
	//given
	sorts := givenNotSingleDateSort()

	//when
	order := extractOrder(sorts, nil)

	//then
	assertNotIncludeTime(t, order)
}

func testIncludeTimeWhenSortContainsIncludeTime(t *testing.T) {
	//given
	sorts := givenSingleIncludeTime()

	//when
	order := extractOrder(sorts, nil)

	//then
	assertIncludeTime(t, order)
}

func testDoNotIncludeTimeWhenSingleNotDateSort(t *testing.T) {
	//given
	sorts := givenSingleNotDateSort()

	//when
	order := extractOrder(sorts, nil)

	//then
	assertNotIncludeTime(t, order)
}

func assertIncludeTime(t *testing.T, order filter.SetOrder) {
	assert.IsType(t, order[0], &filter.KeyOrder{})
	assert.Equal(t, order[0].(*filter.KeyOrder).IncludeTime, true)
}

func assertNotIncludeTime(t *testing.T, order filter.SetOrder) {
	assert.IsType(t, order[0], &filter.KeyOrder{})
	assert.Equal(t, order[0].(*filter.KeyOrder).IncludeTime, false)
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
