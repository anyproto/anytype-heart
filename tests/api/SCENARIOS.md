# JSON API E2E Test Scenarios

End-to-end scenarios for the local JSON API (`core/api`). Each scenario lists
preconditions, the request, the expected response, and any post-conditions to
verify. Scenarios are framework-agnostic — they can be turned into Go HTTP
tests, a Bruno/Postman collection, or `hurl` files.

## Conventions

- **Base URL:** `http://127.0.0.1:<JsonApiListenAddr>` (default `31009`).
- **Auth header:** `Authorization: Bearer <api_key_token>` unless otherwise noted.
- **Version header:** `Anytype-Version: 2025-05-20`.
- **Bootstrap:** every suite assumes one healthy account with at least one
  regular space (`SPACE_ID`) and a tech space (`TECH_SPACE_ID`), plus a valid
  API key issued via the auth flow (Section 1).
- **Cleanup:** each scenario must leave the workspace in the same logical state
  it found it in (delete objects/files it created, unrevoke keys, etc).
- **IDs:** `*_ID` placeholders are resolved at runtime.

## 0. Cross-cutting concerns

### 0.1 Authentication
| # | Scenario | Expected |
|---|---|---|
| 0.1.1 | No `Authorization` header on `/v1/*` | 401, JSON error body |
| 0.1.2 | `Authorization: Bearer ` (empty token) | 401 |
| 0.1.3 | `Authorization: Token foo` (wrong scheme) | 401 |
| 0.1.4 | Bearer token never issued | 401 |
| 0.1.5 | Bearer token that was revoked | 401 |
| 0.1.6 | Bearer token from a different account/process | 401 |
| 0.1.7 | Bearer token used after account closed | 401 |
| 0.1.8 | Auth endpoints (`/v1/auth/*`) reachable without bearer | 200/4xx by case |
| 0.1.9 | Two concurrent requests with the same token | both 200 |

### 0.2 Versioning
| # | Scenario | Expected |
|---|---|---|
| 0.2.1 | `Anytype-Version` omitted | server default applies (200) |
| 0.2.2 | Unknown `Anytype-Version` value | 400 or default — document chosen behavior |
| 0.2.3 | Malformed value (e.g. `not-a-date`) | 400 |
| 0.2.4 | Future date | 400 or accepted with warning |

### 0.3 Pagination
| # | Scenario | Expected |
|---|---|---|
| 0.3.1 | Default `offset`/`limit` (omitted) | `offset=0`, `limit=100` |
| 0.3.2 | `limit=0` | 400 |
| 0.3.3 | `limit=1` | one record, `has_more=true` if more exist |
| 0.3.4 | `limit=1000` (max) | accepted |
| 0.3.5 | `limit=1001` | 400 (clamped) |
| 0.3.6 | `limit=-1` | 400 |
| 0.3.7 | `limit=abc` | 400 |
| 0.3.8 | `offset` beyond total | empty array, `has_more=false` |
| 0.3.9 | `offset=-5` | 400 |
| 0.3.10 | `offset` and `limit` stitched: walk full collection in pages | union equals single-page query |

### 0.4 Rate limiting (write endpoints)
| # | Scenario | Expected |
|---|---|---|
| 0.4.1 | 1 req/sec sustained for 60 s on `POST /v1/spaces` | all 200 |
| 0.4.2 | Burst of 60 requests in <1 s | 60 of them 200, rest 429 |
| 0.4.3 | After 429, wait 2 s, retry | 200 |
| 0.4.4 | Rate limit applied per-token (two tokens, each at limit) | each gets its own bucket — or document shared if shared |
| 0.4.5 | `ANYTYPE_API_DISABLE_RATE_LIMIT=1` | no 429 even under burst |

