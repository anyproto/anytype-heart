// Code generated by mockery. DO NOT EDIT.

package mock_source

import (
	context "context"

	pb "github.com/anyproto/anytype-heart/pb"
	mock "github.com/stretchr/testify/mock"

	smartblock "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"

	source "github.com/anyproto/anytype-heart/core/block/source"

	state "github.com/anyproto/anytype-heart/core/block/editor/state"

	storestate "github.com/anyproto/anytype-heart/core/block/editor/storestate"
)

// MockStore is an autogenerated mock type for the Store type
type MockStore struct {
	mock.Mock
}

type MockStore_Expecter struct {
	mock *mock.Mock
}

func (_m *MockStore) EXPECT() *MockStore_Expecter {
	return &MockStore_Expecter{mock: &_m.Mock}
}

// Close provides a mock function with given fields:
func (_m *MockStore) Close() error {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Close")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockStore_Close_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Close'
type MockStore_Close_Call struct {
	*mock.Call
}

// Close is a helper method to define mock.On call
func (_e *MockStore_Expecter) Close() *MockStore_Close_Call {
	return &MockStore_Close_Call{Call: _e.mock.On("Close")}
}

func (_c *MockStore_Close_Call) Run(run func()) *MockStore_Close_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockStore_Close_Call) Return(err error) *MockStore_Close_Call {
	_c.Call.Return(err)
	return _c
}

func (_c *MockStore_Close_Call) RunAndReturn(run func() error) *MockStore_Close_Call {
	_c.Call.Return(run)
	return _c
}

// GetCreationInfo provides a mock function with given fields:
func (_m *MockStore) GetCreationInfo() (string, int64, error) {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetCreationInfo")
	}

	var r0 string
	var r1 int64
	var r2 error
	if rf, ok := ret.Get(0).(func() (string, int64, error)); ok {
		return rf()
	}
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func() int64); ok {
		r1 = rf()
	} else {
		r1 = ret.Get(1).(int64)
	}

	if rf, ok := ret.Get(2).(func() error); ok {
		r2 = rf()
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// MockStore_GetCreationInfo_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetCreationInfo'
type MockStore_GetCreationInfo_Call struct {
	*mock.Call
}

// GetCreationInfo is a helper method to define mock.On call
func (_e *MockStore_Expecter) GetCreationInfo() *MockStore_GetCreationInfo_Call {
	return &MockStore_GetCreationInfo_Call{Call: _e.mock.On("GetCreationInfo")}
}

func (_c *MockStore_GetCreationInfo_Call) Run(run func()) *MockStore_GetCreationInfo_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockStore_GetCreationInfo_Call) Return(creatorObjectId string, createdDate int64, err error) *MockStore_GetCreationInfo_Call {
	_c.Call.Return(creatorObjectId, createdDate, err)
	return _c
}

func (_c *MockStore_GetCreationInfo_Call) RunAndReturn(run func() (string, int64, error)) *MockStore_GetCreationInfo_Call {
	_c.Call.Return(run)
	return _c
}

// GetFileKeysSnapshot provides a mock function with given fields:
func (_m *MockStore) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetFileKeysSnapshot")
	}

	var r0 []*pb.ChangeFileKeys
	if rf, ok := ret.Get(0).(func() []*pb.ChangeFileKeys); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*pb.ChangeFileKeys)
		}
	}

	return r0
}

// MockStore_GetFileKeysSnapshot_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetFileKeysSnapshot'
type MockStore_GetFileKeysSnapshot_Call struct {
	*mock.Call
}

// GetFileKeysSnapshot is a helper method to define mock.On call
func (_e *MockStore_Expecter) GetFileKeysSnapshot() *MockStore_GetFileKeysSnapshot_Call {
	return &MockStore_GetFileKeysSnapshot_Call{Call: _e.mock.On("GetFileKeysSnapshot")}
}

func (_c *MockStore_GetFileKeysSnapshot_Call) Run(run func()) *MockStore_GetFileKeysSnapshot_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockStore_GetFileKeysSnapshot_Call) Return(_a0 []*pb.ChangeFileKeys) *MockStore_GetFileKeysSnapshot_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockStore_GetFileKeysSnapshot_Call) RunAndReturn(run func() []*pb.ChangeFileKeys) *MockStore_GetFileKeysSnapshot_Call {
	_c.Call.Return(run)
	return _c
}

