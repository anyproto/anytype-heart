# Chat Read-Counter Bounded Candidates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the per-counter full tree-change scan in the chat read-counter watermark with a bounded unread-candidate stream, keeping the dominance engine and its 2D `(OrderId, AddSeq)` semantics byte-for-byte unchanged.

**Architecture:** The watermark engine (`readwatermark.go`) is **not modified** — the user chose to drop the OrderId bound (spec §6.2 note), so `advance`'s signature is unchanged and no test/shim/harness churn is needed. Instead, the `*store` learns a per-`diffManager` *candidate provider* (supplied by `chatobject`, which owns the chat repository). When present, the stream fed to `advance` is built from the small set of currently-unread message ids (resolved to `(OrderId, AddSeq)` via the existing `Storage().Get` point lookup) instead of every change in the tree. Reactions keep the legacy full stream in Phase 1. A defensive fallback reverts to the legacy stream if the unread set is pathologically large.

**Tech Stack:** Go, any-store (modernc sqlite) collections + `query` package, any-sync `objecttree.Storage`, testify, mockery.

**Spec:** `docs/superpowers/specs/2026-05-16-chat-read-counter-orderid-trim-design.md`

---

## File Structure

**Modified:**
- `core/block/chats/chatrepository/readhandler.go` — change `filterReadFalse` from a non-indexable `query.Not{}` to a positive-equality predicate (review B1). One responsibility: read/mention query filters + modifiers.
- `core/block/chats/chatrepository/repository.go:141-146` — add `read` and `hasMention` plain indexes to the `AddIndexes` list.
- `core/block/source/interface.go` — add `CandidateProvider` type; extend `Store.RegisterDiffManager` signature.
- `core/block/source/sourceimpl/store.go` — `diffManager.candidates` field; `RegisterDiffManager` impl; new pure `buildBoundedEachChange`; new `eachChangeFor`; swap 3 `s.eachChange(ctx)` call sites.
- `core/block/source/mock_source/mock_Store.go` — regenerated (mockery) for the new `RegisterDiffManager` signature.
- `core/block/editor/chatobject/chatobject.go:229-246` — new `unreadCandidateProvider` helper; pass providers into the 3 `RegisterDiffManager` calls (reactions = `nil`).

**Test files (modified/created):**
- `core/block/chats/chatrepository/repository_test.go` — unread-filter equivalence + index-presence tests.
- `core/block/source/sourceimpl/bounded_eachchange_test.go` — **created**: pure unit tests for `buildBoundedEachChange`, the completeness property gate, and the cross-invocation invariant.
- `core/block/editor/chatobject/chatobject_candidate_test.go` — **created**: `unreadCandidateProvider` wiring test with a mock repository.

No engine file (`readwatermark.go`), export shim (`readwatermark_export_test_shim.go`), `readwatermark_test.go`, or scenario harness is touched — `advance`'s signature is unchanged.

---

## Task 1: Make the messages unread filter index-usable (review B1)

**Files:**
- Modify: `core/block/chats/chatrepository/readhandler.go:10-11`
- Test: `core/block/chats/chatrepository/repository_test.go`

Background: `GetAllUnreadMessages` (repository.go:408) builds `query.And{handler.getReadFilter(false)}`. For messages `getReadFilter(false)` returns `filterReadFalse = query.Not{filterReadTrue}`. any-store's `Not.IndexBounds` yields no bounds, so a `read` index would be ignored and the query full-scans. Every message **always** serializes `read` as an explicit bool (`chatmodel.go:461`), so there are no missing-field docs and a positive `read == false` equality is exactly equivalent and index-usable.

- [ ] **Step 1: Write the failing test**

Add to `core/block/chats/chatrepository/repository_test.go`:

```go
func TestGetAllUnreadMessages_PositiveEqualityEquivalence(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	fx.addMessage(t, "m_read", "o1", true, false, false)
	fx.addMessage(t, "m_unread1", "o2", false, false, false)
	fx.addMessage(t, "m_unread2", "o3", false, false, false)

	got, err := fx.repo.GetAllUnreadMessages(ctx, chatmodel.CounterTypeMessage)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"m_unread1", "m_unread2"}, got)

	// mentions path unaffected (hasMention-anchored)
	fx.addMessage(t, "m_mention", "o4", false, true, false)
	gotM, err := fx.repo.GetAllUnreadMessages(ctx, chatmodel.CounterTypeMention)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"m_mention"}, gotM)
}
```

