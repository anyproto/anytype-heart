package object

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/iancoleman/strcase"

	"github.com/anyproto/anytype-heart/core/api/apicore"
	"github.com/anyproto/anytype-heart/core/api/internal/space"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	// objects
	ErrObjectNotFound                  = errors.New("object not found")
	ErrObjectDeleted                   = errors.New("object deleted")
	ErrFailedRetrieveObject            = errors.New("failed to retrieve object")
	ErrFailedRetrieveObjects           = errors.New("failed to retrieve list of objects")
	ErrFailedRetrievePropertyFormatMap = errors.New("failed to retrieve property format map")
	ErrFailedDeleteObject              = errors.New("failed to delete object")
	ErrFailedCreateObject              = errors.New("failed to create object")
	ErrInputMissingSource              = errors.New("source is missing for bookmark")
	ErrIconNameColorNotSupported       = errors.New("icon name and color are not supported for object")
	ErrFailedSetPropertyFeatured       = errors.New("failed to set property featured")
	ErrFailedCreateBookmark            = errors.New("failed to fetch bookmark")
	ErrFailedCreateBlock               = errors.New("failed to create block")
	ErrFailedPasteBody                 = errors.New("failed to paste body")

	// types
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

var excludedSystemProperties = map[string]bool{
	bundle.RelationKeyId.String():                     true,
	bundle.RelationKeySpaceId.String():                true,
	bundle.RelationKeyName.String():                   true,
	bundle.RelationKeyIconEmoji.String():              true,
	bundle.RelationKeyIconImage.String():              true,
	bundle.RelationKeyType.String():                   true,
	bundle.RelationKeyResolvedLayout.String():         true,
	bundle.RelationKeyIsFavorite.String():             true,
	bundle.RelationKeyIsArchived.String():             true,
	bundle.RelationKeyIsDeleted.String():              true,
	bundle.RelationKeyIsHidden.String():               true,
	bundle.RelationKeyWorkspaceId.String():            true,
	bundle.RelationKeyInternalFlags.String():          true,
	bundle.RelationKeyRestrictions.String():           true,
	bundle.RelationKeyOrigin.String():                 true,
	bundle.RelationKeySnippet.String():                true,
	bundle.RelationKeySyncStatus.String():             true,
	bundle.RelationKeySyncError.String():              true,
	bundle.RelationKeySyncDate.String():               true,
	bundle.RelationKeyCoverId.String():                true,
	bundle.RelationKeyCoverType.String():              true,
	bundle.RelationKeyCoverScale.String():             true,
	bundle.RelationKeyCoverX.String():                 true,
	bundle.RelationKeyCoverY.String():                 true,
	bundle.RelationKeyMentions.String():               true,
	bundle.RelationKeyOldAnytypeID.String():           true,
	bundle.RelationKeySource.String():                 true,
	bundle.RelationKeySourceFilePath.String():         true,
	bundle.RelationKeyImportType.String():             true,
	bundle.RelationKeyTargetObjectType.String():       true,
	bundle.RelationKeyFeaturedRelations.String():      true,
	bundle.RelationKeySetOf.String():                  true,
	bundle.RelationKeyLinks.String():                  true,
	bundle.RelationKeyBacklinks.String():              true,
	bundle.RelationKeySourceObject.String():           true,
	bundle.RelationKeyLayoutAlign.String():            true,
	bundle.RelationKeyIsHiddenDiscovery.String():      true,
	bundle.RelationKeyLayout.String():                 true,
	bundle.RelationKeyIsReadonly.String():             true,
	bundle.RelationKeyParticipantStatus.String():      true,
	bundle.RelationKeyParticipantPermissions.String(): true,
}

type Service interface {
	SetAccountInfo(accountInfo *model.AccountInfo)
	ListObjects(ctx context.Context, spaceId string, offset int, limit int) ([]Object, int, bool, error)
	GetObject(ctx context.Context, spaceId string, objectId string) (ObjectWithBlocks, error)
	DeleteObject(ctx context.Context, spaceId string, objectId string) (ObjectWithBlocks, error)
	CreateObject(ctx context.Context, spaceId string, request CreateObjectRequest) (ObjectWithBlocks, error)
	ListTypes(ctx context.Context, spaceId string, offset int, limit int) ([]Type, int, bool, error)
	GetType(ctx context.Context, spaceId string, typeId string) (Type, error)
	ListTemplates(ctx context.Context, spaceId string, typeId string, offset int, limit int) ([]Template, int, bool, error)
	GetTemplate(ctx context.Context, spaceId string, typeId string, templateId string) (Template, error)

	MapRelationFormat(format model.RelationFormat) string
	GetObjectFromStruct(details *types.Struct, propertyFormatMap map[string]map[string]string, typeMap map[string]map[string]Type) Object
	GetPropertyFormatMapsFromStore(spaceIds []string) (map[string]map[string]string, error)
	GetTypeMapsFromStore(spaceIds []string) (map[string]map[string]Type, error)
	GetTypeFromDetails(details []*model.ObjectViewDetailsSet, typeId string) Type
}

