# Real-world data anomalies

Findings from round-trip testing `pkg/lib/anyblockjson` against a production
account (`cmd/anyblockroundtrip`, 2026-07-23: ~35 400 objects across ~48
spaces, three iterations). Each entry records a shape that real snapshots
contain but the clean data model does not predict, and how the format
handles it. Kept separate from SPEC.md: the spec defines the format, this
file explains *why* several of its rules exist and what future importers and
migrations must expect in old accounts.

Legend: **handling** = what export/import does today; **spec** = where the
rule lives.

## 1. Content-less blocks (unset content oneof)

Blocks with `Content == nil` exist in two shapes:

- every legacy `STRelation` / `STRelationOption` object wraps its "used in"
  dataview in a bare content-less block (id `rel-<key>` or a 24-hex legacy
  id);
- ordinary pages contain orphaned content-less leaves with no children.

Volume: 277 objects failed on this before the fix (~67% of all failures in
run 2); 87 distinct block ids.

**Handling**: a childless content-less block is dropped; one with children
exports as a transparent `group` so the subtree (e.g. the relation's
dataview) survives. **Spec**: §7 "Content-less blocks".

## 2. File blocks with real block children

The editor treats file blocks as leaves, but legacy data nests genuine text
blocks under them — observed: a table cell with File content holding four
paragraph children (real user text, including a mention). Dropping them was
*silent* data loss.

Volume: 1 object / 4 blocks — rare but the worst class of bug.

**Handling**: `children` on file blocks are allowed and round-trip verbatim;
the schema's `children: false` for the file family was lifted. **Spec**: §5
file row.

## 3. Recommended-relation lists holding bare property keys

Type objects predating per-space derived relation ids store bare property
**keys** (`"creator"`, `"createdDate"`) in `recommendedHiddenRelations` and
friends, where object ids are expected.

Volume: ~15 objects.

**Handling**: export resolves list entries with a fallback chain — property
id → reverse key lookup → bundle (system properties). On import the entry
comes back as a proper object id, i.e. round-tripping *migrates* the legacy
key to an id. **Spec**: §2a.

## 4. Orphaned pre-CID relation ids in recommended lists

The same lists also hold Mongo-ObjectId-style 24-hex ids
(e.g. `68cdaa41e9223c9dc7ce5f30`) that resolve to nothing in the space —
relics of deleted relations or pre-migration data. Some repeat verbatim
across 38–46 different objects (template cloning preserves source block and
detail ids).

Volume: ~14 objects, concentrated in shared/community spaces.

**Handling**: dropped on export (unresolvable), by design. Round-tripping
cleans them up. **Spec**: §2a.

## 5. `_missing_object` sentinels in object-reference details

Dangling object references are stored as the literal sentinel
`"_missing_object"` (`pkg/lib/localstore/addr`) inside detail lists.

Volume: ~60 objects.

**Handling**: dropped on export like any unresolvable reference. Test
tooling must not count their disappearance as loss. **Spec**: §2a; harness:
`cmd/anyblockroundtrip` filters the sentinel in comparisons.

## 6. Duplicate-named select/multiSelect options

Spaces contain multiple option objects with the same display name for one
property. Because option values round-trip by **name** (§3), import resolves
to one canonical option — objects referencing the duplicate get their option
id swapped.

Volume: 7 objects (`tag` property). **Accepted limitation**, kept visible in
round-trip reports as evidence for the §15.3 open question (names vs
`{id, name}` pairs).

## 7. Default-valued details are semantically present

Details like `isHidden: false`, `revision: 0`, `relationFormatIncludeTime:
false` appear *explicitly* on thousands of objects. Presence of a property
key — even with a default/empty value — records that the property was set on
the object; clients rely on it.

Volume: run 1 flagged 14 032 issue lines on 5 577 objects before the design
call.

**Handling**: `properties` values are written verbatim, including `false`,
`0`, `""`, `[]`, and explicit `null`; the omit-empty canon applies only to
block attributes and envelope fields. **Spec**: §3 "Presence is
meaningful".

## 8. Empty-vs-absent recommended lists

Type objects store the four `recommended*Relations` keys as explicit empty
lists; hand-authored documents may omit them entirely. The two must not be
conflated: run 3 caught a type with *all four* lists empty round-tripping to
absent keys.

**Handling**: type documents always carry `typeProperties`, even as `[]` —
presence of the array is the trigger to rebuild all four lists (empty
sections become explicit empty lists); a document without the field leaves
the lists untouched. **Spec**: §2a.

## 9. Point lookups miss what listings return

`spaceindex.GetRelationByKey` misses some legacy relations that
`ListAllRelations` still returns (observed for custom keys like `artist`,
`assignee` in old spaces). Any resolver wired over the object store should
prime id↔key maps from the full listing and use point lookups only as
fallback — `cmd/anyblockroundtrip` does. Root cause in the index itself is
unverified; worth an issue if it reproduces outside legacy spaces.

## 10. Template-cloned block ids repeat across objects

The same 24-hex block ids recur verbatim in dozens of distinct objects
(template instantiation preserved source ids; `fields.analyticsOriginalId`
often matches). Harmless for this format — ids are document-scoped — but any
tooling assuming account-wide block-id uniqueness will be wrong.

---

Final state after all fixes: 35 369 objects, 0 export/import errors, 0
byte-stability failures expected except the accepted duplicate-name option
swaps (#6).
