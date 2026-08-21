# AnyBlock JSON — format specification

Status: **draft v0.21** · Format version: **1** · Package: `pkg/lib/anyblockjson`

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

Changes in v0.21: **every key slot admits before it writes, and every
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
warnings addressed the wrong place (§13): a dropped `type_properties` entry
pointed at the index the next surviving entry takes, and the template-spelling
guard said `/type` wherever it fired, including from
`/type_properties/N/object_types/M`, a field the pointer does not even name.

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
the writable-key rule wherever it is, including `type_properties[].key`,
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
`BuildRecommendedLists`, the PATCH channel for `type_properties`, refused
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
`type`/`template_for` and `type_properties[].object_types` — were slugged on
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
| 1 · export, backup | full — the bytes must re-import to the same document (§11) | full — always (§9a) |
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
  space rather than in this format. Their canonical spelling is the api slug,
  which is snake_case anyway — `icon_emoji`, `due_date`,
  `last_modified_date` — so most of the time the exemption does not show. It
  shows for a key the vocabulary has no slug for: a legacy `wikiPerson`, a
  minted `6a32d4856761631534b22f85`. Those are written verbatim, whatever
  their shape, because an exact stored key is always its own address (§3).
  This section once said the key ↔ key mapping was impossible because
  `Validate` takes no resolver; the answer was to put the mapping in the
  DOCUMENT — the `property_keys` / `type_keys` legends, which a reader with no
  space at all can invert.
- **Platform identifiers** — the reserved widget targets `allObjects` and
  `recentOpen` (§2c), the `dataview` block id (§7), and the `objectId`
  parameter of the `anytype://object` deep link (§8.1) — name things that
  exist in a live space. They are quoted, not translated.