type service struct {
	mw           apicore.ClientCommands
	spaceService space.Service
	AccountInfo  *model.AccountInfo
}

func NewService(mw apicore.ClientCommands, spaceService space.Service) Service {
	return &service{mw: mw, spaceService: spaceService}
}

func (s *service) SetAccountInfo(accountInfo *model.AccountInfo) {
	s.AccountInfo = accountInfo
}

// ListObjects retrieves a paginated list of objects in a specific space.
func (s *service) ListObjects(ctx context.Context, spaceId string, offset int, limit int) (objects []Object, total int, hasMore bool, err error) {
	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
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
				RelationKey: "type.uniqueKey",
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.String("ot-template"),
			},
			{
				Operator:    model.BlockContentDataviewFilter_No,
				RelationKey: bundle.RelationKeyIsHidden.String(),
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.Bool(true),
			},
		},
		Sorts: []*model.BlockContentDataviewSort{{
			RelationKey:    bundle.RelationKeyLastModifiedDate.String(),
			Type:           model.BlockContentDataviewSort_Desc,
			Format:         model.RelationFormat_longtext,
			IncludeTime:    true,
			EmptyPlacement: model.BlockContentDataviewSort_NotSpecified,
		}},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedRetrieveObjects
	}

	total = len(resp.Records)
	paginatedObjects, hasMore := pagination.Paginate(resp.Records, offset, limit)
	objects = make([]Object, 0, len(paginatedObjects))

	// pre-fetch properties and types to fill the objects
	propertyFormatMap, err := s.GetPropertyFormatMapsFromStore([]string{spaceId})
	if err != nil {
		return nil, 0, false, err
	}
	typeMap, err := s.GetTypeMapsFromStore([]string{spaceId})
	if err != nil {
		return nil, 0, false, err
	}

	for _, record := range paginatedObjects {
		objects = append(objects, s.GetObjectFromStruct(record, propertyFormatMap, typeMap))
	}
	return objects, total, hasMore, nil
}

// GetObject retrieves a single object by its ID in a specific space.
func (s *service) GetObject(ctx context.Context, spaceId string, objectId string) (ObjectWithBlocks, error) {
	resp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: objectId,
	})

	if resp.Error != nil {
		if resp.Error.Code == pb.RpcObjectShowResponseError_NOT_FOUND {
			return ObjectWithBlocks{}, ErrObjectNotFound
		}

		if resp.Error.Code == pb.RpcObjectShowResponseError_OBJECT_DELETED {
			return ObjectWithBlocks{}, ErrObjectDeleted
		}

		if resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			return ObjectWithBlocks{}, ErrFailedRetrieveObject
		}
	}

	details := resp.ObjectView.Details[0].Details.Fields
	icon := util.GetIcon(s.AccountInfo.GatewayUrl, details[bundle.RelationKeyIconEmoji.String()].GetStringValue(), details[bundle.RelationKeyIconImage.String()].GetStringValue(), details[bundle.RelationKeyIconName.String()].GetStringValue(), details[bundle.RelationKeyIconOption.String()].GetNumberValue())

	object := ObjectWithBlocks{
		Object: Object{
			Object:     "object",
			Id:         details[bundle.RelationKeyId.String()].GetStringValue(),
			Name:       details[bundle.RelationKeyName.String()].GetStringValue(),
			Icon:       icon,
			Archived:   details[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
			SpaceId:    details[bundle.RelationKeySpaceId.String()].GetStringValue(),
			Snippet:    details[bundle.RelationKeySnippet.String()].GetStringValue(),
			Layout:     model.ObjectTypeLayout_name[int32(details[bundle.RelationKeyResolvedLayout.String()].GetNumberValue())],
			Type:       s.GetTypeFromDetails(resp.ObjectView.Details, details[bundle.RelationKeyType.String()].GetStringValue()),
			Properties: s.getPropertiesFromDetails(resp),
		},
		Blocks: s.getBlocksFromDetails(resp),
	}

	return object, nil
}

