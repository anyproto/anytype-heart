package object

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/iancoleman/strcase"

	"github.com/anyproto/anytype-heart/core/api/apicore"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
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
	ErrFailedRetrievePropertyMap = errors.New("failed to retrieve property  map")
	ErrFailedDeleteObject        = errors.New("failed to delete object")
	ErrFailedCreateObject        = errors.New("failed to create object")
	ErrFailedSetPropertyFeatured = errors.New("failed to set property featured")
	ErrFailedCreateBookmark      = errors.New("failed to fetch bookmark")
	ErrFailedCreateBlock         = errors.New("failed to create block")
	ErrFailedPasteBody           = errors.New("failed to paste body")

	// properties
	ErrFailedRetrieveProperties      = errors.New("failed to retrieve properties")
	ErrFailedRetrieveProperty        = errors.New("failed to retrieve property")
	ErrPropertyNotFound              = errors.New("property not found")
	ErrPropertyDeleted               = errors.New("property deleted")
	ErrFailedRetrievePropertyOptions = errors.New("failed to retrieve property options")

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
	bundle.RelationKeyIconOption.String():             true,
	bundle.RelationKeyIconName.String():               true,
}

type Service interface {
	ListObjects(ctx context.Context, spaceId string, offset int, limit int) ([]Object, int, bool, error)
	GetObject(ctx context.Context, spaceId string, objectId string) (ObjectWithBlocks, error)
	DeleteObject(ctx context.Context, spaceId string, objectId string) (ObjectWithBlocks, error)
	CreateObject(ctx context.Context, spaceId string, request CreateObjectRequest) (ObjectWithBlocks, error)
	ListProperties(ctx context.Context, spaceId string, offset int, limit int) ([]Property, int, bool, error)
	GetProperty(ctx context.Context, spaceId string, propertyId string) (Property, error)
	ListPropertyOptions(ctx context.Context, spaceId string, propertyId string, offset int, limit int) ([]Tag, int, bool, error)
	ListTypes(ctx context.Context, spaceId string, offset int, limit int) ([]Type, int, bool, error)
	GetType(ctx context.Context, spaceId string, typeId string) (Type, error)
	ListTemplates(ctx context.Context, spaceId string, typeId string, offset int, limit int) ([]Template, int, bool, error)
	GetTemplate(ctx context.Context, spaceId string, typeId string, templateId string) (Template, error)

	MapRelationFormat(format model.RelationFormat) string
	GetObjectFromStruct(details *types.Struct, propertyMap map[string]Property, typeMap map[string]Type) Object
	GetPropertyMapFromStore(spaceId string) (map[string]Property, error)
	GetPropertyMapsFromStore(spaceIds []string) (map[string]map[string]Property, error)
	GetTypeMapFromStore(spaceId string, propertyMap map[string]Property) (map[string]Type, error)
	GetTypeMapsFromStore(spaceIds []string, propertyMap map[string]map[string]Property) (map[string]map[string]Type, error)
	GetTypeFromDetails(details []*model.ObjectViewDetailsSet, typeId string, propertyMap map[string]Property) Type
}

type service struct {
	mw         apicore.ClientCommands
	gatewayUrl string
}

func NewService(mw apicore.ClientCommands, gatewayUrl string) Service {
	return &service{mw: mw, gatewayUrl: gatewayUrl}
}

