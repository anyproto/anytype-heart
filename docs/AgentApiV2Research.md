# Agent-native JSON API v2 — research report

Status: research synthesis · 2026-07-23 · branch `go-7383-anyblockjson`
Scope: adopting AnyBlock JSON (`pkg/lib/anyblockjson`, SPEC v0.5) in a fresh version of the
local REST API (`core/api`), designed for AI agents — including small models — with
token-efficiency and one-shot operation as primary goals. Breaking changes allowed;
the v1 API stays alive during migration.

Provenance: synthesized from a 13-agent research sweep (2 local codebase surveys +
6 web research angles + 4 targeted follow-ups; ~800k research tokens). Primary
sources cited inline; the load-bearing ones are benchmarks and first-party
engineering posts, not listicles. Numbers are quoted from sources, with
uncertainty flagged where the sweep flagged it.

---

## 1. Executive summary

The evidence from 2024–2026 agent-API work converges hard on a handful of design
choices, and AnyBlock JSON is already unusually well-positioned for them:

1. **Address edits by stable block id, never by array index or character offset.**
   This is the single most replicated finding (JSON Whisperer/EASE, Google Docs
   postmortems, Notion/Tiptap/BlockNote convergence). AnyBlock's
   optional-but-generated ids are exactly the right primitive — the API must
   return them on every read and require them on round-trip.
2. **Do not use raw RFC 6902 JSON Patch.** Use a small, validated, domain op set
   (`replace`/`insert`/`delete`/`move` by id, `setProperties`, scoped text
   replace), batched in one request. Constrained op sets beat raw patch on both
   reliability and tokens (PatchBoard: 84.6% vs 30.8% task success at ~8× fewer
   tokens).
3. **Whole-document replace exists but is an escape hatch, not the default.**
   It is nearly free to build (the import path's `ResetToVersion` diff-apply
   already does it), and it's needed for import/generation flows — but
   document-editing research (DELEGATE-52) shows whole-doc regeneration is the
   main *corruption* vector: models silently rewrite content they were told to
   preserve. Push agents toward the smallest sufficient granularity.
4. **Read granularity is the token win**: properties-only, blocks-only, outline
   mode (headings + ids, no bodies), subtree-by-id, compact ids by default.
   Notion's whole-page-only MCP (847 blocks loaded to change one number) is the
   cautionary tale; Anthropic measured 3× token swings from response-detail
   knobs alone.
5. **Never round-trip content through lossy Markdown for editing.** Atlassian's
   Rovo MCP silently destroys every rich node that Markdown can't express.
   AnyBlock's "JSON skeleton + inline-md text runs" is the correct hybrid —
   Notion independently converged on the same shape (markdown + XML-ish tags).
6. **Filters: flat, SQL-flavored, restrictive.** The reserved compact filter
   string (SPEC §6.2.1) is validated by the evidence — SQL-shaped syntax
   inherits a ~2× pretraining reliability advantage over nested JSON query
   trees (SQL 47% vs MongoDB MQL 21.5% zero-shot at equal difficulty), and a
   join-free single-type grammar sits in the benchmark tier where even 3B
   models hit 70–85%. Keep the structured filter array too (round-trip/compose);
   accept both, canonicalize internally.
7. **Creates are one-shot and atomic per kind**: `POST` a full AnyBlock document
   for objects; one transactional call for a type *with* its properties and
   default view (SPEC §2a `typeProperties` + create-missing already defines
   this semantics); server defaults everything defaultable. Ship **one worked
   example + the JSON Schema per endpoint** — examples moved parameter accuracy
   72%→90% in Anthropic's measurements and are what small models actually use.
8. **The validation loop is the product surface.** Path-addressed, actionable
   errors (the package's `ValidationError` already emits them) + a callable
   `validate`/dry-run endpoint + idempotency keys. Error-guided retry
   measurably beats generic retry; ~3 attempts is the standard budget.
9. **Ship a companion CLI (+ SKILL.md).** Three vendors independently measured
   85–99% context reductions from code/CLI surfaces over preloaded tool calls;
   a purpose-built agent CLI beat both raw CLI and MCP on cost, turns, and
   success in the one head-to-head benchmark that exists. For bulk operations
   ("update 200 objects") a scriptable CLI is the only token-sane path.
10. **Don't invent exotic wire formats.** Compact JSON in/out (never
    pretty-print: a free 38–46% cut), markdown text runs, familiar op shapes.
    TOON-style tabular output is worth offering *only* for large uniform query
    results, gated behind an on-target-model benchmark — it degrades small
    models on anything nested.

---

## 2. Where we are

### 2.1 The current v1 API, assessed as an agent surface

Source: `core/api/docs/v1/openapi.yaml` (Anytype-Version 2025-11-08) and
`core/api/service/object.go`.

What it is: `/v1/spaces/{space_id}/{objects|types|properties|lists|search|…}`,
bearer auth, offset pagination, snake_case models. Object bodies are
**markdown-only in both directions** — there is no block schema anywhere in the
OpenAPI file. `get_object` has a `format` param whose enum is exactly `[md]`.

The agent-relevant pain points, concretely:

- **Body editing is destroy-and-repaste.** `update_object.markdown` is
  implemented as `ObjectShow` → `BlockListDelete` (everything except header) →
  `BlockCreate`+`BlockPaste`. Changing one word requires GET full markdown →
  client-side string edit → PATCH full markdown. Anything markdown can't
  express (dataviews, table cell blocks, callout icons, mentions) is lossy
  through this path — the exact Rovo-MCP failure class (§3.3).
- **Read/write property asymmetry.** Read: 6-field self-describing objects
  (`{object, id, key, name, format, text}`); write: 2-field `{key, text}`. An
  agent cannot re-POST what it read; it must transform. AnyBlock's
  `properties: {key: value}` map fixes this by construction.
- **id/key duality with no resolver.** Types and properties are referenced by
  `key` in write payloads but by `id` in read responses and path params; the
  translation requires client-side list-and-filter.
- **N× type re-embedding.** Every row of a list/search response embeds the full
  `Type` object *including its entire nested `properties` array*. A 100-row
  list response repeats the type schema 100 times.
- **No partial reads, no sub-object addressing, closed sort enum** (4 values —
  can't sort search by a custom property), two parallel filter syntaxes
  (query-param mini-DSL on list endpoints vs recursive `FilterExpression` on
  search), 12 format-specific `FilterItem` shapes where the value field is
  *named after the format* (`text`/`select`/`date`…) — an agent must know the
  property's format to even name the value field.
- **Flat errors.** `{object, status, code, message}` with no field path — a 400
  from `create_object` doesn't say which field failed. (The AnyBlock validator
  already produces path-addressed issues; v1 just has nothing like it.)

