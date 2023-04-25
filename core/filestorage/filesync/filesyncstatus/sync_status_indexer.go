package filesyncstatus

import (
	"context"

	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/commonspace/syncstatus"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/getblock"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type SyncStatusIndexer interface {
	Index(fileID string, syncStatus syncstatus.SyncStatus)
	app.ComponentRunnable
}

type syncStatusIndexer struct {
	picker getblock.Picker

	closeCh chan struct{}
	indexCh chan indexSyncStatusMessage
}

type indexSyncStatusMessage struct {
	FileID     string
	SyncStatus syncstatus.SyncStatus
}

func NewSyncStatusIndexer(picker getblock.Picker) SyncStatusIndexer {
	return &syncStatusIndexer{
		picker:  picker,
		indexCh: make(chan indexSyncStatusMessage, 50),
		closeCh: make(chan struct{}),
	}
}

func (s *syncStatusIndexer) Index(fileID string, syncStatus syncstatus.SyncStatus) {
	s.indexCh <- indexSyncStatusMessage{
		FileID:     fileID,
		SyncStatus: syncStatus,
	}
}

func (s *syncStatusIndexer) Init(a *app.App) (err error) {
	return nil
}

func (s *syncStatusIndexer) Name() (name string) {
	return "sync_status_indexer"
}

func (s *syncStatusIndexer) Run(ctx context.Context) (err error) {
	go s.run()
	return nil
}

func (s *syncStatusIndexer) run() {
	for {
		select {
		case <-s.closeCh:
			return
		case msg := <-s.indexCh:
			err := getblock.Do(s.picker, msg.FileID, func(b basic.DetailsSettable) (err error) {
				return b.SetDetails(nil, []*pb.RpcObjectSetDetailsDetail{
					{
						Key:   bundle.RelationKeyFileSyncStatus.String(),
						Value: pbtypes.Float64(float64(msg.SyncStatus)),
					},
				}, true)
			})
			if err != nil {
				log.Error("failed to index sync status", zap.String("fileID", msg.FileID), zap.Error(err))
			}
		}
	}
}

func (s *syncStatusIndexer) Close(ctx context.Context) (err error) {
	close(s.closeCh)
	return nil
}
