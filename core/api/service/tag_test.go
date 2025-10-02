package service

import (
	"context"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	mockedPropertyId   = "property-id"
	mockedPropertyKey  = "status"
	mockedTagId        = "tag-id"
	mockedTagName      = "In Progress"
	mockedTagKey       = "tag_key_123"
	mockedTagUniqueKey = "unique_tag_123"
	mockedTagColor     = "yellow"
	mockedCustomTagKey = "custom_status_key"
)

func TestService_ListTags(t *testing.T) {
	t.Run("tags found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedPropertyId,
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyUniqueKey.String():   pbtypes.String("unique-key"),
								bundle.RelationKeyRelationKey.String(): pbtypes.String(mockedPropertyKey),
							},
						},
					},
				},
			},
		}).Once()

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							bundle.RelationKeyId.String():                  pbtypes.String("tag-1"),
							bundle.RelationKeyName.String():                pbtypes.String("To Do"),
							bundle.RelationKeyUniqueKey.String():           pbtypes.String("unique_tag_1"),
							bundle.RelationKeyRelationOptionColor.String(): pbtypes.String("red"),
						},
					},
					{
						Fields: map[string]*types.Value{
							bundle.RelationKeyId.String():                  pbtypes.String("tag-2"),
							bundle.RelationKeyName.String():                pbtypes.String("Done"),
							bundle.RelationKeyUniqueKey.String():           pbtypes.String("unique_tag_2"),
							bundle.RelationKeyApiObjectKey.String():        pbtypes.String("custom_done"),
							bundle.RelationKeyRelationOptionColor.String(): pbtypes.String("lime"),
						},
					},
				},
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		// when
		tags, total, hasMore, err := fx.service.ListTags(ctx, mockedSpaceId, mockedPropertyId, nil, 0, 100)

		// then
		require.NoError(t, err)
		require.Len(t, tags, 2)
		require.Equal(t, 2, total)
		require.False(t, hasMore)

		// Check first tag (without custom key)
		require.Equal(t, "tag-1", tags[0].Id)
		require.Equal(t, "To Do", tags[0].Name)
		require.Equal(t, util.ToTagApiKey("unique_tag_1"), tags[0].Key)
		require.Equal(t, apimodel.ColorRed, tags[0].Color)

		// Check second tag (with custom key)
		require.Equal(t, "tag-2", tags[1].Id)
		require.Equal(t, "Done", tags[1].Name)
		require.Equal(t, "custom_done", tags[1].Key) // Should use custom key
		require.Equal(t, apimodel.ColorLime, tags[1].Color)
	})

	t.Run("no tags found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedPropertyId,
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyUniqueKey.String():   pbtypes.String("unique-key"),
								bundle.RelationKeyRelationKey.String(): pbtypes.String(mockedPropertyKey),
							},
						},
					},
				},
			},
		}).Once()

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{},
				Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		// when
		tags, total, hasMore, err := fx.service.ListTags(ctx, mockedSpaceId, mockedPropertyId, nil, 0, 100)

		// then
		require.NoError(t, err)
		require.Len(t, tags, 0)
		require.Equal(t, 0, total)
		require.False(t, hasMore)
	})

	t.Run("invalid property id", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: "invalid-property",
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NOT_FOUND},
		}).Once()

		// when
		tags, total, hasMore, err := fx.service.ListTags(ctx, mockedSpaceId, "invalid-property", nil, 0, 100)

		// then
		require.ErrorIs(t, err, ErrInvalidPropertyId)
		require.Nil(t, tags)
		require.Equal(t, 0, total)
		require.False(t, hasMore)
	})
}

