package list

import (
	"context"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/internal/object"
	"github.com/anyproto/anytype-heart/core/api/internal/space"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service/mock_service"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	mockedSpaceId     = "mocked-space-id"
	mockedListId      = "mocked-list-id"
	mockedTypeId      = "mocked-type-id"
	mockedSetOfTypeId = "mocked-set-of-type-id"
	mockedUniqueKey   = "mocked-unique-key"
	mockedViewId      = "view-1"
	offset            = 0
	limit             = 100
)

type fixture struct {
	*ListService
	mwMock        *mock_service.MockClientCommandsServer
	objectService *object.ObjectService
}

func newFixture(t *testing.T) *fixture {
	mw := mock_service.NewMockClientCommandsServer(t)
	spaceService := space.NewService(mw)
	objSvc := object.NewService(mw, spaceService)
	objSvc.AccountInfo = &model.AccountInfo{
		TechSpaceId: "mocked-tech-space-id",
		GatewayUrl:  "http://localhost:31006",
	}
	listSvc := NewService(mw, objSvc)
	return &fixture{
		ListService:   listSvc,
		mwMock:        mw,
		objectService: objSvc,
	}
}

func TestListService_GetListViews(t *testing.T) {
	ctx := context.Background()

	t.Run("successful", func(t *testing.T) {
		fx := newFixture(t)

		// Prepare a view with one sort and one filter
		sorts := []*model.BlockContentDataviewSort{
			{
				Id:          "sort-1",
				RelationKey: "dummy-sort",
				Type:        model.BlockContentDataviewSort_Asc,
			},
		}
		filters := []*model.BlockContentDataviewFilter{
			{
				Id:          "filter-1",
				RelationKey: "dummy-filter",
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String("dummy-value"),
			},
		}
		view := &model.BlockContentDataviewView{
			Id:      "view-1",
			Name:    "Test View",
			Sorts:   sorts,
			Filters: filters,
			Type:    model.BlockContentDataviewView_Table,
		}

		resp := &pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Blocks: []*model.Block{
					{
						Id: "dataview",
						Content: &model.BlockContentOfDataview{
							Dataview: &model.BlockContentDataview{
								Views: []*model.BlockContentDataviewView{view},
							},
						},
					},
				},
			},
		}

		fx.mwMock.
			On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
				SpaceId:  mockedSpaceId,
				ObjectId: mockedListId,
			}).
			Return(resp, nil).Once()

		views, total, hasMore, err := fx.GetListViews(ctx, mockedSpaceId, mockedListId, offset, limit)
		require.NoError(t, err)
		require.Len(t, views, 1)
		require.Equal(t, 1, total)
		require.False(t, hasMore)

		retView := views[0]
		require.Equal(t, "view-1", retView.Id)
		require.Equal(t, "Test View", retView.Name)
		require.Len(t, retView.Filters, 1)
		require.Len(t, retView.Sorts, 1)
	})

	t.Run("object show error", func(t *testing.T) {
		fx := newFixture(t)

		fx.mwMock.
			On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
				SpaceId:  mockedSpaceId,
				ObjectId: mockedListId,
			}).
			Return(&pb.RpcObjectShowResponse{
				Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_UNKNOWN_ERROR},
			}, nil).Once()

		_, _, _, err := fx.GetListViews(ctx, mockedSpaceId, mockedListId, offset, limit)
		require.ErrorIs(t, err, ErrFailedGetList)
	})

	t.Run("no dataview block", func(t *testing.T) {
		fx := newFixture(t)

		fx.mwMock.
			On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
				SpaceId:  mockedSpaceId,
				ObjectId: mockedListId,
			}).
			Return(&pb.RpcObjectShowResponse{
				Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
				ObjectView: &model.ObjectView{
					Blocks: []*model.Block{
						{Id: "non-dataview"},
					},
				},
			}, nil).Once()

		_, _, _, err := fx.GetListViews(ctx, mockedSpaceId, mockedListId, offset, limit)
		require.ErrorIs(t, err, ErrFailedGetListDataview)
	})

	t.Run("invalid dataview content", func(t *testing.T) {
		fx := newFixture(t)

		fx.mwMock.
			On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
				SpaceId:  mockedSpaceId,
				ObjectId: mockedListId,
			}).
			Return(&pb.RpcObjectShowResponse{
				Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
				ObjectView: &model.ObjectView{
					Blocks: []*model.Block{
						{Id: "dataview", Content: nil},
					},
				},
			}, nil).Once()

		_, _, _, err := fx.GetListViews(ctx, mockedSpaceId, mockedListId, offset, limit)
		require.ErrorIs(t, err, ErrFailedGetListDataview)
	})

	t.Run("view with no sorts", func(t *testing.T) {
		fx := newFixture(t)

		// Create a view with filters but no sorts
		filters := []*model.BlockContentDataviewFilter{
			{
				Id:          "filter-1",
				RelationKey: "dummy-filter",
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String("dummy-value"),
			},
		}
		view := &model.BlockContentDataviewView{
			Id:      "view-2",
			Name:    "No Sort View",
			Sorts:   []*model.BlockContentDataviewSort{},
			Filters: filters,
			Type:    model.BlockContentDataviewView_Table,
		}

		resp := &pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Blocks: []*model.Block{
					{
						Id: "dataview",
						Content: &model.BlockContentOfDataview{
							Dataview: &model.BlockContentDataview{
								Views: []*model.BlockContentDataviewView{view},
							},
						},
					},
				},
			},
		}

		fx.mwMock.
			On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
				SpaceId:  mockedSpaceId,
				ObjectId: mockedListId,
			}).
			Return(resp, nil).Once()

		views, total, hasMore, err := fx.GetListViews(ctx, mockedSpaceId, mockedListId, offset, limit)
		require.NoError(t, err)
		require.Len(t, views, 1)
		require.Equal(t, 1, total)
		require.False(t, hasMore)
	})

	t.Run("view with multiple sorts", func(t *testing.T) {
		fx := newFixture(t)

		// Create a view with 2 sorts
		sorts := []*model.BlockContentDataviewSort{
			{
				Id:          "sort-1",
				RelationKey: "dummy-sort",
				Type:        model.BlockContentDataviewSort_Asc,
			},
			{
				Id:          "sort-2",
				RelationKey: "dummy-sort2",
				Type:        model.BlockContentDataviewSort_Desc,
			},
		}
		filters := []*model.BlockContentDataviewFilter{
			{
				Id:          "filter-1",
				RelationKey: "dummy-filter",
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String("dummy-value"),
			},
		}
		view := &model.BlockContentDataviewView{
			Id:      "view-3",
			Name:    "Multi-Sort View",
			Sorts:   sorts,
			Filters: filters,
			Type:    model.BlockContentDataviewView_Table,
		}

		resp := &pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Blocks: []*model.Block{
					{
						Id: "dataview",
						Content: &model.BlockContentOfDataview{
							Dataview: &model.BlockContentDataview{
								Views: []*model.BlockContentDataviewView{view},
							},
						},
					},
				},
			},
		}

		fx.mwMock.
			On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
				SpaceId:  mockedSpaceId,
				ObjectId: mockedListId,
			}).
			Return(resp, nil).Once()

		views, total, hasMore, err := fx.GetListViews(ctx, mockedSpaceId, mockedListId, offset, limit)
		require.NoError(t, err)
		require.Len(t, views, 1)
		require.Equal(t, 1, total)
		require.False(t, hasMore)

		require.Equal(t, "view-3", views[0].Id)
		require.Len(t, views[0].Sorts, 2)
	})
}

