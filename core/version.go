package core

import "github.com/anytypeio/go-anytype-middleware/pb"

// Set by ldflags
var GitCommit, GitBranch, GitState, GitSummary, BuildDate string

func GetVersion(req *pb.GetVersionRequest) *pb.GetVersionResponse {
	response := func(version string, code pb.GetVersionResponse_Error_Code, err error) *pb.GetVersionResponse {
		m := &pb.GetVersionResponse{Version: version, Error: &pb.GetVersionResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if len(GitSummary) == 0 {
		return response("", pb.GetVersionResponse_Error_VERSION_IS_EMPTY, nil)
	}

	return response(GitSummary, pb.GetVersionResponse_Error_NULL, nil)
}
