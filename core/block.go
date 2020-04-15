package core

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) BlockCreate(req *pb.RpcBlockCreateRequest) *pb.RpcBlockCreateResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockCreateResponseErrorCode, id string, err error) *pb.RpcBlockCreateResponse {
		m := &pb.RpcBlockCreateResponse{Error: &pb.RpcBlockCreateResponseError{Code: code}, BlockId: id}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	id, err := mw.blockService.CreateBlock(ctx, *req)
	if err != nil {
		return response(pb.RpcBlockCreateResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockCreateResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockCreatePage(req *pb.RpcBlockCreatePageRequest) *pb.RpcBlockCreatePageResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockCreatePageResponseErrorCode, id, targetId string, err error) *pb.RpcBlockCreatePageResponse {
		m := &pb.RpcBlockCreatePageResponse{Error: &pb.RpcBlockCreatePageResponseError{Code: code}, BlockId: id, TargetId: targetId}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	id, targetId, err := mw.blockService.CreatePage(ctx, *req)
	if err != nil {
		return response(pb.RpcBlockCreatePageResponseError_UNKNOWN_ERROR, "", "", err)
	}
	return response(pb.RpcBlockCreatePageResponseError_NULL, id, targetId, nil)
}

func (mw *Middleware) BlockOpen(req *pb.RpcBlockOpenRequest) *pb.RpcBlockOpenResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockOpenResponseErrorCode, err error) *pb.RpcBlockOpenResponse {
		m := &pb.RpcBlockOpenResponse{Error: &pb.RpcBlockOpenResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}

	if err := mw.blockService.OpenBlock(ctx, req.BlockId, req.BreadcrumbsIds...); err != nil {
		switch err {
		case block.ErrBlockNotFound:
			return response(pb.RpcBlockOpenResponseError_BAD_INPUT, err)
		}
		return response(pb.RpcBlockOpenResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcBlockOpenResponseError_NULL, nil)
}

func (mw *Middleware) BlockOpenBreadcrumbs(req *pb.RpcBlockOpenBreadcrumbsRequest) *pb.RpcBlockOpenBreadcrumbsResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockOpenBreadcrumbsResponseErrorCode, id string, err error) *pb.RpcBlockOpenBreadcrumbsResponse {
		m := &pb.RpcBlockOpenBreadcrumbsResponse{Error: &pb.RpcBlockOpenBreadcrumbsResponseError{Code: code}, BlockId: id}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}

	id, err := mw.blockService.OpenBreadcrumbsBlock(ctx)
	if err != nil {
		switch err {
		case block.ErrBlockNotFound:
			return response(pb.RpcBlockOpenBreadcrumbsResponseError_BAD_INPUT, "", err)
		}
		return response(pb.RpcBlockOpenBreadcrumbsResponseError_UNKNOWN_ERROR, "", err)
	}

	return response(pb.RpcBlockOpenBreadcrumbsResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockCutBreadcrumbs(req *pb.RpcBlockCutBreadcrumbsRequest) *pb.RpcBlockCutBreadcrumbsResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockCutBreadcrumbsResponseErrorCode, err error) *pb.RpcBlockCutBreadcrumbsResponse {
		m := &pb.RpcBlockCutBreadcrumbsResponse{Error: &pb.RpcBlockCutBreadcrumbsResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}

	if err := mw.blockService.CutBreadcrumbs(ctx, *req); err != nil {
		return response(pb.RpcBlockCutBreadcrumbsResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcBlockCutBreadcrumbsResponseError_NULL, nil)
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
func (mw *Middleware) BlockCopy(req *pb.RpcBlockCopyRequest) *pb.RpcBlockCopyResponse {
	response := func(code pb.RpcBlockCopyResponseErrorCode, html string, err error) *pb.RpcBlockCopyResponse {
		m := &pb.RpcBlockCopyResponse{
			Error: &pb.RpcBlockCopyResponseError{Code: code},
			Html:  html,
		}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	html, err := mw.blockService.Copy(*req, mw.getImages(req.Blocks))

	if err != nil {
		return response(pb.RpcBlockCopyResponseError_UNKNOWN_ERROR, "", err)
	}

	return response(pb.RpcBlockCopyResponseError_NULL, html, nil)
}

func (mw *Middleware) getImages(blocks []*model.Block) map[string][]byte {
	images := make(map[string][]byte)
	for _, b := range blocks {
		if file := b.GetFile(); file != nil && file.State == model.BlockContentFile_Done {
			getBlobReq := &pb.RpcIpfsImageGetBlobRequest{
				Hash:      file.Hash,
				WantWidth: 1024,
			}
			resp := mw.ImageGetBlob(getBlobReq)
			images[file.Hash] = resp.Blob
		}
	}

	return images
}

func (mw *Middleware) BlockPaste(req *pb.RpcBlockPasteRequest) *pb.RpcBlockPasteResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockPasteResponseErrorCode, blockIds []string, caretPosition int32, err error) *pb.RpcBlockPasteResponse {
		m := &pb.RpcBlockPasteResponse{Error: &pb.RpcBlockPasteResponseError{Code: code}, BlockIds: blockIds, CaretPosition: caretPosition}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}

	blockIds, uploadArr, caretPosition, err := mw.blockService.Paste(ctx, *req)
	log.Debug("Image requests to upload after paste:", uploadArr)
	for _, r := range uploadArr {
		r.ContextId = req.ContextId

		if err := mw.blockService.UploadBlockFile(nil, r); err != nil {
			return response(pb.RpcBlockPasteResponseError_UNKNOWN_ERROR, blockIds, caretPosition, err)
		}
	}

	if err != nil {
		return response(pb.RpcBlockPasteResponseError_UNKNOWN_ERROR, nil, -1, err)
	}

	return response(pb.RpcBlockPasteResponseError_NULL, blockIds, caretPosition, nil)
}

func (mw *Middleware) BlockCut(req *pb.RpcBlockCutRequest) *pb.RpcBlockCutResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockCutResponseErrorCode, textSlot string, htmlSlot string, anySlot []*model.Block, err error) *pb.RpcBlockCutResponse {
		m := &pb.RpcBlockCutResponse{
			Error:    &pb.RpcBlockCutResponseError{Code: code},
			TextSlot: textSlot,
			HtmlSlot: htmlSlot,
			AnySlot:  anySlot,
		}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}

	textSlot, htmlSlot, anySlot, err := mw.blockService.Cut(ctx, *req, mw.getImages(req.Blocks))

	if err != nil {
		var emptyAnySlot []*model.Block
		return response(pb.RpcBlockCutResponseError_UNKNOWN_ERROR, "", "", emptyAnySlot, err)
	}

	return response(pb.RpcBlockCutResponseError_NULL, textSlot, htmlSlot, anySlot, nil)
}

