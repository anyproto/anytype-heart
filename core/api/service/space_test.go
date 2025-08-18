package service

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gogo/protobuf/types"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	iconImage = "bafyreialsgoyflf3etjm3parzurivyaukzivwortf32b4twnlwpwocsrri"
)

func TestSpaceService_ListSpaces(t *testing.T) {
	t.Run("successful retrieval of spaces", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: techSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyResolvedLayout.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
				},
				{
					RelationKey: bundle.RelationKeySpaceLocalStatus.String(),
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       pbtypes.IntList(int(model.SpaceStatus_Unknown), int(model.SpaceStatus_Ok)),
				},
				{
					RelationKey: bundle.RelationKeySpaceAccountStatus.String(),
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       pbtypes.IntList(int(model.SpaceStatus_Unknown), int(model.SpaceStatus_SpaceActive)),
				},
			},
			Sorts: []*model.BlockContentDataviewSort{
				{
					RelationKey:    bundle.RelationKeySpaceOrder.String(),
					Type:           model.BlockContentDataviewSort_Asc,
					EmptyPlacement: model.BlockContentDataviewSort_End,
				},
				{
					RelationKey: bundle.RelationKeyCreatedDate.String(),
					Type:        model.BlockContentDataviewSort_Desc,
					Format:      model.RelationFormat_longtext,
					IncludeTime: true,
				},
			},
			Keys: []string{bundle.RelationKeyTargetSpaceId.String()},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyTargetSpaceId.String(): pbtypes.String("another-space-id"),
					},
				},
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyTargetSpaceId.String(): pbtypes.String("my-space-id"),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		fx.mwMock.On("WorkspaceOpen", mock.Anything, mock.Anything).Return(&pb.RpcWorkspaceOpenResponse{
			Error: &pb.RpcWorkspaceOpenResponseError{Code: pb.RpcWorkspaceOpenResponseError_NULL},
			Info: &model.AccountInfo{
				WorkspaceObjectId: "workspace-object-id-1",
				GatewayUrl:        "gateway-url-1",
				NetworkId:         "network-id-1",
			},
		}, nil).Once()

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  "another-space-id",
			ObjectId: "workspace-object-id-1",
		}).Return(&pb.RpcObjectShowResponse{
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyName.String():        pbtypes.String("Another Workspace"),
								bundle.RelationKeyIconImage.String():   pbtypes.String(iconImage),
								bundle.RelationKeyDescription.String(): pbtypes.String("desc1"),
							},
						},
					},
				},
			},
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
		}, nil).Once()

		fx.mwMock.On("WorkspaceOpen", mock.Anything, mock.Anything).Return(&pb.RpcWorkspaceOpenResponse{
			Error: &pb.RpcWorkspaceOpenResponseError{Code: pb.RpcWorkspaceOpenResponseError_NULL},
			Info: &model.AccountInfo{
				WorkspaceObjectId: "workspace-object-id-2",
				GatewayUrl:        "gateway-url-2",
				NetworkId:         "network-id-2",
			},
		}, nil).Once()

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  "my-space-id",
			ObjectId: "workspace-object-id-2",
		}).Return(&pb.RpcObjectShowResponse{
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyName.String():        pbtypes.String("My Workspace"),
								bundle.RelationKeyIconImage.String():   pbtypes.String(iconImage),
								bundle.RelationKeyDescription.String(): pbtypes.String("desc2"),
							},
						},
					},
				},
			},
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
		}, nil).Once()

		// when
		spaces, total, hasMore, err := fx.service.ListSpaces(nil, nil, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, spaces, 2)

		require.Equal(t, "Another Workspace", spaces[0].Name)
		require.Equal(t, "another-space-id", spaces[0].Id)
		require.Equal(t, "desc1", spaces[0].Description)
		require.Equal(t, "gateway-url-1", spaces[0].GatewayUrl)
		require.Equal(t, "network-id-1", spaces[0].NetworkId)

		require.Equal(t, "My Workspace", spaces[1].Name)
		require.Equal(t, "my-space-id", spaces[1].Id)
		require.Equal(t, "desc2", spaces[1].Description)
		require.Equal(t, &apimodel.Icon{
			WrappedIcon: apimodel.FileIcon{
				Format: apimodel.IconFormatFile,
				File:   gatewayUrl + "/image/" + iconImage,
			},
		}, spaces[1].Icon)
		require.Equal(t, "gateway-url-2", spaces[1].GatewayUrl)
		require.Equal(t, "network-id-2", spaces[1].NetworkId)

		require.Equal(t, 2, total)
		require.False(t, hasMore)
	})

	t.Run("no spaces found", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{},
				Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		// when
		spaces, total, hasMore, err := fx.service.ListSpaces(nil, nil, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, spaces, 0)
		require.Equal(t, 0, total)
		require.False(t, hasMore)
	})

	t.Run("failed workspace open", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							bundle.RelationKeyTargetSpaceId.String(): pbtypes.String("my-space-id"),
						},
					},
				},
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		fx.mwMock.On("WorkspaceOpen", mock.Anything, mock.Anything).
			Return(&pb.RpcWorkspaceOpenResponse{
				Error: &pb.RpcWorkspaceOpenResponseError{Code: pb.RpcWorkspaceOpenResponseError_UNKNOWN_ERROR},
			}, nil).Once()

		// when
		spaces, total, hasMore, err := fx.service.ListSpaces(nil, nil, offset, limit)

		// then
		require.ErrorIs(t, err, ErrFailedOpenWorkspace)
		require.Len(t, spaces, 0)
		require.Equal(t, 0, total)
		require.False(t, hasMore)
	})
}

