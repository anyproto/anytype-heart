package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/svg"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ErrFailedUploadFile   = errors.New("failed to upload file")
	ErrFailedDownloadFile = errors.New("failed to download file")
	ErrFileNotFound       = errors.New("file not found")
	ErrFailedDeleteFile   = errors.New("failed to delete file")
)

// FileContent bundles everything a handler needs to stream a file response.
type FileContent struct {
	Reader   io.ReadSeeker
	MimeType string
	Name     string
	ModTime  int64
}

// GetFileContent fetches a file by its object ID and returns a streaming
// reader plus the metadata required to serve a proper HTTP response.
func (s *Service) GetFileContent(ctx context.Context, objectId string) (*FileContent, error) {
	if s.fileObjectService == nil {
		return nil, fmt.Errorf("%w: file service not available", ErrFailedDownloadFile)
	}

	ctx = rpcstore.ContextWithWaitAvailable(ctx)
	file, err := s.fileObjectService.GetFileData(ctx, objectId)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrFileNotFound, err.Error())
	}

	reader, err := file.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrFailedDownloadFile, err.Error())
	}

	meta := file.Meta()
	return &FileContent{
		Reader:   reader,
		MimeType: meta.Media,
		Name:     meta.Name,
		ModTime:  meta.LastModifiedDate,
	}, nil
}

// GetImageContent fetches an image by ID with optional width-based variant
// selection. SVG images go through a sanitization pipeline identical to the
// gateway's behavior. The id may be either a file object ID or a raw file CID.
func (s *Service) GetImageContent(ctx context.Context, imageId string, width int) (*FileContent, error) {
	if s.fileObjectService == nil {
		return nil, fmt.Errorf("%w: file service not available", ErrFailedDownloadFile)
	}

	ctx = rpcstore.ContextWithWaitAvailable(ctx)

	var (
		img files.Image
		err error
	)
	if domain.IsFileId(imageId) {
		img, err = s.fileObjectService.GetImageDataFromRawId(ctx, domain.FileId(imageId))
	} else {
		img, err = s.fileObjectService.GetImageData(ctx, imageId)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrFileNotFound, err.Error())
	}

	orig, err := img.GetOriginalFile()
	if err != nil {
		return nil, fmt.Errorf("%w: get original file: %s", ErrFailedDownloadFile, err.Error())
	}

	if filepath.Ext(orig.Name()) == constant.SvgExt {
		reader, mimeType, err := svg.ProcessSvg(ctx, orig)
		if err != nil {
			return nil, fmt.Errorf("%w: process svg: %s", ErrFailedDownloadFile, err.Error())
		}
		meta := orig.Meta()
		return &FileContent{
			Reader:   reader,
			MimeType: mimeType,
			Name:     meta.Name,
			ModTime:  meta.LastModifiedDate,
		}, nil
	}

	file := orig
	if width > 0 {
		file, err = img.GetFileForWidth(width)
		if err != nil {
			return nil, fmt.Errorf("%w: get image variant: %s", ErrFailedDownloadFile, err.Error())
		}
	}

	reader, err := file.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrFailedDownloadFile, err.Error())
	}

	meta := file.Meta()
	return &FileContent{
		Reader:   reader,
		MimeType: file.MimeType(),
		Name:     meta.Name,
		ModTime:  meta.LastModifiedDate,
	}, nil
}

// UploadFile uploads a file to the specified space
func (s *Service) UploadFile(ctx context.Context, spaceId string, localPath string) (*apimodel.FileUploadResponse, error) {
	req := &pb.RpcFileUploadRequest{
		SpaceId:   spaceId,
		LocalPath: localPath,
		Type:      model.BlockContentFile_None,
	}

	resp := s.mw.FileUpload(ctx, req)
	if resp.Error != nil && resp.Error.Code != pb.RpcFileUploadResponseError_NULL {
		return nil, fmt.Errorf("%w: %s", ErrFailedUploadFile, resp.Error.Description)
	}

	// Convert details from proto Struct to map
	details := pbtypes.ToMap(resp.Details)

	return &apimodel.FileUploadResponse{
		ObjectId: resp.ObjectId,
		FileId:   resp.PreloadFileId,
		Details:  details,
	}, nil
}

// DeleteFile removes a file object. By default it is moved to the bin
// (archived); when skipBin is true the object is permanently deleted.
func (s *Service) DeleteFile(ctx context.Context, spaceId string, fileId string, skipBin bool) error {
	if skipBin {
		resp := s.mw.ObjectListDelete(ctx, &pb.RpcObjectListDeleteRequest{
			ObjectIds: []string{fileId},
		})
		if resp.Error != nil && resp.Error.Code != pb.RpcObjectListDeleteResponseError_NULL {
			return fmt.Errorf("%w: %s", ErrFailedDeleteFile, resp.Error.Description)
		}
		return nil
	}

	resp := s.mw.ObjectSetIsArchived(ctx, &pb.RpcObjectSetIsArchivedRequest{
		ContextId:  fileId,
		IsArchived: true,
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetIsArchivedResponseError_NULL {
		return fmt.Errorf("%w: %s", ErrFailedDeleteFile, resp.Error.Description)
	}
	return nil
}
