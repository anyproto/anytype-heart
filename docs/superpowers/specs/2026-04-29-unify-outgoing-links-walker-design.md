# Unify outgoing-links walker

**Date:** 2026-04-29
**Issue:** GO-7237 — Objects in a collection are not backlinked to the collection (regression introduced in GO-6052).

## Background

Two functions currently walk a `*state.State` to enumerate outgoing object IDs:

1. **`objectlink.DependentObjectIDs`** (`core/block/object/objectlink/dependent_objects.go`) — flat `[]string`, configured by `objectlink.Flags`. Used by `injectLinksDetails`, `injectMentions`, `dependentSmartIds`, exporters, history, graph converters.

2. **`(*smartBlock).collectOutgoingLinks`** + `collectLinksFromRelations` (`core/block/editor/smartblock/smartblock.go:1454`) — `[]OutgoingLink` with `SourceBlockID`/`RelationKey` attribution. Introduced in GO-6052 (`d6a6c1453`) so that the indexer can write attributed links via `UpdateObjectLinksDetailed`, and the file-GC can ask "this file link came from which block?" Used only by `getDocInfo` → indexer.

The new walker re-implemented a subset of the old preset and silently dropped several rows. The indexer's `spaceindexer.go:209-228` now prefers `info.OutgoingLinks` whenever non-empty, so the dropped rows leak as missing/extra backlinks.

### Confirmed gaps in `collectOutgoingLinks` vs the canonical `injectLinksDetails` preset

Missing:
- Collection `StoreSlice` members (`s.GetStoreSlice(template.CollectionStoreKey)`) — the bug that started this work.
- Inline dataview embed targets (`Dataview.TargetObjectId`).
- Bookmark block targets (`Bookmark.TargetObjectId`).
- Object-type text marks (`BlockContentTextMark_Object`) — partially fixed on branch `GO-0000-backlinks-object-mark`, not yet merged.
- Status- and tag-format relation values (relation-option backlinks).
- Date-format relation values rounded to day.
- Hidden-bundled-relation filter (uses ad-hoc skip list instead of `bundle.IsSystemRelation` + `bundle.GetRelation(k).Hidden`).

Leaks:
- `RelationKeyIconImage`, `RelationKeyPicture` are not skipped — every object's icon/picture image becomes an outgoing link.
- `RelationKeyCoverId` (when `coverType==1`) is included and **not** post-filtered.

The pre-GO-6052 path (`injectLinksDetails` → `objectlink.DependentObjectIDs(Flags{...})` + `relationsToFilterOutForLinks` post-filter) does not have any of these problems.

## Goal

Single source of truth for "what's an outgoing link from this state." The attributed and flat output shapes both come from one walker driven by the same `objectlink.Flags` preset.

## Design

### 1. New attributed entry point in `objectlink`

Add `DependentObjectLinks` next to `DependentObjectIDs`, sharing all the per-block / per-relation / per-store helpers:

```go
// core/block/object/objectlink/dependent_objects.go

// OutgoingLink moved here from core/block/editor/smartblock.
type OutgoingLink struct {
    TargetID      string
    SourceBlockID string // set when the link originated from a block; empty for relation/store links
    RelationKey   string // set when the link originated from a relation; empty otherwise
}

func DependentObjectLinks(
    s *state.State,
    converter KeyToIDConverter,
    fetcher relationutils.RelationFormatFetcher,
    flags Flags,
) []OutgoingLink
```

`DependentObjectIDs` stays. Either:
- (a) leave it as-is (duplicate iteration but no caller churn), or
- (b) reimplement it as `lo.Map(DependentObjectLinks(...), func(l OutgoingLink) string { return l.TargetID })`.

Default to (b) — single iteration, one source of truth, deduping logic shared. If profiling shows an issue we revert to (a).

### 2. Internal refactor: per-source visitors

Split the existing function bodies into small visitor helpers that emit `OutgoingLink` records:

```go
// All package-private. Each one knows its origin tag.
func visitBlockLinks(s *state.State, flags Flags, emit func(OutgoingLink) bool) error
func visitRelationLinks(s *state.State, fetcher RelationFormatFetcher, flags Flags, emit func(OutgoingLink) bool)
func visitTypeLinks(s *state.State, converter KeyToIDConverter, emit func(OutgoingLink) bool)
func visitCollectionStoreLinks(s *state.State, flags Flags, emit func(OutgoingLink) bool)
```

