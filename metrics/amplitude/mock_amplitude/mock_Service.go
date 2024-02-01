// Code generated by mockery. DO NOT EDIT.

package mock_amplitude

import (
	amplitude "github.com/anyproto/anytype-heart/metrics/amplitude"
	mock "github.com/stretchr/testify/mock"
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

// SendEvents provides a mock function with given fields: amplEvents, info
func (_m *MockService) SendEvents(amplEvents []amplitude.Event, info amplitude.AppInfoProvider) error {
	ret := _m.Called(amplEvents, info)

	if len(ret) == 0 {
		panic("no return value specified for SendEvents")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func([]amplitude.Event, amplitude.AppInfoProvider) error); ok {
		r0 = rf(amplEvents, info)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockService_SendEvents_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'SendEvents'
type MockService_SendEvents_Call struct {
	*mock.Call
}

// SendEvents is a helper method to define mock.On call
//   - amplEvents []amplitude.Event
//   - info amplitude.AppInfoProvider
func (_e *MockService_Expecter) SendEvents(amplEvents interface{}, info interface{}) *MockService_SendEvents_Call {
	return &MockService_SendEvents_Call{Call: _e.mock.On("SendEvents", amplEvents, info)}
}

func (_c *MockService_SendEvents_Call) Run(run func(amplEvents []amplitude.Event, info amplitude.AppInfoProvider)) *MockService_SendEvents_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].([]amplitude.Event), args[1].(amplitude.AppInfoProvider))
	})
	return _c
}

func (_c *MockService_SendEvents_Call) Return(_a0 error) *MockService_SendEvents_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockService_SendEvents_Call) RunAndReturn(run func([]amplitude.Event, amplitude.AppInfoProvider) error) *MockService_SendEvents_Call {
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