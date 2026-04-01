# Context-Aware GC Stage 2: All Objects — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the `createdInContext` GC mechanism (currently file-only) to all user-content objects, automatically archiving child objects when their creation-context parent is archived.

**Architecture:** Add `GCEligibleLayouts` whitelist to `core/domain/layout.go`; replace `FileLayouts` filters in `filegc.go` with the new whitelist; set `createdInContext`/`createdInContextRef` on objects created via `BlockLinkCreateWithObject`; remove the buggy `sessionCreatedLinks` skipBin path from smartblock; rename `FileGC` → `ObjectGC`.

**Tech Stack:** Go, testify, objectstore.StoreFixture, existing `filegc` package

**Linear:** GO-7152

---

## File Map

| File | Change |
|------|--------|
| `core/domain/layout.go` | Add `GCEligibleLayouts` slice |
| `core/files/filegc/filegc.go` | Replace `FileLayouts` filters → `GCEligibleLayouts`; broaden early-return guard; add per-layout skipBin override; rename interface/struct |
| `core/files/filegc/filegc_test.go` | Add new test cases for non-file objects and system layouts |
| `core/block/editor/smartblock/smartblock.go` | Remove `sessionCreatedLinks` field + tracking; simplify `performFileGC` to always use `skipBin=false`; update `fileGC` field reference to `objectGC` |
| `core/block/create.go` | After link block creation, set `createdInContext`/`createdInContextRef` on new object |
| `core/block/service.go` | Update `fileGC` field name and type to `ObjectGC` |
| `core/block/detailservice/set_details.go` | Update `fileGC` field reference |
| `core/block/delete.go` | Update `fileGC` field reference |
| `core/block/create_test.go` (new or existing) | Test `CreateLinkToTheNewObject` sets `createdInContext` |

---

## Task 1: Add `GCEligibleLayouts` to `core/domain/layout.go`

**Files:**
- Modify: `core/domain/layout.go`

- [ ] **Step 1: Write the failing test**

Create `core/domain/layout_test.go`:

```go
package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestGCEligibleLayouts_ContainsFileLayouts(t *testing.T) {
	for _, fl := range domain.FileLayouts {
		assert.Contains(t, domain.GCEligibleLayouts, fl, "FileLayouts entry %v must be in GCEligibleLayouts", fl)
	}
}

func TestGCEligibleLayouts_ExcludesSystemLayouts(t *testing.T) {
	systemLayouts := []model.ObjectTypeLayout{
		model.ObjectType_objectType,
		model.ObjectType_relation,
		model.ObjectType_relationOption,
		model.ObjectType_relationOptionsList,
		model.ObjectType_dashboard,
		model.ObjectType_space,
		model.ObjectType_spaceView,
		model.ObjectType_participant,
		model.ObjectType_date,
		model.ObjectType_chatDerived,
		model.ObjectType_discussion,
	}
	for _, sl := range systemLayouts {
		assert.NotContains(t, domain.GCEligibleLayouts, sl, "system layout %v must not be in GCEligibleLayouts", sl)
	}
}

func TestGCEligibleLayouts_ContainsUserContentLayouts(t *testing.T) {
	userLayouts := []model.ObjectTypeLayout{
		model.ObjectType_basic,
		model.ObjectType_profile,
		model.ObjectType_todo,
		model.ObjectType_set,
		model.ObjectType_note,
		model.ObjectType_bookmark,
		model.ObjectType_collection,
	}
	for _, ul := range userLayouts {
		assert.Contains(t, domain.GCEligibleLayouts, ul, "user layout %v must be in GCEligibleLayouts", ul)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./core/domain/... -run TestGCEligibleLayouts -v
```

Expected: FAIL — `domain.GCEligibleLayouts` undefined.

- [ ] **Step 3: Add `GCEligibleLayouts` to `core/domain/layout.go`**

