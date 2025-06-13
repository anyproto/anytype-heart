package service

import (
	"context"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	mockedListId      = "mocked-list-id"
	mockedSetOfTypeId = "mocked-set-of-type-id"
	mockedUniqueKey   = "mocked-unique-key"
	mockedViewId      = "view-1"
)

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

		views, total, hasMore, err := fx.service.GetListViews(ctx, mockedSpaceId, mockedListId, offset, limit)
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

		_, _, _, err := fx.service.GetListViews(ctx, mockedSpaceId, mockedListId, offset, limit)
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

		_, _, _, err := fx.service.GetListViews(ctx, mockedSpaceId, mockedListId, offset, limit)
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

		_, _, _, err := fx.service.GetListViews(ctx, mockedSpaceId, mockedListId, offset, limit)
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

		views, total, hasMore, err := fx.service.GetListViews(ctx, mockedSpaceId, mockedListId, offset, limit)
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

		views, total, hasMore, err := fx.service.GetListViews(ctx, mockedSpaceId, mockedListId, offset, limit)
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

		// Mock util.GetAllRelationKeys
		fx.mwMock.
			On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
				SpaceId: mockedSpaceId,
				Filters: []*model.BlockContentDataviewFilter{
					{
						RelationKey: bundle.RelationKeyResolvedLayout.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
					},
				},
				Keys: []string{bundle.RelationKeyRelationKey.String()},
			}).
			Return(&pb.RpcObjectSearchResponse{
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							bundle.RelationKeyRelationKey.String(): pbtypes.String(bundle.RelationKeyRelationKey.String()),
						},
					},
				},
			}, nil).Once()

		// Expect the ObjectSearchSubscribe call to return one record.
		fx.mwMock.
			On("ObjectSearchSubscribe", mock.Anything, &pb.RpcObjectSearchSubscribeRequest{
				SpaceId:      mockedSpaceId,
				SubId:        subId,
				Limit:        int64(limit),
				Offset:       int64(offset),
				Sorts:        sorts,
				Filters:      filters,
				Source:       []string{mockedUniqueKey},
				Keys:         []string{bundle.RelationKeyRelationKey.String()},
				CollectionId: "",
			}).
			Return(&pb.RpcObjectSearchSubscribeResponse{
				Error:    &pb.RpcObjectSearchSubscribeResponseError{Code: pb.RpcObjectSearchSubscribeResponseError_NULL},
				Counters: &pb.EventObjectSubscriptionCounters{Total: 1},
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							bundle.RelationKeyId.String():   pbtypes.String("object-1"),
							bundle.RelationKeyName.String(): pbtypes.String("Object One"),
						},
					},
				},
			}, nil).Once()

		// Mock unsubscribe after subscription
		fx.mwMock.
			On("ObjectSearchUnsubscribe", mock.Anything, &pb.RpcObjectSearchUnsubscribeRequest{
				SubIds: []string{subId},
			}).
			Return(&pb.RpcObjectSearchUnsubscribeResponse{}, nil).Once()

		// Mock getPropertyMapsFromStore
		fx.mwMock.
			On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
				SpaceId: mockedSpaceId,
				Filters: []*model.BlockContentDataviewFilter{
					{
						RelationKey: bundle.RelationKeyResolvedLayout.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
					},
					{
						RelationKey: bundle.RelationKeyIsHidden.String(),
						Condition:   model.BlockContentDataviewFilter_NotEqual,
						Value:       pbtypes.Bool(true),
					},
				},
				Keys: []string{
					bundle.RelationKeyId.String(),
					bundle.RelationKeyRelationKey.String(),
					bundle.RelationKeyApiObjectKey.String(),
					bundle.RelationKeyName.String(),
					bundle.RelationKeyRelationFormat.String(),
				},
			}).
			Return(&pb.RpcObjectSearchResponse{
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							bundle.RelationKeyRelationKey.String():    pbtypes.String("rel-dummy"),
							bundle.RelationKeyRelationFormat.String(): pbtypes.String(model.RelationFormat_longtext.String()),
						},
					},
				},
			}, nil).Once()

		// Mock getTypeMapFromStore
		fx.mwMock.
			On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
				SpaceId: mockedSpaceId,
				Filters: []*model.BlockContentDataviewFilter{
					{
						RelationKey: bundle.RelationKeyResolvedLayout.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.Int64(int64(model.ObjectType_objectType)),
					},
					{
						RelationKey: bundle.RelationKeyIsDeleted.String(),
					},
				},
				Keys: []string{
					bundle.RelationKeyId.String(),
					bundle.RelationKeyUniqueKey.String(),
					bundle.RelationKeyApiObjectKey.String(),
					bundle.RelationKeyName.String(),
					bundle.RelationKeyPluralName.String(),
					bundle.RelationKeyIconEmoji.String(),
					bundle.RelationKeyIconName.String(),
					bundle.RelationKeyIconOption.String(),
					bundle.RelationKeyRecommendedLayout.String(),
					bundle.RelationKeyIsArchived.String(),
					bundle.RelationKeyRecommendedFeaturedRelations.String(),
					bundle.RelationKeyRecommendedRelations.String(),
				},
			}).
			Return(&pb.RpcObjectSearchResponse{
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							bundle.RelationKeyId.String():                pbtypes.String("type-1"),
							bundle.RelationKeyUniqueKey.String():         pbtypes.String("type-key"),
							bundle.RelationKeyName.String():              pbtypes.String("Type One"),
							bundle.RelationKeyIconEmoji.String():         pbtypes.String(""),
							bundle.RelationKeyIconName.String():          pbtypes.String("icon1"),
							bundle.RelationKeyIconOption.String():        pbtypes.String("option1"),
							bundle.RelationKeyRecommendedLayout.String(): pbtypes.Int64(int64(model.ObjectType_basic)),
							bundle.RelationKeyIsArchived.String():        pbtypes.Bool(false),
						},
					},
				},
			}, nil).Once()

		// Mock getTagMapFromStore
		fx.mwMock.
			On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
				SpaceId: mockedSpaceId,
				Filters: []*model.BlockContentDataviewFilter{
					{
						RelationKey: bundle.RelationKeyResolvedLayout.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.Int64(int64(model.ObjectType_relationOption)),
					},
					{
						RelationKey: bundle.RelationKeyIsHidden.String(),
						Condition:   model.BlockContentDataviewFilter_NotEqual,
						Value:       pbtypes.Bool(true),
					},
				},
				Keys: []string{
					bundle.RelationKeyId.String(),
					bundle.RelationKeyUniqueKey.String(),
					bundle.RelationKeyName.String(),
					bundle.RelationKeyRelationOptionColor.String(),
				},
			}).
			Return(&pb.RpcObjectSearchResponse{
				Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
				Records: []*types.Struct{},
			}, nil).Once()

		// when
		objects, total, hasMore, err := fx.service.GetObjectsInList(ctx, mockedSpaceId, mockedListId, mockedViewId, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, objects, 1)
		require.Equal(t, 1, total)
		require.False(t, hasMore)
		require.Equal(t, "object-1", objects[0].Id)
		require.Equal(t, "Object One", objects[0].Name)
	})

	t.Run("successful with empty viewId", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t)

		// Prepare an ObjectShow response with a dataview block containing a view (which will not be used since viewId is empty)
		resp := &pb.RpcObjectShowResponse{
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
								Views: []*model.BlockContentDataviewView{
									{
										Id: mockedListId,
										Sorts: []*model.BlockContentDataviewSort{
											{
												Id:          "view_sort",
												RelationKey: bundle.RelationKeyLastModifiedDate.String(),
												Format:      model.RelationFormat_date,
												Type:        model.BlockContentDataviewSort_Asc,
											},
										},
										Filters: []*model.BlockContentDataviewFilter{
											{
												Id:          "view_filter",
												RelationKey: bundle.RelationKeyStatus.String(),
												Format:      model.RelationFormat_longtext,
												Condition:   model.BlockContentDataviewFilter_Equal,
												Value:       pbtypes.String("active"),
											},
										},
									},
								},
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

		// Mock util.GetAllRelationKeys
		fx.mwMock.
			On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
				SpaceId: mockedSpaceId,
				Filters: []*model.BlockContentDataviewFilter{
					{
						RelationKey: bundle.RelationKeyResolvedLayout.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
					},
				},
				Keys: []string{bundle.RelationKeyRelationKey.String()},
			}).
			Return(&pb.RpcObjectSearchResponse{
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							bundle.RelationKeyRelationKey.String(): pbtypes.String(bundle.RelationKeyRelationKey.String()),
						},
					},
				},
			}, nil).Once()

		// Since viewId is empty, sorts and filters should be nil.
		fx.mwMock.
			On("ObjectSearchSubscribe", mock.Anything, &pb.RpcObjectSearchSubscribeRequest{
				SpaceId:      mockedSpaceId,
				SubId:        subId,
				Limit:        int64(limit),
				Offset:       int64(offset),
				Sorts:        nil,
				Filters:      nil,
				Keys:         []string{bundle.RelationKeyRelationKey.String()},
				CollectionId: mockedListId,
			}).
			Return(&pb.RpcObjectSearchSubscribeResponse{
				Error:    &pb.RpcObjectSearchSubscribeResponseError{Code: pb.RpcObjectSearchSubscribeResponseError_NULL},
				Counters: &pb.EventObjectSubscriptionCounters{Total: 1},
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							bundle.RelationKeyId.String():   pbtypes.String("object-1"),
							bundle.RelationKeyName.String(): pbtypes.String("Object One"),
						},
					},
				},
			}, nil).Once()

		// Mock unsubscribe after subscription
		fx.mwMock.
			On("ObjectSearchUnsubscribe", mock.Anything, &pb.RpcObjectSearchUnsubscribeRequest{
				SubIds: []string{subId},
			}).
			Return(&pb.RpcObjectSearchUnsubscribeResponse{}, nil).Once()

		// Mock getPropertyMapsFromStore
		fx.mwMock.
			On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
				SpaceId: mockedSpaceId,
				Filters: []*model.BlockContentDataviewFilter{
					{
						RelationKey: bundle.RelationKeyResolvedLayout.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
					},
					{
						RelationKey: bundle.RelationKeyIsHidden.String(),
						Condition:   model.BlockContentDataviewFilter_NotEqual,
						Value:       pbtypes.Bool(true),
					},
				},
				Keys: []string{
					bundle.RelationKeyId.String(),
					bundle.RelationKeyRelationKey.String(),
					bundle.RelationKeyApiObjectKey.String(),
					bundle.RelationKeyName.String(),
					bundle.RelationKeyRelationFormat.String(),
				},
			}).
			Return(&pb.RpcObjectSearchResponse{
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							bundle.RelationKeyRelationKey.String():    pbtypes.String("rel-dummy"),
							bundle.RelationKeyRelationFormat.String(): pbtypes.String(model.RelationFormat_longtext.String()),
						},
					},
				},
			}, nil).Once()

		// Mock getTypeMapFromStore
		fx.mwMock.
			On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
				SpaceId: mockedSpaceId,
				Filters: []*model.BlockContentDataviewFilter{
					{
						RelationKey: bundle.RelationKeyResolvedLayout.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.Int64(int64(model.ObjectType_objectType)),
					},
					{
						RelationKey: bundle.RelationKeyIsDeleted.String(),
					},
				},
				Keys: []string{
					bundle.RelationKeyId.String(),
					bundle.RelationKeyUniqueKey.String(),
					bundle.RelationKeyApiObjectKey.String(),
					bundle.RelationKeyName.String(),
					bundle.RelationKeyPluralName.String(),
					bundle.RelationKeyIconEmoji.String(),
					bundle.RelationKeyIconName.String(),
					bundle.RelationKeyIconOption.String(),
					bundle.RelationKeyRecommendedLayout.String(),
					bundle.RelationKeyIsArchived.String(),
					bundle.RelationKeyRecommendedFeaturedRelations.String(),
					bundle.RelationKeyRecommendedRelations.String(),
				},
			}).
			Return(&pb.RpcObjectSearchResponse{
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							bundle.RelationKeyId.String():                pbtypes.String("type-1"),
							bundle.RelationKeyUniqueKey.String():         pbtypes.String("type-key"),
							bundle.RelationKeyName.String():              pbtypes.String("Type One"),
							bundle.RelationKeyIconEmoji.String():         pbtypes.String(""),
							bundle.RelationKeyIconName.String():          pbtypes.String("icon1"),
							bundle.RelationKeyIconOption.String():        pbtypes.String("option1"),
							bundle.RelationKeyRecommendedLayout.String(): pbtypes.Int64(int64(model.ObjectType_basic)),
							bundle.RelationKeyIsArchived.String():        pbtypes.Bool(false),
						},
					},
				},
			}, nil).Once()

		// Mock getTagMapFromStore
		fx.mwMock.
			On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
				SpaceId: mockedSpaceId,
				Filters: []*model.BlockContentDataviewFilter{
					{
						RelationKey: bundle.RelationKeyResolvedLayout.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.Int64(int64(model.ObjectType_relationOption)),
					},
					{
						RelationKey: bundle.RelationKeyIsHidden.String(),
						Condition:   model.BlockContentDataviewFilter_NotEqual,
						Value:       pbtypes.Bool(true),
					},
				},
				Keys: []string{
					bundle.RelationKeyId.String(),
					bundle.RelationKeyUniqueKey.String(),
					bundle.RelationKeyName.String(),
					bundle.RelationKeyRelationOptionColor.String(),
				},
			}).
			Return(&pb.RpcObjectSearchResponse{
				Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
				Records: []*types.Struct{},
			}, nil).Once()

		// when
		objects, total, hasMore, err := fx.service.GetObjectsInList(ctx, mockedSpaceId, mockedListId, "", offset, limit)

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
		_, _, _, err := fx.service.GetObjectsInList(ctx, mockedSpaceId, mockedListId, "", offset, limit)

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
		_, _, _, err := fx.service.GetObjectsInList(ctx, mockedSpaceId, mockedListId, "", offset, limit)

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
		_, _, _, err := fx.service.GetObjectsInList(ctx, mockedSpaceId, mockedListId, "", offset, limit)

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
		_, _, _, err := fx.service.GetObjectsInList(ctx, mockedSpaceId, mockedListId, "non-existent-view", offset, limit)

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

		// Mock util.GetAllRelationKeys
		fx.mwMock.
			On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
				SpaceId: mockedSpaceId,
				Filters: []*model.BlockContentDataviewFilter{
					{
						RelationKey: bundle.RelationKeyResolvedLayout.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
					},
				},
				Keys: []string{bundle.RelationKeyRelationKey.String()},
			}).
			Return(&pb.RpcObjectSearchResponse{
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							bundle.RelationKeyRelationKey.String(): pbtypes.String(bundle.RelationKeyRelationKey.String()),
						},
					},
				},
			}, nil).Once()

		// Simulate an error from ObjectSearchSubscribe.
		fx.mwMock.
			On("ObjectSearchSubscribe", mock.Anything, &pb.RpcObjectSearchSubscribeRequest{
				SpaceId:      mockedSpaceId,
				SubId:        subId,
				Limit:        int64(limit),
				Offset:       int64(offset),
				Sorts:        sorts,
				Filters:      filters,
				Keys:         []string{bundle.RelationKeyRelationKey.String()},
				CollectionId: mockedListId,
			}).
			Return(&pb.RpcObjectSearchSubscribeResponse{
				Error: &pb.RpcObjectSearchSubscribeResponseError{Code: pb.RpcObjectSearchSubscribeResponseError_UNKNOWN_ERROR},
			}, nil).Once()

		// Mock unsubscribe after subscription
		fx.mwMock.
			On("ObjectSearchUnsubscribe", mock.Anything, &pb.RpcObjectSearchUnsubscribeRequest{
				SubIds: []string{subId},
			}).
			Return(&pb.RpcObjectSearchUnsubscribeResponse{}, nil).Once()

		// when
		_, _, _, err := fx.service.GetObjectsInList(ctx, mockedSpaceId, mockedListId, mockedViewId, offset, limit)

		// then
		require.ErrorIs(t, err, ErrFailedGetObjectsInList)
	})

	t.Run("get property map error", func(t *testing.T) {
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

		// Mock util.GetAllRelationKeys
		fx.mwMock.
			On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
				SpaceId: mockedSpaceId,
				Filters: []*model.BlockContentDataviewFilter{
					{
						RelationKey: bundle.RelationKeyResolvedLayout.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
					},
				},
				Keys: []string{bundle.RelationKeyRelationKey.String()},
			}).
			Return(&pb.RpcObjectSearchResponse{
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							bundle.RelationKeyRelationKey.String(): pbtypes.String(bundle.RelationKeyRelationKey.String()),
						},
					},
				},
			}, nil).Once()

		fx.mwMock.
			On("ObjectSearchSubscribe", mock.Anything, &pb.RpcObjectSearchSubscribeRequest{
				SpaceId:      mockedSpaceId,
				SubId:        subId,
				Limit:        int64(limit),
				Offset:       int64(offset),
				Sorts:        sorts,
				Filters:      filters,
				Keys:         []string{bundle.RelationKeyRelationKey.String()},
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

		// Mock unsubscribe after subscription
		fx.mwMock.
			On("ObjectSearchUnsubscribe", mock.Anything, &pb.RpcObjectSearchUnsubscribeRequest{
				SubIds: []string{subId},
			}).
			Return(&pb.RpcObjectSearchUnsubscribeResponse{}, nil).Once()

		// Mock getPropertyMapsFromStore to return an error.
		fx.mwMock.
			On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
				SpaceId: mockedSpaceId,
				Filters: []*model.BlockContentDataviewFilter{
					{
						RelationKey: bundle.RelationKeyResolvedLayout.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
					},
					{
						RelationKey: bundle.RelationKeyIsHidden.String(),
						Condition:   model.BlockContentDataviewFilter_NotEqual,
						Value:       pbtypes.Bool(true),
					},
				},
				Keys: []string{
					bundle.RelationKeyId.String(),
					bundle.RelationKeyRelationKey.String(),
					bundle.RelationKeyApiObjectKey.String(),
					bundle.RelationKeyName.String(),
					bundle.RelationKeyRelationFormat.String(),
				},
			}).
			Return(&pb.RpcObjectSearchResponse{
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_UNKNOWN_ERROR},
			}, nil).Once()

		// when
		_, _, _, err := fx.service.GetObjectsInList(ctx, mockedSpaceId, mockedListId, "", offset, limit)

		// then
		require.ErrorIs(t, err, ErrFailedRetrievePropertyMap)
	})
}

