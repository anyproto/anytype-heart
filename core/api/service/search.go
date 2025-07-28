package service

import (
	"context"
	"errors"
	"sort"

	"github.com/gogo/protobuf/types"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
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

// GlobalSearch retrieves a paginated list of objects from all spaces that match the search parameters.
func (s *Service) GlobalSearch(ctx context.Context, request apimodel.SearchRequest, offset int, limit int) (objects []apimodel.Object, total int, hasMore bool, err error) {
	spaceIds, err := s.GetAllSpaceIds(ctx)
	if err != nil {
		return nil, 0, false, err
	}

	baseFilters := s.prepareBaseFilters()
	queryFilters := s.prepareQueryFilter(request.Query)
	sorts, criterionToSortAfter := s.prepareSorts(request.Sort)

	var combinedRecords []*types.Struct
	for _, spaceId := range spaceIds {
		// Resolve template and type IDs per spaceId, as they are unique per spaceId
		templateFilter := s.prepareTemplateFilter()
		typeFilters := s.prepareTypeFilters(request.Types, spaceId)
		if len(request.Types) > 0 && len(typeFilters) == 0 {
			// Skip spaces that don’t have any of the requested types
			continue
		}
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

	results := make([]apimodel.Object, 0, len(paginatedRecords))
	for _, record := range paginatedRecords {
		results = append(results, s.getObjectFromStruct(record))
	}

	return results, total, hasMore, nil
}

// Search retrieves a paginated list of objects from a specific space that match the search parameters.
func (s *Service) Search(ctx context.Context, spaceId string, request apimodel.SearchRequest, offset int, limit int) (objects []apimodel.Object, total int, hasMore bool, err error) {
	baseFilters := s.prepareBaseFilters()
	templateFilter := s.prepareTemplateFilter()
	queryFilters := s.prepareQueryFilter(request.Query)

	typeFilters := s.prepareTypeFilters(request.Types, spaceId)
	if len(request.Types) > 0 && len(typeFilters) == 0 {
		// No matching types in this space; return empty result
		return nil, 0, false, nil
	}
	filters := s.combineFilters(model.BlockContentDataviewFilter_And, baseFilters, templateFilter, queryFilters, typeFilters)
	sorts, _ := s.prepareSorts(request.Sort)

	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: filters,
		Sorts:   sorts,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedSearchObjects
	}

	total = len(resp.Records)
	paginatedRecords, hasMore := pagination.Paginate(resp.Records, offset, limit)

	results := make([]apimodel.Object, 0, len(paginatedRecords))
	for _, record := range paginatedRecords {
		results = append(results, s.getObjectFromStruct(record))
	}

	return results, total, hasMore, nil
}

// makeAndCondition combines multiple filter groups with the given operator.
func (s *Service) combineFilters(operator model.BlockContentDataviewFilterOperator, filterGroups ...[]*model.BlockContentDataviewFilter) []*model.BlockContentDataviewFilter {
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
func (s *Service) prepareBaseFilters() []*model.BlockContentDataviewFilter {
	return []*model.BlockContentDataviewFilter{
		{
			RelationKey: bundle.RelationKeyResolvedLayout.String(),
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       pbtypes.IntList(util.LayoutsToIntArgs(util.ObjectLayouts)...),
		},
		{
			RelationKey: bundle.RelationKeyIsHidden.String(),
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       pbtypes.Bool(true),
		},
	}
}

// prepareTemplateFilter returns a filter that excludes templates from the search results.
func (s *Service) prepareTemplateFilter() []*model.BlockContentDataviewFilter {
	return []*model.BlockContentDataviewFilter{
		{
			RelationKey: "type.uniqueKey",
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       pbtypes.String("ot-template"),
		},
	}
}

// prepareQueryFilter combines object name and snippet filters with an OR condition.
func (s *Service) prepareQueryFilter(searchQuery string) []*model.BlockContentDataviewFilter {
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

// prepareTypeFilters combines type filters with an OR condition.
func (s *Service) prepareTypeFilters(types []string, spaceId string) []*model.BlockContentDataviewFilter {
	if len(types) == 0 {
		return nil
	}

	s.typeMapMu.RLock()
	typeMap := s.typeMapCache[spaceId]
	s.typeMapMu.RUnlock()

	if typeMap == nil {
		log.Errorf("prepareTypeFilters: typeMap is nil for spaceId %s", spaceId)
		return nil
	}

	// Prepare nested filters for each type
	nestedFilters := make([]*model.BlockContentDataviewFilter, 0, len(types))
	for _, key := range types {
		if key == "" {
			continue
		}

		uk := s.ResolveTypeApiKey(typeMap, key)
		typeDef, ok := typeMap[uk]
		if !ok {
			continue
		}

		nestedFilters = append(nestedFilters, &model.BlockContentDataviewFilter{
			RelationKey: bundle.RelationKeyType.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.String(typeDef.Id),
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
func (s *Service) prepareSorts(sort apimodel.SortOptions) (sorts []*model.BlockContentDataviewSort, criterionToSortAfter string) {
	primarySort := &model.BlockContentDataviewSort{
		RelationKey:    s.getSortRelationKey(sort.PropertyKey),
		Type:           s.getSortDirection(sort.Direction),
		Format:         s.getSortFormat(sort.PropertyKey),
		IncludeTime:    true,
		EmptyPlacement: model.BlockContentDataviewSort_NotSpecified,
	}

	// last_opened_date possibly is empty, wherefore we sort by last_modified_date as a secondary criterion
	if primarySort.RelationKey == bundle.RelationKeyLastOpenedDate.String() {
		secondarySort := &model.BlockContentDataviewSort{
			RelationKey:    bundle.RelationKeyLastModifiedDate.String(),
			Type:           s.getSortDirection(sort.Direction),
			Format:         model.RelationFormat_date,
			IncludeTime:    true,
			EmptyPlacement: model.BlockContentDataviewSort_NotSpecified,
		}
		return []*model.BlockContentDataviewSort{primarySort, secondarySort}, primarySort.RelationKey
	}

	return []*model.BlockContentDataviewSort{primarySort}, primarySort.RelationKey
}

// getSortRelationKey returns the relation key for the given sort timestamp
func (s *Service) getSortRelationKey(timestamp apimodel.SortProperty) string {
	switch timestamp {
	case apimodel.CreatedDate:
		return bundle.RelationKeyCreatedDate.String()
	case apimodel.LastModifiedDate:
		return bundle.RelationKeyLastModifiedDate.String()
	case apimodel.LastOpenedDate:
		return bundle.RelationKeyLastOpenedDate.String()
	case apimodel.Name:
		return bundle.RelationKeyName.String()
	default:
		return bundle.RelationKeyLastModifiedDate.String()
	}
}

// getSortDirection returns the sort direction for the given string
func (s *Service) getSortDirection(direction apimodel.SortDirection) model.BlockContentDataviewSortType {
	switch direction {
	case apimodel.Asc:
		return model.BlockContentDataviewSort_Asc
	case apimodel.Desc:
		return model.BlockContentDataviewSort_Desc
	default:
		return model.BlockContentDataviewSort_Desc
	}
}

// getSortFormat returns the sort format for the given timestamp
func (s *Service) getSortFormat(timestamp apimodel.SortProperty) model.RelationFormat {
	switch timestamp {
	case apimodel.CreatedDate, apimodel.LastModifiedDate, apimodel.LastOpenedDate:
		return model.RelationFormat_date
	case apimodel.Name:
		return model.RelationFormat_longtext
	default:
		return model.RelationFormat_date
	}
}