func TestService_GetTag(t *testing.T) {
	t.Run("tag found with custom key", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedTagId,
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyId.String():                  pbtypes.String(mockedTagId),
								bundle.RelationKeyName.String():                pbtypes.String(mockedTagName),
								bundle.RelationKeyUniqueKey.String():           pbtypes.String(mockedTagUniqueKey),
								bundle.RelationKeyApiObjectKey.String():        pbtypes.String(mockedCustomTagKey),
								bundle.RelationKeyRelationOptionColor.String(): pbtypes.String(mockedTagColor),
							},
						},
					},
				},
			},
		}).Once()

		// when
		tag, err := fx.service.GetTag(ctx, mockedSpaceId, mockedPropertyId, mockedTagId)

		// then
		require.NoError(t, err)
		require.Equal(t, mockedTagId, tag.Id)
		require.Equal(t, mockedTagName, tag.Name)
		require.Equal(t, mockedCustomTagKey, tag.Key) // Should use custom key
		require.Equal(t, apimodel.ColorYellow, tag.Color)
	})

	t.Run("tag found without custom key", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedTagId,
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyId.String():                  pbtypes.String(mockedTagId),
								bundle.RelationKeyName.String():                pbtypes.String(mockedTagName),
								bundle.RelationKeyUniqueKey.String():           pbtypes.String(mockedTagUniqueKey),
								bundle.RelationKeyRelationOptionColor.String(): pbtypes.String(mockedTagColor),
							},
						},
					},
				},
			},
		}).Once()

		// when
		tag, err := fx.service.GetTag(ctx, mockedSpaceId, mockedPropertyId, mockedTagId)

		// then
		require.NoError(t, err)
		require.Equal(t, mockedTagId, tag.Id)
		require.Equal(t, mockedTagName, tag.Name)
		require.Equal(t, util.ToTagApiKey(mockedTagUniqueKey), tag.Key) // Should use generated key
		require.Equal(t, apimodel.ColorYellow, tag.Color)
	})

	t.Run("tag not found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: "non-existent-tag",
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NOT_FOUND},
		}).Once()

		// when
		tag, err := fx.service.GetTag(ctx, mockedSpaceId, mockedPropertyId, "non-existent-tag")

		// then
		require.ErrorIs(t, err, ErrTagNotFound)
		require.Nil(t, tag)
	})

	t.Run("tag deleted", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: "deleted-tag",
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_OBJECT_DELETED},
		}).Once()

		// when
		tag, err := fx.service.GetTag(ctx, mockedSpaceId, mockedPropertyId, "deleted-tag")

		// then
		require.ErrorIs(t, err, ErrTagDeleted)
		require.Nil(t, tag)
	})
}

