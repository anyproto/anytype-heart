# Lazy Cross-Space Objectstore Queries Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the startup disk spike by making the objectstore preload bounded and asynchronous, while giving correctness-critical cross-space callers an explicit wait that resolves against an authoritative space set.

**Architecture:** `preloadExistingObjectStores` becomes a bounded-concurrency warm-up launched in a background goroutine from `Run()` (never blocking it), iterating the union of `spaceStorage.AllSpaceIds()` and the objectstore filesystem dirs. `collectCrossSpace`/`listStores` no longer force a synchronous full preload (Bucket-1 callers self-heal). A new `ObjectStore.WaitStoresLoaded(ctx)` lets Bucket-2 (destructive/RPC) callers block until warm-up completes; the reconciler additionally per-space-scopes on `OpenedSpaceIds()`.

**Tech Stack:** Go, any-sync `app` component framework, testify, mockery.

**Spec:** `docs/superpowers/specs/2026-05-15-lazy-cross-space-queries-design.md`

---

## File Structure

- `pkg/lib/localstore/objectstore/service.go` — add `spaceIdsLister` iface, `loadedCh`, `spaceStorageLister`; `WaitStoresLoaded`; `authoritativeSpaceIds`; rewrite `preloadExistingObjectStores` (bounded) + `backgroundWarmUp`; launch from `Run`; drop preload call from `listStores`; add `WaitStoresLoaded` to `ObjectStore` interface.
- `pkg/lib/localstore/objectstore/fixture.go` — extract `newStoreFixture(t, extra...)`; add `NewStoreFixtureWithSpaceIds`.
- `pkg/lib/localstore/objectstore/service_warmup_test.go` — new tests.
- `core/files/fileobject/service.go` — `DeleteFileData`: wait before cross-space query.
- `core/files/fileoffloader/offloader.go` — `offloadAllFiles`, `offloadFileSafe`: wait before cross-space query.
- `core/block/template/templateimpl/impl.go` — `TemplateExportAll`: wait before cross-space query.
- `core/debug/service.go` — `DumpLocalstore`: wait before `IterateSpaceIndex`.
- `core/files/reconciler/reconciler.go` — `reconcileRemoteStorage`: wait + per-space scoping.
- `core/files/reconciler/reconciler_test.go` — regression test for per-space scoping.

---

### Task 1: Bounded async warm-up + `WaitStoresLoaded`

**Files:**
- Modify: `pkg/lib/localstore/objectstore/service.go`
- Modify: `pkg/lib/localstore/objectstore/fixture.go`
- Test: `pkg/lib/localstore/objectstore/service_warmup_test.go`

- [ ] **Step 1: Refactor fixture to allow extra components, add space-ids variant**

In `pkg/lib/localstore/objectstore/fixture.go`, replace the body of `NewStoreFixture` so the construction logic lives in a shared helper, and add a stub lister + variant constructor. Replace the existing `func NewStoreFixture(t testing.TB) *StoreFixture { ... }` with:

```go
type spaceIdsListerStub struct{ ids []string }

func (s *spaceIdsListerStub) AllSpaceIds() (ids []string, err error) { return s.ids, nil }
func (s *spaceIdsListerStub) Name() string                          { return "spaceIdsListerStub" }
func (s *spaceIdsListerStub) Init(a *app.App) error                 { return nil }

func NewStoreFixture(t testing.TB) *StoreFixture {
	return newStoreFixture(t)
}

func NewStoreFixtureWithSpaceIds(t testing.TB, ids []string) *StoreFixture {
	return newStoreFixture(t, &spaceIdsListerStub{ids: ids})
}

func newStoreFixture(t testing.TB, extra ...app.Component) *StoreFixture {
	ctx := context.Background()

	fullText := ftsearch.TantivyNew()
	testApp := &app.App{}

	testApp.Register(newWalletStub(t))
	err := fullText.Init(testApp)
	require.NoError(t, err)

	provider, err := anystoreprovider.NewInPath(t.TempDir())
	require.NoError(t, err)

	testApp.Register(provider)
	testApp.Register(fullText)
	testApp.Register(&stubDetailsFromId{})
	testApp.Register(&stubTechSpaceIdProvider{})
	for _, c := range extra {
		testApp.Register(c)
	}

	err = fullText.Init(testApp)
	require.NoError(t, err)
	err = fullText.Run(context.Background())
	require.NoError(t, err)

	ds := New()

	t.Cleanup(func() {
		err = fullText.Close(context.Background())
		if err != nil {
			t.Fatal("FOTAL:", err)
		}
		_ = ds.Close(context.Background())
	})

	err = ds.Init(testApp)
	require.NoError(t, err)

	err = ds.Run(ctx)
	require.NoError(t, err)

	return &StoreFixture{
		dsObjectStore: ds.(*dsObjectStore),
		FullText:      fullText,
	}
}
```

