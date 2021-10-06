package core

import (
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) ObjectAddWithShareLink(req *pb.RpcObjectAddWithShareLinkRequest) *pb.RpcObjectAddWithShareLinkResponse {
	response := func(code pb.RpcObjectAddWithShareLinkResponseErrorCode, err error) *pb.RpcObjectAddWithShareLinkResponse {
		m := &pb.RpcObjectAddWithShareLinkResponse{Error: &pb.RpcObjectAddWithShareLinkResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.ObjectAddWithShareLink(req)
	})
	if err != nil {
		return response(pb.RpcObjectAddWithShareLinkResponseError_BAD_INPUT, err)
	}

	return response(pb.RpcObjectAddWithShareLinkResponseError_NULL, nil)
}

func (mw *Middleware) ObjectShareByLink(req *pb.RpcObjectShareByLinkRequest) *pb.RpcObjectShareByLinkResponse {
	response := func(link string, code pb.RpcObjectShareByLinkResponseErrorCode, err error) *pb.RpcObjectShareByLinkResponse {
		m := &pb.RpcObjectShareByLinkResponse{Link: link, Error: &pb.RpcObjectShareByLinkResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	
	var link string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		link, err = bs.ObjectShareByLink(req)
		return
	})
	if err != nil {
		return response("", pb.RpcObjectShareByLinkResponseError_BAD_INPUT, err)
	}
	return response(link, pb.RpcObjectShareByLinkResponseError_NULL, nil)
}
