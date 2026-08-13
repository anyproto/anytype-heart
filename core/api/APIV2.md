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
| C3 | **Compact JSON always** (no pretty-printing) — **all the way down, not just the envelope**. `anyblockjson.Marshal` returns the format's canonical byte form, which is two-space **indented** (SPEC §4), and the v2 envelope re-embeds those bytes verbatim; until Wave 0.1 every object read was therefore compact on top and pretty-printed underneath, costing a measured 16–26 %. *(Built: `encodeEnvelope` compacts each embedded value — the serving layer, so the format's canonical form and its `Export ∘ Import` byte-stability are untouched; §8.24.)* | free 38–46% (§3.6); 16–26% (TOKENS §1.1) |
| C4 | **Two document shapes, one id axis** (revised Wave 0.2, hardened §8.26; TOKENS §1.2/§10). `?ids=compact` (**the default — the *edit* shape**): **machine-minted** block/row/column/view ids — 24-hex bson and view UUIDs, `isMintedLocalId` — relabel to their 5-char suffixes (legend-less, **lossy**); every id that could carry meaning (`dataview`, `title`, readable imported ids) keeps its full spelling and is **reserved**, so no label can alias a served id. `?ids=full` (**the *export* shape**): full ids everywhere — the **backup/export** read (§3(b)), and the read to clone from when a POST should reuse the source's real ids. Object refs are **full inline on every shape**: the `refs` legend was a measured net loss **on the measured corpus** (85–90 % of refs used once; §1.2's own model has it winning only at ≥2× reuse) and its indirection trapped write-back, so no shape serves one — but legend **resolution on input stays total** (SPEC §9a), so a document arriving with a legend still resolves. Every write channel resolves a block/view/row/column id by exact id **or unique suffix** (`matchBlockRef`), which is what makes the lossy edit shape addressable — **in payload slots as well as reference slots since §8.29**, so no channel takes an id **literally**, which is what made the compact shape a trap (§8.26). *(§8.27 claimed this was already true once PUT was gone. It was not: PATCH resolved `updateBlock.id`, `replaceSubtree.id`, the targeting refs and the table/view refs, but handed `replaceSubtree.blocks[].id`, `updateBlock.set.{rows,columns,views}[].id` and `setCell.value[].id` to the format importer verbatim — reproduced as permanent id corruption on the documented read-then-echo loop.)* A payload id resolving to nothing is refused, not minted over; omitting it is how new content is authored. Never require echoing a full CID. *(Built: `Options.CompactBlockLabels` composed by `objectReadPlan`; `Options.CompactObjectRefs` remains a format-package option no API shape sets. Wave 2 renames the two values to `?mode=edit\|full` with no change of bytes.)* **Outline exception (T7)**: the outline fixes the axis — short labels — and ignores `?ids=`. | ~24×/id, −89% id errors (§3.6); block labels −19…−22% on minted-id documents, the legend a net **loss** of 0.9–11.5% on the measured corpus (TOKENS §1.2, live-measured); id round-trip contract (R1) |
| C5 | Minimal rows: list/search responses carry `id, name, type` + requested property values. **Never embed type objects.** `fields=` expands. | v1's N× multiplier (§2.1) |
| C6 | Error shape everywhere: `{status, code, message, issues:[{path, message, hint}]}` — path-addressed, naming allowed values. Required codes include: `validation_failed`, `version_unsupported` (surfaces SPEC §10's "produced by a newer version" verbatim, naming both versions), `idempotency_conflict` (same key, different body), `etag_mismatch`, `ambiguous_input` (e.g. both `filter` and `filters` supplied), `forbidden` (403 — an operation the caller's identity may not perform, e.g. editing another member's chat message; added by the Phase-6 review; also the `/v2` key-scope gate's refusal of a non-JsonAPI key, which answers in the shared v1 envelope — §8.9). Error text is API surface; test it. | repair loop (§3.2, §4.6); R15 |
| C7 | Every object read returns **`etag`** (short opaque token, ≤8 chars, derived from tree heads — NOT the object's `revision` property, which stays in `properties`) plus an `ETag` header. Mutations accept **`If-Match` header only** (the AnyBlock body has no envelope slot for it). **Advisory by default**: without `If-Match`, ops apply last-write-wins and `diffStats` reports the outcome; with it, mismatch → 409 `etag_mismatch` carrying the current etag. Note: the etag advances on background sync, not only on agent edits — strict If-Match will 409 on sync noise; block-scoped preconditions (ops apply iff the *addressed* blocks are unchanged) are the deferred v2.x refinement. | R2; optimistic concurrency (§3.2) |
| C8 | `Idempotency-Key` honored on all mutations (POST, PATCH, DELETE — v0.3.5/Phase 6; was POST-only); replay with the same key returns the stored result; same key with a different body → 409 `idempotency_conflict`. Response always returns created ids. | agent auto-retry (§3.7); R15 |
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
    ?ids=compact|full               # document shape (C4); default compact (edit)
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
  without blocks suppresses `blocks` entirely); `ids` selects the whole
  document shape (C4: `compact` = the edit read, `full` = the export
  read); illegal combinations → 400 `ambiguous_input` naming the
  conflicting params. The outline shape uses compact block labels but
  **full object refs** (read-only shape; the C4 outline exception —
  `ids` is ignored there), which since Wave 0.2 is also what the default
  and subtree reads emit, so a block id never changes spelling between
  an outline, a `?block=` and a default read.
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
  Payload `indent: 0` = the document's top level; `position` names an end of
  the document when nothing else is targeted — `first` the start, `last` (or
  absent) the end (§8.32; the original shape had no root-prepend and refused
  `position` here at all).
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

**(b) There is no full-document replace — removed §8.27**

`PUT /v2/spaces/{spaceId}/objects/{objectId}` existed through the Phase-3
hardening and is **gone**, with its whole pipeline: the route, the handler,
`PutObject`/`putPipeline`/`normalizePutBody`/`checkPutBlockIds`, the
`ObjectMutator.ResetObject` port and the `preserveEditorOwnedState` repair
its reset machinery needed. **PATCH is the edit surface.** The principle it
leaves behind is §8.27: *snapshots are for creates, edits are ops.*

A `?block=` subtree read is still marked `"subtree": true` and still no
write path accepts it (create names it by path). `?ids=full` survives as
the **backup/export shape** (C4) and as the id vocabulary a clone-from-read
POST should use — not as "the PUT read".

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
  "sorts": [ { "property": "due_date", "direction": "asc" } ],
  "fields": ["name", "dueDate", "status"] }