```go
package domain

import "github.com/anyproto/anytype-heart/pkg/lib/pb/model"

var FileLayouts = []model.ObjectTypeLayout{
	model.ObjectType_file,
	model.ObjectType_image,
	model.ObjectType_video,
	model.ObjectType_audio,
	model.ObjectType_pdf,
}

var GCEligibleLayouts = []model.ObjectTypeLayout{
	// Files
	model.ObjectType_file,
	model.ObjectType_image,
	model.ObjectType_video,
	model.ObjectType_audio,
	model.ObjectType_pdf,
	// User content objects
	model.ObjectType_basic,
	model.ObjectType_profile,
	model.ObjectType_todo,
	model.ObjectType_set,
	model.ObjectType_note,
	model.ObjectType_bookmark,
	model.ObjectType_collection,
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./core/domain/... -run TestGCEligibleLayouts -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add core/domain/layout.go core/domain/layout_test.go
git commit -m "GO-7152 Add GCEligibleLayouts whitelist to domain/layout"
```

---

## Task 2: Add helper + write failing tests for non-file GC in `filegc_test.go`

**Files:**
- Modify: `core/files/filegc/filegc_test.go`

- [ ] **Step 1: Add `basicObject` helper and new test cases at the bottom of `filegc_test.go`**

First, add the helper function after the existing `deletedObject` helper:

```go
func basicObject(id, createdInContext string, backlinks []string) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:               domain.String(id),
		bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
		bundle.RelationKeyCreatedInContext: domain.String(createdInContext),
		bundle.RelationKeyBacklinks:        domain.StringList(backlinks),
	}
}

func systemObject(id, createdInContext string, backlinks []string) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:               domain.String(id),
		bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_participant)),
		bundle.RelationKeyCreatedInContext: domain.String(createdInContext),
		bundle.RelationKeyBacklinks:        domain.StringList(backlinks),
	}
}
```

Then add these test functions:

```go
// -- non-file object GC tests --

func TestCheckFilesOnObjectArchived_NonFileObject_ParentArchived_NoOtherBacklinks(t *testing.T) {
	// given: a basic (non-file) object was created inside parent
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, basicObject("child", "parent", []string{"parent"}))

	// when: parent is archived
	err := fx.CheckFilesOnObjectArchived(testSpaceId, "parent", true)

	// then: child is archived too
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"child"}, fx.archiver.archivedIds)
}

func TestCheckFilesOnObjectArchived_NonFileObject_ParentArchived_WithActiveBacklink(t *testing.T) {
	// given: child still referenced by another active object
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, regularObject("other"))
	fx.addObject(t, basicObject("child", "parent", []string{"parent", "other"}))

	// when
	err := fx.CheckFilesOnObjectArchived(testSpaceId, "parent", true)

	// then: child is kept because "other" is still active
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.archivedIds)
}

func TestCheckFilesOnObjectArchived_NonFileObject_Unarchive(t *testing.T) {
	// given: child was archived alongside parent
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:               domain.String("child"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyCreatedInContext: domain.String("parent"),
			bundle.RelationKeyBacklinks:        domain.StringList([]string{"parent"}),
			bundle.RelationKeyIsArchived:       domain.Bool(true),
		},
	})

	// when: parent is unarchived
	err := fx.CheckFilesOnObjectArchived(testSpaceId, "parent", false)

	// then: child is restored
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"child"}, fx.archiver.unarchivedIds)
}

func TestCheckFilesOnLinksRemoval_NonFileObject_SkipBinForcedFalse(t *testing.T) {
	// given: a basic object whose link is removed; caller requests skipBin=true
	fx := newFixture(t)
	fx.addObject(t, basicObject("child", "parent", []string{"parent"}))
	fx.participantProvider = &mockParticipantProvider{id: "user1"}
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:               domain.String("child"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyCreatedInContext: domain.String("parent"),
			bundle.RelationKeyBacklinks:        domain.StringList([]string{"parent"}),
			bundle.RelationKeyCreator:          domain.String("user1"),
		},
	})

	// when: caller requests skipBin=true (as chat service does for files)
	err := fx.CheckFilesOnLinksRemoval(testSpaceId, "parent", []string{"child"}, true, nil)

	// then: child is archived, NOT permanently deleted — skipBin overridden to false for non-files
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"child"}, fx.archiver.archivedIds)
	// objectDeleter must NOT have been called
}

func TestCheckFilesOnObjectArchived_SystemLayoutObject_NotGCd(t *testing.T) {
	// given: an object with a system layout (participant) has createdInContext set
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		systemObject("sysobj", "parent", []string{"parent"}),
	})

	// when: parent is archived
	err := fx.CheckFilesOnObjectArchived(testSpaceId, "parent", true)

	// then: system object is NOT touched
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.archivedIds)
}

func TestCheckFilesOnLinksRestored_NonFileObject_Restored(t *testing.T) {
	// given: basic object was GC'd (archived), undo re-adds the link
	fx := newFixture(t)
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:               domain.String("child"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyCreatedInContext: domain.String("page"),
			bundle.RelationKeyIsArchived:       domain.Bool(true),
		},
	})

	// when: link re-added via undo
	err := fx.CheckFilesOnLinksRestored(testSpaceId, "page", []string{"child"})

	// then: child is unarchived
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"child"}, fx.archiver.unarchivedIds)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./core/files/filegc/... -run "TestCheckFilesOnObjectArchived_NonFile|TestCheckFilesOnLinksRemoval_NonFile|TestCheckFilesOnObjectArchived_SystemLayout|TestCheckFilesOnLinksRestored_NonFile" -v
```

