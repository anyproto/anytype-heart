# Cleanup Suggestions (space-wide orphan list) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an on-demand `ObjectCleanupSuggestions` RPC returning every orphan in a space as a forest (roots + subtrees, objects and files) with caller-selected keys, plus a `createdInContextIgnored` relation and a thin `ObjectCleanupSuggestionIgnore` RPC to permanently exclude objects. Task 0 first unifies the protocol vocabulary by renaming the phase-1 `OrphansDetected` event to `CleanupSuggestion`.

**Vocabulary boundary (deliberate):** the **protocol/client** says *cleanup suggestion*; **internal Go** keeps *orphan* (the precise term for a node with no inbound references) — `objectgc`, `OrphanCandidates`, `ListOrphans`, `OrphanItem`, `OrphanReason`. Do not rename the internals.

**Architecture:** A new `ListOrphans` method on the existing `objectgc` component computes the orphan set as the greatest fixed point of "candidates whose active backlinks all fall inside the set" — the cascade's existing eviction rule applied space-wide. Three store queries total (candidates, active-backlink batch, parent-state batch); the rest is an in-memory `O(V+E)` worklist. The RPC handler projects requested keys. Ignore is a `details`-source relation written with a new `ChangeTypeCreatedInContext` so it syncs without bumping `lastModifiedDate`.

**Tech Stack:** Go, gogo/protobuf (`make protos`), mockery (`make test-deps`), testify, anystore.

**Spec:** `docs/superpowers/specs/2026-06-14-space-orphan-list-design.md`

**Branch:** `go-7323-cascade-deletion-orphan-events` (continue on it; same issue/PR).

## Global Constraints

- Commit messages MUST be prefixed `GO-7323 ` (repo convention, `CLAUDE.md`).
- Errors MUST be wrapped: `fmt.Errorf("operation description: %w", err)`. Never return a bare `err`.
- Tests use the fixture pattern + testify (`require.NoError`, `assert.ElementsMatch`).
- `go build ./...` prints benign `ld: warning: ignoring duplicate libraries` — ignore those lines.
- This builds on the GO-7323 phase-1 code already on this branch (`OrphanCandidates`, `skipCascade`, the `createdInContextRef` gate).

---

## Key facts the executor must know

- **Deletion tombstones.** `spaceindex/delete.go` `DeleteObject` keeps `id` + `isDeleted=true` ("so we can distinguish links to deleted and not-yet-loaded objects"). So a parent absent from the store means *never synced*, not *deleted*.
- **`Query` injects implicit filters** (`isArchived != true`, `isDeleted != true`). `QueryRaw` does not. Build raw filters as `&database.Filters{FilterObj: database.FilterIn{...}}` (see `objectcreator/installer.go:295`).
- **`FilterIn.Value` is `[]domain.Value`**, not a `domain.Value` list wrapper.
- **`queryActiveIds(idx, ids)` already exists** in `objectgc.go` — one `Id IN [...]` query returning the active subset.
- **Writing a detail with a custom change type** (`delete.go:187-190`) bypasses `basic.SetDetails` validation entirely:
  ```go
  sb.NewState() → st.SetDetail(key, val) → st.SetChangeType(...) → sb.Apply(st)
  ```
  `SetLastModified` only runs when `changeType == domain.ChangeTypeUserChange` (`smartblock.go`), so a non-user change type skips the `lastModifiedDate` bump while still producing a real (syncing) CRDT change.
- **`Details.CopyOnlyKeys(keys...)` and `Details.ToProto()`** live on `domain.GenericMap` (`core/domain/genericmap.go`). `ObjectSearch` (`core/object.go:132-144`) is the projection model.
- **Adding a method to the `ObjectGC` interface breaks 3 test stubs.** Two are compile-checked; **`fileGCDummy` in `core/block/chats/service_test.go` is `app.Register`-ed and resolved by interface at runtime**, so a stale signature compiles fine and fails only when the chats tests run. Always run `go test ./core/block/chats/...`.

---

## File Structure

| File | Change |
|------|--------|
| `pb/protos/events.proto` | **Task 0** — rename `Object.OrphansDetected` → `Object.CleanupSuggestion` (field 146 kept) |
| `core/block/{objectgc,detailservice,delete.go,editor/smartblock,chats}` | **Task 0** — rename event references |
| `docs/GO-7323-cascade-deletion-client-impl.md` | **Task 0** — update client contract |
| `pkg/lib/bundle/relations.json` | Add `createdInContextIgnored` relation |
| `pkg/lib/bundle/relation.gen.go` | Regenerated (`go generate ./pkg/lib/bundle/...`) |
| `core/domain/types.go` | Add `ChangeTypeCreatedInContext` + `String()` case |
| `core/block/objectgc/orphanlist.go` | **New** — `ListOrphans`, `OrphanItem`, `OrphanReason`, fixed point |
| `core/block/objectgc/orphanlist_test.go` | **New** — unit tests |
| `core/block/objectgc/objectgc.go` | Add `ListOrphans` to interface; honor `createdInContextIgnored` in 3 GC paths |
| `core/block/detailservice/service.go` | Add `SetCreatedInContextIgnored` to `Service` |
| `core/block/detailservice/set_details.go` | Implement `SetCreatedInContextIgnored` |
| `pb/protos/commands.proto` | `Rpc.Object.CleanupSuggestions`, `Rpc.Object.CleanupSuggestionIgnore` |
| `pb/protos/service/service.proto` | Register both RPCs |
| `core/object.go` | `ObjectCleanupSuggestions` handler + key projection |
| `core/details.go` | `ObjectCleanupSuggestionIgnore` handler |
| Test stubs | `detailservice/service_test.go`, `smartblock/objectgclinks_test.go`, `chats/service_test.go` |

---

## Task 0: Rename the phase-1 event `OrphansDetected` → `CleanupSuggestion`

**Why now:** GO-7323 phase 1 is pushed but **unmerged**, so this protocol has never shipped. Unifying
the vocabulary (the popup event and the cleanup list are the same concept) is at its cheapest today.

**Files:**
- Modify: `pb/protos/events.proto`, `pb/protos/commands.proto` (comments only)
- Generate: `pb/events.pb.go`, `pb/commands.pb.go`
- Modify: `core/block/objectgc/objectgc.go`, `core/block/objectgc/objectgc_test.go`
- Modify: `core/block/detailservice/set_details.go`, `core/block/detailservice/service_test.go`
- Modify: `core/block/delete.go`
- Modify: `core/block/editor/smartblock/smartblock.go`, `core/block/editor/smartblock/objectgclinks_test.go`
- Modify: `core/block/chats/service.go` (comment only)
- Modify: `docs/GO-7323-cascade-deletion-client-impl.md`

**Produces:** `pb.EventObjectCleanupSuggestion`, `pb.EventMessageValueOfObjectCleanupSuggestion`, enum constants `pb.EventObjectCleanupSuggestion_archive` / `_delete` / `_linkRemoval`.

- [ ] **Step 1: Rename the message and oneof field in `events.proto`**

In `pb/protos/events.proto`:
- `message OrphansDetected {` → `message CleanupSuggestion {`
- the oneof entry `Object.OrphansDetected objectOrphansDetected = 146;` → `Object.CleanupSuggestion objectCleanupSuggestion = 146;`

