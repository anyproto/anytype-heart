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

const (
	mockedSearchTerm = "mocked-search-term"
	mockedTagId1     = "tag1"
	mockedTagValue1  = "Important"
	mockedTagColor1  = "red"
	mockedTagId2     = "tag2"
	mockedTagValue2  = "Urgent"
	mockedTagColor2  = "orange"
)

func TestSearchService_GlobalSearch(t *testing.T) {
	t.Run("objects found globally", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

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
					RelationKey: bundle.RelationKeySpaceAccountStatus.String(),
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       pbtypes.IntList(int(model.SpaceStatus_Unknown), int(model.SpaceStatus_SpaceActive)),
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
							Value:       pbtypes.IntList(util.LayoutsToIntArgs(util.ObjectLayouts)...),
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
				RelationKey: bundle.RelationKeyLastModifiedDate.String(),
				Type:        model.BlockContentDataviewSort_Desc,
				Format:      model.RelationFormat_date,
				IncludeTime: true,
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

		// when
		objects, total, hasMore, err := fx.service.GlobalSearch(ctx, apimodel.SearchRequest{Query: mockedSearchTerm, Types: []string{}, Sort: apimodel.SortOptions{PropertyKey: apimodel.LastModifiedDate, Direction: apimodel.Desc}}, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, objects, 1)
		require.Equal(t, mockedObjectId, objects[0].Id)
		require.Equal(t, mockedObjectName, objects[0].Name)
		require.Equal(t, mockedTypeId, objects[0].Type.Id)
		require.Equal(t, mockedSpaceId, objects[0].SpaceId)
		require.Equal(t, apimodel.ObjectLayoutBasic, objects[0].Layout)
		require.Equal(t, &apimodel.Icon{
			WrappedIcon: apimodel.EmojiIcon{
				Format: apimodel.IconFormatEmoji,
				Emoji:  mockedObjectIcon,
			},
		}, objects[0].Icon)

		for _, propResp := range objects[0].Properties {
			switch v := propResp.WrappedPropertyWithValue.(type) {
			case apimodel.DatePropertyValue:
				switch v.Key {
				case "created_date":
					require.Equal(t, "1970-01-11T06:54:48Z", v.Date)
				case "last_modified_date":
					require.Equal(t, "1970-01-12T13:46:39Z", v.Date)
				}
			case apimodel.ObjectsPropertyValue:
				switch v.Key {
				case "created_by":
					require.Equal(t, []string{mockedParticipantId}, v.Objects)
				case "last_modified_by":
					require.Equal(t, []string{mockedParticipantId}, v.Objects)
				}
			case apimodel.MultiSelectPropertyValue:
				continue
			default:
				t.Errorf("unexpected property type: %T", v)
			}
		}

		// check tags
		tags := []*apimodel.Tag{}
		for _, propResp := range objects[0].Properties {
			if v, ok := propResp.WrappedPropertyWithValue.(apimodel.MultiSelectPropertyValue); ok {
				tags = append(tags, v.MultiSelect...)
			}
		}
		require.Len(t, tags, 2)
		require.Equal(t, mockedTagId1, tags[0].Id)
		require.Equal(t, mockedTagValue1, tags[0].Name)
		require.Equal(t, apimodel.Color(mockedTagColor1), tags[0].Color)
		require.Equal(t, mockedTagId2, tags[1].Id)
		require.Equal(t, mockedTagValue2, tags[1].Name)
		require.Equal(t, apimodel.Color(mockedTagColor2), tags[1].Color)

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
		objects, total, hasMore, err := fx.service.GlobalSearch(ctx, apimodel.SearchRequest{Query: mockedSearchTerm, Types: []string{}, Sort: apimodel.SortOptions{PropertyKey: apimodel.LastModifiedDate, Direction: apimodel.Desc}}, offset, limit)

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
		objects, total, hasMore, err := fx.service.GlobalSearch(ctx, apimodel.SearchRequest{Query: mockedSearchTerm, Types: []string{}, Sort: apimodel.SortOptions{PropertyKey: apimodel.LastModifiedDate, Direction: apimodel.Desc}}, offset, limit)

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
		fx.populateCache(mockedSpaceId)

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
							Value:       pbtypes.IntList(util.LayoutsToIntArgs(util.ObjectLayouts)...),
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
				RelationKey: bundle.RelationKeyLastModifiedDate.String(),
				Type:        model.BlockContentDataviewSort_Desc,
				Format:      model.RelationFormat_date,
				IncludeTime: true,
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

		// when
		objects, total, hasMore, err := fx.service.Search(ctx, mockedSpaceId, apimodel.SearchRequest{Query: mockedSearchTerm, Types: []string{}, Sort: apimodel.SortOptions{PropertyKey: apimodel.LastModifiedDate, Direction: apimodel.Desc}}, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, objects, 1)
		require.Equal(t, mockedObjectId, objects[0].Id)
		require.Equal(t, mockedObjectName, objects[0].Name)
		require.Equal(t, mockedTypeId, objects[0].Type.Id)
		require.Equal(t, mockedSpaceId, objects[0].SpaceId)
		require.Equal(t, apimodel.ObjectLayoutBasic, objects[0].Layout)

		require.Equal(t, 1, total)
		require.False(t, hasMore)
	})

	t.Run("no objects found in space", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		// Mock object search
		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{},
			Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// when
		objects, total, hasMore, err := fx.service.Search(ctx, mockedSpaceId, apimodel.SearchRequest{Query: mockedSearchTerm, Types: []string{}, Sort: apimodel.SortOptions{PropertyKey: apimodel.LastModifiedDate, Direction: apimodel.Desc}}, offset, limit)

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
		objects, total, hasMore, err := fx.service.Search(ctx, mockedSpaceId, apimodel.SearchRequest{Query: mockedSearchTerm, Types: []string{}, Sort: apimodel.SortOptions{PropertyKey: apimodel.LastModifiedDate, Direction: apimodel.Desc}}, offset, limit)

		// then
		require.Error(t, err)
		require.Empty(t, objects)
		require.Equal(t, 0, total)
		require.False(t, hasMore)
	})
}
