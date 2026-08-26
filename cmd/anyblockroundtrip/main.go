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
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/cmd/internal/anyblockbatch"
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
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
		refNames    = flag.Bool("ref-names", false, "render the READ shape: every object reference carries its informative #name suffix (SPEC.md §9)")
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
	if err := run(*mnemonic, *accountId, *rootPath, *outDir, *spaceFilter, *limit, *keepExports, *dumpJSON, *refNames); err != nil {
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

func run(mnemonic, accountId, rootPath, outDir, spaceFilter string, limit int, keepExports, dumpJSON, refNames bool) error {
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
		ss, err := processSpace(ctx, mw, store, s.id, s.name, outDir, limit, keepExports, dumpJSON, refNames)
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
		summary.OmittedRelationDocs += ss.OmittedRelationDocs
		summary.OmittedBytes += ss.OmittedBytes
		summary.DictionaryInstalled += ss.DictionaryInstalled
		summary.DictionaryEntries += ss.DictionaryEntries
		summary.DictionaryBytes += ss.DictionaryBytes
		summary.IndexBytes += ss.IndexBytes
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
	if dumpJSON {
		fmt.Printf("     §2f composition: omitted %d relation docs (%d bytes); dictionaries %d bytes (%d installed, %d entries); indexes %d bytes\n",
			summary.OmittedRelationDocs, summary.OmittedBytes,
			summary.DictionaryBytes, summary.DictionaryInstalled, summary.DictionaryEntries, summary.IndexBytes)
	}
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
	// The §2f composition: how many bundled-identical relation documents the
	// dump omitted (their bytes, as the removed cost), and what the space's
	// dictionary and manifest carry instead.
	OmittedRelationDocs int `json:"omittedRelationDocs,omitempty"`
	OmittedBytes        int `json:"omittedBytes,omitempty"`
	DictionaryInstalled int `json:"dictionaryInstalled,omitempty"`
	DictionaryEntries   int `json:"dictionaryEntries,omitempty"`
	ManifestTypes       int `json:"manifestTypes,omitempty"`
	ManifestOptions     int `json:"manifestOptions,omitempty"`
	DictionaryBytes     int `json:"dictionaryBytes,omitempty"`
	IndexBytes          int `json:"indexBytes,omitempty"`
	// OrphanUsedKeys are referenced property keys with no definition
	// anywhere — no relation object, not bundled — so the dictionary cannot
	// state a format for them (§2f names every property it CAN).
	OrphanUsedKeys []string `json:"orphanUsedKeys,omitempty"`
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
	// the §2f composition, account-wide: what the omission removed and what
	// the dictionary and manifest cost instead — the byte-delta headline.
	OmittedRelationDocs int            `json:"omittedRelationDocs,omitempty"`
	OmittedBytes        int            `json:"omittedBytes,omitempty"`
	DictionaryInstalled int            `json:"dictionaryInstalled,omitempty"`
	DictionaryEntries   int            `json:"dictionaryEntries,omitempty"`
	DictionaryBytes     int            `json:"dictionaryBytes,omitempty"`
	IndexBytes          int            `json:"indexBytes,omitempty"`
	Spaces              []spaceSummary `json:"spaces"`
	SpaceErrors         []spaceError   `json:"spaceErrors,omitempty"`
}

func processSpace(ctx context.Context, mw *core.Middleware, store objectstore.ObjectStore,
	spaceId, spaceName, outDir string, limit int, keepExports, dumpJSON, refNames bool) (*spaceSummary, error) {

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
	// the read shape (§9): both legs of the round trip carry it, so the
	// suffix is exercised as a fixpoint rather than only written
	opts.RefNames = refNames

	composer := newSpaceComposer(opts, spaceName)
	ss := &spaceSummary{SpaceId: spaceId, Total: len(files), Categories: map[string]int{}, IndentHistogram: map[int]int{}}
	for _, f := range files {
		objectId := strings.TrimSuffix(filepath.Base(f), ".pb")
		issues, artifacts, err := roundtripFile(f, opts)
		if err != nil {
			return nil, fmt.Errorf("object %s: %w", objectId, err)
		}
		// the §2f composition: a bundled-identical relation document is not
		// written — its key travels in the dictionary's `installed` list —
		// and the trip it takes instead (installed key → the reader's
		// bundled table) is verified here, per omitted document, through the
		// same comparator as everything else.
		omitted := false
		if artifacts != nil && artifacts.original != nil {
			isOmitted, recon := composer.observeSnapshot(artifacts.original)
			omitted = isOmitted
			issues = append(issues, recon...)
		}
		if artifacts != nil {
			if dumpJSON && artifacts.json1 != nil {
				if omitted {
					composer.omittedDocs++
					composer.omittedBytes += len(artifacts.json1)
				} else {
					// the document as this SPACE renders it: property and type
					// labels through the space vocabulary, option values by name,
					// participant refs folded to the identity and attribution as
					// <identity>#<name> (§9). Rendering with default Options
					// instead produces a technically valid document in which every
					// space-minted key is a bson id and every option value a CID —
					// readable structure, unreadable content.
					out := strings.TrimSuffix(f, ".pb") + ".anyblock.json"
					if err := os.WriteFile(out, artifacts.json1, 0o644); err != nil {
						return nil, fmt.Errorf("dump json for %s: %w", objectId, err)
					}
					composer.observeWritten(artifacts.original, out)
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
	if dumpJSON {
		if err := composer.finish(ss); err != nil {
			return nil, fmt.Errorf("compose bundle files: %w", err)
		}
	}
	fmt.Printf("   %d objects, %d passed, %d failed\n", ss.Total, ss.Passed, ss.Failed)
	if dumpJSON {
		fmt.Printf("   dictionary: %d installed, %d entries; manifest: %d types, %d options; omitted %d relation docs (%d bytes)\n",
			ss.DictionaryInstalled, ss.DictionaryEntries, ss.ManifestTypes, ss.ManifestOptions,
			ss.OmittedRelationDocs, ss.OmittedBytes)
	}

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
//
// When the stripped member was the LAST in its object, the previous line keeps
// a trailing comma the other generation never had — a blindspot this strip
// carried silently until v0.32 exposed it: the §2a settings lift moved
// `plural_name`/`recommended_layout` out of `properties`, which made `creator`
// the final property on most type documents, and every one of them reported
// not_byte_stable over a comma. The comma is trimmed only when the strip
// actually removed the member between it and the closing brace, so a real
// difference on the neighbouring lines still counts.
func stripAttribution(doc []byte) string {
	var out []string
	stripped := false
	for _, line := range strings.Split(string(doc), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, `"creator":`) || strings.HasPrefix(t, `"last_modified_by":`) {
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
	// drops (SPEC §3): `creator` and `last_modified_by` — spelled
	// <identity>#<name> since v0.27 — are recovered from the
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

//
// ---- the §2f composition: dictionary, manifest, omitted relation documents ----
//

// spaceComposer accumulates, across one space's dump, everything the two
// bundle-level files state: which bundled relations are installed (and which
// of their documents the dump omitted), the definitions the dictionary
// carries, and where the manifest finds each type and option. It exists in
// this tool because composition is a bundle-level act the one-document codec
// deliberately does not own (SPEC.md §2f, §13).
type spaceComposer struct {
	opts      anyblockjson.Options
	spaceName string

	installed map[string]bool
	// entries the space's own documents define: a KEPT bundled-key relation
	// document (divergent from the table, or carrying something only a
	// document can) contributes its stored definition, so the dictionary
	// states the divergence the `installed` list alone would paper over
	entries map[string]anyblockjson.PropertyDefinition

	typePaths   map[string]string
	optionPaths map[string]string
	// optionsByKey is the select vocabulary each property actually has in
	// this space, gathered from the option documents so the dictionary can
	// state it inline (§2f). Keyed by STORED property key, and held with the
	// stored `orderId` so the inline array can be written in the order the
	// space actually shows.
	optionsByKey map[string][]storedOption
	written      []string

	// the fields the space's own document and the widget object are the
	// sources of (§2c). Lifted as each document is observed and omitted, so
	// the index states what the dropped documents held.
	index anyblockjson.Index

	omittedDocs  int
	omittedBytes int
}

func newSpaceComposer(opts anyblockjson.Options, spaceName string) *spaceComposer {
	return &spaceComposer{
		opts:         opts,
		spaceName:    spaceName,
		installed:    map[string]bool{},
		entries:      map[string]anyblockjson.PropertyDefinition{},
		typePaths:    map[string]string{},
		optionPaths:  map[string]string{},
		optionsByKey: map[string][]storedOption{},
	}
}

// observeSnapshot classifies one snapshot for the composition. For an
// omitted document it also verifies the trip the object takes INSTEAD of a
// document — installed key → the reader's bundled table — through the same
// comparator as every ordinary round trip, so the omission predicate and
// the reconstruction cannot drift apart silently.
func (c *spaceComposer) observeSnapshot(sw *pb.SnapshotWithType) (omitted bool, issues []issue) {
	base := sw.Snapshot.GetData()
	if base == nil {
		return false, nil
	}
	// the space's own object: index.json states everything it holds (§2c),
	// so the composer lifts those fields and drops the document. The lift
	// runs BEFORE the omission is recorded, so a bundle can never drop the
	// document without having written what it carried.
	if anyblockjson.OmittedSpaceSettings(sw.SbType, base) {
		anyblockjson.IndexFromSpaceSettings(&c.index, base)
		return true, nil
	}
	// the deprecated per-space profile object: superseded by `participant`,
	// and what survives in a real account is an empty hidden object carrying
	// someone else's name, dragged in by an import (§2c)
	if anyblockjson.OmittedProfilePage(sw.SbType, base) {
		return true, nil
	}
	// the sidebar's object: index.json states everything it holds (§2c) —
	// the wrapper-and-link pairs flat in `widgets`, the auto-widget ledger
	// at index level — so the composer lifts those fields and drops the
	// document, the space-settings rule again. The lift runs BEFORE the
	// omission is recorded, and the snapshot a bundle carries INSTEAD
	// (WidgetsSnapshot, the same function cmd/anyblockconvert installs
	// from) is verified against the original through the same comparator as
	// every ordinary round trip, so the lift and the rebuild cannot drift
	// apart silently. A nil snapshot means the index carries no sidebar
	// state because the object held none — the predicate is the proof.
	if anyblockjson.OmittedWidgetObject(sw.SbType, base) {
		anyblockjson.IndexFromWidgetObject(&c.index, base)
		rebuilt, err := anyblockjson.WidgetsSnapshot(&c.index)
		if err != nil {
			return true, []issue{{category: "omitted_reconstruction",
				detail: fmt.Sprintf("widget object: %v", err)}}
		}
		if rebuilt != nil {
			for _, d := range snapshotdiff.Compare(base, rebuilt, sw.SbType, c.opts) {
				issues = append(issues, issue{category: "omitted_reconstruction", detail: d})
			}
		}
		return true, issues
	}
	if key, ok := anyblockjson.OmittedBundledRelation(sw.SbType, base, c.opts); ok {
		c.installed[key] = true
		det, ok := anyblockjson.InstalledRelationDetails(key, c.opts)
		if !ok {
			return true, []issue{{category: "omitted_reconstruction",
				detail: fmt.Sprintf("installed key %q has no bundled reconstruction", key)}}
		}
		got := &model.SmartBlockSnapshotBase{Details: det, ObjectTypes: base.ObjectTypes}
		for _, d := range snapshotdiff.Compare(base, got, sw.SbType, c.opts) {
			issues = append(issues, issue{category: "omitted_reconstruction", detail: d})
		}
		return true, issues
	}
	if det := base.GetDetails().GetFields(); det != nil &&
		(sw.SbType == model.SmartBlockType_STRelation || sw.SbType == model.SmartBlockType_BundledRelation) {
		key := det["relationKey"].GetStringValue()
		if key != "" && bundle.HasRelation(domain.RelationKey(key)) {
			// installed but not omittable: the document stays, and the
			// dictionary carries its stored definition as the full entry
			// the §2f divergence rule requires
			c.installed[key] = true
			c.entries[key] = storedRelationDefinition(base, c.opts)
		}
	}
	return false, nil
}

// observeWritten records a written document's place for the manifest: a type
// by its STORED key, an option by its id — the two namespaces documents
// address without a path (§2c).
func (c *spaceComposer) observeWritten(sw *pb.SnapshotWithType, path string) {
	c.written = append(c.written, path)
	det := sw.Snapshot.GetData().GetDetails().GetFields()
	if det == nil {
		return
	}
	switch sw.SbType {
	case model.SmartBlockType_STType, model.SmartBlockType_BundledObjectType:
		if key := strings.TrimPrefix(det["uniqueKey"].GetStringValue(), "ot-"); key != "" {
			c.typePaths[key] = path
		}
	case model.SmartBlockType_STRelationOption:
		// an option's whole meaning is three details — which property it
		// belongs to, its name, and its colour — wrapped in a document whose
		// remaining forty lines are derived scaffolding. The dictionary
		// states those three inline, so a bundle declares a select vocabulary
		// in the same place it declares the property (§2f).
		if key := det["relationKey"].GetStringValue(); key != "" {
			if name := det["name"].GetStringValue(); name != "" {
				c.optionsByKey[key] = append(c.optionsByKey[key], storedOption{
					order: det["orderId"].GetStringValue(),
					def: anyblockjson.OptionDefinition{
						Name:  name,
						Color: det["relationOptionColor"].GetStringValue(),
						// the option's stored key: minted, so derivable from
						// nothing, unlike its name, colour, position and api
						// key (§2f). Carried by uniqueKey `opt-<key>`.
						InternalKey: strings.TrimPrefix(
							det["uniqueKey"].GetStringValue(), "opt-"),
					},
				})
			}
		}
		if id := det["id"].GetStringValue(); id != "" {
			c.optionPaths[id] = path
		}
	}
}

// finish writes the space's properties.json and index.json at the bundle
// root, and re-reads both — the bundle-level twin of the I1 discipline: a
// file this tool writes that the package's own Unmarshal rejects is a bug
// here, found now rather than at restore time.
func (c *spaceComposer) finish(ss *spaceSummary) error {
	if len(c.written) == 0 {
		return nil
	}
	root := bundleRootOf(c.written)

	// the dictionary names every property the documents actually reference
	// (§2f, used-only): the space's own definitions first (divergent
	// installed copies, space-minted relation documents keep their files but
	// the dictionary still answers for every USED key), then the resolver,
	// then the bundled table. A key none of them can define — an orphan
	// detail no relation object describes — is reported, not invented.
	used, err := anyblockbatch.UsedPropertyKeys(c.written)
	if err != nil {
		return fmt.Errorf("scan used property keys: %w", err)
	}
	entries := map[string]anyblockjson.PropertyDefinition{}
	for key, def := range c.entries {
		if used[key] {
			entries[key] = def
		} else if _, installedToo := c.installed[key]; installedToo {
			// a divergent installed copy is an entry whether or not
			// anything uses it: `installed` would otherwise restore the
			// table's shape over the divergence
			entries[key] = def
		}
	}
	var orphans []string
	for key := range used {
		if _, have := entries[key]; have {
			continue
		}
		if def, ok := resolvedDefinition(key, c.opts); ok {
			entries[key] = def
			continue
		}
		if rel, err := bundle.GetRelation(domain.RelationKey(key)); err == nil {
			entries[key] = anyblockjson.PropertyDefinition{
				Key: domain.RelationKey(key), Name: rel.Name, Format: rel.Format,
				ObjectTypes: bundledTargetKeys(rel.ObjectTypes),
			}
			continue
		}
		orphans = append(orphans, key)
	}
	sort.Strings(orphans)

	// the select vocabulary travels with the property that owns it. A
	// property whose options a space minted has no entry otherwise — it is an
	// ordinary installed bundled key — and its vocabulary would exist only in
	// the option documents, where an author generating a bundle has to know
	// to look for it.
	for key, stored := range c.optionsByKey {
		if !used[key] {
			continue // §2f is used-only: an unused property's vocabulary buys a reader nothing
		}
		// in the order the SPACE shows them, which the stored `orderId`
		// carries: `status` really reads To Do → In Progress → Done, and
		// sorting by name turned that workflow into Done → In Progress →
		// To Do on 42 of the 61 vocabularies that state an order.
		//
		// An option with no orderId sorts AFTER the ordered ones, by name,
		// and that is not a compromise — it is the app's own model. Ordering
		// is a newer feature than options: 229 of 312 vocabularies state no
		// order at all and 21 state one for only some members, and the app's
		// own placement query (objectcreator/relation_option.go) filters
		// `orderId NotEmpty`, so an option without one is not in the app's
		// ordering either. There is no order to lose; name is what makes the
		// canonical form deterministic.
		sort.SliceStable(stored, func(i, j int) bool {
			a, b := stored[i], stored[j]
			if (a.order == "") != (b.order == "") {
				return a.order != ""
			}
			if a.order != b.order {
				return a.order < b.order
			}
			return a.def.Name < b.def.Name
		})
		opts := make([]anyblockjson.OptionDefinition, 0, len(stored))
		for _, so := range stored {
			opts = append(opts, so.def)
		}
		def, have := entries[key]
		if !have {
			if resolved, ok := resolvedDefinition(key, c.opts); ok {
				def = resolved
			} else if rel, err := bundle.GetRelation(domain.RelationKey(key)); err == nil {
				def = anyblockjson.PropertyDefinition{
					Key: domain.RelationKey(key), Name: rel.Name, Format: rel.Format,
					ObjectTypes: bundledTargetKeys(rel.ObjectTypes),
				}
			} else {
				continue // nothing can say what this property is; §2f reports it as an orphan
			}
		}
		def.Options = opts
		entries[key] = def
	}

	dict := &anyblockjson.PropertyDictionary{}
	for key := range c.installed {
		dict.Installed = append(dict.Installed, key)
	}
	for _, key := range sortedStringSet(entries) {
		dict.Properties = append(dict.Properties, entries[key])
	}
	dictData, err := anyblockjson.MarshalPropertyDictionary(dict)
	if err != nil {
		return fmt.Errorf("marshal property dictionary: %w", err)
	}
	if _, err := anyblockjson.UnmarshalPropertyDictionary(dictData); err != nil {
		return fmt.Errorf("re-read property dictionary: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, anyblockjson.PropertiesFileName), dictData, 0o644); err != nil {
		return fmt.Errorf("write property dictionary: %w", err)
	}

	// start from what the space's own document was lifted into (§2c) rather
	// than copying its fields across by hand: the hand-written version listed
	// three, and silently dropped the space ICON the moment the lift learned
	// to carry one. Whatever IndexFromSpaceSettings writes now travels
	// without this function being told about it.
	idx := c.index
	// the listing's name is the fallback for a space whose document has none
	idx.Name = firstNonEmpty(c.index.Name, c.spaceName)
	idx.Manifest = &anyblockjson.Manifest{
		Types:      relPaths(root, c.typePaths),
		Options:    relPaths(root, c.optionPaths),
		Properties: anyblockjson.PropertiesFileName,
	}
	idxData, err := anyblockjson.MarshalIndex(&idx)
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}
	if _, err := anyblockjson.UnmarshalIndex(idxData); err != nil {
		return fmt.Errorf("re-read index: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, anyblockjson.IndexFileName), idxData, 0o644); err != nil {
		return fmt.Errorf("write index: %w", err)
	}

	ss.OmittedRelationDocs = c.omittedDocs
	ss.OmittedBytes = c.omittedBytes
	ss.DictionaryInstalled = len(dict.Installed)
	ss.DictionaryEntries = len(dict.Properties)
	ss.ManifestTypes = len(c.typePaths)
	ss.ManifestOptions = len(c.optionPaths)
	ss.DictionaryBytes = len(dictData)
	ss.IndexBytes = len(idxData)
	ss.OrphanUsedKeys = orphans
	return nil
}

// storedRelationDefinition reads the definition a kept relation document
// states, off its stored details — the §2f full entry for a divergent
// installed copy. Members mirror what the document itself would carry.
// storedOption is one option document's contribution to the inline
// vocabulary: the definition the dictionary states, plus the stored `orderId`
// that decides where it sits. The orderId itself never reaches a document —
// it is a lexid, which is exactly the spelling this format keeps out of an
// author's way; the ARRAY POSITION is what carries the order.
type storedOption struct {
	order string
	def   anyblockjson.OptionDefinition
}

func storedRelationDefinition(base *model.SmartBlockSnapshotBase, opts anyblockjson.Options) anyblockjson.PropertyDefinition {
	det := base.GetDetails().GetFields()
	def := anyblockjson.PropertyDefinition{
		Key:         domain.RelationKey(det["relationKey"].GetStringValue()),
		Name:        det["name"].GetStringValue(),
		Format:      model.RelationFormat(int32(det["relationFormat"].GetNumberValue())),
		Description: det["description"].GetStringValue(),
		MaxCount:    int64(det["relationMaxCount"].GetNumberValue()),
		Readonly:    det["relationReadonlyValue"].GetBoolValue(),
	}
	if v := det["relationFormatIncludeTime"]; v != nil {
		if _, isBool := v.GetKind().(*types.Value_BoolValue); isBool {
			b := v.GetBoolValue()
			def.IncludeTime = &b
		}
	}
	if v := det["relationDefaultValue"]; v != nil {
		if _, isNull := v.GetKind().(*types.Value_NullValue); !isNull {
			def.DefaultValue = pbtypes.ValueToInterface(v)
		}
	}
	if v := det["relationFormatObjectTypes"]; v != nil {
		tr, _ := opts.ResolveProperties.(anyblockjson.TypeResolver)
		for _, entry := range pbtypes.GetStringListValue(v) {
			if key, err := bundle.TypeKeyFromUrl(entry); err == nil {
				def.ObjectTypes = append(def.ObjectTypes, string(key))
				continue
			}
			if tr != nil {
				if key, ok := tr.TypeKeyById(entry); ok && key != "" {
					def.ObjectTypes = append(def.ObjectTypes, key)
					continue
				}
			}
			def.ObjectTypes = append(def.ObjectTypes, entry)
		}
	}
	return def
}

// resolvedDefinition asks the space's resolver for a used key's definition —
// the storeresolver path a live export runs on.
func resolvedDefinition(key string, opts anyblockjson.Options) (anyblockjson.PropertyDefinition, bool) {
	r := opts.ResolveProperties
	if r == nil {
		return anyblockjson.PropertyDefinition{}, false
	}
	if id, ok := r.PropertyId(anyblockjson.PropertyDefinition{Key: domain.RelationKey(key)}); ok {
		if def, ok := r.PropertyById(id); ok {
			return def, true
		}
	}
	return anyblockjson.PropertyDefinition{}, false
}

// bundledTargetKeys turns the bundled table's target urls into type keys.
func bundledTargetKeys(urls []string) []string {
	var out []string
	for _, u := range urls {
		if k, err := bundle.TypeKeyFromUrl(u); err == nil {
			out = append(out, string(k))
		}
	}
	return out
}

// bundleRootOf finds the dumped tree's root — the directory the kind folders
// (objects/, types/, …) hang off — so the manifest's paths are relative to
// the two bundle-level files beside them. Every dumped file sits exactly one
// kind folder below the root; if two files disagree, the shorter answer is
// the root that contains both.
func bundleRootOf(written []string) string {
	root := filepath.Dir(filepath.Dir(written[0]))
	for _, f := range written[1:] {
		if r := filepath.Dir(filepath.Dir(f)); len(r) < len(root) {
			root = r
		}
	}
	return root
}

// relPaths rebases a name → path map onto the bundle root.
func relPaths(root string, paths map[string]string) map[string]string {
	if len(paths) == 0 {
		return nil
	}
	out := make(map[string]string, len(paths))
	for k, p := range paths {
		if rel, err := filepath.Rel(root, p); err == nil {
			out[k] = filepath.ToSlash(rel)
		} else {
			out[k] = p
		}
	}
	return out
}

// sortedStringSet lists a map's keys in order — the canonical entry order.
func sortedStringSet(m map[string]anyblockjson.PropertyDefinition) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// firstNonEmpty returns the first non-empty string, so a field the space's
// own document did not state falls back rather than blanking.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
