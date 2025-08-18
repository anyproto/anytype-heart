package service

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"

	"github.com/gogo/protobuf/types"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ErrFailedListSpaces         = errors.New("failed to retrieve list of spaces")
	ErrFailedOpenWorkspace      = errors.New("failed to open workspace")
	ErrFailedOpenSpace          = errors.New("failed to open space")
	ErrWorkspaceNotFound        = errors.New("workspace not found")
	ErrFailedGenerateRandomIcon = errors.New("failed to generate random icon")
	ErrFailedCreateSpace        = errors.New("failed to create space")
	ErrFailedSetSpaceInfo       = errors.New("failed to set space info")
	ErrFailedListMembers        = errors.New("failed to retrieve list of members")
	ErrFailedGetMember          = errors.New("failed to retrieve member")
	ErrMemberNotFound           = errors.New("member not found")
	ErrInvalidApproveMemberRole = errors.New("role must be 'reader' or 'writer'")
	ErrFailedUpdateMember       = errors.New("failed to update member")
)

// ListSpaces returns a paginated list of spaces for the account.
func (s *Service) ListSpaces(ctx context.Context, additionalFilters []*model.BlockContentDataviewFilter, offset int, limit int) (spaces []apimodel.Space, total int, hasMore bool, err error) {
	filters := append([]*model.BlockContentDataviewFilter{
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
	}, additionalFilters...)

	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: s.techSpaceId,
		Filters: filters,
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
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedListSpaces
	}

	total = len(resp.Records)
	paginatedRecords, hasMore := pagination.Paginate(resp.Records, offset, limit)
	spaces = make([]apimodel.Space, 0, len(paginatedRecords))

	for _, record := range paginatedRecords {
		workspace, err := s.getSpaceInfo(ctx, record.Fields[bundle.RelationKeyTargetSpaceId.String()].GetStringValue())
		if err != nil {
			return nil, 0, false, err
		}

		spaces = append(spaces, workspace)
	}

	return spaces, total, hasMore, nil
}

// GetSpace returns the space info for the space with the given ID.
func (s *Service) GetSpace(ctx context.Context, spaceId string) (apimodel.Space, error) {
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
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return apimodel.Space{}, ErrFailedOpenWorkspace
	}

	if len(resp.Records) == 0 {
		return apimodel.Space{}, ErrWorkspaceNotFound
	}

	return s.getSpaceInfo(ctx, spaceId)
}

// CreateSpace creates a new space with the given name and returns the space info.
func (s *Service) CreateSpace(ctx context.Context, request apimodel.CreateSpaceRequest) (apimodel.Space, error) {
	iconOption, err := rand.Int(rand.Reader, big.NewInt(13))
	if err != nil {
		return apimodel.Space{}, ErrFailedGenerateRandomIcon
	}

	resp := s.mw.WorkspaceCreate(ctx, &pb.RpcWorkspaceCreateRequest{
		Details: &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyName.String():             pbtypes.String(s.sanitizedString(*request.Name)),
				bundle.RelationKeyIconOption.String():       pbtypes.Float64(float64(iconOption.Int64())),
				bundle.RelationKeySpaceDashboardId.String(): pbtypes.String("lastOpened"),
			},
		},
		UseCase:  pb.RpcObjectImportUseCaseRequest_GET_STARTED,
		WithChat: true,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcWorkspaceCreateResponseError_NULL {
		return apimodel.Space{}, ErrFailedCreateSpace
	}

	if request.Description != nil {
		infoResp := s.mw.WorkspaceSetInfo(ctx, &pb.RpcWorkspaceSetInfoRequest{
			SpaceId: resp.SpaceId,
			Details: &types.Struct{
				Fields: map[string]*types.Value{
					bundle.RelationKeyDescription.String(): pbtypes.String(s.sanitizedString(*request.Description)),
				},
			},
		})

		if infoResp.Error != nil && infoResp.Error.Code != pb.RpcWorkspaceSetInfoResponseError_NULL {
			return apimodel.Space{}, ErrFailedSetSpaceInfo
		}
	}

	return s.getSpaceInfo(ctx, resp.SpaceId)
}

// UpdateSpace updates the space with the given ID using the provided request.
func (s *Service) UpdateSpace(ctx context.Context, spaceId string, request apimodel.UpdateSpaceRequest) (apimodel.Space, error) {
	_, err := s.GetSpace(ctx, spaceId)
	if err != nil {
		return apimodel.Space{}, err
	}

	fields := make(map[string]*types.Value)
	if request.Name != nil {
		fields[bundle.RelationKeyName.String()] = pbtypes.String(s.sanitizedString(*request.Name))
	}

	if request.Description != nil {
		fields[bundle.RelationKeyDescription.String()] = pbtypes.String(s.sanitizedString(*request.Description))
	}

	if len(fields) > 0 {
		resp := s.mw.WorkspaceSetInfo(ctx, &pb.RpcWorkspaceSetInfoRequest{
			SpaceId: spaceId,
			Details: &types.Struct{Fields: fields},
		})

		if resp.Error != nil && resp.Error.Code != pb.RpcWorkspaceSetInfoResponseError_NULL {
			return apimodel.Space{}, ErrFailedSetSpaceInfo
		}
	}

	space, err := s.getSpaceInfo(ctx, spaceId)
	if err != nil {
		return apimodel.Space{}, err
	}

	return space, nil
}

// getSpaceInfo returns the workspace info for the space with the given ID.
func (s *Service) getSpaceInfo(ctx context.Context, spaceId string) (space apimodel.Space, err error) {
	workspaceResponse := s.mw.WorkspaceOpen(ctx, &pb.RpcWorkspaceOpenRequest{
		SpaceId: spaceId,
	})

	if workspaceResponse.Error != nil && workspaceResponse.Error.Code != pb.RpcWorkspaceOpenResponseError_NULL {
		return apimodel.Space{}, ErrFailedOpenWorkspace
	}

	spaceResp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: workspaceResponse.Info.WorkspaceObjectId,
	})

	if spaceResp.Error != nil && spaceResp.Error.Code != pb.RpcObjectShowResponseError_NULL {
		return apimodel.Space{}, ErrFailedOpenSpace
	}

	name := spaceResp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyName.String()].GetStringValue()
	icon := getIcon(s.gatewayUrl, "", spaceResp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconImage.String()].GetStringValue(), "", 0)
	description := spaceResp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyDescription.String()].GetStringValue()

	return apimodel.Space{
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
func (s *Service) GetAllSpaceIds(ctx context.Context) ([]string, error) {
	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: s.techSpaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
			},
			{
				RelationKey: bundle.RelationKeySpaceAccountStatus.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.IntList(int(model.SpaceStatus_Unknown), int(model.SpaceStatus_SpaceActive)),
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
