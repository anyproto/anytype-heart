# AnyBlock JSON — format specification

Status: **draft v0.6** · Format version: **1** · Package: `pkg/lib/anyblockjson`

A human- and agent-readable JSON serialization of Anytype objects (the "anyblock"
model), designed for export, import, and generation by external tools and LLM
agents. It replaces the raw `jsonpb` dump (`.pb.json`) as the recommended JSON
interchange format.

Design lineage: the envelope and block tree follow the Atlassian Document
Format (nested `type`-discriminated tree, a single `version` integer with
additive evolution); inline formatting uses a Markdown subset inside text
strings; the vocabulary follows Notion's API and Anytype's public REST API
(`core/api`) wherever an established term exists — the format should be
readable, and mostly writable, by someone who has never seen Anytype
internals.

Changes from v0.3 (freeze review): select values are option names in filters
and custom orders too, not only in properties; canonical key order redefined
(spec-table order, `text` last — "proto field order" dropped); `refs` made an
authoritative opaque map with a full coverage table and agent-editing rules;
filter-tree semantics completed (implicit top-level AND, bare leaves
canonical); mark-boundary whitespace and same-type-overlap rules added;
global absent-vs-empty canon; table arity/cell-id rules and string-shorthand
cells; column `visible` flipped to `hidden`; `collections` renamed `store`;
Header4 export defined; OmitIds scope widened.

Changes from v0.5: **flat blocks** — `blocks` is a flat pre-order array with a
per-block `indent` integer; the nested `children` key is removed from the
format entirely (a breaking change made while the format is a draft with no
external consumers; the format version stays 1 and there is no legacy-input
mode). The block schema is thereby non-recursive, which makes it usable under
strict/constrained decoding (Anthropic structured outputs reject recursive
schemas; FSM-class guided decoders cannot express them) and keeps truncated
generations parseable — see `docs/AgentApiV2Research.md`, Addendum A, for the
decision record. Nesting rules are specified in §4: strict monotonicity
validation by default, a `NormalizeIndent` lenient import mode with
CommonMark-style clamping, and the containment rules formerly expressed in
the schema (leaf types, row→column) re-provided as path-addressed semantic
checks (§12). Table cells with descendants use an array-of-flat-blocks form,
and cells can no longer contain `table` blocks — the cell definition is the
schema's recursion cut (§6.1, §12).

Changes from v0.4: type documents specified (§2a) — `kind: "objectType"` with a
`typeProperties` array replacing the four recommended-relation id lists; a
type's dataview exports as an ordinary block when present; per-type
validation schemas become one-way derived artifacts (retiring the
`pkg/lib/schema` x-key approach).

## 1. Goals

1. **Readable** — a person can read and hand-edit a document; structure is
   visible as nesting, formatting as familiar Markdown; every name answers to
   something the reader knows from Notion, HTML, SQL, or common REST APIs.
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

### Terminology

The format uses six Anytype concepts; everything else is borrowed vocabulary:

- **object** — a page-like unit (page, task, note, …); one JSON document per
  object.
- **property** — a typed key-value on an object (Notion's term; stored
  internally as a *relation* — the internal name never appears in the
  format).
- **type** — the object's user-level type (`page`, `task`, `bookmark`…),
  identified by a key.
- **option** — a named choice of a `select`/`multiSelect` property.
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
  select/multiSelect options — §3; a name sidecar may be a future
  extension).
- An HTML-style sibling format (planned separately, isomorphic to this one —
  the Pandoc precedent).

## 2. Document envelope

```json
{
  "$schema": "https://schemas.anytype.io/anyblock/1.0/object.schema.json",
  "version": 1,
  "id": "bafyreieqh63jv…",
  "type": "page",
  "properties": { … },
  "blocks": [ … ]
}
```

Fields, in **canonical order** (§4):