func TestService_CreateTag(t *testing.T) {
	t.Run("create tag with custom key", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		request := apimodel.CreateTagRequest{
			Key:   "my_custom_tag",
			Name:  "My Tag",
			Color: apimodel.ColorBlue,
		}

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedPropertyId,
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyUniqueKey.String():   pbtypes.String("unique-key"),
								bundle.RelationKeyRelationKey.String(): pbtypes.String(mockedPropertyKey),
							},
						},
					},
				},
			},
		}).Once()

		fx.mwMock.On("ObjectCreateRelationOption", mock.Anything, mock.MatchedBy(func(req *pb.RpcObjectCreateRelationOptionRequest) bool {
			return req.SpaceId == mockedSpaceId &&
				req.Details.Fields[bundle.RelationKeyName.String()].GetStringValue() == "My Tag" &&
				req.Details.Fields[bundle.RelationKeyApiObjectKey.String()].GetStringValue() == "my_custom_tag" &&
				req.Details.Fields[bundle.RelationKeyRelationOptionColor.String()].GetStringValue() == "blue"
		})).Return(&pb.RpcObjectCreateRelationOptionResponse{
			Error:    &pb.RpcObjectCreateRelationOptionResponseError{Code: pb.RpcObjectCreateRelationOptionResponseError_NULL},
			ObjectId: "new-tag-id",
		}).Once()

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: "new-tag-id",
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyId.String():                  pbtypes.String("new-tag-id"),
								bundle.RelationKeyName.String():                pbtypes.String("My Tag"),
								bundle.RelationKeyUniqueKey.String():           pbtypes.String("unique_new_tag"),
								bundle.RelationKeyApiObjectKey.String():        pbtypes.String("my_custom_tag"),
								bundle.RelationKeyRelationOptionColor.String(): pbtypes.String("blue"),
							},
						},
					},
				},
			},
		}).Once()

		// when
		tag, err := fx.service.CreateTag(ctx, mockedSpaceId, mockedPropertyId, request)

		// then
		require.NoError(t, err)
		require.Equal(t, "new-tag-id", tag.Id)
		require.Equal(t, "My Tag", tag.Name)
		require.Equal(t, "my_custom_tag", tag.Key)
		require.Equal(t, apimodel.ColorBlue, tag.Color)
	})

	t.Run("create tag without custom key", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		request := apimodel.CreateTagRequest{
			Name:  "Simple Tag",
			Color: apimodel.ColorLime,
		}

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedPropertyId,
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyUniqueKey.String():   pbtypes.String("unique-key"),
								bundle.RelationKeyRelationKey.String(): pbtypes.String(mockedPropertyKey),
							},
						},
					},
				},
			},
		}).Once()

		fx.mwMock.On("ObjectCreateRelationOption", mock.Anything, mock.MatchedBy(func(req *pb.RpcObjectCreateRelationOptionRequest) bool {
			_, hasApiKey := req.Details.Fields[bundle.RelationKeyApiObjectKey.String()]
			return req.SpaceId == mockedSpaceId &&
				req.Details.Fields[bundle.RelationKeyName.String()].GetStringValue() == "Simple Tag" &&
				!hasApiKey // Should not have custom key field
		})).Return(&pb.RpcObjectCreateRelationOptionResponse{
			Error:    &pb.RpcObjectCreateRelationOptionResponseError{Code: pb.RpcObjectCreateRelationOptionResponseError_NULL},
			ObjectId: "simple-tag-id",
		}).Once()

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: "simple-tag-id",
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyId.String():                  pbtypes.String("simple-tag-id"),
								bundle.RelationKeyName.String():                pbtypes.String("Simple Tag"),
								bundle.RelationKeyUniqueKey.String():           pbtypes.String("unique_simple_tag"),
								bundle.RelationKeyRelationOptionColor.String(): pbtypes.String("lime"),
							},
						},
					},
				},
			},
		}).Once()

		// when
		tag, err := fx.service.CreateTag(ctx, mockedSpaceId, mockedPropertyId, request)

		// then
		require.NoError(t, err)
		require.Equal(t, "simple-tag-id", tag.Id)
		require.Equal(t, "Simple Tag", tag.Name)
		require.Equal(t, util.ToTagApiKey("unique_simple_tag"), tag.Key)
		require.Equal(t, apimodel.ColorLime, tag.Color)
	})

	t.Run("create tag with duplicate custom key", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		// Add existing tag with the same key to cache
		existingTag := &apimodel.Tag{
			Id:        "existing-tag",
			Key:       "duplicate_key",
			Name:      "Existing Tag",
			Color:     apimodel.ColorRed,
			UniqueKey: "unique_existing",
		}
		fx.service.cache.cacheTag(mockedSpaceId, existingTag)

		request := apimodel.CreateTagRequest{
			Key:   "duplicate_key",
			Name:  "New Tag",
			Color: apimodel.ColorBlue,
		}

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedPropertyId,
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyUniqueKey.String():   pbtypes.String("unique-key"),
								bundle.RelationKeyRelationKey.String(): pbtypes.String(mockedPropertyKey),
							},
						},
					},
				},
			},
		}).Once()

		// when
		tag, err := fx.service.CreateTag(ctx, mockedSpaceId, mockedPropertyId, request)

		// then
		require.Error(t, err)
		require.Contains(t, err.Error(), "tag key \"duplicate_key\" already exists")
		require.Nil(t, tag)
	})

	t.Run("invalid property id", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		request := apimodel.CreateTagRequest{
			Name:  "Tag",
			Color: apimodel.ColorRed,
		}

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: "invalid-property",
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NOT_FOUND},
		}).Once()

		// when
		tag, err := fx.service.CreateTag(ctx, mockedSpaceId, "invalid-property", request)

		// then
		require.ErrorIs(t, err, ErrInvalidPropertyId)
		require.Nil(t, tag)
	})
}

