// anyblockroundtrip verifies the AnyBlock JSON round-trip (pkg/lib/anyblockjson)
// against a real account: it recovers the account from a mnemonic, exports every
// object of every space to pb snapshots, converts each snapshot pb → AnyBlock
// JSON → pb, and checks the §11 contract (Export ∘ Import byte-stable, no
// unexpected data loss). Every inconsistency leaves an artifact directory with
// the original snapshot, both JSON generations, and a report for triage.
//
// Usage:
//
//	go run ./cmd/anyblockroundtrip -root-path ~/anyblockroundtrip-repo -out ./roundtrip-out
//
// The mnemonic is read from $ANYTYPE_MNEMONIC or the -mnemonic flag. The root
// path is the account repo directory: point it at a copy of an existing data
// dir to skip network sync, or at an empty dir to sync from the network (slow
// on large accounts; stop the desktop app first if you reuse its data dir —
// two processes cannot share one repo).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/core"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/snapshotdiff"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/storeresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	libcore "github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
)

func main() {
	var (
		mnemonic    = flag.String("mnemonic", os.Getenv("ANYTYPE_MNEMONIC"), "account mnemonic (default $ANYTYPE_MNEMONIC)")
		accountId   = flag.String("account-id", os.Getenv("ANYTYPE_ACCOUNT_ID"), "account id (default $ANYTYPE_ACCOUNT_ID; derived from the mnemonic when empty)")
		rootPath    = flag.String("root-path", "", "account repo directory (copy of a data dir, or empty dir to sync)")
		outDir      = flag.String("out", "roundtrip-out", "output directory for artifacts and summary")
		spaceFilter = flag.String("space", "", "comma-separated space ids to check (default: all)")
		limit       = flag.Int("limit", 0, "max objects per space (0 = all)")
		keepExports = flag.Bool("keep-exports", false, "keep the raw pb export directories for passing objects too")
		dumpJSON    = flag.Bool("dump-json", false, "write each object's AnyBlock JSON beside its .pb, rendered with the SPACE's resolvers (implies -keep-exports)")
	)
	flag.Parse()

	if *mnemonic == "" || *rootPath == "" {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "\nboth -mnemonic (or $ANYTYPE_MNEMONIC) and -root-path are required")
		os.Exit(2)
	}
	if *dumpJSON {
		*keepExports = true // the JSON is written beside the .pb, so the .pb must stay
	}
	if err := run(*mnemonic, *accountId, *rootPath, *outDir, *spaceFilter, *limit, *keepExports, *dumpJSON); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

//
// ---- account bootstrap ----
//

func rpcErr(name string, code int32, description string) error {
	if code == 0 {
		return nil
	}
	return fmt.Errorf("%s: code %d: %s", name, code, description)
}

