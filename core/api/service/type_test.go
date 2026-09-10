package service

import (
	"context"
	"strings"
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

func TestObjectService_ListTypes(t *testing.T) {
	t.Run("types found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

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
		fx.populateCache("empty-space")

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{},
				Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

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
		fx.populateCache(mockedSpaceId)

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
		fx.populateCache(mockedSpaceId)

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

// The key a caller supplies is MINTED before it is stored: what lands in
// apiObjectKey is the address every later request must use, and snake-casing
// alone leaves punctuation in place. Measured over a 38,123-object account,
// 27 of 1,530 stored api keys sat outside the key grammar the api
// advertises.
func TestService_buildTypeDetails_KeyIsMinted(t *testing.T) {
	t.Run("a key outside the grammar is converted", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		request := apimodel.CreateTypeRequest{
			Key:  "Manual export & import",
			Name: "Manual export",
		}

		// when
		details, err := fx.service.buildTypeDetails(ctx, mockedSpaceId, request)

		// then
		require.NoError(t, err)
		assert.Equal(t, "manual_export_import", details.Fields[bundle.RelationKeyApiObjectKey.String()].GetStringValue())
	})

	t.Run("a key longer than a key may be is bounded", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		request := apimodel.CreateTypeRequest{
			Key:  strings.Repeat("k", 300),
			Name: "Long",
		}

		// when
		details, err := fx.service.buildTypeDetails(ctx, mockedSpaceId, request)

		// then
		require.NoError(t, err)
		assert.Len(t, details.Fields[bundle.RelationKeyApiObjectKey.String()].GetStringValue(), bundle.MaxApiSlugLen)
	})

	t.Run("a key with nothing the grammar admits is refused", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		request := apimodel.CreateTypeRequest{
			Key:  "Задача",
			Name: "Task",
		}

		// when
		details, err := fx.service.buildTypeDetails(ctx, mockedSpaceId, request)

		// then
		require.ErrorIs(t, err, util.ErrBad)
		assert.Nil(t, details)
	})
}

func TestService_buildUpdatedTypeDetails_KeyIsMinted(t *testing.T) {
	existing := &apimodel.Type{
		Id:        mockedTypeId,
		Key:       "custom_type_key",
		UniqueKey: "ot-67b0d3e3cda913b84c1299b1",
	}

	t.Run("a key outside the grammar is converted", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		key := "Lists [in work]"
		request := apimodel.UpdateTypeRequest{Key: &key}

		// when
		details, err := fx.service.buildUpdatedTypeDetails(ctx, mockedSpaceId, existing, request)

		// then
		require.NoError(t, err)
		assert.Equal(t, "lists_in_work", details.Fields[bundle.RelationKeyApiObjectKey.String()].GetStringValue())
	})

	t.Run("a key with nothing the grammar admits is refused", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		key := "➡️"
		request := apimodel.UpdateTypeRequest{Key: &key}

		// when
		details, err := fx.service.buildUpdatedTypeDetails(ctx, mockedSpaceId, existing, request)

		// then
		require.ErrorIs(t, err, util.ErrBad)
		assert.Nil(t, details)
	})
}
