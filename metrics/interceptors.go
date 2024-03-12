package metrics

import (
	"context"
	"strings"
	"time"

	"github.com/samber/lo"
	"google.golang.org/grpc"

	"github.com/anyproto/anytype-heart/util/debug"
	"github.com/anyproto/anytype-heart/util/reflection"
)

const (
	unexpectedErrorCode      = -1
	parsingErrorCode         = -2
	defaultUnaryWarningAfter = time.Second * 3
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

func SharedLongMethodsInterceptor(ctx context.Context, req any, methodName string, actualCall func(ctx context.Context, req any) (any, error)) (any, error) {
	doneCh := make(chan struct{})
	start := time.Now()

	l := log.With("method", methodName)

	go func() {
		select {
		case <-doneCh:
		case <-time.After(defaultUnaryWarningAfter):
			l.With("in_progress", true).With("goroutines", debug.StackCompact(true)).With("total", defaultUnaryWarningAfter.Milliseconds()).Warnf("grpc unary request is taking too long")
		}
	}()
	ctx = context.WithValue(ctx, CtxKeyRPC, methodName)
	resp, err := actualCall(ctx, req)
	close(doneCh)
	if time.Since(start) > defaultUnaryWarningAfter {
		l.With("error", err).With("in_progress", false).With("total", time.Since(start).Milliseconds()).Warnf("grpc unary request took too long")
	}
	return resp, err
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
