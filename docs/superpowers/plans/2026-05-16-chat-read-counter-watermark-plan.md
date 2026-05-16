# Chat Read-Counter Watermark — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Replace the per-counter `*objecttree.DiffManager` with a tiny `(OrderId,AddSeq)`-dominance watermark engine (no change graph, no any-sync change), keeping the chat-facing contract, and use the committed scenario catalog as the **semantic sign-off artifact** (non-crux MATCH; the two crux scenarios DIVERGE = the recorded accepted decision).

**Architecture:** `read(X) ⟺ ∃ H∈seenHeads: OrderId(X)≤OrderId(H) ∧ AddSeq(X)≤AddSeq(H)`. `seenHeads` stays the synced anchor (`KeyValueService`, unchanged). `OrderId/AddSeq` resolved locally via `Tree().GetChange`. The store iterates the tree's changes once (id/OrderId/AddSeq only — no payload, no graph, no `BuildHistoryTree`) and fires the existing `onRemove(domintedIds)` → existing `markReadMessages` → repo flags (idempotent). `addToDiffManagers`/`updateInDiffManagers` become no-ops (a fresh change has the newest AddSeq ⇒ never dominated ⇒ unread by construction).

**Tech Stack:** Go; `any-sync/.../objecttree` (`ObjectTree.GetChange`, `Storage().GetAfterOrder`, `Change.{OrderId,AddSeq}`); testify. No new package, no anystoreprovider, no any-sync modification.

---

## Background (verified — rely on these)

- `objecttree.ObjectTree.GetChange(id) (*objecttree.Change, error)` (`objecttree.go:83,226`); `Change` has `OrderId string` (`change.go:35`), `AddSeq uint64` (`change.go:36`). `Storage().GetAfterOrder(ctx,"",iter)` yields every `StorageChange{Id,PrevIds,OrderId,AddSeq}` ascending (`storage.go:233-257`). OrderId = deterministic topological order, content-hash sibling tiebreak (`tree.go:294-308`); AddSeq monotonic, never reassigned (`storage.go:44-49`). Verified facts.
- Chat message id == tree change id; message OrderId == tree order (established by the scenario suite harness; `scenario_harness_test.go:328` `addMessage`).
- `store.go` consumer surface (current, develop): `diffManagers map[string]*diffManager` (`:44`); `diffManager{ diffManager *objecttree.DiffManager; onRemove func([]string) }` (`:82-85`); `ProvideStat` → `.GetIds()`/`.SeenHeads()` (`:61,64`); `RegisterDiffManager(name, onRemoveHook)` (`:103`); `initDiffManagers` (`:111`, reads synced `KeyValueService().Get(seenHeadsKey)` per device value); `InitDiffManager(ctx,name,seenHeads)` (`:144`, builds `objecttree.NewDiffManager`+`Init()`+`SubscribeForKey`); `addToDiffManagers(change)` (`:311`); `updateInDiffManagers(tree)` (`:340`); `MarkSeenHeads(ctx,name,heads)` (`:348`, `Remove`+`StoreSeenHeads`); `StoreSeenHeads`/`seenHeadsKey` (`:357,372`); `s.treeSource.Tree()` available on `store`.
- `chatobject` registers `onRemove` = `markReadMessages(ids, counter)` (`chatobject.go:230/236/242`) → `chatrepository.SetReadFlag` (idempotent; returns only ids it actually flipped). Untouched by this plan.
- Scenario suite (committed, kept): `core/block/editor/chatobject/scenario_harness_test.go` (`buildScenarioTree` builds a real tree with AddSeq enabled — `:159` `SetAddSeq`; `scenarioTree.tree`), `scenario_catalog_test.go` (`scenarioCatalog`, incl. `S-concurrent-merge-divergence` and `S-cat12-dag-ancestor-vs-orderid-prefix` — the two crux scenarios), `testdata/scenario_report.md` (committed baseline; intended=DAG, actual=current; already has 1 recorded DIVERGENCE at `S-cat17` step 4). The harness currently hardcodes the real-DiffManager engine; this plan adds a minimal swap seam.

## File Structure

