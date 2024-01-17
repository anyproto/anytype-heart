// Code generated by mockery. DO NOT EDIT.

package mock_files

import (
	context "context"

	app "github.com/anyproto/any-sync/app"

	domain "github.com/anyproto/anytype-heart/core/domain"

	files "github.com/anyproto/anytype-heart/core/files"

	mock "github.com/stretchr/testify/mock"

	pb "github.com/anyproto/anytype-heart/pb"
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

// FileAdd provides a mock function with given fields: ctx, spaceID, options
func (_m *MockService) FileAdd(ctx context.Context, spaceID string, options ...files.AddOption) (*files.FileAddResult, error) {
	_va := make([]interface{}, len(options))
	for _i := range options {
		_va[_i] = options[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, spaceID)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for FileAdd")
	}

	var r0 *files.FileAddResult
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, ...files.AddOption) (*files.FileAddResult, error)); ok {
		return rf(ctx, spaceID, options...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, ...files.AddOption) *files.FileAddResult); ok {
		r0 = rf(ctx, spaceID, options...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*files.FileAddResult)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, ...files.AddOption) error); ok {
		r1 = rf(ctx, spaceID, options...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockService_FileAdd_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'FileAdd'
type MockService_FileAdd_Call struct {
	*mock.Call
}

// FileAdd is a helper method to define mock.On call
//   - ctx context.Context
//   - spaceID string
//   - options ...files.AddOption
func (_e *MockService_Expecter) FileAdd(ctx interface{}, spaceID interface{}, options ...interface{}) *MockService_FileAdd_Call {
	return &MockService_FileAdd_Call{Call: _e.mock.On("FileAdd",
		append([]interface{}{ctx, spaceID}, options...)...)}
}

func (_c *MockService_FileAdd_Call) Run(run func(ctx context.Context, spaceID string, options ...files.AddOption)) *MockService_FileAdd_Call {
	_c.Call.Run(func(args mock.Arguments) {
		variadicArgs := make([]files.AddOption, len(args)-2)
		for i, a := range args[2:] {
			if a != nil {
				variadicArgs[i] = a.(files.AddOption)
			}
		}
		run(args[0].(context.Context), args[1].(string), variadicArgs...)
	})
	return _c
}

func (_c *MockService_FileAdd_Call) Return(_a0 *files.FileAddResult, _a1 error) *MockService_FileAdd_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockService_FileAdd_Call) RunAndReturn(run func(context.Context, string, ...files.AddOption) (*files.FileAddResult, error)) *MockService_FileAdd_Call {
	_c.Call.Return(run)
	return _c
}

// FileByHash provides a mock function with given fields: ctx, id
func (_m *MockService) FileByHash(ctx context.Context, id domain.FullFileId) (files.File, error) {
	ret := _m.Called(ctx, id)

	if len(ret) == 0 {
		panic("no return value specified for FileByHash")
	}

	var r0 files.File
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, domain.FullFileId) (files.File, error)); ok {
		return rf(ctx, id)
	}
	if rf, ok := ret.Get(0).(func(context.Context, domain.FullFileId) files.File); ok {
		r0 = rf(ctx, id)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(files.File)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, domain.FullFileId) error); ok {
		r1 = rf(ctx, id)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockService_FileByHash_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'FileByHash'
type MockService_FileByHash_Call struct {
	*mock.Call
}

// FileByHash is a helper method to define mock.On call
//   - ctx context.Context
//   - id domain.FullFileId
func (_e *MockService_Expecter) FileByHash(ctx interface{}, id interface{}) *MockService_FileByHash_Call {
	return &MockService_FileByHash_Call{Call: _e.mock.On("FileByHash", ctx, id)}
}

func (_c *MockService_FileByHash_Call) Run(run func(ctx context.Context, id domain.FullFileId)) *MockService_FileByHash_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(domain.FullFileId))
	})
	return _c
}

func (_c *MockService_FileByHash_Call) Return(_a0 files.File, _a1 error) *MockService_FileByHash_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockService_FileByHash_Call) RunAndReturn(run func(context.Context, domain.FullFileId) (files.File, error)) *MockService_FileByHash_Call {
	_c.Call.Return(run)
	return _c
}

// FileGetKeys provides a mock function with given fields: id
func (_m *MockService) FileGetKeys(id domain.FullFileId) (*domain.FileEncryptionKeys, error) {
	ret := _m.Called(id)

	if len(ret) == 0 {
		panic("no return value specified for FileGetKeys")
	}

	var r0 *domain.FileEncryptionKeys
	var r1 error
	if rf, ok := ret.Get(0).(func(domain.FullFileId) (*domain.FileEncryptionKeys, error)); ok {
		return rf(id)
	}
	if rf, ok := ret.Get(0).(func(domain.FullFileId) *domain.FileEncryptionKeys); ok {
		r0 = rf(id)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*domain.FileEncryptionKeys)
		}
	}

	if rf, ok := ret.Get(1).(func(domain.FullFileId) error); ok {
		r1 = rf(id)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockService_FileGetKeys_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'FileGetKeys'
type MockService_FileGetKeys_Call struct {
	*mock.Call
}

// FileGetKeys is a helper method to define mock.On call
//   - id domain.FullFileId
func (_e *MockService_Expecter) FileGetKeys(id interface{}) *MockService_FileGetKeys_Call {
	return &MockService_FileGetKeys_Call{Call: _e.mock.On("FileGetKeys", id)}
}

func (_c *MockService_FileGetKeys_Call) Run(run func(id domain.FullFileId)) *MockService_FileGetKeys_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(domain.FullFileId))
	})
	return _c
}

