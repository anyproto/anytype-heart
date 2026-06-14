# Cascade Deletion → User-Confirmed Object Archival

**Issue:** GO-7323 (follow-up to GO-7152 / GO-7264)
**Date:** 2026-06-14
**Status:** Design approved

## Background

The "Cascade Deletion" feature (a.k.a. "Created in Context" / "Context-to-C") records,
for every object, the context in which it was created via the `createdInContext` relation
(plus `createdInContextRef` for the inner block/relation/message locator). When the
context object is archived, deleted, or has the link to a created object removed, the
created objects were **implicitly archived** alongside it (cascade).

`GO-7152` built the orphan garbage collector (`core/block/objectgc`) and the
`ObjectAutoArchive` / `ObjectAutoRestore` events. `GO-7264` was a hotfix that **scoped the
cascade to files only** in response to user feedback — it added three guards marked
`// temporarily disable non-file objects` / `// todo: remove when we move to orphan events`,
so today non-file objects are silently ignored. Its commit message states:
*"Follow-up will replace this with an explicit user-confirmation flow."* This spec is that
follow-up.

## Goal

Stop implicitly archiving things the user did not explicitly choose. Instead:

- **Direct-child files** of the acted-on object continue to be archived automatically
  (cheap, expected, low surprise).
- **Everything else orphaned** in the subtree (all nested objects at every level, plus
  files at level ≥ 2) is surfaced to the client as a **new event** carrying a list of ids.
  The client renders a confirmation popup; the user decides what to archive.

This avoids the user feedback problem (objects vanishing without consent) while keeping the
existing, already-shipped file behavior.

## Behavior model

### Terminology

- **Operation root / target** — the object being archived/deleted, or the target of a
  removed link.
- **Level 1** — objects/files whose `createdInContext` points directly at the operation
  root/target.
- **Orphaned** — has no active backlinks outside the set being collected in this pass
  (existing orphan criteria, reused unchanged).

### Archive A / Delete A / Remove link A→X  (`skipCascade = false`)

Walk the `createdInContext` tree rooted at the operation target. Categorize each orphaned
node:

| Node | Action |
|------|--------|
| File at **level 1** | **Auto-archive** + existing `ObjectAutoArchive` event |
| Object at **any level** | Add id to **`OrphansDetected` event**; do NOT archive |
| File at **level ≥ 2** | Add id to **`OrphansDetected` event**; do NOT archive |

The single special case is *"a direct-child file is auto-archived."* Every other orphan
(deep files + all objects) is surfaced for confirmation. The BFS descends through **every
object** to enumerate the full subtree; files are never descended into (they have no
`createdInContext` children).

The `OrphansDetected` event is a flat list of ids — it intentionally mixes deep files and
objects, because both require the same explicit user confirmation. The client resolves each
id's layout itself for popup presentation.

### Confirmation call  (`skipCascade = true`)

When the user confirms a subset in the popup, the client re-issues
`ObjectSetIsArchived` / `ObjectListSetIsArchived` for the chosen ids with
`skipCascade = true`. This is a **pure archive** of exactly those ids:

- no level-1 file auto-archival,
- no subtree walk,
- no `OrphansDetected` event.

This is what breaks the otherwise-cyclic re-prompt loop (archiving a confirmed object would
otherwise recompute its orphans and emit another event → another popup → …).

Because the *first* event already enumerated the **entire** transitive subtree, the user has
already seen everything; the confirmation call needs only to apply their choices.

### Unarchive A / Undo link removal

Auto-restore **level-1 files only** (symmetric with archive; restores exactly what was
auto-archived). No event is emitted. Objects and deep files the user archived earlier are
restored **manually by the user from the bin**, where the tree structure is visible.

## API changes

### New event — `pb/protos/events.proto`

Add inside `message Event { message Object { … } }`, alongside `AutoArchive` / `AutoRestore`:

```proto
message OrphansDetected {
  repeated string objectIds = 1;  // orphan ids (objects any level + files level >= 2) created within contextId
  string contextId = 2;           // the object that was archived / deleted / had a link removed
  Trigger trigger = 3;
  enum Trigger {
    archive = 0;
    delete = 1;
    linkRemoval = 2;
  }
}
```

Register it in the `Event.Message` oneof at the next free field number (`146`):

```proto
Object.OrphansDetected objectOrphansDetected = 146;
```

> Event name `OrphansDetected` and field name are provisional — adjust to the client team's
> preferred convention before/with proto generation.

### New request field — `pb/protos/commands.proto`

Add to **both** request messages (field `3` is free in each):

```proto
// Rpc.Object.SetIsArchived.Request
bool skipCascade = 3;

// Rpc.Object.ListSetIsArchived.Request
bool skipCascade = 3;
```

## Implementation plan (by layer)

### 1. GC layer — `core/block/objectgc/objectgc.go`

- Introduce a result type:
  ```go
  type OrphanCandidates struct {
      Files      []string // level-1 orphan files → auto-archived + ObjectAutoArchive
      Candidates []string // all other orphans (objects any level + files >= level 2) → OrphansDetected
  }
  ```
- Remove the three `// temporarily disable non-file objects` guards (GO-7264) so the BFS
  descends the full tree again.
