package objectstore

import (
	"testing"
	"time"

	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspace(t *testing.T) {
	t.Run("no saved workspace", func(t *testing.T) {
		s := newStoreFixture(t)

		_, err := s.GetCurrentWorkspaceId()
		require.Error(t, err)
	})

	t.Run("save and load", func(t *testing.T) {
		s := newStoreFixture(t)

		want := "workspace1"
		err := s.SetCurrentWorkspaceId(want)
		require.NoError(t, err)

		got, err := s.GetCurrentWorkspaceId()
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("remove and load", func(t *testing.T) {
		s := newStoreFixture(t)
		err := s.SetCurrentWorkspaceId("workspace1")
		require.NoError(t, err)

		err = s.RemoveCurrentWorkspaceId()
		require.NoError(t, err)

		_, err = s.GetCurrentWorkspaceId()
		require.Error(t, err)
	})
}

func TestAccountStatus(t *testing.T) {
	t.Run("no saved account status", func(t *testing.T) {
		s := newStoreFixture(t)

		_, err := s.GetAccountStatus()
		require.Error(t, err)
	})

	t.Run("save and load", func(t *testing.T) {
		s := newStoreFixture(t)

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
