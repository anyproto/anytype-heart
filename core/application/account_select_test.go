package application

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

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
		s.eventSender = sender

		// when
		_, err = s.AccountSelect(ctx, &pb.RpcAccountSelectRequest{Id: account.Identity.GetPublic().Account(), RootPath: dir})

		// then
		assert.NotNil(t, err)
		_, err = os.Stat(expectedDir)
		assert.True(t, os.IsNotExist(err))
	})
}
