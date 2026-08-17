---
name: anytype-api
description: Call Anytype's local HTTP API v2 directly — search, read, create and edit objects, sets, collections and chats over REST. Use when writing scripts, SDK code or curl against the API. For interactive note/task work prefer the `anytype` CLI and its skill (cmd/anytype/SKILL.md); this guide is the raw-HTTP layer beneath it.
---

# Anytype API v2 — HTTP guide for agents

Local REST API at `http://127.0.0.1:31009` (the Anytype app must be
running). Every call sends `Authorization: Bearer <key>`; keys are created
in the app (Settings → API keys) — **the API mints none**. Bodies are
compact JSON, and every name this API owns is `snake_case` — params,
fields and op names alike. Every list takes `?offset=&limit=` (default 25,
max 1000) and returns `{data, total, offset, limit, has_more}`.

**First call: `GET /v2/auth/whoami`.** A key may be scoped to particular
spaces and to read-only. `grant.scoped: false` means the whole account;
otherwise `grant.spaces` lists what you may touch and `grant.permission`
whether you may write. Ask this instead of discovering limits through 403s
(`space_not_granted` / `write_not_granted` — the message names the grant).

## The data model in six ideas

- **Spaces** contain everything; nearly every route is
  `/v2/spaces/{space_id}/…`. `GET /v2/spaces` lists them.
- An **object** = `properties` (typed key-values) + `blocks` (document
  content). `type` is always a type **key** (`page`, `task`) — never an id.
- **Properties** are addressed by key, and every key is snake_case —
  bundled, API-created and UI-created alike (`due_date`, `icon_emoji`,
  `manual_property`). `GET …/properties` spells them the same way; so do
  documents. Keys are forgiving on input — `dueDate` or `DueDate` resolve
  to `due_date` too; an input matching two properties is a 400 listing
  both. (One exception: the compact `filter` STRING validates before
  folding — use a listed spelling there.)
  Select/multiSelect values are option **names** (`"In progress"`,
  case-sensitive) — never option ids. Writes create unknown option names
  (a PATCH caps this at 64); unknown property keys are rejected with a
  did-you-mean.
- **Blocks** are a FLAT array in pre-order with an integer `indent`
  (absent = 0) — no `children` key. Inline formatting is markdown inside
  `text`. Use block ids exactly as a read served them.
- Title and description are **not blocks** — they live in `properties`
  (`name`, `description`). A fresh object has zero blocks.
- A **set** is a live query over a type; a **collection** is a hand-curated
  list (edited via `add_items`/`remove_items`). **Chats** store messages
  outside blocks, paged by order-id cursors.

## Which operation

| Intent | Call |
|---|---|
| find objects | `POST …/{space_id}/search` (or `POST /v2/search` across spaces — rows then carry `space_id`). Search with filters; don't enumerate `GET …/objects` |
| read one object | `GET …/objects/{id}` — start with `?outline=true` |
| change property values | PATCH op `set_properties` — `add`/`remove` for list values, `set` for scalars |
| complete a task object | `set_properties` (`"set":{"done":true}` or the status option) — a property, not a block edit |
| change a word/phrase | op `replace_text` `{find, replace}` — `id` optional; never retype the block |
| toggle a checkbox block | op `update_block` `{"match":"Draft timeline","set":{"checked":true}}` — merge; text untouched. `match` or `id`, never both |
| add content | op `insert_blocks` with a `markdown` payload — write markdown, the server parses it |
| restructure | ops `move_block` / `replace_subtree` / `delete_block` (`delete_block` takes `match` too) |
| one table cell | op `set_cell` — never rewrite the table |
| show/hide a view column, edit a view | op `update_view` — works on sets, collections and a type's default view (PATCH the type OBJECT id from `GET …/types/{key}`) |
| add / reorder / remove a view | ops `insert_view` (`copy_from` duplicates one) · `move_view` (`position:"first"` = default tab) · `delete_view` |
| create an object | `POST …/objects` — shortcut `{type, name, properties, markdown}` covers most cases |
| delete an object you created | `DELETE …/objects/{id}` — archives (Bin, reversible in the app). Only works on objects THIS key created after provenance shipped; anything else → 403 `not_created_by_this_key`, permanently — don't retry, archive in the app instead. Ownership is matched on the app name EXACTLY (byte-for-byte — re-pair under the identical name to keep delete rights). User content only: system objects 403. Probe first with `?dry_run=true` |
| curate a collection | PATCH ops `add_items` / `remove_items` on the collection object |
| read a set / collection | `GET …/sets/{id}/objects` · `…/collections/{id}/objects` (`?view=`, `?fields=`) |
| new type / property | `POST …/types` · `POST …/properties`; select options ride the property or create-missing |
| upload a file | `POST …/files` (multipart or `{"url":…}`) → the id file blocks and chat attachments need |
| chat | `GET/POST …/chats/{id}/messages`, `POST …/read` — see Chats |

