# Notion Import (`core/block/import/notion`)

Detailed reference for the Notion importer — the only converter that ingests from a **live HTTP API**
(using an integration token) rather than local files. Companion to `docs/ImportComponent.md`, which
covers the generic import pipeline this plugs into.

All paths below are relative to `core/block/import/notion/`.

---

## 1. Position in the system

- Registered converter `Name() == "Notion"` (`converter.go:24`), dispatched for `model.Import_Notion`.
- Constructed in `notion.New(collectionService)` (`converter.go:37`), which builds a single shared
  `client.Client` and the three sub-services: `search`, `database`, `page`.
- Input is **not a file path** — it is a Notion **integration token** from
  `RpcObjectImportRequest.NotionParams.ApiKey` (`converter.go:159`). There is no zip/dir/source layer.
- Token validation is a separate RPC (`Importer.ValidateNotionToken` → `notion.NewTokenValidator`,
  `validator.go:88`) that the frontend calls before starting an import.
- Output is the standard `*common.Response` (snapshots + root collection id + `CompactList` widget),
  consumed by the generic processor exactly like any other converter.

The importer performs a **full workspace crawl**: every database and page the integration can see is
discovered, fetched, converted, and imported. There is no incremental / selective mode.

---

## 2. Entry point — `GetSnapshots` (`converter.go:46`)

The spine, in order:

1. **Extract token** (`converter.go:48`); empty → error `"failed to extract apikey"`.
2. **Wire cancellation** (`converter.go:53`): a goroutine cancels the crawl `ctx` when
   `progress.Canceled()` or `progress.Done()` fires. Note it creates a *fresh* `context.WithCancel`
   from `context.Background()` — the incoming `ctx` is not chained in.
3. **Search** (`converter.go:62`): `search.Search(ctx, apiKey, 100)` returns all databases + all pages.
   Search failure always aborts, **even under `IGNORE_ERRORS`** — the user must fix the token/permissions.
4. **Compute progress total** (`converter.go:72`): `dbs×2 + pages×4 + uniqueProperties + 1`. The step
   weights are `numberOfStepsForDatabases=2`, `numberOfStepsForPages=4` (3 convert cycles + 1 create),
   `stepForSearch=1` (`converter.go:26`).
5. **Empty check** (`converter.go:78`): no dbs and no pages → `ErrNoObjectInIntegration`.
6. **Start file downloader** (`converter.go:82`): `files.NewFileDownloader(progress)`, `Init`, then
   `go fileDownloader.ProcessDownloadedFiles()`; `defer StopDownload()`.
7. **Create shared import context** (`converter.go:89`): `api.NewNotionImportContext()` — the mutable
   state threaded through everything (§5).
8. **Databases → collections + relations** (`converter.go:90`): `dbService.GetDatabase(...)` returns db
   snapshots **and a shared `PropertiesStore`** (`relations`) that pages reuse so a property common to a
   DB and its pages yields one relation object.
9. **Pages → objects** (`converter.go:99`): `pgService.GetPages(..., relations, ...)`.
10. **Attach pages to their DB collections** (`converter.go:118`): `AddPagesToCollections`.
11. **Build the root "Notion Import" collection** (`converter.go:120`): `AddObjectsToNotionCollection`.
12. Concatenate page + db snapshots, return `Response{Snapshots, RootObjectID, RootObjectWidgetType:
    CompactList}` (`converter.go:132`).

`ShouldAbortImport(0, req.Type)` is checked after the DB and page phases (`converter.go:95`, `:104`) so a
fatal error (rate-limit, cancel) stops before doing more work.

---

## 3. Network layer

### HTTP client — `api/client/client.go`

```go
notionURL  = "https://api.notion.com/v1"   // client.go:19
apiVersion = "2022-06-28"                   // Notion-Version header, client.go:20
```

`NewClient()` uses `&http.Client{Timeout: time.Minute}` — a **hard 1-minute per-request timeout**
(`client.go:39`). `PrepareRequest` (`client.go:46`) sets `Authorization: Bearer <token>`,
`Notion-Version: 2022-06-28`, and `Content-Type: application/json` only when there's a body.

**`DoWithRetry(loggerInfo, maxAttempts, req)` (`client.go:78`)** — the retry engine:

