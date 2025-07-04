// Code generated by mockery. DO NOT EDIT.

package mock_filestorage

import (
	app "github.com/anyproto/any-sync/app"
	blocks "github.com/ipfs/go-block-format"

	cid "github.com/ipfs/go-cid"

	context "context"

	domain "github.com/anyproto/anytype-heart/core/domain"

	filestorage "github.com/anyproto/anytype-heart/core/files/filestorage"

	mock "github.com/stretchr/testify/mock"
)

// MockFileStorage is an autogenerated mock type for the FileStorage type
type MockFileStorage struct {
	mock.Mock
}

type MockFileStorage_Expecter struct {
	mock *mock.Mock
}

func (_m *MockFileStorage) EXPECT() *MockFileStorage_Expecter {
	return &MockFileStorage_Expecter{mock: &_m.Mock}
}

// Add provides a mock function with given fields: ctx, b
func (_m *MockFileStorage) Add(ctx context.Context, b []blocks.Block) error {
	ret := _m.Called(ctx, b)

	if len(ret) == 0 {
		panic("no return value specified for Add")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, []blocks.Block) error); ok {
		r0 = rf(ctx, b)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockFileStorage_Add_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Add'
type MockFileStorage_Add_Call struct {
	*mock.Call
}

// Add is a helper method to define mock.On call
//   - ctx context.Context
//   - b []blocks.Block
func (_e *MockFileStorage_Expecter) Add(ctx interface{}, b interface{}) *MockFileStorage_Add_Call {
	return &MockFileStorage_Add_Call{Call: _e.mock.On("Add", ctx, b)}
}

func (_c *MockFileStorage_Add_Call) Run(run func(ctx context.Context, b []blocks.Block)) *MockFileStorage_Add_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].([]blocks.Block))
	})
	return _c
}

func (_c *MockFileStorage_Add_Call) Return(_a0 error) *MockFileStorage_Add_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockFileStorage_Add_Call) RunAndReturn(run func(context.Context, []blocks.Block) error) *MockFileStorage_Add_Call {
	_c.Call.Return(run)
	return _c
}

// Close provides a mock function with given fields: ctx
func (_m *MockFileStorage) Close(ctx context.Context) error {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for Close")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context) error); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockFileStorage_Close_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Close'
type MockFileStorage_Close_Call struct {
	*mock.Call
}

// Close is a helper method to define mock.On call
//   - ctx context.Context
func (_e *MockFileStorage_Expecter) Close(ctx interface{}) *MockFileStorage_Close_Call {
	return &MockFileStorage_Close_Call{Call: _e.mock.On("Close", ctx)}
}

func (_c *MockFileStorage_Close_Call) Run(run func(ctx context.Context)) *MockFileStorage_Close_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context))
	})
	return _c
}

func (_c *MockFileStorage_Close_Call) Return(err error) *MockFileStorage_Close_Call {
	_c.Call.Return(err)
	return _c
}

func (_c *MockFileStorage_Close_Call) RunAndReturn(run func(context.Context) error) *MockFileStorage_Close_Call {
	_c.Call.Return(run)
	return _c
}

// Delete provides a mock function with given fields: ctx, c
func (_m *MockFileStorage) Delete(ctx context.Context, c cid.Cid) error {
	ret := _m.Called(ctx, c)

	if len(ret) == 0 {
		panic("no return value specified for Delete")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, cid.Cid) error); ok {
		r0 = rf(ctx, c)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockFileStorage_Delete_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Delete'
type MockFileStorage_Delete_Call struct {
	*mock.Call
}

// Delete is a helper method to define mock.On call
//   - ctx context.Context
//   - c cid.Cid
func (_e *MockFileStorage_Expecter) Delete(ctx interface{}, c interface{}) *MockFileStorage_Delete_Call {
	return &MockFileStorage_Delete_Call{Call: _e.mock.On("Delete", ctx, c)}
}

func (_c *MockFileStorage_Delete_Call) Run(run func(ctx context.Context, c cid.Cid)) *MockFileStorage_Delete_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(cid.Cid))
	})
	return _c
}

