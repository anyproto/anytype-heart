package space

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service/mock_service"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	offset      = 0
	limit       = 100
	techSpaceId = "tech-space-id"
	gatewayUrl  = "http://localhost:31006"
	iconImage   = "bafyreialsgoyflf3etjm3parzurivyaukzivwortf32b4twnlwpwocsrri"
)

type fixture struct {
	*SpaceService
	mwMock *mock_service.MockClientCommandsServer
}

func newFixture(t *testing.T) *fixture {
	mw := mock_service.NewMockClientCommandsServer(t)
	spaceService := NewService(mw)
	spaceService.AccountInfo = &model.AccountInfo{
		TechSpaceId: techSpaceId,
		GatewayUrl:  gatewayUrl,
	}

	return &fixture{
		SpaceService: spaceService,
		mwMock:       mw,
	}
}

func TestSpaceService_ListSpaces(t *testing.T) {
	t.Run("successful retrieval of spaces", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: techSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					Operator:    model.BlockContentDataviewFilter_No,
					RelationKey: bundle.RelationKeyResolvedLayout.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
				},
				{
					Operator:    model.BlockContentDataviewFilter_No,
					RelationKey: bundle.RelationKeySpaceLocalStatus.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.SpaceStatus_Ok)),
				},
			},
			Sorts: []*model.BlockContentDataviewSort{
				{
					RelationKey:    "spaceOrder",
					Type:           model.BlockContentDataviewSort_Asc,
					NoCollate:      true,
					EmptyPlacement: model.BlockContentDataviewSort_End,
				},
			},
			Keys: []string{"targetSpaceId", "name", "iconEmoji", "iconImage"},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyName.String():          pbtypes.String("Another Workspace"),
						bundle.RelationKeyTargetSpaceId.String(): pbtypes.String("another-space-id"),
						bundle.RelationKeyIconEmoji.String():     pbtypes.String(""),
						bundle.RelationKeyIconImage.String():     pbtypes.String(iconImage),
					},
				},
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyName.String():          pbtypes.String("My Workspace"),
						bundle.RelationKeyTargetSpaceId.String(): pbtypes.String("my-space-id"),
						bundle.RelationKeyIconEmoji.String():     pbtypes.String("🚀"),
						bundle.RelationKeyIconImage.String():     pbtypes.String(""),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		fx.mwMock.On("WorkspaceOpen", mock.Anything, mock.Anything).Return(&pb.RpcWorkspaceOpenResponse{
			Error: &pb.RpcWorkspaceOpenResponseError{Code: pb.RpcWorkspaceOpenResponseError_NULL},
			Info: &model.AccountInfo{
				HomeObjectId:           "home-object-id",
				ArchiveObjectId:        "archive-object-id",
				ProfileObjectId:        "profile-object-id",
				MarketplaceWorkspaceId: "marketplace-workspace-id",
				WorkspaceObjectId:      "workspace-object-id",
				DeviceId:               "device-id",
				AccountSpaceId:         "account-space-id",
				WidgetsId:              "widgets-id",
				SpaceViewId:            "space-view-id",
				TechSpaceId:            "tech-space-id",
				GatewayUrl:             "gateway-url",
				LocalStoragePath:       "local-storage-path",
				TimeZone:               "time-zone",
				AnalyticsId:            "analytics-id",
				NetworkId:              "network-id",
			},
		}, nil).Twice()

		// when
		spaces, total, hasMore, err := fx.ListSpaces(nil, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, spaces, 2)
		require.Equal(t, "Another Workspace", spaces[0].Name)
		require.Equal(t, "another-space-id", spaces[0].Id)
		require.Regexpf(t, regexp.MustCompile(gatewayUrl+`/image/`+iconImage), *spaces[0].Icon.File, "Icon URL does not match")
		require.Equal(t, "My Workspace", spaces[1].Name)
		require.Equal(t, "my-space-id", spaces[1].Id)
		require.Equal(t, util.Icon{Format: "emoji", Emoji: util.StringPtr("🚀")}, spaces[1].Icon)
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
		spaces, total, hasMore, err := fx.ListSpaces(nil, offset, limit)

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
							bundle.RelationKeyName.String():          pbtypes.String("My Workspace"),
							bundle.RelationKeyTargetSpaceId.String(): pbtypes.String("my-space-id"),
							bundle.RelationKeyIconEmoji.String():     pbtypes.String("🚀"),
							bundle.RelationKeyIconImage.String():     pbtypes.String(""),
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
		spaces, total, hasMore, err := fx.ListSpaces(nil, offset, limit)

		// then
		require.ErrorIs(t, err, ErrFailedOpenWorkspace)
		require.Len(t, spaces, 0)
		require.Equal(t, 0, total)
		require.False(t, hasMore)
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

		fx.mwMock.On("WorkspaceOpen", mock.Anything, mock.Anything).Return(&pb.RpcWorkspaceOpenResponse{
			Error: &pb.RpcWorkspaceOpenResponseError{Code: pb.RpcWorkspaceOpenResponseError_NULL},
			Info: &model.AccountInfo{
				HomeObjectId:           "home-object-id",
				ArchiveObjectId:        "archive-object-id",
				ProfileObjectId:        "profile-object-id",
				MarketplaceWorkspaceId: "marketplace-workspace-id",
				WorkspaceObjectId:      "workspace-object-id",
				DeviceId:               "device-id",
				AccountSpaceId:         "account-space-id",
				WidgetsId:              "widgets-id",
				SpaceViewId:            "space-view-id",
				TechSpaceId:            "tech-space-id",
				GatewayUrl:             "gateway-url",
				LocalStoragePath:       "local-storage-path",
				TimeZone:               "time-zone",
				AnalyticsId:            "analytics-id",
				NetworkId:              "network-id",
			},
		}, nil).Once()

		// when
		space, err := fx.CreateSpace(nil, "New Space")

		// then
		require.NoError(t, err)
		require.Equal(t, "new-space-id", space.Id)
	})

	t.Run("failed workspace creation", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.mwMock.On("WorkspaceCreate", mock.Anything, mock.Anything).
			Return(&pb.RpcWorkspaceCreateResponse{
				Error: &pb.RpcWorkspaceCreateResponseError{Code: pb.RpcWorkspaceCreateResponseError_UNKNOWN_ERROR},
			}).Once()

		// when
		space, err := fx.CreateSpace(nil, "New Space")

		// then
		require.ErrorIs(t, err, ErrFailedCreateSpace)
		require.Equal(t, Space{}, space)
	})
}