Templates are read-only; DELETE is archive-only; select filter values are tag
*ids* while select write values accept key-or-id — same concept, different
rules by context.

### 2.2 What AnyBlock JSON gives the API

From SPEC v0.5 (`pkg/lib/anyblockjson/SPEC.md`):

- One JSON document per object: `{version, kind?, id, type, properties{key:value},
  refs?, blocks[nested tree], items?}` — readable, generatable from one
  example, strictly validatable (draft 2020-12 schema + semantic checks),
  lossless round-trip with a defined normalization.
- Inline markdown subset inside `text` (no offsets anywhere), tables and
  dataviews first-class, block ids optional on input / generated on import,
  **CompactIds** (refs legend + 5-char labels) and **OmitIds** marshal options,
  type documents (`kind: "objectType"`) with `typeProperties` and
  create-missing-property import semantics, select options addressed by
  *name* with create-missing behavior.
- `Marshal`/`Unmarshal`/`Validate`/`DetectFormat` public API, path-addressed
  `ValidationError`, explicitly designed for a generate → validate →
  feed-errors-back loop (§12) because neither vendor's strict decoding handles
  recursive schemas.

Nearly every one of these choices independently matches what the 2024–2026
evidence says to do (details in §3). The format is the asset; the API's job is
to not squander it.

### 2.3 What the middleware can already do (feasibility)

From the local survey of `core/block` and `pb/protos`:

- **Whole-state diff-apply to an existing object exists and is battle-tested.**
  `state.NewDocFromSnapshot` → `SetParent(liveState)` →
  `history.ResetToVersion`/`sb.Apply` diffs the new state against the current
  one (LCS over block ids in `change.go:468–984`) and emits minimal CRDT
  changes. This is exactly what `objectcreator.updateExistingObject` uses for
  import-update-in-place. A "PUT full AnyBlock document" endpoint is close to
  free: `anyblockjson.Unmarshal` → snapshot → diff-apply.
- **The diff matches blocks by id, not content.** If the submitted document
  drops ids, every edit degenerates into full-tree delete+recreate (event
  fan-out, broken undo granularity, no "small diff"). **Id round-trip is the
  load-bearing contract** for both the PUT path and any patch path.
- **~70 fine-grained `Block*` RPCs** (create/move/text/table/dataview/…) exist,
  all addressed by `contextId + blockId (+targetId + Position)` — never by
  index. A patch endpoint can either fan out to these (reusing restrictions/
  undo/hooks) or, like the RPC handlers themselves, open one state, apply N
  mutations, and `Apply` once — one CRDT change set per request. The second
  path is how a batched op endpoint should be built.
- **Guards to respect**: `canUpdateObject` excludes Relation/RelationOption/
  FileObject/Participant from whole-state reset; `resetState` has a revision
  regression guard; title and layout-derived structural blocks are
  editor-owned (`injectDerivedDetails`/`resolveLayout`) — the SPEC already
  keeps them out of documents (§7), which is the right API answer too: title
  is a property, never an editable block.
- Anything routed through `sb.Apply` gets undo, restrictions, and hooks for
  free; a path bypassing it loses all three. Both the PUT and PATCH paths must
  route through `Apply`.

---

## 3. Evidence

Organized by design question. Numbers are from primary sources unless flagged.

### 3.1 How agents edit: format reliability

- **Content-anchored search/replace is the most reliable emit format for
  capable models.** Diff-XYZ (arXiv 2510.12487): exact-match diff generation —
  search/replace 0.94–0.95 (GPT-4.1/Claude-4-Sonnet), 0.68 (Qwen2.5-32B);
  udiff 0.81/0.82; a verbose bespoke marker format scored **0.08** for
  GPT-4.1. Reliability tracks pretraining familiarity — *do not invent a novel
  edit DSL*.
- **Line numbers and offsets are the anti-pattern.** Aider: "GPT is terrible at
  working with source code line numbers"; dropping them (udiff without
  `@@ -a,b` numbers) took GPT-4-Turbo from 20%→61% on a laziness benchmark.
  OpenAI's V4A apply_patch deliberately has no line numbers ("context is
  enough"). Google Docs `batchUpdate` (character indices) is the negative
  exemplar: index drift forces descending-order request hacks, and the
  ecosystem's verdict is "everyone ends up deleting and rewriting entire
  documents."
