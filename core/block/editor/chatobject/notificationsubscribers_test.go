package chatobject

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func TestNotificationSubscribers(t *testing.T) {
	ctx := context.Background()

	t.Run("add one subscriber", func(t *testing.T) {
		fx := newFixture(t)

		err := fx.AddNotificationSubscriber(ctx, "identityA")
		require.NoError(t, err)

		got, err := fx.GetNotificationSubscribers(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{"identityA"}, got)

		detail := fx.CombinedDetails().Get(bundle.RelationKeyNotificationSubscribers).StringList()
		assert.Equal(t, []string{domain.NewParticipantId(testSpaceId, "identityA")}, detail)
	})

	t.Run("remove subscriber", func(t *testing.T) {
		fx := newFixture(t)

		err := fx.AddNotificationSubscriber(ctx, "identityA")
		require.NoError(t, err)

		err = fx.RemoveNotificationSubscriber(ctx, "identityA")
		require.NoError(t, err)

		got, err := fx.GetNotificationSubscribers(ctx)
		require.NoError(t, err)
		assert.Empty(t, got)

		detail := fx.CombinedDetails().Get(bundle.RelationKeyNotificationSubscribers).StringList()
		assert.Empty(t, detail)
	})

	t.Run("add same identity twice is idempotent (Set semantics)", func(t *testing.T) {
		fx := newFixture(t)

		err := fx.AddNotificationSubscriber(ctx, "identityA")
		require.NoError(t, err)
		err = fx.AddNotificationSubscriber(ctx, "identityA")
		require.NoError(t, err)

		got, err := fx.GetNotificationSubscribers(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{"identityA"}, got)

		detail := fx.CombinedDetails().Get(bundle.RelationKeyNotificationSubscribers).StringList()
		assert.Equal(t, []string{domain.NewParticipantId(testSpaceId, "identityA")}, detail)
	})

	t.Run("add two different identities", func(t *testing.T) {
		fx := newFixture(t)

		err := fx.AddNotificationSubscriber(ctx, "identityA")
		require.NoError(t, err)
		err = fx.AddNotificationSubscriber(ctx, "identityB")
		require.NoError(t, err)

		got, err := fx.GetNotificationSubscribers(ctx)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"identityA", "identityB"}, got)

		detail := fx.CombinedDetails().Get(bundle.RelationKeyNotificationSubscribers).StringList()
		want := []string{
			domain.NewParticipantId(testSpaceId, "identityA"),
			domain.NewParticipantId(testSpaceId, "identityB"),
		}
		assert.ElementsMatch(t, want, detail)
	})

	t.Run("remove only targeted identity", func(t *testing.T) {
		fx := newFixture(t)

		err := fx.AddNotificationSubscriber(ctx, "identityA")
		require.NoError(t, err)
		err = fx.AddNotificationSubscriber(ctx, "identityB")
		require.NoError(t, err)

		err = fx.RemoveNotificationSubscriber(ctx, "identityA")
		require.NoError(t, err)

		got, err := fx.GetNotificationSubscribers(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{"identityB"}, got)
	})

	t.Run("remove non-existent subscriber is a no-op", func(t *testing.T) {
		fx := newFixture(t)

		err := fx.RemoveNotificationSubscriber(ctx, "identityA")
		require.NoError(t, err)
	})

	t.Run("add with empty identity returns error", func(t *testing.T) {
		fx := newFixture(t)

		err := fx.AddNotificationSubscriber(ctx, "")
		require.Error(t, err)
	})
}