| Aspect | Behavior |
|---|---|
| Retries on | network `net.Error`, HTTP **429**, and any **≥500** |
| Non-retryable | non-`net.Error` transport errors return immediately; **4xx other than 429** (400/401/404…) are returned as a normal response for the caller to inspect — *not* retried |
| Base delay | 5s, **doubled** each attempt (`delay *= 2`) |
| `Retry-After` | if present and >0, overrides the current delay to exactly that many seconds (`GetRetryAfterError`, `client.go:65`) |
| Body replay | request body is `io.ReadAll`-buffered once, re-wrapped in a fresh `NopCloser` each attempt |
| `maxAttempts` | `0` = unlimited; **Search passes 3** (`search.go:66`); block/page fetches also go through this |
| Cancellation | backoff `select`s on `req.Context().Done()` |

Two landmines: after `Retry-After` overrides the delay, the value is **still doubled** once the wait
elapses (so a subsequent header-less 429/5xx backs off from `2×Retry-After`); and on hitting
`maxAttempts`, `DoWithRetry` returns the **last `(res, err)`** — for a 5xx that is a non-nil `res` with
`err == nil` (body not auto-closed), so callers must inspect `res.StatusCode`. **Block-children and
paginated-property fetches pass `maxAttempts=0` = unlimited** — see §11.

### Error mapping — `api/client/error.go`

`NotionErrorResponse{Status, Code, Message}`. `TransformHTTPCodeToError` (`error.go:17`):
- unparseable body → returns `nil` (caller falls back to a generic status-code error);
- `Status ≥ 500` → wraps `common.ErrNotionServerIsUnavailable`;
- `Status == 429` → wraps `common.ErrNotionServerExceedRateLimit`;
- else a generic `status/code/message` error.

**`ErrNotionServerExceedRateLimit`** is surfaced to the user **regardless of import mode** (it is in the
`shouldReturnError` unconditional set, `processor.go:509`). `ErrNotionServerIsUnavailable` is **not** in
that set: under `IGNORE_ERRORS` it is merged as non-fatal (or masked by the generic "no resources" error);
it aborts only under `ALL_OR_NOTHING`. (Both appear in `isDefinedError`, but that only selects which error
`GetResultError` reports — it is not the mode-independence gate.)

### Token validation — `validator.go`

`Ping` (`validator.go:42`) calls `GET /users?page_size=1` and maps the status to
`ErrorUnauthorized` (401) / `ErrorForbidden` (403) / `ErrorNotionUnavailable` (503/504) / `ErrorInternal`.
`TokenValidator.Validate` (`validator.go:96`) translates those into the
`RpcObjectImportNotionValidateTokenResponseError_*` codes returned to the frontend.

### Search — `api/search/search.go`

`Search(ctx, apiKey, pageSize)` (`search.go:40`) paginates `POST /search` with `page_size=100`:
- loops on `has_more`, threading `start_cursor`/`next_cursor`;
- splits results by the JSON `"object"` discriminator (`"database"` vs `"page"`) via a
  marshal→unmarshal round-trip into `database.Database` / `page.Page` (`search.go:92`);
- `maxAttempts=3`; non-200 → `TransformHTTPCodeToError` or a generic status error.

Returns **all** databases and pages up front — the whole workspace index is held in memory.

---

## 4. Shared import context — `api/context.go`

`NotionImportContext` is the mutable state shared across the entire crawl (`context.go:44`):

```go
type NotionImportContext struct {
    NotionPageIdsToAnytype     map[string]string // Notion page id  → new Anytype object id
    NotionDatabaseIdsToAnytype map[string]string // Notion db id    → new Anytype object id
    PageNameToID               map[string]string // (page id → title, despite the name)
    DatabaseNameToID           map[string]string
    PageTree                   *PageTree         // parent id → child page ids (RWMutex-guarded)
    BlockToPage                *BlockToPage       // parent block id → page id (RWMutex-guarded)
}
```

**The load-bearing design decision:** every page is assigned its final Anytype id **before any block
conversion runs** (`page.go:159`, `NotionPageIdsToAnytype[p.ID] = bson.NewObjectId().Hex()`; databases
similarly at `database.go:127`). Because all ids exist up front, **forward references resolve** — a page
that mentions a page processed later still links correctly. This is what makes the whole
mention/link/relation remap work without a second pass.

Only `PageTree` and `BlockToPage` carry mutexes; the four top-level id/name maps are written
single-threaded (during search/db/page setup) and only *read* during the parallel page conversion.

