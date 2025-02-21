package object

import (
	"context"
	"errors"

	"github.com/gogo/protobuf/types"
	"github.com/iancoleman/strcase"

	"github.com/anyproto/anytype-heart/core/api/internal/space"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	// objects
	ErrObjectNotFound            = errors.New("object not found")
	ErrFailedRetrieveObject      = errors.New("failed to retrieve object")
	ErrFailedRetrieveObjects     = errors.New("failed to retrieve list of objects")
	ErrFailedDeleteObject        = errors.New("failed to delete object")
	ErrFailedCreateObject        = errors.New("failed to create object")
	ErrInputMissingSource        = errors.New("source is missing for bookmark")
	ErrFailedSetRelationFeatured = errors.New("failed to set relation featured")
	ErrFailedFetchBookmark       = errors.New("failed to fetch bookmark")
	ErrFailedPasteBody           = errors.New("failed to paste body")

	// types
	ErrFailedRetrieveTypes        = errors.New("failed to retrieve types")
	ErrTypeNotFound               = errors.New("type not found")
	ErrFailedRetrieveType         = errors.New("failed to retrieve type")
	ErrFailedRetrieveTemplateType = errors.New("failed to retrieve template type")
	ErrTemplateTypeNotFound       = errors.New("template type not found")
	ErrFailedRetrieveTemplate     = errors.New("failed to retrieve template")
	ErrFailedRetrieveTemplates    = errors.New("failed to retrieve templates")
	ErrTemplateNotFound           = errors.New("template not found")
)

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
	mw           service.ClientCommandsServer
	spaceService *space.SpaceService
	AccountInfo  *model.AccountInfo
}

func NewService(mw service.ClientCommandsServer, spaceService *space.SpaceService) *ObjectService {
	return &ObjectService{mw: mw, spaceService: spaceService}
}

// ListObjects retrieves a paginated list of objects in a specific space.
func (s *ObjectService) ListObjects(ctx context.Context, spaceId string, offset int, limit int) (objects []Object, total int, hasMore bool, err error) {
	typeId, err := util.ResolveUniqueKeyToTypeId(s.mw, spaceId, "ot-template")

	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				Operator:    model.BlockContentDataviewFilter_No,
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
			{
				Operator:    model.BlockContentDataviewFilter_No,
				RelationKey: bundle.RelationKeyType.String(),
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.String(typeId),
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

	if resp.Error.Code == pb.RpcObjectShowResponseError_NOT_FOUND || resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue() {
		return Object{}, ErrObjectNotFound
	}

	if resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
		return Object{}, ErrFailedRetrieveObject
	}

	icon := util.GetIconFromEmojiOrImage(s.AccountInfo, resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconImage.String()].GetStringValue())

	object := Object{
		Object:  "object",
		Id:      resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyId.String()].GetStringValue(),
		Name:    resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:    icon,
		Type:    s.getTypeFromDetails(resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyType.String()].GetStringValue(), resp.ObjectView.Details),
		Snippet: resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeySnippet.String()].GetStringValue(),
		Layout:  model.ObjectTypeLayout_name[int32(resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyLayout.String()].GetNumberValue())],
		SpaceId: resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(),
		RootId:  resp.ObjectView.RootId,
		Blocks:  s.getBlocks(resp),
		Details: s.getDetails(resp),
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
				Operator:    model.BlockContentDataviewFilter_No,
				RelationKey: bundle.RelationKeyLayout.String(),
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
			Object:            "type",
			Id:                record.Fields[bundle.RelationKeyId.String()].GetStringValue(),
			UniqueKey:         record.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(),
			Name:              record.Fields[bundle.RelationKeyName.String()].GetStringValue(),
			Icon:              record.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(),
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

	if resp.Error.Code == pb.RpcObjectShowResponseError_NOT_FOUND {
		return Type{}, ErrTypeNotFound
	}

	if resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
		return Type{}, ErrFailedRetrieveType
	}

	return Type{
		Object:            "type",
		Id:                typeId,
		UniqueKey:         resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(),
		Name:              resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:              resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(),
		RecommendedLayout: model.ObjectTypeLayout_name[int32(resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue())],
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
			Object: "template",
			Id:     templateId,
			Name:   templateResp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
			Icon:   templateResp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(),
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

	if resp.Error.Code == pb.RpcObjectShowResponseError_NOT_FOUND {
		return Template{}, ErrTemplateNotFound
	}

	if resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
		return Template{}, ErrFailedRetrieveTemplate
	}

	return Template{
		Object: "template",
		Id:     templateId,
		Name:   resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:   resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(),
	}, nil
}

// getTypeFromDetails returns the type from the details of the ObjectShowResponse.
func (s *ObjectService) getTypeFromDetails(typeId string, details []*model.ObjectViewDetailsSet) Type {
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
		UniqueKey:         objectTypeDetail.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(),
		Name:              objectTypeDetail.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:              objectTypeDetail.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(),
		RecommendedLayout: model.ObjectTypeLayout_name[int32(objectTypeDetail.Fields[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue())],
	}
}