(The JSON Schema's own `$defs` names — `blockCore`, `tableCell`, … — are
neither: they are schema-internal labels a document never contains, and they
keep JSON Schema's conventional camelCase.)

So `{"type": "callout", "icon_emoji": "💡"}` and
`{"properties": {"icon_emoji": "🔥"}}` are both correct in the same document,
and they are not the same thing spelled twice: the first is a field this
format defines, the second is a key belonging to the data, which happens to
be spelled the same way. `{"properties": {"wikiPerson": …}}` is where the two
part company.

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
| `version` | int | **yes** | Format version. This spec defines `1`. Every format change bumps it — there is no additive-within-a-version rule (§10). |
| `kind` | string | no | System-level object kind, snake_case (`page`, `profile_page`, `template`, `archive`, `widget`, `chat`, …) — from `model.SmartBlockType`. `chat` is `ChatDerivedObject`: a standalone chat object whose identity is `key`, like a type's; its messages live in the CRDT store, not in snapshots, so it always imports empty. (`chat_object` is the deprecated predecessor; `discussion` is a hidden type.) **Omitted whenever derivable**: absent means `page`, and `type: "template"` implies `template`. An unrecognized value is a validation error listing the allowed values. |
| `id` | string | no | Object id. Written by export; import treats it as informational (a new id is minted on import) except for resolving intra-export links. Written in full, like every object reference — object references are never compacted (§9a). |
| `type` | string | no | The object's type **slug** (`page`, `task`, `object_type`…) — the key vocabulary of §3, not the stored `ot-`-prefixed key. Maps to `object_types[0]` in the snapshot. Absent when the snapshot has no object types (legacy/system objects). Import inverts the term through the §3 chain in the type namespace — the document's own `type_keys` legend first, then the vocabulary in force (bundled table offline, the space's stored slugs inside a node) — and hands the resulting stored key to the wiring, which resolves it — matching an existing type or creating one (the Markdown importer's behavior). A term the chain does not know passes through verbatim — an exact stored key is always its own address (§3). The spelling `template` is **reserved** for the template type: the kind derivation and `template_for` key off the stored key it resolves to through the DOCUMENT's own chain, and both directions refuse a vocabulary answer that moves it — export writes the stored key instead, import uses the document's own answer, each with a warning (§3). |
| `template_for` | string | no | Only for templates: the target type slug (`object_types[1]`), same vocabulary and legend as `type`. Present when `type` does not **resolve** to the template key — through the document's own chain, `type_keys` legend first (§3) — → validation error. |
| `key` | string | no | Identity key of *system* objects (types, properties). This is the STORED identity key (a `uniqueKey`'s internal part), written verbatim: unlike every key slot in §3 it is **not** translated, so for an object whose stored key is a minted BSON it does not match the slug the public API serves as that object's `key`. Because it is verbatim, its charset is whatever the store already holds: a relation option's key is built from the option's *name*, so `completion_status_Not Started`, `…_C/C++` and `…_тогглы` are all real stored keys. The rule is therefore a deny rule — non-empty, no control characters, at most 255 characters — not an allowlist. An allowlist was tried and falsified: it failed 59 objects of a 36 808-object account, every one a relation option. Never emitted for ordinary documents. |
| `properties` | object | no | The object's properties, §3. |
| `type_properties` | array | no | Only for `kind: "object_type"` documents: the type's property definitions, §2a. Present on any other kind → validation error. |
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
  "properties": { "name": "Task", "icon_emoji": "✅", "recommended_layout": "todo" },
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
| `key` | string | **yes** | Property key, in the document's own spelling — a key slot like any other, inverted through `property_keys` (§3). |
| `name` | string | no | Display name. Import uses it only when the property must be **created**; an existing property keeps its own name. Every bundled key already exists, so a name given for one is inert — `{"key": "description", "name": "Summary"}` renders as *Description*. Validation warns. If the label is the point, mint a custom key instead of reusing a bundled one. |
| `format` | string | no | Property format (§3 names). Same import rule as `name`; a conflict with an existing property's format is an error at the wiring level (the package cannot see the space). |
| `options` | (string \| object)[] | no | A select/multi_select property's **vocabulary, in display order**. Each entry is a bare option name, or `{"name": …, "color": …}` when the option's color is part of the design — the color belongs to the option rather than to a parallel array, so inserting or reordering an option cannot shift it. `color` is one of `grey`, `yellow`, `orange`, `red`, `pink`, `purple`, `blue`, `ice`, `teal`, `lime` (`util/constant`); anything else is a validation error rather than a silently ignored value. The bare string is **canonical** whenever the option declares no color, the object form otherwise — the same rule cells follow in §6.1. Leaving a color out does not mean *no* color: the wiring assigns one, cycling the palette in declaration order and skipping whatever the vocabulary claims explicitly, so a vocabulary that names no colors still gets distinct ones. (The app assigns one at random on every other creation path; cycling keeps a converted bundle identical run to run.) Options are otherwise discovered only from values that happen to be used, so a vocabulary entry no record carries would never exist — its kanban column simply absent — and a discovered option carries no `orderId`, which makes every select sort alphabetically (options order by `[orderId, name]`, `pkg/lib/database.BuildOrderMap`). Declaring them lets the wiring create each one up front with an order id. Every option needs one: the sort concatenates `orderId + name` before comparing, so an option missing an order id is compared by *name* against the others' order ids and lands arbitrarily — ahead of the whole vocabulary when its name sorts below the id alphabet, behind it otherwise. Names discovered from usage rather than declared are ordered after the declared ones. Only meaningful on `select`/`multi_select`; duplicate names are a validation error, across both forms. |
| `object_types` | string[] | no | The **type slugs** an `objects`/`files` property may point at, in priority order — a type-key slot like the envelope `type`, so it speaks the one key vocabulary (§3), claims its spellings through the same type term ledger and owes the same `type_keys` legend; import inverts each entry through the legend first, and a term the chain does not know passes through verbatim. Empty means any object — an untargeted property will happily accept a random page as a task's assignee. Listing the built-in `participant` alongside a bundle's own people type is what makes the current-user filter value usable on that property (§6.2) while still allowing the seeded people as values; the client only offers it when the relation's targets include Participant. The wiring resolves each key to an id the way it resolves properties: a type the batch defines by the id its own document carries, a bundled type by its bundled url (`_ot<key>`). Only meaningful on `objects`/`files`. |
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
it is the format half of the same decision the API surface makes; the slug
itself comes from `core/api/util/key.go`.)

The mapping is a **table, both directions, never a case transform**: for
bundled keys the derived table in `pkg/lib/bundle` (which ships with every
reader, so documents still resolve offline), and for every other key the
entity's stored `apiObjectKey`, which a node-backed reader primes from the
space. `mediaArtistURL` → `media_artist_url` → `ToLowerCamel` would yield
`mediaArtistUrl`, and `_score` does not round-trip at all — string inversion
cannot be the reverse mechanism, and the package's tests pin both cases.

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
  a filter's or sort's `property`, a `type_properties` entry's `key` — the
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
  `type_properties` entry's `key` (§2a) is an ordinary string value the
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
  Only `/properties`, `type_properties[].key` and `type_properties[].
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
`type_properties[].object_types` (§2, §2a). Export claims type spellings
through a term ledger of the namespace's own, seeded by the same census
(every stored type key the snapshot or the resolved type-property
definitions name), and writes identity entries under the same trigger: a
custom type stored as `object_type`, beside bundled `objectType`, exports
`"type_keys": {"object_type": "object_type"}` or a package-only reader lands
on the bundled twin. Four rules are the namespace's own, each from what a
type key is — and one rule above that deliberately does **not** carry over.

- **No duplicate-binding refusal.** `/properties` refuses two spellings that
  bind one stored key; the type namespace admits them.
  `{"type": "a", "template_for": "b", "type_keys": {"a": "template",
  "b": "template"}}` validates, and yields `ObjectTypes:
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

  Merge resolution *is* steerable, but through **properties**, not through
  type keys. `name`, `relation_key`, `relation_format` and `source_object`
  are ordinary writable properties, and the importer uses them to pick which
  existing object a document merges into: a relation matches on
  `relation_format` together with `name` or `relation_key`, and a TYPE
  document matches on `name` alone, since this format strips `unique_key`
  and the name is then the only filter left. They stay writable
  deliberately. Neither half of this codec reads them — they travel the
  generic details path in both directions — so denying them would force
  export to strip them, and a stripped property that import refuses is a
  lossy export: "Marshal never emits a document its own Validate rejects"
  (§11, I1) is the stronger promise. The guarantee that an imported
  document cannot rewrite an EXISTING relation's or type's identity
  therefore belongs at the object layer, which every writer passes through,
  rather than in this format, which is one writer among several. A
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
- **The reserved spelling is `template` — in both directions, and against
  the reader as well as the writer.** Export keys `template_for` emission
  off the spelled term, validation gates `/template_for` on it, and import
  derives the smartblock kind from it (§2) — all through the *document*
  half of the chain (legend → bundled table → verbatim, no reader
  vocabulary). The document's own **legend** may therefore move the spelling
  freely: it moves the kind derivation and the gate with it, so the two stay
  in agreement. A reader's **vocabulary** may not, precisely because that
  half of the chain cannot see it. A vocabulary answering some other stored
  key for `template`, or landing another spelling on the template key,
  splits the two resolutions of one field: export refuses such an answer
  with a warning and writes the stored key, and import refuses it with a
  warning and uses the document's own answer. Without the import half,
  `{"type": "template", "template_for": "task"}` read through a vocabulary
  that re-binds `template` produced a Template smartblock whose object type
  keys do not contain `template` — invisible to every template check
  downstream, all of which test for exactly that key — and whose re-export
  then dropped the target type outright. No shipped vocabulary can give that
  answer; a hand-written `Options.Keys` can, and this is what stops it.
- **Export writes only the slots §2 models, and says what it drops.** The
  envelope carries one type, plus the target type on a template; entries
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
`icon_emoji` and `icon_image` are attributes of a block, not property keys.
They are the format's own vocabulary and follow the format's own rule (§1
Naming, all snake_case); the vocabulary never touches them, so they would
keep their spelling whatever the `icon_emoji` *property* were called one
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
hand-written `{"iconEmoji": …, "icon_emoji": …}` validated clean and then
failed to import). The two halves agree exactly whenever no wider
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

