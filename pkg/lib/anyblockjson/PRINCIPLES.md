# AnyBlock JSON — design principles

Status: living document · applies to format version 1 (SPEC v0.10) ·
Package: `pkg/lib/anyblockjson`

`SPEC.md` says what the format *is*. This document says what it is *for*
and the rules every part of it answers to — the rules a change to the
format, to this package, or to the API v2 document surface must either
serve or knowingly bend. `PRINCIPLES_SHORT.md` is the one-screen version
of this document; `OVERVIEW.md` walks through the individual decisions;
`ANOMALIES.md` records what real data did to them.

---

## What AnyBlock JSON is

One JSON document per Anytype object: an envelope (`version`, `id`, `type`,
`kind` when not derivable), a `properties` map of key → value, and a flat
`blocks` array whose nesting is an integer `indent`. Rich text is a Markdown
subset inside `text`. Types are documents too (`kind: "object_type"` with
`type_properties`); a bundle of documents adds an `index.json`.

It replaces `.pb.json` as the export/import format, it is the document shape
API v2 serves and accepts, and it is what an agent reads and writes when it
edits an object. The same bytes serve every door.

A document as an agent might write it — no block ids (minted on import), a
reference by a label it chose with the legend in the document, formatting
inline:

```json
{
  "version": 1,
  "type": "task",
  "properties": {
    "name": "Ship the export",
    "icon_emoji": "🚢",
    "status": ["In progress"],
    "due_date": "2026-09-30T00:00:00Z"
  },
  "refs": { "roman": "bafyreidfmzjh…" },
  "blocks": [
    { "type": "heading_2", "text": "Goals" },
    { "type": "paragraph",
      "text": "Lossless **and** readable, with <mention object_id=\"roman\">Roman</mention>." },
    { "type": "checkbox", "checked": true, "text": "Draft the spec" },
    { "indent": 1, "type": "bulleted_list_item", "text": "Validate in CI" }
  ]
}
```

---

## The rules

### 1. Lossless for meaning

**What the user expressed survives a round trip. What the system bookkeeps
may be normalized. Every accepted loss is written down.**

The contract is a fixed point, not byte-equality: `Import(Export(S)) ≡
N(S)`, and `Export ∘ Import` is idempotent and byte-stable (SPEC §11). `N`
is the written-down normalization — structural blocks the editor
regenerates, restrictions it rebuilds, ids, offsets, cached formats, UI
state. None of that is meaning. Text, marks, properties, views, column
widths and option vocabularies are, and they round-trip.

Two consequences, both decided against the format's first instinct:
**presence is meaningful** — a property set to `false`, `0`, `""`, `[]` or
`null` is a fact the user created and is written verbatim; the omit-default
canon applies to block attributes and envelope fields only (§3) — and
**escape hatches over drops** — data with no first-class shape rides along
in `fields`, `root` and `store` (§2, §4a). The accepted losses are few and
listed, never smoothed over: emoji marks materialize into text (§8.1),
same-named options of one property collapse (§3), block-label compaction is
lossy and opt-in (§9a). The contract is proven, not believed: a
35,369-object production sweep (`cmd/anyblockroundtrip`) round-tripped
99.86% byte-identically and every remaining case is in `ANOMALIES.md`.

### 2. Readable by a stranger

**A person who has never seen Anytype internals can read a document,
understand it, and hand-edit it.**

Structure is visible as a flat list with an `indent`; formatting is the
Markdown the reader already knows; dates are RFC 3339; layouts, enum values
and block types are names, never numbers (§3, §5); keys come in a fixed,
meaningful order with `text` last (§4). Machinery that exists only for the
editor is hidden: a table is `columns` and `rows` of `cells`, not a subtree
of wrapper blocks with composite ids (§6.1); title, description and icon are
properties, not blocks (§7).

The test: if understanding a field needs the codebase, the field is wrong.
A stranger has the editor for reading a page; the document is for the
moments when they don't — a diff, a backup, a git repository, a review of
what an agent just wrote.

### 3. Borrow words, don't coin them

**Every name answers to something the reader already knows. Internal names
never appear.**