### 0.5 Routing & method
| # | Scenario | Expected |
|---|---|---|
| 0.5.1 | `GET /v1/unknown` | 404 |
| 0.5.2 | `POST /v1/spaces/:id` (only PATCH/GET supported) | 405 |
| 0.5.3 | Trailing slash variant (`/v1/spaces/`) | consistent (200 or 308) |
| 0.5.4 | Path traversal attempt (`/v1/spaces/../objects`) | 404 |
| 0.5.5 | Extremely long path segment (>2048 chars) | 414 or 400 |
| 0.5.6 | URL-encoded special chars in path params | server decodes once, no double-decoding |

### 0.6 CORS
| # | Scenario | Expected |
|---|---|---|
| 0.6.1 | OPTIONS preflight with `Origin: http://localhost:5173` | 200/204 with `Access-Control-*` headers |
| 0.6.2 | Disallowed origin (if allowlist active) | preflight rejected |

### 0.7 Request body limits
| # | Scenario | Expected |
|---|---|---|
| 0.7.1 | JSON body 1 MB | 200 |
| 0.7.2 | JSON body 100 MB | 413 or 400 |
| 0.7.3 | Malformed JSON (`{"a":}`) | 400 with parse error |
| 0.7.4 | Empty body on endpoint expecting JSON | 400 |
| 0.7.5 | Content-Type omitted on POST with JSON body | accepted or 415 (document) |
| 0.7.6 | Slow client trickle body | server times out at `readTimeout` (5 s) |

### 0.8 Error envelope shape
| # | Scenario | Expected |
|---|---|---|
| 0.8.1 | Any 4xx | JSON `{ "error": { "code": ..., "message": "..." } }` (or whatever the agreed shape is — verify it stays stable across endpoints) |
| 0.8.2 | 5xx never leaks internal stack traces |

---

## 1. Auth (`/v1/auth/*`)

| # | Scenario | Expected |
|---|---|---|
| 1.1.1 | Create challenge — happy | 200, returns `challenge_id` |
| 1.1.2 | Create challenge with empty `app_name` | 400 |
| 1.1.3 | Create challenge with 1 KB `app_name` | 200 or 400 (boundary) |
| 1.1.4 | Create challenge with non-UTF8 `app_name` | 400 |
| 1.2.1 | Solve challenge — happy | 200, returns `api_key` + `app_key` |
| 1.2.2 | Reuse same `challenge_id` twice | first 200, second 4xx |
| 1.2.3 | Wrong solution code | 4xx |
| 1.2.4 | Solve after challenge expired | 4xx |
| 1.2.5 | Solve unknown challenge_id | 4xx |
| 1.3.1 | Two API keys for the same app | both valid, distinct tokens |
| 1.3.2 | Use one token after revoking the *other* | unaffected |

---

## 2. Spaces (`/v1/spaces`)

| # | Scenario | Expected |
|---|---|---|
| 2.1.1 | `GET /spaces` lists at least one space | 200, `data[]` contains the bootstrap space |
| 2.1.2 | Result contains `tech-space-id` filtered out (if filter is in place) | verify |
| 2.2.1 | `GET /spaces/{SPACE_ID}` — happy | 200 |
| 2.2.2 | Unknown space_id | 404 |
| 2.2.3 | space_id is an object_id (wrong type) | 404 |
| 2.2.4 | Empty `space_id` (e.g. `/spaces//`) | 400/404 |
| 2.3.1 | `POST /spaces` — minimal valid body | 200 with new id |
| 2.3.2 | Empty `name` | 400 |
| 2.3.3 | Name with 10 000 chars | 400 |
| 2.3.4 | Invalid `space_ux_type` enum | 400 |
| 2.3.5 | Create 50 spaces serially | all succeed |
| 2.4.1 | `PATCH /spaces/{id}` partial update of name | 200, other fields untouched |
| 2.4.2 | Set `icon` to a non-existent image_id | 200, icon URL composed (broken link is client-visible only) |
| 2.4.3 | Set description to empty string | 200, persisted as empty |
| 2.4.4 | Concurrent PATCH with same key | last-writer-wins; both 200 |

---

## 3. Members (`/v1/spaces/{space_id}/members`)

