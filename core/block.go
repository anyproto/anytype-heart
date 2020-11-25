package core

import (
	"fmt"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/globalsign/mgo/bson"
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
	var id string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		id, err = bs.CreateBlock(ctx, *req)
		return
	})
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
	var id, targetId string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		id, targetId, err = bs.CreatePage(ctx, "", *req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockCreatePageResponseError_UNKNOWN_ERROR, "", "", err)
	}
	return response(pb.RpcBlockCreatePageResponseError_NULL, id, targetId, nil)
}

func (mw *Middleware) BlockCreateSet(req *pb.RpcBlockCreateSetRequest) *pb.RpcBlockCreateSetResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockCreateSetResponseErrorCode, id, targetId string, err error) *pb.RpcBlockCreateSetResponse {
		m := &pb.RpcBlockCreateSetResponse{Error: &pb.RpcBlockCreateSetResponseError{Code: code}, BlockId: id, TargetId: targetId}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var id, targetId string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		id, targetId, err = bs.CreateSet(ctx, *req)
		return
	})

	if err != nil {
		if err == block.ErrUnknownObjectType {
			return response(pb.RpcBlockCreateSetResponseError_UNKNOWN_OBJECT_TYPE_URL, "", "", err)
		}

		return response(pb.RpcBlockCreateSetResponseError_UNKNOWN_ERROR, "", "", err)
	}
	return response(pb.RpcBlockCreateSetResponseError_NULL, id, targetId, nil)
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

	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.OpenBlock(ctx, req.BlockId)
	})
	if err != nil {
		if err == source.ErrUnknownDataFormat {
			return response(pb.RpcBlockOpenResponseError_ANYTYPE_NEEDS_UPGRADE, err)
		}
		return response(pb.RpcBlockOpenResponseError_UNKNOWN_ERROR, err)
	}

	err = mw.Anytype.ObjectStore().UpdateLastOpened(req.BlockId, time.Now())
	if err != nil {
		log.Errorf("failed to update last opened for the page %s: %s", req.BlockId, err.Error())
	}

	return response(pb.RpcBlockOpenResponseError_NULL, nil)
}