// ListObjects retrieves a paginated list of objects in a specific space.
func (s *service) ListObjects(ctx context.Context, spaceId string, offset int, limit int) (objects []Object, total int, hasMore bool, err error) {
	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
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
				RelationKey: "type.uniqueKey",
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.String("ot-template"),
			},
			{
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
	propertyMap, err := s.GetPropertyMapFromStore(spaceId)
	if err != nil {
		return nil, 0, false, err
	}
	typeMap, err := s.GetTypeMapFromStore(spaceId, propertyMap)
	if err != nil {
		return nil, 0, false, err
	}

	for _, record := range paginatedObjects {
		objects = append(objects, s.GetObjectFromStruct(record, propertyMap, typeMap))
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

	// pre-fetch properties to fill the object
	propertyMap, err := s.GetPropertyMapFromStore(spaceId)
	if err != nil {
		return ObjectWithBlocks{}, err
	}

	object := ObjectWithBlocks{
		Object:     "object",
		Id:         resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyId.String()].GetStringValue(),
		Name:       resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:       util.GetIcon(s.gatewayUrl, resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconImage.String()].GetStringValue(), resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconName.String()].GetStringValue(), resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconOption.String()].GetNumberValue()),
		Archived:   resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
		SpaceId:    resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(),
		Snippet:    resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeySnippet.String()].GetStringValue(),
		Layout:     model.ObjectTypeLayout_name[int32(resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyResolvedLayout.String()].GetNumberValue())],
		Type:       s.GetTypeFromDetails(resp.ObjectView.Details, resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyType.String()].GetStringValue(), propertyMap),
		Properties: s.getPropertiesFromStruct(resp.ObjectView.Details[0].Details, propertyMap),
		Blocks:     s.getBlocksFromDetails(resp),
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
	details, err := s.buildObjectDetails(ctx, spaceId, request)
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
			fmt.Println("Error creating object:", resp.Error)
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
			fmt.Println("Error creating object:", resp.Error)
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
func (s *service) buildObjectDetails(ctx context.Context, spaceId string, request CreateObjectRequest) (*types.Struct, error) {
	// Validate bookmark source
	if request.TypeKey == "ot-bookmark" && request.Source == "" {
		return nil, util.ErrBadInput("source is missing for bookmark")
	}

	// Validate icon: only allow either emoji or file, and disallow name and color fields.
	if request.Icon.Name != nil || request.Icon.Color != nil {
		return nil, util.ErrBadInput("icon name and color are not supported for object")
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

	if request.Properties == nil {
		return &types.Struct{Fields: fields}, nil
	}

	propertyMap, err := s.GetPropertyMapFromStore(spaceId)
	if err != nil {
		return nil, err
	}

	for key, val := range request.Properties {
		if excludedSystemProperties[key] {
			continue
		}

		if prop, ok := propertyMap[key]; ok {
			sanitized, err := s.sanitizeAndValidatePropertyValue(ctx, spaceId, key, prop.Format, val, prop)
			if err != nil {
				return nil, err
			}
			fields[key] = pbtypes.ToValue(sanitized)
		} else {
			return nil, errors.New("unknown property '" + key + "' must be a string")
		}
	}

	return &types.Struct{Fields: fields}, nil
}

// sanitizeAndValidatePropertyValue checks the value for a property according to its format and ensures referenced IDs exist and are valid.
func (s *service) sanitizeAndValidatePropertyValue(ctx context.Context, spaceId string, key string, format string, value interface{}, property Property) (interface{}, error) {
	switch format {
	case "text", "url", "email", "phone":
		str, ok := value.(string)
		if !ok {
			return nil, util.ErrBadInput("property '" + key + "' must be a string")
		}
		return str, nil
	case "number":
		num, ok := value.(float64)
		if !ok {
			return nil, util.ErrBadInput("property '" + key + "' must be a number")
		}
		return num, nil
	case "checkbox":
		b, ok := value.(bool)
		if !ok {
			return nil, util.ErrBadInput("property '" + key + "' must be a boolean")
		}
		return b, nil
	case "select":
		id, ok := value.(string)
		if !ok {
			return nil, util.ErrBadInput("property '" + key + "' must be a string (option id)")
		}
		if !s.isValidSelectOption(ctx, spaceId, property, id) {
			return nil, util.ErrBadInput("invalid select option for '" + key + "': " + id)
		}
		return id, nil
	case "multi_select":
		ids, ok := value.([]interface{})
		if !ok {
			return nil, util.ErrBadInput("property '" + key + "' must be an array of option ids")
		}
		var validIds []string
		for _, v := range ids {
			id, ok := v.(string)
			if !ok || !s.isValidSelectOption(ctx, spaceId, property, id) {
				return nil, util.ErrBadInput("invalid multi_select option for '" + key + "': " + id)
			}
			validIds = append(validIds, id)
		}
		return validIds, nil
	case "object", "file":
		id, ok := value.(string)
		if !ok {
			return nil, util.ErrBadInput("property '" + key + "' must be a string (object/file id)")
		}
		if !s.isValidObjectReference(ctx, spaceId, id) {
			return nil, util.ErrBadInput("invalid " + format + " id for '" + key + "': " + id)
		}
		return id, nil
	default:
		return nil, util.ErrBadInput("unsupported property format: " + format)
	}
}

// isValidSelectOption checks if the option id is valid for the given property.
func (s *service) isValidSelectOption(ctx context.Context, spaceId string, property Property, optionId string) bool {
	// TODO: refine logic
	tags, _, _, err := s.ListPropertyOptions(ctx, spaceId, property.Id, 0, 1000)
	if err != nil {
		return false
	}
	for _, tag := range tags {
		if tag.Id == optionId {
			return true
		}
	}
	return false
}

func (s *service) isValidObjectReference(ctx context.Context, spaceId string, objectId string) bool {
	// TODO: implement proper validation
	return true
}

// ListProperties returns a list of properties for a specific space.
func (s *service) ListProperties(ctx context.Context, spaceId string, offset int, limit int) (properties []Property, total int, hasMore bool, err error) {
	resp := s.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			{
				RelationKey: bundle.RelationKeyIsHidden.String(),
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.Bool(true),
			},
		},
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyUniqueKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyRelationFormat.String(),
		},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedRetrieveProperties
	}

	total = len(resp.Records)
	paginatedProperties, hasMore := pagination.Paginate(resp.Records, offset, limit)
	properties = make([]Property, 0, len(paginatedProperties))

	for _, record := range paginatedProperties {
		properties = append(properties, Property{
			Id:     record.Fields[bundle.RelationKeyId.String()].GetStringValue(),
			Key:    strings.TrimPrefix(record.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(), "rel-"),
			Name:   record.Fields[bundle.RelationKeyName.String()].GetStringValue(),
			Format: s.MapRelationFormat(model.RelationFormat(record.Fields[bundle.RelationKeyRelationFormat.String()].GetNumberValue())),
		})
	}

	return properties, total, hasMore, nil
}

