# Flat-blocks implementation review — findings for the fixing agent

Reviewed: the uncommitted flat-blocks conversion in `pkg/lib/anyblockjson`
(working tree on top of `4aacdbb10`), against the normative brief `FLAT.md`
(rules F1–F10, V1–V5) and `SPEC.md` v0.6. · 2026-07-23 · GO-7383

Method: 6 dimension reviewers (export, import, validate+schema, tests,
spec-conformance, live behavioral probes) → dedup/triage → adversarial
verification. **Verification was stopped early by the user after 2 verdicts**,
so: finding #2 is CONFIRMED (2/2 verifiers, empirical repro); every other
finding is **unverified — reproduce before fixing**. Finding #1 has
conflicting reviewer reports (see its status line) — reproduction is
mandatory there.

Ground rules for the fix pass:

- The full package suite currently passes (`go test ./pkg/lib/anyblockjson/...`).
  It must still pass after every fix, plus a regression test per fixed finding.
- The V1/V2/V3 error strings are asserted as exact text in tests and are
  the agent-facing repair API — extend, don't casually reword.
- Design decisions are settled (FLAT.md §7.1); fix findings, don't
  relitigate flat-vs-nested, the clamp rule, or the cell recursion cut.
- Do not touch the invariants listed under "Verified clean" (§C) except
  where a finding requires it.
- Commit style: `GO-7383 <description>`; error wrapping per repo CLAUDE.md
  (`fmt.Errorf("operation: %w", err)`).
- If you write probe tests, name them `zz_*_test.go` and delete them before
  finishing (`git status --short` must show only intended files).

---

## A. Findings (fix in this order)

### 1. [MAJOR] Marshal can emit a schema-invalid table cell when a cell has a nested-table descendant

