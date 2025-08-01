package service

import (
	"context"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestObjectService_ListObjects(t *testing.T) {
	t.Run("successfully get objects for a space", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		// Mock object search
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: mockedSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyResolvedLayout.String(),
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       pbtypes.IntList(util.LayoutsToIntArgs(util.ObjectLayouts)...),
				},
				{
					RelationKey: "type.uniqueKey",
					Condition:   model.BlockContentDataviewFilter_NotEqual,
					Value:       pbtypes.String("ot-template"),
				},
				{
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
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():               pbtypes.String(mockedObjectId),
						bundle.RelationKeyName.String():             pbtypes.String(mockedObjectName),
						bundle.RelationKeySnippet.String():          pbtypes.String(mockedObjectSnippet),
						bundle.RelationKeyIconEmoji.String():        pbtypes.String(mockedObjectIcon),
						bundle.RelationKeyType.String():             pbtypes.String(mockedTypeId),
						bundle.RelationKeyResolvedLayout.String():   pbtypes.Float64(float64(model.ObjectType_basic)),
						bundle.RelationKeyCreatedDate.String():      pbtypes.Float64(888888),
						bundle.RelationKeyLastModifiedBy.String():   pbtypes.String(mockedParticipantId),
						bundle.RelationKeyLastModifiedDate.String(): pbtypes.Float64(999999),
						bundle.RelationKeyCreator.String():          pbtypes.String(mockedParticipantId),
						bundle.RelationKeyLastOpenedDate.String():   pbtypes.Float64(0),
						bundle.RelationKeySpaceId.String():          pbtypes.String(mockedSpaceId),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// when
		objects, total, hasMore, err := fx.service.ListObjects(ctx, mockedSpaceId, nil, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, objects, 1)
		require.Equal(t, mockedTypeId, objects[0].Type.Id)
		require.Equal(t, mockedTypeName, objects[0].Type.Name)
		require.Equal(t, mockedTypeKey, objects[0].Type.Key)
		require.Equal(t, &apimodel.Icon{
			WrappedIcon: apimodel.EmojiIcon{
				Format: apimodel.IconFormatEmoji,
				Emoji:  mockedTypeIcon,
			},
		}, objects[0].Type.Icon)

		require.Equal(t, mockedObjectId, objects[0].Id)
		require.Equal(t, mockedObjectName, objects[0].Name)
		require.Equal(t, mockedObjectSnippet, objects[0].Snippet)
		require.Equal(t, &apimodel.Icon{
			WrappedIcon: apimodel.EmojiIcon{
				Format: apimodel.IconFormatEmoji,
				Emoji:  mockedObjectIcon,
			},
		}, objects[0].Icon)

		require.Equal(t, 5, len(objects[0].Properties))

		for _, propResp := range objects[0].Properties {
			switch v := propResp.WrappedPropertyWithValue.(type) {
			case apimodel.DatePropertyValue:
				switch v.Key {
				case "created_date":
					require.Equal(t, "1970-01-11T06:54:48Z", v.Date)
				case "last_modified_date":
					require.Equal(t, "1970-01-12T13:46:39Z", v.Date)
				case "last_opened_date":
					require.Equal(t, "1970-01-01T00:00:00Z", v.Date)
				}
			case apimodel.ObjectsPropertyValue:
				switch v.Key {
				case "creator":
					require.Equal(t, []string{mockedParticipantId}, v.Objects)
				case "last_modified_by":
					require.Equal(t, []string{mockedParticipantId}, v.Objects)
				}
			default:
				t.Errorf("unexpected property type: %T", v)
			}
		}

		require.Equal(t, 1, total)
		require.False(t, hasMore)
	})

	t.Run("no objects found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		// Mock object search
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: mockedSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyResolvedLayout.String(),
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       pbtypes.IntList(util.LayoutsToIntArgs(util.ObjectLayouts)...),
				},
				{
					RelationKey: "type.uniqueKey",
					Condition:   model.BlockContentDataviewFilter_NotEqual,
					Value:       pbtypes.String("ot-template"),
				},
				{
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
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{},
			Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// when
		objects, total, hasMore, err := fx.service.ListObjects(ctx, mockedSpaceId, nil, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, objects, 0)
		require.Equal(t, 0, total)
		require.False(t, hasMore)
	})
}

