package object

import (
	"context"
	"errors"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ErrObjectNotFound             = errors.New("object not found")
	ErrFailedRetrieveObject       = errors.New("failed to retrieve object")
	ErrorFailedRetrieveObjects    = errors.New("failed to retrieve list of objects")
	ErrNoObjectsFound             = errors.New("no objects found")
	ErrFailedDeleteObject         = errors.New("failed to delete object")
	ErrFailedCreateObject         = errors.New("failed to create object")
	ErrInputMissingSource         = errors.New("source is missing for bookmark")
	ErrFailedSetRelationFeatured  = errors.New("failed to set relation featured")
	ErrFailedFetchBookmark        = errors.New("failed to fetch bookmark")
	ErrFailedPasteBody            = errors.New("failed to paste body")
	ErrNotImplemented             = errors.New("not implemented")
	ErrFailedUpdateObject         = errors.New("failed to update object")
	ErrFailedRetrieveTypes        = errors.New("failed to retrieve types")
	ErrNoTypesFound               = errors.New("no types found")
	ErrFailedRetrieveTemplateType = errors.New("failed to retrieve template type")
	ErrTemplateTypeNotFound       = errors.New("template type not found")
	ErrFailedRetrieveTemplate     = errors.New("failed to retrieve template")
	ErrFailedRetrieveTemplates    = errors.New("failed to retrieve templates")
	ErrNoTemplatesFound           = errors.New("no templates found")
)

type Service interface {
	ListObjects(ctx context.Context, spaceId string, offset int, limit int) ([]Object, int, bool, error)
	GetObject(ctx context.Context, spaceId string, objectId string) (Object, error)
	DeleteObject(ctx context.Context, spaceId string, objectId string) error
	CreateObject(ctx context.Context, spaceId string, request CreateObjectRequest) (Object, error)
	UpdateObject(ctx context.Context, spaceId string, objectId string, request UpdateObjectRequest) (Object, error)
	ListTypes(ctx context.Context, spaceId string, offset int, limit int) ([]ObjectType, int, bool, error)
	ListTemplates(ctx context.Context, spaceId string, typeId string, offset int, limit int) ([]ObjectTemplate, int, bool, error)
}

type ObjectService struct {
	mw          service.ClientCommandsServer
	AccountInfo *model.AccountInfo
}

func NewService(mw service.ClientCommandsServer) *ObjectService {
	return &ObjectService{mw: mw}
}

