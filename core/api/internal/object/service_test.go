package object

import (
	"context"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/apicore/mock_apicore"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	offset              = 0
	limit               = 100
	mockedTechSpaceId   = "mocked-tech-space-id"
	gatewayUrl          = "http://localhost:31006"
	mockedSpaceId       = "mocked-space-id"
	mockedObjectId      = "mocked-object-id"
	mockedNewObjectId   = "mocked-new-object-id"
	mockedObjectName    = "mocked-object-name"
	mockedObjectSnippet = "mocked-object-snippet"
	mockedObjectIcon    = "🔍"
	mockedParticipantId = "mocked-participant-id"
	mockedTypeKey       = "ot-page"
	mockedTypeId        = "mocked-type-id"
	mockedTypeName      = "mocked-type-name"
	mockedTypeIcon      = "📝"
	mockedTemplateId    = "mocked-template-id"
	mockedTemplateName  = "mocked-template-name"
	mockedTemplateIcon  = "📃"
)

type fixture struct {
	service Service
	mwMock  *mock_apicore.MockClientCommands
}

func newFixture(t *testing.T) *fixture {
	mwMock := mock_apicore.NewMockClientCommands(t)
	objectService := NewService(mwMock, gatewayUrl)

	return &fixture{
		service: objectService,
		mwMock:  mwMock,
	}
}

