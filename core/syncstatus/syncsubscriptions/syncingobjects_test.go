package syncsubscriptions

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func TestCount(t *testing.T) {
	objSubscription := NewIdSubscription(nil, bundle.RelationKeyId, subscription.SubscribeRequest{})
	objSubscription.sub = map[string]*entry[struct{}]{
		"1": newEmptyEntry[struct{}](),
		"2": newEmptyEntry[struct{}](),
		"4": newEmptyEntry[struct{}](),
	}
	syncing := &syncingObjects{
		objectSubscription: objSubscription,
	}
	cnt := syncing.SyncingObjectsCount([]string{"1", "2", "3"})
	require.Equal(t, 4, cnt)
}
