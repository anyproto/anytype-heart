package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pb"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ErrObjectNotFound            = errors.New("object not found")
	ErrObjectDeleted             = errors.New("object deleted")
	ErrFailedRetrieveObject      = errors.New("failed to retrieve object")
	ErrFailedExportMarkdown      = errors.New("failed to export markdown")
	ErrFailedRetrieveObjects     = errors.New("failed to retrieve list of objects")
	ErrFailedCreateObject        = errors.New("failed to create object")
	ErrFailedSetPropertyFeatured = errors.New("failed to set property featured")
	ErrFailedCreateBookmark      = errors.New("failed to fetch bookmark")
	ErrFailedCreateBlock         = errors.New("failed to create block")
	ErrFailedPasteBody           = errors.New("failed to paste body")
	ErrFailedUpdateObject        = errors.New("failed to update object")
	ErrFailedReplaceBlocks       = errors.New("failed to replace blocks")
	ErrFailedDeleteObject        = errors.New("failed to delete object")
)

// ListObjects retrieves a paginated list of objects in a specific space.
func (s *Service) ListObjects(ctx context.Context, spaceId string, additionalFilters []*model.BlockContentDataviewFilter, offset int, limit int) (objects []apimodel.Object, total int, hasMore bool, err error) {
	filters := append([]*model.BlockContentDataviewFilter{
		{
			RelationKey: bundle.RelationKeyResolvedLayout.String(),
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       pbtypes.IntList(util.LayoutsToIntArgs(util.ObjectLayouts)...),
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
	}, additionalFilters...)

	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: filters,
		Sorts: []*model.BlockContentDataviewSort{{
			RelationKey: bundle.RelationKeyLastModifiedDate.String(),
			Type:        model.BlockContentDataviewSort_Desc,
			IncludeTime: true,
		}},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedRetrieveObjects
	}

	total = len(resp.Records)
	paginatedObjects, hasMore := pagination.Paginate(resp.Records, offset, limit)
	objects = make([]apimodel.Object, 0, len(paginatedObjects))

	for _, record := range paginatedObjects {
		objects = append(objects, s.getObjectFromStruct(record))
	}
	return objects, total, hasMore, nil
}

// GetObject retrieves a single object by its ID in a specific space.
func (s *Service) GetObject(ctx context.Context, spaceId string, objectId string) (*apimodel.ObjectWithBody, error) {
	resp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: objectId,
	})

	if resp.Error != nil {
		if resp.Error.Code == pb.RpcObjectShowResponseError_NOT_FOUND {
			return nil, ErrObjectNotFound
		}

		if resp.Error.Code == pb.RpcObjectShowResponseError_OBJECT_DELETED {
			return nil, ErrObjectDeleted
		}

		if resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			return nil, ErrFailedRetrieveObject
		}
	}

	layout := model.ObjectTypeLayout(resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyResolvedLayout.String()].GetNumberValue())
	markdown, err := s.getMarkdownExport(ctx, spaceId, objectId, layout)
	if err != nil {
		return nil, err
	}

	return s.getObjectWithBlocksFromStruct(resp.ObjectView.Details[0].Details, markdown), nil
}

