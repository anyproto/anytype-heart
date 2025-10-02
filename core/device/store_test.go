package device

import (
	"context"
	"path/filepath"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func newTestAnystore(t *testing.T) anystore.DB {
	db, err := anystore.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return db
}

func TestDeviceStore_SaveDevice(t *testing.T) {
	t.Run("device exist: not save it again", func(t *testing.T) {
		// given
		store, err := NewStore(newTestAnystore(t))
		require.NoError(t, err)

		testInfo1 := &model.DeviceInfo{Id: "test", Name: "test"}
		testInfo2 := &model.DeviceInfo{Id: "test", Name: "test"}
		err = store.SaveDevice(testInfo1)
		assert.Nil(t, err)

		// when
		err = store.SaveDevice(testInfo2)

		// then
		assert.Nil(t, err)
		listDevices, err := store.ListDevices()
		assert.Nil(t, err)
		assert.Len(t, listDevices, 1)
	})

	t.Run("device not exist: save it", func(t *testing.T) {
		// given
		store, err := NewStore(newTestAnystore(t))
		require.NoError(t, err)
		testInfo1 := &model.DeviceInfo{Id: "test", Name: "test"}

		// when
		err = store.SaveDevice(testInfo1)

		// then
		assert.Nil(t, err)
		listDevices, err := store.ListDevices()
		assert.Nil(t, err)
		assert.Len(t, listDevices, 1)
	})
}

func TestDeviceStore_ListDevices(t *testing.T) {
	t.Run("list devices: no devices", func(t *testing.T) {
		// given
		store, err := NewStore(newTestAnystore(t))
		require.NoError(t, err)

		// when
		devices, err := store.ListDevices()

		// then
		assert.Nil(t, err)
		assert.Len(t, devices, 0)
	})
	t.Run("list devices: 2 devices", func(t *testing.T) {
		// given
		store, err := NewStore(newTestAnystore(t))
		require.NoError(t, err)

		testInfo1 := &model.DeviceInfo{Id: "test", Name: "test"}
		testInfo2 := &model.DeviceInfo{Id: "test1", Name: "test"}
		err = store.SaveDevice(testInfo1)
		assert.Nil(t, err)
		err = store.SaveDevice(testInfo2)
		assert.Nil(t, err)

		// when
		devices, err := store.ListDevices()

		// then
		assert.Nil(t, err)
		assert.Len(t, devices, 2)
	})
}

func TestDeviceStore_UpdateDeviceName(t *testing.T) {
	t.Run("update device: device not exist - save it", func(t *testing.T) {
		// given
		store, err := NewStore(newTestAnystore(t))
		require.NoError(t, err)

		// when
		err = store.UpdateDeviceName("id", "test")

		// then
		assert.Nil(t, err)
		listDevices, err := store.ListDevices()
		assert.Nil(t, err)
		assert.Len(t, listDevices, 1)
		assert.Contains(t, listDevices, &model.DeviceInfo{
			Id:   "id",
			Name: "test",
		})
	})
	t.Run("update device: device exists - update it", func(t *testing.T) {
		// given
		store, err := NewStore(newTestAnystore(t))
		require.NoError(t, err)
		testInfo1 := &model.DeviceInfo{Id: "id", Name: "test"}
		err = store.SaveDevice(testInfo1)
		assert.Nil(t, err)
		// when
		err = store.UpdateDeviceName("id", "test1")

		// then
		assert.Nil(t, err)
		listDevices, err := store.ListDevices()
		assert.Nil(t, err)
		assert.Len(t, listDevices, 1)
		assert.Contains(t, listDevices, &model.DeviceInfo{
			Id:   "id",
			Name: "test1",
		})
	})
}
