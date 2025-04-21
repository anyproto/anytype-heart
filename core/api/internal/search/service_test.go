package search

import (
	"context"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/apicore/mock_apicore"
	"github.com/anyproto/anytype-heart/core/api/internal/object"
	"github.com/anyproto/anytype-heart/core/api/internal/space"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	offset              = 0
	limit               = 100
	techSpaceId         = "tech-space-id"
	gatewayUrl          = "http://localhost:31006"
	mockedSpaceId       = "mocked-space-id"
	mockedSearchTerm    = "mocked-search-term"
	mockedObjectId      = "mocked-object-id"
	mockedObjectName    = "mocked-object-name"
	mockedObjectIcon    = "🌐"
	mockedParticipantId = "mocked-participant-id"
	mockedTypeId        = "mocked-type-id"
	mockedTagId1        = "mocked-tag-id-1"
	mockedTagValue1     = "mocked-tag-value-1"
	mockedTagColor1     = "red"
	mockedTagId2        = "mocked-tag-id-2"
	mockedTagValue2     = "mocked-tag-value-2"
	mockedTagColor2     = "blue"
)

type fixture struct {
	service Service
	mwMock  *mock_apicore.MockClientCommands
}

func newFixture(t *testing.T) *fixture {
	mwMock := mock_apicore.NewMockClientCommands(t)
	exportMock := mock_apicore.NewMockExportService(t)
	spaceService := space.NewService(mwMock, gatewayUrl, techSpaceId)
	objectService := object.NewService(mwMock, exportMock, gatewayUrl)
	searchService := NewService(mwMock, spaceService, objectService)

	return &fixture{
		service: searchService,
		mwMock:  mwMock,
	}
}

