# The native AnyBlock JSON exporter — design

Status: IMPLEMENTED (GO-7383, 2026-08-26). §1's pipeline is live:
collection behind `core/block/export/collect` (Closure replacing
`isProtobuf`), composition in `pkg/lib/anyblockjson/compose` (Q10 option b;
the roundtrip harness runs the same code), the exporter wiring in
`core/block/export/anyblock`, and the manifest `files` map as SPEC v0.47
(Q4 option a). Q5 taken as (a), Q8 settled to `.anyblock.json`, Q9 as (a)
via the per-bundle `BundleRoot` prefix. Q11 shipped as close-after-write
with the release gate named at the call site (any-sync PR #769, the
GO-7333 fix, must land first — anyblock.go). Still open, deliberately:
**Q6** (the `Export_AnyBlockJSON` RPC enum — until it lands the exporter
is driven through the Go API and cmd tooling) and **Q7** (default-backup /
pb retirement, a product call). One deviation from §1.1: the manifest
type-path table accumulates at EMIT from actually-written documents rather
than being pre-built at plan — provably consistent with the output (a doc
whose emit fails never enters the manifest), and determinism is unaffected
since finish sorts.

Scope: the production exporter that writes an AnyBlock JSON bundle (SPEC.md
§2c) from a live space — the replacement for the writing half of
`core/block/export`, sitting on the extracted collection half. The
architecture decision (keep collection, replace writing, target shape
`core/block/export/{collect,writer,anyblock}`) is taken and not re-argued
here; this document decides layout, naming, blob handling, the seam, and
concurrency.

Evidence base: the code cited by `file:line` throughout, and a 77-space
production corpus sweep (38,105 source objects, 28,542 emitted documents)
measured with Python for this document. Corpus numbers below are from that
sweep unless said otherwise.

---

## 1. Proposed design

### 1.1 The pipeline: collect → plan → emit → finish

Four phases, two of them new relative to today's exporter:

| phase | threading | does |
|---|---|---|
| **collect** | as today | dependency closure over the request: nested objects, dataview-referenced objects, types, relations, options, templates, linked files, recommended relations (`processProtobuf`, export.go:610). Extracted behind a format-agnostic interface; the bare `isProtobuf bool` (export.go:503-504) becomes an explicit closure mode. Output: `map[id]*Doc`, complete before anything is written. |
| **plan** | single-threaded | classify every collected doc (kind → directory, §1.2), compute every filename (§1.3 — a pure per-document function of the id, no collision machinery), and pre-build the manifest type-path table (stored type keys come from the `uniqueKey` detail). **Plan reads details only — id, name, type/layout, uniqueKey — never content**; that invariant is what keeps it O(collected details) in memory and free of object loads (§1.6). Cheap: map passes over details already in memory, no store reads, no marshal. |
| **emit** | width-bounded concurrent queue tasks (§1.5; the queue is already width-4 today, export.go:152-156) | per document: load state, run the omission predicates on the loaded snapshot (`OmittedBundledRelation`, `OmittedSpaceSettings`, `OmittedWidgetObject`, `OmittedProfilePage` — omittedrelation.go:151, spacesettings.go:156, widgetobject.go:359, profilepage.go:40 — they take the snapshot base, so they CANNOT run at plan time), lift-or-`anyblockjson.Marshal`, write to the planned filename, close (§1.5); for file objects, stream the blob (§1.4). Accumulates bundle facts (installed keys, dictionary entries, option vocabularies, index lift, used property keys) into a mutex-guarded composer. A name planned for a document emit then omits simply goes unused — determinism is unaffected, since omission is itself a deterministic function of state. |
| **finish** | single-threaded, at the `postProcess` seam (export.go:1529) | compose and write `properties.json` and `index.json` (with manifest), re-reading both through the package's own `Unmarshal` before writing — the bundle-level I1 discipline the harness already practices (cmd/anyblockroundtrip/main.go:983-1012). |

The composer is a production re-home of the harness's `spaceComposer`
(cmd/anyblockroundtrip/main.go:711-1030), which already implements the §2f
composition end to end: installed-key census, divergent-entry override,
option vocabulary with `orderId` ordering, index lift from the omitted
space-settings and widget documents, manifest, and the re-read check. What
moves is the code's home and its input source (in-memory states instead of
`.pb` files on disk), not its logic.

One piece cannot move as-is: `anyblockbatch.UsedPropertyKeys` reads written
files back from disk (cmd/internal/anyblockbatch/scan.go:908). A zip export
cannot re-read its own entries (zipWriter has no read path, writer.go:130),
so the used-key scan must run on the marshalled bytes **before** they are
written. The scan logic should be promoted from `cmd/internal/anyblockbatch`
into a place production code may import (a byte-level
`UsedPropertyKeysFromBytes` in `pkg/lib/anyblockjson` or a small exported
subpackage), keeping the cmd tools on the same single implementation.

### 1.2 Directory layout (question a) — SETTLED: kind directories, `objects/` flat

The importer never dispatches on directory names — its only path rule is
skipping `files/` (import/pb/converter.go:38, 338-341); classification is by
the document's own declared kind/type (import/pb/converter.go:337 onward),
and SPEC.md:2319 states outright that "the format defines no folder layout —
`objects/`, `types/`, `relations/` are one exporter's convention". So the
layout is chosen for the human opening the bundle, and for consistency with
the format's own vocabulary — which spells everything snake_case and never
says "relation" (SPEC §1 Naming, SPEC.md:1668; PRINCIPLES rule 3).

Proposed layout, one bundle root per space:

```
<root>/
  index.json          — the bundle index + manifest (SPEC §2c; index.go:30)
  properties.json     — the property dictionary (SPEC §2f; dictionary.go:47)
  objects/            — kind: page (and any kind without a dedicated home,
                        e.g. the rare fail-closed widget document — 1 in
                        28,542 corpus docs). FLAT — no type subdirectories
                        (settled; type grouping belongs to the later
                        human-readable mode, §1.3)
  types/              — kind: object_type
  templates/          — kind: template
  properties/         — kind: property — only the KEPT documents (divergent
                        installed copies and space-minted properties; the
                        rest are omitted into the dictionary per §2f)
  options/            — kind: property_option
  participants/       — kind: participant
  files/              — kind: file_object documents AND their blobs,
                        adjacent (§1.4)
```