func (_c *MockFileStorage_Delete_Call) Return(_a0 error) *MockFileStorage_Delete_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockFileStorage_Delete_Call) RunAndReturn(run func(context.Context, cid.Cid) error) *MockFileStorage_Delete_Call {
	_c.Call.Return(run)
	return _c
}

// ExistsCids provides a mock function with given fields: ctx, ks
func (_m *MockFileStorage) ExistsCids(ctx context.Context, ks []cid.Cid) ([]cid.Cid, error) {
	ret := _m.Called(ctx, ks)

	if len(ret) == 0 {
		panic("no return value specified for ExistsCids")
	}

	var r0 []cid.Cid
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, []cid.Cid) ([]cid.Cid, error)); ok {
		return rf(ctx, ks)
	}
	if rf, ok := ret.Get(0).(func(context.Context, []cid.Cid) []cid.Cid); ok {
		r0 = rf(ctx, ks)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]cid.Cid)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, []cid.Cid) error); ok {
		r1 = rf(ctx, ks)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockFileStorage_ExistsCids_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ExistsCids'
type MockFileStorage_ExistsCids_Call struct {
	*mock.Call
}

// ExistsCids is a helper method to define mock.On call
//   - ctx context.Context
//   - ks []cid.Cid
func (_e *MockFileStorage_Expecter) ExistsCids(ctx interface{}, ks interface{}) *MockFileStorage_ExistsCids_Call {
	return &MockFileStorage_ExistsCids_Call{Call: _e.mock.On("ExistsCids", ctx, ks)}
}

func (_c *MockFileStorage_ExistsCids_Call) Run(run func(ctx context.Context, ks []cid.Cid)) *MockFileStorage_ExistsCids_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].([]cid.Cid))
	})
	return _c
}

func (_c *MockFileStorage_ExistsCids_Call) Return(exists []cid.Cid, err error) *MockFileStorage_ExistsCids_Call {
	_c.Call.Return(exists, err)
	return _c
}

func (_c *MockFileStorage_ExistsCids_Call) RunAndReturn(run func(context.Context, []cid.Cid) ([]cid.Cid, error)) *MockFileStorage_ExistsCids_Call {
	_c.Call.Return(run)
	return _c
}

// Get provides a mock function with given fields: ctx, k
func (_m *MockFileStorage) Get(ctx context.Context, k cid.Cid) (blocks.Block, error) {
	ret := _m.Called(ctx, k)

	if len(ret) == 0 {
		panic("no return value specified for Get")
	}

	var r0 blocks.Block
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, cid.Cid) (blocks.Block, error)); ok {
		return rf(ctx, k)
	}
	if rf, ok := ret.Get(0).(func(context.Context, cid.Cid) blocks.Block); ok {
		r0 = rf(ctx, k)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(blocks.Block)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, cid.Cid) error); ok {
		r1 = rf(ctx, k)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockFileStorage_Get_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Get'
type MockFileStorage_Get_Call struct {
	*mock.Call
}

// Get is a helper method to define mock.On call
//   - ctx context.Context
//   - k cid.Cid
func (_e *MockFileStorage_Expecter) Get(ctx interface{}, k interface{}) *MockFileStorage_Get_Call {
	return &MockFileStorage_Get_Call{Call: _e.mock.On("Get", ctx, k)}
}

func (_c *MockFileStorage_Get_Call) Run(run func(ctx context.Context, k cid.Cid)) *MockFileStorage_Get_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(cid.Cid))
	})
	return _c
}

func (_c *MockFileStorage_Get_Call) Return(_a0 blocks.Block, _a1 error) *MockFileStorage_Get_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockFileStorage_Get_Call) RunAndReturn(run func(context.Context, cid.Cid) (blocks.Block, error)) *MockFileStorage_Get_Call {
	_c.Call.Return(run)
	return _c
}

