package core

import (
	"context"
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/vcs"
)

func (mw *Middleware) AppGetVersion(cctx context.Context, req *pb.RpcAppGetVersionRequest) *pb.RpcAppGetVersionResponse {
	response := func(version, details string, code pb.RpcAppGetVersionResponseErrorCode, err error) *pb.RpcAppGetVersionResponse {
		m := &pb.RpcAppGetVersionResponse{Version: version, Details: details, Error: &pb.RpcAppGetVersionResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	buildDate, revision, modified, cgo := vcs.GetVCSInfo()
	if revision == "" {
		revision = "unknown"
	}

	desc := fmt.Sprintf("build on %s from %s", buildDate.Format("2006-01-02 15:04:05"), revision)
	if !cgo {
		desc += " (no-cgo)"
	}

	if modified {
		desc += " (dirty)"
	}

	return response(revision, desc, pb.RpcAppGetVersionResponseError_NULL, nil)
}