```

Secondary example — programmatic composition (the structured array;
mid/frontier and round-trip flows, mirroring §5's secondary multi-op
illustration):

```json
{ "type": "task",
  "filters": [ { "property": "done", "condition": "equal", "value": false } ],
  "sorts": [ { "property": "due_date", "direction": "asc" } ] }
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
launch op set (they back wrapper tools — §7/S1) · **no full-document
replace at all** (PUT shipped through Phase 3 and was removed — §8.27:
snapshots are for creates, edits are ops), full-block-id round-trip
default, diffStats · flat AnyBlock as the primary content
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
verb-set** (`cmd/anytype`, generated from the same table) · **the MCP
server** (`anytype mcp`, stdio, tier-filtered over the same table —
§8.20) ·
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
  withheld from the prompt) · **full launch op set**. The DELEGATE-52
  corruption baseline was to be a **PUT-only** arm; with PUT removed
  (§8.27) that arm is a *simulated* whole-document rewrite (read the
  document, regenerate it, `replaceSubtree` the root run) — it never was a
  gate arm. Decision rule: whether the
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
| `find` | `space, query?, type?, filter?, limit?` | Phase 4 search **[build — the true §7 blocker]** | filter = string form; results are enumerated handles + minimal fields. *Since §8.33: with none of `query`/`type`/`filter` the call matched nothing, so it LISTS the space instead — unnumbered, assigning no handles, because handle 1 of a listing is not the object anyone asked for* |
| `read` | `object, mode=full\|outline` | Phase 1 read | outline returns short block labels; `full` carries full block ids the wrapper relabels (§7.1/§7.4) |
| `describe` | `space, type` | Phase 1 `types/{type}/schema?flavor=table` **[build — 501 stub today]** | the accuracy lever, **called before create/set** (folds A1 into the flow); interim degraded form assembled wrapper-side (§2 Phase 5); every backing GET is space-scoped, so the tool takes `space` too. *Since §8.33 it reports what is SETTABLE — the type's recommended lists were neither a superset nor a subset of that, and hid `name` and `description`* |
| `create` | `space, type, name, properties?, markdown?` | Phase 2 create | type and property keys validated with did-you-mean; **select option names create-missing by default (R9/§8.1)** — the small-tier pre-validation guard is wrapper-side (§7.4); markdown caveats until the parser lands (below) |
| `set_properties` | `object, set?{key: value}, add?{key: […]}, remove?{key: […]}` | `setProperties` op incl. per-key `add`/`remove` (§8.3) | mirrors the op so a one-tag append never rewrites the whole array (the op's entire rationale — reintroducing the read→rewrite→write trap at the wrapper layer would defeat it); `add` on a non-empty select errors, steering to `set`; scalar→array coercion is server-side |
| `check_item` | `object, block, checked` | `updateBlock` op | the one block-field tool: checkbox **blocks** are a common note shape and `updateBlock` is THE block-update op post-§8.3; other block-field updates (color/align/language/retype) stay excluded — SKILL.md steers task completion to properties (the E4 recipe) |
| `add_blocks` | `object, after?\|under?, markdown` | `insertBlocks` op, `markdown` payload **[build]** | **markdown channel**; server parses → flat blocks (§7.1) |
| `edit_text` | `object, block, find, replace` | `replaceText` op | **anchor channel**; deterministic server replace; find/replace text is markup source until D′1 lands (§7.1); an EMPTY `replace` deletes the found text (Required means present, not non-empty — §8.6) |
| `set_cell` | `object, table, row, col, value` | `setCell` op | flat cell write (as built the tool takes `object` too — the REST op addresses a table within one object, and a table-only reference would need a hidden cross-object table registry; §8.6); row/col take the labels full read mints (rows and columns relabel like blocks); an EMPTY `value` clears the cell (null on the wire) |
| `move_block` / `delete_block` | `object, block, after?\|under?` / `object, block, recursive?` | `moveBlock`/`deleteBlock` ops | handle-addressed |

Excluded from the wrapper: whole-document replace (the DELEGATE-52
corruption vector — and since §8.27 excluded from the REST surface too,
so this line records a gap that closed from the other end), multi-op
batch (single tool
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
   GET+regenerate+write-the-whole-document would reintroduce corruption —
   which is the same argument §8.27 later applied to the REST PUT itself.
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
  harnesses.) (b) *wrapper-side relabeling of full-read block ids* —
  **RETIRED (§8.26)**: the server labels reads itself since Wave 0.2, and
  the wrapper's `relabelDoc`+label-map turned out worse than dead weight —
  its 24-hex predicate matched nothing in a server-labeled document (the
  ambiguity retry's label source went dead), while a STALE map surviving
  in the CLI session file rewrote a just-read label into an outdated full
  id. Refs now pass through verbatim; the minted-shape predicate the
  wrapper pioneered became the server's relabel rule. (c) *suffix
  pass-through* as the write mechanism (`matchBlockRef` resolves unique
  suffixes on every op) — now the ONLY mechanism. (d) *the ambiguity
  retry, scoped honestly (post-§8.26)* — refs go to the server verbatim,
  so a 400 `ambiguous_input` means the ref did not resolve against the
  document the server saw; the wrapper re-reads the object and retries
  once when the ref uniquely tails one of the re-read's own SERVED ids,
  which self-heals exactly the concurrent-modification race (the
  collision the server saw is gone by the re-read). A persistent
  ambiguity is unresolvable in principle — the wrapper cannot know which
  block the model meant — and surfaces the server's error. Because the
  rewrite lands after the Idempotency-Key is minted, `LastWrite` records
  it (`PriorHash`+`Rewrites`) so an identical re-run replays under the
  same key instead of re-applying.
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
sets, plus the `canUpdateObject` exclusions; that diff-apply shape was
Phase 3b's PUT, and it left the API entirely with it — §8.27). §7 structural blocks (title/description) stay
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

**The mutation port (v0.3.4; single-method since §8.27).**
`apicore.ObjectMutator` has ONE method.
`MutateObject(ctx, spaceId, objectId, needs, apply func(edit ObjectEdit) error)`
is PATCH: the adapter locks the object, checks the object-level
Blocks/Details restrictions on the axes the batch touches, hands `apply`
an `ObjectEdit{SbType, Heads, State}` (State = `sb.NewState()`), runs the
bundled-revision guard (never downgrade `revision`; untouched keys are
simply inherited now), and commits with a plain `sb.Apply`. The
`ResetObject` sibling that carried the v0.3.3 reset-to-version machinery
(and `preserveEditorOwnedState` with it) went out with PUT — the child
state inherits everything that repair had to reconstruct, so nothing
replaced it. Dry runs never touch the mutator: the same op applier runs
on a private `state.NewDocFromSnapshot` of a plain read.

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

**C11 write-safety guard.** If the internal marshal of the live state
reports any loss warning (unmapped block, over-deep nesting), the PATCH is
refused (422) — otherwise the write-back would silently drop the
unrepresentable content, exactly what C11 forbids. PUT used to skip this
guard (a full replace being explicitly destructive) and surface the
warnings instead; with PUT gone (§8.27) the guard has no exemption left,
and the 422's advice is now "edit it in the app" rather than "replace it
wholesale".

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
an error; `position` with NO targeting field names an end of the document
(§8.32). `moveBlock` moves the whole subtree and re-bases its indents.

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

**PUT — REMOVED (§8.27).** As built it stripped the envelope
`etag`/`warnings` a GET body carried, rejected a non-matching envelope
`id`, kept the live object's type when `type` was absent, and ran the R9
referential layer like create (with a corpse-key tolerance so a GET→PUT of
the same bytes round-tripped). All of that left with the surface; the
envelope-stripping half survives on the create path (`normalizeCreateBody`,
so a pasted read body clones), the corpse tolerance did not — a PATCH names
only the properties it edits. `canUpdateObject` mirrored as smartblock-type
exclusions (relation, relation option, file object, participant) remains,
on PATCH.

**Structural blocks.** As with Phase-2 creates, SPEC §7 structural blocks
(title/description) are absent from the format, so an edit re-lands the
document without them; name/description content lives in `properties` and
survives, and the editor regenerates the header blocks (same §7 contract).

**Per-op discovery (§5) as shipped.** `GET /v2/schemas/ops/{op}` for the
launch ops; each schema is C13-strict and self-contained, with a shared
payload-block definition covering the realistic edit fields
(`additionalProperties:false` — the full block inventory stays at
`/v2/schemas/object`, which the def points to; the block-type vocabulary
itself is published in the def since §8.32). Every example is a full
single-op PATCH body (enforced by test) — *changed in §8.32: the example is
now the op object itself, an instance of the schema served beside it.* The
index (`GET /v2/schemas`) grew an `ops` list.

### 8.3 Phase-3 revisions (v0.3.5 — pre-release design review, decisions as built)

Six contract changes from the modification-surface design review, taken
while the API is unreleased and breaking changes are cheap.

**Root targeting for `insertBlocks`/`moveBlock`.** Omitting all of
`after`/`before`/`inside` appends at the end of the document root (state:
`InsertTo("", Block_Inner)`). Chosen shape: the omitted-anchor form, not an
explicit `at: "start"|"end"` field — fewer fields for a small model, and it
is exactly the §7 wrapper's omitted-`after` case. *Revised in §8.32:*
`position` is no longer refused here — with no targeting field it picks the
end of the document, so `first` is the root-PREPEND this paragraph declined
to build (anchored before the first document block, never at the state root,
which carries the §7 header as its first child). This closes the structural
hole where an empty object (SPEC §7: no title/description blocks in the
document) had zero addressable anchors and PUT was then the only way to give
it content — which is also why removing PUT (§8.27) cost the surface nothing
here. More than one targeting field is now "at most one of after, before,
inside is allowed" (was "exactly one … is required" — reworded because zero
is legal now).
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

**Idempotency-Key covers PATCH (C8 widened).** The store, request hash,
in-flight reservation and replay were POST-only wiring; agents auto-retry
on timeout, and PATCH is where a blind retry does damage (a retried
successful `insertBlocks` duplicates blocks; a retried `deleteBlock` 404s
misleadingly). The middleware acts on every mutation METHOD — POST, PATCH,
PUT, DELETE — and is registered on the object PATCH route and the
types/properties PATCH routes. (PUT stays in the method set although
§8.27 left v2 with no PUT route: the switch classifies methods, so a
future mutation method is covered by construction.) Semantics unchanged: same key + same request (the §8.1 hash —
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
server binary was NOT built in this phase — it landed later as the tiered
`anytype mcp` verb (§8.20), the long-lived host that constructs the same
Runner with the in-memory session store.

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
method set was POST/PATCH/PUT (PUT's route later went away — §8.27 — but
the method stays in the classifier). `ensureIdempotency` now also acts on
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

- **Restriction refusals on PATCH → 403 `forbidden`.** The per-op
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


**M6 — the five typed Phase-2 bodies bind strict and bounded.**
CreateProperty, UpdateProperty, CreateSet, CreateCollection and the JSON
UploadFile now decode through `decodeStrictJSONBody` like chat/space/
search: unknown fields 400 with the field named in a C6 issue (the
reproduced trap — `"option"` for `"options"` silently creating an
option-less property — now rejects), empty bodies 400 with the shape
hint, and the bodies are capped at 1 MiB (`maxV2StructuredBodySize`)
regardless of Idempotency-Key — previously the cap only engaged when the
idempotency middleware buffered a keyed body, so keyless requests to
these five routes were read unbounded. The bounds the discovery schemas
advertise are enforced at the service layer from named constants
(schema_write.go: name 4096, key 256 + `^[a-zA-Z0-9_]+$`, options 100,
option color 64, filter 4096, sorts 10, views 10, collection items 1000
— checked before the per-item store walk — url 4096), and a drift test
in schemas_test.go pins the served schema JSON to those constants so
neither side can move alone. The one-table derivation the review
suggested (generating the schema strings from the constants) was
REJECTED as not worth it: the schemas are hand-written JSON with prose
descriptions, and the existing chat/space precedent — constants + drift
test — already makes divergence a test failure. `UploadFileRequest.name`
remains accepted-but-unused by the service (pre-existing; the schema
advertises it — recorded, not fixed here).

### 8.13 Surface-review fixes M1 + M5 (2026-08-07 — decisions as built)

**M1 — the edit gate is per-op.** `checkObjectEditable` demanded both
`Restrictions_Blocks` and `Restrictions_Details` of every edit. Sets and
collections carry Blocks but NOT Details (`objRestrictEdit`), so a PATCH to
either was refused whatever it contained: renames, which restrictions never
forbade, and `addItems`/`removeItems`, the only v2 route into an existing
collection. A collection was write-once — seedable at POST, immutable after —
even though §6 retires v1's `AddObjectsToList` in favour of these ops.

`apicore.ObjectRead` now carries `BlocksRefused` and `DetailsRefused`
separately, and `ObjectMutator.MutateObject` takes an `apicore.EditNeeds`
derived from the batch (`v2OpEditNeeds`, `editNeedsForOps`). Item ops need
NEITHER axis: they mutate the collection store
(`template.CollectionStoreKey`), which no object restriction governs — the
same position v1 takes, its `ObjectCollectionAdd` being ungated. (PUT
demanded both, a document replace rewriting blocks and details alike; that
caller is gone — §8.27 — and `EditNeeds` is now derived from ops only.) A
refusal
now addresses the offending op (`/ops/1`), not the request, so a batch mixing
a legal rename with an illegal block edit says which op is the problem.

The set/collection restriction facts are pinned against the LIVE restriction
table in `objectmutateadapter_test.go`, so a change to `objRestrictEdit`
fails there rather than silently restoring the bug.

**M5 — create-missing is bounded, and creates go last.** One FAILING PATCH
permanently created every option it named: 5,000 objects from a ~60 KB body,
~10^6 at the body cap, with no v2 option-delete surface to undo them.

There is no transaction available — options are objects, each its own CRDT
tree, so "create N options and mutate a document" cannot be one commit. The
irreversible part therefore goes last and small, in two halves that catch
different requests (`guardCreateMissing`):

1. **The bound** (`v2MaxCreatedOptionsPerPatch` = 64) is the only thing that
   stops a *well-formed* batch — one that would apply cleanly — from creating
   a million options. Enforced on a probe pass whose resolvers record instead
   of create, so the rejection costs one JSON walk and no RPC. The error names
   the count, the limit and the properties involved.
2. **The ordering** is the only thing that stops a *failing* batch from
   leaving debris: the batch is applied against a private state first, so an
   op that cannot apply is found before any create RPC fires. This subsumes
   case-by-case skip lists (a key claimed by both `set` and `unset`, a scalar
   where a list is required, …) — enumerating the ways a batch can fail is
   open-ended; validating it is not. It runs only when the batch actually
   names new options, so an ordinary PATCH pays nothing.

Both halves apply to dry runs, so C9's preview reaches the same verdict.

What remains is a crash or cancellation between the creates and the apply,
which cannot be eliminated without a cross-object transaction. It is now
bounded by the cap, convergent on retry (`OptionId` resolves an existing
option by name before creating, so a retry adopts the first attempt's options
rather than duplicating them), and detectable — created options carry
`ObjectOrigin_api`. A compensating delete was deliberately NOT added: the
delete is not atomic either, and another client may have started using an
option in the window, so it would add a second failure mode to paper over the
first. Cleanup belongs in a provenance-backed sweep, not the request path.

### 8.14 Surface-review fix M3 (2026-08-07 — decisions as built)

Three malformed structured-filter shapes reached the store as MATCH
EVERYTHING — silently, with no warning — inverting the surface's own promise
("unresolved → did-you-mean, never a silent no-match") in the most damaging
direction available:

1. a node carrying BOTH arms, `{"operator":"and","property":"severity",
   "condition":"equal","value":"High"}`. The codec (`filterFromJSON`) and the
   semantic gate both branch on `Operator != ""`, take it as a group, ignore
   every leaf field and emit an AND with no children. An empty AND is true.
2. a group with an empty `filters` array — the same empty AND, reached
   directly.
3. a leaf with no `condition`, which a typo'd key (`"conditon"`) also
   produces: it reaches the store as `Condition_None`, and
   `database.FiltersFromProto` drops it.

`validateFilterStructure` (`search.go`) enforces the SHAPE the served
`filters` schema already described but nothing checked, and
`decodeFilterNodes` is the single entry both v2 callers use. **The gate runs
on POST /sets as well as the query path**, because a set PERSISTS its filter:
there the same shape is not a bad query but a set that quietly contains the
whole space, for good.

The served schema was tightened to match the enforcement rather than left
advertising the broken shapes: the leaf arm now requires `condition` (it
required only `property`, which is what made shape 3 schema-legal), and the
group arm's `filters` gained `minItems: 1`. The two examples already carried
a condition on every leaf, so nothing published had to change.

Deliberately NOT touched: the shared document codec, which must keep
accepting `Condition_None` — stored dataviews legitimately carry it, and this
is a v2 request gate, not a format change. This is also the one input channel
with no GBNF grammar (a documented C13 exception, the tree being recursive),
so it is precisely where a small model's malformed output lands.

### 8.15 Surface-review fix M4 (2026-08-07 — decisions as built)

Sending the `Idempotency-Key` that C8 mandates on every mutation capped file
uploads at 10 MiB. The middleware buffered the whole body to hash it, so a
multipart upload — whose body IS the file — hit `MaxRequestBody` and got a 413
naming the body, never the header. The disciplined agent was therefore the
only caller that could not upload a large file, and the error steered it to
shrink the file rather than to drop the header it had been told to send.
Every keyed upload under the cap was also buffered whole in RAM and then
re-parsed by `multipart`.

**As built.** `isStreamedUpload` (multipart content types only) switches the
identity from the exact body to a BOUNDED PREFIX (`idempotencyPrefixBytes` =
64 KiB) plus the declared `Content-Length`; the remainder streams to the
handler through an `io.MultiReader`, so nothing buffers whole files and no
size ceiling appears. JSON bodies are unchanged — exact-body hash, 10 MiB cap
— because their bytes are small and ARE the request.

**The trade, stated plainly.** For multipart the conflict guarantee narrows:
two uploads under one key that agree in both declared length AND first 64 KiB
now replay instead of answering 409. In practice the prefix covers the
boundary, the part headers, the filename and the file's opening bytes, so
distinct uploads still differ there — the test drives exactly that case. This
is a narrower guarantee than the exact-body hash, bounded to this one content
type, and it is the price of not having a size ceiling.

**Rejected alternatives.** Exempting the route (the review's third option)
would have created the first C8 exception, which §1 currently says do not
exist — a documentation cost paid forever to avoid a bounded technical one.
Keying on the staged file's digest inside the handler is more correct still,
but moves idempotency out of the middleware for one route and cannot answer
the replay before the upload has already happened.

### 8.16 Surface-review fix M7 (2026-08-07 — decisions as built)

One PATCH could hold the object lock for tens of minutes. `v2MaxOpsPerPatch`
bounds the op count, but every mutating op invalidates the applier's view,
so the next op re-marshals the WHOLE document — under the smartblock lock,
where the cost is not latency for one caller but starvation for every
reader and writer of the object: ObjectOpen, sync, the app's own UI.
Reproduced before changing anything: 400 trivial replaceText ops measured
12.2 s on a 4,200-block document and 71.4 s on a 24,000-block one (~7 µs
per block-render), linear in the document exactly as the O(ops × document)
product predicts; the 10 MiB body cap × 512 ops extrapolates to 15–20
minutes from a single request.

**As built — two bounds, then one exemption that earns its way out.**

1. **The per-op blocks cap** (`v2MaxBlocksPerOp` = 256, enforced in
   `decodePayloadRun`): the served op schemas ALWAYS advertised
   `maxItems: 256` on the blocks channel of insertBlocks and
   replaceSubtree; nothing enforced it, so one op could inflate the
   document by 24,000 blocks for every later op to re-render. The markdown
   channel already enforced the same number; the two channels now share
   `v2MaxBlocksPerOp` by definition. No schema text changed — the schema
   was right, the server was lenient.
2. **The render-work bound** (`v2MaxPatchRenderWork` = 2^20 block-renders,
   `checkPatchRenderWork`): the product itself — view-rebuilding ops ×
   (document blocks + payload blocks, markdown counted at its per-op cap) —
   refused whole after the `begin()` marshal, before any op applies, with
   the numbers in the error. Rejection therefore costs one marshal, the
   same floor a GET pays. 2^20 ≈ 7 s worst case on a desktop machine.
   The hint says split the batch, and splitting is the fix rather than a
   workaround: the object is RELEASED between batches, which is the whole
   point. The check sits in `applyPatchOps`, so the guardCreateMissing
   probe pass, the dry run and the locked run all reach the same verdict
   (C9 dry≡real). Like the 512-op cap it is server-side behavior, not a
   schema-advertised bound.
3. **replaceText maintains the view in place** (`textEdited`): it changes
   exactly one exported field of one block — no ids, no structure, no
   indents — and it is the one op that inherently arrives many-per-batch
   (one find/replace each). It writes the CANONICAL rendering a re-marshal
   would emit: `RenderInlineText(parse(splice))` for markup text (the
   exporter's own `renderInline`; mark compaction is off on the edit path,
   verified in `compactMarks`), the literal splice for code/embed (§8.4),
   field dropped when empty (`setNonEmpty`). Canonical matters: a splice
   can leave adjacent marks (`**re****port**`) whose re-marshal reads
   `**report**`, and the next op's find must match what the agent would
   read back. With the exemption in `v2OpRebuildsView`, a full 512-op
   replaceText batch on a large document is legal again AND cheap:
   400 ops on 24,000 blocks went 71.4 s → 0.8 s, on 4,200 blocks
   12.2 s → 0.15 s — the two remaining renders are begin + the final
   after-document, which diffStats and the R5 net need regardless.

**Tests that fail if the fix is reverted** (verified by reverting): the
per-op cap and work-bound rejections in `TestPatchObject`, the work-bound
exemption for replaceText, and `TestApplierRenderCounts`, which pins the
bounded-work property in the unit that cannot flake in CI — whole-document
renders (`marshalCount`): exactly 2 for a 50-op text batch, per-op for
structural ops. Two semantics tests (sequential-canonical, an
updateBlock merging after a replaceText) pass on BOTH code paths —
they pin that the in-place update is byte-equivalent to the re-marshal
it replaces.

**Deliberately NOT done.** Incremental view maintenance for the
structural ops (insertBlocks, moveBlock, deleteBlock, updateBlock,
replaceSubtree, setCell): their exported form is produced by the
exporter's tree walk — normalization, indent clamping with the C11/B′2
warning contract, table wrapper pinning, the document-wide id domain —
and replicating that per op in the applier is the applier rewrite this
review round warned against. Nor for setProperties/addItems/removeItems:
they batch naturally into ONE op (maps and arrays), so the product term
barely exists for them, and the properties view has real staleness corner
cases (a key unset and re-set in one batch must re-check space existence).
The bound covers what the exemption does not.

**The residual limit, stated plainly.** A structural batch still pays
O(ops × document) up to the 2^20 budget — single-digit seconds of held
lock on a desktop, proportionally more on slower devices. Below the
budget, a hostile-but-legal batch can still buy ~7 s of lock; above it,
the work simply cannot be purchased in one request. And every PATCH keeps
its two-marshal floor, so a very large document costs one render even to
reject — the same floor its GET costs. The exact worst case a caller can
reach is therefore max(two renders of the document, the render budget),
never minutes.

### 8.17 The view write path: updateView (2026-08-07 — decisions as built)

**The gap, as reported by an agent using the API.** Dataview views were
readable three ways (the object document, `GET …/sets/{id}/views`,
`…/collections/{id}/views`) and writable zero ways after creation: the
then-existing PUT refused type documents by kind, the types PATCH accepts
only properties/typeProperties, and no view route accepts a write. The reporter's
concrete case — a custom type's default "All" view rendering every custom
column `hidden: true` — was TWO bugs stacked: no write path (this section),
and the generator regression that hid the columns in the first place
(GO-5969 inverted `MakeDataviewContent`'s precedence so a type's explicitly
passed relation links stopped being marked visible; fixed at the generator,
pinned in `collection_test.go` — a freshly created type now gets a usable
view and the write path is a repair tool, not a required rite of passage).

**Surface: an eleventh PATCH op, `updateView` — not a route.** Views are
part of the object's document (SPEC §6.2); C2 says one concept, one slot,
and the object-edit slot is `PATCH …/objects/{id}`. A dedicated
`PATCH …/views/{viewId}` was rejected: it would be a second way to edit one
object with its own idempotency/dry-run/etag wiring, it cannot compose
atomically with other ops, and it would need THREE registrations (sets,
collections, types) plus a fourth story for inline dataviews — the op works
on all four today, including `PATCH …/objects/{typeObjectId}` with the id
from `GET …/types/{key}` (type objects pass `checkEditPreconditions`; it
was only PUT's kind-gate that refused them, and PUT is gone — §8.27). Whole-array rewrite via `updateBlock` was
rejected twice over: it is the documented small-model trap (resend every
view to flip one bit), and updateBlock's `{Blocks: true}` classification
refuses it on exactly the three object classes that carry dataviews.

**Shape.** `{op, block?, view?, set?, columns?}` — at least one of
set/columns. `block` defaults to the object's only dataview (types, sets,
collections have exactly one, at the fixed id "dataview"); `view` defaults
to the only view; both resolve by full id or unique suffix (the C4 rule
`resolveViewRef` already applies on the read surface — resolution by NAME
was considered and dropped: names collide and localize, and every
ambiguity/not-found error lists `id ("name")` pairs so the repair needs no
second read). `set` merges §6.2 view-level fields with updateBlock
semantics (named fields change, explicit null clears one); `sorts` and
`filters` replace whole when named — small ordered lists; `filter` is the
compact-string alternative to `filters` (parsed exactly as POST /sets
parses it, ambiguous together). `columns` merges PER COLUMN, keyed by
property key: a patch object merges `{hidden, width, align, aggregation}`
into that property's column, appends a column for a key that has none, and
null removes one (removal is deliberately not key-validated — a stale
column for a deleted property must stay removable). `id` immutable,
`set.columns` steered to the columns channel, `groups`/`objectOrders`
rejected as §4a output-only — but they SURVIVE the edit: the merge happens
on the block's exported JSON and re-imports through the format codec
(`UnmarshalBlock`, the setCell pattern), and the importer round-trips
kanban editor state, so untouched views, columns, group orders and manual
object orders land back bit-identical. All validation runs against a
private deep copy first — a failing op leaves state and view untouched.

**One vocabulary, exported from the format.** The op validates view types,
card/list sizes, column align and aggregation against lists the
`anyblockjson` package now exports (`viewvocab.go`), pinned to the codec's
own enum tables by a drift test — necessary because the codec itself maps
unknown enum names silently to defaults on import, which is exactly the
silent-degradation an op surface must not inherit. Sorts and filters
validate through the exported fragment codec (`UnmarshalSorts`,
`UnmarshalFilters` — read-only resolvers, issues rebased onto
`ops[i].set.…` paths), the M3 structural gate runs on `set.filters` for the
same reason it runs on POST /sets (a persisted match-everything filter is a
view that quietly shows the whole space, for good), and the §6.2
unguarded-date-comparison finding rides the C11 warnings channel — PATCH
responses now carry `warnings` for the first time.

**THE RESTRICTION CLASSIFICATION — the decision that could have recreated
M1.** `v2OpEditNeeds["updateView"] = {}` — neither axis. Sets and
collections carry `Restrictions_Blocks` (`objRestrictEdit`) and object
types carry it too (`objRestrictEditAndTemplate`) — the three
dataview-bearing classes, so a Blocks-classified view op would be refused
on precisely the objects it exists to edit, the M1 bug reborn. The
classification is not a convenience but the editor's own position: the
Blocks axis gates document content (`basic.CreateBlock`, tables, clipboard,
uploads all check it) while the native view surface — `sdataview.
UpdateView`/`CreateView`/`DeleteView`, i.e. v1's ungated
`BlockDataviewView*` RPCs — checks NO object-level restriction, which is
how the app edits views on a set at all. Proved three ways:
`objectmutateadapter_test.go` pins sets/collections AND a custom type
object against the LIVE restriction table (Blocks refused, Details not);
`viewops_test.go` drives a PatchObject with production-shaped
`BlocksRefused`+`DetailsRefused` on the read and asserts the op succeeds
with `EditNeeds{}` recorded at the mutator; and the same on the dry-run
path (C9 parity). Verified fail-on-revert by flipping the classification
to `{Blocks: true}`: both tests fail, nothing else notices.

**Create-missing wiring (the M5/B6 interplay).** A view filter's select
values and a custom sort order carry option NAMES, which the dataview
import resolves with create-missing — so `prewarmCreateMissing` learned the
op: it walks `set.filters`, the parsed `set.filter` string and
`set.sorts[].customOrder`, resolving select/tag values BEFORE the object
lock and thereby inside the M5 bound (a channel prewarm cannot see is a
channel the too-many-options cap cannot count — the bound test fails if the
prewarm branch is disabled, verified by disabling it). One §11 alignment
closes the residual gap: `empty`/`notEmpty`/`exists` leaves get their
`value` stripped on store (the canonical form), so the in-lock import never
resolves — never mints — an option the view cannot use, and prewarm and
import see identical work.

**Reference-key rule, and a recorded divergence.** Keys a patch introduces
(columns, sorts, filter leaves, groupBy/coverProperty/endProperty) must be
known to the dataview (pre-merge membership: properties list ∪ any view's
columns) or to the space — rejected with the did-you-mean otherwise;
resolvable keys are appended to the dataview's `properties` list so formats
rehydrate (§6.2 sorts/filters carry no cached format). This is deliberately
LOOSER than POST /sets' R9 rule (type-recommended keys only): generated
views already carry columns outside that set (`backlinks`,
`lastModifiedBy`, `lastOpenedDate`), and an edit surface must not reject
what the surface already shows. The divergence means a two-step
set-build can reach a filter key the one-step create would refuse —
accepted: the native app allows the same, and the cost of the strict rule
here is false rejections on every generated view.

**Bounds (M6 discipline: advertised = enforced).** columns ≤ 64
(`maxV2ViewColumns`), sorts ≤ 10 (shared `maxV2SetSorts`), filter string ≤
4096 (shared `maxV2FilterLength`), pageSize ≤ 1000, width ≤ 10000 px (SPEC
§6.2: the editor's own range is 54…1000; omitted/null lets the client pick
per format), name ≤ 4096, keys/ids ≤ 256. The op rebuilds the document view
(`v2OpRebuildsView`), so the M7 render-work bound counts it with no new
plumbing. Served schema: `GET /v2/schemas/ops/updateView`, C13-strict
except the documented `filters` recursion (small models steered to
`filter`); the example is the one-line repair of the reported gap:
`{"ops":[{"op":"updateView","columns":{"status":{"hidden":false}}}]}`.

**Tests that fail if reverted** (each verified by actually reverting):
the generator-regression case in `collection_test.go` (stash the
`collection.go` fix → fails); the two restriction-classification tests
(flip `v2OpEditNeeds` → fail); the M5 bound test (disable the prewarm
branch → fails); the vocabulary drift test pins the exported lists to the
enum tables; and removing the op registration trivially fails the whole
`TestUpdateViewOp` suite.

**Deliberately NOT built** *(superseded by §8.18 — the view family shipped
the same day)*. View create/delete/reorder (`addView`/
`deleteView` — POST /sets seeds multiple views at creation; editing was the
reported gap; creation-after-the-fact is a separate, smaller decision and
the native RPC precedent has its own last-view invariant). Name-based view
addressing (see above). A dataview-properties op (the `properties` list
self-maintains through key usage). Type-scoped R9 tightening for edits
(recorded divergence above). `activeView` anything — local UI state the
proto excludes from changes. The swagger annotation names the new op;
`make openapi` regeneration is pending per the working agreement.

### 8.18 The view family: insertView, moveView, deleteView (2026-08-07 — decisions as built)

Supersedes §8.17's deferral: create, reorder and delete now exist, so the
view surface is symmetric — everything `GET …/views` can show, PATCH can
make, change, order and remove. The op set grows to 14, and stays learnable
because the three additions introduce NO new grammar: the block family's
verbs (insert/move/delete), view-scoped, sharing `updateView`'s channels
(`set`, `columns`), `updateView`'s block/view addressing, and
insertBlocks/moveBlock's targeting words. One noun, zero new verbs.

**Naming: `insertView`, singular — a deliberate break from `insertBlocks`'
plural.** The blocks payload is a structured RUN (ordered, indent-nested),
which is what the plural names; views have no internal structure, one view
per intent is the overwhelming case, and several views are several ops in
the already-atomic batch. The family symmetry that matters is with
updateView/moveView/deleteView, all singular. A mode-flagged mega-op
(`updateView` with create/delete/move modes) was rejected for the same
reason replaceBlock died in v0.3.5: mode flags are disambiguation load.

**insertView = "updateView aimed at a fresh view."** The base is either
sensible defaults or a `copyFrom` duplicate; `set`/`columns` then merge on
top through the SAME code paths (`applyViewSet`/`applyViewColumns`), so
everything §8.17 established — vocabulary, filter gates, key validation,
warnings, option create-missing (prewarm covers insertView too; the M5
bound test fails if it does not) — holds for create without a second
implementation. `name` is required (a view is a named tab; ≤4096). The
minted id returns in a new `createdViews` response map ("ops[i]" → id);
view ids are always server-minted — a payload has no id slot, `set.id`
stays rejected.

**The bare default is a view someone can look at.** `{"op":"insertView",
"name":"Recent"}` produces: one column per property the dataview lists,
ALL visible, sorted lastModifiedDate-descending. This deliberately breaks
with the native `CreateView` default (`dataview.go:333` — every column
hidden except name), which is the same disease the GO-5969 fix cured for
generated type views; matching native here would have shipped the reported
bug as the create default. The sort matches native (`DefaultLastModified-
DateSort`). `copyFrom` duplicates an existing view of the same dataview —
columns, sorts, filters, type, groupBy, card options, even the per-view
editor state, everything but id and name — because "like that one, but…"
is the common intent; §6.2 nests `groups`/`objectOrders` per view, so the
copied editor state re-keys to the new view id on import for free.

**Reorder is targeted, never a rewrite.** `moveView {view, after|before|
position}` — the moveBlock vocabulary minus `inside` (views are a flat
list; there is no container), with `position: "first"|"last"` standing
alone instead of riding `inside`. `position: "first"` is documented as the
"make this the default tab" verb: `activeView` is local UI state (§6.2,
excluded from changes), so the FIRST view is what a fresh client shows.
*(Revised in §8.19: moveView now REQUIRES a destination — the silent
append default this section originally shipped was judged a
forgotten-field trap that quietly changes the default tab. insertView
keeps its append default, where appending is the natural create
position.)* The splice adjusts the target index across the removal
(move-after-a-later-view is the test case); moving relative to itself
degenerates to a no-op rather than an error. insertView shares the same
targeting for its insertion point.

**Delete has one guard and one deliberate non-behavior.** Deleting the
last view is a clean C6 refusal (`cannot delete the last view` — the
native `DeleteView` invariant surfaced as a 400 with a repair hint, not a
corrupt object; the editor would regenerate a default on open, but relying
on that is sync-dependent). The guard counts the BATCH's state, not the
original document: insert-then-delete in one PATCH is legal and is exactly
how an agent replaces a type's default view atomically (tested). Deleting
a view some client had active is deliberately unhandled server-side:
activeView is per-device local state; that client falls back to the first
view. Per-view editor state (groups, objectOrders) vanishes with its view —
the §6.2 nesting makes orphaned group orders structurally impossible.

**Classification: the whole family is `{}` — neither axis** — same
derivation as §8.17 (all three dataview-bearing object classes refuse the
Blocks axis; the native view RPCs are ungated). Pinned as a FAMILY: one
test drives an insert+move+delete batch through PatchObject against a read
refusing BOTH axes and asserts `EditNeeds{}` at the mutator — flipping any
of the three in `v2OpEditNeeds` fails it (verified by flipping).

**Tests verified fail-on-revert** (by actually reverting each): the family
classification (flip → fails), the last-view guard (remove → fails), the
insertView prewarm coverage (drop from the condition → the M5 bound test
fails). The create-defaults decision is pinned by construction (the bare-
insert test asserts every column visible and the sort).

**Deliberately NOT built.** A per-dataview view-count cap (native has
none; unbounded growth across requests is the insertBlocks precedent, and
the M7 render-work bound covers per-request work — an advertised cap that
existing user data already exceeds would strand updateView). Client-
supplied view ids (nothing needs them pre-creation; minting keeps the id
space server-shaped). copyFrom across dataview blocks (the source must be
a view of the addressed dataview — cross-block copying is a read+insert
composition the agent can already do). View duplication INTO another
object (out of PATCH's one-object scope by definition). `make openapi`
regeneration is pending (the PATCH annotation now names all 14 ops).

### 8.19 View-family review fixes (2026-08-07 — decisions as built)

Three review lenses (correctness/data-safety, agent contract, tests) went
over §8.17/§8.18 as shipped. The headline finding invalidated a §8.17
claim; the rest tightened contracts and closed fixture gaps. Dispositions,
in the reviews' order:

**A — the commit path could mint options under the lock (fixed, the big
one).** The view-op commit re-imported the WHOLE dataview block through the
create-missing resolver. Export writes option values as NAMES (falling back
to the raw id for a dangling reference), so every view's filters and custom
sort orders re-resolved name→id on every view op — including views the op
never touched. Consequences, all reproduced: a dangling reference (deleted
tag, filter keeps the id) round-tripped into a BRAND-NEW option named after
the raw id with the filter rebound to it; those creates fired under the
object lock (the B6 invariant §8.17 claimed could not be reached); they
bypassed both halves of M5 (one moveView over an untouched view with 200
dangling values = 200 options past the cap of 64, and a batch REFUSED as
atomic still left its options created); and with two options legally
sharing a name, a pure moveView repointed a filter to the other twin by
store listing order.

As built, two mechanisms:

1. **The commit imports with a NO-CREATE resolver**
   (`commitImportOptions` / `readOnlyOptionResolver`): names resolve
   through the prewarm's create cache and the store; a miss passes through
   verbatim instead of minting. Op-authored names are created by the
   PRE-LOCK prewarm, so the cache covers them; anything the prewarm cannot
   see is by construction content the op has no business minting for. This
   also closes the narrow prewarm/import format-disagreement case the
   review flagged (dv-list says select, space says longtext): the value now
   passes through verbatim instead of creating under the lock.
2. **Unauthored content is restored from the live proto after the import**
   (`viewCommitPlan` / `restoreUnauthoredViews`): moveView and deleteView
   author nothing — every surviving view is byte-restored, only
   order/membership comes from the splice; updateView and insertView author
   one view, and within it sorts/filters restore from the live (or
   copyFrom-source) proto unless the op's set actually named them. The
   codec round-trip is thereby a no-op for everything the op did not write:
   no rebinding, no twin repointing, no drift.

The fixture hole the reviews named — no test ever put content in a view
the op does not address — is closed by four tests: dangling-value
verbatim survival through moveView, twin-option id stability through
updateView-on-the-other-view, format-disagreement pass-through, and
copyFrom preserving the source's exact ids. Each verified fail-on-revert
by reverting the resolver and the restore separately.

**B — the M7 bound was blind to dataview weight (fixed).** A dataview is
ONE block whose marshal cost is O(views × columns); a fully legal
512×insertView batch on a wide set held the lock ~25 s while scoring 0.05%
of the budget — and §8.18's no-view-cap justification leaned on the bound
it was beating. `checkPatchRenderWork` now takes the parsed blocks: the
document factor counts per-view weight (1 + columns + sorts + filters) and
every insertView adds the document's heaviest per-view weight to the
payload factor (the copyFrom worst case). The §8.18 justification holds
again. Pinned by a test whose 512×insertView batch on a 10-view×50-column
set must be REFUSED — it passes the old cost model, so it fails if the
model reverts (verified; the reverted run also demonstrated the 24 s hold).

**C — the family's M7 registration was asserted, never tested (fixed).**
The marshal-count pin (the TestApplierRenderCounts pattern: two view ops =
begin + one rebuild + final) is now COUPLED in one test to the
`v2OpRebuildsView` entries, so measured per-op re-marshaling and the map
that accounts for it cannot drift apart.

**D — the served sorts schema rejected what reads emit (fixed).** Stored
sorts carry an `id`; the exporter emits it; the strict item schema lacked
it — so read→edit→write of a sort was schema-refused while the server
accepted it. The item schema gains `id` (documented output-only-on-reads,
accepted back). A drift test now walks the served schema and pins the enum
lists to the `viewvocab.go` exports too — the hand-duplicated schema enums
were the one link the vocabulary drift test did not reach.

**E — insertView had two name slots (fixed).** `set.name` silently
overrode the op's required `name`, and `set.name: null` produced a
nameless view from an op whose schema declares name required. insertView
now rejects `set.name` with the steer (updateView keeps it — there it IS
the rename channel), and its served set schema drops the name property
(`v2ViewSetPropDefNoName`), keeping schema and server in agreement.

**F — the indent strip was load-bearing and unexercised (fixed).** All
fixtures kept the dataview at indent 0, so deleting the view-doc `indent`
before re-import was dead weight in every test while being essential for
the shipped inline-dataview shape. An indented-inline fixture now pins it.

**Minors.** Removing a column from a column-less view no longer writes
`columns: null` into the block (a reachable 400). The advertised
`customOrder` maxItems (128) is enforced, and `set.filters` both advertises
and enforces a top-level-nodes cap (32; nesting stays the documented C13
recursion exception). The compact filter string now validates keys against
the same membership as the structured form (the whole dataview: properties
list + every view's columns — it saw only the addressed view's columns, so
the recommended input form rejected keys the structured form accepted,
with a did-you-mean that omitted the right answer). moveView requires a
destination (§8.18 revision note above). The bare-insert default is built
from pre-op membership — its default sort no longer grows the properties
list, so two bare inserts in one batch produce identical views. copyFrom's
kanban editor-state fidelity is now pinned by a fixture that has some.

**Rejected, with evidence.** "copyFrom duplicates per-node sort/filter ids
— a state the native editor never produces": the generator itself ships
fixed node ids on every generated view (`DefaultLastModifiedDateSort`'s
`byLastModifiedDate`, `defaultChatSort`'s `byLastMessageDate` — identical
across every set in every space), so shared node ids are generator-normal;
and after fix A the copy's sorts/filters are proto-restored from the
source, making a JSON-side strip literally unreachable code (it was
written, then deleted on that evidence).

**Deferred.** `?fields=` projection on GET …/views (a read-cost
optimization, no correctness stake). The Date-object `state.ErrRestricted`
500 is pre-existing, shared with every block op, and ticket-worthy — not a
view-family fix.

### 8.20 The MCP delivery: two model tiers over one table (2026-08-07 — decisions as built)

The §3 "MCP server binary" item is built: `anytype mcp --tier small|large`
serves the §7 tool table over MCP stdio. This section records the research
verdict that scoped it, the tier split, the selection and transport
decisions, and the repair-loop contract.

**Who MCP is for — the research verdict (confirm-with-caveats).** The
hypothesis under test: MCP is not worth it for large models (Sonnet-class
should get `core/api/v2/SKILL.md` + raw HTTP, or the CLI + its skill);
MCP exists to serve small local models. The evidence *confirms the
narrow form and rejects the sharp one*:

- *Context cost and tool-count degradation are real and hit large models
  too* — Anthropic's own engineering posts measure a 150k→2k token drop
  from keeping MCP schemas out of context (code-execution-with-MCP,
  2025-11) and a 49%→74% (Opus 4) / 79.5%→88.1% (Opus 4.5) accuracy gain
  from deferred tool loading (advanced-tool-use); practitioner
  measurements put single servers at ~55k tokens of schemas; BFCL and
  successor benchmarks show selection accuracy falling with tool count.
- *But the ecosystem's fix was to repair MCP's discovery mechanics
  (deferred/searchable tool loading), not to abandon MCP* — so "MCP is
  wrong for large models" is not the lesson; "don't preload schemas you
  don't need" is.
- *For CLI-capable large-model harnesses the skill+CLI/HTTP path
  measurably wins on cost, not correctness*: Arize's 500-trial eval
  (2026) found correctness statistically tied between MCP and CLI/skills
  while MCP ran ~6× the cost and ~5× the latency on hard tasks. Simon
  Willison's skills-vs-MCP token argument is the same conclusion from
  the token side.
- *The standing counterargument*: non-terminal hosts (desktop apps,
  chat clients, mobile) cannot run a CLI at all — for them MCP is the
  only delivery at ANY model size. And small-model tool calling is where
  strict schemas + grammar constraints genuinely compensate for a
  capability gap (sub-7B models emit malformed calls unaided; GBNF
  fixes syntax, not semantics).

Decision as recorded: the MCP server here **targets local small models**
(the task's premise, upheld); large CLI-capable agents keep being pointed
at the CLI skill (`cmd/anytype/SKILL.md`) or the raw-HTTP skill
(`core/api/v2/SKILL.md`). The caveat is recorded rather than acted on:
a capable model in a non-terminal host may legitimately use `--tier
large`, and nothing in the design penalizes that — at ≤12 tools this
server never enters the schema-bloat regime the research warns about
(the whole large-tier `tools/list` is ~1.5k tokens).

**The tier split — a field on the one table, never a second list.** The
§8.6 one-definition invariant extends: `Tool.Tier` marks the smallest
tier a tool is served to; `ToolsForTier`/`BuildManifestForTier` filter;
golden-list tests pin both sets and a mandatory-tier test makes an
undeclared tool a failure, not a silent omission (`tier.go`,
`tier_test.go` — verified fail-on-revert).

- **small (~8B, Gemma-class), 8 tools**: `spaces, find, read, describe,
  create, set_properties, add_blocks, edit_text` — the tasks a local
  assistant actually performs (find/read notes, capture, set a status,
  append content, fix wording). Omissions are decisions, each on the
  misuse-worse-than-missing principle: `check_item` (the E4 recipe steers
  task completion to `set_properties` anyway; a block-addressed toggle is
  niche and adds a whole reference-resolution failure surface),
  `set_cell` (five required args on a rare shape), `move_block`
  (restructuring is rare; the after/under anchor vocabulary is the most
  confused one in the set), `delete_block` (destructive with a
  `recursive` escalation — the worst cost for a wrong guess).
- **large (~20B, Qwen-class), 12 tools**: the whole table. No NEW tools
  were minted for it: §7.2's exclusions (whole-document replace — since
  §8.27 not a REST surface either — batches, structured filters, block-field updates, archive-without-a-route) were re-checked
  and stand — they were excluded for corruption/ambiguity reasons, not
  for being beyond a 3–4B. The tier field makes a future 13th tool a
  one-line tier decision; chats are the named candidate when a use case
  shows up.
- Arguments are NOT tiered: the table's args are already flat and
  minimal, and per-arg tiering would fork the schema/GBNF/CLI renderings
  of one definition — rejected as complexity without a demonstrated win.
- The CLI verb set is NOT tiered (coding agents are large models);
  `anytype tools --tier small` narrows the manifest for non-MCP
  function-calling hosts.

**Selection + packaging.** One binary, an `mcp` verb on `cmd/anytype`,
tier by `--tier` flag (default `large`). Rejected: a separate `cmd/`
(duplicates env/flag plumbing and splits the skill story; the §8.6
"verb set == tool set by construction" argument applies to deliveries
too); tier-per-server-name (two registrations of one binary with no
added expressiveness — hosts pass args natively); an env var (flags are
visible in host config where the choice is made). The server constructs
the §8.6 long-lived Runner over the in-memory session store — the CLI's
session file stays the CLI's (concurrent MCP servers must not fight
over it, and a host restart starting clean is predictable behavior).

**Transport.** MCP stdio (newline-delimited JSON-RPC 2.0), hand-rolled
in `wrapper/mcp.go`: `initialize` (version negotiation across
2024-11-05/2025-03-26/2025-06-18 — the tools-only surface is identical
across them; unknown versions answer ours), `tools/list` (the C13
schemas as `inputSchema`, `readOnlyHint` on the four non-mutating
tools), `tools/call`, `ping`; notifications acknowledged by silence,
batching refused with steering. A third-party MCP SDK was weighed and
rejected: the needed subset is ~300 lines, the repo carries no MCP
dependency today, and hand-rolling keeps the wire shapes pinned by our
own tests instead of a vendor's release cadence — the SDK becomes worth
it the day this server needs resources/prompts/elicitation, and that
day should be a §3 item, not a drive-by. `initialize` also serves
tier-aware `instructions` (the SKILL.md loop compressed: spaces → find
→ describe-before-create → read-before-edit, dates/@me, "follow the
error, retry once").

**The repair loop (the guessability contract).** Tool failures return
IN-BAND (`isError: true` + text) so the model reads the tip; only
malformed JSON-RPC and a name outside the tier are protocol errors —
and the unknown-tool message still lists the tier's tools. The tip
chain, outermost first: wrapper argument validation (already
steering-shaped), the §8.6 ops→tool vocabulary translation (server C6
hints arrive saying `under`/`block`/`read mode=outline`, never
`ops[0].inside`), and two MCP-layer additions for the conditions whose
fix is outside the model's reach — API unreachable ("ask the user to
start the Anytype app") and key rejected ("ask the user to check
ANYTYPE_API_KEY"), both ending "no change to the call will help" so a
small model stops burning retries. `TestMCPRepairLoop` pins the loop
end-to-end per case: the wrong call, the EXACT tip, then the corrected
call succeeding in the same session (handle-before-find, missing
required arg, after+under together, bad enum, server C6 in tool
vocabulary, 401, unreachable). The vocabulary translation and the
in-band error path were both verified fail-on-revert.

**Not built, stated.** No resources/prompts/sampling/elicitation (no
use case on a localhost notes API today); no HTTP/SSE transport (local
hosts spawn stdio; the API server itself is the HTTP surface); no
per-tier GBNF re-derivation beyond what the manifest already serves
(the grammars are per-tool and tier filtering subsets them); no
Claude-facing MCP recommendation (per the verdict, capable CLI-running
agents keep the skill+CLI path).

### 8.21 Small-model benchmark fixes (2026-08-08 — decisions as built)

The first LIVE benchmark of the shipped MCP surface: `anytype mcp
--tier small` (8 tools) driven by gemma4:e4b and gemma4:e2b over
Ollama against a running Anytype API, 8 realistic tasks. The numbers
that motivated this section:

- **e4b**: right tool 7/8, executed 5/8 first try — the existing tips
  repaired 2 of the 3 failures.
- **e2b**: right tool 6/8 but "executed" 8/8, because two successes
  were SILENT WRONG ACTIONS — it called `spaces` for "what properties
  does the page type have", and `read` for "change the word draft to
  final". A wrong action that returns 200 is worse than any refusal:
  nothing in the transcript invites a repair.
- **Every argument error was a naming/capitalisation guess** —
  `"Page"` for type `page`, `set.Name` for property `name`, `"page
  type"` lifted from the prompt's phrasing — never a structural or
  schema violation. The GBNF/schema layer is doing its job; the
  remaining error surface is semantics, and semantics is repaired by
  candidates in error texts, not by grammar.
- The one tip WITH candidates (unknown property key: "known … keys:
  …") repaired on the first retry; the one WITHOUT (type not found)
  produced **no retry at all** — same run, same model. Candidate lists
  are not decoration; they are the difference between one repaired
  call and a dead end.

Three fixes, each shaped by those observations:

**1. `edit_text.block` is optional — the snippet locates the block.**
Both models routed around `edit_text` (the `read` silent-wrong-action
above IS this defect): a required block id is unknowable on turn one,
and a tool that requires a prior call is a tool a small model will not
use. When `block` is omitted the wrapper reads the document and
applies the mandatory ambiguity rule — the snippet must identify
exactly ONE block, and (the existing rule) occur exactly once within
it. Zero matches → refusal steering to `read mode=outline`; several
matching blocks → refusal LISTING the candidate block labels with
~30 chars of surrounding context, so the retry passes `block`
explicitly; several occurrences in the one block → the existing
more-context refusal, issued during the locate (no wasted PATCH). A
silent wrong edit is far worse than any of these refusals. The locate
read retains labels (the next call starts resolved); a located id is
full, so the §7.4 ambiguity retry never double-fires. The manifest
example is now the block-less form — the call a small model can
actually make first. Rider: the server's `replace_all` escape hint is
stripped by the ops→tool vocabulary translation (edit_text
deliberately has no `replace_all`, §8.6 — the hint steered models
into an argument the tool rejects). One-table invariant held: schema,
GBNF, example and CLI flag re-derive from the same Arg row.

**2. The not-found family lists candidates (server-side).** The
routes addressing a type or property BY KEY (GET/PATCH/DELETE
`types/{key}`, options listing, PATCH/DELETE `properties/{key}`)
answered a bare "not found — list keys with GET …" while the R9
create path always listed keys + did-you-mean — an inconsistency in
our own error surface, and the measured dead end above. One composer
(`notFoundWithKeys` in refs.go) now serves the family: known keys
capped at 15, nearest-match did-you-mean, the list route only when
the list was truncated with no suggestion. Family survey, recorded:
view refs and the filter option path already listed candidates; block
refs steer to outline (the candidate list IS the outline); the space
404 gains the `GET /v2/spaces` steer but never a candidate list (ids
are opaque — no did-you-mean can help — and a scoped grant must not
imply the full space list); the object 404 is left alone (unbounded
candidate set, and wrapper models reach objects through find handles
— the wrapper's own no-session/stale-handle errors steer the re-find).

**3. Type and property keys fold case — in the WRAPPER, not the API.**
The judgment, argued: C2 (a key is a key) is the REST surface's
contract — programmatic clients depend on exact-match strictness, and
two keys differing only by case must stay distinguishable over REST.
The wrapper is the layer built to be forgiving for small models
(§7.3 already places @me, relative dates and the A2 guard there), so
the fold sits beside them. The hard rule either way: if two keys
differ only by case, refuse naming both — never pick one. Property
keys fold in `prepareValues` against the format index the call
already fetched (zero extra requests), BEFORE the format lookup — so
`"DueDate": "friday"` also gets its date resolution and the A2 guard
runs on the folded key; a key given together with its case variant
refuses instead of last-write-wins. Type keys fold on the ERROR path
only (find, describe, create retry once with the unique case
variant): the correct-key common case never pays a type listing, and
a folded create re-derives its Idempotency-Key — a different resolved
request must not reuse the failed body's key (C8). A fold miss
surfaces the server's now-candidate-bearing error untouched. NOT
folded, stated: select option NAMES (user data — `done` and `Done`
can both legitimately exist; the A2 guard's case-insensitive
did-you-mean already covers the guess) and property keys inside
filter STRINGS (parsed server-side; the parse error carries
did-you-mean — a wrapper-side fold would mean parsing the filter
twice; revisit if a benchmark shows filters failing on case).

Tests: `tools_smallmodel_test.go` pins all four locate outcomes, the
fold-and-refuse matrix and the replace_all strip;
`discovery_test.go`/`schema_write_test.go` pin the server candidate
lists. Every behavioral assertion was verified fail-on-revert (the
fold-miss and unknown-key pass-through cases assert unchanged
behavior and pass either way, by design). Deferred, named: case-fold
inside filter strings; a candidates steer on the object 404; B4
re-tuning of tool descriptions once the benchmark re-runs.

### 8.22 The (a) identity layer — mint, corpse policy, key resolution (2026-08-08 — decisions as built)

Implements the safety core of `pkg/lib/anyblockjson/ADDRESSING.md` (§7.5,
§7.5a, §7.6 build step 3 plus the step-5 corpse policy), superseding the
queued "point v2 at apiObjectKey" fix — which, alone, would have imported
the slug layer's collision problem (§2.3-1). Nine commits (seven code,
two docs), each verified
fail-on-revert where the behavior is invisible until data is wrong.

**The derived slug table** (`pkg/lib/bundle/apislug.go`): the authority for
a bundled key's snake api slug is a fixed table in code, both directions —
verified collision-free (194 relation slugs distinct, 29 type slugs
distinct, fold layer clean) by tests that fail on the first bundle change
that mints a collision. `ApiSlug` is deliberately the same transform
objectcreator's `injectApiObjectKey` applies at mint. One dossier example
respells: `dueDate2`/`due_date2` converge on `due_date_2` (strcase
separates trailing digits), not `due_date2` — same collision, different
joint spelling.

**The corpse policy** (§7.5-req-2, the §8-OQ2 vacate lean — both live
defects fixed): UI delete sets only `isUninstalled`, which the query
layer's injected defaults never filtered — so a UI-deleted property still
listed in `GET /properties`, still resolved on PATCH/DELETE, and blocked a
same-key create with "already exists" pointing at a corpse, while an
archived one was invisible to the guard and the create died downstream on
a raw `ErrTreeExists`. Now: `keys.go`'s live queries exclude corpses
everywhere the API addresses schema (listings, routes, did-you-mean
candidate lists, existence guards), corpses vacate the slug namespace at
mint, and delete-then-recreate is a clean create with fresh identity.
DELETE of an already-deleted property/type is a 404, not a re-archive.

**The mint** (§7.5 strategy (a), the strategy-(b) remnants retired):
`POST /properties` no longer writes the caller's key as the stored
relation key, `POST /types` no longer derives the uniqueKey from the
document key, and `PropertyId` no longer pins typeProperties keys —
every create mints a BSON internal key and the caller's key becomes the
`apiObjectKey` slug, snake-normalized. The union collision check ships
WITH the mint (bundled keys + bundled-derived slugs + live stored keys +
live stored slugs; §8.23 unified it onto the resolution chain — all mint
paths, fold classes included): `due_date`, `Due Date` and `dueDate` can no longer
shadow the bundled property, sequential normalized twins refuse loudly,
and a name-derived slug (key omitted) is guarded identically — the
refusal steers to the existing holder or an explicit different key
(auto-suffixing was considered and rejected for POST: explicit beats
silent). Bundled keys keep the derived install path — convergence IS the
install mechanism (§2.4-1).

**Input resolution** (§7.5a-5 + §7.5a-3): every place v2 takes a type or
property key walks the chain — exact stored key, live slug namespace,
bundled vocabulary (exact or derived slug), fold layer (`DueDate`,
`due-date` → `dueDate`) — with ambiguity a loud 400 listing every holder,
never store order. Wired into route params, search/set type scopes,
document creates (`canonicalizeDocumentKeys` — detail keys and
`ot-` URLs are the store's vocabulary, not the wire's; it guarded PUT the
same way until §8.27), `setProperties`
keys, and the option-create prewarm (a slug-keyed select would otherwise
mint options bound to the slug string). The §8.21 benchmark's Title-Case
miss now resolves with zero retries.

**Output spelling** (the dossier's blessed interim, NOT yet the full
§7.5a): `GET /properties`/`GET /types` rows and the type column on
object/search rows spell a BSON-keyed entity by its slug iff the slug
round-trips to that row (twins and stored-key shadows keep the honest
BSON; corpses always do). Readable stored keys keep today's spelling.

**The §2a format check** (§7.5-req-4): a typeProperties entry whose
declared format contradicts the resolved relation (live, slug-resolved or
bundled) is a path-addressed 400 on both POST and PATCH types, checked
before the create-missing resolver can mint. It lives in the v2 wiring
because `PropertyDefinition` cannot distinguish an absent format from
longtext (enum zero).

**Slug hygiene**: the option path's un-snaked second injection branch
(§2.3-3) removed — it was dead code (ToSnake never empties a non-empty
transliteration).

**§8 leans built as assumed**: OQ1 — `apiObjectKey` stays mutable,
address-only (nothing freezes it); OQ2 — vacate + loud floor. Deferred,
named: the ACTIVE re-slug-on-revive half of OQ2 (no v2 revive endpoint
exists; the bundled reinstall path cannot collide because minting over
bundled slugs is refused; a UI bin-restore of a custom twin is caught by
the ambiguity-loud lookups and the conservative served spelling, not yet
auto-repaired); the full §7.5a respelling sweep (bundled keys → snake on
the wire, SPEC §3 key-vocabulary flip, schemas/goldens/SKILL/eval
respell); ADDRESSING §7.6 steps 1–2 (pins + the D1 kill — SPEC-level);
§7.4 strict-on-PATCH write defaults; the §7.5-req-5 backfill (its own GO
issue — until it runs, pre-slug custom BSON keys have no stable bare-op
address, exactly as the dossier states); key slots inside view-op set
channels accept stored keys only — set filters and the whole query
surface canonicalize since §8.23 (slug inputs in the one remaining channel fail
loud via R9, never silently). The heart-side mint (`injectApiObjectKey`)
still checks nothing — UI creates can still mint twin slugs; v2 defends
via the ambiguity 400 and round-trip serving. OpenAPI regeneration is
pending (`make openapi` not run here); no annotation shapes changed.

### 8.23 Identity-layer review pass: five causes, fixed as causes (2026-08-08 — decisions as built)

A five-lens review of §8.22 found incomplete wiring in five structural
clusters; each is fixed at its cause. Seven commits, every behavioral fix
verified fail-on-revert (by targeted behavior reverts where a whole-file
revert would only fail compilation).

**Cause 1 — the document body was an unguarded input channel
(REGRESSION, reproduced).** A type document's properties map copied
verbatim into the create RPC; `uniqueKey` is itself a bundled relation,
so a forged `{"uniqueKey":"ot-page"}` rode into `getUniqueKeyOrGenerate`
and `DeriveTreeObject` — occupying the id a later bundled install
converges to (strategy (b)'s silent merge, reachable under (a) through a
channel the union check never inspected). Fixed with a reject list
(uniqueKey, relationKey, isReadonly, restrictions — export strips all
four, so no legitimate round trip carries them; path-addressed 400) and
a drop list for system-managed details a round trip DOES carry
(apiObjectKey — a supplied value bypassed the union check when the name
slugged empty — origin, spaceId, isArchived, isDeleted, isUninstalled).
The same forgery's second channel — an envelope `key` on an OBJECT
document becoming `snapshot.Key` → `uniqueKeyInternal` →
`DeriveTreeObject` (found while fixing; the review named the details
channel) — is rejected in `validateDocumentRefs`, covering every document
create (it covered PUT too until §8.27).

**Cause 2 — one canonicalization chain everywhere.** The prewarm's
`canonicalPropertyKey` and `PropertyId`'s resolution now ARE
`resolvePropertyInput` — the §7.5a-5 chain every other channel walks —
closing at one stroke: the M5 bypass (the prewarm lacked the fold the
in-lock pass had; a folded spelling made 70 option creates run INSIDE
the object lock past the 64 cap — the repro is now a test), the
corpse-resolving typeProperties path (custom corpse keys mint fresh;
bundled keep the storeresolver fallback — bundled identity is derived
and invariant), and the stored-key shadow (`myKey` beside a legacy
`my_key` resolves to the legacy relation via the fold; a spelling whose
fold misses but whose minted slug collides — `"My Key"`, the space
survives folding — is refused by the slug-side union re-check). The
canonicalization-equivalence table test pins prewarm ≡ in-lock over
stored/slug/folded/ambiguous/miss spellings, so the next divergence
fails there first.

**Cause 4 — guards robust.** `liveProperties`/`liveTypes` return their
store error (a hiccup no longer empties the namespace and waves
collisions through — fail closed; hint-only lists degrade); entries are
primed once per request and mandatory in the chain (the N+1 loops in
document validation and setProperties/view checkKey share one snapshot);
the mint remembers its own request (`mintedSlugs` — two spellings of one
key in one document refuse instead of both minting `warranty_until`);
the union check covers fold classes (`moodlevel` beside `mood_level`
refuses — an occupied folded spelling would be permanently ambiguous);
the type union check runs BEFORE Unmarshal (a refused type create leaves
no orphan typeProperties relations — M5's lesson in a new path);
`canonicalizeDocumentKeys` reports duplicates deterministically; and
ambiguity candidates print stored key + id — the addresses that always
resolve (twins printed one identical slug; nothing was actionable).
Hidden holders vacate the slug namespace (resolution, fold, collision,
serving) while keeping exact-stored-key addressability: a hidden twin is
invisible and undeletable to the caller and must not 400 the visible
holder's slug.

**Cause 3 — the query channels speak what the listings advertise.**
`keycanon.go` canonicalizes every concrete property input of search
(fields — read from the stored key, emitted under the requested
spelling — structured filters, the compact string, sorts, format and
option lookups), list `?fields=`, and set creation (the request's
filters/sorts/views rewritten in generic JSON before validation and the
persisted document — a served slug would have become a permanently dead
dataview filter). Membership accepts stored AND served spellings (plus
bundled derived slugs — acceptance is wider than advertising); candidate
lists speak served spellings only. The type filter LEAF resolves through
the chain, corpse-aware — one spelling now works at every level, and a
UI-deleted type stopped being a usable query scope (`typeKeyExists`
likewise: objects/templates of a corpse type refuse). PUT tolerated
corpse-HELD property keys (GET emits them for objects still carrying
values, and a GET→PUT of the same bytes had to round-trip) while POST kept
live-only; §8.27 retired the tolerance with PUT on the grounds that
live-only was now the whole rule — **wrong, and reverted in §8.29**: PATCH
has its own in-document escape (`checkKey` passes any key already on the
document), and create is the channel a read body is pasted into, so the
tolerance moved to **create** as `propertyKeyHeldByAnyRelation`. The
file aliases' deactivation is chain- and corpse-aware (an uninstalled
`mimeType` relation no longer silently drops the field space-wide).

**Cause 5 — derived-slug hygiene.** Name- and document-key-derived slugs
are sanitized to the advertised `^[a-zA-Z0-9_]+$`/maxLength grammar
("50% done", "C++", "☕" → unidecode "?" — all previously became
identity-bearing, unaddressable apiObjectKey values); empty means no
derivable slug and the minted BSON stays the only address.

**Rejected, with evidence:** corpse-awareness for `isCollectionType`
(the input is the object's OWN stored type key — a data predicate;
refusing addItems on an existing collection whose type was uninstalled
would diverge from the app) and for set-source resolution
(`setSourceFilters` reads setOf — stored identifiers, never wire
spellings; a set over a deleted type still lists its objects in the
app, and v2 stays at parity).

**Still deferred, restated:** view-op set channels (updateView/
insertView filters and sorts) accept stored keys only — slug inputs
there fail loud via the view-key validation, never silently; folded
spellings are accepted on routes, documents and ops but NOT inside
search/set filter validation sets (only stored, served and bundled-slug
spellings enumerate); the §7.5a respelling sweep, pins/D1, §7.4
defaults, the backfill and active re-slug-on-revive as in §8.22.
Truthfulness fixes to §8.22 ride this pass (commit count, the union
check's scope, search's former stored-keys-only state).

### 8.24 Wave 0.1 — the served body is compact all the way down (2026-08-09 — decisions as built)

**The defect.** C3 promised "compact JSON always" and the envelope
delivered it only at the top level. `anyblockjson.Marshal` emits the
format's canonical byte form — two-space indented with a trailing
newline (`marshalCanonical`, SPEC §4) — and the read path splits that
document into `map[string]json.RawMessage` and re-emits a compact
envelope whose `properties`/`blocks`/`refs` values keep their indented
bytes **verbatim**. Every default object read shipped an indented
document inside a compact wrapper.

**Measured on the live account** (o200k, the six-document TOKENS corpus,
served bytes → the same documents re-rendered compact): −15.5 % (XS,
0 blocks) · −20.2 % (S, 12) · −21.3 % (M, 24) · −26.4 % (L, 66) ·
−23.9 % (R, 22) · −21.4 % (K, 31); corpus total **−23.2 %**. This
reproduces TOKENS §1.1 (16–26 %) within a point on every document.

**Where the fix lives, and why not in the format package.** In
`encodeEnvelope` (`v2/service/service.go`), which compacts each embedded
value with `json.Compact` before concatenating it. Three reasons the
serving layer is the right one:

1. `marshalCanonical` is the format's **canonical byte encoding**. SPEC
   §4's serialization canon ("UTF-8, LF, two-space indent") is what
   §11.2's `Export ∘ Import` byte-stability is defined over, and what the
   four `testdata/rich*.json` goldens pin byte-for-byte. Making it
   compact would move all four goldens and silently flip ~30 in-package
   `assert.Contains` probes that match on `": "` — six of them
   `NotContains`, which would go **false-green**, not red. None of that
   is a price a token saving should pay.
2. Nothing outside the tests depends on the *whitespace*: the etag hashes
   tree heads (not the document), C8 idempotency hashes the client's
   request body, `snapshotdiff` and the eval corruption scorer are
   state-level, and every round-trip/byte-stability assertion compares
   Marshal output to Marshal output under the same options. The one
   cosmetic loser would be `cmd/anyblockroundtrip`'s `firstDiff` line
   reporter and the human-diffable `.json` artifacts `anyblockrecover`
   writes — both of which want the indent.
3. The envelope fix is **total — for the right reason** (corrected by the
   Wave-0 review: the first write-up named "PATCH/PUT response documents"
   and a "create echo" that do not exist — the edit routes return
   `EditResult` and creates return `CreateResult`, plain structs via `c.JSON`, never
   through `encodeEnvelope`). Only three handlers write bytes verbatim
   (`c.Data`): `GetObject`, `markdownEnvelope` and `GetType` — and all
   three serve `encodeEnvelope` output. Everything else exits via
   `c.JSON`, which compacts embedded `json.RawMessage` on its own. So the
   invariant is guarded by the **handler's write-path choice**: a future
   read handler reaching for `c.Data` with bytes that did not pass through
   `encodeEnvelope` reopens the hole.

`json.Compact` is whitespace-only — the exported form runs with HTML
escaping OFF, so it neither re-escapes `<`/`>`/`&` nor rewrites
U+2028/U+2029, and the format writer's deliberately non-HTML-escaped
strings survive byte for byte (pinned by a test whose fixture carries the
raw U+2028/U+2029 characters, not just the six-character escape —
`json.Compact` could never touch the escape, so only the raw characters
pin that half of the claim). A value the JSON scanner rejects is appended
verbatim rather than erroring: `encodeEnvelope` is a formatter, not a
validator, so malformed input keeps exactly the body it produced before
compaction existed.

**No golden moved and no fixture was regenerated** — which is the signal
that the change landed at the right layer.

### 8.25 Wave 0.2 — `?ids=` splits into two document shapes (2026-08-09 — decisions as built)

**The defect.** `CompactIds` is shorthand for two mechanisms with
**opposite** economics (`export.go:35-37`), and one query parameter moved
both:

- `CompactBlockLabels` relabels doc-local block/row/column/view ids to
  short suffixes. **Legend-less** (the server resolves a label by exact
  id, else unique suffix — `matchBlockRef`, shared by `?block=`, `?view=`,
  every block-addressed op and `resolveTablePart`) and **lossy** (the
  originals are not recoverable from the document alone). A pure win on
  reads: cheaper *and* easier for a small model to echo back.
- `CompactObjectRefs` shortens object refs through the `refs` legend.
  Lossless, and a measured net **loss**: 85–90 % of refs in real documents
  are used exactly once, so the legend lines cost more than the inline ids
  they replace, and the indirection traps write-back of object-valued
  properties (a model that saw `"ai52e"` inline must dig the legend to
  name the object again).

So you could not take the winner without the loser. The outline shape
already moved them independently (T7), which is the precedent this
generalizes.

**As built.** `objectReadPlan` carries the two axes separately;
`V2ObjectQuery.validate` composes them into one of two shapes, and
`GetObject` just applies them:

| `?ids=` | block ids | object refs | for |
|---|---|---|---|
| absent / `compact` | short doc-local labels | full inline, no legend | the default **edit** read |
| `full` | full | the `refs` legend | the **export** read: backups, and the shape to clone from |
| (outline, any `?ids=`) | short doc-local labels | full inline, no legend | T7, unchanged |

*(Superseded by §8.26 in two cells: `full` no longer carries the legend —
object refs are full inline on every shape — and "short doc-local labels"
narrowed to machine-minted ids only.)*

The old `?ids=full` (no compaction on either axis) is gone; nothing
depended on it and Wave 2's `?mode=` enum has no such profile. **Today's
default shape did not disappear — it became `?ids=full`**, so no shape was
invented and the export loop keeps exactly the bytes it had. *(§8.26 then
dropped the legend from `full`, so the export bytes changed once more —
deliberately. §8.27 removed the write-back leg of that loop entirely: the
export shape is a BACKUP shape, not a round-trip-through-PUT shape.)*

**Legend resolution on input is untouched.** SPEC §9a's resolution rule is
total, and the create/PATCH paths still accept any document carrying a
legend, whoever produced it.

**`GET …/types/{key}` rides along** — it delegates to `GetObject`, so a
type document's minted view ids come back as labels too. That is safe
because every consumer resolves by suffix
(`updateView`/`insertView`/`moveView`/`deleteView` via `matchBlockRef`,
`?view=` via `resolveViewRef`), and because the internal documents those
ops work on are rendered **without** compaction — `list_read.go`'s
fixed-`"dataview"` block lookup and the applier's per-op re-render both
see full ids. Type creates reject a `blocks` array outright, so there was
no GET-type → PUT-type block loop to break even before PUT went away.
*(As first shipped this hardcoded the default query — so "the export shape is one query parameter
away" was false for types, and the well-known `dataview` block id was
served as `aview`. §8.26 threads `?ids=` through and the minted-shape
rule keeps `dataview` full by construction.)*

**Measured on the live account** (o200k, the six-document TOKENS corpus,
each axis isolated against the same compact-encoded baseline):

| doc | blocks | block-label axis | legend axis (live served bytes) |
|---|---|---|---|
| XS-props | 0 | — | −11.5 % |
| S-12blk | 12 | **0.0 %** | −4.6 % |
| M-24blk | 24 | **0.0 %** | −5.6 % |
| L-66blk | 66 | −19.1 % | −2.4 % |
| R-20refs | 22 | −1.8 % | −5.3 % |
| K-recipe | 31 | −21.9 % | −0.9 % |

The legend column confirms TOKENS §1.2 on every document: dropping it is
a saving, never a cost.

**The block-label column is bimodal, and the review's flat "~15 %" is
not what the corpus shows.** The rule behind the bimodality (as hardened
in §8.26): **opaque machine-minted ids compact, meaningful ids do not** —
relabeling fires on 24-hex bson ids and view UUIDs, where it is worth
**19–22 %**, and never on anything else. (As first shipped the mechanism
was accidental — a dash-free label *charset*, which keyed on the id's
last five characters rather than the id's nature: the UUID
`32726bf3-…-688e9525ed67` relabeled to `5ed67` despite its dashes, while
`pages-roadmap-home-1` did not, purely because of its tail `ome-1`. §8.26
replaces the charset accident with the explicit minted-shape predicate,
which an agent can reason about in one sentence.) The S/M/R rows are 0 %
because their seeded ids are readable — a property of this demo account's
seeding, not of production documents; but it is also a real ceiling: a
document of meaningful ids gets nothing from the axis, by design. The
useful corollary: **on such documents the default read IS the export
read** — every id serves in full, so the two shapes are byte-identical
there, and the id-adoption trap (while PUT still existed) was confined to
minted-id documents.

**Combined with §8.24, against the actual served bytes:** XS −24.9 % ·
S −23.5 % · M −25.1 % · L −42.1 % · R −28.9 % · K −39.3 %; corpus total
**−33.1 %** — a third off every object read.

**The one behaviour that got worse, named plainly — and then removed.**
PUT took the document's block ids literally. As first shipped, a
GET(default) → edit → PUT loop on a minted-id document did not "re-mint
with `diffStats` visibility" as this section originally claimed — it
**adopted the 5-char labels as the stored block ids, permanently**
(reproduced: after the PUT the stored ids *were* `1bcb9`, `1c5c4`, … and
a PATCH with the original 24-hex id 404ed), breaking every id another
client held; and on tables `diffStats` under-reported the rewrite (§8.26).
PATCH was never affected (it suffix-resolves). §8.26 closed the trap with
a refusal, and **§8.27 closed it by construction**: the literal-id channel
is gone, so no served vocabulary can reach the identity channel at all.
The follow-up this section left open — "teach PUT the same suffix
resolution the write ops already use", parked for Wave 2.1 — is retired
with its subject.

### 8.26 Wave 0 hardening — served ids are a vocabulary, not an identity channel (2026-08-09 — decisions as built)

A three-lens opus review of Wave 0 (encoding correctness, live
round-trips, tests/truthfulness) found the compaction half sound and the
id half leaking: the default read's block ids had become a different
vocabulary from the stored ids, and several write paths took served ids
literally. This section records the fixes — the relationship was fixed,
not the nine symptoms — plus the defects recorded deliberately unfixed.

**The relabel rule inverted: only machine-minted ids relabel.** The
shipped predicate was "relabel unless the candidate label has a dirty
charset" — an accident that keyed on an id's last five characters rather
than its nature, relabeled meaning-bearing ids (`table1` → `able1` in the
golden; `featuredRelations` → `tions`; the documented `dataview` constant
→ `aview` on type reads), and carried a genuine aliasing hole: the label
census only counted ids longer than the label width, so a 5-char id and a
minted id sharing that suffix were served as the SAME id — silent
wrong-block PATCHes, a wrong `?block=` subtree, and a write-back of the
server's own read 400ing on `duplicate id` (live today via `?outline=true`
regardless of Wave 0.2). Now `isMintedLocalId` recognises the actual
minting sites — 24-char lowercase hex (`bson.NewObjectId().Hex()`: every
editor block/row/column id, and the format's own `defaultGenerateId`) and
RFC-4122 UUIDs (`uuid.New().String()`: view ids) — and everything else
keeps its full spelling *and is reserved*: the census counts every local
id and the `fullIds` avoid-set (already used by the refs labeler) rejects
any label equal to a reserved id. A false negative costs a few tokens; a
false positive destroys a meaningful identifier. The invariant "no two
blocks ever share a served id" is pinned by a test independently of the
rule that produces it. Notably the wrapper had this predicate first
(`fullBlockIdRe = ^[0-9a-f]{24}$`) — the server's charset rule was the
accidental one.

**PUT refuses unowned ids instead of adopting them — SUPERSEDED by
§8.27, which removed PUT.** Reproduced live: GET(default) → PUT stored the
5-char labels AS the block ids, permanently — not the "re-mint visible in
diffStats" §8.25 first claimed. The guard (`checkPutBlockIds`) collected
the body's local ids (blocks, table columns/rows, views — the relabel
domain) and refused any id the object's own export-shape marshal did not
carry, before the creating resolvers ran. It was explicitly *the honest
interim* until PUT learned the unique-suffix resolution C4 permits; the
interim ended by deleting the surface instead, which is the stronger
form of the same fix — a channel that cannot take an id literally cannot
adopt one. What survives is `docLocalIds` and the create-side
`warnLabelShapedIds` warning, whose subject (a clone adopting labels) is
real and harmless.

**Partial reads are marked partial.** A `?block=` subtree read was a
schema-valid envelope with nothing marking it partial, and PUT of that
exact body deleted every block outside the subtree (reproduced:
`blocksRemoved: 5` on a 6-block page) while the equally partial outline
was refused loudly. The subtree envelope carries `"subtree": true` —
schema-invalid by `additionalProperties: false`, the way outline is
partial by construction — and create names it precisely before the schema
does. (The PUT half of the refusal went with PUT; the marker and the
create refusal stand, and the delete-everything-outside failure mode is
now unreachable by construction.)

**Create accepts a pasted read.** `POST /objects` 400ed on the `etag` of
every read shape while PUT stripped the same field. The create body goes
through the same envelope stripping (etag, warnings) and the subtree
refusal — and since §8.27 removed PUT, `normalizeCreateBody` is the only
place that stripping lives. Label-shaped ids (5 lowercase hex — every served label's exact
shape) ride a warning rather than a refusal: with no owned-id baseline a
label is indistinguishable from a rare authored 5-hex id, a clone of a
document that truly owns such ids must keep working, and adoption on a
fresh object breaks no other holder of the ids.

**No shape serves the refs legend.** The export shape kept the legend that
this work's own measurement shows as a pure loss on the measured corpus
(the §8.25 legend axis: it costs 0.9–11.5 % per document, 5.3 % on the
ref-heaviest row, and saves on none) — which left no shape offering full
block ids without the indirection §8.25 itself calls a write-back trap.
*(Corrected by the release review: this section first cited "+0.6 % on a
41-block ref-heavy document" — a figure with no traceable provenance; the
corpus has no 41-block document and its ref-heaviest row is the 22-block
R-20refs at 5.3 %.)* `?ids=full` now means full ids AND full inline refs.
Legend *resolution on input* is untouched (SPEC §9a is total). The claim
stays scoped to the corpus: §1.2's own model has the legend winning at
≥2× ref reuse, which the corpus rarely shows.

**`GET …/types/{key}` threads `?ids=`** so the export shape is one query
parameter away on types too; `dataview` needs no exemption because it is
not machine-minted.

**The wrapper retired client-side relabeling.** `relabelDoc` was not the
harmless no-op §8.25's era assumed: its 24-hex predicate matches nothing
in a server-labeled document, so `session.Labels` was never written —
making the §7.4 ambiguity retry dead code — while the `if labels != nil`
guard preserved a STALE map that rewrote a just-read label into a full id
from a previous document version (accepted by `matchBlockRef` as an exact
match; reachable through the CLI's persistent session file across an
upgrade). The label map is gone; refs pass through verbatim;
`retryAmbiguous` rebuilds its pool from the re-read document's own served
ids; and because the ambiguity rewrite happens after the Idempotency-Key
is minted, `LastWrite` records it (`PriorHash` + `Rewrites`) so an
identical re-run reproduces the rewrite and REPLAYS against the C8 store
instead of 409ing or re-applying under a fresh key.

**Recorded, deliberately unfixed:**

- `diffStats` under-reported a PUT identity rewrite on tables: reproduced
  4 added/4 removed while 3 row ids and 6 derived cell ids also changed
  identity. Any §8.25-era argument that "diffStats makes it visible" was
  therefore half-true on tables. *(Moot since §8.27: no surface performs a
  whole-document identity rewrite. The narrower fact — diffStats does not
  count derived table-cell identity — still holds and is still unfixed.)*
- Table columns **with at least one stored cell** never relabel — the
  column's 5-char tail is shared with every derived cell id in that
  column, so the census counts ≥ 2 — which means a served table mixes a
  5-char `row` with a 24-char `col`, `setCell` sends that mix back
  (resolveTablePart handles both), and such columns contribute 0 % of the
  label saving. *(Scoped by the release review — "structurally never" was
  overstated: a column with NO stored cells has no derived ids in the
  census and DOES relabel, verified live, so served column ids are 5-char
  or 24-char depending on sparsity.)*

**Numbers.** The §8.25 measurements were not re-run live (the running
desktop app predates this branch; reviewers reconstructed served bytes
with a branch-built harness) and are expected to hold within noise:
the documents that saved were minted-id documents, which still relabel;
the 0 % rows were readable-id documents, which still do not — now by
rule rather than by charset accident.

**Two trades of the minted-shape rule, unnoted when it landed.** The
outline got materially longer on readable-id documents — `ing-1` became
`notes-2024-meeting-1` — which matters because outline's whole value is
being the cheap map; readable ids are also the documents where the label
axis already saved 0 %, so the outline is where their cost now shows.
And the old charset rule *accidentally laundered* charset-dirty stored
block ids (an id the schema's `^[A-Za-z0-9_-]{1,64}$` pattern rejects
used to serve as its clean 5-char label); such ids now serve verbatim,
so a slightly wider class of documents fails its own `Validate` —
pre-existing data shape, no live producer found (ANOMALIES #12).

**Release pass (the fourth-lens review of this section's own work).** A
follow-up review of the hardening found five code defects, all fixed on
this branch with fail-on-revert tests:

- The `?block=` stored-id fallback mapped the resolved stored id back to
  a served spelling with a first-match-wins suffix scan — any earlier
  served id that happened to tail the matched stored id won (reproduced:
  `?block=<full minted id>` returned the unrelated block `b1`). The
  mapping is positional now (a second, uncompacted marshal of the same
  read — same block set and order, only spellings differ); stored ids
  never served (root, table wrappers, cells) 404, and a served-vocabulary
  ambiguity stays a refusal.
- The bullet this section previously recorded as *deliberately unfixed* —
  "`checkFreshIds` treats copied labels as fresh ids … cosmetic debris" —
  was **false as written**: "never relabel or alias afterwards" covered
  serving, not resolution. `matchBlockRef` returns on the first EXACT
  match, so an adopted label *captured the reference* — reproduced: read →
  `insertBlocks` with the read's own label → the next `replaceText` on
  that label edited the copy while the original silently lost it. PATCH
  now refuses any payload id that tails a kept block id, diagnosing the
  pasted-label shape.
- `docLocalIds` skipped table-cell **descendants** (the §6.1 F10 array
  form renders them as flat blocks with ids, in the relabel pool), so a
  body whose only minted id lived inside a cell PUT its label back 200,
  adopted permanently. It recurses now. The PUT consumer is gone (§8.27);
  the fix lives on in create's label warning, which is where the helper
  now sits.
- The wrapper's C8 record had no lower bound on its reuse window (a
  backwards clock step revived an arbitrarily old key and its rewrite —
  `LastWrite` persists in the CLI session file), judged the window from
  two `now()` readings that could disagree across the boundary (a
  rewritten body under a fresh identity), and kept only a single-level
  rewrite chain (`PriorHash` captured the already-rewritten hash on a
  second rewrite — reproduced double-apply). Floored, single-reading,
  and PriorHash-once + merged rewrites now.
- `POST …/types` 400ed on the etag of its own `GET …/types/{key}` read —
  this pass taught GetType to serve etag but never gave CreateType
  `normalizeCreateBody`. It normalizes like every other create now.

**Record corrections** (the commits cannot be amended, so the corrections
live here):

- `9e18ac568`'s message ("additive h1 entries for versions already listed
  — go-md2man, blackfriday — no dependency change") under-describes the
  diff: 10 lines across SIX module@version pairs. `urfave/cli/v2 v2.25.7`
  and `xrash/smetrics` were absent from go.sum entirely;
  `go-md2man v2.0.7`, `urfave/cli v1.22.17` and `sigs.k8s.io/yaml v1.3.0`
  are new versions (only blackfriday matches the message). No functional
  harm — go.mod is untouched and `go mod verify` passes — but the record
  was wrong.
- `e966b5bd2`'s message under-reports its yaml diff: 10 of the 17 hunks
  predate the claimed Q5 backlog (strict-bind prose and `413` responses
  originating in `b794b0c36`'s annotations, flushed here because the
  artifacts had not been regenerated since). And "the sorts id" is not in
  the artifacts at all — it lives in the `v2ViewSetPropDef` string
  literal served by `GET /v2/schemas/ops/{op}`, structurally invisible
  to swag.

### 8.27 PUT removed — snapshots are for creates, edits are ops (2026-08-10 — decisions as built)

`PUT /v2/spaces/{spaceId}/objects/{objectId}` replaced an object's whole
document: the body became a `SmartBlockSnapshotBase`, `NewDocFromSnapshot`
materialized it, and `history.ResetToVersion` diff-applied it against the
live object. **It is gone, whole.** This section records why, so the shape
is not re-proposed.

**The principle it leaves.** *Snapshots are for creates; edits are ops.* A
surface that requires materialising a WHOLE document to change part of it
is the wrong shape for this API — it pays whole-document cost in both
directions, it re-derives identity on every write, and it needs a repair
layer (`preserveEditorOwnedState`) to undo the damage its own round trip
causes. Approaches that lead back to snapshot generation on the edit path
are to be treated as a design smell, not a shortcut.

**Four reasons, in the order they weigh.**

1. **Token cost, both ways.** Changing one word cost ~2 400 tokens to read
   plus ~2 400 to write, against **33** for a PATCH `replaceText` op —
   two orders of magnitude, on the commonest edit there is (TOKENS §3's
   flow table, M-24blk: "GET default → PATCH `replaceText` by id =
   2 417 + 33"; PUT pays the 2 417 again on the way out).
2. **Its one distinguishing property was conditional and unexercised.**
   The id-matched minimal CRDT diff only helped a client that had
   preserved the stored block ids. A client that had not got a full
   rewrite — which `setProperties` + `replaceSubtree` already produce, on
   an id-addressed path that cannot silently mutate identity.
3. **No consumer.** Nothing in the tree called it: not the §7 wrapper
   (which excluded it by design), not the CLI, not the evals, not the
   integration tests. Only its own unit tests.
4. **It cost three review rounds.** Every id-identity defect of the Wave-0
   hardening is downstream of one asymmetry: **PUT took block ids
   literally while PATCH resolves them.** Labels adopted as stored ids,
   the cell-descendant hole, the `?ids=full`-before-PUT ceremony, the
   owned-vocabulary refusal (§8.26) — all of it exists to protect one
   surface from a vocabulary the rest of the API handles by construction.
   Removing the consumer removes the asymmetry.
   **Corrected by §8.29:** PATCH resolved its *reference* slots and took its
   *payload* slots literally, so removing PUT removed one literal channel
   and left three. The asymmetry was inside PATCH as well; §8.29 closes it
   there. The reason still stands — it was just not the whole account.

**Removed:** the route (`registerEditRoutes`) and its authz registry entry;
`PutObjectV2Handler` and its OpenAPI operation (`v2_put_object`);
`V2Service.PutObject`, `putPipeline`, `runEdit`, `normalizePutBody`,
`checkPutBlockIds`/`maxPutIdIssues`, `finishEdit`, `marshalForEdit`;
`apicore.ObjectMutator.ResetObject` (the port is single-method now) and the
adapter's reset implementation with `preserveEditorOwnedState`,
`preserveStructuralBlocks`, `copySubtree` and `isStructuralBlock`; and
`docCreateOptions.tolerateCorpseKeys` with its `anyRelationByKeyExists`
probe, whose only purpose was letting a GET→PUT of a corpse-held property
key round-trip (§8.23 cause 3). **That last removal was wrong and is
reverted in §8.29** — the tolerance belonged to the pasted-read-body case,
not to PUT, and create is where that case lives now.

**Kept, and re-framed.** `?ids=full` survives as the **backup/export
shape** and as the read to clone from — not as "the PUT read"; the
id-spelling ceremony that existed only to feed PUT is deleted from the
guides. `docLocalIds` and the `"subtree": true` marker are shared with
create and keep their create halves (`warnLabelShapedIds`; the subtree
refusal). The C11 write-safety guard loses its PUT exemption: its 422 now
says "edit it in the app" rather than "replace it wholesale with PUT".
`ensureIdempotency` keeps `PUT` in its METHOD set deliberately — it
classifies methods, not routes, so a future mutation method is covered by
construction.

**What this retires from the plan.** Wave 2.1's "teach PUT the
unique-suffix resolution C4 already permits" disappears with its subject,
as does the review-debt item "PUT and `POST /sets` still run whole-document
creating-resolver imports" for the PUT half (`POST /sets` still does).

**The one capability it nominally served — "clear the document and write
new content" — is owed a replacement.** Today that costs a `deleteBlock`
per top-level block. The named follow-up is a **range block-remove op** on
PATCH (APIV2_PLAN.md), so the clear-and-rewrite case is one bounded op plus
one `insertBlocks`, at op cost rather than document cost. Deliberately not
built here: removing the wrong shape first is what makes the right one
easy to specify.

**If a whole-document-replace client ever materialises**, it gets rebuilt
with id **resolution** from the start (`matchBlockRef`, as every other
write channel does) rather than inheriting the literal semantics that
caused these bugs.

### 8.28 The id-compaction mechanism is not agent-facing (2026-08-10 — decisions as built)

The compaction rule (§8.25) is now **absent from both agent guides**, and
that is deliberate rather than an omission.

`core/api/v2/SKILL.md` and `cmd/anytype/SKILL.md` used to explain it: that
machine-minted ids are served as short labels, that labels are derived per
read, and that a structural edit can change one. Every word was true and
every word was a liability — it hands a model a concept it must reason
about, in exchange for a case the model cannot act on differently. Small
models were the ones this cost most.

**What the guides say instead:** *use block ids exactly as a read served
them; if one is rejected as unknown, re-read.* One instruction, one
recovery, no mechanism.

**Why that is safe, not merely shorter** — three properties of the rule as
built, none of which the agent has to know. *(This list was written before
the §8.29 audit and two of its three claims were overstated; they are
restated here as what the code actually guarantees. The section stands, but
it stands on the corrected version.)*

1. **A label is never ambiguous — and a subset read cannot make one.**
   When two minted ids share a last-5 suffix, *neither* shortens
   (`mintedSuffixLabels`, the `counts[suffix] == 1` guard), and the census
   runs in `buildCompactIds` over the WHOLE snapshot before any slicing —
   so a `?block=` subtree read cannot hand out a label that the blocks it
   omitted also claim. This is the load-bearing half and it is sound.
   Pinned by `TestServedLabelsAvoidTailCollisions`, including the subset
   case.
   **Not** "the read never serves a block it cannot then resolve": that
   conclusion is false for cell descendants, which a default read serves
   with ids no channel resolves (F4 / §8.29). The guarantee is about
   *labels being unambiguous*, not about *every served id being
   addressable*.
2. **Every id slot resolves the same way — since §8.29.** `matchBlockRef`
   tries the exact id, then a unique suffix, in *reference* slots and
   *payload* slots alike. A full id, a served label and any unique tail are
   all valid everywhere.
   As originally written — "no write path depends on which shape a read
   chose" — this was **false**: the payload slots took the served spelling
   literally, so a document echoed back from a default read was renamed to
   its labels. It is true now, and it is true because it was fixed, not
   because it was restated.
3. **Staleness fails loud.** A label from an older read either still
   resolves or is refused as unknown — which is exactly what the recovery
   instruction answers. Unchanged, and now total: an unresolvable id in a
   payload is refused too, rather than silently becoming a new block.

**The residual, stated for the record and not for the guides:** a cached
label can retarget. Two paths, not one:

- **Collision plus deletion.** The block is deleted *and* a new block
  appears sharing that 20-bit tail. Inherent to any suffix scheme.
- **Retarget with neither** (missed when this section was written): a
  suffix match is resolved against the CURRENT document, so a label cached
  from an older read resolves to whatever now owns that tail. No deletion
  is needed and no collision is needed — only that the tail's owner
  changed. Post-§8.29 this is bounded by the ambiguity refusal (a tail with
  two claimants is a 400, never a silent pick) and by minting: new ids are
  24-hex random, so an accidental tail hand-off is ~2⁻²⁰ per minted block.

Documenting either to agents would cost more comprehension than the risk it
removes; the recovery instruction ("if an id is rejected, re-read") is the
agent-facing answer to both.

**`?ids=full` keeps exactly one framing:** the backup/export shape — the
read to archive or clone from. With PUT gone (§8.27) there is no write-back
read, so it is not an editing knob and the guides do not present it as one.

**If a future review finds the code and wants to "fix" the docs to match:**
this section is the answer — *the corrected version of it*. An audit
(§8.29) read the original three properties against the code and falsified
two of them; that is what this section is for, and being the standing answer
does not make it exempt from being checked. Check it again if you have
reason to. The mechanism belongs in §8.25 and in the API reference; it does
not belong in a guide whose readers are language models.

### 8.29 Audit round after the PUT removal: payload ids, corpse keys, honest hints (2026-08-10 — decisions as built)

§8.27 removed PUT on the stated grounds that no channel took block ids
literally any more, and §8.28 recorded three safety properties of the
compaction rule. An audit of both reproduced data corruption. Four
findings, fixed at their causes; every fix verified fail-on-revert.

**F1 — the PATCH payload slots took ids literally (data corruption,
reproduced).** Reference slots resolved (`matchBlockRef`: exact, then unique
suffix). Payload slots did not — they went to the format importer verbatim,
which reads an id as an identity. The documented loop

```
GET ?block=aaaa1                                   # default = compact shape
PATCH {"op":"replaceSubtree","id":"aaaa1","blocks":<that exact array>}
```

answered **200** with `blocksAdded: 1, blocksRemoved: 1` and permanently
renamed the stored `0000000000000000000aaaa1` to `aaaa1`. Consequences:
other clients' cached ids 404; the adopted id is not minted-shaped so it
never relabels again *and* permanently reserves that label in the
exporter's avoid-set; the CRDT records a delete plus a create where an edit
belonged. `updateBlock set:{rows:[…]}` did the same and reported it as the
innocuous `blocksChanged: 1`. `setCell value` did it to cell descendants.

The old guard (`checkFreshIds`) missed exactly these because
`keptBlockIds(leaving)` **subtracts the subtree being replaced** — so it
covered `insertBlocks` (where nothing is leaving) and skipped every op
whose payload replaces the label's own owner.

*Fix:* payload ids resolve like reference ids, against the pre-op
vocabulary **including the leaving subtree** (`payloadids.go`;
`v2EditDoc.localIds` is the vocabulary — block, row, column,
cell-descendant and view ids, the exact domain relabeling covers, shared
with create's `docLocalIds` so the two cannot drift). The loop above is now
*correct*, not merely refused: identity is preserved and a no-op echo is a
genuine no-op (`diffStats` all zero).

*Unmatched id — refused, not minted.* A payload id that resolves to nothing
is a 400 with the C6 hint naming both legitimate moves. Minting over it
would silently turn a stale or mistyped id into a new block — the same
silent-wrong-thing class F1 is about — and it would keep a literal channel
open for non-minted-shaped ids. Refusing costs a caller who meant new
content one edit (`id` is omitted, the server mints, `createdBlocks`
reports it), and it makes the rule total, which is what lets C4 finally say
"no channel takes ids literally" truthfully. This retires the one
affordance it removes: a client can no longer choose the id of a block it
creates through PATCH. Nothing in the tree used it (the wrapper authors via
`markdown`), and `createdBlocks` returns what was minted.

*Ambiguous suffix* is a 400 `ambiguous_input` listing the candidates.
*`checkFreshIds` is gone*, rewritten as `claimPayloadIds` — resolution
subsumed its tail-scan half, and two overlapping guards with different
coverage were the bug. *`createdBlocks` now reports only minted ids*: a
resolved payload position names something that already existed.

**F2 — the GET → POST clone broke on corpse-held property keys.** §8.27
removed `tolerateCorpseKeys` because "PATCH names only the properties it
edits, so live-only is now the whole rule". Both halves were wrong: PATCH
has its own in-document escape (`checkKey` passes any key already in
`doc.properties`), and **create** — the channel advertised as "a pasted read
body creates a copy" — became the only write path with no tolerance at all.
A `GET` of an object holding a value of a since-archived relation, `POST`ed
back, was a 400; with a live key spelled one character away the hint read
*"did you mean corpse_kez?"*, steering the caller to move the value onto an
unrelated property. *Fix:* `propertyKeyHeldByAnyRelation` on the create
path. It is a round-trip tolerance, never an address — nothing resolves a
corpse key to a property object and no listing advertises it — and a key no
relation holds at all is still refused. (Both ways of dying are covered: a
UI-deleted relation carries `isUninstalled`, an archived one `isArchived`,
and the explicit no-op `isArchived Condition:None` filter is what suppresses
the store's injected `isArchived: false` default so the second is visible at
all. Both arms are pinned.)

*The asymmetry left standing, on purpose.* PATCH `setProperties` still
refuses a corpse-held key the target object does not already carry, with the
same near-miss did-you-mean — so the F2 argument above ("the hint steers the
caller onto an unrelated property") applies verbatim to the PATCH
cross-object case, and it is not being fixed. The difference is what the
channel is FOR. Create's whole advertised loop is "a pasted read body creates
a copy" (§3(b)): the document the caller pastes is one this API served, and
the key is in it because the source object holds a value of that relation —
refusing is refusing our own output. `setProperties` is not a paste channel;
it names the properties an edit changes, one at a time, and a key the object
does not already hold arriving there is a caller writing a NEW value onto a
dead relation, which the near-miss hint is at least approximately right
about. The in-document escape (`checkKey` passes any key already on the
document) covers the round-trip half of PATCH — echoing a read body back —
which is the only part that shares create's justification. A later reader
should take this as decided, not overlooked: the day `setProperties` grows a
paste-shaped channel, the tolerance moves with it.

**F3 — the system-managed exclusion lost its only coverage.** The
`STRelation | STRelationOption | FileObject | Participant` switch in
`checkEditPreconditions` was pinned solely by a PUT test.
`TestPatchExcludesSystemManagedObjects` covers all four on the PATCH path.

**F4 — the 404 hints lied.** Both said *"GET the object with
`?outline=true` to list block ids"*. Cell descendants are served by a
default read (inside `rows[].cells[]`), resolve on **no** channel — not
`?block=` on either spelling, not `replaceText` — and the outline does not
list them (`blockIds()` and `exportShapeBlockIds` read only top-level
`blocks[]`). The guides' one recovery instruction therefore looped forever.
*Fix:* the hints now scope the outline's promise to the document's blocks
array and say outright which served ids are not block references (a table's
rows and columns are addressed by `setCell`'s `row`/`col`, a dataview's
views by the view ops, and a block inside a cell is not individually
addressable — rewrite its cell). Pinned by a property test: every outline
entry must resolve as a `?block=` reference, and the cell descendant a
default read serves must not be in the outline.

**Making cell descendants addressable is NOT taken here — ticketed.** What
it would cost, so the decision is a decision and not an oversight:

1. The block-ref vocabulary would move from `doc.blockIds()` to
   `doc.localIds()` — cheap, already built.
2. But `resolveRef` returns an **index into `doc.blocks`**, and every
   ref-taking op works on `doc.blocks[idx]`. Cell descendants have no entry
   there, so each of `updateBlock`, `replaceSubtree`, `moveBlock`,
   `deleteBlock`, `replaceText` and the `insertBlocks` targeting needs a
   second addressing mode routed through the owning table's re-import (the
   `setCell` path) — or the flat view has to promote cell descendants to
   first-class entries, which changes the served document shape.
3. `?block=` on a cell descendant would have to serve a *partial cell run*,
   a shape the AnyBlock envelope cannot express (a cell descendant is not a
   top-level block).
4. `?outline=true` would have to list them with a container marker, or the
   caller reads them as siblings of the top-level run.

The state side is already there — they are real blocks — so the entire cost
is in the addressing model and the served shapes. That is a format-level
decision, not a bug fix.

### 8.30 A field that cannot succeed does not appear in that op's schema (2026-08-10 — decisions as built)

§8.29-F1 made every PATCH payload id slot resolve through `matchBlockRef`
instead of being taken literally, and refused an id that resolves to
nothing. Correct, and it stays. But it finished a change of meaning that had
been half-made: after it, a payload `id` means exactly one thing — *name an
existing element and keep its identity* — and `insertBlocks`, whose payload
has no existing content to name, was left advertising a field in which
**every value is an error**. An id that resolves is refused as a duplicate
(`claimPayloadIds` with an empty allow-set); one that does not resolve is
refused as unresolvable. Both refusals are correct and neither is
reachable-by-repair: there is no third value.

**Why the runtime guard is the wrong instrument.** A refusal is a fine
answer to a caller who guessed wrong. It is not an answer to the consumer
this API is built for. A small model emits the fields it is shown, and under
constrained decoding it does so *by construction* — the grammar compiled
from the published schema had `id` in it, so `id` was always emittable and
the guard could only ever fire after the fact. The model has no channel to
learn from the 400 within the request, and the next request is generated
from the same grammar. The instrument that reaches a constrained
decoder is the schema itself: with `additionalProperties: false` (C13), a
field absent from the schema cannot be emitted at all. *(Overstated as "the
only instrument" here, and only half-built: as shipped this covered the
block's OWN id slot — the nested `columns`/`rows` were untyped arrays and
`views` was not a block property at all, so below the top level the runtime
guard was doing all the work. Corrected and the nested entries typed in
§8.31.)*

**The split.** The payload block def became two:

- `v2OpNewBlockDef` — **new content** (`insertBlocks`): no `id`, on the
  block or nested in `rows`/`columns` (typed as such since §8.31; `views` is
  not a block property on either shape, so nothing can name one here).
- `v2OpBlockDef` — **existing content** (`replaceSubtree`): keeps `id`,
  because naming the block being replaced is what makes echoing a read back
  a no-op instead of a rename (§8.29).

`updateBlock`'s `set.{rows,columns,views}` and `setCell`'s `value` are
existing-content payloads too — both allow ids drawn from the addressed
block's own subtree — and their schemas publish those channels untyped, so
they advertise no `id` to remove.

The runtime is unchanged in what it accepts: `insertBlocks` still refuses an
id. What changed is the verdict's meaning — it no longer resolves the id at
all (`rejectPayloadIds`), so it says *"id is not part of this op"* instead
of reporting a duplicate or an unresolvable reference, neither of which
names the actual repair.

**The rule going forward: a field that cannot succeed in an op does not
appear in that op's schema.** A runtime refusal is a backstop for the caller
who ignored the schema, never the mechanism. This is C2 — one concept, one
slot — applied one level down, to a *field*: `id` carried two meanings
("name this existing element" and "choose the id of this new one"), and the
two sharing one slot is precisely what produced F1. Splitting the slot is
the same move C2 makes on the surface's nouns.

**Boundary check.** Every id-bearing payload slot was re-derived from the
code rather than assumed. Table rows, columns and cell descendants are real
state blocks, so `claimPayloadIds` sees them and the always-error verdict
holds for them in `insertBlocks`. Dataview **view** ids are the one slot no
guard covered: they are not blocks, so an `insertBlocks` payload naming an
existing view used to import a *second* dataview holding that view id — no
refusal, a duplicate view id in the document. Rejecting the field closes
that for `insertBlocks` without a new guard — but only for `insertBlocks`:
the EXISTING-content payloads (`updateBlock set:{views}`, `replaceSubtree`)
still had no view-id guard at all, which is the seam §8.31 closes.
`updateView`/`insertView` `set.sorts[].id` is a
different id domain (a sort's own id, output-only on reads, accepted back so
a read round-trips) and is untouched.

**Not taken.** The unreferenced `$defs.block` that every op schema carries —
`setProperties` publishes a payload-block def it never `$ref`s — is noise,
not a trap: a decoder reaches only what the root schema references. Left
alone. The format's own document schema (`GET /v2/schemas/object`,
`pkg/lib/anyblockjson/schema`) is a different contract — a create body is a
snapshot, where an id is legitimate — and was not touched.

> **Superseded in part by §8.33 (2026-08-11).** The verdict below —
> *"the mechanism is unconfirmed"* — was correct for this probe and is no
> longer the last word. A three-arm A/B on `edit_text`'s optional `block`,
> varying **only** the published definition, moved emission from 11/11 to
> **0/13** by removing the field from the schema and left it at 8/8 when the
> field stayed and prose told the model not to use it. Schema shape changes
> behaviour; prose does not — on a field these models actually emit, which is
> the separation the control below could not produce. What §8.33 does **not**
> support is the inference that removal is therefore good: the arm without
> the field relocated the value into `object`, the only id-shaped slot left,
> and looped. Read the mechanism as confirmed and the *remedy* as
> field-by-field.

**Measured 2026-08-10: the removal is unregressed, the mechanism is
unconfirmed.** `cmd/apiv2eval -probe` put 210 real calls through the
published op schemas (`gemma4:e2b` and `gemma4:e4b`, three runs — 120 with
the example as published, 60 with it at op level, 30 with the discriminator
diagnostic; artifacts in the gitignored `eval-out/`). **Zero** payload `id`
emissions, at any path, in all 210:

| arm | payloads | schema publishes a payload `id`? | `id` emitted |
|---|---|---|---|
| `insertBlocks` | 140 | no (this section's change) | 0 |
| `replaceSubtree` | 70 | **yes** — and half of them come from the `echo_block_existing` case, which quotes a read block WITH its id and asks for the replacement *"keeping the block's identity"* | 0 |

The second row is the control, and it is what limits the claim. These models
did not write a payload `id` **whether they were shown one or not**, so the
two arms did not separate and the probe cannot say the schema is what stopped
the first arm. What it does say is that the removal cost nothing: no
generation needed the slot it lost. The argument for §8.30 therefore rests
where it needs no behavioural claim at all — a field in which every value is
an error is an incoherent thing to publish, whatever any particular decoder
would have done with it. The paragraph above still reads *"the instrument
that reaches a constrained decoder is the schema itself"*; treat that as the
design's reasoning, not as a measured result. *(§8.33 measured it: on a
field the models do emit, the schema is what decides — so that sentence is
now a result and not only reasoning. The removal being the right response
to it remains a per-field judgment, and §8.33 is the case where it was
not.)*

Where the rule *is* argued by measurement is its second instance: `position`
on `insertBlocks` was published with a description that made it read inert,
and in the no-target shape **every** value of it was a 400 — a shape
`gemma4:e2b` produced on 20 of its payloads, 10 of 10 in each of the two
cases that reach for it. That is the same rule catching a second field, and
it was fixed the other way round — §8.32 gave the field a meaning instead of
removing it, because both of its values named a real intent.

### 8.31 The two halves of the id rule disagreed about what "exists" means (2026-08-10 — decisions as built)

§8.29 made every PATCH payload id slot RESOLVE, and §8.30 removed the slot
from the ops where no value of it can succeed. Both are about what an id may
*name*. The other half of the rule — what an id may *claim* — was never
brought along, and the two halves were reading different documents:

| | resolution (`payloadids.go`) | collision guard (`claimPayloadIds`) |
|---|---|---|
| domain | `doc.localIds()` — blocks, table columns, rows, cell descendants **and dataview views** | `a.st.Exists` — **blocks only**, because it walks the imported `[]*model.Block` and a view is not an element of it |

A dataview **view id** therefore resolved but could never collide. Two
reproduced consequences, both 200 before:

**(a) Duplicate view ids sailed through.**
`updateBlock {"id":"dataview","set":{"views":[{"id":"viewAll1",…},{"id":"viewAll1",…}]}}`
stored a document holding two views under one id. `matchViewRef` returns the
first exact match, so a later `updateView {"view":"viewAll1"}` renamed only
view #1 — and every subsequent view op addressed view #1 **forever**; the
second was unreachable for the life of the object. A column or a row in the
identical position was refused correctly, which is what makes this a seam
rather than a policy: the guard covered every id slot that happened to be a
block.

**(b) A block could adopt a view's id.**
`replaceSubtree {"id":"blockPara1","blocks":[{"id":"viewA1","type":"paragraph"}]}`
stored a paragraph whose id equals a live view's — reachable by compact
suffix too, since view ids are in the relabel pool (§9a). Damage is bounded
today (block refs and view refs resolve against different lists) but the
document holds an identity collision no read can distinguish, and "bounded
today" is not a property to ship.

**Fix at the API layer.** `payloadIdExists` is now the one answer to "does
this id already name something in this object", and it is the **union** of
the two views above: `a.st.Exists` (which sees the state blocks the served
document does not carry at all — root, title, header) **∪** the pre-op
`doc.localIds()` (which sees the doc-local ids that are not blocks — views).
`claimPayloadIds` claims view ids alongside block ids, so a duplicate within
one payload is caught, and `collectSubtreeIds` — the "ids this op may reuse"
set — now carries a dataview block's view ids, because an op that replaces a
dataview block replaces the identities it holds and echoing its views back
must keep them. Both halves ask one question of one domain; there is no
remaining slot where the resolver and the guard disagree.

**And at the format layer.** `anyblockjson.Validate`'s uniqueness domain
(`claimId`) covered blocks, columns, rows and derived cells but not
`views[].id` — so a duplicate view id was invalid-but-unvalidated on **every**
channel, create and import included, not only PATCH. It is now checked in
`checkDataviewViews`, which means (a) is refused by the fragment import
before the API guard is even reached. That ordering is the point of fixing
it there.

**The uniqueness scope chosen for view ids: within the dataview BLOCK, not
the document.** This is the only id domain in the format that is not
document-wide (SPEC §4), and both directions were checked against the code
rather than assumed:

- *Why not narrower.* Every consumer resolves a view reference within ONE
  dataview's `views` list (`matchViewRef` over `viewIdList(views)`; the
  client's tabs), and the per-view editor state — `groupOrders`,
  `objectOrders` — is keyed by view id **inside the same
  `BlockContentDataview`**. A repeat inside one block makes the second view
  permanently unaddressable, which is exactly bug (a).
- *Why not document-wide.* It would reject data the app itself produces.
  `template.MakeDataviewContent` mints the first view of every set,
  collection and type with the literal id `"default"`, and
  `dataviewservice.CopyDataviewToBlock` copies a target object's views
  **verbatim** into an inline dataview block — so a page with two inline
  collections legitimately holds two views called `"default"`. A
  document-wide error would fail on real exports.

**Why the API layer is nevertheless stricter than the format**, and why that
is not an inconsistency: the format lets a view id collide with a *block* id
(different domains), while the API refuses it. The API's own id vocabulary
**is** document-wide — one compact-relabel pool (§9a) and one payload
resolver whose `localIds()` deduplicates — so it declines to *create* a
collision it could not later describe to a reader. It does not retro-refuse
one it is shown: a document that already holds one still reads and still
resolves.

**The receipt the refusals promise is now delivered.** Both
`unresolvedPayloadIdError` and `newContentIdError` tell the caller to omit
the id because *"the server mints one and returns it in `createdBlocks`"* —
but `createdBlocks` was written only in `decodePayloadRun`, for **top-level
run blocks**. A minted view id, a minted cell descendant, and the row/column
ids of a table created through `insertBlocks` were all unreported — and
those are precisely the slots the refusals fire on (the §8.30 test asserts
path `ops[0].blocks[0].rows[0].id`). Reporting was chosen over rewording:
the alternative costs a model that must re-read to learn an id it just
created a whole round trip, and the fix is one walk. Minting moved from the
format importer into the API's slot walk, so every empty id slot is filled
and reported under **its own payload path** — `ops[0].blocks[0].rows[1]`,
`ops[0].value[1]`, `ops[0].set.views[2]`. Views go to `createdViews` (a view
is not a block; that map already existed for `insertView`), everything else
to `createdBlocks`.

**One walk, three passes.** Resolution and rejection used to be two hand-kept
walkers over the same slot set (`resolvePayloadBlock` / `rejectPayloadIds`),
with a comment promising they could not drift — and the drift that mattered
was a third participant, the guard, walking something else entirely.
`walkPayloadIdSlots` is now the only walk; its visitors are the three things
a slot can need (resolve, reject, mint).

**§8.30's overclaim, corrected.** It argued the schema is *"the only
instrument that works against a decoder that emits what it sees"*, and
`v2OpNewBlockDef` said *"no id slot — neither here nor inside
rows/columns/views"*. Neither was true of the nested slots: the served
`columns`/`rows` were `{"type":"array","maxItems":N}` with **no `items`**,
and `views` is not a property of the block def at all. The schema constrained
the top-level slot and nothing below it; the runtime guard did all the work
there, and the new test's `schemaPropertyOwners` found no nested `id` only
because there was no nested schema — the same assertion would have passed for
`replaceSubtree`. *Fix:* the nested entries are typed. `columns` and `rows`
publish `items` defs that are themselves `additionalProperties: false`, and
the id slot inside them follows the same §8.30 split as the block's own —
present on `v2OpBlockDef`, absent on `v2OpNewBlockDef`. What is **not** typed
is the interior of a cell run (string | null | object | array of blocks —
recursive, and a strict recursive def is a real cost to a constrained
decoder); there the runtime guard is the instrument, and the cell
description says so rather than implying otherwise. `views` is published on
**neither** shape, so no payload block can name a view through this channel
at all — the corrected claim, and what the block descriptions now say. The
test asserts the nested defs exist, are strict, and carry an `id` exactly on
the existing-content shape, so it fails both if a nested slot regains an id
and if the nested defs are removed again.

**The new-content op set is one set.** The runtime literal `"insertBlocks"`
and `opSchemaNewContent(…)` were independent statements of the same fact.
Runtime-without-schema is merely strict; **schema-without-runtime re-creates
§8.30's exact bug** — an op publishing an id no value of which can succeed.
`v2NewContentOps` (ops.go) is now read by both: `decodePayloadRun` picks the
rejecting visitor from it, and `opSchema` picks the payload-block def from
it. `opSchema` takes the op NAME as its first argument for the same reason —
the `op` const, the required `op` field and the block def all derive from it
instead of being spelled three times. The schema test is table-driven over
`v2NewContentOps` × `v2OpNames`, so an op added to one half and missed in
the other fails.

**What the seam says about the design.** The id rule was stated once and
implemented twice, and the second implementation was a *by-product* — a
guard written when payload ids were taken literally, kept when they started
resolving, never re-derived against the vocabulary the resolver had grown.
The rule "ids resolve against `doc.localIds()`" was true of one half and
false of the other for two releases, and nothing in the type system, the
tests or the prose could see the disagreement because each half was
individually correct about its own domain. The structural answer taken here
is to give the domain a NAME (`payloadIdExists`) that both halves call,
rather than two correct-looking expressions; the same move as `localIds()`
being shared by create's warning and the PATCH resolver. Where a rule has two
enforcement points, the shared thing should be the predicate, not the
sentence in a comment saying they agree.

### 8.32 Three defects a small model found that five review rounds did not (2026-08-10 — decisions as built)

§8.24–§8.31 were six passes of design review over the same surface, by
readers who know what every field means. `cmd/apiv2eval -probe` put 210 calls
from `gemma4:e2b`/`e4b` through the schemas the product actually publishes,
with no live API and no repair loop, and the transcripts named three defects
in an afternoon. None of them is subtle in hindsight. All three are the same
kind of thing: **the surface was reviewed as a specification and consumed as
a prompt**, and a reader who already knows the answer cannot see a field that
reads wrong, an example that contradicts its schema, or a vocabulary that was
never published. A generator has no other information, so it sees exactly
that and nothing else.

**F1. `position` with no targeting field was a guaranteed 400 — now it names
an end of the document.** `resolveTarget` refused it: *"position without a
targeting field is meaningless"*. The published description said *"with
`inside` only; default last"*, which reads to a model as *last is the
default, so naming it is harmless* — and `gemma4:e2b` wrote exactly

```json
{"markdown":"## Risks\n- Vendor delay","op":"insertBlocks","position":"last"}
```

on 20 payloads: 10 of 10 in *add a section at the end*, 10 of 10 in *copy
this block as new content*. Every one a 400 at `ops[0].position`.

Accepting-and-ignoring was refused. `first` and `last` are **different
intents**, and silently discarding one would do the wrong thing for that
one — the silent-wrong-action class five rounds have been spent removing.
The field was made meaningful instead: with no `after`/`before`/`inside`,
`position` picks which end of the DOCUMENT, `first` the start and `last` (or
absent) the end. That turns a guaranteed refusal into the obvious reading,
and it gives *"insert at the beginning"* an expression that previously
required reading the document first only to learn the first block's id.
`moveBlock` shares `resolveTarget` and gets the same meaning: one vocabulary,
one meaning, in both ops that use it.

**Root-first is not the state root's `InnerFirst`.** `InsertTo("")` targets
the state root, whose FIRST child is the structural header — title,
description, featured properties — which SPEC §7 keeps out of the served
document. Prepending there lands the block *above the title*, and nothing
repairs it until the object is next initialised (`template.RequireHeader` /
`normalizeTree` run at open, not at apply). The start of the *document* is
"before the first document block" — the same slot `before: <first block>`
names, needing no knowledge of the header at all — with the append as the
fallback when there is no document block to sit before. `rootTarget`
(stateops.go) is that one resolution, and `moveBlock` passes it the moved
subtree to skip, so `position:"first"` on the block that is already first
anchors against the block *after* it instead of failing InsertTo's
"blockIds contains target" or, worse, falling through to the append at the
other end.

The refusal that remains is the one the anchor makes redundant: `position`
alongside `after`/`before` is still `validation_failed`, because there the
anchor already names the slot. An out-of-enum value is still refused too —
`checkPosition` is now one check for both placements.

**F2. Every `GET /v2/schemas/ops/{op}` served an example its own schema
rejects.** All 14. The `schema` describes ONE op — `additionalProperties:
false`, `op` required with a `const` — and the `example` was a whole request
body, `{"ops":[{…}]}`. The example beside the schema was therefore not an
instance of it, and a consumer that reads the pair together (the small
consumer §5 built this route for) got two contradictory shapes. Measured:
with the wrapped example, `gemma4:e4b` omitted the required `op` field on
**9 of 60** calls (`e2b`: 10 of 60); with the example unwrapped to op level,
**0 of 60** — and 0 of 30 in the follow-up run.

Examples are now the op object itself. The pin the wrapper manifest has had
all along (`TestExamplesAcceptedByOwnGBNF` — every served example must be in
the language of the served grammar) is now on this route too, table-driven
over all 14 ops and compiling the served schema with a real validator, so a
new op cannot land with an example its own schema refuses. The eval harness's
`TestServedOpExampleIsNotAnInstanceOfItsOwnSchema`, written to *document* the
defect, is inverted rather than deleted: it now asserts the instance
relation, and the harness's `-probe-example` knob no longer separates two
shapes because both are the same bytes.

**F3. The payload block's `type` published no vocabulary.** It was
`{"type":"string","maxLength":64}`, beside a block description pointing at
`GET /v2/schemas/object` — a fetch a decoder cannot make. Asked to add a
checkbox item, `gemma4:e2b` answered

```json
{"type":"bulletedListItem","text":"[ ] Follow up"}
```

**10 times out of 10**: a plausible type plus a literal markdown checkbox in
the text, which is what inventing a vocabulary looks like. `checkbox` is a
real type; it was simply never shown.

The enum is now published on both payload-block defs, and it is **derived,
not copied**: `anyblockjson.BlockTypeNames()` reads the names out of the
format's own embedded JSON Schema (`$defs.blockCore.properties.type.enum`) at
startup, and `AuthorableBlockTypeNames()` subtracts the §7 structural types —
`title`, `description`, `featuredProperties` — which import absorbs into
properties or drops, so a payload naming one produces no block. That
subtraction is §8.30's rule one level down, applied to a *value*: an enum
must not offer a value no caller can succeed with. The structural set is the
same map `topLevelBlocks` (import.go) switches on, so "which types are
structural" is stated once. A hand-copied list is the drift class §8.31 was
about; there is no second list here to drift.

**The token cost, on the record.** 39 names in the format's enum, 36
published. Measured with the Gemma 3 tokenizer (ollama `prompt_eval_count`,
delta method so the chat template cancels): the `type` property goes from
**12 to 100 tokens** — **+88** — and a served op schema from ~867–945 to
~956–1034 tokens, **+89 each, +9.4% to +10.3%**. That is paid on all 14 op
schemas, because `$defs.block` is carried unconditionally (§8.30 "Not
taken"); 12 of the 14 never `$ref` it, so a consumer that fetches every op
pays ~1.2k tokens for an enum most of those schemas do not reference. Making
`$defs` conditional on reference would recover that and about 1.5 KB per
non-referencing op besides — a bigger win than this enum is a cost, and
deliberately left as its own change rather than folded in here. Against the
cost: the vocabulary is 10% of the schema and it is the 10% without which the
`type` field is a guess.

**What the three have in common, and what it says about review.** Each was
a *published artifact* that a specification reader completes from knowledge
they already have — "position obviously needs a target", "the example is
obviously illustrative", "the type list is obviously at the other endpoint" —
and that a generator completes from the bytes in front of it. Five rounds of
careful review did not find them because review reads for *correctness of
meaning*, and all three were meaning-correct; they were wrong as *stimulus*.
The cheap instrument for that class is not another reading. It is 210 calls
through the published bytes with no repair loop, which is a couple of hours
of a small model's time. Where the earlier rounds did land is in what made
these fixes short: the derivation points already existed (`v2NewContentOps`,
the vocabulary exports of §8.17), so each fix is a wiring, not a new list.

**Not taken.** Neither `-probe` case set nor the model host was re-run to
measure F1 and F3 after the fix (the full matrix is a 3-hour serialized job)
— F1's before/after is pinned by unit tests and by the live surface, F3's by
the enum being the format's own vocabulary. The `position` semantics are
unchanged for the `inside` placement, and no other op's targeting vocabulary
was touched.

### 8.33 The A/B that settled §8.30's mechanism, and three defects a live run found (2026-08-11 — decisions as built)

§8.32 read the surface as a prompt with no live API behind it. This run put
`gemma4:e2b` through the product's own MCP wrapper against the real local
server — 60 attempts, four tasks, temperature 0, artifacts in the gitignored
`eval-out/attempts.jsonl` (run `20260810-222855`). The A/B answers the
question §8.30 could not.

**Every number in this section is `gemma4:e2b`.** The next run added
`gemma4:e4b` and it scores far worse on the wrapper — for one string-handling
reason that has nothing to do with the wrapper's shape, and that this section
predates. Read §8.34 before drawing a model-vs-surface conclusion from either. The three defects are what a *loop* exposes that a
schema read cannot: each needs a tool call to have happened before the next
one goes wrong.

#### The A/B: schema shape changes behaviour, prose does not

Three arms differed in **one thing** — the `edit_text` definition published
in `tools/list`. The runner behind them was the same object, so a `block`
sent to an arm that publishes none still works and is still counted.

| arm | `edit_text` definition | calls | …with `block` |
|---|---|---|---|
| `ab/a-shipped` | as shipped: `block` optional | 11 | **11** |
| `ab/b2-prose` | `block` published, description says the snippet locates it and to prefer that | 8 | **8** |
| `ab/b1-noblock` | `block` removed from the schema | 13 | **0** |

The field is supplied on **every single call** where the schema shows it,
including the arm whose prose argues against it, and on **none** where the
schema does not. This is the separation §8.30's 210-call probe could not
produce: there the control field (`replaceSubtree`'s payload `id`) was not
emitted *whether it was shown or not*, so the arms never diverged and the
probe could only report that the removal cost nothing. Here the field is one
these models reach for by default, and it moves 11/11 → 0/13 on a schema
edit and 11/11 → 8/8 on a prose edit. §8.30's *"the instrument that reaches a
constrained decoder is the schema itself"* is now measured, not argued.

**And `block` stays published.** The arm that removed it was worse on
everything the removal was supposed to help. Wasted reads rose 8 → 19. With
no `block` slot, the model put the block label in `object` — the only
id-shaped argument left — and then, on the useless refusal that produced
(defect 2 below), repeated the identical call three times and abandoned:
`identical_repeat=3`, a signal that appears in **no other arm**. Removing a
field does not remove the impulse to supply the value. It relocates it, and
`object` is a worse home for a block reference than `block` is. The rule
§8.30 states is about fields *in which every value is an error*; `block` is
not that field — every value of it can succeed — so the rule never applied
to it, and the A/B is evidence about the **mechanism**, not a licence to run
the removal anywhere the mechanism operates.

**The success rates are noise and are not cited.** 60%/80%/85% across the
arms, n=5 per cell. The two tasks that never call `edit_text` at all
(`append-section`, `set-property`) varied as much between arms as the two
that do. Nothing in this section rests on them.

#### Defect 1 — a `find` with no criteria was not a search, but said it was

`find {"space": …}` with no `query`, `type` or `filter` matched nothing and
rendered *"78 matches"* over a numbered handle list. **12 of 67 find calls in
the run were this shape** — just under a fifth of all searches, and every one
of them on a task whose prompt names the note by title, so every one a
dropped argument rather than an intent to browse. What the model did with
the result:

- 4 attempts addressed handle 1 — an arbitrary object. Three read it,
  noticed, and re-found;
- **the fourth read it and then wrote to it**: three blocks appended to
  `Kimubabe` while the task named `Takosize`, reported as done;
- 2 attempts stopped on the bare `find` and claimed success having written
  nothing;
- 9 of the 12 re-ran `find` *with* a query unprompted, which is the repair
  already being in the model's repertoire.

The receipt worked and did not help: `ok — "Kimubabe": 3 added` named the
wrong object plainly and the model read past it. A receipt is a record, not
a control.

**Decided: a bare `find` lists, and a listing assigns no handles.** The
alternatives were refusing the call outright, or leaving it and steering
harder.

Refusing was rejected on the B1 finding above: the impulse relocates.
`find {space, type:"page"}` is a legitimate search that returns the same
arbitrary first row, so a refusal that only bans the bare shape moves the
mistake one argument sideways and makes it un-refusable. Steering harder was
rejected on the B2 finding: the MCP instructions already say *"find (space +
query/type/filter)"* and the model dropped them anyway.

What is left is changing what the call *produces*. A bare `find` now returns
the space's objects **unnumbered**, under a first line stating that nothing
was searched for, and clears `Session.Handles`. The browse intent is still
served — you can see what a space holds, which is a real thing to want — but
nothing the listing returns can be passed as `object`, so the wrong-object
write is unreachable rather than discouraged. A handle used afterwards earns
its own refusal naming the repair, and *not* `errNoSession`'s "run find
first", which would be false (find has run) and reads as a repair already
attempted. Any one of the three criteria makes it a search again, numbered
as before.

The name the act gets is in the output, not in a thirteenth tool. Adding one
costs every model on every call — schema tokens, one more row of selection
pressure, and the set sits deliberately under the >15-tool cliff — to serve
an intent that is one degenerate argument shape of a tool already present.
What failed was never that browsing was unavailable. It was that browsing
looked like matching, and that is fixed where it was broken: in the
rendering and in the handle table.

#### Defect 2 — the `object` refusal named no repair

`edit_text {"object":"767cb", "find":"Q3", "replace":"Q4"}` earned `object
"767cb" not found in space "bafyrei…"`. True, and inert: it names the
failure and no repair, and the model sent it three more times before giving
up. This is H3's worst case, and the run's own numbers say the rest of the
surface does better — refusals that name a field get `fixed_named_field`,
`switched_to_read`, `switched_tool`; only this one produced
`identical_repeat`.

Three shapes arrive in `object` and each has a known repair: a block
reference the model just read (5 in the run), the **space** id (6 in the
run), and an object's name. The wrapper now says which:

```
object "767cb" not found in space "bafyrei…" — that is a block reference:
read serves those, and they go in `block`. `object` takes a handle number
from the last find (1, 2, …)
```

Three properties of how it is built, each deliberate:

- **It appends, never replaces.** The server's 404 is the fact; the hint is
  the repair. Dropping the fact would hide which reference failed.
- **It runs on `Run`'s error path, not inside an executor.** The mistake is
  a property of the *argument*, so every tool taking `object` gets the steer
  by construction — `read`, `set_properties`, `move_block`, all of them —
  and a new tool taking `object` inherits it without a line of code.
  Fixing it in `runEditText` would have fixed one call site of nine.
- **It fires only after the server has said not-found**, and only when the
  server's message names the caller's own reference. A pre-flight shape
  check would have to prove that no legitimate object id is 5–24 hex
  characters; the post-404 hint needs no such proof, because the 404 has
  already established the value is not an object. The cost is one HTTP round
  trip on a mistake, which is nothing.

#### Defect 3 — `describe` answered a different question than it was asked

Told to set a description, the model ran `describe`, did not find one, and
answered *"The note type does not have a 'description' property, so I cannot
set it."* The API does it in one call.

The cause is that `describe` read the type's **recommended** lists — what a
client renders in its properties panel — and served them under *"use these
exact property keys … in create and set_properties"*. Those are different
sets, and they disagree in both directions. Live, `page` recommends
`createdDate` and `creator` (which `setProperties` **refuses**, SPEC §4a)
and `backlinks`, `links`, `lastModifiedBy`, `lastOpenedDate` (which it
accepts and silently ignores — see below); it recommends neither `name` nor
`description`.

**Both of the two properties every object has were invisible.** `description`
is in the space's property index and no type names it. `name` is worse: it
is `hidden` in the bundle, so `GET /v2/spaces/{s}/properties` — which
excludes hidden relations — does not list it either, and no type recommends
it because clients render it as the title. It was reachable from no read of
the surface at all, on the one tool whose job is to answer *what can I set*.

`describe` now reports the settable set: the type's own properties first
(the curation is real signal and its order carries it), then the rest of the
space's property index, then a line naming the output-only keys as
read-only rather than dropping them — a caller who saw `createdDate` in a
read and cannot find it in `describe` would reasonably conclude `describe`
is incomplete, which is the defect this rendering exists to close. `name` is
added from a small always-settable list, since neither source can produce
it.

The output-only set is now stated **once**, in `v2model`, and both layers
read it: the service refuses a `setProperties` naming one, and `describe`
must not advertise one as settable. A second copy in the wrapper is the
drift class §8.31 was about.

Option lists stay bounded to the type's own selects — one HTTP call each,
and fetching them for a whole space's index would turn `describe` into
dozens of round trips for values the A2 guard already names on refusal.

**The token cost, on the record.** Measured with the Gemma 3 tokenizer
(ollama `prompt_eval_count`, delta method), `describe page` in the eval
space goes from **76 to 281 tokens — +205**, for 38 space properties. It is
paid once per task, against a ~10k-token attempt, and it buys the difference
between a tool that answers the question and a tool that produced a
confident refusal of a possible task. The cost scales with the space's
property count; the off-type listing is bounded at 120 rows and degrades
into a count past that.

#### Also found, filed not fixed: SPEC §4a's output-only set is too small

Verified live against the running API: `setProperties` accepts
`backlinks`, `links`, `lastModifiedBy` and `lastOpenedDate`, answers **200
with `propertiesChanged: 0`**, and writes nothing. They are derived, marked
`readonly` in the bundle, and absent from `v2OutputOnlyPropertyKeys`. That
is an accepted write that does nothing, reported as success — the
silent-wrong-action class these rounds have been spent removing, and the one
place it survives. It is four of the six rows `page`'s recommended list
leads with, so it is also what `describe`'s first section now shows.

Not fixed here: widening the refusal set turns previously-200 requests into
400s on the REST surface, which is a contract change that deserves its own
decision rather than a fold-in to a wrapper fix. `PropertyRow` carries no
read-only flag either, so the wrapper cannot mark them without the server
saying so first — the fix is server-side in both halves.

#### What the spot checks did and did not show

One live attempt per fix (`gemma4:e2b`, one task, one attempt, run
sequentially). All three passed — and **none of the three reached the
failure point**: the model supplied its `query`, put its block ref in
`block`, and set `description` without calling `describe` at all. They are
evidence of no regression on the changed surface and nothing more; a
sample of one on a high-variance model is not a rate, and is not offered as
one. The before/after that actually pins each fix is deterministic — the
live CLI against the running server, and a unit test per fix that fails when
the fix is reverted (all three reverts run and confirmed).

### 8.34 A refusal that is correct and unactionable is a defect — stated once (2026-08-11 — decisions as built)

Run `20260810-235748`: 110 attempts, `gemma4:e2b` and `gemma4:e4b`, three
surfaces, temperature 0, artifacts in the gitignored
`eval-out/attempts.jsonl`. It looked like a capability result and was a
string-handling quirk.

| model | arm | passed | of |
|---|---|---|---|
| `gemma4:e2b` | wrapper/small | 18 | 20 |
| `gemma4:e2b` | wrapper/large | 21 | 30 |
| `gemma4:e2b` | ops | 23 | 30 |
| `gemma4:e4b` | wrapper/small | 3 | 8 |
| `gemma4:e4b` | wrapper/large | **2** | **12** |
| `gemma4:e4b` | ops | 8 | 10 |

The obvious reading — the bigger model is worse at the wrapper and fine on
the raw op surface — is wrong twice over.

#### The finding: one string, mangled in one argument

A space id has two dot-joined parts:
`bafyreihwvsaekzzyb54o7um4hdpvpn5b2invn75lmijhhtghblvphxwz2i.28y6mgnwgodt7`.
**`gemma4:e4b` truncates it at the dot**, passing only the prefix — plausibly
reading `.28y6mgnwgodt7` as a file extension. It earned:

```
space "bafyreihwvsaekzzyb54o7um4hdpvpn5b2invn75lmijhhtghblvphxwz2i" not found — list spaces with GET /v2/spaces
```

**74 of the 79 `find` calls in e4b's 15 wrapper failures are this** (83 of 93
across all 20 of its wrapper attempts). Every single truncation is in
`find`'s `space` argument; **not one** appears in any other argument of any
other tool. `gemma4:e2b` never does it, in any arm. In one attempt e4b called
`spaces`, was served the full ids, and went straight back to the truncated
form — the identical call **seven times**, until the turn budget ended the
attempt. Ten of the fifteen failures stopped on `turn_budget`, against zero
for e2b anywhere in the run.

**And the ops arm is not a control.** Its tools take no `space` and no object
id at all — the harness binds the target, and across all 39 ops-arm calls in
the run the argument names are `op`, `id`, `find`, `replace`, `blocks`,
`row`/`col`/`tableId`, `value`, `position`, `set`, `markdown`, `after`,
`outline`, `recursive`. e4b scores 8/10 there because it never has to handle
a space id, not because it handles one well. The two arms differ in the
argument, not in the difficulty of the work.

#### Fix 1 — the refusal names the mistake, with the full id in it

A rejected space id that is a **prefix of a known space id** is a
recognisable error with a known repair, and the model already has everything
else right. The wrapper now answers:

```
space "bafyrei…z2i" not found — that is the first part of the space id
"bafyrei…z2i.28y6mgnwgodt7": a space id has two parts joined by a dot and
BOTH are part of the id — pass it whole, exactly as `spaces` prints it
```

Built the same way §8.33's `object` steer was, for the same reasons: it
**appends to the server's 404** (the fact is which value failed, the hint is
the repair); it runs on `Run`'s **error path**, so all three tools taking
`space` — `find`, `describe`, `create` — are covered by construction and a
fourth inherits it; and it fires **only after the server has refused**, so
nothing has to be proven impossible up front. The cost is one `GET /v2/spaces`
on a mistake and zero on a working call (asserted). A value that is nobody's
prefix, or an unreadable space list, leaves the server's refusal exactly as
it was written.

One thing is new: the specific repair **supersedes** the generic one. The
message no longer also says "list them with the `spaces` tool", because two
repairs in one refusal compete and this run shows which one loses — the model
had already run `spaces`. That is a declared field on the steer table
(`supersedes`), not a special case in the code.

**Object ids do not share the hazard**, and the check is worth recording.
Object ids are CIDs, `_bundled` keys, `_date_…` keys or participant ids, and
the two derived ids that *do* embed a space id — `NewParticipantId` and
`NewPersonalWidgetsId` (`core/domain/id.go`) — **re-encode the dot as an
underscore**, with the comment "to avoid issues on Desktop client". The one
dotted identifier on this surface is the space id. What can still happen is
the truncated space id landing in `object` instead of `space`, so the
`object` steer recognises that too (a strict prefix of the working space, 16
characters or more — two CIDs in the same multibase share about eight).

#### Fix 2 — the wrapper stopped speaking REST to a tool caller

The refusal above ended in *"list spaces with GET /v2/spaces"*. On the HTTP
surface that is right: routes are its vocabulary. On the wrapper surface the
tool is `spaces`, the model **had already called it**, and the hint named a
thing it cannot do while leaving the thing it can do unnamed.

The audit found this was never one message. Every hint the server writes for
a route the wrapper calls reaches a tool caller verbatim, and the wrapper had
exactly two ad-hoc rewrites — one in `describe` for two type-key phrasings,
one in `opsVocab` for a block hint the server **no longer sends** (§8.29
rewrote it, and the rewrite was not followed here, so the current phrasing
leaked). Reachable and unhandled: the space hint, the type-key hints on
`find`/`create`, the property-key hint, the option-names hint, the current
block hint on every editing tool, the members hint behind `@me`.

So the vocabulary is one table now (`steer.go`), applied to every tool error
— text, issue messages and issue hints alike, since the rendered text is
built from all three. Six phrases get a real tool-shaped repair. Behind them
is a **generic rule**: anything still matching `METHOD /vN/…` becomes "the
HTTP API". That is deliberately a bare noun phrase — it stays grammatical
wherever a route can appear in a sentence, and it says the true thing, which
is that the repair is not on this surface. A hint added server-side tomorrow
therefore cannot reach a tool caller as a route; the worst it can do is lose
its specificity, and a test asserts no route survives the pass.

Not changed: the raw HTTP surface, where naming the route is correct and
stays.

#### The pattern, stated once as a rule

This is the third instance of one shape, and it is now a rule rather than a
per-field discovery:

> **A refusal that is correct and unactionable is a defect.** When a rejected
> value is wrong in a *recognisable* way, the refusal must name the repair —
> and it must name it in the caller's own vocabulary. A small model handed a
> true "not found" with no repair does not explore: it re-sends the identical
> call until its turn budget ends the attempt.

The three instances: the mis-shaped `object` (§8.33 defect 2, three identical
repeats), the B1 arm's relocated block reference (§8.33 — removing the field
moved the mistake, it did not remove it), and the truncated space id here
(seven identical repeats). In every case the wrong value was one shape with
one repair, and in every case the model's *other* arguments were correct.

The code answer to "one rule, not one hook per field" is `steer.go`: an
`argSteers` table keyed by argument name, evaluated on `Run`'s error path. A
new diagnosable argument is a row, not a hook in an executor — and because it
is keyed by the argument and not the tool, every tool taking that argument is
covered the moment the row exists. `describe`'s private rewrite and the
`object` steer both moved into it; nothing in any executor knows a steer
exists.

What the rule does **not** license is guessing. Each repair fires only when
the server has already refused *and* the refusal quotes the caller's own
value *and* the shape is provable (a prefix of a real space id, a pure-hex
block reference, the working space id). Everything else keeps the server's
words.

#### Considered and not done

**Changing the `space` argument description** to warn about the dot. §8.33's
B2 arm is direct evidence that description prose does not change what a model
puts in an argument, and it would cost tokens on every call by every model to
repair a mistake one model makes. The repair costs nothing when the model is
right.

**Changing the identifier**. Out of scope by instruction and correctly so —
see the Phase 9 note below for what the suffix actually is.

#### Where the suffix comes from, and what it would cost to hide it

Reported, not acted on. `NewSpaceId(cid, repKey)` is
`cid + "." + strconv.FormatUint(repKey, 36)`
(any-sync `commonspace/spacepayloads/payloads.go`). The prefix is the CIDv1
of the marshalled, signed space header; the suffix is the space's
**replication key**, a `uint64` in base36. It is load-bearing twice over:
`ValidateSpaceHeader` rejects a header whose id suffix does not equal
`FormatUint(header.ReplicationKey, 36)`, and `nodeconf.ReplKey` feeds **only
the suffix** into the consistent hash that picks which any-sync nodes are
responsible for the space. The CID half plays no part in node selection.

Two consequences for anyone tempted to shorten it. First, the suffix is
*not* per-space in practice: derived spaces take `fnv64(accountPubKey)` and
every space a client creates reuses the personal space's key
(`space/service.go`, `space/create.go`), which is why all 17 spaces on the
eval account end in `.28y6mgnwgodt7` — the CID half is the part that
distinguishes them, which is exactly why the truncation *looks* survivable
and is not. Second, there is **no prefix index anywhere**: every local store
keys by the full id, and nothing in either repo does a prefix lookup. The
wrapper's steer reads the space list and matches in memory, which is fine for
an error path and is not a resolution mechanism.

So handing tool callers something they cannot truncate means either an alias
the API mints and resolves (a new identity to keep, invalidate and reconcile
— the corpse-key class of problem §8.22 already paid for once), or not asking
them for a space id at all. The second is Phase 9, and it is cheaper.

#### Evidence for Phase 9 (`APIV2_SURFACES.md` §10.3)

The plan predicted `spaceId` would be the argument a small model most often
**omits**, because it never appears in the user's request. The measurement
says it is the argument a model most often **mangles**: 83 of 93 `find` calls
in e4b's wrapper attempts, zero mangles in any other argument, and an ops arm
that takes no space id scoring 8/10 for the same model on the same tasks. The
prediction was right about which argument and wrong about the failure mode,
and the fix it proposes covers both. Added to the plan item; not implemented
here.

#### Verification

Live, against the running API, before and after — the mistake reproduces
through the CLI alone, no model needed:

```
before: space "bafyrei…z2i" not found — list spaces with GET /v2/spaces
after:  space "bafyrei…z2i" not found — that is the first part of the space id
        "bafyrei…z2i.28y6mgnwgodt7": a space id has two parts joined by a dot
        and BOTH are part of the id — pass it whole, exactly as `spaces` prints it
```

The same steer on `describe` (the by-construction claim), an unknown space id
keeping its plain refusal, and a working call issuing no extra request, all
checked live and pinned by tests.

**One `gemma4:e4b` spot check**, one task, one attempt: it truncated the space
id on its first `find`, read the repair, passed the full id on the very next
turn, and completed the task — 4 turns, `model_done`. That is the point it
previously did not get past. It is a spot check and not a rate.

Each fix has a test that fails when the fix is reverted; all three reverts
were run: dropping the `space` row from `argSteers` (`TestSpaceRefSteering`),
unwiring the vocabulary pass from `steerError` (`TestRestVocabulary`), and
dropping the truncated-space case from the `object` repair
(`TestObjectRefSteering`).

### 8.35 A space reference a model cannot truncate (2026-08-11 — decisions as built)

§8.34 measured the mistake and made it recoverable. This removes it: `/v2`
now **serves** spaces by a short reference and **accepts** either spelling
everywhere a space id is accepted. The full id keeps working, forever; this
is additive addressing over the existing identity, not a new identity.

#### The measurement this answers

A space id is `<CID>.<base36 replication key>` —
`bafyreihwvsaekzzyb54o7um4hdpvpn5b2invn75lmijhhtghblvphxwz2i.28y6mgnwgodt7`.
`gemma4:e4b` truncates it at the dot, plausibly reading the suffix as a file
extension: **83 of 93 `find` calls** across its wrapper attempts in run
`20260810-235748`, **zero** mangles in any other argument of any other tool.
It collapsed that model's wrapper arm to 2/12 until §8.34's repair made the
refusal recoverable. `space` was the last place a raw composite id was asked
for — every object-addressed tool already takes `object` alone — and it is
exactly where the failure was measured.

#### The form: the TAIL of the CID half, and NOT the replication key

**Off the tail, not the head.** Every space CID on one account is a CIDv1 in
the same multibase and codec, so they all start `bafyrei…` — about eight
characters that distinguish nothing. That is the same reasoning that makes
anyblockjson's block labels shortest-unique-**suffix**
(`mintedSuffixLabels`), and the same machinery shape is reused here: a
census over the whole visible set, a fixed tail length, and no short form at
all for anything that collides.

**Without the replication key**, which is what removes the dot — and with it
the character the model cut at. Excluding it costs nothing, because it
*distinguishes nothing*: derived spaces hash the account key and every
client-created space reuses the personal space's, so **all 17 spaces on the
eval account end `.28y6mgnwgodt7`**. `nodeconf.ReplKey` hashes it to pick
responsible nodes; it is not a per-space discriminator.

That second fact is also the trap, and it is why the fixture is the load-
bearing part of the test file. **The census must run over the CID half, after
splitting on the dot.** Feed the composite ids in and every tail is
identical, the collision rule fires on every pair, and the feature emits
*nothing* — no error, no failing test, silence on exactly the accounts it
exists for. `spaceref_test.go` therefore pins the production shape: CIDs that
differ, replication keys that are **the same**, and an explicit assertion
that a composite-tail census would collapse to one bucket while the real one
does not. (Same shape as the Phase-3 review's "fixture-lossless mask", where
every edit fixture was built through `Unmarshal` and so could not observe a
loss that only happened on real documents.)

**Six characters.** The CID half is 59 base32 characters, and its last
character carries only **3 bits** — 288 payload bits do not divide by 5, so
the final symbol is padded (the 17 real ids use 7 of its 8 possible values).
A tail of n characters holds 3 + 5(n−1) bits: 28 bits at n = 6. Over the real
account all 17 tails are distinct at every length from 2 up; at 6 the
birthday probability of any pair colliding is ~7 × 10⁻⁵ even at 200 spaces,
and a collision degrades gracefully rather than failing.

**Only real space ids enter the mechanism.** `isSpaceIdShaped` requires
`<base32 cid ≥ 32 chars>.<base36 key>`; anything else — a fixture's
`spaceLive`, a tech space id, a hand-written string — neither shortens nor
answers to a tail. That is `mintedSuffixLabels`' own rule (`isMintedLocalId`,
"anything unrecognised stays full"): a false negative costs a few tokens, a
false positive turns a meaningful identifier into a guess. It is also why
this change moved no existing test.

#### Resolution: the rule the codebase already has, not a second one

`matchSpaceRef` **is** `matchBlockRef` (object.go) applied to space ids over
their CID halves: exact full id first, then a unique suffix; more than one
claimant is a 400 listing the candidates. Nothing new was invented, and the
one rule now covers block refs, view refs, payload ids and space refs.

A consequence worth stating: **the §8.34 truncation now resolves**, because
the whole CID half is a suffix of itself. The mistake that cost 15 attempts
is a working call.

#### Where it runs: a middleware, in front of the grant gate

Every space-addressing `/v2` route uses one param name (`SpaceParam`; the
conformance walk refuses any other), so one middleware covers the surface —
including routes added later, the same by-construction argument the grant
gate rests on. It **must** run before `ensureSpaceGrant`: grants are keyed by
full space id, so a short reference reaching the gate unresolved would be
refused as a non-granted space. Resolving inside the services would be too
late by one middleware.

**The scoped-key path, checked.** Resolution's candidate set is
`liveSpaceRows` — live spaces intersected with the ctx grant, the same
enumeration `GET /v2/spaces` serves from. So:

- a short reference can only ever resolve to a space the caller can already
  see. It is not a probe: a tail belonging to a non-granted space does not
  resolve, the param is left exactly as it arrived, and the request meets the
  refusal any other unknown value meets — quoting the caller's own value, not
  naming the space the tail belongs to.
- a full id is returned untouched without reading the space list, so the
  common path costs nothing and a non-granted full id still gets the same
  403 it got before.
- the grant check is not weakened anywhere: it still compares full ids, still
  runs on every request, and the service-level `ensureSpaceGranted` backstop
  is unchanged.

`liveSpaceRows` is also the point of a small refactor: `ListSpaces`,
`spaceRefs` (the global-search fan-out) and whoami's name resolution used to
enumerate separately. They are one enumeration now, because a short form is
only unambiguous *relative to a fixed set* — two censuses would be two
answers to "which spaces exist".

#### Serve short, accept either, echo what you were served

Served short: `GET /v2/spaces` rows, `GET`/`POST`/`PATCH /v2/spaces/{id}`,
global-search rows' `spaceId`, whoami's `grant.spaces[].id` (a granted space
the caller cannot SEE has no census entry and keeps its full spelling — the
grant echo must stay complete), and therefore the wrapper's `spaces` tool,
which passes `SpaceRow` through verbatim. The census runs over the WHOLE
visible set before pagination, so a page cannot mint a tail a space on
another page also claims.

The echo is one hook, not twenty. `v2handler.RespondV2Error` — the single
place a v2 error becomes bytes — re-spells the resolved full id back into
the caller's own reference across the message, every issue message and every
hint. The alternative was the same substitution at ~20 `Sprintf` sites, where
one could drift from nineteen and a message added tomorrow would inherit
nothing. Substituting into a repair URL keeps it valid: every route takes
either spelling. The not-found and not-granted refusals needed nothing —
resolution only ever succeeds, so those messages already quote the caller's
own value.

Per §8.28 the mechanism is **not** explained to agents: neither
`core/api/v2/SKILL.md` nor `cmd/anytype/SKILL.md` mentions short versus long.
Both already say "list the spaces and pass the id", which is now true of a
value with no dot in it. The mechanism is in the API reference (the v2
OpenAPI description) and here.

**The compact `filter` string is unaffected.** A space id cannot appear in
it: filter keys are the space's property keys plus the four-key system
allowlist, and `spaceId` is not among the 38 keys the eval space publishes —
checked, not assumed. So §C2's validate-before-fold exception raises no
question here, and the string keeps treating identifiers exactly as it did.

#### The token cost, on the record

Measured with the Gemma 3 tokenizer (ollama `prompt_eval_count` via the
served `gemma4:e4b`), against the real 17-space account:

| surface | before | after | change |
|---|---|---|---|
| the `spaces` tool's whole output | 895 tok | 159 tok | **−82.2 %** |
| one space id, as the model must EMIT it | 44 tok | 2 tok | **−95.5 %** |

The second row is the one that matters: 44 tokens of exact base32 copying, on
every `find`, `describe` and `create` — which is the copy the model got
wrong.

#### Live verification

Before (the running API, old binary):

```
GET /v2/spaces        → "bafyreia4znhhjvxek2iux7enfzilekj5vwlgddgogpfgnx5qnim7bugaxa.28y6mgnwgodt7" …×17
GET /v2/spaces/hxwz2i → 404 space "hxwz2i" not found — list spaces with GET /v2/spaces
GET /v2/spaces/bafyrei…hxwz2i/objects → 404 (the §8.34 truncation)
```

After:

```
GET /v2/spaces        → bugaxa | Project Tracker … hxwz2i | APIv2 eval   (17 rows, all distinct)
GET /v2/spaces/hxwz2i → {"id":"hxwz2i","name":"APIv2 eval"}
GET /v2/spaces/<full> → {"id":"hxwz2i","name":"APIv2 eval"}      (full still works, served short)
GET /v2/spaces/bafyrei…hxwz2i/objects → 200                      (the truncation resolves)
POST /v2/spaces/hxwz2i/search         → 200                      (nested routes too)
GET /v2/spaces/zzzzzz/objects         → 404 space "zzzzzz" not found  (the caller's own value)
GET /v2/spaces/hxwz2i/types/nope      → … not found in space "hxwz2i" … GET /v2/spaces/hxwz2i/types
GET /v2/spaces/<full>/types/nope      → … not found in space "bafyrei….28y6mgnwgodt7" …
POST /v2/search                       → rows carry "spaceId":"hxwz2i"
```

Through the wrapper CLI, all three `space`-taking tools: `spaces` prints the
short ids, and `find --space hxwz2i`, `find --space <full>` and
`find --space <truncated>` all return the same object.

**One `gemma4:e4b` spot check**, one task, one attempt (`edit-one-word`,
wrapper/large): it passed `space: "hxwz2i"` verbatim on its first `find`,
made **zero** failed calls, and finished in 3 turns on `model_done`. Its
reasoning names the short id as the space. It is a spot check and not a rate.

#### What this does to §8.34

§8.34's `space` steer is **superseded for the measured mistake** and stays in
place, unchanged, as a backstop. Its live trigger is gone twice over: the
served form has no dot to cut, and a truncated FULL id pasted from elsewhere
now resolves. The one case it could still meet — a truncated id belonging to
a space the credential cannot see — is one no repair can help, since that
space is unusable to the caller either way. Worth recording honestly:
`spaceIdsWithPrefix` reads `GET /v2/spaces`, which now serves short ids, so
the prefix match will not fire against a served list; the value that would
have needed the repair resolves instead.

#### Tests, and the revert each one catches

`core/api/v2/service/spaceref_test.go`:

- **`TestShortSpaceRefs`** — a real-shaped account (identical replication
  keys) shortens every space; a composite-tail census would collapse to one;
  colliding CID tails keep BOTH full spellings; an unshaped id never
  shortens. *Reverts caught:* computing the tail over the composite id
  (the silent-nothing regression), dropping the `counts[tail] != 1` guard,
  dropping `isSpaceIdShaped`.
- **`TestMatchSpaceRef`** — exact wins; a unique tail resolves; the whole CID
  half resolves; an ambiguous tail reports both claimants. *Revert caught:*
  matching the suffix against the composite id, or dropping the exact-first
  branch.
- **`TestResolveSpaceRef`** — a full id passes through with an EMPTY store
  (proving the common path reads no space list); ambiguity is a 400 with
  candidates; **a non-granted space's tail does not resolve and the grant
  backstop still refuses it**; a non-granted FULL id is still refused, not
  hidden. *Reverts caught:* widening the candidate set past the grant
  intersection, resolving before the grant filter, returning an error instead
  of the caller's value on no match.
- **`TestSpacesSurfaceServesShortRefs`** — the list serves short; colliding
  spaces are listed in full; the census precedes pagination; GET-one serves
  short. *Reverts caught:* minting per page, serving the full id.
- **`TestWhoamiServesShortSpaceRefs`** — the maps stay keyed by FULL id (a
  grant is keyed by space id); an invisible granted space keeps its full
  spelling. *Revert caught:* keying `resolveGrantedSpaceNames` off the served
  id, which silently empties whoami's space names.

`core/api/v2/spaceref_test.go` (the route half):

- **`TestResolveSpaceRefMiddleware`** — the param rewrite; the full id
  untouched; the §8.34 truncation resolving; ambiguity refused before the
  handler; the refusal echoing the caller's spelling and NOT the full id.
  *Reverts caught:* unwiring `resolveSpaceRef` from the router, dropping the
  echo from `RespondV2Error`.
- **`TestResolveSpaceRefAndTheGrantGate`** — resolution BEFORE the gate (a
  granted space's short reference passes); a non-granted tail is refused
  quoting the caller and never naming the space; a non-granted full id is
  refused exactly as before; an invisible space's tail resolves nowhere.
  *Revert caught:* moving `resolveSpaceRef` after `ensureSpaceGrant` — which
  turns every short reference into a 403 for a scoped key.

`core/api/server/v2_spaceref_test.go` (the REAL engine):

- **`TestV2ShortSpaceRefThroughTheRealEngine`** — the two tests above build
  their own middleware chain, so neither can see whether
  `apiv2.RegisterRoutes` installs the middleware at all. This walks the real
  engine: the list serves short and does not leak the full id, a short
  reference on a path param resolves, the full id still works, a granted key
  reaches its space by the short reference, and a non-granted tail is still
  refused. *Revert caught:* deleting the `v2.Use(resolveSpaceRef(...))` line
  from the router — which the package-local tests do not notice.

All seven reverts were run and each failed the named test.

One free consequence, recorded: the C8 idempotency key is scoped by
`c.Param("space_id")` (`middleware.go`), which the middleware has already
rewritten — so a retry that spells the space differently still hits the same
cached entry, rather than replaying the mutation under a second key.

#### Retiring Phase 9

Phase 9 (space-optional object routes) is **retired** — see `APIV2_PLAN.md`
3.2 and `APIV2_SURFACES.md` §10.3. It was proposed for this problem and this
supersedes it for the measured version of it. Recorded there: what Phase 9
uniquely solved (a cold-pasted object id with no prior `find`), that no eval
has ever produced that case, and the two defects found while scoping it —
`ResolveSpaceIdWithRetry` is `retry.Attempts(0)` (infinite, bounded only by
the context, so an unresolvable id spins instead of 404ing), and
`set_properties` needs a space regardless because `propertyFormats`, the
option-name guard, `@me` and relative dates are all space-scoped. Decision D2
is **moot**, not decided.

### 8.36 The full space id has to stay reachable (2026-08-11 — decisions as built)

§8.35 made the short reference the served spelling and left the full id
reachable "everywhere" — on input. On **output** it became unreachable:
`?ids=full` was wired only into the object read (`V2IdsCompact` /
`V2IdsFull`, `object.go`), and the space surfaces ignored it. There was no
call in `/v2` that answered with a full space id.

#### Why that matters: a short reference is not an identifier

The short form is **unique only against the caller's current visible space
set**. Join a space whose CID tail collides and the collision rule fires:
both spaces drop back to their full spelling and the reference that was
printed yesterday addresses nothing today. That is the correct behaviour —
§8.35 chose graceful degradation over a wrong resolution — and it is exactly
why the short form is an *addressing convenience*, not an identity.

So every caller that **persists** a space reference needs the full id: a
config file, a shell script, a log line, a support ticket, a gRPC call into
the heart, another API. `/v2` is not the only thing that speaks about spaces,
and the ones that are not `/v2` only know the composite id.

Recorded honestly: the full id was *derivable* even before this change, by
string surgery on an unrelated object's id — `domain.NewParticipantId` embeds
the space id in every participant id with its first dot replaced by an
underscore, so `GET …/members/me` leaks it. That is a coincidence of an
internal id format, not a contract, and "parse our member id backwards" is
not an answer to "how do I store this space id".

#### One parameter, not a second one

`?ids=` already means "which spelling of ids do you want" and already has
`compact` (default) and `full`, a documented meaning (the backup/export
shape, C4), and a 400 for anything else. A space id is an id. So `?ids=full`
was extended to the space surfaces rather than given a sibling:

- **not** `?fullIds=` — two parameters that answer the same question, which
  a caller then has to set consistently, and which will eventually disagree;
- **not** a second `fullId` field beside `id` — C2, one concept one slot. It
  would also double the token cost of the very list §8.35 shrank by 82 %, on
  every caller including the ones that never wanted it.

The value list and its refusal live in **one function**, `ParseIdsShape`
(`service/idshape.go`). The object read's own plan validation calls it, so
the block-id axis and the space-id axis cannot come to disagree about what
`ids=export` means — a second copy of a value list is how a surface starts
accepting on one route what it refuses on another.

#### Where it runs: one middleware, like `ensureDryRun`

`ensureIdsShape` (`v2/idshape.go`) parses `?ids=` once for the whole group
and records a full request on the **request context** — the carrier the
§8.35 echo already uses. `servedSpaceRefs` (the census) and `servedSpaceRef`
(the single-id form) consult it; the five serving surfaces call those and
need no branch of their own.

It is group-wide for `ensureDryRun`'s reason: the parameter is group-wide, its
values are a closed set, and an unknown value must be a 400 on every route
rather than on the routes someone remembered to wire. A space-serving surface
added later inherits the behaviour by calling `servedSpaceRefs` — the same
by-construction argument the grant gate and the reference resolver rest on.

It runs **in front of `resolveSpaceRef`**, so the candidate list an ambiguous
reference is refused with is spelled the way the request asked for.

The ctx carries only the *full* case: an untouched context means compact,
which is what every internal caller and every existing test already carries,
and is why this change moved no existing test.

#### Per-surface decisions

| surface | serves a space id? | `?ids=full` |
|---|---|---|
| `GET /v2/spaces` | rows' `id` | **honoured** |
| `GET /v2/spaces/{id}` | `id` | **honoured** |
| `POST /v2/spaces` | the created `id` | **honoured** — a create is precisely when a caller has a new id to store |
| `PATCH /v2/spaces/{id}` | `id` (via GET-one) | **honoured** |
| `POST /v2/search` (global) | rows' `spaceId` | **honoured** — the one place an agent learns a space id it did not name |
| `GET /v2/auth/whoami` | `grant.spaces[].id` | **honoured**. It has no space in its path, but it does have a space id in its body, and it is the surface a holder reads to learn which spaces it holds — the answer a scoped integration writes into its own config |
| ambiguity refusals (`ResolveSpaceRef` candidates) | the candidates | **honoured** — a refusal's repair value is part of the response |
| `POST /v2/spaces/{id}/search`, `GET …/objects`, members, types, properties, chats, sets/collections | **no space id in the body** | parsed and ignored: there is nothing to spell |
| `GET …/objects/{id}`, `GET …/types/{key}` | no space id in the document | see below |

Two surfaces deliberately did **not** change:

- **Accepting.** Unchanged and unchangeable: exact-then-unique-suffix, both
  spellings always accepted, on every route. `?ids=` is about what is
  **served**. `GET /v2/spaces/{short}?ids=full` is the round trip a caller
  makes to turn a reference it was handed into one it can store.
- **Member ids.** `_participant_<spaceId>_<identity>` keeps the full space id
  inside it under both shapes. It is an id of a different object, addressable
  as a unit; rewriting its interior would break it.

#### The object-read overlap: same knob, and nothing to spell

Should `?ids=full` on an object read imply full space spellings in that
response? **Yes — and it is the same flag, set by the same middleware, on
that request too.** There is no second knob to reconcile: a caller asking for
the export shape has asked for it, full stop.

What that changes on the object read today is **nothing**, because an
AnyBlock document carries no space id — not in the envelope, not in a
property value, not in a ref. The same is true of the object list and the
space-scoped search (`includeSpaceId` is set only by the global fan-out). So
the answer is "yes, and it is currently a no-op there" — which is the useful
form of the answer, because the flag is already correct for the surface that
adds a `spaceId` tomorrow.

#### Not agent-facing (§8.28)

Neither `core/api/v2/SKILL.md` nor `cmd/anytype/SKILL.md` gains a word about
short versus long — the §8.28 rule stands, and an agent that lists spaces and
passes the id it was given is still correct under both shapes. `?ids=full`
stays framed exactly as C4 frames it: the backup/export shape. The mechanism
lives in the v2 OpenAPI description (where the info block now names the
stability limit and points at `?ids=full` for anything stored outside the
API), in the six `@Param ids` annotations, and here.

The wrapper and the CLI gain nothing either: they are the agent-facing tier,
they pass `SpaceRow` through verbatim, and nothing they do outlives the
session that made the call.

One free consequence, recorded: the C8 hash already covers the query string,
so a retried create that *adds* `?ids=full` earns the 409
`idempotency_conflict` rather than replaying — the same rule `?dry_run` has
lived under since §8.15. A retry must repeat the request it is retrying.

#### Tests, and the revert each one catches

`core/api/v2/service/idshape_test.go`:

- **`TestParseIdsShape`** — the three legal values and which is the default;
  `export`/`FULL`/`true`/`short` are 400s addressed at `ids` and naming both
  allowed values; and `V2ObjectQuery.validate` produces those same answers.
  *Reverts caught:* dropping the `default` branch (unknown values silently
  becoming compact); re-inlining a private switch in `object.go` so the two
  axes can drift.
- **`TestFullIdsCtx`** — an untouched context means compact.

`core/api/v2/service/spaceref_test.go`:

- **`TestSpacesSurfaceServesFullRefsOnRequest`** — under `?ids=full` the
  list, GET-one, the global-search fan-out and whoami's grant echo all serve
  the full id, while the default still serves short; and an ambiguity's
  candidates follow the request's shape (the fixture is two spaces differing
  at the SIXTH character from the end, the only ambiguity in which candidates
  *have* short forms of their own). *Reverts caught:* `servedSpaceRefs`
  ignoring the ctx — four subtests fail; `servedSpaceRef` losing its
  short-circuit — the GET-one subtest fails.

`core/api/v2/idshape_test.go` (the route half):

- **`TestEnsureIdsShapeMiddleware`** — the middleware's own contract over the
  real GET-one handler: `?ids=full` serves the full id, the default and
  `?ids=compact` serve short, a short reference still *addresses* the space
  it serves in full, an unknown value is a 400 before the handler, and the
  shape is read before resolution. *Reverts caught:* dropping the error
  branch from the middleware; the `servedSpaceRef` short-circuit again, this
  time through a real chain.

`core/api/server/v2_spaceref_test.go` (the REAL engine):

- **`TestV2FullSpaceIdsThroughTheRealEngine`** — the package-local tests build
  their own middleware chain, so none of them can see whether
  `apiv2.RegisterRoutes` installs `ensureIdsShape` at all, or where. This
  walks the registered engine: the list, GET-one-by-short-reference, whoami
  (which nothing else exercises for this parameter), the unknown-value 400,
  and the candidate spelling. *Reverts caught:* deleting
  `v2.Use(ensureIdsShape())` from the router — five subtests fail while the
  package-local ones stay green, which is precisely the blind spot this file
  exists for; moving it AFTER `resolveSpaceRef` — exactly one subtest fails,
  the candidate-spelling one.

All six reverts were run and each failed the named tests, with the named
subtest granularity.

### 8.37 Wave 1 — the identity layer finished: mint, backfill, one vocabulary (2026-08-13 — decisions as built)

Wave 1 of `APIV2_PLAN.md` (items 1.1–1.4), specified in
`ADDRESSING.md` §7.5 / §7.5a. Three commits, in dependency order — the
union check has to exist before the backfill can be safe, and both have to
exist before the re-spelling sweep can be anything but silent
mis-resolution.

**1.1 — the heart-side mint checks a union** (`objectcreator/apikey.go`).
`injectApiObjectKey` derived the api slug from the create-time name and
validated **nothing** (§2.3-1); v2 defended only at resolution. The mint is
where the address is decided, so the check moved there, and it tests the
whole union §7.5a-6 names — not just stored slugs: (1) the space's live
stored `apiObjectKey` slugs, (2) the space's live stored **keys** (a legacy
readable key is an address at chain step 1, so a slug spelled the same is a
shadow), (3) the **bundled-derived** vocabulary via
`pkg/lib/bundle/apislug.go`. Arm 3 is the one that catches the headline
case — a UI property named "Due Date" slugs to `due_date`, which is exactly
bundled `dueDate`'s derived slug, and **no stored detail exists for it in
an old space**, so a store-only check would wave it through. Arms 1–2 come
from one bounded listing (§7.5a-2), arm 3 from a point lookup.

Two deliberate asymmetries with v2's mint: a **bundled install skips the
check** (its slug is derived, not minted — the table in code is its
authority, and convergence is the install mechanism, §2.4-1), and a
collision **suffixes rather than refuses** (`due_date_2`) because a UI
create has no caller to steer. Corpses vacate the namespace; an entity
never collides with itself. A store error degrades the mint and is logged —
failing a user's property create because the store hiccuped is the worse
trade, and the ambiguity-loud lookups are the standing backstop.

**1.2 — the backfill, and the floor under the collisions it cannot fix**
(`space/internal/components/migration/apiobjectkey`). `systemobjectreviser`
never set `apiObjectKey` — its revised-keys list has none, and it only
visits bundled/system objects anyway, which are exactly the ones that need
no stored slug. A migration, built like one: it fills only EMPTY slugs
(never re-points one — `apiObjectKey` is mutable and **v1-visible**), skips
bundled keys, processes candidates in ascending object-id order so two
devices converge, and costs one filtered query in the steady state.

**The already-taken case is a deliberate no-op**, named in code as
`takenSlugPolicy` and pinned by tests. The mint suffixes because a UI
create must succeed and carries a name the user just chose; a backfill has
neither, and `due_date_2` invented unattended is a permanent, v1-visible
address minted out of an ordering accident. Skipping is the reversible
choice: the entity keeps exactly the addressability it has today, and a
later run picks it up if the obstacle clears. Which resolution the case
ultimately gets is ADDRESSING §8 open question 3 (lean: "floor first").

**The plan's stated defect for 1.2 was mis-attributed, and the real one is
now loud.** The wrong-entity write ("a UI property named Due Date claims
`due_date`, so `setProperties` lands in it instead of the bundled one") is
not what a backfill fixes: the squatter *has* a slug, so the backfill never
touches it. 1.1 prevents new ones; existing ones cannot be repaired without
re-pointing an address v1 serves. What needed no permission was the failure
mode — a stored slug that the bundled table resolves to a **different** key
is now **ambiguous**, and ambiguity is a loud 400 listing both holders.
`servedKey` refuses the same spelling, so the API never advertises an
address that 400s. Silent-and-wrong became refused-and-actionable; the
backfill's own value is the one §7.5a-6 states — pre-slug custom keys had
**no** bare-op address at all.

**1.3 — one key vocabulary, bundled keys included.** 153 of 194 bundled
relation keys and 5 of 29 type keys (`objectType`, `relationOption`,
`spaceView`, `diaryEntry`, `chatDerived`) re-spell on the wire.

*The reverse is a table, both directions, built from the bundle — never a
case transform.* `anyblockjson/keyvocab.go` states it and two tests pin it
with the counterexamples that prove it: `mediaArtistURL` →
`media_artist_url` → `ToLowerCamel` yields `mediaArtistUrl`, and `_score`
does not round-trip. A later "simplification" to a case function fails
there first.

*Two authorities, one chain.* The package default is the bundled derived
table, which ships with every reader, so a document resolves its bundled
keys offline with no store. Inside a node, `storeresolver/keyvocab.go`
widens it with the space's stored slugs — **one bounded details query per
kind per resolver instance** (a resolver is per request/operation), never a
point query per reference, because `apiObjectKey` is an ordinary hidden
detail with no index. Precedence is §7.5a-5 and is load-bearing: an exact
live stored key wins over the slug layer, a twin slug resolves to neither,
and a store error degrades to the bundled table rather than to a partial
map — a half-built vocabulary would resolve a write against the wrong
property, the exact class §7.5a-2 forbids a cache from producing.

*The format's key slots follow* (`properties` map, `typeProperties[].key`,
`typeProperties[].objectTypes`, envelope `type`/`templateFor`, dataview
`properties[].key`/`groupBy`/`coverProperty`/`endProperty`/sorts/filters/
columns, the `property` block's `key`, a link block's `properties`): export
spells, import inverts. **`objectTypes` is the complete inventory's last
entry and was the one the wave missed** — it NAMES types, so it is a key slot
by the same test every other one passes, and leaving it untranslated would
have made one array in a type document speak a vocabulary the envelope two
lines above it does not (§8.38). Two
consequences worth writing down: canonical property order now sorts by the
**spelling**, because the reader sorts what it sees; and two stored keys
whose slugs would collapse onto one JSON key keep the honest stored
spelling for the second — a duplicate map key loses a value.

*What did NOT re-spell, deliberately* (§7.5a-4): the envelope and DTO field
names (`spaceId`, `iconSize`, `defaultTemplateId`, `has_more`'s existing
carve-out), block attribute names (a callout's `iconEmoji`), enum **values**
(`kind: "objectType"`, layout names) — `objectType` the layout value
coexists with `object_type` the type key, and that is intended — the
`index.json` envelope (§2c), and a type document's envelope `key` (it is
not an address: v2 stopped deriving identity from it in §8.22, and the
§8.23 reject list drops `uniqueKey` outright).

*The cascade*, all of it mechanical: SPEC §3's key-vocabulary rule (rewritten
— the old text said "as stored, camelCase"), its worked examples and the §14
full document; OVERVIEW and ANOMALIES; the four golden files; `schemas.go`'s
served examples and `schemas_ops.go`'s op schemas; the served EBNF examples
and the filter parser's hints; the wrapper's tool manifest; both SKILL
guides (the v2 one's teaching sentence changed meaning, not just spelling);
and a new eval task — every existing one named single-word keys only, which
cannot tell one vocabulary from the other.

*Two functional channels needed the wire spelling taught to them*, found by
following the sweep rather than by test failure: `UpdateType`'s `properties`
patch never went through `canonicalizeDocumentKeys`, so the schema now
advertised `icon_emoji` against a check that only knew `iconEmoji`; and
`IsOutputOnlyProperty` is called by the wrapper with SERVED keys, so
`created_date` would have stopped reading as output-only. Both now accept
either spelling through the bundled table, and the output-only listing
advertises slugs.

**1.4 — the deferrals, resolved rather than restated.** Both fell out of
1.3, and one turned from debt into a defect on the way:

- **View-op `set`/`columns` key channels** were stored-key-only. That was
  survivable while documents spelled stored keys; the moment they spell
  slugs it is a defect, because the ops merge into the exported document —
  a stored-key column address stops matching the column it names (it
  appends a twin), and a folded spelling lands verbatim in the stored
  `groupBy`, where nothing can ever match it. `canonicalViewKey` now
  translates every key input once, on the way in, to the document's
  spelling: `set.groupBy`/`coverProperty`/`endProperty`, `set.sorts`,
  `set.filters`, the compact `set.filter` string's **output**, and the
  `columns` map keys.
- **The compact filter string stays fold-strict on input** (§C2's stated
  exception), unchanged: it validates against the served spellings before
  folding. What changed is that its served spellings are now the slugs, so
  the exception costs a caller nothing it can see — and its parsed output
  is canonicalized like every other channel, which it was not before.

**Verification.** Every behaviour has a test that fails on revert, and each
revert was run: the mint's union arms and its wiring in `createRelation`
(`objectcreator/apikey_test.go` — 10 subtests fail with the check stubbed
out); the backfill's fill, skip, idempotence and convergence
(`apiobjectkey_test.go`); the bundled-slug serving and the shadow's loud
400 (`keys_output_test.go` — reverting `servedKey` to the BSON-only rule
and dropping the `shadowedBundled*` branch fails 3 tests); the view-op key
spellings (`viewops_test.go` — stubbing `canonicalViewKey` fails both
subtests); and the vocabulary's two counterexamples
(`anyblockjson/keyvocab_test.go`).

**v1 is untouched** — no file under `core/api/handler`, `core/api/service`,
`core/api/model`, `core/api/filter` or `core/api/docs/v1` changed, and its
suites pass. v1 keeps its own key vocabulary by construction: it reads
`apiObjectKey` directly and never consults the derived table.

**`cmd/anyblockroundtrip` — can it still run, and what would it need?** Yes,
unchanged, and it is the sweep's most valuable outstanding verification.
The harness exports each object and re-imports it through the same
`storeresolver` wiring, so the vocabulary is applied symmetrically in both
directions and the comparison is still snapshot-vs-snapshot, not
bytes-vs-bytes. Two things to expect on the next run: documents now carry
slugs, so any **saved corpus artifacts** from an earlier run compare
against the old vocabulary and should be regenerated rather than diffed;
and a space holding a **shadow** (a custom slug over a bundled key) is the
one place the round trip can now lose a key — the exporter keeps the honest
stored spelling for the second holder, which imports back correctly, so the
expected result is a pass, but it is the case to watch in the anomaly
report. It was **not run here**: it needs account credentials, which this
work deliberately did not touch.

**Not done, and why.** OpenAPI regeneration (`make openapi`) is still
pending across Waves 0–1 (Q5); no annotation *shapes* changed here, only
description and example text. The `?ids=`/pins work, the §7.4 write
defaults and the active re-slug-on-revive remain as §8.22 recorded them.

### 8.38 Wave 1 review: the write half spoke a different vocabulary than the read half (2026-08-13 — decisions as built)

A three-lens review of Wave 1 (§8.37) reproduced every finding below by
execution rather than by reading. Two were data-corrupting, two more served
addresses that resolve to the wrong entity, and one made two thirds of a
PATCH surface return 400 for every spelling the wave had just taught. All
five were shipped over a green suite.

**The fixture-blindness lesson, stated plainly, because this is the third
time.** Every blind fixture in Wave 1 used a key the bundled table happens to
invert — `name`, `page`, `dueDate`, `severity`, `created_date`. A test whose
fixture key spells the same in both vocabularies **cannot fail** when the two
vocabularies diverge, whatever it asserts. `TestV2UpdateType` used `name` and
stayed green while `icon_emoji` and `recommended_layout` 400'd. The wrapper
suite stubbed `dueDate` and `createdDate` as *served* keys and so modelled a
server that no longer existed. `TestV2ListingsServeBundledSlugs` asserted only
the row that the guard it was testing already covered.

The rule this leaves behind, and the one to apply to every future test in this
layer: **a key-vocabulary fixture must use a BSON-keyed entity with a stored
`apiObjectKey`, or a stored key the bundled table resolves elsewhere.**
Anything else proves nothing. Both new test files (`keys_write_test.go`,
`storeresolver/keyvocab_test.go`) say so in their headers.

#### 1. The write half fell back to the bundled table (critical)

`creatingResolvers.Options()` set `ResolveFormat`/`ResolveOptions`/
`ResolveProperties` and **no `Keys`**, so every import channel of the service
fell through to `BundledKeyVocabulary` while the read half exported through
`storeresolver`. Both directions were wrong, for opposite reasons:

- the bundled table does not know a BSON-keyed relation's slug, so
  `manual_property` imported as the **literal stored key** `manual_property`.
  Executed: `PATCH …/objects/obj1 {"op":"updateView","columns":{"severity":
  {"hidden":false}}}` on a space whose `severity` is a UI property stored as
  `6a76…e107` rewrote the dataview's relation key to `severity` — a dataview
  naming a key no relation object owns. Columns unbind, filters and sorts
  match nothing, silently. Every view op re-imports the whole dataview, so
  every view op did this.
- the bundled table over-reaches in the other direction: a space holding a
  live relation **stored** under `due_date` had `canonicalizeDocumentKeys`
  resolve it correctly at chain step 1 and then `Unmarshal` rewrite it to
  bundled `dueDate`. The value landed on the wrong property. Same on the
  type-create path, through `applyTypeProperties`.

Fix: `Keys: r.reads`. `storeresolver.PropertyKey` already implements chain
step 1 (an exact live stored key wins over the slug layer), which is exactly
what makes it safe on every call site — `create.go`'s document import,
`schema_write.go`'s type import, and `stateops.go`'s `insertBlocks`/
`replaceSubtree` fragments and whole-dataview re-import. Verified per site,
not assumed.

#### 2. `PATCH /v2/spaces/{s}/types/{key}` was broken for every spelling it taught

`schema_write.go` read the patch map with the **stored** key while the map is
keyed by the **wire** spelling: `typeDetailValue(key, patch.Properties[key])`.
For any key the two vocabularies spell differently the lookup returned nil and
the decode failed. Executed: `{"icon_emoji":"✅"}` → 400 with issue path
`/properties/iconEmoji`, a key the request never sent; `{"recommended_layout":
"todo"}` → 400. Two of the surface's four fields were dead, and only the
spelling the wave had just removed from every document still worked.

Fixed to `patch.Properties[raw]`, and `typeDetailValue` now takes the caller's
spelling for its issue paths — an error naming a key the caller did not send
is unactionable. `TestV2UpdateType` gained a table over `icon_emoji`,
`recommended_layout` and the camelCase form.

#### 3. The cascade re-spelled block attributes, which its own rule excludes

`v2OpBlockCommonProps` gained `icon_emoji`/`icon_image`. Block attributes are
**not** key slots — §7.5a-4 names the exclusion in the same commit. Both
payload defs are `additionalProperties:false`, so a grammar-constrained
decoder could not author a callout icon **at all**, while `GET
/v2/schemas/object` went on serving the format's own schema declaring
`iconEmoji`: two served schemas contradicting each other one request apart.

Reverted, and — the part that matters — **the exclusion is now enforced by the
build**. `TestSchemaOp` already cross-checked the op schema's block-*type*
enum against `anyblockjson.AuthorableBlockTypeNames()`; the mirror is now
there too: every property name `v2OpBlockCommonProps` publishes must exist in
the format's own block schema (`anyblockjson.KnownBlockProperty`, read out of
the embedded JSON Schema the same way the type enum is). Prose became a test.

The same over-reach hit `SPEC.md`, where it made the document contradict
itself: the §2c table re-spelled the `index.json` envelope fields while the
example six lines above still showed `iconEmoji` — and `index.schema.json` is
`additionalProperties:false` and rejects `icon_emoji`, so the **table** was
the wrong half. Reverted there, plus the callout row, the id-remap table, and
`spaceDashboardId`, which is a `pb.Profile` field name and not a key slot at
all. SPEC §3 now states the exclusion inline rather than leaving it in
ADDRESSING only.

#### 4. The emit side had no chain step 1 and no shadow check

`storeresolver`'s `PropertySlug`/`TypeSlug` returned `bundle.ApiSlug(key)`
unconditionally for a bundled key, never consulting the space. The accept side
(`PropertyKey`) honours both guards. Probed: a space where a BSON squatter
holds `due_date` exported the **bundled** property's value under an address
that resolves to the squatter; a space with a live stored key `due_date`
exported to the v1 relation; the type namespace had the same hole. A served
document labelled a value with a key denoting a different entity — while
`servedKey` applied the guards, so the listing and the document disagreed.

The emit side now runs one predicate, `keyMaps.roundTrips`, with all three
chain steps: a live stored key wins the spelling (step 1), another live holder
makes it ambiguous (step 2), and the bundled table resolving it elsewhere is
the §7.5a-6 shadow (step 3). It is deliberately the same predicate
`servedKeyOf` applies to a listing row — **the address a document carries and
the address a listing advertises are the same address**, so the two must not
be able to drift.

Two smaller defects in the same file: `keyMaps.add` cleared a twin slug from
the reverse map but returned before clearing the first holder's forward entry,
so export emitted a slug import refuses to invert; and `keyMaps.slug` never
asked whether a slug was ambiguous. Both closed.

`storeresolver/keyvocab.go` had **no test file at all** — 176 new lines pinned
by one incidental assertion elsewhere. It has one now, 16 subtests, every
fixture BSON-keyed or shadowed.

#### 5. `servedKey` advertised a shadowed slug (medium)

`servedKeyOf` tested other stored holders but never asked whether the bundled
table resolves the candidate elsewhere — the exact predicate the input side
added in Wave 1.4. Executed: with the bundled relation **not installed**,
`ListProperties` served `key="due_date"` for the squatter and
`resolve("due_date")` then 400'd as ambiguous, one request later. The existing
test asserted only the bundled row, which the holder guard already covered, so
the gap was untested. Third guard added; both namespaces pinned.

#### 6. New shadows were still mintable through v1's rename channel (medium)

`PATCH /v1/…/properties/{id}` and `/types/{id}` stamp `apiObjectKey` through
`ObjectSetDetails` and never enter `objectcreator`, so they bypass
`ensureUniqueApiObjectKey` entirely. Their only guard was v1's per-space
cache, which has no row for a bundled relation **not installed** in the space
— so renaming a custom key to `dueDate` minted a fresh `due_date` shadow,
through a door beside the one the mint hardening had just closed. The bundled
arm of the union check now runs on both rename paths, and `keys.go`'s claim
about new shadows is restated to name the channel rather than to assert
something broader than what was true.

**This is the first time this wave touched v1** (§8.37's "v1 is untouched" no
longer holds): `core/api/service/property.go` and `type.go` each gain one
refusal. The v1 **document** is byte-identical after `make openapi` — verified
by checksum — because no annotation changed; only two v2 descriptions did
(`chatDerived` → `chat_derived`, `setOf` → `set_of`).

#### 7. The last untranslated key slot, and the wrapper's fold

`typeProperties[].objectTypes` NAMES types and was passing through
untranslated, which would have left one array in a type document speaking a
vocabulary the envelope two lines above it does not. **Decided: it is a
type-key slot and speaks the one vocabulary**, like `type` and `templateFor`.
Export spells, import inverts, an unknown term passes through verbatim (chain
step 1, so a stored-key spelling keeps working). The offline bundle linter
(`cmd/internal/anyblockbatch`) accepts both. APIV2's key-slot inventory and
SPEC §2a now say so.

`BuildRecommendedLists` — the PATCH-types channel — writes the same §2a array
and inverted nothing, because it took a bare `PropertyResolver` and had no
vocabulary in scope. It now takes the full `Options`, so one type's property
list means the same thing whichever endpoint writes it. Nothing observable
changed through v2's own resolver, which duplicates the chain internally; what
changed is that the two channels can no longer drift apart, and the
package-level test says so with a recording resolver.

Updating the wrapper suite to the real served vocabulary exposed one live
defect it had been masking: the wrapper's forgiving fold was
`strings.EqualFold`, which does not match `dueDate` to `due_date`. Since the
served vocabulary flipped, `dueDate` is both the most natural guess a model
makes and the spelling every pre-1.3 document used. The wrapper passes an
unfolded key through and the server still resolves it at chain step 4, so the
write lands — what silently stopped is everything the wrapper does with the
key's **format**: the relative-date convenience and the option guard. Loud on
the server, silent in the layer whose whole job is forgiveness. The fold is
now `bundle.FoldApiKey`, the server's own (§7.5a-3).

**Verification.** Every fix has a test that fails on revert, and every revert
was executed:

| Fix | Revert run | Fails |
|---|---|---|
| 1 | drop `Keys: r.reads` from `Options()` | 4 subtests, `keys_write_test.go` |
| 2 | `patch.Properties[key]` | 2 subtests, `schema_write_test.go` |
| 3 | re-spell to `icon_emoji`/`icon_image` | `TestSchemaOp` structural guard |
| 4 | unconditional `bundle.ApiSlug`; drop the bundled arm of `roundTrips`; drop `delete(m.slugByKey, first)`; drop the corpse filter; drop `PropertyKey`'s step-1 branch | 5 separate runs, `storeresolver/keyvocab_test.go` |
| 5 | drop `shadowed()` from `servedKeyOf` | 3 subtests, `keys_output_test.go` |
| 6 | drop both `shadowsBundled*Key` calls | 2 subtests, `core/api/service` |
| 7 | drop `typeSlugs`/`typeKeys`; drop `BuildRecommendedLists`'s inversion; revert the wrapper fold | 2 + 1 + 2 subtests |
| collapse | drop `sort.Strings(keys)`; drop the `stored[slug]` arm; drop the guard | 3 runs, `anyblockjson/keyvocab_test.go` |

**One more defect, found by writing the test rather than by reading.** The
duplicate-slug collapse guard in `buildProperties` needs a deliberately
non-injective vocabulary to exercise, which no bundled fixture can produce —
and the moment one existed, the test failed intermittently. The collapse pass
ran over Go's **map iteration order**, so which of two holders keeps a
contested spelling was a coin flip per run: the canonical form was not
canonical, and export∘import byte-stability was chance on exactly the spaces
that hold a shadow. The pass now runs over the sorted stored keys. The same
test also exposed the guard's missing second arm: the contested spelling can
be another **stored key on the same object**, not only another holder's slug —
emitting it would bind the value to that key on the way back at chain step 1.
Both arms are pinned, the ordering one with a 32-iteration loop.

Test debt closed alongside: `buildProperties`' duplicate-slug collapse (its
failure loses a value), `IsOutputOnlyProperty`'s bundled
fallback in both spellings, `createObjectType`'s mint wiring (the type
namespace had the helper tested and the wiring not), and the
`shadowedBundledType` branch, which had no test at all.

**Not done.** The `apiObjectKey` backfill migration is untouched — a design
decision on replacing it with deterministic derivation is pending.
`cmd/anyblockroundtrip` still needs an account to run and was not run.
