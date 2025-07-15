package service

import (
	"context"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestObjectService_ListTypes(t *testing.T) {
	t.Run("types found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							bundle.RelationKeyId.String():        pbtypes.String("type-1"),
							bundle.RelationKeyName.String():      pbtypes.String("Type One"),
							bundle.RelationKeyUniqueKey.String(): pbtypes.String("type-one-key"),
							bundle.RelationKeyIconEmoji.String(): pbtypes.String("🗂️"),
						},
					},
				},
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		// Mock getPropertyMapFromStore
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: mockedSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyResolvedLayout.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
				},
				{
					RelationKey: bundle.RelationKeyIsHidden.String(),
					Condition:   model.BlockContentDataviewFilter_NotEqual,
					Value:       pbtypes.Bool(true),
				},
			},
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyRelationKey.String(),
				bundle.RelationKeyApiObjectKey.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyRelationFormat.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			Records: []*types.Struct{},
		}, nil).Once()

		// when
		types, total, hasMore, err := fx.service.ListTypes(ctx, mockedSpaceId, nil, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, types, 1)
		require.Equal(t, "type-1", types[0].Id)
		require.Equal(t, "Type One", types[0].Name)
		require.Equal(t, "type_one_key", types[0].Key)
		require.Equal(t, &apimodel.Icon{
			WrappedIcon: apimodel.EmojiIcon{
				Format: apimodel.IconFormatEmoji,
				Emoji:  "🗂️",
			},
		}, types[0].Icon)
		require.Equal(t, 1, total)
		require.False(t, hasMore)
	})

	t.Run("no types found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{},
				Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		// Mock getPropertyMapFromStore
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: "empty-space",
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyResolvedLayout.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
				},
				{
					RelationKey: bundle.RelationKeyIsHidden.String(),
					Condition:   model.BlockContentDataviewFilter_NotEqual,
					Value:       pbtypes.Bool(true),
				},
			},
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyRelationKey.String(),
				bundle.RelationKeyApiObjectKey.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyRelationFormat.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			Records: []*types.Struct{},
		}, nil).Once()

		// when
		types, total, hasMore, err := fx.service.ListTypes(ctx, "empty-space", nil, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, types, 0)
		require.Equal(t, 0, total)
		require.False(t, hasMore)
	})
}

func TestObjectService_GetType(t *testing.T) {
	t.Run("type found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedTypeId,
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyId.String():                pbtypes.String(mockedTypeId),
								bundle.RelationKeyName.String():              pbtypes.String(mockedTypeName),
								bundle.RelationKeyUniqueKey.String():         pbtypes.String(mockedTypeKey),
								bundle.RelationKeyIconEmoji.String():         pbtypes.String(mockedTypeIcon),
								bundle.RelationKeyRecommendedLayout.String(): pbtypes.Float64(float64(model.ObjectType_basic)),
							},
						},
					},
				},
			},
		}).Once()

		// Mock getPropertyMapFromStore
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: mockedSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyResolvedLayout.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
				},
				{
					RelationKey: bundle.RelationKeyIsHidden.String(),
					Condition:   model.BlockContentDataviewFilter_NotEqual,
					Value:       pbtypes.Bool(true),
				},
			},
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyRelationKey.String(),
				bundle.RelationKeyApiObjectKey.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyRelationFormat.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			Records: []*types.Struct{},
		}, nil).Once()

		// when
		ot, err := fx.service.GetType(ctx, mockedSpaceId, mockedTypeId)

		// then
		require.NoError(t, err)
		require.Equal(t, mockedTypeId, ot.Id)
		require.Equal(t, mockedTypeName, ot.Name)
		require.Equal(t, mockedTypeKey, ot.Key)
		require.Equal(t, &apimodel.Icon{
			WrappedIcon: apimodel.EmojiIcon{
				Format: apimodel.IconFormatEmoji,
				Emoji:  mockedTypeIcon,
			},
		}, ot.Icon)
		require.Equal(t, apimodel.ObjectLayoutBasic, ot.Layout)
	})

	t.Run("type not found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedTypeId,
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NOT_FOUND},
		}).Once()

		// when
		ot, err := fx.service.GetType(ctx, mockedSpaceId, mockedTypeId)

		// then
		require.ErrorIs(t, err, ErrTypeNotFound)
		require.Empty(t, ot)
	})
}
