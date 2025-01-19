package object

import (
	"context"
	"errors"
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/services/space"
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
	ErrFailedRetrieveObjects      = errors.New("failed to retrieve list of objects")
	ErrFailedDeleteObject         = errors.New("failed to delete object")
	ErrFailedCreateObject         = errors.New("failed to create object")
	ErrInputMissingSource         = errors.New("source is missing for bookmark")
	ErrFailedSetRelationFeatured  = errors.New("failed to set relation featured")
	ErrFailedFetchBookmark        = errors.New("failed to fetch bookmark")
	ErrFailedPasteBody            = errors.New("failed to paste body")
	ErrFailedRetrieveTypes        = errors.New("failed to retrieve types")
	ErrFailedRetrieveTemplateType = errors.New("failed to retrieve template type")
	ErrTemplateTypeNotFound       = errors.New("template type not found")
	ErrFailedRetrieveTemplate     = errors.New("failed to retrieve template")
	ErrFailedRetrieveTemplates    = errors.New("failed to retrieve templates")
)

type Service interface {
	ListObjects(ctx context.Context, spaceId string, offset int, limit int) ([]Object, int, bool, error)
	GetObject(ctx context.Context, spaceId string, objectId string) (Object, error)
	DeleteObject(ctx context.Context, spaceId string, objectId string) (Object, error)
	CreateObject(ctx context.Context, spaceId string, request CreateObjectRequest) (Object, error)
	ListTypes(ctx context.Context, spaceId string, offset int, limit int) ([]Type, int, bool, error)
	ListTemplates(ctx context.Context, spaceId string, typeId string, offset int, limit int) ([]Template, int, bool, error)
}

type ObjectService struct {
	mw           service.ClientCommandsServer
	spaceService *space.SpaceService
	AccountInfo  *model.AccountInfo
}

func NewService(mw service.ClientCommandsServer, spaceService *space.SpaceService) *ObjectService {
	return &ObjectService{mw: mw, spaceService: spaceService}
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

	if resp.Error.Code == pb.RpcObjectShowResponseError_NOT_FOUND {
		return Object{}, ErrObjectNotFound
	}

	if resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
		return Object{}, ErrFailedRetrieveObject
	}

	icon := util.GetIconFromEmojiOrImage(s.AccountInfo, resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconImage.String()].GetStringValue())
	objectTypeName, err := util.ResolveTypeToName(s.mw, spaceId, resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyType.String()].GetStringValue())
	if err != nil {
		return Object{}, err
	}

	object := Object{
		Type:       "object",
		Id:         resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyId.String()].GetStringValue(),
		Name:       resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:       icon,
		Layout:     model.ObjectTypeLayout_name[int32(resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyLayout.String()].GetNumberValue())],
		ObjectType: objectTypeName,
		SpaceId:    resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(),
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
			bundle.RelationKeyName.String():        pbtypes.String(request.Name),
			bundle.RelationKeyIconEmoji.String():   pbtypes.String(request.Icon),
			bundle.RelationKeyDescription.String(): pbtypes.String(request.Description),
			bundle.RelationKeySource.String():      pbtypes.String(request.Source),
			bundle.RelationKeyOrigin.String():      pbtypes.Int64(int64(model.ObjectOrigin_api)),
		},
	}

	resp := s.mw.ObjectCreate(ctx, &pb.RpcObjectCreateRequest{
		Details:             details,
		TemplateId:          request.TemplateId,
		SpaceId:             spaceId,
		ObjectTypeUniqueKey: request.ObjectTypeUniqueKey,
		WithChat:            false,
	})

	if resp.Error.Code != pb.RpcObjectCreateResponseError_NULL {
		return Object{}, ErrFailedCreateObject
	}

	// ObjectRelationAddFeatured if description was set
	if request.Description != "" {
		relAddFeatResp := s.mw.ObjectRelationAddFeatured(ctx, &pb.RpcObjectRelationAddFeaturedRequest{
			ContextId: resp.ObjectId,
			Relations: []string{bundle.RelationKeyDescription.String()},
		})

		if relAddFeatResp.Error.Code != pb.RpcObjectRelationAddFeaturedResponseError_NULL {
			object, _ := s.GetObject(ctx, spaceId, resp.ObjectId) // nolint:errcheck
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
			object, _ := s.GetObject(ctx, spaceId, resp.ObjectId) // nolint:errcheck
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
			object, _ := s.GetObject(ctx, spaceId, resp.ObjectId) // nolint:errcheck
			return object, ErrFailedCreateObject
		}

		blockPasteResp := s.mw.BlockPaste(ctx, &pb.RpcBlockPasteRequest{
			ContextId:      resp.ObjectId,
			FocusedBlockId: blockCreateResp.BlockId,
			TextSlot:       request.Body,
		})

		if blockPasteResp.Error.Code != pb.RpcBlockPasteResponseError_NULL {
			object, _ := s.GetObject(ctx, spaceId, resp.ObjectId) // nolint:errcheck
			return object, ErrFailedPasteBody
		}
	}

	return s.GetObject(ctx, spaceId, resp.ObjectId)
}

