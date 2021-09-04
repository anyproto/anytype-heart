package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) VersionGet(req *pb.RpcVersionGetRequest) *pb.RpcVersionGetResponse {
	response := func(version, details string, code pb.RpcVersionGetResponseErrorCode, err error) *pb.RpcVersionGetResponse {
		m := &pb.RpcVersionGetResponse{Version: version, Details: details, Error: &pb.RpcVersionGetResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	return response(mw.app.Version(), mw.app.VersionDescription(), pb.RpcVersionGetResponseError_NULL, nil)
}