// GetMany provides a mock function with given fields: ctx, ks
func (_m *MockFileStorage) GetMany(ctx context.Context, ks []cid.Cid) <-chan blocks.Block {
	ret := _m.Called(ctx, ks)

	if len(ret) == 0 {
		panic("no return value specified for GetMany")
	}

	var r0 <-chan blocks.Block
	if rf, ok := ret.Get(0).(func(context.Context, []cid.Cid) <-chan blocks.Block); ok {
		r0 = rf(ctx, ks)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(<-chan blocks.Block)
		}
	}

	return r0
}

// MockFileStorage_GetMany_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetMany'
type MockFileStorage_GetMany_Call struct {
	*mock.Call
}

// GetMany is a helper method to define mock.On call
//   - ctx context.Context
//   - ks []cid.Cid
func (_e *MockFileStorage_Expecter) GetMany(ctx interface{}, ks interface{}) *MockFileStorage_GetMany_Call {
	return &MockFileStorage_GetMany_Call{Call: _e.mock.On("GetMany", ctx, ks)}
}

func (_c *MockFileStorage_GetMany_Call) Run(run func(ctx context.Context, ks []cid.Cid)) *MockFileStorage_GetMany_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].([]cid.Cid))
	})
	return _c
}

func (_c *MockFileStorage_GetMany_Call) Return(_a0 <-chan blocks.Block) *MockFileStorage_GetMany_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockFileStorage_GetMany_Call) RunAndReturn(run func(context.Context, []cid.Cid) <-chan blocks.Block) *MockFileStorage_GetMany_Call {
	_c.Call.Return(run)
	return _c
}

// Init provides a mock function with given fields: a
func (_m *MockFileStorage) Init(a *app.App) error {
	ret := _m.Called(a)

	if len(ret) == 0 {
		panic("no return value specified for Init")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(*app.App) error); ok {
		r0 = rf(a)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockFileStorage_Init_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Init'
type MockFileStorage_Init_Call struct {
	*mock.Call
}

// Init is a helper method to define mock.On call
//   - a *app.App
func (_e *MockFileStorage_Expecter) Init(a interface{}) *MockFileStorage_Init_Call {
	return &MockFileStorage_Init_Call{Call: _e.mock.On("Init", a)}
}

func (_c *MockFileStorage_Init_Call) Run(run func(a *app.App)) *MockFileStorage_Init_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*app.App))
	})
	return _c
}

func (_c *MockFileStorage_Init_Call) Return(err error) *MockFileStorage_Init_Call {
	_c.Call.Return(err)
	return _c
}

func (_c *MockFileStorage_Init_Call) RunAndReturn(run func(*app.App) error) *MockFileStorage_Init_Call {
	_c.Call.Return(run)
	return _c
}

// IterateFiles provides a mock function with given fields: ctx, iterFunc
func (_m *MockFileStorage) IterateFiles(ctx context.Context, iterFunc func(domain.FullFileId)) error {
	ret := _m.Called(ctx, iterFunc)

	if len(ret) == 0 {
		panic("no return value specified for IterateFiles")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, func(domain.FullFileId)) error); ok {
		r0 = rf(ctx, iterFunc)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockFileStorage_IterateFiles_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'IterateFiles'
type MockFileStorage_IterateFiles_Call struct {
	*mock.Call
}

// IterateFiles is a helper method to define mock.On call
//   - ctx context.Context
//   - iterFunc func(domain.FullFileId)
func (_e *MockFileStorage_Expecter) IterateFiles(ctx interface{}, iterFunc interface{}) *MockFileStorage_IterateFiles_Call {
	return &MockFileStorage_IterateFiles_Call{Call: _e.mock.On("IterateFiles", ctx, iterFunc)}
}

func (_c *MockFileStorage_IterateFiles_Call) Run(run func(ctx context.Context, iterFunc func(domain.FullFileId))) *MockFileStorage_IterateFiles_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(func(domain.FullFileId)))
	})
	return _c
}

