# AnyBlock JSON — format specification

Status: **draft** · Format version: **2** · Package: `pkg/lib/anyblockjson`

A human- and agent-readable JSON serialization of Anytype objects (the "anyblock"
model), designed for export, import, and generation by external tools and LLM
agents. It replaces the raw `jsonpb` dump (`.pb.json`) as the recommended JSON
interchange format.

Design lineage: the envelope and block tree follow the Atlassian Document
Format (nested `type`-discriminated tree, a single `version` integer —
though evolution here is never additive, §10); inline formatting uses a
Markdown subset inside text
strings; the vocabulary follows Anytype's public REST API (`core/api`) and
common block-editor conventions wherever an established term exists — the format should be
readable, and mostly writable, by someone who has never seen Anytype
internals.

## 1. Goals

1. **Readable** — a person can read and hand-edit a document; structure is
   visible as nesting, formatting as familiar Markdown; every name answers to
   something the reader knows from HTML, Markdown, SQL, or common REST APIs.
2. **Generatable** — an LLM or script can produce a valid document from one
   example, without offsets, id cross-references, or Anytype-specific
   instructions.
3. **Strictly validatable** — a published JSON Schema (draft 2020-12) covers
   the structural format; the Go package validates on import (schema +
   semantic checks, including the inline grammar) and returns structured,
   path-addressed errors.
4. **Lossless round-trip** — `Import(Export(S))` reproduces the state `S` up
   to the normalization defined in §11; export is canonical and
   `Export ∘ Import` is idempotent (§11).

### The four consumers

The four goals above are not four independent wishes. Each is claimed hardest
by one of four consumers, and the consumer that claims a goal hardest is the
one that sets its bar.

