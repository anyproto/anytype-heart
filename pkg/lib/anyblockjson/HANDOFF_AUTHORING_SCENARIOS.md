# Handoff — authoring-mistake scenario catalogue

**Status: specified, not started.** Nothing has been built yet. This document is
self-contained: it assumes no context from the session that produced it.

Repo: `/Users/roman/anytype/anytype-heart_go-7383-anyblockjson` (a git worktree —
work only there, never `cd` to the main checkout).

---

## 0. The one rule that matters most

**Never run a state-changing git command.** No `checkout`, `restore`, `stash`,
`clean`, `reset`, `commit`, `add`. Read-only git (`git show`, `git log`,
`git diff`) is fine.

This is not boilerplate. During the session that wrote this document, two
separate agents ran `git checkout` to "clean up after themselves" and destroyed
a large body of concurrent uncommitted work, twice. The worktree carries ~120
modified files that are **intentional and uncommitted**. To remove a scratch
file, delete it by exact name (`rm path/to/file`).

Also: add **no new module dependency**. CI runs `license_finder` against
`anyproto/open`'s `decisions.yml`, so a new dep blocks the build until someone
files a decision entry. Everything below is implementable with the standard
library.

---

## 1. The goal

AnyBlock JSON (`pkg/lib/anyblockjson`) is a JSON document format about to freeze
at v1. Its **primary author is an LLM agent** — the spec says so, and there is a
dedicated "authoring subset" (§2g, `schema/authoring/*.schema.json`,
`ValidateAuthoring`) built for exactly that consumer.

The goal, in the project owner's words: *an agent cannot break things, and even
when it writes something invalid it must get the correct error to fix it from.*

That decomposes into two testable properties:

1. **Safety** — no input causes a panic, hang, OOM, or a *silent* bad import.
2. **Actionability** — every rejection produces an error an agent can recover
   from unaided. This one is barely tested today and is what this work targets.

Actionability decomposes further, and the spec already commits to most of it:

- **One fault, one issue** (§12 promises this). Twelve issues for one typo makes
  an agent flail.
- **Correctly path-addressed** — the JSON Pointer names the offending node.
- **The repair is named** — the spec says "with the repair named" repeatedly.
- **The repair actually works** — apply what the message says and the document
  becomes valid. Nobody tests this today. It is the bar the owner chose.

---

## 2. Evidence already gathered

I probed `ValidateAuthoring` with plausible LLM mistakes against
`pkg/lib/anyblockjson/testdata/authoring/habit_tracker/objects/morning-run.json`.
Reproduce by writing a throwaway `_test.go` in the package that mutates that
fixture and calls `ValidateAuthoring`. Findings, verbatim:

**Error quality is inconsistent by layer.** The curated semantic rules are
excellent:

```
[/properties/createdDate] "createdDate" is a timestamp the app stamps — the app
derives it, so an author does not write it. Every spelling of that key is
refused here, not just this one
```

The raw schema refusals are not:

```
[/blocks/0/colour] property "colour" is not allowed
```

Same document, same author, completely different quality of help. The second is
not machine-applicable: it does not say to delete the member, and does not
suggest `color`, which is one edit away.

**No "did you mean" anywhere.** `bulleted_list` (instead of
`bulleted_list_item`) produces a 21-value list to choose from. `colour` produces
nothing. Edit-distance suggestion against the closed vocabularies is likely the
single highest-value cheap improvement in this whole surface.

**Document-level validation structurally cannot catch the most likely agent
mistake.** Both of these are **accepted silently**:

