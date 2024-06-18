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
		updater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.Error, domain.StorageLimitExceed, domain.Files)).Return()
		s.sendSpaceStatusUpdate(filesyncstatus.Limited, "spaceId")
	})
	t.Run("file synced", func(t *testing.T) {
		// given
		updater := mock_spacesyncstatus.NewMockUpdater(t)
		s := &service{
			spaceSyncStatus: updater,
		}

		// when
		updater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.Synced, domain.Null, domain.Files)).Return()
		s.sendSpaceStatusUpdate(filesyncstatus.Synced, "spaceId")
	})
	t.Run("file queued", func(t *testing.T) {
		// given
		updater := mock_spacesyncstatus.NewMockUpdater(t)
		s := &service{
			spaceSyncStatus: updater,
		}

		// when
		updater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.Syncing, domain.Null, domain.Files)).Return()
		s.sendSpaceStatusUpdate(filesyncstatus.Queued, "spaceId")
	})
	t.Run("file syncing", func(t *testing.T) {
		// given
		updater := mock_spacesyncstatus.NewMockUpdater(t)
		s := &service{
			spaceSyncStatus: updater,
		}

		// when
		updater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.Syncing, domain.Null, domain.Files)).Return()
		s.sendSpaceStatusUpdate(filesyncstatus.Syncing, "spaceId")
	})
	t.Run("file unknown status", func(t *testing.T) {
		// given
		updater := mock_spacesyncstatus.NewMockUpdater(t)
		s := &service{
			spaceSyncStatus: updater,
		}

		// when
		updater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.Error, domain.NetworkError, domain.Files)).Return()
		s.sendSpaceStatusUpdate(filesyncstatus.Unknown, "spaceId")
	})

}
