package chatobject

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/source/sourceimpl"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestScenarioSchema_Constructs(t *testing.T) {
	sc := scenario{
		Name:    "schema-smoke",
		Devices: []string{"A"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "m1", Prev: []string{"G"}, Author: "A", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepAddChanges, Device: "A", ChangeIDs: []string{"G", "m1"}},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 0},
			}},
		},
		Intent: "smoke",
	}
	assert.Equal(t, "schema-smoke", sc.Name)
	assert.Len(t, sc.DAG, 2)
	assert.Equal(t, kindMessage, sc.DAG[1].Kind)
}

// changeKind classifies a synthetic change in a scenario DAG. kindReaction is
// retained for DAG modeling completeness but the reaction unread-counter is out
// of suite scope (only CounterTypeMessage/CounterTypeMention are real
// chatmodel.CounterTypes; reactions are tracked via a separate path).
type changeKind int

const (
	kindSystem changeKind = iota
	kindMessage
	kindMention
	kindReaction
)

type scenarioChange struct {
	ID      string
	Prev    []string
	Author  string
	Kind    changeKind
	ReactTo string // kindReaction: target message id
	Mention string // kindMention: mentioned identity
}

type stepKind int

const (
	stepAddChanges stepKind = iota // make changes visible to a device's tree
	stepReadUpTo                   // device reads up to a message id
	stepMarkUnread                 // device MarkMessagesAsUnread(afterOrderId)
	stepSync                       // merge seenHeads across listed devices
	stepRestart                    // device cold-start: rebuild diff engine
	stepCheckpoint                 // assert
)

type step struct {
	Kind       stepKind
	Device     string
	ChangeIDs  []string
	UpToMsgID  string
	AfterOrder string
	SyncWith   []string
	Counter    chatmodel.CounterType
	Expect     []checkpointExpect
}

type checkpointExpect struct {
	Device      string
	Counter     chatmodel.CounterType
	WantUnread  int
	WantReadIDs []string // optional exact read-flagged ids; nil = don't check
}

type scenario struct {
	Name    string
	Devices []string
	DAG     []scenarioChange
	Steps   []step
	Intent  string
}

func TestBuildScenarioTree_BranchMergeDeterministic(t *testing.T) {
	dag := []scenarioChange{
		{ID: "G", Kind: kindSystem},
		{ID: "m1", Prev: []string{"G"}, Author: "A", Kind: kindMessage},
		{ID: "x1", Prev: []string{"G"}, Author: "B", Kind: kindMessage}, // concurrent
		{ID: "mm", Prev: []string{"m1", "x1"}, Author: "A", Kind: kindSystem}, // merge
	}
	tr1 := buildScenarioTree(t, dag)
	tr2 := buildScenarioTree(t, dag)

	// single head after merge
	assert.Equal(t, []string{"mm"}, tr1.tree.Heads())
	// OrderId assignment is a deterministic function of the DAG (hash-tiebroken
	// siblings) — identical across independent builds
	assert.Equal(t, tr1.orderOf, tr2.orderOf)
	// topological invariant: every change's OrderId > its prev's
	assert.Less(t, tr1.orderOf["G"], tr1.orderOf["m1"])
	assert.Less(t, tr1.orderOf["m1"], tr1.orderOf["mm"])
	assert.Less(t, tr1.orderOf["x1"], tr1.orderOf["mm"])
}

type scenarioTree struct {
	tree    objecttree.ObjectTree
	orderOf map[string]string // changeId -> OrderId
}

func scenarioAclList(t testing.TB) list.AclList {
	keys, err := accountdata.NewRandom()
	require.NoError(t, err)
	acl, err := list.NewInMemoryDerivedAcl("spaceId", keys)
	require.NoError(t, err)
	return acl
}

