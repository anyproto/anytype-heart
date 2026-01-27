package spaceindex

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/anyproto/any-store/anyenc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
	"golang.org/x/text/collate"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func removeScoreFromRecords(records []database.Record) []database.Record {
	for i := range records {
		records[i].Details.Delete("_score")
	}
	return records
}

func removeMetaFromRecords(records []database.Record) []database.Record {
	for i := range records {
		records[i].Meta = model.SearchMeta{}
	}
	return records
}

func assertRecordsEqual(t *testing.T, want []TestObject, got []database.Record) {
	wantRaw := make([]database.Record, 0, len(want))
	for _, w := range want {
		wantRaw = append(wantRaw, database.Record{Details: makeDetails(w)})
	}
	assert.Equal(t, wantRaw, got)
}

func assertRecordsMatch(t *testing.T, want []TestObject, got []database.Record) {
	wantRaw := make([]database.Record, 0, len(want))

	for _, w := range want {
		wantRaw = append(wantRaw, database.Record{Details: makeDetails(w)})
	}
	got = removeScoreFromRecords(got)
	got = removeMetaFromRecords(got)

	assert.ElementsMatch(t, wantRaw, got)
}

func TestQuery(t *testing.T) {
	t.Run("no filters", func(t *testing.T) {
		s := NewStoreFixture(t)
		obj1 := TestObject{
			bundle.RelationKeyId:   domain.String("id1"),
			bundle.RelationKeyName: domain.String("name1"),
		}
		obj2 := TestObject{
			bundle.RelationKeyId:   domain.String("id2"),
			bundle.RelationKeyName: domain.String("name2"),
		}
		obj3 := TestObject{
			bundle.RelationKeyId:   domain.String("id3"),
			bundle.RelationKeyName: domain.String("name3"),
		}
		s.AddObjects(t, []TestObject{obj1, obj2, obj3})

		recs, err := s.Query(database.Query{})
		require.NoError(t, err)

		assertRecordsEqual(t, []TestObject{
			obj1,
			obj2,
			obj3,
		}, recs)
	})

	t.Run("with filter", func(t *testing.T) {
		s := NewStoreFixture(t)
		obj1 := TestObject{
			bundle.RelationKeyId:   domain.String("id1"),
			bundle.RelationKeyName: domain.String("name1"),
		}
		obj2 := TestObject{
			bundle.RelationKeyId:   domain.String("id2"),
			bundle.RelationKeyName: domain.String("name2"),
		}
		obj3 := TestObject{
			bundle.RelationKeyId:   domain.String("id3"),
			bundle.RelationKeyName: domain.String("name3"),
		}
		s.AddObjects(t, []TestObject{obj1, obj2, obj3})

		recs, err := s.Query(database.Query{
			Filters: []database.FilterRequest{
				{
					RelationKey: bundle.RelationKeyName,
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       domain.String("name2"),
				},
			},
		})
		require.NoError(t, err)

		assertRecordsEqual(t, []TestObject{
			obj2,
		}, recs)
	})

	t.Run("with multiple filters", func(t *testing.T) {
		s := NewStoreFixture(t)
		obj1 := TestObject{
			bundle.RelationKeyId:   domain.String("id1"),
			bundle.RelationKeyName: domain.String("name"),
		}
		obj2 := TestObject{
			bundle.RelationKeyId:          domain.String("id2"),
			bundle.RelationKeyName:        domain.String("name"),
			bundle.RelationKeyDescription: domain.String("description"),
		}
		obj3 := TestObject{
			bundle.RelationKeyId:          domain.String("id3"),
			bundle.RelationKeyName:        domain.String("name"),
			bundle.RelationKeyDescription: domain.String("description"),
		}
		s.AddObjects(t, []TestObject{obj1, obj2, obj3})

		recs, err := s.Query(database.Query{
			Filters: []database.FilterRequest{
				{
					RelationKey: bundle.RelationKeyName,
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       domain.String("name"),
				},
				{
					RelationKey: bundle.RelationKeyDescription,
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       domain.String("description"),
				},
			},
		})
		require.NoError(t, err)

		assertRecordsEqual(t, []TestObject{
			obj2,
			obj3,
		}, recs)
	})

	t.Run("full text search", func(t *testing.T) {
		s := NewStoreFixture(t)
		obj1 := TestObject{
			bundle.RelationKeyId:          domain.String("id1"),
			bundle.RelationKeyName:        domain.String("name"),
			bundle.RelationKeyDescription: domain.String("foo"),
		}
		obj2 := TestObject{
			bundle.RelationKeyId:          domain.String("id2"),
			bundle.RelationKeyName:        domain.String("some important note"),
			bundle.RelationKeyDescription: domain.String("foo"),
		}
		obj3 := TestObject{
			bundle.RelationKeyId:          domain.String("id3"),
			bundle.RelationKeyName:        domain.String(""),
			bundle.RelationKeyDescription: domain.String("bar"),
		}
		s.AddObjects(t, []TestObject{obj1, obj2, obj3})

		err := s.fts.Index(ftsearch.SearchDoc{
			Id:    "id1/r/name",
			Title: "myname1",
		})
		require.NoError(t, err)

		err = s.fts.Index(ftsearch.SearchDoc{
			Id:    "id1/r/pluralName",
			Title: "mynames",
		})
		require.NoError(t, err)

		err = s.fts.Index(ftsearch.SearchDoc{
			Id:    "id2/b/321",
			Title: "some important note",
		})
		require.NoError(t, err)

		err = s.fts.Index(ftsearch.SearchDoc{
			Id:    "id3/b/435",
			Title: "",
			Text:  "very important text",
		})
		require.NoError(t, err)

		t.Run("just full-text", func(t *testing.T) {
			recs, err := s.Query(database.Query{
				TextQuery: "important",
			})
			require.NoError(t, err)

			// Full-text engine has its own ordering, so just don't rely on it here and check only the content.
			assertRecordsMatch(t, []TestObject{
				obj2,
				obj3,
			}, recs)
		})

		t.Run("fulltext by relation", func(t *testing.T) {
			recs, err := s.Query(database.Query{
				TextQuery: "myname1",
			})
			require.NoError(t, err)

			// Full-text engine has its own ordering, so just don't rely on it here and check only the content.
			assertRecordsMatch(t, []TestObject{
				obj1,
			}, recs)
		})

		t.Run("fulltext by plural name relation", func(t *testing.T) {
			recs, err := s.Query(database.Query{
				TextQuery: "mynames",
			})
			require.NoError(t, err)

			assert.Equal(t, "pluralName", recs[0].Meta.RelationKey)
			// Full-text engine has its own ordering, so just don't rely on it here and check only the content.
			assertRecordsMatch(t, []TestObject{
				obj1,
			}, recs)
		})

		t.Run("full-text and filter", func(t *testing.T) {
			recs, err := s.Query(database.Query{
				TextQuery: "important",
				Filters: []database.FilterRequest{
					{
						RelationKey: bundle.RelationKeyDescription,
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.String("foo"),
					},
				},
			})
			require.NoError(t, err)

			// Full-text engine has its own ordering, so just don't rely on it here and check only the content.
			assertRecordsMatch(t, []TestObject{
				obj2,
			}, recs)
		})
	})

	t.Run("full text meta", func(t *testing.T) {
		s := NewStoreFixture(t)
		obj1 := TestObject{
			bundle.RelationKeyId:          domain.String("id1"),
			domain.RelationKey("bsonid1"): domain.String("relid1"),
			bundle.RelationKeyDescription: domain.String("this is the first object description"),
		}
		obj2 := TestObject{
			bundle.RelationKeyId:          domain.String("id2"),
			bundle.RelationKeyType:        domain.String("typeid1"),
			bundle.RelationKeyDescription: domain.String("this is the second object description"),
		}

		obj3 := TestObject{
			bundle.RelationKeyId:   domain.String("id3"),
			bundle.RelationKeyType: domain.String("typeid1"),
		}

		relObj := TestObject{
			bundle.RelationKeyId:             domain.String("relid1"),
			bundle.RelationKeyRelationKey:    domain.String("bsonid1"),
			bundle.RelationKeyName:           domain.String("relname"),
			bundle.RelationKeyDescription:    domain.String("this is a relation's description"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
		}

		relObjDeleted := TestObject{
			bundle.RelationKeyId:             domain.String("relid2"),
			bundle.RelationKeyRelationKey:    domain.String("bsonid1"),
			bundle.RelationKeyName:           domain.String("deletedtag"),
			bundle.RelationKeyIsDeleted:      domain.Bool(true),
			bundle.RelationKeyDescription:    domain.String("this is a deleted relation's description"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
		}

		relObjArchived := TestObject{
			bundle.RelationKeyId:             domain.String("relid3"),
			bundle.RelationKeyRelationKey:    domain.String("bsonid1"),
			bundle.RelationKeyName:           domain.String("archived"),
			bundle.RelationKeyIsDeleted:      domain.Bool(true),
			bundle.RelationKeyDescription:    domain.String("this is a archived relation's description"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
		}

		typeObj := TestObject{
			bundle.RelationKeyId:             domain.String("typeid1"),
			bundle.RelationKeyName:           domain.String("typename"),
			bundle.RelationKeyDescription:    domain.String("this is a type's description"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		}

		s.AddObjects(t, []TestObject{obj1, obj2, obj3, relObj, relObjDeleted, relObjArchived, typeObj})
		err := s.fts.Index(ftsearch.SearchDoc{
			Id:   "id1/r/description",
			Text: obj1[bundle.RelationKeyDescription].String(),
		})
		require.NoError(t, err)

		err = s.fts.Index(ftsearch.SearchDoc{
			Id:   "id2/r/description",
			Text: obj2[bundle.RelationKeyDescription].String(),
		})
		require.NoError(t, err)

		err = s.fts.Index(ftsearch.SearchDoc{
			Id:   "relid1/r/description",
			Text: relObj[bundle.RelationKeyDescription].String(),
		})
		require.NoError(t, err)
		err = s.fts.Index(ftsearch.SearchDoc{
			Id:    "relid1/r/name",
			Title: relObj[bundle.RelationKeyName].String(),
		})
		require.NoError(t, err)

		err = s.fts.Index(ftsearch.SearchDoc{
			Id:   "relid2/r/description",
			Text: relObjDeleted[bundle.RelationKeyDescription].String(),
		})
		require.NoError(t, err)
		err = s.fts.Index(ftsearch.SearchDoc{
			Id:    "relid2/r/name",
			Title: relObjDeleted[bundle.RelationKeyName].String(),
		})
		require.NoError(t, err)

		err = s.fts.Index(ftsearch.SearchDoc{
			Id:   "relid3/r/description",
			Text: relObjArchived[bundle.RelationKeyDescription].String(),
		})
		require.NoError(t, err)
		err = s.fts.Index(ftsearch.SearchDoc{
			Id:    "relid3/r/name",
			Title: relObjArchived[bundle.RelationKeyName].String(),
		})
		require.NoError(t, err)

		err = s.fts.Index(ftsearch.SearchDoc{
			Id:    "typeid1/r/name",
			Title: typeObj[bundle.RelationKeyName].String(),
		})
		require.NoError(t, err)

		err = s.fts.Index(ftsearch.SearchDoc{
			Id:   "id1/b/block1",
			Text: "this is a beautiful block block",
		})
		require.NoError(t, err)

		err = s.fts.Index(ftsearch.SearchDoc{
			Id:   "id1/b/block2",
			Text: "this is a clever block as it has a lot of text. On the other hand, this block is not very cozy. But because it has multiple mention of word 'block' it will have a higher score.",
		})
		require.NoError(t, err)

		err = s.fts.Index(ftsearch.SearchDoc{
			Id:   "id2/b/321",
			Text: "this is a sage block",
		})
		require.NoError(t, err)

		err = s.fts.Index(ftsearch.SearchDoc{
			Id: "id3/b/block1",
			Text: "Why did the dog sit in the shade? Because it didn’t want to be a hot dog! And what do you call a dog that can do magic? A labracadabrador! " +
				"Just remember, if your dog is barking at the back door and your cat is yowling at the front door, you might just live in a pet-operated zoo!",
		})
		require.NoError(t, err)

		err = s.fts.Index(ftsearch.SearchDoc{
			Id: "id3/b/block2",
			Text: "Найближча до нас зоря, в якої, на відміну від усіх інших зірок, " +
				"можна вести спостереження за диском і за допомогою телескопа вивчати на ньому дрібні деталі, розміром до кількох сотень кілометрів. " +
				"Це типова зоря, тому її вивчення допомагає зрозуміти природу зірок загалом. " +
				"За зоряною класифікацією Сонце має спектральний клас G2V. Водночас Сонце доволі часто класифікують як жовтий карлик.",
		})
		require.NoError(t, err)

		t.Run("full-text relation description", func(t *testing.T) {
			recs, err := s.Query(database.Query{
				TextQuery: "first object",
			})
			require.NoError(t, err)
			removeScoreFromRecords(recs)
			assert.ElementsMatch(t, []database.Record{
				{
					Details: makeDetails(obj1),
					Meta: model.SearchMeta{
						Highlight: "this is the first object description",
						HighlightRanges: []*model.Range{{
							From: 12,
							To:   17,
						}, {
							From: 18,
							To:   24,
						}},
						RelationKey: "description",
					},
				}, {
					Details: makeDetails(obj2),
					Meta: model.SearchMeta{
						Highlight: "this is the second object description",
						HighlightRanges: []*model.Range{{
							From: 19,
							To:   25,
						}},
						RelationKey: "description",
					},
				}}, recs)
		})

		t.Run("full-text block single match", func(t *testing.T) {
			recs, err := s.Query(database.Query{
				TextQuery: "sage",
			})
			require.NoError(t, err)
			removeScoreFromRecords(recs)
			assert.ElementsMatch(t, []database.Record{
				{
					Details: makeDetails(obj2),
					Meta: model.SearchMeta{
						Highlight: "this is a sage block",
						HighlightRanges: []*model.Range{{
							From: 10,
							To:   14,
						}},
						BlockId: "321",
					},
				}}, recs)
		})

		t.Run("full-text block multi match", func(t *testing.T) {
			recs, err := s.Query(database.Query{
				TextQuery: "block",
				Sorts: []database.SortRequest{
					{
						RelationKey: bundle.RelationKeyId,
						Type:        model.BlockContentDataviewSort_Asc,
					},
				},
			})
			require.NoError(t, err)
			removeScoreFromRecords(recs)
			assert.ElementsMatch(t, []database.Record{
				{
					Details: makeDetails(obj1),
					Meta: model.SearchMeta{
						Highlight: "this is a beautiful block block",
						HighlightRanges: []*model.Range{
							{
								From: 20,
								To:   25,
							},
							{
								From: 26,
								To:   31,
							},
						},
						BlockId: "block1",
					},
				},
				// only one result per object
				{
					Details: makeDetails(obj2),
					Meta: model.SearchMeta{
						Highlight: "this is a sage block",
						HighlightRanges: []*model.Range{
							{
								From: 15,
								To:   20,
							},
						},
						BlockId: "321",
					},
				},
			}, recs)
		})

		t.Run("full-text block single match truncated", func(t *testing.T) {
			recs, err := s.Query(database.Query{
				TextQuery: "dog",
			})
			require.NoError(t, err)
			removeScoreFromRecords(recs)
			assert.ElementsMatch(t, []database.Record{
				{
					Details: makeDetails(obj3),
					Meta: model.SearchMeta{
						Highlight: "Why did the dog sit in the shade? Because it didn’t want to be a hot dog! And what do you call a dog that can do magic? A labracadabrador! Just",
						HighlightRanges: []*model.Range{
							{
								From: 12,
								To:   15,
							},
							{
								From: 69,
								To:   72,
							},
							{
								From: 97,
								To:   100,
							},
						},
						BlockId: "block1",
					},
				},
			}, recs)
		})

		t.Run("full-text block single match truncated cyrillic", func(t *testing.T) {
			recs, err := s.Query(database.Query{
				TextQuery: "Сонце",
			})
			require.NoError(t, err)
			removeScoreFromRecords(recs)
			assert.ElementsMatch(t, []database.Record{
				{
					Details: makeDetails(obj3),
					Meta: model.SearchMeta{
						Highlight: "зрозуміти природу зірок загалом. За зоряною класифікацією Сонце має спектральний",
						HighlightRanges: []*model.Range{
							{
								From: 58,
								To:   63,
							},
						},
						BlockId: "block2",
					},
				},
			}, recs)
		})

		t.Run("full-text by tag", func(t *testing.T) {
			recs, err := s.Query(database.Query{
				TextQuery: "relname",
				Filters: []database.FilterRequest{
					{
						Operator:    0,
						RelationKey: bundle.RelationKeyResolvedLayout,
						Condition:   model.BlockContentDataviewFilter_NotIn,
						Value:       domain.Int64List([]int64{int64(model.ObjectType_relationOption)}),
					},
				},
			})
			require.NoError(t, err)
			removeScoreFromRecords(recs)
			assert.ElementsMatch(t, []database.Record{
				{
					Details: makeDetails(obj1),
					Meta: model.SearchMeta{
						RelationKey:     "bsonid1",
						RelationDetails: makeDetails(relObj).CopyOnlyKeys(bundle.RelationKeyResolvedLayout, bundle.RelationKeyId, bundle.RelationKeyName).ToProto(),
					},
				}}, recs)
		})

		t.Run("full-text by deleted tag", func(t *testing.T) {
			recs, err := s.Query(database.Query{
				TextQuery: "deleted",
				Filters: []database.FilterRequest{
					{
						Operator:    0,
						RelationKey: "layout",
						Condition:   model.BlockContentDataviewFilter_NotIn,
						Value:       domain.Int64List([]int64{int64(model.ObjectType_relationOption)}),
					},
				},
			})
			require.NoError(t, err)
			removeScoreFromRecords(recs)
			assert.Len(t, recs, 0)
		})

		t.Run("full-text by archived tag", func(t *testing.T) {
			recs, err := s.Query(database.Query{
				TextQuery: "archived",
				Filters: []database.FilterRequest{
					{
						Operator:    0,
						RelationKey: "layout",
						Condition:   model.BlockContentDataviewFilter_NotIn,
						Value:       domain.Int64List([]int64{int64(model.ObjectType_relationOption)}),
					},
				},
			})
			require.NoError(t, err)
			removeScoreFromRecords(recs)
			assert.Len(t, recs, 0)
		})

		t.Run("full-text by type", func(t *testing.T) {
			recs, err := s.Query(database.Query{
				TextQuery: "typename",
				Filters: []database.FilterRequest{
					{
						Operator:    0,
						RelationKey: bundle.RelationKeyResolvedLayout,
						Condition:   model.BlockContentDataviewFilter_NotIn,
						Value:       domain.Int64List([]int64{int64(model.ObjectType_objectType)}),
					},
				},
			})
			require.NoError(t, err)
			removeScoreFromRecords(recs)
			assert.Equal(t, []database.Record{
				{
					Details: makeDetails(obj2),
					Meta: model.SearchMeta{
						RelationKey:     "type",
						RelationDetails: makeDetails(typeObj).CopyOnlyKeys(bundle.RelationKeyResolvedLayout, bundle.RelationKeyId, bundle.RelationKeyName).ToProto(),
					},
				},
				{
					Details: makeDetails(obj3),
					Meta: model.SearchMeta{
						RelationKey:     "type",
						RelationDetails: makeDetails(typeObj).CopyOnlyKeys(bundle.RelationKeyResolvedLayout, bundle.RelationKeyId, bundle.RelationKeyName).ToProto(),
					},
				},
			}, recs)
		})
	})

	t.Run("with ascending order and filter", func(t *testing.T) {
		s := NewStoreFixture(t)
		obj1 := makeObjectWithName("id1", "dfg")
		obj2 := makeObjectWithName("id2", "abc")
		obj3 := makeObjectWithName("id3", "012")
		obj4 := makeObjectWithName("id4", "ignore")
		s.AddObjects(t, []TestObject{obj1, obj2, obj3, obj4})

		recs, err := s.Query(database.Query{
			Filters: []database.FilterRequest{
				{
					RelationKey: bundle.RelationKeyName,
					Condition:   model.BlockContentDataviewFilter_NotEqual,
					Value:       domain.String("ignore"),
				},
			},
			Sorts: []database.SortRequest{
				{
					RelationKey: bundle.RelationKeyName,
					Type:        model.BlockContentDataviewSort_Asc,
				},
			},
		})
		require.NoError(t, err)

		assertRecordsEqual(t, []TestObject{
			obj3,
			obj2,
			obj1,
		}, recs)
	})

	t.Run("with descending order", func(t *testing.T) {
		s := NewStoreFixture(t)
		obj1 := makeObjectWithName("id1", "dfg")
		obj2 := makeObjectWithName("id2", "abc")
		obj3 := makeObjectWithName("id3", "012")
		s.AddObjects(t, []TestObject{obj1, obj2, obj3})

		recs, err := s.Query(database.Query{
			Sorts: []database.SortRequest{
				{
					RelationKey: bundle.RelationKeyName,
					Type:        model.BlockContentDataviewSort_Desc,
				},
			},
		})
		require.NoError(t, err)

		assertRecordsEqual(t, []TestObject{
			obj1,
			obj2,
			obj3,
		}, recs)
	})

	t.Run("with multiple orders", func(t *testing.T) {
		s := NewStoreFixture(t)
		obj1 := makeObjectWithNameAndDescription("id1", "dfg", "foo")
		obj2 := makeObjectWithNameAndDescription("id2", "abc", "foo")
		obj3 := makeObjectWithNameAndDescription("id3", "012", "bar")
		obj4 := makeObjectWithNameAndDescription("id4", "bcd", "bar")
		s.AddObjects(t, []TestObject{obj1, obj2, obj3, obj4})

		recs, err := s.Query(database.Query{
			Sorts: []database.SortRequest{
				{
					RelationKey: bundle.RelationKeyDescription,
					Type:        model.BlockContentDataviewSort_Desc,
				},
				{
					RelationKey: bundle.RelationKeyName,
					Type:        model.BlockContentDataviewSort_Asc,
				},
			},
		})
		require.NoError(t, err)

		assertRecordsEqual(t, []TestObject{
			obj2,
			obj1,
			obj3,
			obj4,
		}, recs)
	})

	t.Run("with offset", func(t *testing.T) {
		// Given
		s := NewStoreFixture(t)
		var objects []TestObject
		for i := 0; i < 100; i++ {
			objects = append(objects, generateObjectWithRandomID())
		}
		s.AddObjects(t, objects)

		// When
		recs, err := s.Query(database.Query{
			Sorts: []database.SortRequest{
				{
					RelationKey: bundle.RelationKeyName,
					Type:        model.BlockContentDataviewSort_Desc,
				},
			},
			Offset: 10,
		})
		require.NoError(t, err)

		// Then
		want := slices.Clone(objects)
		sort.Slice(want, func(i, j int) bool {
			a := makeDetails(want[i])
			b := makeDetails(want[j])
			idA := a.GetString(bundle.RelationKeyName)
			idB := b.GetString(bundle.RelationKeyName)
			// Desc order
			return idA > idB
		})
		assertRecordsEqual(t, want[10:], recs)
	})

	t.Run("with limit", func(t *testing.T) {
		// Given
		s := NewStoreFixture(t)
		var objects []TestObject
		for i := 0; i < 100; i++ {
			objects = append(objects, generateObjectWithRandomID())
		}
		s.AddObjects(t, objects)

		// When
		recs, err := s.Query(database.Query{
			Sorts: []database.SortRequest{
				{
					RelationKey: bundle.RelationKeyName,
					Type:        model.BlockContentDataviewSort_Asc,
				},
			},
			Limit: 15,
		})
		require.NoError(t, err)

		// Then
		want := slices.Clone(objects)
		sort.Slice(want, func(i, j int) bool {
			a := makeDetails(want[i])
			b := makeDetails(want[j])
			idA := a.GetString(bundle.RelationKeyName)
			idB := b.GetString(bundle.RelationKeyName)
			return idA < idB
		})
		assertRecordsEqual(t, want[:15], recs)
	})

	t.Run("with limit and offset", func(t *testing.T) {
		// Given
		s := NewStoreFixture(t)
		var objects []TestObject
		for i := 0; i < 100; i++ {
			objects = append(objects, generateObjectWithRandomID())
		}
		s.AddObjects(t, objects)

		// When
		limit := 15
		offset := 20
		recs, err := s.Query(database.Query{
			Sorts: []database.SortRequest{
				{
					RelationKey: bundle.RelationKeyName,
					Type:        model.BlockContentDataviewSort_Asc,
				},
			},
			Limit:  limit,
			Offset: offset,
		})
		require.NoError(t, err)

		// Then
		want := slices.Clone(objects)
		sort.Slice(want, func(i, j int) bool {
			a := makeDetails(want[i])
			b := makeDetails(want[j])
			idA := a.GetString(bundle.RelationKeyName)
			idB := b.GetString(bundle.RelationKeyName)
			return idA < idB
		})
		assertRecordsEqual(t, want[offset:offset+limit], recs)
	})

	t.Run("with filter, limit and offset", func(t *testing.T) {
		// Given
		s := NewStoreFixture(t)
		var objects []TestObject
		var filteredObjects []TestObject
		for i := 0; i < 100; i++ {
			if i%2 == 0 {
				objects = append(objects, generateObjectWithRandomID())
			} else {
				obj := makeObjectWithName(fmt.Sprintf("id%02d", i), "this name")
				filteredObjects = append(filteredObjects, obj)
				objects = append(objects, obj)
			}

		}
		s.AddObjects(t, objects)

		// When
		limit := 60
		offset := 20
		recs, err := s.Query(database.Query{
			Filters: []database.FilterRequest{
				{
					RelationKey: bundle.RelationKeyName,
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       domain.String("this name"),
				},
			},
			Sorts: []database.SortRequest{
				{
					RelationKey: bundle.RelationKeyId,
					Type:        model.BlockContentDataviewSort_Asc,
				},
			},
			Limit:  limit,
			Offset: offset,
		})
		require.NoError(t, err)

		// Then
		// Limit is much bigger than the number of filtered objects, so we should get all of them, considering offset
		assertRecordsEqual(t, filteredObjects[offset:], recs)
	})
}

func TestQueryObjectIds(t *testing.T) {
	t.Run("no filters", func(t *testing.T) {
		s := NewStoreFixture(t)
		obj1 := makeObjectWithName("id1", "name1")
		obj2 := makeObjectWithName("id2", "name2")
		obj3 := makeObjectWithName("id3", "name3")
		s.AddObjects(t, []TestObject{obj1, obj2, obj3})

		ids, _, err := s.QueryObjectIds(database.Query{})
		require.NoError(t, err)
		assert.Equal(t, []string{"id1", "id2", "id3"}, ids)
	})

	t.Run("with basic filter and smartblock types filter", func(t *testing.T) {
		// Given
		s := NewStoreFixture(t)

		obj1 := makeObjectWithNameAndDescription("id1", "file1", "foo")
		obj2 := makeObjectWithNameAndDescription("id2", "page2", "foo")
		obj3 := makeObjectWithNameAndDescription("id3", "page3", "bar")

		s.AddObjects(t, []TestObject{obj1, obj2, obj3})

		// When
		ids, _, err := s.QueryObjectIds(database.Query{
			Filters: []database.FilterRequest{
				{
					RelationKey: bundle.RelationKeyDescription,
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       domain.String("foo"),
				},
			},
		})
		require.NoError(t, err)

		// Then
		assert.Equal(t, []string{"id1", "id2"}, ids)
	})
}

func TestQueryRaw(t *testing.T) {
	arena := &anyenc.Arena{}

	t.Run("with nil filter expect error", func(t *testing.T) {
		s := NewStoreFixture(t)

		_, err := s.QueryRaw(nil, 0, 0)
		require.Error(t, err)
	})

	t.Run("with uninitialized filter expect error", func(t *testing.T) {
		s := NewStoreFixture(t)
		obj1 := makeObjectWithName("id1", "name1")
		s.AddObjects(t, []TestObject{obj1})

		_, err := s.QueryRaw(&database.Filters{}, 0, 0)
		require.Error(t, err)
	})

	t.Run("no filters", func(t *testing.T) {
		s := NewStoreFixture(t)
		obj1 := makeObjectWithName("id1", "name1")
		obj2 := makeObjectWithName("id2", "name2")
		obj3 := makeObjectWithName("id3", "name3")
		s.AddObjects(t, []TestObject{obj1, obj2, obj3})

		flt, err := database.NewFilters(database.Query{}, s, arena, &collate.Buffer{})
		require.NoError(t, err)

		recs, err := s.QueryRaw(flt, 0, 0)
		require.NoError(t, err)
		assertRecordsEqual(t, []TestObject{obj1, obj2, obj3}, recs)
	})

	t.Run("with filter", func(t *testing.T) {
		s := NewStoreFixture(t)
		obj1 := makeObjectWithNameAndDescription("id1", "name1", "foo")
		obj2 := makeObjectWithNameAndDescription("id2", "name2", "bar")
		obj3 := makeObjectWithNameAndDescription("id3", "name3", "foo")
		s.AddObjects(t, []TestObject{obj1, obj2, obj3})

		flt, err := database.NewFilters(database.Query{
			Filters: []database.FilterRequest{
				{
					RelationKey: bundle.RelationKeyDescription,
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       domain.String("foo"),
				},
			},
		}, s, arena, &collate.Buffer{})
		require.NoError(t, err)

		recs, err := s.QueryRaw(flt, 0, 0)
		require.NoError(t, err)
		assertRecordsEqual(t, []TestObject{obj1, obj3}, recs)
	})

	t.Run("with nested filter", func(t *testing.T) {
		t.Run("equal", func(t *testing.T) {
			s := NewStoreFixture(t)
			obj1 := TestObject{
				bundle.RelationKeyId:   domain.String("id1"),
				bundle.RelationKeyType: domain.String("type1"),
			}
			type1 := TestObject{
				bundle.RelationKeyId:        domain.String("type1"),
				bundle.RelationKeyType:      domain.String("objectType"),
				bundle.RelationKeyUniqueKey: domain.String("ot-note"),
			}

			s.AddObjects(t, []TestObject{obj1, type1})

			flt, err := database.NewFilters(database.Query{
				Filters: []database.FilterRequest{
					{
						RelationKey: "type.uniqueKey",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.String("ot-note"),
					},
				},
			}, s, arena, &collate.Buffer{})
			require.NoError(t, err)

			recs, err := s.QueryRaw(flt, 0, 0)
			require.NoError(t, err)
			assertRecordsEqual(t, []TestObject{obj1}, recs)
		})
		t.Run("not equal", func(t *testing.T) {
			s := NewStoreFixture(t)
			obj1 := TestObject{
				bundle.RelationKeyId:             domain.String("id1"),
				bundle.RelationKeyType:           domain.String("type1"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			}
			obj2 := TestObject{
				bundle.RelationKeyId:             domain.String("id2"),
				bundle.RelationKeyType:           domain.String("type2"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			}
			type1 := TestObject{
				bundle.RelationKeyId:             domain.String("type1"),
				bundle.RelationKeyType:           domain.String("objectType"),
				bundle.RelationKeyUniqueKey:      domain.String("ot-template"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
			}
			type2 := TestObject{
				bundle.RelationKeyId:             domain.String("type2"),
				bundle.RelationKeyType:           domain.String("objectType"),
				bundle.RelationKeyUniqueKey:      domain.String("ot-page"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
			}

			s.AddObjects(t, []TestObject{obj1, obj2, type1, type2})

			flt, err := database.NewFilters(database.Query{
				Filters: []database.FilterRequest{
					{
						RelationKey: "type.uniqueKey",
						Condition:   model.BlockContentDataviewFilter_NotEqual,
						Value:       domain.String("ot-template"),
					},
					{
						RelationKey: bundle.RelationKeyResolvedLayout,
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.Int64(int64(model.ObjectType_basic)),
					},
				},
			}, s, arena, &collate.Buffer{})
			require.NoError(t, err)

			recs, err := s.QueryRaw(flt, 0, 0)
			require.NoError(t, err)
			assertRecordsEqual(t, []TestObject{obj2}, recs)
		})

	})
}

type dummySourceService struct {
	objectToReturn TestObject
}

func (s dummySourceService) DetailsFromIdBasedSource(id domain.FullID) (*domain.Details, error) {
	return makeDetails(s.objectToReturn), nil
}

func TestQueryById(t *testing.T) {
	t.Run("no ids", func(t *testing.T) {
		s := NewStoreFixture(t)

		recs, err := s.QueryByIds(nil)
		require.NoError(t, err)
		assert.Empty(t, recs)
	})

	t.Run("just ordinary objects", func(t *testing.T) {
		s := NewStoreFixture(t)
		obj1 := makeObjectWithName("id1", "name1")
		obj2 := makeObjectWithName("id2", "name2")
		obj3 := makeObjectWithName("id3", "name3")
		s.AddObjects(t, []TestObject{obj1, obj2, obj3})

		recs, err := s.QueryByIds([]string{"id1", "id3"})
		require.NoError(t, err)
		assertRecordsEqual(t, []TestObject{obj1, obj3}, recs)

		t.Run("reverse order", func(t *testing.T) {
			recs, err := s.QueryByIds([]string{"id3", "id1"})
			require.NoError(t, err)
			assertRecordsEqual(t, []TestObject{obj3, obj1}, recs)
		})
	})

	t.Run("some objects are not indexable and derive details from its source", func(t *testing.T) {
		s := NewStoreFixture(t)

		obj1 := makeObjectWithName("id1", "name2")

		// obj4 is not indexable, so don't try to add it to store
		dateID := addr.DatePrefix + "01_02_2005"
		obj2 := makeObjectWithName(dateID, "i'm special")

		s.AddObjects(t, []TestObject{obj1})

		s.sourceService = dummySourceService{objectToReturn: obj2}

		recs, err := s.QueryByIds([]string{"id1", dateID})
		require.NoError(t, err)
		assertRecordsEqual(t, []TestObject{obj1, obj2}, recs)
	})
}

func TestQueryByIdAndSubscribeForChanges(t *testing.T) {
	s := NewStoreFixture(t)
	obj1 := makeObjectWithName("id1", "name1")
	obj2 := makeObjectWithName("id2", "name2")
	obj3 := makeObjectWithName("id3", "name3")
	s.AddObjects(t, []TestObject{obj1, obj2, obj3})

	recordsCh := make(chan *domain.Details)
	sub := database.NewSubscription(nil, recordsCh)

	recs, closeSub, err := s.QueryByIdsAndSubscribeForChanges([]string{"id1", "id3"}, sub)
	require.NoError(t, err)
	defer closeSub()

	assertRecordsEqual(t, []TestObject{obj1, obj3}, recs)

	t.Run("update details called, but there are no changes", func(t *testing.T) {
		err = s.UpdateObjectDetails(context.Background(), "id1", makeDetails(obj1))
		require.NoError(t, err)

		select {
		case <-recordsCh:
			require.Fail(t, "unexpected record")
		case <-time.After(10 * time.Millisecond):
		}
	})

	t.Run("update details order", func(t *testing.T) {
		for i := 1; i <= 1000; i++ {
			err = s.UpdateObjectDetails(context.Background(), "id1", makeDetails(makeObjectWithName("id1", fmt.Sprintf("%d", i))))
			require.NoError(t, err)
		}

		prev := 0
		for {
			select {
			case rec := <-recordsCh:
				name := rec.GetString(bundle.RelationKeyName)
				num, err := strconv.Atoi(name)
				require.NoError(t, err)
				require.Equal(t, prev+1, num)
				if num == 1000 {
					return
				}
				prev = num
			case <-time.After(10 * time.Millisecond):
				require.Fail(t, "update has not been received")
			}
		}
	})
}

func TestIndex(t *testing.T) {
	s := NewStoreFixture(t)
	obj1 := TestObject{
		bundle.RelationKeyId:        domain.String("id1"),
		bundle.RelationKeyName:      domain.String("name1"),
		bundle.RelationKeyIsDeleted: domain.Bool(true),
	}
	obj2 := TestObject{
		bundle.RelationKeyId:   domain.String("id2"),
		bundle.RelationKeyName: domain.String("name2"),
	}
	obj3 := TestObject{
		bundle.RelationKeyId:   domain.String("id3"),
		bundle.RelationKeyName: domain.String("name3"),
	}
	s.AddObjects(t, []TestObject{obj1, obj2, obj3})

	recs, err := s.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyIsDeleted,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Bool(true),
			},
		},
	})
	require.NoError(t, err)

	assertRecordsEqual(t, []TestObject{
		obj2, obj3,
	}, recs)
}

func TestDsObjectStore_QueryAndProcess(t *testing.T) {
	const spaceId = "spaceId"
	s := NewStoreFixture(t)
	s.AddObjects(t, []TestObject{
		{
			bundle.RelationKeyId:      domain.String("id1"),
			bundle.RelationKeySpaceId: domain.String(spaceId),
			bundle.RelationKeyName:    domain.String("first"),
		},
		{
			bundle.RelationKeyId:         domain.String("id2"),
			bundle.RelationKeySpaceId:    domain.String(spaceId),
			bundle.RelationKeyName:       domain.String("favorite"),
			bundle.RelationKeyIsFavorite: domain.Bool(true),
		},
		{
			bundle.RelationKeyId:          domain.String("id3"),
			bundle.RelationKeySpaceId:     domain.String(spaceId),
			bundle.RelationKeyName:        domain.String("hi!"),
			bundle.RelationKeyDescription: domain.String("hi!"),
		},
	})

	t.Run("counter", func(t *testing.T) {
		var counter = 0
		err := s.QueryIterate(database.Query{Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeySpaceId,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(spaceId),
			},
		}}, func(_ *domain.Details) {
			counter++
		})

		assert.NoError(t, err)
		assert.Equal(t, 3, counter)
	})

	t.Run("favorites collector", func(t *testing.T) {
		favs := make([]string, 0)
		err := s.QueryIterate(database.Query{Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeySpaceId,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(spaceId),
			},
		}}, func(s *domain.Details) {
			if s.GetBool(bundle.RelationKeyIsFavorite) {
				favs = append(favs, s.GetString(bundle.RelationKeyId))
			}
		})

		assert.NoError(t, err)
		assert.Equal(t, []string{"id2"}, favs)
	})

	t.Run("name and description analyzer", func(t *testing.T) {
		ids := make([]string, 0)
		err := s.QueryIterate(database.Query{Filters: []database.FilterRequest{{
			RelationKey: bundle.RelationKeySpaceId,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(spaceId),
		}}}, func(s *domain.Details) {
			if s.GetString(bundle.RelationKeyName) == s.GetString(bundle.RelationKeyDescription) {
				ids = append(ids, s.GetString(bundle.RelationKeyId))
			}
		})

		assert.NoError(t, err)
		assert.Equal(t, []string{"id3"}, ids)
	})
}