// DeleteObject deletes an existing object in a specific space.
func (s *service) DeleteObject(ctx context.Context, spaceId string, objectId string) (ObjectWithBlocks, error) {
	object, err := s.GetObject(ctx, spaceId, objectId)
	if err != nil {
		return ObjectWithBlocks{}, err
	}

	resp := s.mw.ObjectSetIsArchived(ctx, &pb.RpcObjectSetIsArchivedRequest{
		ContextId:  objectId,
		IsArchived: true,
	})

	if resp.Error.Code != pb.RpcObjectSetIsArchivedResponseError_NULL {
		return ObjectWithBlocks{}, ErrFailedDeleteObject
	}

	return object, nil
}

// CreateObject creates a new object in a specific space.
func (s *service) CreateObject(ctx context.Context, spaceId string, request CreateObjectRequest) (ObjectWithBlocks, error) {
	details, err := s.buildObjectDetails(request)
	if err != nil {
		return ObjectWithBlocks{}, err
	}

	var objectId string
	if request.TypeKey == "ot-bookmark" {
		resp := s.mw.ObjectCreateBookmark(ctx, &pb.RpcObjectCreateBookmarkRequest{
			Details:    details,
			SpaceId:    spaceId,
			TemplateId: request.TemplateId,
		})

		if resp.Error.Code != pb.RpcObjectCreateBookmarkResponseError_NULL {
			return ObjectWithBlocks{}, ErrFailedCreateBookmark
		}
		objectId = resp.ObjectId
	} else {
		resp := s.mw.ObjectCreate(ctx, &pb.RpcObjectCreateRequest{
			Details:             details,
			TemplateId:          request.TemplateId,
			SpaceId:             spaceId,
			ObjectTypeUniqueKey: request.TypeKey,
		})

		if resp.Error.Code != pb.RpcObjectCreateResponseError_NULL {
			return ObjectWithBlocks{}, ErrFailedCreateObject
		}
		objectId = resp.ObjectId
	}

	// ObjectRelationAddFeatured if description was set
	if request.Description != "" {
		relAddFeatResp := s.mw.ObjectRelationAddFeatured(ctx, &pb.RpcObjectRelationAddFeaturedRequest{
			ContextId: objectId,
			Relations: []string{bundle.RelationKeyDescription.String()},
		})

		if relAddFeatResp.Error.Code != pb.RpcObjectRelationAddFeaturedResponseError_NULL {
			object, _ := s.GetObject(ctx, spaceId, objectId) // nolint:errcheck
			return object, ErrFailedSetPropertyFeatured
		}
	}

	// First call BlockCreate at top, then BlockPaste to paste the body
	if request.Body != "" {
		blockCreateResp := s.mw.BlockCreate(ctx, &pb.RpcBlockCreateRequest{
			ContextId: objectId,
			TargetId:  "",
			Block: &model.Block{
				Id:              "",
				BackgroundColor: "",
				Align:           model.Block_AlignLeft,
				VerticalAlign:   model.Block_VerticalAlignTop,
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{
						Text:      "",
						Style:     model.BlockContentText_Paragraph,
						Checked:   false,
						Color:     "",
						IconEmoji: "",
						IconImage: "",
					},
				},
			},
			Position: model.Block_Bottom,
		})

		if blockCreateResp.Error.Code != pb.RpcBlockCreateResponseError_NULL {
			object, _ := s.GetObject(ctx, spaceId, objectId) // nolint:errcheck
			return object, ErrFailedCreateBlock
		}

		blockPasteResp := s.mw.BlockPaste(ctx, &pb.RpcBlockPasteRequest{
			ContextId:      objectId,
			FocusedBlockId: blockCreateResp.BlockId,
			TextSlot:       request.Body,
		})

		if blockPasteResp.Error.Code != pb.RpcBlockPasteResponseError_NULL {
			object, _ := s.GetObject(ctx, spaceId, objectId) // nolint:errcheck
			return object, ErrFailedPasteBody
		}
	}

	return s.GetObject(ctx, spaceId, objectId)
}