Expected: FAILs — non-file objects not matched by `FileLayouts` filter, system objects incorrectly included.

Note: `TestCheckFilesOnLinksRemoval_NonFileObject_SkipBinForcedFalse` also needs `mockParticipantProvider` — add it to the fixture helpers:

```go
type mockParticipantProvider struct{ id string }

func (m *mockParticipantProvider) MyParticipantId(_ string) string { return m.id }
```

And update `newFixture` to initialise the provider field:

```go
func newFixture(t *testing.T) *fixture {
	store := objectstore.NewStoreFixture(t)
	archiver := &mockArchiver{}
	gc := &fileGC{
		objectStore:         store,
		objectArchiver:      archiver,
		backlinksWatcher:    &noopFlusher{},
		componentCtx:        context.Background(),
		participantProvider: &mockParticipantProvider{},
	}
	return &fixture{
		fileGC:   gc,
		store:    store,
		archiver: archiver,
	}
}
```

- [ ] **Step 3: Commit the failing tests**

```bash
git add core/files/filegc/filegc_test.go
git commit -m "GO-7152 Add failing tests for non-file object GC"
```

---

## Task 3: Generalize GC queries and rename interface in `filegc.go`

**Files:**
- Modify: `core/files/filegc/filegc.go`

- [ ] **Step 1: Rename interface and struct; replace `FileLayouts` filters; update early-return guard; add per-layout skipBin override**

Replace the entire file with the following (key diffs explained inline):

**a) Rename interface and constructor:**

```go
// ObjectGC is the interface for the object garbage collector.
type ObjectGC interface {
	app.ComponentRunnable
	CheckFilesOnLinksRemoval(spaceId, contextId string, removedLinks []string, skipBin bool, onlyBlockIds []string) error
	CheckFilesOnObjectArchived(spaceId, objectId string, isArchived bool) error
	CheckFilesOnLinksRestored(spaceId, contextId string, addedLinks []string) error
}

// ...

func New() ObjectGC {
	return &fileGC{}
}
```

**b) Replace `makeFileLayouts()` with `makeGCEligibleLayouts()`:**