**Keep field number 146**, and keep the payload (`objectIds`, `contextId`, `trigger`) and the
`Trigger` enum (`archive`, `delete`, `linkRemoval`) exactly as they are.

- [ ] **Step 2: Update the `skipCascade` comments in `commands.proto`**

Both `SetIsArchived.Request` and `ListSetIsArchived.Request` document `skipCascade` as suppressing
the "OrphansDetected event". Replace that phrase with "CleanupSuggestion event". (`skipCascade` keeps
its name — it skips the whole cascade: file auto-archive *and* the suggestion.)

- [ ] **Step 3: Regenerate**

Run: `make protos`
Expected: `pb/events.pb.go`, `pb/commands.pb.go`, `docs/proto.md` modified.

- [ ] **Step 4: Verify the new generated names**

Run:
```bash
grep -nE "type EventObjectCleanupSuggestion struct|EventMessageValueOfObjectCleanupSuggestion|EventObjectCleanupSuggestion_archive" pb/events.pb.go | head
```
Expected: all three present. If the enum constants differ, note the real names — they are used in Step 5.

- [ ] **Step 5: Rename the Go references**

The generated pb files are already correct; only hand-written code refers to the old names. Run this
from the repo root (ordered longest-first so nothing cascades wrongly):

```bash
for f in core/block/objectgc/objectgc.go core/block/objectgc/objectgc_test.go \
         core/block/detailservice/set_details.go core/block/detailservice/service_test.go \
         core/block/delete.go \
         core/block/editor/smartblock/smartblock.go core/block/editor/smartblock/objectgclinks_test.go \
         core/block/chats/service.go; do
  perl -pi -e 's/EventMessageValueOfObjectOrphansDetected/EventMessageValueOfObjectCleanupSuggestion/g;
               s/EventObjectOrphansDetected/EventObjectCleanupSuggestion/g;
               s/ObjectOrphansDetected/ObjectCleanupSuggestion/g;
               s/objectOrphansDetected/objectCleanupSuggestion/g;
               s/OrphansDetected/CleanupSuggestion/g;' "$f"
done
```

This renames the `FilterExplicitIds` switch case in `objectgc.go`, the event construction in
`set_details.go` (`appendGCEvents`), `delete.go` (broadcast), `smartblock.go`
(`performGCOnLinksRemoval`), and the test assertions. It also fixes the comment in `chats/service.go`.

- [ ] **Step 6: Confirm nothing stale remains in hand-written code**

Run: `grep -rn "OrphansDetected" core/ pb/protos/ | grep -v "\.pb\.go"`
Expected: no output.

- [ ] **Step 7: Build and test**

Run: `go build ./... && go test ./core/block/objectgc/... ./core/block/detailservice/... ./core/block/editor/smartblock/... ./core/block/chats/...`
Expected: build success; all `ok`. The renamed tests (e.g. `TestFilterExplicitIds_RemovesFromCleanupSuggestion`) pass.

- [ ] **Step 8: Update the client-facing brief**

In `docs/GO-7323-cascade-deletion-client-impl.md`, replace `OrphansDetected` with `CleanupSuggestion`
and `objectOrphansDetected` with `objectCleanupSuggestion` throughout, and note in the protocol
section that the read RPC is `ObjectCleanupSuggestions` and the ignore RPC is
`ObjectCleanupSuggestionIgnore` (phase 2).

- [ ] **Step 9: Commit**

```bash
git add pb/ core/ docs/
git commit -m "GO-7323 Rename OrphansDetected event to CleanupSuggestion

Phase 1 is unmerged, so the protocol has never shipped. The popup event and
the new space-wide cleanup list are the same concept; unify the client-facing
vocabulary on 'cleanup suggestion'. Internal Go keeps the precise term
'orphan'. Field number 146 and the Trigger enum are unchanged."
```

> **Follow-up, separate branch:** the e2e tests in the sibling repo `anytype-suite` (branch
> `go-7323-cascade-deletion-orphan-events`, `src/scenarios/cascade-deletion/` — 8 files) reference
> `objectOrphansDetected` and must be renamed too. **Its working tree is currently checked out on a
> different branch (`go-7320-position-events-convergence`)** — do not switch it as a side effect of
> this task.

---

## Task 1: Bundle relation `createdInContextIgnored` + `ChangeTypeCreatedInContext`

**Files:**
- Modify: `pkg/lib/bundle/relations.json`
- Generate: `pkg/lib/bundle/relation.gen.go`
- Modify: `core/domain/types.go`

**Produces:** `bundle.RelationKeyCreatedInContextIgnored` (a `domain.RelationKey`), `domain.ChangeTypeCreatedInContext`.

- [ ] **Step 1: Add the relation to `relations.json`**

Insert this object into the JSON array (it is sorted by nothing in particular; place it next to the other `createdInContext*` entries). Mirrors `isHidden` — a hidden checkbox relation stored in details:

```json
  {
    "description": "Ignore this object's createdInContext link: it is excluded from cleanup suggestions and from automatic context-driven archival",
    "format": "checkbox",
    "hidden": true,
    "key": "createdInContextIgnored",
    "maxCount": 1,
    "name": "Created in context ignored",
    "readonly": false,
    "source": "details"
  },
```

Do **not** add it to `systemRelations.json`: the ignore RPC writes the detail via `st.SetDetail` + `Apply`, which never calls `FetchRelationByKey`. `createdInContextRef` is precedent for a details relation that is not a system relation.

- [ ] **Step 2: Regenerate the bundle**

Run: `go generate ./pkg/lib/bundle/...`
Expected: `pkg/lib/bundle/relation.gen.go` modified; it now contains `RelationKeyCreatedInContextIgnored`.

- [ ] **Step 3: Verify the generated key exists**

Run: `grep -n "RelationKeyCreatedInContextIgnored" pkg/lib/bundle/relation.gen.go`
Expected: one or more lines, e.g. `RelationKeyCreatedInContextIgnored RelationKey = "createdInContextIgnored"`.

- [ ] **Step 4: Add the change type**

In `core/domain/types.go`, add `ChangeTypeCreatedInContext` as the **last** constant in the `const` block (appending keeps existing values stable):

```go
	ChangeTypeDescriptionToggle
	ChangeTypeCreatedInContext
)
```

and add its `String()` case just before `default:`:

```go
	case ChangeTypeCreatedInContext:
		return "CreatedInContext"
```

> This change type is deliberately **generic**, not ignore-specific: any `createdInContext`-related
> write should use it. A concrete second consumer already exists — `core/migration/objectcontext.go`
> backfills `createdInContext`/`createdInContextRef` via `detailsService.ModifyDetails`, which applies
> with the default *user* change type and therefore bumps `lastModifiedDate` on every backfilled file.
> Converting that migration is a clean follow-up (out of scope for this plan).

- [ ] **Step 5: Build**

Run: `go build ./pkg/lib/bundle/... ./core/domain/...`
Expected: success.

- [ ] **Step 6: Commit**

```bash
git add pkg/lib/bundle/relations.json pkg/lib/bundle/relation.gen.go core/domain/types.go
git commit -m "GO-7323 Add createdInContextIgnored relation and ChangeTypeCreatedInContext"
```

---

## Task 2: `objectgc.ListOrphans` — the orphan forest