func TestSearchService_GlobalSearch(t *testing.T) {
	t.Run("objects found globally", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// Mock retrieving spaces first
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: techSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyResolvedLayout.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
				},
				{
					RelationKey: bundle.RelationKeySpaceLocalStatus.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.SpaceStatus_Ok)),
				},
			},
			Keys: []string{bundle.RelationKeyTargetSpaceId.String()},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyTargetSpaceId.String(): pbtypes.String(mockedSpaceId),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// Mock objects in space
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: mockedSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					Operator: model.BlockContentDataviewFilter_And,
					NestedFilters: []*model.BlockContentDataviewFilter{
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
							RelationKey: bundle.RelationKeyIsHidden.String(),
							Condition:   model.BlockContentDataviewFilter_NotEqual,
							Value:       pbtypes.Bool(true),
						},
						{
							RelationKey: "type.uniqueKey",
							Condition:   model.BlockContentDataviewFilter_NotEqual,
							Value:       pbtypes.String("ot-template"),
						},
						{
							Operator: model.BlockContentDataviewFilter_Or,
							NestedFilters: []*model.BlockContentDataviewFilter{
								{
									RelationKey: bundle.RelationKeyName.String(),
									Condition:   model.BlockContentDataviewFilter_Like,
									Value:       pbtypes.String(mockedSearchTerm),
								},
								{
									RelationKey: bundle.RelationKeySnippet.String(),
									Condition:   model.BlockContentDataviewFilter_Like,
									Value:       pbtypes.String(mockedSearchTerm),
								},
							},
						},
					},
				},
			},
			Sorts: []*model.BlockContentDataviewSort{{
				RelationKey:    bundle.RelationKeyLastModifiedDate.String(),
				Type:           model.BlockContentDataviewSort_Desc,
				Format:         model.RelationFormat_date,
				IncludeTime:    true,
				EmptyPlacement: model.BlockContentDataviewSort_NotSpecified,
			}},
			Limit: int32(offset + limit),
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():               pbtypes.String(mockedObjectId),
						bundle.RelationKeyName.String():             pbtypes.String(mockedObjectName),
						bundle.RelationKeyIconEmoji.String():        pbtypes.String(mockedObjectIcon),
						bundle.RelationKeyType.String():             pbtypes.String(mockedTypeId),
						bundle.RelationKeyResolvedLayout.String():   pbtypes.Float64(float64(model.ObjectType_basic)),
						bundle.RelationKeyCreatedDate.String():      pbtypes.Float64(888888),
						bundle.RelationKeyLastModifiedBy.String():   pbtypes.String(mockedParticipantId),
						bundle.RelationKeyLastModifiedDate.String(): pbtypes.Float64(999999),
						bundle.RelationKeyCreator.String():          pbtypes.String(mockedParticipantId),
						bundle.RelationKeyLastOpenedDate.String():   pbtypes.Float64(0),
						bundle.RelationKeySpaceId.String():          pbtypes.String(mockedSpaceId),
						bundle.RelationKeyTag.String():              pbtypes.StringList([]string{mockedTagId1, mockedTagId2}),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// Mock GetPropertyMapsFromStore
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
				bundle.RelationKeyUniqueKey.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyRelationFormat.String(),
			},
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
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyUniqueKey.String():      pbtypes.String(bundle.RelationKeyTag.String()),
						bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(model.RelationFormat_tag)),
					},
				},
			},
		}, nil).Once()

		// Mock GetTypeMapsFromStore
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
				bundle.RelationKeyRecommendedFeaturedRelations.String(),
				bundle.RelationKeyRecommendedRelations.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String(): pbtypes.String(mockedTypeId),
					},
				},
			},
		}, nil).Once()

		// Mock tag-1 open
		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedTagId1,
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyId.String():                  pbtypes.String(mockedTagId1),
								bundle.RelationKeyName.String():                pbtypes.String(mockedTagValue1),
								bundle.RelationKeyRelationOptionColor.String(): pbtypes.String(mockedTagColor1),
							},
						},
					},
				},
			},
		}, nil).Once()

		// Mock tag-2 open
		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedTagId2,
		}).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyId.String():                  pbtypes.String(mockedTagId2),
								bundle.RelationKeyName.String():                pbtypes.String(mockedTagValue2),
								bundle.RelationKeyRelationOptionColor.String(): pbtypes.String(mockedTagColor2),
							},
						},
					},
				},
			},
		}, nil).Once()

		// when
		objects, total, hasMore, err := fx.service.GlobalSearch(ctx, SearchRequest{Query: mockedSearchTerm, Types: []string{}, Sort: SortOptions{Property: LastModifiedDate, Direction: Desc}}, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, objects, 1)
		require.Equal(t, mockedObjectId, objects[0].Id)
		require.Equal(t, mockedObjectName, objects[0].Name)
		require.Equal(t, mockedTypeId, objects[0].Type.Id)
		require.Equal(t, mockedSpaceId, objects[0].SpaceId)
		require.Equal(t, model.ObjectTypeLayout_name[int32(model.ObjectType_basic)], objects[0].Layout)
		require.Equal(t, object.Icon{Format: "emoji", Emoji: object.StringPtr(mockedObjectIcon)}, objects[0].Icon)

		// check details
		for _, property := range objects[0].Properties {
			if property.Id == "created_date" {
				require.Equal(t, "1970-01-11T06:54:48Z", *property.Date)
			} else if property.Id == "last_modified_date" {
				require.Equal(t, "1970-01-12T13:46:39Z", *property.Date)
			} else if property.Id == "created_by" {
				require.Equal(t, []string{mockedParticipantId}, property.Objects)
			} else if property.Id == "last_modified_by" {
				require.Equal(t, []string{mockedParticipantId}, property.Objects)
			}
		}

		// check tags
		tags := []object.Tag{}
		for _, detail := range objects[0].Properties {
			for _, tag := range detail.MultiSelect {
				tags = append(tags, tag)
			}
		}
		require.Len(t, tags, 2)
		require.Equal(t, mockedTagId1, tags[0].Id)
		require.Equal(t, mockedTagValue1, tags[0].Name)
		require.Equal(t, object.Color(mockedTagColor1), tags[0].Color)
		require.Equal(t, mockedTagId2, tags[1].Id)
		require.Equal(t, mockedTagValue2, tags[1].Name)
		require.Equal(t, object.Color(mockedTagColor2), tags[1].Color)

		require.Equal(t, 1, total)
		require.False(t, hasMore)
	})

	t.Run("no objects found globally", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{},
			Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// when
		objects, total, hasMore, err := fx.service.GlobalSearch(ctx, SearchRequest{Query: mockedSearchTerm, Types: []string{}, Sort: SortOptions{Property: LastModifiedDate, Direction: Desc}}, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, objects, 0)
		require.Equal(t, 0, total)
		require.False(t, hasMore)
	})

	t.Run("error during global search", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).Return(&pb.RpcObjectSearchResponse{
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_UNKNOWN_ERROR},
		}).Once()

		// when
		objects, total, hasMore, err := fx.service.GlobalSearch(ctx, SearchRequest{Query: mockedSearchTerm, Types: []string{}, Sort: SortOptions{Property: LastModifiedDate, Direction: Desc}}, offset, limit)

		// then
		require.Error(t, err)
		require.Empty(t, objects)
		require.Equal(t, 0, total)
		require.False(t, hasMore)
	})
}

