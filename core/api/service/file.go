package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/anytype-heart/pb"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ErrFailedUploadFile = errors.New("failed to upload file")
)

// UploadFile uploads a file to the specified space
func (s *Service) UploadFile(ctx context.Context, spaceId string, localPath string) (*apimodel.FileUploadResponse, error) {
	req := &pb.RpcFileUploadRequest{
		SpaceId:   spaceId,
		LocalPath: localPath,
		Type:      model.BlockContentFile_File, // default to generic file
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
