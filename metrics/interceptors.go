package metrics

import (
	"bytes"
	"context"
	"strings"
	"time"

	"github.com/samber/lo"
	"go.uber.org/atomic"
	"google.golang.org/grpc"

	"github.com/anyproto/anytype-heart/util/debug"
	"github.com/anyproto/anytype-heart/util/reflection"
)

const (
	unexpectedErrorCode = -1
	parsingErrorCode    = -2
)

var (
	// every duration will be added to the previous ones
	UnaryTraceCollections = []time.Duration{time.Second * 3, time.Second * 7, time.Second * 10, time.Second * 30, time.Second * 50} // 3, 10, 20, 50, 100
	// UnaryWarningInProgressIndex specify the index of UnaryTraceCollections when we will send the warning log in-progress without waiting for the command to finish
	UnaryWarningInProgressIndex = 1
)
var excludedMethods = []string{
	"BlockSetCarriage",
	"BlockTextSetText",
	"ObjectSearchSubscribe",
	"ObjectSearchUnsubscribe",
	"ObjectSubscribeIds",
	"MetricsSetParameters",
	"AppSetDeviceState",
}

func UnaryTraceInterceptor(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	return SharedTraceInterceptor(ctx, req, extractMethodName(info.FullMethod), handler)
}

func extractMethodName(info string) string {
	// it looks like that, we need the last part /anytype.ClientCommands/FileNodeUsage
	return strings.Split(info, "/")[2]
}

func SharedTraceInterceptor(ctx context.Context, req any, methodName string, actualCall func(ctx context.Context, req any) (any, error)) (any, error) {
	start := time.Now().UnixMilli()
	resp, err := actualCall(ctx, req)
	delta := time.Now().UnixMilli() - start
	SendMethodEvent(methodName, err, resp, delta)
	return resp, err
}

func LongMethodsInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	return SharedLongMethodsInterceptor(ctx, req, extractMethodName(info.FullMethod), handler)
}

var excludedLongExecutionMethods = []string{
	"DebugRunProfiler",
}

func stackTraceHasMethod(method string, stackTrace []byte) bool {
	return bytes.Contains(stackTrace, []byte("core.(*Middleware)."+method+"("))
}

func SharedLongMethodsInterceptor(ctx context.Context, req any, methodName string, actualCall func(ctx context.Context, req any) (any, error)) (any, error) {
	if lo.Contains(excludedLongExecutionMethods, methodName) {
		return actualCall(ctx, req)
	}
	doneCh := make(chan struct{})
	start := time.Now()

	lastTrace := atomic.NewString("")
	l := log.With("method", methodName)
	go func() {
	loop:
		for i, duration := range UnaryTraceCollections {
			select {
			case <-doneCh:
				break loop
			case <-time.After(duration):
				trace := debug.Stack(true)
				// double check, because we can have a race and the stack trace can be taken after the method is already finished
				if stackTraceHasMethod(methodName, trace) {
					lastTrace.Store(string(trace))
					if i == UnaryWarningInProgressIndex {
						traceCompressed := debug.CompressBytes(trace)
						l.With("ver", 2).With("in_progress", true).With("goroutines", traceCompressed).With("total", time.Since(start).Milliseconds()).Warnf("grpc unary request is taking too long")
					}
				}
			}
		}
	}()
	ctx = context.WithValue(ctx, CtxKeyRPC, methodName)
	resp, err := actualCall(ctx, req)
	close(doneCh)
	if len(UnaryTraceCollections) > 0 && time.Since(start) > UnaryTraceCollections[0] {
		// todo: save long stack trace to files
		lastTraceB := debug.CompressBytes([]byte(lastTrace.String()))
		l.With("ver", 2).With("error", err).With("in_progress", false).With("goroutines", lastTraceB).With("total", time.Since(start).Milliseconds()).Warnf("grpc unary request took too long")
		Service.Send(
			&LongMethodEvent{
				methodName: methodName,
				middleTime: elapsed.Milliseconds(),
				stack:      lastTrace.Load(),
			},
		)
	}
	return resp, err
}

func stackTraceHasMethod(method string, stackTrace []byte) bool {
	return bytes.Contains(stackTrace, []byte("core.(*Middleware)."+method+"("))
}

func SendMethodEvent(method string, err error, resp any, delta int64) {
	if !lo.Contains(excludedMethods, method) {
		if err != nil {
			sendUnexpectedError(method, err.Error())
		}
		errorCode, description, err := reflection.GetError(resp)
		if err != nil {
			sendErrorParsingError(method)
		}
		if errorCode > 0 {
			sendExpectedError(method, errorCode, description)
		}
		sendSuccess(method, delta)
	}
}

func sendSuccess(method string, delta int64) {
	Service.Send(
		&MethodEvent{
			methodName: method,
			middleTime: delta,
		},
	)
}

func sendExpectedError(method string, code int64, description string) {
	Service.Send(
		&MethodEvent{
			methodName:  method,
			errorCode:   code,
			description: description,
		},
	)
}

func sendErrorParsingError(method string) {
	Service.Send(
		&MethodEvent{
			methodName: method,
			errorCode:  parsingErrorCode,
		},
	)
}

func sendUnexpectedError(method string, description string) {
	Service.Send(
		&MethodEvent{
			methodName:  method,
			errorCode:   unexpectedErrorCode,
			description: description,
		},
	)
}
