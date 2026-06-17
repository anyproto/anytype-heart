# GO-4284 — Remove deprecated object-level relationLinks

## Why

Object-level `relationLinks` (the `model.RelationLink{Key,Format}` list attached to an
object, distinct from dataview relations) are **no longer read by any client or by the
local indexer**. They are kept alive only by auto-add writers.

This causes a real bug: when one device sets a detail whose relation link is missing
(e.g. `discussionId`), every *other* online participant's device independently runs the
link reconciliation, emits a `RelationAdd` change **under its own identity**, and pushes
it. Result: a single local edit produces phantom history entries authored by every other
member of the space (observed in version history: 6× `relationAdd discussionId` at the
same second from 4 different identities).

The objectstore already derives relation keys from **details**, not links
(`FetchRelationByLinks` has zero live callers; `state.HasRelation`/`iterateKeys` read
details keys). So the links are dead weight.

## Scope

**IN scope:** object-level `state.relationLinks` and everything that auto-adds / reads /
serializes it.

**OUT of scope:** dataview relation links (`model.BlockContentDataview.RelationLinks`,
`core/block/simple/dataview`, `core/block/editor/dataview`) — separate, user-driven
feature. `bundle.HasRelation`, schema `Type.HasRelation`, `ObjectPath.HasRelation` are
unrelated despite the name.

## Backward compatibility

- Keep the protobuf types (`pb.ChangeRelationAdd/Remove`,
  `model.SmartBlockSnapshotBase.RelationLinks`, `EventObjectRelationsAmend/Remove`) so
  existing trees still parse.
- Keep `changeRelationAdd` / `changeRelationRemove` apply handlers as **no-ops** (parse
  but don't store), so old changes load without error.
- Stop *populating* snapshot relation links on write.

---

## Inventory

### A. Core state machinery — `core/block/editor/state/`
- `state.go:124` — field `relationLinks pbtypes.RelationLinks`
- `state.go:174-202` — `filterRelations` (filters relationLinks)
- `state.go:703-735` — Apply diff: emits `ObjectRelationsAmend` / `ObjectRelationsRemove`
  events + undo `RelationLinks` (**write path into tree**)
- `state.go:793-794, 852-853` — parent propagation of relationLinks
- `state.go:960` — StringDebug loop over relationLinks
- `state.go:989-993` — `SetDetailAndBundledRelation` (calls `AddBundledRelationLinks`)
- `state.go:1326` — `Copy()` copies relationLinks
- `state.go:1692-1699` — `AddRelationLinks`
- `state.go:1703-1726` — `PickRelationLinks`, `pickRelationLinks`, `getRelationLinks`
- `state.go:1798-1811` — `AddBundledRelationLinks`, `AddBundledRelationLinks` impl
- `details.go:164-...` — `RemoveRelation` (filters relationLinks; keep detail/featured removal)
- `change.go:85` — snapshot read into `relationLinks`
- `change.go:200-205, 280-291` — `changeRelationAdd` / `changeRelationRemove` apply handlers (→ no-op)
- `change.go:471, 551, 583-606, 626-644` — change generation: `RelationAdd`/`RelationRemove`
  ops + `filterLocalAndDerivedRelations[ByKey]` (**write path into tree**)

### B. Snapshot serialization / derivation
- `core/block/source/sourceimpl/source.go:447-449` — `RelationLinks: State.PickRelationLinks()` (stop populating) ✅ DONE
- `core/block/import/common/types.go:86,102` — propagate `sn.RelationLinks` (kept: backward-compat read of old imports)

### B2. SEPARATE, STILL-LIVE surface — `model.ObjectType.RelationLinks` (NOT removed)
This is the **object-type's recommended relations** exposed as links, a different concept
from object-level `state.relationLinks`. It is actively consumed, so it was intentionally
left in place. Removing it requires migrating these consumers to read `recommendedRelations`
details directly — a separate task:
- `pkg/lib/localstore/objectstore/spaceindex/object_type.go:54-58` — `getRelationLinksForRecommendedRelations`
- `core/relationutils/objecttype.go:21` — iterates `ot.RelationLinks` (consumer)
- `core/block/source/sourceimpl/bundledobjecttype.go:55` — iterates `ot.RelationLinks` (consumer)
- `pkg/lib/schema/{schema,type,exporter}.go`, `pkg/lib/schema/yaml/exporter.go` — schema export
- Follow-up: `core/block/undo/undo.go` `Action.RelationLinks` field is now always nil (dead, can be removed)

### C. Object-level WRITE callers
- **`SetDetailAndBundledRelation` — 122 callers across 32 files** (top: detailsinject.go 18,
  spaceview.go 14, history.go 8, filerequest.go 8, spaceinfo/* 16). Signature is identical
  to `SetDetail(key, value)` → mechanical rename.
- **`AddBundledRelationLinks` — 13 callers** (smartblock.go ×4, template.go ×2, participant.go,
  source.go, fileindex.go, templateimpl/impl.go, builtintemplate.go, +def)
- **`AddRelationLinks` — 10 callers** (basic/details.go ×2, basic.go, import ×3, bundledobjecttype.go,
  smartblock.go, smarttest, +def)
- **`AddRelationLinksToState` — interface method** (smartblock.go iface+impl, clipboard.go caller,
  smarttest) → remove from `SmartBlock` interface, regenerate mocks

### D. READ sites
- `source.go:449` `PickRelationLinks()` (snapshot build) — covered in B
- `RemoveRelation` reads links internally — covered in A
- `pkg/lib/localstore/objectstore/spaceindex/relations.go:95` `FetchRelationByLinks` — **dead, remove**
  (interface `store.go:110`, `invalid.go:194`)

### E. Backward-compat (KEEP, make no-op)
- `changeRelationAdd` / `changeRelationRemove` handlers — parse, don't store
- pb types + `model.SmartBlockSnapshotBase.RelationLinks` — keep generated

---

## Execution order (each commit `GO-4284 …`)

1. **Stop writing links into the tree** (fixes the bug): remove `RelationAdd`/`RelationRemove`
   change generation (`change.go`), the diff emit + undo (`state.go:703-735`), and stop
   populating snapshot links (`source.go:449`). After this, no new link data enters any tree.
2. **Remove writers**: `SetDetailAndBundledRelation` → `SetDetail` (mass rename + delete method);
   remove `AddBundledRelationLinks` / `AddRelationLinks` / `AddRelationLinksToState` + callers.
3. **Remove state field & accessors**: `relationLinks` field, `PickRelationLinks`,
   `pickRelationLinks`, `getRelationLinks`, `filterRelations`, parent prop, Copy, StringDebug;
   strip link logic from `RemoveRelation`.
4. **Remove derivation/serialization**: `object_type.go`, schema exporters, import propagation,
   dead `FetchRelationByLinks`.
5. **Make apply handlers no-op**, keep pb for compat. Regenerate mocks. `make test`.
