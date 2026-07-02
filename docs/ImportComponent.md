# Import Component (`core/block/import`)

Reference for the current import subsystem, written to inform a from-scratch reimplementation.
It describes what the component does, the contracts callers rely on, the formats it supports, and
the full lifecycle of an import — including the parts a rewrite is free to change vs. the invariants
it must preserve.

> The `AGENTS.md` currently in this package is auto-generated and **inaccurate** (it invents APIs like
> `detectFormat`/`Process`/`io.Reader`-based `Init` that do not exist). Trust this document instead.

---

## 1. Responsibilities

The importer turns **external content** into **native Anytype objects** inside a target space. It is a
single `any-sync` app component (`CName = "importer"`, `core/block/import/importer.go:64`) that:

1. **Dispatches** an import request to a format-specific *converter* (Markdown, Notion, HTML, CSV, TXT,
   Protobuf/Anytype-export, Web), or accepts pre-built snapshots directly (`External`).
2. **Converts** the source into an in-memory list of *snapshots* (`[]*common.Snapshot`) — the
   format-neutral representation of an object (blocks + details + type + relations + collection store).
3. **Assigns identity**: maps every source ID to a new/derived/existing space object ID, and produces
   tree-create payloads for objects that need a new CRDT tree.
4. **Materializes** objects: creates/updates real smartblocks in parallel via a worker pool, installs
   bundled types/relations, uploads files, and rewrites all cross-object links to final IDs.
5. **Wraps up**: builds a root collection + widget for the import, emits progress/notifications/events,
   and triggers file sync.

It is the only write-path that ingests foreign data at object granularity, so it owns a lot of subtle
ID-remapping and dedup logic that the rest of the app depends on.

---

## 2. Public API (the contract others depend on)

### Component interface — `core/block/import/types.go:56`

```go
type Importer interface {
    app.Component
    Import(ctx context.Context, req *ImportRequest) *ImportResponse
    ListImports(req *pb.RpcObjectImportListRequest) ([]*pb.RpcObjectImportListImportResponse, error)
    ImportWeb(ctx context.Context, req *ImportRequest) (string, *domain.Details, error)
    ValidateNotionToken(ctx, *pb.RpcObjectImportNotionValidateTokenRequest) (…ErrorCode, error)
}
```

Request/response wrappers (Go-only, not proto) — `types.go:27`:

```go
type ImportRequest struct {
    *pb.RpcObjectImportRequest        // the wire request (params, type, mode, spaceId, snapshots…)
    Origin           objectorigin.ObjectOrigin // provenance: user import vs. usecase/migration
    Progress         process.Progress          // caller-supplied progress (nil ⇒ importer creates one)
    SendNotification bool                       // emit a completion Notification
    IsSync           bool                       // block until done vs. fire-and-forget goroutine
}
type ImportResponse struct {
    RootCollectionId string
    RootWidgetLayout model.BlockContentWidgetLayout
    ProcessId        string
    ObjectsCount     int64
    Err              error
}
```

**Sync vs async** (`importer.go:165`): `IsSync=true` runs inline and returns a populated
`*ImportResponse`; `IsSync=false` spawns `conc.Go` and returns `nil` — the real outcome is delivered
later via a Notification + an `EventImportFinish` broadcast.

- The async goroutine runs on the importer's **`componentCtx`, not the `ctx` passed to `Import`**
  (`importer.go:170`). So a caller cancelling its own context has **no effect** on an async import — the
  only way to cancel it is through its `Progress` (`ProcessCancel`); `componentCtx` is cancelled only on
  component `Close` (app shutdown). The passed `ctx` matters only for the inline `IsSync=true` path.
- The async goroutine is **fire-and-forget**: nothing tracks it, and `Close` does not drain in-flight
  imports (`importer.go:157`). At shutdown, cancelling `componentCtx` aborts writes mid-flight (leaving
  partial objects — see §6) and can race component teardown. `conc.Go` also only logs a recovered panic
  when its value is an `error`; other panic values are swallowed silently.

### gRPC surface — handlers in `core/object.go`

| RPC | Handler | Delegates to |
|---|---|---|
| `ObjectImport` | `core/object.go:637` | `Importer.Import` — always `IsSync=false`, `SendNotification=true`, `Origin=Import(req.Type)` |
| `ObjectImportList` | `core/object.go:650` | `Importer.ListImports` |
| `ObjectImportNotionValidateToken` | `core/object.go:668` | `Importer.ValidateNotionToken` |
| `ObjectImportUseCase` | `core/object.go:701` | **`builtinobjects`**, *not* the importer directly |
| `ObjectImportExperience` | `core/object.go:723` | **`builtinobjects`**, *not* the importer directly |