- Create `core/block/source/sourceimpl/readwatermark.go` — `readPair`, `dominated`, `watermark` engine.
- Create `core/block/source/sourceimpl/readwatermark_test.go` — unit tests (predicate + engine).
- Modify `core/block/source/sourceimpl/store.go` — swap the per-counter engine; keep the contract.
- Modify `core/block/editor/chatobject/scenario_harness_test.go` — minimal `readCounterEngine` seam + `watermarkEngine` adapter.
- Create `core/block/editor/chatobject/scenario_signoff_test.go` — the sign-off gate.
- Modify `core/block/editor/chatobject/testdata/scenario_report.md` — regenerated (the sign-off record).

---

### Task 1: dominance predicate

**Files:** Create `readwatermark.go`; Test `readwatermark_test.go`

- [ ] **Step 1: Failing test**

```go
package sourceimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDominated(t *testing.T) {
	// OrderIds width-consistent (real lexids sort lexicographically ==
	// topologically; "o9" > "o10" would break naive labels).
	f := []readPair{{"o05", 5}}
	assert.True(t, dominated(readPair{"o03", 3}, f))
	assert.False(t, dominated(readPair{"o07", 4}, f))

	// late in-past insert: orderId old but AddSeq newer than frontier → NOT read
	assert.False(t, dominated(readPair{"o02", 9}, f), "in-past insert (newer AddSeq) must be unread")

	// multi-head: per-head dominance, NOT per-axis max
	multi := []readPair{{"o10", 5}, {"o04", 20}}
	assert.False(t, dominated(readPair{"o07", 8}, multi),
		"X not dominated by any single head (per-axis max would wrongly say read)")
	assert.True(t, dominated(readPair{"o03", 4}, multi)) // dominated by H2 (o04,a20)
	assert.True(t, dominated(readPair{"o09", 5}, multi)) // dominated by H1 (o10,a5)
}
```

- [ ] **Step 2: Run — expect FAIL** `go test ./core/block/source/sourceimpl/ -run TestDominated -v` → `undefined: readPair/dominated`.

- [ ] **Step 3: Implement** `readwatermark.go`

```go
package sourceimpl

// readPair is a change's local read coordinates. OrderId is a lexid string
// (lexicographic order == topological order); AddSeq is the monotonic insert
// sequence.
type readPair struct {
	OrderId string
	AddSeq  uint64
}

// dominated reports whether X is "read": some seen head H was at-or-after X in
// the timeline AND already existed (AddSeq) when the user read H. Per-head
// (∃H), never per-axis max — see the multi-head test.
func dominated(x readPair, frontier []readPair) bool {
	for _, h := range frontier {
		if x.OrderId <= h.OrderId && x.AddSeq <= h.AddSeq {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run — expect PASS.**
- [ ] **Step 5: Commit** `git add core/block/source/sourceimpl/readwatermark.go core/block/source/sourceimpl/readwatermark_test.go && git commit -m "GO-7290 read-counter: (OrderId,AddSeq) dominance predicate"`.

---

### Task 2: `watermark` engine

**Files:** Modify `readwatermark.go`; Test `readwatermark_test.go`

The engine is pure/injectable: it knows nothing of tree/repo — callers supply `resolve` (seenId → pair) and a `changes` iterator (candidate id → pair).

- [ ] **Step 1: Failing test**

```go
func TestWatermark_AdvanceFiresDominatedAndDefersPending(t *testing.T) {
	all := map[string]readPair{
		"G": {"o1", 1}, "m1": {"o2", 2}, "x1": {"o3", 3}, "M": {"o4", 4},
		"late": {"o2", 9}, // in-past orderId, newer AddSeq
	}
	resolve := func(id string) (readPair, bool) { p, ok := all[id]; return p, ok }
	eachChange := func(yield func(string, readPair)) {
		for id, p := range all {
			yield(id, p)
		}
	}
	var got []string
	w := newWatermark(func(ids []string) { got = append(got, ids...) })

	// seen up to M(o4,a4): G,m1,x1,M dominated; "late" NOT (a9>a4)
	w.advance([]string{"M", "absent"}, resolve, eachChange)
	assert.ElementsMatch(t, []string{"G", "m1", "x1", "M"}, got)
	assert.Contains(t, w.pending, "absent") // unresolved seen id deferred

	// "absent" arrives later (resolvable) → re-resolved, no panic
	got = nil
	all["absent"] = readPair{"o5", 5}
	w.advance(nil, resolve, eachChange) // re-resolve pending only
	assert.NotContains(t, w.pending, "absent")
}
```

- [ ] **Step 2: Run — expect FAIL** → `undefined: newWatermark`.

- [ ] **Step 3: Implement** (append to `readwatermark.go`)

```go
// watermark is the per-counter read engine. It holds the resolved seen
// frontier and fires onRemove with every currently-dominated change id
// (downstream SetReadFlag is idempotent, so re-firing already-read ids is
// harmless — correctness does not depend on computing a minimal delta).
type watermark struct {
	onRemove func([]string)
	frontier []readPair
	pending  map[string]struct{} // seen ids not yet resolvable
}

