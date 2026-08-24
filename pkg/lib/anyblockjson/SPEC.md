# AnyBlock JSON — format specification

Status: **draft v0.32** · Format version: **1** · Package: `pkg/lib/anyblockjson`

A human- and agent-readable JSON serialization of Anytype objects (the "anyblock"
model), designed for export, import, and generation by external tools and LLM
agents. It replaces the raw `jsonpb` dump (`.pb.json`) as the recommended JSON
interchange format.

Design lineage: the envelope and block tree follow the Atlassian Document
Format (nested `type`-discriminated tree, a single `version` integer —
though evolution here is never additive, §10); inline formatting uses a
Markdown subset inside text
strings; the vocabulary follows Notion's API and Anytype's public REST API
(`core/api`) wherever an established term exists — the format should be
readable, and mostly writable, by someone who has never seen Anytype
internals.

Changes in v0.32: **a property is described by one shape, wherever it is
described** (§2e) — the property-dictionary change.

**A bundle carries a property dictionary** (§2f): `properties.json` at the
bundle root, sibling of `index.json`, with its own published schema. One
file naming every property the bundle's objects use — `installed` lists the
bundled keys present as keys only (98% of installed copies are
field-identical to the bundled table, measured over 9,675), and
`properties` carries one `propertyDefinition` per property actually
referenced, used-only (a space installs a median 125 bundled properties and
uses 57), plus a full entry for each of the 174 divergent installed copies.
Keys are STORED keys — the legend already binds a document's labels to
them, so the dictionary needs no legend of its own — and every entry
carries its `format`, required by the schema, because a third-party reader
must interpret a backup WITHOUT shipping `bundle/relations.json`: that
self-sufficiency constraint is what killed the simpler "just stop exporting
bundled relations" idea. A dictionary entry is the third home of
`$defs/propertyDefinition`, referenced across schema files by its published
URL; an `installed` key the reader's table cannot name is skipped, never
refused, so a newer app's backup stays readable one version back — while
the writer refuses what its own table cannot name.

**`index.json` gains a manifest** (§2c): stored type key → file, option id →
file, and a pointer to the dictionary — the only two namespaces documents
address without a path (types 22/space, options 34/space; 5.1 KB/space,
0.23%), so a reader resolves a type without scanning and without the folder
convention the spec has never defined. And **composition omits a bundled
relation document whose definition matches the table** — the §11 N(S)
addition, owned by exported predicates (`OmittedBundledRelation`,
`RelationInstallArtifactKey`, `InstallStampedDefault`) that the round-trip
comparator reads rather than copies, fail-closed on every axis: a divergent
member, an unclassified key, an alien-kinded value or a real block each
keep the document. The import wiring reads the dictionary as a declaration
source: an entry feeds the batch's format table and is minted up front with
its whole declared shape, whether or not any type lists it.

**`relation_settings` groups the relation definition** (§2d). The three
members v0.31 put at the document root — `format`, `include_time`,
`object_types` — move into one group that is a layer over
`$defs/propertyDefinition`: a relation document is literally an envelope
plus one property definition. Churn on freshly shipped fields, accepted
deliberately: the alternative was two patterns for one idea, and the
dictionary is expected to lift `description`/`max_count`/`readonly`/
`default_value` as well, at which point a relation has seven definition
members and the root would have been wrong anyway. Presence still mirrors
stored presence exactly, so the snapshot comparator again needed no new
rule; both older spellings (the v0.31 root members, the pre-v0.31 raw
number in `properties`) are refused with the repair named.

**`type_settings` holds everything defining the type** (§2a). One gated
subtree carries the five settings lifted from `properties` — `layout`
(recommendedLayout), `api_key` (apiObjectKey — NOT `slug`: the document's
own label differs from it on 941 of 1,326 corpus types), `plural_name`,
`default_template`, `default_view` — plus `property_definitions`, the array
that lived at the root as `type_properties`. The flat spellings are refused
in `properties` ON TYPE DOCUMENTS only, because the lift is kind-scoped:
`apiObjectKey` is real data on 9,725 relation documents, where it stays an
ordinary property. And a type document stops carrying its own install
provenance: eight keys dropped, each admitted individually against 1,760
corpus type documents, five candidates kept because they carry something
real — the verdicts are in §2a, the normalizations in §11 N(S), and the
comparator reads them through the format's own predicates.

The schema now publishes `$defs/propertyDefinition`, and every surface that
describes a property is a layer over a reference to it: a
`property_definitions[]` entry is a propertyDefinition plus `section` — the
ONE field that belongs to the type rather than the property, proven by the
corpus (of 1,614 properties declared by 2+ types within one space, zero
differ in anything else). The shape carries the ten decided members: the
five every home already spoke (`key`, `name`, `format`, `options`,
`object_types`) and the five the dictionary lifts (`description`,
`include_time`, `max_count`, `readonly`, `default_value`). A home layers
narrowings over the reference rather than restating members — an authored
home may not declare `map`, a type's `object_types` is a real array — and
the codec threads the whole decoded definition to the resolver's create
path, so a member the schema admits is never shed at the seam. The homes
are asserted to REFERENCE the shape, the way `authorableFormat` is asserted
to be `propertyFormat` minus `map`, because a fourth spelling of "a
property definition" is exactly the §15 #14 disease.

Changes in v0.31: **a relation document states its own format, in the
format's own vocabulary, on the envelope** (§2d — the §15 #14 spelling
decision, taken).

A `kind: relation` document used to spell `relation_format: 100` inside
`properties` — a raw enum number — while a `type_properties` entry three
sections up spelled the same fact `format: "objects"`: one concept, two
spellings, in one format. And the raw spelling was a live trap, not a
blemish: in a 198-run small-model eval, 9 of 9 attempts wrote `properties:
{"format": "number"}`, which VALIDATED — inside `properties` every key is a
property spelling — and imported as a phantom custom property named
`format`, leaving the relation longtext forever, silently.

Three stored keys lift onto the envelope, and no others: `relationFormat` →
`format` (required, a §3 NAME), `relationFormatIncludeTime` →
`include_time`, `relationFormatObjectTypes` → `object_types` (a type-key
slot under the same legend discipline as `property_definitions[].object_types`,
with the id↔key translation supplied by the new `TypeResolver` capability).
The flat spellings are refused in `properties` with the repair named — the
§2b precedent, refusal included. Unlike §2b, the envelope fields mirror
stored presence EXACTLY (false, `[]` and null all travel): #14's verdict was
to fix the spelling and leave the emptiness collapse to its own change, and
mirroring is what lets the snapshot comparator run with no new rule.

Measured over 38,061 production documents (10,617 relation documents, the
whole population): every one carries `relationFormat`, so requiring `format`
refuses nothing; `include_time` is true only on dates (543 of 9,035
present), null on 80; `object_types` is non-empty only on objects/files
(1,089 + 167 of 10,159 present); none of the three keys occurs on any other
kind. Format 102 (`map`, the bundled `templatePlaceholders` relation) occurs
72 times, which forced the vocabulary total: **`map` is the fourteenth
format name** (§3), because a required name over real data may not have
holes. The full 38,061-document sweep re-runs clean: zero I1 violations,
zero import errors, `Export ∘ Import` byte-stable from the second
generation, and the §2d wrong-format warnings fire zero times.

Changes in v0.30: **a key is spelled the way its name reads** (§1, §3).

Two changes to the label rule, both measured against 38,061 production
documents, and both invisible to any reader that keeps its legend — a label
is legend-backed, so every document already written keeps resolving, and the
old spelling keeps resolving too, since the two are one fold class.

**The api slug's snake-caser is gone from normalization.** It splits acronyms
and digit runs, and a display name is full of both: `P2P Sync` →
`p_2_p_sync`, `Platform SDKs` → `platform_sd_ks`, `GitHub` → `git_hub`,
`Objectives S3Y24` → `objectives_s_3_y_24`. It was there so the format and
the api slug would converge; convergence lost. It also bought nothing on the
real input, because camelCase is a KEY phenomenon and this rule is fed a
display NAME, which separates its own words already.

**A stored slug is re-spelled by its name within one fold class.** The
mangled spellings above are STORED — the api minted them — so fixing the
normalizer alone would not reach them. Where slug and name name the same word
and disagree only about breaks, the name wins. Where the slug says something
the name does not (`restaurant_rating` for "Rating", `workspace_id` for
"Space") it keeps its spelling: 64 such properties in the corpus, untouched.
14 are re-spelled.

**A leading `_` run now survives normalization** — `_` is `identStart`, so
`__amemory_salience` never needed repair, and 20 relations from two
integrations namespace themselves that way.

