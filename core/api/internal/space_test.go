package internal

import (
	"context"
	"regexp"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/api/apimodel"
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
								bundle.RelationKeyIconEmoji.String():   pbtypes.String(""),
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
								bundle.RelationKeyIconEmoji.String():   pbtypes.String("🚀"),
								bundle.RelationKeyIconImage.String():   pbtypes.String(""),
								bundle.RelationKeyDescription.String(): pbtypes.String("desc2"),
							},
						},
					},
				},
			},
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
		}, nil).Once()

		// when
		spaces, total, hasMore, err := fx.service.ListSpaces(nil, offset, limit)

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
		require.Equal(t, apimodel.Icon{Format: "emoji", Emoji: apimodel.StringPtr("🚀")}, spaces[1].Icon)
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
		spaces, total, hasMore, err := fx.service.ListSpaces(nil, offset, limit)

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
		spaces, total, hasMore, err := fx.service.ListSpaces(nil, offset, limit)

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
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.SpaceStatus_Ok)),
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
								bundle.RelationKeyIconEmoji.String():   pbtypes.String("🚀"),
								bundle.RelationKeyIconImage.String():   pbtypes.String(""),
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
		require.Equal(t, apimodel.Icon{Format: "emoji", Emoji: apimodel.StringPtr("🚀")}, space.Icon)
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
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.SpaceStatus_Ok)),
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
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.SpaceStatus_Ok)),
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
								bundle.RelationKeyIconEmoji.String():   pbtypes.String("🚀"),
								bundle.RelationKeyIconImage.String():   pbtypes.String(""),
								bundle.RelationKeyDescription.String(): pbtypes.String("A new space"),
							},
						},
					},
				},
			},
		}, nil).Once()

		// when
		space, err := fx.service.CreateSpace(nil, apimodel.CreateSpaceRequest{Name: "New Space", Description: "A new space"})

		// then
		require.NoError(t, err)
		require.Equal(t, "new-space-id", space.Id)
		require.Equal(t, "New Space", space.Name)
		require.Equal(t, "A new space", space.Description)
		require.Equal(t, apimodel.Icon{Format: "emoji", Emoji: apimodel.StringPtr("🚀")}, space.Icon)
	})

	t.Run("failed workspace creation", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.mwMock.On("WorkspaceCreate", mock.Anything, mock.Anything).
			Return(&pb.RpcWorkspaceCreateResponse{
				Error: &pb.RpcWorkspaceCreateResponseError{Code: pb.RpcWorkspaceCreateResponseError_UNKNOWN_ERROR},
			}).Once()

		// when
		space, err := fx.service.CreateSpace(nil, apimodel.CreateSpaceRequest{Name: "New Space"})

		// then
		require.ErrorIs(t, err, ErrFailedCreateSpace)
		require.Equal(t, apimodel.Space{}, space)
	})
}