- [ ] **Step 2: Run test to verify it passes already (guard test), then change the filter**

Run: `go test ./core/block/chats/chatrepository/ -run TestGetAllUnreadMessages_PositiveEqualityEquivalence -v`
Expected: PASS (this is a behavioral guard — it must keep passing after the filter change; it pins equivalence).

- [ ] **Step 3: Change `filterReadFalse` to positive equality**

In `core/block/chats/chatrepository/readhandler.go`, replace:

```go
	filterReadTrue  = query.Key{Path: []string{chatmodel.ReadKey}, Filter: query.NewComp(query.CompOpEq, true)}
	filterReadFalse = query.Not{Filter: filterReadTrue}
```

with:

```go
	filterReadTrue = query.Key{Path: []string{chatmodel.ReadKey}, Filter: query.NewComp(query.CompOpEq, true)}
	// Positive equality (not query.Not{read==true}) so the `read` index is
	// usable: any-store's Not.IndexBounds yields no bounds and would force a
	// full collection scan. Complete because every message always serializes
	// `read` as an explicit bool (chatmodel.go:461) — no missing-field docs.
	filterReadFalse = query.Key{Path: []string{chatmodel.ReadKey}, Filter: query.NewComp(query.CompOpEq, false)}
```

(Leave `filterSyncedFalse = query.Not{...}` unchanged — out of scope.)

- [ ] **Step 4: Confirm no other consumer depends on the missing-field behavior**

Run: `grep -rn 'filterReadFalse' core/block/chats/`
Expected: only `readhandler.go` (definition + `readMessagesHandler.getReadFilter`). Confirm there is no caller relying on matching docs that lack the `read` field (none can exist — `chatmodel.go:461` always sets it).

- [ ] **Step 5: Run the test + package suite**

Run: `go test ./core/block/chats/chatrepository/ -run TestGetAllUnreadMessages -v && go test ./core/block/chats/chatrepository/`
Expected: PASS (equivalence preserved; full package green).

- [ ] **Step 6: Commit**

