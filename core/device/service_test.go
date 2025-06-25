package device

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache/mock_objectcache"
	wallet2 "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

func TestService_SaveDeviceInfo(t *testing.T) {
	deviceObjectId := "deviceObjectId"
	t.Run("save device in object", func(t *testing.T) {
		// given
		testDevice := &model.DeviceInfo{
			Id:   "id",
			Name: "test",
		}

		devicesService := newFixture(t, deviceObjectId)

		deviceObject := &editor.Page{SmartBlock: smarttest.New(deviceObjectId)}
		state := deviceObject.NewState()
		state.AddDevice(testDevice)
		err := deviceObject.Apply(state)
		assert.Nil(t, err)

		// when
		err = devicesService.SaveDeviceInfo(smartblock.ApplyInfo{State: deviceObject.NewState()})

		// then
		assert.Nil(t, err)
		assert.NotNil(t, deviceObject.NewState().GetDevice("id"))
		deviceInfos, err := devicesService.store.ListDevices()
		assert.Nil(t, err)
		assert.Contains(t, deviceInfos, testDevice)
	})

	t.Run("save device in object, device exist", func(t *testing.T) {
		// given
		testDevice := &model.DeviceInfo{
			Id:   "id",
			Name: "test",
		}

		devicesService := newFixture(t, deviceObjectId)
		deviceObject := &editor.Page{SmartBlock: smarttest.New(deviceObjectId)}

		testDevice1 := &model.DeviceInfo{
			Id:   "id",
			Name: "test1",
		}
		state := deviceObject.NewState()
		state.AddDevice(testDevice)
		err := deviceObject.Apply(state)
		assert.Nil(t, err)

		// when
		err = devicesService.SaveDeviceInfo(smartblock.ApplyInfo{State: deviceObject.NewState()})
		err = devicesService.SaveDeviceInfo(smartblock.ApplyInfo{State: deviceObject.NewState()})

		// then
		assert.Nil(t, err)
		assert.NotNil(t, deviceObject.NewState().GetDevice("id"))
		assert.Equal(t, "test", deviceObject.NewState().GetDevice("id").Name)
		deviceInfos, err := devicesService.store.ListDevices()
		assert.Nil(t, err)
		assert.Contains(t, deviceInfos, testDevice)
		assert.NotContains(t, deviceInfos, testDevice1)
	})
}

func TestService_UpdateName(t *testing.T) {
	deviceObjectId := "deviceObjectId"
	techSpaceId := "techSpaceId"
	t.Run("update name, device not exist", func(t *testing.T) {
		// given

		devicesService := newFixture(t, deviceObjectId)
		virtualSpace := clientspace.NewVirtualSpace(techSpaceId, clientspace.VirtualSpaceDeps{})
		devicesService.mockSpaceService.EXPECT().Get(context.Background(), techSpaceId).Return(virtualSpace, nil)
		devicesService.mockSpaceService.EXPECT().TechSpaceId().Return(techSpaceId)

		deviceObject := &editor.Page{SmartBlock: smarttest.New(deviceObjectId)}
		mockCache := mock_objectcache.NewMockCache(t)
		mockCache.EXPECT().GetObject(context.Background(), deviceObjectId).Return(deviceObject, nil)

		virtualSpace.Cache = mockCache

		// when
		err := devicesService.UpdateName(context.Background(), "id", "new name")

		// then
		assert.Nil(t, err)
		assert.NotNil(t, deviceObject.NewState().GetDevice("id"))
		assert.Equal(t, "new name", deviceObject.NewState().GetDevice("id").Name)
		deviceInfos, err := devicesService.store.ListDevices()
		assert.Nil(t, err)
		assert.Contains(t, deviceInfos, &model.DeviceInfo{
			Id:   "id",
			Name: "new name",
		})
	})

	t.Run("update name, device exists", func(t *testing.T) {
		// given
		testDevice := &model.DeviceInfo{
			Id:   "id",
			Name: "test",
		}

		devicesService := newFixture(t, deviceObjectId)
		virtualSpace := clientspace.NewVirtualSpace(techSpaceId, clientspace.VirtualSpaceDeps{})
		devicesService.mockSpaceService.EXPECT().Get(context.Background(), techSpaceId).Return(virtualSpace, nil)
		devicesService.mockSpaceService.EXPECT().TechSpaceId().Return(techSpaceId)

		deviceObject := &editor.Page{SmartBlock: smarttest.New(deviceObjectId)}
		mockCache := mock_objectcache.NewMockCache(t)
		mockCache.EXPECT().GetObject(context.Background(), deviceObjectId).Return(deviceObject, nil)

		state := deviceObject.NewState()
		state.AddDevice(testDevice)
		err := deviceObject.Apply(state)
		assert.Nil(t, err)

		virtualSpace.Cache = mockCache
		err = devicesService.SaveDeviceInfo(smartblock.ApplyInfo{State: deviceObject.NewState()})
		assert.Nil(t, err)

		// when
		err = devicesService.UpdateName(context.Background(), "id", "new name")

		// then
		assert.Nil(t, err)
		assert.NotNil(t, deviceObject.NewState().GetDevice("id"))
		assert.Equal(t, "new name", deviceObject.NewState().GetDevice("id").Name)
		deviceInfos, err := devicesService.store.ListDevices()
		assert.Nil(t, err)
		assert.NotContains(t, deviceInfos, testDevice)
		testDevice.Name = "new name"
		assert.Contains(t, deviceInfos, testDevice)
	})
}

