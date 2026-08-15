# Import V2 — Client Integration Guide

Audience: anytype-ts / anytype-swift / anytype-kotlin developers.
Scope: the rewritten import engine (Markdown, Obsidian, Notion) and the optional BYOK LLM structure
enrichment on top of it.

Engineering references: [ImportV2Design.md](ImportV2Design.md) (engine), [ImportV2LLM.md](ImportV2LLM.md)
(LLM plan phase), [ImportErrorCodes.md](ImportErrorCodes.md) (error-code mapping).

---

## 1. TL;DR for clients

- **The RPC surface is unchanged.** `ObjectImport` is still fire-and-forget, still async, still
  reports through the same process events and the same `NotificationImport`. An existing client
  keeps working against the v2 engine with **zero changes**.
- **Three fields are new** and all are optional:
  - `NotificationImport.reportObjectId` + `.issuesCount` — an in-space **import report page**
    listing everything the import decided or could not do.
  - `EventImportFinish.reportObjectId` + `.issuesCount` — the same, on the completion event.
  - `Rpc.Object.Import.Request.aiParams` (field 16) — opt-in LLM structure enrichment.
- **What the user gets for free** (no client work): better object typing on Notion databases and
  Obsidian folders, per-object issue reporting instead of a single opaque error, and correct
  single-select cardinality on Notion `select` columns.
- **What is worth client work**: surfacing the report page (§6), and the AI settings UI (§7).

---

## 2. Which imports go through V2

Routing happens inside `ObjectImport` per `req.type`, gated by config flags:

| `model.Import.Type` | V2? | Flag (env) |
|---|---|---|
| `Markdown` (1) | yes | `ImportV2Markdown` / `ANYTYPE_IMPORTV2MARKDOWN` |
| `Obsidian` (7) | yes | `ImportV2Markdown` / `ANYTYPE_IMPORTV2MARKDOWN` |
| `Notion` (0) | yes | `ImportV2Notion` / `ANYTYPE_IMPORTV2NOTION` |
| `Html`, `Txt`, `Csv`, `Pb`, `External` | no — v1 | — |

While the flags are off, every import runs on v1 exactly as before. The flags are middleware-side;
clients cannot set them and must not depend on which engine served a request. **Write client code so
it works on both** — the v2-only fields are additive and arrive empty from v1.

`ObjectImportList` and `ObjectImportUseCase` are untouched.

---

## 3. Starting an import

```
Rpc.Object.Import.Request {
  spaceId          = 14   // required
  type             = 10   // model.Import.Type
  mode             = 11   // ALL_OR_NOTHING (0) | IGNORE_ERRORS (1)
  noProgress       = 12
  isNewSpace       = 15
  updateExistingObjects = 9
  aiParams         = 16   // NEW, optional — see §7
  oneof params { notionParams | markdownParams | ... }
}
```

Per-format params used by v2:

| Format | Params |
|---|---|
| Notion | `notionParams.apiKey` (required) |
| Markdown / Obsidian | `markdownParams.path[]` (required), `createDirectoryPages`, `includePropertiesAsBlock`, `noCollection` |

Notes that matter in practice:

- **`path` accepts a directory or a `.zip`.** Multiple paths run as independent sequential imports
  (one root collection each) — this is a known parity gap with v1's merged run.
- **`type = Obsidian`** forces the Obsidian dialect profile. Plain `Markdown` auto-detects the
  dialect from the file listing (Notion export / Obsidian vault / Anytype export / generic) and
  reports which one it picked as an info issue on the report page. Send `Obsidian` when the user
  explicitly picked "Obsidian" in the UI; send `Markdown` otherwise.
- **`mode`**: `ALL_OR_NOTHING` rolls the run back on the first fatal error (objects created so far
  are deleted); `IGNORE_ERRORS` keeps going and reports per-object failures. v2 honors both.
- **The response is empty and deprecated.** `Rpc.Object.Import.Response.error`, `.collectionId`,
  `.objectsCount` are never populated for async imports. Do not read them; wait for the events.

---

## 4. Lifecycle: process, progress, cancellation