```go
func makeGCEligibleLayouts() []int64 {
	layouts := make([]int64, 0, len(domain.GCEligibleLayouts))
	for _, layout := range domain.GCEligibleLayouts {
		layouts = append(layouts, int64(layout))
	}
	return layouts
}
```

**c) In `CheckFilesOnLinksRemoval`: replace `makeFileLayouts()` → `makeGCEligibleLayouts()`; add per-layout skipBin override after the `len(activeBacklinks) > 0` check:**

```go
fileLayouts := makeGCEligibleLayouts()
// ... (rest of query unchanged, uses fileLayouts variable)

// Per-layout override: non-file objects must never be permanently deleted
shouldSkipBin := skipBin
layout := model.ObjectTypeLayout(int32(record.Details.GetInt64(bundle.RelationKeyResolvedLayout)))
if !slices.Contains(domain.FileLayouts, layout) {
	shouldSkipBin = false
}
if shouldSkipBin {
	// Additional safety: only permanently delete if the file was created by the current user.
	fileCreator := record.Details.GetString(bundle.RelationKeyCreator)
	myParticipantId := gc.participantProvider.MyParticipantId(spaceId)
	if fileCreator != myParticipantId {
		log.With("fileId", fileId).Debugf("file was created by another user - archiving instead of deleting")
		shouldSkipBin = false
	}
}
```

**d) In `CheckFilesOnObjectArchived`: broaden early-return guard from `FileLayouts` to `GCEligibleLayouts`:**

```go
if !slices.Contains(domain.GCEligibleLayouts, model.ObjectTypeLayout(int32(d.GetInt64(bundle.RelationKeyResolvedLayout)))) {
	// system/unsupported objects can't have GC-tracked children
	return nil
}
```

**e) In `archiveOrphanedFiles`, `restoreFilesOnUnarchive`, `CheckFilesOnLinksRestored`: replace `makeFileLayouts()` → `makeGCEligibleLayouts()` for the layout filter variable.**

- [ ] **Step 2: Run the new tests to verify they pass**

```bash
go test ./core/files/filegc/... -v
```

Expected: all tests PASS including the new non-file ones.

- [ ] **Step 3: Commit**

```bash
git add core/files/filegc/filegc.go
git commit -m "GO-7152 Generalize GC queries to GCEligibleLayouts, rename FileGC to ObjectGC"
```

---

## Task 4: Update all `fileGC` field references to `ObjectGC`

The interface rename from `FileGC` to `ObjectGC` must be propagated to all consumers.

**Files:**
- Modify: `core/block/service.go`
- Modify: `core/block/detailservice/set_details.go`
- Modify: `core/block/delete.go`
- Modify: `core/block/editor/smartblock/smartblock.go`

- [ ] **Step 1: Update `core/block/service.go`**

Find the field declaration and update the type:

```go
// Before:
fileGC filegc.FileGC

// After:
objectGC filegc.ObjectGC
```

Find the `Init` assignment (search for `fileGC =` in service.go):

```go
// Before:
s.fileGC = app.MustComponent[filegc.FileGC](a)

// After:
s.objectGC = app.MustComponent[filegc.ObjectGC](a)
```

- [ ] **Step 2: Update `core/block/detailservice/set_details.go`**

```go
// Before:
fileGC filegc.FileGC

// After:
objectGC filegc.ObjectGC
```

Update all references `s.fileGC.` → `s.objectGC.` in that file.

- [ ] **Step 3: Update `core/block/delete.go`**

```go
// Before:
s.fileGC.CheckFilesOnObjectArchived(...)

// After:
s.objectGC.CheckFilesOnObjectArchived(...)
```

- [ ] **Step 4: Update `core/block/editor/smartblock/smartblock.go`**

```go
// Before:
fileGC filegc.FileGC

// After:
objectGC filegc.ObjectGC
```

Update all `sb.fileGC.` references → `sb.objectGC.`.