- [ ] **Step 2: Write the failing tests**

Create `pkg/lib/localstore/objectstore/service_warmup_test.go`:

```go
package objectstore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWaitStoresLoaded_OpensAuthoritativeSet(t *testing.T) {
	want := []string{"space-a", "space-b", "space-c"}
	fx := NewStoreFixtureWithSpaceIds(t, want)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, fx.WaitStoresLoaded(ctx))

	opened := map[string]struct{}{}
	for _, id := range fx.OpenedSpaceIds() {
		opened[id] = struct{}{}
	}
	for _, id := range want {
		_, ok := opened[id]
		assert.Truef(t, ok, "space %s should be opened by warm-up", id)
	}
}

func TestWaitStoresLoaded_ContextCancelled(t *testing.T) {
	s := &dsObjectStore{
		loadedCh:     make(chan struct{}), // never closed
		componentCtx: context.Background(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := s.WaitStoresLoaded(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestPreloadConcurrencyOne_LoadsAll(t *testing.T) {
	old := preloadConcurrency
	preloadConcurrency = 1
	t.Cleanup(func() { preloadConcurrency = old })

	want := []string{"s1", "s2", "s3", "s4", "s5"}
	fx := NewStoreFixtureWithSpaceIds(t, want)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, fx.WaitStoresLoaded(ctx))

	opened := map[string]struct{}{}
	for _, id := range fx.OpenedSpaceIds() {
		opened[id] = struct{}{}
	}
	for _, id := range want {
		_, ok := opened[id]
		assert.Truef(t, ok, "space %s should be opened with concurrency=1", id)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./pkg/lib/localstore/objectstore/ -run 'TestWaitStoresLoaded|TestPreloadConcurrencyOne' -count=1`
Expected: FAIL — compile errors (`fx.WaitStoresLoaded` undefined, `dsObjectStore.loadedCh` undefined, `preloadConcurrency` undefined, `NewStoreFixtureWithSpaceIds` undefined).

- [ ] **Step 4: Add the `spaceIdsLister` interface and struct fields**

In `pkg/lib/localstore/objectstore/service.go`, add this interface just above `type dsObjectStore struct {`:

```go
// spaceIdsLister enumerates every space the account has on disk, independent
// of the (derived, possibly-incomplete) objectstore index. Satisfied by
// space/spacecore/storage.ClientStorage. Resolved optionally: absent in
// lightweight test/migrator app assemblies.
type spaceIdsLister interface {
	AllSpaceIds() (ids []string, err error)
}
```

In the `dsObjectStore` struct, add these fields next to `spaceStoreDirsCheck sync.Once`:

```go
	spaceStorageLister spaceIdsLister
	loadedCh           chan struct{}
```

- [ ] **Step 5: Initialize `loadedCh` in `New()` and resolve the lister in `Init`**

In `New()`, add `loadedCh: make(chan struct{}),` to the returned `&dsObjectStore{...}` literal.

In `Init`, immediately before `return s.initCollections(s.componentCtx)`, add:

```go
	if lister, lerr := app.GetComponent[spaceIdsLister](a); lerr == nil {
		s.spaceStorageLister = lister
	}
```

- [ ] **Step 6: Add `authoritativeSpaceIds`, rewrite preload bounded, add warm-up + wait**

In `pkg/lib/localstore/objectstore/service.go`, replace the entire existing `func (s *dsObjectStore) preloadExistingObjectStores() error { ... }` with:

```go
const preloadConcurrencyDefault = 4

// preloadConcurrency caps parallel per-space store opens during warm-up.
// Variable (not const) so tests can pin it.
var preloadConcurrency = preloadConcurrencyDefault

// authoritativeSpaceIds returns the union of every space dir on disk
// (objectstore index dirs) and every spacecore storage space id. The latter
// is authoritative for "every space that could hold data" and is independent
// of the objectstore index; the former covers index dirs with no matching
// raw storage. Either source failing degrades coverage but never blocks.
func (s *dsObjectStore) authoritativeSpaceIds() []string {
	seen := map[string]struct{}{}
	var ids []string
	add := func(list []string) {
		for _, id := range list {
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	fsIds, err := s.anystoreProvider.ListSpaceIdsFromFilesystem()
	if err != nil {
		log.Error("list space ids from filesystem", zap.Error(err))
	}
	add(fsIds)
	if s.spaceStorageLister != nil {
		storageIds, serr := s.spaceStorageLister.AllSpaceIds()
		if serr != nil {
			log.Error("list space ids from spacestorage", zap.Error(serr))
		} else {
			add(storageIds)
		}
	}
	return ids
}

// preloadExistingObjectStores opens every authoritative space's per-space DB
// with bounded concurrency. It is the body of the background warm-up; it
// never runs on a query hot path and never blocks Run().
func (s *dsObjectStore) preloadExistingObjectStores() {
	s.spaceStoreDirsCheck.Do(func() {
		spaceIds := s.authoritativeSpaceIds()
		sem := make(chan struct{}, preloadConcurrency)
		var wg sync.WaitGroup
		for _, spaceId := range spaceIds {
			select {
			case <-s.componentCtx.Done():
				wg.Wait()
				return
			case sem <- struct{}{}:
			}
			wg.Add(1)
			go func(spaceId string) {
				defer wg.Done()
				defer func() { <-sem }()
				// SpaceIndex opens the per-space DB and, on success,
				// calls markSpaceIndexOpened (fires OnSpaceIndexOpened).
				// On Init error it returns an invalid store and the space
				// is left out of OpenedSpaceIds (intended).
				s.SpaceIndex(spaceId)
			}(spaceId)
		}
		wg.Wait()
	})
}

// backgroundWarmUp runs the bounded preload and signals completion. Launched
// as a goroutine from Run so component startup is never blocked.
func (s *dsObjectStore) backgroundWarmUp() {
	defer close(s.loadedCh)
	s.preloadExistingObjectStores()
}

// WaitStoresLoaded blocks until the background warm-up has opened every
// authoritative-set store, or ctx / the component context is done. Safe to
// call from any non-Run goroutine. Designed to be extended later to also
// await per-space indexation.
func (s *dsObjectStore) WaitStoresLoaded(ctx context.Context) error {
	select {
	case <-s.loadedCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.componentCtx.Done():
		return s.componentCtx.Err()
	}
}
```

- [ ] **Step 7: Launch warm-up from `Run`, drop preload from `listStores`**

In `func (s *dsObjectStore) Run(ctx context.Context) error`, replace the trailing:

```go
	s.Store = store

	return err
```

with:

```go
	s.Store = store

	go s.backgroundWarmUp()

	return nil
```

Replace the entire `func (s *dsObjectStore) listStores() []spaceindex.Store { ... }` with:

```go
func (s *dsObjectStore) listStores() []spaceindex.Store {
	s.lock.Lock()
	stores := make([]spaceindex.Store, 0, len(s.spaceIndexes))
	for _, store := range s.spaceIndexes {
		stores = append(stores, store)
	}
	s.lock.Unlock()
	return stores
}
```

- [ ] **Step 8: Add `WaitStoresLoaded` to the `ObjectStore` interface**

In `pkg/lib/localstore/objectstore/service.go`, in the `ObjectStore` interface, immediately after the `OpenedSpaceIds() []string` line, add:

```go
	// WaitStoresLoaded blocks until the background warm-up has opened every
	// space in the authoritative set, or ctx is done. Lazy/Bucket-1 callers
	// must not call this; correctness-critical callers must.
	WaitStoresLoaded(ctx context.Context) error
```

- [ ] **Step 9: Run tests to verify they pass**

Run: `go test ./pkg/lib/localstore/objectstore/ -run 'TestWaitStoresLoaded|TestPreloadConcurrencyOne' -count=1 -race`
Expected: PASS (3 tests).

- [ ] **Step 10: Verify whole package + dependents still build and existing tests pass**

Run: `go build ./... && go test ./pkg/lib/localstore/objectstore/ -count=1`
Expected: build OK; package tests PASS (existing tests unaffected — empty temp dir → warm-up closes immediately).

- [ ] **Step 11: Regenerate mocks for the changed interface**

Run: `grep -rl "MockObjectStore" --include=*.go . | head` then `make test-deps`
Expected: `make test-deps` regenerates any `ObjectStore` mocks with the new `WaitStoresLoaded` method; `go build ./...` still OK afterward.

- [ ] **Step 12: Commit**

