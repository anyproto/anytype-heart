# AnyBlock JSON — pre-freeze review (SPEC v0.6, format version 1)

Three independent adversarial reviews of the format at `a6218db49`, each with a
distinct lens, each running probes against the package rather than reading only:

- **Evolution & migration** — what breaks when the format changes (the future).
- **Round-trip fidelity** — what silently changes or disappears (existing data).
- **Public contract & hostile input** — what a third party or an LLM does with
  the spec, and what a malformed document does to the reader.

This document is the synthesis: cross-lens agreements first (independently
found twice or three times, so highest confidence), then the ranked action
list, then what was verified sound so it does not get re-litigated.

Findings marked **[confirmed]** were reproduced by execution against the
package's exported API. Findings marked *[code]* come from reading.

---

## 0. The one recalibration the individual reviews got wrong

Two lenses treated `import.go` as a live untrusted-input surface, on the
strength of the bundle path (index.json creates spaces, installs widgets and
types, unpacks files). Today it is not: `anyblockjson` is wired only into
developer CLIs (`cmd/anyblockconvert`, `cmd/anyblockbatch`,
`cmd/anyblockrecover`). `anyblockconvert` turns documents into old-format pb
snapshots offline, which are then zipped as builtinobjects archives and fed to
the existing pb importer. Bundle authors are currently the team.

This does not retire the hostile-input findings — it dates them. They are
**prerequisites for accepting documents from anyone outside the team**, which
is the format's stated purpose ("written by external tools and LLM agents"),
not live exploits. The severity ranking below reflects that: correctness bugs
that hurt the current offline flow rank above attack surface that activates
when the format is wired to a user-facing import.

---

## 1. Cross-lens agreements

These were found independently by two or three lenses, from different angles.
They are the load-bearing conclusions.

### 1.1 `Marshal` emits documents its own `Validate` rejects — six instances, one root cause

The invariant is stated in the spec (§11) and restated in the code
(`table.go:461`: *"Emitting one verbatim would make Marshal write a document
its own Validate rejects, so normalize it once here"*). The sanitize-and-
disambiguate discipline that comment describes was applied **only to table
inner ids**. Every other id surface skipped it:

| # | Construction | Result | Lens |
|---|---|---|---|
| a | block id outside `[A-Za-z0-9_-]{1,64}` (`a.b`, `dir/file`, `блок`, 65 chars) | `Marshal` OK, `Validate` fails; `export.go:486` writes `e.localId(b.Id)` verbatim | 2, 3 |
| b | paragraph id equal to a derived cell id (`r1-c1`) with a table whose row `r1` omits cell `c1` | duplicate id | 2 |
| c | stored column id `c-1` sanitizes to `c_1`, colliding with a sibling paragraph `c_1` (`tableIdsUsed` covers only table inner ids) | duplicate id | 2 |
| d | blocks `block_12345` + `12345` under `CompactIds` — local-label disallow predicate is charset-only (`export.go:869`), unlike the refs path at `:864` which also checks `fullIds` | duplicate id | 2 |
| e | authored table row id `dataview` + a dataview block — `pinPrimaryDataview` (`import.go:335-351`) scans only top-level ids, so it mints a collision *after* validation; re-export then drops the entire table body via a failed type assertion | silent data loss | 3 |
| f | `Options.GenerateId` mints an id an author already used — `genId()` (`import.go:113-118`) never checks authored ids, unlike `claimTableInnerId` (`table.go:391-430`) which does | duplicate id | 3 |

All **[confirmed]**. Consequences: (a)–(d) produce an export that fails at
import — an unrestorable archive discovered late; (e) loses a table body
silently; (f) is directly reachable from `anyblockconvert`, whose generated ids
are `<sanitized-file-path>-<n>` with both halves author-controlled.

This is the same class as FLAT_REVIEW #3 (indent > 32), which *was* fixed. The
fix was applied to the instance, not to the rule.

**Fix:** one document-wide id set seeded from validation (authored block ids,
derived cell ids, table inner ids), consulted by the export sanitizer, `genId`,
`pinPrimaryDataview`, and the compact-id labeller. Then the property test
FLAT_REVIEW §D.3 already asks for — `Validate(Marshal(S)) == nil` for every `S`
that `Marshal` accepts — driven by a hostile snapshot generator, not by
`richSnapshot()`.

### 1.2 `Validate` accepts what `Unmarshal` rejects — two failure modes, same missing rule

FLAT_REVIEW #2 fixed this for `indent` and wrote the rule down (`import.go:45`:
*"Unmarshal must accept everything Validate does"*). The rule was not
generalised. **[confirmed]** on both sides:

