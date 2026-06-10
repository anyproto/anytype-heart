package block

import (
	"context"
	"sync"
	"testing"

	"github.com/cheggaaa/mb/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription/mock_subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func TestService_GetPriorityIds(t *testing.T) {
	const spaceId = "space1"
	const otherSpaceId = "space2"

	// seed a per-space chat-id subscription with the given ids (id field only)
	newService := func(chatIds []string, opened map[string]string) *Service {
		var records []*domain.Details
		for _, id := range chatIds {
			records = append(records, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId: domain.String(id),
			}))
		}
		sub := objectsubscription.NewIdSubscriptionFromQueue(mb.New[*pb.EventMessage](0), records)
		return &Service{
			chatSubs:   map[string]*objectsubscription.ObjectSubscription[struct{}]{spaceId: sub},
			openedObjs: &openedObjects{objects: opened, lock: &sync.Mutex{}},
		}
	}

	t.Run("chat objects come first, then opened objects in the space", func(t *testing.T) {
		// given: page1 (non-chat) is opened in this space
		svc := newService([]string{"chatA", "chatB"}, map[string]string{"page1": spaceId})

		// when
		got := svc.GetPriorityIds(spaceId)

		// then: two chats first (any order), then the opened non-chat page
		require.Len(t, got, 3)
		assert.ElementsMatch(t, []string{"chatA", "chatB"}, got[:2])
		assert.Equal(t, []string{"page1"}, got[2:])
	})

	t.Run("a chat that is also open is listed once", func(t *testing.T) {
		// given: chatA is both a chat and currently open in this space
		svc := newService([]string{"chatA", "chatB"}, map[string]string{"chatA": spaceId, "page1": spaceId})

		// when
		got := svc.GetPriorityIds(spaceId)

		// then: no duplicate chatA; chats first, then the opened non-chat page
		require.Len(t, got, 3)
		assert.ElementsMatch(t, []string{"chatA", "chatB", "page1"}, got)
		assert.Equal(t, []string{"page1"}, got[2:])
	})

	t.Run("opened objects from other spaces are excluded", func(t *testing.T) {
		// given: an object opened in a different space
		svc := newService([]string{"chatA", "chatB"}, map[string]string{"otherPage": otherSpaceId})

		// when
		got := svc.GetPriorityIds(spaceId)

		// then: only the two chats, no foreign opened object
		assert.ElementsMatch(t, []string{"chatA", "chatB"}, got)
	})

	t.Run("ReleasePriorityIds unsubscribes and drops the space subscription", func(t *testing.T) {
		// given
		svc := newService([]string{"chatA"}, map[string]string{})
		subService := mock_subscription.NewMockService(t)
		subService.EXPECT().Unsubscribe("block-chat-priority-" + spaceId).Return(nil)
		svc.subscriptionService = subService

		// when
		svc.ReleasePriorityIds(spaceId)

		// then: the subscription is gone; releasing again is a no-op
		assert.Empty(t, svc.chatSubs)
		svc.ReleasePriorityIds(spaceId)
	})

	t.Run("after Close no subscription is restarted", func(t *testing.T) {
		// given
		svc := newService([]string{"chatA"}, map[string]string{"page1": spaceId})
		require.NoError(t, svc.Close(context.Background()))

		// when: a diffsync cycle racing shutdown asks for priority ids
		got := svc.GetPriorityIds(spaceId)

		// then: chat subs are closed and not re-created, opened objects still listed
		assert.Equal(t, []string{"page1"}, got)
		assert.Empty(t, svc.chatSubs)
	})
}
