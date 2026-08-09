# API v2 token economics — a measured review (GO-7383)

Status: review v1.0 · 2026-08-08 · companion to `APIV2.md` (C3/C4/C5, §7,
§8.20–8.22) and `pkg/lib/anyblockjson/ADDRESSING.md` (§7.5a, §7.6).

Method: everything here is **measured against the running app** (a real
~16-space account) unless marked *estimate* or *sim:* (simulated offline from
the served JSON — exact for the field edits applied, e.g. deleting `id`
keys). Tokenizers: **o200k_base** via tiktoken (the GPT-4o/o-/5-family
count), and the **served `gemma4:e2b` tokenizer** measured as
`usage.prompt_tokens` deltas on the live Ollama endpoint — real counts, not
estimates. Gemma counts run 5–25 % higher than o200k on this JSON; every
*ratio* below holds under both, so tables quote o200k and call out gemma
where it matters. bonsai-27b's tokenizer was not measured (see §7's
availability note). Model evals ran real tool-calling against
`gemma4:e2b`, `gemma4:e4b` and `gpt-5-mini`/`gpt-5` (OpenAI). Measurement
scripts lived in the session scratchpad and are not committed; every number
is reproducible from the live account + this description.

Corpus — real documents from the live account (plus two documents created
through the API for the evals; the account's seeded docs carry
human-readable block ids, API-minted ones carry 24-hex — flagged where it
matters):

| tag | doc | blocks | refs | default read (o200k) | (gemma) |
|---|---|---|---|---|---|
| XS-props | Sales crmActivity (properties only) | 0 | 5 | 607 | 701 |
| S-12blk | Company Wiki policy page | 12 | 7 | 1 238 | 1 410 |
| M-24blk | Project Brief — Q3 Website Relaunch | 24 | 14 | 2 417 | 2 722 |
| L-66blk | "Properties" (Get Started) | 66 | 9 | 4 441 | 5 542 |
| R-20refs | Q3 2026 planning (mention-heavy) | 22 | 20 | 3 814 | 4 363 |
| K-recipe | Personal recipe, 4 UI-created BSON-key props | 31 | 1 | 1 989 | 2 391 |

---

## 1. Three costs nobody had priced

These dominate everything the knobs can do, and none of them is a knob.

### 1.1 Default reads are pretty-printed inside — a 16–26 % tax on every GET (C3 violation)

`anyblockjson.Marshal` emits the format's canonical byte form — **two-space
indented** (`marshalCanonical`, `pkg/lib/anyblockjson/json.go:156`). The v2
read path (`core/api/v2/service/object.go`) splits that document into
`map[string]json.RawMessage` and re-emits a compact **envelope** whose
`properties`/`blocks`/`refs` values keep their indented bytes verbatim. So
every default object read is compact at the top level and pretty-printed
underneath, while C3 promises "compact JSON always (free 38–46 %)".

| doc | served (o200k) | compact re-encode | tax |
|---|---|---|---|
| XS-props | 607 | 509 | 16.1 % |
| S-12blk | 1 238 | 981 | 20.8 % |
| M-24blk | 2 417 | 1 903 | 21.3 % |
| L-66blk | 4 441 | 3 271 | 26.3 % |
| R-20refs | 3 814 | 2 902 | 23.9 % |

(gemma agrees: M-24blk 2 722 → 1 984, −27 %.) The outline and markdown
shapes are nearly unaffected (they re-encode). **Fix: compact the embedded
values at the envelope (or add a compact option to `Marshal`)** — zero
semantic change, the single largest saving in this review, and it makes
every percentage elsewhere in this document better than quoted (they are
measured against the *served* form).

### 1.2 The refs legend loses tokens on reads — C4's default is inverted by real documents

C4 compacts object refs by default via the `refs` legend (5-char label
inline + `"label": "<59-char id>"` legend line). That trade only wins when
a ref is used ≥ 2×. Measured usage multiplicity on the corpus: **85–90 %
of refs are used exactly once** (M-24blk: 12 of 14; R-20refs: 18 of 20;
S-12blk: 5 of 7). Result — `?ids=full` is *cheaper* than the compact
default on **every** document measured:

| doc | compact default | `ids=full` | legend overhead |
|---|---|---|---|
| XS-props | 509 | 456 | +10.4 % |
| S-12blk | 981 | 947 | +3.5 % |
| M-24blk | 1 903 | 1 811 | +4.8 % |
| L-66blk | 3 271 | 3 194 | +2.4 % |
| R-20refs | 2 902 | 2 763 | +4.8 % |