```bash
git add pkg/lib/localstore/objectstore/ $(git diff --name-only | grep -i mock)
git commit -m "GO-7288 Bounded async objectstore warm-up + WaitStoresLoaded

preloadExistingObjectStores is now a bounded-concurrency background
warm-up launched from Run (never blocking it), driven by the union of
spacecore storage ids and objectstore fs dirs. listStores no longer
force-opens all spaces, so Bucket-1 cross-space queries are lazy and
self-heal. New ObjectStore.WaitStoresLoaded gates correctness-critical
callers."
```

---

### Task 2: Bucket-2 callers wait before cross-space queries

**Files:**
- Modify: `core/files/fileobject/service.go:704` (`DeleteFileData`)
- Modify: `core/files/fileoffloader/offloader.go:127,192` (`offloadAllFiles`, `offloadFileSafe`)
- Modify: `core/block/template/templateimpl/impl.go:400` (`TemplateExportAll`)
- Modify: `core/debug/service.go:284` (`DumpLocalstore`)

- [ ] **Step 1: `DeleteFileData` — wait before the cross-space reference check**

In `core/files/fileobject/service.go`, in `func (s *service) DeleteFileData(spaceId string, objectId string) error`, immediately before `records, err := s.objectStore.QueryCrossSpace(database.Query{` insert:

```go
	if err := s.objectStore.WaitStoresLoaded(s.componentCtx); err != nil {
		return fmt.Errorf("wait stores loaded: %w", err)
	}
```

(The function has no `ctx` parameter; `s.componentCtx` is already used later in this function for `CanDeleteFile`.)

- [ ] **Step 2: `offloadAllFiles` — wait before the not-pinned query**

In `core/files/fileoffloader/offloader.go`, in `func (s *service) offloadAllFiles(ctx context.Context, includeNotPinned bool) (err error)`, as the first statements inside `if !includeNotPinned {` and before `records, err := s.objectStore.QueryCrossSpace(database.Query{`, insert:

```go
		if werr := s.objectStore.WaitStoresLoaded(ctx); werr != nil {
			return fmt.Errorf("wait stores loaded: %w", werr)
		}
```

- [ ] **Step 3: `offloadFileSafe` — wait before the cross-space reference check**

In `core/files/fileoffloader/offloader.go`, in `func (s *service) offloadFileSafe(...) (uint64, error)`, immediately before `existingObjects, err := s.objectStore.QueryCrossSpace(database.Query{` insert:

```go
	if werr := s.objectStore.WaitStoresLoaded(ctx); werr != nil {
		return 0, fmt.Errorf("wait stores loaded: %w", werr)
	}
```

- [ ] **Step 4: `TemplateExportAll` — wait before the cross-space query**

In `core/block/template/templateimpl/impl.go`, in `func (s *service) TemplateExportAll(ctx context.Context, path string) (string, error)`, immediately before `records, err := s.store.QueryCrossSpace(database.Query{` insert:

```go
	if err := s.store.WaitStoresLoaded(ctx); err != nil {
		return "", fmt.Errorf("wait stores loaded: %w", err)
	}
```

- [ ] **Step 5: `DumpLocalstore` — wait before iterating space indexes**

In `core/debug/service.go`, in `func (d *debug) DumpLocalstore(ctx context.Context, spaceID string, objIds []string, path string) (filename string, err error)`, immediately before `err = d.store.IterateSpaceIndex(func(store spaceindex.Store) error {` insert:

```go
	if werr := d.store.WaitStoresLoaded(ctx); werr != nil {
		return "", fmt.Errorf("wait stores loaded: %w", werr)
	}
```

- [ ] **Step 6: Build and run affected package tests**

Run: `go build ./... && go test ./core/files/fileobject/... ./core/files/fileoffloader/... ./core/block/template/... ./core/debug/... -count=1`
Expected: build OK; tests PASS (test fixtures use `objectstore.StoreFixture`, whose warm-up over an empty temp dir closes `loadedCh` immediately, so `WaitStoresLoaded` returns nil instantly).

- [ ] **Step 7: Commit**

```bash
git add core/files/fileobject/service.go core/files/fileoffloader/offloader.go core/block/template/templateimpl/impl.go core/debug/service.go
git commit -m "GO-7288 Gate destructive cross-space queries on WaitStoresLoaded

DeleteFileData, offloadAllFiles, offloadFileSafe, TemplateExportAll and
DumpLocalstore now block on the bounded warm-up before their cross-space
query so they never act on a partial local view. All are user/GC/RPC
triggered, never on the startup hot path."
```

