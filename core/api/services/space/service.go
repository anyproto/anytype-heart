package space

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ErrNoSpacesFound            = errors.New("no spaces found")
	ErrFailedListSpaces         = errors.New("failed to retrieve list of spaces")
	ErrFailedOpenWorkspace      = errors.New("failed to open workspace")
	ErrFailedGenerateRandomIcon = errors.New("failed to generate random icon")
	ErrFailedCreateSpace        = errors.New("failed to create space")
	ErrBadInput                 = errors.New("bad input")
	ErrNoMembersFound           = errors.New("no members found")
	ErrFailedListMembers        = errors.New("failed to retrieve list of members")
)

type Service interface {
	ListSpaces(ctx context.Context, offset int, limit int) ([]Space, int, bool, error)
	CreateSpace(ctx context.Context, name string) (Space, error)
	ListMembers(ctx context.Context, spaceId string, offset int, limit int) ([]Member, int, bool, error)
}

type SpaceService struct {
	mw          service.ClientCommandsServer
	AccountInfo *model.AccountInfo
}

func NewService(mw service.ClientCommandsServer) *SpaceService {
	return &SpaceService{mw: mw}
}

// ListSpaces returns a paginated list of spaces for the account.
func (s *SpaceService) ListSpaces(ctx context.Context, offset int, limit int) (spaces []Space, total int, hasMore bool, err error) {
	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: s.AccountInfo.TechSpaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				Operator:    model.BlockContentDataviewFilter_No,
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
			},
			{
				Operator:    model.BlockContentDataviewFilter_No,
				RelationKey: bundle.RelationKeySpaceLocalStatus.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.SpaceStatus_Ok)),
			},
			{
				Operator:    model.BlockContentDataviewFilter_No,
				RelationKey: bundle.RelationKeySpaceRemoteStatus.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.SpaceStatus_Ok)),
			},
		},
		Sorts: []*model.BlockContentDataviewSort{
			{
				RelationKey:    string(bundle.RelationKeySpaceOrder),
				Type:           model.BlockContentDataviewSort_Asc,
				NoCollate:      true,
				EmptyPlacement: model.BlockContentDataviewSort_End,
			},
		},
		Keys: []string{string(bundle.RelationKeyTargetSpaceId), string(bundle.RelationKeyName), string(bundle.RelationKeyIconEmoji), string(bundle.RelationKeyIconImage)},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedListSpaces
	}

	if len(resp.Records) == 0 {
		return nil, 0, false, ErrNoSpacesFound
	}

	total = len(resp.Records)
	paginatedRecords, hasMore := pagination.Paginate(resp.Records, offset, limit)
	spaces = make([]Space, 0, len(paginatedRecords))

	for _, record := range paginatedRecords {
		workspace, err := s.getWorkspaceInfo(record.Fields[string(bundle.RelationKeyTargetSpaceId)].GetStringValue())
		if err != nil {
			return nil, 0, false, err
		}

		// TODO: name and icon are only returned here; fix that
		workspace.Name = record.Fields[string(bundle.RelationKeyName)].GetStringValue()
		workspace.Icon = util.GetIconFromEmojiOrImage(s.AccountInfo, record.Fields[string(bundle.RelationKeyIconEmoji)].GetStringValue(), record.Fields[string(bundle.RelationKeyIconImage)].GetStringValue())

		spaces = append(spaces, workspace)
	}

	return spaces, total, hasMore, nil
}

// CreateSpace creates a new space with the given name and returns the space info.
func (s *SpaceService) CreateSpace(ctx context.Context, name string) (Space, error) {
	iconOption, err := rand.Int(rand.Reader, big.NewInt(13))
	if err != nil {
		return Space{}, ErrFailedGenerateRandomIcon
	}

	// Create new workspace with a random icon and import default use case
	resp := s.mw.WorkspaceCreate(ctx, &pb.RpcWorkspaceCreateRequest{
		Details: &types.Struct{
			Fields: map[string]*types.Value{
				string(bundle.RelationKeyIconOption):       pbtypes.Float64(float64(iconOption.Int64())),
				string(bundle.RelationKeyName):             pbtypes.String(name),
				string(bundle.RelationKeySpaceDashboardId): pbtypes.String("lastOpened"),
			},
		},
		UseCase:  pb.RpcObjectImportUseCaseRequest_GET_STARTED,
		WithChat: true,
	})

	if resp.Error.Code != pb.RpcWorkspaceCreateResponseError_NULL {
		return Space{}, ErrFailedCreateSpace
	}

	return s.getWorkspaceInfo(resp.SpaceId)
}