func TestService_UpdateTag(t *testing.T) {
	t.Run("update tag name and color", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		newName := "Updated Name"
		newColor := apimodel.ColorPurple

		request := apimodel.UpdateTagRequest{
			Name:  &newName,
			Color: &newColor,
		}

		// Mock GetTag
		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedTagId,
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyId.String():                  pbtypes.String(mockedTagId),
								bundle.RelationKeyName.String():                pbtypes.String("Old Name"),
								bundle.RelationKeyUniqueKey.String():           pbtypes.String(mockedTagUniqueKey),
								bundle.RelationKeyRelationOptionColor.String(): pbtypes.String("red"),
							},
						},
					},
				},
			},
		}).Times(2) // Called twice: once for GetTag check, once after update

		fx.mwMock.On("ObjectSetDetails", mock.Anything, mock.MatchedBy(func(req *pb.RpcObjectSetDetailsRequest) bool {
			return req.ContextId == mockedTagId &&
				len(req.Details) == 2 &&
				req.Details[0].Key == bundle.RelationKeyName.String() &&
				req.Details[0].Value.GetStringValue() == newName &&
				req.Details[1].Key == bundle.RelationKeyRelationOptionColor.String() &&
				req.Details[1].Value.GetStringValue() == "purple"
		})).Return(&pb.RpcObjectSetDetailsResponse{
			Error: &pb.RpcObjectSetDetailsResponseError{Code: pb.RpcObjectSetDetailsResponseError_NULL},
		}).Once()

		// when
		tag, err := fx.service.UpdateTag(ctx, mockedSpaceId, mockedPropertyId, mockedTagId, request)

		// then
		require.NoError(t, err)
		require.Equal(t, mockedTagId, tag.Id)
	})

	t.Run("update tag with custom key", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		newKey := "updated_custom_key"
		request := apimodel.UpdateTagRequest{
			Key: &newKey,
		}

		// Mock GetTag
		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedTagId,
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyId.String():                  pbtypes.String(mockedTagId),
								bundle.RelationKeyName.String():                pbtypes.String(mockedTagName),
								bundle.RelationKeyUniqueKey.String():           pbtypes.String(mockedTagUniqueKey),
								bundle.RelationKeyApiObjectKey.String():        pbtypes.String("old_key"),
								bundle.RelationKeyRelationOptionColor.String(): pbtypes.String(mockedTagColor),
							},
						},
					},
				},
			},
		}).Times(2)

		fx.mwMock.On("ObjectSetDetails", mock.Anything, mock.MatchedBy(func(req *pb.RpcObjectSetDetailsRequest) bool {
			return req.ContextId == mockedTagId &&
				len(req.Details) == 1 &&
				req.Details[0].Key == bundle.RelationKeyApiObjectKey.String() &&
				req.Details[0].Value.GetStringValue() == newKey
		})).Return(&pb.RpcObjectSetDetailsResponse{
			Error: &pb.RpcObjectSetDetailsResponseError{Code: pb.RpcObjectSetDetailsResponseError_NULL},
		}).Once()

		// when
		tag, err := fx.service.UpdateTag(ctx, mockedSpaceId, mockedPropertyId, mockedTagId, request)

		// then
		require.NoError(t, err)
		require.Equal(t, mockedTagId, tag.Id)
	})

	t.Run("update tag with duplicate custom key", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		// Add existing tag with key to cache
		existingTag := &apimodel.Tag{
			Id:        "other-tag",
			Key:       "taken_key",
			Name:      "Other Tag",
			Color:     apimodel.ColorRed,
			UniqueKey: "unique_other",
		}
		fx.service.cache.cacheTag(mockedSpaceId, existingTag)

		duplicateKey := "taken_key"
		request := apimodel.UpdateTagRequest{
			Key: &duplicateKey,
		}

		// Mock GetTag
		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedTagId,
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyId.String():                  pbtypes.String(mockedTagId),
								bundle.RelationKeyName.String():                pbtypes.String(mockedTagName),
								bundle.RelationKeyUniqueKey.String():           pbtypes.String(mockedTagUniqueKey),
								bundle.RelationKeyApiObjectKey.String():        pbtypes.String("current_key"),
								bundle.RelationKeyRelationOptionColor.String(): pbtypes.String(mockedTagColor),
							},
						},
					},
				},
			},
		}).Once()

		// when
		tag, err := fx.service.UpdateTag(ctx, mockedSpaceId, mockedPropertyId, mockedTagId, request)

		// then
		require.Error(t, err)
		require.Contains(t, err.Error(), "tag key \"taken_key\" already exists")
		require.Nil(t, tag)
	})

	t.Run("update non-existent tag", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		newName := "Updated"

		request := apimodel.UpdateTagRequest{
			Name: &newName,
		}

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: "non-existent",
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NOT_FOUND},
		}).Once()

		// when
		tag, err := fx.service.UpdateTag(ctx, mockedSpaceId, mockedPropertyId, "non-existent", request)

		// then
		require.ErrorIs(t, err, ErrTagNotFound)
		require.Nil(t, tag)
	})

	t.Run("update with no changes", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		request := apimodel.UpdateTagRequest{} // No fields to update

		// Mock GetTag
		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedTagId,
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyId.String():                  pbtypes.String(mockedTagId),
								bundle.RelationKeyName.String():                pbtypes.String(mockedTagName),
								bundle.RelationKeyUniqueKey.String():           pbtypes.String(mockedTagUniqueKey),
								bundle.RelationKeyRelationOptionColor.String(): pbtypes.String(mockedTagColor),
							},
						},
					},
				},
			},
		}).Times(2)

		// when
		tag, err := fx.service.UpdateTag(ctx, mockedSpaceId, mockedPropertyId, mockedTagId, request)

		// then
		require.NoError(t, err)
		require.Equal(t, mockedTagId, tag.Id)
		require.Equal(t, mockedTagName, tag.Name)

		// Verify ObjectSetDetails was NOT called since there are no changes
		fx.mwMock.AssertNotCalled(t, "ObjectSetDetails")
	})
}

