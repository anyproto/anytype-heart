# AnyBlock JSON — format specification

Status: **draft v0.10** · Format version: **1** · Package: `pkg/lib/anyblockjson`

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

Changes in v0.10: **admission runs on the resolved stored key** (§3, §12).
The v0.7 property checks — the internal-key deny rule, the layout-name
check, the format-shape warning — predated the v0.9 key vocabulary and keyed
off the raw document spelling, so the canonical slug walked past all three:
`{"unique_key": "ot-page"}` validated clean and landed a `uniqueKey` detail,
which is one of the keys the importer uses to pick which *existing* object a
snapshot merges into. The `property_keys` legend was the same hole squared —
an unchecked rebind primitive that bound any spelling to any stored key,
`id` included. Now every `properties` key resolves (legend → bundled table →
verbatim) before the checks run, import re-runs the deny rule on its own
resolved key for vocabularies wider than Validate can see, and a legend value
obeys the writable-key rule (schema-enforced).

Changes in v0.9: the key vocabulary of §3 arrives from the API v2 branch —
types and properties are named by their snake_case api slug, inverted through a
table in both directions, never a case transform — and with it
**`property_keys`**, the envelope legend that makes the slug layer invertible
from the document alone (§3). Without it, a slug derived from a space's stored
key reads back as a *different* relation in any reader that cannot ask that
space: a 36 808-object sweep found 12 objects re-pointed exactly that way, and
the reader-side half of the same defect — the accept side bound a spelling the
emit side refuses to write — is fixed with it.

Changes in v0.8: **the format's own vocabulary is `snake_case`** — 100
identifiers, every block type, field name, enum value and inline tag attribute
the format defines, with the rule and its two exemptions written down in §1's
*Naming*. The format itself passes property keys through as it is given them,
and platform identifiers are quoted, not translated. A caller that wires a key
vocabulary (§3, the API v2 path) hands the format slugs, so its documents read
`due_date` in both halves; one that does not gets the stored `dueDate` in the
property half and `snake_case` in the format half, and both are correct.
Canonical bytes change and the format version stays 1,
the same call the flat-blocks change made at v0.6: the format is a draft with
no external consumers, so a rename now costs a golden regeneration, and after
the freeze it costs a version.

Changes in v0.7 (pre-freeze review, `PREFREEZE_REVIEW.md` Tier 1). One
byte-changing rule and five reader rules, all inside format version 1:
**canonical output escapes every tag-shaped `<`**, reserving the whole
`</?[A-Za-z]` space for later versions and closing the delimiter set in
exchange (§8.2, §10); one **id domain** across every id surface, so sanitizing,
compacting and generating cannot collide, and a derived cell id is claimed
whether or not its cell is written (§4, §6.1, §9); **Validate and Unmarshal
agree** — schema-integer fields read as JSON numbers with `minimum`/`maximum`,
column `width` is an integer, and a number outside `float64` is rejected
wherever it appears (§12); a **date with no RFC 3339 form** is written as a raw
number instead of an unreadable string (§3); **property-key admission** is
symmetric with export's stripping, with a writable-key rule and format-shape
warnings (§3, §4a); and **one fault produces one validation issue** rather than
the validator's own bookkeeping (§12).

Changes in v0.7 (API v2 scope split): the §6.2.1 compact filter syntax is **split in scope**,
resolving the contradiction with API v2 (`core/api/APIV2.md`), which made
the filter-string parser a launch dependency while this document still
called the whole feature reserved/post-v1. The grammar and its parser ship
**now** as a library subpackage (`filterstring/`, §13) consumed by the API
v2 search/sets request surface; the **document** side is unchanged — the
view field `filter` stays reserved post-v1, export keeps writing the
structured array, and a v1.0 reader on a document carrying `filter` still
reports "produced by a newer version". §12's generation-path note updated
to match.

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

Changes from v0.4: type documents specified (§2a) — `kind: "object_type"` with a
`type_properties` array replacing the four recommended-relation id lists; a
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

### Naming

**Every identifier the format defines is `snake_case`**: block types, field
names, enum values, and inline tag attributes. The spelling is the internal
name run through the same conversion Anytype's public REST API applies to its
own keys (`strcase.ToSnake`, `core/api/util/key.go`), **digits included** —
`heading_1`, `toggle_heading_1`, `bulleted_list_item`, `table_of_contents`.
Stating the rule rather than a list means a name added later needs no decision.

This follows the two vocabularies §1 claims lineage from — Notion's API
(`bulleted_list_item`, `heading_1`) and Anytype's public API
(`background_color`, `added_at`) — and, more to the point, it is the spelling a
generating model produces unprompted: the format's own pre-freeze review
records an LLM writing `"type": "bulleted_list_item"` against a camelCase
draft, which the reader then had to reject.

**Two kinds of string are exempt, both because they name something outside the
format.** They are not inconsistencies to be tidied away later:

- **Property keys** (§3) are stored relation keys, written exactly as stored:
  `iconEmoji`, `dueDate`, `lastModifiedDate`. Renaming them would need a
  key ↔ key mapping the reader cannot invert (`Validate` takes no resolver),
  and it would reintroduce a collision class the format currently does not
  have (§11). When the API's snake_case property keys become the stored keys,
  this exemption disappears on its own with no format change.
- **Platform identifiers** — the reserved widget targets `allObjects` and
  `recentOpen` (§2c), the `dataview` block id (§7), and the `objectId`
  parameter of the `anytype://object` deep link (§8.1) — name things that
  exist in a live space. They are quoted, not translated.