func (_c *MockFileStorage_IterateFiles_Call) Return(_a0 error) *MockFileStorage_IterateFiles_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockFileStorage_IterateFiles_Call) RunAndReturn(run func(context.Context, func(domain.FullFileId)) error) *MockFileStorage_IterateFiles_Call {
	_c.Call.Return(run)
	return _c
}

// LocalDiskUsage provides a mock function with given fields: ctx
func (_m *MockFileStorage) LocalDiskUsage(ctx context.Context) (uint64, error) {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for LocalDiskUsage")
	}

	var r0 uint64
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context) (uint64, error)); ok {
		return rf(ctx)
	}
	if rf, ok := ret.Get(0).(func(context.Context) uint64); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(uint64)
	}

	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockFileStorage_LocalDiskUsage_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'LocalDiskUsage'
type MockFileStorage_LocalDiskUsage_Call struct {
	*mock.Call
}

// LocalDiskUsage is a helper method to define mock.On call
//   - ctx context.Context
func (_e *MockFileStorage_Expecter) LocalDiskUsage(ctx interface{}) *MockFileStorage_LocalDiskUsage_Call {
	return &MockFileStorage_LocalDiskUsage_Call{Call: _e.mock.On("LocalDiskUsage", ctx)}
}

func (_c *MockFileStorage_LocalDiskUsage_Call) Run(run func(ctx context.Context)) *MockFileStorage_LocalDiskUsage_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context))
	})
	return _c
}

func (_c *MockFileStorage_LocalDiskUsage_Call) Return(_a0 uint64, _a1 error) *MockFileStorage_LocalDiskUsage_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockFileStorage_LocalDiskUsage_Call) RunAndReturn(run func(context.Context) (uint64, error)) *MockFileStorage_LocalDiskUsage_Call {
	_c.Call.Return(run)
	return _c
}

// Name provides a mock function with given fields:
func (_m *MockFileStorage) Name() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Name")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// MockFileStorage_Name_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Name'
type MockFileStorage_Name_Call struct {
	*mock.Call
}

// Name is a helper method to define mock.On call
func (_e *MockFileStorage_Expecter) Name() *MockFileStorage_Name_Call {
	return &MockFileStorage_Name_Call{Call: _e.mock.On("Name")}
}

func (_c *MockFileStorage_Name_Call) Run(run func()) *MockFileStorage_Name_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockFileStorage_Name_Call) Return(name string) *MockFileStorage_Name_Call {
	_c.Call.Return(name)
	return _c
}

func (_c *MockFileStorage_Name_Call) RunAndReturn(run func() string) *MockFileStorage_Name_Call {
	_c.Call.Return(run)
	return _c
}

// NewLocalStoreGarbageCollector provides a mock function with given fields:
func (_m *MockFileStorage) NewLocalStoreGarbageCollector() filestorage.LocalStoreGarbageCollector {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for NewLocalStoreGarbageCollector")
	}

	var r0 filestorage.LocalStoreGarbageCollector
	if rf, ok := ret.Get(0).(func() filestorage.LocalStoreGarbageCollector); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(filestorage.LocalStoreGarbageCollector)
		}
	}

	return r0
}

// MockFileStorage_NewLocalStoreGarbageCollector_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'NewLocalStoreGarbageCollector'
type MockFileStorage_NewLocalStoreGarbageCollector_Call struct {
	*mock.Call
}

// NewLocalStoreGarbageCollector is a helper method to define mock.On call
func (_e *MockFileStorage_Expecter) NewLocalStoreGarbageCollector() *MockFileStorage_NewLocalStoreGarbageCollector_Call {
	return &MockFileStorage_NewLocalStoreGarbageCollector_Call{Call: _e.mock.On("NewLocalStoreGarbageCollector")}
}

