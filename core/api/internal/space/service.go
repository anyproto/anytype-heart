package space

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"strings"

	"github.com/gogo/protobuf/types"
	"github.com/iancoleman/strcase"

	"github.com/anyproto/anytype-heart/core/api/apicore"
	"github.com/anyproto/anytype-heart/core/api/internal/object"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ErrFailedListSpaces           = errors.New("failed to retrieve list of spaces")
	ErrFailedOpenWorkspace        = errors.New("failed to open workspace")
	ErrFailedOpenSpace            = errors.New("failed to open space")
	ErrWorkspaceNotFound          = errors.New("workspace not found")
	ErrFailedGenerateRandomIcon   = errors.New("failed to generate random icon")
	ErrFailedCreateSpace          = errors.New("failed to create space")
	ErrFailedSetSpaceInfo         = errors.New("failed to set space info")
	ErrFailedListMembers          = errors.New("failed to retrieve list of members")
	ErrFailedGetMember            = errors.New("failed to retrieve member")
	ErrMemberNotFound             = errors.New("member not found")
	ErrInvalidApproveMemberStatus = errors.New("status must be 'active', 'declined', or 'removed'")
	ErrInvalidApproveMemberRole   = errors.New("role must be 'reader' or 'writer'")
	ErrFailedUpdateMember         = errors.New("failed to update member")
)

type Service interface {
	ListSpaces(ctx context.Context, offset int, limit int) ([]Space, int, bool, error)
	GetSpace(ctx context.Context, spaceId string) (Space, error)
	CreateSpace(ctx context.Context, request CreateSpaceRequest) (Space, error)
	ListMembers(ctx context.Context, spaceId string, offset int, limit int) ([]Member, int, bool, error)
	GetMember(ctx context.Context, spaceId string, memberId string) (Member, error)
	UpdateMember(ctx context.Context, spaceId string, memberId string, request UpdateMemberRequest) (Member, error)
	GetAllSpaceIds() ([]string, error)
}

type service struct {
	mw          apicore.ClientCommands
	gatewayUrl  string
	techSpaceId string
}

func NewService(mw apicore.ClientCommands, gatewayUrl string, techSpaceId string) Service {
	return &service{mw: mw, gatewayUrl: gatewayUrl, techSpaceId: techSpaceId}
}

// ListSpaces returns a paginated list of spaces for the account.
func (s *service) ListSpaces(ctx context.Context, offset int, limit int) (spaces []Space, total int, hasMore bool, err error) {
	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: s.techSpaceId,
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
				RelationKey:    bundle.RelationKeySpaceOrder.String(),
				Type:           model.BlockContentDataviewSort_Asc,
				NoCollate:      true,
				EmptyPlacement: model.BlockContentDataviewSort_End,
			},
		},
		Keys: []string{bundle.RelationKeyTargetSpaceId.String()},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedListSpaces
	}

	total = len(resp.Records)
	paginatedRecords, hasMore := pagination.Paginate(resp.Records, offset, limit)
	spaces = make([]Space, 0, len(paginatedRecords))

	for _, record := range paginatedRecords {
		workspace, err := s.getSpaceInfo(record.Fields[bundle.RelationKeyTargetSpaceId.String()].GetStringValue())
		if err != nil {
			return nil, 0, false, err
		}

		spaces = append(spaces, workspace)
	}

	return spaces, total, hasMore, nil
}

// GetSpace returns the space info for the space with the given ID.
func (s *service) GetSpace(ctx context.Context, spaceId string) (Space, error) {
	// Check if the workspace exists and is active
	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: s.techSpaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyTargetSpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(spaceId),
			},
			{
				RelationKey: bundle.RelationKeySpaceLocalStatus.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.SpaceStatus_Ok)),
			},
		},
		Keys: []string{bundle.RelationKeyTargetSpaceId.String()},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return Space{}, ErrFailedOpenWorkspace
	}

	if len(resp.Records) == 0 {
		return Space{}, ErrWorkspaceNotFound
	}

	return s.getSpaceInfo(spaceId)
}

