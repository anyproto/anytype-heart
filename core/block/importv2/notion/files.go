package notion

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sync"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
	"github.com/anyproto/anytype-heart/core/domain"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

// fileRegistry dedupes file objects within the run. For Notion-hosted files
// keys derive from the URL path only — Notion re-signs the same S3 object
// with fresh query params per response, and both must resolve to one file
// object. External URLs keep their query: it can be the only thing that
// distinguishes two files (drive.google.com/uc?id=A vs ?id=B). The registry
// (and the temp dir) is run-scoped: no cross-run reuse (v1's global temp
// dir silently served stale bytes from previous imports).
type fileRegistry struct {
	mu   sync.Mutex
	seen map[string]string // url identity → source key
	next int
}

func newFileRegistry() *fileRegistry {
	return &fileRegistry{seen: map[string]string{}}
}

func (r *fileRegistry) sourceKeyFor(rawUrl string, external bool) (sourceKey string, created bool) {
	identity := rawUrl
	if parsed, err := url.Parse(rawUrl); err == nil {
		identity = parsed.Host + parsed.Path
		if external && parsed.RawQuery != "" {
			identity += "?" + parsed.RawQuery
		}
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

// urlRefresher re-mints an expired pre-signed URL from its source (block or
// entity). An empty result means the source no longer offers one.
type urlRefresher func(ctx context.Context) (string, error)

// emitFileFromUrl registers (and on first sight emits) the file object for
// a remote URL. The download is lazy: it runs inside the persist worker via
// FileSource.Open, in parallel with conversion, into the run's temp dir —
// with the fetcher's bounded retries. Returns the reference source key.
func (c *Converter) emitFileFromUrl(ctx context.Context, sink importv2.Sink, rawUrl, name string, external bool, refresh urlRefresher) (string, error) {
	sourceKey, created := c.files.sourceKeyFor(rawUrl, external)
	if !created {
		return sourceKey, nil
	}
	if name == "" {
		name = displayNameOf(rawUrl)
	}
	if external {
		// External URLs are not signed; a 403 there is a real denial.
		refresh = nil
	}
	object := &importv2.Object{
		SourceKey: sourceKey,
		SbType:    coresb.SmartBlockTypeFileObject,
		Payload:   &importv2.Snapshot{Details: domain.NewDetails()},
		File: &importv2.FileSource{
			Name: name,
			URL:  rawUrl,
			Open: c.downloadOpener(sourceKey, name, rawUrl, refresh),
		},
	}
	return sourceKey, sink.Object(ctx, object)
}

// downloadOpener downloads the url into the run temp dir (resettable file,
// so mid-body retries work) and hands back a reader over the result.
//
// Notion-signed URLs expire (~1h) and the string is captured at block-fetch
// time while the download runs in a persist worker much later — on a long
// import the gap exceeds the expiry. An expiry-shaped failure
// re-mints the URL from its source once and retries.
func (c *Converter) downloadOpener(sourceKey, name, rawUrl string, refresh urlRefresher) func(ctx context.Context) (io.ReadCloser, error) {
	return func(ctx context.Context) (io.ReadCloser, error) {
		dstPath := filepath.Join(c.tempDir, shortHash(sourceKey)+"_"+sanitizeBase(name))
		dst, err := os.Create(dstPath)
		if err != nil {
			return nil, fmt.Errorf("create download target: %w", err)
		}
		target := &resettableFile{File: dst}
		fetchErr := c.fetcher.Fetch(ctx, rawUrl, target)
		if fetchErr != nil && refresh != nil && client.IsExpiredUrl(fetchErr) {
			if freshUrl, refreshErr := refresh(ctx); refreshErr == nil && freshUrl != "" && freshUrl != rawUrl {
				if target.Reset() == nil {
					fetchErr = c.fetcher.Fetch(ctx, freshUrl, target)
				}
			}
		}
		if fetchErr != nil {
			dst.Close()
			os.Remove(dstPath)
			return nil, fmt.Errorf("download %q: %w", name, fetchErr)
		}
		if _, err := dst.Seek(0, io.SeekStart); err != nil {
			dst.Close()
			return nil, fmt.Errorf("rewind download: %w", err)
		}
		return dst, nil
	}
}

// blockUrlRefresher re-fetches the owning block and extracts a fresh signed
// URL from it. blockId must be the real Notion id (hoisted synced-block
// content carries suffixed ids).
func (c *Converter) blockUrlRefresher(blockId string, extract func(*notionBlock) string) urlRefresher {
	return func(ctx context.Context) (string, error) {
		var block notionBlock
		if err := c.client.Request(ctx, http.MethodGet, "/blocks/"+blockId, nil, &block); err != nil {
			return "", fmt.Errorf("refresh block url: %w", err)
		}
		return extract(&block), nil
	}
}

// entityUrlRefresher re-fetches a page or data source for a fresh icon/cover
// URL. fetchPath is "/pages/{id}" or "/data_sources/{id}".
func (c *Converter) entityUrlRefresher(fetchPath string, extract func(icon *iconValue, cover *fileValue) string) urlRefresher {
	return func(ctx context.Context) (string, error) {
		var payload struct {
			Icon  *iconValue `json:"icon"`
			Cover *fileValue `json:"cover"`
		}
		if err := c.client.Request(ctx, http.MethodGet, fetchPath, nil, &payload); err != nil {
			return "", fmt.Errorf("refresh entity url: %w", err)
		}
		return extract(payload.Icon, payload.Cover), nil
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
