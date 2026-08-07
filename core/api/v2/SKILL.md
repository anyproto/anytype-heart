---
name: anytype-api
description: Call Anytype's local HTTP API v2 directly — search, read, create and edit objects, sets, collections and chats over REST. Use when writing scripts, SDK code or curl against the API. For interactive note/task work prefer the `anytype` CLI and its skill (cmd/anytype/SKILL.md); this guide is the raw-HTTP layer beneath it.
---

# Anytype API v2 — HTTP guide for agents

Local REST API at `http://127.0.0.1:31009` (the Anytype app must be
running). Every call sends `Authorization: Bearer <key>`; keys are created
in the app (Settings → API keys) — **the API mints none**. Bodies are
compact camelCase JSON. Every list takes `?offset=&limit=` (default 25,
max 1000) and returns `{data, total, offset, limit, has_more}`.

**First call: `GET /v2/auth/whoami`.** A key may be scoped to particular
spaces and to read-only. `grant.scoped: false` means the whole account;
otherwise `grant.spaces` lists what you may touch and `grant.permission`
whether you may write. Ask this instead of discovering limits through 403s
(`space_not_granted` / `write_not_granted` — the message names the grant).

## The data model in six ideas

- **Spaces** contain everything; nearly every route is
  `/v2/spaces/{spaceId}/…`. `GET /v2/spaces` lists them.
- An **object** = `properties` (typed key-values) + `blocks` (document
  content). `type` is always a type **key** (`page`, `task`) — never an id.
- **Properties** are addressed by camelCase key. Select/multiSelect values
  are option **names** (`"In progress"`, case-sensitive) — never option
  ids. Writes create unknown option names (a PATCH caps this at 64);
  unknown property keys are rejected with a did-you-mean.
- **Blocks** are a FLAT array in pre-order with an integer `indent`
  (absent = 0) — no `children` key. Inline formatting is markdown inside
  `text`. Block ids are stable; any block reference accepts the full id or
  a unique suffix.
- Title and description are **not blocks** — they live in `properties`
  (`name`, `description`). A fresh object has zero blocks.
- A **set** is a live query over a type; a **collection** is a hand-curated
  list (edited via `addItems`/`removeItems`). **Chats** store messages
  outside blocks, paged by order-id cursors.

## Which operation

| Intent | Call |
|---|---|
| find objects | `POST …/{spaceId}/search` (or `POST /v2/search` across spaces — rows then carry `spaceId`). Search with filters; don't enumerate `GET …/objects` |
| read one object | `GET …/objects/{id}` — start with `?outline=true` |
| change property values | PATCH op `setProperties` — `add`/`remove` for list values, `set` for scalars |
| complete a task object | `setProperties` (`"set":{"done":true}` or the status option) — a property, not a block edit |
| change a word/phrase | op `replaceText` `{id, find, replace}` — never retype the block |
| toggle a checkbox block | op `updateBlock` `{"id":…,"set":{"checked":true}}` — merge; text untouched |
| add content | op `insertBlocks` with a `markdown` payload — write markdown, the server parses it |
| restructure | ops `moveBlock` / `replaceSubtree` / `deleteBlock` |
| one table cell | op `setCell` — never rewrite the table |
| create an object | `POST …/objects` — shortcut `{type, name, properties, markdown}` covers most cases |
| curate a collection | PATCH ops `addItems` / `removeItems` on the collection object |
| read a set / collection | `GET …/sets/{id}/objects` · `…/collections/{id}/objects` (`?view=`, `?fields=`) |
| new type / property | `POST …/types` · `POST …/properties`; select options ride the property or create-missing |
| upload a file | `POST …/files` (multipart or `{"url":…}`) → the id file blocks and chat attachments need |
| chat | `GET/POST …/chats/{id}/messages`, `POST …/read` — see Chats |
| replace a whole document | `PUT …/objects/{id}` — last resort; check `diffStats` for an accidental full rewrite |

## Read cheaply

- `GET …/objects/{id}?outline=true` → every block's `{indent, id, type}`
  (text on headings only) — structure + addressable ids at a fraction of
  the tokens. Follow up with `?block={id}` for one subtree, or PATCH
  directly: **editing needs no prior full read once you know the ids**,
  and ids survive across reads.
- `?include=properties` or `?include=blocks` reads half the object.
  `?format=md` is a read-only markdown rendering.
- Object ids in read bodies are compacted via a `refs` legend by default
  (`?ids=full` opts out); block ids are always full on default reads.
- List/search rows are minimal `{id, name, type}`; add columns with
  `fields=` (property keys) instead of GETting each object.
