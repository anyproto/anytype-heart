package device

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
)

const deviceService = "deviceService"

var log = logging.Logger("notifications")

type Service interface {
	app.ComponentRunnable
	UpdateName(name string) error
	ListDevices() ([]*model.DeviceInfo, error)
}

func NewDevices() Service {
	return &Devices{devices: make(map[string]*model.DeviceInfo, 0)}
}

type Devices struct {
	devices        map[string]*model.DeviceInfo
	deviceObjectId string
	spaceService   space.Service
	store          Store
}

func (d *Devices) Init(a *app.App) (err error) {
	datastoreService := app.MustComponent[datastore.Datastore](a)
	db, err := datastoreService.LocalStorage()
	if err != nil {
		return fmt.Errorf("failed to initialize notification store %w", err)
	}
	d.store = NewDeviceStore(db)
	d.spaceService = app.MustComponent[space.Service](a)
	return nil
}

func (d *Devices) Name() (name string) {
	return deviceService
}

func (d *Devices) Run(ctx context.Context) (err error) {
	uk, err := domain.NewUniqueKey(sb.SmartBlockTypeNotificationObject, "")
	if err != nil {
		log.Errorf("failed to get devices object unique key: %v", err)
		return
	}
	techSpace, err := d.spaceService.GetTechSpace(ctx)
	if err != nil {
		return
	}
	deviceObject, err := techSpace.DeriveTreeObject(ctx, objectcache.TreeDerivationParams{
		Key: uk,
		InitFunc: func(id string) *smartblock.InitContext {
			return &smartblock.InitContext{
				Ctx:     ctx,
				SpaceID: techSpace.Id(),
				State:   state.NewDoc(id, nil).(*state.State),
			}
		},
	})
	if err != nil && !errors.Is(err, treestorage.ErrTreeExists) {
		log.Errorf("failed to derive device object: %v", err)
		return
	}
	if err == nil {
		d.deviceObjectId = deviceObject.Id()
	}
	if errors.Is(err, treestorage.ErrTreeExists) {
		id, err := techSpace.DeriveObjectID(ctx, uk)
		if err != nil {
			log.Errorf("failed to derive notification object id: %v", err)
			return
		}
		d.deviceObjectId = id
	}
	d.
}

func (d *Devices) Close(ctx context.Context) (err error) {
	return
}

func (d *Devices) UpdateName(name string) error {
	d.store.SaveDeviceInfo()
}

func (d *Devices) ListDevices() ([]*model.DeviceInfo, error) {
	return d.ListDevices()
}