| Field | Type | Req | Notes |
|---|---|---|---|
| `$schema` | string | no | Schema URL; written by export, ignored by import except for version detection (§10). |
| `version` | int | **yes** | Format version. This spec defines `1`. Evolution is additive within a version (ADF model); a breaking change bumps it. |
| `kind` | string | no | System-level object kind, lowerCamel (`page`, `profilePage`, `template`, `archive`, `widget`, …) — from `model.SmartBlockType`. **Omitted whenever derivable**: absent means `page`, and `type: "template"` implies `template`. An unrecognized value is a validation error listing the allowed values. |
| `id` | string | no | Object id. Written by export; import treats it as informational (a new id is minted on import) except for resolving intra-export links. Never compacted (§9a). |
| `type` | string | no | The object's type key without the `ot-` prefix (`page`, `task`, `bookmark`…). Maps to `objectTypes[0]` in the snapshot. Absent when the snapshot has no object types (legacy/system objects). Import preserves the key verbatim; the import wiring resolves it — matching an existing type or creating one (the Markdown importer's behavior). |
| `templateFor` | string | no | Only for templates: the target type key (`objectTypes[1]`). Present with `type != "template"` → validation error. |
| `key` | string | no | Identity key of *system* objects (types, properties); matches the public API's `key`. Never emitted for ordinary documents. |
| `properties` | object | no | The object's properties, §3. |
| `typeProperties` | array | no | Only for `kind: "objectType"` documents: the type's property definitions, §2a. Present on any other kind → validation error. |
| `refs` | object | no | Short-id legend for compact documents (§9a): maps labels to full object ids. Placed before `blocks` so the legend precedes use when read linearly. |
| `blocks` | array | no | The document's blocks as a **flat pre-order array**; nesting via `indent` (§4). |
| `items` | array | no | For collection objects: member object ids, in order (from the internal collection store key `objects`). Present on a non-collection document → validation error — enforced by the import *wiring* (collection-ness resolves against the space's types, not offline); the package's `Validate` checks structure only (implementation decision). |
| `store` | object | no | Escape hatch: remaining internal store content as a free-form JSON object, with the `objects` key lifted into `items`. Output-only (§4a). (Named `store` — its internal name — to avoid colliding with the collection concept.) |
| `root` | object | no | Escape hatch for non-default root-block attributes (`fields`, `backgroundColor`); absent in the common case. Output-only (§4a). |

The root block of the snapshot (whose id equals the object id) is
**implicit**: its subtree becomes the `blocks` array (its direct children are
the indent-0 blocks).

**The title, description, icon, and cover are properties (§3), not blocks.**
There is no title block in a document (§7).

Snapshot fields **excluded** from the format:

- `fileInfo` — only present on old-format (deprecated) file objects; export
  drops it, import leaves it empty.
- `relationLinks` — deprecated protocol-wide, scheduled for removal; not
  represented. Property formats are handled via resolvers (§3).
- `removedCollectionKeys` — dropped (meaningful only for change replay, not
  for fresh imports).
- `fileKeys`, `extraRelations` — deprecated in proto.

### 2a. Type documents (`kind: "objectType"`)

A type is not just a schema: it also owns the views over its objects
(columns, filters, sorts — presented through a dataview). Both live on one
object in the underlying model, so the format keeps them in **one
document** — a type never splits across files, and no JSON Schema file is
involved.

```json
{
  "version": 1,
  "kind": "objectType",
  "key": "task",
  "properties": { "name": "Task", "iconEmoji": "✅", "recommendedLayout": "todo" },
  "typeProperties": [
    { "key": "dueDate",  "name": "Due date", "format": "date",    "section": "featured" },
    { "key": "assignee", "name": "Assignee", "format": "objects", "section": "featured" },
    { "key": "status",   "name": "Status",   "format": "select" }
  ],
  "blocks": [ { "type": "dataview", … } ]
}
```

The type's own details (`name`, `iconEmoji`, `recommendedLayout`, …) stay in
`properties` under their stored keys (§3). The four recommended-relation id
lists (`recommendedFeaturedRelations`, `recommendedRelations`,
`recommendedFileRelations`, `recommendedHiddenRelations`) are **replaced**
by `typeProperties` — resolved entries, never raw relation ids.

`typeProperties` entry fields (canonical order):

| Field | Type | Req | Notes |
|---|---|---|---|
| `key` | string | **yes** | Property key (as stored). |
| `name` | string | no | Display name. Import uses it only when the property must be created; an existing property's name wins. |
| `format` | string | no | Property format (§3 names). Same import rule as `name`; a conflict with an existing property's format is an error at the wiring level (the package cannot see the space). |
| `section` | string | no | `featured` \| `hidden` \| `file` — which list the property belongs to. Absent = a regular (sidebar) property. |

Export emits entries in section order featured → regular → file → hidden,
preserving order within each list, and drops ids that no longer resolve to a
property (including the `_missing_object` sentinel of already-dangling
references); legacy lists that store bare property **keys** instead of ids
resolve through the reverse lookup, falling back to the bundle for system
properties. The canonical form writes `name` and `format` on every entry
(`format` defaults to `text` when absent on input), and writes the
`typeProperties` array **even when empty** — its presence is what tells
import to rebuild the lists. Import then rebuilds all four id lists — empty
sections become explicit empty lists, matching how type objects store them —
resolving each `key` against the space and creating missing properties (the
same policy as select option names, §3). A document without a
`typeProperties` field leaves the lists untouched.

Property ids are space-local, so the rewrite requires a property resolver
(`Options.ResolveProperties`, §13). Without one, export leaves the four
lists in `properties` as raw id lists, and import passes unresolved keys
through in place of ids for the wiring to reconcile — the same degradation
as option values without an option resolver (§3). A document carrying both
`typeProperties` and any of the four raw lists in `properties` is ambiguous
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

**Derived schemas.** `kind: "objectType"` documents are the canonical type
definition. Per-type validation artifacts — a JSON Schema constraining
objects of that type, a TypeScript-style declaration, a prompt-ready
property table — are **generated one-way** from the type document (planned
`GenerateSchema`, §13) and are never imported or treated as authoritative.
This retires the legacy per-type JSON Schema export (`pkg/lib/schema`) with
its `x-` extension keys.

## 3. Properties

`properties` is a JSON object keyed by **property key** (as stored,
camelCase — note the public REST API exposes snake_case aliases for some of
these; this format uses the stored keys so documents resolve offline).
Values are encoded by the property's format:

| Format | JSON encoding |
|---|---|
| `text` (default), `shortText`, `url`, `email`, `phone` | string |
| `number` | number |
| `checkbox` | boolean |
| `date` | RFC 3339 date-time string, UTC (`"2026-07-06T15:04:05Z"`); import converts back to unix seconds. Import also accepts date-only strings (UTC midnight), non-UTC offsets (converted to UTC), and fractional seconds (truncated to whole seconds). Export always writes the full UTC form. |
| `select`, `multiSelect` | array of option **names** (strings) — see below |
| `objects`, `files` | array of object ids (strings) |
| unresolvable format | value passes through verbatim in both directions |

Format names follow the public REST API (`select`, `multiSelect`, …);
internally they map to `model.RelationFormat` (`status`→`select`,
`tag`→`multiSelect`, `longtext`→`text`, `shorttext`→`shortText`,
`object`→`objects`, `file`→`files`; `emoji` and `properties` exist for
internal formats).

**Select options are names, not ids — everywhere.** This rule covers
property values here, filter `value`s, and sort `customOrder` entries
(§6.2). Export writes option names (`"status": ["In progress"]`); import
resolves names against the property's existing options and **creates
missing ones** (the behavior of the CSV and Notion importers, and of the
public API's tag endpoints). This is the one deliberate departure from
id-fidelity: two options of one property sharing a name collapse on
round-trip, and renaming an option breaks the link on reimport — accepted,
because opaque option ids are unwritable by agents and unreadable by humans
(§11 lists this normalization).

**Format resolution.** The format does not carry per-property formats;
`Marshal` and `Unmarshal` accept an optional resolver (§13). Property keys in
`bundle` resolve built-in; other keys resolve via the caller's resolver or
fall back to verbatim passthrough. Export and import must be wired with
equivalent resolvers for custom date/select properties to round-trip in
their pretty form; with no resolver the value still round-trips losslessly,
just unprettified.

**Well-known properties** (the magic keys every generator needs):

| Key | Format | Meaning |
|---|---|---|
| `name` | shortText | the object's title |
| `description` | text | subtitle/description line |
| `iconEmoji` | shortText | page icon as emoji |
| `iconImage` | files | page icon as image (object id) |
| `coverId` / `coverType` | shortText / number | page cover — output-only (§4a) |
| `done` | checkbox | completion state on task-like types |
| `dueDate` | date | due date on task-like types |

**Canonical key order in `properties`** (implementation decision): the
well-known keys `name`, `description`, `iconEmoji`, `iconImage` first (in
that order, when present), then all remaining keys alphabetically.

**Presence is meaningful.** A key's presence in `properties` records that the
property was set on the object — clients use it to show the property even
when its value is empty. The §4 omit-empty canon therefore does **not**
apply to property values: every key present in the snapshot is written, with
its value verbatim — including `false`, `0`, `""`, `[]`, and explicit
`null`. Import preserves them all (an explicit `null` stays a null value).
Omitting a key and writing an empty value are different statements: absent =
property not set; empty value = property set, currently empty.

**Value shape** (implementation decision): select/multiSelect and
objects/files values are always JSON arrays; import stores them as lists, so
internally scalar-stored values (e.g. `creator`) normalize to
single-element lists on round-trip (§11).

**Stripping.** Export removes internal/derived properties
(`bundle.LocalAndDerivedRelationKeys`) **except** those the importer
meaningfully preserves (mirroring `core/block/import/pb`): `createdDate`,
`lastModifiedDate`, `creator`, `isFavorite`, `isArchived`, `resolvedLayout`.
Those six are **output-only** (§4a): export writes them, generators should
not. `id` is lifted to the envelope and `type` to `type`. Everything else
round-trips.

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
  { "id": "b1", "type": "bulletedListItem", "text": "top level" },
  { "indent": 1, "id": "b2", "type": "bulletedListItem", "text": "nested" },
  { "indent": 2, "id": "b3", "type": "paragraph", "text": "deeper" }
]
```

| Field | Type | Req | Notes |
|---|---|---|---|
| `indent` | integer ≥ 0 | no | Nesting depth. Absent = `0` (top level); canonical form omits `indent: 0`. Values above **32** fail validation (adversarial-input bound; real documents reach ~6). See the nesting rules below. |
| `type` | string | **yes** | Discriminator; full inventory in §5. Unrecognized values fail schema validation (see §10 for forward compatibility). |
| `id` | string | no | `[A-Za-z0-9_-]{1,64}`. Uniqueness is enforced over the whole document, including derived table cell ids `<rowId>-<colId>` (§6.1) — a non-table block id that collides with a derived cell id is a validation error. Export writes ids by default — the `OmitIds` option drops them (§9); import generates missing ids with the editor's standard id generator. |
| `align` | `left · center · right · justify` | no | Omit when default (`left`). |
| `verticalAlign` | `top · middle · bottom` | no | Omit when default (`top`). |
| `backgroundColor` | string | no | Anytype color name. Omit when empty. |
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
  deeper than its predecessor`). A consequence worth stating: every prefix
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
  keys: `indent` first (structure before identity, and generation commits
  the cheap structural token first), then `id`, `type`, then the
  type-specific props **in the order listed for that type in §5** (`text`
  always last), then `align`, `verticalAlign`, `backgroundColor`, `fields`.
  Nested dataview/table objects: the order listed in §6. `refs` entries
  sorted by key.