// ListMembers returns a paginated list of members in the space with the given ID.
func (s *SpaceService) ListMembers(ctx context.Context, spaceId string, offset int, limit int) (members []Member, total int, hasMore bool, err error) {
	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				Operator:    model.BlockContentDataviewFilter_No,
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_participant)),
			},
			{
				Operator:    model.BlockContentDataviewFilter_No,
				RelationKey: bundle.RelationKeyParticipantStatus.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ParticipantStatus_Active)),
			},
		},
		Sorts: []*model.BlockContentDataviewSort{
			{
				RelationKey: string(bundle.RelationKeyName),
				Type:        model.BlockContentDataviewSort_Asc,
			},
		},
		Keys: []string{string(bundle.RelationKeyId), string(bundle.RelationKeyName), string(bundle.RelationKeyIconEmoji), string(bundle.RelationKeyIconImage), string(bundle.RelationKeyIdentity), string(bundle.RelationKeyGlobalName), string(bundle.RelationKeyParticipantPermissions)},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedListMembers
	}

	if len(resp.Records) == 0 {
		return nil, 0, false, ErrNoMembersFound
	}

	total = len(resp.Records)
	paginatedMembers, hasMore := pagination.Paginate(resp.Records, offset, limit)
	members = make([]Member, 0, len(paginatedMembers))

	for _, record := range paginatedMembers {
		icon := util.GetIconFromEmojiOrImage(s.AccountInfo, record.Fields[string(bundle.RelationKeyIconEmoji)].GetStringValue(), record.Fields[string(bundle.RelationKeyIconImage)].GetStringValue())

		member := Member{
			Type:       "member",
			Id:         record.Fields[string(bundle.RelationKeyId)].GetStringValue(),
			Name:       record.Fields[string(bundle.RelationKeyName)].GetStringValue(),
			Icon:       icon,
			Identity:   record.Fields[string(bundle.RelationKeyIdentity)].GetStringValue(),
			GlobalName: record.Fields[string(bundle.RelationKeyGlobalName)].GetStringValue(),
			Role:       model.ParticipantPermissions_name[int32(record.Fields[string(bundle.RelationKeyParticipantPermissions)].GetNumberValue())],
		}

		members = append(members, member)
	}

	return members, total, hasMore, nil
}

func (s *SpaceService) GetParticipantDetails(mw service.ClientCommandsServer, accountInfo *model.AccountInfo, spaceId string, participantId string) Member {
	resp := mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				Operator:    model.BlockContentDataviewFilter_No,
				RelationKey: bundle.RelationKeyId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(participantId),
			},
		},
		Keys: []string{string(bundle.RelationKeyId), string(bundle.RelationKeyName), string(bundle.RelationKeyIconEmoji), string(bundle.RelationKeyIconImage), string(bundle.RelationKeyIdentity), string(bundle.RelationKeyGlobalName), string(bundle.RelationKeyParticipantPermissions)},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return Member{}
	}

	if len(resp.Records) == 0 {
		return Member{}
	}

	icon := util.GetIconFromEmojiOrImage(accountInfo, "", resp.Records[0].Fields[string(bundle.RelationKeyIconImage)].GetStringValue())

	return Member{
		Type:       "member",
		Id:         resp.Records[0].Fields[string(bundle.RelationKeyId)].GetStringValue(),
		Name:       resp.Records[0].Fields[string(bundle.RelationKeyName)].GetStringValue(),
		Icon:       icon,
		Identity:   resp.Records[0].Fields[string(bundle.RelationKeyIdentity)].GetStringValue(),
		GlobalName: resp.Records[0].Fields[string(bundle.RelationKeyGlobalName)].GetStringValue(),
		Role:       model.ParticipantPermissions_name[int32(resp.Records[0].Fields[string(bundle.RelationKeyParticipantPermissions)].GetNumberValue())],
	}
}

// getWorkspaceInfo returns the workspace info for the space with the given ID.
func (s *SpaceService) getWorkspaceInfo(spaceId string) (space Space, err error) {
	workspaceResponse := s.mw.WorkspaceOpen(context.Background(), &pb.RpcWorkspaceOpenRequest{
		SpaceId:  spaceId,
		WithChat: true,
	})

	if workspaceResponse.Error.Code != pb.RpcWorkspaceOpenResponseError_NULL {
		return Space{}, ErrFailedOpenWorkspace
	}

	return Space{
		Type:                   "space",
		Id:                     spaceId,
		HomeObjectId:           workspaceResponse.Info.HomeObjectId,
		ArchiveObjectId:        workspaceResponse.Info.ArchiveObjectId,
		ProfileObjectId:        workspaceResponse.Info.ProfileObjectId,
		MarketplaceWorkspaceId: workspaceResponse.Info.MarketplaceWorkspaceId,
		WorkspaceObjectId:      workspaceResponse.Info.WorkspaceObjectId,
		DeviceId:               workspaceResponse.Info.DeviceId,
		AccountSpaceId:         workspaceResponse.Info.AccountSpaceId,
		WidgetsId:              workspaceResponse.Info.WidgetsId,
		SpaceViewId:            workspaceResponse.Info.SpaceViewId,
		TechSpaceId:            workspaceResponse.Info.TechSpaceId,
		GatewayUrl:             workspaceResponse.Info.GatewayUrl,
		LocalStoragePath:       workspaceResponse.Info.LocalStoragePath,
		Timezone:               workspaceResponse.Info.TimeZone,
		AnalyticsId:            workspaceResponse.Info.AnalyticsId,
		NetworkId:              workspaceResponse.Info.NetworkId,
	}, nil
}