// CreateSpace creates a new space with the given name and returns the space info.
func (s *service) CreateSpace(ctx context.Context, request CreateSpaceRequest) (Space, error) {
	name := request.Name
	iconOption, err := rand.Int(rand.Reader, big.NewInt(13))
	if err != nil {
		return Space{}, ErrFailedGenerateRandomIcon
	}

	resp := s.mw.WorkspaceCreate(ctx, &pb.RpcWorkspaceCreateRequest{
		Details: &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyName.String():             pbtypes.String(name),
				bundle.RelationKeyIconOption.String():       pbtypes.Float64(float64(iconOption.Int64())),
				bundle.RelationKeySpaceDashboardId.String(): pbtypes.String("lastOpened"),
			},
		},
		UseCase:  pb.RpcObjectImportUseCaseRequest_GET_STARTED,
		WithChat: true,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcWorkspaceCreateResponseError_NULL {
		return Space{}, ErrFailedCreateSpace
	}

	description := request.Description
	if description != "" {
		infoResp := s.mw.WorkspaceSetInfo(ctx, &pb.RpcWorkspaceSetInfoRequest{
			SpaceId: resp.SpaceId,
			Details: &types.Struct{
				Fields: map[string]*types.Value{
					bundle.RelationKeyDescription.String(): pbtypes.String(description),
				},
			},
		})

		if infoResp.Error != nil && infoResp.Error.Code != pb.RpcWorkspaceSetInfoResponseError_NULL {
			return Space{}, ErrFailedSetSpaceInfo
		}
	}

	return s.getSpaceInfo(resp.SpaceId)
}

// ListMembers returns a paginated list of members in the space with the given ID.
func (s *service) ListMembers(ctx context.Context, spaceId string, offset int, limit int) (members []Member, total int, hasMore bool, err error) {
	activeResp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
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
		Keys: []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String(), bundle.RelationKeyIconEmoji.String(), bundle.RelationKeyIconImage.String(), bundle.RelationKeyIdentity.String(), bundle.RelationKeyGlobalName.String(), bundle.RelationKeyParticipantPermissions.String(), bundle.RelationKeyParticipantStatus.String()},
	})

	if activeResp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedListMembers
	}

	joiningResp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
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
	members = make([]Member, 0, len(paginatedMembers))

	for _, record := range paginatedMembers {
		icon := object.GetIcon(s.gatewayUrl, record.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), record.Fields[bundle.RelationKeyIconImage.String()].GetStringValue(), "", 0)

		member := Member{
			Object:     "member",
			Id:         record.Fields[bundle.RelationKeyId.String()].GetStringValue(),
			Name:       record.Fields[bundle.RelationKeyName.String()].GetStringValue(),
			Icon:       icon,
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
func (s *service) GetMember(ctx context.Context, spaceId string, memberId string) (Member, error) {
	// Member ID can be either a participant ID or an identity.
	relationKey := bundle.RelationKeyId
	if !strings.HasPrefix(memberId, "_participant") {
		relationKey = bundle.RelationKeyIdentity
	}

	resp := s.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
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

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return Member{}, ErrFailedGetMember
	}

	if len(resp.Records) == 0 {
		return Member{}, ErrMemberNotFound
	}

	icon := object.GetIcon(s.gatewayUrl, "", resp.Records[0].Fields[bundle.RelationKeyIconImage.String()].GetStringValue(), "", 0)

	return Member{
		Object:     "member",
		Id:         resp.Records[0].Fields[bundle.RelationKeyId.String()].GetStringValue(),
		Name:       resp.Records[0].Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:       icon,
		Identity:   resp.Records[0].Fields[bundle.RelationKeyIdentity.String()].GetStringValue(),
		GlobalName: resp.Records[0].Fields[bundle.RelationKeyGlobalName.String()].GetStringValue(),
		Status:     strcase.ToSnake(model.ParticipantStatus_name[int32(resp.Records[0].Fields[bundle.RelationKeyParticipantStatus.String()].GetNumberValue())]),
		Role:       s.mapMemberPermissions(model.ParticipantPermissions(resp.Records[0].Fields[bundle.RelationKeyParticipantPermissions.String()].GetNumberValue())),
	}, nil
}

