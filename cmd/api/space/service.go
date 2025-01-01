package space

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/cmd/api/pagination"
	"github.com/anyproto/anytype-heart/cmd/api/util"
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
	ErrNoMembersFound           = errors.New("no members found")
	ErrFailedListMembers        = errors.New("failed to retrieve list of members")
)

type Service interface {
	ListSpaces(ctx context.Context, offset int, limit int) ([]Space, int, bool, error)
	CreateSpace(ctx context.Context, name string) (Space, error)
}

type SpaceService struct {
	mw          service.ClientCommandsServer
	AccountInfo *model.AccountInfo
}

func NewService(mw service.ClientCommandsServer) *SpaceService {
	return &SpaceService{mw: mw}
}

func (s *SpaceService) ListSpaces(ctx context.Context, offset int, limit int) (spaces []Space, total int, hasMore bool, err error) {
	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: s.AccountInfo.TechSpaceId,
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
		workspace, err := s.getWorkspaceInfo(record.Fields["targetSpaceId"].GetStringValue())
		if err != nil {
			return nil, 0, false, err
		}

		// TODO: name and icon are only returned here; fix that
		workspace.Name = record.Fields["name"].GetStringValue()
		workspace.Icon = util.GetIconFromEmojiOrImage(s.AccountInfo, record.Fields["iconEmoji"].GetStringValue(), record.Fields["iconImage"].GetStringValue())

		spaces = append(spaces, workspace)
	}

	return spaces, total, hasMore, nil
}

func (s *SpaceService) CreateSpace(ctx context.Context, name string) (Space, error) {
	iconOption, err := rand.Int(rand.Reader, big.NewInt(13))
	if err != nil {
		return Space{}, ErrFailedGenerateRandomIcon
	}

	// Create new workspace with a random icon and import default use case
	resp := s.mw.WorkspaceCreate(ctx, &pb.RpcWorkspaceCreateRequest{
		Details: &types.Struct{
			Fields: map[string]*types.Value{
				"iconOption":       pbtypes.Float64(float64(iconOption.Int64())),
				"name":             pbtypes.String(name),
				"spaceDashboardId": pbtypes.String("lastOpened"),
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

func (s *SpaceService) ListMembers(ctx context.Context, spaceId string, offset int, limit int) (members []Member, total int, hasMore bool, err error) {
	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
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
				RelationKey: "name",
				Type:        model.BlockContentDataviewSort_Asc,
			},
		},
		Keys: []string{"id", "name", "iconEmoji", "iconImage", "identity", "globalName", "participantPermissions"},
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
		icon := util.GetIconFromEmojiOrImage(s.AccountInfo, record.Fields["iconEmoji"].GetStringValue(), record.Fields["iconImage"].GetStringValue())

		member := Member{
			Type:       "space_member",
			Id:         record.Fields["id"].GetStringValue(),
			Name:       record.Fields["name"].GetStringValue(),
			Icon:       icon,
			Identity:   record.Fields["identity"].GetStringValue(),
			GlobalName: record.Fields["globalName"].GetStringValue(),
			Role:       model.ParticipantPermissions_name[int32(record.Fields["participantPermissions"].GetNumberValue())],
		}

		members = append(members, member)
	}

	return members, total, hasMore, nil
}

// getWorkspaceInfo returns the workspace info for the space with the given ID
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