---

### Task 3: Reconciler per-space scoping (data-loss regression guard)

**Files:**
- Modify: `core/files/reconciler/reconciler.go` (`reconcileRemoteStorage`)
- Test: `core/files/reconciler/reconciler_test.go`

- [ ] **Step 1: Write the failing regression test**

Append to `core/files/reconciler/reconciler_test.go`:

```go
func TestReconcileRemoteStorage_SkipsUnaccountedSpace(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	require.NoError(t, fx.reconciler.Run(ctx))

	const (
		spaceA    = "spaceA"
		keptFile  = domain.FileId("bafkreigh2akiscaildcqabsyg3dfr6chu3fgpregiymsck7e7aqa4s52zy")
		orphanA   = domain.FileId("bafkreihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyaa")
		closedB   = "spaceB"
		fileInB   = domain.FileId("bafkreihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvybb")
	)

	// spaceA gets opened (StoreFixture.AddObjects opens its store) and
	// contains one file object referencing keptFile.
	fx.objectStore.AddObjects(t, spaceA, []objectstore.TestObject{
		{
			bundle.RelationKeyId:     domain.String("objA"),
			bundle.RelationKeySpaceId: domain.String(spaceA),
			bundle.RelationKeyFileId: domain.String(keptFile.String()),
		},
	})

	// Remote node reports three files: keptFile (kept), orphanA (orphan in
	// an accounted space -> deleted), fileInB (space never opened -> skipped).
	fx.fileStorage.EXPECT().IterateFiles(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, cb func(domain.FullFileId)) error {
			cb(domain.FullFileId{SpaceId: spaceA, FileId: keptFile})
			cb(domain.FullFileId{SpaceId: spaceA, FileId: orphanA})
			cb(domain.FullFileId{SpaceId: closedB, FileId: fileInB})
			return nil
		})

	deleted := map[domain.FileId]struct{}{}
	fx.fileSync.EXPECT().DeleteFile(mock.Anything, mock.Anything).RunAndReturn(
		func(_ string, id domain.FullFileId) error {
			deleted[id.FileId] = struct{}{}
			return nil
		}).Maybe()

	require.NoError(t, fx.reconciler.reconcileRemoteStorage(ctx))

	_, keptDeleted := deleted[keptFile]
	_, orphanDeleted := deleted[orphanA]
	_, bDeleted := deleted[fileInB]
	assert.False(t, keptDeleted, "referenced file must not be deleted")
	assert.True(t, orphanDeleted, "orphan in accounted space must be deleted")
	assert.False(t, bDeleted, "file in unaccounted (never-opened) space must be skipped")
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./core/files/reconciler/ -run TestReconcileRemoteStorage_SkipsUnaccountedSpace -count=1`
Expected: FAIL — `fileInB` is deleted (current code keys `haveIds` only by `FileId` and does not skip unaccounted spaces), so `bDeleted` is true.

- [ ] **Step 3: Add wait + per-space scoping to `reconcileRemoteStorage`**

In `core/files/reconciler/reconciler.go`, replace the entire `func (r *reconciler) reconcileRemoteStorage(ctx context.Context) error { ... }` with:

```go
func (r *reconciler) reconcileRemoteStorage(ctx context.Context) error {
	if err := r.objectStore.WaitStoresLoaded(ctx); err != nil {
		return fmt.Errorf("wait stores loaded: %w", err)
	}
	records, err := r.objectStore.QueryCrossSpace(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyFileId,
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
			},
			{
				RelationKey: bundle.RelationKeyIsDeleted,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Bool(true),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("query file objects: %w", err)
	}

	accounted := map[string]struct{}{}
	for _, spaceId := range r.objectStore.OpenedSpaceIds() {
		accounted[spaceId] = struct{}{}
	}

	haveIds := map[domain.FileId]struct{}{}
	for _, rec := range records {
		fileId := domain.FileId(rec.Details.GetString(bundle.RelationKeyFileId))
		if fileId.Valid() {
			haveIds[fileId] = struct{}{}
		}
	}

	err = r.fileStorage.IterateFiles(ctx, func(fileId domain.FullFileId) {
		// Never treat a file as orphaned if its space was not positively
		// accounted for (store not loaded): we cannot prove it is unreferenced.
		if _, ok := accounted[fileId.SpaceId]; !ok {
			return
		}
		if _, ok := haveIds[fileId.FileId]; !ok {
			log.Warn("file not found in local vault, enqueue deletion", zap.String("fileId", fileId.FileId.String()))
			err := r.fileSync.DeleteFile("", fileId)
			if err != nil {
				log.Error("add to deletion queue", zap.String("fileId", fileId.FileId.String()), zap.Error(err))
			}
			err = r.deletedFiles.Set(context.Background(), fileId.FileId.String(), struct{}{})
			if err != nil {
				log.Error("add to deleted files", zap.String("fileId", fileId.FileId.String()), zap.Error(err))
			}
		}
	})
	if err != nil {
		return fmt.Errorf("iterate files: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./core/files/reconciler/ -run TestReconcileRemoteStorage_SkipsUnaccountedSpace -count=1 -race`