func TestListService_GetObjectsInList(t *testing.T) {

	t.Run("successful", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// Prepare a dataview view with dummy sorts and filters.
		sorts := []*model.BlockContentDataviewSort{
			{
				RelationKey: "dummy-sort",
				Type:        model.BlockContentDataviewSort_Asc,
			},
		}
		filters := []*model.BlockContentDataviewFilter{
			{
				RelationKey: "dummy-filter",
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String("dummy-value"),
			},
		}
		view := &model.BlockContentDataviewView{
			Id:      mockedViewId,
			Sorts:   sorts,
			Filters: filters,
		}

		// Expect the ObjectShow call for the list to return the type in the details and dataview block.
		fx.mwMock.
			On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
				SpaceId:  mockedSpaceId,
				ObjectId: mockedListId,
			}).
			Return(&pb.RpcObjectShowResponse{
				Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
				ObjectView: &model.ObjectView{
					Details: []*model.ObjectViewDetailsSet{
						{
							Id: mockedListId,
							Details: &types.Struct{
								Fields: map[string]*types.Value{
									bundle.RelationKeyType.String():  pbtypes.String(mockedTypeId),
									bundle.RelationKeySetOf.String(): pbtypes.StringList([]string{mockedSetOfTypeId}),
								},
							},
						},
						{
							Id: mockedTypeId,
							Details: &types.Struct{
								Fields: map[string]*types.Value{
									bundle.RelationKeyRecommendedLayout.String(): pbtypes.Int64(int64(model.ObjectType_set)),
								},
							},
						},
					},
					Blocks: []*model.Block{
						{
							Id: "dataview",
							Content: &model.BlockContentOfDataview{
								Dataview: &model.BlockContentDataview{
									Views: []*model.BlockContentDataviewView{view},
								},
							},
						},
					},
				},
			}, nil).Once()

		// Expect the ObjectShow to return the type's unique key.
		fx.mwMock.
			On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
				SpaceId:  mockedSpaceId,
				ObjectId: mockedSetOfTypeId,
			}).
			Return(&pb.RpcObjectShowResponse{
				Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
				ObjectView: &model.ObjectView{
					Details: []*model.ObjectViewDetailsSet{
						{
							Id: mockedSetOfTypeId,
							Details: &types.Struct{
								Fields: map[string]*types.Value{
									bundle.RelationKeyUniqueKey.String(): pbtypes.String(mockedUniqueKey),
								},
							},
						},
					},
				},
			}, nil).Once()

		// Expect the ObjectSearchSubscribe call to return one record.
		fx.mwMock.
			On("ObjectSearchSubscribe", mock.Anything, &pb.RpcObjectSearchSubscribeRequest{
				SpaceId: mockedSpaceId,
				Limit:   int64(limit),
				Offset:  int64(offset),
				Keys:    []string{bundle.RelationKeyId.String()},
				Sorts:   sorts,
				Filters: filters,
				Source:  []string{mockedUniqueKey},
			}).
			Return(&pb.RpcObjectSearchSubscribeResponse{
				Error:    &pb.RpcObjectSearchSubscribeResponseError{Code: pb.RpcObjectSearchSubscribeResponseError_NULL},
				Counters: &pb.EventObjectSubscriptionCounters{Total: 1},
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							bundle.RelationKeyId.String(): pbtypes.String("object-1"),
						},
					},
				},
			}, nil).Once()

		// Expect the object service to be called to get details for "object-1".
		fx.mwMock.
			On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
				SpaceId:  mockedSpaceId,
				ObjectId: "object-1",
			}).
			Return(&pb.RpcObjectShowResponse{
				Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
				ObjectView: &model.ObjectView{
					Details: []*model.ObjectViewDetailsSet{
						{
							Id: "object-1",
							Details: &types.Struct{
								Fields: map[string]*types.Value{
									bundle.RelationKeyId.String():   pbtypes.String("object-1"),
									bundle.RelationKeyName.String(): pbtypes.String("Object One"),
								},
							},
						},
					},
				},
			}, nil).Once()

		// when
		objects, total, hasMore, err := fx.GetObjectsInList(ctx, mockedSpaceId, mockedListId, "", offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, objects, 1)
		require.Equal(t, 1, total)
		require.False(t, hasMore)
		require.Equal(t, "object-1", objects[0].Id)
		require.Equal(t, "Object One", objects[0].Name)
	})

	t.Run("object show error", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// Simulate an error response (non-NULL error code) from ObjectShow for the list.
		fx.mwMock.
			On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
				SpaceId:  mockedSpaceId,
				ObjectId: mockedListId,
			}).
			Return(&pb.RpcObjectShowResponse{
				Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_UNKNOWN_ERROR},
			}, nil).Once()

		// when
		_, _, _, err := fx.GetObjectsInList(ctx, mockedSpaceId, mockedListId, "", offset, limit)

		// then
		require.ErrorIs(t, err, ErrFailedGetList)
	})

	t.Run("no dataview block", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// Return an ObjectView that does not contain a block with ID "dataview".
		fx.mwMock.
			On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
				SpaceId:  mockedSpaceId,
				ObjectId: mockedListId,
			}).
			Return(&pb.RpcObjectShowResponse{
				Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
				ObjectView: &model.ObjectView{
					Blocks: []*model.Block{
						{Id: "non-dataview"},
					},
				},
			}, nil).Once()

		// when
		_, _, _, err := fx.GetObjectsInList(ctx, mockedSpaceId, mockedListId, "", offset, limit)

		// then
		require.ErrorIs(t, err, ErrFailedGetListDataview)
	})

	t.Run("invalid dataview content", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// Return a "dataview" block that does not have the expected content type.
		fx.mwMock.
			On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
				SpaceId:  mockedSpaceId,
				ObjectId: mockedListId,
			}).
			Return(&pb.RpcObjectShowResponse{
				Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
				ObjectView: &model.ObjectView{
					Blocks: []*model.Block{
						{
							Id:      "dataview",
							Content: nil,
						},
					},
				},
			}, nil).Once()

		// when
		_, _, _, err := fx.GetObjectsInList(ctx, mockedSpaceId, mockedListId, "", offset, limit)

		// then
		require.ErrorIs(t, err, ErrFailedGetListDataview)
	})

	t.Run("view not found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// Prepare a dataview that only contains a view with an ID different from the one requested.
		view := &model.BlockContentDataviewView{
			Id:      "some-other-view",
			Sorts:   []*model.BlockContentDataviewSort{},
			Filters: []*model.BlockContentDataviewFilter{},
		}

		fx.mwMock.
			On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
				SpaceId:  mockedSpaceId,
				ObjectId: mockedListId,
			}).
			Return(&pb.RpcObjectShowResponse{
				Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
				ObjectView: &model.ObjectView{
					Blocks: []*model.Block{
						{
							Id: "dataview",
							Content: &model.BlockContentOfDataview{
								Dataview: &model.BlockContentDataview{
									Views: []*model.BlockContentDataviewView{view},
								},
							},
						},
					},
				},
			}, nil).Once()

		// when
		_, _, _, err := fx.GetObjectsInList(ctx, mockedSpaceId, mockedListId, "non-existent-view", offset, limit)

		// then
		require.ErrorIs(t, err, ErrFailedGetListDataviewView)
	})

	t.Run("search subscribe error", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// Prepare an empty dataview view (no sorts/filters).
		sorts := []*model.BlockContentDataviewSort{}
		filters := []*model.BlockContentDataviewFilter{}
		view := &model.BlockContentDataviewView{
			Id:      mockedViewId,
			Sorts:   sorts,
			Filters: filters,
		}

		fx.mwMock.
			On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
				SpaceId:  mockedSpaceId,
				ObjectId: mockedListId,
			}).
			Return(&pb.RpcObjectShowResponse{
				Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
				ObjectView: &model.ObjectView{
					Details: []*model.ObjectViewDetailsSet{
						{
							Id: mockedListId,
							Details: &types.Struct{
								Fields: map[string]*types.Value{
									bundle.RelationKeyType.String():  pbtypes.String(mockedTypeId),
									bundle.RelationKeySetOf.String(): pbtypes.StringList([]string{mockedSetOfTypeId}),
								},
							},
						},
						{
							Id: mockedTypeId,
							Details: &types.Struct{
								Fields: map[string]*types.Value{
									bundle.RelationKeyRecommendedLayout.String(): pbtypes.Int64(int64(model.ObjectType_collection)),
								},
							},
						},
					},
					Blocks: []*model.Block{
						{
							Id: "dataview",
							Content: &model.BlockContentOfDataview{
								Dataview: &model.BlockContentDataview{
									Views: []*model.BlockContentDataviewView{view},
								},
							},
						},
					},
				},
			}, nil).Once()

		// Simulate an error from ObjectSearchSubscribe.
		fx.mwMock.
			On("ObjectSearchSubscribe", mock.Anything, &pb.RpcObjectSearchSubscribeRequest{
				SpaceId:      mockedSpaceId,
				Limit:        int64(limit),
				Offset:       int64(offset),
				Keys:         []string{bundle.RelationKeyId.String()},
				Sorts:        sorts,
				Filters:      filters,
				CollectionId: mockedListId,
			}).
			Return(&pb.RpcObjectSearchSubscribeResponse{
				Error: &pb.RpcObjectSearchSubscribeResponseError{Code: pb.RpcObjectSearchSubscribeResponseError_UNKNOWN_ERROR},
			}, nil).Once()

		// when
		_, _, _, err := fx.GetObjectsInList(ctx, mockedSpaceId, mockedListId, "", offset, limit)

		// then
		require.ErrorIs(t, err, ErrFailedGetObjectsInList)
	})

	t.Run("get object error", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		var sorts []*model.BlockContentDataviewSort
		var filters []*model.BlockContentDataviewFilter
		view := &model.BlockContentDataviewView{
			Id:      mockedViewId,
			Sorts:   sorts,
			Filters: filters,
		}

		fx.mwMock.
			On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
				SpaceId:  mockedSpaceId,
				ObjectId: mockedListId,
			}).
			Return(&pb.RpcObjectShowResponse{
				Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
				ObjectView: &model.ObjectView{
					Details: []*model.ObjectViewDetailsSet{
						{
							Id: mockedListId,
							Details: &types.Struct{
								Fields: map[string]*types.Value{
									bundle.RelationKeyType.String():  pbtypes.String(mockedTypeId),
									bundle.RelationKeySetOf.String(): pbtypes.StringList([]string{mockedSetOfTypeId}),
								},
							},
						},
						{
							Id: mockedTypeId,
							Details: &types.Struct{
								Fields: map[string]*types.Value{
									bundle.RelationKeyRecommendedLayout.String(): pbtypes.Int64(int64(model.ObjectType_collection)),
								},
							},
						},
					},
					Blocks: []*model.Block{
						{
							Id: "dataview",
							Content: &model.BlockContentOfDataview{
								Dataview: &model.BlockContentDataview{
									Views: []*model.BlockContentDataviewView{view},
								},
							},
						},
					},
				},
			}, nil).Once()

		fx.mwMock.
			On("ObjectSearchSubscribe", mock.Anything, &pb.RpcObjectSearchSubscribeRequest{
				SpaceId:      mockedSpaceId,
				Limit:        int64(limit),
				Offset:       int64(offset),
				Keys:         []string{bundle.RelationKeyId.String()},
				Sorts:        sorts,
				Filters:      filters,
				CollectionId: mockedListId,
			}).
			Return(&pb.RpcObjectSearchSubscribeResponse{
				Error:    &pb.RpcObjectSearchSubscribeResponseError{Code: pb.RpcObjectSearchSubscribeResponseError_NULL},
				Counters: &pb.EventObjectSubscriptionCounters{Total: 1},
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							bundle.RelationKeyId.String(): pbtypes.String("object-err"),
						},
					},
				},
			}, nil).Once()

		// Simulate an error when trying to retrieve the object details.
		fx.mwMock.
			On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
				SpaceId:  mockedSpaceId,
				ObjectId: "object-err",
			}).
			Return(&pb.RpcObjectShowResponse{
				Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NOT_FOUND},
			}, nil).Once()

		// when
		_, _, _, err := fx.GetObjectsInList(ctx, mockedSpaceId, mockedListId, "", offset, limit)

		// then
		require.ErrorIs(t, err, object.ErrObjectNotFound)
	})
}

