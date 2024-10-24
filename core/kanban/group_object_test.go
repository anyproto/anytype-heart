package kanban

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestGroupObject_InitGroups(t *testing.T) {
	t.Run("no relations - return error", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		groupObject := GroupObject{
			key:   "test",
			store: storeFixture,
		}
		// when
		err := groupObject.InitGroups("spaceId", nil)

		// then
		assert.NotNil(t, err)
	})
	t.Run("no objects with type from relation - only empty group", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		groupObject := GroupObject{
			key:   "test",
			store: storeFixture,
		}
		storeFixture.AddObjects(t, "spaceId", []objectstore.TestObject{
			{
				bundle.RelationKeyId:          pbtypes.String("test"),
				bundle.RelationKeyRelationKey: pbtypes.String("test"),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeySpaceId:     pbtypes.String("spaceId"),
			},
		})

		// when
		err := groupObject.InitGroups("spaceId", nil)
		assert.Nil(t, err)

		groups, err := groupObject.MakeDataViewGroups()
		assert.Nil(t, err)

		// then
		assert.Len(t, groups, 1)
		assert.Equal(t, "empty", groups[0].Id)
	})
	t.Run("objects with relation exists - create groups based on these objects", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		groupObject := GroupObject{
			key:   "test",
			store: storeFixture,
		}
		storeFixture.AddObjects(t, "spaceId", []objectstore.TestObject{
			{
				bundle.RelationKeyId:                        pbtypes.String("test"),
				bundle.RelationKeyRelationKey:               pbtypes.String("test"),
				bundle.RelationKeyLayout:                    pbtypes.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeySpaceId:                   pbtypes.String("spaceId"),
				bundle.RelationKeyRelationFormatObjectTypes: pbtypes.StringList([]string{"typeId"}),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("object1"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				bundle.RelationKeyType:    pbtypes.String("typeId"),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("object2"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				bundle.RelationKeyType:    pbtypes.String("typeId"),
			},
		})

		// when
		err := groupObject.InitGroups("spaceId", nil)
		assert.Nil(t, err)

		groups, err := groupObject.MakeDataViewGroups()
		assert.Nil(t, err)

		// then
		assert.Len(t, groups, 3) // empty, object1, object2
	})
	t.Run("objects with types exists - create groups based on these objects", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		groupObject := GroupObject{
			key:   "test",
			store: storeFixture,
		}
		storeFixture.AddObjects(t, "spaceId", []objectstore.TestObject{
			{
				bundle.RelationKeyId:                        pbtypes.String("test"),
				bundle.RelationKeyRelationKey:               pbtypes.String("test"),
				bundle.RelationKeyLayout:                    pbtypes.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeySpaceId:                   pbtypes.String("spaceId"),
				bundle.RelationKeyRelationFormatObjectTypes: pbtypes.StringList([]string{"typeId"}),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("object1"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				bundle.RelationKeyType:    pbtypes.String("typeId"),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("object2"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				bundle.RelationKeyType:    pbtypes.String("typeId"),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("object3"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				bundle.RelationKeyType:    pbtypes.String("typeId3"),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("object3"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				"test":                    pbtypes.StringList([]string{"object1"}),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("object4"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				"test":                    pbtypes.StringList([]string{"object2"}),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("object5"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				"test":                    pbtypes.StringList([]string{"object1", "object2"}),
			},
		})

		// when
		err := groupObject.InitGroups("spaceId", nil)
		assert.Nil(t, err)

		groups, err := groupObject.MakeDataViewGroups()
		assert.Nil(t, err)

		// then
		assert.Len(t, groups, 4) // empty, object1, object2, object1 object 2
	})
	t.Run("objects with types exists, but we also have additional filter - create groups based on these objects", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		groupObject := GroupObject{
			key:   "test",
			store: storeFixture,
		}
		storeFixture.AddObjects(t, "spaceId", []objectstore.TestObject{
			{
				bundle.RelationKeyId:                        pbtypes.String("test"),
				bundle.RelationKeyRelationKey:               pbtypes.String("test"),
				bundle.RelationKeyLayout:                    pbtypes.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeySpaceId:                   pbtypes.String("spaceId"),
				bundle.RelationKeyRelationFormatObjectTypes: pbtypes.StringList([]string{"typeId"}),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("object1"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				bundle.RelationKeyType:    pbtypes.String("typeId"),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("object2"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				bundle.RelationKeyType:    pbtypes.String("typeId"),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("object3"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				"test":                    pbtypes.StringList([]string{"object1"}),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("object4"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				"test":                    pbtypes.StringList([]string{"object2"}),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("object5"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				"test":                    pbtypes.StringList([]string{"object1", "object2"}),
			},
		})

		// when
		err := groupObject.InitGroups(
			"spaceId",
			&database.Filters{FilterObj: database.FilterNot{Filter: database.FilterEq{
				Key:   bundle.RelationKeyId.String(),
				Cond:  model.BlockContentDataviewFilter_Equal,
				Value: pbtypes.String("object5"),
			}}},
		)
		assert.Nil(t, err)

		groups, err := groupObject.MakeDataViewGroups()
		assert.Nil(t, err)

		// then
		assert.Len(t, groups, 3) // empty, object1, object2
	})
	t.Run("relation without type", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		groupObject := GroupObject{
			key:   "test",
			store: storeFixture,
		}
		storeFixture.AddObjects(t, "spaceId", []objectstore.TestObject{
			{
				bundle.RelationKeyId:          pbtypes.String("test"),
				bundle.RelationKeyRelationKey: pbtypes.String("test"),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeySpaceId:     pbtypes.String("spaceId"),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("object1"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				bundle.RelationKeyType:    pbtypes.String("typeId"),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("object2"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				bundle.RelationKeyType:    pbtypes.String("typeId2"),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("object3"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				bundle.RelationKeyType:    pbtypes.String("typeId3"),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("object3"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				"test":                    pbtypes.StringList([]string{"object1"}),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("object4"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				"test":                    pbtypes.StringList([]string{"object2"}),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("object5"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				"test":                    pbtypes.StringList([]string{"object1", "object2"}),
			},
		})

		// when
		err := groupObject.InitGroups("spaceId", nil)
		assert.Nil(t, err)

		groups, err := groupObject.MakeDataViewGroups()
		assert.Nil(t, err)

		// then
		assert.Len(t, groups, 7) // empty, object1, object2, object1 object 2
	})
}
