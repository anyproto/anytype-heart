package main

// native.go — the -native mode: drive the REAL exporter
// (core/block/export/anyblock) over every space of a live account and verify
// what its unit tests cannot. The default mode round-trips the LEGACY pb
// export through the codec, which exercises the codec and (since the
// composer moved to pkg/lib/anyblockjson/compose) the composition — but
// never the exporter itself: its layout, kind classification, blob binding
// and concurrency had no real-data coverage until this mode existed.
//
// Ground truth per space is the legacy pb export taken in the same process,
// moments earlier, with the same flags (no files, so the pb path's
// Source-clobber never fires and its snapshots are unmutilated). For every
// document the native exporter writes, three fidelity checks run against
// that truth:
//
//   - byte equality with Marshal(pb snapshot): the exporter builds its
//     snapshot from live state (BlocksToSave, CombinedDetails, …) while the
//     pb file is the pbc converter's rendering of the same state — if the
//     two constructions differ in ANY corner, the canonical bytes differ
//     and this check names the first line;
//   - snapshotdiff against the pb snapshot after a full Unmarshal: the
//     baseline data-loss measure, on the native bytes — "no worse than the
//     pb sweep's 34/38,105" is checked here, object for object;
//   - Marshal ∘ Unmarshal byte stability of the native document itself,
//     attribution-stripped like the default mode (import drops the derived
//     creator/last_modified_by, so gen2 cannot restate them).
//
// A pb document that the native bundle does NOT carry must be claimed by an
// omission predicate (space settings, profile page, widget, installed
// bundled relation) — anything else is a document the exporter LOST, the
// disqualifying failure. The reverse (native-only) and byte-diff cases can
// also arise from real state drift between the two exports (sync is live
// during the sweep); the per-category counts make that judgement possible.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/cmd/internal/anyblockbatch"
	"github.com/anyproto/anytype-heart/core"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/export"
	"github.com/anyproto/anytype-heart/core/block/export/anyblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/snapshotdiff"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/storeresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
)

// nativeDocExt is spelled out here rather than imported from compose:
// the layout check is only a cross-check if its vocabulary does not come
// from the code under test.
const nativeDocExt = ".anyblock.json"

// nativeDirs is the settled bundle layout (EXPORTER_DESIGN.md §1.2, Q1).
var nativeDirs = map[string]bool{
	"objects": true, "types": true, "templates": true, "properties": true,
	"options": true, "participants": true, "files": true,
}

// legacyDirNames is what must NOT appear anywhere in a native bundle — the
// store-vocabulary layout the design replaced.
var legacyDirNames = map[string]bool{
	"relations": true, "relationsOptions": true, "filesObjects": true, "profile": true,
}

// kindDirs maps a document's own declared `kind` to the directory it must
// land in. Deliberately a SECOND table, independent of compose.KindDirectory
// (which maps smartblock types): the classification cross-check is only a
// check if the two answers come from different roads. A kind not listed —
// or an omitted kind, which the format defines as `page` — belongs in
// objects/, the home for everything without a dedicated one.
var kindDirs = map[string]string{
	"object_type": "types", "bundled_object_type": "types",
	"template": "templates", "bundled_template": "templates",
	"property": "properties", "bundled_property": "properties",
	"property_option": "options",
	"participant":     "participants",
	"file_object":     "files", "file": "files",
}

type nativeSpaceSummary struct {
	SpaceId    string         `json:"spaceId"`
	SpaceName  string         `json:"spaceName,omitempty"`
	Succeed    int            `json:"succeed"`
	Docs       int            `json:"docs"`
	Blobs      int            `json:"blobs"`
	Omitted    int            `json:"omitted"`
	DirCounts  map[string]int `json:"dirCounts"`
	Issues     map[string]int `json:"issues,omitempty"`
	IndexBytes int            `json:"indexBytes,omitempty"`
	DictBytes  int            `json:"dictBytes,omitempty"`
}