**Files:**
- Create: `core/block/objectgc/orphanlist.go`
- Create: `core/block/objectgc/orphanlist_test.go`
- Modify: `core/block/objectgc/objectgc.go` (interface only)
- Modify: `core/block/detailservice/service_test.go`, `core/block/editor/smartblock/objectgclinks_test.go`, `core/block/chats/service_test.go` (stubs)

**Consumes:** `bundle.RelationKeyCreatedInContextIgnored` (Task 1). Existing `queryActiveIds`, `makeGCEligibleLayouts`, `gc.backlinksWatcher`, `gc.objectStore`.

**Produces:**
```go
type OrphanReason int
const (
	OrphanReasonNone OrphanReason = iota
	OrphanReasonContextArchived
	OrphanReasonContextDeleted
	OrphanReasonContextUnlinked
)
type OrphanItem struct {
	Details *domain.Details
	IsRoot  bool
	Reason  OrphanReason // set on roots only; OrphanReasonNone for descendants
}
ListOrphans(spaceId string) ([]OrphanItem, error) // on the ObjectGC interface
```

- [ ] **Step 1: Add `ListOrphans` to the `ObjectGC` interface**

In `core/block/objectgc/objectgc.go`, inside `type ObjectGC interface { ... }`, add:

```go
	// ListOrphans returns every orphan in the space as a forest: roots (whose createdInContext
	// parent is outside the orphan set) plus their full transitive subtrees, objects and files.
	// Pure read operation.
	ListOrphans(spaceId string) ([]OrphanItem, error)
```

- [ ] **Step 2: Create `orphanlist.go` with the types and query helpers**

Create `core/block/objectgc/orphanlist.go`:

```go
package objectgc

import (
	"fmt"
	"sort"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// OrphanReason explains why a root is orphaned. It is set on roots only; descendants carry
// OrphanReasonNone because the reason belongs to their root.
type OrphanReason int

const (
	OrphanReasonNone OrphanReason = iota
	OrphanReasonContextArchived
	OrphanReasonContextDeleted
	OrphanReasonContextUnlinked
)

// OrphanItem is one member of the removable set.
type OrphanItem struct {
	Details *domain.Details
	IsRoot  bool
	Reason  OrphanReason
}

// parentState is the store state of a candidate's createdInContext parent.
type parentState int

const (
	parentAbsent parentState = iota // not indexed at all → never synced (deletion tombstones)
	parentArchived
	parentDeleted
	parentActive
)

// queryParentStates resolves existence + archived/deleted state for the given ids using QueryRaw,
// which (unlike Query) applies no implicit isArchived/isDeleted filters. Ids not returned are absent.
func (gc *objectGC) queryParentStates(idx spaceindex.Store, ids map[string]struct{}) (map[string]parentState, error) {
	states := make(map[string]parentState, len(ids))
	if len(ids) == 0 {
		return states, nil
	}
	values := make([]domain.Value, 0, len(ids))
	for id := range ids {
		states[id] = parentAbsent
		values = append(values, domain.String(id))
	}
	records, err := idx.QueryRaw(&database.Filters{FilterObj: database.FilterIn{
		Key:   bundle.RelationKeyId,
		Value: values,
	}}, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("query parent states: %w", err)
	}
	for _, r := range records {
		id := r.Details.GetString(bundle.RelationKeyId)
		switch {
		case r.Details.GetBool(bundle.RelationKeyIsDeleted):
			states[id] = parentDeleted
		case r.Details.GetBool(bundle.RelationKeyIsArchived):
			states[id] = parentArchived
		default:
			states[id] = parentActive
		}
	}
	return states, nil
}
```

- [ ] **Step 3: Write the failing test for the simplest case (archived parent → root)**

Create `core/block/objectgc/orphanlist_test.go`. It reuses `newFixture`, `regularObject`, `archivedObject`, `basicObjectWithRef` from `objectgc_test.go` (same package):

```go
package objectgc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// ids returns the ids of the items, for order-insensitive assertions.
func ids(items []OrphanItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Details.GetString(bundle.RelationKeyId))
	}
	return out
}

// find returns the item with the given id.
func find(t *testing.T, items []OrphanItem, id string) OrphanItem {
	t.Helper()
	for _, it := range items {
		if it.Details.GetString(bundle.RelationKeyId) == id {
			return it
		}
	}
	t.Fatalf("item %s not found in %v", id, ids(items))
	return OrphanItem{}
}

func TestListOrphans_ArchivedParent_ChildIsRoot(t *testing.T) {
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, basicObjectWithRef("child", "parent", "block1", []string{"parent"}))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"child"}, ids(items))
	child := find(t, items, "child")
	assert.True(t, child.IsRoot)
	assert.Equal(t, OrphanReasonContextArchived, child.Reason)
}
```

- [ ] **Step 4: Run it and watch it fail**

Run: `go test ./core/block/objectgc/ -run TestListOrphans_ArchivedParent -v`
Expected: FAIL — compile error `fx.ListOrphans undefined` (the method does not exist yet).

- [ ] **Step 5: Implement `ListOrphans`**

Append to `core/block/objectgc/orphanlist.go`:

```go
// ListOrphans computes the space's orphan forest.
//
// Candidates: active objects with createdInContext + non-empty createdInContextRef, a GC-eligible
// layout, not createdInContextIgnored, and whose parent is present in the store (sync-gap guard).
//
// The removable set S is the greatest subset of the candidates such that every member's *active*
// backlinks fall inside S. Anything reachable from an active object outside S is evicted, and that
// eviction cascades. Roots are the members whose parent is outside S.
func (gc *objectGC) ListOrphans(spaceId string) ([]OrphanItem, error) {
	gc.backlinksWatcher.FlushUpdates()
	idx := gc.objectStore.SpaceIndex(spaceId)

	// 1) Candidates. Query injects the implicit isArchived/isDeleted filters, so these are active.
	records, err := idx.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyCreatedInContext,
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
			},
			{
				RelationKey: bundle.RelationKeyCreatedInContextRef,
				Condition:   model.BlockContentDataviewFilter_NotEmpty, // collections have empty ref
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List(makeGCEligibleLayouts()),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query orphan candidates: %w", err)
	}

	candidates := make(map[string]*domain.Details, len(records))
	for _, r := range records {
		// The ignore gate is applied in memory to avoid depending on NotEqual-vs-missing-key
		// filter semantics.
		if r.Details.GetBool(bundle.RelationKeyCreatedInContextIgnored) {
			continue
		}
		candidates[r.Details.GetString(bundle.RelationKeyId)] = r.Details
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	// 2) Parent states, for the sync-gap guard and the per-root reason. Parents that are themselves
	// candidates are active by construction and need no lookup.
	toLookup := make(map[string]struct{})
	for _, d := range candidates {
		p := d.GetString(bundle.RelationKeyCreatedInContext)
		if _, ok := candidates[p]; !ok {
			toLookup[p] = struct{}{}
		}
	}
	parentStates, err := gc.queryParentStates(idx, toLookup)
	if err != nil {
		return nil, err
	}

	// Sync-gap guard: a parent absent from the store was never synced (deletion tombstones the row),
	// so we must not recommend removing its children.
	for id, d := range candidates {
		p := d.GetString(bundle.RelationKeyCreatedInContext)
		if _, ok := candidates[p]; ok {
			continue
		}
		if parentStates[p] == parentAbsent {
			delete(candidates, id)
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	// 3) Active status of every backlink target that is not itself a candidate.
	linksToCheck := make(map[string]struct{})
	for id, d := range candidates {
		for _, b := range d.GetStringList(bundle.RelationKeyBacklinks) {
			if b == id {
				continue
			}
			if _, ok := candidates[b]; ok {
				continue
			}
			linksToCheck[b] = struct{}{}
		}
	}
	activeIds, err := gc.queryActiveIds(idx, linksToCheck)
	if err != nil {
		return nil, fmt.Errorf("query active backlinks: %w", err)
	}

	inS := gc.evictToFixedPoint(candidates, activeIds)
	if len(inS) == 0 {
		return nil, nil
	}
	return gc.buildForest(candidates, inS, parentStates), nil
}

// evictToFixedPoint returns the greatest subset of candidates whose active backlinks all fall
// inside the subset. Worklist algorithm: O(V+E), not a naive restart loop.
func (gc *objectGC) evictToFixedPoint(candidates map[string]*domain.Details, activeIds map[string]struct{}) map[string]struct{} {
	inS := make(map[string]struct{}, len(candidates))
	for id := range candidates {
		inS[id] = struct{}{}
	}

	// reverse index: backlink target -> candidates that list it as a backlink
	revIdx := make(map[string][]string)
	for id, d := range candidates {
		for _, b := range d.GetStringList(bundle.RelationKeyBacklinks) {
			if b == id {
				continue
			}
			revIdx[b] = append(revIdx[b], id)
		}
	}

	// Seed: evict candidates that have an active backlink outside the candidate set.
	var queue []string
	for id, d := range candidates {
		for _, b := range d.GetStringList(bundle.RelationKeyBacklinks) {
			if b == id {
				continue
			}
			if _, ok := candidates[b]; ok {
				continue
			}
			if _, active := activeIds[b]; active {
				queue = append(queue, id)
				break
			}
		}
	}

	// Propagate: an evicted candidate is active and now outside S, so anything backlinking it
	// must be evicted too.
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, ok := inS[id]; !ok {
			continue
		}
		delete(inS, id)
		for _, x := range revIdx[id] {
			if _, ok := inS[x]; ok {
				queue = append(queue, x)
			}
		}
	}
	return inS
}
```