func run(mnemonic, accountId, rootPath, outDir, spaceFilter string, limit int, keepExports, dumpJSON bool) error {
	ctx := context.Background()
	mw := core.New()
	mw.SetEventSender(event.NewCallbackSender(func(*pb.Event) {}))

	if resp := mw.InitialSetParameters(ctx, &pb.RpcInitialSetParametersRequest{
		Platform:           "cli",
		Version:            "0.0.0-anyblockroundtrip",
		Workdir:            rootPath,
		DoNotSendLogs:      true,
		DoNotSaveLogs:      true,
		DoNotSendTelemetry: true,
	}); resp.Error != nil && resp.Error.Code != 0 {
		return rpcErr("InitialSetParameters", int32(resp.Error.Code), resp.Error.Description)
	}

	if resp := mw.WalletRecover(ctx, &pb.RpcWalletRecoverRequest{
		RootPath: rootPath,
		Mnemonic: mnemonic,
	}); resp.Error != nil && resp.Error.Code != 0 {
		return rpcErr("WalletRecover", int32(resp.Error.Code), resp.Error.Description)
	}

	if accountId == "" {
		// the same derivation CreateSession uses for its identity check; the
		// mnemonic auth path returns an empty account id (GO-1854)
		derived, err := libcore.WalletAccountAt(mnemonic, 0)
		if err != nil {
			return fmt.Errorf("derive account from mnemonic: %w", err)
		}
		accountId = derived.Identity.GetPublic().Account()
	}
	fmt.Println("account:", accountId)

	selectResp := mw.AccountSelect(ctx, &pb.RpcAccountSelectRequest{
		Id:       accountId,
		RootPath: rootPath,
	})
	if selectResp.Error != nil && selectResp.Error.Code != 0 {
		return rpcErr("AccountSelect", int32(selectResp.Error.Code), selectResp.Error.Description)
	}
	defer mw.AccountStop(ctx, &pb.RpcAccountStopRequest{})

	a := mw.GetApp()
	store := app.MustComponent[objectstore.ObjectStore](a)
	spaceService := app.MustComponent[space.Service](a)

	spaces, err := listSpaces(store, spaceService.TechSpaceId())
	if err != nil {
		return fmt.Errorf("list spaces: %w", err)
	}
	if spaceFilter != "" {
		wanted := map[string]bool{}
		for _, id := range strings.Split(spaceFilter, ",") {
			wanted[strings.TrimSpace(id)] = true
		}
		var filtered []spaceInfo
		for _, s := range spaces {
			if wanted[s.id] {
				filtered = append(filtered, s)
			}
		}
		spaces = filtered
	}
	fmt.Printf("spaces to check: %d\n", len(spaces))

	summary := &summary{Account: accountId, Categories: map[string]int{}, IndentHistogram: map[int]int{}}
	for _, s := range spaces {
		fmt.Printf("\n== space %s (%s)\n", s.id, s.name)
		ss, err := processSpace(ctx, mw, store, s.id, outDir, limit, keepExports, dumpJSON)
		if err != nil {
			fmt.Printf("   space failed: %v\n", err)
			summary.SpaceErrors = append(summary.SpaceErrors, spaceError{SpaceId: s.id, Error: err.Error()})
			continue
		}
		ss.SpaceName = s.name
		summary.Spaces = append(summary.Spaces, *ss)
		summary.Total += ss.Total
		summary.Passed += ss.Passed
		summary.Failed += ss.Failed
		for c, n := range ss.Categories {
			summary.Categories[c] += n
		}
		for d, n := range ss.IndentHistogram {
			summary.IndentHistogram[d] += n
		}
		summary.CellsWithChildren += ss.CellsWithChildren
	}

	summaryPath := filepath.Join(outDir, "summary.json")
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal summary: %w", err)
	}
	if err := os.WriteFile(summaryPath, data, 0o644); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}

	fmt.Printf("\n==== total: %d objects, %d passed, %d failed\n", summary.Total, summary.Passed, summary.Failed)
	for _, c := range sortedKeys(summary.Categories) {
		fmt.Printf("     %-16s %d\n", c, summary.Categories[c])
	}
	p50, p95, maxD := indentPercentiles(summary.IndentHistogram)
	fmt.Printf("     block depth (max indent per object): p50=%d p95=%d max=%d\n", p50, p95, maxD)
	fmt.Printf("     cells with children: %d\n", summary.CellsWithChildren)
	fmt.Println("summary:", summaryPath)
	if summary.Failed > 0 || len(summary.SpaceErrors) > 0 {
		fmt.Println("artifacts:", filepath.Join(outDir, "artifacts"))
		return fmt.Errorf("%d objects failed round-trip", summary.Failed)
	}
	return nil
}

//
// ---- spaces ----
//

type spaceInfo struct {
	id   string
	name string
}