// GetProperty retrieves a single property by its ID in a specific space.
func (s *service) GetProperty(ctx context.Context, spaceId string, propertyId string) (Property, error) {
	// TODO: change to object show to return possible deleted status, after fixing id / key
	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyUniqueKey.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(propertyId),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
			},
		},
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyUniqueKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyRelationFormat.String(),
		},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return Property{}, ErrFailedRetrieveProperty
	}

	if len(resp.Records) == 0 {
		return Property{}, ErrPropertyNotFound
	}

	property := resp.Records[0]
	return Property{
		Id:     property.Fields[bundle.RelationKeyId.String()].GetStringValue(),
		Key:    strings.TrimPrefix(property.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(), "rel-"),
		Name:   property.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Format: s.MapRelationFormat(model.RelationFormat(property.Fields[bundle.RelationKeyRelationFormat.String()].GetNumberValue())),
	}, nil
}

// ListPropertyOptions returns all tag options for a given property (relation) id in a space.
func (s *service) ListPropertyOptions(ctx context.Context, spaceId string, propertyId string, offset int, limit int) (options []Tag, total int, hasMore bool, err error) {
	// TODO: decide key vs id
	// propertyKey, err := util.ResolveIdtoUniqueKey(s.mw, spaceId, propertyId)
	// if err != nil {
	// 	return nil, 0, false, err
	// }
	propertyKey := propertyId

	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_relationOption)),
			},
			{
				RelationKey: bundle.RelationKeyRelationKey.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(propertyKey),
			},
		},
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyRelationOptionColor.String(),
		},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedRetrievePropertyOptions
	}

	if len(resp.Records) == 0 {
		return []Tag{}, 0, false, nil
	}

	total = len(resp.Records)
	paginatedOptions, hasMore := pagination.Paginate(resp.Records, offset, limit)
	options = make([]Tag, 0, len(paginatedOptions))

	for _, record := range resp.Records {
		options = append(options, Tag{
			Id:    record.Fields[bundle.RelationKeyId.String()].GetStringValue(),
			Name:  record.Fields[bundle.RelationKeyName.String()].GetStringValue(),
			Color: util.ColorOptionToColor[record.Fields[bundle.RelationKeyRelationOptionColor.String()].GetStringValue()],
		})
	}

	return options, total, hasMore, nil
}

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

