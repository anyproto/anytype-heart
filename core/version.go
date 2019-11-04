package core

import "github.com/anytypeio/go-anytype-middleware/pb"

// Set by ldflags
var GitCommit, GitBranch, GitState, GitSummary, BuildDate string

func (mw *Middleware) VersionGet(req *pb.Rpc_Version_Get_Request) *pb.Rpc_Version_Get_Response {
	response := func(version string, code pb.Rpc_Version_Get_Response_Error_Code, err error) *pb.Rpc_Version_Get_Response {
		m := &pb.Rpc_Version_Get_Response{Version: version, Error: &pb.Rpc_Version_Get_Response_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if len(GitSummary) == 0 {
		return response("", pb.Rpc_Version_Get_Response_Error_VERSION_IS_EMPTY, nil)
	}

	return response(GitSummary, pb.Rpc_Version_Get_Response_Error_NULL, nil)
}