| | Consumer | May assume | Owes |
|---|---|---|---|
| **1** | **Export and import** — backup, migration, round trip: `Marshal`/`Unmarshal` (§13), an archive or a bundle on disk (§2c) | a live space at both ends, and that the bytes it reads are bytes it wrote | goal 4 (lossless up to §11, canonical output) and goal 1 |
| **2** | **Authored documents** — an agent, a script or a person writing objects, types and whole bundles for a space that may not exist yet | nothing but the document, the schema, and the bundled key table every reader ships | goal 2, and the offline half of goal 3 |
| **3** | **The API over the format** — API v2 (`core/api/v2`): explicit operations against a live space | a space, a store, and the resolvers of §13 | the store-backed half of goal 3 — resolve, refuse or create, and say which |
| **4** | **Tool wrappers over that API** — the task-shaped tool set models drive instead of the raw surface, delivered as CLI verbs and as an on-device manifest (the API v2 design record's tool layer; the record itself is retired from the tree) | everything layer 3 guarantees | nothing this format defines: a context budget |

**Layer 1 sets the readability bar, and every other layer inherits it.** Its
reader is the one that can ask nothing — a file open in an editor, a bundle on
a disk, a space that no longer exists. So identity that cannot be spelled
readably is not demoted to an id: the label stays in place and the map goes in
the envelope — `property_internal_keys`, `type_internal_keys` and `option_ids` (§3). *Naming* above records that move being made once already: the
key ↔ key mapping went into the DOCUMENT precisely because `Validate` takes no
resolver. One grammar, so the weakest reader sets the spelling. How far this
goes is bounded and the bound is written down — formats still resolve through
caller-wired resolvers (§3) and a name sidecar is a v1 non-goal; `PRINCIPLES.md`
rule 6 (*Names, not ids*) marks that gap tracked, not accepted.

**Which is why a select value is spelled by its name** (§3) — and not merely
because names read better. *A bundle carries no option objects.* A linked
object travels in the bundle and the importer relinks it; an option id from
another space would dangle. The name is the only address that survives the
trip, a fact about what moves between layers 1 and 2 rather than a preference
about what reads nicely. The scope is exactly select and multi_select values:
object references stay ids by decision (*Non-goals* below). The id is not lost
— it rides in `option_ids` beside the name, honoured only where the target space
still serves it as a live option of that relation (§3). A format whose
readers can always resolve a code can afford to demote the human label beside
it to a non-authoritative hint; ours frequently cannot, so the name stays
load-bearing and the id is filed where a stale one does no damage.

**Layer 2 is why ids are optional.** Its author has none — no id to quote, no
prior state to preserve, nothing to fetch before writing. So block ids are
optional on input and `OmitIds` writes that shape back out (§9); the envelope
`id` is not part of that trade and stays. What cross-document identity a bundle
needs, it mints for itself: `index.json` and every widget target are the
bundle's own slugs, relinked on install like any other reference (§2c). An
id-less document is a first-class input and an alternative serialization, never
a dialect (`PRINCIPLES_SHORT.md` rule 9).

**The line between layers 2 and 3 is a function signature.** `Validate(data
[]byte) error` takes bytes and nothing else — no space, no store, no resolver
(§13). Everything on the near side is checkable by an author with no account:
id collisions across a document (§4), two property spellings binding onto one
stored key, a `group_by` no view can honour (§6.2), a legend entry that can
never be consulted (§9a, §12), a malformed inline grammar (§8.1). Everything of
the form *does this already exist here* — is that type present, is that name
already an option, which of the two options named `High` did you mean, does that
id still resolve — is structurally on the far side, and no argument `Validate`
could take would move it. That is not a gap to be filled later; it is the
boundary, and it is what lets a bundle be checked before the space it creates
exists. The cross-document half belongs to the bundle tooling —
`anyblockvalidate` builds its id set from the bundle's own files to reject a
widget target no document defines (§2c), and still asks no space anything.

**So the format has no "create this option" flag** — the request that arrives
once per reader. Three reasons; the third settles it.

- **The format describes a state, and a state has no verb slot.** A document
  says what an object *is*; `create` is something a caller *does*.
- **Its value is fixed per consumer, never per document.** Layer 2 *requires*
  implicit creation — an authored bundle declares types, properties and options
  no space has seen, and the bundle **is** their creation (§2 `type`, §2a, §3).
  Layer 1 authors nothing: everything its documents name existed in the space
  they came from, so a missing option at import is a restoration rather than a
  new design decision. A field that is always true for one caller and always
  false for another is describing the caller's intent, not the object.
- **Only layer 3 holds the store the question is about** — and it answers it
  there, in the direction this argument predicts. The format's own import
  default creates what is missing (§3); API v2 gates that behind an explicit
  request parameter defaulting to off, answering a name it cannot resolve with
  a did-you-mean error instead, and the tool wrapper is stricter still (the
  small-model review's finding, recorded in the retired API v2 design
  record). Same document, opposite
  defaults, chosen by the caller.

**Failure is loud at every layer** (`PRINCIPLES.md` rule 8, *Strict in, canonical out*): version refusal
with no partial read (§10), one unsupported file and a whole bundle declines to
install (§2c), path-addressed import errors (§12), a warning for an `option_ids` entry
nothing can consult (§9a), and layer 3 naming the candidates rather than
choosing one. The two places the line bends are both documented as such and
both compensated: name resolution answers the FIRST when two options of one
property share a name, which is the whole reason the document carries the id
beside the name (§3); and a widget target that resolves to nothing, silently, which is why the
tooling refuses it before install rather than leaving it to be discovered (§2c).

**Layer 4 subtracts — but it has already added.** A tool set hides what a model
should not spend context on, ids first of all: the wrapper resolves enumerated
handles server-side so the model never emits a CID (the API v2 design
record's tool layer). What it
may not do is invent a dialect. It would be false, though, to say the format
owes it nothing. Block ids relabel to short
suffixes because a 59-character CID costs more tokens than the sentence around
it, and they can do so safely only because that relabeling carries no legend to
trap a write-back (§9a); `OmitIds` exists for templates and
prompt examples (§9); `blocks` is a flat array partly because guided decoders
cannot express a recursive schema. Layer 4's needs shape this format —
they are met as vocabulary and as serialization options, never as a second
format.

**One grammar, four serializations.** The layers pay in different currencies,
so the same document is written differently at each:

| | block ids | object refs |
|---|---|---|
| 1 · export, backup | full — the bytes re-import to the same document, up to what the editor regenerates (§7, §7a, §11) | full — always (§9a) |
| 2 · authored | none (§9, `OmitIds`) | the bundle's own slugs (§2c) |
| 3 · API v2 default read | compact by default; `?ids=full` opts out | full — always |
| 4 · read-only and tool shapes | short labels (outline, prompt examples) | hidden behind enumerated handles |

Rows 3 and 4 are the API's choice, not the format's (API spec C4). What the
format contributes is that the one compaction it offers carries no legend
(§9a) — a read can be relabeled without a backup having to carry an
indirection table a later edit could desynchronise — and that all four rows
are one document, one validator, one version.

### Naming

**Every identifier the format defines is `snake_case`**: block types, field
names, enum values, and inline tag attributes. The spelling is the internal
name run through the same conversion Anytype's public REST API applies to its
own keys (`strcase.ToSnake`, `core/api/util/key.go`), **digits included** —
`heading_1`, `toggle_heading_1`, `bulleted_list_item`, `table_of_contents`.
Stating the rule rather than a list means a name added later needs no decision.

This follows the vocabularies §1 claims lineage from — common block-editor
naming (`bulleted_list_item`, `heading_1`) and Anytype's public API
(`background_color`, `added_at`) — and, more to the point, it is the spelling a
generating model produces unprompted: the format's own pre-freeze review
records an LLM writing `"type": "bulleted_list_item"` against a camelCase
draft, which the reader then had to reject.

**Two kinds of string are exempt, both because they name something outside the
format.** They are not inconsistencies to be tidied away later:

- **Property and type keys** (§3) name relations and types, which live in a
  space rather than in this format. Their canonical spelling is the entity's
  **display name**, NFC-normalized and otherwise verbatim — `"Due date"`,
  `"Plural name"`, `"Publish Date"`, `"Тоггл"` — so the exemption is the
  ordinary case, visibly: spaces, capitals and any script, exactly as the
  user named the thing. Where no name can spell the key at all (an empty or
  unwritable name, a collision the §3 ladder cannot suffix), the stored key
  is written verbatim, whatever its shape, because an exact stored key is
  always its own address (§3).
  The key ↔ key mapping goes into the DOCUMENT — the
  `property_internal_keys` / `type_internal_keys` legends — because `Validate`
  takes no resolver, and a reader with no space at all can invert them.
- **Platform identifiers** — the `dataview` block id (§7) and the `objectId`
  parameter of the `anytype://object` deep link (§8.1) — name things that
  exist in a live space. They are quoted, not translated.

### The `_` namespace

**A value beginning with `_` addresses the platform, and nothing a document or
a bundle mints may begin with one.** The platform's own addresses already live
there — `_otpage`, `_brdue_date`, `_missing_object`, `_participant_…`,
`_date_2024-01-01` — and the format borrows the same namespace for the
built-in screens and listings an `index.json` may name: `_favorite`,
`_recent`, `_recent_open`, `_set`, `_collection`, `_all_objects`, `_chat`,
`_bin`, `_widgets`, `_graph` (§2c).

The point is that the two sets are then disjoint **by construction**, not by
inspection. While the reserved listings were bare words, a bundle shipping an
object with id `set` captured every widget that meant *the Sets listing*: the
pb importer resolves a widget target through the bundle's own id map first
(`common.UpdateLinksToObjects`) and only then asks
`widget.IsPredefinedWidgetTargetId`, so the object won, silently, with no
finding from any check. The reserved homepages had the same collision with the
precedence reversed — `builtinobjects.setWorkspaceSettings` matches the
reserved names *before* resolving an id, so a bundle object with id `graph`
could never be the homepage. One rule closes both directions.

This is a **prefix** rule, which is why it can be permanent: "no minted id
STARTS with `_`" is a promise the platform can keep, where reserving a word
means banning a new id every time a listing is added, retroactively. The name
after the prefix is still this format's own and is still snake_case, which is
why `_all_objects` and `_recent_open` are spelled that way rather than
quoting the live space's `allObjects` / `recentOpen`: the format's spelling
is its own everywhere, and the wire's camelCase comes back at the boundary
with everything else (`WireWidgetTarget`).

The prefix is translated away at the wire boundary, where the importer's own
spellings are bare (`favorite`, `graph`), by `WireWidgetTarget` /
`WireHomepage`. Writing `_set` into a link block would be strictly worse than
the shadowing it replaces — an unrecognised target becomes
`addr.MissingObject` and the widget is then stripped without an error.

**Which is why a bundle-local id may not be one of those bare spellings
either** — `favorite`, `recent`, `recentOpen`, `set`, `collection`,
`allObjects`, `chat`, `bin`, `widgets`, `graph`. The prefix rule alone would
only move the collision one step downstream: the link block that leaves this
format saying `_set` reaches the importer saying `set`, and
`handleLinkBlock` still resolves through the bundle's ids first. The prefix
is what makes the *format* unambiguous — a reader never has to guess which
of the two kinds a target meant, and a typo inside the namespace can be
refused by name. The wire-word ban is what makes the *wire* unambiguous.
Both, or neither is worth having.

(The JSON Schema's own `$defs` names — `blockCore`, `tableCell`, … — are
neither: they are schema-internal labels a document never contains, and they
keep JSON Schema's conventional camelCase.)

So `{"type": "callout", "icon": {"format": "emoji", "emoji": "💡"}}` and
`{"properties": {"icon_emoji": "☕"}}` are both correct in the same document,
and they are not the same thing spelled twice: the first is a field this
format defines, the second is a key belonging to the data — and
it can only be a SPACE-MINTED relation that happens to be stored under that
name, because the bundled `iconEmoji` is refused there (§2b). 54 production
objects hold exactly that pair. `{"properties": {"wikiPerson": …}}` is where
the two part company.

### Terminology

The format uses six Anytype concepts; everything else is borrowed vocabulary:

- **object** — a page-like unit (page, task, note, …); one JSON document per
  object.
- **property** — a typed key-value on an object (stored
  internally as a *relation* — the internal name never appears in the
  format).
- **type** — the object's user-level type (`page`, `task`, `bookmark`…),
  identified by a key.
- **option** — a named choice of a `select`/`multi_select` property.
- **set vs collection** — a *set* is a live query over a type; a
  *collection* is a manually curated list of objects. Both are presented
  through dataview blocks/objects.
- **space** — the container all object ids resolve within (never appears in
  documents; ids are space-local).

### Non-goals (v1)

- Replacing the Markdown export (stays as the lossy human format).
- Multi-object archives: this spec defines a **single object per document**;
  archive layout (one file per object, file binaries alongside) is owned by
  the export writer, unchanged.
- Resolving object references to names (values stay ids, except
  select/multi_select options — §3; a name sidecar may be a future
  extension).
- An HTML-style sibling format (planned separately, isomorphic to this one —
  the Pandoc precedent).

## 2. Document envelope

```json
{
  "$schema": "https://schemas.anytype.io/anyblock/2/object.schema.json",
  "version": 2,
  "id": "bafyreieqh63jv…",
  "type": "Page",
  "icon": { "format": "emoji", "emoji": "🔥" },
  "properties": { … },
  "blocks": [ … ]
}
```

Fields, in **canonical order** (§4):

| Field | Type | Req | Notes |
|---|---|---|---|
| `$schema` | string | no | Schema URL; written by export, ignored by import except for version detection (§10). |
| `version` | int | **yes** | Format version. This spec defines `1`. Every format change bumps it — there is no additive-within-a-version rule (§10). |
| `kind` | string | no | System-level object kind, snake_case (`page`, `profile_page`, `template`, `archive`, `widget`, `chat`, …) — from `model.SmartBlockType`. `chat` is `ChatDerivedObject`: a standalone chat object whose identity is `internal_key`, like a type's; its messages live in the CRDT store, not in snapshots, so it always imports empty. (`chat_object` is the deprecated predecessor; `discussion` is a hidden type.) **Omitted whenever derivable**: absent means `page`. It is the SOLE authority on whether a document is a template — `template_for` is admitted on it, the second type slot exists on it, and no type spelling implies it (§3). A template therefore always spells its kind. An unrecognized value is a validation error listing the allowed values. |
| `id` | string | no | Object id. Written by export; import treats it as informational (a new id is minted on import) except for resolving intra-export links. Written in full, like every object reference — object references are never compacted (§9a). |
| `type` | string | no | The object's type **slug** (`page`, `task`, `object_type`…) — the key vocabulary of §3, not the stored `ot-`-prefixed key. Maps to `object_types[0]` in the snapshot. Absent when the snapshot has no object types (legacy/system objects). Import inverts the term through the §3 chain in the type namespace — the document's own `type_internal_keys` legend first, then the vocabulary in force (bundled table offline, the space's stored slugs inside a node) — and hands the resulting stored key to the wiring, which resolves it — matching an existing type or creating one (the Markdown importer's behavior). A term the chain does not know passes through verbatim — an exact stored key is always its own address (§3). No spelling is reserved: `template` is an ordinary type term that a legend or a vocabulary may bind wherever it likes, because `kind` — a field no chain touches — carries the template semantics it used to carry. The one exception is a byte comparison, not a resolution: a document with **no `kind`** whose `type` is literally `template` is the legacy spelling of a template and is refused, naming the repair (§10). |
| `template_for` | string | no | Only for templates: the target type slug (`object_types[1]`), same vocabulary and legend as `type`. Admitted on `kind: "template"` and nothing else — present without it, or without a `type` beside it to be `object_types[0]`, is a validation error. Note what this is NOT keyed off: the template's own type. A template whose `object_types` do not begin with the template key is a shape the model permits. The target does not depend on what `object_types[0]` holds. |
| `internal_key` | string | no | Identity key of *system* objects (types, properties). This is the STORED identity key (a `uniqueKey`'s internal part), written verbatim: unlike every key slot in §3 it is **not** translated, so for an object whose stored key is a minted BSON it does not match the slug the public API serves as that object's `key`. The name says what the value is — an id the app MINTS (a bson for a custom definition, the camelCase bundled key for a bundled one), never something an author derives — where the word `key` used to name this stored id AND a property definition's spelling one level down, one word for two concepts (§15 #14). Because it is verbatim, its charset is whatever the store already holds: a relation option's key is built from the option's *name*, so `completion_status_Not Started`, `…_C/C++` and `…_тогглы` are all real stored keys. The rule is therefore a deny rule — non-empty, no control characters, at most 255 characters — not an allowlist. An allowlist was tried and falsified: it failed 59 objects of a 36 808-object account, every one a relation option. Never emitted for ordinary documents. |
| `property_settings` | object | on `kind: "property"` | Only for property documents, where it is **required**: the definition of the property this document IS — one `propertyDefinition` (§2d, §2e). Carries `format` (required, a §3 format NAME — never a raw enum number; stands for the stored `relationFormat` key, which `properties` refuses), `include_time` and `object_types`, each present exactly when its stored key is, value included. Illegal on every other kind. |
| `icon` | object | no | The object's icon — ONE object whose `format` selects the variant (§2b). Stands for the stored `iconEmoji` / `iconImage` / `iconName` / `iconOption` keys, which `properties` refuses. |
| `cover` | object | no | The object's cover — same shape, three variants (§2b). Stands for the stored `coverId` / `coverType` / `coverScale` / `coverX` / `coverY` keys, which `properties` refuses. |
| `properties` | object | no | The object's properties, §3. |
| `type_settings` | object | no | Only for type documents (`kind: "object_type"`, `"bundled_object_type"`): everything that defines the TYPE, in one gated subtree — `layout`, `api_key`, `plural_name`, `default_template`, `default_view`, and `property_definitions` (§2a). Present on any other kind → validation error. The root spelling `type_properties` is refused with the repair named. |
| `property_internal_keys` | object | no | Legend: the stored property key each spelling in this document names (§3). Written for every spelling the **bundled table does not bind to the key being written** — a slug the table cannot invert (a space's own key) *and* the **identity entry**, which is the ordinary case: a custom key written verbatim names itself, because nothing else in the document says the term is a stored key rather than somebody's slug. A reader consults it **before** its own vocabulary and takes the value as **authoritative**: it is not liveness-checked, deliberately (§3). Absent only from a document whose every spelling is bundled. |
| `type_internal_keys` | object | no | Legend: the stored type key each type slug in this document names — `property_internal_keys`' twin on the TYPE namespace, written and consulted under the same rule (§3). A separate map, deliberately: a space may slug a relation and a type onto one term, so one map could not carry both meanings of a shared spelling. |
| `option_ids` | object | no | Legend: the id of the option each select/multi_select **name** in this document stands for — nested, `{property spelling: {option name: option id}}` (§3, §9a). Written **unconditionally** wherever export spells an option by name; dropped by `OmitIds` (§9). Read as a **hint**, not an address: an id is honoured only where the target space still serves it as a live option of that relation, and otherwise the name resolves exactly as it did before the legend existed. |
| `blocks` | array | no | The document's blocks as a **flat pre-order array**; nesting via `indent` (§4). |
| `items` | array | no | For collection objects: member object ids, in order (from the internal collection store key `objects`). Present on a non-collection document → validation error — enforced by the import *wiring* (collection-ness resolves against the space's types, not offline); the package's `Validate` checks structure only (implementation decision). |
| `store` | object | no | Escape hatch: remaining internal store content as a free-form JSON object, with the `objects` key lifted into `items`. Output-only (§4a). (Named `store` — its internal name — to avoid colliding with the collection concept.) |
| `root` | object | no | Escape hatch for non-default root-block attributes (`fields`, `background_color`); absent in the common case. Output-only (§4a). |

The root block of the snapshot (whose id equals the object id) is
**implicit**: its subtree becomes the `blocks` array (its direct children are
the indent-0 blocks).

**The title and description are properties (§3), not blocks; the icon and the
cover are envelope fields of their own (§2b).** There is no title block in a
document (§7), and no icon block.

Snapshot fields **excluded** from the format:

- `fileInfo` — only present on old-format (deprecated) file objects; export
  drops it, import leaves it empty.
- `relationLinks` — deprecated protocol-wide, scheduled for removal; not
  represented. Property formats are handled via resolvers (§3).
- `removedCollectionKeys` — dropped (meaningful only for change replay, not
  for fresh imports).
- `fileKeys`, `extraRelations` — deprecated in proto.

### 2a. Type documents (`kind: "object_type"`)

A type is not just a schema: it also owns the views over its objects
(columns, filters, sorts — presented through a dataview). Both live on one
object in the underlying model, so the format keeps them in **one
document** — a type never splits across files, and no JSON Schema file is
involved.

```json
{
  "version": 2,
  "kind": "object_type",
  "internal_key": "task",
  "icon": { "format": "icon", "name": "hammer", "color": "orange" },
  "properties": { "Name": "Task", "Description": "…" },
  "type_settings": {
    "layout": "todo",
    "api_key": "task",
    "plural_name": "Tasks",
    "default_template": "bafyrei…",
    "default_view": "table",
    "property_definitions": [
      { "property": "Due date", "name": "Due date", "format": "date",    "section": "featured" },
      { "property": "Assignee", "name": "Assignee", "format": "objects", "section": "featured" },
      { "property": "Status",   "name": "Status",   "format": "select",
        "options": ["Backlog", {"name": "In progress", "color": "blue"},
                    {"name": "Done", "color": "lime"}] }
    ]
  },
  "blocks": [ { "type": "dataview", … } ]
}
```

**Everything that defines the type lives in `type_settings`** — one gated
subtree. Nesting is not tidiness: §2d already put one root `allOf`
conditional on the schema, five more root fields would be five more, the
eval found models putting `type_properties` on non-type documents precisely
BECAUSE the root had no conditionals, and many constrained decoders do not
implement `if`/`then` at all. One group is one conditional, and a per-kind
generated schema includes or omits it in one move. The five settings members
lift from `properties` (their flat spellings — `recommended_layout`,
`api_object_key`, `plural_name`, `default_template_id`, `default_view_type`
— are refused there ON TYPE DOCUMENTS with the repair named; the refusal is
kind-scoped where §2b's and §2d's are unconditional, because `apiObjectKey`
is real data on 9,725 relation documents, where it stays an ordinary
property):

| member | stored key | shape |
|---|---|---|
| `layout` | `recommendedLayout` | the recommended layout of objects OF this type, as a layout name; a stored number outside the vocabulary passes through raw, and an unknown NAME is refused (it would import as a string onto a number detail, silently read as `basic`). |
| `api_key` | `apiObjectKey` | the type's public API key. **`api_key`, not `slug`**: of 1,326 corpus type documents with one, it differs from the document's own spelling in 247 (the `property` type's api key is `relation`, the word the public API kept when the format renamed the concept) — calling it a slug would imply it is the term used elsewhere in the document, which for those 247 it is not. |
| `plural_name` | `pluralName` | the plural display name. |
| `default_template` | `defaultTemplateId` | the object id of the template new objects start from — a scalar: the stored value is a list in every corpus document, with at most one entry (55 of 142; 87 empty), and a second entry is dropped with a warning. |
| `default_view` | `defaultViewType` | the default view type, as a §6.2 view-type name; same raw-number/unknown-name policy as `layout`. |

The five follow the **§4 omit-empty canon** — a `pluralName` of `""` (145
corpus docs) or a `defaultTemplateId` of `[]` (87) says nothing a reader
could act on — unlike §2d's members, which are a property's definition and
mirror presence exactly; the comparator reads the same rule through
`DroppedEmptyTypeSetting` (§11).

The type's REMAINING details (`name`, `description`, `is_hidden`,
`order_id`, …) stay in `properties` under their stored keys (§3); its icon
is the envelope field every object has (§2b) — a type's icon is where the
`icon` variant is overwhelmingly used, since all 1,530 objects in the corpus
carrying an `iconName` are types. The four recommended-property id lists
(`recommended_featured_properties`, `recommended_properties`,
`recommended_file_properties`, `recommended_hidden_properties`) are
**replaced** by `type_settings.property_definitions` — resolved entries,
never raw property ids. The word is `property_definitions` rather
than `properties` because the document already uses that word for property
VALUES at the root, and one word carrying two meanings in one file is the
same shape as the `featured_properties` collision below — one word per
concept.

**A type document does not carry its own install provenance.** Seven stored
keys are omitted on export and dropped on import (stale, not wrong — the
transient-key policy, scoped by kind), each admitted to the drop
individually against 1,760 corpus type documents (§15 #12; the verdicts
live on `typeProvenanceKeys`, and §11 N(S) records the normalization):
`layout` and `resolvedLayout` (ONE distinct value each — "object_type" —
derivable from the kind), `smartblockTypes` (occurs only on installed
copies of bundled types, restating the bundled table), `sourceObject`
(derivable from the type key: `_ot<key>`), `origin` (how the INSTALL
happened — on ordinary objects origin is real provenance and stays),
`addedDate` (epoch-zero on 1,600 of 1,627), and `setOf` — which is the
type document's **own id** on 1,756 of 1,757, re-stamped by
`WithForcedDetail` from the object's id on every init, so it is a function
of the id rather than a fact about the type.

Six candidates FAILED the admission test and stay in `properties`:
`is_hidden` (cannot be proven install-only), `order_id` (the user's own
ordering of types), `layout_width`/`layout_align` (the type object's own
page display, set by a person where non-zero), `featured_properties` —
which means what this type OBJECT features, while `section: "featured"`
means what objects OF this type feature: the two differ in 361 of 400
corpus cases, so they are two things, not one — and **`revision`**, which
was admitted at first and then failed. `systemobjectreviser` short-circuits
on `bundleRevision <= localObject.GetInt64(revisionKey)`; an absent
revision reads 0, the guard stops firing, and the bundled definition is
copied over the local one for `name`, `pluralName`, `recommendedLayout`,
`isHidden` and `relationMaxCount`. Of 1,599 installed bundled type
documents, **40 carry a local name the reviser would overwrite** (key
`relation` is locally "Relation", bundled "Property") and 36 a local plural
name. Dropping it reverts a user's rename on restore, silently.

`property_definitions` entry fields (canonical order):

| Field | Type | Req | Notes |
|---|---|---|---|
| `property` | string | no* | The property's document-facing SPELLING — a key slot like any other, inverted through `property_internal_keys` (§3). Deliberately not called a key: the word used to name this spelling AND the envelope's stored id at once (§15 #14). |
| `internal_key` | string | no* | The property's STORED internal key, verbatim — never run through the §3 ladder, because a stored id is its own address and the bundled fold would rebind a slug-shaped one (`due_date` onto `dueDate`). Export writes it beside `property` for fidelity; an author never needs it, and cannot produce a correct one for a custom property (the app mints those — a bson id). *An entry must state an identity: `property`, or `internal_key`, or a `name` the spelling derives from; when both `property` and `internal_key` are present the spelling wins, and export writes an agreeing pair. A custom property whose entry states no `internal_key` gets a FRESH minted internal key from the import wiring's create path, the way the app mints one when a user creates a property — the spelling must not silently become the stored key. |
| `name` | string | no | Display name. Import uses it only when the property must be **created**; an existing property keeps its own name. Every bundled key already exists, so a name given for one is inert — `{"property": "Description", "name": "Summary"}` renders as *Description*. Validation warns. If the label is the point, mint a custom key instead of reusing a bundled one. |
| `format` | string | no | Property format (§3 names). Same import rule as `name`; a conflict with an existing property's format is an error at the wiring level (the package cannot see the space). |
| `options` | (string \| object)[] | no | A select/multi_select property's **vocabulary, in display order**. Each entry is a bare option name, or `{"name": …, "color": …}` when the option's color is part of the design — the color belongs to the option rather than to a parallel array, so inserting or reordering an option cannot shift it. `color` is one of `grey`, `yellow`, `orange`, `red`, `pink`, `purple`, `blue`, `ice`, `teal`, `lime` (`util/constant`); anything else is a validation error rather than a silently ignored value. The bare string is **canonical** whenever the option declares no color, the object form otherwise — the same rule cells follow in §6.1. Leaving a color out does not mean *no* color: the wiring assigns one, cycling the palette in declaration order and skipping whatever the vocabulary claims explicitly, so a vocabulary that names no colors still gets distinct ones. (The app assigns one at random on every other creation path; cycling keeps a converted bundle identical run to run.) Options are otherwise discovered only from values that happen to be used, so a vocabulary entry no record carries would never exist — its kanban column simply absent — and a discovered option carries no `orderId`, which makes every select sort alphabetically (options order by `[orderId, name]`, `pkg/lib/database.BuildOrderMap`). Declaring them lets the wiring create each one up front with an order id. Every option needs one: the sort concatenates `orderId + name` before comparing, so an option missing an order id is compared by *name* against the others' order ids and lands arbitrarily — ahead of the whole vocabulary when its name sorts below the id alphabet, behind it otherwise. Names discovered from usage rather than declared are ordered after the declared ones. Only meaningful on `select`/`multi_select`; duplicate names are a validation error, across both forms. |
| `object_types` | string[] | no | The **type slugs** an `objects`/`files` property may point at, in priority order — a type-key slot like the envelope `type`, so it speaks the one key vocabulary (§3), claims its spellings through the same type term ledger and owes the same `type_internal_keys` legend; import inverts each entry through the legend first, and a term the chain does not know passes through verbatim. Empty means any object — an untargeted property will happily accept a random page as a task's assignee. Listing the built-in `participant` alongside a bundle's own people type is what makes the current-user filter value usable on that property (§6.2) while still allowing the seeded people as values; the client only offers it when the relation's targets include Participant. The wiring resolves each key to an id the way it resolves properties: a type the batch defines by the id its own document carries, a bundled type by its bundled url (`_ot<key>`). Only meaningful on `objects`/`files`. |
| `description` | string | no | The property's own description (its relation object's `description` detail). Same import rule as `name`: read when the property is created, inert on an existing one. |
| `include_time` | bool \| null | no | Whether a date property's values carry a time of day. Same import rule as `name`; meaningful on `date` only. |
| `max_count` | int | no | How many values the property holds; 0 (or absent) is unlimited, the stored default. Same import rule as `name`. |
| `readonly` | bool | no | Whether the property's value is user-writable. Same import rule as `name`. |
| `default_value` | any | no | The value a new object receives for this property. Same import rule as `name`. |
| `section` | string | no | `featured` \| `hidden` \| `file` — which list the property belongs to. Absent = a regular (sidebar) property. **The one field that belongs to the type rather than the property** (§2e): of 1,614 properties declared by 2+ types within one space, zero differ in anything else. |

An entry is the one `propertyDefinition` shape plus `section` (§2e): the
schema expresses it as a reference to `$defs/propertyDefinition` with a
layer of narrowings (`format` to the authorable vocabulary, `object_types`
to a real array), never as a restatement. The five members after
`object_types` follow the `name` rule — read when the property must be
created, inert on an existing one — and the codec hands the WHOLE decoded
definition to the resolver's create path, so a member the schema admits is
never shed at the seam.

Export emits entries in section order featured → regular → file → hidden,
preserving order within each list, and drops ids that no longer resolve to a
property (including the `_missing_object` sentinel of already-dangling
references); legacy lists that store bare property **keys** instead of ids
resolve through the reverse lookup, falling back to the bundle for system
properties. The canonical form writes `property`, `internal_key`, `name` and `format` on
every entry (`format` defaults to `text` when absent on input), and writes the
`property_definitions` array **even when empty** — its presence is what tells
import to rebuild the lists. Import then rebuilds all four id lists — empty
sections become explicit empty lists, matching how type objects store them —
resolving each entry's identity against the space and creating missing
properties (the same policy as select option names, §3). A document without a
`property_definitions` member leaves the lists untouched.

Property ids are space-local, so the rewrite requires a property resolver
(`Options.ResolveProperties`, §13). Without one, export leaves the four
lists in `properties` as raw id lists, and import passes unresolved keys
through in place of ids for the wiring to reconcile — the same degradation
as option values without an option resolver (§3). A document carrying both
`property_definitions` and any of the four raw lists in `properties` is ambiguous
and fails validation.

**Dataview.** A type's views live in a single dataview block on the type
object itself. When the snapshot contains it, export writes it as an
ordinary `dataview` block in `blocks` (§6.2) — most types customize their
views, so this is the common case and it round-trips losslessly. When the
document has no dataview block, import leaves it absent and the editor
generates the default at first open (from the recommended properties, as it
does today); import never fabricates or rewrites one. Export performs no
"is this the default?" comparison — presence in the snapshot is the only
criterion.

**Derived schemas.** `kind: "object_type"` documents are the canonical type
definition. Per-type validation artifacts — a JSON Schema constraining
objects of that type, a TypeScript-style declaration, a prompt-ready
property table — are **generated one-way** from the type document (planned
`GenerateSchema`, §13) and are never imported or treated as authoritative.
This retires the legacy per-type JSON Schema export (`pkg/lib/schema`) with
its `x-` extension keys.

## 2b. Icon and cover

An object's icon and its cover are each **one envelope field holding one
object**, whose `format` member says which kind it is:

```json
{ "icon":  { "format": "emoji", "emoji": "📕" } }
{ "icon":  { "format": "file", "file": "bafyreicfdcmfn…" } }
{ "icon":  { "format": "icon", "name": "hammer", "color": "orange" } }
{ "icon":  { "format": "color", "color": "teal" } }

{ "cover": { "format": "image", "file": "bafyreigejp…", "y": -0.25 } }
{ "cover": { "format": "color", "color": "black" } }
{ "cover": { "format": "gradient", "gradient": "pinkOrange" } }
```

### `icon` — four variants

| `format` | required | optional | stands for |
|---|---|---|---|
| `emoji` | `emoji` (non-empty string) | `color` | `iconEmoji` |
| `file` | `file` (object reference) | `color` | `iconImage[0]` |
| `icon` | `name` (a built-in icon name) | `color`, `emoji` (output-only) | `iconName` |
| `color` | `color` | — | `iconOption` alone |

- **`color` is on every variant, because it is orthogonal to the source.**
  87 production objects attach one to something other than a named icon — 53
  workspaces and 2 profiles give an avatar image its background colour, 3 an
  emoji — and 29 more carry a colour with no source at all (the letter-avatar
  background; the API reports every one of those as having no icon). Its
  value is one of the ten palette names §2a already mandates for select
  options, mapped positionally from the stored number: `iconOption: n` is
  `palette[n-1]`.
- **`color` also admits a raw integer ≥ 1**, for a stored value the palette
  has no name for. This is not decoration: two generators in this repo
  disagree about the range (`rand.Intn(16)+1` in the pb importer,
  `rand.Intn(10)+1` in the markdown one), so 12, 13 and 15 exist in real
  data. It is the same escape §3 already gives a layout number outside the
  enum. `iconOption: 0` is the proto zero, **not** the first colour — 145
  production objects carry it and none of them is grey.
- **`name` is an OPEN string with a shape rule, not a closed enum.** The
  ~397-name vocabulary lives in `core/api/model/icon.go`, which `pkg/lib` may
  not import, and closing the enum would violate I1 the first time the app
  ships a new icon. All 79 distinct values in the corpus are inside the API's
  set. This is where the design is weakest for an offline generator, and it
  is a deliberate trade against I1 (§15).
- **`emoji` on the `icon` branch is a carry-over, and is output-only (§4a).**
  Exactly 200 production objects hold both an `iconName` and an `iconEmoji` —
  every one a bundled type mid-migration (`Space` 🌎/`folder` ×18, `Type`
  🥚/`extension-puzzle` ×12) — and `format` has already answered which one
  wins, so the emoji is baggage rather than ambiguity. Export writes it with
  a warning; a document that supplies it is not choosing an icon.

**Precedence, when the store holds more than one source:** `iconName` →
`iconEmoji` → `iconImage`. That is `core/api/service/icon.go`'s rule, the
only precedence implementation in heart — every other converter emits all
four channels and lets the consumer decide. See §15 for what is unverified
about it.

### `cover` — three variants

| `format` | required | optional | stored `coverType` |
|---|---|---|---|
| `image` | `file` (object reference) | `source` (`unsplash` \| `prebuilt`, output-only), `scale`, `x`, `y` | 1 / 5 / 4 |
| `color` | `color` (an opaque name) | — | 2 |
| `gradient` | `gradient` (an opaque name) | — | 3 |

The `coverType` relation's own bundled description is this union written as
prose: *"1-image, 2-color, 3-gradient, 4-prebuilt bg image, 5-unsplash image.
Value stored in coverId"*.

- **One `image` branch, not three.** A generator that has just uploaded an
  image has no basis to choose between "image", "unsplash" and "prebuilt",
  and choosing `unsplash` writes a permanent false provenance claim into cold
  storage. `source` carries the provenance and is output-only.
- **`color` and `gradient` are opaque names.** Those two vocabularies live in
  the clients and appear nowhere in this repo, so validation checks the shape
  only. Observed colours: `black`, `ice`, `blue`; observed gradients:
  `pinkOrange`, `red`, `sky`, `blue`, `bluePink`, `greenOrange`. A name
  outside the app's set validates and shows as no cover — the one corner
  where the typed shape cannot do what it exists to do (§15).
- **`cover.color` and `icon.color` share a member name but not a
  vocabulary.** `black` is a cover colour and is *not* in the option palette.
  Read each per variant.
- **Framing (`scale`, `x`, `y`) belongs to an image and to nothing else.**
  In 36,966 objects those three are non-zero only under cover types 1 and 5,
  though they are *present and zero* on colours, gradients and cleared covers
  alike.

### Object references, and the layer that is allowed to fetch

`icon.file` and `cover.file` are **object references**: the id of an image
object in this space, a bundle-local slug, or the `_missing_object`
sentinel. Never a URL, never a filesystem path. The schema enforces it with
`^[^/]+$` — a shape rule rather than a URL-scheme rule, because the compiler
runs Go's RE2, which has no lookahead, and because a slash is what every
unwritable value in 36,966 objects has in common.

**This format does no I/O, so it can only name what the store already holds.**
A URL is a job, not a value. A layer above — the API, the use-case installer
— may extend the schema with a `url` variant, fetch it, mint the file object
and rewrite the value into the plain `file` variant *before* anything reaches
`Unmarshal`. The whole extension is one entry appended to `icon`'s `allOf`
and one value appended to `format`'s enum; the `if` guards are mutually
exclusive by `const`, so nothing already valid is reclassified, and the
reader's own diagnostics name the new variant for free, because the union a
missing-`format` verdict lists is read out of the published schema rather
than restated in code.

That is what the typed shape buys over another flat key. The precedent for
the flat one exists and is unpoliced:
`core/block/import/notion/api/commonobjects.go` writes a raw Notion URL
straight into `coverId` with `coverType: 1`, expecting a later pass to
download and rewrite it — and on 33 production objects that pass never ran,
leaving an absolute path into a temp directory that no longer exists. The
typed variant makes that state unrepresentable at the format boundary.

### Where they live, and why not in `properties`

Four reasons, all forced:

1. **`cover` is already a stored property key**, in 30 production objects,
   with `pageCover` in 66 more — both Notion imports, neither a bundled
   relation. A schema node keyed on a `properties` member could also be
   rebound by the `property_internal_keys` legend to point at an arbitrary relation,
   which is an I1 hole and a laundering primitive. Envelope field names are
   outside the key namespace and immune to the legend.
2. **`properties` carries presence-is-meaningful; the envelope omits empty
   (§4).** Presence-is-meaningful is what generated the noise in the first
   place. All nine relations are `hidden: true` — they have no property row
   for presence to be meaningful *to*.
3. **It closes a gap §4a recorded and could not fix**: `coverId`/`coverType`
   were output-only with no schema node of their own to annotate. They have
   one now.
4. **It fixes a live readability bug.** 54 production objects hold both the
   bundled `iconEmoji` (empty) and a space-minted relation whose *stored key*
   is literally `icon_emoji` (holding, in one real space, `"☕"`). Anything
   reading "the icon" out of `icon_emoji` in those documents reads a
   coffee-tasting note. After the lift the document carries `"icon": {…}` at
   the top and `"icon_emoji": "☕"` in `properties` — visibly two different
   things.

The precedent is not `id`/`type`; it is **the §2a property list** (`type_properties` then, `type_settings.property_definitions` now): stored
keys lifted into one labelled envelope member, with the flat spelling refused
where it used to sit.

### The nine spellings are refused in `properties`

`iconEmoji`, `iconImage`, `iconName`, `iconOption`, `coverId`, `coverType`,
`coverScale`, `coverX`, `coverY` — under any spelling that RESOLVES to one of
them (§3), the legend included — are refused, and the refusal names the
repair:

```
/properties/Emoji: "iconEmoji" is written as "icon": {"format": "emoji",
                   "emoji": "…"} (§2b), not as a property
```

The refusal is **derived** from the export side's own lift list, never
restated: a restated list is how the two surfaces drifted apart the last
time (§3, `deniedPropertyKey`). It is unconditional rather than conditional
on the typed field being present, because there is no second way to write an
icon — a format with two legal spellings for one thing, one of which a small
model has seen far more of in training data, defeats the whole point.

Resolution on the *stored* key is what keeps the 54 dual-key objects above
working: their space-minted relation resolves to the stored key `icon_emoji`,
not `iconEmoji`, so it is an ordinary property and sails through.

### The same shape elsewhere

A **callout block** (§5) and a **bundle index** (§2c) carry the same `icon`,
restricted to the two variants a block or a bundle can hold (`emoji`,
`file`) — one `$ref`, narrowed by an enum, not a second definition. Shipping
the envelope field without them would leave two icon conventions inside one
document, which is the defect being removed.

## 2c. The bundle index (`index.json`)

Every document described so far is one object. **A bundle also needs to say
things about itself** — what the space is called, what opens when a user
enters it, what the sidebar shows — and none of that belongs to any single
object. That is `index.json`, one file at the bundle root, validated against
`index.schema.json`:

```json
{
  "$schema": "https://schemas.anytype.io/anyblock/2/index.schema.json",
  "version": 2,
  "name": "Company Wiki",
  "description": "Everything we know, with an owner.",
  "icon": { "format": "emoji", "emoji": "📚" },
  "homepage": "page-wiki-home",
  "widgets": [
    { "target": "page-wiki-home" },
    { "target": "type-wiki-page", "layout": "view", "limit": 6 },
    { "target": "_favorite", "layout": "compact_list" },
    { "target": "_all_objects", "card_style": "card", "icon_size": "medium" }
  ]
}
```

| Field | Meaning |
|---|---|
| `name` · `description` | the space's own identity, applied on install |
| `icon` | the space's icon, in exactly the shape an object's icon has (§2b), restricted to the two variants a bundle can hold: `{"format": "emoji", "emoji": "📚"}`, or `{"format": "file", "file": "<object id of an image in the bundle>"}`. The image variant needs the image object *and* its file in the archive, so a generated bundle uses an emoji. It is one `$ref` into the object schema, not a copy — an index and an object cannot disagree about what an icon is. |
| `homepage` | what opens on entering the space: an object id, or the reserved `_widgets` (the sidebar dashboard, the default) or `_graph` |
| `widgets` | sidebar widgets, in order. **The first one is what the install opens**, so the entry point goes first |

`version` is the same format version, with the same rules, that object
documents carry (§10): one integer, one namespace, bumped together. A reader
rejects an index declaring a version newer than its own, naming both — the
same dedicated error object documents get, never a generic constraint failure.

**A bundle is one artifact and is versioned as one.** If the index or *any*
document in it declares a version the reader does not support, the bundle is
rejected as a whole and **nothing is installed** — a bundle creates a space,
installs types and widgets and unpacks files, so a partial install leaves a
space half-built with no way for the user to tell what is missing. (A
conversion or validation *tool* may keep going to report every offending file
at once, as `anyblockconvert` does, but it must still fail the run rather than
present its output as usable.) In a well-formed bundle every file carries the
same `version`; a bundle whose files disagree is malformed, and the reader
gates on the highest version it finds.

A widget is flat — `{ target, layout, limit, view_id, auto_added,
card_style, icon_size, description, properties }` — though the wire carries
it as two blocks (see below). The members are the two blocks' own §5
members, verbatim: `layout` (`link · tree · list · compact_list · view`,
defaulting to `link`), `limit`, `view_id` (which of the target's views a
`view` widget shows; omitted, the target's default view) and `auto_added`
(the client placed this widget itself and treats it as its own to manage)
are the widget block's; `card_style`, `icon_size`, `description` and
`properties` are the link block's display members, with the same
vocabularies — the schema states each as one `$ref` into the object schema
rather than a copy that drifts. `properties` keys resolve through the
bundle's property dictionary (§2f), the file that answers for stored keys,
since there is no per-document legend here.

`target` is an object id from the bundle — a page, a type, a set, a
collection — or one of the eight reserved listings `_favorite · _recent ·
_recent_open · _set · _collection · _all_objects · _chat · _bin`, which name
a built-in rather than something the bundle ships. The leading `_` is what
keeps the two kinds of target apart (§1): an object id from the bundle may
never begin with one, so a bundle cannot shadow a listing with an object of
its own, and a reader never has to guess which of the two a target meant.
The inventory is what live sidebars actually hold — measured over a 77-space
account, 33 of 218 widgets name a listing (chat 11 · bin 10 · allObjects 8 ·
recent 1 · set 1) — and `widget.IsPredefinedWidgetTargetId` knows every wire
spelling, so all eight survive import.

Two index-level members belong to the sidebar without belonging to any one
widget, and both are machine state the authoring subset refuses (§2g):
`auto_widget_targets`, the client's ledger of targets it has already
auto-added a widget for — usually naming widgets NOT in the sidebar any
more, which is the point: the ledger is what stops a restored client
re-adding what the user deleted (21 of 77 spaces carry one) — and
`auto_widget_disabled`, the per-space switch that turns auto-widgets off
entirely (2 of 77).

A `_`-prefixed target that is not one of the eight is refused by name, with
the inventory in the message. It cannot be an object id, so the alternative
diagnostic — "no object with that id in the bundle" — would point an author
with a typo at the wrong repair.

**A bundle carries no widget document.** The sidebar of a live space is a
hidden `kind: "widget"` object whose blocks encode exactly this array: one
widget wrapper block per widget, each with one indented link child naming
the target. Measured over 77 spaces the encoding is pure scaffolding — 218
wrapper blocks, 218 link children, in perfectly regular pairs, plus the
header scaffolding §7 already drops — and every detail the object carries is
either lifted here (`autoWidgetTargets`, `autoWidgetDisabled`), constant
(`isHidden`, the dashboard layout), or the object's own timestamps, which a
restored sidebar re-mints the way a restored space re-mints its own.
So export lifts the object into these fields (`IndexFromWidgetObject`) and
omits the document — fail-closed, like the space document beside it: an
unpaired widget block, a target the index cannot spell (two strays in the
corpus, `bookmark` and `lists`, words no client defines), a non-empty name,
real page content, or any detail this package cannot account for keeps the
document, so a widget object carrying something unforeseen travels rather
than vanishing. The comparator consults the same predicates
(`OmittedWidgetObject`, `WidgetObjectResidualKey`), so the omission and the
round-trip check cannot drift apart (§11).

### The manifest

The index also says **where to find what a reader must resolve by key or id
rather than by walking**. The format defines no folder layout — `objects/`,
`types/`, `files/` are one exporter's convention (below) — and an object
names its type by *spelling* alone, so without a manifest a reader resolves
a type by scanning every document for a matching `internal_key`, and a file
document's bytes by guessing at a layout.

```json
{ "manifest": {
    "types":      { "Task": "types/bafyrei….anyblock.json" },
    "properties": "properties.json",
    "files":      { "bafyreigp3him…": "files/bafyreigp3him….png" } } }
```

- **`types`** — the type's CANONICAL SPELLING → the type document's path.
  The canonical spelling, not a per-document term: the display name from
  the shipped table for a bundled type, the stored key verbatim for a
  space-minted one — a pure function of the key, the same rule the
  dictionary applies to its own entry keys (§2f), because the index has no
  legend and its keys must resolve through the shipped ladder alone (an
  exact stored key names itself, then the name table, then the fold). A
  reader inverting a document's own spelling still goes through that
  document's `type_internal_keys` legend first, as everywhere.
  The manifest does NOT locate options. A manifest exists
  to answer a lookup a reader would otherwise have to scan for, and no reader
  has that lookup for an option: the dictionary states a property's whole
  vocabulary inline — each option's name, colour, position and, since the
  vocabulary learned `internal_key`, its stored key (§2f) — so everything an
  option MEANS is in hand before a single document is opened. The map was
  2,641 entries across a 77-space export, pointing at documents nothing
  needed to read.

  `option_ids` is unaffected and does a different job: it carries option
  OBJECT ids, resolved against the IMPORTING space's live store so a value
  survives a rename (§9a), never against the bundle. It never needed a path
  beside it.
- **`properties`** — the property dictionary's path (§2f). A pointer rather
  than an inline map, because properties resolve through each document's
  own legend and the dictionary is the file that answers for the keys those
  legends bind.
- **`files`** — file object id → the blob's path, one entry per
  file document whose bytes travel. The authoritative binding between a
  `kind: "file_object"` document and its bytes: the document itself carries no
  path, because a document member is not a slot for archive bookkeeping —
  the lesson of the legacy `source`-clobber, which overwrote a real,
  editable `url` relation that bookmarks legitimately hold, and whose
  destruction round-tripped through import. Every importer holding a file
  document must find its bytes and every export tool must enumerate them —
  and the map has that reader wired: `cmd/anyblockconvert` copies each
  binding into the installable archive and writes the archive-side
  `source` detail from it (the pb importer's own contract, resolved by
  `normalizeFilePath`), and the future production native importer resolves
  the same map. The clobber the format banished from its DOCUMENTS is
  legitimate at the archive boundary — the archive is a transport
  artifact, not a document.
  Keys are object ids verbatim — no re-spelling on either side — and an
  authored bundle writes `"files": {"logo": "assets/logo.png"}` against its
  own minted ids and any layout it likes, which is what makes an authored
  bundle a first-class citizen rather than a convention-follower. The
  tooling's contract, precisely: an entry that cannot be honoured — a key
  naming no document, a path escaping the bundle, a blob missing at the
  path — is a cross-document REFUSAL; a file document a present map does
  not bind is a WARNING (its bytes did not travel — the exporter writes
  the document and omits the binding when a blob cannot be streamed, and
  counts it, so a partial export is loud at both ends); a bundle with no
  map at all is a metadata-only export, tolerated as a mode.
  The bundle is FAT (§15 #20): the bytes travel, nothing
  else — no variant keys, no encryption keys, and the thin bundle's future
  marker slot stays untouched.

Paths are relative to the index file. The reader flow, with no scanning and
no folder convention: object → `type: "task"` → the object's legend → stored
key → manifest → the type file → `property_definitions` → property not
there → manifest → the dictionary → the entry; a file document → `manifest.files`
→ the bytes.

**This exporter's convention** (the "one exporter's convention" slot,
recorded so a reader of OUR bundles knows the layout without reverse-
engineering it; none of it is format — a reader must still walk and index,
because an authored bundle may put documents anywhere):

- One bundle root per space; a multi-space export wraps each in
  `spaces/<spaceId>/`, and the wrapper is load-bearing — the same id
  legitimately recurs across spaces (448 cross-space repeats measured,
  chiefly participant identities), so flattening the wrapper collides.
- Every document is `<dir>/<id>.anyblock.json`, the id verbatim — id→path
  is a pure function of the reference itself, which is the naming
  decision's whole point: a reference carries an id and nothing else, so
  any name-bearing filename would force a scan. The kind directories are
  `objects/` (kind `page`, flat — plus any kind without a dedicated home),
  `types/`, `templates/`, `properties/` (kept `property` documents only;
  the rest are omitted into the dictionary, §2f), `options/`,
  `participants/`, and `files/`.
- `files/` holds both halves of a file, adjacent: the document at
  `files/<id>.anyblock.json`, the blob at `files/<id>.<ext>` with `<ext>`
  the stored `file_ext` lowercased and restricted to `[a-z0-9]{1,10}`,
  else the conventional extension for `file_mime_type`, else `bin` —
  a blob wearing the document extension is refused at plan time, and
  the manifest map above, not the adjacency, is what binds them.
  Putting file DOCUMENTS in `files/` is safe against the one reader known
  to skip that directory: the legacy pb importer
  (core/block/import/pb/converter.go) skips `files/` while walking a PB
  archive — but it also parses every other `.json` file as jsonpb, which
  refuses every AnyBlock document loudly, in every directory. A native
  bundle fed to it fails whole, never partially; there is no path on
  which the skip silently drops just the file documents while the rest
  imports. Native bundles are read by the native wiring
  (`cmd/anyblockconvert` → the experience path), which walks everything.
- `index.json` and `properties.json` sit at the bundle root; there is no
  `profile` file and no root `index.<ext>` home special case — the
  homepage is a field of `index.json` and the home object an ordinary
  document under `objects/`.

The manifest is optional — a bundle without one is walked the way every
bundle was before it existed — and closed: the index root already refuses
undeclared members (`additionalProperties: false`), and the manifest extends
that inside itself, because an undeclared lookup table is one no reader
opens. Whether its paths resolve is the same cross-document question every
other id in the index poses, answered by the tooling, not this package
(§13).

### How it reaches the space

A bundle is installed with `ObjectImportExperience`, which reaches
`builtinobjects.CreateObjectsForExperience`. That is a different path from the
one the built-in use cases take (`inject`), and it reads much less. The two
outputs the wiring produces, and who reads them:

| output | written by | read by |
|---|---|---|
| `profile` at the archive root — `pb.Profile`, raw protobuf whatever format the snapshots are in, since `getProfile` reads it with `pb.Profile.Unmarshal` | `cmd/anyblockconvert` (`profile.go`) | `CreateObjectsForExperience` reads **`name`, `avatar` and `spaceDashboardId`** — on a NEW-space install; installing into an existing space reads none of it (the whole read is gated on `isNewSpace`) |
| a snapshot with `sbType: Widget` among the objects — one root block plus a wrapper-and-link pair per widget | `cmd/anyblockconvert` (`widgets.go`), built from `index.widgets` by `WidgetsSnapshot` — the same function the round-trip verifier holds against the widget object it omits, so the install artifact and the loss check cannot drift | the pb importer: `shouldImportSnapshot` admits a Widget snapshot precisely when the import type is `EXPERIENCE`, and `objectcreator.updateWidgetObject` merges its widgets into the space's own widget object |

| `index.json` | reaches the space as | effect |
|---|---|---|
| `homepage`, falling back to `entrypoint` | `profile.spaceDashboardId` | the space's `homepage` detail — what opens on **every** entry, and on this path the only thing that decides what a new user sees |
| `widgets` | the Widget snapshot's root children, in order | the sidebar |
| `entrypoint` | `profile.widgets[0].targetObjectId` | the object the install opens **once** — on the `inject` path only. On a bundle's own path it lands only through the `homepage` fallback above |
| `name` | `profile.name` | the space's own name, when the install CREATES the space; nothing on an install into an existing space |
| `icon` (the `file` variant) | `profile.avatar` | the space's icon (the file object's id re-mapped to its imported id), under the same new-space gate |

Five consequences worth stating, because none is obvious from the wire format:

- **`profile.widgets` is inert here.** On a bundle's own (pb) path
  `CreateObjectsForExperience` never calls `getWidgets` or `createWidgets`;
  those belong to `inject`. (Its Markdown/AI branch DOES call
  `createWidgets` — one link widget built from the manifest's dashboard
  page, not from `profile.widgets`, which stays unread there too.) The
  wiring still fills the field, so an archive it produces is also a valid
  built-in archive, but nothing on a bundle's own path reads it. **The
  sidebar comes from the Widget snapshot** — which an app export carries as a stored
  object, and which this format's wiring rebuilds from `index.widgets`,
  since a bundle carries no widget document (above). The snapshot's
  `autoWidgetTargets` / `autoWidgetDisabled` details are inert the same way:
  `updateWidgetObject` merges only the BLOCKS into the space's own widget
  object, so the ledger reaches the archive truthfully but no importer reads
  it back yet. The index is where the state survives.
- **`name` and the icon land only on a NEW space.**
  `CreateObjectsForExperience` calls `setWorkspaceSettings(profile, spaceId,
  true)` — but only inside its `isNewSpace` gate, so the profile's name and
  icon become the created space's own identity and can never overwrite a
  name the user already chose: an install into an existing space skips the
  profile read entirely.
- **`entrypoint` is encoded as the first widget.** There is no independent
  field for "open this after import" — `inject` takes
  `widgets[0].targetObjectId` as its starting page, and the deprecated
  `startingPage` is only read when `widgets` is empty, so it cannot coexist
  with a sidebar. The wiring therefore has to make the entrypoint
  `widgets[0]`, prepending a widget for it when the author listed something
  else first. The entry point consequently always appears first in the
  sidebar. `entrypoint` exists as a separate field anyway, because expressing
  it by sorting `widgets` means reordering the sidebar silently changes what
  a new user sees.

  On a bundle's own path even that does not fire: `CreateObjectsForExperience`
  computes no starting page and `ObjectImportExperience` returns none, so
  nothing is opened once. What a new user lands on is the space `homepage` —
  which is why an omitted `homepage` falling back to `entrypoint` is what
  makes the field mean anything at all here.
- **Omitting `homepage` does not mean the widgets screen.** An absent
  `spaceDashboardId` makes `setWorkspaceSettings` default to `widgets`, which
  is the right default for a *blank* space and the wrong one for a use case:
  on desktop the widgets are already in the sidebar, so it leaves the main
  pane empty. So an omitted `homepage` resolves to the `entrypoint` instead,
  and only an explicit `"_widgets"` or `"_graph"` gives up a real page.

**A widget target that does not resolve loses the widget, silently.** This is
the only reference in the format whose failure produces no diagnostic at all:
`common.handleLinkBlock` rewrites a link target it cannot resolve to
`addr.MissingObject`, and `WidgetObject.Init` then removes the broken link
*and* its now-empty wrapper. The import succeeds, the widget is not there, and
the only trace is a log line. An id no document in the bundle defines is
therefore an error in `anyblockvalidate` and `anyblockconvert` rather than
something an author discovers by installing — and so is a reserved listing
the importer does not recognise, a set that is empty today (the importer
knows all eight) and that the batch checker still guards, for the day a
listing is added to this format before the importer learns it.

Nothing per-object substitutes for this file. In particular **`isFavorite` is
not an entry point**: it adds an object to Favorites and nothing more. It
does not open anything, create a widget, or set the homepage.

Ids in `index.json` are the bundle's own — the same slugs every other
document uses — and the wiring relinks them like any other reference. Whether
they resolve is a cross-document question this package does not answer
(§13): an index validates on its own terms while naming an object no
document defines.

## 2d. Property documents (`kind: "property"`, `"bundled_property"`)

A relation object IS a property definition, and its document states what it
defines in **`property_settings`** — one `propertyDefinition` (§2e), in the
format's own vocabulary:

```json
{
  "version": 2,
  "kind": "property",
  "id": "bafyrei…",
  "internal_key": "budget",
  "property_settings": { "format": "number", "include_time": false },
  "properties": { "Name": "Budget", "Description": "Planned spend" }
}
```

The three members are grouped rather than sitting at the document root,
because the
dictionary entry and a type's property-definition entry are groups holding
the same shape and two patterns for one idea is the §15 #14 disease one
level up. The group is a layer over `$defs/propertyDefinition`: the members
another surface already owns are refused with the home named (`internal_key`
is the envelope's — and `property`, the spelling, is refused with it: a
property document is addressed by its stored key and its spelling is derived,
never stated — `name` and `description` are `properties`', `options` are
property_option documents), and `max_count`/`readonly`/`default_value` keep
travelling in `properties` under their stored keys until the dictionary
lifts them — admitting a second spelling of any of those here would
reintroduce exactly the duality this section removed.

**Two kinds are property documents**, and both carry the group: `property`
and `bundled_property`. Only the first comes out of a live store — 0 of
38,061 corpus documents are the other — but the `kind` enum offers it beside
`property` with nothing marking it non-authorable, and a small model
authoring from the schema alone picked it unprompted, walking straight back
into the bug this section exists to stop. The export gate, the import gate
and the schema's `if` therefore name the same two: a half that lifts for
fewer than the schema validates for emits a document its own Validate
rejects (§11 I1), and a half that reads back fewer drops the definition.

Two neighbouring kinds are deliberately outside the set. `property_option`,
because an option document is a value rather than a property definition, so
`format` there is an ordinary custom property key. And **`sub_object`,
because it is deprecated** — a kind being retired must not acquire a new
obligation in a format about to freeze. Nothing observable turns on it (0
corpus documents either way); what turns on it is not extending support to
something on its way out.

`format` here may name **every** format a store carries, including `map` —
the shape of a hidden system relation's value, whose only carrier is the
bundled `templatePlaceholders` (72 production documents). The two AUTHORED
format slots may not: `property_definitions[].format` and a dataview's
`properties[].format` reference `authorableFormat`, which is the same
vocabulary minus `map`. A relation document has to be able to say what it
defines; a type or a view has no business declaring a property whose values
only the client writes, and none does — 0 of 19,862 type-property entries
and 0 of 28,034 dataview property entries carry it.

Exactly **three stored details lift**, and no others:

| stored key | `property_settings` member | shape |
|---|---|---|
| `relationFormat` | `format` | a §3 format NAME — **required**. Export refuses to write a relation whose stored format it cannot name (corrupt data only: `formatNames` is total over the model enum, test-pinned), because the fallback — writing `"text"` for a format that is not text — would import as a permanent silent format rewrite, the exact disease this lift kills. `"text"` resolves per key on the way back in, through the envelope `internal_key`, exactly as a property-definition entry's format does (§3): a bundled short-text relation keeps its stored format across a round trip. |
| `relationFormatIncludeTime` | `include_time` | `true` \| `false` \| `null`. Meaningful on `date` only; a `true` against any other format is a **warning**, carried unread. |
| `relationFormatObjectTypes` | `object_types` | the target **type keys**, in priority order — a type-key slot exactly like `property_definitions[].object_types` (§2a): the §3 type vocabulary, the same term ledger, the same `type_internal_keys` legend. Non-empty against a format other than `objects`/`files` is a **warning**. Meaningful entries: `[]` is a cleared target set, `null` a stored null. |

**Presence mirrors presence.** Each member is present exactly when its
stored key is present, and carries its value — `false`, `[]` and `null` all travel
(80 production relations hold a null `includeTime`; 8,903 hold an empty
target list). This deliberately stops the §4 omit-empty canon at these three
members, and it is the opposite of §2b's emptiness carve-out, for a reason
worth stating: these fields are the property's *definition*, not decoration,
and §15 #14's verdict was to fix the SPELLING and leave the emptiness
collapse to its own change. Mirroring is what makes the lift a pure spelling
change — the same details go in and out, so the snapshot comparator
(snapshotdiff) needed **no new rule**, where §2a and §2b each cost one.

**Why the envelope and not `properties`.** Inside `properties` every key is
a property spelling, so a bare `format` there means "a custom property named
format" — and that is a measured live bug, not a hypothesis: in a 198-run
small-model eval, 9 of 9 attempts wrote `properties: {"format": "number"}`,
it validated with no warning, and it imported as a phantom property, leaving
the relation with no `relationFormat` at all — longtext forever, silently.
The container was the problem, not the word. The §2b reasons apply
unchanged; the schema gates the group on `kind: "property"` and keeps it
illegal at every other root, so the same member name cannot be reclassified
by kind drift.

**The three spellings are refused in `properties`** — under any spelling
that RESOLVES to one of the stored keys (§3), legend included, on every
kind, with the repair named:

```
/properties/Format: "relationFormat" is written on a property
                    document's envelope as "format": "<a §3 format
                    name>" in property_settings (§2d), not as a
                    property
```

(`"Format"` is the stored key's wire spelling — its display name, §3; the
stored key `relationFormat` written verbatim trips the same refusal. The
retired slugs — `relation_format` and `property_format` — resolve
to nothing any more: a denied key's fold class answers nothing, so they are
ordinary custom keys that cannot trip it.)

The refusal is derived from the export side's own lift list, never restated
(§2b's rule), and it is unconditional: a non-relation snapshot carrying one
of the three details drops it with a warning, because there is no §2d member
off a relation document to carry it — never observed, 0 of 27,444
non-relation documents. And on a relation document, a `properties` member
spelling one of the three MEMBER names (`format`, `include_time`,
`object_types`) is a **warning**: it names a custom property, not the
relation's own definition — the phantom shape the 9-of-9 eval failures
wrote, which with the group's member also present would otherwise validate
in silence. A warning and not a refusal because the spelling is a legitimate
custom key (a media space really can have a `format` column) and a relation
object carrying one must stay exportable (I1). Refusing a key is not refusing to NAME it: a slot
that references the relation — the Property type's own property definitions and
dataview columns, in 64 production spaces — keeps the §3 spelling
(`"Format"`, the display name), because the deny rule protects the legend
and a bundled-bound spelling needs no legend entry.

**Target types translate at the boundary.** The store keeps
`relationFormatObjectTypes` as type OBJECT ids
(`objectcreator.fillRelationFormatObjectTypes`); the document spells type
keys. The translation is the optional `TypeResolver` capability of
`Options.ResolveProperties` (storeresolver implements it from the same one
bounded type listing the §3 vocabulary budgets): export inverts id → key, import key →
this space's id, so a resolver-wired round trip is id-exact. A bare key
legacy imports stored directly (21 production entries) passes through
**verbatim in both directions**, its own address (§3): a key is vocabulary,
and a vocabulary miss is never evidence of nonexistence. What no longer
passes is an entry the space's own store disowns (§9): the
`_missing_object` sentinel, and an object id the wired existence capability
says names no row — 56 production properties carry one, type ids from the
account where a shipped use case was AUTHORED, and an object id differs in
every space while a type key does not. Both drop, the real id with a warning
naming it; `object_types` is a list, and a list expresses absence by being
shorter. Note the store answers for CORPSES too: an uninstalled type still
has a row and still inverts through `TypeKeyById` (its id names something),
so only an id with no row at all drops. Without any resolver the whole list
passes through verbatim and the offline round trip is byte-exact — an id
the store merely could not be asked about is still the stored value's
meaning, and a backup format that deleted it on export would be
disqualifying.

Corpus facts the design rests on (38,061 documents, 10,617 relation
documents): every relation document carries `relationFormat`, so requiring
`format` refuses nothing real; the format distribution covers 14 of 15 enum
values (everything but `relations`=101), including `map`=102 on 72 documents
— all the bundled `templatePlaceholders` relation — which is why the §3
vocabulary gained the name; `include_time` is `true` only on dates (543),
`object_types` non-empty only on objects/files (1,089 + 167); target entries
are 1,301 ids, 21 bare keys, 9 `_missing_object`.

## 2e. One property, one shape (`propertyDefinition`)

A property is described by **one shape**, `$defs/propertyDefinition`,
wherever this format describes a property. The shape has exactly **three
homes**, and no fourth:

| home | shape |
|---|---|
| a property-dictionary entry (§2f) | one `propertyDefinition` |
| a type document's property-definition entry (§2a) | one `propertyDefinition` + `section` |
| a property document's definition fields (§2d) | one `propertyDefinition` |

The shape's eleven members: `property`, `internal_key`, `name`, `format`,
`options`, `object_types`, `description`, `include_time`, `max_count`,
`readonly`, `default_value`. The first two are one identity split into its
two concepts — `property` the document-facing spelling, `internal_key` the
stored id the app mints — because one word carrying both meanings is the
§15 #14 disease this shape existed to end (§2a's entry table has the rules). It states no `required` of its own and stays open — each
home layers over a `$ref` to it, adds its own requirements, narrows what it
must, refuses the members another surface already owns, and closes itself
with `unevaluatedProperties: false`. A home may **narrow** a shared member
(an authored home pins `format` to `authorableFormat`; a type's
`object_types` is a real array, since only a relation's stored value can
hold a null) but never restate its shape: two statements of one member
agree today and drift tomorrow (§15 #14).

The rule is test-pinned the way the format vocabulary is: the homes are
asserted to REFERENCE `$defs/propertyDefinition`, the way
`authorableFormat` is asserted to be `propertyFormat` minus `map` rather
than restated. On the Go side the codec threads the whole decoded
definition to the resolver's create path through one builder shared by both
doors the §2a array arrives by (the document, and the PATCH-type channel),
so a member the schema admits cannot be silently shed at the seam — a
document that validates and then quietly means less than it says is worse
than one the schema refuses.

## 2f. The property dictionary (`properties.json`)

A bundle names every property its objects use in **one file**,
`properties.json`, at the bundle root beside `index.json` and validated
against `properties.schema.json`. It is a sibling of the index and not a
section inside it, deliberately: an index says *where* things are, a
dictionary says *what they mean* (the manifest
belongs in the index because a manifest is what an index is).

```json
{
  "$schema": "https://schemas.anytype.io/anyblock/2/properties.schema.json",
  "version": 2,
  "installed": ["Creation date", "Due date", "Tag"],
  "properties": [
    { "property": "6a32d4856761631534b22f85",
      "internal_key": "6a32d4856761631534b22f85", "name": "Budget", "format": "number" },
    { "property": "693c14f2aa11631534b22f01",
      "internal_key": "693c14f2aa11631534b22f01", "name": "Owner", "format": "objects",
      "object_types": ["Space member"] },
    { "property": "Due date", "internal_key": "dueDate", "name": "End Date", "format": "date" }
  ]
}
```

**Why it exists, measured.** 10,617 of 38,061 corpus documents are
property documents (`kind: relation` when measured, `property` now)
— 5.8% of the bytes — and 9,675 of them are installed
copies of the 194 bundled relations, **98% field-identical to
`bundle/relations.json`**. Each spends a ~967-byte document, with its own
envelope, attribution and system properties, to restate `{key, name,
format}` a table every reader already ships. The dictionary replaces those
restatements: export omits a bundled relation document whose definition
matches the table (§11), and one key in `installed` stands for it.

Two members:

- **`installed`** — the BUNDLED properties present in the space, spelled by
  canonical name (§3): presence, not definition. A restore reinstalls each key
  from the reader's own bundled table. An installed copy that DIVERGES from
  the table — a rename, a changed `is_hidden` (174 of 9,675 in the corpus:
  132 by `is_hidden` alone, 8 real renames) — keeps its relation document
  AND gets a full entry here, which overrides the table member for member. A
  key the reader's table cannot name is **skipped, not refused**: the
  bundled table grows independently of the format version, so a backup
  written by a newer app must stay readable one app version back. (The
  writer has no such excuse — `MarshalPropertyDictionary` refuses a key its
  own table cannot name, since it would tell the reader to install nothing;
  the repair is a full entry, where the format travels along.)
**Precedence, when a property is described more than once.** The composition
puts a property's definition in up to three places at once — a dictionary
entry, a kept property document's `property_settings`, and a type's
`property_definitions` — and 273 kept bundled-key relation documents in a
77-space export also carry an entry, so the pair is ordinary rather than
exotic. The order is:

1. **The bundled table**, for a key it names. It ships with every reader and
   is the same in every space (§3), so no document can redefine a
   bundled property — an entry for one DOCUMENTS it, and the tools warn when
   the two disagree rather than accepting the entry in silence.
2. **The dictionary entry**, for every other key. It is the bundle-wide
   statement, and the one an author writes when there is no relation
   document at all.
3. **A type's `property_definitions` entry**, which narrows nothing and adds
   only `section` — what THIS type does with the property, not what the
   property is.
4. **A kept property document's `property_settings`**, which is the same
   `propertyDefinition` and should agree by construction; where it does not,
   the dictionary is the bundle's answer.

The redundancy is deliberate: a type document is a self-sufficient authoring
unit (§2a), and the dictionary is what a bulk reader consults (§15 #14).

- **`properties`** — one `propertyDefinition` (§2e) per property the
  bundle's objects actually REFERENCE. **Used-only, not
  everything installed**: a space installs a median 125 bundled properties
  and uses 57 (47%), and the 68 nothing touches buy a reader nothing a
  restore does not already provide. Space-minted properties appear here in
  full — the dictionary is where an author declares a property without
  writing a relation document at all, in the same vocabulary as a type's
  `property_definitions` entry.

**The dictionary ANSWERS for stored keys, and its entries carry both
halves of the identity.** A document spells a property by its display name;
its `property_internal_keys` legend binds the spelling to the stored key;
the stored key is what the dictionary answers for. An entry states that key
as `internal_key`, verbatim, and its `property` in the CANONICAL SPELLING
every other slot uses — the display name from the shipped table for a
bundled key (`"Due date"`, and `"Format"` for the key stored as
`relationFormat`), the stored key verbatim for a space-minted one (nothing
is ever derived from a bson id, and the dictionary has no legend, so its
spelling must be a pure function of the key — the only pure spelling a
space-minted key has is itself).
Entries need no legend of their own: an `internal_key` never resolves at
all, and a `property` spelling recovers its stored key through the ladder
below. An author states any one identity — `property`, `internal_key`, or a
`name` — and a custom property with no `internal_key` gets a fresh minted
one from the import wiring, like everywhere else in the format (§2a).

**The reader flow, in full, and the step that is easy to miss.** A spelling
resolves in this order — the document's own `property_internal_keys`
legend; then a verbatim match against a dictionary key; then **the shipped
name table over the dictionary's own keys** (the same table every §3 slot
resolves through), with the forgiving fold behind it for near-misses and
legacy derived-slug spellings. That third step is not optional
garnish: §3's exhaustive rule writes a legend line only for a spelling the
bundled table does not bind, so a bundled property's spelling never gets
one — measured before the re-spell, over a produced 77-space export of
503,919 property value slots, 69.4% of slots resolved only through the
shipped table's step.
**Look up, never transform** — the name and the key say different words
("Creation date" / `createdDate`), so no derivation in either direction
exists; a reader holds the shipped table and asks it.

**Every entry carries its `format`, and the schema requires it.**
Self-sufficiency is the constraint that shapes the dictionary: a
third-party reader must be able to interpret a backup WITHOUT shipping
`bundle/relations.json` — tell a date from a string, an option name from
free text. Dropping bundled relation documents with *no* dictionary was
considered and rejected for exactly this reason; it is the same "stands
alone" property that keeps a space id off the envelope. (`installed` keys
carry no format because they are not interpretation, they are restore
instructions — the definitions a reader interprets by are the entries.)
`format` resolves per key exactly as everywhere else (§3): `"text"` on a
bundled short-text key stays short text.

A dictionary entry is the **third home** of `$defs/propertyDefinition`
(§2e), referenced across files by the published URL the way the index
references `plainIcon`, and closed with `unevaluatedProperties`. Its layer
narrows `object_types` back to a real array — only a relation's STORED
value can hold a null (§2d), and a dictionary describes a property rather
than mirroring a store slot. `section` is refused: it is the one type-owned
member, meaningless off a type document. One key, one slot: a key stated
twice — in `installed` or in `properties` — is refused on read and on
write alike, with the first occurrence named.

`version` follows the same rule as every other file in a bundle (§2c, §10).

The tooling knows this is not an object document, the way it knows
`index.json` is not: `anyblockbatch.DiscoverJSONFiles` excludes it,
`anyblockvalidate` validates it against its own schema (and warns — tools
warn, the codec tolerates — on an `installed` key the local table cannot
name), and `anyblockconvert` reads it as a declaration source (§3, the
import wiring).

## 2g. The authoring subset (`authoring/*.schema.json`)

The three published schemas serve two audiences at once, and they pull in
opposite directions. One is a **backup**: a full-fidelity round trip of a
real space, which needs ids, attribution, legends, minted internal keys,
provenance, derived state. The other is an **author** — layer 2 of §1,
increasingly an LLM agent — generating a use case from nothing: a few types,
some properties, a handful of objects. An authoring agent reading the full
object schema reads 56 KB of which most is noise it must actively ignore —
and worse, it IMITATES what it sees: a 251-run evaluation of small models
against this format showed them inventing bson ids because every example
carried one, and writing key fields whose only correct values a real space
mints.

So each grammar also publishes an **authoring subset**, one schema beside
each full one:

    schema/authoring/object.schema.json      https://schemas.anytype.io/anyblock/2/authoring/object.schema.json
    schema/authoring/index.schema.json       https://schemas.anytype.io/anyblock/2/authoring/index.schema.json
    schema/authoring/properties.schema.json  https://schemas.anytype.io/anyblock/2/authoring/properties.schema.json

**A subset, not a different format.** Same `version`, same reader, same
wire: an authored document imports through the same `Unmarshal` an exported
one does, and the authoring URLs keep the trailing file names `DocumentKind`
dispatches on, so declaring one routes to the same reader. The invariant
that keeps the subset honest is that **every document valid under an
authoring schema is valid under the corresponding full schema and full
reader** — and it is a TEST (`authoring_test.go`), not a claim: a fixture
per structure the subset can express, an enum sweep that builds one document
per value the authoring schemas state, and a worked example, each pushed
through the full `Validate` and the real codec. The semantic rules of §12
apply on top, unchanged — the subset narrows the grammar, never the checks.

**What the subset removes** is everything whose value only a live space can
produce, or that a reader derives: ids on blocks, views, sorts and filters;
the three legends (§9a — an author writes spellings, and the legends exist
to bind spellings to STORED keys the author does not have); attribution and
timestamps; `internal_key` where it is app-minted; provenance and derived
state (`origin`, `revision`, `snippet`, `backlinks`, sync state — the
`properties` schema node refuses the spellings small models actually write,
so the phantom-member failure of §2d is an error at generation time, not an
imported phantom); the output-only surfaces of §4a (`store`, `root`,
`fields`, `source`, `groups`, `object_orders`); the input aliases
(`heading_4`, `equation`, `group`); and every kind an author never writes —
the subset's `kind` enum is `page`, `object_type`, `template`, nothing else.
Property documents are gone whole: the dictionary (§2f) is where an author
declares a property, and the import wiring mints the stored identity, which
is exactly what that split was built for.

**Two survivals are deliberate, and both are the SPEC's own rulings.** The
envelope `id` stays — it is the bundle-local slug every cross-file reference
resolves through (§1: "the envelope `id` is not part of that trade"), so the
subset requires its shape instead of dropping it: no leading `_`, none of
the six reserved bare words (§1, §2c). And `internal_key` stays on TYPE
documents only, required there: a bundle's own type key is a slug the author
mints (`habit`), objects name the type by writing that key in `type`, and
the batch wiring resolves the two against each other by exactly that member
(§2a, `anyblockbatch.TypeIds`).

**Each authoring schema is self-contained** — no `$ref` crosses a file,
where the full dictionary and index reference into the object schema (§2e,
§2b). That is a deliberate trade of the one-shape-one-statement rule for the
subset's whole point: an agent handed one file has the whole grammar for
that surface, with no 56 KB import. What keeps the restatements honest is
the same subset test that keeps everything else honest. One narrowing is
semantic rather than surface: the two counting date presets
(`number_of_days_ago`/`_now`) are not in the subset's enum, because where
they apply they REQUIRE a day count in `value` (§6.2), and a subset
admitting them bare would admit documents `Validate` refuses.

**Two subset rules are stated on the RESOLVED property key, not in the
schema**, and they have to be: JSON Schema matches a member name literally
while the format resolves a property key case- and separator-insensitively,
so a literal rule over a key the codec spells many ways is not a narrower
rule — it is a rule with holes in it. Both had one.

- **A type document names itself.** Written as `required: ["name"]`, it
  REFUSED the canonical `{"Name": "Habit"}` and accepted only the retired
  lowercase spelling. Any spelling that resolves to the name property
  satisfies it now; a type document with no name at all still fails.
- **The subset refuses the app's own derived keys in `properties`.** The
  schema's literal list still names the pre-raw-name spellings, and nine
  keys — `creator`, `createdDate`, `lastModifiedBy`, `lastModifiedDate`,
  `addedDate`, `revision`, `internalFlags`, `featuredRelations`,
  `isArchived` — are exactly the ones the FULL format DROPS rather than
  refuses, so nothing downstream caught them under a display-name spelling:
  an author's value disappeared without a word where it used to be refused
  at authoring time. Those nine are enforced on the resolved key. The
  `layout_align` narrowing (an alignment NAME, not the stored number) had
  the same defect and takes the same treatment.

The schema keeps its literal list — it is what an agent actually reads, and
it carries both spellings of each — and a test pins the list against the
enforced set in both directions, so neither can rot again.

`ValidateAuthoring`, `ValidateAuthoringIndex` and
`ValidateAuthoringPropertyDictionary` (§13) run the FULL validation first —
so refusals carry §12's curated wording — then those semantic rules, then
the subset schema, whose verdicts name themselves subset verdicts. The
semantic rules run before the schema because they can say which key was
written and why the app owns it, where a literal `not`/`enum` can only say
that some member matched. A nil return means the document is valid AnyBlock
JSON, not merely subset-shaped.

**The worked example** lives at `testdata/authoring/habit_tracker/`: an
index, one type, a three-property dictionary, a welcome page and two
objects. It validates against the authoring schemas and the real codec,
warning-free, and its cross-file references are asserted coherent — it is
the bundle an authoring agent should be shown first, and the test is what
keeps it worth imitating.

Sizes: object 56,105 → 33,690 bytes, index 8,845 →
4,003, properties 6,727 → 5,691 — the three surfaces together 71,677 →
43,384 (−39%), with every remaining `description` rewritten for an author:
short, concrete, saying what to write. The byte count understates the
narrowing where it matters to a generator: 3 authorable kinds where the full
enum offers 31, 23 block types of 39, 12 envelope members of 19, and no
output-only member anywhere — the test asserts that literally.

## 3. Properties

`properties` is a JSON object keyed by **property key**, always spelled by
the property's **display name** — `"Due date"`, `"Plural name"`,
`"Manual property"`, `"Publish Date"` — NFC-normalized, otherwise verbatim;
bundled, API-created and UI-created keys alike. One uniform rule, no derived
identifier anywhere in the format, no table a writer must classify against:
*a key is the property's name*. A reader never has to know which kind of key
it holds, and a writer never has to transform anything — the measured hazard
of key writing is the derivation step (models normalize names improvisationally
and inconsistently across documents; copying a name byte-exactly is a solved
behavior), so the format deletes the derivation instead of policing it.
(The api slug lives on as the API surface's own
addressing convention — a separate decision — and `apiObjectKey` is never
read by this format.)

The mapping is a **table, both directions, never a string transform**: for
bundled keys the name table derived from the shipped
`relations.json`/`types.json` (which travels with every reader, so documents
still resolve offline), and for every other key the space's own display
names, which a node-backed reader primes from the space — and which the
document carries the inverse of, entry by entry (`property_internal_keys`,
below). "Creation date" says a different word than `createdDate`; no case
transform in either direction exists, and the package's tests pin that the
reverse is a lookup, never a derivation.

**The spelling, and the two authorities it comes from.** A key's spelling is
its display name, and there are two places a name lives:

1. **A bundled key spells the name in the shipped table** — `createdDate`
   spells `"Creation date"`, `tag` spells `"Tag"`, and the relation TYPE
   (stored key `relation`) spells `"Property"`, because that is its bundled
   name. The table ships with every reader, so these spellings resolve
   offline with no legend entry. Names in the shipped table are unique over
   the wire-reachable population and never byte-equal another entry's stored
   key — a CI guard holds that as the condition under which a bundled entry
   may be added or renamed (the `audioGenre` "Genre" → "Audio genre" rename
   is exactly that guard firing early). The nine hidden transients sharing
   the name "Underlying file id" are the tolerated remainder: all nine are
   stripped internal keys, and a shared name is refused as a spelling
   outright, so the tolerance can never leak into a document.
2. **A space-minted key spells what its space names it.** NFC of the stored
   display name, and nothing else: no case fold, no separator collapse, no
   transliteration, no grammar escape. Only three inputs still yield no
   spelling — an empty name, a name over the 128-character writable bound
   (refused, never truncated: a truncation invents a spelling nobody chose),
   and a name carrying control characters — plus the two member names §2
   refuses before any resolution (`id` and `type`, byte-exact). Each of
   those degrades through the collision rule below to the stored key
   verbatim, which is always its own address. A name that merely repeats the
   stored key is no spelling either; the verbatim key already says that.

Raw naming has no normalization step, so `"#"`, `"☕"`, `"C++"` and
`"50% done"` are each a valid property key exactly as written — a rule that
cannot fail needs no repair path. (The one normalization surviving in the package,
`refNameNormalize`, serves the informative `#name` reference suffix (§9),
which is a different surface with a `#`-free grammar to keep.)

**A name is carried exactly as the space holds it** — edge whitespace and
invisible characters included (`'Email 📧 '` is a real production name).
Validation warns about both (§12) and never refuses or trims: one stored
name must not make an object unexportable, and a cleanup belongs where a
user creates or renames the property — one normalization, applied once, at
authoring time — not at the export seam on every write. The forgiving fold
below bridges the near-misses either way.

**Collisions are resolved per DOCUMENT, not per space.** Names are not
unique, and the format does not pretend they are: a document carries a map,
and a map already guarantees its own keys are distinct, so a name that is
ambiguous space-wide but appears once in this document spells its plain
name. Measured, genuine in-document collisions are 60 of 28,560 documents
(0.21%), across five names. Where a document does collide — two properties
claiming one spelling, or a name equal to a stored key the document names
(verbatim-first: a stored key always keeps its own term) — **every claimant
degrades**, deterministically, through one ladder:

- **(a)** the stored key verbatim, when it is itself readable (not a minted
  24-hex bson id) — the `producer_region` / `wine_region` shape;
- **(b)** else `<name> (<tail6>)`, tail6 = the stored key's last six hex —
  deterministic, immutable while the key lives, visibly synthetic;
- **(c)** a residual tie (two claimants minting one suffix, or a suffix the
  document already speaks for) falls to the full stored key, which is always
  its own address.

All claimants degrading — rather than first-claim keeping the plain name —
is what makes the suffix stable across exports and the plain name
trustworthy: a plain spelling in a document is never one of two same-named
claimants. A suffixed spelling never moves while its neighbours live;
deleting one claimant un-suffixes the other on its next export — cosmetic
churn, correct via the legend.

**A claimant is a key this document actually writes**, and two populations
look like claimants without being one. Both are carved out for the same
reason: a claimant that will not be there next time must not decide anybody
else's spelling, or a second export of the same object differs from the
first and the round trip stops being a fixpoint (§11).

- **A key the `properties` emit drops** — a type document's install
  provenance, a participant's load timestamp, an admitted system-stamped key
  whose value is empty, a name-over-number key holding a string its
  vocabulary cannot name — is written nowhere, so it is not counted at all:
  it claims no spelling and reserves no stored key. `isHidden: false` beside
  a custom property named "Hidden" used to write `Hidden (b90aa1)` on one
  export and `Hidden` on the next.
- **The attribution keys** are the opposite case: export WRITES them and
  import drops them, so they occupy a member of this document and none of
  the next one. They **yield** — alone on a spelling they take it as usual;
  contested at all they take their own stored key, which is always readable,
  and the normal claimants keep the verdict they will re-derive once the
  attribution line is gone.

**The map-less reader resolves a shared name within the declared type.** An
authored document need carry no legend, so a reader handed a bare name that
several live properties answer to resolves it against the declared type's
own property list first. Unambiguous there — the overwhelming case, measured
at 1 ambiguous type of 1,753 — and it is resolved. Ambiguous even within the
type, or absent from it, and the reader raises a loud, actionable error
naming the term and asking for the `property_internal_keys` entry that would
settle it. It never guesses between live properties and never mints a
phantom key while two live properties bear that exact name. (A term NO live
entity answers to still resolves verbatim — chain step 5 below — with a
warning; that is the price of any name-addressed scheme, stated in §11.)

Three consequences worth stating outright:

- **Non-Latin scripts are kept, never transliterated.** `Тоггл` is `Тоггл`
  and `日本語のプロパティ` is itself. The api slug's transliteration exists
  because a slug is a URL path segment there; it would answer `toggl` and
  `ri_ben_yu_nopuropatei` here — unguessable and unreadable at once, which
  is strictly worse than either the name or the key. The measured
  degradations are not merely lossy but wrong: `作業内容` (Japanese)
  transliterates through Chinese readings.
- **The name is the address, so a rename moves the spelling — and the
  legend keeps every written document resolvable.** A spelling derived from
  a name changes when the name changes, and the next export writes the new
  one; the stored key never moves, and the `property_internal_keys` line
  every non-bundled key carries binds the exported spelling to it, so a
  document written under "Budget" imports correctly after the property
  becomes "Cost", and a new property later named "Budget" cannot capture the
  old document's values. What the legend cannot protect is the legendless
  (hand- or agent-authored) document, whose stale name misses silently and
  mints a phantom key — accepted as the price of any name-addressed scheme,
  mitigated by the unknown-term warning (§12) and by one measured
  consolation: the likeliest bundled guesses land through the fold
  (`created_at` misses under every scheme, but a guessed `"Created Date"`
  folds onto `createdDate`'s class and resolves).
- **A spelling that is already answered is not up for grabs.** A live stored
  key outranks any name (verbatim-first, below), so a name byte-equal to
  another live stored key degrades through the collision ladder; and `id`
  and `type` are never minted as property spellings because §2 refuses those
  two member names before any resolution. A custom property MAY share a
  bundled name — "Description", "Priority" and "Emoji" all have real custom
  twins in production — because the legend and the per-document ladder keep
  both addressable; a shared spelling with no legend is exactly what the
  type-scoped resolution above answers, loudly when it cannot.

An **absent** `format` in either slot that carries one (`property_definitions[]`,
a dataview's `properties[]`) says the document did not speak, and the §3
chain answers — the bundled table, then the caller's resolver. It is NOT a
declaration of `text`: that reading silently overrode the table, so
`{"property": "Due date"}` in a dataview's list pinned a bundled DATE
property to longtext and its filters stopped being dates, while omitting the
list entirely resolved correctly. Canonical export always writes a format, so an
absent one only ever arrives from a hand-written document — the population
that means "I did not say".

**Resolution — one rule, stated once, covering both namespaces.** The
format names keys in two namespaces — property keys and TYPE keys — and
every key slot lands its term on a stored key through the same chain, first
answer wins, run against the slot's own namespace: its legend, its half of
the bundled table, its stored-key set.

1. **The document's own legend** (`property_internal_keys` for property slots,
   `type_internal_keys` for type slots — identity entries included) — the only
   statement the *document* makes about its spellings.
2. **An exact stored key — verbatim-first.** A term that names a stored key
   means that key, always; the name tables apply only to terms that are
   *not* stored keys. A node-backed reader answers this step from its store
   (`storeresolver`, both namespaces); a package-only reader has no
   stored-key set and knows a term is a stored key only when the legend says
   so — which is why export owes the identity entry below for every term the
   bundled table does not bind to the key being written.
3. **The name tables**: the bundled name table, which ships with every
   reader, and — for a node-backed reader — the space's own names, where
   EXACTLY ONE live entity answers to the term. Several answering is an
   ambiguity this step refuses: the type-scoped resolution above, or the
   loud error, is what happens next — never a guess.
4. **The forgiving fold**, answering only when exactly one candidate
   remains: NFC, casefold, trim, strip default-ignorable code points, drop
   `_`, `-` and spaces. This is the near-miss layer, and it is also the
   whole of legacy continuity: ToSnake only inserts `_` and lowercases, so
   fold(ToSnake(key)) == fold(key) by construction and every pre-change
   derived-slug spelling (`created_date`) lands in its stored key's fold
   class with no compatibility table; `due_date_2` bridges to "Due Date 2"
   the same way. A DENIED key's fold class answers nothing, deliberately —
   forgiveness toward a key import refuses would turn the phantom-member
   warning on `format` and `include_time` into a refusal. (The sixteen
   retired alias spellings — `featured_properties`, … — are outside this
   proof and are cut, not kept: pre-freeze, no back-compat is owed, and
   existing bundles re-export under the names either way.)
5. **Verbatim** — the term *is* the stored key, which is what keeps a
   package-only reader — with no space to ask — lossless on custom keys.
   With a space-backed vocabulary in force, a verbatim term that is no live
   entity's stored key draws a warning (§12): the stale-or-guessed-name
   phantom, every naming scheme's shared hole, named at the seam.

A conforming document resolves identically in every conforming reader:
steps 1, 3(bundled), 4 and 5 need nothing but the document and the shipped
table, and wherever the shipped table cannot answer for a term the document
itself uses, the document carries the entry that moves the answer into step
1 — so step 2 and the space half of step 3, the steps that need a store, are
never load-bearing for a document's own spellings. Every other statement of
resolution order in this document is shorthand for this chain.

The namespaces are **disjoint claim domains**: a property and a type may
share a spelling without conflict (a space may name a relation and a type
one word, and `objectType` the stored type key coexists with `object_type`
the layout value below), which is why the legends are two maps and export
runs one term ledger per namespace — a shared domain would back a key off a
spelling the other namespace owns.

**The document carries its own inverse: `property_internal_keys`.** The name
layer is a re-spelling of key identity, and like every compaction in this
format it has to be invertible from the document alone — the rule §9a
already states for object ids. A name the space minted is not:
`6a32d485…` spelled `"Priority"` reads back through the bundled table — a
different relation — in any reader that cannot ask that space, silently. So
export writes the entry:

```json
"property_internal_keys": { "Priority": "6a32d4856761631534b22f85" }
```

- **Emitted for every spelling the bundled table does not bind to the key
  being written.** One condition, two halves, and they ask different
  questions: the bundled table must **bind** this spelling to this very key
  (it ships with every reader, so `"Due date"` → `dueDate` owes nothing),
  *and* the vocabulary in force must **invert** it (a reader may bind a
  spelling the bundled table binds correctly, and the writer's own space is
  the reader most likely to read the document back — a space holding a
  custom twin of a bundled NAME cannot uniquely invert that name, so the
  bundled key's own usage carries the entry there too, which is what keeps
  the document self-resolving in the one space that is confused about it).

  The asymmetry is what makes the rule exhaustive. A term that is a stored key
  written verbatim trivially *inverts* through any table, because a table that
  does not know a term answers the term itself (chain step 5) — so asking the
  bundled half as an inversion let every custom key pass with no entry at all,
  and the document said nothing about the one population no reader can resolve
  without it. That silence is the **corpse-after-export** hole: the key is
  live and unambiguous the day it is written, and the moment the relation is
  UI-deleted its stored key stops being live while its freed NAME becomes
  another live relation's spelling. Every legendless line already written
  re-points, offline, and no writer could have warned about it — the delete
  happened afterwards. Only the document itself can close that, so a spelling
  the bundled table does not bind owes an entry, verbatim or not.

  **The identity entry is therefore the ordinary line, not the exception.**
  Every custom key names itself: `{"customStatus": "customStatus"}`. Two
  shapes that used to be called out as special are just instances of the one
  rule now.

  The first is the bundled *shadow*: a space whose relation is keyed with
  the literal string of a bundled key's fold class — `due_date`, beside
  bundled `dueDate` — exports
  `"property_internal_keys": {"due_date": "due_date"}`: the document's only
  way to tell a reader with no store that the term is a stored key (chain
  step 2). Without it, the fold silently moved the value onto the bundled
  twin in every package-only reader.

  The second is **the vocabulary in force**, which is the half that stays an
  inversion, and it stays for a measured reason: dropping it — "ask one table,
  not two" — loses `{"task": "task"}`, and a template comes back pointing at
  an unrelated custom type; and loses the entry that keeps a bundled name
  addressable in a space holding its custom twin. A vocabulary is consulted
  *before* the bundled table (chain steps 2–3 are a node-backed reader's
  store), so a term the bundled table inverts correctly can still be bound
  elsewhere by the reader most likely to read the document back: the
  writer's own space. This is not a hypothetical about hand-written
  vocabularies — it is what a **delete** produces. A UI-deleted type or
  property vacates the name namespace while every object it ever named
  keeps its stored key, and its freed name becomes another live entity's
  spelling: `initiative` stops being a live stored key while a live type is
  NAMED "initiative", so `"type": "initiative"` written with no entry came
  back as that other type, silently. The property namespace produces the
  same fault one ladder rung later — the live twin takes the suffixed
  spelling and both terms carry their entries. Export therefore asks both
  tables, and writes `{"initiative": "initiative"}` when either would
  answer something other than the key being written. The entry is
  authoritative for *every* reader, which is the point of a legend; what it
  cannot cover is a reader whose vocabulary disagrees with the bundled
  table in a way the writer never saw, and that is the `KeyVocabulary`
  precondition (§11), not a legend rule.

  The legend is therefore empty for a document whose every spelling is
  bundled, and costs one line per non-bundled key otherwise. **Size**: the
  four golden documents, which each carry two custom keys, grow 93 bytes —
  about 2%. The adversarial corpus, where every document carries five or more
  custom keys, grows up to 15%; that is an upper bound, not an estimate. The
  product's store-backed path pays close to nothing new, because a
  store-minted relation key is a 24-hex bson while its spelling is the
  display name, so spelling ≠ key and the entry already existed.
- **Consulted first, before any vocabulary.** The legend is the only statement
  the *document* makes about its own spellings; a vocabulary belongs to the
  reader, and two readers disagreeing about a spelling is exactly how a
  property ends up naming a different relation than it was exported from.
- **It covers every key slot, not just `properties`.** Wherever the format
  names a property — a `property` block's `property`, a link block's
  `properties` list, a dataview's `properties[].property`, a view's
  `group_by`/`cover_property`/`end_property`, a filter's, sort's or
  column's `property`, a property-definition entry's `property` — the
  spelling is written through the same recording step and read back through
  the legend first. A slot that writes the spelling without recording the
  entry inverts only when some *other* slot in the same document happened to
  record it, which is luck rather than a guarantee; a slot that reads
  without the legend never inverts at all, even when the entry is right
  there.
- **One term, one key — document-wide.** Export claims every spelling
  through a single term ledger, exactly as ids go through one id domain
  (§4): a stored key the document names *anywhere* always keeps its own term
  (verbatim-first — no other key's name may take it), an uncontested
  spelling goes to its claimant, and a contested one degrades EVERY claimant
  through the collision ladder above — computed once from the document's own
  key census, so which spelling a key gets never depends on which slot
  happened to claim first. The discipline covers every key slot, not just
  `/properties` — a `property` block whose spelling collided with a
  `/properties` spelling used to record a legend entry that rebound the
  term, so that property's value landed on a different relation, silently;
  and two blocks sharing one spelling collapsed into naming one key.
- **A legend value is a stored key, and is admitted like one.** It obeys the
  writable-key rule — non-empty, no control characters, at most 128
  characters, the same shape rule property names carry, enforced by the
  schema — **and the §3 deny rule**: a value naming an internal key
  (`uniqueKey`, `oldAnytypeID`, `spaceId`, `id`, …) is refused, by
  validation and import alike, whether or not any member spells the entry.
  The legend is step one of resolution, so an unchecked value was a
  laundering primitive: it could bind any harmless spelling onto a key
  admission refuses — in a key slot outside `/properties`, without admission
  ever seeing it.

  **Export admits an entry before it records one**, and drops the entry, with
  a warning, when it cannot. Two guards were supposed to cover this and both
  had the same hole: a denied key never takes a spelling, and an unwritable
  spelling is never written — but a key with *no* spelling at all skips both
  checks, and the term that reaches the ledger is then the raw stored key. So a stored
  key of 140 characters, or one carrying a newline, or an internal one,
  reached the legend as an identity entry the moment the vocabulary in force
  bound its spelling elsewhere; `Marshal` emitted a legend its own `Validate`
  and `Unmarshal` reject, and the object became unexportable with nothing
  said. Reachable through `Options.Keys` alone, which this format accepts
  from anyone.

  Dropping the entry is the smaller loss, and it is not a loss of content: the
  term is written **verbatim** either way — the ledger backed it off to the
  stored key long before this point — so the object still round-trips through
  any reader that reaches chain step 4. What it gives up is *portability for
  that one key*: a reader whose vocabulary binds that spelling elsewhere has
  no statement in the document to override it with. Such a key has no writable
  spelling anywhere in this format, so no legend entry could have been written
  for it under any rule; the warning names it.
- **A property key slot carries the writable-key rule wherever it is,
  including where it is a JSON string VALUE.** `/properties` and the legends
  are member names, so the schema states the rule as `propertyNames`; a
  property-definition entry's `property` (§2a) is an ordinary string value the
  schema can only reach as one, and for a while `minLength: 1` was the only
  bound it had — a 140-character key, or one carrying a newline, validated
  clean and then failed to import. The rule is the namespace's, not the
  slot's: `/properties` is the property namespace's home surface and cannot
  express a key that is not a member name, so a property with such a key
  cannot appear in a document at all, and a slot that could carry one would
  be offering an address the rest of the format has no way to use. (The type
  namespace answers the same question the other way, and for the same reason
  — its home surface is `type`, a value. See its own rules below.) Export
  drops a type-property entry whose stored key is unwritable, with a warning,
  rather than emit one the seam refuses.
- **A key slot has to name something — at every slot, through every door.**
  This is the one rule that binds *all sixteen* key slots (twelve property,
  four type), and it is the minimum: it says nothing about length or charset,
  only that a slot which names nothing names nothing. Three doors carry it.

  **The document.** Every key-slot string is `minLength: 1` in the schema.
  Only `/properties`, `property_definitions[].property` and `property_definitions[].
  object_types[]` used to be; the other thirteen took an empty spelling from a
  plain document, no vocabulary needed, and then LOST the slot on the way back
  out, in silence: a column and a sort vanish, a property block and a link's
  shown-property list come back nameless, a filter re-exports as a node that
  filters on nothing, and `"type": ""` costs the object its type. A dataview
  filter also has to *carry* the member — `required: ["property"]`, as its
  sibling sort and column always have — and validation states that rule in its
  own words, because the schema can only state it inside a `oneOf` and the
  branch that fails takes the other branch's whole verdict with it.

  **Export.** A filter and a `property` block whose stored key is empty are
  **dropped**, with a warning, which is what the sort and the column beside
  them have always done with the same input. Written out they were nameless
  nodes: the schema accepted them, import stored the empty key, and the next
  export wrote them again — forever, meaning nothing.

  **The import seam.** A vocabulary answering `("", true)` for a non-empty
  spelling is refused at every slot. `/properties`, `type`, `template_for` and
  `object_types[]` refused it from the start; the other nine stored it. The
  refusal names the *spelling*, because the fault is the reader's table rather
  than the document, and that is the fact a caller can act on.

  What this rule deliberately does NOT do is bound length or charset at these
  slots. See the two bullets below: `/properties` cannot express such a key
  because it is a member name, and the primary type slots stay unbounded on
  purpose — bounding them would make a stored key unexportable, which is a
  larger loss than the one it would prevent.
- **The legend cannot launder a spelling onto an internal key.** Entries are
  honored during validation and admission exactly as during import — the
  legend is step one of key resolution — so `{"prio": "uniqueKey"}` does not
  smuggle a `uniqueKey` write past the §3 deny rule twice over: the entry
  itself is refused (previous bullet), and the *resolved* key is what
  admission judges regardless (see below). Conversely, a legend entry that
  binds a denied SPELLING to a harmless stored key (a custom property may be
  NAMED "Format", and an identity entry for a shadow stored key is exactly
  this shape) is honored: nothing lands on the internal key, so nothing is
  refused.

**The type namespace carries the same inverse: `type_internal_keys`.** Everything
above holds with `type_internal_keys` for the legend, the type half of the bundled
table, and the type slots — the envelope `type` and `template_for`, and
`property_definitions[].object_types` (§2, §2a). Export claims type spellings
through a term ledger of the namespace's own, seeded by the same census
(every stored type key the snapshot or the resolved type-property
definitions name), and writes identity entries under the same trigger: a
custom type stored as `object_type`, beside bundled `objectType`, exports
`"type_internal_keys": {"object_type": "object_type"}` or a package-only reader lands
on the bundled twin. Four rules are the namespace's own, each from what a
type key is — and one rule above that deliberately does **not** carry over.

- **No duplicate-binding refusal.** `/properties` refuses two spellings that
  bind one stored key; the type namespace admits them.
  `{"kind": "template", "type": "a", "template_for": "b", "type_internal_keys":
  {"a": "template", "b": "template"}}` validates, and yields `ObjectTypes:
  ["ot-template", "ot-template"]`. The property refusal exists because two
  members collapse into one details field, so one of the two values is lost
  with nothing to say which — a document that means two things and stores
  one. Two type entries collapse into nothing: `ObjectTypes` is an ordered
  list, a repeated entry is a repeated entry, and no value is displaced.
  Refusing here would buy nothing and would refuse documents that lose
  nothing.
- **No deny rule** — and the reason is not that the type namespace is
  harmless. The property deny rule is *import refuses exactly what export
  strips*, and export strips no type KEY: what it drops is positional (the
  entries past the slots §2 models, and keyless entries, both below), never a
  particular key, so the derived set is empty. The stronger reason is that a deny
  rule here would guard nothing: **every effect a document-chosen type key
  can produce is separately, and more directly, writable through the
  property namespace.** Layout — `"type": "participant"` reaches
  `resolvedLayout` through that type's own `recommendedLayout` — is
  reachable as `{"properties": {"layout": …}}`, and `layout` is the FIRST
  thing the resolver that computes `resolvedLayout` consults, above the
  type's answer. There is one place a type key selects a code path in the
  import wiring — a legacy `sub_object` document, whose first object type
  picks which real kind it migrates into — and all that path does is set the
  smartblock kind, which is `kind`, and fill in `sourceObject` when the
  document left it empty, which is `{"properties": {"Source object": …}}`.
  And merge resolution never reads the type list at all: the importer
  derives a document's identity from `kind` plus the envelope `internal_key`, and
  from `unique_key` — never from the object types.

  Merge resolution *is* steerable, but through the **document's own
  fields**, not through type keys. `name`, `relationKey` and
  `sourceObject` are ordinary writable properties, and the relation's
  format — now the envelope's `format` (§2d), where it is
  just as writable and lands on the same stored detail — travels beside
  them; the importer uses them to pick which existing object a document
  merges into: a relation matches on its format together with `name` or
  `property_key`, and a TYPE document matches on `name` alone, since this
  format strips `unique_key` and the name is then the only filter left.
  They stay writable deliberately — the §2d lift moved a spelling, never a
  capability, exactly because a stripped value that import refuses is a
  lossy export and "Marshal never emits a document its own Validate
  rejects" (§11, I1) is the stronger promise. The guarantee that an
  imported document cannot rewrite an EXISTING relation's or type's
  identity therefore belongs at the object layer, which every writer passes
  through, rather than in this format, which is one writer among several. A
  `type_internal_keys` value is admitted by shape alone — the writable-key rule the
  schema enforces on both legends.
- **The primary type slots are unbounded, on purpose.** A `type_internal_keys`
  spelling and a `type_internal_keys` value both carry the writable-key rule (1–128
  characters, no control characters) — the first because it is a JSON
  member name, the second because it is a legend value like any other. The
  envelope `type` and `template_for` carry neither: no pattern, no length
  bound. A term written there is a JSON string *value*, so the member-name
  shape rule does not bind it, and a non-empty stored type key of any shape
  round-trips verbatim. One consequence is worth naming rather than fixing:
  a type key containing `-` yields the object-type unique key `ot-a-b`,
  which does not parse — a unique key is at most two `-`-separated parts —
  so such a type is invisible to a space-backed vocabulary, which reads its
  stored type keys back out of `unique_key`. It still round-trips through
  this format verbatim, and that is the point: the format carries what the
  store holds; which of the store's keys the rest of the system can address
  is not its ruling to make. The one thing refused here is the **empty** type
  key, in both its forms — the literal `"type": ""` (schema `minLength: 1`)
  and a vocabulary resolving a non-empty spelling onto nothing — because it
  would store the unwritable `ot-` and re-export as no type at all, silently.
  That is not an exception to "unbounded on purpose": an empty string is not
  a stored type key of any shape, it is the absence of one.
- **No reserved spelling.** No type key is a reserved word — including
  `template` — because *which type an object has* (`type`) and *what kind
  of smartblock it is* (`kind`) are two separate fields, and the §3 chain
  never touches `kind`. Two checks remain, resolving nothing on their own:
  export keeps `kind` explicit whenever the type term it is about to write
  is literally `template`, and `Validate` refuses a document with no `kind`
  whose `type` is literally `template` (§10).
- **Export writes only the slots §2 models, and says what it drops.** The
  envelope carries one type, plus — on a TEMPLATE — the target type; entries
  past those are not written. An entry with **no key** — a stored `ot-`,
  which older builds wrote whenever a vocabulary resolved a spelling onto
  the empty key — has no spelling at all, so it is dropped and the entries
  behind it move up. Written in place it was contagious: it silenced the
  slot it landed in, and a silent `type` slot makes `template_for`
  inexpressible, so `["ot-", "ot-task"]` exported as no types at all and the
  good entry died beside the bad one. **Both** kinds of drop are reported
  through `OnWarning`, as an unwritable property key is — the keyless entry,
  and the keyed entry the positional truncation leaves nowhere to go, each
  naming the position it stood in among the snapshot's object types. The
  truncation is the format's shape rather than a fault, but it is still a
  type the caller holds and the document does not, and nothing in the
  document says so. And only the slots actually
  written claim a term, so the legend names only types the document
  mentions: claiming a term is what records the legend entry it owes, and
  slugging an entry no slot writes published a space's slug→key mapping in a
  `type_internal_keys` line naming a type the document never spells.

  **The census sees the same list.** Verbatim-first reserves every stored type
  key the document NAMES, and reserving more than that is not merely
  wasteful: a key no slot spells backs another key's slug off, so the same
  object exported before and after a round trip through this format produced
  two different documents — one with the stored key, one with the slug and a
  legend line to invert it. `["ot-custom1", "ot-cust"]`, with a vocabulary
  spelling `custom1` as `cust`, is the whole shape. So the census runs the
  reduction above, and asks of a type property exactly what the emit asks:
  will this entry be written? Nothing is lost by the narrower reservation —
  a key the document never names cannot be taken as another key's spelling by
  a reader who never sees it.

**What is not a key slot.** The vocabulary applies where
a document NAMES a type or property, and nowhere else. Envelope and DTO field
names, enum *values* (`kind: "object_type"`, layout and view-type names), the
`index.json` envelope, view field names like `default_template_id`, and — the
one most easily mistaken for a key — **block attribute names**: a callout's
`icon` and its `format`/`emoji`/`file` members are attributes of a block, not
property keys. They are the format's own vocabulary and follow the format's
own rule (§1 Naming, all snake_case); the vocabulary never touches them, so
they would keep their spelling whatever any *property* were called one
section over. The layout VALUE `object_type` coexists with the type key
spelled `object_type` — one is an enum this format defines, the other is a
name in the space — and that is intended.

Values are encoded by the property's format:

| Format | JSON encoding |
|---|---|
| `text` (default), `url`, `email`, `phone` | string |
| `number` | number |
| `checkbox` | boolean |
| `date` | RFC 3339 date-time string, UTC (`"2026-07-06T15:04:05Z"`); import converts back to unix seconds. Import also accepts date-only strings (UTC midnight), non-UTC offsets (converted to UTC), and fractional seconds (truncated to whole seconds). Export always writes the full UTC form — **except** for a stored value outside the years RFC 3339 can express (0000–9999), which export writes as the **raw number**, with a warning. There is no string form for such a value, and writing one anyway (`"57482-01-22T22:43:20Z"`, from milliseconds stored where seconds belong) would not parse back, so the value would return as a *string* on a date property and stay one. A reader must therefore accept a number here; the number is a stored value it cannot interpret as a date, not a second date encoding. |
| `select`, `multi_select` | array of option **names** (strings) — see below |
| `objects`, `files` | array of object ids (strings). A resolver-wired export drops an entry the SPACE does not hold — the stored `_missing_object` sentinel included — and the emptied list stays `[]`, because the key's presence is meaningful; a package-only export drops nothing (§9) |
| unresolvable format | value passes through verbatim in both directions |

**Enum-valued properties are named, not numbered.** Seven stored keys hold
numbers whose meaning is a proto enum (their bundled relations have format
`number`), and the format writes the enum **name** — a bare integer would
be an opaque enum in an otherwise self-describing format. Each key's
vocabulary, one table per concept (`namedEnumProperties`):

- `recommendedLayout`, `layout`, `resolvedLayout` — the object layout:
  `basic · profile · todo · set · object_type · property · file ·
  dashboard · image · note · space · bookmark · property_options_list ·
  property_option · collection · audio · video · date · space_view ·
  participant · pdf · chat_deprecated · chat_derived · tag · notification ·
  missing_object · devices · discussion` (`$defs/objectLayout`).
- `layoutAlign` — the object's own page alignment: `left · center ·
  right · justify`, the SAME vocabulary a block's `align` and a view
  column's `align` spell (`$defs/blockAlign` — one definition, three
  slots, §15 #14).
- `origin` — how the object entered its space: `none · clipboard ·
  drag_and_drop · import · webclipper · sharing_extension · usecase ·
  builtin · bookmark · api` (`$defs/objectOrigin`). Real provenance, kept
  on ordinary objects (the §2a admission dropped it from TYPE documents
  only, as install provenance) — and all ten values occur in real data.
- `importType` — which importer created an import- or usecase-originated
  object: `notion · markdown · external · pb · html · txt · csv ·
  obsidian` (`$defs/importType`). Named or refused, never a stray string:
  the underlying enum's ZERO is notion, so an unchecked string here read
  back as a false claim that the object came from Notion.
- `imageKind` — what an image object is used AS: `basic · cover · icon ·
  automatically_added` (`$defs/imageKind`). Stored on 4,079 corpus
  documents; named for the same reason as the rest, since a bare integer
  would be an opaque enum in a self-describing format.

Import maps a name to its number and still accepts a raw number, so older
documents keep working; export always writes the name for an in-vocabulary
number and the raw number for anything else — a stored value outside the
vocabulary round-trips as its number rather than being lost. An
unrecognized NAME is a validation error stating the vocabulary, because
the silent alternative was measured and bad: the string imported onto the
number-format detail and every consumer reading it with an int getter saw
the enum's zero. The property slots' vocabularies are enforced by the
semantic pass on the RESOLVED key, not by the schema — a property SPELLING
is not fixed to its stored key (a legend may rebind it, above) — so the
schema states each vocabulary in `$defs` for the reader and the semantic
pass owns the refusal. On the way out the same rule binds export: a stored
STRING a vocabulary does not name has no written form and is dropped with
a warning (written verbatim it was a document Marshal's own `Validate`
rejects, §11 I1), while a stored string that IS a name survives and reads
back as the number.

The remaining layout-ish bundled keys stay numbers deliberately:
`layoutWidth` is a fraction, not an enum, and `widgetLayout` /
`headerRelationsLayout` hold enums too marginal to earn a name vocabulary —
13 and 51 occurrences across 28,604 real exported documents. (The 51 was
first miscounted as 0; the corrected count changes the evidence, not the
verdict — a name table is bought for keys models actually write, and
neither key is one.)

Format names follow the public REST API (`select`, `multi_select`, …);
internally they map to `model.RelationFormat` (`status`→`select`,
`tag`→`multi_select`, `longtext`→`text`,
`object`→`objects`, `file`→`files`; `emoji`, `properties` and `map` exist
for internal formats). The vocabulary is **total** over the model enum
(shorttext's fold aside), and that is load-bearing rather than tidy: a
relation document states its format as a required NAME (§2d), so a stored
format without a name is a relation object that cannot be exported. `map`
earned its name that way — the API does not serve it, but 72 production
relation documents carry format 102 (the bundled `templatePlaceholders`
relation), and a required name over real data may not have holes. The one
statement of the list lives in the published schema (`$defs/propertyFormat`),
referenced from every slot that speaks it.

**There is one text format, `text`.** The editor offers a single Text
property type; the stored `longtext`/`shorttext` split is legacy, carries no
meaning an author could act on, and is **not part of this format** —
`shortText` is not a valid format name and is rejected by the schema.

The collapse is not lossy, because `text` resolves per key rather than
blindly:

- **Export** writes `text` for both stored formats.
- **Import** reads `text` as the key's *existing* format when that key is
  already known to be `shorttext` — bundled properties (`name`,
  `plural_name`, `source`, …) and anything the wiring's `ResolveFormat`
  recognizes. So a
  short-text property keeps its stored format across a round-trip even
  though the document never names it.
- Otherwise `text` means `longtext`, which is what a **new** property
  declared as `text` becomes.

Any other format name is taken literally — the document is authoritative
about its properties, and only the text/text collapse needs a key to
disambiguate.

**Properties are space-wide, not per-type.** Two types whose
`property_definitions` name the same select share one option pool, so their
vocabularies merge into a single dropdown. That is the point for a property
whose values are genuinely common (`tag`) and a defect for the lifecycle
selects a schema reaches for, where the same word means different things per
type. Documents that want distinct vocabularies must use distinct keys.

**Select options are names, not ids — everywhere.** This rule covers
property values here, filter `value`s, and sort `custom_order` entries
(§6.2). Export writes option names (`"status": ["In progress"]`); import
resolves names against the property's existing options and **creates
missing ones** (the behavior of the CSV and Notion importers, and of the
public API's tag endpoints). Names, not ids, because a bundle carries no
option objects — unlike a linked object, which the bundle carries and the
importer relinks, an option id from another space would dangle — and because
opaque option ids are unwritable by agents and unreadable by humans.

**The document carries the id beside the name: `option_ids`.**
Name-addressing alone loses identity in two ways a live account shows, and
both were measured on a 34 339-object sweep: two options of one property may
share a name, and name resolution answers the FIRST, so an object sitting on
the second came back pointing at the other one (7 objects); and an option
renamed between export and import stops resolving at all, so the wiring mints
a NEW option carrying the stale name — resurrecting the duplicate and
orphaning the object from the renamed option. So export writes the id beside
the name, in a legend keyed by the property that owns the option (§9a):

```json
"priority": ["High"],
"severity": ["High"],
"option_ids": {
  "Priority": { "High": "bafyrei…opt1" },
  "Severity": { "High": "bafyrei…opt2" }
}
```

The outer key is the property **spelling this document writes**, the inner
key the option **name** exactly as the value spells it; §9a states the shape
and the emission rule. It is written wherever export substitutes a name for
an id — property values, filter values, custom orders — and behind no option
at all, because it is identity rather than compaction.

**Reading one option value: three steps, first answer wins.**

1. **`option_ids[<the spelling this slot wrote>][<the name>]`** — honoured
   only when the id it names is a **live option of that relation** in the
   target space. There is no reachability precondition left to state: a
   reader indexes the legend by the spelling the slot in hand wrote, so an
   entry under any other spelling is simply never looked up (§9a warns about
   one). The liveness check is the whole reason the entry is safe to write
   unconditionally: an id from a space the reader never had is not an answer,
   and the document falls through as if it carried none.
2. **Name resolution** against the property's existing options, as before.
3. **The value unchanged** — creating the missing option is the wiring's job.

A reader with no option resolver (§13) has no space in which to ask either
question and stops at step 3, exactly as it did before this legend existed.

**The legends do not answer to one rule, and the difference is deliberate.**
A `property_internal_keys` or `type_internal_keys` value is **authoritative**: the reader takes
it as the stored key, unchecked. Liveness-checking it would re-open the fault
the legend exists to close — a slug vacated by a deletion and reclaimed by a
new entity, where the key the document names is precisely the one the target
space no longer serves under that spelling. An `option_ids` value is a
**hint**, checked, because an option id names exactly one option of exactly
one relation, so the target space can answer whether the id is that; and
where the answer is no, the name is a better address than a dead id. The two
rules differ because the two questions do: a stored key IS the address, while
an option id is a shortcut past a name that is already one.

What the authoritative rule costs, stated precisely: a legend value is a key
as the writing space holds it, so a reader in another space lands it
verbatim. That is *not* the same as saying legend values are
source-space-only. A **bundled** key is identical in every space, and a key
that arrived through an older pb-format import of the same data is reproduced
identically in every space that imported it — for both, a legend value
travels as well as the document does. The caveat is exactly the
**space-minted** key: a bson `6a32d485…` one space minted for its own
relation names nothing in another, so a document carrying it lands on a key
the target space has never seen instead of merging onto that space's
equivalent property. A bundle survives this because it ships the entity's own
document under that key; a document lifted out of a bundle does not. (The
import *wiring* narrows this further — `core/block/import/pb` re-homes a
non-bundled key onto an existing relation of the same format bearing the same
display name — but that is the wiring's behaviour, not the codec's: the codec
binds the slot to the stored key and hands it on.)

What remains normalized, and what no longer is: **one object holding two
same-named options of one property** still collapses — the document spells
`["books", "books"]`, and two identical strings have no way to say which entry
means which option. Export keeps the first writing, so the collapse is
deterministic and a second export reproduces the first byte for byte (§11
guarantee 3);
it is no better than name resolution here, and no worse. A rename, and a
duplicate name an object touches only once, are no longer lossy (§11).

**Format resolution.** The format does not carry per-property formats;
`Marshal` and `Unmarshal` accept an optional resolver (§13). Property keys in
`bundle` resolve built-in; other keys resolve via the caller's resolver or
fall back to verbatim passthrough. Export and import must be wired with
equivalent resolvers for custom date/select properties to round-trip in
their pretty form; with no resolver the value still round-trips losslessly,
just unprettified.

**Well-known properties** (the magic keys every generator needs). The
spelling is the display name (§3); the stored key is what it resolves to:

| Spelling | Stored key | Format | Meaning |
|---|---|---|---|
| `Name` | `name` | text | the object's title |
| `Description` | `description` | text | subtitle/description line |
| `Done` | `done` | checkbox | completion state on task-like types |
| `Due date` | `dueDate` | date | due date on task-like types |

The icon and the cover are **not** in this table: they are envelope fields of
their own (§2b), and the nine stored keys behind them are refused here.

**Canonical key order in `properties`** (implementation decision): the
well-known keys `name`, `description` first (in that order, when present),
then all remaining members alphabetically BY SPELLING — the reader sorts
what it sees, so the order is over the display names, while which two go
first is decided on the stored keys. Both `icon_emoji` and
`icon_image` are lifted above `properties` entirely — a stronger
version of the same idea, since a reader now meets the icon before the
property list rather than at the top of it.

**Presence is meaningful.** A key's presence in `properties` records that the
property was set on the object — clients use it to show the property even
when its value is empty. The §4 omit-empty canon therefore does **not**
apply to property values: every key present in the snapshot is written, with
its value verbatim — including `false`, `0`, `""`, `[]`, and explicit
`null`. Import preserves them all (an explicit `null` stays a null value).
Omitting a key and writing an empty value are different statements: absent =
property not set; empty value = property set, currently empty.

**Seven system-stamped keys are the exception** (§15 #12). `isHidden`,
`isHiddenDiscovery`, `isArchived`, `relationReadonlyValue`, `revision`,
`relationMaxCount` and `relationDefaultValue` are written only when their
value is NOT empty. Nothing sets them but the system, and for each the empty
value IS the semantic default — `false` is visible, `0` is unlimited, an
empty default value is no default — so no reader distinguishes absent from
present-and-empty: every one reaches the value through a typed getter that
answers the same either way. Measured over 36,967 production documents,
their empty values are 1.13% of all bytes but the distribution is bimodal
(p50 1.21%, p90 13.55%, max 23.22%): they cluster on RELATION and TYPE
documents, so an agent reading a space's SCHEMA reads exactly the documents
that pay ~20%.

It is a **whitelist, not a category**. The blanket form — every key in
`bundle.SystemRelations` minus an exception list — was declined: it admits
every system relation added in future sight-unseen, and buys almost nothing,
since the saving is top-heavy (these seven carry ~50% of it; the thirty-key
tail carries 0.04% of all bytes). The keys that FAILED admission are as
important as the ones that passed: `relationFormat` is excluded because its
`0` is `longtext`, a real format rather than "unset" (§15 #14), and
`relationFormatObjectTypes` and `featuredRelations` because they are
list-valued and user-intent-bearing — an empty list is how a CLEARED set is
expressed, the same reasoning GO-7451 settled for a type's recommended
lists. (The two `relationFormat*` keys have since moved to a relation
document's envelope, where the same verdict holds: the §2d fields mirror
stored presence, empty values included.) This is a state normalization,
recorded in `N(S)` (§11).

**Value shape** (implementation decision): select/multi_select and
objects/files values are always JSON arrays; import stores them as lists, so
internally scalar-stored values (e.g. `assignee` holding one participant)
normalize to single-element lists on round-trip (§11). The two attribution
properties are the exception and are plain strings — see below.

**Stripping.** Export removes internal/derived properties
(`bundle.LocalAndDerivedRelationKeys`) **except** those the importer
meaningfully preserves (mirroring `core/block/import/pb`): `createdDate`,
`lastModifiedDate`, `isFavorite`, `isArchived`, `resolvedLayout` — spelled
"Creation date", "Last modified date", "Favorited", "Archived" and
"Resolved layout".
Those five are **output-only** (§4a): export writes them, generators should
not — with one deliberate exception. **`isFavorite` is authorable**, because
the pb importer reads it to choose a space's root objects
(`core/block/import/pb/space.go`), which is how a generated bundle
designates the object a user should land on. A bundle with no favourite, no
`homepage` and no `spaceDashboardId` imports as an undifferentiated list. `id` is lifted to the envelope and `type` to `type`. Everything else
round-trips.

**A participant document does not carry `createdDate`** (the
transient-key policy scoped by kind, like the type-provenance drop in §2a —
the verdict lives on `participantProvenanceKeys`). A participant is derived
from the ACL and has no creation change, so the store stamps `createdDate`
with `time.Now()` on every cold build. Measured, which is what admitted the drop: two exports of the
same 7 spaces, 1,164 documents compared field-by-field — the ONLY drifting
kind is participant (22 of 22) and the ONLY drifting field `createdDate`;
on a full 155-space run, 2,322 drifts against 2,492 participants, every
other kind byte-stable. Export omits the key on participants whatever it
holds; import drops it there (stale, not wrong); the §11 comparator
consults the same predicate. `creator` and `lastModifiedBy` STAY on
participants by decision, although both read `_anytype_profile` on 2,492 of
2,492 corpus participants: that placeholder is upstream's bug to fix — a
participant's creator should be the real identity — not this format's to
paper over by omission.

**Attribution: `creator` and `lastModifiedBy` are the member's RESOLVABLE
id, named by the informative suffix — `<identity>#<name>`, as a plain
string.**

```json
"Created by": "A6eK73JmBUM9Aar2BJ4Pd6VkLW7cjhoWL7tJHDM9gk8fhpkc#roma_kha",
"Last modified by": "A6eK73JmBUM9Aar2BJ4Pd6VkLW7cjhoWL7tJHDM9gk8fhpkc#roma_kha"
```

Not an array. Both relations are `maxCount: 1` and 0 of 36,966 production
values were multi-valued, so the list wrapper the other object-format
properties take is definitionally wrong here.

The spelling is the general §9 reference shape: the stored participant id
through the participant fold (48 characters instead of 135), the member's
display name riding after the `#` as a caption. An earlier design wrote the NAME alone: it broke API v2, whose consumers need an id to resolve a
member (avatar, profile), and **two members of one space can carry the same
display name** — 76 of 2,478 production participants do — so the name
identified nobody. The suffix keeps what the name-only form bought (a reader sees WHO,
not an address) and the id restores what it traded away.

Both are `source: derived, readonly: true`: their value is recovered from the
object tree root's own cryptographic signature on every rebuild
(`treeSource.GetCreationInfo` → `NewParticipantId(spaceId, identity)`), and
four independent seams discard whatever a document supplies —
`state.StructCutKeys(details, LocalAndDerivedRelationKeys)`
(`core/block/editor/state/change.go`), the pb importer's preserve-list, which
names neither, `changeBlockDetailsSet`, and the API's "cannot be set
directly". **Import drops both keys**, whatever they carry. That reasoning
does **not** extend to `assignee`, `author`, `stakeholders` or any custom
`objects` property: those are `source: details`, chosen by a person; they
keep the array shape and the ordinary §9 reference rules.

The name comes from a `ParticipantResolver` (§13), which export asks and
import does not have — and unlike the ordinary reference suffix it is NOT
behind `RefNames`: both keys are dropped on import, so no byte-stability is
at stake, and the name is the reason the line is worth writing at all.
**Without a resolver, or for a member this space has no name for, the id is
written BARE** — never a dangling `#`, and never an omitted property: the id
is the resolvable half and is complete without its caption. Only a value
holding no id at all omits the property — and so does the one degenerate id
production data actually holds: 9,103 of 37,429 corpus objects store
`lastModifiedBy = _participant_<space>_`, the composite built from a BLANK
identity. Eighty-six characters that address nobody are the id-shaped
analogue of a blank name and get the blank name's verdict.

**The name is not an address, and nothing resolves it back.** It is the §9
informative suffix: trimmed unread, never required, never unique. There is
deliberately no `option_ids`-style legend for it: the legend exists where a
name has to invert (§9a), and here nothing may.

**Admission is symmetric with one documented exception: import refuses what
export strips, except for the keys it DROPS in silence.** Two families
qualify, and each entry owes the same two answers: what the key means in the
app, and why nothing downstream of an import can act on it.

- **Transient keys** describe the *moment* an object was written rather than
  the object. `internalFlags` carries editor state (`editorDeleteEmpty`,
  `editorSelectType`, `editorSelectTemplate`: "this object was just created,
  offer the type picker"), and a restored object is never mid-creation.
  Export removes them like everything else on the stripped list. (Measured
  across 36,967 real objects it was the single largest source of exported
  noise — present on 18,647 of them, and empty on every one.)
  `fileBackupStatus` and `fileIndexingStatus` are the same family from the
  file machinery: which sync/index state THIS device last observed, stamped
  on every file object (all 10,248 in a 28,604-document corpus), and the
  destination's machinery determines its own — `fileIndexingStatus` carried
  ONE distinct value across all occurrences and, imported, told the
  destination's indexer the restored file needed no indexing.
- **Attribution keys** — `creator`, `lastModifiedBy` — name the member who
  wrote the object. Their stored VALUE is stripped like every other derived
  key; what export writes is the `<id>#<name>` spelling above, which no
  write path could honour (the value is re-derived from the tree on every
  rebuild). This closes an asymmetry with no reason behind it: `creator`
  used to be accepted (it sat on the preserve-list, so the deny rule never
  saw it) and landed a detail the next rebuild overwrote, while
  `lastModifiedBy` — an identical relation definition — was refused
  outright.

Either way, import drops instead of refusing because a document carrying one
is *stale*, not hostile, and refusing it would make an older export
unimportable for no gain. Everything else on the stripped list is derived
state or a merge-resolution vector, and those stay errors.

The rest of the rule, unchanged: **import refuses exactly what export strips.** The
list above is the only list — the reader derives its deny-list from it rather
than restating it, because a restated list drifts, and the drift ran one way:
import used to accept every key an author supplied, so `isArchived`,
`isDeleted`, `spaceId`, `restrictions` and `uniqueKey` all landed on details
while export removed them. Setting one is an error naming the key. Two more keys
are refused with them, because they are how the importer decides which
*existing* object a document merges into
(`core/block/import/common/objectid/existingobject.go`): `oldAnytypeID` and
`sourceFilePath`, alongside `uniqueKey` from the list. Those two are bundled
relations like any other — each has an api slug — but they are absent from
`bundle.LocalAndDerivedRelationKeys`, which is the list the deny-rule derives
from, so they have to be named by hand. Export strips those
three too, so the symmetry holds in both directions. `id` and `type` are
refused by name as well — they are the envelope's (§2), and dropping them in
silence left an author with no explanation for why the id they wrote had no
effect. (Those two are refused as *spellings*: the importer lifts them into
the envelope before any resolution runs, so the legend cannot re-purpose
them.)

**Admission runs on the resolved stored key, not on the raw spelling.** The
document spells slugs, so a reader first lands each `properties` key on its
stored key through the §3 resolution chain, and *then* applies the deny
rule, the enum-name check and the format-shape warning to the result.
Checked against the raw spelling instead, all three were dead for exactly
the documents this format produces: `unique_key` walked past the rule that
`uniqueKey` tripped, and a `property_internal_keys` entry could rebind any harmless
spelling onto any internal key — including `id` itself, which overwrote the
envelope id from inside `properties`. `Validate` resolves with the chain
steps it has — legend, bundled table, verbatim; it holds no store, so chain
step 2 reaches it only through the identity entries export owes, and it
takes no resolver (§13). A reader whose vocabulary resolves *further* — a
node-backed caller whose space maps a slug to a stored key the bundled
table never knew — must re-run admission on **its** final resolved key,
which import does at the seam where details are written (`importer.build`).
Admission at that seam is three refusals, and validation mirrors every one
of them: a **denied** resolved key; an **unwritable** resolved key (a wider
vocabulary can resolve a spelling onto the empty string, which used to land
`details[""]` in silence and vanish on re-export); and **two spellings
binding onto one stored key** (refused only at import for a while, so a
hand-written `{"pluralName": …, "plural_name": …}` validated clean and then
failed to import; the original repro was the icon pair, which the icon rule refuses
one step earlier). The two halves agree exactly whenever no wider
vocabulary is in force, which is what keeps Validate and Unmarshal
accepting the same documents (§12).

**A property key has to be writable.** Non-empty, no control characters, at
most 128 characters (`propertyNames` in the schema, restated in the reader so
the issue can name the offending key — §12). This is a *deny* rule and
not an allowlist on purpose: real stored keys are bundled camelCase keys, bson-hex
ids, and bare names from old accounts, and an allowlist could only be trusted
after checking every key in every account — while the shapes ruled out here
(the empty key, a key with a newline in it) are keys nothing can read. Export
drops such a stored key with a warning, since there is no way to write it.

The rule binds the **spelling**, and the spelling is whatever the vocabulary
answers: the shipped label rule enforces it (§3 — a name outside the
writable bound is no label at all), but `Options.Keys` accepts an
implementation from anyone, and the raw material underneath is a display
name that is arbitrary user text with no length bound and no reserved-word
check — so nothing upstream *guarantees* a spelling this format accepts.
Export therefore checks the spelling it is about to write, and one it
cannot honor falls back to the stored key — always its own address
(verbatim-first) — with a warning naming the vocabulary's answer. Three
answers export cannot honor: an **unwritable** spelling (over-long, empty,
control characters — on either side of a legend entry); a spelling the deny
rule refuses before any resolution (`id`, `type` — the envelope's, which
the legend cannot re-purpose and therefore cannot rescue; a property
literally named "id" really mints this spelling); and any spelling for a
**denied key**, whose legend entry would carry a value admission refuses.
Checking the stored key and
then emitting the slug unchecked made `Marshal` produce a document its own
`Validate` rejects, on `/properties` and `/property_internal_keys` at once, which
§11 rules out.

**A value whose shape its format cannot hold is a warning**, not an error, and
only for keys the bundle resolves after the resolution chain runs (`Validate`
takes no resolver, §13): `"Due date": "next Friday"` is stored as written and
then read as no date at all, which nothing else would ever report. It stays a
warning because the same check as an error would make one already-corrupt
stored value enough to make an object unexportable, and "Marshal never emits
what Validate rejects" (§11) is the stronger promise.

Validation: the schema types `properties` loosely (`object` with scalar/array
values). Strict per-type validation against the object-type schemas generated
by `pkg/lib/schema` is a possible future layer (it would need a key↔name and
id↔name translation, since those schemas key by display name); v1 does not
provide this.

## 4. Blocks — common structure

`blocks` is a **flat array in pre-order**: a parent precedes its descendants
and a subtree is a contiguous run. Nesting is expressed by the per-block
`indent` integer — there is no `children` key (a document containing one
fails schema validation). Every block is an object:

```json
[
  { "id": "b1", "type": "bulleted_list_item", "text": "top level" },
  { "indent": 1, "id": "b2", "type": "bulleted_list_item", "text": "nested" },
  { "indent": 2, "id": "b3", "type": "paragraph", "text": "deeper" }
]
```

| Field | Type | Req | Notes |
|---|---|---|---|
| `indent` | integer ≥ 0 | no | Nesting depth. Absent = `0` (top level); canonical form omits `indent: 0`. Values above **32** fail validation (adversarial-input bound). Real documents reach **6** — that is the deepest nesting anywhere in a 36,967-object corpus once transparent containers are lifted (§7a); before the lift the same corpus reached 26, all of it wrapper. See the nesting rules below. |
| `type` | string | **yes** | Discriminator; full inventory in §5. Unrecognized values fail schema validation (see §10 for forward compatibility). |
| `id` | string | no | `[A-Za-z0-9_-]{1,64}`. Uniqueness is enforced over the whole document, including derived table cell ids `<rowId>-<colId>` — the whole grid, written cells and unwritten ones alike (§6.1) — so a non-table block id that collides with a derived cell id is a validation error. Dataview **view** ids are the one exception: they are unique **within their dataview block**, not document-wide (§6.2). Export writes ids by default — the `OmitIds` option drops them (§9); import generates missing ids with the editor's standard id generator. |
| `align` | `left · center · right · justify` | no | Omit when default (`left`). |
| `vertical_align` | `top · middle · bottom` | no | Omit when default (`top`). |
| `background_color` | string | no | Anytype color name. Omit when empty. |
| `fields` | object | no | Verbatim internal per-block key-value data **minus** keys lifted into first-class props (e.g. `lang` §5.1, `width` §6.1). Output-only escape hatch (§4a) that keeps unknown data lossless. |

### Nesting

- **Reconstruction** (import semantics, normative): walk the array with a
  stack seeded `(root, indent = −1)`. For a block with indent *k*: pop the
  stack until the top's indent is *k − 1*; the top is the parent; append the
  block to the parent's children; push `(block, k)`.
- **Validity** (strict, the default): the first block's indent MUST be 0,
  and a block's indent MUST be at most one greater than its predecessor's.
  Violations are **errors**, path-addressed and naming both indents
  (`/blocks/7: indent 3 follows indent 1 — a block can be at most one level
  deeper than its predecessor`). Every prefix
  of a valid `blocks` array is itself valid — a truncated document parses as
  a well-formed prefix of blocks (enforced by test).
- **Lenient mode** (`Options.NormalizeIndent`, import only, default off):
  an over-deep indent (jump > +1) is **clamped to the previous block's
  indent + 1** — CommonMark's list rule: a level that hasn't been
  established cannot be opened — and a first block with indent > 0 is
  clamped to 0. Every clamp is reported as a warning-grade issue with the
  block's JSON path (`Options.OnWarning`). Indents outside [0, 32] are
  errors even in lenient mode.
- **Containment** (semantic checks, §12): leaf block types cannot be
  parents — a block indented under one is an error naming the parent type
  (the leaf types are marked in §5); a block whose parent is a `row` must
  be a `column`.

Block restrictions are **not** part of the format: they are runtime policy,
reconstructed by the editor on import.

**Serialization canon** — what export produces; `Export ∘ Import` is
byte-stable over it (§11):

- UTF-8, LF, two-space indent.
- **Key order = spec order.** Envelope keys in the §2 table order. Block
  keys: `indent` first, then `id`, `type`, then the
  type-specific props **in the order listed for that type in §5** (`text`
  always last), then `align`, `vertical_align`, `background_color`, `fields`.
  Nested dataview/table objects: the order listed in §6. `property_internal_keys`,
  `type_internal_keys` and `option_ids` entries sorted by key, and each `option_ids`
  inner map sorted by option name.
- **Omit empty and default.** Canonical form never writes an empty string,
  empty array, or empty object (envelope included — no `"properties": {}`),
  nor a default scalar (`"indent": 0`, `"checked": false`, `"align":
  "left"`, `"hidden": false`…). Absent `text` means empty text. Import
  accepts explicit empties/defaults and canonicalizes them away.

### 4a. Output-only fields

Some fields exist purely so that export → import loses nothing. Export
writes them; **generators should omit them** — import accepts documents
without them, and where a supplied value would not be safe to take it is
refused rather than quietly used: the internal property keys are a deny-list
in the reader (§3), which is where "authoritative only where semantically
safe" is actually implemented. Most output-only fields carry
`x-output-only: true` in the JSON Schema so tooling can warn; the one kind
that cannot is the preserved internal properties, which live inside the
free-form `propertyMap` and so have no schema node of their own to annotate.

Output-only surfaces: `fields` (any block), `root`, `store`, `source`
(dataview), `groups`/`object_orders` (views, §6.2), `id` on sorts/filters,
filter `nested_property` (reserved), `cover.source` and the `emoji`
carry-over on `icon`'s named-icon branch (§2b), the five preserved internal
properties listed in §3, and the two attribution properties
`creator`/`lastModifiedBy`.

The attribution pair is output-only in the strictest sense on the list:
export writes it and import does not merely ignore a supplied value, it
drops the key. Everything else here at worst round-trips.

## 5. Block type inventory

Text styles are promoted into `type`; every proto content type maps to one or
more JSON types. The "Proto origin" column is informative (for implementers),
not part of the format. Prop lists are in **canonical order** (§4). Complete
mapping:

| JSON `type` | Proto origin | Type-specific props (canonical order) |
|---|---|---|
| `paragraph` | Text/Paragraph | `color`, `text` |
| `heading_1` … `heading_3` | Text/Header1..3 | `color`, `text`. Input aliases `heading_4`/`header_4` map to `heading_3`; stored deprecated Header4 blocks **export as** `heading_3` (§11) |
| `quote` | Text/Quote | `color`, `text` |
| `code` | Text/Code | `language` (from `fields["lang"]`), `text` (**literal**, §8.4) |
| `title` | Text/Title | — structural, see §7 |
| `description` | Text/Description | — structural, see §7 |
| `checkbox` | Text/Checkbox | `checked`, `color`, `text` |
| `bulleted_list_item` | Text/Marked | `color`, `text` (common block-editor naming) |
| `numbered_list_item` | Text/Numbered | `color`, `text` (numbering is derived from position among consecutive siblings; never stored) |
| `toggle` | Text/Toggle | `color`, `text` |
| `callout` | Text/Callout | `icon` (§2b, `emoji` or `file` only), `color`, `text` |
| `toggle_heading_1` … `toggle_heading_3` | Text/ToggleHeader1..3 | `color`, `text` |
| `file` `image` `video` `audio` `pdf` | File (Type enum promoted; `Type_None` → `file` with no `object_id`) | `object_id` (target file object), `name`, `mime_type`, `size` (bytes), `style` (`auto · link · embed`), `added_at` (RFC 3339; omitted with a warning when the stored timestamp is outside the representable years, §3 — unlike a property value there is no number form to fall back to). Legacy `hash` accepted on input. On export, a block with only the legacy `hash` set writes it as `object_id` (the hash migrates on round-trip, §11); when both are set, `object_id` wins and the hash is dropped. `state` is not serialized: import sets `Done` when `object_id`/`hash` is present, `Empty` otherwise. File blocks are leaves in the editor, but legacy data can nest real blocks under them — indented descendants are allowed and round-trip verbatim |
| `bookmark` | Bookmark | `url`, `object_id` (target bookmark object). `state` handled like file blocks. Deprecated preview fields and `type` (derivable) are dropped — preview data lives on the target object |
| `link` | Link | `object_id` (target object), `card_style` (`text · card · inline`), `icon_size` (`none · small · medium`), `description` (`none · manual · content`), `properties` (string array: property keys shown on the card). Deprecated `style` and legacy `fields` are dropped |
| `divider` | Div | `style` (`line · dots`, default `line`) |
| `row` / `column` | Layout/Row, Layout/Column | — (descendants carry content; a `row` contains only `column`s — §4 containment, read on the lifted tree, §7a) |
| `group` | Layout/Div (legacy) | — **accepted on input only; lifted** (§7a). No export ever writes one |
| `table` | Table (+ structural children) | `columns`, `rows` — see §6.1 |
| `embed` | Latex | `processor`, `text` (**literal**, §8.4) — see §5.2 |
| `table_of_contents` | TableOfContents | — |
| `property` | Relation | `property` (the property's spelling, the member every property-naming slot uses; renders the property inline) |
| `dataview` | Dataview | fully specified in §6.2 |
| `widget` | Widget | `layout` (`link · tree · list · compact_list · view`), `limit`, `view_id`, `auto_added`. Appears only inside a widget object — and a bundle carries no widget document: its sidebar is `index.widgets`, which states these members flat beside the link child's (§2c) |
| `chat` | Chat | — (rare) |
| `featured_properties` | FeaturedRelations | — structural, see §7 |
| `icon` | Icon | `name` (legacy profile objects only) |

Enum values serialize as snake_case strings (§1 Naming); defaults are omitted.

**Leaf types.** `embed` (and its `equation` alias), `bookmark`, `link`,
`divider`, `table`, `property`, `dataview`, `icon`, `table_of_contents`,
`featured_properties`, and `chat` cannot be parents: a block indented under
one is a validation error naming the parent type (§4 containment, §12).
Every other type may be a parent.

Normalization notes:

- `checked` on styles other than `checkbox` is dropped (the editor only
  honors it there).
- Stored marks on `code`/`embed` blocks are dropped on export (their `text`
  is literal).

### 5.1 Code blocks

`language` is lifted from the internal `fields["lang"]` (the storage location
used by the editor and all importers). On import it is written back; a `lang`
key inside an explicit `fields` object is an error when `language` is also
set.

### 5.2 Embed blocks

`processor` selects the embed kind — full enum, snake_case: `latex`
(default), `mermaid`, `chart`, `youtube`, `vimeo`, `soundcloud`,
`google_maps`, `miro`, `figma`, `twitter`, `open_street_map`, `reddit`,
`facebook`, `instagram`, `telegram`, `github_gist`, `codepen`, `bilibili`,
`excalidraw`, `kroki`, `graphviz`, `sketchfab`, `image`, `drawio`,
`spotify`.

`text` carries **source code** for renderer processors (`latex`, `mermaid`,
`chart`, `graphviz`, `kroki`, `excalidraw`, `drawio`) and a **URL** for
service processors (everything else); for service processors import also
accepts the URL under a `url` key as an input alias.

Standalone math is `{ "type": "embed", "processor": "latex", "text": "…" }`;
import accepts `equation` as a type alias for it (what Notion-trained
generators will write).

## 6. Complex blocks

Two content types carry structure beyond text and props; both get first-class
mappings rather than raw protojson — the format is meant to be fully
readable/writable, not only its Markdown-shaped primitives.

### 6.1 Tables

Anyblock stores tables as a block subtree (table → row/column layout wrappers
→ cells with composite ids `<rowId>-<colId>`). The JSON format hides this
machinery:

```json
{
  "type": "table",
  "columns": [ { "id": "col1" }, { "id": "col2", "width": 120 } ],
  "rows": [
    { "id": "row1", "is_header": true, "cells": [ "Name", "Status" ] },
    { "id": "row2", "cells": [ "Export",
        { "type": "checkbox", "checked": true, "text": "done" } ] },
    { "id": "row3", "cells": [ null, "spec" ] }
  ]
}
```

- `cells[i]` corresponds to `columns[i]`; `null` = empty cell. A row with
  **fewer** cells than columns is padded with trailing empties; **more**
  cells than columns is a validation error.
- A cell is a plain string, `null`, a block object, or an array of flat
  blocks. The string form is shorthand for a plain paragraph and is
  **canonical** whenever the cell qualifies (a `paragraph` with only `text`
  set); a block object is used otherwise. A bare cell block carries no
  `indent` (validation error if present). The **array form** exists for the
  legacy case of a cell block with descendants: the cell block first at
  indent 0, its descendants following per the §4 rules; export uses it only
  when descendants exist (single-block cells stay bare — canonical). Cells
  **never carry `id`** — cell ids are derived (`<rowId>-<colId>`); an `id`
  on a cell block (bare, or first element of the array form) is a
  validation error. Cell blocks (and their array-form descendants) **cannot
  be `table` blocks**: cells use a dedicated non-recursive block definition,
  which is what keeps the whole block schema recursion-free (§12).
- Column/row `id`s are optional; when present they must match
  `[A-Za-z0-9_]{1,64}` — **no `-`**, which is the composite-cell-id
  separator. Import generates missing ids.
- `width` on a column entry (pixels) is first-class (lifted from the
  internal `fields["width"]`); other column data round-trips via `fields`.
- **Generated row/column ids obey the same charset as authored ones.** A
  cell's id is `rowId + "-" + colId`, and the editor recovers the column with
  `SplitN(id, "-", 2)` (`table.ParseCellID`),
  so a `-` anywhere in a row or column id silently reassigns cells to the
  wrong column. `Options.GenerateId` belongs to the caller and need not
  respect that, so import
  sanitizes generated ids into `[A-Za-z0-9_]{1,64}` and disambiguates
  collisions rather than trusting the generator. Both apply only where they
  are needed: a generated id that already fits the charset and collides with
  nothing keeps the name the generator gave it, as every other minted id does
  (§9). Export sanitizes stored ids
  the same way, since data predating this rule contains dashes and `Marshal`
  must never emit a document its own `Validate` rejects.
- **A table owns its whole grid of derived ids, written cells or not.** The
  id `<rowId>-<colId>` belongs to the table for every row×column pair,
  because the editor materializes a missing cell at exactly that id the
  first time it is filled — an unwritten cell's id is reserved, not free.
  All three surfaces claim the same set: validation over the grid, export
  before it labels any other block, import before it generates any id.
  **The plain block is the side that yields.** A derived id has no spelling
  of its own — it is whatever the row and column ids make it — so a block
  whose stored id collides with one is written under a disambiguated label
  (`r1-c1` → `r1-c1_2`) while the row and column keep theirs. The reverse
  would rename two authored ids to move one grid, and move every other
  derived id in the table with it.
- Header rows must come first (editor invariant); import reorders
  (normalizes) rather than rejects, same as the editor does.
- Export normalizes before flattening, mirroring the editor's own table
  normalization: cells sorted into column order, orphan cells dropped. Only
  a structurally unrecognizable subtree (missing row/column wrappers) is an
  export error.
- An empty plain-paragraph cell and an absent cell are the same thing:
  export writes `null` for both, import creates no cell block for `null`,
  `""`, or a bare empty paragraph (normalization, §11). Trailing empty
  cells are omitted (import pads).

### 6.2 Dataview

Dataview blocks embed a queryable view over objects — a *set* (live query)
or a *collection* (curated list, `is_collection: true`) — that they reference
but do not own.
Field-for-field from `Content.Dataview`, with cleaned names, snake_case
string enums, and defaults omitted:

```json
{
  "type": "dataview",
  "object_id": "bafyrei…targetSet",
  "properties": [
    { "property": "Name", "format": "text" },
    { "property": "Status", "format": "select" },
    { "property": "Due date", "format": "date" }
  ],
  "views": [
    {
      "id": "v1",
      "type": "kanban",
      "name": "By status",
      "group_by": "Status",
      "sorts": [
        { "property": "Due date", "direction": "asc", "empty_placement": "end" }
      ],
      "filters": [
        { "property": "Due date", "condition": "less", "date_preset": "current_week" },
        { "property": "Done", "condition": "equal", "value": false }
      ],
      "columns": [
        { "property": "Name" },
        { "property": "Due date", "width": 120, "align": "right" },
        { "property": "Status", "aggregation": "count_distinct" }
      ]
    }
  ]
}
```

**Dataview props** (`Content.Dataview`), canonical order as listed:

| Prop | Proto field | Notes |
|---|---|---|
| `object_id` | `TargetObjectId` | the set/collection object this view queries; empty for original set/collection objects and detached inline sets |
| `is_collection` | `is_collection` | |
| `source` | `source` | legacy, detached inline sets only; output-only (§4a) |
| `properties` | `relationLinks` | array of `{ "property", "format" }` — the properties available to this view, with formats per §3's vocabulary; `property` is the same member name the columns, sorts and filters use to refer to one (one spelling per concept). **This field is live** (maintained by the dataview editor), unlike the deprecated snapshot-level relationLinks |
| `views` | `views` | see below |

Dropped (normalization): `activeView` (local UI state; the proto itself
excludes it from changes) and the deprecated proto `relations` field.

**View props** (`Dataview.View`), canonical order: `id`, `type`
(`table · list · gallery · kanban · calendar · graph`, omit `table` — the public API currently says `grid`),
`name`, `group_by` (property key; from `groupRelationKey`), `cover_property`
(from `coverRelationKey`), `end_property` (from `endRelationKey`; the end
date of a range — **inert today**, see below), `hide_icon`, `card_size` (`small · medium · large`,
omit `small`), `cover_fit`, `colored_groups` (from `groupBackgroundColors`),
`page_size` (from `pageLimit`), `default_template_id`, `default_type_id` (from
`defaultObjectTypeId`), `wrap_content`, `list_size` (`compact · regular`,
omit `compact`), `alternate_rows`, then `sorts`, `filters`, `columns`,
`groups`, `object_orders`.

**View id uniqueness is scoped to the dataview block.** Two views of ONE
dataview may not share an `id` — that is a validation error naming both
positions — but two views in *different* dataview blocks may. This is the
only id domain in the format that is not document-wide (§4). Across blocks, each view is reached through its own block and
nothing is ambiguous — and the app itself produces that case: the default
view of every set, collection and type is minted with the literal id
`default`, and creating an inline set from an existing object copies that
object's views verbatim, so a page with two inline collections legitimately
holds two views called `default`.

Editor state nested per view, both output-only (§4a):

- `groups`: `[{ "id", "hidden", "background_color" }]` — kanban group
  display order (array order; the proto's per-group `index` is derived).
  From `Dataview.groupOrders`, matched by view id.
- `object_orders`: `[{ "group_id", "object_ids": […] }]` — manual object order
  within groups. From `Dataview.objectOrders`.

**Column** (`View.Relation`), canonical order: `property` (the property
key), `hidden` (inverse of proto `isVisible`; omitted = visible, so the
common case costs nothing), `width` (displayed column width in **pixels**,
see below), `aggregation`
(`count · count_value · count_distinct · count_empty · count_not_empty ·
percent_empty · percent_not_empty · sum · average · median · min · max · range`
— from proto `formula`; omit `none`), `align`. Deprecated per-column
date/time fields are dropped.

**Column `width` is in pixels**, a non-negative **integer** (the proto stores
an `int32`, and the schema says so, so `33.3` is a validation error rather
than a silent truncation to `33` — a fractional width is almost always a
percentage the author meant), the same unit as the proto's `width` — not
a percentage, and not a share of the table. A row of columns summing to
`100` produces four unreadable slivers, not four proportional columns.
Serialization passes the number through unchanged: the client owns
rendering, and this package neither clamps nor defaults it — so **omitting
`width` is the better default than guessing one**: the client already applies
sensible per-format defaults and never clamps a non-zero value on render, so
the choice tracks the client rather than freezing here. Write a number only
to pin a deliberate layout.

**There is no timeline/Gantt view, and `end_property` currently does
nothing.** The proto's view type enum ends at `Graph = 5`; the client
carries a sixth, `Timeline`, but it is gated behind `config.experimental`
and has no proto value, so it cannot be described here. `endRelationKey` is
read by that timeline component and by nothing else — a calendar view does
**not** use it, and shows single dates from `group_by` alone. `end_property`
round-trips faithfully for data that already carries it, but setting it on
any expressible view type has no effect. A view stored with the
experimental type reads back as `table`, since an out-of-range enum is
omitted rather than emitted as a schema-invalid empty string.

**Sort** (`Dataview.Sort`), canonical order: `property` (from
`RelationKey`), `direction` (`asc · desc · custom`, omit `asc`),
`custom_order` (for `custom`; select values by option **name** per §3, other
values verbatim), `empty_placement` (`start · end`, omit unspecified),
`include_time` (include time-of-day when comparing dates), `no_collate`
(disable locale-aware collation; compare raw strings), `id` (output-only).

**Dates are not empty-safe.** An object with no value for a date property
matches `less` and `less_or_equal` regardless of the threshold. An "overdue" view
must therefore pair the comparison with a `not_empty` on the same property
inside an `and` group; a `not_empty` under an `or` guards nothing.
`greater`/`greater_or_equal` are unaffected. Import warns on an unguarded
comparison rather than rejecting it — including undated objects is a legal
thing to want, and stored data contains such filters.

**Filter** (`Dataview.Filter`) — a filter node is either a **group** or a
**leaf** (schema `oneOf`); the top-level `filters` array combines its nodes
with an implicit **AND** (canonical form uses bare leaves at the top level;
a group exists only for `or` or nesting):

- group: `{ "operator": "and" | "or", "filters": [nodes…] }`. Export maps a
  proto node with non-empty `nestedFilters` to a group and drops its leaf
  fields; import writes `operator` only on groups (leaves get the proto
  default).
- leaf, canonical order: `property` (**required** — a leaf filter names the
  property it filters on, like the sort and the column beside it; export drops
  a filter whose stored relation key is empty rather than write a node that
  filters on nothing, §3), `condition`, `value`, `date_preset`,
  `include_time`, `nested_property` (reserved, output-only), `id`
  (output-only). `condition` values: `equal · not_equal · greater · less ·
  greater_or_equal · less_or_equal · contains · not_contains · in · not_in ·
  empty · not_empty · all_in · not_all_in · exact_in · not_exact_in · exists`
  (`contains`/`not_contains` from proto `Like`/`NotLike` — the public API
  agrees). `date_preset` from proto `quickOption` (`yesterday · today ·
  tomorrow · last_week · current_week · next_week · last_month · current_month ·
  next_month · number_of_days_ago · number_of_days_now · last_year · current_year ·
  next_year`, omit `exactDate`). `value`: for select/multi_select properties,
  option **names** per §3; dates stay unix numbers in the structured form;
  everything else verbatim. `value` is **dropped** on
  `empty`/`not_empty`/`exists` leaves (§11).

  **Dynamic values.** A `value` entry of the form `_filter_template_<n>_` is
  a placeholder the *client* substitutes for a real object id before issuing
  the query (`Dataview.valueTemplateMapper`): `_filter_template_2_` is the
  current user, resolving to `_participant_<space>_<account>`, and
  `_filter_template_1_` is the object hosting an inline dataview, resolving
  to its id. They are stored verbatim and are **opaque to the middleware** —
  nothing in Go resolves them, so a query evaluated server-side compares
  against the literal string and matches nothing. They are not object ids, and nothing in either direction rewrites them —
  object references are never compacted, so there is no legend one could be
  swallowed into (§9a). They are meaningful only on `objects`/`files`
  properties, since they resolve to an object id; on any other format the
  placeholder is stored UI state that matches nothing, and the mismatch is a
  **warning, not a refusal** — the same severity the neighbouring date-preset
  rule takes, for the same reason. Both doors warn — the fragment surface
  too, or one filter would validate on one door and be refused on the other.

  **A preset applies on a date property, under six conditions only.**
  `transformDateFilter` returns a filter whose format is not `date` before it
  computes anything at all; on a date filter it computes the range but
  substitutes it into the query for `equal`, `in`, `less`, `greater`,
  `less_or_equal` and `greater_or_equal` only. Fail either half — a preset on
  a text or select property, a preset under `not_equal` — and the preset is
  stored UI state with no effect on what the view matches. A preset resolves
  to a day *range*, and the condition picks the endpoint:
  `less`/`greater_or_equal` compare against the range start,
  `greater`/`less_or_equal` against its end, and `equal`/`in` expand into a
  pair bracketing both.

  A preset under a condition that does not apply is a **warning**, not an
  error: the author wrote "verified this week" and the view means "verified,
  ever", which is worth saying, but export writes the pairing because stored
  filters carry it, and refusing it would make one stored filter enough to
  make an object unexportable (§11, I1). The format half is not warned about,
  because the format of a filter's property usually comes from outside the
  document — the bundled table, the space — so "not a date" is as often "not
  known here", and a warning that fires on a correct filter makes every
  warning cheaper to ignore (§12).

  `number_of_days_ago` and `number_of_days_now` are the two presets that **take an
  operand**: `getDateRange` reads the day count from `value`
  (`pkg/lib/database/quickoptions.go`), so they are the one case where a
  preset and a `value` legitimately coexist, and **where the preset applies**
  — both halves of the gate above — a leaf carrying such a preset without a
  **day count** in `value` is a validation error: the count would default to
  `0`, silently meaning today. The rule reads the operand, not the member:
  `getDateRange` reads it with `domain.Value.Int64`, which answers `0` for a
  `null`, a string, a list — for every kind that is not a number — so those
  are the same silent "today" a missing member is, and a presence-only rule
  refused one and admitted the others. A day count is a **whole number in
  `[0, 36500]`**, the bound the compact grammar already puts on `daysAgo(n)`
  (§6.2.1): two forms of one filter language admit the same filters. Because
  the count is meaningful data rather than an absent field, export writes it
  even when it is `0`, overriding the usual empty-elision (§4) — and writes
  the count the query engine reads out of a stored operand that is not one
  (`0` for a non-number, the truncation for a fraction, the bound for
  anything past it), with an `OnWarning`, because the slot has one written
  form and a document carrying the junk verbatim is one this package's own
  `Validate` refuses (§11, I1). Anywhere the preset does not apply the rule does not
  either, because the count is never read: nothing is silently anything.

Sorts and filters do **not** carry the proto's cached per-node `format`:
import rehydrates it from the dataview `properties` list and `bundle`
(unresolvable keys get format 0, which the query engine tolerates).

Proto-default edge cases (implementation decisions): a leaf whose proto
condition is `None` (0) omits `condition` — absent means `None`; a proto
group node with operator `No` (0) exports as `"and"`; contentless filter
nodes (groups with no live children, leaves carrying at most an id) and
sorts without a property key are no-ops and are dropped on export;
out-of-range proto enum values are omitted rather than serialized (an
unknown *text style* is an export error — silently restyling content would
be worse).

#### 6.2.1 Compact filter syntax — shipped grammar, reserved document field

**Status: split scope.** The grammar below and its parser ship
**now**, as the library subpackage `pkg/lib/anyblockjson/filterstring`
(§13): parse a filter string → the §6.2 structured filter tree
(`model.BlockContentDataviewFilter` nodes), with **offset-addressed
errors** naming the offending token and its position. Its consumer is the
API v2 request surface (`POST …/search` and the `filter` field of
`POST …/sets`), where the string is the documented
small-model form and both request forms land on one internal tree. The
grammar is thereby pinned by the parser and served via the API's discovery
surface.

The **document** side is unchanged and stays reserved: v1 documents ship
the structured `filters` array only. The view field name `filter`
(singular, string) is **reserved** for a post-v1 extension: v1 schemas do
not define it, so introducing it later is a version bump (§10 — a
v1.0 reader encountering it reports "produced by a newer version"; export
keeps writing the structured array; the `CompactFilters` export option
stays reserved in `Options`). When that lands, the two forms coexist
permanently — `filter` and `filters` mutually exclusive per view, import
accepting both, export choosing via option. One consequence of raw-name
addressing (§3) is already known for that future field: a display name is
not a bare identifier in this grammar, so the document-side form will need
a quoted-key production (`"Due date" < currentWeek()`); the bare-key
grammar below is the API request surface's, whose key convention is a
separate decision.

The design, normative for the parser (and unchanged for the future
document extension): a view carries its filter as a single SQL/JQL-flavored
query string:

```json
{ "type": "kanban", "group_by": "Status",
  "filter": "done = false AND (due_date < currentWeek() OR due_date IS EMPTY)" }
```

Grammar (informal here; the `filterstring` parser is the normative
artifact, and the EBNF it pins is what the API discovery surface serves):
`OR` over `AND` over parenthesized groups over leaves; `AND` binds tighter,
parentheses group. There is deliberately **no free-standing `NOT (…)`** —
the internal model has no NOT-group; negation exists only in negated
conditions, keeping string ⇄ structured 1:1.

| Condition | Syntax |
|---|---|
| equal / not_equal | `priority = 3` / `priority != 3` |
| greater / less / greater_or_equal / less_or_equal | `> < >= <=` |
| contains / not_contains | `name CONTAINS "report"` / `NOT CONTAINS` |
| in / not_in | `status IN ("In progress", "Blocked")` / `NOT IN (…)` |
| all_in / not_all_in | `tags HAS ALL ("urgent", "q3")` / `NOT HAS ALL (…)` |
| exact_in / not_exact_in | `tags = ("a", "b")` / `!= (…)` — set literal on the right |
| empty / not_empty | `assignee IS EMPTY` / `IS NOT EMPTY` |
| exists | `assignee EXISTS` |

Values: double-quoted strings, bare numbers, `true`/`false`, RFC 3339 dates
in quotes (`due_date < "2026-08-01"`), and date-preset **functions** —
`yesterday() · today() · tomorrow() · lastWeek() · currentWeek() ·
nextWeek() · lastMonth() · currentMonth() · nextMonth() · lastYear() ·
currentYear() · nextYear() · daysAgo(n) · daysFromNow(n)` (the parameterized
pair maps to `number_of_days_ago`/`number_of_days_now` with the value as `n`;
parens distinguish presets from string literals).

**Property keys are bare identifiers, and they reach a spelling through the
fold.** The grammar has no quoted-key form, so a key is written with
identifier characters only (the exact charset is given below) and must not be one of
the grammar's reserved words. That is narrower than what a property may be
SPELLED, since a spelling is a display name and names carry spaces: `Due
date` cannot be written here. It does not have to be, because resolution
folds away case and separators, so the bare `due_date` addresses it — and
`Дата_выполнения` addresses "Дата выполнения" the same way. What no
identifier folds onto — `C++`, `50% done`, a name colliding with `AND` or
`IS` — has no compact form at all, and the parser says so and names the
structured `filters` array as the way to express it.
Select/multi_select values are option **names**, per §3 (the structured form
agrees; only date values differ — RFC 3339 here, unix numbers
there). The RFC 3339 → unix conversion is **format-driven, not
string-driven**: it happens only for keys whose format resolves to `date`
through the consumer-wired `Options.ResolveFormat` (a date-looking string
on a text property stays a string; a non-RFC-3339 string on a date
property is a parse error steering to the presets). A consumer that wires
no resolver gets string values verbatim — executing such a filter against
date properties matches nothing, so query surfaces MUST wire the resolver.

Parser interpretation calls (normative, matching the shipped parser):
keywords match **case-insensitively** (`and` ≡ `AND`) and are **reserved**
— none can be a bare property key (a colliding key is reachable only
through the structured form); property keys are Unicode identifiers
(`identStart identPart*` — letters of any script, digits, `_`, and the
combining marks the vowels of Indic and SE-Asian scripts are written with), **the
grammar §3 mints every label through**, so a key a document spells can
always be written here — the reason `50% done` labels `_50_done` and a
bson-keyed property labels its name rather than its key; presets are **excluded
from value lists** and from conditions the engine does not transform
(`notEqual`, `contains`, …: only `= > < >= <=` take a preset); set
literals require `=` / `!=` (a list after an ordering operator errors);
the counting presets take a whole day count in `[0, 36500]`. Bounds: the
input is capped at **4096 bytes** and parenthesis nesting at **32** (the
§4 document nesting bound) — both are ordinary offset-addressed parse
errors.

Canonical rendering: uppercase keywords, `", "` separators, double quotes
with backslash escapes, parentheses only where precedence requires. Export
will keep writing the structured array by default; a future `CompactFilters`
option will emit the string form for any view whose filter is fully
expressible (every leaf free of output-only fields like `nested_property`,
every option name resolvable), falling back to the structured array per
view otherwise. Import will accept both forms; string-parse errors report
the view's JSON path, the offending token, and its position.

## 7. Structural blocks

The following blocks are **derivable** and are dropped on export:

- the root block (implicit, §2),
- the header wrapper and its children `title`, `description`,
  `featured_properties` — their content duplicates `properties.name` /
  `properties.description`.

Import does **not** attempt to rebuild them: which structural blocks an
object gets depends on its layout (note objects have no title block at all,
todo objects bind `done`, …), which the editor resolves from the type's
recommended layout at first open (`template.InitTemplate`). The package
preserves `resolvedLayout` in `properties` (§3) and leaves structural blocks
absent; the editor regenerates them on open. `N(S)` in §11 is defined
accordingly.

A document that nevertheless contains such blocks at indent 0 is accepted
(agents will produce them): import merges `title` / `description` text into
the corresponding properties when those are unset and drops the blocks
otherwise — together with any blocks indented under them; a top-level
`featured_properties` block (which carries no content) is simply dropped.

**The primary dataview** is the one structural id import *does* rebuild.
Object types, sets and collections keep their own dataview at the fixed
block id `dataview` (`state.DataviewBlockID`); the editor recreates it on
open only *if absent* (`template.WithDataviewIDIfNotExists`), so a document
whose dataview lands on a generated id gets a second, empty dataview
alongside the configured one. Unlike `title`/`description`, the block cannot
simply be dropped and regenerated — its views, columns and widths are the
author's configuration, not derivable — so import **pins the id** instead:

> the first indent-0 `dataview` block with neither an explicit `id` nor an
> `object_id` becomes `dataview`.

`object_id` is what separates the two cases: an inline view of *another* set
or collection has it set (§6.2) and keeps its generated id, as does any
dataview nested below indent 0, and any dataview after the first. If some
block already claims `dataview`, that block wins and nothing is pinned — an
explicit id stays authoritative and cannot collide (§13). Export is
unchanged: it emits the id verbatim, and under `omitIds` (§9) the rule
restores it on the way back in.

**Content-less blocks** (legacy data): old accounts hold blocks whose
content oneof is unset — relation objects wrap their "used in" dataview in
one, and pages can contain orphaned empty leaves. They are transparent
containers (§7a): the block is dropped either way, and a subtree under one is
lifted into its place.

## 7a. Transparent containers

A block is a **transparent container** when it contributes containment and
nothing else:

- its content is `Layout` with style **`Div`** (`model.BlockContentLayout_Div`
  — the editor's fan-out wrapper, minted by `state.wrapChildrenToDiv` when a
  parent exceeds `maxChildrenThreshold` children), or
- its content oneof is **unset** (legacy data), with or without children.

The test is on **content**, never on the `div-` id prefix the normalizer
mints. Keying on a prefix would make id *spelling* semantically load-bearing,
and it would leave an authored
`{"type": "group"}` round-tripping into a permanent wrapper.

**Export** writes nothing for a container and emits its children at the
container's **own indent**, with the container's own top-level status. In
JSON terms: `group` is a type no export ever produces, on any surface.
Consequences, stated so they are not re-derived:

- A **childless** container emits nothing at all.
- **Nested containers collapse fully**: a chain of *n* removes *n* levels.
- **Every attribute the container carried goes with it** — `align`,
  `vertical_align`, `background_color`, `fields`. No conditional
  preservation; recorded in `N(S)` (§11) and reported through
  `Options.OnWarning` when there was anything to lose.
- The lift runs **before** the depth bound is checked, so both the value
  compared against 32 and the emitted `indent` are post-lift.
- A container at indent 0 is transparent for §7 too: a structural block
  underneath it is at the document's top level and is dropped there, rather
  than being preserved by the accident of a wrapper standing over it.
- The rule applies on every export surface — the document, a table cell's
  descendants, and a block subtree (§13.1) **including its root**, since no
  read surface ever serves a container id, so no caller can address one
  except out of a stale cache. A subtree rooted at a container marshals as
  its lifted children; rooted at a childless one, as an empty run.

**Import** does the inverse, as a pre-pass over the flat run: a `group` entry
contributes no block, and every following entry indented deeper than it
re-bases one level shallower — recursively, for nested containers. Any
attribute on the entry is ignored, and so is its id. The lift runs **before**
the primary-dataview pin and before top-level structural absorption (§7),
which is what lets a wrapped dataview be seen at the indent-0 position the pin
requires and a wrapped `title` be absorbed into `properties.name`. Because the
lift is positional, a lifted structural block is at indent 0 for every
purpose, on both sides.

Monotonicity survives by construction, so the lift can never manufacture an F6
violation: a container at indent *g* satisfied *g ≤ p+1*, and its first child,
at *g+1*, lands at *g*.

**The two positions that address exactly one block cannot lift, and say so
rather than resolving to nothing:** the single-block fragment entry point
(§13.1) refuses a lone container, and a table **cell's own block** cannot be
one — a cell is a position, not a run — which `Validate` refuses too, so the
two agree. That holds for **both cell spellings** (§6.1): the array form is
refused at index 0 of the run, the object form on the cell itself. They are
separate checks because they are separate readers (I2, in the one shape §7a
cannot lift). A cell whose
stored block *is* a container renders as an empty cell.

**Containment (§12) is judged against the lifted tree**, because that is the
tree import builds. `row > group > column` is **valid**: it says
`row > column`. `row > group > paragraph` is invalid and is reported against
the row, naming the container in between (`nested under a group inside a row
— a row block can only contain column blocks, got paragraph`), or the message
reads as wrong to whoever wrote the `group`. A container is itself exempt from
the check: it becomes nothing, so there is nothing to place.

**What comes back.** Nothing in this format re-creates a container, and no
importer wiring is needed: the editor's own normalization re-wraps on
`ApplyState`, which runs on creation and on every cache load. It puts back a
**different** partition — the split point is a function of arrival order, not
of the document — and for a document whose content has since shrunk below the
threshold it puts back nothing at all. Both are covered by `N(S)` (§11).

**The re-wrapping is an obligation on the wiring, not the format:** the
re-wrapping is the editor's, not the format's. A writer that builds a
snapshot and stores it WITHOUT going through the object-creation path that
enables layouts (`EnableLayouts`) will land a thousand-child object in front of a
renderer the threshold exists to protect. Every path that writes an imported
document has to run the editor's apply, exactly as the import wiring does
today.

**The other five layout styles are unaffected**: `Row` and `Column` are
author-created and grammar-bearing (a column carries `fields.width`),
`Header` is structural (§7), and `TableRows`/`TableColumns` belong to a
table's internals (§6.1). A stray `TableRows`/`TableColumns` outside a table
still drops its whole subtree, deliberately: folding it into this rule would
put table cells at top level.

## 8. Rich text: inline markup

Text-bearing blocks carry a single `text` string. Formatting is expressed
inline — **offsets never appear in the format.**

```json
{
  "type": "paragraph",
  "text": "Ship the **new export** by Q3 with <mention object_id=\"bafyreidf…\">Roman</mention>"
}
```

### 8.1 Grammar

A CommonMark-inline subset plus a small whitelist of inline tags for marks
with no Markdown equivalent:

| Syntax | Proto `Mark.Type` | Notes |
|---|---|---|
| `**text**` | Bold | |
| `*text*` | Italic | canonical form; `_text_` accepted on input |
| `~~text~~` | Strikethrough | |
| `` `text` `` | Keyboard | inline code; content is literal (CommonMark code-span rules, §8.2) |
| `[text](url)` | Link | external URLs |
| `[text](anytype://object?objectId=<id>)` | Object | inline link to an Anytype object — Anytype's standard deep-link shape. The form is **exact**: scheme `anytype`, host `object`, and a single `object_id` parameter, with the id percent-encoded. Any other `anytype://` destination — a second parameter, a different host, a path — is **not** an object reference and stays a plain Link, preserved verbatim (§10) |
| `<mention object_id="<id>">text</mention>` | Mention | decorated object reference (icon + name in UI) |
| `<u>text</u>` | Underscored | standard HTML |
| `<font color="red">text</font>` | TextColor | Anytype color names |
| `<font background="yellow">text</font>` | BackgroundColor | coincident color+background ranges combine into one tag: `<font color="red" background="yellow">` |
| — | Emoji | not writable: export **materializes** the mark by splicing its emoji over the covered text (the mark's semantics are replacement; this matches the Markdown export and the chat renderer). On import emoji are plain text |

Inline tags: a tag name and an attribute name are `[A-Za-z][A-Za-z_]*` —
snake_case like every other identifier the format defines (§1 *Naming*), which
is why `object_id` is an attribute name and not the attribute `object`
followed by a stray character. Import accepts any attribute order, single or
double quotes, and surrounding whitespace; canonical form is double quotes,
single spaces, `color` before `background`. An attribute the tag does not
define is an error naming it (§12), so a document written against an older
draft fails loudly rather than dropping the mark. Zero-length tags (e.g.
`<mention object_id="x"></mention>`) are dropped on input.

Everything else is literal text. No other Markdown constructs are recognized
inside `text` — no block syntax, no images, no autolinks, no HTML beyond the
whitelisted tags. `\n` inside `text` is a soft line break within the block
(Shift+Enter), encoded as the JSON `\n` escape.

### 8.2 Escaping

CommonMark backslash escapes apply: literal `*`, `` ` ``, `[`, `]`, `~`, `<`,
`\` in prose are written `\*`, `` \` ``, `\[`, `\]`, `\~`, `\<`, `\\`. Input
additionally accepts HTML entities (`&lt;`, `&amp;`). Canonical export uses
backslash escapes, applied minimally (only where the character would
otherwise be parsed as markup).

Code spans follow CommonMark, where backslash escapes do **not** apply:
content containing backticks is delimited by the shortest backtick run not
present in the content, space-padded when the content starts or ends with a
backtick (`` `` `code` `` ``).

**Canonical escaping, made precise** (implementation decision — "minimally"
is defined as the following deterministic rule set; at internal mark
boundaries the unseen neighbor is treated as punctuation, conservatively):

- `*` — escaped unless whitespace on both sides.
- `_` — escaped iff it could open or close under underscore flanking
  (intraword underscores stay literal).
- `` ` `` — always escaped in prose.
- `~` — escaped when adjacent to another tilde or sitting at a mark
  boundary. On input, only runs of exactly two tildes are strikethrough
  delimiters; other run lengths are literal.
- `[` — always escaped in prose (a bare `[` could assemble a false link
  with text from a later mark segment; no local lookahead can rule it out).
- `]` — escaped only inside link labels.
- `<` — escaped before any **tag-shaped** sequence: `<`, an optional `/`,
  then at least one ASCII letter. Deliberately wider than the three tag
  names version 2 knows: `<sub>x</sub>` in prose exports as
  `\<sub>x\</sub>`. This is the tag namespace's **reserved syntax space**
  (§10) — see the note below.
- `&` — escaped only where a valid entity follows. Recognized entities:
  `lt gt amp quot apos nbsp` and numeric (`&#65;`, `&#x41;`).
- `\` — escaped when followed by ASCII punctuation (input accepts a
  backslash before any ASCII punctuation as an escape, per CommonMark).

**Reserved syntax space** (the one escaping rule that is not minimal, and
why). A `text` string carries no version marker, so bytes are the only thing
a later version has to work with. If canonical output escaped `<` only for
`u`/`font`/`mention`, a version-2 document could contain a literal
`<sub>x</sub>`, and the day a version adds `sub` those same bytes read as
markup — the reader cannot tell version-2-literal from version-3-markup, and
because a malformed instance of a *known* tag is an error (§8.3), a stored
document that was valid could become invalid. Escaping the tag *shape*
closes that: in canonical output, an unescaped `<` is never followed by a
letter, so the entire `</?[A-Za-z]…>` space is free for any future version to
define, with no text-rewriting migration and no ambiguity. The cost is a
backslash on prose that looks like markup (`a\<b`).

The delimiter namespace is closed instead of reserved: `**`, `*`, `~~`,
`` ` ``, `[…](…)` are the complete set (§10), and a future mark arrives as a
tag rather than as new punctuation. That is what makes `==mark==`, `~one~`,
`^sup^` and friends safe to leave literal and unescaped forever.

Link destinations render bare with `\`-escaped `` \ ( ) & < [ ] ` ``, or
angle-wrapped (`<…>`) when the URL contains whitespace (brackets and
backticks escaped there too — raw ones would join the enclosing label or
code-span scan when links nest); entities are decoded in destinations and
attribute values on input. `_` delimiter runs parse exactly like `*` runs
(so `__x__` is bold — liberal input; canonical output always uses stars).

**Resource bounds** (implementation decision — deterministic local rules
that keep parsing linear on the untrusted-document boundary): link
destinations longer than 2048 UTF-16 code units, destinations surrounded by
more than 32 whitespace characters, and link labels nested more than 32
deep are not recognized — the `[` stays literal. Export drops Link/Object
marks whose rendered destination would exceed the bound, and Emoji marks
whose param exceeds 64 code units, as invalid (§8.3 step 1), so round trips
stay byte-stable.

### 8.3 Canonical rendering (the round-trip contract for marks)

Internal marks are ranges over UTF-16 code units and may overlap arbitrarily.
Export:

1. Materialize Emoji marks (§8.1). Drop zero-length and invalid ranges.
2. Normalize boundaries: Markdown-delimited marks (`**`, `*`, `~~`, `` ` ``)
   shrink past leading/trailing whitespace at their boundaries — whitespace
   at a boundary carries no visible styling, and CommonMark's flanking rules
   reject delimiters against whitespace. Tag-delimited marks (`<u>`,
   `<font>`, mention) and links are unaffected.
3. Resolve same-type overlaps: two marks of the same type with different
   params (two links, two mentions) cannot wrap one segment — the
   earlier-starting mark wins the overlap and the later range is truncated
   to start where the earlier ends (zero-length results dropped).
4. Split the text at every remaining mark boundary; each segment carries its
   mark set.
5. Emit segments left to right, opening/closing delimiters so that nesting
   is deterministic — fixed order outermost→innermost: Mention, Object,
   Link, `<font color>`, `<font background>` (coincident ranges combine into
   one tag), `<u>`, `~~`, `**`, `*`, `` ` ``. Delimiters shared by adjacent
   segments stay open (maximal runs).

Implementation decisions:

- **Step 1 details**: "invalid" ranges are out-of-bounds, inverted,
  zero-length, or splitting a UTF-16 surrogate pair; a param-carrying mark
  (link, mention, object, colors, emoji) with an empty param is dropped;
  a param on a param-less mark type is cleared (so equal ranges merge);
  params beyond the §8.2 resource bounds are dropped. A **Link mark whose
  param is exactly the `anytype://object?objectId=<id>` deep-link (§8.1 —
  one parameter, nothing else) normalizes to an Object mark** — the two
  render identically, and without the normalization the parse-back type flip
  would change same-type overlap resolution. A Link carrying any *other*
  `anytype://` destination is left alone: reinterpreting it would have to
  guess which part is the id, and guessing wrong is unrecoverable, whereas
  preserving it verbatim always round-trips.
- **Step 2 extension**: emphasis-family marks (`**`, `*`, `~~`) additionally
  exclude any whitespace run touched by a *stack-outer* mark's endpoint —
  the outer change forces the emphasis delimiter to close/reopen inside the
  run, and an emphasis delimiter against whitespace cannot re-parse
  (flanking). Whitespace styling is invisible for these types, so the split
  is a rendering no-op.
- **Step 3 tie-break**: at equal starts the longer range wins; the shorter
  same-type range is truncated to nothing and dropped.

Import parses the grammar back to ranges: each maximal contiguous run of a
mark becomes one range; offsets are computed in UTF-16 code units (matching
editor semantics; `util/text` helpers).

**Parser discipline** (implementation decision): the parser is the exact
inverse of the canonical renderer — a deterministic delimiter stack (close
the top entry while it matches, open with the remainder, demote what can do
neither to literal text), *not* CommonMark's delimiter-run algorithm. The
rule-of-three resolution is not invertible, and §11's byte-stability over
arbitrarily overlapping ranges requires an exact inverse; the grammar stays
syntax-compatible with CommonMark/anymark for well-formed input. Unmatched
Markdown delimiters demote to literal text (CommonMark spirit); malformed,
unclosed, or misnested *whitelisted tags* are validation errors (§12) —
once `<u`/`<font`/`<mention` is recognized, strictness gives agents a real
error instead of silent text. Import emits marks sorted by (from asc, to
desc, nesting order, param).

Consequence: a document whose overlapping ranges are exported and re-imported
gets equivalent-but-normalized marks (same styled rendering, possibly
different range decomposition), and adjacent same-type mark ranges merge.
This — plus emoji materialization and steps 2–3 — is the intended
normalization; §11 defines round-trip up to it.

### 8.4 Literal text blocks

`code` and `embed` blocks are **not** parsed for inline markup: their `text`
is the raw content, verbatim (only JSON string escaping applies). Stored marks
on such blocks are dropped on export (§5).

## 9. Ids and references

- Block ids are **optional on input**: a document written entirely without
  `id`s (blocks, table rows/columns, views) is valid; import generates them
  on insert. This is the expected shape for agent-generated documents.
- All `object_id` props and mark targets are object ids, opaque to this
  format; there are no intra-document block references (table cell ids are
  derived, §6.1).
- Import id policy: missing → generated (the editor's standard generator);
  provided → validated for uniqueness (§4) and charset, preserved so that
  re-exports diff cleanly.
- On output, export writes ids by default (stable diffs, §11 canon). The
  `OmitIds` marshal option (§13) instead drops **every id in the document**
  — blocks, table rows/columns, views, sort/filter ids — along with the
  id-dependent output-only view state (`groups`, `object_orders`) and the
  `option_ids` legend (§9a), which is nothing but ids: a shape that declares
  itself id-less and then ships a map of them is not one. For templates,
  prompt examples, and any content meant to be re-inserted rather than
  diffed. An id-less export is valid but not the canonical round-trip form
  (re-importing mints fresh ids, and option values resolve by name).

**What dropping `option_ids` costs, precisely.** "Option values resolve by
name" is not a neutral fallback, and a reader choosing this flag should have
the number. A name is not an identity: a space may hold two distinct options
of one property under one name, and name resolution answers the **first** one
the resolver lists (ANOMALIES.md #6). On a 34 339-object sweep that put **7
objects** on an option they had never been on. `option_ids` is precisely what
closes that (§9a), so an `OmitIds` document read back into the space it came
from can move such a value onto the sibling option — silently, because the
two options read identically in the document and in the UI. An option renamed
between writing and reading is the same loss from the other end: with no id to
fall back on, the wiring mints a *second* option under the stale name.

This is **accepted, not fixed** — the shape's entire content is "no ids"
(§9a). The loss is small, real, and rare, on the read/prompt path, which is
the one an agent sees most.

**Export does not warn about it.** At the moment export substitutes a
name for an id it could ask the resolver the question *import* will ask —
`OptionId(key, name)` — and warn whenever the answer is a different id. That
probe would be exact and costs nothing (the option list is already loaded by
the `OptionName` call beside it). It is declined because the loss belongs to
name resolution, not to `OmitIds`: the legend is a **hint**, honoured only
where the target space still serves the id as a live option of that relation
(§9a), so a *default*-shape document read into any other space degrades in
exactly the same way. A warning gated on the flag would therefore assert what
export cannot know — that the legend will be honoured — while staying silent
on the identical loss under the default shape. Ungated, it says "an option
name in this document is ambiguous in the space it came from" on every export
of that data, forever, about something no edit to the document can fix, and
§12's rule on the cost of a marginal check applies to the export channel too.
The place the question can be answered is the reader's, where the destination
space is known — and `OptionResolver` cannot answer it there either, having no
way to enumerate a relation's options. That is a gap in the resolver
interface, recorded here rather than papered over with a signal that is right
by accident.

### Object references: `id`, optionally `id#name`

An object reference is a full id, and it MAY carry an informative name after
a `#`:

```json
"Related":    ["bafyrei…#local_first_ux"],
"Assignee":   ["A6eK73Jm…#roma_kha"],
"Created by": "A6eK73Jm…#roma_kha",
"id":       "A6eK73Jm…"
```

- **The suffix is informative only.** Import trims it at the FIRST `#`,
  unread; nothing resolves it, nothing requires it, and nothing depends on
  it being unique — two objects sharing a display name suffix identically
  and collide on nothing. Do not resolve by it.
- **A bare id is exactly as valid and imports identically.** A model writing
  a new reference has no name to add and must not need one. This is an §11
  I2 surface.
- **The split at the first `#` is unconditional and safe from both ends.**
  No id form this format writes can contain `#`: CIDs are base32
  `[a-z2-7]`, participant ids base32+base58, the bundled `_ot`/`_br` slugs
  are `[a-zA-Z0-9_]` across all 223 keys, `_date_…`/`_missing_object` are
  fixed shapes — measured over 81,696 production documents across two
  corpora, zero values in a reference slot contain one. (One id-shaped
  value elsewhere does: a `uniqueKey` derived from an option named `C#`.
  Option names are free text and mint keys from it, so the split's safety
  rests on object ids never being derived from a uniqueKey, which they are
  not.) And the name half is **normalized through the same
  identifier grammar key labels use** (§3: letters of any script, digits,
  `_`, combining marks — `letter | digit | _ | mark`), which admits no `#`
  either, truncated at 64 characters (a hint, not an address, so truncation
  invents nothing). A writer MUST normalize; a raw display name would break
  the split from both ends.
- **Writing a suffix needs a name.** Export asks `ResolveObjectNames`
  (§13) about the STORED id and writes the suffix only where it answers
  with a name that survives normalization — never a partial or invented
  one. No resolver, bare ids, everywhere.
- **Opt-in per shape.** The suffix rides `Options.RefNames`, default OFF:
  the export/backup shape stays minimal and stable under renames of
  referenced objects (a rename would otherwise dirty every backup diff);
  read shapes opt in, the way they opt into `CompactBlockLabels`. `OmitIds`
  is orthogonal — it drops doc-local ids, and object references are
  content, not doc-local ids. The one exception is attribution (§3), whose
  suffix rides the participant resolver instead: those two values are
  dropped on import, so no shape's stability is at stake.
- **The slots**: object/file-format property values, `items`, every block
  `object_id` (link, file, bookmark, dataview), object-valued filter
  `value`s and sort `custom_order` entries, `object_orders[].object_ids`,
  and the two attribution properties. NOT on ids that already say what they
  mean — a `_date_…` reference, the `_missing_object` sentinel, a
  `_filter_template_…` placeholder — and NOT on non-reference slots: a
  select value is an option NAME already (and may legitimately contain `#`,
  as in `C#` — import trims nothing there), a date is a date. Mention and
  object-link targets inside `text` keep their ids verbatim: the mention's
  own text already names the target.
- **A caption is only written where it can be taken off.** A snapshot's
  reference slots are untrusted (§11) and may hold an id this format could
  not have written. Export therefore captions no id that already contains a
  `#`, because `x#y` + `#name` reads back as `x` — the caption would be
  paid for with the id itself. The degenerate case is worse: `#name` has no
  id half, `splitRefName` refuses to split at index 0 precisely so import
  never invents an empty id, so it imports whole and an export willing to
  caption it would append again every generation, without bound. That shape
  is what a writer produces copying only the readable half of `id#name`;
  Validate reports it as a warning wherever it can see the property's
  format (`/properties/<key>`, bundled table or a declared format — a
  space-minted key it cannot resolve passes unremarked), and the value is
  stored as written, addressing nothing.
- **An id containing `#` still loses its tail on read**, since the reader
  cannot tell that `#` from a caption's. It is the format's one reference
  normalization, listed in `N(S)` (§11), and it converges after one
  generation.
- **Round trip**: byte-stable given the same resolver — import trims, the
  next export re-derives the same names. Absent a resolver the suffix is
  absent, the same class of resolver-dependence as option names (§3).

### The participant fold

`_participant_<spaceId>_<identity>` is a derived id
(`core/domain.NewParticipantId`): the space half restates the document's own
space, and the 48-character identity is the whole of the content. Every
reference slot above folds it to the bare identity on export, and import
rebuilds the composite against `Options.SpaceId` (§13):

- **The trigger is the VALUE's shape, never the property name.** The
  heaviest participant slots in production are space-minted custom
  properties (`owner`, `voters`, …) with no declared target type, and
  `assignee`/`author` may legitimately hold a contact. The classifier is the
  identity's own strkey checksum (`crypto.DecodeAccountAddress`): no CID,
  bson id or `_`-prefixed derived id can pass it, so unfold cannot fire on
  anything else.
- **The participant document's own envelope `id` folds too** — otherwise a
  reader could not textually join a folded reference to the document it
  points at. This makes participants the documented special case in the
  envelope `id` slot, which otherwise always holds a real object id; import
  rebuilds the composite as the object id and the root block id.
- **`Options.SpaceId` arms it, in both directions at once.** The format
  carries no space id anywhere in the envelope, so the wiring supplies one
  exactly as it supplies resolvers (`storeresolver` wires the index's own).
  With no SpaceId nothing folds and nothing unfolds. **Any reader of a
  folded document MUST set it** — it is the space the document is being
  read INTO, which every importer necessarily knows, since an import lands
  in a space. A reader that names none stores the bare identity where a
  composite belongs, addressing no object; because the classifier is exact,
  the reader knows this has happened and reports it through the warning
  sink, once for the document (§13). The one caller with genuinely no
  target space is a converter that does not import — `cmd/anyblockconvert`
  — and it is the path the warning exists for.
- **An empty identity is not an identity.** `_participant_<space>_`, built
  from a blank identity, addresses nobody; 9,103 of 37,429 production
  objects store one in `lastModifiedBy`. It does not fold — and only the
  classifier refuses it, since `NewParticipantId(space, "")` rebuilds that
  exact string, so the round-trip recheck cannot. Without the classifier it
  would fold to the empty string and the reference would be deleted.
- **Only this space's composites fold.** A composite embedding a DIFFERENT
  space passes through whole in both directions: folding it would silently
  re-home the member on import. (A document carried into another space
  re-homes deliberately and correctly, because its folded references
  rebuild against the READER's SpaceId.)
- Measured (37,429 production objects): 3,446 same-space composite
  occurrences across properties, `items`, block `object_id`s, filter
  values, object orders and the participants' own envelope ids — all fold,
  none remain. The corpus held zero cross-space composites.

### References the space cannot serve

A reference to an object that does not exist in the SPACE is not written as
if it did. The space stores already state this for the references
their importers resolved — `_missing_object`
(`pkg/lib/localstore/addr.MissingObject`) stands 1,089 times across a
28,617-document corpus — and export now applies the same honesty to ids
that dangle without the sentinel, and a consistent policy to the sentinels
it re-exports.

**The split is by what the slot can express.**

- A **singular slot** — a block's `object_id` (link, bookmark, file, image,
  video, audio, pdf, dataview) and a `<mention object_id="…">` target (§8)
  — REWRITES the id to `_missing_object`. Omission cannot express "no
  target" there: only deleting the block (or the mark) could, and that
  would lose the fact that a link or mention existed — the mention's text
  stays, only its address is gone. A stored sentinel is kept as-is.
- A **list slot** — an objects/files property value (§3), a property
  document's `object_types` (§2d) — DROPS the entry: a list expresses
  absence by being shorter. A stored sentinel drops too. The emptied list
  stays `[]`, never omitted: the key's presence is meaningful (§3), and for
  `object_types` an empty list is a cleared target set (§2d).
- Everything else is deliberately out of scope: collection `items`, filter
  values, custom orders, `object_orders`, a type's `default_template_id`,
  and object-link marks keep their ids verbatim. Each of those can be
  extended later on this section's precedent; none was in the evidence.

**"Missing from this export" and "missing from the space" are different
facts, and only the second may cause a rewrite.** An export of a single
object references its neighbours in the space; those objects exist and were
simply not exported, and rewriting them would corrupt a perfectly good
export. The exporter never sees the export set — it works one document at a
time — so the only question it can ask is of the STORE, which is the right
question: does this space hold a row for this id? The answer comes through
the `ObjectExistenceResolver` capability (§13), asked affirmatively —
`known && !exists` — so a store failure moves nothing. And it is a NEW
capability because the resolver already standing in the object namespace
cannot answer it: `ObjectNameResolver`'s ok is `name != ""`, which reads
"exists but untitled" as "no". Untitled objects are common; conflating the
two questions rewrites live references.

**The question only reaches ids the space index is the authority for**:
CID-shaped ids (`isObjectIdShaped` — `cid.Decode` behind a length gate).
A `_date_…` id is virtual, `_ot…`/`_br…` resolve against the bundled
tables, a participant composite against the fold, a bare type key against
the key vocabulary, a widget link target against the editor's constants —
a store that was never an id's authority cannot declare it missing, so
none of those are ever asked about, let alone rewritten. A deleted
object's tombstone is a row: its id still means something in this space,
and references to it are untouched — with ONE deliberate exception, the
icon. An icon is optional where a link or mention target is not, so an
`iconImage` whose target the space DELETED is dropped rather than kept or
rewritten: export asks the narrower question through the
`ObjectDeletionResolver` capability (§13, `DroppedDeletedIconRef` — the
predicate is exported so the comparator applies the same rule), and the
document falls through to whatever icon channel is left, exactly as an
image that is not an object id already does (§2b). Measured before the
rule: 134 corpus bookmark documents shipped an icon pointing at a favicon
whose file object was a tombstone in their own space's store. A store
failure (`known == false`) drops nothing, and no other reference slot asks
about deletion at all.

**With no capability wired, nothing moves — the sentinel included.** A
package-only export passes every reference through verbatim, exactly as
before this rule existed: the absence of an answer is not evidence of
absence, and the offline round trip stays byte-exact.

**Warnings follow what is lost.** A rewrite or a real-id drop destroys the
stored id — the warning is that id's last appearance anywhere — so both
warn, naming the id. A stored sentinel kept or dropped says nothing: which
object it was is already gone, and ~990 silent sentinel drops per corpus
would drown the channel §12 just reclaimed.

**Round trip**: the change converges in one generation and is a fixpoint
after — the first export rewrites and drops, import stores what was
written, and `Export(Import(Export(S))) = Export(S)` holds (§11 guarantee
3). The comparator applies the same exported predicate
(`DroppedMissingObjectRef`) to both sides, so a dropped-by-design entry is
a normalization, not loss (§11).

### 9a. The legends, and compact ids

The envelope carries **three legends** and no other indirection. Each answers
one question the rest of the document cannot:

| legend | maps | question |
|---|---|---|
| `property_internal_keys` | property spelling → stored relation key | which relation does this spelling name? (§3) |
| `type_internal_keys` | type spelling → stored type key | which type does this spelling name? (§3) |
| `option_ids` | property spelling → (option name → option id) | which option does this name mean? (§3) |

Three maps rather than one, and `option_ids` nested rather than flat, for one
reason stated twice at two scales: **a name in this format is arbitrary user
text, so no character can be reserved to join it to its scope.** The property
and type namespaces are disjoint claim domains and a space may slug a
relation and a type onto one term (§3), so a single spelling→key map would
hold two answers for it. One step down, an option name may contain anything a
JSON string may, and so may the property spelling that owns it — under raw
naming a property really is named `C#`, and its spelling is exactly that. A
flat map keyed
`<name>#<property>` therefore had no representable entry at all for an
option of a property named `C#` — the escape hatch was unreachable exactly
where it was needed — and re-opening that after the freeze costs a version
(§10).
Nesting removes the separator, and with it the split rule, the key admission
rule, the two charsets, and the joined key's length bound.

**`option_ids`.**

```json
"properties": { "Priority": ["High"], "Severity": ["High"] },
"option_ids": {
  "priority": { "High": "bafyrei…opt1" },
  "severity": { "High": "bafyrei…opt2" }
}
```

- **Outer key**: a property **spelling as this document writes it** — the
  reader that resolves the entry is reading the document, not the store — so
  it carries the writable-key rule every property spelling carries (1–128
  characters, no control characters, §3), and the property it names inverts
  through `property_internal_keys` like any spelling elsewhere in the document. The
  reader does not invert the outer key itself: it indexes the legend by the
  spelling the slot in hand wrote, and matches or does not. Export writes the
  spelling the slot itself just used, so its outer keys are spellings the
  document holds by construction.
- **Inner key**: the option **name**, character for character as the value
  spells it, bounded only by being non-empty. It carries no charset rule,
  deliberately: it is the same string the value slot already holds, and a
  legend that cannot name a value its own document carries is the `C#` hole
  again, one level down.
- **Value**: the full option id.
- **Written unconditionally**, wherever export substitutes a name for an id —
  property values, dataview filter values, sort custom orders (§3). Behind no
  compaction flag, because this is identity rather than compaction; and
  behind no ambiguity test either, though one is computable: such a test sees
  only the divergence that exists when the document is written, and the
  rename it would guard against happens in the gap between writing and
  reading. Nothing is pruned because nothing unused is written — the entry is
  recorded at the substitution itself.
- **Read as a hint, not an address** — §3's three steps: the id, honoured
  only where the target space still serves it as a live option of that
  relation; then name resolution; then the value unchanged. A reader with no
  option resolver ignores the legend entirely, having no space in which to
  ask, which is what keeps a bundle carried elsewhere working exactly as it
  does without it.
- **`OmitIds` drops it** (§9): the export and backup shape keeps the legend,
  the prompt shape does not. §9 states what that gives up — the two losses above, back,
  on the read/prompt shape — and why export does not warn about it.
- **An outer key naming a property this document never spells is a warning**
  (§12) — a key-set comparison, not a parse. The entry can never be
  consulted, since a reader indexes by the spelling the slot in hand wrote. A
  warning rather than an error, because a legend may carry more than one
  document needs; but an entry that degrades to name resolution in silence is
  the kind of silence this format reports everywhere else.

**Object references are never compacted.** Every object id — mention and
object-link targets in `text`, `object_id` props, a callout's `icon.file`,
the envelope `icon.file` and `cover.file` (§2b), `objects`/`files` property
values, `items`,
view `default_template_id`/`default_type_id`, `object_orders[].object_ids`,
and filter `value`/sort `custom_order` entries of `objects`/`files`
properties — is written in full, on every shape, with no legend. The §9
`#name` suffix and the participant fold are not exceptions: the suffix adds
a caption to a full id and inverts by deletion (no table to carry, nothing
to keep in sync), and the folded identity IS the participant id's content,
rebuilt from the reader's own space rather than looked up anywhere.

This is a deletion. The format used to carry a `refs` map of short labels to
full ids behind a `CompactObjectRefs` flag, and two independent measurements
retired it. API v2 removed the same legend from its read shape after
measuring a net token **loss** per document, and because the indirection
trapped write-back: an agent editing an object-valued property through a
label has to keep the legend in step, and one that regenerates the document
without it silently re-points every reference it held. The freeze review
measured the loss from the other end — a 200-item collection grew **32.7%**
under compaction, because a label used once costs more than it saves. Two
measurements, one verdict.

**The compaction that survives is the legend-less one**, and that is the rule
this section has left. `CompactBlockLabels` relabels ids the document itself
defines, so a short label needs no table to invert: it is a placeholder
within its containing document, never an address outside it, and a write
endpoint resolves one against the live object by unique suffix. There is
nothing to carry, nothing to keep in sync, and nothing to read back. An
indirection table has all three obligations, and the object legend failed all
three at once — which is why the half sold as "lossless, because the legend
inverts it" is gone and the half documented as *lossy* stayed.

With `CompactBlockLabels` (or `CompactIds`, which now selects that one half),
block/row/column/view ids are relabeled to their last 5 characters. Only
machine-minted opaque ids relabel: `dataview` is a documented constant,
`title`/`header` are structural, and an imported document's human-readable
ids carry meaning that relabeling would destroy for no benefit. Labels are
constrained to the schema charsets (the block-id charset `[A-Za-z0-9_-]{1,64}`
of §4; row and column relabels additionally dash-free, since `-` is the
derived-cell-id separator of §6.1), and an id whose label would collide with
another id in the document — relabeled or not — or that yields no valid
label stays uncompacted (implementation decision — fixed-width suffixes with
a full-id fallback, chosen over shortest-unique lengthening for simplicity; 5
characters over CID/hex alphabets make collisions birthday-rare).

The collision rule counts BOTH id populations, and that is not an accident of
implementation: the labeller's own census sees only the doc-local ids it may
relabel, so the object ids — every one of them now spelled verbatim in the
document, in the folded spelling where the §9 participant fold applies —
enter it as an avoid-set (both spellings: the document spells the folded
form, and a suffix-trimming reader recovers the raw one). A short object id spelled in a mention
and a minted block whose suffix equals it would otherwise both answer to one
name in one document. Deleting object compaction made this guard matter more,
not less.

**The census counts the ids the document SPELLS, not every id the snapshot
holds** — the same principle the term census follows (§3). A block the
document does not spell — a transparent container (§7a), a structural block
(§7), a content-less leaf, anything unreachable —
is gone from the snapshot a round trip rebuilds, so reserving its suffix slot
makes the two reads disagree: the first keeps a paragraph's id full because
an invisible block shares its 5-char tail, the second compacts it, and
guarantee 3 (§11) fails on the API's default read shape. The protection given
up is illusory in any case: a container the editor re-creates gets a FRESH id
no census could have reserved against.

**One unspelled id is reserved all the same: a cell's.** A cell carries no id
in the flat form (§6.1), but unlike everything else in that list it is not
gone from the rebuilt snapshot — import re-derives `rowId-colId` from row and
column ids the document DOES spell, so the same cell ids come back and
reserving them is stable across generations. It is also necessary: a cell id
ends with its column's id in full, so its last five characters ARE the
column's label. Leave cells out and the column wins that bucket alone and
compacts to a label its own cells share as a suffix in the live object —
which breaks this section's own promise that a served label is neither equal
to nor an ambiguous suffix of another served id, and makes the wiring's
resolve-by-unique-suffix allowance below unsound. Measured before the fix:
899 documents in a 36,966-object account served such a label.

**The census costs a second block emit.** `emittedLocalIds` runs the emit
again on a throwaway exporter rather than re-deriving the drop rules, because
a second statement of "what export emits" would be a second thing to keep in
step with the first, and the census is correct only while the two agree
exactly. Measured on a 1,630-block document: 4.2 ms → 6.7 ms, +57%. It is
paid only where labels are minted — that is, on the API's default read shape,
and never on the export/backup shape or under `OmitIds`, which writes no id
for a plan to label.

The two shapes the API serves are the two this leaves: API v2 default reads
use block labels (the server resolves them by unique suffix) and keep object
refs full inline, while its export shape — the backup/round-trip shape, whose
bytes re-import to the same document up to what the editor regenerates (§7,
§7a) — keeps block ids full (API spec C4).

**A wiring may still shorten what the format does not.** Import wiring MAY
resolve an id it cannot find by unique suffix against the target space
(useful for hand-written documents naming known objects), and a write
endpoint MAY resolve a block-label reference the same way against the live
object. Both belong to the wiring, not to this package: they are lookups
against live state, not indirection a document carries.

`CompactBlockLabels` and `OmitIds` compose: together they yield the most
prompt-friendly form (no block ids at all). Both are alternative
serializations — the canonical round-trip form (§11) remains the default
full-id export.

## 10. Versioning and compatibility

`version` is a **single integer with no minor axis**. It is required, it is
the sole authority on format identity, and it is checked before anything else
in the document is interpreted.

- **A reader rejects any document whose `version` is greater than its own**,
  with a dedicated error naming both versions rather than a generic schema
  failure. There is no partial or best-effort read of a newer document and no
  forward compatibility: a change an older reader cannot handle is exactly
  what a version bump means.
- **A reader accepts any document whose `version` is less than or equal to its
  own**, migrating older documents forward before parsing — with ONE
  exception, stated here rather than left implied: **`version` 1 is refused**.
  It is the pre-freeze draft integer (below), carried by every export made
  while the grammar was still moving, so there is no single grammar to
  migrate it from; the reader says so at `/version` and names re-export as
  the repair. Version 2 is the first frozen grammar and the first this rule
  will ever migrate FROM. Because `version` is required and unambiguous, a
  later migration has complete information about the grammar a stored
  document used.
- **Every format change bumps the version.** There is no additive-within-a-
  version rule, because there is nothing additive to have: the schema closes
  every object (`additionalProperties: false`) and every enum is exhaustive,
  so a new block type, a new property, a new enum value, a new mark, or a
  renamed key is rejected whole-document by an older reader regardless of how
  it is introduced. Saying so plainly is cheaper than a reserved-field
  mechanism that buys nothing under the rule above.
- **Two regimes, and every field belongs to exactly one.** The bullet above
  is the CLOSED regime, and it is not the whole format. A **closed** slot —
  an enum this document states as a fixed set of names, or a JSON object's
  own membership — refuses what it does not recognize, whole-document, with
  no degradation. An **open** slot — a property or type spelling, an option
  id, a dictionary key, or a numeric detail the app itself stores and reads
  as opaque data — degrades instead of refusing, because the entity it names
  lives in a space or a bundled table this reader may not fully know: it
  passes the value through verbatim, never inventing and never silently
  coercing to a default, and warns exactly where the degradation would
  otherwise be invisible. A field is closed when every value it can legally
  hold is enumerable at freeze time and a wrong one cannot be repaired by
  resolving it against a live space or an older bundle; it is open
  otherwise. **A new field's author states which regime it joins, in the
  same sentence that adds it.**

  The three open-regime behaviours, and why they differ: a stored number
  outside a named-enum property's vocabulary passes through RAW and lossless
  (§3), because the app treats it as opaque data; an out-of-range proto enum
  on a struct-typed field is OMITTED, which reads back as that field's
  default, because the slot has a safe default and no raw form (§6.2); and a
  content discriminator — `kind`, a block `type`, a relation `format` —
  REFUSES the whole document at export rather than misrepresent content.
- The `$schema` URL carries the same integer
  (`https://schemas.anytype.io/anyblock/<version>/object.schema.json`) and is
  **decorative**: it is optional, no reader gates on it, and the schema at a
  version's URL is mutable in place — a correction that does not change the
  format is republished there rather than given a new number. The frozen
  grammar is `anyblock/2/`. Format identity lives in `version` and nowhere
  else.
- `index.json` shares the same version number and the same rules (§2c), and a
  bundle is versioned as one artifact: if the index or any document in it
  declares an unsupported version, the whole bundle is rejected rather than
  partially imported.
- **A pre-release grammar change left no version marker, and the freeze
  closes that hole.** Every revision this document records — the three
  legends replacing `refs`, most sharply — happened under `version` 1, so a
  draft written against any of them is indistinguishable from a draft written
  against the last. The integer therefore moved ONCE at the freeze: the
  frozen grammar is 2, and 1 is refused outright at the version gate (§15 #9).
  That is the whole of what the bump buys — not migration, which no single
  grammar could define, but a clean refusal in place of a silent misread.
  A superseded draft that somehow reaches the schema is still refused there
  too, by the members the current grammar does not admit, and the reader
  names the member (`/refs`) with the rule that replaced it and the repair,
  rather than reporting a closed-set violation at the document root (§12).

  The relation lift (§2d) is the same shape, and the same decision —
  **refuse, loudly, with the repair named**, never read-and-migrate. A
  legacy relation document spells `relation_format` inside `properties`
  and has no envelope `format`. It trips the missing-`format` refusal, which
  carries the whole repair: the message lists the vocabulary and, when a
  legacy spelling sits in `properties`, says outright that it is the
  legacy form and where the value moved. Measured over all 10,617 legacy
  relation documents in a 38,061-document corpus, every one trips exactly
  that refusal and exactly one — the `/properties/relation_format` refusal
  cannot also fire, because it lives in the semantic pass and a schema
  failure never reaches it. It appears on the second pass, once the envelope
  field exists and the old member is still there. The same message also
  names `format` in `properties` when that is what the author wrote, which
  is the commoner mistake and the one a missing-member verdict would
  otherwise never mention. Reading the old spelling with a warning was
  declined for the reason §2b records — this format is a draft with no
  external consumers, so the refusal strands nobody.

  The `relation`→`property` rename moves the first refusal a legacy document meets, without
  changing the decision: a legacy relation document spells
  `kind: "relation"`, which the kind enum now refuses by name before any
  member is read, and the vacated `relation_format` spelling resolves to
  nothing at all any more. (The alias spellings are retired in turn: the
  refusal-by-resolution now fires on the display name `"Format"` and on the
  verbatim stored key, the two spellings that still name the detail — §3.)

**Syntax inside `text` is versioned too, and the reader is exact about it.**
A `text` string carries no version marker of its own, so the only thing that
keeps a stored document readable across a bump is that the reader recognizes
*exactly* the syntax its version defines and treats everything else as
literal. This binds three namespaces:

| namespace | version 2 recognizes | anything else | status |
|---|---|---|---|
| inline tags (§8.1) | `u`, `font`, `mention` | literal text, never an error — reported as a warning, since canonical output would have escaped it | **reserved**: canonical output escapes every tag-shaped `<` (§8.2), so the whole `</?[A-Za-z]` space is free for later versions |
| Markdown delimiters (§8.1) | `**` `*` `~~` `` ` `` `[…](…)` | literal text | **closed**: the set is complete; a future mark is a tag, never new punctuation |
| `anytype://` destinations (§8.1) | `anytype://object?objectId=<id>`, one parameter | a plain Link, preserved verbatim | matched by exact form, so a second parameter is available to a later version |

Being exact is what makes a later migration possible: when a version adds a
tag, a delimiter, or a deep-link parameter, the migration escapes or rewrites
the prior occurrences that a stored document meant literally, and it can only
do that if version 2's rule was unambiguous. A reader that guessed — matching
a deep link by prefix, say, and taking whatever followed as the id — would
have already destroyed the information a migration needs.

The reservation is what keeps that migration from being needed at all for
canonical documents: because export escapes tag-shaped `<`, a version that
adds a tag can read version-2 documents as they are. Only hand-written
documents can carry an unescaped tag-shaped sequence, which is why import
warns about one instead of silently accepting it — the warning is the
author's notice that those bytes are only literal by virtue of the document's
`version`, and that canonical form spells them `\<`.

**The cost this accepts.** When version 3 ships, a client still on version 2
cannot open *any* document a version-3 client exported — refused, not
degraded. For an export and interchange format written by external tools and
agents that is the right trade: it buys a contract with exactly one rule, and
the alternative — readers that tolerate unknown constructs — obliges every
reader to carry a degradation behaviour for every construct that will ever be
added. It would be the wrong trade if AnyBlock JSON became a cross-device wire
format, so that is a deliberate constraint on where the format is used, and it
is recorded here rather than discovered later.

## 11. Round-trip guarantees

Let `N(S)` be state normalization (given export and import wired with
equivalent resolvers, §3): structural blocks dropped, to be regenerated by
the editor at first open (§7); **transparent containers dropped and their
children lifted to the container's own position, with every attribute the
container carried** (§7a) — the editor re-creates wrappers on the next
`ApplyState`, but conditionally and in a shape that is a function of arrival
order rather than of the document, so unlike `title`/`header` what comes back
is neither the same partition nor guaranteed to come back at all;
restrictions rebuilt (§4); properties
stripped per §3 (with the exemption list), the attribution pair
`creator`/`lastModifiedBy` among them — export spells them `<id>#<name>`
and import drops the key, so a round trip clears both (§3); informative
reference suffixes trimmed and participant composites folded/rebuilt (§9) —
exact inverses for every id either side WRITES, and the round trip is
byte-stable for them, but three residues remain because a snapshot's
reference slots are untrusted and may hold what the format cannot spell:
**an id containing `#` loses everything from the first one**, since the
reader cannot tell that `#` from the one a caption hangs on (this is the
only place the format silently narrows a value it was handed; export no
longer captions such an id, so the loss happens once and the value is a
fixpoint after — measured across two corpora, 81,696 documents, zero occur);
**a bare account identity already stored in an object or file slot comes
back as this space's participant id**, because unfold cannot know the fold
never fired (every bare identity in the corpus sits in a text-format
property, where the object arm never runs); and **a reader wired without a
SpaceId leaves folded identities bare**, which addresses no object — it is
told so through the warning sink, once for the document, since the fault is
the wiring and every such reference in the object shares it;
select/multi_select option ids
replaced by name resolution — in properties, filter values, and custom
orders (§3, §6.2) — which `option_ids` inverts exactly (§9a), leaving two
residues: **two same-named options of one property held by ONE object**
collapse onto the first, because the document spells one string twice; and a
reader wired with no option resolver ignores the legend and keeps the names,
having no space in which an id could be an option at all; the seven
system-stamped keys of §3 come back ABSENT when their stored value was empty
(§15 #12) — a whitelist, so every other key present-and-empty still survives;
deprecated snapshot
and block fields cleared (§2, §5); deprecated `Header4` re-styled to
`heading_3` (§5); `checked` outside checkboxes dropped and marks on literal
blocks dropped (§5); marks normalized — emoji materialized, whitespace
boundaries shrunk, same-type overlaps truncated, adjacent ranges merged
(§8.3); file/bookmark `state` recomputed (§5); empty strings/arrays/objects
and default scalars dropped from block attributes and envelope fields — but
never from property values, whose presence is meaningful (§3, §4); tables
normalized and ids canonicalized
(§6.1, including empty-paragraph cells collapsing to absent cells); dataview
`activeView`, cached sort/filter formats, deprecated per-column date/time
fields and `value` on `empty`/`not_empty`/`exists` leaves dropped, group
`index` derived from order (§6.2); scalar-stored select/objects/files
property values become single-element lists and the legacy file `hash`
migrates into `object_id` (§3, §5); object types reduced to the positions §2
models — one type, plus, on a template, the target type — with keyless
entries (`ot-`, `""`) dropped first, so the remaining entries close ranks
rather than lose the slot a keyless one would have silenced (§3);
**icon and cover reduced to the single winning variant** (§2b), which is
seven clauses of its own:
(a) the four icon channels collapse under `iconName` → `iconEmoji` →
`iconImage`, with `iconOption ≥ 1` attached as `color` to whichever won and
standing alone as the `color` variant when none did;
(b) a source whose stored value is EMPTY (`""`, `[]`, `0`, `null`) is not a
source, so a key present and empty comes back ABSENT — the one place this
format overrides §3's presence-is-meaningful rule, and it rests on all nine
relations being `hidden: true`, so no property row exists for presence to be
meaningful to (1,358 production objects carry only empty sources and end up
with no icon and no cover at all);
(c) `iconOption: 0` is the proto zero, not a colour, and is dropped;
(d) `iconImage` entries beyond the first are dropped with a warning (never
observed — the relation is `maxCount: 1`);
(e) a `file` value that is not id-shaped is dropped with a warning, because
there is no way to write it (33 production objects, every one an absolute
filesystem path a Notion import left in `coverId`);
(f) a `coverType` outside `0..5`, a `coverType` of 0, or a `coverType` with
an empty `coverId`, produces no cover, with a warning where anything was
lost;
(g) `coverScale`/`coverX`/`coverY` with no image cover to frame are dropped.
A callout's icon reduces the same way, `emoji` over `file`; and a type
object gains an empty list for every recommended role nothing occupies —
`property_definitions` (§2a) collapses the four role lists into one labelled
array, and import rebuilds all four from it, so a role the store left absent
comes back as `[]`. An absent list and an empty one say the same thing, and
the empty list is the only way this format can express a role being
*cleared*, since `property_definitions` cannot name a section that exists with no
members. Whether the object state itself should carry all four consistently
is a question about the state, not the format (GO-7451).

The §2d relation lift adds almost nothing here, by design — presence mirrors
presence, so `false`, `[]` and `null` all survive and the three keys are
otherwise untouched — but three residues are real and stated: **a relation
snapshot with no stored `relationFormat` comes back with an explicit 0**,
because `format` is required and absent-reads-as-longtext is what every
consumer of the detail already does (never observed: all 10,617 production
relation documents carry the key); **the §3 text collapse now reaches the
relation's own definition** — a non-bundled shorttext relation read without
a format resolver comes back longtext, exactly the residue §3 states for
every other format slot (53 of 10,617 under bare options in the corpus;
zero with the space's resolver, which knows every live relation's format);
and **`object_types` entries take the §3 list normalizations** — a
scalar-stored value wraps, empty-string entries drop — while the id↔key
translation is exact for every id the store actually speaks: ids out, ids
back under the `TypeResolver` capability, verbatim both ways without it.
One residue, measured at 27 corpus relations: **a legacy bare type KEY
stored where the store speaks object ids comes back as this space's type
object id** — export passes the key through verbatim (it is no id the
resolver serves), and import writes the id the key names, which is the
store's own spelling for the same type. A respelling, not a rebinding — the
comparator normalizes both sides to keys through the same capability, the
treatment the recommended lists already get, so only a change of the type
NAMED reports.

The deleted-icon rule (§9) adds one normalization of its own, armed only
when the wiring supplies the `ObjectDeletionResolver` capability (§13):
**an `iconImage` reference whose target is a tombstone in the space's own
store is dropped**, and the document falls through to the remaining icon
channel. The predicate is exported (`DroppedDeletedIconRef`) and the
comparator consults it on the icon/cover comparison, the same-commit
discipline every owned predicate here follows — without it the comparator
reads every dropped icon as data loss, the drift class that once produced
1,344 false failures in a single sweep.

The missing-reference rule (§9) adds one normalization, armed only
when the wiring supplies the `ObjectExistenceResolver` capability (§13) —
under bare options it adds nothing and every reference passes verbatim:
**a reference to an object the space's store holds no row for is rewritten
to `_missing_object` in a singular slot (block `object_id`s, mention
targets) and dropped from a list slot (objects/files values,
`object_types`), and a stored sentinel follows the same split — kept in a
singular slot, dropped from a list.** The movement converges in one
generation: the first export writes the sentinel or the shorter list,
import stores exactly that, and every later export is byte-identical — so
guarantee 3 below holds, with the rewritten id's warning as its last
appearance anywhere. The predicate is exported
(`DroppedMissingObjectRef`) and the comparator applies it to BOTH sides
of the objects/files and `relationFormatObjectTypes` comparisons, the
same-commit discipline every owned predicate above follows: a
dropped-by-design entry is not loss, a live entry that vanishes still
reports, and a comparator handed no capability excuses nothing.

The §2a `type_settings` group adds three normalizations, all scoped
to TYPE documents and all owned by exported predicates the comparator reads
(`DroppedTypeProvenanceKey`, `DroppedEmptyTypeSetting`), so the two sides
cannot drift the way that once produced 1,344 false failures in one sweep:
**the seven install-provenance keys come back ABSENT** — `layout`,
`resolvedLayout`, `smartblockTypes`, `sourceObject`, `origin`, `addedDate`,
`setOf`, each admitted to the drop individually against 1,760 corpus type
documents (the verdicts live on `typeProvenanceKeys`, §2a; `revision` was
admitted and then failed — it guards a type's own name against the bundled
reviser) —
while the same keys on any other kind survive untouched; **the five lifted
settings come back ABSENT when their stored value was empty** (`pluralName`
`""` on 145 corpus docs, `defaultTemplateId` `[]` on 87), the §4 omit-empty
canon where §2d mirrors presence, because these are settings with defined
defaults rather than a property's definition; and **a `defaultTemplateId`
with a second entry keeps only its first**, with a warning — the member is
the one default template, and 0 of 1,760 corpus documents carry more.

The §2f dictionary adds one normalization, and it is a COMPOSITION rule
rather than a document one — the per-document codec is untouched: **a
bundled-identical relation document is not written at all**. Its key travels
in the dictionary's `installed` list, and a reader reconstructs the object
from its own bundled table, across which trip (a) the install artifacts —
`createdDate`, `origin`, `addedDate`, `sourceObject`, `revision`,
`apiObjectKey`, `featuredRelations`, `scope`, `importType`,
`lastModifiedDate`, `layout`/`resolvedLayout`, the three recommended-list
stamps — come back ABSENT, re-stamped by the next install, and (b) a
definition member the copy never stored comes back as its explicit empty
default, because an install states the whole definition. Both movements are
owned by exported predicates the comparator reads
(`OmittedBundledRelation`, `RelationInstallArtifactKey`,
`InstallStampedDefault`) and both are scoped to snapshots the omission
predicate itself admits, so the ordinary document round trip keeps its full
sensitivity. The predicate is fail-closed on every axis — a divergent
definition member, an unclassified detail key, an alien-kinded value, a
block the format preserves (19 corpus relation documents carry a dataview
or free text) each keep the document — because omitting a document that
carried real data would delete it silently, which is disqualifying for a
backup format. Each admitted artifact key passed the §15 #12 test
individually against the 9,675 bundled-key relation documents; the verdicts
live on `relationInstallArtifactKeys`, and the keys that FAILED
(`isUninstalled`, `isFavorite`, `isArchived`, the bare `includeTime`) keep
their documents.

Export emits `blocks` in pre-order with exact depths, so export can never
produce a monotonicity violation and the flat shape does not disturb
byte-stability. Strict inputs add nothing to `N(S)`; for lenient
(`NormalizeIndent`) inputs, the clamped indents are part of the documented
normalization. **Marshal never emits a document its own validation
rejects**: a snapshot nested deeper than the indent bound (32) fails export
with an error naming the block, as does a table anywhere inside a table
cell (§6.1).

The snapshot's block graph is untrusted: export emits each block **once**
(the first parent listing it wins), which both terminates on cyclic
`ChildrenIds` and keeps duplicate/shared blocks from producing invalid
documents; duplicate table column/row ids are likewise dropped
(implementation decision).

**What "equivalent resolvers" requires.** Both guarantees below are stated
for export and import wired with equivalent resolvers, and for the key
vocabulary that means three things, none of which follows from the one
before it (`KeyVocabulary`, §13). One: whatever `…Slug` emits, `…Key` must
invert. Two: **no answer, in either direction, may bind a spelling that the
bundled table binds to a different key.** Three: **a live stored key
outranks the vocabulary's own NAME binding** — chain step 2 as an obligation
on the implementation, so a term that is some live entity's stored key
answers "not a spelling", and no spelling is emitted that a live stored key
answers to. Without the third, a document naming a property by its stored
key lands on whichever other property carries that string as its display
name.

The second is what the legend can only partly rest on. A document owes an
entry for every spelling a reader's chain would bind elsewhere, and export
asks the two chains it can see: the bundled table, which ships with every
reader, and the vocabulary it is running under (§3). A third reader's
vocabulary is not one of them — so a stored key both visible chains invert is
written with no entry, and a reader whose vocabulary disagrees with the
bundled table for that spelling silently resolves it elsewhere. A vocabulary
can satisfy the first rule completely and still turn a template for the
bundled `task` type into a template for an unrelated custom type. The
vocabulary this system ships (`storeresolver`) refuses such an answer in both
directions; the rule is stated because `Options.Keys` accepts an
implementation from anyone.

1. `Import(Export(S)) ≡ N(S)` — state-level equality on the snapshot after
   normalization.
2. `Export ∘ Import` is **idempotent and byte-stable**: for any valid
   document `J`, `Export(Import(J))` is the canonical form of `J`, and
   re-importing/re-exporting it is byte-identical. (Byte equality with the
   *original* `J` holds only when `J` is already canonical — import mints
   missing ids, merges marks, maps aliases like `heading_4`/`equation`,
   absorbs top-level title/description blocks, and export spells every key
   with the LABEL its authority gives it now, so a document written before
   the property was renamed — or before this rule — comes back naming the
   same stored key with a different term. That is a change of spelling and
   not of state: the label resolves through the document's own legend
   first, so `N(S)` is untouched and the object is the same object either
   way.)
3. `Export(S) = Export(Import(Export(S)))` — the same guarantee anchored on
   the SNAPSHOT rather than on a document, and the one an object exported
   twice depends on: once directly, once after a round trip through this
   format. It is what §9's "re-exports diff cleanly" means for everything
   that is not an id, and it is why the term census reserves only the keys
   the document spells (§3). Ids are the documented exception in the same
   direction as (2): a snapshot carrying a block or view with no id exports a
   document that is not canonical, and import mints one.

   **The attribution pair is the second documented exception, and the only
   one that is not an id.** A snapshot carrying a `creator` exports a document
   naming the member; import drops the value, so the next export has nothing
   to write and `Export(Import(Export(S)))` is one property shorter. Nothing
   there is recoverable and none of it was data: the value is derived from the
   object tree root's signature, and an imported object gets the importing
   account's own from its own new tree. What still holds — and is what a
   re-export diff actually depends on — is that the loss happens **once**:
   `Export(Import(Export(S))) = Export(Import(Export(Import(Export(S)))))`,
   so every export after the first is byte-identical to the next.

Both properties are enforced by tests in the package: golden-file tests for
representative documents plus property-based round-trip tests over generated
states (all block types, mark overlap/adjacency/whitespace-boundary cases,
emoji, tables, dataviews, UTF-16 payloads such as astral-plane characters).

## 12. Validation

**What earns a check.** A validation rule has to meet both of these, or it
does not belong here:

1. **It catches something silent.** The document validates, converts and
   imports, and is wrong somewhere the author will not look — a width read as
   pixels when written as a percent, a `group_by` the view cannot honour, a
   `less` on a date matching every record that has none, a target type that
   resolves to nothing. If the defect is visible the moment the object is
   opened, looking at the result catches it and a check only adds noise.
2. **It traces to a mechanism.** Every rule below points at the code that
   makes it true. A rule justified by taste rather than by behaviour cannot
   be argued with, and mixing the two is what turns warnings into something
   readers skip.

The cost of a marginal check is not the code, it is that every warning
becomes cheaper to ignore — including the ones that matter. Conventions that
fail neither test belong in authoring guidance and in review.


- Schema: JSON Schema **draft 2020-12**, hand-authored (the format
  deliberately diverges from proto shape), one file, blocks discriminated on
  `type`. The block definition is **non-recursive** — the flat encoding has
  no `children`, and table cells reference a dedicated `cellBlock`
  definition (same core, no table arm) so the block↔cell cycle is cut —
  which is what makes the block schema usable under strict/constrained
  decoding. The one remaining recursive definition
  is the dataview **filter tree** (`filterNode` groups nest, §6.2) — it is
  inherent to the filter model; a reduced core-profile schema (planned
  follow-up) without dataview is fully non-recursive, and the compact
  filter string (§6.2.1 — its parser ships as the `filterstring`
  subpackage for the API query surface; the *document* field stays
  reserved) removes it from the generation path. To keep validation errors usable for LLM
  producers, validation dispatches on the `type` const first (per-type
  `if/then` or programmatic pre-dispatch) instead of a flat 30-branch
  `oneOf` whose error output is noise. **The same rule governs every
  discriminated union in the schema**, and `icon`/`cover` (§2b) are where it
  was measured rather than assumed: `oneOf` reported 10 issues for one wrong
  member and never named the alternatives, `if`/`then` reports one and does.
  Output-only fields carry
  `x-output-only: true` (§4a). Annotated `x-app: Anytype` in line with
  `pkg/lib/schema`.
- Published at a stable URL and embedded in the package (`go:embed`);
  validated with `santhosh-tekuri/jsonschema/v6` (new dependency; the repo's
  existing `gojsonschema` is draft-07 only).
- Import = schema validation first, then semantic checks the schema can't
  express: **indent monotonicity** (§4 validity — errors name both
  indents), **leaf containment** and **row→column** (§4 containment, judged
  against the tree §7a's lift builds and naming the effective parent), id
  uniqueness over the whole document (§4), table shape and cell rules
  (§6.1, a cell block that is a transparent container included), envelope combinations (`items`/`template_for`/`kind`, §2),
  **property-key admission on the resolved stored key** (§3 — each
  `properties` spelling resolves through the §3 chain before the deny rule,
  the enum-name check and the format-shape warning run; validation
  mirrors the importer's details seam refusal for refusal — a **denied**
  resolved key, an **unwritable** resolved key, and **two spellings binding
  onto one stored key** are all errors — and a `property_internal_keys` *value* is
  admitted like the stored key it is, deny rule included; import re-runs
  the seam's checks on its own resolved key when a wider vocabulary is in
  force; the TYPE namespace mirrors the same way, minus one thing it used to
  need: the `/template_for` gate and the kind read `kind` alone, so neither
  runs the §3 chain and `Validate` no longer keeps a private copy of it, a
  `type_internal_keys` spelling or value gets the same writable-key restatement as
  `property_internal_keys`, and the import seam refuses a term a vocabulary resolves
  onto the empty type key, §3), **a typed field with no `format`** (§2b — the
  schema's `required` says a member is missing but not that it is a CHOICE,
  so the reader states the alternatives, reading them out of the published
  schema rather than restating them, and the schema's own verdict at that
  pointer is suppressed so the document still gets one fault, one issue),
  `language`-vs-`fields.lang` conflicts, an **`option_ids` key naming a
  property this document never spells** (§9a — a warning: the entry can never
  be consulted and the value degrades to name resolution; a key-set
  comparison against the document's property census, not a parse of the key),
  and
  **inline-markup parsing** (§8) — grammar errors report the block's JSON
  path and the offending snippet. The indent bound [0, 32] lives in the
  schema.
- **Validate and Unmarshal agree, in both directions.** Whatever Validate
  accepts, Unmarshal decodes; whatever Validate rejects, Unmarshal rejects
  too, with the same path-addressed issues. This is the promise that makes
  Validate worth calling — "this document imports" — and it constrains the
  reader in two places where JSON's value model is wider than Go's:
  - JSON Schema counts `2048.0` and `1e3` as integers, so every
    schema-integer field (`indent`, `size`, `limit`, `page_size`, column
    `width`) is read as a JSON number and converted, never decoded straight
    into an `int64`/`int32`; and each carries `minimum`/`maximum` for the
    range its stored type can hold, so the conversion cannot truncate.
  - a JSON number has no range, `float64` does, and every number in this
    format ends up in one (proto `Struct` values are doubles). A number
    outside `float64` — `1e400` — is therefore rejected wherever it appears,
    including on the loose surfaces (§3 property values, block `fields`,
    `store`, filter values) that have no schema bound by design.
  Both are enforced by a corpus invariant test over hand-written documents;
  a corpus generated from export cannot find these, because export never
  writes them.
- These path-addressed errors are the guardrail for agent-generated
  documents: generate → validate → feed errors back. With the flat schema
  the generate step can also run under strict/guided decoding end to end.
- **One fault, one issue.** Because the errors are fed back to a generator,
  an issue that is *confidently wrong* costs more than a missing one: an
  agent told `property "type" is not allowed` removes `type`, and its next
  attempt is further from valid. Two mechanics in the schema produce such
  issues, and the reader prunes both rather than passing the validator's
  bookkeeping through (implementation decision, `validate.go`):
  - `unevaluatedProperties: false` is what closes a block to the fields its
    `type` admits, but it can only see the properties that *successfully*
    evaluated subschemas annotated. One bad field makes the type's subschema
    fail, and then every property of that block is reported as not allowed.
    So a "not allowed" verdict is **dropped** when the object it belongs to
    failed for some other reason *and* the property name appears somewhere in
    the schema. A name the schema never mentions cannot be admitted under any
    reading, so that verdict stands — a hallucinated key is still reported
    alongside the real fault, in the same round.
  - an `anyOf` (table cells, §6.1) reports every branch it tried. Branches
    that failed only on the instance's type never applied, so they are
    dropped; if none applied, they merge into one issue naming every
    admissible shape.
  A reader that reports more than this is not wrong about the document being
  invalid, but its extra issues are not statements about the document.
- **Three warnings watch the raw-name seams**, all cheap, none a refusal
  (introduced with the raw-name re-spell). (i) A key spelling carrying edge
  whitespace or an invisible (default-ignorable) code point — 8 of 767
  measured production names do — draws a hygiene warning at Validate: the
  name is carried exactly as the space holds it, and an exact match must
  reproduce bytes the eye cannot check; a cleanup belongs where the entity
  is named, not at this seam. (ii) At import, a term that resolves verbatim
  and EXTENDS a live or bundled name past a word boundary
  (`Lists [in work] (text)`) is warned as a probable glued annotation — the
  one real raw-name failure shape the generation eval produced. (iii) At
  import under a space-backed vocabulary, a term that resolves verbatim and
  is no live entity's stored key is warned as the stale-or-guessed-name
  phantom — every name-addressed scheme's shared hole, named at the seam.
  Warnings (ii) and (iii) fire once per term per document, at every key
  slot, both namespaces: the diagnosis is a fact about the term, not about
  any one slot.
- **A malformed `option_ids` entry is an error; an unconsulted one is a
  warning.** The two look like degrees of one fault and are not. An outer key
  naming a property the document never spells is **well-formed content the
  document does not need**, and §9a permits a legend to carry more than one
  document needs — refusing it would refuse a legitimate document. A value
  that is empty or not a string is **not a legend entry at all**: the slot is
  typed as an option id and holds something that is not one, under no reading
  of the format. Three things keep it an error. The published schema types the
  slot (`{"type": "string", "minLength": 1}`), and the promise above is that
  an external validator running that schema *and nothing else* reaches the
  same verdict — downgrading the reader would make `Validate` accept what the
  schema we publish rejects, and the only way to close that divergence is to
  loosen the schema until an id slot admits `null` and `12`, at which point it
  no longer describes the format. `Marshal` never writes one (§11, I1), so the
  sole source is authoring — the case that can still be fixed. And the cost is
  bounded and already paid correctly: the fault is reported **once**, at the
  member's own pointer (`/option_ids/<property>/<name>`), not as a verdict
  about the document. The objection this answers — that a whole document,
  blocks and all, is refused over a field a reader may legitimately ignore —
  is equally true of every typed member of the envelope, and singling out
  `option_ids` would make it the one member whose type is advisory.
- **An issue names the member it is about.** The key slots are the one place
  where the schema cannot: `propertyNames` — the writable-key rule on
  `properties`, on `property_internal_keys` and `type_internal_keys` spellings (§3), and on
  `option_ids` outer keys (§9a) — is checked by validating each name as a
  *standalone string
  instance*, so the verdict carries neither the enclosing object's location
  nor, for a length bound, the name itself. A 200-character property key was
  reported as `maxLength: got 200, want 128` at the document **root**, which
  names no property at all. The rule stays in the published schema, because
  an external validator runs that and nothing else, and the reader
  **restates** it where the key is in hand: `/properties/<key>`,
  `/property_internal_keys/<spelling>`, `/type_internal_keys/<spelling>`,
  `/option_ids/<spelling>` and `/option_ids/<spelling>/<name>`, with the
  offending string in the message. A `property_internal_keys` *value* is covered the same way — the schema
  addresses it correctly but describes the bound rather than the string. The
  schema's own verdict is suppressed only for the members the restated check
  spoke for, so if the two statements of a rule ever diverge the document is
  still refused, with the schema's wording, rather than passed. Every issue
  path is a JSON **pointer**, so a segment taken from the document is escaped
  as one (RFC 6901: `~` → `~0`, `/` → `~1`); a stored key may hold either
  character. Both statements of a rule build it that way, which is what makes
  the suppression above possible at all — it is keyed by pointer, so one
  unescaped spelling is one fault reported twice.

  The **envelope** was the other place the schema could not name the member,
  for a different reason: it closes with `additionalProperties: false`, whose
  verdict names every unknown member of one object *inside its own text* and
  carries the **object's** location — `additional properties 'refs' not
  allowed`, at the document root. Inside a block the same fault is addressed
  correctly, because blocks close with `unevaluatedProperties`, which the
  library reports per member; so the promise held everywhere except the
  envelope, which is exactly where a document written against an older grammar
  fails. The reader splits that verdict into **one issue per member, at its
  own pointer**, in sorted order — the names are collected by ranging over the
  instance's map, so unsorted they come back in a different order run to run.
  Unlike an unevaluated-property verdict these are never pruned:
  `additionalProperties` consults only its own schema object's `properties`
  and `patternProperties`, which always evaluate, so its verdict never depends
  on a sibling subschema having succeeded, and the unreliability the pruning
  exists for cannot arise.
- **A removed key is told what replaced it.** `children` and `refs` are the
  two names a document written against a superseded grammar brings, and for
  both the bare "not allowed" points at the wrong repair: drop the subtree
  rather than flatten it into `indent` (§4), and delete the legend rather than
  expand what it inverted (§9a) — which strands every short label in the
  document as an id that addresses nothing. Each is answered instead with the
  rule that replaced it and the repair to make. This is the whole of the
  migration story for a document written before a rule changed, because
  `version` does not move for a pre-release grammar change (§10): the
  diagnostic is the only notice such a document gets.
- **A superseded MEANING gets the same treatment, and needs it more.** A
  removed key at least fails on its own — the schema has never heard of it.
  A member whose meaning changed still validates, and imports as something
  else. There is one: `{"type": "template"}` with no `kind` meant a template
  in an earlier revision, and means an ordinary page whose type is the Template type
  after it (§2). Both readings are well-formed, so nothing structural
  separates them, and the failure is silent in the worst way available — the
  object arrives, under the wrong kind, invisible to every template check
  downstream. So the shape is refused outright, by name, with the repair
  (`add "kind": "template"`), and export never emits it: a page whose type
  term is literally `template` keeps its `kind` explicit, or `Marshal` would
  be writing what `Validate` rejects (I1, §11). The refusal is a byte
  comparison on the raw spelling — no legend, no vocabulary — because it
  identifies a byte sequence a previous version of this format wrote rather
  than resolving a type. It is deletable at the version bump.

## 13. Package layout and API

```
pkg/lib/anyblockjson/
  SPEC.md                    — this document
  PRINCIPLES.md              — the design rules the format answers to,
                               with the priority order for conflicts
  PRINCIPLES_SHORT.md        — the same rules on one screen
  ANOMALIES.md               — real-world data anomalies found by prod
                               round-trip testing, and how the format
                               handles each
  schema/object.schema.json  — the published JSON Schema (embedded)
  schema/index.schema.json   — the bundle index's own schema (§2c, embedded)
  schema/authoring/          — the authoring subset (§2g): one self-contained
                               schema per surface, embedded like the full
                               three; strict subsets, enforced by test
  authoring.go               — the authoring subset surface (§2g):
                               ValidateAuthoring and its index/dictionary
                               siblings — the full validation first, then
                               the subset schema
  export.go                  — snapshot → JSON
  import.go                  — JSON → snapshot
  inline.go                  — marks ↔ inline markup codec (§8)
  table.go                   — table subtree ↔ columns/rows
  dataview.go                — dataview content mapping (§6.2)
  optionrefs.go              — the `option_ids` legend and the whole of
                               option resolution (§3, §9a): the export site
                               that records an entry, the one import function
                               that resolves a select value, and the property
                               census Validate's reachability warning is
                               taken against
  validate.go                — schema + semantic validation
  json.go                    — ordered canonical-JSON writer, enum tables,
                               proto value bridges, id helpers
  typeproperties.go          — property_definitions ↔ recommended lists (§2a);
                               GenerateSchema derived artifacts are planned
                               here (post-v1)
  keyvocab.go                — KeyVocabulary: the stored key ↔ spelling table,
                               both namespaces, and the bundled default (§3)
  label.go                   — the label rule (§3): what a document spells
                               for a key the bundled table does not speak
                               for — the display name, NFC and verbatim. A
                               vocabulary calls it; the codec never does
  bundledname.go             — the bundled name tables (§3): key ↔ display
                               name, both namespaces, the forgiving fold,
                               and the collision-ladder helper
  blockvocab.go              — the block-type name tables (§5)
  viewvocab.go               — the dataview enum name tables (§6.2)
  fragment.go                — the FRAGMENT surface: one block, a flat run,
                               one property value, the §8 inline codec (below)
  filters.go                 — the fragment surface for a §6.2 filter tree and
                               sorts array, standalone (query paths)
  index.go                   — the bundle index (§2c)
  storeresolver/             — the space-backed implementations of the four
                               resolvers, including KeyVocabulary; the only
                               place that reads a space's display names, and
                               the one that applies the §3 label rule to
                               them
  snapshotdiff/              — snapshot ↔ snapshot diffing for the PATCH path
  compose/                   — the bundle-level composition (§2c, §2f): the
                               path plan, the concurrent-emit composer that
                               accumulates the dictionary, manifest and
                               index lift, and the used-key census — shared
                               by the production exporter
                               (core/block/export/anyblock) and the cmd
                               tools, so the sweep exercises the shipping
                               composition rather than a private copy
  filterstring/              — compact filter string parser (§6.2.1):
                               string → §6.2 filter tree, offset-addressed
                               errors (planned with API v2 Phase 4; the
                               document-field integration stays post-v1)
  markdownblocks.go          — ParseMarkdownBlocks: block-level markdown →
                               a §4 flat run (id-less). Inline text passes
                               through verbatim as §8 markup source; only
                               the block slicing (headings, lists/indent,
                               fences, quotes, dividers, tables) lives
                               here. Never fails: unknown constructs
                               degrade to paragraphs, indents clamp per
                               the §4 lenient rule, and every output run
                               imports through UnmarshalBlocks (by test).
                               Built for API v2 Phase 5 (the insertBlocks
                               markdown payload and the create shortcut).
  roundtrip_test.go          — §11 property tests + state assertions
  golden_gen_test.go         — golden files (testdata/, -update to refresh)
```

```go
// FormatResolver reports the format of a property key, when known.
// Bundle properties are resolved internally; the resolver covers custom keys.
type FormatResolver func(key domain.RelationKey) (model.RelationFormat, bool)

// OptionResolver maps select/multi_select option ids to names on export and
// names to ids on import (creating options is the import wiring's job).
// OptionName carries a second duty on the import side: it is the liveness
// question `option_ids` is checked against — it answers for an id exactly
// when that id is an option of that relation here (§3, §9a).
type OptionResolver interface {
    OptionName(key domain.RelationKey, id string) (string, bool)
    OptionId(key domain.RelationKey, name string) (string, bool)
}

// PropertyDefinition describes a property object referenced by a type
// document (§2a). Options is the declared select vocabulary in display
// order; ObjectTypes restricts which types an objects/files property may
// point at, given as STORED type keys.
type PropertyDefinition struct {
    Key         domain.RelationKey
    Name        string
    Format      model.RelationFormat
    Options     []OptionDefinition
    ObjectTypes []string
}

// PropertyResolver maps property object ids to definitions on export and
// definitions back to ids on import; PropertyId receives the full definition
// so the wiring can create-and-return missing properties in one step (§2a).
type PropertyResolver interface {
    PropertyById(id string) (PropertyDefinition, bool)
    PropertyId(def PropertyDefinition) (string, bool)
}

// ParticipantResolver names the space member a participant id stands for,
// for the derived attribution properties creator/lastModifiedBy — spelled
// <identity>#<name> (§3, §9). EXPORT ONLY, and there is deliberately no
// inverse: a display name is a label, not an address — two members of one
// space may share one — and both properties are derived from the object
// tree's own signature, so an importer has nothing to do with the value
// even if it could resolve it. Answering false writes the id bare: the id
// is the resolvable half and is complete without its caption.
type ParticipantResolver interface {
    ParticipantName(id string) (string, bool)
}

// ObjectNameResolver names the object behind a reference, for the
// informative #name suffix (§9). EXPORT ONLY, behind Options.RefNames;
// import trims the suffix without asking anyone. Answering false writes the
// reference bare — never a partial or invented suffix. storeresolver
// implements it from the space index (one point lookup, cached).
type ObjectNameResolver interface {
    ObjectName(id string) (string, bool)
}

// ObjectExistenceResolver answers whether the space's store holds an object
// under an id — the missing-reference rule's question (§9). An optional
// capability of Options.ResolveObjectNames, discovered by type assertion
// (the TypeResolver pattern); storeresolver implements it off the same
// cached point lookup ObjectName pays for. It is a SEPARATE question from
// ObjectName deliberately: that seam's ok is name != "", which reads
// "exists but untitled" as "no" — using it as an existence check rewrites
// live references. known=false (a store failure) moves nothing; a
// tombstone row is a row, so a deleted object's references are untouched
// everywhere but the icon slot, whose optionality earns it the narrower
// ObjectDeletionResolver capability (§9).
// With no implementation wired, export rewrites and drops NOTHING.
type ObjectExistenceResolver interface {
    ObjectExists(id string) (exists, known bool)
}

// ObjectDeletionResolver is the narrower capability the icon slot uses: an
// icon reference to a TOMBSTONED object is dropped, where an ordinary
// missing reference is rewritten (§9, §11). Discovered by type assertion on
// ResolveObjectNames, like ObjectExistenceResolver. With no implementation
// wired, no icon reference is dropped for deletion.
type ObjectDeletionResolver interface {
    ObjectDeleted(id string) (deleted, known bool)
}

// Marshal serializes a snapshot into canonical AnyBlock JSON.
func Marshal(sbType model.SmartBlockType, snapshot *model.SmartBlockSnapshotBase, opts Options) ([]byte, error)

// Unmarshal validates data and reconstructs a snapshot.
// Errors wrap *ValidationError with JSON-path–addressed issues.
func Unmarshal(data []byte, opts Options) (model.SmartBlockType, *model.SmartBlockSnapshotBase, error)

// Validate checks data against the embedded schema and semantic rules
// without building a snapshot.
func Validate(data []byte) error

// ValidateAuthoring checks data against the FULL validation above and then
// the authoring subset schema (§2g), so a nil return means valid AnyBlock
// JSON, not merely subset-shaped. ValidateAuthoringIndex and
// ValidateAuthoringPropertyDictionary do the same for the other two
// surfaces; AuthoringSchemaURL and siblings name the published locations.
func ValidateAuthoring(data []byte) error

// DetectFormat reports the version and $schema markers without validating —
// the cheap dispatch probe for import wiring.
func DetectFormat(data []byte) (version int, schemaURL string, ok bool)

// FormatVersion (= 1) and SchemaURL (the published schema location) are
// exported constants for the wiring's dispatch.

// KeyVocabulary translates between the STORED keys a snapshot carries and
// the SPELLINGS a document writes, in both namespaces and both directions
// (§3). The default is BundledKeyVocabulary — the name table that ships
// with every reader, and nothing else. storeresolver supplies the
// space-backed one. Three preconditions on an implementation, none implied
// by the one before it: it inverts what it emits; it never binds a spelling
// the bundled table binds elsewhere; and a live stored key outranks its own
// name binding (§3, §11).
type KeyVocabulary interface {
    PropertySlug(key string) string
    PropertyKey(slug string) (key string, ok bool)
    TypeSlug(key string) string
    TypeKey(slug string) (key string, ok bool)
}

// ScopedKeyVocabulary is an OPTIONAL capability a KeyVocabulary may also
// carry, discovered by type assertion on Options.Keys. Display names are not
// unique, so a map-less reader meeting a shared spelling needs the space's
// candidate lists to resolve it within the declared type instead of guessing
// (§3); without this capability an ambiguous spelling is an error naming the
// legend as the repair. storeresolver implements it.
type ScopedKeyVocabulary interface {
    // Every live key whose exact document spelling is the term, as a sorted
    // set. Says nothing about stored keys: verbatim-first is the caller's
    // step, asked before this one.
    PropertyKeyCandidates(spelling string) []string
    TypeKeyCandidates(spelling string) []string
    // The stored property keys a type declares — the disambiguating scope
    // for a shared property name, counted once per key.
    TypePropertyKeys(typeKey string) []string
    // Diagnose one term for the verbatim-resolution warnings (§12).
    PropertyTermFacts(term string) KeyTermFacts
    TypeTermFacts(term string) KeyTermFacts
}

// Legend carries the three legends of the document a FRAGMENT was cut out
// of, so a fragment entry point runs the §3 chain from step 1 rather than
// from the reader's vocabulary. Marshal and Unmarshal ignore it: a whole
// document carries its own. The zero value is "no legend".
type Legend struct {
    PropertyKeys map[string]string            // spelling → stored relation key (§3)
    TypeKeys     map[string]string            // spelling → stored type key (§3)
    OptionIds    map[string]map[string]string // {spelling: {option name: id}} (§9a)
}

type Options struct {
    ResolveFormat     FormatResolver   // optional; nil = bundle-only resolution (§3)
    ResolveOptions    OptionResolver   // optional; nil = option values pass through as ids
    ResolveProperties PropertyResolver // optional; nil = type documents keep raw recommended-relation ids (§2a)
    ResolveParticipants ParticipantResolver // optional; export only. nil = attribution ids written bare (§3)
    ResolveObjectNames ObjectNameResolver // optional; export only. The #name suffix rides it behind RefNames;
                                       // nil = references written bare (§9). An implementation may also carry
                                       // ObjectExistenceResolver (type-asserted), which arms the
                                       // missing-reference rule (§9) — without it nothing is rewritten or dropped.
    SpaceId           string           // the space this run reads from / writes into; arms the
                                       // participant fold in BOTH directions — empty disables it (§9).
                                       // Supplied by the wiring exactly as resolvers are; the format
                                       // itself carries no space id.
    RefNames          bool             // export only: write the informative #name suffix on object
                                       // references (§9). Default off — the backup shape stays minimal
                                       // and rename-stable; read shapes opt in.
    Keys              KeyVocabulary    // optional; nil = BundledKeyVocabulary (§3)
    Legend            Legend           // fragment entry points only: the enclosing document's legends (§3)
    OmitIds            bool            // export only: drop every id, the option_ids legend included (§9, §9a)
    CompactBlockLabels bool            // export only: relabel doc-local block/row/column/view ids (§9a; lossy, legend-less)
    CompactIds         bool            // export only: alias for CompactBlockLabels — object refs are never compacted (§9a)
    GenerateId        func() string    // import only: id generator for missing ids;
                                      // nil = random 24-hex (editor-shaped). The wiring
                                      // passes the editor's generator.
    NormalizeIndent   bool             // import only: clamp over-deep indents instead of
                                      // rejecting (§4 lenient mode)
    OnWarning         func(Issue)      // optional sink for warning-grade issues
                                      // (NormalizeIndent clamps, path-addressed)
    // CompactFilters (reserved): filters as query strings — post-v1, §6.2.1
}
```

### 13.1 The fragment surface

The entry points above take a whole document. The **fragment surface** takes
a piece of one — a single block, a flat run, one property value, one filter
tree — for wiring that edits a live object op-by-op instead of round-tripping
it (API v2 PATCH). It is the same codec throughout: a fragment run is
validated by wrapping it in a synthetic document, so §4 monotonicity and the
§5 per-type shape rules apply unchanged.

```go
// MarshalBlockSubtree serializes one block subtree into a fragment envelope:
// {"property_internal_keys": {…}, "option_ids": {…}, "blocks": […]} — plus "type_internal_keys",
// which no block slot can owe today, so it never appears in practice
// — the flat §4 run beside the legends its blocks owe, in the envelope's own
// member order. OmitIds and the compaction flags are REFUSED here.
func MarshalBlockSubtree(subtree []*model.Block, opts Options) (json.RawMessage, error)

// UnmarshalBlocks converts a flat run into model blocks with the ChildrenIds
// graph wired; topIds names the run's top-level blocks, ready for a splice.
func UnmarshalBlocks(run []json.RawMessage, opts Options) (blocks []*model.Block, topIds []string, err error)

// UnmarshalBlock converts one block object into its model block(s); forcedId,
// when non-empty, keeps a replaced block's identity.
func UnmarshalBlock(raw json.RawMessage, forcedId string, opts Options) ([]*model.Block, error)

// MarshalPropertyValue converts one property value to its JSON form, plus
// this key's share of the option legend — {option name: option id} (§9a).
func MarshalPropertyValue(key string, v *types.Value, opts Options) (any, map[string]string)

// UnmarshalPropertyValue is its inverse. `key` is a STORED key, not a
// spelling; the option legend arrives through Options.Legend.OptionIds.
func UnmarshalPropertyValue(key string, v any, opts Options) *types.Value

// UnmarshalFilters and UnmarshalSorts convert a standalone §6.2 filter tree
// or sorts array — the query paths, which carry no document.
func UnmarshalFilters(raw json.RawMessage, opts Options) ([]*model.BlockContentDataviewFilter, error)
func UnmarshalSorts(raw json.RawMessage, opts Options) ([]*model.BlockContentDataviewSort, error)

// BuildRecommendedLists is the PATCH-type door into the §2a array
// applyTypeProperties reads out of a document: it resolves a typeProperties
// array into the four recommended-relation id lists, refusing exactly what
// the document path refuses.
func BuildRecommendedLists(props []TypeProperty, opts Options) ([]RecommendedList, error)

// ParseInlineText and RenderInlineText are the §8 inline codec, exported:
// the single-field pair Marshal uses for every text-bearing block.
func ParseInlineText(md string) (string, []*model.BlockContentTextMark, error)
func RenderInlineText(text string, marks []*model.BlockContentTextMark) string

// ParseMarkdownBlocks slices block-level markdown into a §4 flat run
// (markdownblocks.go). Never fails; unknown constructs degrade to paragraphs.
func ParseMarkdownBlocks(md string) []json.RawMessage
```

**A fragment has no envelope, so it has no legend of its own — and that is
the one thing every entry point here has to be handed.** The §3 chain's
first and highest step is the document's own statement about its spellings,
and a fragment cut out of a document that said
`property_internal_keys: {"priority": "6a32d485…"}` carries the spelling and not the
statement. Resolved through the reader's vocabulary alone, `priority` lands
on whichever relation THAT space gives the spelling to — the exact
misresolution §3 wrote the legend to prevent, at the one seam that writes to
a live object. So: `MarshalBlockSubtree` and `MarshalPropertyValue` **return**
the legends their output owes, and every reading entry point takes them back
through **`Options.Legend`**. A caller that assembled the fragment itself
leaves the field zero.

**`OmitIds` and the compaction flags are refused on a fragment, not
ignored.** This surface exists to address a live document, and both take the
addresses away: `OmitIds` drops every block id, the view id and the filter
id, so the run says what to write but not where; block-label compaction
rewrites doc-local ids to short suffixes that are local to the emitted run
and are not the object's ids at all. Either produced a fragment that reads
correctly and cannot be applied.

Other exported helpers, in service of the same wiring: `ValidateWarn`
(validation with a warning sink), `SchemaJSON` (the embedded schema bytes),
`InternalPropertyKeys` (what §3 strips), `IsCompactLabelShaped`,
`LeafBlockType` / `TextBlockType`, `FormatName` / `FormatByName`, the four
vocabulary listers, and the `index.json` namespace helpers (§1, §2c):
`IsPlatformId`, `IsReservedWidgetTarget`, `IsImportableWidgetTarget`,
`ReservedWidgetTargets`, `IsReservedHomepage`, the translators between a
reserved name and the importer's own bare spelling (`WireWidgetTarget`,
`FormatWidgetTarget`, `WireHomepage`, `FormatHomepage`), and the widget
object's omission seam (§2c): `OmittedWidgetObject`, `IndexFromWidgetObject`,
`WidgetObjectResidualKey`, and `WidgetsSnapshot` — the one builder both
`cmd/anyblockconvert` and the round-trip verifier use, so the archive a
bundle installs from and the reconstruction the sweep verifies are the same
bytes by construction.

The bundle index (§2c) has its own pair, since it is not an object snapshot,
and the property dictionary (§2f) another, on the same reasoning:

```go
func UnmarshalIndex(data []byte) (*Index, error)
func MarshalIndex(idx *Index) ([]byte, error)
func UnmarshalPropertyDictionary(data []byte) (*PropertyDictionary, error)
func MarshalPropertyDictionary(d *PropertyDictionary) ([]byte, error)
```

The dictionary's Go surface is `[]PropertyDefinition` — the same struct the
resolvers speak and both doors of the §2a array build — rather than a
dictionary-local entry type: §2e's one-shape rule holds on the Go side too,
and a fourth field list is how a fourth spelling starts.

The §2f composition predicates are exported beside them, for the wiring
that composes a bundle and the comparator that verifies one:
`OmittedBundledRelation` (may this relation document be omitted, and under
which `installed` key), `InstalledRelationDetails` (the reconstruction a
reader builds from that key), `RelationInstallArtifactKey` and
`InstallStampedDefault` (the two movements the omission trip makes, which
`snapshotdiff.Compare` reads rather than restates).

The DROP predicates of §9 and §11 are exported for the same reason — the
comparator has to read the rule export applied, or a deliberate drop reads
back as data loss: `DroppedMissingObjectRef` and `DroppedDeletedIconRef`
(the two reference drops, §9), `DroppedTypeProvenanceKey` and
`DroppedEmptyTypeSetting` (the type-document admissions, §2a), plus
`DroppedParticipantProvenanceKey`, `DroppedEmptyIconCover` and
`DroppedEmptySystemProperty`. Each answers for one normalization `N(S)`
names (§11).

The package is deliberately **pipeline-agnostic**: it depends only on
`pkg/lib/pb/model`, `core/domain`, `pkg/lib/bundle`, `util/text`, the proto
runtime (`gogo/protobuf/types`) and `santhosh-tekuri/jsonschema/v6` (§12).
It must not import anything from `core/block/import` or `core/block/export`
— including `anymark`; the inline codec is implemented in-package because
canonical, byte-stable rendering needs stricter guarantees than `anymark`'s
best-effort import parsing, while staying syntax-compatible with it (§8.1).

Wiring status — export landed, import is the follow-up:
- Export SHIPS: `core/block/export/anyblock` is the production exporter,
  wired straight into the export service's format switch
  (`core/block/export/export.go`) rather than through a
  `converter.Converter` shim — the earlier plan for a
  `core/converter/anyblockjson` shim was superseded by that wiring and no
  such package exists. It passes the storeresolver-backed format, option
  and key resolvers (§13) and shares the `compose` composition with the
  cmd tools, so the sweep exercises the shipping path.
- Import (follow-up, not this package): an entry that dispatches on the
  `version`+`$schema` markers so `RpcObjectImportRequest` accepts the
  format. This must be built on the ImportV2 engine (branch
  `go-7349-import-refactor`), not the legacy import pipeline, and must
  supply resolvers equivalent to the export side's (§3), including
  create-missing-option behavior.

## 14. Full example

```json
{
  "$schema": "https://schemas.anytype.io/anyblock/2/object.schema.json",
  "version": 2,
  "id": "bafyreieqh63jv…",
  "type": "Page",
  "icon": { "format": "emoji", "emoji": "🔥" },
  "cover": { "format": "gradient", "gradient": "pinkOrange" },
  "properties": {
    "Name": "Project Phoenix",
    "Status": ["In progress"]
  },
  "option_ids": {
    "Status": { "In progress": "bafyrei…opt1" }
  },
  "blocks": [
    { "id": "b1", "type": "heading_2", "text": "Goals" },
    { "id": "b2", "type": "paragraph",
      "text": "Ship the **new export** by Q3 with <mention object_id=\"bafyreidf…\">Roman</mention>" },
    { "id": "b3", "type": "bulleted_list_item", "text": "Flat JSON schema" },
    { "indent": 1, "id": "b4", "type": "bulleted_list_item", "text": "Validate in CI" },
    { "id": "b5", "type": "checkbox", "checked": true, "text": "Draft spec" },
    { "id": "b6", "type": "code", "language": "go",
      "text": "func main() {\n\tfmt.Println(\"hi\")\n}" },
    { "id": "b7", "type": "table",
      "columns": [ { "id": "c1" }, { "id": "c2", "width": 120 } ],
      "rows": [
        { "id": "r1", "is_header": true, "cells": [ "Format", "Size" ] },
        { "id": "r2", "cells": [ "pb.json", "3020 B" ] }
      ] },
    { "id": "b8", "type": "callout", "icon": { "format": "emoji", "emoji": "💡" },
      "text": "See the [ADF docs](https://developer.atlassian.com/cloud/jira/platform/apis/document/structure/) for the reference shape" },
    { "id": "b9", "type": "dataview",
      "object_id": "bafyrei…tasksSet",
      "properties": [
        { "property": "Name", "format": "text" },
        { "property": "Status", "format": "select" },
        { "property": "Due date", "format": "date" }
      ],
      "views": [
        { "id": "v1", "type": "kanban", "name": "By status",
          "group_by": "Status",
          "sorts": [
            { "property": "Due date", "direction": "asc", "empty_placement": "end" }
          ],
          "filters": [
            { "property": "Due date", "condition": "less", "date_preset": "current_week" },
            { "property": "Done", "condition": "equal", "value": false }
          ],
          "columns": [
            { "property": "Name" },
            { "property": "Due date", "width": 120, "align": "right" },
            { "property": "Status", "aggregation": "count_distinct" }
          ]
        }
      ]
    }
  ]
}
```

## 15. Decisions and deferrals

The draft kept its open questions here. At freeze the ledger is verdicts:
what was decided and where each rule now lives, what is deliberately
deferred past v1, and the one item still genuinely open. Item numbers are
stable — the rest of this document, the code, and `specclaims_test.go`
cite them as §15 #N — and the house style stands: a rejected design keeps
the decision, the overturned position, and the evidence that killed it,
the evidence pinned as assertions in `specclaims_test.go` so a rejected
design cannot come back after its counter-evidence has quietly stopped
being true.

### Decided

- **#1 Extension** — settled: `.anyblock.json`. A FAT bundle legitimately
  carries blobs that are themselves `.json` files (12 corpus file objects
  have `file_ext == "json"`), so "is this file a document" needs one cheap,
  collision-free test, and the double extension is that test — the entire
  skip rule for non-document files, at zero cost. `$schema`/`version`
  disambiguate the three grammars only once a file IS a document.

- **#2 `dataview` vs `database`** — kept `dataview`: ownership semantics
  differ from a database table. A judgment call, recorded as one.

- **#3 Option names vs `{id, name}` objects** — settled: names stay in the
  value, generatable and readable, and the id rides beside them in
  `option_ids`, under the property that owns the option (§9a). Three
  alternatives were each proposed more than once; each is falsified by
  evidence pinned in `specclaims_test.go`.

  - **A flat legend map with a separator** (`#`, deleted). No separator
    survives real names: `bundle.ApiSlug("C#") == "c#"` and
    `ApiSlug("#1 priority") == "#1_priority"`, so `#` appears inside both
    halves of the joined key. The nested shape needs no separator (§9a).
  - **A sigil in the value** (`"@opt-high"` marking a handle). A legal
    property slug can BEGIN with the sigil — `ApiSlug("@home") == "@home"`
    — and `Validate(data []byte) error` takes no resolver (§13), so it
    must accept the sigil everywhere (breaking I2) or refuse it where
    Marshal emits it (breaking I1). Export's deep links are not the
    counter-example they look like: `objectLinkDest` percent-encodes, so a
    leading `@` is written `%40`.
  - **`{name, id}` value pairs.** Not the format-only change it was
    believed to be: `model.RelationOption` is
    `{Id, Text, Color, RelationKey, OrderId}` — no key field — so the
    store cannot supply the stored keys the byte-cost argument rested on.
    It also puts a second value shape in the slot small models write most
    often.

  One argument that must not come back attached to any of these: the sigil
  designs were largely defended as protecting `object_ids` against a
  dropped legend. Object-reference compaction was deleted and `object_ids`
  never shipped — the only `object_ids` in this format is the dataview's
  `object_orders[].object_ids` (§6.2); object references print in full,
  everywhere, and need no legend. The §9 `#name` suffix is not that legend
  returning: a caption on a full id, inverted by deletion, split id-first —
  the `#` inside an option NAME that killed the flat legend provably
  cannot reach the id half.

- **#3a Attribution spelling** — settled twice; the second answer stands.
  The first spelled `creator`/`lastModifiedBy` as the member's display
  name alone; the standing rule is a resolvable id with the name as the
  informative `#name` suffix (§3, §9). Name-only broke API v2's need for a
  resolvable id, and a display name shared by two members (76 of 2,478 in
  production) identifies neither. `CREATOR_SPEC.md` (outside this repo) is
  SUPERSEDED on this point.

- **#4 Mention syntax** — `<mention object_id="…">` (§8.1), implemented:
  unambiguous and LLM-friendly. Client-side confirmation that the tag
  renders well remains welcome and is non-blocking.

- **#5 Emoji materialization** — lossy by design (§8.1): the mark
  disappears, its rendering is preserved. No surface has claimed to need
  the mark itself; that confirmation is likewise non-blocking.

- **#6 Icon block** — mooted by the icon lift: the block round-trips on
  the legacy profile objects that carry it (§5's table admits it, `name`
  only) and appears nowhere else, so there is no drop decision left to
  make.

- **#7 `type_properties` naming** — settled (§2a): the group is
  `type_settings`, the array `property_definitions`, and the `section`
  enum won over three booleans — mutual exclusion for free. Not to
  re-propose: `definition.properties` (extra nesting) and a `schema` field
  (collides with `$schema`, and the section is more than a schema).

- **#8 Property documents** — settled; §2d and §2f are that section.
  `kind: "property"` documents carry the definition group
  (`property_settings`), and the dictionary (§2f) is where a bundle
  declares a property without a document at all, options resolved by name
  with `internal_key` beside them.

- **#9 The `version` bump** — resolved: the integer moves to **2** at
  freeze. Everything written during the draft period is refused by the
  version gate, with its dedicated both-versions error (§10, §12), rather
  than by a member name. That makes the special-case refusal of
  `{"type": "template"}` with no `kind` dead code — it existed only
  because that one shape was well-formed under both readings — and it is
  deleted along with the last use of the `template` string constant
  outside export's emission rule.

- **#10 `kind` as sole template authority; the `_` namespace** — settled,
  with the cheaper alternatives declined (§2, §3; §1, §2c). Deriving the
  kind from the type term when `kind` is absent costs no migration but
  leaves the type term carrying structural meaning — two authorities, half
  the incoherence kept; making `kind` REQUIRED everywhere costs ~16 bytes
  on every page and contradicts §4's omit-every-default rule. Bare-word
  reserved listings plus a ban on the six words is a word list — every
  listing added later retroactively bans an id that was legal. The ban did
  NOT go away: the `_` prefix makes the FORMAT unambiguous, and the ban on
  the six wire spellings is still what makes the WIRE unambiguous, since
  the importer's own spellings are bare.

- **#11 The §3 chain's store step** — stays exactly as stated: step 3c is
  optional, store-backed readers only, single-candidate-or-nothing.
  Deletion and promotion were each proposed, and each is falsified by one
  call (pinned in `specclaims_test.go`). Deletion:
  `bundle.RelationKeysByApiFold("Severity") == []` — the bundled fold
  knows nothing about a space's custom keys, so a store-less step 3 would
  silently mint a second relation beside the one an agent just read.
  Promotion: `bundle.TypeKeysByApiFold("Task") == [task]` — a mandatory
  fold would overrule verbatim-first (§3 step 2) on a live stored key this
  format itself can create. The asymmetry is the point: a reader may
  resolve MORE than another, never DIFFERENTLY (§3).

- **#12 System-property trim** — settled: a whitelist of seven keys,
  spelled out in §3 and `systemtrim.go`. "The §15 #12 test" cited across
  this document and the code means the per-key admission discipline: a key
  is trimmed only where its empty value is both the proto zero and the
  semantic default, verified individually against the corpus. The inverse
  rule (`bundle.SystemRelations` minus an exception list) fails open —
  every future system relation joins the trim set sight-unseen — and buys
  almost nothing: the seven vetted keys carry ~50% of the 1.13% saving,
  the thirty-key tail 3.6% of 1.13%, or 0.04% of all bytes. `done` was
  never a candidate (not a system relation); the keys that FAILED
  admission are in §3. `internalFlags` (24% of the measured saving) is
  transient editor state and went to the `transientProperties` strip list
  outright, independent of this item.

- **#14, the spelling half** — taken (§2d), exactly as this entry
  prescribed when only one more pre-freeze change fit: the raw
  `relation_format: 100` became the envelope's required
  `format: "objects"`, `include_time` and `object_types` lifted beside it,
  and the flat spellings are refused with the repair named. The disease —
  one concept spelled two ways (a raw number on a standalone relation
  document, a name in a `type_properties` entry), and one word naming two
  concepts — is what "the §15 #14 disease" means wherever this document
  cites it (§2a, §2e, §3). The emptiness half is deferred, below.

- **#15 `picture` stays flat** — deliberate (§3). It has the same relation
  format as `iconImage` (`file`, `objectTypes: ["image"]`) and 1,946
  production objects carry one, so folding it into `icon` looks tempting —
  but it is a bookmark's preview image, not the object's identity, and
  folding it in would make one union mean two things. It reads correctly
  as an ordinary `files` property. Written down so it is not re-litigated.

- **#18 One statement of what a property KEY may be** — fixed. The
  writable-key rule is enforced at every key slot with matching schema
  bounds, `dataviewProperty` included, with export and the schema moved
  together so Marshal cannot emit what its own Validate rejects (§11 I1) —
  the coordinated change this entry said a lone `$ref` could not be. The
  drift it closes was demonstrated: a 200-character key accepted in a
  dataview block's `properties[]` and refused in a definition, in the same
  document — the §2e one-shape rule violated invisibly until measured.

### Deferred past v1

- **#14, the emptiness half** — deliberately not taken with the spelling:
  `include_time` is still present-and-false on 8,375 documents and
  `object_types` present-and-empty on 8,903, now on the envelope, because
  presence mirrors the store (§2d); collapsing it is a separate decision
  with its own snapshot-comparator cost. `file_variant_*` (7 parallel
  arrays on every file object, 8.35% of corpus bytes), `space_invite_*`
  and `widget_*` remain deferred with less at stake — machine-written,
  never authored.

- **#16 Reusing a key across spaces** — follow-up, and NOT a format
  change. Measured: 39 spellings in a 77-space account already bind to
  more than one stored key, `date` to three. The format already has the
  answer, and it is the legend: mint the key ONCE, ship it in
  `type_internal_keys`/`property_internal_keys`, and get the same key in
  every space, deterministically and offline — using a RANDOM key, since a
  readable one can collide with an unrelated property a space already has
  and merge the two in silence. The tempting alternative — look the
  type/property up in the user's OTHER spaces and reuse the key — is
  declined for the format (non-deterministic, order-dependent, cross-space
  reads on the creation path, a name-heuristic equivalence test that
  silently merges exactly what §3's chain and the exhaustive legend rule
  keep apart) and left as a possible import feature. If built, it should
  be a **suggestion, never a bind**.

- **#17 `order_id` → `sort_position`** — deferred to v2, recorded here so
  the deferral does not freeze in by omission. `order_id` survived the
  §2a admission test because it carries the user's own ordering, but what
  it carries is a lexid coupled to store internals — 946 documents carry
  one (603 `relation_option`, 343 `object_type`), every value exactly four
  characters, commonest `VVVV` — which an author cannot compute and a
  reader cannot sort on without the whole set. `sort_position: 2` is the
  right document spelling, the same move as `relation_format: 100` →
  `format: "number"` (§2d), but its own attack pass is unresolved — what
  import does when two entries claim one position, and whether export
  renumbers densely or preserves gaps — so it does not go in under freeze
  pressure. The store keeps its lexid either way.

- **#19 `layout` and `resolved_layout` follow the featured list into
  deprecation** — follow-up. The type owns an instance's layout: the UI no
  longer offers a per-object choice, so `layout` records a decision nobody
  can make any more, and `resolved_layout` is a cache of
  `type_settings.layout`. The corpus agrees from two directions: `layout`
  restates `resolved_layout` on 18,515 documents and has never once
  disagreed, and both are declared `number` in 76–77 of the 77
  dictionaries while every document writes enum-name strings — the largest
  single class of the format-does-not-predict-shape problem, 45,369 slots.
  Deferred because `resolved_layout` is load-bearing on the way IN — a
  reader with no type document to consult still needs to render — so
  retiring it means deciding what an importer does when the type is
  absent: a question about the bundle, not one document.
  `type_settings.layout` is untouched either way — the declaration, not
  the cache.

- **#20 A bundle that carries files BY REFERENCE** — follow-up,
  deliberately not in v1. Today's bundle is FAT: the bytes travel, the
  importing account uploads them under keys of its own, and §3 refuses to
  carry `fileVariantKeys` and its siblings because a shared bundle
  carrying the source's keys would hand its recipient the keys to every
  file in that space, for no benefit. The thin bundle — each file named by
  cid with the key that opens it, the importing account DOWNLOADS instead
  of uploading — is worth having and is not being built now. It needs its
  own bundle-level marker, so a reader knows an absent blob is intended
  rather than missing; that marker is what makes carrying a key defensible
  in that mode and only that mode. The keys are absent because today's
  bundle is the FAT kind, not because a key can never appear in this
  format.

- **#21 Option documents vs the dictionary** — follow-up, after the
  freeze. A bundle writes 2,641 `kind: "relation_option"` documents the
  dictionary nearly restates: since the dictionary learned `internal_key`,
  option identity is settled, and the api key need not travel at all —
  measured over a 77-space export, all 514 real option api keys are
  reproduced by the app's own mint-from-name rule (470 by the api slug, 44
  by the transliterate fallback), so not one survived a rename. The
  obstacle is the used-only rule: 175 of the 2,641 options belong to
  properties no document references, so dropping the documents today
  silently loses those 175. The real question — should a dictionary state
  a vocabulary nobody in this bundle uses — changes what the dictionary
  MEANS (the properties the bundle exercises vs the space's schema), and
  is not being decided under freeze pressure.

### Open

- **#13 The icon and cover assumptions the clients own** (§2b). Four, in
  descending order of what a wrong guess would cost — each closeable only
  with evidence from outside this repository:

  - **Icon precedence is unverified outside heart.** `iconName` >
    `iconEmoji` > `iconImage` comes from `core/api/service/icon.go`, the
    only precedence implementation in this repository — every other
    converter (`dot`, `graphjson`, `publish/relationswhitelist`) emits all
    four channels and lets the consumer decide. If the desktop client
    renders the emoji over the named icon, the export picks a different
    icon than the app shows for the 200 objects that hold both. **One grep
    in the client repo settles it** — anyone with a client checkout can
    answer — and the answer changes one line of the export rule.
  - **`coverType: 4` (prebuilt) has zero instances** in 36,966 objects,
    and the prebuilt id vocabulary exists nowhere in this repository. It
    is modelled as `{"format": "image", "file": …, "source": "prebuilt"}`
    because `state/details.go` and `cmd/usecasevalidator` both treat
    `{1,4,5}` as file-backed. What closes it: a client engineer confirming
    whether a prebuilt `coverId` is an object id or a client-side asset
    *name* — if the latter, the `image` branch is wrong for it and fixing
    it costs a version bump.
  - **The gradient and cover-colour vocabularies live only in the
    clients**, so `cover.color` and `cover.gradient` stay opaque names. A
    document can say `{"format": "gradient", "gradient": "sunset"}` and
    get a broken cover with no validation error — the one corner where the
    typed shape does not do what it exists to do. The format cannot close
    this alone; what closes it is the clients publishing the two lists, at
    which point the API's discovery layer can serve the enum.
  - **`icon.name` is an open string** (§2b) — where this design is weakest
    for an offline generator, and open in the sense that only ownership
    closes it: the ~397-name enum cannot be frozen into `pkg/lib/*`
    without violating I1 the first time the app ships a new icon, so
    closing it means someone owning lockstep maintenance with the client
    icon set forever. The API's own list currently contains a stray
    `t.txt` between `sync` and `tablet-landscape` — what that maintenance
    looks like when nobody owns it.
