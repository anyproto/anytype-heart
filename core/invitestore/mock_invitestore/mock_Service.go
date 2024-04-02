// Code generated by mockery. DO NOT EDIT.

package mock_invitestore

import (
	app "github.com/anyproto/any-sync/app"
	cid "github.com/ipfs/go-cid"

	context "context"

	crypto "github.com/anyproto/any-sync/util/crypto"

	mock "github.com/stretchr/testify/mock"

	model "github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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

// Close provides a mock function with given fields: ctx
func (_m *MockService) Close(ctx context.Context) error {
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

// MockService_Close_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Close'
type MockService_Close_Call struct {
	*mock.Call
}

// Close is a helper method to define mock.On call
//   - ctx context.Context
func (_e *MockService_Expecter) Close(ctx interface{}) *MockService_Close_Call {
	return &MockService_Close_Call{Call: _e.mock.On("Close", ctx)}
}

func (_c *MockService_Close_Call) Run(run func(ctx context.Context)) *MockService_Close_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context))
	})
	return _c
}

func (_c *MockService_Close_Call) Return(err error) *MockService_Close_Call {
	_c.Call.Return(err)
	return _c
}

func (_c *MockService_Close_Call) RunAndReturn(run func(context.Context) error) *MockService_Close_Call {
	_c.Call.Return(run)
	return _c
}

// GetInvite provides a mock function with given fields: ctx, id, key
func (_m *MockService) GetInvite(ctx context.Context, id cid.Cid, key crypto.SymKey) (*model.Invite, error) {
	ret := _m.Called(ctx, id, key)

	if len(ret) == 0 {
		panic("no return value specified for GetInvite")
	}

	var r0 *model.Invite
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, cid.Cid, crypto.SymKey) (*model.Invite, error)); ok {
		return rf(ctx, id, key)
	}
	if rf, ok := ret.Get(0).(func(context.Context, cid.Cid, crypto.SymKey) *model.Invite); ok {
		r0 = rf(ctx, id, key)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.Invite)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, cid.Cid, crypto.SymKey) error); ok {
		r1 = rf(ctx, id, key)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockService_GetInvite_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetInvite'
type MockService_GetInvite_Call struct {
	*mock.Call
}

// GetInvite is a helper method to define mock.On call
//   - ctx context.Context
//   - id cid.Cid
//   - key crypto.SymKey
func (_e *MockService_Expecter) GetInvite(ctx interface{}, id interface{}, key interface{}) *MockService_GetInvite_Call {
	return &MockService_GetInvite_Call{Call: _e.mock.On("GetInvite", ctx, id, key)}
}

func (_c *MockService_GetInvite_Call) Run(run func(ctx context.Context, id cid.Cid, key crypto.SymKey)) *MockService_GetInvite_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(cid.Cid), args[2].(crypto.SymKey))
	})
	return _c
}

func (_c *MockService_GetInvite_Call) Return(_a0 *model.Invite, _a1 error) *MockService_GetInvite_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockService_GetInvite_Call) RunAndReturn(run func(context.Context, cid.Cid, crypto.SymKey) (*model.Invite, error)) *MockService_GetInvite_Call {
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

// RemoveInvite provides a mock function with given fields: ctx, id
func (_m *MockService) RemoveInvite(ctx context.Context, id cid.Cid) error {
	ret := _m.Called(ctx, id)

	if len(ret) == 0 {
		panic("no return value specified for RemoveInvite")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, cid.Cid) error); ok {
		r0 = rf(ctx, id)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockService_RemoveInvite_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'RemoveInvite'
type MockService_RemoveInvite_Call struct {
	*mock.Call
}

// RemoveInvite is a helper method to define mock.On call
//   - ctx context.Context
//   - id cid.Cid
func (_e *MockService_Expecter) RemoveInvite(ctx interface{}, id interface{}) *MockService_RemoveInvite_Call {
	return &MockService_RemoveInvite_Call{Call: _e.mock.On("RemoveInvite", ctx, id)}
}

func (_c *MockService_RemoveInvite_Call) Run(run func(ctx context.Context, id cid.Cid)) *MockService_RemoveInvite_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(cid.Cid))
	})
	return _c
}

func (_c *MockService_RemoveInvite_Call) Return(_a0 error) *MockService_RemoveInvite_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockService_RemoveInvite_Call) RunAndReturn(run func(context.Context, cid.Cid) error) *MockService_RemoveInvite_Call {
	_c.Call.Return(run)
	return _c
}

// Run provides a mock function with given fields: ctx
func (_m *MockService) Run(ctx context.Context) error {
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

// MockService_Run_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Run'
type MockService_Run_Call struct {
	*mock.Call
}

// Run is a helper method to define mock.On call
//   - ctx context.Context
func (_e *MockService_Expecter) Run(ctx interface{}) *MockService_Run_Call {
	return &MockService_Run_Call{Call: _e.mock.On("Run", ctx)}
}

func (_c *MockService_Run_Call) Run(run func(ctx context.Context)) *MockService_Run_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context))
	})
	return _c
}

func (_c *MockService_Run_Call) Return(err error) *MockService_Run_Call {
	_c.Call.Return(err)
	return _c
}

func (_c *MockService_Run_Call) RunAndReturn(run func(context.Context) error) *MockService_Run_Call {
	_c.Call.Return(run)
	return _c
}

// StoreInvite provides a mock function with given fields: ctx, invite
func (_m *MockService) StoreInvite(ctx context.Context, invite *model.Invite) (cid.Cid, crypto.SymKey, error) {
	ret := _m.Called(ctx, invite)

	if len(ret) == 0 {
		panic("no return value specified for StoreInvite")
	}

	var r0 cid.Cid
	var r1 crypto.SymKey
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, *model.Invite) (cid.Cid, crypto.SymKey, error)); ok {
		return rf(ctx, invite)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *model.Invite) cid.Cid); ok {
		r0 = rf(ctx, invite)
	} else {
		r0 = ret.Get(0).(cid.Cid)
	}

	if rf, ok := ret.Get(1).(func(context.Context, *model.Invite) crypto.SymKey); ok {
		r1 = rf(ctx, invite)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(crypto.SymKey)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context, *model.Invite) error); ok {
		r2 = rf(ctx, invite)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// MockService_StoreInvite_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'StoreInvite'
type MockService_StoreInvite_Call struct {
	*mock.Call
}

// StoreInvite is a helper method to define mock.On call
//   - ctx context.Context
//   - invite *model.Invite
func (_e *MockService_Expecter) StoreInvite(ctx interface{}, invite interface{}) *MockService_StoreInvite_Call {
	return &MockService_StoreInvite_Call{Call: _e.mock.On("StoreInvite", ctx, invite)}
}

func (_c *MockService_StoreInvite_Call) Run(run func(ctx context.Context, invite *model.Invite)) *MockService_StoreInvite_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*model.Invite))
	})
	return _c
}

func (_c *MockService_StoreInvite_Call) Return(id cid.Cid, key crypto.SymKey, err error) *MockService_StoreInvite_Call {
	_c.Call.Return(id, key, err)
	return _c
}

func (_c *MockService_StoreInvite_Call) RunAndReturn(run func(context.Context, *model.Invite) (cid.Cid, crypto.SymKey, error)) *MockService_StoreInvite_Call {
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