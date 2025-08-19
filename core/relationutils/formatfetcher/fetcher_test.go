package formatfetcher

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/mock_subscription"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const testSpaceId = "test-space-1"

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
		f := &formatFetcher{
			subs: map[string]*spaceSubscription{
				testSpaceId: {
					cache: map[domain.RelationKey]model.RelationFormat{
						"custom_relation": model.RelationFormat_longtext,
					},
				},
			},
		}

		// when
		format, err := f.GetRelationFormatByKey(testSpaceId, "custom_relation")

		// then
		assert.NoError(t, err)
		assert.Equal(t, model.RelationFormat_longtext, format)
	})

	t.Run("returns error for non-existent relation in existing subscription", func(t *testing.T) {
		// given
		f := &formatFetcher{
			subs: map[string]*spaceSubscription{
				testSpaceId: {
					cache: map[domain.RelationKey]model.RelationFormat{},
				},
			},
		}

		// when
		_, err := f.GetRelationFormatByKey(testSpaceId, "non_existent")

		// then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "relation format not found for key non_existent")
	})

	t.Run("sets up new subscription and returns format for custom relations", func(t *testing.T) {
		// given
		f := &formatFetcher{
			subs: map[string]*spaceSubscription{},
		}

		mockSub := mock_subscription.NewMockService(t)
		mockSub.EXPECT().Search(mock.MatchedBy(func(req subscription.SubscribeRequest) bool {
			return req.SpaceId == testSpaceId &&
				req.SubId == buildSubId(testSpaceId) &&
				len(req.Keys) == 2 &&
				req.Keys[0] == bundle.RelationKeyRelationKey.String() &&
				req.Keys[1] == bundle.RelationKeyRelationFormat.String() &&
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
		}, nil)
		f.subscription = mockSub

		// when
		format, err := f.GetRelationFormatByKey(testSpaceId, "custom_relation")

		// then
		assert.NoError(t, err)
		assert.Equal(t, model.RelationFormat_number, format)
		assert.Contains(t, f.subs, testSpaceId)
	})

	t.Run("returns error when subscription setup fails", func(t *testing.T) {
		// given
		f := &formatFetcher{
			subs: map[string]*spaceSubscription{},
		}

		mockSub := mock_subscription.NewMockService(t)
		mockSub.EXPECT().Search(mock.Anything).Return(nil, fmt.Errorf("subscription error"))
		f.subscription = mockSub

		// when
		format, err := f.GetRelationFormatByKey(testSpaceId, "custom_relation")

		// then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to setup relation formats subscription")
		assert.Equal(t, model.RelationFormat(0), format)
	})

	t.Run("returns error for non-existent relation in new subscription", func(t *testing.T) {
		// given
		f := &formatFetcher{
			subs: map[string]*spaceSubscription{},
		}

		mockSub := mock_subscription.NewMockService(t)
		mockSub.EXPECT().Search(mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil)
		f.subscription = mockSub

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
		f := &formatFetcher{
			subs: map[string]*spaceSubscription{},
		}

		mockSub := mock_subscription.NewMockService(t)
		mockSub.EXPECT().Search(mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{
				domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyRelationKey:    domain.String("custom_relation_1"),
					bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_tag)),
				}),
				domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyRelationKey:    domain.String("custom_relation_2"),
					bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
				}),
			},
		}, nil)
		f.subscription = mockSub

		// when
		sub, err := f.setupSub(testSpaceId)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, sub)
		assert.NotNil(t, sub.sub)
		assert.NotNil(t, sub.queue)
		assert.NotNil(t, sub.cache)
		assert.Len(t, sub.cache, 2)
		assert.Equal(t, model.RelationFormat_tag, sub.cache["custom_relation_1"])
		assert.Equal(t, model.RelationFormat_status, sub.cache["custom_relation_2"])
		assert.Contains(t, f.subs, testSpaceId)
	})

	t.Run("bundle relations are not included in cache", func(t *testing.T) {
		// given
		f := &formatFetcher{
			subs: map[string]*spaceSubscription{},
		}

		mockSub := mock_subscription.NewMockService(t)
		mockSub.EXPECT().Search(mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{
				domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeyName.String()),
					bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_shorttext)),
				}),
				domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyRelationKey:    domain.String("custom_relation"),
					bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_number)),
				}),
			},
		}, nil)
		f.subscription = mockSub

		// when
		sub, err := f.setupSub(testSpaceId)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, sub)
		assert.Len(t, sub.cache, 1)
		assert.Equal(t, model.RelationFormat_number, sub.cache["custom_relation"])
		assert.NotContains(t, sub.cache, bundle.RelationKeyName)
	})

	t.Run("returns error when search fails", func(t *testing.T) {
		// given
		f := &formatFetcher{
			subs: map[string]*spaceSubscription{},
		}

		mockSub := mock_subscription.NewMockService(t)
		mockSub.EXPECT().Search(mock.Anything).Return(nil, fmt.Errorf("search failed"))
		f.subscription = mockSub

		// when
		sub, err := f.setupSub(testSpaceId)

		// then
		assert.Error(t, err)
		assert.Nil(t, sub)
		assert.Contains(t, err.Error(), "failed to setup relation formats subscription")
	})
}

