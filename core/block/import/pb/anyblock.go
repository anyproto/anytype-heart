package pb

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	bundleconvert "github.com/anyproto/any-block/bundle/convert"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/source"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var anyBlockLog = logging.Logger("import-anyblock")

// anyBlockBundle recognizes a v2 bundle before the legacy importer treats its
// index and property dictionary as protobuf snapshots. Conversion is completed
// before handing the native snapshots to the existing import pipeline.
func (p *Pb) anyBlockBundle(ctx context.Context, input string) (source.Source, bool, error) {
	info, err := os.Stat(input)
	if err != nil {
		// Let the legacy source report the path error.
		return nil, false, nil
	}
	var fsys fs.FS
	var closeSource func() error
	switch {
	case info.IsDir():
		root, err := os.OpenRoot(input)
		if err != nil {
			return nil, false, err
		}
		fsys, closeSource = root.FS(), root.Close
	case strings.EqualFold(filepath.Ext(input), ".zip"):
		archive, err := zip.OpenReader(input)
		if err != nil {
			return nil, false, err
		}
		fsys, closeSource = &archive.Reader, archive.Close
	default:
		return nil, false, nil
	}
	keepSource := false
	defer func() {
		if !keepSource {
			_ = closeSource()
		}
	}()
	fsys, found, err := findAnyBlockRoot(fsys)
	if err != nil || !found {
		return nil, found, err
	}
	// Reading through this wrapper also checks cancellation during validation.
	fsys = &cancelableBundleFS{FS: fsys, check: func() error {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%w: %w", common.ErrCancel, err)
		}
		if err := p.progress.TryStep(0); err != nil {
			return common.ErrCancel
		}
		return nil
	}}
	result, err := bundleconvert.Bundle(fsys, bundleconvert.Options{
		SpaceID:   p.spaceID,
		OnWarning: func(message string) { anyBlockLog.Warnf("%s: %s", input, message) },
	})
	if err != nil {
		return nil, true, fmt.Errorf("convert AnyBlock v2 bundle: %w", err)
	}
	keepSource = true
	return &snapshotSource{entries: result.Entries, files: result.Files, fsys: fsys, close: closeSource, unresolved: result.Unresolved}, true, nil
}

// Archives created by file managers often wrap the bundle in one directory.
func findAnyBlockRoot(fsys fs.FS) (fs.FS, bool, error) {
	for {
		_, err := fs.Stat(fsys, anyblockjson.IndexFileName)
		if err == nil {
			return fsys, true, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, false, err
		}
		entries, err := fs.ReadDir(fsys, ".")
		if err != nil {
			return nil, false, err
		}
		directory := ""
		for _, entry := range entries {
			if entry.Name() == "__MACOSX" || entry.Name() == ".DS_Store" {
				continue
			}
			if !entry.IsDir() || directory != "" {
				return nil, false, nil
			}
			directory = entry.Name()
		}
		if directory == "" {
			return nil, false, nil
		}
		fsys, err = fs.Sub(fsys, directory)
		if err != nil {
			return nil, false, err
		}
	}
}

type cancelableBundleFS struct {
	fs.FS
	check func() error
}

func (f *cancelableBundleFS) Open(name string) (fs.File, error) {
	if err := f.check(); err != nil {
		return nil, err
	}
	return f.FS.Open(name)
}