// getRecommendedPropertiesFromLists combines featured and regular properties into a list of Properties.
func (s *service) getRecommendedPropertiesFromLists(featured, regular *types.ListValue, propertyMap map[string]Property) []Property {
	var props []Property
	lists := []*types.ListValue{featured, regular}
	for _, lst := range lists {
		if lst == nil {
			continue
		}
		for _, v := range lst.Values {
			key := v.GetStringValue()
			if key == "" {
				continue
			}
			if p, ok := propertyMap[key]; ok {
				props = append(props, p)
			}
		}
	}
	return props
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
func (s *service) buildProperty(id string, key string, name string, format string, val interface{}) Property {
	prop := &Property{
		Id:     id,
		Key:    key,
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
			tags := s.getTagsFromStore(details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(), []string{kind.StringValue})
			if len(tags) > 0 {
				return tags[0]
			}
			return nil
		}
		if format == "multi_select" {
			return s.getTagsFromStore(details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(), []string{kind.StringValue})
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
		if format == "select" {
			listValues := kind.ListValue.Values
			if len(listValues) > 0 {
				tags := s.getTagsFromStore(details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(), []string{listValues[0].GetStringValue()})
				if len(tags) > 0 {
					return tags[0]
				}
			}
			return nil
		}
		if format == "multi_select" {
			listValues := kind.ListValue.Values
			if len(listValues) > 0 {
				listStringValues := make([]string, len(listValues))
				for i, v := range listValues {
					listStringValues[i] = v.GetStringValue()
				}
				return s.getTagsFromStore(details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(), listStringValues)
			}
			return nil
		}
		var list []interface{}
		for _, v := range kind.ListValue.Values {
			list = append(list, s.convertPropertyValue(key, v, format, details))
		}
		return list
	default:
		return nil
	}
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
				Icon:    util.GetIcon(s.gatewayUrl, content.Text.IconEmoji, content.Text.IconImage, "", 0),
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
				// TODO: is it sufficient to return the key only?
				Key: content.Relation.Key,
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
func (s *service) getTagsFromStore(spaceId string, tagIds []string) []Tag {
	tags := make([]Tag, 0, len(tagIds))
	for _, tagId := range tagIds {
		if tagId == "" {
			continue
		}

		resp := s.mw.ObjectShow(context.Background(), &pb.RpcObjectShowRequest{
			SpaceId:  spaceId,
			ObjectId: tagId,
		})

		if resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			continue
		}

		tags = append(tags, Tag{
			Id:    tagId,
			Name:  resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
			Color: util.ColorOptionToColor[resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyRelationOptionColor.String()].GetStringValue()],
		})
	}

	return tags
}

