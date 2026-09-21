# Import V2 — Architecture & Plan

Design for the greenfield replacement of `core/block/import` ("v1"). This is the engine's one
architecture document: sections 1-14 carry the engine's design and the decisions that bind it;
sections 15-18 describe the subsystems that followed, so what the engine does and why is in one place. The client-facing contract lives in `docs/ImportV2ClientIntegration.md`.

Scope of the first implementation: **engine core + Markdown**, then **Notion**. Other formats (PB, CSV,
HTML, TXT, Web, External) stay on v1 and are out of scope until parity is reached and a switch is
approved.

---

## 1. Design summary

The v1 importer is a **batch** system: convert everything into memory, assign all ids behind a global
barrier, then create in parallel. V2 is a **pipeline** system:

```
                 ┌──────────────── pass 1: identity (cheap, no heavy payloads) ───────────────┐
   Source ──►  Converter.EnumerateIdentities ──► Identity service ──► resident id index        │
                 └────────────────────────────────────────────────────────────────────────────┘
                 ┌──────────────── pass 2: streaming convert + persist ────────────────────────┐
   Source ──►  Converter.Convert ──sink──► bounded channel ──► Resolver ──► Persist pool (K)   │
                 (emits Objects one at a time;      (cap C)     (rewrites     (creates/updates │
                  definitions-before-use order)                 refs via      real objects,    │
                                                                index)        uploads files)   │
                 └────────────────────────────────────────────────────────────────────────────┘
   Finalize: root collection + widget, file-sync events, result/notification
   On fatal error or cancel: compensate from the run journal
```

The one hard problem — cross-object references under streaming — is solved by observing that object
identity in this system falls into **three classes**, and only one of them needs a resident index:

| Class | Examples | How the id is known |
|---|---|---|
| **Derivable** | relations, types, relation options, workspace, widget, participants, date objects, bundled objects | Deterministically computable from a unique key at any time, by any component. No index required. |
| **Minted** | pages, collections, templates | Random tree id, minted once. The *key space* (file paths, Notion page/db ids) is enumerable **cheaply** — from a directory/zip listing or the Notion search index — without reading any block trees or file bytes. This is pass 1. |
| **Upload-determined** | file objects | The final id comes from the (content-addressed, deduping) upload itself. Represented in the index as a **future** resolved when the upload completes. |

So the resident state is exactly what the handoff allows: a compact `sourceKey → id` index (plus small
per-id metadata), never block trees or file contents. The v1 global barrier disappears because pass 1
never touches heavy payloads, and file ids resolve through futures instead of eager serial uploads.

Everything else follows from that spine: converters are streaming producers that never see final ids
(they emit references by *source key*; the resolver rewrites them), persistence is a bounded worker pool
with backpressure, cancellation is one context, failures compensate from a journal, and every layer sits
behind a narrow injected interface.

---

## 2. The object/stream model

Per the handoff, `model.Block` + the Anytype state/snapshot model is the canonical payload — no new
block IR. V2 defines a thin envelope around it (own package, so v1 can be deleted later without breaking
v2):

```go
// package importmodel (core/block/importv2/model)

// Object is the unit that flows converter → resolver → persister.
type Object struct {
    // SourceKey is the converter-scoped stable identity of this object in the source:
    // a normalized file path (Markdown), a Notion page/database/property id, etc.
    // All cross-object references inside Payload use SourceKeys (or derivable keys / final
    // Anytype ids for bundled/date targets). The resolver rewrites them.
    SourceKey string

    SbType   coresb.SmartBlockType
    TypeKeys []domain.TypeKey

    // Payload is the state snapshot: blocks, details, relation links, collection store,
    // file info — structurally identical to model.SmartBlockSnapshotBase in domain types
    // (same shape as v1's common.StateSnapshot, redefined in this package).
    Payload *Snapshot

    // File is set for file objects only: where the bytes are and how to upload them.
    File *FileSource

    // Hints for the engine.
    IsRootCandidate bool // member of the root collection
    Favorite, Archived bool
}

// FileSource describes file bytes without holding them.
type FileSource struct {
    // Exactly one of Path / Open is set. Open lets zip-backed converters stream the entry
    // on demand (the engine spills to a sanitized temp file only if the uploader needs a path).
    Path string
    Open func(ctx context.Context) (io.ReadCloser, error)
    EncryptionKeys map[string]string
    URL  string // original remote URL, for provenance/diagnostics
}
```

Rules of the stream (the **converter contract**, enforced by engine assertions in tests):

1. **One object at a time.** `sink.Object(ctx, obj)` blocks for backpressure; after it returns the
   converter must drop its reference. The engine owns the object from then on.
2. **Definitions before use.** An object that *defines* identity others reference — a relation, a type,
   an option, a **file object** — must be emitted before (or by the same converter call that first
   emits) an object referencing it. This is a local ordering constraint, trivially satisfiable by both
   converters, and it is what replaces v1's global barrier for the derivable/upload classes.
3. **No silent drops.** Anything the converter cannot represent must produce either a placeholder block
   or `sink.Issue(...)` with a structured code (§8). Tests pin every deliberate loss decision.
4. **No store access.** Converters see only the `Source`, the network (Notion), and the sink.
5. **Deterministic.** Given the same source, a converter emits the same objects in the same order (stable
   iteration, no map-order dependence, no `bson.NewObjectId()` for anything that affects output shape,
   hash-derived option colors instead of random ones). This is what makes golden tests possible.

```go
// package importmodel

type Converter interface {
    Name() string

    // EnumerateIdentities is pass 1: cheaply yield one IdentityClaim per minted-class object
    // this converter will emit, WITHOUT reading heavy content. For Markdown this is a source
    // listing walk (+ parsing the small JSON schema files); for Notion it is the /search crawl.
    EnumerateIdentities(ctx context.Context, yield func(IdentityClaim) error) error

    // Convert is pass 2: stream every object. Must honor ctx promptly.
    Convert(ctx context.Context, sink Sink) error
}

type IdentityClaim struct {
    SourceKey      string
    SbType         coresb.SmartBlockType
    // Dedup keys, in match priority order (see §4): oldAnytypeID, uniqueKey, sourceFilePath.
    OldAnytypeID   string
    UniqueKey      string
    SourceFilePath string
}

type Sink interface {
    Object(ctx context.Context, o *Object) error // backpressure; error => stop converting
    Issue(i Issue)                               // warnings & per-object errors, never blocks
    Progress(delta int64)                        // optional finer-grained ticks
}
```

A converter is **constructed per run** (fixing v1's singleton-with-mutable-state race — concurrent
imports currently share converter instances).

### Why not the other §3 options

- **Full two-pass re-streaming** (re-read the source twice including content): correct but wasteful for
  Markdown and *prohibitive* for Notion (a second full API crawl against rate limits). The identity pass
  here is the same idea restricted to what pass 2 actually needs — ids — which for every supported
  source is obtainable without content reads.
- **Deferred link patching** (persist with dangling refs, patch later): requires a second write per
  object (more sync traffic, more failure states), makes ALL_OR_NOTHING accounting murky (an object is
  "created" but not yet correct), and the pattern of "cheap id enumeration" makes it unnecessary. We keep
  it as the documented fallback for a hypothetical future source whose ids are *not* cheaply enumerable.
- **Topological ordering**: reference graphs here are cyclic (pages link both ways); not generally
  applicable.

---

