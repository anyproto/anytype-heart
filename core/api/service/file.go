package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/core/api/filecontent"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	ErrFailedUploadFile   = errors.New("failed to upload file")
	ErrFailedDownloadFile = filecontent.ErrDownload
	ErrFileNotFound       = filecontent.ErrNotFound
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

// FileContent contains a streaming reader and its response metadata.
type FileContent = filecontent.Content

func (s *Service) GetFileContent(ctx context.Context, objectId string, width int) (*FileContent, error) {
	return filecontent.Get(ctx, s.fileObjectService, objectId, width)
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