// Heads provides a mock function with given fields:
func (_m *MockStore) Heads() []string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Heads")
	}

	var r0 []string
	if rf, ok := ret.Get(0).(func() []string); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]string)
		}
	}

	return r0
}

// MockStore_Heads_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Heads'
type MockStore_Heads_Call struct {
	*mock.Call
}

// Heads is a helper method to define mock.On call
func (_e *MockStore_Expecter) Heads() *MockStore_Heads_Call {
	return &MockStore_Heads_Call{Call: _e.mock.On("Heads")}
}

func (_c *MockStore_Heads_Call) Run(run func()) *MockStore_Heads_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockStore_Heads_Call) Return(_a0 []string) *MockStore_Heads_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockStore_Heads_Call) RunAndReturn(run func() []string) *MockStore_Heads_Call {
	_c.Call.Return(run)
	return _c
}

// Id provides a mock function with given fields:
func (_m *MockStore) Id() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Id")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// MockStore_Id_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Id'
type MockStore_Id_Call struct {
	*mock.Call
}

// Id is a helper method to define mock.On call
func (_e *MockStore_Expecter) Id() *MockStore_Id_Call {
	return &MockStore_Id_Call{Call: _e.mock.On("Id")}
}

func (_c *MockStore_Id_Call) Run(run func()) *MockStore_Id_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockStore_Id_Call) Return(_a0 string) *MockStore_Id_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockStore_Id_Call) RunAndReturn(run func() string) *MockStore_Id_Call {
	_c.Call.Return(run)
	return _c
}

// PushChange provides a mock function with given fields: params
func (_m *MockStore) PushChange(params source.PushChangeParams) (string, error) {
	ret := _m.Called(params)

	if len(ret) == 0 {
		panic("no return value specified for PushChange")
	}

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(source.PushChangeParams) (string, error)); ok {
		return rf(params)
	}
	if rf, ok := ret.Get(0).(func(source.PushChangeParams) string); ok {
		r0 = rf(params)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(source.PushChangeParams) error); ok {
		r1 = rf(params)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockStore_PushChange_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'PushChange'
type MockStore_PushChange_Call struct {
	*mock.Call
}

// PushChange is a helper method to define mock.On call
//   - params source.PushChangeParams
func (_e *MockStore_Expecter) PushChange(params interface{}) *MockStore_PushChange_Call {
	return &MockStore_PushChange_Call{Call: _e.mock.On("PushChange", params)}
}

func (_c *MockStore_PushChange_Call) Run(run func(params source.PushChangeParams)) *MockStore_PushChange_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(source.PushChangeParams))
	})
	return _c
}

func (_c *MockStore_PushChange_Call) Return(id string, err error) *MockStore_PushChange_Call {
	_c.Call.Return(id, err)
	return _c
}

func (_c *MockStore_PushChange_Call) RunAndReturn(run func(source.PushChangeParams) (string, error)) *MockStore_PushChange_Call {
	_c.Call.Return(run)
	return _c
}

// PushStoreChange provides a mock function with given fields: ctx, params
func (_m *MockStore) PushStoreChange(ctx context.Context, params source.PushStoreChangeParams) (string, error) {
	ret := _m.Called(ctx, params)

	if len(ret) == 0 {
		panic("no return value specified for PushStoreChange")
	}

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, source.PushStoreChangeParams) (string, error)); ok {
		return rf(ctx, params)
	}
	if rf, ok := ret.Get(0).(func(context.Context, source.PushStoreChangeParams) string); ok {
		r0 = rf(ctx, params)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(context.Context, source.PushStoreChangeParams) error); ok {
		r1 = rf(ctx, params)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockStore_PushStoreChange_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'PushStoreChange'
type MockStore_PushStoreChange_Call struct {
	*mock.Call
}

// PushStoreChange is a helper method to define mock.On call
//   - ctx context.Context
//   - params source.PushStoreChangeParams
func (_e *MockStore_Expecter) PushStoreChange(ctx interface{}, params interface{}) *MockStore_PushStoreChange_Call {
	return &MockStore_PushStoreChange_Call{Call: _e.mock.On("PushStoreChange", ctx, params)}
}

func (_c *MockStore_PushStoreChange_Call) Run(run func(ctx context.Context, params source.PushStoreChangeParams)) *MockStore_PushStoreChange_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(source.PushStoreChangeParams))
	})
	return _c
}

