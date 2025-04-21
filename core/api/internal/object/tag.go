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
	resp := s.mw.ObjectShow(context.Background(), &pb.RpcObjectShowRequest{
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
	// TODO: implement tag creation
	return Tag{}, nil
}

// UpdateTag updates an existing tag option for a given property ID in a space.
func (s *service) UpdateTag(ctx context.Context, spaceId string, propertyId string, tagId string, request UpdateTagRequest) (Tag, error) {
	// TODO: implement tag update
	return Tag{}, nil
}

// DeleteTag deletes a tag option for a given property ID in a space.
func (s *service) DeleteTag(ctx context.Context, spaceId string, propertyId string, tagId string) (Tag, error) {
	// TODO: implement tag deletion
	return Tag{}, nil
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

		tags = append(tags, s.mapTagFromRecord(resp.ObjectView.Details[0].Details))
	}

	return tags
}

func (s *service) mapTagFromRecord(record *types.Struct) Tag {
	return Tag{
		Id:    record.Fields[bundle.RelationKeyId.String()].GetStringValue(),
		Key:   ToTagApiKey(record.Fields[bundle.RelationKeyRelationKey.String()].GetStringValue()),
		Name:  record.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Color: ColorOptionToColor[record.Fields[bundle.RelationKeyRelationOptionColor.String()].GetStringValue()],
	}
}
