package detailsupdater

import (
	"time"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const CName = "core.syncstatus.objectsyncstatus.updater"

type Updater interface {
	app.Component
	UpdateDetails(objectId string, status domain.SyncStatus, syncError domain.SyncError) error
}

type syncStatusUpdater struct {
	objectGetter cache.ObjectGetter
}

func (u *syncStatusUpdater) Init(a *app.App) (err error) {
	u.objectGetter = app.MustComponent[cache.ObjectGetter](a)
	return nil
}

func (u *syncStatusUpdater) Name() (name string) {
	return CName
}

func NewUpdater() Updater {
	return &syncStatusUpdater{}
}

func (u *syncStatusUpdater) UpdateDetails(objectId string, status domain.SyncStatus, syncError domain.SyncError) error {
	return cache.Do(u.objectGetter, objectId, func(sb basic.DetailsSettable) error {
		syncStatusDetails := []*model.Detail{
			{
				Key:   bundle.RelationKeySyncStatus.String(),
				Value: pbtypes.Int64(int64(status)),
			},
		}
		syncStatusDetails = append(syncStatusDetails, &model.Detail{
			Key:   bundle.RelationKeySyncError.String(),
			Value: pbtypes.Int64(int64(syncError)),
		})
		syncStatusDetails = append(syncStatusDetails, &model.Detail{
			Key:   bundle.RelationKeySyncDate.String(),
			Value: pbtypes.Int64(time.Now().Unix()),
		})
		return sb.SetDetails(nil, syncStatusDetails, false)
	})
}
