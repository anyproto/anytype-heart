package object

import (
	"context"
	"errors"
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
	ErrObjectNotFound            = errors.New("object not found")
	ErrObjectDeleted             = errors.New("object deleted")
	ErrFailedRetrieveObject      = errors.New("failed to retrieve object")
	ErrFailedRetrieveObjects     = errors.New("failed to retrieve list of objects")
	ErrFailedDeleteObject        = errors.New("failed to delete object")
	ErrFailedCreateObject        = errors.New("failed to create object")
	ErrInputMissingSource        = errors.New("source is missing for bookmark")
	ErrIconNameColorNotSupported = errors.New("icon name and color are not supported for object")
	ErrFailedSetPropertyFeatured = errors.New("failed to set property featured")
	ErrFailedCreateBookmark      = errors.New("failed to fetch bookmark")
	ErrFailedCreateBlock         = errors.New("failed to create block")
	ErrFailedPasteBody           = errors.New("failed to paste body")

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
	ListObjects(ctx context.Context, spaceId string, offset int, limit int) ([]Object, int, bool, error)
	GetObject(ctx context.Context, spaceId string, objectId string) (Object, error)
	DeleteObject(ctx context.Context, spaceId string, objectId string) (Object, error)
	CreateObject(ctx context.Context, spaceId string, request CreateObjectRequest) (Object, error)
	ListTypes(ctx context.Context, spaceId string, offset int, limit int) ([]Type, int, bool, error)
	GetType(ctx context.Context, spaceId string, typeId string) (Type, error)
	ListTemplates(ctx context.Context, spaceId string, typeId string, offset int, limit int) ([]Template, int, bool, error)
	GetTemplate(ctx context.Context, spaceId string, typeId string, templateId string) (Template, error)
}

type ObjectService struct {
	mw           apicore.ClientCommands
	spaceService *space.SpaceService
	AccountInfo  *model.AccountInfo
}

func NewService(mw apicore.ClientCommands, spaceService *space.SpaceService) *ObjectService {
	return &ObjectService{mw: mw, spaceService: spaceService}
}

// ListObjects retrieves a paginated list of objects in a specific space.
func (s *ObjectService) ListObjects(ctx context.Context, spaceId string, offset int, limit int) (objects []Object, total int, hasMore bool, err error) {
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
		Keys: []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String()},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedRetrieveObjects
	}

	total = len(resp.Records)
	paginatedObjects, hasMore := pagination.Paginate(resp.Records, offset, limit)
	objects = make([]Object, 0, len(paginatedObjects))

	for _, record := range paginatedObjects {
		object, err := s.GetObject(ctx, spaceId, record.Fields[bundle.RelationKeyId.String()].GetStringValue())
		if err != nil {
			return nil, 0, false, err
		}

		objects = append(objects, object)
	}
	return objects, total, hasMore, nil
}

