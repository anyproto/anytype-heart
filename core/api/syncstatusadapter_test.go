package api

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/files/filesync"
	"github.com/anyproto/anytype-heart/pb"
)

// fakeSpaceStatus answers per space, so a call forwarded for the wrong space
// answers wrong.
type fakeSpaceStatus map[string]statusAnswer

type statusAnswer struct {
	event   *pb.EventSpaceSyncStatusUpdate
	checked bool
}

func (f fakeSpaceStatus) SpaceStatus(spaceId string) (*pb.EventSpaceSyncStatusUpdate, bool, error) {
	answer, ok := f[spaceId]
	if !ok {
		return nil, false, errors.New("no such space")
	}
	return answer.event, answer.checked, nil
}

type fakeNetwork pb.RpcAccountNetworkMode

func (f fakeNetwork) GetNetworkMode() pb.RpcAccountNetworkMode { return pb.RpcAccountNetworkMode(f) }

type limitsAnswer struct {
	limits filesync.SpaceLimits
	ok     bool
	err    error
}

type fakeSpaceLimits map[string]limitsAnswer

func (f fakeSpaceLimits) SpaceLimits(spaceId string) (filesync.SpaceLimits, bool, error) {
	answer := f[spaceId]
	return answer.limits, answer.ok, answer.err
}

func TestSyncStatusAdapter(t *testing.T) {
	t.Run("the space's status is the app's, for that space", func(t *testing.T) {
		// given
		want := &pb.EventSpaceSyncStatusUpdate{Id: "space1", Status: pb.EventSpace_Syncing, SyncingObjectsCounter: 3}
		adapter := newSyncStatusAdapter(fakeSpaceStatus{
			"space1": {event: want, checked: true},
			"space2": {event: &pb.EventSpaceSyncStatusUpdate{Id: "space2", Status: pb.EventSpace_Synced}},
		}, fakeSpaceLimits{}, fakeNetwork(pb.RpcAccount_DefaultConfig))

		// when
		got, checked, err := adapter.SpaceSyncStatus("space1")

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
		assert.True(t, checked)
	})
	t.Run("an unchecked connection is passed on", func(t *testing.T) {
		// given
		adapter := newSyncStatusAdapter(fakeSpaceStatus{
			"space1": {event: &pb.EventSpaceSyncStatusUpdate{Id: "space1", Status: pb.EventSpace_Synced}},
		}, fakeSpaceLimits{}, fakeNetwork(pb.RpcAccount_DefaultConfig))

		// when
		_, checked, err := adapter.SpaceSyncStatus("space1")

		// then
		require.NoError(t, err)
		assert.False(t, checked)
	})
	t.Run("the network is the account's", func(t *testing.T) {
		for mode, want := range map[pb.RpcAccountNetworkMode]pb.EventSpaceNetwork{
			pb.RpcAccount_DefaultConfig: pb.EventSpace_Anytype,
			pb.RpcAccount_CustomConfig:  pb.EventSpace_SelfHost,
			pb.RpcAccount_LocalOnly:     pb.EventSpace_LocalOnly,
		} {
			adapter := newSyncStatusAdapter(fakeSpaceStatus{}, fakeSpaceLimits{}, fakeNetwork(mode))
			assert.Equal(t, want, adapter.Network(), mode.String())
		}
	})
	t.Run("a failed status read names the operation", func(t *testing.T) {
		// given
		adapter := newSyncStatusAdapter(fakeSpaceStatus{}, fakeSpaceLimits{}, fakeNetwork(pb.RpcAccount_DefaultConfig))

		// when
		_, _, err := adapter.SpaceSyncStatus("space1")

		// then
		require.ErrorContains(t, err, "space status: no such space")
	})
	t.Run("a reported quota, for that space", func(t *testing.T) {
		// given
		at := time.Unix(1_700_000_000, 0)
		adapter := newSyncStatusAdapter(fakeSpaceStatus{}, fakeSpaceLimits{
			"space1": {limits: filesync.SpaceLimits{
				LimitBytes: 5 << 30, NodeUsageBytes: 400, ReportedAt: at, PlanLimit: true,
				Confirmed: map[string]filesync.ConfirmedUpload{"file1": {Bytes: 7, InReport: true}},
			}, ok: true},
			"space2": {limits: filesync.SpaceLimits{LimitBytes: 1}, ok: true},
		}, fakeNetwork(pb.RpcAccount_DefaultConfig))
		want := apicore.SpaceStorage{
			LimitBytes: 5 << 30, NodeUsedBytes: 400, ReportedAt: at, PlanLimit: true,
			Confirmed: map[string]apicore.ConfirmedUpload{"file1": {Bytes: 7, InReport: true}},
		}

		// when
		got, ok, err := adapter.SpaceStorage("space1")

		// then
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, want, got)
	})
	t.Run("no report yet", func(t *testing.T) {
		// given
		adapter := newSyncStatusAdapter(fakeSpaceStatus{}, fakeSpaceLimits{
			"space1": {limits: filesync.SpaceLimits{LimitBytes: 1}},
		}, fakeNetwork(pb.RpcAccount_DefaultConfig))

		// when
		got, ok, err := adapter.SpaceStorage("space1")

		// then
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Equal(t, apicore.SpaceStorage{}, got)
	})
	t.Run("a failure names the space", func(t *testing.T) {
		// given
		cause := errors.New("spaceView not found")
		adapter := newSyncStatusAdapter(fakeSpaceStatus{}, fakeSpaceLimits{"space1": {err: cause}}, fakeNetwork(pb.RpcAccount_DefaultConfig))

		// when
		_, ok, err := adapter.SpaceStorage("space1")

		// then
		require.ErrorIs(t, err, cause)
		assert.Contains(t, err.Error(), "space1")
		assert.False(t, ok)
	})
}