func TestListService_AddObjectsToList(t *testing.T) {

	t.Run("success", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		request := apimodel.AddObjectsToListRequest{Objects: []string{"obj-1", "obj-2"}}

		fx.mwMock.
			On("ObjectCollectionAdd", mock.Anything, &pb.RpcObjectCollectionAddRequest{
				ContextId: mockedListId,
				ObjectIds: request.Objects,
			}).
			Return(&pb.RpcObjectCollectionAddResponse{
				Error: &pb.RpcObjectCollectionAddResponseError{Code: pb.RpcObjectCollectionAddResponseError_NULL},
			}, nil).Once()

		// when
		err := fx.service.AddObjectsToList(ctx, mockedSpaceId, mockedListId, request)

		// then
		require.NoError(t, err)
	})

	t.Run("failure", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		request := apimodel.AddObjectsToListRequest{Objects: []string{"obj-1"}}

		fx.mwMock.
			On("ObjectCollectionAdd", mock.Anything, &pb.RpcObjectCollectionAddRequest{
				ContextId: mockedListId,
				ObjectIds: request.Objects,
			}).
			Return(&pb.RpcObjectCollectionAddResponse{
				Error: &pb.RpcObjectCollectionAddResponseError{Code: pb.RpcObjectCollectionAddResponseError_UNKNOWN_ERROR},
			}, nil).Once()

		// when
		err := fx.service.AddObjectsToList(ctx, mockedSpaceId, mockedListId, request)

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
		err := fx.service.RemoveObjectsFromList(ctx, mockedSpaceId, mockedListId, objectIds)

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
		err := fx.service.RemoveObjectsFromList(ctx, mockedSpaceId, mockedListId, objectIds)

		// then
		require.ErrorIs(t, err, ErrFailedRemoveObjectsFromList)
	})
}
