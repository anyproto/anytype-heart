package pb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	smartblock2 "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
		rootCollectionState, err := state.NewDocFromSnapshot("", collection[0].Snapshot.ToProto())
		require.NoError(t, err)
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
	t.Run("no widget object - add all objects (except subobjects) in Protobuf Import collection", func(t *testing.T) {
		// given
		p := SpaceImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		allSnapshot := []*common.Snapshot{
			{
				Id: "id1",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock2.SmartBlockTypePage,
				},
			},
			{
				Id: "id2",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock2.SmartBlockTypeSubObject,
				},
			},
			{
				Id: "id3",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock2.SmartBlockTypeTemplate,
				},
			},
			{
				Id: "id4",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock2.SmartBlockTypePage,
				},
			},
		}
		snapshotList := common.NewSnapshotContext().Add(allSnapshot...)

		// when
		collection, err := p.ProvideCollection(snapshotList, nil, params, false)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.Len(t, collection, 1)
		rootCollectionState, err := state.NewDocFromSnapshot("", collection[0].Snapshot.ToProto())
		require.NoError(t, err)
		objectsInCollection := rootCollectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, objectsInCollection, 3)
		assert.Equal(t, objectsInCollection[0], "id1")
		assert.Equal(t, objectsInCollection[1], "id3")
		assert.Equal(t, objectsInCollection[2], "id4")
	})
	t.Run("widget with sets - add only sets in Protobuf Import collection", func(t *testing.T) {
		// given
		p := SpaceImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		allSnapshot := []*common.Snapshot{
			// skip objects
			{
				Id: "id2",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock2.SmartBlockTypeSubObject,
				},
			},
			{
				Id: "id3",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock2.SmartBlockTypeTemplate,
					Data: &common.StateSnapshot{
						ObjectTypes: []string{bundle.TypeKeyTemplate.URL()},
					},
				},
			},
			// page
			{
				Id: "id1",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock2.SmartBlockTypePage,
					Data: &common.StateSnapshot{
						Details:     domain.NewDetails(),
						ObjectTypes: []string{bundle.TypeKeyPage.URL()},
					},
				},
			},
			// collection
			{
				Id: "id4",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock2.SmartBlockTypePage,
					Data: &common.StateSnapshot{
						Details:     domain.NewDetails(),
						ObjectTypes: []string{bundle.TypeKeyCollection.URL()},
					},
				},
			},
			// set
			{
				Id: "id5",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock2.SmartBlockTypePage,
					Data: &common.StateSnapshot{
						Details:     domain.NewDetails(),
						ObjectTypes: []string{bundle.TypeKeySet.URL()},
					},
				},
			},
		}
		// set widget
		widgetSnapshot := &common.Snapshot{
			Id: "widgetID",
			Snapshot: &common.SnapshotModel{
				SbType: smartblock2.SmartBlockTypeWidget,
				Data: &common.StateSnapshot{
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
		snapshotList := common.NewSnapshotContext().Add(allSnapshot...).SetWidget(widgetSnapshot)

		// when
		collection, err := p.ProvideCollection(snapshotList, nil, params, false)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.Len(t, collection, 1)
		rootCollectionState, err := state.NewDocFromSnapshot("", collection[0].Snapshot.ToProto())
		require.NoError(t, err)
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
				Id: "id2",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock2.SmartBlockTypeSubObject,
				},
			},
			{
				Id: "id3",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock2.SmartBlockTypeTemplate,
					Data: &common.StateSnapshot{
						ObjectTypes: []string{bundle.TypeKeyTemplate.URL()},
					},
				},
			},
			// page
			{
				Id: "id1",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock2.SmartBlockTypePage,
					Data: &common.StateSnapshot{
						Details:     domain.NewDetails(),
						ObjectTypes: []string{bundle.TypeKeyPage.URL()},
					},
				},
			},
			// collection
			{
				Id: "id4",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock2.SmartBlockTypePage,
					Data: &common.StateSnapshot{
						Details:     domain.NewDetails(),
						ObjectTypes: []string{bundle.TypeKeyCollection.URL()},
					},
				},
			},
			// set
			{
				Id: "id5",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock2.SmartBlockTypePage,
					Data: &common.StateSnapshot{
						Details:     domain.NewDetails(),
						ObjectTypes: []string{bundle.TypeKeySet.URL()},
					},
				},
			},
		}

		// collection widget
		widgetSnapshot := &common.Snapshot{
			Id: "widgetID",
			Snapshot: &common.SnapshotModel{
				SbType: smartblock2.SmartBlockTypeWidget,
				Data: &common.StateSnapshot{
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
		snapshotList := common.NewSnapshotContext().Add(allSnapshot...).SetWidget(widgetSnapshot)

		// when
		collection, err := p.ProvideCollection(snapshotList, nil, params, false)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.Len(t, collection, 1)
		rootCollectionState, err := state.NewDocFromSnapshot("", collection[0].Snapshot.ToProto())
		require.NoError(t, err)
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
				Id: "id2",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock2.SmartBlockTypeSubObject,
				},
			},
			{
				Id: "id3",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock2.SmartBlockTypeTemplate,
					Data: &common.StateSnapshot{
						ObjectTypes: []string{bundle.TypeKeyTemplate.URL()},
					},
				},
			},
			// favorite page
			{
				Id: "id1",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock2.SmartBlockTypePage,
					Data: &common.StateSnapshot{
						Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
							bundle.RelationKeyIsFavorite: domain.Bool(true),
						}),
						ObjectTypes: []string{bundle.TypeKeyPage.URL()},
					},
				},
			},
			// collection
			{
				Id: "id4",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock2.SmartBlockTypePage,
					Data: &common.StateSnapshot{
						Details:     domain.NewDetails(),
						ObjectTypes: []string{bundle.TypeKeyCollection.URL()},
					},
				},
			},
			// set
			{
				Id: "id5",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock2.SmartBlockTypePage,
					Data: &common.StateSnapshot{
						Details:     domain.NewDetails(),
						ObjectTypes: []string{bundle.TypeKeySet.URL()},
					},
				},
			},
			// dashboard
			{
				Id: "id6",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock2.SmartBlockTypeWorkspace,
					Data: &common.StateSnapshot{
						Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
							bundle.RelationKeySpaceDashboardId: domain.StringList([]string{"spaceDashboardId"}),
						}),
						ObjectTypes: []string{bundle.TypeKeyDashboard.URL()},
					},
				},
			},
		}

		// object with widget
		widgetSnapshot := &common.Snapshot{
			Id: "widgetID",
			Snapshot: &common.SnapshotModel{
				SbType: smartblock2.SmartBlockTypeWidget,
				Data: &common.StateSnapshot{
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
		snapshotList := common.NewSnapshotContext().Add(allSnapshot...).SetWidget(widgetSnapshot)

		// when
		collection, err := p.ProvideCollection(snapshotList, map[string]string{"oldObjectInWidget": "newObjectInWidget"}, params, false)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.Len(t, collection, 1)
		rootCollectionState, err := state.NewDocFromSnapshot("", collection[0].Snapshot.ToProto())
		require.NoError(t, err)
		objectsInCollection := rootCollectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Equal(t, []string{"newObjectInWidget", "id1", "spaceDashboardId"}, objectsInCollection)
	})
}