## Read cheaply

- `GET …/objects/{id}?outline=true` → every block's `{indent, id, type}`
  (text on headings only) — structure + addressable ids at a fraction of
  the tokens. Follow up with `?block={id}` for one subtree, or PATCH
  directly: **editing needs no prior full read once you know the ids**.
- When the request already quotes the text to change, skip the read
  entirely: `replace_text {find, replace}` locates the block itself, and
  `update_block`/`delete_block` take `match` for the same job (one match, or
  a refusal listing the candidates).
- `?include=properties` or `?include=blocks` reads half the object.
  `?format=md` is a read-only markdown rendering.
- Echo block ids back exactly as a read served them; if one is rejected as
  unknown, re-read and use the fresh ids. `?ids=full` is the backup/export
  shape — the read to archive or clone from, not needed for editing.
- List/search rows are minimal `{id, name, type}`; add columns with
  `fields=` (property keys) instead of GETting each object.
- Every object read returns an `etag` (envelope + `ETag` header).

## Edit: PATCH ops

`PATCH …/objects/{id}` body `{"ops":[…]}` — one atomic batch (≤512 ops,
≤256 blocks per op): any invalid op rejects the whole PATCH with
`ops[i]`-addressed issues. Fourteen ops:

```json
{ "ops": [
  { "op": "set_properties", "set": {"status": ["Done"]}, "unset": ["oldKey"],
    "add": {"tags": ["urgent"]}, "remove": {"assignee": ["bafy…"]} },
  { "op": "update_block",  "match": "Draft timeline", "set": {"checked": true} },
  { "op": "replace_text",  "find": "Q3 report", "replace": "Q4 report" },
  { "op": "insert_blocks", "after": "b3", "markdown": "## Notes\n- first\n- second" },
  { "op": "move_block",    "id": "b9", "inside": "b2", "position": "last" },
  { "op": "delete_block",  "id": "b4", "recursive": true },
  { "op": "set_cell",      "table_id": "t1", "row": "r2", "col": "c1", "value": "done" },
  { "op": "update_view",   "columns": {"status": {"hidden": false}} },
  { "op": "insert_view",   "name": "Board", "copy_from": "viewAll1",
    "set": {"type": "kanban", "groupBy": "status"} }
] }
```

- **`set_properties`**: a key appears in at most one of
  `set`/`unset`/`add`/`remove`. `add`/`remove` are per-entry list edits
  (select/multiSelect/objects/files) — appending one tag never rewrites
  the array. `remove` never creates the option it names. `set: {"k": []}`
  = present-but-empty; `unset` removes presence.
- **`update_block`** is THE block-field op (merge; explicit `null` clears a
  field) — checkbox, color, language, retype, or full text rewrite.
- **`match` addresses the block by its TEXT** on `update_block` and
  `delete_block` — the `id` alternative: give one or the other, **never
  both** (and never neither). The text must appear in exactly ONE block or
  the op refuses: zero → read the outline, several → the error lists
  candidate ids to retry with. Repeats inside the one matched block are
  fine — `match` names a block, not an occurrence. It reads the document as
  the ops before it in the batch left it.