func TestService_ListDevices(t *testing.T) {
	deviceObjectId := "deviceObjectId"
	t.Run("list devices, no devices", func(t *testing.T) {
		// given

		devicesService := newFixture(t, deviceObjectId)

		// when
		close(devicesService.finishLoad)
		devicesList, err := devicesService.ListDevices(context.Background())

		// then
		assert.Nil(t, err)
		assert.Len(t, devicesList, 0)
	})

	t.Run("list devices", func(t *testing.T) {
		// given
		testDevice := &model.DeviceInfo{
			Id:   "id",
			Name: "test",
		}

		testDevice1 := &model.DeviceInfo{
			Id:   "id1",
			Name: "test1",
		}

		devicesService := newFixture(t, deviceObjectId)
		deviceObject := &editor.Page{SmartBlock: smarttest.New(deviceObjectId)}
		state := deviceObject.NewState()
		state.AddDevice(testDevice)
		state.AddDevice(testDevice1)
		err := deviceObject.Apply(state)
		assert.Nil(t, err)

		err = devicesService.SaveDeviceInfo(smartblock.ApplyInfo{State: deviceObject.NewState()})
		assert.Nil(t, err)

		// when
		close(devicesService.finishLoad)
		devicesList, err := devicesService.ListDevices(context.Background())

		// then
		assert.NoError(t, err)
		assert.ElementsMatch(t, []*model.DeviceInfo{testDevice, testDevice1}, devicesList)
	})
}

func TestService_loadDevices(t *testing.T) {
	deviceObjectId := "deviceObjectId"
	techSpaceId := "techSpaceId"
	ctx := context.Background()
	t.Run("loadDevices, device object not exist", func(t *testing.T) {
		// given
		devicesService := newFixture(t, deviceObjectId)
		virtualSpace := clientspace.NewVirtualSpace(techSpaceId, clientspace.VirtualSpaceDeps{})
		devicesService.mockSpaceService.EXPECT().GetTechSpace(ctx).Return(virtualSpace, nil)

		deviceObject := &editor.Page{SmartBlock: smarttest.New(deviceObjectId)}
		mockCache := mock_objectcache.NewMockCache(t)
		mockCache.EXPECT().GetObject(mock.Anything, deviceObjectId).Return(nil, fmt.Errorf("error"))
		mockCache.EXPECT().DeriveTreeObject(mock.Anything, mock.Anything).Return(deviceObject, nil)
		mockCache.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).Return(deviceObjectId, nil)
		virtualSpace.Cache = mockCache

		// when
		devicesService.loadDevices(ctx)

		// then
		assert.NotNil(t, deviceObject.NewState().GetDevice(devicesService.wallet.GetDevicePrivkey().GetPublic().PeerId()))
	})

	t.Run("loadDevices, device object exist", func(t *testing.T) {
		// given
		devicesService := newFixture(t, deviceObjectId)
		virtualSpace := clientspace.NewVirtualSpace(techSpaceId, clientspace.VirtualSpaceDeps{})
		devicesService.mockSpaceService.EXPECT().GetTechSpace(ctx).Return(virtualSpace, nil)

		deviceObject := &editor.Page{SmartBlock: smarttest.New(deviceObjectId)}
		mockCache := mock_objectcache.NewMockCache(t)
		mockCache.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).Return(deviceObjectId, nil)
		mockCache.EXPECT().GetObject(mock.Anything, deviceObjectId).Return(deviceObject, nil)
		virtualSpace.Cache = mockCache

		// when
		devicesService.loadDevices(ctx)

		// then
		assert.NotNil(t, deviceObject.NewState().GetDevice(devicesService.wallet.GetDevicePrivkey().GetPublic().PeerId()))
	})

	t.Run("loadDevices, device object exists, but loading is long", func(t *testing.T) {
		// given
		devicesService := newFixture(t, deviceObjectId)
		virtualSpace := clientspace.NewVirtualSpace(techSpaceId, clientspace.VirtualSpaceDeps{})
		devicesService.mockSpaceService.EXPECT().GetTechSpace(ctx).Return(virtualSpace, nil)

		deviceObject := &editor.Page{SmartBlock: smarttest.New(deviceObjectId)}
		var visited bool
		mockCache := mock_objectcache.NewMockCache(t)
		mockCache.EXPECT().GetObject(mock.Anything, deviceObjectId).RunAndReturn(func(context.Context, string) (smartblock.SmartBlock, error) {
			if visited {
				return deviceObject, nil
			}
			visited = true
			return nil, fmt.Errorf("failed to load object")
		}).Maybe()
		mockCache.EXPECT().DeriveTreeObject(mock.Anything, mock.Anything).Return(nil, treestorage.ErrTreeExists)
		mockCache.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).Return(deviceObjectId, nil)
		virtualSpace.Cache = mockCache

		// when
		devicesService.loadDevices(ctx)

		// then
		assert.NotNil(t, deviceObject.NewState().GetDevice(devicesService.wallet.GetDevicePrivkey().GetPublic().PeerId()))
	})

	t.Run("loadDevices, save devices from derived objects", func(t *testing.T) {
		// given
		devicesService := newFixture(t, deviceObjectId)
		virtualSpace := clientspace.NewVirtualSpace(techSpaceId, clientspace.VirtualSpaceDeps{})
		devicesService.mockSpaceService.EXPECT().GetTechSpace(ctx).Return(virtualSpace, nil)

		deviceObject := &editor.Page{SmartBlock: smarttest.New(deviceObjectId)}
		mockCache := mock_objectcache.NewMockCache(t)
		mockCache.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).Return(deviceObjectId, nil)
		mockCache.EXPECT().GetObject(mock.Anything, deviceObjectId).Return(deviceObject, nil)
		virtualSpace.Cache = mockCache

		state := deviceObject.NewState()
		state.AddDevice(&model.DeviceInfo{
			Id:          "test",
			Name:        "test",
			IsConnected: true,
		})
		state.AddDevice(&model.DeviceInfo{
			Id:   "test1",
			Name: "test1",
		})
		err := deviceObject.Apply(state)
		assert.Nil(t, err)
		deviceObject.AddHook(devicesService.SaveDeviceInfo, smartblock.HookAfterApply)

		// when
		devicesService.loadDevices(ctx)

		// then
		assert.NotNil(t, deviceObject.NewState().GetDevice(devicesService.wallet.GetDevicePrivkey().GetPublic().PeerId()))
		listDevices, err := devicesService.store.ListDevices()
		assert.Nil(t, err)
		assert.Len(t, listDevices, 3)
	})
}