- [ ] **Step 5: Verify it compiles**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add core/block/service.go core/block/detailservice/set_details.go core/block/delete.go core/block/editor/smartblock/smartblock.go
git commit -m "GO-7152 Rename fileGC field to objectGC throughout"
```

---

## Task 5: Remove `sessionCreatedLinks` and simplify `performFileGC` in `smartblock.go`

**Files:**
- Modify: `core/block/editor/smartblock/smartblock.go`

- [ ] **Step 1: Remove the `sessionCreatedLinks` field from the struct**

Find the struct definition (~line 275) and remove:

```go
// Remove these lines:
// sessionCreatedLinks tracks links added locally in this session.
// These are considered "session-created" and will be permanently deleted (skipBin=true) when removed.
// nil means object was never explicitly opened → safe default (archive all removals).
// empty map means object was opened but no local links added yet.
sessionCreatedLinks map[string]struct{}
```

- [ ] **Step 2: Remove `initSessionTracking` and its call site**

Remove the `initSessionTracking` method entirely:

```go
// Remove this method:
func (sb *smartBlock) initSessionTracking() {
    if sb.sessionCreatedLinks != nil {
        return
    }
    sb.sessionCreatedLinks = make(map[string]struct{})
}
```

Remove the call to it inside `RegisterSession`:

```go
func (sb *smartBlock) RegisterSession(ctx session.Context) {
    sb.sessions[ctx.ID()] = ctx
    // Remove: sb.initSessionTracking()
}
```

- [ ] **Step 3: Remove session link tracking in `Apply`**

Find the block in `Apply` (~line 870) that populates/deletes `sessionCreatedLinks` and remove it entirely. It looks like:

```go
// Remove this block:
if sb.sessionCreatedLinks != nil {
    for _, link := range addedLinks {
        sb.sessionCreatedLinks[link] = struct{}{}
    }
}
// ...
if sb.sessionCreatedLinks != nil {
    for _, link := range removedLinks {
        delete(sb.sessionCreatedLinks, link)
    }
}
```

- [ ] **Step 4: Simplify `performFileGC` to always use `skipBin=false`**

Replace the entire `performFileGC` method body with:

```go
func (sb *smartBlock) performFileGC(spaceId, contextId string, removedLinks []string) {
	if sb.objectGC == nil {
		return
	}
	if len(removedLinks) == 0 {
		return
	}
	if err := sb.objectGC.CheckFilesOnLinksRemoval(spaceId, contextId, removedLinks, false, nil); err != nil {
		log.With("objectId", contextId).Errorf("object gc on links removal failed: %v", err)
	}
}
```

- [ ] **Step 5: Update `restoreArchivedFilesOnLinksAdded` to use `objectGC`**

```go
func (sb *smartBlock) restoreArchivedFilesOnLinksAdded(spaceId, contextId string, addedLinks []string) {
	if sb.objectGC == nil {
		return
	}
	if err := sb.objectGC.CheckFilesOnLinksRestored(spaceId, contextId, addedLinks); err != nil {
		log.With("objectId", contextId).Errorf("object restore on links added failed: %v", err)
	}
}
```

- [ ] **Step 6: Verify it compiles and tests pass**

```bash
go build ./core/block/editor/smartblock/...
go test ./core/block/editor/smartblock/... -v
```

Expected: compiles and tests pass.

- [ ] **Step 7: Commit**

```bash
git add core/block/editor/smartblock/smartblock.go
git commit -m "GO-7152 Remove sessionCreatedLinks tracking, always archive on link removal in pages"
```

---

## Task 6: Set `createdInContext` in `CreateLinkToTheNewObject`

**Files:**
- Modify: `core/block/create.go`
- Create: `core/block/create_test.go`

The `cache.DoStateCtx` call in `CreateLinkToTheNewObject` depends on a live object cache, making it hard to unit test end-to-end. Instead, extract the `createdInContext` write into a small private method and test that in isolation.

- [ ] **Step 1: Write the failing test in `core/block/create_test.go`**

```go
package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/detailservice/mock_detailservice"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func TestService_setCreatedInContext_SetsFields(t *testing.T) {
	// given
	detailsSvc := mock_detailservice.NewMockService(t)
	svc := &Service{detailsService: detailsSvc}
	sctx := session.NewContext()

	var capturedDetails *domain.Details
	detailsSvc.EXPECT().
		ModifyDetails(mock.Anything, "obj1", mock.Anything).
		RunAndReturn(func(_ session.Context, _ string, modifier func(*domain.Details) (*domain.Details, error)) error {
			d := domain.NewDetails()
			result, err := modifier(d)
			capturedDetails = result
			return err
		})

	// when
	svc.setCreatedInContext(sctx, "obj1", "ctx1", "link1")

	// then
	require.NotNil(t, capturedDetails)
	assert.Equal(t, "ctx1", capturedDetails.GetString(bundle.RelationKeyCreatedInContext))
	assert.Equal(t, "link1", capturedDetails.GetString(bundle.RelationKeyCreatedInContextRef))
}

