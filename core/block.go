package core

import (
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) BlockCreate(req *pb.RpcBlockCreateRequest) *pb.RpcBlockCreateResponse {
	response := func(code pb.RpcBlockCreateResponseErrorCode, id string, err error) *pb.RpcBlockCreateResponse {
		m := &pb.RpcBlockCreateResponse{Error: &pb.RpcBlockCreateResponseError{Code: code}, BlockId: id}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	id, err := mw.blockService.CreateBlock(*req)
	if err != nil {
		response(pb.RpcBlockCreateResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockCreateResponseError_NULL, id, nil)
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
	if err := mw.blockService.CloseBlock(req.Id); err != nil {
		return response(pb.RpcBlockCloseResponseError_UNKNOWN_ERROR, err)
	}
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

func (mw *Middleware) BlockContentUpload(req *pb.RpcBlockActionContentUploadRequest) *pb.RpcBlockActionContentUploadResponse {
	response := func(code pb.RpcBlockActionContentUploadResponseErrorCode, err error) *pb.RpcBlockActionContentUploadResponse {
		m := &pb.RpcBlockActionContentUploadResponse{Error: &pb.RpcBlockActionContentUploadResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockActionContentUploadResponseError_NULL, nil)
}

func (mw *Middleware) BlockContentDownload(req *pb.RpcBlockActionContentDownloadRequest) *pb.RpcBlockActionContentDownloadResponse {
	response := func(code pb.RpcBlockActionContentDownloadResponseErrorCode, err error) *pb.RpcBlockActionContentDownloadResponse {
		m := &pb.RpcBlockActionContentDownloadResponse{Error: &pb.RpcBlockActionContentDownloadResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockActionContentDownloadResponseError_NULL, nil)
}

func (mw *Middleware) BlockMarkSet(req *pb.RpcBlockActionMarkSetRequest) *pb.RpcBlockActionMarkSetResponse {
	response := func(code pb.RpcBlockActionMarkSetResponseErrorCode, err error) *pb.RpcBlockActionMarkSetResponse {
		m := &pb.RpcBlockActionMarkSetResponse{Error: &pb.RpcBlockActionMarkSetResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockActionMarkSetResponseError_NULL, nil)
}

func (mw *Middleware) BlockMarksGet(req *pb.RpcBlockActionMarksGetRequest) *pb.RpcBlockActionMarksGetResponse {
	response := func(code pb.RpcBlockActionMarksGetResponseErrorCode, err error) *pb.RpcBlockActionMarksGetResponse {
		m := &pb.RpcBlockActionMarksGetResponse{Error: &pb.RpcBlockActionMarksGetResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockActionMarksGetResponseError_NULL, nil)
}

func (mw *Middleware) BlocksDrop(req *pb.RpcBlockActionBlocksDropRequest) *pb.RpcBlockActionBlocksDropResponse {
	response := func(code pb.RpcBlockActionBlocksDropResponseErrorCode, err error) *pb.RpcBlockActionBlocksDropResponse {
		m := &pb.RpcBlockActionBlocksDropResponse{Error: &pb.RpcBlockActionBlocksDropResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockActionBlocksDropResponseError_NULL, nil)
}

func (mw *Middleware) switchAccount(accountId string) {
	if mw.blockService != nil {
		mw.blockService.Close()
	}
	mw.blockService = block.NewService(accountId, mw.Anytype, mw.SendEvent)
}
