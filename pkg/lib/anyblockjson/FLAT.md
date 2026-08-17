# Flat AnyBlock — research dossier and work brief

> **Vocabulary note (SPEC v0.8):** this document predates the `snake_case`
> rename and spells identifiers as the draft did then (`bulletedListItem`,
> `objectId`). The encoding it describes is unchanged; for current spellings
> see SPEC §1 *Naming*.


Status: approved-for-implementation draft · 2026-07-23 · GO-7383, branch `go-7383-anyblockjson`
Audience: the agent implementing the flat block encoding in `pkg/lib/anyblockjson`.
This document is self-contained: it carries the decision, the evidence (with
references), the normative format changes, the per-file work plan, and the
acceptance criteria. The companion documents are `SPEC.md` (v0.5, to be
edited per §4 below) and `docs/AgentApiV2Research.md` (Addendum A — the
decision record this brief operationalizes).

Ground rule inherited from this package's development style: **ambiguity is a
spec bug**. If anything here is ambiguous while implementing, that is a
finding to surface, not to silently resolve.

---

## 1. The decision

`blocks` changes from a nested `children` tree to a **flat pre-order array
with a per-block `indent` integer** (omitted when 0):

```jsonc
// before (nested, SPEC v0.5)
{ "type": "bulletedListItem", "text": "a",
  "children": [
    { "type": "bulletedListItem", "text": "b",
      "children": [ { "type": "paragraph", "text": "c" } ] } ] }

// after (flat)
[ { "type": "bulletedListItem", "text": "a" },
  { "indent": 1, "type": "bulletedListItem", "text": "b" },
  { "indent": 2, "type": "paragraph", "text": "c" } ]
```

- Flat-only breaking change (the format is a draft with no external
  consumers; format `version` stays `1`). No `oneOf` nested/flat union —
  a union schema reintroduces the strict-mode friction flat exists to
  remove and permanently doubles the schema+import surface.
- Blocks stay JSON objects; hierarchy is the explicit integer — **not**
  significant whitespace and **not** a TOON-style tabular syntax (evidence
  §2.5).
- The three **mandatory conditions** from the decision record are normative
  requirements of this work (§3.3–§3.5): a spec'd invalid-jump rule, loud
  monotonicity validation, and containment rules re-expressed as
  path-addressed semantic checks.

**Product datum that closes the last open question:** real Anytype documents
nest to **~6 levels at the typical maximum** (user-supplied, 2026-07-23).
Depth 6 exceeds the sub-7B practical nesting ceiling (~3 levels) and sits
above OpenAI's documented ~5-level schema-nesting comfort zone — so the
depth-reliability argument for flat is live for our actual content, not
hypothetical. (The roundtrip depth histogram in §6.4 verifies this number
against the full corpus.)

## 2. Evidence dossier (why flat, with references)

### 2.1 Flat unlocks constrained decoding — the small-model generation path

- The current schema is genuinely recursive: `schema/object.schema.json:75`
  (`children: {items: {$ref: "#/$defs/block"}}`) plus a second recursion
  path through table cells (`$defs/tableCell` → `$ref: "#/$defs/block"`).
- Anthropic strict structured outputs list **"Recursive schemas"** as
  unsupported (GA, first-party):
  https://platform.claude.com/docs/en/build-with-claude/structured-outputs
- OpenAI supports recursion but documents hard caps (~**5 nesting levels**,
  100 properties, 15k schema chars) with opaque errors:
  https://community.openai.com/t/measuring-maximum-depth-and-object-properties-in-structured-outputs/918388
