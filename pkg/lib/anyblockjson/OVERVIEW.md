# AnyBlock JSON — what it is and why it looks like this

A readable, strictly-validatable JSON representation of an Anytype object.
It replaces `.pb.json` (jsonpb of `SnapshotWithType`) as the export/import
format, and it is the document shape API v2 serves and accepts.

`SPEC.md` is normative and long. This is the short version: the decisions
that shaped it, why each one was made, and what they look like.

The audience assumption behind almost every decision: **the reader and
writer is often a language model.** That is not a nice-to-have framing —
it is what settled most of the arguments below, usually against the choice
a human-only format would have made.

---

## The document

```json
{
  "$schema": "https://schemas.anytype.io/anyblock/1/object.schema.json",
  "version": 1,
  "id": "bafyreieqh63jv…",
  "type": "page",
  "properties": {
    "name": "Project Phoenix",
    "icon_emoji": "🔥",
    "status": ["In progress"]
  },
  "blocks": [
    { "id": "b1", "type": "heading_2", "text": "Goals" },
    { "id": "b2", "type": "paragraph",
      "text": "Ship the **new export** by Q3 with <mention object_id=\"bafyreidf…\">Roman</mention>" },
    { "id": "b3", "type": "bulleted_list_item", "text": "Flat JSON schema" },
    { "indent": 1, "id": "b4", "type": "bulleted_list_item", "text": "Validate in CI" },
    { "id": "b5", "type": "checkbox", "checked": true, "text": "Draft spec" },
    { "id": "b6", "type": "code", "language": "go",
      "text": "func main() {\n\tfmt.Println(\"hi\")\n}" }
  ]
}
```

Three things are load-bearing and worth noticing before the rationale:
blocks are a **flat array**, formatting lives as **markdown inside
`text`**, and `properties` is a plain **key → value** map.

---

## The decisions

### 1. Blocks are flat, with an integer `indent` — not a nested tree

Pre-order array; `indent` omitted when 0; no `children` key anywhere.

**Why.** A nested tree needs a recursive schema (`$defs/block` referring to
itself), and a recursive schema **cannot be used with constrained decoding
or provider strict-mode**. Constrained decoding is precisely the mechanism
that rescues small models — in our prior-art review it took a 7B model from
0% to 75% on valid emission. Trading it away to keep `children` was not
close.

Two supporting reasons. A truncated flat array is still a **valid prefix**
of the document, so a cut-off generation degrades to "fewer blocks" rather
than "unparseable". And transformer failure on structured output tracks
*depth*, not length — a real corpus datum here is that typical documents
nest ~6 deep, with outliers to 26, which is beyond the reliable depth for
sub-7B models and beyond some providers' nesting caps.

The cost is honest: an off-by-one `indent` silently mis-parents a block
where a misplaced `children` bracket would have been a parse error. That is
paid for with strict monotonicity validation on import (an indent jump
greater than +1 is an error, path-addressed), plus a documented lenient
clamp mode that follows CommonMark's rule.

### 2. Inline formatting is markdown inside `text`, not mark ranges

`Ship the **new export**` — not `{"text": "...", "marks": [{"from": 9, "to": 21, "type": "bold"}]}`.