func (_c *MockStore_PushStoreChange_Call) Return(changeId string, err error) *MockStore_PushStoreChange_Call {
	_c.Call.Return(changeId, err)
	return _c
}

func (_c *MockStore_PushStoreChange_Call) RunAndReturn(run func(context.Context, source.PushStoreChangeParams) (string, error)) *MockStore_PushStoreChange_Call {
	_c.Call.Return(run)
	return _c
}

// ReadDoc provides a mock function with given fields: ctx, receiver, empty
func (_m *MockStore) ReadDoc(ctx context.Context, receiver source.ChangeReceiver, empty bool) (state.Doc, error) {
	ret := _m.Called(ctx, receiver, empty)

	if len(ret) == 0 {
		panic("no return value specified for ReadDoc")
	}

	var r0 state.Doc
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, source.ChangeReceiver, bool) (state.Doc, error)); ok {
		return rf(ctx, receiver, empty)
	}
	if rf, ok := ret.Get(0).(func(context.Context, source.ChangeReceiver, bool) state.Doc); ok {
		r0 = rf(ctx, receiver, empty)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(state.Doc)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, source.ChangeReceiver, bool) error); ok {
		r1 = rf(ctx, receiver, empty)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockStore_ReadDoc_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ReadDoc'
type MockStore_ReadDoc_Call struct {
	*mock.Call
}

// ReadDoc is a helper method to define mock.On call
//   - ctx context.Context
//   - receiver source.ChangeReceiver
//   - empty bool
func (_e *MockStore_Expecter) ReadDoc(ctx interface{}, receiver interface{}, empty interface{}) *MockStore_ReadDoc_Call {
	return &MockStore_ReadDoc_Call{Call: _e.mock.On("ReadDoc", ctx, receiver, empty)}
}

func (_c *MockStore_ReadDoc_Call) Run(run func(ctx context.Context, receiver source.ChangeReceiver, empty bool)) *MockStore_ReadDoc_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(source.ChangeReceiver), args[2].(bool))
	})
	return _c
}

func (_c *MockStore_ReadDoc_Call) Return(doc state.Doc, err error) *MockStore_ReadDoc_Call {
	_c.Call.Return(doc, err)
	return _c
}

func (_c *MockStore_ReadDoc_Call) RunAndReturn(run func(context.Context, source.ChangeReceiver, bool) (state.Doc, error)) *MockStore_ReadDoc_Call {
	_c.Call.Return(run)
	return _c
}

// ReadOnly provides a mock function with given fields:
func (_m *MockStore) ReadOnly() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for ReadOnly")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// MockStore_ReadOnly_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ReadOnly'
type MockStore_ReadOnly_Call struct {
	*mock.Call
}

// ReadOnly is a helper method to define mock.On call
func (_e *MockStore_Expecter) ReadOnly() *MockStore_ReadOnly_Call {
	return &MockStore_ReadOnly_Call{Call: _e.mock.On("ReadOnly")}
}

func (_c *MockStore_ReadOnly_Call) Run(run func()) *MockStore_ReadOnly_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockStore_ReadOnly_Call) Return(_a0 bool) *MockStore_ReadOnly_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockStore_ReadOnly_Call) RunAndReturn(run func() bool) *MockStore_ReadOnly_Call {
	_c.Call.Return(run)
	return _c
}

