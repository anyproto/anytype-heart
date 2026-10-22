package recovery

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/net"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
)

const techSpaceId = "tech.space"

func (fx *fixture) pull(kind commonspace.PullEventKind, spaceId, peerId string, err error) {
	fx.ObservePullEvent(commonspace.PullEvent{Kind: kind, SpaceId: spaceId, PeerId: peerId, Err: err})
}

func TestTracker_AccountFetch(t *testing.T) {
	t.Run("tech space pull: waiting, attempt, failure, and a new round", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.OnTechSpaceId(techSpaceId)
		pullErr := fmt.Errorf("pull: %w", net.ErrUnableToConnect)

		// when
		fx.pull(commonspace.PullEventWaiting, techSpaceId, "", nil)
		fx.pull(commonspace.PullEventAttempt, techSpaceId, "node1", nil)
		fx.pull(commonspace.PullEventResult, techSpaceId, "node1", pullErr)
		fx.pull(commonspace.PullEventWaiting, techSpaceId, "", nil)

		// then
		ups := fx.sender.updates()
		require.Len(t, ups, 5)
		want := []pb.IsEventAccountRecoveryUpdatePayload{
			&pb.EventAccountRecoveryUpdatePayloadOfAccountFetchStarted{AccountFetchStarted: &pb.EventAccountRecoveryAccountFetchStarted{
				SpaceId: techSpaceId, Attempt: 1,
			}},
			&pb.EventAccountRecoveryUpdatePayloadOfAccountFetchStarted{AccountFetchStarted: &pb.EventAccountRecoveryAccountFetchStarted{
				SpaceId: techSpaceId, PeerId: "node1", Attempt: 1,
			}},
			&pb.EventAccountRecoveryUpdatePayloadOfAccountFetchError{AccountFetchError: &pb.EventAccountRecoveryAccountFetchError{
				PeerId: "node1", Attempt: 1,
				Error: &pb.EventAccountRecoveryErrorInfo{Class: pb.EventAccountRecovery_PeerUnreachable, Retryable: true, DebugMessage: pullErr.Error()},
			}},
			&pb.EventAccountRecoveryUpdatePayloadOfAccountFetchStarted{AccountFetchStarted: &pb.EventAccountRecoveryAccountFetchStarted{
				SpaceId: techSpaceId, Attempt: 2,
			}},
		}
		for i, w := range want {
			assert.Equal(t, w, ups[i+1].Payload, "update %d", i+1)
		}
		snap := fx.Snapshot()
		assert.True(t, snap.AccountFetchStarted)
		assert.Equal(t, int32(2), snap.AccountFetchAttempt)
		assert.Equal(t, pb.EventAccountRecovery_PeerUnreachable, snap.AccountFetchError.Class)
		assert.False(t, snap.AccountReady)
		assert.Equal(t, pb.EventAccountRecovery_LookingForPeers, snap.Phase, "no dial, no node: not even Connecting")
	})

	t.Run("FetchingAccount needs an open node connection", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.OnTechSpaceId(techSpaceId)
		fx.pull(commonspace.PullEventWaiting, techSpaceId, "", nil)
		fx.dialStarted("node1", 2)
		require.Equal(t, pb.EventAccountRecovery_Connecting, fx.Snapshot().Phase)
		fx.connected("lan1", "yamux", true, 0) // a LAN peer is not a node
		require.Equal(t, pb.EventAccountRecovery_Connecting, fx.Snapshot().Phase)

		// when
		fx.connected("node1", "quic", false, 300*time.Millisecond)

		// then
		changes := fx.phaseChanges()
		last := changes[len(changes)-1]
		assert.Equal(t, pb.EventAccountRecovery_FetchingAccount, last.Phase)
		assert.Equal(t, pb.EventAccountRecovery_Connecting, last.FromPhase)
	})

	t.Run("a deleted account is an account-level class", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.OnTechSpaceId(techSpaceId)
		fx.pull(commonspace.PullEventWaiting, techSpaceId, "", nil)

		// when
		fx.pull(commonspace.PullEventResult, techSpaceId, "node1", spacesyncproto.ErrSpaceIsDeleted)

		// then
		got := fx.lastUpdate(t).Payload.(*pb.EventAccountRecoveryUpdatePayloadOfAccountFetchError).AccountFetchError
		assert.Equal(t, pb.EventAccountRecovery_AccountDeleted, got.Error.Class)
		assert.False(t, got.Error.Retryable)
	})

	t.Run("a missing tech space is account not found", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.OnTechSpaceId(techSpaceId)
		fx.pull(commonspace.PullEventWaiting, techSpaceId, "", nil)

		// when
		fx.pull(commonspace.PullEventResult, techSpaceId, "node1", spacesyncproto.ErrSpaceMissing)

		// then
		got := fx.lastUpdate(t).Payload.(*pb.EventAccountRecoveryUpdatePayloadOfAccountFetchError).AccountFetchError
		assert.Equal(t, pb.EventAccountRecovery_AccountNotFound, got.Error.Class)
		assert.False(t, got.Error.Retryable)
	})

	t.Run("successful and cancelled results emit nothing", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.OnTechSpaceId(techSpaceId)
		fx.pull(commonspace.PullEventWaiting, techSpaceId, "", nil)
		before := len(fx.sender.updates())

		// when
		fx.pull(commonspace.PullEventResult, techSpaceId, "node1", context.Canceled)
		fx.pull(commonspace.PullEventResult, techSpaceId, "node1", nil)

		// then
		assert.Len(t, fx.sender.updates(), before)
		assert.Nil(t, fx.Snapshot().AccountFetchError)
	})

	t.Run("pull events for other spaces are space pulls, not the account fetch; unknown kinds are ignored", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)

		// when: a pull before the tech space id is known can only be filed
		// as a regular space (production always pushes the id first)
		fx.pull(commonspace.PullEventWaiting, techSpaceId, "", nil)
		fx.OnTechSpaceId(techSpaceId)
		fx.pull(commonspace.PullEventWaiting, "regular.space", "", nil)
		fx.pull(commonspace.PullEventKind(99), techSpaceId, "", nil)

		// then
		snap := fx.Snapshot()
		assert.False(t, snap.AccountFetchStarted)
		require.Len(t, snap.Spaces, 2)
		assert.Equal(t, pb.EventAccountRecovery_Pulling, snap.Spaces[0].State)
		assert.Equal(t, pb.EventAccountRecovery_Pulling, snap.Spaces[1].State)

		// and the misfiled tech entry self-corrects at AccountReady
		fx.OnAccountReady()
		tech := fx.space(t, techSpaceId)
		assert.Equal(t, pb.EventAccountRecovery_Tech, tech.Kind)
		assert.Equal(t, pb.EventAccountRecovery_Loaded, tech.State)
	})
}