func (mw *Middleware) BlockExport(req *pb.RpcBlockExportRequest) *pb.RpcBlockExportResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockExportResponseErrorCode, path string, err error) *pb.RpcBlockExportResponse {
		m := &pb.RpcBlockExportResponse{
			Error: &pb.RpcBlockExportResponseError{Code: code},
			Path:  path,
		}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}

	path, err := mw.blockService.Export(*req, mw.getImages(req.Blocks))

	if err != nil {
		return response(pb.RpcBlockExportResponseError_UNKNOWN_ERROR, path, err)
	}

	return response(pb.RpcBlockExportResponseError_NULL, path, nil)
}

func (mw *Middleware) BlockUpload(req *pb.RpcBlockUploadRequest) *pb.RpcBlockUploadResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockUploadResponseErrorCode, err error) *pb.RpcBlockUploadResponse {
		m := &pb.RpcBlockUploadResponse{Error: &pb.RpcBlockUploadResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	if err := mw.blockService.UploadBlockFile(ctx, *req); err != nil {
		return response(pb.RpcBlockUploadResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockUploadResponseError_NULL, nil)
}

func (mw *Middleware) BlockUnlink(req *pb.RpcBlockUnlinkRequest) *pb.RpcBlockUnlinkResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockUnlinkResponseErrorCode, err error) *pb.RpcBlockUnlinkResponse {
		m := &pb.RpcBlockUnlinkResponse{Error: &pb.RpcBlockUnlinkResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	if err := mw.blockService.UnlinkBlock(ctx, *req); err != nil {
		return response(pb.RpcBlockUnlinkResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockUnlinkResponseError_NULL, nil)
}

func (mw *Middleware) BlockListDuplicate(req *pb.RpcBlockListDuplicateRequest) *pb.RpcBlockListDuplicateResponse {
	ctx := state.NewContext(nil)
	response := func(ids []string, code pb.RpcBlockListDuplicateResponseErrorCode, err error) *pb.RpcBlockListDuplicateResponse {
		m := &pb.RpcBlockListDuplicateResponse{BlockIds: ids, Error: &pb.RpcBlockListDuplicateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	ids, err := mw.blockService.DuplicateBlocks(ctx, *req)
	if err != nil {
		return response(nil, pb.RpcBlockListDuplicateResponseError_UNKNOWN_ERROR, err)
	}
	return response(ids, pb.RpcBlockListDuplicateResponseError_NULL, nil)
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
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockSetFieldsResponseErrorCode, err error) *pb.RpcBlockSetFieldsResponse {
		m := &pb.RpcBlockSetFieldsResponse{Error: &pb.RpcBlockSetFieldsResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	if err := mw.blockService.SetFields(ctx, *req); err != nil {
		return response(pb.RpcBlockSetFieldsResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockSetFieldsResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetDetails(req *pb.RpcBlockSetDetailsRequest) *pb.RpcBlockSetDetailsResponse {
	response := func(code pb.RpcBlockSetDetailsResponseErrorCode, err error) *pb.RpcBlockSetDetailsResponse {
		m := &pb.RpcBlockSetDetailsResponse{Error: &pb.RpcBlockSetDetailsResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	if err := mw.blockService.SetDetails(*req); err != nil {
		return response(pb.RpcBlockSetDetailsResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockSetDetailsResponseError_NULL, nil)
}

func (mw *Middleware) BlockListSetFields(req *pb.RpcBlockListSetFieldsRequest) *pb.RpcBlockListSetFieldsResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockListSetFieldsResponseErrorCode, err error) *pb.RpcBlockListSetFieldsResponse {
		m := &pb.RpcBlockListSetFieldsResponse{Error: &pb.RpcBlockListSetFieldsResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	if err := mw.blockService.SetFieldsList(ctx, *req); err != nil {
		return response(pb.RpcBlockListSetFieldsResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockListSetFieldsResponseError_NULL, nil)
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

func (mw *Middleware) BlockSetPageIsArchived(req *pb.RpcBlockSetPageIsArchivedRequest) *pb.RpcBlockSetPageIsArchivedResponse {
	response := func(code pb.RpcBlockSetPageIsArchivedResponseErrorCode, err error) *pb.RpcBlockSetPageIsArchivedResponse {
		m := &pb.RpcBlockSetPageIsArchivedResponse{Error: &pb.RpcBlockSetPageIsArchivedResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	if err := mw.blockService.SetPageIsArchived(*req); err != nil {
		return response(pb.RpcBlockSetPageIsArchivedResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockSetPageIsArchivedResponseError_NULL, nil)
}

func (mw *Middleware) BlockReplace(req *pb.RpcBlockReplaceRequest) *pb.RpcBlockReplaceResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockReplaceResponseErrorCode, blockId string, err error) *pb.RpcBlockReplaceResponse {
		m := &pb.RpcBlockReplaceResponse{Error: &pb.RpcBlockReplaceResponseError{Code: code}, BlockId: blockId}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	blockId, err := mw.blockService.ReplaceBlock(ctx, *req)
	if err != nil {
		return response(pb.RpcBlockReplaceResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockReplaceResponseError_NULL, blockId, nil)
}

func (mw *Middleware) BlockSetTextColor(req *pb.RpcBlockSetTextColorRequest) *pb.RpcBlockSetTextColorResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockSetTextColorResponseErrorCode, err error) *pb.RpcBlockSetTextColorResponse {
		m := &pb.RpcBlockSetTextColorResponse{Error: &pb.RpcBlockSetTextColorResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	if err := mw.blockService.SetTextColor(nil, req.ContextId, req.Color, req.BlockId); err != nil {
		return response(pb.RpcBlockSetTextColorResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockSetTextColorResponseError_NULL, nil)
}

func (mw *Middleware) BlockListSetBackgroundColor(req *pb.RpcBlockListSetBackgroundColorRequest) *pb.RpcBlockListSetBackgroundColorResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockListSetBackgroundColorResponseErrorCode, err error) *pb.RpcBlockListSetBackgroundColorResponse {
		m := &pb.RpcBlockListSetBackgroundColorResponse{Error: &pb.RpcBlockListSetBackgroundColorResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	if err := mw.blockService.SetBackgroundColor(ctx, req.ContextId, req.Color, req.BlockIds...); err != nil {
		return response(pb.RpcBlockListSetBackgroundColorResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockListSetBackgroundColorResponseError_NULL, nil)
}

func (mw *Middleware) BlockListSetAlign(req *pb.RpcBlockListSetAlignRequest) *pb.RpcBlockListSetAlignResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockListSetAlignResponseErrorCode, err error) *pb.RpcBlockListSetAlignResponse {
		m := &pb.RpcBlockListSetAlignResponse{Error: &pb.RpcBlockListSetAlignResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	if err := mw.blockService.SetAlign(ctx, req.ContextId, req.Align, req.BlockIds...); err != nil {
		return response(pb.RpcBlockListSetAlignResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockListSetAlignResponseError_NULL, nil)
}

func (mw *Middleware) ExternalDropFiles(req *pb.RpcExternalDropFilesRequest) *pb.RpcExternalDropFilesResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcExternalDropFilesResponseErrorCode, err error) *pb.RpcExternalDropFilesResponse {
		m := &pb.RpcExternalDropFilesResponse{Error: &pb.RpcExternalDropFilesResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	if err := mw.blockService.DropFiles(*req); err != nil {
		return response(pb.RpcExternalDropFilesResponseError_UNKNOWN_ERROR, err)
	}
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
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockListMoveResponseErrorCode, err error) *pb.RpcBlockListMoveResponse {
		m := &pb.RpcBlockListMoveResponse{Error: &pb.RpcBlockListMoveResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	if err := mw.blockService.MoveBlocks(ctx, *req); err != nil {
		return response(pb.RpcBlockListMoveResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockListMoveResponseError_NULL, nil)
}

func (mw *Middleware) BlockListMoveToNewPage(req *pb.RpcBlockListMoveToNewPageRequest) *pb.RpcBlockListMoveToNewPageResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockListMoveToNewPageResponseErrorCode, linkId string, err error) *pb.RpcBlockListMoveToNewPageResponse {
		m := &pb.RpcBlockListMoveToNewPageResponse{Error: &pb.RpcBlockListMoveToNewPageResponseError{Code: code}, LinkId: linkId}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	linkId, err := mw.blockService.MoveBlocksToNewPage(ctx, *req)

	if err != nil {
		return response(pb.RpcBlockListMoveToNewPageResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockListMoveToNewPageResponseError_NULL, linkId, nil)
}

func (mw *Middleware) BlockListConvertChildrenToPages(req *pb.RpcBlockListConvertChildrenToPagesRequest) *pb.RpcBlockListConvertChildrenToPagesResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockListConvertChildrenToPagesResponseErrorCode, linkIds []string, err error) *pb.RpcBlockListConvertChildrenToPagesResponse {
		m := &pb.RpcBlockListConvertChildrenToPagesResponse{Error: &pb.RpcBlockListConvertChildrenToPagesResponseError{Code: code}, LinkIds: linkIds}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	linkIds, err := mw.blockService.ConvertChildrenToPages(ctx, *req)

	if err != nil {
		return response(pb.RpcBlockListConvertChildrenToPagesResponseError_UNKNOWN_ERROR, []string{}, err)
	}
	return response(pb.RpcBlockListConvertChildrenToPagesResponseError_NULL, linkIds, nil)
}

func (mw *Middleware) BlockListSetTextStyle(req *pb.RpcBlockListSetTextStyleRequest) *pb.RpcBlockListSetTextStyleResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockListSetTextStyleResponseErrorCode, err error) *pb.RpcBlockListSetTextStyleResponse {
		m := &pb.RpcBlockListSetTextStyleResponse{Error: &pb.RpcBlockListSetTextStyleResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	if err := mw.blockService.SetTextStyle(ctx, req.ContextId, req.Style, req.BlockIds...); err != nil {
		return response(pb.RpcBlockListSetTextStyleResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockListSetTextStyleResponseError_NULL, nil)
}

func (mw *Middleware) BlockListSetDivStyle(req *pb.RpcBlockListSetDivStyleRequest) *pb.RpcBlockListSetDivStyleResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockListSetDivStyleResponseErrorCode, err error) *pb.RpcBlockListSetDivStyleResponse {
		m := &pb.RpcBlockListSetDivStyleResponse{Error: &pb.RpcBlockListSetDivStyleResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	if err := mw.blockService.SetDivStyle(ctx, req.ContextId, req.Style, req.BlockIds...); err != nil {
		return response(pb.RpcBlockListSetDivStyleResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockListSetDivStyleResponseError_NULL, nil)
}

func (mw *Middleware) BlockListSetTextColor(req *pb.RpcBlockListSetTextColorRequest) *pb.RpcBlockListSetTextColorResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockListSetTextColorResponseErrorCode, err error) *pb.RpcBlockListSetTextColorResponse {
		m := &pb.RpcBlockListSetTextColorResponse{Error: &pb.RpcBlockListSetTextColorResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	if err := mw.blockService.SetTextColor(ctx, req.ContextId, req.Color, req.BlockIds...); err != nil {
		return response(pb.RpcBlockListSetTextColorResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockListSetTextColorResponseError_NULL, nil)
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
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockSetTextStyleResponseErrorCode, err error) *pb.RpcBlockSetTextStyleResponse {
		m := &pb.RpcBlockSetTextStyleResponse{Error: &pb.RpcBlockSetTextStyleResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	if err := mw.blockService.SetTextStyle(ctx, req.ContextId, req.Style, req.BlockId); err != nil {
		return response(pb.RpcBlockSetTextStyleResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockSetTextStyleResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetTextChecked(req *pb.RpcBlockSetTextCheckedRequest) *pb.RpcBlockSetTextCheckedResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockSetTextCheckedResponseErrorCode, err error) *pb.RpcBlockSetTextCheckedResponse {
		m := &pb.RpcBlockSetTextCheckedResponse{Error: &pb.RpcBlockSetTextCheckedResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	if err := mw.blockService.SetTextChecked(ctx, *req); err != nil {
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

func (mw *Middleware) switchAccount(accountId string) {
	if mw.blockService != nil {
		mw.blockService.Close()
	}

	mw.blockService = block.NewService(accountId, anytype.NewService(mw.Anytype), mw.linkPreview, mw.SendEvent)
}

func (mw *Middleware) BlockSplit(req *pb.RpcBlockSplitRequest) *pb.RpcBlockSplitResponse {
	ctx := state.NewContext(nil)
	response := func(blockId string, code pb.RpcBlockSplitResponseErrorCode, err error) *pb.RpcBlockSplitResponse {
		m := &pb.RpcBlockSplitResponse{BlockId: blockId, Error: &pb.RpcBlockSplitResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	blockId, err := mw.blockService.SplitBlock(ctx, *req)
	if err != nil {
		return response("", pb.RpcBlockSplitResponseError_UNKNOWN_ERROR, err)
	}
	return response(blockId, pb.RpcBlockSplitResponseError_NULL, nil)
}

func (mw *Middleware) BlockMerge(req *pb.RpcBlockMergeRequest) *pb.RpcBlockMergeResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockMergeResponseErrorCode, err error) *pb.RpcBlockMergeResponse {
		m := &pb.RpcBlockMergeResponse{Error: &pb.RpcBlockMergeResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	if err := mw.blockService.MergeBlock(ctx, *req); err != nil {
		return response(pb.RpcBlockMergeResponseError_UNKNOWN_ERROR, nil)
	}
	return response(pb.RpcBlockMergeResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetLinkTargetBlockId(req *pb.RpcBlockSetLinkTargetBlockIdRequest) *pb.RpcBlockSetLinkTargetBlockIdResponse {
	response := func(code pb.RpcBlockSetLinkTargetBlockIdResponseErrorCode, err error) *pb.RpcBlockSetLinkTargetBlockIdResponse {
		m := &pb.RpcBlockSetLinkTargetBlockIdResponse{Error: &pb.RpcBlockSetLinkTargetBlockIdResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	// TODO
	return response(pb.RpcBlockSetLinkTargetBlockIdResponseError_NULL, nil)
}

func (mw *Middleware) BlockBookmarkFetch(req *pb.RpcBlockBookmarkFetchRequest) *pb.RpcBlockBookmarkFetchResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockBookmarkFetchResponseErrorCode, err error) *pb.RpcBlockBookmarkFetchResponse {
		m := &pb.RpcBlockBookmarkFetchResponse{Error: &pb.RpcBlockBookmarkFetchResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	if err := mw.blockService.BookmarkFetch(ctx, *req); err != nil {
		return response(pb.RpcBlockBookmarkFetchResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockBookmarkFetchResponseError_NULL, nil)
}

func (mw *Middleware) UploadFile(req *pb.RpcUploadFileRequest) *pb.RpcUploadFileResponse {
	response := func(hash string, code pb.RpcUploadFileResponseErrorCode, err error) *pb.RpcUploadFileResponse {
		m := &pb.RpcUploadFileResponse{Error: &pb.RpcUploadFileResponseError{Code: code}, Hash: hash}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	hash, err := mw.blockService.UploadFile(*req)
	if err != nil {
		return response("", pb.RpcUploadFileResponseError_UNKNOWN_ERROR, err)
	}
	return response(hash, pb.RpcUploadFileResponseError_NULL, nil)
}

func (mw *Middleware) BlockBookmarkCreateAndFetch(req *pb.RpcBlockBookmarkCreateAndFetchRequest) *pb.RpcBlockBookmarkCreateAndFetchResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockBookmarkCreateAndFetchResponseErrorCode, id string, err error) *pb.RpcBlockBookmarkCreateAndFetchResponse {
		m := &pb.RpcBlockBookmarkCreateAndFetchResponse{Error: &pb.RpcBlockBookmarkCreateAndFetchResponseError{Code: code}, BlockId: id}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	id, err := mw.blockService.BookmarkCreateAndFetch(ctx, *req)
	if err != nil {
		return response(pb.RpcBlockBookmarkCreateAndFetchResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockBookmarkCreateAndFetchResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockFileCreateAndUpload(req *pb.RpcBlockFileCreateAndUploadRequest) *pb.RpcBlockFileCreateAndUploadResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockFileCreateAndUploadResponseErrorCode, id string, err error) *pb.RpcBlockFileCreateAndUploadResponse {
		m := &pb.RpcBlockFileCreateAndUploadResponse{Error: &pb.RpcBlockFileCreateAndUploadResponseError{Code: code}, BlockId: id}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	id, err := mw.blockService.CreateAndUploadFile(ctx, *req)
	if err != nil {
		return response(pb.RpcBlockFileCreateAndUploadResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockFileCreateAndUploadResponseError_NULL, id, nil)
}
