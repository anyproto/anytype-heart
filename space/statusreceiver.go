package space

import (
	"context"

	"github.com/anyproto/any-sync/commonspace/syncstatus"
	"go.uber.org/zap"
)

type statusReceiver struct {
}

func (s *statusReceiver) UpdateNodeStatus(status syncstatus.ConnectionStatus) {
	log.With(zap.Int("nodes status", int(status))).Debug("updating node status")
}

func (s *statusReceiver) UpdateTree(ctx context.Context, treeId string, status syncstatus.SyncStatus) (err error) {
	log.With(zap.String("treeId", treeId), zap.Bool("synced", status == syncstatus.StatusSynced)).
		Debug("updating sync status")
	return nil
}
