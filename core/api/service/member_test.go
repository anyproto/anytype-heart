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
		members, total, hasMore, err := fx.service.ListMembers(nil, "space-id", nil, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, members, 2)

		require.Equal(t, "member-1", members[0].Id)
		require.Equal(t, "Jane Doe", members[0].Name)
		require.Equal(t, &apimodel.Icon{
			WrappedIcon: apimodel.FileIcon{
				Format: apimodel.IconFormatFile,
				File:   gatewayUrl + "/image/" + iconImage,
			},
		}, members[0].Icon)
		require.Equal(t, "jane.any", members[0].GlobalName)
		require.Equal(t, "joining", members[0].Status)
		require.Equal(t, "no_permissions", members[0].Role)

		require.Equal(t, "member-2", members[1].Id)
		require.Equal(t, "John Doe", members[1].Name)
		require.Equal(t, &apimodel.Icon{
			WrappedIcon: apimodel.EmojiIcon{
				Format: apimodel.IconFormatEmoji,
				Emoji:  "👤",
			},
		}, members[1].Icon)
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
		members, total, hasMore, err := fx.service.ListMembers(nil, "space-id", nil, offset, limit)

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
		require.Equal(t, &apimodel.Icon{
			WrappedIcon: apimodel.FileIcon{
				Format: apimodel.IconFormatFile,
				File:   gatewayUrl + "/image/icon.png",
			},
		}, member.Icon)
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
		require.Nil(t, member)
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
		require.Nil(t, member)
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
		require.Equal(t, &apimodel.Icon{
			WrappedIcon: apimodel.FileIcon{
				Format: apimodel.IconFormatFile,
				File:   gatewayUrl + "/image/participant.png",
			},
		}, member.Icon)
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
		status := apimodel.MemberStatusActive
		role := apimodel.MemberRoleViewer
		member, err := fx.service.UpdateMember(ctx, "space-id", "member-1", apimodel.UpdateMemberRequest{
			Status: &status,
			Role:   &role,
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
		status := apimodel.MemberStatusActive
		role := apimodel.MemberRoleEditor
		member, err := fx.service.UpdateMember(ctx, "space-id", "member-2", apimodel.UpdateMemberRequest{
			Status: &status,
			Role:   &role,
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
		status := apimodel.MemberStatusDeclined
		member, err := fx.service.UpdateMember(ctx, "space-id", "member-3", apimodel.UpdateMemberRequest{
			Status: &status,
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
		status := apimodel.MemberStatusRemoved
		member, err := fx.service.UpdateMember(ctx, "space-id", "member-4", apimodel.UpdateMemberRequest{
			Status: &status,
		})

		// then
		require.NoError(t, err)
		require.Equal(t, "removed", member.Status)
	})

	t.Run("missing role for active update returns error", func(t *testing.T) {
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
						bundle.RelationKeyName.String():                   pbtypes.String("Member Missing Role"),
						bundle.RelationKeyIconEmoji.String():              pbtypes.String(""),
						bundle.RelationKeyIconImage.String():              pbtypes.String(""),
						bundle.RelationKeyIdentity.String():               pbtypes.String("member-6"),
						bundle.RelationKeyGlobalName.String():             pbtypes.String("missingrole.any"),
						bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(model.ParticipantPermissions_Reader)),
						bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(model.ParticipantStatus_Joining)),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}, nil).Once()

		// when
		status := apimodel.MemberStatusActive
		_, err := fx.service.UpdateMember(ctx, "space-id", "member-6", apimodel.UpdateMemberRequest{
			Status: &status,
			Role:   nil, // Missing role for active status
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
		status := apimodel.MemberStatusActive
		role := apimodel.MemberRoleViewer
		_, err := fx.service.UpdateMember(ctx, "space-id", "member-7", apimodel.UpdateMemberRequest{
			Status: &status,
			Role:   &role,
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
		status := apimodel.MemberStatusActive
		role := apimodel.MemberRoleViewer
		_, err := fx.service.UpdateMember(ctx, "space-id", "member-8", apimodel.UpdateMemberRequest{
			Status: &status,
			Role:   &role,
		})

		// then
		require.ErrorIs(t, err, ErrMemberNotFound)
	})
}