// ReadStoreDoc provides a mock function with given fields: ctx, stateStore, onUpdateHook
func (_m *MockStore) ReadStoreDoc(ctx context.Context, stateStore *storestate.StoreState, onUpdateHook func()) error {
	ret := _m.Called(ctx, stateStore, onUpdateHook)

	if len(ret) == 0 {
		panic("no return value specified for ReadStoreDoc")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *storestate.StoreState, func()) error); ok {
		r0 = rf(ctx, stateStore, onUpdateHook)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockStore_ReadStoreDoc_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ReadStoreDoc'
type MockStore_ReadStoreDoc_Call struct {
	*mock.Call
}

// ReadStoreDoc is a helper method to define mock.On call
//   - ctx context.Context
//   - stateStore *storestate.StoreState
//   - onUpdateHook func()
func (_e *MockStore_Expecter) ReadStoreDoc(ctx interface{}, stateStore interface{}, onUpdateHook interface{}) *MockStore_ReadStoreDoc_Call {
	return &MockStore_ReadStoreDoc_Call{Call: _e.mock.On("ReadStoreDoc", ctx, stateStore, onUpdateHook)}
}

func (_c *MockStore_ReadStoreDoc_Call) Run(run func(ctx context.Context, stateStore *storestate.StoreState, onUpdateHook func())) *MockStore_ReadStoreDoc_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(*storestate.StoreState), args[2].(func()))
	})
	return _c
}

func (_c *MockStore_ReadStoreDoc_Call) Return(err error) *MockStore_ReadStoreDoc_Call {
	_c.Call.Return(err)
	return _c
}

func (_c *MockStore_ReadStoreDoc_Call) RunAndReturn(run func(context.Context, *storestate.StoreState, func()) error) *MockStore_ReadStoreDoc_Call {
	_c.Call.Return(run)
	return _c
}

// SetPushChangeHook provides a mock function with given fields: onPushChange
func (_m *MockStore) SetPushChangeHook(onPushChange source.PushChangeHook) {
	_m.Called(onPushChange)
}

// MockStore_SetPushChangeHook_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'SetPushChangeHook'
type MockStore_SetPushChangeHook_Call struct {
	*mock.Call
}

// SetPushChangeHook is a helper method to define mock.On call
//   - onPushChange source.PushChangeHook
func (_e *MockStore_Expecter) SetPushChangeHook(onPushChange interface{}) *MockStore_SetPushChangeHook_Call {
	return &MockStore_SetPushChangeHook_Call{Call: _e.mock.On("SetPushChangeHook", onPushChange)}
}

func (_c *MockStore_SetPushChangeHook_Call) Run(run func(onPushChange source.PushChangeHook)) *MockStore_SetPushChangeHook_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(source.PushChangeHook))
	})
	return _c
}

func (_c *MockStore_SetPushChangeHook_Call) Return() *MockStore_SetPushChangeHook_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockStore_SetPushChangeHook_Call) RunAndReturn(run func(source.PushChangeHook)) *MockStore_SetPushChangeHook_Call {
	_c.Call.Return(run)
	return _c
}

// SpaceID provides a mock function with given fields:
func (_m *MockStore) SpaceID() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for SpaceID")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// MockStore_SpaceID_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'SpaceID'
type MockStore_SpaceID_Call struct {
	*mock.Call
}

// SpaceID is a helper method to define mock.On call
func (_e *MockStore_Expecter) SpaceID() *MockStore_SpaceID_Call {
	return &MockStore_SpaceID_Call{Call: _e.mock.On("SpaceID")}
}

func (_c *MockStore_SpaceID_Call) Run(run func()) *MockStore_SpaceID_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockStore_SpaceID_Call) Return(_a0 string) *MockStore_SpaceID_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockStore_SpaceID_Call) RunAndReturn(run func() string) *MockStore_SpaceID_Call {
	_c.Call.Return(run)
	return _c
}

// Type provides a mock function with given fields:
func (_m *MockStore) Type() smartblock.SmartBlockType {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Type")
	}

	var r0 smartblock.SmartBlockType
	if rf, ok := ret.Get(0).(func() smartblock.SmartBlockType); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(smartblock.SmartBlockType)
	}

	return r0
}

// MockStore_Type_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Type'
type MockStore_Type_Call struct {
	*mock.Call
}

// Type is a helper method to define mock.On call
func (_e *MockStore_Expecter) Type() *MockStore_Type_Call {
	return &MockStore_Type_Call{Call: _e.mock.On("Type")}
}

func (_c *MockStore_Type_Call) Run(run func()) *MockStore_Type_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockStore_Type_Call) Return(_a0 smartblock.SmartBlockType) *MockStore_Type_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockStore_Type_Call) RunAndReturn(run func() smartblock.SmartBlockType) *MockStore_Type_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockStore creates a new instance of MockStore. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockStore(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockStore {
	mock := &MockStore{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}