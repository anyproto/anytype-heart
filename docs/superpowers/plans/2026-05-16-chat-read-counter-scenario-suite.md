# Chat Read-Counter Multi-Device Scenario Suite — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a deterministic, table-driven Go suite that exercises the *real* chat read-counter pipeline (real `objecttree.DiffManager` over a scenario-built tree → real `chatrepository` unread counts) across multi-device collaboration scenarios, recording intended vs current-actual behavior as characterization goldens before any redesign.

**Architecture:** Each scenario = a synthetic change DAG + ordered per-device steps. A harness builds a real `objecttree.ObjectTree` from the DAG (`objecttree.NewMockChangeCreator`/`CreateRaw`/`AddRawChanges`, the `store_apply_test.go:205` pattern), constructs the real any-sync `objecttree.DiffManager` over it, and reuses the existing `chatobject` test fixture's real repository/markRead/subscription — replacing only that fixture's *faked* `MarkSeenHeads` (crude `msg.Id <= seen`, `chatobject_test.go:~192`) with the real `DiffManager`. Multi-device is the per-device seenHeads-value union loop from `store.initDiffManagers`. Characterization mode records intended vs actual without failing on semantic divergence.

**Tech Stack:** Go, testify, `github.com/anyproto/any-sync/.../objecttree`, `anytype-heart` `chatobject`/`chatrepository`, `anystore`.

**Spec:** `docs/superpowers/specs/2026-05-16-chat-read-counter-multidevice-scenarios-design.md` (hybrid level, A1 table-driven — both decided).

---

## File Structure

- **Create** `core/block/editor/chatobject/scenario_harness_test.go` — scenario types, `buildScenarioTree`, `diffEngine`, `newScenarioFixture`, device model, `runScenario`, report writer. One responsibility: the harness.
- **Create** `core/block/editor/chatobject/scenario_catalog_test.go` — the scenario table (`scenarioCatalog`) + `TestChatReadCounterScenarios`. One responsibility: the catalog + entry point.
- **Create** `core/block/editor/chatobject/testdata/scenario_report.md` — generated, committed for review.

All files are `package chatobject` test files (access the existing `newFixture`, `storeObject`, `chatId`, `testSpaceId`, `testCreator`).

Type names (used consistently across tasks): `changeKind`, `scenarioChange`, `stepKind`, `step`, `checkpointExpect`, `scenario`, `buildScenarioTree`, `diffEngine`, `newScenarioFixture`, `deviceState`, `runScenario`.

---

### Task 1: Scenario schema types

**Files:**
- Create: `core/block/editor/chatobject/scenario_harness_test.go`

- [ ] **Step 1: Write the failing test**

```go
package chatobject

import (
	"testing"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/stretchr/testify/assert"
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./core/block/editor/chatobject/ -run TestScenarioSchema_Constructs`
Expected: FAIL — `undefined: scenario` / `scenarioChange` / `kindSystem` / `stepAddChanges` / `checkpointExpect`.

- [ ] **Step 3: Write minimal implementation**

Append to `scenario_harness_test.go` (after imports — keep the test above; add the types):

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./core/block/editor/chatobject/ -run TestScenarioSchema_Constructs`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add core/block/editor/chatobject/scenario_harness_test.go
git commit -m "GO-7290 Add chat read-counter scenario schema types"
```

---

### Task 2: `buildScenarioTree` — DAG → real object tree

**Files:**
- Modify: `core/block/editor/chatobject/scenario_harness_test.go`