- **Integer-valued floats**: `{"size": 2048.0}`, `{"limit": 1e1}`,
  `{"pageSize": 50.0}` — schema says integer, Go says `int64`/`int32`, decode
  fails.
- **Overflow**: `{"limit": 1e20}`, `{"size": 1e30}`, `{"pageSize": 1e19}` —
  schema has no bounds, decode fails.

Both surface as a bare `decode document: %w` with no JSON pointer, i.e. outside
the path-addressed error contract §13 promises. Adjacent: `viewColumn.width` is
float64 → `int32` (`dataview.go:426`), silently truncating `120.7` and
implementation-defined out of range.

**Fix:** `json.Number` those fields, add `minimum`/`maximum` in the schema,
range-check width, and add the corpus test `Validate(d)==nil ⇒ Unmarshal(d)`
has no decode error.

### 1.3 `index.json` does not honour the version contract §10 writes for it

`checkVersion` is called from exactly one place — `validate.go:143`, the
document path. `UnmarshalIndex` (`index.go:184-222`) never calls it; the index
schema pins `"version": {"const": 1}`, so `{"version": 2}` yields
**[confirmed]** `/version: value must be 1` — a generic schema failure with
neither version named and no `NewerFormat` flag, which is precisely what §10
forbids. The existing test (`index_test.go:133`) asserts only that *an* error
occurs, so the drift is test-blessed.

Compounding, from the evolution lens: the version is per-document *and*
per-bundle, and nothing defines what an importer does when they disagree
(index 1, documents 2, or a mixed bundle).

### 1.4 The loose surfaces invert the format's strictness

Unknown-input behaviour is bimodal and the spec never says so. **Strict**
(schema-enumerated fields): hard-reject the whole document. **Loose**
(`properties`, block `fields`, `store`, `root`, filter `value`): accept
anything and store it verbatim.

- *Contract lens, **[confirmed]***: import copies every supplied property key
  onto details, skipping only `id`/`type` (`import.go:180-187`). `isArchived`,
  `isDeleted`, `creator`, `spaceId`, `restrictions`, the empty key, and a key
  containing a newline all land. Export strips
  `bundle.LocalAndDerivedRelationKeys` — so the import surface is strictly
  wider than the export surface, the exact asymmetry §4a claims does not exist
  ("treats supplied values as authoritative only where semantically safe" —
  nothing implements that clause). Values are never checked against the
  property's resolved format either: `"dueDate": "next Friday"` stores a string
  on a date relation, re-exports unchanged, and reads as 0 forever.