| # | Scenario | Expected |
|---|---|---|
| 3.1.1 | List members — happy | 200, includes self |
| 3.1.2 | Pagination across many members | union matches single-page |
| 3.2.1 | `GET .../members/{participant_id}` | 200 |
| 3.2.2 | `GET .../members/{identity}` (not participant id) | 200 (falls back) |
| 3.2.3 | Unknown member id | 404 |
| 3.2.4 | Member in different space | 404 |
| 3.3.1 | Member icon URL points to API base (not gateway) | starts with `http://127.0.0.1:31009/v1/spaces/{SPACE_ID}/images/` |
| 3.3.2 | Member icon URL is fetchable with the same bearer token | 200 |

---

## 4. Objects (`/v1/spaces/{space_id}/objects`)

### 4.1 Create
| # | Scenario | Expected |
|---|---|---|
| 4.1.1 | Create with `type_key=page`, minimal body | 200, returns object |
| 4.1.2 | Create with no body | 400 |
| 4.1.3 | Missing `type_key` | 400 |
| 4.1.4 | Unknown `type_key` | 400 |
| 4.1.5 | Name with 10 000 chars | 400 or truncated (document) |
| 4.1.6 | Name with emoji + RTL + zero-width chars | 200, preserved verbatim |
| 4.1.7 | Body with valid markdown (headings, lists, links) | 200, markdown round-trips |
| 4.1.8 | Body with 1 MB markdown | 200 or 413 |
| 4.1.9 | Body with `<script>` HTML | sanitized in markdown round-trip |
| 4.1.10 | Properties with valid values | 200 |
| 4.1.11 | Property of wrong format (string into number) | 400 |
| 4.1.12 | Unknown property key | 400 |
| 4.1.13 | Self-referencing `objects` property (object references itself) | 200 |
| 4.1.14 | Create + Update in burst (< rate limit window) | all 200 |
| 4.1.15 | Create in space the caller has read-only access to | 403 |

### 4.2 Read
| # | Scenario | Expected |
|---|---|---|
| 4.2.1 | Get existing object | 200, includes type, properties, blocks |
| 4.2.2 | Get with markdown content | `markdown` field populated |
| 4.2.3 | Get unknown object_id | 404 |
| 4.2.4 | Get archived object | 200 with `archived=true` (or 404 — document) |
| 4.2.5 | Get object from different space | 404 |
| 4.2.6 | List objects empty space | 200, `data: []`, `has_more=false` |
| 4.2.7 | List objects with filters (when supported via query) | matches filter |

### 4.3 Update
| # | Scenario | Expected |
|---|---|---|
| 4.3.1 | Patch name | 200 |
| 4.3.2 | Patch property values | 200, only those keys change |
| 4.3.3 | Patch with invalid property format | 400 |
| 4.3.4 | Patch deleted object | 404 |
| 4.3.5 | Concurrent patches to different keys | both 200, both applied |
| 4.3.6 | Concurrent patches to same key | last-writer-wins, both return 200 |

### 4.4 Delete
| # | Scenario | Expected |
|---|---|---|
| 4.4.1 | Delete existing object | 200 with archived object body |
| 4.4.2 | Delete already-archived object | idempotent (200) or 404 — document |
| 4.4.3 | Delete system object (e.g. profile, space-view) | 403 or 400 |
| 4.4.4 | Delete from other space | 404 |

---

## 5. Types (`/v1/spaces/{space_id}/types`)

| # | Scenario | Expected |
|---|---|---|
| 5.1.1 | List bundled types | 200, contains `page`, `task`, etc. |
| 5.1.2 | Create custom type | 200, gets `type_id` |
| 5.1.3 | Create type with name colliding with bundled | 400 or 409 |
| 5.1.4 | Create type with invalid `layout` enum | 400 |
| 5.1.5 | Create type with `recommended_properties` referencing unknown props | 400 |
| 5.1.6 | Get unknown type_id | 404 |
| 5.1.7 | Patch bundled type | 403 (read-only) |
| 5.1.8 | Patch custom type | 200 |
| 5.1.9 | Delete bundled type | 403 |
| 5.1.10 | Delete custom type that's in use | 409 or 400 |
| 5.1.11 | Type icon: emoji → `EmojiIcon`; name → `NamedIcon`; never `FileIcon` | verify shape |

