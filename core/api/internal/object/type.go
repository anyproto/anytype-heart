package object

import (
	"context"
	"errors"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ErrFailedRetrieveTypes        = errors.New("failed to retrieve types")
	ErrTypeNotFound               = errors.New("type not found")
	ErrTypeDeleted                = errors.New("type deleted")
	ErrFailedRetrieveType         = errors.New("failed to retrieve type")
	ErrFailedRetrieveTemplateType = errors.New("failed to retrieve template type")
	ErrTemplateTypeNotFound       = errors.New("template type not found")
	ErrFailedRetrieveTemplate     = errors.New("failed to retrieve template")
	ErrFailedRetrieveTemplates    = errors.New("failed to retrieve templates")
	ErrTemplateNotFound           = errors.New("template not found")
	ErrTemplateDeleted            = errors.New("template deleted")
)

// ListTypes returns a paginated list of types in a specific space.
func (s *service) ListTypes(ctx context.Context, spaceId string, offset int, limit int) (types []Type, total int, hasMore bool, err error) {
	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_objectType)),
			},
			{
				RelationKey: bundle.RelationKeyIsHidden.String(),
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.Bool(true),
			},
		},
		Sorts: []*model.BlockContentDataviewSort{
			{
				RelationKey: bundle.RelationKeyName.String(),
				Type:        model.BlockContentDataviewSort_Asc,
			},
		},
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyUniqueKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyIconEmoji.String(),
			bundle.RelationKeyIconName.String(),
			bundle.RelationKeyIconOption.String(),
			bundle.RelationKeyRecommendedLayout.String(),
			bundle.RelationKeyIsArchived.String(),
			bundle.RelationKeyRecommendedFeaturedRelations.String(),
			bundle.RelationKeyRecommendedRelations.String()},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedRetrieveTypes
	}

	total = len(resp.Records)
	paginatedTypes, hasMore := pagination.Paginate(resp.Records, offset, limit)
	types = make([]Type, 0, len(paginatedTypes))

	propertyMap, err := s.GetPropertyMapFromStore(spaceId)
	if err != nil {
		return nil, 0, false, err
	}

	for _, record := range paginatedTypes {
		types = append(types, Type{
			Object:     "type",
			Id:         record.Fields[bundle.RelationKeyId.String()].GetStringValue(),
			Key:        record.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(),
			Name:       record.Fields[bundle.RelationKeyName.String()].GetStringValue(),
			Icon:       util.GetIcon(s.gatewayUrl, record.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", record.Fields[bundle.RelationKeyIconName.String()].GetStringValue(), record.Fields[bundle.RelationKeyIconOption.String()].GetNumberValue()),
			Archived:   record.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
			Layout:     model.ObjectTypeLayout_name[int32(record.Fields[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue())],
			Properties: s.getRecommendedPropertiesFromLists(record.Fields[bundle.RelationKeyRecommendedFeaturedRelations.String()].GetListValue(), record.Fields[bundle.RelationKeyRecommendedRelations.String()].GetListValue(), propertyMap),
		})
	}
	return types, total, hasMore, nil
}

// GetType returns a single type by its ID in a specific space.
func (s *service) GetType(ctx context.Context, spaceId string, typeId string) (Type, error) {
	resp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: typeId,
	})

	if resp.Error != nil {
		if resp.Error.Code == pb.RpcObjectShowResponseError_NOT_FOUND {
			return Type{}, ErrTypeNotFound
		}

		if resp.Error.Code == pb.RpcObjectShowResponseError_OBJECT_DELETED {
			return Type{}, ErrTypeDeleted
		}

		if resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			return Type{}, ErrFailedRetrieveType
		}
	}

	// pre-fetch properties to fill the type
	propertyMap, err := s.GetPropertyMapFromStore(spaceId)
	if err != nil {
		return Type{}, err
	}

	details := resp.ObjectView.Details[0].Details.Fields
	return Type{
		Object:     "type",
		Id:         typeId,
		Key:        details[bundle.RelationKeyUniqueKey.String()].GetStringValue(),
		Name:       details[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:       util.GetIcon(s.gatewayUrl, details[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", details[bundle.RelationKeyIconName.String()].GetStringValue(), details[bundle.RelationKeyIconOption.String()].GetNumberValue()),
		Archived:   details[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
		Layout:     model.ObjectTypeLayout_name[int32(details[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue())],
		Properties: s.getRecommendedPropertiesFromLists(details[bundle.RelationKeyRecommendedFeaturedRelations.String()].GetListValue(), details[bundle.RelationKeyRecommendedRelations.String()].GetListValue(), propertyMap),
	}, nil
}

// ListTemplates returns a paginated list of templates in a specific space.
func (s *service) ListTemplates(ctx context.Context, spaceId string, typeId string, offset int, limit int) (templates []Template, total int, hasMore bool, err error) {
	// First, determine the type ID of "ot-template" in the space
	templateTypeIdResp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyUniqueKey.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String("ot-template"),
			},
		},
		Keys: []string{bundle.RelationKeyId.String()},
	})

	if templateTypeIdResp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedRetrieveTemplateType
	}

	if len(templateTypeIdResp.Records) == 0 {
		return nil, 0, false, ErrTemplateTypeNotFound
	}

	// Then, search all objects of the template type and filter by the target object type
	templateTypeId := templateTypeIdResp.Records[0].Fields[bundle.RelationKeyId.String()].GetStringValue()
	templateObjectsResp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(templateTypeId),
			},
			{
				RelationKey: bundle.RelationKeyTargetObjectType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(typeId),
			},
		},
		Keys: []string{bundle.RelationKeyId.String(), bundle.RelationKeyTargetObjectType.String(), bundle.RelationKeyName.String(), bundle.RelationKeyIconEmoji.String(), bundle.RelationKeyIsArchived.String()},
	})

	if templateObjectsResp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedRetrieveTemplates
	}

	total = len(templateObjectsResp.Records)
	paginatedTemplates, hasMore := pagination.Paginate(templateObjectsResp.Records, offset, limit)
	templates = make([]Template, 0, len(paginatedTemplates))

	// Finally, open each template and populate the response
	for _, record := range paginatedTemplates {
		templates = append(templates, Template{
			Object:   "template",
			Id:       record.Fields[bundle.RelationKeyId.String()].GetStringValue(),
			Name:     record.Fields[bundle.RelationKeyName.String()].GetStringValue(),
			Icon:     util.GetIcon(s.gatewayUrl, record.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", "", 0),
			Archived: record.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
		})
	}

	return templates, total, hasMore, nil
}

