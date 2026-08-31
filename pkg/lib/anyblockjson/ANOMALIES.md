# Real-world data anomalies

Findings from round-trip testing `pkg/lib/anyblockjson` against a production
account (`cmd/anyblockroundtrip`): four iterations on 2026-07-23 over
~35 400 objects across ~48 spaces, and a later sweep over a 36 808-object
account during the v0.9–v0.11 work. Which figure belongs to which run, and
what a pass rate measures, is the ledger at the end of this file. Each entry
records a shape that real snapshots contain but the clean data model does
not predict, and how the format handles it. Kept separate from SPEC.md: the
spec defines the format, this file explains *why* several of its rules exist
and what future importers and migrations must expect in old accounts.

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

**Handling**: the block is dropped either way — it is a transparent
container (§7a) — and a subtree under one (e.g. the relation's dataview) is
lifted into its place. It was once written out as a `group` block, and
on 160 of these objects that wrapper was what kept §7's primary-dataview pin
from firing. **Spec**: §7 "Content-less blocks", §7a.

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
**keys** (`"creator"`, `"createdDate"`) in `recommended_hidden_relations` and
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

Volume: 7 objects (`tag` property). **Closed on the default shape, accepted
on the id-less one.** The `option_ids` legend (§9a) carries the id beside the
name and is written unconditionally, so a default export read back into a
space that still serves the id lands on the option the object was actually
on. Two readings still resolve by name and still swap: `OmitIds`, which drops
the legend because an id-less document shipping a map of ids is not one (§9),
and any read into a space that does not serve those ids — where the legend is
a hint that fails its liveness check and falls through by design (§3). §15.3
(names vs `{id, name}` value objects) is settled on the strength of that: the
id rides beside the name rather than inside the value.

## 7. Default-valued details are semantically present

Details like `is_hidden: false`, `revision: 0`, `property_format_include_time:
false` (legacy spelling) appear *explicitly* on thousands of objects. Presence of a property
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

**Handling**: type documents always carry `type_properties`, even as `[]` —
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

## 11. Flat encoding: the sweep verifications (v0.6)

The v0.6 flat-blocks change carried assumptions the flat-encoding sweep
(run 4, 2026-07-23, 35 372 objects) checked; measured results:

- **Cells with descendants**: counter reported **0** across the account —
  the §6.1 array form is implemented losslessly anyway (and #2's historical
  case proves the class exists), but it is effectively absent from current
  data.
- **Tables nested inside table cells**: none — zero failures, so the
  non-recursive `cellBlock` restriction (§12) matches reality. Should such
  data ever appear, Marshal now rejects it loudly (`export_error` in the
  sweep) instead of emitting a document its own validation rejects; the
  depth bound (32) is enforced at export the same way.
- **Depth histogram** (per-object max indent): 0 → 34 057, 1 → 816,
  2 → 323, 3 → 119, 4 → 37, 5 → 5, 6 → 6, 7–8 → 2, and a long-tail of
  7 outliers at 16–26. The "~6 typical max" datum holds for the bulk; the
  outliers stay comfortably under the 32 bound (`indent` > 32 is a
  validation error).
- **Run 4 result**: 21 failures = 7 accepted duplicate-name option swaps
  (#6) + 14 tool-resolver asymmetry (a relation resolvable by id via point
  lookup but absent from both the listing and the by-key lookup — the #9
  class; export emitted its key, import could not invert it, re-export
  dropped the entry). Fixed in `cmd/anyblockroundtrip` by caching point-
  lookup hits in both directions; format and package unaffected.

## 12. Charset-dirty block ids are no longer laundered by relabeling

The schema's block-id pattern is `^[A-Za-z0-9_-]{1,64}$`, but stored ids
are not guaranteed to match it (legacy/imported data could carry other
characters; no live producer found). The earlier charset relabel rule
*accidentally laundered* such an id whenever its 5-char tail was clean —
the served document carried the clean label and validated. Under the
minted-shape rule (`isMintedLocalId`, API v2 Wave 0 hardening) a
non-minted id serves verbatim, so a document holding a charset-dirty
block id now fails its own `Validate` on export. **Handling**: closed, not accepted —
the minted-shape rule decides *whether* an id is compacted, and the export
sanitizer decides how whatever comes out is spelled, so a charset-dirty id
is written as `a_b` rather than verbatim and `table1` keeps its name. Both
rules landed on the same branch and compose; `TestExport_BlockIdOutsideCharsetIsSanitized`
pins the first half and the compact golden pins the second. No such id
appeared in the 2026-07-23 sweeps (~35 400 objects) or the later
36 808-object one. **Spec**: §9a (relabel rule), schema `$defs/blockId`.

## 13. A quarter of all image covers are absolute filesystem paths

`coverId` is declared `longtext` with no constraint, so nothing in the store
or in the format ever checked that a cover marked `coverType: 1` (an image)
names an image object. `core/block/import/notion/api/commonobjects.go` sets
`coverId` to `cover.External.URL` with `coverType: 1`, expecting a later
pass to download the file and rewrite the reference. On **33 objects** of a
36,966-object account that pass never ran, leaving values like:

```
/var/folders/j0/b3km_psx1bd14q06gdpvzk5m0000gn/T/anytype_notion_import/f99972cc….png
```

The temp directory is long gone. That is 33 of the 130 `coverType: 1`
covers — **25% of every image cover in the account** — permanently corrupt,
and nothing reported it, because a `longtext` holding a path is a perfectly
good `longtext`. The same importer writes URLs into `iconImage` by the same
mechanism; no leaked icon survived in this account (0 of 12,011).

**Handling**: refused rather than carried. §2b's `cover.file` is an object
reference (`^[^/]+$`), so the value cannot be written — and carrying it
would make `Marshal` emit what its own `Validate` rejects (I1). Export drops
the cover with a warning naming the value, which turns permanent silent
corruption into a named event, and `snapshotdiff` still reports it as loss:
66 findings over those 33 objects, the only loss the icon/cover collapse
causes. **Spec**: §2b, §11 `N(S)` clause (e). The importer bug is a separate
ticket — this file records the data, not the fix.

Two smaller ones from the same census, both handled rather than open:
`iconOption` holds 12, 13 and 15 on six objects, because
`core/block/import/pb/converter.go` mints `rand.Intn(16)+1` while
`core/block/import/markdown/schema.go` mints `rand.Intn(10)+1` for the same
ten-colour palette — §2b's integer colour escape carries them. And 54
objects hold the bundled `iconEmoji` (empty) beside a **space-minted
relation whose own stored key is literally `icon_emoji`**, holding a real
emoji; anything reading "the icon" out of `icon_emoji` in those documents
reads user data. §2b's lift separates the two visibly.

---

## The compact goldens no longer differ from the plain ones

**Status: open, recorded rather than fixed — and now measured.** With
object-ref compaction deleted, `CompactIds` selects only block-label
relabeling, and the rich fixture's block ids are all short or hand-authored
(`b1`, `dv1`, `v1`, `table1`), none of them minted-shaped, so none relabels.
Two of the four goldens therefore freeze nothing the other two do not.

**Measured**, not inferred, at `35ec288e6` — `cmp` plus `shasum -a 256` over
`pkg/lib/anyblockjson/testdata`:

| golden | bytes | sha256 (first 16) |
|---|---|---|
| `rich.json` | 4694 | `3ed93ddf8025c87b` |
| `rich_compact_ids.json` | 4694 | `3ed93ddf8025c87b` |
| `rich_omit_ids.json` | 3669 | `ce96eee96aea3d4b` |
| `rich_compact_omit.json` | 3669 | `ce96eee96aea3d4b` |

Both pairs are byte-identical — re-verified after the raw-name
regeneration (`cmp`: 4,846 bytes and 3,821 bytes per pair). While that
holds — i.e. while no id in the
rich fixture is minted-shaped — a change to the relabel rule *alone* produces
**zero** golden drift, and zero drift is exactly what reads as "the goldens
saw it and it was fine". That is the misleading part, not the duplication.

The path is not uncovered — `TestExport_MintedShapeRelabeling` and
`compactsplit_test.go` both pin block relabeling against minted ids, and
`TestExport_CompactIdsIsAnAliasForBlockLabels` pins the alias against a
fixture that does relabel. So this is a redundancy in the golden set, not a
hole in the coverage.

**Recommendation (not performed — goldens are load-bearing here, and this is
a maintainer's call).** If it is closed, close it with a *new, small* pair —
a two-block fixture carrying one minted-shaped id and one meaningful id, with
its own plain and `CompactIds` goldens — rather than by minting an id into
`rich`. Minting into `rich` moves block ids in all four existing goldens for a
property that has nothing to do with what `rich` is for (a frozen picture of a
hand-authored document exercising every block type), and it produces a golden
diff that has to be justified line by line for lines that carry no meaning. A
dedicated pair differs by exactly the one thing the flag does, which is what a
golden is worth freezing. Leaving it as it stands is also defensible: the
named tests above carry the rule, and this entry is the note that keeps the
zero-drift signal from being read as coverage. **Spec**: §9a.

---

## No golden carried a `property_internal_keys` legend

**Status: closed by the exhaustive legend.** The four goldens used to hold
bundled or verbatim keys only, so the §3 legend never appeared in a frozen
document and the goldens proved nothing about it — including nothing about
its canonical position relative to `option_ids`, which is keyed by the
spellings `property_internal_keys` inverts. Since the legend became
exhaustive — one entry for every spelling the bundled table does not bind
— the rich fixture's two custom keys earn their entries, all four goldens
now freeze a two-entry `property_internal_keys`, and the two id-bearing
goldens freeze its canonical position before `option_ids` (see
`testdata/rich.json`). `TestOptionRefs_TheLegendFollowsPropertyKeys` still
pins the ordering independently of the fixtures. **Spec**: §2, §4.

---

## Run ledger

Four object counts circulate in this file and in the package, because the
account grew between sweeps. Each belongs to one run, and none of them is a
statement about the others.

What a pass rate measures here: `snapshotdiff` compares detail values (up to
the documented normalizations) and the plain text of text blocks as a
multiset, and the harness compares the re-exported bytes with the exported
ones. Marks, block order, table shape, dataview content and file/bookmark
metadata are not compared, so a systematic loss in any of them is
byte-stable and invisible to the number — a blind spot the pre-freeze
review first named, and the comparator now says itself: its findings are
triage input, not proof.

- **Runs 1–3** (2026-07-23, ~35 400 objects across ~48 spaces, pre-flat).
  Run 1 flagged 14 032 issue lines on 5 577 objects, all default-valued
  details (#7); run 2 failed 277 objects on content-less blocks, ~67% of its
  failures (#1). Final state after those fixes: 35 369 objects, 0
  export/import errors, 99.86% byte-identical, the remainder the accepted
  duplicate-name option swaps (#6). That 99.86% is a pre-flat number and
  belongs to run 3 alone.
- **Run 4** (2026-07-23, 35 372 objects) is the v0.6 flat-encoding sweep;
  its results are §11 — 21 failures = 7 accepted swaps (#6) + 14 from a
  resolver asymmetry in the harness (#9), fixed there. This file used to
  close by calling that rerun pending — a line added by the same commit that
  recorded §11's measured results, so it was stale on arrival. The run
  happened: 21 failures in 35 372 objects is 99.94%, above the ≥ 99.86% bar
  that line set, with neither failure category new.
- **The 36 808-object sweep** (August 2026, during the v0.9–v0.11 work) has
  **no pass rate recorded anywhere** — only the findings it produced: 59
  objects failing their own export on the envelope `key` charset (SPEC §2's
  deny rule; `TestValidate_EnvelopeKeyAcceptsRealStoredKeys` carries the
  real keys), 12 objects whose dataview came back pointing at another
  property (SPEC §3's `property_internal_keys` legend; `storeresolver/keyvocab.go`),
  two date-filter documents export emitted and validation rejected
  (`datefilter_test.go`), and 10 378 false data-loss issues caused by the
  harness's own stale copy of the internal-property list (now
  `InternalPropertyKeys`, `export.go`).
- **Nothing since has been measured against real data.** Every v0.9–v0.11
  fix is demonstrated by unit test, not by a sweep — the 12 re-pointed
  objects have not been swept again since the guard landed — and no sweep is
  recorded against the package as it stands after them.
