package core

import (
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
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
	if err := mw.blockService.UnlinkBlock(*req); err != nil {
		return response(pb.RpcBlockUnlinkResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockUnlinkResponseError_NULL, nil)
}

func (mw *Middleware) BlockDuplicate(req *pb.RpcBlockDuplicateRequest) *pb.RpcBlockDuplicateResponse {
	response := func(id string, code pb.RpcBlockDuplicateResponseErrorCode, err error) *pb.RpcBlockDuplicateResponse {
		m := &pb.RpcBlockDuplicateResponse{BlockId: id, Error: &pb.RpcBlockDuplicateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	id, err := mw.blockService.DuplicateBlock(*req)
	if err != nil {
		return response("", pb.RpcBlockDuplicateResponseError_UNKNOWN_ERROR, err)
	}
	return response(id, pb.RpcBlockDuplicateResponseError_NULL, nil)
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
	if err := mw.blockService.SetFields(*req); err != nil {
		return response(pb.RpcBlockSetFieldsResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockSetFieldsResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetRestrictions(req *pb.RpcBlockSetRestrictionsRequest) *pb.RpcBlockSetRestrictionsResponse {
	response := func(code pb.RpcBlockSetRestrictionsResponseErrorCode, err error) *pb.RpcBlockSetRestrictionsResponse {
		m := &pb.RpcBlockSetRestrictionsResponse{Error: &pb.RpcBlockSetRestrictionsResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockSetRestrictionsResponseError_NULL, nil)
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

func (mw *Middleware) BlockReplace(req *pb.RpcBlockReplaceRequest) *pb.RpcBlockReplaceResponse {
	response := func(code pb.RpcBlockReplaceResponseErrorCode, err error) *pb.RpcBlockReplaceResponse {
		m := &pb.RpcBlockReplaceResponse{Error: &pb.RpcBlockReplaceResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	if err := mw.blockService.ReplaceBlock(*req); err != nil {
		return response(pb.RpcBlockReplaceResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockReplaceResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetTextColor(req *pb.RpcBlockSetTextColorRequest) *pb.RpcBlockSetTextColorResponse {
	response := func(code pb.RpcBlockSetTextColorResponseErrorCode, err error) *pb.RpcBlockSetTextColorResponse {
		m := &pb.RpcBlockSetTextColorResponse{Error: &pb.RpcBlockSetTextColorResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	if err := mw.blockService.SetTextColor(*req); err != nil {
		return response(pb.RpcBlockSetTextColorResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockSetTextColorResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetTextBackgroundColor(req *pb.RpcBlockSetTextBackgroundColorRequest) *pb.RpcBlockSetTextBackgroundColorResponse {
	response := func(code pb.RpcBlockSetTextBackgroundColorResponseErrorCode, err error) *pb.RpcBlockSetTextBackgroundColorResponse {
		m := &pb.RpcBlockSetTextBackgroundColorResponse{Error: &pb.RpcBlockSetTextBackgroundColorResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	if err := mw.blockService.SetTextBackgroundColor(*req); err != nil {
		return response(pb.RpcBlockSetTextBackgroundColorResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockSetTextBackgroundColorResponseError_NULL, nil)
}

func (mw *Middleware) ExternalDropFiles(req *pb.RpcExternalDropFilesRequest) *pb.RpcExternalDropFilesResponse {
	response := func(code pb.RpcExternalDropFilesResponseErrorCode, err error) *pb.RpcExternalDropFilesResponse {
		m := &pb.RpcExternalDropFilesResponse{Error: &pb.RpcExternalDropFilesResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcExternalDropFilesResponseError_NULL, nil)
}

func (mw *Middleware) ExternalDropContent(req *pb.RpcExternalDropContentRequest) *pb.RpcExternalDropContentResponse {
	response := func(code pb.RpcExternalDropContentResponseErrorCode, err error) *pb.RpcExternalDropContentResponse {
		m := &pb.RpcExternalDropContentResponse{Error: &pb.RpcExternalDropContentResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcExternalDropContentResponseError_NULL, nil)
}

func (mw *Middleware) BlockListMove(req *pb.RpcBlockListMoveRequest) *pb.RpcBlockListMoveResponse {
	response := func(code pb.RpcBlockListMoveResponseErrorCode, err error) *pb.RpcBlockListMoveResponse {
		m := &pb.RpcBlockListMoveResponse{Error: &pb.RpcBlockListMoveResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	if err := mw.blockService.MoveBlocks(*req); err != nil {
		return response(pb.RpcBlockListMoveResponseError_UNKNOWN_ERROR, err)
	}
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

func (mw *Middleware) BlockSetTextText(req *pb.RpcBlockSetTextTextRequest) *pb.RpcBlockSetTextTextResponse {
	response := func(code pb.RpcBlockSetTextTextResponseErrorCode, err error) *pb.RpcBlockSetTextTextResponse {
		m := &pb.RpcBlockSetTextTextResponse{Error: &pb.RpcBlockSetTextTextResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	if err := mw.blockService.SetTextText(*req); err != nil {
		return response(pb.RpcBlockSetTextTextResponseError_UNKNOWN_ERROR, err)
	}
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
	if err := mw.blockService.SetTextStyle(*req); err != nil {
		return response(pb.RpcBlockSetTextStyleResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockSetTextStyleResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetTextChecked(req *pb.RpcBlockSetTextCheckedRequest) *pb.RpcBlockSetTextCheckedResponse {
	response := func(code pb.RpcBlockSetTextCheckedResponseErrorCode, err error) *pb.RpcBlockSetTextCheckedResponse {
		m := &pb.RpcBlockSetTextCheckedResponse{Error: &pb.RpcBlockSetTextCheckedResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	if err := mw.blockService.SetTextChecked(*req); err != nil {
		return response(pb.RpcBlockSetTextCheckedResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockSetTextCheckedResponseError_NULL, nil)
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
	if err := mw.blockService.SetIconName(*req); err != nil {
		return response(pb.RpcBlockSetIconNameResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockSetIconNameResponseError_NULL, nil)
}

func (mw *Middleware) switchAccount(accountId string) {
	if mw.blockService != nil {
		mw.blockService.Close()
	}

	mw.blockService = block.NewService(accountId, anytype.NewAnytype(mw.Anytype), mw.SendEvent)
}

func (mw *Middleware) BlockSplit(req *pb.RpcBlockSplitRequest) *pb.RpcBlockSplitResponse {
	response := func(blockId string, code pb.RpcBlockSplitResponseErrorCode, err error) *pb.RpcBlockSplitResponse {
		m := &pb.RpcBlockSplitResponse{BlockId: blockId, Error: &pb.RpcBlockSplitResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	blockId, err := mw.blockService.SplitBlock(*req)
	if err != nil {
		return response("", pb.RpcBlockSplitResponseError_UNKNOWN_ERROR, err)
	}
	return response(blockId, pb.RpcBlockSplitResponseError_NULL, nil)
}

func (mw *Middleware) BlockMerge(req *pb.RpcBlockMergeRequest) *pb.RpcBlockMergeResponse {
	response := func(code pb.RpcBlockMergeResponseErrorCode, err error) *pb.RpcBlockMergeResponse {
		m := &pb.RpcBlockMergeResponse{Error: &pb.RpcBlockMergeResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	if err := mw.blockService.MergeBlock(*req); err != nil {
		return response(pb.RpcBlockMergeResponseError_UNKNOWN_ERROR, nil)
	}
	return response(pb.RpcBlockMergeResponseError_NULL, nil)
}
