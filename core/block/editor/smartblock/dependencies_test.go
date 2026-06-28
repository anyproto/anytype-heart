package smartblock

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
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

func amendedKeys(t *testing.T, e *pb.Event) []string {
	t.Helper()
	var keys []string
	for _, m := range e.Messages {
		if a := m.GetObjectDetailsAmend(); a != nil {
			for _, kv := range a.Details {
				keys = append(keys, kv.Key)
			}
		}
	}
	return keys
}

func TestOnMetaChangeStripKeys(t *testing.T) {
	setup := func(t *testing.T) (*fixture, *[]*pb.Event) {
		mainObjId := "id"
		fx := newFixture(mainObjId, t)
		root := bb.Root(bb.ID(mainObjId), bb.Children())
		fx.Doc = state.NewDoc(mainObjId, root.BuildMap()).NewState()

		var events []*pb.Event
		fx.RegisterSession(session.NewContext())
		fx.eventSender.EXPECT().
			SendToSession(mock.Anything, mock.Anything).
			Run(func(_ string, e *pb.Event) { events = append(events, e) }).
			Maybe()
		return fx, &events
	}

	t.Run("dependent sync-only change emits no event", func(t *testing.T) {
		fx, events := setup(t)
		depId := "dep1"
		fx.onMetaChange(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:         domain.String(depId),
			bundle.RelationKeyName:       domain.String("Dep"),
			bundle.RelationKeySyncStatus: domain.Int64(1),
		}))
		*events = nil // discard first-sight Set

		fx.onMetaChange(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:         domain.String(depId),
			bundle.RelationKeyName:       domain.String("Dep"),
			bundle.RelationKeySyncStatus: domain.Int64(2),
			bundle.RelationKeySyncDate:   domain.Int64(123456),
		}))
		assert.Empty(t, *events)
	})

	t.Run("dependent name change emits amend without sync keys", func(t *testing.T) {
		fx, events := setup(t)
		depId := "dep2"
		fx.onMetaChange(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:         domain.String(depId),
			bundle.RelationKeyName:       domain.String("Old"),
			bundle.RelationKeySyncStatus: domain.Int64(1),
		}))
		*events = nil

		fx.onMetaChange(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:         domain.String(depId),
			bundle.RelationKeyName:       domain.String("New"),
			bundle.RelationKeySyncStatus: domain.Int64(2),
		}))
		require.Len(t, *events, 1)
		keys := amendedKeys(t, (*events)[0])
		assert.Contains(t, keys, bundle.RelationKeyName.String())
		assert.NotContains(t, keys, bundle.RelationKeySyncStatus.String())
	})

	t.Run("self sync change is still emitted", func(t *testing.T) {
		fx, events := setup(t)
		selfId := fx.Id() // == "id"
		fx.onMetaChange(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:         domain.String(selfId),
			bundle.RelationKeySyncStatus: domain.Int64(1),
		}))
		*events = nil

		fx.onMetaChange(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:         domain.String(selfId),
			bundle.RelationKeySyncStatus: domain.Int64(2),
		}))
		require.Len(t, *events, 1)
		keys := amendedKeys(t, (*events)[0])
		assert.Contains(t, keys, bundle.RelationKeySyncStatus.String())
	})
}

func findViewDetails(t *testing.T, set []*model.ObjectViewDetailsSet, id string) *types.Struct {
	t.Helper()
	for _, d := range set {
		if d.Id == id {
			return d.Details
		}
	}
	t.Fatalf("ObjectViewDetailsSet for %s not found", id)
	return nil
}

func TestFetchMetaStripsDependentKeys(t *testing.T) {
	mainObjId := "id"
	fx := newFixture(mainObjId, t)

	depId := "obj1"
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:         domain.String(depId),
			bundle.RelationKeySpaceId:    domain.String(testSpaceId),
			bundle.RelationKeyName:       domain.String("Object 1"),
			bundle.RelationKeySyncStatus: domain.Int64(1),
			bundle.RelationKeySyncDate:   domain.Int64(999),
		},
	})
	fx.spaceIdResolver.EXPECT().ResolveSpaceID(depId).Return(testSpaceId, nil)
	fx.space.EXPECT().Id().Return(testSpaceId)

	root := bb.Root(bb.ID(mainObjId), bb.Children(bb.Link(depId)))
	fx.Doc = state.NewDoc(mainObjId, root.BuildMap()).NewState()
	fx.Doc.(*state.State).SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:         domain.String(mainObjId),
		bundle.RelationKeySpaceId:    domain.String(testSpaceId),
		bundle.RelationKeyName:       domain.String("Main object"),
		bundle.RelationKeySyncStatus: domain.Int64(1),
	}))

	details, err := fx.fetchMeta()
	require.NoError(t, err)
	defer fx.closeRecordsSub()

	dep := findViewDetails(t, details, depId)
	assert.Contains(t, dep.Fields, bundle.RelationKeyName.String())
	assert.NotContains(t, dep.Fields, bundle.RelationKeySyncStatus.String())
	assert.NotContains(t, dep.Fields, bundle.RelationKeySyncDate.String())

	self := findViewDetails(t, details, mainObjId)
	assert.Contains(t, self.Fields, bundle.RelationKeySyncStatus.String())
}