## 3. Reference resolution (the spine)

The **Resolver** rewrites, inside each streamed object: link blocks, text `Mention`/`Object` marks,
bookmark targets, dataview targets, object-format detail values, and collection-store membership.

Resolution table, applied per reference:

| Reference target | Resolution |
|---|---|
| Bundled relation/type URLs, date object ids, existing Anytype ids explicitly marked by the converter | preserved as-is |
| Derivable key (relation/type/option unique key) | computed via the Identity service (derive or dedup-match, memoized) |
| Minted key present in index | index lookup (O(1)) |
| File key | **future**: wait (ctx-aware) until the file object's upload resolves the id |
| Anything else | rewritten to the explicit `_missing_object` marker **and** recorded as a structured `Issue{Code: MissingTarget}` — visible, never silent |

Object-format detail rewriting needs to know which relation keys are object/file-format. V2 keeps a
resident **format registry**: seeded from bundled relations, extended as relation-definition objects
stream through (definitions-before-use guarantees availability), backed by a store lookup for
pre-existing custom relations. O(number of relations), small strings.

**File futures and deadlock-freedom.** A file object's index entry is created (as an unresolved future)
the moment the converter emits the file object. Persist workers pull from the stream FIFO, so by the
time a referencing object P is being persisted, its file F is either done or already occupying a worker;
F's completion never depends on P → no cycles. Waits are ctx-cancellable, and a bounded wait guard
(deadline + engine invariant check) turns a contract violation (use-before-definition) into a loud error
instead of a hang.