// buildObjectDetails extracts the details structure from the CreateObjectRequest.
func (s *service) buildObjectDetails(request CreateObjectRequest) (*types.Struct, error) {
	// Validate bookmark source
	if request.TypeKey == "ot-bookmark" && request.Source == "" {
		return nil, ErrInputMissingSource
	}

	// Validate icon: only allow either emoji or file, and disallow name and color fields.
	if request.Icon.Name != nil || request.Icon.Color != nil {
		return nil, ErrIconNameColorNotSupported
	}

	iconFields := map[string]*types.Value{}
	if request.Icon.Emoji != nil {
		iconFields[bundle.RelationKeyIconEmoji.String()] = pbtypes.String(*request.Icon.Emoji)
	} else if request.Icon.File != nil {
		iconFields[bundle.RelationKeyIconImage.String()] = pbtypes.String(*request.Icon.File)
	}

	fields := map[string]*types.Value{
		bundle.RelationKeyName.String():        pbtypes.String(request.Name),
		bundle.RelationKeyDescription.String(): pbtypes.String(request.Description),
		bundle.RelationKeySource.String():      pbtypes.String(request.Source),
		bundle.RelationKeyOrigin.String():      pbtypes.Int64(int64(model.ObjectOrigin_api)),
	}
	for k, v := range iconFields {
		fields[k] = v
	}

	return &types.Struct{Fields: fields}, nil
}

