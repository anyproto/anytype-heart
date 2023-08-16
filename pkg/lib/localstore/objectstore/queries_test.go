package objectstore

import (
	"fmt"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/typeprovider/mock_typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

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

		recs, _, err := s.Query(database.Query{})
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

		recs, _, err := s.Query(database.Query{
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

		recs, _, err := s.Query(database.Query{
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
			Id:    "id1",
			Title: "name",
		})
		require.NoError(t, err)

		err = s.fts.Index(ftsearch.SearchDoc{
			Id:    "id2",
			Title: "some important note",
		})
		require.NoError(t, err)

		err = s.fts.Index(ftsearch.SearchDoc{
			Id:    "id3",
			Title: "",
			Text:  "very important text",
		})
		require.NoError(t, err)

		t.Run("just full-text", func(t *testing.T) {
			recs, _, err := s.Query(database.Query{
				FullText: "important",
			})
			require.NoError(t, err)

			// Full-text engine has its own ordering, so just don't rely on it here and check only the content.
			assertRecordsMatch(t, []TestObject{
				obj2,
				obj3,
			}, recs)
		})

		t.Run("full-text and filter", func(t *testing.T) {
			recs, _, err := s.Query(database.Query{
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

	t.Run("without system objects", func(t *testing.T) {
		s := NewStoreFixture(t)
		typeProvider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
		s.sbtProvider = typeProvider

		obj1 := TestObject{
			bundle.RelationKeyId:      pbtypes.String("id1"),
			bundle.RelationKeyName:    pbtypes.String("Favorites page"),
			bundle.RelationKeySpaceId: pbtypes.String("space1"),
		}
		typeProvider.EXPECT().Type("space1", "id1").Return(smartblock.SmartBlockTypeHome, nil)

		obj2 := TestObject{
			bundle.RelationKeyId:      pbtypes.String("id2"),
			bundle.RelationKeyName:    pbtypes.String("name2"),
			bundle.RelationKeySpaceId: pbtypes.String("space1"),
		}
		typeProvider.EXPECT().Type("space1", "id2").Return(smartblock.SmartBlockTypePage, nil)

		obj3 := TestObject{
			bundle.RelationKeyId:      pbtypes.String("id3"),
			bundle.RelationKeyName:    pbtypes.String("Archive page"),
			bundle.RelationKeySpaceId: pbtypes.String("space1"),
		}
		typeProvider.EXPECT().Type("space1", "id3").Return(smartblock.SmartBlockTypeArchive, nil)

		s.AddObjects(t, []TestObject{obj1, obj2, obj3})

		recs, _, err := s.Query(database.Query{})
		require.NoError(t, err)

		assertRecordsEqual(t, []TestObject{
			obj2,
		}, recs)
	})

	t.Run("with ascending order and filter", func(t *testing.T) {
		s := NewStoreFixture(t)
		obj1 := makeObjectWithName("id1", "dfg")
		obj2 := makeObjectWithName("id2", "abc")
		obj3 := makeObjectWithName("id3", "012")
		obj4 := makeObjectWithName("id4", "ignore")
		s.AddObjects(t, []TestObject{obj1, obj2, obj3, obj4})

		recs, _, err := s.Query(database.Query{
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

		recs, _, err := s.Query(database.Query{
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

		recs, _, err := s.Query(database.Query{
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

	t.Run("with limit", func(t *testing.T) {
		s := NewStoreFixture(t)
		var objects []TestObject
		for i := 0; i < 100; i++ {
			objects = append(objects, generateSimpleObject(i))
		}
		s.AddObjects(t, objects)

		recs, _, err := s.Query(database.Query{
			Sorts: []*model.BlockContentDataviewSort{
				{
					RelationKey: bundle.RelationKeyId.String(),
					Type:        model.BlockContentDataviewSort_Asc,
				},
			},
			Limit: 15,
		})
		require.NoError(t, err)

		assertRecordsEqual(t, objects[:15], recs)
	})

	t.Run("with limit and offset", func(t *testing.T) {
		s := NewStoreFixture(t)
		var objects []TestObject
		for i := 0; i < 100; i++ {
			objects = append(objects, generateSimpleObject(i))
		}
		s.AddObjects(t, objects)

		limit := 15
		offset := 20
		recs, _, err := s.Query(database.Query{
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

		assertRecordsEqual(t, objects[offset:offset+limit], recs)
	})

	t.Run("with filter, limit and offset", func(t *testing.T) {
		s := NewStoreFixture(t)
		var objects []TestObject
		var filteredObjects []TestObject
		for i := 0; i < 100; i++ {
			if i%2 == 0 {
				objects = append(objects, generateSimpleObject(i))
			} else {
				obj := makeObjectWithName(fmt.Sprintf("id%02d", i), "this name")
				filteredObjects = append(filteredObjects, obj)
				objects = append(objects, obj)
			}

		}
		s.AddObjects(t, objects)

		limit := 60
		offset := 20
		recs, _, err := s.Query(database.Query{
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

		ids, _, err := s.QueryObjectIDs(database.Query{}, nil)
		require.NoError(t, err)
		assert.Equal(t, []string{"id1", "id2", "id3"}, ids)
	})

	t.Run("with smartblock types filter", func(t *testing.T) {
		s := NewStoreFixture(t)
		typeProvider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
		s.sbtProvider = typeProvider

		obj1 := makeObjectWithName("id1", "file1")
		typeProvider.EXPECT().Type("space1", "id1").Return(smartblock.SmartBlockTypeFile, nil)

		obj2 := makeObjectWithName("id2", "type2")
		typeProvider.EXPECT().Type("space1", "id2").Return(smartblock.SmartBlockTypeSubObject, nil)

		obj3 := makeObjectWithName("id3", "page3")
		typeProvider.EXPECT().Type("space1", "id3").Return(smartblock.SmartBlockTypePage, nil)
		s.AddObjects(t, []TestObject{obj1, obj2, obj3})

		ids, _, err := s.QueryObjectIDs(database.Query{}, []smartblock.SmartBlockType{smartblock.SmartBlockTypeFile, smartblock.SmartBlockTypePage})
		require.NoError(t, err)
		assert.Equal(t, []string{"id1", "id3"}, ids)

		t.Run("with limit", func(t *testing.T) {
			ids, _, err := s.QueryObjectIDs(database.Query{
				Limit: 1,
			}, []smartblock.SmartBlockType{smartblock.SmartBlockTypeFile, smartblock.SmartBlockTypePage})
			require.NoError(t, err)
			assert.Equal(t, []string{"id1"}, ids)
		})
		t.Run("with limit and offset", func(t *testing.T) {
			ids, _, err := s.QueryObjectIDs(database.Query{
				Limit:  1,
				Offset: 1,
			}, []smartblock.SmartBlockType{smartblock.SmartBlockTypeFile, smartblock.SmartBlockTypePage})
			require.NoError(t, err)
			assert.Equal(t, []string{"id3"}, ids)
		})
	})

	t.Run("with basic filter and smartblock types filter", func(t *testing.T) {
		s := NewStoreFixture(t)
		typeProvider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
		s.sbtProvider = typeProvider

		obj1 := makeObjectWithNameAndDescription("id1", "file1", "foo")
		typeProvider.EXPECT().Type("space1", "id1").Return(smartblock.SmartBlockTypeFile, nil)

		obj2 := makeObjectWithNameAndDescription("id2", "page2", "foo")
		typeProvider.EXPECT().Type("space1", "id2").Return(smartblock.SmartBlockTypePage, nil)

		obj3 := makeObjectWithNameAndDescription("id3", "page3", "bar")
		typeProvider.EXPECT().Type("space1", "id3").Return(smartblock.SmartBlockTypePage, nil)

		s.AddObjects(t, []TestObject{obj1, obj2, obj3})

		ids, _, err := s.QueryObjectIDs(database.Query{
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyDescription.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("foo"),
				},
			},
		}, []smartblock.SmartBlockType{smartblock.SmartBlockTypePage})
		require.NoError(t, err)
		assert.Equal(t, []string{"id2"}, ids)
	})
}

func TestQueryRaw(t *testing.T) {
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

		flt, err := database.NewFilters(database.Query{}, nil)
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
		}, s)
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
			bundle.RelationKey("typeKey"): pbtypes.String("note"),
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
		}, nil, s)
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
		err = s.UpdateObjectDetails("id1", makeDetails(obj1))
		require.ErrorIs(t, err, ErrDetailsNotChanged)

		select {
		case <-recordsCh:
			require.Fail(t, "unexpected record")
		case <-time.After(10 * time.Millisecond):
		}
	})

	t.Run("update details", func(t *testing.T) {
		err = s.UpdateObjectDetails("id1", makeDetails(makeObjectWithName("id1", "name1 updated")))
		require.NoError(t, err)

		select {
		case rec := <-recordsCh:
			assert.Equal(t, "name1 updated", pbtypes.GetString(rec, bundle.RelationKeyName.String()))
		case <-time.After(10 * time.Millisecond):
			require.Fail(t, "update has not been received")
		}
	})
}

func TestGetSpaceIDFromFilters(t *testing.T) {
	t.Run("spaceID provided", func(t *testing.T) {
		spaceID := "myspace"
		f := database.AndFilters{
			database.Eq{
				Key:   bundle.RelationKeyCreator.String(),
				Value: pbtypes.String("anytype"),
			},
			database.Eq{
				Key:   bundle.RelationKeySpaceId.String(),
				Value: pbtypes.String(spaceID),
			},
			database.Not{
				Filter: database.Eq{
					Key:   bundle.RelationKeyName.String(),
					Value: pbtypes.String("hidden obj"),
				},
			},
		}
		assert.Equal(t, spaceID, getSpaceIDFromFilter(f))
	})

	t.Run("no spaceID provided", func(t *testing.T) {
		f := database.AndFilters{
			database.Eq{
				Key:   bundle.RelationKeyId.String(),
				Value: pbtypes.String("some id"),
			},
			database.Empty{
				Key: bundle.RelationKeyType.String(),
			},
		}
		assert.Equal(t, "", getSpaceIDFromFilter(f))
	})

	t.Run("filters is filter.Eq with spaceID", func(t *testing.T) {
		spaceID := "open space"
		f := database.Eq{
			Key:   bundle.RelationKeySpaceId.String(),
			Value: pbtypes.String(spaceID),
		}
		assert.Equal(t, spaceID, getSpaceIDFromFilter(f))
	})

	t.Run("filters is filter.Eq without spaceID", func(t *testing.T) {
		f := database.Eq{
			Key:   bundle.RelationKeySetOf.String(),
			Value: pbtypes.String("ot-note"),
		}
		assert.Equal(t, "", getSpaceIDFromFilter(f))
	})

	t.Run("spaceID is nested in and filters", func(t *testing.T) {
		spaceID := "secret_space"
		f := database.AndFilters{
			database.AndFilters{
				database.Empty{Key: "somekey"},
				database.Eq{Key: "key", Value: pbtypes.String("value")},
				database.AndFilters{
					database.Eq{Key: "amount", Value: pbtypes.Float64(15)},
					database.Eq{Key: "type", Value: pbtypes.String("ot-note")},
					database.Eq{
						Key:   bundle.RelationKeySpaceId.String(),
						Value: pbtypes.String(spaceID),
					},
				},
			},
		}
		assert.Equal(t, spaceID, getSpaceIDFromFilter(f))
	})
}