**Why.** The protocol stores marks as UTF-16 offset ranges. Models cannot
produce or maintain offset bookkeeping — this is a well-documented failure
(Google Docs' "write backwards" workaround; Liveblocks' "great at
rewriting, terrible at patching"). Markdown puts the formatting *where the
formatting is*, so editing a sentence cannot desynchronize it from its
marks.

A whitelist of tags covers what markdown lacks: `<u>`, `<mention
object_id="…">`, `<font color|background>`. Emoji marks are materialized
into the text — lossy by design, and the only deliberate loss in the
format.

### 3. Names, not ids, wherever a human wrote the name

`select` and `multi_select` values are option **names** (`"In progress"`),
in property values, filter values and custom orders alike. Properties are
addressed by key; types by type key.

**Why.** An id is unguessable, so a model must fetch before it can write;
a name is already in the user's request. Import creates missing options by
name, matching the existing import semantics. The trade — two options with
the same name collapse on import — was accepted explicitly, and it is
recorded as a known anomaly rather than hidden.

### 4. Presence is meaningful

Property values are written **verbatim**, including `false`, `0`, `""`,
`[]` and `null`. The omit-empty-and-default canonicalization applies only
to block attributes and envelope fields.

**Why.** This one was decided by data. The first production sweep flagged
14,032 "issues" that were all the same thing: default scalars
(`is_hidden:false`, `revision:0`) that canonicalization had dropped. The
ruling was that **a user setting a property to empty is a fact**, and the
format has no business deleting it. Blocks are different — an absent
attribute there genuinely means "default".

### 5. Vocabulary chosen for outsiders, not for the codebase

`relation` → **property** everywhere. `smartBlockType` → `kind`.
`header*` → `heading*`. `bulleted` → `bulleted_list_item` (Notion's name).
Formats are `select`/`multi_select`/`text`/`files`/`objects` — the REST
API's names, not the internal `status`/`tag`/`longtext`; the stored
shorttext/longtext split has one name between them, `text`. And everything
the format defines is spelled `snake_case`, digits included.

**Why.** The instruction was "rename everything, minimize new terms". The
format is read by people and models with no exposure to Anytype's
internals, and the largest single source of confusion was a vocabulary
that only made sense if you knew the history. "Relation" now appears
nowhere in the format.

### 6. The compaction that survives is the legend-less one

Full object ids are ~59-character CIDs — a single mention can cost more
tokens than the sentence containing it. There used to be two compactions:
one that shortened object references behind a `refs` legend, and one that
relabels document-local block/row/column/view ids to short suffixes. The
first is deleted; only the second is left.

- **`CompactBlockLabels`** relabels doc-local block/row/column/view ids to
  their last 5 characters. **Legend-less and lossy.** `CompactIds` is now an
  alias for it.
- `OmitIds` drops ids entirely, for generation.
- **Object references are written in full, on every shape.**

**Why the "lossless" half died and the "lossy" half stayed.** An indirection
table has three obligations — it must be carried, kept in sync, and read
back — and the object legend failed all three. API v2 removed the same
legend from its read shape after measuring a net token *loss* per document
and finding that it trapped write-back: an agent editing an object-valued
property through a label has to keep the legend in step, and one that
regenerates the document without it silently re-points every reference. The
freeze review measured a 200-item collection growing 32.7% under compaction.

A block label has none of those obligations. It is a placeholder inside its
own document, never an address outside it, and a write endpoint resolves one
against the live object by unique suffix. Nothing to desynchronise.

### 7. The round-trip contract is a fixed point, not byte-equality

`Import(Export(S)) ≡ N(S)`, and `Export∘Import` is idempotent and
byte-stable — where `N` is a documented normalization (structural blocks
dropped, option ids resolved to names, marks canonicalized, deprecated
fields cleared, and so on).

**Why not byte-equality with arbitrary input.** Because the format
deliberately drops things: structural blocks are regenerated by the editor
at first open (they are layout-dependent — a note has no title), and
normalization is the point rather than an accident. Promising byte-equality
would have meant carrying every legacy shape forward forever.

### 8. Validation is discriminator-first, with path-addressed errors

The schema branches on `type` before validating a block, rather than
presenting a flat `oneOf`.

**Why.** A flat `oneOf` produces "does not match any of 23 schemas", which
is useless to a model *and* to a human. Discriminator-first produces "at
`/blocks/7/columns`: a `table` requires `columns`". Errors are the repair
instruction, so they are addressed to the exact path that is wrong.

---

## What it has been tested against

The format is swept over a real production account — every object exported
to AnyBlock JSON, re-imported, and re-exported, comparing both the state and
the bytes. That sweep is `cmd/anyblockroundtrip`; its run history and every
figure belong in `ANOMALIES.md`.

What the state comparison covers is narrower than it sounds. `snapshotdiff`
compares detail values (up to the documented normalizations) and the plain
text of text blocks as a multiset — not marks, not block order, not table
shape, not dataview content, not file or bookmark metadata; and
byte-stability is self-consistency of the pipeline, so a systematic drop can
be byte-stable and invisible. Its own package doc says it: findings are
triage input, not proof.

Within those limits it earns its keep. The pre-flat sweep (run 3, 35,369
objects) round-tripped 99.86% byte-identically; the flat-encoding sweep that
followed (run 4, 35,372 objects) left 21 failures, all in categories already
known. The most recent one, over a 36,808-object account, is where the last
round of findings came from — and no pass rate for it is recorded anywhere.
The fixes made since have unit tests, not a re-measurement.

Every anomaly found along the way is written up in `ANOMALIES.md` rather
than smoothed over — including two genuine silent-data-loss bugs the sweeps
caught that no unit test had. That is the reason to check the round-trip
contract against real data instead of merely believing it, and the same
reason not to read a pass rate as a proof of it.

---

## Deliberate non-goals

- **Not a wire format.** It is an export/interchange and agent-editing
  format; the CRDT protocol is unchanged.
- **Not byte-equal to arbitrary input** — see decision 7.
- **No backward-compatibility burden yet.** The format has never shipped,
  which is why the decisions above could still be reversed on evidence.
- **Emoji marks are lossy**, materialized into the text. The only
  deliberate loss.

---

## Where to look next

| | |
|---|---|
| `PRINCIPLES.md` | the ten rules the format answers to, and the order they yield in |
| `PRINCIPLES_SHORT.md` | the same ten rules on one screen |
| `SPEC.md` | normative, complete, §14 has a full worked example |
| `ANOMALIES.md` | every real-data oddity found, with evidence |
| `APIV2_ADDRESSING.md` | the identifier-layer dossier (naming, keys, minting) |
| `cmd/anyblockroundtrip` | the production sweep harness |
| `schema/*.json` | the hand-authored JSON Schema (2020-12) |
