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
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestGalleryImport_ProvideCollection(t *testing.T) {
	t.Run("no widget in experience - only objects root collection", func(t *testing.T) {
		// given
		collectionProvider := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{}

		// when
		collection, err := collectionProvider.ProvideCollection(nil, nil, params, false)

		// then
		assert.Nil(t, err)
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
		assert.Nil(t, err)
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
		assert.Nil(t, err)
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
				Id:     "id2",
				SbType: smartblock.SmartBlockTypeSubObject,
			},
			{
				Id:     "id3",
				SbType: smartblock.SmartBlockTypeTemplate,
			},
			// page
			{
				Id:     "id1",
				SbType: smartblock.SmartBlockTypePage,
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
				SbType: smartblock.SmartBlockTypePage,
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
				SbType: smartblock.SmartBlockTypePage,
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
			SbType: smartblock.SmartBlockTypeWidget,
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
				Id:     "id1",
				SbType: smartblock.SmartBlockTypePage,
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
				SbType: smartblock.SmartBlockTypePage,
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
				SbType: smartblock.SmartBlockTypePage,
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details:     &types.Struct{Fields: map[string]*types.Value{}},
						ObjectTypes: []string{bundle.TypeKeySet.URL()},
					},
				},
			},
		}

		// object with widget
		widgetSnapshot := &common.Snapshot{
			Id:     "widgetID",
			SbType: smartblock.SmartBlockTypeWidget,
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
		collection, err := p.ProvideCollection(&snapshotSet{List: allSnapshot, Widget: widgetSnapshot}, nil, params, false)

		// then
		assert.Nil(t, err)
		assert.Len(t, collection, 2)
		rootCollectionState := state.NewDocFromSnapshot("", collection[0].Snapshot).(*state.State)
		objectsInCollection := rootCollectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, objectsInCollection, 1)
		assert.Equal(t, objectsInCollection[0], "oldObjectInWidget")
		assert.False(t, pbtypes.GetBool(rootCollectionState.Details(), bundle.RelationKeyIsFavorite.String()))

		rootCollectionState = state.NewDocFromSnapshot("", collection[1].Snapshot).(*state.State)
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
			Id:     "widgetID",
			SbType: smartblock.SmartBlockTypeWidget,
			Snapshot: &pb.ChangeSnapshot{
				Data: &model.SmartBlockSnapshotBase{
					Blocks: []*model.Block{},
				},
			},
		}

		// when
		collection, err := p.ProvideCollection(&snapshotSet{Widget: widgetSnapshot}, nil, params, false)

		// then
		assert.Nil(t, err)
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
		assert.Nil(t, err)
		assert.Len(t, collection, 1)
		assert.Empty(t, pbtypes.GetString(collection[0].Snapshot.Data.Details, bundle.RelationKeyIconImage.String()))
	})
	t.Run("workspace without icon - root collection without icon", func(t *testing.T) {
		// given
		p := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		workspace := &common.Snapshot{
			Id:     "workspace",
			SbType: smartblock.SmartBlockTypeWorkspace,
			Snapshot: &pb.ChangeSnapshot{
				Data: &model.SmartBlockSnapshotBase{
					Details:     &types.Struct{Fields: map[string]*types.Value{}},
					ObjectTypes: []string{bundle.TypeKeyDashboard.URL()},
				},
			},
		}
		// when
		collection, err := p.ProvideCollection(&snapshotSet{Workspace: workspace}, nil, params, false)

		// then
		assert.Nil(t, err)
		assert.Len(t, collection, 1)
		assert.Empty(t, pbtypes.GetString(collection[0].Snapshot.Data.Details, bundle.RelationKeyIconImage.String()))
	})
	t.Run("workspace with icon - root collection with icon", func(t *testing.T) {
		// given
		p := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		workspace := &common.Snapshot{
			Id:     "workspace",
			SbType: smartblock.SmartBlockTypeWorkspace,
			Snapshot: &pb.ChangeSnapshot{
				Data: &model.SmartBlockSnapshotBase{
					Details: &types.Struct{Fields: map[string]*types.Value{
						bundle.RelationKeyIconImage.String(): pbtypes.String("icon"),
					}},
					ObjectTypes: []string{bundle.TypeKeyDashboard.URL()},
				},
			},
		}
		// when
		collection, err := p.ProvideCollection(&snapshotSet{Workspace: workspace}, nil, params, false)

		// then
		assert.Nil(t, err)
		assert.Len(t, collection, 1)
		assert.Equal(t, "icon", pbtypes.GetString(collection[0].Snapshot.Data.Details, bundle.RelationKeyIconImage.String()))
	})
	t.Run("if import in new space - not create anything", func(t *testing.T) {
		// given
		p := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		// when
		collection, err := p.ProvideCollection(nil, nil, params, true)

		// then
		assert.Nil(t, err)
		assert.Nil(t, collection)
	})

	t.Run("widget has only deleted objects - not create widget collection", func(t *testing.T) {
		// given
		p := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		// object with widget
		widgetSnapshot := &common.Snapshot{
			Id:     "widgetID",
			SbType: smartblock.SmartBlockTypeWidget,
			Snapshot: &pb.ChangeSnapshot{
				Data: &model.SmartBlockSnapshotBase{
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

		// when
		collection, err := p.ProvideCollection(&snapshotSet{Widget: widgetSnapshot}, nil, params, false)

		// then
		assert.Nil(t, err)
		assert.Len(t, collection, 1)
		assert.NotContains(t, widgetCollectionPattern, collection[0].FileName)
	})
}