(compact-encoded pairs, so this isolates the legend from §1.1.) The legend
also adds an indirection: a model that wants to write an object id back
(e.g. `setProperties.remove` of one linked object) saw `"ai52e"` inline and
must dig the legend for the full id — a correctness hazard on top of a
token loss. C4's research rationale (−89 % id *transcription* errors) is
about models *generating* references — which happens in agent-authored
create/PUT documents and in ops, where **block** ids are the vocabulary
(and those are full on default reads anyway). On reads the legend earns
nothing. **Recommendation: serve full object ids inline by default; keep
legend resolution on *input* documents unchanged (SPEC §9a is total), and
keep the legend in the export/backup shape only (§6).**

### 1.3 `format=md` mention links cost ~60 tokens each

The markdown exporter renders every mention as
`[Name](anytype://object?objectId=<59 chars>&spaceId=<66 chars>)`.
Measured: md is 27–33 % of the default read on link-light docs but **83–84 %
on mention-heavy ones** (M-24blk: 2 036 of 2 417; R-20refs: 3 165 of
3 814) — the "cheap text mode" is nearly as expensive as the full document
exactly where documents are link-rich. Fix candidates, cheapest first:
drop the `spaceId` query param (same-space links; halves the cost), or
render `[Name](anytype:<5-char label>)` with a one-line legend appendix
(lossless, ~50 tok saved per mention). md is read-only (C11), so this is
purely a rendering decision.

---

## 2. Knob inventory — what exists today, its default, and whether the default is right

Verified against handlers/service (`core/api/v2/handler/*.go`,
`v2/service/object.go`, `v2/router.go`) and exercised live. **`pins` do
not exist anywhere in v2** — `?pins=` and the pin tables are ADDRESSING
§7.1/§7.6 design, explicitly unshipped (§7.6 steps 1–2); nothing below is
a pin. `OmitIds` exists only as an unexposed export option in the format
package — no API surface reaches it.

| knob | surface | default | measured cost/saving | default right? |
|---|---|---|---|---|
| `?include=properties,blocks` | object GET | both | props-only 7–53 % of default; blocks-only 57–98 % (barely saves) | both-by-default is right for the edit read; `include=blocks` never earns its existence |
| `?outline=true` | object GET | off | 13–29 % of default; `+include=properties` 20–60 % on block-bearing docs (89 % on the 0-block XS doc, where properties are the document) | right as opt-in; the shape itself is the API's best token lever |
| `?block={id\|suffix}` | object GET | — | one subtree | right; keep as the orthogonal target param |
| `?ids=compact\|full` | object GET | compact | **compact is 0.5–10.4 % more expensive** (§1.2) | **wrong — flip to full** |
| `?format=anyblock\|md` | object GET | anyblock | md 11–84 % of default (§1.3) | right as opt-in; fix the mention links |
| `?fields=` | lists, sets/collections, search body | rows = `{id,name,type}` (~50 tok/row, ~30 of them the id) | +2 fields ≈ +28 tok/row | right (C5); nothing to change |
| `?offset=/&limit=` | every list | 25 / max 1000 (`v2/router.go:23`) | 15 pages ≈ 758 tok | right |
| `?view=` | set/collection reads | object's default view | — | right |
| `?reactions=counts\|full` | chat messages | counts | — | right |
| `?prefix=` | options list | — | — | right |
| `Options.CompactObjectRefs` | format pkg; ON for default reads | — | §1.2 | flip with `?ids` |
| `Options.CompactBlockLabels` | format pkg; ON only in outline | — | part of outline's 75 % saving | right (C4/T7) |
| `Options.OmitIds` | format pkg; **never exposed** | — | sim: −16…−36 % of compact read | right to keep unexposed as a raw knob; see §4 |
| `?pins=` | **does not exist** | (planned "all") | sim: +22 tok per custom key per read | planned default is wrong — see §6 |

Defaults matter more than options — §7's eval shows the smallest model
never touches a parameter. The three defaults that are wrong (§1.1
encoding, §1.2 ids, §6 planned pins) are exactly the ones every
untouched-parameters caller pays.

---

## 3. Do we always need ids? Measured id budgets

Per-id prices (o200k, measured): a 24-hex **block id** in a default read
costs **18.2 tok** including its `"id":` scaffolding (436 tok / 24 blocks
on the API-minted eval doc); a 59-char **object id** ≈ 30 tok; a 66-char
space-suffixed id ≈ 33. Block ids in aggregate are **8–36 % of a compact
default read** (sim:omitIds deltas: S −16 %, M −17 %, L −31 %, 48-block
K-page −36 %). The account's seeded docs carry short readable block ids
(13.3 tok/block) — real API-minted documents pay the full 18.2.

