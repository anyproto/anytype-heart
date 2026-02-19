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
	limitedFilesSubscription   *objectsubscription.ObjectSubscription[struct{}]
	uploadingFilesSubscription *objectsubscription.ObjectSubscription[struct{}]
	subscriptionService        subscription.Service
	spaceId                    string
	myParticipantId            string
}

func newSyncingObjects(spaceId string, subService subscription.Service, myParticipantId string) *syncingObjects {
	return &syncingObjects{
		subscriptionService: subService,
		spaceId:             spaceId,
		myParticipantId:     myParticipantId,
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
					int64(domain.ObjectSyncStatusSyncing),
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
	s.objectSubscription = objectsubscription.NewIdSubscription(s.subscriptionService, objectReq)
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
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(filesyncstatus.Limited),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List(domain.FileLayouts),
			},
		},
	}
	s.limitedFilesSubscription = objectsubscription.NewIdSubscription(s.subscriptionService, filesReq)
	err := s.limitedFilesSubscription.Run()
	if err != nil {
		return fmt.Errorf("run not synced files sub: %w", err)
	}

	uplFilesReq := subscription.SubscribeRequest{
		SpaceId:           s.spaceId,
		SubId:             fmt.Sprintf("spacestatus.uploadingFiles.%s", s.spaceId),
		Internal:          true,
		NoDepSubscription: true,
		Keys:              []string{bundle.RelationKeyId.String()},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyCreator,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(s.myParticipantId),
			},
			{
				RelationKey: bundle.RelationKeyFileBackupStatus,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List([]filesyncstatus.Status{filesyncstatus.Syncing, filesyncstatus.Queued}),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List(domain.FileLayouts),
			},
		},
	}
	s.uploadingFilesSubscription = objectsubscription.NewIdSubscription(s.subscriptionService, uplFilesReq)
	err = s.uploadingFilesSubscription.Run()
	if err != nil {
		return fmt.Errorf("run uploading files sub: %w", err)
	}
	return nil
}

func (s *syncingObjects) Close() {
	s.objectSubscription.Close()
	s.limitedFilesSubscription.Close()
}

func (s *syncingObjects) GetObjectSubscription() *objectsubscription.ObjectSubscription[struct{}] {
	return s.objectSubscription
}

func (s *syncingObjects) LimitedFilesCount() int {
	return s.limitedFilesSubscription.Len()
}

func (s *syncingObjects) UploadingFilesCount() int {
	return s.uploadingFilesSubscription.Len()
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
