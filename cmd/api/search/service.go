package search

import (
	"context"
	"errors"
	"sort"

	"github.com/anyproto/anytype-heart/cmd/api/object"
	"github.com/anyproto/anytype-heart/cmd/api/pagination"
	"github.com/anyproto/anytype-heart/cmd/api/space"
	"github.com/anyproto/anytype-heart/cmd/api/util"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ErrFailedSearchObjects = errors.New("failed to retrieve objects from space")
	ErrNoObjectsFound      = errors.New("no objects found")
)

type Service interface {
	Search(ctx context.Context) ([]object.Object, error)
}

type SearchService struct {
	mw            service.ClientCommandsServer
	spaceService  *space.SpaceService
	objectService *object.ObjectService
	AccountInfo   *model.AccountInfo
}

func NewService(mw service.ClientCommandsServer, spaceService *space.SpaceService, objectService *object.ObjectService) *SearchService {
	return &SearchService{mw: mw, spaceService: spaceService, objectService: objectService}
}

func (s *SearchService) Search(ctx context.Context, searchQuery string, objectType string, offset, limit int) (objects []object.Object, total int, hasMore bool, err error) {
	spaces, _, _, err := s.spaceService.ListSpaces(ctx, 0, 100)
	if err != nil {
		return nil, 0, false, err
	}

	// Then, get objects from each space that match the search parameters
	var filters = []*model.BlockContentDataviewFilter{
		{
			RelationKey: bundle.RelationKeyLayout.String(),
			Condition:   model.BlockContentDataviewFilter_In,
			Value: pbtypes.IntList([]int{
				int(model.ObjectType_basic),
				int(model.ObjectType_profile),
				int(model.ObjectType_todo),
				int(model.ObjectType_note),
				int(model.ObjectType_bookmark),
				int(model.ObjectType_set),
				int(model.ObjectType_collection),
				int(model.ObjectType_participant),
			}...),
		},
		{
			RelationKey: bundle.RelationKeyIsHidden.String(),
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       pbtypes.Bool(true),
		},
	}

	if searchQuery != "" {
		// TODO also include snippet for notes
		filters = append(filters, &model.BlockContentDataviewFilter{
			RelationKey: bundle.RelationKeyName.String(),
			Condition:   model.BlockContentDataviewFilter_Like,
			Value:       pbtypes.String(searchQuery),
		})
	}

	if objectType != "" {
		filters = append(filters, &model.BlockContentDataviewFilter{
			RelationKey: bundle.RelationKeyType.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.String(objectType),
		})
	}

	results := make([]object.Object, 0)
	for _, space := range spaces {
		spaceId := space.Id
		objResp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
			SpaceId: spaceId,
			Filters: filters,
			Sorts: []*model.BlockContentDataviewSort{{
				RelationKey:    bundle.RelationKeyLastModifiedDate.String(),
				Type:           model.BlockContentDataviewSort_Desc,
				Format:         model.RelationFormat_longtext,
				IncludeTime:    true,
				EmptyPlacement: model.BlockContentDataviewSort_NotSpecified,
			}},
			Keys: []string{"id", "name", "type", "layout", "iconEmoji", "iconImage"},
			// TODO split limit between spaces
			// Limit: paginationLimitPerSpace,
			// FullText: searchTerm,
		})

		if objResp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
			return nil, 0, false, ErrFailedSearchObjects
		}

		if len(objResp.Records) == 0 {
			continue
		}

		for _, record := range objResp.Records {
			icon := util.GetIconFromEmojiOrImage(s.AccountInfo, record.Fields["iconEmoji"].GetStringValue(), record.Fields["iconImage"].GetStringValue())
			objectTypeName, err := util.ResolveTypeToName(s.mw, spaceId, record.Fields["type"].GetStringValue())
			if err != nil {
				return nil, 0, false, err
			}

			showResp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
				SpaceId:  spaceId,
				ObjectId: record.Fields["id"].GetStringValue(),
			})

			results = append(results, object.Object{
				Type:       model.ObjectTypeLayout_name[int32(record.Fields["layout"].GetNumberValue())],
				Id:         record.Fields["id"].GetStringValue(),
				Name:       record.Fields["name"].GetStringValue(),
				Icon:       icon,
				ObjectType: objectTypeName,
				SpaceId:    spaceId,
				RootId:     showResp.ObjectView.RootId,
				Blocks:     s.objectService.GetBlocks(showResp),
				Details:    s.objectService.GetDetails(showResp),
			})
		}
	}

	if len(results) == 0 {
		return nil, 0, false, ErrNoObjectsFound
	}

	// sort after lastModifiedDate to achieve descending sort order across all spaces
	sort.Slice(results, func(i, j int) bool {
		return results[i].Details[0].Details["lastModifiedDate"].(float64) > results[j].Details[0].Details["lastModifiedDate"].(float64)
	})

	// TODO: solve global pagination vs per space pagination
	total = len(results)
	paginatedResults, hasMore := pagination.Paginate(results, offset, limit)
	return paginatedResults, total, hasMore, nil
}
