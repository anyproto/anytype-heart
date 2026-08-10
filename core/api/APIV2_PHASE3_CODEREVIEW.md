# API v2 Phase 3 (edit surface) — code review synthesis (2026-07-30)

Four opus lenses over `260635c6d..HEAD` (5 commits): spec/format conformance ·
state-mutation/CRDT correctness · robustness/security · tests + agent
contract. Deduped and severity-tiered; cross-confirmation noted. Three lenses
ran live probes (reproduced bugs, not just read code). The synthesizer
re-verified the top mutation-lens claims in source (marked ✓): `Unmarshal`
never sets `RelationLinks`; `ResetToVersion` repairs only *bundled* relation
links (its own GO-7217 comment names the wipe-on-replay class this leaves
open for custom keys); `marshalForEdit` discards PUT warnings.

> **2026-08-10 — findings retired by the removal of PUT.** APIV2.md §8.27
> removed the full-document replace surface with its whole pipeline
> (`PutObject`, `putPipeline`, `marshalForEdit`, `ResetObject`,
> `preserveEditorOwnedState`). **B5** (dead `Warnings` on the PUT path) and
> **B7** (create-only guards running before the live-type fallback) are
> retired with their subject, as is every reference below to the
> `marshal → mutate-JSON → Unmarshal → ResetToVersion` apply path — PATCH
> has been a child-state `sb.Apply` since v0.3.4. Findings about the op
> layer, the lock story and PATCH itself stand as written.

## Overall verdict

The **op layer** is largely faithful and genuinely well-tested at its own
level: R3 indent arithmetic, move/subtree splicing, the R5 validation net,
error texts, suffix addressing (S3 closed), response economy — all check out,
and the lock story is sound (everything under one `DoContextFullID`, no
TOCTOU, no C1-class pointer escape). What does not hold is the **apply
path**: the marshal → mutate-JSON → Unmarshal → `ResetToVersion` pipeline
resets onto the live object a snapshot that the format *deliberately does not
fully carry* — no RelationLinks, no structural blocks, no resolvedLayout, no
extra object types — and `Apply(NoHistory, DoSnapshot, NoRestrictions)` turns
each absence into real CRDT changes. Unlike the Phase-0 review (surgical
fixes), this one concludes the apply path needs a **correction pass** before
the edit surface can ship.

---

## TIER A — the apply-path cluster (data loss / integrity; fix as one unit)

### A1 — Custom property values are WIPED on replay: snapshot carries no RelationLinks ✓ [CRDT #1]
`anyblockjson.Unmarshal` never populates `RelationLinks` (✓ verified —
`import.go` has no reference). `ResetToVersion` repairs bundled keys only
(smartblock.go:940-951, the GO-7217 partial fix — ✓ its comment describes
this exact failure). The state diff then emits `RelationRemove` for every
**custom** relation key; on replay/reindex/another device,
`changeRelationRemove → RemoveDetail` deletes the value. Concrete: PATCH one
paragraph on an object with a user-defined "Client" property → locally fine,
on the second device "Client" is gone. **Fix:** seed the reset state with the
live state's relation links (`PickRelationLinks`) in the adapter before
`ResetToVersion` (what the import path does).

### A2 — An unnamed object loses its first paragraph (layout conversion fires) [CRDT #2]
The reset state has no `resolvedLayout` (stripped as local/derived), so
`Apply → resolveLayout` sees unset→recommendedLayout as a change and runs
`convertLayoutBlocks`; the title block was just dropped by the format, so
`WithNameFromFirstBlock` copies the first text block into `name` and
**unlinks the block**. PATCH anything on an unnamed page → its first
paragraph disappears. **Fix:** inject the live `resolvedLayout` into the
reset state before Apply.

### A3 — Every edit deletes structural blocks; featuredRelations doesn't come back [CRDT #3]
The format drops header/title/description/featuredRelations (SPEC §7); the
diff emits BlockRemove + root ChildrenIds changes on every edit. Title/header
return via A2's conversion (an accident, not the contract);
`featuredRelations` returns only on a full source rebuild — open clients see
the featured row vanish. §8.2's "the editor regenerates" is only partly true,
and none of this hits the C11 guard or diffStats. **Fix:** preserve the live
structural subtree by id (or re-apply the `template.With*` transforms) in the
adapter before `ResetToVersion`.