Precedence when naming anything: the term Anytype's public API already
uses; then HTML, CommonMark, SQL and the vocabulary common block-editor
APIs share; a new word only when none of those has one. The format owns exactly six Anytype concepts — object, property,
type, option, set/collection, space (§1) — and everything it defines is
`snake_case`, digits included, stated as a rule so a name added later needs
no decision (§1 *Naming*). So `relation` → `property`, `smartBlockType` →
`kind`, `header_1` → `heading_1`, `status`/`tag`/`longtext` →
`select`/`multi_select`/`text`. "Relation" appears nowhere in the format.

Borrow only where the meaning matches. `dataview` stayed `dataview` rather
than becoming `database`, because a dataview references objects it does not
own (§6.2): a familiar word that lies costs more than a new one.

### 4. Nothing to guess

**A valid document needs only what is in the author's head and in one
example. No offsets, no ids to fetch first, no bookkeeping — and that
includes small models.**

The writer is often a language model, and the bar is set at the small end.
The question for any shape is *would a 3–7B model under a grammar emit this
correctly?* If not, the shape is wrong, not the model. That question settled
most of the format's arguments, usually against what a human-only format
would have chosen:

- Inline formatting is Markdown inside `text`, not offset ranges (§8):
  models cannot keep offset bookkeeping; Markdown puts the formatting where
  the formatting is.
- Blocks are a flat pre-order array with an integer `indent`, not a nested
  tree (§4): a recursive schema cannot be used under constrained decoding —
  the mechanism that took a 7B model from 0% to 75% valid emission in the
  prior-art review — and a truncated flat array is still a valid prefix.
- Ids are optional and minted on import (§9); numbering, structural blocks
  and restrictions are derived, never written (§5, §7).
- The schema is closed and exhaustive (`additionalProperties: false`,
  enumerated values, discriminator-first), so there is nothing to invent
  (§12); at the API door every endpoint serves its schema and one worked
  example (API v2 convention C12).
- Errors are repair instructions — path-addressed, naming allowed values,
  one fault → one issue — so generate → validate → feed back converges
  instead of drifting (§12).

What a model cannot be shown an example of, it will hallucinate. *Names,
not ids* (rule 6) is this rule applied to references.

### 5. Token-efficient, not terse

**Spend no token that carries no meaning; never save one by making the
reader decode.**

The free savings are taken: defaults and empties are omitted so the common
case costs nothing (§4); object references compact to short labels with a
`refs` legend — a 59-character CID costs more tokens than the sentence
around it (§9a); the API sends compact JSON and minimal rows. *Names, not
ids* is a token rule too: the expensive unit is a round trip, not a byte,
and a name the author already has saves a fetch.

The limit is decoding. The format stays per-block JSON objects with
readable keys because every terser encoding tried cost more than it saved —
a tabular encoding produced 44.6% valid one-shot generations against JSON's
75%, and a 2026 study of agent-facing conventions found aggressive
compression raised total session cost 67% while cutting input tokens 17%.
Token efficiency means semantic density: every token carries meaning the
reader uses, nothing is repeated that a legend can carry once, and nothing
needs a lookup or a reasoning step to mean something.

### 6. Names, not ids

**Wherever a human would write a name, the format carries the name.**

Select options are names — in values, filter values and custom orders alike
(§3, §6.2). Properties are addressed by slug key, types by type slug,
layouts and block types by name. Only objects keep ids, because nothing
else about an object is unique — and even those shorten to labels an agent
can choose (`"roman": "bafyrei…"`) with the legend in the document (§9a).

An id is unguessable: a model must fetch before it can write, or it invents
one — the hallucination surface in its purest form. A name is already in the
request. Import creates missing options by name, as the CSV importer and
the public API already do. The cost is accepted and listed:
same-named options collapse; renaming an option breaks the link on
reimport (§3).

### 7. A document stands alone

**One exported object is understandable and re-importable without the space
it came from.**

Every compaction carries its own inverse in the document: `refs` for object
ids, `property_keys` for a space's own property slugs (§3, §9a) — a slug
that reads back as a *different* property in a reader that cannot ask the
space is how twelve objects were silently re-pointed in a sweep before the
legend existed. A type document carries its property definitions with their
option vocabularies, colors and target types, in one file (§2a); a bundle
carries `index.json` and is versioned as one artifact (§2c).

The intention goes further than the format currently does. The structural
facts a reader needs about an object — which format a custom property has,
the vocabulary of a select, the display name behind a key — should travel
with the document, so that a single-object export is a complete artifact
for a reader with no space and no resolver. Today formats resolve through
caller-wired resolvers (§3 *Format resolution*) and a names sidecar is
listed as a future extension (§1 *Non-goals*). That gap is tracked, not
accepted.