- FSM-class constrained decoders cannot express recursion at all
  (lm-format-enforcer: "Large, infinite, and recursive schemas are not
  supported" — https://github.com/noamgat/lm-format-enforcer; Outlines per
  the vLLM structured-decoding writeup —
  https://blog.vllm.ai/2025/01/14/struct-decode-intro.html). PDA/CFG engines
  (XGrammar — arXiv 2411.15100; llguidance) can, but portability requires
  not depending on it.
- Deeply nested schemas are the dominant constrained-decoding failure mode:
  JSONSchemaBench (arXiv 2501.10868) — GitHub-Hard nested schemas drop to
  Guidance 41% / Outlines 3% coverage.
- Constrained decoding is what rescues small models: the TOON generation
  benchmark (arXiv 2603.03306) measured Qwen2.5-Coder-7B at **0% valid plain
  JSON generation → 75% under constrained decoding**.
- Small-model baseline rates that make this matter: 3–4B models emit
  unparseable JSON 23–36% of the time (SchemaBench, arXiv 2502.18878).

A flat block schema is non-recursive at any document depth → usable under
every strict/guided decoder, including on depth-6 documents.

*Honest caveat:* flatness removes the **categorical** blocker (recursion),
but the ~30-branch type discriminator may still strain vendor strict-mode
complexity caps (e.g. Anthropic's union limits). §7.3 proposes a derived
"core profile" schema as the follow-up answer; do not let it block this work.

### 2.2 Truncation and streaming degrade gracefully

- Nested JSON truncated at `max_tokens` orphans every open ancestor;
  first-party report: openai/codex#9504 ("JSON and XML structured data gets
  truncated in ways that break parseability") —
  https://github.com/openai/codex/issues/9504
- Repair-tool telemetry foregrounds unclosed brackets/braces and truncated
  streaming values as the primary LLM malformations (json_repair —
  https://github.com/mangiucugna/json_repair; the 288-call analysis,
  blog-grade, ranks malformed brackets the "leading cause").
- The NDJSON/JSONL consensus exists for exactly this reason: one
  self-contained value per element keeps any prefix parseable.
- In flat form the only unclosed obligation is the single outer `]`; a
  truncated document is a valid prefix of blocks. In nested form a cut at
  depth 6 loses six closing scopes.

### 2.3 Depth, not length, is the transformer failure axis

- Bounded-Dyck theory: transformers process bounded-depth hierarchical
  languages; capacity is tied to depth (Yao et al., ACL 2021, arXiv
  2105.11115).
- Depth generalization collapses even when length generalization holds
  (arXiv 2512.02677: models "fail sharply when recursion depth increases…
  even when they perform well on longer but non-nested sequences").
- Lost-in-the-middle (Liu et al., TACL 2024) explains the mechanism for
  distant closing brackets: the opener is mid-context by closing time.
- PARSE (arXiv 2510.08623): GPT-4 shows an 11.97% invalid-response rate on
  complex nested extraction schemas, and an automated schema optimizer
  spends **55% of its edits flattening nested structures** — flattening
  independently rediscovered as the dominant reliability lever.
- Sub-7B models have a practical ~3-level nesting ceiling (practitioner
  benchmarks; consistent with the center-embedding literature, Lampinen,
  Computational Linguistics 50:4). Our documents reach ~6.

### 2.4 Prior art — the encoding is proven, and its one bug is documented

- **Portable Text** (Sanity) — flat array + `listItem`/`level` int; the
  closest shipping precedent. Spec: https://github.com/portabletext/portabletext
  Documented pain: level jumps have no canonical parent — nested items
  "drop to a new parent list item… renumbers all subsequent items
  incorrectly" (issue #4542); adjacent-list ambiguity (issue #3379 — does
  NOT transfer to us, see below). The spec never defined an invalid-jump
  rule; renderers improvise.
- **Quill Delta** — flat ops + `indent` attribute
  (https://quilljs.com/docs/delta/). Same bug class: #979 (open since
  2016), #2221 (renumbering), #3247 (no depth cap).
- **Google Docs API** — flat paragraphs + bullet `nestingLevel` 0–8;
  tables stay nested; re-indent is delete+recreate
  (https://developers.google.com/workspace/docs/api/concepts/structure).
- **Tana Paste** — indented plain text explicitly pitched as LLM-parseable
  ("output from external AIs… automatically parsed"), scoped to small
  generations (https://tana.inc/docs/tana-paste).
- **prosemirror-flat-list** — deliberately replaced the nested
  `bulletList > listItem` model ("insane logic to keep numbering
  synchronized") (https://github.com/ocavue/prosemirror-flat-list).
- Contrast (nested camp): ADF, ProseMirror content-expressions, mdast —
  their advantage is schema-enforced containment, which we re-provide as
  semantic checks (§3.5).
- **Non-transfer note:** Portable Text #3379 (two adjacent same-level lists
  indistinguishable from one) does not apply to AnyBlock — the format has
  no list containers at all; list items are consecutive siblings and
  numbering is derived from adjacency (SPEC §5, `numberedListItem`).
  Flat and nested are identical on this point.
- **The lesson to implement, not just note:** every flat precedent shipped
  without an invalid-jump rule and regretted it. CommonMark's list-indent
  normalization (content-offset rule; under-indent pops to the nearest
  enclosing level; a deeper-than-established level cannot be opened) is the
  one mature spec of the needed rule: https://spec.commonmark.org/0.31.2/#lists

### 2.5 What NOT to do (counter-evidence)

- Do not use whitespace-significant or novel tabular syntax: TOON generated
  far worse than plain JSON one-shot (44.6% vs 75.0%; 0% on deep
  hierarchy) — arXiv 2603.03306. The win comes from staying inside JSON
  fluency and removing recursion, not from exotic compaction.
- Do not sell token savings: nested pays `,"children":[…]` (~4–6 tokens per
  parent), flat pays `"indent":N,` (~3–4 tokens per non-root block); for
  typical shallow-mostly documents this is a wash with a marginal flat
  edge.
- The explicit integer sidesteps the silent whitespace-miscounting failures
  of markdown/YAML nesting (markdown lists silently flatten past ~2–3
  levels — e.g. go-gitea#4009), and the best proxy for "can models emit a
  correct per-line structure integer" is code indentation: 94.7% of
  generated Python runs error-free with only 4.08% IndentationErrors,
  auto-fixable (arXiv 2409.00676).

### 2.6 The cost being accepted (and its mitigation)

A wrong `indent` parses cleanly and silently mis-parents one subtree —
nested structure had no such class (brackets are loud). Mitigations, all
normative here: the clamp rule (§3.3), strict-by-default monotonicity
validation making violations loud (§3.4), and the fact that the residual
(±1 errors within an established range) is local to one attachment point,
whereas nesting's signature failure (unclosed bracket / truncation)
corrupts the remainder of the document.

### 2.7 Why not YAML (asked and answered — verified mid-2026)

The obvious follow-up: if the format is flat and indentation-shaped, why
not go all the way to YAML? Verified against current sources (2026-07);
every leg of the case holds:

- **No engine can enforce it.** All three vendors' structured-output modes
  are JSON-Schema-only (OpenAI, Anthropic `output_config.format:
  json_schema`, Gemini `responseSchema`); no YAML mode exists. OSS
  engines: Outlines closed the YAML grammar request as *not planned*
  (dottxt-ai/outlines#871); XGrammar/lm-format-enforcer/vLLM/SGLang
  support JSON/regex/CFG only. Root cause is theoretical, not backlog:
  YAML's indentation and plain-scalar rules are **context-sensitive**
  (stateful INDENT/DEDENT lexing, unbounded lookahead) — not expressible
  as a CFG, so PDA/CFG engines cannot fully enforce it. Choosing YAML
  forfeits constrained decoding — the decisive factor behind this whole
  redesign (§2.1).
- **JSON generates more reliably, and the gap widens on small models.**
  StructEval (arXiv 2505.20139): text→format generation accuracy JSON vs
  YAML — GPT-4.1-mini 99.3% vs 95.3%; Qwen3-4B 91.0% vs 78.7%;
  **Phi-3-mini 68.8% vs 36.9%**. The format targets small models; YAML is
  worst exactly there.
- **The token argument is backwards.** YAML only beats *pretty-printed*
  JSON (−23 to −31%); **compact JSON beats YAML** (−37.5% flat / −45.7%
  nested, tiktoken measurements). The 2023 "YAML function calling is
  cheaper" claim compared against pretty JSON; a finetuning follow-up
  found the saving small (7–16%) and JSON *more accurate* at 1B/3B.
- **Whitespace-significance is the silent failure we just engineered
  out.** F1–F8 exist because counted whitespace fails silently past ~2–3
  levels; YAML makes the entire document whitespace-significant.
- **Implicit typing corrupts user data.** Norway problem (`NO` → false),
  `9.3`/`3.10` → floats, `Null` → null, auto-dates; YAML 1.2 removed some
  of this but the dominant parsers (PyYAML) still default to 1.1
  semantics. "JSON is valid YAML" holds only for 1.2 and only one
  direction — a JSON document fed to a 1.1-default parser can silently
  change meaning. Disqualifying for a data-interchange pipeline.
- **The 2026 YAML tailwind is the other use case.** Frameworks adopting
  YAML (Microsoft Agent Framework, Pydantic AI agent specs) use it for
  *human-authored, version-controlled config* — not model-generated
  interchange, where payloads remain JSON everywhere (MCP, vendor SDKs).

What flat+indent keeps from YAML: its one good idea (indentation as
hierarchy) as an explicit, machine-checkable integer instead of counted
whitespace. What YAML keeps in our ecosystem: the **`.md` export surface**
— YAML frontmatter for properties in the lossy markdown format is the
Obsidian-ecosystem idiom and the right place for it (that exporter, not
this package). YAML's genuinely attractive block scalars (`text: |`, no
`\n`/quote escaping) are acknowledged; the JSON escape tax is mitigated by
tolerant parsing and the repair loop (§2.6 of the API research), not by
switching serialization.

References for this subsection: OpenAI structured outputs docs ·
Anthropic structured outputs docs · Gemini structured output docs ·
dottxt-ai/outlines#871 · ktbarrett "Parsing YAML" (context-sensitivity) ·
"Principled parsing for indentation-sensitive languages" (POPL 2013) ·
StructEval arXiv 2505.20139 · jangwook tiktoken format measurements ·
Mohapatra JSON-vs-YAML function-calling finetune ·
hitchdev StrictYAML "why implicit typing was removed" ·
john-millikin.com "JSON is not a YAML subset".

---

## 3. Normative format changes

These are the rules to encode into SPEC.md, the schema, and the package.
Numbered for reference from code comments and tests.

### 3.1 The `indent` field

- **F1.** `blocks` is a flat array in **pre-order** (a parent precedes its
  descendants; a subtree is a contiguous run).
- **F2.** Every block MAY carry `indent` (integer ≥ 0). Absent = `0`
  (top level). Canonical form **omits** `indent: 0` (existing
  omit-empty-and-default canon, SPEC §4).
- **F3.** Canonical key order: `indent` is written **first**, before `id`
  (`indent`, `id`, `type`, …). Rationale: reading linearly, structure
  before identity; and generation-wise the model commits the cheap
  structural token first. (Implementation decision — if the user prefers
  `id` first, this is a one-line change in `json.go`; flag it in review.)
- **F4.** Resource bound: `indent` > **32** is a validation error
  (adversarial-input guard in the style of SPEC §8.2's bounds; typical
  real maximum is ~6, so 32 is generous).
- **F5.** `children` is removed from the format entirely. A document
  containing a `children` key fails schema validation (unknown property) —
  there is no legacy-input mode. (The importer of *old drafts* is not a
  goal; nothing shipped.)

### 3.2 Tree reconstruction (import semantics)

- **F6.** Reconstruction algorithm (normative): walk the array with a
  stack seeded `(root, indent = −1)`. For block *i* with indent *k*:
  pop the stack until the top's indent is *k − 1*; the top is the parent;
  append block *i* to the parent's children; push `(block i, k)`.
- **F7.** Validity (strict, the default): the first block's indent MUST
  be 0; block *i*'s indent MUST be ≤ (indent of block *i−1*) + 1.
  Violations are **errors** (§3.4), not warnings.

### 3.3 The clamp rule (lenient mode only)

- **F8.** `Options.NormalizeIndent bool` (import-only, default false).
  When set, an over-deep indent (jump > +1) is **clamped to
  (previous block's indent + 1)** — CommonMark's "you cannot open a level
  that hasn't been established" rule — and a first block with indent > 0
  is clamped to 0. Every clamp is reported as a warning-grade issue with
  the block's JSON path. Negative indents and indents > 32 are errors even
  in lenient mode.
- Rationale: strict default preserves the loud-failure property for the
  generate→validate→repair loop; the lenient mode exists for API-level
  repair layers and hand-authored input, and it must never be silent.

### 3.4 Validation rules moving from schema to semantic checks

All report through the existing `Issue`/`ValidationError` path
(`validate.go:41–117`) with the same path-addressed granularity the schema
gives today (`blocks[7]`-style paths). New/updated semantic checks:

- **V1** (monotonicity): F7 violations. Error text must name both indents:
  `blocks[7]: indent 3 follows indent 1 — a block can be at most one level
  deeper than its predecessor`.
- **V2** (leaf types): the 12 types that today carry `children: false` in
  the schema (embed and its `equation` alias, bookmark, link, divider,
  table, property, dataview, icon, tableOfContents, featuredProperties,
  chat — see `object.schema.json` if/then blocks) cannot be parents: if block *i* has
  one of these types and block *i+1* has indent > indent(*i*), error at
  `blocks[i+1]` naming the parent type.
- **V3** (row→column): a block whose parent (per F6) is a `row` MUST be a
  `column` — replaces `object.schema.json:211–220`. Keep parity with
  today: the reverse direction (a `column` outside a `row`) is NOT
  currently enforced; do not add it silently (note as an open item, §7.2).
- **V4** (bounds): indent ∈ [0, 32] (schema expresses `minimum: 0`; the
  upper bound can live in the schema too — put it there, it costs
  nothing).
- **V5** (id uniqueness): unchanged — already computed over the flattened
  tree (`validate.go:265–311`); the walker just becomes a loop.

### 3.5 Tables stay grid literals (and cells, the second recursion path)

**Tables are NOT flattened into the blocks array — normative.** `indent`
encodes the outline dimension; a table is two-dimensional, and flattening
its grid into indent runs would make the cell→column correspondence a
positional count across runs — the exact index-arithmetic failure class
this redesign eliminates (JSON Whisperer, §2). Every flat precedent keeps
true containers encapsulated (Google Docs: flat paragraphs, nested tables;
Portable Text: structure inside custom blocks), and markdown itself uses
row-per-line syntax for tables, not indentation. SPEC §6.1's existing
encoding — one `table` block with `columns`/`rows` arrays, string-shorthand
cells in column order — is already the JSON twin of a GFM table row and is
the pretraining-familiar shape. It stays as is; a `table` block is simply
one element of the flat array (a leaf per V2). Do not confuse table grids
with *layout* `row`/`column` blocks — those are outline structure, DO live
in the flat array (`row` at *k*, `column`s at *k+1*), and are guarded by V3.

- **F9.** `tableCell` remains `string | null | block object` (SPEC §6.1
  unchanged). *(Corrected post-implementation:)* dropping `children` alone
  does **not** make the cell `$ref` non-recursive — the
  `block → table arm → tableRow → tableCell → block` cycle survives it. The
  implemented cut: cells reference a dedicated **`cellBlock`** definition
  (shared `blockCore`, no table arm, `type ≠ table`), so **cells cannot
  contain `table` blocks** at any depth, and export errors loudly on such
  data (none observed in prod — ANOMALIES #11). A cell block object carries
  **no `indent`** (error if present, alongside the existing "no `id`"
  rule).
- **F10.** Cell descendants: today `cellFromJSON` (`table.go:295–297`)
  builds "any nested children" of a cell block. In flat form a lone block
  object cannot carry descendants. Rule: a cell with descendants is
  represented as an **array of flat blocks** (`tableCell = string | null |
  block | [block…]`, the array form using `indent` internally, first
  element indent 0). Export uses the array form only when descendants
  exist; single-block cells stay bare (canonical).
  **Verification item (do this first):** measure how often real cell
  blocks have children (grep the roundtrip corpus / add a counter to the
  §6.4 sweep). If effectively zero, prefer the simpler rule — cells are
  childless, export drops cell descendants with a warning — and record it
  in ANOMALIES.md. Decide with data; if in doubt, implement F10 as
  written (lossless).

### 3.6 Round-trip canon (SPEC §11 updates)

- Export emits pre-order with exact depths — export can never produce an
  F7 violation, so `Export ∘ Import` byte-stability is unaffected.
- Add to the normalization list `N(S)`: nothing new for strict inputs; for
  lenient (`NormalizeIndent`) inputs, clamped indents are part of the
  documented normalization.
- The §11 cycle/duplicate rule ("each block emitted once, first parent
  wins", `export.go:91` `visited`) is unchanged — it's about the snapshot
  graph, not the JSON shape.
- OmitIds, CompactIds: semantics unchanged (CompactIds' 5-char block-id
  relabeling and refs legend are orthogonal to array shape).

### 3.7 Schema changes (`schema/object.schema.json`)

- `$defs/block`: remove `children`; add `indent` (`type: integer`,
  `minimum: 0`, `maximum: 32`). The def becomes **non-recursive**.
- Delete the 12 `children: false` if/then arms and the row→column arm
  (`:211–220`) — now V2/V3 semantic checks. Keep the per-type prop
  if/then dispatch (SPEC §12's discriminator-first validation) as is.
- `tableCell`: per F9/F10 (`anyOf` gains the array form if F10 stands;
  `indent: false` and `id: false` on the bare-block form).
- Top-level `blocks` unchanged (`array` of block).
- Keep `$id` at `https://schemas.anytype.io/anyblock/1.0/object.schema.json`
  and `version: 1` — draft, nothing shipped. (If the schema URL was ever
  handed to anyone, bump instead; check with the user — §7.2.)

### 3.8 SPEC.md edit list

Add a "Changes from v0.5" block at the top, then:

- §2 envelope table: `blocks` description → "flat pre-order array; nesting
  via `indent` (§4)".
- §4 (Blocks — common structure): replace the `children` row with `indent`
  (F1–F5); update canonical key order (F3); add F6–F8 as a new "Nesting"
  subsection; keep alignment/backgroundColor/fields rows unchanged.
- §5 inventory: the "children" notes per type become "may be a parent /
  leaf" — fold the 12-leaf list into one sentence referencing V2; `row`
  keeps its "children must be columns" note (now V3).
- §6.1 tables: cell rule per F9/F10.
- §7 structural blocks, §8 inline, §9/§9a ids: unchanged (state it).
- §11: per §3.6. §12: list V1–V5 among the semantic checks. §13: add
  `NormalizeIndent` to `Options`; note the flat schema is non-recursive
  and why (one line, cite Addendum A).
- §14 full example: convert to flat form (it becomes shorter — that's the
  point; keep the same content).

---

## 4. Work plan (per file, with current anchors)

Recommended sequence — each step leaves the package green:

1. **Verification pass** (read-only): confirm the 12 leaf types against
   both schema and `export.go`'s `withChildren = false` sites
   (`export.go:453–528` — these are the same list expressed in code; they
   must agree, discrepancies are findings). Run the cell-children counter
   (§3.5). Check whether anything outside the package consumes the JSON
   shape (`cmd/anyblockroundtrip` treats Marshal/Unmarshal as a black box —
   `main.go`; `typeproperties.go` is unaffected).
2. **Schema** (§3.7) + `Validate`: new indent field, drop recursion; add
   V1–V4 semantic checks in `validate.go` (the `walkBlock` recursion at
   `:291–311` becomes a single loop threading an indent stack; `claimId`
   at `:265–275` unchanged).
3. **Export**: `childrenToJSON`/`blockToJSON`/`finishBlockJSON`
   (`export.go:400–585`) — thread a `depth int` through the existing walk
   and append to a flat slice instead of nesting the return value; emit
   `indent` per F2/F3 via the canonical writer (`json.go` key order).
   The `withChildren` logic keeps meaning "descend or not".
4. **Import**: replace the recursive descent (`blockFromJSON` `:389`,
   `childrenFromJSON` `:520`) with the F6 stack rebuild. Note the comment
   at `import.go:386–387` — the internal representation is *already*
   pre-order, which is why this is a rewrite of the loop, not of the data
   flow. Implement F7 strict + F8 lenient. Table path: `cellFromJSON`
   (`table.go:295`) per F9/F10.
5. **Tests** (§5) and golden migration: convert the four
   `testdata/rich*.json` fixtures (write a tiny throwaway converter or
   hand-convert — they're small); regenerate via `golden_gen_test.go
   -update` only **after** hand-reviewing the converted shape.
6. **SPEC.md** (§3.8) and ANOMALIES.md if the cell decision (§3.5) drops
   anything.
7. **Prod sweep** (§6.4) via `cmd/anyblockroundtrip` with the depth
   histogram added.

Explicitly out of scope: the HTTP API surface (docs/AgentApiV2Research.md
§4 — separate work), the `pkg/lib/schema` retirement, CompactFilters.

## 5. Test plan

Beyond keeping every existing test green (they assert state-level
round-trip equality, so most are shape-agnostic):

- **Property tests** (`roundtrip_test.go`): generators already produce
  nested states — add depth-heavy cases (chains to depth 10+, mixed
  wide/deep, depth ≥ 6 to cover the real maximum) and assert
  `Export ∘ Import` byte-stability over them.
- **Prefix property** (new, this is the truncation claim made testable):
  for any exported document, every prefix of the `blocks` array (with the
  envelope intact) passes `Validate` — i.e. pre-order + F7 guarantees
  prefix validity. If this property does NOT hold for some construct,
  that's a spec finding to surface.
- **V1–V4 error cases**: first block indent 1; jump 0→2; indent under a
  leaf type (one per category, not all 12); non-column under row; indent
  33; negative indent; `indent` present on a table-cell block; explicit
  `indent: 0` canonicalized away on re-export.
- **Lenient mode**: clamp 0→3 lands at 1 with a path-addressed warning;
  first-block clamp; identical final state to the equivalent valid
  document.
- **Error-message quality**: assert the V1 message names both indents
  (these errors are the agent-facing repair loop — treat message text as
  API).
- **Golden files**: converted fixtures byte-stable; CompactIds/OmitIds
  variants unchanged in behavior.

## 6. Acceptance criteria

1. All package tests green; new tests of §5 present.
2. `schema/object.schema.json` contains **no recursive `$ref` among the
   block/cell definitions** (ref-graph walk proves it; the dataview
   `filterNode` tree stays recursive by design — nested filter groups,
   SPEC §12) and still validates the converted `rich.json`.
3. `Validate` errors for V1–V4 are path-addressed and name the offending
   values.
4. **Prod sweep parity**: rerun `cmd/anyblockroundtrip` over the full
   account (35,369 objects at last run); pass rate ≥ run 3 (99.86%) with
   only the known residue (~7 duplicate-name option swaps, see
   ANOMALIES.md). Any new failure category is a blocker, not a footnote.
5. **Depth histogram** added to the sweep summary (p50 / p95 / max block
   depth per object, plus a count of cells-with-children for §3.5) —
   verifies the "~6 typical max" datum and records it in ANOMALIES.md or
   the sweep summary.
6. SPEC.md updated per §3.8 with a v0.6 changelog block; SPEC and
   implementation agree (spot-check F1–F10 against code).

## 7. Decisions ledger

### 7.1 Decided (do not relitigate)

- Flat + explicit integer `indent`, omit-0, pre-order (Addendum A verdict).
- Flat-only break; no nested/flat union schema; no legacy-input mode (F5).
- Strict monotonicity by default; clamp only under `NormalizeIndent` (F7/F8).
- Containment rules become path-addressed semantic checks (V2/V3).
- JSON-object blocks; no whitespace/TOON syntax.

### 7.2 Ask the user (small, don't block on them collectively)

- F3 key order (`indent` first vs after `id`).
- F4 bound value (32 proposed).
- V3 reverse direction (`column` only under `row`) — add or keep parity?
- Schema `$id`/`version` untouched (assumed fine while draft) — confirm
  nothing external consumes the published schema URL yet.
- F10 vs the simpler childless-cell rule, once the counter (§3.5) reports.

### 7.3 Follow-ups (record, don't do)

- **Core-profile schema**: a derived, reduced block schema (paragraph,
  headings, list items, checkbox, quote, code, callout, divider, toggle)
  sized to fit vendor strict-mode complexity caps for full-document
  generation; generated one-way like the §2a type artifacts. The full
  schema remains the validation authority.
- A small generation A/B (3–4B + mid model) on this format: flat schema
  under constrained decoding vs validate-repair loop — quantifies the
  §2.1 lift on our own format (the public literature has no direct
  flat-vs-nested A/B; JSONSchemaBench methodology is a template).
- **GFM table input alias**: accept `{ "type": "table", "markdown":
  "| a | b |\n|---|---|\n| 1 | 2 |" }` as an input-only alias, parsed to
  the structured form on import, never emitted — the alias pattern the
  spec already uses (`equation`, `heading4`). Rationale: small models
  emit GFM tables natively. Costs: a GFM-row parser, `|`-escaping rules,
  plain-string cells only. Optional; needs a user call.
- API PATCH ops (docs/AgentApiV2Research.md §4.3) — unaffected by this
  change (id-addressed, operate on the live tree), but `insertBlocks`
  payloads become flat lists with `indent`, which composes naturally.

## 8. References

Primary sources (web):

- Anthropic structured outputs (recursive schemas unsupported; caps):
  https://platform.claude.com/docs/en/build-with-claude/structured-outputs
- OpenAI structured outputs (recursion + ~5-level/100-prop caps):
  https://openai.com/index/introducing-structured-outputs-in-the-api/ ·
  https://community.openai.com/t/measuring-maximum-depth-and-object-properties-in-structured-outputs/918388
- JSONSchemaBench — schema validity vs complexity (GitHub-Hard: 41%/3%):
  https://arxiv.org/html/2501.10868v1
- lm-format-enforcer (no recursion): https://github.com/noamgat/lm-format-enforcer ·
  vLLM structured decoding (FSM vs PDA): https://blog.vllm.ai/2025/01/14/struct-decode-intro.html ·
  XGrammar: https://arxiv.org/pdf/2411.15100
- TOON vs JSON generation benchmark (JSON 75% vs TOON 44.6% one-shot;
  constrained decoding 7B 0→75%): https://arxiv.org/html/2603.03306
- SchemaBench (small-model parser-error rates 23–36%):
  https://arxiv.org/html/2502.18878v1
- Bounded Dyck / depth limits: Yao et al., ACL 2021,
  https://arxiv.org/abs/2105.11115 · depth generalization:
  https://arxiv.org/abs/2512.02677 · lost in the middle:
  https://aclanthology.org/2024.tacl-1.9/
- PARSE (11.97% invalid on nested; 55% of optimizer edits = flattening):
  https://arxiv.org/html/2510.08623v1
- Truncation breaks nested JSON (first-party):
  https://github.com/openai/codex/issues/9504 · json_repair:
  https://github.com/mangiucugna/json_repair
- Portable Text spec + issues: https://github.com/portabletext/portabletext
  (#3379, #4542) · Quill Delta: https://quilljs.com/docs/delta/
  (quill #979, #2221, #3247) · Google Docs structure:
  https://developers.google.com/workspace/docs/api/concepts/structure ·
  Tana Paste: https://tana.inc/docs/tana-paste ·
  prosemirror-flat-list: https://github.com/ocavue/prosemirror-flat-list
- CommonMark list-indent rules (the clamp rule's source):
  https://spec.commonmark.org/0.31.2/#lists
- Code-indentation proxy (4.08% IndentationError, auto-fixable):
  https://arxiv.org/html/2409.00676v1 · markdown silent flattening:
  https://github.com/go-gitea/gitea/issues/4009
- JSON Whisperer / EASE (stable keys over positions, −31% tokens):
  https://arxiv.org/html/2510.04717v1

Repo anchors (verified 2026-07-23, this branch):

- `schema/object.schema.json` — recursive `$ref` at `:75`; row→column
  `:211–220`; 12 `children: false` arms; `tableCell` → `$ref: block`.
- `export.go` — walk `:400–585`; `withChildren` sites `:453–528`;
  `visited` cycle-break `:91`.
- `import.go` — recursive descent `:389–520`; pre-order comment
  `:386–387`.
- `validate.go` — `Issue`/`ValidationError` `:41–117`; `claimId`
  `:265–275`; `walkBlock` `:291–311`.
- `table.go` — `cellFromJSON` (cell children) `:295–297`; string-cell
  canonicalization `:140–155`.
- `testdata/rich*.json` (4 fixtures, max depth 3) · `golden_gen_test.go`
  (`-update`) · `cmd/anyblockroundtrip` (35,369-object sweep, run 3 =
  99.86% pass; failure artifacts all depth-1 — not representative).
- Decision record: `docs/AgentApiV2Research.md`, Addendum A.