// ListTypes returns a paginated list of types in a specific space.
func (s *service) ListTypes(ctx context.Context, spaceId string, offset int, limit int) (types []Type, total int, hasMore bool, err error) {
	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				Operator:    model.BlockContentDataviewFilter_No,
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_objectType)),
			},
			{
				Operator:    model.BlockContentDataviewFilter_No,
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
		Keys: []string{bundle.RelationKeyId.String(), bundle.RelationKeyUniqueKey.String(), bundle.RelationKeyName.String(), bundle.RelationKeyIconEmoji.String(), bundle.RelationKeyIconName.String(), bundle.RelationKeyIconOption.String(), bundle.RelationKeyRecommendedLayout.String(), bundle.RelationKeyIsArchived.String()},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedRetrieveTypes
	}

	total = len(resp.Records)
	paginatedTypes, hasMore := pagination.Paginate(resp.Records, offset, limit)
	types = make([]Type, 0, len(paginatedTypes))

	for _, record := range paginatedTypes {
		types = append(types, Type{
			Object:            "type",
			Id:                record.Fields[bundle.RelationKeyId.String()].GetStringValue(),
			Key:               record.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(),
			Name:              record.Fields[bundle.RelationKeyName.String()].GetStringValue(),
			Icon:              util.GetIcon(s.AccountInfo.GatewayUrl, record.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", record.Fields[bundle.RelationKeyIconName.String()].GetStringValue(), record.Fields[bundle.RelationKeyIconOption.String()].GetNumberValue()),
			Archived:          record.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
			RecommendedLayout: model.ObjectTypeLayout_name[int32(record.Fields[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue())],
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

	details := resp.ObjectView.Details[0].Details.Fields
	return Type{
		Object:            "type",
		Id:                typeId,
		Key:               details[bundle.RelationKeyUniqueKey.String()].GetStringValue(),
		Name:              details[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:              util.GetIcon(s.AccountInfo.GatewayUrl, details[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", details[bundle.RelationKeyIconName.String()].GetStringValue(), details[bundle.RelationKeyIconOption.String()].GetNumberValue()),
		Archived:          details[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
		RecommendedLayout: model.ObjectTypeLayout_name[int32(details[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue())],
	}, nil
}

// ListTemplates returns a paginated list of templates in a specific space.
func (s *service) ListTemplates(ctx context.Context, spaceId string, typeId string, offset int, limit int) (templates []Template, total int, hasMore bool, err error) {
	// First, determine the type ID of "ot-template" in the space
	templateTypeIdResp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				Operator:    model.BlockContentDataviewFilter_No,
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
				Operator:    model.BlockContentDataviewFilter_No,
				RelationKey: bundle.RelationKeyType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(templateTypeId),
			},
		},
		Keys: []string{bundle.RelationKeyId.String(), bundle.RelationKeyTargetObjectType.String(), bundle.RelationKeyName.String(), bundle.RelationKeyIconEmoji.String(), bundle.RelationKeyIsArchived.String()},
	})

	if templateObjectsResp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedRetrieveTemplates
	}

	templateIds := make([]string, 0)
	for _, record := range templateObjectsResp.Records {
		if record.Fields[bundle.RelationKeyTargetObjectType.String()].GetStringValue() == typeId {
			templateIds = append(templateIds, record.Fields[bundle.RelationKeyId.String()].GetStringValue())
		}
	}

	total = len(templateIds)
	paginatedTemplates, hasMore := pagination.Paginate(templateIds, offset, limit)
	templates = make([]Template, 0, len(paginatedTemplates))

	// Finally, open each template and populate the response
	for _, templateId := range paginatedTemplates {
		templateResp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
			SpaceId:  spaceId,
			ObjectId: templateId,
		})

		if templateResp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			return nil, 0, false, ErrFailedRetrieveTemplate
		}

		templates = append(templates, Template{
			Object:   "template",
			Id:       templateId,
			Name:     templateResp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
			Icon:     util.GetIcon(s.AccountInfo.GatewayUrl, templateResp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", "", 0),
			Archived: templateResp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
		})
	}

	return templates, total, hasMore, nil
}

// GetTemplate returns a single template by its ID in a specific space.
func (s *service) GetTemplate(ctx context.Context, spaceId string, typeId string, templateId string) (Template, error) {
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
		Icon:     util.GetIcon(s.AccountInfo.GatewayUrl, resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", "", 0),
		Archived: resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
	}, nil
}

// GetTypeFromDetails retrieves the type from the details.
func (s *service) GetTypeFromDetails(details []*model.ObjectViewDetailsSet, typeId string) Type {
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
		Object:            "type",
		Id:                objectTypeDetail.Fields[bundle.RelationKeyId.String()].GetStringValue(),
		Key:               objectTypeDetail.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(),
		Name:              objectTypeDetail.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:              util.GetIcon(s.AccountInfo.GatewayUrl, objectTypeDetail.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", objectTypeDetail.Fields[bundle.RelationKeyIconName.String()].GetStringValue(), objectTypeDetail.Fields[bundle.RelationKeyIconOption.String()].GetNumberValue()),
		RecommendedLayout: model.ObjectTypeLayout_name[int32(objectTypeDetail.Fields[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue())],
	}
}

// getPropertiesFromDetails returns a list of properties by iterating over all properties found in the RelationLinks and mapping their format and value.
func (s *service) getPropertiesFromDetails(resp *pb.RpcObjectShowResponse) []Property {
	propertyFormatMap := s.getPropertyFormatMapFromLinks(resp.ObjectView.RelationLinks)
	linkedProperties := resp.ObjectView.RelationLinks
	primaryDetailFields := resp.ObjectView.Details[0].Details.Fields

	properties := make([]Property, 0, len(linkedProperties))
	for _, r := range linkedProperties {
		key := r.Key
		if _, isExcluded := excludedSystemProperties[key]; isExcluded {
			continue
		}
		if _, ok := primaryDetailFields[key]; !ok {
			continue
		}

		id, name := s.getPropertyIdAndName(key, resp.ObjectView.Details[0].Details)
		format := propertyFormatMap[key]
		convertedVal := s.convertPropertyValue(key, primaryDetailFields[key], format, resp.ObjectView.Details[0].Details)

		if s.isMissingObject(convertedVal) {
			continue
		}

		properties = append(properties, s.buildProperty(id, name, format, convertedVal))
	}

	return properties
}

// isMissingObject returns true if val indicates a "_missing_object" placeholder.
func (s *service) isMissingObject(val interface{}) bool {
	switch v := val.(type) {
	case string:
		return v == "_missing_object"
	case []interface{}:
		if len(v) == 1 {
			if str, ok := v[0].(string); ok {
				return str == "_missing_object"
			}
		}
	case Tag:
		return v.Id == "_missing_object"
	case []Tag:
		if len(v) == 1 && v[0].Id == "_missing_object" {
			return true
		}
	}
	return false
}

// buildProperty creates a Property based on the format and converted value.
func (s *service) buildProperty(id string, name string, format string, val interface{}) Property {
	prop := &Property{
		Id:     id,
		Name:   name,
		Format: format,
	}

	switch format {
	case "text":
		if str, ok := val.(string); ok {
			prop.Text = &str
		}
	case "number":
		if num, ok := val.(float64); ok {
			prop.Number = &num
		}
	case "select":
		if sel, ok := val.(Tag); ok {
			prop.Select = &sel
		}
	case "multi_select":
		if ms, ok := val.([]Tag); ok {
			prop.MultiSelect = ms
		}
	case "date":
		if dateStr, ok := val.(string); ok {
			prop.Date = &dateStr
		}
	case "file":
		if fileList, ok := val.([]interface{}); ok {
			var files []string
			for _, v := range fileList {
				if str, ok := v.(string); ok {
					files = append(files, str)
				}
			}
			prop.File = files
		}
	case "checkbox":
		if cb, ok := val.(bool); ok {
			prop.Checkbox = &cb
		}
	case "url":
		if urlStr, ok := val.(string); ok {
			prop.Url = &urlStr
		}
	case "email":
		if email, ok := val.(string); ok {
			prop.Email = &email
		}
	case "phone":
		if phone, ok := val.(string); ok {
			prop.Phone = &phone
		}
	case "object":
		if obj, ok := val.(string); ok {
			prop.Object = []string{obj}
		} else if objSlice, ok := val.([]interface{}); ok {
			var objects []string
			for _, v := range objSlice {
				if str, ok := v.(string); ok {
					objects = append(objects, str)
				}
			}
			prop.Object = objects
		}
	default:
		if str, ok := val.(string); ok {
			prop.Text = &str
		}
	}

	return *prop
}

// getPropertyIdAndName returns the property id and name from the ObjectShowResponse.
func (s *service) getPropertyIdAndName(key string, details *types.Struct) (string, string) {
	// Handle special cases first
	switch key {
	case bundle.RelationKeyCreator.String():
		return "created_by", "Created By"
	case bundle.RelationKeyCreatedDate.String():
		return "created_date", "Created Date"
	}

	if property, err := bundle.GetRelation(domain.RelationKey(key)); err == nil {
		return strcase.ToSnake(key), property.Name
	}

	// Fallback to resolving the property name
	spaceId := details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue()
	if name, err2 := util.ResolveRelationKeyToPropertyName(s.mw, spaceId, key); err2 == nil {
		return key, name
	}
	return key, key
}

// convertPropertyValue converts a protobuf types.Value into a native Go value.
func (s *service) convertPropertyValue(key string, value *types.Value, format string, details *types.Struct) interface{} {
	switch kind := value.Kind.(type) {
	case *types.Value_NullValue:
		return nil
	case *types.Value_NumberValue:
		if format == "date" {
			return time.Unix(int64(kind.NumberValue), 0).UTC().Format(time.RFC3339)
		}
		return kind.NumberValue
	case *types.Value_StringValue:
		// TODO: investigate how this is possible? select option not list and not returned in further details
		if format == "select" {
			return s.getTagFromStore(details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(), kind.StringValue)
		}
		if format == "multi_select" {
			return []Tag{s.getTagFromStore(details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(), kind.StringValue)}
		}
		return kind.StringValue
	case *types.Value_BoolValue:
		return kind.BoolValue
	case *types.Value_StructValue:
		m := make(map[string]interface{})
		for k, v := range kind.StructValue.Fields {
			m[k] = s.convertPropertyValue(key, v, format, details)
		}
		return m
	case *types.Value_ListValue:
		var list []interface{}
		for _, v := range kind.ListValue.Values {
			list = append(list, s.convertPropertyValue(key, v, format, details))
		}
		if format == "select" {
			return s.getTagFromStore(details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(), key)
		}
		if format == "multi_select" {
			return s.getTagFromStore(details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(), key)
		}
		return list
	default:
		return nil
	}
}

// getPropertyFormatMapFromLinks returns the map of property key to property format from the ObjectShowResponse.
func (s *service) getPropertyFormatMapFromLinks(propertyLinks []*model.RelationLink) map[string]string {
	propertyFormatToName := make(map[int32]string, len(model.RelationFormat_name))
	for k := range model.RelationFormat_name {
		propertyFormatToName[k] = s.MapRelationFormat(model.RelationFormat(k))
	}

	propertyFormatMap := map[string]string{}
	for _, detail := range propertyLinks {
		propertyFormatMap[detail.Key] = propertyFormatToName[int32(detail.Format)]
	}

	return propertyFormatMap
}

// getTagsFromDetails returns the list of tags from the ObjectShowResponse
func (s *service) getTagsFromDetails(key string, details []*model.ObjectViewDetailsSet) []Tag {
	tags := []Tag{}

	tagField, ok := details[0].Details.Fields[key]
	if !ok || tagField.GetListValue() == nil {
		return tags
	}

	for _, tagId := range tagField.GetListValue().Values {
		id := tagId.GetStringValue()
		for _, detail := range details {
			if detail.Id == id {
				tags = append(tags, Tag{
					Id:    id,
					Name:  detail.Details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
					Color: detail.Details.Fields[bundle.RelationKeyRelationOptionColor.String()].GetStringValue(),
				})
				break
			}
		}
	}
	return tags
}

// getBlocksFromDetails returns the list of blocks from the ObjectShowResponse.
func (s *service) getBlocksFromDetails(resp *pb.RpcObjectShowResponse) []Block {
	blocks := []Block{}

	for _, block := range resp.ObjectView.Blocks {
		var text *Text
		var file *File
		var property *Property

		switch content := block.Content.(type) {
		case *model.BlockContentOfText:
			text = &Text{
				Text:    content.Text.Text,
				Style:   model.BlockContentTextStyle_name[int32(content.Text.Style)],
				Checked: content.Text.Checked,
				Color:   content.Text.Color,
				Icon:    util.GetIcon(s.AccountInfo.GatewayUrl, content.Text.IconEmoji, content.Text.IconImage, "", 0),
			}
		case *model.BlockContentOfFile:
			file = &File{
				Hash:           content.File.Hash,
				Name:           content.File.Name,
				Type:           model.BlockContentFileType_name[int32(content.File.Type)],
				Mime:           content.File.Mime,
				Size:           content.File.Size(),
				AddedAt:        int(content.File.AddedAt),
				TargetObjectId: content.File.TargetObjectId,
				State:          model.BlockContentFileState_name[int32(content.File.State)],
				Style:          model.BlockContentFileStyle_name[int32(content.File.Style)],
			}
		case *model.BlockContentOfRelation:
			property = &Property{
				// TODO: is it sufficient to return the id only?
				Id: content.Relation.Key,
			}
		}
		// TODO: other content types?

		blocks = append(blocks, Block{
			Id:              block.Id,
			ChildrenIds:     block.ChildrenIds,
			BackgroundColor: block.BackgroundColor,
			Align:           model.BlockAlign_name[int32(block.Align)],
			VerticalAlign:   model.BlockVerticalAlign_name[int32(block.VerticalAlign)],
			Text:            text,
			File:            file,
			Property:        property,
		})
	}

	return blocks
}

// MapRelationFormat maps the relation format to a string.
func (s *service) MapRelationFormat(format model.RelationFormat) string {
	switch format {
	case model.RelationFormat_longtext:
		return "text"
	case model.RelationFormat_shorttext:
		return "text"
	case model.RelationFormat_tag:
		return "multi_select"
	case model.RelationFormat_status:
		return "select"
	default:
		return strcase.ToSnake(model.RelationFormat_name[int32(format)])
	}
}

// TODO: remove once bug of select option not being returned in details is fixed
func (s *service) getTagFromStore(spaceId string, tagId string) Tag {
	if tagId == "" {
		return Tag{}
	}

	resp := s.mw.ObjectShow(context.Background(), &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: tagId,
	})

	if resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
		return Tag{}
	}

	return Tag{
		Id:    tagId,
		Name:  resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Color: resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyRelationOptionColor.String()].GetStringValue(),
	}
}

