package core

import (
	"context"
	"fmt"
	"io/ioutil"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
)

func (mw *Middleware) ImageGetBlob(req *pb.RpcIpfsImageGetBlobRequest) *pb.RpcIpfsImageGetBlobResponse {
	mw.m.RLock()
	defer mw.m.RUnlock()
	response := func(blob []byte, code pb.RpcIpfsImageGetBlobResponseErrorCode, err error) *pb.RpcIpfsImageGetBlobResponse {
		m := &pb.RpcIpfsImageGetBlobResponse{Blob: blob, Error: &pb.RpcIpfsImageGetBlobResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if mw.app == nil {
		response(nil, pb.RpcIpfsImageGetBlobResponseError_NODE_NOT_STARTED, fmt.Errorf("anytype is nil"))
	}

	at := mw.app.MustComponent(core.CName).(core.Service)

	if !at.IsStarted() {
		response(nil, pb.RpcIpfsImageGetBlobResponseError_NODE_NOT_STARTED, fmt.Errorf("anytype node not started"))
	}

	image, err := at.ImageByHash(context.TODO(), req.GetHash())
	if err != nil {
		if err == core.ErrFileNotFound {
			return response(nil, pb.RpcIpfsImageGetBlobResponseError_NOT_FOUND, err)
		}

		return response(nil, pb.RpcIpfsImageGetBlobResponseError_UNKNOWN_ERROR, err)
	}
	file, err := image.GetFileForWidth(context.TODO(), int(req.WantWidth))
	if err != nil {
		if err == core.ErrFileNotFound {
			return response(nil, pb.RpcIpfsImageGetBlobResponseError_NOT_FOUND, err)
		}

		return response(nil, pb.RpcIpfsImageGetBlobResponseError_UNKNOWN_ERROR, err)
	}

	rd, err := file.Reader()
	if err != nil {
		if err == core.ErrFileNotFound {
			return response(nil, pb.RpcIpfsImageGetBlobResponseError_NOT_FOUND, err)
		}

		return response(nil, pb.RpcIpfsImageGetBlobResponseError_UNKNOWN_ERROR, err)
	}
	data, err := ioutil.ReadAll(rd)
	if err != nil {
		if err == core.ErrFileNotFound {
			return response(nil, pb.RpcIpfsImageGetBlobResponseError_NOT_FOUND, err)
		}

		return response(nil, pb.RpcIpfsImageGetBlobResponseError_UNKNOWN_ERROR, err)
	}
	return response(data, pb.RpcIpfsImageGetBlobResponseError_NULL, nil)
}