- [ ] **Step 6: Implement `buildForest` (roots, cycles, reasons)**

Append to `core/block/objectgc/orphanlist.go`:

```go
// buildForest marks roots (parent outside S), resolves each root's reason, and handles
// createdInContext cycles (a component with no parent-outside-S) by electing the lowest id as root.
func (gc *objectGC) buildForest(candidates map[string]*domain.Details, inS map[string]struct{}, parentStates map[string]parentState) []OrphanItem {
	parentOf := func(id string) string {
		return candidates[id].GetString(bundle.RelationKeyCreatedInContext)
	}

	children := make(map[string][]string)
	var roots []string
	for id := range inS {
		p := parentOf(id)
		if _, ok := inS[p]; ok {
			children[p] = append(children[p], id)
		} else {
			roots = append(roots, id)
		}
	}

	visited := make(map[string]struct{}, len(inS))
	var markReachable func(from string)
	markReachable = func(from string) {
		if _, ok := visited[from]; ok {
			return
		}
		visited[from] = struct{}{}
		for _, c := range children[from] {
			markReachable(c)
		}
	}
	for _, r := range roots {
		markReachable(r)
	}

	// Anything unvisited sits in (or below) a createdInContext cycle: its parent chain never
	// leaves S. Elect the lowest id on the cycle as the root so rendering is deterministic.
	var leftover []string
	for id := range inS {
		if _, ok := visited[id]; !ok {
			leftover = append(leftover, id)
		}
	}
	sort.Strings(leftover)
	for _, start := range leftover {
		if _, ok := visited[start]; ok {
			continue
		}
		// Walk parents until we revisit a node in this walk — those nodes form the cycle.
		seen := map[string]int{}
		cur := start
		for {
			if _, ok := seen[cur]; ok {
				break
			}
			seen[cur] = len(seen)
			cur = parentOf(cur)
		}
		cycleStart := seen[cur]
		cycle := make([]string, 0, len(seen))
		for id, order := range seen {
			if order >= cycleStart {
				cycle = append(cycle, id)
			}
		}
		sort.Strings(cycle)
		root := cycle[0]
		roots = append(roots, root)
		markReachable(root)
	}

	rootSet := make(map[string]struct{}, len(roots))
	for _, r := range roots {
		rootSet[r] = struct{}{}
	}

	reasonFor := func(id string) OrphanReason {
		p := parentOf(id)
		if _, ok := candidates[p]; ok {
			// Parent is an active candidate that was evicted (or a cycle peer): it is alive and does
			// not link this object, otherwise this object would have been evicted too.
			return OrphanReasonContextUnlinked
		}
		switch parentStates[p] {
		case parentArchived:
			return OrphanReasonContextArchived
		case parentDeleted:
			return OrphanReasonContextDeleted
		default:
			return OrphanReasonContextUnlinked
		}
	}

	items := make([]OrphanItem, 0, len(inS))
	for id := range inS {
		item := OrphanItem{Details: candidates[id]}
		if _, ok := rootSet[id]; ok {
			item.IsRoot = true
			item.Reason = reasonFor(id)
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Details.GetString(bundle.RelationKeyId) < items[j].Details.GetString(bundle.RelationKeyId)
	})
	return items
}
```

- [ ] **Step 7: Fix the three `ObjectGC` test stubs (two compile-checked, one runtime-only)**

`core/block/detailservice/service_test.go` — add to `fileGCStub`:

```go
func (f *fileGCStub) ListOrphans(spaceId string) ([]objectgc.OrphanItem, error) {
	return nil, nil
}
```

`core/block/editor/smartblock/objectgclinks_test.go` — add to `objectGCCallRecorder`:

```go
func (r *objectGCCallRecorder) ListOrphans(spaceId string) ([]objectgc.OrphanItem, error) {
	return nil, nil
}
```

`core/block/chats/service_test.go` — add to `fileGCDummy`:

```go
func (s *fileGCDummy) ListOrphans(spaceId string) ([]objectgc.OrphanItem, error) {
	return nil, nil
}
```

> `fileGCDummy` is `app.Register`-ed and resolved by interface **at runtime** — a stale signature compiles fine and only fails when the chats tests run. Do not skip it.

- [ ] **Step 8: Run the first test — it should now pass**

Run: `go test ./core/block/objectgc/ -run TestListOrphans_ArchivedParent -v`
Expected: PASS.

- [ ] **Step 9: Add the rest of the unit tests**

Append to `core/block/objectgc/orphanlist_test.go`:

