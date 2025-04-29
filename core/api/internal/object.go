package internal

import (
	"context"
	"errors"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/api/apimodel"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
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
	ErrFailedCreateObject        = errors.New("failed to create object")
	ErrFailedSetPropertyFeatured = errors.New("failed to set property featured")
	ErrFailedCreateBookmark      = errors.New("failed to fetch bookmark")
	ErrFailedCreateBlock         = errors.New("failed to create block")
	ErrFailedPasteBody           = errors.New("failed to paste body")
	ErrFailedUpdateObject        = errors.New("failed to update object")
	ErrFailedDeleteObject        = errors.New("failed to delete object")
)

// ListObjects retrieves a paginated list of objects in a specific space.
func (s *Service) ListObjects(ctx context.Context, spaceId string, offset int, limit int) (objects []apimodel.Object, total int, hasMore bool, err error) {
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
	objects = make([]apimodel.Object, 0, len(paginatedObjects))

	// pre-fetch properties, types and tags to fill the objects
	propertyMap, err := s.GetPropertyMapFromStore(spaceId)
	if err != nil {
		return nil, 0, false, err
	}
	typeMap, err := s.GetTypeMapFromStore(spaceId, propertyMap)
	if err != nil {
		return nil, 0, false, err
	}
	tagMap, err := s.GetTagMapFromStore(spaceId)
	if err != nil {
		return nil, 0, false, err
	}

	for _, record := range paginatedObjects {
		objects = append(objects, s.GetObjectFromStruct(record, propertyMap, typeMap, tagMap))
	}
	return objects, total, hasMore, nil
}

// GetObject retrieves a single object by its ID in a specific space.
func (s *Service) GetObject(ctx context.Context, spaceId string, objectId string) (apimodel.ObjectWithBlocks, error) {
	resp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: objectId,
	})

	if resp.Error != nil {
		if resp.Error.Code == pb.RpcObjectShowResponseError_NOT_FOUND {
			return apimodel.ObjectWithBlocks{}, ErrObjectNotFound
		}

		if resp.Error.Code == pb.RpcObjectShowResponseError_OBJECT_DELETED {
			return apimodel.ObjectWithBlocks{}, ErrObjectDeleted
		}

		if resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			return apimodel.ObjectWithBlocks{}, ErrFailedRetrieveObject
		}
	}

	propertyMap, err := s.GetPropertyMapFromStore(spaceId)
	if err != nil {
		return apimodel.ObjectWithBlocks{}, err
	}
	typeMap, err := s.GetTypeMapFromStore(spaceId, propertyMap)
	if err != nil {
		return apimodel.ObjectWithBlocks{}, err
	}
	tagMap, err := s.GetTagMapFromStore(spaceId)
	if err != nil {
		return apimodel.ObjectWithBlocks{}, err
	}

	return s.GetObjectWithBlocksFromStruct(resp.ObjectView.Details[0].Details, resp.ObjectView.Blocks, propertyMap, typeMap, tagMap), nil
}