- **Omit empty and default.** Canonical form never writes an empty string,
  empty array, or empty object (envelope included — no `"properties": {}`),
  nor a default scalar (`"indent": 0`, `"checked": false`, `"align":
  "left"`, `"hidden": false`…). Absent `text` means empty text. Import
  accepts explicit empties/defaults and canonicalizes them away.

### 4a. Output-only fields

Some fields exist purely so that export → import loses nothing. Export
writes them; **generators should omit them** — import accepts documents
without them and treats supplied values as authoritative only where
semantically safe. They are annotated `x-output-only: true` in the JSON
Schema so tooling can warn.

Output-only surfaces: `fields` (any block), `root`, `store`, `source`
(dataview), `groups`/`objectOrders` (views, §6.2), `id` on sorts/filters,
filter `nestedProperty` (reserved), `coverId`/`coverType`, and the six
preserved internal properties listed in §3.

## 5. Block type inventory

Text styles are promoted into `type`; every proto content type maps to one or
more JSON types. The "Proto origin" column is informative (for implementers),
not part of the format. Prop lists are in **canonical order** (§4). Complete
mapping:

| JSON `type` | Proto origin | Type-specific props (canonical order) |
|---|---|---|
| `paragraph` | Text/Paragraph | `color`, `text` |
| `heading1` … `heading3` | Text/Header1..3 | `color`, `text`. Input aliases `heading4`/`header4` map to `heading3`; stored deprecated Header4 blocks **export as** `heading3` (§11) |
| `quote` | Text/Quote | `color`, `text` |
| `code` | Text/Code | `language` (from `fields["lang"]`), `text` (**literal**, §8.4) |
| `title` | Text/Title | — structural, see §7 |
| `description` | Text/Description | — structural, see §7 |
| `checkbox` | Text/Checkbox | `checked`, `color`, `text` |
| `bulletedListItem` | Text/Marked | `color`, `text` (Notion/BlockNote naming) |
| `numberedListItem` | Text/Numbered | `color`, `text` (numbering is derived from position among consecutive siblings; never stored) |
| `toggle` | Text/Toggle | `color`, `text` |
| `callout` | Text/Callout | `iconEmoji`, `iconImage` (file object id), `color`, `text` |
| `toggleHeading1` … `toggleHeading3` | Text/ToggleHeader1..3 | `color`, `text` |
| `file` `image` `video` `audio` `pdf` | File (Type enum promoted; `Type_None` → `file` with no `objectId`) | `objectId` (target file object), `name`, `mimeType`, `size` (bytes), `style` (`auto · link · embed`), `addedAt` (RFC 3339). Legacy `hash` accepted on input. On export, a block with only the legacy `hash` set writes it as `objectId` (the hash migrates on round-trip, §11); when both are set, `objectId` wins and the hash is dropped. `state` is not serialized: import sets `Done` when `objectId`/`hash` is present, `Empty` otherwise. File blocks are leaves in the editor, but legacy data can nest real blocks under them — indented descendants are allowed and round-trip verbatim |
| `bookmark` | Bookmark | `url`, `objectId` (target bookmark object). `state` handled like file blocks. Deprecated preview fields and `type` (derivable) are dropped — preview data lives on the target object |
| `link` | Link | `objectId` (target object), `cardStyle` (`text · card · inline`), `iconSize` (`none · small · medium`), `description` (`none · manual · content`), `properties` (string array: property keys shown on the card). Deprecated `style` and legacy `fields` are dropped |
| `divider` | Div | `style` (`line · dots`, default `line`) |
| `row` / `column` | Layout/Row, Layout/Column | — (descendants carry content; a `row` contains only `column`s — §4 containment) |
| `group` | Layout/Div (legacy) | semantics-free container |
| `table` | Table (+ structural children) | `columns`, `rows` — see §6.1 |
| `embed` | Latex | `processor`, `text` (**literal**, §8.4) — see §5.2 |
| `tableOfContents` | TableOfContents | — |
| `property` | Relation | `key` (property key; renders the property inline) |
| `dataview` | Dataview | fully specified in §6.2 |
| `widget` | Widget | `layout` (`link · tree · list · compactList · view`), `limit`, `viewId`, `autoAdded` |
| `chat` | Chat | — (rare) |
| `featuredProperties` | FeaturedRelations | — structural, see §7 |
| `icon` | Icon | `name` (legacy profile objects only) |

