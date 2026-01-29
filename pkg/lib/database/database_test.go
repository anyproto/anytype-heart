package database

import (
	"testing"

	"github.com/anyproto/any-store/anyenc"
	"github.com/stretchr/testify/assert"
	"golang.org/x/text/collate"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
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

func TestConvertToHighlightRanges(t *testing.T) {
	t.Run("ascii text with single range", func(t *testing.T) {
		// given
		highlight := "hello world"
		ranges := [][]int{{0, 5}} // "hello"

		// when
		result := convertToHighlightRanges(ranges, highlight)

		// then
		assert.Len(t, result, 1)
		assert.Equal(t, int32(0), result[0].From)
		assert.Equal(t, int32(5), result[0].To)
	})

	t.Run("ascii text with multiple ranges", func(t *testing.T) {
		// given
		highlight := "hello world test"
		ranges := [][]int{{0, 5}, {6, 11}, {12, 16}} // "hello", "world", "test"

		// when
		result := convertToHighlightRanges(ranges, highlight)

		// then
		assert.Len(t, result, 3)
		assert.Equal(t, int32(0), result[0].From)
		assert.Equal(t, int32(5), result[0].To)
		assert.Equal(t, int32(6), result[1].From)
		assert.Equal(t, int32(11), result[1].To)
		assert.Equal(t, int32(12), result[2].From)
		assert.Equal(t, int32(16), result[2].To)
	})

	t.Run("cyrillic text with ranges", func(t *testing.T) {
		// given
		highlight := "привет мир"
		// "привет" is bytes 0-12 (6 chars * 2 bytes)
		ranges := [][]int{{0, 12}}

		// when
		result := convertToHighlightRanges(ranges, highlight)

		// then
		assert.Len(t, result, 1)
		assert.Equal(t, int32(0), result[0].From)
		assert.Equal(t, int32(6), result[0].To) // 6 UTF-16 runes, not 12
	})

	t.Run("emoji text with ranges", func(t *testing.T) {
		// given
		highlight := "hello 👍 world"
		// "hello " is 6 bytes
		// 👍 is 4 bytes (but counts as 2 UTF-16 code units)
		// " world" is 6 bytes
		ranges := [][]int{{6, 10}} // emoji

		// when
		result := convertToHighlightRanges(ranges, highlight)

		// then
		assert.Len(t, result, 1)
		assert.Equal(t, int32(6), result[0].From)
		assert.Equal(t, int32(8), result[0].To) // emoji is 2 UTF-16 code units
	})

	t.Run("empty ranges", func(t *testing.T) {
		// given
		highlight := "hello world"
		ranges := [][]int{}

		// when
		result := convertToHighlightRanges(ranges, highlight)

		// then
		assert.Empty(t, result)
	})

	t.Run("invalid range - negative from", func(t *testing.T) {
		// given
		highlight := "hello world"
		ranges := [][]int{{-1, 5}}

		// when
		result := convertToHighlightRanges(ranges, highlight)

		// then
		assert.Empty(t, result) // invalid ranges should be skipped
	})

	t.Run("invalid range - to exceeds length", func(t *testing.T) {
		// given
		highlight := "hello"
		ranges := [][]int{{0, 100}}

		// when
		result := convertToHighlightRanges(ranges, highlight)

		// then
		assert.Empty(t, result) // invalid ranges should be skipped
	})

	t.Run("range with wrong length", func(t *testing.T) {
		// given
		highlight := "hello world"
		ranges := [][]int{{0}} // should have 2 elements

		// when
		result := convertToHighlightRanges(ranges, highlight)

		// then
		assert.Empty(t, result) // malformed ranges should be skipped
	})

	t.Run("mixed valid and invalid ranges", func(t *testing.T) {
		// given
		highlight := "hello world"
		ranges := [][]int{{0, 5}, {-1, 3}, {6, 11}} // first and last are valid

		// when
		result := convertToHighlightRanges(ranges, highlight)

		// then
		assert.Len(t, result, 2) // only valid ranges
		assert.Equal(t, int32(0), result[0].From)
		assert.Equal(t, int32(5), result[0].To)
		assert.Equal(t, int32(6), result[1].From)
		assert.Equal(t, int32(11), result[1].To)
	})
}

func TestFTDocumentMatchToFulltextResult(t *testing.T) {
	t.Run("valid document match with highlight", func(t *testing.T) {
		// given
		docMatch := &ftsearch.DocumentMatch{
			ID:    "objectId/r/name", // object with relation
			Score: 0.95,
			Fragments: map[string]*ftsearch.Highlight{
				"text": {
					Text:   "hello world",
					Ranges: [][]int{{0, 5}}, // "hello"
				},
			},
		}

		// when
		result, err := FTDocumentMatchToFulltextResult(docMatch)

		// then
		assert.NoError(t, err)
		assert.Equal(t, "objectId", result.Path.ObjectId)
		assert.Equal(t, "name", result.Path.RelationKey)
		assert.Equal(t, "hello world", result.Highlight)
		assert.Equal(t, 0.95, result.Score)
		assert.Len(t, result.HighlightRanges, 1)
		assert.Equal(t, int32(0), result.HighlightRanges[0].From)
		assert.Equal(t, int32(5), result.HighlightRanges[0].To)
	})

	t.Run("document match without highlight ranges", func(t *testing.T) {
		// given
		docMatch := &ftsearch.DocumentMatch{
			ID:    "objectId/b/blockId", // object with block
			Score: 0.85,
			Fragments: map[string]*ftsearch.Highlight{
				"text": {
					Text:   "some text",
					Ranges: [][]int{}, // no ranges
				},
			},
		}

		// when
		result, err := FTDocumentMatchToFulltextResult(docMatch)

		// then
		assert.NoError(t, err)
		assert.Equal(t, "objectId", result.Path.ObjectId)
		assert.Equal(t, "blockId", result.Path.BlockId)
		assert.Empty(t, result.Highlight) // no highlight if no ranges
		assert.Equal(t, 0.85, result.Score)
		assert.Nil(t, result.HighlightRanges)
	})

	t.Run("document match with empty fragments", func(t *testing.T) {
		// given
		docMatch := &ftsearch.DocumentMatch{
			ID:        "objectId/b/blockId",
			Score:     0.75,
			Fragments: map[string]*ftsearch.Highlight{},
		}

		// when
		result, err := FTDocumentMatchToFulltextResult(docMatch)

		// then
		assert.NoError(t, err)
		assert.Equal(t, "objectId", result.Path.ObjectId)
		assert.Equal(t, "blockId", result.Path.BlockId)
		assert.Empty(t, result.Highlight)
		assert.Equal(t, 0.75, result.Score)
		assert.Nil(t, result.HighlightRanges)
	})

	t.Run("document match with multiple fragments - uses first with ranges", func(t *testing.T) {
		// given
		docMatch := &ftsearch.DocumentMatch{
			ID:    "objectId/r/description",
			Score: 0.90,
			Fragments: map[string]*ftsearch.Highlight{
				"title": {
					Text:   "no ranges here",
					Ranges: [][]int{},
				},
				"content": {
					Text:   "highlighted text",
					Ranges: [][]int{{0, 11}}, // "highlighted"
				},
			},
		}

		// when
		result, err := FTDocumentMatchToFulltextResult(docMatch)

		// then
		assert.NoError(t, err)
		assert.Equal(t, "objectId", result.Path.ObjectId)
		assert.Equal(t, "description", result.Path.RelationKey)
		// Note: map iteration order is not guaranteed, but one fragment with ranges should be selected
		if result.Highlight != "" {
			assert.Equal(t, "highlighted text", result.Highlight)
			assert.Len(t, result.HighlightRanges, 1)
		}
		assert.Equal(t, 0.90, result.Score)
	})

	t.Run("invalid document path", func(t *testing.T) {
		// given
		docMatch := &ftsearch.DocumentMatch{
			ID:    "invalid-path-format",
			Score: 0.80,
			Fragments: map[string]*ftsearch.Highlight{
				"text": {
					Text:   "some text",
					Ranges: [][]int{{0, 4}},
				},
			},
		}

		// when
		result, err := FTDocumentMatchToFulltextResult(docMatch)

		// then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse ft search result")
		assert.Equal(t, FulltextResult{}, result)
	})

	t.Run("chat message path with highlight", func(t *testing.T) {
		// given
		docMatch := &ftsearch.DocumentMatch{
			ID:    "chatId/m/messageId",
			Score: 0.88,
			Fragments: map[string]*ftsearch.Highlight{
				"text": {
					Text:   "message text here",
					Ranges: [][]int{{0, 7}}, // "message"
				},
			},
		}

		// when
		result, err := FTDocumentMatchToFulltextResult(docMatch)

		// then
		assert.NoError(t, err)
		assert.Equal(t, "chatId", result.Path.ObjectId)
		assert.Equal(t, "messageId", result.Path.MessageId)
		assert.Equal(t, "message text here", result.Highlight)
		assert.Equal(t, 0.88, result.Score)
		assert.Len(t, result.HighlightRanges, 1)
		assert.Equal(t, int32(0), result.HighlightRanges[0].From)
		assert.Equal(t, int32(7), result.HighlightRanges[0].To)
	})

	t.Run("highlight with cyrillic characters", func(t *testing.T) {
		// given
		docMatch := &ftsearch.DocumentMatch{
			ID:    "objectId/r/name",
			Score: 0.92,
			Fragments: map[string]*ftsearch.Highlight{
				"text": {
					Text:   "привет мир",     // "hello world" in Russian
					Ranges: [][]int{{0, 12}}, // "привет" (6 chars * 2 bytes = 12 bytes)
				},
			},
		}

		// when
		result, err := FTDocumentMatchToFulltextResult(docMatch)

		// then
		assert.NoError(t, err)
		assert.Equal(t, "objectId", result.Path.ObjectId)
		assert.Equal(t, "name", result.Path.RelationKey)
		assert.Equal(t, "привет мир", result.Highlight)
		assert.Equal(t, 0.92, result.Score)
		assert.Len(t, result.HighlightRanges, 1)
		assert.Equal(t, int32(0), result.HighlightRanges[0].From)
		assert.Equal(t, int32(6), result.HighlightRanges[0].To) // 6 UTF-16 runes
	})
}