---

## 6. Properties (`/v1/spaces/{space_id}/properties`)

| # | Scenario | Expected |
|---|---|---|
| 6.1.1 | List properties | 200, contains system + custom |
| 6.1.2 | Create text property | 200 |
| 6.1.3 | Create each property format (number, select, multi_select, date, files, checkbox, url, email, phone, objects) | 200 each |
| 6.1.4 | Create property with reserved key (`name`, `id`) | 400 |
| 6.1.5 | Create property with duplicate api key | 409 |
| 6.1.6 | Get unknown property | 404 |
| 6.1.7 | Patch system property | 403 |
| 6.1.8 | Patch custom property name | 200 |
| 6.1.9 | Change property `format` after creation | 400 (immutable) |
| 6.1.10 | Delete property in use | 409 |

---

## 7. Tags (`/v1/spaces/{space_id}/properties/{property_id}/tags`)

| # | Scenario | Expected |
|---|---|---|
| 7.1.1 | List tags for select property | 200 |
| 7.1.2 | List tags for non-select property (e.g. text) | 400 or empty |
| 7.1.3 | Create tag with color | 200 |
| 7.1.4 | Create tag with unknown color | 400 |
| 7.1.5 | Create duplicate-name tag | 409 |
| 7.1.6 | Delete tag in use | object loses that tag, 200 |
| 7.1.7 | Patch tag name/color | 200 |

---

## 8. Templates (`/v1/spaces/{space_id}/types/{type_id}/templates`)

| # | Scenario | Expected |
|---|---|---|
| 8.1.1 | List templates for a type | 200 |
| 8.1.2 | List templates for a type with none | 200, empty |
| 8.1.3 | Get unknown template | 404 |
| 8.1.4 | Get template with mismatched `type_id` in path | 404 |

---

## 9. Lists / Collections (`/v1/spaces/{space_id}/lists/{list_id}`)

| # | Scenario | Expected |
|---|---|---|
| 9.1.1 | Get list views | 200 |
| 9.1.2 | Get objects in view | 200, paginated |
| 9.1.3 | Add object to collection | 200 |
| 9.1.4 | Add unknown object | 400/404 |
| 9.1.5 | Add object already in collection | idempotent |
| 9.1.6 | Remove object | 200 |
| 9.1.7 | Remove object not in collection | 404 or 200 (document) |
| 9.1.8 | Add to query-style list (not a collection) | 400 |
| 9.1.9 | View filters: dataview filter passes through correctly | row count matches |

---

## 10. Search (`/v1/search`, `/v1/spaces/{space_id}/search`)

| # | Scenario | Expected |
|---|---|---|
| 10.1.1 | Global search empty query → recent objects | 200 |
| 10.1.2 | Search with text query | hits include match |
| 10.1.3 | Search with filters: by type, by date range | applied correctly |
| 10.1.4 | Search with bogus filter key | 400 |
| 10.1.5 | Search with `sort` on unknown field | 400 |
| 10.1.6 | Space-scoped search excludes other spaces | verify |
| 10.1.7 | Search across spaces returns `space_id` per record | verify |
| 10.1.8 | Pagination cursor over 10 000 results | consistent ordering |
| 10.1.9 | Query with very long string (10 KB) | 400 or accepted |
| 10.1.10 | Query with regex metachars (`.*+?^$()[]`) | treated literally, no regex injection |

---

## 11. Files

