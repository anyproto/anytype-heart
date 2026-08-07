# Anytype Local API v2 — specification and phased plan

Status: **draft v0.4** · 2026-07-31 · GO-7383 follow-on
Depends on: AnyBlock JSON v1 flat (`pkg/lib/anyblockjson/SPEC.md` v0.7).
Evidence base: `docs/AgentApiV2Research.md` (+ Addendum A) and
`pkg/lib/anyblockjson/FLAT.md` — decisions cite sections there instead of
re-arguing. v1 (`/v1`) stays untouched for the deprecation window.

Changes from v0.3.5 (this refresh): three read-only reviews checked the
Phase-4/5 plan and the cross-cutting text against the shipped Phase-0–3
surface; this version applies their findings. Phases 0–3 now read as fact
(resolver wiring, the referential validation layer and the `Options`
id-compaction split moved from build items to built; §8.2's
post-op-validation paragraph corrected — the whole-document net is ON by
default and rejecting, per review B′3; the idempotency hash and etag
comparison passages corrected to the shipped formulas). **Phase 4 was
replanned against the current primitives**: collections gained a read/query
path, the search body follows C2/C10 (`sorts`, query-param pagination), the
primary example is single-form, the former one-line "design deltas" are now
explicit rules (key scope without a type, a system-key allowlist, read-only
option resolution, per-space global semantics, the empty-date warning,
`type` as a filter pseudo-key), stored-view execution substitutes the SPEC
§6.2 dynamic placeholders, search is declared a read (exempt from C8/C9),
and the internal build-vs-reuse inventory is named. **Phase 5's §7 was
aligned with the shipped op set**: create-missing option names per R9,
`add`/`remove` on `set_properties`, a `check_item` tool over `updateBlock`,
markdown decided as an `insertBlocks` payload alternative, the reference
and editing channels qualified (full-read relabeling; D′1 markup caveat),
and the hard dependency order stated. §3/§4 refreshed so every gate is
still decidable (B1/B2 reworded — `replaceText`/`setCell` and both filter
forms ship regardless). The SPEC §6.2.1 contradiction is **resolved**: the
filter grammar + parser ship now as a library
(`pkg/lib/anyblockjson/filterstring`) consumed by the API; only the
*document* view field `filter` stays reserved post-v1 (SPEC v0.7). One
Phase-1 route — `DELETE /objects` (archive) — was found never registered
and is re-marked [build], due before Phase 5.

Changes from v0.2: applied the small-model (3–4B) review
(`core/api/APIV2_REVIEW_SMALLMODEL.md`). The small-model contract is now a
first-class **task-tool wrapper** (§7) — a curated ~9-tool layer over the
full REST API, identical to the Phase-5 CLI verb-set, exposed as CLI verbs
and an on-device function-calling/MCP manifest. It picks agent-friendly
*channels* the raw REST body can't (markdown-in for authoring, anchor-in for
editing, enumerated handles for reference), which closes the review's three
structural findings. Consequently `replaceText` and `setCell` become
**launch primitives** (the wrapper depends on them), the filter-string parser
becomes a launch dependency, and the primary worked examples switch to
single-op / single-filter forms. See §7 and the reweighted §2/§3/§4.

Changes from v0.1 (carried): the 3-lens review (`core/api/APIV2_REVIEW.md`,
R1–R15) — block ids full on edit reads (R1); `revision` → advisory `etag`
(R2); `inside` indent + `updateBlock` (R3/R4); post-op validity normative
(R5); build-items marked (R6); `type` everywhere (R7); C13 strict schemas
(R8); `/validate` split (R9); sets path (R10); files/spaces/members/archive
(R11); read matrix (R12); benchmarks (R13); op gaps (R14); errors (R15).

Design premise: agents — including small models — are the primary consumers.
Small models drive the **task-tool wrapper (§7)**, not the raw REST surface;
large models and programmatic clients use the full API. Every choice
optimizes for one-shot success, token economy, and a generate → validate →
repair loop with path-addressed errors.

---

## 1. Conventions (apply to every endpoint)

| # | Convention | Rationale (research ref) |
|---|---|---|
| C1 | Base path `/v2`, localhost, bearer auth and `Anytype-Version` date header as in v1. | migration §4.8 |
| C2 | **One vocabulary: the format's.** camelCase, property **keys**, option **names** — everywhere, both directions. The object-type field is **`type`** (a type key) on every surface: envelope, rows, search, shortcuts. No id/key duality, no snake_case. Object ids remain ids. | v1's top agent trap (§2.1) |
| C3 | **Compact JSON always** (no pretty-printing). | free 38–46% (§3.6) |
| C4 | **Object ids compact by default** (refs legend per SPEC §9a — lossless); `?ids=full` opts out. **Block ids are always full on default reads** — block-label compaction (5-char, legend-less, lossy) appears only in explicitly read-only shapes (`outline`, prompt/example exports). Write endpoints MAY additionally resolve block-id references by unique-suffix match against the live object (SPEC §9a wiring allowance). Never require echoing a full CID. *(Built: `Options.CompactObjectRefs` and `Options.CompactBlockLabels` are separate flags; `CompactIds` is the shorthand for both.)* **Outline exception (T7)**: the outline shape compacts block labels but keeps object refs **full** — it drops the refs legend with the properties map, so a legend-compacted ref inside a heading's text would be unresolvable; `?ids=` is ignored there. | ~24×/id, −89% id errors (§3.6); id round-trip contract (R1) |
| C5 | Minimal rows: list/search responses carry `id, name, type` + requested property values. **Never embed type objects.** `fields=` expands. | v1's N× multiplier (§2.1) |
| C6 | Error shape everywhere: `{status, code, message, issues:[{path, message, hint}]}` — path-addressed, naming allowed values. Required codes include: `validation_failed`, `version_unsupported` (surfaces SPEC §10's "produced by a newer version" verbatim, naming both versions), `idempotency_conflict` (same key, different body), `etag_mismatch`, `ambiguous_input` (e.g. both `filter` and `filters` supplied), `forbidden` (403 — an operation the caller's identity may not perform, e.g. editing another member's chat message; added by the Phase-6 review; also the `/v2` key-scope gate's refusal of a non-JsonAPI key, which answers in the shared v1 envelope — §8.9). Error text is API surface; test it. | repair loop (§3.2, §4.6); R15 |
| C7 | Every object read returns **`etag`** (short opaque token, ≤8 chars, derived from tree heads — NOT the object's `revision` property, which stays in `properties`) plus an `ETag` header. Mutations accept **`If-Match` header only** (the AnyBlock body has no envelope slot for it). **Advisory by default**: without `If-Match`, ops apply last-write-wins and `diffStats` reports the outcome; with it, mismatch → 409 `etag_mismatch` carrying the current etag. Note: the etag advances on background sync, not only on agent edits — strict If-Match will 409 on sync noise; block-scoped preconditions (ops apply iff the *addressed* blocks are unchanged) are the deferred v2.x refinement. | R2; optimistic concurrency (§3.2) |
| C8 | `Idempotency-Key` honored on all mutations (POST, PATCH, PUT — v0.3.5; was POST-only); replay with the same key returns the stored result; same key with a different body → 409 `idempotency_conflict`. Response always returns created ids. | agent auto-retry (§3.7); R15 |
| C9 | `?dry_run=true` on every mutation → would-be diff summary + issues, nothing committed. **Recorded C2 carve-out**: the response's `dry_run` echo keeps the query parameter's snake_case spelling (uniform across all v2 mutation DTOs — §8.8). | highest-leverage affordance (§3.7) |
| C10 | Pagination on **every** list surface — objects, search, and the discovery lists (types, properties, **options**): default `limit=25`, `has_more`, truncation messages steer ("312 matches — narrow with filter…"). Options lists take a `prefix=` filter (tag-like properties can hold thousands of options). | Linear/AXI (§3.4, §3.7); R-minor |
| C11 | Reads never fail on unknown *content*; anything a representation cannot express is listed in `warnings` (array of the C6 issue shape, warning-grade). Writes never pass through a lossy representation. | ADF disaster (§3.3) |
| C12 | Every endpoint documents **one worked example + its JSON Schema**, embedded in OpenAPI and fetchable (§5 discovery). | examples 72→90% (§3.4) |
| C13 | **Every schema served by discovery for generation purposes — ops, creates, properties, types, the filter string — is strict-mode-compatible**: non-recursive, `additionalProperties: false`, bounded, within vendor complexity caps. Known exception: the structured `filters` array is recursive (SPEC §12 filterNode) and therefore **not constrained-decodable** — documented as such; small models are steered to the `filter` string. | the format's own premise (Addendum A.1); R8 |

## 2. Phases

Each phase ships independently and is agent-usable on its own. **[B*n*]**
marks a choice gated on a benchmark from §4. **[build]** marks a dependency
that does not exist yet and must be built (not assumed).

### Phase 0 — plumbing + eval harness

- Error contract (C6) incl. the named codes; etag plumbing (C7); idempotency
  store (C8); dry-run scaffolding (C9).
- `POST /v2/validate` — body: AnyBlock document → `{issues, warnings}`.
  **Structural + format-semantic only** (exposes `anyblockjson.Validate`;
  the package cannot see the space). Referential validation — option names,
  a type's actual property keys — is a **space-aware layer built in
  Phase 2**, not a free consequence of this endpoint (R9).
- **Eval harness** (internal, built on `cmd/anyblockroundtrip`'s corpus and
  state-diff machinery): given (task, model, method) → runs an agent loop
  against a scratch space, scores **apply-success, corruption
  (round-trip backtranslation, DELEGATE-52 method), output tokens, turns**.
  Task set: append paragraph · edit one word · toggle a checkbox ·
  restructure a section · fill a table cell (`setCell`) · create task with
  properties · build a set with filter. Model tiers: small (3–8B local), mid (Haiku-class),
  frontier. **The small tier runs under grammar-constrained decoding**
  (XGrammar-class) — without that floor, 3–8B loops produce null data and
  gates are undecidable (R13; §3.5 evidence).

### Phase 1 — read

```
GET /v2/spaces/{spaceId}/objects/{objectId}
    ?include=properties,blocks      # subset; default both
    ?outline=true                   # block skeleton, see below
    ?block={blockId}                # subtree only (contiguous indent-run)
    ?ids=compact|full               # object ids only (C4); default compact
    ?format=anyblock|md             # md read-only, with warnings (C11)
GET /v2/spaces/{spaceId}/objects            # minimal rows (C5)
DELETE /v2/spaces/{spaceId}/objects/{objectId}   # archive (v1 parity); ?permanent=true later  [build — never registered, see below]
GET /v2/spaces                              # spaces list (read)
GET /v2/spaces/{spaceId}/members            # members list (read) — agents need member ids for assignee/creator values
GET /v2/spaces/{spaceId}/types              # keys + names (paginated, C10)
GET /v2/spaces/{spaceId}/types/{type}       # the kind:"objectType" AnyBlock document
GET /v2/spaces/{spaceId}/types/{type}/schema?flavor=json-schema|table|example   [build]
GET /v2/spaces/{spaceId}/properties         # key, name, format (paginated)
GET /v2/spaces/{spaceId}/properties/{key}/options    # option names (+color), paginated + prefix=
```

- **`outline=true` returns the full block skeleton**: every block's
  `{indent, id, type}`, with `text` included **only** on heading blocks —
  so every block id is addressable for a follow-up `?block=` read or a
  PATCH (headings-only would break outline-then-fetch for non-heading
  targets — R12).
- **Param legality** (R12): `outline` and `block` are mutually exclusive
  with each other and with `format=md`; `outline` implies blocks (an
  accompanying `include=properties` adds the properties map; `include`
  without blocks suppresses `blocks` entirely); `ids` affects any shape
  that contains object ids; illegal combinations → 400 `ambiguous_input`
  naming the conflicting params. The outline shape uses compact block
  labels but **full object refs** (read-only shape; the C4 outline
  exception — `ids` is ignored there).
- The object response is the flat AnyBlock document + `etag` (+
  `warnings`).
- `types/{type}/schema` **[build]**: the derived artifact (SPEC §2a
  `GenerateSchema` — *planned there, not implemented*; this endpoint is
  its first consumer). `table` flavor = prompt-ready property table with
  live option names — requires an objectstore join per select property
  (options live on option objects, not in `typeProperties`) (R6). **As
  shipped, the route is a 501 `not_implemented` stub** (no flavor parsing)
  steering to `GET types/{type}`; the `GenerateSchema` artifact +
  store-backed option join stays an open §3 build item, **due before the
  wrapper's `describe` tool (Phase 5)**. It remains the highest-leverage
  accuracy lever (§3.4).
- **`DELETE /objects` (archive) is an open build item**: the route was
  specced with Phase 1 but never registered — v2 DELETE exists only for
  types and properties; object archive is v1-only today. §6's v1-parity
  note depends on it; build it **before Phase 5** so the wrapper/CLI can
  carry an object-archive verb (§2 Phase 5).
- Implementation note: read via the **live smartblock state** →
  snapshot → `anyblockjson.Marshal` (not `ObjectShow`, whose ObjectView is
  the wrong type; not the store snapshot, which lags). Derive `etag` from
  that same state so token and content are consistent (R-note).

### Phase 2 — create (one-shot)

```
POST /v2/spaces/{spaceId}/objects       # body: AnyBlock document (ids optional — id-less input, SPEC §9)
                                        # or shortcut {type, name, properties, markdown}
POST /v2/spaces/{spaceId}/types         # kind:"objectType" doc — typeProperties creates missing properties
POST /v2/spaces/{spaceId}/properties    # {key?, name, format, options?:[{name,color?}]}
POST /v2/spaces/{spaceId}/sets          # {name, type, filters|filter, sorts?, views?}
POST /v2/spaces/{spaceId}/collections   # {name, items?}
POST /v2/spaces/{spaceId}/templates     # AnyBlock doc with templateFor → generic object-create path
POST /v2/spaces/{spaceId}/files         # upload (multipart or URL) → file object id
PATCH/DELETE /v2/spaces/{spaceId}/types/{type}        # update (type doc semantics) / archive
PATCH/DELETE /v2/spaces/{spaceId}/properties/{key}    # update / archive
```

- `POST /objects` body discriminator (R7): presence of `version` or
  `blocks` ⇒ full AnyBlock document; otherwise the shortcut shape.
- **`POST /files` is load-bearing**, not an extra: file/image/video/pdf
  blocks and `iconImage` carry file object ids an agent cannot otherwise
  obtain (R11).
- **Sets** (R10): `ObjectCreateSet` accepts no filters/sorts/views. Build
  the set by constructing its **initial state with a fully-formed dataview
  block** (filters/sorts/views included) via the AnyBlock create path —
  one change set, honestly atomic. Collections are clean (the `items`
  import path exists).
- **Templates**: no create-from-body RPC exists; `POST /templates` targets
  the generic AnyBlock create path (Template kind + `templateFor`), which
  the importer already supports (R-note).
- **Resolver wiring was the substance of this phase** (shipped:
  `core/api/v2/service/resolver.go`, `creatingResolvers`): the
  create-missing property/option bridging from `anyblockjson`'s
  `PropertyResolver`/`OptionResolver` to objectstore +
  `ObjectCreateRelation`/`ObjectCreateRelationOption`. The **referential
  validation layer** (R9) landed here too: a set filter naming a property
  the type lacks errors listing the type's actual keys (policy as built:
  §8.1 "Create-vs-reject policy").
- Every kind: schema + worked example (C12/C13); `Idempotency-Key`;
  `dry_run`.

### Phase 3 — edit

Two modes at launch, one gated addition, additive extensions later.

**Normative rule first (R5):** the post-op document must satisfy the
format's semantic checks (SPEC §12, V1–V5 — monotonicity, leaf containment,
row→column, bounds, id uniqueness). Any violation rejects the **whole
PATCH** with path-addressed errors (`ops[i]` + block path). `moveBlock`
into the moved block's own subtree is a cycle → error. `updateBlock`
changing a parent's type to a leaf type while descendants exist → error
naming the descendant count.

**(a) `PATCH /v2/spaces/{spaceId}/objects/{objectId}` — batched ops (default path)**

```json
{ "ops": [
    { "op": "setProperties", "set": { "status": ["Done"] }, "unset": ["oldKey"] },
    { "op": "updateBlock",   "id": "b5", "set": { "checked": true } },
    { "op": "updateBlock",   "id": "b3", "set": { "text": "new **text**" } },
    { "op": "replaceSubtree","id": "b7", "blocks": [ { "type": "bulletedListItem", "text": "a" },
                                                     { "indent": 1, "type": "paragraph", "text": "b" } ] },
    { "op": "insertBlocks",  "after": "b3", "blocks": [ { "type": "checkbox", "text": "todo" } ] },
    { "op": "moveBlock",     "id": "b9", "inside": "b2", "position": "last" },
    { "op": "deleteBlock",   "id": "b4", "recursive": true }
  ] }
```

- Closed op set, id-addressed, atomic (one `state.Apply` per request), no
  positional/index/offset addressing anywhere (§3.1–3.2).
- **`updateBlock` (R4)**: THE one block-update op (v0.3.5 — `replaceBlock`
  removed). Merge semantics — only the fields in `set` change; everything
  else (including `text`) is untouched; explicit `null` clears a field. The
  op for checkbox toggles, color/align changes, language switches, retypes
  and text rewrites alike. `replaceSubtree` swaps block + descendants for
  the payload run.
- **Relative indent in payloads** (R3): for `after`/`before` and
  `replaceSubtree`, payload `indent: 0` = the anchor's level. For
  `inside`, payload `indent: 0` = **the container's child level**
  (anchor + 1). Worked examples:
  sibling-insert — `{"op":"insertBlocks","after":"b3","blocks":[{"type":"paragraph","text":"same level as b3"}]}`;
  child-insert — `{"op":"insertBlocks","inside":"b3","position":"last","blocks":[{"type":"paragraph","text":"child of b3"},{"indent":1,"type":"paragraph","text":"grandchild"}]}`.
