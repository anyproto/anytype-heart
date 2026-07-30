# APIV2.md review — synthesis of 3 opus lenses (2026-07-23)

Lenses: format-correctness (vs SPEC v0.6) · agent-ergonomics (vs the evidence
base) · implementation-feasibility (vs middleware code). Findings deduped,
adjudicated, and ranked; cross-confirmation noted. "Adjudication" = the
synthesizer's resolution where reviewers proposed different fixes.

Overall verdict (shared across lenses): the conventions layer and phase
structure are sound and evidence-aligned, but (a) one load-bearing
contradiction breaks the default read→edit loop, (b) the PATCH op set is
under-specified exactly on the edge cases the format makes reachable, and
(c) several things are presented as existing that are new build work.

---

## Tier 1 — must fix before anyone implements (cross-confirmed)

**R1 · Compact ids by default break block addressing and the PUT diff
contract.** [confirmed by ALL THREE lenses]
C4/Phase 1 default `ids=compact`; SPEC §9a relabels **block** ids to 5-char
doc-local labels with **no legend** (only object ids get `refs`), and
canonical round-trip is explicitly the full-id export. So the natural
GET → edit → PATCH/PUT loop sends block labels the server cannot resolve;
PUT degenerates to full-tree delete+create — the exact DELEGATE-52
signature diffStats exists to expose.
*Adjudicated fix:* split id compaction: default reads = **compact object
ids (refs legend, lossless) + FULL block ids**; block-label compaction only
in explicitly read-only shapes (outline, prompt exports). Additionally,
write endpoints MAY resolve block-id references by unique-suffix match
against the live object (SPEC §9a already sanctions this at the wiring
level) as a lenient convenience. Requires a small package change: decouple
object-ref compaction from block relabeling in `Options` (today one
`CompactIds` flag does both).

**R2 · `revision` is undefined, has no backing field, and whole-object
If-Match 409s on sync noise.** [feasibility ×2 majors + ergonomics major +
format minor]
No usable `revision` exists on user objects (`RelationKeyRevision` is for
bundled objects only — objectcreator.go:389); real tokens are tree heads /
`lastChangeId`, which are CID-length (violating C4's no-long-echo rule if
required in the body) and advance on every background-sync tick, so
whole-object If-Match would 409 on changes the agent never caused. The
name also collides with the `revision` relation key an object's
`properties` can contain.
*Adjudicated fix:* rename to **`etag`**: short opaque token (e.g. 8-char
head-hash derivation), returned on reads (+ `ETag` header), accepted via
`If-Match` header only. Make it **optional/advisory by default**
(last-write-wins per-op + diffStats), strict opt-in. Record block-scoped
staleness (ops apply iff the *addressed* blocks are unchanged) as the
deferred v2.x refinement — id-addressed ops make it tractable.

**R3 · `insertBlocks inside` relative-indent rule is self-contradictory.**
[format + ergonomics]
"0 = anchor level" is right for `after`/`before`/`replaceSubtree` but wrong
for `inside` (would make the payload a *sibling* of the container).
*Fix:* `inside` ⇒ payload `indent: 0` = container's child level (anchor+1).
Add two worked examples (sibling-insert, child-insert) — C12 demands them
anyway.

**R4 · No attribute-level edit: `replaceBlock` silently wipes text on a
checkbox toggle.** [format major]
SPEC §4: absent `text` = empty text. So `replaceBlock {id, block:
{type:"checkbox", checked:true}}` erases the block's text — a silent
data-loss trap on the most common small edit. Table cells are likewise
only editable by replacing the whole `table` block, yet Phase 0's harness
lists "fill a table cell" as a task.
*Adjudicated fix:* add **`updateBlock {id, set:{…}}`** — merge semantics on
the op level (only provided fields change; `text` untouched unless
provided); BlockNote's `update` precedent. Keep `setCell` deferred but
re-express the harness task as replaceBlock-whole-table at launch, and gate
`setCell` on it.

**R5 · Post-op validity is never required; format-reachable edge cases
undefined.** [format major]
Ops can produce documents the format rejects, each with unspecified
behavior: type-change to a leaf with descendants kept; insert/move `inside`
a leaf type; `moveBlock` into its own descendant (cycle); column/row
containment breaks.
*Fix:* one normative sentence — every op's result must satisfy SPEC §12
semantic checks (V1–V5); violations reject the whole PATCH with
path-addressed errors — plus explicit cycle-prevention for `moveBlock`.

**R6 · Two "shipping" dependencies are actually unbuilt (and one is
underestimated).** [format + feasibility]
(a) The §6.2.1 filter string is *reserved, post-v1, dataview-scoped* in
SPEC — reusing it for search is new design (date encoding differs: RFC 3339
/preset functions vs unix numbers in structured filters) and a new parser.
(b) `GenerateSchema` is planned, not implemented — and the "table with live
option names" flavor needs an objectstore join (options live on option
objects, not in `typeProperties`).
*Fix:* mark both as build items with owners in the phase plan; keep
placement (string in Phase 4, schema endpoint in Phase 1) but stop implying
availability.

