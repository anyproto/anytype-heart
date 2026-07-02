package notion

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sync"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

// fileRegistry dedupes file objects within the run. Keys derive from the
// URL path only — Notion re-signs the same S3 object with fresh query
// params per response, and both must resolve to one file object. The
// registry (and the temp dir) is run-scoped: no cross-run reuse (v1's
// global temp dir silently served stale bytes from previous imports).
type fileRegistry struct {
	mu   sync.Mutex
	seen map[string]string // url path → source key
	next int
}

func newFileRegistry() *fileRegistry {
	return &fileRegistry{seen: map[string]string{}}
}

func (r *fileRegistry) sourceKeyFor(rawUrl string) (sourceKey string, created bool) {
	identity := rawUrl
	if parsed, err := url.Parse(rawUrl); err == nil {
		identity = parsed.Host + parsed.Path
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if key, ok := r.seen[identity]; ok {
		return key, false
	}
	r.next++
	key := "file:" + shortHash(identity)
	r.seen[identity] = key
	return key, true
}

// emitFileFromUrl registers (and on first sight emits) the file object for
// a remote URL. The download is lazy: it runs inside the persist worker via
// FileSource.Open, in parallel with conversion, into the run's temp dir —
// with the fetcher's bounded retries. Returns the reference source key.
func (c *Converter) emitFileFromUrl(ctx context.Context, sink importv2.Sink, rawUrl, name string) (string, error) {
	sourceKey, created := c.files.sourceKeyFor(rawUrl)
	if !created {
		return sourceKey, nil
	}
	if name == "" {
		name = displayNameOf(rawUrl)
	}
	object := &importv2.Object{
		SourceKey: sourceKey,
		SbType:    coresb.SmartBlockTypeFileObject,
		Payload:   &importv2.Snapshot{Details: domain.NewDetails()},
		File: &importv2.FileSource{
			Name: name,
			URL:  rawUrl,
			Open: c.downloadOpener(sourceKey, name, rawUrl),
		},
	}
	return sourceKey, sink.Object(ctx, object)
}

// downloadOpener downloads the url into the run temp dir (resettable file,
// so mid-body retries work) and hands back a reader over the result.
func (c *Converter) downloadOpener(sourceKey, name, rawUrl string) func(ctx context.Context) (io.ReadCloser, error) {
	return func(ctx context.Context) (io.ReadCloser, error) {
		dstPath := filepath.Join(c.tempDir, shortHash(sourceKey)+"_"+sanitizeBase(name))
		dst, err := os.Create(dstPath)
		if err != nil {
			return nil, fmt.Errorf("create download target: %w", err)
		}
		if err := c.fetcher.Fetch(ctx, rawUrl, &resettableFile{File: dst}); err != nil {
			dst.Close()
			os.Remove(dstPath)
			return nil, fmt.Errorf("download %q: %w", name, err)
		}
		if _, err := dst.Seek(0, io.SeekStart); err != nil {
			dst.Close()
			return nil, fmt.Errorf("rewind download: %w", err)
		}
		return dst, nil
	}
}

// resettableFile lets the fetcher restart a partially written download.
type resettableFile struct {
	*os.File
}

func (f *resettableFile) Reset() error {
	if err := f.Truncate(0); err != nil {
		return err
	}
	_, err := f.Seek(0, io.SeekStart)
	return err
}

func displayNameOf(rawUrl string) string {
	if parsed, err := url.Parse(rawUrl); err == nil {
		if base := path.Base(parsed.Path); base != "." && base != "/" {
			return base
		}
	}
	return "file"
}

func sanitizeBase(name string) string {
	base := filepath.Base(filepath.FromSlash(name))
	if base == "." || base == string(filepath.Separator) {
		return "file"
	}
	return base
}