Enum values serialize as lowerCamel strings; defaults are omitted.

**Leaf types.** `embed` (and its `equation` alias), `bookmark`, `link`,
`divider`, `table`, `property`, `dataview`, `icon`, `tableOfContents`,
`featuredProperties`, and `chat` cannot be parents: a block indented under
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

`processor` selects the embed kind — full enum, lowerCamel: `latex`
(default), `mermaid`, `chart`, `youtube`, `vimeo`, `soundcloud`,
`googleMaps`, `miro`, `figma`, `twitter`, `openStreetMap`, `reddit`,
`facebook`, `instagram`, `telegram`, `githubGist`, `codepen`, `bilibili`,
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
    { "id": "row1", "isHeader": true, "cells": [ "Name", "Status" ] },
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
or a *collection* (curated list, `isCollection: true`) — that they reference
but do not own (closer to Obsidian's Dataview than to a Notion database).
Field-for-field from `Content.Dataview`, with cleaned names, lowerCamel
string enums, and defaults omitted:

```json
{
  "type": "dataview",
  "objectId": "bafyrei…targetSet",
  "properties": [
    { "key": "name", "format": "shortText" },
    { "key": "status", "format": "select" },
    { "key": "dueDate", "format": "date" }
  ],
  "views": [
    {
      "id": "v1",
      "type": "kanban",
      "name": "By status",
      "groupBy": "status",
      "sorts": [
        { "property": "dueDate", "direction": "asc", "emptyPlacement": "end" }
      ],
      "filters": [
        { "property": "dueDate", "condition": "less", "datePreset": "currentWeek" },
        { "property": "done", "condition": "equal", "value": false }
      ],
      "columns": [
        { "property": "name" },
        { "property": "dueDate", "width": 120, "align": "right" },
        { "property": "status", "aggregation": "countDistinct" }
      ]
    }
  ]
}
```

**Dataview props** (`Content.Dataview`), canonical order as listed:

| Prop | Proto field | Notes |
|---|---|---|
| `objectId` | `TargetObjectId` | the set/collection object this view queries; empty for original set/collection objects and detached inline sets |
| `isCollection` | `isCollection` | |
| `source` | `source` | legacy, detached inline sets only; output-only (§4a) |
| `properties` | `relationLinks` | array of `{ "key", "format" }` — the properties available to this view, with formats per §3's vocabulary. **This field is live** (maintained by the dataview editor), unlike the deprecated snapshot-level relationLinks |
| `views` | `views` | see below |

Dropped (normalization): `activeView` (local UI state; the proto itself
excludes it from changes) and the deprecated proto `relations` field.

**View props** (`Dataview.View`), canonical order: `id`, `type`
(`table · list · gallery · kanban · calendar · graph`, omit `table` — note
the public API currently says `grid`; `table` is the more familiar term),
`name`, `groupBy` (property key; from `groupRelationKey`), `coverProperty`
(from `coverRelationKey`), `endProperty` (from `endRelationKey`; end date
for calendar/timeline), `hideIcon`, `cardSize` (`small · medium · large`,
omit `small`), `coverFit`, `coloredGroups` (from `groupBackgroundColors`),
`pageSize` (from `pageLimit`), `defaultTemplateId`, `defaultTypeId` (from
`defaultObjectTypeId`), `wrapContent`, `listSize` (`compact · regular`,
omit `compact`), `alternateRows`, then `sorts`, `filters`, `columns`,
`groups`, `objectOrders`.

Editor state nested per view, both output-only (§4a):

- `groups`: `[{ "id", "hidden", "backgroundColor" }]` — kanban group
  display order (array order; the proto's per-group `index` is derived).
  From `Dataview.groupOrders`, matched by view id.
- `objectOrders`: `[{ "groupId", "objectIds": […] }]` — manual object order
  within groups. From `Dataview.objectOrders`.

**Column** (`View.Relation`), canonical order: `property` (the property
key), `hidden` (inverse of proto `isVisible`; omitted = visible, so the
common case costs nothing), `width` (displayed column width in **pixels**,
see below), `aggregation`
(`count · countValue · countDistinct · countEmpty · countNotEmpty ·
percentEmpty · percentNotEmpty · sum · average · median · min · max · range`
— from proto `formula`; omit `none`), `align`. Deprecated per-column
date/time fields are dropped.

**Column `width` is in pixels**, the same unit as the proto's `width` — not
a percentage, and not a share of the table. A row of columns summing to
`100` produces four unreadable slivers, not four proportional columns.
Serialization passes the number through unchanged: the client owns
rendering, and this package neither clamps nor defaults it. What the client
does with it (`anytype-ts`, `J.Size.dataview.cell` / `Relation.width`):

| value | rendered width |
|---|---|
| omitted / `0`, a property stored as `shorttext` (`name`, …) | `500` |
| omitted / `0`, any other format | `192` |
| any non-zero `n` | exactly `n` — **no clamping on render** |

So **omitting `width` is the better default than guessing one**: the client
picks per format, and the choice tracks the client rather than freezing here.
Write a number only to pin a deliberate layout — the editor's own drag-resize
stays within `54…1000` (`min`/`max`), and columns at or below `70` (`small`)
and `120` (`medium`) get progressively stripped-down cell rendering, so
anything under ~`54` is a slice of a column with no room for its content.
Widths written by the editor itself land in the low hundreds (`150`–`320` for
text and object columns, `60`–`100` for numbers and short values).

**Sort** (`Dataview.Sort`), canonical order: `property` (from
`RelationKey`), `direction` (`asc · desc · custom`, omit `asc`),
`customOrder` (for `custom`; select values by option **name** per §3, other
values verbatim), `emptyPlacement` (`start · end`, omit unspecified),
`includeTime` (include time-of-day when comparing dates), `noCollate`
(disable locale-aware collation; compare raw strings), `id` (output-only).

**Filter** (`Dataview.Filter`) — a filter node is either a **group** or a
**leaf** (schema `oneOf`); the top-level `filters` array combines its nodes
with an implicit **AND** (canonical form uses bare leaves at the top level;
a group exists only for `or` or nesting):

- group: `{ "operator": "and" | "or", "filters": [nodes…] }`. Export maps a
  proto node with non-empty `nestedFilters` to a group and drops its leaf
  fields; import writes `operator` only on groups (leaves get the proto
  default).
- leaf, canonical order: `property`, `condition`, `value`, `datePreset`,
  `includeTime`, `nestedProperty` (reserved, output-only), `id`
  (output-only). `condition` values: `equal · notEqual · greater · less ·
  greaterOrEqual · lessOrEqual · contains · notContains · in · notIn ·
  empty · notEmpty · allIn · notAllIn · exactIn · notExactIn · exists`
  (`contains`/`notContains` from proto `Like`/`NotLike` — the public API
  agrees). `datePreset` from proto `quickOption` (`yesterday · today ·
  tomorrow · lastWeek · currentWeek · nextWeek · lastMonth · currentMonth ·
  nextMonth · numberOfDaysAgo · numberOfDaysNow · lastYear · currentYear ·
  nextYear`, omit `exactDate`). `value`: for select/multiSelect properties,
  option **names** per §3; dates stay unix numbers in the structured form;
  everything else verbatim. `value` is **dropped** on
  `empty`/`notEmpty`/`exists` leaves (§11).

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

#### 6.2.1 Compact filter syntax — reserved extension (post-v1)

**Status: designed but not part of v1.** v1 ships the structured `filters`
array only. The view field name `filter` (singular, string) is **reserved**
for this extension: v1 schemas do not define it, so introducing it later is
an additive 1.x release (§10 — a v1.0 reader encountering it reports
"produced by a newer version"). The two forms will coexist permanently —
`filter` and `filters` mutually exclusive per view, import accepting both,
export choosing via option.

The agreed design, recorded so the extension stays buildable as specified:
a view carries its filter as a single SQL/JQL-flavored query string:

```json
{ "type": "kanban", "groupBy": "status",
  "filter": "done = false AND (dueDate < currentWeek() OR dueDate IS EMPTY)" }
```

Grammar (informal): `OR` over `AND` over parenthesized groups over leaves;
`AND` binds tighter, parentheses group. There is deliberately **no
free-standing `NOT (…)`** — the internal model has no NOT-group; negation
exists only in negated conditions, keeping string ⇄ structured 1:1.

| Condition | Syntax |
|---|---|
| equal / notEqual | `priority = 3` / `priority != 3` |
| greater / less / greaterOrEqual / lessOrEqual | `> < >= <=` |
| contains / notContains | `name CONTAINS "report"` / `NOT CONTAINS` |
| in / notIn | `status IN ("In progress", "Blocked")` / `NOT IN (…)` |
| allIn / notAllIn | `tags HAS ALL ("urgent", "q3")` / `NOT HAS ALL (…)` |
| exactIn / notExactIn | `tags = ("a", "b")` / `!= (…)` — set literal on the right |
| empty / notEmpty | `assignee IS EMPTY` / `IS NOT EMPTY` |
| exists | `assignee EXISTS` |

Values: double-quoted strings, bare numbers, `true`/`false`, RFC 3339 dates
in quotes (`dueDate < "2026-08-01"`), and date-preset **functions** —
`yesterday() · today() · tomorrow() · lastWeek() · currentWeek() ·
nextWeek() · lastMonth() · currentMonth() · nextMonth() · lastYear() ·
currentYear() · nextYear() · daysAgo(n) · daysFromNow(n)` (the parameterized
pair maps to `numberOfDaysAgo`/`numberOfDaysNow` with the value as `n`;
parens distinguish presets from string literals). Property keys are bare.
Select/multiSelect values are option **names**, per §3 (the structured form
agrees since v0.4; only date values differ — RFC 3339 here, unix numbers
there — with a deterministic mapping both ways).

Canonical rendering: uppercase keywords, `", "` separators, double quotes
with backslash escapes, parentheses only where precedence requires. Export
will keep writing the structured array by default; a future `CompactFilters`
option will emit the string form for any view whose filter is fully
expressible (every leaf free of output-only fields like `nestedProperty`,
every option name resolvable), falling back to the structured array per
view otherwise. Import will accept both forms; string-parse errors report
the view's JSON path, the offending token, and its position.

## 7. Structural blocks

The following blocks are **derivable** and are dropped on export:

- the root block (implicit, §2),
- the header wrapper and its children `title`, `description`,
  `featuredProperties` — their content duplicates `properties.name` /
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
`featuredProperties` block (which carries no content) is simply dropped.

**Content-less blocks** (legacy data): old accounts hold blocks whose
content oneof is unset — relation objects wrap their "used in" dataview in
one, and pages can contain orphaned empty leaves. Export drops a childless
content-less block and serializes one with children as a transparent
`group`, so the subtree survives (part of `N(S)`, §11).

## 8. Rich text: inline markup

Text-bearing blocks carry a single `text` string. Formatting is expressed
inline — **offsets never appear in the format.**

```json
{
  "type": "paragraph",
  "text": "Ship the **new export** by Q3 with <mention objectId=\"bafyreidf…\">Roman</mention>"
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
| `[text](anytype://object?objectId=<id>)` | Object | inline link to an Anytype object — Anytype's standard deep-link shape |
| `<mention objectId="<id>">text</mention>` | Mention | decorated object reference (icon + name in UI) |
| `<u>text</u>` | Underscored | standard HTML |
| `<font color="red">text</font>` | TextColor | Anytype color names |
| `<font background="yellow">text</font>` | BackgroundColor | coincident color+background ranges combine into one tag: `<font color="red" background="yellow">` |
| — | Emoji | not writable: export **materializes** the mark by splicing its emoji over the covered text (the mark's semantics are replacement; this matches the Markdown export and the chat renderer). On import emoji are plain text |

Inline tags: import accepts any attribute order, single or double quotes,
and surrounding whitespace; canonical form is double quotes, single spaces,
`color` before `background`. Zero-length tags (e.g.
`<mention objectId="x"></mention>`) are dropped on input.

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
- `<` — escaped only before a whitelisted tag prefix (`</?` + `u`/`font`/
  `mention` + delimiter).
- `&` — escaped only where a valid entity follows. Recognized entities:
  `lt gt amp quot apos nbsp` and numeric (`&#65;`, `&#x41;`).
- `\` — escaped when followed by ASCII punctuation (input accepts a
  backslash before any ASCII punctuation as an escape, per CommonMark).

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

Implementation decisions (v0.4 freeze):

- **Step 1 details**: "invalid" ranges are out-of-bounds, inverted,
  zero-length, or splitting a UTF-16 surrogate pair; a param-carrying mark
  (link, mention, object, colors, emoji) with an empty param is dropped;
  a param on a param-less mark type is cleared (so equal ranges merge);
  params beyond the §8.2 resource bounds are dropped. A **Link mark whose
  param is an `anytype://object?objectId=` deep-link normalizes to an
  Object mark** — the two render identically, and without the
  normalization the parse-back type flip would change same-type overlap
  resolution.
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
rule-of-three resolution is not invertible, and §11.2's byte-stability over
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
- All `objectId` props and mark targets are object ids, opaque to this
  format; there are no intra-document block references (table cell ids are
  derived, §6.1).
- Import id policy: missing → generated (the editor's standard generator);
  provided → validated for uniqueness (§4) and charset, preserved so that
  re-exports diff cleanly.
- On output, export writes ids by default (stable diffs, §11 canon). The
  `OmitIds` marshal option (§13) instead drops **every id in the document**
  — blocks, table rows/columns, views, sort/filter ids — along with the
  id-dependent output-only view state (`groups`, `objectOrders`). For
  templates, prompt examples, and any content meant to be re-inserted rather
  than diffed. An id-less export is valid but not the canonical round-trip
  form (re-importing mints fresh ids).

### 9a. Compact ids

Full object ids are ~59-character CIDs; a single mention costs more tokens
than the sentence around it. The `CompactIds` marshal option (§13) shortens
ids and adds a `refs` legend to the envelope:

```json
"refs": { "miovm": "bafyreieqh63jv…miovm", "roman": "bafyreidfmzjh…" }
```

**`refs` is an authoritative opaque map.** Keys match
`[A-Za-z0-9_-]{1,64}`; values are full object ids. Keys need **not** be
suffixes of their values — "the id's last 5 characters" is merely export's
key-choice algorithm (suffixes, because CIDs share prefixes; a suffix that
collides with another referenced id's, or that the charset rejects, makes
that id stay uncompacted — the full-id fallback is always correct under the
resolution rule below). Agents editing a document may add entries with any label
(`"roman": "bafyrei…"`) and reference them; humans may rename keys.

**Resolution rule** — total, wherever an object id is expected: if the
value is a key in `refs`, it resolves to that full id; otherwise it **is** a
full id. There is no "short-looking" heuristic. Consequences: an unused
`refs` entry is ignored (export prunes them); two keys may map to one full
id (export never produces this); if a `refs` key equals a full id used
literally elsewhere in the document, the legend wins — export must (and,
with ≥5-char suffix keys vs 59-char CIDs, trivially does) choose keys that
collide with no full id present in the document. Import wiring MAY
additionally resolve unresolved ids by unique suffix against the target
space (useful for hand-written documents referencing known objects); that
behavior belongs to the wiring, not this package.

**Coverage** — with `CompactIds`, export rewrites every id-valued surface:

| Surface | Compacted |
|---|---|
| `objectId` props (file/image/video/audio/pdf, bookmark, link, dataview) | yes |
| mention / object-link targets in `text` | yes |
| `iconImage` (callout, and the `iconImage` property) | yes |
| property values of `objects`/`files` formats | yes |
| `items` | yes |
| view `defaultTemplateId`, `defaultTypeId` | yes |
| `objectOrders[].objectIds` | yes |
| filter `value` / sort `customOrder` entries of `objects`/`files` properties | yes |
| envelope `id`, `refs` values themselves | **never** |

Block/row/column/view ids are relabeled to their last 5 characters
(doc-local, no legend needed — same rule as refs keys). Labels are
constrained to the schema charsets (refs keys `[A-Za-z0-9_-]{1,64}`, local
relabels additionally dash-free); an id whose suffix collides or yields no
valid label stays uncompacted (implementation decision — fixed-width
suffixes with a full-id fallback, chosen over shortest-unique lengthening
for simplicity; 5 characters over CID/hex alphabets make collisions
birthday-rare).

`CompactIds` and `OmitIds` compose: together they yield the most
prompt-friendly form (no block ids, short object refs with legend). Both are
alternative serializations — the canonical round-trip form (§11) remains the
default full-id export.

## 10. Versioning and compatibility

- `version` is required; readers **reject** documents with a greater version
  than they support, with a dedicated error naming both versions (not a
  generic schema failure).
- Within version 1, evolution is **additive only**: new block types, new
  props, new mark syntax. This rule binds from the first non-draft release;
  while the format is a draft with no external consumers, breaking changes
  may occur under version 1 (as the v0.6 children→indent change did). The published schema is re-released with a minor
  suffix (`1.1`, `1.2`, …); documents citing an older `$schema` stay valid.
  When validation fails and the document's `$schema` minor is newer than the
  reader's, the error must say "produced by a newer version" rather than
  surfacing the raw constraint failure (export-new → import-old across
  devices is a normal user flow).
- Third-party consumers must skip-and-preserve blocks whose `type` they don't
  recognize (Portable Text rule). Our own importer validates against the
  bundled schema, so an unknown type is a validation error — by construction
  the importer is never older than the format it ships with.

## 11. Round-trip guarantees

Let `N(S)` be state normalization (given export and import wired with
equivalent resolvers, §3): structural blocks dropped, to be regenerated by
the editor at first open (§7); restrictions rebuilt (§4); properties
stripped per §3 (with the exemption list); select/multiSelect option ids
replaced by name resolution — in properties, filter values, and custom
orders (§3, §6.2 — duplicate-named options collapse); deprecated snapshot
and block fields cleared (§2, §5); deprecated `Header4` re-styled to
`heading3` (§5); `checked` outside checkboxes dropped and marks on literal
blocks dropped (§5); marks normalized — emoji materialized, whitespace
boundaries shrunk, same-type overlaps truncated, adjacent ranges merged
(§8.3); file/bookmark `state` recomputed (§5); empty strings/arrays/objects
and default scalars dropped from block attributes and envelope fields — but
never from property values, whose presence is meaningful (§3, §4); tables
normalized and ids canonicalized
(§6.1, including empty-paragraph cells collapsing to absent cells); dataview
`activeView`, cached sort/filter formats, deprecated per-column date/time
fields and `value` on `empty`/`notEmpty`/`exists` leaves dropped, group
`index` derived from order (§6.2); scalar-stored select/objects/files
property values become single-element lists and the legacy file `hash`
migrates into `objectId` (§3, §5).

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

1. `Import(Export(S)) ≡ N(S)` — state-level equality on the snapshot after
   normalization.
2. `Export ∘ Import` is **idempotent and byte-stable**: for any valid
   document `J`, `Export(Import(J))` is the canonical form of `J`, and
   re-importing/re-exporting it is byte-identical. (Byte equality with the
   *original* `J` holds only when `J` is already canonical — import mints
   missing ids, merges marks, maps aliases like `heading4`/`equation`,
   absorbs top-level title/description blocks.)

Both properties are enforced by tests in the package: golden-file tests for
representative documents plus property-based round-trip tests over generated
states (all block types, mark overlap/adjacency/whitespace-boundary cases,
emoji, tables, dataviews, UTF-16 payloads such as astral-plane characters).

## 12. Validation

- Schema: JSON Schema **draft 2020-12**, hand-authored (the format
  deliberately diverges from proto shape), one file, blocks discriminated on
  `type`. The block definition is **non-recursive** — the flat encoding has
  no `children`, and table cells reference a dedicated `cellBlock`
  definition (same core, no table arm) so the block↔cell cycle is cut —
  which is what makes the block schema usable under strict/constrained
  decoding (see the v0.6 changelog). The one remaining recursive definition
  is the dataview **filter tree** (`filterNode` groups nest, §6.2) — it is
  inherent to the filter model; a reduced core-profile schema (planned
  follow-up) without dataview is fully non-recursive, and the reserved
  compact filter string (§6.2.1) removes it from the generation path. To keep validation errors usable for LLM
  producers, validation dispatches on the `type` const first (per-type
  `if/then` or programmatic pre-dispatch) instead of a flat 30-branch
  `oneOf` whose error output is noise. Output-only fields carry
  `x-output-only: true` (§4a). Annotated `x-app: Anytype` in line with
  `pkg/lib/schema`.
- Published at a stable URL and embedded in the package (`go:embed`);
  validated with `santhosh-tekuri/jsonschema/v6` (new dependency; the repo's
  existing `gojsonschema` is draft-07 only).
- Import = schema validation first, then semantic checks the schema can't
  express: **indent monotonicity** (§4 validity — errors name both
  indents), **leaf containment** and **row→column** (§4 containment), id
  uniqueness over the whole document (§4), table shape and cell rules
  (§6.1), envelope combinations (`items`/`templateFor`/`kind`, §2),
  `language`-vs-`fields.lang` conflicts, and **inline-markup parsing** (§8)
  — grammar errors report the block's JSON path and the offending snippet.
  The indent bound [0, 32] lives in the schema.
- These path-addressed errors are the guardrail for agent-generated
  documents: generate → validate → feed errors back. With the flat schema
  the generate step can also run under strict/guided decoding end to end.

## 13. Package layout and API

```
pkg/lib/anyblockjson/
  SPEC.md                    — this document
  ANOMALIES.md               — real-world data anomalies found by prod
                               round-trip testing, and how the format
                               handles each
  schema/object.schema.json  — the published JSON Schema (embedded)
  export.go                  — snapshot → JSON
  import.go                  — JSON → snapshot
  inline.go                  — marks ↔ inline markup codec (§8)
  table.go                   — table subtree ↔ columns/rows
  dataview.go                — dataview content mapping (§6.2)
  validate.go                — schema + semantic validation
  json.go                    — ordered canonical-JSON writer, enum tables,
                               proto value bridges, id helpers
  typeproperties.go          — typeProperties ↔ recommended lists (§2a);
                               GenerateSchema derived artifacts are planned
                               here (post-v1)
  roundtrip_test.go          — §11 property tests + state assertions
  golden_gen_test.go         — golden files (testdata/, -update to refresh)
```

```go
// FormatResolver reports the format of a property key, when known.
// Bundle properties are resolved internally; the resolver covers custom keys.
type FormatResolver func(key domain.RelationKey) (model.RelationFormat, bool)

// OptionResolver maps select/multiSelect option ids to names on export and
// names to ids on import (creating options is the import wiring's job).
type OptionResolver interface {
    OptionName(key domain.RelationKey, id string) (string, bool)
    OptionId(key domain.RelationKey, name string) (string, bool)
}

// PropertyDefinition describes a property object referenced by a type
// document (§2a).
type PropertyDefinition struct {
    Key    domain.RelationKey
    Name   string
    Format model.RelationFormat
}

// PropertyResolver maps property object ids to definitions on export and
// definitions back to ids on import; PropertyId receives the full definition
// so the wiring can create-and-return missing properties in one step (§2a).
type PropertyResolver interface {
    PropertyById(id string) (PropertyDefinition, bool)
    PropertyId(def PropertyDefinition) (string, bool)
}

// Marshal serializes a snapshot into canonical AnyBlock JSON.
func Marshal(sbType model.SmartBlockType, snapshot *model.SmartBlockSnapshotBase, opts Options) ([]byte, error)

// Unmarshal validates data and reconstructs a snapshot.
// Errors wrap *ValidationError with JSON-path–addressed issues.
func Unmarshal(data []byte, opts Options) (model.SmartBlockType, *model.SmartBlockSnapshotBase, error)

// Validate checks data against the embedded schema and semantic rules
// without building a snapshot.
func Validate(data []byte) error

// DetectFormat reports the version and $schema markers without validating —
// the cheap dispatch probe for import wiring.
func DetectFormat(data []byte) (version int, schemaURL string, ok bool)

// FormatVersion (= 1) and SchemaURL (the published schema location) are
// exported constants for the wiring's dispatch. The §8 inline codec is
// internal to the package — it is not part of the public API.

type Options struct {
    ResolveFormat     FormatResolver   // optional; nil = bundle-only resolution (§3)
    ResolveOptions    OptionResolver   // optional; nil = option values pass through as ids
    ResolveProperties PropertyResolver // optional; nil = type documents keep raw recommended-relation ids (§2a)
    OmitIds           bool             // export only: drop every id (§9)
    CompactIds        bool             // export only: shorten ids, emit refs legend (§9a)
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

The package is deliberately **pipeline-agnostic**: it depends only on
`pkg/lib/pb/model`, `core/domain`, `pkg/lib/bundle`, `util/text`, the proto
runtime (`gogo/protobuf/types`) and `santhosh-tekuri/jsonschema/v6` (§12).
It must not import anything from `core/block/import` or `core/block/export`
— including `anymark`; the inline codec is implemented in-package because
canonical, byte-stable rendering needs stricter guarantees than `anymark`'s
best-effort import parsing, while staying syntax-compatible with it (§8.1).
(Goldmark was authorized but turned out unnecessary: the deterministic
stack parser in §8.3 replaces CommonMark emphasis resolution entirely.)

Wiring (follow-up work, not this package):
- Export: a `core/converter/anyblockjson` shim implementing
  `converter.Converter` (extension `.json`), passing objectStore-backed
  format and option resolvers.
- Import: an entry that dispatches on the `version`+`$schema` markers so
  `RpcObjectImportRequest` accepts the format. This must be built on the
  ImportV2 engine (branch `go-7349-import-refactor`), not the legacy import
  pipeline, and must supply resolvers equivalent to the export side's (§3),
  including create-missing-option behavior.

## 14. Full example

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
      "text": "func main() {\n\tfmt.Println(\"hi\")\n}" },
    { "id": "b7", "type": "table",
      "columns": [ { "id": "c1" }, { "id": "c2", "width": 120 } ],
      "rows": [
        { "id": "r1", "isHeader": true, "cells": [ "Format", "Size" ] },
        { "id": "r2", "cells": [ "pb.json", "3020 B" ] }
      ] },
    { "id": "b8", "type": "callout", "iconEmoji": "💡",
      "text": "See the [ADF docs](https://developer.atlassian.com/cloud/jira/platform/apis/document/structure/) for the reference shape" },
    { "id": "b9", "type": "dataview",
      "objectId": "bafyrei…tasksSet",
      "properties": [
        { "key": "name", "format": "shortText" },
        { "key": "status", "format": "select" },
        { "key": "dueDate", "format": "date" }
      ],
      "views": [
        { "id": "v1", "type": "kanban", "name": "By status",
          "groupBy": "status",
          "sorts": [
            { "property": "dueDate", "direction": "asc", "emptyPlacement": "end" }
          ],
          "filters": [
            { "property": "dueDate", "condition": "less", "datePreset": "currentWeek" },
            { "property": "done", "condition": "equal", "value": false }
          ],
          "columns": [
            { "property": "name" },
            { "property": "dueDate", "width": 120, "align": "right" },
            { "property": "status", "aggregation": "countDistinct" }
          ]
        }
      ]
    }
  ]
}
```

## 15. Open questions

1. **Extension**: `.json` vs `.any.json` for exported files (current pbjson
   uses `.pb.json`). Leaning `.json` — the `$schema`/`version` markers
   disambiguate for the importer.
2. **`dataview` vs `database`**: kept `dataview` (ownership semantics differ
   from Notion databases; Obsidian precedent) — flagged as a judgment call.
3. **Option names vs `{id, name}` objects** (§3): names-only chosen for
   generatability; Notion's `{id, name}` shape is the fallback if the
   duplicate-name/rename caveats prove unacceptable.
4. **Mention syntax**: `<mention objectId="…">` tag vs unifying with the
   `anytype://` link form plus a marker. The tag is unambiguous and
   LLM-friendly; confirm clients are happy rendering it.
5. **Emoji materialization** (§8.1): confirmed lossy-by-design (the mark
   disappears, its rendering is preserved). Acceptable, or does any surface
   still need the mark itself?
6. **Icon block**: only appears in legacy profile objects — confirm it must
   round-trip or can be dropped like other structural blocks.
7. **`typeProperties` naming** (§2a): alternatives considered —
   `definition.properties` (extra nesting) and a `schema` field (collides
   with `$schema`, and the section is more than a schema). The `section`
   enum vs three booleans is also open; the enum gets mutual exclusion for
   free.
8. **Property documents** (`kind: "property"`?): custom select/multiSelect
   properties own their option lists; a future section may specify them the
   same way types are specified in §2a (resolved options by name, not id).
9. **Trim system-property noise** (follow-up): refine §3 "presence is
   meaningful" — keys in `bundle.SystemRelations` are machine-stamped
   metadata (`isHidden`, `revision`, `relationFormatIncludeTime`, …) and
   could safely omit empty/default values, keeping documents compact for
   LLMs; presence stays preserved for user-intent keys via a small
   exception list (`name`, `description`, `iconEmoji`, `iconImage`,
   `done`) and for every non-system key. Deliberately static (no
   type-schema lookup at export time) to keep the canonical form
   deterministic. Decide `done` membership and wire `buildProperties` +
   the round-trip comparator accordingly.
