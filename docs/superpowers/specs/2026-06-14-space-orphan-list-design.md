# Cleanup Suggestions (space-wide orphan list) & Ignore

> **Vocabulary boundary (deliberate).** The **protocol / client** says *cleanup suggestion*. **Internal
> Go** keeps *orphan* — the precise term for a node with no inbound references (`objectgc`,
> `OrphanCandidates`, `ListOrphans`, `OrphanItem`, `OrphanReason`). This spec uses "orphan" when
> describing the algorithm and "cleanup suggestion" when describing the API.

**Issue:** GO-7323 (second phase of the same issue — extends the user-confirmed object archival work)
**Date:** 2026-06-14
**Status:** Design approved

## Background

GO-7323 replaced implicit cascade archival with an **`CleanupSuggestion` event**: when a user
archives/deletes an object or removes a link, the backend reports the orphaned objects so the client
can prompt for confirmation. That event is **fire-and-forget** and **rooted at the object the user
just acted on**.

Two gaps remain:

1. If the user dismisses the popup (or the orphaning happened before this feature shipped), those
   orphans are never surfaced again.
2. There is no way to answer "what in this space is now unreachable and could be cleaned up?"

This spec adds a **queryable, space-wide orphan list** so the client can present a *cleanup
recommendation*, plus a way for the user to **permanently ignore** an object so it stops being
recommended.

## Goal

- An on-demand RPC that returns every orphan in a space, as a **forest** (roots + their full
  transitive subtrees, objects **and** files), with caller-selected relation keys.
- A way to **ignore** an object (and, for free, its subtree) so it is excluded from both the orphan
  list and future cascade prompts.
- Reuse the existing orphan semantics — exactly one definition of "orphan" in the codebase.

Non-goals: a live-updating subscription, and pagination. See *Out of scope*.

## Orphan definition

Let the **candidate set** `C` be every object `o` in the space where all hold:

