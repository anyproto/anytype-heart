package syncsubscritions

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

type syncingObjects struct {
	fileSubscription   *ObjectSubscription[struct{}]
	objectSubscription *ObjectSubscription[struct{}]
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
		SubId:             fmt.Sprintf("spacestatus.objects.%s", s.spaceId),
		Internal:          true,
		NoDepSubscription: true,
		Keys:              []string{bundle.RelationKeyId.String()},
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeySyncStatus.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(domain.SpaceSyncStatusSyncing)),
			},
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_NotIn,
				Value: pbtypes.IntList(
					int(model.ObjectType_file),
					int(model.ObjectType_image),
					int(model.ObjectType_video),
					int(model.ObjectType_audio),
				),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(s.spaceId),
			},
		},
	}
	fileReq := subscription.SubscribeRequest{
		SubId:             fmt.Sprintf("spacestatus.files.%s", s.spaceId),
		Internal:          true,
		NoDepSubscription: true,
		Keys:              []string{bundle.RelationKeyId.String()},
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyFileBackupStatus.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.IntList(int(filesyncstatus.Syncing), int(filesyncstatus.Queued)),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(s.spaceId),
			},
		},
	}
	s.fileSubscription = NewIdSubscription(s.service, fileReq)
	s.objectSubscription = NewIdSubscription(s.service, objectReq)
	errFiles := s.fileSubscription.Run()
	errObjects := s.objectSubscription.Run()
	if errFiles != nil || errObjects != nil {
		return fmt.Errorf("error running syncing objects: %v %v", errFiles, errObjects)
	}
	return nil
}

func (s *syncingObjects) Close() {
	s.fileSubscription.Close()
	s.objectSubscription.Close()
}

func (s *syncingObjects) GetFileSubscription() *ObjectSubscription[struct{}] {
	return s.fileSubscription
}

func (s *syncingObjects) GetObjectSubscription() *ObjectSubscription[struct{}] {
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

func (s *syncingObjects) FileSyncingObjectsCount() int {
	return s.fileSubscription.Len()
}