Reference pattern: `core/block/source/sourceimpl/store_apply_test.go:185-219` (`buildTree`, `MockChangeCreator`, `BuildTestableTree`, `MarkNewChangeFlusher`). any-sync APIs: `objecttree.NewMockChangeCreator(func() anystore.DB)`, `mcc.CreateRoot(id, aclId)`, `mcc.CreateRaw(id, aclId, snapshotId string, isSnapshot bool, prevIds ...string)`, `mcc.CreateNewTreeStorage(t, treeId, aclHeadId string, isDerived bool) Storage`, `objecttree.BuildTestableTree(storage, aclList)`, `tree.AddRawChanges(ctx, objecttree.RawChangesPayload{NewHeads, RawChanges})`.

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./core/block/editor/chatobject/ -run TestBuildScenarioTree_BranchMergeDeterministic`
Expected: FAIL — `undefined: buildScenarioTree`.

- [ ] **Step 3: Write minimal implementation**

Append to `scenario_harness_test.go`. Add imports `context`, `sync/atomic`, `os`, `path/filepath`, `testing`, plus:
`anystore "github.com/anyproto/any-store"`,
`"github.com/anyproto/any-sync/commonspace/object/acl/list"`,
`"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"`,
`"github.com/anyproto/any-sync/util/crypto"`,
`"github.com/anyproto/any-sync/app/ocache"` is NOT needed,
`accountdata "github.com/anyproto/any-sync/accountservice"` is NOT the acl one — use `"github.com/anyproto/any-sync/commonspace/object/acl/list"` and `"github.com/anyproto/any-sync/util/crypto"`; for acl keys use `"github.com/anyproto/any-sync/accountservice"`? No — match `store_apply_test.go` which uses `accountdata "github.com/anyproto/any-sync/util/crypto"`? Use exactly the imports `store_apply_test.go` uses for `prepareAclList`: `"github.com/anyproto/any-sync/commonspace/object/acl/list"` and `accountdata "github.com/anyproto/any-sync/commonspace/object/accountdata"`.

```go
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
```

Add import `"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"` and `"github.com/stretchr/testify/require"`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./core/block/editor/chatobject/ -run TestBuildScenarioTree_BranchMergeDeterministic -count=1`
Expected: PASS. If `AddRawChanges` rejects unknown root snapshotId, adjust `mcc.CreateRaw`'s `snapshotId` to `rootId` (already set) and ensure `dag[0]` produced the root via `CreateNewTreeStorage` (it does). If a sibling order assertion is environment-flaky, that itself is a finding — STOP and report (determinism is a suite premise).

- [ ] **Step 5: Commit**

```bash
git add core/block/editor/chatobject/scenario_harness_test.go
git commit -m "GO-7290 Add scenario DAG to real object tree builder"
```

---

### Task 3: `diffEngine` — real DiffManager + multi-device union

**Files:**
- Modify: `core/block/editor/chatobject/scenario_harness_test.go`

Mirrors `store.InitDiffManager` (`core/block/source/sourceimpl/store.go:146`): `objecttree.NewDiffManager(seenHeads, curHeads, treeBuilder, onRemove)`; and `store.initDiffManagers` (`:113-133`): per persisted value, `differ.Remove(seenHeads)`.

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./core/block/editor/chatobject/ -run TestDiffEngine_AncestorRemovalGolden -count=1`
Expected: FAIL — `undefined: newDiffEngine`.

- [ ] **Step 3: Write minimal implementation**

Append:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./core/block/editor/chatobject/ -run TestDiffEngine_AncestorRemovalGolden -count=1`
Expected: PASS (removed == {G, x1}; m1 absent — confirms real DAG ancestor semantics, the behavior under characterization).

- [ ] **Step 5: Commit**

```bash
git add core/block/editor/chatobject/scenario_harness_test.go
git commit -m "GO-7290 Add real DiffManager engine with multi-device union"
```

---

### Task 4: `newScenarioFixture` — real repo/markRead wired to the real engine

**Files:**
- Modify: `core/block/editor/chatobject/scenario_harness_test.go`

Reuses the existing `newFixture` (`chatobject_test.go:114`). That fixture registers `onSeenHooks` via `source.EXPECT().RegisterDiffManager(...)` and fakes `MarkSeenHeads`. We capture the hooks and add scenario messages to the real repo so unread counts are observable via `fx.GetMessages`.

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./core/block/editor/chatobject/ -run TestScenarioFixture_LinearReadCount -count=1`
Expected: FAIL — `undefined: newScenarioFixture`.

- [ ] **Step 3: Write minimal implementation**

Append. `addScenarioMessage` inserts a chat message into the real repo with `Id == changeId` and `OrderId == tree order` so counters are observable; reuse the fixture's existing message-insert path used by `reading_test.go` (`fx.applyToStore` via `PushStoreChange`) — concretely, call the same helper the chatobject tests use to add a message. Use `fx.add*`/`PushStoreChange` analog: the chatobject fixture exposes message creation through normal chat add; the minimal deterministic route is to write directly via the real repository the fixture already wired (`fx.storeObject.repository...`). Implementation:

```go
type scenarioFixture struct {
	t      testing.TB
	fx     *fixture
	st     *scenarioTree
	engine map[chatmodel.CounterType]*diffEngine // one per counter
	hooks  map[string]func([]string)
}

