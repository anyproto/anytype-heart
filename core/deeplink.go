package core

import (
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) DeeplinkOpen(req *pb.RpcDeeplinkOpenRequest) *pb.RpcDeeplinkOpenResponse {
	response := func(code pb.RpcDeeplinkOpenResponseErrorCode, err error) *pb.RpcDeeplinkOpenResponse {
		m := &pb.RpcDeeplinkOpenResponse{Error: &pb.RpcDeeplinkOpenResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.OpenDeeplink(req)
	})
	if err != nil {
		return response(pb.RpcDeeplinkOpenResponseError_BAD_INPUT, err)
	}

	return response(pb.RpcDeeplinkOpenResponseError_NULL, nil)
}

func (mw *Middleware) DeeplinkCreateFromBlock(req *pb.RpcDeeplinkCreateFromBlockRequest) *pb.RpcDeeplinkCreateFromBlockResponse {
	response := func(deeplink string, code pb.RpcDeeplinkCreateFromBlockResponseErrorCode, err error) *pb.RpcDeeplinkCreateFromBlockResponse {
		m := &pb.RpcDeeplinkCreateFromBlockResponse{Deeplink: deeplink, Error: &pb.RpcDeeplinkCreateFromBlockResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	
	var deeplink string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		deeplink, err = bs.CreateDeeplinkFromBlock(req)
		return
	})
	if err != nil {
		return response("", pb.RpcDeeplinkCreateFromBlockResponseError_BAD_INPUT, err)
	}
	return response(deeplink, pb.RpcDeeplinkCreateFromBlockResponseError_NULL, nil)
}