```go
func TestListOrphans_DeletedParent_ReasonDeleted(t *testing.T) {
	fx := newFixture(t)
	fx.addObject(t, deletedObject("parent"))
	fx.addObject(t, basicObjectWithRef("child", "parent", "block1", []string{"parent"}))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"child"}, ids(items))
	assert.Equal(t, OrphanReasonContextDeleted, find(t, items, "child").Reason)
}

func TestListOrphans_LinkRemoved_ParentActive_ReasonUnlinked(t *testing.T) {
	// parent is alive but no longer links the child (child's backlinks are empty)
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, basicObjectWithRef("child", "parent", "block1", nil))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"child"}, ids(items))
	child := find(t, items, "child")
	assert.True(t, child.IsRoot)
	assert.Equal(t, OrphanReasonContextUnlinked, child.Reason)
}

func TestListOrphans_ReachableObject_Excluded(t *testing.T) {
	// parent is alive and still links the child → child has an active backlink outside S
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, basicObjectWithRef("child", "parent", "block1", []string{"parent"}))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestListOrphans_CascadeEviction(t *testing.T) {
	// X has an external active backlink → evicted. Y's only backlink is X → also evicted.
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, regularObject("external"))
	fx.addObject(t, basicObjectWithRef("X", "parent", "block1", []string{"external"}))
	fx.addObject(t, basicObjectWithRef("Y", "parent", "block1", []string{"X"}))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestListOrphans_Descendant_NotRoot(t *testing.T) {
	// parent archived → P is a root; X created in P, backlinked only by P → descendant
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, basicObjectWithRef("P", "parent", "block1", []string{"parent"}))
	fx.addObject(t, basicObjectWithRef("X", "P", "block1", []string{"P"}))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"P", "X"}, ids(items))
	assert.True(t, find(t, items, "P").IsRoot)
	assert.Equal(t, OrphanReasonContextArchived, find(t, items, "P").Reason)
	x := find(t, items, "X")
	assert.False(t, x.IsRoot)
	assert.Equal(t, OrphanReasonNone, x.Reason)
}

func TestListOrphans_Cycle_DeterministicRoot(t *testing.T) {
	// A created in B, B created in A, nothing else links them
	fx := newFixture(t)
	fx.addObject(t, basicObjectWithRef("A", "B", "block1", []string{"B"}))
	fx.addObject(t, basicObjectWithRef("B", "A", "block1", []string{"A"}))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"A", "B"}, ids(items))
	assert.True(t, find(t, items, "A").IsRoot, "lowest id is elected root")
	assert.False(t, find(t, items, "B").IsRoot)
}

func TestListOrphans_ParentAbsentFromStore_Skipped(t *testing.T) {
	// "ghost" is never added to the store → sync gap, not a deletion
	fx := newFixture(t)
	fx.addObject(t, basicObjectWithRef("child", "ghost", "block1", nil))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestListOrphans_EmptyRef_Excluded(t *testing.T) {
	// collection-created object: empty createdInContextRef
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, basicObject("child", "parent", []string{"parent"}))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestListOrphans_SystemLayout_Excluded(t *testing.T) {
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, systemObject("sysobj", "parent", []string{"parent"}))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestListOrphans_FileIncluded(t *testing.T) {
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, fileObjectWithRef("f1", "parent", "block1", []string{"parent"}))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"f1"}, ids(items))
	assert.True(t, find(t, items, "f1").IsRoot)
}

func TestListOrphans_Ignored_Excluded_AndDropsSubtree(t *testing.T) {
	// ignoring root B must also drop its child C: C's only backlink is B, which is active and
	// no longer a candidate → C is evicted.
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:                  domain.String("B"),
			bundle.RelationKeyResolvedLayout:      domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyCreatedInContext:    domain.String("parent"),
			bundle.RelationKeyCreatedInContextRef: domain.String("block1"),
			bundle.RelationKeyBacklinks:           domain.StringList([]string{"parent"}),
			bundle.RelationKeyCreatedInContextIgnored:       domain.Bool(true),
		},
	})
	fx.addObject(t, basicObjectWithRef("C", "B", "block1", []string{"B"}))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.Empty(t, items)
}
```

Add these imports to `orphanlist_test.go`:

```go
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
```

- [ ] **Step 10: Run the full objectgc suite**

Run: `go test ./core/block/objectgc/...`
Expected: `ok` — all `TestListOrphans_*` plus the existing GO-7323 tests pass.

- [ ] **Step 11: Build everything and run the runtime-stub package**

Run: `go build ./... && go test ./core/block/chats/... ./core/block/detailservice/... ./core/block/editor/smartblock/...`
Expected: build success; all `ok`. (If chats panics with "component with interface *objectgc.ObjectGC is not found", the `fileGCDummy` stub in Step 7 was missed.)

- [ ] **Step 12: Commit**

```bash
git add core/block/objectgc/ core/block/detailservice/service_test.go core/block/editor/smartblock/objectgclinks_test.go core/block/chats/service_test.go
git commit -m "GO-7323 Add objectgc.ListOrphans computing the space orphan forest"
```

---

## Task 3: Honor `createdInContextIgnored` in the three GC candidate paths

**Files:**
- Modify: `core/block/objectgc/objectgc.go`
- Modify: `core/block/objectgc/objectgc_test.go`

**Consumes:** `bundle.RelationKeyCreatedInContextIgnored` (Task 1).

An ignored object must never appear in an `CleanupSuggestion` popup, and an ignored level-1 file must not be auto-archived. Apply the gate as an in-memory predicate on the already-loaded record details, exactly like the existing layout checks.

- [ ] **Step 1: Write the failing tests**

Append to `core/block/objectgc/objectgc_test.go`:

```go
// ignoredBasicObject builds a candidate object that the user has ignored.
func ignoredBasicObject(id, createdInContext, ref string, backlinks []string) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:                  domain.String(id),
		bundle.RelationKeyResolvedLayout:      domain.Int64(int64(model.ObjectType_basic)),
		bundle.RelationKeyCreatedInContext:    domain.String(createdInContext),
		bundle.RelationKeyCreatedInContextRef: domain.String(ref),
		bundle.RelationKeyBacklinks:           domain.StringList(backlinks),
		bundle.RelationKeyCreatedInContextIgnored:       domain.Bool(true),
	}
}

func TestCheckObjectsOnObjectArchived_IgnoredObject_NotACandidate(t *testing.T) {
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, ignoredBasicObject("child", "parent", "block1", []string{"parent"}))

	res, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	require.NoError(t, err)
	assert.Empty(t, res.Files)
	assert.Empty(t, res.Candidates)
}

func TestCheckObjectsOnObjectArchived_IgnoredLevel1File_NotAutoArchived(t *testing.T) {
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:                  domain.String("f1"),
			bundle.RelationKeyResolvedLayout:      domain.Int64(int64(model.ObjectType_image)),
			bundle.RelationKeyCreatedInContext:    domain.String("parent"),
			bundle.RelationKeyCreatedInContextRef: domain.String("block1"),
			bundle.RelationKeyBacklinks:           domain.StringList([]string{"parent"}),
			bundle.RelationKeyCreatedInContextIgnored:       domain.Bool(true),
		},
	})

	res, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	require.NoError(t, err)
	assert.Empty(t, res.Files)
	assert.Empty(t, res.Candidates)
}

func TestArchiveOrphansOnLinksRemoval_IgnoredObject_NotACandidate(t *testing.T) {
	fx := newFixture(t)
	fx.addObject(t, ignoredBasicObject("child", "parent", "block1", []string{"parent"}))

	res, err := fx.ArchiveOrphansOnLinksRemoval(testSpaceId, "parent", []string{"child"}, false, nil)

	require.NoError(t, err)
	assert.Empty(t, res.Files)
	assert.Empty(t, res.Candidates)
}
```

- [ ] **Step 2: Run them and watch them fail**