type nativeSummary struct {
	Account     string               `json:"account"`
	Spaces      int                  `json:"spaces"`
	Docs        int                  `json:"docs"`
	Blobs       int                  `json:"blobs"`
	Omitted     int                  `json:"omitted"`
	DirCounts   map[string]int       `json:"dirCounts"`
	Issues      map[string]int       `json:"issues"`
	LossObjects int                  `json:"lossObjects"`
	PerSpace    []nativeSpaceSummary `json:"perSpace"`
	SpaceErrors []spaceError         `json:"spaceErrors,omitempty"`
}

// runNative is the -native mode's whole flow over one account.
func runNative(ctx context.Context, mw *core.Middleware, store objectstore.ObjectStore,
	spaces []spaceInfo, outDir string, includeFiles bool) error {

	a := mw.GetApp()
	exporter := &anyblock.Exporter{
		Collector:   app.MustComponent[export.Export](a),
		Picker:      app.MustComponent[cache.CachedObjectGetter](a),
		ObjectStore: store,
		SbtProvider: app.MustComponent[typeprovider.SmartBlockTypeProvider](a),
	}

	sum := &nativeSummary{Account: "", Spaces: len(spaces), DirCounts: map[string]int{}, Issues: map[string]int{}}
	for _, s := range spaces {
		fmt.Printf("\n== native %s (%s)\n", s.id, s.name)
		ss, err := processSpaceNative(ctx, mw, store, exporter, s, outDir, includeFiles)
		if err != nil {
			fmt.Printf("   space failed: %v\n", err)
			sum.SpaceErrors = append(sum.SpaceErrors, spaceError{SpaceId: s.id, Error: err.Error()})
			continue
		}
		sum.PerSpace = append(sum.PerSpace, *ss)
		sum.Docs += ss.Docs
		sum.Blobs += ss.Blobs
		sum.Omitted += ss.Omitted
		for d, n := range ss.DirCounts {
			sum.DirCounts[d] += n
		}
		for c, n := range ss.Issues {
			sum.Issues[c] += n
		}
		if n := ss.Issues["native_data_loss_object"]; n > 0 {
			sum.LossObjects += n
		}
	}

	data, err := json.MarshalIndent(sum, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal native summary: %w", err)
	}
	sumPath := filepath.Join(outDir, "native-summary.json")
	if err := os.WriteFile(sumPath, data, 0o644); err != nil {
		return fmt.Errorf("write native summary: %w", err)
	}

	fmt.Printf("\n==== native: %d spaces, %d docs, %d blobs, %d omitted\n", sum.Spaces, sum.Docs, sum.Blobs, sum.Omitted)
	for _, d := range sortedKeys(sum.DirCounts) {
		fmt.Printf("     %-14s %d\n", d, sum.DirCounts[d])
	}
	if len(sum.Issues) == 0 {
		fmt.Println("     issues: none")
	}
	for _, c := range sortedKeys(sum.Issues) {
		fmt.Printf("     ISSUE %-28s %d\n", c, sum.Issues[c])
	}
	fmt.Println("summary:", sumPath)
	hard := 0
	for c, n := range sum.Issues {
		// codec-level loss is measured against the pb sweep's own baseline in
		// the analysis, and timestamp drift is upstream state instability
		// (stripVolatileDates); everything else here is an exporter defect
		if c != "native_data_loss" && c != "native_data_loss_object" && c != "state_drift_timestamps" {
			hard += n
		}
	}
	if hard > 0 || len(sum.SpaceErrors) > 0 {
		return fmt.Errorf("native sweep found %d issues, %d space errors", hard, len(sum.SpaceErrors))
	}
	return nil
}

