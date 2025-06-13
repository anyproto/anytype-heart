package service

import (
	"context"
	"errors"

	"github.com/gogo/protobuf/types"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ErrInvalidPropertyId  = errors.New("invalid property id")
	ErrFailedRetrieveTags = errors.New("failed to retrieve tags")
	ErrTagNotFound        = errors.New("tag not found")
	ErrTagDeleted         = errors.New("tag deleted")
	ErrFailedRetrieveTag  = errors.New("failed to retrieve tag")
	ErrFailedCreateTag    = errors.New("failed to create tag")
	ErrFailedUpdateTag    = errors.New("failed to update tag")
	ErrFailedDeleteTag    = errors.New("failed to delete tag")
)

// ListTags returns all tags for a given property id in a space.
func (s *Service) ListTags(ctx context.Context, spaceId string, propertyId string, offset int, limit int) (tags []apimodel.Tag, total int, hasMore bool, err error) {
	_, rk, err := util.ResolveIdtoUniqueKeyAndRelationKey(s.mw, spaceId, propertyId)
	if err != nil {
		return nil, 0, false, ErrInvalidPropertyId
	}

	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.IntList(util.LayoutsToIntArgs(util.TagLayouts)...),
			},
			{
				RelationKey: bundle.RelationKeyRelationKey.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(rk),
			},
		},
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyUniqueKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyRelationOptionColor.String(),
		},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedRetrieveTags
	}

	if len(resp.Records) == 0 {
		return []apimodel.Tag{}, 0, false, nil
	}

	total = len(resp.Records)
	paginatedTags, hasMore := pagination.Paginate(resp.Records, offset, limit)
	tags = make([]apimodel.Tag, 0, len(paginatedTags))

	for _, record := range resp.Records {
		tags = append(tags, s.getTagFromStruct(record))
	}

	return tags, total, hasMore, nil
}

// GetTag retrieves a single tag for a given property id in a space.
func (s *Service) GetTag(ctx context.Context, spaceId string, propertyId string, tagId string) (apimodel.Tag, error) {
	resp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: tagId,
	})

	if resp.Error != nil {
		if resp.Error.Code == pb.RpcObjectShowResponseError_NOT_FOUND {
			return apimodel.Tag{}, ErrTagNotFound
		}

		if resp.Error.Code == pb.RpcObjectShowResponseError_OBJECT_DELETED {
			return apimodel.Tag{}, ErrTagDeleted
		}

		if resp.Error != nil && resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			return apimodel.Tag{}, ErrFailedRetrieveTag
		}
	}

	return s.getTagFromStruct(resp.ObjectView.Details[0].Details), nil
}