func newWatermark(onRemove func([]string)) *watermark {
	return &watermark{onRemove: onRemove, pending: map[string]struct{}{}}
}

// advance extends the frontier with newly seen ids (deferring unresolved ones)
// then fires onRemove for all dominated changes.
func (w *watermark) advance(seenIds []string, resolve func(string) (readPair, bool), eachChange func(yield func(string, readPair))) {
	for id := range w.pending {
		seenIds = append(seenIds, id)
	}
	for _, id := range seenIds {
		if p, ok := resolve(id); ok {
			w.frontier = append(w.frontier, p)
			delete(w.pending, id)
		} else {
			w.pending[id] = struct{}{}
		}
	}
	if len(w.frontier) == 0 {
		return
	}
	var read []string
	eachChange(func(id string, p readPair) {
		if dominated(p, w.frontier) {
			read = append(read, id)
		}
	})
	if len(read) > 0 {
		w.onRemove(read)
	}
}

// seenIds returns the frontier's source ids (debug / ProvideStat parity).
func (w *watermark) frontierLen() int { return len(w.frontier) }
```

- [ ] **Step 4: Run — expect PASS.**
- [ ] **Step 5: Commit** `git commit -m "GO-7290 read-counter: watermark engine (advance, pending deferral)"`.

---

### Task 3: Swap `store.go` to the watermark engine (keep the chat contract)

**Files:** Modify `core/block/source/sourceimpl/store.go`

- [ ] **Step 1:** Replace the `diffManager` struct (`:82-85`):

```go
type diffManager struct {
	wm       *watermark
	onRemove func(removed []string)
}
```

- [ ] **Step 2:** `RegisterDiffManager` (`:103-109`) — store the engine:

```go
func (s *store) RegisterDiffManager(name string, onRemoveHook func(removed []string)) {
	if _, ok := s.diffManagers[name]; !ok {
		s.diffManagers[name] = &diffManager{onRemove: onRemoveHook, wm: newWatermark(onRemoveHook)}
	}
}
```

- [ ] **Step 3:** Add resolver + change iterator helpers (append near `seenHeadsKey`):

```go
func (s *store) resolvePair(id string) (readPair, bool) {
	ch, err := s.treeSource.Tree().GetChange(id)
	if err != nil || ch == nil {
		return readPair{}, false
	}
	return readPair{OrderId: ch.OrderId, AddSeq: ch.AddSeq}, true
}

