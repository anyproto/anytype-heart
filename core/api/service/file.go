package service

import (
	"context"
	"errors"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	ErrFailedUploadFile = errors.New("failed to upload file")
	ErrInvalidFile      = errors.New("invalid file")
)

// UploadFile uploads a file and returns the object ID.
func (s *Service) UploadFile(ctx context.Context, spaceId string, localPath string, fileType apimodel.FileType) (string, error) {
	pbType := apiFileTypeToProtoType(fileType)

	resp := s.mw.FileUpload(ctx, &pb.RpcFileUploadRequest{
		SpaceId:   spaceId,
		LocalPath: localPath,
		Type:      pbType,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcFileUploadResponseError_NULL {
		switch resp.Error.Code {
		case pb.RpcFileUploadResponseError_BAD_INPUT:
			return "", ErrInvalidFile
		default:
			return "", ErrFailedUploadFile
		}
	}

	return resp.ObjectId, nil
}

// apiFileTypeToProtoType converts an API file type to a protobuf file type.
func apiFileTypeToProtoType(t apimodel.FileType) model.BlockContentFileType {
	switch t {
	case apimodel.FileTypeFile:
		return model.BlockContentFile_File
	case apimodel.FileTypeImage:
		return model.BlockContentFile_Image
	case apimodel.FileTypeVideo:
		return model.BlockContentFile_Video
	case apimodel.FileTypeAudio:
		return model.BlockContentFile_Audio
	case apimodel.FileTypePDF:
		return model.BlockContentFile_PDF
	default:
		return model.BlockContentFile_None
	}
}