// CreateTag creates a new tag option for a given property ID in a space.
func (s *Service) CreateTag(ctx context.Context, spaceId string, propertyId string, request apimodel.CreateTagRequest) (apimodel.Tag, error) {
	_, rk, err := util.ResolveIdtoUniqueKeyAndRelationKey(s.mw, spaceId, propertyId)
	if err != nil {
		return apimodel.Tag{}, ErrInvalidPropertyId
	}

	details := &types.Struct{
		Fields: map[string]*types.Value{
			bundle.RelationKeyRelationKey.String():         pbtypes.String(rk),
			bundle.RelationKeyName.String():                pbtypes.String(s.sanitizedString(request.Name)),
			bundle.RelationKeyRelationOptionColor.String(): pbtypes.String(apimodel.ColorToColorOption[request.Color]),
			bundle.RelationKeyOrigin.String():              pbtypes.Int64(int64(model.ObjectOrigin_api)),
		},
	}

	resp := s.mw.ObjectCreateRelationOption(ctx, &pb.RpcObjectCreateRelationOptionRequest{
		SpaceId: spaceId,
		Details: details,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectCreateRelationOptionResponseError_NULL {
		return apimodel.Tag{}, ErrFailedCreateTag
	}

	return s.GetTag(ctx, spaceId, propertyId, resp.ObjectId)
}

// UpdateTag updates an existing tag option for a given property ID in a space.
func (s *Service) UpdateTag(ctx context.Context, spaceId string, propertyId string, tagId string, request apimodel.UpdateTagRequest) (apimodel.Tag, error) {
	_, err := s.GetTag(ctx, spaceId, propertyId, tagId)
	if err != nil {
		return apimodel.Tag{}, err
	}

	var details []*model.Detail
	if request.Name != nil {
		details = append(details, &model.Detail{
			Key:   bundle.RelationKeyName.String(),
			Value: pbtypes.String(s.sanitizedString(*request.Name)),
		})
	}
	if request.Color != nil {
		details = append(details, &model.Detail{
			Key:   bundle.RelationKeyRelationOptionColor.String(),
			Value: pbtypes.String(apimodel.ColorToColorOption[*request.Color]),
		})
	}

	if len(details) > 0 {
		resp := s.mw.ObjectSetDetails(ctx, &pb.RpcObjectSetDetailsRequest{
			ContextId: tagId,
			Details:   details,
		})

		if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetDetailsResponseError_NULL {
			return apimodel.Tag{}, ErrFailedUpdateTag
		}
	}

	return s.GetTag(ctx, spaceId, propertyId, tagId)
}

// DeleteTag deletes a tag option for a given property ID in a space.
func (s *Service) DeleteTag(ctx context.Context, spaceId string, propertyId string, tagId string) (apimodel.Tag, error) {
	tag, err := s.GetTag(ctx, spaceId, propertyId, tagId)
	if err != nil {
		return apimodel.Tag{}, err
	}

	resp := s.mw.ObjectSetIsArchived(ctx, &pb.RpcObjectSetIsArchivedRequest{
		ContextId:  tagId,
		IsArchived: true,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetIsArchivedResponseError_NULL {
		return apimodel.Tag{}, ErrFailedDeleteTag
	}

	return tag, nil
}

// getTagMapsFromStore retrieves all tags for all spaces.
func (s *Service) getTagMapsFromStore(ctx context.Context, spaceIds []string) (map[string]map[string]apimodel.Tag, error) {
	spacesToTags := make(map[string]map[string]apimodel.Tag)
	for _, spaceId := range spaceIds {
		tagMap, err := s.getTagMapFromStore(ctx, spaceId)
		if err != nil {
			return nil, err
		}
		spacesToTags[spaceId] = tagMap
	}
	return spacesToTags, nil
}

// getTagMapFromStore retrieves all tags for a specific space.
func (s *Service) getTagMapFromStore(ctx context.Context, spaceId string) (map[string]apimodel.Tag, error) {
	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_relationOption)),
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
			bundle.RelationKeyRelationOptionColor.String(),
		},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, ErrFailedRetrieveTags
	}

	tags := make(map[string]apimodel.Tag)
	for _, record := range resp.Records {
		tag := s.getTagFromStruct(record)
		tags[tag.Id] = tag
	}

	return tags, nil
}

// getTagFromStruct converts a tag's details from a struct to an apimodel.Tag.
func (s *Service) getTagFromStruct(details *types.Struct) apimodel.Tag {
	return apimodel.Tag{
		Object: "tag",
		Id:     details.Fields[bundle.RelationKeyId.String()].GetStringValue(),
		Key:    util.ToTagApiKey(details.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue()),
		Name:   details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Color:  apimodel.ColorOptionToColor[details.Fields[bundle.RelationKeyRelationOptionColor.String()].GetStringValue()],
	}
}

func (s *Service) getTagsFromStruct(tagIds []string, tagMap map[string]apimodel.Tag) []apimodel.Tag {
	tags := make([]apimodel.Tag, 0, len(tagIds))
	for _, tagId := range tagIds {
		if tagId == "" {
			continue
		}

		tag, ok := tagMap[tagId]
		if !ok {
			continue
		}

		tags = append(tags, tag)
	}

	return tags
}