Run: `go test ./core/block/objectgc/ -run "Ignored" -v`
Expected: FAIL — the ignored objects are still collected (`res.Candidates` / `res.Files` non-empty).

- [ ] **Step 3: Gate Query 1 of the BFS**

In `core/block/objectgc/objectgc.go`, inside `collectOrphanedObjects`, in the loop that builds the per-level `candidates` map from `childRecords`, skip ignored records. Change:

```go
		for _, record := range childRecords {
			id := record.Details.GetString(bundle.RelationKeyId)
			if _, seen := visited[id]; seen {
				continue
			}
			candidates[id] = record.Details
		}
```

to:

```go
		for _, record := range childRecords {
			id := record.Details.GetString(bundle.RelationKeyId)
			if _, seen := visited[id]; seen {
				continue
			}
			if record.Details.GetBool(bundle.RelationKeyCreatedInContextIgnored) {
				// user detached this object's lifecycle from its creation context
				continue
			}
			candidates[id] = record.Details
		}
```

- [ ] **Step 4: Gate Query 2 (the backlinker case)**

In the same function, in the final `for _, record := range backlinkRecords` loop, immediately after the `if _, seen := visited[id]; seen { continue }` guard, add:

```go
			if record.Details.GetBool(bundle.RelationKeyCreatedInContextIgnored) {
				continue
			}
```

(There are two loops over `backlinkRecords`; add it to the **second** one — the one that appends to `res.Candidates`. Adding it to the first, which only collects `idsToCheck`, is harmless but unnecessary.)

- [ ] **Step 5: Gate `ArchiveOrphansOnLinksRemoval`**

In `ArchiveOrphansOnLinksRemoval`, at the top of the `for _, record := range records` loop, right after `id := record.Details.GetString(bundle.RelationKeyId)`, add:

```go
		if record.Details.GetBool(bundle.RelationKeyCreatedInContextIgnored) {
			continue
		}
```

- [ ] **Step 6: Run the tests**

Run: `go test ./core/block/objectgc/...`
Expected: `ok` — the three new tests pass and every existing GO-7323 test still passes.

- [ ] **Step 7: Commit**

```bash
git add core/block/objectgc/objectgc.go core/block/objectgc/objectgc_test.go
git commit -m "GO-7323 Exclude createdInContextIgnored objects from cascade GC candidates"
```

---

## Task 4: Proto — `ObjectCleanupSuggestions` + `ObjectCleanupSuggestionIgnore`

**Files:**
- Modify: `pb/protos/commands.proto`
- Modify: `pb/protos/service/service.proto`
- Generate: `pb/commands.pb.go`, `pb/service/service.pb.go`

**Produces:** `pb.RpcObjectCleanupSuggestionsRequest/Response`, `pb.RpcObjectCleanupSuggestionsResponseItem` (+ its `Reason` enum), `pb.RpcObjectCleanupSuggestionIgnoreRequest/Response`.

- [ ] **Step 1: Add both messages to `commands.proto`**

Inside `message Rpc { message Object { ... } }` — next to the existing `ListSetIsArchived` message — add:

```proto
        message CleanupSuggestions {
            message Request {
                string spaceId = 1;
                // relation keys to return; empty => a default set.
                // id, createdInContext and resolvedLayout are always included.
                repeated string keys = 2;
            }

            message Response {
                Error error = 1;
                repeated Item items = 2;

                message Item {
                    google.protobuf.Struct details = 1;
                    // true for forest roots (createdInContext parent is outside the orphan set)
                    bool isRoot = 2;
                    // set on roots only; none for descendants
                    Reason reason = 3;

                    enum Reason {
                        none = 0;
                        contextArchived = 1;
                        contextDeleted = 2;
                        contextUnlinked = 3;
                    }
                }

                message Error {
                    Code code = 1;
                    string description = 2;

                    enum Code {
                        NULL = 0;
                        UNKNOWN_ERROR = 1;
                        BAD_INPUT = 2;
                    }
                }
            }
        }

        message CleanupSuggestionIgnore {
            message Request {
                repeated string objectIds = 1;
                bool ignored = 2;
            }

            message Response {
                Error error = 1;

                message Error {
                    Code code = 1;
                    string description = 2;

                    enum Code {
                        NULL = 0;
                        UNKNOWN_ERROR = 1;
                        BAD_INPUT = 2;
                    }
                }
            }
        }
```

- [ ] **Step 2: Register both RPCs in `service.proto`**

In `pb/protos/service/service.proto`, in the `service ClientCommands` block, next to the other `Object*` rpcs, add:

```proto
    rpc ObjectCleanupSuggestions (anytype.Rpc.Object.CleanupSuggestions.Request) returns (anytype.Rpc.Object.CleanupSuggestions.Response);
    rpc ObjectCleanupSuggestionIgnore (anytype.Rpc.Object.CleanupSuggestionIgnore.Request) returns (anytype.Rpc.Object.CleanupSuggestionIgnore.Response);
```

- [ ] **Step 3: Regenerate**

Run: `make protos`
Expected: completes; `pb/commands.pb.go` and the service files are modified.

- [ ] **Step 4: Verify the generated names (they are referenced verbatim in Task 6)**

Run:
```bash
grep -nE "type RpcObjectCleanupSuggestionsRequest struct|type RpcObjectCleanupSuggestionsResponseItem struct|RpcObjectCleanupSuggestionsResponseItem_contextArchived|type RpcObjectCleanupSuggestionIgnoreRequest struct" pb/commands.pb.go | head
```
Expected: all four present. If the enum constants differ from `RpcObjectCleanupSuggestionsResponseItem_contextArchived` / `_contextDeleted` / `_contextUnlinked` / `_none`, note the real names — Task 6 references them.

- [ ] **Step 5: Build**

Run: `go build ./pb/...`
Expected: success.

- [ ] **Step 6: Commit**

```bash
git add pb/protos/commands.proto pb/protos/service/service.proto pb/ docs/
git commit -m "GO-7323 Add ObjectCleanupSuggestions and ObjectCleanupSuggestionIgnore protos"
```

---

## Task 5: `detailservice.SetCreatedInContextIgnored`

**Files:**
- Modify: `core/block/detailservice/service.go`
- Modify: `core/block/detailservice/set_details.go`
- Modify: `core/block/detailservice/service_test.go`
- Regenerate: `core/block/detailservice/mock_detailservice/mock_Service.go`

**Consumes:** `bundle.RelationKeyCreatedInContextIgnored`, `domain.ChangeTypeCreatedInContext` (Task 1).

**Produces:** `SetCreatedInContextIgnored(ctx context.Context, objectIds []string, ignored bool) error` on `detailservice.Service`.

- [ ] **Step 1: Write the failing test**

> **Why this asserts the change type rather than `lastModifiedDate`:** the bump is applied inside the
> *real* `smartblock.Apply` (`pushChange` → `SetLastModified`, gated on
> `changeType == domain.ChangeTypeUserChange`). `smarttest.SmartTest` does not run that code, so an
> assertion that `lastModifiedDate` is unchanged would pass vacuously and prove nothing. The
> mechanism that actually prevents the bump is the change type on the applied state — so assert that,
> with a spy that captures the state handed to `Apply`.

Append to `core/block/detailservice/service_test.go`:

```go
// applySpy wraps a smarttest block to capture the change type of the state passed to Apply.
type applySpy struct {
	*smarttest.SmartTest
	lastChangeType domain.ChangeType
}

func (a *applySpy) Apply(s *state.State, flags ...smartblock.ApplyFlag) error {
	a.lastChangeType = s.GetChangeType()
	return a.SmartTest.Apply(s, flags...)
}

func TestSetCreatedInContextIgnored_SetsDetailWithNonUserChangeType(t *testing.T) {
	fx := newFixture(t)
	spy := &applySpy{SmartTest: smarttest.New("obj1")}
	fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
		return spy, nil
	})

	// when
	err := fx.SetCreatedInContextIgnored(context.Background(), []string{"obj1"}, true)

	// then: the flag is set...
	require.NoError(t, err)
	assert.True(t, spy.NewState().Details().GetBool(bundle.RelationKeyCreatedInContextIgnored))
	// ...via a non-user change type, which is what makes smartblock.Apply skip SetLastModified
	assert.Equal(t, domain.ChangeTypeCreatedInContext, spy.lastChangeType)
}

func TestSetCreatedInContextIgnored_Reversible(t *testing.T) {
	fx := newFixture(t)
	spy := &applySpy{SmartTest: smarttest.New("obj1")}
	fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
		return spy, nil
	})

	require.NoError(t, fx.SetCreatedInContextIgnored(context.Background(), []string{"obj1"}, true))
	require.NoError(t, fx.SetCreatedInContextIgnored(context.Background(), []string{"obj1"}, false))

	assert.False(t, spy.NewState().Details().GetBool(bundle.RelationKeyCreatedInContextIgnored))
}
```

Add the `state` import to `service_test.go`:
`"github.com/anyproto/anytype-heart/core/block/editor/state"`

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./core/block/detailservice/ -run TestSetCreatedInContextIgnored -v`
Expected: FAIL — compile error `fx.SetCreatedInContextIgnored undefined`.

- [ ] **Step 3: Add the method to the `Service` interface**

In `core/block/detailservice/service.go`, after `SetListIsArchivedNoGC`, add:

```go
	// SetCreatedInContextIgnored detaches (or re-attaches) an object's lifecycle from its creation context.
	// Written with ChangeTypeCreatedInContext so it syncs without bumping lastModifiedDate.
	SetCreatedInContextIgnored(ctx context.Context, objectIds []string, ignored bool) error
```

- [ ] **Step 4: Implement it**

Append to `core/block/detailservice/set_details.go`:

```go
// SetCreatedInContextIgnored excludes objects from cleanup suggestions and from automatic
// context-driven archival, by ignoring their createdInContext link. The detail is written directly on
// the state with a non-user change type, which skips the lastModifiedDate bump (SetLastModified is
// only called for domain.ChangeTypeUserChange) while still producing a real, syncing CRDT change.
func (s *service) SetCreatedInContextIgnored(ctx context.Context, objectIds []string, ignored bool) error {
	var (
		resultErr  error
		anySucceed bool
	)
	for _, objectId := range objectIds {
		err := cache.Do(s.objectGetter, objectId, func(sb smartblock.SmartBlock) error {
			st := sb.NewState()
			st.SetDetail(bundle.RelationKeyCreatedInContextIgnored, domain.Bool(ignored))
			st.SetChangeType(domain.ChangeTypeCreatedInContext)
			return sb.Apply(st)
		})
		if err != nil {
			log.Error("failed to set createdInContextIgnored", zap.String("objectId", objectId), zap.Error(err))
			resultErr = errors.Join(resultErr, fmt.Errorf("set createdInContextIgnored on %s: %w", objectId, err))
			continue
		}
		anySucceed = true
	}
	if anySucceed {
		return nil
	}
	return resultErr
}
```

- [ ] **Step 5: Run the test**

Run: `go test ./core/block/detailservice/ -run TestSetCreatedInContextIgnored -v`
Expected: PASS.

- [ ] **Step 6: Regenerate mocks**

Run: `make test-deps`
Expected: `mock_detailservice/mock_Service.go` gains `SetCreatedInContextIgnored`.

Verify: `grep -n "func (_m \*MockService) SetCreatedInContextIgnored" core/block/detailservice/mock_detailservice/mock_Service.go`

- [ ] **Step 7: Build + test**

Run: `go build ./... && go test ./core/block/detailservice/...`
Expected: build success; `ok`.

- [ ] **Step 8: Commit**

```bash
git add core/block/detailservice/ && git commit -m "GO-7323 Add detailservice.SetCreatedInContextIgnored with ChangeTypeCreatedInContext"
```

---

## Task 6: RPC handlers

**Files:**
- Modify: `core/object.go` (`ObjectCleanupSuggestions`)
- Modify: `core/details.go` (`ObjectCleanupSuggestionIgnore`)

**Consumes:** `objectgc.ListOrphans` + `OrphanItem`/`OrphanReason` (Task 2); `detailservice.SetCreatedInContextIgnored` (Task 5); the pb types (Task 4).

- [ ] **Step 1: Implement `ObjectCleanupSuggestions` in `core/object.go`**

Append to `core/object.go` (imports needed: `github.com/anyproto/anytype-heart/core/block/objectgc`, `github.com/anyproto/anytype-heart/core/domain`, `github.com/gogo/protobuf/types`, `github.com/samber/lo`, `github.com/anyproto/anytype-heart/util/slice`):

```go
// defaultCleanupKeys is returned when the caller passes no keys.
var defaultCleanupKeys = []domain.RelationKey{
	bundle.RelationKeyName,
	bundle.RelationKeyType,
	bundle.RelationKeyCreator,
	bundle.RelationKeyCreatedDate,
	bundle.RelationKeySnippet,
	bundle.RelationKeyIconEmoji,
	bundle.RelationKeyIconImage,
	bundle.RelationKeyResolvedLayout,
}

// forcedCleanupKeys are always returned: the client cannot render the forest without them.
var forcedCleanupKeys = []domain.RelationKey{
	bundle.RelationKeyId,
	bundle.RelationKeyCreatedInContext,
	bundle.RelationKeyResolvedLayout,
}

func orphanReasonToProto(r objectgc.OrphanReason) pb.RpcObjectCleanupSuggestionsResponseItemReason {
	switch r {
	case objectgc.OrphanReasonContextArchived:
		return pb.RpcObjectCleanupSuggestionsResponseItem_contextArchived
	case objectgc.OrphanReasonContextDeleted:
		return pb.RpcObjectCleanupSuggestionsResponseItem_contextDeleted
	case objectgc.OrphanReasonContextUnlinked:
		return pb.RpcObjectCleanupSuggestionsResponseItem_contextUnlinked
	default:
		return pb.RpcObjectCleanupSuggestionsResponseItem_none
	}
}

// cleanupKeys resolves the relation keys to project: the caller's keys (or the default set when the
// caller passes none), always unioned with the forced keys.
func cleanupKeys(reqKeys []string) []domain.RelationKey {
	keys := defaultCleanupKeys
	if len(reqKeys) > 0 {
		keys = slice.StringsInto[domain.RelationKey](reqKeys)
	}
	keys = append(append([]domain.RelationKey{}, keys...), forcedCleanupKeys...)
	return lo.Uniq(keys)
}