(The JSON Schema's own `$defs` names — `blockCore`, `tableCell`, … — are
neither: they are schema-internal labels a document never contains, and they
keep JSON Schema's conventional camelCase.)

So `{"type": "callout", "icon_emoji": "💡"}` and
`{"properties": {"iconEmoji": "🔥"}}` are both correct in the same document:
the first is a field this format defines, the second is a key belonging to
the data.

### Terminology

The format uses six Anytype concepts; everything else is borrowed vocabulary:

- **object** — a page-like unit (page, task, note, …); one JSON document per
  object.
- **property** — a typed key-value on an object (Notion's term; stored
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
  "$schema": "https://schemas.anytype.io/anyblock/1/object.schema.json",
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
| `kind` | string | no | System-level object kind, snake_case (`page`, `profile_page`, `template`, `archive`, `widget`, `chat`, …) — from `model.SmartBlockType`. `chat` is `ChatDerivedObject`: a standalone chat object whose identity is `key`, like a type's; its messages live in the CRDT store, not in snapshots, so it always imports empty. (`chat_object` is the deprecated predecessor; `discussion` is a hidden type.) **Omitted whenever derivable**: absent means `page`, and `type: "template"` implies `template`. An unrecognized value is a validation error listing the allowed values. |
| `id` | string | no | Object id. Written by export; import treats it as informational (a new id is minted on import) except for resolving intra-export links. Never compacted (§9a). |
| `type` | string | no | The object's type **slug** (`page`, `task`, `object_type`…) — the key vocabulary of §3, not the stored `ot-`-prefixed key. Maps to `object_types[0]` in the snapshot. Absent when the snapshot has no object types (legacy/system objects). Import inverts the slug through the vocabulary in force (bundled table offline, the space's stored slugs inside a node) and hands the resulting stored key to the wiring, which resolves it — matching an existing type or creating one (the Markdown importer's behavior). A term the vocabulary does not know passes through verbatim, which is chain step 1. |
| `template_for` | string | no | Only for templates: the target type slug (`object_types[1]`), same vocabulary as `type`. Present with `type != "template"` → validation error. |
| `key` | string | no | Identity key of *system* objects (types, properties). This is the STORED identity key (a `uniqueKey`'s internal part), written verbatim: unlike every key slot in §3 it is **not** translated, so for an object whose stored key is a minted BSON it does not match the slug the public API serves as that object's `key`. Because it is verbatim, its charset is whatever the store already holds: a relation option's key is built from the option's *name*, so `completion_status_Not Started`, `…_C/C++` and `…_тогглы` are all real stored keys. The rule is therefore a deny rule — non-empty, no control characters, at most 255 characters — not an allowlist. An allowlist was tried and falsified: it failed 59 objects of a 36 808-object account, every one a relation option. Never emitted for ordinary documents. |
| `properties` | object | no | The object's properties, §3. |
| `type_properties` | array | no | Only for `kind: "object_type"` documents: the type's property definitions, §2a. Present on any other kind → validation error. |
| `refs` | object | no | Short-id legend for compact documents (§9a): maps labels to full object ids. Placed before `blocks` so the legend precedes use when read linearly. |
| `property_keys` | object | no | Legend: the stored property key each slug in this document names (§3). Written only for slugs the bundled table cannot invert, i.e. a space's own keys; a reader consults it **before** its own vocabulary. Absent from documents that use only bundled and verbatim keys, which is most of them. |
| `blocks` | array | no | The document's blocks as a **flat pre-order array**; nesting via `indent` (§4). |
| `items` | array | no | For collection objects: member object ids, in order (from the internal collection store key `objects`). Present on a non-collection document → validation error — enforced by the import *wiring* (collection-ness resolves against the space's types, not offline); the package's `Validate` checks structure only (implementation decision). |
| `store` | object | no | Escape hatch: remaining internal store content as a free-form JSON object, with the `objects` key lifted into `items`. Output-only (§4a). (Named `store` — its internal name — to avoid colliding with the collection concept.) |
| `root` | object | no | Escape hatch for non-default root-block attributes (`fields`, `background_color`); absent in the common case. Output-only (§4a). |

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

### 2a. Type documents (`kind: "object_type"`)

A type is not just a schema: it also owns the views over its objects
(columns, filters, sorts — presented through a dataview). Both live on one
object in the underlying model, so the format keeps them in **one
document** — a type never splits across files, and no JSON Schema file is
involved.

```json
{
  "version": 1,
  "kind": "object_type",
  "key": "task",
  "properties": { "name": "Task", "iconEmoji": "✅", "recommended_layout": "todo" },
  "type_properties": [
    { "key": "due_date",  "name": "Due date", "format": "date",    "section": "featured" },
    { "key": "assignee", "name": "Assignee", "format": "objects", "section": "featured" },
    { "key": "status",   "name": "Status",   "format": "select",
      "options": ["Backlog", {"name": "In progress", "color": "blue"},
                  {"name": "Done", "color": "lime"}] }
  ],
  "blocks": [ { "type": "dataview", … } ]
}
```

The type's own details (`name`, `icon_emoji`, `recommended_layout`, …) stay in
`properties` under their stored keys (§3). The four recommended-relation id
lists (`recommended_featured_relations`, `recommended_relations`,
`recommended_file_relations`, `recommended_hidden_relations`) are **replaced**
by `type_properties` — resolved entries, never raw relation ids.

`type_properties` entry fields (canonical order):

| Field | Type | Req | Notes |
|---|---|---|---|
| `key` | string | **yes** | Property key (as stored). |
| `name` | string | no | Display name. Import uses it only when the property must be **created**; an existing property keeps its own name. Every bundled key already exists, so a name given for one is inert — `{"key": "description", "name": "Summary"}` renders as *Description*. Validation warns. If the label is the point, mint a custom key instead of reusing a bundled one. |
| `format` | string | no | Property format (§3 names). Same import rule as `name`; a conflict with an existing property's format is an error at the wiring level (the package cannot see the space). |
| `options` | (string \| object)[] | no | A select/multi_select property's **vocabulary, in display order**. Each entry is a bare option name, or `{"name": …, "color": …}` when the option's color is part of the design — the color belongs to the option rather than to a parallel array, so inserting or reordering an option cannot shift it. `color` is one of `grey`, `yellow`, `orange`, `red`, `pink`, `purple`, `blue`, `ice`, `teal`, `lime` (`util/constant`); anything else is a validation error rather than a silently ignored value. The bare string is **canonical** whenever the option declares no color, the object form otherwise — the same rule cells follow in §6.1. Leaving a color out does not mean *no* color: the wiring assigns one, cycling the palette in declaration order and skipping whatever the vocabulary claims explicitly, so a vocabulary that names no colors still gets distinct ones. (The app assigns one at random on every other creation path; cycling keeps a converted bundle identical run to run.) Options are otherwise discovered only from values that happen to be used, so a vocabulary entry no record carries would never exist — its kanban column simply absent — and a discovered option carries no `orderId`, which makes every select sort alphabetically (options order by `[orderId, name]`, `pkg/lib/database.BuildOrderMap`). Declaring them lets the wiring create each one up front with an order id. Every option needs one: the sort concatenates `orderId + name` before comparing, so an option missing an order id is compared by *name* against the others' order ids and lands arbitrarily — ahead of the whole vocabulary when its name sorts below the id alphabet, behind it otherwise. Names discovered from usage rather than declared are ordered after the declared ones. Only meaningful on `select`/`multi_select`; duplicate names are a validation error, across both forms. |
| `object_types` | string[] | no | The **type slugs** an `objects`/`files` property may point at, in priority order — a type-key slot like the envelope `type`, so it speaks the one key vocabulary (§3) and import inverts it; a term the vocabulary does not know passes through verbatim. Empty means any object — an untargeted property will happily accept a random page as a task's assignee. Listing the built-in `participant` alongside a bundle's own people type is what makes the current-user filter value usable on that property (§6.2) while still allowing the seeded people as values; the client only offers it when the relation's targets include Participant. The wiring resolves each key to an id the way it resolves properties: a type the batch defines by the id its own document carries, a bundled type by its bundled url (`_ot<key>`). Only meaningful on `objects`/`files`. |
| `section` | string | no | `featured` \| `hidden` \| `file` — which list the property belongs to. Absent = a regular (sidebar) property. |

Export emits entries in section order featured → regular → file → hidden,
preserving order within each list, and drops ids that no longer resolve to a
property (including the `_missing_object` sentinel of already-dangling
references); legacy lists that store bare property **keys** instead of ids
resolve through the reverse lookup, falling back to the bundle for system
properties. The canonical form writes `name` and `format` on every entry
(`format` defaults to `text` when absent on input), and writes the
`type_properties` array **even when empty** — its presence is what tells
import to rebuild the lists. Import then rebuilds all four id lists — empty
sections become explicit empty lists, matching how type objects store them —
resolving each `key` against the space and creating missing properties (the
same policy as select option names, §3). A document without a
`type_properties` field leaves the lists untouched.

Property ids are space-local, so the rewrite requires a property resolver
(`Options.ResolveProperties`, §13). Without one, export leaves the four
lists in `properties` as raw id lists, and import passes unresolved keys
through in place of ids for the wiring to reconcile — the same degradation
as option values without an option resolver (§3). A document carrying both
`type_properties` and any of the four raw lists in `properties` is ambiguous
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

## 2c. The bundle index (`index.json`)

Every document described so far is one object. **A bundle also needs to say
things about itself** — what the space is called, what opens when a user
enters it, what the sidebar shows — and none of that belongs to any single
object. That is `index.json`, one file at the bundle root, validated against
`index.schema.json`:

```json
{
  "$schema": "https://schemas.anytype.io/anyblock/1/index.schema.json",
  "version": 1,
  "name": "Company Wiki",
  "description": "Everything we know, with an owner.",
  "icon_emoji": "📚",
  "homepage": "page-wiki-home",
  "widgets": [
    { "target": "page-wiki-home" },
    { "target": "type-wiki-page", "layout": "view", "limit": 6 },
    { "target": "favorite", "layout": "compact_list" }
  ]
}
```

| Field | Meaning |
|---|---|
| `name` · `description` · `icon_emoji` | the space's own identity, applied on install |
| `icon_image` | the space icon as an image: the **object id** of an image in the bundle, the same thing the `iconImage` property means on any object (§3). Needs the image object *and* its file in the archive, so a generated bundle uses `icon_emoji` |
| `homepage` | what opens on entering the space: an object id, or the reserved `widgets` (the sidebar dashboard, the default) or `graph` |
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

A widget is `{ target, layout, limit }`. `layout` is `link · tree · list ·
compact_list · view`, defaulting to `link` and omitted when default (§4).
`target` is an object id from the bundle — a page, a type, a set, a
collection — or one of the reserved listings `favorite · recent · set ·
collection`, which name a built-in rather than something the bundle ships.
Those four and no others: a live space also has `allObjects` and `recentOpen`
widgets, but the import path does not know those names
(`widget.IsPredefinedWidgetTargetId`), so a widget declaring one is **dropped
on install with no error** — see below. The tooling rejects them.

### How it reaches the space

A bundle is installed with `ObjectImportExperience`, which reaches
`builtinobjects.CreateObjectsForExperience`. That is a different path from the
one the built-in use cases take (`inject`), and it reads much less. The two
outputs the wiring produces, and who reads them:

| output | written by | read by |
|---|---|---|
| `profile` at the archive root — `pb.Profile`, raw protobuf whatever format the snapshots are in, since `getProfile` reads it with `pb.Profile.Unmarshal` | `cmd/anyblockconvert` (`profile.go`) | `CreateObjectsForExperience` reads **`spaceDashboardId` only** |
| a snapshot with `sbType: Widget` among the objects — one root block plus a wrapper-and-link pair per widget | `cmd/anyblockconvert` (`widgets.go`) | the pb importer: `shouldImportSnapshot` admits a Widget snapshot precisely when the import type is `EXPERIENCE`, and `objectcreator.updateWidgetObject` merges its widgets into the space's own widget object |

| `index.json` | reaches the space as | effect |
|---|---|---|
| `homepage`, falling back to `entrypoint` | `profile.spaceDashboardId` | the space's `homepage` detail — what opens on **every** entry, and on this path the only thing that decides what a new user sees |
| `widgets` | the Widget snapshot's root children, in order | the sidebar |
| `entrypoint` | `profile.widgets[0].targetObjectId` | the object the install opens **once** — on the `inject` path only. On a bundle's own path it lands only through the `homepage` fallback above |
| `name` | `profile.name` | nothing, on this path |
| `icon_image` | `profile.avatar` | nothing, on this path |

Five consequences worth stating, because none is obvious from the wire format:

- **`profile.widgets` is inert here.** `CreateObjectsForExperience` never
  calls `getWidgets` or `createWidgets`; those belong to `inject`. The wiring
  still fills the field, so an archive it produces is also a valid built-in
  archive, but nothing on a bundle's own path reads it. **The sidebar comes
  from the Widget snapshot**, which is also how a real app export carries it —
  export a space and the widget object is a file under `objects/` while the
  export's own `profile` has `"widgets": []`.
- **`name` and `icon_image` are discarded on this path.**
  `CreateObjectsForExperience` calls `setWorkspaceSettings(profile, spaceId,
  false)`, and that function applies `profile.Name` and `profile.Avatar` only
  when `isBundle` is true. The space keeps whatever name the caller of
  `ObjectImportExperience` gave it.
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
  and only an explicit `"widgets"` or `"graph"` gives up a real page.

**A widget target that does not resolve loses the widget, silently.** This is
the only reference in the format whose failure produces no diagnostic at all:
`common.handleLinkBlock` rewrites a link target it cannot resolve to
`addr.MissingObject`, and `WidgetObject.Init` then removes the broken link
*and* its now-empty wrapper. The import succeeds, the widget is not there, and
the only trace is a log line. That covers both an id no document in the bundle
defines and a reserved listing the importer does not recognise
(`allObjects`, `recentOpen`). Both are therefore errors in `anyblockvalidate`
and `anyblockconvert` rather than something an author discovers by installing.

Nothing per-object substitutes for this file. In particular **`is_favorite` is
not an entry point**: it adds an object to Favorites and nothing more. It
does not open anything, create a widget, or set the homepage.

Ids in `index.json` are the bundle's own — the same slugs every other
document uses — and the wiring relinks them like any other reference. Whether
they resolve is a cross-document question this package does not answer
(§13): an index validates on its own terms while naming an object no
document defines.

## 3. Properties

`properties` is a JSON object keyed by **property key**, always in the
snake_case **api slug** spelling — `due_date`, `icon_emoji`,
`manual_property` — bundled, API-created and UI-created keys alike. One
vocabulary, no aliases, no duality: a reader never has to know which kind of
key it holds. (This overturns the earlier "as stored, camelCase" rule, and
it is the format half of the same decision the API surface makes —
APIV2_ADDRESSING.md §7.5a, §7.3.)

The mapping is a **table, both directions, never a case transform**: for
bundled keys the derived table in `pkg/lib/bundle` (which ships with every
reader, so documents still resolve offline), and for every other key the
entity's stored `apiObjectKey`, which a node-backed reader primes from the
space. `mediaArtistURL` → `media_artist_url` → `ToLowerCamel` would yield
`mediaArtistUrl`, and `_score` does not round-trip at all — string inversion
cannot be the reverse mechanism, and the package's tests pin both cases.

A key the vocabulary does not know passes through verbatim in both
directions: an exact stored key is always an address (the resolution chain's
first step), which is what keeps a package-only reader — with no space to
ask — lossless on custom keys.

**The document carries its own inverse: `property_keys`.** The slug layer is a
compaction of key *spelling*, and like every compaction in this format it has
to be invertible from the document alone — the rule §9a already states for
object ids. A slug derived from a space's stored `apiObjectKey` is not:
`6a32d485…` spelled `priority` reads back as the key `priority` in any reader
that cannot ask that space, which is a different relation, silently. So export
writes the entry:

```json
"property_keys": { "priority": "6a32d4856761631534b22f85" }
```

- **Emitted only where the bundled table cannot invert.** A bundled key needs
  no entry (`due_date` → `dueDate` ships with every reader), and a key spelled
  as itself is its own address (chain step 1). The legend is therefore empty
  for documents a package-only reader wrote, and costs one line per space-slugged
  key otherwise.
- **Consulted first, before any vocabulary.** The legend is the only statement
  the *document* makes about its own spellings; a vocabulary belongs to the
  reader, and two readers disagreeing about a slug is exactly how a property
  ends up naming a different relation than it was exported from.
- **A stored key keeps its own term.** When a slug would collide with a key
  another property on the same object is stored under, the later holder is
  spelled with its stored key instead — the term belongs to the entity that
  answers to it, and the legend never rebinds a spelling out from under one.
- **A legend value is a stored key, and obeys the writable-key rule.**
  Non-empty, no control characters, at most 128 characters — the same shape
  rule property names carry, enforced by the schema. Export only ever
  records values that passed it, so the bound refuses nothing export writes.
- **The legend cannot launder a spelling onto an internal key.** Entries are
  honored during validation and admission exactly as during import — the
  legend is step one of key resolution — so `{"prio": "uniqueKey"}` does not
  smuggle a `uniqueKey` write past the §3 deny rule: the *resolved* key is
  what admission judges (see below). Conversely, a legend entry that binds a
  denied *slug spelling* to a harmless stored key (a space-minted
  `apiObjectKey` may collide with a bundled spelling) is honored too: nothing
  lands on the internal key, so nothing is refused.

**What is not a key slot** (ADDRESSING §7.5a-4). The vocabulary applies where
a document NAMES a type or property, and nowhere else. Envelope and DTO field
names, enum *values* (`kind: "objectType"`, layout and view-type names), the
`index.json` envelope, view field names like `defaultTemplateId`, and — the
one most easily mistaken for a key — **block attribute names**: a callout's
`iconEmoji` and `iconImage` are attributes of a block, not property keys, and
stay camelCase however the `icon_emoji` *property* is spelled one section
over. `objectType` the layout value coexists with `object_type` the type key,
and that is intended.

Values are encoded by the property's format:

| Format | JSON encoding |
|---|---|
| `text` (default), `url`, `email`, `phone` | string |
| `number` | number |
| `checkbox` | boolean |
| `date` | RFC 3339 date-time string, UTC (`"2026-07-06T15:04:05Z"`); import converts back to unix seconds. Import also accepts date-only strings (UTC midnight), non-UTC offsets (converted to UTC), and fractional seconds (truncated to whole seconds). Export always writes the full UTC form — **except** for a stored value outside the years RFC 3339 can express (0000–9999), which export writes as the **raw number**, with a warning. There is no string form for such a value, and writing one anyway (`"57482-01-22T22:43:20Z"`, from milliseconds stored where seconds belong) would not parse back, so the value would return as a *string* on a date property and stay one. A reader must therefore accept a number here; the number is a stored value it cannot interpret as a date, not a second date encoding. |
| `select`, `multi_select` | array of option **names** (strings) — see below |
| `objects`, `files` | array of object ids (strings) |
| unresolvable format | value passes through verbatim in both directions |

**Layout is named, not numbered.** `recommended_layout`, `layout` and
`resolved_layout` are stored as numbers (their bundled relations have format
`number`), but the format writes the enum **name** — `basic · profile · todo ·
set · object_type · relation · file · dashboard · image · note · space ·
bookmark · relation_options_list · relation_option · collection · audio · video ·
date · space_view · participant · pdf · chat_deprecated · chat_derived · tag ·
notification · missing_object · devices · discussion`. A bare integer would be
the one opaque enum in an otherwise self-describing format. Import maps the
name to its number and still accepts a raw number, so older documents keep
working; export always writes the name; an unrecognized name is a validation
error. Note this applies only to these three keys — `layoutAlign`,
`layoutWidth`, `widgetLayout` and `headerRelationsLayout` hold *different*
enums and pass through as numbers.

Format names follow the public REST API (`select`, `multi_select`, …);
internally they map to `model.RelationFormat` (`status`→`select`,
`tag`→`multi_select`, `longtext`→`text`,
`object`→`objects`, `file`→`files`; `emoji` and `properties` exist for
internal formats).

**There is one text format, `text`.** The editor offers a single Text
property type; the stored `longtext`/`shorttext` split is legacy, carries no
meaning an author could act on, and is **not part of this format** —
`shortText` is not a valid format name and is rejected by the schema.

The collapse is not lossy, because `text` resolves per key rather than
blindly:

- **Export** writes `text` for both stored formats.
- **Import** reads `text` as the key's *existing* format when that key is
  already known to be `shorttext` — bundled properties (`name`, `icon_emoji`,
  `cover_id`, …) and anything the wiring's `ResolveFormat` recognizes. So a
  short-text property keeps its stored format across a round-trip even
  though the document never names it.
- Otherwise `text` means `longtext`, which is what a **new** property
  declared as `text` becomes.

Any other format name is taken literally — the document is authoritative
about its properties, and only the text/text collapse needs a key to
disambiguate.

**Properties are space-wide, not per-type.** Two types whose
`type_properties` name the same select share one option pool, so their
vocabularies merge into a single dropdown. That is the point for a property
whose values are genuinely common (`tag`) and a defect for the lifecycle
selects a schema reaches for, where the same word means different things per
type. Documents that want distinct vocabularies must use distinct keys.

**Select options are names, not ids — everywhere.** This rule covers
property values here, filter `value`s, and sort `custom_order` entries
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
| `name` | text | the object's title |
| `description` | text | subtitle/description line |
| `icon_emoji` | text | page icon as emoji |
| `icon_image` | files | page icon as image (object id) |
| `cover_id` / `cover_type` | text / number | page cover — output-only (§4a) |
| `done` | checkbox | completion state on task-like types |
| `due_date` | date | due date on task-like types |

**Canonical key order in `properties`** (implementation decision): the
well-known keys `name`, `description`, `icon_emoji`, `icon_image` first (in
that order, when present), then all remaining keys alphabetically.

**Presence is meaningful.** A key's presence in `properties` records that the
property was set on the object — clients use it to show the property even
when its value is empty. The §4 omit-empty canon therefore does **not**
apply to property values: every key present in the snapshot is written, with
its value verbatim — including `false`, `0`, `""`, `[]`, and explicit
`null`. Import preserves them all (an explicit `null` stays a null value).
Omitting a key and writing an empty value are different statements: absent =
property not set; empty value = property set, currently empty.

**Value shape** (implementation decision): select/multi_select and
objects/files values are always JSON arrays; import stores them as lists, so
internally scalar-stored values (e.g. `creator`) normalize to
single-element lists on round-trip (§11).

**Stripping.** Export removes internal/derived properties
(`bundle.LocalAndDerivedRelationKeys`) **except** those the importer
meaningfully preserves (mirroring `core/block/import/pb`): `created_date`,
`last_modified_date`, `creator`, `is_favorite`, `is_archived`, `resolved_layout`.
Those six are **output-only** (§4a): export writes them, generators should
not — with one deliberate exception. **`is_favorite` is authorable**, because
the pb importer reads it to choose a space's root objects
(`core/block/import/pb/space.go`), which is how a generated bundle
designates the object a user should land on. A bundle with no favourite, no
`homepage` and no `spaceDashboardId` imports as an undifferentiated list. `id` is lifted to the envelope and `type` to `type`. Everything else
round-trips.

**Admission is symmetric: import refuses exactly what export strips.** The
list above is the only list — the reader derives its deny-list from it rather
than restating it, because a restated list drifts, and the drift ran one way:
import used to accept every key an author supplied, so `isArchived`,
`isDeleted`, `spaceId`, `restrictions` and `uniqueKey` all landed on details
while export removed them. Setting one is an error naming the key. Two keys
that are not properties at all are refused with them, because they are how
the importer decides which *existing* object a document merges into
(`core/block/import/common/objectid/existingobject.go`): `oldAnytypeID` and
`sourceFilePath`, alongside `uniqueKey` from the list. Export strips those
three too, so the symmetry holds in both directions. `id` and `type` are
refused by name as well — they are the envelope's (§2), and dropping them in
silence left an author with no explanation for why the id they wrote had no
effect. (Those two are refused as *spellings*: the importer lifts them into
the envelope before any resolution runs, so the legend cannot re-purpose
them.)

**Admission runs on the resolved stored key, not on the raw spelling.** The
document spells slugs, so a reader first lands each `properties` key on its
stored key — the document's own legend, then the bundled table, then the
spelling verbatim (the §3 chain) — and *then* applies the deny rule, the
layout-name check and the format-shape warning to the result. Checked
against the raw spelling instead, all three were dead for exactly the
documents this format produces: `unique_key` walked past the rule that
`uniqueKey` tripped, and a `property_keys` entry could rebind any harmless
spelling onto any internal key — including `id` itself, which overwrote the
envelope id from inside `properties`. `Validate` resolves with the only
vocabulary it has (legend, bundled table, verbatim — it takes no resolver,
§13); a reader whose vocabulary resolves *further* — a node-backed caller
whose space maps a slug to a stored key the bundled table never knew — must
re-run admission on **its** final resolved key, which import does at the
seam where details are written (`importer.build`). The two agree exactly
whenever no wider vocabulary is in force, which is what keeps Validate and
Unmarshal accepting the same documents (§12).

**A property key has to be writable.** Non-empty, no control characters, at
most 128 characters (`propertyNames` in the schema). This is a *deny* rule and
not an allowlist on purpose: real keys are bundled lowerCamel names, bson-hex
ids, and bare names from old accounts, and an allowlist could only be trusted
after checking every key in every account — while the shapes ruled out here
(the empty key, a key with a newline in it) are keys nothing can read. Export
drops such a stored key with a warning, since there is no way to write it.

**A value whose shape its format cannot hold is a warning**, not an error, and
only for keys the bundle resolves after the resolution chain runs (`Validate`
takes no resolver, §13): `"due_date": "next Friday"` is stored as written and
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
| `indent` | integer ≥ 0 | no | Nesting depth. Absent = `0` (top level); canonical form omits `indent: 0`. Values above **32** fail validation (adversarial-input bound; real documents reach ~6). See the nesting rules below. |
| `type` | string | **yes** | Discriminator; full inventory in §5. Unrecognized values fail schema validation (see §10 for forward compatibility). |
| `id` | string | no | `[A-Za-z0-9_-]{1,64}`. Uniqueness is enforced over the whole document, including derived table cell ids `<rowId>-<colId>` (§6.1) — a non-table block id that collides with a derived cell id is a validation error. Dataview **view** ids are the one exception: they are unique **within their dataview block**, not document-wide (§6.2). Export writes ids by default — the `OmitIds` option drops them (§9); import generates missing ids with the editor's standard id generator. |
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
  always last), then `align`, `vertical_align`, `background_color`, `fields`.
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
without them, and where a supplied value would not be safe to take it is
refused rather than quietly used: the internal property keys are a deny-list
in the reader (§3), which is where "authoritative only where semantically
safe" is actually implemented. Most output-only fields carry
`x-output-only: true` in the JSON Schema so tooling can warn; the two that
cannot are `coverId`/`coverType` and the six preserved internal properties,
which live inside the free-form `propertyMap` and so have no schema node of
their own to annotate.

Output-only surfaces: `fields` (any block), `root`, `store`, `source`
(dataview), `groups`/`object_orders` (views, §6.2), `id` on sorts/filters,
filter `nested_property` (reserved), `cover_id`/`cover_type`, and the six
preserved internal properties listed in §3.

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
| `bulleted_list_item` | Text/Marked | `color`, `text` (Notion/BlockNote naming) |
| `numbered_list_item` | Text/Numbered | `color`, `text` (numbering is derived from position among consecutive siblings; never stored) |
| `toggle` | Text/Toggle | `color`, `text` |
| `callout` | Text/Callout | `icon_emoji`, `icon_image` (file object id), `color`, `text` |
| `toggle_heading_1` … `toggle_heading_3` | Text/ToggleHeader1..3 | `color`, `text` |
| `file` `image` `video` `audio` `pdf` | File (Type enum promoted; `Type_None` → `file` with no `object_id`) | `object_id` (target file object), `name`, `mime_type`, `size` (bytes), `style` (`auto · link · embed`), `added_at` (RFC 3339; omitted with a warning when the stored timestamp is outside the representable years, §3 — unlike a property value there is no number form to fall back to). Legacy `hash` accepted on input. On export, a block with only the legacy `hash` set writes it as `object_id` (the hash migrates on round-trip, §11); when both are set, `object_id` wins and the hash is dropped. `state` is not serialized: import sets `Done` when `object_id`/`hash` is present, `Empty` otherwise. File blocks are leaves in the editor, but legacy data can nest real blocks under them — indented descendants are allowed and round-trip verbatim |
| `bookmark` | Bookmark | `url`, `object_id` (target bookmark object). `state` handled like file blocks. Deprecated preview fields and `type` (derivable) are dropped — preview data lives on the target object |
| `link` | Link | `object_id` (target object), `card_style` (`text · card · inline`), `icon_size` (`none · small · medium`), `description` (`none · manual · content`), `properties` (string array: property keys shown on the card). Deprecated `style` and legacy `fields` are dropped |
| `divider` | Div | `style` (`line · dots`, default `line`) |
| `row` / `column` | Layout/Row, Layout/Column | — (descendants carry content; a `row` contains only `column`s — §4 containment) |
| `group` | Layout/Div (legacy) | semantics-free container |
| `table` | Table (+ structural children) | `columns`, `rows` — see §6.1 |
| `embed` | Latex | `processor`, `text` (**literal**, §8.4) — see §5.2 |
| `table_of_contents` | TableOfContents | — |
| `property` | Relation | `key` (property key; renders the property inline) |
| `dataview` | Dataview | fully specified in §6.2 |
| `widget` | Widget | `layout` (`link · tree · list · compact_list · view`), `limit`, `view_id`, `auto_added` |
| `chat` | Chat | — (rare) |
| `featured_properties` | FeaturedRelations | — structural, see §7 |
| `icon` | Icon | `name` (legacy profile objects only) |

Enum values serialize as lowerCamel strings; defaults are omitted.

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

`processor` selects the embed kind — full enum, lowerCamel: `latex`
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
  `SplitN(id, "-", 2)` (`table.ParseCellID`, which every column
  insert/delete/move, the HTML converter and table normalization depend on),
  so a `-` anywhere in a row or column id silently reassigns cells to the
  wrong column. `Options.GenerateId` belongs to the caller and need not
  respect that — the convert wiring derives ids from file paths — so import
  sanitizes generated ids into `[A-Za-z0-9_]{1,64}` and disambiguates
  collisions rather than trusting the generator. Export sanitizes stored ids
  the same way, since data predating this rule contains dashes and `Marshal`
  must never emit a document its own `Validate` rejects.
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
but do not own (closer to Obsidian's Dataview than to a Notion database).
Field-for-field from `Content.Dataview`, with cleaned names, lowerCamel
string enums, and defaults omitted:

```json
{
  "type": "dataview",
  "object_id": "bafyrei…targetSet",
  "properties": [
    { "key": "name", "format": "text" },
    { "key": "status", "format": "select" },
    { "key": "due_date", "format": "date" }
  ],
  "views": [
    {
      "id": "v1",
      "type": "kanban",
      "name": "By status",
      "group_by": "status",
      "sorts": [
        { "property": "due_date", "direction": "asc", "empty_placement": "end" }
      ],
      "filters": [
        { "property": "due_date", "condition": "less", "date_preset": "current_week" },
        { "property": "done", "condition": "equal", "value": false }
      ],
      "columns": [
        { "property": "name" },
        { "property": "due_date", "width": 120, "align": "right" },
        { "property": "status", "aggregation": "count_distinct" }
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
| `properties` | `relationLinks` | array of `{ "key", "format" }` — the properties available to this view, with formats per §3's vocabulary. **This field is live** (maintained by the dataview editor), unlike the deprecated snapshot-level relationLinks |
| `views` | `views` | see below |

Dropped (normalization): `activeView` (local UI state; the proto itself
excludes it from changes) and the deprecated proto `relations` field.

**View props** (`Dataview.View`), canonical order: `id`, `type`
(`table · list · gallery · kanban · calendar · graph`, omit `table` — note
the public API currently says `grid`; `table` is the more familiar term),
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
only id domain in the format that is not document-wide (§4), and the scope
is the one in which a duplicate does damage: a view reference always
resolves within a single dataview's `views` list, and the per-view editor
state below (`groups`, `objectOrders`) is keyed by view id inside that same
block, so a repeat inside one block makes the second view permanently
unaddressable. Across blocks, each view is reached through its own block and
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
matches `less` and `less_or_equal` regardless of the threshold: `Compare`
returns `1` when the filter carries a value and the record does not, and `1`
is what `Less` tests for (`pkg/lib/database/filter.go`). An "overdue" view
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
- leaf, canonical order: `property`, `condition`, `value`, `date_preset`,
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
  against the literal string and matches nothing. They are not object ids:
  import must not remap them and export must not compact them into the refs
  legend (§9a). Valid only on `objects`/`files` properties, since they
  resolve to an object id; anywhere else is a validation error. Note the
  date presets are a *different* mechanism — a first-class `quickOption`
  field with real Go-side semantics (§6.2, `quickoptions.go`) — and the
  template-placeholder feature (`model.Placeholder_PlaceholderCurrentUser`)
  is unrelated: it fills property defaults when an object is created from a
  template, is resolved in Go, and never appears in a filter.

  `number_of_days_ago` and `number_of_days_now` are the two presets that **take an
  operand**: `getDateRange` reads the day count from `value`
  (`pkg/lib/database/quickoptions.go`), so they are the one case where a
  preset and a `value` legitimately coexist, and a leaf carrying one without
  a `value` is a validation error — the count would default to `0`, silently
  meaning today. Because the count is meaningful data rather than an absent
  field, export writes it even when it is `0`, overriding the usual
  empty-elision (§4). A preset resolves to a day *range*, and the condition
  picks the endpoint: `less`/`greater_or_equal` compare against the range
  start, `greater`/`less_or_equal` against its end.

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

**Status: split scope (v0.7).** The grammar below and its parser ship
**now**, as the library subpackage `pkg/lib/anyblockjson/filterstring`
(§13): parse a filter string → the §6.2 structured filter tree
(`model.BlockContentDataviewFilter` nodes), with **offset-addressed
errors** naming the offending token and its position. Its consumer is the
API v2 request surface (`core/api/APIV2.md` Phase 4 — `POST …/search` and
the `filter` field of `POST …/sets`), where the string is the documented
small-model form and both request forms land on one internal tree. The
grammar is thereby pinned by the parser and served via the API's discovery
surface (APIV2.md §5).

The **document** side is unchanged and stays reserved: v1 documents ship
the structured `filters` array only. The view field name `filter`
(singular, string) is **reserved** for a post-v1 extension: v1 schemas do
not define it, so introducing it later is an additive 1.x release (§10 — a
v1.0 reader encountering it reports "produced by a newer version"; export
keeps writing the structured array; the `CompactFilters` export option
stays reserved in `Options`). When that lands, the two forms coexist
permanently — `filter` and `filters` mutually exclusive per view, import
accepting both, export choosing via option.

The design, normative for the parser (and unchanged for the future
document extension): a view carries its filter as a single SQL/JQL-flavored
query string:

```json
{ "type": "kanban", "group_by": "status",
  "filter": "done = false AND (due_date < current_week() OR due_date IS EMPTY)" }
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
`yesterday() · today() · tomorrow() · last_week() · current_week() ·
next_week() · last_month() · current_month() · next_month() · last_year() ·
current_year() · next_year() · daysAgo(n) · daysFromNow(n)` (the parameterized
pair maps to `number_of_days_ago`/`number_of_days_now` with the value as `n`;
parens distinguish presets from string literals). Property keys are bare.
Select/multi_select values are option **names**, per §3 (the structured form
agrees since v0.4; only date values differ — RFC 3339 here, unix numbers
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
(`identStart identPart*` — letters, digits, `_`); presets are **excluded
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
preserves `resolved_layout` in `properties` (§3) and leaves structural blocks
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
one, and pages can contain orphaned empty leaves. Export drops a childless
content-less block and serializes one with children as a transparent
`group`, so the subtree survives (part of `N(S)`, §11).

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
  names version 1 knows: `<sub>x</sub>` in prose exports as
  `\<sub>x\</sub>`. This is the tag namespace's **reserved syntax space**
  (§10) — see the note below.
- `&` — escaped only where a valid entity follows. Recognized entities:
  `lt gt amp quot apos nbsp` and numeric (`&#65;`, `&#x41;`).
- `\` — escaped when followed by ASCII punctuation (input accepts a
  backslash before any ASCII punctuation as an escape, per CommonMark).

**Reserved syntax space** (the one escaping rule that is not minimal, and
why). A `text` string carries no version marker, so bytes are the only thing
a later version has to work with. If canonical output escaped `<` only for
`u`/`font`/`mention`, a version-1 document could contain a literal
`<sub>x</sub>`, and the day a version adds `sub` those same bytes read as
markup — the reader cannot tell version-1-literal from version-2-markup, and
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

Implementation decisions (v0.4 freeze):

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
- All `object_id` props and mark targets are object ids, opaque to this
  format; there are no intra-document block references (table cell ids are
  derived, §6.1).
- Import id policy: missing → generated (the editor's standard generator);
  provided → validated for uniqueness (§4) and charset, preserved so that
  re-exports diff cleanly.
- On output, export writes ids by default (stable diffs, §11 canon). The
  `OmitIds` marshal option (§13) instead drops **every id in the document**
  — blocks, table rows/columns, views, sort/filter ids — along with the
  id-dependent output-only view state (`groups`, `object_orders`). For
  templates, prompt examples, and any content meant to be re-inserted rather
  than diffed. An id-less export is valid but not the canonical round-trip
  form (re-importing mints fresh ids).

### 9a. Compact ids

Full object ids are ~59-character CIDs; a single mention costs more tokens
than the sentence around it. Compaction has two independent halves (§13):
`CompactObjectRefs` shortens object references and adds a `refs` legend to
the envelope (lossless — the legend inverts it); `CompactBlockLabels`
relabels doc-local block/row/column/view ids to short suffixes (legend-less,
lossy). `CompactIds` remains as shorthand for both. The split exists because
consumers legitimately want one without the other, and because the two
halves pay differently: API v2 default reads use block labels (the server
resolves them by unique suffix) and keep object refs full inline, while its
export shape — the backup/round-trip shape, whose bytes must re-import to
the same document — keeps block ids full and takes the legend (API spec
C4). Legend example:

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

**Coverage** — with `CompactObjectRefs` (or `CompactIds`), export rewrites
every id-valued surface:

| Surface | Compacted |
|---|---|
| `object_id` props (file/image/video/audio/pdf, bookmark, link, dataview) | yes |
| mention / object-link targets in `text` | yes |
| `icon_image` (callout) and the `iconImage` property | yes |
| property values of `objects`/`files` formats | yes |
| `items` | yes |
| view `default_template_id`, `default_type_id` | yes |
| `object_orders[].object_ids` | yes |
| filter `value` / sort `custom_order` entries of `objects`/`files` properties | yes |
| envelope `id`, `refs` values themselves | **never** |

With `CompactBlockLabels` (or `CompactIds`), block/row/column/view ids are
relabeled to their last 5 characters
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

`version` is a **single integer with no minor axis**. It is required, it is
the sole authority on format identity, and it is checked before anything else
in the document is interpreted.

- **A reader rejects any document whose `version` is greater than its own**,
  with a dedicated error naming both versions rather than a generic schema
  failure. There is no partial or best-effort read of a newer document and no
  forward compatibility: a change a version-1 reader cannot handle is exactly
  what a version bump means.
- **A reader accepts any document whose `version` is less than or equal to its
  own**, migrating older documents forward before parsing. Version 1 has
  nothing to migrate from, so the migration mechanism ships with the first
  migration that needs it; because `version` is required and unambiguous, a
  later migration has complete information about the grammar a stored
  document used.
- **Every format change bumps the version.** There is no additive-within-a-
  version rule, because there is nothing additive to have: the schema closes
  every object (`additionalProperties: false`) and every enum is exhaustive,
  so a new block type, a new property, a new enum value, a new mark, or a
  renamed key is rejected whole-document by an older reader regardless of how
  it is introduced. Saying so plainly is cheaper than a reserved-field
  mechanism that buys nothing under the rule above.
- The `$schema` URL carries the same integer
  (`https://schemas.anytype.io/anyblock/<version>/object.schema.json`) and is
  **decorative**: it is optional, no reader gates on it, and the schema at a
  version's URL is mutable in place — a correction that does not change the
  format is republished there rather than given a new number. Version 2 gets
  `anyblock/2/`. Format identity lives in `version` and nowhere else.
- `index.json` shares the same version number and the same rules (§2c), and a
  bundle is versioned as one artifact: if the index or any document in it
  declares an unsupported version, the whole bundle is rejected rather than
  partially imported.

**Syntax inside `text` is versioned too, and the reader is exact about it.**
A `text` string carries no version marker of its own, so the only thing that
keeps a stored document readable across a bump is that the reader recognizes
*exactly* the syntax its version defines and treats everything else as
literal. This binds three namespaces:

| namespace | version 1 recognizes | anything else | status |
|---|---|---|---|
| inline tags (§8.1) | `u`, `font`, `mention` | literal text, never an error — reported as a warning, since canonical output would have escaped it | **reserved**: canonical output escapes every tag-shaped `<` (§8.2), so the whole `</?[A-Za-z]` space is free for later versions |
| Markdown delimiters (§8.1) | `**` `*` `~~` `` ` `` `[…](…)` | literal text | **closed**: the set is complete; a future mark is a tag, never new punctuation |
| `anytype://` destinations (§8.1) | `anytype://object?objectId=<id>`, one parameter | a plain Link, preserved verbatim | matched by exact form, so a second parameter is available to a later version |

Being exact is what makes a later migration possible: when a version adds a
tag, a delimiter, or a deep-link parameter, the migration escapes or rewrites
the prior occurrences that a stored document meant literally, and it can only
do that if version 1's rule was unambiguous. A reader that guessed — matching
a deep link by prefix, say, and taking whatever followed as the id — would
have already destroyed the information a migration needs.

The reservation is what keeps that migration from being needed at all for
canonical documents: because export escapes tag-shaped `<`, a version that
adds a tag can read version-1 documents as they are. Only hand-written
documents can carry an unescaped tag-shaped sequence, which is why import
warns about one instead of silently accepting it — the warning is the
author's notice that those bytes are only literal by virtue of the document's
`version`, and that canonical form spells them `\<`.

**The cost this accepts.** When version 2 ships, a client still on version 1
cannot open *any* document a version-2 client exported — refused, not
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
the editor at first open (§7); restrictions rebuilt (§4); properties
stripped per §3 (with the exemption list); select/multi_select option ids
replaced by name resolution — in properties, filter values, and custom
orders (§3, §6.2 — duplicate-named options collapse); deprecated snapshot
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
migrates into `object_id` (§3, §5).

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
   missing ids, merges marks, maps aliases like `heading_4`/`equation`,
   absorbs top-level title/description blocks.)

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
  decoding (see the v0.6 changelog). The one remaining recursive definition
  is the dataview **filter tree** (`filterNode` groups nest, §6.2) — it is
  inherent to the filter model; a reduced core-profile schema (planned
  follow-up) without dataview is fully non-recursive, and the compact
  filter string (§6.2.1 — its parser ships as the `filterstring`
  subpackage for the API query surface; the *document* field stays
  reserved) removes it from the generation path. To keep validation errors usable for LLM
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
  (§6.1), envelope combinations (`items`/`template_for`/`kind`, §2),
  **property-key admission on the resolved stored key** (§3 — each
  `properties` spelling resolves legend → bundled table → verbatim before
  the deny rule, the layout-name check and the format-shape warning run;
  import re-runs the deny rule on its own resolved key when a wider
  vocabulary is in force), `language`-vs-`fields.lang` conflicts, and
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
  typeproperties.go          — type_properties ↔ recommended lists (§2a);
                               GenerateSchema derived artifacts are planned
                               here (post-v1)
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
    CompactIds        bool             // export only: shorthand for the two flags below (§9a)
    CompactObjectRefs bool             // export only: shorten object refs, emit refs legend (§9a; lossless)
    CompactBlockLabels bool            // export only: relabel doc-local block/row/column/view ids (§9a; lossy)
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

The bundle index (§2c) has its own pair, since it is not an object snapshot:

```go
func UnmarshalIndex(data []byte) (*Index, error)
func MarshalIndex(idx *Index) ([]byte, error)
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
      "text": "func main() {\n\tfmt.Println(\"hi\")\n}" },
    { "id": "b7", "type": "table",
      "columns": [ { "id": "c1" }, { "id": "c2", "width": 120 } ],
      "rows": [
        { "id": "r1", "is_header": true, "cells": [ "Format", "Size" ] },
        { "id": "r2", "cells": [ "pb.json", "3020 B" ] }
      ] },
    { "id": "b8", "type": "callout", "icon_emoji": "💡",
      "text": "See the [ADF docs](https://developer.atlassian.com/cloud/jira/platform/apis/document/structure/) for the reference shape" },
    { "id": "b9", "type": "dataview",
      "object_id": "bafyrei…tasksSet",
      "properties": [
        { "key": "name", "format": "text" },
        { "key": "status", "format": "select" },
        { "key": "due_date", "format": "date" }
      ],
      "views": [
        { "id": "v1", "type": "kanban", "name": "By status",
          "group_by": "status",
          "sorts": [
            { "property": "due_date", "direction": "asc", "empty_placement": "end" }
          ],
          "filters": [
            { "property": "due_date", "condition": "less", "date_preset": "current_week" },
            { "property": "done", "condition": "equal", "value": false }
          ],
          "columns": [
            { "property": "name" },
            { "property": "due_date", "width": 120, "align": "right" },
            { "property": "status", "aggregation": "count_distinct" }
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
4. **Mention syntax**: `<mention object_id="…">` tag vs unifying with the
   `anytype://` link form plus a marker. The tag is unambiguous and
   LLM-friendly; confirm clients are happy rendering it.
5. **Emoji materialization** (§8.1): confirmed lossy-by-design (the mark
   disappears, its rendering is preserved). Acceptable, or does any surface
   still need the mark itself?
6. **Icon block**: only appears in legacy profile objects — confirm it must
   round-trip or can be dropped like other structural blocks.
7. **`type_properties` naming** (§2a): alternatives considered —
   `definition.properties` (extra nesting) and a `schema` field (collides
   with `$schema`, and the section is more than a schema). The `section`
   enum vs three booleans is also open; the enum gets mutual exclusion for
   free.
8. **Property documents** (`kind: "property"`?): custom select/multi_select
   properties own their option lists; a future section may specify them the
   same way types are specified in §2a (resolved options by name, not id).
9. **Trim system-property noise** (follow-up): refine §3 "presence is
   meaningful" — keys in `bundle.SystemRelations` are machine-stamped
   metadata (`is_hidden`, `revision`, `relation_format_include_time`, …) and
   could safely omit empty/default values, keeping documents compact for
   LLMs; presence stays preserved for user-intent keys via a small
   exception list (`name`, `description`, `icon_emoji`, `icon_image`,
   `done`) and for every non-system key. Deliberately static (no
   type-schema lookup at export time) to keep the canonical form
   deterministic. Decide `done` membership and wire `buildProperties` +
   the round-trip comparator accordingly.
