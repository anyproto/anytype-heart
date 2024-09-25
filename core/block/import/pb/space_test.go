package pb

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	smartblock2 "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestSpaceImport_ProvideCollection(t *testing.T) {
	t.Run("no snapshots - root collection without objects", func(t *testing.T) {
		// given
		collectionProvider := SpaceImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		// when
		collection, err := collectionProvider.ProvideCollection(nil, nil, params, false)

		// then
		assert.Nil(t, err)
		assert.Len(t, collection, 1)
		rootCollectionState := state.NewDocFromSnapshot("", collection[0].Snapshot).(*state.State)
		objectsInCollection := rootCollectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, objectsInCollection, 0)
	})
	t.Run("NoCollection parameter is true - no collection was created", func(t *testing.T) {
		// given
		collectionProvider := SpaceImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: true}

		// when
		collection, err := collectionProvider.ProvideCollection(nil, nil, params, false)

		// then
		assert.Nil(t, err)
		assert.Nil(t, collection)
	})
	t.Run("no widget object - add all objects (except template and subobjects) in Protobuf Import collection", func(t *testing.T) {
		// given
		p := SpaceImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		allSnapshot := []*common.Snapshot{
			{
				Id:     "id1",
				SbType: smartblock2.SmartBlockTypePage,
			},
			{
				Id:     "id2",
				SbType: smartblock2.SmartBlockTypeSubObject,
			},
			{
				Id:     "id3",
				SbType: smartblock2.SmartBlockTypeTemplate,
			},
			{
				Id:     "id4",
				SbType: smartblock2.SmartBlockTypePage,
			},
		}

		// when
		collection, err := p.ProvideCollection(&snapshotSet{List: allSnapshot}, nil, params, false)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.Len(t, collection, 1)
		rootCollectionState := state.NewDocFromSnapshot("", collection[0].Snapshot).(*state.State)
		objectsInCollection := rootCollectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, objectsInCollection, 2)
		assert.Equal(t, objectsInCollection[0], "id1")
		assert.Equal(t, objectsInCollection[1], "id4")
	})
	t.Run("widget with sets - add only sets in Protobuf Import collection", func(t *testing.T) {
		// given
		p := SpaceImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		allSnapshot := []*common.Snapshot{
			// skip objects
			{
				Id:     "id2",
				SbType: smartblock2.SmartBlockTypeSubObject,
			},
			{
				Id:     "id3",
				SbType: smartblock2.SmartBlockTypeTemplate,
			},
			// page
			{
				Id:     "id1",
				SbType: smartblock2.SmartBlockTypePage,
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details:     &types.Struct{Fields: map[string]*types.Value{}},
						ObjectTypes: []string{bundle.TypeKeyPage.URL()},
					},
				},
			},
			// collection
			{
				Id:     "id4",
				SbType: smartblock2.SmartBlockTypePage,
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details:     &types.Struct{Fields: map[string]*types.Value{}},
						ObjectTypes: []string{bundle.TypeKeyCollection.URL()},
					},
				},
			},
			// set
			{
				Id:     "id5",
				SbType: smartblock2.SmartBlockTypePage,
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details:     &types.Struct{Fields: map[string]*types.Value{}},
						ObjectTypes: []string{bundle.TypeKeySet.URL()},
					},
				},
			},
		}
		// set widget
		widgetSnapshot := &common.Snapshot{
			Id:     "widgetID",
			SbType: smartblock2.SmartBlockTypeWidget,
			Snapshot: &pb.ChangeSnapshot{
				Data: &model.SmartBlockSnapshotBase{
					Blocks: []*model.Block{
						{
							Id: "widgetID",
							Content: &model.BlockContentOfLink{
								Link: &model.BlockContentLink{
									TargetBlockId: widget.DefaultWidgetSet,
								},
							},
						},
					},
				},
			},
		}

		// when
		collection, err := p.ProvideCollection(&snapshotSet{List: allSnapshot, Widget: widgetSnapshot}, nil, params, false)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.Len(t, collection, 1)
		rootCollectionState := state.NewDocFromSnapshot("", collection[0].Snapshot).(*state.State)
		objectsInCollection := rootCollectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, objectsInCollection, 1)
		assert.Equal(t, objectsInCollection[0], "id5")
	})
	t.Run("widget with collection - add collection in Protobuf Import collection", func(t *testing.T) {
		// given
		p := SpaceImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		allSnapshot := []*common.Snapshot{
			// skip objects
			{
				Id:     "id2",
				SbType: smartblock2.SmartBlockTypeSubObject,
			},
			{
				Id:     "id3",
				SbType: smartblock2.SmartBlockTypeTemplate,
			},
			// page
			{
				Id:     "id1",
				SbType: smartblock2.SmartBlockTypePage,
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details:     &types.Struct{Fields: map[string]*types.Value{}},
						ObjectTypes: []string{bundle.TypeKeyPage.URL()},
					},
				},
			},
			// collection
			{
				Id:     "id4",
				SbType: smartblock2.SmartBlockTypePage,
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details:     &types.Struct{Fields: map[string]*types.Value{}},
						ObjectTypes: []string{bundle.TypeKeyCollection.URL()},
					},
				},
			},
			// set
			{
				Id:     "id5",
				SbType: smartblock2.SmartBlockTypePage,
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details:     &types.Struct{Fields: map[string]*types.Value{}},
						ObjectTypes: []string{bundle.TypeKeySet.URL()},
					},
				},
			},
		}

		// collection widget
		widgetSnapshot := &common.Snapshot{
			Id:     "widgetID",
			SbType: smartblock2.SmartBlockTypeWidget,
			Snapshot: &pb.ChangeSnapshot{
				Data: &model.SmartBlockSnapshotBase{
					Blocks: []*model.Block{
						{
							Id: "widgetID",
							Content: &model.BlockContentOfLink{
								Link: &model.BlockContentLink{
									TargetBlockId: widget.DefaultWidgetCollection,
								},
							},
						},
					},
				},
			},
		}

		// when
		collection, err := p.ProvideCollection(&snapshotSet{List: allSnapshot, Widget: widgetSnapshot}, nil, params, false)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.Len(t, collection, 1)
		rootCollectionState := state.NewDocFromSnapshot("", collection[0].Snapshot).(*state.State)
		objectsInCollection := rootCollectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, objectsInCollection, 1)
		assert.Equal(t, objectsInCollection[0], "id4")
	})
	t.Run("there are favorites objects, dashboard and objects in widget - favorites, objects in widget, dashboard in Protobuf Import", func(t *testing.T) {
		// given
		p := SpaceImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		allSnapshot := []*common.Snapshot{
			// skip object
			{
				Id:     "id2",
				SbType: smartblock2.SmartBlockTypeSubObject,
			},
			{
				Id:     "id3",
				SbType: smartblock2.SmartBlockTypeTemplate,
			},
			// favorite page
			{
				Id:     "id1",
				SbType: smartblock2.SmartBlockTypePage,
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details: &types.Struct{Fields: map[string]*types.Value{
							bundle.RelationKeyIsFavorite.String(): pbtypes.Bool(true),
						}},
						ObjectTypes: []string{bundle.TypeKeyPage.URL()},
					},
				},
			},
			// collection
			{
				Id:     "id4",
				SbType: smartblock2.SmartBlockTypePage,
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details:     &types.Struct{Fields: map[string]*types.Value{}},
						ObjectTypes: []string{bundle.TypeKeyCollection.URL()},
					},
				},
			},
			// set
			{
				Id:     "id5",
				SbType: smartblock2.SmartBlockTypePage,
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details:     &types.Struct{Fields: map[string]*types.Value{}},
						ObjectTypes: []string{bundle.TypeKeySet.URL()},
					},
				},
			},
			// dashboard
			{
				Id:     "id6",
				SbType: smartblock2.SmartBlockTypeWorkspace,
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details: &types.Struct{Fields: map[string]*types.Value{
							bundle.RelationKeySpaceDashboardId.String(): pbtypes.String("spaceDashboardId"),
						}},
						ObjectTypes: []string{bundle.TypeKeyDashboard.URL()},
					},
				},
			},
		}

		// object with widget
		widgetSnapshot := &common.Snapshot{
			Id:     "widgetID",
			SbType: smartblock2.SmartBlockTypeWidget,
			Snapshot: &pb.ChangeSnapshot{
				Data: &model.SmartBlockSnapshotBase{
					Blocks: []*model.Block{
						{
							Id: "widgetID",
							Content: &model.BlockContentOfLink{
								Link: &model.BlockContentLink{
									TargetBlockId: "oldObjectInWidget",
								},
							},
						},
					},
				},
			},
		}

		// when
		collection, err := p.ProvideCollection(&snapshotSet{List: allSnapshot, Widget: widgetSnapshot},
			map[string]string{"oldObjectInWidget": "newObjectInWidget"}, params, false)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.Len(t, collection, 1)
		rootCollectionState := state.NewDocFromSnapshot("", collection[0].Snapshot).(*state.State)
		objectsInCollection := rootCollectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, objectsInCollection, 3)
		assert.Equal(t, objectsInCollection[0], "newObjectInWidget")
		assert.Equal(t, objectsInCollection[1], "id1")
		assert.Equal(t, objectsInCollection[2], "spaceDashboardId")
	})
}
