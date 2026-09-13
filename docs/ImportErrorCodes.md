# Import error codes

The wire enum is `model.Import.ErrorCode`, delivered in `NotificationImport.ErrorCode` (the only error
signal for async imports; the RPC reply is deprecated and empty). The v2 engine maps its internal typed
issues (`importv2.IssueCode`) onto the same values in
`core/block/importv2/adapter/adapter.go` (`errorCode`), so the frontend contract is unchanged.

### Standard

| Code | Meaning | v2 source |
|---|---|---|
| `NULL` | finished without a fatal error | run succeeded (per-object warnings may still exist) |
| `INTERNAL_ERROR` | any unclassified failure | fallback for unmapped fatal issues |
| `IMPORT_IS_CANCELED` | user cancelled | `IssueCancelled` (process cancel joined into the run context) |

### Files (Markdown/Obsidian and other file-based formats)

| Code | Meaning | v2 source |
|---|---|---|
| `FILE_IMPORT_NO_OBJECTS_IN_ZIP_ARCHIVE` | archive contains nothing importable for the chosen type | `IssueNoObjects` when any request path is a `.zip` |
| `FILE_IMPORT_NO_OBJECTS_IN_DIRECTORY` | directory contains nothing importable | `IssueNoObjects` otherwise |
| `FILE_LOAD_ERROR` | file content could not be fetched/uploaded | `IssueFileFetchFailed` |

### Notion

| Code | Meaning | v2 source |
|---|---|---|
| `NOTION_NO_OBJECTS_IN_INTEGRATION` | the integration token sees no pages/databases | `IssueNoObjects` on a Notion request |
| `NOTION_SERVER_IS_UNAVAILABLE` | 5xx from Notion after bounded retries | `client.ErrUnavailable` in the fatal chain |
| `NOTION_RATE_LIMIT_EXCEEDED` | 429/529 persisted past the retry budget | `client.ErrRateLimited` / `IssueRateLimited` |
| `INSUFFICIENT_PERMISSIONS` | token unauthorized/forbidden (also mapped by v1 from list errors) | `client.ErrUnauthorized` / `ErrForbidden` / `IssueAuthFailed` |

### Legacy (v1-only formats, unchanged)

`HTML_WRONG_HTML_STRUCTURE`, `PB_NOT_ANYBLOCK_FORMAT`, `CSV_LIMIT_OF_ROWS_OR_RELATIONS_EXCEEDED`,
`BAD_INPUT`, `UNKNOWN_ERROR` — emitted only by v1 paths until those formats move to v2.

Non-fatal conditions (missing link targets, unsupported blocks, deliberate data-loss decisions) never
set an error code: they are structured warnings in the v2 run result, visible in logs.
