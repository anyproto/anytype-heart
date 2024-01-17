// Code generated by mockery. DO NOT EDIT.

package mock_fileobject

import (
	context "context"

	app "github.com/anyproto/any-sync/app"
	clientspace "github.com/anyproto/anytype-heart/space/clientspace"

	domain "github.com/anyproto/anytype-heart/core/domain"

	fileobject "github.com/anyproto/anytype-heart/core/files/fileobject"

	mock "github.com/stretchr/testify/mock"

	pb "github.com/anyproto/anytype-heart/pb"

	source "github.com/anyproto/anytype-heart/core/block/source"

	state "github.com/anyproto/anytype-heart/core/block/editor/state"

	types "github.com/gogo/protobuf/types"
)

// MockService is an autogenerated mock type for the Service type
type MockService struct {
	mock.Mock
}

type MockService_Expecter struct {
	mock *mock.Mock
}

func (_m *MockService) EXPECT() *MockService_Expecter {
	return &MockService_Expecter{mock: &_m.Mock}
}

// Create provides a mock function with given fields: ctx, spaceId, req
func (_m *MockService) Create(ctx context.Context, spaceId string, req fileobject.CreateRequest) (string, *types.Struct, error) {
	ret := _m.Called(ctx, spaceId, req)

	if len(ret) == 0 {
		panic("no return value specified for Create")
	}

	var r0 string
	var r1 *types.Struct
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, string, fileobject.CreateRequest) (string, *types.Struct, error)); ok {
		return rf(ctx, spaceId, req)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, fileobject.CreateRequest) string); ok {
		r0 = rf(ctx, spaceId, req)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, fileobject.CreateRequest) *types.Struct); ok {
		r1 = rf(ctx, spaceId, req)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*types.Struct)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context, string, fileobject.CreateRequest) error); ok {
		r2 = rf(ctx, spaceId, req)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// MockService_Create_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Create'
type MockService_Create_Call struct {
	*mock.Call
}

// Create is a helper method to define mock.On call
//   - ctx context.Context
//   - spaceId string
//   - req fileobject.CreateRequest
func (_e *MockService_Expecter) Create(ctx interface{}, spaceId interface{}, req interface{}) *MockService_Create_Call {
	return &MockService_Create_Call{Call: _e.mock.On("Create", ctx, spaceId, req)}
}

func (_c *MockService_Create_Call) Run(run func(ctx context.Context, spaceId string, req fileobject.CreateRequest)) *MockService_Create_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string), args[2].(fileobject.CreateRequest))
	})
	return _c
}

func (_c *MockService_Create_Call) Return(id string, object *types.Struct, err error) *MockService_Create_Call {
	_c.Call.Return(id, object, err)
	return _c
}

func (_c *MockService_Create_Call) RunAndReturn(run func(context.Context, string, fileobject.CreateRequest) (string, *types.Struct, error)) *MockService_Create_Call {
	_c.Call.Return(run)
	return _c
}

// DeleteFileData provides a mock function with given fields: ctx, space, objectId
func (_m *MockService) DeleteFileData(ctx context.Context, space clientspace.Space, objectId string) error {
	ret := _m.Called(ctx, space, objectId)

	if len(ret) == 0 {
		panic("no return value specified for DeleteFileData")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, clientspace.Space, string) error); ok {
		r0 = rf(ctx, space, objectId)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockService_DeleteFileData_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'DeleteFileData'
type MockService_DeleteFileData_Call struct {
	*mock.Call
}

// DeleteFileData is a helper method to define mock.On call
//   - ctx context.Context
//   - space clientspace.Space
//   - objectId string
func (_e *MockService_Expecter) DeleteFileData(ctx interface{}, space interface{}, objectId interface{}) *MockService_DeleteFileData_Call {
	return &MockService_DeleteFileData_Call{Call: _e.mock.On("DeleteFileData", ctx, space, objectId)}
}

func (_c *MockService_DeleteFileData_Call) Run(run func(ctx context.Context, space clientspace.Space, objectId string)) *MockService_DeleteFileData_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(clientspace.Space), args[2].(string))
	})
	return _c
}

func (_c *MockService_DeleteFileData_Call) Return(_a0 error) *MockService_DeleteFileData_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockService_DeleteFileData_Call) RunAndReturn(run func(context.Context, clientspace.Space, string) error) *MockService_DeleteFileData_Call {
	_c.Call.Return(run)
	return _c
}

