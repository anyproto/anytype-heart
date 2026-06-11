package subscription

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// TestConcurrentSearch runs concurrent subscriptions with object-relation sorts
// on one space: required-keys computation touches the dependency service
// relation format cache and must stay under the space lock (run with -race)
func TestConcurrentSearch(t *testing.T) {
	fx := NewInternalTestService(t)
	fx.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:      domain.String("obj1"),
			bundle.RelationKeyName:    domain.String("first"),
			bundle.RelationKeyCreator: domain.StringList([]string{"creator1"}),
		},
		{
			bundle.RelationKeyId:   domain.String("creator1"),
			bundle.RelationKeyName: domain.String("creator"),
		},
	})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := fx.Search(SubscribeRequest{
				SpaceId: testSpaceId,
				SubId:   fmt.Sprintf("concurrent-%d", i),
				Sorts: []database.SortRequest{{
					RelationKey: bundle.RelationKeyCreator,
					Type:        model.BlockContentDataviewSort_Asc,
					Format:      model.RelationFormat_object,
				}},
				Keys:     []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String()},
				Internal: true,
			})
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()
}

// TestEntryProjection verifies that cached entries retain only the union of
// keys required by their subscriptions and that events stay complete for every
// subscription regardless of projection.
func TestEntryProjection(t *testing.T) {
	fx := NewInternalTestService(t)
	fx.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:          domain.String("obj1"),
			bundle.RelationKeyName:        domain.String("first"),
			bundle.RelationKeyDescription: domain.String("first description"),
			bundle.RelationKeySnippet:     domain.String("first snippet"),
			bundle.RelationKeyIconEmoji:   domain.String("📦"),
		},
	})

	resp1, err := fx.Search(SubscribeRequest{
		SpaceId:  testSpaceId,
		SubId:    "sub-name",
		Keys:     []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String()},
		Internal: true,
	})
	require.NoError(t, err)
	require.Len(t, resp1.Records, 1)
	assert.True(t, resp1.Records[0].Has(bundle.RelationKeyName))
	assert.False(t, resp1.Records[0].Has(bundle.RelationKeyDescription))

	resp2, err := fx.Search(SubscribeRequest{
		SpaceId:  testSpaceId,
		SubId:    "sub-description",
		Keys:     []string{bundle.RelationKeyId.String(), bundle.RelationKeyDescription.String()},
		Internal: true,
	})
	require.NoError(t, err)
	require.Len(t, resp2.Records, 1)
	assert.True(t, resp2.Records[0].Has(bundle.RelationKeyDescription))
	assert.False(t, resp2.Records[0].Has(bundle.RelationKeyName))

	getCachedEntryData := func(t *testing.T) *domain.Details {
		s := fx.Service.(*service)
		s.lock.Lock()
		spaceSubs := s.spaceSubs[testSpaceId]
		s.lock.Unlock()
		require.NotNil(t, spaceSubs)
		spaceSubs.m.Lock()
		defer spaceSubs.m.Unlock()
		e := spaceSubs.cache.Get("obj1")
		require.NotNil(t, e)
		return e.data
	}

	t.Run("cached entry holds only the union of required keys", func(t *testing.T) {
		data := getCachedEntryData(t)
		assert.True(t, data.Has(bundle.RelationKeyId))
		assert.True(t, data.Has(bundle.RelationKeyName))
		assert.True(t, data.Has(bundle.RelationKeyDescription))
		assert.False(t, data.Has(bundle.RelationKeySnippet), "snippet is not requested by any subscription")
		assert.False(t, data.Has(bundle.RelationKeyIconEmoji), "iconEmoji is not requested by any subscription")
	})

	collectAmendKeys := func(t *testing.T, output interface {
		Wait(ctx context.Context) ([]*pb.EventMessage, error)
	}) map[string]struct{} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		msgs, err := output.Wait(ctx)
		require.NoError(t, err)
		keys := map[string]struct{}{}
		for _, msg := range msgs {
			amend := msg.GetObjectDetailsAmend()
			require.NotNil(t, amend, "expected only amend events, got %T", msg.Value)
			for _, kv := range amend.Details {
				keys[kv.Key] = struct{}{}
			}
		}
		return keys
	}

	t.Run("both subscriptions get their keys amended after one change", func(t *testing.T) {
		fx.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:          domain.String("obj1"),
				bundle.RelationKeyName:        domain.String("second"),
				bundle.RelationKeyDescription: domain.String("second description"),
				bundle.RelationKeySnippet:     domain.String("second snippet"),
				bundle.RelationKeyIconEmoji:   domain.String("📦"),
			},
		})
		time.Sleep(batchTime)

		keys1 := collectAmendKeys(t, resp1.Output.NewCond().WithMin(1))
		assert.Contains(t, keys1, bundle.RelationKeyName.String())
		assert.NotContains(t, keys1, bundle.RelationKeySnippet.String())

		keys2 := collectAmendKeys(t, resp2.Output.NewCond().WithMin(1))
		assert.Contains(t, keys2, bundle.RelationKeyDescription.String())
		assert.NotContains(t, keys2, bundle.RelationKeySnippet.String())

		data := getCachedEntryData(t)
		assert.Equal(t, "second", data.GetString(bundle.RelationKeyName))
		assert.Equal(t, "second description", data.GetString(bundle.RelationKeyDescription))
		assert.False(t, data.Has(bundle.RelationKeySnippet))
	})

	t.Run("change of a key no subscription needs produces no events", func(t *testing.T) {
		fx.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:          domain.String("obj1"),
				bundle.RelationKeyName:        domain.String("second"),
				bundle.RelationKeyDescription: domain.String("second description"),
				bundle.RelationKeySnippet:     domain.String("third snippet"),
				bundle.RelationKeyIconEmoji:   domain.String("🚀"),
			},
		})
		time.Sleep(2 * batchTime)

		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		msgs, err := resp1.Output.NewCond().WithMin(1).Wait(ctx)
		assert.Error(t, err, "no events expected for sub-name, got %v", msgs)

		ctx2, cancel2 := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel2()
		msgs2, err := resp2.Output.NewCond().WithMin(1).Wait(ctx2)
		assert.Error(t, err, "no events expected for sub-description, got %v", msgs2)
	})
}

