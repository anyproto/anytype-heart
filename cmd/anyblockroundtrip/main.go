// anyblockroundtrip verifies the AnyBlock JSON round-trip (pkg/lib/anyblockjson)
// against a real account: it recovers the account from a mnemonic, exports every
// object of every space to pb snapshots, converts each snapshot pb → AnyBlock
// JSON → pb, and checks the contract (Export ∘ Import byte-stable, no
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

	"github.com/anyproto/anytype-heart/core"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	libcore "github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
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
	)
	flag.Parse()

	if *mnemonic == "" || *rootPath == "" {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "\nboth -mnemonic (or $ANYTYPE_MNEMONIC) and -root-path are required")
		os.Exit(2)
	}
	if err := run(*mnemonic, *accountId, *rootPath, *outDir, *spaceFilter, *limit, *keepExports); err != nil {
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

func run(mnemonic, accountId, rootPath, outDir, spaceFilter string, limit int, keepExports bool) error {
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
		ss, err := processSpace(ctx, mw, store, s.id, outDir, limit, keepExports)
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
	// the array-form trigger for table cells).
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
	spaceId, outDir string, limit int, keepExports bool) (*spaceSummary, error) {

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

	resolvers := newSpaceResolvers(store.SpaceIndex(spaceId))
	opts := anyblockjson.Options{
		ResolveFormat:     resolvers.resolveFormat,
		ResolveOptions:    resolvers,
		ResolveProperties: resolvers,
	}

	ss := &spaceSummary{SpaceId: spaceId, Total: len(files), Categories: map[string]int{}, IndentHistogram: map[int]int{}}
	for _, f := range files {
		objectId := strings.TrimSuffix(filepath.Base(f), ".pb")
		issues, artifacts, err := roundtripFile(f, opts)
		if err != nil {
			return nil, fmt.Errorf("object %s: %w", objectId, err)
		}
		if artifacts != nil {
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
	if string(json1) != string(json2) {
		arts.json2 = json2
		fail("not_byte_stable", "first divergence: %s", firstDiff(json1, json2))
	}

	for _, d := range lossIssues(base, reimported, opts) {
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
// the trigger for the array-form cell encoding.
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
// ---- loss heuristics ----
//

// strippedKeys mirrors the export-side strip set: LocalAndDerived minus
// the keys the importer meaningfully preserves.
var strippedKeys = func() map[string]bool {
	kept := map[string]bool{
		"createdDate": true, "lastModifiedDate": true, "creator": true,
		"isFavorite": true, "isArchived": true, "resolvedLayout": true,
	}
	out := map[string]bool{"id": true, "type": true}
	for _, k := range bundle.LocalAndDerivedRelationKeys {
		if !kept[string(k)] {
			out[string(k)] = true
		}
	}
	return out
}()

// lossIssues compares the original snapshot with the reimported one on the
// axes the format promises to preserve: detail values (up to the documented
// normalizations) and the text content of non-structural text blocks. It is a
// heuristic — findings are triage input, not proof.
func lossIssues(orig, got *model.SmartBlockSnapshotBase, opts anyblockjson.Options) []string {
	var out []string

	if orig.Details != nil {
		gotFields := map[string]*types.Value{}
		if got.Details != nil {
			gotFields = got.Details.Fields
		}
		keys := make([]string, 0, len(orig.Details.Fields))
		for k := range orig.Details.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if strippedKeys[k] {
				continue
			}
			if !detailEqual(k, orig.Details.Fields[k], gotFields[k], opts) {
				out = append(out, fmt.Sprintf("detail %q changed: %s -> %s",
					k, valuePreview(orig.Details.Fields[k]), valuePreview(gotFields[k])))
			}
		}
	}

	origTexts := textInventory(orig)
	gotTexts := textInventory(got)
	for text, n := range origTexts {
		if gotTexts[text] < n {
			out = append(out, fmt.Sprintf("text block lost (%dx): %q", n-gotTexts[text], preview(text)))
		}
	}
	return out
}

// detailEqual compares one detail value up to the documented normalizations:
// scalars of list-shaped formats become single-element lists, dates truncate
// to whole seconds.
func detailEqual(key string, a, b *types.Value, opts anyblockjson.Options) bool {
	if b == nil {
		return false
	}
	if recommendedDetailKeys[key] && opts.ResolveProperties != nil {
		return equalStrings(
			normalizeRecommended(stringsOf(a), opts.ResolveProperties),
			normalizeRecommended(stringsOf(b), opts.ResolveProperties))
	}
	format, _ := resolveFormat(key, opts)
	switch format {
	case model.RelationFormat_object, model.RelationFormat_file,
		model.RelationFormat_status, model.RelationFormat_tag:
		// mirror the format's list extraction: scalars wrap, empty strings drop
		return equalStrings(stringsOf(a), stringsOf(b))
	case model.RelationFormat_date:
		return int64(a.GetNumberValue()) == int64(b.GetNumberValue())
	}
	return proto.Equal(a, b)
}

// recommendedDetailKeys are the four lists a type document lifts into typeProperties.
// They round-trip by property KEY, and legacy data mixes ids and bare keys,
// so comparison normalizes both sides to keys and skips entries neither
// side can resolve (dropped-by-design, like missing-object sentinels).
var recommendedDetailKeys = map[string]bool{
	"recommendedFeaturedRelations": true,
	"recommendedRelations":         true,
	"recommendedFileRelations":     true,
	"recommendedHiddenRelations":   true,
}

func normalizeRecommended(entries []string, r anyblockjson.PropertyResolver) []string {
	var out []string
	for _, id := range entries {
		if def, ok := r.PropertyById(id); ok {
			out = append(out, string(def.Key))
			continue
		}
		if _, ok := r.PropertyId(anyblockjson.PropertyDefinition{Key: domain.RelationKey(id)}); ok {
			out = append(out, id) // already a key
			continue
		}
		if _, err := bundle.GetRelation(domain.RelationKey(id)); err == nil {
			out = append(out, id) // bundle key without a space object
		}
		// otherwise unresolvable: dropped by design on export, skip
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// missingObjectSentinel marks a dangling object reference in stored details
// (pkg/lib/localstore/addr). Export legitimately drops these unresolvable
// refs, so the comparison must not count them as loss.
const missingObjectSentinel = "_missing_object"

// stringsOf reads a value as the format's string list: single strings wrap,
// empty strings drop (the export-side valueStringList semantics), and
// pre-broken missing-object sentinels are ignored.
func stringsOf(v *types.Value) []string {
	if s := v.GetStringValue(); s != "" && s != missingObjectSentinel {
		return []string{s}
	}
	var out []string
	for _, el := range v.GetListValue().GetValues() {
		if s := el.GetStringValue(); s != "" && s != missingObjectSentinel {
			out = append(out, s)
		}
	}
	return out
}

func resolveFormat(key string, opts anyblockjson.Options) (model.RelationFormat, bool) {
	if f, err := bundle.GetRelationFormat(domain.RelationKey(key)); err == nil {
		return f, true
	}
	if opts.ResolveFormat != nil {
		return opts.ResolveFormat(domain.RelationKey(key))
	}
	return 0, false
}

// textInventory counts the plain text of text blocks the format preserves.
// Structural styles are dropped by design; blocks with emoji marks are skipped
// because emoji materialization changes the text lossily by design.
func textInventory(s *model.SmartBlockSnapshotBase) map[string]int {
	out := map[string]int{}
	for _, b := range s.Blocks {
		t := b.GetText()
		if t == nil || t.Text == "" {
			continue
		}
		switch t.Style {
		case model.BlockContentText_Title, model.BlockContentText_Description:
			continue
		}
		skip := false
		for _, m := range t.Marks.GetMarks() {
			if m != nil && m.Type == model.BlockContentTextMark_Emoji {
				skip = true
				break
			}
		}
		if !skip {
			out[t.Text]++
		}
	}
	return out
}

func preview(s string) string {
	if len(s) > 80 {
		return s[:80] + "…"
	}
	return s
}

func valuePreview(v *types.Value) string {
	if v == nil {
		return "<absent>"
	}
	s := v.String()
	return preview(s)
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

//
// ---- objectstore-backed resolvers ----
//

type spaceResolvers struct {
	index      spaceindex.Store
	optionsFor map[domain.RelationKey][]*model.RelationOption

	relsLoaded bool
	relById    map[string]anyblockjson.PropertyDefinition
	relKeyToId map[string]string
}

func newSpaceResolvers(index spaceindex.Store) *spaceResolvers {
	return &spaceResolvers{index: index, optionsFor: map[domain.RelationKey][]*model.RelationOption{}}
}

// loadRelations snapshots the space's relation objects once: the point
// lookups (GetRelationByKey) miss for some legacy relations that the full
// listing still returns, so the map is the primary source and the point
// lookups are the fallback.
func (r *spaceResolvers) loadRelations() {
	if r.relsLoaded {
		return
	}
	r.relsLoaded = true
	r.relById = map[string]anyblockjson.PropertyDefinition{}
	r.relKeyToId = map[string]string{}
	rels, err := r.index.ListAllRelations()
	if err != nil {
		return
	}
	for _, rel := range rels {
		if rel == nil || rel.Relation == nil {
			continue
		}
		def := anyblockjson.PropertyDefinition{
			Key:    domain.RelationKey(rel.Key),
			Name:   rel.Name,
			Format: rel.Format,
		}
		r.relById[rel.Id] = def
		if _, taken := r.relKeyToId[rel.Key]; !taken {
			r.relKeyToId[rel.Key] = rel.Id
		}
	}
}

func (r *spaceResolvers) resolveFormat(key domain.RelationKey) (model.RelationFormat, bool) {
	rel, err := r.index.GetRelationByKey(string(key))
	if err != nil || rel == nil {
		return 0, false
	}
	return rel.Format, true
}

func (r *spaceResolvers) options(key domain.RelationKey) []*model.RelationOption {
	if cached, ok := r.optionsFor[key]; ok {
		return cached
	}
	opts, err := r.index.ListRelationOptions(key)
	if err != nil {
		opts = nil
	}
	r.optionsFor[key] = opts
	return opts
}

func (r *spaceResolvers) OptionName(key domain.RelationKey, id string) (string, bool) {
	for _, o := range r.options(key) {
		if o.Id == id {
			return o.Text, true
		}
	}
	return "", false
}

func (r *spaceResolvers) OptionId(key domain.RelationKey, name string) (string, bool) {
	for _, o := range r.options(key) {
		if o.Text == name {
			return o.Id, true
		}
	}
	return "", false
}

func (r *spaceResolvers) PropertyById(id string) (anyblockjson.PropertyDefinition, bool) {
	r.loadRelations()
	if def, ok := r.relById[id]; ok {
		return def, true
	}
	rel, err := r.index.GetRelationById(id)
	if err != nil || rel == nil {
		return anyblockjson.PropertyDefinition{}, false
	}
	def := anyblockjson.PropertyDefinition{
		Key:    domain.RelationKey(rel.Key),
		Name:   rel.Name,
		Format: rel.Format,
	}
	// cache the point-lookup hit both ways: some relations resolve by id but
	// are absent from the listing AND the by-key lookup (deleted or index
	// gap — anomaly #9 class), so without this PropertyId cannot invert the
	// key export just produced and the entry is dropped on re-export
	// (resolvers must be equivalent both directions)
	r.relById[id] = def
	if _, taken := r.relKeyToId[rel.Key]; !taken {
		r.relKeyToId[rel.Key] = id
	}
	return def, true
}

func (r *spaceResolvers) PropertyId(def anyblockjson.PropertyDefinition) (string, bool) {
	r.loadRelations()
	if id, ok := r.relKeyToId[string(def.Key)]; ok {
		return id, true
	}
	rel, err := r.index.GetRelationByKey(string(def.Key))
	if err != nil || rel == nil {
		return "", false
	}
	r.relKeyToId[rel.Key] = rel.Id
	return rel.Id, true
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