- `o` is **active** — not archived, not deleted (the store's implicit query filter).
- `o.createdInContext` is non-empty.
- `o.createdInContextRef` is non-empty — the existing "collections have empty ref" gate.
- `o.resolvedLayout ∈ domain.GCEligibleLayouts` (user content + files; excludes system layouts).
- `o.createdInContextIgnored != true` (new relation, below).
- **`o.createdInContext` is present in the store** — sync-gap guard, see *Safety*.

Define `activeBacklinks(o) = { b ∈ o.backlinks : b ≠ o.id and b is active }`.

The **removable set `S`** is the **greatest subset of `C`** such that

> for every `o ∈ S`: `activeBacklinks(o) ⊆ S`

i.e. every member's active backlinks fall *inside* the set. Anything reachable from an active object
outside `S` is excluded, and that exclusion cascades.

**Roots** = `{ o ∈ S : o.createdInContext ∉ S }`. These are the tops of the orphan trees.

### Why this is the right definition

It is the same eviction rule the GO-7323 cascade BFS already uses, applied space-wide instead of
rooted at one object. Worked examples:

| Situation | Outcome |
|---|---|
| `B` created in `A`; `A` archived | `A` is inactive → `activeBacklinks(B) = ∅` → **B is a root** |
| `X` created in active page `P`; `P` still links `X` | `activeBacklinks(X) = {P}`, `P ∉ C` (top-level) → **X evicted** (reachable) |
| `X` created in `P`; link removed; `P` still active | `P` is no longer a backlink → **X is a root** (link-removal orphan) |
| `P` created in archived `A`; `X` created in `P` | `P` root, `X` descendant (its only active backlink `P` is in `S`) |
| `X ∈ S` but linked from active top-level `T` | `T ∉ S` → **X evicted**, and anything whose only backlink was `X` is evicted too (cascade) |
| `A` created in `B`, `B` created in `A` (cycle), nothing else links them | both stay in `S`; no parent-outside-`S` → **no root** → pick a deterministic root |

It also naturally catches **detached islands** (mutually-linked groups nothing active points at).

## Algorithm

Lives in `core/block/objectgc` so orphan semantics stay in one place. **Three store queries total**,
independent of orphan count; the rest is in memory.

1. `backlinksWatcher.FlushUpdates()` — backlinks must be current (objectgc already does this before
   every GC query).
2. **Candidate query** — one `Query`/`QueryIterate` with filters
   `createdInContext NotEmpty`, `createdInContextRef NotEmpty`, `resolvedLayout IN GCEligibleLayouts`
   (implicit active filter applies). Materialize each candidate's `Details`.
   Apply `createdInContextIgnored != true` as an **in-memory predicate** on the loaded details rather than a
   store filter, to avoid depending on `NotEqual`-vs-missing-key filter semantics.
3. **Active-status batch** — `queryActiveIds(backlinkTargets \ C)`: a single `Id IN [...]` query
   using the implicit filter, so returned ids are exactly the active ones. Members of `C` are active
   by construction and need no lookup.
4. **Parent-state batch** — one `QueryRaw` (no implicit filters) over the distinct
   `createdInContext` ids, projecting `id, isArchived, isDeleted`. This yields, per parent:
   *absent* / *archived* / *deleted* / *active*. Parents that are **absent** fail the sync-gap guard
   and their candidates are dropped from `C`. Parent ids are few (distinct contexts), so this is
   cheap. It supplies both the guard and the per-root `reason` below.
5. **Fixed point** (worklist, `O(V+E)` — not the naive `O(n²)` restart loop):
   - Build a reverse index `backlinkTarget → candidates that list it`.
   - Seed: evict every `o ∈ C` having an active backlink outside `C`.
   - Propagate: when `o` is evicted it becomes "active and outside `S`", so every surviving candidate
     `x` with `o ∈ x.backlinks` is evicted in turn. Repeat until the worklist drains.
   - Survivors = `S`.
6. **Forest** — build `parent → children` over `S` via `createdInContext`; mark roots
   (`parent ∉ S`). For a cycle component with no root, pick the **lowest id** as root so the tree
   still renders deterministically.
7. **Reason** — for each root, derive it from the parent-state map (step 4).

### Cost

`createdInContext`, `backlinks`, `isArchived`, `isDeleted` are **not indexed**; the candidate query
uses the `resolvedLayout` index and filters the rest in memory. This is the same shape as
`ObjectTypeListConflictingRelations`, and strictly cheaper than `RelationListWithValue`, which
already full-scans **every** object in a space in production. ~10k objects is the codebase's working
assumption for a large space. Acceptable for an on-demand RPC.

## API

### 1. List RPC — `ObjectCleanupSuggestions`

Request:
```proto
message Request {
  string spaceId = 1;
  repeated string keys = 2; // relation keys to return; empty => default set
}
```

Response:
```proto
message Item {
  google.protobuf.Struct details = 1; // only the requested keys
  bool isRoot = 2;                    // server-computed forest root
  Reason reason = 3;                  // why it is orphaned; set on roots only

  enum Reason {
    none = 0;            // descendants — the reason belongs to their root
    contextArchived = 1; // createdInContext parent is in the Bin
    contextDeleted = 2;  // createdInContext parent was removed
    contextUnlinked = 3; // parent is still active, but the link to this object was removed
  }
}
message Response {
  Error error = 1;
  repeated Item items = 2;
}
```

**The three reasons are total and provable for roots.** A root's parent is:

- never in `S` (that is the definition of a root),
- never absent from the store (the sync-gap guard already dropped such candidates, and deletion
  *tombstones* the row — `DeleteObject` keeps `id` + `isDeleted=true` precisely "so we can
  distinguish links to deleted and not-yet-loaded objects"),
- never active-and-still-linking the child (that would make it an active backlink, and the child
  would have been evicted from `S`).

So the parent is exactly one of *archived*, *deleted*, or *active* — and *active* can only mean the
link was removed. The backend computes this authoritatively from the parent-state batch rather than
making the client infer it from an absent lookup.

`reason` is `none` for descendants: a descendant's `createdInContext` parent is itself an orphan
sitting in `S` (active, but not "unlinked"). That is what `isRoot` is for.

Returning details (rather than bare ids) is **free**: the scan already materializes every
candidate's `Details`, and `S ⊆ C`, so the response is pure projection from memory. It saves the
client a second request and saves the backend a second scan.

**Force-included keys** (always returned, regardless of `keys`) — the client cannot render the
result without them:

- `id` — identity
- `createdInContext` — the client nests the tree itself, on the same relation the Bin tree uses
- `resolvedLayout` — the client groups objects vs files (`resolvedLayout ∈ FileLayouts`)

Consequently there is **no bespoke `kind` or `contextId` field**: file-ness is `resolvedLayout`, and
the context is the `createdInContext` key. The only thing this response adds over a normal search
record is `isRoot`.

**Default `keys` when empty:** `name, type, creator, createdDate, snippet, iconEmoji, iconImage,
resolvedLayout` (plus the three forced keys).

Ordering is unspecified; the client sorts. `snippet` can be sizable — a client wanting only a
count/badge should pass a minimal `keys`.

### 2. Ignore RPC — `ObjectCleanupSuggestionIgnore`

```proto
message Request {
  repeated string objectIds = 1;
  bool ignored = 2;
}
```

Sets the `createdInContextIgnored` detail on each object. Implemented in `detailservice`, and it **must** set
`state.SetChangeType(domain.ChangeTypeCreatedInContext)` before `Apply`.

Why a dedicated RPC rather than the generic `ObjectListSetDetails`:

- `SetLastModified` is only called when `changeType == domain.ChangeTypeUserChange`
  (`smartblock.go`). The change type can only be set **server-side**, so a generic details write from
  the client would bump `lastModifiedDate` and shove every ignored object to the top of "recently
  modified".
- It avoids relying on per-relation `readonly` being unenforced on the gRPC details path (true today,
  but an implicit contract).

The write is still a real CRDT change, so **the ignore syncs across devices**. It is reversible
(`ignored = false`).

### 3. Actions — no new RPC

The client archives the selected ids with the existing
`ObjectListSetIsArchived(ids, isArchived: true, skipCascade: true)`, or deletes them with
`ObjectListDelete`.

> **Load-bearing:** because the confirm call uses `skipCascade=true`, an orphan root's direct-child
> files are **not** auto-archived. That is precisely why the returned subtree must include files —
> the client sends them explicitly.

## New schema

### Relation `createdInContextIgnored`

Add to `pkg/lib/bundle/relations.json`, mirroring `isHidden` (the exact analog: a hidden checkbox
relation stored in object details):

| field | value |
|---|---|
| `key` | `createdInContextIgnored` |
| `format` | `checkbox` |
| `source` | `details` (CRDT state → syncs) |
| `hidden` | `true` (not user-editable in the relations panel) |
| `readonly` | `false` (the backend legitimately writes it; no reliance on the readonly loophole) |
| `maxCount` | 1 |

Also register it in `pkg/lib/bundle/systemRelations.json` (as `isHidden` is). This matters because
the ignore RPC writes through `basic.SetDetails`, whose `validateDetailFormat` calls
`objectStore.FetchRelationByKey(key)` — the relation must be resolvable. *(`createdInContextRef` is
in `relations.json` but not `systemRelations.json`, so the minimal path may also work; a test that the
ignore RPC succeeds on a fresh space settles it.)*

Regenerate with `go generate ./pkg/lib/bundle/...` (`//go:generate go run ./generator` in
`init.go`), which rewrites `relation.gen.go`. This bumps the bundle checksum, so the indexer
reindexes **bundled relations** automatically. There is **no user-data migration**: an absent key
means "not ignored".

### `domain.ChangeTypeCreatedInContext`

New constant in `core/domain/types.go` (+ its `String()` case), mirroring
`ChangeTypeDescriptionToggle` — a user-initiated but non-content change that intentionally skips the
`lastModifiedDate` bump.

## GC integration

`createdInContextIgnored` means *"this object's lifecycle is detached from its creation context."* It is
honored in **both** surfaces:

- The new orphan list (step 2 of the algorithm).
- The **three existing GC candidate paths** in `objectgc.go` — `collectOrphanedObjects` Query 1 and
  Query 2, and `ArchiveOrphansOnLinksRemoval` — as an in-memory predicate on the already-loaded
  record details, exactly like the layout check.

Consequences: an ignored object never appears in an `CleanupSuggestion` popup, and an **ignored
level-1 file is not auto-archived** when its parent is archived. Both follow from the single meaning
above.

**Ignoring a root drops its whole subtree from the list for free**: the root leaves `C`, so its
children have an active backlink (the still-active root) outside `S` and are evicted — no extra
logic.

## Client flow

1. User opens the cleanup screen → `ObjectCleanupSuggestions{spaceId, keys}`.
2. Client nests items by `createdInContext` and renders the **Bin's existing tree view**, grouping
   objects and files by `resolvedLayout`. `isRoot` marks the tree tops, and `reason` explains each
   root ("its page is in the Bin" / "its page was deleted" / "the link to it was removed").
3. Checkboxes with parent→subtree cascade (the Bin tree already does this).
4. Per selection: **Move to Bin** (`ObjectListSetIsArchived(..., skipCascade: true)`),
   **Delete permanently** (`ObjectListDelete`), or **Ignore**
   (`ObjectCleanupSuggestionIgnore(ids, true)` → re-request the list).

The existing `CleanupSuggestion` event stays as the in-the-moment prompt; this list is the periodic
cleanup surface that also catches dismissed and pre-existing orphans.

## Safety & edge cases

- **Sync gap.** A candidate whose `createdInContext` parent id is absent from the store is skipped.
  This is safe because `DeleteObject` **tombstones** rather than purges — it keeps `id` and
  `isDeleted=true` (`spaceindex/delete.go`), explicitly "so we can distinguish links to deleted and
  not-yet-loaded objects". Therefore an absent parent means *never synced*, not *deleted*. Same
  conservatism as `isConfirmedInactive`.
  *(Note: `DeleteDetails` does purge, but it is a separate API not used by normal object deletion.)*
- **Cycles.** A `createdInContext` cycle yields a component with no parent-outside-`S`; pick the
  lowest id as root.
- **Self-references** are excluded from `activeBacklinks`.
- **Collection-created objects** (empty `createdInContextRef`) are excluded, as today.
- **System layouts** are excluded via `GCEligibleLayouts`.
- **Ignored + archived.** An ignored object that the user later archives behaves normally; ignore only
  governs orphan/cascade handling.

## Testing

**objectgc unit tests** (`StoreFixture`, mirroring the GO-7323 suite):

- archived-parent orphan → root, `reason = contextArchived`.
- deleted-parent orphan (tombstoned, `isDeleted=true`) → root, `reason = contextDeleted`.
- link-removal orphan (parent active, no backlink) → root, `reason = contextUnlinked`.
- descendants carry `reason = none`.
- reachable object (active backlink to a non-candidate) → excluded.
- cascade eviction: `X` evicted (external backlink) → `Y` whose only backlink is `X` → also evicted.
- descendant: `P` root, `X` under it, `X` not a root.
- cycle `A↔B` → both in `S`, deterministic root chosen.
- parent absent from store → candidate skipped (sync gap).
- empty `createdInContextRef` → excluded; system layout → excluded.
- files included, distinguishable by `resolvedLayout`.
- `createdInContextIgnored=true` → excluded; **ignoring a root removes its whole subtree**.

**RPC tests:** requested-keys projection; forced keys always present; default key set when `keys`
empty.

**Ignore RPC tests:** sets `createdInContextIgnored`; **`lastModifiedDate` unchanged** (assert before/after);
reversible with `ignored=false`.

**GC regression tests:** an ignored object does not appear in `CleanupSuggestion` candidates; an
ignored level-1 file is **not** auto-archived on parent archive.

**e2e (anytype-suite):** archive a parent → `ObjectCleanupSuggestions` lists the child; ignore it →
re-request returns empty; archive from the list with `skipCascade=true` succeeds.

## Out of scope

- **Live subscription.** Subscriptions can only filter on *stored* details, so a live orphan badge
  would require materializing an `isOrphan` derived relation (precedent: `backlinks`, maintained by a
  watcher) plus a reindex backfill. The on-demand RPC needs none of that. This design does not
  preclude that upgrade: the RPC contract would not change.
- **Pagination.** A ~10k-object space yields at most a few hundred orphans. Add a `limit` if it ever
  bites.
- Client UI implementation (separate client work; this spec defines the contract).

## Open items

- **Phase-1 event rename (part of this work).** `Object.OrphansDetected` → `Object.CleanupSuggestion`
  (oneof field `objectCleanupSuggestion`; field number 146 and the `Trigger` enum unchanged). Phase 1
  is pushed but unmerged, so the protocol has never shipped and the rename is free now. The e2e tests
  in `anytype-suite` (branch `go-7323-cascade-deletion-orphan-events`,
  `src/scenarios/cascade-deletion/`, 8 files) reference the old name and must be renamed on that
  branch too — its working tree is currently on a different branch, so do not switch it implicitly.
- `ChangeTypeCreatedInContext` is deliberately **generic**, not ignore-specific. A second consumer
  already exists: `core/migration/objectcontext.go` backfills `createdInContext` via
  `ModifyDetails` with the default *user* change type, so it currently bumps `lastModifiedDate` on
  every backfilled file. Converting it is a clean follow-up.
- `skipCascade` keeps its name — it skips the whole cascade (file auto-archive *and* the suggestion).
- Builds directly on the first phase of GO-7323 (`skipCascade`, the `createdInContextRef` gate) and
  lands on the same branch / PR (`go-7323-cascade-deletion-orphan-events`).
