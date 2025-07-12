// Package tantivycheck provides a DRY-RUN consistency check for Tantivy index
// directories.
//
// It verifies that
//
//   - every segment listed in meta.json has files on disk
//   - every expected <segment>.<opstamp>.del file exists
//   - there are no extra segment prefixes on disk
//   - there are no extra .del files on disk
//   - the special lock files INDEX_WRITER_LOCK and META_LOCK are noted
//
// Nothing is modified on disk; you simply get a ConsistencyReport back.
package tantivycheck

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

// -----------------------------------------------------------------------------
// Package-level helpers (compiled once)
// -----------------------------------------------------------------------------

var (
	segPrefixRe = regexp.MustCompile(`^([0-9a-f]{32})(?:\..+)?$`)
	delFileRe   = regexp.MustCompile(`^([0-9a-f]{32})\.(\d+)\.del$`)
)

// -----------------------------------------------------------------------------
// Public API
// -----------------------------------------------------------------------------

// ConsistencyReport gathers all findings of the dry-run.
type ConsistencyReport struct {
	dir string // Directory that was checked
	// Segments present in meta.json but with no files on disk.
	MissingSegments []string
	// <segment>.<opstamp>.del files that meta.json expects but are absent.
	MissingDelFiles []string
	// Segment prefixes that exist on disk but are not referenced in meta.json.
	ExtraSegments []string
	// .del files on disk that are not referenced (wrong opstamp or orphan).
	ExtraDelFiles []string

	// Lock-file information
	WriterLockPresent bool // true if INDEX_WRITER_LOCK exists
	MetaLockPresent   bool // true if META_LOCK exists

	// Informational counters
	TotalSegmentsInMeta         int
	UniqueSegmentPrefixesOnDisk int
}

// Check runs the consistency test against dir and returns a report.
//
// It fails with an error if meta.json is absent or can’t be decoded.
func Check(dir string) (ConsistencyReport, error) {
	// ---------------------------------------------------------------------
	// 1) Parse meta.json
	// ---------------------------------------------------------------------
	meta, err := readMeta(filepath.Join(dir, "meta.json"))
	if err != nil {
		return ConsistencyReport{}, err
	}

	// Build metaSegments:  32-hex-id (no dashes) → expected opstamp (nil if none)
	metaSegments := make(map[string]*uint64, len(meta.Segments))
	for _, s := range meta.Segments {
		id := stripDashes(s.SegmentID)
		if s.Deletes != nil {
			metaSegments[id] = &s.Deletes.Opstamp
		} else {
			metaSegments[id] = nil
		}
	}

	// ---------------------------------------------------------------------
	// 2) Walk directory once
	// ---------------------------------------------------------------------
	segmentPrefixesDisk := map[string]struct{}{}
	delFilesDisk := map[[2]string]struct{}{} // key = [segPrefix, opstamp]

	var writerLockPresent, metaLockPresent bool

	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		name := d.Name()

		switch name {
		case "INDEX_WRITER_LOCK", ".tantivy-writer.lock":
			writerLockPresent = true
		case "META_LOCK", ".tantivy-meta.lock":
			metaLockPresent = true
		}

		if m := segPrefixRe.FindStringSubmatch(name); m != nil {
			segmentPrefixesDisk[m[1]] = struct{}{}
		}
		if m := delFileRe.FindStringSubmatch(name); m != nil {
			delFilesDisk[[2]string{m[1], m[2]}] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return ConsistencyReport{}, fmt.Errorf("scanning directory: %w", err)
	}

	// ---------------------------------------------------------------------
	// 3) Compare sets
	// ---------------------------------------------------------------------
	var (
		missingSegments []string
		extraSegments   []string
		missingDelFiles []string
		extraDelFiles   []string
	)

	// missing segments
	for id := range metaSegments {
		if _, ok := segmentPrefixesDisk[id]; !ok {
			missingSegments = append(missingSegments, id)
		}
	}

	// extra segments
	for id := range segmentPrefixesDisk {
		if _, ok := metaSegments[id]; !ok {
			extraSegments = append(extraSegments, id)
		}
	}

	// missing del files
	for id, opPtr := range metaSegments {
		if opPtr == nil {
			continue // no deletes expected
		}
		opStr := strconv.FormatUint(*opPtr, 10)
		if _, ok := delFilesDisk[[2]string{id, opStr}]; !ok {
			missingDelFiles = append(missingDelFiles, fmt.Sprintf("%s.%s.del", id, opStr))
		}
	}

	// extra del files
	for key := range delFilesDisk {
		id, opStr := key[0], key[1]
		expectedOpPtr, segKnown := metaSegments[id]
		if !segKnown || expectedOpPtr == nil || strconv.FormatUint(*expectedOpPtr, 10) != opStr {
			extraDelFiles = append(extraDelFiles, fmt.Sprintf("%s.%s.del", id, opStr))
		}
	}

	// ---------------------------------------------------------------------
	// 4) Return aggregated report
	// ---------------------------------------------------------------------
	return ConsistencyReport{
		dir:                         dir,
		MissingSegments:             missingSegments,
		MissingDelFiles:             missingDelFiles,
		ExtraSegments:               extraSegments,
		ExtraDelFiles:               extraDelFiles,
		WriterLockPresent:           writerLockPresent,
		MetaLockPresent:             metaLockPresent,
		TotalSegmentsInMeta:         len(metaSegments),
		UniqueSegmentPrefixesOnDisk: len(segmentPrefixesDisk),
	}, nil
}

