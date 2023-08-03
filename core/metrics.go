package core

import (
	"context"
	"errors"

	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) MetricsSetParameters(cctx context.Context, req *pb.RpcMetricsSetParametersRequest) *pb.RpcMetricsSetParametersResponse {
	response := func(code pb.RpcMetricsSetParametersResponseErrorCode, err error) *pb.RpcMetricsSetParametersResponse {
		m := &pb.RpcMetricsSetParametersResponse{Error: &pb.RpcMetricsSetParametersResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	if req.Version == "" {
		return response(pb.RpcMetricsSetParametersResponseError_BAD_INPUT,
			errors.New("version is empty. Version must be in format: 1.0.0-optional-commit-hash-for-dev-builds"))
	}
	mw.applicationService.SetClientVersion(req.Platform, req.Version)

	metrics.SharedClient.SetPlatform(req.Platform)

	return response(pb.RpcMetricsSetParametersResponseError_NULL, nil)
}