```
ObjectImport (returns immediately, empty response)
        │
        ├── Event.Process.New      { process.id, spaceId, message = Import{} }
        ├── Event.Process.Update   { progress.total, .done, .message }   ×N
        ├── Event.Process.Done
        │
        ├── Event.Import.Finish    { rootCollectionID, objectsCount, importType,
        │                            reportObjectId, issuesCount }
        └── Notification (payload = NotificationImport)
```

**Progress messages.** `progress.message` carries a human phase name; v2 emits, in order:
`"Scanning source"` → `"Importing objects"` → `"Finalizing"`. Treat the string as a display hint,
not an enum — phases may be added (the LLM analysis phase is a planned addition, see §11).

**Totals move.** `total` is set after the scan pass and may be *raised* mid-run (Notion discovers
pages that the search index omitted). Clients must tolerate `total` increasing; never assume it is
final until `Process.Done`.

**Cancellation** is `ProcessCancel(processId)` — the same call as before. v2 joins the cancel into
the run context, so in-flight fetches and uploads stop promptly, and the run finishes with
`IMPORT_IS_CANCELED`. In `ALL_OR_NOTHING` mode, cancelling triggers the same compensation
(created objects are deleted) as any other fatal outcome.

**`noProgress = true`** suppresses the process registration **and the notification**. You still get
`Event.Import.Finish`. Use it only for background/automated imports (migrations, gallery installs).

**Root collection & widget.** On success, `rootCollectionID` is the collection holding everything the
run imported, and the middleware creates a widget pointing at it automatically. Markdown imports with
`createDirectoryPages` instead return the root *directory page* id with a Tree widget layout. Notion
returns a collection ("Notion Import") with a CompactList widget. Clients should navigate the user to
`rootCollectionID` when it is non-empty.

---

## 5. Completion, errors and the notification

`NotificationImport`:

| Field | Meaning |
|---|---|
| `processId` | correlates with the `Process.*` events |
| `errorCode` | `model.Import.ErrorCode`; `NULL` = success |
| `importType` | echo of the request type |
| `spaceId` | target space |
| `reportObjectId` | **new** — id of the import report page; empty when the run was clean |
| `issuesCount` | **new** — number of warning-or-worse issues |

Error codes v2 can produce (full mapping in [ImportErrorCodes.md](ImportErrorCodes.md)):

| Code | When |
|---|---|
| `NULL` | finished; per-object warnings may still exist (check `issuesCount`) |
| `IMPORT_IS_CANCELED` | user cancelled |
| `NOTION_NO_OBJECTS_IN_INTEGRATION` | the Notion token sees no pages/databases |
| `FILE_IMPORT_NO_OBJECTS_IN_ZIP_ARCHIVE` / `..._IN_DIRECTORY` | nothing importable found |
| `NOTION_RATE_LIMIT_EXCEEDED`, `NOTION_SERVER_IS_UNAVAILABLE` | Notion API, after bounded retries |
| `INSUFFICIENT_PERMISSIONS` | Notion token unauthorized/forbidden |
| `FILE_LOAD_ERROR` | file content could not be fetched or uploaded |
| `INTERNAL_ERROR` | anything unclassified |

**A successful import can still have issues.** This is the key behavioral change: v1 reported one
error code and nothing else, so partial data loss was invisible. v2 finishes with `NULL` and
`issuesCount > 0` when, say, 3 of 400 pages failed or 12 links could not be resolved.

Recommended UI states:

| Condition | Suggested UI |
|---|---|
| `errorCode = NULL`, `issuesCount = 0` | "Import complete" |
| `errorCode = NULL`, `issuesCount > 0` | "Imported with N issues" + link to `reportObjectId` |
| `errorCode ≠ NULL`, `reportObjectId ≠ ""` | error message + link to the report |
| `errorCode ≠ NULL`, `reportObjectId = ""` | error message only (fatal before anything ran) |

---

## 6. The import report page

When a run records any issue, the engine creates a normal page object in the target space titled
`Import report — <source name>` (📋 icon) and returns its id. It is a real object: openable,
linkable, deletable by the user. It is **not** added to the space's root collection listing of
imported objects — but it *is* reachable from the import's root collection.

Contents: a summary table (severity × code × count) followed by one collapsible group per issue kind,
each listing the affected objects as clickable mentions.

Issue codes a user may see (stable strings, safe to localize by mapping):