**R7 · `typeKey` vs `type` reintroduces the read/write vocabulary split C2
promises to kill.** [ergonomics major]
The envelope field is `type`; rows/search/shortcut using `typeKey` forces
renaming between read and write.
*Fix:* `type` everywhere; define the `POST /objects` doc-vs-shortcut
discriminator explicitly (presence of `version`/`blocks` ⇒ full document).

**R8 · The small-model premise is never enforced on the discovery schemas —
and the structured `filters` array is recursive.** [ergonomics major]
Nothing requires `/v2/schemas/{kind}` outputs to be strict-mode-compatible;
the filterNode tree is recursive, hence not constrained-decodable at all.
*Fix:* new convention (C13): all op/create/property/type/filter schemas
served by discovery are strict-mode-compatible (non-recursive,
`additionalProperties:false`, bounded). Document `filters` as the
exception, and make the *string* filter the settled small-model form —
which also narrows benchmark B2 to mid/frontier programmatic composition
instead of relitigating primacy.

## Tier 2 — real defects, contained

**R9 · `/validate` is structural-only**; the advertised referential errors
(option lists, type's actual keys) need a net-new space-aware layer. Split
the contract: structural in Phase 0, referential as named Phase 2 work.
[feasibility]

**R10 · `POST /sets` is not one atomic RPC**: `ObjectCreateSet` takes no
filters/sorts/views. Either orchestrate create-then-configure or (better,
adjudicated) construct the set's initial state with a fully-formed dataview
block via the AnyBlock create path — genuinely one change set. Soften the
"one transaction" claim accordingly. [feasibility]

**R11 · Missing surface vs the "v1 superset" claim**: no file upload (file
blocks and `iconImage` need file object ids — v1 has upload), no
spaces/members read, no archive/delete object, no type/property
update/delete. Add `POST /files` to Phase 2, spaces/members reads + archive
to Phase 1–2, or qualify §6's superset claim. [feasibility]

**R12 · Read-param contract**: no precedence/legality matrix for
`include × outline × block × format × ids`; `outline` ambiguous (headings
only vs full skeleton — if headings-only, outline-then-fetch breaks for
non-heading targets). Define outline = every block's `{indent,id,type}`
with `text` only on headings; add the matrix. [ergonomics]

**R13 · Benchmark program repairs**: B1's PUT-only arm is a corruption
*baseline*, not a gate arm — reframe (don't drop; it anchors the
DELEGATE-52 comparison). Small-model tiers must run under constrained
decoding (else 3–8B loops produce null data and gates are undecidable).
B4 tunes prompt/SKILL guidance, not the C12 endpoint docs. [ergonomics]

**R14 · Op-contract gaps**: `deleteBlock` without `recursive` on a parent =
undefined (fix: error naming descendant count; state the default);
`moveBlock` addressing weaker than `insertBlocks` (give it
`after`/`before`/`inside+position` — enables indent/outdent; add example);
`setProperties` must define empty-vs-unset (`set:{k:[]}` = present-empty;
`unset` = absent) and reject output-only keys; created-block id map needs a
correlation basis (payload array index; optionally client temp ids).
[format + ergonomics]

**R15 · Error-catalogue gaps**: `version_unsupported` (SPEC §10's
"produced by a newer version" must surface verbatim); idempotency conflict
(same key, different body → defined 409); `filter`+`filters` both supplied
→ defined C6 error. [ergonomics]

## Tier 3 — notes

- `diffStats` and `warnings` need concrete schemas (reuse the §12 Issue
  shape for warnings); PUT carries the etag via `If-Match` only (the
  AnyBlock envelope has no such field). [format]
- Phase 1 impl note "ObjectShow → Marshal" is a type mismatch; read via
  live smartblock state → snapshot → Marshal, and derive `etag` from that
  same state (store snapshots lag). [feasibility]
- `format=md` warnings require instrumenting `core/converter/md` (today it
  silently drops dataview/widget/etc. — no error channel). New work, note
  it. [feasibility]
- Resolver wiring (create-missing properties/options bridging anyblockjson
  to objectstore RPCs) is Phase 2's real substance — budget it. `POST
  /templates` should target the generic AnyBlock create path (no
  create-from-body template RPC exists). [feasibility]
- Per-op schema fetch (`/v2/schemas/ops/{op}`) with single-op examples;
  the 6-op composite stays as a secondary illustration. C10 (limits +
  steering) applies to types/properties/options lists too — options can be
  thousands of entries; add a prefix filter. "OmitIds shape" phrasing →
  "id-less input document (SPEC §9)" (`OmitIds` is export-only). [ergonomics]

---

## Disposition summary

Every Tier-1 item changes APIV2.md text; R1/R2 also touch the package
(`Options` split) and server design (etag). Nothing found invalidates the
phase structure, the op-set approach, or the benchmark program's existence —
the defects are contract precision, three false "already exists" claims,
and the id/etag echo loop. Recommended order: apply R1–R8 as one spec
revision (v0.2), fold Tier 2/3 in the same pass, then re-run a single
verification read against SPEC v0.6.