- Every object read returns an `etag` (envelope + `ETag` header).

## Edit: PATCH ops

`PATCH …/objects/{id}` body `{"ops":[…]}` — one atomic batch (≤512 ops,
≤256 blocks per op): any invalid op rejects the whole PATCH with
`ops[i]`-addressed issues. Ten ops:

```json
{ "ops": [
  { "op": "setProperties", "set": {"status": ["Done"]}, "unset": ["oldKey"],
    "add": {"tags": ["urgent"]}, "remove": {"assignee": ["bafy…"]} },
  { "op": "updateBlock",  "id": "b5", "set": {"checked": true} },
  { "op": "replaceText",  "id": "b2", "find": "Q3", "replace": "Q4" },
  { "op": "insertBlocks", "after": "b3", "markdown": "## Notes\n- first\n- second" },
  { "op": "moveBlock",    "id": "b9", "inside": "b2", "position": "last" },
  { "op": "deleteBlock",  "id": "b4", "recursive": true },
  { "op": "setCell",      "tableId": "t1", "row": "r2", "col": "c1", "value": "done" }
] }
```

- **`setProperties`**: a key appears in at most one of
  `set`/`unset`/`add`/`remove`. `add`/`remove` are per-entry list edits
  (select/multiSelect/objects/files) — appending one tag never rewrites
  the array. `remove` never creates the option it names. `set: {"k": []}`
  = present-but-empty; `unset` removes presence.
- **`updateBlock`** is THE block-field op (merge; explicit `null` clears a
  field) — checkbox, color, language, retype, or full text rewrite.
