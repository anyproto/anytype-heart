package object

import (
	"context"
	"errors"

	"github.com/gogo/protobuf/types"

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
func (s *service) ListTags(ctx context.Context, spaceId string, propertyId string, offset int, limit int) (tags []Tag, total int, hasMore bool, err error) {
	_, rk, err := util.ResolveIdtoUniqueKeyAndRelationKey(s.mw, spaceId, propertyId)
	if err != nil {
		return nil, 0, false, ErrInvalidPropertyId
	}

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
				Value:       pbtypes.String(rk),
			},
		},
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyRelationKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyRelationOptionColor.String(),
		},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedRetrieveTags
	}

	if len(resp.Records) == 0 {
		return []Tag{}, 0, false, nil
	}

	total = len(resp.Records)
	paginatedTags, hasMore := pagination.Paginate(resp.Records, offset, limit)
	tags = make([]Tag, 0, len(paginatedTags))

	for _, record := range resp.Records {
		tags = append(tags, s.mapTagFromRecord(record))
	}

	return tags, total, hasMore, nil
}

// GetTag retrieves a single tag for a given property id in a space.
func (s *service) GetTag(ctx context.Context, spaceId string, propertyId string, tagId string) (Tag, error) {
	resp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: tagId,
	})

	if resp.Error != nil {
		if resp.Error.Code == pb.RpcObjectShowResponseError_NOT_FOUND {
			return Tag{}, ErrTagNotFound
		}

		if resp.Error.Code == pb.RpcObjectShowResponseError_OBJECT_DELETED {
			return Tag{}, ErrTagDeleted
		}

		if resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			return Tag{}, ErrFailedRetrieveTag
		}
	}

	return s.mapTagFromRecord(resp.ObjectView.Details[0].Details), nil
}

// CreateTag creates a new tag option for a given property ID in a space.
func (s *service) CreateTag(ctx context.Context, spaceId string, propertyId string, request CreateTagRequest) (Tag, error) {
	_, rk, err := util.ResolveIdtoUniqueKeyAndRelationKey(s.mw, spaceId, propertyId)
	if err != nil {
		return Tag{}, ErrInvalidPropertyId
	}

	details := &types.Struct{
		Fields: map[string]*types.Value{
			bundle.RelationKeyRelationKey.String():         pbtypes.String(rk),
			bundle.RelationKeyName.String():                pbtypes.String(s.sanitizedString(request.Name)),
			bundle.RelationKeyRelationOptionColor.String(): pbtypes.String(ColorToColorOption[request.Color]),
		},
	}

	resp := s.mw.ObjectCreateRelationOption(ctx, &pb.RpcObjectCreateRelationOptionRequest{
		SpaceId: spaceId,
		Details: details,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectCreateRelationOptionResponseError_NULL {
		return Tag{}, ErrFailedCreateTag
	}

	return s.GetTag(ctx, spaceId, propertyId, resp.ObjectId)
}

// UpdateTag updates an existing tag option for a given property ID in a space.
func (s *service) UpdateTag(ctx context.Context, spaceId string, propertyId string, tagId string, request UpdateTagRequest) (Tag, error) {
	_, err := s.GetTag(ctx, spaceId, propertyId, tagId)
	if err != nil {
		return Tag{}, err
	}

	var details []*model.Detail
	if request.Name != "" {
		details = append(details, &model.Detail{
			Key:   bundle.RelationKeyName.String(),
			Value: pbtypes.String(s.sanitizedString(request.Name)),
		})
	}
	if request.Color != "" {
		details = append(details, &model.Detail{
			Key:   bundle.RelationKeyRelationOptionColor.String(),
			Value: pbtypes.String(ColorToColorOption[request.Color]),
		})
	}

	if len(details) > 0 {
		resp := s.mw.ObjectSetDetails(ctx, &pb.RpcObjectSetDetailsRequest{
			ContextId: tagId,
			Details:   details,
		})

		if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetDetailsResponseError_NULL {
			return Tag{}, ErrFailedUpdateTag
		}
	}

	return s.GetTag(ctx, spaceId, propertyId, tagId)
}

// DeleteTag deletes a tag option for a given property ID in a space.
func (s *service) DeleteTag(ctx context.Context, spaceId string, propertyId string, tagId string) (Tag, error) {
	tag, err := s.GetTag(ctx, spaceId, propertyId, tagId)
	if err != nil {
		return Tag{}, err
	}

	resp := s.mw.ObjectSetIsArchived(ctx, &pb.RpcObjectSetIsArchivedRequest{
		ContextId: tagId,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetIsArchivedResponseError_NULL {
		return Tag{}, ErrFailedDeleteTag
	}

	return tag, nil
}

// GetTagMapsFromStore retrieves all tags for all spaces.
func (s *service) GetTagMapsFromStore(spaceIds []string) (map[string]map[string]Tag, error) {
	spacesToTags := make(map[string]map[string]Tag)
	for _, spaceId := range spaceIds {
		tagMap, err := s.GetTagMapFromStore(spaceId)
		if err != nil {
			return nil, err
		}
		spacesToTags[spaceId] = tagMap
	}
	return spacesToTags, nil
}

// GetTagMapFromStore retrieves all tags for a specific space.
func (s *service) GetTagMapFromStore(spaceId string) (map[string]Tag, error) {
	resp := s.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_relationOption)),
			},
		},
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyRelationKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyRelationOptionColor.String(),
		},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, ErrFailedRetrieveTags
	}

	tags := make(map[string]Tag)
	for _, record := range resp.Records {
		tag := s.mapTagFromRecord(record)
		tags[tag.Id] = tag
	}

	return tags, nil
}

func (s *service) mapTagFromRecord(record *types.Struct) Tag {
	return Tag{
		Object: "tag",
		Id:     record.Fields[bundle.RelationKeyId.String()].GetStringValue(),
		Key:    ToTagApiKey(record.Fields[bundle.RelationKeyRelationKey.String()].GetStringValue()),
		Name:   record.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Color:  ColorOptionToColor[record.Fields[bundle.RelationKeyRelationOptionColor.String()].GetStringValue()],
	}
}

func (s *service) getTagsFromStruct(tagIds []string, tagMap map[string]Tag) []Tag {
	tags := make([]Tag, 0, len(tagIds))
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
