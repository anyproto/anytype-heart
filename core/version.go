package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/pb"
)

// Set by ldflags
var GitCommit, GitBranch, GitState, GitSummary, BuildDate string

func (mw *Middleware) VersionGet(req *pb.RpcVersionGetRequest) *pb.RpcVersionGetResponse {
	response := func(version, details string, code pb.RpcVersionGetResponseErrorCode, err error) *pb.RpcVersionGetResponse {
		m := &pb.RpcVersionGetResponse{Version: version, Details: details, Error: &pb.RpcVersionGetResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if len(GitSummary) == 0 {
		return response("", "", pb.RpcVersionGetResponseError_VERSION_IS_EMPTY, nil)
	}

	details := fmt.Sprintf("build on %s from %s at #%s(%s)", BuildDate, GitBranch, GitCommit, GitState)

	return response(GitSummary, details, pb.RpcVersionGetResponseError_NULL, nil)
}