// GetPropertyFormatMapsFromStore retrieves all properties from the store and returns a map of spaceId to property keys to their formats.
func (s *service) GetPropertyFormatMapsFromStore(spaceIds []string) (map[string]map[string]string, error) {
	spacesToProperties := make(map[string]map[string]string, len(spaceIds))

	for _, spaceId := range spaceIds {
		resp := s.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
			SpaceId: spaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyResolvedLayout.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
				},
				{
					Operator:    model.BlockContentDataviewFilter_No,
					RelationKey: bundle.RelationKeyIsHidden.String(),
					Condition:   model.BlockContentDataviewFilter_NotEqual,
					Value:       pbtypes.Bool(true),
				},
			},
			Keys: []string{bundle.RelationKeyUniqueKey.String(), bundle.RelationKeyRelationFormat.String()},
		})

		if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
			return nil, ErrFailedRetrievePropertyFormatMap
		}

		propertyFormatMap := make(map[string]string, len(resp.Records))
		for _, record := range resp.Records {
			name := strings.TrimPrefix(record.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(), "rel-")
			format := model.RelationFormat(record.Fields[bundle.RelationKeyRelationFormat.String()].GetNumberValue())
			propertyFormatMap[name] = s.MapRelationFormat(format)
		}

		spacesToProperties[spaceId] = propertyFormatMap
	}

	return spacesToProperties, nil
}