func TestSpaceService_GetSpace(t *testing.T) {
	t.Run("successful retrieval of space", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: techSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyTargetSpaceId.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("space-id"),
				},
				{
					RelationKey: bundle.RelationKeySpaceLocalStatus.String(),
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       pbtypes.IntList(int(model.SpaceStatus_Unknown), int(model.SpaceStatus_Ok)),
				},
				{
					RelationKey: bundle.RelationKeySpaceAccountStatus.String(),
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       pbtypes.IntList(int(model.SpaceStatus_Unknown), int(model.SpaceStatus_SpaceActive)),
				},
			},
			Keys: []string{
				bundle.RelationKeyTargetSpaceId.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyTargetSpaceId.String(): pbtypes.String("space-id"),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		fx.mwMock.On("WorkspaceOpen", mock.Anything, mock.Anything).Return(&pb.RpcWorkspaceOpenResponse{
			Error: &pb.RpcWorkspaceOpenResponseError{Code: pb.RpcWorkspaceOpenResponseError_NULL},
			Info: &model.AccountInfo{
				WorkspaceObjectId: "workspace-object-id",
				GatewayUrl:        "gateway-url",
				NetworkId:         "network-id",
			},
		}, nil).Once()

		// Expect ObjectShow call to return space details.
		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  "space-id",
			ObjectId: "workspace-object-id",
		}).Return(&pb.RpcObjectShowResponse{
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyName.String():        pbtypes.String("My Workspace"),
								bundle.RelationKeyIconImage.String():   pbtypes.String(iconImage),
								bundle.RelationKeyDescription.String(): pbtypes.String("A description"),
							},
						},
					},
				},
			},
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
		}, nil).Once()

		// when
		space, err := fx.service.GetSpace(nil, "space-id")

		// then
		require.NoError(t, err)
		require.Equal(t, "My Workspace", space.Name)
		require.Equal(t, "space-id", space.Id)
		require.Equal(t, &apimodel.Icon{
			WrappedIcon: apimodel.FileIcon{
				Format: apimodel.IconFormatFile,
				File:   gatewayUrl + "/image/" + iconImage,
			},
		}, space.Icon)
		require.Equal(t, "gateway-url", space.GatewayUrl)
		require.Equal(t, "network-id", space.NetworkId)
	})

	t.Run("workspace not found", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: techSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyTargetSpaceId.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("space-id"),
				},
				{
					RelationKey: bundle.RelationKeySpaceLocalStatus.String(),
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       pbtypes.IntList(int(model.SpaceStatus_Unknown), int(model.SpaceStatus_Ok)),
				},
				{
					RelationKey: bundle.RelationKeySpaceAccountStatus.String(),
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       pbtypes.IntList(int(model.SpaceStatus_Unknown), int(model.SpaceStatus_SpaceActive)),
				},
			},
			Keys: []string{bundle.RelationKeyTargetSpaceId.String()},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{},
			Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// when
		space, err := fx.service.GetSpace(nil, "space-id")

		// then
		require.ErrorIs(t, err, ErrWorkspaceNotFound)
		require.Equal(t, apimodel.Space{}, space)
	})

	t.Run("failed workspace open", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: techSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyTargetSpaceId.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("space-id"),
				},
				{
					RelationKey: bundle.RelationKeySpaceLocalStatus.String(),
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       pbtypes.IntList(int(model.SpaceStatus_Unknown), int(model.SpaceStatus_Ok)),
				},
				{
					RelationKey: bundle.RelationKeySpaceAccountStatus.String(),
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       pbtypes.IntList(int(model.SpaceStatus_Unknown), int(model.SpaceStatus_SpaceActive)),
				},
			},
			Keys: []string{bundle.RelationKeyTargetSpaceId.String()},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyTargetSpaceId.String(): pbtypes.String("space-id"),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		fx.mwMock.On("WorkspaceOpen", mock.Anything, mock.Anything).
			Return(&pb.RpcWorkspaceOpenResponse{
				Error: &pb.RpcWorkspaceOpenResponseError{Code: pb.RpcWorkspaceOpenResponseError_UNKNOWN_ERROR},
			}, nil).Once()

		// when
		space, err := fx.service.GetSpace(nil, "space-id")

		// then
		require.ErrorIs(t, err, ErrFailedOpenWorkspace)
		require.Equal(t, apimodel.Space{}, space)
	})
}