**For a model that wants to patch ONE object** (the user quoted the
sentence to fix), the measured paths on M-24blk:

| path | model-visible tokens | works? |
|---|---|---|
| A. GET default → PATCH `replaceText` by id | 2 417 + 33 (op) | yes — today's canonical flow |
| B. GET `?include=blocks` → PATCH | 2 252 + 33 | yes, marginal saving |
| C. GET `?outline=true` → PATCH | 582 + 33 | **no** — outline has no body text; you cannot find the word |
| D. GET `format=md` → PATCH | 2 036 + … | **no** — no block ids to address (today) |
| E. wrapper `edit_text`, `block` omitted (shipped) | **≈ 45** (call + result; server does one internal GET) | yes — verified live: unique find applies; ambiguous find refuses listing candidates (54-tok refusal) |
| F. raw-API locator op (proposed, §5) | ≈ 50–60, zero reads | — |

So: **the raw API needs the whole id-bearing document (~2.4 k tokens) to
patch one word; the wrapper's locator does it for ~45.** The raw API has
no equivalent of E — `replaceText.id` is required
(`v2/service/stateops.go` `applyReplaceText` → `resolveRef`); that gap is
§5. Structural edits (move/delete) genuinely want ids — and outline serves
them at 13–29 % of the default read; that is the id budget an edit
actually needs.

---

## 4. Id-free mode: trap or mode?

What works with **no block ids at all** today: `setProperties`,
`addItems`/`removeItems` (object ids from search rows, not reads),
`insertBlocks` in append form (no anchors), the whole view family on
single-dataview objects (`updateView` block/view optional, columns keyed
by property key), create, search. What cannot: `replaceText`,
`updateBlock`, `deleteBlock`, `moveBlock`, `replaceSubtree`, `setCell`,
anchored `insertBlocks` — every block-addressed op.

Measured saving of a hypothetical id-free full read (sim:omitIds,
compact): 50–66 % of the served default (M 1 583, L 2 247, R 2 438) —
real money; the 48-block K-page drops 36 %. But **without a write-back
story it is a trap**: the model
reads a document it cannot then edit at block level, and the failure
arrives one turn later as a missing-id dead end (§8.21 measured exactly
this pattern: a required-but-unknowable id made small models route around
the edit tool entirely).

**With locators (§5) the verdict flips**: read without ids, patch by
content. The pairing is coherent for *text* edits (the majority editing
class); structural edits keep outline (which is also id-bearing and
cheap). Even then, a separate `omitIds` JSON read mode is not worth a
knob: `format=md` (mention links fixed, §1.3) is the natural id-free
reading surface at 27–33 %, and outline covers structure at 13–29 %.
**Recommendation: do not expose raw `OmitIds`; make md the id-free mode
and make it writable via locators.**

---

## 5. Locators instead of ids in PATCH ops

Proposal under review: ops accept a *locator* that must resolve to exactly
one block, instead of requiring an id.

**Prior evidence (§8.21, live benchmark of the shipped MCP small tier):**
with `edit_text.block` required, gemma4:e4b and e2b both picked `read`
instead of `edit_text` for an explicit "change the word" task — the
rational move when the required id is unknowable on turn one. Making
`block` optional (snippet locates the block, one-match-or-refuse) took
tool selection from 7/8 and 6/8 to 8/8. The primitive already exists and
already paid off; the question is generalising it.

### 5.1 Which ops can take one

