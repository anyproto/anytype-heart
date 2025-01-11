package search

import (
	"context"
	"errors"
	"sort"
	"strings"

	object2 "github.com/anyproto/anytype-heart/core/api/object"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/space"
	"github.com/anyproto/anytype-heart/core/api/util"
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
	Search(ctx context.Context, searchQuery string, objectTypes []string, offset, limit int) (objects []object2.Object, total int, hasMore bool, err error)
}

type SearchService struct {
	mw            service.ClientCommandsServer
	spaceService  *space.SpaceService
	objectService *object2.ObjectService
	AccountInfo   *model.AccountInfo
}

func NewService(mw service.ClientCommandsServer, spaceService *space.SpaceService, objectService *object2.ObjectService) *SearchService {
	return &SearchService{mw: mw, spaceService: spaceService, objectService: objectService}
}

// Search retrieves a paginated list of objects from all spaces that match the search parameters.
func (s *SearchService) Search(ctx context.Context, searchQuery string, objectTypes []string, offset, limit int) (objects []object2.Object, total int, hasMore bool, err error) {
	spaces, _, _, err := s.spaceService.ListSpaces(ctx, 0, 100)
	if err != nil {
		return nil, 0, false, err
	}

	baseFilters := s.prepareBaseFilters()
	queryFilters := s.prepareQueryFilter(searchQuery)

	results := make([]object2.Object, 0)
	for _, space := range spaces {
		// Resolve object type IDs per space, as they are unique per space
		objectTypeFilters := s.prepareObjectTypeFilters(space.Id, objectTypes)
		filters := s.combineFilters(model.BlockContentDataviewFilter_And, baseFilters, queryFilters, objectTypeFilters)

		objResp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
			SpaceId: space.Id,
			Filters: filters,
			Sorts: []*model.BlockContentDataviewSort{{
				RelationKey:    bundle.RelationKeyLastModifiedDate.String(),
				Type:           model.BlockContentDataviewSort_Desc,
				Format:         model.RelationFormat_date,
				IncludeTime:    true,
				EmptyPlacement: model.BlockContentDataviewSort_NotSpecified,
			}},
			Keys: []string{"id", "name"},
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
			object, err := s.objectService.GetObject(ctx, space.Id, record.Fields["id"].GetStringValue())
			if err != nil {
				return nil, 0, false, err
			}
			results = append(results, object)
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

// makeAndCondition combines multiple filter groups with the given operator.
func (s *SearchService) combineFilters(operator model.BlockContentDataviewFilterOperator, filterGroups ...[]*model.BlockContentDataviewFilter) []*model.BlockContentDataviewFilter {
	nestedFilters := make([]*model.BlockContentDataviewFilter, 0)
	for _, group := range filterGroups {
		if len(group) > 0 {
			nestedFilters = append(nestedFilters, group...)
		}
	}

	if len(nestedFilters) == 0 {
		return nil
	}

	return []*model.BlockContentDataviewFilter{
		{
			Operator:      operator,
			NestedFilters: nestedFilters,
		},
	}
}

// prepareBaseFilters returns a list of default filters that should be applied to all search queries.
func (s *SearchService) prepareBaseFilters() []*model.BlockContentDataviewFilter {
	return []*model.BlockContentDataviewFilter{
		{
			Operator:    model.BlockContentDataviewFilter_No,
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
			Operator:    model.BlockContentDataviewFilter_No,
			RelationKey: bundle.RelationKeyIsHidden.String(),
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       pbtypes.Bool(true),
		},
	}
}

// prepareQueryFilter combines object name and snippet filters with an OR condition.
func (s *SearchService) prepareQueryFilter(searchQuery string) []*model.BlockContentDataviewFilter {
	if searchQuery == "" {
		return nil
	}

	return []*model.BlockContentDataviewFilter{
		{
			Operator: model.BlockContentDataviewFilter_Or,
			NestedFilters: []*model.BlockContentDataviewFilter{
				{
					Operator:    model.BlockContentDataviewFilter_No,
					RelationKey: bundle.RelationKeyName.String(),
					Condition:   model.BlockContentDataviewFilter_Like,
					Value:       pbtypes.String(searchQuery),
				},
				{
					Operator:    model.BlockContentDataviewFilter_No,
					RelationKey: bundle.RelationKeySnippet.String(),
					Condition:   model.BlockContentDataviewFilter_Like,
					Value:       pbtypes.String(searchQuery),
				},
			},
		},
	}
}

// prepareObjectTypeFilters combines object type filters with an OR condition.
func (s *SearchService) prepareObjectTypeFilters(spaceId string, objectTypes []string) []*model.BlockContentDataviewFilter {
	if len(objectTypes) == 0 || objectTypes[0] == "" {
		return nil
	}

	// Prepare nested filters for each object type
	nestedFilters := make([]*model.BlockContentDataviewFilter, 0, len(objectTypes))
	for _, objectType := range objectTypes {
		typeId := objectType

		if strings.HasPrefix(objectType, "ot-") {
			var err error
			typeId, err = util.ResolveUniqueKeyToTypeId(s.mw, spaceId, objectType)
			if err != nil {
				continue
			}
		}

		nestedFilters = append(nestedFilters, &model.BlockContentDataviewFilter{
			Operator:    model.BlockContentDataviewFilter_No,
			RelationKey: bundle.RelationKeyType.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.String(typeId),
		})
	}

	if len(nestedFilters) == 0 {
		return nil
	}

	// Combine all filters with an OR operator
	return []*model.BlockContentDataviewFilter{
		{
			Operator:      model.BlockContentDataviewFilter_Or,
			NestedFilters: nestedFilters,
		},
	}
}