func (mw *Middleware) ObjectCleanupSuggestions(cctx context.Context, req *pb.RpcObjectCleanupSuggestionsRequest) *pb.RpcObjectCleanupSuggestionsResponse {
	response := func(code pb.RpcObjectCleanupSuggestionsResponseErrorCode, items []*pb.RpcObjectCleanupSuggestionsResponseItem, err error) *pb.RpcObjectCleanupSuggestionsResponse {
		m := &pb.RpcObjectCleanupSuggestionsResponse{Error: &pb.RpcObjectCleanupSuggestionsResponseError{Code: code}, Items: items}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}

	orphans, err := mustService[objectgc.ObjectGC](mw).ListOrphans(req.SpaceId)
	if err != nil {
		return response(pb.RpcObjectCleanupSuggestionsResponseError_UNKNOWN_ERROR, nil, err)
	}

	keys := cleanupKeys(req.Keys)

	items := lo.Map(orphans, func(it objectgc.OrphanItem, _ int) *pb.RpcObjectCleanupSuggestionsResponseItem {
		return &pb.RpcObjectCleanupSuggestionsResponseItem{
			Details: it.Details.CopyOnlyKeys(keys...).ToProto(),
			IsRoot:  it.IsRoot,
			Reason:  orphanReasonToProto(it.Reason),
		}
	})
	return response(pb.RpcObjectCleanupSuggestionsResponseError_NULL, items, nil)
}
```

> If Task 4 Step 4 reported different generated enum constant names, substitute them here.

- [ ] **Step 2: Implement `ObjectCleanupSuggestionIgnore` in `core/details.go`**

Append to `core/details.go`:

```go
func (mw *Middleware) ObjectCleanupSuggestionIgnore(cctx context.Context, req *pb.RpcObjectCleanupSuggestionIgnoreRequest) *pb.RpcObjectCleanupSuggestionIgnoreResponse {
	response := func(code pb.RpcObjectCleanupSuggestionIgnoreResponseErrorCode, err error) *pb.RpcObjectCleanupSuggestionIgnoreResponse {
		m := &pb.RpcObjectCleanupSuggestionIgnoreResponse{Error: &pb.RpcObjectCleanupSuggestionIgnoreResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	err := mustService[detailservice.Service](mw).SetCreatedInContextIgnored(cctx, req.ObjectIds, req.Ignored)
	if err != nil {
		return response(pb.RpcObjectCleanupSuggestionIgnoreResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectCleanupSuggestionIgnoreResponseError_NULL, nil)
}
```

- [ ] **Step 3: Test the key projection (spec requires it)**

Create `core/object_cleanup_test.go`:

```go
package core

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func TestCleanupKeys_EmptyRequest_UsesDefaultsPlusForced(t *testing.T) {
	keys := cleanupKeys(nil)

	assert.Subset(t, keys, defaultCleanupKeys)
	assert.Subset(t, keys, forcedCleanupKeys)
}

func TestCleanupKeys_CallerKeys_ForcedAlwaysIncluded(t *testing.T) {
	keys := cleanupKeys([]string{"name"})

	assert.Contains(t, keys, domain.RelationKey("name"))
	assert.Contains(t, keys, bundle.RelationKeyId)
	assert.Contains(t, keys, bundle.RelationKeyCreatedInContext)
	assert.Contains(t, keys, bundle.RelationKeyResolvedLayout)
	// caller keys do not pull in the default set
	assert.NotContains(t, keys, bundle.RelationKeySnippet)
}

func TestCleanupKeys_Deduplicates(t *testing.T) {
	// resolvedLayout is both a caller key and a forced key
	keys := cleanupKeys([]string{"resolvedLayout", "resolvedLayout"})

	count := 0
	for _, k := range keys {
		if k == bundle.RelationKeyResolvedLayout {
			count++
		}
	}
	assert.Equal(t, 1, count)
}
```

Run: `go test ./core/ -run TestOrphanKeys -v`
Expected: PASS (3 tests).

- [ ] **Step 4: Build**

Run: `go build ./...`
Expected: success. (A missing method on `ClientCommands` here means Task 4 Step 2 was skipped.)

- [ ] **Step 5: Verify the handlers are wired to the gRPC service**

Run: `grep -rn "ObjectCleanupSuggestions\|ObjectCleanupSuggestionIgnore" pb/service/service.pb.go | head -4`
Expected: the generated server interface lists both methods.

- [ ] **Step 6: Commit**

```bash
git add core/object.go core/details.go core/object_cleanup_test.go
git commit -m "GO-7323 Add ObjectCleanupSuggestions and ObjectCleanupSuggestionIgnore RPC handlers"
```

---

## Task 7: Full verification

- [ ] **Step 1: Build and vet**

Run: `go build ./... && go vet ./core/block/objectgc/... ./core/block/detailservice/...`
Expected: success (pre-existing `relations_test.go` unkeyed-field warnings are unrelated).

- [ ] **Step 2: Run every touched package**

Run:
```bash
go test ./core/block/objectgc/... ./core/block/detailservice/... ./core/block/editor/smartblock/... ./core/block/chats/... ./core/files/fileobject/... ./core/block/object/objectcreator/...
```
Expected: all `ok`. `core/block/chats` may flake on `TestSubscribeToMessagePreviews` under parallel load (a known `SendHeader` stream race) — re-run that package alone to confirm.

- [ ] **Step 3: Confirm all core test packages still compile**

Run: `go test -count=1 -run='^$' ./core/...`
Expected: exit 0, no `.go:` errors. (This catches any other stub implementing `ObjectGC` or `detailservice.Service`.)

- [ ] **Step 4: gofmt**

Run: `gofmt -l core/block/objectgc/ core/block/detailservice/ core/object.go core/details.go core/domain/types.go`
Expected: no output. If any file is listed, run `gofmt -w` on it.

- [ ] **Step 5: Final commit**

```bash
git add -A -- core/ pkg/ pb/ docs/
git commit -m "GO-7323 Tidy after space orphan list" --allow-empty
```

---

## Follow-up (out of this plan's scope)

The spec's **e2e scenarios** live in the sibling repo `anytype-suite` (gRPC/middleware-level, vitest),
on branch `go-7323-cascade-deletion-orphan-events` under `src/scenarios/cascade-deletion/`. After this
lands, add a scenario that archives a parent, calls `ObjectCleanupSuggestions`, ignores the child via
`ObjectCleanupSuggestionIgnore`, and re-requests to assert it disappears. Rebuild the suite's server with
`npm run server:build-local` (builds from the local checkout — `build-from-git.sh` clones from GitHub
and cannot see unpushed work) and `npm run server:use -- local --update-protos`.

## Self-Review notes (for the implementer)

- `Query` injects `isArchived != true` / `isDeleted != true`; `QueryRaw` does not. The candidate query relies on the former; `queryParentStates` relies on the latter. Do not swap them.
- The sync-gap guard runs **before** the active-backlink batch, so a dropped candidate is correctly treated as an "active object outside the candidate set" when its children are evaluated.
- `evictToFixedPoint` seeds on *active backlinks outside the candidate set*, then propagates: an evicted candidate is itself active and outside `S`.
- Reason for a cycle-elected root: its parent is a candidate (a cycle peer), so `reasonFor` returns `OrphanReasonContextUnlinked`. That is intentional — nothing outside the component links it.
- Do not forget `fileGCDummy` in `core/block/chats/service_test.go` (runtime-resolved, not compile-checked).