// UpdateMember approves member with a defined role or removes them
func (s *service) UpdateMember(ctx context.Context, spaceId string, memberId string, request UpdateMemberRequest) (Member, error) {
	member, err := s.GetMember(ctx, spaceId, memberId)
	if err != nil {
		return Member{}, err
	}

	if request.Status != "active" && request.Status != "removed" && request.Status != "declined" {
		return Member{}, ErrInvalidApproveMemberStatus
	}

	switch request.Status {
	case "active":
		if request.Role != "viewer" && request.Role != "editor" {
			return Member{}, ErrInvalidApproveMemberRole
		}

		if member.Status == "joining" {
			// Approve the member's join request.
			approveResp := s.mw.SpaceRequestApprove(ctx, &pb.RpcSpaceRequestApproveRequest{
				SpaceId:     spaceId,
				Identity:    memberId,
				Permissions: s.mapMemberRole(request.Role),
			})
			if approveResp.Error.Code != pb.RpcSpaceRequestApproveResponseError_NULL {
				return Member{}, ErrFailedUpdateMember
			}
		} else {
			// Update the member's role.
			resp := s.mw.SpaceParticipantPermissionsChange(ctx, &pb.RpcSpaceParticipantPermissionsChangeRequest{
				SpaceId: spaceId,
				Changes: []*model.ParticipantPermissionChange{{Identity: memberId, Perms: s.mapMemberRole(request.Role)}},
			})
			if resp.Error.Code != pb.RpcSpaceParticipantPermissionsChangeResponseError_NULL {
				return Member{}, ErrFailedUpdateMember
			}
		}
	case "declined":
		// Reject the member's join request.
		rejectResp := s.mw.SpaceRequestDecline(ctx, &pb.RpcSpaceRequestDeclineRequest{
			SpaceId:  spaceId,
			Identity: memberId,
		})
		if rejectResp.Error.Code != pb.RpcSpaceRequestDeclineResponseError_NULL {
			return Member{}, ErrFailedUpdateMember
		}
	case "removed":
		// Remove the member from the space.
		removeResp := s.mw.SpaceParticipantRemove(ctx, &pb.RpcSpaceParticipantRemoveRequest{
			SpaceId:    spaceId,
			Identities: []string{memberId},
		})
		if removeResp.Error.Code != pb.RpcSpaceParticipantRemoveResponseError_NULL {
			return Member{}, ErrFailedUpdateMember
		}
	default:
		return Member{}, ErrInvalidApproveMemberStatus
	}

	member, err = s.GetMember(ctx, spaceId, memberId)
	if err != nil {
		return Member{}, err
	}

	return member, nil
}

// getSpaceInfo returns the workspace info for the space with the given ID.
func (s *service) getSpaceInfo(spaceId string) (space Space, err error) {
	workspaceResponse := s.mw.WorkspaceOpen(context.Background(), &pb.RpcWorkspaceOpenRequest{
		SpaceId: spaceId,
	})

	if workspaceResponse.Error != nil && workspaceResponse.Error.Code != pb.RpcWorkspaceOpenResponseError_NULL {
		return Space{}, ErrFailedOpenWorkspace
	}

	spaceResp := s.mw.ObjectShow(context.Background(), &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: workspaceResponse.Info.WorkspaceObjectId,
	})

	if spaceResp.Error != nil && spaceResp.Error.Code != pb.RpcObjectShowResponseError_NULL {
		return Space{}, ErrFailedOpenSpace
	}

	name := spaceResp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyName.String()].GetStringValue()
	icon := object.GetIcon(s.gatewayUrl, spaceResp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), spaceResp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconImage.String()].GetStringValue(), "", 0)
	description := spaceResp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyDescription.String()].GetStringValue()

	return Space{
		Object:      "space",
		Id:          spaceId,
		Name:        name,
		Icon:        icon,
		Description: description,
		GatewayUrl:  workspaceResponse.Info.GatewayUrl,
		NetworkId:   workspaceResponse.Info.NetworkId,
	}, nil
}

// GetAllSpaceIds effectively retrieves all space IDs from the tech space.
func (s *service) GetAllSpaceIds() ([]string, error) {
	resp := s.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: s.techSpaceId,
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
		Keys: []string{bundle.RelationKeyTargetSpaceId.String()},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, ErrFailedListSpaces
	}

	spaceIds := make([]string, 0, len(resp.Records))
	for _, record := range resp.Records {
		if id := record.Fields[bundle.RelationKeyTargetSpaceId.String()].GetStringValue(); id != "" {
			spaceIds = append(spaceIds, id)
		}
	}

	return spaceIds, nil
}

// mapMemberPermissions maps participant permissions to a role
func (s *service) mapMemberPermissions(permissions model.ParticipantPermissions) string {
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
func (s *service) mapMemberRole(role string) model.ParticipantPermissions {
	switch role {
	case "viewer":
		return model.ParticipantPermissions_Reader
	case "editor":
		return model.ParticipantPermissions_Writer
	default:
		return model.ParticipantPermissions_Reader
	}
}