---

## 5. Pages → objects

### Orchestration — `api/page/page.go`

`GetPages` (`page.go:64`):
1. `fillNotionImportContext` (`page.go:154`) pre-assigns page ids + builds the parent/child `PageTree`
   and `PageNameToID` (see §4).
2. Worker pool of **10** (`workerPoolSize`), dropping to **1** when `len(pages) < 10` (`page.go:80`).
3. `addWorkToPool` (`page.go:121`) wraps each page in a `Task` and **sleeps 5ms between submissions**
   (`page.go:138`) as a rate-limit defense; shares two mutexes (`relMutex`, `relOptMutex`) so
   relation/option creation is serialized across workers.
4. `readResultFromPool` (`page.go:99`) drains results, one `progress.TryStep(1)` each; `ALL_OR_NOTHING`
   stops the pool on first error.

### Per-page conversion — `api/page/task.go`

`Task.Execute` (`task.go:55`) → `makeSnapshotFromPages` (`task.go:86`):
1. `provideDetails` — map Notion page properties to Anytype details + relation/option snapshots.
2. `GetBlocksAndChildren(ctx, p.ID, apiKey, 100, mode)` — recursively fetch the full block tree.
3. `MapNotionBlocksToAnytype` — convert Notion blocks to Anytype blocks.
4. `uploadFilesLocally` — queue file/icon block URLs for download, swap to local paths.
5. `provideSnapshot` — assemble the `StateSnapshot`.

The page snapshot is `SbType = SmartBlockTypePage`, `ObjectTypes = [TypeKeyPage]`, `Id` = the
pre-assigned Anytype id (`task.go:64`, `:172`). Each relation/option becomes its own snapshot
(`SmartBlockTypeRelation` / `SmartBlockTypeRelationOption`, `task.go:398`).

**Page details** (`prepareDetails`, `task.go:178`): `SourceFilePath = Notion page id`. An **emoji** icon
sets `IconEmoji` and is **not** queued for download (`SetIcon` returns no relation link); only a
**file/external** icon (`IconImage`) and the cover become `RelationFormat_file` relations queued for
download. Also `IsArchived`, `IsFavorite=false`, `LastModifiedDate`/`CreatedDate`,
`ResolvedLayout = ObjectType_basic`. **The page name comes from the Notion `title` property**, not from
explicit code — `TitleItem.SetDetail` always writes `RelationKeyName` (§7).

---

## 6. Block conversion

### Fetch + recurse — `api/block/retrieve.go`

`GetBlocksAndChildren` (`retrieve.go:48`) fetches `/blocks/{id}/children` paginated (`page_size`,
`start_cursor`, **5ms sleep between pages**, `retrieve.go:119`) and **recurses** into any block that is a
`ChildSetter` with `HasChild()==true` — **no depth limit** (bounded only by the actual Notion tree).
`fillBlocks` (`retrieve.go:128`) is the string-type → concrete-Go-struct switch; `MapBlocks`
(`mapper.go:20`) then dispatches each struct's `GetBlocks(req, pageID)` polymorphically.

### Block-type mapping table

| Notion block | Anytype result | File |
|---|---|---|
| `paragraph` | text `Paragraph` | `text.go:20` |
| `heading_1/2/3` | text `Header1/2/3` (recurses only if `IsToggleable`) | `text.go:61/88/103` |
| `bulleted_list_item` | text `Marked` | `text.go:298` |
| `numbered_list_item` | text `Numbered` | `text.go:225` |
| `to_do` | text `Checkbox` (+`Checked`) | `text.go:254` |
| `toggle` | text `Toggle` | `text.go:327` |
| `quote` | text `Quote` | `text.go:196` |
| `callout` | text `Callout` (+ icon → `IconEmoji`/`IconImage`) | `text.go:140` |
| `code` | text `Code` (lang in `Fields["lang"]`); `mermaid` → Latex `Mermaid` | `text.go:380` |
| `equation` | Latex block | `text.go:436` |
| `divider` | `Div` | `div.go:15` |
| `table_of_contents` | `TableOfContents` | `tableofcontent.go:21` |
| `table` + `table_row` | full Anytype table subtree (column/row layout blocks, deterministic cell ids) | `table.go:50` |
| `column_list` + `column` | Anytype `Row`/`Column` layout subtree | `columns.go:31` |
| `image` / `file` / `pdf` | file block (`Image`/`File`/`PDF`) | `file.go:37/24/50` |
| `video` | file block `Video`, **or** Latex `Youtube`/`Vimeo` if URL matches | `file.go:63` |
| `audio` | file block `Audio`, **or** Latex `Soundcloud` if URL matches | `file.go:107` |
| `embed` | Latex (Google Maps/Miro/SoundCloud/GitHub Gist/Codepen), else `LinkToWeb` text link | `link.go:36` |
| `link_preview` | text block with a `Link` mark | `link.go:91` |
| `bookmark` | `Bookmark{Url, Title}` (title from caption) | `link.go:253` |
| `child_page` | `Link` to resolved Anytype page, else error text block | `link.go:132` |
| `child_database` | `Link` to db, or inline collection `Dataview` if unresolved | `link.go:179` |
| `link_to_page` | `Link` to page/db Anytype id | `link.go:224` |
| `unsupported` / `template` / `synced_block` | placeholder text block `"Unsupported block"` | `block.go:76`, `retrieve.go:331` |
| *any type not in the switch* | **silently dropped** (no default case) | `retrieve.go:128` |

