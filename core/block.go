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
		return response(pb.RpcBlockCreateResponseError_UNKNOWN_ERROR, "", err)
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

	if err := mw.blockService.OpenBlock(req.BlockId); err != nil {
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
	if err := mw.blockService.CloseBlock(req.BlockId); err != nil {
		return response(pb.RpcBlockCloseResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockCloseResponseError_NULL, nil)
}

func (mw *Middleware) BlockUpload(req *pb.RpcBlockUploadRequest) *pb.RpcBlockUploadResponse {
	response := func(code pb.RpcBlockUploadResponseErrorCode, err error) *pb.RpcBlockUploadResponse {
		m := &pb.RpcBlockUploadResponse{Error: &pb.RpcBlockUploadResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockUploadResponseError_NULL, nil)
}

func (mw *Middleware) BlockUnlink(req *pb.RpcBlockUnlinkRequest) *pb.RpcBlockUnlinkResponse {
	response := func(code pb.RpcBlockUnlinkResponseErrorCode, err error) *pb.RpcBlockUnlinkResponse {
		m := &pb.RpcBlockUnlinkResponse{Error: &pb.RpcBlockUnlinkResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockUnlinkResponseError_NULL, nil)
}

func (mw *Middleware) BlockDownload(req *pb.RpcBlockDownloadRequest) *pb.RpcBlockDownloadResponse {
	response := func(code pb.RpcBlockDownloadResponseErrorCode, err error) *pb.RpcBlockDownloadResponse {
		m := &pb.RpcBlockDownloadResponse{Error: &pb.RpcBlockDownloadResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockDownloadResponseError_NULL, nil)
}

func (mw *Middleware) BlockGetMarks(req *pb.RpcBlockGetMarksRequest) *pb.RpcBlockGetMarksResponse {
	response := func(code pb.RpcBlockGetMarksResponseErrorCode, err error) *pb.RpcBlockGetMarksResponse {
		m := &pb.RpcBlockGetMarksResponse{Error: &pb.RpcBlockGetMarksResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockGetMarksResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetFields(req *pb.RpcBlockSetFieldsRequest) *pb.RpcBlockSetFieldsResponse {
	response := func(code pb.RpcBlockSetFieldsResponseErrorCode, err error) *pb.RpcBlockSetFieldsResponse {
		m := &pb.RpcBlockSetFieldsResponse{Error: &pb.RpcBlockSetFieldsResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockSetFieldsResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetPermissions(req *pb.RpcBlockSetPermissionsRequest) *pb.RpcBlockSetPermissionsResponse {
	response := func(code pb.RpcBlockSetPermissionsResponseErrorCode, err error) *pb.RpcBlockSetPermissionsResponse {
		m := &pb.RpcBlockSetPermissionsResponse{Error: &pb.RpcBlockSetPermissionsResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockSetPermissionsResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetIsArchived(req *pb.RpcBlockSetIsArchivedRequest) *pb.RpcBlockSetIsArchivedResponse {
	response := func(code pb.RpcBlockSetIsArchivedResponseErrorCode, err error) *pb.RpcBlockSetIsArchivedResponse {
		m := &pb.RpcBlockSetIsArchivedResponse{Error: &pb.RpcBlockSetIsArchivedResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockSetIsArchivedResponseError_NULL, nil)
}

func (mw *Middleware) BlockListMove(req *pb.RpcBlockListMoveRequest) *pb.RpcBlockListMoveResponse {
	response := func(code pb.RpcBlockListMoveResponseErrorCode, err error) *pb.RpcBlockListMoveResponse {
		m := &pb.RpcBlockListMoveResponse{Error: &pb.RpcBlockListMoveResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockListMoveResponseError_NULL, nil)
}

func (mw *Middleware) BlockListSetTextStyle(req *pb.RpcBlockListSetTextStyleRequest) *pb.RpcBlockListSetTextStyleResponse {
	response := func(code pb.RpcBlockListSetTextStyleResponseErrorCode, err error) *pb.RpcBlockListSetTextStyleResponse {
		m := &pb.RpcBlockListSetTextStyleResponse{Error: &pb.RpcBlockListSetTextStyleResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockListSetTextStyleResponseError_NULL, nil)
}

func (mw *Middleware) BlockListSetTextMarker(req *pb.RpcBlockListSetTextMarkerRequest) *pb.RpcBlockListSetTextMarkerResponse {
	response := func(code pb.RpcBlockListSetTextMarkerResponseErrorCode, err error) *pb.RpcBlockListSetTextMarkerResponse {
		m := &pb.RpcBlockListSetTextMarkerResponse{Error: &pb.RpcBlockListSetTextMarkerResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockListSetTextMarkerResponseError_NULL, nil)
}

func (mw *Middleware) BlockListSetTextCheckable(req *pb.RpcBlockListSetTextCheckableRequest) *pb.RpcBlockListSetTextCheckableResponse {
	response := func(code pb.RpcBlockListSetTextCheckableResponseErrorCode, err error) *pb.RpcBlockListSetTextCheckableResponse {
		m := &pb.RpcBlockListSetTextCheckableResponse{Error: &pb.RpcBlockListSetTextCheckableResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockListSetTextCheckableResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetTextText(req *pb.RpcBlockSetTextTextRequest) *pb.RpcBlockSetTextTextResponse {
	response := func(code pb.RpcBlockSetTextTextResponseErrorCode, err error) *pb.RpcBlockSetTextTextResponse {
		m := &pb.RpcBlockSetTextTextResponse{Error: &pb.RpcBlockSetTextTextResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockSetTextTextResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetTextStyle(req *pb.RpcBlockSetTextStyleRequest) *pb.RpcBlockSetTextStyleResponse {
	response := func(code pb.RpcBlockSetTextStyleResponseErrorCode, err error) *pb.RpcBlockSetTextStyleResponse {
		m := &pb.RpcBlockSetTextStyleResponse{Error: &pb.RpcBlockSetTextStyleResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockSetTextStyleResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetTextToggleable(req *pb.RpcBlockSetTextToggleableRequest) *pb.RpcBlockSetTextToggleableResponse {
	response := func(code pb.RpcBlockSetTextToggleableResponseErrorCode, err error) *pb.RpcBlockSetTextToggleableResponse {
		m := &pb.RpcBlockSetTextToggleableResponse{Error: &pb.RpcBlockSetTextToggleableResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockSetTextToggleableResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetTextMarker(req *pb.RpcBlockSetTextMarkerRequest) *pb.RpcBlockSetTextMarkerResponse {
	response := func(code pb.RpcBlockSetTextMarkerResponseErrorCode, err error) *pb.RpcBlockSetTextMarkerResponse {
		m := &pb.RpcBlockSetTextMarkerResponse{Error: &pb.RpcBlockSetTextMarkerResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockSetTextMarkerResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetTextCheckable(req *pb.RpcBlockSetTextCheckableRequest) *pb.RpcBlockSetTextCheckableResponse {
	response := func(code pb.RpcBlockSetTextCheckableResponseErrorCode, err error) *pb.RpcBlockSetTextCheckableResponse {
		m := &pb.RpcBlockSetTextCheckableResponse{Error: &pb.RpcBlockSetTextCheckableResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockSetTextCheckableResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetTextCheck(req *pb.RpcBlockSetTextCheckRequest) *pb.RpcBlockSetTextCheckResponse {
	response := func(code pb.RpcBlockSetTextCheckResponseErrorCode, err error) *pb.RpcBlockSetTextCheckResponse {
		m := &pb.RpcBlockSetTextCheckResponse{Error: &pb.RpcBlockSetTextCheckResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockSetTextCheckResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetFileName(req *pb.RpcBlockSetFileNameRequest) *pb.RpcBlockSetFileNameResponse {
	response := func(code pb.RpcBlockSetFileNameResponseErrorCode, err error) *pb.RpcBlockSetFileNameResponse {
		m := &pb.RpcBlockSetFileNameResponse{Error: &pb.RpcBlockSetFileNameResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockSetFileNameResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetImageName(req *pb.RpcBlockSetImageNameRequest) *pb.RpcBlockSetImageNameResponse {
	response := func(code pb.RpcBlockSetImageNameResponseErrorCode, err error) *pb.RpcBlockSetImageNameResponse {
		m := &pb.RpcBlockSetImageNameResponse{Error: &pb.RpcBlockSetImageNameResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockSetImageNameResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetImageWidth(req *pb.RpcBlockSetImageWidthRequest) *pb.RpcBlockSetImageWidthResponse {
	response := func(code pb.RpcBlockSetImageWidthResponseErrorCode, err error) *pb.RpcBlockSetImageWidthResponse {
		m := &pb.RpcBlockSetImageWidthResponse{Error: &pb.RpcBlockSetImageWidthResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockSetImageWidthResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetVideoName(req *pb.RpcBlockSetVideoNameRequest) *pb.RpcBlockSetVideoNameResponse {
	response := func(code pb.RpcBlockSetVideoNameResponseErrorCode, err error) *pb.RpcBlockSetVideoNameResponse {
		m := &pb.RpcBlockSetVideoNameResponse{Error: &pb.RpcBlockSetVideoNameResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockSetVideoNameResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetVideoWidth(req *pb.RpcBlockSetVideoWidthRequest) *pb.RpcBlockSetVideoWidthResponse {
	response := func(code pb.RpcBlockSetVideoWidthResponseErrorCode, err error) *pb.RpcBlockSetVideoWidthResponse {
		m := &pb.RpcBlockSetVideoWidthResponse{Error: &pb.RpcBlockSetVideoWidthResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockSetVideoWidthResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetIconName(req *pb.RpcBlockSetIconNameRequest) *pb.RpcBlockSetIconNameResponse {
	response := func(code pb.RpcBlockSetIconNameResponseErrorCode, err error) *pb.RpcBlockSetIconNameResponse {
		m := &pb.RpcBlockSetIconNameResponse{Error: &pb.RpcBlockSetIconNameResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockSetIconNameResponseError_NULL, nil)
}

func (mw *Middleware) switchAccount(accountId string) {
	if mw.blockService != nil {
		mw.blockService.Close()
	}
	mw.blockService = block.NewService(accountId, mw.Anytype, mw.SendEvent)
}