// CreateObject creates a new object in a specific space.
func (s *Service) CreateObject(ctx context.Context, spaceId string, request apimodel.CreateObjectRequest) (*apimodel.ObjectWithBody, error) {
	details, err := s.buildObjectDetails(ctx, spaceId, request)
	if err != nil {
		return nil, err
	}

	typeUk := s.ResolveTypeApiKey(spaceId, request.TypeKey)

	var objectId string
	if typeUk == "ot-bookmark" {
		resp := s.mw.ObjectCreateBookmark(ctx, &pb.RpcObjectCreateBookmarkRequest{
			Details:    details,
			SpaceId:    spaceId,
			TemplateId: request.TemplateId,
		})

		if resp.Error != nil && resp.Error.Code != pb.RpcObjectCreateBookmarkResponseError_NULL {
			return nil, ErrFailedCreateBookmark
		}
		objectId = resp.ObjectId
	} else {
		resp := s.mw.ObjectCreate(ctx, &pb.RpcObjectCreateRequest{
			Details:             details,
			TemplateId:          request.TemplateId,
			SpaceId:             spaceId,
			ObjectTypeUniqueKey: typeUk,
		})

		if resp.Error != nil && resp.Error.Code != pb.RpcObjectCreateResponseError_NULL {
			return nil, ErrFailedCreateObject
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
		if err := s.createAndPasteBody(ctx, spaceId, objectId, request.Body); err != nil {
			object, _ := s.GetObject(ctx, spaceId, objectId) // nolint:errcheck
			return object, err
		}
	}

	return s.GetObject(ctx, spaceId, objectId)
}

// UpdateObject updates an existing object in a specific space.
func (s *Service) UpdateObject(ctx context.Context, spaceId string, objectId string, request apimodel.UpdateObjectRequest) (*apimodel.ObjectWithBody, error) {
	_, err := s.GetObject(ctx, spaceId, objectId)
	if err != nil {
		return nil, err
	}

	if request.TypeKey != nil {
		typeUk := s.ResolveTypeApiKey(spaceId, *request.TypeKey)
		typeResp := s.mw.ObjectSetObjectType(ctx, &pb.RpcObjectSetObjectTypeRequest{
			ContextId:           objectId,
			ObjectTypeUniqueKey: typeUk,
		})
		if typeResp.Error != nil && typeResp.Error.Code != pb.RpcObjectSetObjectTypeResponseError_NULL {
			return nil, util.ErrBadInput(fmt.Sprintf("failed to update object, invalid type key: %q", *request.TypeKey))
		}
	}

	details, err := s.buildUpdatedObjectDetails(ctx, spaceId, request)
	if err != nil {
		return nil, err
	}

	resp := s.mw.ObjectSetDetails(ctx, &pb.RpcObjectSetDetailsRequest{
		ContextId: objectId,
		Details:   structToDetails(details),
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetDetailsResponseError_NULL {
		return nil, ErrFailedUpdateObject
	}

	if request.Markdown != nil {
		showResp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
			SpaceId:  spaceId,
			ObjectId: objectId,
		})

		if showResp.Error != nil && showResp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			return nil, ErrFailedRetrieveObject
		}

		allRootChildrenExceptHeader := make([]string, 0, len(showResp.ObjectView.Blocks[0].ChildrenIds))
		for _, childId := range showResp.ObjectView.Blocks[0].ChildrenIds {
			if childId != "header" {
				allRootChildrenExceptHeader = append(allRootChildrenExceptHeader, childId)
			}
		}

		if len(allRootChildrenExceptHeader) > 0 {
			deleteResp := s.mw.BlockListDelete(ctx, &pb.RpcBlockListDeleteRequest{
				ContextId: objectId,
				BlockIds:  allRootChildrenExceptHeader,
			})

			if deleteResp.Error != nil && deleteResp.Error.Code != pb.RpcBlockListDeleteResponseError_NULL {
				return nil, ErrFailedReplaceBlocks
			}
		}

		if *request.Markdown != "" {
			if err := s.createAndPasteBody(ctx, spaceId, objectId, *request.Markdown); err != nil {
				object, _ := s.GetObject(ctx, spaceId, objectId) // nolint:errcheck
				return object, err
			}
		}
	}

	return s.GetObject(ctx, spaceId, objectId)
}