func TestNestedFilters(t *testing.T) {
	t.Run("not in", func(t *testing.T) {
		store := NewStoreFixture(t)

		store.AddObjects(t, []TestObject{
			{
				bundle.RelationKeyId:   domain.String("id1"),
				bundle.RelationKeyType: domain.String("templateType"),
			},
			{
				bundle.RelationKeyId:   domain.String("id2"),
				bundle.RelationKeyType: domain.String("pageType"),
			},
			{
				bundle.RelationKeyId:   domain.String("id3"),
				bundle.RelationKeyType: domain.String("pageType"),
			},
			{
				bundle.RelationKeyId:   domain.String("id4"),
				bundle.RelationKeyType: domain.String("hiddenType"),
			},
			{
				bundle.RelationKeyId:        domain.String("templateType"),
				bundle.RelationKeyUniqueKey: domain.String("ot-template"),
			},
			{
				bundle.RelationKeyId:        domain.String("pageType"),
				bundle.RelationKeyUniqueKey: domain.String("ot-page"),
			},
			{
				bundle.RelationKeyId:        domain.String("hiddenType"),
				bundle.RelationKeyUniqueKey: domain.String("ot-hidden"),
			},
		})

		got, err := store.Query(database.Query{
			Filters: []database.FilterRequest{
				{
					RelationKey: "type.uniqueKey",
					Condition:   model.BlockContentDataviewFilter_NotIn,
					Value:       domain.StringList([]string{"ot-hidden", "ot-template"}),
				},
			},
		})
		require.NoError(t, err)

		assertRecordsHaveIds(t, got, []string{"id2", "id3", "templateType", "pageType", "hiddenType"})
	})
}

func assertRecordsHaveIds(t *testing.T, records []database.Record, wantIds []string) {
	require.Equal(t, len(wantIds), len(records))

	gotIds := map[string]struct{}{}
	for _, r := range records {
		gotIds[r.Details.GetString(bundle.RelationKeyId)] = struct{}{}
	}

	for _, id := range wantIds {
		_, ok := gotIds[id]
		require.True(t, ok)
	}
}