### Wire request — `pb/protos/commands.proto:3165`

```proto
message Request {
    string spaceId = 14;
    oneof params {                     // one format-specific param block
        NotionParams   notionParams   = 1;   // { apiKey }
        BookmarksParams bookmarksParams = 2;  // { url }  — Web import, internal use
        MarkdownParams markdownParams = 3;    // { path[], createDirectoryPages, includePropertiesAsBlock, noCollection }
        HtmlParams     htmlParams     = 4;    // { path[] }
        TxtParams      txtParams      = 5;    // { path[] }
        PbParams       pbParams       = 6;    // { path[], noCollection, collectionTitle, importType(SPACE|EXPERIENCE) }
        CsvParams      csvParams      = 7;    // { path[], mode(COLLECTION|TABLE), useFirstRowForRelations, delimiter, transpose }
    }
    repeated Snapshot snapshots = 8;   // for type=External (external developers)
    bool   updateExistingObjects = 9;  // dedup against existing objects by source path
    anytype.model.Import.Type type = 10;
    Mode   mode = 11;                  // ALL_OR_NOTHING | IGNORE_ERRORS
    bool   noProgress = 12;
    bool   isMigration = 13;
    bool   isNewSpace = 15;
}
```

`model.Import.Type` (`models.proto:1237`): `Notion=0, Markdown=1, External=2, Pb=3, Html=4, Txt=5,
Csv=6, Obsidian=7`. There is **no `Web` type** — web/bookmark import is reached only via `ImportWeb`
(keyed internally by `web.Name`, `processor.go:486`), and `Obsidian` is an alias for the Markdown
converter (`importer.go:146`).

> The proto response fields (`collectionId`, `objectsCount`, `error`) are all marked **deprecated** — the
> async model means results now flow through notifications/events, not the RPC reply. In fact the
> `ObjectImport` handler returns a **fully empty** `RpcObjectImportResponse` (not even `Error` is set,
> `core/object.go:637`) and never blocks. For an async import the **only** error signal is
> `NotificationImport.ErrorCode`; the `EventImportFinish` broadcast (`RootCollectionID`, `ObjectsCount`,
> `ImportType`) carries no error/status.

