package lib

import "github.com/anytypeio/go-anytype-middleware/pb"
import "github.com/gogo/protobuf/proto"

// Set by ldflags
var GitCommit, GitBranch, GitState, GitSummary, BuildDate string

// Version is the current application's version literal
const Version = "0.0.1"

func GetVersion(b []byte) []byte {
	response := func(code pb.GetVersionResponse_Error_Code, err error) []byte {
		m := &pb.GetVersionResponse{Version: Version}
		if code != pb.GetVersionResponse_Error_NULL {
			m.Error = &pb.GetVersionResponse_Error{Code: code}
			if err != nil {
				m.Error.Description = err.Error()
			}
		}

		return Marshal(m)
	}
	var q pb.GetVersionRequest
	err := proto.Unmarshal(b, &q)
	if err != nil {
		return response(pb.GetVersionResponse_Error_BAD_INPUT, err)
	}

	if len(Version) == 0 {
		return response(pb.GetVersionResponse_Error_VERSION_IS_EMPTY, nil)
	}

	return response(pb.GetVersionResponse_Error_NULL, nil)
}