func processSpaceNative(ctx context.Context, mw *core.Middleware, store objectstore.ObjectStore,
	exporter *anyblock.Exporter, s spaceInfo, outDir string, includeFiles bool) (*nativeSpaceSummary, error) {

	ss := &nativeSpaceSummary{SpaceId: s.id, SpaceName: s.name,
		DirCounts: map[string]int{}, Issues: map[string]int{}}
	report := func(category, format string, args ...any) {
		if ss.Issues[category] < 10 { // keep the log usable; the counts carry the rest
			fmt.Printf("   ISSUE %s: %s\n", category, fmt.Sprintf(format, args...))
		}
		ss.Issues[category]++
	}

	// 1. ground truth: the legacy pb export, same process, same flags —
	// no files, so the Source-clobber never fires
	pbDir := filepath.Join(outDir, "native-pb", s.id)
	resp := mw.ObjectListExport(ctx, &pb.RpcObjectListExportRequest{
		SpaceId:         s.id,
		Path:            pbDir,
		Format:          model.Export_Protobuf,
		IncludeArchived: true,
		NoProgress:      true,
	})
	if resp.Error != nil && resp.Error.Code != 0 {
		return nil, rpcErr("ObjectListExport", int32(resp.Error.Code), resp.Error.Description)
	}
	pbRoot := resp.Path
	if pbRoot == "" {
		pbRoot = pbDir
	}
	defer os.RemoveAll(pbDir)
	pbPaths := map[string]string{} // id → .pb path
	err := filepath.WalkDir(pbRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".pb") {
			pbPaths[strings.TrimSuffix(filepath.Base(p), ".pb")] = p
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk pb export: %w", err)
	}

	// 2. the native bundle
	bundleRoot := filepath.Join(outDir, "native", s.id)
	if err := os.RemoveAll(bundleRoot); err != nil {
		return nil, fmt.Errorf("clean bundle dir: %w", err)
	}
	wr, err := anyblock.NewDirWriter(bundleRoot)
	if err != nil {
		return nil, fmt.Errorf("create bundle writer: %w", err)
	}
	req := anyblock.Request{
		SpaceId:         s.id,
		SpaceName:       s.name,
		IncludeArchived: true,
		IncludeFiles:    includeFiles,
	}
	result, err := exporter.Export(ctx, req, wr)
	if err != nil {
		return nil, fmt.Errorf("native export: %w", err)
	}
	ss.Succeed = result.Succeed
	if result.DocErrors > 0 {
		report("doc_emit_failed", "%d document(s) failed to emit", result.DocErrors)
	}
	if result.BlobErrors > 0 {
		report("blob_stream_failed", "%d file blob(s) could not be streamed; documents travel without bytes", result.BlobErrors)
	}

	// 3. layout, filenames, kinds
	docs, blobs, err := checkBundleLayout(bundleRoot, ss, report)
	if err != nil {
		return nil, fmt.Errorf("check bundle layout: %w", err)
	}
	ss.Docs = len(docs)
	ss.Blobs = len(blobs)

	// 4. the bundle files and the manifest's blob bindings
	idx := checkBundleFiles(bundleRoot, docs, blobs, ss, report)

	// 5. fidelity against the pb ground truth
	opts := storeresolver.New(store.SpaceIndex(s.id)).Options()
	nativeIds := map[string]bool{}
	for _, docPath := range docs {
		// the filename stem is the ENVELOPE id; the pb export names files by
		// the STORE id, which for a participant is the unfolded composite —
		// try both spellings, and mark both as covered
		id := strings.TrimSuffix(filepath.Base(docPath), nativeDocExt)
		nativeIds[id] = true
		pbPath, ok := pbPaths[id]
		if !ok {
			composite := domain.NewParticipantId(s.id, id)
			if pbPath, ok = pbPaths[composite]; ok {
				nativeIds[composite] = true
			}
		}
		if !ok {
			report("only_in_native", "%s has no pb ground truth (state drift, or a doc the legacy path skips)", id)
			continue
		}
		checkNativeDoc(id, filepath.Join(bundleRoot, filepath.FromSlash(docPath)), pbPath, opts, report)
	}
	expectedOmissions := 0
	for id, pbPath := range pbPaths {
		if nativeIds[id] {
			continue
		}
		sw, err := readPbSnapshot(pbPath)
		if err != nil {
			report("pb_unreadable", "%s: %v", id, err)
			continue
		}
		base := sw.Snapshot.GetData()
		switch {
		case anyblockjson.OmittedSpaceSettings(sw.SbType, base),
			anyblockjson.OmittedProfilePage(sw.SbType, base),
			anyblockjson.OmittedWidgetObject(sw.SbType, base):
			expectedOmissions++
		default:
			if _, ok := anyblockjson.OmittedBundledRelation(sw.SbType, base, opts); ok {
				expectedOmissions++
			} else {
				report("missing_in_native", "%s (%v) is in the pb export but not the native bundle and no omission predicate claims it", id, sw.SbType)
			}
		}
	}
	ss.Omitted = expectedOmissions

	// 6. determinism: export again, byte-compare the whole tree, drop the copy
	detRoot := filepath.Join(outDir, "native-det", s.id)
	if err := os.RemoveAll(detRoot); err != nil {
		return nil, fmt.Errorf("clean determinism dir: %w", err)
	}
	detWr, err := anyblock.NewDirWriter(detRoot)
	if err != nil {
		return nil, fmt.Errorf("create determinism writer: %w", err)
	}
	if _, err := exporter.Export(ctx, req, detWr); err != nil {
		return nil, fmt.Errorf("determinism export: %w", err)
	}
	compareTrees(bundleRoot, detRoot, report)
	if err := os.RemoveAll(detRoot); err != nil {
		return nil, fmt.Errorf("drop determinism tree: %w", err)
	}

	_ = idx
	fmt.Printf("   %d docs (%d blobs, %d omitted), dirs: %v\n", ss.Docs, ss.Blobs, ss.Omitted, dirLine(ss.DirCounts))
	return ss, nil
}