- **`moveBlock`** takes the same targeting as `insertBlocks`:
  `after`/`before`/`inside`+`position: first|last` — so reorder-to-slot,
  indent (`inside` previous sibling), and outdent (`after` the parent) are
  all expressible (R14).
- **Root targeting (v0.3.5)**: omitting all of `after`/`before`/`inside` on
  `insertBlocks` and `moveBlock` appends at the **end of the document root**.
  This is the ops-path into an empty object: SPEC §7 keeps title/description
  out of the document, so a fresh object has zero addressable blocks and an
  anchor-required contract left PUT as the only way to give it content — the
  corruption vector the design steers agents away from. It also backs the §7
  wrapper's `add_blocks(object, after?, markdown)` omitted-`after` case.
  Payload `indent: 0` = the document's top level; `position` still requires
  `inside` (no root-prepend — one shape, fewer fields for a small model).
- **`deleteBlock`**: `recursive` defaults to false; deleting a block that
  has descendants without `recursive:true` → error naming the descendant
  count (R14).
- **`setProperties`** (R14): `set` writes presence — `"k": []` means
  present-but-empty (SPEC §3 presence-is-meaningful); `unset` removes
  presence. Output-only properties (SPEC §4a) are rejected with a
  path-addressed error.
- **`setProperties` `add`/`remove` (v0.3.5)**: per-key list edits for
  list-shaped formats only (select, multiSelect, objects, files — SPEC §3).
  `{"op":"setProperties","add":{"tags":["urgent"]},"remove":{"assignee":["bafy…"]}}`
  — `add` appends entries without duplicating existing ones; `remove`
  deletes matching entries and is a no-op when absent (never creates
  presence, never creates the option it names). Scalar-format keys are
  rejected with a path-addressed error naming the format. A key may appear
  in at most one of `set`/`unset`/`add`/`remove` per op. Rationale:
  appending one tag to a 40-entry multiSelect used to require read →
  whole-array rewrite → write — the corruption pattern in miniature, plus a
  token tax; collections already had `addItems`/`removeItems`.
- Collection ops: `addItems` / `removeItems` (member ids).
- Block-id references accept full ids (canonical) and unique-suffix labels
  (lenient, C4).
- Response: new `etag`, created-block id map **keyed by payload position**
  (`ops[3].blocks[0] → "b1a2…"`; client-supplied ids are echoed as-is)
  (R14), `diffStats`.
- **`diffStats` schema**: `{blocksAdded, blocksRemoved, blocksChanged,
  blocksMoved, propertiesChanged}` (integers).
- Implementation: build child state from live state, apply ops via
  `simple.Block`/`state.State` mutations, one `sb.Apply` — the pattern
  Block* RPC handlers use internally (research §2.3; feasibility-verified).

**(b) `PUT /v2/spaces/{spaceId}/objects/{objectId}` — full-document replace (escape hatch)**

Body = full AnyBlock doc; etag via `If-Match` header only. Server
diff-applies via `Unmarshal → NewDocFromSnapshot → SetParent →
ResetToVersion` — minimal CRDT changes **iff block ids round-trip from the
GET** (which C4's full-block-ids default now guarantees for the natural
loop). Response includes `diffStats`, making an accidental full rewrite
visible (the DELEGATE-52 signature). System kinds excluded per
`canUpdateObject`. Docs steer agents to PATCH.

**(c) `replaceText` — str_replace scoped to one block's `text` (LAUNCH)**

```json
{ "op": "replaceText", "id": "b2", "find": "Q3", "replace": "Q4" }
```

Exact-match within one block, must match exactly once, Anthropic-style
errors ("found 2 matches — provide more context"), `replace_all` escape
hatch. **In the launch op set** (moved from the v0.2 B1 gate): it is the
`edit_text` wrapper tool's backing primitive (§7), and the small-model
review showed deferring it forces the commonest edit (change-one-word)
through whole-block verbatim reproduction — the documented 3B collapse
mode. The server does the replace deterministically; the model supplies
only the short anchor. B1 now only measures whether large models *also*
prefer it over `updateBlock`.

**(d) `setCell` — scoped table-cell write (LAUNCH)**

```json
{ "op": "setCell", "tableId": "t1", "row": "r2", "col": "c1", "value": "done" }
```

`value` is a string (paragraph-cell shorthand), `null` (clear), or a block
object (SPEC §6.1 cell forms). **In the launch op set** (moved from
deferred): it backs the `set_cell` wrapper tool, and a whole-`table`
rewrite for a one-cell change is the same verbatim-collapse trap as (c).
Flat, non-recursive — trivially grammar-constrainable.

**(e) Deferred, additive later** (closed op set, versioned): dataview view
ops, `replaceProperties` full-map swap, cross-object batch, block-scoped
preconditions (C7 note).

### Phase 4 — query

```
POST /v2/spaces/{spaceId}/search        (+ POST /v2/search global)
GET  /v2/spaces/{spaceId}/sets/{setId}/objects?view={viewId}&fields=…
GET  /v2/spaces/{spaceId}/sets/{setId}/views
GET  /v2/spaces/{spaceId}/collections/{collectionId}/objects?fields=…
GET  /v2/spaces/{spaceId}/collections/{collectionId}/views
```

Primary worked example (single-filter form — the small-model form, C12):

```json
{ "query": "report", "type": "task",
  "filter": "done = false AND (dueDate < currentWeek() OR dueDate IS EMPTY)",
  "sorts": [ { "property": "dueDate", "direction": "asc" } ],
  "fields": ["name", "dueDate", "status"] }
```

Secondary example — programmatic composition (the structured array;
mid/frontier and round-trip flows, mirroring §5's secondary multi-op
illustration):

```json
{ "type": "task",
  "filters": [ { "property": "done", "condition": "equal", "value": false } ],
  "sorts": [ { "property": "dueDate", "direction": "asc" } ] }
```

