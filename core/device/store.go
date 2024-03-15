package device

import (
	"github.com/dgraph-io/badger/v4"
	"github.com/gogo/protobuf/proto"
	ds "github.com/ipfs/go-datastore"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

var deviceInfoKey = ds.NewKey("/devices/info")

type Store interface {
	SaveDeviceInfo(deviceInfo *model.DeviceInfo) error
	ListDevices() ([]*model.DeviceInfo, error)
}

type deviceStore struct {
	db *badger.DB
}

func (d *deviceStore) SaveDeviceInfo(deviceInfo *model.DeviceInfo) error {
	return badgerhelper.SetValue(d.db, deviceInfoKey.ChildString(deviceInfo.Id).Bytes(), deviceInfo)
}

func (d *deviceStore) ListDevices() ([]*model.DeviceInfo, error) {
	return badgerhelper.ViewTxnWithResult(d.db, func(txn *badger.Txn) ([]*model.DeviceInfo, error) {
		keys := localstore.GetKeys(txn, deviceInfoKey.String(), 0)
		devices, err := localstore.GetLeavesFromResults(keys)
		if err != nil {
			return nil, err
		}
		devicesInfo := make([]*model.DeviceInfo, 0, len(devices))
		for _, id := range devices {
			device := deviceInfoKey.ChildString(id)
			deviceInfo, err := badgerhelper.GetValueTxn(txn, device.Bytes(), unmarshalDeviceInfo)
			if badgerhelper.IsNotFound(err) {
				continue
			}
			devicesInfo = append(devicesInfo, deviceInfo)
		}
		return devicesInfo, nil
	})
}

func NewDeviceStore(db *badger.DB) Store {
	return &deviceStore{db: db}
}

func unmarshalDeviceInfo(raw []byte) (*model.DeviceInfo, error) {
	v := &model.DeviceInfo{}
	return v, proto.Unmarshal(raw, v)
}
