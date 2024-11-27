package syncsubscriptions

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

type syncingObjects struct {
	objectSubscription *objectsubscription.ObjectSubscription[struct{}]
	service            subscription.Service
	spaceId            string
}

func newSyncingObjects(spaceId string, service subscription.Service) *syncingObjects {
	return &syncingObjects{
		service: service,
		spaceId: spaceId,
	}
}

func (s *syncingObjects) Run() error {
	objectReq := subscription.SubscribeRequest{
		SpaceId:           s.spaceId,
		SubId:             fmt.Sprintf("spacestatus.objects.%s", s.spaceId),
		Internal:          true,
		NoDepSubscription: true,
		Keys:              []string{bundle.RelationKeyId.String()},
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeySyncStatus.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.IntList(int(domain.SpaceSyncStatusSyncing), int(domain.ObjectSyncStatusQueued), int(domain.ObjectSyncStatusError)),
			},
		},
	}
	s.objectSubscription = objectsubscription.NewIdSubscription(s.service, objectReq)
	errObjects := s.objectSubscription.Run()
	if errObjects != nil {
		return fmt.Errorf("error running syncing objects: %w", errObjects)
	}
	return nil
}

func (s *syncingObjects) Close() {
	s.objectSubscription.Close()
}

func (s *syncingObjects) GetObjectSubscription() *objectsubscription.ObjectSubscription[struct{}] {
	return s.objectSubscription
}

func (s *syncingObjects) SyncingObjectsCount(missing []string) int {
	ids := make([]string, 0, s.objectSubscription.Len())
	s.objectSubscription.Iterate(func(id string, _ struct{}) bool {
		ids = append(ids, id)
		return true
	})
	_, added := slice.DifferenceRemovedAdded(ids, missing)
	return len(ids) + len(added)
}
