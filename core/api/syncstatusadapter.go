package api

import (
	"fmt"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/files/filesync"
	"github.com/anyproto/anytype-heart/core/syncstatus/spacesyncstatus"
	"github.com/anyproto/anytype-heart/pb"
)

// syncStatusAdapter implements apicore.SyncStatus over the space sync status
// the app is sent (without its storage warning, which reads the account's
// usage) and the per-space quota the file uploader checks. None of its reads
// waits on the backup node.
type syncStatusAdapter struct {
	status  spaceStatusSource
	limits  spaceLimitsSource
	network networkModeSource
}

type spaceStatusSource interface {
	SpaceStatus(spaceId string) (*pb.EventSpaceSyncStatusUpdate, bool, error)
}

type spaceLimitsSource interface {
	SpaceLimits(spaceId string) (filesync.SpaceLimits, bool, error)
}

type networkModeSource interface {
	GetNetworkMode() pb.RpcAccountNetworkMode
}

var (
	_ spaceStatusSource = (spacesyncstatus.Updater)(nil)
	_ spaceLimitsSource = (filesync.FileSync)(nil)
	_ networkModeSource = (spacesyncstatus.NetworkConfig)(nil)
)

func newSyncStatusAdapter(status spaceStatusSource, limits spaceLimitsSource, network networkModeSource) apicore.SyncStatus {
	return &syncStatusAdapter{status: status, limits: limits, network: network}
}

func (a *syncStatusAdapter) Network() pb.EventSpaceNetwork {
	return spacesyncstatus.MapNetworkMode(a.network.GetNetworkMode())
}

func (a *syncStatusAdapter) SpaceSyncStatus(spaceId string) (*pb.EventSpaceSyncStatusUpdate, bool, error) {
	status, checked, err := a.status.SpaceStatus(spaceId)
	if err != nil {
		return nil, false, fmt.Errorf("space status: %w", err)
	}
	return status, checked, nil
}

func (a *syncStatusAdapter) SpaceStorage(spaceId string) (apicore.SpaceStorage, bool, error) {
	limits, ok, err := a.limits.SpaceLimits(spaceId)
	if err != nil {
		return apicore.SpaceStorage{}, false, fmt.Errorf("space limits of %s: %w", spaceId, err)
	}
	if !ok {
		return apicore.SpaceStorage{}, false, nil
	}
	confirmed := make(map[string]apicore.ConfirmedUpload, len(limits.Confirmed))
	for id, upload := range limits.Confirmed {
		confirmed[id] = apicore.ConfirmedUpload{Bytes: upload.Bytes, InReport: upload.InReport}
	}
	return apicore.SpaceStorage{
		LimitBytes:    limits.LimitBytes,
		NodeUsedBytes: limits.NodeUsageBytes,
		Confirmed:     confirmed,
		ReportedAt:    limits.ReportedAt,
		PlanLimit:     limits.PlanLimit,
	}, true, nil
}