// checkBundleLayout walks the bundle and verifies the settled layout: only
// the seven kind directories and the two bundle files at the root, no
// legacy names anywhere, every document named <id>.anyblock.json with the
// stem equal to its own envelope id, every document in the directory its
// declared kind demands, and non-document files only in files/.
func checkBundleLayout(root string, ss *nativeSpaceSummary, report func(string, string, ...any)) (docs, blobs []string, err error) {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil, nil // an empty space writes nothing
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read bundle root: %w", err)
	}
	for _, e := range entries {
		name := e.Name()
		if legacyDirNames[name] {
			report("layout_legacy_name", "legacy entry %q at the bundle root", name)
			continue
		}
		if e.IsDir() {
			if !nativeDirs[name] {
				report("layout_alien_entry", "unexpected directory %q at the bundle root", name)
			}
			continue
		}
		if name != anyblockjson.IndexFileName && name != anyblockjson.PropertiesFileName {
			report("layout_alien_entry", "unexpected file %q at the bundle root", name)
		}
	}
	err = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		parts := strings.SplitN(rel, "/", 3)
		if len(parts) == 1 {
			return nil // the two root files, checked above
		}
		if len(parts) > 2 {
			report("layout_alien_entry", "nested path %q — the layout is one kind directory deep", rel)
			return nil
		}
		dir, base := parts[0], parts[1]
		if legacyDirNames[dir] {
			report("layout_legacy_name", "legacy directory in %q", rel)
			return nil
		}
		if !strings.HasSuffix(base, nativeDocExt) {
			if dir != "files" {
				report("layout_alien_entry", "non-document file %q outside files/", rel)
				return nil
			}
			blobs = append(blobs, rel)
			return nil
		}
		docs = append(docs, rel)
		ss.DirCounts[dir]++
		id := strings.TrimSuffix(base, nativeDocExt)
		var probe struct {
			Id   string `json:"id"`
			Kind string `json:"kind"`
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &probe); err != nil {
			report("doc_unparsable", "%s: %v", rel, err)
			return nil
		}
		if probe.Id != id {
			report("stem_id_mismatch", "%s: filename stem %q, envelope id %q — the path stopped being a pure function of the id", rel, id, probe.Id)
		}
		expected := "objects"
		if d, ok := kindDirs[probe.Kind]; ok {
			expected = d
		}
		if dir != expected {
			report("misfiled_kind", "%s declares kind %q and belongs in %s/", rel, probe.Kind, expected)
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("walk bundle: %w", err)
	}
	sort.Strings(docs)
	sort.Strings(blobs)
	return docs, blobs, nil
}

