package object

import (
	"context"
	"errors"
	"slices"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/api/apicore"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
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
)

type Service interface {
	ListObjects(ctx context.Context, spaceId string, offset int, limit int) ([]Object, int, bool, error)
	GetObject(ctx context.Context, spaceId string, objectId string) (ObjectWithBlocks, error)
	CreateObject(ctx context.Context, spaceId string, request CreateObjectRequest) (ObjectWithBlocks, error)
	DeleteObject(ctx context.Context, spaceId string, objectId string) (ObjectWithBlocks, error)
	GetObjectExport(ctx context.Context, spaceId string, objectId string, format string) (string, error)

	ListProperties(ctx context.Context, spaceId string, offset int, limit int) ([]Property, int, bool, error)
	GetProperty(ctx context.Context, spaceId string, propertyId string) (Property, error)
	CreateProperty(ctx context.Context, spaceId string, request CreatePropertyRequest) (Property, error)
	UpdateProperty(ctx context.Context, spaceId string, propertyId string, request UpdatePropertyRequest) (Property, error)
	DeleteProperty(ctx context.Context, spaceId string, propertyId string) (Property, error)

	ListTags(ctx context.Context, spaceId string, propertyId string, offset int, limit int) ([]Tag, int, bool, error)
	GetTag(ctx context.Context, spaceId string, propertyId string, tagId string) (Tag, error)
	CreateTag(ctx context.Context, spaceId string, propertyId string, request CreateTagRequest) (Tag, error)
	UpdateTag(ctx context.Context, spaceId string, propertyId string, tagId string, request UpdateTagRequest) (Tag, error)
	DeleteTag(ctx context.Context, spaceId string, propertyId string, tagId string) (Tag, error)

	ListTypes(ctx context.Context, spaceId string, offset int, limit int) ([]Type, int, bool, error)
	GetType(ctx context.Context, spaceId string, typeId string) (Type, error)
	ListTemplates(ctx context.Context, spaceId string, typeId string, offset int, limit int) ([]Template, int, bool, error)
	GetTemplate(ctx context.Context, spaceId string, typeId string, templateId string) (Template, error)

	MapRelationFormat(format model.RelationFormat) PropertyFormat
	GetObjectFromStruct(details *types.Struct, propertyMap map[string]Property, typeMap map[string]Type) Object
	GetPropertyMapFromStore(spaceId string) (map[string]Property, error)
	GetPropertyMapsFromStore(spaceIds []string) (map[string]map[string]Property, error)
	GetTypeMapFromStore(spaceId string, propertyMap map[string]Property) (map[string]Type, error)
	GetTypeMapsFromStore(spaceIds []string, propertyMap map[string]map[string]Property) (map[string]map[string]Type, error)
	GetTypeFromDetails(details []*model.ObjectViewDetailsSet, typeId string, propertyMap map[string]Property) Type
}

type service struct {
	mw            apicore.ClientCommands
	gatewayUrl    string
	exportService apicore.ExportService
}

func NewService(mw apicore.ClientCommands, exportService apicore.ExportService, gatewayUrl string) Service {
	return &service{mw: mw, exportService: exportService, gatewayUrl: gatewayUrl}
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
		Icon:       GetIcon(s.gatewayUrl, resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconImage.String()].GetStringValue(), resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconName.String()].GetStringValue(), resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconOption.String()].GetNumberValue()),
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
		if !IsEmoji(*request.Icon.Emoji) {
			return nil, util.ErrBadInput("icon emoji is not valid")
		}
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
		rk := FromPropertyApiKey(key)
		if _, isExcluded := excludedSystemProperties[rk]; isExcluded {
			continue
		}

		if slices.Contains(bundle.LocalAndDerivedRelationKeys, domain.RelationKey(key)) {
			return nil, util.ErrBadInput("property '" + key + "' cannot be set directly")
		}

		if prop, ok := propertyMap[rk]; ok {
			sanitized, err := s.sanitizeAndValidatePropertyValue(ctx, spaceId, key, prop.Format, val, prop)
			if err != nil {
				return nil, err
			}
			fields[rk] = pbtypes.ToValue(sanitized)
		} else {
			return nil, errors.New("unknown property '" + key + "' must be a string")
		}
	}

	return &types.Struct{Fields: fields}, nil
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
				Icon:    GetIcon(s.gatewayUrl, content.Text.IconEmoji, content.Text.IconImage, "", 0),
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

// GetObjectFromStruct creates an ObjectWithBlocks without blocks from the details.
func (s *service) GetObjectFromStruct(details *types.Struct, propertyMap map[string]Property, typeMap map[string]Type) Object {
	return Object{
		Object:     "object",
		Id:         details.Fields[bundle.RelationKeyId.String()].GetStringValue(),
		Name:       details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:       GetIcon(s.gatewayUrl, details.GetFields()[bundle.RelationKeyIconEmoji.String()].GetStringValue(), details.GetFields()[bundle.RelationKeyIconImage.String()].GetStringValue(), details.GetFields()[bundle.RelationKeyIconName.String()].GetStringValue(), details.GetFields()[bundle.RelationKeyIconOption.String()].GetNumberValue()),
		Archived:   details.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
		SpaceId:    details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(),
		Snippet:    details.Fields[bundle.RelationKeySnippet.String()].GetStringValue(),
		Layout:     model.ObjectTypeLayout_name[int32(details.Fields[bundle.RelationKeyResolvedLayout.String()].GetNumberValue())],
		Type:       s.getTypeFromStruct(details, typeMap),
		Properties: s.getPropertiesFromStruct(details, propertyMap),
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