### A4 — No restrictions, no type allowlist: any object in the space is editable [robustness #1 + CRDT #4 + CRDT #9 — three findings converge]
`checkEditPreconditions` excludes only 4 sbTypes; `ResetToVersion` applies
with `NoRestrictions` and no object-level restriction check exists in the v2
path. PATCH can rewrite the workspace object, Archive/Home, widgets, dates,
chats, sets' dataview blocks, and **type objects** — bypassing the
`/v2/types` endpoint's own guards (featured-list guard, PUT's objectType
rejection, which PATCH never calls) and omitting the type-specific repair
(`InitTemplate` recommendedLayout) that the mirrored import path performs.
**Fix:** allowlist editable sbTypes (Page/Note/Task/Set/Collection/Template…),
check `sb.Restrictions()` in the adapter, drop `NoRestrictions`.

### A5 — Every PATCH writes a FULL-document snapshot change [CRDT #5]
`ResetToVersion` forces `DoSnapshot`: a one-word edit on a 500-block document
attaches a complete `ChangeSnapshot` to the tree and to sync — defeating the
minimal-diff contract at the change layer even though the Content diff is
minimal — and `checkChangeSize` can reject with `ErrBigChangeSize`, making a
large-but-app-editable object permanently un-PATCHable. **Fix:** don't force
`DoSnapshot` for API edits (flag on `ResetToVersion`, or a dedicated apply
once A1–A3 are handled).

### A6 — Failures are not clean: events fire before push; side effects survive [CRDT #6 + robustness #5]
`ApplyState` mutates the live doc and dispatches events **before**
`pushChange`; a push failure (size, ACL) returns an error while open clients
already rendered the edit and the cache diverges until eviction. And
create-missing options/properties minted during `Unmarshal` survive every
later failure (validation, guard, apply) — a loop of crafted failing PATCHes
is an unmetered write amplifier. **Fix:** resolve/create refs only after
validation passes (see B6), and either accept+document the event window or
apply through a path that pushes before dispatching.

## TIER B — the seams (broken flows an agent hits)

### B1 — Compact-read / full-write seam: PATCH silently writes dangling object refs [conformance #1]
Default GET returns object ids as 5-char legend labels (C4); the edit
pipeline builds a full-id, legend-less doc and **nothing resolves labels on
the write side**: `addItems` appends verbatim (broken members), `removeItems`
silently no-ops, objects/files property values store garbage. SPEC §9a's
resolution rule makes the label "be" a full id. This inverts C4's promise on
the whole write surface. **Fix:** resolve object-id-valued strings by unique
suffix against the space (the §9a wiring allowance) on the mutate path,
and/or accept a `refs` legend in the PATCH body; minimally, existence-check
items/objects values and reject path-addressed.

### B2 — No op can add the first block to a block-less object [conformance #2]
Every `insertBlocks` target resolves against existing blocks; a fresh
shortcut-created page has none, so PATCH cannot add content at all — the
create-then-write flow falls back to PUT, the path agents are steered away
from. A spec gap faithfully inherited. **Fix:** anchor-less `insertBlocks`
(document-root append / `at: "start"|"end"`), spec'd in §2 Phase 3(a) + the
op schema.

### B3 — `replaceText` splices the replacement into markup source unescaped [conformance #5 + tests #3, PROBED]
`strings.Replace` on the markup-encoded text; the replacement is then
re-parsed. Probe: `replace:"2*3*4"` stored `234` with an invented italic;
`[a](b)` became a link. Breaks §7's `edit_text` "deterministic server-side
replace" — neither deterministic nor literal for `*_[]`\`` — and R5-net
inline-markup failures surface without the `ops[i]` prefix. **Fix:** escape
the replacement per SPEC §8.2 for text-bearing blocks (literal blocks stay
literal); prefix R5 markup issues with the op index.