**Why file uploads move out of pass 1** (v1's biggest serialization point): in v1, Phase 1 uploads every
file synchronously, single-threaded, so file-heavy imports are bounded by one goroutine. In v2 file
objects are ordinary stream objects persisted by the pool (upload concurrency additionally capped by the
existing `fileuploader` limiter of 8); references synchronize on the future instead of on a global
phase boundary.

---

## 4. Identity & dedup service

One component owns *all* id semantics (v1 spreads this across `objectid/*` + processor ordering):

```go
type Identity interface {
    // Pass 1: mint or match one claim; records the result in the index.
    Claim(ctx context.Context, c IdentityClaim) error
    // Pass 2 lookups.
    Resolve(sourceKey string) (id string, ok bool)
    ResolveFile(ctx context.Context, sourceKey string) (id string, err error) // future-aware
    // Derivable ids on demand (memoized; dedup-checks the store on first use).
    DeriveKeyed(ctx context.Context, sbType coresb.SmartBlockType, uniqueKey domain.UniqueKey,
        hints DedupHints) (id string, err error)
    // Payload retrieval for objects that need a new tree.
    TakeCreatePayload(id string) (treestorage.TreeStorageCreatePayload, bool)
}
```

Semantics preserved from v1 (product-critical, per handoff §5):

- **Match order**: `oldAnytypeID` → unique key → (`updateExistingObjects` only) `sourceFilePath` →
  type-specific fallbacks (relation by name+format, option by name+relationKey, type by name/uniqueKey).
  Backed by `objectstore.SpaceIndex(spaceId)` queries exactly as `existingobject.go` does today.
- **Derivation**: relations/types/options/profile via `space.DeriveTreePayload` (deterministic from
  unique key); deleted-object check before deriving; workspace/widget ids from `DerivedIDs()`;
  participant ids from `domain.NewParticipantId`.
- **Minting**: pages/collections/templates via `space.CreateTreePayload`. The returned
  `TreeStorageCreatePayload` (root raw change, ~hundreds of bytes) must be retained between pass 1 and
  creation because the tree id is a hash of that change. It lives in a **payload store**: in-memory map
  by default, with a disk-spill implementation behind the same interface if profiling shows it matters
  (100k objects ≈ tens of MB — already ~2 orders of magnitude below v1's resident set, so spill is a
  contingency, not a launch requirement). A dedup *match* stores no payload.
- **Revision guard**: an existing object whose `Revision` is newer than the import's is never
  overwritten (protects bundled types/relations).
- **Idempotent re-import**: derived objects always converge; pages converge when
  `updateExistingObjects=true` (matched by the keys above). Same as v1, but now covered by tests.

The index itself: `map[sourceKey]entry` where entry = `{id string, mode created|matched|derived,
future *fileFuture}` + the format registry + (for root collection) the member id list. All O(objects) of
small strings — the only thing allowed to scale with workspace size.

---

## 5. Persistence, concurrency, backpressure

```go
// package importpersist — consumes the resolved stream.

type Persister interface {
    Persist(ctx context.Context, o *importmodel.Object) (Outcome, error)
}
```

Pipeline shape: `sink.Object` → bounded channel (**C = 16**) → **K = 8** workers. Peak heavy-object
residency is `C + K` objects plus whatever single file is being uploaded per worker — the memory
invariant, enforced by a test (§10). Channels unbuffered-ish and blocking end-to-end: a slow store stalls
the converter, a slow Notion API starves the pool; neither accumulates.

Per-object persist (direct descendant of v1's `objectcreator.Create`, minus its landmines):

1. Stamp provenance (`RelationKeyOrigin`, `RelationKeyImportType`, source path, created/modified dates —
   refusing `time.Now()` fallback for created-date, as v1 does) — **before** state build.
2. Build `state.State` from the payload; run the resolver rewrites (links, marks, relation values,
   store membership) — for file objects the detail values were already resolved by the converter
   contract, mirrored from v1's "skip double remap" rule.
3. **Bundled deps**: referenced bundled relations/types go through an **install coordinator** — a
   singleflight keyed by bundled id in front of `objectcreator.InstallBundledObjects` — so K workers
   stop racing the installer (v1 relies on the installer's internal dedup under 10-way concurrency).
4. **Create or update**:
   - create: `space.CreateTreeObjectWithPayload(payload, initFunc)`; on `treestorage.ErrTreeExists`
     fall back to opening the existing object (kept — it is what makes racy re-imports idempotent);
   - update (dedup match + updatable): open via object getter, Revision guard, reset state
     (`history.ResetToVersion`), **collections: membership replaces, per §13.1**;
   - workspace/widget: the two v1 special cases preserved (set details on workspace; merge widget
     blocks), reached only by PB imports today but kept in the engine since they're format-neutral.
5. **File objects**: upload via `BlockService.UploadFile` with `CustomEncryptionKeys` (or
   `fileobject.CreateFromImport` for already-content-addressed CIDs) → resolve the future with the
   returned id. Content dedup is free (CreateFromImport / upload return the existing object id).
6. Favorite/archive via `detailservice`.
7. Journal the outcome (§7) and report `Outcome{Created|Updated|Skipped, id, details}`.

**No post-create syncers.** v1's `FileSyncer`/`IconSyncer`/`BookmarkSyncer` exist because file ids were
unknown at creation time, forcing re-entry into just-created objects (with the collect-then-run-unlocked
deadlock dance). In v2 file ids resolve *before* the referencing object is finalized, so
`TargetObjectId`/`IconImage`/file detail values are written correctly the first time. Bookmark fetching
(URL → title/preview) is not part of persistence correctness; it stays post-create as an optional,
journaled enrichment step for bookmark blocks (same behavior the user sees today).

Seams injected into the persister (all narrow, all mockable, signatures already verified against
current code):

| Seam | Methods | Today's implementation |
|---|---|---|
| `SpaceLoader` | `Get(ctx, spaceId) (Space, error)` | `space.Service` |
| `Space` | `CreateTreePayload`, `DeriveTreePayload`, `CreateTreeObjectWithPayload`, `Do`, `DerivedIDs`, `IsReadOnly` | `clientspace.Space` |
| `Store` | `SpaceIndex(spaceId)`: `QueryObjectIds/Query/QueryRaw/GetOutboundLinksById`; `AddFileKeys` | `objectstore.ObjectStore` |
| `FileUploader` | `UploadFile(ctx, spaceId, FileUploadRequest)`, `CreateFromImport(fullFileId, origin, details)` | `block.Service` + `fileobject.Service` |
| `BundledInstaller` | `InstallBundledObjects(ctx, space, ids)` | `objectcreator.Service` |
| `ObjectAccess` | open/update/delete (`cache.Do`-style) | `ObjectGetterDeleter` |
| `Details` | `SetIsFavorite`, `SetIsArchived` | `detailservice.Service` |
| `CollectionBuilder` | `CreateCollection(details, flags)` | `collection.Service` |
| `Reporting` | progress, notifications, `EventImportFinish`, `SendImportEvents/ClearImportEvents` | `process`, `notifications`, `event.Sender`, `filesync.FileSync` |

---

## 6. Cancellation — one mechanism

A run owns a single `runCtx`:

- **Sync** callers (builtinobjects — stays on v1 for now, but the engine supports it): `runCtx` derives
  from the caller's ctx.
- **Async** (gRPC): `runCtx` derives from the component ctx; the run is **tracked**, and component
  `Close()` cancels and then *waits* (bounded grace) for in-flight runs — fixing v1's fire-and-forget
  goroutine that races shutdown.
- The registered `process.Progress` is joined to it using the proven `fileuploader` pattern: the process
  object owns `context.WithCancel`; its `Cancel()` cancels `runCtx`. One signal, no second channel.

Everything long-running takes `runCtx`: source iteration, converter network calls and retries
(back-off `select`s on ctx), file downloads **and their read loops** (v1's stall monitor couldn't
interrupt a hung read; v2 uses `http.Request.WithContext` + a read deadline wrapper so a half-open
connection dies with the ctx), uploads, tree creation, future waits. Cancellation is therefore honored
*mid-object*, not just between objects. `ErrCancelled` is a Fatal issue → compensation runs (§7),
notification reports `IMPORT_IS_CANCELED`.

---

## 7. Failure model — journal + compensation (the one story)

Chosen story: **explicit, complete compensation**, not transactional rollback — a CRDT space that may
already be syncing has no transaction to abort; deletion *is* the native undo and propagates correctly.

Every effect is recorded in an in-memory **run journal** before/as it happens:

| Effect | Journal record | Compensation |
|---|---|---|
| Created tree object | id | delete object |
| Uploaded/created file object | id | delete object **unless** it has inbound links from outside this run (`GetOutboundLinksById`-style check — dedup means an "uploaded" file may pre-date the run; matched ids are journaled as *matched*, never deleted) |
| Updated existing object | id + previous heads/version id | report as non-compensated in the result, explicitly (§13.2 — restoring a previous version is a possible later addition) |
| Favorite/archive flags | id + previous value | revert |
| Installed bundled objects | ids | none (idempotent, shared, harmless — deliberately not compensated) |
| Root collection + widget | ids | delete |

On **fatal error or cancel**: stop intake, drain workers, run compensation (itself ctx-bounded but on a
fresh context so cancel doesn't abort the cleanup), and report `Result{Err, compensated: n, leaked: m}`
— if any compensation step fails, the result *says so per object* instead of silently orphaning, which
is the v1 behavior being replaced. Under `IGNORE_ERRORS` (continue mode), per-object failures are
collected, that object's own partial effects are compensated (v1's `onFinish`, with the
`filesToDelete` nested-loop bug fixed by construction), and the run proceeds.

Crash-during-import leaves orphans exactly as v1 does; a persisted journal for crash recovery is noted
as future work, out of scope.

---

## 8. Error model

One typed issue, one severity ladder, one predicate (fixing v1's per-phase divergent mode checks and
dual enums):

```go
type Severity int // Warning < ObjectError < Fatal

type Issue struct {
    Severity  Severity
    Code      IssueCode // typed: MissingTarget, UnsupportedBlock, DataLoss(kind), FileFetchFailed,
                        // RateLimited, AuthFailed, SourceInvalid, Cancelled, StoreError, ...
    SourceKey string    // which source object
    ObjectId  string    // which created object, when known
    Err       error     // wrapped chain, fmt.Errorf("op: %w", ...) per CLAUDE.md
}
```

- **Warning**: deliberate data-loss decisions, placeholders, skipped unknowns. Never aborts, always
  reported (counted + capped list in the result).
- **ObjectError**: this object failed. Aborts iff `mode == ALL_OR_NOTHING`; otherwise skip + continue.
- **Fatal**: aborts regardless of mode. Exactly the current always-fatal set: auth/permission failure,
  rate-limit exhaustion, empty source ("no objects"), cancel, plus engine invariant violations.

The single predicate `abort(issue, mode)` lives in one function used by every stage.

**Wire mapping** (thin, in the adapter): highest-severity issue → `model.Import.ErrorCode` using the
existing enum values (`NOTION_RATE_LIMIT_EXCEEDED`, `FILE_LOAD_ERROR`, `IMPORT_IS_CANCELED`,
`NO_OBJECTS…`, `INSUFFICIENT_PERMISSIONS`, …) so the frontend contract is untouched. The internal result
is richer:

```go
type Result struct {
    RootCollectionId string
    WidgetLayout     model.BlockContentWidgetLayout
    Created, Updated, Skipped, Failed int64
    Issues           []Issue // capped (e.g. 1000) + overflow counter
    Compensated, Leaked int   // failure-path accounting
    Err              error    // fatal error if aborted
}
```

`docs/ImportErrorCodes.md` gets superseded by a generated table from `IssueCode` → wire code (including
the currently undocumented `INSUFFICIENT_PERMISSIONS`).

---

## 9. gRPC adapter & coexistence

**No wire changes.** The existing handlers keep working; v1 stays registered and untouched.

- New component `importv2` (own CName), registered in `bootstrap.go` next to v1.
- Gating via `Config` bools (envconfig auto-binds): `ImportV2Markdown`, `ImportV2Notion`
  (`ANYTYPE_IMPORTV2MARKDOWN=1`, …). Default **off**.
- One small guarded branch in the `ObjectImport` handler (`core/object.go`): if the request type is
  Markdown/Obsidian (or Notion) *and* the flag is on → `importv2`; else v1, exactly as today.
  `ObjectImportNotionValidateToken` routes the same way when Notion-v2 is on. `ObjectImportList`,
  `ImportWeb`, `builtinobjects`, `bookmarkimporter` keep calling v1 unconditionally.
- The adapter is thin: translate `pb.RpcObjectImportRequest` → engine `Request`; register the progress
  process (`ProcessAdd`); on finish map `Result` → `NotificationImport` + `EventImportFinish` +
  `SendImportEvents`/`ClearImportEvents`, with v1's exact delivery semantics (`noProgress` ⇒ NoOp
  progress ⇒ no notification; caller-supplied Progress ⇒ both flags ignored; `EventImportFinish` only
  for async runs).
- Root collection + widget: engine builds the collection (via `CollectionBuilder`, name "Markdown
  Import"/"Notion Import" + date, `WithRelations`), adapter-independent; widget created via
  `CreateWidgetBlock` with the converter-chosen layout (Markdown: `Link`, single-dir-page → `Tree`, no
  collection; Notion: `CompactList`) — behavior-compatible with v1.

Switch-over (deliverable 4, not now): flip defaults per format after parity checks; only then discuss
deleting v1 paths. Nothing in v2 imports v1 packages except the adapter's use of `pb` types and, if
convenient, `anymark` (explicitly allowed).

---

## 10. Test architecture

**Layered unit tests** (fixture pattern, `want` structs, testify, mockery via `.mockery.yaml` — new
entries for the v2 seams; `make test-deps`):

- Identity: determinism (same claims twice ⇒ same ids), match-priority order, Revision guard, deleted-
  object handling — against the *real* `objectstore.StoreFixture` (as v1's objectid tests already do)
  plus the existing `mock_space`/`mock_clientspace`.
- Resolver: forward refs, file futures (including the use-before-definition invariant turning into an
  error), `_missing_object` + Issue emission, bundled/date passthrough, format-registry behavior.
- Persister: create/update/ErrTreeExists fallback, install coordinator singleflight, journal contents.
- Engine: cancellation actually interrupts (converter blocked on sink, worker blocked on a slow fake
  upload — both must return promptly on cancel), mode predicate uniformity, compensation completeness
  (every journal entry compensated; injected compensation failure ⇒ `Leaked` reported).

**Golden round-trip harness** (`importv2/enginetest`): input fixture (md dir/zip; Notion cassette) →
run the full engine against `StoreFixture` + an in-memory fake `Space` that materializes created trees
as inspectable states → serialize the resulting object set (details, blocks, relation links, membership,
resolved links) to golden JSON. `-update` flag regenerates. Converter determinism (§2 rule 5) is what
makes these stable.

**Streaming/memory tests**: the engine exposes a test-only gauge of in-flight heavy objects; a synthetic
10k-object source asserts the gauge never exceeds `C + K + ε` and that a reintroduced
"collect-then-process" would fail the test. A large-file source test asserts file bytes never transit
through a `[]byte` of file size (reader-based accounting on the fake uploader).

**Idempotency tests**: run the same import twice into one store (`updateExistingObjects` on and off);
assert no duplicated relations/types/options ever, page duplication only in the documented
`updateExisting=false` case.

**Notion** (§11): cassettes for real-shape coverage + hand-written transport mocks for hostile cases.

**Data-loss pinning**: every row of the deliberate-decision table (§11.3) has a test asserting the
chosen placeholder/warning behavior.

---

## 11. Converters

### 11.1 Markdown (first)

Reproduces the v1 product surface (verified against the current code) in streaming form:

- **Pass 1** (`EnumerateIdentities`): walk the source listing once — claims for every `.md` (page),
  `.csv` (collection; membership = sibling-dir rule), directory page (when `createDirectoryPages`,
  disabled if schemas exist), with `sourceFilePath` = normalized path (NFC, like v1). Parse the small
  `.json` schema files (`"x-app"`-marked) here — they define types/relations/options, all derivable-
  class, and force-disable dir-pages/properties-as-blocks as today.
- **Pass 2** (`Convert`), per file, one at a time: read the file (single-file `io.ReadAll` is fine —
  bounded by one file, not the set), split YAML front-matter (`pkg/lib/schema/yaml` reused), run
  `anymark.MarkdownToBlocks` (reused as permitted), then do *locally* what v1 does in nine global
  mutation stages: title/emoji extraction from H1, link marks → mentions/page-links/file-blocks/
  bookmarks with source-key targets, CSV-link substitution, per-file property blocks
  (`includePropertiesAsBlock`). Emit newly-encountered YAML relation/option/type definition objects
  before the page; emit referenced file objects (streamed from the source via `FileSource.Open`) before
  the page; emit the page. Directory pages and CSV collections emit after their contents (membership
  lists are source-key lists, resolved later — order irrelevant for stores).
- v1 behaviors deliberately kept: front-matter format inference + `tag`/`status`/`created`/`modified`
  bundled remaps; schema key/format adoption; Obsidian = alias (wikilinks/anchors already handled);
  orphan self-link block; root-collection rules incl. the single-dir-page `Tree` case.
- v1 defects fixed by design: zip-slip (all archive entry names sanitized: reject `..`/absolute before
  any join — in v2's own source layer), basename-collision link resolution (prefer same-dir, then
  unique-basename, else Issue instead of silently wrong page), nondeterministic keys/colors/order
  (stable derivation), the `fmt.Println` debug leftover, per-run converter state, progress
  total mismatch, and the audited anymark nesting bugs get regression tests where we touch them (fixing
  `anymark` internals is in scope only where imports are wrong, tracked as explicit tasks).

### 11.2 Notion (second, web-first)

Design against the current official API docs (verify block/property coverage against
developers.notion.com during implementation; the doc-driven deltas — `synced_block` children being
fetchable, `custom_emoji` icon shape, `link_preview`, current rate-limit guidance — get closed
deliberately, see 11.3).

- **Transport seam** (the testability keystone):

  ```go
  type Transport interface { // one Notion API request
      Do(ctx context.Context, req *http.Request) (*http.Response, error)
  }
  type FileFetcher interface { // one file download to a local spill path
      Fetch(ctx context.Context, url string, dst io.Writer) error
  }
  ```

  Production: `Transport` = retrying client (bounded attempts — e.g. 5 — capped exponential backoff,
  honoring `Retry-After` without v1's post-override doubling; per-attempt *and* total budgets; all
  ctx-aware) behind a **shared token-bucket rate limiter** (~3 rps average per Notion guidance,
  adaptive on 429) replacing v1's 5ms-sleep folklore. `FileFetcher` = same discipline (v1's bare
  `http.DefaultClient` with no timeout/retry is gone), ctx-interruptible reads.
- **Pass 1**: paginated `/search` (bounded retries; `has_more` with null cursor → typed error, not the
  v1 nil-panic; bodies decoded incrementally and released — no marshal/unmarshal double-parse, no
  retained response bodies). Retain only ids/titles/parents → claims (`sourceFilePath` = Notion id, as
  v1) + the small title/parent maps the converter needs for `child_page` title resolution.
- **Pass 2**: databases first (collections + schema relations, shared property/option dedup store —
  v1's `PropertiesStore` logic kept, including the exact Tag-redirection rules and the
  ReplaceRelations/AddRelations dataview split), then pages through a converter-internal fetch pool
  whose output feeds the sink one page at a time. Per page: properties → details (+ new relation/option
  definitions emitted first), block tree fetch (bounded retries, **depth cap** with Issue, 100-page
  pagination), block mapping (v1's table preserved), files downloaded to a **per-run temp dir**
  (removed on finish; names keyed by content-source identity *including* a per-run component — kills
  the stale-reuse and rename-race defects) and emitted as file objects before the page. UTF-16 offsets,
  mention remaps, and child-page title resolution preserved as-is.
- Failure semantics: download failures are `ObjectError`s (not silently-empty paths); rate-limit
  exhaustion and auth failures Fatal; per-page errors respect the one mode predicate; cancel interrupts
  fetches mid-read.
- Token validation RPC: same `Ping` logic behind the `Transport` seam.

### 11.3 Deliberate data decisions (each pinned by a test + Issue)

Proposed defaults — flag disagreements in review:

| Construct | v1 behavior | v2 proposal |
|---|---|---|
| Unknown Notion block type | silently dropped | placeholder text block + `Warning(UnsupportedBlock)` |
| `synced_block` | "Unsupported block", content lost | fetch children (original & duplicate via `synced_from`) → import as duplicated content + Warning; hoisted ids get a per-occurrence suffix so repeated references to one original stay distinct blocks |
| Toggleable heading children | flattened to siblings | properly nested under the heading |
| `to_do` nesting | two-parent bug | correct child filtering |
| Table headers | inverted | match Notion semantics (`has_column_header` → header row) |
| `custom_emoji` icons | dropped | download the emoji image → image icon + Warning |
| Date ranges / time zones | end + tz dropped; malformed → epoch 0 | apply tz; store start; end → companion "<name> (end)" relation; malformed → Warning, detail omitted (never epoch-0) |
| Date formulas | dropped | import as date detail |
| `verification` property | dropped | skipped + Warning (no Anytype analogue) |
| Notion `select` (single-choice) | tag relation — pick-one cardinality lost | **status** relation (single-select preserved; `multi_select` stays tag) — decision §13.5 |
| People (`created_by`, mentions…) | name strings / dangling raw ids | name strings kept; dangling ids never emitted (skip + Warning). Property maps to tag-of-names for now; an object-format mapping to imported member pages is a possible later improvement |
| Rollup arrays | pseudo-tag with unbacked values | joined longtext + Warning (no unbacked tag values) |
| Brown color | silently → default | nearest supported color + Warning |
| Block/media captions | dropped (except bookmark) | emitted as description text under the block |
| Markdown H1-as-title | first H1 stripped into title | kept (product expectation), documented |
| Markdown missing file target | empty file block | placeholder + `Warning(MissingTarget)` |

### 11.4 Markdown flavour profiles (phase 4)

**Problem.** The markdown converter serves four dialects through one implicit behavior set: generic
markdown folders/zips, Notion markdown+CSV zip exports, Obsidian vaults (`Import_Obsidian` is a plain
alias), and Anytype's own markdown export (YAML front-matter + x-app JSON schemas). The 2026-07-07
v1-parity review showed the flavour-specific pieces are either applied unconditionally (the CSV↔dir
collection convention fires on generic folders), missing entirely (Notion property lines, the
`Collection:` front-matter store), or wrongly generic-ified (emoji token handling). Adding more
flavours (Logseq, Bear, Joplin) onto the same path recreates v1's failure mode: one converter with all
conventions interleaved.

**Decision.** Engine, identity, resolve and persist stay flavour-blind (they already are). The
markdown converter gains a **flavour profile**: a compile-time strategy value selected once per run —
request override first, listing-based detection otherwise. Not runtime plugins: a `map[string]Flavour`
is the whole registry; a new flavour is one file plus fixtures. Profiles transform page-local data
only; they never see the sink — the converter keeps sole ownership of emission order, so
definitions-before-use can't be broken by a profile.

```go
// package markdown — sketch; the field set is fixed by the hook inventory below
type Flavour struct {
    Name string // "generic" | "notion-export" | "obsidian" | "anytype-export"

    // 1. Syntax: goldmark extenders enabled for this profile (appended to
    //    the base set — tables, strikethrough, <details> toggles, <u>,
    //    wiki-links — which stays global: cheap and unambiguous). Shipped
    //    as variadic goldmark.Extender on MarkdownToBlocks; existing
    //    callers unchanged; nested toggle reparses run base-only for now.
    Anymark []goldmark.Extender

    // 2. Metadata: page-level property extraction beyond the shared YAML
    //    front-matter pass (which stays global). Runs after H1 title
    //    extraction, before reference rewriting.
    ExtractMetadata func(c *Converter, page *pageContext) // nil = YAML only

    // 3. Link targets: fallback tried after the generic chain (exact path
    //    → unescape → unique basename) misses; receives the chain's
    //    normalized, percent-unescaped form of the target.
    ResolveTarget func(c *Converter, target string) (entryName string, ok bool)

    // 4. Structure conventions. CSVCollections is uniform: claims,
    //    collection emission, link/mention target classification and
    //    directory-tree membership all follow it.
    CSVCollections        bool // Notion's `Db.csv` ↔ `Db/` membership rule
    DirectoryPagesDefault bool // profile default; can only ENABLE — see below
    CollectionByName      bool // front-matter *name* "Collection" → collection store
    SuggestTypes          bool // csv titles type their member pages (§11.5)
}
```

**Hook inventory.** Defaults in step 1 reproduce today's behavior exactly (pure-refactor gate);
divergences land in step 2 with their own goldens.

| Hook | Generic default | Flavour overrides |
|---|---|---|
| Syntax extensions | global base set above | obsidian: callouts (`> [!note]`), `==highlight==`, `%%comments%%`; logseq (future): `key:: value` inline properties, outliner mode |
| Metadata extraction | YAML front matter only | notion-export: first-paragraph `Key: value` lines → details + Mention marks resolved by trailing Notion id (v1 `processFieldBlockIfItIs`); logseq (future): `::` page properties |
| Link resolution | generic chain; ambiguity → Issue | notion-export: trailing-32-hex-id match when the path misses; obsidian: extension-less basename, shortest-path preference |
| CSV collections | on (v1 parity — open question below) | notion-export: on; obsidian: candidate for off |
| Directory pages | request param, default off | obsidian: candidate default on (vault tree). Caveat: the proto bool cannot express explicit-off, so a profile default can only enable; before any profile ships `true` the request param needs a tri-state (open question) |
| Collection property | `_collection` key honored **globally** (unambiguous, anytype-only key) | anytype-export: additionally match by display name "Collection" (v1 `EqualFold` rule — too loose to run globally) |
| Type suggestion | off | notion-export: csv collection titles type their typeless member pages (§11.5, name-only evidence) |
| JSON schemas | honored globally when present, as v1 (the x-app marker never false-positives); schema presence keeps force-disabling dir pages / properties-as-blocks | — (schema presence *is* the anytype-export detection signal) |
| Title/icon | leading H1 → title, leading emoji **grapheme cluster** → icon | none yet; obsidian filename-as-title is an open product question (v1 extracted H1 for every flavour) |

**Detection.** Once per run, on the pass-1 listing already in memory (schemas are already sniffed
there):

1. Request wins: `model.Import_Obsidian` → obsidian. (`Import_Markdown` has no flavour field; adding
   one to `RpcObjectImportRequestMarkdownParams` is deferred — detection covers the common cases.)
2. x-app JSON schema parsed → anytype-export.
3. `.obsidian/` directory in the listing → obsidian.
4. Notion signature — ≥ max(2, 20% of `.md` files) basenames ending in ` <32-hex>` before the
   extension, or ≥1 `X.csv` with a sibling `X/` directory containing `.md` → notion-export.
5. Otherwise generic.

The choice is reported as an informational Issue ("markdown source detected as notion-export") —
cheap observability for "why did my import behave that way". Detected-generic stays silent (nothing
flavour-specific is enabled); a forced flavour is always reported ("requested as …"). Once hooks
actually enable behavior (step 2+), the message grows the enabled-behaviors clause ("… property
lines and id-based link resolution enabled").
Mixed sources are the norm (an Obsidian vault containing pasted Notion pages), so profiles are
defaults-plus-leniency, not strict modes: only heuristics with false-positive risk (field-block lines,
hex-id matching, name-based Collection) are profile-gated; unambiguous syntax stays global.

**Flavour-neutral (no hooks, unchanged):** claims/identity, definitions-before-use emission, YAML
property→relation/option/type minting, deterministic keys/colors, ref rewriting + file futures,
dir-page mechanics, root spec/collection, the error model.

**Review deltas folded in (2026-07-07 v1-parity review).** Each finding is either a flavour-neutral
fix in place or the first real implementation of a hook:

| Finding (severity) | Disposition |
|---|---|
| `Collection:`/`_collection` front matter loses the collection store (high) | `CollectionByName` hook + global `_collection` handling; store + v1's member resolution (`.md`-append, exact/dir-relative/basename) |
| Notion first-paragraph property lines dropped (high) | `ExtractMetadata` hook, notion-export |
| multi-code-point emoji split; emoji-only H1 → empty Name (med) | neutral fix: v1's whole-first-token rule (flags/ZWJ/skin tones survive intact) + keep-title-when-emoji-only |
| `isEmojiRune` misses 2194–25FF / 2B00–329F (⏰⭐⭕) (med) | neutral fix: v1 ranges ∪ 1FAE0+ |
| read/parse-failed pages leave dangling claimed ids (med) | converter emits a placeholder page (filename title, no blocks) so the claimed id stays real — v1 behavior. An engine-side `MissingObject` rewrite is impossible: pages reference forward, so refs resolve before the target's emission outcome is known. The ObjectError still aborts all-or-nothing runs |
| inline (mid-sentence) local file link drops the attachment (med) | neutral fix: emit the file object and retype the mark to a Mention of it (keeps text, unlike v1's destructive block replacement) |
| `![](page.md)` / `![](db.csv)` source-key collision (med) | neutral fix: `isPageEntry` guard in `rewriteFileBlock`; csv file-blocks → page links (v1 rule) |
| deep-tree root not collapsed; dir-page child order; properties-as-blocks exclusion list; type featured-relations split (low) | neutral parity fixes |

**Testing.** One fixture directory per flavour under `enginetest/testdata` with goldens; a detection
table test; cross-contamination tests (field-block lines NOT parsed under generic, hex-id resolution
NOT under obsidian); the parity harness gains one fixture per v1 dialect (new flavours have no v1 to
compare — they are spec'd from the source app's export format and pinned by goldens only).

**Sequencing (phase 4).**
1. Introduce `Flavour` + detection with the four existing profiles, all defaults = current behavior.
   Goldens unchanged except the new detection Issue. Pure refactor.
2. Land the review-delta table on the seam — first real divergence; per-flavour goldens.
3. New flavours one at a time (candidate order: Logseq, Bear, Joplin): profile file + anymark
   extenders (where syntax differs) + fixtures + goldens.

**Open questions (decide before step 2):** should generic keep the CSV↔dir collection convention
(v1 parity) or route bare csv to table-import instead; Obsidian filename-vs-H1 title precedence;
whether `Import_Markdown` grows an explicit flavour override param for the clients; a tri-state
`CreateDirectoryPages` request param (required before any profile defaults directory pages on).

### 11.5 Type suggestion

**Problem.** Every imported page is a plain Page, even when the source screams its type: a Notion
database called "Tasks" with a Due-date column, a "People" database with email and phone columns.
Typing those rows (Task, Contact, …) is most of the difference between an import that feels native
and one that needs an afternoon of manual retyping.

**Seam.** `core/block/importv2/typesuggest` — one interface, converter-agnostic evidence:

```go
type Evidence struct {
    ContainerName string     // database title / csv collection title, id-stripped
    Properties    []Property // schema property names + relation formats, when known
}
type Suggestor interface {
    Suggest(evidence Evidence) (Suggestion, bool) // TypeKey, Confidence, Reason
}
```

Rules of the seam: suggestions only ever fill the **default-Page gap** — an explicit type (front
matter, schema) always wins, enforced by the callers; implementations return only suggestions
confident enough to *apply* (there is no suggestion UI in the import flow); output is deterministic
for identical evidence. The naive implementation ships first; a learned model (and with it
non-English container names) replaces it behind the same interface in a later iteration.

**Naive rules** (deliberately conservative — a wrong type on every row of a database is worse than
Page): exact normalized container-name matches against per-type keyword tables (task/todo/backlog…,
project, contact/people/clients…, note, journal→diary entry, goal/okr, book/reading list, movie,
recipe; plurals listed, not derived; emoji and punctuation stripped) at confidence 0.9; property-shape
corroboration when the name says nothing — email+phone → contact (0.8), a completion-named checkbox
(done/complete/…) → task (0.75), due-date + status property → task (0.7).

**Evidence sites.** Notion converter: `convertDatabase` suggests from the data-source title + schema
(sorted property names, mapped formats) and records the verdict under both the entity and data-source
ids; `convertPage` types rows through either parent form. Databases convert before pages, so
suggestions are complete when rows ask. Markdown: `SuggestTypes` profiles (notion-export) suggest per
csv collection from its id-stripped title only — csv rows are never parsed (schema evidence would
need the header line; a possible later increment).

**Observability.** Every adopted suggestion is an Info issue (`typeSuggested`): `database "Tasks"
pages imported as task (container name)`.

**Real-workspace validation** (the committed cassette): 9 of 35 databases match — Tasks / Notes /
People / Projects by name, "CRM (SB)" via email+phone, "90 Day Sprint Planner" / "Launch Tracker" /
"Tasks & Features" / "Approvals" via due-date+status — typing 64 of 368 pages, no visible false
positives. The counts are pinned in the cassette fidelity snapshot.

**Future increments** (behind the same seam): dataview `DefaultObjectTypeId` on the database
collection view (blocked on resolver passthrough + installed-type ids for bundled types); csv header
line as property evidence; per-page content signals; multilingual/learned matching.

**LLM enrichment.** The "learned model behind the same interface" iteration is section 15: an
optional BYOK (OpenAI-compatible) schema-plan step that sees
all container schemas before conversion and returns whole-workspace types *plus* property
normalization (bundled-key remaps, format fixes, cross-container merges). It generalizes this seam
into an injectable `Planner`; absent config or on any failure the import degrades to exactly the
naive rules above.

---

## 12. Package layout

```
core/block/importv2/
    importv2.go          // app component: CName, Init(seams), Run tracking, Close drain
    model/               // Object, Snapshot, Converter, Sink, IdentityClaim, Issue, Result, Request
    engine/              // orchestrator: passes, channel plumbing, journal, compensation, progress
    identity/            // index, payload store, futures, dedup queries, derivation
    resolve/             // reference rewriting + format registry
    persist/             // workers, create/update, install coordinator, provenance, seam interfaces
    source/              // re-readable streaming zip/dir/file with path sanitization
    markdown/            // converter (uses anymark, pkg/lib/schema)
    notion/              // converter
        client/          // Transport, FileFetcher, rate limiter, retry policy
        testdata/cassettes/
    adapter/             // Importer-facing adapter: pb translation, notification/event mapping
    enginetest/          // golden harness, fakes (in-memory Space), memory gauge
```

Plus: two `Config` bools, one guarded branch in `core/object.go`, one `Register(...)` line in
`bootstrap.go`, `.mockery.yaml` entries. v1 untouched.

---

## 13. Decisions that bind

These are settled and load-bearing; a change to any of them is a design change, not a bug fix.

1. **Collection membership on re-import replaces, it does not union.** A re-import converges to the
   source (v1 unioned, so a removed row never disappeared).
2. **Compensation covers created objects, not updated ones.** Under `ALL_OR_NOTHING` with
   `updateExistingObjects`, compensation deletes what the run created; objects it modified are
   journaled and reported as non-compensated. History-based restore is a possible later addition.
3. **Package layout.** The contract types (`Object`, `Snapshot`, `Converter`, `Sink`, `IdentityClaim`,
   `Issue`, `Result`, `Request`) live in the root package `importv2` — the name avoids a clash with
   `pkg/lib/pb/model`. The app component registered in bootstrap lives in `importv2/adapter`.
   Subpackages import the root; they never import each other's internals.
4. **The issue ledger's user surface is an object, not a popup.** A run that finishes with at least one
   issue creates an import report object in the target space (`importv2/report`), added to the root
   collection and root widget so it sits next to the imported content. Grouping is by
   **(severity, message)**, not by issue code: one code spans unrelated causes, and a reader groups by
   what happened rather than by taxonomy. The message is the constant half of an issue — whatever varies
   goes in `Issue.Subject` and `Issue.Count`, and a message that interpolates a value splits its own
   group. Objects appear by **name**, never by source key (the engine records what it called everything
   it emitted and hands the report a `Lookup`); objects it cannot name collapse into one counted line.
   That same table gates the mention mark: a key that resolves through the identity table but has no
   object behind it renders as `_missing_object`. Severity renders as an outcome — "Not imported",
   "Imported with changes", "Note" — and notes are counted apart from problems, because "these rows
   became Tasks" is a decision, not damage.
5. **Select cardinality follows UX, not v1 parity.** Notion single `select` maps to **status**, which is
   Anytype's single-select format, so pick-one survives the import; `multi_select` and `people` stay
   tag (`notion/properties.go:320`). v1 collapsed `select` into tag and the choice was irreversibly
   multi-valued afterwards.
6. **Export lossiness is out of scope.** Embeds, bookmarks, inline sets and option colors flattened by
   markdown export are an *exporter* problem. The v2 `anytype-export` flavour maximizes what import can
   recover, but it cannot restore data the exporter never wrote.
7. **Destination-owned identity and configuration are immutable to a normal import.** The space's name
   and icon, and existing types and their views, are mutable only under an explicit, separately
   requested full-restore mode. Persist rejects Workspace and Widget snapshots today; this binds the
   future pb converter.

## 14. Coexistence with v1

**v2 is the default engine** for Markdown, Obsidian and Notion: `ImportV2Markdown` and `ImportV2Notion`
both default to `true` (`core/anytype/config/config.go:272-273`) and `adapter.Handles`
(`importv2/adapter/adapter.go:216-224`) routes those three types to the engine. The kill switches are
`ANYTYPE_IMPORTV2MARKDOWN=false` and `ANYTYPE_IMPORTV2NOTION=false` — env-only, no build needed.

**v1 still serves** PB, CSV, HTML, TXT, Web and External, plus `builtinobjects` (the Pb and
AI-experience markdown paths) and `bookmarkimporter`.

**Differences from v1 that are intended, not parity bugs:**

- v2 derives stable relation and option keys and colors where v1 rolled random ones;
- collection membership on re-import replaces instead of unioning (§13.1);
- the orphan self-link block (v1 step 6, flagged "not understood" in v1's own code) is dropped;
- multi-path selections run per path instead of merged;
- `sourceFilePath` uses the same hash as v1, so cross-version re-import dedup keeps working.

`tests/integration/import_parity_test.go` runs one markdown fixture through both engines against real
accounts and compares engine-independent projections — pages, files, collections, custom relations,
options.

**Before v1 can be deleted:** `builtinobjects` and `bookmarkimporter`/`ImportWeb` must migrate onto the
engine (which needs a pb converter and a web converter, or their retirement), `ListImports`
(`core/block/import/importer.go:199`) must be reimplemented or dropped, and
`core/block/import/common/filetime` must be relocated — v2 imports it today
(`importv2/markdown/page.go`).

---


## 15. Schema planning — the model names kinds, code maps properties

Consolidated from the four planner design notes (always-mint, whitelist mapper, LLM
enrichment, reuse-existing-types), which this section replaces.

**The split.** An optional BYOK step (any OpenAI-compatible endpoint) sees only the *schema*
of the source — container names, property names and formats, and optionally a few sample
values — never the page content unless the caller opts in. It answers one question: which
containers hold the same *kind* of thing, and what is that kind called. Everything else —
which property maps to which Anytype relation, which keys are shared, what a type's
properties are — is computed in code (`schemaplan.CompleteKinds`). Property mapping was
tried as a model responsibility first and produced the failure modes below; moving it into
code removed a whole class of them at once.

```
containerSchemas()  →  llmplan kinds call  →  schemaplan.CompleteKinds  →  Plan  →  Sanitize
   (both converters)     one structured        pure code: type keys,        (unchanged   (the only
                         completion per        whitelist bundled targets,    shape)       trust
                         chunk of 8            kind-local property keys,                  boundary)
                         containers            union of member properties
```

`schemaplan.Plan` keeps its shape whatever produced it, so the naive planner, a scripted test
plan and an LLM plan are interchangeable and every downstream invariant is inherited rather
than re-implemented.

### 17.1 Always mint, but one type per kind

Every confident container gets a minted type; bundled types are never *redefined* (a plan
defining a document under a bundled key is re-keyed, not dropped). But minting must not
become one type per database: three task trackers are all tasks. A Notion database becomes
its own collection regardless, so containers sharing a type yields the right model — **N
collections over one type**, each keeping its identity as a list while the rows share a shape.

*Pointing* a container's pages at a bundled type is different from *defining* one under that
key, and is always allowed — it is what the built-in type suggestor has always done. Only the
prompt enforces always-mint, because `Sanitize` also governs the no-LLM paths.

### 17.2 A property belongs to its type

If two databases are different kinds, their "Status" columns are two different properties.
Sharing across types is the exception and must be requested explicitly by naming one of the
five whitelisted bundled targets. The scope is the container's type when it has one, else the
container itself.

This holds with no plan at all. The Notion converter used to dedup relations by name+format,
so same-named columns in unrelated databases collapsed into one relation with one shared
option pool — in the recorded workspace `Status` appeared in 18 databases, `Category` in 5.
Dedup is now by Notion property id, which still collapses the case that must collapse (a
database and its pages describing the same column). Markdown/Obsidian front matter still keys
by property name across a vault: without a plan a folder is not a type, so there is nothing
to scope the property to.

### 17.3 Type names are grammar, not model output

A type carries two names — "Project" for one member, "Projects" for a list — and the response
schema asks for both. Measured against the recorded 35-container workspace with a current
frontier model, 34 kinds: **14 came back with an empty plural, and 13 answered the singular
field with the Notion database's own title**, which is plural. In the editor that reads as a
blank "Type plural name" beside a type called "AI Projects".

So `Sanitize` derives the pair (`schemaplan/inflect.go`). A plural that adds nothing — absent,
or the same word — counts as unanswered. Only the head noun inflects; both sides of a
conjunction do; irregulars and uncountables come from deliberately short tables, because a
wrong guess reads worse than the rule it replaced. Three guards, each pinned by a name the
live run produced: a trailing bracketed tag is not the head noun ("Reminders (SB)" →
"Reminder (SB)"), a head that does not end in a letter never inflects ("Q1 2024",
"Connected,"), and a name cut at the length cap is its own plural rather than half a word
plus an s. Words that only look plural stay put: "Status", "Analysis", "Series", "Ideas".

The metric that should have caught this scored plurals with `EqualFold(name, plural)`, which
is false for an empty string, so a run of blank plurals looked clean. Empty plurals are their
own column now, and `IMPORTV2_DUMP_NAMES=1` prints each kind as raw pair → repaired pair.

### 17.4 Chunking, failure, and the trust boundary

The kinds call is split at 8 containers per request; chunk size and balance are the largest
quality lever measured so far, ahead of prompt wording. Starved runtimes fall back to
per-container one-field calls.

A planner failure is never fatal: the run degrades to the built-in rules with a warning on the
report (silently only when the run is being cancelled). `Sanitize` is a pure function and the
single trust boundary — it bounds display names, holds the closed icon vocabulary, re-keys
collisions, and drops entries naming things that do not exist. Nothing the model returns
reaches the space unexamined.

### 17.5 Reusing the user's existing types — designed, not built

An opt-in `reuseExistingTypes` was designed and deliberately not implemented; nothing in the
wire or the code carries it today. The design, if it is picked up: candidates are non-bundled,
non-archived types with at least one object, capped at ~40 most-used and passed *into*
`Sanitize` as data so it keeps no dependency on the space; a container may name a candidate
instead of defining a type; the container keeps its collection (single-database replacement
applies only to minted types, since a pre-existing type's objects are not this database's
rows); and the type itself is never rewritten — name, layout, icon and recommended relations
are the user's, the import may only add objects.

---

## 16. Durable runs — the ledger, compensation, resume

Consolidated from the durable-runs design note, which this section replaces. Shipped in
`core/block/importv2/runstore` and `resume`.

**Three drivers, all real.** Imports land in spaces with years of user data, so "throw the
space away" is not available and compensation must be precise. A large Notion crawl is
pacer-bound at 3 rps with ~2 requests per page — hours of work that a laptop lid-close used to
destroy. And a headless heart running imports for hosted orgs cannot be redeployed while any
import runs, because close cancelled every run and cancellation compensates.

**The fact that forces the shape.** A minted tree id is the hash of a root change containing
random seed bytes, so re-minting after a restart yields *different* ids — while objects
persisted before the crash carry references already rewritten to the old ones. Without the
identity ledger on disk, a resumed run can never reconnect to its own first half. Resume
cannot be bolted on later; the identity index and payload store are in scope from the start.

| | Decision |
|---|---|
| D1 | One any-store DB **per engine run**, in its own directory beside the run's file spill; disposal is `os.RemoveAll(runDir)` |
| D2 | Durable: run manifest, identity ledger (entries, create payloads, derived memo), effect/outcome ledger, files ledger, issues |
| D3 | Effect rows are write-through — one transaction per persisted object. Only pass-1 claims batch, because their loss is harmless: no side effects exist yet |
| D4 | Resume reloads the ledger and re-runs the passes with a skip set of persisted source keys; unfinished creates ride the existing tree-exists fallback, upgraded to heal-by-update on a resumed incarnation |
| D5 | `Close()` **suspends** — flush, mark suspended, return. Only user cancel compensates, and now durably, so a crash mid-cleanup resumes cleaning |
| D6 | The adapter sweeps run dirs at startup: finish deleting, finish compensating, or resume, with capped attempts |
| D7 | Compensation-critical fields are a version-frozen core, additive-only, so a future schema bump can still clean up an old run |
| D8 | Volatile mode remains: no run store means today's in-memory journal and identity, for tests and the golden harness |

The spill directory moves inside the run dir, so downloaded bytes survive a restart and are
collected with everything else instead of leaking from an OS temp dir.

---

## 17. Deferred materialization — fetch, convert, spool, then materialize

Consolidated from the deferred-materialization design note, which this section replaces.

**The requirement is product, not optimization.** The engine used to persist from the first
seconds, so objects trickled into the user's space for the entire multi-hour crawl. A space
that is half-built for three hours is the thing being forbidden.

```
pass 1  identity           claims, dedup match, local id minting
pass 2  fetch/convert/spool crawl, convert, download attachments, write to the run dir.
                            NOTHING enters the space.
pass 3  materialize         replay the spool: identity, reference resolution, tree creation,
                            file upload, journalled effects. No network beyond file sync.
finalize                    root collection, widget, report
```

Pass 3 runs at persist speed rather than network speed, so the visibly half-built window
shrinks from hours to minutes. Everything before it is abortable by deleting the run
directory: nothing to compensate, because nothing was done.

**The design move:** the spool reader implements the converter interface, so pass 3 *is* the
existing engine back-half — sink, workers, resolver, persister, journal — fed by a converter
that replays a spool instead of crawling. The bounded-channel backpressure and the memory
invariant apply to pass 3 unchanged; what this adds is a durable seam between the halves, not
a second pipeline.

**Scope of the guarantee: per engine run, not per user request.** A multi-path markdown
request runs N sequential engine runs, so path 1 has materialized before path 2 starts
fetching. Notion is a single run and gets the full property.

**Progress.** The phase vocabulary is shared by engine, converters and adapter. It is
deliberately coarse and carries **no blended overall percentage**: fetching is rate-limit-bound
at a couple of items a second while creating runs two orders of magnitude faster, so one
number averaging them would be a lie in both directions. The displayed `currentItem` is user
content — displayable, never loggable.

---

## 18. Notion fidelity — what the warnings were hiding

Consolidated from the fidelity review, which this section replaces. Measured against the
recorded workspace (403 pages, 35 data sources, 5248 blocks): 960 issues that turned out to be
11 distinct shapes.

**What Notion withholds.** 435 of the "unsupported block" warnings were buttons — Notion's own
API returns `{"type":"unsupported"}` for them and will never return more, so they are reported
as *Notion withheld this*, not as our failure, and leave no placeholder paragraph behind. 171
were linked database views, which the API returns as a `child_database` titled "Untitled". 135
were images and embeds whose external URL came back empty. 67 were people whose names the API
withholds when the integration's User Capabilities are off — a workspace setting, and the
report now says so instead of naming an id.

**Icons.** Notion's named icon set was inventoried in full (474 names, via the CDN's own
`icons/<name>_<color>.svg` oracle) and mapped to Anytype's. Named icons are a **type-only**
feature here — the API rejects them elsewhere and the client renders them only for the
object-type layout — so a page or callout carrying one gets the nearest emoji instead, from a
hand-checked table, while types take the icon itself. Objects with icons went from 5 to 250.

**Untitled pages.** 53 nameless rows: 47 genuinely empty (no name, no values, no content, no
icon — 45 of them from a single template) and referenced by nothing. Empty rows are archived
rather than skipped, because the database collection lists its rows as members before they are
fetched and dropping them would dangle those references. Nameless objects in the space: 53 → 8.

**The report.** Issues group by (severity, message) with per-object rollups and occurrence
counts, so 960 rows became 396 and the page went from 954 lines to about 200. Messages name
Notion pages, databases and properties, never ids; the resolver rewrites source keys to
mentions of the objects that were actually created.
