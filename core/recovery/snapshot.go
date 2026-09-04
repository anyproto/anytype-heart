package recovery

import (
	"sort"

	"github.com/anyproto/anytype-heart/pb"
)

// buildSnapshotLocked is the ONE builder behind both the RecoveryState RPC and
// the session-hook Snapshot, over the same state the push events mutate.
func (t *Tracker) buildSnapshotLocked() *pb.EventAccountRecoverySnapshot {
	snap := &pb.EventAccountRecoverySnapshot{
		RunId:               t.run.runId,
		LastEventId:         t.nextId - 1,
		Mode:                t.run.mode,
		NetworkId:           t.run.networkId,
		StartedAtMs:         t.run.startedAt.UnixMilli(),
		Phase:               t.phase.current,
		PhaseStartedAtMs:    t.phase.startedAt.UnixMilli(),
		Done:                t.terminalLocked(),
		Error:               t.phase.failed.toPb(),
		Discovery:           t.net.discovery,
		AccountFetchStarted: t.account.fetchStarted,
		AccountFetchAttempt: int32(t.account.attempt),
		AccountFetchError:   t.account.lastError.toPb(),
		AccountReady:        t.account.ready,
		ViewsConfirmed:      t.phase.viewsConfirmed,
		LocalPeers:          t.net.localPeers,
	}
	peerIds := make([]string, 0, len(t.net.peers))
	for id := range t.net.peers {
		peerIds = append(peerIds, id)
	}
	sort.Strings(peerIds)
	for _, id := range peerIds {
		p := t.net.peers[id]
		snap.Peers = append(snap.Peers, &pb.EventAccountRecoverySnapshotPeer{
			PeerId:            id,
			Kind:              p.kind,
			NodeTypes:         p.nodeTypes,
			OpenConnections:   int32(p.open),
			Transport:         p.lastTransport,
			ProtoVersion:      p.lastProto,
			DialAttempts:      int32(p.dialAttempts),
			LastError:         p.lastError.toPb(),
			DiscoveredLocally: p.discoveredLocally,
			Exchanged:         p.exchanged,
			HasAccountSpace:   p.hasAccountSpace,
			SharedSpaceCount:  int32(p.sharedSpaceCount),
		})
	}
	spaceIds := make([]string, 0, len(t.spaces))
	for id := range t.spaces {
		spaceIds = append(spaceIds, id)
	}
	sort.Strings(spaceIds)
	for _, id := range spaceIds {
		s := t.spaces[id]
		snap.Spaces = append(snap.Spaces, &pb.EventAccountRecoverySnapshotSpace{
			SpaceId:     id,
			SpaceViewId: s.spaceViewId,
			Kind:        s.kind,
			State:       s.state,
			Error:       s.lastError.toPb(),
			Attempt:     int32(s.attempt),
		})
		snap.SpacesTotal++
		switch s.state {
		case pb.EventAccountRecovery_Loaded:
			snap.SpacesLoaded++
		case pb.EventAccountRecovery_Error:
			snap.SpacesFailed++
		}
	}
	return snap
}
