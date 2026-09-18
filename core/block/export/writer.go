package export

import (
	"archive/zip"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gosimple/slug"

	"github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/util/anyerror"
)

type writer interface {
	Path() string
	Namer() Namer
	WriteFile(filename string, r io.Reader, lastModifiedDate int64) (err error)
	Close() (err error)
}

type Namer interface {
	Get(path, hash, title, ext string) (name string)
}

// exportNamesMaxLength includes the separator between the space and object.
const exportNamesMaxLength = 100

func makeExportName(spaceName, objectName string, date time.Time) string {
	names := normalizeExportName(spaceName)
	if objectName != "" {
		objectName = normalizeExportName(objectName)
		budget := exportNamesMaxLength - 1
		// Share the budget when both names are long, and give unused space
		// from a short name to the other one.
		spaceLimit := min(len(names), max(budget/2, budget-len(objectName)))
		objectLimit := min(len(objectName), budget-spaceLimit)
		names = strings.TrimRight(names[:spaceLimit], "-") + "-" +
			strings.TrimRight(objectName[:objectLimit], "-")
	} else if len(names) > exportNamesMaxLength {
		names = strings.TrimRight(names[:exportNamesMaxLength], "-")
	}
	return "anytype-" + names + "-" + date.Format("2006-01-02-150405.000")
}

func normalizeExportName(name string) string {
	name = slug.Make(name)
	if name == "" {
		return defaultFileName
	}
	return name
}

func newDirWriter(path, name string, includeFiles bool) (writer, error) {
	path = filepath.Join(path, name)
	fullPath := path
	if includeFiles {
		fullPath = filepath.Join(path, "files")
	}
	if err := os.MkdirAll(fullPath, 0777); err != nil {
		return nil, fmt.Errorf("create export directory: %w", err)
	}
	return &dirWriter{
		path: path,
	}, nil
}

type dirWriter struct {
	path string
	fn   *namer
	m    sync.Mutex
}

func (d *dirWriter) Namer() Namer {
	d.m.Lock()
	defer d.m.Unlock()
	if d.fn == nil {
		d.fn = newNamer()
	}
	return d.fn
}

func (d *dirWriter) Path() string {
	return d.path
}

func (d *dirWriter) WriteFile(filename string, r io.Reader, lastModifiedDate int64) (err error) {
	dir := filepath.Dir(filename)
	err = os.MkdirAll(filepath.Join(d.path, dir), 0700)
	if err != nil {
		return fmt.Errorf("create subdirectory: %w", err)
	}
	filename = path.Join(d.path, filename)
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()
	if _, err = io.Copy(f, r); err != nil {
		return fmt.Errorf("copy content to file: %w", err)
	}
	if lastModifiedDate == 0 {
		lastModifiedDate = time.Now().Unix()
	}
	lastModifiedDateUnix := time.Unix(lastModifiedDate, 0)
	err = os.Chtimes(filename, time.Now(), lastModifiedDateUnix)
	if err != nil {
		return fmt.Errorf("failed to set date modified of export file: %w", anyerror.CleanupError(err))
	}
	return
}

// RemoveFile deletes one file below the export root. Nothing in the legacy
// formats calls it: it exists for the native AnyBlock JSON exporter's
// un-write hook, which reaches it through an optional interface assertion
// — a blob whose stream fails half way is worse left truncated than
// missing (core/block/export/anyblock, emitDoc). A zip export cannot offer
// the same, since its entries are already streamed.
func (d *dirWriter) RemoveFile(filename string) error {
	if err := os.Remove(filepath.Join(d.path, filepath.FromSlash(filename))); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove file: %w", err)
	}
	return nil
}

func (d *dirWriter) Close() (err error) {
	return nil
}

func newZipWriter(path, name string) (writer, error) {
	fileName := filepath.Join(path, name)
	f, err := os.Create(fileName)
	if err != nil {
		return nil, fmt.Errorf("create zip file: %w", err)
	}
	return &zipWriter{
		path: fileName,
		zw:   zip.NewWriter(f),
		f:    f,
	}, nil
}

type zipWriter struct {
	path string
	zw   *zip.Writer
	f    io.Closer
	m    sync.Mutex
	fn   *namer
}

func (d *zipWriter) Namer() Namer {
	d.m.Lock()
	defer d.m.Unlock()
	if d.fn == nil {
		d.fn = newNamer()
	}
	return d.fn
}

func (d *zipWriter) Path() string {
	return d.path
}

func (d *zipWriter) WriteFile(filename string, r io.Reader, lastModifiedDate int64) (err error) {
	d.m.Lock()
	defer d.m.Unlock()
	if lastModifiedDate == 0 {
		lastModifiedDate = time.Now().Unix()
	}
	zf, err := d.zw.CreateHeader(&zip.FileHeader{
		Name:     filename,
		Method:   zip.Deflate,
		Modified: time.Unix(lastModifiedDate, 0),
	})
	if err != nil {
		return fmt.Errorf("create zip entry: %w", err)
	}
	if _, err = io.Copy(zf, r); err != nil {
		return fmt.Errorf("copy content to zip: %w", err)
	}
	return nil
}

func (d *zipWriter) Close() (err error) {
	if err = d.zw.Close(); err != nil {
		return fmt.Errorf("close zip writer: %w", err)
	}
	if err = d.f.Close(); err != nil {
		return fmt.Errorf("close zip file: %w", err)
	}
	return nil
}

type InMemoryWriter struct {
	data map[string][]byte
	fn   Namer
	m    sync.Mutex
}

func (d *InMemoryWriter) Namer() Namer {
	return d.fn
}

func (d *InMemoryWriter) Path() string {
	return ""
}

func (d *InMemoryWriter) WriteFile(filename string, r io.Reader, lastModifiedDate int64) (err error) {
	d.m.Lock()
	defer d.m.Unlock()
	if d.data == nil {
		d.data = make(map[string][]byte)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read content: %w", err)
	}
	d.data[filename] = b
	return
}

func (d *InMemoryWriter) Close() (err error) {
	return nil
}

func (d *InMemoryWriter) GetData(id string) []byte {
	d.m.Lock()
	defer d.m.Unlock()
	return d.data[id]
}

// deepLinkNamer used to render a single-object export, in md format
type deepLinkNamer struct {
	gatewayUrl url.URL
	spaceId    string
}

func (fn *deepLinkNamer) Get(path, hash, title, ext string) (name string) {
	if ext == ".md" {
		// object links via deeplink to the app
		return "anytype://object?objectId=" + hash + "&spaceId=" + fn.spaceId
	}

	// files links via gateway
	if fn.gatewayUrl.Host == "" {
		return "anytype://object?objectId=" + hash + "&spaceId=" + fn.spaceId
	}
	u := fn.gatewayUrl
	if mill.IsImageExt(ext) {
		u.Path = "image/" + hash
	} else {
		u.Path = "file/" + hash
	}

	return u.String()
}