- **`replaceText`**: `find` must match exactly once ("found 2 matches —
  provide more context"); `replace_all: true` is the escape. Preferred over
  `updateBlock` for word-level edits. `replaceSubtree {id, blocks}` swaps a
  block plus descendants.
- **`insertBlocks`**: `blocks` (flat array) or `markdown` — mutually
  exclusive, same targeting. Target with one of `after`/`before`/`inside`
  (+`position: first|last` with `inside`); omit all three to append at the
  document end — the way into an empty object. Payload `indent: 0` = the
  anchor's level (`after`/`before`) or the container's child level
  (`inside`). `moveBlock` targets the same way.
- Response: new `etag`, `createdBlocks` (payload position → real id),
  `created` (options minted by create-missing),
  `diffStats {blocksAdded, blocksRemoved, blocksChanged, blocksMoved,
  propertiesChanged}`.
- `PUT` replaces the whole document (body = a full AnyBlock doc; keep the
  block ids from your GET so the server can diff-apply). Prefer PATCH.

## Query

`POST …/search` body: `{query?, type?, filter?|filters?, sorts?, fields?}`.
Pagination is the query params — a body `limit` is rejected. Search is a
read: no `Idempotency-Key`, `dry_run` ignored.

```json
{ "query": "report", "type": "task",
  "filter": "done = false AND (dueDate < currentWeek() OR dueDate IS EMPTY)",
  "sorts": [ { "property": "dueDate", "direction": "asc" } ],
  "fields": ["name", "dueDate", "status"] }
```

- **Prefer the compact `filter` string** (≤4096 chars):
  `status IN ("In progress", "Blocked")` · `name CONTAINS "report"` ·
  `lastModifiedDate > daysAgo(7)` · `tags HAS ALL ("urgent", "q3") AND
  assignee IS NOT EMPTY`. Dates are RFC 3339 or preset functions
  (`today()`, `currentWeek()`, `daysAgo(n)`). Parse errors are
  offset-addressed with did-you-mean.
- The structured `filters` array: leaf =
  `{"property","condition","value"}`, group =
  `{"operator":"and|or","filters":[…]}` (non-empty). Date values there are
  **unix seconds**, not RFC 3339 (the string form converts for you).
  `filter` and `filters` together → 400 `ambiguous_input`.
- `type` is also a filter pseudo-key for multi-type: `type IN ("task",
  "bug")`. **File rows appear only when a file type is named** in the type
  channel (`type = "image"`, `type IN (… "file")`) — `size > 5` alone
  matches nothing; compose `type = "image" AND size > 5`. `mimeType` and
  `size` work in fields/filters/sorts.
- An unguarded `dueDate < …` also matches objects with **no** date — the
  response warns; add `AND dueDate IS NOT EMPTY` unless intended.
- Full-text `total` is a lower bound while `has_more` is true — walk
  pages, don't plan on the number.
- Sorts: any property key, `{"property", "direction": "asc|desc"}`;
  default is `lastModifiedDate desc`.

## Chats

- `GET …/chats/{id}/messages` returns `{messages, state, messageCount,
  has_more, nextBefore?, nextAfter?}`. `state` carries `unreadMessages`,
  `unreadMentions`, `lastStateId` — so "anything new?" is a `?limit=1`
  read. Cursors only (`?after=` walks forward; otherwise newest-first via
  `nextBefore`); `?offset=` is rejected.
- Message `text` is inline markup both ways (mentions as
  `<mention objectId="…">`); ≤8000 chars; `attachments` = up to 32 object
  ids from `POST …/files`. `?reactions=full` adds who reacted.
- Mark read: `POST …/chats/{id}/read` with `{"upTo": <order>,
  "lastStateId": <id>}` — **both** from the same GET, else nothing marks.
- `PATCH …/messages/{id}` `{"text"}` edits text only (attachments kept);
  editing/deleting another member's message → 403. DELETE permanently
  removes orphaned attachments — the response warns with their ids.
- No etag/If-Match on chats; order ids are the concurrency vocabulary.

## Conventions on every call

- **Errors** are `{status, code, message, issues:[{path, message,
  hint}]}` — built to be repaired in ONE retry: fix the named path per the
  hint, resend once. Never loop blindly; 403s and validation failures do
  not improve with repetition.
- `warnings` on success responses are advisory — no retry needed.
- **`Idempotency-Key`** (all mutations incl. DELETE): mint a fresh random
  key per logical mutation; reuse the SAME key only to retry the identical
  request — a replay answers `Idempotency-Replayed: true`. The same key
  with a different body/path/query → 409 `idempotency_conflict`.
- **`?dry_run=true`** on any mutation: full validation, identical verdicts,
  nothing committed (response echoes `dry_run: true`).
- **`If-Match`** (objects only): send an etag back verbatim when a
  concurrent overwrite would matter; mismatch → 409 `etag_mismatch`
  carrying the current etag. Omit it by default — sync also moves the
  etag, so habitual If-Match 409s on noise.
- `POST /v2/validate` pre-flights an AnyBlock document: 200 with
  `{issues, warnings}` even for an invalid one.

## Look it up at runtime — don't guess

- `GET /v2/schemas` — index. `GET /v2/schemas/{kind}` — strict JSON Schema
  + worked example per request kind (`object`, `shortcut`, `type`,
  `template`, `property`, `set`, `collection`, `file`, `search`, `space`,
  `filters`, `chat`, `chatMessage`, `chatMessageEdit`, `chatReaction`,
  `chatRead`). The `filters` kind also serves the filter-string grammar
  (EBNF + examples). `GET /v2/schemas/ops/{op}` — per-op schema + example.
- Live vocabulary: `GET …/types` and `GET …/types/{key}` (the type
  document, incl. its property keys); `GET …/properties`;
  `GET …/properties/{key}/options?prefix=` (check before writing select
  values); `GET …/members` and `…/members/me` (participant ids for
  `assignee`/`creator`; `/me` is your own).
- Full reference: `GET /v2/docs/openapi.json` (or `.yaml`).

## Mistakes that actually happen

- RFC 3339 dates in the **structured** `filters` array (unix seconds
  there — or use the filter string, which converts).
- Rewriting a whole multiSelect array to add one entry — use
  `setProperties.add`.
- Option **ids**, or wrong-case option names, as values — names are the
  identity; check `…/options` first.
- Reusing one `Idempotency-Key` across different requests → 409. Keys are
  per logical mutation, not per session.
- `replaceText`/`edit` text is markup SOURCE: `*`, `[`, `~~` in a
  replacement become real formatting — escape with `\`.
- Deleting a parent block without `recursive: true`, or moving a block
  into its own subtree — rejected with the reason; read the error.
- Filtering on file metadata without naming a file type — zero rows.

## Not in v2 (yet)

- **No object archive/delete** — `DELETE …/objects/{id}` does not exist;
  only types and properties have DELETE. Archive via the app or v1.
- **No file byte download** under /v2 (Phase 8; bytes live on v1's
  `GET /v1/spaces/{id}/files/{fileId}` — unreachable for space-scoped
  keys, which /v1 refuses). No file content extraction ("read this PDF")
  anywhere in the API.
- **No chat SSE stream** under /v2 and no per-chat message full-text
  search (both v1 for now); poll `GET messages?limit=1` instead.
- `GET …/types/{key}/schema` is a 501 stub — compose from
  `GET …/types/{key}` + `…/properties/{key}/options`.
- No option rename/recolor/delete under /v2 (v1 tags admin, Phase 8).