// checkBundleFiles reads index.json and properties.json back through the
// package, runs the manifest's blob bindings cross-check, and verifies
// every on-disk blob is bound and no blob path collides with a document.
func checkBundleFiles(root string, docs, blobs []string, ss *nativeSpaceSummary, report func(string, string, ...any)) *anyblockjson.Index {
	if len(docs) == 0 {
		return nil
	}
	idxData, err := os.ReadFile(filepath.Join(root, anyblockjson.IndexFileName))
	if err != nil {
		report("bundle_file_missing", "index.json: %v", err)
		return nil
	}
	ss.IndexBytes = len(idxData)
	idx, err := anyblockjson.UnmarshalIndex(idxData)
	if err != nil {
		report("bundle_file_invalid", "index.json: %v", err)
		return nil
	}
	dictData, err := os.ReadFile(filepath.Join(root, anyblockjson.PropertiesFileName))
	if err != nil {
		report("bundle_file_missing", "properties.json: %v", err)
	} else {
		ss.DictBytes = len(dictData)
		if _, err := anyblockjson.UnmarshalPropertyDictionary(dictData); err != nil {
			report("bundle_file_invalid", "properties.json: %v", err)
		}
	}

	docPaths := make([]string, 0, len(docs))
	docSet := map[string]bool{}
	for _, d := range docs {
		docPaths = append(docPaths, filepath.Join(root, filepath.FromSlash(d)))
		docSet[d] = true
	}
	for _, bad := range anyblockbatch.CheckManifestFiles(idx, root, docPaths) {
		report("manifest_files", "%s %s: %s", bad.Property, bad.Target, bad.Reason)
	}
	bound := map[string]bool{}
	if idx.Manifest != nil {
		for _, p := range idx.Manifest.Files {
			bound[p] = true
			if docSet[p] {
				report("blob_doc_collision", "manifest binds %q, which is a document path", p)
			}
		}
	}
	for _, b := range blobs {
		if !bound[b] {
			report("blob_orphan", "blob %q on disk with no manifest.files entry", b)
		}
	}
	return idx
}

// checkNativeDoc runs the three fidelity checks for one document against
// its pb ground truth (see the file comment).
func checkNativeDoc(id, docPath, pbPath string, opts anyblockjson.Options, report func(string, string, ...any)) {
	nativeJson, err := os.ReadFile(docPath)
	if err != nil {
		report("doc_unreadable", "%s: %v", id, err)
		return
	}
	sw, err := readPbSnapshot(pbPath)
	if err != nil {
		report("pb_unreadable", "%s: %v", id, err)
		return
	}
	base := sw.Snapshot.GetData()
	if base == nil {
		return
	}

	json1, err := anyblockjson.Marshal(sw.SbType, base, opts)
	if err != nil {
		report("pb_marshal_error", "%s: %v", id, err)
		return
	}
	if string(json1) != string(nativeJson) {
		if stripVolatileDates(json1) == stripVolatileDates(nativeJson) {
			report("state_drift_timestamps", "%s: only created/last-modified date lines differ (load-time-stamped, see stripVolatileDates)", id)
		} else {
			report("native_byte_diff", "%s: native bytes differ from Marshal(pb snapshot); first divergence: %s",
				id, firstDiff(json1, nativeJson))
		}
	}

	sbType2, reimported, err := anyblockjson.Unmarshal(nativeJson, opts)
	if err != nil {
		report("native_import_error", "%s: %v", id, err)
		return
	}
	if sbType2 != sw.SbType {
		report("native_kind_mismatch", "%s: pb %v, native reads back as %v", id, sw.SbType, sbType2)
	}
	if normalizeVolatileDates(base, reimported) {
		report("state_drift_timestamps", "%s: created/last-modified date drifted between the two exports", id)
	}
	diffs := snapshotdiff.Compare(base, reimported, sw.SbType, opts)
	for _, d := range diffs {
		report("native_data_loss", "%s: %s", id, d)
	}
	if len(diffs) > 0 {
		report("native_data_loss_object", "%s: %d finding(s)", id, len(diffs))
	}

	json2, err := anyblockjson.Marshal(sbType2, reimported, opts)
	if err != nil {
		report("native_reexport_error", "%s: %v", id, err)
		return
	}
	if stripAttribution(nativeJson) != stripAttribution(json2) {
		report("native_not_byte_stable", "%s: first divergence: %s", id,
			firstDiff([]byte(stripAttribution(nativeJson)), []byte(stripAttribution(json2))))
	}
}

