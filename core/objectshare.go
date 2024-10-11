package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) ObjectShareByLink(cctx context.Context, req *pb.RpcObjectShareByLinkRequest) *pb.RpcObjectShareByLinkResponse {
	response := func(link string, code pb.RpcObjectShareByLinkResponseErrorCode, err error) *pb.RpcObjectShareByLinkResponse {
		m := &pb.RpcObjectShareByLinkResponse{Link: link, Error: &pb.RpcObjectShareByLinkResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}

		return m
	}

	var link string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		link, err = bs.ObjectShareByLink(req)
		return
	})
	if err != nil {
		return response("", pb.RpcObjectShareByLinkResponseError_BAD_INPUT, err)
	}
	return response(link, pb.RpcObjectShareByLinkResponseError_NULL, nil)
}