`visitBlockLinks` walks `s.Iterate`, applies `DataviewBlockOnlyTarget` / `NoImages` short-circuits as today, then calls `b.FillSmartIds(nil)` for the catch-all path and tags every returned ID with `SourceBlockID = b.Model().Id`. No changes to any `simple/*` block package — `FillSmartIds` stays `[]string → []string`.

`visitRelationLinks` walks `s.AllRelationKeys()`, applies `NoSystemRelations` / `NoHiddenBundledRelations` / `Relations` flags as today, calls `collectIdsFromDetail` per key, and tags every returned ID with `RelationKey = key.String()`.

`visitCollectionStoreLinks` reads `s.GetStoreSlice(template.CollectionStoreKey)` and emits each member with empty `SourceBlockID` and empty `RelationKey` (or a sentinel if GC needs to distinguish — see Open Questions).

`DependentObjectLinks` orchestrates the visitors, dedupes by `TargetID`, removes self-id, and applies the post-filter currently in `injectLinksDetails` (`relationsToFilterOutForLinks`: drop targets that appear *only* via icon/picture/fileId/cover relations) when configured. The post-filter is added as a new flag bit, e.g. `FilterPresentationOnly`, defaulting on for the indexer preset and off for callers that don't want it.

### 3. Wire `getDocInfo` to the new entry point

```go
// core/block/editor/smartblock/smartblock.go

func (sb *smartBlock) getDocInfo(st *state.State) DocInfo {
    // ...
    outgoingLinks := objectlink.DependentObjectLinks(st, sb.Space(), sb.formatFetcher, objectlink.Flags{
        Blocks:                   true,
        Details:                  true,
        Relations:                sb.includeRelationObjectsAsDependents,
        Types:                    false,
        Collection:               !internalflag.NewFromState(st).Has(model.InternalFlag_collectionDontIndexLinks),
        DataviewBlockOnlyTarget:  true,
        NoSystemRelations:        true,
        NoHiddenBundledRelations: true,
        NoImages:                 false,
        RoundDateIdsToDay:        true,
        FilterPresentationOnly:   true,
    })
    // ...
}
```

Same flag set as `injectLinksDetails`. Output type changes from `[]string` to `[]objectlink.OutgoingLink`.

`DocInfo.OutgoingLinks` field type changes from `[]smartblock.OutgoingLink` to `[]objectlink.OutgoingLink`. The `smartblock.OutgoingLink` struct is removed.

### 4. Delete `collectOutgoingLinks` and `collectLinksFromRelations`

Both functions, plus `relationsToSkipLinksIndexing` and `relationsToFilterOutForLinks` if redundant after move, are deleted. `injectLinksDetails` either keeps calling `DependentObjectIDs` or is rewritten as `lo.Map` over `DependentObjectLinks` — pick whichever leaves the diff smaller.

### 5. Indexer call site stays unchanged

`core/indexer/spaceindexer.go:209-228` continues to convert `info.OutgoingLinks` to `spaceindex.OutgoingLink` and call `UpdateObjectLinksDetailed`. The fallback branch (`UpdateObjectLinks(info.Id, info.Links)`) becomes effectively dead code for indexable types, but stays as a safety net.

## Behavior changes (expected)

For every existing object that gets re-indexed after this lands:

- Collection members gain a backlink to their collection (the bug).
- Pages with inline embeds of a Set/Collection appear in the embedded object's backlinks.
- Bookmarked objects gain backlinks from objects whose bookmark blocks point to them.
- Tag/Status options gain backlinks from objects using them.
- Date objects (e.g. day pages) gain backlinks from objects whose date relations point to that day.
- Cover/icon-only file references stop polluting backlinks.
- Hidden bundled relation values stop polluting backlinks.

The reverse-index re-converges via `StateAppend` / re-index on next change, but a one-shot reindex pass for affected smartblock types is not in scope here (filed separately if needed).

## Out of scope

- Object-type text mark (`Mark_Object`) handling — already addressed on branch `GO-0000-backlinks-object-mark`. After this lands, that fix collapses to a no-op because `Text.FillSmartIds` already returns Object marks; the gap was only in `collectOutgoingLinks`.
- A force-reindex migration. Steady state recovers as objects get touched; if product wants instant correction we add a one-shot pass under a separate ticket.
- Touching `injectMentions` (uses a different flag preset on purpose; no behavior change needed).