// compareTrees byte-compares two directory trees — the corpus-scale
// determinism check (same space, exported twice, same bytes).
func compareTrees(a, b string, report func(string, string, ...any)) {
	list := func(root string) map[string]string {
		out := map[string]string{}
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			out[filepath.ToSlash(rel)] = p
			return nil
		})
		return out
	}
	first, second := list(a), list(b)
	for rel, pa := range first {
		pb2, ok := second[rel]
		if !ok {
			report("nondeterministic", "%s exists in the first export only", rel)
			continue
		}
		da, ea := os.ReadFile(pa)
		db, eb := os.ReadFile(pb2)
		if ea != nil || eb != nil {
			report("nondeterministic", "%s: unreadable (%v, %v)", rel, ea, eb)
			continue
		}
		if string(da) != string(db) {
			if stripVolatileDates(da) == stripVolatileDates(db) {
				report("state_drift_timestamps", "%s: only load-time-stamped date lines differ between the two exports", rel)
			} else {
				report("nondeterministic", "%s: content differs between two exports; first divergence: %s", rel, firstDiff(da, db))
			}
		}
	}
	for rel := range second {
		if _, ok := first[rel]; !ok {
			report("nondeterministic", "%s exists in the second export only", rel)
		}
	}
}

func readPbSnapshot(path string) (*pb.SnapshotWithType, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}
	var sw pb.SnapshotWithType
	if err := proto.Unmarshal(data, &sw); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	return &sw, nil
}

func dirLine(counts map[string]int) string {
	parts := make([]string, 0, len(counts))
	for _, d := range sortedKeys(counts) {
		parts = append(parts, fmt.Sprintf("%s=%d", d, counts[d]))
	}
	return strings.Join(parts, " ")
}

// stripVolatileDates removes the two LOAD-TIME-STAMPED details from a
// rendered document, the way stripAttribution removes the tree-derived
// pair: an object whose root change carries no creation date gets
// `createdDate = time.Now()` stamped on Apply
// (core/block/editor/smartblock/smartblock.go:733), so a fresh load — and
// close-after-write means every export IS a fresh load — re-mints it.
// Participant documents are the population that trips this (derived
// objects, no creation info in the tree). Two exports seconds apart then
// differ on exactly these lines with identical content otherwise; that is
// upstream state instability, not exporter nondeterminism — the legacy pb
// exporter re-exported after an eviction shifts the same way — and the
// classification keeps the determinism check sharp instead of failing the
// sweep on an app behaviour the exporter cannot control.
func stripVolatileDates(doc []byte) string {
	var out []string
	stripped := false
	for _, line := range strings.Split(string(doc), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, `"created_date":`) || strings.HasPrefix(t, `"last_modified_date":`) {
			stripped = true
			continue
		}
		if stripped && len(out) > 0 && strings.HasPrefix(t, "}") {
			out[len(out)-1] = strings.TrimSuffix(out[len(out)-1], ",")
		}
		stripped = false
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// normalizeVolatileDates aligns the two load-time-stamped details between
// the pb ground truth and the reimported native snapshot before the loss
// comparison, and reports whether they actually differed. Only a VALUE
// difference with both sides present is aligned — a side missing the key
// entirely stays visible to the comparator, so a dropped timestamp still
// counts as loss.
func normalizeVolatileDates(pbBase, re *model.SmartBlockSnapshotBase) (drifted bool) {
	pf := pbBase.GetDetails().GetFields()
	rf := re.GetDetails().GetFields()
	if pf == nil || rf == nil {
		return false
	}
	for _, key := range []string{"createdDate", "lastModifiedDate"} {
		pv, rv := pf[key], rf[key]
		if pv == nil || rv == nil {
			continue
		}
		if pv.GetNumberValue() != rv.GetNumberValue() {
			drifted = true
			rf[key] = pv
		}
	}
	return drifted
}
