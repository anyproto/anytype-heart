package application

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
)

func TestService_AccountSelect(t *testing.T) {
	t.Run("account select finish with error", func(t *testing.T) {
		// given
		s := New()
		dir := t.TempDir()
		s.SetClientVersion("platform", "1")
		mnemonic, err := core.WalletGenerateMnemonic(wordCount)
		assert.NoError(t, err)
		account, err := core.WalletAccountAt(mnemonic, 0)
		assert.NoError(t, err)
		s.derivedKeys = &account
		expectedDir := filepath.Join(dir, account.Identity.GetPublic().Account())

		sender := mock_event.NewMockSender(t)
		sender.EXPECT().Name().Return("service")
		ctx := context.Background()
		sender.EXPECT().Init(mock.Anything).Return(ErrFailedToStartApplication)
		// the recovery tracker is registered after the sender, so its Init never
		// runs; Fail must still publish Started and then the Failed verdict
		var broadcasts []*pb.Event
		sender.EXPECT().Broadcast(mock.Anything).Run(func(ev *pb.Event) {
			broadcasts = append(broadcasts, ev)
		}).Times(2)
		s.eventSender = sender

		// when
		_, err = s.AccountSelect(ctx, &pb.RpcAccountSelectRequest{Id: account.Identity.GetPublic().Account(), RootPath: dir})

		// then
		assert.NotNil(t, err)
		_, err = os.Stat(expectedDir)
		assert.True(t, os.IsNotExist(err))
		require.Len(t, broadcasts, 2)
		started := recoveryUpdate(t, broadcasts[0]).Payload.(*pb.EventAccountRecoveryUpdatePayloadOfStarted).Started
		assert.Equal(t, pb.EventAccountRecovery_ColdRecovery, started.Mode)
		failed := recoveryUpdate(t, broadcasts[1]).Payload.(*pb.EventAccountRecoveryUpdatePayloadOfPhaseChanged).PhaseChanged
		assert.Equal(t, pb.EventAccountRecovery_Failed, failed.Phase)
		snapshot, err := s.AccountRecoveryState()
		require.NoError(t, err)
		assert.True(t, snapshot.Done)
		assert.Equal(t, int64(2), snapshot.LastEventId)
	})
}

func recoveryUpdate(t *testing.T, ev *pb.Event) *pb.EventAccountRecoveryUpdate {
	t.Helper()
	require.Len(t, ev.Messages, 1)
	value, ok := ev.Messages[0].Value.(*pb.EventMessageValueOfAccountRecoveryUpdate)
	require.True(t, ok)
	return value.AccountRecoveryUpdate
}
