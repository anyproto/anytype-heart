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

func TestObjectService_ListTemplates(t *testing.T) {
	t.Run("templates found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		// Mock template type search
		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():        pbtypes.String("template-type-id"),
						bundle.RelationKeyUniqueKey.String(): pbtypes.String("ot-template"),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// Mock actual template objects search
		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():               pbtypes.String("template-1"),
						bundle.RelationKeyTargetObjectType.String(): pbtypes.String("target-type-id"),
						bundle.RelationKeyName.String():             pbtypes.String("Template Name"),
						bundle.RelationKeyIconEmoji.String():        pbtypes.String("📝"),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// when
		templates, total, hasMore, err := fx.service.ListTemplates(ctx, mockedSpaceId, "target-type-id", nil, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, templates, 1)
		require.Equal(t, "template-1", templates[0].Id)
		require.Equal(t, "Template Name", templates[0].Name)
		require.Equal(t, &apimodel.Icon{
			WrappedIcon: apimodel.EmojiIcon{
				Format: apimodel.IconFormatEmoji,
				Emoji:  "📝",
			},
		}, templates[0].Icon)
		require.Equal(t, 1, total)
		require.False(t, hasMore)
	})

	t.Run("no template type found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{},
				Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		// when
		templates, total, hasMore, err := fx.service.ListTemplates(ctx, mockedSpaceId, "missing-type-id", nil, offset, limit)

		// then
		require.ErrorIs(t, err, ErrTemplateTypeNotFound)
		require.Len(t, templates, 0)
		require.Equal(t, 0, total)
		require.False(t, hasMore)
	})
}

func TestObjectService_GetTemplate(t *testing.T) {
	t.Run("template found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedTemplateId,
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyId.String():        pbtypes.String(mockedTemplateId),
								bundle.RelationKeyName.String():      pbtypes.String(mockedTemplateName),
								bundle.RelationKeyIconEmoji.String(): pbtypes.String(mockedTemplateIcon),
							},
						},
					},
				},
			},
		}).Once()

		// Mock ExportMarkdown
		fx.mwMock.On("ObjectExport", mock.Anything, &pb.RpcObjectExportRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedTemplateId,
			Format:   model.Export_Markdown,
		}).Return(&pb.RpcObjectExportResponse{
			Result: "dummy markdown",
			Error:  &pb.RpcObjectExportResponseError{Code: pb.RpcObjectExportResponseError_NULL},
		}, nil).Once()

		// when
		template, err := fx.service.GetTemplate(ctx, mockedSpaceId, mockedTypeId, mockedTemplateId)

		// then
		require.NoError(t, err)
		require.Equal(t, mockedTemplateId, template.Id)
		require.Equal(t, mockedTemplateName, template.Name)
		require.Equal(t, &apimodel.Icon{
			WrappedIcon: apimodel.EmojiIcon{
				Format: apimodel.IconFormatEmoji,
				Emoji:  mockedTemplateIcon,
			},
		}, template.Icon)
	})

	t.Run("template not found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedTemplateId,
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NOT_FOUND},
		}).Once()

		// when
		template, err := fx.service.GetTemplate(ctx, mockedSpaceId, mockedTypeId, mockedTemplateId)

		// then
		require.ErrorIs(t, err, ErrTemplateNotFound)
		require.Empty(t, template)
	})
}