### 8. Strict in, canonical out

**Export has one spelling of every document. Import validates strictly,
addresses every error to a path, and never guesses.**

Canonical output — fixed key order, omitted defaults, minimal escaping (§4)
— is what makes diffs meaningful and `Export ∘ Import` a fixed point.
Import accepts liberal forms only where listed and canonicalizes them
(`heading_4`, `equation`, `_x_`, HTML entities, date-only strings); an
unknown block type, an unknown tag attribute, a `children` key, an
over-deep indent are errors, not silent drops (§4, §8, §12). Lenient modes
exist, are opt-in, and report every clamp with its path.

Never guess: the `anytype://object` deep link is matched by exact form; a
tag-shaped sequence the version does not define is literal text plus a
warning; `refs` resolution is total, with no "short-looking id" heuristic
(§8.1, §9a, §10). `Validate` and `Unmarshal` accept and reject the same
documents, and `Marshal` never emits what `Validate` rejects (§11, §12) —
the promises that make "this document imports" a statement rather than a
hope. And a check has to earn its place: it catches something silent *and*
traces to a mechanism, or it does not ship — every marginal warning makes
the ones that matter cheaper to ignore (§12).

### 9. One shape, every door

**Export, import, bundles, API v2, templates and prompt examples all speak
the same document. There are no dialects.**

A file export, an `index.json` bundle, a `GET /v2/…/objects/{id}` body, a
`POST` that creates an object, the worked example in an API schema — same
envelope, same blocks, same vocabulary, same validator. `OmitIds`,
`CompactObjectRefs` and `CompactBlockLabels` are serializations of one
format chosen per consumer, not formats; the canonical full-id form is the
round-trip form (§9, §9a). Anything learned from reading an export applies
to the API, and the other way round.

This is why the format, not the API, owns block ids (a flat array with
optional stable ids is exactly what id-addressed edit operations need), the
filter tree (the compact filter string parses to it; §6.2.1), and the
validation errors the API returns. Markdown remains the lossy human format:
served read-only with warnings, never an editing channel. The package is
pipeline-agnostic — it depends on the model and the bundle, never on the
import or export pipelines (§13) — because the format is upstream of both.

### 10. Evolution is explicit

**One version integer. A reader refuses what it does not know, migrates
what it does, and every change is a bump.**

`version` is the sole authority on format identity (§10). A reader rejects
a newer document with a dedicated error naming both versions, accepts an
older one by migrating it forward, and every format change bumps — a closed
schema has nothing additive to offer an older reader anyway. Inside `text`,
where no version marker can live, canonical output escapes every tag-shaped
`<` so the whole tag space stays reserved for later versions, and the
Markdown delimiter set is closed: a future mark is a tag, never new
punctuation (§8.2).

This is the opposite of HTML's *degrade gracefully*, on purpose: an
interchange format with one reader per version and no partial semantics is
better off refusing than silently half-reading. It would be the wrong trade
for a wire format — one reason AnyBlock is not one.

---

## When rules collide

Decide in this order:

1. the user's meaning survives (rule 1);
2. a model — small ones included — can write it (rules 4, 6);
3. a stranger can read it (rules 2, 3);
4. it costs few tokens (rule 5);
5. it is convenient to implement, or faithful to an internal name.

The order is not a wish; it is what the format's recorded decisions already
did: *presence is meaningful* (1 over 4 — 14,032 omitted defaults put
back); *flat blocks* (2 over 3 — a nested tree reads a little better to a
person, but a model cannot be constrained to it; humans have the editor,
models only have the bytes); *option names* (2 over the strictest reading
of 1 — the collapse is listed, because the alternative is an id no one can
write); *`dataview`, not `database`* (understandable over familiar);
*refuse newer versions* (a one-rule contract over client convenience).

Two things never yield: `Marshal` never emits what `Validate` rejects, and
no loss is silent.

## Non-goals

- **Not a sync or wire format.** The CRDT protocol is unchanged; AnyBlock is
  interchange, backup, and the agent editing surface — which is what makes
  rule 10's refusal affordable.
- **Not a replacement for the Markdown export.** Markdown stays the lossy
  human format, served read-only with warnings.