// GetPropertyMapFromStore retrieves all properties for a specific space
// Property entries are also keyed by property id. Required for filling types with properties, as recommended properties are referenced by id and not key.
func (s *service) GetPropertyMapFromStore(spaceId string) (map[string]Property, error) {
	resp := s.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			{
				RelationKey: bundle.RelationKeyIsHidden.String(),
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.Bool(true),
			},
		},
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyUniqueKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyRelationFormat.String(),
		},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, ErrFailedRetrievePropertyMap
	}

	propertyMap := make(map[string]Property, len(resp.Records))
	for _, record := range resp.Records {
		propertyKey := strings.TrimPrefix(record.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(), "rel-")

		var key, name string
		switch propertyKey {
		case bundle.RelationKeyCreator.String():
			key = "created_by"
			name = "Created By"
		case bundle.RelationKeyCreatedDate.String():
			key = "created_date"
			name = "Created Date"
		default:
			// check if the property is custom or bundled
			if len(propertyKey) == 24 && strings.ContainsAny(propertyKey, "0123456789") {
				key = propertyKey
			} else {
				key = strcase.ToSnake(propertyKey)
			}
			name = record.Fields[bundle.RelationKeyName.String()].GetStringValue()
		}

		p := Property{
			Id:     record.Fields[bundle.RelationKeyId.String()].GetStringValue(),
			Key:    key,
			Name:   name,
			Format: s.MapRelationFormat(model.RelationFormat(record.Fields[bundle.RelationKeyRelationFormat.String()].GetNumberValue())),
		}
		propertyMap[propertyKey] = p
		propertyMap[p.Id] = p // add property under id as key to map as well
	}

	return propertyMap, nil
}

// GetPropertyMapsFromStore retrieves all properties for all spaces.
func (s *service) GetPropertyMapsFromStore(spaceIds []string) (map[string]map[string]Property, error) {
	spacesToProperties := make(map[string]map[string]Property, len(spaceIds))

	for _, spaceId := range spaceIds {
		propertyMap, err := s.GetPropertyMapFromStore(spaceId)
		if err != nil {
			return nil, err
		}
		spacesToProperties[spaceId] = propertyMap
	}

	return spacesToProperties, nil
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

// GetObjectFromStruct creates an ObjectWithBlocks without blocks from the details.
func (s *service) GetObjectFromStruct(details *types.Struct, propertyMap map[string]Property, typeMap map[string]Type) Object {
	return Object{
		Object:     "object",
		Id:         details.Fields[bundle.RelationKeyId.String()].GetStringValue(),
		Name:       details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:       util.GetIcon(s.gatewayUrl, details.GetFields()[bundle.RelationKeyIconEmoji.String()].GetStringValue(), details.GetFields()[bundle.RelationKeyIconImage.String()].GetStringValue(), details.GetFields()[bundle.RelationKeyIconName.String()].GetStringValue(), details.GetFields()[bundle.RelationKeyIconOption.String()].GetNumberValue()),
		Archived:   details.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
		SpaceId:    details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(),
		Snippet:    details.Fields[bundle.RelationKeySnippet.String()].GetStringValue(),
		Layout:     model.ObjectTypeLayout_name[int32(details.Fields[bundle.RelationKeyResolvedLayout.String()].GetNumberValue())],
		Type:       s.getTypeFromStruct(details, typeMap),
		Properties: s.getPropertiesFromStruct(details, propertyMap),
	}
}

// getTypeFromStruct retrieves the type from the details.
func (s *service) getTypeFromStruct(details *types.Struct, typeMap map[string]Type) Type {
	return typeMap[details.Fields[bundle.RelationKeyType.String()].GetStringValue()]
}

// getPropertiesFromStruct retrieves the properties from the details.
func (s *service) getPropertiesFromStruct(details *types.Struct, propertyMap map[string]Property) []Property {
	properties := make([]Property, 0)
	for propertyKey, value := range details.GetFields() {
		if _, isExcluded := excludedSystemProperties[propertyKey]; isExcluded {
			continue
		}

		key := propertyMap[propertyKey].Key
		format := propertyMap[propertyKey].Format
		convertedVal := s.convertPropertyValue(key, value, format, details)

		if s.isMissingObject(convertedVal) {
			continue
		}

		id := propertyMap[propertyKey].Id
		name := propertyMap[propertyKey].Name
		properties = append(properties, s.buildProperty(id, key, name, format, convertedVal))
	}

	return properties
}