func (mw *Middleware) BlockGetPublicWebURL(req *pb.RpcBlockGetPublicWebURLRequest) *pb.RpcBlockGetPublicWebURLResponse {
	response := func(url string, code pb.RpcBlockGetPublicWebURLResponseErrorCode, err error) *pb.RpcBlockGetPublicWebURLResponse {
		m := &pb.RpcBlockGetPublicWebURLResponse{Url: url, Error: &pb.RpcBlockGetPublicWebURLResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	/*
		b, err := mw.Anytype.GetBlock(req.BlockId)
		if err != nil {
			return response("", pb.RpcBlockGetPublicWebURLResponseError_UNKNOWN_ERROR, err)
		}

		snap, err := b.GetLastSnapshot()
		if err != nil {
			return response("", pb.RpcBlockGetPublicWebURLResponseError_UNKNOWN_ERROR, err)
		}

		u, err := snap.PublicWebURL()
		if err != nil {
			return response("", pb.RpcBlockGetPublicWebURLResponseError_UNKNOWN_ERROR, err)
		}

		return response(u, pb.RpcBlockGetPublicWebURLResponseError_NULL, nil)

	*/
	// TODO: make with changes
	return response("", pb.RpcBlockGetPublicWebURLResponseError_UNKNOWN_ERROR, fmt.Errorf("disabled"))
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
	var id string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		id, err = bs.OpenBreadcrumbsBlock(ctx)
		return
	})
	if err != nil {
		return response(pb.RpcBlockOpenBreadcrumbsResponseError_UNKNOWN_ERROR, "", err)
	}

	return response(pb.RpcBlockOpenBreadcrumbsResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockSetBreadcrumbs(req *pb.RpcBlockSetBreadcrumbsRequest) *pb.RpcBlockSetBreadcrumbsResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockSetBreadcrumbsResponseErrorCode, err error) *pb.RpcBlockSetBreadcrumbsResponse {
		m := &pb.RpcBlockSetBreadcrumbsResponse{Error: &pb.RpcBlockSetBreadcrumbsResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetBreadcrumbs(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockSetBreadcrumbsResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcBlockSetBreadcrumbsResponseError_NULL, nil)
}

func (mw *Middleware) BlockClose(req *pb.RpcBlockCloseRequest) *pb.RpcBlockCloseResponse {
	response := func(code pb.RpcBlockCloseResponseErrorCode, err error) *pb.RpcBlockCloseResponse {
		m := &pb.RpcBlockCloseResponse{Error: &pb.RpcBlockCloseResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.CloseBlock(req.BlockId)
	})
	if err != nil {
		return response(pb.RpcBlockCloseResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockCloseResponseError_NULL, nil)
}
func (mw *Middleware) BlockCopy(req *pb.RpcBlockCopyRequest) *pb.RpcBlockCopyResponse {
	response := func(code pb.RpcBlockCopyResponseErrorCode, textSlot string, htmlSlot string, anySlot []*model.Block, err error) *pb.RpcBlockCopyResponse {
		m := &pb.RpcBlockCopyResponse{
			Error:    &pb.RpcBlockCopyResponseError{Code: code},
			TextSlot: textSlot,
			HtmlSlot: htmlSlot,
			AnySlot:  anySlot,
		}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	var textSlot, htmlSlot string
	var anySlot []*model.Block
	err := mw.doBlockService(func(bs block.Service) (err error) {
		textSlot, htmlSlot, anySlot, err = bs.Copy(*req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockCopyResponseError_UNKNOWN_ERROR, textSlot, htmlSlot, anySlot, err)
	}

	return response(pb.RpcBlockCopyResponseError_NULL, textSlot, htmlSlot, anySlot, nil)
}

func (mw *Middleware) BlockPaste(req *pb.RpcBlockPasteRequest) *pb.RpcBlockPasteResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockPasteResponseErrorCode, blockIds []string, caretPosition int32, isSameBlockCaret bool, err error) *pb.RpcBlockPasteResponse {
		m := &pb.RpcBlockPasteResponse{Error: &pb.RpcBlockPasteResponseError{Code: code}, BlockIds: blockIds, CaretPosition: caretPosition, IsSameBlockCaret: isSameBlockCaret}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var (
		blockIds         []string
		caretPosition    int32
		isSameBlockCaret bool
		groupId          = bson.NewObjectId().Hex()
	)
	err := mw.doBlockService(func(bs block.Service) (err error) {
		var uploadArr []pb.RpcBlockUploadRequest
		blockIds, uploadArr, caretPosition, isSameBlockCaret, err = bs.Paste(ctx, *req, groupId)
		if err != nil {
			return
		}
		log.Debug("Image requests to upload after paste:", uploadArr)
		for _, r := range uploadArr {
			r.ContextId = req.ContextId
			if err = bs.UploadBlockFile(nil, r, groupId); err != nil {
				return err
			}
		}
		return
	})
	if err != nil {
		return response(pb.RpcBlockPasteResponseError_UNKNOWN_ERROR, nil, -1, isSameBlockCaret, err)
	}

	return response(pb.RpcBlockPasteResponseError_NULL, blockIds, caretPosition, isSameBlockCaret, nil)
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
	var (
		textSlot, htmlSlot string
		anySlot            []*model.Block
	)
	err := mw.doBlockService(func(bs block.Service) (err error) {
		textSlot, htmlSlot, anySlot, err = bs.Cut(ctx, *req)
		return
	})
	if err != nil {
		var emptyAnySlot []*model.Block
		return response(pb.RpcBlockCutResponseError_UNKNOWN_ERROR, "", "", emptyAnySlot, err)
	}

	return response(pb.RpcBlockCutResponseError_NULL, textSlot, htmlSlot, anySlot, nil)
}

func (mw *Middleware) BlockImportMarkdown(req *pb.RpcBlockImportMarkdownRequest) *pb.RpcBlockImportMarkdownResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockImportMarkdownResponseErrorCode, rootLinkIds []string, err error) *pb.RpcBlockImportMarkdownResponse {
		m := &pb.RpcBlockImportMarkdownResponse{
			Error:       &pb.RpcBlockImportMarkdownResponseError{Code: code},
			RootLinkIds: rootLinkIds,
		}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}

	var rootLinkIds []string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		rootLinkIds, err = bs.ImportMarkdown(ctx, *req)
		return err
	})

	if err != nil {
		return response(pb.RpcBlockImportMarkdownResponseError_UNKNOWN_ERROR, rootLinkIds, err)
	}

	return response(pb.RpcBlockImportMarkdownResponseError_NULL, rootLinkIds, nil)
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
	var path string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		path, err = bs.Export(*req)
		return
	})
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.UploadBlockFile(ctx, *req, "")
	})
	if err != nil {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.UnlinkBlock(ctx, *req)
	})
	if err != nil {
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
	var ids []string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		ids, err = bs.DuplicateBlocks(ctx, *req)
		return
	})
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetFields(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockSetFieldsResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockSetFieldsResponseError_NULL, nil)
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetFieldsList(ctx, *req)
	})
	if err != nil {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetPageIsArchived(*req)
	})
	if err != nil {
		return response(pb.RpcBlockSetPageIsArchivedResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockSetPageIsArchivedResponseError_NULL, nil)
}