// eachChange streams every change's (id, pair) from tree storage — id/OrderId/
// AddSeq only, no payload decode, no graph, no BuildHistoryTree.
func (s *store) eachChange(ctx context.Context) func(yield func(string, readPair)) {
	return func(yield func(string, readPair)) {
		_ = s.treeSource.Tree().Storage().GetAfterOrder(ctx, "", func(_ context.Context, ch objecttree.StorageChange) (bool, error) {
			yield(ch.Id, readPair{OrderId: ch.OrderId, AddSeq: ch.AddSeq})
			return true, nil
		})
	}
}
```

- [ ] **Step 4:** Rewrite `InitDiffManager` (`:144-187`) — **replace the whole body** (the `objecttree.NewDiffManager`/`Init` block and the `buildTree`); keep the synced `SubscribeForKey` exactly, just point its callback at `wm.advance`:

```go
func (s *store) InitDiffManager(ctx context.Context, name string, seenHeads []string) (err error) {
	manager, ok := s.diffManagers[name]
	if !ok {
		return nil
	}
	manager.wm.advance(seenHeads, s.resolvePair, s.eachChange(ctx))

	err = s.getTechSpace().KeyValueService().SubscribeForKey(s.seenHeadsKey(name), name, func(key string, val keyvalueservice.Value) {
		s.ObjectTree.Lock()
		defer s.ObjectTree.Unlock()
		newSeenHeads, uerr := unmarshalSeenHeads(val.Data)
		if uerr != nil {
			log.Errorf("subscribe for seenHeads: %s: %v", name, uerr)
			return
		}
		manager.wm.advance(newSeenHeads, s.resolvePair, s.eachChange(context.Background()))
	})
	if err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	return nil
}
```

- [ ] **Step 5:** `initDiffManagers` (`:111-133`) — the per-device-value loop now feeds `advance` (union via repeated advance, frontier accumulates):

```go
func (s *store) initDiffManagers(ctx context.Context) error {
	for name, manager := range s.diffManagers {
		if err := s.InitDiffManager(ctx, name, nil); err != nil {
			return fmt.Errorf("init diff manager: %w", err)
		}
		vals, err := s.getTechSpace().KeyValueService().Get(ctx, s.seenHeadsKey(name))
		if err != nil {
			log.With("error", err).Error("init diff manager: get value")
			continue
		}
		for _, val := range vals {
			seenHeads, uerr := unmarshalSeenHeads(val.Data)
			if uerr != nil {
				log.With("error", uerr).Error("init diff manager: unmarshal seen heads")
				continue
			}
			manager.wm.advance(seenHeads, s.resolvePair, s.eachChange(ctx))
		}
	}
	return nil
}
```

- [ ] **Step 6:** `MarkSeenHeads` (`:348-355`) → `manager.wm.advance(heads, s.resolvePair, s.eachChange(ctx))` then `s.StoreSeenHeads(ctx, name)`. `StoreSeenHeads` (`:357-370`): replace `manager.diffManager.SeenHeads()` — persist the input heads. Simplest correct: have `MarkSeenHeads` marshal+`KeyValueService().Set(seenHeadsKey, heads)` directly (the synced value is the seen-head ids, exactly as today). Keep `seenHeadsKey` unchanged.

- [ ] **Step 7:** `addToDiffManagers` (`:311-316`) and `updateInDiffManagers` (`:340-345`) → **no-ops** with a comment:

```go
// addToDiffManagers: no-op. A freshly pushed/synced change has the newest
// AddSeq, so it can never be dominated by an existing seen frontier ⇒ it is
// unread by construction. No graph to maintain.
func (s *store) addToDiffManagers(change *objecttree.Change) {}