// ListObjects retrieves a paginated list of objects in a specific space.
func (s *ObjectService) ListObjects(ctx context.Context, spaceId string, offset int, limit int) (objects []Object, total int, hasMore bool, err error) {
	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
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
		},
		Sorts: []*model.BlockContentDataviewSort{{
			RelationKey:    bundle.RelationKeyLastModifiedDate.String(),
			Type:           model.BlockContentDataviewSort_Desc,
			Format:         model.RelationFormat_longtext,
			IncludeTime:    true,
			EmptyPlacement: model.BlockContentDataviewSort_NotSpecified,
		}},
		FullText:         "",
		Offset:           0,
		Limit:            0,
		ObjectTypeFilter: []string{},
		Keys:             []string{"id", "name"},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrorFailedRetrieveObjects
	}

	if len(resp.Records) == 0 {
		return nil, 0, false, ErrNoObjectsFound
	}

	total = len(resp.Records)
	paginatedObjects, hasMore := pagination.Paginate(resp.Records, offset, limit)
	objects = make([]Object, 0, len(paginatedObjects))

	for _, record := range paginatedObjects {
		object, err := s.GetObject(ctx, spaceId, record.Fields["id"].GetStringValue())
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

	if resp.Error.Code == pb.RpcObjectShowResponseError_NOT_FOUND {
		return Object{}, ErrObjectNotFound
	}

	if resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
		return Object{}, ErrFailedRetrieveObject
	}

	icon := util.GetIconFromEmojiOrImage(s.AccountInfo, resp.ObjectView.Details[0].Details.Fields["iconEmoji"].GetStringValue(), resp.ObjectView.Details[0].Details.Fields["iconImage"].GetStringValue())
	objectTypeName, err := util.ResolveTypeToName(s.mw, spaceId, resp.ObjectView.Details[0].Details.Fields["type"].GetStringValue())
	if err != nil {
		return Object{}, err
	}

	object := Object{
		Type:       "object",
		Id:         resp.ObjectView.Details[0].Details.Fields["id"].GetStringValue(),
		Name:       resp.ObjectView.Details[0].Details.Fields["name"].GetStringValue(),
		Icon:       icon,
		Layout:     model.ObjectTypeLayout_name[int32(resp.ObjectView.Details[0].Details.Fields["layout"].GetNumberValue())],
		ObjectType: objectTypeName,
		SpaceId:    resp.ObjectView.Details[0].Details.Fields["spaceId"].GetStringValue(),
		RootId:     resp.ObjectView.RootId,
		Blocks:     s.GetBlocks(resp),
		Details:    s.GetDetails(resp),
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
	if request.ObjectTypeUniqueKey == "ot-bookmark" && request.Source == "" {
		return Object{}, ErrInputMissingSource
	}

	details := &types.Struct{
		Fields: map[string]*types.Value{
			"name":        pbtypes.String(request.Name),
			"iconEmoji":   pbtypes.String(request.Icon),
			"description": pbtypes.String(request.Description),
			"source":      pbtypes.String(request.Source),
		},
	}

	resp := s.mw.ObjectCreate(ctx, &pb.RpcObjectCreateRequest{
		Details:             details,
		TemplateId:          request.TemplateId,
		SpaceId:             spaceId,
		ObjectTypeUniqueKey: request.ObjectTypeUniqueKey,
		WithChat:            request.WithChat,
	})

	if resp.Error.Code != pb.RpcObjectCreateResponseError_NULL {
		return Object{}, ErrFailedCreateObject
	}

	// ObjectRelationAddFeatured if description was set
	if request.Description != "" {
		relAddFeatResp := s.mw.ObjectRelationAddFeatured(ctx, &pb.RpcObjectRelationAddFeaturedRequest{
			ContextId: resp.ObjectId,
			Relations: []string{"description"},
		})

		if relAddFeatResp.Error.Code != pb.RpcObjectRelationAddFeaturedResponseError_NULL {
			object, _ := s.GetObject(ctx, spaceId, resp.ObjectId)
			return object, ErrFailedSetRelationFeatured
		}
	}

	// ObjectBookmarkFetch after creating a bookmark object
	if request.ObjectTypeUniqueKey == "ot-bookmark" {
		bookmarkResp := s.mw.ObjectBookmarkFetch(ctx, &pb.RpcObjectBookmarkFetchRequest{
			ContextId: resp.ObjectId,
			Url:       request.Source,
		})

		if bookmarkResp.Error.Code != pb.RpcObjectBookmarkFetchResponseError_NULL {
			object, _ := s.GetObject(ctx, spaceId, resp.ObjectId)
			return object, ErrFailedFetchBookmark
		}
	}

	// First call BlockCreate at top, then BlockPaste to paste the body
	if request.Body != "" {
		blockCreateResp := s.mw.BlockCreate(ctx, &pb.RpcBlockCreateRequest{
			ContextId: resp.ObjectId,
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
			object, _ := s.GetObject(ctx, spaceId, resp.ObjectId)
			return object, ErrFailedCreateObject
		}

		blockPasteResp := s.mw.BlockPaste(ctx, &pb.RpcBlockPasteRequest{
			ContextId:      resp.ObjectId,
			FocusedBlockId: blockCreateResp.BlockId,
			TextSlot:       request.Body,
		})

		if blockPasteResp.Error.Code != pb.RpcBlockPasteResponseError_NULL {
			object, _ := s.GetObject(ctx, spaceId, resp.ObjectId)
			return object, ErrFailedPasteBody
		}
	}

	return s.GetObject(ctx, spaceId, resp.ObjectId)
}

// UpdateObject updates an existing object in a specific space.
func (s *ObjectService) UpdateObject(ctx context.Context, spaceId string, objectId string, request UpdateObjectRequest) (Object, error) {
	// TODO: Implement logic to update an existing object
	return Object{}, ErrNotImplemented
}

// ListTypes returns a paginated list of types in a specific space.
func (s *ObjectService) ListTypes(ctx context.Context, spaceId string, offset int, limit int) (types []ObjectType, total int, hasMore bool, err error) {
	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
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
				RelationKey: "name",
				Type:        model.BlockContentDataviewSort_Asc,
			},
		},
		Keys: []string{"id", "uniqueKey", "name", "iconEmoji", "recommendedLayout"},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedRetrieveTypes
	}

	if len(resp.Records) == 0 {
		return nil, 0, false, ErrNoTypesFound
	}

	total = len(resp.Records)
	paginatedTypes, hasMore := pagination.Paginate(resp.Records, offset, limit)
	objectTypes := make([]ObjectType, 0, len(paginatedTypes))

	for _, record := range paginatedTypes {
		objectTypes = append(objectTypes, ObjectType{
			Type:              "object_type",
			Id:                record.Fields["id"].GetStringValue(),
			UniqueKey:         record.Fields["uniqueKey"].GetStringValue(),
			Name:              record.Fields["name"].GetStringValue(),
			Icon:              record.Fields["iconEmoji"].GetStringValue(),
			RecommendedLayout: model.ObjectTypeLayout_name[int32(record.Fields["recommendedLayout"].GetNumberValue())],
		})
	}
	return objectTypes, total, hasMore, nil
}

