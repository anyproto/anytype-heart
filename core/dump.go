package core

import (
	"context"

	"github.com/anytypeio/go-anytype-middleware/core/dump"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
)

func (mw *Middleware) UserDataDump(ctx context.Context, req *pb.RpcUserDataDumpRequest) *pb.RpcUserDataDumpResponse {
	response := func(code pb.RpcUserDataDumpResponseErrorCode, err error) *pb.RpcUserDataDumpResponse {
		m := &pb.RpcUserDataDumpResponse{Error: &pb.RpcUserDataDumpResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	dumpService := mw.app.MustComponent(dump.Name).(*dump.Service)
	anytype := mw.app.MustComponent(core.CName).(*core.Anytype)

	profile, err := anytype.LocalProfile()
	if err != nil {
		return response(pb.RpcUserDataDumpResponseError_UNKNOWN_ERROR, err)
	}
	err = dumpService.Dump(req.Path, mw.mnemonic, profile, mw.rootPath)
	if err != nil {
		return response(pb.RpcUserDataDumpResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcUserDataDumpResponseError_NULL, nil)
}
