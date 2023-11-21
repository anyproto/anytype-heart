package pb

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestGalleryImport_ProvideCollection(t *testing.T) {
	t.Run("no widget in experience - root collection without objects", func(t *testing.T) {
		// given
		collectionProvider := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{}

		// when
		collection, err := collectionProvider.ProvideCollection(nil, nil, nil, params, nil)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		rootCollectionState := state.NewDocFromSnapshot("", collection.Snapshot).(*state.State)
		objectsInCollection := rootCollectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, objectsInCollection, 0)
	})
	t.Run("CollectionTitle parameter is empty - collection with default name", func(t *testing.T) {
		// given
		collectionProvider := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{}

		// when
		collection, err := collectionProvider.ProvideCollection(nil, nil, nil, params, nil)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.Equal(t, rootCollectionName, collection.FileName)
	})
	t.Run("CollectionTitle parameter is equeal 'test' - collection with name test", func(t *testing.T) {
		// given
		collectionProvider := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{CollectionTitle: "test"}

		// when
		collection, err := collectionProvider.ProvideCollection(nil, nil, nil, params, nil)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.Equal(t, "test", collection.FileName)
	})
	t.Run("widget with sets - root collection without objects as we ignore default sets", func(t *testing.T) {
		// given
		p := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		allSnapshot := []*converter.Snapshot{
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
		widgetSnapshot := &converter.Snapshot{
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
		collection, err := p.ProvideCollection(allSnapshot, widgetSnapshot, nil, params, nil)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		rootCollectionState := state.NewDocFromSnapshot("", collection.Snapshot).(*state.State)
		objectsInCollection := rootCollectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, objectsInCollection, 0)
	})
	t.Run("default sets and objects in widget - root collection with objects from widget", func(t *testing.T) {
		// given
		p := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		allSnapshot := []*converter.Snapshot{
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
		widgetSnapshot := &converter.Snapshot{
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
		collection, err := p.ProvideCollection(allSnapshot, widgetSnapshot, nil, params, nil)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		rootCollectionState := state.NewDocFromSnapshot("", collection.Snapshot).(*state.State)
		objectsInCollection := rootCollectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, objectsInCollection, 1)
		assert.Equal(t, objectsInCollection[0], "oldObjectInWidget")
	})
	t.Run("widget is empty - root collection without objects", func(t *testing.T) {
		// given
		p := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		// object with widget
		widgetSnapshot := &converter.Snapshot{
			Id:     "widgetID",
			SbType: smartblock.SmartBlockTypeWidget,
			Snapshot: &pb.ChangeSnapshot{
				Data: &model.SmartBlockSnapshotBase{
					Blocks: []*model.Block{},
				},
			},
		}

		// when
		collection, err := p.ProvideCollection(nil, widgetSnapshot, nil, params, nil)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		rootCollectionState := state.NewDocFromSnapshot("", collection.Snapshot).(*state.State)
		objectsInCollection := rootCollectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, objectsInCollection, 0)
	})
	t.Run("workspace is empty - root collection without icon", func(t *testing.T) {
		// given
		p := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		// when
		collection, err := p.ProvideCollection(nil, nil, nil, params, nil)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.Empty(t, pbtypes.GetString(collection.Snapshot.Data.Details, bundle.RelationKeyIconImage.String()))
	})
	t.Run("workspace without icon - root collection without icon", func(t *testing.T) {
		// given
		p := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		workspace := &converter.Snapshot{
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
		collection, err := p.ProvideCollection(nil, nil, nil, params, workspace)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.Empty(t, pbtypes.GetString(collection.Snapshot.Data.Details, bundle.RelationKeyIconImage.String()))
	})
	t.Run("workspace with icon - root collection with icon", func(t *testing.T) {
		// given
		p := GalleryImport{}
		params := &pb.RpcObjectImportRequestPbParams{NoCollection: false}

		workspace := &converter.Snapshot{
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
		collection, err := p.ProvideCollection(nil, nil, nil, params, workspace)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, collection)
		assert.Equal(t, "icon", pbtypes.GetString(collection.Snapshot.Data.Details, bundle.RelationKeyIconImage.String()))
	})
}