**Per-flag effects** (each flag's real, load-bearing behavior — not just its presence):

| Flag | Effect | Read at |
|---|---|---|
| `mode` | `ALL_OR_NOTHING` vs `IGNORE_ERRORS` — but applied with **different predicates per phase** (§6) | `error.go:122`, `processor.go:253/385/510` |
| `updateExistingObjects` | Enables extra dedup match on `sourceFilePath` (`getExistingObject`) — the re-import identity switch | `existingobject.go:86`, `processor.go:295` |
| `noProgress` | Uses a `NewNoOp` progress (not `Notificationable`) → **suppresses notifications even if `SendNotification=true`** | `processor.go:97` |
| `isMigration` | Migration progress bar model; **PB-only**: remaps the archive profile onto the account's own profile id (`getIDForUserProfile`) | `processor.go:90`, `pb/converter.go:391` |
| `isNewSpace` | **PB-only**: import the Workspace snapshot; suppress the gallery root collection. **Ignored by `SpaceImport`** (uses `noCollection` instead) | `pb/converter.go:449`, `pb/gallery.go:36`, `pb/space.go:31` |
| `noCollection` (param) | Suppress the root collection (PB/Markdown/CSV) → also no root widget | `pb/space.go:33`, `processor.go:438` |
| `snapshots` (External) | Caller-supplied snapshots; **cannot convey `SbType`** — see the External caveat in §9 | `processor.go:114` |
| `spaceId` | Required by `Execute` (empty → error); **not** validated by `ExecuteWebImport` | `processor.go:59` |

`ValidateNotionToken` returns `UNAUTHORIZED/FORBIDDEN/SERVICE_UNAVAILABLE/ACCOUNT_IS_NOT_RUNNING` codes
(the last when the app is not running, `core/object.go:692`). `ListImports` is **lossy/legacy**: it
returns one entry per registered converter (8, incl. the Obsidian→Markdown alias) but the
`RpcObjectImportListImportResponseType` enum only defines Notion/Markdown/Html/Txt, so Pb/Csv/web/Obsidian
all collapse to `Notion(0)` (`importer.go:200`).

### Internal callers (the full set)

Only three components depend on the `Importer` interface:

1. **gRPC** (`core/object.go`) — user-initiated imports; async, notification-driven, any format.
2. **`util/builtinobjects`** (`builtinobjects.go:490`, inside `importArchive`) — onboarding/use-case
   templates, gallery/experience import, and migration dashboard. **Synchronous**, `Origin=Usecase()`,
   `Mode=ALL_OR_NOTHING`, `IsMigration=true`, format `Import_Pb` (or `Import_Markdown` for AI
   experiences), `NoCollection=true`. Reached from `CreateObjectsForUseCase` (new-space creation,
   `core/block/create.go:205`), `CreateObjectsForExperience` (gallery), and `InjectMigrationDashboard`
   (`core/application/account_create_from_export.go:124`).
   - Use-case archives are **`//go:embed data/*.zip`** and mapped by use-case enum; `inject` writes the
     zip to a temp file, imports it synchronously, then (because `NoCollection` suppresses the importer's
     own widget/collection) reads the archive `profile` and **creates widgets + sets homepage/name/icon
     itself** (`inject`→`setWorkspaceSettings`). `CreateObjectsForExperience` takes a local path or
     **downloads** the zip from a whitelisted gallery URL with a byte-counting progress reader (30%
     download / 10% copy split). AI experiences use `Import_Markdown`, read `manifest.json` for the
     dashboard page, and add a "Grid" view to every non-system type (`addTypesView`).
3. **`core/block/bookmark/bookmarkimporter`** — a thin decorator exposing only `ImportWeb`, used to
   create bookmark objects from a URL (synchronous).

---

## 3. Supported formats

All local-file formats accept `path[]` (a dir, a `.zip`, or individual files). Each converter is
registered by `Name()` and dispatched by `req.Type.String()` (`importer.go:128`).

| Format | Type | Input | Network | Root collection | Notes |
|---|---|---|---|---|---|
| **Markdown** | `Markdown` (+`Obsidian` alias) | dir/zip/`.md` | – | Yes (**`Link` widget**; single dir-page → `Tree` and no collection) | Heaviest. 9-stage in-memory mutation pass; reads Anytype JSON schemas; YAML front-matter → relations |
| **Notion** | `Notion` | API token | **Yes** | Yes | Full `POST /search` crawl; 10-worker page fetch, 5-worker file download to temp dir |
| **Protobuf** | `Pb` | dir/zip (Anytype export) | – | Yes (`SpaceImport` or gallery `GalleryImport`) | Re-materializes each snapshot as a `state.Doc` to remap IDs; sub-object → real-object migration |
| **CSV** | `Csv` | dir/zip/`.csv` | – | Yes | Two strategies: **Table** (1 table block/file) or **Collection** (1 object/row + relation/column). Limits: 10 cols, 1000 rows → `ErrCsvLimitExceeded` |
| **HTML** | `Html` | dir/zip/`.html` | – | Yes | `anymark.HTMLToBlocks`; local file/img copied to temp dir |
| **TXT** | `Txt` | dir/zip/`.txt` | – | Yes | Parsed as markdown; 1 basic page/file |
| **Web** | (via `ImportWeb`) | single URL | **Yes** | **No** | Only `/wiki/<slug>` parser registered; returns exactly one Bookmark snapshot |
| **External** | `External` | `snapshots[]` on the request | – | No | Caller supplies snapshots directly; skips converters entirely (`processor.go:114`) |

All non-Markdown converters use a `CompactList` root widget; Markdown uses a `Link` widget (or `Tree`).

### PB (Anytype-export) specifics

The PB converter carries most of the import system's identity/dedup subtlety and diverges sharply by
import type — a rewrite must replicate both providers:

- **Root-collection provider** is chosen by import type (`pb/collectionprovider.go:18`):
  `EXPERIENCE → GalleryImport`, else `SpaceImport`.
  - **`SpaceImport`** (`pb/space.go`): if the export has a widget snapshot, membership is derived from
    the widget + dashboard/favorites; otherwise **every** object except sub-objects/types/relations/
    options is added. Appends the import date to the name (`WithAddDate`). Gated by the `noCollection`
    param (ignores `isNewSpace`).
  - **`GalleryImport`** (`pb/gallery.go`): returns **no collection when `isNewSpace`**; names it after
    `CollectionTitle`; reuses the space icon; includes only `Page` objects; and if the widget references
    non-predefined objects, builds a second `"<name>'s Widgets"` collection and rewrites the widget
    snapshot to a single link block pointing at it.
- **What gets imported is gated** (`shouldImportSnapshot`, `converter.go:448`): the **Workspace**
  snapshot only if `isNewSpace`; **Widget** snapshots only if the export came from this same account
  (`profile.Address == accountId`) or it is an EXPERIENCE import; all other objects always. The archive
  profile maps onto the real `ProfileObjectId()` **only when `isMigration`** (`getIDForUserProfile`).
- **`normalizeSnapshot`** (`converter.go:307`) rewrites legacy `SubObject` snapshots into real
  ObjectType/Relation/RelationOption snapshots (setting `SourceObject` to the bundled URL), fixes unknown
  numeric `SbType`, assigns a random profile `iconOption` 1–16, and resolves file paths.
- **`updateDetails`** (`converter.go:498`) strips all `LocalAndDerivedRelationKeys` except a preserve-list
  (favorite/archived/created/modified/id/resolvedLayout); `injectImportDetails` copies the source `Id`
  into `RelationKeyOldAnytypeID` (the **primary dedup key**) and sets `SourceFilePath = spaceId/id`.

**`NoCollection` cascade:** all `builtinobjects` imports set `NoCollection=true`, so `SpaceImport`
short-circuits → no `RootObjectID` → `addRootCollectionWidget` returns early → empty `RootCollectionId`.
That caller therefore does **not** rely on the importer's collection/widget and builds widgets + homepage
itself afterward from the archive profile.

---

## 4. The converter contract

Every format implements one small interface — `core/block/import/common/types.go:25`:

```go
type Converter interface {
    GetSnapshots(ctx, req *pb.RpcObjectImportRequest, progress process.Progress) (*Response, *ConvertError)
    Name() string
}
```

`GetSnapshots` **fully materializes** its output in memory (no streaming out). It returns:

```go
type Response struct {
    Snapshots            []*Snapshot                     // every object to create
    RootObjectID         string                          // usually the root-collection snapshot id
    RootObjectWidgetType model.BlockContentWidgetLayout  // how to render the root widget
    TypesCreated         []domain.TypeKey
}

type Snapshot struct {
    Id       string          // synthetic source id (often a uuid)
    FileName string          // source-derived name
    Snapshot *SnapshotModel
}
type SnapshotModel struct {
    SbType   coresb.SmartBlockType
    LogHeads map[string]string
    Data     *StateSnapshot  // the real payload ↓
    FileKeys []*pb.ChangeFileKeys
}
type StateSnapshot struct {          // domain mirror of proto model.SmartBlockSnapshotBase
    Blocks, Details, ObjectTypes,    // the minimum a converter fills
    Collections,                     // set for collection objects (store of member ids)
    ExtraRelations, RelationLinks, RemovedCollectionKeys,
    Key, OriginalCreatedTimestamp, FileInfo, FileKeys …
}
```

Proto↔model conversion helpers live in the same file (`ToProto`, `NewStateSnapshotFromProto`,
`NewSnapshotModelFromProto` + a `migrateAddMissingUniqueKey` shim).

### Source abstraction — `common/source/`

`GetSource(path)` (`source.go:32`) picks an impl by extension: `.zip`→`Zip`, single supported-ext
file→`File`, else `Directory` (`FilterableSource` adds `InitializeWithFilter` for selective subsets).

```go
type Source interface {
    Initialize(importPath string) error
    Iterate(cb func(name string, r io.ReadCloser) bool) error
    ProcessFile(name string, cb func(r io.ReadCloser) error) error
    CountFilesWithGivenExtensions(exts []string) int
    IsRootFile(name string) bool
    Close()
}
```

**The Source layer is streaming/callback-based and never buffers whole files.** `Zip`/`Directory` index
only entry *headers/paths* at `Initialize`; content flows through short-lived `io.ReadCloser`s closed
right after each callback. Archive entries that need a real filesystem path (icons, embedded files) are
**spilled to a temp file** via a buffered writer (`filenameprovider.go:44`), not held in memory.

> Consequence for a rewrite: peak memory is driven by what *converters retain* (all snapshots, plus
> whole-file `io.ReadAll` buffers), **not** by the source layer. See §8.

---

## 5. Lifecycle of an import operation

`Import` → `importObjects` → `NewProcessor(deps, req).Execute(ctx)` (`processor.go:58`). The processor
is single-use (`importProcessor` holds the per-run maps).

```
Execute(ctx)
 ├─ validate SpaceId
 ├─ metrics: ImportStartedEvent
 ├─ setupProgressBar()                     ── unless caller passed a Progress
 │     • NoProgress → process.NewNoOp
 │     • SendNotification → NewNotificationProcess  else NewProgress
 │     • register with blockService.ProcessAdd  (shows in UI, cancelable)
 ├─ branch:
 │   • type==External → handleExternalImport   (snapshots already provided)
 │   • else           → handleBuiltinConverterImport
 │        └─ converter.GetSnapshots(ctx, req, progress)   ── format-specific, §4
 │           (shouldReturnError gates fatal vs. partial per Mode)
 ├─ initConversionFields()   ── alloc oldIDToNew / createPayloads / relationKeysToFormat
 ├─ createObjects(ctx)       ── TWO PHASES ↓
 └─ defer finalize(ctx, importId)
        finishImportProcess()  → Progress.Finish / FinishWithNotification
        sendFileEvents()       → fileSync.SendImportEvents (+ always ClearImportEvents)
        addRootCollectionWidget(ctx) → CreateWidgetBlock pointing at RootCollectionId
        metrics: ImportFinishedEvent
        sendImportFinishEvent() → broadcast EventImportFinish (async imports only)
```

### `createObjects` — the two phases (`processor.go:206`)

**Phase 1 — ID assignment (`assignIdsToAllObjects`, single-threaded, order matters):**

1. Normal snapshots first (skipping relation-options and file-objects).
2. **Relation options** next (after relations, so all relation keys are known) — their `relationKey`
   is remapped first.
3. **File objects last** — their object-relations in details are remapped to final IDs *before* upload.

For each snapshot, `processSnapshot`:
- `preloadFileKeys` → `objectStore.AddFileKeys`.
- `id, payload = idProvider.GetIDAndPayload(ctx, spaceId, snapshot, now, UpdateExistingObjects, origin)`.
- record `oldIDToNew[snapshot.Id] = id`; for **new** objects also record
  `createPayloads[id] = payload` (only when `payload.RootRawChange != nil && !isBundled`).
- `extractInternalKey` maps old→new unique keys too.

> **Phase 1 is not just bookkeeping — it performs real, committed writes, single-threaded.** For
> `FileObject`/legacy `File`, `GetIDAndPayload` **uploads the file synchronously** inside ID assignment
> (`objectid/fileobject.go:37`, `oldfile.go:41`); `preloadFileKeys` writes encryption keys to the store.
> Consequences: (a) file-heavy imports are bounded by Phase 1, **not** the 10-worker pool; (b) the file's
> real id (returned by the upload) must be in `oldIDToNew` before Phase 2 rewrites links to it — the
> actual reason file objects go last; (c) these writes are already persisted if Phase 2 later aborts.
>
> **Global barrier invariant:** all of Phase 1 must complete before **any** Phase 2 worker starts.
> Workers rewrite links/relations against the fully-populated `oldIDToNew`; an id not yet in the map
> becomes `_missing_object`. A streaming convert→assign→create pipeline (floated in §9) must keep this
> barrier or add a second link-resolution pass, or it silently drops links to later-assigned objects.

**Phase 2 — creation (parallel):** a single shared `*DataObject` (read-only maps) is fed to a
`workerpool` of **10** workers (or 1 if fewer snapshots). `addWork` wraps every snapshot in a
`creator.Task`; `readResults` drains results, stepping progress per object and collecting
`details[newID]`. Channels are **unbuffered** → natural backpressure (≤ ~10 objects in flight).

`RootCollectionId = oldIDToNew[Response.RootObjectID]`. `ObjectsCount` excludes the root collection.

### Object creation per snapshot — `objectcreator.Service.Create` (`objectcreator/objectcreator.go:84`)

1. `newID := oldIDToNew[sn.Id]`; legacy `File` type is already created in Phase 1 → early return.
2. `injectImportDetails` (origin, import type, created/modified timestamps), then build state:
   `state.NewDocFromSnapshot(newID, sn.Snapshot.ToProto())` — note `ToProto` is on `SnapshotModel`,
   not `Snapshot`.
3. **Remap in state**: `UpdateObjectIDsInRelations` (skipped for FileObject — already pre-mapped) +
   `UpdateLinksToObjects` (unresolved targets → `_missing_object`, bundled targets preserved) +
   `updateKeys`.
4. Early exits for **Workspace** (`setWorkspaceDetails`) and **Widget** (`updateWidgetObject`).
5. Sync file relations in details (`ModifyLinkedFilesInDetails` → `FileRelationSyncer`).
6. **Install bundled deps**: `installBundledRelationsAndTypes` → `InstallBundledObjects` for referenced
   bundled relations/types. This runs **inside each parallel worker's `Create`**, so up to 10 workers may
   install overlapping bundled ids concurrently, and an object may reference a bundled dep still
   mid-install → `InstallBundledObjects` must be concurrency-safe and callers must not assume the dep is
   already indexed. Install failures are logged, not fatal.
7. **Create vs update**: if a create payload exists → `CreateTreeObjectWithPayload`; else if updatable →
   reset the existing object to the snapshot state via `history.ResetToVersion` (a **full overwrite** that
   discards local edits), guarded by `Revision` (skip if the stored revision is newer — this is what
   protects bundled objects; ordinary imported objects have `Revision:0` and are always overwritten).
   Collections are the exception — their member list is **unioned** with the existing one, so re-import
   accretes members. The **`ErrTreeExists` fallback** (read the existing object instead of failing) is a
   correctness feature: a deduped id can pass the store-index check yet already exist as a tree (index
   lag / prior run), and this keeps re-import idempotent.
8. Set favorite/archived (via `detailsService`, not the snapshot state); then **`syncFilesAndLinks`**.
   Syncer closures are **collected while holding the object lock but executed only after it is released**
   (`objectcreator.go:443`/`:458`) — mandatory, because each syncer re-enters the same object via
   `cache.Do` to write `TargetObjectId` (`syncer/file.go:93`); running one under the outer lock
   self-deadlocks. Returns `(details, newID, err)`.

> **Re-import idempotency (summary).** Derived objects (relations/types/options) always dedup and are
> truly idempotent. Pages/templates dedup only when `updateExistingObjects=true` (match on
> `oldAnytypeID`/unique key, then `sourceFilePath`); with it **false**, each run mints a **new random tree
> id** → duplicate pages. A dedup *match* does not merge — it overwrites (except collections, which union).

### ID strategies — `objectid/` (dispatched by `SbType`)

| SbType | Strategy | Result |
|---|---|---|
| Page, Template | `treeObject` | dedup, else new random tree payload |
| Relation, ObjectType, RelationOption, ProfilePage | `derivedObject` | dedup (always), else deterministic derive from unique key |
| FileObject | `fileObject` | `treeObject` id, then **upload** file → real id (empty payload) |
| File (legacy) | `oldFile` | upload / `CreateFromImport` → id, created in Phase 1 |
| Workspace / Widget | fixed | `spc.DerivedIDs().Workspace` / `.Widgets` |
| Participant | `participant` | `NewParticipantId(spaceID, identity)` |

Dedup (`existingObject.go`) matches on `oldAnytypeID` / unique key, then (if `updateExisting`) on
`sourceFilePath`, then type-specific fallbacks (relation format, option name+key, type name). **File
objects are processed last precisely because their own id comes from an upload and their details point
at other imported objects** — deferring them guarantees `oldIDToNew` is complete before their relations
are rewritten and before upload.

### Post-creation syncers — `common/syncer/`

`Factory.GetSyncer(block)` → `FileSyncer` (file blocks: migrate/upload → `TargetObjectId`),
`BookmarkSyncer` (fetch URL → bookmark object), `IconSyncer` (text-block `IconImage`). Separately,
`FileRelationSyncer` remaps a file id inside a *detail* relation. All wrap failures in
`common.ErrFileLoad`; all skip ids already created in this import (`newIdsSet`) or `_missing_object`.

### Object provenance & created-date sourcing

- **Origin is stamped on every object**: `injectImportDetails` writes `RelationKeyOrigin` and (for
  `import`/`usecase` origins) `RelationKeyImportType` into details before state creation
  (`objectcreator.go:202`). Downstream indexing keys off this. Note `handleExternalImport` **ignores the
  caller's `Origin`** and forces `Import(model.Import_External)` (`processor.go:130`).
- **Created/modified dates come from OS file metadata**, per-platform (`filetime.ExtractFileTimes`, via
  `GetCommonDetails`); **android returns `0,0`**. `injectImportDetails` deliberately refuses to fall back
  to `time.Now()` for the created date (it must match the tree header) and only warns if both are missing.

### The `ImportWeb` path is *not* `Execute`

`ImportWeb`/`ExecuteWebImport` (`processor.go:476`) is a distinct, stripped path: it parses the URL via
the `web` converter, runs `createObjects`, and returns `(firstSnapshotId, details, err)`. It does **not**
validate `SpaceId`, does **not** run `finalize` (no file events, root collection, widget, notification,
`EventImportFinish`, or metrics), ignores `req.Type`/`SendNotification`/`IsSync`, and returns a plain
error (no error-code mapping). The only caller, `bookmarkimporter`, invokes it with `ctx=nil` and an
**empty `SpaceId`** — a rewrite must keep `ImportWeb` tolerant of both.

---

## 6. Error handling & modes

`ConvertError` (`common/error.go:34`) accumulates `[]error` plus the request `Mode`:

- **`ALL_OR_NOTHING`** — any error aborts; `readResults` stops the pool on the first failing object.
- **`IGNORE_ERRORS`** — non-fatal errors are collected and the import continues; `RootCollectionId` is
  cleared if a result error remains.

**Mode is applied with different predicates per phase** — a rewrite must preserve each, not a single
rule: the converter and Phase-1 (ID-assignment) paths abort on **anything other than `IGNORE_ERRORS`**
(`processor.go:253/510`), while Phase-2 (`readResults`) and `ConvertError.ShouldAbortImport` abort only on
**`ALL_OR_NOTHING` exactly** (`processor.go:385`, `error.go:122`). These agree for the two defined values
but diverge for an out-of-range `mode`.

**Always-surfaced sentinels.** Two different mechanisms are easy to conflate: `shouldReturnError`
(`processor.go:509`) forces an error to the user **regardless of mode** for `ErrNotionServerExceedRateLimit`,
`ErrCsvLimitExceeded`, no-object, and `ErrCancel`. Separately, `isDefinedError` (`error.go:176`) only
selects *which* error `GetResultError` reports (its list also includes Notion-unavailable, `ErrFileLoad`,
`ErrPbNotAnyBlockFormat`, `ErrWrongHTMLFormat`) — being in that list does **not** by itself make an error
mode-independent.

**There is no rollback — failure and cancel are not transactional.** On abort (fatal error in
`ALL_OR_NOTHING`, or cancel) `readResults` stops the pool but every object already created in this run
**stays in the space**: committed trees, uploaded files (recall Phase 1 uploads eagerly), installed
bundled types, favorite/archived flags, and file-encryption keys. `ALL_OR_NOTHING` only suppresses the
success report and clears `RootCollectionId`/the widget — it leaves orphaned partial objects with no root
collection. The only compensation is `objectcreator.onFinish` (`objectcreator.go:304`): best-effort,
scoped to the **one** failed object's uploaded files, guarded by an inbound-link check, and never touching
already-succeeded objects. (Note a latent bug: `filesToDelete` is deleted **inside** the per-block loop
(`objectcreator.go:306-313`), so it is skipped entirely when the object has no file blocks and repeated
when it has several — hoist it out in a rewrite.)

**Cancellation is cooperative, coarse-grained, and uses two unrelated signals:**
1. `process.Progress.Cancel()` closes an in-process `cancel` channel (`progress.go:104`), polled only by
   `TryStep` in `readResults` **between finished objects**. An object already inside `Create` (tree build,
   file upload) runs to completion.
2. The Go `ctx` threaded into converters/creation is a **separate** mechanism (caller's ctx for sync;
   `componentCtx` for async). It is **not** joined to the progress cancel — cancelling the process does
   not cancel the ctx, and vice-versa.

`GetImportNotificationErrorCode` maps sentinels to the `model.Import.ErrorCode` in the completion
notification. Note there are **two** error-code enums: the deprecated `RpcObjectImportResponse.Error.Code`
(never populated) and the live `model.Import.ErrorCode` carried in `NotificationImport.ErrorCode` — the
latter is the only one clients receive. Any sentinel not in the switch → `INTERNAL_ERROR`; `nil` → `NULL`.
(`docs/ImportErrorCodes.md` is missing `INSUFFICIENT_PERMISSIONS`, which `error.go:153` actively maps from
`list.ErrInsufficientPermissions`.)

---

## 7. Progress, notifications & events

- **Progress** (`core/block/process`): registered via `blockService.ProcessAdd`; drives the UI progress
  bar and cancellation. Variants: `NewProgress`, `NewNotificationProcess` (finishes with a
  Notification), `NewNoOp` (when `NoProgress`). `SetProgressMessage("Create objects")` marks phase 2.
- **Notification**: on finish, `NotificationImport{ProcessId, ErrorCode, ImportType, SpaceId}` — the
  primary result channel for async imports. **Delivery is an interaction, not one flag**: `setupProgressBar`
  runs only when `ImportRequest.Progress == nil`, and emits a notification only when `noProgress=false`
  **and** `SendNotification=true`. `noProgress=true` yields a `NewNoOp` progress that is not
  `Notificationable`, so `SendNotification` is silently ignored; if the caller supplies its own `Progress`,
  both flags are ignored.
- **Event**: `EventImportFinish{RootCollectionID, ObjectsCount, ImportType}` broadcast for async imports.
- **File events**: `fileSync.SendImportEvents()` (on success) triggers download/sync of imported files;
  `ClearImportEvents()` always runs.
- **Metrics**: `ImportStartedEvent` / `ImportFinishedEvent` carry only a per-run UUID + import type — **no**
  object count, duration, error, or space id. Result reporting is entirely via Notification/Event.

---

## 8. Memory & performance characteristics (key for a rewrite)

- **Everything is resident.** The converter materializes **all** snapshots up front; the same
  `[]*Snapshot` slice is held through Phase 1 and Phase 2, plus three maps that scale with object count
  (`oldIDToNew`, `createPayloads`, `relationKeysToFormat`) and a full `TreeStorageCreatePayload` (raw
  change bytes) per new object until its `Create` runs. **There is no streaming/paging** across the
  convert→assign→create boundary — this is the dominant memory constraint.
- **Converter buffering.** Markdown/CSV/HTML/TXT `io.ReadAll` each file; Markdown additionally keeps the
  entire parsed file set + YAML details in a `fileContainer`; PB rebuilds each snapshot as a transient
  `state.State` (~2× working set) for ID remap. Notion holds the whole workspace metadata graph.
- **The source layer itself is streaming** (§4) — so the fix space is in the converters and the
  convert→create pipeline, not in file reading.
- **Concurrency**: fixed 10-worker pool with unbuffered channels; workers share read-only maps (no
  locks). Notion/file downloads have their own worker pools + temp-dir spill.

---

## 9. Notes for the reimplementation (contract vs. incidental)

**Invariants a rewrite must preserve** (things callers/downstream rely on):

- The `Importer` component contract: `Import` (sync/async), `ListImports`, `ImportWeb`,
  `ValidateNotionToken`, keyed by `CName="importer"`; `ImportRequest.{Origin,Progress,IsSync,SendNotification}`.
- Result delivery for async imports via **Notification + `EventImportFinish`** (the proto reply is
  deprecated/empty). A root collection + widget is created and its id surfaced as `RootCollectionId`.
- `Origin` distinguishes user vs. usecase/migration imports and propagates onto every created object —
  builtinobjects and downstream indexing depend on this.
- **ID identity semantics**: dedup on `oldAnytypeID`/unique key/`sourceFilePath`; deterministic derive
  for relations/types/options; file objects last; `_missing_object` for unresolved links. Getting this
  wrong corrupts links and duplicates types on re-import.
- `Mode` (ALL_OR_NOTHING vs IGNORE_ERRORS), the always-fatal sentinel set, and the
  `model.Import.ErrorCode` mapping used by the frontend.
- Progress/cancellation via `process.Progress` registered with `ProcessAdd`.

**Free to change for a better/leaner API** (incidental to the current implementation):

- The `common.Snapshot`/`Response` shape and the whole-set-in-memory model — a streaming
  convert→assign→create pipeline (bounded working set per object) would cut peak memory dramatically, but
  must respect the **global Phase-1 barrier** (§5) and the **eager Phase-1 file uploads** (which give file
  objects their final ids) — otherwise cross-object links break.
- The deprecated proto response fields.
- Worker-pool sizing/backpressure design; the `DataObject` shared-map plumbing.
- The `AGENTS.md` doc (replace or delete — it is inaccurate).
- The Obsidian=Markdown alias and the out-of-band `ImportWeb` path could be unified under a cleaner
  dispatch.

**Landmines to design around:** the load-bearing ordering in Phase 1 (relations → options → files); the
double-remap avoidance for file objects (`UpdateObjectIDsInRelations` is deliberately skipped for them);
the `Revision`-guard that prevents overwriting newer bundled objects; the deadlock avoidance in
`syncFilesAndLinks` (syncers run after the object lock is released); and the `onFinish` `filesToDelete`
nested-loop bug (§6).

**External import is a dead-end as wired.** The wire `Snapshot` message is `{id, SmartBlockSnapshotBase}`
— it has **no field for `SbType`**, and `handleExternalImport` leaves `SbType` at its zero value
(`AccountOld`) without deriving it. The id-provider dispatches purely on `SbType` and has no `AccountOld`
handler, so ID assignment returns `"unsupported smartblock to import"` for every external snapshot; the
existing unit test only passes because the id-provider is mocked. A reimplementation that wants External
to actually create objects must extend the wire message with an `SbType` (or derive it from the
snapshot's details/`Key`) before dispatch.

---

### File index

| Area | Files |
|---|---|
| Orchestration | `importer.go`, `processor.go`, `types.go` |
| Converter contract & data model | `common/types.go`, `common/error.go`, `common/common.go` |
| Source / naming | `common/source/{source,zip,directory,file}.go`, `common/filenameprovider.go` |
| Root collection | `common/collection.go` |
| ID assignment | `common/objectid/{provider,existingobject,treeobject,derivedobject,fileobject,oldfile,workspace,widget,participant}.go` |
| Object creation | `common/objectcreator/{objectcreator,task}.go`, `common/workerpool/workerpool.go` |
| Post-create syncers | `common/syncer/{syncer,file,bookmark,icon,relationsyncer,types}.go` |
| Formats | `markdown/`, `notion/`, `pb/`, `csv/`, `html/`, `txt/`, `web/` |
| RPC handlers | `core/object.go` (`ObjectImport*`) |
| Callers | `util/builtinobjects/builtinobjects.go`, `core/block/bookmark/bookmarkimporter/` |
| Progress/errors | `core/block/process/`, `docs/ImportErrorCodes.md` |
</content>
</invoke>
