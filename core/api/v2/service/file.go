package v2service

// file.go implements POST /v2/spaces/{space_id}/files (APIV2.md §2 —
// load-bearing, R11): upload by multipart (staged to a local path by the
// handler) or by URL, returning the file object id that file/image blocks
// and iconImage values need.

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileuploader"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// UploadFile uploads one file into the space, from a staged local path
// (multipart upload) or a URL — exactly one must be set.
func (s *Service) UploadFile(ctx context.Context, spaceId, localPath, url string, dryRun bool) (*v2model.FileUploadResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	if (localPath == "") == (url == "") {
		return nil, v2model.ValidationFailed("provide a file or a url",
			v2model.Issue{Message: "upload multipart/form-data with a file field, or JSON {\"url\": …}"})
	}
	// the advertised url bound (M6, the file kind's maxLength)
	if err := validateV2FieldLength("/url", url, maxV2UrlLength); err != nil {
		return nil, err
	}
	if dryRun {
		return &v2model.FileUploadResult{DryRun: true}, nil
	}

	resp := s.mw.FileUpload(ctx, &pb.RpcFileUploadRequest{
		SpaceId:   spaceId,
		LocalPath: localPath,
		Url:       url,
		Type:      model.BlockContentFile_None,
		Origin:    model.ObjectOrigin_api,
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcFileUploadResponseError_NULL {
		return nil, v2FileRpcError(fmt.Sprintf("upload file to space %s", spaceId), url,
			int32(resp.Error.Code), int32(pb.RpcFileUploadResponseError_BAD_INPUT), resp.Error.Description)
	}
	details := domain.NewDetailsFromProto(resp.Details)
	return &v2model.FileUploadResult{
		Id:       resp.ObjectId,
		Name:     details.GetString(bundle.RelationKeyName),
		MimeType: details.GetString(bundle.RelationKeyFileMimeType),
		Size:     details.GetInt64(bundle.RelationKeySizeInBytes),
	}, nil
}

// v2FileRpcError classifies a FileUpload RPC failure into the C6 shape.
// The middleware has exactly ONE error branch for the whole upload — always
// UNKNOWN_ERROR (core/file.go) — so, like chat and spaces, the
// classification rides the error description (surface review M2c); without
// it every failed upload was a retry-looping 500. The URL-mode arms are
// permanent refusals of the caller-supplied url → 400 with the /url path:
//   - the uploader's non-2xx source refusal, matched through the
//     fileuploader.ErrFailedToDownload sentinel itself so a producer
//     rewording updates this matcher at compile time;
//   - the net/http fetch failure (`Get "…": …` — url.Error's fixed
//     rendering, pinned by TestV2UploadFile against the stdlib type;
//     CleanupError masks the URL inside but keeps the shape) covering DNS,
//     connection and TLS failures on the way to the source.
//
// Anything else — storage faults, local-path staging failures — stays a
// 500 carrying the description.
func v2FileRpcError(op, url string, code, badInputCode int32, description string) error {
	if code == badInputCode {
		return v2model.ValidationFailed(fmt.Sprintf("%s: invalid input", op),
			v2model.Issue{Message: description})
	}
	if url != "" {
		switch {
		case strings.Contains(description, fileuploader.ErrFailedToDownload.Error()):
			return v2model.ValidationFailed(fmt.Sprintf("%s: the source URL did not yield the file", op),
				v2model.Issue{Path: "/url", Message: description,
					Hint: "the URL must answer 2xx with the file bytes — check it in a browser first"})
		case strings.Contains(description, `Get "`):
			return v2model.ValidationFailed(fmt.Sprintf("%s: the source URL could not be fetched", op),
				v2model.Issue{Path: "/url", Message: description,
					Hint: "the host did not answer (DNS, connection or TLS failure) — check the URL"})
		}
	}
	msg := op + " failed"
	if description != "" {
		msg += ": " + description
	}
	return v2model.NewError(http.StatusInternalServerError, v2model.CodeInternalError, msg)
}
