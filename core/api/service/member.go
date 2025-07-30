package service

import (
	"context"
	"strings"

	"github.com/gogo/protobuf/types"
	"github.com/iancoleman/strcase"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// ListMembers returns a paginated list of members in the space with the given ID.
func (s *Service) ListMembers(ctx context.Context, spaceId string, additionalFilters []*model.BlockContentDataviewFilter, offset int, limit int) (members []apimodel.Member, total int, hasMore bool, err error) {
	filters := append([]*model.BlockContentDataviewFilter{
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
	}, additionalFilters...)

	activeResp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: filters,
		Sorts: []*model.BlockContentDataviewSort{
			{
				RelationKey: bundle.RelationKeyName.String(),
				Type:        model.BlockContentDataviewSort_Asc,
			},
		},
		Keys: []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String(), bundle.RelationKeyIconEmoji.String(), bundle.RelationKeyIconImage.String(), bundle.RelationKeyIdentity.String(), bundle.RelationKeyGlobalName.String(), bundle.RelationKeyParticipantPermissions.String(), bundle.RelationKeyParticipantStatus.String()},
	})

	if activeResp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedListMembers
	}

	joiningFilters := append([]*model.BlockContentDataviewFilter{
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
	}, additionalFilters...)

	joiningResp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: joiningFilters,
		Sorts: []*model.BlockContentDataviewSort{
			{
				RelationKey: bundle.RelationKeyName.String(),
				Type:        model.BlockContentDataviewSort_Asc,
			},
		},
		Keys: []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String(), bundle.RelationKeyIconEmoji.String(), bundle.RelationKeyIconImage.String(), bundle.RelationKeyIdentity.String(), bundle.RelationKeyGlobalName.String(), bundle.RelationKeyParticipantPermissions.String(), bundle.RelationKeyParticipantStatus.String()},
	})

	if joiningResp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedListMembers
	}

	combinedRecords := make([]*types.Struct, 0, len(joiningResp.Records)+len(activeResp.Records))
	combinedRecords = append(combinedRecords, joiningResp.Records...)
	combinedRecords = append(combinedRecords, activeResp.Records...)

	total = len(combinedRecords)
	paginatedMembers, hasMore := pagination.Paginate(combinedRecords, offset, limit)
	members = make([]apimodel.Member, 0, len(paginatedMembers))

	for _, record := range paginatedMembers {

		member := apimodel.Member{
			Object:     "member",
			Id:         record.Fields[bundle.RelationKeyId.String()].GetStringValue(),
			Name:       record.Fields[bundle.RelationKeyName.String()].GetStringValue(),
			Icon:       GetIcon(s.gatewayUrl, record.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), record.Fields[bundle.RelationKeyIconImage.String()].GetStringValue(), "", 0),
			Identity:   record.Fields[bundle.RelationKeyIdentity.String()].GetStringValue(),
			GlobalName: record.Fields[bundle.RelationKeyGlobalName.String()].GetStringValue(),
			Status:     strcase.ToSnake(model.ParticipantStatus_name[int32(record.Fields[bundle.RelationKeyParticipantStatus.String()].GetNumberValue())]),
			Role:       s.mapMemberPermissions(model.ParticipantPermissions(record.Fields[bundle.RelationKeyParticipantPermissions.String()].GetNumberValue())),
		}

		members = append(members, member)
	}

	return members, total, hasMore, nil
}