| Severity | Code | Meaning |
|---|---|---|
| info | `flavourDetected` | which markdown dialect was detected |
| info | `typeSuggested` | a container's pages were typed (e.g. "database *Tasks* imported as task") |
| info | `propertyMapped` | a property was normalized onto another relation (LLM plan only) |
| warning | `dataLoss` | a deliberate, documented conversion loss |
| warning | `unsupportedBlock` | a source block has no Anytype counterpart |
| warning | `missingTarget` | a link/mention target could not be resolved |
| warning | `llmPlanFailed` | LLM analysis was requested but unavailable; built-in rules used |
| warning | `llmPlanEntryDropped` | one LLM suggestion failed validation and was ignored |
| objectError | `objectFailed`, `fileFetchFailed`, `objectTooLarge` | a single object did not import |
| fatal | `sourceInvalid`, `noObjects`, `authFailed`, `rateLimited`, `cancelled`, `storeError` | run-level failure |

The list is capped (1000 issues); overflow is stated on the page and counted in `issuesCount`.

**Client work**: link to it from the notification, and ideally offer a "discard report" affordance
(it is an ordinary object — deleting it is `ObjectSetIsArchived` / delete like any page).

---

## 7. LLM structure enrichment (BYOK) — `aiParams`

### 7.1 What it does

Optional. When configured, the engine sends the **schema** of the import — database/folder names,
property names, formats, select option names — to an OpenAI-compatible endpoint of the user's
choosing, and applies the returned plan:

- **types**: a Notion database called "Client Projects" becomes Project objects; an Obsidian folder
  of meeting notes becomes a purpose-built type. Without it, only literal keyword matches are typed
  ("Tasks", "People", …) and most containers import as plain Pages.
- **property normalization**: `Deadline` → the bundled *Due date* relation, `Done?` → *Done*,
  duplicate near-identical properties across databases merged into one relation, formats corrected.
- **new types**: for containers no bundled type fits, with sensible recommended properties.

Without `aiParams` the import behaves exactly as it does today (built-in keyword/shape rules).

### 7.2 Wire format

```proto
Rpc.Object.Import.Request.AIParams {
  Rpc.AI.ProviderConfig config = 1;
  bool includeContentSamples   = 2;
}

Rpc.AI.ProviderConfig {
  Provider provider = 1;   // OLLAMA=0 | OPENAI=1 | LMSTUDIO=2 | LLAMACPP=3
  string   endpoint = 2;   // base URL incl. /v1; empty = provider default
  string   model    = 3;   // required — the feature is OFF when empty
  string   token    = 4;   // bearer token; required for OPENAI
  float    temperature = 5; // IGNORED by import (forced to ~0 for determinism)
}
```

Rules the client must know:

- **`model` empty (or `aiParams` absent) = feature off.** This is the "off" signal — do not send a
  half-filled config expecting it to be ignored gracefully; a present-but-broken config surfaces a
  visible `llmPlanFailed` warning by design (the user asked for it, so the failure is not silent).
- **`endpoint` defaults per provider** when empty: ollama `http://localhost:11434/v1`,
  LM Studio `http://localhost:1234/v1`, llama.cpp `http://localhost:8080/v1`,
  OpenAI `https://api.openai.com/v1`.
- **`token`**: required for `OPENAI` (rejected up front if missing). Local servers usually ignore it,
  but ollama still wants the header — send any non-empty string if the user's setup requires it.
- An **OpenAI key over plain `http://` to a non-local host is refused** (cleartext leak). Use
  `https://` for remote endpoints.
- **`temperature` is ignored**: the import forces ~0 because the plan must be reproducible.

### 7.3 `includeContentSamples` — the privacy switch

| Value | What leaves the device |
|---|---|
| `false` (default) | Metadata only: container names, property names, formats, select option names. |
| `true` | The above **plus** a few page titles per container. |

Page bodies are **never** sent, in either mode. Present this as an explicit, off-by-default checkbox
("Send sample page titles for better accuracy") next to the endpoint settings, and make clear that
any use of this feature sends data to the endpoint the user configured.

### 7.4 Failure behavior — it can never break an import

Every LLM failure path (no endpoint, auth error, rate limit, invalid response, 90s budget exceeded)
degrades to the built-in rules and emits **one** `llmPlanFailed` warning on the report page. The
import itself proceeds and succeeds. Clients do not need error handling specific to the AI step — it
shows up as a warning like any other.

