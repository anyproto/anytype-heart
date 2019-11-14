package core

import (
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) BlockCreate(req *pb.RpcBlockCreateRequest) *pb.RpcBlockCreateResponse {
	response := func(code pb.RpcBlockCreateResponseErrorCode, err error) *pb.RpcBlockCreateResponse {
		m := &pb.RpcBlockCreateResponse{Error: &pb.RpcBlockCreateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	/*block := &model.Block{} // TODO

	m := &pb.Event{Message: &pb.EventBlockCreate{&pb.RpcBlockCreate{Block: block}}}

	if mw.SendEvent != nil {
		mw.SendEvent(m)
	}*/

	return response(pb.RpcBlockCreateResponseError_NULL, nil)
}

func (mw *Middleware) BlockOpen(req *pb.RpcBlockOpenRequest) *pb.RpcBlockOpenResponse {
	response := func(code pb.RpcBlockOpenResponseErrorCode, err error) *pb.RpcBlockOpenResponse {
		m := &pb.RpcBlockOpenResponse{Error: &pb.RpcBlockOpenResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if err := mw.blockService.OpenBlock(req.Id); err != nil {
		switch err {
		case block.ErrBlockNotFound:
			return response(pb.RpcBlockOpenResponseError_BAD_INPUT, err)
		}
		return response(pb.RpcBlockOpenResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcBlockOpenResponseError_NULL, nil)
}

func (mw *Middleware) BlockClose(req *pb.RpcBlockCloseRequest) *pb.RpcBlockCloseResponse {
	response := func(code pb.RpcBlockCloseResponseErrorCode, err error) *pb.RpcBlockCloseResponse {
		m := &pb.RpcBlockCloseResponse{Error: &pb.RpcBlockCloseResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockCloseResponseError_NULL, nil)
}

func (mw *Middleware) BlockUpdate(req *pb.RpcBlockUpdateRequest) *pb.RpcBlockUpdateResponse {
	response := func(code pb.RpcBlockUpdateResponseErrorCode, err error) *pb.RpcBlockUpdateResponse {
		m := &pb.RpcBlockUpdateResponse{Error: &pb.RpcBlockUpdateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	/*
		 changes := &pb.RpcBlockChanges{} // TODO

		 m := &pb.Event{Message: &pb.EventBlockUpdate{&pb.RpcBlockUpdate{changes}}}

		if mw.SendEvent != nil {
			mw.SendEvent(m)
		}*/

	return response(pb.RpcBlockUpdateResponseError_NULL, nil)
}

func (mw *Middleware) switchAccount(accountId string) {
	if mw.blockService != nil {
		mw.blockService.Close()
	}
	mw.blockService = block.NewService(accountId, mw.Anytype, mw.SendEvent)
}