Expected: PASS.

- [ ] **Step 5: Run the full reconciler suite**

Run: `go test ./core/files/reconciler/ -count=1`
Expected: PASS (no regressions in existing reconciler tests).

- [ ] **Step 6: Commit**

```bash
git add core/files/reconciler/reconciler.go core/files/reconciler/reconciler_test.go
git commit -m "GO-7288 Reconciler: wait + per-space scoping on OpenedSpaceIds

reconcileRemoteStorage now waits for warm-up and skips any remote file
whose space was not positively accounted for (store not loaded), so a
not-yet-loaded space's files are never mistaken for orphans and deleted
remotely. Regression test added."
```

---

### Task 4: Full verification

- [ ] **Step 1: Build everything**

Run: `go build ./...`
Expected: success.

- [ ] **Step 2: Run the touched suites under race**

Run: `go test -race -count=1 ./pkg/lib/localstore/objectstore/... ./core/subscription/crossspacesub/... ./core/files/reconciler/... ./core/files/fileoffloader/... ./core/files/fileobject/... ./core/block/template/... ./core/debug/...`
Expected: all PASS.

- [ ] **Step 3: Confirm no remaining synchronous full-preload on the query hot path**

Run: `grep -n "preloadExistingObjectStores\|backgroundWarmUp" pkg/lib/localstore/objectstore/service.go`
Expected: `preloadExistingObjectStores` is called only from `backgroundWarmUp`; `backgroundWarmUp` only from `Run` (as `go s.backgroundWarmUp()`); `listStores` no longer references it.

---

## Self-Review

**Spec coverage:**
- Bounded preload + async-from-Run → Task 1 (Steps 6–7). ✓
- Authoritative space set (R1, union spacecore∪fs) → Task 1 (Step 6 `authoritativeSpaceIds`). ✓
- Wait primitive → Task 1 (Step 6 `WaitStoresLoaded`, Step 8 interface). ✓
- `listStores`/`collectCrossSpace` lazy (Bucket-1 self-heal) → Task 1 (Step 7). ✓
- Bucket-2 callers opt into wait → Task 2 (DeleteFileData, offloadAllFiles, offloadFileSafe, TemplateExportAll, DumpLocalstore). ✓
- Reconciler wait + per-space scoping on `OpenedSpaceIds` → Task 3. ✓
- `Run()` non-blocking; app start unaffected → Task 1 Step 7 (`go s.backgroundWarmUp()`); Task 4 Step 3 check. ✓
- Error handling: per-space `Init` failure excluded from `OpenedSpaceIds` (existing `SpaceIndex` behavior, reused); `WaitStoresLoaded` returns ctx err; lister failure degrades (logged) — Task 1 Step 6. ✓
- Out of scope (R2 Synced-guard, FT epoch, remote-truth, indexation-wait) → not in plan, matches spec Non-goals. ✓

**Placeholder scan:** none — every code/command step contains concrete content.

**Type consistency:** `WaitStoresLoaded(ctx context.Context) error` identical in interface (Task 1 Step 8) and impl (Step 6) and all call sites (Task 2, Task 3). `preloadConcurrency` (var) defined Step 6, used in test Step 2. `loadedCh chan struct{}` defined Step 4, init Step 5, used Steps 6/test. `spaceIdsLister` defined Step 4, resolved Step 5, used Step 6 and fixture stub (Task 1 Step 1). `authoritativeSpaceIds`/`backgroundWarmUp` names consistent across Steps 6/7 and Task 4. Reconciler uses existing `r.objectStore.OpenedSpaceIds()` / new `WaitStoresLoaded` — both on the `ObjectStore` interface the reconciler already holds (`objectstore.ObjectStore`).