func counterDiffName(c chatmodel.CounterType) string { return c.DiffManagerName() }

func newScenarioFixture(t testing.TB, dag []scenarioChange) *scenarioFixture {
	fx := newFixture(t.(*testing.T))
	st := buildScenarioTree(t, dag)

	sf := &scenarioFixture{t: t, fx: fx, st: st, engine: map[chatmodel.CounterType]*diffEngine{}, hooks: map[string]func([]string){}}

	// capture the real chatobject onSeen hooks (registered in storeObject.Init
	// via source.RegisterDiffManager). The existing fixture stored them in a
	// local map; re-register through the same mock to capture here.
	sf.fx.source.EXPECT().RegisterDiffManager(mock.Anything, mock.Anything).Run(func(name string, hook func([]string)) {
		sf.hooks[name] = hook
	}).Return().Maybe()

	// real engines, one per counter, onRemove -> the real chat markRead hook
	for _, c := range []chatmodel.CounterType{chatmodel.CounterTypeMessage, chatmodel.CounterTypeMention, chatmodel.CounterTypeReaction} {
		cc := c
		sf.engine[cc] = newDiffEngine(t, st.tree, func(ids []string) {
			if h := sf.hooks[counterDiffName(cc)]; h != nil {
				h(ids)
			}
		})
	}

	// replace the fixture's FAKE MarkSeenHeads with the real engine
	sf.fx.source.EXPECT().MarkSeenHeads(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, name string, seen []string) error {
			for c, eng := range sf.engine {
				if counterDiffName(c) == name {
					eng.applyDeviceValues([][]string{seen})
				}
			}
			return nil
		}).Maybe()

	// materialize kindMessage/kindMention changes as real chat messages so
	// unread counters are observable through the real repository.
	for _, ch := range dag {
		if ch.Kind == kindMessage || ch.Kind == kindMention {
			sf.addMessage(ch)
		}
	}
	return sf
}

func (sf *scenarioFixture) addMessage(ch scenarioChange) {
	msg := chatmodel.NewMessage()
	msg.Id = ch.ID
	msg.OrderId = sf.st.orderOf[ch.ID]
	msg.Creator = ch.Author
	msg.Message = &model.ChatMessageMessageContent{Text: ch.ID}
	if ch.Kind == kindMention {
		msg.HasMention = true
	}
	require.NoError(sf.t, sf.fx.storeObject.repository.AddMessage(context.Background(), chatId, msg))
}

func (sf *scenarioFixture) readUpTo(msgID string, c chatmodel.CounterType) {
	require.NoError(sf.t, sf.engineApplySeen(c, []string{msgID}))
}

func (sf *scenarioFixture) engineApplySeen(c chatmodel.CounterType, seen []string) error {
	sf.engine[c].applyDeviceValues([][]string{seen})
	return nil
}