func TestSpaceService_ListMembers(t *testing.T) {
	joiningReq := &pb.RpcObjectSearchRequest{
		SpaceId: "space-id",
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_participant)),
			},
			{
				RelationKey: bundle.RelationKeyParticipantStatus.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ParticipantStatus_Joining)),
			},
		},
		Sorts: []*model.BlockContentDataviewSort{
			{
				RelationKey: bundle.RelationKeyName.String(),
				Type:        model.BlockContentDataviewSort_Asc,
			},
		},
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyIconEmoji.String(),
			bundle.RelationKeyIconImage.String(),
			bundle.RelationKeyIdentity.String(),
			bundle.RelationKeyGlobalName.String(),
			bundle.RelationKeyParticipantPermissions.String(),
			bundle.RelationKeyParticipantStatus.String(),
		},
	}

	activeReq := &pb.RpcObjectSearchRequest{
		SpaceId: "space-id",
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_participant)),
			},
			{
				RelationKey: bundle.RelationKeyParticipantStatus.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ParticipantStatus_Active)),
			},
		},
		Sorts: []*model.BlockContentDataviewSort{
			{
				RelationKey: bundle.RelationKeyName.String(),
				Type:        model.BlockContentDataviewSort_Asc,
			},
		},
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyIconEmoji.String(),
			bundle.RelationKeyIconImage.String(),
			bundle.RelationKeyIdentity.String(),
			bundle.RelationKeyGlobalName.String(),
			bundle.RelationKeyParticipantPermissions.String(),
			bundle.RelationKeyParticipantStatus.String(),
		},
	}

	t.Run("successfully get members", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, joiningReq).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							bundle.RelationKeyId.String():                     pbtypes.String("member-1"),
							bundle.RelationKeyName.String():                   pbtypes.String("Jane Doe"),
							bundle.RelationKeyIconImage.String():              pbtypes.String(iconImage),
							bundle.RelationKeyIdentity.String():               pbtypes.String("AAjLbEwPF4nkEh7AWkqEnzcQ8HziGB4ETjiTpvRCQvWnSMD4"),
							bundle.RelationKeyGlobalName.String():             pbtypes.String("jane.any"),
							bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(model.ParticipantStatus_Joining)),
							bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(model.ParticipantPermissions_NoPermissions)),
						},
					},
				},
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		fx.mwMock.On("ObjectSearch", mock.Anything, activeReq).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							bundle.RelationKeyId.String():                     pbtypes.String("member-2"),
							bundle.RelationKeyName.String():                   pbtypes.String("John Doe"),
							bundle.RelationKeyIconEmoji.String():              pbtypes.String("👤"),
							bundle.RelationKeyIdentity.String():               pbtypes.String("AAjEaEwPF4nkEh7AWkqEnzcQ8HziGB4ETjiTpvRCQvWnSMDZ"),
							bundle.RelationKeyGlobalName.String():             pbtypes.String("john.any"),
							bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(model.ParticipantStatus_Active)),
							bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(model.ParticipantPermissions_Owner)),
						},
					},
				},
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		// when
		members, total, hasMore, err := fx.service.ListMembers(nil, "space-id", offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, members, 2)

		require.Equal(t, "member-1", members[0].Id)
		require.Equal(t, "Jane Doe", members[0].Name)
		require.Regexpf(t, regexp.MustCompile(gatewayUrl+`/image/`+iconImage), *members[0].Icon.File, "Icon URL does not match")
		require.Equal(t, "jane.any", members[0].GlobalName)
		require.Equal(t, "joining", members[0].Status)
		require.Equal(t, "no_permissions", members[0].Role)

		require.Equal(t, "member-2", members[1].Id)
		require.Equal(t, "John Doe", members[1].Name)
		require.Equal(t, apimodel.Icon{Format: "emoji", Emoji: apimodel.StringPtr("👤")}, members[1].Icon)
		require.Equal(t, "john.any", members[1].GlobalName)
		require.Equal(t, "active", members[1].Status)
		require.Equal(t, "owner", members[1].Role)

		require.Equal(t, 2, total)
		require.False(t, hasMore)
	})

	t.Run("no members found", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, activeReq).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{},
				Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		fx.mwMock.On("ObjectSearch", mock.Anything, joiningReq).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{},
				Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		// when
		members, total, hasMore, err := fx.service.ListMembers(nil, "space-id", offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, members, 0)
		require.Equal(t, 0, total)
		require.False(t, hasMore)
	})
}

