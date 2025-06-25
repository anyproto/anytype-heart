package device

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/net/peer"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/wallet"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/spacecore/peermanager"
)

const deviceService = "deviceService"

var log = logging.Logger(deviceService)

type Service interface {
	app.ComponentRunnable
	UpdateName(ctx context.Context, id, name string) error
	ListDevices(ctx context.Context) ([]*model.DeviceInfo, error)
	SaveDeviceInfo(info smartblock.ApplyInfo) error
}

func NewDevices() Service {
	return &devices{finishLoad: make(chan struct{})}
}

type devices struct {
	deviceObjectId string
	spaceService   space.Service
	wallet         wallet.Wallet
	cancel         context.CancelFunc
	store          Store

	finishLoad chan struct{}
}

func (d *devices) Init(a *app.App) (err error) {
	d.spaceService = app.MustComponent[space.Service](a)
	d.wallet = a.MustComponent(wallet.CName).(wallet.Wallet)

	provider := app.MustComponent[anystoreprovider.Provider](a)
	d.store, err = NewStore(provider.GetCommonDb())
	if err != nil {
		return fmt.Errorf("failed to initialize notification store %w", err)
	}
	return nil
}

func (d *devices) Name() (name string) {
	return deviceService
}

func (d *devices) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	go d.loadDevices(ctx)
	return nil
}

func (d *devices) loadDevices(ctx context.Context) {
	defer close(d.finishLoad)

	deviceObject, err := d.getDeviceObject(ctx)
	if err != nil {
		log.Errorf("failed to get device object: %v", err)
		return
	}

	hostname, err := os.Hostname()
	if err != nil {
		log.Errorf("failed to get hostname: %v", err)
		return
	}
	deviceObject.Lock()
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
	}
	deviceObject.Unlock()
}

func (d *devices) getDeviceObject(ctx context.Context) (object smartblock.SmartBlock, err error) {
	uk, err := domain.NewUniqueKey(sb.SmartBlockTypeDevicesObject, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get devices object unique key: %w", err)
	}
	techSpace, err := d.spaceService.GetTechSpace(ctx)
	if err != nil {
		return
	}
	ctx = context.WithValue(ctx, peermanager.ContextPeerFindDeadlineKey, time.Now().Add(30*time.Second))
	ctx = peer.CtxWithPeerId(ctx, peer.CtxResponsiblePeers)

	id, err := techSpace.DeriveObjectID(ctx, uk)
	if err != nil {
		return nil, fmt.Errorf("failed to derive device object id: %w", err)
	}
	d.deviceObjectId = id
	object, err = techSpace.GetObject(ctx, d.deviceObjectId)
	if err == nil {
		return
	}

	// failed to get device object, let's derive it
	object, err = techSpace.DeriveTreeObject(ctx, objectcache.TreeDerivationParams{
		Key: uk,
		InitFunc: func(id string) *smartblock.InitContext {
			return &smartblock.InitContext{
				Ctx:     ctx,
				SpaceID: techSpace.Id(),
				State:   state.NewDoc(id, nil).(*state.State),
			}
		},
	})
	if err == nil {
		d.deviceObjectId = object.Id()
		return
	}
	if !errors.Is(err, treestorage.ErrTreeExists) {
		return nil, fmt.Errorf("failed to derive device object: %w", err)
	}

	// derivation failed with ErrTreeExists, second attempt to get device object
	object, err = techSpace.GetObject(ctx, d.deviceObjectId)
	if err != nil {
		return nil, fmt.Errorf("failed to get device object: %w", err)
	}
	return
}

func (d *devices) Close(ctx context.Context) error {
	if d.cancel != nil {
		d.cancel()
	}
	return nil
}

func (d *devices) SaveDeviceInfo(info smartblock.ApplyInfo) error {
	if info.State == nil {
		return nil
	}
	deviceId := d.wallet.GetDevicePrivkey().GetPublic().PeerId()
	for _, deviceInfo := range info.State.ListDevices() {
		if deviceInfo.Id == deviceId {
			deviceInfo.IsConnected = true
		}
		err := d.store.SaveDevice(deviceInfo)
		if err != nil {
			return fmt.Errorf("failed to save device: %w", err)
		}
	}
	return nil
}

func (d *devices) UpdateName(ctx context.Context, id, name string) error {
	err := d.store.UpdateDeviceName(id, name)
	if err != nil {
		return fmt.Errorf("failed to update device name: %w", err)
	}
	spc, err := d.spaceService.Get(ctx, d.spaceService.TechSpaceId())
	if err != nil {
		return fmt.Errorf("failed to get space: %w", err)
	}
	return spc.Do(d.deviceObjectId, func(sb smartblock.SmartBlock) error {
		st := sb.NewState()
		st.SetDeviceName(id, name)
		return sb.Apply(st)
	})
}

func (d *devices) ListDevices(ctx context.Context) ([]*model.DeviceInfo, error) {
	return d.store.ListDevices()
}
