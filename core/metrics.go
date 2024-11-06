package core

import (
	"context"
	"errors"

	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
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
	mw.applicationService.SetClientVersion(req.Platform, req.Version)

	metrics.Service.SetPlatform(req.Platform)
	metrics.Service.SetStartVersion(req.Version)
	metrics.Service.SetEnabled(!req.DoNotSendTelemetry)
	logging.Init(req.Workdir, req.LogLevel, !req.DoNotSendLogs, !req.DoNotSaveLogs)

	return response(pb.RpcInitialSetParametersResponseError_NULL, nil)
}