func scenarioAnystore(t testing.TB) anystore.DB {
	db, err := anystore.Open(context.Background(), filepath.Join(t.TempDir(), "tree.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// buildScenarioTree turns a synthetic change DAG into a real storage-backed
// objecttree.ObjectTree (mirrors store_apply_test.go:205 buildTree). Change ids
// are author-chosen and stable, so the hash-tiebroken sibling order — and thus
// OrderId — is deterministic across independent builds.
func buildScenarioTree(t testing.TB, dag []scenarioChange) *scenarioTree {
	ctx := context.Background()
	acl := scenarioAclList(t)
	mcc := objecttree.NewMockChangeCreator(func() anystore.DB { return scenarioAnystore(t) })

	// root is the first DAG entry (must have no Prev)
	require.NotEmpty(t, dag)
	require.Empty(t, dag[0].Prev, "first DAG change is the root and must have no Prev")
	rootId := dag[0].ID
	storage := mcc.CreateNewTreeStorage(t.(*testing.T), rootId, acl.Head().Id, false)
	if setter, ok := storage.(interface{ SetAddSeq(*atomic.Uint64) }); ok {
		setter.SetAddSeq(&atomic.Uint64{})
	}
	tree, err := objecttree.BuildTestableTree(storage, acl)
	require.NoError(t, err)
	tree.SetFlusher(objecttree.MarkNewChangeFlusher())

	// apply non-root changes in DAG order; ids are author-chosen and stable
	var raws []*treechangeproto.RawTreeChangeWithId
	for _, c := range dag[1:] {
		require.NotEmpty(t, c.Prev, "non-root change %s must have Prev", c.ID)
		raws = append(raws, mcc.CreateRaw(c.ID, acl.Head().Id, rootId, false, c.Prev...))
	}
	heads := computeDagHeads(dag)
	_, err = tree.AddRawChanges(ctx, objecttree.RawChangesPayload{NewHeads: heads, RawChanges: raws})
	require.NoError(t, err)

	orderOf := map[string]string{}
	require.NoError(t, tree.IterateRoot(nil, func(ch *objecttree.Change) bool {
		orderOf[ch.Id] = ch.OrderId
		return true
	}))
	return &scenarioTree{tree: tree, orderOf: orderOf}
}

// computeDagHeads = ids not referenced by any Prev.
func computeDagHeads(dag []scenarioChange) []string {
	ref := map[string]bool{}
	for _, c := range dag {
		for _, p := range c.Prev {
			ref[p] = true
		}
	}
	var heads []string
	for _, c := range dag {
		if !ref[c.ID] {
			heads = append(heads, c.ID)
		}
	}
	return heads
}

func TestDiffEngine_AncestorRemovalGolden(t *testing.T) {
	dag := []scenarioChange{
		{ID: "G", Kind: kindSystem},
		{ID: "m1", Prev: []string{"G"}, Author: "A", Kind: kindMessage},
		{ID: "x1", Prev: []string{"G"}, Author: "B", Kind: kindMessage},
		{ID: "mm", Prev: []string{"m1", "x1"}, Author: "A", Kind: kindSystem},
	}
	st := buildScenarioTree(t, dag)
	var removed []string
	eng := newDiffEngine(t, st.tree, func(ids []string) { removed = append(removed, ids...) })

	// seenHeads {x1}: DAG ancestors of x1 = {G, x1}; m1 (concurrent) NOT removed
	eng.applyDeviceValues([][]string{{"x1"}})
	assert.ElementsMatch(t, []string{"G", "x1"}, removed)
	assert.NotContains(t, removed, "m1")
}

// readCounterEngine is the swap seam: the legacy DAG baseline (*diffEngine)
// and the proposed watermark engine both satisfy it.
type readCounterEngine interface {
	rebuild(seen []string)
	applyDeviceValues(deviceValues [][]string)
	restart(deviceValues [][]string)
}

// engineFactory builds one engine per counter. Default = legacy DAG baseline
// (the committed scenario_report.md golden). Tests swap this to compare.
var engineFactory = func(t testing.TB, tree objecttree.ObjectTree, onRemove func([]string)) readCounterEngine {
	return newDiffEngine(t, tree, onRemove)
}

// watermarkEngine drives the production (OrderId,AddSeq)-dominance predicate
// against the scenario tree + repo (faithful: same predicate store.go uses).
type watermarkEngine struct {
	t        testing.TB
	tree     objecttree.ObjectTree
	onRemove func([]string)
	wm       *sourceimpl.Watermark
}

func newWatermarkEngine(t testing.TB, tree objecttree.ObjectTree, onRemove func([]string)) *watermarkEngine {
	return &watermarkEngine{t: t, tree: tree, onRemove: onRemove, wm: sourceimpl.NewWatermark(onRemove)}
}

func (e *watermarkEngine) resolve(id string) (sourceimpl.ReadPair, bool) {
	ch, err := e.tree.Storage().Get(context.Background(), id)
	if err != nil {
		return sourceimpl.ReadPair{}, false
	}
	return sourceimpl.ReadPair{OrderId: ch.OrderId, AddSeq: ch.AddSeq}, true
}

func (e *watermarkEngine) each(yield func(string, sourceimpl.ReadPair)) {
	_ = e.tree.Storage().GetAfterOrder(context.Background(), "", func(_ context.Context, ch objecttree.StorageChange) (bool, error) {
		yield(ch.Id, sourceimpl.ReadPair{OrderId: ch.OrderId, AddSeq: ch.AddSeq})
		return true, nil
	})
}

func (e *watermarkEngine) rebuild(seen []string) {
	e.wm = sourceimpl.NewWatermark(e.onRemove)
	e.wm.Advance(seen, e.resolve, e.each)
}

func (e *watermarkEngine) applyDeviceValues(dv [][]string) {
	// Merge device values → one advance (mirrors production initDiffManagers,
	// which now applies the union of all per-device seenHeads in one pass).
	var merged []string
	for _, v := range dv {
		merged = append(merged, v...)
	}
	e.wm.Advance(merged, e.resolve, e.each)
}

func (e *watermarkEngine) restart(dv [][]string) { e.rebuild(nil); e.applyDeviceValues(dv) }

// diffEngine wraps the real any-sync objecttree.DiffManager — the exact type
// store.InitDiffManager constructs (core/block/source/sourceimpl/store.go:146).
type diffEngine struct {
	t        testing.TB
	tree     objecttree.ObjectTree
	onRemove func([]string)
	dm       *objecttree.DiffManager
}

func newDiffEngine(t testing.TB, tree objecttree.ObjectTree, onRemove func([]string)) *diffEngine {
	e := &diffEngine{t: t, tree: tree, onRemove: onRemove}
	e.rebuild(nil)
	return e
}

// rebuild mirrors store.InitDiffManager: nil initial seenHeads, treeBuilder
// returns the real scenario tree (NewDiffManager only iterates it).
func (e *diffEngine) rebuild(seen []string) {
	dm, err := objecttree.NewDiffManager(
		seen,
		e.tree.Heads(),
		func(_ []string) (objecttree.ReadableObjectTree, error) { return e.tree, nil },
		e.onRemove,
	)
	require.NoError(e.t, err)
	dm.Init()
	e.dm = dm
}

// applyDeviceValues mirrors store.initDiffManagers: one persisted value per
// device; Remove is called per value (union of per-device seenHeads).
func (e *diffEngine) applyDeviceValues(deviceValues [][]string) {
	for _, v := range deviceValues {
		e.dm.Remove(v)
	}
}

// restart mirrors a cold start: discard in-memory differ, rebuild from the
// union of persisted device values.
func (e *diffEngine) restart(deviceValues [][]string) {
	e.rebuild(nil)
	e.applyDeviceValues(deviceValues)
}

func TestScenarioFixture_LinearReadCount(t *testing.T) {
	dag := []scenarioChange{
		{ID: "G", Kind: kindSystem},
		{ID: "m1", Prev: []string{"G"}, Author: testCreator, Kind: kindMessage},
		{ID: "m2", Prev: []string{"m1"}, Author: "bob", Kind: kindMessage},
		{ID: "m3", Prev: []string{"m2"}, Author: "bob", Kind: kindMessage},
	}
	sf := newScenarioFixture(t, dag)

	// before any read: m2,m3 unread for the local account (own m1 excluded)
	assert.Equal(t, 2, sf.unread(chatmodel.CounterTypeMessage))

	// device reads up to m3 -> seenHeads {m3}; engine removes ancestors; markRead fires
	sf.readUpTo("m3", chatmodel.CounterTypeMessage)
	assert.Equal(t, 0, sf.unread(chatmodel.CounterTypeMessage))
}

// scenarioFixture extends the existing chatobject newFixture (real repository,
// real markRead, real subscription) but replaces its FAKED MarkSeenHeads with
// real per-counter objecttree.DiffManagers built from the scenario DAG.
//
// Suite scope is the two real chatmodel.CounterTypes (Message, Mention);
// reactions are tracked off a separate maxOrderId path, out of scope by design.
type scenarioFixture struct {
	t      testing.TB
	fx     *fixture
	st     *scenarioTree
	engine map[chatmodel.CounterType]readCounterEngine // engineFactory per counter
}

// scenarioCounters is the suite's counter set (Message + Mention).
var scenarioCounters = []chatmodel.CounterType{chatmodel.CounterTypeMessage, chatmodel.CounterTypeMention}

func newScenarioFixture(t testing.TB, dag []scenarioChange) *scenarioFixture {
	fx := newFixture(t.(*testing.T))
	st := buildScenarioTree(t, dag)

	sf := &scenarioFixture{t: t, fx: fx, st: st, engine: map[chatmodel.CounterType]readCounterEngine{}}

	// Real engines, one per counter. onRemove drives the REAL chat
	// markReadMessages — the exact closure chatobject.go:230/236 registers as
	// the diff-manager hook. The fixture registered those hooks into a private
	// map during object.Init (already run inside newFixture), so a post-Init
	// re-capture is impossible; calling the real *storeObject method directly
	// is faithful and exercises the same SetReadFlag -> repo path.
	for _, c := range scenarioCounters {
		cc := c
		sf.engine[cc] = engineFactory(t, st.tree, func(ids []string) {
			require.NoError(t, sf.fx.markReadMessages(ids, cc))
		})
	}

	// Materialize message/mention changes as real chat-repo rows so unread
	// counters are observable through the real repository.
	for _, ch := range dag {
		if ch.Kind == kindMessage || ch.Kind == kindMention {
			sf.addMessage(ch)
		}
	}
	return sf
}

// addMessage inserts a real chat row whose Id == DAG change id and OrderId ==
// tree order, so the real DiffManager.onRemove ids match repo rows. Own-author
// (testCreator) rows are inserted already-read, mirroring the chat system's
// own-message exclusion (chathandler marks own messages read on insert).
func (sf *scenarioFixture) addMessage(ch scenarioChange) {
	isOwn := ch.Author == testCreator
	msg := &chatmodel.Message{
		ChatMessage: &model.ChatMessage{
			Id:          ch.ID,
			OrderId:     sf.st.orderOf[ch.ID],
			Creator:     ch.Author,
			Message:     &model.ChatMessageMessageContent{Text: ch.ID},
			Read:        isOwn,
			HasMention:  ch.Kind == kindMention,
			MentionRead: isOwn,
		},
	}
	require.NoError(sf.t, sf.fx.repository.AddTestMessage(context.Background(), msg))
}

// readUpTo applies a single-device seenHeads value {msgID} to the counter's
// real DiffManager (mirrors one device's persisted KeyValueService value).
func (sf *scenarioFixture) readUpTo(msgID string, c chatmodel.CounterType) {
	sf.engine[c].applyDeviceValues([][]string{{msgID}})
}

// unread reads the real repository's unread count for the counter. The real
// markReadMessages -> SetReadFlag writes land in this same repo, so this is the
// end-to-end characterization point (DiffManager -> markRead -> repo).
func (sf *scenarioFixture) unread(c chatmodel.CounterType) int {
	ids, err := sf.fx.repository.GetAllUnreadMessages(context.Background(), c)
	require.NoError(sf.t, err)
	return len(ids)
}

func TestScenario_TwoDeviceOfflineReadThenSync(t *testing.T) {
	dag := []scenarioChange{
		{ID: "G", Kind: kindSystem},
		{ID: "a1", Prev: []string{"G"}, Author: "alice", Kind: kindMessage},
		{ID: "a2", Prev: []string{"a1"}, Author: "alice", Kind: kindMessage},
	}
	sf := newScenarioFixture(t, dag)
	dev := newDeviceModel([]string{"A", "B"})

	// device B reads up to a2 while "offline" (its own seenHeads value)
	dev.readUpTo("B", "a2")
	// sync union of A+B values into the real engine
	sf.syncDevices(dev, []string{"A", "B"}, chatmodel.CounterTypeMessage)

	// local account != alice -> a1,a2 are peer messages; after B read+sync, 0 unread
	assert.Equal(t, 0, sf.unread(chatmodel.CounterTypeMessage))
}

// deviceState holds one device's persisted seenHeads value per counter (the
// value the real KeyValueService persists). Keyed by chatmodel.CounterType to
// stay consistent with scenarioFixture.engine (the plan keyed by diff-manager
// name, dropped since the reaction counter is out of scope).
type deviceState struct {
	seen map[chatmodel.CounterType][]string
}

type deviceModel struct {
	devices map[string]*deviceState
}

func newDeviceModel(ids []string) *deviceModel {
	m := &deviceModel{devices: map[string]*deviceState{}}
	for _, id := range ids {
		m.devices[id] = &deviceState{seen: map[chatmodel.CounterType][]string{}}
	}
	return m
}

// readUpTo records a device's seenHeads value for every in-scope counter.
func (m *deviceModel) readUpTo(device, msgID string) {
	d := m.devices[device]
	for _, c := range scenarioCounters {
		d.seen[c] = append(d.seen[c], msgID)
	}
}

func (m *deviceModel) deviceValues(devices []string, c chatmodel.CounterType) [][]string {
	var vals [][]string
	for _, id := range devices {
		if v := m.devices[id].seen[c]; len(v) > 0 {
			vals = append(vals, v)
		}
	}
	return vals
}

// syncDevices mirrors store.initDiffManagers reading all persisted values:
// rebuild the engine and Remove per device value (union).
func (sf *scenarioFixture) syncDevices(m *deviceModel, devices []string, c chatmodel.CounterType) {
	sf.engine[c].restart(m.deviceValues(devices, c))
}

// checkpoint compares one expectation; returns (got, ok).
func (sf *scenarioFixture) checkpoint(e checkpointExpect) (int, bool) {
	got := sf.unread(e.Counter)
	return got, got == e.WantUnread
}

type checkResult struct {
	Scenario string
	StepIdx  int
	Device   string
	Counter  chatmodel.CounterType
	Want     int
	Got      int
	Match    bool
}

func runScenario(t testing.TB, sc scenario) []checkResult {
	sf := newScenarioFixture(t, sc.DAG)
	dev := newDeviceModel(sc.Devices)
	var results []checkResult
	for i, s := range sc.Steps {
		switch s.Kind {
		case stepAddChanges:
			// changes already materialized at fixture build; no-op for now
		case stepReadUpTo:
			dev.readUpTo(s.Device, s.UpToMsgID)
			sf.readUpTo(s.UpToMsgID, s.Counter)
		case stepMarkUnread:
			// AfterOrder is authored as a change id (the suite addresses by id;
			// OrderId is opaque tree state). "" passes through = mark all unread.
			after := s.AfterOrder
			if oid, ok := sf.st.orderOf[after]; ok {
				after = oid
			}
			require.NoError(t, sf.fx.storeObject.MarkMessagesAsUnread(context.Background(), after, s.Counter))
		case stepSync:
			sf.syncDevices(dev, s.SyncWith, s.Counter)
		case stepRestart:
			sf.engine[s.Counter].restart(dev.deviceValues(sc.Devices, s.Counter))
		case stepCheckpoint:
			for _, e := range s.Expect {
				got, ok := sf.checkpoint(e)
				results = append(results, checkResult{sc.Name, i, e.Device, e.Counter, e.WantUnread, got, ok})
			}
		}
	}
	return results
}

// runCatalog runs every scenario twice (intra-run determinism) and records
// results. Characterization mode: does NOT fail on Want!=Got (those are
// findings); DOES fail on harness errors and non-determinism.
func runCatalog(t testing.TB, catalog []scenario) {
	var all []checkResult
	for _, sc := range catalog {
		r1 := runScenario(t, sc)
		r2 := runScenario(t, sc)
		require.Equal(t, len(r1), len(r2), "scenario %s: result count not deterministic", sc.Name)
		for i := range r1 {
			require.Equal(t, r1[i].Got, r2[i].Got, "scenario %s step %d: non-deterministic actual", sc.Name, r1[i].StepIdx)
		}
		all = append(all, r1...)
	}
	require.NoError(t, writeScenarioReport(catalog, all))
}

func TestScenarioReport_Generated(t *testing.T) {
	runCatalog(t, scenarioCatalog)
	b, err := os.ReadFile("testdata/scenario_report.md")
	require.NoError(t, err)
	s := string(b)
	assert.Contains(t, s, "# Chat Read-Counter Scenario Report")
	assert.Contains(t, s, "S-baseline-linear")
	assert.Contains(t, s, "S-concurrent-merge-divergence")
	// the crux scenario must be flagged when current-actual != intended
	assert.Regexp(t, `S-concurrent-merge-divergence[\s\S]*?(MATCH|DIVERGENCE)`, s)
}

// writeScenarioReport renders the intended-vs-actual characterization report.
// Intended = strict DAG-ancestor (current contract, authored in the catalog);
// Actual = current system behavior. DIVERGENCE rows are findings for the
// redesign decision, not regressions.
func writeScenarioReport(catalog []scenario, results []checkResult) error {
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		return fmt.Errorf("mkdir testdata: %w", err)
	}
	byScenario := map[string][]checkResult{}
	for _, r := range results {
		byScenario[r.Scenario] = append(byScenario[r.Scenario], r)
	}
	var b strings.Builder
	b.WriteString("# Chat Read-Counter Scenario Report\n\n")
	b.WriteString("Generated by TestChatReadCounterScenarios. Intended = strict DAG-ancestor (current contract); Actual = current system behavior. DIVERGENCE rows are findings for the redesign decision, not regressions.\n\n")
	for _, sc := range catalog {
		b.WriteString(fmt.Sprintf("## %s\n\n%s\n\n", sc.Name, sc.Intent))
		for _, r := range byScenario[sc.Name] {
			status := "MATCH"
			if !r.Match {
				status = "DIVERGENCE"
			}
			b.WriteString(fmt.Sprintf("- step %d · device %s · %s · want=%d got=%d · **%s**\n",
				r.StepIdx, r.Device, r.Counter.DiffManagerName(), r.Want, r.Got, status))
		}
		b.WriteString("\n")
	}
	return os.WriteFile("testdata/scenario_report.md", []byte(b.String()), 0o644)
}