| op | locator form | verdict |
|---|---|---|
| `replaceText` | `find` doubles as the locator — `id` becomes optional; `under`/`nth` narrow | **adopt** — the shipped wrapper semantics, moved down |
| `updateBlock` | optional `match` (exact substring of the block's text) + `under`/`nth`, alternative to `id` | **adopt** — the checkbox-toggle case ("check 'Draft timeline' under Planning") |
| `deleteBlock` | same | adopt — destructive, so the one-match rule is load-bearing |
| `moveBlock` | `match` for the subject; `after`/`before`/`inside` anchors accept the same forms | adopt (anchors too — "after the heading 'Risks'") |
| `replaceSubtree` | same as updateBlock | adopt, lower priority |
| anchored `insertBlocks` | anchor locators only (payload has no ids) | adopt with moveBlock |
| `setCell` | col by header text, row by index/first cell — a *different* vocabulary | **defer** — rare shape, its own design; labels from full reads already work |
| view ops | already locator-ish (optional-when-unique, suffix match); accepting view *names* is the remaining gap | note, not part of this change |
| `setProperties`, items ops | address no blocks | n/a |

### 5.2 Syntax: a small closed vocabulary, not a query language

Ship exactly three fields: **`match`** (exact substring that must identify
one block; for `replaceText`, `find` *is* `match`), **`under`** (heading
text — restrict to that section's indent run; trivial on the flat format),
**`nth`** (1-based, document order, within scope). All flat strings/int:
C13-strict, GBNF-trivial, nothing to parse.

Against a CSS/XPath/jq-style selector, argued from this repo's own
history: the one general parser v2 shipped (the compact filter string)
required a recursion bound (`filterstring.go:36 maxGroupDepth`), fuzzing
and grammar-pinning to close a reachable process-fatal DoS. A locator
language re-inherits that whole class for expressiveness no measured
consumer used: in §5.4 the models never needed more than `under` + the
snippet, and gpt-5-mini composed `under` correctly *unprompted*. The
failure mode of an expressive locator is a silent wrong match — the exact
thing the one-match rule exists to kill. Small closed vocabulary wins on
the evidence; revisit only if a benchmark shows tasks failing for lack of
expressiveness.

### 5.3 Resolution semantics (the load-bearing part)

Reuse the shipped rule, once, server-side — never a second rule:

- **Exactly one or refuse.** Zero matches → 404-class C6 steering to the
  outline read (the candidate list *is* the outline), and — from
  `applyReplaceText` — "copy the find text exactly, including inline
  markup". Multiple blocks → `ambiguous_input` listing ≤ 8 candidates as
  block ids + ~30 chars of context (the wrapper's `locateBlock` refusal,
  verified live: 54 tokens, repaired first-try in §8.21's measurements).
  Multiple occurrences within the one block → the existing more-context
  refusal (`nth` is the escape). Never a guess.
- **Mid-batch:** resolution runs per-op against the applier's live
  document view — the same view id-suffix resolution uses
  (`stateops.go` maintains it across ops; `replaceText` updates it in
  place, M7) — so op *i* sees op *i−1*'s edits. Deterministic, and the
  natural reading of a batch.
- **Dry-run vs apply:** both resolve at apply time under the object lock;
  a C9 dry run's resolution is advisory exactly as every C9 verdict is. A
  document change between dry-run and apply either creates a second match
  (→ refusal, the safe direction) or removes the match (→ refusal). The
  residual case — the unique match *moved* — is strictly safer than the
  id equivalent: **a locator is inherently a content precondition** (the
  op fires only where the text it names exists), which an id never is.
  If-Match remains the strict guard (C7), unchanged.

### 5.4 Measured: can the models actually write locators?

Six edit tasks against two live documents (the API-minted 24-block brief
with 24-hex ids; a purpose-built two-section doc where "Budget: TBD" and
"Draft timeline" each appear twice). Arm **ID**: compact read *with* ids
in context, tool requires `block_id` (resolution = shipped
`matchBlockRef`: exact, else unique suffix; one retry on the server
error). Arm **LOC**: id-free read in context, tool takes
`find`/`under`/`occurrence` (resolution = §5.3; ambiguity refusal lists
candidates; one retry). Success = the edit lands on the intended block.

| model | ID arm | LOC arm | notes |
|---|---|---|---|
| gpt-5-mini | 6/6 (6 first-try) | 6/6 (5 first-try) | copied 24-hex ids perfectly; used `under:"Execution"`/`"Planning"` unprompted, first try; on the one genuinely two-match task the ambiguity refusal listed candidates and the retry repaired — §5.3 working as specified |
| gpt-5 | 6/6 (6 first-try) | 6/6 (6 first-try) | |
| gemma4:e2b / e4b | *blocked* | *blocked* | see below |
| bonsai-27b-q1_0 | *blocked* | *blocked* | see below |

**Environment fault, stated rather than papered over:** midway through
this session the remote Ollama box's Metal compiler wedged while loading
bonsai-27b (`llama-server … MTLCompilerService … Reentrancy avoided`);
from that point every local model answered 500 and did not recover within
the session (a server restart is required), so the direct gemma ID-vs-LOC
arms could not be completed. The §7 knob eval ran *before* the fault and
is unaffected. Read the frontier rows accordingly: **a 6/6 ID arm at
gpt-5 scale is precisely NOT the case locators exist for** — frontier
models copy 24-hex ids fine; the claim under test is about the small
tier, and **the direct ID-vs-LOC comparison at the small tier is
untested**. The standing small-model evidence is §8.21's earlier live
benchmark — the same comparison in tool-selection form, against the real
MCP server: with a required block id, gemma4:e4b/e2b scored 7/8 and 6/8
and routed around the edit tool; with the snippet locator both reached
8/8. That supports "a required id makes small models avoid the edit
path"; it does not yet measure small-model *locator authoring* accuracy
head-to-head against id copying.

*To close the gap once the box is restarted* (whoever reruns needs no
re-derivation): two arms × 6 tasks × {`gemma4:e2b`, `gemma4:e4b`,
`bonsai-27b-q1_0`}, temperature 0, one tool call + one error-fed retry.
Arm ID: context = compact default read *with* ids, tool
`replace_text(object_id, block_id, find, replace)`, resolution = shipped
`matchBlockRef` (exact-else-unique-suffix). Arm LOC: context = the same
read with block ids stripped, tool
`replace_text(object_id, find, replace, under?, occurrence?)`,
resolution = §5.3 (one-match-or-refuse; ambiguity refusal lists ≤ 8
candidate blocks with ~30-char context). Documents: the two eval objects
left in the test account's Project Tracker space — "Relaunch brief
(locator eval copy)" (24 blocks, API-minted 24-hex ids; three
unique-match tasks: 61 %→58 %, ~140 posts→~150, 4.2s→3.9s — the last
matches two blocks, score either) and "Locator eval doc" (two sections
where "Budget: TBD" and "Draft timeline" repeat; tasks: Execution budget
→ $40k, Planning budget → $12k, "scope creep"→"schedule slip"). Success
= the edit lands on the intended block; report first-try and
after-retry separately.

Two observations beyond the scores. Context-cost asymmetry, same
documents: the ID arm's read is 1 551 tok, the LOC arm's 1 115 (−28 %);
the id op is 33 tok vs the locator op's 24; and the LOC arm composes with
§3's flow F — when the user's request already contains the anchor text,
the read can be skipped entirely. Second: in the LOC arm, where
`object_id` was incidental, even the frontier models filled it with
garbage (`"current"`, `"1"`, `"page"`, the document's *name*) — id-echo
discipline is weak the moment an id stops being the model's focus, which
is the §8.21 finding at frontier scale and an argument for locators plus
enumerated handles everywhere ids are secondary.

**Verdict: adopt, in the reduced form of §5.2** — `find`-as-locator with
optional `id` on `replaceText`, plus `match`/`under`/`nth` on
`updateBlock`/`deleteBlock`/`moveBlock`/`replaceSubtree` and the
`insertBlocks`/`moveBlock` anchors; one-match-or-refuse with the shipped
candidate-listing refusal; setCell deferred; no selector language. The
frontier arms show locators cost nothing in correctness at the ceiling;
§8.21 shows they are the difference between routing around the edit path
and using it at the small end; and they are what turns §4's id-free reads
from a trap into the cheapest correct loop.

### 5.5 Where it lives: the API op set, not the wrapper

- **One implementation, every client.** CLI, MCP (both tiers), raw HTTP,
  third-party SDKs — the wrapper-only version serves one of four surfaces.
- **The wrapper's version is a read-then-patch TOCTOU.** `locateBlock`
  GETs the document, resolves, then PATCHes by id — the document can move
  between the two. In-API resolution runs under the object lock; the race
  disappears. §7.3 item 1's own principle (bounded server-side primitives,
  never GET+compute+PUT at a layer above) argues this side.
- **The §8.21-fix-3 precedent does not apply.** Case folding went
  wrapper-side because folding is *forgiveness* — it changes what a
  spelling resolves to, and REST clients depend on exact-match strictness.
  A locator is exact-match addressing with a hard ambiguity refusal —
  deterministic, C2-clean, additive (an optional alternative to `id`, not
  a change to `id`).
- The wrapper keeps `edit_text` unchanged and drops its double-read
  (`locateBlock` becomes "omit the op's id").

Cost *(estimate)*: `resolveLocator` on the applier's existing doc view
(linear scan + indent-run scope) is tens of lines plus tests; ~15 tok per
op schema in discovery; +3 SKILL lines (§8). No new parser, no new
grammar artifact.

---

## 6. Can the tuning collapse into ONE knob?

### 6.1 The proposed ordinal (`fullIds > allPins > minPins > noPins > noIds`) — rejected as the one knob

It linearizes only the *id-spelling* axis, and that axis is third-order:

- The measured money is in the **shape** axis — outline (13–29 %), props
  (7–53 %), md (11–84 %) — which the ordinal does not touch at all.
- Its levels mix **independent** choices the shipped surface already
  treats independently: block-id spelling vs object-ref spelling (outline
  compacts the former and *fulls* the latter — the C4/T7 exception exists
  precisely because they must move separately), and pins (a
  rename-protection legend, ~6 % — measured +87 tok on a 1.5 k read for a
  4-custom-key doc) vs ids (an addressing vocabulary, −5…−35 %).
- `allPins` vs `minPins` is a distinction below the noise floor of one
  paragraph of content. And `noIds` is not a *cheaper level of the same
  thing* — it changes what the document can do (write-back), i.e. it is a
  different shape, not a smaller spelling.

### 6.2 What the evidence supports: five named profiles on one parameter

`?mode=outline | text | props | edit | full` on the object GET, default
`edit`. (`view` as a name is taken — set/collection reads use `?view=`
for stored dataview views; the wrapper's read already calls this `mode`.)

| mode | contents | encoding | M-24blk cost (today 2 417) |
|---|---|---|---|
| `outline` | skeleton `{indent,id,type}` + heading text **+ properties** + etag | block labels, object refs full (T7 kept), compact | 773 |
| `text` | markdown envelope | short mention links (§1.3 fix); no ids | ~1 150 *(estimate after fix — 14 mentions × ~55 saved; 2 036 today)* |
| `props` | properties + etag, no blocks | full object ids | 774 |
| `edit` (default) | properties + blocks | **full block ids, full object ids inline, no legend, compact (§1.1+§1.2)** | 1 811 |
| `full` | canonical export: refs legend + full block ids + **pins when they ship** | the backup/PUT-round-trip shape = `Marshal` default | ~1 900 + pins |

- `include=` and `ids=` retire (props/edit/full cover every measured use;
  `include=blocks` saved 2–7 % on content-bearing docs and §7 shows
  nobody selects it);
  `outline`/`format` fold in; `?block=` stays orthogonal (a target, not a
  shape). v2 is pre-GA — cut clean rather than alias (ADDRESSING §7.4's
  own argument: possible precisely because nothing has shipped).
- `outline` gains properties relative to today (+4–34 pp of the default
  read, median ~10 — the top end is the property-rich wiki page): the
  dominant outline use is orient-before-acting, and properties are where
  status/assignee live — one call instead of two. The bare skeleton
  disappears as a distinct shape; its saving over outline+props was
  240–460 tok on the corpus, less than the second round trip it invites.
- **Pins land in `full` only.** ADDRESSING §7.1 plans "API v2 default
  read: all pins"; measured, that is +22 tok per custom key on every
  read to protect a GET→PUT round trip that PATCH-first agents almost
  never run — and PATCH inputs never consume pins (key resolution walks
  the §7.5a-5 chain regardless). `full` is the document-shaped read; it
  carries the protection. This review recommends amending §7.1's emission
  table accordingly; `?pins=min` dies with the ordinal (§6.1).
- The BSON→slug respelling sweep (§8.22 deferred work) is worth shipping
  for reads: measured on the live "Personal" space recipe,
  `include=properties` drops 313 → 253 (o200k, −19 %; gemma 353 → 279,
  −21 %), the full doc 1 563 → 1 503, and — the larger effect — the
  *comprehension* call disappears: today the model must fetch
  `GET /properties` (722 tok on this space) to learn that
  `6a764b3f61fab21cd4b9e0a7` means `prep_time`; a slug-keyed read needs
  no discovery call at all. Key spelling is a migration, not a knob — it
  belongs in no mode.

### 6.3 The deciding evidence

The profiles are not just tidier — §7 measures that models *use* them:
the same tasks that leave the current five parameters untouched at 2B
scale (2/8 optimal) are solved 7–8/8 through the one enum, at every model
size tested. A knob nobody turns saves nobody tokens; §7 is why this
section recommends profiles rather than better documentation for the
existing parameters.

---

## 7. Model comprehension — the empirical core

**Setup.** Eight realistic read tasks (overview, property check, edit
prep, render, export ids, subtree follow-up, text grep, restructure
prep), one tool call each, temperature 0 (gpt-5-family calls run at API
default — the endpoint rejects the parameter). Arm **A**: `read_object` with
today's five parameters, descriptions verbatim from the served OpenAPI
document. Arm **B**: `read_object` with the §6.2 `view`/`mode` enum + `block`.
Scored per task: *optimal* (cheapest sufficient shape), *ok* (correct but
overpays — in practice: sent a bare default read), *miss/illegal*
(another round trip or a 400). Scoring is symmetric-lenient: a spelled-out
default (`include="properties,blocks"`) counts as the default.

| model | arm A optimal | arm A ok (paid default) | arm A miss | arm B optimal | arm B miss |
|---|---|---|---|---|---|
| gemma4:e2b | **2/8** | 6 | 0 | **7/8** | 1 |
| gemma4:e4b | 5/8 | 3 | 0 | **8/8** | 0 |
| gpt-5-mini | 7/8 | 1 | 0 | **8/8** | 0 |
| gpt-5 | 8/8 | 0 | 0 | 7/8 (+1 ok) | 0 |
| bonsai-27b-q1_0 | — | — | — | — | — |

(bonsai-27b could not be measured: the remote Ollama box fails to load it
— an `MTLCompilerService` fault from llama-server that then took the box
down for the session (§5.4) — an environment fault, not a model verdict.
A second gpt-5-mini arm-A sample — gpt-5-family calls run without
`temperature`, so samples vary — produced `format=md` **with**
`include=properties,blocks`: one of the served 400 `ambiguous_input`
combinations. Even a frontier-mini can trip the param-legality matrix;
the enum has no illegal combinations to trip.)

**The null result, plainly: the smallest model ignores the current
parameters entirely.** gemma4:e2b sent the bare default read for 6 of 8
tasks — never `outline`, never `format=md`, never `include`, never `ids` —
exactly §8.21's route-around pattern, now reproduced on the read surface.
It is not that e2b *cannot* choose: given the enum it chose the right
profile 7/8 (its one miss: `view=text` before an edit — which locators,
§5, turn from a dead end into a working flow). e4b never discovered
`ids=full` in arm A (sent a default read for the export task) but was
perfect with the enum. Even gpt-5-mini improved. Nobody produced an
illegal combination in arm A — the failure mode is under-use, not misuse:
**parameters whose value requires understanding the format's internals
(`ids`, `include`) are dead weight; one enum whose values name tasks is
usable at every size.**

Token consequence (priced with the M-24blk measurements, subtree read
885): e2b's arm-A choices fetch **17 804** tokens across the eight tasks
vs **11 299** for its own arm-B choices — the multi-knob surface costs
the smallest model **~58 % more input**, and on the pure-read tasks
(overview, restructure) 4.2× per call. (Arm B's one miss — `text` before
an edit — would cost a second read today; with §5 locators it wouldn't.)

Caveats, stated: selection quality was judged against schema+description
only (no SKILL.md in context) — that is the real condition for MCP and
function-calling hosts, but CLI/HTTP agents that load SKILL.md get prose
steering this eval does not credit. Judges were authored with the tasks
(risk of construct bias); the arm-B enum is this review's own proposal,
so arm B had descriptions written to be chosen well — which is, in fact,
the point being demonstrated.

---

## 8. SKILL.md delta

`core/api/v2/SKILL.md` is at 262 of 300 lines; every recommendation above
that ships must displace text. Exact edits, keyed to today's lines — net
**−2 lines** if all of §1/§5/§6 land:

- **L71–77 ("Read cheaply" bullets 1–2)**: replace the
  outline/include/format bullets with the `mode=` table — 5 shapes, one
  line each ("`?mode=outline` structure+props at ~⅓ price · `text`
  readable md · `props` values only · `edit` (default) full ids ·
  `full` export with legend/pins"). −1 line.
- **L78–79 ("Object ids in read bodies are compacted via a refs legend by
  default (`?ids=full` opts out); block ids are always full")**: becomes
  "Ids are full on `edit` reads; `full` mode adds the refs legend for
  export/PUT round-trips." −1 line.
- **L74–75 ("editing needs no prior full read once you know the ids")**:
  append the locator sentence: "text edits need no read at all —
  `replaceText` with no `id` locates by `find` (must match exactly once;
  narrow with `under`/`nth`)." +1 line.
- **L95 (replaceText op line)** gains `— id optional`: ±0.
- **L113–116 (replaceText bullet)**: fold the locator narrowing rule into
  the existing "found 2 matches" sentence: ±0.
- **Mistakes section (L234+)**: add "a `find` that matches several blocks
  refuses and lists them — narrow with `under`, don't guess." +1 line;
  drop the now-moot "Option ids as values" duplicate hint (L240–241
  overlaps L33–35) −1 line.

If only the encoding fixes (§1.1/§1.2) ship: the sole edit is L78–79 (the
compaction sentence flips), ±0 lines.

---

## 9. Ranked actions

| # | change | saving (measured basis) | effort |
|---|---|---|---|
| 1 | ~~Compact the embedded envelope values (§1.1)~~ **DONE** (Wave 0.1, APIV2.md §8.24) | 16–26 % of every object read — **re-measured on the live corpus: −15.5…−26.4 %, total −23.2 %** | trivial |
| 2 | ~~Split the `?ids=` knob: short block labels + full object refs on `edit` (§9a below)~~ **DONE** (Wave 0.2, APIV2.md §8.25) | re-measured live: **0.9–11.5 % (refs, confirmed)**; block labels are **bimodal, not ~15 %** — −19…−22 % on 24-hex minted ids, **0 %** where block ids carry dashes (the local-label charset is dash-free, SPEC §6.1). With action 1: **−33.1 % across the corpus** | trivial |
| 3 | Locators on block-addressed ops (§5) | read-free text edits (~2.4 k → ~50 tok on the quoted-sentence flow); id-free reads become writable | small |
| 4 | `?mode=` profiles replacing include/outline/ids/format (§6.2) | 4× on pure-read calls for small models (§7); −37 % across the task mix for e2b | small-medium |
| 5 | md mention-link short form (§1.3) | up to ~55 % of md reads on mention-heavy docs | small |
| 6 | BSON→slug respelling sweep (already planned, §8.22-deferred) | −19 % on property reads + kills a 722-tok discovery call per space | planned |
| 7 | Amend ADDRESSING §7.1: pins default `none` on API reads, `all` in `full`/export (§6.2) | +22 tok/key/read avoided when pins ship | doc-only |

The knobs were never the problem; the defaults and the encodings were.
One profile knob a 2B model can drive, full ids under it, locators so the
cheapest read is also writable — that is the whole recommendation.

---

## 10. Decisions taken on this review (human, 2026-08-09)

**1. `?ids=` splits; it currently bundles a winner with a loser.**
`CompactIds` is shorthand for two mechanisms with opposite economics
(`export.go:35-37`): `CompactBlockLabels` relabels doc-local block ids to
short suffixes and is **legend-less** (the server resolves them —
`matchBlockRef`: exact, else unique suffix), while `CompactObjectRefs`
shortens object refs **via the refs legend**, which §1.2 measures as a net
loss because 85–90 % of refs are used exactly once. Outline already moves
them independently; the read knob should too.

**`edit` (the default) therefore emits: short block labels, full inline
object refs, no pins.** Block labels are ~15 % of a default read (18.2 tok
per 24-hex id → 2–3 per suffix) and are *simultaneously* cheaper and
easier for a small model to echo back — the only place in this review
where cost and usability point the same way. Note `CompactBlockLabels` is
marked **lossy** (the original ids are not recoverable from the document
alone), so `full`/export keeps full block ids, the refs legend and pins.

**2. The "legend only for refs used ≥ 2×" hybrid is REJECTED.** It was the
cheap way to keep some compaction on object refs without new machinery,
but repeated mentions of one object inside a single document are rare
enough that the hybrid would almost never fire, leaving per-document
counting logic that earns nothing. Object refs go full inline, and the
future direction is a **space-wide short object-id map** — a resolver that
makes a short form addressable across documents rather than within one.
That is a real build (it needs a cheap space-scoped short→full lookup, and
a decision about what happens when a short form stops being unique), and
it is deliberately deferred, not designed here.

Worth recording as its motivation: object ids are the **dominant cost of
search results**, not of documents — a search row measures ≈ 50 tok of
which ≈ 30 is the object id (§3), and search returns rows by the dozen. A
space-wide short id would pay there far more than it ever could inside a
single document, which is the opposite of where the refs legend was aimed.

**3. Space-optional object routes (decided 2026-08-09 — specified in
`APIV2_SURFACES.md` §10.3).** Object ids are content-addressed and unique
across spaces, and `spaceresolverstore.GetSpaceId` already binds
objectId → spaceId as a keyed point lookup — so `spaceId` is redundant on
any route that already names the object. It drops a required argument from
every object-addressed tool, which matters here for a reason this review
keeps running into: the tokens are a rounding error, but `space` is the
argument a small model is most likely to omit or invent, because it never
appears in the user's request. Recorded here for the cross-reference; it is
a surface simplification, not a token knob, and the build items and the
403-vs-404 decision live in the surfaces doc.