// ListTemplates returns a paginated list of templates in a specific space.
func (s *ObjectService) ListTemplates(ctx context.Context, spaceId string, typeId string, offset int, limit int) (templates []ObjectTemplate, total int, hasMore bool, err error) {
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
		Keys: []string{"id"},
	})

	if templateTypeIdResp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedRetrieveTemplateType
	}

	if len(templateTypeIdResp.Records) == 0 {
		return nil, 0, false, ErrTemplateTypeNotFound
	}

	// Then, search all objects of the template type and filter by the target object type
	templateTypeId := templateTypeIdResp.Records[0].Fields["id"].GetStringValue()
	templateObjectsResp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(templateTypeId),
			},
		},
		Keys: []string{"id", "targetObjectType", "name", "iconEmoji"},
	})

	if templateObjectsResp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedRetrieveTemplates
	}

	if len(templateObjectsResp.Records) == 0 {
		return nil, 0, false, ErrNoTemplatesFound
	}

	templateIds := make([]string, 0)
	for _, record := range templateObjectsResp.Records {
		if record.Fields["targetObjectType"].GetStringValue() == typeId {
			templateIds = append(templateIds, record.Fields["id"].GetStringValue())
		}
	}

	total = len(templateIds)
	paginatedTemplates, hasMore := pagination.Paginate(templateIds, offset, limit)
	templates = make([]ObjectTemplate, 0, len(paginatedTemplates))

	// Finally, open each template and populate the response
	for _, templateId := range paginatedTemplates {
		templateResp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
			SpaceId:  spaceId,
			ObjectId: templateId,
		})

		if templateResp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			return nil, 0, false, ErrFailedRetrieveTemplate
		}

		templates = append(templates, ObjectTemplate{
			Type: "object_template",
			Id:   templateId,
			Name: templateResp.ObjectView.Details[0].Details.Fields["name"].GetStringValue(),
			Icon: templateResp.ObjectView.Details[0].Details.Fields["iconEmoji"].GetStringValue(),
		})
	}

	return templates, total, hasMore, nil
}

// GetDetails returns the list of details from the ObjectShowResponse.
func (s *ObjectService) GetDetails(resp *pb.RpcObjectShowResponse) []Detail {
	return []Detail{
		{
			Id: "lastModifiedDate",
			Details: map[string]interface{}{
				"lastModifiedDate": resp.ObjectView.Details[0].Details.Fields["lastModifiedDate"].GetNumberValue(),
			},
		},
		{
			Id: "createdDate",
			Details: map[string]interface{}{
				"createdDate": resp.ObjectView.Details[0].Details.Fields["createdDate"].GetNumberValue(),
			},
		},
		{
			Id: "tags",
			Details: map[string]interface{}{
				"tags": s.getTags(resp),
			},
		},
	}
}

// getTags returns the list of tags from the ObjectShowResponse
func (s *ObjectService) getTags(resp *pb.RpcObjectShowResponse) []Tag {
	tags := []Tag{}

	tagField, ok := resp.ObjectView.Details[0].Details.Fields["tag"]
	if !ok {
		return tags
	}

	for _, tagId := range tagField.GetListValue().Values {
		id := tagId.GetStringValue()
		for _, detail := range resp.ObjectView.Details {
			if detail.Id == id {
				tags = append(tags, Tag{
					Id:    id,
					Name:  detail.Details.Fields["name"].GetStringValue(),
					Color: detail.Details.Fields["relationOptionColor"].GetStringValue(),
				})
				break
			}
		}
	}
	return tags
}

// GetBlocks returns the list of blocks from the ObjectShowResponse.
func (s *ObjectService) GetBlocks(resp *pb.RpcObjectShowResponse) []Block {
	blocks := []Block{}

	for _, block := range resp.ObjectView.Blocks {
		var text *Text
		var file *File

		switch content := block.Content.(type) {
		case *model.BlockContentOfText:
			text = &Text{
				Text:    content.Text.Text,
				Style:   model.BlockContentTextStyle_name[int32(content.Text.Style)],
				Checked: content.Text.Checked,
				Color:   content.Text.Color,
				Icon:    util.GetIconFromEmojiOrImage(s.AccountInfo, content.Text.IconEmoji, content.Text.IconImage),
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
			// TODO: other content types?
		}

		blocks = append(blocks, Block{
			Id:              block.Id,
			ChildrenIds:     block.ChildrenIds,
			BackgroundColor: block.BackgroundColor,
			Align:           mapAlign(block.Align),
			VerticalAlign:   mapVerticalAlign(block.VerticalAlign),
			Text:            text,
			File:            file,
		})
	}

	return blocks
}

// mapAlign maps the protobuf BlockAlign to a string.
func mapAlign(align model.BlockAlign) string {
	switch align {
	case model.Block_AlignLeft:
		return "left"
	case model.Block_AlignCenter:
		return "center"
	case model.Block_AlignRight:
		return "right"
	case model.Block_AlignJustify:
		return "justify"
	default:
		return "unknown"
	}
}

// mapVerticalAlign maps the protobuf BlockVerticalAlign to a string.
func mapVerticalAlign(align model.BlockVerticalAlign) string {
	switch align {
	case model.Block_VerticalAlignTop:
		return "top"
	case model.Block_VerticalAlignMiddle:
		return "middle"
	case model.Block_VerticalAlignBottom:
		return "bottom"
	default:
		return "unknown"
	}
}