func TestService_Init(t *testing.T) {
	t.Run("successfully started and closed service", func(t *testing.T) {
		// given
		deviceObjectId := "deviceObjectId"
		ctx := context.Background()
		techSpaceId := "techSpaceId"
		devicesService := newFixture(t, deviceObjectId)
		virtualSpace := clientspace.NewVirtualSpace(techSpaceId, clientspace.VirtualSpaceDeps{})
		devicesService.mockSpaceService.EXPECT().GetTechSpace(mock.Anything).Return(virtualSpace, nil).Maybe()

		deviceObject := &editor.Page{SmartBlock: smarttest.New(deviceObjectId)}
		mockCache := mock_objectcache.NewMockCache(t)
		mockCache.EXPECT().GetObject(mock.Anything, deviceObjectId).Return(deviceObject, nil).Maybe()
		mockCache.EXPECT().DeriveTreeObject(mock.Anything, mock.Anything).Return(nil, treestorage.ErrTreeExists).Maybe()
		mockCache.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).Return(deviceObjectId, nil).Maybe()
		virtualSpace.Cache = mockCache

		// when
		assert.Nil(t, devicesService.Run(ctx))

		// then
		assert.Nil(t, devicesService.Close(ctx))
	})
}

type deviceFixture struct {
	*devices

	mockSpaceService *mock_space.MockService
	mockCache        *mock_objectcache.MockCache
	wallet           wallet2.Wallet
}

func newFixture(t *testing.T, deviceObjectId string) *deviceFixture {
	mockSpaceService := mock_space.NewMockService(t)
	mockCache := mock_objectcache.NewMockCache(t)
	wallet := wallet2.NewWithRepoDirAndRandomKeys(os.TempDir())

	dbProvider, err := anystoreprovider.NewInPath(t.TempDir())
	require.NoError(t, err)

	df := &deviceFixture{
		mockSpaceService: mockSpaceService,
		mockCache:        mockCache,
		wallet:           wallet,
		devices:          &devices{deviceObjectId: deviceObjectId, finishLoad: make(chan struct{})},
	}

	a := &app.App{}

	a.Register(testutil.PrepareMock(context.Background(), a, mockSpaceService)).
		Register(wallet).
		Register(dbProvider)

	err = wallet.Init(a)
	assert.NoError(t, err)
	err = df.Init(a)
	assert.NoError(t, err)
	return df
}