func (_c *MockService_FileGetKeys_Call) Return(_a0 *domain.FileEncryptionKeys, _a1 error) *MockService_FileGetKeys_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockService_FileGetKeys_Call) RunAndReturn(run func(domain.FullFileId) (*domain.FileEncryptionKeys, error)) *MockService_FileGetKeys_Call {
	_c.Call.Return(run)
	return _c
}

// FileOffload provides a mock function with given fields: ctx, id
func (_m *MockService) FileOffload(ctx context.Context, id domain.FullFileId) (uint64, error) {
	ret := _m.Called(ctx, id)

	if len(ret) == 0 {
		panic("no return value specified for FileOffload")
	}

	var r0 uint64
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, domain.FullFileId) (uint64, error)); ok {
		return rf(ctx, id)
	}
	if rf, ok := ret.Get(0).(func(context.Context, domain.FullFileId) uint64); ok {
		r0 = rf(ctx, id)
	} else {
		r0 = ret.Get(0).(uint64)
	}

	if rf, ok := ret.Get(1).(func(context.Context, domain.FullFileId) error); ok {
		r1 = rf(ctx, id)
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
//   - id domain.FullFileId
func (_e *MockService_Expecter) FileOffload(ctx interface{}, id interface{}) *MockService_FileOffload_Call {
	return &MockService_FileOffload_Call{Call: _e.mock.On("FileOffload", ctx, id)}
}

func (_c *MockService_FileOffload_Call) Run(run func(ctx context.Context, id domain.FullFileId)) *MockService_FileOffload_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(domain.FullFileId))
	})
	return _c
}

func (_c *MockService_FileOffload_Call) Return(totalSize uint64, err error) *MockService_FileOffload_Call {
	_c.Call.Return(totalSize, err)
	return _c
}

func (_c *MockService_FileOffload_Call) RunAndReturn(run func(context.Context, domain.FullFileId) (uint64, error)) *MockService_FileOffload_Call {
	_c.Call.Return(run)
	return _c
}

// GetNodeUsage provides a mock function with given fields: ctx
func (_m *MockService) GetNodeUsage(ctx context.Context) (*files.NodeUsageResponse, error) {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for GetNodeUsage")
	}

	var r0 *files.NodeUsageResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context) (*files.NodeUsageResponse, error)); ok {
		return rf(ctx)
	}
	if rf, ok := ret.Get(0).(func(context.Context) *files.NodeUsageResponse); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*files.NodeUsageResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockService_GetNodeUsage_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetNodeUsage'
type MockService_GetNodeUsage_Call struct {
	*mock.Call
}

// GetNodeUsage is a helper method to define mock.On call
//   - ctx context.Context
func (_e *MockService_Expecter) GetNodeUsage(ctx interface{}) *MockService_GetNodeUsage_Call {
	return &MockService_GetNodeUsage_Call{Call: _e.mock.On("GetNodeUsage", ctx)}
}

func (_c *MockService_GetNodeUsage_Call) Run(run func(ctx context.Context)) *MockService_GetNodeUsage_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context))
	})
	return _c
}

func (_c *MockService_GetNodeUsage_Call) Return(_a0 *files.NodeUsageResponse, _a1 error) *MockService_GetNodeUsage_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockService_GetNodeUsage_Call) RunAndReturn(run func(context.Context) (*files.NodeUsageResponse, error)) *MockService_GetNodeUsage_Call {
	_c.Call.Return(run)
	return _c
}

// GetSpaceUsage provides a mock function with given fields: ctx, spaceID
func (_m *MockService) GetSpaceUsage(ctx context.Context, spaceID string) (*pb.RpcFileSpaceUsageResponseUsage, error) {
	ret := _m.Called(ctx, spaceID)

	if len(ret) == 0 {
		panic("no return value specified for GetSpaceUsage")
	}

	var r0 *pb.RpcFileSpaceUsageResponseUsage
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (*pb.RpcFileSpaceUsageResponseUsage, error)); ok {
		return rf(ctx, spaceID)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) *pb.RpcFileSpaceUsageResponseUsage); ok {
		r0 = rf(ctx, spaceID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*pb.RpcFileSpaceUsageResponseUsage)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, spaceID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockService_GetSpaceUsage_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetSpaceUsage'
type MockService_GetSpaceUsage_Call struct {
	*mock.Call
}

// GetSpaceUsage is a helper method to define mock.On call
//   - ctx context.Context
//   - spaceID string
func (_e *MockService_Expecter) GetSpaceUsage(ctx interface{}, spaceID interface{}) *MockService_GetSpaceUsage_Call {
	return &MockService_GetSpaceUsage_Call{Call: _e.mock.On("GetSpaceUsage", ctx, spaceID)}
}

func (_c *MockService_GetSpaceUsage_Call) Run(run func(ctx context.Context, spaceID string)) *MockService_GetSpaceUsage_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string))
	})
	return _c
}

