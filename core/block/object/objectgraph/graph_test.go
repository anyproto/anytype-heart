package objectgraph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/mock_subscription"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/mock_objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider/mock_typeprovider"
)

type fixture struct {
	Builder
	objectStoreMock         *mock_objectstore.MockObjectStore
	sbtProviderMock         *mock_typeprovider.MockSmartBlockTypeProvider
	subscriptionServiceMock *mock_subscription.MockService
}

func newFixture(t *testing.T) *fixture {
	objectStore := mock_objectstore.NewMockObjectStore(t)
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
		fixture := newFixture(t)
		fixture.objectStoreMock.EXPECT().ListAllRelations(mock.Anything).Return([]*relationutils.Relation{
			{Relation: bundle.MustGetRelation(bundle.RelationKeyId)},
			{Relation: bundle.MustGetRelation(bundle.RelationKeyName)},
			{Relation: bundle.MustGetRelation(bundle.RelationKeyAuthor)},
			{Relation: bundle.MustGetRelation(bundle.RelationKeyAttachments)},
		}, nil)
		fixture.subscriptionServiceMock.EXPECT().Search(mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil)
		fixture.subscriptionServiceMock.EXPECT().Unsubscribe(mock.Anything).Return(nil)

		req := ObjectGraphRequest{}
		graph, edges, err := fixture.ObjectGraph(req)
		assert.NoError(t, err)
		assert.True(t, len(graph) == 0)
		assert.True(t, len(edges) == 0)
	})

	t.Run("graph", func(t *testing.T) {
		fixture := newFixture(t)
		fixture.objectStoreMock.EXPECT().ListAllRelations(mock.Anything).Return([]*relationutils.Relation{
			{Relation: bundle.MustGetRelation(bundle.RelationKeyId)},
			{Relation: bundle.MustGetRelation(bundle.RelationKeyName)},
			{Relation: bundle.MustGetRelation(bundle.RelationKeyAssignee)},
			{Relation: bundle.MustGetRelation(bundle.RelationKeyAttachments)},
		}, nil)
		fixture.subscriptionServiceMock.EXPECT().Search(mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{
				domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyId:       domain.String("id1"),
					bundle.RelationKeyAssignee: domain.String("id2"),
					bundle.RelationKeyLinks:    domain.StringList([]string{"id2", "id3"}),
				}),
				domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyId: domain.String("id2"),
				}),
				domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyId: domain.String("id3"),
				}),
			},
		}, nil)
		fixture.subscriptionServiceMock.EXPECT().Unsubscribe(mock.Anything).Return(nil)
		fixture.sbtProviderMock.EXPECT().Type(mock.Anything, mock.Anything).Return(smartblock.SmartBlockTypePage, nil)

		req := ObjectGraphRequest{}
		graph, edges, err := fixture.ObjectGraph(req)
		assert.NoError(t, err)
		assert.True(t, len(graph) == 3)
		assert.True(t, len(edges) == 2)
		assert.Equal(t, "id1", edges[0].Source)
		assert.Equal(t, "id2", edges[0].Target)
		assert.Equal(t, "id1", edges[1].Source)
		assert.Equal(t, "id3", edges[1].Target)
	})

}

func Test_isRelationShouldBeIncludedAsEdge(t *testing.T) {

	tests := []struct {
		name string
		rel  *relationutils.Relation
		want bool
	}{
		{"creator",
			&relationutils.Relation{Relation: bundle.MustGetRelation(bundle.RelationKeyCreator)},
			false,
		},
		{"assignee",
			&relationutils.Relation{Relation: bundle.MustGetRelation(bundle.RelationKeyAssignee)},
			true,
		},
		{"cover",
			&relationutils.Relation{Relation: bundle.MustGetRelation(bundle.RelationKeyCoverId)},
			false,
		},
		{"file relation",
			&relationutils.Relation{Relation: bundle.MustGetRelation(bundle.RelationKeyTrailer)},
			true,
		},
		{"custom relation",
			&relationutils.Relation{Relation: &model.Relation{Name: "custom", Format: model.RelationFormat_object}},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRelationShouldBeIncludedAsEdge(tt.rel); got != tt.want {
				t.Errorf("isRelationShouldBeIncludedAsEdge() = %v, want %v", got, tt.want)
			}
		})
	}
}
