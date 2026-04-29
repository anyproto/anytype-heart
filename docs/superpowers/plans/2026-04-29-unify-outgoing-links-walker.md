# Unify Outgoing-Links Walker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the divergent `smartblock.collectOutgoingLinks` walker with a single attributed-output entry point in `objectlink` driven by the same `Flags` preset that `injectLinksDetails` already uses, restoring collection-member backlinks and closing the gap class.

**Architecture:** Introduce `objectlink.DependentObjectLinks` next to `DependentObjectIDs`. Both functions share private per-source visitors (`visitBlockLinks`, `visitRelationLinks`, `visitTypeLinks`, `visitCollectionStoreLinks`). `DependentObjectIDs` becomes a thin projection over the attributed walker. `getDocInfo` calls `DependentObjectLinks` with the canonical preset; `collectOutgoingLinks` and `collectLinksFromRelations` are deleted. The `OutgoingLink` struct moves from `smartblock` to `objectlink`.

**Tech Stack:** Go, testify, mockery (for existing mocks), make (`make test`).

**Spec:** `docs/superpowers/specs/2026-04-29-unify-outgoing-links-walker-design.md`

---

## File Structure

**Modified:**
- `core/block/object/objectlink/dependent_objects.go` — add `OutgoingLink` type, `DependentObjectLinks` function, `FilterPresentationOnly` flag, private visitor helpers.
- `core/block/object/objectlink/dependent_objects_test.go` — new tests for visitors and the public function.
- `core/block/editor/smartblock/smartblock.go` — remove `OutgoingLink`, `collectOutgoingLinks`, `collectLinksFromRelations`; change `DocInfo.OutgoingLinks` field type to `[]objectlink.OutgoingLink`; rewrite `getDocInfo` to call `DependentObjectLinks`.
- `core/block/editor/smartblock/links.go` — remove `relationsToSkipLinksIndexing` / `relationsToFilterOutForLinks` (now lives in `objectlink` flag handling). `injectLinksDetails` keeps using `DependentObjectIDs`.
- `core/block/editor/smartblock/smartblock_test.go` — rewrite `TestSmartBlock_CollectOutgoingLinks` cases as integration tests over `getDocInfo`, OR delete and keep coverage in `objectlink` tests. (See Task 8.)
- `core/indexer/spaceindexer.go` — update import; loop body changes from `link.SourceBlockID` to `link.SourceBlockID` (struct moved, field name unchanged).
- `core/block/collection/service_test.go` — add backlink integration test.

**Untouched but affected by re-export of `OutgoingLink`:**
- All callers that referenced `smartblock.OutgoingLink` directly (none on develop other than smartblock itself and the indexer).

---

## Task 1: Move `OutgoingLink` struct to `objectlink` (no behavior change)

**Why first:** Mechanical move. Every later task depends on the type living in `objectlink`. Doing this in isolation makes the diff easy to review.

**Files:**
- Modify: `core/block/object/objectlink/dependent_objects.go`
- Modify: `core/block/editor/smartblock/smartblock.go`
- Modify: `core/indexer/spaceindexer.go`

- [ ] **Step 1.1: Add `OutgoingLink` struct in `objectlink` package**

In `core/block/object/objectlink/dependent_objects.go`, append after the existing types (after the `Flags` struct):

```go
// OutgoingLink represents a link from one object to another, with optional source attribution.
// SourceBlockID is set when the link originates from a block; RelationKey is set when it
// originates from a relation; both are empty for collection-store membership links.
type OutgoingLink struct {
	TargetID      string
	SourceBlockID string
	RelationKey   string
}
```

- [ ] **Step 1.2: Replace local `OutgoingLink` in `smartblock` with a type alias**

In `core/block/editor/smartblock/smartblock.go`, delete the local struct (currently around lines 204-209):

```go
// OutgoingLink represents a link from this object to another object
type OutgoingLink struct {
	TargetID      string // ID of the target object
	SourceBlockID string // Block ID where the link originates (empty for relation links)
	RelationKey   string // Relation key (empty for block links)
}
```

Replace it with an alias so callers using `smartblock.OutgoingLink` keep compiling:

```go
// OutgoingLink is an alias for objectlink.OutgoingLink.
// Kept for backwards compatibility while smartblock callers migrate.
type OutgoingLink = objectlink.OutgoingLink
```

Add the import for `objectlink` if not already present:

```go
"github.com/anyproto/anytype-heart/core/block/object/objectlink"
```

- [ ] **Step 1.3: Verify the build passes and tests still compile**

Run:

```bash
go build ./core/...
go test -run TestSmartBlock_CollectOutgoingLinks ./core/block/editor/smartblock/
```

Expected: build succeeds; existing tests still pass (the alias makes the rename transparent).

- [ ] **Step 1.4: Commit**