func TestSpaceService_CreateSpace(t *testing.T) {
	t.Run("successful create space", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.mwMock.On("WorkspaceCreate", mock.Anything, mock.Anything).
			Return(&pb.RpcWorkspaceCreateResponse{
				Error:   &pb.RpcWorkspaceCreateResponseError{Code: pb.RpcWorkspaceCreateResponseError_NULL},
				SpaceId: "new-space-id",
			}).Once()

		fx.mwMock.On("WorkspaceSetInfo", mock.Anything, &pb.RpcWorkspaceSetInfoRequest{
			SpaceId: "new-space-id",
			Details: &types.Struct{
				Fields: map[string]*types.Value{
					bundle.RelationKeyDescription.String(): pbtypes.String("A new space"),
				},
			},
		}).Return(&pb.RpcWorkspaceSetInfoResponse{
			Error: &pb.RpcWorkspaceSetInfoResponseError{Code: pb.RpcWorkspaceSetInfoResponseError_NULL},
		}, nil).Once()

		fx.mwMock.On("WorkspaceOpen", mock.Anything, mock.Anything).Return(&pb.RpcWorkspaceOpenResponse{
			Error: &pb.RpcWorkspaceOpenResponseError{Code: pb.RpcWorkspaceOpenResponseError_NULL},
			Info: &model.AccountInfo{
				WorkspaceObjectId: "workspace-object-id",
				GatewayUrl:        "gateway-url",
				NetworkId:         "network-id",
			},
		}, nil).Once()

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  "new-space-id",
			ObjectId: "workspace-object-id",
		}).Return(&pb.RpcObjectShowResponse{
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyName.String():        pbtypes.String("New Space"),
								bundle.RelationKeyIconImage.String():   pbtypes.String(iconImage),
								bundle.RelationKeyDescription.String(): pbtypes.String("A new space"),
							},
						},
					},
				},
			},
		}, nil).Once()

		// when
		space, err := fx.service.CreateSpace(nil, apimodel.CreateSpaceRequest{Name: util.PtrString("New Space"), Description: util.PtrString("A new space")})

		// then
		require.NoError(t, err)
		require.Equal(t, "new-space-id", space.Id)
		require.Equal(t, "New Space", space.Name)
		require.Equal(t, "A new space", space.Description)
		require.Equal(t, &apimodel.Icon{
			WrappedIcon: apimodel.FileIcon{
				Format: apimodel.IconFormatFile,
				File:   gatewayUrl + "/image/" + iconImage,
			},
		}, space.Icon)
	})

	t.Run("failed workspace creation", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.mwMock.On("WorkspaceCreate", mock.Anything, mock.Anything).
			Return(&pb.RpcWorkspaceCreateResponse{
				Error: &pb.RpcWorkspaceCreateResponseError{Code: pb.RpcWorkspaceCreateResponseError_UNKNOWN_ERROR},
			}).Once()

		// when
		space, err := fx.service.CreateSpace(nil, apimodel.CreateSpaceRequest{Name: util.PtrString("New Space")})

		// then
		require.ErrorIs(t, err, ErrFailedCreateSpace)
		require.Equal(t, apimodel.Space{}, space)
	})
}
