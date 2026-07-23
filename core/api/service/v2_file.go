package service

// v2_file.go implements POST /v2/spaces/{spaceId}/files (APIV2.md §2 —
// load-bearing, R11): upload by multipart (staged to a local path by the
// handler) or by URL, returning the file object id that file/image blocks
// and iconImage values need.

import (
	"context"
	"fmt"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// UploadFile uploads one file into the space, from a staged local path
// (multipart upload) or a URL — exactly one must be set.
func (s *V2Service) UploadFile(ctx context.Context, spaceId, localPath, url string, dryRun bool) (*apimodel.V2FileUploadResult, error) {
	if err := s.ensureSpace(spaceId); err != nil {
		return nil, err
	}
	if (localPath == "") == (url == "") {
		return nil, apimodel.V2ValidationFailed("provide a file or a url",
			apimodel.V2Issue{Message: "upload multipart/form-data with a file field, or JSON {\"url\": …}"})
	}
	if dryRun {
		return &apimodel.V2FileUploadResult{DryRun: true}, nil
	}

	resp := s.mw.FileUpload(ctx, &pb.RpcFileUploadRequest{
		SpaceId:   spaceId,
		LocalPath: localPath,
		Url:       url,
		Type:      model.BlockContentFile_None,
		Origin:    model.ObjectOrigin_api,
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcFileUploadResponseError_NULL {
		return nil, fmt.Errorf("upload file to space %s: %s", spaceId, resp.Error.Description)
	}
	details := domain.NewDetailsFromProto(resp.Details)
	return &apimodel.V2FileUploadResult{
		Id:       resp.ObjectId,
		Name:     details.GetString(bundle.RelationKeyName),
		MimeType: details.GetString(bundle.RelationKeyFileMimeType),
		Size:     details.GetInt64(bundle.RelationKeySizeInBytes),
	}, nil
}
