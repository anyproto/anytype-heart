package syncsubscriptions

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

type syncingObjects struct {
	objectSubscription         *objectsubscription.ObjectSubscription[struct{}]
	notSyncedFilesSubscription *objectsubscription.ObjectSubscription[struct{}]
	service                    subscription.Service
	spaceId                    string
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
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeySyncStatus,
				Condition:   model.BlockContentDataviewFilter_In,
				Value: domain.Int64List([]int64{
					int64(domain.SpaceSyncStatusSyncing),
					int64(domain.ObjectSyncStatusQueued),
					int64(domain.ObjectSyncStatusError),
				}),
			},
			{
				RelationKey: bundle.RelationKeySyncError,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Int64(domain.SyncErrorOversized),
			},
		},
	}
	s.objectSubscription = objectsubscription.NewIdSubscription(s.service, objectReq)
	errObjects := s.objectSubscription.Run()
	if errObjects != nil {
		return fmt.Errorf("error running syncing objects: %w", errObjects)
	}

	filesReq := subscription.SubscribeRequest{
		SpaceId:           s.spaceId,
		SubId:             fmt.Sprintf("spacestatus.notSyncedFiles.%s", s.spaceId),
		Internal:          true,
		NoDepSubscription: true,
		Keys:              []string{bundle.RelationKeyId.String()},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyFileBackupStatus,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Int64(filesyncstatus.Synced),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List(domain.FileLayouts),
			},
		},
	}
	s.notSyncedFilesSubscription = objectsubscription.NewIdSubscription(s.service, filesReq)
	err := s.notSyncedFilesSubscription.Run()
	if err != nil {
		return fmt.Errorf("run not synced files sub: %w", err)
	}
	return nil
}

func (s *syncingObjects) Close() {
	s.objectSubscription.Close()
	s.notSyncedFilesSubscription.Close()
}

func (s *syncingObjects) GetObjectSubscription() *objectsubscription.ObjectSubscription[struct{}] {
	return s.objectSubscription
}

func (s *syncingObjects) NotSyncedFilesCount() int {
	var count int
	s.notSyncedFilesSubscription.Iterate(func(id string, _ struct{}) bool {
		count++
		return true
	})
	return count
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