- `filter` (compact string) and `filters` (structured array) are mutually
  exclusive; **both supplied → 400 `ambiguous_input`** ("provide `filter`
  or `filters`, not both") (R15). Both forms land on **one internal tree**
  (the SPEC §6.2 filter node).
- **Request-shape conventions** (fixes v0.3.x drift): the sort field is
  **`sorts`** — the SPEC §6.2 view name and the shipped POST /sets name
  (research §4.5 introduced the singular; one concept two names is exactly
  the duality C2 bans). Pagination is the **C10 query params**
  (`offset`/`limit`, default 25, `has_more`) like every shipped v2 list —
  no body `limit` (body-vs-query duality with no offset story); a body
  `limit` is rejected as an unknown field by the strict request schema.
- **Search is a read** (POST only because the request needs a body): exempt
  from `Idempotency-Key` (responses are never stored or replayed —
  registration is per-route, so the idempotency middleware is simply not
  attached) and from `dry_run` — a supplied `dry_run` is **ignored** (a
  read is its own dry run; erroring would punish a harmless habit).
- **The filter-string parser (built — §8.4)** (R6): `pkg/lib/anyblockjson/
  filterstring` — the SPEC §6.2.1 grammar (scope split there: the
  grammar + parser ship as a library; only the *document* view field stays
  reserved post-v1). Parse → the §6.2 structured tree; offset-addressed
  parse errors naming the offending token and position, with did-you-mean.
  The string uses RFC 3339 dates / preset functions; the structured form
  uses unix numbers — the §6.2.1 mapping applies. The `POST /sets`
  wiring (replacing the §8.1 501), through the same R9 referential layer,
  shipped with it.
- **Validation & resolution rules** (previously a one-line "design deltas"
  note; now decided):
  1. **Key scope.** With a top-level `type`, filter/sort/field keys
     validate against the type's recommended keys + `name` (the shipped R9
     sets rule, `typePropertyKeys`) plus the system allowlist below.
     Without `type`, and on global search, keys validate against the
     **space's property keys** (per space, for global). Unknown keys →
     path-addressed did-you-mean.
  2. **System-key allowlist.** `createdDate`, `lastModifiedDate`,
     `creator`, `lastOpenedDate` — §3-output-only/system keys that appear
     in no type's recommended lists yet back bread-and-butter queries
     (`lastModifiedDate > yesterday()`; v2 ListObjects itself sorts by
     lastModifiedDate). Always part of the query-surface reference set —
     for search AND for set filters/sorts (widening the shipped R9 sets
     rule is part of this phase's wiring).
  3. **Option names resolve READ-ONLY on the query path.** SPEC §3's
     create-missing is write/import behavior; a query must never mint the
     very option it names (the §8.3 `remove` precedent). Unresolved names →
     did-you-mean error, never a silent no-match.
  4. **Global search resolves per space** (v1 GlobalSearch precedent):
     type keys and option names resolve inside each space's loop
     iteration; a name that resolves in only some spaces queries those
     spaces and carries a C6 warning naming the spaces where it did not
     resolve. **Honest totals**: results merge per-space queries by the
     requested sort; `has_more` is true when any space reported more;
     `total` is the sum of per-space store counts — do NOT copy v1's
     `total = len(fetched)` approximation.
  5. **Empty-date hazard surfaces.** SPEC §6.2: an unguarded `less`/
     `lessOrEqual` date comparison matches undated objects (`dueDate <
     currentWeek()` matches objects with no dueDate). Document import
     warns; the search path must too — the same warning text rides the C6
     `warnings` channel on the response.
  6. **`type` is a filterable pseudo-key.** The top-level `type` stays a
     single type key (the small-model form, C2). Multi-type queries use
     the filter channel: `type IN ("task", "bug")` — resolved key→id
     server-side like any reference. A top-level `type` and a `type`
     filter compose by AND (same channel, two convenience levels — no
     ambiguity error).
- **Stored-view execution (`?view=`) substitutes dynamic placeholders.**
  SPEC §6.2's `_filter_template_<n>_` values are client-substituted and
  opaque to the middleware — a query evaluated server-side against the
  literal string matches nothing (v1 GetObjectsInList has exactly this
  silent-empty-result bug; do not copy it). The handler substitutes
  `_filter_template_2_` → the caller's participant id
  (`_participant_<space>_<account>` — the same identity §7.3's `@me`
  needs) and `_filter_template_1_` → the hosting object id before building
  the store query; any other placeholder degrades to a C6 warning on the
  response, never a silent no-match.
- **Sets AND collections both get a read path.** Phases 2–3 shipped a full
  collection write surface (POST /collections, `addItems`/`removeItems`)
  with no read/query endpoint — a collection's members were readable only
  as the raw `items` id array on GET object (unpaginated bare ids), a
  regression vs v1's `/lists/{listId}`. The GET routes above cover both:
  `sets/{id}/*` requires a set (its dataview source drives the query),
  `collections/{id}/*` requires a collection (membership rows = the store
  slice, in its order); one handler branches on layout exactly as v1's
  GetObjectsInList does, and a wrong-layout target is a 400 naming the
  other route. Rows follow C5.
- **Small-model form is settled** (C13): the structured `filters` array is
  recursive and not constrained-decodable, so the string is the documented
  default for small models; the array serves round-trip and programmatic
  composition (both ship — B2 only tunes steering, §4).
- **Engine exists; translation is the build.** `database.Query` already
  expresses the whole surface: full-text via `TextQuery`, any-key filters
  with `QuickOption` date presets and `NestedFilters`, any-key
  `SortRequest` with `Format`/`EmptyPlacement`/`IncludeTime`/`NoCollate`
  (v1's closed sort enum is purely an API-layer artifact). Also shipped
  and reusable: read-only option resolution (the Phase-3 `remove` path),
  `typePropertyKeys` + did-you-mean (`v2_refs.go`), `fields` row shaping
  (ListObjects + `MarshalPropertyValue`). The translation layer — the
  filter-string parser, the exported filter/sort fragment codec
  (`anyblockjson.UnmarshalFilters`/`UnmarshalSorts`, the one tree both
  request forms land on), the view-execution resolver over the **direct
  store-query path** (explicitly NOT v1's shared-subId
  `ObjectSearchSubscribe` hack — the constant `subId = "json-api-internal"`
  is racy under concurrent requests), and the global-search merge — is
  **built** (§8.4); only the [B3] rows encoder stays gated.
- Sort by any property key. **[B3]** `resultFormat=rows` gated as before
  (now runnable — the harness prerequisite, Phase 3a, is met).

### Phase 5 — the task-tool wrapper (CLI + skill + on-device manifest)

**Built** — `core/api/wrapper` (the tool table + manifest + runner) and
`cmd/anytype` (the verb-set) + `cmd/anytype/SKILL.md`; decisions as built
in §8.6. The dependency items below read as the plan they were; their
dispositions live in §3 (Built / still-open) and §8.6.

The Phase-5 deliverable is the **task-tool wrapper** (§7): the curated
~10-tool layer over `/v2`, delivered as (a) CLI verbs (`anytype find | read |
describe | create | set-properties | add-blocks | edit-text | check-item |
set-cell | move-block | delete-block`) with AXI/Chow output conventions and
SKILL.md three-tier packaging for coding-agent harnesses, and (b) a
function-calling/MCP manifest of the same tools for on-device small models.
Both are thin over the same server primitives; bulk work via scripts.
(Evidence §3.7; §7 for the tool contract.)

- **Hard dependency order.** The wrapper is code-complete only after:
  (1) **Phase-4 search + the filter-string parser** — backs `find`, the
  true blocker (no degraded form beyond ListObjects paging); (2)
  **`GenerateSchema` + the store-backed option join** — backs `describe`,
  a 501 stub today; an interim degraded describe can be assembled
  wrapper-side from `GET /types/{t}` + `GET /properties/{key}/options` so
  wrapper development can start; (3) the **markdown→flat-blocks parser**
  as an `insertBlocks` `markdown` payload alternative — backs `add_blocks`
  and upgrades the create shortcut off the two-change-set paste path
  (§8.1). Everything else — handle state, full-read relabeling, `@me`,
  relative dates, the GBNF artifacts, the D′1 escape decision — is §7.4
  wrapper-layer work that should not block starting.
- **CLI verb naming**: `move-block`/`delete-block`, matching the tool
  names — a plain `delete` would be read by coding agents as *object*
  deletion, which no v2 surface offers today (`DELETE /objects` is the
  open Phase-1 build item, due before this phase; when it ships, a plain
  `archive` verb may take the object meaning — until then object archive
  stays out of the wrapper).

## 3. Decisions ledger

**Decided**: id-addressed closed op set, no RFC 6902, no index/offset
addressing · `updateBlock` merge op + `replaceText` + `setCell` in the
launch op set (they back wrapper tools — §7/S1) · PUT-with-server-diff as
escape hatch (excluded from the small-model wrapper), full-block-id
round-trip default, diffStats · flat AnyBlock as the primary content
representation on the REST write path, plus **one markdown-in alternative:
an `insertBlocks` `markdown` payload** (mutually exclusive with `blocks`,
same targeting incl. root-append — the server parses; it backs the
wrapper's `add_blocks` channel and, once landed, the create shortcut);
markdown read-only otherwise · compact object ids + full block ids on REST
reads / **short handles on wrapper reads** · one vocabulary incl. `type`
(C2) · per-endpoint schema + worked example, strict-mode-compatible
(C12/C13) · path-addressed errors + /validate + dry_run + idempotency ·
etag advisory by default · filter string as the small-model filter form
(parser home: `pkg/lib/anyblockjson/filterstring` — it backs `find` in the
wrapper, Phase-4 search, and the POST /sets `filter` field; SPEC §6.2.1
scope split, v0.7) · atomic composite creates (sets via initial-state
dataview) · **search is a read** — exempt from Idempotency-Key and
dry_run (Phase 4) · **`type` as a filter pseudo-key**; top-level `type`
stays a single key · stored-view execution substitutes the §6.2 dynamic
placeholders (template_2 → caller participant, template_1 → host object;
others degrade to C6 warnings) · `@me` identity served by
`GET /members/me` server-side; sentinel + relative-date math in the
wrapper handler (§7.3) · **the small-model contract is the task-tool
wrapper (§7), not a REST mode**.

**Benchmark-gated**: B1 — whether *large*-model docs/steering prefer
`replaceText`/`setCell` over `updateBlock` (all are launch ops; nothing
ships or unships on B1) · B2 — which filter form the docs/steering
recommend per tier (both forms ship regardless; small-model primacy of the
string is settled — R8) · B3 tabular result format · B4 wrapper-tool
prompt/skill guidance.

**Deferred**: dataview/view ops · cross-object batch · block-scoped
preconditions · conflict rebase · events/subscriptions · core-profile
strict schema (FLAT.md §7.3) · `?permanent=true` hard delete.

**Named build items** (open today; budget them):

- **`resultFormat=rows` encoder**. [B3-gated]
- **`GenerateSchema` + store-backed option join** — the
  `types/{type}/schema` route is a 501 stub today. Phase 5 shipped
  `describe` in its sanctioned degraded form (wrapper-side composition,
  §8.6), so this item no longer blocks the wrapper — landing it collapses
  the wrapper's composition to one GET.
- **`DELETE /v2/spaces/{spaceId}/objects/{objectId}` (archive)** — specced
  with Phase 1, never registered; still open after Phase 5 (deliberately
  NOT a wrapper tool until it exists — §7.2/§2 Phase 5).
- **the D′1 escape decision** for `edit_text`/`replaceText` (§7.1) — still
  open; the tool description and SKILL.md carry the markup caveat.
- **an MCP server binary** hosting the manifest as a long-lived process —
  the integration point exists (`wrapper.BuildManifest()` + the
  MemoryStore runner, §8.6); the process wrapper itself is not built.
- **md-export loss detector** (converter/md has no warning channel).

**Built** (previously listed as build items; moved out so no one
re-budgets them): resolver wiring (create-missing properties/options —
`v2_resolver.go`, Phase 2) · referential validation layer (did-you-mean,
§8.1 policy, Phase 2) · `anyblockjson.Options` id-compaction split
(`CompactObjectRefs`/`CompactBlockLabels`, `CompactIds` shorthand — C4) ·
scalar→array coercion for list-shaped formats
(`anyblockjson.UnmarshalPropertyValue`, on every write path) ·
**filter-string parser** (`pkg/lib/anyblockjson/filterstring`, Phase 4 —
§8.4) · **exported filter/sort fragment codec**
(`anyblockjson.UnmarshalFilters`/`UnmarshalSorts`, Phase 4) ·
**view-execution resolver** (direct store query over setOf / the
collection store slice, with placeholder substitution — Phase 4, §8.4) ·
**global-search per-space loop + merge** with honest totals (Phase 4,
§8.4) · **POST /sets `filter` wiring** (the §8.1 501 is gone) · the
Phase-4 discovery additions (`search` kind; the grammar on the `filters`
kind) and the R9 sets-rule system-key widening · **markdown→flat-blocks
parser** (`anyblockjson.ParseMarkdownBlocks` + the `insertBlocks`
`markdown` payload + the single-change-set create fold — Phase 5, §8.6) ·
**the task-tool wrapper** (`core/api/wrapper`: the 12-tool table, manifest,
schemas, per-tool GBNF + the filter-string GBNF, handle/label session
state, ambiguity retry, idempotency machinery, `@me` + relative dates,
option pre-validation, degraded `describe` — Phase 5, §8.6) · **the CLI
verb-set** (`cmd/anytype`, generated from the same table) ·
**`GET /v2/spaces/{spaceId}/members/me`** (server-side identity) ·
**the Phase-6 chat surface** (§8.7: v2 chat DTOs + the inline-markup
bridge both directions, `GET/POST /chats` as C5 rows over a store query,
`GET /messages` with the state+messageCount passthrough + has_more
cursors, message POST/PATCH/DELETE + the reactions toggle with C8/C9,
`POST /read` requiring `{upTo, lastStateId}`, the five chat discovery
kinds, and the C8 DELETE widening — now uniform across every v2 DELETE)
· **the Phase-6 review hardening** (§8.7, 2026-08-06: the lastStateId
silent-no-op closed, chat RPC error classification incl. the new C6
`forbidden` code, text/attachment caps enforced + schema drift tests,
delete/toggle existence checks + file-GC warnings, RFC 3339 chat dates,
the reactions/reactedBy split, `blocksText`, and the chat handler test
layer) · **the Phase-7 periphery** (§8.8, 2026-08-06: the space surface —
`GET /v2/spaces/{spaceId}` as an RPC-free tech-space-view read,
`POST /v2/spaces` as ONE WorkspaceCreate call, `PATCH` with the
at-least-one-field contract, C8 on both mutations; the search
file-layout opt-in keyed off the type channel — positive type leaves
and the top-level type widen the row scope to `ObjectAndFileLayouts`
on both request forms — plus the `mimeType`/`size` aliases; the `space`
discovery kind) · **the Phase-7 review hardening** (§8.8, 2026-08-06:
per-space alias shadowing + the aliases live in filters/sorts, the
negation-scoped opt-in condition set incl. `allIn`, the live-space
predicate on GET-one/list + `description` on the list row, workspace
RPC error classification, the 4096 caps enforced, the no-space-id
500, the keyed POST /v2/spaces replay pin) · **key-scoping P1c:
whoami + legacy-key signals** (§8.11, 2026-08-06: `GET /v2/auth/whoami`
with the explicit `grant.scoped` boolean, per-entry space permissions
and grant-intersected names; the `Anytype-Key-Status`/`Anytype-Notice`/
`Link rel="deprecation"` signal — deliberately not RFC 9745
`Deprecation`/`Sunset` — with the body echo and the rate-limited usage
log; the gitleaks/TruffleHog rules in `docs/secret-scanning/`).

## 4. Benchmark program

All on the Phase-0 harness; small tiers run under constrained decoding
(R13). Metrics: apply-success, corruption (round-trip backtranslation),
output tokens, turns; per model tier.

- **B1 — edit-primitive steering** (tunes documentation; ships nothing —
  `replaceText`/`setCell` are launch ops per §2(c)/(d), so B1 no longer
  gates them): arms = **updateBlock-only** (`replaceText`/`setCell`
  withheld from the prompt) · **full launch op set**. **PUT-only runs as
  the corruption baseline**, anchoring the DELEGATE-52 comparison — it is
  not a gate arm (PUT's existence is decided). Decision rule: whether the
  REST docs and B4 guidance point *large* models at `replaceText`/`setCell`
  for text/cell edits or leave them on `updateBlock` (small-model steering
  is settled — the wrapper channels).
- **B2 — filter-form steering**: both forms ship regardless (the array is
  load-bearing for sets creation and round-trip; the string is the settled
  small-model form — R8), so B2 decides which form the docs/steering
  recommend for mid/frontier programmatic flows (round-trip, multi-step
  composition). Scored by execution semantics, never string equality.
- **B3 — result format** (gates `resultFormat=rows`): reading-accuracy +
  tokens, compact JSON vs rows over 10/100/1000-row results. Now runnable —
  the harness prerequisite (Phase 3a) is met.
- **B4 — creation guidance** (tunes prompt/SKILL.md guidance — *not* the
  C12 endpoint docs, which always ship schema + example): which in-context
  combination (example-only / schema+example / constrained core-profile)
  maximizes one-shot validity per tier.

## 5. Discovery

```
GET /v2/schemas                     # index: kinds, endpoints, examples, ops list (§8.2)
GET /v2/schemas/{kind}              # JSON Schema + worked example
                                    #   shipped kinds: object · shortcut · type · template ·
                                    #   property · set · collection · file · filters
                                    #   Phase 4 adds: search
GET /v2/schemas/ops/{op}            # per-op tiny strict schema + single-op minimal example
```

Per-op fetch keeps the smallest consumers at the smallest schema surface
(§3.5 capability cliff); the multi-op composite example (currently 7 ops,
§2a) remains as a secondary "multiple ops in one request" illustration. All
generation-facing schemas follow C13. **Phase-4 discovery**: a `search`
kind (strict request schema; its `filters` property references the
documented recursive exception), and the compact filter-string **grammar**
gets its discovery slot — served on the existing `filters` kind (one
concept, one slot, C2): that kind's response carries the structured-array
schema AND the string grammar (EBNF + examples), the same artifact the
Phase-5 GBNF conversion consumes (this assigns the C13 "filter string"
discovery slot, previously unassigned). The per-type artifact's route
shipped with Phase 1 as a 501 stub; the `GenerateSchema` artifact is an
open §3 build item.

## 6. Rollout

1. As built: `/v2` ships **ungated** alongside v1 on the same localhost
   server — no experimental flag exists (`V2Deps` is always fully
   populated; the only gating is nil-dependency skips for degraded test
   construction). Whether a flag is wanted before the first public release
   is an **open rollout task**, not a shipped fact. OpenAPI generated from
   day one.
2. Phase 2, then 3a/3b — shipped. **v1-parity note**: full parity
   additionally requires the Phase 1–2 surface additions of R11 — files
   and spaces/members reads shipped; **object archive (`DELETE /objects`)
   is the one R11 item still outstanding** (§2 Phase 1 [build]); §6's
   earlier "superset" claim is scoped to the object surface *including
   those*.
3. Benchmarks run once the harness + Phase 3a exist; gated items land in
   minor releases (additive to the closed op set).
4. Phase 4, Phase 5 — shipped (`cmd/anytype` is the CLI, §8.6). The v1
   deprecation clock starts when the CLI ships in a release build.

Each phase's exit criterion: its endpoints pass the harness's task set at
parity-or-better vs the v1 baseline flow for the same task (fewer calls,
fewer tokens, ≥ success rate), and every error message a failing run
produced has been reviewed as "actionable for a model" (C6).

## 7. Small-model surface: the task-tool wrapper

The full REST surface (§1–§6) is for large models and programmatic clients.
**Small models (3–4B on-device: Gemma 3n E4B and peers) never touch it
directly** — they drive a curated, task-shaped **tool-calling wrapper** over
the same primitives. This is the small-model review's conclusion
(`APIV2_REVIEW_SMALLMODEL.md`): a "mode" threaded through 30 endpoints leaves
the surface's sharp edges reaching a 3B; a wrapper both cuts the tool-count
cliff (>15 tools → 0–49%) and lets each tool pick a *channel* the raw REST
body can't. It is not the "ugly MCP over legacy" anti-pattern — the
underlying API is agent-native and the wrapper is task-shaped, which the
evidence actively recommends (Anthropic writing-tools-for-agents; Linear;
Notion markdown-for-agents).

**One artifact, two deliveries.** The wrapper is a single tool manifest
mapping ~10 task tools to `/v2` primitives; it is exposed as **CLI verbs**
(coding-agent harnesses; the CLI-over-MCP finding) and as an **on-device
function-calling / MCP manifest** (3B models). It is the Phase-5 deliverable.
Full REST + wrapper = two surfaces sharing one AnyBlock format, one
validation contract, one error contract. The two deliveries share the
manifest but NOT a state story: `find`'s enumerated handles outlive a tool
call, which a long-lived MCP process holds in memory while the
process-per-invocation CLI must persist — §7.4 states where handle state
lives.

### 7.1 The three channels that close the review's structural findings

The wrapper's leverage is that a tool argument can be a friendlier channel
than the REST body:

- **Authoring channel = markdown** (not AnyBlock JSON). `add_blocks` takes a
  markdown string; the server parses it to flat blocks. **Decided (v0.4):
  the parser's home is an `insertBlocks` `markdown` payload alternative** —
  mutually exclusive with `blocks`, same targeting incl. root-append — so
  markdown-in rides the whole op pipeline (validation, `createdBlocks`,
  `diffStats`, dry-run, idempotency) and the CLI vendors nothing; this is
  the "real server-side primitive" §7.3 item 1 demands (the create
  shortcut's two-change-set BlockPaste stopgap, §8.1, cannot back it).
  Removes inline-markup-in-JSON escaping (the top 3B failure) and the
  relative-indent arithmetic (markdown indentation → server computes
  `indent`). [closes S2]
- **Editing channel = anchor + deterministic server edit.** `edit_text`
  takes `find`/`replace`; the server does the string replace in code and
  applies it via the `replaceText` primitive. The model supplies a short
  anchor, never reproduces the block. [closes S1's change-one-word collapse]
  **Caveat — anchors and replacements are markup SOURCE**: `replaceText`
  matches the block's §8 markup text and re-parses the result, so a
  replacement containing `*`, `[` or mention syntax mints real marks —
  JSON-escaping is removed, markup-awareness is NOT (the open D′1 debt,
  a **named Phase-5 dependency for the small tier**; until it lands the
  tool description must say find/replace text is treated as markup). The
  tool also deliberately omits `replaceText`'s `replace_all` — single-match
  only for the small tier; the CLI may expose `--all` for larger consumers.
- **Reference channel = enumerated handles.** `find` returns `1,2,3`; `read`
  exposes short block labels **in outline mode — full body-bearing reads
  carry full 24-hex block ids (C4/T7), so the wrapper relabels them
  itself** (labels are unique suffixes; every write path resolves suffixes
  unconditionally via `matchBlockRef`, so labels pass straight through
  writes). The wrapper resolves handles/labels → CIDs, with the §7.4
  ambiguity retry. The model never sees or emits a 24-hex id. [closes S3]

### 7.2 Tool set (12 as built; flat, grammar-constrainable args)

| Tool | Args (flat) | Backing primitive | Channel notes |
|---|---|---|---|
| `spaces` | `limit?` | Phase 4 `GET /v2/spaces` | the bootstrap tool (added post-review): every trace needs a space id and nothing else in the set could produce one — `name — id` rows, no handles |
| `find` | `space, query?, type?, filter?, limit?` | Phase 4 search **[build — the true §7 blocker]** | filter = string form; results are enumerated handles + minimal fields |
| `read` | `object, mode=full\|outline` | Phase 1 read | outline returns short block labels; `full` carries full block ids the wrapper relabels (§7.1/§7.4) |
| `describe` | `space, type` | Phase 1 `types/{type}/schema?flavor=table` **[build — 501 stub today]** | the accuracy lever, **called before create/set** (folds A1 into the flow); interim degraded form assembled wrapper-side (§2 Phase 5); every backing GET is space-scoped, so the tool takes `space` too |
| `create` | `space, type, name, properties?, markdown?` | Phase 2 create | type and property keys validated with did-you-mean; **select option names create-missing by default (R9/§8.1)** — the small-tier pre-validation guard is wrapper-side (§7.4); markdown caveats until the parser lands (below) |
| `set_properties` | `object, set?{key: value}, add?{key: […]}, remove?{key: […]}` | `setProperties` op incl. per-key `add`/`remove` (§8.3) | mirrors the op so a one-tag append never rewrites the whole array (the op's entire rationale — reintroducing the read→rewrite→write trap at the wrapper layer would defeat it); `add` on a non-empty select errors, steering to `set`; scalar→array coercion is server-side |
| `check_item` | `object, block, checked` | `updateBlock` op | the one block-field tool: checkbox **blocks** are a common note shape and `updateBlock` is THE block-update op post-§8.3; other block-field updates (color/align/language/retype) stay excluded — SKILL.md steers task completion to properties (the E4 recipe) |
| `add_blocks` | `object, after?\|under?, markdown` | `insertBlocks` op, `markdown` payload **[build]** | **markdown channel**; server parses → flat blocks (§7.1) |
| `edit_text` | `object, block, find, replace` | `replaceText` op | **anchor channel**; deterministic server replace; find/replace text is markup source until D′1 lands (§7.1); an EMPTY `replace` deletes the found text (Required means present, not non-empty — §8.6) |
| `set_cell` | `object, table, row, col, value` | `setCell` op | flat cell write (as built the tool takes `object` too — the REST op addresses a table within one object, and a table-only reference would need a hidden cross-object table registry; §8.6); row/col take the labels full read mints (rows and columns relabel like blocks); an EMPTY `value` clears the cell (null on the wire) |
| `move_block` / `delete_block` | `object, block, after?\|under?` / `object, block, recursive?` | `moveBlock`/`deleteBlock` ops | handle-addressed |

Excluded from the wrapper: PUT full-document replace (the DELEGATE-52
corruption vector — REST-only, large models), multi-op batch (single tool
call per intent), the structured `filters` array (recursive, not
constrained-decodable — string only), relative-indent authoring (markdown
channel replaces it), block-field updates beyond `checked` (deliberate
curation, recorded so the gap is not read as an artifact of the
replaceBlock-era table), object archive (no v2 route yet — §2 Phase 1
[build]), set building (`POST /sets` is the REST path; the "build a set
with filter" eval task runs on the REST surface, not the wrapper).

**Create-with-markdown caveats — DISSOLVED (Phase 5 as built).** The
markdown→flat-blocks parser landed (`anyblockjson.ParseMarkdownBlocks`) and
the create shortcut folds markdown into the single-change-set create
snapshot: dry runs validate the markdown too, a failure builds nothing, and
the C8 cache replays safely. The historical two-operation paste path
(create snapshot, then v1 BlockPaste — §8.1) is gone; this paragraph
survives only so the old caveats are not re-derived.

### 7.3 What the wrapper does NOT let us skip

1. **The bounded server primitives must exist** — `edit_text`/`set_cell` are
   safe only because `replaceText`/`setCell` are real server-side scoped ops
   (the S1 launch reweighting in §3). A wrapper that implemented them as
   GET+regenerate+PUT would reintroduce corruption.
2. **Constrained decoding still required, now tractable** — on-device
   function-calling (Ollama/llama.cpp GBNF) needs the *tool* schemas
   grammar-emittable; C13 applies to the small flat tool args here instead of
   the recursive block tree. The wrapper serves a GBNF/CFG artifact per tool
   (a Phase-5 build item), including the filter-string grammar for `find` —
   **sequenced after the Phase-4 parser pins the grammar** (§6.2.1's design
   is the source; no GBNF or JSON-Schema→grammar seam exists anywhere yet).
3. **Conveniences, placed** — scalar→array coercion is **already served**
   (`anyblockjson.UnmarshalPropertyValue` wraps scalars of list-shaped
   formats; every write path routes through it — not wrapper work). Still
   to build, placement decided: `GET /v2/spaces/{spaceId}/members/me`
   **server-side** (only the server knows the account identity — the same
   identity Phase 4's placeholder substitution uses); `@me` sentinel
   resolution and relative-date math **in the wrapper handler** (simplest,
   matches this section's framing — the REST value path stays literal).
   Option-name accuracy for the small tier is a **wrapper-side
   pre-validation pass** (§7.4) — the REST primitives create missing
   select option names by design (R9/§8.1), so the A2 guard the wrapper
   wants must sit in front of them, not be assumed of them.
   If-Match/Idempotency-Key management stays wrapper-owned (the model
   authors none of these).

### 7.4 New build items for the wrapper

Beyond §3's list:

- **markdown→flat-blocks parser** — the `insertBlocks` `markdown` payload
  alternative (§7.1; also dissolves the create-shortcut caveats in §7.2).
- **handle↔CID resolver, fully stated**: (a) *handle state outlives a CLI
  invocation* — persisted in a session file (scratch dir, keyed by space),
  written by `find`, read by the id-taking verbs, invalidated/renumbered
  by each new `find`; the MCP delivery keeps the same table in memory.
  (Alternative considered: stateless short unique object-id prefixes
  resolved server-side like block suffixes — a simpler state story, but
  handles stop being small stable integers and prefixes grow with the
  space; revisit if the session file proves fragile under concurrent
  harnesses.) (b) *wrapper-side relabeling of full-read block ids* — full
  body-bearing reads carry full ids (C4/T7; the S3 `?ids=compact` shape
  was consciously not adopted), so the wrapper relabels and keeps the
  label→full-id map. (c) *suffix pass-through* as the write mechanism
  (`matchBlockRef` resolves unique suffixes on every op). (d) *the
  ambiguity retry, scoped honestly (post-review)* — retained labels
  resolve to full ids BEFORE the send, so a 400 `ambiguous_input` can
  only name an unretained ref (an outline label, a pruned session); the
  wrapper re-reads the object and retries once when the ref resolves
  uniquely against the CURRENT document, which self-heals exactly the
  concurrent-modification race (the collision the server saw is gone by
  the re-read). A persistent ambiguity is unresolvable in principle —
  the wrapper cannot know which block the model meant — and surfaces
  the server's error; the re-read's labels are retained either way, so
  the model's next call starts resolved.
- **wrapper-side option-name pre-validation** (the A2 guard for the small
  tier): `GET /properties/{key}/options` + did-you-mean before
  `create`/`set_properties`, stated as wrapper logic the REST primitive
  does not perform (R9/§8.1 create-missing stands on REST).
- **per-tool GBNF/CFG artifacts** — after the Phase-4 parser pins the
  filter grammar; keep the served op/tool schemas' `$ref`/`$defs` style
  within what the chosen GBNF converter supports, and assert
  convertibility by test to keep C13 honest.
- **`@me` self-resolution** (+ server-side `GET /members/me`) ·
  **relative-date input resolution** (placement per §7.3 item 3).
- **the D′1 escape decision** for `edit_text` (§7.1 — escape `replace` for
  text-bearing blocks, or plain-text find/replace with offset-shifted
  marks; a named Phase-5 dependency for the small tier).

These join `replaceText`/`setCell` (launch ops) and the filter-string
parser (a launch dependency) as the small-model launch set. Benchmark
**B4** tunes the wrapper's tool descriptions / SKILL guidance per model
tier.

## 8. Implementation notes (v0.3.1 — closes Phase-0/1 gaps for a fresh build)

Concrete decisions a fresh implementer needs, so the design ambiguities that
would stall Phase 0/1 are resolved (genuine format/design ambiguities still
surface as spec bugs per the package's tradition).

**Server & auth.** `/v2` **extends the existing `core/api` gin server** — a
new route group under the same middleware stack (Recovery / Metadata /
Logger / Pagination / Cache / Auth / RateLimit / Analytics) and the same
layering as v1 (handlers in `core/api/handler`, services in
`core/api/service`, DTOs in `core/api/model`, routes in
`core/api/server/router.go`; follow `core/api/CLAUDE.md` — fixture pattern,
mockery, error wrapping). **Auth is shared with v1**: `/v2` reuses the v1
bearer token, api-key, and challenge endpoints and the Auth middleware — no
new authentication surface. One `/v2`-only *authorization* addition landed
later: a group-level key-scope gate — only `JsonAPI`- and `Full`-scoped
keys may use `/v2`; a `Limited` key gets 403 here while staying served on
`/v1` (§8.9). Pagination is offset/limit as in v1.

**etag (C7), concrete.** The agent-facing token = first 8 hex of
`sha256(sorted object tree heads)` (heads via `sb.GetDocInfo().Heads` in
the adapter, captured under the same locked read as the snapshot). Returned
as the `ETag` header and the envelope `etag`. `If-Match` comparison runs
server-side against the **full** head-set hash; **the 8-char display form
is accepted as its prefix**, so the etag a GET returned can be sent back
verbatim in `If-Match` (quoted, `W/`-prefixed and bare forms are all
normalized — RFC 7232). It is NOT the `revision` relation. Advisory by
default (C7): absent `If-Match` ⇒ last-write-wins.

**Read path (C5/Phase 1), concrete.** Read via the **live smartblock state**
→ snapshot → `anyblockjson.Marshal` with objectstore-backed
format/option/property resolvers — NOT `ObjectShow` (its `ObjectView` is the
wrong type) and NOT the store snapshot (it lags live edits/sync). Derive the
`etag` from that same state read so token and content are consistent. The
per-object read is exactly `cmd/anyblockroundtrip`'s snapshot→Marshal path,
minus the round-trip.

**`?block=` subtree, concrete.** Indents stay **absolute** (not rebased to
the anchor) so a block's id and depth are identical whether read in full or
as a subtree — an agent can cache and cross-reference either way. The block
reference resolves by **exact id or unique suffix** (§9a): a compact outline
label (the id's last few chars) round-trips to `?block=`; a suffix matching
more than one id is a 400 steering to the full id; zero matches is a 404.
`?block=` implies blocks (an accompanying `include=properties` adds the
properties map rather than emptying the read).

**`/validate` result semantics, concrete.** `POST /v2/validate` returns
**200 with `{issues, warnings}`** even for an invalid document — the issues
are the repair-loop's food, never a 4xx. A document produced by a newer
format version surfaces as a `/version` issue with a hint (SPEC §10), not a
transport error. (The `version_unsupported` HTTP error exists for Phase-2
*body* rejection on create/replace, where an unparseable version must fail
the write.)

**C11 read warnings, concrete.** A read never fails on content the format
can't represent: unmapped block types and over-deep nesting degrade to
`warnings[]` on the envelope (the `anyblockjson.Options.OnWarning` sink);
canonical export leaves the sink nil and still errors. The markdown export
path has no loss channel yet, so `format=md` carries no warnings (build
item: md-export loss detector).

**Idempotency store (C8).** In-process, keyed by `(space, Idempotency-Key)` →
`(body-hash, stored-result)`, with an in-flight reservation so concurrent
same-key requests never double-execute (the second blocks then replays).
Same key + same body-hash ⇒ replay stored result; same key + different
body-hash ⇒ 409 `idempotency_conflict`. Only successful (2xx) responses are
cached; a failed/panicked request releases the reservation so a retry
re-executes. TTL is an impl detail (a bounded LRU is fine); persistence
across restart is not required for v2.0.

**Eval-harness ordering (correction to §2 Phase 0).** Phase 0 delivers the
**scoring primitives + plumbing + `/validate`**, NOT the full agent-loop
runner (which has nothing to score until an edit path exists). Scoring
primitives: (a) the **corruption metric** — DELEGATE-52 backtranslation: apply
a forward edit instruction and its inverse in sequence, then measure residual
drift against the untouched document using the state-diff / text-multiset
comparator already in `cmd/anyblockroundtrip`; (b) token and turn counters;
(c) the task fixtures. The **agent-loop runner pairs with Phase 3a** — the
first point at which benchmark B1 has competing methods to score. Scratch
space = an ephemeral test account/space provisioned as
`cmd/anyblockroundtrip` does (data-dir copy). The small-tier model runner
MUST support grammar-constrained decoding (GBNF via Ollama/llama.cpp), per
the constrained-decoding requirement (C13, R13).

**First-increment scope (fresh build).** Phase 0 plumbing (C6 error contract
incl. the named codes; C7 etag; C8 idempotency store; C9 dry-run scaffold) +
`POST /v2/validate` (exposes `anyblockjson.Validate`, structural/format-
semantic only) + Phase 1 read endpoints (`GET object` with
`include`/`outline`/`block`/`ids`/`format`; `GET objects`/`types`/
`properties`/`spaces`/`members` lists) + the Phase-0 scoring primitives above.
Exit criterion: Phase 1 reads pass the fixture task set at parity-or-better
vs the v1 GET flow (fewer tokens for the same content), and the scoring
primitives run against the fixtures. Phases 2–7 are out of this increment.

### 8.1 Phase-2 implementation notes (v0.3.2 — decisions as built)

**The create path, decided.** One mechanism for every create:
`anyblockjson.Unmarshal` → `state.NewDocFromSnapshot` →
`objectcreator.CreateSmartBlockFromState` (a new `apicore.ObjectCreator`
port + `core/api` adapter, the same pattern as Phase 0's `ObjectReader`).
The whole document lands as the object's **initial state — one change set**,
which is what makes composite creates (a set with its dataview) honestly
atomic (R10). Rejected alternatives: the import engine's
`common/objectcreator.Create` (needs the whole import DataObject machinery —
id remapping, payload pre-creation, file syncers — the wrong altitude for a
single-object API create) and create-empty-then-`ResetToVersion` (two change
sets, plus the `canUpdateObject` exclusions; that diff-apply shape is
Phase 3b's PUT, not create). §7 structural blocks (title/description) stay
absent per SPEC §7 — the editor regenerates them at first open. Bundled
types/relations a document references are installed on the way (mirrors the
import path's `installBundledRelationsAndTypes`); the `origin` detail is
stamped `api`. The create response's `etag` comes from an immediate Phase-1
read-back (best effort — failure degrades to a warning, never a 5xx after a
successful create).

**Create-vs-reject policy (R9), explicit.** Select/multiSelect option
*names* → **create-missing** everywhere they appear as values (SPEC §3);
`typeProperties` keys on POST/PATCH types → **create-missing** (SPEC §2a);
property keys in an object's `properties` map → **reject** with a
path-addressed did-you-mean listing the space's actual keys; type keys
(`type`, `templateFor`, a set's `type`) → **reject** with did-you-mean;
set filter/sort/view property keys → **reject** unless among the queried
type's recommended keys (+ `name`), listing the type's actual keys.
Did-you-mean ranks by prefix/containment and edit distance ≤ 2. Created
side effects (real or would-be, on dry runs) are reported under `created`
in the response.

**Types.** POST /types routes through the `ObjectCreateObjectType` RPC (it
owns unique-key derivation, recommended-relation filling/install, layout
detail, orderId, bundled-template install) with details built from the
unmarshaled type document; `typeProperties` resolves through the
create-missing resolver first. `recommendedLayout` accepts the layout
**name** (`"todo"` — the §2a worked example's shape) as well as the stored
number; note the anyblockjson exporter currently passes the stored int64
through verbatim, so the SPEC §2a example and export disagree (flagged as a
SPEC bug, resolution pending). A type document carrying `blocks` (its
dataview) is **rejected explicitly** for now — the editor generates default
views at first open (the case SPEC §2a already blesses); silently dropping
the views would violate C11's write rule. Deferral, not a decision.

**Sets.** The synthesized set document pins the dataview block id to
`"dataview"` (`template.DataviewBlockId`) — any other id makes the editor
add a second, default dataview at first open. The compact `filter` string
returns 501 `not_implemented` steering to `filters` (the parser is the
Phase-4 build item); `filter`+`filters` → 400 `ambiguous_input` (C6);
`views` is mutually exclusive with top-level `filters`/`sorts`.

**Shortcut markdown.** `{type,name,properties,markdown}` creates from the
synthesized document, then lands `markdown` via the v1 block-paste path —
the one place a create takes two change sets, accepted as a convenience
until the markdown→flat-blocks parser (Phase-5 build item) exists; dry runs
validate type/properties only and say so in `warnings`.

**Idempotency hash identifies the whole request** (corrected in v0.4 to the
shipped formula): `(space, key)` → `sha256(method ‖ 0x00 ‖ path ‖ 0x00 ‖
rawQuery ‖ 0x00 ‖ body)`. The query inclusion means a cached `?dry_run=true`
result never replays as its later real twin; the method+path inclusion is
equally load-bearing and agent-visible — one key reused across two
byte-identical PATCHes to *different objects* (the object id lives in the
path) 409s instead of silently replaying the first object's success. Same
key with any differing part is a 409 `idempotency_conflict`, per C8's
"different body" rule read as "different request".

**Discovery (§5) as shipped.** `GET /v2/schemas` + `/v2/schemas/{kind}` for
kinds `object · shortcut · type · template · property · set · collection ·
file · filters`. AnyBlock-document kinds serve the format's embedded schema
verbatim; the rest are hand-written strict (C13) schemas; `filters` is the
documented recursive exception. Every served example passes
`anyblockjson.Validate` (enforced by test). `/v2/schemas/ops/{op}` shipped
with Phase 3 (§8.2).

### 8.2 Phase-3 implementation notes (v0.3.4 — decisions as built)

**The edit pipeline, redecided (v0.3.4 — supersedes v0.3.3's
document-level apply).** PATCH ops are **operations**: they apply to a
child `*state.State` of the live object and the adapter commits with ONE
ordinary `sb.Apply(st)` — exactly the Block* RPC handler model. The
v0.3.3 pipeline (marshal → mutate flat JSON → Unmarshal →
`ResetToVersion`) reset onto the live object a snapshot the format
deliberately does not fully carry (no RelationLinks, no structural
blocks, no resolvedLayout, no extra type keys) and applied it with
`NoHistory`/`DoSnapshot`/`NoRestrictions` — the Tier-A cluster of the
Phase-3 review. The state route fixes those **by construction**: a child
state inherits everything the live doc owns, Apply runs per-block
restriction checks, records undo, fires hooks/events, and emits the
minimal id-matched change diff with no forced full snapshot. The flat
document is still rendered under the lock, but only as the **read-only
view** the ops address (refs, suffixes, indent arithmetic, error texts —
the same shape agents read) and as the diffStats input; payload blocks
are interpreted by the format package at **fragment granularity**
(`anyblockjson.UnmarshalBlocks`/`UnmarshalBlock`/`UnmarshalPropertyValue`
+ the inline codec), so only the format package ever parses AnyBlock
JSON.

**The mutation port (v0.3.4).** `apicore.ObjectMutator` has two methods.
`MutateObject(ctx, spaceId, objectId, apply func(edit ObjectEdit) error)`
is PATCH: the adapter locks the object, checks the object-level
Blocks/Details restrictions, hands `apply` an `ObjectEdit{SbType, Heads,
State}` (State = `sb.NewState()`), runs the bundled-revision guard (never
downgrade `revision`; untouched keys are simply inherited now), and
commits with a plain `sb.Apply`. `ResetObject(ctx, spaceId, objectId,
build func(cur ObjectRead) (*SmartBlockSnapshotBase, error))` keeps the
v0.3.3 reset-to-version machinery (with `preserveEditorOwnedState`) for
PUT until its own rework. Dry runs never touch the mutator: the same op
applier runs on a private `state.NewDocFromSnapshot` of a plain read.

**Ops → state, exact.** setProperties → `st.SetDetail`/`RemoveDetail` +
`st.AddRelationLinks` for the key (mandatory — a value without its link
is wiped on replay, the A1 class); values decode via
`UnmarshalPropertyValue` (dates, option names, ref lists — §3 rules).
updateBlock → merge on the block's exported JSON form → `UnmarshalBlock`
with the forced id → set in place, live ChildrenIds kept (non-table).
replaceSubtree → fragment run →
unlink old subtree, splice the run's top blocks at the old position (id
reuse from the replaced subtree is allowed). insertBlocks →
`st.InsertTo` (after→Bottom, before→Top, inside last→Inner, inside
first→InnerFirst). moveBlock → `st.Unlink` + `st.InsertTo` (children
ride along). deleteBlock → `st.Unlink` (apply-side normalization drops
the orphaned subtree). replaceText → find/replace on the block's
document text (markup source; literal for code/embed §8.4) →
`ParseInlineText` back to text+marks. setCell → edit the cell on the
table's document form, re-import the one table block (rows/columns/
derived cell ids round-trip, so untouched cells land unchanged; the
internal wrapper blocks are re-minted — accepted churn, strictly less
than v0.3.3's whole-document reimport). addItems/removeItems →
`st.GetStoreSlice("objects")`/`UpdateStoreSlice`.

**R5 post-op validity: fragment pre-checks + the restored whole-document
net** (corrected in v0.4 — the previous text described a dropped-checks
draft with an opt-in log-only net that never shipped; the shipped design
is review B′3's, see §8.3). V1 monotonicity is structural — a state tree
has no indent arithmetic to get wrong — plus the unchanged payload-run
monotonicity pre-check. Fragment payloads are validated by wrapping the
run in a minimal synthetic page document and running the format's
document validation (so the §5 shape checks apply verbatim); a failure
rejects the whole PATCH under the unchanged message "the ops would
produce an invalid document — no op was applied", with fragment-relative
block paths. Structural block types
(`title`/`description`/`featuredProperties`) are rejected explicitly in
payloads (the whole-document import would have absorbed them silently),
and no primary-dataview pinning happens on fragments. Id uniqueness —
v0.3.3's V5 net — is an explicit check against the live state and the
PATCH's own claims, with op-shaped `ops[i].blocks[j].id` paths (an
improvement over the old document paths); it also covers ids the format
keeps out of the document (table internals, structural blocks), which
the old net could not see. The op-shaped pre-checks (cycle, leaf
containment, delete-without-recursive, leaf-anchor) are unchanged,
running against the view. **On top of the pre-checks, the R5
whole-document net is ON by default and rejecting**:
`anyblockjson.Validate` runs on the marshaled would-be after-document —
nearly free, since the after-document is already marshaled for
diffStats — restoring the invariants no single fragment can see (V3
row→column containment, the document-wide id domain, the absolute
nesting bound). A failure rejects the whole PATCH under the same
message. `ANYTYPE_API_V2_SKIP_EDIT_VALIDATE=1` is the **debug-only
disable** (for a suspected false rejection); there is no log-only mode.

**Create-missing runs before the lock (v0.3.4, review B6/A6).** The only
create surface in PATCH payloads is setProperties select/multiSelect
option names; a lenient pre-pass resolves (and creates) them before
`MutateObject`, so no create-RPC ever runs while holding the edited
object's lock. In-lock resolution hits the resolver cache. Trade-off,
documented — and narrowed in v0.3.5 (review A′1, §8.3): the prewarm now
runs only after the object read and the precondition checks pass, so the
leak is confined to ops that fail **validation** later; a PATCH to a
nonexistent/restricted object or with a stale If-Match no longer creates
the options it named (v0.3.3 leaked the same way for post-Unmarshal
failures).

**diffStats stay the canonical document diff.** Considered and rejected:
deriving them from `st.GetChanges()` — the change list only exists after
a real Apply, so dry runs (which never Apply) would diverge from real
runs, breaking C9's dry≡real contract. The before/after documents are
rendered under the lock anyway (the view), so the numbers are unchanged —
and with the Tier-A churn gone from the emitted changes, the diff is no
longer blind to anything real (C9's concern).

**Untouched state is untouched (v0.3.4).** Because nothing round-trips
the whole document anymore: relation links, structural blocks,
resolvedLayout, extra type keys, hidden legacy children (link/bookmark/
content-less blocks), big integers in untouched blocks' fields, stored
option ids — all simply inherited by the child state. The C11 marshal
guard remains on PATCH (a loss warning on the live state still refuses
the edit — the view itself would be lossy), and B8's float64 concern is
narrowed to the one block an op re-imports.

**C11 write-safety guard (PATCH only).** If the internal marshal of the live
state reports any loss warning (unmapped block, over-deep nesting), the
PATCH is refused (422) — otherwise the write-back would silently drop the
unrepresentable content, exactly what C11 forbids. PUT skips the guard (a
full replace is explicitly destructive; the GET the body came from carried
the same warnings) and surfaces the warnings instead.

**diffStats.** Canonical-before vs canonical-after document diff (the after
side is the applied snapshot re-marshaled, so import/export normalization
cancels): added/removed by block-id set; changed = same id, different
content (block JSON minus indent/id); moved = parent changed OR the nearest
*common* preceding sibling changed (pure insertions don't mark their
followers moved). `propertiesChanged` counts differing keys.
`addItems`/`removeItems` changes are not counted (no diffStats field for
membership; deliberate — the schema is closed at the five integers).

**Relative indent (R3), exact.** Payload `indent` 0 = the anchor's level
(after/before/replaceSubtree) or the container's child level (inside);
payload runs must start at 0 and obey +1 monotonicity internally, checked
with `ops[i].blocks[j].indent` paths before the document-level net.
`insertBlocks` inserts after the anchor's whole subtree for `after`;
`inside` defaults `position` to `last`; `position` with `after`/`before` is
an error. `moveBlock` moves the whole subtree and re-bases its indents.

**Small op decisions.** `updateBlock`: `set` is merge; explicit `null`
clears a field; `id`/`indent` in `set` are rejected (steering to moveBlock);
the addressed id and indent survive a retype. `replaceSubtree`
mints fresh ids for id-less payload blocks (the old subtree's ids die with
it). Every payload block — minted or client-supplied — lands in
`createdBlocks` keyed `ops[i].blocks[j]`. `replaceText` requires a
text-bearing type and a non-empty `find`; error texts are the Anthropic
shapes ("no match found…", "found N matches — provide more context…").
`setCell` resolves row/col ids with the same unique-suffix leniency as block
refs and accepts all §6.1 cell forms (string, null, block object, array);
invalid inner shapes fall to the R5 net. `addItems`/`removeItems` require a
collection (type `collection` or a collection-layout type), dedupe/no-op
respectively, and do not existence-check member ids (v1 parity).
`setProperties`: §4a output-only keys rejected (`isFavorite` stays
authorable per SPEC §3), unknown keys rejected with did-you-mean (Phase-2
policy), select option names create-missing and ride `created`, `unset` of
an absent key is a no-op, a key in both `set` and `unset` is an error.

**PUT.** The envelope `etag`/`warnings` a GET body carries are stripped
before validation (C7: preconditions are the If-Match header only); a
non-matching envelope `id` is rejected, and the id is pinned to the URL's. A
body without `type` keeps the live object's type (a replace is about
content, not retyping by omission). The R9 referential layer guards PUT like
create. `canUpdateObject` mirrored as smartblock-type exclusions
(relation, relation option, file object, participant) — applied to PATCH
too, same machinery, same risks.

**Structural blocks.** As with Phase-2 creates, SPEC §7 structural blocks
(title/description) are absent from the format, so an edit re-lands the
document without them; name/description content lives in `properties` and
survives, and the editor regenerates the header blocks (same §7 contract).

**Per-op discovery (§5) as shipped.** `GET /v2/schemas/ops/{op}` for the
launch ops; each schema is C13-strict and self-contained, with a shared
payload-block definition covering the realistic edit fields
(`additionalProperties:false` — the full block inventory stays at
`/v2/schemas/object`, which the def points to). Every example is a full
single-op PATCH body (enforced by test); the index (`GET /v2/schemas`) grew
an `ops` list.

### 8.3 Phase-3 revisions (v0.3.5 — pre-release design review, decisions as built)

Six contract changes from the modification-surface design review, taken
while the API is unreleased and breaking changes are cheap.

**Root targeting for `insertBlocks`/`moveBlock`.** Omitting all of
`after`/`before`/`inside` appends at the end of the document root (state:
`InsertTo("", Block_Inner)`). Chosen shape: the omitted-anchor form, not an
explicit `at: "start"|"end"` field — fewer fields for a small model, and it
is exactly the §7 wrapper's omitted-`after` case. No root-prepend; `position`
still requires `inside` (position without any targeting field is a 400
naming the root-append behavior). This closes the structural hole where an
empty object (SPEC §7: no title/description blocks in the document) had zero
addressable anchors and PUT was the only way to give it content. More than
one targeting field is now "at most one of after, before, inside is allowed"
(was "exactly one … is required" — reworded because zero is legal now).
Payload indents stay R3-relative: at root, indent 0 = document top level.

**`replaceBlock` removed (BREAKING, deliberate — the API is unreleased).**
Four routes to changing a block's text (updateBlock/replaceBlock/
replaceSubtree/replaceText) was the surface's largest disambiguation load,
and `replaceBlock`'s silent text-wipe (a checkbox toggle via replaceBlock
losing the text) was the documented small-model trap; BlockNote and Tiptap
each ship ONE block-update op. `updateBlock {id, set}` — merge with
explicit-null-clears — expresses everything replaceBlock did except the
wipe (a full wipe is `set` naming every field, `null`ing the rest, or
`replaceSubtree`). The op set is 10 ops. An agent that sends `replaceBlock`
gets the unknown-op error with a hint that names updateBlock's semantics
before listing the allowed ops. All other error texts are unchanged.

**`setProperties` per-key `add`/`remove`.** Only for list-shaped formats
(select/multiSelect/objects/files); scalar-format keys are rejected
path-addressed, naming the format. `add` resolves entries with the same
create-missing option-name semantics as `set` — including in the pre-lock
prewarm, which scans `add` alongside `set` so no create-RPC runs under the
object lock. `remove` resolves entries READ-ONLY (store-backed resolver):
a remove must never mint the very option it names; unresolved names match
nothing. `remove` of the last entry leaves the key present-but-empty
(`unset` removes presence); `remove` of an absent key stays absent. SPEC §3
presence semantics unchanged: `set: []` still means present-but-empty. Key
validation matches `set` (output-only rejected, unknown keys did-you-mean);
a key in more than one of set/unset/add/remove is a path-addressed error.
The empty-op error is now "setProperties needs at least one of set, unset,
add, remove".

**Idempotency-Key covers PATCH and PUT (C8 widened).** The store, request
hash, in-flight reservation and replay were POST-only wiring; agents
auto-retry on timeout, and PATCH is where a blind retry does damage (a
retried successful `insertBlocks` duplicates blocks; a retried `deleteBlock`
404s misleadingly). The middleware now acts on POST, PATCH and PUT and is
registered on the object PATCH/PUT routes and the types/properties PATCH
routes. Semantics unchanged: same key + same request (the §8.1 hash —
method ‖ path ‖ query ‖ body) ⇒ replay; any differing part ⇒ 409
`idempotency_conflict`; only 2xx results are cached.

**The R5 whole-document net restored (review B′3).** The state pipeline's
fragment validation cannot see invariants that span the spliced result —
V3 row→column containment, the document-wide id domain, the absolute
nesting bound — so the whole-document `anyblockjson.Validate` runs on the
marshaled would-be after-document **by default** and **rejects** the PATCH
on failure (same agent-facing message; nearly free — the after-document is
already marshaled for diffStats). `ANYTYPE_API_V2_SKIP_EDIT_VALIDATE=1` is
the debug-only disable. §8.2's post-op-validity paragraph is corrected
accordingly (it previously described a dropped-checks draft with an opt-in
log-only net that never shipped).

**Edit-path ordering and dry-run parity (reviews A′1/C′3).**
`apicore.ObjectRead` carries the per-axis restriction verdicts
(`BlocksRefused`/`DetailsRefused` — per-op since surface review M1) —
captured under the same locked read as the snapshot and heads — checked
before the prewarm and on dry runs, so a dry run reaches the same 403 the
adapter would return (C9's dry≡real contract now covers restrictions; the
refusal really is a 403 since surface review M2a — see §8.12). And the create-missing prewarm runs only AFTER the object
read and the precondition checks pass (read → preconditions → prewarm →
lock): a PATCH to a nonexistent or restricted object, or with a stale
If-Match, no longer creates the options it named — §8.2's documented
option-leak trade-off is thereby narrowed to validation failures only.

### 8.4 Phase-4 implementation notes (decisions as built)

**The parser** (`pkg/lib/anyblockjson/filterstring`). Recursive-descent
over the §6.2.1 grammar; emits the §6.2 structured filters ARRAY as
canonical JSON — the literal convergence point: the API feeds either the
client's structured array or the parser's output through the same
`anyblockjson.UnmarshalFilters` call, so there is exactly one
JSON-tree→model translation. Decisions a reader of §6.2.1 needs: keywords
match case-insensitively (canonical rendering stays uppercase — small
models write `and`); the keywords are reserved words, rejected as property
keys; RFC 3339 → unix conversion happens at parse time and only for keys
whose format resolves to `date` through the wired resolver (a date-looking
string on a text property stays a string; a non-RFC-3339 string on a date
property is a parse error steering to the preset functions); a preset
function on a non-date key is a parse error naming the actual format; the
counting presets require a whole non-negative operand and keep it as
`value`; presets are excluded from value lists; set literals require `=` /
`!=` (a list after an ordering operator errors). Reference sets are wired
per call site: `KnownKeys` (offset-addressed unknown-key error +
did-you-mean), `KnownOptions` (read-only option names — the QUERY path
wires it; the SETS-CREATE path deliberately does not, because a set create
is a write where option names create-missing per R9/§8.1). Every error is
`*filterstring.Error{Offset, Token, Message, Hint}`; the API maps it to
one C6 issue at `/filter` carrying the offset text. The EBNF the parser
pins is exported (`filterstring.EBNF` + `Examples`) and served on the
`filters` discovery kind (§5) via new `grammar`/`grammarExamples` fields
on the schema-entry payload — every served example is
asserted-parseable by test.

**The fragment codec** (`anyblockjson.UnmarshalFilters`/`UnmarshalSorts`).
Validates the enum vocabulary (conditions, datePresets, directions,
emptyPlacement, operators) with `/filters/i/…`-`/sorts/i/…` paths, then
reuses the document path's per-view semantic checks (counting-preset
operand, placeholder-on-non-object rule, and the unguarded-date-comparison
WARNING — the same text, riding `Options.OnWarning`, which the search
handler forwards onto the response `warnings`; for the string form the
issue path is remapped to `/filter`). Conversion runs through the same
importer the whole-document dataview path uses, so option-name→id
resolution and format rehydration behave identically on both request
forms.

**Search execution.** One `searchPlan` per space: reference set (rules
1–2, plus `type` as a pseudo-key), both filter forms → one model tree,
read-only option pre-validation (rule 3 — structured form path-addressed
here, string form offset-addressed in the parser), `type`-leaf key→id
resolution AFTER the shared codec (both forms converge before it), and an
**effective sort list**: explicit sorts win; a full-text query without a
score sort gets `_final_score desc` appended as tiebreak (which also
stops the engine from PREPENDING its own score sort — explicit sorts stay
primary under full-text); no sorts and no query defaults to
`lastModifiedDate desc` (the ListObjects order). Non-text queries run
`QueryAndCount` (store-side paging + honest total); full-text queries
materialize the (candidate-bounded) result set and page in memory — the
engine's `QueryAndCount` cannot do fulltext, and `total = len(matches)`
of the materialized set is the honest count within the engine's
documented candidate budget. Base row scope mirrors ListObjects (object
layouts, no templates, no hidden). Global search merges per-space pages
by the effective sort list with a value comparator (no locale collation —
an accepted approximation of the store's order; ties break by space id
then object id for determinism); per-space failures skip the space with a
"space X was skipped: …" warning, and only when NO space resolves does
the first per-space error become the response. Global rows carry
`spaceId` (addressing info for the follow-up read — a deliberate C5
extension). `V2ListResponse` gained `warnings` (C6-shaped, deduped).
The request schema is strict: an unknown body field 400s, and
`limit`/`offset` in the body get the C10 steering hint. Search routes
carry no idempotency middleware (asserted by a router test: the same
keyed authorized search executed twice runs twice and never returns an
`Idempotency-Replayed` header — the earlier 401-based assertion was
vacuous, since group auth aborts before any route middleware) and the
handlers never read the dry-run flag.

**Sets/collections reads.** Layout is read from the live snapshot's
`resolvedLayout` (the same locked read as the content); the wrong-layout
400 names the other route verbatim. A set's `setOf` resolves like
dataview sources (unique keys, type ids, relation ids → `type In` /
`NotEmpty`, OR-combined); an EMPTY or unresolvable `setOf` is an explicit
400 ("queries nothing"), never an unscoped full-space query. A collection
without stored-view sorts reads in store-slice order: the matching
members are fetched in one `id In` query and reordered to the slice
in memory (honest `total` = matching members; dangling ids drop out); a
stored view's sorts override membership order via the store-side path.
`?view=` resolves by exact id or unique suffix (the C4 leniency); the
0/2+ errors list the view ids / steer to the full id. `?fields=` is
validated like search's `/fields` (rule 1 over the space's property keys +
the system allowlist, 400 + did-you-mean at path `fields`) — a typoed key
must never degrade to rows that silently carry no properties. Placeholder
substitution runs on the per-read snapshot copy (never live state):
`_filter_template_2_` → `domain.NewParticipantId(space, account)` — the
account identity rides `V2Deps.AccountId`, probed from the account
component (the `apicore.AccountService` port stays GetInfo-only); an
unresolvable placeholder (unknown index, or a missing account identity)
DROPS its leaf and warns — evaluated literally it would match nothing,
which is v1's silent-empty-result bug. Groups whose children all drop are
dropped. The views read renders the live dataview block through
`MarshalBlockSubtree` (no compaction — a fragment has no refs legend), so
views come back in the §6.2 vocabulary with option names resolved.

### 8.5 Phase-4 review fixes (decisions as built)

Changes landed after the four-lens Phase-4 review; everything here is
agent-visible surface or a resource bound.

**Parser bounds and lexing** (`filterstring`). The input is capped at
4096 bytes (the schema's advertised `maxLength`) and parenthesis nesting
at 32 (the §4 document bound) — before these, a paren-bomb `filter`
overflowed the goroutine stack, a runtime FATAL that gin.Recovery cannot
catch. The lexer decodes full runes: non-ASCII property keys (`café`,
`дата`) are ordinary identifiers, and "unexpected character" names the
rune the caller wrote, never a stray byte. Date presets are rejected on
the conditions the engine's `transformDateFilter` would silently drop
(everything but `= > < >= <=` — notably `!=`), addressed at the preset
name token; the counting presets bound their operand to `[0, 36500]`.
Unterminated-string errors echo at most 32 runes. New steering: a single
quote hints the double-quote form; a reserved-word key and a known key
the syntax cannot spell (hyphen/space/keyword collision) both steer to
the structured `filters` array. Option-name validation SKIPS when the
store cannot list a property's options (`propertyOptionNames` reports
ok=false) instead of asserting "no such option" about unread data.

**Structured-form date values.** `validateStructuredFilters` rejects a
string value on a date-formatted property, path-addressed at
`/filters/i/value`, spelling out the conversion (`the structured form
takes unix seconds (1785542400), not "2026-08-01"`) and steering to the
filter string / a `datePreset`. Before, the string survived to the store,
compared string-against-int64 and silently matched nothing (`less`
inverted through the quick-option transform matched everything) — the
exact rule-3 hazard, on the form the parser could not protect. A
convergence test now asserts both request forms compile to the identical
filter tree for dates, presets, set literals, booleans and the `type`
pseudo-key.

**Full-text pagination.** The full-text store query carries
`Limit: offset+limit+1` so the engine's candidate-budget escalation sees
the requested page — with `Limit: 0` the budget froze at the 100-doc
floor: `total` capped near 100 and the page after ~page 4 came back empty
with `has_more: false`, silently ending enumeration. The +1 record makes
the reported `total` exact when the store had fewer matches and a lower
bound (`has_more: true`) when it clipped; truncation beyond the engine's
2000-doc candidate hard limit remains the documented approximation.

**Global search bounds.** `offset` is capped at 2000 (400 with steering
to the space-scoped search — the merge materializes offset+limit rows
per space). An unknown `fields` entry no longer drops a space from
results and `total` (fields are display, not scope): the space is
queried and a warning notes the omitted column; filter/sort keys keep
the skip-with-warning semantics. Reference sets are computed lazily — a
bare `{"query": …}` fan-out no longer pays a full relation listing per
space. `spaceRefs` filters space views to live ones (v1 ListSpaces'
status predicate), so a removing/deleted space no longer gets an index
minted as a search side effect. The search handlers cap the body at
1 MiB (413 `request_too_large`) — the search routes carry no idempotency
middleware, so nothing else bounded the read.

**Sort granularity.** A date-formatted user sort that omits
`includeTime` defaults to second granularity (matching the default
`lastModifiedDate` sort), so ordering no longer changes with the
presence of the full-text tiebreak (the store's `isSingleDateSort`
compensation only fired on single-entry sort lists). An explicit
`includeTime: false` is honored.

**Sets/collections.** `?fields=` is validated (see §8.4). The collection
membership reorder is O(n log n) with one details read per record (the
insertion sort was O(n²) over the whole membership). POST /sets answers
a `type` filter leaf — which discovery's shared grammar example invites —
with a targeted 400 (`a set is already scoped to type "chore" — drop the
type filter`) instead of "unknown property key". Issue-path convention,
now uniform: JSON pointers (`/filters/0/value`) address the request
BODY; bare names (`view`, `fields`, `offset`, `dry_run`) address query
parameters.

**Warnings semantics.** Response `warnings` are advisory — they never
require a retry. The unguarded-date warning is suppressed when an OR
sibling carries `IS EMPTY` on the same property (the filter's own text
declares the empties intended), so the canonical worked example no
longer warns on every execution.

**Discovery.** The `search` and `set` kinds no longer embed the
recursive structured `filters` array (an array without `items` breaks
every constrained decoder — the C13 exception would otherwise have
swallowed the whole kind); their `filter` string description points at
kind `filters` for the escape hatch, which the endpoints still accept.
The `filters` kind documents that date values are unix seconds. The EBNF
defines `identifier`/`number` and states keyword case-insensitivity
in-grammar; a test pins every parser-accepted token to the served text.

### 8.6 Phase-5 implementation notes (decisions as built)

Phase 5 shipped the §7 task-tool wrapper: one tool table in
`core/api/wrapper` delivered as the CLI verb-set (`cmd/anytype`) and the
machine-readable function-calling manifest — plus the three server
primitives it rides (the markdown payload, the create fold, `/members/me`).

**Packaging (judged, recorded).** The manifest is Go data
(`wrapper.Tools()` — name, description, typed args, one C12 example), not
a standalone JSON file: it references op names, routes and error texts
that must move in lockstep with the handlers, and the CLI imports the
table so verb set == tool set *by construction* (a test asserts the
executors map and the table agree; another asserts every example
validates against its own args). `wrapper.BuildManifest()` renders the
JSON delivery: per tool `{name, description, parameters (strict C13
schema), example, gbnf}` + the filter grammar artifact; `anytype tools`
prints it. The tools call the API **over localhost HTTP**, not in-process:
the CLI is out-of-process by nature, and HTTP keeps one enforcement point
— auth, write rate limit, and crucially the C8 idempotency store live in
server middleware, which in-process service calls would silently bypass.
Two surfaces, not three: the wrapper stays a client of `/v2`. An MCP
server binary was NOT built (open §3 item); a long-lived host constructs
the same Runner with the in-memory session store.

**The markdown channel (server-side, as §7.1 decided).** The parser is
`anyblockjson.ParseMarkdownBlocks` — in the format package root, the
block-level sibling of the §8 inline codec: it slices markdown into a §4
flat run and passes inline text through VERBATIM as §8 markup source, so
authoring and reading stay on one dialect (goldmark/anymark was rejected
for exactly that reason — its inline semantics are not §8's). It never
fails: unknown constructs degrade to paragraphs, over-deep indents clamp
by the §4 lenient rule — AND by the two containment rules the +1 clamp
alone would break (post-review fixes): a line after a §5 leaf block
(divider, table) stays its sibling, and the F4 depth bound of 32 caps
every level — so a run always imports (tested through `UnmarshalBlocks`
over every block type the parser can emit, plus a fuzz target).
`insertBlocks` gained the `markdown` payload (mutually exclusive with
`blocks`, same targeting incl. root-append; the op schema's `required`
dropped to `op` with exactly-one enforced server-side, like the targeting
exclusivity). The parsed run is CAPPED at 256 blocks per op — the blocks
channel's own maxItems, shared so the byte-bounded markdown channel
cannot smuggle ~350k blocks per MiB — and at 2048 on the create shortcut;
the bounded parse stops early, and the error names the limit.
`createdBlocks` keys read `ops[i].markdown[j]` — j = the parsed position,
the honest analogue of the blocks[j] payload position. The create
shortcut folds parsed markdown into the create snapshot: ONE change set;
the §7.2 caveats paragraph is historical. Create-shortcut validation
issues arising from markdown-derived blocks readdress `/blocks/<j>` to
`/markdown[<j>]` (the caller never wrote a blocks array — C6), and
whitespace-only markdown on create is the same `markdown produced no
blocks` error the op path gives, not a silent empty-object 200. Scope
bounds (deterministic over clever, in the parser's file comment): ATX
headings only (`---` is always a divider), one quote level,
2-spaces-or-tab list nesting, tables need the separator row (a closing
fence marker tolerates ≤3 leading spaces; fence info strings are
constrained to a language-ish token), no image→file-block mapping (file
ids come from POST /files).

**The reference channel.** `find` numbers rows 1..N and persists
`{space, handles}`; every find renumbers (§7.4) and prunes labels of
objects no longer referenced. Full `read` relabels 24-hex block ids —
and table ROW and COLUMN ids, the same bson-hex shape `set_cell` takes
(post-review; uniqueness is computed over the whole pool) — to
shortest-unique-SUFFIX labels (min 5 chars — the same uniqueness rule as
the server's `matchBlockRef`, pinned to it by test, so labels pass
through writes even unresolved) by textual replacement over the
canonical document (key order survives), retaining label→full-id per
object. Writes resolve labels client-side when retained; the **ambiguity
retry** re-reads and retries ONCE with the SAME Idempotency-Key when the
ref resolves uniquely against the current document — §7.4(d) states its
honest scope (the concurrent-modification race); a persistent ambiguity
surfaces the server's error untouched. Every mutation receipt names its
resolved target (`ok — "Groceries": 1 changed`), so a find that
renumbered handles between composing and running a call is visible in
the transcript. Handle state lives in a session file for the CLI
(`os.UserCacheDir()/anytype-cli/session.json`, `ANYTYPE_CLI_SESSION`
overrides; corrupt files start fresh, never brick; saves are atomic via
temp-file rename) and in memory for long-lived hosts — the `Store`
interface is the seam; `Run` serializes on a Runner mutex and
`MemoryStore` hands out deep copies, so concurrent tool calls in a
long-lived host cannot race the session maps.

**Idempotency (the §7.3 machinery, placement decided; identity corrected
post-review).** Every mutation mints a random key; transport errors and
429/502/503/504 resend the exact body with the same key (max 3 attempts,
1s/2s backoff — the server's write budget is 1 req/s — and an exhausted
retryable status surfaces the server's LAST error body, not a bare
status). The reuse identity is the RESOLVED request — sha256 over method
+ path + encoded query + marshalled body, the server's own C8 identity —
NOT the tool name + raw args (the as-first-built form: it made a dry run
and its real twin, or one call re-addressed by a re-find, share a key,
which the C8 store answers with 409 `idempotency_conflict`). An identical
resolved request repeated within 60s reuses the previous key
(`Session.LastWrite`) — a regenerated-retry or a harness re-run after a
timeout OR FAILURE (the session, key included, is saved on the error path
too), and C8 replays it; after the window an identical request is
presumed intentional and applies fresh. A successful ambiguity retry
re-stamps the key onto the rewritten request so a later
client-side-resolved re-run still replays. A pure body-hash key (the
review's sketch) was rejected: it would make every intentional repeat
replay forever. The task tools never send If-Match (C7 advisory — sync
noise 409s a small model cannot answer); the CLI exposes `--if-match`
for scripts.

**Conveniences, as placed by §7.3.** `@me` resolves through the new
`GET /v2/spaces/{spaceId}/members/me` (participant id is deterministic —
served even before the participant object is indexed; no account identity
→ 404 steering to the members list), cached per space in the session; in
`find` filters the quoted `"@me"` value substitutes textually, and in
property VALUES it substitutes on object-FORMAT keys only (post-review —
a description literally containing "@me" is data, not an identity).
Relative dates resolve wrapper-side on date-FORMAT keys only (the wrapper
loads the space's property formats ONCE per tool call — set+add+remove
share the fetch): `today`/`tomorrow`/`yesterday`, weekday names (next
occurrence, today included), `±Nd` — to RFC 3339 local midnight; anything
else passes through literally for the server to judge. The **A2 option
guard** pre-validates select/multiSelect names in `create`/
`set_properties` `set`+`add` (never `remove` — the op cannot create)
against the live options with a did-you-mean; `--create-missing` is the
deliberate CLI escape to the REST R9 semantics. `describe` marks a FAILED
option listing per property (`optionsUnavailable`, rendered as "could not
be listed — run describe again") instead of showing an optionless select
that invites an invented name.

**Tool-set deviations from the §7.2 table (recorded there too).**
`spaces` was added post-review as the 12th tool (bootstrap: nothing else
could produce a space id; still under the 15-tool cliff). `set_cell`
takes `object` — the REST op addresses a table within one object; a bare
table handle would need hidden cross-object state. The wrapper's `under`
maps to the ops' `inside` (+`position: last`), and server error texts
are translated BACK to the tool vocabulary before the model sees them
(`inside`→`under`, `id`→`block`, `tableId`→`table`, the `ops[0].` prefix
stripped, `?outline=true` hints become `read mode=outline`) — without
this the server's own repair hint names a field the tool rejects.
`edit_text` deliberately has no `replace_all`; its `replace` and
`set_cell`'s `value` are required-but-may-be-EMPTY (`AllowEmpty`:
deleting a phrase and clearing a cell are first-tier intents; empty
`value` sends the op's documented null-clears), and the wrapper's error
text distinguishes a missing arg from an empty one. `describe` takes
`space, type` (every backing GET is space-scoped) and ships in the
§2-sanctioned degraded form: `GET /types/{type}` + live option lists,
composed wrapper-side (25 options per property, truncation marked); the
`GenerateSchema` §3 item stays open and collapses this to one GET when it
lands. `find` has no `fields` arg (C5 minimal rows only) and search
carries no idempotency key (a read). The wrapper's route templates are
exported (`RouteTemplates`) and asserted against the gin router in the
server suite, so a renamed /v2 route fails loudly instead of 404ing every
tool in production.

**GBNF (§7.4, kept honest by test).** Per-tool grammars are GENERATED
from the Arg table: the argument object with required args first in
declared order, then each optional arg as an independently omittable
`("," pair)?` group — a pinned key order, which is what constrained
decoding wants (an optional-only tool like `spaces` emits a nested
optional chain so no comma dangles). The served C12 example is
pre-rendered JSON in that SAME order (post-review: a Go map serialized
alphabetically, so 9 of 11 examples were not in the language of the
grammar shipped beside them). The filter-string GBNF is transcribed from
the pinned `filterstring.EBNF`, constraining to the canonical surface
(uppercase keywords, camelCase presets, ASCII keys; the parser stays
lenient); its leaf REQUIRES whitespace between a key and a word-led
condition (post-review — `titleEXISTS` was grammar-legal but
parser-illegal; `key` still admits reserved words, a documented GBNF
limitation). It is served as a SEPARATE artifact beside find's grammar:
composing a DSL into a JSON-string production would require re-escaping
every DSL quote through the JSON encoding — a transformation GBNF cannot
express; the seam is documented on the artifact. A GBNF well-formedness
checker (rule syntax, terminated literals/classes, balanced groups, no
undefined references, root present) runs over every served grammar in
tests, over broken grammars to prove it catches breakage — and a
test-only backtracking GBNF MATCHER asserts every served example against
its own served grammar and the filter grammar against
`filterstring.Parse` (examples in, the pre-fix false positives out).

**SKILL.md** lives at `cmd/anytype/SKILL.md` (frontmatter description →
body → references three-tier): the spaces→find→describe→read loop, the
E4 intent→verb recipes (complete-a-task steers to `set-properties`, NOT
`check-item`; delete-a-phrase and clear-a-cell via the empty-string
forms), filter-string examples, and the caveats (D′1 markup source,
options-never-created, handle renumbering + receipts naming the target,
no batches, safe retries incl. after failure). B4 tunes this text per
tier once the benchmark runs.

### 8.7 Phase-6 implementation notes (chats — decisions as built)

Phase 6 gave chats their /v2 home (the completeness decision, 2026-08-06:
a v2 client never types /v1 for its task loop — but never the same shape
at a new URL). The phase's motivating finding held under verification:
`ChatGetMessages` returns `chatState` and `messageCount` and the v1
service throws both away (`service/chat.go:100` reads only
`resp.Messages`), and NO v1 response carries a state id (the DTO omits
it; the SSE converter has no ChatStateUpdate case,
`model/chat.go:279-314`) — so the `ReadChatMessagesRequest.lastStateId`
race guard was unreachable by construction. v2 passes both through and
`POST read` forwards the guard.

**Endpoints** (`server/router.go registerV2ChatRoutes`; handlers
`v2/handler/chat.go`, service `v2/service/chat.go`, DTOs
`v2/model/chat.go`):

```
GET    /v2/spaces/{spaceId}/chats                                  # C5 rows {id,name}
POST   /v2/spaces/{spaceId}/chats                                  # {name} → row
GET    /v2/spaces/{spaceId}/chats/{chatId}/messages                # ?after&before&limit&reactions
POST   /v2/spaces/{spaceId}/chats/{chatId}/messages                # {text, replyTo?, attachments?} → {id}
PATCH  /v2/spaces/{spaceId}/chats/{chatId}/messages/{messageId}    # {text} — text-only merge
DELETE /v2/spaces/{spaceId}/chats/{chatId}/messages/{messageId}
POST   /v2/spaces/{spaceId}/chats/{chatId}/messages/{messageId}/reactions  # {emoji} → {added}
POST   /v2/spaces/{spaceId}/chats/{chatId}/read                    # {upTo, lastStateId, scope?}
```

**The three reshapes, as built.** (1) *State passthrough*: the messages
read returns `{messages, state, messageCount, has_more, nextAfter?,
nextBefore?}`; `state` carries
`unreadMessages/unreadMentions/oldestUnreadOrder/oldestUnreadMentionOrder/
unreadReactionOrder/lastStateId`; `messageCount` is the chat's LIFETIME
total, not the range size — `has_more` (fetched via limit+1, the
ListChats pattern) plus the boundary cursors are the range signal, so an
agent never infers "more" from `len==limit` (C10's spirit). A poll is a
`limit=1` read; the exit flow "summarize what's new and mark it read" is
two calls (GET messages → POST read `{upTo, lastStateId}`). (2) *Inline
markup both directions*: read renders `text` via
`anyblockjson.RenderInlineText` (mentions as `<mention objectId="…">`
tags); write parses via `ParseInlineText` — the D′1 caveat is documented
on both write endpoints; offset mark arrays never cross the API. `style`
is dropped on read and not accepted on write (a fresh message is always
`paragraph`; an edit preserves the stored style). Block-composed content
(desktop quotes, rich pastes — a message may be VALID with empty text
and only blocks, `chatmodel.Validate`) surfaces read-only as
`blocksText`: text-bearing blocks rendered as §8 markup, newline-joined;
link/embed blocks stay invisible (recorded deferral). (3) *C5 rows +
compact reactions*: chat rows are `{id, name}` (Q3 as recommended —
counter-free list; computing list-wide counters means opening every
chat, the GO-7302 cost; `ListChats` is a pure store query over
`ChatLayouts` and its test would fail on ANY RPC call). Reactions are
ALWAYS the counts map `{"👍":2}` (Q4 as recommended);
`?reactions=full` adds `reactedBy` — **participant-id** lists (one
vocabulary with `authorId`, C2), never raw identities — in its OWN slot,
so neither field ever changes type (C2: the review killed the original
polymorphic `reactions` slot, which no strict response schema could
describe).

**C7 exemption, stated.** No chat response carries an etag and no chat
mutation reads If-Match: order ids and `lastStateId` are the chat's
native concurrency vocabulary. Documented on every chat endpoint (the
deliberate exemption class search's C8/C9 one established).

**C8 widened to DELETE (additive contract change, recorded loudly).**
The surfaces doc demands idempotency on *every* chat mutation; C8's
method set was POST/PATCH/PUT. `ensureIdempotency` now also acts on
DELETE — and after the Phase-6 review, EVERY registered v2 DELETE
carries the middleware: chat message, type and property alike. (As
first built, only the chat delete was keyed; the review called the
route-dependent behavior an invisible contract — an agent that
dutifully keys every mutation, as the Phase-5 wrapper does, must not
get replay on one DELETE and silently none on another. The router test
pins all three.) A blindly retried v2 delete used to 404 misleadingly;
now it replays.

**Decisions the spec left open.**

- **`upTo` AND `lastStateId` are BOTH REQUIRED for scopes
  messages/mentions.** The RPC's range query bounds with
  `orderId <= beforeOrderId` AND `stateId <= lastStateId`
  (`chatrepository/repository.go:387-392`), and every stored message
  carries a non-empty bson state id (`chathandler.go:116`) — so an
  empty value for EITHER selects nothing and returns success, the
  silent no-op trap (v1's `read_all` path sends exactly that shape; the
  phase's first build required only `upTo` and the review reproduced
  markedCount=0 with a 200 when `lastStateId` was omitted — the same
  trap one field over). Both values ride the same GET messages response
  (the newest order + `state.lastStateId`), so requiring both costs the
  agent nothing; the missing-field 400 is path-addressed to each.
  Server-side filling was REJECTED: a guard resolved at POST time is
  newer than the client's read and would mark late-arriving unseen
  messages as read — silently weakening the exact race the guard
  exists to close. There is deliberately no "mark all" form. (The RPC
  gives v2 no marked count to return — `RpcChatReadMessagesResponse` is
  empty and `chats.Service.ReadMessages` discards it — so a
  zero-effect read still cannot be surfaced in the response; requiring
  both bounds closes every known silent path instead.)
- **The reactions scope is all-or-nothing.** `ChatReadReactions`
  *ignores* its `orderId` (`core/chats.go:325` calls
  `ReadReaction(ctx, chatObjectId)` without it), so the planned
  "`upTo` → read reactions" forwarding is impossible today; scope
  `reactions` rejects `upTo`/`lastStateId` with path-addressed 400s
  rather than pretending a bound exists.
- **Chat RPC failures are classified in v2 — the RPC codes are dead.**
  `core/chats.go` maps no chat errors (`mapErrorCode` with no
  `errToCode` entries returns UNKNOWN_ERROR for everything), so the
  BAD_INPUT→400 branch never fires and, as first built, every caller
  mistake was a retry-looping 500. Three layers now close it: (a) v2
  pre-validates what it owns — parsed text length against
  `chatmodel.MaxMessageLength` (8000 UTF-16 units) as a path-addressed
  400 on `/text`, and the attachment cap (32, the schema's maxItems) on
  `/attachments`; (b) both DELETE and the reaction toggle run the
  existence read on the COMMITTING path too, so a missing message 404s
  identically to its dry run (the store handler treats deleting a
  missing doc as success — without the check the real DELETE answered
  200 for a deletion that never happened, C9 broken); (c)
  `v2ChatRpcError` classifies the remaining RPC descriptions:
  `validate: …` → 400 validation_failed, `not found` → 404, and the two
  foreign-message refusals → **403 `forbidden`** (a NEW C6 code,
  additive — C6's list is non-exhaustive "include"). The foreign-message
  arms match through the producers' exported sentinels
  (`chatobject.ErrModifyForeignMessage` — the EDIT wording,
  "can't modify someone else's message" — and
  `chatobject.ErrDeleteForeignMessage`, "can't delete not own message"),
  so a middleware rewording updates the classifier at compile time; as
  first shipped only the delete prose was matched and a foreign EDIT
  fell through to a retry-looping 500 (surface review M2b, fixed with a
  test that feeds the string the edit path really produces). Anything
  else stays 500 with the description carried.
- **PATCH message is a read-merge.** The middleware edit replaces the
  whole message content — chatmodel `content` =
  `{message, attachments, blocks}` (`chatmodel.go:441-444`) — so a naive
  `{text}`-only forward would WIPE attachments on every text edit. The
  service reads the message first and carries style, attachments and
  blocks through unchanged (also giving edit/delete dry runs their
  existence check for free). Markup caveat recorded: an Emoji MARK is
  materialized into its literal emoji on read
  (`RenderInlineText`/materializeEmoji), and the re-parse does not
  re-mint it — a read→PATCH round trip turns the mark into plain emoji
  text (visually identical, mark gone). Same D′1 family as the marks
  re-derivation; pinned by the non-BMP round-trip test.
- **Attachments are bare object ids** (the surfaces-doc shape), at most
  **32 per message** — enforced before any store lookup, so the strict
  schema's maxItems is true (unbounded, each id was one synchronous
  index lookup and one permanently replicated CRDT entry); the
  attachment kind is inferred from the target's layout — `image` →
  image, other file layouts → file, anything else → link. An unknown id
  is a path-addressed 400 steering to POST /v2/files (attaching a
  nonexistent object would send a broken message; the indexing race is
  an actionable retry, not a silent default).
- **DELETE names its irreversible side effect.** The middleware
  permanently deletes (skipBin — no bin) attachment/link targets
  orphaned by the delete, asynchronously, AFTER the API replies
  (`chats/service.go ArchiveOrphansOnLinksRemoval`). Both the dry run
  and the real receipt carry C6 warnings naming the attachment ids at
  risk, and the endpoint description says so — this is also where the
  previously dead `V2ChatMessageResult.Warnings` field earns its keep.
- **Author enrichment is store-backed, not the v1 cache.** The plan said
  "reusing the participant cache" — that cache is the v1 service's
  cross-space *subscription* cache, which V2Service deliberately does
  not own. Participant objects are indexed under their deterministic ids
  (`domain.NewParticipantId`), so the v2 service resolves names straight
  from the space index (memoized per read); an unindexed participant
  degrades to an empty name exactly like a v1 cache miss.
- **`ensureChat` guard.** Every chat-scoped route first resolves the id
  in the store: unknown → 404 steering to GET /chats; a non-chat layout
  → targeted 400 (the sets/collections wrong-layout precedent) — the
  chat RPCs' own failure for a bad target is opaque.
- **Message DTO shape**: `{id, order, author?, authorId?, at, editedAt?,
  text, blocksText?, replyTo?, reactions?, reactedBy?, attachments?,
  pinned?}` — `editedAt` only when the message was edited; sync/read
  flags deliberately absent (agent noise). `at`/`editedAt` are
  **RFC 3339 UTC strings** — the review fixed the original unix-epoch
  ints: one date shape across v2 (AnyBlock dates, search filters, file
  addedAt all render RFC 3339), and Phase 8's SSE stream reuses these
  DTOs. Cursor pagination only: `?offset=` on the messages read is a
  400 steering to the cursors (a silently honored offset would fake
  offset paging the RPC does not do). Cursor asymmetry documented on
  the endpoint: only `?after` alone walks forward (the repository's one
  ASC sort); `?before`, no cursor, or BOTH bounds anchor at the newest
  end and page backward via `nextBefore` — after+before does NOT
  advance a forward cursor through the window.
- **Reaction dry run needs an identity.** `V2Service.accountId` may be
  empty (documented degraded mode); with no identity nothing matches
  the stored reactions and the predicted `added` would be wrong
  whenever the caller already reacted — so `added` became `*bool`,
  omitted with a C6 warning in that case (mirroring the
  `v2_discovery.go` guard) instead of asserting a coin flip.
- **POST /chats requires a non-empty name** (the row is `{id,name}`; an
  unnamed chat is unaddressable) and is a thin `ObjectCreate` with the
  `chatDerived` type — NOT the Phase-2 snapshot path, which has never
  been exercised for store-backed smartblocks.
- **Dry runs** (C9): create/message/read validate everything and send
  nothing; edit/delete stop after the existence check; the reactions
  toggle reads the message and reports the would-be `added` from the
  caller's current reaction.

**Discovery (§5).** Five kinds — `chat`, `chatMessage`,
`chatMessageEdit`, `chatReaction`, `chatRead` — strict (C13), each with
a worked example asserted against its own schema by test (the review
added the edit/reaction kinds: the handlers decode strictly, so a
guessed field name was an avoidable 400). `chatMessage` is the
authoring surface (markup-source text, bare-id attachments); its
`text.maxLength` is **8000** — `chatmodel.MaxMessageLength`, UTF-16
code units — pinned to the constant by a drift test after the original
build advertised 65536 (8× the store cap, steering schema-obedient
models into rejections).

**Deliberately deferred / reused from v1.** The SSE stream stays on v1
this phase — Phase 8 mounts it under /v2 carrying these DTOs (they were
built for reuse: the converter is `apimodel.V2ChatMessageFromProto` +
`V2ChatStateFromProto`, and a v2 stream must ALSO forward
ChatStateUpdate, closing the converter gap this phase documented).
Per-chat FT search (`…/messages/search`) stays on v1 (named exception).
Not ported: v1's GET single message (not in the surface — PATCH/DELETE
address by id, reads page by cursor), editing attachments on PATCH
(v1 keeps that), message pinning writes, and `read_all` (see `upTo`).
Link/embed message blocks have no `blocksText` rendering (only
text-bearing blocks appear); a full `blocks` array is deferred until a
consumer needs it. Open (needs a real account to verify): the
space-level chat is created name-less (`AddChatDerivedObject` sets only
a uniqueKey), so it may render as `{id, name:""}` in the C5 list — a
space-name fallback or `isSpaceChat` flag is the candidate fix once
runtime-verified, incl. whether the object is indexed unhidden.

**Tests that pin the phase** (each fails if its behavior reverts):
state+messageCount passthrough incl. `lastStateId`
(`v2/service/chat_test.go`), the markup bridge both directions + the
round-trip (`v2/model/chat_test.go`), reactions counts/full as
participant ids, the no-chat-opens list (any RPC fails the mock), the
edit merge preserving attachments, `upTo` required / reactions-scope
bounds rejected, lastStateId forwarding on POST read, C8 wiring on all
six chat mutations incl. the DELETE replay
(`server/v2_router_test.go`, `api/v2/middleware_test.go`), and the
chat discovery kinds' strictness (`v2/service/schemas_test.go`).

### 8.8 Phase-7 implementation notes (periphery — decisions as built)

Phase 7 closed the periphery (APIV2_SURFACES.md §8 Phase 7): the space
surface, and the query surface's file-layout blindness — the latter a
live bug in shipped Phase-4 code, fixed first. The §8 item 2
(`GET /members/me`) was verified ALREADY SHIPPED by Phase 5 (route
`router.go`, service `v2_discovery.go GetMemberMe`, tests
`v2_discovery_test.go`) and was not rebuilt.

**The file-layout opt-in (the live bug, APIV2_SURFACES.md §4).** The
evidence held: v2 search's base row scope and ListObjects both pin
`util.ObjectLayouts`, which contains no file layout, while v1 search has
`prepareBaseFilters(includeFileLayouts)` — so a pure-v2 agent could
upload a file (POST /files) and never find it again. As built:

- **Trigger = the type channel, both request forms.** A top-level `type`
  naming a file type key (`file`, `image`, `video`, `audio` —
  `util.IsFileTypeKey`, v1's `fileTypeUniqueKeySet` vocabulary; there is
  no `pdf` type key — pdf is a *layout* of type `file`, and the widened
  scope includes it) or a **positive** `type` filter leaf sets the plan's
  `includeFileLayouts`, which switches the base row scope to
  `util.ObjectAndFileLayouts` for that query. The leaf detection sits
  in `resolveTypeLeaves` — AFTER the two filter forms converge on one
  tree — so the string and structured forms behave identically. A mixed
  `type IN ("task","image")` widens; a **negated** leaf (`!=`, `NOT IN`,
  `notAllIn`, `notExactIn`, `notContains` — `negatedFilterConditions`)
  does NOT (excluding a type is not asking for files — pinned by a test
  with a second file object the negation must not leak in). *Positive* is
  decided by exclusion from that negated family, NOT by an `=`/`IN`
  allowlist: the review reproduced `allIn` (the compact string's
  `HAS ALL`) silently returning zero file rows under the original
  allowlist — the exact upload-then-never-find-it bug this opt-in exists
  to kill, for an advertised condition. Two scope consequences, decided
  and pinned: the widening is **query-global** (plan-level), so a
  positive file-type leaf under `OR` widens the other arms' row scope
  too — the caller named a file type, and per-branch scoping would make
  the row scope depend on tree shape; and the opt-in trigger stays the
  type channel ONLY — a filter on file metadata alone (`size > 5`)
  does not widen, so it matches nothing until composed with a file type
  (`type = "image" AND size > 5`).
- **Scope: the search surface only** (space-scoped and global — one plan
  builder). ListObjects has NO type channel and deliberately gains none
  (§4's "the v1 opt-in reproduced *without a new parameter*"); file
  discovery is search's job. The sets/collections reads never had the
  layout scope at all (their filters are the setOf/membership
  translation — verified `listObjects` in `v2_list_read.go`), so a set
  over a file type already returned its rows; nothing to widen there.
- **The file vocabulary: `mimeType` and `size`, live in EVERY channel.**
  Aliases mapped to the backing store relations
  `fileMimeType`/`sizeInBytes` (`v2FieldAliases`, `v2_object.go`). The
  names are the format's OWN file vocabulary — the SPEC §5 file-block
  fields and the POST /files result. As hardened by the Phase-7 review
  (two findings, both reproduced):
  - **Resolution is per SPACE, never per row** (`activeFieldAliases`):
    an alias is active only when no real property of the space claims
    its key. A user relation literally keyed `size` deactivates the
    alias for the whole result set — the original per-record fallback
    reported a file's byte count under the user's short-text `size`
    property on rows that lacked a value, one key meaning two things in
    one result set.
  - **An active alias works in `fields=`, filters AND sorts** —
    translated to the backing relation in the plan (`rewriteAliasLeaves`,
    the sort rewrite, `aliasedFormatName`). The original display-only
    scoping made the one advertised spelling a dead end outside
    `fields=` while the store spellings worked, splitting the vocabulary
    by channel. Filtering composes with the type-channel opt-in above:
    `size > 5` alone stays in the file-less base scope.
  - **Honesty note on C2**: the store names `fileMimeType`/`sizeInBytes`
    remain live property keys wherever they are real keys of the space
    (bundled relations, type recommendations) — rejecting a genuine,
    discovery-advertised key would be worse than the duality. The alias
    is the canonical, schema-advertised spelling; the store names are
    ordinary properties, not part of the advertised file vocabulary.
    (The earlier §8.8 claim that the aliases *prevent* two names on one
    concept was wrong — the review reproduced both spellings rendering
    in one row when both are requested.)

**The space surface (APIV2_SURFACES.md §2 shapes).**

```
GET   /v2/spaces/{spaceId}     → {"id","name","description"}
POST  /v2/spaces               {"name","description"?} → the same shape (201)
PATCH /v2/spaces/{spaceId}     {"name"?,"description"?} → the same shape
```

(`v2/handler/space.go`, `v2/service/space.go`, DTOs in `v2/model/model.go`;
routes beside the spaces list in `router.go`.) `gatewayUrl`/`networkId`
stay v1-only (client infrastructure, not agent fields). No space delete
(v1 has none; deletion is an account-level operation). The spaces LIST
row gained `description` in the review hardening — it sits in the same
tech-space record for free, and withholding it forced a GET-one per
space on the canonical "list my spaces, pick one" trace (a 1+N pushed
onto the agent).

- **The read is one tech-space store query, zero RPCs.** The §2 claim
  verified: v1's `getSpaceInfo` opens `WorkspaceOpen` + `ObjectShow` per
  call — and v1's *list* does that per row (`service/space.go:88-94`,
  `212-250`), the N+1 by construction. The space view mirrors the
  workspace object's `name` AND `description` (`workspaceKeysToCopy`,
  `core/block/editor/spaceview.go`), so `GetSpaceViewDetails` serves the
  whole v2 shape. Consequence, recorded: the row is as fresh as the
  async workspace→spaceview sync, the same freshness the shipped v2
  spaces LIST already has.
- **Create is ONE WorkspaceCreate call.** `CreateWorkspace` applies
  every detail to the workspace object (`core/block/create.go`), so the
  description rides the create request — v1's second `WorkspaceSetInfo`
  RPC for it is dropped. Everything else is v1 parity: `CHAT_SPACE` use
  case, random icon option, regular space type, widgets homepage,
  trimmed strings. A success carrying no space id is answered 500, not
  an id-less 201 (C8 promises created ids; a cached id-less 201 would
  replay forever, while a 500 is not cached and a keyed retry
  re-executes).
- **`name`/`description` are capped at 4096 characters** (code points),
  the bound the `space` discovery kind advertises — enforced on POST and
  PATCH with a path-addressed 400 (`validateSpaceField`; a drift test
  pins the schema's maxLength to the enforced constant). Before the
  review the schema out-promised the endpoint: a 200,000-character name
  was accepted and propagated to every member's device. The other kinds'
  advertised maxLengths remain unenforced where the store itself bounds
  them (chats) or nothing does — hardening them is follow-up work, not
  silently claimed here.
- **A recorded C8 window, not closed**: `CreateWorkspace` creates the
  space first and can still fail later (set-details, use-case import);
  the RPC then discards the space id (`core/workspace.go`) and v2
  answers 500 — which is deliberately NOT cached, so a keyed retry
  re-executes and can create a SECOND space. Caching 5xx would make
  transient failures permanent (worse), and surfacing the orphan id on
  error needs a middleware change that could misreport a
  partially-initialized space as created. Accepted as the lesser evil;
  the failure mode is rare (the create path is local).
- **C8 on both mutations** via the route idempotency middleware — the §2
  finding was the motivation: an auto-retried space create with no key
  duplicates an *entire space*, the worst possible duplicate. The
  router test pins both registrations. **C9 scoped honestly**: the
  create dry run validates the body only and says so (a space create
  cannot be simulated); the PATCH dry run reports the would-be row.
- **PATCH contract.** At least one of `name`/`description` (the
  setProperties empty-op precedent — an accepted `{}` would let an agent
  believe it renamed something); `name` present-but-empty is rejected
  (the POST /chats precedent: the C5 row is `{id, name}`), while
  `description: ""` clears. Unknown space 404s before body validation.
  The response overlays the patch onto the current view row instead of
  re-reading (the async view sync would race an immediate read-back).
- **Status predicate, re-decided in the review hardening.** GET-one and
  the spaces LIST serve **live spaces only** — `isLiveSpaceView`, v1's
  two-axis predicate (local status Unknown/Ok AND account status
  Unknown/SpaceActive), now shared with the global-search fan-out
  (`spaceRefs`) so the three surfaces agree on what a space *is*. The
  original as-built served any space with a view; the review showed a
  deleted space's row is indistinguishable from a live one (the shape
  has no status slot), so an agent picking it would PATCH or write into
  a space that can never load. Two recorded asymmetries: (1) the
  earlier "same contract as ensureSpace" equivalence claim was wrong
  even as written — `ensureSpace` short-circuits the TECH space, so
  space-scoped routes resolve it while GET-one 404s it (and always
  did); (2) `ensureSpace` itself stays laxer (any view + the tech
  space) — sub-routes of a just-dead space keep answering during status
  transitions rather than flapping 404, and the tech space remains an
  internal address no space row ever advertises.
- **C7 exemption, recorded**: the space surface carries no etag and
  PATCH ignores If-Match — a space view has no agent-visible tree head
  to hash (the object/chat etags hash CRDT heads), and the name/
  description pair is last-write-wins by design. Concurrent renames
  race silently; acceptable for a two-field resource.
- **Discovery: one `space` kind** (strict, `required:["name"]`). PATCH
  takes the same two fields (both optional, at least one) and is
  documented on the kind's endpoint string rather than minting a
  `spaceUpdate` kind — the chat precedent (`chatMessageEdit`) split
  kinds because the shapes diverged; here they share every field name,
  and a strict-schema agent that includes `name` on PATCH is simply
  valid.
- **RPC error mapping**: BAD_INPUT → 400 `validation_failed` carrying
  the description; then the review hardening added the description
  classification the chat surface has (`v2SpaceRpcError`) — the
  workspace RPCs answer UNKNOWN_ERROR for everything reachable (core
  `mapErrorCode` has no workspace mappings), so without it a PATCH
  racing a space deletion, or a reader's PATCH in a shared space, was a
  retry-looping 500. Pinned strings: `space not exists` /
  `space is deleted` / `space storage missing`
  (space/service.go sentinels) → 404 `not_found`; `restricted`
  (restriction.ErrRestricted via SetDetails) → 403 `forbidden`;
  everything else stays 500 with the description carried.

**Handler plumbing note.** The chat handlers' strict body decoder was
generalized (`decodeStrictJSONBody`, `v2/handler/error.go`) and is shared
by the space handlers; chat error texts are unchanged
(`decodeChatBody` delegates).

**Tests that pin the phase**: the RPC-free space read (any RPC fails the
mock), the single-call create carrying the description (a
`WorkspaceSetInfo` expectation would fail), the at-least-one-field and
empty-name PATCH 400s, C9 dry runs sending nothing (service AND handler
layers — a regressed `dry_run` would create a real space), C8 wiring on
POST/PATCH spaces (`server/v2_router_test.go`), the space kind's
strictness, the opt-in matrix (top-level type / string leaf / structured
leaf / mixed IN / negated leaf / bare search) where the widening test
can only pass through `ObjectAndFileLayouts`, and the fields aliases
rendering from the backing relations
(`v2/service/search_test.go TestV2SearchFileLayoutOptIn`,
`v2/service/space_test.go`, `v2/handler/space_test.go`).

**Review hardening (2026-08-06, three opus lenses)** added the pins for
everything the hardening changed: per-space alias shadowing
(`TestV2FieldAliasShadowing` — a user property keyed `size` must
deactivate the alias for the whole result set), the alias filter/sort
channels, `allIn`/`HAS ALL` widening, the query-global OR widening as a
decision, the live-space predicate on GET-one and the list (deleted
space → 404 / filtered out) plus `description` on the list row, the
workspace-RPC description classification (404/403/500 matrix), the
no-space-id 500, the 4096 caps with the schema-drift pin, the
end-to-end keyed `POST /v2/spaces` replay (`server/v2_router_test.go` —
`WorkspaceCreate` mocked `.Once()`, second response
`Idempotency-Replayed`), and the set-over-a-file-type read rendering
`?fields=mimeType,size` (`v2_list_read_test.go`). Two C2/C8 footnotes
recorded rather than changed: `dry_run` keeps its snake_case spelling
in response bodies — a deliberate, uniform C2 carve-out across all
eight v2 mutation DTOs (the echo mirrors the `?dry_run=` query
parameter it answers; renaming mid-stream would fork the dialect the
shipped phases already speak); and an Idempotency-Key reused across
`POST /v2/spaces` and `POST /v2/validate` (the two space-less routes
sharing the empty-space key namespace) answers 409
`idempotency_conflict`, which is correct — a key names one logical
operation.

### 8.9 The /v2 key-scope gate (2026-08-06 — decisions as built)

The one amendment to §8's "no new auth surface": authentication stays
shared with v1 (same bearer keys, same pairing endpoints, same
`ensureAuthenticated`), but `/v2` carries a group-level authorization
gate v1 does not — `ensureJsonApiScope`, installed on the v2 group
directly after Auth. Only keys whose scope is `JsonAPI` or `Full` may use
`/v2`; every other scope — the web clipper's `Limited`, and any future
enum member until explicitly admitted — is refused with 403, distinct
from the invalid-key 401. Legacy keys minted without a scope carry
`Limited` (the enum zero value; anytype-cli's `CreateApp` historically
sent none) and are grandfathered on `/v1`: they keep working there
exactly as they ship today and hit this 403 on every `/v2` route.
Migration stance: `docs/superpowers/specs/2026-08-06-api-key-scoping-design.md`.

- **The 403 body names the remedy**: `api key scope does not allow json
  api access: key "<appName>" has <Scope> scope, create a new api key
  with JsonAPI scope` — the failure reads as "re-issue the key", not as
  a transient permissions bug. Error text is API surface; tested
  verbatim.
- **Envelope, recorded**: the gate answers in the shared v1 envelope
  (`{object, status, code, message}`, code `forbidden`), not the C6
  shape — the same seam as the group-level auth 401, which aborts before
  any v2 route middleware runs. Group-level refusals speak the shared
  server's dialect; the C6 shape starts where v2's own middleware and
  handlers do.
- **Coverage is a test, not a convention**
  (`server/v2_router_test.go`, "every /v2 route carries the scope
  gate"): every route under `/v2` in the real engine's table must answer
  the gate's exact 403 to a cached `Limited` key, except the two public
  documents (`GET /v2/docs/openapi.{yaml,json}`) on an explicit exempt
  list — so a `/v2` route registered outside the gated group fails the
  walk instead of shipping ungated.

### 8.10 The /v2 space-grant gate (2026-08-06 — decisions as built)

Where §8.9's gate decides the key's KIND, this layer decides which spaces
and which verbs a key's *grant* covers. A key may carry a grant record
(`{spaces, perms: read|readwrite}`, sealed into the app-link file); the
grant — never the key-string format — is what enforcement reads.
`Grant == nil` is the legacy unscoped key and behaves exactly as before.
Design: `docs/superpowers/specs/2026-08-06-api-key-scoping-design.md`.

- **The gate** (`v2/authz.go ensureSpaceGrant`, installed directly after
  the key-scope gate): a `:space_id` must be in the grant's space list,
  else 403 `space_not_granted`; the tech space is denied unless
  explicitly granted (this gate runs BEFORE the service's `ensureSpace`,
  which admits the tech space as an ordinary id). A `read` grant on a
  write-classified route → 403 `write_not_granted`. Both messages NAME
  the actual grant — error-guided self-correction over enumeration
  resistance, which is a non-goal for a localhost single-user API.
- **The registry, not inference**: every route is classified in an
  explicit table (`v2RouteAuthz`) — verb (`POST /v2/search` and
  `/v2/validate` are READS; chat `POST …/read` is a WRITE, it mutates
  the synced read watermark) and, for no-`:space_id` routes, a global
  class: `auth-exempt` (public docs), `data-free-allow` (`/v2/validate`,
  `/v2/schemas*`), `service-filtered` (`GET /v2/spaces`,
  `POST /v2/search` — allowed through, constrained in the service), or
  `scoped-denied` (`POST /v2/spaces`: a key that can mint spaces it then
  owns is not meaningfully scoped). An UNREGISTERED no-space route is
  refused, fail closed; the conformance walk
  (`server/grant_gate_test.go TestV2RouteAuthzConformance`) makes a
  missing or stale classification a CI failure in both directions, pins
  the `auth-exempt` precondition behaviorally (an exempt route must
  answer without credentials, every other /v2 route must 401), and
  refuses unknown route-param names — the gate reads the addressed space
  from exactly `:space_id` (`apiv2.SpaceParam`), so a space param under
  any other name must fail CI rather than slide into a global class.
- **Fan-out + backstop**: the two service-filtered surfaces intersect
  their space set with the ctx grant at the INPUT (`spaceRefs`,
  `ListSpaces`) — not the output rows, so a per-space warning cannot
  disclose a non-granted space's existence. The service layer carries
  BOTH backstop halves, in the gate's precedence (space first, then
  verb): `ensureSpace` consults the grant before its tech-space
  admission, and the write entry points go through
  `ensureSpaceWrite`/`ensureChatWrite`, which also refuse a read-only
  grant (`ensureWriteGranted`) — so a future route that forgets the
  middleware can neither reach a non-granted space nor mutate a granted
  one with a `read` key. `GetSpace`/`CreateSpace`/`UpdateSpace`, which
  bypass `ensureSpace`, carry their own checks.
- **Envelope**: this gate is v2's own middleware, so it answers in C6
  (codes `space_not_granted` / `write_not_granted`) — unlike §8.9's
  shared-server gate. Granted keys are refused on `/v1` with C6
  `v1_not_available_for_scoped_keys` pointing at `/v2` (grant presence
  decides, never format; legacy keys stay served on `/v1`).
- **WWW-Authenticate** rides every auth failure (MCP clients are
  required to parse it): 401 → `Bearer realm="anytype"` (bare when no
  credential was sent, `error="invalid_token"` otherwise); 403 →
  `Bearer error="insufficient_scope"`, with
  `scope="space:<spaceId>:<read|readwrite>"` when the request addressed
  one space — the scope-string shape is implementation-defined
  (RFC 6750 §3.1) and this is the documented one.
- **Grant edits bite immediately**: `LinkLocalUpdateApp` evicts the
  key's cached HTTP session entries (`RevokeToken`), so an in-place
  NARROWING is enforced on the very next request
  (`TestGrantEditTakesEffectOnNextRequest`) — a stale cached grant would
  be a silent authorization bypass. The sweep can only evict entries
  that exist, so a mint IN FLIGHT during the sweep must not cache
  afterwards: the server keeps an eviction generation, snapshotted with
  the cache read and re-checked at the cache write — on a mismatch the
  minted entry serves that one request and is dropped, and the next
  request re-mints against what the wallet holds then
  (`TestGrantEditDuringMintIsNotLost`).

### 8.11 whoami + legacy-key signals (2026-08-06 — decisions as built)

The P1c introspection layer over §8.10's enforcement: an agent that cannot
read its own grant either over-requests and fails or under-requests and
does nothing useful. Design:
`docs/superpowers/specs/2026-08-06-api-key-scoping-design.md` (P1 §6).

- **`GET /v2/auth/whoami`** (authenticated) describes the CREDENTIAL,
  never the person. Body (camelCase per C2, RFC 3339 UTC dates):
  `{key: {id, name, createdAt, expiresAt}, scope, grant: {scoped,
  permission, spaces: [{id, name, permission}]}, api: {version},
  keyStatus, notice?}`.
  - `grant.scoped` is the REQUIRED explicit boolean and the load-bearing
    field. A legacy unscoped key is `{scoped: false, spaces: [],
    permission: null}` — NEVER `spaces: null`: consumers get the
    null-vs-empty test backwards, and that failure direction is
    fail-open (the agent concludes it may touch every space).
  - `spaces[]` entries are OBJECTS with a per-entry `permission`
    (uniform today) so P2's per-space permissions land without a wire
    break; the grant-level `permission` stays as the compact form agents
    string-match on.
  - `spaces[].name` is resolved through the SAME grant-intersected
    `ListSpaces` path `GET /v2/spaces` serves, so a non-granted space's
    name cannot appear even by accident. The grant record stays
    authoritative for WHICH spaces are listed: a granted space missing
    from the live list keeps its entry with an empty name.
  - **The mirror is the gate's own record**: whoami is discovery, not
    enforcement, and derives from the request-context carriers
    `ensureAuthenticated` populated — `util.ApiGrantFromCtx`, the same
    accessor `ensureSpaceGrant` and the service backstop read — never a
    second derivation path (that is how a mirror starts lying).
    `TestWhoamiAgreesWithTheGate` derives gate expectations ONLY from
    the whoami body and fails on any disagreement.
  - The token is read ONLY from the `Authorization` header by the shared
    auth middleware; a query/body token is never accepted, an unknown or
    revoked key gets the middleware's plain 401 — deliberately NOT
    RFC 7662's introspection shape (no `active`, no POST), which would
    make the route an enumeration oracle.
  - **Registry class, reasoned**: `service-filtered` — authenticated
    (auth-exempt is impossible inside the gated group and the
    conformance walk enforces that behaviorally), addresses no single
    space, and its body's space names come from the service's own
    grant-intersected path, the exact pattern the class names.
    `data-free-allow` would be wrong: names are space data.
  - The `key.id`/`createdAt` plumbing is two additive
    `WalletCreateSession` response fields (`appHash`, `appCreatedAt`),
    cached on the session entry like scope and grant.
  - **`key.id` is credential-adjacent**: it is the app link's hash —
    sha256 over the raw key bytes, the same id ListApps shows — so a
    whoami response (and the api-server log line below) carries a full
    offline VERIFIER for the credential: not invertible (256 random
    bits) and computable by the holder anyway, but treat pasted whoami
    bodies and shared api-server logs as credential-adjacent artifacts.
  - **One gate decision the mirror cannot express**: `POST /v2/spaces`
    is `scoped-denied` (§8.10) — refused for EVERY granted key,
    readwrite included — and the whoami vocabulary (spaces ×
    permission) has no field for it. Kept un-modeled deliberately while
    the class covers exactly one route, and creating a NEW space is
    outside "the spaces I was granted" by plain reading; if the class
    ever takes a second route, add an additive
    `grant.restrictions: [...]` array instead of letting the mirror
    under-tell further.
- **Legacy-key deprecation signals** — emitted by `ensureAuthenticated`
  on BOTH route groups (legacy keys live on `/v1`). Deliberately NOT
  RFC 9745 `Deprecation`/`Sunset`: that header requires a Date (the
  boolean form died in draft) and §2.2 scopes it to the RESOURCE in the
  response — on `/v1` it would declare `/v1` deprecated, the opposite of
  the grandfathering promise (a test pins that neither header ever
  appears). Instead: `Anytype-Key-Status` (`legacy`|`scoped`, ALWAYS
  present so absence never means anything; grant PRESENCE decides), and
  `Anytype-Notice` (one printable single-line ASCII sentence, never
  interpolating user data) plus
  `Link: <…/docs/guides/get-started/authentication>; rel="deprecation"`
  (legal without a `Deprecation` header — RFC 9745 §3.1's own worked
  example for "policy, no date committed"; the target is the live
  authentication guide until the dedicated key-scoping page ships,
  because a policy link that 404s inverts the signal). The remedial
  pair — notice and Link — is emitted ONLY for nil-grant keys of
  JsonAPI scope: a grant is only ever valid on JsonAPI scope
  (`wallet.ValidateAppLinkGrant`), so a Limited (clipper) or Full
  credential cannot follow the "re-issue as a scoped key" advice; those
  keys still read `Anytype-Key-Status: legacy` (they ARE unscoped) but
  get no impossible instruction. The whoami body repeats the signal
  (`keyStatus`, `notice`) under the same rule — agents read bodies, not
  headers. A rate-limited INFO log line (once per key per process
  start, re-armed hourly; nothing is wrong, so never warn) names the
  key id and app name for nil-grant JsonAPI keys only — it exists so WE
  can tell whether anyone still presents legacy JSON-API keys before a
  sunset is ever contemplated, and counting clipper keys would inflate
  exactly that number.
- **Secret-scanner rules** (the §1b deliverable): detection-only
  gitleaks + TruffleHog rules in `docs/secret-scanning/` (README there
  explains why the GitHub partner program is unavailable to a local-first
  app and why TruffleHog cannot live-verify a localhost credential — the
  offline CRC32 is the stand-in). Both rules carry the published RANGE
  pattern verbatim; `core/wallet/applink_scanner_rules_test.go` pins them
  against a freshly minted key and the repo's own `anytype_…`
  identifiers, pins the two rules against each other, and walks the
  tracked tree asserting every full-shape match (the swagger example key
  and the OpenAPI documents generated from it) falls under the shipped
  gitleaks allowlist — the rule must come back clean on the repo that
  ships it. Coverage boundary, recorded per spec §1b: Limited/gRPC keys
  keep minting unprefixed and stay invisible to the rules by design.
- **OpenAPI**: `make openapi` is green again and the `docs/v2` documents
  are regenerated (whoami included, 401/403 documented as the shared
  middleware envelopes). The v2 swag step used to die on
  `json.RawMessage` (swag cannot resolve the stdlib alias, and its v3
  parser panics outright on `swaggertype:"array,…"` — Items is never
  set); the fixes are `swaggertype:"object"` on `SchemaEntry`'s
  object-valued raw fields, plus two doc-only stand-ins for the
  array-valued cases: `v2model.ViewObject` for the view-listing
  responses and `v2model.SearchRequestDoc` for the search body (a
  reflection test pins the twin's JSON field set to `SearchRequest`'s so
  the published document cannot drift from the decoded type).

### 8.12 Surface-review fixes M2 + M6 (2026-08-07 — decisions as built)

**M2 — permanent refusals no longer dress as retryable 500s.** Four
producers used to fall through `RespondV2Error`'s 500 fallback, sending
retrying agents into loops on refusals that can never succeed:

- **Restriction refusals on PATCH/PUT → 403 `forbidden`.** The per-op
  gate (`editNeedsForOps`/`restrictionRefusal`, ops.go) now produces the
  C6 403 at the verdict site — message carries the adapter's refusal text
  plus the offending op and `/ops/i` path, and the issue hint states the
  refusal is permanent. The mutator path (the adapter's in-lock
  `checkObjectEditable` re-check and `Apply`'s per-block restrictions) is
  classified by `mapWriteError` (object.go) on
  `restriction.ErrRestricted` via `errors.Is` — sentinel-backed, no
  string matching. Dry runs reach the same 403 (the verdict rides the
  read). The earlier refusal tests fed a ready-made `*v2model.Error`
  into `BlocksRefused` — green against a shape production never
  produces; they now feed the adapter's real wrapped-sentinel chain.
- **Foreign chat edits/deletes → 403** through the exported
  `chatobject` sentinels (§8.7's classification bullet, updated there).
- **File-upload failures are classified** (`v2FileRpcError`, file.go):
  in URL mode, a source answering non-2xx (matched through the
  `fileuploader.ErrFailedToDownload` sentinel, which the uploader now
  wraps — pinned by an uploader test that fails if the wrap is dropped)
  and a fetch that never got a response (`Get "…"` — `url.Error`'s
  fixed rendering, pinned against the stdlib type in the v2 test;
  `CleanupError` masks the URL inside but keeps the shape) are 400s
  naming `/url`. Local-path staging failures and storage faults stay
  500 — those are genuinely retryable or server-side. The upload
  pipeline has no size-cap error to classify: nothing bounds a URL
  download's size today (recorded, not fixed here).
- **The space classifier's strings are compile-pinned** (M2d): the
  workspace-RPC arms now match `space.ErrSpaceNotExists` /
  `ErrSpaceDeleted` / `ErrSpaceStorageMissig` and
  `restriction.ErrRestricted` through the sentinels' own `.Error()`
  text instead of duplicated literals, so a producer rewording updates
  the matcher at compile time. Behavior unchanged.

