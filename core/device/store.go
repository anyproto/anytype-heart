package device

import (
	"github.com/dgraph-io/badger/v4"
	ds "github.com/ipfs/go-datastore"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

const devicesInfo = "devices"

var deviceInfo = ds.NewKey("/" + devicesInfo + "/info")

type Store interface {
	SaveDevice(device *model.DeviceInfo) error
	ListDevices() ([]*model.DeviceInfo, error)
	UpdateDeviceName(id, name string) error
}

type deviceStore struct {
	db *badger.DB
}

func NewStore(db *badger.DB) Store {
	return &deviceStore{db: db}
}

func (n *deviceStore) SaveDevice(device *model.DeviceInfo) error {
	return n.db.Update(func(txn *badger.Txn) error {
		_, err := txn.Get(deviceInfo.ChildString(device.Id).Bytes())
		if err != nil && !badgerhelper.IsNotFound(err) {
			return err
		}
		if badgerhelper.IsNotFound(err) {
			infoRaw, err := device.MarshalVT()
			if err != nil {
				return err
			}
			return txn.Set(deviceInfo.ChildString(device.Id).Bytes(), infoRaw)
		}
		return nil
	})
}

func (n *deviceStore) ListDevices() ([]*model.DeviceInfo, error) {
	return badgerhelper.ViewTxnWithResult(n.db, func(txn *badger.Txn) ([]*model.DeviceInfo, error) {
		keys := localstore.GetKeys(txn, deviceInfo.String(), 0)
		devicesIds, err := localstore.GetLeavesFromResults(keys)
		if err != nil {
			return nil, err
		}
		deviceInfos := make([]*model.DeviceInfo, 0, len(devicesIds))
		for _, id := range devicesIds {
			info := deviceInfo.ChildString(id)
			device, err := badgerhelper.GetValueTxn(txn, info.Bytes(), unmarshalDeviceInfo)
			if badgerhelper.IsNotFound(err) {
				continue
			}
			deviceInfos = append(deviceInfos, device)
		}
		return deviceInfos, nil
	})
}

func (n *deviceStore) UpdateDeviceName(id, name string) error {
	return n.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(deviceInfo.ChildString(id).Bytes())
		if err != nil && !badgerhelper.IsNotFound(err) {
			return err
		}
		var info *model.DeviceInfo
		if badgerhelper.IsNotFound(err) {
			info = &model.DeviceInfo{
				Id:   id,
				Name: name,
			}
		} else {
			if err = item.Value(func(val []byte) error {
				info, err = unmarshalDeviceInfo(val)
				return err
			}); err != nil {
				return err
			}
			info.Name = name
		}
		infoRaw, err := info.MarshalVT()
		if err != nil {
			return err
		}
		return txn.Set(deviceInfo.ChildString(id).Bytes(), infoRaw)
	})
}

func unmarshalDeviceInfo(raw []byte) (*model.DeviceInfo, error) {
	v := &model.DeviceInfo{}
	return v, v.UnmarshalVT(raw)
}