func (s *store) updateInDiffManagers(tree objecttree.ObjectTree) {}
```

- [ ] **Step 8:** `ProvideStat` (`:58-72`) — replace `.GetIds()`/`.SeenHeads()` with `manager.wm.frontierLen()` (and drop the now-unused stat fields or set them empty). Keep it compiling; this is debug only.

- [ ] **Step 9: Build + regression**

`go build ./core/block/source/sourceimpl/...` → success (remove now-unused imports/symbols: `objecttreebuilder` if unused, `unmarshalSeenHeads` kept).
`go test ./core/block/source/sourceimpl/... ./core/block/editor/chatobject/... 2>&1 | grep -E "FAIL|ok "` → all `ok`. (`chatobject` tests mock the source — unaffected. `sourceimpl` tests construct `store` and exercise store-doc paths; the diff-manager paths now use the watermark engine over the real test tree.) If a `sourceimpl` test asserted DiffManager-specific behavior, adjust it to the watermark contract (it is a behavior change by design — see the design spec §5).

- [ ] **Step 10: Commit** `git commit -m "GO-7290 store: replace DiffManager with (OrderId,AddSeq) watermark engine"`.

---

### Task 4: `MarkMessagesAsUnread` compatibility

**Files:** Modify `core/block/source/sourceimpl/store.go` (only if a hook exists); Test `readwatermark_test.go`

`MarkMessagesAsUnread(afterOrderId)` is handled chat-side as an OrderId-range repo op (`reading.go`); the watermark engine must not re-mark those read on the next `advance`. Because the frontier only ever advances and the repo flag is the durable truth, an explicit unread of `orderId > afterOrderId` stays unread **only if** the frontier does not still dominate them. After an explicit unread the chat re-derives seenHeads via `collectSeenHeads(afterOrderId)` and calls `InitDiffManager` with the *reduced* seen set (`reading.go:90-94`) — so the frontier is rebuilt from the reduced seenHeads, not the old one.

- [ ] **Step 1: Test** the rebuild-from-reduced-seen path:

```go
func TestWatermark_RebuildFromReducedSeenShrinksRead(t *testing.T) {
	all := map[string]readPair{"a": {"o1", 1}, "b": {"o2", 2}, "c": {"o3", 3}}
	resolve := func(id string) (readPair, bool) { p, ok := all[id]; return p, ok }
	each := func(y func(string, readPair)) {
		for id, p := range all {
			y(id, p)
		}
	}
	var got []string
	w := newWatermark(func(ids []string) { got = append(got, ids...) })
	w.advance([]string{"c"}, resolve, each) // read all
	assert.ElementsMatch(t, []string{"a", "b", "c"}, got)

	// explicit unread → chat rebuilds with reduced seen {a}: a fresh engine
	got = nil
	w2 := newWatermark(func(ids []string) { got = append(got, ids...) })
	w2.advance([]string{"a"}, resolve, each)
	assert.ElementsMatch(t, []string{"a"}, got, "only a is read after reduced-seen rebuild")
}
```

- [ ] **Step 2:** Confirm `InitDiffManager` constructs a **fresh** `watermark` on each call (so a reduced seen set yields a smaller frontier). Adjust `RegisterDiffManager`/`InitDiffManager`: `InitDiffManager` should reset `manager.wm = newWatermark(manager.onRemove)` before `advance` (mirrors the old `NewDiffManager` rebuild). Add that line at the top of `InitDiffManager` (after the `manager, ok` check).

- [ ] **Step 3: Run — expect PASS**; rerun Task 3 Step 9 regression → still `ok`.
- [ ] **Step 4: Commit** `git commit -m "GO-7290 store: fresh watermark per InitDiffManager (markUnread/reduced-seen correctness)"`.

---

### Task 5: Harness engine seam + watermark adapter

**Files:** Modify `core/block/editor/chatobject/scenario_harness_test.go`

- [ ] **Step 1:** Add the minimal seam (above `type diffEngine struct`):

```go
type readCounterEngine interface {
	rebuild(seen []string)
	applyDeviceValues(deviceValues [][]string)
	restart(deviceValues [][]string)
}