func (_c *MockFileStorage_NewLocalStoreGarbageCollector_Call) Run(run func()) *MockFileStorage_NewLocalStoreGarbageCollector_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockFileStorage_NewLocalStoreGarbageCollector_Call) Return(_a0 filestorage.LocalStoreGarbageCollector) *MockFileStorage_NewLocalStoreGarbageCollector_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockFileStorage_NewLocalStoreGarbageCollector_Call) RunAndReturn(run func() filestorage.LocalStoreGarbageCollector) *MockFileStorage_NewLocalStoreGarbageCollector_Call {
	_c.Call.Return(run)
	return _c
}

// NotExistsBlocks provides a mock function with given fields: ctx, bs
func (_m *MockFileStorage) NotExistsBlocks(ctx context.Context, bs []blocks.Block) ([]blocks.Block, error) {
	ret := _m.Called(ctx, bs)

	if len(ret) == 0 {
		panic("no return value specified for NotExistsBlocks")
	}

	var r0 []blocks.Block
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, []blocks.Block) ([]blocks.Block, error)); ok {
		return rf(ctx, bs)
	}
	if rf, ok := ret.Get(0).(func(context.Context, []blocks.Block) []blocks.Block); ok {
		r0 = rf(ctx, bs)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]blocks.Block)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, []blocks.Block) error); ok {
		r1 = rf(ctx, bs)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockFileStorage_NotExistsBlocks_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'NotExistsBlocks'
type MockFileStorage_NotExistsBlocks_Call struct {
	*mock.Call
}

// NotExistsBlocks is a helper method to define mock.On call
//   - ctx context.Context
//   - bs []blocks.Block
func (_e *MockFileStorage_Expecter) NotExistsBlocks(ctx interface{}, bs interface{}) *MockFileStorage_NotExistsBlocks_Call {
	return &MockFileStorage_NotExistsBlocks_Call{Call: _e.mock.On("NotExistsBlocks", ctx, bs)}
}

func (_c *MockFileStorage_NotExistsBlocks_Call) Run(run func(ctx context.Context, bs []blocks.Block)) *MockFileStorage_NotExistsBlocks_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].([]blocks.Block))
	})
	return _c
}

func (_c *MockFileStorage_NotExistsBlocks_Call) Return(notExists []blocks.Block, err error) *MockFileStorage_NotExistsBlocks_Call {
	_c.Call.Return(notExists, err)
	return _c
}

func (_c *MockFileStorage_NotExistsBlocks_Call) RunAndReturn(run func(context.Context, []blocks.Block) ([]blocks.Block, error)) *MockFileStorage_NotExistsBlocks_Call {
	_c.Call.Return(run)
	return _c
}

// Run provides a mock function with given fields: ctx
func (_m *MockFileStorage) Run(ctx context.Context) error {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for Run")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context) error); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockFileStorage_Run_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Run'
type MockFileStorage_Run_Call struct {
	*mock.Call
}

// Run is a helper method to define mock.On call
//   - ctx context.Context
func (_e *MockFileStorage_Expecter) Run(ctx interface{}) *MockFileStorage_Run_Call {
	return &MockFileStorage_Run_Call{Call: _e.mock.On("Run", ctx)}
}

func (_c *MockFileStorage_Run_Call) Run(run func(ctx context.Context)) *MockFileStorage_Run_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context))
	})
	return _c
}

func (_c *MockFileStorage_Run_Call) Return(err error) *MockFileStorage_Run_Call {
	_c.Call.Return(err)
	return _c
}

func (_c *MockFileStorage_Run_Call) RunAndReturn(run func(context.Context) error) *MockFileStorage_Run_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockFileStorage creates a new instance of MockFileStorage. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockFileStorage(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockFileStorage {
	mock := &MockFileStorage{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
