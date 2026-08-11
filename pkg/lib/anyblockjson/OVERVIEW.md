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
  "$schema": "https://schemas.anytype.io/anyblock/1.0/object.schema.json",
  "version": 1,
  "id": "bafyreieqh63jv…",
  "type": "page",
  "properties": {
    "name": "Project Phoenix",
    "iconEmoji": "🔥",
    "status": ["In progress"]
  },
  "blocks": [
    { "id": "b1", "type": "heading2", "text": "Goals" },
    { "id": "b2", "type": "paragraph",
      "text": "Ship the **new export** by Q3 with <mention objectId=\"bafyreidf…\">Roman</mention>" },
    { "id": "b3", "type": "bulletedListItem", "text": "Flat JSON schema" },
    { "indent": 1, "id": "b4", "type": "bulletedListItem", "text": "Validate in CI" },
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
objectId="…">`, `<font color|background>`. Emoji marks are materialized
into the text — lossy by design, and the only deliberate loss in the
format.

### 3. Names, not ids, wherever a human wrote the name

Select and multiSelect values are option **names** (`"In progress"`), in
property values, filter values and custom orders alike. Properties are
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
(`isHidden:false`, `revision:0`) that canonicalization had dropped. The
ruling was that **a user setting a property to empty is a fact**, and the
format has no business deleting it. Blocks are different — an absent
attribute there genuinely means "default".

### 5. Vocabulary chosen for outsiders, not for the codebase

`relation` → **property** everywhere. `smartBlockType` → `kind`.
`header*` → `heading*`. `bulleted` → `bulletedListItem` (Notion's name).
Formats are `select`/`multiSelect`/`text`/`shortText`/`files`/`objects` —
the REST API's names, not the internal `status`/`tag`/`longtext`.

**Why.** The instruction was "rename everything, minimize new terms". The
format is read by people and models with no exposure to Anytype's
internals, and the largest single source of confusion was a vocabulary
that only made sense if you knew the history. "Relation" now appears
nowhere in the format.

### 6. Ids compact in two independent halves

Full object ids are ~59-character CIDs — a single mention can cost more
tokens than the sentence containing it.

- **`CompactObjectRefs`** shortens object references and adds a `refs`
  legend to the envelope. **Lossless** — the legend inverts it.
- **`CompactBlockLabels`** relabels document-local block/row/column/view
  ids to short suffixes. **Legend-less and lossy.**
- `OmitIds` drops ids entirely, for generation.

```json
"refs": { "miovm": "bafyreieqh63jv…miovm", "roman": "bafyreidfmzjh…" }
```

**Why two halves rather than one switch.** They pay differently and
consumers legitimately want one without the other: an editing surface wants
short block labels but full inline object refs, while a backup shape wants
full block ids and takes the legend, because its bytes must re-import to
the same document.

**`refs` is an authoritative opaque map**, not a shortening convention.
Keys need not be suffixes of their values — "last 5 characters" is merely
export's key-choice algorithm (suffixes, because CIDs share *prefixes*).
Agents may add entries with any label they like and reference them:
`"roman": "bafyrei…"`. The resolution rule is total — if a value is a key
in `refs` it resolves to that id, otherwise it *is* a full id. No
"short-looking" heuristic, so there is no ambiguous middle case.

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

The format was swept over a real production account — **35,369 objects**
across 30+ spaces — exporting each to AnyBlock JSON, re-importing, and
re-exporting, comparing both the state and the bytes.

The final sweep passed **99.86%**, and the remaining failures were traced
and fixed until only accepted anomalies remained (duplicate-named tag
options collapsing, per decision 3). Every anomaly found along the way is
written up in `ANOMALIES.md` rather than smoothed over — including two
genuine silent-data-loss bugs the sweep caught that no unit test had.

That sweep is `cmd/anyblockroundtrip`, and it is the reason to trust the
round-trip contract rather than merely believe it.

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
| `SPEC.md` | normative, complete, §14 has a full worked example |
| `ANOMALIES.md` | every real-data oddity found, with evidence |
| `ADDRESSING.md` | the identifier-layer dossier (naming, keys, minting) |
| `cmd/anyblockroundtrip` | the production sweep harness |
| `schema/*.json` | the hand-authored JSON Schema (2020-12) |