// GetTypeMapsFromStore retrieves all types from the store and returns a map of spaceId to type id to type.
func (s *service) GetTypeMapsFromStore(spaceIds []string) (map[string]map[string]Type, error) {
	spacesToTypes := make(map[string]map[string]Type, len(spaceIds))

	for _, spaceId := range spaceIds {
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
			Keys: []string{bundle.RelationKeyId.String(), bundle.RelationKeyUniqueKey.String(), bundle.RelationKeyName.String(), bundle.RelationKeyIconEmoji.String(), bundle.RelationKeyIconName.String(), bundle.RelationKeyIconOption.String(), bundle.RelationKeyRecommendedLayout.String(), bundle.RelationKeyIsArchived.String()},
		})

		if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
			return nil, ErrFailedRetrieveTypes
		}

		typeMap := make(map[string]Type, len(resp.Records))
		for _, record := range resp.Records {
			typeMap[record.Fields[bundle.RelationKeyId.String()].GetStringValue()] = Type{
				Object:            "type",
				Id:                record.Fields[bundle.RelationKeyId.String()].GetStringValue(),
				Key:               record.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(),
				Name:              record.Fields[bundle.RelationKeyName.String()].GetStringValue(),
				Icon:              util.GetIcon(s.AccountInfo.GatewayUrl, record.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", record.Fields[bundle.RelationKeyIconName.String()].GetStringValue(), record.Fields[bundle.RelationKeyIconOption.String()].GetNumberValue()),
				Archived:          record.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
				RecommendedLayout: model.ObjectTypeLayout_name[int32(record.Fields[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue())],
			}
		}

		spacesToTypes[spaceId] = typeMap
	}

	return spacesToTypes, nil
}