// TestDepEntriesRegainProjectedKeys verifies that a dependency entry served
// from the cache is refreshed from the store when another subscription's
// projection trimmed away keys this subscription needs.
func TestDepEntriesRegainProjectedKeys(t *testing.T) {
	fx := NewInternalTestService(t)
	fx.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:                  domain.String("opt1"),
			bundle.RelationKeyName:                domain.String("urgent"),
			bundle.RelationKeyRelationKey:         domain.String(bundle.RelationKeyTag.String()),
			bundle.RelationKeyRelationOptionColor: domain.String("red"),
		},
		{
			bundle.RelationKeyId:   domain.String("task1"),
			bundle.RelationKeyName: domain.String("task"),
			bundle.RelationKeyTag:  domain.StringList([]string{"opt1"}),
		},
	})

	// the first subscription caches opt1 projected to {id, name}
	respA, err := fx.Search(SubscribeRequest{
		SpaceId: testSpaceId,
		SubId:   "sub-option",
		Filters: []database.FilterRequest{{
			RelationKey: bundle.RelationKeyId,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String("opt1"),
		}},
		Keys:     []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String()},
		Internal: true,
	})
	require.NoError(t, err)
	require.Len(t, respA.Records, 1)

	// the second subscription depends on opt1 via tag and requests its color:
	// the projected cache entry must be refreshed from the store
	respB, err := fx.Search(SubscribeRequest{
		SpaceId: testSpaceId,
		SubId:   "sub-task",
		Filters: []database.FilterRequest{{
			RelationKey: bundle.RelationKeyId,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String("task1"),
		}},
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyTag.String(),
			bundle.RelationKeyRelationOptionColor.String(),
		},
		Internal: true,
	})
	require.NoError(t, err)
	require.Len(t, respB.Records, 1)
	require.Len(t, respB.Dependencies, 1)
	dep := respB.Dependencies[0]
	assert.Equal(t, "opt1", dep.GetString(bundle.RelationKeyId))
	assert.Equal(t, "red", dep.GetString(bundle.RelationKeyRelationOptionColor))
}
