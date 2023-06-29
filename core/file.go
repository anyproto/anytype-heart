package core

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) FileDownload(cctx context.Context, req *pb.RpcFileDownloadRequest) *pb.RpcFileDownloadResponse {
	ctx := mw.newContext(cctx)
	response := func(path string, code pb.RpcFileDownloadResponseErrorCode, err error) *pb.RpcFileDownloadResponse {
		m := &pb.RpcFileDownloadResponse{Error: &pb.RpcFileDownloadResponseError{Code: code}, LocalPath: path}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	var path string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		path, err = bs.DownloadFile(ctx, req)
		return
	})
	if err != nil {
		// TODO Maybe use the appropriate error code?
		return response("", pb.RpcFileDownloadResponseError_UNKNOWN_ERROR, err)
	}

	return response(path, pb.RpcFileDownloadResponseError_NULL, nil)
}

func (mw *Middleware) FileDrop(cctx context.Context, req *pb.RpcFileDropRequest) *pb.RpcFileDropResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcFileDropResponseErrorCode, err error) *pb.RpcFileDropResponse {
		m := &pb.RpcFileDropResponse{Error: &pb.RpcFileDropResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.DropFiles(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcFileDropResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcFileDropResponseError_NULL, nil)
}

func (mw *Middleware) FileListOffload(cctx context.Context, req *pb.RpcFileListOffloadRequest) *pb.RpcFileListOffloadResponse {
	mw.m.RLock()
	defer mw.m.RUnlock()
	response := func(filesOffloaded uint64, bytesOffloaded uint64, code pb.RpcFileListOffloadResponseErrorCode, err error) *pb.RpcFileListOffloadResponse {
		m := &pb.RpcFileListOffloadResponse{Error: &pb.RpcFileListOffloadResponseError{Code: code}, BytesOffloaded: bytesOffloaded, FilesOffloaded: int32(filesOffloaded)}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if mw.app == nil {
		return response(0, 0, pb.RpcFileListOffloadResponseError_NODE_NOT_STARTED, fmt.Errorf("anytype is nil"))
	}

	fileService := app.MustComponent[files.Service](mw.app)
	totalBytesOffloaded, totalFilesOffloaded, err := fileService.FileListOffload(req.OnlyIds, req.IncludeNotPinned)
	if err != nil {
		return response(0, 0, pb.RpcFileListOffloadResponseError_UNKNOWN_ERROR, err)
	}

	return response(totalFilesOffloaded, totalBytesOffloaded, pb.RpcFileListOffloadResponseError_NULL, nil)
}

func (mw *Middleware) FileOffload(cctx context.Context, req *pb.RpcFileOffloadRequest) *pb.RpcFileOffloadResponse {
	mw.m.RLock()
	defer mw.m.RUnlock()
	response := func(bytesOffloaded uint64, code pb.RpcFileOffloadResponseErrorCode, err error) *pb.RpcFileOffloadResponse {
		m := &pb.RpcFileOffloadResponse{BytesOffloaded: bytesOffloaded, Error: &pb.RpcFileOffloadResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if mw.app == nil {
		return response(0, pb.RpcFileOffloadResponseError_NODE_NOT_STARTED, fmt.Errorf("anytype is nil"))
	}

	fileService := app.MustComponent[files.Service](mw.app)

	bytesRemoved, err := fileService.FileOffload(req.Id, req.IncludeNotPinned)
	if err != nil {
		log.Errorf("failed to offload file %s: %s", req.Id, err.Error())
	}

	return response(bytesRemoved, pb.RpcFileOffloadResponseError_NULL, nil)
}

func (mw *Middleware) FileUpload(cctx context.Context, req *pb.RpcFileUploadRequest) *pb.RpcFileUploadResponse {
	ctx := mw.newContext(cctx)
	response := func(hash string, code pb.RpcFileUploadResponseErrorCode, err error) *pb.RpcFileUploadResponse {
		m := &pb.RpcFileUploadResponse{Error: &pb.RpcFileUploadResponseError{Code: code}, Hash: hash}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	var hash string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		hash, err = bs.UploadFile(ctx, *req)
		return
	})
	if err != nil {
		return response("", pb.RpcFileUploadResponseError_UNKNOWN_ERROR, err)
	}
	return response(hash, pb.RpcFileUploadResponseError_NULL, nil)
}

func (mw *Middleware) FileSpaceUsage(cctx context.Context, req *pb.RpcFileSpaceUsageRequest) *pb.RpcFileSpaceUsageResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcFileSpaceUsageResponseErrorCode, err error, usage *pb.RpcFileSpaceUsageResponseUsage) *pb.RpcFileSpaceUsageResponse {
		m := &pb.RpcFileSpaceUsageResponse{
			Error: &pb.RpcFileSpaceUsageResponseError{Code: code},
			Usage: usage,
		}

		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	usage, err := getService[files.Service](mw).GetSpaceUsage(ctx)
	if err != nil {
		return response(pb.RpcFileSpaceUsageResponseError_UNKNOWN_ERROR, err, nil)
	}
	return response(pb.RpcFileSpaceUsageResponseError_NULL, nil, usage)
}