var engineFactory = func(t testing.TB, tree objecttree.ObjectTree, onRemove func([]string)) readCounterEngine {
	return newDiffEngine(t, tree, onRemove) // default: legacy DAG baseline
}
```

Change `scenarioFixture.engine` to `map[chatmodel.CounterType]readCounterEngine`; in `newScenarioFixture` construct via `engineFactory(t, st.tree, func(ids []string){ require.NoError(t, sf.fx.markReadMessages(ids, cc)) })`. `*diffEngine` already satisfies the interface — pure refactor.

- [ ] **Step 2: Refactor regression** `go test ./core/block/editor/chatobject/ -run 'TestChatReadCounterScenarios|TestScenarioReport_Generated' 2>&1 | tail -3` → PASS; `git diff --stat core/block/editor/chatobject/testdata/scenario_report.md` → **no change** (default factory still the DAG engine).

- [ ] **Step 3:** Add the watermark adapter (drives the production predicate against the scenario tree + repo):

```go
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
	ch, err := e.tree.GetChange(id)
	if err != nil || ch == nil {
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
	for _, v := range dv {
		e.wm.Advance(v, e.resolve, e.each)
	}
}
func (e *watermarkEngine) restart(dv [][]string) { e.rebuild(nil); e.applyDeviceValues(dv) }
```

Add tiny exported aliases in `sourceimpl` (new file `core/block/source/sourceimpl/readwatermark_export_test_shim.go`): `type Watermark = watermark`; `type ReadPair = readPair`; `func NewWatermark(onRemove func([]string)) *Watermark { return newWatermark(onRemove) }`; `func (w *Watermark) Advance(seen []string, resolve func(string)(ReadPair,bool), each func(func(string,ReadPair))) { w.advance(seen, resolve, each) }`. Import `"github.com/anyproto/anytype-heart/core/block/source/sourceimpl"` in the harness.

- [ ] **Step 4: Build** `go test ./core/block/editor/chatobject/ -run TestChatReadCounterScenarios -count=1 2>&1 | tail -2` (compiles; default factory unchanged → still PASS, report unchanged).
- [ ] **Step 5: Commit** `git commit -m "GO-7290 harness: engine seam + watermark adapter"`.

---

### Task 6: Sign-off gate (the accepted-semantic record)

**Files:** Create `core/block/editor/chatobject/scenario_signoff_test.go`; regenerate `testdata/scenario_report.md`

- [ ] **Step 1: Test** — run the full catalog with the watermark engine; assert **non-crux** scenarios reproduce the DAG baseline `Got`, and the **two crux** scenarios DIVERGE (the accepted semantic). The crux set is exactly the design's two: `S-concurrent-merge-divergence`, `S-cat12-dag-ancestor-vs-orderid-prefix`. (The pre-existing baseline DIVERGENCE at `S-cat17` is compared `Got==baseline Got`, unaffected.)

```go
package chatobject

import (
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/stretchr/testify/require"
)

var watermarkAcceptedDivergence = map[string]bool{
	"S-concurrent-merge-divergence":          true,
	"S-cat12-dag-ancestor-vs-orderid-prefix": true,
}

func TestWatermark_SignOffGate(t *testing.T) {
	baseline := map[string][]checkResult{}
	for _, sc := range scenarioCatalog {
		baseline[sc.Name] = runScenario(t, sc)
	}
	prev := engineFactory
	engineFactory = func(t testing.TB, tree objecttree.ObjectTree, onRemove func([]string)) readCounterEngine {
		return newWatermarkEngine(t, tree, onRemove)
	}
	defer func() { engineFactory = prev }()

	for _, sc := range scenarioCatalog {
		w := runScenario(t, sc)
		base := baseline[sc.Name]
		require.Equal(t, len(base), len(w), "scenario %s checkpoint count", sc.Name)
		diverged := false
		for i := range base {
			if base[i].Got != w[i].Got {
				diverged = true
			}
		}
		if watermarkAcceptedDivergence[sc.Name] {
			require.Truef(t, diverged,
				"crux scenario %s MUST diverge from DAG (accepted OrderId+AddSeq semantic); if it now matches, the model or scenario changed — investigate", sc.Name)
			continue
		}
		for i := range base {
			require.Equalf(t, base[i].Got, w[i].Got,
				"NON-crux scenario %s step %d %s/%s: watermark got=%d baseline got=%d — this is a real bug, not an accepted divergence",
				sc.Name, base[i].StepIdx, base[i].Device, base[i].Counter.DiffManagerName(), w[i].Got, base[i].Got)
		}
	}
}
```

- [ ] **Step 2: Run — iterate until green.** `go test ./core/block/editor/chatobject/ -run TestWatermark_SignOffGate -v 2>&1 | tail -20`. Non-crux mismatches are real watermark bugs → fix the engine/predicate (Task 1-2), never the catalog or the assert. Expected end state: every non-crux scenario MATCHes the DAG baseline; the two crux scenarios DIVERGE.

- [ ] **Step 3: Regenerate the sign-off report.** Add a generator (or extend `TestScenarioReport_Generated`) that writes a second section `## Watermark engine (proposed)` with per-scenario MATCH/DIVERGENCE vs the DAG baseline, into `testdata/scenario_report.md`. Run it; `git add core/block/editor/chatobject/testdata/scenario_report.md`. The committed report now explicitly records: which scenarios change and how — **this file is the owner sign-off artifact** referenced by the design spec §5.

