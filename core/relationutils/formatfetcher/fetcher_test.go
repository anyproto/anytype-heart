package formatfetcher

import (
	"fmt"
	"testing"

	"github.com/cheggaaa/mb/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/mock_subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/futures"
)

const testSpaceId = "test-space-1"

type fixture struct {
	*formatFetcher
	sub *mock_subscription.MockService
	ev  *mb.MB[*pb.EventMessage]
}

func (f *fixture) close(t *testing.T) {
	if f.ev != nil {
		require.NoError(t, f.ev.Close())
	}
}

func newFixture(t *testing.T) *fixture {
	sub := mock_subscription.NewMockService(t)
	ev := mb.New[*pb.EventMessage](0)

	f := &formatFetcher{
		subs:         map[string]*futures.Future[*objectsubscription.ObjectSubscription[model.RelationLink]]{},
		subscription: sub,
	}
	return &fixture{
		formatFetcher: f,
		sub:           sub,
		ev:            ev,
	}
}

func TestFormatFetcher_GetRelationFormatByKey(t *testing.T) {
	t.Run("returns bundle relation format", func(t *testing.T) {
		// given
		f := &formatFetcher{}

		// when
		format, err := f.GetRelationFormatByKey(testSpaceId, bundle.RelationKeyName)

		// then
		assert.NoError(t, err)
		assert.Equal(t, model.RelationFormat_shorttext, format)
	})

	t.Run("returns cached format for existing subscription", func(t *testing.T) {
		// given
		f := newFixture(t)
		defer f.close(t)
		f.sub.EXPECT().Search(mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{
				domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyId:             domain.String("rel1"),
					bundle.RelationKeyRelationKey:    domain.String("custom_relation"),
					bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_longtext)),
				}),
			},
			Output: f.ev,
		}, nil)

		// when
		format, err := f.GetRelationFormatByKey(testSpaceId, "custom_relation")

		// then
		assert.NoError(t, err)
		assert.Equal(t, model.RelationFormat_longtext, format)
	})

	t.Run("returns error for non-existent relation in existing subscription", func(t *testing.T) {
		// given
		f := newFixture(t)
		defer f.close(t)
		f.sub.EXPECT().Search(mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
			Output:  f.ev,
		}, nil)

		// when
		_, err := f.GetRelationFormatByKey(testSpaceId, "non_existent")

		// then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "relation format not found for key non_existent")
	})

	t.Run("sets up new subscription and returns format for custom relations", func(t *testing.T) {
		// given
		f := newFixture(t)
		defer f.close(t)
		f.sub.EXPECT().Search(mock.MatchedBy(func(req subscription.SubscribeRequest) bool {
			return req.SpaceId == testSpaceId &&
				req.SubId == buildSubId(testSpaceId) &&
				len(req.Keys) == 3 &&
				req.NoDepSubscription == true &&
				req.Internal == true &&
				len(req.Filters) == 1 &&
				req.Filters[0].RelationKey == bundle.RelationKeyResolvedLayout
		})).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{
				domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyRelationKey:    domain.String("custom_relation"),
					bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_number)),
				}),
			},
			Output: f.ev,
		}, nil)

		// when
		format, err := f.GetRelationFormatByKey(testSpaceId, "custom_relation")

		// then
		assert.NoError(t, err)
		assert.Equal(t, model.RelationFormat_number, format)
		assert.Contains(t, f.subs, testSpaceId)
	})

	t.Run("returns error when subscription setup fails", func(t *testing.T) {
		// given
		f := newFixture(t)
		defer f.close(t)
		f.sub.EXPECT().Search(mock.Anything).Return(nil, fmt.Errorf("subscription error"))

		// when
		format, err := f.GetRelationFormatByKey(testSpaceId, "custom_relation")

		// then
		assert.Error(t, err)
		assert.Equal(t, model.RelationFormat(0), format)
	})

	t.Run("returns error for non-existent relation in new subscription", func(t *testing.T) {
		// given
		f := newFixture(t)
		defer f.close(t)
		f.sub.EXPECT().Search(mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
			Output:  f.ev,
		}, nil)

		// when
		_, err := f.GetRelationFormatByKey(testSpaceId, "non_existent")

		// then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "relation format not found for key non_existent")
	})
}

func TestFormatFetcher_setupSub(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		f := newFixture(t)
		defer f.close(t)
		f.sub.EXPECT().Search(mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{
				domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyId:             domain.String("rel1"),
					bundle.RelationKeyRelationKey:    domain.String("custom_relation_1"),
					bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_tag)),
				}),
				domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyId:             domain.String("rel2"),
					bundle.RelationKeyRelationKey:    domain.String("custom_relation_2"),
					bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
				}),
			},
			Output: f.ev,
		}, nil)

		// when
		sub, err := f.setupSub(testSpaceId)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, sub)
		assert.NotNil(t, sub)

		relLink, ok := sub.GetByKey("custom_relation_1")
		require.True(t, ok)
		assert.Equal(t, "custom_relation_1", relLink.Key)
		assert.Equal(t, model.RelationFormat_tag, relLink.Format)

		relLink, ok = sub.GetByKey("custom_relation_2")
		require.True(t, ok)
		assert.Equal(t, "custom_relation_2", relLink.Key)
		assert.Equal(t, model.RelationFormat_status, relLink.Format)
	})

	t.Run("bundle relations are not included in cache", func(t *testing.T) {
		// given
		f := newFixture(t)
		defer f.close(t)
		f.sub.EXPECT().Search(mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{
				domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyId:             domain.String(bundle.RelationKeyName.URL()),
					bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeyName.String()),
					bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_shorttext)),
				}),
				domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyId:             domain.String("rel_id"),
					bundle.RelationKeyRelationKey:    domain.String("custom_relation"),
					bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_number)),
				}),
			},
			Output: f.ev,
		}, nil)

		// when
		sub, err := f.setupSub(testSpaceId)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, sub)

		relLink, ok := sub.GetByKey("custom_relation")
		require.True(t, ok)
		assert.Equal(t, "custom_relation", relLink.Key)
		assert.Equal(t, model.RelationFormat_number, relLink.Format)

		relLink, ok = sub.GetByKey(bundle.RelationKeyName.String())
		require.False(t, ok)
	})

	t.Run("returns error when search fails", func(t *testing.T) {
		// given
		f := newFixture(t)
		defer f.close(t)
		f.sub.EXPECT().Search(mock.Anything).Return(nil, fmt.Errorf("search failed"))

		// when
		sub, err := f.setupSub(testSpaceId)

		// then
		assert.Error(t, err)
		assert.Nil(t, sub)
	})
}