func (sf *scenarioFixture) unread(c chatmodel.CounterType) int {
	resp, err := sf.fx.GetMessages(context.Background(), chatrepository.GetMessagesRequest{})
	require.NoError(sf.t, err)
	switch c {
	case chatmodel.CounterTypeMessage:
		return int(resp.ChatState.Messages.Counter)
	case chatmodel.CounterTypeMention:
		return int(resp.ChatState.Mentions.Counter)
	default:
		ids, err := sf.fx.storeObject.repository.GetAllUnreadReactionChangeIds(context.Background())
		require.NoError(sf.t, err)
		return len(ids)
	}
}
```

Add imports: `"github.com/stretchr/testify/mock"`, `"github.com/anyproto/anytype-heart/core/block/chats/chatrepository"`, `"github.com/anyproto/anytype-heart/pkg/lib/pb/model"` (alias `model`), `"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"`.

> If `chatmodel.NewMessage()` / `repository.AddMessage` signatures differ, use the exact constructor + insert used in `core/block/chats/chatrepository/repository_test.go:46` (`fixture.addMessage`) — match that call shape. Verify with: `grep -n "func.*AddMessage\|func NewMessage" core/block/chats/chatrepository/repository.go core/block/chats/chatmodel/*.go` and adjust the two lines accordingly. This is a known-narrow lookup, not a placeholder: the insert must produce a row with `Id`, `OrderId`, `Creator`, `HasMention`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./core/block/editor/chatobject/ -run TestScenarioFixture_LinearReadCount -count=1`
Expected: PASS. If unread is 3 not 2, the own-author exclusion uses `Creator==testCreator`; ensure `m1.Author == testCreator`. If 0 before read, the messages were auto-read on insert — set them unread explicitly via the repo insert flags (mirror `repository_test.go:46` `read=false`).

- [ ] **Step 5: Commit**

```bash
git add core/block/editor/chatobject/scenario_harness_test.go
git commit -m "GO-7290 Add scenario fixture wiring real engine to real repo"
```

---

### Task 5: Device model, multi-device sync, restart, checkpoint

**Files:**
- Modify: `core/block/editor/chatobject/scenario_harness_test.go`

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./core/block/editor/chatobject/ -run TestScenario_TwoDeviceOfflineReadThenSync -count=1`
Expected: FAIL — `undefined: newDeviceModel` / `syncDevices`.

- [ ] **Step 3: Write minimal implementation**

Append:

```go
type deviceState struct {
	seen map[string][]string // counterDiffName -> seenHeads value for this device
}

type deviceModel struct {
	devices map[string]*deviceState
}

func newDeviceModel(ids []string) *deviceModel {
	m := &deviceModel{devices: map[string]*deviceState{}}
	for _, id := range ids {
		m.devices[id] = &deviceState{seen: map[string][]string{}}
	}
	return m
}

// readUpTo records a device's seenHeads value for all counters (the value the
// real KeyValueService would persist for that device).
func (m *deviceModel) readUpTo(device, msgID string) {
	d := m.devices[device]
	for _, c := range []chatmodel.CounterType{chatmodel.CounterTypeMessage, chatmodel.CounterTypeMention, chatmodel.CounterTypeReaction} {
		n := counterDiffName(c)
		d.seen[n] = append(d.seen[n], msgID)
	}
}

func (m *deviceModel) deviceValues(devices []string, c chatmodel.CounterType) [][]string {
	n := counterDiffName(c)
	var vals [][]string
	for _, id := range devices {
		if v := m.devices[id].seen[n]; len(v) > 0 {
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./core/block/editor/chatobject/ -run TestScenario_TwoDeviceOfflineReadThenSync -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add core/block/editor/chatobject/scenario_harness_test.go
git commit -m "GO-7290 Add device model, sync, restart and checkpoint"
```

---

### Task 6: `runScenario` + the two spec worked scenarios + determinism

**Files:**
- Modify: `core/block/editor/chatobject/scenario_harness_test.go`
- Create: `core/block/editor/chatobject/scenario_catalog_test.go`

- [ ] **Step 1: Write the failing test** (`scenario_catalog_test.go`)

```go
package chatobject

import (
	"testing"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
)

var scenarioCatalog = []scenario{
	{
		Name:    "S-baseline-linear",
		Devices: []string{"A"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "m1", Prev: []string{"G"}, Author: testCreator, Kind: kindMessage},
			{ID: "m2", Prev: []string{"m1"}, Author: "bob", Kind: kindMessage},
			{ID: "m3", Prev: []string{"m2"}, Author: "bob", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 2}}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "m3", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
		},
		Intent: "Linear single-device: reading the head clears all peer messages.",
	},
	{
		Name:    "S-concurrent-merge-divergence",
		Devices: []string{"A", "B"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "m1", Prev: []string{"G"}, Author: "alice", Kind: kindMessage},
			{ID: "x1", Prev: []string{"G"}, Author: "bob", Kind: kindMessage},
			{ID: "mm", Prev: []string{"m1", "x1"}, Author: "alice", Kind: kindSystem},
		},
		Steps: []step{
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "x1", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepSync, SyncWith: []string{"A", "B"}, Counter: chatmodel.CounterTypeMessage},
			// Intent = strict DAG-ancestor: ancestors({x1})={G,x1}; m1 stays unread => 1.
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 1}}},
		},
		Intent: "DAG-ancestor model: seeing x1 does not clear concurrent m1. Crux differential case vs OrderId-prefix redesign.",
	},
}

func TestChatReadCounterScenarios(t *testing.T) {
	runCatalog(t, scenarioCatalog)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./core/block/editor/chatobject/ -run TestChatReadCounterScenarios -count=1`
Expected: FAIL — `undefined: runCatalog`.

- [ ] **Step 3: Write minimal implementation** (append to `scenario_harness_test.go`)

```go
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
			require.NoError(t, sf.fx.storeObject.MarkMessagesAsUnread(context.Background(), s.AfterOrder, s.Counter))
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./core/block/editor/chatobject/ -run TestChatReadCounterScenarios -count=1`
Expected: PASS (it asserts determinism + writes the report; it does NOT fail when Want!=Got). `writeScenarioReport` is added in Task 8 — until then, add a temporary stub at the end of `scenario_harness_test.go`: `func writeScenarioReport(_ []scenario, _ []checkResult) error { return nil }` and note it is replaced in Task 8.

- [ ] **Step 5: Commit**

```bash
git add core/block/editor/chatobject/scenario_harness_test.go core/block/editor/chatobject/scenario_catalog_test.go
git commit -m "GO-7290 Add runScenario, catalog entrypoint, determinism gate"
```

---

### Task 7: Generate the scenario catalog via a dispatched agent

**Files:**
- Modify: `core/block/editor/chatobject/scenario_catalog_test.go`

- [ ] **Step 1: Dispatch the catalog agent**

Use the Agent tool (`general-purpose`) with this exact prompt:

```
READ-ONLY for production code. Output: Go source for additional entries of `var scenarioCatalog = []scenario{...}` in package chatobject, appended to core/block/editor/chatobject/scenario_catalog_test.go. Do not modify any other file.

Context: read docs/superpowers/specs/2026-05-16-chat-read-counter-multidevice-scenarios-design.md (§5 lists 15 categories) and core/block/editor/chatobject/scenario_harness_test.go for the exact `scenario`/`scenarioChange`/`step`/`checkpointExpect` types and `kind*`/`step*` constants. The local reading account id is the constant `testCreator`; messages authored by it are excluded from its own unread count.

Produce >=1 concrete scenario per category 1..15 (categories in spec §5), each: a valid DAG (first entry is the root system change with no Prev; non-root entries reference existing ids; model concurrency as siblings sharing a Prev and merges as a change with 2+ Prev), ordered Steps mixing stepAddChanges/stepReadUpTo/stepMarkUnread/stepSync/stepRestart/stepCheckpoint, and for every checkpoint an explicit WantUnread authored from the STRICT DAG-ANCESTOR model (current contract: reading head H clears exactly ancestors-of-H; concurrent changes stay unread; own-author messages never count for that account). Cover all three counters (Message/Mention/Reaction). Set Intent to one sentence describing the collaboration situation and the expected semantic. Name scenarios S-<category-short>-<variant>.

Return ONLY the Go slice-literal entries (no package/imports), ready to paste into the existing `scenarioCatalog`. Ensure ids are short and stable so OrderId is deterministic.
```

- [ ] **Step 2: Integrate the returned entries**

Paste the agent's entries into the `scenarioCatalog` slice in `scenario_catalog_test.go` (after the two Task 6 entries). Do not hand-edit expected numbers — they are the intended (DAG) contract.

- [ ] **Step 3: Compile + vet**

Run: `go vet ./core/block/editor/chatobject/ && go build ./core/block/editor/chatobject/`
Expected: success. Fix only compilation issues (typos, undefined ids in a DAG). If a DAG is structurally invalid (non-root without Prev, dangling Prev), correct the DAG structure, not the expected counts.

- [ ] **Step 4: Run the suite (determinism only, characterization)**

Run: `go test ./core/block/editor/chatobject/ -run TestChatReadCounterScenarios -count=1`
Expected: PASS (determinism + report write; Want!=Got does not fail). If a scenario errors in the harness (panic/build-tree failure), fix that scenario's DAG.

- [ ] **Step 5: Commit**

```bash
git add core/block/editor/chatobject/scenario_catalog_test.go
git commit -m "GO-7290 Add generated multi-device read-counter scenario catalog"
```

---

### Task 8: Report generation + run on current system + divergence summary

**Files:**
- Modify: `core/block/editor/chatobject/scenario_harness_test.go` (replace the Task 6 `writeScenarioReport` stub)
- Create: `core/block/editor/chatobject/testdata/scenario_report.md` (generated)

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./core/block/editor/chatobject/ -run TestScenarioReport_Generated -count=1`
Expected: FAIL — stub `writeScenarioReport` writes nothing / file absent.

- [ ] **Step 3: Replace the stub with the real report writer**

Replace `func writeScenarioReport(_ []scenario, _ []checkResult) error { return nil }` with:

```go
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
```

Add imports `"fmt"`, `"strings"`, `"os"` (if not already present from earlier tasks).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./core/block/editor/chatobject/ -run 'TestScenarioReport_Generated|TestChatReadCounterScenarios' -count=1`
Expected: PASS; `core/block/editor/chatobject/testdata/scenario_report.md` exists with per-scenario MATCH/DIVERGENCE lines.

- [ ] **Step 5: Run the full suite on the current system & summarize**

Run: `go test ./core/block/editor/chatobject/ -run 'TestScenario|TestBuildScenarioTree|TestDiffEngine|TestChatReadCounterScenarios|TestScenarioReport' -count=1 -race`
Expected: PASS (characterization — no failures on intent≠actual). Then read `testdata/scenario_report.md`; in the commit message and a short note, list every DIVERGENCE (scenario, want vs got) — these are the inputs to the OrderId-watermark redesign decision and the GO-7290 disposition.

- [ ] **Step 6: Commit**

```bash
git add core/block/editor/chatobject/scenario_harness_test.go core/block/editor/chatobject/testdata/scenario_report.md
git commit -m "GO-7290 Generate scenario report; capture current-system divergences"
```

---

## Self-Review

**Spec coverage:**
- §1 differential intent → Task 6 (`Intent` field, characterization mode), Task 8 (report intended vs actual, DIVERGENCE flagging). ✓
- §2 scope / simulated boundary → harness drives real DiffManager+repo+markRead, simulates seenHeads union (Tasks 3,5). ✓
- §3 hybrid level → Tasks 2–4. ✓
- §4 A1 table-driven + schema + grounding → Tasks 1,2,6. ✓
- §4.3 two worked scenarios → Task 6 catalog entries. ✓
- §5 15 categories → Task 7 agent dispatch with exact prompt. ✓
- §6 execution/output, characterization vs lock, determinism, report → Task 6 (`runCatalog` determinism, no-fail), Task 8 (report). Lock mode is opt-in/future — explicitly out of this plan's tasks; noted here as a deliberate deferral (YAGNI until goldens blessed). ✓ (gap acknowledged, not silent)
- §7 reuse for redesign → engine-agnostic `scenario`/`runScenario` boundary (Task 6). ✓
- §8 risks → Task 4 reuses existing fixture + Task 2 reuses `store_apply_test.go` pattern; determinism asserted (Task 6). ✓

**Placeholder scan:** No TBD/TODO. Two "verify exact signature" notes (Task 4 `AddMessage`/`NewMessage`, Task 2 acl imports) are explicit narrow lookups with the source location and the exact shape required, plus a fallback — not open-ended placeholders. The Task 6 `writeScenarioReport` stub is explicitly introduced and explicitly replaced in Task 8 (not a dangling placeholder).

**Type consistency:** `scenario`, `scenarioChange{ID,Prev,Author,Kind,ReactTo,Mention}`, `step{Kind,Device,ChangeIDs,UpToMsgID,AfterOrder,SyncWith,Counter,Expect}`, `checkpointExpect{Device,Counter,WantUnread,WantReadIDs}`, `changeKind`/`kind*`, `stepKind`/`step*`, `buildScenarioTree→*scenarioTree{tree,orderOf}`, `newDiffEngine/diffEngine.{rebuild,applyDeviceValues,restart}`, `newScenarioFixture→*scenarioFixture`, `newDeviceModel→*deviceModel.{readUpTo,deviceValues}`, `sf.syncDevices/checkpoint/unread/readUpTo`, `runScenario→[]checkResult`, `runCatalog`, `writeScenarioReport(catalog,results)` — names/signatures consistent across Tasks 1–8.

**Out of scope (unchanged):** the redesign; any-sync changes; transport sync simulator; lock-mode regression gate (deferred until divergences reviewed & goldens blessed).
