package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// copyBundleFiles copies a bundle's files/ directory — the real binary
// assets a "fileObject" document's "source" property points at (SPEC.md
// §2c, §3: iconImage and any object-level image/file reference resolve by
// looking up that property) — into the output archive at the same relative
// path.
//
// anyblockconvert only ever discovers *.json documents
// (anyblockbatch.DiscoverJSONFiles), and anyblockinstall.sh's zip step only
// packs whatever this tool wrote to outDir. Neither step ever reads raw
// bytes off disk on its own, so without this, a bundle authoring
// "source": "files/icon.png" against a real PNG converts clean and installs
// with no icon: core/block/import/pb's normalizeFilePath resolves that path
// against the *archive* being imported, not the source bundle, and finds
// nothing there.
func copyBundleFiles(inDir, outDir string) (int, error) {
	src := filepath.Join(inDir, "files")
	info, err := os.Stat(src)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("stat %s: %w", src, err)
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("%s exists but is not a directory", src)
	}

	dst := filepath.Join(outDir, "files")
	var n int
	walkErr := filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() || fi.Name() == ".DS_Store" {
			return nil
		}
		rel, relErr := filepath.Rel(src, p)
		if relErr != nil {
			return relErr
		}
		if copyErr := copyFile(p, filepath.Join(dst, rel)); copyErr != nil {
			return fmt.Errorf("copy %s: %w", rel, copyErr)
		}
		n++
		return nil
	})
	return n, walkErr
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// danglingSource is a document whose "source" property (SPEC.md §3: the
// property a fileObject's real bytes are found by) names a files/ path that
// does not exist in the bundle. Exactly the class of bug CheckTargetTypes
// catches for object references: valid JSON, valid schema, silently wrong
// once installed — here it's a blank icon or a dead file block instead of a
// dangling id.
type danglingSource struct {
	file, source string
}

// checkFileSources scans every document's top-level "properties.source" for
// a files/-relative path and confirms it exists under inDir. It only looks
// at the object-level "source" property (the one iconImage/fileObject
// resolution reads via bundle.RelationKeySource) — not block-level file
// content, which carries its own objectId/hash instead.
func checkFileSources(inDir string, files []string) ([]danglingSource, error) {
	var out []danglingSource
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		var doc struct {
			Properties map[string]any `json:"properties"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			// malformed JSON is reported by the real schema validation this
			// tool already runs elsewhere; skip it here rather than double-report
			continue
		}
		src, ok := doc.Properties["source"].(string)
		if !ok || !strings.HasPrefix(src, "files/") {
			continue
		}
		if _, statErr := os.Stat(filepath.Join(inDir, filepath.FromSlash(src))); os.IsNotExist(statErr) {
			out = append(out, danglingSource{file: f, source: src})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].file < out[j].file })
	return out, nil
}

func reportDanglingSources(ds []danglingSource) string {
	var b strings.Builder
	for _, d := range ds {
		fmt.Fprintf(&b, "  %s: source %q names no file in the bundle's files/ directory\n", d.file, d.source)
	}
	return b.String()
}
