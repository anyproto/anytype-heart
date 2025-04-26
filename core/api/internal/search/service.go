package search

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/api/apicore"
	"github.com/anyproto/anytype-heart/core/api/internal/object"
	"github.com/anyproto/anytype-heart/core/api/internal/space"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ErrFailedSearchObjects = errors.New("failed to retrieve objects from space")
)

type Service interface {
	GlobalSearch(ctx context.Context, request SearchRequest, offset int, limit int) (objects []object.Object, total int, hasMore bool, err error)
	Search(ctx context.Context, spaceId string, request SearchRequest, offset int, limit int) (objects []object.Object, total int, hasMore bool, err error)
}

type service struct {
	mw            apicore.ClientCommands
	spaceService  space.Service
	objectService object.Service
}

func NewService(mw apicore.ClientCommands, spaceService space.Service, objectService object.Service) Service {
	return &service{mw: mw, spaceService: spaceService, objectService: objectService}
}

// GlobalSearch retrieves a paginated list of objects from all spaces that match the search parameters.
func (s *service) GlobalSearch(ctx context.Context, request SearchRequest, offset int, limit int) (objects []object.Object, total int, hasMore bool, err error) {
	spaceIds, err := s.spaceService.GetAllSpaceIds()
	if err != nil {
		return nil, 0, false, err
	}

	baseFilters := s.prepareBaseFilters()
	queryFilters := s.prepareQueryFilter(request.Query)
	sorts := s.prepareSorts(request.Sort)
	if len(sorts) == 0 {
		return nil, 0, false, errors.New("no sort criteria provided")
	}
	criterionToSortAfter := sorts[0].RelationKey

	var combinedRecords []*types.Struct
	for _, spaceId := range spaceIds {
		// Resolve template type and object type IDs per spaceId, as they are unique per spaceId
		templateFilter := s.prepareTemplateFilter()
		typeFilters := s.prepareObjectTypeFilters(spaceId, request.Types)
		filters := s.combineFilters(model.BlockContentDataviewFilter_And, baseFilters, templateFilter, queryFilters, typeFilters)

		objResp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
			SpaceId: spaceId,
			Filters: filters,
			Sorts:   sorts,
			Limit:   int32(offset + limit), // nolint: gosec
		})

		if objResp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
			return nil, 0, false, ErrFailedSearchObjects
		}

		for _, record := range objResp.Records {
			combinedRecords = append(combinedRecords, record)
		}
	}

	// Directly sort the raw records by extracting the sort field in the comparator.
	sort.SliceStable(combinedRecords, func(i, j int) bool {
		if criterionToSortAfter == bundle.RelationKeyName.String() {
			nameI := combinedRecords[i].Fields[bundle.RelationKeyName.String()].GetStringValue()
			nameJ := combinedRecords[j].Fields[bundle.RelationKeyName.String()].GetStringValue()
			if sorts[0].Type == model.BlockContentDataviewSort_Asc {
				return nameI < nameJ
			}
			return nameI > nameJ
		} else {
			numI := combinedRecords[i].Fields[criterionToSortAfter].GetNumberValue()
			numJ := combinedRecords[j].Fields[criterionToSortAfter].GetNumberValue()
			if sorts[0].Type == model.BlockContentDataviewSort_Asc {
				return numI < numJ
			}
			return numI > numJ
		}
	})

	total = len(combinedRecords)
	paginatedRecords, hasMore := pagination.Paginate(combinedRecords, offset, limit)

	// pre-fetch properties, types and tags to fill the objects
	propertyMaps, err := s.objectService.GetPropertyMapsFromStore(spaceIds)
	if err != nil {
		return nil, 0, false, err
	}
	typeMaps, err := s.objectService.GetTypeMapsFromStore(spaceIds, propertyMaps)
	if err != nil {
		return nil, 0, false, err
	}
	tagMap, err := s.objectService.GetTagMapsFromStore(spaceIds)
	if err != nil {
		return nil, 0, false, err
	}

	results := make([]object.Object, 0, len(paginatedRecords))
	for _, record := range paginatedRecords {
		results = append(results, s.objectService.GetObjectFromStruct(record, propertyMaps[record.Fields[bundle.RelationKeySpaceId.String()].GetStringValue()], typeMaps[record.Fields[bundle.RelationKeySpaceId.String()].GetStringValue()], tagMap[record.Fields[bundle.RelationKeySpaceId.String()].GetStringValue()]))
	}

	return results, total, hasMore, nil
}