### Rich text, marks, mentions — `api/block/text.go`, `textobject.go`

`GetTextBlocks` (`textobject.go:32`) walks a block's `RichText`:
- Annotations → marks (`BuildMarkdownFromAnnotations`, `commonobjects.go:63`): Bold, Italic,
  Strikethrough, Underline→Underscored, Color→Text/BackgroundColor, Code→Keyboard.
- Offsets computed in **UTF-16** (`textUtil.UTF16RuneCountString`) — matches Notion/JS semantics.
- **Link remap** (`handleTextType`, `textobject.go:101`): an intra-workspace URL is rewritten to the
  target's Anytype id via `NotionDatabaseIdsToAnytype`/`NotionPageIdsToAnytype`.
- **Mentions** (`handleMentionType`, `textobject.go:122`):
  - page/database mention → a `Mention` mark whose `Param` is the target **Anytype id**;
  - date mention → a zero-width `Mention` mark pointing at a generated date-object id
    (`dateutil.BuildDateObjectFromId`);
  - user mention → plain text; link-preview → text + `Link` mark.
- Color containing `"background"` → block `BackgroundColor`, else text color.

### Child-page name resolution — `getTargetBlock` (`link.go:299`)

Notion's `child_page`/`child_database` blocks give a **title, not an id**. `getTargetBlock` walks the
`PageTree` (children of the parent block → current page → top-level) matching by title, and **deletes the
matched child from the tree** so duplicate-titled siblings each resolve to a distinct page. Ambiguous
matches across all pages produce an error text block.

### Block file uploads — `task.uploadFilesLocally` (`task.go:101`)

For each file block (URL sits in `File.Name`) and each callout icon (`Text.IconImage`), the URL is
queued via `fileDownloader.QueueFileForDownload`, then a goroutine `WaitForLocalPath()`s and swaps the
URL for the local temp path. All such tasks run concurrently, joined by a `WaitGroup`.

---

## 7. Databases → collections & properties → relations

### Database → collection — `api/database/database.go`

Each Notion database becomes a **Page smartblock carrying the Collection type/layout**
(`provideDatabaseSnapshot`, `database.go:303`): `ObjectTypes=[TypeKeyCollection]`,
`ResolvedLayout=ObjectType_collection`, `Collections = st.Store()`, `SourceFilePath = db id`. Cover/icon
are downloaded (`UploadFileRelationLocally`). `GetDatabase` (`database.go:71`) creates **one shared
`PropertiesStore`** returned to the page phase.

The collection also carries `Description` (newline-joined rich text), `Creator`/`LastModifiedBy` as plain
**name strings** (not linked people), `IsArchived`, `IsFavorite=false`, and dates (`database.go:271`). Its
`Name` is `d.Title[0].PlainText` — the **first title run only** (`database.go:275`), whereas a *page*
title joins all runs; multi-run database titles are truncated.

Each database schema property becomes a standalone **relation object snapshot**
(`makeRelationsSnapshots`, `database.go:140`; relation object = `SmartBlockTypeRelation`,
`ObjectTypes=[TypeKeyRelation]`), wired into the collection's dataview. Deduped three ways via
`provideRelationSnapshot` (`database.go:217`): by name+format, by Notion property id, else created new.

