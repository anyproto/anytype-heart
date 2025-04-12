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
	spaceLimit             = 64
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
	spaces, _, _, err := s.spaceService.ListSpaces(ctx, 0, spaceLimit)
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

	type sortRecord struct {
		Id          string
		SpaceId     string
		numericSort float64
		stringSort  string
		rawRecord   *types.Struct
	}
	combinedRecords := make([]sortRecord, 0)

	allResponses := make([]*pb.RpcObjectSearchResponse, 0, len(spaces))
	for _, space := range spaces {
		// Resolve template type and object type IDs per space, as they are unique per space
		templateFilter := s.prepareTemplateFilter()
		typeFilters := s.prepareObjectTypeFilters(space.Id, request.Types)
		filters := s.combineFilters(model.BlockContentDataviewFilter_And, baseFilters, templateFilter, queryFilters, typeFilters)

		objResp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
			SpaceId: space.Id,
			Filters: filters,
			Sorts:   sorts,
			Limit:   int32(offset + limit), // nolint: gosec
		})

		if objResp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
			return nil, 0, false, ErrFailedSearchObjects
		}

		allResponses = append(allResponses, objResp)
	}

	for _, objResp := range allResponses {
		for _, record := range objResp.Records {
			sr := sortRecord{
				Id:        record.Fields[bundle.RelationKeyId.String()].GetStringValue(),
				SpaceId:   record.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(),
				rawRecord: record,
			}
			if criterionToSortAfter == bundle.RelationKeyName.String() {
				sr.stringSort = record.Fields[criterionToSortAfter].GetStringValue()
			} else {
				sr.numericSort = record.Fields[criterionToSortAfter].GetNumberValue()
			}
			combinedRecords = append(combinedRecords, sr)
		}
	}

	sortFunc := func(i, j int) bool {
		if criterionToSortAfter == bundle.RelationKeyName.String() {
			if sorts[0].Type == model.BlockContentDataviewSort_Asc {
				return combinedRecords[i].stringSort < combinedRecords[j].stringSort
			}
			return combinedRecords[i].stringSort > combinedRecords[j].stringSort
		} else {
			if sorts[0].Type == model.BlockContentDataviewSort_Asc {
				return combinedRecords[i].numericSort < combinedRecords[j].numericSort
			}
			return combinedRecords[i].numericSort > combinedRecords[j].numericSort
		}
	}
	sort.SliceStable(combinedRecords, sortFunc)

	total = len(combinedRecords)
	paginatedRecords, hasMore := pagination.Paginate(combinedRecords, offset, limit)

	// pre-fetch properties and types to fill the objects
	spaceIds := make([]string, 0, len(spaces))
	for _, space := range spaces {
		spaceIds = append(spaceIds, space.Id)
	}
	propertyFormatMap, err := s.objectService.GetPropertyFormatMapsFromStore(spaceIds)
	if err != nil {
		return nil, 0, false, err
	}
	typeMap, err := s.objectService.GetTypeMapsFromStore(spaceIds)
	if err != nil {
		return nil, 0, false, err
	}

	results := make([]object.Object, 0, len(paginatedRecords))
	for _, record := range paginatedRecords {
		results = append(results, s.objectService.GetObjectFromStruct(record.rawRecord, propertyFormatMap, typeMap))
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
	propertyFormatMap, err := s.objectService.GetPropertyFormatMapsFromStore([]string{spaceId})
	if err != nil {
		return nil, 0, false, err
	}
	typeMap, err := s.objectService.GetTypeMapsFromStore([]string{spaceId})
	if err != nil {
		return nil, 0, false, err
	}

	results := make([]object.Object, 0, len(paginatedRecords))
	for _, record := range paginatedRecords {
		results = append(results, s.objectService.GetObjectFromStruct(record, propertyFormatMap, typeMap))
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
func (s *service) prepareTemplateFilter() []*model.BlockContentDataviewFilter {
	return []*model.BlockContentDataviewFilter{
		{
			Operator:    model.BlockContentDataviewFilter_No,
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
func (s *service) prepareObjectTypeFilters(spaceId string, objectTypes []string) []*model.BlockContentDataviewFilter {
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
