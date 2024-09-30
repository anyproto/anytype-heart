package inviteservice

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrors(t *testing.T) {
	t.Run("removeInviteError", func(t *testing.T) {
		err := removeInviteError("test", nil)
		require.True(t, errors.Is(err, ErrInviteRemove))
	})
	t.Run("generateInviteError", func(t *testing.T) {
		err := generateInviteError("test", nil)
		require.True(t, errors.Is(err, ErrInviteGenerate))
	})
	t.Run("getInviteError", func(t *testing.T) {
		err := getInviteError("test", nil)
		require.True(t, errors.Is(err, ErrInviteGet))
	})
	t.Run("badContentError", func(t *testing.T) {
		err := badContentError("test", nil)
		require.True(t, errors.Is(err, ErrInviteBadContent))
	})
}