func TestObjectService_ListObjects(t *testing.T) {
	t.Run("successfully get objects for a space", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// Mock object search
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: mockedSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyResolvedLayout.String(),
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

		// Expect the ObjectSearch call to get the relation format for the relation key.
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
			Keys: []string{bundle.RelationKeyUniqueKey.String(), bundle.RelationKeyRelationFormat.String()},
		}).Return(&pb.RpcObjectSearchResponse{
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyUniqueKey.String():      pbtypes.String(bundle.RelationKeyCreatedDate.String()),
						bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(model.RelationFormat_date)),
					},
				},
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyUniqueKey.String():      pbtypes.String(bundle.RelationKeyCreator.String()),
						bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(model.RelationFormat_object)),
					},
				},
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyUniqueKey.String():      pbtypes.String(bundle.RelationKeyLastModifiedDate.String()),
						bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(model.RelationFormat_date)),
					},
				},
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyUniqueKey.String():      pbtypes.String(bundle.RelationKeyLastModifiedBy.String()),
						bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(model.RelationFormat_object)),
					},
				},
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyUniqueKey.String():      pbtypes.String(bundle.RelationKeyLastOpenedDate.String()),
						bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(model.RelationFormat_date)),
					},
				},
			},
		}, nil).Once()

		// Expect the ObjectSearch call to get the type map.
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: mockedSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyResolvedLayout.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.ObjectType_objectType)),
				},
				{
					RelationKey: bundle.RelationKeyIsDeleted.String(),
				},
			},
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyUniqueKey.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyIconEmoji.String(),
				bundle.RelationKeyIconName.String(),
				bundle.RelationKeyIconOption.String(),
				bundle.RelationKeyRecommendedLayout.String(),
				bundle.RelationKeyIsArchived.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():                pbtypes.String(mockedTypeId),
						bundle.RelationKeyUniqueKey.String():         pbtypes.String(mockedTypeKey),
						bundle.RelationKeyName.String():              pbtypes.String(mockedTypeName),
						bundle.RelationKeyIconEmoji.String():         pbtypes.String(mockedTypeIcon),
						bundle.RelationKeyIconName.String():          pbtypes.String(""),
						bundle.RelationKeyIconOption.String():        pbtypes.String("option1"),
						bundle.RelationKeyRecommendedLayout.String(): pbtypes.Float64(float64(model.ObjectType_basic)),
						bundle.RelationKeyIsArchived.String():        pbtypes.Bool(false),
					},
				},
			},
		}, nil).Once()

		// when
		objects, total, hasMore, err := fx.service.ListObjects(ctx, mockedSpaceId, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, objects, 1)
		require.Equal(t, mockedTypeId, objects[0].Type.Id)
		require.Equal(t, mockedTypeName, objects[0].Type.Name)
		require.Equal(t, mockedTypeKey, objects[0].Type.Key)
		require.Equal(t, util.Icon{Format: "emoji", Emoji: util.StringPtr(mockedTypeIcon)}, objects[0].Type.Icon)
		require.Equal(t, mockedObjectId, objects[0].Id)
		require.Equal(t, mockedObjectName, objects[0].Name)
		require.Equal(t, mockedObjectSnippet, objects[0].Snippet)
		require.Equal(t, util.Icon{Format: "emoji", Emoji: util.StringPtr(mockedObjectIcon)}, objects[0].Icon)
		require.Equal(t, 5, len(objects[0].Properties))

		for _, detail := range objects[0].Properties {
			if detail.Id == "created_date" {
				require.Equal(t, "1970-01-11T06:54:48Z", *detail.Date)
			} else if detail.Id == "created_by" {
				require.Equal(t, []string{mockedParticipantId}, detail.Object)
			} else if detail.Id == "last_modified_date" {
				require.Equal(t, "1970-01-12T13:46:39Z", *detail.Date)
			} else if detail.Id == "last_modified_by" {
				require.Equal(t, []string{mockedParticipantId}, detail.Object)
			} else if detail.Id == "last_opened_date" {
				require.Equal(t, "1970-01-01T00:00:00Z", *detail.Date)
			} else if detail.Id == "tag" {
				require.Empty(t, detail.MultiSelect)
			} else {
				t.Errorf("unexpected detail id: %s", detail.Id)
			}
		}

		require.Equal(t, 1, total)
		require.False(t, hasMore)
	})

	t.Run("no objects found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// Mock object search
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: mockedSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyResolvedLayout.String(),
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

		// Mock property and type map search
		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{},
			Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Times(2)

		// when
		objects, total, hasMore, err := fx.service.ListObjects(ctx, mockedSpaceId, offset, limit)

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
					RelationLinks: []*model.RelationLink{
						{
							Key:    bundle.RelationKeyLastModifiedDate.String(),
							Format: model.RelationFormat_date,
						},
						{
							Key:    bundle.RelationKeyLastModifiedBy.String(),
							Format: model.RelationFormat_object,
						},
						{
							Key:    bundle.RelationKeyCreatedDate.String(),
							Format: model.RelationFormat_date,
						},
						{
							Key:    bundle.RelationKeyCreator.String(),
							Format: model.RelationFormat_object,
						},
						{
							Key:    bundle.RelationKeyLastOpenedDate.String(),
							Format: model.RelationFormat_date,
						},
						{
							Key:    bundle.RelationKeyTag.String(),
							Format: model.RelationFormat_tag,
						},
					},
				},
			}, nil).Once()

		// when
		object, err := fx.service.GetObject(ctx, mockedSpaceId, mockedObjectId)

		// then
		require.NoError(t, err)
		require.Equal(t, "object", object.Object)
		require.Equal(t, mockedTypeId, object.Type.Id)
		require.Equal(t, mockedTypeName, object.Type.Name)
		require.Equal(t, mockedTypeKey, object.Type.Key)
		require.Equal(t, util.Icon{Format: "emoji", Emoji: util.StringPtr(mockedTypeIcon)}, object.Type.Icon)
		require.Equal(t, mockedObjectId, object.Id)
		require.Equal(t, mockedObjectName, object.Name)
		require.Equal(t, mockedObjectSnippet, object.Snippet)
		require.Equal(t, util.Icon{Format: "emoji", Emoji: util.StringPtr(mockedObjectIcon)}, object.Icon)
		require.Equal(t, 3, len(object.Properties))

		for _, property := range object.Properties {
			if property.Id == "created_date" {
				require.Equal(t, "1970-01-11T06:54:48Z", *property.Date)
			} else if property.Id == "created_by" {
				require.Empty(t, property.Object)
			} else if property.Id == "last_modified_date" {
				require.Equal(t, "1970-01-12T13:46:39Z", *property.Date)
			} else if property.Id == "last_modified_by" {
				require.Empty(t, property.Object)
			} else if property.Id == "last_opened_date" {
				require.Equal(t, "1970-01-01T00:00:00Z", *property.Date)
			} else if property.Id == "tag" {
				require.Empty(t, property.MultiSelect)
			} else {
				t.Errorf("unexpected property id: %s", property.Id)
			}
		}
	})

	t.Run("object not found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

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

		fx.mwMock.On("ObjectCreate", mock.Anything, &pb.RpcObjectCreateRequest{
			Details: &types.Struct{
				Fields: map[string]*types.Value{
					bundle.RelationKeyName.String():        pbtypes.String(mockedObjectName),
					bundle.RelationKeyIconEmoji.String():   pbtypes.String(mockedObjectIcon),
					bundle.RelationKeyDescription.String(): pbtypes.String(""),
					bundle.RelationKeySource.String():      pbtypes.String(""),
					bundle.RelationKeyOrigin.String():      pbtypes.Int64(int64(model.ObjectOrigin_api)),
				},
			},
			TemplateId:          mockedTemplateId,
			SpaceId:             mockedSpaceId,
			ObjectTypeUniqueKey: mockedTypeKey,
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

		// when
		object, err := fx.service.CreateObject(ctx, mockedSpaceId, CreateObjectRequest{
			Name:       mockedObjectName,
			Icon:       util.Icon{Format: util.IconFormatEmoji, Emoji: util.StringPtr(mockedObjectIcon)},
			TemplateId: mockedTemplateId,
			TypeKey:    mockedTypeKey,
		})

		// then
		require.NoError(t, err)
		require.Equal(t, "object", object.Object)
		require.Equal(t, mockedTypeId, object.Type.Id)
		require.Equal(t, mockedTypeName, object.Type.Name)
		require.Equal(t, mockedTypeKey, object.Type.Key)
		require.Equal(t, util.Icon{Format: "emoji", Emoji: util.StringPtr(mockedTypeIcon)}, object.Type.Icon)
		require.Equal(t, mockedNewObjectId, object.Id)
		require.Equal(t, mockedObjectName, object.Name)
		require.Equal(t, util.Icon{Format: "emoji", Emoji: util.StringPtr(mockedObjectIcon)}, object.Icon)
		require.Equal(t, mockedSpaceId, object.SpaceId)
	})

	t.Run("creation error", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("ObjectCreate", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectCreateResponse{
				Error: &pb.RpcObjectCreateResponseError{Code: pb.RpcObjectCreateResponseError_UNKNOWN_ERROR},
			}).Once()

		// when
		object, err := fx.service.CreateObject(ctx, mockedSpaceId, CreateObjectRequest{
			Name: "Fail Object",
			Icon: util.Icon{},
		})

		// then
		require.ErrorIs(t, err, ErrFailedCreateObject)
		require.Empty(t, object)
	})
}

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

		// when
		types, total, hasMore, err := fx.service.ListTypes(ctx, mockedSpaceId, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, types, 1)
		require.Equal(t, "type-1", types[0].Id)
		require.Equal(t, "Type One", types[0].Name)
		require.Equal(t, "type-one-key", types[0].Key)
		require.Equal(t, util.Icon{Format: "emoji", Emoji: util.StringPtr("🗂️")}, types[0].Icon)
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

		// when
		types, total, hasMore, err := fx.service.ListTypes(ctx, "empty-space", offset, limit)

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

		// when
		objType, err := fx.service.GetType(ctx, mockedSpaceId, mockedTypeId)

		// then
		require.NoError(t, err)
		require.Equal(t, mockedTypeId, objType.Id)
		require.Equal(t, mockedTypeName, objType.Name)
		require.Equal(t, mockedTypeKey, objType.Key)
		require.Equal(t, util.Icon{Format: "emoji", Emoji: util.StringPtr(mockedTypeIcon)}, objType.Icon)
		require.Equal(t, model.ObjectTypeLayout_name[int32(model.ObjectType_basic)], objType.Layout)
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
		objType, err := fx.service.GetType(ctx, mockedSpaceId, mockedTypeId)

		// then
		require.ErrorIs(t, err, ErrTypeNotFound)
		require.Empty(t, objType)
	})
}

func TestObjectService_ListTemplates(t *testing.T) {
	t.Run("templates found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

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
		templates, total, hasMore, err := fx.service.ListTemplates(ctx, mockedSpaceId, "target-type-id", offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, templates, 1)
		require.Equal(t, "template-1", templates[0].Id)
		require.Equal(t, "Template Name", templates[0].Name)
		require.Equal(t, util.Icon{Format: "emoji", Emoji: util.StringPtr("📝")}, templates[0].Icon)
		require.Equal(t, 1, total)
		require.False(t, hasMore)
	})

	t.Run("no template type found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{},
				Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		// when
		templates, total, hasMore, err := fx.service.ListTemplates(ctx, mockedSpaceId, "missing-type-id", offset, limit)

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

		// when
		template, err := fx.service.GetTemplate(ctx, mockedSpaceId, mockedTypeId, mockedTemplateId)

		// then
		require.NoError(t, err)
		require.Equal(t, mockedTemplateId, template.Id)
		require.Equal(t, mockedTemplateName, template.Name)
		require.Equal(t, util.Icon{Format: "emoji", Emoji: util.StringPtr(mockedTemplateIcon)}, template.Icon)
	})

	t.Run("template not found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

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