- **`replace_text`**: `id` is optional — omitted, `find` locates the block
  and must appear in exactly ONE block (zero or several matching blocks
  refuse; the ambiguity error lists candidate ids to retry with). Within
  the matched block `find` must match exactly once ("found 2 matches —
  provide more context"); `replace_all: true` is the escape, within that
  one block only. Preferred over `update_block` for word-level edits.
  `replace_subtree {id, blocks}` swaps a block plus descendants.
- **`insert_blocks`**: `blocks` (flat array) or `markdown` — mutually
  exclusive, same targeting. Target with one of `after`/`before`/`inside`
  (+`position: first|last` inside that container); omit all three and
  `position` picks an end of the DOCUMENT — `last` (or absent) appends,
  `first` inserts at the start, both on an empty object too. Payload
  `indent: 0` = the anchor's level (`after`/`before`) or the container's
  child level (`inside`). `move_block` targets the same way, so
  `{"op":"move_block","id":"b9","position":"first"}` moves a block to the top
  of the document.
- **Author new content without ids** — an `id` names an EXISTING block, so
  `insert_blocks` takes none anywhere in its payload (rows and columns
  included); the server mints them and returns them in `created_blocks`,
  keyed by the payload path that produced each — `ops[0].blocks[0]`,
  `ops[0].blocks[0].rows[1]`, `ops[0].blocks[0].columns[0]`. The same holds
  wherever you leave an id out of an existing-content payload (a new row in
  `update_block set.rows`, a block inside a `set_cell` array), so you never
  have to re-read to learn an id you just created.
- **`update_view`** edits ONE dataview view — never resend the views array.
  `block`/`view` are optional when the object has one dataview and it one
  view (types, sets, collections usually do). `set` merges view fields
  (`name`, `type`, `groupBy`, `sorts`, `filters` — arrays replace whole;
  `filter` takes the compact string; null clears a field); `columns` merges
  per property key: `{"hidden": false}` shows a column, `null` removes it,
  a new key appends one. Works on Blocks-restricted objects — view config
  is not a block edit.
- **`insert_view`/`move_view`/`delete_view`** complete the family (same
  addressing, same channels; insert_view's name is its own required field —
  not in `set`). insert_view needs only `name` — bare default: every listed
  property visible, newest first; `copy_from` duplicates a view (then
  `set`/`columns` override); the minted id returns in `created_views`,
  keyed `ops[i]`. move_view REQUIRES one of `after`/`before`/`position`
  (`"first"` = default tab). delete_view refuses the last view — insert the
  replacement first (one atomic batch swaps a bad default view).
- Response: new `etag`, `created_blocks` (payload position → real id;
  nested row/column/cell slots included), `created_views` (same, for minted
  view ids), `created` (options minted by create-missing),
  `diff_stats {blocks_added, blocks_removed, blocks_changed, blocks_moved,
  properties_changed}`, `warnings` (advisory, e.g. an unguarded date filter).
- **There is no whole-document replace** — never read a document,
  regenerate it and write it back. Replace a section with
  `replace_subtree`; start over by batching `delete_block`s with the new
  `insert_blocks`.

## Query

`POST …/search` body: `{query?, type?, filter?|filters?, sorts?, fields?}`.
Pagination is the query params — a body `limit` is rejected. Search is a
read: no `Idempotency-Key`, `dry_run` ignored.

```json
{ "query": "report", "type": "task",
  "filter": "done = false AND (due_date < currentWeek() OR due_date IS EMPTY)",
  "sorts": [ { "property": "due_date", "direction": "asc" } ],
  "fields": ["name", "due_date", "status"] }
```

- **Prefer the compact `filter` string** (≤4096 chars):
  `status IN ("In progress", "Blocked")` · `name CONTAINS "report"` ·
  `last_modified_date > daysAgo(7)` · `tags HAS ALL ("urgent", "q3") AND
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
- An unguarded `due_date < …` also matches objects with **no** date — the
  response warns; add `AND due_date IS NOT EMPTY` unless intended.
- Full-text `total` is a lower bound while `has_more` is true — walk
  pages, don't plan on the number.
- Sorts: any property key, `{"property", "direction": "asc|desc"}`;
  default is `last_modified_date desc`.

## Chats

- `GET …/chats/{id}/messages` returns `{messages, state, message_count,
  has_more, next_before?, next_after?}`. `state` carries `unread_messages`,
  `unread_mentions`, `last_state_id` — so "anything new?" is a `?limit=1`
  read. Cursors only (`?after=` walks forward; otherwise newest-first via
  `next_before`); `?offset=` is rejected.
- Message `text` is inline markup both ways (mentions as
  `<mention objectId="…">`); ≤8000 chars; `attachments` = up to 32 object
  ids from `POST …/files`. `?reactions=full` adds who reacted.
- Mark read: `POST …/chats/{id}/read` with `{"up_to": <order>,
  "last_state_id": <id>}` — **both** from the same GET, else nothing marks.
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
  `set_properties.add`.
- Option **ids**, or wrong-case option names, as values — names are the
  identity; check `…/options` first.
- Reusing one `Idempotency-Key` across different requests → 409. Keys are
  per logical mutation, not per session.
- `replace_text`/`edit` text is markup SOURCE: `*`, `[`, `~~` in a
  replacement become real formatting — escape with `\`.
- Deleting a parent block without `recursive: true`, or moving a block
  into its own subtree — rejected with the reason; read the error.
- Filtering on file metadata without naming a file type — zero rows.

## Not in v2 (yet)

- **Object DELETE is own-output-only** — `DELETE …/objects/{id}` archives
  only objects THIS key created (recorded at creation, immutably). Objects
  created before that shipped, in the app, by import or by other members
  are permanently 403 for every key — there is no "delete arbitrary
  objects" capability. `?dry_run=true` is the cheap deletability probe;
  types/properties still use their own DELETE routes.
- **No file byte download** under /v2 (Phase 8; bytes live on v1's
  `GET /v1/spaces/{id}/files/{fileId}` — unreachable for space-scoped
  keys, which /v1 refuses). No file content extraction ("read this PDF")
  anywhere in the API.
- **No chat SSE stream** under /v2 and no per-chat message full-text
  search (both v1 for now); poll `GET messages?limit=1` instead.
- `GET …/types/{key}/schema` is a 501 stub — compose from
  `GET …/types/{key}` + `…/properties/{key}/options`.
- No option rename/recolor/delete under /v2 (v1 tags admin, Phase 8).