// GetObjectFromStruct creates an ObjectWithBlocks without blocks from the details.
func (s *service) GetObjectFromStruct(details *types.Struct, propertyFormatMap map[string]map[string]string, typeMap map[string]map[string]Type) Object {
	return Object{
		Object:     "object",
		Id:         details.Fields[bundle.RelationKeyId.String()].GetStringValue(),
		Name:       details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:       s.getIconFromStruct(details, s.AccountInfo.GatewayUrl),
		Archived:   details.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
		SpaceId:    details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(),
		Snippet:    details.Fields[bundle.RelationKeySnippet.String()].GetStringValue(),
		Layout:     model.ObjectTypeLayout_name[int32(details.Fields[bundle.RelationKeyResolvedLayout.String()].GetNumberValue())],
		Type:       s.getTypeFromStruct(details, typeMap[details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue()]),
		Properties: s.getPropertiesFromStruct(details, propertyFormatMap[details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue()]),
	}
}

// getTypeFromStruct retrieves the type from the details.
func (s *service) getTypeFromStruct(details *types.Struct, typeMap map[string]Type) Type {
	return typeMap[details.Fields[bundle.RelationKeyType.String()].GetStringValue()]
}

// getPropertiesFromStruct retrieves the properties from the details.
func (s *service) getPropertiesFromStruct(details *types.Struct, propertyFormatMap map[string]string) []Property {
	properties := []Property{}
	for key, value := range details.GetFields() {
		if _, isExcluded := excludedSystemProperties[key]; isExcluded {
			continue
		}

		id, name := s.getPropertyIdAndName(key, details)
		format := propertyFormatMap[key]
		convertedVal := s.convertPropertyValue(key, value, format, details)

		if s.isMissingObject(convertedVal) {
			continue
		}

		properties = append(properties, s.buildProperty(id, name, format, convertedVal))
	}

	return properties
}

// getIconFromStruct creates an icon from the details.
func (s *service) getIconFromStruct(details *types.Struct, gatewayUrl string) util.Icon {
	return util.GetIcon(gatewayUrl, details.GetFields()[bundle.RelationKeyIconEmoji.String()].GetStringValue(), details.GetFields()[bundle.RelationKeyIconImage.String()].GetStringValue(), details.GetFields()[bundle.RelationKeyIconName.String()].GetStringValue(), details.GetFields()[bundle.RelationKeyIconOption.String()].GetNumberValue())
}
