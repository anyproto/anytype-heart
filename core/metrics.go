package core

import (
	"context"
	"errors"

	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/initialparams"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

func (mw *Middleware) InitialSetParameters(cctx context.Context, req *pb.RpcInitialSetParametersRequest) *pb.RpcInitialSetParametersResponse {
	response := func(code pb.RpcInitialSetParametersResponseErrorCode, err error) *pb.RpcInitialSetParametersResponse {
		m := &pb.RpcInitialSetParametersResponse{Error: &pb.RpcInitialSetParametersResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}

		return m
	}
	if req.Version == "" {
		return response(pb.RpcInitialSetParametersResponseError_BAD_INPUT,
			errors.New("version is empty. Version must be in format: 1.0.0-optional-commit-hash-for-dev-builds"))
	}

	params, err := initialparams.Init(req)
	if err != nil {
		return response(pb.RpcInitialSetParametersResponseError_BAD_INPUT, err)
	}

	mw.applicationService.SetClientVersion(params.Platform, params.Version)
	metrics.Service.SetPlatform(params.Platform)
	metrics.Service.SetStartVersion(params.Version)
	metrics.Service.SetEnabled(params.SendTelemetry)
	logging.Init(params.LogLevel, params.SaveLogs)

	return response(pb.RpcInitialSetParametersResponseError_NULL, nil)
}