### 11.1 Upload — `POST /v1/spaces/{space_id}/files`
| # | Scenario | Expected |
|---|---|---|
| 11.1.1 | Plain text 12 B — happy | 200, returns `object_id`, `file_id`, `details` |
| 11.1.2 | PNG 200×200 | 200, `details.media` = `image/png`, width/height populated |
| 11.1.3 | JPEG with EXIF | 200, EXIF stripped or preserved per policy |
| 11.1.4 | GIF (animated) | 200, mime `image/gif` |
| 11.1.5 | SVG with `<script>` | 200; on serve via 11.3.5 the script is stripped |
| 11.1.6 | 0-byte file | 200 or 400 — document |
| 11.1.7 | 1 GB file | 200 if under server limit; 413 otherwise |
| 11.1.8 | File name with spaces, dashes | preserved |
| 11.1.9 | File name with Unicode (`файл.txt`, `日本.png`) | preserved |
| 11.1.10 | File name with path separators (`a/b.txt`, `..\c.txt`) | basename only is used; no traversal |
| 11.1.11 | File name with NUL bytes | 400 |
| 11.1.12 | Missing `file` form field | 400, `missing file in request` |
| 11.1.13 | `file` field present but empty filename | 200 or 400 (document) |
| 11.1.14 | Multiple `file` fields | first wins or 400 (document) |
| 11.1.15 | Wrong `Content-Type` (not `multipart/form-data`) | 400 |
| 11.1.16 | `multipart` but no boundary | 400 |
| 11.1.17 | Two sequential uploads of identical bytes into the **same** space | **same `object_id`** returned both times — per-space CID-level object dedup; `details` identical |
| 11.1.17b | Same bytes uploaded into two **different** spaces | distinct `object_id`s, same underlying `file_id` (CID) |
| 11.1.18 | Same content reuploaded after delete | document: archived object is still found by CID lookup, so the archived `object_id` is returned — confirm whether the API should un-archive, return error, or expose a "force-new" flag |
| 11.1.19 | Upload to unknown `space_id` | 400 or 404 |
| 11.1.20 | Upload to read-only space | 403 |
| 11.1.21 | Upload while at disk-quota limit | 5xx or 413 with clear message |
| 11.1.22 | Trickle upload over 6 s (longer than `readTimeout`) | 408/500 — server doesn't hang |
| 11.1.23 | Connection closed mid-upload | server cleans up tmp file (verify no leak in `os.TempDir()`) |
| 11.1.24 | Content-Type sniffing: `.exe` renamed to `.png`, magic bytes still PE | `details.media` reflects sniffed type, not extension |
| 11.1.25 | Upload returns `details` map with expected keys (`name`, `sizeInBytes`, `media`, etc.) | shape stable |
| 11.1.26 | Upload triggers analytics event `UploadFile` | verify event broadcast (if observable) |