Changes in v0.29: **seven system-stamped keys stop writing their emptiness**
(§3, §11, §15 #12).

`presence is meaningful` earns its keep for anything a person touched — an
empty `tag` list is someone having cleared the tags — and costs nothing on an
ordinary page. It costs ~20% on the documents this format most exists to be
read: `relationDefaultValue`, `relationMaxCount` and their neighbours appear
only on RELATION and TYPE documents, so an agent reading a space's schema pays
for every unset machine flag. Measured over 36,967 documents: 1.13% of all
bytes, but p50 1.21% against p90 13.55% and max 23.22%.

Seven keys — `isHidden`, `isHiddenDiscovery`, `isArchived`,
`relationReadonlyValue`, `revision`, `relationMaxCount`,
`relationDefaultValue` — are now written only when non-empty. It is a
**whitelist**: the blanket form over `bundle.SystemRelations` fails open on
every relation added later, and buys 0.04% of bytes for the privilege. Three
keys were considered and refused, and §3 says why for each.

Changes in v0.28: **the separator earns its keep at both ends** (§9, §11).

v0.27 made `#` the one load-bearing character in the id domain and split on
it unconditionally, which is safe for every id this format WRITES — no id
form can contain one, measured across 81,696 documents in two corpora — but
a snapshot's reference slots are untrusted (§11) and a document is
hand-written, so both ends met values the rule had never been tried on.

**Export now captions only an id the reader can uncaption.** An id already
carrying a `#` gets none, because `x#y` + `#name` reads back as `x`. The
degenerate case was worse: `#name` has no id half, import returns it whole
(the split refuses index 0 so it never invents an empty id), and an export
willing to caption it appended another name every generation, without
bound — from a document Validate accepted, and exactly the shape a writer
produces copying only the readable half of `id#name`. Validate now reports
that shape as a warning wherever it can see the property's format.

**A reader wired without `Options.SpaceId` says so instead of corrupting.**
The fold trades the space half of a participant id for the reader's own
space, so a reader naming none stores a bare identity where a composite
belongs. Every importer has a target space — an import lands in one — but a
converter that does not import has none, and that is the path the warning
exists for. It warns rather than refuses because Validate never sees
Options, and the two surfaces may not disagree about one document (§12 I2).

`N(S)` (§11) now lists the three residues these leave rather than claiming
the trim and the fold are pure inverses: they are inverses for every id
either side writes, which is a narrower promise than the one it made.

Changes in v0.27: **references get a readable half, and identity comes back
to attribution** (§3, §9, §11, §13). Three connected changes, one shape.

**Every object reference may carry an informative `#name` suffix** —
`bafyrei…#local_first_ux` — so a reader sees what a reference points at
instead of a 59-character CID. The suffix is informative only: import trims
it at the first `#`, unread, and a bare id stays exactly as valid (§9). It
is opt-in per shape (`RefNames`, default off): the export/backup shape stays
minimal and rename-stable, read shapes opt in.

**Participant ids fold to the bare identity.**
`_participant_<space>_<identity>` is the document's own space restated
around a 48-character identity; every reference slot — the participant
document's own envelope `id` included — now spells the identity alone, and
import rebuilds the composite against `Options.SpaceId` (§9). 135
characters down to 48, and a document carried into another space re-homes
the member correctly, because the reader rebuilds against ITS space.

**`creator` and `last_modified_by` return to a RESOLVABLE id**, spelled
`<identity>#<name>` (§3). v0.24's name-only spelling broke API v2 — its
consumers need an id to resolve a member, and 76 of 2,478 production
participants share a display name, so the name identified nobody. The v0.24
readability win is kept by the suffix; the resolvability it traded away is
back, at ~57 characters against the 135 the id had before v0.24. Both keys
stay derived and stay DROPPED on import; only the spelling changes.

Measured over the same 37,429-object corpus (offline resolvers built from
the corpus itself; baseline = v0.26 tip, same resolvers): the backup shape
costs **+1.70%** of all bytes (168.28 MB → 171.13 MB — the attribution id
returning), the read shape **+2.34%** (→ 172.21 MB) with 119,462 suffixed
references; 3,446 same-space participant composites fold, 0 remain; round
trip byte-stable on 37,429 of 37,429; data-loss findings identical to the
v0.26 codec (the 34 pre-existing coverId/coverType/tag findings, no new
ones). The sweep also surfaced 9,103 objects storing
`lastModifiedBy = _participant_<space>_` — a composite built from a BLANK
identity, which v0.24's omit-on-no-name rule had been hiding; an id that
addresses nobody is omitted (§3).

Changes in v0.26: **a key's label is normalized through the format's own
grammar** (§1, §3, §6.2.1, §11, §13).

A key the bundled table does not speak for used to be spelled by its stored
`apiObjectKey`, or — with none — by its stored key. Both halves borrow a
constraint this format does not have and miss one it does. `apiObjectKey` is
minted for API v2, where a slug is a URL PATH SEGMENT, so its charset is
`^[a-zA-Z0-9_]+$`: `Тоггл` transliterates to `toggl` and
`日本語のプロパティ` to `ri_ben_yu_nopuropatei`, unguessable and unreadable at
once. And a relation with no slug is spelled by a 24-character bson id — 39
relations and 18 types in a 36,966-object corpus, every one an ordinary user
property with an ordinary name (`Publish Date`, `Active competitors`,
`Website`).

The constraint the format DOES have is §6.2.1's: a key is a Unicode
identifier and not one of the filter grammar's keywords. **A bson id starts
with a digit, so it cannot be written as a filter key at all** — those 39
properties could not be filtered on, in a grammar this format serves to
models as EBNF. So the label is normalized through that grammar instead
(§3): a conforming stored slug unchanged — **but re-spelled by the display
name when the two are one fold class**, since there they name the same word
and only the slug is snake-cased — else that slug normalized, else the
display NAME normalized, else the stored key verbatim as before. Non-Latin
scripts are kept, never transliterated. All 223 bundled slugs already
conform, which a test asserts rather than assumes, and a cross-package test
asserts the other half — every label the rule mints parses as a key in the
subpackage that owns the grammar.

Measured over the same 36,966 objects, both codec halves, against the v0.25
tip:

- **474 documents change a spelling (1.28%)**, 471 of them shorter; the
  corpus shrinks by 13,681 bytes. The legend costs nothing new: 30,729
  entries before and after, because §3's exhaustive rule already owed one for
  every non-bundled key — only the label's own length changes.
- **398 documents stop spelling a bson id** (2,193 → 1,795), and 7 opaque
  keys become readable words.
- **No bundled label moves**, in either namespace: 0 of 12,166 vocabulary
  entries.
- **Only 1 of the 39 slugless relations gets a name label**, and the other 38
  are the format's existing rules doing their job rather than the new one
  failing: 15 names land on a spelling the BUNDLED table binds (`Due Date`,
  `Priority`, `Email`, `Phone`, `URL`), 11 on a spelling another relation in
  the same space already stores as its api slug (the duplicated-property
  shape a Notion import leaves), 8 on a live stored key, 2 are `id` and
  `type`, and 2 are named `#`. Every one of those keeps the stored key,
  which is what it had before. The population this rule can help is small
  because the population that is *both* nameless-in-the-store and
  unclaimed-in-its-space is small.
- **An explicit slug outranks a derived name**, which is not a tie-break
  detail: 14 name-derived labels in the corpus collide with a slug another
  relation minted years ago, and the flat twin rule would have taken the
  spelling from both.

Changes in v0.25: **`icon` and `cover` are typed envelope fields** (§2, §2b,
§2c, §3, §4a, §5, §9a, §11, §12).

Nine hidden stored keys — `iconEmoji`, `iconImage`, `iconName`, `iconOption`,
`coverId`, `coverType`, `coverScale`, `coverX`, `coverY` — sat in
`properties` as nine independent slots, and not one of them said which of the
others it excluded. Over 36,966 production objects that is **22 distinct flat
key-sets** for what is really one choice with eight shapes, plus one pair no
reader can decode: `"cover_id": "blue"` is a COLOUR under `"cover_type": 2`
and a GRADIENT under `"cover_type": 3`, and **both occur in the corpus**.
`cover_id` alone is undecodable, so this family could not be simplified any
other way — only typed.

They collapse into two envelope fields, `icon` and `cover`, each ONE object
whose `format` member selects the variant (§2b). The nine spellings are
refused in `properties`, and the refusal names the repair.

**The disease was not competing values.** Of 36,966 objects, 883 carry both
`iconEmoji` and `iconImage` and in **zero** of them are both non-empty: the
siblings are `""` and `[]`, present because §3 makes key presence meaningful.
The one real conflict is `iconName` + `iconEmoji`, exactly 200 objects, every
one a bundled type mid-migration from an emoji to a named icon — and the
emoji is carried on the named-icon branch as annotated baggage, because a
backup format that silently deletes a non-empty stored value is
disqualifying. A typed union deletes the whole empty-sibling class by
construction: an absent variant is absent, not empty.

**The encoding is `if`/`then`, not `oneOf`, and that is the point of the
change rather than a detail of it.** Measured through the package's own error
renderer:

| a model writes | `oneOf` | `if`/`then` |
|---|---|---|
| `{"format":"emoji","emoji":"📕","name":"rocket"}` | **10 issues**, three contradictory verdicts on `format`, twice told to delete the *correct* member | **1**: `/icon/name: property "name" is not allowed` |
| `{"format":"url","url":"…"}` | **12 issues**, none naming the alternatives | **1**: `/icon/format: value must be one of 'emoji', 'file', 'icon', 'color'` |
| `{"emoji":"🚀"}` | **6 issues** | **1**, naming the union |

`branchLeaves` (§12) cannot prune the difference: it prunes branches that
failed on the instance's own TYPE, and every icon branch is `type: object`.
A small model spends most of its time in the failure path, and that one line
naming the union is the whole reason to type the field.

**The fields live in the ENVELOPE, not in `properties`**, for reasons that
are forced rather than aesthetic (§2b). `cover` is already a stored property
key in 30 production objects, `pageCover` in 66 more — both Notion imports,
neither bundled. A `properties` member can be rebound by the `property_keys`
legend to point at any relation at all, which is both an I1 hole and a
laundering primitive. `properties` carries presence-is-meaningful while the
envelope omits empties. And an envelope member has a schema node of its own
to annotate, which closes a gap §4a recorded and could not fix.

Measured over the same 36,966 objects, whole codec, against the v0.24 tip:

- **14,992 objects gain a typed field** — 14,949 an `icon`, 126 a `cover`,
  83 both.
- **1,358 carry one of the nine and produce neither**: every source present
  and empty. This is the one place the change overrides
  presence-is-meaningful, and the carve-out is principled — all nine
  relations are `hidden: true`, so there is no property row for presence to
  be meaningful *to*.
- **33 objects lose a cover, and it was already dead.** Every one is
  `coverType: 1` holding an absolute path into a long-gone temp directory
  (`/var/folders/…/anytype_notion_import/….png`), written by
  `core/block/import/notion` as if a path were a file reference. That is
  **25% of every image cover in the corpus**, permanently corrupt by exactly
  the mechanism the typed shape exists to prevent. The drop turns silent
  corruption into a named warning.
- **+253,804 bytes, +0.157%.** 0 export errors, 0 invalid documents, 0
  byte-unstable round trips.

Three seams land with it, and none is optional. The **callout block icon**
and the **`index.json` icon** move onto the same shape (§5, §2c) — shipping
the envelope field alone would leave two icon conventions inside one
document. `snapshotdiff` learns that a source stored empty is not a source:
without that amendment the same sweep reports **3,038 findings over 2,307
objects**, nearly double the noise that buried a previous one, and with it 66
over the 33 that really lost something.

Changes in v0.24: **attribution is a name, not an address** (§3, §4a, §11, §13).

`creator` and `lastModifiedBy` held a 135-character participant id — the
single most repeated string this format wrote, present on 100% of 36,966
production objects. They now hold the member's display **name**, as a plain
string: `"creator": "Roma Kha"`. Not an array; the relation is `maxCount: 1`
and 0 of 36,966 values were multi-valued, so the list wrapper was
definitionally wrong rather than merely verbose.

The name is safe HERE and nowhere else. Both relations are
`source: derived, maxCount: 1, readonly: true`: the value is recovered from
the object tree root's own cryptographic signature on every rebuild
(`treeSource.GetCreationInfo`), and four independent seams already discard
whatever a document supplies —
`state.StructCutKeys(details, LocalAndDerivedRelationKeys)`, the pb importer's
preserve-list, `changeBlockDetailsSet`, and the API's "cannot be set
directly". The exported line was already informational, so there is no
fidelity to lose. `assignee`, `author`, `stakeholders` and every custom
`objects` property are UNCHANGED — they are `source: details`, chosen by a
person, and the id is the whole of their meaning. Spelling one as a name
would be data loss, and the temptation is real.

**Two members of one space can carry the same display name** — 76 of the
2,478 participants in the production corpus answer to `Roma Kha` — so the
name identifies nobody, and nothing tries to resolve it back. Import DROPS
both keys, in silence, like a transient key and for a related reason: no
write path could honour the value. That closes an asymmetry nobody could
state a reason for, where `creator` was accepted (it sat on the preserve-list
it never belonged on) and `lastModifiedBy`, one word apart in the bundle with
an identical definition, was refused outright.

**No name, no property.** With no participant resolver wired, or a member
this space has no name for, the key is OMITTED — never the raw id, never a
blank. A format whose `creator` is sometimes a name and sometimes a
135-character address is worse to read, and worse to write a reader for, than
one that consistently carries neither.

Measured over the same 36,966 objects, against the v0.23 tip:

- **−2.36% of all bytes** (167,564,862 → 163,612,878). Dropping the
  attribution entirely would have been −3.48%; the 1.12pp is what the two
  name lines cost, and it buys back the attribution a readable format wants.
- **90.1% of documents carry a creator name** with names resolved from the
  corpus's own participant objects. Of the rest, 7.9% name
  `_anytype_profile` — the app itself, whose participant object is not in an
  export but is in a live space index — and 2.0% a member whose profile name
  is empty in that space. In a live node the first group resolves too.
- `lastModifiedBy` is on **100%** of objects, not the 71 an earlier count
  suggested; those 71 were the relation OBJECTS for it, one per space,
  matched on their `api_object_key`.

What it costs, stated plainly: an object carrying a creator is the first
thing that is not an id for which `Export(S) ≠ Export(Import(Export(S)))`
(§11, guarantee 3). Import drops the name, so a second export has nothing to
write. Nothing there is recoverable and none of it was ever data — an
imported object gets the importing account's own creator from its own new
tree — and the loss happens exactly once: the second and third exports are
byte-identical.

One amendment lands with it (§9a): the term census reserves a spelling
exactly when the emit writes it. The attribution keys are on the stripped
list — their stored VALUE never reaches a document — so the ordinary details
walk stopped seeing them, and a custom relation whose api slug is `creator`
would then claim the spelling and collapse both properties onto one JSON
member.

Changes in v0.23: **transparent containers — the format stops spelling a
rendering budget** (§4, §5, §7a, §9a, §11, §12).

A `Layout/Div` is not a block. `state.wrapChildrenToDiv` mints one whenever a
parent exceeds 40 children — a 2020 performance constant, retuned twice
inside the same month — so no user gesture makes one, none of the 7,303 in
the production corpus carries a single attribute, and nothing in any snapshot
references one. A block whose content oneof is unset is the same thing with
even less to it. Both are now **transparent containers** (§7a): export writes
nothing for them and lifts their children to the container's own depth,
import does the inverse, and the editor's normalization re-creates whatever
wrappers it wants on the other side, for free.

What it buys, measured over 36,967 production objects:

- **It closes an I1 hole that is live today.** A `Layout_Row` with more than
  40 columns normalizes to `row > div > columns`, which `Marshal` emitted and
  its own `Validate` rejected — `/blocks/1: a row block can only contain
  column blocks, got group` — making the object unexportable AND
  unrestorable. Containment is now judged against the LIFTED tree (§12), so
  `row > group > column` says what it becomes: `row > column`.
- **It repairs 160 real objects.** Their own dataview sits at indent 1 inside
  a wrapper, where §7's primary-dataview pin — which fires only at indent 0 —
  never fires: under `omitIds` the id `dataview` was lost and the editor added
  a second, empty dataview on open. Lifted, 160 gain the id and 0 lose it.
- **The indent bound stops being burned on structure no reader can see.**
  Indents in the corpus reach **26**; 9,128 blocks sit at indent ≥ 7. With
  containers lifted the deepest real nesting anywhere is **6**, and 0 blocks
  sit at 7 or deeper. 145,750 of 159,301 emitted `indent` lines disappear.
- **3,349,749 bytes** across the 709 affected documents (5.44% of them,
  ~10% at the median, 24% worst case), byte-identical in both id modes.

What it costs, stated plainly. Content order and content parentage survive
in 709 of 709 affected documents and no block's indent ever increases — but
the wrappers that come back are **not the same wrappers**: after one round
trip 38.8% of documents get an identical partition, 39.1% a different one,
and 22.1% none at all, because the content has since shrunk below the
threshold that created them. That last group is a repair, not a loss.
Regeneration is therefore conditional and non-deterministic in shape, unlike
`title`/`header`, which the editor rebuilds deterministically from the type;
`N(S)` (§11) says so. An authored `group` carrying attributes loses them AND
its children stop being its children — they become siblings of what followed
— which is reported through `OnWarning` on the lenient path and is invisible
to a strict caller. And the format now depends on a heart constant it hides
but does not control: move `maxChildrenThreshold` and the hidden layer moves
under a read that did not change. Tolerable only because the layer is
semantics-free, which is why the rule is scoped to `Layout/Div` plus
content-less and never to "any `group`".

`group` stays a **readable** input token that no export produces — `Validate`
must keep accepting what `Unmarshal` accepts (I2) — so it stays in the
schema's `blockCore.type` enum and in `BlockTypeNames`, and leaves
`AuthorableBlockTypeNames`. The rule keys on CONTENT, never on the `div-` id
prefix the normalizer happens to mint: keying on a prefix would make id
spelling semantically load-bearing, and would leave an authored
`{"type": "group"}` round-tripping into a permanent wrapper.

One amendment lands with it and is not optional (§9a): the id census counts
what the document SPELLS. Without it, dropping a container frees the
suffix slot it was holding, so a paragraph sharing its 5-char tail stays full
on the first read and compacts on the second — `Export(S) ≠
Export(Import(Export(S)))`, guarantee 3, on the API's default read shape.
Measured: the export lift without the amendment breaks byte-stability under
compaction on 2 production documents; with it, 0 of 36,967. (One of those 2
was already broken before the lift, for the same reason with a table cell in
place of a container.)

Changes in v0.22: **two namespaces stop overloading a value that was already
saying something else** (§1, §2, §2c, §3, §10, §15).
(1) The reserved `index.json` widget targets and homepages move into the
platform's own address space: `_favorite`, `_recent`, `_set`, `_collection`,
`_all_objects`, `_recent_open`, `_widgets`, `_graph` — and **no bundle-local
object id may begin with `_`**. They were bare words in the same namespace as
the ids a bundle mints, and the collision was reachable in both directions:
the pb importer resolves a widget target through the bundle's own id map
before asking `widget.IsPredefinedWidgetTargetId`, so an object with id `set`
silently captured every widget meaning *the Sets listing* (and made
`Index.EntryPoint` disagree with `EffectiveEntryPoint` about what the bundle
opens), while `setWorkspaceSettings` matches the reserved homepages *before*
resolving an id, so an object with id `graph` could never be one. A prefix
rule closes both permanently, where a word list has to grow a retroactive ban
per listing. The prefix is translated back off at the wire boundary
(`WireWidgetTarget` / `WireHomepage`), a `_`-prefixed target that is not one
of the six is refused by name with the inventory in the message, and a
generated id derived from a file called `_drafts.json` is escaped rather than
minted. The prefix alone would only have moved the collision downstream — the
importer's own spellings are bare, so the six bare words are unmintable as
bundle ids too.

(2) **`kind` is the sole authority on what a template is** (§2, §3, §10).
`template` was a reserved TYPE SPELLING: `template_for` was admitted on it,
the second type slot existed because of it, and the smartblock kind was
derived from it — through the document's own chain only, since `Validate` has
no vocabulary (§13) and had to reach the same verdict. That made one field
answer two unrelated questions (*which type does this object have*, *what
kind of object is it*), and holding the two answers together took a
reservation in five places: two export refusals, an import guard, a private
document-only resolver, and a duplicate of the §3 chain inside `Validate`.

All five delete. What the change BUYS, beyond the deletion: a template whose
`object_types` do not begin with the template key — a shape nothing in the
model forbids — could not express its target type at all. The second slot
existed only when `object_types[0]` was the template key, so
`Template + [ot-task, ot-extra]` exported as `{"kind": "template", "type":
"task"}` and the target was dropped with a warning; `snapshotdiff` agreed the
drop was by design, so the loss was invisible on both sides at once. It now
round-trips whole. `kind: "template"` also licenses `template_for`, which it
did not: a document stating its kind was refused the field the kind is for.

Costs, stated plainly. A template document now always spells `kind`, ~21
bytes. A Page whose type IS the Template type keeps one type slot rather than
two, and the drop is warned. `snapshotdiff.Compare` takes an `sbType`
parameter, because how many type slots the envelope had is no longer
answerable from a snapshot. And the pre-v0.22 spelling — `{"type":
"template"}` with no `kind` — is **refused**, not migrated (§10), naming the
member and the repair. That refusal is what makes the change shippable: a
template exported yesterday whose target type happened to be absent carries
nothing to trip the `template_for` gate, so without it the document would
import as an ordinary page and nothing anywhere would say so. It is
deletable at the version bump.

(3) **§15 carries the evidence, not just the verdict** (§15). The rejected
legend spellings were recorded as conclusions — "no separator survives
arbitrary option names", "option names may legally begin with `@`" — which is
exactly the form in which a rejected design comes back. Each now cites what
falsifies it: `bundle.ApiSlug("C#") == "c#"` and `ApiSlug("#1 priority") ==
"#1_priority"` for the separator, `ApiSlug("@home") == "@home"` plus
`Validate`'s resolver-less signature for the sigil, and — the one that was
believed to be a format-only change and is not — `model.RelationOption`
having **no key field**, so `ListRelationOptions` cannot supply the stored
keys the `{name, id}` pair shape's byte argument rested on. Two proposals
about the §3 chain's store step are recorded as killed for the first time:
deleting it (`bundle.RelationKeysByApiFold("Severity") == []`, so an agent
reading a space's property listing would silently mint a duplicate relation)
and making it mandatory (`bundle.TypeKeysByApiFold("Task") == [task]`, so a
space holding a live stored type key `Task` — which this format creates —
would be silently retyped to the bundled Task type). §15 also records what
v0.22 settled together with the cheaper option each change declined, and two
§13 typos are fixed: a `(§2b)` citation to a section that does not exist, and
a stray `}` closing the index code fence.

Changes in v0.21 (superseded by v0.22): **every key slot admits before it writes, and every
fragment carries what its keys mean** (§3, §6, §12, §13).
(1) The two key legends now admit an entry before recording it. `property_keys`
and `type_keys` were the last key slots in the format with no admission on the
way in: the two guards that were supposed to cover them — a denied key never
takes a slug, an unwritable slug is never spelled — both sit on the *slug*
path, and a stored key with no slug at all skips both. A vocabulary that binds
such a key's spelling to a different stored key (which `Options.Keys` accepts
from anyone, and which a **deleted** relation or type produces in ordinary
data) then made the identity entry owed, and the entry was one the schema
refuses: `Marshal` emitted a legend its own `Validate` and `Unmarshal` reject,
so the whole object was unexportable and nothing said so. Confirmed on a
140-character stored key and on one carrying a newline, in both namespaces, and
on an internal key at the deny rule. The entry is now dropped with a warning;
the term is written verbatim either way, so nothing the document carried is
lost — only portability for that one key, which had no writable spelling in
this format under any rule.

(2) **The key legends are exhaustive**: an entry is written for every spelling
the **bundled table does not BIND to the key being written**, identity entries
included — where the rule used to ask whether that table *inverts* the term. A
table that does not know a term answers the term itself (chain step 4), so
every custom key written verbatim "inverted" trivially and owed nothing, and
the document said nothing about the one population no reader can resolve
without it. The key is unambiguous the day it is written; it re-points later,
when the relation is deleted and the freed spelling becomes another relation's
api key — and the writer, who had nothing to warn about at the time, is the
only party who can close it. The vocabulary half of the condition stays an
inversion: asking one table instead of two was measured, and it drops
`{"task": "task"}` (a template re-points at an unrelated type) and
`{"due_date": "dueDate"}` (a bundled value lands on a custom relation). Cost:
+93 bytes on each of the four goldens, ~2%.

"Every spelling" means every KEY SLOT — the twelve property-key slots and the
four type-key slots §3 enumerates. A dataview's `source` is not one of them: it
carries stored type keys (`ot-initiative`) verbatim in both directions, with no
slug and no legend line, in fragments and whole documents alike. That is
pre-existing and symmetric, so it costs no round trip, but it is the one place
a non-bundled key travels outside the legends, and a reader that cannot resolve
it gets no help from the document.

(3) **A key slot has to name something, at all sixteen slots and through all
three doors** (§3, §6, §12). Three slots enforced it; thirteen took an empty
spelling from a plain document — no vocabulary needed — and silently lost the
slot on the way back out, `"type": ""` costing an object its type. The schema
now carries `minLength: 1` at every key slot and `required: ["property"]` on a
dataview filter (the sort and the column beside it always had it), export
**drops** a nameless filter and a nameless `property` block as it already
dropped the nameless sort and column, and the import seam refuses a vocabulary
resolving a non-empty spelling onto the empty key at the nine slots that used
to store it. The rule bounds nothing else: length and charset at these slots
stay as §3 already argued them, because bounding them would make a stored key
unexportable.

(4) **The fragment surface carries what its pieces mean** (§13.1, new). The
entry points that read and write a piece of a document — one block, a flat
run, one property value, a filter tree, a type's property array — ran the §3
chain from step 2 in both directions: export computed all three legends and
discarded them at the return, import started from an empty one. A block cut
out of a document that said `{"priority": "6a32d485…"}` resolved `priority`
through the READER's table, at the one seam that writes to a live object.
`MarshalBlockSubtree` now returns a fragment ENVELOPE —
`{property_keys, type_keys, option_ids, blocks}` — rather than a bare array,
`MarshalPropertyValue` returns its key's option ids beside the value, and
every reading entry point takes them back through **`Options.Legend`**.
`OmitIds` and the compaction flags are **refused** on a fragment rather than
honoured: both take away the addresses the surface exists to use.

§13 is also corrected. It cited `Options.Keys` / `KeyVocabulary` seven times
without defining either, omitted the whole fragment surface, claimed "the §8
inline codec is internal to the package — it is not part of the public API"
while `ParseInlineText` / `RenderInlineText` are exported, listed neither
`fragment.go`, `filters.go`, `keyvocab.go`, `blockvocab.go`, `viewvocab.go`,
`index.go`, `storeresolver/` nor `snapshotdiff/`, and gave
`PropertyDefinition` without `Options` or `ObjectTypes`, both of which §2a
depends on.

Changes in v0.20 (superseded by v0.21): **three legends, and one compaction that carries none**
(§2, §3, §9, §9a, §11, §12, §13). The envelope's indirection is now exactly
`property_keys`, `type_keys` and `option_ids`; **`refs` is deleted**, both
populations with it. (1) A select value's id moves out of the flat `refs`
map into **`option_ids`**, nested `{property spelling: {option name: option
id}}`. The `#` grammar goes with it — the split rule, the two charsets, the
qualified-key admission rule, and the property census as a *parse* — because
no separator can join a name to its scope when both halves are arbitrary
user text: `strcase.ToSnake("C#")` is `c#`, so an option of a property named
`C#` had no representable entry at all, and re-opening that hole after the
freeze would cost a version (§10). The census survives only as a key-set
comparison behind a warning (§12). (2) The legend is written
**unconditionally** — the condition could only ever be evaluated at export
time, and the rename it guards against happens in the gap between export and
read — and `OmitIds` now **drops** it, since an id-less shape that ships ids
was not one. (3) **Object-reference compaction is deleted outright**:
`CompactObjectRefs`, the object-id legend, and the resolution step that read
it. Two independent measurements retired it — API v2 removed the same legend
from its read shape after measuring a net token LOSS per document and
finding that the indirection trapped write-back of object-valued properties,
and the freeze review measured a 200-item collection growing **32.7%** under
compaction. Object references are written in full on every shape. What is
left is the rule worth stating: **the compaction that survives is the
legend-less one** — `CompactBlockLabels` relabels ids the document itself
defines, so there is no table to carry, keep in sync, or read back. (4) Two
legends, two rules, deliberately: a `property_keys`/`type_keys` value is
**authoritative** and unchecked (the corpse case requires it), an
`option_ids` value is a **liveness-checked hint** (§3). (5) The consequences
of (2) and (3) are now stated where a reader meets them: what `OmitIds` gives
up by dropping the legend, and why export does not warn about it (§9); why a
malformed legend entry is an error where an unconsulted one is a warning
(§12); and — since the grammar changed without moving `version` — that a
document carrying `refs` is refused **by name**, at `/refs`, with the rule
that replaced the legend and the repair to make (§10, §12).

Changes in v0.19 (superseded by v0.20): **a qualified `refs` key has to name a property the
document uses** (§3, §9a, §12). The key shape admits any `<name>#<spelling>`
whose halves are writable strings — it has to, because both directions must
sort a key into its population knowing nothing but the key — but the right
half is a *property spelling*, and a spelling the document never writes
qualifies nothing: import builds its lookup key from the spelling the slot it
is resolving wrote, so such an entry can never be consulted. The document's
**property census** — every position where a spelling can appear, from a
`properties` member to a nested filter's `property` — is now a layer on top of
the shape check. Import honours an entry only when its property half is in the
census (defence in depth: export writes the spelling it just used, so its keys
are in the census by construction), and Validate **warns** about one that is
not. A warning, not an error, because §9a freezes the opposite rule for the
other population — an unused `refs` entry is ignored — but `"High#priorty"`
validating clean and then silently degrading to name resolution is exactly the
kind of silence this format reports everywhere else.

Changes in v0.18 (superseded by v0.20): **a select value says which option it means** (§3, §9a,
§11). Option values are spelled by NAME because a bundle carries no option
objects, and a live account showed what the name alone costs: a space may
hold two options with one name under one relation, and resolution answers the
first — 7 objects of a 34 339-object sweep came back on an option they were
never on; and an option renamed between export and import resolves to nothing,
so the wiring mints a new option carrying the stale name and orphans the
object from the renamed one. The id now travels beside the name, in the `refs`
legend the format already has, under a key that qualifies the name with the
property that owns it (`"High#priority"`). Four things follow. (1) `refs`
holds **two disjoint key populations**, told apart by the separator alone: a
compaction label is reachable only from an object-id slot, a qualified key
only from a select value under that property, and neither from the other's
slot. (2) The entry is a **hint checked against the target space**, not an
address — an id that is not a live option of that relation there falls through
to name resolution, which is what keeps a bundle carried elsewhere working
exactly as it does now. (3) It is written wherever export substitutes a name
for an id — property values, filter values, custom orders — and behind no
option, because identity is not compaction; a document with nothing compacted
still carries it. (4) The `refs` key charset relaxes to admit both shapes: the
old `[A-Za-z0-9_-]{1,64}` rejected `import issue`, an ordinary tag name, and
each half of a qualified key now carries the writable-key rule instead
(1–128 characters, no control characters).

Changes in v0.17: **the legend answers to the reader that actually reads
the document, and three things that were said but not done** (§3, §6.2,
§11.1). (1) A term owes a legend entry when the **vocabulary in force** would
bind it to a stored key other than the one being written, not only when the
bundled table would. Export asked the bundled table alone, so a conforming
vocabulary — the one a space grows by deleting a type or a property, which
vacates the slug namespace while the objects keep the stored key — re-pointed
an object's type in silence, and the property namespace made `Marshal` emit a
document its own `Unmarshal` refuses (two spellings addressing one property,
an I1 break). The entry is authoritative for every reader, which is what a
legend is for. (2) `KeyVocabulary` gains the third precondition
`storeresolver` has always implemented and the interface never stated: a live
stored key outranks the vocabulary's own slug binding (§11.1). (3) §3 promises
every dropped object type is reported; the **positional** drop — a keyed
entry past the one type the envelope models — was silent, so a user's second
type left the archive with nothing said. (4) The counting-preset rule reads
the **operand**, not the member: `"value": null`, a string or a list all
count as 0 days, which is the trap the message describes, and the day count
carries the compact grammar's `[0, 36500]` bound. Export writes the count the
engine reads for a stored operand that is not one, with a warning, so the
tightened rule cannot make an object unexportable. (5) The §2a array's
declared **format** now resolves the same way through both doors: the
PATCH-types channel read `text` literally while the document path resolved it
per key (§3), so the bundled `name` property was created as `longtext`
through one endpoint and stayed `shorttext` through the other. (6) Two
warnings addressed the wrong place (§13): a dropped property-definition entry
pointed at the index the next surviving entry takes, and the template-spelling
guard said `/type` wherever it fired, including from
`…/property_definitions/N/object_types/M`, a field the pointer does not even name.

Changes in v0.16: **the date-preset rules read the same gate the query
engine does, and one fault stays one issue** (§6.2, §12). (1) A preset is
applied only on a `date` filter — `transformDateFilter` returns a filter of
any other format before it computes a range — and the counting-preset operand
rule checked the condition half of that gate and not the format half, so
`status greater number_of_days_ago` was refused for a count nothing would
have read. The gate now resolves the format the way import does: the
dataview's `properties` list first, `bundle` on the resolved stored key
second. (2) A preset under a condition it does not apply to is a **warning**
now rather than nothing, and deliberately not an error: export writes the
pairing because stored filters carry it, so refusing it would make an
unexportable object out of every one that has one. (3) Issue paths are JSON
pointers and a segment taken from the document is escaped as one (RFC 6901).
Joining the raw tokens addressed the wrong place for a key holding `/` or
`~`, and — because the suppression of the schema's second opinion is keyed by
pointer — reported one empty legend value three times.

Changes in v0.15: **the round-trip invariants, held where they were only
stated** (§3, §11). (1) §11's "equivalent resolvers" precondition is written
out, because it is stronger than `KeyVocabulary` said: a vocabulary may not
bind a spelling the bundled table binds to a different key. One that does can
still be a strict inverse pair, and it turns a template for the bundled
`task` type into a template for an unrelated custom type — the legend cannot
help, because a spelling the bundled table inverts is written with no legend
entry at all. No shipped vocabulary can do it; `Options.Keys` takes one from
anyone. (2) §11 gains the snapshot-anchored guarantee, `Export(S) =
Export(Import(Export(S)))`, and §3 the census rule that makes it true: the
term census reserves the keys the document SPELLS, not every key the snapshot
holds. Reserving more backed a real slug off, so one object exported before
and after a round trip produced two documents. (3) A property key slot carries
the writable-key rule wherever it is, including `property_definitions[].key`,
which is a JSON string value the schema could only bound at `minLength: 1`:
a 140-character key validated clean and then failed to import. Export drops
such an entry now rather than emit one the seam refuses. (4) §3's argument
for having no type deny rule no longer rests on "export strips no object
types", which is false — export truncates to the positions §2 models. What it
strips is positional, never a particular key, which is what the derivation
needs. (5) Two documentation corrections in the same family: §1's Naming
section still described the pre-vocabulary rule (property keys "written
exactly as stored: `iconEmoji`, `dueDate`") and §3's "what is not a key slot"
paragraph still named the format's own fields in their pre-snake_case
spellings (`kind: "objectType"`, `defaultTemplateId`, a callout's
`iconEmoji`), which the schema has not used since v0.8. The §2a type-document
example spelled `iconEmoji` in `properties` two lines above prose calling it
`icon_emoji`.

Changes in v0.14: **the type namespace's own rules, made true and made
two-sided** (§3). Four corrections, one clarification, one recorded
asymmetry. (1) Export dropped a keyless object type in silence AND took its
siblings with it — a stored `ot-` has no spelling, so the slot it landed in
went unwritten, and an unwritten `type` slot makes `template_for`
inexpressible: `["ot-", "ot-task"]` exported as no types at all. Keyless
entries are now dropped with a warning and the survivors close ranks. (2)
The type legend recorded a line for every object type, written or not,
publishing a space's slug→key mapping for a type the document never spells;
only written slots claim a term now, matching the property side, which
filters before it slugs. (3) The `template` spelling reservation was
one-sided: export refused a vocabulary answer that moved it, import took
one, so a Template smartblock could come back with no `template` in its
object type keys — invisible to every template check. The reservation binds
the vocabulary on both sides now; the document's own legend still moves the
spelling, because the kind derivation moves with it. (4)
`BuildRecommendedLists`, the PATCH channel for `property_definitions`, refused
nothing, so a vocabulary bug wrote the empty key into a type's recommended
lists; it now refuses what the document path refuses, and returns an error.
(5) §3's justification for having no type deny rule was false as written —
merge resolution is steerable, through `name`, `relation_key`,
`relation_format` and `source_object`, all writable properties. The
conclusion stands on a better argument: every effect a document-chosen type
key can produce is separately and more directly writable through the
property namespace. Those properties stay writable — export must stay
lossless — and the guarantee against rewriting an existing object's identity
belongs at the object layer. (6) The type namespace has no duplicate-binding
refusal, deliberately: two type entries duplicate in an ordered list, where
two property spellings would collapse and lose a value. §3 now records it.

Changes in v0.13: **the type namespace gets the property treatment** (§2,
§2a, §3, §12): `type_keys`, the envelope legend that makes type spellings
invertible from the document alone. The type key slots — envelope
`type`/`template_for` and `property_definitions[].object_types` — were slugged on
the way out with nothing to invert them: a node-backed vocabulary slugging a
custom type `69bbfc…` to `task` exported `"type": "task"`, and a package-only
reader bound it to the bundled Task type — a different type, silently. The
slots now run the §3 chain against a namespace of their own: a separate term
ledger and a separate legend (a property slug and a type slug may share a
spelling by design), identity entries under the same trigger (`object_type`
the stored key beside bundled `objectType`), and slugs claimed through the
same census-and-back-off discipline. The `Options.typeSlugs`/`typeKeys` list
helpers — the last key slots reachable without the ledger — are gone, like
their property twins before them. Two rules are the namespace's own: there
is **no deny rule** (no type KEY is denied — what export drops is positional;
see v0.14 for the argument that replaced the "a type key is not a resolution
vector" one originally given here), and the reserved spelling is
**`template`** — export
refuses to move it in either direction, and the template gate and the kind
derivation run on the stored key the spelling resolves to through the
document's own chain, in validation and import alike (§12). The seam refuses
a term resolving to the empty type key, the one silent-loss resolution this
namespace has.

Changes in v0.12: **verbatim-first, made implementable offline** (§3, §12).
Chain step 2 — an exact stored key is always its own address, and the
bundled table applies only to terms that are not stored keys — was
implemented by the node-backed resolver but merely narrated to the package
half, which resolved table-first: `{"properties": {"due_date": …}}` meant
one relation to a node-backed reader and a different one to a package-only
reader, silently. §3 now states the resolution chain **once**, and export
carries the mechanism a storeless reader needs to run it: an **identity
entry** (`{"due_date": "due_date"}`) wherever a stored key's verbatim
spelling would otherwise fall through to the bundled table and land on a
different key — which also restores §11 for a stored key spelled like an
internal slug (`unique_key`), whose export used to fail its own validation.
Export claims every spelling through one document-wide **term ledger** (a
stored key always keeps its own term; a slug goes to its first claimant), so
a block slot can no longer record a legend entry that rebinds a term
`/properties` owns — which moved that property's value onto a different
relation — nor collapse two keys into one slug. The legend can no longer
launder: a `property_keys` value is admitted like the stored key it is —
the deny rule runs on the value itself, member or no member spelling it —
and export never mints the refused shapes, because a denied key never takes
a slug and a vocabulary answer of `id`/`type` (the spellings refused before
any resolution) falls back to the stored key like an over-long slug always
did. And validation now mirrors the importer's details seam refusal for
refusal — denied resolved key, **unwritable** resolved key (new at the seam
too, closing the vocabulary that resolved a spelling onto `""`), and two
spellings binding onto one stored key — so `Validate` and `Unmarshal`
accept and reject the same documents under default `Options` (§12).

Changes in v0.11: three rules that two surfaces disagreed about, each
found as `Marshal` emitting a document its own `Validate` rejects. **A table
owns its whole grid of derived cell ids** (§4, §6.1) — validation always
claimed every `<rowId>-<colId>` pair, written or not, but export reserved only
the cells that exist as blocks, so a snapshot with a paragraph named `r1-c1`
beside a table with row `r1` and column `c1` exported unimportable; the plain
block is now the side that yields, since a derived id has no spelling of its
own. **The `property_keys` legend covers every key slot** (§3), not the ones
that happened to route through the recording step: a link block's
`properties` and a `property` block's `key` wrote space-slugged spellings
with no legend entry, and the link's reader ignored the legend even when the
entry was there. **A counting date preset needs its day count only where the
preset's range is applied** (§6.2) — `transformDateFilter` substitutes the
range for six conditions and leaves every other filter unchanged, so on the
`empty`/`not_empty`/`exists` leaves whose `value` export drops, the count is
never read and demanding it asked export for a field it must not write.
Also: a generated row/column id that needs no sanitizing keeps the name the
generator gave it (§6.1) instead of collecting a `_2` from its own claim; and
the key-slot rules the schema states as `propertyNames` are **restated in the
reader**, so an unwritable property key, legend spelling, legend stored key or
`refs` label is reported against the member that carries it instead of against
the document root (§12). Which documents are accepted does not change.

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
resolved key for vocabularies wider than Validate can see, a legend value
obeys the writable-key rule (schema-enforced), and export checks the
writability of the *slug* it emits — not just the stored key — falling back
to the stored spelling when a vocabulary misbehaves, so `Marshal` cannot be
talked into output its own `Validate` rejects.

Changes in v0.9: the key vocabulary of §3 arrives from the API v2 branch —
types and properties are named by their snake_case api slug, inverted through a
table in both directions, never a case transform — and with it
**`property_keys`**, the envelope legend that makes the slug layer invertible
from the document alone (§3). Without it, a slug derived from a space's stored
key reads back as a *different* relation in any reader that cannot ask that
space: a 36 808-object sweep found 12 objects re-pointed exactly that way, and
the reader-side half of the same defect — the accept side bound a spelling the
emit side refuses to write — is fixed with it. That fix is demonstrated by
unit test; those 12 objects have not been swept again since it landed.

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
`property_definitions` array replacing the four recommended-relation id lists; a
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

### The four consumers

The four goals above are not four independent wishes. Each is claimed hardest
by one of four consumers, and the consumer that claims a goal hardest is the
one that sets its bar. Naming the four makes the rest of this document
arguable: several rules below look arbitrary from one row of this table and
forced from another.

| | Consumer | May assume | Owes |
|---|---|---|---|
| **1** | **Export and import** — backup, migration, round trip: `Marshal`/`Unmarshal` (§13), an archive or a bundle on disk (§2c) | a live space at both ends, and that the bytes it reads are bytes it wrote | goal 4 (lossless up to §11, canonical output) and goal 1 |
| **2** | **Authored documents** — an agent, a script or a person writing objects, types and whole bundles for a space that may not exist yet | nothing but the document, the schema, and the bundled key table every reader ships | goal 2, and the offline half of goal 3 |
| **3** | **The API over the format** — API v2 (`core/api/APIV2.md`): explicit operations against a live space | a space, a store, and the resolvers of §13 | the store-backed half of goal 3 — resolve, refuse or create, and say which |
| **4** | **Tool wrappers over that API** — the task-shaped tool set models drive instead of the raw surface, delivered as CLI verbs and as an on-device manifest (`APIV2.md` §7) | everything layer 3 guarantees | nothing this format defines: a context budget |

**Layer 1 sets the readability bar, and every other layer inherits it.** Its
reader is the one that can ask nothing — a file open in an editor, a bundle on
a disk, a space that no longer exists. So identity that cannot be spelled
readably is not demoted to an id: the label stays in place and the map goes in
the envelope — `property_keys`, `type_keys` and `option_ids` (§3). *Naming* above records that move being made once already: the
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
  a did-you-mean error instead, and the tool wrapper is stricter still
  (`APIV2_REVIEW_SMALLMODEL.md` A2, `APIV2.md` §7.2). Same document, opposite
  defaults, chosen by the caller. Record APIs that meet this question elsewhere
  put the switch in the same place: on the write request, off by default, never
  in the record being written.

**Failure is loud at every layer** (`PRINCIPLES.md` rule 8, *Strict in, canonical out*): version refusal
with no partial read (§10), one unsupported file and a whole bundle declines to
install (§2c), path-addressed import errors (§12), a warning for an `option_ids` entry
nothing can consult (§9a), and layer 3 naming the candidates rather than
choosing one. The two places the line bends are both documented as such and
both compensated: name resolution answers the FIRST when two options of one
property share a name, which is the whole reason the document carries the id
beside the name (§3); and a widget target that resolves to nothing is dropped
by a path outside this package with no diagnostic at all, which is why the
tooling refuses it before install rather than leaving it to be discovered (§2c).

**Layer 4 subtracts — but it has already added.** A tool set hides what a model
should not spend context on, ids first of all: the wrapper resolves enumerated
handles server-side so the model never emits a CID (`APIV2.md` §7.1). What it
may not do is invent a dialect. It would be false, though, to say the format
owes it nothing. Block ids relabel to short
suffixes because a 59-character CID costs more tokens than the sentence around
it, and they can do so safely only because that relabeling carries no legend to
trap a write-back (§9a); `OmitIds` exists for templates and
prompt examples (§9); `blocks` is a flat array partly because guided decoders
cannot express a recursive schema (v0.6). Layer 4's needs shape this format —
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

This follows the two vocabularies §1 claims lineage from — Notion's API
(`bulleted_list_item`, `heading_1`) and Anytype's public API
(`background_color`, `added_at`) — and, more to the point, it is the spelling a
generating model produces unprompted: the format's own pre-freeze review
records an LLM writing `"type": "bulleted_list_item"` against a camelCase
draft, which the reader then had to reject.

**Two kinds of string are exempt, both because they name something outside the
format.** They are not inconsistencies to be tidied away later:

- **Property and type keys** (§3) name relations and types, which live in a
  space rather than in this format. Their canonical spelling is their
  **label**, which is snake_case by construction — a bundled key's api slug
  (`plural_name`, `due_date`, `last_modified_date`), or a space-minted key's
  stored slug or display name normalized through the identifier grammar of
  §6.2.1 (`publish_date`, and `тоггл`, which is snake_case in a script that
  has no case) — so most of the time the exemption does not show. It shows
  where no label can be derived at all: a legacy `wikiPerson` whose name
  normalizes back onto its own key, a minted `6a32d4856761631534b22f85`
  whose relation has no name and no slug. Those are written verbatim,
  whatever their shape, because an exact stored key is always its own
  address (§3).
  This section once said the key ↔ key mapping was impossible because
  `Validate` takes no resolver; the answer was to put the mapping in the
  DOCUMENT — the `property_keys` / `type_keys` legends, which a reader with no
  space at all can invert.
- **Platform identifiers** — the `dataview` block id (§7) and the `objectId`
  parameter of the `anytype://object` deep link (§8.1) — name things that
  exist in a live space. They are quoted, not translated.

### The `_` namespace

**A value beginning with `_` addresses the platform, and nothing a document or
a bundle mints may begin with one.** The platform's own addresses already live
there — `_otpage`, `_brdue_date`, `_missing_object`, `_participant_…`,
`_date_2024-01-01` — and the format borrows the same namespace for the
built-in screens and listings an `index.json` may name: `_favorite`,
`_recent`, `_set`, `_collection`, `_all_objects`, `_recent_open`, `_widgets`,
`_graph` (§2c).

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
why the two listings that never reach the wire are spelled `_all_objects` and
`_recent_open` rather than quoting the live space's `allObjects` /
`recentOpen`: nothing quotes them, because nothing emits them (see §2c).

The prefix is translated away at the wire boundary, where the importer's own
spellings are bare (`favorite`, `graph`), by `WireWidgetTarget` /
`WireHomepage`. Writing `_set` into a link block would be strictly worse than
the shadowing it replaces — an unrecognised target becomes
`addr.MissingObject` and the widget is then stripped without an error.

**Which is why a bundle-local id may not be one of those bare spellings
either** — `favorite`, `recent`, `set`, `collection`, `widgets`, `graph`. The
prefix rule alone would only move the collision one step downstream: the link
block that leaves this format saying `_set` reaches the importer saying `set`,
and `handleLinkBlock` still resolves through the bundle's ids first. The
prefix is what makes the *format* unambiguous — a reader never has to guess
which of the two kinds a target meant, and a typo inside the namespace can be
refused by name. The six-word ban is what makes the *wire* unambiguous. Both,
or neither is worth having.

(The JSON Schema's own `$defs` names — `blockCore`, `tableCell`, … — are
neither: they are schema-internal labels a document never contains, and they
keep JSON Schema's conventional camelCase.)

So `{"type": "callout", "icon": {"format": "emoji", "emoji": "💡"}}` and
`{"properties": {"icon_emoji": "☕"}}` are both correct in the same document,
and they are not the same thing spelled twice: the first is a field this
format defines, the second is a key belonging to the data — and after v0.25
it can only be a SPACE-MINTED relation that happens to be stored under that
name, because the bundled `iconEmoji` is refused there (§2b). 54 production
objects hold exactly that pair. `{"properties": {"wikiPerson": …}}` is where
the two part company.

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
| `kind` | string | no | System-level object kind, snake_case (`page`, `profile_page`, `template`, `archive`, `widget`, `chat`, …) — from `model.SmartBlockType`. `chat` is `ChatDerivedObject`: a standalone chat object whose identity is `key`, like a type's; its messages live in the CRDT store, not in snapshots, so it always imports empty. (`chat_object` is the deprecated predecessor; `discussion` is a hidden type.) **Omitted whenever derivable**: absent means `page`. It is the SOLE authority on whether a document is a template — `template_for` is admitted on it, the second type slot exists on it, and no type spelling implies it (§3). A template therefore always spells its kind. An unrecognized value is a validation error listing the allowed values. |
| `id` | string | no | Object id. Written by export; import treats it as informational (a new id is minted on import) except for resolving intra-export links. Written in full, like every object reference — object references are never compacted (§9a). |
| `type` | string | no | The object's type **slug** (`page`, `task`, `object_type`…) — the key vocabulary of §3, not the stored `ot-`-prefixed key. Maps to `object_types[0]` in the snapshot. Absent when the snapshot has no object types (legacy/system objects). Import inverts the term through the §3 chain in the type namespace — the document's own `type_keys` legend first, then the vocabulary in force (bundled table offline, the space's stored slugs inside a node) — and hands the resulting stored key to the wiring, which resolves it — matching an existing type or creating one (the Markdown importer's behavior). A term the chain does not know passes through verbatim — an exact stored key is always its own address (§3). No spelling is reserved: `template` is an ordinary type term that a legend or a vocabulary may bind wherever it likes, because `kind` — a field no chain touches — carries the template semantics it used to carry (v0.22). The one exception is a byte comparison, not a resolution: a document with **no `kind`** whose `type` is literally `template` is the pre-v0.22 spelling of a template and is refused, naming the repair (§10). |
| `template_for` | string | no | Only for templates: the target type slug (`object_types[1]`), same vocabulary and legend as `type`. Admitted on `kind: "template"` and nothing else — present without it, or without a `type` beside it to be `object_types[0]`, is a validation error. Note what this is NOT keyed off: the template's own type. A template whose `object_types` do not begin with the template key is a shape the model permits, and until v0.22 it could not express its target at all — the second slot existed only when `object_types[0]` was the template key, so the target was dropped with a warning and no way to keep it. |
| `key` | string | no | Identity key of *system* objects (types, properties). This is the STORED identity key (a `uniqueKey`'s internal part), written verbatim: unlike every key slot in §3 it is **not** translated, so for an object whose stored key is a minted BSON it does not match the slug the public API serves as that object's `key`. Because it is verbatim, its charset is whatever the store already holds: a relation option's key is built from the option's *name*, so `completion_status_Not Started`, `…_C/C++` and `…_тогглы` are all real stored keys. The rule is therefore a deny rule — non-empty, no control characters, at most 255 characters — not an allowlist. An allowlist was tried and falsified: it failed 59 objects of a 36 808-object account, every one a relation option. Never emitted for ordinary documents. |
| `relation_settings` | object | on `kind: "relation"` | Only for relation documents, where it is **required**: the definition of the property this document IS — one `propertyDefinition` (§2d, §2e). Carries `format` (required, a §3 format NAME — never a raw enum number; stands for the stored `relationFormat` key, which `properties` refuses), `include_time` and `object_types`, each present exactly when its stored key is, value included. Illegal on every other kind. |
| `icon` | object | no | The object's icon — ONE object whose `format` selects the variant (§2b). Stands for the stored `iconEmoji` / `iconImage` / `iconName` / `iconOption` keys, which `properties` refuses. |
| `cover` | object | no | The object's cover — same shape, three variants (§2b). Stands for the stored `coverId` / `coverType` / `coverScale` / `coverX` / `coverY` keys, which `properties` refuses. |
| `properties` | object | no | The object's properties, §3. |
| `type_settings` | object | no | Only for type documents (`kind: "object_type"`, `"bundled_object_type"`): everything that defines the TYPE, in one gated subtree — `layout`, `api_key`, `plural_name`, `default_template`, `default_view`, and `property_definitions` (§2a). Present on any other kind → validation error. Until v0.32 the property list sat at the root as `type_properties`; that spelling is refused with the repair named. |
| `property_keys` | object | no | Legend: the stored property key each spelling in this document names (§3). Written for every spelling the **bundled table does not bind to the key being written** — a slug the table cannot invert (a space's own key) *and* the **identity entry**, which is the ordinary case: a custom key written verbatim names itself, because nothing else in the document says the term is a stored key rather than somebody's slug. A reader consults it **before** its own vocabulary and takes the value as **authoritative**: it is not liveness-checked, deliberately (§3). Absent only from a document whose every spelling is bundled. |
| `type_keys` | object | no | Legend: the stored type key each type slug in this document names — `property_keys`' twin on the TYPE namespace, written and consulted under the same rule (§3). A separate map, deliberately: a space may slug a relation and a type onto one term, so one map could not carry both meanings of a shared spelling. |
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
  "version": 1,
  "kind": "object_type",
  "key": "task",
  "icon": { "format": "icon", "name": "hammer", "color": "orange" },
  "properties": { "name": "Task", "description": "…" },
  "type_settings": {
    "layout": "todo",
    "api_key": "task",
    "plural_name": "Tasks",
    "default_template": "bafyrei…",
    "default_view": "table",
    "property_definitions": [
      { "key": "due_date",  "name": "Due date", "format": "date",    "section": "featured" },
      { "key": "assignee", "name": "Assignee", "format": "objects", "section": "featured" },
      { "key": "status",   "name": "Status",   "format": "select",
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
| `api_key` | `apiObjectKey` | the type's public API key. **`api_key`, not `slug`**: of 1,326 corpus type documents with one, the document's own label differs from it in 941 (`Space member` has api key `participant` and label `space_member`) — calling it a slug would imply it is the term used elsewhere in the document, which for 71% of types it is not. |
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
carrying an `iconName` are types. The four recommended-relation id lists
(`recommended_featured_relations`, `recommended_relations`,
`recommended_file_relations`, `recommended_hidden_relations`) are
**replaced** by `type_settings.property_definitions` — resolved entries,
never raw relation ids. The array lived at the document root as
`type_properties` until v0.32; the word is `property_definitions` rather
than `properties` because the document already uses that word for property
VALUES at the root, and one word carrying two meanings in one file is the
same shape as the `featured_relations` collision below — one word per
concept.

**A type document does not carry its own install provenance.** Seven stored
keys are omitted on export and dropped on import (stale, not wrong — the
transient-key policy, scoped by kind), each admitted to the drop
individually against 1,760 corpus type documents (§15 #12; the verdicts
live on `typeProvenanceKeys`, and §11 N(S) records the normalization):
`layout` and `resolved_layout` (ONE distinct value each — "object_type" —
derivable from the kind), `smartblock_types` (occurs only on installed
copies of bundled types, restating the bundled table), `source_object`
(derivable from the type key: `_ot<key>`), `origin` (how the INSTALL
happened — on ordinary objects origin is real provenance and stays),
`added_date` (epoch-zero on 1,600 of 1,627), and `set_of` — which is the
type document's **own id** on 1,756 of 1,757, re-stamped by
`WithForcedDetail` from the object's id on every init, so it is a function
of the id rather than a fact about the type. (An earlier draft recorded
`set_of` as "1,757 targets, none resolving" — a measurement that compared
raw values against bare ids while the corpus dump carried `#name`
suffixes, so every comparison missed. The verdict was right; the evidence
for it was not, and §15 #12 is an evidence discipline.)

Six candidates FAILED the admission test and stay in `properties`:
`is_hidden` (cannot be proven install-only), `order_id` (the user's own
ordering of types), `layout_width`/`layout_align` (the type object's own
page display, set by a person where non-zero), `featured_relations` —
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
| `key` | string | **yes** | Property key, in the document's own spelling — a key slot like any other, inverted through `property_keys` (§3). |
| `name` | string | no | Display name. Import uses it only when the property must be **created**; an existing property keeps its own name. Every bundled key already exists, so a name given for one is inert — `{"key": "description", "name": "Summary"}` renders as *Description*. Validation warns. If the label is the point, mint a custom key instead of reusing a bundled one. |
| `format` | string | no | Property format (§3 names). Same import rule as `name`; a conflict with an existing property's format is an error at the wiring level (the package cannot see the space). |
| `options` | (string \| object)[] | no | A select/multi_select property's **vocabulary, in display order**. Each entry is a bare option name, or `{"name": …, "color": …}` when the option's color is part of the design — the color belongs to the option rather than to a parallel array, so inserting or reordering an option cannot shift it. `color` is one of `grey`, `yellow`, `orange`, `red`, `pink`, `purple`, `blue`, `ice`, `teal`, `lime` (`util/constant`); anything else is a validation error rather than a silently ignored value. The bare string is **canonical** whenever the option declares no color, the object form otherwise — the same rule cells follow in §6.1. Leaving a color out does not mean *no* color: the wiring assigns one, cycling the palette in declaration order and skipping whatever the vocabulary claims explicitly, so a vocabulary that names no colors still gets distinct ones. (The app assigns one at random on every other creation path; cycling keeps a converted bundle identical run to run.) Options are otherwise discovered only from values that happen to be used, so a vocabulary entry no record carries would never exist — its kanban column simply absent — and a discovered option carries no `orderId`, which makes every select sort alphabetically (options order by `[orderId, name]`, `pkg/lib/database.BuildOrderMap`). Declaring them lets the wiring create each one up front with an order id. Every option needs one: the sort concatenates `orderId + name` before comparing, so an option missing an order id is compared by *name* against the others' order ids and lands arbitrarily — ahead of the whole vocabulary when its name sorts below the id alphabet, behind it otherwise. Names discovered from usage rather than declared are ordered after the declared ones. Only meaningful on `select`/`multi_select`; duplicate names are a validation error, across both forms. |
| `object_types` | string[] | no | The **type slugs** an `objects`/`files` property may point at, in priority order — a type-key slot like the envelope `type`, so it speaks the one key vocabulary (§3), claims its spellings through the same type term ledger and owes the same `type_keys` legend; import inverts each entry through the legend first, and a term the chain does not know passes through verbatim. Empty means any object — an untargeted property will happily accept a random page as a task's assignee. Listing the built-in `participant` alongside a bundle's own people type is what makes the current-user filter value usable on that property (§6.2) while still allowing the seeded people as values; the client only offers it when the relation's targets include Participant. The wiring resolves each key to an id the way it resolves properties: a type the batch defines by the id its own document carries, a bundled type by its bundled url (`_ot<key>`). Only meaningful on `objects`/`files`. |
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
properties. The canonical form writes `name` and `format` on every entry
(`format` defaults to `text` when absent on input), and writes the
`property_definitions` array **even when empty** — its presence is what tells
import to rebuild the lists. Import then rebuilds all four id lists — empty
sections become explicit empty lists, matching how type objects store them —
resolving each `key` against the space and creating missing properties (the
same policy as select option names, §3). A document without a
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
   rebound by the `property_keys` legend to point at an arbitrary relation,
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
/properties/icon_emoji: "iconEmoji" is written as "icon": {"format": "emoji",
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
  "$schema": "https://schemas.anytype.io/anyblock/1/index.schema.json",
  "version": 1,
  "name": "Company Wiki",
  "description": "Everything we know, with an owner.",
  "icon": { "format": "emoji", "emoji": "📚" },
  "homepage": "page-wiki-home",
  "widgets": [
    { "target": "page-wiki-home" },
    { "target": "type-wiki-page", "layout": "view", "limit": 6 },
    { "target": "_favorite", "layout": "compact_list" }
  ]
}
```

| Field | Meaning |
|---|---|
| `name` · `description` | the space's own identity, applied on install |
| `icon` | the space's icon, in exactly the shape an object's icon has (§2b), restricted to the two variants a bundle can hold: `{"format": "emoji", "emoji": "📚"}`, or `{"format": "file", "file": "<object id of an image in the bundle>"}`. The image variant needs the image object *and* its file in the archive, so a generated bundle uses an emoji. It is one `$ref` into the object schema, not a copy — an index and an object cannot disagree about what an icon is. Two flat keys stood here until v0.25, with no rule for which won and with the image spelled as a scalar while the object surface spelled it as a list |
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

A widget is `{ target, layout, limit }`. `layout` is `link · tree · list ·
compact_list · view`, defaulting to `link` and omitted when default (§4).
`target` is an object id from the bundle — a page, a type, a set, a
collection — or one of the reserved listings `_favorite · _recent · _set ·
_collection`, which name a built-in rather than something the bundle ships.
The leading `_` is what keeps the two kinds of target apart (§1): an object id
from the bundle may never begin with one, so a bundle cannot shadow a listing
with an object of its own, and a reader never has to guess which of the two a
target meant. Those four and no others: a live space also has an All Objects
and a Recently Opened widget, but the import path does not know those names
(`widget.IsPredefinedWidgetTargetId`), so a widget declaring one is **dropped
on install with no error** — see below. The tooling rejects them, which is
also why they are spelled `_all_objects` / `_recent_open` rather than quoted
from the live space: they exist only to be refused, so there is no live
spelling for them to preserve.

A `_`-prefixed target that is not one of the six is refused by name, with the
inventory in the message. It cannot be an object id, so the alternative
diagnostic — "no object with that id in the bundle" — would point an author
with a typo at the wrong repair.

### The manifest

The index also says **where to find what a reader must resolve by key or id
rather than by walking**. The format defines no folder layout — `objects/`,
`types/`, `relations/` are one exporter's convention — and an object names
its type by *spelling* alone, so without a manifest a reader resolves a type
by scanning every document for a matching `key`. Measured, exactly two
namespaces are addressed that way: types (22/space) and options (34/space) —
~5.1 KB per space, 0.23% of the bytes.

```json
{ "manifest": {
    "types":      { "task": "types/bafyrei….anyblock.json" },
    "options":    { "bafyrei…opt1": "relationsOptions/bafyrei….anyblock.json" },
    "properties": "properties.json" } }
```

- **`types`** — STORED type key → the type document's path. Stored keys,
  not per-document spellings, the same rule the dictionary applies (§2f): a
  document's `type_keys` legend binds its spelling to the stored key, and
  the stored key is what the manifest answers for.
- **`options`** — option object id → the relation_option document's path.
  Ids, because that is how documents address options: a value carries the
  option's NAME and the `option_ids` legend beside it carries the id (§9a) —
  the id is the spelling that survives a rename.
- **`properties`** — the property dictionary's path (§2f). A pointer rather
  than an inline map, because properties resolve by stored key through each
  document's own legend and the dictionary is the file that answers for
  stored keys.

Paths are relative to the index file. The reader flow, with no scanning and
no folder convention: object → `type: "task"` → the object's legend → stored
key → manifest → the type file → `property_definitions` → property not
there → manifest → the dictionary → the entry.

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
| `profile` at the archive root — `pb.Profile`, raw protobuf whatever format the snapshots are in, since `getProfile` reads it with `pb.Profile.Unmarshal` | `cmd/anyblockconvert` (`profile.go`) | `CreateObjectsForExperience` reads **`spaceDashboardId` only** |
| a snapshot with `sbType: Widget` among the objects — one root block plus a wrapper-and-link pair per widget | `cmd/anyblockconvert` (`widgets.go`) | the pb importer: `shouldImportSnapshot` admits a Widget snapshot precisely when the import type is `EXPERIENCE`, and `objectcreator.updateWidgetObject` merges its widgets into the space's own widget object |

| `index.json` | reaches the space as | effect |
|---|---|---|
| `homepage`, falling back to `entrypoint` | `profile.spaceDashboardId` | the space's `homepage` detail — what opens on **every** entry, and on this path the only thing that decides what a new user sees |
| `widgets` | the Widget snapshot's root children, in order | the sidebar |
| `entrypoint` | `profile.widgets[0].targetObjectId` | the object the install opens **once** — on the `inject` path only. On a bundle's own path it lands only through the `homepage` fallback above |
| `name` | `profile.name` | nothing, on this path |
| `icon` (the `file` variant) | `profile.avatar` | nothing, on this path |

Five consequences worth stating, because none is obvious from the wire format:

- **`profile.widgets` is inert here.** `CreateObjectsForExperience` never
  calls `getWidgets` or `createWidgets`; those belong to `inject`. The wiring
  still fills the field, so an archive it produces is also a valid built-in
  archive, but nothing on a bundle's own path reads it. **The sidebar comes
  from the Widget snapshot**, which is also how a real app export carries it —
  export a space and the widget object is a file under `objects/` while the
  export's own `profile` has `"widgets": []`.
- **`name` and the icon are discarded on this path.**
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
  and only an explicit `"_widgets"` or `"_graph"` gives up a real page.

**A widget target that does not resolve loses the widget, silently.** This is
the only reference in the format whose failure produces no diagnostic at all:
`common.handleLinkBlock` rewrites a link target it cannot resolve to
`addr.MissingObject`, and `WidgetObject.Init` then removes the broken link
*and* its now-empty wrapper. The import succeeds, the widget is not there, and
the only trace is a log line. That covers both an id no document in the bundle
defines and a reserved listing the importer does not recognise
(`_all_objects`, `_recent_open`). Both are therefore errors in
`anyblockvalidate` and `anyblockconvert` rather than something an author
discovers by installing.

Nothing per-object substitutes for this file. In particular **`is_favorite` is
not an entry point**: it adds an object to Favorites and nothing more. It
does not open anything, create a widget, or set the homepage.

Ids in `index.json` are the bundle's own — the same slugs every other
document uses — and the wiring relinks them like any other reference. Whether
they resolve is a cross-document question this package does not answer
(§13): an index validates on its own terms while naming an object no
document defines.

## 2d. Relation documents (`kind: "relation"`, `"bundled_relation"`)

A relation object IS a property definition, and it states what it defines in
**`relation_settings`** — one `propertyDefinition` (§2e), in the format's own
vocabulary:

```json
{
  "version": 1,
  "kind": "relation",
  "id": "bafyrei…",
  "key": "budget",
  "relation_settings": { "format": "number", "include_time": false },
  "properties": { "name": "Budget", "description": "Planned spend" }
}
```

v0.31 put the three members at the document root; v0.32 regrouped them —
churn on freshly shipped fields, accepted deliberately, because the
dictionary entry and a type's property-definition entry are groups holding
the same shape and two patterns for one idea is the §15 #14 disease one
level up. The group is a layer over `$defs/propertyDefinition`: the members
another surface already owns are refused with the home named (`key` is the
envelope's, `name` and `description` are `properties`', `options` are
relation_option documents), and `max_count`/`readonly`/`default_value` keep
travelling in `properties` under their stored keys until the dictionary
lifts them — admitting a second spelling of any of those here would
reintroduce exactly the duality this section removed.

**Two kinds are relation documents**, and both carry the group: `relation`
and `bundled_relation`. Only the first comes out of a live store — 0 of
38,061 corpus documents are the other — but the `kind` enum offers it beside
`relation` with nothing marking it non-authorable, and a small model
authoring from the schema alone picked it unprompted, walking straight back
into the bug this section exists to stop. The export gate, the import gate
and the schema's `if` therefore name the same two: a half that lifts for
fewer than the schema validates for emits a document its own Validate
rejects (§11 I1), and a half that reads back fewer drops the definition.
Both breaks happened while this section was being written, each time from
widening one list and not its siblings.

Two neighbouring kinds are deliberately outside the set. `relation_option`,
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

| stored key | `relation_settings` member | shape |
|---|---|---|
| `relationFormat` | `format` | a §3 format NAME — **required**. Export refuses to write a relation whose stored format it cannot name (corrupt data only: `formatNames` is total over the model enum, test-pinned), because the fallback — writing `"text"` for a format that is not text — would import as a permanent silent format rewrite, the exact disease this lift kills. `"text"` resolves per key on the way back in, through the envelope `key`, exactly as a property-definition entry's format does (§3): a bundled short-text relation keeps its stored format across a round trip. |
| `relationFormatIncludeTime` | `include_time` | `true` \| `false` \| `null`. Meaningful on `date` only; a `true` against any other format is a **warning**, carried unread. |
| `relationFormatObjectTypes` | `object_types` | the target **type keys**, in priority order — a type-key slot exactly like `property_definitions[].object_types` (§2a): the §3 type vocabulary, the same term ledger, the same `type_keys` legend. Non-empty against a format other than `objects`/`files` is a **warning**. Meaningful entries: `[]` is a cleared target set, `null` a stored null. |

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
unchanged; the schema gates the group on `kind: "relation"` and keeps it
illegal at every other root, so the same member name cannot be reclassified
by kind drift.

**The three spellings are refused in `properties`** — under any spelling
that RESOLVES to one of the stored keys (§3), legend included, on every
kind, with the repair named:

```
/properties/relation_format: "relationFormat" is written on a relation
                             document's envelope as "format": "<a §3 format
                             name>" in relation_settings (§2d), not as a
                             property
```

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
custom key (a media space really can have a "Format" column) and a relation
object carrying one must stay exportable (I1). Refusing a key is not refusing to NAME it: a slot
that references the relation — the Property type's own property definitions and
dataview columns, in 64 production spaces — keeps the §3 slug
(`relation_format`), because the deny rule protects the legend and a
bundled-bound slug needs no legend entry.

**Target types translate at the boundary.** The store keeps
`relationFormatObjectTypes` as type OBJECT ids
(`objectcreator.fillRelationFormatObjectTypes`); the document spells type
keys. The translation is the optional `TypeResolver` capability of
`Options.ResolveProperties` (storeresolver implements it from the same one
bounded type listing §7.5a-2 budgets): export inverts id → key, import key →
this space's id, so a resolver-wired round trip is id-exact. An entry the
capability cannot answer — a bare key legacy imports stored directly (21
production entries), an id the space no longer serves, the
`_missing_object` sentinel — passes through **verbatim in both directions**,
its own address (§3). Pass-through rather than §2a's drop-the-dangling
policy, deliberately: there the resolver is the only thing that knows what a
recommended-list id meant, here the stored value IS the meaning, and a
backup format that deletes it on export is disqualifying. Without any
resolver the whole list passes through verbatim and the offline round trip
is byte-exact.

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
| a relation document's definition fields (§2d) | one `propertyDefinition` |

The shape's ten members: `key`, `name`, `format`, `options`,
`object_types`, `description`, `include_time`, `max_count`, `readonly`,
`default_value`. It states no `required` of its own and stays open — each
home layers over a `$ref` to it, adds its own requirements, narrows what it
must, refuses the members another surface already owns, and closes itself
with `unevaluatedProperties: false`. A home may **narrow** a shared member
(an authored home pins `format` to `authorableFormat`; a type's
`object_types` is a real array, since only a relation's stored value can
hold a null) but never restate its shape: two statements of one member
agree today and drift tomorrow, which is the §15 #14 disease — one concept,
two spellings, in one format — that §2d was written to end.

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
dictionary says *what they mean* (§7.3 of the design record; the manifest
belongs in the index because a manifest is what an index is).

```json
{
  "$schema": "https://schemas.anytype.io/anyblock/1/properties.schema.json",
  "version": 1,
  "installed": ["createdDate", "dueDate", "tag"],
  "properties": [
    { "key": "6a32d4856761631534b22f85", "name": "Budget", "format": "number" },
    { "key": "693c14f2aa11631534b22f01", "name": "Owner", "format": "objects",
      "object_types": ["participant"] },
    { "key": "dueDate", "name": "End Date", "format": "date" }
  ]
}
```

**Why it exists, measured.** 10,617 of 38,061 corpus documents are
`kind: relation` — 5.8% of the bytes — and 9,675 of them are installed
copies of the 194 bundled relations, **98% field-identical to
`bundle/relations.json`**. Each spends a ~967-byte document, with its own
envelope, attribution and system properties, to restate `{key, name,
format}` a table every reader already ships. The dictionary replaces those
restatements: export omits a bundled relation document whose definition
matches the table (§11), and one key in `installed` stands for it.

Two members:

- **`installed`** — the BUNDLED property keys present in the space, as
  stored keys only: presence, not definition. A restore reinstalls each key
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
entry, a kept relation document's `relation_settings`, and a type's
`property_definitions` — and 273 kept bundled-key relation documents in a
77-space export also carry an entry, so the pair is ordinary rather than
exotic. The order is:

1. **The bundled table**, for a key it names. It ships with every reader and
   is the same in every space (§7.5a-1), so no document can redefine a
   bundled property — an entry for one DOCUMENTS it, and the tools warn when
   the two disagree rather than accepting the entry in silence.
2. **The dictionary entry**, for every other key. It is the bundle-wide
   statement, and the one an author writes when there is no relation
   document at all.
3. **A type's `property_definitions` entry**, which narrows nothing and adds
   only `section` — what THIS type does with the property, not what the
   property is.
4. **A kept relation document's `relation_settings`**, which is the same
   `propertyDefinition` and should agree by construction; where it does not,
   the dictionary is the bundle's answer.

The redundancy is deliberate and stated here so it stays reasoned: a type
document is a self-sufficient authoring unit (§2a), and the dictionary is
what a bulk reader consults. Unreasoned duplication is how one concept ends
up with two spellings — §15 #14 is the record of that happening.

- **`properties`** — one `propertyDefinition` (§2e) per property the
  bundle's objects actually REFERENCE. **Used-only, not
  everything installed**: a space installs a median 125 bundled properties
  and uses 57 (47%), and the 68 nothing touches buy a reader nothing a
  restore does not already provide. Space-minted properties appear here in
  full — the dictionary is where an author declares a property without
  writing a relation document at all, in the same vocabulary as a type's
  `property_definitions` entry.

**Keys are STORED keys, never document spellings.** A document spells a
property by its label; its `property_keys` legend binds the label to the
stored key; the stored key is what the dictionary answers for. Entries
therefore need no legend of their own: every key slot in this file (`key`,
`object_types`) speaks the stored spelling directly.

**The reader flow, in full, and the step that is easy to miss.** A label
resolves in this order — the document's own `property_keys` legend; then a
verbatim match against a dictionary key; then **the api-slug derivation
applied FORWARD to the dictionary's own keys**. That third step is not
optional garnish: measured over a produced 77-space export, of 503,919
property value slots **5.7% resolve through a legend line, 24.0% match a
dictionary key verbatim, and 69.4% resolve only through the derivation** —
because §3's exhaustive rule writes a legend line only for a spelling the
bundled table does not bind, so a bundled property's label never gets one.

The derivation is `snake_case` of the stored key (`bundle.ApiSlug` is
exactly `strcase.ToSnake`, a pure function of the key with no table behind
it): `addedDate` → `added_date`, `mediaArtistURL` → `media_artist_url`,
acronym and digit runs split. A reader applies it to every key in
`installed` and every entry `key`, building its own label→key map once.
**Derive forward, never invert** — four bundled keys do not survive a
reverse transform (`mediaArtistURL`, `oldAnytypeID`, `_score`,
`_final_score`), and forward derivation from keys you already hold has no
such ambiguity.

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

`version` is the same format version, under the same rules, as every other
file in a bundle (§2c, §10): one integer, one namespace, gated before the
schema so a newer version gets the dedicated both-versions error rather
than a generic const failure.

The tooling knows this is not an object document, the way it knows
`index.json` is not: `anyblockbatch.DiscoverJSONFiles` excludes it,
`anyblockvalidate` validates it against its own schema (and warns — tools
warn, the codec tolerates — on an `installed` key the local table cannot
name), and `anyblockconvert` reads it as a declaration source (§3, the
import wiring).

## 3. Properties

`properties` is a JSON object keyed by **property key**, always in its
snake_case **label** — `due_date`, `plural_name`, `manual_property`,
`publish_date` — bundled, API-created and UI-created keys alike. One
vocabulary, no aliases, no duality: a reader never has to know which kind of
key it holds. (This overturns the earlier "as stored, camelCase" rule, and
it is the format half of the same decision the API surface makes; a bundled
key's label is the api slug from `core/api/util/key.go`.)

The mapping is a **table, both directions, never a case transform**: for
bundled keys the derived table in `pkg/lib/bundle` (which ships with every
reader, so documents still resolve offline), and for every other key the
space's own vocabulary, which a node-backed reader primes from the space —
and which the document carries the inverse of, entry by entry (`property_keys`,
below). `mediaArtistURL` → `media_artist_url` → `ToLowerCamel` would yield
`mediaArtistUrl`, and `_score` does not round-trip at all — string inversion
cannot be the reverse mechanism, and the package's tests pin both cases.

**The label, and the grammar it is minted through.** A key's label comes from
its own authority, and there are two:

1. **A bundled key spells its derived api slug**, from the table that ships
   with every reader — `dueDate` → `due_date`. All 223 bundled slugs are
   already legal keys under the grammar below, which a test asserts rather
   than assumes.
2. **A space-minted key spells what its space says it is**, through one
   ladder, first answer wins: its stored `apiObjectKey` when that is already
   a legal key — **re-spelled by the display name when the two are one fold
   class** (`bundle.FoldApiKey` drops `_`, so `git_hub_stars` and
   `github_stars` are already indistinguishable to every reader; the slug
   decides WHICH WORD, the name decides how to break it, and the slug is
   reliably the mangled half because it is snake-cased at mint); else that
   slug **normalized**; else its display **name** normalized; else nothing — and then the stored key is written verbatim,
   which is always its own address (chain step 4 below). A slug that merely
   repeats the stored key is not a slug (rows exist that carry the bson id as
   their own `apiObjectKey`), so it falls to the name; and a key the bundled
   table speaks for never consults its space's row at all, or a localized
   name would take a spelling from the table that ships with every reader.

An **absent** `format` in either slot that carries one (`property_definitions[]`,
a dataview's `properties[]`) says the document did not speak, and the §3
chain answers — the bundled table, then the caller's resolver. It is NOT a
declaration of `text`: that reading silently overrode the table, so
`{"key": "due_date"}` pinned a bundled DATE property to longtext and its
filters stopped being dates, while omitting the list entirely resolved
correctly. Naming a property was strictly worse than staying silent about
it. Canonical export always writes a format, so an absent one only ever
arrives from a hand-written document — the population that means "I did not
say".

**Normalization** is NFC, then: letters and digits of **any script** are kept
and lowercased, combining marks are dropped (a mark belongs to the letter
before it), every other rune is a separator, runs of separators collapse to
one `_`, and a trailing run trims. **A LEADING `_` run is kept** — `_` is
`identStart`, so `__amemory_salience` needs no repair, and 20 production
relations from two integrations namespace themselves exactly that way in both
their name and their slug; a leading run is a first character, not a gap
between words.

Note what is NOT applied: the `strcase.ToSnake` the api slug is minted with.
It used to be, so the two surfaces would converge — but it splits acronyms
and digit runs, and a display name is full of both: `P2P Sync` →
`p_2_p_sync`, `Platform SDKs` → `platform_sd_ks`, `GitHub` → `git_hub`. It
also bought nothing on the real input, since camelCase is a KEY phenomenon
(`dueDate`, `iconEmoji` are stored keys) and this rule is fed a display NAME,
which separates its own words because a person typed it. A name that IS
camelCase is a key pasted into a name field; the corpus holds two, and
`iconemoji` is the whole price. A result that starts with a digit, or that *is* one of
the filter grammar's keywords, takes a leading `_` — `50% done` → `_50_done`,
`All` → `_all`. A result that is empty, or longer than the 128-character key
bound, is no label at all.

The grammar is **§6.2.1's**, and it is normative rather than stylistic: a key
is a Unicode identifier — `identStart identPart*`, letters of any script,
combining marks (UAX #31 `ID_Continue` admits `Mn`/`Mc`, and in Devanagari,
Thai, Bengali, Tamil, Khmer and Myanmar the VOWELS are marks — dropping them
does not shorten a word, it changes it: मिल/मूल/मल/मैल would all become मल),
digits, `_` — and not one of the filter language's reserved words. A label
outside it **cannot be written in a compact filter string**, which is a
surface this format serves to models as an EBNF grammar. That is not a corner
case: a space-minted stored key is a 24-character bson id, and a bson id
starts with a digit, so before this rule 39 relations in a 36,966-object
corpus had no spelling that could be filtered on at all.

Three consequences worth stating outright:

- **Non-Latin scripts are kept, never transliterated.** `Тоггл` is `тоггл`
  and `日本語のプロパティ` is itself. The api slug's transliteration exists
  because a slug is a URL path segment there; it would answer `toggl` and
  `ri_ben_yu_nopuropatei` here — unguessable and unreadable at once, which is
  strictly worse than either the name or the key.
- **A minted `apiObjectKey` outranks a display name, and is the
  rename-stable address.** A label derived from a name changes when the name
  changes, and the next export writes the new one; the stored key never
  moves, and the `property_keys` line every non-bundled key already carries
  keeps every document already written resolvable through chain step 1. Where
  two entities in one space want one label, the explicit claim keeps it and
  the derived one goes without; where the two claims are equal — two stored
  slugs, or two names — neither wins and both are written verbatim, because
  an ambiguous address must never resolve by store order. `Priority` and
  `Priority 📌` are a real pair, in a real space.
- **A label may not take a spelling that is already answered.** The bundled
  table's spellings are not up for grabs (a space-minted relation named "Due
  date" does not become `due_date`), a live stored key outranks any label
  (verbatim-first, below), and `id` and `type` are never minted as property
  labels because §2 refuses those two spellings before any resolution.
  Within one document the term ledger applies the same discipline (*one term,
  one key*, below).

**Resolution — one rule, stated once, covering both namespaces.** The
format names keys in two namespaces — property keys and TYPE keys — and
every key slot lands its term on a stored key through the same chain, first
answer wins, run against the slot's own namespace: its legend, its half of
the bundled table, its stored-key set.

1. **The document's own legend** (`property_keys` for property slots,
   `type_keys` for type slots — identity entries included) — the only
   statement the *document* makes about its spellings.
2. **An exact stored key — verbatim-first.** A term that names a stored key
   means that key, always; the bundled slug table applies only to terms that
   are *not* stored keys. A node-backed reader answers this step from its
   store (`storeresolver`, both namespaces); a package-only reader has no
   stored-key set and knows a term is a stored key only when the legend says
   so — which is why export owes the identity entry below for every term the
   bundled table does not bind to the key being written.
3. **The bundled derived table**, which ships with every reader.
4. **Verbatim** — the term *is* the stored key, which is what keeps a
   package-only reader — with no space to ask — lossless on custom keys.

A conforming document resolves identically in every conforming reader:
steps 1, 3 and 4 need nothing but the document and the shipped table, and
wherever step 3 cannot answer for a term the document itself uses, the
document carries the entry that moves the answer into step 1 — so step 2,
the one step that needs a store, is never load-bearing for a document's own
spellings. Every other
statement of resolution order in this document is shorthand for this chain.

The namespaces are **disjoint claim domains**: a property and a type may
share a spelling without conflict (`object_type` the type key coexists with
`objectType` the layout value below, and a space may slug a relation and a
type onto one term), which is why the legends are two maps and export runs
one term ledger per namespace — a shared domain would back a key off a slug
the other namespace owns, a conflict this format defines away.

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

- **Emitted for every spelling the bundled table does not bind to the key
  being written.** One condition, two halves, and they ask different
  questions: the bundled table must **bind** this spelling to this very key
  (it ships with every reader, so `due_date` → `dueDate` owes nothing), *and*
  the vocabulary in force must **invert** it (a reader may bind a spelling the
  bundled table binds correctly, and the writer's own space is the reader most
  likely to read the document back).

  The asymmetry is what makes the rule exhaustive. A term that is a stored key
  written verbatim trivially *inverts* through any table, because a table that
  does not know a term answers the term itself (chain step 4) — so asking the
  bundled half as an inversion let every custom key pass with no entry at all,
  and the document said nothing about the one population no reader can resolve
  without it. That silence is the **corpse-after-export** hole: the key is
  live and unambiguous the day it is written, and the moment the relation is
  UI-deleted its stored key stops being live while the freed spelling becomes
  some other relation's api key. Every document already written re-points,
  offline, and no writer could have warned about it — the delete happened
  afterwards. Only the document itself can close that, so a spelling the
  bundled table does not bind owes an entry, verbatim or not.

  **The identity entry is therefore the ordinary line, not the exception.**
  Every custom key names itself: `{"customStatus": "customStatus"}`. Two
  shapes that used to be called out as special are just instances of the one
  rule now.

  The first is the bundled *shadow*: a space whose relation is keyed
  `due_date`, beside bundled `dueDate`, exports
  `"property_keys": {"due_date": "due_date"}` — the document's only way to
  tell a reader with no store that the term is a stored key (chain step 2).
  Without it, the value silently moved onto the bundled twin in every
  package-only reader.

  The second is **the vocabulary in force**, which is the half that stays an
  inversion, and it stays for a measured reason: dropping it — "ask one table,
  not two" — loses `{"task": "task"}`, and a template comes back pointing at
  an unrelated custom type; and loses `{"due_date": "dueDate"}`, and dueDate's
  value lands on the custom relation that wanted the spelling. Both are silent
  losses of user data. A vocabulary is consulted *before* the bundled table (chain step
  2 is a node-backed reader's store), so a term the bundled table inverts
  correctly can still be bound elsewhere by the reader most likely to read
  the document back: the writer's own space. This is not a hypothetical about
  hand-written vocabularies — it is what a **delete** produces. A UI-deleted
  type or property vacates the slug namespace while every object it ever
  named keeps its stored key, and the freed spelling becomes another live
  entity's api key: `initiative` stops being a live stored key and starts
  being the slug of some other type, so `"type": "initiative"` written with
  no entry came back as that other type, silently. The property namespace
  produces the loud half of the same fault — the two spellings then address
  one property and the document's own Unmarshal refuses it (§11, I1). Export
  therefore asks both tables, and writes `{"initiative": "initiative"}` when
  either would answer something other than the key being written. The entry
  is authoritative for *every* reader, which is the point of a legend; what
  it cannot cover is a reader whose vocabulary disagrees with the bundled
  table in a way the writer never saw, and that is the `KeyVocabulary`
  precondition (§11.1), not a legend rule.

  The legend is therefore empty for a document whose every spelling is
  bundled, and costs one line per non-bundled key otherwise. **Size**: the
  four golden documents, which each carry two custom keys, grow 93 bytes —
  about 2%. The adversarial corpus, where every document carries five or more
  custom keys, grows up to 15%; that is an upper bound, not an estimate. The
  product's store-backed path pays close to nothing new, because a
  store-minted relation key is a 24-hex bson while its api slug is derived
  from the name, so slug ≠ key and the entry already existed.
- **Consulted first, before any vocabulary.** The legend is the only statement
  the *document* makes about its own spellings; a vocabulary belongs to the
  reader, and two readers disagreeing about a slug is exactly how a property
  ends up naming a different relation than it was exported from.
- **It covers every key slot, not just `properties`.** Wherever the format
  names a property — a `property` block's `key`, a link block's `properties`
  list, a dataview's `property`/`group_by`/`cover_property`/`end_property`,
  a filter's or sort's `property`, a property-definition entry's `key` — the
  slug is written through the same recording step and read back through the
  legend first. A slot that writes the slug without recording the entry
  inverts only when some *other* slot in the same document happened to record
  it, which is luck rather than a guarantee; a slot that reads without the
  legend never inverts at all, even when the entry is right there.
- **One term, one key — document-wide.** Export claims every spelling
  through a single term ledger, exactly as ids go through one id domain
  (§4): a stored key the document names *anywhere* always keeps its own term
  (verbatim-first — no other key's slug may take it), a slug goes to its
  first claimant, and a later key whose slug is already claimed is spelled
  with its stored key instead, which is always its own address. The
  discipline covers every key slot, not just `/properties` — a `property`
  block whose slug collided with a `/properties` spelling used to record a
  legend entry that rebound the term, so that property's value landed on a
  different relation, silently; and two blocks sharing one slug collapsed
  into naming one key.
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
  had the same hole: a denied key never takes a slug, and an unwritable slug
  is never spelled — but a key with *no* slug at all skips both checks, and
  the term that reaches the ledger is then the raw stored key. So a stored
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
  property-definition entry's `key` (§2a) is an ordinary string value the
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
  Only `/properties`, `property_definitions[].key` and `property_definitions[].
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
  binds a denied *slug spelling* to a harmless stored key (a space-minted
  `apiObjectKey` may collide with a bundled spelling, and an identity entry
  for a shadow stored key is exactly this shape) is honored: nothing lands
  on the internal key, so nothing is refused.

**The type namespace carries the same inverse: `type_keys`.** Everything
above holds with `type_keys` for the legend, the type half of the bundled
table, and the type slots — the envelope `type` and `template_for`, and
`property_definitions[].object_types` (§2, §2a). Export claims type spellings
through a term ledger of the namespace's own, seeded by the same census
(every stored type key the snapshot or the resolved type-property
definitions name), and writes identity entries under the same trigger: a
custom type stored as `object_type`, beside bundled `objectType`, exports
`"type_keys": {"object_type": "object_type"}` or a package-only reader lands
on the bundled twin. Four rules are the namespace's own, each from what a
type key is — and one rule above that deliberately does **not** carry over.

- **No duplicate-binding refusal.** `/properties` refuses two spellings that
  bind one stored key; the type namespace admits them.
  `{"kind": "template", "type": "a", "template_for": "b", "type_keys":
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
  `resolved_layout` through that type's own `recommended_layout` — is
  reachable as `{"properties": {"layout": …}}`, and `layout` is the FIRST
  thing the resolver that computes `resolved_layout` consults, above the
  type's answer. There is one place a type key selects a code path in the
  import wiring — a legacy `sub_object` document, whose first object type
  picks which real kind it migrates into — and all that path does is set the
  smartblock kind, which is `kind`, and fill in `source_object` when the
  document left it empty, which is `{"properties": {"source_object": …}}`.
  And merge resolution never reads the type list at all: the importer
  derives a document's identity from `kind` plus the envelope `key`, and
  from `unique_key` — never from the object types.

  Merge resolution *is* steerable, but through the **document's own
  fields**, not through type keys. `name`, `relation_key` and
  `source_object` are ordinary writable properties, and the relation's
  format — spelled `relation_format` in `properties` when this passage was
  first written, since lifted to the envelope's `format` (§2d), where it is
  just as writable and lands on the same stored detail — travels beside
  them; the importer uses them to pick which existing object a document
  merges into: a relation matches on its format together with `name` or
  `relation_key`, and a TYPE document matches on `name` alone, since this
  format strips `unique_key` and the name is then the only filter left.
  They stay writable deliberately — the §2d lift moved a spelling, never a
  capability, exactly because a stripped value that import refuses is a
  lossy export and "Marshal never emits a document its own Validate
  rejects" (§11, I1) is the stronger promise. The guarantee that an
  imported document cannot rewrite an EXISTING relation's or type's
  identity therefore belongs at the object layer, which every writer passes
  through, rather than in this format, which is one writer among several. A
  `type_keys` value is admitted by shape alone — the writable-key rule the
  schema enforces on both legends.
- **The primary type slots are unbounded, on purpose.** A `type_keys`
  spelling and a `type_keys` value both carry the writable-key rule (1–128
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
- **No reserved spelling** — and the *removal* of one is worth recording,
  because the reservation was elaborate and every piece of it is gone.
  `template` used to be reserved in both directions and against the reader
  as well as the writer: export keyed `template_for` emission off the
  spelled term, validation gated `/template_for` on it, and import derived
  the smartblock kind from it (§2) — all through the *document* half of the
  chain (legend → bundled table → verbatim, no reader vocabulary), so that
  `Validate`, which has no vocabulary at all (§13), reached the same verdict
  the importer did. The document's own legend could move the spelling
  freely, because it moved all three with it. A reader's vocabulary could
  not, precisely because that half of the chain could not see it: a
  vocabulary answering some other stored key for `template`, or landing
  another spelling on the template key, split the two resolutions of one
  field, and `{"type": "template", "template_for": "task"}` read through
  such a vocabulary produced a Template smartblock whose object type keys do
  not contain `template` — invisible to every template check downstream, all
  of which test for exactly that key — and whose re-export then dropped the
  target type outright.

  All of that was the cost of making ONE FIELD answer two unrelated
  questions: *which type does this object have* and *what kind of object is
  it*. `kind` answers the second, off a field no chain touches, so the two
  cannot disagree and there is nothing left to hold a vocabulary to. What
  deleted with it: both export refusals and their warnings, the import
  guard and its warning, the importer's private document-only resolver, and
  `Validate`'s duplicate copy of the §3 chain. What survives is two byte
  comparisons that resolve nothing — export keeps `kind` explicit when the
  term it is about to write is literally `template`, and `Validate` refuses
  a document with no `kind` whose `type` is literally `template` (§10). Only
  making `kind` mandatory on *every* document would delete those too, at
  ~16 bytes on every page in the format.
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
  `type_keys` line naming a type the document never spells.

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
  "priority": { "High": "bafyrei…opt1" },
  "severity": { "High": "bafyrei…opt2" }
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
A `property_keys` or `type_keys` value is **authoritative**: the reader takes
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
deterministic and a second export reproduces the first byte for byte (§11.3);
it is no better than name resolution here, and no worse. A rename, and a
duplicate name an object touches only once, are no longer lossy (§11).

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
| `done` | checkbox | completion state on task-like types |
| `due_date` | date | due date on task-like types |

The icon and the cover are **not** in this table: they are envelope fields of
their own (§2b), and the nine stored keys behind them are refused here.

**Canonical key order in `properties`** (implementation decision): the
well-known keys `name`, `description` first (in that order, when present),
then all remaining keys alphabetically. The list held `icon_emoji` and
`icon_image` until v0.25 lifted both above `properties` entirely — a stronger
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
meaningfully preserves (mirroring `core/block/import/pb`): `created_date`,
`last_modified_date`, `is_favorite`, `is_archived`, `resolved_layout`.
Those five are **output-only** (§4a): export writes them, generators should
not — with one deliberate exception. **`is_favorite` is authorable**, because
the pb importer reads it to choose a space's root objects
(`core/block/import/pb/space.go`), which is how a generated bundle
designates the object a user should land on. A bundle with no favourite, no
`homepage` and no `spaceDashboardId` imports as an undifferentiated list. `id` is lifted to the envelope and `type` to `type`. Everything else
round-trips.

**Attribution: `creator` and `last_modified_by` are the member's RESOLVABLE
id, named by the informative suffix — `<identity>#<name>`, as a plain
string.**

```json
"creator": "A6eK73JmBUM9Aar2BJ4Pd6VkLW7cjhoWL7tJHDM9gk8fhpkc#roma_kha",
"last_modified_by": "A6eK73JmBUM9Aar2BJ4Pd6VkLW7cjhoWL7tJHDM9gk8fhpkc#roma_kha"
```

Not an array. Both relations are `maxCount: 1` and 0 of 36,966 production
values were multi-valued, so the list wrapper the other object-format
properties take is definitionally wrong here.

The spelling is the general §9 reference shape: the stored participant id
through the participant fold (48 characters instead of 135), the member's
display name riding after the `#` as a caption. This is a deliberate
reversal of v0.24, which wrote the NAME alone. Name-only was a mistake this
document owns: it broke API v2, whose consumers need an id to resolve a
member (avatar, profile), and **two members of one space can carry the same
display name** — 76 of 2,478 production participants do — so the name
identified nobody. The suffix keeps what v0.24 bought (a reader sees WHO,
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
rule, the layout-name check and the format-shape warning to the result.
Checked against the raw spelling instead, all three were dead for exactly
the documents this format produces: `unique_key` walked past the rule that
`uniqueKey` tripped, and a `property_keys` entry could rebind any harmless
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
failed to import; the original repro was the icon pair, which v0.25 refuses
one step earlier). The two halves agree exactly whenever no wider
vocabulary is in force, which is what keeps Validate and Unmarshal
accepting the same documents (§12).

**A property key has to be writable.** Non-empty, no control characters, at
most 128 characters (`propertyNames` in the schema, restated in the reader so
the issue can name the offending key — §12). This is a *deny* rule and
not an allowlist on purpose: real keys are bundled lowerCamel names, bson-hex
ids, and bare names from old accounts, and an allowlist could only be trusted
after checking every key in every account — while the shapes ruled out here
(the empty key, a key with a newline in it) are keys nothing can read. Export
drops such a stored key with a warning, since there is no way to write it.

The rule binds the **spelling**, and the spelling is whatever the vocabulary
answers: the shipped one normalizes it (§3's label rule bounds the length and
the charset), but `Options.Keys` accepts an implementation from anyone, and
the raw material underneath is an `apiObjectKey` that is user-supplied or
strcase-derived from the property name with no length bound and no
reserved-word check — so nothing upstream *guarantees* a spelling this format
accepts. Export
therefore checks the slug it is about to write, and one it cannot honor
falls back to the stored key — always its own address (verbatim-first) —
with a warning naming the vocabulary's answer. Three answers export cannot
honor: an **unwritable** slug (over-long, empty, control characters — on
either side of a legend entry); a slug the deny rule refuses **as a
spelling** before any resolution (`id`, `type` — the envelope's, which the
legend cannot re-purpose and therefore cannot rescue; a property named "ID"
really mints this slug); and any slug for a **denied key**, whose legend
entry would carry a value admission refuses. Checking the stored key and
then emitting the slug unchecked made `Marshal` produce a document its own
`Validate` rejects, on `/properties` and `/property_keys` at once, which
§11 rules out.

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
  Nested dataview/table objects: the order listed in §6. `property_keys`,
  `type_keys` and `option_ids` entries sorted by key, and each `option_ids`
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
(`coverId`/`coverType` stood beside them until v0.25; the typed `cover` field
gave them a node for the first time, and the `source` member that carries
their provenance is annotated.)

Output-only surfaces: `fields` (any block), `root`, `store`, `source`
(dataview), `groups`/`object_orders` (views, §6.2), `id` on sorts/filters,
filter `nested_property` (reserved), `cover.source` and the `emoji`
carry-over on `icon`'s named-icon branch (§2b), the five preserved internal
properties listed in §3, and the two attribution properties
`creator`/`last_modified_by`.

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
| `bulleted_list_item` | Text/Marked | `color`, `text` (Notion/BlockNote naming) |
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
  swallowed into (§9a). Valid only on `objects`/`files` properties, since they
  resolve to an object id; anywhere else is a validation error. Note the
  date presets are a *different* mechanism — a first-class `quickOption`
  field with real Go-side semantics (§6.2, `quickoptions.go`) — and the
  template-placeholder feature (`model.Placeholder_PlaceholderCurrentUser`)
  is unrelated: it fills property defaults when an object is created from a
  template, is resolved in Go, and never appears in a filter.

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
  either, because the count is never read: nothing is silently anything. That
  scope is not a nicety, on either half. The condition half is where the rule
  met the one above it, since `value` is *dropped* on
  `empty`/`not_empty`/`exists` leaves, so a stored filter combining one of
  those with a counting preset made `Marshal` emit a document its own
  `Validate` rejected (§11); the format half was refusing a document the app
  runs exactly as written, since a preset on a `select` property is read by
  nothing. The format the check reads is the one import attaches (below): the
  dataview's own `properties` list first, then `bundle` on the stored key the
  §3 chain resolves the term to.

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
not define it, so introducing it later is a version bump (§10 — a
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
which is the worst thing to freeze, and it would leave an authored
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
separate checks because they are separate readers, and for one revision only
the array form was closed, so `Validate` accepted an object-form container that
`Unmarshal` hard-refused — I2, in the one shape §7a cannot lift. A cell whose
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

**This is an obligation on the wiring, and it is worth writing down**: the
re-wrapping is the editor's, not the format's. A writer that builds a
snapshot and stores it WITHOUT going through the object-creation path that
enables layouts (`EnableLayouts`, whose one non-test call site today is
`core/block/editor/page.go`) will land a thousand-child object in front of a
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

This is **accepted, not fixed**. The shape's entire content is "no ids", and
an id-less document carrying a map of ids would not be one; the export and
backup shape keeps the legend, and that is the shape a round trip uses (§11).
The loss is small, real, and rare — and it is on the read/prompt path, which
is the one an agent sees most.

**Export does not warn about it**, and the reason is worth writing down
because the information is nearly in hand. At the moment export substitutes a
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
"related":  ["bafyrei…#local_first_ux"],
"assignee": ["A6eK73Jm…#roma_kha"],
"creator":  "A6eK73Jm…#roma_kha",
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

### 9a. The legends, and compact ids

The envelope carries **three legends** and no other indirection. Each answers
one question the rest of the document cannot:

| legend | maps | question |
|---|---|---|
| `property_keys` | property spelling → stored relation key | which relation does this spelling name? (§3) |
| `type_keys` | type spelling → stored type key | which type does this spelling name? (§3) |
| `option_ids` | property spelling → (option name → option id) | which option does this name mean? (§3) |

Three maps rather than one, and `option_ids` nested rather than flat, for one
reason stated twice at two scales: **a name in this format is arbitrary user
text, so no character can be reserved to join it to its scope.** The property
and type namespaces are disjoint claim domains and a space may slug a
relation and a type onto one term (§3), so a single spelling→key map would
hold two answers for it. One step down, an option name may contain anything a
JSON string may, and so may the property spelling that owns it:
`strcase.ToSnake("C#")` is `c#`, a legal api slug. A flat map keyed
`<name>#<property>` therefore had no representable entry at all for an option
of a property named `C#` — the escape hatch was unreachable exactly where it
was needed — and re-opening that after the freeze costs a version (§10).
Nesting removes the separator, and with it the split rule, the key admission
rule, the two charsets, and the joined key's length bound.

**`option_ids`.**

```json
"properties": { "priority": ["High"], "severity": ["High"] },
"option_ids": {
  "priority": { "High": "bafyrei…opt1" },
  "severity": { "High": "bafyrei…opt2" }
}
```

- **Outer key**: a property **spelling as this document writes it** — the
  reader that resolves the entry is reading the document, not the store — so
  it carries the writable-key rule every property spelling carries (1–128
  characters, no control characters, §3), and the property it names inverts
  through `property_keys` like any spelling elsewhere in the document. The
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
- **`OmitIds` drops it** (§9). An id-less shape that ships ids is not an
  id-less shape; the export and backup shape keeps the legend, the prompt
  shape does not. §9 states what that gives up — the two losses above, back,
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
holds** — the same principle the term census follows (§3, v0.15). A block the
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
- **A pre-release grammar change leaves no version marker.** `version` is 1,
  and the revisions this document records — v0.20's three legends replacing
  `refs`, most sharply — did not move it. A stored document written against a
  superseded revision is therefore refused by the *schema*, not by the version
  gate above: the marker it carries is a member the current grammar does not
  admit. That refusal is the only notice it gets, which is why the reader
  names the member (`/refs`) and states the rule that replaced it and the
  repair, rather than reporting a closed-set violation at the document root
  (§12). Whether the integer should move once before the format ships is a
  release decision, not a rule of the format (§15).

  v0.31's relation lift (§2d) is the same shape, and the same decision —
  **refuse, loudly, with the repair named**, never read-and-migrate. A
  pre-v0.31 relation document spells `relation_format` inside `properties`
  and has no envelope `format`. It trips the missing-`format` refusal, which
  carries the whole repair: the message lists the vocabulary and, when a
  legacy spelling sits in `properties`, says outright that it is the
  pre-v0.31 form and where the value moved. Measured over all 10,617 legacy
  relation documents in a 38,061-document corpus, every one trips exactly
  that refusal and exactly one — the `/properties/relation_format` refusal
  cannot also fire, because it lives in the semantic pass and a schema
  failure never reaches it. It appears on the second pass, once the envelope
  field exists and the old member is still there. The same message also
  names `format` in `properties` when that is what the author wrote, which
  is the commoner mistake and the one a missing-member verdict would
  otherwise never mention. Reading the old spelling with a warning was
  declined for the reason §2b records: a format with two legal spellings for
  one thing, one of them a raw enum number a small model has seen far more
  of, defeats the lift — and this format is a draft with no external
  consumers, so the refusal strands nobody.

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

The §2a `type_settings` group (v0.32) adds three normalizations, all scoped
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
outranks the vocabulary's own slug binding** — chain step 2 as an obligation
on the implementation, so a term that is some live entity's stored key
answers "not a slug", and no slug is emitted that a live stored key answers
to. Without the third, a document naming a relation by its stored key lands
on whichever other relation minted that string as its api key.

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
  decoding (see the v0.6 changelog). The one remaining recursive definition
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
  the layout-name check and the format-shape warning run; validation
  mirrors the importer's details seam refusal for refusal — a **denied**
  resolved key, an **unwritable** resolved key, and **two spellings binding
  onto one stored key** are all errors — and a `property_keys` *value* is
  admitted like the stored key it is, deny rule included; import re-runs
  the seam's checks on its own resolved key when a wider vocabulary is in
  force; the TYPE namespace mirrors the same way, minus one thing it used to
  need: the `/template_for` gate and the kind read `kind` alone, so neither
  runs the §3 chain and `Validate` no longer keeps a private copy of it, a
  `type_keys` spelling or value gets the same writable-key restatement as
  `property_keys`, and the import seam refuses a term a vocabulary resolves
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
  `properties`, on `property_keys` and `type_keys` spellings (§3), and on
  `option_ids` outer keys (§9a) — is checked by validating each name as a
  *standalone string
  instance*, so the verdict carries neither the enclosing object's location
  nor, for a length bound, the name itself. A 200-character property key was
  reported as `maxLength: got 200, want 128` at the document **root**, which
  names no property at all. The rule stays in the published schema, because
  an external validator runs that and nothing else, and the reader
  **restates** it where the key is in hand: `/properties/<key>`,
  `/property_keys/<spelling>`, `/type_keys/<spelling>`,
  `/option_ids/<spelling>` and `/option_ids/<spelling>/<name>`, with the
  offending string in the message. A `property_keys` *value* is covered the same way — the schema
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
  until v0.22 and means an ordinary page whose type is the Template type
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
                               for, normalized through §6.2.1's identifier
                               grammar. A vocabulary calls it; the codec
                               never does
  blockvocab.go              — the block-type name tables (§5)
  viewvocab.go               — the dataview enum name tables (§6.2)
  fragment.go                — the FRAGMENT surface: one block, a flat run,
                               one property value, the §8 inline codec (below)
  filters.go                 — the fragment surface for a §6.2 filter tree and
                               sorts array, standalone (query paths)
  index.go                   — the bundle index (§2c)
  storeresolver/             — the space-backed implementations of the four
                               resolvers, including KeyVocabulary; the only
                               place that reads a space's api slugs and
                               display names, and the one that applies the
                               §3 label rule to them
  snapshotdiff/              — snapshot ↔ snapshot diffing for the PATCH path
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
// exported constants for the wiring's dispatch.

// KeyVocabulary translates between the STORED keys a snapshot carries and
// the SLUGS a document spells, in both namespaces and both directions (§3).
// The default is BundledKeyVocabulary — the derived table that ships with
// every reader, and nothing else. storeresolver supplies the space-backed
// one. Three preconditions on an implementation, none implied by the one
// before it: it inverts what it emits; it never binds a spelling the bundled
// table binds elsewhere; and a live stored key outranks its own slug binding
// (§3, §11.1).
type KeyVocabulary interface {
    PropertySlug(key string) string
    PropertyKey(slug string) (key string, ok bool)
    TypeSlug(key string) string
    TypeKey(slug string) (key string, ok bool)
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
    ResolveObjectNames ObjectNameResolver // optional; export only, behind RefNames. nil = references written bare (§9)
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
// {"property_keys": {…}, "option_ids": {…}, "blocks": […]} — plus "type_keys",
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
`property_keys: {"priority": "6a32d485…"}` carries the spelling and not the
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
`ReservedWidgetTargets`, `IsReservedHomepage`, and the two that translate a
reserved name into the importer's own bare spelling, `WireWidgetTarget` and
`WireHomepage`.

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
  "icon": { "format": "emoji", "emoji": "🔥" },
  "cover": { "format": "gradient", "gradient": "pinkOrange" },
  "properties": {
    "name": "Project Phoenix",
    "status": ["In progress"]
  },
  "option_ids": {
    "status": { "In progress": "bafyrei…opt1" }
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
3. **Option names vs `{id, name}` objects** (§3): **settled** — names stay in
   the value, generatable and readable, and the id rides beside them in
   `option_ids`, under the property that owns the option (§9a).

   The rejected alternatives are recorded here WITH the evidence that killed
   them, because each was proposed more than once and each looks reasonable
   until the evidence is in hand.

   - **A flat legend map with a separator** (`#`, deleted at v0.20). No
     separator survives contact with real option names and property slugs:
     `bundle.ApiSlug("C#") == "c#"` and `ApiSlug("#1 priority") ==
     "#1_priority"`, so `#` appears inside both halves of the joined key. The
     nested shape needs no separator at all (§9a).
   - **A sigil in the value itself** (`"@opt-high"` marking a handle). Two
     independent falsifications. A legal property slug can BEGIN with the
     sigil — `ApiSlug("@home") == "@home"` — so the marker is not
     distinguishable from data. And `Validate(data []byte) error` takes no
     resolver of any kind (§13), so it cannot know whether a `/properties`
     value is a select value or an object reference: it would have to accept
     the sigil everywhere (breaking I2, since `Unmarshal` with a resolver
     refuses more) or refuse it somewhere `Marshal` emits it (breaking I1).
     Export's own deep links are not the counter-example they look like:
     `objectLinkDest` builds the URL with `url.Values{}.Encode()`
     (`inline.go`), so an id starting with `@` is written `%40…`.
   - **`{name, id}` value pairs.** This was the standing fallback for the
     duplicate-name and rename caveats, and it was believed to be a
     format-only change. It is not: `model.RelationOption` is
     `{Id, Text, Color, RelationKey, OrderId}` — there is **no key field** —
     so `ListRelationOptions` cannot supply the stored keys the byte-cost
     argument for the pair shape rested on. It also puts a second value shape
     in the slot small models write most often.

   One argument that must NOT come back attached to any of these: the sigil
   designs were largely defended as protecting `object_ids` against a dropped
   legend. Object-reference compaction was deleted at v0.20 and `object_ids`
   never shipped — the only `object_ids` in this document is the dataview's
   `object_orders[].object_ids` field. Object references print in full,
   everywhere, and need no legend to survive. (The §9 `#name` suffix, added
   at v0.27, is not that legend coming back: it is a caption on a full id,
   inverted by deletion, with nothing to carry and nothing to resolve. The
   `#` inside an option NAME that killed the flat legend is harmless here
   because the id half of a reference provably contains none — the split
   runs id-first, not name-first.)
3a. **Attribution spelling** (§3): **settled twice, second answer stands.**
   v0.24 spelled `creator`/`last_modified_by` as the member's display name
   alone; v0.27 reverts to a resolvable id with the name as the informative
   `#name` suffix. An earlier working note (`CREATOR_SPEC.md`, outside this
   repo) argued the name-only position and is SUPERSEDED on this point: the
   name-only spelling broke API v2's need for a resolvable id, and a display
   name shared by two members (76 of 2,478 in production) identifies
   neither. Recorded here rather than silently changed, per this section's
   own rule that rejected positions carry the evidence that killed them.
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
9. **Should `version` move once before release?** The v0.20 grammar change —
   `refs` out, three legends in — was made under `version: 1`, which is
   correct while the format is unreleased (nothing is stored against the old
   grammar that a migration owes anything to) and is why a pre-v0.20 document
   is refused by the schema rather than by the version gate (§10). The
   question is whether the integer should be bumped once at release anyway, so
   that anything written during the draft period is refused by the gate, with
   its dedicated both-versions error, rather than by a member name. It is a
   release decision: §10's rules do not change either way, and the diagnostic
   (§12) stands either way.

   There is now one piece of code waiting on the answer: the v0.22 refusal of
   `{"type": "template"}` with no `kind` (§10). It exists because that shape
   is well-formed under both readings and would otherwise import silently as
   the wrong kind of object. A version bump refuses every draft-era document
   at the gate, at which point the special case is dead code and should be
   deleted along with the last use of the `template` string constant outside
   export's emission rule.
10. **Settled at v0.22, with the alternatives that were declined.** Both
    changes had a cheaper option that was considered and rejected on the
    evidence, and both are recorded so the cheaper one is not re-proposed as
    if it were new.

    - **`kind` as the sole template authority** (§2, §3). The declined
      alternative was to keep deriving the kind from the type term when
      `kind` is absent, which costs no migration at all: yesterday's
      `{"type": "template", "template_for": "task"}` would keep working.
      Rejected because it leaves the type term carrying structural meaning —
      the format would have two authorities and the incoherence the change
      set out to remove would survive in half, with
      `{"type": "template", "type_keys": {"template": "myThing"}}` meaning a
      template of type `myThing` for no stated reason. The opposite extreme,
      making `kind` REQUIRED on every document, was also declined: it deletes
      the `template` constant outright and refuses every draft-era document
      through the schema's `required`, but it costs ~16 bytes on every page,
      contradicts §4's omit-every-default rule, and refuses documents with
      nothing wrong with them.
    - **The `_` namespace** (§1, §2c). The declined alternative was to leave
      the reserved listings spelled as bare words and simply ban those six
      words as bundle-local ids — about six lines, no format change, no
      translation table, and it closes the shadowing completely. Rejected
      because it is a word list: every listing added later retroactively bans
      an id that was legal, and nothing about a bare `set` in a document tells
      a reader which of the two kinds of target it is. Note that the ban did
      NOT go away — the `_` prefix makes the FORMAT unambiguous, and the ban
      on the six wire spellings is still what makes the WIRE unambiguous,
      since the importer's own spellings are bare.

11. **The §3 chain's store step: both directions proposed, both declined.**
    Step 3c — the stored-key fold, store-backed readers only,
    single-candidate-or-nothing — has been proposed for deletion and for
    promotion, by different reviewers, and both are recorded here because
    each is falsified by one call.

    - **Delete it, and let the reader's own table be the whole of step 3.**
      Rejected: `bundle.RelationKeysByApiFold("Severity") == []`. The bundled
      fold knows nothing about a space's custom keys, so an agent that read a
      space's property listing and POSTed
      `{"properties": {"severity": […]}}` would resolve `severity` to
      nothing, pass it through verbatim, and silently mint a SECOND relation
      beside the one it just read. The store step is what closes that, and
      only a store-backed reader can.
    - **Promote it — make the fold mandatory, so every reader resolves the
      same way.** Rejected: `bundle.TypeKeysByApiFold("Task") == [task]`. A
      space holding a live stored type key `Task` — which this format
      creates, `{"kind": "object_type", "key": "Task"}` is legal — would have
      every reference to it folded onto the bundled Task type. Verbatim-first
      (§3 step 2) exists precisely so a stored key is its own address, and a
      mandatory fold would overrule it.

    The asymmetry is the point and is what §3 already states: a reader may
    resolve MORE than another, never DIFFERENTLY. Deleting the step makes a
    store-backed reader resolve less than it can; promoting it makes an
    offline reader resolve differently than it should.

12. **Trim system-property noise**: **settled at v0.29** — a whitelist of
    seven keys, spelled out in §3 and in `systemtrim.go` with the admission
    test each had to pass.

    The proposal was the inverse: a rule over `bundle.SystemRelations` (108
    keys) minus a small exception list. It was declined for two reasons that
    are worth keeping. It **fails open** — every system relation added in
    future joins the trim set with nobody having looked at it, and the cost
    of a wrong entry is a silently dropped value. And it buys nearly
    nothing: the saving is top-heavy, so seven vetted keys carry ~50% of it
    while the thirty-key tail carries 3.6% of 1.13%, or **0.04% of all
    bytes**. An explicit list gets nearly all the benefit with every
    omission reviewed.

    The `done` membership question the proposal left open is moot: `done` is
    not a system relation and was never a candidate. The real question turned
    out to be which system keys to ADMIT, and the answer is those whose empty
    value is both the proto zero and the semantic default — see §3 for the
    three that failed it.

    The measurement that motivated this also found `internalFlags` to be 24%
    of the saving and not a trimming question at all: it is transient editor
    state, so it went to the `transientProperties` strip list outright,
    independent of this item.

13. **The icon and cover assumptions the clients own** (§2b). Four, in
    descending order of what they would cost:

    - **Icon precedence is unverified outside heart.** `iconName` >
      `iconEmoji` > `iconImage` comes from `core/api/service/icon.go`, the
      only implementation in this repository — every other converter (`dot`,
      `graphjson`, `publish/relationswhitelist`) emits all four channels and
      lets the consumer decide. If the desktop client renders the emoji over
      the named icon, the export picks a different icon than the app shows
      for the 200 objects that hold both. **One grep in the client repo
      settles it**, and the answer changes one line of the export rule.
    - **`coverType: 4` (prebuilt) has zero instances** in 36,966 objects, and
      the prebuilt id vocabulary exists nowhere in this repository. It is
      modelled as `{"format": "image", "file": …, "source": "prebuilt"}`
      because `state/details.go` and `cmd/usecasevalidator` both treat
      `{1,4,5}` as file-backed. If a prebuilt `coverId` is a client-side
      asset *name* rather than an object id, the `image` branch is wrong for
      it and fixing it costs a version bump.
    - **The gradient and cover-colour vocabularies live only in the
      clients**, so `cover.color` and `cover.gradient` stay opaque names. A
      document can say `{"format": "gradient", "gradient": "sunset"}` and get
      a broken cover with no validation error — the one corner where the
      typed shape does not do what it exists to do. The format cannot close
      this alone; the API's discovery layer can serve the enum once the
      clients publish the list.
    - **`icon.name` is an open string** (§2b), which is where this design is
      weakest for an offline generator. Closing the ~397-name enum would
      violate I1 the first time the app ships a new icon, and would put
      someone on the hook for keeping `pkg/lib/*` in lockstep with the client
      icon set forever — the API's own list currently contains a stray
      `t.txt` between `sync` and `tablet-landscape`, which is what that
      maintenance looks like when nobody owns it.

14. **The `relation_format_*` family is the same disease at ten times the
    volume** — and the **spelling decision was taken in v0.31** (§2d),
    exactly as the sentence below prescribed: `relation_format: 100` became
    the envelope's required `format: "objects"`, `include_time` and
    `object_types` lifted beside it, and the flat spellings are refused with
    the repair named. What was deliberately NOT taken is the emptiness
    collapse this entry also describes: `include_time` is still
    present-and-false on 8,375 documents and `object_types`
    present-and-empty on 8,903, now on the envelope, because presence
    mirrors the store (§2d) and trimming it is a separate decision with its
    own snapshot-comparator cost. The original record, for the reasoning:
    `relation_format_include_time` is meaningful only on `date`,
    `relation_format_object_types` only on `object`/`file`, and both are
    present-and-empty on thousands; a standalone relation document spelled
    `relation_format: 100` — a raw number — while a `type_properties` entry
    spelled `format: "objects"`: one concept, two spellings, in one format.
    Of everything deferred it was the only one that cost a version bump if
    it slipped past the freeze, and the instruction was: if exactly one more
    thing fits, make it the **spelling** decision rather than the whole
    collapse. `file_variant_*` (7 parallel arrays on every file object,
    8.35% of corpus bytes), `space_invite_*` and `widget_*` remain deferred
    with less at stake — machine-written, never authored.

15. **`picture` stays flat, deliberately** (§3). It has the same relation
    format as `iconImage` (`file`, `objectTypes: ["image"]`) and 1,946
    production objects carry one, so folding it into `icon` looks tempting.
    It is a different concept — a bookmark's preview image, not the object's
    identity — and folding it in would make one union mean two things. It
    reads correctly as an ordinary `files` property. Written down so it is
    not re-litigated.

16. **Reusing a key across spaces** (follow-up, and NOT a format change).
    An agent that adds the same new type with the same new properties to
    several spaces gets a different stored key in each, because each space
    mints its own — so the same logical data stops being comparable across
    them. Measured: **39 spellings in a 77-space account already bind to
    more than one stored key**, `date` to three (`67dab1b0…` in 21 spaces,
    plus two others).

    The format already has the answer, and it is the legend: an author that
    mints the key ONCE and ships it in `type_keys`/`property_keys` gets the
    same key in every space, deterministically and offline. A legend entry
    naming a key the space has never seen creates it under that key. An
    agent minting for this purpose should use a RANDOM key, not a readable
    one — a readable key can collide with an unrelated property a space
    already has, and that collision merges two different properties in
    silence.

    The tempting alternative — look the type/property up in the user's
    OTHER spaces at creation time and reuse the key when it looks like the
    same thing — is declined for the format and left as a possible import
    feature. It is non-deterministic (the answer depends on which spaces
    exist at that moment), order-dependent (whichever space was created
    first defines identity for the rest), needs cross-space reads on the
    creation path, ties a private space's schema to a shared one, and its
    equivalence test is a name heuristic — so it silently merges exactly
    what §3's chain and the exhaustive legend rule exist to keep apart (see
    the cross-space mis-binding case there: without a legend entry, a
    document's `data` lands on the READER's unrelated `data` property, with
    no error and no warning).

    If it is built, it belongs in the import pipeline and it should be a
    **suggestion, never a bind** — "this space already has a property named
    Estimated Hours; reuse it?" — so that merging two identities is a
    decision someone made rather than one that happened.


17. **`order_id` should become `sort_position`, an integer.** It survived the
    §2a admission test because it carries something real — the user's own
    ordering — but what it carries is a **lexid**, written for a machine that
    needs cheap insertion between two neighbours. Measured: 946 documents
    carry one (603 `relation_option`, 343 `object_type`), every value exactly
    four characters, 184 distinct, commonest `VVVV`, `VWUz`, `VXUU`. Nothing
    about `VWUz` says "second": an agent asked to insert an option between two
    others cannot compute a value, and cannot read the order back without
    sorting the whole set first. `sort_position: 2` says what it means, an
    author can write one, and a reader can sort on it without knowing the
    encoding. The store keeps its lexid — this is a question about what the
    DOCUMENT spells, exactly like `relation_format: 100` becoming
    `format: "number"` in §2d. Its own attack pass: what import does when two
    entries claim one position, and whether export renumbers densely or
    preserves gaps. **Pre-freeze if it is wanted at all.**

18. **One statement of what a property KEY may be.** `$defs/propertyDefinition`
    bounds its `key` to 128 characters and a control-character-free pattern;
    `$defs/dataviewProperty` restates the slot as `{type: string, minLength:
    1}` and bounds neither. Demonstrated: a 200-character key is accepted in a
    dataview block's `properties[]` and refused in a definition, in the same
    document. The obvious repair — point both at one `$defs/propertyKey` — was
    tried and reverted: export really does emit such a column (verified), so
    tightening the schema alone makes Marshal emit what its own Validate
    rejects (§11 I1). The fix has to move export and the schema together,
    which is a coordinated change rather than a one-line `$ref`. Recorded
    because the one-shape rule §2e establishes is exactly what this violates,
    and it is the sort of drift that is invisible until someone measures it.