- `"type": "Habitt"` (a typo'd type name)
- `"last_done"` instead of `"Last done"` (a snake_case slug where the display
  name belongs) — and it imports as detail key `last_done`, i.e. a **new phantom
  property**, not the one the author meant.

This is not a bug in `Validate`. A single document carries no vocabulary, and §3
says a term the resolution chain does not know passes through verbatim.
**Bundle-level checking does catch it** — `anyblockbatch.CheckPropertyFormats`
reports both the spelling and what it resolved to.

**Consequence for the design: the test unit must be the bundle, not the
document.** The highest-value agent errors are only catchable there.

**Silent normalizations exist by design** — e.g. a scalar where a multi-select
list belongs (`"Frequency": "Daily"`) is accepted and normalized. Correct
behavior, but the author gets no signal that what they wrote was not canonical.
Worth capturing as `must_warn` scenarios (probably currently `known_gap`).

---

## 3. Plan

| Stage | What |
|---|---|
| 1 | Build the harness: scenario file format, parser, JSON Patch engine, runner, 3 seed scenarios |
| 2 | Fan out scenario authoring across 10 fault families (parallel agents) |
| 3 | Adversarial verification of every scenario by a different agent |
| 4 | Completeness critic: which implemented refusals have no scenario |

Stage 1 must finish and be proven before stage 2 — the scenario files are the
tests, so the format has to be real first.

**The output is not a green suite. It is a punch list**: every scenario marked
`known_gap` is an authoring mistake that currently gets no error or an unusable
one, with a reproduction attached. That list is the deliverable, to be fixed
before the format freezes.

---

## 4. Stage 1 — the harness

### Where

**`cmd/internal/anyblockbatch`.** That package describes itself as "the import
wiring", already imports `pkg/lib/anyblockjson`, and owns the bundle-wide
checks. Putting the harness there lets one runner validate at both the document
and bundle level without the format package importing upward (SPEC §13 forbids
the format package depending on import/export wiring).

Scenario files: `cmd/internal/anyblockbatch/testdata/scenarios/<category>/<id>.md`

### The scenario file format

Implement the parser to this exactly — stage-2 agents will write hundreds of
these against this spec.

````markdown
---
id: prop-slug-instead-of-name
category: property-spelling
source: SPEC §3 — a key's spelling is the entity's display name
surface: document
target: objects/morning-run.json
verdict: must_error
expect_path: /properties/last_done
expect_message:
  - Last done
status: expected
---

Prose: what the author did, why it is a plausible mistake, what the correct
form is. One or two paragraphs.

## Mutate

```json
[{"op": "move", "from": "/properties/Last done", "path": "/properties/last_done"}]
```

## Repair

```json
[{"op": "move", "from": "/properties/last_done", "path": "/properties/Last done"}]
```
````

Field semantics:

| Field | Meaning |
|---|---|
| `id` | kebab-case, unique across all scenarios, MUST equal the filename stem |
| `category` | free string, used for grouping in the summary |
| `source` | SPEC section or code symbol this is derived from. Required, non-empty |
| `surface` | `document` or `bundle` |
| `target` | path relative to the bundle root of the file to mutate |
| `verdict` | `must_error` \| `must_warn` \| `must_accept` |
| `expect_path` | JSON Pointer where the issue must appear. Required for `must_error`/`must_warn`, forbidden for `must_accept` |
| `expect_message` | list of substrings the message must contain (may be absent) |
| `status` | `expected` (behavior implemented) \| `known_gap` (documents a gap) |

Sections: `## Mutate` (required) and `## Repair` (required unless verdict is
`must_accept`), each a fenced ```json block holding a JSON Patch array.

Write a **minimal front-matter parser** — only scalars and lists of scalars are
needed; do not add a YAML dependency. Fail loudly with the filename on any
unknown key, missing required key, or malformed patch. A broken scenario must be
a loud failure, never a silent skip.

### JSON Patch subset

Implement `add`, `remove`, `replace`, `move` over `map[string]any` / `[]any`,
with RFC 6901 pointer parsing including `~0`/`~1` unescaping and `-` for array
append. **Unit-test the patch engine itself** — it is load-bearing for every
scenario, and a silently wrong patch makes a scenario pass for the wrong reason.

### The base bundle

`pkg/lib/anyblockjson/testdata/authoring/habit_tracker/`:

```
index.json
properties.json
types/habit.json
objects/start.json
objects/morning-run.json
objects/weekly-review.json
```

Load it fresh per scenario. **Never mutate it on disk.** For bundle-surface
scenarios, materialize the mutated bundle into `t.TempDir()` and run the checks
over real files.

Note it is already covered by `TestAuthoringExample_HabitTracker` in
`pkg/lib/anyblockjson/authoring_test.go`, which asserts the pristine bundle is
valid, warning-free and internally coherent. Read that test first — it shows how
the bundle hangs together.

### What the runner asserts

1. Load the pristine bundle; apply `Mutate` to `target`.
2. Validate for the surface:
   - **document** → `anyblockjson.ValidateAuthoring` for object and type files;
     `ValidateAuthoringIndex` for `index.json`; `ValidateAuthoringPropertyDictionary`
     for `properties.json`. Collect warnings separately via `ValidateWarn`.
   - **bundle** → materialize to a temp dir and run the same check set
     `cmd/anyblockvalidate/main.go` runs. Read that file; the set is
     `ScanFormats`, `DictionaryFormats`, `MergeDictionaryFormats`,
     `CheckPropertyFormats`, `CheckViewProperties`, `CheckIndexTargets`,
     `CheckManifestFiles`, `CheckBundleIds`, `CheckSharedSelects`,
     `UnboundFileDocuments`. Signatures are in
     `cmd/internal/anyblockbatch/scan.go`.
3. Assert the verdict:
   - `must_error` — a `*anyblockjson.ValidationError` containing an issue at
     `expect_path` whose `Message` contains **every** `expect_message` substring.
   - `must_warn` — no error, and a warning at `expect_path` matching likewise.
   - `must_accept` — no error and no warnings.
4. **Repair convergence** (skip only for `must_accept`): apply `Repair` to the
   *mutated* document; the result must validate with **no error and no
   warnings**. This is the strict bar — it proves the stated repair actually
   recovers the document.
5. **Known-gap inversion.** `status: known_gap` means the behavior is not
   implemented. Run the same checks; if they now **pass**, FAIL with a message
   telling the author to change `status` to `expected` — so closing a gap forces
   the catalogue to update. If they still fail, record and `t.Log`; do **not**
   fail the suite.

### Gotcha that will bite you

`ValidateAuthoring` runs the full validation first, then the subset schema. When
a document is valid AnyBlock JSON but outside the authoring subset,
`validateAuthoringSubset` emits a **preamble issue at the document root** *plus*
the real refusal. So "exactly one issue" is wrong as an assertion — the runner
must tolerate extra issues and look for the one at `expect_path`.

Observed shape:

```
issues: 2
  - []                          valid AnyBlock JSON, but outside the authoring subset — …
  - [/properties/createdDate]   "createdDate" is a timestamp the app stamps — …
```

### Deliverables

1. `cmd/internal/anyblockbatch/scenario.go` — parser + patch engine.
2. `cmd/internal/anyblockbatch/scenario_test.go` — unit tests for both,
   including malformed input.
3. `cmd/internal/anyblockbatch/authoring_scenarios_test.go` —
   `TestAuthoringScenarios`, walking the scenario directory.
4. **Three seed scenarios** proving all three verdicts and both surfaces:
   - `must_error`, document: writing stored key `createdDate` into `properties`
     is refused at `/properties/createdDate`, message contains
     `the app derives it`. (Verified — this is the preamble case above.)
   - `known_gap`, document: `"type": "Habitt"` in `objects/morning-run.json` is
     currently accepted silently; it *should* error naming the unknown type.
     Write as `verdict: must_error`, `status: known_gap`. (Verified accepted.)
   - one `bundle`-surface scenario genuinely exercising the bundle checks — e.g.
     a widget target in `index.json` naming an id no file declares, or a
     property spelling used in an object but declared nowhere.

### Summary output

At the end print, to test output: per category, counts of `ok` / `known_gap` /
`broken`; then the **full list of known gaps** with id and one-line prose. That
list is the punch list this exercise exists to produce — make it easy to read
and easy to paste into an issue.

---

## 5. Stage 2 — the fault families

One agent per family. Each reads its spec sections **and the code implementing
the refusals**, writes scenarios, and self-verifies by running the harness.
Every scenario must cite its `source`.

| Family | Derived from |
|---|---|
| `property-spelling` | §3 — slug vs display name, stored key, typo, case, NFC/NFD, edge whitespace, a name that is another entity's stored key |
| `denied-keys` | §2b and §3 deny rules; `omittedrelation.go`, `systemtrim.go` — icon/cover flat spellings, `internal_key`, transient/derived keys |
| `block-structure` | §4, §5, §6.1, §7, §7a — unknown type, indent bounds and jumps, missing required members, table row/column/cell rules, transparent containers |
| `enum-values` | every closed vocabulary: `blockvocab.go`, `viewvocab.go`, `json.go` — block type, align, layout, view type, filter condition, sort direction, date preset, relation format, icon/cover format, card style, embed processor, option color |
| `dataview` | §6.2, `dataview.go`, `filters.go` — filter naming an unknown property, bad condition/format pairing, missing day-count operand, `group_by`, duplicate view ids, columns referencing undeclared properties |
| `type-document` | §2a, §2e — `type_settings` shape, `property_definitions`, `template_for` misuse, `kind` mismatches, root `type_properties` |
| `bundle-coherence` **(bundle surface)** | §2c and the `anyblockbatch` checks — undeclared property spelling, dangling widget/link target, missing manifest file, reserved id, duplicate id, unbound file document, shared select conflict |
| `inline-markup` | §8.1–§8.4 — malformed mention, unbalanced marks, bad escape, bad link destination, unknown tag, astral-plane text |
| `property-values` | §3 — scalar vs list, wrong JSON type per format, malformed RFC 3339 date, unknown option, null, number precision |
| `subset-boundary` | §2g, §4a — export-only members (`store`, `root`, `fields`, `source`, `object_orders`) in an authoring document |

Guidance for family agents:

- Prefer **plausible** faults over exotic ones. The target is what an LLM
  actually emits: a slug where a name belongs, a near-miss enum, a scalar where
  a list belongs, a member at the wrong nesting level, a stored key copied from
  API docs. Random byte corruption is a different job (see §7).
- A scenario whose fault is currently **accepted** is not a failure to report —
  write it as `known_gap`. Those are the most valuable entries in the catalogue.
- Every `Repair` must be a genuine inverse: applying it must return the document
  to validity, not merely delete the offending member if deletion loses the
  author's intent.

---

## 6. Stages 3 and 4

**Stage 3 — adversarial verification.** A scenario can be wrong in ways its
author will not see: the mutation does not produce the fault the prose claims;
the repair is not a true inverse; `expect_path` is off by a level; the assertion
passes for the wrong reason (e.g. matching the preamble issue instead of the
real one). Have a *different* agent re-run each family's scenarios and classify
them. Treat a scenario that passes for the wrong reason as broken.

**Stage 4 — completeness critic.** Enumerate every refusal the code actually
implements — `addIssue` call sites in `validate.go`, the closed `enum`s in
`schema/*.json`, and the `anyblockbatch` check functions — and report which have
**no scenario**. The uncovered remainder is itself a finding, and it is how we
know the catalogue is honest rather than merely large.

---

## 7. Explicitly out of scope for this phase

Deferred deliberately by the project owner, to a later stage:

- **Round-trip / conversion testing** — mutating the protobuf corpus, the §11
  byte fixpoint, and `snapshotdiff.Compare` snapshot-equivalence. That is the
  *export* direction and belongs to phase 2.
- **Coverage-guided byte fuzzing** for crash/hang/OOM safety. Valuable, but a
  different oracle; random bytes die at the JSON parse layer and teach nothing
  about error quality. A separate research document,
  `pkg/lib/anyblockjson/HANDOFF_FUZZING_RESEARCH.md`, covers this in depth if it is wanted.

---

## 8. Verify before reporting

```
go build ./...
go vet ./cmd/...
go test ./cmd/internal/anyblockbatch/...
```

Report the verbatim output including the summary table. Do not claim green
without running it.

If a seed scenario does not behave as §4 describes, **say so plainly and show
what actually happened** — the probe behind this document may have been wrong,
and a correction is more useful than a workaround.

---

## 9. Two findings likely to dominate the punch list

Worth knowing up front, because they will recur across families:

1. **"Did you mean" is absent everywhere.** Edit-distance suggestion against the
   closed vocabularies is a small, contained change with an outsized effect on
   whether an agent recovers unaided.
2. **The two error layers are inconsistent.** Curated §12 wording is genuinely
   good; raw schema refusals are bare (`property "colour" is not allowed`). The
   catalogue will show exactly which faults fall through to the bare layer —
   that is the list of places to add curated wording before the freeze.