- Track depth in the BFS (root = level 0, its direct children = level 1, …). Categorize:
  `level == 1 && layout ∈ FileLayouts` → `Files`; every other orphaned node → `Candidates`.
  Continue to descend through objects only.
- `CheckObjectsOnObjectArchived(spaceId, objectId string, isArchived bool) (OrphanCandidates, error)`
  — archive direction returns both buckets; the orphan backlink/pending-set logic is reused
  unchanged (the pending set is the union of `Files` + `Candidates`, i.e. "what would be gone
  if the whole subtree were archived").
- `ArchiveOrphansOnLinksRemoval(...)` — keep archiving level-1 orphan files internally (as
  today) and return them as `Files`; collect the rest of the subtree's orphans into
  `Candidates`. (Descent through orphaned target objects so the user sees the full subtree.)
- Restore path (`restoreObjectsOnUnarchive` / `collectOrphanedForRestore` /
  `RestoreOrphansOnLinksAdded`) — scope to **level-1 files only** (add file-layout +
  level guard); never restore objects implicitly.

### 2. Detail service — `core/block/detailservice/set_details.go` + `service.go`

- Thread `skipCascade bool` through:
  - interface `SetIsArchived(sctx, ctx, objectId, isArchived, skipCascade)`
  - interface `SetListIsArchived(sctx, ctx, objectIds, isArchived, skipCascade)`
  - `setIsArchivedForObjects(..., skipCascade)`
- If `skipCascade`: skip `triggerGCOnArchive` entirely — archive only the explicitly
  requested ids, emit no GC events.
- Else:
  - `triggerGCOnArchive` returns aggregated `Files` plus per-context `Candidates`
    (so each `OrphansDetected` event carries the correct `contextId`).
  - `Files` are merged into the archive batch and reported via `ObjectAutoArchive` (unchanged).
  - For each originating object with non-empty `Candidates`, append one `OrphansDetected`
    event (`trigger = archive`) to `sctx`.
- Extend `FilterExplicitIds` to also strip explicitly-requested ids from `OrphansDetected`
  events (so an id that is both explicit and a candidate is not double-reported).

### 3. Delete path — `core/block/delete.go`

- `DeleteObjectByFullID` already calls `CheckObjectsOnObjectArchived(..., true)` and archives
  the result when the client skipped archiving. Update for the new return type:
  - archive `Files`,
  - emit an `OrphansDetected` event (`trigger = delete`, `contextId = id.ObjectID`) for
    `Candidates`. No session context is available here, so **broadcast** via
    `s.eventSender.Broadcast(event.NewEventSingleMessage(spaceId, …))`.

### 4. Link-removal / restore paths — `core/block/editor/smartblock/smartblock.go`

- `performGCOnLinksRemoval`: `ArchiveOrphansOnLinksRemoval` now returns
  `OrphanCandidates` — keep `ObjectAutoArchive` for `Files`, append an `OrphansDetected`
  event (`trigger = linkRemoval`) for `Candidates` on `sctx`.
- `restoreObjectsOnLinksAdded`: unchanged behavior (file-only restore) once the GC restore
  path is file-scoped.

### 5. RPC handlers — `core/details.go`

- `ObjectSetIsArchived`: pass `req.SkipCascade` into `SetIsArchived`.
- `ObjectListSetIsArchived`: pass `req.SkipCascade` into `SetListIsArchived`.

### 6. Codegen

- `make protos` — regenerate `pb/events.pb.go` and `pb/commands.pb.go`.
- `make test-deps` — regenerate mocks affected by the changed `ObjectGC` return type and the
  changed `detailservice.Service` signatures (`mock_detailservice`, objectgc mocks, etc.).

## Testing

- **Flip the GO-7264 hotfix assertions** in `core/block/objectgc/objectgc_test.go`: non-file
  objects must now appear in `Candidates` (not `Empty`); level-1 files in `Files`.
- New unit tests for the GC categorization:
  - level-1 file → `Files`; level-1 object → `Candidates`.
  - level-2 file → `Candidates`; level-2 object → `Candidates`.
  - deep tree: full subtree of objects enumerated into `Candidates`, exactly the level-1
    files in `Files`.
  - orphan criteria still respected (object/file with an external active backlink is excluded
    from both buckets).
- `skipCascade = true` short-circuits: no GC call, no events, only the explicit ids archived
  (detailservice test).
- Each trigger emits `OrphansDetected` with the correct `trigger` and `contextId`
  (archive via sctx, delete via broadcast, link-removal via sctx).
- `FilterExplicitIds` strips explicit ids from `OrphansDetected`.
- Restore direction restores level-1 files only; objects are not auto-restored.

## Out of scope / non-goals

- Client-side popup UI (separate client work; this spec defines the contract only).
- Changing the orphan-detection (backlink) algorithm itself — it is reused unchanged.
- Transitive file auto-archival (explicitly rejected as confusing).
- A "restore candidates" event for the unarchive direction (rejected; manual restore from bin).

## Open items (low stakes, resolve during implementation)

- Final names: event `OrphansDetected` / field `objectOrphansDetected` / request field
  `skipCascade` — confirm against client conventions.
- Exact oneof field number (`146` assumed free).