func (mw *Middleware) BlockListDeletePage(req *pb.RpcBlockListDeletePageRequest) *pb.RpcBlockListDeletePageResponse {
	response := func(code pb.RpcBlockListDeletePageResponseErrorCode, err error) *pb.RpcBlockListDeletePageResponse {
		m := &pb.RpcBlockListDeletePageResponse{Error: &pb.RpcBlockListDeletePageResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.DeletePages(*req)
	})
	if err != nil {
		return response(pb.RpcBlockListDeletePageResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockListDeletePageResponseError_NULL, nil)
}

func (mw *Middleware) BlockListSetPageIsArchived(req *pb.RpcBlockListSetPageIsArchivedRequest) *pb.RpcBlockListSetPageIsArchivedResponse {
	response := func(code pb.RpcBlockListSetPageIsArchivedResponseErrorCode, err error) *pb.RpcBlockListSetPageIsArchivedResponse {
		m := &pb.RpcBlockListSetPageIsArchivedResponse{Error: &pb.RpcBlockListSetPageIsArchivedResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetPagesIsArchived(*req)
	})
	if err != nil {
		return response(pb.RpcBlockListSetPageIsArchivedResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockListSetPageIsArchivedResponseError_NULL, nil)
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
	var blockId string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		blockId, err = bs.ReplaceBlock(ctx, *req)
		return
	})
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetTextColor(nil, req.ContextId, req.Color, req.BlockId)
	})
	if err != nil {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetBackgroundColor(ctx, req.ContextId, req.Color, req.BlockIds...)
	})
	if err != nil {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetAlign(ctx, req.ContextId, req.Align, req.BlockIds...)
	})
	if err != nil {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.DropFiles(*req)
	})
	if err != nil {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.MoveBlocks(ctx, *req)
	})
	if err != nil {
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

	var linkId string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		linkId, err = bs.MoveBlocksToNewPage(ctx, *req)
		return
	})

	if err != nil {
		return response(pb.RpcBlockListMoveToNewPageResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockListMoveToNewPageResponseError_NULL, linkId, nil)
}

func (mw *Middleware) BlockListConvertChildrenToPages(req *pb.RpcBlockListConvertChildrenToPagesRequest) *pb.RpcBlockListConvertChildrenToPagesResponse {
	response := func(code pb.RpcBlockListConvertChildrenToPagesResponseErrorCode, linkIds []string, err error) *pb.RpcBlockListConvertChildrenToPagesResponse {
		m := &pb.RpcBlockListConvertChildrenToPagesResponse{Error: &pb.RpcBlockListConvertChildrenToPagesResponseError{Code: code}, LinkIds: linkIds}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	var linkIds []string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		linkIds, err = bs.ConvertChildrenToPages(*req)
		return
	})
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetTextStyle(ctx, req.ContextId, req.Style, req.BlockIds...)
	})
	if err != nil {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetDivStyle(ctx, req.ContextId, req.Style, req.BlockIds...)
	})
	if err != nil {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetTextColor(ctx, req.ContextId, req.Color, req.BlockIds...)
	})
	if err != nil {
		return response(pb.RpcBlockListSetTextColorResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockListSetTextColorResponseError_NULL, nil)
}