// IsOk returns true when the report is free of inconsistencies:
//
//   - no segments are missing
//   - no .del files are missing
//   - no extra segments are present
//   - no extra .del files are present
//
// The presence of INDEX_WRITER_LOCK or META_LOCK is *not* considered
// an inconsistency—these files are expected during normal operation and
// merely reported for information.
func (r *ConsistencyReport) IsOk() bool {
	return len(r.MissingSegments) == 0 &&
		len(r.MissingDelFiles) == 0 &&
		len(r.ExtraSegments) == 0 &&
		len(r.ExtraDelFiles) == 0 &&
		!r.WriterLockPresent &&
		!r.MetaLockPresent
}

var segmentFileExts = []string{".fast", ".fieldnorm", ".pos", ".store", ".term", ".idx"}

// GCExtraFiles removes all extra segment files and .del files that are not
// referenced in meta.json.
// MUST be called before any write operations to the index directory.
func (r *ConsistencyReport) GCExtraFiles() error {
	if r.WriterLockPresent || r.MetaLockPresent {
		return fmt.Errorf("cannot run GC when INDEX_WRITER_LOCK or META_LOCK is present")
	}

	for _, seg := range r.ExtraSegments {
		for _, ext := range segmentFileExts {
			segFile := filepath.Join(r.dir, seg+ext)
			if err := os.Remove(segFile); err != nil {
				if os.IsNotExist(err) {
					continue // file already gone
				}
				return fmt.Errorf("removing segment file %s: %w", segFile, err)
			}
			fmt.Printf("ft: Removed extra segment file: %s\n", segFile)
		}
	}
	for _, delFile := range r.ExtraDelFiles {
		if err := os.Remove(filepath.Join(r.dir, delFile)); err != nil {
			return fmt.Errorf("removing extra .del file %s: %w", delFile, err)
		}
		fmt.Printf("ft: Removed extra .del file: %s\n", delFile)
	}
	return nil
}

// -----------------------------------------------------------------------------
// Internal helpers
// -----------------------------------------------------------------------------

// metaFile mirrors only the parts of meta.json we need.
type metaFile struct {
	Segments []struct {
		SegmentID string `json:"segment_id"`
		Deletes   *struct {
			Opstamp uint64 `json:"opstamp"`
		} `json:"deletes"`
	} `json:"segments"`
}

func readMeta(path string) (*metaFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var m metaFile
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("decoding meta.json: %w", err)
	}
	return &m, nil
}

func stripDashes(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '-' {
			out = append(out, s[i])
		}
	}
	return string(out)
}
