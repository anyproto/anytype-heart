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
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestService_GetPriorityIds(t *testing.T) {
	const spaceId = "space1"
	const otherSpaceId = "space2"

	record := func(id string, layout model.ObjectTypeLayout) *domain.Details {
		return domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:             domain.String(id),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(layout)),
		})
	}

	// seed a per-space priority subscription with the given records
	newService := func(records []*domain.Details, opened map[string]string) *Service {
		sub := objectsubscription.NewFromQueue(mb.New[*pb.EventMessage](0), prioritySubParams, records)
		return &Service{
			prioritySubs: map[string]*objectsubscription.ObjectSubscription[int64]{spaceId: sub},
			openedObjs:   &openedObjects{objects: opened, lock: &sync.Mutex{}},
		}
	}

	t.Run("chat objects come first, then opened objects in the space", func(t *testing.T) {
		// given: page1 (non-chat) is opened in this space
		svc := newService([]*domain.Details{
			record("chatA", model.ObjectType_chatDerived),
			record("chatB", model.ObjectType_chatDerived),
		}, map[string]string{"page1": spaceId})

		// when
		got := svc.GetPriorityIds(spaceId)

		// then: two chats first (any order), then the opened non-chat page
		require.Len(t, got, 3)
		assert.ElementsMatch(t, []string{"chatA", "chatB"}, got[:2])
		assert.Equal(t, []string{"page1"}, got[2:])
	})

	t.Run("chatDerived, then discussions, then files, then opened objects", func(t *testing.T) {
		// given: a mix of chats, discussions and files, plus an opened page
		svc := newService([]*domain.Details{
			record("fileA", model.ObjectType_file),
			record("discA", model.ObjectType_discussion),
			record("chatA", model.ObjectType_chatDerived),
			record("imageA", model.ObjectType_image),
			record("discB", model.ObjectType_discussion),
			record("chatB", model.ObjectType_chatDerived),
		}, map[string]string{"page1": spaceId})

		// when
		got := svc.GetPriorityIds(spaceId)

		// then: chatDerived first, discussions next, files after, opened page last
		require.Len(t, got, 7)
		assert.ElementsMatch(t, []string{"chatA", "chatB"}, got[:2])
		assert.ElementsMatch(t, []string{"discA", "discB"}, got[2:4])
		assert.ElementsMatch(t, []string{"fileA", "imageA"}, got[4:6])
		assert.Equal(t, []string{"page1"}, got[6:])
	})

	t.Run("a chat that is also open is listed once", func(t *testing.T) {
		// given: chatA is both a chat and currently open in this space
		svc := newService([]*domain.Details{
			record("chatA", model.ObjectType_chatDerived),
			record("chatB", model.ObjectType_chatDerived),
		}, map[string]string{"chatA": spaceId, "page1": spaceId})

		// when
		got := svc.GetPriorityIds(spaceId)

		// then: no duplicate chatA; chats first, then the opened non-chat page
		require.Len(t, got, 3)
		assert.ElementsMatch(t, []string{"chatA", "chatB", "page1"}, got)
		assert.Equal(t, []string{"page1"}, got[2:])
	})

	t.Run("opened objects from other spaces are excluded", func(t *testing.T) {
		// given: an object opened in a different space
		svc := newService([]*domain.Details{
			record("chatA", model.ObjectType_chatDerived),
			record("chatB", model.ObjectType_chatDerived),
		}, map[string]string{"otherPage": otherSpaceId})

		// when
		got := svc.GetPriorityIds(spaceId)

		// then: only the two chats, no foreign opened object
		assert.ElementsMatch(t, []string{"chatA", "chatB"}, got)
	})

	t.Run("ReleasePriorityIds unsubscribes and drops the space subscription", func(t *testing.T) {
		// given
		svc := newService([]*domain.Details{record("chatA", model.ObjectType_chatDerived)}, map[string]string{})
		subService := mock_subscription.NewMockService(t)
		subService.EXPECT().Unsubscribe(prioritySubId(spaceId)).Return(nil)
		svc.subscriptionService = subService

		// when
		svc.ReleasePriorityIds(spaceId)

		// then: the subscription is gone; releasing again is a no-op
		assert.Empty(t, svc.prioritySubs)
		svc.ReleasePriorityIds(spaceId)
	})

	t.Run("after Close no subscription is restarted", func(t *testing.T) {
		// given
		svc := newService([]*domain.Details{record("chatA", model.ObjectType_chatDerived)}, map[string]string{"page1": spaceId})
		require.NoError(t, svc.Close(context.Background()))

		// when: a diffsync cycle racing shutdown asks for priority ids
		got := svc.GetPriorityIds(spaceId)

		// then: priority subs are closed and not re-created, opened objects still listed
		assert.Equal(t, []string{"page1"}, got)
		assert.Empty(t, svc.prioritySubs)
	})
}