### B4 — `setProperties` validates keys but not values; the output-only denylist is too narrow [tests #2 + robustness #2, PROBED]
Any raw JSON value lands in details (`{"name":{"nested":true}}` → struct in a
string relation, 200) — the server is more permissive than its own published
schema. And the 7-key denylist accepts every other bundled key, including
`revision`/`sourceObject`, letting an agent mark an object bundled-derived
and pin its revision — defeating `guardBundledRevision` itself. **Fix:**
validate values against the relation's format (Phase-2 resolvers already
know it); reject `bundle.LocalAndDerivedRelationKeys` minus the export
exemption list on both PATCH and PUT.

### B5 — `V2EditResult.Warnings` is dead: PUT's documented safety valve doesn't exist ✓ [conformance #4 + robustness #8 + tests #6 — three lenses]
`marshalForEdit` collects C11 warnings and discards them on the PUT path
(✓ verified); nothing assigns `result.Warnings`. A PUT that drops
unrepresentable content returns a clean 200 with no signal — §8.2's stated
mitigation is untrue. **Fix:** thread the warnings into the PUT response;
test it.

### B6 — Unbounded work + create-RPCs inside the object lock [robustness #3 + #4 + CRDT #7]
No op-count cap (10 MiB ≈ 330k ops), `resolveRef` is O(blocks) per op with a
fresh slice each time, no ctx checks in the loop, and each unseen option name
fires a synchronous `ObjectCreateRelationOption` — a full object creation —
while holding the edited object's non-reentrant lock (self-deadlock risk on
any nested Do of the same id; UI blocked for the duration; 200k options
mintable in one request). The published schema bounds (`maxItems`,
`maxProperties`, `maxLength`) are never enforced [robustness #7]. **Fix:**
enforce the schema bounds server-side; cap ops (~512) and created options;
hoist an id→index map; resolve/create missing refs in a pre-pass **outside**
the lock (also fixes half of A6).

### B7 — PUT runs create-only guards before knowing the live type [conformance #3]
`validateDocumentRefs`/`rejectRestrictedType` run on the raw body before the
live-type fallback: a collection PUT omitting `type` is rejected
("items requires type collection, got ''") contradicting §8.2's "absent type
keeps the live type"; restricted-type and template-shape create rules leak
into edits. **Fix:** pin the effective type from the live object before the
R9 layer; gate create-only guards to the create path.

### B8 — Edit pipeline decodes numbers as float64 — corrupts blocks it never touched [conformance #8]
`parseEditDoc` lacks `UseNumber`; any integer > 2⁵³ in `fields`/dataview
filter values is rewritten (or re-emitted as `1e+20`, which then fails the
schema) on every PATCH. `anyblockjson.Validate` itself uses `json.Number` to
avoid exactly this. **Fix:** `UseNumber()` in `parseEditDoc`; teach
`blockIndent` to accept `json.Number`.

### B9 — Raw internal errors leak into 500 envelopes (P4 recurrence) [robustness #6 + #9]
`RespondV2Error`'s fallback puts `err.Error()` on the wire; Phase 3 feeds it
wrapped internals (resolver/marshal/apply chains), and one crafted
table+setCell input produces an unwrapped 500 with an internal message
(empty-ref match on a non-object row — also reject empty row/col/id refs).
**Fix:** generic 500 message + server-side log; `V2ValidationFailed` for the
table case; typed 409/422 for `guardBundledRevision` [CRDT #11].

## TIER C — contained defects and coverage debt

- **C-1** `leafBlockTypes` incomplete (code/file/image/video/audio/pdf/widget/
  row/column/group missing) — `insertBlocks inside` an image passes both the
  pre-check and the V2 net [conformance #6]. Extend the map; fixture per type.
- **C-2** Per-op schemas not C13-strict below the root (`updateBlock.set`,
  `setProperties.set`, `setCell.value` object arms lack
  `additionalProperties:false`); the test only checks the root — walk
  recursively [conformance #7].
- **C-3** **Fixture-lossless mask** [tests #1, PROBED]: every edit fixture
  builds "live" state via `Unmarshal`, so write-back loss is invisible —
  probe showed an unrelated PATCH silently dropping a bookmark's `Title` and
  flipping its `State` with no C11 warning (field-level loss has no warning
  channel). Add a hand-built rich-snapshot fixture asserting byte-survival
  of untouched blocks; extend C11 warnings to field-level drops or document.
- **C-4** The mutate adapter has **zero tests** [tests #4 + CRDT #12] — the
  lock/heads contract, `guardBundledRevision` branches, and everything in
  Tier A is unexercised. A smartblock-fixture test asserting the emitted
  `pb.ChangeContent` list for one `updateBlock` would have caught A1/A2/A3/A5
  at once. Also: no handler-layer PATCH/PUT tests (ETag header, If-Match
  passthrough, dry_run binding, status mapping) [tests #7]; the C11 422
  guard untested [tests #5]; targeting arithmetic tested only at depth ≤1 and
  2 of 5 shapes [tests #8]; no multi-op interdependence tests [tests #9];
  dry-run skips the mutator-side checks so dry≠real on guard failures
  [tests #10].
- **C-5** Option resolution round-trip: export falls back to raw option ids
  when the name misses → write-back creates junk options literally named
  `bafyrei…`; same-named options collapse to first match on every PATCH
  [CRDT #8]. Keep resolvable ids as ids; warn instead of create on id-shaped
  "names".
- **C-6** Multi-type objects lose `ObjectTypes[1..]` on first PATCH, silently
  [CRDT #10].
- **C-7** Dry-run mints `createdBlocks` ids the real run re-mints differently
  (an agent planning a two-step edit 404s) [conformance #9]; dry-run also
  inflates repeated would-create side effects unboundedly [robustness #11].
- **C-8** `addItems`/`removeItems` return all-zero diffStats — success is
  indistinguishable from no-op, which compounds B1's silent no-op
  [conformance #10 + tests #11]. Also `blocksChanged`/`blocksMoved` are not
  disjoint (double-count on edit+move) — document or fix.
- **C-9** diffStats is blind to the Tier-A churn (both diff sides are
  canonical docs, so structural/relation-link/type-key losses cancel) — the
  "accidental full rewrite signal" cannot see the actual change set
  [CRDT #12]. Note: fixing Tier A largely fixes this.
- **C-10** `replace_all` is the only snake_case key in the op vocabulary (C2
  contradiction inherited from the spec body) [conformance NOTE]; op examples
  use 2-char ids that teach an unrealistic shape [tests NOTE]; empty
  rows/columns error text names an empty list [conformance #11]; PATCH/PUT
  retry-safety (idempotency is POST-only by design) deserves an explicit
  "send If-Match for retry safety" doc note [robustness NOTE].

## Verified sound (recorded so they aren't re-raised)
One-lock consistency (If-Match/marshal/ops/apply/heads — no TOCTOU); no C1
pointer escape (`readLiveState` copies the store); block-id round-trip
through the pipeline → minimal Content diff; body reads bounded (10 MiB);
`ensureSpace` + write rate limit + dry-run wired on PATCH/PUT; suffix
addressing on op refs implemented + tested; setCell serves the full §6.1
value set; response economy good (~90-byte plain PATCH response); no
reachable panic found in the op machinery; R3 arithmetic correct (verified by
probe at the tested depths); revision guard faithful to the import mirror.

## Disposition

The op layer survives review; the **apply path does not ship as-is**. Fix
order:
1. **Tier A as one correction pass** in the adapter: seed the reset state
   with live RelationLinks + structural blocks + resolvedLayout + extra
   object types; drop `NoRestrictions` (add restriction checks + sbType
   allowlist); drop forced `DoSnapshot`; move create-RPCs outside the lock.
   Then add the C-4 adapter test asserting the emitted change set for a
   one-block edit — the test that would have caught A1/A2/A3/A5.
2. Tier B seam fixes (B1 suffix-resolution on writes, B2 root insert, B3
   escape, B4 value validation, B5 warnings, B6 bounds, B7 ordering, B8
   UseNumber, B9 hygiene).
3. Tier C coverage + polish.
Nothing invalidates the op vocabulary, the document-level pipeline concept,
or the endpoints — the correction is concentrated in
`objectmutateadapter.go` + `v2_edit.go` seams, not the op set.