Two more schema details a rewrite must reproduce:
- **Zero-valued details are seeded onto the collection object.** Every schema property calls
  `databaseProperty.SetDetail(relKey, st.Details())` (`database.go:197`), writing an empty value (`""`,
  `0`, `false`, `[]`) so the relation is present on the collection even when no page fills it; the title
  writes a blank `Name`.
- **Dataview wiring uses two functions.** `Name` and the redirected Tag relation already exist (hidden) in
  the default dataview, so they are surfaced via `ReplaceRelationsInDataView` (`collection.go:160` →
  `IsVisible:true`); every *other* schema relation is genuinely new and added via `st.AddRelationLinks` +
  `AddRelationsToDataView` (`database.go:209`). Getting this split wrong duplicates or hides columns. Page
  objects carry **no dataview** — pages emit only details + `RelationLinks`.

### Property value → relation format table — `api/property/propertyitem.go`

Every Notion property **value** on a page becomes a **detail**; its relation gets a format:

| Notion property | Detail value written | Relation format | File |
|---|---|---|---|
| `title` | joined plain text → **always `RelationKeyName`** | shorttext | `propertyitem.go:69` |
| `rich_text` | joined plain text | longtext | `:103` |
| `number` | float | number | `:137` |
| `select` | `[option id]` | tag | `:162` |
| `multi_select` | `[option ids]` | tag | `:191` |
| `status` | `[option id]` | status | `:588` |
| `people` | `[person ids]` | tag | `:326` |
| `date` | unix int64 | date | `:218` |
| `created_time` / `last_edited_time` | unix int64 | date | `:486` / `:537` |
| `created_by` / `last_edited_by` | person **name string** (not linked) | shorttext | `:514` / `:565` |
| `checkbox` | bool | checkbox | `:388` |
| `url` | string | url | `:411` |
| `email` | string | email | `:436` |
| `phone_number` | string | phone | `:461` |
| `files` | `[urls]` (queued for download) | file | `:357` |
| `relation` | `[linked Anytype ids]` | object | `:294` |
| `formula` | string/number/bool stringified (**date formula dropped**) | shorttext | `:251` |
| `rollup` | number / date / flattened array (lossy — see §12) | number/date/tag/longtext | `:629` |
| `unique_id` | `"PREFIX-123"` | longtext | `:721` |
| `verification` | **no-op — dropped** | date | `:701` |

### Options & the Tag special case

`select`/`multi_select`/`status`/`people` options become **RelationOption** snapshots — created during
*page* processing (`task.go:444`), colour via `NotionColorToAnytype`. The option's `.ID` is mutated in
place to the new Anytype option id so `SetDetail` emits Anytype ids.

**Options dedup across the entire import**, not just within a page. `PropertiesStore` actually holds
**three** maps (`relationsmaps.go`): name+format, Notion property id, and `RelationsIdsToOptions`
(relation key → its option snapshots). Because the single store is created in `GetDatabase` and threaded
into `GetPages`, `isOptionAlreadyExist` (`task.go:537`) matches `(relationKey, optionName)` against
options created on **any earlier page** — so each select/status/people value yields exactly one
RelationOption workspace-wide (and all Tag options collapse into the bundled Tag relation).

**Tag redirection is narrower than "Tag/Tags → Tag":** `IsPropertyMatchTagRelation` (`common.go:14`) fires
for a `select`/`multi_select` named exactly `"Tag"`, or `"Tags"`/`"tags"` **only when no `"Tag"` property
exists**; `status` and `people` are **never** redirected (though they still produce options). Only the
**first** match wins — a second `"Tag"` select becomes an ordinary relation. The DB side matches the
property `Name`, the page side the properties-map **key**; if those diverge the two sides split the
relation. A `people` value with a blank name is skipped and its raw Notion user id is left in the detail
as a **dangling value** (`task.go:465`).

### Relation-property linking — `handleLinkRelationsIDWithAnytypeID` (`task.go:327`)

For a `relation` property, each linked Notion page/db id is rewritten in place via
`NotionPageIdsToAnytype`/`NotionDatabaseIdsToAnytype`. Ids not imported stay as raw Notion ids.

Note the fetch cost: `rich_text` and `people` properties carry no item-level `has_more`, so the importer
**always re-fetches** them via `GET /pages/{id}/properties/{id}` when non-empty (`isPropertyPaginated`,
`task.go:429`) — an extra round-trip per such property per page. Only `relation` is re-fetched
conditionally, on `HasMore`; empty values skip the call.

