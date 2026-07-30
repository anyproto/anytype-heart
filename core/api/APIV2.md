# Anytype Local API v2 — specification and phased plan

Status: **draft v0.3.5** · 2026-07-31 · GO-7383 follow-on
Depends on: AnyBlock JSON v1 flat (`pkg/lib/anyblockjson/SPEC.md` v0.6).
Evidence base: `docs/AgentApiV2Research.md` (+ Addendum A) and
`pkg/lib/anyblockjson/FLAT.md` — decisions cite sections there instead of
re-arguing. v1 (`/v1`) stays untouched for the deprecation window.

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
| C4 | **Object ids compact by default** (refs legend per SPEC §9a — lossless); `?ids=full` opts out. **Block ids are always full on default reads** — block-label compaction (5-char, legend-less, lossy) appears only in explicitly read-only shapes (`outline`, prompt/example exports). Write endpoints MAY additionally resolve block-id references by unique-suffix match against the live object (SPEC §9a wiring allowance). Never require echoing a full CID. *Package change required: split object-ref compaction from block relabeling in `anyblockjson.Options` (today one `CompactIds` flag does both).* | ~24×/id, −89% id errors (§3.6); id round-trip contract (R1) |
| C5 | Minimal rows: list/search responses carry `id, name, type` + requested property values. **Never embed type objects.** `fields=` expands. | v1's N× multiplier (§2.1) |
| C6 | Error shape everywhere: `{status, code, message, issues:[{path, message, hint}]}` — path-addressed, naming allowed values. Required codes include: `validation_failed`, `version_unsupported` (surfaces SPEC §10's "produced by a newer version" verbatim, naming both versions), `idempotency_conflict` (same key, different body), `etag_mismatch`, `ambiguous_input` (e.g. both `filter` and `filters` supplied). Error text is API surface; test it. | repair loop (§3.2, §4.6); R15 |
| C7 | Every object read returns **`etag`** (short opaque token, ≤8 chars, derived from tree heads — NOT the object's `revision` property, which stays in `properties`) plus an `ETag` header. Mutations accept **`If-Match` header only** (the AnyBlock body has no envelope slot for it). **Advisory by default**: without `If-Match`, ops apply last-write-wins and `diffStats` reports the outcome; with it, mismatch → 409 `etag_mismatch` carrying the current etag. Note: the etag advances on background sync, not only on agent edits — strict If-Match will 409 on sync noise; block-scoped preconditions (ops apply iff the *addressed* blocks are unchanged) are the deferred v2.x refinement. | R2; optimistic concurrency (§3.2) |
| C8 | `Idempotency-Key` honored on all mutations (POST, PATCH, PUT — v0.3.5; was POST-only); replay with the same key returns the stored result; same key with a different body → 409 `idempotency_conflict`. Response always returns created ids. | agent auto-retry (§3.7); R15 |
| C9 | `?dry_run=true` on every mutation → would-be diff summary + issues, nothing committed. | highest-leverage affordance (§3.7) |
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
DELETE /v2/spaces/{spaceId}/objects/{objectId}   # archive (v1 parity); ?permanent=true later
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
  labels (read-only shape, C4).
- The object response is the flat AnyBlock document + `etag` (+
  `warnings`).
- `types/{type}/schema` **[build]**: the derived artifact (SPEC §2a
  `GenerateSchema` — *planned there, not implemented*; this endpoint is
  its first consumer). `table` flavor = prompt-ready property table with
  live option names — requires an objectstore join per select property
  (options live on option objects, not in `typeProperties`) (R6). Still
  Phase 1: it is the highest-leverage accuracy lever (§3.4).
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
- **Resolver wiring is the substance of this phase [build]**: the
  create-missing property/option bridging from `anyblockjson`'s
  `PropertyResolver`/`OptionResolver` to objectstore +
  `ObjectCreateRelation`/`ObjectCreateRelationOption` does not exist yet
  (R-note). The **referential validation layer** (R9) lands here: a set
  filter naming a property the type lacks → error listing the type's
  actual keys.
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
{ "query": "…", "type": "task",
  "filter":  "done = false AND (dueDate < currentWeek() OR dueDate IS EMPTY)",
  "filters": [ { "property": "done", "condition": "equal", "value": false } ],
  "sort": [ { "property": "dueDate", "direction": "asc" } ],
  "fields": ["name","dueDate","status"], "limit": 25 }
GET /v2/spaces/{spaceId}/sets/{setId}/objects?view={viewId}&fields=…
GET /v2/spaces/{spaceId}/sets/{setId}/views
```

- `filter` (compact string) and `filters` (structured array) are mutually
  exclusive; **both supplied → 400 `ambiguous_input`** ("provide `filter`
  or `filters`, not both") (R15). One internal tree.
- **The filter string is a build item [build]** (R6): SPEC §6.2.1 reserves
  the grammar as a *post-v1, dataview-scoped* extension — no parser
  exists, and search scope adds design deltas (the string uses RFC 3339
  dates / preset functions; the structured form uses unix numbers — the
  §6.2.1 mapping applies). Position-addressed parse errors with
  did-you-mean, validated against the type's real property keys and
  option names.
- **Small-model form is settled** (C13): the structured `filters` array is
  recursive and not constrained-decodable, so the string is the documented
  default for small models; the array serves round-trip and programmatic
  composition.
- Sort by any property key. **[B3]** `resultFormat=rows` gated as before.

### Phase 5 — the task-tool wrapper (CLI + skill + on-device manifest)

The Phase-5 deliverable is the **task-tool wrapper** (§7): the curated
~9-tool layer over `/v2`, delivered as (a) CLI verbs (`anytype find | read |
describe | create | set-properties | add-blocks | edit-text | set-cell |
move | delete`) with AXI/Chow output conventions and SKILL.md three-tier
packaging for coding-agent harnesses, and (b) a function-calling/MCP
manifest of the same tools for on-device small models. Both are thin over
the same server primitives; bulk work via scripts. (Evidence §3.7; §7 for
the tool contract.)

## 3. Decisions ledger

**Decided**: id-addressed closed op set, no RFC 6902, no index/offset
addressing · `updateBlock` merge op + `replaceText` + `setCell` in the
launch op set (they back wrapper tools — §7/S1) · PUT-with-server-diff as
escape hatch (excluded from the small-model wrapper), full-block-id
round-trip default, diffStats · flat AnyBlock as the only content
representation on the REST write path; **markdown-in on the wrapper's
`add_blocks`** channel; markdown read-only on REST · compact object ids +
full block ids on REST reads / **short handles on wrapper reads** · one
vocabulary incl. `type` (C2) · per-endpoint schema + worked example,
strict-mode-compatible (C12/C13) · path-addressed errors + /validate +
dry_run + idempotency · etag advisory by default · filter string as the
small-model filter form · atomic composite creates (sets via initial-state
dataview) · **the small-model contract is the task-tool wrapper (§7), not a
REST mode**.

**Benchmark-gated**: B1 `replaceText`/`setCell` value for *large* models
(they are already launch primitives for the wrapper) · B2 structured-filters
value for mid/frontier programmatic composition (small-model primacy is
settled — R8) · B3 tabular result format · B4 wrapper-tool prompt/skill
guidance.

**Deferred**: dataview/view ops · cross-object batch · block-scoped
preconditions · conflict rebase · events/subscriptions · core-profile
strict schema (FLAT.md §7.3) · `?permanent=true` hard delete.

**Named build items** (exist nowhere today; budget them): filter-string
parser (search-scoped — now a launch dependency, backs `find`/`set` in the
wrapper) · `GenerateSchema` + store-backed option join (backs `describe`) ·
resolver wiring (create-missing properties/options) · referential
validation layer · md-export loss detector (converter/md has no warning
channel) · **markdown→flat-blocks parser for `add_blocks`** (the wrapper's
authoring channel) · **handle↔CID resolver** (the wrapper's reference
channel) · `anyblockjson.Options` id-compaction split (C4).

## 4. Benchmark program

All on the Phase-0 harness; small tiers run under constrained decoding
(R13). Metrics: apply-success, corruption (round-trip backtranslation),
output tokens, turns; per model tier.

- **B1 — edit primitives** (gates 3c): arms = launch ops · launch ops +
  `replaceText`. **PUT-only runs as the corruption baseline**, anchoring
  the DELEGATE-52 comparison — it is not a gate arm (PUT's existence is
  decided). Decision rule: `replaceText` ships iff it improves small-model
  apply-success or corruption on the one-word/sentence-edit tasks without
  regressing the rest.
- **B2 — structured filters for composition**: does the array ever beat
  the string for mid/frontier programmatic flows (round-trip, multi-step
  composition)? Scored by execution semantics, never string equality.
- **B3 — result format** (gates `resultFormat=rows`): reading-accuracy +
  tokens, compact JSON vs rows over 10/100/1000-row results.
- **B4 — creation guidance** (tunes prompt/SKILL.md guidance — *not* the
  C12 endpoint docs, which always ship schema + example): which in-context
  combination (example-only / schema+example / constrained core-profile)
  maximizes one-shot validity per tier.

## 5. Discovery

```
GET /v2/schemas                     # index: kinds, endpoints, examples
GET /v2/schemas/{kind}              # JSON Schema + worked example (object|type|property|set|collection|template|filters)
GET /v2/schemas/ops/{op}            # per-op tiny strict schema + single-op minimal example
```

Per-op fetch keeps the smallest consumers at the smallest schema surface
(§3.5 capability cliff); the 6-op composite example remains as a secondary
"multiple ops in one request" illustration. All generation-facing schemas
follow C13. The per-type artifact lives in Phase 1.

## 6. Rollout

1. Phase 0+1 behind an experimental flag; OpenAPI generated from day one.
2. Phase 2, then 3a/3b. **v1-parity note**: full parity additionally
   requires the Phase 1–2 surface additions of R11 (files, spaces/members
   reads, archive); §6's earlier "superset" claim is scoped to the object
   surface *including those*.
3. Benchmarks run once the harness + Phase 3a exist; gated items land in
   minor releases (additive to the closed op set).
4. Phase 4, Phase 5. v1 deprecation clock starts when the CLI ships.

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
mapping ~9 task tools to `/v2` primitives; it is exposed as **CLI verbs**
(coding-agent harnesses; the CLI-over-MCP finding) and as an **on-device
function-calling / MCP manifest** (3B models). It is the Phase-5 deliverable.
Full REST + wrapper = two surfaces sharing one AnyBlock format, one
validation contract, one error contract.

### 7.1 The three channels that close the review's structural findings

The wrapper's leverage is that a tool argument can be a friendlier channel
than the REST body:

- **Authoring channel = markdown** (not AnyBlock JSON). `add_blocks` takes a
  markdown string; the server parses it to flat blocks. Removes inline-
  markup-in-JSON escaping (the top 3B failure) and the relative-indent
  arithmetic (markdown indentation → server computes `indent`). [closes S2]
- **Editing channel = anchor + deterministic server edit.** `edit_text`
  takes `find`/`replace`; the server does the string replace in code and
  applies it via the `replaceText` primitive. The model supplies a short
  anchor, never reproduces the block. [closes S1's change-one-word collapse]
- **Reference channel = enumerated handles.** `find` returns `1,2,3`; `read`
  exposes short block labels; the wrapper resolves handles/labels → CIDs
  server-side. The model never sees or emits a 24-hex id. [closes S3]

### 7.2 Tool set (~9; flat, grammar-constrainable args)

| Tool | Args (flat) | Backing primitive | Channel notes |
|---|---|---|---|
| `find` | `space, query?, type?, filter?, limit?` | Phase 4 search | filter = string form; results are enumerated handles + minimal fields |
| `read` | `object, mode=full\|outline` | Phase 1 read | returns short block labels; `full` includes text |
| `describe` | `type` | Phase 1 `types/{type}/schema?flavor=table` | the accuracy lever, **called before create/set** (folds A1 into the flow) |
| `create` | `space, type, name, properties?, markdown?` | Phase 2 create | `type`/options validated with did-you-mean; no silent create-missing (A2) |
| `set_properties` | `object, {key: value}` | `setProperties` op | server coerces scalar→array, `@me`, relative dates |
| `add_blocks` | `object, after?\|under?, markdown` | `insertBlocks` op | **markdown channel**; server parses → flat blocks |
| `edit_text` | `object, block, find, replace` | `replaceText` op | **anchor channel**; deterministic server replace |
| `set_cell` | `table, row, col, value` | `setCell` op | flat cell write |
| `move_block` / `delete_block` | `object, block, after?\|under?` / `object, block, recursive?` | `moveBlock`/`deleteBlock` ops | handle-addressed |

Excluded from the wrapper: PUT full-document replace (the DELEGATE-52
corruption vector — REST-only, large models), multi-op batch (single tool
call per intent), the structured `filters` array (recursive, not
constrained-decodable — string only), relative-indent authoring (markdown
channel replaces it).

### 7.3 What the wrapper does NOT let us skip

1. **The bounded server primitives must exist** — `edit_text`/`set_cell` are
   safe only because `replaceText`/`setCell` are real server-side scoped ops
   (the S1 launch reweighting in §3). A wrapper that implemented them as
   GET+regenerate+PUT would reintroduce corruption.
2. **Constrained decoding still required, now tractable** — on-device
   function-calling (Ollama/llama.cpp GBNF) needs the *tool* schemas
   grammar-emittable; C13 applies to the small flat tool args here instead of
   the recursive block tree. The wrapper serves a GBNF/CFG artifact per tool
   (a Phase-5 build item), including the filter-string grammar for `find`.
3. **Server conveniences owned by the handler** — `@me` (+ `GET /members/me`),
   relative-date resolution on property values, scalar→array coercion,
   validate-not-create-missing, and If-Match/Idempotency-Key management
   (the model authors none of these; the wrapper does).

### 7.4 New build items for the wrapper

Beyond §3's list: markdown→flat-blocks parser (`add_blocks` authoring
channel) · handle↔CID resolver (reference channel) · per-tool GBNF/CFG
artifacts · `@me` self-resolution · relative-date input resolution. These
join `replaceText`/`setCell` (now launch ops) and the filter-string parser
(now a launch dependency) as the small-model launch set. Benchmark **B4**
tunes the wrapper's tool descriptions / SKILL guidance per model tier.

## 8. Implementation notes (v0.3.1 — closes Phase-0/1 gaps for a fresh build)

Concrete decisions a fresh implementer needs, so the design ambiguities that
would stall Phase 0/1 are resolved (genuine format/design ambiguities still
surface as spec bugs per the package's tradition).

**Server & auth.** `/v2` **extends the existing `core/api` gin server** — a
new route group under the same middleware stack (Recovery / Metadata /
Logger / Pagination / Cache / Auth / RateLimit / Analytics) and the same
layering as v1 (handlers in `core/api/handler`, services in
`core/api/service`, DTOs in `core/api/model`, routes in
`core/api/server/routes.go`; follow `core/api/CLAUDE.md` — fixture pattern,
mockery, error wrapping). **Auth is shared with v1**: `/v2` reuses the v1
bearer token, api-key, and challenge endpoints and the Auth middleware — no
new auth surface. Pagination is offset/limit as in v1.

**etag (C7), concrete.** The agent-facing token = first 8 hex of
`sha256(sorted object tree heads)` (`sb.source.Heads()` on the live
smartblock). Returned as the `ETag` header and the envelope `etag`. `If-Match`
comparison is done server-side against the **full** head-set (the 8-char form
is display only). It is NOT the `revision` relation. Advisory by default
(C7): absent `If-Match` ⇒ last-write-wins.

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

**Idempotency hash covers the query string.** `(space, key)` →
`sha256(body ‖ 0x00 ‖ rawQuery)`: a cached `?dry_run=true` result must never
replay as its later real twin — same key with a different query is a 409
`idempotency_conflict`, per C8's "different body" rule read as "different
request".

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

**R5 post-op validity without the whole-document Validate.** V1
monotonicity is now structural — a state tree has no indent arithmetic to
get wrong — plus the unchanged payload-run monotonicity pre-check.
Fragment payloads are validated by wrapping the run in a minimal
synthetic page document and running the format's document validation
(so the §5 shape checks apply verbatim); a failure rejects the whole
PATCH under the unchanged message "the ops would produce an invalid
document — no op was applied", with fragment-relative block paths.
Structural block types (`title`/`description`/`featuredProperties`) are
rejected explicitly in payloads (the whole-document import would have
absorbed them silently), and no primary-dataview pinning happens on
fragments. Id uniqueness — v0.3.3's V5 net — is an explicit check
against the live state and the PATCH's own claims, with op-shaped
`ops[i].blocks[j].id` paths (an improvement over the old document
paths); it also covers ids the format keeps out of the document (table
internals, structural blocks), which the old net could not see. The
op-shaped pre-checks (cycle, leaf containment, delete-without-recursive,
leaf-anchor) are unchanged, running against the view. Two knowingly
dropped document-level checks: total nesting depth beyond the format
bound and cross-op invariants no single fragment can violate — the
editor itself has no such limits, and the read side clamps depth (C11
warning). A **debug-gated safety net** (`ANYTYPE_API_V2_VALIDATE_EDITS=1`)
marshals + validates the would-be document read-only after the ops and
logs (never fails) any issue.

**Create-missing runs before the lock (v0.3.4, review B6/A6).** The only
create surface in PATCH payloads is setProperties select/multiSelect
option names; a lenient pre-pass resolves (and creates) them before
`MutateObject`, so no create-RPC ever runs while holding the edited
object's lock. In-lock resolution hits the resolver cache. Trade-off,
documented: an op that fails validation later can now leave an option
created (v0.3.3 leaked the same way for post-Unmarshal failures).

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

Four contract changes from the modification-surface design review, taken
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

**Idempotency-Key covers PATCH and PUT (C8 widened).** The store, body+query
hash, in-flight reservation and replay were POST-only wiring; agents
auto-retry on timeout, and PATCH is where a blind retry does damage (a
retried successful `insertBlocks` duplicates blocks; a retried `deleteBlock`
404s misleadingly). The middleware now acts on POST, PATCH and PUT and is
registered on the object PATCH/PUT routes and the types/properties PATCH
routes. Semantics unchanged: same key + same body+query ⇒ replay; different
body/query ⇒ 409 `idempotency_conflict`; only 2xx results are cached.