func TestService_DeleteTag(t *testing.T) {
	t.Run("delete existing tag", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// Mock GetTag
		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedTagId,
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyId.String():                  pbtypes.String(mockedTagId),
								bundle.RelationKeyName.String():                pbtypes.String(mockedTagName),
								bundle.RelationKeyUniqueKey.String():           pbtypes.String(mockedTagUniqueKey),
								bundle.RelationKeyRelationOptionColor.String(): pbtypes.String(mockedTagColor),
							},
						},
					},
				},
			},
		}).Once()

		fx.mwMock.On("ObjectSetIsArchived", mock.Anything, &pb.RpcObjectSetIsArchivedRequest{
			ContextId:  mockedTagId,
			IsArchived: true,
		}).Return(&pb.RpcObjectSetIsArchivedResponse{
			Error: &pb.RpcObjectSetIsArchivedResponseError{Code: pb.RpcObjectSetIsArchivedResponseError_NULL},
		}).Once()

		// when
		tag, err := fx.service.DeleteTag(ctx, mockedSpaceId, mockedPropertyId, mockedTagId)

		// then
		require.NoError(t, err)
		require.Equal(t, mockedTagId, tag.Id)
		require.Equal(t, mockedTagName, tag.Name)
	})

	t.Run("delete non-existent tag", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: "non-existent",
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NOT_FOUND},
		}).Once()

		// when
		tag, err := fx.service.DeleteTag(ctx, mockedSpaceId, mockedPropertyId, "non-existent")

		// then
		require.ErrorIs(t, err, ErrTagNotFound)
		require.Nil(t, tag)
	})

	t.Run("delete already deleted tag", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: "deleted-tag",
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_OBJECT_DELETED},
		}).Once()

		// when
		tag, err := fx.service.DeleteTag(ctx, mockedSpaceId, mockedPropertyId, "deleted-tag")

		// then
		require.ErrorIs(t, err, ErrTagDeleted)
		require.Nil(t, tag)
	})
}

func TestService_getTagFromStruct(t *testing.T) {
	fx := newFixture(t)

	t.Run("with custom key", func(t *testing.T) {
		details := &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():                  pbtypes.String("tag-id"),
				bundle.RelationKeyName.String():                pbtypes.String("Tag Name"),
				bundle.RelationKeyUniqueKey.String():           pbtypes.String("unique_tag_key"),
				bundle.RelationKeyApiObjectKey.String():        pbtypes.String("custom_api_key"),
				bundle.RelationKeyRelationOptionColor.String(): pbtypes.String("blue"),
			},
		}

		tag := fx.service.getTagFromStruct(details)

		assert.Equal(t, "tag-id", tag.Id)
		assert.Equal(t, "Tag Name", tag.Name)
		assert.Equal(t, "custom_api_key", tag.Key) // Should use custom key
		assert.Equal(t, "unique_tag_key", tag.UniqueKey)
		assert.Equal(t, apimodel.ColorBlue, tag.Color)
	})

	t.Run("without custom key", func(t *testing.T) {
		details := &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():                  pbtypes.String("tag-id"),
				bundle.RelationKeyName.String():                pbtypes.String("Tag Name"),
				bundle.RelationKeyUniqueKey.String():           pbtypes.String("unique_tag_key"),
				bundle.RelationKeyRelationOptionColor.String(): pbtypes.String("lime"),
			},
		}

		tag := fx.service.getTagFromStruct(details)

		assert.Equal(t, "tag-id", tag.Id)
		assert.Equal(t, "Tag Name", tag.Name)
		assert.Equal(t, util.ToTagApiKey("unique_tag_key"), tag.Key) // Should use generated key
		assert.Equal(t, "unique_tag_key", tag.UniqueKey)
		assert.Equal(t, apimodel.ColorLime, tag.Color)
	})

	t.Run("with empty custom key", func(t *testing.T) {
		details := &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():                  pbtypes.String("tag-id"),
				bundle.RelationKeyName.String():                pbtypes.String("Tag Name"),
				bundle.RelationKeyUniqueKey.String():           pbtypes.String("unique_tag_key"),
				bundle.RelationKeyApiObjectKey.String():        pbtypes.String(""), // Empty string
				bundle.RelationKeyRelationOptionColor.String(): pbtypes.String("red"),
			},
		}

		tag := fx.service.getTagFromStruct(details)

		assert.Equal(t, "tag-id", tag.Id)
		assert.Equal(t, "Tag Name", tag.Name)
		assert.Equal(t, util.ToTagApiKey("unique_tag_key"), tag.Key) // Should use generated key when custom key is empty
		assert.Equal(t, "unique_tag_key", tag.UniqueKey)
		assert.Equal(t, apimodel.ColorRed, tag.Color)
	})
}