func TestSpaceService_GetMember(t *testing.T) {
	t.Run("successful retrieval of member", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: "space-id",
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyIdentity.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("member-id"),
				},
			},
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyIconEmoji.String(),
				bundle.RelationKeyIconImage.String(),
				bundle.RelationKeyIdentity.String(),
				bundle.RelationKeyGlobalName.String(),
				bundle.RelationKeyParticipantPermissions.String(),
				bundle.RelationKeyParticipantStatus.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():                     pbtypes.String("member-id"),
						bundle.RelationKeyName.String():                   pbtypes.String("John Doe"),
						bundle.RelationKeyIconEmoji.String():              pbtypes.String("👤"),
						bundle.RelationKeyIconImage.String():              pbtypes.String("icon.png"),
						bundle.RelationKeyIdentity.String():               pbtypes.String("member-id"),
						bundle.RelationKeyGlobalName.String():             pbtypes.String("john.any"),
						bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(model.ParticipantPermissions_Owner)),
						bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(model.ParticipantStatus_Active)),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// when
		member, err := fx.service.GetMember(nil, "space-id", "member-id")

		// then
		require.NoError(t, err)
		require.Equal(t, "member-id", member.Id)
		require.Equal(t, "John Doe", member.Name)
		require.Regexpf(t, regexp.MustCompile(gatewayUrl+`/image/icon.png`), *member.Icon.File, "Icon URL does not match")
		require.Equal(t, "member-id", member.Identity)
		require.Equal(t, "john.any", member.GlobalName)
		require.Equal(t, "active", member.Status)
		require.Equal(t, "owner", member.Role)
	})

	t.Run("member not found", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: "space-id",
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyIdentity.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("member-id")},
			},
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyIconEmoji.String(),
				bundle.RelationKeyIconImage.String(),
				bundle.RelationKeyIdentity.String(),
				bundle.RelationKeyGlobalName.String(),
				bundle.RelationKeyParticipantPermissions.String(),
				bundle.RelationKeyParticipantStatus.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{},
			Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// when
		member, err := fx.service.GetMember(nil, "space-id", "member-id")

		// then
		require.ErrorIs(t, err, ErrMemberNotFound)
		require.Equal(t, apimodel.Member{}, member)
	})
	t.Run("failed get member", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: "space-id",
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyIdentity.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("member-id"),
				},
			},
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyIconEmoji.String(),
				bundle.RelationKeyIconImage.String(),
				bundle.RelationKeyIdentity.String(),
				bundle.RelationKeyGlobalName.String(),
				bundle.RelationKeyParticipantPermissions.String(),
				bundle.RelationKeyParticipantStatus.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():                     pbtypes.String("member-id"),
						bundle.RelationKeyName.String():                   pbtypes.String("John Doe"),
						bundle.RelationKeyIconEmoji.String():              pbtypes.String(""),
						bundle.RelationKeyIconImage.String():              pbtypes.String("icon.png"),
						bundle.RelationKeyIdentity.String():               pbtypes.String("member-id"),
						bundle.RelationKeyGlobalName.String():             pbtypes.String("john.any"),
						bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(model.ParticipantPermissions_Owner)),
						bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(model.ParticipantStatus_Active)),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_UNKNOWN_ERROR},
		}).Once()

		// when
		member, err := fx.service.GetMember(nil, "space-id", "member-id")

		// then
		require.ErrorIs(t, err, ErrFailedGetMember)
		require.Equal(t, apimodel.Member{}, member)
	})

	t.Run("successful retrieval of member with participant id", func(t *testing.T) {
		// given
		fx := newFixture(t)
		participantId := "_participant123"

		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: "space-id",
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyId.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String(participantId),
				},
			},
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyIconEmoji.String(),
				bundle.RelationKeyIconImage.String(),
				bundle.RelationKeyIdentity.String(),
				bundle.RelationKeyGlobalName.String(),
				bundle.RelationKeyParticipantPermissions.String(),
				bundle.RelationKeyParticipantStatus.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():                     pbtypes.String(participantId),
						bundle.RelationKeyName.String():                   pbtypes.String("Alice"),
						bundle.RelationKeyIconEmoji.String():              pbtypes.String("😊"),
						bundle.RelationKeyIconImage.String():              pbtypes.String("participant.png"),
						bundle.RelationKeyIdentity.String():               pbtypes.String("alice-identity"),
						bundle.RelationKeyGlobalName.String():             pbtypes.String("alice.any"),
						bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(model.ParticipantPermissions_Writer)),
						bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(model.ParticipantStatus_Active)),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// when
		member, err := fx.service.GetMember(nil, "space-id", participantId)

		// then
		require.NoError(t, err)
		require.Equal(t, participantId, member.Id)
		require.Equal(t, "Alice", member.Name)
		require.Regexpf(t, regexp.MustCompile(gatewayUrl+`/image/participant.png`), *member.Icon.File, "Icon URL does not match")
		require.Equal(t, "alice-identity", member.Identity)
		require.Equal(t, "alice.any", member.GlobalName)
		require.Equal(t, "active", member.Status)
		require.Equal(t, "editor", member.Role)
	})
}

