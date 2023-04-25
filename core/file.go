package core

import (
	"context"
	"fmt"

	"github.com/anytypeio/any-sync/app"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/filestorage/filesync"
	"github.com/anytypeio/go-anytype-middleware/pb"
	pb2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/filestore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pin"
	"github.com/anytypeio/go-anytype-middleware/space"
)

func (mw *Middleware) FileDownload(cctx context.Context, req *pb.RpcFileDownloadRequest) *pb.RpcFileDownloadResponse {
	response := func(path string, code pb.RpcFileDownloadResponseErrorCode, err error) *pb.RpcFileDownloadResponse {
		m := &pb.RpcFileDownloadResponse{Error: &pb.RpcFileDownloadResponseError{Code: code}, LocalPath: path}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	var path string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		path, err = bs.DownloadFile(req)
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
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.DropFiles(*req)
	})
	if err != nil {
		return response(pb.RpcFileDropResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcFileDropResponseError_NULL, nil)
}

func (mw *Middleware) FileListOffload(cctx context.Context, req *pb.RpcFileListOffloadRequest) *pb.RpcFileListOffloadResponse {
	mw.m.RLock()
	defer mw.m.RUnlock()
	response := func(filesOffloaded int32, bytesOffloaded uint64, code pb.RpcFileListOffloadResponseErrorCode, err error) *pb.RpcFileListOffloadResponse {
		m := &pb.RpcFileListOffloadResponse{Error: &pb.RpcFileListOffloadResponseError{Code: code}, BytesOffloaded: bytesOffloaded, FilesOffloaded: filesOffloaded}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if mw.app == nil {
		return response(0, 0, pb.RpcFileListOffloadResponseError_NODE_NOT_STARTED, fmt.Errorf("anytype is nil"))
	}

	at := mw.app.MustComponent(core.CName).(core.Service)
	pin := mw.app.MustComponent(pin.CName).(pin.FilePinService)

	if !at.IsStarted() {
		return response(0, 0, pb.RpcFileListOffloadResponseError_NODE_NOT_STARTED, fmt.Errorf("anytype node not started"))
	}

	fileStore := app.MustComponent[filestore.FileStore](mw.app)
	files, err := fileStore.ListTargets()
	if err != nil {
		return response(0, 0, pb.RpcFileListOffloadResponseError_UNKNOWN_ERROR, err)
	}
	pinStatus := pin.PinStatus(files...)
	var (
		totalBytesOffloaded uint64
		totalFilesOffloaded int32
		totalFilesSkipped   int
	)

	for _, fileId := range files {
		if st, exists := pinStatus[fileId]; (!exists || st.Status != pb2.PinStatus_Done) && !req.IncludeNotPinned {
			totalFilesSkipped++
			continue
		}
		bytesRemoved, err := at.FileOffload(fileId)
		if err != nil {
			log.Errorf("failed to offload file %s: %s", fileId, err.Error())
			continue
		}
		if bytesRemoved > 0 {
			totalBytesOffloaded += bytesRemoved
			totalFilesOffloaded++
		}
	}

	return response(totalFilesOffloaded, uint64(totalBytesOffloaded), pb.RpcFileListOffloadResponseError_NULL, nil)
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

	at := mw.app.MustComponent(core.CName).(core.Service)
	pin := mw.app.MustComponent(pin.CName).(pin.FilePinService)

	if !at.IsStarted() {
		return response(0, pb.RpcFileOffloadResponseError_NODE_NOT_STARTED, fmt.Errorf("anytype node not started"))
	}

	pinStatus := pin.PinStatus(req.Id)
	var (
		totalBytesOffloaded uint64
	)
	for fileId, status := range pinStatus {
		if status.Status != pb2.PinStatus_Done && !req.IncludeNotPinned {
			continue
		}
		bytesRemoved, err := at.FileOffload(fileId)
		if err != nil {
			log.Errorf("failed to offload file %s: %s", fileId, err.Error())
			continue
		}
		totalBytesOffloaded += bytesRemoved
	}

	return response(uint64(totalBytesOffloaded), pb.RpcFileOffloadResponseError_NULL, nil)
}

func (mw *Middleware) FileUpload(cctx context.Context, req *pb.RpcFileUploadRequest) *pb.RpcFileUploadResponse {
	response := func(hash string, code pb.RpcFileUploadResponseErrorCode, err error) *pb.RpcFileUploadResponse {
		m := &pb.RpcFileUploadResponse{Error: &pb.RpcFileUploadResponseError{Code: code}, Hash: hash}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	var hash string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		hash, err = bs.UploadFile(*req)
		return
	})
	if err != nil {
		return response("", pb.RpcFileUploadResponseError_UNKNOWN_ERROR, err)
	}
	return response(hash, pb.RpcFileUploadResponseError_NULL, nil)
}

func (mw *Middleware) FileGetSpaceQuota(cctx context.Context, req *pb.RpcFileGetSpaceQuotaRequest) *pb.RpcFileGetSpaceQuotaResponse {
	response := func(path string, code pb.RpcFileGetSpaceQuotaResponseErrorCode, err error, stat filesync.SpaceStat) *pb.RpcFileGetSpaceQuotaResponse {
		m := &pb.RpcFileGetSpaceQuotaResponse{
			Error:      &pb.RpcFileGetSpaceQuotaResponseError{Code: code},
			FilesCount: uint32(stat.FileCount),
			CidsCount:  uint32(stat.CidsCount),
			BytesUsage: uint32(stat.BytesUsage),
			BytesLimit: uint32(stat.BytesLimit),
		}

		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	var path string

	fileSync := app.MustComponent[filesync.FileSync](mw.app)
	spaceService := app.MustComponent[space.Service](mw.app)

	stat, err := fileSync.SpaceStat(context.Background(), spaceService.AccountId())
	if err != nil {
		return response("", pb.RpcFileGetSpaceQuotaResponseError_UNKNOWN_ERROR, err, stat)
	}

	return response(path, pb.RpcFileGetSpaceQuotaResponseError_NULL, nil, stat)
}
