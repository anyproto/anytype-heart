package core

import (
	"context"

	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) MetricsSetParameters(cctx context.Context, req *pb.RpcMetricsSetParametersRequest) *pb.RpcMetricsSetParametersResponse {
	response := func(code pb.RpcMetricsSetParametersResponseErrorCode, err error) *pb.RpcMetricsSetParametersResponse {
		m := &pb.RpcMetricsSetParametersResponse{Error: &pb.RpcMetricsSetParametersResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	metrics.SharedClient.SetPlatform(req.Platform)

	return response(pb.RpcMetricsSetParametersResponseError_NULL, nil)
}
