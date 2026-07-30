# APIV2.md v0.2 — small-model (3–4B) review synthesis (2026-07-23)

Three opus lenses, all targeting the 3–4B on-device class (Gemma 3n E4B,
Qwen2.5-3B, Llama-3.2-3B, Phi-3-mini): **emission** (can it produce the
bytes), **comprehension** (can it read responses and choose right),
**end-to-end** (can it finish multi-step tasks). Grounded in the evidence
base's small-model facts (3–4B emit unparseable JSON 23–36%; escaping inside
JSON strings is a top failure; sub-7B tool-calling near-zero raw but
constrained decoding lifts a 7B 0→75%; verbatim reproduction collapses past
a few hundred tokens; capability collapses with schema surface).

## Shared verdict

The **skeleton is right** for a 3B — one vocabulary (C2), path-addressed
errors (C6), advisory header-only etag (C7 correctly removes the etag echo
burden), per-op discovery (§5), object-id compaction (C4). But three
*structural* problems make the launch surface unusable by a 3B as specced,
and they all point the same way: **the spec was written for a capable model
and then annotated for small ones, when it needs a first-class small-model
profile that is a strict subset with server-side conveniences.** The single
unifying recommendation is to define that profile (§ "The fix" below) rather
than trying to make the full 30-endpoint surface individually 3B-safe.

Every lens independently reached the same top-level conclusion from a
different direction: emission says "the content channel isn't
grammar-reachable," comprehension says "the examples teach the hardest
form," end-to-end says "the phase order ships the corrupting paths and defers
the safe ones." Same target, three sides.

---

## Structural findings (the load-bearing three)

### S1 — Phase ordering is inverted for small models [end-to-end ×3, comprehension] — MAJOR
The bounded-scope ops a 3B needs most are deferred/gated; the
corruption-prone whole-unit rewrites ship at launch.

| A 3B needs (deferred/gated/[build]) | Launch ships instead (whole-unit → verbatim collapse) |
|---|---|
| `replaceText` — scoped str_replace (B1) | whole-block `replaceBlock` (resend entire text run + marks) |
| `setCell` — flat cell write (deferred) | whole-`table` `replaceBlock` (re-emit every cell) |
| filter **string** (build item) | recursive `filters` array — which C13 itself says a 3B can't generate |

The research's own reconciliation (§3.8): "let models rewrite, but only at
the smallest sufficient scope — a whole-document rewrite bounds neither
emission burden nor blast radius." The launch set hands the 3B the unbounded
paths and withholds the bounded ones. Concretely, the two commonest edits —
change-one-word and fill-one-cell — route through whole-unit reproduction,
the documented 3B collapse mode (zero perfect transcriptions past ~300
tokens); and the commonest query (a filtered set) has *only* the
non-constrained-decodable array form at launch.

**Fix:** treat `replaceText`, `setCell`, and a constrainable filter string as
small-model **launch blockers**, not benchmark-gated additions. B1 can still
decide whether `replaceText` is worth it *for large models*; for the 3B path
its own evidence base (str_replace 28→51% pass@1) already settles it.

### S2 — Constrained decoding stops at the envelope; the content channel is where 3Bs fail and it isn't grammar-reachable [emission ×2] — MAJOR
C13 makes the op/create *schemas* strict-mode-compatible, but on-device
runners (llama.cpp/Ollama/MLX) constrain via GBNF/XGrammar over the JSON
body — and a grammar can force `"text":"<string>"` yet **cannot** enforce
valid inline markup inside it (balanced `**`, well-formed
`<mention>…</mention>`, minimal `\"`/`\n` escaping). The inline grammar
(SPEC §8) is a context-dependent delimiter stack, never provided in
grammar-emittable form. Same for the filter *string* (a mini-DSL inside a
JSON value, grammar "informal" and a build item). So the exact surfaces
where 3Bs fail — markup-in-`text`, the filter — are the ones constrained
decoding cannot rescue. Two more emission gaps compound it: the escape-free
mention form `[name](anytype://object?objectId=…)` (zero `\"`, normalizes to
the identical Object mark) exists but is never steered to; and §5 serves only
`flavor=json-schema` with no grammar/GBNF artifact and no stated
JSON-Schema→grammar contract, and if the served *generation* schema mirrors
SPEC §12's `if/then` validation dispatch it won't convert to a grammar at all
(needs `oneOf`+`const`).

**Fix:** (a) a **plain-text profile** where small-model `text` carries no
inline markup — block types only, mentions via the paren link-form,
marks unavailable — so a trivial string grammar suffices; (b) publish the
filter grammar as an emittable CFG composed into the `filter` value; (c) add
a served grammar artifact (or state the conversion seam), and require served
*generation* schemas to use `oneOf`+`const` discriminators distinct from the
`if/then` validation schema.