The rule binds the **spelling**, and the spelling is the slug: a vocabulary's
slug comes from `apiObjectKey`, which is user-supplied or strcase-derived
from the property name with no length bound and no reserved-word check, so
nothing upstream guarantees it is a spelling this format accepts. Export
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
| `indent` | integer ≥ 0 | no | Nesting depth. Absent = `0` (top level); canonical form omits `indent: 0`. Values above **32** fail validation (adversarial-input bound; real documents reach ~6). See the nesting rules below. |
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
object-link targets in `text`, `object_id` props, a callout's `icon_image`
and the `iconImage` property, `objects`/`files` property values, `items`,
view `default_template_id`/`default_type_id`, `object_orders[].object_ids`,
and filter `value`/sort `custom_order` entries of `objects`/`files`
properties — is written in full, on every shape, with no legend.

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
document — enter it as an avoid-set. A short object id spelled in a mention
and a minted block whose suffix equals it would otherwise both answer to one
name in one document. Deleting object compaction made this guard matter more,
not less.

The two shapes the API serves are the two this leaves: API v2 default reads
use block labels (the server resolves them by unique suffix) and keep object
refs full inline, while its export shape — the backup/round-trip shape, whose
bytes must re-import to the same document — keeps block ids full (API spec
C4).

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
orders (§3, §6.2) — which `option_ids` inverts exactly (§9a), leaving two
residues: **two same-named options of one property held by ONE object**
collapse onto the first, because the document spells one string twice; and a
reader wired with no option resolver ignores the legend and keeps the names,
having no space in which an id could be an option at all; deprecated snapshot
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
models — one type, plus the target type on a template — with keyless
entries (`ot-`, `""`) dropped first, so the remaining entries close ranks
rather than lose the slot a keyless one would have silenced (§3); and a type
object gains an empty list for every recommended role nothing occupies —
`type_properties` (§2a) collapses the four role lists into one labelled
array, and import rebuilds all four from it, so a role the store left absent
comes back as `[]`. An absent list and an empty one say the same thing, and
the empty list is the only way this format can express a role being
*cleared*, since `type_properties` cannot name a section that exists with no
members. Whether the object state itself should carry all four consistently
is a question about the state, not the format (GO-7451).

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
   absorbs top-level title/description blocks.)
