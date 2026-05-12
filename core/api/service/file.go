package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/svg"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	ErrFailedUploadFile   = errors.New("failed to upload file")
	ErrFailedDownloadFile = errors.New("failed to download file")
	ErrFileNotFound       = errors.New("file not found")
	ErrFailedDeleteFile   = errors.New("failed to delete file")
	ErrSpaceNotFound      = errors.New("space not found")
	ErrSpaceDeleted       = errors.New("space is deleted")
	ErrForbidden          = errors.New("forbidden")
)

// classifyUploadError maps a middleware error description into a sentinel the
// handler layer can translate to the right HTTP status. The middleware's
// FileUpload only emits UNKNOWN_ERROR for every failure, so we fall back to
// substring matching on the wrapped error string.
func classifyUploadError(description string) error {
	lower := strings.ToLower(description)
	switch {
	case strings.Contains(lower, "space not exists"),
		strings.Contains(lower, "space not found"),
		strings.Contains(lower, "no such space"):
		return fmt.Errorf("%w: %s", ErrSpaceNotFound, description)
	case strings.Contains(lower, "space is deleted"):
		return fmt.Errorf("%w: %s", ErrSpaceDeleted, description)
	case strings.Contains(lower, "read only"),
		strings.Contains(lower, "permission"),
		strings.Contains(lower, "forbidden"):
		return fmt.Errorf("%w: %s", ErrForbidden, description)
	default:
		return fmt.Errorf("%w: %s", ErrFailedUploadFile, description)
	}
}

// FileContent bundles everything a handler needs to stream a file response.
type FileContent struct {
	Reader   io.ReadSeeker
	MimeType string
	Name     string
	ModTime  int64
}

// GetFileContent fetches a file by its object ID (or raw file CID) and returns
// a streaming reader plus the metadata required to serve a proper HTTP
// response. When the file is an image and width > 0, a pre-rendered variant
// at that pixel width is returned (best-effort). SVG images are always run
// through the sanitization pipeline. Non-image files ignore width.
func (s *Service) GetFileContent(ctx context.Context, objectId string, width int) (*FileContent, error) {
	if s.fileObjectService == nil {
		return nil, fmt.Errorf("%w: file service not available", ErrFailedDownloadFile)
	}

	ctx = rpcstore.ContextWithWaitAvailable(ctx)

	// Try the image pipeline first — it handles width variants and SVG
	// sanitization. If the object isn't an image, fall through to the
	// generic file pipeline.
	if img, err := s.fetchImage(ctx, objectId); err == nil {
		return s.serveImage(ctx, img, width)
	}

	file, err := s.fileObjectService.GetFileData(ctx, objectId)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrFileNotFound, err.Error())
	}

	reader, err := file.Reader(ctx)
	if err != nil {
		// Stale cache after a hard delete: GetFileData succeeds but the
		// underlying blob is gone. Surface as 404 rather than 500.
		return nil, fmt.Errorf("%w: %s", ErrFileNotFound, err.Error())
	}

	meta := file.Meta()
	return &FileContent{
		Reader:   reader,
		MimeType: meta.Media,
		Name:     meta.Name,
		ModTime:  meta.LastModifiedDate,
	}, nil
}

func (s *Service) fetchImage(ctx context.Context, id string) (files.Image, error) {
	if domain.IsFileId(id) {
		return s.fileObjectService.GetImageDataFromRawId(ctx, domain.FileId(id))
	}
	return s.fileObjectService.GetImageData(ctx, id)
}

func (s *Service) serveImage(ctx context.Context, img files.Image, width int) (*FileContent, error) {
	orig, err := img.GetOriginalFile()
	if err != nil {
		// A stale cached smartblock can pass the GetImageData step but fail
		// once we actually reach for the underlying file (blob offloaded by
		// hard delete). Treat that as a clean miss.
		return nil, fmt.Errorf("%w: get original file: %s", ErrFileNotFound, err.Error())
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
		variant, err := img.GetFileForWidth(width)
		if err != nil {
			return nil, fmt.Errorf("%w: get image variant: %s", ErrFailedDownloadFile, err.Error())
		}
		file = variant
	}

	reader, err := file.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrFileNotFound, err.Error())
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
		return nil, classifyUploadError(resp.Error.Description)
	}

	details := domain.NewDetailsFromProto(resp.Details)
	return &apimodel.FileUploadResponse{
		ObjectId:    resp.ObjectId,
		Name:        details.GetString(bundle.RelationKeyName),
		Media:       details.GetString(bundle.RelationKeyFileMimeType),
		Extension:   details.GetString(bundle.RelationKeyFileExt),
		SizeInBytes: details.GetInt64(bundle.RelationKeySizeInBytes),
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