// FileOffload provides a mock function with given fields: ctx, objectId, includeNotPinned
func (_m *MockService) FileOffload(ctx context.Context, objectId string, includeNotPinned bool) (uint64, error) {
	ret := _m.Called(ctx, objectId, includeNotPinned)

	if len(ret) == 0 {
		panic("no return value specified for FileOffload")
	}

	var r0 uint64
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, bool) (uint64, error)); ok {
		return rf(ctx, objectId, includeNotPinned)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, bool) uint64); ok {
		r0 = rf(ctx, objectId, includeNotPinned)
	} else {
		r0 = ret.Get(0).(uint64)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, bool) error); ok {
		r1 = rf(ctx, objectId, includeNotPinned)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockService_FileOffload_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'FileOffload'
type MockService_FileOffload_Call struct {
	*mock.Call
}

// FileOffload is a helper method to define mock.On call
//   - ctx context.Context
//   - objectId string
//   - includeNotPinned bool
func (_e *MockService_Expecter) FileOffload(ctx interface{}, objectId interface{}, includeNotPinned interface{}) *MockService_FileOffload_Call {
	return &MockService_FileOffload_Call{Call: _e.mock.On("FileOffload", ctx, objectId, includeNotPinned)}
}

func (_c *MockService_FileOffload_Call) Run(run func(ctx context.Context, objectId string, includeNotPinned bool)) *MockService_FileOffload_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string), args[2].(bool))
	})
	return _c
}

func (_c *MockService_FileOffload_Call) Return(totalSize uint64, err error) *MockService_FileOffload_Call {
	_c.Call.Return(totalSize, err)
	return _c
}

func (_c *MockService_FileOffload_Call) RunAndReturn(run func(context.Context, string, bool) (uint64, error)) *MockService_FileOffload_Call {
	_c.Call.Return(run)
	return _c
}

// FileSpaceOffload provides a mock function with given fields: ctx, spaceId, includeNotPinned
func (_m *MockService) FileSpaceOffload(ctx context.Context, spaceId string, includeNotPinned bool) (int, uint64, error) {
	ret := _m.Called(ctx, spaceId, includeNotPinned)

	if len(ret) == 0 {
		panic("no return value specified for FileSpaceOffload")
	}

	var r0 int
	var r1 uint64
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, string, bool) (int, uint64, error)); ok {
		return rf(ctx, spaceId, includeNotPinned)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, bool) int); ok {
		r0 = rf(ctx, spaceId, includeNotPinned)
	} else {
		r0 = ret.Get(0).(int)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, bool) uint64); ok {
		r1 = rf(ctx, spaceId, includeNotPinned)
	} else {
		r1 = ret.Get(1).(uint64)
	}

	if rf, ok := ret.Get(2).(func(context.Context, string, bool) error); ok {
		r2 = rf(ctx, spaceId, includeNotPinned)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// MockService_FileSpaceOffload_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'FileSpaceOffload'
type MockService_FileSpaceOffload_Call struct {
	*mock.Call
}

// FileSpaceOffload is a helper method to define mock.On call
//   - ctx context.Context
//   - spaceId string
//   - includeNotPinned bool
func (_e *MockService_Expecter) FileSpaceOffload(ctx interface{}, spaceId interface{}, includeNotPinned interface{}) *MockService_FileSpaceOffload_Call {
	return &MockService_FileSpaceOffload_Call{Call: _e.mock.On("FileSpaceOffload", ctx, spaceId, includeNotPinned)}
}

func (_c *MockService_FileSpaceOffload_Call) Run(run func(ctx context.Context, spaceId string, includeNotPinned bool)) *MockService_FileSpaceOffload_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string), args[2].(bool))
	})
	return _c
}

func (_c *MockService_FileSpaceOffload_Call) Return(filesOffloaded int, totalSize uint64, err error) *MockService_FileSpaceOffload_Call {
	_c.Call.Return(filesOffloaded, totalSize, err)
	return _c
}

func (_c *MockService_FileSpaceOffload_Call) RunAndReturn(run func(context.Context, string, bool) (int, uint64, error)) *MockService_FileSpaceOffload_Call {
	_c.Call.Return(run)
	return _c
}

