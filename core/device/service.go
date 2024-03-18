package device

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/wallet"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
)

const deviceService = "deviceService"

var log = logging.Logger("notifications")

type Service interface {
	app.ComponentRunnable
	UpdateName(ctx context.Context, id, name string) error
	ListDevices(ctx context.Context) ([]*model.DeviceInfo, error)
}

func NewDevices() Service {
	return &Devices{}
}

type Devices struct {
	deviceObjectId string
	spaceService   space.Service
	wallet         wallet.Wallet
	cancel         context.CancelFunc
}

func (d *Devices) Init(a *app.App) (err error) {
	d.spaceService = app.MustComponent[space.Service](a)
	d.wallet = a.MustComponent(wallet.CName).(wallet.Wallet)
	return nil
}

func (d *Devices) Name() (name string) {
	return deviceService
}

func (d *Devices) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	go d.loadDevices(ctx)
	return nil
}

func (d *Devices) loadDevices(ctx context.Context) {
	uk, err := domain.NewUniqueKey(sb.SmartBlockTypeDeviceObject, "")
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
			log.Errorf("failed to derive device object id: %v", err)
			return
		}
		d.deviceObjectId = id
	}
	if deviceObject == nil {
		deviceObject, err = techSpace.GetObject(ctx, d.deviceObjectId)
		if err != nil {
			log.Errorf("failed to get device object id: %v", err)
			return
		}
	}
	hostname, err := os.Hostname()
	if err != nil {
		log.Errorf("failed to get hostname: %v", err)
		return
	}
	st := deviceObject.NewState()
	deviceId := d.wallet.GetDevicePrivkey().GetPublic().PeerId()
	st.AddDevice(&model.DeviceInfo{
		Id:      deviceId,
		Name:    hostname,
		AddDate: time.Now().Unix(),
	})
	err = deviceObject.Apply(st)
	if err != nil {
		log.Errorf("failed to apply device state: %v", err)
		return
	}
}

func (d *Devices) Close(ctx context.Context) error {
	if d.cancel != nil {
		d.cancel()
	}
	return nil
}

func (d *Devices) SaveDeviceInfo(ctx context.Context, device *model.DeviceInfo) error {
	spc, err := d.spaceService.Get(ctx, d.spaceService.TechSpaceId())
	if err != nil {
		return nil
	}
	return spc.Do(d.deviceObjectId, func(sb smartblock.SmartBlock) error {
		st := sb.NewState()
		st.AddDevice(device)
		return sb.Apply(st)
	})
}

func (d *Devices) UpdateName(ctx context.Context, id, name string) error {
	spc, err := d.spaceService.Get(ctx, d.spaceService.TechSpaceId())
	if err != nil {
		return err
	}
	return spc.Do(d.deviceObjectId, func(sb smartblock.SmartBlock) error {
		st := sb.NewState()
		st.SetDeviceName(id, name)
		return sb.Apply(st)
	})
}

func (d *Devices) ListDevices(ctx context.Context) ([]*model.DeviceInfo, error) {
	spc, err := d.spaceService.Get(ctx, d.spaceService.TechSpaceId())
	if err != nil {
		return nil, err
	}
	var devices []*model.DeviceInfo
	err = spc.Do(d.deviceObjectId, func(sb smartblock.SmartBlock) error {
		st := sb.NewState()
		for _, info := range st.ListDevices() {
			devices = append(devices, info)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return devices, nil
}