// getDetails returns a list of details by iterating over all relations found in the RelationLinks and mapping their format and value.
func (s *ObjectService) getDetails(resp *pb.RpcObjectShowResponse) []Detail {
	relationFormatMap := s.getRelationFormatMap(resp.ObjectView.RelationLinks)
	linkedRelations := resp.ObjectView.RelationLinks
	primaryDetailFields := resp.ObjectView.Details[0].Details.Fields

	// system relations to be excluded
	excludeRelations := map[string]bool{
		bundle.RelationKeyId.String():                true,
		bundle.RelationKeySpaceId.String():           true,
		bundle.RelationKeyName.String():              true,
		bundle.RelationKeyIconEmoji.String():         true,
		bundle.RelationKeyIconImage.String():         true,
		bundle.RelationKeyType.String():              true,
		bundle.RelationKeyLayout.String():            true,
		bundle.RelationKeyIsFavorite.String():        true,
		bundle.RelationKeyIsArchived.String():        true,
		bundle.RelationKeyIsDeleted.String():         true,
		bundle.RelationKeyIsHidden.String():          true,
		bundle.RelationKeyWorkspaceId.String():       true,
		bundle.RelationKeyInternalFlags.String():     true,
		bundle.RelationKeyRestrictions.String():      true,
		bundle.RelationKeyOrigin.String():            true,
		bundle.RelationKeySnippet.String():           true,
		bundle.RelationKeySyncStatus.String():        true,
		bundle.RelationKeySyncError.String():         true,
		bundle.RelationKeySyncDate.String():          true,
		bundle.RelationKeyCoverId.String():           true,
		bundle.RelationKeyCoverType.String():         true,
		bundle.RelationKeyCoverScale.String():        true,
		bundle.RelationKeyCoverX.String():            true,
		bundle.RelationKeyCoverY.String():            true,
		bundle.RelationKeyMentions.String():          true,
		bundle.RelationKeyOldAnytypeID.String():      true,
		bundle.RelationKeySource.String():            true,
		bundle.RelationKeySourceFilePath.String():    true,
		bundle.RelationKeyImportType.String():        true,
		bundle.RelationKeyTargetObjectType.String():  true,
		bundle.RelationKeyFeaturedRelations.String(): true,
		bundle.RelationKeySetOf.String():             true,
		bundle.RelationKeyLinks.String():             true,
		bundle.RelationKeyBacklinks.String():         true,
		bundle.RelationKeySourceObject.String():      true,
		bundle.RelationKeyLayoutAlign.String():       true,
	}

	var details []Detail
	for _, r := range linkedRelations {
		key := r.Key
		if _, isExcluded := excludeRelations[key]; isExcluded {
			continue
		}

		if val, ok := primaryDetailFields[key]; ok {
			id, name := s.getRelation(key, resp)
			format := relationFormatMap[key]
			details = append(details, Detail{
				Id: id,
				Details: map[string]interface{}{
					"name": name,
					"type": format,
					format: s.convertValue(key, val, format, resp.ObjectView.Details),
				},
			})
		}
	}
	return details
}

// getRelationName returns the relation id and relation name from the ObjectShowResponse.
func (s *ObjectService) getRelation(key string, resp *pb.RpcObjectShowResponse) (id string, name string) {
	relation, err := bundle.GetRelation(domain.RelationKey(key))
	if err != nil {
		name, err = util.ResolveRelationKeyToRelationName(s.mw, resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(), key)
		if err != nil {
			return key, key
		}
		return key, name
	}

	// special cases of relation keys and names
	if key == bundle.RelationKeyCreator.String() {
		return "created_by", "Created By"
	} else if key == bundle.RelationKeyCreatedDate.String() {
		return "created_date", "Created Date"
	}

	return strcase.ToSnake(key), relation.Name
}

// convertValue converts a protobuf types.Value into a native Go value.
func (s *ObjectService) convertValue(key string, value *types.Value, format string, details []*model.ObjectViewDetailsSet) interface{} {
	switch kind := value.Kind.(type) {
	case *types.Value_NullValue:
		return nil
	case *types.Value_NumberValue:
		if format == "date" {
			return util.PosixToISO8601(kind.NumberValue)
		}
		return kind.NumberValue
	case *types.Value_StringValue:
		if key == bundle.RelationKeyCreator.String() || key == bundle.RelationKeyLastModifiedBy.String() {
			member, err := s.spaceService.GetMember(context.Background(), details[0].Details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(), kind.StringValue)
			if err != nil {
				return nil
			}
			return member
		}

		// TODO: investigate how this is possible? select option not list and not returned in further details
		if format == "select" || format == "multi_select" {
			return s.resolveTag(details[0].Details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(), kind.StringValue)
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

		if format == "select" || format == "multi_select" {
			return s.getTags(key, details)
		}

		return list
	default:
		return nil
	}
}

// getRelationFormatMapFromResponse returns the map of relation key to relation format from the ObjectShowResponse.
func (s *ObjectService) getRelationFormatMap(relationLinks []*model.RelationLink) map[string]string {
	relationFormatToName := make(map[int32]string, len(model.RelationFormat_name))
	for k, v := range model.RelationFormat_name {
		relationFormatToName[k] = v
	}
	relationFormatToName[int32(model.RelationFormat_longtext)] = "text"
	relationFormatToName[int32(model.RelationFormat_shorttext)] = "text"
	relationFormatToName[int32(model.RelationFormat_tag)] = "multi_select"
	relationFormatToName[int32(model.RelationFormat_status)] = "select"

	relationFormatMap := map[string]string{}
	for _, detail := range relationLinks {
		relationFormatMap[detail.Key] = relationFormatToName[int32(detail.Format)]
	}

	return relationFormatMap
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
		var relation *Relation

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
		case *model.BlockContentOfRelation:
			relation = &Relation{
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
			Relation:        relation,
		})
	}

	return blocks
}
