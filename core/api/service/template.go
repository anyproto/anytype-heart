package service

import (
	"context"
	"errors"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ErrFailedRetrieveTemplate  = errors.New("failed to retrieve template")
	ErrFailedRetrieveTemplates = errors.New("failed to retrieve templates")
	ErrTemplateNotFound        = errors.New("template not found")
	ErrTemplateDeleted         = errors.New("template deleted")
)

// ListTemplates returns a paginated list of templates in a specific space.
func (s *Service) ListTemplates(ctx context.Context, spaceId string, typeId string, offset int, limit int) (templates []apimodel.Object, total int, hasMore bool, err error) {
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

	// Then, search all objects of the template type and filter by the target type
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
	})

	if templateObjectsResp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedRetrieveTemplates
	}

	total = len(templateObjectsResp.Records)
	paginatedTemplates, hasMore := pagination.Paginate(templateObjectsResp.Records, offset, limit)
	templates = make([]apimodel.Object, 0, len(paginatedTemplates))

	propertyMap, err := s.getPropertyMapFromStore(ctx, spaceId, true)
	if err != nil {
		return nil, 0, false, err
	}
	typeMap, err := s.getTypeMapFromStore(ctx, spaceId, propertyMap, false)
	if err != nil {
		return nil, 0, false, err
	}
	tagMap, err := s.getTagMapFromStore(ctx, spaceId)
	if err != nil {
		return nil, 0, false, err
	}

	for _, record := range paginatedTemplates {
		templates = append(templates, s.getObjectFromStruct(record, propertyMap, typeMap, tagMap))
	}

	return templates, total, hasMore, nil
}

// GetTemplate returns a single template by its ID in a specific space.
func (s *Service) GetTemplate(ctx context.Context, spaceId string, _ string, templateId string) (apimodel.ObjectWithBody, error) {
	resp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: templateId,
	})

	if resp.Error != nil {
		if resp.Error.Code == pb.RpcObjectShowResponseError_NOT_FOUND {
			return apimodel.ObjectWithBody{}, ErrTemplateNotFound
		}

		if resp.Error.Code == pb.RpcObjectShowResponseError_OBJECT_DELETED {
			return apimodel.ObjectWithBody{}, ErrTemplateDeleted
		}

		if resp.Error != nil && resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			return apimodel.ObjectWithBody{}, ErrFailedRetrieveTemplate
		}
	}

	propertyMap, err := s.getPropertyMapFromStore(ctx, spaceId, true)
	if err != nil {
		return apimodel.ObjectWithBody{}, err
	}
	typeMap, err := s.getTypeMapFromStore(ctx, spaceId, propertyMap, false)
	if err != nil {
		return apimodel.ObjectWithBody{}, err
	}
	tagMap, err := s.getTagMapFromStore(ctx, spaceId)
	if err != nil {
		return apimodel.ObjectWithBody{}, err
	}

	markdown, err := s.getMarkdownExport(ctx, spaceId, templateId, model.ObjectTypeLayout(resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyResolvedLayout.String()].GetNumberValue()))
	if err != nil {
		return apimodel.ObjectWithBody{}, err
	}

	return s.getObjectWithBlocksFromStruct(resp.ObjectView.Details[0].Details, markdown, propertyMap, typeMap, tagMap), nil
}
