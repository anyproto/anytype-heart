package objectgraph

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/mock_subscription"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider/mock_typeprovider"
	"github.com/anyproto/anytype-heart/util/dateutil"
)

type fixture struct {
	Builder
	objectStoreMock         *objectstore.StoreFixture
	sbtProviderMock         *mock_typeprovider.MockSmartBlockTypeProvider
	subscriptionServiceMock *mock_subscription.MockService
}

func newFixture(t *testing.T) *fixture {
	objectStore := objectstore.NewStoreFixture(t)
	sbtProvider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
	subscriptionService := mock_subscription.NewMockService(t)

	return &fixture{
		Builder: Builder{
			objectStore:         objectStore,
			sbtProvider:         sbtProvider,
			subscriptionService: subscriptionService,
		},
		objectStoreMock:         objectStore,
		sbtProviderMock:         sbtProvider,
		subscriptionServiceMock: subscriptionService,
	}
}

func Test(t *testing.T) {
	t.Run("sub request - added proper relations", func(t *testing.T) {
		fx := newFixture(t)
		spaceId := "space1"
		fx.objectStoreMock.AddObjects(t, spaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel1"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeyId.String()),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
			},
			{
				bundle.RelationKeyId:             domain.String("rel2"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeyName.String()),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_shorttext)),
			},
			{
				bundle.RelationKeyId:             domain.String("rel3"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeyAuthor.String()),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
			},
			{
				bundle.RelationKeyId:             domain.String("rel4"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeyLinkedProjects.String()),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
			},
		})
		fx.subscriptionServiceMock.EXPECT().Search(mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil)
		fx.subscriptionServiceMock.EXPECT().Unsubscribe(mock.Anything).Return(nil)

		req := ObjectGraphRequest{
			SpaceId: spaceId,
		}
		graph, edges, err := fx.ObjectGraph(req)
		assert.NoError(t, err)
		assert.Empty(t, graph)
		assert.Empty(t, edges)
	})

	t.Run("graph", func(t *testing.T) {
		fx := newFixture(t)
		spaceId := "space1"
		dateObject := dateutil.NewDateObject(time.Now(), false)
		fx.objectStoreMock.AddObjects(t, spaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel1"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeyId.String()),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
			},
			{
				bundle.RelationKeyId:             domain.String("rel2"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeyName.String()),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_shorttext)),
			},
			{
				bundle.RelationKeyId:             domain.String("rel3"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeyAuthor.String()),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
			},
			{
				bundle.RelationKeyId:             domain.String("rel4"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeyLinkedProjects.String()),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
			},
		})
		fx.objectStoreMock.AddVirtualDetails(dateObject.Id(), domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:             domain.String(dateObject.Id()),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_date)),
			bundle.RelationKeyName:           domain.String(dateObject.Name()),
			bundle.RelationKeyTimestamp:      domain.Int64(dateObject.Time().Unix()),
		}))
		fx.subscriptionServiceMock.EXPECT().Search(mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{
				domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyId:       domain.String("id1"),
					bundle.RelationKeyAssignee: domain.String("id2"),
					bundle.RelationKeyLinks:    domain.StringList([]string{"id2", "id3", dateObject.Id()}),
				}),
				domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyId: domain.String("id2"),
				}),
				domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyId: domain.String("id3"),
				}),
			},
		}, nil)
		fx.subscriptionServiceMock.EXPECT().Unsubscribe(mock.Anything).Return(nil)
		fx.sbtProviderMock.EXPECT().Type(mock.Anything, mock.Anything).RunAndReturn(func(spcId string, id string) (smartblock.SmartBlockType, error) {
			require.Equal(t, spcId, spaceId)
			if _, err := dateutil.BuildDateObjectFromId(id); err == nil {
				return smartblock.SmartBlockTypeDate, err
			}
			return smartblock.SmartBlockTypePage, nil
		})

		req := ObjectGraphRequest{
			SpaceId: spaceId,
		}
		graph, edges, err := fx.ObjectGraph(req)
		assert.NoError(t, err)
		require.Len(t, graph, 4)
		require.Len(t, edges, 3)
		assert.Equal(t, "id1", edges[0].Source)
		assert.Equal(t, "id2", edges[0].Target)
		assert.Equal(t, "id1", edges[1].Source)
		assert.Equal(t, "id3", edges[1].Target)
		assert.Equal(t, "id1", edges[2].Source)
		assert.Equal(t, dateObject.Id(), edges[2].Target)
	})

}

func Test_isRelationShouldBeIncludedAsEdge(t *testing.T) {

	tests := []struct {
		name             string
		rel              *relationutils.Relation
		includeTypeEdges bool
		want             bool
	}{
		{"creator",
			&relationutils.Relation{Relation: bundle.MustGetRelation(bundle.RelationKeyCreator)},
			false,
			false,
		},
		{"assignee",
			&relationutils.Relation{Relation: bundle.MustGetRelation(bundle.RelationKeyAssignee)},
			false,
			true,
		},
		{"cover",
			&relationutils.Relation{Relation: bundle.MustGetRelation(bundle.RelationKeyCoverId)},
			false,
			false,
		},
		{"file relation",
			&relationutils.Relation{Relation: bundle.MustGetRelation(bundle.RelationKeyPicture)},
			false,
			true,
		},
		{"custom relation",
			&relationutils.Relation{Relation: &model.Relation{Name: "custom", Format: model.RelationFormat_object}},
			false,
			true,
		},
		{"type with includeTypeEdges false",
			&relationutils.Relation{Relation: bundle.MustGetRelation(bundle.RelationKeyType)},
			false,
			false,
		},
		{"type with includeTypeEdges true",
			&relationutils.Relation{Relation: bundle.MustGetRelation(bundle.RelationKeyType)},
			true,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRelationShouldBeIncludedAsEdge(tt.rel, tt.includeTypeEdges); got != tt.want {
				t.Errorf("isRelationShouldBeIncludedAsEdge() = %v, want %v", got, tt.want)
			}
		})
	}
}