```bash
git add core/block/object/objectlink/dependent_objects.go core/block/editor/smartblock/smartblock.go
git commit -m "GO-7237 Move OutgoingLink to objectlink package via alias"
```

---

## Task 2: Add private visitor helpers in `objectlink` (refactor with no behavior change)

**Why:** The existing `DependentObjectIDs` body has all the per-source logic we need. Extract each source into its own visitor that emits `OutgoingLink` records (block ID / relation key attached). `DependentObjectIDs` then becomes a thin loop that projects to `[]string`. Pure refactor, no public API change.

**Files:**
- Modify: `core/block/object/objectlink/dependent_objects.go`

- [ ] **Step 2.1: Write a regression test for `DependentObjectIDs` to lock current behavior**

Append to `core/block/object/objectlink/dependent_objects_test.go` (create the file if it doesn't exist):

```go
package objectlink_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/object/objectlink"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// stubConverter / stubFetcher: minimal interface implementations sufficient for the test.
type stubConverter struct{ id string }

func (s *stubConverter) GetRelationIdByKey(_ context.Context, k domain.RelationKey) (string, error) {
	return "rel_" + string(k), nil
}
func (s *stubConverter) GetTypeIdByKey(_ context.Context, k domain.TypeKey) (string, error) {
	return "type_" + string(k), nil
}
func (s *stubConverter) Id() string { return s.id }

type stubFetcher struct{ formats map[domain.RelationKey]model.RelationFormat }

func (s *stubFetcher) GetRelationFormatByKey(_ string, k domain.RelationKey) (model.RelationFormat, error) {
	if f, ok := s.formats[k]; ok {
		return f, nil
	}
	return model.RelationFormat_object, nil
}

func newStateWithCollection(t *testing.T, members []string) *state.State {
	t.Helper()
	root := simple.New(&model.Block{Id: "root"})
	doc := state.NewDoc("root", map[string]simple.Block{"root": root}).(*state.Doc)
	st := doc.NewState()
	st.UpdateStoreSlice(template.CollectionStoreKey, members)
	return st
}

func TestDependentObjectIDs_CollectionStoreIncluded(t *testing.T) {
	// given
	st := newStateWithCollection(t, []string{"obj1", "obj2"})
	conv := &stubConverter{id: "spc1"}
	f := &stubFetcher{}

	// when
	ids := objectlink.DependentObjectIDs(st, conv, f, objectlink.Flags{
		Blocks:     true,
		Collection: true,
	})

	// then
	require.Subset(t, ids, []string{"obj1", "obj2"})
}
```

- [ ] **Step 2.2: Run the test — should PASS already**

```bash
go test ./core/block/object/objectlink/ -run TestDependentObjectIDs_CollectionStoreIncluded -v
```

Expected: PASS. This locks current behavior so the refactor in subsequent steps cannot regress it.

- [ ] **Step 2.3: Extract `visitBlockLinks` from the existing `collectIdsFromBlocks`**

Edit `core/block/object/objectlink/dependent_objects.go`. Add the new helper above `collectIdsFromBlocks`:

```go
// visitBlockLinks calls emit for every outgoing-link target found in blocks. Each emitted
// record carries SourceBlockID = block.Model().Id. emit returning false stops iteration.
func visitBlockLinks(s *state.State, flags Flags, emit func(OutgoingLink) bool) {
	err := s.Iterate(func(b simple.Block) bool {
		blockId := b.Model().Id

		if flags.DataviewBlockOnlyTarget {
			if dv := b.Model().GetDataview(); dv != nil {
				if dv.TargetObjectId != "" {
					if !emit(OutgoingLink{TargetID: dv.TargetObjectId, SourceBlockID: blockId}) {
						return false
					}
				}
				return true
			}
		}

		if flags.NoImages {
			if f := b.Model().GetFile(); f != nil {
				if f.TargetObjectId != "" && f.Type != model.BlockContentFile_Image {
					if !emit(OutgoingLink{TargetID: f.TargetObjectId, SourceBlockID: blockId}) {
						return false
					}
				}
				return true
			}
		}

		if ls, ok := b.(linkSource); ok {
			for _, id := range ls.FillSmartIds(nil) {
				if !emit(OutgoingLink{TargetID: id, SourceBlockID: blockId}) {
					return false
				}
			}
		}
		return true
	})
	if err != nil {
		log.With("objectID", s.RootId()).Errorf("failed to iterate over simple blocks: %s", err)
	}
}
```

- [ ] **Step 2.4: Rewrite `collectIdsFromBlocks` to use `visitBlockLinks`**

Replace the body of `collectIdsFromBlocks` (currently lines 139-169) with:

```go
func collectIdsFromBlocks(s *state.State, flags Flags) (ids []string) {
	visitBlockLinks(s, flags, func(link OutgoingLink) bool {
		ids = append(ids, link.TargetID)
		return true
	})
	return ids
}
```

- [ ] **Step 2.5: Run the regression test plus existing objectlink tests**

```bash
go test ./core/block/object/objectlink/ -v
```

Expected: all tests pass. No behavior change.

- [ ] **Step 2.6: Extract `visitRelationLinks` from the existing relation loop in `DependentObjectIDs`**

Add another helper:

```go
// visitRelationLinks calls emit for every outgoing-link target found in object relations.
// Each record carries RelationKey = relation key string. Honors the Flags filters.
func visitRelationLinks(s *state.State, converter KeyToIDConverter, fetcher relationutils.RelationFormatFetcher, flags Flags, emit func(OutgoingLink) bool) {
	det := s.CombinedDetails()
	if det == nil {
		return
	}

	for _, key := range s.AllRelationKeys() {
		if flags.Relations {
			id, err := converter.GetRelationIdByKey(context.Background(), key)
			if err != nil {
				log.With("objectID", s.RootId()).Errorf("failed to get relation id by key %s: %s", key, err)
			} else if !emit(OutgoingLink{TargetID: id, RelationKey: key.String()}) {
				return
			}
		}

		if !flags.Details {
			continue
		}

		format, err := fetcher.GetRelationFormatByKey(converter.Id(), key)
		if err != nil {
			format = model.RelationFormat_object
		}

		ids := collectIdsFromDetail(&model.RelationLink{Key: key.String(), Format: format}, det, flags)
		for _, id := range ids {
			if !emit(OutgoingLink{TargetID: id, RelationKey: key.String()}) {
				return
			}
		}
	}
}
```

- [ ] **Step 2.7: Rewrite the relation loop in `DependentObjectIDs` to call `visitRelationLinks`**

In `DependentObjectIDs` (around lines 67-93), replace the entire `for _, key := range s.AllRelationKeys() { ... }` block plus the `var det *domain.Details / if flags.Details / det = s.CombinedDetails()` lines with:

```go
visitRelationLinks(s, converter, fetcher, flags, func(link OutgoingLink) bool {
	ids = append(ids, link.TargetID)
	return true
})
```

- [ ] **Step 2.8: Add `visitTypeLinks` and `visitCollectionStoreLinks` helpers**

Below the other visitors:

```go
func visitTypeLinks(s *state.State, converter KeyToIDConverter, emit func(OutgoingLink) bool) {
	for _, k := range s.ObjectTypeKeys() {
		if k == "" {
			continue
		}
		id, err := converter.GetTypeIdByKey(context.Background(), k)
		if err != nil {
			log.With("objectID", s.RootId()).Errorf("failed to get object type id by key %s: %s", k, err)
			continue
		}
		if !emit(OutgoingLink{TargetID: id}) {
			return
		}
	}
}

func visitCollectionStoreLinks(s *state.State, emit func(OutgoingLink) bool) {
	for _, id := range s.GetStoreSlice(template.CollectionStoreKey) {
		if id == "" {
			continue
		}
		if !emit(OutgoingLink{TargetID: id}) {
			return
		}
	}
}
```

- [ ] **Step 2.9: Switch `DependentObjectIDs` body to use all four visitors**

The full rewritten function:

```go
func DependentObjectIDs(s *state.State, converter KeyToIDConverter, fetcher relationutils.RelationFormatFetcher, flags Flags) (ids []string) {
	if flags.Blocks {
		visitBlockLinks(s, flags, func(link OutgoingLink) bool {
			ids = append(ids, link.TargetID)
			return true
		})
	}

	if flags.Types {
		visitTypeLinks(s, converter, func(link OutgoingLink) bool {
			ids = append(ids, link.TargetID)
			return true
		})
	}

	visitRelationLinks(s, converter, fetcher, flags, func(link OutgoingLink) bool {
		ids = append(ids, link.TargetID)
		return true
	})

	if flags.Collection {
		visitCollectionStoreLinks(s, func(link OutgoingLink) bool {
			ids = append(ids, link.TargetID)
			return true
		})
	}

	if flags.RoundDateIdsToDay {
		ids = roundDateIds(ids)
	}

	ids = lo.Uniq(ids)
	return
}
```

- [ ] **Step 2.10: Run the full objectlink test suite**

```bash
go test ./core/block/object/objectlink/ -v
```

Expected: all pass. Then run callers to confirm no regression:

```bash
go test ./core/block/editor/smartblock/... ./core/indexer/... ./core/converter/dot/... ./core/converter/graphjson/... ./core/history/... ./core/block/export/...
```

Expected: all pass.

- [ ] **Step 2.11: Commit**

```bash
git add core/block/object/objectlink/
git commit -m "GO-7237 Refactor objectlink walker into per-source visitors"
```

---

## Task 3: Add `DependentObjectLinks` public API and tests

**Files:**
- Modify: `core/block/object/objectlink/dependent_objects.go`
- Modify: `core/block/object/objectlink/dependent_objects_test.go`

- [ ] **Step 3.1: Write the failing test**

Append to `dependent_objects_test.go`:

```go
func TestDependentObjectLinks_AttributesBlockAndRelation(t *testing.T) {
	// given a state with one link block, one object relation
	root := simple.New(&model.Block{Id: "root", ChildrenIds: []string{"link1"}})
	linkBlk := simple.New(&model.Block{
		Id: "link1",
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{TargetBlockId: "tgtBlock"},
		},
	})
	doc := state.NewDoc("root", map[string]simple.Block{"root": root, "link1": linkBlk}).(*state.Doc)
	st := doc.NewState()
	st.SetDetail(bundle.RelationKeyAssignee, domain.String("tgtRel"))

	conv := &stubConverter{id: "spc1"}
	f := &stubFetcher{formats: map[domain.RelationKey]model.RelationFormat{
		bundle.RelationKeyAssignee: model.RelationFormat_object,
	}}

	// when
	links := objectlink.DependentObjectLinks(st, conv, f, objectlink.Flags{
		Blocks:  true,
		Details: true,
	})

	// then
	require.Len(t, links, 2)
	byTarget := map[string]objectlink.OutgoingLink{}
	for _, l := range links {
		byTarget[l.TargetID] = l
	}
	assert.Equal(t, "link1", byTarget["tgtBlock"].SourceBlockID)
	assert.Empty(t, byTarget["tgtBlock"].RelationKey)
	assert.Equal(t, bundle.RelationKeyAssignee.String(), byTarget["tgtRel"].RelationKey)
	assert.Empty(t, byTarget["tgtRel"].SourceBlockID)
}
```

- [ ] **Step 3.2: Run test, expect FAIL**

```bash
go test ./core/block/object/objectlink/ -run TestDependentObjectLinks_AttributesBlockAndRelation -v
```

Expected: FAIL — `DependentObjectLinks` undefined.

- [ ] **Step 3.3: Implement `DependentObjectLinks`**

Append to `dependent_objects.go` (next to `DependentObjectIDs`):

```go
// DependentObjectLinks returns outgoing links from a state with per-source attribution
// (SourceBlockID for block-derived links, RelationKey for relation-derived links).
// Same Flags semantics as DependentObjectIDs.
func DependentObjectLinks(s *state.State, converter KeyToIDConverter, fetcher relationutils.RelationFormatFetcher, flags Flags) []OutgoingLink {
	var (
		links []OutgoingLink
		seen  = make(map[string]struct{})
	)
	emit := func(link OutgoingLink) bool {
		if link.TargetID == "" {
			return true
		}
		if _, ok := seen[link.TargetID]; ok {
			return true
		}
		seen[link.TargetID] = struct{}{}
		links = append(links, link)
		return true
	}

	if flags.Blocks {
		visitBlockLinks(s, flags, emit)
	}
	if flags.Types {
		visitTypeLinks(s, converter, emit)
	}
	visitRelationLinks(s, converter, fetcher, flags, emit)
	if flags.Collection {
		visitCollectionStoreLinks(s, emit)
	}

	if flags.RoundDateIdsToDay {
		for i := range links {
			if rounded := roundDateIds([]string{links[i].TargetID}); len(rounded) == 1 {
				links[i].TargetID = rounded[0]
			}
		}
	}
	return links
}
```

- [ ] **Step 3.4: Run test, expect PASS**

```bash
go test ./core/block/object/objectlink/ -run TestDependentObjectLinks_AttributesBlockAndRelation -v
```

Expected: PASS.

- [ ] **Step 3.5: Add a collection-store-attribution test**

Append:

```go
func TestDependentObjectLinks_CollectionStoreEmittedWithoutAttribution(t *testing.T) {
	// given
	st := newStateWithCollection(t, []string{"obj1", "obj2"})
	conv := &stubConverter{id: "spc1"}
	f := &stubFetcher{}

	// when
	links := objectlink.DependentObjectLinks(st, conv, f, objectlink.Flags{
		Blocks:     true,
		Collection: true,
	})

	// then
	require.Len(t, links, 2)
	for _, l := range links {
		assert.Empty(t, l.SourceBlockID)
		assert.Empty(t, l.RelationKey)
		assert.Contains(t, []string{"obj1", "obj2"}, l.TargetID)
	}
}
```

- [ ] **Step 3.6: Run; expect PASS**

```bash
go test ./core/block/object/objectlink/ -run TestDependentObjectLinks -v
```

Expected: PASS.

- [ ] **Step 3.7: Commit**

```bash
git add core/block/object/objectlink/
git commit -m "GO-7237 Add objectlink.DependentObjectLinks attributed walker"
```

---

## Task 4: Add `FilterPresentationOnly` flag and migrate the post-filter

**Why:** `injectLinksDetails` currently does a post-filter that drops icon/picture/cover/fileId-only references (`relationsToFilterOutForLinks`). To make `DependentObjectLinks` produce the same set as the canonical preset, this filter must be available inside the walker. We add it as a flag rather than always applying it, to preserve `DependentObjectIDs` behavior for non-link callers (export, history, dot).

**Files:**
- Modify: `core/block/object/objectlink/dependent_objects.go`
- Modify: `core/block/object/objectlink/dependent_objects_test.go`

- [ ] **Step 4.1: Write the failing test**

Append to `dependent_objects_test.go`:

```go
func TestDependentObjectLinks_FilterPresentationOnlyDropsIconOnlyReferences(t *testing.T) {
	// given a state where a file id appears ONLY as iconImage
	root := simple.New(&model.Block{Id: "root"})
	doc := state.NewDoc("root", map[string]simple.Block{"root": root}).(*state.Doc)
	st := doc.NewState()
	st.SetDetail(bundle.RelationKeyIconImage, domain.String("iconFileId"))

	conv := &stubConverter{id: "spc1"}
	f := &stubFetcher{formats: map[domain.RelationKey]model.RelationFormat{
		bundle.RelationKeyIconImage: model.RelationFormat_file,
	}}

	// when filter is on
	linksFiltered := objectlink.DependentObjectLinks(st, conv, f, objectlink.Flags{
		Blocks:                 true,
		Details:                true,
		FilterPresentationOnly: true,
	})
	// when filter is off
	linksAll := objectlink.DependentObjectLinks(st, conv, f, objectlink.Flags{
		Blocks:  true,
		Details: true,
	})

	// then
	for _, l := range linksFiltered {
		assert.NotEqual(t, "iconFileId", l.TargetID, "filter should drop icon-only refs")
	}
	hasIcon := false
	for _, l := range linksAll {
		if l.TargetID == "iconFileId" {
			hasIcon = true
		}
	}
	assert.True(t, hasIcon, "without filter the icon ref must still be present")
}
```

- [ ] **Step 4.2: Run; expect FAIL — flag doesn't exist**

```bash
go test ./core/block/object/objectlink/ -run TestDependentObjectLinks_FilterPresentationOnly -v
```

- [ ] **Step 4.3: Add the flag and the filter logic**

In `dependent_objects.go`:

1. Extend the `Flags` struct (the field list) to add `FilterPresentationOnly` after `RoundDateIdsToDay`:

```go
type Flags struct {
	Blocks,
	Details,
	Relations,
	Types,
	Collection,
	CreatorModifierWorkspace,
	DataviewBlockOnlyTarget,
	NoSystemRelations,
	NoHiddenBundledRelations,
	NoImages,
	RoundDateIdsToDay,
	FilterPresentationOnly,
	NoBackLinks bool
}
```

2. Add a package-private constant near the top of the file:

```go
// presentationOnlyRelations are relations whose values are presentation-only references
// (icon, picture, cover, file binding). Targets that appear only via these relations are
// filtered out by FilterPresentationOnly.
var presentationOnlyRelations = []domain.RelationKey{
	bundle.RelationKeyIconImage,
	bundle.RelationKeyPicture,
	bundle.RelationKeyFileId,
	bundle.RelationKeyCoverId,
}
```

3. Add the filter helper, used by both `DependentObjectIDs` and `DependentObjectLinks`:

```go
// filterPresentationOnly removes target IDs that appear ONLY as values of presentation
// relations (icon, picture, cover, fileId). If the same target appears via any other source
// (block, non-presentation relation, store slice) it is kept.
func filterPresentationOnly[T any](
	s *state.State,
	items []T,
	targetOf func(T) string,
	originOf func(T) (relationKey string, isRelationOrigin bool),
) []T {
	if len(items) == 0 {
		return items
	}
	det := s.Details()
	if det == nil {
		return items
	}
	presentationOnly := map[string]bool{}
	for _, key := range presentationOnlyRelations {
		if !det.Has(key) {
			continue
		}
		val := det.Get(key)
		if str, ok := val.TryString(); ok && str != "" {
			presentationOnly[str] = true
		}
		if list, ok := val.TryStringList(); ok {
			for _, v := range list {
				presentationOnly[v] = true
			}
		}
	}
	if len(presentationOnly) == 0 {
		return items
	}

	// Mark targets that have at least one non-presentation source.
	hasNonPresentation := map[string]bool{}
	for _, it := range items {
		t := targetOf(it)
		if t == "" {
			continue
		}
		relKey, isRel := originOf(it)
		if !isRel {
			hasNonPresentation[t] = true
			continue
		}
		isPresentation := false
		for _, k := range presentationOnlyRelations {
			if relKey == k.String() {
				isPresentation = true
				break
			}
		}
		if !isPresentation {
			hasNonPresentation[t] = true
		}
	}

	out := items[:0]
	for _, it := range items {
		t := targetOf(it)
		if presentationOnly[t] && !hasNonPresentation[t] {
			continue
		}
		out = append(out, it)
	}
	return out
}
```

4. In `DependentObjectLinks`, apply the filter just before the dedup-finalize:

```go
if flags.FilterPresentationOnly {
	links = filterPresentationOnly(s, links,
		func(l OutgoingLink) string { return l.TargetID },
		func(l OutgoingLink) (string, bool) { return l.RelationKey, l.RelationKey != "" },
	)
}
```

5. In `DependentObjectIDs`, apply the equivalent before `lo.Uniq`. We need attribution for the filter — the cleanest path is to project the result of `DependentObjectLinks` when the flag is set:

```go
// If presentation filter is requested, route through the attributed walker so we can
// distinguish presentation relations from other sources before flattening.
if flags.FilterPresentationOnly {
	links := DependentObjectLinks(s, converter, fetcher, flags)
	out := make([]string, 0, len(links))
	for _, l := range links {
		out = append(out, l.TargetID)
	}
	return out
}
```

(Place this near the top of `DependentObjectIDs`. Do not run the rest of the body when the filter is requested.)

- [ ] **Step 4.4: Run the new test**

```bash
go test ./core/block/object/objectlink/ -run TestDependentObjectLinks_FilterPresentationOnly -v
```

Expected: PASS.

- [ ] **Step 4.5: Run full objectlink suite**

```bash
go test ./core/block/object/objectlink/ -v
```

Expected: all pass.

- [ ] **Step 4.6: Commit**

```bash
git add core/block/object/objectlink/
git commit -m "GO-7237 Add FilterPresentationOnly flag for icon/cover/picture filtering"
```

---

## Task 5: Switch `getDocInfo` to call `DependentObjectLinks`

**Files:**
- Modify: `core/block/editor/smartblock/smartblock.go`

- [ ] **Step 5.1: Write a failing test asserting collection members appear in `OutgoingLinks`**

Append to `core/block/editor/smartblock/smartblock_test.go`:

```go
func TestSmartBlock_GetDocInfo_CollectionMembersInOutgoingLinks(t *testing.T) {
	// given a smartblock state with two collection members
	objectId := "coll1"
	fx := newFixture(objectId, t)
	fx.init(t, []*model.Block{
		{Id: objectId},
	})
	st := fx.NewState()
	st.UpdateStoreSlice(template.CollectionStoreKey, []string{"member1", "member2"})
	require.NoError(t, fx.Apply(st))

	// when
	info := fx.GetDocInfo()

	// then
	targets := map[string]bool{}
	for _, l := range info.OutgoingLinks {
		targets[l.TargetID] = true
	}
	assert.True(t, targets["member1"], "member1 must appear in OutgoingLinks")
	assert.True(t, targets["member2"], "member2 must appear in OutgoingLinks")
}
```

(If `template` and `model` are not yet imported in this file, add the imports.)

- [ ] **Step 5.2: Run the test, expect FAIL**

```bash
go test ./core/block/editor/smartblock/ -run TestSmartBlock_GetDocInfo_CollectionMembersInOutgoingLinks -v
```

Expected: FAIL — `info.OutgoingLinks` does not include collection members because `collectOutgoingLinks` is still in use.

- [ ] **Step 5.3: Replace `collectOutgoingLinks(st)` call in `getDocInfo` with `objectlink.DependentObjectLinks`**

In `core/block/editor/smartblock/smartblock.go`, find the line in `getDocInfo` (around line 1321):

```go
outgoingLinks := sb.collectOutgoingLinks(st)
```

Replace with:

```go
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
```

Add imports if missing:

```go
"github.com/anyproto/anytype-heart/core/block/object/objectlink"
"github.com/anyproto/anytype-heart/util/internalflag"
"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
```

- [ ] **Step 5.4: Run the test, expect PASS**

```bash
go test ./core/block/editor/smartblock/ -run TestSmartBlock_GetDocInfo_CollectionMembersInOutgoingLinks -v
```

Expected: PASS.

- [ ] **Step 5.5: Commit**

```bash
git add core/block/editor/smartblock/smartblock.go core/block/editor/smartblock/smartblock_test.go
git commit -m "GO-7237 Wire getDocInfo to objectlink.DependentObjectLinks"
```

---

## Task 6: Delete obsolete `collectOutgoingLinks` / `collectLinksFromRelations`

**Why:** Now that `getDocInfo` no longer calls them, they're dead code. Removing them prevents future regressions where someone reaches for the wrong walker.

**Files:**
- Modify: `core/block/editor/smartblock/smartblock.go`
- Modify: `core/block/editor/smartblock/smartblock_test.go`

- [ ] **Step 6.1: Delete the two functions and their test cases**

In `smartblock.go`, delete:
- `func (sb *smartBlock) collectOutgoingLinks(st *state.State) []OutgoingLink` (currently around lines 1454-1506)
- `func (sb *smartBlock) collectLinksFromRelations(...) []OutgoingLink` (around lines 1509-1555)
- `func guessRelationFormatFromValue(...)` (around lines 1559-1584) — only called by `collectLinksFromRelations`. Verify with grep first.

In `smartblock_test.go`, delete the entire `TestSmartBlock_CollectOutgoingLinks` test function (it tested the deleted private function). The new collection test in Task 5 covers the most important integration case; the per-source unit tests now live in `objectlink/dependent_objects_test.go` (Tasks 2-4).

Verify nothing else in the repo references those identifiers:

```bash
grep -rn "collectOutgoingLinks\|collectLinksFromRelations\|guessRelationFormatFromValue" --include="*.go" core/ pkg/
```

Expected: zero results.

- [ ] **Step 6.2: Build and run smartblock tests**

```bash
go build ./...
go test ./core/block/editor/smartblock/ -v
```

Expected: build clean, tests pass.

- [ ] **Step 6.3: Commit**

```bash
git add core/block/editor/smartblock/
git commit -m "GO-7237 Remove obsolete collectOutgoingLinks/collectLinksFromRelations"
```

---

## Task 7: Clean up `links.go` (remove now-unused locals)

**Files:**
- Modify: `core/block/editor/smartblock/links.go`

- [ ] **Step 7.1: Check if `relationsToSkipLinksIndexing` and `relationsToFilterOutForLinks` still have references**

```bash
grep -rn "relationsToSkipLinksIndexing\|relationsToFilterOutForLinks" --include="*.go" core/ pkg/
```

If only their declarations in `links.go` come back, both are dead — they were used by the deleted `collectLinksFromRelations` and `injectLinksDetails`'s post-filter respectively. The post-filter logic now lives in `objectlink.filterPresentationOnly`.

- [ ] **Step 7.2: Remove the dead lists**

In `core/block/editor/smartblock/links.go`, delete:

```go
var relationsToSkipLinksIndexing = []domain.RelationKey{...}
var relationsToFilterOutForLinks = []domain.RelationKey{...}
```

Then simplify `injectLinksDetails` to drop the manual post-filter — it now relies on `FilterPresentationOnly` on the inside, but `injectLinksDetails` calls `DependentObjectIDs`, not `DependentObjectLinks`. Add `FilterPresentationOnly: true` to the flag set:

```go
func (sb *smartBlock) injectLinksDetails(s *state.State) {
	links := objectlink.DependentObjectIDs(s, sb.Space(), sb.formatFetcher, objectlink.Flags{
		Blocks:                   true,
		Details:                  true,
		Relations:                sb.includeRelationObjectsAsDependents,
		Types:                    false,
		Collection:               !internalflag.NewFromState(s).Has(model.InternalFlag_collectionDontIndexLinks),
		DataviewBlockOnlyTarget:  true,
		NoSystemRelations:        true,
		NoHiddenBundledRelations: true,
		NoImages:                 false,
		RoundDateIdsToDay:        true,
		FilterPresentationOnly:   true,
	})
	links = slice.RemoveMut(links, sb.Id())
	s.SetLocalDetail(bundle.RelationKeyLinks, domain.StringList(links))
}
```

Remove now-unused imports (`slices`).

- [ ] **Step 7.3: Run smartblock tests + indexer tests**

```bash
go test ./core/block/editor/smartblock/ ./core/indexer/...
```

Expected: pass.

- [ ] **Step 7.4: Commit**

```bash
git add core/block/editor/smartblock/links.go
git commit -m "GO-7237 Centralize presentation-only filter in objectlink"
```

---

## Task 8: Add a collection-backlinks integration test

**Files:**
- Modify: `core/block/collection/service_test.go`

- [ ] **Step 8.1: Write the failing test**

In `core/block/collection/service_test.go`, append:

```go
func TestService_AddToCollection_PopulatesObjectBacklinks(t *testing.T) {
	// given a collection and one object
	fx := newFixture(t)
	collId := "coll1"
	objId := "obj1"

	fx.objectStore.AddObjects(t, fx.spaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:             domain.String(objId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		},
	})
	// (collection setup uses existing fixture helpers — see TestService_Add for pattern)

	// when
	err := fx.Add(nil, &pb.RpcObjectCollectionAddRequest{
		ContextId: collId,
		ObjectIds: []string{objId},
	})
	require.NoError(t, err)

	// flush the backlinks watcher synchronously
	fx.backlinksUpdater.FlushUpdates()

	// then
	details, err := fx.objectStore.SpaceIndex(fx.spaceId).GetDetails(objId)
	require.NoError(t, err)
	backlinks := details.GetStringList(bundle.RelationKeyBacklinks)
	assert.Contains(t, backlinks, collId, "object should have collection in its backlinks")
}
```

Adapt the fixture/helpers to match the existing `service_test.go` pattern in this file. If the existing fixture does not expose `backlinksUpdater`, add it.

- [ ] **Step 8.2: Run; expect FAIL or PASS**

```bash
go test ./core/block/collection/ -run TestService_AddToCollection_PopulatesObjectBacklinks -v
```

If the test PASSES, the integration path is already wired correctly thanks to Task 5; document the test as a regression guard and continue.

If it FAILS, debug: check that `fx.GetDocInfo` for the collection now returns `OutgoingLinks` containing `objId`, that the indexer is invoked, and that the backlinks watcher consumes the resulting update.

- [ ] **Step 8.3: Commit**

```bash
git add core/block/collection/service_test.go
git commit -m "GO-7237 Add regression test for collection-member backlinks"
```

---

## Task 9: Full repo verification

- [ ] **Step 9.1: Run the full test suite**

```bash
go test ./...
```

Expected: all green. If any unrelated tests fail, investigate before continuing.

- [ ] **Step 9.2: Run linters**

```bash
make lint
```

Expected: no new warnings. Fix anything introduced by this change.

- [ ] **Step 9.3: Confirm no dangling references**

```bash
grep -rn "collectOutgoingLinks\|collectLinksFromRelations" --include="*.go" .
grep -rn "smartblock\.OutgoingLink" --include="*.go" .
```

Expected: zero results for the first command. The second should only find the type alias declaration in `smartblock.go` (kept for backwards compat).

- [ ] **Step 9.4: Smoke-test the actual behavior in the running app**

Run a debug build of anytype-heart (developer build path is project-specific — see `docs/Build.md`). In a test space:
1. Create a Collection.
2. Add an existing Object to it.
3. Open the Object and verify the Collection now appears in its **Backlinks** relation.
4. Remove the Object from the Collection.
5. Verify the Collection disappears from the Object's backlinks.

Document the manual smoke test result in the PR description.

- [ ] **Step 9.5: Open PR**

```bash
gh pr create --title "GO-7237 Unify outgoing-links walker and restore collection backlinks" --body "$(cat <<'EOF'
## Summary
- Consolidate the two divergent outgoing-link walkers (`smartblock.collectOutgoingLinks` and `objectlink.DependentObjectIDs`) onto a shared visitor pipeline in `objectlink`.
- Add `objectlink.DependentObjectLinks` (attributed `[]OutgoingLink` output) sharing internals with `DependentObjectIDs`.
- Wire `getDocInfo` to the unified walker — restoring collection-member backlinks, inline dataview embed backlinks, bookmark backlinks, status/tag option backlinks, and date-object backlinks; and stops icon/picture/cover-only references from polluting backlinks.
- Spec: `docs/superpowers/specs/2026-04-29-unify-outgoing-links-walker-design.md`

## Test plan
- [x] `go test ./core/block/object/objectlink/...`
- [x] `go test ./core/block/editor/smartblock/...`
- [x] `go test ./core/indexer/...`
- [x] `go test ./core/block/collection/...`
- [x] Full `go test ./...`
- [x] `make lint`
- [x] Manual smoke test (collection backlink behavior — see PR description body)
EOF
)"
```

---

## Self-review checklist (run after writing the plan)

- [x] Spec coverage:
  - "Unify outgoing-links walker" — Tasks 2-5.
  - "Move OutgoingLink struct from smartblock to objectlink" — Task 1.
  - "Honour same flag preset that injectLinksDetails uses" — Task 5.
  - "Add FilterPresentationOnly" — Task 4.
  - "Delete collectOutgoingLinks / collectLinksFromRelations" — Task 6.
  - "Integration test for collection backlinks" — Task 8.
  - "All existing tests still pass" — Task 9.
- [x] No "TBD" / "TODO" / vague placeholder steps. Each step has either a code block or a concrete command + expected output.
- [x] Type consistency: `OutgoingLink{TargetID, SourceBlockID, RelationKey}` used identically in objectlink, smartblock alias, and tests. `Flags` field name `FilterPresentationOnly` consistent across Tasks 4, 5, 7.
- [x] Bite-sized: each step is one action; commits are per task.
- [x] Open question 1 (sentinel for collection store records) — resolved as "empty SourceBlockID + empty RelationKey", matched in Step 3.5 test.
- [x] Open question 2 (option a vs b for `DependentObjectIDs`) — resolved as option (b) via shared visitors in Task 2.
- [x] Open question 3 (FilterPresentationOnly as flag) — resolved as a flag, default off; explicitly enabled by `injectLinksDetails` and `getDocInfo` in Tasks 5/7.