func (mw *Middleware) BlockListSetTextMark(req *pb.RpcBlockListSetTextMarkRequest) *pb.RpcBlockListSetTextMarkResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockListSetTextMarkResponseErrorCode, err error) *pb.RpcBlockListSetTextMarkResponse {
		m := &pb.RpcBlockListSetTextMarkResponse{Error: &pb.RpcBlockListSetTextMarkResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetTextMark(ctx, req.ContextId, req.Mark, req.BlockIds...)
	})
	if err != nil {
		return response(pb.RpcBlockListSetTextMarkResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockListSetTextMarkResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetTextText(req *pb.RpcBlockSetTextTextRequest) *pb.RpcBlockSetTextTextResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockSetTextTextResponseErrorCode, err error) *pb.RpcBlockSetTextTextResponse {
		m := &pb.RpcBlockSetTextTextResponse{Error: &pb.RpcBlockSetTextTextResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetTextText(ctx, *req)
	})
	if err != nil {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetTextStyle(ctx, req.ContextId, req.Style, req.BlockId)
	})
	if err != nil {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetTextChecked(ctx, *req)
	})
	if err != nil {
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
	var id string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		id, err = bs.SplitBlock(ctx, *req)
		return
	})
	if err != nil {
		return response("", pb.RpcBlockSplitResponseError_UNKNOWN_ERROR, err)
	}
	return response(id, pb.RpcBlockSplitResponseError_NULL, nil)
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.MergeBlock(ctx, *req)
	})
	if err != nil {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.BookmarkFetch(ctx, *req)
	})
	if err != nil {
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
	var hash string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		hash, err = bs.UploadFile(*req)
		return
	})
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
	var id string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		id, err = bs.BookmarkCreateAndFetch(ctx, *req)
		return
	})
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
	var id string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		id, err = bs.CreateAndUploadFile(ctx, *req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockFileCreateAndUploadResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockFileCreateAndUploadResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockObjectTypeAdd(req *pb.RpcBlockObjectTypeAddRequest) *pb.RpcBlockObjectTypeAddResponse {
	panic("implement me")
}

func (mw *Middleware) BlockObjectTypeRemove(req *pb.RpcBlockObjectTypeRemoveRequest) *pb.RpcBlockObjectTypeRemoveResponse {
	panic("implement me")
}

func (mw *Middleware) BlockRelationSetKey(req *pb.RpcBlockRelationSetKeyRequest) *pb.RpcBlockRelationSetKeyResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockRelationSetKeyResponseErrorCode, err error) *pb.RpcBlockRelationSetKeyResponse {
		m := &pb.RpcBlockRelationSetKeyResponse{Error: &pb.RpcBlockRelationSetKeyResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}

	if err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetRelationKey(ctx, *req)
	}); err != nil {
		return response(pb.RpcBlockRelationSetKeyResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcBlockRelationSetKeyResponseError_NULL, nil)
}

func (mw *Middleware) BlockRelationAdd(req *pb.RpcBlockRelationAddRequest) *pb.RpcBlockRelationAddResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockRelationAddResponseErrorCode, err error) *pb.RpcBlockRelationAddResponse {
		m := &pb.RpcBlockRelationAddResponse{Error: &pb.RpcBlockRelationAddResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}

	if err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.AddRelationBlock(ctx, *req)
	}); err != nil {
		return response(pb.RpcBlockRelationAddResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcBlockRelationAddResponseError_NULL, nil)
}