// GetTemplate returns a single template by its ID in a specific space.
func (s *service) GetTemplate(ctx context.Context, spaceId string, _ string, templateId string) (Template, error) {
	resp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: templateId,
	})

	if resp.Error != nil {
		if resp.Error.Code == pb.RpcObjectShowResponseError_NOT_FOUND {
			return Template{}, ErrTemplateNotFound
		}

		if resp.Error.Code == pb.RpcObjectShowResponseError_OBJECT_DELETED {
			return Template{}, ErrTemplateDeleted
		}

		if resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			return Template{}, ErrFailedRetrieveTemplate
		}
	}

	return Template{
		Object:   "template",
		Id:       templateId,
		Name:     resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:     util.GetIcon(s.gatewayUrl, resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", "", 0),
		Archived: resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
	}, nil
}

// GetTypeMapsFromStore retrieves all types from all spaces.
func (s *service) GetTypeMapsFromStore(spaceIds []string, propertyMap map[string]map[string]Property) (map[string]map[string]Type, error) {
	spacesToTypes := make(map[string]map[string]Type, len(spaceIds))

	for _, spaceId := range spaceIds {
		typeMap, err := s.GetTypeMapFromStore(spaceId, propertyMap[spaceId])
		if err != nil {
			return nil, err
		}
		spacesToTypes[spaceId] = typeMap
	}

	return spacesToTypes, nil
}

// GetTypeMapFromStore retrieves all types for a specific space.
func (s *service) GetTypeMapFromStore(spaceId string, propertyMap map[string]Property) (map[string]Type, error) {
	resp := s.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_objectType)),
			},
			{
				// resolve deleted types as well
				RelationKey: bundle.RelationKeyIsDeleted.String(),
			},
		},
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyUniqueKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyIconEmoji.String(),
			bundle.RelationKeyIconName.String(),
			bundle.RelationKeyIconOption.String(),
			bundle.RelationKeyRecommendedLayout.String(),
			bundle.RelationKeyIsArchived.String(),
			bundle.RelationKeyRecommendedFeaturedRelations.String(),
			bundle.RelationKeyRecommendedRelations.String(),
		},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, ErrFailedRetrieveTypes
	}

	typeMap := make(map[string]Type, len(resp.Records))
	for _, record := range resp.Records {
		typeMap[record.Fields[bundle.RelationKeyId.String()].GetStringValue()] = Type{
			Object:     "type",
			Id:         record.Fields[bundle.RelationKeyId.String()].GetStringValue(),
			Key:        record.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(),
			Name:       record.Fields[bundle.RelationKeyName.String()].GetStringValue(),
			Icon:       util.GetIcon(s.gatewayUrl, record.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", record.Fields[bundle.RelationKeyIconName.String()].GetStringValue(), record.Fields[bundle.RelationKeyIconOption.String()].GetNumberValue()),
			Archived:   record.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
			Layout:     model.ObjectTypeLayout_name[int32(record.Fields[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue())],
			Properties: s.getRecommendedPropertiesFromLists(record.Fields[bundle.RelationKeyRecommendedFeaturedRelations.String()].GetListValue(), record.Fields[bundle.RelationKeyRecommendedRelations.String()].GetListValue(), propertyMap),
		}
	}
	return typeMap, nil
}

// GetTypeFromDetails retrieves the type from the details.
func (s *service) GetTypeFromDetails(details []*model.ObjectViewDetailsSet, typeId string, propertyMap map[string]Property) Type {
	var objectTypeDetail *types.Struct
	for _, detail := range details {
		if detail.Id == typeId {
			objectTypeDetail = detail.GetDetails()
			break
		}
	}

	if objectTypeDetail == nil {
		return Type{}
	}

	return Type{
		Object:     "type",
		Id:         typeId,
		Key:        objectTypeDetail.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(),
		Name:       objectTypeDetail.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:       util.GetIcon(s.gatewayUrl, objectTypeDetail.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", objectTypeDetail.Fields[bundle.RelationKeyIconName.String()].GetStringValue(), objectTypeDetail.Fields[bundle.RelationKeyIconOption.String()].GetNumberValue()),
		Layout:     model.ObjectTypeLayout_name[int32(objectTypeDetail.Fields[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue())],
		Properties: s.getRecommendedPropertiesFromLists(objectTypeDetail.Fields[bundle.RelationKeyRecommendedFeaturedRelations.String()].GetListValue(), objectTypeDetail.Fields[bundle.RelationKeyRecommendedRelations.String()].GetListValue(), propertyMap),
	}
}

// getTypeFromStruct retrieves the type from the details.
func (s *service) getTypeFromStruct(details *types.Struct, typeMap map[string]Type) Type {
	return typeMap[details.Fields[bundle.RelationKeyType.String()].GetStringValue()]
}