### S3 — Full block-id echo on edit is the unfixed BAML hole [all three lenses] — MAJOR
C4 fixed object ids (compact + legend) and C7 removed the etag echo — but
**block ids stay full (24-hex) on every body-bearing read**, and every
block-addressed op must echo one. That is the BAML long-id-mutation failure
(48.5 errors vs ~6 with short handles), incurred N times per multi-op PATCH.
The 5-char label mitigation lives *only* in `outline`, which carries no
`text` — so to edit a paragraph the 3B must do a full read (gets text + long
id), never an outline (gets short label, no text). And "write endpoints MAY
resolve by unique-suffix" is (a) optional and (b) useless for emission — a 3B
shown a 24-char id copies the 24-char id; it will not compute a minimal
suffix.

**Fix:** add a body-bearing read shape that returns `text` **and** 5-char
block labels together (extend `?ids=compact` to relabel block ids on content
reads); make write-side unique-suffix resolution a **MUST** for that shape.
Then the 3B composes a content-aware edit echoing a 5-char token. Resolves
the `outline→PATCH`-hinges-on-a-MAY inconsistency (end-to-end #6) too.

---

## High-leverage cheap fixes (examples teach the hardest form) [comprehension] — MAJOR/MINOR

The worked examples seed the failures the spec then recovers from:

- **E1** Search example populates **both** `filter` and `filters` → the 3B
  copies the shape and hits `ambiguous_input` on turn 1. Show only `filter`
  as primary; move `filters` to a separate "structured alternative — do not
  combine" example. (comprehension MAJOR)
- **E2** PATCH §3a leads with a **6-op composite blob** → high-surface first
  impression, contradicting §5's own "per-op fetch is primary." Lead with a
  single-op minimal example; demote the composite. (emission + comprehension)
- **E3** `replaceBlock`-wipes-text is prose-only → a 3B toggling a checkbox
  via `replaceBlock` (omitting `text`) silently empties the block. Make
  `updateBlock` the *only* op shown for field-level changes; mark `text`
  required in the `replaceBlock` per-op schema or emit a `warnings` entry.
  (comprehension MAJOR)
- **E4** "done" ambiguity → the composite shows `setProperties{status:["Done"]}`
  and `updateBlock{checked:true}` side by side while `done` is also a
  well-known checkbox property; 3 correct decisions with no steering. Ship
  intent→op recipes in SKILL.md ("complete a task" → the exact well-known
  key). (comprehension MAJOR)
- **E5** Atomic whole-PATCH rejection forces re-emission of the good ops on
  any single-op error (the corruption-on-retry trap), yet the batch is the
  default shape. Document single-op PATCH as the small-tier default; consider
  a per-op status array (partial apply) for the small profile. (comprehension)

## Accuracy lever unwired + silent create-missing [comprehension + end-to-end] — MAJOR

- **A1** `types/{type}/schema?flavor=table` is labeled the highest-leverage
  accuracy lever (Jackal 35→85%) but is a `[build]` item and is **not a step
  in the write flow** — nothing tells the 3B to fetch live keys/option names
  before create/setProperties, so it hallucinates them at emission. Make the
  discovery GET step 0 of the create/setProperties C12 examples; ship the
  endpoint before the write path depends on it.
- **A2** create-missing (SPEC §2 type, §3 options) turns a hallucinated
  `type:"task"` or `status:["In Progress"]` (real: "In progress") into a
  **silent new entity** — the repair loop never fires, worst case for the
  model that hallucinates. Validate `type`/option names by default with
  did-you-mean errors; gate create-missing behind explicit
  `?create_missing=true`.

## Relative-indent origin flip is a silent trap [emission + earlier format R3] — MAJOR

Reads give **absolute** indent; write payloads want **relative** indent whose
zero moves (`after`/`before`/`replaceSubtree`: 0 = anchor level; `inside`:
0 = anchor+1). "Add a child under b3" is expressible two ways with different
indents; a 3B pattern-matching the absolute indent it read silently gets a
sibling — parses clean, wrong tree (Addendum A.2's exact silent-failure
class, amplified). **Fix:** on the small-model path forbid `indent>0` in
`insertBlocks`/`moveBlock` payloads (single-level inserts; push multi-level to
`replaceSubtree`), so the keyword alone places the run and no origin is
computed. Or accept absolute payload indent and reconcile server-side.

## Harness/CLI must own the unrecoverable machinery [all lenses] — MINOR/NOTE

Things a raw 3B cannot author or reason about, which the Phase-5 CLI/harness
must own (and which are therefore unprotected until it ships):
- **If-Match** on a sync-advancing etag → `etag_mismatch` on sync noise the
  3B can't reason about. Harness omits If-Match for the small tier.
- **Idempotency-Key** dedupes only across *identical* bodies; a 3B's
  regenerated retry body → `idempotency_conflict`. Harness auto-derives a
  stable key from body+target; caches the exact original body across retries.
- **"Assigned to me"** — assignee is a member object id with no
  self-identification; add `GET /members/me` + a server-resolved `@me`
  sentinel.
- **"Due Friday"** — date values require absolute RFC 3339; date-preset
  functions exist only in filters. Accept `daysFromNow(n)`/weekday presets on
  date property values, server-resolved.
- **Markdown-body shortcut** — the one genuinely low-surface create path is
  one under-specified line with no example and no loss contract. First-class
  it: its own C12 schema+example, an explicit `warnings` loss contract, a
  clear discriminator (reject `blocks` when `markdown` present).
- **`setProperties` value shape** is format-dependent (single select still
  needs a one-element array); accept scalar-for-single and coerce
  server-side, reported in `warnings`.
- Filter **keyword** did-you-mean (not just property keys) — a 3B tries
  `IS NULL`/`LIKE`/`IN`; enumerate the valid keyword at the error position.

---

## The fix: a task-tool wrapper (the small-model contract)

Every finding above is an instance of one gap: the spec has no defined
small-model contract, so the full surface's sharp edges reach the 3B. The
delivery vehicle is **not a "mode" threaded through the 30-endpoint REST
surface** — it is a curated, task-shaped **tool-calling wrapper** over the
full API, and it is the same artifact as the Phase-5 CLI (the CLI verbs *are*
the tools). This dodges the research's "ugly MCP over legacy" critique
because the underlying API is already agent-native and the wrapper is
task-shaped, which is what the evidence actively recommends (Anthropic
writing-tools-for-agents: "build `schedule_event`, not `list_users` +
`list_events` + `create_event`"; Linear's 23 task tools beating raw GraphQL;
Notion's markdown-for-agents layer).

**Why a wrapper beats a REST profile: it can pick better channels than the
REST body allows** — this is what makes it more than packaging, and it is
what actually closes S1–S3:

- **Author as markdown, not AnyBlock JSON.** `add_blocks(object, after,
  markdown)` takes a markdown string; the *server* parses it to flat blocks.
  Dissolves **S2** (no inline-markup-in-JSON escaping) and the
  relative-indent arithmetic (markdown indentation → server computes indent).
- **Edit by anchor; the server does the edit deterministically.**
  `edit_text(object, block, find, replace)` — the model supplies a short
  anchor, the wrapper does the string replace in code and applies it. The
  fast-apply pattern (§3.1): model says *what* changes, server reproduces
  verbatim. Gives the bounded `replaceText` interface safely, closing **S1**'s
  change-one-word collapse without the model regenerating text.
- **Reference by enumerated handle.** `find` returns `1,2,3`; reads expose
  short block labels; the wrapper maps handles → CIDs. The §3.6 handle
  pattern — closes **S3** (the model never sees or emits a 24-hex id).
- **Server conveniences in the handler:** `@me`, relative dates,
  scalar→array coercion, validate-not-create-missing, discovery folded into
  the tool description (A1/A2).

**Tool set (~9, under the >15 cliff; flat, grammar-constrainable args):**
`find` · `read(full|outline)` · `describe(type)` · `create` ·
`set_properties` · `add_blocks(markdown)` · `edit_text(find/replace)` ·
`set_cell` · `move_block` · `delete_block`.

**Two things the wrapper does NOT let us skip:**

1. **The bounded server primitives must still exist.** `edit_text`/`set_cell`
   are safe only if they map to real server-side scoped edits; otherwise the
   wrapper does GET+regenerate+PUT and reintroduces corruption. So **S1's**
   "pull `replaceText`/`setCell` into the launch op set" still holds — the
   wrapper *consumes* them.
2. **Constrained decoding is still required, just made tractable.** On-device
   function-calling (Ollama/llama.cpp GBNF) needs the *tool* schemas
   emittable as grammars; C13 now applies to small flat tool args instead of
   the recursive block tree — easy instead of impossible (**S2**).

**Layering (avoids a third surface):** primitives live server-side (bounded
ops, id/handle resolution, `@me`); the wrapper is a thin **task-tool
manifest** mapping ~9 tools to those primitives. That manifest is the CLI
verb-set, exposed two ways — CLI verbs for coding-agent harnesses (the
CLI-over-MCP finding), a function-calling/MCP manifest for on-device 3Bs.
Full REST stays for large models and programmatic use. **One shared AnyBlock
format + validation + error contract; two surfaces, not three.**

**Discipline that keeps it honest:** the wrapper must stay task-shaped and be
defined in the same format/error vocabulary. The moment it becomes a 1:1
re-export of the endpoints, it is the anti-pattern the research warned about
and there are now two divergent APIs.

## Disposition

None of this invalidates the architecture — the op set, flat format, phase
structure, and benchmark program stand. It (a) reweights *what ships first*
(S1: `replaceText`/`setCell`/filter-string are launch primitives, not
gated), (b) closes the two content-channel gaps via wrapper channels (S2
markdown-in, S3 handles), (c) fixes examples that teach failure (E1–E5), and
(d) delivers all of it as the **task-tool wrapper = CLI = on-device tool
manifest**, which is also exactly what benchmark B4 should tune. Folded into
APIV2 v0.3 as a new "§7 Small-model surface: the task-tool wrapper" plus the
S1 launch-set reweighting.
