package syncstatus

import (
	"testing"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/spacesyncstatus/mock_spacesyncstatus"
)

func Test_sendSpaceStatusUpdate(t *testing.T) {
	t.Run("file limited", func(t *testing.T) {
		// given
		updater := mock_spacesyncstatus.NewMockUpdater(t)
		s := &service{
			spaceSyncStatus: updater,
		}

		// when
		updater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.SpaceSyncStatusError, domain.SyncErrorStorageLimitExceed)).Return()
		s.sendSpaceStatusUpdate(filesyncstatus.Limited, "spaceId", 0)
	})
	t.Run("file limited, but over 1% of storage is available", func(t *testing.T) {
		// given
		updater := mock_spacesyncstatus.NewMockUpdater(t)
		s := &service{
			spaceSyncStatus: updater,
		}

		// when
		updater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.SpaceSyncStatusSynced, domain.SyncErrorNull)).Return()
		s.sendSpaceStatusUpdate(filesyncstatus.Limited, "spaceId", 0.9)
	})
	t.Run("file synced", func(t *testing.T) {
		// given
		updater := mock_spacesyncstatus.NewMockUpdater(t)
		s := &service{
			spaceSyncStatus: updater,
		}

		// when
		updater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.SpaceSyncStatusSynced, domain.SyncErrorNull)).Return()
		s.sendSpaceStatusUpdate(filesyncstatus.Synced, "spaceId", 0)
	})
	t.Run("file queued", func(t *testing.T) {
		// given
		updater := mock_spacesyncstatus.NewMockUpdater(t)
		s := &service{
			spaceSyncStatus: updater,
		}

		// when
		updater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.SpaceSyncStatusSyncing, domain.SyncErrorNull)).Return()
		s.sendSpaceStatusUpdate(filesyncstatus.Queued, "spaceId", 0)
	})
	t.Run("file syncing", func(t *testing.T) {
		// given
		updater := mock_spacesyncstatus.NewMockUpdater(t)
		s := &service{
			spaceSyncStatus: updater,
		}

		// when
		updater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.SpaceSyncStatusSyncing, domain.SyncErrorNull)).Return()
		s.sendSpaceStatusUpdate(filesyncstatus.Syncing, "spaceId", 0)
	})
	t.Run("file unknown status", func(t *testing.T) {
		// given
		updater := mock_spacesyncstatus.NewMockUpdater(t)
		s := &service{
			spaceSyncStatus: updater,
		}

		// when
		updater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.SpaceSyncStatusError, domain.SyncErrorNetworkError)).Return()
		s.sendSpaceStatusUpdate(filesyncstatus.Unknown, "spaceId", 0)
	})

}