## Test plan

- `core/block/object/objectlink/dependent_objects_test.go` — new test file (or extend existing) covering each visitor:
  - `visitBlockLinks`: link block, file (image and non-image), bookmark, mention mark, object mark, dataview block (`DataviewBlockOnlyTarget` on/off, `NoImages` on/off).
  - `visitRelationLinks`: object/file/status/tag/date relations, system relation skip, hidden bundled skip, presentation-only post-filter.
  - `visitCollectionStoreLinks`: collection with members, with `InternalFlag_collectionDontIndexLinks` set/clear.
  - Self-id stripped, dedup across sources, attribution preserved.
- `core/block/editor/smartblock/smartblock_test.go` — replace the existing `TestSmartBlock_CollectOutgoingLinks` with assertions that `getDocInfo` produces attributed links matching the expected preset, including a regression test: collection with two member objects → `getDocInfo` returns one `OutgoingLink` per member with empty `SourceBlockID`/`RelationKey`.
- Integration: a focused test in `core/block/collection` (or extend `service_test.go`) that adds an object to a collection, runs the indexer, and asserts the object's `bundle.RelationKeyBacklinks` now contains the collection ID.
- Run existing tests for `objectlink`, `smartblock`, indexer, exporter, and the dot/graphjson converters — none of those callers should observe any change in `DependentObjectIDs` output.

## Open questions

1. Should `visitCollectionStoreLinks` set a sentinel `RelationKey = "_collection"` on emitted records so GC can distinguish collection-membership links from genuine relation links, or is empty `RelationKey` + empty `SourceBlockID` enough? Current GC consumers (verify before implementing) appear to only switch on `SourceBlockID`, so empty/empty likely suffices.
2. Keep `DependentObjectIDs` as a thin adapter over `DependentObjectLinks` (option b above) or leave both walking independently (option a)? Default: (b) — but verify no perf regression for high-fan-out exports.
3. Whether to add `FilterPresentationOnly` as a distinct flag, or always apply the icon/picture/cover post-filter in the attributed walker (since it's only ever called for "outgoing links" semantics today)? Default: distinct flag, preserves current `DependentObjectIDs` behavior for non-link callers.

## Files touched

Modified:
- `core/block/object/objectlink/dependent_objects.go` — add `DependentObjectLinks`, `OutgoingLink`, visitor splits, new `FilterPresentationOnly` flag.
- `core/block/editor/smartblock/smartblock.go` — remove `collectOutgoingLinks`, `collectLinksFromRelations`, `OutgoingLink`; update `getDocInfo` to call `DependentObjectLinks`; update `DocInfo.OutgoingLinks` field type.
- `core/block/editor/smartblock/links.go` — leave or simplify (`injectLinksDetails`); remove `relationsToSkipLinksIndexing` / `relationsToFilterOutForLinks` if no longer referenced after the move (their semantics relocate into `objectlink` flag handling).
- `core/indexer/spaceindexer.go` — update import, struct conversion uses `objectlink.OutgoingLink`.
- `core/block/objectgc/...` — update import path for `OutgoingLink`.
- `core/block/editor/smartblock/smartblock_test.go` — adapt tests.

Added:
- `core/block/object/objectlink/dependent_objects_test.go` — visitor-level tests (or extend existing test file).

Removed:
- `smartblock.OutgoingLink` (moved to `objectlink`).
- `smartblock.collectOutgoingLinks`, `smartblock.collectLinksFromRelations` (in `smartblock.go`).
- `relationsToSkipLinksIndexing` / `relationsToFilterOutForLinks` (in `links.go`) — only if no remaining references after the move.

## Acceptance criteria

- Adding an object to a collection produces a backlink from that object to the collection (verified by integration test and by running the app).
- An inline embed of Set/Collection on another page produces a backlink on the embedded object.
- Cover/icon-only file references no longer appear in objects' `links`/`backlinks`.
- All existing `objectlink`, `smartblock`, indexer, exporter, history, dot, graphjson tests pass without modification (except the targeted smartblock test rewrite).
- `grep -r 'collectOutgoingLinks\|collectLinksFromRelations' core/` returns nothing.