func TestListService_AddObjectsToList(t *testing.T) {

	t.Run("success", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		objectIds := []string{"obj-1", "obj-2"}

		fx.mwMock.
			On("ObjectCollectionAdd", mock.Anything, &pb.RpcObjectCollectionAddRequest{
				ContextId: mockedListId,
				ObjectIds: objectIds,
			}).
			Return(&pb.RpcObjectCollectionAddResponse{
				Error: &pb.RpcObjectCollectionAddResponseError{Code: pb.RpcObjectCollectionAddResponseError_NULL},
			}, nil).Once()

		// when
		err := fx.AddObjectsToList(ctx, mockedSpaceId, mockedListId, objectIds)

		// then
		require.NoError(t, err)
	})

	t.Run("failure", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		objectIds := []string{"obj-1"}

		fx.mwMock.
			On("ObjectCollectionAdd", mock.Anything, &pb.RpcObjectCollectionAddRequest{
				ContextId: mockedListId,
				ObjectIds: objectIds,
			}).
			Return(&pb.RpcObjectCollectionAddResponse{
				Error: &pb.RpcObjectCollectionAddResponseError{Code: pb.RpcObjectCollectionAddResponseError_UNKNOWN_ERROR},
			}, nil).Once()

		// when
		err := fx.AddObjectsToList(ctx, mockedSpaceId, mockedListId, objectIds)

		// then
		require.ErrorIs(t, err, ErrFailedAddObjectsToList)
	})
}