Rationale, against the legacy names (export.go:96-103):

- **Format vocabulary, not store vocabulary.** `relations` →
  `properties/`, `relationsOptions` → `options/`, matching the kinds the
  documents themselves declare (`kind: "property"`, `"property_option"`).
  The format promised "`relation` appears nowhere" (PRINCIPLES rule 3); the
  directory a reader sees first should keep that promise too.
- **snake_case / single words.** `filesObjects` and `relationsOptions` are
  camelCase compounds in an archive whose every document member is
  snake_case. All proposed names are single lowercase words, sidestepping
  the case question entirely.
- **`participants/` is new.** 2,492 participant documents are 8.7% of the
  corpus and today land in `objects/`, where they bury real pages (the
  median space has 78 documents total). They are machine-derived membership
  records; giving them their own room keeps `objects/` browsable.
- **`files/` holds both halves of a file** — see §1.4. This deletes the
  legacy `filesObjects`/`files` split, which forced a human to correlate
  two directories by id.
- **Kind counts justify the split**: file_object 10,254 · page 9,688 ·
  property_option 2,641 · participant 2,492 · object_type 1,760 · property
  1,215 · template 491 across the corpus. Every proposed directory earns
  its place in a real account; none is speculative.
- **No `profile` file.** The raw-protobuf `profile` is an install artifact
  of the `ObjectImportExperience` path and is written by `cmd/anyblockconvert`
  when preparing an installable experience (SPEC §2c "How it reaches the
  space"); a native backup bundle carries the same facts in `index.json`.
  The legacy exporter's `createProfileFile` (export.go:1316) does not carry
  over.
- **No `index.pb`-style home special case.** Legacy writes the home object
  as `index<ext>` at the root (export.go:1267-1268) — which for a JSON
  format collides head-on with `index.json`. The native bundle records the
  homepage in `index.json` (`homepage`, SPEC §2c) and the home object is an
  ordinary document under `objects/`.

Multi-space export keeps the `spaces/<spaceId>/` wrapper (export.go:96,
1381-1385), each space directory being a complete self-contained bundle root
with its own `index.json` and `properties.json`. The wrapper is also what
keeps id filenames collision-free across spaces: the same id legitimately
recurs in several bundles — 448 cross-space repeats measured in the
corpus, chiefly participant identities exported into every space the
member belongs to — and each lands in its own bundle root. A reader who
flattens a multi-space export into one directory WILL hit real filename
collisions; the per-space root is load-bearing, not cosmetic.

**The kind-split tension, resolved.** With id filenames (§1.3), id→path is
a pure function only WITHIN a directory; a reference does not say which
kind its target is, so resolving an arbitrary id against this layout is a
probe over the seven kind directories. Position taken: **the bounded probe
is acceptable, and the rule is stated plainly** — "a document is
`<dir>/<id>.anyblock.json` for exactly one of the seven directories; to
resolve an id, check them in order". The probe constant is 7, fixed by this
design, independent of space size. In practice it is not even a probe: a
zip reader holds the archive's entire central directory in memory, so
resolving any path is one map hit regardless of folders; on a filesystem it
is at most 7 stats, and any reader resolving many references builds a full
id→path map in one walk (7 readdirs) — which it must be able to do ANYWAY,
because the layout is one exporter's convention (SPEC.md:2319) and an
authored bundle may put its documents anywhere, so a general reader walks
and indexes regardless (`DiscoverJSONFiles`,
cmd/internal/anyblockbatch/scan.go:266). What the kind split still buys
once filenames are opaque: `files/` blobs separated from documents, kind
tallies visible at a glance when debugging an export, and the enumeration
of each kind without opening anything. The genuinely-flat alternative
(every document in one directory — a strictly purer id→path function) is
recorded in §2; it remains cheap to adopt later precisely because nothing
dispatches on directories.

Authored bundles (consumer 2, SPEC.md:1543) are unconstrained by all of
this: a hand-written bundle may put documents anywhere, because nothing
resolves by path except through the manifest the author writes. The layout
above is what OUR exporter emits, recorded in SPEC §2c's "one exporter's
convention" slot.

### 1.3 File naming (question b) — SETTLED: `<id>.anyblock.json`

**The rule.** Every document is named by its envelope id, verbatim:

```
objects/bafyreickzryfg6w3srlo3tlirqkftg7rhhgaxzpnjmuwv5kg7hjebd2j3u.anyblock.json
types/bafyreihayoh64xvkp2rdr34eudnwoht36d5cdmii465v5y7haojvdyu534.anyblock.json
files/bafyreigp3himcyqenxemyyk3iu7qtnmlglu4qgnn3r63b743ytzyp6hpv4.anyblock.json
participants/AAjEaEwPF4nkEh9AWkqEnzcQ8HziBB4ETjiTpvRCQvWnSMDZ.anyblock.json
```

This is what the harness already writes (cmd/anyblockroundtrip/main.go:377).

**The argument that settled it** (human decision, 2026-08-26; a hybrid
`<slug>--<id8>` scheme was the standing proposal and was overturned): a
reference inside a document carries an **id and nothing else** — a link
block's `object_id`, a mention target, an option id in `option_ids`, every
manifest key. The format addresses everything by id, on every shape (SPEC
§9a: "object references are never compacted"). With any name-bearing
filename there is **no way to get from a reference to its file except
scanning documents**; with `<id>.anyblock.json` the mapping is a pure
function of the reference itself. The human's framing: the format is
already transparent — "we have ids everywhere" — and the bundle's main
consumer here is machine reading with clear rules; an authoring agent
minting a use case can even choose ids that ARE its filenames. Legibility
of the LISTING is deliberately traded away in this mode and comes back
whole in a later mode (below).

**Why ids are safe, measured.** Corpus ids are exactly two populations:
26,050 ids of 59 chars (lowercase-base32 CIDs) and 2,492 of 48 chars
(base58 participant identities). Their combined character set is
`1-9 A-H J-N P-Z a-k m-z` — no `0`, `I`, `O`, `l`, no path-hostile
characters, no Unicode, no normalization surface, no Windows reserved
stems, no length hazard (59 + 14 = 73 bytes per component maximum, under
the 255-byte limit; worst full path with `spaces/<59-char id>/objects/`
prefixes ≈ 150 chars, under Windows' 260 default). Uniqueness is by
construction (ids are unique per space; measured: zero duplicates within
any of the 77 bundles). Case-insensitive filesystems are covered by two
different arguments, one per population, and the distinction matters: the
59-char CIDs **cannot** case-collide structurally — their alphabet has no
uppercase, so folding is the identity function on them; the 48-char
identities ARE mixed-case, so a fold collision is not structurally
impossible for them — merely astronomically improbable (two distinct
identities would have to differ only in the case of their letters), and
**zero occur across the 2,492 measured** (true case-fold collisions within
a bundle across all 28,542 ids: 0). Determinism is free: the
name is the id, no collision machinery, no global set needed — which also
retires `namer.Get`'s `rand.Int63n` nondeterminism (export.go:1435, 1462)
without replacing it with anything.

**Two bonuses, both secondary to the argument above:** archives are
rename-stable (a renamed object keeps its path, so backup diffs show only
the content change — the same property SPEC §9 chose for `RefNames`,
default off, "the backup shape stays minimal and rename-stable"); and the
plan phase (§1.1) no longer performs collision resolution at all — the path
is a per-document pure function, and plan's remaining naming job is just
the manifest table.

**The later human-readable mode — one mode, not designed now.** Readable
output is an export MODE to be added later, and it bundles **both**
readable filenames **and** type-subdirectory grouping under `objects/` into
one switch — one rule per mode, nothing half-legible. The default mode must
not foreclose it, and does not: nothing dispatches on paths
(import/pb/converter.go:338-341; SPEC.md:2319), and the manifest carries
whatever paths the writing mode chose, so the two modes differ only in the
exporter's path function. Facts already in hand for whoever designs it
(measured; do not re-derive):

- The gain is thin for most spaces: ordinary objects per space median 20
  (max 2,001), distinct types per space median 3 (max 42) — grouping 20
  objects into 3 folders — and 115 of 359 type directories (32%) would
  hold ≤ 2 objects.
- Type-name slugs are safe as directory names, unlike object names: zero
  within-space type-slug collisions, one type name with a path-hostile
  character (object names: 7.5% hostile, §2).
- Directories must be named by the resolved type NAME slug (stored key →
  type document → name), never by the `type` wire spelling: **617 ordinary
  objects (6.4%), across 52 distinct types, spell their `type` as a bson
  key** (e.g. `69346f554c932bae256cbd02`) — 52 opaque hex directories
  otherwise.
- The name-hazard table and slug-collision measurements in §2 apply to its
  filename half.

### 1.4 File blobs, and finding them (question c)

Two problems must be solved together: how the bundle binds a `file_object`
document to its bytes, and how a human finds both. The thing being replaced
is the `source`-clobber: legacy export stuffs the archive-relative blob path
into `bundle.RelationKeySource` (export.go:1236, second site export.go:1196),
overwriting a real, user-facing, editable `url` relation named "Source"
(pkg/lib/bundle/relations.json:930-935) that bookmarks legitimately hold —
the corpus's very first sampled bookmark carries a real URL there — and the
pb importer reads the path back out of the same key
(import/pb/converter.go:404-414). A document member may not be a slot for
archive bookkeeping; that is the lesson, and neither alternative below puts
a path into the document.

Facts that shape the design: 10,254 file objects (36% of all corpus
documents; median 25 per space, p90 505, max 2,242). Every one carries
`name`, `file_ext`, `file_mime_type`, `size_in_bytes` in `properties` —
but `file_ext` is dirty as a path component: 431 empty, 9 longer than 10
chars, dozens non-alphanumeric (`0-rc01`, `9-alpha` — shrapnel of versioned
library filenames), and 12 literally `json`. SPEC §15 #20 (SPEC.md:6545)
fixes the bundle as FAT — the bytes travel, no `fileVariantKeys`, no
encryption keys — and this design carries bytes and nothing else; the thin
bundle's future marker slot is left untouched.

**Alternative A — adjacency convention.** The blob sits beside its document
in `files/`, same stem, real extension:

```
files/bafyreigp3himcyqenxemyyk3iu7qtnmlglu4qgnn3r63b743ytzyp6hpv4.anyblock.json  ← the document
files/bafyreigp3himcyqenxemyyk3iu7qtnmlglu4qgnn3r63b743ytzyp6hpv4.png            ← the bytes
```

Binding rule: same stem, the one sibling that is not `*.anyblock.json`.
(Under §1.3's settled id naming the stem is the file object's id, so
adjacency and the pure id→path function coincide: doc and blob are both
direct functions of the id, differing only in extension.)
Human answer: the two files sort adjacent in any listing — nothing to
correlate, nothing to open. Nothing is added to any document or index.
Weaknesses: the rule is a convention a reader must know (exactly what
SPEC.md:2319 says the format refuses to define); the blob's extension must
be sanitized (empty/dirty `file_ext` → derive from `file_mime_type`, else
`.bin` — three-step rule where Alternative B needs none); an authored bundle
is forced into the same layout to be understood; and "exactly one non-doc
sibling per stem" is an invariant only tooling can police.

**Alternative B — the manifest binds blobs.** `index.json`'s manifest —
whose charter is precisely "where to find what a reader must resolve by key
or id rather than by walking" (SPEC §2c, SPEC.md:2316-2319) — gains a third
member beside `types` and `properties`:

```json
{ "manifest": {
    "types":      { "task": "types/bafyreihayoh64….anyblock.json" },
    "properties": "properties.json",
    "files":      { "bafyreigp3him…": "files/bafyreigp3him….png" } } }
```

One entry per file object: object id → archive-relative blob path. This is
the lookup the deleted manifest `options` map never had a reader for
(SPEC §2c, removed v0.46) — but blobs have exactly that reader: every
importer holding a file_object document must find its bytes, and every
export tool must enumerate them. The paths are free: an authored bundle
writes `"files": {"logo": "assets/logo.png"}` against its own slug ids and
any layout it likes, which is what makes consumer 2 (SPEC.md:1543) a
first-class citizen rather than a convention-follower. An absent blob for a
declared file object — or an entry pointing outside the bundle — is a
cross-document refusal in `anyblockvalidate`, like every other manifest
path (SPEC §2c). Cost: manifest weight (max observed 2,242 entries ≈
200 KB in the heaviest space — noise next to the blobs themselves) and one
indirection when browsing by hand.

**Recommendation: B as the mechanism, A as the layout.** The manifest
`files` map is the authoritative binding — the only rule a reader needs,
author-writable, layout-free, and refusable by tooling. Our exporter then
CHOOSES to lay blobs out adjacently (same directory, same stem — the file
object's id under §1.3, a readable stem again in the later human mode —
sanitized real extension), so doc and blob always sort side by side —
but adjacency is convention, carried by the map, never load-bearing. No
fallback stem-matching in the reader: one mechanism, or the two drift. The
document itself carries no path, `source` keeps meaning what its relation
says it means, and the round-trip comparator never has to special-case an
archive artifact out of the diff.

Blob extension sanitation (cosmetic only, since the map binds and
`file_mime_type` travels in the document): `file_ext` lowercased and
restricted to `[a-z0-9]{1,10}`; failing that, the conventional extension
for `file_mime_type`; failing that, `bin`. `anyblock.json` as a computed
final suffix is impossible by construction (`[a-z0-9]{1,10}` admits no dot).

### 1.5 Concurrency and determinism

The constraint: `writeDoc` runs as concurrent queue tasks
(export.go:426-428), while the dictionary accumulates across every document
and the manifest accumulates paths. Three options were on the table:

- **Single-threaded compose** (what the harness does). Simplest, but the
  emit phase is where all the I/O and marshal cost lives — the biggest
  observed space is 4,884 documents plus 2,242 blobs, and an all-spaces
  export is 38k documents — and serializing it is a real regression
  against today's exporter for zero correctness gain.
- **Two-phase (emit concurrently, compose from re-read files)**. Dies on
  the zip writer: entries cannot be re-read before `Close`
  (writer.go:130-148), so composition facts must be captured in-process
  anyway — the re-read variant only works for directory exports and would
  fork the code path.
- **Plan/emit/finish with a mutex-guarded accumulator** — chosen. The plan
  phase (§1.1) removes the one ordering-sensitive computation — filenames —
  from the concurrent section entirely: under §1.3 each name is a pure
  function of its own id, and the plan fixes the manifest tables before
  the first task starts (`namer.Get`'s nondeterminism is retired with the
  naming scheme itself, §1.3). What remains shared during emit is commutative map/set insertion
  (installed keys, dictionary entries, option vocabularies, used property
  keys, the index lift) — guarded by one mutex, held for microseconds per
  document against marshal work measured in milliseconds (the §9a census
  alone is 4.2 ms → 6.7 ms on a 1,630-block document, SPEC.md:5200), so
  contention is noise. Determinism of the OUTPUT never leans on write
  order: `finish` sorts everything it writes, and the package's canonical
  marshallers already sort keys and refuse unstable forms (SPEC §2c/§2f;
  I1, SPEC §11).

Writer-level concurrency is already safe: `zipWriter.WriteFile` serializes
on its own mutex (writer.go:130-131), and `dirWriter.WriteFile`
(writer.go:66) needs no lock because the plan guarantees distinct paths.
The zip's per-entry `Modified` timestamps come from document state
(`lastModifiedDate`, export.go:1272), not from the clock, so archive bytes
stay stable; the one clock leak is the archive's own root/temp name
(`Anytype.20060102.150405.99`, writer.go:30), which names the artifact, not
its contents.

**Bounded width.** The export queue is ALREADY width-bounded — `NewQueue(…,
4, …)` (export.go:152-156; process/queue.go:42-44, 94) runs at most 4 tasks
at once, and the N queued tasks are thin closures, not loaded objects. So
in-flight marshal work is capped today; the unbounded term is cache
retention, not task count (§1.6). The width itself should follow the repo's
existing prior art for exactly this problem: the reindex limiter caps
cross-space passes at **2 on mobile, 4 on desktop**
(`maxConcurrentSpaceReindexFor`, core/indexer/reindexlimiter.go:15-20,
wired at indexer.go:123), with a rationale comment describing precisely our
situation — "each pass cold-builds every … object into the space's object
cache and relies on the cache TTL to release it, so … the resident set
peaks at hundreds of MB" (reindexlimiter.go:8-14). Recommendation: keep 4
on desktop (matches today's export behaviour), drop to 2 on
mobile, same platform switch. A width cap trades wall-clock for peak RAM;
the trade is cheap here because emit is storage-read-bound (the same
argument reindexlimiter.go:12-13 makes), so halving width on mobile costs
much less than 2× wall-clock while halving the in-flight term.

**Close after write — active, immediate, TTL-independent.** The cache has
no refcount, and its PASSIVE path is TTL-only — `GC()` closes entries where
`isActive() && lastUsage.Before(now-ttl)` (any-sync
app/ocache/ocache.go:343-355, entry.go:42-46; `WithTTL(60s)` +
`WithGCPeriod(time.Minute)`, objectcache/cache.go:80-85) — so an exporter
that does NOTHING retains **throughput × 60-120 s** of loaded CRDT trees:
4 workers at a few ms per document sustain hundreds of documents per
second, thousands of resident trees for a big space, each far larger than
its exported JSON. But the exporter need not do nothing, and the active
path does not wait for the TTL at all (verified): `ocache.TryRemove`
delegates to `e.value.TryClose(c.ttl)` (ocache.go:250-271), and
`smartBlock.TryClose` **ignores the TTL argument entirely** — its whole
body is `TryLock()`-or-fail, `IsLocked()`-or-fail, else `closeLocked()`
(core/block/editor/smartblock/smartblock.go:1153-1162), where `IsLocked`
counts sessions with an ACTIVE event sender, i.e. clients that currently
have the object open in the UI (smartblock.go:633-641). **An object closes
iff nobody else has it open, immediately, regardless of TTL or last-usage
time.** The repo's bulk-walk precedent already works this way: the
fulltext indexer opens the object, extracts, then calls
`TryRemoveFromCache(ctx, objectId)` and logs rather than fails on error
(core/indexer/fulltext.go:330-352, the call at :348;
core/block/service.go:222-234 → objectcache/cache.go:161). The emit task
ends the same way: write, observe, `TryRemoveFromCache`, log-only on
failure. No TTL or GC-period tuning appears anywhere in this design — both
are irrelevant once the exporter closes actively; TTL remains only as the
backstop that collects the failure cases below.

The failure modes, all benign and all bounded:

- **`TryLock` fails** — someone holds the lock at that instant. Transient;
  skip it, no retry loop, the passive TTL collects it later.
- **`IsLocked` true** — the user genuinely has the object open. Not
  evicting it is the CORRECT behaviour, not a limitation; the term it
  contributes to peak RAM is user-bounded (§1.6), never space-bounded.
- **Two editors refuse to close unconditionally** — `SpaceView.TryClose`
  (core/block/editor/spaceview.go:141) and `accountObject.TryClose`
  (core/block/editor/accountobject/accountobject.go:340) both
  `return false, nil`. Both are singletons, and measured across all 28,542
  exported corpus documents there are **zero** `space_view` and zero
  `chat` documents in an export — noted for completeness, irrelevant to
  export memory.
- **`chatobject.storeObject.TryClose`** refuses while a subscription is
  active (core/block/editor/chatobject/chatobject.go:635-649) — again
  user-bounded, and again a kind exports do not carry.

**The GO-7333 dependency, named.** The recommended path reaches a known,
filed-but-unfixed any-sync bug: `ocache.TryRemove` on an entry still in
`entryStateLoading` can nil-deref at ocache.go:271 — which is exactly the
`e.value.TryClose` call this design leans on — or hang permanently
(`setClosing` flips loading→closing but `e.load` is never closed, so later
waiters block forever), and races on `e.value`. It is a race window — the
indexer has been calling this on every indexed object without it obviously
firing — but a high-volume exporter would be the third and heaviest
caller. The exporter's own call happens after its own `cache.Do` returns,
so the entry it evicts is loaded, not loading; the race arms only when
another caller (UI open, sync) re-loads the same object concurrently.
Weighed in Q11; the design assumes close-after-write and names the bug as
its dependency.

### 1.6 The memory model

Hard constraint: exporting a huge space must not hold all object content in
memory — peak RAM bounded by a window, not by space size. The pipeline's
terms, honestly labelled, with measured sizes (corpus: 77 spaces, 28,542
documents; the worst single space 4,884 documents):

**O(all objects) — retained for the whole export, and accepted:**

- **Collected details.** `Doc` holds only `*domain.Details`
  (export.go:200-204); content is never resident in the collection —
  blocks/state load per-document inside the emit task via `cache.Do`
  (export.go:1222) and go out of scope after the write. Measured: **16.9 MB
  of `properties` JSON across all 28,542 documents, 593 B average**; as
  in-memory proto Structs with Go map overhead, budget several times that —
  call it a few tens of MB for a 38k-object account. This term is inherent
  to computing the dependency closure and the plan, and it is the price
  this design knowingly pays. (`transformToDetailsMap`, export.go:227-235,
  re-wraps the same `Details` pointers, not copies — and the native emit
  path does not need `SetKnownDocs` at all, since references print as full
  ids with resolvers wired from the store.)
- **The plan table** (§1.1): one path string + kind per document, ~100 B
  each — ~3 MB at 28k documents.
- **The composer aggregates** — and never the documents. What emit retains
  per document is: used property keys (a set; a space USES a median 57
  keys, SPEC §2f), installed bundled keys, divergent dictionary entries,
  option vocabularies, type→path manifest entries (22 types/space median,
  SPEC §2c), and the index lift. The proof of size is the files these
  aggregates become: measured per space over the sweep, `properties.json`
  is median 13.4 KB, p90 26.8 KB, **max 120.8 KB**, total 1.42 MB across
  all 77 spaces; `index.json` median 2.6 KB, max 8.3 KB. The aggregate is
  three orders of magnitude below the content that streams through it.

**O(in-flight window) — the content term, and the cap is the whole lever:**

- **Loaded CRDT trees in the object cache.** A loaded smartblock holds the
  full change history — far larger than its exported JSON. Because §1.5's
  close-after-write is immediate and TTL-independent (an object closes iff
  nobody else has it open, smartblock.go:1153-1162), the resident content
  set is **≈ the emit width, exactly** — at most N export-loaded objects at
  any instant, where N is the concurrency cap. Peak content RAM is
  therefore **not** O(throughput × TTL) and **not** proportional to space
  size; the cap is a real design parameter with a clean meaning — "at most
  N objects resident for the export" — not a throughput guess. That is
  what justifies the default: 4 on desktop / 2 on mobile per the
  reindex-limiter precedent (§1.5), i.e. at most four trees plus their
  marshal buffers in flight.

**O(UI-open objects) — user-bounded, not space-bounded:**

- Objects the close call correctly refuses: locked-right-now (transient),
  UI-open (`IsLocked`, smartblock.go:633-641), the always-refusing
  singletons and subscribed chat stores (§1.5 failure modes — and the
  corpus shows zero of those kinds in any export). This term scales with
  what the user is looking at, never with what is being exported.

**O(width) spikes — bounded but non-trivial, stated:**

- **Marshalled document buffers.** Each emit worker holds ONE loaded state
  plus its marshalled bytes (kept briefly for the used-key scan, §1.1)
  before write. Measured: blocks total 153.1 MB JSON at 5.4 KB average, but
  the **largest single document is 9.30 MB** and 23 documents exceed
  256 KB. Worst case width × max ≈ 4 × 9.3 MB ≈ 37 MB of JSON buffers —
  a spike, not a leak, and nothing in the pipeline ever holds more than one
  document's content per worker.
- **Blobs are streamed, never buffered**: `saveFile` pipes
  `file.Reader → wr.WriteFile → io.Copy` (export.go:1306-1311,
  writer.go:66-90), and the native emit keeps that shape.

Summary — the peak-RAM model this pipeline is designed to:

```
peak  =  O(in-flight emit window)      width x (one tree + one marshal buffer);
                                       the concurrency cap controls it (4/2)
      +  O(UI-open objects)            user-bounded, never space-bounded
      +  O(all details)                16.9 MB JSON / 28,542 docs measured;
                                       a few tens of MB resident, accepted
      +  O(bundle aggregates)          <= 120.8 KB max observed per space
```

Without the active close, the passive TTL window (throughput x 60-120 s)
dominates everything and the constraint is not met — which is why
close-after-write is design, not optimization.

### 1.7 What the composer owes the format

- **I1 at bundle scope**: `finish` re-reads `index.json` and
  `properties.json` through `UnmarshalIndex`/`UnmarshalPropertyDictionary`
  before writing, as the harness does (main.go:983-1012) — a bundle this
  exporter writes that the package refuses is this exporter's bug, found
  at export time.
- **Omissions are lifts, never drops**: the space-settings document, the
  widget object, and matching bundled-relation documents are omitted only
  through the package predicates, whose lift-before-omit ordering and
  reconstruction checks (`WidgetsSnapshot` verified via `snapshotdiff`,
  main.go:786-800) come along unchanged.
- **Deterministic bytes end to end**: same space state ⇒ same file set,
  same names, same bytes per file. This is a testable property and should
  be a test: export twice, compare trees.

---

## 2. Alternatives considered and rejected

**Layout: keep the legacy directory names.** Rejected: `relations`/
`relationsOptions` reintroduce the word the format banned (PRINCIPLES rule
3), the camelCase compounds contradict the format's own naming rule (SPEC
§1, SPEC.md:1668), and since the importer provably never reads directory
names (import/pb/converter.go:338-341 is the only path rule), compatibility
buys nothing.

**Layout: genuinely flat — every document in one directory.** The purer
endpoint of §1.3's id rule: `objects/<id>.anyblock.json` for ALL kinds
makes id→path a total pure function with no kind probe at all, and with
id filenames the kind directories' human value is thin anyway (opaque
names in legible folders). Declined, not killed: the kind split was
approved (Q1), it still separates blobs from documents and keeps kind
tallies visible when debugging, the 7-directory probe it costs is bounded
and free in practice (§1.2, the zip central directory), and — because no
reader dispatches on paths — collapsing to flat later is a convention
change, not a format change. Per-object folders (one directory per
document) stay rejected outright: they double every path and answer
nothing.

**Naming: pure slugs with dedup counters.** Never viable, on measurement
(28,542 corpus documents):

| hazard | count |
|---|---|
| empty name | 654 (2.3%) |
| contains a path-hostile char (`/ \ : * ? " < > \|`) | 2,144 (7.5%) — `/` 1,514 · `:` 972 · `\|` 288 · `"` 161 · `?` 109 |
| leading/trailing space or dot | 444 |
| non-ASCII | 2,192 (7.7%); NFC ≠ NFD for 250 |
| longer than 100 UTF-8 bytes | 495 (max 2,536 bytes — over the 255-byte component limit ten times over) |
| Windows reserved device names (CON, PRN, AUX, NUL, COM1-9, LPT1-9) | 0 |

Deduplicating by slug alone, every one of the 77 spaces has collisions —
4,964 documents (17.4%) collide with a sibling; pages only is still 1,569
of 9,688 (16.2%) in 42 of 77 spaces; file objects are worst at 25.6% (one
space holds 373 files that all slug to `github-com-cover`). The "rare"
collision suffix would be the fourth-commonest thing in the archive, and
any first-writer-wins counter is nondeterministic under the concurrent
queue — `namer.Get` (export.go:1435) is this alternative's existing
implementation, and its `rand.Int63n` suffix is the exhibit. Raw names
with escaping instead of slugging fail the same table with extra steps
(`ApiSlug`'s `/`, `#`, `@` leakage, SPEC.md:6222-6235, is the same lesson
one layer down).

**Naming: hybrid `<slug>--<id8>.anyblock.json`.** This document's own
prior proposal, fully specified (deterministic slug transform, 8-char id
suffix, case-folded planning, suffix-lengthening tie-break) — and
**overturned by the human on the reference-resolution argument** (§1.3): a
reference carries an id and nothing else, so under hybrid names the only
path from a reference to its file is scanning documents (or a
name-carrying index that must then be kept in step — the same indirection
§9a's deleted `refs` legend was killed for, one level up). The slug half
bought listing legibility and nothing else; that value moves whole into
the later human-readable mode, where it belongs, bundled with type-subdir
grouping so no mode is half-legible. The measurements above survive the
overturn: they still prove slugs need an id beside them wherever slugs
appear — now in that mode's design instead of the default's.

**Blobs: keep the `source` detail hack.** Rejected — it is the thing being
replaced: it destroys a real user value in a real editable relation
(relations.json:930), the destruction round-trips through import
(import/pb/converter.go:404-414), and it makes a document's content a
function of the archive that contains it.

**Blobs: a path member on the file_object envelope** (e.g. `"file":
"files/x.png"`). Rejected: it moves the Source hack into the format instead
of deleting it — the document would describe its container, export ≠
Marshal for one kind, the §11 comparator needs an exemption, and a document
copied between bundles silently dangles. The format's own precedent is the
other way: `icon.file` holds an object id, never a path (SPEC §2b), and the
33 corpus objects holding filesystem paths in `coverId` are treated as
damage, not data (SPEC §11(e)).

**Blobs: adjacency convention alone (no manifest map).** Rejected as sole
mechanism: it makes a layout convention load-bearing in a format that
explicitly refuses to define layout (SPEC.md:2319), leaves authored bundles
(consumer 2) with no way to point at assets laid out their own way, and its
"exactly one sibling" invariant is unpoliceable except by tooling that
would then be reimplementing the map it refused to write. Kept as our
exporter's LAYOUT under the map (§1.4).

**Concurrency: single-threaded compose / re-read-based compose.** Both
rejected in §1.5 — the first for cost on the measured worst cases, the
second because the zip writer cannot re-read its own entries.

---

## 3. QUESTIONS

Q1-Q3 were answered by the human on 2026-08-26 and are kept below as
settled records, SPEC §15 house style — the decision, the overturned
alternative, and the reasoning that decided it. Q4-Q11 remain open.

**Q1. Directory names — SETTLED: approved as proposed, `objects/` flat.**
The set `objects/ types/ templates/ properties/ options/ participants/
files/` stands (§1.2), `properties/` beside `properties.json` included.
The human added one ruling the proposal had left implicit: **`objects/`
stays FLAT by default** — no type subdirectories. Type grouping belongs
exclusively to the later human-readable mode (§1.3, Q3), and when that
mode builds it, directories are named by the resolved type NAME slug
(stored key → type document → name), never the `type` wire spelling — 617
ordinary objects (6.4%), across 52 distinct types, spell their `type` as a
bson key, which would otherwise mint 52 opaque hex directories. The
measured case against default type-subdirs: median space has 20 ordinary
objects across 3 types, and 115 of 359 type directories (32%) would hold
≤ 2 objects. The kind-split-vs-flat tension this creates with Q2's id rule
is resolved in §1.2 (bounded 7-directory probe, stated plainly; the
genuinely-flat alternative recorded in §2).

**Q2. Document filenames — SETTLED: `<id>.anyblock.json`, hybrid
overturned.** The standing proposal was hybrid `<slug>--<id8>`; the human
rejected it on the reference-resolution argument, recorded in full in
§1.3: a reference carries an id and nothing else, the format addresses
everything by id, so id→file must be a pure function rather than a scan —
"right now it's transparent — we have ids everywhere"; an authoring agent's
minted ids can BE its filenames. Rename-stability of backups is a
secondary bonus, not the reason. The slug-hazard and collision
measurements move to §2, where they still kill pure slugs — now in
support of this conclusion instead of the hybrid.

**Q3. Readable filenames as an option — SETTLED, inverted.** Not
"ids-only as an option": ids ARE the default, and **human-readable output
is the option, added later** — one mode bundling BOTH readable filenames
AND type-subdirectory grouping under `objects/`, so no mode is
half-legible (one rule per mode). The default forecloses nothing: no
reader dispatches on paths, and the manifest carries whichever paths the
writing mode chose (§1.3). The mode itself is deliberately NOT designed in
this document; §1.3 records the measurements its designer will need.

**Q4. Blob binding — manifest `files` map (Alt B) with adjacent layout?**
Why it matters: this is the `source`-hack replacement, it touches
`index.json`'s schema (a format change: new manifest member,
`index.schema.json`, §2c text), and it decides how authored bundles ship
assets. Options: (a) manifest map + adjacent layout, no reader-side
stem-matching (§1.4 recommendation); (b) adjacency convention only, no
format change; (c) both mechanisms with map-wins precedence.
**Recommendation: (a). (b) leaves consumer 2 without free layout and makes
convention load-bearing; (c) is drift by construction.**

**Q5. May a native bundle's file_object documents live beside blobs in
`files/`, given the pb importer skips that directory wholesale?**
Why it matters: a native bundle fed to the LEGACY pb importer
(import/pb/converter.go:338-341) would have its file documents silently
skipped — but a native bundle is not pb-importable anyway (different codec,
different extension), so the question is really whether we guarantee
anything about legacy importers seeing native bundles. Options: (a) no
guarantee — native bundles are read by native wiring
(`anyblockconvert`/`ObjectImportExperience` path), the layouts are
independent; (b) keep file documents out of `files/` (a `file_docs/` split)
purely for defensive overlap. **Recommendation: (a) — the defensive split
re-creates the legacy two-directory correlation cost to protect a path that
cannot parse the files anyway.**

**Q6. Which RPC surface does the native exporter answer to?**
Why it matters: `model.ExportFormat` today has `Protobuf`/`JSON` (pbjson)
routed by `isAnyblockExport` (export.go:499); the native format needs an
addressable enum value, which is a protocol change in anytype-proto, and a
deprecation story for `Export_JSON` (pbjson). Options: (a) new enum value
(e.g. `Export_AnyBlockJSON`), pbjson untouched until clients migrate;
(b) repurpose `Export_JSON` in place — silently changes what existing
clients receive; (c) ship native behind `Export_JSON` + a request flag.
**Recommendation: (a); (b) breaks consumers on a version boundary they
can't see, (c) is a dialect switch inside one format id.**

**Q7. Does the native format become the DEFAULT space backup, and on what
timeline does pb export retire?**
Why it matters: decides how much compatibility weight the writer carries
(e.g. whether anyone still needs `profile` emitted) and what
`ExportSingleInMemory` (export.go:112) serves. Not answerable by
measurement — product call. **Recommendation: native becomes default only
after the native import path ships and a full-account round-trip soak
matches the sweep's 99.98%; single-object in-memory export emits exactly
one document, no bundle files, per PRINCIPLES rule 7 ("a document stands
alone").**

**Q8. Extension: settle SPEC §15 #1 as `.anyblock.json`?**
Why it matters: §15 #1 (SPEC.md:6215) still leans bare `.json`, but the
bundle now legitimately contains blobs that are themselves `.json` files
(12 corpus file objects have `file_ext == "json"`), and `DiscoverJSONFiles`
plus the importer need one cheap, collision-free document test.
Options: (a) `.anyblock.json` (what the harness already writes,
main.go:377); (b) bare `.json` with content sniffing via `DetectFormat`.
**Recommendation: (a) — the double extension is the entire skip-rule for
non-document files, and it costs nothing; update §15 #1 to settled.**

**Q9. Should `index.json` carry any cross-space super-index for
multi-space exports?** (re-answered after Q2's overturn — the prior
recommendation used the rejected hybrid scheme for space directories)
Why it matters: `spaces/<id>/` wrappers make each space a self-contained
bundle; a top-level listing (space names → directories) would help a human
facing 77 CID-named directories, but it is a new format surface with no
reader yet. Options: (a) nothing at top level — plain `spaces/<spaceId>/`,
consistent with Q2: a space reference is an id, and id→bundle-root stays a
pure function; (b) a minimal top-level `index.json` naming each space
bundle; (c) readable space directory names — now part of the later
human-readable mode (Q3), not the default, by the same one-rule-per-mode
principle. **Recommendation: (a) for the default mode; readable space
directories ride the human mode when it is designed. (b) only if a real
consumer materializes — each per-space `index.json` already carries the
space's own name, so a tool can build the listing in one pass.**

**Q10. Where does the promoted composition code live?**
Why it matters: the composer must be importable by `core/block/export/anyblock`
AND the cmd tools, and `cmd/internal/anyblockbatch` is importable by
neither production code nor anything outside `cmd/`. Options: (a) the
store-wired composer in `core/block/export/anyblock`, with the pure
byte-level pieces (used-key scan, path planning helpers) exported from
`pkg/lib/anyblockjson`; (b) a new `pkg/lib/anyblockjson/compose`
subpackage holding the whole bundle-level composition, wired by both.
**Recommendation: (b) — SPEC §13 already gives composition a named home
("bundle tooling"), the harness's spaceComposer moves there nearly intact,
and the cmd tools shed their private copy; `core/block/export/anyblock`
then only wires store, cache, and writer to it.**

**Q11. Close-after-write ships against unfixed GO-7333 — fix first, or
gate?**
Why it matters: the memory model (§1.6) stands on the emit task calling
`TryRemoveFromCache` after every write (§1.5), and that path reaches the
filed-but-unfixed any-sync bug GO-7333 — `ocache.TryRemove` on a
still-loading entry can nil-deref at ocache.go:271 (the very
`e.value.TryClose` call), hang later waiters forever, or race on
`e.value`. The exporter would be the heaviest caller of this path ever.
Options: (a) fix GO-7333 in any-sync first and make the exporter depend on
the bumped version; (b) ship close-after-write anyway, accepting the same
race the fulltext indexer already runs on every indexed object
(fulltext.go:348) — the exporter's call lands after its own `cache.Do`
returns, so the entry is loaded, and the window arms only on a concurrent
re-load by another caller; (c) TTL-only until fixed — rejected by the
memory model itself (§1.6: the passive window is throughput-proportional
and unbounded by space size). **Recommendation: (a) — the fix is small and
already scoped in the GO-7333 filing, and an export that can hang an ocache
entry forever is a worse failure than the memory peak it prevents; (b) is
acceptable as an interim only if the any-sync bump cannot land in the same
release, since the indexer has soaked the identical race in production at
scale.**

---

## 4. Migration and compatibility notes

**Existing exports.** Nothing changes for them: legacy pb/pbjson archives
keep importing through the pb importer, whose only path rule
(import/pb/converter.go:338-341) native bundles never relied on. The legacy
writer, `namer`, and md/pb/dot/graphjson converters are untouched — the
extraction moves collection OUT of `export.go`; the legacy writing path
keeps calling it through the same interface.

**Native bundles** are read by the native wiring only
(`cmd/anyblockconvert` → `ObjectImportExperience` path today; the
production native importer is separate future work). A native bundle is not
a valid pb import and does not pretend to be.

**Markdown later.** The md exporter keeps its own naming (`makeMarkdownName`,
export.go:1362) and writer for now; the collect interface below is
format-agnostic (`Closure` replaces `isProtobuf`), so md can migrate onto
the same seam later without this design changing — that migration is
explicitly not designed here.

**Extracted collection interface** (outline only — signatures, no bodies):

```go
// core/block/export/collect
package collect

type Closure int

const (
    // ClosureContent — the md-style closure: nested objects and linked
    // files only (export.go processNotProtobuf, :593).
    ClosureContent Closure = iota
    // ClosureDerived — the collect-everything-derived closure the native
    // format wants: types, relations, options, templates, dataview
    // dependencies, recommended relations (export.go processProtobuf, :610).
    ClosureDerived
)

type Request struct {
    SpaceId          string
    Ids              []string // empty = whole space (export.go getExistedObjects, :1138)
    Closure          Closure  // replaces the bare isProtobuf bool (export.go:503-504)
    IncludeNested    bool
    IncludeFiles     bool
    IncludeArchived  bool
    IncludeBacklinks bool
    IncludeSpace     bool
    StateFilters     *state.Filters
}

type Doc struct {
    Details *domain.Details
    IsLink  bool
}

type Collector interface {
    Collect(ctx context.Context, req Request) (map[string]*Doc, error)
}
```

```go
// pkg/lib/anyblockjson/compose (per Q10 recommendation)
package compose

// Plan is the deterministic path table (a pure per-id function under §1.3)
// plus the manifest tables, built single-threaded from the collected
// details before any emit (design §1.1; omission is decided at emit).
type Plan struct { /* id → {path, kind}; manifest tables */ }

func BuildPlan(docs map[string]DocMeta, opts PlanOptions) (*Plan, error)

// Composer accumulates bundle facts during concurrent emit and writes the
// two bundle files at finish. Observe* methods are safe for concurrent use.
type Composer struct { /* unexported; mutex-guarded */ }

func NewComposer(opts anyblockjson.Options, plan *Plan) *Composer
func (c *Composer) ObserveDocument(id string, data []byte, sw SnapshotMeta) error
func (c *Composer) ObserveOmitted(sw SnapshotMeta) error
func (c *Composer) Finish() (index, properties []byte, err error) // re-read-verified (I1)

// UsedPropertyKeysFromBytes — the byte-level promotion of
// cmd/internal/anyblockbatch.UsedPropertyKeys (scan.go:908), shared with
// the cmd tools (design §1.1).
func UsedPropertyKeysFromBytes(doc []byte) (map[string]bool, error)
```

```go
// core/block/export/anyblock — the wiring: store + cache + writer around compose
package anyblock

type Exporter struct { /* picker, objectStore, fileService, resolvers */ }

func (e *Exporter) Export(ctx context.Context, req collect.Request, wr writer.Writer) (succeed int, err error)
// internally: collect → compose.BuildPlan → queue tasks
//   {Marshal + wr.WriteFile + blob stream + composer.Observe*} → composer.Finish
//   → wr.WriteFile(index.json, properties.json)   (the postProcess seam, export.go:1529)
```

**SPEC follow-ups this design creates** (to be filed with the SPEC when
implementation starts): the manifest `files` member (Q4) — schema + §2c
text + `anyblockvalidate` cross-checks; §15 #1 settled to `.anyblock.json`
(Q8); a §2c note recording THIS exporter's directory and filename
convention in the "one exporter's convention" slot.