### 11.2 Download — `GET /v1/spaces/{space_id}/files/{file_id}`
| # | Scenario | Expected |
|---|---|---|
| 11.2.1 | Download right after upload — bytes match exactly | 200, body == uploaded bytes |
| 11.2.2 | `Content-Type` matches `details.media` | verify |
| 11.2.3 | `Cache-Control: max-age=31536000, private` | header present |
| 11.2.4 | `Range: bytes=0-99` on a 1 KB file | 206 with first 100 bytes |
| 11.2.5 | `Range: bytes=100-` (open-ended) | 206 with bytes from 100 to EOF |
| 11.2.6 | `Range: bytes=999999-` on small file | 416 |
| 11.2.7 | `If-Modified-Since` future date | 304 |
| 11.2.8 | `If-None-Match` matching ETag (if emitted) | 304 |
| 11.2.9 | `HEAD` request | 200 with headers, empty body |
| 11.2.10 | Unknown `file_id` | 404 |
| 11.2.11 | `file_id` belongs to a different space (`SPACE_ID` mismatch in path) | document — current impl ignores path `space_id`; either enforce or note |
| 11.2.12 | Bearer missing | 401 |
| 11.2.13 | Bearer revoked between upload and download | 401 |
| 11.2.14 | 16 concurrent downloads of same file | all 200, no corruption |
| 11.2.15 | Download soft-deleted file (post 11.4.1) | 200 (file still resolvable in archive) or 404 — document |
| 11.2.16 | Download hard-deleted file (post 11.4.2) | 404 |
| 11.2.17 | Download large file (1 GB), client streams without buffering | server doesn't hold full body in RAM |
| 11.2.18 | Cancel mid-stream (client closes connection) | server logs once, no goroutine leak |
| 11.2.19 | Response body decompresses correctly if `Accept-Encoding: gzip` is offered (or compression isn't applied — document) | verify |

### 11.3 Image-specific behavior — same endpoint, `?width=` query
The download endpoint above also serves images. When the underlying file is an image, the optional `width` query selects a pre-rendered variant; SVGs are sanitized inline. `width` is ignored on non-image files.

| # | Scenario | Expected |
|---|---|---|
| 11.3.1 | PNG file_id → returns PNG bytes (no width) | 200, original |
| 11.3.2 | `?width=200` on a PNG file_id | 200, variant width ≤ 200 |
| 11.3.3 | `?width=0` | treated as no-width (original) |
| 11.3.4 | `?width=-5` | 400 |
| 11.3.5 | `?width=abc` | 400 (covered in unit) |
| 11.3.6 | `?width=99999999` | clamped to original or 400 |
| 11.3.7 | SVG file with `<script>alert(1)</script>` | 200, body has script removed |
| 11.3.8 | SVG with external xlink reference | external blocked |
| 11.3.9 | Raw CID as `file_id` for an image | 200 (gateway parity preserved) |
| 11.3.10 | `?width=200` on a non-image file (e.g. text/plain) | 200, width silently ignored, original bytes returned |
| 11.3.11 | Animated GIF with `?width=N` | first frame returned (document if so) |
| 11.3.12 | Verify Content-Disposition header (or absence) | stable across non-image / image / SVG paths |
| 11.3.13 | Image larger than any pre-rendered variant + no width | returns original |
| 11.3.14 | Image deleted while request in flight | request that already started succeeds; new request 404 |

### 11.4 Delete — `DELETE /v1/spaces/{space_id}/files/{file_id}`
| # | Scenario | Expected |
|---|---|---|
| 11.4.1 | Default (no query) → archive | 204; object now has `archived=true` |
| 11.4.2 | `?skip_bin=true` → permanent | 204; subsequent GET object → 404 |
| 11.4.3 | `?skip_bin=false` | same as 11.4.1 |
| 11.4.4 | `?skip_bin=1` | 204 (Go `strconv.ParseBool` accepts `1`) |
| 11.4.5 | `?skip_bin=TRUE` | 204 |
| 11.4.6 | `?skip_bin=yes` | 400 (not a bool) |
| 11.4.7 | `?skip_bin=true&skip_bin=false` (repeated query) | gin uses first; document |
| 11.4.8 | Delete unknown file_id with `skip_bin=false` | 4xx — depends on `ObjectSetIsArchived` behavior for missing id |
| 11.4.9 | Delete unknown file_id with `skip_bin=true` | 4xx |
| 11.4.10 | Delete already-archived file with `skip_bin=true` | 204 (purge skips archive check) |
| 11.4.11 | Delete then immediately download (hard) | 404 |
| 11.4.12 | Delete then re-upload same bytes | new object created; references to old object are still archived |
| 11.4.13 | Delete file used as an object's `iconImage` | object still resolvable, icon URL now 404 |
| 11.4.14 | Two parallel hard-deletes of the same file | one 204, other 204 or 4xx — must not panic |
| 11.4.15 | Delete from read-only space | 403 |
| 11.4.16 | `DELETE` on file currently being downloaded | download finishes; subsequent GETs 404 |

---

## 12. Icon proxy invariants

| # | Scenario | Expected |
|---|---|---|
| 12.1 | Object with `iconEmoji` set | icon = `{format: emoji, emoji: "..."}` |
| 12.2 | Object with `iconName` set | icon = `{format: icon, name: "...", color: "..."}` |
| 12.3 | Object with `iconImage` set | icon = `{format: file, file: "http://127.0.0.1:31009/v1/spaces/{SPACE_ID}/files/{image_id}"}` |
| 12.4 | Object with no icon fields | `icon: null` |
| 12.5 | Icon URL fetched with same bearer | 200 |
| 12.6 | Icon URL fetched without bearer | 401 |
| 12.7 | Icon URL fetched with `?width=64` | 200 with resized variant |
| 12.8 | Member icon, space icon, type icon: all use the same URL pattern | verify across endpoints |
| 12.9 | Tech-space objects' icons use `TECH_SPACE_ID` in URL | verify |
| 12.10 | URL contains no query-string token (`?token=...`) | absent (proxy approach uses Authorization header, not gateway tokens) |

---

## 13. Concurrency & consistency

| # | Scenario | Expected |
|---|---|---|
| 13.1 | Create 100 objects in parallel (10 workers) | all 200, all returned ids are distinct |
| 13.2 | Update + read interleaved | reads see writes in order (no torn reads) |
| 13.3 | Subscribe to types via cross-space sub, then create new type | cache reflects new type within reasonable bound |
| 13.4 | Delete object while another request reads it | reader sees consistent state (full body or 404) |
| 13.5 | Server restart between upload and download | downloaded bytes match (file persisted) |
| 13.6 | `ReassignAddress` while requests in flight | old connections close gracefully; new bind succeeds |

---

## 14. Performance smoke

| # | Scenario | Expected |
|---|---|---|
| 14.1 | 1000 sequential `GET /v1/spaces` | p95 < 50 ms |
| 14.2 | 10 parallel uploads of 1 MB each | all complete within 5 s |
| 14.3 | List 10 000 objects via pagination | total wall time < 10 s |
| 14.4 | Single-process memory does not grow unboundedly across the suite | RSS stable within ±20% |

---

## 15. Security-flavored edge cases

| # | Scenario | Expected |
|---|---|---|
| 15.1 | SQL/NoSQL injection in search query | treated literally |
| 15.2 | Header injection via name fields (`\r\nSet-Cookie: ...`) | sanitized in response |
| 15.3 | Stored XSS attempt in object name | name returned verbatim in JSON; client renders safely |
| 15.4 | Bearer token logged in server stdout/file | should not be — verify in test |
| 15.5 | Error responses never include raw stack traces | verify |
| 15.6 | Path traversal in upload filename (`../../etc/passwd`) | basename used, no escape |
| 15.7 | XXE in any XML-parsing surface (none expected) | n/a |
| 15.8 | Open redirect via `/swagger` (currently 301 to docs site) | target is fixed, not user-controlled |
| 15.9 | TLS — API binds to 127.0.0.1 only by default; binding to 0.0.0.0 requires explicit env | verify default |

---

## 16. Observability hooks

| # | Scenario | Expected |
|---|---|---|
| 16.1 | Each request emits an analytics event with correct `code` (e.g. `UploadFile`, `DeleteFile`) | verify via mock or event bus |
| 16.2 | 4xx responses still emit events (with non-200 status) | verify |
| 16.3 | Slow handler (>1 s) produces a log line | verify |

---

## Test data inventory

The suite needs:
- A 12 B `hello.txt` text file
- A 5 KB `tiny.png`
- A 200 KB `medium.jpg` with EXIF
- A 5 MB `large.pdf`
- A 50 MB `huge.bin` (skip in CI by default)
- A 1 KB `evil.svg` containing `<script>alert(1)</script>` and an `<image xlink:href="http://example.com/x">`
- A `.exe` file renamed to `.png`
- A file named `файл (1) — Copy.txt`

## Suggested execution shape

Recommended order of work to make scenarios runnable:
1. Stand up a Go test harness in `tests/api/` that boots the API server against a temp account and yields `(baseURL, bearer)` to each test.
2. Implement Section 0 (cross-cutting) first — these protect every later scenario.
3. Section 1 (auth) — gates everything else.
4. Sections 11 + 12 (files/icons) — biggest recent surface area, biggest risk.
5. Sections 2–10 in any order.
6. Sections 13–16 as a separate "soak" suite.
