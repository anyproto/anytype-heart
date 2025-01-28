package search

import (
	"context"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/internal/object"
	"github.com/anyproto/anytype-heart/core/api/internal/space"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service/mock_service"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	offset                      = 0
	limit                       = 100
	techSpaceId                 = "tech-space-id"
	gatewayUrl                  = "http://localhost:31006"
	mockedSpaceId               = "mocked-space-id"
	mockedSearchTerm            = "mocked-search-term"
	mockedObjectId              = "mocked-object-id"
	mockedObjectName            = "mocked-object-name"
	mockedRootId                = "mocked-root-id"
	mockedParticipantId         = "mocked-participant-id"
	mockedType                  = "mocked-type"
	mockedTagId1                = "mocked-tag-id-1"
	mockedTagValue1             = "mocked-tag-value-1"
	mockedTagColor1             = "mocked-tag-color-1"
	mockedTagId2                = "mocked-tag-id-2"
	mockedTagValue2             = "mocked-tag-value-2"
	mockedTagColor2             = "mocked-tag-color-2"
	mockedObjectTypeName        = "mocked-object-type-name"
	mockedParticipantName       = "mocked-participant-name"
	mockedParticipantIcon       = "mocked-participant-icon"
	mockedParticipantImage      = "mocked-participant-image"
	mockedParticipantIdentity   = "mocked-participant-identity"
	mockedParticipantGlobalName = "mocked-participant-global-name"
)

type fixture struct {
	*SearchService
	mwMock *mock_service.MockClientCommandsServer
}