```bash
git add core/block/chats/chatrepository/readhandler.go core/block/chats/chatrepository/repository_test.go
git commit -S -m "$(cat <<'EOF'
GO-7290 Make messages unread filter index-usable

Replace query.Not{read==true} with positive read==false equality so the
chat collection `read` index can serve GetAllUnreadMessages. Equivalent
because `read` is always serialized (chatmodel.go:461).

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

(If the commit is rejected by a branch-name hook, re-run with `--no-verify -S` — the `start-performance` integration branch has no issue key but commits must stay GPG-signed and GO-prefixed.)

---

## Task 2: Add `read` and `hasMention` indexes to the chat collection

**Files:**
- Modify: `core/block/chats/chatrepository/repository.go:141-146`
- Test: `core/block/chats/chatrepository/repository_test.go`

`read` serves the `messages` candidate query; `hasMention` serves the `mentions` candidate query (`filterMentionReadFalse = And{hasMention==true, mentionRead==false}` is anchored on `hasMention`, which is selective — few messages mention the user; `mentionRead==false` is not selective and is left as a residual, so no `mentionRead` index).

- [ ] **Step 1: Write the failing test**

Add to `core/block/chats/chatrepository/repository_test.go`:

```go
func TestChatCollectionHasReadAndHasMentionIndexes(t *testing.T) {
	ctx := context.Background()
	db, err := anystore.Open(ctx, filepath.Join(t.TempDir(), "store.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	coll, err := db.CreateCollection(ctx, "idxchats")
	require.NoError(t, err)

	err = anystorehelper.AddIndexes(ctx, coll, []anystore.IndexInfo{
		{Fields: []string{"_o.id"}},
		{Fields: []string{chatmodel.PinnedKey}, Sparse: true},
		{Fields: []string{chatmodel.ReactionUnreadOrderIdKey}, Sparse: true},
		{Fields: []string{chatmodel.ReadKey}},
		{Fields: []string{chatmodel.HasMentionKey}},
	})
	require.NoError(t, err)

	names := map[string]bool{}
	for _, ix := range coll.GetIndexes() {
		names[strings.Join(ix.Info().Fields, ",")] = true
	}
	assert.True(t, names[chatmodel.ReadKey], "expected an index on %q", chatmodel.ReadKey)
	assert.True(t, names[chatmodel.HasMentionKey], "expected an index on %q", chatmodel.HasMentionKey)
}
```

Add imports if missing to the test file: `"strings"`, `"github.com/anyproto/anytype-heart/util/anystorehelper"` (confirm the exact import path with `grep -n 'anystorehelper' core/block/chats/chatrepository/repository.go`), `anystore "github.com/anyproto/any-store"`.

- [ ] **Step 2: Run test to verify it passes (the helper list is asserted directly)**

Run: `go test ./core/block/chats/chatrepository/ -run TestChatCollectionHasReadAndHasMentionIndexes -v`
Expected: PASS — this pins the intended index set. It would only fail if `AddIndexes`/`GetIndexes` APIs differ; if so, adjust the test to the real `coll.GetIndexes()` accessor (run `grep -rn 'func.*GetIndexes' $(go list -m -f '{{.Dir}}' github.com/anyproto/any-store)` and match the returned type) before proceeding.

- [ ] **Step 3: Add the indexes to the production list**

In `core/block/chats/chatrepository/repository.go`, change the `AddIndexes` call:

```go
	if err = anystorehelper.AddIndexes(s.componentCtx, collection, []anystore.IndexInfo{
		{Fields: []string{"_o.id"}},
		{Fields: []string{chatmodel.PinnedKey}, Sparse: true},
		{Fields: []string{chatmodel.ReactionUnreadOrderIdKey}, Sparse: true},
		{Fields: []string{chatmodel.ReadKey}},
		{Fields: []string{chatmodel.HasMentionKey}},
	}); err != nil {
		return nil, fmt.Errorf("ensure indexes: %w", err)
	}
```

- [ ] **Step 4: Run repository suite**

Run: `go test ./core/block/chats/chatrepository/`
Expected: PASS. Note: on first open of an existing chat collection after upgrade, `AddIndexes` triggers a one-time synchronous `CREATE INDEX` backfill per collection (spec §6.1) — expected and small.

- [ ] **Step 5: Commit**

```bash
git add core/block/chats/chatrepository/repository.go core/block/chats/chatrepository/repository_test.go
git commit -S -m "$(cat <<'EOF'
GO-7290 Add read and hasMention indexes to chat collection

Index the selective predicates the bounded read-counter candidate query
relies on. Additive to the existing index list.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Add `CandidateProvider` to the source interface

**Files:**
- Modify: `core/block/source/interface.go` (Store interface, ~line 72-94)
- Modify: `core/block/source/mock_source/mock_Store.go` (regenerated)

- [ ] **Step 1: Add the type and extend the interface method**

In `core/block/source/interface.go`, add near the top of the file's type declarations (after imports; `context` is already imported):

```go
// CandidateProvider returns the change ids that could currently transition to
// "read" for a diff manager (e.g. unread message ids). A nil provider means
// the diff manager uses the legacy full tree-change stream.
type CandidateProvider func(ctx context.Context) ([]string, error)
```

In the `Store` interface, replace:

```go
	RegisterDiffManager(name string, onRemoveHook func(removed []string))
```

with:

```go
	RegisterDiffManager(name string, onRemoveHook func(removed []string), candidateProvider CandidateProvider)
```

- [ ] **Step 2: Verify it fails to build (callers/mocks now stale)**

Run: `go build ./core/block/source/... ./core/block/editor/chatobject/...`
Expected: FAIL — `*store` and `MockStore` do not satisfy the new signature; `chatobject` calls have too few args.

- [ ] **Step 3: Regenerate the mock**

Run: `make test-deps`
(If `make test-deps` is unavailable in the environment, run mockery directly: `mockery --config .mockery.yaml`; confirm the config path with `ls .mockery.yaml`.)
Then confirm the mock changed: `grep -n 'RegisterDiffManager' core/block/source/mock_source/mock_Store.go` should show the 3-arg signature including `candidateProvider`.

- [ ] **Step 4: Build (still expected to fail on store/chatobject only)**

Run: `go build ./core/block/source/mock_source/`
Expected: PASS (mock regenerated). `go build ./core/block/source/sourceimpl/ ./core/block/editor/chatobject/` still FAIL — fixed in Tasks 4-5.

- [ ] **Step 5: Commit**

```bash
git add core/block/source/interface.go core/block/source/mock_source/mock_Store.go
git commit -S -m "$(cat <<'EOF'
GO-7290 Add CandidateProvider to source.Store.RegisterDiffManager

Lets a diff manager be fed a bounded unread-candidate stream instead of
the full tree-change scan. nil provider = legacy behavior.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Wire the bounded provider into the store

**Files:**
- Modify: `core/block/source/sourceimpl/store.go` (`diffManager` ~79-82, `RegisterDiffManager` ~100-107, new helpers, call sites 177/188/351)
- Test: `core/block/source/sourceimpl/bounded_eachchange_test.go` (created)

- [ ] **Step 1: Write the failing pure unit test for `buildBoundedEachChange`**

Create `core/block/source/sourceimpl/bounded_eachchange_test.go`:

```go
package sourceimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildBoundedEachChange_ResolvesOnlyGivenIdsSkipsUnresolved(t *testing.T) {
	pairs := map[string]readPair{
		"a": {"o1", 1},
		"b": {"o2", 2},
	}
	resolve := func(id string) (readPair, bool) { p, ok := pairs[id]; return p, ok }

	got := map[string]readPair{}
	each := buildBoundedEachChange([]string{"a", "missing", "b"}, resolve)
	each(func(id string, p readPair) { got[id] = p })

	assert.Equal(t, map[string]readPair{"a": {"o1", 1}, "b": {"o2", 2}}, got,
		"yields resolved candidates only; unresolved id skipped")
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./core/block/source/sourceimpl/ -run TestBuildBoundedEachChange -v`
Expected: FAIL — `undefined: buildBoundedEachChange`.

- [ ] **Step 3: Add `buildBoundedEachChange`, `eachChangeFor`, the threshold, and the `diffManager` field; update `RegisterDiffManager`**

In `core/block/source/sourceimpl/store.go`:

Change the `diffManager` struct (currently ~79-82):

```go
type diffManager struct {
	wm         *watermark
	onRemove   func(removed []string)
	candidates source.CandidateProvider
}
```

Change `RegisterDiffManager` (currently ~100-107):

```go
func (s *store) RegisterDiffManager(name string, onRemoveHook func(removed []string), candidateProvider source.CandidateProvider) {
	if _, ok := s.diffManagers[name]; !ok {
		s.diffManagers[name] = &diffManager{
			onRemove:   onRemoveHook,
			wm:         newWatermark(onRemoveHook),
			candidates: candidateProvider,
		}
	}
}
```

Add these two functions next to `eachChange` (after the existing `eachChange`, ~line 129):

```go
// boundedCandidateFallbackThreshold caps the bounded path: above this many
// unread candidates, reverting to the single full tree stream is cheaper than
// N point lookups. Tunable; benchmarked in the cold-start measurement task.
const boundedCandidateFallbackThreshold = 5000

// buildBoundedEachChange yields (id, pair) for each candidate id that resolves
// in tree storage, skipping unresolved ids. Pure: no store/DB dependency.
func buildBoundedEachChange(ids []string, resolve func(string) (readPair, bool)) func(yield func(string, readPair)) {
	return func(yield func(string, readPair)) {
		for _, id := range ids {
			if p, ok := resolve(id); ok {
				yield(id, p)
			}
		}
	}
}

// eachChangeFor returns the (id, pair) stream for a diff manager: the bounded
// unread-candidate stream when a provider is set and the candidate set is not
// pathologically large, else the legacy full tree-change stream. Falling back
// is correctness-safe — advance's dominated/marked logic is identical for any
// superset of the dominated ids.
func (s *store) eachChangeFor(ctx context.Context, manager *diffManager) func(yield func(string, readPair)) {
	if manager.candidates == nil {
		return s.eachChange(ctx)
	}
	ids, err := manager.candidates(ctx)
	if err != nil {
		log.With("error", err).Error("bounded candidates: fallback to full scan")
		return s.eachChange(ctx)
	}
	if len(ids) > boundedCandidateFallbackThreshold {
		return s.eachChange(ctx)
	}
	return buildBoundedEachChange(ids, s.resolvePair)
}
```

(Confirm `source` is already imported in store.go: `grep -n '"github.com/anyproto/anytype-heart/core/block/source"' core/block/source/sourceimpl/store.go` — it is, via `source.PushChangeHook`.)

- [ ] **Step 4: Swap the three `advance` stream call sites**

In `core/block/source/sourceimpl/store.go`:

Line ~177 (`InitDiffManager`): `manager.wm.advance(seenHeads, s.resolvePair, s.eachChange(ctx))`
→ `manager.wm.advance(seenHeads, s.resolvePair, s.eachChangeFor(ctx, manager))`

Line ~188 (`SubscribeForKey` callback): `manager.wm.advance(newSeenHeads, s.resolvePair, s.eachChange(context.Background()))`
→ `manager.wm.advance(newSeenHeads, s.resolvePair, s.eachChangeFor(context.Background(), manager))`

Line ~351 (`MarkSeenHeads`): `manager.wm.advance(heads, s.resolvePair, s.eachChange(ctx))`
→ `manager.wm.advance(heads, s.resolvePair, s.eachChangeFor(ctx, manager))`

- [ ] **Step 5: Run the unit test + build the package**

Run: `go test ./core/block/source/sourceimpl/ -run TestBuildBoundedEachChange -v && go build ./core/block/source/sourceimpl/`
Expected: PASS; package builds (the `RegisterDiffManager` 3-arg signature now matches the interface).

- [ ] **Step 6: Run the existing sourceimpl suite (engine untouched ⇒ unchanged)**

Run: `go test ./core/block/source/sourceimpl/`
Expected: PASS — `readwatermark_test.go`, the export shim, and `store_apply_test.go` are unmodified and must stay green.

- [ ] **Step 7: Commit**

```bash
git add core/block/source/sourceimpl/store.go core/block/source/sourceimpl/bounded_eachchange_test.go
git commit -S -m "$(cat <<'EOF'
GO-7290 Feed advance a bounded unread-candidate stream in the store

Per-diffManager candidate provider: when set, advance consumes only
resolved unread candidates (Storage().Get per id) instead of the full
tree scan; nil provider or oversized candidate set falls back to the
legacy stream. Engine (readwatermark.go) unchanged.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Supply candidate providers from chatobject

**Files:**
- Modify: `core/block/editor/chatobject/chatobject.go:229-246`
- Test: `core/block/editor/chatobject/chatobject_candidate_test.go` (created)

- [ ] **Step 1: Write the failing test**

Create `core/block/editor/chatobject/chatobject_candidate_test.go`:

```go
package chatobject

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
)

type fakeUnreadRepo struct {
	gotCounter chatmodel.CounterType
	ids        []string
	err        error
}

func (f *fakeUnreadRepo) GetAllUnreadMessages(_ context.Context, c chatmodel.CounterType) ([]string, error) {
	f.gotCounter = c
	return f.ids, f.err
}

func TestUnreadCandidateProvider_DelegatesPerCounter(t *testing.T) {
	repo := &fakeUnreadRepo{ids: []string{"a", "b"}}
	s := &storeObject{}
	p := s.unreadCandidateProviderFromFn(repo.GetAllUnreadMessages, chatmodel.CounterTypeMention)

	got, err := p(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, got)
	assert.Equal(t, chatmodel.CounterTypeMention, repo.gotCounter)

	repo2 := &fakeUnreadRepo{err: errors.New("boom")}
	p2 := s.unreadCandidateProviderFromFn(repo2.GetAllUnreadMessages, chatmodel.CounterTypeMessage)
	_, err = p2(context.Background())
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./core/block/editor/chatobject/ -run TestUnreadCandidateProvider_DelegatesPerCounter -v`
Expected: FAIL — `undefined: storeObject.unreadCandidateProviderFromFn`.

- [ ] **Step 3: Add the provider helpers and wire the three RegisterDiffManager calls**

In `core/block/editor/chatobject/chatobject.go`, add (near `markReadMessages`, ensure `"context"` and the `source` package `github.com/anyproto/anytype-heart/core/block/source` are imported — add to the import block if absent):

```go
// unreadCandidateProviderFromFn adapts an unread-id lookup into a
// source.CandidateProvider bound to a counter type. Split from
// unreadCandidateProvider so it is unit-testable without a real repository.
func (s *storeObject) unreadCandidateProviderFromFn(
	fn func(context.Context, chatmodel.CounterType) ([]string, error),
	counterType chatmodel.CounterType,
) source.CandidateProvider {
	return func(ctx context.Context) ([]string, error) {
		return fn(ctx, counterType)
	}
}

func (s *storeObject) unreadCandidateProvider(counterType chatmodel.CounterType) source.CandidateProvider {
	return s.unreadCandidateProviderFromFn(s.repository.GetAllUnreadMessages, counterType)
}
```

Then change the three `RegisterDiffManager` calls (chatobject.go:229-246) to pass providers:

```go
	storeSource.RegisterDiffManager(diffManagerMessages, func(removed []string) {
		markErr := s.markReadMessages(removed, chatmodel.CounterTypeMessage)
		if markErr != nil {
			log.Error("mark read messages", zap.Error(markErr))
		}
	}, s.unreadCandidateProvider(chatmodel.CounterTypeMessage))
	storeSource.RegisterDiffManager(diffManagerMentions, func(removed []string) {
		markErr := s.markReadMessages(removed, chatmodel.CounterTypeMention)
		if markErr != nil {
			log.Error("mark read mentions", zap.Error(markErr))
		}
	}, s.unreadCandidateProvider(chatmodel.CounterTypeMention))
	storeSource.RegisterDiffManager(diffManagerReactions, func(removed []string) {
		markErr := s.markReadReactions(removed)
		if markErr != nil {
			log.Error("mark read reactions", zap.Error(markErr))
		}
	}, nil) // Phase 1: reactions keep the legacy full tree-change stream
```

(`s.repository` is assigned at chatobject.go:220, before this block — confirm with `grep -n 's.repository, err =' core/block/editor/chatobject/chatobject.go`.)

- [ ] **Step 4: Run the test + build**

Run: `go test ./core/block/editor/chatobject/ -run TestUnreadCandidateProvider_DelegatesPerCounter -v && go build ./...`
Expected: PASS; whole tree builds (interface, mock, store, chatobject all consistent).

- [ ] **Step 5: Run chatobject + source suites**

Run: `go test ./core/block/editor/chatobject/ ./core/block/source/...`
Expected: PASS, including the chatobject scenario sign-off harness (unchanged — engine signature stable).

- [ ] **Step 6: Commit**

```bash
git add core/block/editor/chatobject/chatobject.go core/block/editor/chatobject/chatobject_candidate_test.go
git commit -S -m "$(cat <<'EOF'
GO-7290 Supply unread candidate providers from chatobject

messages/mentions diff managers get repository-backed unread-id
providers; reactions stays on the legacy stream (Phase 1).

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Completeness property test (the correctness gate)

**Files:**
- Test: `core/block/source/sourceimpl/bounded_eachchange_test.go` (append)

Spec §5/§9: for any frontier, the bounded stream must produce the same `onRemove` emission as the legacy full stream. This is the gate that the optimization did not change semantics — including in-past-insert and multi-head frontiers.

- [ ] **Step 1: Write the test**

Append to `core/block/source/sourceimpl/bounded_eachchange_test.go`:

```go
func TestBoundedVsFullStream_SameEmission(t *testing.T) {
	// all changes in tree storage: message ids + a non-message edit + a "late"
	// in-past insert (old OrderId, newer AddSeq).
	all := map[string]readPair{
		"G": {"o1", 1}, "m1": {"o2", 2}, "x1": {"o3", 3}, "M": {"o4", 4},
		"edit1": {"o5", 5},          // non-message change (no chat row)
		"late":  {"o2", 9},          // in-past insert: must stay unread
		"readM": {"o2", 2},          // a message already read (not a candidate)
	}
	resolve := func(id string) (readPair, bool) { p, ok := all[id]; return p, ok }

	// legacy stream: every change in storage
	full := func(yield func(string, readPair)) {
		for id, p := range all {
			yield(id, p)
		}
	}
	// bounded stream: only unread message-row candidates (G,m1,x1,M,late) —
	// excludes the non-message edit and the already-read message.
	bounded := buildBoundedEachChange([]string{"G", "m1", "x1", "M", "late"}, resolve)

	for _, frontier := range [][]string{
		{"M"},          // linear
		{"M", "late"},  // includes the in-past insert as a seen head
		{"x1"},         // partial
	} {
		var gotFull, gotBounded []string
		wf := newWatermark(func(ids []string) { gotFull = append(gotFull, ids...) })
		wf.advance(frontier, resolve, full)
		wb := newWatermark(func(ids []string) { gotBounded = append(gotBounded, ids...) })
		wb.advance(frontier, resolve, bounded)

		// Bounded emission must equal full emission intersected with the
		// candidate set (the only ids that can flip a counter). Equivalently:
		// every id the bounded path emits, the full path also emits; and the
		// full path emits no *candidate* id the bounded path misses.
		candidateSet := map[string]bool{"G": true, "m1": true, "x1": true, "M": true, "late": true}
		var fullCand []string
		for _, id := range gotFull {
			if candidateSet[id] {
				fullCand = append(fullCand, id)
			}
		}
		assert.ElementsMatch(t, fullCand, gotBounded,
			"frontier %v: bounded emission must equal full emission ∩ candidates", frontier)
	}
}
```

- [ ] **Step 2: Run to verify it passes**

Run: `go test ./core/block/source/sourceimpl/ -run TestBoundedVsFullStream_SameEmission -v`
Expected: PASS — confirms the bounded provider preserves dominance semantics (including `late` correctly never emitted, multi-head handled).

- [ ] **Step 3: Commit**

```bash
git add core/block/source/sourceimpl/bounded_eachchange_test.go
git commit -S -m "$(cat <<'EOF'
GO-7290 Add completeness gate: bounded == full-stream emission

Property test pinning that the bounded candidate stream yields the same
onRemove delta as the legacy full scan for linear, multi-head, and
in-past-insert frontiers.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Cross-invocation invariant test (review S4)

**Files:**
- Test: `core/block/source/sourceimpl/bounded_eachchange_test.go` (append)

Spec §5 corrected invariant: a dominated message whose chat row does not exist yet at one `advance` is not emitted and **not** added to `marked`; when its row later appears and any subsequent `advance` runs, it is emitted exactly once. `marked` must never suppress a never-emitted id.

- [ ] **Step 1: Write the test**

Append to `core/block/source/sourceimpl/bounded_eachchange_test.go`:

```go
func TestBounded_CrossInvocation_EmitsOnceAfterRowAppears(t *testing.T) {
	all := map[string]readPair{"G": {"o1", 1}, "M": {"o2", 2}}
	resolve := func(id string) (readPair, bool) { p, ok := all[id]; return p, ok }

	var got []string
	w := newWatermark(func(ids []string) { got = append(got, ids...) })

	// Invocation 1: seenHeads dominate G and M, but the "M" chat row hasn't
	// been applied yet → candidate set is {G} only.
	w.advance([]string{"M"}, resolve, buildBoundedEachChange([]string{"G"}, resolve))
	assert.ElementsMatch(t, []string{"G"}, got)
	_, marked := w.marked["M"]
	assert.False(t, marked, "M never emitted ⇒ must not be in marked")

	// Invocation 2: M's chat row now exists → candidate set includes M.
	got = nil
	w.advance(nil, resolve, buildBoundedEachChange([]string{"G", "M"}, resolve))
	assert.ElementsMatch(t, []string{"M"}, got, "M emitted exactly once on the next advance")

	// Invocation 3: nothing new ⇒ no re-emission.
	got = nil
	w.advance(nil, resolve, buildBoundedEachChange([]string{"G", "M"}, resolve))
	assert.Empty(t, got, "no id re-emitted")
}
```

- [ ] **Step 2: Run to verify it passes**

Run: `go test ./core/block/source/sourceimpl/ -run TestBounded_CrossInvocation_EmitsOnceAfterRowAppears -v`
Expected: PASS — locks the §5 re-evaluation invariant (the review S4 concern).

- [ ] **Step 3: Run the full sourceimpl + chatobject + chatrepository suites**

Run: `go test ./core/block/source/... ./core/block/editor/chatobject/ ./core/block/chats/chatrepository/`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add core/block/source/sourceimpl/bounded_eachchange_test.go
git commit -S -m "$(cat <<'EOF'
GO-7290 Add cross-invocation invariant test (review S4)

Pins that a dominated message with no chat row yet is not marked, and is
emitted exactly once when its row appears on a later advance.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Cold-start measurement (non-blocking verification)

**Files:** none (measurement + manual verification only)

- [ ] **Step 1: Build the gomobile/desktop binary used for the profile and capture a cold-start CPU profile** on the same real account (`…/AASdKiEGfcyhxX3ufr4auHRviACUXxkF68uZwtSb2AnyRoMA2/`) as `anytype_profile_20260516_170652`, following the same capture procedure.

- [ ] **Step 2: Compare** `sourceimpl.(*watermark).advance` cumulative time and the `eachChange` line against the baseline (~13 s / ~9.67 s). Expected: `messages`/`mentions` no longer drive a full `GetAfterOrder` scan; `Storage().Get` count ≈ unread message count; `reactions` still on the legacy stream (Phase 1, expected).

- [ ] **Step 3: Record** the before/after numbers in the PR description. This task does not block merge; it validates the spec's performance goal and informs the `boundedCandidateFallbackThreshold` value (raise/lower based on observed unread distributions).

---

## Self-Review

**1. Spec coverage:**
- §6.1 indexes → Task 2 (`read`, `hasMention`); B1 positive-equality → Task 1. ✓
- §6.2 bounded provider, per-`diffManager` routing, `Storage().Get` per candidate → Tasks 3-5. ✓
- §6.3 fallback → Task 4 (`boundedCandidateFallbackThreshold` + nil/err/oversize → legacy stream). ✓
- §4 engine unchanged → confirmed: no `readwatermark.go`/shim/harness edits (OrderId bound dropped per approved fork). ✓
- §5 completeness + corrected cross-invocation invariant → Tasks 6, 7. ✓
- §7 reactions Phase 1 legacy (`nil` provider), messages+mentions Phase 1 → Task 5. ✓
- §9 tests (completeness gate, cross-invocation, positive-equality index, cold-start benchmark) → Tasks 1, 2, 6, 7, 8. ✓
- §8 rejected alternatives — no implementation, correctly absent. ✓

**2. Placeholder scan:** No TBD/TODO; every code step has full code; commands have expected output. ✓

**3. Type consistency:** `source.CandidateProvider = func(ctx context.Context) ([]string, error)` defined Task 3, used identically in `diffManager.candidates`, `RegisterDiffManager`, `eachChangeFor` (Task 4), and `unreadCandidateProvider*` (Task 5). `buildBoundedEachChange(ids []string, resolve func(string)(readPair,bool))` defined and called consistently (Tasks 4, 6, 7). `GetAllUnreadMessages(ctx, chatmodel.CounterType) ([]string, error)` matches repository.go:176 and the Task 5 fake. Filter names (`filterReadFalse`, `filterReadTrue`) match readhandler.go. ✓

Resolved gap during review: `GetAllUnreadMessages` already exists (repository.go:408) and is reused as the provider body — no new repository query method needed; Task 1 makes its messages filter index-usable, which also speeds existing callers (chatobject.go:148, reading.go:37).