func (p *Pb) anyBlockDocument(data []byte) (*common.SnapshotModel, bool, error) {
	var header map[string]json.RawMessage
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, false, nil
	}
	// Older AnyBlock documents used a numeric version marker. Native v1
	// protobuf JSON carries its data under snapshot and keeps its own reader.
	if header["formatVersion"] == nil && (header["version"] == nil || header["snapshot"] != nil) {
		return nil, false, nil
	}
	var referenceErr error
	opts := anyblockjson.Options{
		SpaceId: p.spaceID,
		OnWarning: func(issue anyblockjson.Issue) {
			anyBlockLog.Warnf("%s", issue)
			switch issue.Code {
			case anyblockjson.IssueCodeFoldedParticipantsWithoutSpace, anyblockjson.IssueCodeFoldedTypesWithoutResolver:
				referenceErr = fmt.Errorf("unresolved AnyBlock reference: %s", issue)
			}
		},
	}
	// A standalone file declares its own type ID. The native import pipeline
	// relinks that ID and references to it together when it installs the type.
	var localType standaloneTypeReferences
	if p.spaceID != "" && json.Unmarshal(header["id"], &localType.id) == nil && strings.HasPrefix(localType.id, "type-") {
		// The codec validates that the kind and internal_key own this reserved ID.
		opts.ResolveProperties = &localType
	}
	kind, snapshot, err := anyblockjson.Unmarshal(data, opts)
	if err != nil {
		return nil, true, err
	}
	if referenceErr != nil {
		return nil, true, referenceErr
	}
	if localType.missing != "" {
		return nil, true, fmt.Errorf("unresolved AnyBlock reference: %s is outside this standalone type file", localType.missing)
	}
	result, err := common.NewSnapshotModelFromProto(&pb.SnapshotWithType{SbType: kind, Snapshot: &pb.ChangeSnapshot{Data: snapshot}})
	return result, true, err
}

// A single type file has no property dictionary or external type definitions.
type standaloneTypeReferences struct {
	id      string
	missing string
}

func (r *standaloneTypeReferences) TypeIdByKey(key string) (string, bool) {
	if r.id == "type-"+key {
		return r.id, true
	}
	r.missing = "type-" + key
	return "", false
}

func (r *standaloneTypeReferences) TypeKeyById(id string) (string, bool) {
	if id != r.id {
		return "", false
	}
	return strings.TrimPrefix(id, "type-"), true
}

func (*standaloneTypeReferences) PropertyById(string) (anyblockjson.PropertyDefinition, bool) {
	return anyblockjson.PropertyDefinition{}, false
}

func (*standaloneTypeReferences) PropertyId(anyblockjson.PropertyDefinition) (string, bool) {
	return "", false
}

// snapshotSource exposes the converted native archive without temporary files.
// File bytes stay in the input filesystem and are read only when requested.
type snapshotSource struct {
	entries map[string][]byte
	files   map[string]string
	fsys    fs.FS
	close   func() error
	// unresolved is what the bundle's index declared it could not carry, by
	// class (SPEC §2c). Deleted ids are kept verbatim through import.
	unresolved bundleconvert.UnresolvedTargets
}

func (s snapshotSource) Initialize(string) error { return nil }
func (s snapshotSource) Close() {
	if s.close != nil {
		_ = s.close()
	}
}
func (s snapshotSource) IsRootFile(name string) bool { return path.Dir(name) == "." }
func (s snapshotSource) Iterate(callback func(string, io.ReadCloser) bool) error {
	names := make([]string, 0, len(s.entries))
	for name := range s.entries {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		reader := io.NopCloser(bytes.NewReader(s.entries[name]))
		more := callback(name, reader)
		reader.Close()
		if !more {
			break
		}
	}
	return nil
}
func (s snapshotSource) ProcessFile(name string, callback func(io.ReadCloser) error) error {
	if blob, ok := s.files[name]; ok {
		reader, err := s.fsys.Open(blob)
		if err != nil {
			return err
		}
		defer reader.Close()
		return callback(reader)
	}
	data, found := s.entries[name]
	if !found {
		return fs.ErrNotExist
	}
	reader := io.NopCloser(bytes.NewReader(data))
	defer reader.Close()
	return callback(reader)
}
func (s snapshotSource) CountFilesWithGivenExtensions(extensions []string) int {
	count := 0
	for name := range s.entries {
		if slices.Contains(extensions, path.Ext(name)) {
			count++
		}
	}
	return count
}
