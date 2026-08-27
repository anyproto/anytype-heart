# Anytype Local API v2 — specification and phased plan

Status: **draft v0.3** · 2026-07-23 · GO-7383 follow-on
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
| C4 | **Object ids are never compacted** — they are written in full, on every shape. The refs legend that used to shorten them is deleted from the format (SPEC §9a, v0.20): this API measured a **net token LOSS of 0.9–11.5% per document** from it, and it trapped write-back — an agent editing an object-valued property through a label has to keep the legend in step, and one that regenerates the document without it silently re-points every reference. The format's freeze review measured the same loss from the other end (a 200-item collection **+32.7%**). What is compacted is **block ids**: `?ids=compact` (the default read) relabels them to 5-char doc-local labels, which is safe precisely because that relabeling carries **no legend** — a label is a placeholder inside its own document, and write endpoints resolve one by unique-suffix match against the live object (SPEC §9a wiring allowance). `?ids=full` opts out and is the shape whose bytes re-import to the same document (the export/backup shape). Never require echoing a full CID. | ~24×/id, −89% id errors (§3.6); id round-trip contract (R1) |
| C5 | Minimal rows: list/search responses carry `id, name, type` + requested property values. **Never embed type objects.** `fields=` expands. | v1's N× multiplier (§2.1) |
| C6 | Error shape everywhere: `{status, code, message, issues:[{path, message, hint}]}` — path-addressed, naming allowed values. Required codes include: `validation_failed`, `version_unsupported` (surfaces SPEC §10's "produced by a newer version" verbatim, naming both versions), `idempotency_conflict` (same key, different body), `etag_mismatch`, `ambiguous_input` (e.g. both `filter` and `filters` supplied). Error text is API surface; test it. | repair loop (§3.2, §4.6); R15 |
| C7 | Every object read returns **`etag`** (short opaque token, ≤8 chars, derived from tree heads — NOT the object's `revision` property, which stays in `properties`) plus an `ETag` header. Mutations accept **`If-Match` header only** (the AnyBlock body has no envelope slot for it). **Advisory by default**: without `If-Match`, ops apply last-write-wins and `diffStats` reports the outcome; with it, mismatch → 409 `etag_mismatch` carrying the current etag. Note: the etag advances on background sync, not only on agent edits — strict If-Match will 409 on sync noise; block-scoped preconditions (ops apply iff the *addressed* blocks are unchanged) are the deferred v2.x refinement. | R2; optimistic concurrency (§3.2) |
| C8 | `Idempotency-Key` honored on all POSTs; replay with the same key returns the stored result; same key with a different body → 409 `idempotency_conflict`. Response always returns created ids. | agent auto-retry (§3.7); R15 |
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
  restructure a section · fill a table cell (expressed at launch as
  whole-`table` `replaceBlock` — R4) · create task with properties · build
  a set with filter. Model tiers: small (3–8B local), mid (Haiku-class),
  frontier. **The small tier runs under grammar-constrained decoding**
  (XGrammar-class) — without that floor, 3–8B loops produce null data and
  gates are undecidable (R13; §3.5 evidence).

### Phase 1 — read

```
GET /v2/spaces/{spaceId}/objects/{objectId}
    ?include=properties,blocks      # subset; default both
    ?outline=true                   # block skeleton, see below
    ?block={blockId}                # subtree only (contiguous indent-run)
    ?ids=compact|full               # BLOCK ids (C4); default compact.
                                    # object ids are always full — never compacted
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
into the moved block's own subtree is a cycle → error. `replaceBlock` /
`updateBlock` changing a parent's type to a leaf type while descendants
exist → error naming the descendant count.

**(a) `PATCH /v2/spaces/{spaceId}/objects/{objectId}` — batched ops (default path)**

```json
{ "ops": [
    { "op": "setProperties", "set": { "status": ["Done"] }, "unset": ["oldKey"] },
    { "op": "updateBlock",   "id": "b5", "set": { "checked": true } },
    { "op": "replaceBlock",  "id": "b3", "block": { "type": "paragraph", "text": "new **text**" } },
    { "op": "replaceSubtree","id": "b7", "blocks": [ { "type": "bulletedListItem", "text": "a" },
                                                     { "indent": 1, "type": "paragraph", "text": "b" } ] },
    { "op": "insertBlocks",  "after": "b3", "blocks": [ { "type": "checkbox", "text": "todo" } ] },
    { "op": "moveBlock",     "id": "b9", "inside": "b2", "position": "last" },
    { "op": "deleteBlock",   "id": "b4", "recursive": true }
  ] }
```

- Closed op set, id-addressed, atomic (one `state.Apply` per request), no
  positional/index/offset addressing anywhere (§3.1–3.2).
- **`updateBlock` (R4)**: merge semantics — only the fields in `set`
  change; everything else (including `text`) is untouched. The op for
  checkbox toggles, color/align changes, language switches.
  **`replaceBlock` replaces the whole block** (absent `text` = empty text
  per SPEC §4 — resend `text` or use `updateBlock`); descendants kept.
  `replaceSubtree` swaps block + descendants for the payload run.
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
- **`deleteBlock`**: `recursive` defaults to false; deleting a block that
  has descendants without `recursive:true` → error naming the descendant
  count (R14).
- **`setProperties`** (R14): `set` writes presence — `"k": []` means
  present-but-empty (SPEC §3 presence-is-meaningful); `unset` removes
  presence. Output-only properties (SPEC §4a) are rejected with a
  path-addressed error.
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
deferred): it backs the `set_cell` wrapper tool, and whole-`table`
`replaceBlock` for a one-cell change is the same verbatim-collapse trap as
(c). Flat, non-recursive — trivially grammar-constrainable.

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