func TestObjectService_GetObject(t *testing.T) {
	t.Run("object found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedObjectId,
		}).
			Return(&pb.RpcObjectShowResponse{
				Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
				ObjectView: &model.ObjectView{
					RootId: mockedObjectId,
					Details: []*model.ObjectViewDetailsSet{
						{
							Id: mockedObjectId,
							Details: &types.Struct{
								Fields: map[string]*types.Value{
									bundle.RelationKeyId.String():               pbtypes.String(mockedObjectId),
									bundle.RelationKeyName.String():             pbtypes.String(mockedObjectName),
									bundle.RelationKeySnippet.String():          pbtypes.String(mockedObjectSnippet),
									bundle.RelationKeyIconEmoji.String():        pbtypes.String(mockedObjectIcon),
									bundle.RelationKeyType.String():             pbtypes.String(mockedTypeId),
									bundle.RelationKeyLastModifiedDate.String(): pbtypes.Float64(999999),
									bundle.RelationKeyCreatedDate.String():      pbtypes.Float64(888888),
									bundle.RelationKeyLastOpenedDate.String():   pbtypes.Float64(0),
									bundle.RelationKeySpaceId.String():          pbtypes.String(mockedSpaceId),
								},
							},
						},
						{
							Id: mockedTypeId,
							Details: &types.Struct{
								Fields: map[string]*types.Value{
									bundle.RelationKeyId.String():        pbtypes.String(mockedTypeId),
									bundle.RelationKeyName.String():      pbtypes.String(mockedTypeName),
									bundle.RelationKeyUniqueKey.String(): pbtypes.String(mockedTypeKey),
									bundle.RelationKeyIconEmoji.String(): pbtypes.String(mockedTypeIcon),
								},
							},
						},
					},
				},
			}, nil).Once()

		// Mock ExportMarkdown
		fx.mwMock.On("ObjectExport", mock.Anything, &pb.RpcObjectExportRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedObjectId,
			Format:   model.Export_Markdown,
		}).Return(&pb.RpcObjectExportResponse{
			Result: "dummy markdown",
			Error:  &pb.RpcObjectExportResponseError{Code: pb.RpcObjectExportResponseError_NULL},
		}, nil).Once()

		// when
		object, err := fx.service.GetObject(ctx, mockedSpaceId, mockedObjectId)

		// then
		require.NoError(t, err)
		require.Equal(t, "object", object.Object)
		require.Equal(t, mockedTypeId, object.Type.Id)
		require.Equal(t, mockedTypeName, object.Type.Name)
		require.Equal(t, mockedTypeKey, object.Type.Key)
		require.Equal(t, &apimodel.Icon{
			WrappedIcon: apimodel.EmojiIcon{
				Format: apimodel.IconFormatEmoji,
				Emoji:  mockedTypeIcon,
			},
		}, object.Type.Icon)

		require.Equal(t, mockedObjectId, object.Id)
		require.Equal(t, mockedObjectName, object.Name)
		require.Equal(t, mockedObjectSnippet, object.Snippet)
		require.Equal(t, &apimodel.Icon{
			WrappedIcon: apimodel.EmojiIcon{
				Format: apimodel.IconFormatEmoji,
				Emoji:  mockedObjectIcon,
			},
		}, object.Icon)

		require.Equal(t, 3, len(object.Properties))

		for _, propResp := range object.Properties {
			switch v := propResp.WrappedPropertyWithValue.(type) {
			case apimodel.DatePropertyValue:
				switch v.Key {
				case "created_date":
					require.Equal(t, "1970-01-11T06:54:48Z", v.Date)
				case "last_modified_date":
					require.Equal(t, "1970-01-12T13:46:39Z", v.Date)
				case "last_opened_date":
					require.Equal(t, "1970-01-01T00:00:00Z", v.Date)
				}
			case apimodel.ObjectsPropertyValue:
				switch v.Key {
				case "creator":
					require.Equal(t, []string{mockedParticipantId}, v.Objects)
				case "last_modified_by":
					require.Equal(t, []string{mockedParticipantId}, v.Objects)
				}
			default:
				t.Errorf("unexpected property type: %T", v)
			}
		}
	})

	t.Run("object not found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		fx.mwMock.On("ObjectShow", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectShowResponse{
				Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NOT_FOUND},
			}, nil).Once()

		// when
		object, err := fx.service.GetObject(ctx, mockedSpaceId, "missing-obj")

		// then
		require.ErrorIs(t, err, ErrObjectNotFound)
		require.Empty(t, object)
	})
}