- *Evolution lens*: the same looseness means a **future** value encoding
  (§15.3's `{id, name}` option objects, a name sidecar for object refs) is
  *accepted* by a v1.0 reader and stored as a literal `Struct` detail — silent
  corruption, where the strict surfaces would at least have rejected.

So "what does a reader do with data it does not understand" has two opposite
answers depending on which key the data hides under, and the dangerous answer
covers the surface carrying the most user data.

The forward-looking half of this (verified against
`core/block/import/common/objectid/existingobject.go:52-101`): that file
resolves **which existing object a snapshot merges into** using
`oldAnytypeID`, `uniqueKey`, `sourceFilePath` and — for types — `name`. All
four are ordinary detail keys, so once anyblockjson documents arrive from
outside the team, a document can aim itself at an existing object in the
victim's space. `key` is likewise unconstrained (`{"type": "string"}`), and
`NewUniqueKey(ObjectType, "page")` = `ot-page`, the bundled type's derived id.

**Fix:** a deny-list in `validate.go` (local/derived keys minus the six §3
exemptions; `uniqueKey`/`oldAnytypeID`/`sourceFilePath` never), a
`propertyNames` pattern in the schema, a charset on `key`, format-shape
validation with path-addressed errors — and a §10 sentence declaring the loose
surfaces contractually frozen at the §3 shapes, so a future encoding change is
a major-version event rather than silent corruption on deployed readers.

### 1.5 The test corpus cannot detect the classes of bug found here

Three lenses, three angles on the same gap:

- **The 99.86% measures much less than it sounds like.** `lossIssues` in
  `cmd/anyblockroundtrip/main.go:533-565` compares exactly two things: detail
  values, and a multiset of plain-text strings of text blocks — *skipping
  Title/Description styles*. It never compares mark ranges, block order or
  nesting, table shape, any dataview content, file-block metadata, or bookmark
  fields. Byte-stability is self-consistency of the JSON pipeline, so a
  systematic mark drop is byte-stable and invisible. Two confirmed
  silent-corruption paths below would sail through this at 100%. And per
  ANOMALIES' last line the v0.6 flat-encoding rerun is still **pending** —
  99.86% is a pre-flat number inherited from run 3.
- **`testdata/` is four export goldens generated from one Go fixture**
  (`richSnapshot()` via `golden_gen_test.go`). There is no input-side corpus —
  no must-accept, must-reject, or must-parse-to-these-marks cases — and nothing
  promises the goldens are stable, so a third party has no conformance suite.
- **No harness enforces the additivity promise.** §10 says documents citing an
  older `$schema` stay valid; nothing validates a frozen corpus against the
  next schema, so an accidentally non-additive schema edit ships silently.

### 1.6 §8 needs to become normative — for two independent reasons

- *Contract lens*: §8 is prose, not a grammar. The normative definition is
  `inline.go:1645-1674` plus the flanking rule at `:1071`. **[confirmed]**
  divergences no reader could predict from the spec: `*a**b*` imports as text
  `"ab"` — **two characters deleted** — where CommonMark gives `<em>a**b</em>`;
  `****a****` gives Bold only; `[[a](b)](c)` produces two coincident Link marks
  over one range, the shape §8.3 step 3 declares impossible. §8.3 claims the
  grammar "stays syntax-compatible with CommonMark for well-formed input";
  `*a**b*` is well-formed CommonMark and loses characters. §1 invites third
  parties to render the string with a Markdown library, which will show
  different content than Anytype.
- *Evolution lens*: the grammar reserves **no syntax space**. Escaping of `<`
  is whitelist-anchored to `u`/`font`/`mention` (`inline.go:722-734`), so
  literal user text `<sub>x</sub>`, `==mark==` and `~one~` all export
  **[confirmed]** unescaped. The day a reader whitelists `sub`, those bytes
  become a mark — and the reader cannot tell 1.0-literal from 1.1-markup,
  because text strings carry no version. Worse, once a tag is whitelisted,
  malformed instances become validation errors (§8.3), so old *valid*
  documents become invalid: a mark addition is non-additive from inside the
  text strings, where the version integer cannot reach.

Both point at the same work item, and the escaping half **must land before
freeze because it changes canonical bytes**.

---

## 2. Ranked action list

### Tier 1 — fix before freeze (correctness, localized, cheap)

1. **Unify the id domain** (§1.1) — one document-wide id set + the
   `Validate(Marshal(S))` property test with hostile generators. Six confirmed
   instances, two of which lose data.
2. **Reserve the inline-grammar syntax space** (§1.6) — escape `<` before any
   `</?[A-Za-z]`, and define unknown-tag leniency (a tag-shaped sequence a
   reader does not know stays literal with a warning, never an error).
   Byte-changing, so pre-freeze or never.
3. **`Validate`/`Unmarshal` agreement** (§1.2) — `json.Number` + schema bounds
   + the corpus test.
4. **Date out of RFC 3339 range silently flips number → string**
   **[confirmed]**: `customDate = 1751791445000` (ms stored where s belong, a
   real corruption class) exports as `"57482-01-22T22:43:20Z"`, which
   `parseDate` cannot read back, so the detail becomes a *string* on a
   date-format property — permanent and quiet, and byte-stable thereafter.
   Emit the raw number when outside representable range, or fail loudly.
5. **Property-key admission control + format-shape validation** (§1.4) — the
   deny-list, the key charset, the `key`/`type` charsets, and path-addressed
   format errors.
6. **Fix the validation error cascade** — **[confirmed]** an LLM writing
   `"type": "bulleted_list_item"` is told `/blocks/0/type: property "type" is
   not allowed` **and** `text is not allowed`; a wrong `checked` type produces
   the real error plus three noise issues; a table cell with an `id` yields six
   issues, four noise. Cause: `allOf` + `unevaluatedProperties:false`
   (`object.schema.json:54-70`) discards `properties` annotations when any
   subschema fails, and `validate.go:237-250` rewrites the result into a
   *confident* "not allowed". §12 promises exactly the opposite. An agent that
   follows this feedback deletes `type`. Since the agent-retry loop is the
   format's reason to exist, this is a Tier 1 item, not polish.

### Tier 2 — decide and document before freeze (policy; some byte-changing)

7. **Write §10's missing migration half**: which versions a reader must accept
   (recommend: all ≤ its own, forever), who migrates (recommend: importer-side
   per-version decode, keeping the frozen v1 schema embedded alongside future
   ones and dispatching on `version`), and what a v1→v2 migrator looks like.
   Today `checkVersion` accepts a *range* while everything downstream is
   single-version — one embedded schema, no dispatch, no `Migrate` in §13 — so
   the first bump lets v1 documents through the gate and then fails them in the
   body with generic errors. Resolve §15.9's status while here: it plans to
   change which property values export writes, i.e. canonical bytes and
   presence semantics, described as a "refinement" with no version discussion.
8. **Declare the loose surfaces frozen** at the §3 shapes (§1.4) — cheaper and
   more honest than tightening `propertyMap`.
9. **State the real forward-compatibility posture.** The schema closes every
   surface (`additionalProperties:false` everywhere, exhaustive enums), so any
   1.x addition is a whole-document rejection on every not-yet-updated device,
   and there is no unknown-field preservation anywhere. §10 simultaneously
   tells third parties to skip-and-preserve unknown types *and* to validate
   against the strict published schema — you cannot do both. Either implement a
   lenient/best-effort import mode (reserve its semantics in §10 now even if it
   ships later) or drop the ADF/Portable-Text additive claims: a versioning
   promise the code does not keep is worse than none.
10. **Enum tripwires + declared unknown-enum policy.** **[confirmed]** export
    flips out-of-range proto enums to defaults: view `Type=99` → no `type` →
    round-trips as `Table`; **filter `Condition=99` → no `condition` → the
    filter silently stops filtering**; embed `Processor=99` → `latex`. Yet
    unknown *content* types error loudly (`export.go:596`). The policy is loud
    where evolution is rare and silent where it is frequent. Bind every
    `enumNames` table to its proto descriptor in a test so a proto addition
    fails this package's CI, then choose per table: error for behaviour-bearing
    enums (conditions), degrade-with-warning for cosmetic ones.
11. **Newer-version signalling**: a `SchemaMinor` constant driving both
    `SchemaURL` and the `citesNewerSchema` comparison (today the reader's minor
    is hardcoded 0 at `validate.go:213`, so 1.1 will misreport ordinary
    authoring errors as "produced by a newer version"); `checkVersion` in
    `UnmarshalIndex`; a bundle version-skew rule (§1.3).

### Tier 3 — before publishing as a third-party contract

12. **Make §8 normative** (§1.6) — token grammar plus the delimiter-stack
    algorithm in pseudocode, a normative parse-table fixture, an explicit
    ruling on the CommonMark divergence, and a ban on the two-coincident-links
    shape. Or state plainly that `inline.go` is normative and retire the
    "writable by someone who has never seen Anytype internals" claim for
    marked-up text.
13. **Ship an input-side conformance corpus** (`accept/`, `reject/`, `parse/`)
    with a stability promise and a README (§1.5), plus a frozen 1.0 corpus that
    every future schema release must still validate.
14. **Close the spec gaps a third party cannot work around**: key order for
    free-form objects (`fields`, `store`, `root` — sorted alphabetically in
    `json.go:576`, unspecified in §4, so canonical bytes are unreproducible);
    the `kind` enum (spec gives "page, profilePage, …", schema has 32); three
    `required` constraints the spec never states (`sort.property`,
    `viewColumn.property`, `dataviewProperty.key`); the escaping algorithm
    (§8.2 never mentions that `escapeAttr` numeric-entity-encodes `[`, `]` and
    backtick, and does not define `\q` or a trailing `\`); and correct §4a's
    false claim that output-only fields are `x-output-only`-annotated —
    `coverId`/`coverType` and the six preserved internal properties live inside
    the free-form `propertyMap` and cannot be.
15. **Input bounds.** **[confirmed]** measured: 800 KB of `[a](` repeats →
    `Unmarshal` 4.3 s / 3.3 GB allocated; 4 MB → 20.9 s / 16.5 GB. Cause:
    `scanBareDest` buffers up to 2048 runes per `[` and discards on failure
    (`inline.go:1319-1430`); `Validate` pays it too, so it is not the cheap
    gate §13 implies, and `Unmarshal` then parses every text a second time.
    Nothing bounds document size, `text` length, `blocks` length,
    `rows`×`columns`, or `refs`. Bound them in `Options` with documented
    defaults and put the bounds in §12 — otherwise a rejecting and an accepting
    implementation are both conformant.
16. **`refs` legend can rebind a literal full id.** **[confirmed]**
    `refs: {"bafyrei<victim>": "bafyrei<attacker>"}` silently re-points a link
    and a mention that name the victim id inline. §9a states the rule and puts
    the burden entirely on the writer; the reader enforces nothing
    (`import.go:122-130` is an unconditional lookup, keys allowed 64 chars —
    every CID fits). Make a colliding key a validation error, or cap refs keys
    at the 16 characters export actually uses.
17. **`_filter_template_*` tokens get compacted into the refs legend**
    **[confirmed]** — a direct §6.2 violation ("export must not compact them
    into the refs legend"). `buildCompactIds` (`export.go:797-804`) lacks the
    `isFilterTemplate` guard that already exists at `json.go:485`. One line.
18. **Array-form table cells: `Validate` and `Unmarshal` disagree on the parent
    relation** **[confirmed]** — three indent-0 paragraphs in one cell validate
    fine, then import as `a` with children `[b, c]`, because `validate.go:498`
    seeds `prev = -1` while `table.go:368` seeds the stack with the cell block.
    An LLM writing a two-paragraph cell hits this. Require `indent ≥ 1` after
    the first element and say so in §6.1.

### Tier 4 — decisions, not necessarily code

- **Keyboard (inline-code) marks lose boundary whitespace** **[confirmed]**
  (`" x "` → Keyboard[1,2)): §8.3 groups Keyboard with emphasis under
  "whitespace at a boundary carries no visible styling", which is false for
  inline code — it renders with a background in every client. Accept with an
  honest rationale, or keep interior boundary spaces.
- **Adjacent same-target mentions merge into one chip** **[confirmed]** — §11
  files this under "same styled rendering", but for a Mention the range *is*
  the chip, so two chips become one double-named chip. Reachable via concurrent
  CRDT merges. Consider exempting Mention/Object from adjacency merge.
- **Surrogate-splitting marks are dropped whole rather than clamped**
  **[confirmed]** (`"😀bold"` with Bold[1,3) exports with no bold at all).
  Spec'd in §8.3 step 1, but drop-vs-clamp deserves an explicit decision — the
  plausible source is an old rune-offset client, and clamping preserves intent.
- **Title/description block text diverging from the `name` detail is silently
  destroyed** **[confirmed]** — §7's premise is an editor invariant, not a
  snapshot invariant, and `textInventory` *skips* these styles, so the 35k-object
  sweep is structurally blind to the class. Cheap export-side check: if they
  differ, emit the block or warn.
- **Legacy bookmark preview data dropped with no surviving copy** — §5 says
  "preview data lives on the target object", false when `targetObjectId` is
  empty (pre-objectization blocks in unopened/archived spaces). Preserve, fail
  loud, or document with measured volume.
- **Silent whole-subtree drops** — orphan blocks unreachable from root, and
  `GroupOrders`/`ObjectOrders` for deleted views (routine editor residue), both
  vanish with no diagnostic. The `OnWarning` sink already exists in `Options`.
- **Bidi/control characters pass through unescaped both ways** **[confirmed]**
  (`"safe‮gnp.exe"` survives and is re-emitted raw; `json.go:127-154`
  escapes only `<0x20`, U+2028, U+2029). Cheapest spoofing primitive available
  for a format whose premise is sharing bundles.
- **Stray `TableRow`/`TableColumn` outside a table fails the whole object's
  export** **[confirmed]** — loud beats silent, but one corrupt legacy block
  makes an object unexportable in a backup pipeline. Consider the `group`-style
  fallback.
- **Duplicate JSON keys**: last-wins in both `Validate` and `Unmarshal` (so no
  divergence), but an agent-emitted duplicate loses content undetectably. A
  token-level pre-scan is cheap and worth it for the generation use case.
- **`PropertyResolver.PropertyId` can create properties on the export path**
  (`typeproperties.go:172-188`) — split lookup from create, or document that
  export-side resolvers must be read-only.
- **`<font color>` accepts arbitrary strings** while option colors are
  enum-checked — same reasoning as §2a, opposite outcome.
- **`store` and `items` both present**: `items` silently wins
  (`import.go:235-252`), while the analogous `typeProperties` conflict *is* an
  error.

---

## 3. Verified sound — do not re-litigate

Independent adversarial work found the core codec solid, which is worth
recording because the finding list above is long:

- **The §8 delimiter-stack renderer/parser pair is sound.** A 60 000-case
  independent *fidelity* fuzz (coverage-equality against an externally computed
  `N(S)`, over adversarial text: delimiters, tag prefixes, entities, trailing
  backslashes, ZWJ, astral planes, combining characters, hostile link/mention
  params with quotes, brackets, newlines) found **zero failures**. A separate
  200 000-case *stability* fuzz through `Export(Import(·))` twice also found
  **zero failures**. Escaping is context-minimal yet safe in every attacked
  case, including `</mention>` inside a mention label and code-span padding.
  The §8 problems above are about the *spec* and *reserved syntax space*, not
  the implementation.
- Absent-vs-empty canon is correctly asymmetric in both directions (verbatim
  property values including `null`/`""`/`[]`/`false`, omit-empty elsewhere).
- Number properties round-trip float64 exactly; int64 `size` round-trips
  exactly.
- The `propname` collision class does not exist — properties key off raw stored
  keys with no sanitization.
- Schema correctly bans `id`/`indent` on bare cells and `table` inside cells;
  kind/format/condition enums are closed.
- Unbalanced `**` correctly demotes to literal text; a scalar where a list is
  expected (`"status": "In progress"`) is correctly wrapped.
- The flat-blocks encoding itself, the id-optional posture, and the
  option-names-not-ids call all reviewed as right. No lens argued against a
  design decision; the exposure is in reader strictness, spec precision, and
  the absence of an evolution policy.

---

## 4. Verdict

Not ready to freeze as v1, but nothing found requires a redesign — every item
is fixable inside this branch, and the fixes are mostly small and local.

The pattern across all three lenses is the same: **rules were discovered,
written down in a comment or a review note, and then applied to the single
instance that prompted them instead of to the class.** `tableInnerId`
sanitizes ids; block ids do not. `indent` became `json.Number` so Validate and
Unmarshal agree; `size`/`limit`/`pageSize` did not. `claimTableInnerId` checks
authored ids; `genId` does not. Layout names are format-validated because "a
typo would otherwise import as a raw string onto a number-format property";
every other property format is not. The highest-leverage change is not any
single fix but the two property tests that turn these into classes:
`Validate(Marshal(S)) == nil` for all `S`, and `Validate(d) == nil ⇒
Unmarshal(d)` succeeds — both driven by hostile generators rather than
`richSnapshot()`.

The second theme is that the format has no evolution story yet. It currently
has three different behaviours for data a reader does not understand — hard
reject (strict surfaces), silent corruption (loose surfaces), silent
degradation (proto enums) — and no policy, no migrator, no owner, and no
reserved syntax space in the one place (inside text strings) the version
integer cannot reach. That last item is the only one on this list that becomes
*impossible* to fix after freeze, because it changes canonical bytes.

Finally: re-run the acceptance sweep on v0.6 before shipping, and either extend
the comparator to marks, structure, dataviews and title-block text or restate
the claim as what it measures — "canonical stability, details, and text
preserved". The residual 0.14% is benign, but the metric cannot see the
corruption classes found here, two of which would score 100%.