func TestSpaceService_UpdateMember(t *testing.T) {
	t.Run("successful approval for joining member", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// First GetMember call returns a member with status "joining"
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: "space-id",
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyIdentity.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("member-1"),
				},
			},
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyIconEmoji.String(),
				bundle.RelationKeyIconImage.String(),
				bundle.RelationKeyIdentity.String(),
				bundle.RelationKeyGlobalName.String(),
				bundle.RelationKeyParticipantPermissions.String(),
				bundle.RelationKeyParticipantStatus.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():                     pbtypes.String("member-1"),
						bundle.RelationKeyName.String():                   pbtypes.String("Joining Member"),
						bundle.RelationKeyIconEmoji.String():              pbtypes.String(""),
						bundle.RelationKeyIconImage.String():              pbtypes.String("icon.png"),
						bundle.RelationKeyIdentity.String():               pbtypes.String("member-1"),
						bundle.RelationKeyGlobalName.String():             pbtypes.String("joining.any"),
						bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(model.ParticipantPermissions_Reader)),
						bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(model.ParticipantStatus_Joining)),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}, nil).Once()

		// Expect approval call (for a joining member, role 'viewer' maps to ParticipantPermissions_Reader)
		fx.mwMock.On("SpaceRequestApprove", mock.Anything, &pb.RpcSpaceRequestApproveRequest{
			SpaceId:     "space-id",
			Identity:    "member-1",
			Permissions: model.ParticipantPermissions_Reader,
		}).Return(&pb.RpcSpaceRequestApproveResponse{
			Error: &pb.RpcSpaceRequestApproveResponseError{Code: pb.RpcSpaceRequestApproveResponseError_NULL},
		}, nil).Once()

		// Second GetMember call returns the updated member with status "active"
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: "space-id",
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyIdentity.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("member-1"),
				},
			},
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyIconEmoji.String(),
				bundle.RelationKeyIconImage.String(),
				bundle.RelationKeyIdentity.String(),
				bundle.RelationKeyGlobalName.String(),
				bundle.RelationKeyParticipantPermissions.String(),
				bundle.RelationKeyParticipantStatus.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():                     pbtypes.String("member-1"),
						bundle.RelationKeyName.String():                   pbtypes.String("Joining Member"),
						bundle.RelationKeyIconEmoji.String():              pbtypes.String(""),
						bundle.RelationKeyIconImage.String():              pbtypes.String("icon.png"),
						bundle.RelationKeyIdentity.String():               pbtypes.String("member-1"),
						bundle.RelationKeyGlobalName.String():             pbtypes.String("joining.any"),
						bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(model.ParticipantPermissions_Reader)),
						bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(model.ParticipantStatus_Active)),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}, nil).Once()

		// when
		member, err := fx.service.UpdateMember(ctx, "space-id", "member-1", apimodel.UpdateMemberRequest{
			Status: "active",
			Role:   "viewer",
		})

		// then
		require.NoError(t, err)
		require.Equal(t, "active", member.Status)
		require.Equal(t, "viewer", member.Role)
	})

	t.Run("successful role update for active member", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// First GetMember call returns a member with status "active" and role "viewer"
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: "space-id",
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyIdentity.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("member-2"),
				},
			},
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyIconEmoji.String(),
				bundle.RelationKeyIconImage.String(),
				bundle.RelationKeyIdentity.String(),
				bundle.RelationKeyGlobalName.String(),
				bundle.RelationKeyParticipantPermissions.String(),
				bundle.RelationKeyParticipantStatus.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():                     pbtypes.String("member-2"),
						bundle.RelationKeyName.String():                   pbtypes.String("Active Member"),
						bundle.RelationKeyIconEmoji.String():              pbtypes.String("👤"),
						bundle.RelationKeyIconImage.String():              pbtypes.String(""),
						bundle.RelationKeyIdentity.String():               pbtypes.String("member-2"),
						bundle.RelationKeyGlobalName.String():             pbtypes.String("active.any"),
						bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(model.ParticipantPermissions_Reader)),
						bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(model.ParticipantStatus_Active)),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}, nil).Once()

		// Expect role update call (for an active member, updating role to 'editor' maps to ParticipantPermissions_Writer)
		fx.mwMock.On("SpaceParticipantPermissionsChange", mock.Anything, &pb.RpcSpaceParticipantPermissionsChangeRequest{
			SpaceId: "space-id",
			Changes: []*model.ParticipantPermissionChange{
				{
					Identity: "member-2",
					Perms:    model.ParticipantPermissions_Writer,
				},
			},
		}).Return(&pb.RpcSpaceParticipantPermissionsChangeResponse{
			Error: &pb.RpcSpaceParticipantPermissionsChangeResponseError{Code: pb.RpcSpaceParticipantPermissionsChangeResponseError_NULL},
		}, nil).Once()

		// Second GetMember call returns the updated member with role "editor"
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: "space-id",
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyIdentity.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("member-2"),
				},
			},
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyIconEmoji.String(),
				bundle.RelationKeyIconImage.String(),
				bundle.RelationKeyIdentity.String(),
				bundle.RelationKeyGlobalName.String(),
				bundle.RelationKeyParticipantPermissions.String(),
				bundle.RelationKeyParticipantStatus.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():                     pbtypes.String("member-2"),
						bundle.RelationKeyName.String():                   pbtypes.String("Active Member"),
						bundle.RelationKeyIconEmoji.String():              pbtypes.String("👤"),
						bundle.RelationKeyIconImage.String():              pbtypes.String(""),
						bundle.RelationKeyIdentity.String():               pbtypes.String("member-2"),
						bundle.RelationKeyGlobalName.String():             pbtypes.String("active.any"),
						bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(model.ParticipantPermissions_Writer)),
						bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(model.ParticipantStatus_Active)),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}, nil).Once()

		// when
		member, err := fx.service.UpdateMember(ctx, "space-id", "member-2", apimodel.UpdateMemberRequest{
			Status: "active",
			Role:   "editor",
		})

		// then
		require.NoError(t, err)
		require.Equal(t, "active", member.Status)
		require.Equal(t, "editor", member.Role)
	})

	t.Run("successful decline of member", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// First GetMember call returns a member record
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: "space-id",
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyIdentity.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("member-3"),
				},
			},
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyIconEmoji.String(),
				bundle.RelationKeyIconImage.String(),
				bundle.RelationKeyIdentity.String(),
				bundle.RelationKeyGlobalName.String(),
				bundle.RelationKeyParticipantPermissions.String(),
				bundle.RelationKeyParticipantStatus.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():                     pbtypes.String("member-3"),
						bundle.RelationKeyName.String():                   pbtypes.String("Member To Decline"),
						bundle.RelationKeyIconEmoji.String():              pbtypes.String(""),
						bundle.RelationKeyIconImage.String():              pbtypes.String("icon.png"),
						bundle.RelationKeyIdentity.String():               pbtypes.String("member-3"),
						bundle.RelationKeyGlobalName.String():             pbtypes.String("decline.any"),
						bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(model.ParticipantPermissions_Reader)),
						bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(model.ParticipantStatus_Joining)),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}, nil).Once()

		// Expect decline call
		fx.mwMock.On("SpaceRequestDecline", mock.Anything, &pb.RpcSpaceRequestDeclineRequest{
			SpaceId:  "space-id",
			Identity: "member-3",
		}).Return(&pb.RpcSpaceRequestDeclineResponse{
			Error: &pb.RpcSpaceRequestDeclineResponseError{Code: pb.RpcSpaceRequestDeclineResponseError_NULL},
		}, nil).Once()

		// Second GetMember call returns the member with status "declined"
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: "space-id",
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyIdentity.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("member-3"),
				},
			},
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyIconEmoji.String(),
				bundle.RelationKeyIconImage.String(),
				bundle.RelationKeyIdentity.String(),
				bundle.RelationKeyGlobalName.String(),
				bundle.RelationKeyParticipantPermissions.String(),
				bundle.RelationKeyParticipantStatus.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():                     pbtypes.String("member-3"),
						bundle.RelationKeyName.String():                   pbtypes.String("Member To Decline"),
						bundle.RelationKeyIconEmoji.String():              pbtypes.String(""),
						bundle.RelationKeyIconImage.String():              pbtypes.String("icon.png"),
						bundle.RelationKeyIdentity.String():               pbtypes.String("member-3"),
						bundle.RelationKeyGlobalName.String():             pbtypes.String("decline.any"),
						bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(model.ParticipantPermissions_Reader)),
						bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(model.ParticipantStatus_Declined)),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}, nil).Once()

		// when
		member, err := fx.service.UpdateMember(ctx, "space-id", "member-3", apimodel.UpdateMemberRequest{
			Status: "declined",
		})

		// then
		require.NoError(t, err)
		require.Equal(t, "declined", member.Status)
	})

	t.Run("successful removal of member", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// First GetMember call returns a member with status "active"
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: "space-id",
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyIdentity.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("member-4"),
				},
			},
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyIconEmoji.String(),
				bundle.RelationKeyIconImage.String(),
				bundle.RelationKeyIdentity.String(),
				bundle.RelationKeyGlobalName.String(),
				bundle.RelationKeyParticipantPermissions.String(),
				bundle.RelationKeyParticipantStatus.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():                     pbtypes.String("member-4"),
						bundle.RelationKeyName.String():                   pbtypes.String("Member To Remove"),
						bundle.RelationKeyIconEmoji.String():              pbtypes.String("👤"),
						bundle.RelationKeyIconImage.String():              pbtypes.String(""),
						bundle.RelationKeyIdentity.String():               pbtypes.String("member-4"),
						bundle.RelationKeyGlobalName.String():             pbtypes.String("remove.any"),
						bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(model.ParticipantPermissions_Writer)),
						bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(model.ParticipantStatus_Active)),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}, nil).Once()

		// Expect removal call
		fx.mwMock.On("SpaceParticipantRemove", mock.Anything, &pb.RpcSpaceParticipantRemoveRequest{
			SpaceId:    "space-id",
			Identities: []string{"member-4"},
		}).Return(&pb.RpcSpaceParticipantRemoveResponse{
			Error: &pb.RpcSpaceParticipantRemoveResponseError{Code: pb.RpcSpaceParticipantRemoveResponseError_NULL},
		}, nil).Once()

		// Second GetMember call returns the member with status "removed"
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: "space-id",
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyIdentity.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("member-4"),
				},
			},
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyIconEmoji.String(),
				bundle.RelationKeyIconImage.String(),
				bundle.RelationKeyIdentity.String(),
				bundle.RelationKeyGlobalName.String(),
				bundle.RelationKeyParticipantPermissions.String(),
				bundle.RelationKeyParticipantStatus.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():                     pbtypes.String("member-4"),
						bundle.RelationKeyName.String():                   pbtypes.String("Member To Remove"),
						bundle.RelationKeyIconEmoji.String():              pbtypes.String("👤"),
						bundle.RelationKeyIconImage.String():              pbtypes.String(""),
						bundle.RelationKeyIdentity.String():               pbtypes.String("member-4"),
						bundle.RelationKeyGlobalName.String():             pbtypes.String("remove.any"),
						bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(model.ParticipantPermissions_Writer)),
						bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(model.ParticipantStatus_Removed)),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}, nil).Once()

		// when
		member, err := fx.service.UpdateMember(ctx, "space-id", "member-4", apimodel.UpdateMemberRequest{
			Status: "removed",
		})

		// then
		require.NoError(t, err)
		require.Equal(t, "removed", member.Status)
	})

	t.Run("invalid status returns error", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// First GetMember call returns a member record
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: "space-id",
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyIdentity.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("member-5"),
				},
			},
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyIconEmoji.String(),
				bundle.RelationKeyIconImage.String(),
				bundle.RelationKeyIdentity.String(),
				bundle.RelationKeyGlobalName.String(),
				bundle.RelationKeyParticipantPermissions.String(),
				bundle.RelationKeyParticipantStatus.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():                     pbtypes.String("member-5"),
						bundle.RelationKeyName.String():                   pbtypes.String("Member Invalid Status"),
						bundle.RelationKeyIconEmoji.String():              pbtypes.String(""),
						bundle.RelationKeyIconImage.String():              pbtypes.String(""),
						bundle.RelationKeyIdentity.String():               pbtypes.String("member-5"),
						bundle.RelationKeyGlobalName.String():             pbtypes.String("invalid.any"),
						bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(model.ParticipantPermissions_Reader)),
						bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(model.ParticipantStatus_Active)),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}, nil).Once()

		// when
		_, err := fx.service.UpdateMember(ctx, "space-id", "member-5", apimodel.UpdateMemberRequest{
			Status: "invalid",
			Role:   "viewer",
		})

		// then
		require.ErrorIs(t, err, ErrInvalidApproveMemberStatus)
	})

	t.Run("invalid role for active update returns error", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// First GetMember call returns a member with status "joining"
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: "space-id",
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyIdentity.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("member-6"),
				},
			},
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyIconEmoji.String(),
				bundle.RelationKeyIconImage.String(),
				bundle.RelationKeyIdentity.String(),
				bundle.RelationKeyGlobalName.String(),
				bundle.RelationKeyParticipantPermissions.String(),
				bundle.RelationKeyParticipantStatus.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():                     pbtypes.String("member-6"),
						bundle.RelationKeyName.String():                   pbtypes.String("Member Invalid Role"),
						bundle.RelationKeyIconEmoji.String():              pbtypes.String(""),
						bundle.RelationKeyIconImage.String():              pbtypes.String(""),
						bundle.RelationKeyIdentity.String():               pbtypes.String("member-6"),
						bundle.RelationKeyGlobalName.String():             pbtypes.String("invalidrole.any"),
						bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(model.ParticipantPermissions_Reader)),
						bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(model.ParticipantStatus_Joining)),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}, nil).Once()

		// when
		_, err := fx.service.UpdateMember(ctx, "space-id", "member-6", apimodel.UpdateMemberRequest{
			Status: "active",
			Role:   "invalid",
		})

		// then
		require.ErrorIs(t, err, ErrInvalidApproveMemberRole)
	})

	t.Run("failure in update operation returns error", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// First GetMember call returns a member with status "joining"
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: "space-id",
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyIdentity.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("member-7"),
				},
			},
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyIconEmoji.String(),
				bundle.RelationKeyIconImage.String(),
				bundle.RelationKeyIdentity.String(),
				bundle.RelationKeyGlobalName.String(),
				bundle.RelationKeyParticipantPermissions.String(),
				bundle.RelationKeyParticipantStatus.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():                     pbtypes.String("member-7"),
						bundle.RelationKeyName.String():                   pbtypes.String("Member Approval Fail"),
						bundle.RelationKeyIconEmoji.String():              pbtypes.String(""),
						bundle.RelationKeyIconImage.String():              pbtypes.String(""),
						bundle.RelationKeyIdentity.String():               pbtypes.String("member-7"),
						bundle.RelationKeyGlobalName.String():             pbtypes.String("fail.any"),
						bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(model.ParticipantPermissions_Reader)),
						bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(model.ParticipantStatus_Joining)),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}, nil).Once()

		// Expect approval call fails
		fx.mwMock.On("SpaceRequestApprove", mock.Anything, &pb.RpcSpaceRequestApproveRequest{
			SpaceId:     "space-id",
			Identity:    "member-7",
			Permissions: model.ParticipantPermissions_Reader,
		}).Return(&pb.RpcSpaceRequestApproveResponse{
			Error: &pb.RpcSpaceRequestApproveResponseError{Code: pb.RpcSpaceRequestApproveResponseError_UNKNOWN_ERROR},
		}, nil).Once()

		// when
		_, err := fx.service.UpdateMember(ctx, "space-id", "member-7", apimodel.UpdateMemberRequest{
			Status: "active",
			Role:   "viewer",
		})

		// then
		require.ErrorIs(t, err, ErrFailedUpdateMember)
	})

	t.Run("failed to get member returns error", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: "space-id",
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyIdentity.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("member-8"),
				},
			},
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyIconEmoji.String(),
				bundle.RelationKeyIconImage.String(),
				bundle.RelationKeyIdentity.String(),
				bundle.RelationKeyGlobalName.String(),
				bundle.RelationKeyParticipantPermissions.String(),
				bundle.RelationKeyParticipantStatus.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{},
			Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}, nil).Once()

		// when
		_, err := fx.service.UpdateMember(ctx, "space-id", "member-8", apimodel.UpdateMemberRequest{
			Status: "active",
			Role:   "viewer",
		})

		// then
		require.ErrorIs(t, err, ErrMemberNotFound)
	})
}