func TestFormatFetcher_buildSubscriptionParams(t *testing.T) {
	t.Run("SetDetails function works correctly", func(t *testing.T) {
		// given
		f := &formatFetcher{
			subs: map[string]*spaceSubscription{
				testSpaceId: {
					cache: map[domain.RelationKey]model.RelationFormat{},
				},
			},
		}

		params := f.buildSubscriptionParams(testSpaceId)

		details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:             domain.String("test-id"),
			bundle.RelationKeyRelationKey:    domain.String("custom_relation"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_url)),
		})

		// when
		id, entry := params.SetDetails(details)

		// then
		assert.Equal(t, "test-id", id)
		assert.Equal(t, "custom_relation", entry.Key)
		assert.Equal(t, model.RelationFormat_url, entry.Format)
		assert.Equal(t, model.RelationFormat_url, f.subs[testSpaceId].cache["custom_relation"])
	})

	t.Run("SetDetails ignores bundle relations", func(t *testing.T) {
		// given
		f := &formatFetcher{
			subs: map[string]*spaceSubscription{
				testSpaceId: {
					cache: map[domain.RelationKey]model.RelationFormat{},
				},
			},
		}

		params := f.buildSubscriptionParams(testSpaceId)

		details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:             domain.String("test-id"),
			bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeyName.String()),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_shorttext)),
		})

		// when
		id, entry := params.SetDetails(details)

		// then
		assert.Equal(t, "test-id", id)
		assert.Equal(t, bundle.RelationKeyName.String(), entry.Key)
		assert.Equal(t, model.RelationFormat_shorttext, entry.Format)
		assert.Empty(t, f.subs[testSpaceId].cache)
	})

	t.Run("OnAdded function works correctly", func(t *testing.T) {
		// given
		f := &formatFetcher{
			subs: map[string]*spaceSubscription{
				testSpaceId: {
					cache: map[domain.RelationKey]model.RelationFormat{},
				},
			},
		}

		params := f.buildSubscriptionParams(testSpaceId)

		entry := model.RelationLink{
			Key:    "custom_relation",
			Format: model.RelationFormat_date,
		}

		// when
		params.OnAdded("test-id", entry)

		// then
		assert.Equal(t, model.RelationFormat_date, f.subs[testSpaceId].cache["custom_relation"])
	})

	t.Run("OnAdded ignores bundle relations", func(t *testing.T) {
		// given
		f := &formatFetcher{
			subs: map[string]*spaceSubscription{
				testSpaceId: {
					cache: map[domain.RelationKey]model.RelationFormat{},
				},
			},
		}

		params := f.buildSubscriptionParams(testSpaceId)

		entry := model.RelationLink{
			Key:    bundle.RelationKeyName.String(),
			Format: model.RelationFormat_shorttext,
		}

		// when
		params.OnAdded("test-id", entry)

		// then
		assert.Empty(t, f.subs[testSpaceId].cache)
	})
}
