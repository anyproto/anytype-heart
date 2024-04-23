package syncstatus

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
)

type NetworkConfig interface {
	GetNetworkMode() pb.RpcAccountNetworkMode
}

type UpdateSender interface {
	app.ComponentRunnable
	SendUpdate(status pb.EventSpaceStatus, objectsNumber int64, syncError pb.EventSpaceSyncError, isFilesSync, isObjectSync bool)
}

type spaceSyncStatus struct {
	eventSender   event.Sender
	networkConfig NetworkConfig
	batcher       *mb.MB[*pb.EventSpaceSyncStatusUpdate]

	mu                              sync.Mutex
	isFilesSyncing, isObjectSyncing bool
}

func (s *spaceSyncStatus) SendUpdate(status pb.EventSpaceStatus, objectsNumber int64, syncError pb.EventSpaceSyncError, isFilesSync, isObjectSync bool) {
	if status == pb.EventSpace_Synced && (s.isFilesSyncing || s.isObjectSyncing) {
		if isFilesSync {
			s.mu.Lock()
			s.isFilesSyncing = false
			s.mu.Unlock()
		}
		if isObjectSync {
			s.mu.Lock()
			s.isObjectSyncing = false
			s.mu.Unlock()
		}
		return
	}

	if isFilesSync {
		s.mu.Lock()
		s.isFilesSyncing = true
		s.mu.Unlock()
	}
	if isObjectSync {
		s.mu.Lock()
		s.isObjectSyncing = true
		s.mu.Unlock()
	}

	e := s.batcher.Add(context.Background(), &pb.EventSpaceSyncStatusUpdate{
		Status:                status,
		Network:               mapNetworkMode(s.networkConfig.GetNetworkMode()),
		Error:                 syncError,
		SyncingObjectsCounter: objectsNumber,
	})
	if e != nil {
		log.Errorf("failed to add space sync event to queue %s", e)
	}
}

func NewSpaceSyncStatus() UpdateSender {
	return &spaceSyncStatus{batcher: mb.New[*pb.EventSpaceSyncStatusUpdate](0)}
}

func (s *spaceSyncStatus) Init(a *app.App) (err error) {
	s.eventSender = app.MustComponent[event.Sender](a)
	s.networkConfig = app.MustComponent[NetworkConfig](a)
	return
}

func (s *spaceSyncStatus) Name() (name string) {
	return "spaceSyncStatus"
}

func (s *spaceSyncStatus) Run(ctx context.Context) (err error) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		msg, err := s.batcher.WaitOne(context.Background())
		if err != nil {
			return err
		}
		s.eventSender.Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{
				Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
					SpaceSyncStatusUpdate: msg,
				},
			}},
		})
	}
}

func (s *spaceSyncStatus) Close(ctx context.Context) (err error) {
	return s.batcher.Close()
}

func mapNetworkMode(mode pb.RpcAccountNetworkMode) pb.EventSpaceNetwork {
	switch mode {
	case pb.RpcAccount_LocalOnly:
		return pb.EventSpace_LocalOnly
	case pb.RpcAccount_CustomConfig:
		return pb.EventSpace_SelfHost
	default:
		return pb.EventSpace_Anytype
	}
}
