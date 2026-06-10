package application

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountPreloadRemainingSpaces(t *testing.T) {
	t.Run("not running app returns ErrApplicationIsNotRunning", func(t *testing.T) {
		s := New()

		err := s.AccountPreloadRemainingSpaces(context.Background())

		require.ErrorIs(t, err, ErrApplicationIsNotRunning)
	})
}
