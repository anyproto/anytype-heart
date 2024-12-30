package space

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gogo/protobuf/types"

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
	spaceService := &SpaceService{mw: mw, AccountInfo: &model.AccountInfo{TechSpaceId: techSpaceId, GatewayUrl: gatewayUrl}}

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
					RelationKey: bundle.RelationKeyLayout.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
				},
				{
					RelationKey: bundle.RelationKeySpaceLocalStatus.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.SpaceStatus_Ok)),
				},
				{
					RelationKey: bundle.RelationKeySpaceRemoteStatus.String(),
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
						"name":          pbtypes.String("Another Workspace"),
						"targetSpaceId": pbtypes.String("another-space-id"),
						"iconEmoji":     pbtypes.String(""),
						"iconImage":     pbtypes.String(iconImage),
					},
				},
				{
					Fields: map[string]*types.Value{
						"name":          pbtypes.String("My Workspace"),
						"targetSpaceId": pbtypes.String("my-space-id"),
						"iconEmoji":     pbtypes.String("🚀"),
						"iconImage":     pbtypes.String(""),
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
		require.Regexpf(t, regexp.MustCompile(gatewayUrl+`/image/`+iconImage), spaces[0].Icon, "Icon URL does not match")
		require.Equal(t, "My Workspace", spaces[1].Name)
		require.Equal(t, "my-space-id", spaces[1].Id)
		require.Equal(t, "🚀", spaces[1].Icon)
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
		require.ErrorIs(t, err, ErrNoSpacesFound)
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
							"name":          pbtypes.String("My Workspace"),
							"targetSpaceId": pbtypes.String("my-space-id"),
							"iconEmoji":     pbtypes.String("🚀"),
							"iconImage":     pbtypes.String(""),
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
							"id":         pbtypes.String("member-1"),
							"name":       pbtypes.String("John Doe"),
							"iconEmoji":  pbtypes.String("👤"),
							"identity":   pbtypes.String("AAjEaEwPF4nkEh7AWkqEnzcQ8HziGB4ETjiTpvRCQvWnSMDZ"),
							"globalName": pbtypes.String("john.any"),
						},
					},
					{
						Fields: map[string]*types.Value{
							"id":         pbtypes.String("member-2"),
							"name":       pbtypes.String("Jane Doe"),
							"iconImage":  pbtypes.String(iconImage),
							"identity":   pbtypes.String("AAjLbEwPF4nkEh7AWkqEnzcQ8HziGB4ETjiTpvRCQvWnSMD4"),
							"globalName": pbtypes.String("jane.any"),
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
		require.Equal(t, "👤", members[0].Icon)
		require.Equal(t, "john.any", members[0].GlobalName)
		require.Equal(t, "member-2", members[1].Id)
		require.Equal(t, "Jane Doe", members[1].Name)
		require.Regexpf(t, regexp.MustCompile(gatewayUrl+`/image/`+iconImage), members[1].Icon, "Icon URL does not match")
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
		require.ErrorIs(t, err, ErrNoMembersFound)
		require.Len(t, members, 0)
		require.Equal(t, 0, total)
		require.False(t, hasMore)
	})
}
