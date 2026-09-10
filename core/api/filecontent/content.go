// Package filecontent reads file and image bytes for both API versions.
// Callers must authorize the requested object or raw CID before calling Get.
package filecontent

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/svg"
)

var (
	ErrDownload = errors.New("failed to download file")
	ErrNotFound = errors.New("file not found")
)

// Content bundles everything a handler needs to stream a file response.
type Content struct {
	Reader   io.ReadSeeker
	MimeType string
	Name     string
	ModTime  int64
	ETag     string
}

// The CID identifies immutable content, while width selects its representation.
// Bump the version if the SVG sanitization or variant selection changes.
func contentETag(fileId domain.FileId, width int, mimeType string) string {
	return fmt.Sprintf(`"%x"`, sha256.Sum256([]byte(fmt.Sprintf("api-file-v1:%s:%d:%s", fileId, width, mimeType))))
}

// Get fetches a file by its object ID (or raw file CID) and returns
// a streaming reader plus the metadata required to serve a proper HTTP
// response. When the file is an image and width > 0, a pre-rendered variant
// at that pixel width is returned (best-effort). SVG images use the build's
// SVG processing pipeline. Non-image files ignore width.
func Get(ctx context.Context, fileService apicore.FileObjectService, objectId string, width int) (*Content, error) {
	if fileService == nil {
		return nil, fmt.Errorf("%w: file service not available", ErrDownload)
	}

	ctx = rpcstore.ContextWithWaitAvailable(ctx)

	// Try the image pipeline first — it handles width variants and SVG
	// processing. If the object isn't an image, fall through to the
	// generic file pipeline.
	if img, err := fetchImage(ctx, fileService, objectId); err == nil {
		content, err := serveImage(ctx, img, width)
		if err != nil {
			return nil, err
		}
		// Wrap the reader so transient block-fetch errors during streaming
		// are retried with backoff (mirrors gateway behavior). EOF is never
		// retried.
		content.Reader = newRetryReadSeeker(content.Reader, blockFetchRetryOptions(ctx)...)
		return content, nil
	}

	file, err := fileService.GetFileData(ctx, objectId)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, err.Error())
	}

	reader, err := file.Reader(ctx)
	if err != nil {
		// Stale cache after a hard delete: GetFileData succeeds but the
		// underlying blob is gone. Surface as 404 rather than 500.
		return nil, fmt.Errorf("%w: %s", ErrNotFound, err.Error())
	}

	meta := file.Meta()
	return &Content{
		Reader:   reader,
		MimeType: meta.Media,
		Name:     meta.Name,
		ModTime:  meta.LastModifiedDate,
		ETag:     contentETag(file.FileId(), 0, meta.Media),
	}, nil
}

func fetchImage(ctx context.Context, fileService apicore.FileObjectService, id string) (files.Image, error) {
	if domain.IsFileId(id) {
		return fileService.GetImageDataFromRawId(ctx, domain.FileId(id))
	}
	return fileService.GetImageData(ctx, id)
}

func serveImage(ctx context.Context, img files.Image, width int) (*Content, error) {
	orig, err := img.GetOriginalFile()
	if err != nil {
		// A stale cached smartblock can pass the GetImageData step but fail
		// once we actually reach for the underlying file (blob offloaded by
		// hard delete). Treat that as a clean miss.
		return nil, fmt.Errorf("%w: get original file: %s", ErrNotFound, err.Error())
	}

	if filepath.Ext(orig.Name()) == constant.SvgExt {
		reader, mimeType, err := svg.ProcessSvg(ctx, orig)
		if err != nil {
			return nil, fmt.Errorf("%w: process svg: %s", ErrDownload, err.Error())
		}
		meta := orig.Meta()
		return &Content{
			Reader:   reader,
			MimeType: mimeType,
			Name:     meta.Name,
			ModTime:  meta.LastModifiedDate,
			ETag:     contentETag(orig.FileId(), 0, mimeType),
		}, nil
	}

	file := orig
	if width > 0 {
		variant, err := img.GetFileForWidth(width)
		if err != nil {
			return nil, fmt.Errorf("%w: get image variant: %s", ErrDownload, err.Error())
		}
		file = variant
	}

	reader, err := file.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, err.Error())
	}

	meta := file.Meta()
	mimeType := file.MimeType()
	return &Content{
		Reader:   reader,
		MimeType: mimeType,
		Name:     meta.Name,
		ModTime:  meta.LastModifiedDate,
		ETag:     contentETag(file.FileId(), width, mimeType),
	}, nil
}