func TestSearchService_Search(t *testing.T) {
	t.Run("objects found in a specific space", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// Mock objects in space
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: mockedSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					Operator: model.BlockContentDataviewFilter_And,
					NestedFilters: []*model.BlockContentDataviewFilter{
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
							RelationKey: bundle.RelationKeyIsHidden.String(),
							Condition:   model.BlockContentDataviewFilter_NotEqual,
							Value:       pbtypes.Bool(true),
						},
						{
							RelationKey: "type.uniqueKey",
							Condition:   model.BlockContentDataviewFilter_NotEqual,
							Value:       pbtypes.String("ot-template"),
						},
						{
							Operator: model.BlockContentDataviewFilter_Or,
							NestedFilters: []*model.BlockContentDataviewFilter{
								{
									RelationKey: bundle.RelationKeyName.String(),
									Condition:   model.BlockContentDataviewFilter_Like,
									Value:       pbtypes.String(mockedSearchTerm),
								},
								{
									RelationKey: bundle.RelationKeySnippet.String(),
									Condition:   model.BlockContentDataviewFilter_Like,
									Value:       pbtypes.String(mockedSearchTerm),
								},
							},
						},
					},
				},
			},
			Sorts: []*model.BlockContentDataviewSort{{
				RelationKey:    bundle.RelationKeyLastModifiedDate.String(),
				Type:           model.BlockContentDataviewSort_Desc,
				Format:         model.RelationFormat_date,
				IncludeTime:    true,
				EmptyPlacement: model.BlockContentDataviewSort_NotSpecified,
			}},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():      pbtypes.String(mockedObjectId),
						bundle.RelationKeyName.String():    pbtypes.String(mockedObjectName),
						bundle.RelationKeySpaceId.String(): pbtypes.String(mockedSpaceId),
						bundle.RelationKeyType.String():    pbtypes.String(mockedTypeId),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// Mock GetPropertyMapsFromStore
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
				bundle.RelationKeyUniqueKey.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyRelationFormat.String(),
			},
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
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyUniqueKey.String():      pbtypes.String(bundle.RelationKeyTag.String()),
						bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(model.RelationFormat_tag)),
					},
				},
			},
		}, nil).Once()

		// Mock GetTypeMapFromStore
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
				bundle.RelationKeyRecommendedFeaturedRelations.String(),
				bundle.RelationKeyRecommendedRelations.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String(): pbtypes.String(mockedTypeId),
					},
				},
			},
		}, nil).Once()

		// when
		objects, total, hasMore, err := fx.service.Search(ctx, mockedSpaceId, SearchRequest{Query: mockedSearchTerm, Types: []string{}, Sort: SortOptions{Property: LastModifiedDate, Direction: Desc}}, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, objects, 1)
		require.Equal(t, mockedObjectId, objects[0].Id)
		require.Equal(t, mockedObjectName, objects[0].Name)
		require.Equal(t, mockedTypeId, objects[0].Type.Id)
		require.Equal(t, mockedSpaceId, objects[0].SpaceId)
		require.Equal(t, model.ObjectTypeLayout_name[int32(model.ObjectType_basic)], objects[0].Layout)

		require.Equal(t, 1, total)
		require.False(t, hasMore)
	})

	t.Run("no objects found in space", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// Mock object and property + type map search
		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{},
			Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Times(3)

		// when
		objects, total, hasMore, err := fx.service.Search(ctx, mockedSpaceId, SearchRequest{Query: mockedSearchTerm, Types: []string{}, Sort: SortOptions{Property: LastModifiedDate, Direction: Desc}}, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, objects, 0)
		require.Equal(t, 0, total)
		require.False(t, hasMore)
	})

	t.Run("error during search", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).Return(&pb.RpcObjectSearchResponse{
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_UNKNOWN_ERROR},
		}).Once()

		// when
		objects, total, hasMore, err := fx.service.Search(ctx, mockedSpaceId, SearchRequest{Query: mockedSearchTerm, Types: []string{}, Sort: SortOptions{Property: LastModifiedDate, Direction: Desc}}, offset, limit)

		// then
		require.Error(t, err)
		require.Empty(t, objects)
		require.Equal(t, 0, total)
		require.False(t, hasMore)
	})
}
