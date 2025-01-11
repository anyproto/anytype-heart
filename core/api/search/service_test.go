package search

import (
	"context"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	object2 "github.com/anyproto/anytype-heart/core/api/object"
	"github.com/anyproto/anytype-heart/core/api/space"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service/mock_service"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	offset      = 0
	limit       = 100
	techSpaceId = "tech-space-id"
	gatewayUrl  = "http://localhost:31006"
)

type fixture struct {
	*SearchService
	mwMock *mock_service.MockClientCommandsServer
}

func newFixture(t *testing.T) *fixture {
	mw := mock_service.NewMockClientCommandsServer(t)

	spaceService := space.NewService(mw)
	spaceService.AccountInfo = &model.AccountInfo{TechSpaceId: techSpaceId}
	objectService := object2.NewService(mw)
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

func TestSearchService_Search(t *testing.T) {
	t.Run("objects found globally", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		// Mock retrieving spaces first
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: techSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyLayout.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
				},
				{
					RelationKey: bundle.RelationKeySpaceLocalStatus.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.SpaceStatus_Ok)),
				},
				{
					RelationKey: bundle.RelationKeySpaceRemoteStatus.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.SpaceStatus_Ok)),
				},
			},
			Sorts: []*model.BlockContentDataviewSort{
				{
					RelationKey:    "spaceOrder",
					Type:           model.BlockContentDataviewSort_Asc,
					NoCollate:      true,
					EmptyPlacement: model.BlockContentDataviewSort_End,
				},
			},
			Keys: []string{"targetSpaceId", "name", "iconEmoji", "iconImage"},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						"targetSpaceId": pbtypes.String("space-1"),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// Mock workspace opening
		fx.mwMock.On("WorkspaceOpen", mock.Anything, &pb.RpcWorkspaceOpenRequest{
			SpaceId:  "space-1",
			WithChat: true,
		}).Return(&pb.RpcWorkspaceOpenResponse{
			Info: &model.AccountInfo{
				TechSpaceId: "space-1",
			},
			Error: &pb.RpcWorkspaceOpenResponseError{Code: pb.RpcWorkspaceOpenResponseError_NULL},
		}).Once()

		// Mock objects in space-1
		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						"id":   pbtypes.String("obj-global-1"),
						"name": pbtypes.String("Global Object"),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Twice()

		// Mock object show for object blocks and details
		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  "space-1",
			ObjectId: "obj-global-1",
		}).Return(&pb.RpcObjectShowResponse{
			ObjectView: &model.ObjectView{
				RootId: "root-123",
				Blocks: []*model.Block{
					{
						Id: "root-123",
						Restrictions: &model.BlockRestrictions{
							Read:   false,
							Edit:   false,
							Remove: false,
							Drag:   false,
							DropOn: false,
						},
						ChildrenIds: []string{"header", "text-block", "relation-block"},
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
						Id: "root-123",
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								"id":               pbtypes.String("obj-global-1"),
								"name":             pbtypes.String("Global Object"),
								"layout":           pbtypes.Int64(int64(model.ObjectType_basic)),
								"iconEmoji":        pbtypes.String("🌐"),
								"lastModifiedDate": pbtypes.Float64(999999),
								"createdDate":      pbtypes.Float64(888888),
								"spaceId":          pbtypes.String("space-1"),
								"tag":              pbtypes.StringList([]string{"tag-1", "tag-2"}),
							},
						},
					},
					{
						Id: "tag-1",
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								"name":                pbtypes.String("Important"),
								"relationOptionColor": pbtypes.String("red"),
							},
						},
					},
					{
						Id: "tag-2",
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								"name":                pbtypes.String("Optional"),
								"relationOptionColor": pbtypes.String("blue"),
							},
						},
					},
				},
			},
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
		}, nil).Once()

		// when
		objects, total, hasMore, err := fx.Search(ctx, "search-term", []string{}, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, objects, 1)
		require.Equal(t, "object", objects[0].Type)
		require.Equal(t, "space-1", objects[0].SpaceId)
		require.Equal(t, "Global Object", objects[0].Name)
		require.Equal(t, "obj-global-1", objects[0].Id)
		require.Equal(t, "basic", objects[0].Layout)
		require.Equal(t, "🌐", objects[0].Icon)
		require.Equal(t, "This is a sample text block", objects[0].Blocks[2].Text.Text)

		// check details
		for _, detail := range objects[0].Details {
			if detail.Id == "createdDate" {
				require.Equal(t, float64(888888), detail.Details["createdDate"])
			} else if detail.Id == "lastModifiedDate" {
				require.Equal(t, float64(999999), detail.Details["lastModifiedDate"])
			}
		}

		// check tags
		tags := []object2.Tag{}
		for _, detail := range objects[0].Details {
			if tagList, ok := detail.Details["tags"].([]object2.Tag); ok {
				for _, tag := range tagList {
					tags = append(tags, tag)
				}
			}
		}
		require.Len(t, tags, 2)
		require.Equal(t, "tag-1", tags[0].Id)
		require.Equal(t, "Important", tags[0].Name)
		require.Equal(t, "red", tags[0].Color)
		require.Equal(t, "tag-2", tags[1].Id)
		require.Equal(t, "Optional", tags[1].Name)
		require.Equal(t, "blue", tags[1].Color)

		require.Equal(t, 1, total)
		require.False(t, hasMore)
	})
}
