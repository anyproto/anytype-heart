package smartblock

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	bb "github.com/anyproto/anytype-heart/tests/blockbuilder"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestDependenciesSubscription(t *testing.T) {
	t.Run("with existing dependencies", func(t *testing.T) {
		mainObjId := "id"
		fx := newFixture(mainObjId, t)

		space1obj1 := "obj1"
		space1obj2 := "obj2"
		space2obj1 := "obj3"

		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String(space1obj1),
				bundle.RelationKeySpaceId: domain.String(testSpaceId),
				bundle.RelationKeyName:    domain.String("Object 1"),
			},
			{
				bundle.RelationKeyId:      domain.String(space1obj2),
				bundle.RelationKeySpaceId: domain.String(testSpaceId),
				bundle.RelationKeyName:    domain.String("Object 2"),
			},
		})
		fx.objectStore.AddObjects(t, "space2", []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String(space2obj1),
				bundle.RelationKeySpaceId: domain.String("space2"),
				bundle.RelationKeyName:    domain.String("Object 3"),
			},
		})

		fx.spaceIdResolver.EXPECT().ResolveSpaceID(space1obj1).Return(testSpaceId, nil)
		fx.spaceIdResolver.EXPECT().ResolveSpaceID(space1obj2).Return(testSpaceId, nil)
		fx.spaceIdResolver.EXPECT().ResolveSpaceID(space2obj1).Return("space2", nil)

		fx.space.EXPECT().Id().Return(testSpaceId)

		root := bb.Root(
			bb.ID(mainObjId),
			bb.Children(
				bb.Link(space1obj1),
				bb.Link(space1obj2),
				bb.Link(space2obj1),
			),
		)

		fx.Doc = state.NewDoc(mainObjId, root.BuildMap()).NewState()
		objDetails := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:      domain.String(mainObjId),
			bundle.RelationKeySpaceId: domain.String(testSpaceId),
			bundle.RelationKeyName:    domain.String("Main object"),
			bundle.RelationKeyLayout:  domain.Int64(int64(model.ObjectType_todo)),
		})

		fx.Doc.(*state.State).SetDetails(objDetails)

		details, err := fx.fetchMeta()
		require.NoError(t, err)
		require.NotEmpty(t, details)

		wantDetails := []*model.ObjectViewDetailsSet{
			{
				Id: mainObjId,
				Details: &types.Struct{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():      pbtypes.String(mainObjId),
						bundle.RelationKeySpaceId.String(): pbtypes.String(testSpaceId),
						bundle.RelationKeyName.String():    pbtypes.String("Main object"),
						bundle.RelationKeyLayout.String():  pbtypes.Int64(int64(model.ObjectType_todo)),
					},
				},
			},
			{
				Id: space1obj1,
				Details: &types.Struct{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():      pbtypes.String(space1obj1),
						bundle.RelationKeySpaceId.String(): pbtypes.String(testSpaceId),
						bundle.RelationKeyName.String():    pbtypes.String("Object 1"),
					},
				},
			},
			{
				Id: space1obj2,
				Details: &types.Struct{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():      pbtypes.String(space1obj2),
						bundle.RelationKeySpaceId.String(): pbtypes.String(testSpaceId),
						bundle.RelationKeyName.String():    pbtypes.String("Object 2"),
					},
				},
			},
			{
				Id: space2obj1,
				Details: &types.Struct{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():      pbtypes.String(space2obj1),
						bundle.RelationKeySpaceId.String(): pbtypes.String("space2"),
						bundle.RelationKeyName.String():    pbtypes.String("Object 3"),
					},
				},
			},
		}

		assert.ElementsMatch(t, wantDetails, details)

		fx.closeRecordsSub()
	})

	t.Run("with added dependencies", func(t *testing.T) {
		mainObjId := "id"
		fx := newFixture(mainObjId, t)

		root := bb.Root(
			bb.ID(mainObjId),
			bb.Children(),
		)
		fx.Doc = state.NewDoc(mainObjId, root.BuildMap()).NewState()

		details, err := fx.fetchMeta()
		require.NoError(t, err)
		require.Len(t, details, 1) // Only its own details

		// Simulate changes in state

		space1obj1 := "obj1"
		space1obj2 := "obj2"
		space2obj1 := "obj3"

		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String(space1obj1),
				bundle.RelationKeySpaceId: domain.String(testSpaceId),
				bundle.RelationKeyName:    domain.String("Object 1"),
			},
			{
				bundle.RelationKeyId:      domain.String(space1obj2),
				bundle.RelationKeySpaceId: domain.String(testSpaceId),
				bundle.RelationKeyName:    domain.String("Object 2"),
			},
		})
		fx.objectStore.AddObjects(t, "space2", []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String(space2obj1),
				bundle.RelationKeySpaceId: domain.String("space2"),
				bundle.RelationKeyName:    domain.String("Object 3"),
			},
		})

		fx.spaceIdResolver.EXPECT().ResolveSpaceID(space1obj1).Return(testSpaceId, nil)
		fx.spaceIdResolver.EXPECT().ResolveSpaceID(space1obj2).Return(testSpaceId, nil)
		fx.spaceIdResolver.EXPECT().ResolveSpaceID(space2obj1).Return("space2", nil)

		root = bb.Root(
			bb.ID(mainObjId),
			bb.Children(
				bb.Link(space1obj1),
				bb.Link(space1obj2),
				bb.Link(space2obj1),
			),
		)
		fx.Doc = state.NewDoc(mainObjId, root.BuildMap()).NewState()

		fx.CheckSubscriptions()

		assert.Contains(t, fx.smartBlock.lastDepDetails, space1obj1)
		assert.Contains(t, fx.smartBlock.lastDepDetails, space1obj2)
		assert.Contains(t, fx.smartBlock.lastDepDetails, space2obj1)
	})

}