// GetMember returns the member with the given ID in the space with the given ID.
func (s *Service) GetMember(ctx context.Context, spaceId string, memberId string) (*apimodel.Member, error) {
	// Member ID can be either a participant ID or an identity.
	relationKey := bundle.RelationKeyId
	if !strings.HasPrefix(memberId, "_participant") {
		relationKey = bundle.RelationKeyIdentity
	}

	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: relationKey.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(memberId),
			},
		},
		Keys: []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String(), bundle.RelationKeyIconEmoji.String(), bundle.RelationKeyIconImage.String(), bundle.RelationKeyIdentity.String(), bundle.RelationKeyGlobalName.String(), bundle.RelationKeyParticipantPermissions.String(), bundle.RelationKeyParticipantStatus.String()},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, ErrFailedGetMember
	}

	if len(resp.Records) == 0 {
		return nil, ErrMemberNotFound
	}

	return &apimodel.Member{
		Object:     "member",
		Id:         resp.Records[0].Fields[bundle.RelationKeyId.String()].GetStringValue(),
		Name:       resp.Records[0].Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:       GetIcon(s.gatewayUrl, "", resp.Records[0].Fields[bundle.RelationKeyIconImage.String()].GetStringValue(), "", 0),
		Identity:   resp.Records[0].Fields[bundle.RelationKeyIdentity.String()].GetStringValue(),
		GlobalName: resp.Records[0].Fields[bundle.RelationKeyGlobalName.String()].GetStringValue(),
		Status:     strcase.ToSnake(model.ParticipantStatus_name[int32(resp.Records[0].Fields[bundle.RelationKeyParticipantStatus.String()].GetNumberValue())]),
		Role:       s.mapMemberPermissions(model.ParticipantPermissions(resp.Records[0].Fields[bundle.RelationKeyParticipantPermissions.String()].GetNumberValue())),
	}, nil
}

// UpdateMember approves member with a defined role or removes them
func (s *Service) UpdateMember(ctx context.Context, spaceId string, memberId string, request apimodel.UpdateMemberRequest) (*apimodel.Member, error) {
	member, err := s.GetMember(ctx, spaceId, memberId)
	if err != nil {
		return nil, err
	}

	status := *request.Status

	switch status {
	case apimodel.MemberStatusActive:
		if request.Role == nil {
			return nil, ErrInvalidApproveMemberRole
		}

		role := *request.Role

		if member.Status == "joining" {
			// Approve the member's join request.
			approveResp := s.mw.SpaceRequestApprove(ctx, &pb.RpcSpaceRequestApproveRequest{
				SpaceId:     spaceId,
				Identity:    memberId,
				Permissions: s.mapMemberRole(string(role)),
			})
			if approveResp.Error.Code != pb.RpcSpaceRequestApproveResponseError_NULL {
				return nil, ErrFailedUpdateMember
			}
		} else {
			// Update the member's role.
			resp := s.mw.SpaceParticipantPermissionsChange(ctx, &pb.RpcSpaceParticipantPermissionsChangeRequest{
				SpaceId: spaceId,
				Changes: []*model.ParticipantPermissionChange{{Identity: memberId, Perms: s.mapMemberRole(string(role))}},
			})
			if resp.Error != nil && resp.Error.Code != pb.RpcSpaceParticipantPermissionsChangeResponseError_NULL {
				return nil, ErrFailedUpdateMember
			}
		}
	case apimodel.MemberStatusDeclined:
		// Reject the member's join request.
		rejectResp := s.mw.SpaceRequestDecline(ctx, &pb.RpcSpaceRequestDeclineRequest{
			SpaceId:  spaceId,
			Identity: memberId,
		})
		if rejectResp.Error.Code != pb.RpcSpaceRequestDeclineResponseError_NULL {
			return nil, ErrFailedUpdateMember
		}
	case apimodel.MemberStatusRemoved:
		// Remove the member from the space.
		removeResp := s.mw.SpaceParticipantRemove(ctx, &pb.RpcSpaceParticipantRemoveRequest{
			SpaceId:    spaceId,
			Identities: []string{memberId},
		})
		if removeResp.Error.Code != pb.RpcSpaceParticipantRemoveResponseError_NULL {
			return nil, ErrFailedUpdateMember
		}
	}

	member, err = s.GetMember(ctx, spaceId, memberId)
	if err != nil {
		return nil, err
	}

	return member, nil
}

// mapMemberPermissions maps participant permissions to a role
func (s *Service) mapMemberPermissions(permissions model.ParticipantPermissions) string {
	switch permissions {
	case model.ParticipantPermissions_Reader:
		return "viewer"
	case model.ParticipantPermissions_Writer:
		return "editor"
	default:
		return strcase.ToSnake(model.ParticipantPermissions_name[int32(permissions)])
	}
}

// mapMemberPermissions maps a role to participant permissions
func (s *Service) mapMemberRole(role string) model.ParticipantPermissions {
	switch role {
	case "viewer":
		return model.ParticipantPermissions_Reader
	case "editor":
		return model.ParticipantPermissions_Writer
	default:
		return model.ParticipantPermissions_Reader
	}
}
