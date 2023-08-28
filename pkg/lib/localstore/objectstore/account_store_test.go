package objectstore

import (
	"testing"
	"time"

	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountStatus(t *testing.T) {
	t.Run("no saved account status", func(t *testing.T) {
		s := NewStoreFixture(t)

		_, err := s.GetAccountStatus()
		require.Error(t, err)
	})

	t.Run("save and load", func(t *testing.T) {
		s := NewStoreFixture(t)

		want := &coordinatorproto.SpaceStatusPayload{
			Status:            coordinatorproto.SpaceStatus_SpaceStatusDeleted,
			DeletionTimestamp: time.Now().Unix(),
		}
		err := s.SaveAccountStatus(want)
		require.NoError(t, err)

		got, err := s.GetAccountStatus()
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})
}