Individual suggestions that fail validation (a hallucinated database id, an illegal type, a property
mapping that would corrupt values) are dropped one by one as `llmPlanEntryDropped` warnings while the
rest of the plan applies.

### 7.5 What the user sees

Adopted decisions appear on the report page as info issues: `typeSuggested`
("database *Sprint work* pages imported as task (LLM plan)") and `propertyMapped`
("folder *Work* property *Deadline* imported as *Due date*"). That page is the audit trail for
everything the model changed — worth mentioning in the AI settings copy.

### 7.6 Suggested settings UI

```
[ ] Improve structure with AI (optional)
    Provider  [ Ollama ▾ ]        (Ollama / LM Studio / llama.cpp / OpenAI)
    Endpoint  [ http://localhost:11434/v1 ]   (prefilled per provider)
    Model     [ qwen3:8b            ]   ← required
    API key   [ ••••••••            ]   (required for OpenAI)
    [ ] Also send sample page titles for better accuracy
    ⓘ Your schema is sent to this endpoint. Page contents are never sent.
```

Persist these per user (not per import) and reuse for both Notion and Obsidian flows. There is no
"test connection" RPC today — validation happens on the first import, visible as a report warning.

### 7.7 Cost & latency expectations

One completion per import (plus at most one corrective retry), a few thousand tokens for a typical
workspace. Budgeted at 90 seconds; on a local model this is the slowest single step of a small
import, so keep the progress UI responsive (see §11 about the missing phase label).

---

## 8. Notion token validation

`ObjectImportNotionValidateToken(token)` is unchanged and is served by v2 when the Notion flag is on.
Codes: `NULL`, `UNAUTHORIZED`, `FORBIDDEN`, `SERVICE_UNAVAILABLE`, `INTERNAL_ERROR`,
`ACCOUNT_IS_NOT_RUNNING`. Same UX as today: validate before enabling the "Import" button.

Notion specifics worth surfacing in the UI: the integration must be granted access to the pages the
user wants (Notion shares nothing by default), and v2 paces itself to Notion's ~3 req/s allowance, so
large workspaces take minutes — the progress bar is the honest indicator.

---

## 9. Behavior changes users may notice (v1 → v2)

| Area | v1 | v2 |
|---|---|---|
| Notion `select` column | became a multi-value Tag relation | becomes a single-select **Status** relation (pick-one preserved) |
| Failed pages | silently missing | a placeholder object + an `objectFailed` issue on the report |
| Typing | everything a Page | keyword/shape typing for containers; LLM typing when configured |
| Errors | one code, no detail | per-object issue ledger on a real page |
| Re-import | duplicated objects in some paths | object ids are derived deterministically, so a repeated import of the same source no longer duplicates; with `updateExistingObjects` the run also matches existing objects by source path and updates them in place |
| Collection membership on re-import | union with previous | **replace** with the current import's list |

None of these require client changes; they are worth knowing for support and release notes.

---

## 10. Integration checklist

1. Nothing to do to keep working — v2 is API-compatible. ✔
2. Read `reportObjectId` / `issuesCount` from `NotificationImport` (and/or `EventImportFinish`);
   show "Imported with N issues" and link to the report page.
3. Tolerate `progress.total` increasing mid-run; don't cache it as final.
4. Ignore the deprecated `Rpc.Object.Import.Response` fields.
5. If shipping AI enrichment: add the settings UI (§7.6), send `aiParams` on Notion/Obsidian/Markdown
   imports, default `includeContentSamples = false`, and require a non-empty `model`.
6. Keep sending `type = Obsidian` for explicit Obsidian imports (it selects the dialect profile).

---

## 11. Known gaps / not yet implemented

Be aware of these when planning UI:

- **No "test connection" RPC** for the AI endpoint; the first import is the test.
- **No chunking for very large workspaces**: the whole schema goes in one prompt. An oversized
  workspace fails closed (built-in rules + `llmPlanFailed` warning), it does not error the import.
- **Multi-path markdown imports** run as independent sequential imports with one root collection each
  (v1 merged them). Prefer a single path per request in the UI.
- **The AI RPC family** (`Rpc.AI.*` writing tools) remains stubbed and unrelated to this feature;
  `Rpc.AI.ProviderConfig` is reused here purely as the config message.