// ListTypes returns a paginated list of types in a specific space.
func (s *ObjectService) ListTypes(ctx context.Context, spaceId string, offset int, limit int) (types []Type, total int, hasMore bool, err error) {
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
				RelationKey: bundle.RelationKeyName.String(),
				Type:        model.BlockContentDataviewSort_Asc,
			},
		},
		Keys: []string{bundle.RelationKeyId.String(), bundle.RelationKeyUniqueKey.String(), bundle.RelationKeyName.String(), bundle.RelationKeyIconEmoji.String(), bundle.RelationKeyRecommendedLayout.String()},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedRetrieveTypes
	}

	total = len(resp.Records)
	paginatedTypes, hasMore := pagination.Paginate(resp.Records, offset, limit)
	types = make([]Type, 0, len(paginatedTypes))

	for _, record := range paginatedTypes {
		types = append(types, Type{
			Type:              "type",
			Id:                record.Fields[bundle.RelationKeyId.String()].GetStringValue(),
			UniqueKey:         record.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(),
			Name:              record.Fields[bundle.RelationKeyName.String()].GetStringValue(),
			Icon:              record.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(),
			RecommendedLayout: model.ObjectTypeLayout_name[int32(record.Fields[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue())],
		})
	}
	return types, total, hasMore, nil
}

// ListTemplates returns a paginated list of templates in a specific space.
func (s *ObjectService) ListTemplates(ctx context.Context, spaceId string, typeId string, offset int, limit int) (templates []Template, total int, hasMore bool, err error) {
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
		},
		Keys: []string{bundle.RelationKeyId.String(), bundle.RelationKeyTargetObjectType.String(), bundle.RelationKeyName.String(), bundle.RelationKeyIconEmoji.String()},
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
			Type: "template",
			Id:   templateId,
			Name: templateResp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
			Icon: templateResp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(),
		})
	}

	return templates, total, hasMore, nil
}

// GetDetails returns the list of details from the ObjectShowResponse.
func (s *ObjectService) GetDetails(resp *pb.RpcObjectShowResponse) []Detail {
	creator := resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyCreator.String()].GetStringValue()
	lastModifiedBy := resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyLastModifiedBy.String()].GetStringValue()

	var creatorId, lastModifiedById string
	for _, detail := range resp.ObjectView.Details {
		if detail.Id == creator {
			creatorId = detail.Id
		}
		if detail.Id == lastModifiedBy {
			lastModifiedById = detail.Id
		}
	}

	return []Detail{
		{
			Id: "last_modified_date",
			Details: map[string]interface{}{
				"last_modified_date": PosixToISO8601(resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyLastModifiedDate.String()].GetNumberValue()),
			},
		},
		{
			Id: "last_modified_by",
			Details: map[string]interface{}{
				"details": s.spaceService.GetParticipantDetails(s.mw, s.AccountInfo, resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(), lastModifiedById),
			},
		},
		{
			Id: "created_date",
			Details: map[string]interface{}{
				"created_date": PosixToISO8601(resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyCreatedDate.String()].GetNumberValue()),
			},
		},
		{
			Id: "created_by",
			Details: map[string]interface{}{
				"details": s.spaceService.GetParticipantDetails(s.mw, s.AccountInfo, resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(), creatorId),
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
					Name:  detail.Details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
					Color: detail.Details.Fields[bundle.RelationKeyRelationOptionColor.String()].GetStringValue(),
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
			Align:           model.BlockAlign_name[int32(block.Align)],
			VerticalAlign:   model.BlockVerticalAlign_name[int32(block.VerticalAlign)],
			Text:            text,
			File:            file,
		})
	}

	return blocks
}

func PosixToISO8601(posix float64) string {
	t := time.Unix(int64(posix), 0).UTC()
	return t.Format(time.RFC3339)
}