func TestService_setCreatedInContext_NoOpWhenContextIdEmpty(t *testing.T) {
	// given
	detailsSvc := mock_detailservice.NewMockService(t)
	svc := &Service{detailsService: detailsSvc}
	// detailsSvc.ModifyDetails must NOT be called

	// when
	svc.setCreatedInContext(nil, "obj1", "", "link1")

	// then: no mock calls expected — testify will fail the test if ModifyDetails is called unexpectedly
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./core/block/... -run TestService_setCreatedInContext -v
```

Expected: FAIL — `svc.setCreatedInContext` undefined.

- [ ] **Step 3: Add the helper method and call it from `CreateLinkToTheNewObject` in `core/block/create.go`**

Add the private helper (can go at the bottom of `create.go`):

```go
// setCreatedInContext sets createdInContext and createdInContextRef on an object.
// No-op when contextId or contextRef is empty.
func (s *Service) setCreatedInContext(sctx session.Context, objectId, contextId, contextRef string) {
	if contextId == "" || contextRef == "" {
		return
	}
	if err := s.detailsService.ModifyDetails(sctx, objectId, func(current *domain.Details) (*domain.Details, error) {
		current.SetString(bundle.RelationKeyCreatedInContext, contextId)
		current.SetString(bundle.RelationKeyCreatedInContextRef, contextRef)
		return current, nil
	}); err != nil {
		log.With("objectId", objectId).Warnf("set createdInContext: %v", err)
	}
}
```

Then call it at the end of `CreateLinkToTheNewObject`, after the `cache.DoStateCtx` block (so `linkID` is known):

```go
// After the cache.DoStateCtx block, before the final return:
if err == nil {
	s.setCreatedInContext(sctx, objectId, req.ContextId, linkID)
}
return
```

You will also need to add a `log` variable in `create.go` if not already present — check by searching for `var log` in that file. If absent, add:

```go
var log = logging.Logger("block")
```

(The `logging` package is already imported in `service.go` — check the import path used there and match it in `create.go`.)

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./core/block/... -run TestService_setCreatedInContext -v
```

Expected: PASS.

- [ ] **Step 5: Run all filegc tests to make sure nothing regressed**

```bash
go test ./core/files/filegc/... -v
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add core/block/create.go core/block/create_test.go
git commit -m "GO-7152 Set createdInContext on BlockLinkCreateWithObject"
```

---

## Task 7: Final verification

- [ ] **Step 1: Build the whole project**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 2: Run all affected test packages**

```bash
go test ./core/domain/... ./core/files/filegc/... ./core/block/... ./core/block/editor/smartblock/... -v 2>&1 | tail -50
```

Expected: all PASS.

- [ ] **Step 3: Commit if any loose ends remain, otherwise done**

```bash
git log --oneline -8
```

Verify the commit history shows all 6 commits from this plan.
