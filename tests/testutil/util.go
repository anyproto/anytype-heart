package testutil

import (
	"context"
	"reflect"

	"github.com/anyproto/any-sync/app"
	mock2 "github.com/stretchr/testify/mock"
)

func PrepareMock(ctx context.Context, a *app.App, mock app.Component) app.Component {
	mockValue := reflect.ValueOf(mock)
	mockName := mockValue.Type().String()

	result := callChainOfMethods(mockValue, []methodNameAndParams{
		{
			name:   "EXPECT",
			params: nil,
		},
		{
			name:   "Name",
			params: nil,
		},
		{
			name:   "Return",
			params: []reflect.Value{reflect.ValueOf(mockName)},
		},
	})
	call := result[0]
	callAnyTimes(call)

	result = callChainOfMethods(mockValue, []methodNameAndParams{
		{
			name:   "EXPECT",
			params: nil,
		},
		{
			name:   "Init",
			params: []reflect.Value{reflect.ValueOf(a)},
		},
		{
			name:   "Return",
			params: []reflect.Value{reflect.Zero(reflect.TypeOf((*error)(nil)).Elem())},
		},
	})
	call = result[0]
	callAnyTimes(call)

	if _, ok := mock.(app.ComponentRunnable); ok {
		result = callChainOfMethods(mockValue, []methodNameAndParams{
			{
				name:   "EXPECT",
				params: nil,
			},
			{
				name:   "Run",
				params: []reflect.Value{reflect.ValueOf(ctx)},
			},
			{
				name:   "Return",
				params: []reflect.Value{reflect.Zero(reflect.TypeOf((*error)(nil)).Elem())},
			},
		})
		call = result[0]
		callAnyTimes(call)

		result = callChainOfMethods(mockValue, []methodNameAndParams{
			{
				name:   "EXPECT",
				params: nil,
			},
			{
				name:   "Close",
				params: []reflect.Value{reflect.ValueOf(mock2.Anything)},
			},
			{
				name:   "Return",
				params: []reflect.Value{reflect.Zero(reflect.TypeOf((*error)(nil)).Elem())},
			},
		})
		call = result[0]
		callAnyTimes(call)
	}
	return mock
}

type methodNameAndParams struct {
	name   string
	params []reflect.Value
}

func callChainOfMethods(target reflect.Value, callParams []methodNameAndParams) []reflect.Value {
	if len(callParams) == 0 {
		panic("callParams must not be empty")
	}
	if len(callParams) == 1 {
		callParam := callParams[0]
		method := target.MethodByName(callParam.name)
		return method.Call(callParam.params)
	}

	callParam := callParams[0]
	method := target.MethodByName(callParam.name)
	result := method.Call(callParam.params)
	return callChainOfMethods(result[0], callParams[1:])
}

func callAnyTimes(call reflect.Value) {
	if method := call.MethodByName("AnyTimes"); method.IsValid() { // From gomock
		method.Call(nil)
	} else if method := call.MethodByName("Maybe"); method.IsValid() { // From mockery
		method.Call(nil)
	} else {
		panic("mock method AnyTimes or Maybe not found")
	}
}
