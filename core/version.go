package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/pb"
)

// Set by ldflags
var GitCommit, GitBranch, GitState, GitSummary, BuildDate string

func (mw *Middleware) VersionGet(req *pb.VersionGetRequest) *pb.VersionGetResponse {
	response := func(version, details string, code pb.VersionGetResponse_Error_Code, err error) *pb.VersionGetResponse {
		m := &pb.VersionGetResponse{Version: version, Error: &pb.VersionGetResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if len(GitSummary) == 0 {
		return response("", "", pb.VersionGetResponse_Error_VERSION_IS_EMPTY, nil)
	}

	details := fmt.Sprintf("build on %s from %s at #%s(%s)", BuildDate, GitCommit, GitBranch, GitState)

	return response(GitSummary, details, pb.VersionGetResponse_Error_NULL, nil)
}