// FilesOffload provides a mock function with given fields: ctx, objectIds, includeNotPinned
func (_m *MockService) FilesOffload(ctx context.Context, objectIds []string, includeNotPinned bool) (int, uint64, error) {
	ret := _m.Called(ctx, objectIds, includeNotPinned)

	if len(ret) == 0 {
		panic("no return value specified for FilesOffload")
	}

	var r0 int
	var r1 uint64
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, []string, bool) (int, uint64, error)); ok {
		return rf(ctx, objectIds, includeNotPinned)
	}
	if rf, ok := ret.Get(0).(func(context.Context, []string, bool) int); ok {
		r0 = rf(ctx, objectIds, includeNotPinned)
	} else {
		r0 = ret.Get(0).(int)
	}

	if rf, ok := ret.Get(1).(func(context.Context, []string, bool) uint64); ok {
		r1 = rf(ctx, objectIds, includeNotPinned)
	} else {
		r1 = ret.Get(1).(uint64)
	}

	if rf, ok := ret.Get(2).(func(context.Context, []string, bool) error); ok {
		r2 = rf(ctx, objectIds, includeNotPinned)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// MockService_FilesOffload_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'FilesOffload'
type MockService_FilesOffload_Call struct {
	*mock.Call
}

// FilesOffload is a helper method to define mock.On call
//   - ctx context.Context
//   - objectIds []string
//   - includeNotPinned bool
func (_e *MockService_Expecter) FilesOffload(ctx interface{}, objectIds interface{}, includeNotPinned interface{}) *MockService_FilesOffload_Call {
	return &MockService_FilesOffload_Call{Call: _e.mock.On("FilesOffload", ctx, objectIds, includeNotPinned)}
}

func (_c *MockService_FilesOffload_Call) Run(run func(ctx context.Context, objectIds []string, includeNotPinned bool)) *MockService_FilesOffload_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].([]string), args[2].(bool))
	})
	return _c
}

func (_c *MockService_FilesOffload_Call) Return(filesOffloaded int, totalSize uint64, err error) *MockService_FilesOffload_Call {
	_c.Call.Return(filesOffloaded, totalSize, err)
	return _c
}

func (_c *MockService_FilesOffload_Call) RunAndReturn(run func(context.Context, []string, bool) (int, uint64, error)) *MockService_FilesOffload_Call {
	_c.Call.Return(run)
	return _c
}

// GetFileIdFromObject provides a mock function with given fields: ctx, objectId
func (_m *MockService) GetFileIdFromObject(ctx context.Context, objectId string) (domain.FullFileId, error) {
	ret := _m.Called(ctx, objectId)

	if len(ret) == 0 {
		panic("no return value specified for GetFileIdFromObject")
	}

	var r0 domain.FullFileId
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (domain.FullFileId, error)); ok {
		return rf(ctx, objectId)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) domain.FullFileId); ok {
		r0 = rf(ctx, objectId)
	} else {
		r0 = ret.Get(0).(domain.FullFileId)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, objectId)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockService_GetFileIdFromObject_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetFileIdFromObject'
type MockService_GetFileIdFromObject_Call struct {
	*mock.Call
}

// GetFileIdFromObject is a helper method to define mock.On call
//   - ctx context.Context
//   - objectId string
func (_e *MockService_Expecter) GetFileIdFromObject(ctx interface{}, objectId interface{}) *MockService_GetFileIdFromObject_Call {
	return &MockService_GetFileIdFromObject_Call{Call: _e.mock.On("GetFileIdFromObject", ctx, objectId)}
}

func (_c *MockService_GetFileIdFromObject_Call) Run(run func(ctx context.Context, objectId string)) *MockService_GetFileIdFromObject_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string))
	})
	return _c
}

func (_c *MockService_GetFileIdFromObject_Call) Return(_a0 domain.FullFileId, _a1 error) *MockService_GetFileIdFromObject_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockService_GetFileIdFromObject_Call) RunAndReturn(run func(context.Context, string) (domain.FullFileId, error)) *MockService_GetFileIdFromObject_Call {
	_c.Call.Return(run)
	return _c
}

