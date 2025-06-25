package device

import (
	"context"
	"errors"
	"fmt"

	anystore "github.com/anyproto/any-store"
	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
)

type Store interface {
	SaveDevice(device *model.DeviceInfo) error
	ListDevices() ([]*model.DeviceInfo, error)
	UpdateDeviceName(id, name string) error
}

type deviceStore struct {
	db keyvaluestore.Store[*model.DeviceInfo]
}

func NewStore(db anystore.DB) (Store, error) {
	kv, err := keyvaluestore.New(db, "devices", func(info *model.DeviceInfo) ([]byte, error) {
		return info.Marshal()
	}, func(raw []byte) (*model.DeviceInfo, error) {
		v := &model.DeviceInfo{}
		return v, proto.Unmarshal(raw, v)
	})
	if err != nil {
		return nil, fmt.Errorf("init store: %w", err)
	}
	return &deviceStore{db: kv}, nil
}

func (n *deviceStore) SaveDevice(device *model.DeviceInfo) error {
	tx, err := n.db.WriteTx(context.Background())
	if err != nil {
		return fmt.Errorf("create write tx: %w", err)
	}
	defer tx.Rollback()

	ok, err := n.db.Has(tx.Context(), device.Id)
	if err != nil {
		return fmt.Errorf("has: %w", err)
	}

	if !ok {
		err = n.db.Set(tx.Context(), device.Id, device)
		if err != nil {
			return fmt.Errorf("set: %w", err)
		}
	}
	return tx.Commit()
}

func (n *deviceStore) ListDevices() ([]*model.DeviceInfo, error) {
	return n.db.ListAllValues(context.Background())
}

func (n *deviceStore) UpdateDeviceName(id, name string) error {
	tx, err := n.db.WriteTx(context.Background())
	if err != nil {
		return fmt.Errorf("create write tx: %w", err)
	}
	defer tx.Rollback()

	info, err := n.db.Get(tx.Context(), id)
	if errors.Is(err, anystore.ErrDocNotFound) {
		info = &model.DeviceInfo{
			Id:   id,
			Name: name,
		}
	} else if err != nil {
		return fmt.Errorf("get device: %w", err)
	} else {
		info.Name = name
	}

	err = n.db.Set(tx.Context(), id, info)
	if err != nil {
		return fmt.Errorf("set: %w", err)
	}

	return tx.Commit()
}