// Search retrieves a paginated list of objects from a specific space that match the search parameters.
func (s *service) Search(ctx context.Context, spaceId string, request SearchRequest, offset int, limit int) (objects []object.Object, total int, hasMore bool, err error) {
	baseFilters := s.prepareBaseFilters()
	templateFilter := s.prepareTemplateFilter()
	queryFilters := s.prepareQueryFilter(request.Query)
	typeFilters := s.prepareObjectTypeFilters(spaceId, request.Types)
	filters := s.combineFilters(model.BlockContentDataviewFilter_And, baseFilters, templateFilter, queryFilters, typeFilters)

	sorts := s.prepareSorts(request.Sort)
	if len(sorts) == 0 {
		return nil, 0, false, errors.New("no sort criteria provided")
	}

	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: filters,
		Sorts:   sorts,
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedSearchObjects
	}

	total = len(resp.Records)
	paginatedRecords, hasMore := pagination.Paginate(resp.Records, offset, limit)

	// pre-fetch properties and types to fill the objects
	propertyMap, err := s.objectService.GetPropertyMapFromStore(spaceId)
	if err != nil {
		return nil, 0, false, err
	}
	typeMap, err := s.objectService.GetTypeMapFromStore(spaceId, propertyMap)
	if err != nil {
		return nil, 0, false, err
	}
	tagMap, err := s.objectService.GetTagMapFromStore(spaceId)
	if err != nil {
		return nil, 0, false, err
	}

	results := make([]object.Object, 0, len(paginatedRecords))
	for _, record := range paginatedRecords {
		results = append(results, s.objectService.GetObjectFromStruct(record, propertyMap, typeMap, tagMap))
	}

	return results, total, hasMore, nil
}

// makeAndCondition combines multiple filter groups with the given operator.
func (s *service) combineFilters(operator model.BlockContentDataviewFilterOperator, filterGroups ...[]*model.BlockContentDataviewFilter) []*model.BlockContentDataviewFilter {
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
func (s *service) prepareBaseFilters() []*model.BlockContentDataviewFilter {
	return []*model.BlockContentDataviewFilter{
		{
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
			RelationKey: bundle.RelationKeyIsHidden.String(),
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       pbtypes.Bool(true),
		},
	}
}

// prepareTemplateFilter returns a filter that excludes templates from the search results.
func (s *service) prepareTemplateFilter() []*model.BlockContentDataviewFilter {
	return []*model.BlockContentDataviewFilter{
		{
			RelationKey: "type.uniqueKey",
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       pbtypes.String("ot-template"),
		},
	}
}

// prepareQueryFilter combines object name and snippet filters with an OR condition.
func (s *service) prepareQueryFilter(searchQuery string) []*model.BlockContentDataviewFilter {
	if searchQuery == "" {
		return nil
	}

	return []*model.BlockContentDataviewFilter{
		{
			Operator: model.BlockContentDataviewFilter_Or,
			NestedFilters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyName.String(),
					Condition:   model.BlockContentDataviewFilter_Like,
					Value:       pbtypes.String(searchQuery),
				},
				{
					RelationKey: bundle.RelationKeySnippet.String(),
					Condition:   model.BlockContentDataviewFilter_Like,
					Value:       pbtypes.String(searchQuery),
				},
			},
		},
	}
}

// prepareObjectTypeFilters combines object type filters with an OR condition.
func (s *service) prepareObjectTypeFilters(spaceId string, objectTypes []string) []*model.BlockContentDataviewFilter {
	if len(objectTypes) == 0 || objectTypes[0] == "" {
		return nil
	}

	// Prepare nested filters for each object type
	nestedFilters := make([]*model.BlockContentDataviewFilter, 0, len(objectTypes))
	for _, objectType := range objectTypes {
		typeId := objectType

		if strings.HasPrefix(objectType, "ot-") { // TODO: replace with constant
			var err error
			typeId, err = util.ResolveUniqueKeyToTypeId(s.mw, spaceId, objectType)
			if err != nil {
				continue
			}
		}

		nestedFilters = append(nestedFilters, &model.BlockContentDataviewFilter{
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
func (s *service) prepareSorts(sort SortOptions) []*model.BlockContentDataviewSort {
	primarySort := &model.BlockContentDataviewSort{
		RelationKey:    s.getSortRelationKey(sort.Property),
		Type:           s.getSortDirection(sort.Direction),
		Format:         s.getSortFormat(sort.Property),
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
func (s *service) getSortRelationKey(timestamp SortProperty) string {
	switch timestamp {
	case CreatedDate:
		return bundle.RelationKeyCreatedDate.String()
	case LastModifiedDate:
		return bundle.RelationKeyLastModifiedDate.String()
	case LastOpenedDate:
		return bundle.RelationKeyLastOpenedDate.String()
	case Name:
		return bundle.RelationKeyName.String()
	default:
		return bundle.RelationKeyLastModifiedDate.String()
	}
}

// getSortDirection returns the sort direction for the given string
func (s *service) getSortDirection(direction SortDirection) model.BlockContentDataviewSortType {
	switch direction {
	case Asc:
		return model.BlockContentDataviewSort_Asc
	case Desc:
		return model.BlockContentDataviewSort_Desc
	default:
		return model.BlockContentDataviewSort_Desc
	}
}

// getSortFormat returns the sort format for the given timestamp
func (s *service) getSortFormat(timestamp SortProperty) model.RelationFormat {
	switch timestamp {
	case CreatedDate, LastModifiedDate, LastOpenedDate:
		return model.RelationFormat_date
	case Name:
		return model.RelationFormat_longtext
	default:
		return model.RelationFormat_date
	}
}