func TestObjectService_CreateObject(t *testing.T) {
	t.Run("successful object creation", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		fx.mwMock.On("ObjectCreate", mock.Anything, &pb.RpcObjectCreateRequest{
			Details: &types.Struct{
				Fields: map[string]*types.Value{
					bundle.RelationKeyName.String():      pbtypes.String(mockedObjectName),
					bundle.RelationKeyIconEmoji.String(): pbtypes.String(mockedObjectIcon),
					bundle.RelationKeyOrigin.String():    pbtypes.Int64(int64(model.ObjectOrigin_api)),
				},
			},
			TemplateId:          mockedTemplateId,
			SpaceId:             mockedSpaceId,
			ObjectTypeUniqueKey: "ot-" + mockedTypeKey,
			WithChat:            false,
		}).Return(&pb.RpcObjectCreateResponse{
			ObjectId: mockedNewObjectId,
			Details: &types.Struct{
				Fields: map[string]*types.Value{
					bundle.RelationKeyId.String():        pbtypes.String(mockedNewObjectId),
					bundle.RelationKeyName.String():      pbtypes.String(mockedObjectName),
					bundle.RelationKeyIconEmoji.String(): pbtypes.String(mockedObjectIcon),
					bundle.RelationKeySpaceId.String():   pbtypes.String(mockedSpaceId),
				},
			},
			Error: &pb.RpcObjectCreateResponseError{Code: pb.RpcObjectCreateResponseError_NULL},
		}).Once()

		// Mock object show for object details
		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedNewObjectId,
		}).Return(&pb.RpcObjectShowResponse{
			ObjectView: &model.ObjectView{
				RootId: mockedNewObjectId,
				Details: []*model.ObjectViewDetailsSet{
					{
						Id: mockedNewObjectId,
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyId.String():             pbtypes.String(mockedNewObjectId),
								bundle.RelationKeyName.String():           pbtypes.String(mockedObjectName),
								bundle.RelationKeyResolvedLayout.String(): pbtypes.Float64(float64(model.ObjectType_basic)),
								bundle.RelationKeyType.String():           pbtypes.String(mockedTypeId),
								bundle.RelationKeyIconEmoji.String():      pbtypes.String(mockedObjectIcon),
								bundle.RelationKeySpaceId.String():        pbtypes.String(mockedSpaceId),
							},
						},
					},
					{
						Id: mockedTypeId,
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyId.String():        pbtypes.String(mockedTypeId),
								bundle.RelationKeyName.String():      pbtypes.String(mockedTypeName),
								bundle.RelationKeyUniqueKey.String(): pbtypes.String(mockedTypeKey),
								bundle.RelationKeyIconEmoji.String(): pbtypes.String(mockedTypeIcon),
							},
						},
					},
				},
			},
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
		}).Once()

		// Mock ExportMarkdown
		fx.mwMock.On("ObjectExport", mock.Anything, &pb.RpcObjectExportRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedNewObjectId,
			Format:   model.Export_Markdown,
		}).Return(&pb.RpcObjectExportResponse{
			Result: "dummy markdown",
			Error:  &pb.RpcObjectExportResponseError{Code: pb.RpcObjectExportResponseError_NULL},
		}, nil).Once()

		// when
		object, err := fx.service.CreateObject(ctx, mockedSpaceId, apimodel.CreateObjectRequest{
			Name:       mockedObjectName,
			Icon:       apimodel.Icon{WrappedIcon: apimodel.EmojiIcon{Format: apimodel.IconFormatEmoji, Emoji: mockedObjectIcon}},
			TemplateId: mockedTemplateId,
			TypeKey:    mockedTypeKey,
		})

		// then
		require.NoError(t, err)
		require.Equal(t, "object", object.Object)
		require.Equal(t, mockedNewObjectId, object.Id)
		require.Equal(t, mockedObjectName, object.Name)
		require.Equal(t, &apimodel.Icon{WrappedIcon: apimodel.EmojiIcon{Format: apimodel.IconFormatEmoji, Emoji: mockedObjectIcon}}, object.Icon)
		require.Equal(t, mockedSpaceId, object.SpaceId)
	})

	t.Run("creation error", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		fx.mwMock.On("ObjectCreate", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectCreateResponse{
				Error: &pb.RpcObjectCreateResponseError{Code: pb.RpcObjectCreateResponseError_UNKNOWN_ERROR},
			}).Once()

		// when
		object, err := fx.service.CreateObject(ctx, mockedSpaceId, apimodel.CreateObjectRequest{
			Name: "Fail Object",
			Icon: apimodel.Icon{},
		})

		// then
		require.ErrorIs(t, err, ErrFailedCreateObject)
		require.Empty(t, object)
	})
}
