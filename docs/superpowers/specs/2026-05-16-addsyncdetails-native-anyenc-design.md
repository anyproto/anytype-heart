# Rework `addSyncDetails` to a native anyenc modifier

GO-7291

## Problem

`indexer.addSyncDetails` (core/indexer/reindex.go) backfills the three local
sync relations — `SyncStatus`, `SyncDate`, `SyncError` — on every object that
is missing any of them. It runs on every space reindex/startup.

Today the per-object write goes through `store.ModifyObjectDetailsCtx` with
`helper.InjectsSyncDetails` as the proc. Per object that path does:

1. `domain.NewDetailsFromAnyEnc(val)` — full deserialize of the stored doc.
2. `helper.InjectsSyncDetails` — sets only the absent sync relations.
3. `newDetails.ToAnyEnc(arena)` — full reserialize.
4. `pbtypes.DiffAnyEnc(val, jsonVal)` — full diff to decide whether to write.

`ListIdsWithoutSyncDetails` returns the same candidate set on every startup.
For every candidate the full deserialize/reserialize/diff runs even though the
modification touches at most three integer fields, and in the common case
nothing is missing and the write is discarded by the diff. The round-trip is
pure overhead on the cold-start path (GO-7291: this op already runs under a
non-cancelable context inside chunked shared write transactions specifically to
keep startup cheap and deadlock-free).

## Goal

Replace the proto round-trip with a native `any-store` modifier that operates
directly on the raw `*anyenc.Value`:

- Check presence of each sync relation on the raw doc (no deserialize).
- Set only the absent ones via `anyenc`.
- If none are absent, return `modified = false` — no write, no diff, no
  subscription notify.

Keep the existing batch / `WriteTx` / non-cancelable-context / per-batch
`FilterNotExists` scaffolding in `reindex.go` exactly as it is (it is the
GO-7291 deadlock fix and is documented in place). Only the inner per-object
modifier call changes.

## Design

### New store method

Add to `pkg/lib/localstore/objectstore/spaceindex/sync.go`, next to
`ListIdsWithoutSyncDetails` and `syncDetailRelations` (which already declare
exactly these three keys):

```go
func (s *dsObjectStore) ModifySyncDetailsCtx(
    ctx context.Context,
    id string,
    status domain.ObjectSyncStatus,
    syncError domain.SyncError,
) error
```

Behaviour:

- Build a native `query.ModifyFunc(func(arena *anyenc.Arena, val *anyenc.Value)
  (*anyenc.Value, bool, error))`.
- For each key in `syncDetailRelations`
  (`SyncStatus`, `SyncDate`, `SyncError`):
  - presence test: `val.Get(string(key)) == nil`. This mirrors the
    `Not{Exists}` filter used by `ListIdsWithoutSyncDetails`, so it is
    value-agnostic — `ObjectSyncStatusSynced` and `SyncErrorNull` are both the
    zero enum value and must still count as "present".
  - if absent, set it via `domain.Int64(v).ToAnyEnc(arena)` for canonical
    encoding consistent with the rest of the system:
    - `SyncStatus` → `int64(status)`
    - `SyncError`  → `int64(syncError)`
    - `SyncDate`   → `time.Now().Unix()`
  - each key is handled independently, so an object missing only `SyncDate` is
    still written. This keeps the write set consistent with
    `ListIdsWithoutSyncDetails` (which selects on any-of-three absent) and
    avoids an object being re-listed every startup but never written.
- If no key was absent → return `(nil, false, nil)`: no write, no notify. This
  is the steady-state path: a cheap existence check with zero deserialization.
- If at least one key was set → build `domain.Details` from the mutated `val`
  via `domain.NewDetailsFromAnyEnc(val)` once and call
  `s.sendUpdatesToSubscriptions(id, det)` (preserving current subscription
  behaviour; only on the rare first-launch write path), then return
  `(val, true, nil)`.
- Apply with `s.objects.UpdateId(ctx, id, modifier)` (not upsert — these
  objects already exist; matches the `upsert == false` branch of
  `ModifyObjectDetailsCtx`). Swallow `anystore.ErrDocNotFound` (return nil).
  Wrap other errors with `fmt.Errorf("modify sync details: %w", err)`.

The `ctx` carries the tx (via `WriteTx`) and non-cancelability exactly as it
does for `ModifyObjectDetailsCtx` today — no change to tx handling semantics.

Subscription notify stays per-id (decision: no batch notify API exists; adding
one is out of scope).

### `reindex.go` change

In `addSyncDetails`, replace the inner loop body:

```go
modErr := store.ModifyObjectDetailsCtx(txn.Context(), id, func(details *domain.Details) (*domain.Details, bool, error) {
    return helper.InjectsSyncDetails(details, syncStatus, syncError), true, nil
}, true)
```

with:

```go
modErr := store.ModifySyncDetailsCtx(txn.Context(), id, syncStatus, syncError)
```

Everything else in `addSyncDetails` (batch sizing, `FilterNotExists` per batch,
`WriteTx`/`Commit`, non-cancelable context, logging) is unchanged. Update the
`addSyncDetails` doc comment to reference the new method instead of
`helper.InjectsSyncDetails`.

### Cleanup

`helper.InjectsSyncDetails` has exactly one production caller
(`reindex.go:529`); after this change it is dead. Remove it. Keep
`helper.SyncRelationsSmartblockTypes` (used elsewhere). Update the
`syncDetailRelations` doc comment in `sync.go` (currently "Keep in sync with
[InjectsSyncDetails]") to point at `ModifySyncDetailsCtx`.

## Testing

Existing `core/indexer/reindex_test.go` cases are the behavioural contract and
must stay green unchanged:

- first run writes sync details for objects missing them
- local-only mode writes error status
- repeat run is a no-op when sync details already present (also asserts
  `SyncDate` is not bumped — covered by the presence test)
- missing relation is added without overwriting existing ones
- re-filters every batch so an id cached mid-run is skipped

Add a direct unit test for `ModifySyncDetailsCtx` in
`pkg/lib/localstore/objectstore/spaceindex` (e.g. `sync_test.go`):

- all three present → returns nil, doc unchanged, no subscription emit
- partial missing (e.g. only `SyncError` absent) → only the absent key added,
  existing values preserved, subscription emit fires once
- all three missing → all set (status/error from args, `SyncDate` non-zero),
  subscription emit fires once
- non-existent id → returns nil (ErrDocNotFound swallowed)

Use the existing `StoreFixture` / `AddObjects` patterns per CLAUDE.md.

## Out of scope

- Batch subscription-notify API.
- Any change to `addSyncDetails` batching / tx / `FilterNotExists` ordering.
- Changes to `ListIdsWithoutSyncDetails` or `nonSyncableLayouts`.
