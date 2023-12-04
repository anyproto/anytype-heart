package metrics

import (
	"context"
	"strings"
	"time"

	"github.com/samber/lo"
	"google.golang.org/grpc"

	"github.com/anyproto/anytype-heart/util/reflection"
)

const (
	BlockSetCarriage      = "BlockSetCarriage"
	BlockTextSetText      = "BlockTextSetText"
	ObjectSearchSubscribe = "ObjectSearchSubscribe"
	unexpectedErrorCode   = -1
	parsingErrorCode      = -2
)

var excludedMethods = []string{
	BlockSetCarriage,
	BlockTextSetText,
	ObjectSearchSubscribe,
}

func UnaryTraceInterceptor() func(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now().UnixMilli()
		resp, err := handler(ctx, req)
		delta := time.Now().UnixMilli() - start

		// it looks like that, we need the last part /anytype.ClientCommands/FileNodeUsage
		method := strings.Split(info.FullMethod, "/")[2]
		SendMethodEvent(method, err, resp, delta)

		return resp, err
	}
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
