package search

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/anyproto/anytype-heart/core/api/internal/object"
	"github.com/anyproto/anytype-heart/core/api/internal/space"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	spaceLimit             = 64
	ErrFailedSearchObjects = errors.New("failed to retrieve objects from space")
)

type Service interface {
	GlobalSearch(ctx context.Context, request SearchRequest, offset int, limit int) (objects []object.Object, total int, hasMore bool, err error)
	Search(ctx context.Context, spaceId string, request SearchRequest, offset int, limit int) (objects []object.Object, total int, hasMore bool, err error)
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

// GlobalSearch retrieves a paginated list of objects from all spaces that match the search parameters.
func (s *SearchService) GlobalSearch(ctx context.Context, request SearchRequest, offset int, limit int) (objects []object.Object, total int, hasMore bool, err error) {
	spaces, _, _, err := s.spaceService.ListSpaces(ctx, 0, spaceLimit)
	if err != nil {
		return nil, 0, false, err
	}

	baseFilters := s.prepareBaseFilters()
	queryFilters := s.prepareQueryFilter(request.Query)
	sorts := s.prepareSorts(request.Sort)
	dateToSortAfter := sorts[0].RelationKey

	allResponses := make([]*pb.RpcObjectSearchResponse, 0, len(spaces))
	for _, space := range spaces {
		// Resolve template type and object type IDs per space, as they are unique per space
		templateFilter := s.prepareTemplateFilter(space.Id)
		typeFilters := s.prepareObjectTypeFilters(space.Id, request.Types)
		filters := s.combineFilters(model.BlockContentDataviewFilter_And, baseFilters, templateFilter, queryFilters, typeFilters)

		objResp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
			SpaceId: space.Id,
			Filters: filters,
			Sorts:   sorts,
			Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeySpaceId.String(), dateToSortAfter},
			Limit:   int32(offset + limit), // nolint: gosec
		})

		if objResp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
			return nil, 0, false, ErrFailedSearchObjects
		}

		allResponses = append(allResponses, objResp)
	}

	combinedRecords := make([]struct {
		Id              string
		SpaceId         string
		DateToSortAfter float64
	}, 0)
	for _, objResp := range allResponses {
		for _, record := range objResp.Records {
			combinedRecords = append(combinedRecords, struct {
				Id              string
				SpaceId         string
				DateToSortAfter float64
			}{
				Id:              record.Fields[bundle.RelationKeyId.String()].GetStringValue(),
				SpaceId:         record.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(),
				DateToSortAfter: record.Fields[dateToSortAfter].GetNumberValue(),
			})
		}
	}

	// sort after posix last_modified_date to achieve descending sort order across all spaces
	sort.SliceStable(combinedRecords, func(i, j int) bool {
		return combinedRecords[i].DateToSortAfter > combinedRecords[j].DateToSortAfter
	})

	total = len(combinedRecords)
	paginatedRecords, hasMore := pagination.Paginate(combinedRecords, offset, limit)

	results := make([]object.Object, 0, len(paginatedRecords))
	for _, record := range paginatedRecords {
		object, err := s.objectService.GetObject(ctx, record.SpaceId, record.Id)
		if err != nil {
			return nil, 0, false, err
		}
		results = append(results, object)
	}

	return results, total, hasMore, nil
}

// Search retrieves a paginated list of objects from a specific space that match the search parameters.
func (s *SearchService) Search(ctx context.Context, spaceId string, request SearchRequest, offset int, limit int) (objects []object.Object, total int, hasMore bool, err error) {
	baseFilters := s.prepareBaseFilters()
	templateFilter := s.prepareTemplateFilter(spaceId)
	queryFilters := s.prepareQueryFilter(request.Query)
	typeFilters := s.prepareObjectTypeFilters(spaceId, request.Types)
	filters := s.combineFilters(model.BlockContentDataviewFilter_And, baseFilters, templateFilter, queryFilters, typeFilters)

	sorts := s.prepareSorts(request.Sort)
	dateToSortAfter := sorts[0].RelationKey

	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: filters,
		Sorts:   sorts,
		Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeySpaceId.String(), dateToSortAfter},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedSearchObjects
	}

	total = len(resp.Records)
	paginatedRecords, hasMore := pagination.Paginate(resp.Records, offset, limit)

	results := make([]object.Object, 0, len(paginatedRecords))
	for _, record := range paginatedRecords {
		object, err := s.objectService.GetObject(ctx, record.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(), record.Fields[bundle.RelationKeyId.String()].GetStringValue())
		if err != nil {
			return nil, 0, false, err
		}
		results = append(results, object)
	}

	return results, total, hasMore, nil
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
			RelationKey: bundle.RelationKeyResolvedLayout.String(),
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

// prepareTemplateFilter returns a filter that excludes templates from the search results.
func (s *SearchService) prepareTemplateFilter(spaceId string) []*model.BlockContentDataviewFilter {
	typeId, err := util.ResolveUniqueKeyToTypeId(s.mw, spaceId, "ot-template")
	if err != nil {
		return nil
	}

	return []*model.BlockContentDataviewFilter{
		{
			Operator:    model.BlockContentDataviewFilter_No,
			RelationKey: bundle.RelationKeyType.String(),
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       pbtypes.String(typeId),
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

// prepareSorts returns a sort filter based on the given sort parameters
func (s *SearchService) prepareSorts(sort SortOptions) []*model.BlockContentDataviewSort {
	primarySort := &model.BlockContentDataviewSort{
		RelationKey:    s.getSortRelationKey(sort.Timestamp),
		Type:           s.getSortDirection(sort.Direction),
		Format:         model.RelationFormat_date,
		IncludeTime:    true,
		EmptyPlacement: model.BlockContentDataviewSort_NotSpecified,
	}

	// last_opened_date possibly is empty, wherefore we sort by last_modified_date as secondary criterion
	if primarySort.RelationKey == bundle.RelationKeyLastOpenedDate.String() {
		secondarySort := &model.BlockContentDataviewSort{
			RelationKey:    bundle.RelationKeyLastModifiedDate.String(),
			Type:           s.getSortDirection(sort.Direction),
			Format:         model.RelationFormat_date,
			IncludeTime:    true,
			EmptyPlacement: model.BlockContentDataviewSort_NotSpecified,
		}
		return []*model.BlockContentDataviewSort{primarySort, secondarySort}
	}

	return []*model.BlockContentDataviewSort{primarySort}
}

// getSortRelationKey returns the relation key for the given sort timestamp
func (s *SearchService) getSortRelationKey(timestamp string) string {
	switch timestamp {
	case "created_date":
		return bundle.RelationKeyCreatedDate.String()
	case "last_modified_date":
		return bundle.RelationKeyLastModifiedDate.String()
	case "last_opened_date":
		return bundle.RelationKeyLastOpenedDate.String()
	default:
		return bundle.RelationKeyLastModifiedDate.String()
	}
}

// getSortDirection returns the sort direction for the given string
func (s *SearchService) getSortDirection(direction string) model.BlockContentDataviewSortType {
	switch direction {
	case "asc":
		return model.BlockContentDataviewSort_Asc
	case "desc":
		return model.BlockContentDataviewSort_Desc
	default:
		return model.BlockContentDataviewSort_Desc
	}
}
