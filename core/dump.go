package core

import (
	"context"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"google.golang.org/grpc"
)

func (mw *Middleware) UserDataDump(ctx context.Context, req *pb.RpcUserDataDumpRequest, opts ...grpc.CallOption) (*pb.RpcUserDataDumpResponse, error) {
	response := func(code pb.RpcBlockCreateWidgetResponseErrorCode, id string, err error) *pb.RpcBlockCreateWidgetResponse {
		m := &pb.RpcBlockCreateWidgetResponse{Error: &pb.RpcBlockCreateWidgetResponseError{Code: code}, BlockId: id}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	mw.app.MustComponent()
	if err != nil {
		return response(pb.RpcBlockCreateWidgetResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockCreateWidgetResponseError_NULL, id, nil)
}