func TestListService_RemoveObjectsFromList(t *testing.T) {

	t.Run("success", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		objectIds := []string{"obj-1", "obj-2"}

		fx.mwMock.
			On("ObjectCollectionRemove", mock.Anything, &pb.RpcObjectCollectionRemoveRequest{
				ContextId: mockedListId,
				ObjectIds: objectIds,
			}).
			Return(&pb.RpcObjectCollectionRemoveResponse{
				Error: &pb.RpcObjectCollectionRemoveResponseError{Code: pb.RpcObjectCollectionRemoveResponseError_NULL},
			}, nil).Once()

		// when
		err := fx.RemoveObjectsFromList(ctx, mockedSpaceId, mockedListId, objectIds)

		// then
		require.NoError(t, err)
	})

	t.Run("failure", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		objectIds := []string{"obj-1"}

		fx.mwMock.
			On("ObjectCollectionRemove", mock.Anything, &pb.RpcObjectCollectionRemoveRequest{
				ContextId: mockedListId,
				ObjectIds: objectIds,
			}).
			Return(&pb.RpcObjectCollectionRemoveResponse{
				Error: &pb.RpcObjectCollectionRemoveResponseError{Code: pb.RpcObjectCollectionRemoveResponseError_UNKNOWN_ERROR},
			}, nil).Once()

		// when
		err := fx.RemoveObjectsFromList(ctx, mockedSpaceId, mockedListId, objectIds)

		// then
		require.ErrorIs(t, err, ErrFailedRemoveObjectsFromList)
	})
}