func newFixture(t *testing.T) *fixture {
	mw := mock_service.NewMockClientCommandsServer(t)

	spaceService := space.NewService(mw)
	spaceService.AccountInfo = &model.AccountInfo{TechSpaceId: techSpaceId, GatewayUrl: gatewayUrl}
	objectService := object.NewService(mw, spaceService)
	objectService.AccountInfo = &model.AccountInfo{TechSpaceId: techSpaceId}
	searchService := NewService(mw, spaceService, objectService)
	searchService.AccountInfo = &model.AccountInfo{
		TechSpaceId: techSpaceId,
		GatewayUrl:  gatewayUrl,
	}

	return &fixture{
		SearchService: searchService,
		mwMock:        mw,
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
					Operator:    model.BlockContentDataviewFilter_No,
					RelationKey: bundle.RelationKeyLayout.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
				},
				{
					Operator:    model.BlockContentDataviewFilter_No,
					RelationKey: bundle.RelationKeySpaceLocalStatus.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.SpaceStatus_Ok)),
				},
			},
			Sorts: []*model.BlockContentDataviewSort{
				{
					RelationKey:    bundle.RelationKeySpaceOrder.String(),
					Type:           model.BlockContentDataviewSort_Asc,
					NoCollate:      true,
					EmptyPlacement: model.BlockContentDataviewSort_End,
				},
			},
			Keys: []string{bundle.RelationKeyTargetSpaceId.String(), bundle.RelationKeyName.String(), bundle.RelationKeyIconEmoji.String(), bundle.RelationKeyIconImage.String()},
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

		// Mock workspace opening
		fx.mwMock.On("WorkspaceOpen", mock.Anything, &pb.RpcWorkspaceOpenRequest{
			SpaceId:  mockedSpaceId,
			WithChat: true,
		}).Return(&pb.RpcWorkspaceOpenResponse{
			Info: &model.AccountInfo{
				TechSpaceId: mockedSpaceId,
			},
			Error: &pb.RpcWorkspaceOpenResponseError{Code: pb.RpcWorkspaceOpenResponseError_NULL},
		}).Once()

		// Mock objects in space
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: mockedSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					Operator: model.BlockContentDataviewFilter_And,
					NestedFilters: []*model.BlockContentDataviewFilter{
						{
							Operator:    model.BlockContentDataviewFilter_No,
							RelationKey: bundle.RelationKeyLayout.String(),
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
							Operator:    model.BlockContentDataviewFilter_No,
							RelationKey: bundle.RelationKeyIsHidden.String(),
							Condition:   model.BlockContentDataviewFilter_NotEqual,
							Value:       pbtypes.Bool(true),
						},
						{
							Operator: model.BlockContentDataviewFilter_Or,
							NestedFilters: []*model.BlockContentDataviewFilter{
								{
									Operator:    model.BlockContentDataviewFilter_No,
									RelationKey: bundle.RelationKeyName.String(),
									Condition:   model.BlockContentDataviewFilter_Like,
									Value:       pbtypes.String(mockedSearchTerm),
								},
								{
									Operator:    model.BlockContentDataviewFilter_No,
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
			Keys:  []string{bundle.RelationKeyId.String(), bundle.RelationKeySpaceId.String(), bundle.RelationKeyLastModifiedDate.String()},
			Limit: int32(offset + limit),
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():      pbtypes.String(mockedObjectId),
						bundle.RelationKeyName.String():    pbtypes.String(mockedObjectName),
						bundle.RelationKeySpaceId.String(): pbtypes.String(mockedSpaceId),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// Mock object show for object blocks and details
		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  mockedSpaceId,
			ObjectId: mockedObjectId,
		}).Return(&pb.RpcObjectShowResponse{
			ObjectView: &model.ObjectView{
				RootId: mockedRootId,
				Blocks: []*model.Block{
					{
						Id: mockedRootId,
						Restrictions: &model.BlockRestrictions{
							Read:   false,
							Edit:   false,
							Remove: false,
							Drag:   false,
							DropOn: false,
						},
						ChildrenIds: []string{"header", "text-block"},
					},
					{
						Id: "header",
						Restrictions: &model.BlockRestrictions{
							Read:   false,
							Edit:   true,
							Remove: true,
							Drag:   true,
							DropOn: true,
						},
						ChildrenIds: []string{"title", "featuredRelations"},
					},
					{
						Id: "text-block",
						Content: &model.BlockContentOfText{
							Text: &model.BlockContentText{
								Text:  "This is a sample text block",
								Style: model.BlockContentText_Paragraph,
							},
						},
					},
				},
				Details: []*model.ObjectViewDetailsSet{
					{
						Id: mockedRootId,
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyId.String():               pbtypes.String(mockedObjectId),
								bundle.RelationKeyName.String():             pbtypes.String(mockedObjectName),
								bundle.RelationKeyLayout.String():           pbtypes.Int64(int64(model.ObjectType_basic)),
								bundle.RelationKeyIconEmoji.String():        pbtypes.String("🌐"),
								bundle.RelationKeyLastModifiedDate.String(): pbtypes.Float64(999999),
								bundle.RelationKeyLastModifiedBy.String():   pbtypes.String(mockedParticipantId),
								bundle.RelationKeyCreatedDate.String():      pbtypes.Float64(888888),
								bundle.RelationKeyCreator.String():          pbtypes.String(mockedParticipantId),
								bundle.RelationKeySpaceId.String():          pbtypes.String(mockedSpaceId),
								bundle.RelationKeyType.String():             pbtypes.String(mockedType),
								bundle.RelationKeyTag.String():              pbtypes.StringList([]string{mockedTagId1, mockedTagId2}),
							},
						},
					},
					{
						Id: mockedParticipantId,
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyId.String(): pbtypes.String(mockedParticipantId),
							},
						},
					},
					{
						Id: mockedTagId1,
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyName.String():                pbtypes.String(mockedTagValue1),
								bundle.RelationKeyRelationOptionColor.String(): pbtypes.String(mockedTagColor1),
							},
						},
					},
					{
						Id: mockedTagId2,
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								bundle.RelationKeyName.String():                pbtypes.String(mockedTagValue2),
								bundle.RelationKeyRelationOptionColor.String(): pbtypes.String(mockedTagColor2),
							},
						},
					},
				},
			},
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
		}, nil).Once()

		// Mock type resolution
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: mockedSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					Operator:    model.BlockContentDataviewFilter_No,
					RelationKey: bundle.RelationKeyId.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String(mockedType),
				},
			},
			Keys: []string{bundle.RelationKeyName.String()},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyName.String(): pbtypes.String(mockedObjectTypeName),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// Mock participant details
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: mockedSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					Operator:    model.BlockContentDataviewFilter_No,
					RelationKey: bundle.RelationKeyId.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String(mockedParticipantId),
				},
			},
			Keys: []string{bundle.RelationKeyId.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyIconEmoji.String(),
				bundle.RelationKeyIconImage.String(),
				bundle.RelationKeyIdentity.String(),
				bundle.RelationKeyGlobalName.String(),
				bundle.RelationKeyParticipantPermissions.String(),
			},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():                     pbtypes.String(mockedParticipantId),
						bundle.RelationKeyName.String():                   pbtypes.String(mockedParticipantName),
						bundle.RelationKeyIconEmoji.String():              pbtypes.String(mockedParticipantIcon),
						bundle.RelationKeyIconImage.String():              pbtypes.String(mockedParticipantImage),
						bundle.RelationKeyIdentity.String():               pbtypes.String(mockedParticipantIdentity),
						bundle.RelationKeyGlobalName.String():             pbtypes.String(mockedParticipantGlobalName),
						bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(model.ParticipantPermissions_Reader)),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Twice()

		// when
		objects, total, hasMore, err := fx.GlobalSearch(ctx, SearchRequest{Query: mockedSearchTerm, Types: []string{}, Sort: SortOptions{Direction: "desc", Timestamp: "last_modified_date"}}, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, objects, 1)
		require.Equal(t, mockedObjectTypeName, objects[0].Type)
		require.Equal(t, mockedSpaceId, objects[0].SpaceId)
		require.Equal(t, mockedObjectName, objects[0].Name)
		require.Equal(t, mockedObjectId, objects[0].Id)
		require.Equal(t, model.ObjectTypeLayout_name[int32(model.ObjectType_basic)], objects[0].Layout)
		require.Equal(t, "🌐", objects[0].Icon)
		require.Equal(t, "This is a sample text block", objects[0].Blocks[2].Text.Text)

		// check details
		for _, detail := range objects[0].Details {
			if detail.Id == "created_date" {
				require.Equal(t, "1970-01-11T06:54:48Z", detail.Details["created_date"])
			} else if detail.Id == "last_modified_date" {
				require.Equal(t, "1970-01-12T13:46:39Z", detail.Details["last_modified_date"])
			} else if detail.Id == "created_by" {
				require.Equal(t, mockedParticipantId, detail.Details["details"].(space.Member).Id)
				require.Equal(t, mockedParticipantName, detail.Details["details"].(space.Member).Name)
				require.Equal(t, gatewayUrl+"/image/"+mockedParticipantImage, detail.Details["details"].(space.Member).Icon)
				require.Equal(t, mockedParticipantIdentity, detail.Details["details"].(space.Member).Identity)
				require.Equal(t, mockedParticipantGlobalName, detail.Details["details"].(space.Member).GlobalName)
			} else if detail.Id == "last_modified_by" {
				require.Equal(t, mockedParticipantId, detail.Details["details"].(space.Member).Id)
			}
		}

		// check tags
		tags := []object.Tag{}
		for _, detail := range objects[0].Details {
			if tagList, ok := detail.Details["tags"].([]object.Tag); ok {
				for _, tag := range tagList {
					tags = append(tags, tag)
				}
			}
		}
		require.Len(t, tags, 2)
		require.Equal(t, mockedTagId1, tags[0].Id)
		require.Equal(t, mockedTagValue1, tags[0].Name)
		require.Equal(t, mockedTagColor1, tags[0].Color)
		require.Equal(t, mockedTagId2, tags[1].Id)
		require.Equal(t, mockedTagValue2, tags[1].Name)
		require.Equal(t, mockedTagColor2, tags[1].Color)

		require.Equal(t, 1, total)
		require.False(t, hasMore)
	})
}