- **File**: `table.go:173` (`cellToJSON`), schema `$defs/cellBlock` / `$defs/tableCell`
- **Status**: unverified — **conflicting reviewer reports**. Two reviewers
  (import + validate-schema dimensions) report it broken, one with a claimed
  live repro; the export-dimension reviewer reported the same path CLEAN
  ("a table block inside a cell exports as a valid … flat table element in
  the array form and round-trips"). **Reproduce first; trust the repro.**
- **Claim**: `cellToJSON` serializes cell descendants via
  `appendBlocksFlat` (`arr := []any{m}; e.appendBlocksFlat(&arr,
  cell.ChildrenIds, 1, false)`), which renders *any* child — including a
  `table` block (`{type:"table",columns,rows}`). But the schema types cell
  array items as `cellBlock` (= `blockCore` + `type: {not:{const:"table"}}`,
  `unevaluatedProperties:false`, no `columns`/`rows`), so Marshal would
  produce a document its own Validate/Unmarshal rejects.
- **Claimed repro**: snapshot with a table cell (text block) whose child is a
  table block → Marshal(OmitIds) succeeds emitting
  `cells:[[{type:paragraph},{indent:1,type:table,…}]]` → re-import fails
  schema validation (`…/cells/0/1/type: 'not' failed`, `…/columns: false
  schema`) → Export∘Import data loss for that object. Prod sweep
  (ANOMALIES #11) found 0 cells-with-descendants, so this needs legacy/
  adversarial data — but Marshal must never emit output its own Validate
  rejects, regardless of reachability.
- **Fix direction**: make `cellToJSON` handle a `table` (or other
  cell-forbidden type) among cell descendants explicitly — drop with a
  warning + ANOMALIES note, or flatten to a representable form — so export
  output always validates. Also delete the stale comment at
  `validate.go:480` ("nested tables join the id uniqueness domain") if it
  survives the fix.
- **Regression test**: exactly this snapshot shape; assert
  `Validate(Marshal(S))` is nil and the behavior (drop/flatten) is
  deterministic and warned.

### 2. [MAJOR — CONFIRMED 2/2] `indentOf` silently reads integer-valued floats as indent 0, bypassing V1/V2/V3 and diverging from Unmarshal

- **File**: `validate.go:275` (`indentOf`), cf. `checkVersion`
  (`validate.go:179–188`), `import.go:45` (`Indent int`)
- **Status**: **CONFIRMED** by both adversarial verifiers with live repro
  before verification was stopped.
- **Claim**: JSON Schema 2020-12 accepts `2.0` as `type: integer`
  (santhosh-tekuri v6 does), but `indentOf` does
  `num.Int64(); if err != nil { return 0 }` — so a float-form indent is
  silently read as **0** during semantic validation, while `Unmarshal`'s
  `Indent int` rejects the same bytes.
- **Verified repro**:
  `{"version":1,"blocks":[{"type":"divider"},{"indent":1.0,"type":"paragraph"}]}`
  → `Validate` returns **nil** (V2 divider-containment never fires; with
  integer `1` it correctly errors); `"indent":5.0` after an indent-0 block
  bypasses V1's jump check the same way. The identical bytes then fail
  `Unmarshal` with a decode error — a Validate-passes/Unmarshal-fails
  divergence. `checkVersion` already handles this exact case with
  `Float64()+math.Trunc` — in-file inconsistency.
- **Fix direction**: in `indentOf`, on `Int64()` error fall back to
  `Float64()` + integrality check (mirror `checkVersion`); non-integral →
  the schema already rejects. Consider also making `jsonBlock.Indent` a
  `json.Number`-tolerant decode OR keeping `int` and accepting that the
  float form is *valid but normalized* — whichever you pick, **Validate and
  Unmarshal must agree on every input**.
- **Regression test**: float-form indents (`1.0`, `5.0`, `1e0`) behave
  identically in Validate and Unmarshal (both accept-with-value or both
  reject), and V1/V2 fire on float-form violations.

### 3. [minor] Export emits indent > 32 for snapshots nested deeper than 32 — output its own validator rejects

- **File**: `export.go:467` (`m.setNonEmpty("indent", depth)`; unbounded
  `depth+1` at `export.go:436`); schema bound `indent maximum:32`
- **Status**: unverified; the finder reported reproducing via a throwaway
  test (Marshal produced indent 33–39; Unmarshal rejected in both modes —
  `blockIndents` doesn't clamp >32 even lenient).
- **Failure**: a legacy/adversarial snapshot ≥ 33 levels deep marshals
  fine, then can never be re-imported. Regression vs the nested format
  (no depth cap). Typical real max is ~6 (histogram: p99 well below 10),
  so edge-case — but Marshal must not emit self-invalid output.
- **Fix direction**: at the F4 bound, either error loudly from Marshal
  naming the block id, or clamp-with-`fields`-escape-hatch; erroring is
  more honest and matches the loud-failure doctrine. Document in SPEC §11
  (export-side bound) + ANOMALIES if the prod sweep ever hits it.
- **Regression test**: 40-deep chain snapshot → Marshal errors (or clamps,
  per choice) — and `Validate(Marshal(S))` nil for every S that Marshal
  accepts (this is a good general property to assert on the generated-docs
  suite while you're here).

### 4. [minor] Removed/unknown property (e.g. `children`) rejects with opaque `false schema` instead of naming the key

- **File**: `validate.go:145` (flatten appends
  `ErrorKind.LocalizedString` verbatim)
- **Status**: unverified (behavior confirmed in probe notes: `children` →
  `/blocks/0/children: false schema`; table-in-cell yields ~8 sibling noise
  issues for one problem).
- **Why it matters**: FLAT.md F5 promises `children` fails "as unknown
  property"; the repair loop gets a path but no rule name. This is the #1
  error agents will actually hit (every nested-era generation).
- **Fix direction**: post-process `false schema` / unevaluatedProperties
  errors into `property "children" is not allowed (the flat format has no
  children — use indent, §4)` — special-case `children` with the
  migration hint; generic unknown keys get `property "X" is not allowed`.
  Consider collapsing sibling noise issues for the same instance path.
- **Regression test**: assert the improved message text for a `children`
  doc and for one arbitrary unknown key.

### 5. [minor] F10 array-form cell has zero round-trip test coverage

- **File**: `table.go:171` (`cellToJSON` array branch),
  `table.go:332–344` (`cellFromJSON` → `flatSubtree`)
- **Status**: unverified as a gap; note the probes DID run one
  array-form-cell round-trip successfully (§C.5), so the path works for
  the simple case — the gap is in the *committed* test suite.
- **Fix**: permanent test — cell whose block has a text child: Marshal
  emits `[[…]]`, Unmarshal rebuilds cell + child, Export∘Import
  byte-stable. (Extends naturally into finding #1's regression test.)

### 6. [minor] V2 leaf-containment tested for 1 of 12 leaf types; leaf-set/export agreement unasserted

- **File**: `validate_test.go:116`; `validate.go:239–244`
  (`leafBlockTypes`); `export.go:498–575` (withChildren sites)
- **Fix**: add V2 cases for `table`, `dataview`, `bookmark`/`link`, and one
  more; add an assertion that `leafBlockTypes` == the export
  withChildren=false set ∪ {equation} (reviewers verified they agree
  TODAY — §C.3; the test prevents drift).

### 7. [minor] Lenient-mode clamp × V2/V3 interaction untested despite explicit code claim

- **File**: `validate_test.go:224` (TestNormalizeIndent);
  claim at `validate.go:287`
- **Fix**: NormalizeIndent subtest: `[{type:divider},{indent:5,…}]` →
  clamps to 1 AND still errors "nested under a divider block"; same for
  non-column clamped under a row. (Probes confirmed the behavior is
  correct today — §C.6; the test locks it.)

### 8. [minor] Prefix-property test capped at depth 2

- **File**: `validate_test.go:274` (TestValidate_PrefixProperty over
  richSnapshot, max indent 2)
- **Fix**: run the same prefix loop over a depth ≥ 6 document (hand-built
  chain or a deep generated doc). Depth-6 is the stated real-world regime.

### 9. [minor] SPEC §4 error example uses `blocks[7]` but implementation emits `/blocks/7`

- **File**: `SPEC.md:343`; code `validate.go:379,398`;
  asserted in `validate_test.go:217–218`
- **Fix**: change the SPEC example to `/blocks/7: indent 3 follows…` (or
  note `blocks[N]` ≡ pointer `/blocks/N`). The error text is API; the spec
  must show the real string.

### 10. [minor] SPEC §10 "additive only within version 1" contradicts the v0.6 breaking change

- **File**: `SPEC.md:979` vs the v0.6 changelog (`SPEC.md:27–30`)
- **Fix**: add to §10: additive-only applies from the first non-draft
  release; while draft, breaking changes may occur under version 1 (as
  children→indent did).

### 11. [minor] FLAT.md V2 says "12 types" but enumerates 11 (missing `equation`)

- **File**: `FLAT.md:305` — add `equation` (or "embed and its equation
  alias") so the list matches the count. Code and SPEC are correct.

### 12. [minor] FLAT.md acceptance criterion #2 ("no recursive $ref, grep proves it") is false — filterNode legitimately recurses

- **File**: `FLAT.md:476` — scope the criterion to the block/cell
  definitions; the dataview `filterNode` tree stays recursive by design
  (SPEC §12 documents it). Don't "fix" the filter recursion.

### 13. [minor] FLAT.md F9 rationale is wrong about how cell recursion was cut

- **File**: `FLAT.md:342` — F9 claims the cell `$ref` becomes
  "automatically non-recursive" once block drops children; actually the
  block→table-arm→tableCell→block cycle needed the dedicated `cellBlock`
  def + the "cells cannot contain `table`" restriction (which the
  implementation added, and SPEC §6.1/§12 + ANOMALIES #11 record). Update
  F9 to describe the real cut so the brief matches shipped reality.

---

## B. Also noted, not findings

- ANOMALIES.md depth histogram (lines ~159–163) sums to 35,373 vs the
  stated 35,372 objects — off-by-one, cosmetic; fix if touching the file.
- Probes confirmed **`column` at top level is ACCEPTED** — V3's reverse
  direction is deliberately unenforced (parity with the nested schema,
  FLAT.md §7.2 open item). Leave as is unless the user decides otherwise.

## C. Verified clean — do not break these

Condensed from reviewer coverage notes and 18 green live probes (probe
files were deleted; behaviors verified against the working tree):

1. **F6 stack rebuild**: `flatSubtree` seeded (root,−1); pop-past-root
   impossible; equal-indent runs after deep pops attach correctly
   (`[0,1,2,3,1,2,0,1]` verified); validation's `checkFlatRun` frame stack
   and import's rebuild use the identical parent relation.
2. **F7/F8**: strict rejections all fire with correct path+text (first
   block ≠ 0, jump > +1 naming both indents, 33/negative via schema,
   leaf/row containment); clamp cascades track the *clamped* previous
   value consistently in both validate and import; clamped docs re-export
   byte-stable and pass strict Validate.
3. **Leaf-set agreement (today)**: `leafBlockTypes` (12 incl. equation) ==
   export withChildren=false sites (11; equation exports as embed) ==
   SPEC §5 == old schema arms. `widget`, `file` family, `group`,
   `row`/`column` parent-capable everywhere.
4. **Export canon**: indent-first key order, omit-0 (incl. return-to-0
   after deep runs), OmitIds keeps indent / drops ids, CompactIds relabels
   ids / leaves indent; all four golden fixtures consistent.
5. **Cells**: bare cell block never carries indent; array form starts
   descendants at indent 1; id-strip works; a simple array-form cell
   (paragraph + child) round-trips byte-stable; `table`-in-cell and
   id-on-first-element are *rejected by Validate* (the open question is
   export's side — finding #1).
6. **Lenient × containment**: V2/V3 evaluate on clamped indents and still
   error in NormalizeIndent mode (behavior verified; test coverage is
   finding #7).
7. **§7 structural blocks**: title at indent 0 absorbed into `name`,
   its indented run dropped with it, siblings kept.
8. **Untrusted graphs**: shared child emitted once (first parent wins,
   contiguous runs, re-import = N(S)); leaf-with-children in the snapshot
   drops children silently (matches nested semantics); export can never
   emit an F7 violation.
9. **Prefix validity**: holds for every prefix of the rich export
   (tables self-contained) — depth extension is finding #8.
10. **Schema**: only `filterNode` recurses (26 $refs traced); `cellBlock`
    $refs `blockCore` (no prop duplication/drift); indent bounds and
    `children` rejection enforced; float `1.5` indent rejected (the
    integral-float hole is finding #2).

## D. Acceptance for the fix pass

1. Findings #1–#4 fixed (majors first), #5–#8 test gaps closed,
   #9–#13 doc corrections applied.
2. `go test ./pkg/lib/anyblockjson/...` green; no `zz_*` probe files left
   (`git status --short`).
3. New general property while you're in there (from #3): for every
   snapshot the generated-docs suite produces, `Validate(Marshal(S))` is
   nil — Marshal never emits self-invalid output.
4. If any fix changes an error string, update the asserting test AND
   SPEC §4/§12 examples together.
5. Re-run the prod sweep (`cmd/anyblockroundtrip`) only if fix #1 or #3
   changes export behavior; acceptance = parity with run 3 (99.86%) and
   the known residue.
