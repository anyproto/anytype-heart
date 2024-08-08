package objectstore

import (
	context2 "context"
	"fmt"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fastjson"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func removeScoreFromRecords(records []database.Record) []database.Record {
	for i := range records {
		delete(records[i].Details.Fields, "_score")
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
			bundle.RelationKeyId:   pbtypes.String("id1"),
			bundle.RelationKeyName: pbtypes.String("name1"),
		}
		obj2 := TestObject{
			bundle.RelationKeyId:   pbtypes.String("id2"),
			bundle.RelationKeyName: pbtypes.String("name2"),
		}
		obj3 := TestObject{
			bundle.RelationKeyId:   pbtypes.String("id3"),
			bundle.RelationKeyName: pbtypes.String("name3"),
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
			bundle.RelationKeyId:   pbtypes.String("id1"),
			bundle.RelationKeyName: pbtypes.String("name1"),
		}
		obj2 := TestObject{
			bundle.RelationKeyId:   pbtypes.String("id2"),
			bundle.RelationKeyName: pbtypes.String("name2"),
		}
		obj3 := TestObject{
			bundle.RelationKeyId:   pbtypes.String("id3"),
			bundle.RelationKeyName: pbtypes.String("name3"),
		}
		s.AddObjects(t, []TestObject{obj1, obj2, obj3})

		recs, err := s.Query(database.Query{
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyName.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("name2"),
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
			bundle.RelationKeyId:   pbtypes.String("id1"),
			bundle.RelationKeyName: pbtypes.String("name"),
		}
		obj2 := TestObject{
			bundle.RelationKeyId:          pbtypes.String("id2"),
			bundle.RelationKeyName:        pbtypes.String("name"),
			bundle.RelationKeyDescription: pbtypes.String("description"),
		}
		obj3 := TestObject{
			bundle.RelationKeyId:          pbtypes.String("id3"),
			bundle.RelationKeyName:        pbtypes.String("name"),
			bundle.RelationKeyDescription: pbtypes.String("description"),
		}
		s.AddObjects(t, []TestObject{obj1, obj2, obj3})

		recs, err := s.Query(database.Query{
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyName.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("name"),
				},
				{
					RelationKey: bundle.RelationKeyDescription.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("description"),
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
			bundle.RelationKeyId:          pbtypes.String("id1"),
			bundle.RelationKeyName:        pbtypes.String("name"),
			bundle.RelationKeyDescription: pbtypes.String("foo"),
		}
		obj2 := TestObject{
			bundle.RelationKeyId:          pbtypes.String("id2"),
			bundle.RelationKeyName:        pbtypes.String("some important note"),
			bundle.RelationKeyDescription: pbtypes.String("foo"),
		}
		obj3 := TestObject{
			bundle.RelationKeyId:          pbtypes.String("id3"),
			bundle.RelationKeyName:        pbtypes.String(""),
			bundle.RelationKeyDescription: pbtypes.String("bar"),
		}
		s.AddObjects(t, []TestObject{obj1, obj2, obj3})

		err := s.fts.Index(ftsearch.SearchDoc{
			Id:    "id1/r/name",
			Title: "myname1",
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
				FullText: "important",
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
				FullText: "myname1",
			})
			require.NoError(t, err)

			// Full-text engine has its own ordering, so just don't rely on it here and check only the content.
			assertRecordsMatch(t, []TestObject{
				obj1,
			}, recs)
		})

		t.Run("full-text and filter", func(t *testing.T) {
			recs, err := s.Query(database.Query{
				FullText: "important",
				Filters: []*model.BlockContentDataviewFilter{
					{
						RelationKey: bundle.RelationKeyDescription.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String("foo"),
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
			bundle.RelationKeyId:          pbtypes.String("id1"),
			domain.RelationKey("bsonid1"): pbtypes.String("relid1"),
			bundle.RelationKeyDescription: pbtypes.String("this is the first object description"),
		}
		obj2 := TestObject{
			bundle.RelationKeyId:          pbtypes.String("id2"),
			bundle.RelationKeyType:        pbtypes.String("typeid1"),
			bundle.RelationKeyDescription: pbtypes.String("this is the second object description"),
		}

		obj3 := TestObject{
			bundle.RelationKeyId:   pbtypes.String("id3"),
			bundle.RelationKeyType: pbtypes.String("typeid1"),
		}

		relObj := TestObject{
			bundle.RelationKeyId:          pbtypes.String("relid1"),
			bundle.RelationKeyRelationKey: pbtypes.String("bsonid1"),
			bundle.RelationKeyName:        pbtypes.String("relname"),
			bundle.RelationKeyDescription: pbtypes.String("this is a relation's description"),
			bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relationOption)),
		}

		relObjDeleted := TestObject{
			bundle.RelationKeyId:          pbtypes.String("relid2"),
			bundle.RelationKeyRelationKey: pbtypes.String("bsonid1"),
			bundle.RelationKeyName:        pbtypes.String("deletedtag"),
			bundle.RelationKeyIsDeleted:   pbtypes.Bool(true),
			bundle.RelationKeyDescription: pbtypes.String("this is a deleted relation's description"),
			bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relationOption)),
		}

		relObjArchived := TestObject{
			bundle.RelationKeyId:          pbtypes.String("relid3"),
			bundle.RelationKeyRelationKey: pbtypes.String("bsonid1"),
			bundle.RelationKeyName:        pbtypes.String("archived"),
			bundle.RelationKeyIsDeleted:   pbtypes.Bool(true),
			bundle.RelationKeyDescription: pbtypes.String("this is a archived relation's description"),
			bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relationOption)),
		}

		typeObj := TestObject{
			bundle.RelationKeyId:          pbtypes.String("typeid1"),
			bundle.RelationKeyName:        pbtypes.String("typename"),
			bundle.RelationKeyDescription: pbtypes.String("this is a type's description"),
			bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_objectType)),
		}

		s.AddObjects(t, []TestObject{obj1, obj2, obj3, relObj, relObjDeleted, relObjArchived, typeObj})
		err := s.fts.Index(ftsearch.SearchDoc{
			Id:   "id1/r/description",
			Text: obj1[bundle.RelationKeyDescription].GetStringValue(),
		})
		require.NoError(t, err)

		err = s.fts.Index(ftsearch.SearchDoc{
			Id:   "id2/r/description",
			Text: obj2[bundle.RelationKeyDescription].GetStringValue(),
		})
		require.NoError(t, err)

		err = s.fts.Index(ftsearch.SearchDoc{
			Id:   "relid1/r/description",
			Text: relObj[bundle.RelationKeyDescription].GetStringValue(),
		})
		require.NoError(t, err)
		err = s.fts.Index(ftsearch.SearchDoc{
			Id:    "relid1/r/name",
			Title: relObj[bundle.RelationKeyName].GetStringValue(),
		})
		require.NoError(t, err)

		err = s.fts.Index(ftsearch.SearchDoc{
			Id:   "relid2/r/description",
			Text: relObjDeleted[bundle.RelationKeyDescription].GetStringValue(),
		})
		require.NoError(t, err)
		err = s.fts.Index(ftsearch.SearchDoc{
			Id:    "relid2/r/name",
			Title: relObjDeleted[bundle.RelationKeyName].GetStringValue(),
		})
		require.NoError(t, err)

		err = s.fts.Index(ftsearch.SearchDoc{
			Id:   "relid3/r/description",
			Text: relObjArchived[bundle.RelationKeyDescription].GetStringValue(),
		})
		require.NoError(t, err)
		err = s.fts.Index(ftsearch.SearchDoc{
			Id:    "relid3/r/name",
			Title: relObjArchived[bundle.RelationKeyName].GetStringValue(),
		})
		require.NoError(t, err)

		err = s.fts.Index(ftsearch.SearchDoc{
			Id:    "typeid1/r/name",
			Title: typeObj[bundle.RelationKeyName].GetStringValue(),
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
				FullText: "first object",
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
				FullText: "sage",
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
				FullText: "block",
				Sorts: []*model.BlockContentDataviewSort{
					{
						RelationKey: bundle.RelationKeyId.String(),
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
				FullText: "dog",
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
				FullText: "Сонце",
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
				FullText: "relname",
				Filters: []*model.BlockContentDataviewFilter{
					{
						Operator:    0,
						RelationKey: "layout",
						Condition:   model.BlockContentDataviewFilter_NotIn,
						Value:       pbtypes.IntList(int(model.ObjectType_relationOption)),
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
						RelationDetails: pbtypes.StructFilterKeys(makeDetails(relObj), []string{bundle.RelationKeyLayout.String(), bundle.RelationKeyId.String(), bundle.RelationKeyName.String()}),
					},
				}}, recs)
		})

		t.Run("full-text by deleted tag", func(t *testing.T) {
			recs, err := s.Query(database.Query{
				FullText: "deleted",
				Filters: []*model.BlockContentDataviewFilter{
					{
						Operator:    0,
						RelationKey: "layout",
						Condition:   model.BlockContentDataviewFilter_NotIn,
						Value:       pbtypes.IntList(int(model.ObjectType_relationOption)),
					},
				},
			})
			require.NoError(t, err)
			removeScoreFromRecords(recs)
			assert.Len(t, recs, 0)
		})

		t.Run("full-text by archived tag", func(t *testing.T) {
			recs, err := s.Query(database.Query{
				FullText: "archived",
				Filters: []*model.BlockContentDataviewFilter{
					{
						Operator:    0,
						RelationKey: "layout",
						Condition:   model.BlockContentDataviewFilter_NotIn,
						Value:       pbtypes.IntList(int(model.ObjectType_relationOption)),
					},
				},
			})
			require.NoError(t, err)
			removeScoreFromRecords(recs)
			assert.Len(t, recs, 0)
		})

		t.Run("full-text by type", func(t *testing.T) {
			recs, err := s.Query(database.Query{
				FullText: "typename",
				Filters: []*model.BlockContentDataviewFilter{
					{
						Operator:    0,
						RelationKey: "layout",
						Condition:   model.BlockContentDataviewFilter_NotIn,
						Value:       pbtypes.IntList(int(model.ObjectType_objectType)),
					},
				},
			})
			require.NoError(t, err)
			removeScoreFromRecords(recs)
			assert.ElementsMatch(t, []database.Record{
				{
					Details: makeDetails(obj2),
					Meta: model.SearchMeta{
						RelationKey:     "type",
						RelationDetails: pbtypes.StructFilterKeys(makeDetails(typeObj), []string{bundle.RelationKeyLayout.String(), bundle.RelationKeyId.String(), bundle.RelationKeyName.String()}),
					},
				},
				{
					Details: makeDetails(obj3),
					Meta: model.SearchMeta{
						RelationKey:     "type",
						RelationDetails: pbtypes.StructFilterKeys(makeDetails(typeObj), []string{bundle.RelationKeyLayout.String(), bundle.RelationKeyId.String(), bundle.RelationKeyName.String()}),
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
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyName.String(),
					Condition:   model.BlockContentDataviewFilter_NotEqual,
					Value:       pbtypes.String("ignore"),
				},
			},
			Sorts: []*model.BlockContentDataviewSort{
				{
					RelationKey: bundle.RelationKeyName.String(),
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
			Sorts: []*model.BlockContentDataviewSort{
				{
					RelationKey: bundle.RelationKeyName.String(),
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
			Sorts: []*model.BlockContentDataviewSort{
				{
					RelationKey: bundle.RelationKeyDescription.String(),
					Type:        model.BlockContentDataviewSort_Desc,
				},
				{
					RelationKey: bundle.RelationKeyName.String(),
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
			Sorts: []*model.BlockContentDataviewSort{
				{
					RelationKey: bundle.RelationKeyId.String(),
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
			idA := pbtypes.GetString(a, bundle.RelationKeyId.String())
			idB := pbtypes.GetString(b, bundle.RelationKeyId.String())
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
			Sorts: []*model.BlockContentDataviewSort{
				{
					RelationKey: bundle.RelationKeyId.String(),
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
			idA := pbtypes.GetString(a, bundle.RelationKeyId.String())
			idB := pbtypes.GetString(b, bundle.RelationKeyId.String())
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
			Sorts: []*model.BlockContentDataviewSort{
				{
					RelationKey: bundle.RelationKeyId.String(),
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
			idA := pbtypes.GetString(a, bundle.RelationKeyId.String())
			idB := pbtypes.GetString(b, bundle.RelationKeyId.String())
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
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyName.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("this name"),
				},
			},
			Sorts: []*model.BlockContentDataviewSort{
				{
					RelationKey: bundle.RelationKeyId.String(),
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

		ids, _, err := s.QueryObjectIDs(database.Query{})
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
		ids, _, err := s.QueryObjectIDs(database.Query{
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyDescription.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("foo"),
				},
			},
		})
		require.NoError(t, err)

		// Then
		assert.Equal(t, []string{"id1", "id2"}, ids)
	})
}

func TestQueryRaw(t *testing.T) {
	arena := &fastjson.Arena{}

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

		flt, err := database.NewFilters(database.Query{}, s, arena)
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
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyDescription.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("foo"),
				},
			},
		}, s, arena)
		require.NoError(t, err)

		recs, err := s.QueryRaw(flt, 0, 0)
		require.NoError(t, err)
		assertRecordsEqual(t, []TestObject{obj1, obj3}, recs)
	})

	t.Run("with nested filter", func(t *testing.T) {
		s := NewStoreFixture(t)
		obj1 := TestObject{
			bundle.RelationKeyId:   pbtypes.String("id1"),
			bundle.RelationKeyType: pbtypes.String("type1"),
		}
		type1 := TestObject{
			bundle.RelationKeyId:          pbtypes.String("type1"),
			bundle.RelationKeyType:        pbtypes.String("objectType"),
			domain.RelationKey("typeKey"): pbtypes.String("note"),
		}

		s.AddObjects(t, []TestObject{obj1, type1})

		flt, err := database.NewFilters(database.Query{
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: "type.typeKey",
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("note"),
				},
			},
		}, s, arena)
		require.NoError(t, err)

		recs, err := s.QueryRaw(flt, 0, 0)
		require.NoError(t, err)
		assertRecordsEqual(t, []TestObject{obj1}, recs)
	})
}

type dummySourceService struct {
	objectToReturn TestObject
}

func (s dummySourceService) DetailsFromIdBasedSource(id string) (*types.Struct, error) {
	return makeDetails(s.objectToReturn), nil
}

func TestQueryById(t *testing.T) {
	t.Run("no ids", func(t *testing.T) {
		s := NewStoreFixture(t)

		recs, err := s.QueryByID(nil)
		require.NoError(t, err)
		assert.Empty(t, recs)
	})

	t.Run("just ordinary objects", func(t *testing.T) {
		s := NewStoreFixture(t)
		obj1 := makeObjectWithName("id1", "name1")
		obj2 := makeObjectWithName("id2", "name2")
		obj3 := makeObjectWithName("id3", "name3")
		s.AddObjects(t, []TestObject{obj1, obj2, obj3})

		recs, err := s.QueryByID([]string{"id1", "id3"})
		require.NoError(t, err)
		assertRecordsEqual(t, []TestObject{obj1, obj3}, recs)

		t.Run("reverse order", func(t *testing.T) {
			recs, err := s.QueryByID([]string{"id3", "id1"})
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

		recs, err := s.QueryByID([]string{"id1", dateID})
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

	recordsCh := make(chan *types.Struct)
	sub := database.NewSubscription(nil, recordsCh)

	recs, closeSub, err := s.QueryByIDAndSubscribeForChanges([]string{"id1", "id3"}, sub)
	require.NoError(t, err)
	defer closeSub()

	assertRecordsEqual(t, []TestObject{obj1, obj3}, recs)

	t.Run("update details called, but there are no changes", func(t *testing.T) {
		err = s.UpdateObjectDetails(context2.Background(), "id1", makeDetails(obj1))
		require.NoError(t, err)

		select {
		case <-recordsCh:
			require.Fail(t, "unexpected record")
		case <-time.After(10 * time.Millisecond):
		}
	})

	t.Run("update details order", func(t *testing.T) {
		for i := 1; i <= 1000; i++ {
			err = s.UpdateObjectDetails(context2.Background(), "id1", makeDetails(makeObjectWithName("id1", fmt.Sprintf("%d", i))))
			require.NoError(t, err)
		}

		prev := 0
		for {
			select {
			case rec := <-recordsCh:
				name := pbtypes.GetString(rec, bundle.RelationKeyName.String())
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

func TestGetSpaceIDFromFilters(t *testing.T) {
	t.Run("spaceID provided", func(t *testing.T) {
		spaceId := "myspace"
		f := database.FiltersAnd{
			database.FilterEq{
				Key:   bundle.RelationKeyCreator.String(),
				Value: pbtypes.String("anytype"),
			},
			database.FilterEq{
				Key:   bundle.RelationKeySpaceId.String(),
				Value: pbtypes.String(spaceId),
			},
			database.FilterNot{
				Filter: database.FilterEq{
					Key:   bundle.RelationKeyName.String(),
					Value: pbtypes.String("hidden obj"),
				},
			},
		}
		assert.Equal(t, []string{spaceId}, getSpaceIdsFromFilter(f))
	})

	t.Run("no spaceID provided", func(t *testing.T) {
		f := database.FiltersAnd{
			database.FilterEq{
				Key:   bundle.RelationKeyId.String(),
				Value: pbtypes.String("some id"),
			},
			database.FilterEmpty{
				Key: bundle.RelationKeyType.String(),
			},
		}
		assert.Equal(t, 0, len(getSpaceIdsFromFilter(f)))
	})

	t.Run("filters is filter.FilterEq with spaceID", func(t *testing.T) {
		spaceId := "open space"
		f := database.FilterEq{
			Key:   bundle.RelationKeySpaceId.String(),
			Value: pbtypes.String(spaceId),
		}
		assert.Equal(t, []string{spaceId}, getSpaceIdsFromFilter(f))
	})

	t.Run("filters is filter.FilterEq without spaceID", func(t *testing.T) {
		f := database.FilterEq{
			Key:   bundle.RelationKeySetOf.String(),
			Value: pbtypes.String("ot-note"),
		}
		assert.Equal(t, 0, len(getSpaceIdsFromFilter(f)))
	})

	t.Run("filters is filter.FilterIn with spaceId", func(t *testing.T) {
		list, err := pbtypes.ValueListWrapper(pbtypes.StringList([]string{"space1"}))
		assert.NoError(t, err)

		f := database.FilterIn{
			Key:   bundle.RelationKeySpaceId.String(),
			Value: list,
		}
		assert.Equal(t, []string{"space1"}, getSpaceIdsFromFilter(f))
	})

	t.Run("filters is filter.FilterIn with many spaceId", func(t *testing.T) {
		list, err := pbtypes.ValueListWrapper(pbtypes.StringList([]string{"space1", "space2"}))
		assert.NoError(t, err)

		f := database.FilterIn{
			Key:   bundle.RelationKeySpaceId.String(),
			Value: list,
		}
		assert.Equal(t, []string{"space1", "space2"}, getSpaceIdsFromFilter(f))
	})

	t.Run("spaceID is nested in and filters", func(t *testing.T) {
		spaceId := "secret_space"
		f := database.FiltersAnd{
			database.FiltersAnd{
				database.FilterEmpty{Key: "somekey"},
				database.FilterEq{Key: "key", Value: pbtypes.String("value")},
				database.FiltersAnd{
					database.FilterEq{Key: "amount", Value: pbtypes.Float64(15)},
					database.FilterEq{Key: "type", Value: pbtypes.String("ot-note")},
					database.FilterEq{
						Key:   bundle.RelationKeySpaceId.String(),
						Value: pbtypes.String(spaceId),
					},
				},
			},
		}
		assert.Equal(t, []string{spaceId}, getSpaceIdsFromFilter(f))
	})
}
