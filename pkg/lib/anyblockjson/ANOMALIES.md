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

**Handling**: descendants of file blocks are allowed and round-trip verbatim
(in the flat encoding: blocks indented under the file block; file types are
not in the leaf list). Because the observed case lives in a *table cell*,
this anomaly is also the standing evidence for the cell **array form** —
a cell block with descendants serializes as an array of flat blocks (§6.1)
rather than dropping them. **Spec**: §5 file row, §6.1.

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

## 9. Point lookups and listings disagree, in both directions

The spaceindex relation lookups are mutually inconsistent:

- `GetRelationByKey` misses some relations that `ListAllRelations` still
  returns (observed for custom keys like `artist`, `assignee` in old
  spaces).
- The inverse (run 4): `GetRelationById` resolves a relation that is absent
  from **both** `ListAllRelations` and `GetRelationByKey` — observed for
  ordinary user-created relations (bson-id keys, e.g. a "Tag" property),
  presumably deleted or partially indexed. A resolver that answers by id
  but cannot invert the resulting key breaks the round trip: import passes
  the key through and re-export drops the entry.

Any resolver wired over the object store should prime id↔key maps from the
full listing, use point lookups as fallback, and **cache every point-lookup
hit in both directions** so resolution stays invertible —
`cmd/anyblockroundtrip` does both. Root cause in the index itself is
unverified; worth an issue.

## 10. Template-cloned block ids repeat across objects

The same 24-hex block ids recur verbatim in dozens of distinct objects
(template instantiation preserved source ids; `fields.analyticsOriginalId`
often matches). Harmless for this format — ids are document-scoped — but any
tooling assuming account-wide block-id uniqueness will be wrong.

## 11. Flat encoding: pending sweep verifications (v0.6)

The v0.6 flat-blocks change carried assumptions the flat-encoding sweep
(run 4, 2026-07-23, 35 372 objects) checked; measured results:

- **Cells with descendants**: counter reported **0** across the account —
  the §6.1 array form is implemented losslessly anyway (and #2's historical
  case proves the class exists), but it is effectively absent from current
  data.
- **Tables nested inside table cells**: none — zero `import_error` failures,
  so the non-recursive `cellBlock` restriction (§12) matches reality.
- **Depth histogram** (per-object max indent): 0 → 34 057, 1 → 816,
  2 → 323, 3 → 119, 4 → 37, 5 → 5, 6 → 6, 7–8 → 2, and a long-tail of
  8 outliers at 16–26. The "~6 typical max" datum holds for the bulk; the
  outliers stay comfortably under the 32 bound (`indent` > 32 is a
  validation error).
- **Run 4 result**: 21 failures = 7 accepted duplicate-name option swaps
  (#6) + 14 tool-resolver asymmetry (a relation resolvable by id via point
  lookup but absent from both the listing and the by-key lookup — the #9
  class; export emitted its key, import could not invert it, re-export
  dropped the entry). Fixed in `cmd/anyblockroundtrip` by caching point-
  lookup hits in both directions; format and package unaffected.

---

Final state after all fixes: 35 369 objects, 0 export/import errors, 0
byte-stability failures expected except the accepted duplicate-name option
swaps (#6). The v0.6 flat-encoding rerun (acceptance: pass rate ≥ 99.86%,
no new failure categories) is pending.