3. `Export(S) = Export(Import(Export(S)))` — the same guarantee anchored on
   the SNAPSHOT rather than on a document, and the one an object exported
   twice depends on: once directly, once after a round trip through this
   format. It is what §9's "re-exports diff cleanly" means for everything
   that is not an id, and it is why the term census reserves only the keys
   the document spells (§3). Ids are the documented exception in the same
   direction as (2): a snapshot carrying a block or view with no id exports a
   document that is not canonical, and import mints one.

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
  `properties` spelling resolves through the §3 chain before the deny rule,
  the layout-name check and the format-shape warning run; validation
  mirrors the importer's details seam refusal for refusal — a **denied**
  resolved key, an **unwritable** resolved key, and **two spellings binding
  onto one stored key** are all errors — and a `property_keys` *value* is
  admitted like the stored key it is, deny rule included; import re-runs
  the seam's checks on its own resolved key when a wider vocabulary is in
  force; the TYPE namespace mirrors the same way: the `/template_for` gate
  and the kind derivation run on the stored key `type` resolves to through
  the document's own chain — `type_keys` legend first — in validation and
  import alike, a `type_keys` spelling or value gets the same writable-key
  restatement as `property_keys`, and the import seam refuses a term a
  vocabulary resolves onto the empty type key, §3), `language`-vs-`fields.lang` conflicts, an **`option_ids` key naming a
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
  typeproperties.go          — type_properties ↔ recommended lists (§2a);
                               GenerateSchema derived artifacts are planned
                               here (post-v1)
  keyvocab.go                — KeyVocabulary: the stored key ↔ spelling table,
                               both namespaces, and the bundled default (§3)
  blockvocab.go              — the block-type name tables (§5)
  viewvocab.go               — the dataview enum name tables (§6.2)
  fragment.go                — the FRAGMENT surface: one block, a flat run,
                               one property value, the §8 inline codec (below)
  filters.go                 — the fragment surface for a §6.2 filter tree and
                               sorts array, standalone (query paths)
  index.go                   — the bundle index (§2c)
  storeresolver/             — the space-backed implementations of the four
                               resolvers, including KeyVocabulary; the only
                               place that reads a space's api slugs
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
vocabulary listers, and `IsReservedWidgetTarget` /
`IsImportableWidgetTarget` / `IsReservedHomepage` (§2b).

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
3. **Option names vs `{id, name}` objects** (§3): **settled** — names stay in
   the value, generatable and readable, and the id rides beside them in
   `option_ids`, under the property that owns the option (§9a). The
   `{id, name}` value shape was the standing fallback for the
   duplicate-name/rename caveats; the legend closes both without asking a
   generator to write an id it does not have, and without putting a second
   value shape in the slot small models write most. Two spellings for the
   legend were considered at the freeze and rejected: a flat map with a `#`
   separator (deleted at v0.20 — no separator survives contact with arbitrary
   option names and property slugs, §9a), and an `@` sigil marking a handle
   in the value itself (option names may legally begin with `@`, and
   `Validate` takes no format resolver, so it cannot tell an option value
   from an object reference — the sigil would make `Marshal` emit documents
   its own `Validate` rejects, §11 I1).
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
10. **Trim system-property noise** (follow-up): refine §3 "presence is
    meaningful" — keys in `bundle.SystemRelations` are machine-stamped
    metadata (`is_hidden`, `revision`, `relation_format_include_time`, …) and
    could safely omit empty/default values, keeping documents compact for
    LLMs; presence stays preserved for user-intent keys via a small
    exception list (`name`, `description`, `icon_emoji`, `icon_image`,
    `done`) and for every non-system key. Deliberately static (no
    type-schema lookup at export time) to keep the canonical form
    deterministic. Decide `done` membership and wire `buildProperties` +
    the round-trip comparator accordingly.