### Dropped at the schema level

Database-schema `formula` and `rollup` map to `nil` (no format info in the schema JSON) — they only
materialize from *page values* (`databaseproperty.go:67`). Schema `phone_number` is mapped to `number`
format (a quirk; page values still use `phone`).

---

## 8. File downloads — `api/files/`

A dedicated subsystem, independent of the API client.

- **Pool**: `workerpool.NewPool(5)` (`downloader.go:14`), started in `Init`; `ProcessDownloadedFiles`
  consumes results.
- **Temp dir**: `os.TempDir()/anytype_notion_import/` (`downloader.go:15`), `0700`.
- **Queueing** (`QueueFileForDownload`, `downloader.go:61`): non-blocking cancel check, then three dedup
  layers — completed `sync.Map`, in-progress `sync.Map`, and the filesystem.
- **Naming/dedup** (`file.go:180`): filename = `hex(blake3(url.Path)) + ext` — **the query string is
  excluded**, so the same S3 object with different signature params dedups to one file. Written as
  `_<name>` then atomically `os.Rename`d to `<name>` on success.
- **Download transport** (`file.go:115`): bare `http.DefaultClient` — **no auth, no retry, no timeout**
  (Notion file URLs are pre-signed S3 links). Protected only by ctx cancellation and a **30s stall
  monitor** (`monitorFileDownload`, `file.go:195`) that fails a download making no byte progress.
- **Sync point**: `WaitForLocalPath()` (`file.go:70`) blocks the relation/block upload tasks until the
  file lands, then the URL in details/blocks is swapped for the local path.

> Two distinct HTTP paths: **API** calls (search/pages/blocks/databases) use the retrying
> `client.Client`; **file** downloads use bare `http.DefaultClient`. A reimplementation should decide
> whether that asymmetry is intentional (it means transient file-download failures are not retried).

Two more gaps a rewrite should address:
- **Temp dir is never cleaned** — `$TMPDIR/anytype_notion_import/` and its files persist across imports and
  restarts (`StopDownload` does not remove them), a monotonic disk leak. Worse, `generateFile` treats any
  pre-existing final file as already-downloaded (`os.ErrExist`), so a later import producing the same
  `blake3(url.Path)` name **silently reuses stale bytes** from a prior run.
- **Concurrent collision** — the dedup `sync.Map`s key on the *full* URL but the filename keys on
  `blake3(url.Path)` (query excluded). Notion re-signs URLs per response, so two different signed URLs for
  the same object bypass URL dedup and can download concurrently, both `os.Create`-ing the same `_<name>`
  temp and racing the `os.Rename` — a corruption risk, not just wasted work. FS-existence dedup only
  helps when one download fully lands before the other starts.

---

## 9. Collections & nesting

- **`AddPagesToCollections`** (`database.go:323`): each page/nested-db whose parent database was imported
  is added to that database's collection store.
- **`AddObjectsToNotionCollection`** (`database.go:346`): builds the root **"Notion Import"** collection
  via `common.NewImportCollection(...).MakeImportCollection(settings)`. `filterObjects` (`database.go:365`)
  adds an object only if it is
  *orphaned* — parented to the workspace, or to a database/page/block that was **not** itself imported —
  preventing double-nesting of objects that already live inside an imported db/page.
- Root widget layout is `CompactList` (`converter.go:137`).

---

## 10. Output shape

A Notion import produces a flat `[]*common.Snapshot` mixing:
- **Page** snapshots (`SmartBlockTypePage`, `ObjectType_basic`) with blocks + property details;
- **Collection** snapshots (`SmartBlockTypePage`, `ObjectType_collection`) — one per database + one root;
- **Relation** snapshots (`SmartBlockTypeRelation`) — one per unique property (deduped);
- **RelationOption** snapshots (`SmartBlockTypeRelationOption`) — for select/multiselect/status/people;
- the bundled **Tag** relation is reused rather than recreated.

These then flow through the generic pipeline (`docs/ImportComponent.md` §5): id assignment (relations →
options → files ordering), parallel creation, link/relation remap, file sync.

---

## 11. Error handling & modes

- Search failure and empty integration always abort (even under `IGNORE_ERRORS`) via
  `ErrNoObjectInIntegration` and the `shouldReturnError` gate.