- [ ] **Step 4: Commit** `git commit -m "GO-7290 sign-off gate: watermark == DAG except the 2 accepted crux scenarios; report regenerated"`.

---

### Task 7: Final verification + migration note

**Files:** none (verification) + a note in the design spec if needed

- [ ] **Step 1:** Full regression: `go test ./core/block/source/sourceimpl/... ./core/block/editor/chatobject/... ./core/block/chats/... 2>&1 | grep -E "FAIL|ok "` → all `ok`.
- [ ] **Step 2:** `go vet ./core/block/source/sourceimpl/... 2>&1 | grep -vE "duplicate libraries|unkeyed fields" | tail -2` → no new findings in `readwatermark*`/`store.go`; `go build ./... 2>&1 | grep -iE "error|cannot|undefined" | head` → builds.
- [ ] **Step 3:** Migration check (no code): first run after deploy has no special state — `seenHeads` is the existing synced value; the engine recomputes read flags from `(seenHeads, local tree)` on `InitDiffManager`; the per-message SQLite flag is the durable truth and `SetReadFlag` is idempotent, so existing flags converge with zero data migration. Confirm `reading.go`'s `collectSeenHeads`/`MarkReadMessages`/`InitDiffManager` call sites still compile against the unchanged signatures (they do — only the engine behind `InitDiffManager`/`MarkSeenHeads` changed, not their interfaces). Record this paragraph in the design spec §4 if not already covered.
- [ ] **Step 4: Commit** (if any doc note added) `git commit -m "GO-7290 read-counter watermark: final verification; migration is no-op (flags are truth)"`.

---

## Self-Review

**Spec coverage** (`2026-05-16-chat-read-counter-design.md`):
- §2 predicate `∃H: OrderId(X)≤OrderId(H) ∧ AddSeq(X)≤AddSeq(H)` → Task 1 (`dominated`), incl. multi-head per-head (not per-axis) and late-in-past-unread cases.
- §2 local resolution via `Tree().GetChange` → Task 3 (`resolvePair`); no graph/`BuildHistoryTree` → Task 3 (`eachChange` = id/order/addseq only).
- §4.1 monotone/F3-immune → Task 4 (fresh engine per Init from current seenHeads; sticky idempotent `SetReadFlag` downstream, unchanged). §4.2 pending-seen → Task 2 (`pending` defer/resolve). §4.3 `MarkMessagesAsUnread` → Task 4 (reduced-seen rebuild). §4.4 counters → unchanged (three independent engines, chat side untouched).
- §5 residual semantic + sign-off → Task 6 (non-crux MATCH; the two named crux scenarios DIVERGE; report regenerated as the record).
- §6 surface: keep `RegisterDiffManager/onRemove/MarkSeenHeads/InitDiffManager`, `addTo/updateIn` no-ops, synced `seenHeads` unchanged → Task 3. §7 no any-sync change, no new package, no persistence → honored (the export shim is test-only).
- **Honest scope note:** Task 3's `eachChange` does one `GetAfterOrder` pass per `InitDiffManager` (id/OrderId/AddSeq only — no payload decode, no tree build, no graph) — already far cheaper than the deleted `BuildHistoryTree`. Bounding it to a `GetAfterAddSeq(lastSeenAddSeq)` tail is a **documented follow-up optimization** (needs device-local cursor persistence), deliberately out of v1 to keep this minimal; correctness does not depend on it (`SetReadFlag` idempotent).

**Placeholder scan:** none — every code step is complete; every command has an expected result; the one test-visibility shim (Task 5) is concrete with rationale.

**Type consistency:** `readPair{OrderId string,AddSeq uint64}`, `dominated(readPair,[]readPair)bool`, `watermark`+`newWatermark(func([]string))`+`advance(seen []string, resolve func(string)(readPair,bool), each func(func(string,readPair)))`+`pending`+`frontierLen()`; exported test aliases `Watermark`/`ReadPair`/`NewWatermark`/`Advance`; harness `readCounterEngine{rebuild,applyDeviceValues,restart}`+`engineFactory`+`watermarkEngine`. Consistent across Tasks 1-7 and matched to verified `store.go`/`objecttree` APIs.
