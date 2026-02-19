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
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestGalleryImport_ProvideCollection(t *testing.T) {
	t.Run("no widget in experience - only objects root collection", func(t *testing.T) {
		// given
		collectionProvider := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{}

		// when
		collection, err := collectionProvider.ProvideCollection(nil, nil, params, false)

		// then
		assert.NoError(t, err)
		assert.Len(t, collection, 1)
		assert.NotContains(t, widgetCollectionPattern, collection[0].FileName)
	})
	t.Run("CollectionTitle parameter is empty - collection with default name", func(t *testing.T) {
		// given
		collectionProvider := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{}

		// when
		collection, err := collectionProvider.ProvideCollection(nil, nil, params, false)

		// then
		assert.NoError(t, err)
		assert.Len(t, collection, 1)
		assert.Equal(t, rootCollectionName, collection[0].FileName)
	})
	t.Run("CollectionTitle parameter is equal 'test' - collection with name test", func(t *testing.T) {
		// given
		collectionProvider := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{CollectionTitle: "test"}

		// when
		collection, err := collectionProvider.ProvideCollection(nil, nil, params, false)

		// then
		assert.NoError(t, err)
		assert.Len(t, collection, 1)
		assert.Equal(t, "test", collection[0].FileName)
	})
	t.Run("widget with sets - only objects root collection as we ignore default sets and not create widgets collection", func(t *testing.T) {
		// given
		p := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		allSnapshot := []*common.Snapshot{
			// skip objects
			{
				Id: "id2",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock.SmartBlockTypeSubObject,
				},
			},
			{
				Id: "id3",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock.SmartBlockTypeTemplate,
				},
			},
			// page
			{
				Id: "id1",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock.SmartBlockTypePage,
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
					SbType: smartblock.SmartBlockTypePage,
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
					SbType: smartblock.SmartBlockTypePage,
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
				SbType: smartblock.SmartBlockTypeWidget,
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
		assert.NoError(t, err)
		assert.Len(t, collection, 1)
		assert.NotContains(t, widgetCollectionPattern, collection[0].FileName)
	})
	t.Run("default sets and objects in widget - root collection with objects from widget and object root collection are created", func(t *testing.T) {
		// given
		p := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		allSnapshot := []*common.Snapshot{
			// favorite page
			{
				Id: "id1",
				Snapshot: &common.SnapshotModel{
					SbType: smartblock.SmartBlockTypePage,
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
					SbType: smartblock.SmartBlockTypePage,
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
					SbType: smartblock.SmartBlockTypePage,
					Data: &common.StateSnapshot{
						Details:     domain.NewDetails(),
						ObjectTypes: []string{bundle.TypeKeySet.URL()},
					},
				},
			},
		}

		// object with widget
		widgetSnapshot := &common.Snapshot{
			Id: "widgetID",
			Snapshot: &common.SnapshotModel{
				SbType: smartblock.SmartBlockTypeWidget,
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
		collection, err := p.ProvideCollection(snapshotList, nil, params, false)

		// then
		assert.NoError(t, err)
		assert.Len(t, collection, 2)
		rootCollectionState, err := state.NewDocFromSnapshot("", collection[0].Snapshot.ToProto())
		require.NoError(t, err)
		objectsInCollection := rootCollectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, objectsInCollection, 1)
		assert.Equal(t, objectsInCollection[0], "oldObjectInWidget")
		assert.False(t, rootCollectionState.Details().GetBool(bundle.RelationKeyIsFavorite))

		rootCollectionState, err = state.NewDocFromSnapshot("", collection[1].Snapshot.ToProto())
		require.NoError(t, err)
		objectsInCollection = rootCollectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, objectsInCollection, 3)
		assert.Equal(t, objectsInCollection[0], "id1")
		assert.Equal(t, objectsInCollection[1], "id4")
		assert.Equal(t, objectsInCollection[2], "id5")
	})
	t.Run("widget is empty - only objects root collection created", func(t *testing.T) {
		// given
		p := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		// object with widget
		widgetSnapshot := &common.Snapshot{
			Id: "widgetID",
			Snapshot: &common.SnapshotModel{
				SbType: smartblock.SmartBlockTypeWidget,
				Data: &common.StateSnapshot{
					Blocks: []*model.Block{},
				},
			},
		}
		snapshotList := common.NewSnapshotContext().SetWidget(widgetSnapshot)

		// when
		collection, err := p.ProvideCollection(snapshotList, nil, params, false)

		// then
		assert.NoError(t, err)
		assert.Len(t, collection, 1)
		assert.NotContains(t, widgetCollectionPattern, collection[0].FileName)

	})
	t.Run("workspace is empty - root collection without icon", func(t *testing.T) {
		// given
		p := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		// when
		collection, err := p.ProvideCollection(nil, nil, params, false)

		// then
		assert.NoError(t, err)
		assert.Len(t, collection, 1)
		assert.Empty(t, collection[0].Snapshot.Data.Details.GetString(bundle.RelationKeyIconImage))
	})
	t.Run("workspace without icon - root collection without icon", func(t *testing.T) {
		// given
		p := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		workspace := &common.Snapshot{
			Id: "workspace",
			Snapshot: &common.SnapshotModel{
				SbType: smartblock.SmartBlockTypeWorkspace,
				Data: &common.StateSnapshot{
					Details:     domain.NewDetails(),
					ObjectTypes: []string{bundle.TypeKeyDashboard.URL()},
				},
			},
		}
		snapshotList := common.NewSnapshotContext().SetWorkspace(workspace)

		// when
		collection, err := p.ProvideCollection(snapshotList, nil, params, false)

		// then
		assert.NoError(t, err)
		assert.Len(t, collection, 1)
		assert.Empty(t, collection[0].Snapshot.Data.Details.GetString(bundle.RelationKeyIconImage))
	})
	t.Run("workspace with icon - root collection with icon", func(t *testing.T) {
		// given
		p := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		workspace := &common.Snapshot{
			Id: "workspace",
			Snapshot: &common.SnapshotModel{
				SbType: smartblock.SmartBlockTypeWorkspace,
				Data: &common.StateSnapshot{
					Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
						bundle.RelationKeyIconImage: domain.String("icon"),
					}),
					ObjectTypes: []string{bundle.TypeKeyDashboard.URL()},
				},
			},
		}
		snapshotList := common.NewSnapshotContext().SetWorkspace(workspace)

		// when
		collection, err := p.ProvideCollection(snapshotList, nil, params, false)

		// then
		assert.NoError(t, err)
		assert.Len(t, collection, 1)
		assert.Equal(t, "icon", collection[0].Snapshot.Data.Details.GetString(bundle.RelationKeyIconImage))
	})
	t.Run("if import in new space - not create anything", func(t *testing.T) {
		// given
		p := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		// when
		collection, err := p.ProvideCollection(nil, nil, params, true)

		// then
		assert.NoError(t, err)
		assert.Nil(t, collection)
	})

	t.Run("widget has only deleted objects - not create widget collection", func(t *testing.T) {
		// given
		p := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		// object with widget
		widgetSnapshot := &common.Snapshot{
			Id: "widgetID",
			Snapshot: &common.SnapshotModel{
				SbType: smartblock.SmartBlockTypeWidget,
				Data: &common.StateSnapshot{
					Blocks: []*model.Block{
						{
							Id: "widgetID",
							Content: &model.BlockContentOfLink{
								Link: &model.BlockContentLink{
									TargetBlockId: addr.MissingObject,
								},
							},
						},
					},
				},
			},
		}
		snapshotList := common.NewSnapshotContext().SetWidget(widgetSnapshot)

		// when
		collection, err := p.ProvideCollection(snapshotList, nil, params, false)

		// then
		assert.NoError(t, err)
		assert.Len(t, collection, 1)
		assert.NotContains(t, widgetCollectionPattern, collection[0].FileName)
	})
}