func TestSpaceService_ListMembers(t *testing.T) {
	t.Run("successfully get members", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							bundle.RelationKeyId.String():         pbtypes.String("member-1"),
							bundle.RelationKeyName.String():       pbtypes.String("John Doe"),
							bundle.RelationKeyIconEmoji.String():  pbtypes.String("👤"),
							bundle.RelationKeyIdentity.String():   pbtypes.String("AAjEaEwPF4nkEh7AWkqEnzcQ8HziGB4ETjiTpvRCQvWnSMDZ"),
							bundle.RelationKeyGlobalName.String(): pbtypes.String("john.any"),
						},
					},
					{
						Fields: map[string]*types.Value{
							bundle.RelationKeyId.String():         pbtypes.String("member-2"),
							bundle.RelationKeyName.String():       pbtypes.String("Jane Doe"),
							bundle.RelationKeyIconImage.String():  pbtypes.String(iconImage),
							bundle.RelationKeyIdentity.String():   pbtypes.String("AAjLbEwPF4nkEh7AWkqEnzcQ8HziGB4ETjiTpvRCQvWnSMD4"),
							bundle.RelationKeyGlobalName.String(): pbtypes.String("jane.any"),
						},
					},
				},
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		// when
		members, total, hasMore, err := fx.ListMembers(nil, "space-id", offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, members, 2)
		require.Equal(t, "member-1", members[0].Id)
		require.Equal(t, "John Doe", members[0].Name)
		require.Equal(t, util.Icon{Format: "emoji", Emoji: util.StringPtr("👤")}, members[0].Icon)
		require.Equal(t, "john.any", members[0].GlobalName)
		require.Equal(t, "member-2", members[1].Id)
		require.Equal(t, "Jane Doe", members[1].Name)
		require.Regexpf(t, regexp.MustCompile(gatewayUrl+`/image/`+iconImage), *members[1].Icon.File, "Icon URL does not match")
		require.Equal(t, "jane.any", members[1].GlobalName)
		require.Equal(t, 2, total)
		require.False(t, hasMore)
	})

	t.Run("no members found", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{},
				Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		// when
		members, total, hasMore, err := fx.ListMembers(nil, "space-id", offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, members, 0)
		require.Equal(t, 0, total)
		require.False(t, hasMore)
	})
}