- **Anthropic's `str_replace` semantics are the template for text edits**:
  exact match required, host enforces uniqueness ("Found 3 matches… provide
  more context"), errors are instructive. All major coding agents converged on
  this shape. Caveat from postmortems: exact-match is also the largest single
  source of edit *failures* (models reproduce target text slightly wrong) —
  the tool must absorb imprecision (fuzzy fallback, normalized whitespace)
  and return match-count errors.
- **Fuzzy, forgiving apply harnesses matter enormously**: disabling aider's
  flexible patching increased edit errors **9×**; high-level (whole-unit)
  edits over surgical line edits cut errors 30–50%.
- **Small models change the calculus.** Diff-XYZ: small open models "benefit
  little from any formatting choice" — you must *minimize what they emit*.
  A str_replace tool took an open model from ~28%→~51% pass@1 on SWE-bench Pro
  (QwenLM #179). Fast-apply (Morph: 7B merge model, ~98% merge accuracy,
  50–60% token savings on lazy edits) shows the workaround shape: let the
  generator be sloppy/abridged, make a deterministic server merge.
- **Tree-structured editing specifically**: Liveblocks/Tiptap *abandoned*
  streamed node-id edit ops ("the model could easily lose track of structure,
  break references, or output malformed operations") in favor of full-doc
  rewrite in strict markup with **server-side diffing**. Tiptap's shipped AI
  Toolkit exposes exactly 5 tools; edits are `[op, target, content]` with
  `replace`/`insertBefore`/`insertAfter` addressed by node id or range.
  BlockNote gives the model three tools (add/update/delete block) addressed by
  id. The convergent lesson: *few ops, id-addressed, whole-node payloads*.

### 3.2 JSON Patch and structured mutation

- **RFC 6902's documented LLM failure modes** (JSON Whisperer, arXiv
  2510.04717, EMNLP-Industry 2025): (a) array-index arithmetic — models
  miscalculate post-remove shifts and conflate 0-/1-based; (b) fragmented
  patches miss related updates. Plus JSON Pointer's `~0`/`~1` escaping tax.
- **The fix is stable keys**: their EASE encoding (arrays → dicts with stable
  2-char keys + a `display_order` field) makes patches order-invariant.
  Patch+EASE: **−31% tokens at within-5% quality of full regeneration**,
  −42% cost (Claude). AnyBlock block ids + sibling order *are* EASE applied
  natively.
- **Constrained op sets beat raw patch**: PatchBoard (arXiv 2605.29313,
  preliminary) — validated, schema-grounded ops with `remove` gated: 84.6%
  task success vs 30.8% (LangGraph baseline), 45.5k vs 368k tokens.
- **JSON Merge Patch (RFC 7386)** is right for exactly one thing here: the flat
  `properties` map (natural partial updates). It cannot surgically edit
  arrays and conflates null with delete — never use it for `blocks`. (Note:
  AnyBlock property presence-vs-absence semantics mean the API's property
  merge needs an explicit delete sentinel distinct from "set to empty";
  see §4.3.)
- **One batched patch document beats per-op tool calls**: batching studies
  show ~74% fewer calls / ~41% fewer tokens / ~57% faster on multi-field
  tasks; id-addressing removes the ordering dependency that makes batching
  dangerous.
- **Preconditions**: a `test`-on-version / If-Match ETag aborts atomically on
  stale context — optimistic concurrency "for free," needed once multiple
  agents/devices touch one object.

### 3.3 What document platforms actually did

- **Notion**: rejected markdown for block JSON in 2022, *came back to markdown
  for agents in 2025* ("Enhanced Markdown": GFM + XML-ish tags for mentions/
  callouts/toggles/columns). Hosted MCP is deliberately **whole-page
  replace only** — no block endpoints — accepting overwrite risk to keep the
  surface simple; a teardown measured 847 blocks loaded to change one number.
  Third-party clone (easy-notion-mcp) claims 6–7× token savings but is honest
  that the win is **metadata omission** (per-block UUIDs/timestamps/authors),
  not markdown-vs-JSON syntax.
- **Atlassian**: the cautionary tale *and* the working pattern in one company.
  Rovo MCP's lossy ADF→markdown→ADF edit path **silently deletes** every node
  markdown can't express (smart links, mentions, status, panels, tables,
  macros — atlassian-mcp-server#60). Their creation canvas instead has the
  LLM emit **ADF JSON directly**, passed through a proprietary **repair
  library** that fixes near-miss schema violations, with editor-style ops
  (`replaceNode`/`insertNodeAfter`/`removeNode`) in a reflection loop.
- **Tiptap/BlockNote**: §3.1 — schema-validated JSON models, id-addressed
  block ops, chunked positional reads (`tiptapRead(from)`), selection-scoped
  reads.
- **Canvas systems** (Claude Artifacts `update`, OpenAI Canvas): exact-string
  replace + whole-doc `rewrite` as a two-mode split — targeted edit by
  default, rewrite as the explicit escape hatch.
- Transferable synthesis: keep structure as JSON; markdown only inside text
  runs; id-addressed ops + scoped text replace; granular reads; repair layer
  over hard rejection; **surface what a lossy view omits** (easy-notion-mcp
  returns warnings listing dropped blocks) so a rewrite never deletes what the
  model never saw.

### 3.4 Query and filter DSLs

- **SQL-shaped strings inherit a massive pretraining prior.** SM3-Text-to-Query
  (NeurIPS'24): the same questions rendered to four query languages —
  SQL 47.05% > Cypher 34.45% > MongoDB MQL (nested JSON) 21.55% > SPARQL 3.3%
  zero-shot. Best available head-to-head for "flat string vs nested JSON
  query," and the nested-JSON language loses ~2×.
- **A join-free single-type filter grammar sits in the easy tier.** Spider 1.0
  (filter-heavy, single-DB): fine-tuned 3B models hit 70–85% execution
  accuracy; enterprise SQL (Spider 2.0, joins/CTEs/huge schemas) collapses to
  17–23% even for o1-class. Forbidding joins/subqueries/functions keeps
  agents in the high-accuracy regime — the SPEC §6.2.1 grammar already does.
- **Syntax validity is the small-model failure mode, and it's solvable**:
  grammar-constrained decoding lifts validity from 46.5%→92.6% (Llama 3) /
  36.6%→87.0% (Phi-3 Mini); modern engines (XGrammar-class) are near-zero
  overhead. The *semantic* bottleneck is intent→property/value mapping:
  Jackal (text-to-JQL, 23 models) shows the same model swinging ~35%→~85%
  based purely on how precisely the request names fields — which argues for
  investing in property-key discoverability (per-type schema/example
  endpoints) more than in filter syntax.
- **Don't make agents author a query language for *shape*** (field selection).
  Apollo's own conclusion: LLM-written GraphQL "adds a source of syntax
  errors"; their fix is human-curated persisted operations. Use `include`/
  `fields` params for shape; reserve the DSL strictly for filtering.
- Exact-string equality is useless for validating generated filters (Jackal:
  0.08% exact-match on semantically-correct queries) — validate by parse +
  execution semantics, return path/position-addressed parse errors.

### 3.5 Structured outputs, constrained decoding, small models

- **Anthropic strict structured outputs: recursive schemas are flatly
  unsupported** (first-party docs, GA). OpenAI supports recursion but caps
  ~5 nesting levels / 100 properties / 15k schema chars, with opaque errors.
  FSM-based open engines (Outlines, lm-format-enforcer) can't do recursion;
  PDA/CFG engines (XGrammar, llguidance) can. Deeply nested schemas are the
  dominant constrained-decoding failure mode (JSONSchemaBench: GitHub-Hard
  → Guidance 41%, Outlines 3%).
- Consequence, already anticipated by SPEC §12: **the nested AnyBlock document
  cannot be a vendor strict-mode schema; the validate→repair loop is the
  guardrail.** What *can* be strict-mode: the patch-op envelope (each op is a
  tiny flat object), create-property/type shapes, filters — keep those
  schemas small, non-recursive, `additionalProperties: false`-able.
- **Small models**: sub-7B tool calling is near-zero raw; 3–4B models produce
  unparseable JSON 23–36% of the time; escaping inside JSON strings is a top
  failure even for GPT-4o (37% on escape-translation tasks — relevant tax on
  inline-markdown-in-JSON, mitigated by tolerant parsing). Schema volume is a
  capability cliff — fewer required fields per op is a direct reliability
  lever. Constrained decoding fixes syntax (86–96% coverage) but not
  semantics ("constraint tax": valid-but-wrong outputs persist).
- **Reason-then-format**: forcing chain-of-thought inside rigid JSON cost
  Claude-3-Haiku 63pts on GSM8K ("Let Me Speak Freely", with the dottxt
  rebuttal showing matched prompts + visible schema erase most of the gap).
  Practical rule: always show the schema *and one worked example*; let
  reasoning happen outside the constrained field.

### 3.6 Token efficiency

- **Compact JSON is a free 38–46% cut vs pretty-printed.** Never pretty-print
  agent responses.
- **Long opaque ids are poison**: each UUID ≈ 24 tokens and models mutate them
  (BAML: 48.5 errors with raw UUIDs vs ~6 with integer handles on a 200-item
  aggregation ≈ 89% error reduction). 59-char CIDs are worse than UUIDs.
  CompactIds' refs legend is exactly the right mitigation — make it the
  default for agent reads, and never require echoing a full CID.
- **Metadata omission, not syntax, is where big savings live** (the
  easy-notion-mcp 6–7× result). Strip per-block/per-row metadata the agent
  didn't ask for; don't re-embed type schemas per row.
- **Tabular formats for uniform result sets**: TSV/CSV −60%, markdown table
  −54% vs pretty JSON; TOON −40% with format-aware headers. But TOON degrades
  small models and nested data ("not safe as a default in multi-turn agentic
  systems"; ranked last on nested data in one independent eval), and a
  9,649-trial study found format doesn't significantly move aggregate
  accuracy. Verdict: compact JSON default; optional CSV-ish/TOON-ish rows
  for large uniform query results, only after benchmarking on target models.
- **Partial reads / outline-first**: filtering before context is the 98.7%
  class of win (Anthropic code-execution numbers); outline-then-fetch is the
  established reading pattern for large docs.
- Response envelopes: naked compact resource; skip HATEOAS noise (an
  `available_actions` name list is the pragmatic middle if affordances are
  wanted).

### 3.7 CLI and code execution as the agent interface

- Anthropic "Code execution with MCP": 150k→2k tokens (−98.7%) by exposing
  tools as a discoverable code API with progressive disclosure. Cloudflare
  Code Mode: 1.17M→~1k tokens over 2,500 endpoints. Anthropic Tool Search:
  −85% context, accuracy up (Opus 4: 49%→74%).
- The one head-to-head benchmark (490 runs, GitHub tasks): a purpose-built
  agent CLI ("AXI") beat raw `gh`, MCP, Tool Search, and Code Mode
  simultaneously — 100% success, $0.074/task, 4.5 turns. MCP burned tokens
  2–3× faster than CLI.
- Agent-CLI design canon (AXI, Chow, Speakeasy, clispec): non-interactive by
  default; `--json`; minimal default fields + `--fields`; truncation *with
  hints*; explicit empty-state messages; pre-computed aggregates; typed exit
  codes; strict rejection of unknown flags (hallucination guard); `--dry-run`;
  stdin for document payloads; a `--describe`/schema verb; SKILL.md packaging
  with three-tier progressive disclosure (~100-token metadata → <5k
  instructions → scripts on demand). Google (`gws`), GitHub (`gh skill`),
  Stripe all shipped human+agent CLIs in 2025–26.
- Idempotency/dry-run evidence: agents auto-retry, so mutations need
  idempotency keys; dry-run-as-planning-feedback ("what would change") is
  called the highest-leverage reliability affordance in the agent-API
  literature.

### 3.8 Document editing is not code editing — the corruption result

The follow-up sweep on *prose/document* (not code) editing produced the most
sobering result:

- **DELEGATE-52** (arXiv 2604.15597; 19 models × 52 domains, round-trip
  backtranslation): after 20 delegated edits, frontier models corrupt
  documents 19–29% (~50% average across models), **primarily by rewriting
  spans they were told to preserve** — paraphrases that still "parse," so the
  damage is silent. Agentic file tools made it *worse* (models chose
  whole-file rewrite for 45–81% of tool calls). No plateau at 100
  interactions.
- Verbatim reproduction collapses past a few hundred tokens (zero perfect
  transcriptions at N=300 across 11 models); models paraphrase even when told
  to quote (production text-anchoring pipelines require fuzzy alignment).
- Multi-turn prose editing degrades faster than code (FineEdit: Wiki BLEU
  0.85→0.70 across turns while code holds).

Implication: the two research threads that seem to conflict — "small models do
better rewriting than patching" (§3.1 Liveblocks) vs "whole-doc regeneration
corrupts" (DELEGATE-52) — reconcile into one rule: **let models rewrite, but
only at the smallest sufficient scope, addressed by id, with the server doing
the diff.** A block or subtree rewrite bounds both the emission burden (small
models) and the blast radius (corruption). A whole-document rewrite bounds
neither.

Notable gap: no public benchmark isolates id-node-ops vs search/replace vs
whole-rewrite on a Notion-like block tree. Whatever we pick should be validated
with an in-house eval on target models (see §5).

---

## 4. Recommended design

### 4.1 Principles

1. AnyBlock JSON is the wire format for object content — one representation
   for read, create, and full replace. No lossy markdown edit path, ever.
2. Stable block ids are the addressing primitive everywhere; the server
   returns them on every read and preserves them across round-trips.
   Positional/index addressing does not exist in the API.
3. Every mutation routes through `sb.Apply` (undo, restrictions, hooks) and
   lands as one CRDT change set per request.
4. Minimal tokens by default: compact JSON, compact ids, minimal fields,
   partial reads. Verbosity is opt-in, never opt-out.
5. Validation errors are a first-class interface: path-addressed, actionable,
   with allowed values, designed for a ≤3-attempt repair loop.
6. Per-op schemas stay tiny, flat, and strict-mode-compatible; the recursive
   document schema is handled by the validate→repair loop instead.
7. One-shot wherever a task has a natural single intent (create type with
   properties and view; create object with full content; batch ops in one
   patch).

### 4.2 Read surface

```
GET /v2/spaces/{space}/objects/{id}
  ?include=properties,blocks     # any subset; default: both
  ?outline=true                  # headings + block ids/types only, no bodies
  ?block={blockId}               # subtree rooted at a block
  ?ids=compact|full              # default compact (refs legend + short labels)
  ?format=anyblock|md            # md is read-only convenience for prose flows
```

- Default response is the AnyBlock document (compact JSON, CompactIds), with
  the envelope `id` full (per SPEC §9a) and a `revision` field for
  concurrency (see §4.3).
- `outline=true` is the entry point for large documents: the
  outline-then-fetch pattern, and the cheap way for an agent to find the
  subtree it needs before a scoped read/edit.
- If a requested representation omits content (e.g. `format=md` can't express
  a dataview), the response carries a `warnings` array naming what was
  dropped — the easy-notion-mcp pattern, so rewrites never silently delete
  what the model never saw. (`format=md` content is never accepted back for
  update.)
- List/search rows are minimal by default: `{id, name, type(key), …requested
  property values}` — no embedded type objects, no sync/derived metadata.
  `fields=` expands. Types are fetched once via their own endpoints.

### 4.3 Edit surface

Three modes, in order of preference. All accept `If-Match: <revision>`
(or a `revision` body field) and return the new revision; mismatch → 409 with
the current revision and a re-read hint. All support `?dry_run=true`
returning the computed diff summary + validation issues without committing.

**(a) Batched ops — the default (`PATCH /v2/spaces/{space}/objects/{id}`)**

A single request body with a small, id-addressed, mostly order-independent op
vocabulary (not RFC 6902):

```json
{
  "revision": "…",
  "ops": [
    { "op": "setProperties", "set": { "status": ["Done"], "dueDate": "2026-08-01T00:00:00Z" },
      "unset": ["oldField"] },
    { "op": "replaceBlock", "id": "b3", "block": { "type": "paragraph", "text": "New text with **marks**" } },
    { "op": "insertBlocks", "after": "b3", "blocks": [ { "type": "checkbox", "text": "Follow up" } ] },
    { "op": "moveBlock", "id": "b7", "inside": "b2", "position": "last" },
    { "op": "deleteBlock", "id": "b4" },
    { "op": "replaceText", "id": "b2", "find": "old wording", "replace": "new wording" }
  ]
}
```

Design points, each evidence-backed:

- **Op vocabulary is closed and small** (PatchBoard; Tiptap/BlockNote/Rovo all
  converged on 3–5 ops): `setProperties`, `replaceBlock` (payload may carry
  `children` → subtree replace), `insertBlocks` (`before`/`after`/`inside` +
  `position: first|last` — Notion's `after`-id pattern), `moveBlock`,
  `deleteBlock`, `replaceText`. Collection membership: `addItems`/`removeItems`.
  Nothing addresses by index; nothing takes offsets.
- **`setProperties` is merge-patch-shaped but with an explicit `unset`** —
  AnyBlock's presence-is-meaningful rule (§3 of the SPEC) means `null` must
  remain a legal stored value, so deletion is a separate list, not a null
  sentinel (avoiding RFC 7386's conflation).
- **`replaceText` is scoped `str_replace`**: exact match within one block's
  `text`, must match exactly once, errors follow the Anthropic template
  ("found 2 matches — provide more context", "no match — nearest was …"),
  with normalized-whitespace fuzzy fallback behind a conservative threshold.
  Scoping to a block keeps anchors short — directly mitigating the
  verbatim-reproduction failure (§3.8). `replace_all` flag for intentional
  multi-site edits within the block.
- **New blocks in op payloads need no ids** (server generates, response
  returns the id map); referenced ids resolve through the document's `refs`
  legend by the SPEC's total resolution rule, so agents can use the compact
  labels they read.
- Ops apply atomically (all-or-nothing) in one `Apply`; validation runs
  per-op with **op-index+path-addressed errors** (`ops[2].blocks[0].type:
  unknown block type "callout2"; closest: "callout"`).
- Gate blast radius: `deleteBlock` on a subtree with children requires
  `"recursive": true` (PatchBoard gates `remove`; cheap insurance).
- Table/dataview interiors start as whole-block `replaceBlock` (the SPEC's
  table/dataview JSON is compact enough); fine-grained cell/view ops are a
  post-v1 extension if usage shows the need.

**(b) Subtree/document replace — the rewrite escape hatch
(`PUT /v2/spaces/{space}/objects/{id}` and `replaceBlock` with children)**

Full AnyBlock document in, server diff-applies via
`Unmarshal → NewDocFromSnapshot → SetParent → ResetToVersion` (§2.3). Cheap to
build, needed for generation/import flows and large restructurings (the
Claude-Artifacts `rewrite` split). Two hard rules:

- **Ids must round-trip.** The docs/examples say it plainly: edit what you
  GET; don't strip ids. A submitted document whose ids don't match produces a
  full-tree rewrite — legal, but the response should include a
  `diffStats` (blocks added/removed/changed) so an agent (or eval harness)
  can notice "I meant to change one paragraph and rewrote 40 blocks" — the
  DELEGATE-52 corruption signature made visible.
- System kinds excluded per `canUpdateObject`; revision guard as in import.

**(c) Property-only update (`PATCH …/objects/{id}/properties`)** — the
`setProperties` op exposed standalone for the most common cheap mutation, so
simple flows never touch block machinery.

### 4.4 Create surface

Separate endpoints per kind, each one-shot and atomic, each documented with
**its JSON Schema and one worked example** (fetchable, and embedded in the
OpenAPI description — examples are the highest-leverage documentation
artifact per Anthropic's 72%→90% measurement):

```
POST /v2/spaces/{space}/objects        # body: AnyBlock document (or {type, name, properties, markdown} shortcut)
POST /v2/spaces/{space}/types          # body: kind:"objectType" document — typeProperties creates missing properties atomically (SPEC §2a)
POST /v2/spaces/{space}/properties     # body: {key?, name, format, options?[{name,color}]}
POST /v2/spaces/{space}/sets           # body: {name, typeKey, filters|filter, sorts?, views?}
POST /v2/spaces/{space}/collections    # body: {name, items?[]}
POST /v2/spaces/{space}/templates      # body: AnyBlock document with templateFor
```

- **`POST /objects` accepts a full AnyBlock document** — properties, blocks,
  inline tables/dataviews in one shot; ids optional (OmitIds-shaped input is
  the expected agent form); select option names and type keys resolve with
  create-missing semantics exactly as the SPEC's import wiring defines.
- **`POST /types` is the atomic composite**: type details + `typeProperties`
  (creating missing properties in the same transaction) + optional dataview
  block for default views. This kills the dangling-reference failure class
  (agents issuing type→property→set as separate calls produce half-built
  schemas). Server defaults everything defaultable (Notion/Airtable pattern:
  implicit name property, format defaults to `text`, layout default).
- **Referential validation with repair-shaped errors**: a set/dataview
  referencing a property key that doesn't exist on the type is rejected with
  the *actual available keys* in the error (schema-linking hallucination is
  documented and common — models invent plausible keys).
- **Idempotency-Key header** honored on all creates (agents auto-retry;
  duplicate pages are the classic symptom); response always returns created
  ids so the agent can verify.
- Discoverability endpoints (these do more for accuracy than any syntax
  choice, per Jackal's ~35%→~85% spec-detail swing):
  - `GET /v2/schemas/{kind}` — JSON Schema + worked example per create kind;
  - `GET /v2/spaces/{space}/types/{key}/schema` — the per-type derived
    artifact (SPEC §2a `GenerateSchema`): property table with formats and
    live select option names, prompt-ready. This is the one-call answer to
    "what can I write on a task."

### 4.5 Query surface

```
POST /v2/spaces/{space}/search
{
  "query": "…",                      # full-text
  "type": "task",                    # single type key (or list)
  "filter": "done = false AND (dueDate < currentWeek() OR dueDate IS EMPTY)",
  "filters": [ …structured form… ],  # mutually exclusive with filter
  "sort": [{ "property": "dueDate", "direction": "asc" }],
  "fields": ["name", "dueDate", "status"],
  "limit": 25
}
GET /v2/spaces/{space}/sets/{id}/objects?view={viewId}&fields=…&limit=…
```

- **Accept both filter forms; canonicalize to one internal tree.** The compact
  string (SPEC §6.2.1 — ship it, stop reserving it) is the recommended
  agent-authoring form: SQL-flavored tokens inherit the pretraining prior
  (§3.4), the grammar is deliberately join/subquery/function-free (the
  high-accuracy regime), option values are names, date presets are functions.
  The structured `filters` array remains for round-trip and programmatic
  composition — and per the flat-beats-nested evidence, its canonical form is
  the SPEC's: bare leaves at top level, groups only for `or`/nesting, never
  gratuitous depth.
- **Parse errors are position-addressed** ("filter, offset 23: unknown
  property `duedate` — did you mean `dueDate`? Available on task: …"),
  validated against the target type's actual property keys and option names.
- Sort by any property key (kills the v1 closed enum).
- **Result shaping**: `fields` selects property columns; default row is
  minimal (§4.2). Optional `resultFormat: "rows"` returns a compact tabular
  encoding (header + value rows) for large uniform results — added only if it
  wins an on-target-model benchmark (§3.6 caveats); compact JSON is the
  default regardless.
- Default `limit` small (~25, Linear-style), `has_more` + steering hint on
  truncation ("312 matches — narrow with filter or increase limit").

### 4.6 Errors, validation, safety

- One error shape everywhere:

```json
{
  "status": 400,
  "code": "validation_failed",
  "message": "document invalid (2 issues)",
  "issues": [
    { "path": "blocks[3].type", "message": "unknown block type \"callout2\"", "hint": "closest match: \"callout\"" },
    { "path": "properties.status", "message": "\"Doone\" is not an option of select property \"status\"", "hint": "options: [\"To do\", \"In progress\", \"Done\"]; unknown names are created only on import endpoints" }
  ]
}
```

  Path-addressed (the package's `ValidationError` already is), specific,
  carrying allowed values — the shape that makes error-guided retry work.
- `POST /v2/validate` — `anyblockjson.Validate` as a first-class endpoint
  (document in, issues out, nothing written). `?dry_run=true` on every
  mutation returns the would-be diff + issues. Dry-run-as-planning-feedback
  is the highest-leverage reliability affordance in the literature.
- **Repair layer over hard rejection** for near-misses (Rovo's ADF-repair
  precedent): tolerant input parsing per the SPEC (explicit defaults/empties,
  alias types like `heading4`/`equation`, attribute-order/quote tolerance in
  inline tags), plus API-level absorption of trivial slop (e.g. a `blocks`
  payload where a block is a bare string → paragraph). Anything auto-repaired
  is reported in `warnings` so behavior stays predictable.
- Scopes: read-only vs read-write API keys (Stripe's trust-boundary pattern);
  writes space-scoped. Prompt-injected content is a real path into a local
  agent (Supabase MCP exploit class) — read-only default keys for
  read-mostly integrations are cheap defense in depth.
- Rate-limit state proactively in headers so agents self-throttle.

### 4.7 The companion CLI (+ skill)

Evidence says this is not an accessory — for bulk work it's the primary
interface (§3.7). Thin client over the local HTTP API:

```
anytype search --space s1 --type task --filter 'done = false' --fields name,dueDate
anytype get s1/obj123 [--outline | --block b3 | --properties]
anytype create object --space s1 -f doc.json          # or stdin: -
anytype patch s1/obj123 -f ops.json [--dry-run]
anytype apply s1/obj123 -f full-doc.json [--dry-run]  # PUT semantics
anytype validate -f doc.json
anytype schema type|property|set|object [--type task] # worked example + schema
anytype types|properties --space s1
```

Conventions (AXI/Chow canon, §3.7): non-interactive always; compact JSON to
stdout, diagnostics to stderr; minimal default fields + `--fields`; truncation
hints; explicit "0 results" messages; exit 0/1/2; unknown flags rejected
loudly; `--dry-run` everywhere; stdin (`-`) for documents. Packaged with a
SKILL.md (three-tier: ~100-token metadata, <5k instructions with the worked
examples, scripts for bulk patterns like "export → edit → apply") so Claude
Code/Codex-class agents pick it up natively. Scripted loops over the CLI give
the code-execution win (intermediate data never enters context) without us
building a sandbox.

### 4.8 Versioning and migration

- New surface under `/v2/`; v1 untouched and maintained for the deprecation
  window. `Anytype-Version` date header continues for in-v2 evolution.
- v1's `get_object?format=` enum (`[md]`) is the natural seam: v2's `format`
  starts at `anyblock` with `md` as read-only legacy convenience.
- The AnyBlock schema's own versioning (SPEC §10 — additive 1.x, reject-newer
  with a clear error) covers document evolution independently of API
  versioning; the API should surface that error class verbatim ("produced by
  a newer version").

---

## 5. Open questions and risks

1. **Nested vs flat block representation for strict decoding.** The nested
   document can't be a vendor strict-mode schema (Anthropic: no recursion;
   OpenAI: ~5-level cap). Our answer is the validate→repair loop (SPEC §12)
   plus strict-friendly *op* schemas — but no public benchmark A/Bs
   flat-id-list vs nested-tree generation validity on identical payloads.
   Worth an internal eval with a 4–14B model before considering a flat
   read/write mode (`blocks` as id+parentId list) as an additive option.
2. **No public benchmark isolates id-ops vs scoped-str-replace vs rewrite on a
   block tree.** We're composing well-evidenced ingredients, not copying a
   proven whole. Build a small in-house eval harness (per-op apply-success and
   corruption rate per target model — DELEGATE-52's round-trip
   backtranslation trick is cheap to reproduce on AnyBlock docs) and let it
   gate format decisions; the research is unanimous that format reliability is
   model-dependent enough to require measuring on *your* formats.
3. **Escape tax on inline markdown inside JSON strings.** Documented failure
   area (GPT-4o 37% on escape translation; aider's "JSON-wrapping causes
   errors"). The SPEC's minimal-escaping canon + tolerant input parsing
   mitigate; the eval harness should measure it specifically (marks + `\n` +
   quotes in `text`).
4. **Concurrency semantics.** Local-first means the object can change under
   the agent (sync, user typing). Revision + If-Match gives loud failure;
   whether to add server-side rebase for non-conflicting ops (id-addressed
   ops make this tractable) is a post-v1 question.
5. **`replaceText` fuzzy threshold.** Fuzzy fallback is essential (§3.8) but a
   wrong fuzzy match is silent corruption. Start strict + normalized
   whitespace only; loosen with eval data.
6. **Tabular result format** — only behind a benchmark (§3.6).
7. **Bulk multi-object mutation** (Notion-3.0-scale "update hundreds of
   pages"): v2 answer is the CLI/script path over per-object endpoints; a
   server-side batch endpoint (`ops` across objects, transactional per
   object) is a natural later addition once single-object semantics settle.

---

## 6. Key sources

**Edit formats / patching**
- JSON Whisperer (EASE) — arXiv 2510.04717: RFC 6902 failure modes; stable-key
  fix; −31% tokens within 5% quality.
- Diff-XYZ — arXiv 2510.12487: per-format×model edit reliability;
  search/replace 0.95 vs bespoke DSL 0.08.
- Aider docs (edit formats, unified diffs, leaderboards): no line numbers;
  fuzzy apply 9×; format compliance by model tier.
- Anthropic text-editor tool docs; OpenAI GPT-4.1 prompting guide (V4A).
- PatchBoard — arXiv 2605.29313 (preliminary): constrained validated op set.
- Liveblocks/Tiptap AI copilot postmortem; Tiptap AI Toolkit docs; BlockNote
  AI reference; Atlassian Rovo canvas blog + atlassian-mcp-server#60.
- DELEGATE-52 — arXiv 2604.15597: document corruption by regeneration.
- FineEdit/InstrEditBench — arXiv 2502.13358; verbatim-transcription failures —
  arXiv 2601.03640.

**Platforms**
- Notion: hosted-MCP inside look, Enhanced Markdown docs, StackOne teardown,
  easy-notion-mcp (metadata-omission honesty).
- Google Docs batchUpdate concepts (index-drift guidance).

**Query DSLs**
- SM3-Text-to-Query (NeurIPS'24): SQL 47% > MQL 21.5%.
- Jackal text-to-JQL — arXiv 2509.23579: spec detail 35%→85%; syntax vs
  semantics split.
- Spider 1.0/2.0, FINER-SQL — arXiv 2605.03465 (small-model tiers);
  kirill-markin SQL-like DSL writeup; Martin Fowler "DSLs Enable Reliable Use
  of LLMs"; Apollo GraphQL agent post.

**Structured outputs / small models**
- Anthropic structured-outputs docs (no recursive schemas; caps); OpenAI
  structured outputs (recursion + 5-level cap); JSONSchemaBench — arXiv
  2501.10868; grammar masking — arXiv 2407.06146; SchemaBench — arXiv
  2502.18878; "Let Me Speak Freely" — arXiv 2408.02442 + dottxt rebuttal.

**Token efficiency**
- BAML UUID study (~89% error reduction from short handles); jangwook 9-format
  token measurements; TOON spec + critiques (arXiv 2605.29676, 2603.03306);
  Anthropic "Writing effective tools for agents" (ResponseFormat 206→72
  tokens; UUID→name precision) and "Advanced tool use" (examples 72%→90%).

**CLI / code execution**
- Anthropic "Code execution with MCP" (150k→2k); Cloudflare Code Mode
  (1.17M→1k); AXI principles + 490-run benchmark; Trevin Chow "7 Principles
  for Agent-Friendly CLIs"; Agent Skills authoring docs.

**Local codebase**
- `core/api/docs/v1/openapi.yaml`, `core/api/service/object.go` (markdown
  destroy-and-repaste), `core/block/editor/state/change.go` (id-matched
  diff), `core/block/import/common/objectcreator` (update-in-place via
  `ResetToVersion`), `pb/protos/service/service.proto` (Block* RPC
  inventory), `pkg/lib/anyblockjson/` (Marshal/Unmarshal/Validate).

---

## Addendum A — Flat blocks with an `indent` level (2026-07-23)

Follow-up question while the format is still a draft: should `blocks` stop
being a nested `children` tree and become a **flat pre-order array with a
per-block indentation level** — markdown-style, harder for an LLM to "forget
to close"?

```jsonc
// nested (current)
{ "type": "bulletedListItem", "text": "a",
  "children": [ { "type": "bulletedListItem", "text": "b",
    "children": [ { "type": "paragraph", "text": "c" } ] } ] }

// flat + indent (proposal)
[ { "type": "bulletedListItem", "text": "a" },
  { "type": "bulletedListItem", "indent": 1, "text": "b" },
  { "type": "paragraph", "indent": 2, "text": "c" } ]
```

Researched via 3 agents (prior art; LLM structure-failure evidence; local
spec impact) + an adversarial judge instructed to steelman both sides.

### Verdict

**Flatten — as a clean flat-only breaking change while the spec is a draft.**
Confidence: moderate-to-high on direction. But it is **conditional**: the
naive version of this proposal ships a bug that every flat precedent shipped
and never fixed (§A.3). Keep per-block JSON-object syntax with an explicit
integer `indent` (omitted when 0) — *not* significant whitespace, *not* a
TOON-style tabular syntax.

### A.1 Why flat wins (the two decisive factors are binary, not matters of degree)

1. **A non-recursive schema is the only door to the most reliable
   small-model generation path.** `schema/object.schema.json:75` makes
   `$defs/block` self-referential — exactly what Anthropic strict structured
   outputs reject outright and FSM-based constrained decoders (Outlines,
   lm-format-enforcer) cannot express; OpenAI caps ~5 levels. Constrained
   decoding is what rescues small models — the TOON generation benchmark
   (arXiv 2603.03306) measured a 7B model at 0% valid plain generation →
   75% under constrained decoding. The nested tree structurally forecloses
   this path on the strictest vendor; for a format whose stated goal is
   "generatable by an LLM one-shot," that is self-defeating. Flat removes
   the recursion entirely — the schema becomes strict-mode-compatible
   everywhere, at any document depth.
2. **Truncation and streaming degrade gracefully.** A flat array truncated at
   `max_tokens` is a valid prefix of blocks (drop the trailing partial
   element, close one `]`); nested truncation orphans every ancestor of the
   cut point. First-party confirmation: openai/codex#9504 documents exactly
   this pathology and its fix is element-boundary-aligned output; repair-tool
   telemetry (json_repair et al.) puts unclosed brackets/truncation at the
   top of the LLM-malformation distribution; NDJSON is the industry answer
   for the same reason.
3. **Depth, not length, is the transformer failure axis.** Bounded-Dyck
   theory (Yao, ACL 2021), depth-generalization results (arXiv 2512.02677:
   models "fail sharply when recursion depth increases even when they
   perform well on longer non-nested sequences"), and lost-in-the-middle
   (distant closers are exactly the error-prone case) all point one way.
   PARSE (arXiv 2510.08623) is the neat corroboration: an automated schema
   optimizer, tuning purely for extraction reliability, spent **55% of its
   edits flattening nested structures**. Code models emit indentation-as-
   structure ~96% correctly (only 4.08% IndentationErrors in generated
   Python, trivially auto-fixable) — an explicit integer is strictly easier
   than that.
4. **The format is already half-flat.** Tables and dataviews are flat sibling
   arrays validated by semantic checks, not by the recursive tree. Flattening
   the outer tree removes an internal inconsistency, and the semantic-check +
   path-addressed-error machinery flat needs already exists in `validate.go`.

Secondary: export is nearly free (thread a depth counter through the existing
walk); token cost is roughly neutral for shallow docs (marginal flat edge —
don't sell this as a token win); the API's patch ergonomics are unaffected
(id-addressed ops operate on the live tree, not on JSON shape).

### A.2 What it honestly costs

- **A new silent failure class.** Nested structure parses to exactly what its
  brackets say or fails loudly; a wrong `"indent": 3` where 2 was meant
  parses fine and silently reparents a subtree. This is the *documented,
  unfixed* bug of both shipping precedents of this exact encoding: Portable
  Text #4542 (child list becomes a sibling `<li>`, renumbering "completely
  out of whack") and Quill #979 (open since 2016) / #2221 — both because
  neither spec defined what an invalid level jump means. Mitigable (§A.3),
  and the residual (±1 errors) is local to one subtree, versus nesting's
  unclosed-bracket failure which is loud but corrupts the whole remainder.
- **Containment rules leave the schema.** `row→column` and the 12 leaf
  `children: false` rules are declarative `if/then` schema today, giving
  agents free path-addressed structural errors; flat moves them into
  hand-written indent-stack walkers in Go. Real cost, bounded — the pattern
  and the `Issue`/`ValidationError` reporting path already exist for tables.
- **Import rewrite.** `blockFromJSON`/`childrenFromJSON` recursive descent →
  iterative stack-based rebuild, plus a new validation class (non-monotonic
  indent) that cannot exist today. Export, by contrast, is ~a one-line
  change. Golden fixtures and the roundtrip comparator need migrating.
- §11's canon gains one clause (canonical indent normalization). Minor
  readability loss for humans hand-reading raw JSON (hierarchy becomes
  integers, not visible nesting).
- Non-transfer note: Portable Text's other pain (#3379, adjacent same-level
  lists indistinguishable) does **not** transfer — AnyBlock never had list
  containers (list items are siblings; numbering derives from consecutive
  siblings), so flat and nested are identical on this point.

### A.3 Mandatory conditions (without these, don't flatten)

1. **Specify the invalid-jump rule up front, in the spec** — adopt
   CommonMark's list-indent normalization: a block can never open a deeper
   level than `parent + 1`; any jump > +1 clamps to the nearest established
   ancestor's child level. Every flat precedent failed to define this and
   every one regretted it.
2. **Validate indent monotonicity on import** and report violations as loud,
   path-addressed errors (with the clamp applied only under an explicit
   lenient mode) — converting the would-be silent mis-parent into a boundary
   error, preserving the format's loud-failure property.
3. **Re-express `row→column` and the leaf no-children rules as semantic
   checks** with the same path-addressed granularity agents get from the
   schema today, so the self-validation story (SPEC goal 3) survives the move
   out of declarative JSON Schema.

Do a **flat-only break, not coexistence**: a `oneOf` nested/flat union schema
reintroduces the exact strict-mode friction flat exists to remove, and
permanently doubles the schema + import surface. If a bridge is wanted, the
only acceptable hybrid is flat-canonical (the published schema) + nested
*accepted on input* transitionally, with a sunset.

### A.4 The cheap experiment that settles the residual uncertainty

The biggest evidence gap is **real-world nesting depth** — in-repo signal is
thin (the 51 persisted roundtrip failure artifacts are all depth-1; the
crafted fixture is depth-3). `cmd/anyblockroundtrip` already exports the full
35,369-object account: one run with a depth histogram added answers it. If
p95 depth ≤ 3, the depth-generation argument narrows (the decision then rests
on Anthropic strict-mode + truncation — still pro-flat, smaller margin); if
the tail is deep, the flat case is overwhelming. Optional second step: a
small generation A/B (3–4B + mid model, flat non-recursive schema under
constrained decoding vs nested under the validate-repair loop) to quantify
the lift on this format specifically.

### A.5 Addendum sources

- Portable Text spec + issues #3379, #4542; Quill Delta design doc + issues
  #979, #2221, #3247; Google Docs API (bullet `nestingLevel`, 0–8, tables
  stay nested; re-indent = delete+recreate); Tana Paste docs (LLM-parseable,
  "small generative results" caveat); prosemirror-flat-list; mdast-flat.
- Yao et al., ACL 2021 (bounded Dyck); arXiv 2512.02677 (depth
  generalization); Liu et al., TACL 2024 (lost in the middle); PARSE —
  arXiv 2510.08623 (55% of schema-optimizer edits = flattening); arXiv
  2409.00676 (4.08% IndentationError in generated Python); TOON generation
  benchmark — arXiv 2603.03306 (JSON 75% vs TOON 44.6% one-shot; constrained
  decoding 7B 0%→75%); openai/codex#9504 (truncation breaks nested JSON);
  json_repair; CommonMark list-indent rules.
- Local: `schema/object.schema.json` (recursive $ref at :75; row→column
  :211-220; 12 leaf types), `export.go` walk (:400–585), `import.go`
  recursive descent (:389–520, pre-order comment :386), `validate.go`
  flattened-tree id uniqueness (:265–311), `testdata/rich*.json`,
  roundtrip-out depth scan.