- Rate-limit (`ErrNotionServerExceedRateLimit`) aborts **regardless of mode**; unavailable
  (`ErrNotionServerIsUnavailable`) aborts only under `ALL_OR_NOTHING`. Both map to notification error codes.
- **Block-children and paginated-property fetches retry unbounded** (`DoWithRetry(..., 0, ...)` =
  unlimited, `retrieve.go:395`, `propertyobject.go:175`) — only Search caps at 3. A persistently
  failing (429/5xx) endpoint loops with 5s→×2 backoff until `ctx` cancellation; the 1-minute client
  timeout is **per-attempt**, not a total budget, so a single bad page can stall a worker indefinitely.
- On an `ALL_OR_NOTHING` abort, `pool.Stop()` stops result consumption but does **not** interrupt a worker
  already inside `Execute`, and this path does **not** cancel the crawl `ctx` — so in-flight (unbounded-retry)
  block/property fetches keep hitting the API until `progress` later fires the ctx cancel.
- **File-download failures are swallowed**: a failed `WaitForLocalPath` is logged and the block/relation is
  left with an empty local path (`""`) — silent data loss that occurs **even under `ALL_OR_NOTHING`** (the
  downloader never feeds `ConvertError`).
- Per-page and per-property errors under `IGNORE_ERRORS` are logged and skipped; under `ALL_OR_NOTHING`
  the first error stops the page pool.
- Cancellation propagates through `progress.Canceled()`/`Done()` → crawl ctx cancel → in-flight requests +
  file downloads abort. But the crawl `ctx` is built from `context.Background()` (not chained to the
  caller/app ctx) and `cancel` is only ever called from the watcher goroutine (never `defer`red): so
  **app/caller ctx cancellation does not stop the crawl** — only user cancel via `progress` does — and if
  `progress` never signals, the watcher goroutine + context leak. The database phase ignores `ctx`
  entirely (`GetDatabase(_ context.Context, …)`).

---

## 12. Known quirks & data loss

**Block-level data loss & mis-nesting:**
- **Silently dropped blocks**: any Notion block type not in the `fillBlocks` switch is lost with no
  placeholder (only `unsupported`/`template`/`synced_block` get a `"Unsupported block"` text block). A
  `synced_block`'s real nested content is **never fetched** (it doesn't implement `ChildSetter`) — synced
  content is fully lost.
- **Toggleable-heading children are flattened**: a collapsible heading calls `GetTextBlocks(..., nil, ...)`
  so the heading's `ChildrenIds` stays `nil` and its recursed children are merged as **siblings**, not
  nested (`text.go:62`). Notion heading-containment is lost.
- **`to_do` two-parent nesting** *(latent bug)*: `to_do` sets its `ChildrenIds` to the entire flat
  descendant list (`childResp.BlockIDs`, `text.go:260`) instead of `getChildID`-filtered top-level ids, so
  a nested grandchild ends up referenced by two parents.
- **Inline equations are extracted**: an `equation` run inside a paragraph becomes a standalone Latex block
  placed *before* the text block; if a run is a single equation, `isNotTextBlocks` suppresses the text
  block entirely.
- **Captions dropped** for `code` and file/image/pdf/video blocks (parsed but never emitted); only
  `bookmark` uses its caption (as title).
- **Table headers mishandled** *(likely bug)*: `has_column_header` is parsed but ignored; `has_row_header`
  is applied to the first *row* (`table.go:135`) — the inverse of Notion semantics.
- **Marks are not coalesced** across adjacent rich-text runs (two adjacent bold spans → two Bold marks).
- **`link_preview` mention** writes a `Link` mark whose `Param` is **empty** (no target); page/db mentions
  only get a `Mention` mark on the happy path (title resolved) — otherwise plain text / a `Link` to
  `Href` / the literal `"Can't access object from Notion"`.

**Property / value data loss:**
- **`verification` and date formulas** are dropped; **`created_by`/`last_edited_by`** and DB
  `Creator`/`LastModifiedBy` import as plain **name strings**, not linked people.
- **Date ranges (`End`) and `time_zone` are dropped** — only `Start` is imported (`propertyitem.go:225`).
  `ConvertStringToTime` accepts only RFC3339 / date-only and returns **`0` (epoch)** otherwise, so a
  malformed date silently imports as 1970. A **date mention** whose value carries a *time* component
  (`…T09:00…`) fails `BuildDateObjectFromId` and is dropped entirely (no mark, no text).