// DeleteObject deletes an existing object in a specific space.
func (s *Service) DeleteObject(ctx context.Context, spaceId string, objectId string) (*apimodel.ObjectWithBody, error) {
	object, err := s.GetObject(ctx, spaceId, objectId)
	if err != nil {
		return nil, err
	}

	resp := s.mw.ObjectSetIsArchived(ctx, &pb.RpcObjectSetIsArchivedRequest{
		ContextId:  objectId,
		IsArchived: true,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetIsArchivedResponseError_NULL {
		return nil, ErrFailedDeleteObject
	}

	return object, nil
}

// buildObjectDetails extracts the details structure from the CreateObjectRequest.
func (s *Service) buildObjectDetails(ctx context.Context, spaceId string, request apimodel.CreateObjectRequest) (*types.Struct, error) {
	fields := map[string]*types.Value{
		bundle.RelationKeyName.String():   pbtypes.String(s.sanitizedString(request.Name)),
		bundle.RelationKeyOrigin.String(): pbtypes.Int64(int64(model.ObjectOrigin_api)),
	}

	iconFields, err := s.processIconFields(spaceId, request.Icon, false)
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

// buildUpdatedObjectDetails build a partial details struct for UpdateObjectRequest.
func (s *Service) buildUpdatedObjectDetails(ctx context.Context, spaceId string, request apimodel.UpdateObjectRequest) (*types.Struct, error) {
	fields := make(map[string]*types.Value)
	if request.Name != nil {
		fields[bundle.RelationKeyName.String()] = pbtypes.String(s.sanitizedString(*request.Name))
	}

	if request.Icon != nil {
		iconFields, err := s.processIconFields(spaceId, *request.Icon, false)
		if err != nil {
			return nil, err
		}
		for k, v := range iconFields {
			fields[k] = v
		}
	}

	if request.Properties != nil {
		propFields, err := s.processProperties(ctx, spaceId, *request.Properties)
		if err != nil {
			return nil, err
		}
		for k, v := range propFields {
			fields[k] = v
		}
	}

	return &types.Struct{Fields: fields}, nil
}

// processIconFields returns the detail fields corresponding to the given icon.
func (s *Service) processIconFields(spaceId string, icon apimodel.Icon, isType bool) (map[string]*types.Value, error) {
	iconFields := make(map[string]*types.Value)
	switch e := icon.WrappedIcon.(type) {
	case apimodel.NamedIcon:
		if isType {
			iconFields[bundle.RelationKeyIconName.String()] = pbtypes.String(string(e.Name))
			iconFields[bundle.RelationKeyIconOption.String()] = pbtypes.Int64(apimodel.ColorToIconOption[e.Color])
		} else {
			return nil, util.ErrBadInput("icon name and color are not supported for object")
		}
	case apimodel.EmojiIcon:
		if len(e.Emoji) > 0 && !isEmoji(e.Emoji) {
			return nil, util.ErrBadInput("icon emoji is not valid")
		}
		iconFields[bundle.RelationKeyIconEmoji.String()] = pbtypes.String(e.Emoji)
	case apimodel.FileIcon:
		fileId := s.sanitizedString(e.File)
		if !s.isValidFileReference(spaceId, fileId) {
			return nil, util.ErrBadInput("icon file is not valid")
		}
		iconFields[bundle.RelationKeyIconImage.String()] = pbtypes.String(fileId)
	}
	return iconFields, nil
}

// ! Deprecated method, until json blocks properly implemented
// getBlocksFromDetails returns the list of blocks from the ObjectShowResponse.
// func (s *Service) getBlocksFromDetails(blocks []*model.Block) []apimodel.Block {
// 	b := make([]apimodel.Block, 0, len(blocks))
//
// 	for _, block := range blocks {
// 		var text *apimodel.Text
// 		var file *apimodel.File
// 		var property *apimodel.Property
//
// 		switch content := block.Content.(type) {
// 		case *model.BlockContentOfText:
// 			text = &apimodel.Text{
// 				Object:  "text",
// 				Text:    content.Text.Text,
// 				Style:   model.BlockContentTextStyle_name[int32(content.Text.Style)],
// 				Checked: content.Text.Checked,
// 				Color:   content.Text.Color,
// 				Icon:    getIcon(s.gatewayUrl, content.Text.IconEmoji, content.Text.IconImage, "", 0),
// 			}
// 		case *model.BlockContentOfFile:
// 			file = &apimodel.File{
// 				Object:         "file",
// 				Hash:           content.File.Hash,
// 				Name:           content.File.Name,
// 				Type:           model.BlockContentFileType_name[int32(content.File.Type)],
// 				Mime:           content.File.Mime,
// 				Size:           content.File.Size(),
// 				AddedAt:        int(content.File.AddedAt),
// 				TargetObjectId: content.File.TargetObjectId,
// 				State:          model.BlockContentFileState_name[int32(content.File.State)],
// 				Style:          model.BlockContentFileStyle_name[int32(content.File.Style)],
// 			}
// 		case *model.BlockContentOfRelation:
// 			property = &apimodel.Property{
// 				// TODO: is it sufficient to return the key only?
// 				Object: "property",
// 				Key:    content.Relation.Key,
// 			}
// 		}
// 		// TODO: other content types?
//
// 		b = append(b, apimodel.Block{
// 			Object:          "block",
// 			Id:              block.Id,
// 			ChildrenIds:     block.ChildrenIds,
// 			BackgroundColor: block.BackgroundColor,
// 			Align:           model.BlockAlign_name[int32(block.Align)],
// 			VerticalAlign:   model.BlockVerticalAlign_name[int32(block.VerticalAlign)],
// 			Text:            text,
// 			File:            file,
// 			Property:        property,
// 		})
// 	}
//
// 	return b
// }

// getObjectFromStruct creates an Object without blocks from the details.
func (s *Service) getObjectFromStruct(details *types.Struct) apimodel.Object {
	spaceId := details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue()
	typeMap := s.cache.getTypes(spaceId)

	return apimodel.Object{
		Object:     "object",
		Id:         details.Fields[bundle.RelationKeyId.String()].GetStringValue(),
		Name:       details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:       getIcon(s.gatewayUrl, details.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), details.Fields[bundle.RelationKeyIconImage.String()].GetStringValue(), details.Fields[bundle.RelationKeyIconName.String()].GetStringValue(), details.Fields[bundle.RelationKeyIconOption.String()].GetNumberValue()),
		Archived:   details.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
		SpaceId:    spaceId,
		Snippet:    details.Fields[bundle.RelationKeySnippet.String()].GetStringValue(),
		Layout:     s.otLayoutToObjectLayout(model.ObjectTypeLayout(details.Fields[bundle.RelationKeyResolvedLayout.String()].GetNumberValue())),
		Type:       typeMap[details.Fields[bundle.RelationKeyType.String()].GetStringValue()],
		Properties: s.getPropertiesFromStruct(details),
	}
}

// getObjectWithBlocksFromStruct creates an ObjectWithBody from the details.
func (s *Service) getObjectWithBlocksFromStruct(details *types.Struct, markdown string) *apimodel.ObjectWithBody {
	spaceId := details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue()
	typeMap := s.cache.getTypes(spaceId)

	return &apimodel.ObjectWithBody{
		Object:     "object",
		Id:         details.Fields[bundle.RelationKeyId.String()].GetStringValue(),
		Name:       details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:       getIcon(s.gatewayUrl, details.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), details.Fields[bundle.RelationKeyIconImage.String()].GetStringValue(), details.Fields[bundle.RelationKeyIconName.String()].GetStringValue(), details.Fields[bundle.RelationKeyIconOption.String()].GetNumberValue()),
		Archived:   details.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
		SpaceId:    spaceId,
		Snippet:    details.Fields[bundle.RelationKeySnippet.String()].GetStringValue(),
		Layout:     s.otLayoutToObjectLayout(model.ObjectTypeLayout(details.Fields[bundle.RelationKeyResolvedLayout.String()].GetNumberValue())),
		Type:       typeMap[details.Fields[bundle.RelationKeyType.String()].GetStringValue()],
		Properties: s.getPropertiesFromStruct(details),
		Markdown:   markdown,
	}
}

// getMarkdownExport retrieves the Markdown export of an object.
func (s *Service) getMarkdownExport(ctx context.Context, spaceId string, objectId string, layout model.ObjectTypeLayout) (string, error) {
	if util.IsObjectLayout(layout) {
		resp := s.mw.ObjectExport(ctx, &pb.RpcObjectExportRequest{
			SpaceId:  spaceId,
			ObjectId: objectId,
			Format:   model.Export_Markdown,
		})

		if resp.Error != nil && resp.Error.Code != pb.RpcObjectExportResponseError_NULL {
			return "", ErrFailedExportMarkdown
		}

		markdown := resp.Result

		if len(markdown) > 0 && markdown[0] == '#' {
			for i := 0; i < len(markdown); i++ {
				if markdown[i] == '\n' {
					if i+1 < len(markdown) && markdown[i+1] == '\n' {
						markdown = markdown[i+2:]
					} else {
						markdown = markdown[i+1:]
					}
					break
				}
			}
		}

		return markdown, nil
	}
	return "", nil
}

// structToDetails converts a Struct to a list of Details.
func structToDetails(details *types.Struct) []*model.Detail {
	detailList := make([]*model.Detail, 0, len(details.Fields))
	for k, v := range details.Fields {
		detailList = append(detailList, &model.Detail{
			Key:   k,
			Value: v,
		})
	}
	return detailList
}

// createAndPasteBody creates a text block and pastes the body content into it.
func (s *Service) createAndPasteBody(ctx context.Context, spaceId string, objectId string, body string) error {
	blockCreateResp := s.mw.BlockCreate(ctx, &pb.RpcBlockCreateRequest{
		ContextId: objectId,
		TargetId:  "",
		Block: &model.Block{
			Id:            "",
			Align:         model.Block_AlignLeft,
			VerticalAlign: model.Block_VerticalAlignTop,
			Content:       &model.BlockContentOfText{Text: &model.BlockContentText{}},
		},
		Position: model.Block_Bottom,
	})

	if blockCreateResp.Error.Code != pb.RpcBlockCreateResponseError_NULL {
		return ErrFailedCreateBlock
	}

	blockPasteResp := s.mw.BlockPaste(ctx, &pb.RpcBlockPasteRequest{
		ContextId:      objectId,
		FocusedBlockId: blockCreateResp.BlockId,
		TextSlot:       body,
	})

	if blockPasteResp.Error.Code != pb.RpcBlockPasteResponseError_NULL {
		return ErrFailedPasteBody
	}

	return nil
}