func (_c *MockService_GetSpaceUsage_Call) Return(_a0 *pb.RpcFileSpaceUsageResponseUsage, _a1 error) *MockService_GetSpaceUsage_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockService_GetSpaceUsage_Call) RunAndReturn(run func(context.Context, string) (*pb.RpcFileSpaceUsageResponseUsage, error)) *MockService_GetSpaceUsage_Call {
	_c.Call.Return(run)
	return _c
}

// ImageAdd provides a mock function with given fields: ctx, spaceID, options
func (_m *MockService) ImageAdd(ctx context.Context, spaceID string, options ...files.AddOption) (*files.ImageAddResult, error) {
	_va := make([]interface{}, len(options))
	for _i := range options {
		_va[_i] = options[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, spaceID)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for ImageAdd")
	}

	var r0 *files.ImageAddResult
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, ...files.AddOption) (*files.ImageAddResult, error)); ok {
		return rf(ctx, spaceID, options...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, ...files.AddOption) *files.ImageAddResult); ok {
		r0 = rf(ctx, spaceID, options...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*files.ImageAddResult)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, ...files.AddOption) error); ok {
		r1 = rf(ctx, spaceID, options...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockService_ImageAdd_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ImageAdd'
type MockService_ImageAdd_Call struct {
	*mock.Call
}

// ImageAdd is a helper method to define mock.On call
//   - ctx context.Context
//   - spaceID string
//   - options ...files.AddOption
func (_e *MockService_Expecter) ImageAdd(ctx interface{}, spaceID interface{}, options ...interface{}) *MockService_ImageAdd_Call {
	return &MockService_ImageAdd_Call{Call: _e.mock.On("ImageAdd",
		append([]interface{}{ctx, spaceID}, options...)...)}
}

func (_c *MockService_ImageAdd_Call) Run(run func(ctx context.Context, spaceID string, options ...files.AddOption)) *MockService_ImageAdd_Call {
	_c.Call.Run(func(args mock.Arguments) {
		variadicArgs := make([]files.AddOption, len(args)-2)
		for i, a := range args[2:] {
			if a != nil {
				variadicArgs[i] = a.(files.AddOption)
			}
		}
		run(args[0].(context.Context), args[1].(string), variadicArgs...)
	})
	return _c
}

func (_c *MockService_ImageAdd_Call) Return(_a0 *files.ImageAddResult, _a1 error) *MockService_ImageAdd_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockService_ImageAdd_Call) RunAndReturn(run func(context.Context, string, ...files.AddOption) (*files.ImageAddResult, error)) *MockService_ImageAdd_Call {
	_c.Call.Return(run)
	return _c
}

// ImageByHash provides a mock function with given fields: ctx, id
func (_m *MockService) ImageByHash(ctx context.Context, id domain.FullFileId) (files.Image, error) {
	ret := _m.Called(ctx, id)

	if len(ret) == 0 {
		panic("no return value specified for ImageByHash")
	}

	var r0 files.Image
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, domain.FullFileId) (files.Image, error)); ok {
		return rf(ctx, id)
	}
	if rf, ok := ret.Get(0).(func(context.Context, domain.FullFileId) files.Image); ok {
		r0 = rf(ctx, id)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(files.Image)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, domain.FullFileId) error); ok {
		r1 = rf(ctx, id)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockService_ImageByHash_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ImageByHash'
type MockService_ImageByHash_Call struct {
	*mock.Call
}

// ImageByHash is a helper method to define mock.On call
//   - ctx context.Context
//   - id domain.FullFileId
func (_e *MockService_Expecter) ImageByHash(ctx interface{}, id interface{}) *MockService_ImageByHash_Call {
	return &MockService_ImageByHash_Call{Call: _e.mock.On("ImageByHash", ctx, id)}
}

func (_c *MockService_ImageByHash_Call) Run(run func(ctx context.Context, id domain.FullFileId)) *MockService_ImageByHash_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(domain.FullFileId))
	})
	return _c
}

func (_c *MockService_ImageByHash_Call) Return(_a0 files.Image, _a1 error) *MockService_ImageByHash_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockService_ImageByHash_Call) RunAndReturn(run func(context.Context, domain.FullFileId) (files.Image, error)) *MockService_ImageByHash_Call {
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