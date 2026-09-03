package application

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/recovery"
	"github.com/anyproto/anytype-heart/pb"
)

func TestService_AccountRecoveryState(t *testing.T) {
	t.Run("before any account start the answer is the idle snapshot, not an error", func(t *testing.T) {
		// given
		s := New()

		// when
		got, err := s.AccountRecoveryState()

		// then
		require.NoError(t, err)
		want := recovery.IdleSnapshot()
		assert.Equal(t, want, got)
		assert.Equal(t, "", got.RunId)
		assert.Equal(t, pb.EventAccountRecovery_NotStarted, got.Phase)
	})

	t.Run("a zero-value service still answers", func(t *testing.T) {
		// given
		s := &Service{}

		// when
		got, err := s.AccountRecoveryState()

		// then
		require.NoError(t, err)
		assert.Equal(t, recovery.IdleSnapshot(), got)
	})
}