func listSpaces(store objectstore.ObjectStore, techSpaceId string) ([]spaceInfo, error) {
	records, err := store.SpaceIndex(techSpaceId).Query(database.Query{
		Filters: []database.FilterRequest{{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectType_spaceView)),
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("query space views: %w", err)
	}
	var out []spaceInfo
	seen := map[string]bool{}
	for _, r := range records {
		id := r.Details.GetString(bundle.RelationKeyTargetSpaceId)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, spaceInfo{id: id, name: r.Details.GetString(bundle.RelationKeyName)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out, nil
}

type spaceSummary struct {
	SpaceId    string         `json:"spaceId"`
	SpaceName  string         `json:"spaceName,omitempty"`
	Total      int            `json:"total"`
	Passed     int            `json:"passed"`
	Failed     int            `json:"failed"`
	Categories map[string]int `json:"categories,omitempty"`
	// IndentHistogram maps each object's max block indent to how many
	// objects have that maximum — verifies the "~6 typical max" depth datum.
	IndentHistogram map[int]int `json:"indentHistogram,omitempty"`
	// CellsWithChildren counts table cell blocks with real descendants (the
	// §6.1 array-form trigger).
	CellsWithChildren int `json:"cellsWithChildren"`
}

type spaceError struct {
	SpaceId string `json:"spaceId"`
	Error   string `json:"error"`
}

type summary struct {
	Account           string         `json:"account"`
	Total             int            `json:"total"`
	Passed            int            `json:"passed"`
	Failed            int            `json:"failed"`
	Categories        map[string]int `json:"categories"`
	IndentHistogram   map[int]int    `json:"indentHistogram"`
	CellsWithChildren int            `json:"cellsWithChildren"`
	Spaces            []spaceSummary `json:"spaces"`
	SpaceErrors       []spaceError   `json:"spaceErrors,omitempty"`
}

func processSpace(ctx context.Context, mw *core.Middleware, store objectstore.ObjectStore,
	spaceId, outDir string, limit int, keepExports, dumpJSON bool) (*spaceSummary, error) {

	exportDir := filepath.Join(outDir, "export", spaceId)
	resp := mw.ObjectListExport(ctx, &pb.RpcObjectListExportRequest{
		SpaceId:         spaceId,
		Path:            exportDir,
		Format:          model.Export_Protobuf,
		IncludeArchived: true,
		NoProgress:      true,
	})
	if resp.Error != nil && resp.Error.Code != 0 {
		return nil, rpcErr("ObjectListExport", int32(resp.Error.Code), resp.Error.Description)
	}
	exportPath := resp.Path
	if exportPath == "" {
		exportPath = exportDir
	}

	var files []string
	err := filepath.WalkDir(exportPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".pb") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk export dir: %w", err)
	}
	sort.Strings(files)
	if limit > 0 && len(files) > limit {
		files = files[:limit]
	}

	opts := storeresolver.New(store.SpaceIndex(spaceId)).Options()

	ss := &spaceSummary{SpaceId: spaceId, Total: len(files), Categories: map[string]int{}, IndentHistogram: map[int]int{}}
	for _, f := range files {
		objectId := strings.TrimSuffix(filepath.Base(f), ".pb")
		issues, artifacts, err := roundtripFile(f, opts)
		if err != nil {
			return nil, fmt.Errorf("object %s: %w", objectId, err)
		}
		if artifacts != nil {
			if dumpJSON && artifacts.json1 != nil {
				// the document as this SPACE renders it: property and type
				// labels through the space vocabulary, option values by name,
				// participants by name. Rendering with default Options
				// instead produces a technically valid document in which every
				// space-minted key is a bson id and every option value a CID —
				// readable structure, unreadable content.
				out := strings.TrimSuffix(f, ".pb") + ".anyblock.json"
				if err := os.WriteFile(out, artifacts.json1, 0o644); err != nil {
					return nil, fmt.Errorf("dump json for %s: %w", objectId, err)
				}
			}
			if artifacts.json1 != nil {
				ss.IndentHistogram[maxIndentOf(artifacts.json1)]++
			}
			if artifacts.original != nil {
				ss.CellsWithChildren += countCellsWithChildren(artifacts.original.Snapshot.GetData())
			}
		}
		if len(issues) == 0 {
			ss.Passed++
			continue
		}
		ss.Failed++
		for _, is := range issues {
			ss.Categories[is.category]++
		}
		if err := writeArtifacts(filepath.Join(outDir, "artifacts", spaceId, objectId), f, issues, artifacts); err != nil {
			return nil, fmt.Errorf("write artifacts for %s: %w", objectId, err)
		}
		fmt.Printf("   FAIL %s: %s\n", objectId, issueLine(issues))
	}
	fmt.Printf("   %d objects, %d passed, %d failed\n", ss.Total, ss.Passed, ss.Failed)

	if !keepExports {
		if err := os.RemoveAll(exportDir); err != nil {
			return nil, fmt.Errorf("clean export dir: %w", err)
		}
	}
	return ss, nil
}

//
// ---- round-trip ----
//

type issue struct {
	category string
	detail   string
}

func issueLine(issues []issue) string {
	parts := make([]string, 0, len(issues))
	for _, is := range issues {
		parts = append(parts, is.category)
	}
	return strings.Join(parts, ", ")
}

// artifacts collects everything worth persisting when an object fails.
type artifactSet struct {
	original   *pb.SnapshotWithType
	json1      []byte
	json2      []byte
	reimported *pb.SnapshotWithType
}

// stripAttribution removes the two derived attribution members from a rendered
// document so two generations can be compared on what the format is actually
// responsible for. It is deliberately textual and deliberately narrow: it drops
// only a top-level `"creator"` or `"last_modified_by"` line, so an attribution
// value appearing anywhere else still counts as a difference.
func stripAttribution(doc []byte) string {
	var out []string
	for _, line := range strings.Split(string(doc), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, `"creator":`) || strings.HasPrefix(t, `"last_modified_by":`) {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func roundtripFile(path string, opts anyblockjson.Options) ([]issue, *artifactSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read snapshot: %w", err)
	}
	var sw pb.SnapshotWithType
	if err := proto.Unmarshal(data, &sw); err != nil {
		return nil, nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	base := sw.Snapshot.GetData()
	if base == nil {
		return nil, nil, fmt.Errorf("snapshot has no data")
	}

	arts := &artifactSet{original: &sw}
	var issues []issue
	fail := func(category, format string, args ...any) {
		issues = append(issues, issue{category: category, detail: fmt.Sprintf(format, args...)})
	}

	json1, err := anyblockjson.Marshal(sw.SbType, base, opts)
	if err != nil {
		fail("export_error", "%v", err)
		return issues, arts, nil
	}
	arts.json1 = json1

	sbType2, reimported, err := anyblockjson.Unmarshal(json1, opts)
	if err != nil {
		fail("import_error", "%v", err)
		return issues, arts, nil
	}
	arts.reimported = &pb.SnapshotWithType{SbType: sbType2, Snapshot: &pb.ChangeSnapshot{Data: reimported}}
	if sbType2 != sw.SbType {
		fail("kind_mismatch", "exported %v, reimported %v", sw.SbType, sbType2)
	}

	json2, err := anyblockjson.Marshal(sbType2, reimported, opts)
	if err != nil {
		fail("reexport_error", "%v", err)
		return issues, arts, nil
	}
	// Attribution is written from a DERIVED detail that import deliberately
	// drops (SPEC §3): `creator` and `last_modified_by` are recovered from the
	// object tree's own signature, so a real import re-derives them — but this
	// round trip has no tree, so gen2 has nothing to write and the two
	// generations differ on exactly those lines. Comparing them raw reports
	// every object in an account as unstable and buries whatever else moved:
	// measured, 37,011 of 37,429, and 4,000 of a 4,001 sample differed by
	// nothing else. So the check strips the lines it knows are owed to the
	// tree, the way the detail comparator already skips the internal keys.
	if stripAttribution(json1) != stripAttribution(json2) {
		arts.json2 = json2
		fail("not_byte_stable", "first divergence: %s",
			firstDiff([]byte(stripAttribution(json1)), []byte(stripAttribution(json2))))
	}

	// the ORIGINAL smartblock type: how many type slots the envelope had is a
	// question about the snapshot that went in (§2), and sbType2 is the answer
	// the round trip produced — using it would make the diff agree with a
	// round trip that changed the kind
	for _, d := range snapshotdiff.Compare(base, reimported, sw.SbType, opts) {
		fail("data_loss", "%s", d)
	}
	return issues, arts, nil
}

// maxIndentOf reads the deepest block indent in an exported document — the
// per-object depth datum for the sweep histogram.
func maxIndentOf(jsonDoc []byte) int {
	var doc struct {
		Blocks []struct {
			Indent int `json:"indent"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(jsonDoc, &doc); err != nil {
		return 0
	}
	maxIndent := 0
	for _, b := range doc.Blocks {
		if b.Indent > maxIndent {
			maxIndent = b.Indent
		}
	}
	return maxIndent
}

// countCellsWithChildren counts table cell blocks carrying real descendants —
// the trigger for the §6.1 array-form cell encoding.
func countCellsWithChildren(base *model.SmartBlockSnapshotBase) int {
	if base == nil {
		return 0
	}
	byId := map[string]*model.Block{}
	for _, b := range base.Blocks {
		if b != nil {
			byId[b.Id] = b
		}
	}
	n := 0
	for _, b := range base.Blocks {
		if b == nil {
			continue
		}
		if _, ok := b.Content.(*model.BlockContentOfTableRow); !ok {
			continue
		}
		for _, cid := range b.ChildrenIds {
			if cell := byId[cid]; cell != nil && len(cell.ChildrenIds) > 0 {
				n++
			}
		}
	}
	return n
}

// indentPercentiles reads p50/p95/max of the per-object max-indent histogram.
func indentPercentiles(hist map[int]int) (p50, p95, maxDepth int) {
	total := 0
	depths := make([]int, 0, len(hist))
	for d, n := range hist {
		depths = append(depths, d)
		total += n
	}
	if total == 0 {
		return 0, 0, 0
	}
	sort.Ints(depths)
	maxDepth = depths[len(depths)-1]
	cum := 0
	got50, got95 := false, false
	for _, d := range depths {
		cum += hist[d]
		if !got50 && cum*100 >= total*50 {
			p50, got50 = d, true
		}
		if !got95 && cum*100 >= total*95 {
			p95, got95 = d, true
			break
		}
	}
	return p50, p95, maxDepth
}

// firstDiff reports the first differing line between two JSON generations.
func firstDiff(a, b []byte) string {
	la, lb := strings.Split(string(a), "\n"), strings.Split(string(b), "\n")
	for i := 0; i < len(la) && i < len(lb); i++ {
		if la[i] != lb[i] {
			return fmt.Sprintf("line %d: %q vs %q", i+1, strings.TrimSpace(la[i]), strings.TrimSpace(lb[i]))
		}
	}
	return fmt.Sprintf("lengths differ: %d vs %d lines", len(la), len(lb))
}

//
// ---- artifacts ----
//

func writeArtifacts(dir, pbPath string, issues []issue, arts *artifactSet) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	raw, err := os.ReadFile(pbPath)
	if err != nil {
		return fmt.Errorf("read original: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "original.pb"), raw, 0o644); err != nil {
		return fmt.Errorf("write original.pb: %w", err)
	}
	marshaler := jsonpb.Marshaler{Indent: "  "}
	// typed parameter, not proto.Message: a nil *pb.SnapshotWithType wrapped
	// in the interface would defeat the nil guard and crash jsonpb
	writeProtoJSON := func(name string, m *pb.SnapshotWithType) error {
		if m == nil {
			return nil
		}
		s, err := marshaler.MarshalToString(m)
		if err != nil {
			return fmt.Errorf("jsonpb %s: %w", name, err)
		}
		return os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644)
	}
	if err := writeProtoJSON("original.pb.json", arts.original); err != nil {
		return err
	}
	if err := writeProtoJSON("reimported.pb.json", arts.reimported); err != nil {
		return err
	}
	if arts.json1 != nil {
		if err := os.WriteFile(filepath.Join(dir, "roundtrip.json"), arts.json1, 0o644); err != nil {
			return fmt.Errorf("write roundtrip.json: %w", err)
		}
	}
	if arts.json2 != nil {
		if err := os.WriteFile(filepath.Join(dir, "roundtrip2.json"), arts.json2, 0o644); err != nil {
			return fmt.Errorf("write roundtrip2.json: %w", err)
		}
	}
	var report strings.Builder
	for _, is := range issues {
		fmt.Fprintf(&report, "[%s] %s\n", is.category, is.detail)
	}
	if err := os.WriteFile(filepath.Join(dir, "report.txt"), []byte(report.String()), 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