// GetObject retrieves a single object by its ID in a specific space.
func (s *ObjectService) GetObject(ctx context.Context, spaceId string, objectId string) (Object, error) {
	resp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: objectId,
	})

	if resp.Error != nil {
		if resp.Error.Code == pb.RpcObjectShowResponseError_NOT_FOUND {
			return Object{}, ErrObjectNotFound
		}

		if resp.Error.Code == pb.RpcObjectShowResponseError_OBJECT_DELETED {
			return Object{}, ErrObjectDeleted
		}

		if resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			return Object{}, ErrFailedRetrieveObject
		}
	}

	details := resp.ObjectView.Details[0].Details.Fields
	icon := util.GetIcon(s.AccountInfo, details[bundle.RelationKeyIconEmoji.String()].GetStringValue(), details[bundle.RelationKeyIconImage.String()].GetStringValue(), details[bundle.RelationKeyIconName.String()].GetStringValue(), details[bundle.RelationKeyIconOption.String()].GetNumberValue())

	object := Object{
		Object:     "object",
		Id:         details[bundle.RelationKeyId.String()].GetStringValue(),
		Name:       details[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:       icon,
		Archived:   details[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
		SpaceId:    details[bundle.RelationKeySpaceId.String()].GetStringValue(),
		Snippet:    details[bundle.RelationKeySnippet.String()].GetStringValue(),
		Layout:     model.ObjectTypeLayout_name[int32(details[bundle.RelationKeyResolvedLayout.String()].GetNumberValue())],
		Type:       s.GetTypeFromDetails(details[bundle.RelationKeyType.String()].GetStringValue(), resp.ObjectView.Details),
		Blocks:     s.getBlocks(resp),
		Properties: s.getProperties(resp),
	}

	return object, nil
}

// DeleteObject deletes an existing object in a specific space.
func (s *ObjectService) DeleteObject(ctx context.Context, spaceId string, objectId string) (Object, error) {
	object, err := s.GetObject(ctx, spaceId, objectId)
	if err != nil {
		return Object{}, err
	}

	resp := s.mw.ObjectSetIsArchived(ctx, &pb.RpcObjectSetIsArchivedRequest{
		ContextId:  objectId,
		IsArchived: true,
	})

	if resp.Error.Code != pb.RpcObjectSetIsArchivedResponseError_NULL {
		return Object{}, ErrFailedDeleteObject
	}

	return object, nil
}

// CreateObject creates a new object in a specific space.
func (s *ObjectService) CreateObject(ctx context.Context, spaceId string, request CreateObjectRequest) (Object, error) {
	details, err := s.buildObjectDetails(request)
	if err != nil {
		return Object{}, err
	}

	var objectId string
	if request.TypeKey == "ot-bookmark" {
		resp := s.mw.ObjectCreateBookmark(ctx, &pb.RpcObjectCreateBookmarkRequest{
			Details:    details,
			SpaceId:    spaceId,
			TemplateId: request.TemplateId,
		})

		if resp.Error.Code != pb.RpcObjectCreateBookmarkResponseError_NULL {
			return Object{}, ErrFailedCreateBookmark
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
			return Object{}, ErrFailedCreateObject
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
func (s *ObjectService) buildObjectDetails(request CreateObjectRequest) (*types.Struct, error) {
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
func (s *ObjectService) ListTypes(ctx context.Context, spaceId string, offset int, limit int) (types []Type, total int, hasMore bool, err error) {
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
			Icon:              util.GetIcon(s.AccountInfo, record.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", record.Fields[bundle.RelationKeyIconName.String()].GetStringValue(), record.Fields[bundle.RelationKeyIconOption.String()].GetNumberValue()),
			Archived:          record.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
			RecommendedLayout: model.ObjectTypeLayout_name[int32(record.Fields[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue())],
		})
	}
	return types, total, hasMore, nil
}

// GetType returns a single type by its ID in a specific space.
func (s *ObjectService) GetType(ctx context.Context, spaceId string, typeId string) (Type, error) {
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
		Icon:              util.GetIcon(s.AccountInfo, details[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", details[bundle.RelationKeyIconName.String()].GetStringValue(), details[bundle.RelationKeyIconOption.String()].GetNumberValue()),
		Archived:          details[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
		RecommendedLayout: model.ObjectTypeLayout_name[int32(details[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue())],
	}, nil
}

// ListTemplates returns a paginated list of templates in a specific space.
func (s *ObjectService) ListTemplates(ctx context.Context, spaceId string, typeId string, offset int, limit int) (templates []Template, total int, hasMore bool, err error) {
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
			Icon:     util.GetIcon(s.AccountInfo, templateResp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", "", 0),
			Archived: templateResp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
		})
	}

	return templates, total, hasMore, nil
}

// GetTemplate returns a single template by its ID in a specific space.
func (s *ObjectService) GetTemplate(ctx context.Context, spaceId string, typeId string, templateId string) (Template, error) {
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
		Icon:     util.GetIcon(s.AccountInfo, resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", "", 0),
		Archived: resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
	}, nil
}

// GetTypeFromDetails returns the type from the details of the ObjectShowResponse.
func (s *ObjectService) GetTypeFromDetails(typeId string, details []*model.ObjectViewDetailsSet) Type {
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
		Id:                typeId,
		Key:               objectTypeDetail.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(),
		Name:              objectTypeDetail.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:              util.GetIcon(s.AccountInfo, objectTypeDetail.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", objectTypeDetail.Fields[bundle.RelationKeyIconName.String()].GetStringValue(), objectTypeDetail.Fields[bundle.RelationKeyIconOption.String()].GetNumberValue()),
		RecommendedLayout: model.ObjectTypeLayout_name[int32(objectTypeDetail.Fields[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue())],
	}
}

// getProperties returns a list of properties by iterating over all properties found in the RelationLinks and mapping their format and value.
func (s *ObjectService) getProperties(resp *pb.RpcObjectShowResponse) []Property {
	propertyFormatMap := s.getPropertyFormatMap(resp.ObjectView.RelationLinks)
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

		id, name := s.getProperty(key, resp)
		format := propertyFormatMap[key]
		convertedVal := s.convertValue(key, primaryDetailFields[key], format, resp.ObjectView.Details)

		if s.isMissingObject(convertedVal) {
			continue
		}

		properties = append(properties, s.buildProperty(id, name, format, convertedVal))
	}

	return properties
}

// isMissingObject returns true if val indicates a "_missing_object" placeholder.
func (s *ObjectService) isMissingObject(val interface{}) bool {
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
func (s *ObjectService) buildProperty(id string, name string, format string, val interface{}) Property {
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

// getProperty returns the property id and name from the ObjectShowResponse.
func (s *ObjectService) getProperty(key string, resp *pb.RpcObjectShowResponse) (string, string) {
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
	spaceId := resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue()
	if name, err2 := util.ResolveRelationKeyToPropertyName(s.mw, spaceId, key); err2 == nil {
		return key, name
	}
	return key, key
}

// convertValue converts a protobuf types.Value into a native Go value.
func (s *ObjectService) convertValue(key string, value *types.Value, format string, details []*model.ObjectViewDetailsSet) interface{} {
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
			return s.resolveTag(details[0].Details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(), kind.StringValue)
		}
		if format == "multi_select" {
			return []Tag{s.resolveTag(details[0].Details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(), kind.StringValue)}
		}
		return kind.StringValue
	case *types.Value_BoolValue:
		return kind.BoolValue
	case *types.Value_StructValue:
		m := make(map[string]interface{})
		for k, v := range kind.StructValue.Fields {
			m[k] = s.convertValue(key, v, format, details)
		}
		return m
	case *types.Value_ListValue:
		var list []interface{}
		for _, v := range kind.ListValue.Values {
			list = append(list, s.convertValue(key, v, format, details))
		}
		if format == "select" {
			tags := s.getTags(key, details)
			if len(tags) > 0 {
				return tags[0]
			}
			return nil
		}
		if format == "multi_select" {
			return s.getTags(key, details)
		}
		return list
	default:
		return nil
	}
}

// getPropertyFormatMap returns the map of property key to property format from the ObjectShowResponse.
func (s *ObjectService) getPropertyFormatMap(propertyLinks []*model.RelationLink) map[string]string {
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

// TODO: remove once bug of select option not being returned in details is fixed
func (s *ObjectService) resolveTag(spaceId, tagId string) Tag {
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

// getTags returns the list of tags from the ObjectShowResponse
func (s *ObjectService) getTags(key string, details []*model.ObjectViewDetailsSet) []Tag {
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

// getBlocks returns the list of blocks from the ObjectShowResponse.
func (s *ObjectService) getBlocks(resp *pb.RpcObjectShowResponse) []Block {
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
				Icon:    util.GetIcon(s.AccountInfo, content.Text.IconEmoji, content.Text.IconImage, "", 0),
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
func (s *ObjectService) MapRelationFormat(format model.RelationFormat) string {
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