// GetObjectIdByFileId provides a mock function with given fields: fileId
func (_m *MockService) GetObjectIdByFileId(fileId domain.FileId) (string, error) {
	ret := _m.Called(fileId)

	if len(ret) == 0 {
		panic("no return value specified for GetObjectIdByFileId")
	}

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(domain.FileId) (string, error)); ok {
		return rf(fileId)
	}
	if rf, ok := ret.Get(0).(func(domain.FileId) string); ok {
		r0 = rf(fileId)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(domain.FileId) error); ok {
		r1 = rf(fileId)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockService_GetObjectIdByFileId_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetObjectIdByFileId'
type MockService_GetObjectIdByFileId_Call struct {
	*mock.Call
}

// GetObjectIdByFileId is a helper method to define mock.On call
//   - fileId domain.FileId
func (_e *MockService_Expecter) GetObjectIdByFileId(fileId interface{}) *MockService_GetObjectIdByFileId_Call {
	return &MockService_GetObjectIdByFileId_Call{Call: _e.mock.On("GetObjectIdByFileId", fileId)}
}

func (_c *MockService_GetObjectIdByFileId_Call) Run(run func(fileId domain.FileId)) *MockService_GetObjectIdByFileId_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(domain.FileId))
	})
	return _c
}

func (_c *MockService_GetObjectIdByFileId_Call) Return(_a0 string, _a1 error) *MockService_GetObjectIdByFileId_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockService_GetObjectIdByFileId_Call) RunAndReturn(run func(domain.FileId) (string, error)) *MockService_GetObjectIdByFileId_Call {
	_c.Call.Return(run)
	return _c
}

// Init provides a mock function with given fields: a
func (_m *MockService) Init(a *app.App) error {
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

// MockService_Init_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Init'
type MockService_Init_Call struct {
	*mock.Call
}

// Init is a helper method to define mock.On call
//   - a *app.App
func (_e *MockService_Expecter) Init(a interface{}) *MockService_Init_Call {
	return &MockService_Init_Call{Call: _e.mock.On("Init", a)}
}

func (_c *MockService_Init_Call) Run(run func(a *app.App)) *MockService_Init_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*app.App))
	})
	return _c
}

func (_c *MockService_Init_Call) Return(err error) *MockService_Init_Call {
	_c.Call.Return(err)
	return _c
}

func (_c *MockService_Init_Call) RunAndReturn(run func(*app.App) error) *MockService_Init_Call {
	_c.Call.Return(run)
	return _c
}

// MigrateBlocks provides a mock function with given fields: st, spc, keys
func (_m *MockService) MigrateBlocks(st *state.State, spc source.Space, keys []*pb.ChangeFileKeys) {
	_m.Called(st, spc, keys)
}

// MockService_MigrateBlocks_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'MigrateBlocks'
type MockService_MigrateBlocks_Call struct {
	*mock.Call
}

// MigrateBlocks is a helper method to define mock.On call
//   - st *state.State
//   - spc source.Space
//   - keys []*pb.ChangeFileKeys
func (_e *MockService_Expecter) MigrateBlocks(st interface{}, spc interface{}, keys interface{}) *MockService_MigrateBlocks_Call {
	return &MockService_MigrateBlocks_Call{Call: _e.mock.On("MigrateBlocks", st, spc, keys)}
}

func (_c *MockService_MigrateBlocks_Call) Run(run func(st *state.State, spc source.Space, keys []*pb.ChangeFileKeys)) *MockService_MigrateBlocks_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*state.State), args[1].(source.Space), args[2].([]*pb.ChangeFileKeys))
	})
	return _c
}

func (_c *MockService_MigrateBlocks_Call) Return() *MockService_MigrateBlocks_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockService_MigrateBlocks_Call) RunAndReturn(run func(*state.State, source.Space, []*pb.ChangeFileKeys)) *MockService_MigrateBlocks_Call {
	_c.Call.Return(run)
	return _c
}

// Name provides a mock function with given fields:
func (_m *MockService) Name() string {
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

// MockService_Name_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Name'
type MockService_Name_Call struct {
	*mock.Call
}

// Name is a helper method to define mock.On call
func (_e *MockService_Expecter) Name() *MockService_Name_Call {
	return &MockService_Name_Call{Call: _e.mock.On("Name")}
}

func (_c *MockService_Name_Call) Run(run func()) *MockService_Name_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockService_Name_Call) Return(name string) *MockService_Name_Call {
	_c.Call.Return(name)
	return _c
}

func (_c *MockService_Name_Call) RunAndReturn(run func() string) *MockService_Name_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockService creates a new instance of MockService. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockService(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockService {
	mock := &MockService{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}