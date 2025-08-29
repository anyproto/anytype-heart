package core

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files/filedownloader"
	"github.com/anyproto/anytype-heart/core/files/fileoffloader"
	"github.com/anyproto/anytype-heart/core/files/filespaceusage"
	"github.com/anyproto/anytype-heart/core/files/reconciler"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (mw *Middleware) FileDownload(cctx context.Context, req *pb.RpcFileDownloadRequest) *pb.RpcFileDownloadResponse {
	response := func(path string, code pb.RpcFileDownloadResponseErrorCode, err error) *pb.RpcFileDownloadResponse {
		m := &pb.RpcFileDownloadResponse{Error: &pb.RpcFileDownloadResponseError{Code: code}, LocalPath: path}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	var path string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		path, err = bs.DownloadFile(cctx, req)
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
			m.Error.Description = getErrorDescription(err)
		} else {
			m.Event = mw.getResponseEvent(ctx)
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
	fileOffloader := mustService[fileoffloader.Service](mw)
	err := fileOffloader.FilesOffload(cctx, req.OnlyIds, req.IncludeNotPinned)
	if err != nil {
		return &pb.RpcFileListOffloadResponse{
			Error: &pb.RpcFileListOffloadResponseError{
				Code:        pb.RpcFileListOffloadResponseError_UNKNOWN_ERROR,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcFileListOffloadResponse{
		Error: &pb.RpcFileListOffloadResponseError{
			Code: pb.RpcFileListOffloadResponseError_NULL,
		},
	}
}

func (mw *Middleware) FileOffload(cctx context.Context, req *pb.RpcFileOffloadRequest) *pb.RpcFileOffloadResponse {
	response := func(bytesOffloaded uint64, code pb.RpcFileOffloadResponseErrorCode, err error) *pb.RpcFileOffloadResponse {
		m := &pb.RpcFileOffloadResponse{BytesOffloaded: bytesOffloaded, Error: &pb.RpcFileOffloadResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}

		return m
	}

	if mw.applicationService.GetApp() == nil {
		return response(0, pb.RpcFileOffloadResponseError_NODE_NOT_STARTED, fmt.Errorf("anytype is nil"))
	}

	fileOffloader := mustService[fileoffloader.Service](mw)
	bytesRemoved, err := fileOffloader.FileOffload(cctx, req.Id, req.IncludeNotPinned)
	if err != nil {
		log.Errorf("failed to offload file %s: %s", req.Id, err)
	}

	return response(bytesRemoved, pb.RpcFileOffloadResponseError_NULL, nil)
}

func (mw *Middleware) FileSpaceOffload(cctx context.Context, req *pb.RpcFileSpaceOffloadRequest) *pb.RpcFileSpaceOffloadResponse {
	response := func(filesOffloaded int, bytesOffloaded uint64, code pb.RpcFileSpaceOffloadResponseErrorCode, err error) *pb.RpcFileSpaceOffloadResponse {
		m := &pb.RpcFileSpaceOffloadResponse{
			FilesOffloaded: int32(filesOffloaded),
			BytesOffloaded: bytesOffloaded,
			Error:          &pb.RpcFileSpaceOffloadResponseError{Code: code},
		}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}

		return m
	}

	fileOffloader := mustService[fileoffloader.Service](mw)
	filesOffloaded, bytesRemoved, err := fileOffloader.FileSpaceOffload(cctx, req.SpaceId, false)
	if err != nil {
		return response(0, 0, pb.RpcFileSpaceOffloadResponseError_UNKNOWN_ERROR, err)
	}
	return response(filesOffloaded, bytesRemoved, pb.RpcFileSpaceOffloadResponseError_NULL, nil)
}

func (mw *Middleware) FileUpload(cctx context.Context, req *pb.RpcFileUploadRequest) *pb.RpcFileUploadResponse {
	response := func(objectId string, details *types.Struct, code pb.RpcFileUploadResponseErrorCode, err error) *pb.RpcFileUploadResponse {
		m := &pb.RpcFileUploadResponse{Error: &pb.RpcFileUploadResponseError{Code: code}, ObjectId: objectId, Details: details}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	var (
		objectId string
		details  *domain.Details
		fileType model.BlockContentFileType
	)
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		dto := block.FileUploadRequest{RpcFileUploadRequest: *req, ObjectOrigin: objectorigin.ObjectOrigin{Origin: req.Origin}}
		objectId, fileType, details, err = bs.UploadFile(cctx, req.SpaceId, dto)
		return
	})

	var typeKey domain.TypeKey
	switch fileType {
	case model.BlockContentFile_Audio:
		typeKey = bundle.TypeKeyAudio
	case model.BlockContentFile_Image:
		typeKey = bundle.TypeKeyImage
	case model.BlockContentFile_Video:
		typeKey = bundle.TypeKeyVideo
	case model.BlockContentFile_PDF, model.BlockContentFile_File:
		typeKey = bundle.TypeKeyFile
	default:

	}
	if typeKey != "" {
		// do not create widget if type is not detected. Shouldn't happen, but just in case
		err := mw.doBlockService(func(bs *block.Service) (err error) {
			err = bs.CreateTypeWidgetIfMissing(cctx, req.SpaceId, typeKey)
			return err
		})
		if err != nil {
			return response(objectId, nil, pb.RpcFileUploadResponseError_UNKNOWN_ERROR, err)
		}
	}

	if err != nil {
		return response("", nil, pb.RpcFileUploadResponseError_UNKNOWN_ERROR, err)
	}
	return response(objectId, details.ToProto(), pb.RpcFileUploadResponseError_NULL, nil)
}

func (mw *Middleware) FileSpaceUsage(cctx context.Context, req *pb.RpcFileSpaceUsageRequest) *pb.RpcFileSpaceUsageResponse {
	response := func(code pb.RpcFileSpaceUsageResponseErrorCode, err error, usage *pb.RpcFileSpaceUsageResponseUsage) *pb.RpcFileSpaceUsageResponse {
		m := &pb.RpcFileSpaceUsageResponse{
			Error: &pb.RpcFileSpaceUsageResponseError{Code: code},
			Usage: usage,
		}

		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}

	usage, err := mustService[filespaceusage.Service](mw).GetSpaceUsage(cctx, req.SpaceId)
	if err != nil {
		return response(pb.RpcFileSpaceUsageResponseError_UNKNOWN_ERROR, err, nil)
	}
	return response(pb.RpcFileSpaceUsageResponseError_NULL, nil, usage)
}

func (mw *Middleware) FileNodeUsage(ctx context.Context, req *pb.RpcFileNodeUsageRequest) *pb.RpcFileNodeUsageResponse {
	usage, err := mustService[filespaceusage.Service](mw).GetNodeUsage(ctx)
	code := mapErrorCode[pb.RpcFileNodeUsageResponseErrorCode](err)
	resp := &pb.RpcFileNodeUsageResponse{
		Error: &pb.RpcFileNodeUsageResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
	if usage != nil {
		resp.Usage = &pb.RpcFileNodeUsageResponseUsage{
			CidsCount:       uint64(usage.Usage.TotalCidsCount),
			BytesUsage:      uint64(usage.Usage.TotalBytesUsage),
			BytesLimit:      uint64(usage.Usage.AccountBytesLimit),
			BytesLeft:       usage.Usage.BytesLeft,
			LocalBytesUsage: usage.LocalUsageBytes,
		}

		resp.Spaces = make([]*pb.RpcFileNodeUsageResponseSpace, 0, len(usage.Usage.Spaces))
		for _, space := range usage.Usage.Spaces {
			resp.Spaces = append(resp.Spaces, &pb.RpcFileNodeUsageResponseSpace{
				SpaceId:    space.SpaceId,
				FilesCount: uint64(space.FileCount),
				CidsCount:  uint64(space.CidsCount),
				BytesUsage: uint64(space.SpaceBytesUsage),
			})
		}
	}

	return resp
}

func (mw *Middleware) FileReconcile(ctx context.Context, req *pb.RpcFileReconcileRequest) *pb.RpcFileReconcileResponse {
	err := mustService[reconciler.Reconciler](mw).Start(ctx)
	if err != nil {
		return &pb.RpcFileReconcileResponse{
			Error: &pb.RpcFileReconcileResponseError{
				Code:        mapErrorCode[pb.RpcFileReconcileResponseErrorCode](err),
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcFileReconcileResponse{}
}

func (mw *Middleware) FileSetAutoDownload(ctx context.Context, req *pb.RpcFileSetAutoDownloadRequest) *pb.RpcFileSetAutoDownloadResponse {
	mustService[filedownloader.Service](mw).SetEnabled(req.Enabled)
	return &pb.RpcFileSetAutoDownloadResponse{}
}