- **Not byte-equal to arbitrary input.** The canonical form is the fixed
  point (rules 1, 8).
- **Not forward-compatible** (rule 10).
- **Not a per-object schema language.** Per-type validation artifacts — a
  JSON Schema, a prompt-ready property table — are derived one-way from type
  documents and never imported (§2a).
- **Not an archive layout.** One object per document; how files are laid out
  and binaries shipped belongs to the writer (§1).
- **Not yet:** the isomorphic HTML sibling (the Pandoc precedent) and the
  compact filter string *inside documents* (its parser ships for the API;
  the document field is reserved, §6.2.1).

## How the rules are kept

- A hand-authored JSON Schema 2020-12 — closed objects, exhaustive enums,
  non-recursive block definition, discriminator-first dispatch — embedded
  in the package and published at a stable URL (§12).
- Package tests: golden files, property-based round-trip tests over
  generated states, and a corpus invariant that `Validate` and `Unmarshal`
  agree (§11, §12). `cmd/anyblockroundtrip` sweeps a production account;
  `ANOMALIES.md` is its ledger.
- Review rules for changes: a new name follows rule 3's precedence; a new
  field either round-trips (rule 1) or is output-only and marked
  `x-output-only` (§4a); a new validation check catches something silent
  and traces to a mechanism (rule 8); a new serialization option is a view
  of the one format, never a dialect (rule 9).

## How this document changes

- A change to the format, to this package, or to API v2's document surface
  names the rule it serves — or the rule it bends and the cost it accepts,
  recorded in `SPEC.md`'s changelog and, when real data drove it, in
  `ANOMALIES.md`.
- Rules move on evidence — a sweep, a benchmark on the target model tier, a
  falsified assumption — never on taste. Several already have: nested →
  flat (constrained-decoding evidence); key allowlist → deny rule (59 real
  keys falsified it); omit-default properties → presence is meaningful
  (14,032 flags); camelCase → snake_case (a model wrote
  `bulleted_list_item` unprompted against a camelCase draft).
- When this document and `SPEC.md` disagree, one of them has a bug, and the
  change that fixes it says which.
- It is short on purpose. Details belong in `SPEC.md`, decisions in
  `OVERVIEW.md`, data in `ANOMALIES.md`.

## Prior art — what was taken, and what was left

| Source | Taken | Deliberately left |
|---|---|---|
| [HTML Design Principles](https://www.w3.org/TR/html-design-principles/) (W3C) | the shape of this document; *do not reinvent the wheel*, *pave the cowpaths* (rule 3); *priority of constituencies* (collisions); *well-defined behavior* (rule 8) | *degrade gracefully* — reversed in rule 10, which a non-wire format can afford |
| [CommonMark](https://spec.commonmark.org/) | readability as the overriding goal; the inline subset and escaping; spec-by-examples → golden files | the delimiter-run algorithm — replaced by an exact-inverse stack parser, because byte-stability needs an inverse (§8.3) |
| [Djot rationale](https://github.com/jgm/djot#rationale) | linear parsing; no expressive blind spots; one spelling per construct | — |
| Block-editor APIs (common vocabulary) | block and property names (`bulleted_list_item`, `heading_1`, *property*); options by name | `{id, name}` option objects (rule 6); `database` (rule 3) |
| Atlassian Document Format | an envelope with a single `version` integer; `type`-discriminated nodes | additive-within-a-version; nested `content` trees (rule 4) |
| [Portable Text](https://www.portabletext.org/) | JSON blocks as the unit; a legend referenced by key (`markDefs` → `refs`) | marks as arrays on spans — Markdown in `text` instead (rule 4) |
| [JSON Canvas](https://jsoncanvas.org/), [OKF](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md) | a short spec with its purpose stated first; goals and non-goals up front; longevity, readability, interoperability as the brief | — |
| Anytype public REST API (`core/api`) | format names (`select`, `multi_select`, `text`, `objects`, `files`); snake_case keys; the slug vocabulary | id/key duality; value fields named after formats |
| Agent-API evidence 2024–2026 (`docs/AgentApiV2Research.md`; [Ustynov 2026](https://arxiv.org/abs/2604.07502)) | id-addressed edits; constrained decoding as the small-model floor; examples over prose; SQL-shaped filters; the validation loop as product surface; compact but not exotic | tabular/TOON-style output by default; raw JSON Patch; whole-document rewrite as the default edit |