- **Rollup `array` is lossy**: only scalar string/bool/number values + `TitleItem` names are kept; nested
  lists/date-objects are dropped. Its format is `tag` but the values are **raw strings, not option ids**,
  and no RelationOptions are created — a tag relation with unbacked values (`propertyitem.go:654`).
- **Schema `formula`/`rollup`** produce no relation from the DB schema (only from page values); schema
  `phone_number` gets `number` format. Unnamed properties default to `"Untitled"`.

**Icons & colours:**
- Only unicode emoji and file/external icons are handled; Notion **`custom_emoji` (named) icons are
  unmodeled and dropped** for callouts and page/db icons.
- `NotionColorToAnytype`: **`brown`/`brown_background` → `""`** (dropped to default); `green→lime`,
  `gray→grey` are renamed.

**Resilience & misc** (details in §11/§13):
- **File downloads have no retry/timeout**; the 30s stall monitor **cannot interrupt a hung read** (it is
  checked only after `io.Copy` returns), so a half-open connection blocks a worker until `ctx` cancel.
- A `has_more:true` response with a **null `next_cursor` nil-panics** the crawl (`*NextCursor` deref in
  `search.go:124` / `retrieve.go:118`) — on a worker goroutine, taking down the process.
- **No recursion depth cap** on block-tree fetch; rate-limit defenses are only the 5ms sleeps + client
  retry/backoff.
- `PageNameToID` is actually keyed by page id (maps id→title) despite the name.

---

## 13. Memory & performance

- **Whole workspace is resident**: search loads every db+page index; each page's full block tree is
  fetched and converted in memory; every snapshot (pages, collections, relations, options) is retained
  until the generic pipeline runs — on top of the generic pipeline's own all-snapshots-in-memory model.
  Search is the memory-dominant step: `defer res.Body.Close()` sits **inside** the pagination loop but
  runs only when `Search` returns, so every page's body is retained for the whole crawl, and each result
  is **marshalled then unmarshalled** into a typed struct (a double-parse over the entire index).
- **Concurrency**: 10-worker page pool (1 for small imports) + 5-worker file-download pool, both with
  unbuffered channels (backpressure). Relation/option creation is mutex-serialized across page workers.
- **Rate-limit posture**: 5ms stagger on page submission and block pagination; API retries with 5s→×2
  backoff honoring `Retry-After`; file downloads land on disk (temp dir), not memory.
- The dominant costs are the sequential-ish API crawl latency (bounded by Notion rate limits) and holding
  the entire converted workspace before creation begins.

---

## 14. Notes for a reimplementation

**Preserve (correctness-critical):**
- Assign all Anytype ids **before** conversion so forward mentions/links/relations resolve in one pass —
  or replace with an explicit two-pass remap.
- The mention/link/relation remap (URLs, `Mention` mark `Param`, relation values, child-page title
  resolution) — this is the bulk of Notion-specific value and the easiest to break.
- UTF-16 offset math for text marks.
- The Tag-relation redirection and property/option dedup via the shared `PropertiesStore`.
- Always-fatal handling of rate-limit/unavailable/auth errors, and the token-validation RPC.

**Candidates to improve:**
- Stream the crawl → convert → create rather than holding the whole workspace (largest memory win;
  requires the id map to still be complete before link resolution — e.g. pre-fetch the page/db id index
  first, then stream bodies).
- Give file downloads the same retry/timeout discipline as API calls.
- Replace magic step-weights and the 5ms sleeps with a proper rate limiter driven by `Retry-After`.
- Add a recursion depth guard on the block tree.
- Decide the fate of currently-dropped data (unknown blocks, verification, date formulas, people
  identities) explicitly rather than silently.

### File index

| Area | Files |
|---|---|
| Spine / token | `converter.go`, `validator.go` |
| Network | `api/client/{client,error}.go`, `api/search/search.go` |
| Shared state | `api/context.go`, `api/commonobjects.go` |
| Pages | `api/page/{page,task}.go` |
| Blocks | `api/block/{block,mapper,retrieve,text,textobject,file,table,columns,div,link,tableofcontent}.go` |
| Databases | `api/database/database.go` |
| Properties | `api/property/{propertyitem,databaseproperty,propertyobject,relationsmaps,common}.go` |
| Files | `api/files/{downloader,file}.go` |
</content>