// CreateObject creates a new object in a specific space.
func (s *Service) CreateObject(ctx context.Context, spaceId string, request apimodel.CreateObjectRequest) (apimodel.ObjectWithBlocks, error) {
	details, err := s.buildObjectDetails(ctx, spaceId, request)
	if err != nil {
		return apimodel.ObjectWithBlocks{}, err
	}

	var objectId string
	if request.TypeKey == "ot-bookmark" {
		resp := s.mw.ObjectCreateBookmark(ctx, &pb.RpcObjectCreateBookmarkRequest{
			Details:    details,
			SpaceId:    spaceId,
			TemplateId: request.TemplateId,
		})

		if resp.Error.Code != pb.RpcObjectCreateBookmarkResponseError_NULL {
			return apimodel.ObjectWithBlocks{}, ErrFailedCreateBookmark
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
			return apimodel.ObjectWithBlocks{}, ErrFailedCreateObject
		}
		objectId = resp.ObjectId
	}

	// ObjectRelationAddFeatured if description was set
	if details.Fields[bundle.RelationKeyDescription.String()] != nil {
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

// UpdateObject updates an existing object in a specific space.
func (s *Service) UpdateObject(ctx context.Context, spaceId string, objectId string, request apimodel.UpdateObjectRequest) (apimodel.ObjectWithBlocks, error) {
	details, err := s.buildUpdatedObjectDetails(ctx, spaceId, request)
	if err != nil {
		return apimodel.ObjectWithBlocks{}, err
	}

	detailList := make([]*model.Detail, 0, len(details.Fields))
	for k, v := range details.Fields {
		detailList = append(detailList, &model.Detail{
			Key:   k,
			Value: v,
		})
	}

	resp := s.mw.ObjectSetDetails(ctx, &pb.RpcObjectSetDetailsRequest{
		ContextId: objectId,
		Details:   detailList,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetDetailsResponseError_NULL {
		return apimodel.ObjectWithBlocks{}, ErrFailedUpdateObject
	}

	return s.GetObject(ctx, spaceId, objectId)
}

// DeleteObject deletes an existing object in a specific space.
func (s *Service) DeleteObject(ctx context.Context, spaceId string, objectId string) (apimodel.ObjectWithBlocks, error) {
	object, err := s.GetObject(ctx, spaceId, objectId)
	if err != nil {
		return apimodel.ObjectWithBlocks{}, err
	}

	resp := s.mw.ObjectSetIsArchived(ctx, &pb.RpcObjectSetIsArchivedRequest{
		ContextId:  objectId,
		IsArchived: true,
	})

	if resp.Error.Code != pb.RpcObjectSetIsArchivedResponseError_NULL {
		return apimodel.ObjectWithBlocks{}, ErrFailedDeleteObject
	}

	return object, nil
}

// buildObjectDetails extracts the details structure from the CreateObjectRequest.
func (s *Service) buildObjectDetails(ctx context.Context, spaceId string, request apimodel.CreateObjectRequest) (*types.Struct, error) {
	if request.TypeKey == "ot-bookmark" && request.Source == "" {
		return nil, util.ErrBadInput("source is missing for bookmark")
	}

	fields := map[string]*types.Value{
		bundle.RelationKeyName.String():   pbtypes.String(s.sanitizedString(request.Name)),
		bundle.RelationKeySource.String(): pbtypes.String(s.sanitizedString(request.Source)),
		bundle.RelationKeyOrigin.String(): pbtypes.Int64(int64(model.ObjectOrigin_api)),
	}

	iconFields, err := s.processIconFields(ctx, spaceId, request.Icon)
	if err != nil {
		return nil, err
	}
	for k, v := range iconFields {
		fields[k] = v
	}

	propFields, err := s.processProperties(ctx, spaceId, request.Properties)
	if err != nil {
		return nil, err
	}
	for k, v := range propFields {
		fields[k] = v
	}

	return &types.Struct{Fields: fields}, nil
}

// buildUpdatedObjectDetails extracts the details structure from the UpdateObjectRequest.
func (s *Service) buildUpdatedObjectDetails(ctx context.Context, spaceId string, request apimodel.UpdateObjectRequest) (*types.Struct, error) {
	fields := make(map[string]*types.Value)
	if request.Name != "" {
		fields[bundle.RelationKeyName.String()] = pbtypes.String(s.sanitizedString(request.Name))
	}

	iconFields, err := s.processIconFields(ctx, spaceId, request.Icon)
	if err != nil {
		return nil, err
	}
	for k, v := range iconFields {
		fields[k] = v
	}

	propFields, err := s.processProperties(ctx, spaceId, request.Properties)
	if err != nil {
		return nil, err
	}
	for k, v := range propFields {
		fields[k] = v
	}

	return &types.Struct{Fields: fields}, nil
}

// processIconFields returns the detail fields corresponding to the given icon.
func (s *Service) processIconFields(ctx context.Context, spaceId string, icon apimodel.Icon) (map[string]*types.Value, error) {
	if icon.Name != nil || icon.Color != nil {
		return nil, util.ErrBadInput("icon name and color are not supported for object")
	}
	iconFields := make(map[string]*types.Value)
	if icon.Emoji != nil {
		if len(*icon.Emoji) > 0 && !apimodel.IsEmoji(*icon.Emoji) {
			return nil, util.ErrBadInput("icon emoji is not valid")
		}
		iconFields[bundle.RelationKeyIconEmoji.String()] = pbtypes.String(*icon.Emoji)
	} else if icon.File != nil {
		if !s.isValidFileReference(ctx, spaceId, s.sanitizedString(*icon.File)) {
			return nil, util.ErrBadInput("icon file is not valid")
		}
		iconFields[bundle.RelationKeyIconImage.String()] = pbtypes.String(*icon.File)
	}
	return iconFields, nil
}

// getBlocksFromDetails returns the list of blocks from the ObjectShowResponse.
func (s *Service) getBlocksFromDetails(blocks []*model.Block) []apimodel.Block {
	b := make([]apimodel.Block, 0, len(blocks))

	for _, block := range blocks {
		var text *apimodel.Text
		var file *apimodel.File
		var property *apimodel.Property

		switch content := block.Content.(type) {
		case *model.BlockContentOfText:
			text = &apimodel.Text{
				Object:  "text",
				Text:    content.Text.Text,
				Style:   model.BlockContentTextStyle_name[int32(content.Text.Style)],
				Checked: content.Text.Checked,
				Color:   content.Text.Color,
				Icon:    apimodel.GetIcon(s.gatewayUrl, content.Text.IconEmoji, content.Text.IconImage, "", 0),
			}
		case *model.BlockContentOfFile:
			file = &apimodel.File{
				Object:         "file",
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
			property = &apimodel.Property{
				// TODO: is it sufficient to return the key only?
				Object: "property",
				Key:    content.Relation.Key,
			}
		}
		// TODO: other content types?

		b = append(b, apimodel.Block{
			Object:          "block",
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

	return b
}

// GetObjectFromStruct creates an Object without blocks from the details.
func (s *Service) GetObjectFromStruct(details *types.Struct, propertyMap map[string]apimodel.Property, typeMap map[string]apimodel.Type, tagMap map[string]apimodel.Tag) apimodel.Object {
	return apimodel.Object{
		Object:     "object",
		Id:         details.Fields[bundle.RelationKeyId.String()].GetStringValue(),
		Name:       details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:       apimodel.GetIcon(s.gatewayUrl, details.GetFields()[bundle.RelationKeyIconEmoji.String()].GetStringValue(), details.GetFields()[bundle.RelationKeyIconImage.String()].GetStringValue(), details.GetFields()[bundle.RelationKeyIconName.String()].GetStringValue(), details.GetFields()[bundle.RelationKeyIconOption.String()].GetNumberValue()),
		Archived:   details.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
		SpaceId:    details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(),
		Snippet:    details.Fields[bundle.RelationKeySnippet.String()].GetStringValue(),
		Layout:     s.otLayoutToObjectLayout(model.ObjectTypeLayout(details.Fields[bundle.RelationKeyResolvedLayout.String()].GetNumberValue())),
		Type:       s.getTypeFromStruct(details, typeMap),
		Properties: s.getPropertiesFromStruct(details, propertyMap, tagMap),
	}
}

// GetObjectWithBlocksFromStruct creates an ObjectWithBlocks from the details.
func (s *Service) GetObjectWithBlocksFromStruct(details *types.Struct, blocks []*model.Block, propertyMap map[string]apimodel.Property, typeMap map[string]apimodel.Type, tagMap map[string]apimodel.Tag) apimodel.ObjectWithBlocks {
	return apimodel.ObjectWithBlocks{
		Object:     "object",
		Id:         details.Fields[bundle.RelationKeyId.String()].GetStringValue(),
		Name:       details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:       apimodel.GetIcon(s.gatewayUrl, details.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), details.Fields[bundle.RelationKeyIconImage.String()].GetStringValue(), details.Fields[bundle.RelationKeyIconName.String()].GetStringValue(), details.Fields[bundle.RelationKeyIconOption.String()].GetNumberValue()),
		Archived:   details.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
		SpaceId:    details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(),
		Snippet:    details.Fields[bundle.RelationKeySnippet.String()].GetStringValue(),
		Layout:     s.otLayoutToObjectLayout(model.ObjectTypeLayout(details.Fields[bundle.RelationKeyResolvedLayout.String()].GetNumberValue())),
		Type:       s.getTypeFromStruct(details, typeMap),
		Properties: s.getPropertiesFromStruct(details, propertyMap, tagMap),
		Blocks:     s.getBlocksFromDetails(blocks),
	}
}

// isMissingObject returns true if val indicates a "_missing_object" placeholder.
func (s *Service) isMissingObject(val interface{}) bool {
	switch v := val.(type) {
	case string:
		return v == "_missing_object"
	case []interface{}:
		if len(v) == 1 {
			if str, ok := v[0].(string); ok {
				return str == "_missing_object"
			}
		}
	case apimodel.Tag:
		return v.Id == "_missing_object"
	case []apimodel.Tag:
		if len(v) == 1 && v[0].Id == "_missing_object" {
			return true
		}
	}
	return false
}