func TestTracker_AccountReady(t *testing.T) {
	t.Run("fires once, loads the tech space and moves to LoadingSpaces", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.OnTechSpaceId(techSpaceId)
		fx.pull(commonspace.PullEventWaiting, techSpaceId, "", nil)
		fx.pull(commonspace.PullEventResult, techSpaceId, "node1", errors.New("first try"))
		fx.clock.Advance(2500 * time.Millisecond)

		// when
		fx.OnAccountReady()
		fx.OnAccountReady()

		// then
		ups := fx.sender.updates()
		require.Len(t, ups, 7) // Started, FetchStarted, FetchError, AccountReady, SpaceDiscovered, SpaceStateChanged, PhaseChanged
		assert.Equal(t, &pb.EventAccountRecoveryAccountReady{DurationMs: 2500},
			ups[3].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfAccountReady).AccountReady)
		assert.Equal(t, &pb.EventAccountRecoverySpaceDiscovered{SpaceId: techSpaceId, Kind: pb.EventAccountRecovery_Tech},
			ups[4].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfSpaceDiscovered).SpaceDiscovered)
		assert.Equal(t, &pb.EventAccountRecoverySpaceStateChanged{SpaceId: techSpaceId, State: pb.EventAccountRecovery_Loaded, FromState: pb.EventAccountRecovery_Queued},
			ups[5].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfSpaceStateChanged).SpaceStateChanged)
		changed := ups[6].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfPhaseChanged).PhaseChanged
		assert.Equal(t, pb.EventAccountRecovery_LoadingSpaces, changed.Phase)
		snap := fx.Snapshot()
		assert.True(t, snap.AccountReady)
		assert.Nil(t, snap.AccountFetchError, "cleared: the fetch succeeded")
		require.Len(t, snap.Spaces, 1)
		want := &pb.EventAccountRecoverySnapshotSpace{SpaceId: techSpaceId, Kind: pb.EventAccountRecovery_Tech, State: pb.EventAccountRecovery_Loaded}
		assert.Equal(t, want, snap.Spaces[0])
		assert.Equal(t, int32(1), snap.SpacesTotal)
		assert.Equal(t, int32(1), snap.SpacesLoaded)
	})

	t.Run("without a known tech space id the phase still moves", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.init(t)

		// when
		fx.OnAccountReady()

		// then
		snap := fx.Snapshot()
		assert.Equal(t, pb.EventAccountRecovery_LoadingSpaces, snap.Phase)
		assert.Empty(t, snap.Spaces)
	})

	t.Run("a warm start jumps from Connecting straight to LoadingSpaces", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.init(t)
		fx.OnTechSpaceId(techSpaceId)
		fx.dialStarted("node1", 2)

		// when
		fx.OnAccountReady()

		// then
		changes := fx.phaseChanges()
		require.Len(t, changes, 2)
		assert.Equal(t, pb.EventAccountRecovery_Connecting, changes[0].Phase)
		assert.Equal(t, pb.EventAccountRecovery_LoadingSpaces, changes[1].Phase)
		assert.Equal(t, pb.EventAccountRecovery_Connecting, changes[1].FromPhase)
	})
}

func TestClassifyAccount_NotFoundSentinel(t *testing.T) {
	got := classifyAccount(errors.Join(ErrAccountNotFound, errors.New("init personal space: space not exists")))
	require.NotNil(t, got)
	assert.Equal(t, pb.EventAccountRecovery_AccountNotFound, got.class)
	assert.False(t, got.retryable)
}
