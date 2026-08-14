# AnyBlock JSON — addressing: one identity story for five reference kinds

Status: **design dossier** (research + recommendation, no normative force) ·
2026-08-08 · GO-7383 · revised same day after three review rounds
(normative label minting, write-side defaults flipped, compatibility
constraint removed; the slugs-always surface adopted in §7.5a; the
internal-key strategy argued to (b) and then **flipped to (a) on
falsifying evidence** — the reversal is kept visible in §7.5 rather than
rewritten away). Companion to `SPEC.md` (v0.7) and `FLAT.md`; feeds a
SPEC revision and closes SPEC §15.3. Every claim about current behaviour is
verified against source at the cited line; prior art was researched against
vendor documentation (§6).

**Verdict, up front.** Keep readable strings in every value slot — option
*names*, type/property *keys* — and generalize the §9a refs legend into a
per-kind **pin table**: an envelope map from label to internal identity,
with the §9a total resolution rule. **A label is an opaque map key**: it
must be unique within the document and nothing else; its readability is a
courtesy to the reader, not a mechanism, and resolution never parses one
(§7.1). Write paths flip to **strict-by-default wherever a stale read can
exist** (PATCH): an unknown option name is a loud 400 with did-you-mean
unless the request says `create: true`; an ambiguous name is always a 400;
and **only options may ever be created implicitly** — an option's name is
its entire definition, while properties, types and objects carry rich
content only an explicit create can supply (§7.4).
On the surface, one rule with no exceptions (§7.5a, adopted): **API
v2 addresses every type and property by the snake_case api-key slug** —
`dueDate` is `due_date` on the wire, and bundled, API-created and
UI-created keys are indistinguishable to a caller; the document format's
key vocabulary follows the surface (a deliberate cascade into SPEC §3).
On the internal-key question the decision **flipped on review evidence**
(§7.5): every new type and property mints a BSON internal key — v1's
identity layer — because derived readable keys converge concurrent
different-intent creates into silent format merges (no guard exists, and
none can exist above the derivation — §7.5-1) and make delete-then-
recreate a structural dead end (the derived tree persists — §7.5-2);
the caller's key lives only in the hardened slug, which §7.5a made the
entire visible surface anyway.
**Nothing here has shipped** — AnyBlock JSON and API v2 have no users,
no exported documents, no third-party consumers — so no default below was
chosen for compatibility; each is chosen because it is right, and this is
the cheapest moment there will ever be to choose it (§7.6).

---

## 1. The problem

AnyBlock JSON serves two masters:

1. **Lossless round trip** — backup, export/import, migration
   (`snapshotdiff`, `cmd/anyblockroundtrip`; production acceptance ≥ 99.86%,
   `ANOMALIES.md`). Needs identity that survives renames and duplicates.
2. **API v2 and agents** — an LLM reads and writes the format. Needs
   guessable, human-readable, token-cheap identifiers. A 59-char CID costs
   ~24 tokens and models mutate opaque ids in flight (short handles ≈ −89%
   id errors — `docs/AgentApiV2Research.md` §3.6). A 24-hex property key is
   unusable by a small model.

The collision happens at the identifier layer, because there are **four ways
to address most things** — internal id, internal key, api key (normalized
slug), display name — across **five reference kinds**: type keys, property
keys, select/status/tag option values, object references, block references.
Two of the five (objects, blocks) already have a settled answer; the other
three do not, and the defects are live.

## 2. Ground truth today (verified)

### 2.1 The matrix

| Kind | Internal identity | Format (SPEC v0.7) | v1 API | v2 API |
|---|---|---|---|---|
| type | uniqueKey `ot-<key>`; key = bundled word (`task`) or 24-hex BSON (UI-created) | key, `ot-` trimmed (`export.go:159-180`) | `apiObjectKey` slug, BSON fallback (`core/api/service/type.go:190-199`, `core/api/util/key.go:49-57`) | raw internal key; never reads `apiObjectKey` (grep: zero hits under `core/api/v2/`) |
| property | relation key: bundled camelCase (`dueDate`) or 24-hex BSON (`objectcreator/relation.go:47`) | stored key verbatim (§3) | `apiObjectKey` slug, BSON fallback (`property.go:553-562`, `key.go:38-45`) | raw relation key (`v2/service/discovery.go:276-284`) |
| option | option object id (derived from uniqueKey `opt-<bson>`; bare 24-hex in legacy data) | display **name**; silent raw-id fallback (`export.go:383-390`) | `apiObjectKey` slug + id (`tag.go:219`, `key.go:61-69`) | name only (`discovery.go:292-319`) |
| object | space-local CID (~59 chars) | full id, or refs-legend label (§9a) | id | id; compact refs legend by default (C4) |
| block | doc-local id (24-hex or author-chosen) | id, optional on input (§9) | n/a | full id on edit reads; 5-char suffix labels in outline; unique-suffix match on writes (`v2/service/object.go:404-429`) |

Blocks and objects are the *solved* kinds and the template for the rest:

- **Blocks** (APIV2.md C4, §7.1): full ids where round-tripping matters,
  short suffix labels in read-only shapes, and `matchBlockRef` on every
  write path — exact id first, else unique suffix, **ambiguity is a loud
  400** naming the remedy, zero matches a loud 404.
- **Objects** (SPEC §9a): the `refs` legend — an authoritative opaque map,
  labels chosen by export (id suffixes) or by humans/agents (any label),
  with a **total resolution rule**: in the map → that id; not in the map →
  it *is* a full id. No shape heuristics. Lossless, because the legend
  inverts.

### 2.2 The two live defects (the unsolved kinds)

**D1 — the silent id fallback.** `exporter.optionName` returns the raw
stored value when the resolver cannot map an option id to a name
(`export.go:383-390`):

```go
func (e *exporter) optionName(key, id string) string {
    if e.opts.ResolveOptions != nil {
        if name, ok := e.opts.ResolveOptions.OptionName(domain.RelationKey(key), id); ok {
            return name
        }
    }
    return id
}
```

So the format **already emits a mix of names and ids in the same slot**, and
a consumer cannot tell which it is holding — there is no marker. On
re-import the id is a name like any other (`dataview.go:545-553`, no
id-lookup), and the API's create-missing resolver
(`v2/service/resolver.go:128-162`) then **mints a real option literally
named `6a7663db…`**. This is the sharpest symptom of the whole problem: a
lossless-looking pipeline that manufactures garbage vocabulary, silently.
It is not confined to file export — v2 API object reads wire the same
resolver (`v2/service/object.go:191`), and the view-op commit path
re-serializes whole dataview blocks through it (the `readOnlyOptionResolver`
comment in `resolver.go:325-331` names "a dangling option reference exported
as its raw id" as a live case).

**D2 — duplicate names resolve by store order.** Duplicate option names are
legal (nothing in `objectcreator/relation_option.go` checks name uniqueness;
production data has them — `ANOMALIES.md` #6, 7 objects on `tag`).
`storeresolver.OptionId` returns the **first** option whose `Text` matches,
in `ListRelationOptions` order (`storeresolver.go:106-113`) — an order the
API contract nowhere defines. Resolution of a twin name is a coin flip;
round-tripping an object that referenced the losing twin silently re-points
it (the one accepted loss in the ≥ 99.86% figure). Scope matters here:
**options belong to one property** — every lookup is `(propertyKey, name)`
— so the same name on two *different* properties ("High" on `status`,
"High" on `priority`) is two unrelated options and entirely normal. The
defect is twins **within one property**; nothing in this dossier treats
cross-property name reuse as a duplicate.

### 2.3 Findings beyond the known picture (worse than assumed)

1. **`apiObjectKey` is not unique.** `injectApiObjectKey`
   (`objectcreator/util.go:18-26`) derives the slug from the create-time
   name with **no collision check**, and none of its callers
   (`relation.go:44`, `object_type.go:34`, `relation_option.go:49`) check
   either. Two properties named "Manual property" both get
   `manual_property`. v1's tag cache then keys one map by id, uniqueKey
   *and* slug (`cache_manager.go:114-116`) — the duplicate slug silently
   overwrites: last write wins. **Pointing v2 at `apiObjectKey` as-is
   imports the duplicate-name ambiguity from the option layer into the key
   layer.** The in-flight v2 fix is a good interim; it is not the answer.
2. **`apiObjectKey` is frozen at birth *and* mutable by hand** — the worst
   combination for an identity. It is derived from the name at creation and
   never recomputed, so after a UI rename it matches neither the current
   name nor anything an agent would guess; yet v1 lets any client re-point
   it at any time (`type.go:288-298`, `property.go:282`, `tag.go:174`), so a
   stored reference pinned to it can be silently re-aimed. As an *address*
   (a git branch) that is fine; as *stored identity* (what a document
   carries) it is unsafe on both axes.
3. **The slug layer is unaudited.** The option path has a second injection
   branch that transliterates but forgets to snake_case
   (`relation_option.go:51-53`), diverging from `injectApiObjectKey` one
   call above it. Cosmetic, but evidence that nothing enforces slug
   discipline.
4. **The BSON pass-through is the deliberate v1 policy**, not an accident:
   `ToPropertyApiKey`/`ToTypeApiKey`/`ToTagApiKey` all detect a 24-hex key
   and return it verbatim (`key.go:38-69`) — v1 documents the giving-up in
   its own comments.
5. **SPEC §2a's format-conflict guard is unimplemented.** The SPEC
   promises "a conflict with an existing property's format is an error at
   the wiring level"; the wiring never checks: `creatingResolvers.
   PropertyId` returns the existing relation on a key hit with the
   declared format ignored (`resolver.go:357-363`), and `createRelation`
   validates only that the format is present and a valid enum
   (`relation.go:24-30`) — never against an existing same-key relation.
6. **A production corpse carries TWO flags, and `isDeleted` is what
   actually hides it.** *(Corrected 2026-08-14 — the original text below
   described a shape production never has, and every corpse fixture in
   the repo inherited it.)*

   `deleteDerivedObject` sets `isUninstalled=true`
   (`core/block/delete.go:113-127`), and the **same Apply** stamps
   `isDeleted=true` beside it: `injectDerivedDetails` writes `isDeleted`
   whenever `isUninstalled` is present on the state
   (`smartblock/detailsinject.go:219-226`). `BeforeDelete` then tombstones
   the index row, and because the tree itself survives (§2.4-5) the next
   space load re-indexes it — the row comes back with **full details and
   both flags**. So the persisted shape of a UI delete is
   `{isUninstalled:true, isDeleted:true}`, never `{isUninstalled:true}`
   alone.

   That matters for what is visible to whom. Every plain store query
   injects `isDeleted != true` (`database.go:109-123`), so a production
   corpse is **already invisible** to ordinary queries — including the
   ones under `core/api/` that filter nothing themselves. The explicit
   `isUninstalled` filters v2 added (`livePropertyFilters`,
   `liveTypeFilters`) are therefore belt-and-braces rather than the sole
   defence: they pin the corpse policy to the flag that *means* "the user
   deleted this", instead of leaning on an injected default that any
   query suppressing it — or any point lookup that never had it — loses.

   Two consequences of that lean survive and are load-bearing:

   - **Point lookups by id are unfiltered.** `GetRelationById` falls back
     to reading details directly, so a corpse referenced by a type's
     `recommendedRelations` is still served in `typeProperties`, both
     flags and all (§8.40).
   - **A query that suppresses the defaults sees corpses again.** Any
     probe written with `Condition None` — the round-trip tolerance, the
     removal set — must suppress **both** `isArchived` and `isDeleted`, or
     it silently sees only the fixtures' shape and never production's
     (the §8.40 defect: a tolerance that worked for archived properties
     and not for the uninstalled ones it was written for).

   The original finding, corrected: "nothing filters it, so a corpse
   remains fully visible" holds only for the `{isUninstalled}`-alone shape
   — the fixtures' shape. For the shape production persists, a plain
   query hides the corpse on the `isDeleted` default alone, and what
   remains visible is narrower and more specific: the channels that
   bypass those defaults (point lookups by id, probes that suppress them)
   and the derivation layer, where a same-key create meets the surviving
   tree with no store query involved at all (§7.5-2). Any claim about
   corpse visibility must name which of the two shapes it is about.

### 2.4 How unique keys derive object ids (verified — it decides §7.5)

The review asked to confirm the parallel-create hazard behind BSON internal
keys. Confirmed, and sharper than folklore:

- A derived object's change payload is a **pure function of (smartblock
  type, internal key)** — `createChangePayload`,
  `objectcache/payload.go:18-28`.
- In a **shared space**, the tree derives from (spaceId, that payload)
  with **no account key, no timestamp, no randomness** —
  `derivePayload`/`DeriveTree`, `payload.go:30-38`, `tree.go:90-95`. So the
  object id is a pure function of (space, kind, internal key): **any member
  deriving the same unique key computes the same object id.** (The personal
  space adds the account sign key — `payload.go:40-48` — but has one
  account, so per-space determinism holds there too.) Ordinary objects, by
  contrast, are created from a random 32-byte seed plus a timestamp
  (`payload.go:50-58`) and can never collide.

Four consequences:

1. **Convergence is the install mechanism.** Bundled types/relations
   (`rel-dueDate`, `ot-task`) install idempotently *because* every device
   derives the same object.
2. **Concurrent same-key creates converge.** A second local create of an
   existing key fails on put; two members creating the same key offline
   each succeed locally and their trees **merge on sync into one object**,
   conflicting details resolving in CRDT order — one name/format silently
   wins. Same key ⇒ same object is a space-level invariant, not a race
   outcome.
3. **This is exactly why UI creates mint BSON.** A name-derived readable
   key would make two users' unrelated "Status" properties merge into one
   object; the UI buys distinctness with opacity (`relation.go:46-47`;
   options always `opt-<bson>` via `getUniqueKeyOrGenerate`,
   `objectcreator/util.go:32-44`).
4. **v1 and v2 already embody the two candidate key strategies.** v1
   `POST /properties` never sets the relation key — the create mints a BSON
   and the caller's key lands only in `apiObjectKey`
   (`core/api/service/property.go:208-229` + `relation.go:46-47`): that is
   strategy (a) live, twin slugs and all. v2's create-missing pins the
   document's key as the stored relation key (`resolver.go:386-401`), and
   v2 `POST /types` and `POST /properties` derive identity from the
   caller's key (`schema_write.go:235-240`, `schema_write.go:455`): that
   is strategy (b) live. §7.5 decides between them — for (a).
5. **Derived objects are never destroyed — deletion is a flag, and the
   tree persists.** UI delete sets `isUninstalled=true` and keeps the
   tree (`delete.go:113-127`); v2's DeleteProperty/DeleteType merely
   archive (`ObjectSetIsArchived` — `schema_write.go:372, 536`); the
   bundled reinstall path flips the flags back and **reuses the same
   object with whatever content it accumulated** (`installer.go:210-232`).
   Re-deriving a "deleted" key therefore cannot mint a fresh object:
   `PutTree` on the surviving tree returns `ErrTreeExists`, which only
   the installer tolerates (`installer.go:128`; `objectcache/tree.go:57-59`
   propagates it everywhere else). Same key ⇒ same tree, forever — the
   fact that decides §7.5.

## 3. Scenario analysis

What each consumer actually needs, and what breaks when the wrong identity
is picked. "Name" below means display name for options, readable slug for
types/properties.

| Scenario | Identity that works | What breaks if you pick wrong |
|---|---|---|
| Full-account backup → restore (same space) | **id / stored key**. Everything resolves; renames since the backup are survived only by id. | Names: a rename between backup and restore mints a twin option and re-points values to it (silent); twin names collapse (D2). This is *the* case names cannot serve. |
| Export → import into another account | **name / self-contained key**. Foreign ids resolve to nothing — id-only documents are dead on arrival; create-missing (SPEC §3, §2a) rebuilds vocabulary from names/keys. | Ids: values dangle or, worse, collide with unrelated local ids. This is *the* case ids cannot serve. |
| API v2 read | name + key (C2), compact object refs (C4). | Raw BSON keys: unusable, unguessable, ~10 tokens each — v2's live state for UI-created properties. Raw ids in name slots: D1 reaches the API read surface. |
| API v2 write (agent authors JSON) | name + key, with loud resolution (§7.4). | Ids: agents mutate them in flight (−89% errors from short handles). Names alone with silent create-missing: the rename race (§3.1) and twin ambiguity (D2). |
| Small-model authoring (wrapper tier) | names only, zero opaque tokens, no legend in sight (APIV2.md §7.1: "the model never sees or emits a 24-hex id"). | Anything composite or tagged: a 3B model strips suffixes and mangles unions; both forms then need accepting forever. |
| Third-party integration / sync | **stable id + name**, both, per reference (the Kubernetes ownerReference shape). Sync must *detect* renames, not perceive a new entity. | Name-only: every rename is a delete+create to the integration. Id-only: cross-tenant sync impossible. |
| Diff / merge two documents | stable anchors (ids/keys); names as values. | Name-as-identity: a rename diffs as remove+add everywhere the value occurs; twins misalign the diff. |

The pull is real and structural: backup wants ids, transfer wants names,
agents want names, sync wants both. **No single-vocabulary design serves
all rows.** The only designs that survive the table are those that carry a
readable value *and* an id, and they differ only in where the id lives
(inline, composite string, legend, sidecar, second profile).

### 3.1 The rename problem, exactly

A user renames option "High" → "Critical" (id `o1` unchanged). What happens
per design, for the two critical windows:

| Design | Backup taken before rename, restored after | Agent read "High" yesterday, writes today |
|---|---|---|
| **Names only (today)** | Restore resolves "High" → nothing → **mints a twin option "High"**; restored objects point at the twin, live objects at "Critical". Silent divergence. | Write "High" → twin minted, silently (`created` side-effect is reported but nothing steers the agent to look). |
| **Ids only** | Restore resolves `o1` → correct. | Agent cannot plausibly author ids; DOA. |
| **Composite `High#6a76`** | Suffix resolves → correct; stale name half is cosmetic (Stack Overflow slug-URL semantics). | Only if the agent echoes the suffix — small models won't; bare "High" must stay legal → the race returns. |
| **Inline `{id,name}`** | id wins → correct (Notion semantics). | Same caveat: agents author bare strings; both forms legal forever. |
| **Pins + strict writes (recommended)** | Pin `"High": "o1"` resolves by id → correct; the label is cosmetic. | Document paths (create) carry pins → resolve by id. Bare ops (`setProperties`) hit the strict default: "High" no longer matches → **400 with did-you-mean ("Critical") and the `create:true` remedy** — loud, before damage (§7.4). |
| **Two profiles** | Backup profile (ids) → correct. | Agent profile is names-only → the race, unmitigated, plus a reader fork. |

Property/type renames are the benign half **because the stored key never
changes on rename** — `name` is a detail, the relation key/uniqueKey is
immutable. The format's stored-key addressing is already rename-proof for
these two kinds; the problem there is purely that UI-created keys are
unreadable BSON. Only *options* conflate display name with identity in the
format today.

## 4. The design space

Each candidate: mechanism → what kills it (or why it survives).

**A. Envelope mode selector** (`"addressing": "ids"|"names"|"both"`).
One flag, reader forks on it. Dies because neither pure mode serves even
one full scenario row (§3): backup-in-ids is cross-account-dead,
agent-in-names is rename-dead — so real use always picks `both`, and then
the selector is dead weight on top of whatever encodes "both". Also makes
every consumer conditional (two parsers in every tool).

**B. Per-kind selectors** (types by key, options by id, … declared in the
envelope). Same death as A with more axes: the modes multiply
(4 kinds × 3 modes), documents stop being one language, and every reader
carries the product. Per-kind *rules* are right; per-kind *modes* are not.

**C. Per-value inline tagging** (`"High"` vs `{"id": "o1", "name": "High"}`).
The Notion shape; unambiguous and rename-safe where the object form is
used. Dies on four counts: every value slot becomes a union (JSON-Schema
`oneOf` in exactly the schemas C13 promised to keep strict/flat — the
constrained-decoding floor the flat encoding was built for); token cost is
paid per value occurrence, not per distinct value; agents author the bare
form anyway so both live forever (two ways to say one thing, against the
format's own canon §4); and C2 loses "option names, everywhere" — the slot
no longer holds one vocabulary.

**D. Composite string** (`"High#6a76"`). Readable, self-describing,
compact. Dies as a *value* encoding: needs escaping (`#` is legal in option
names — "C#" is a real tag), needs a parser in every consumer, pollutes
equality (a filter comparing the stored name against the composite), and
small models strip or mangle the suffix so bare names must stay legal —
returning the ambiguity it was built to kill. **Survives as label
cosmetics**: inside a legend, `High#6a76` is an opaque map key — no
escaping, no parser, no equality problem (§7.1).

**E. Refs legend, generalized (pins)** — the §9a mechanism applied to
options, property keys, type keys. Values stay bare readable strings; an
envelope map pins each label to its internal identity; resolution is total
(in-map → pinned id/key; absent → the string is a name/key). Survives
everything: lossless when pinned (legend inverts, iCal-UID property),
readable always, rename-safe where pinned, cross-account portable (pins
strip cleanly; names/keys remain), duplicate-safe (twin names get distinct
labels), and the *absence* of a pin is itself information — the D1
mix-of-names-and-ids becomes expressible and therefore fixable. Costs: an
envelope field, a SPEC section, and legend tokens on reads (zero in the
common case — see §7.4). This is the recommendation; full mechanism in §7.

**F. Dual emission / sidecar id map.** A parallel `optionIds` map keyed by
value, or a separate sidecar file. The in-envelope variant *is* E keyed by
name instead of label — strictly less expressive (cannot represent twins).
The separate-file variant dies immediately: agents don't fetch sidecars,
files separate, and the atomicity of one-document-one-truth is the point of
the format.

**G. Two serialization profiles** (backup profile: ids/keys everywhere;
agent profile: names). The honest-sounding answer, and protobuf (wire
numbers vs ProtoJSON names) proves the pattern ships. Dies on three counts.
(1) The backup profile still cannot be id-only, because backup-grade
artifacts are exactly what gets imported cross-account (use-case bundles,
space sharing) — so the "backup profile" needs names too, i.e. a both-form,
i.e. E or C anyway. (2) The protobuf lesson cuts the other way: protobuf's
*human* serialization is the rename-unsafe one — having two profiles does
not give the agent profile an id story, it just names the gap. (3) Every
tool in the middle (verifier, differ, importer) either forks or handles
both. What *is* right about G survives in E: pin-present vs pin-stripped
**are** the two profiles, produced by emission policy over one format with
one resolution rule — the shared core G never manages to state.

**H. Stored slugs as identity** (address options by their `apiObjectKey`,
frozen at birth). Rename-stable and readable-ish. Dies because a
frozen-at-birth slug diverges from the display name after any rename — the
agent reads `high` while the UI says "Critical", the worst of both
readability worlds — and because today's slug layer is non-unique and
mutable (§2.3). Slugs are the right *API address* once hardened (§7.5);
they are not the right *document value* for options.

**I. Kind-prefixed opaque ids** (Stripe `opt_…`, `rel_…`). Self-identifying
tokens would make the D1 mix at least detectable. Rejected as the design:
it hardens the id vocabulary the agent surface is trying to leave. Worth
stealing only as hygiene *if* ids ever get re-minted — not a near-term
lever, given ids are CIDs/derived and not ours to reshape.

## 5. Scoring

Criteria per the brief. `●` good · `◐` partial · `○` fails. "Failure mode"
is the tie-breaker: **a design that fails loudly beats one that fails
silently** — the lesson of D1.

| Design | Unambiguous | Lossless | Rename-safe | Cross-account | Small-model guessable | Token cost | Impl/migration cost | Failure mode |
|---|---|---|---|---|---|---|---|---|
| Names only (today) | ○ (D1, D2) | ○ | ○ | ● | ● | ● | — | **silent** |
| Ids only | ● | ● | ● | ○ | ○ | ○ | low | loud but dead-end |
| A. mode selector | ◐ (per doc) | ◐ | ◐ | ◐ | ◐ | ◐ | med + reader fork | mixed |
| C. inline `{id,name}` | ● | ● | ● | ● | ○ (unions) | ○ (per occurrence) | high (schemas, C13) | loud |
| D. composite value | ◐ (echo-dependent) | ◐ | ◐ | ● | ○ (suffix loss) | ◐ | med (escaping, parser) | silent when suffix dropped |
| **E. pins + strict writes** | **●** | **●** | ● (docs; bare ops fail loud, §7.4) | **●** | **●** (authors never see them) | ◐→● (a legend line per non-bundled key and per distinct select value; zero on fully-bundled docs; `?pins=` opts down) | **med-high, additive** (incl. slug hardening + v2 create rework, §7.5) | **loud** |
| F. sidecar | ◐ (no twins) | ◐ | ◐ | ● | ● | ◐ | med | silent (sidecar lost) |
| G. profiles | ◐ (per profile) | ● / ○ | ● / ○ | ○ / ● | ○ / ● | split | high (everything ×2) | split |
| H. stored slugs | ○ today (§2.3) | ◐ | ● | ◐ | ◐ (stale slugs) | ● | med (hardening + backfill) | silent (shadowing) |

## 6. Prior art

What was actually researched (vendor docs, not summaries), what it
contributed, what was rejected.

**Shaped the answer:**

- **Notion** — the closest complete analog. Property values carry `{id,
  name}`; *"id may be used in place of name"* on writes; the id *"remains
  constant when the property name changes"*. Select options: **names are
  unique case-insensitive by schema constraint** (Notion deleted the twin
  problem rather than solving it), and writing an unknown name
  **auto-creates the option, gated on write scope**. Contribution: the
  id-primary/name-secondary duality works and users accept auto-vivify; but
  Notion pays for it with unions in every value (design C's costs). We take
  the semantics and move the id out of the slot.
- **Airtable** — `returnFieldsByFieldId` is a *read-shape* toggle (a mode
  selector that works because it is per-request, not per-document); writes
  accept name or id interchangeably; and **unknown select options are a hard
  error unless `typecast=true`** — auto-creation is opt-in "to ensure data
  integrity". Contribution: the strict/permissive switch belongs on the
  *write request*, not in the document — directly §7.4's verb-bound default
  plus `create` flag.
- **Kubernetes** — `metadata.name` is a reusable human handle;
  `metadata.uid` is identity; `ownerReferences` store **both** and GC
  honors the reference only when the uid matches, so a reused name can
  never re-bind a reference. Contribution: the pin table is exactly
  "store the uid next to the name"; a resolved pin whose target is gone
  must *fail*, not re-bind to a newer namesake.
- **git** — abbreviated SHAs are shortest-*unique* prefixes, checked
  against the actual object corpus, auto-lengthening as it grows, and an
  ambiguous abbreviation is a **hard error, never a guess**. Contribution:
  the label-minting rule (§7.1), the write-side ambiguity 400 (§7.4), and
  the existing `matchBlockRef` behavior it validates.
- **JSON-LD `@context`** — a document-level legend (term → IRI) declared
  once and applied everywhere is standardized, mainstream technology.
  Contribution: precedent for pins as an envelope concept; nothing else
  (it addresses vocabulary, not mutable user data).
- **Linear** (`ENG-123`) — the readable key is **frozen at assignment**,
  never recomputed from the mutable title. Contribution: the `apiObjectKey`
  hardening shape — mint once, never re-derive (§7.5).
- **Stack Overflow URLs** (`/questions/{id}/{slug}`) — id authoritative,
  slug cosmetic, stale slug redirects. Contribution: the reading of pinned
  labels — the name half may go stale; the pin resolves; nothing breaks.
- **iCalendar UID** (RFC 5545) — a mandatory persistent id exists purely so
  re-import matches instead of duplicating. Contribution: the backup
  scenario's requirement stated as a 25-year-old MUST.
- **HTML heading anchors** — the canonical *anti-pattern*: address derived
  live from display text; edit the heading, silently break every inbound
  link. This is precisely names-as-identity for options today, and the
  reason H (live-derived slugs) is rejected.
- **Excel structured references** — rename-safe *names* are achievable only
  inside a live single-writer app that intercepts the rename and rewrites
  all references transactionally. Contribution: the explanation of *why*
  this problem exists at all — anytype-heart's live state is Excel; a
  serialized document is not, so the document must carry ids.

**Rejected:**

- **Discord snowflakes / render-time resolution** (ids only in content,
  names resolved at render) — requires a live resolver at read time; a
  backup file has none.
- **Protobuf field numbers** — validates two-profiles in the abstract, but
  its own human profile (ProtoJSON, keyed by mutable names) is the
  rename-unsafe one; nothing to borrow beyond the warning (see G).
- **MCP / tool-schema conventions, llms.txt** — surveyed and empty: no
  published mechanism for token-cheap stable identifiers; the field
  recapitulates id + display-name pairing. Confirms we must originate the
  answer, not borrow it.
- **Slack channel id/name duality** — accepts both, but Slack itself warns
  channel ids can change (Connect re-prefixing); a caution to state our
  ids' immutability explicitly, otherwise inapplicable.
- **ENS/DNS** — name→address resolution with no document/reference duality;
  wrong problem shape.
- **SQL surrogate-vs-natural keys literature** — decades without a general
  winner; a meta-lesson (the answer is contextual, per-kind) rather than a
  design.
- **CSV import header-matching** (Flatfile/Dromo-class fuzzy matching) —
  not a design to adopt but the floor to remember: cross-account import
  *is* name reconciliation; any design that drops names from the document
  ends up rebuilding this machinery, badly.

## 7. Recommendation

**Adopt design E with strict writes: readable labels in every value slot,
one per-kind pin table in the envelope, the §9a total resolution rule, loud
failure everywhere a resolution can go wrong. Fold the existing `refs`
legend in as the object kind of the same concept. Keys follow strategy (a):
every new type and property mints a BSON internal key, the caller's key
lives in the hardened unique slug, and the slug is the only key any
surface speaks (§7.5, §7.5a).**

One clarification the review demanded, stated once and relied on
throughout: **the pin-map key for each kind is that kind's existing C2
vocabulary term** — display *names* for options, *keys* for properties and
types. That is not a new duality introduced by pins; it is C2's own
per-kind vocabulary ("property **keys**, option **names** — everywhere"),
with one resolution rule laid over all four kinds.

### 7.1 Mechanism (normative sketch for the SPEC revision)

**The tenet.** A label is an **opaque map key**. It must be non-empty and
unique within its namespace — nothing else. Its readability is a courtesy
to the reader, not a mechanism: resolution never parses a label, `#` has no
grammar, there is no suffix syntax to unescape. A string either
exact-matches a pin key, or it is a bare term of whatever the slot's C2
vocabulary says — an option name in a value, a key in a key position, a
full id in an id position (§9a, unchanged). This sentence dissolves most
questions below: every "what if the name contains/looks like X" case is
answered by "nothing is ever inferred from a label's shape".

Envelope gains one field, `pins`, placed where `refs` is today (legend
precedes use, §2):

```json
"pins": {
  "types":      { "recipe": "6b21f0e3cda913b84c1299aa" },
  "properties": { "manual_property": "6a7663db61fab21cd4b9e745" },
  "options":    { "status": { "High#4f2a": { "id": "bafy…4f2a", "name": "High" } } },
  "objects":    { "roman": "bafyreidfmzjh…" }
}
```

- `pins.types` — label → internal type key. Covers `type`, `templateFor`,
  the envelope `key` of a type document, `typeProperties[].objectTypes`.
  Labels are the snake_case slugs (§7.5a). **Population (§7.5's (a)
  decision):** canonical exports pin **every non-bundled key** — all new
  keys are BSON, and a bare slug rides a mutable `apiObjectKey`, exactly
  the silent re-aim class this dossier exists to kill; bundled keys
  travel bare (the derived table resolves them) *except* a suffixed
  bundled label (twin collision with a custom slug), which cannot resolve
  through the table and carries a pin like any other. API reads may opt
  down via `?pins=`.
- `pins.properties` — label → stored relation key. Covers `properties` map
  keys, `typeProperties[].key`, dataview `properties[].key` / `groupBy` /
  column `property` / sort and filter `property`, the `property` block's
  `key`, link-block `properties` entries. Labels are slugs; population is
  the same rule as `pins.types`: every non-bundled key pinned in
  canonical exports, bundled bare unless suffixed, `?pins=` opt-down on
  reads (§7.5a-5).
- `pins.options` — per property label: label → option entry. Covers
  select/multiSelect values in `properties`, filter `value`s, sort
  `customOrder` entries, and §2a vocabulary entries.
- `pins.objects` — the current `refs`, folded into the family (decided,
  §8). Same charset, same §9a rules, string entries only.

**Namespaces and charsets.** Labels are unique per namespace: one namespace
each for types, properties, objects; **one per property for options** —
mirroring the store, where every option lookup is scoped by its property.
"High" on `status` and "High" on `priority` live in unrelated namespaces:
neither is a twin, neither gets a suffix, and no rule below ever compares
labels across properties. Option,
property and type labels are arbitrary non-empty JSON strings of at most
64 Unicode code points (property/type labels SHOULD additionally be
identifier-shaped — letters, digits, `_` — so they remain typable in the
§6.2.1 filter-string grammar, which takes bare identifiers; a
non-identifier label is reachable only through the structured form, the
grammar's existing rule for colliding keys). Object labels keep §9a's
`[A-Za-z0-9_-]{1,64}` because they ride inside markup positions (mention
attributes, link destinations) that arbitrary strings would break.

**Entry forms.** `pins.types`, `pins.properties`, `pins.objects`: always
`"label": "<identity>"` (strings — on a miss these kinds fall back to the
**label**, not a name, so the entry needs no payload; the normative miss
rules are below). `pins.options`: `"label": "<optionId>"` when the label
equals the option's exact current name, otherwise
`"label": {"id": "<optionId>", "name": "<name>"}` — `name` present-but-empty
means the option's name *is* empty; `name` absent means unknown (a dangling
id). The object form exists only for options because options are the only
kind whose pin-miss fallback needs a *name* to resolve or create by —
property and type creation only ever proceeds from a definition
(`typeProperties`, a type document), which carries its own name and
format, so their pins stay bare strings.

**Label minting (export) — normative.** A pure function of the namespace's
(base, identity) pairs; entities are processed in ascending internal-
identity order so suffixing cascades deterministically; **store order can
never influence a label** (order-dependence is what made D2 a coin flip).

1. `base` := the entity's C2 term: an option's display name; a property's
   or type's **slug** — for bundled keys the derived table entry
   (`snake_case(key)`, §7.5a-1), for every non-bundled key the stored
   `apiObjectKey` (which for API-created ones equals the caller's
   snake-normalized key by mint — §7.5), and only for pre-backfill keys
   with no stored slug `snake_case(transliterate(name))` derived at
   export time and never written back; an object's name slugified into
   the object charset.
2. A suffix is **required** when: `base` is empty; `base` exceeds 60 code
   points (truncate to 60 first); or `base` is exactly equal
   (case-sensitive) to another entity's base in the namespace — in which
   case **every** holder of that base gets a suffix; none keeps the bare
   name. Otherwise `label := base`.
3. `suffix` := the shortest trailing run of the entity's internal identity,
   minimum 4 characters, such that `base + sep + suffix` collides with no
   other label and no other base in the namespace; lengthen until unique
   (the git rule). `sep` is `#` for options/properties/types and nothing
   for objects with an empty base (a bare id-tail — exactly today's §9a
   suffix labels), `-` otherwise.
4. Case-sensitivity is exact everywhere, matching the store's own
   comparison (`storeresolver.go:108`): `high` and `High` are distinct
   names, distinct bases, distinct labels — no suffixes. (Notion dedupes
   options case-insensitively; Anytype does not, and the format follows
   the store.)

Per-case behaviour — mint on the left, what the resolver does on the right
(resolution is always the same exact-match lookup; only the minting and
the miss-handling differ):

| Case | Minted label | Resolution |
|---|---|---|
| unique name `High` | `High` | map hit → id. |
| twins `High`, `High` | `High#4f2a`, `High#9c1e` — both suffixed | map hits → the right twin each. A *bare* `High` on a write is ambiguous → 400 listing both labels (§7.4). |
| empty name | `#77d0` | map hit → id. On pin-miss: the entry's `name` is `""`, which cannot create (`createRelationOption` rejects empty names) → identity kept verbatim + warning. |
| name contains `#` (`C#`) | `C#`, verbatim | map hit. Never parsed; a minted label that would collide with it lengthens its own suffix instead. |
| name shaped like an id | verbatim | map hit. A bare id-shaped string in a value is a *name* — no shape heuristics, and post-D1 export never emits a bare id there, so the case cannot arise from our own output. |
| names differing only by case | both bare, no suffixes | distinct map keys, distinct store names — nothing to disambiguate. |
| name > 60 code points | first 60 + `#suffix` (suffix mandatory) | map hit; the full name rides the entry's object form. |
| dangling id (D1) | `#4f2a`, entry `{"id": …}` with no `name` | map hit → id unresolvable → identity kept verbatim + warning. **Never an option created from an id.** |

**Pin-miss semantics by kind (normative).** The table above is the option
rule; the flip to (a) makes the property/type miss a *hot path* — every
cross-account import of a canonical document misses on foreign BSON pins —
so it is specified, not left to the implementer:

- **Options**: resolve-or-create by the entry's `name` under the §7.4 verb
  rules; no name → identity verbatim + warning (the table above).
- **Properties and types**: a pin whose stored key resolves to nothing
  **degrades to its label** and re-resolves through the §7.5a-5 chain,
  with a warning naming the label and the lost key. It never writes the
  pinned key verbatim — a BSON in a key slot is the D1 shape — and never
  hard-errors on its own, which would break cross-account restore.
  Whether the chain's step 4 then *creates* is the §7.4 kind axis:
  creation proceeds only from a definition (`typeProperties`, a type
  document — minting a fresh BSON with `apiObjectKey` = label, §7.5);
  a bare reference follows the §8.1 policy (API: reject with
  did-you-mean; package-level import: the label passes through as the
  key for the wiring to reconcile, today's §3 degradation). Import
  wiring therefore lands **definitions before references**, so a batch
  resolves its own vocabulary — chain step 2 hits the slugs the batch's
  definitions just minted.
- **Objects**: §9a unchanged — never created; unresolvable refs dangle
  with a warning.

**Writer rules** (a hand-authored or agent-edited document): a label is
only a label if it has a pin entry — a string without one is a bare term of
the slot's vocabulary, full stop. When authoring pins: any label within the
kind's charset and length; unique in its namespace; it MUST NOT equal a
bare unpinned term of the same kind and scope used elsewhere in the
document (the legend-wins rule would capture that term — validation
rejects the document naming both sites); use the object form whenever the
label is not the option's exact current name.

**Worked example** (two same-named options, one unnamed option, one
unnamed object):

```json
{
  "version": 1,
  "type": "task",
  "pins": {
    "properties": { "priority": "6a7663db61fab21cd4b9e745" },
    "objects": { "roman": "bafyreidfmzjh…", "x7ke": "bafyreiuv…x7ke" },
    "options": { "status": {
      "High#4f2a": { "id": "bafy…4f2a", "name": "High" },
      "High#9c1e": { "id": "bafy…9c1e", "name": "High" },
      "#77d0":     { "id": "bafy…77d0", "name": "" }
    } }
  },
  "properties": { "name": "Q3 report", "priority": 2,
                  "status": ["High#4f2a"] },
  "blocks": [ { "type": "paragraph",
    "text": "Review with <mention objectId=\"roman\">Roman</mention>, cf. <mention objectId=\"x7ke\">(untitled)</mention>" } ]
}
```

Same-space restore: every pin resolves by identity — the twins stay twins
(the `ANOMALIES.md` #6 loss class closes), the unnamed option survives.
Cross-account import: the option ids miss; `High#4f2a` and `High#9c1e`
fall back to their entries' `name` and **collapse into one created
"High"** (correct there — the twins are indistinguishable to a human in
the target space); `#77d0` has an empty name, cannot be created, and is
dropped with a warning naming it. The property pin: same-space,
`priority` resolves through its pin to the BSON relation; cross-account
the BSON misses and the pin **degrades to its label** — `priority`
re-enters the §7.5a-5 chain and resolves against the vocabulary the
batch's own definitions just created (a type document's `typeProperties`
entry minted a fresh BSON stamped `apiObjectKey: priority`), warning if
the batch carries no such definition; the BSON is never written as a
key. Object pins follow §9a (never created — `roman` resolves or dangles
with a warning). Every outcome is visible in `created`/`warnings`;
nothing happens silently.

**The D1 fix on export:** `optionName` misses stop emitting the raw id as a
name. The value becomes a minted `#suffix` label pinned to the raw id, plus
a C11 warning on API reads. The mix of names and ids in one slot becomes
representable, so it stops being invisible.

**Emission policies (this is the whole "profiles" story):**

| Shape | Pins |
|---|---|
| Canonical export / backup (`Marshal` default) | **all** — every option value, every non-bundled key (plus any suffixed bundled label). Lossless; a backup restored after a rename re-points correctly. |
| API v2 default read | all (the whole-document round trip is a document path and inherits the protection; cost = one legend line per non-bundled key and per distinct select value — zero on fully-bundled documents); `?pins=min|none` opts down. **Superseded: APIV2.md §8.27 removed the document WRITE path (PUT), so v2 has no round trip left to protect on the default read — TOKENS §6 puts pins in the export shape only.** |
| Outline / prompt / example shapes | none (matches `OmitIds`+labels today). |
| Agent-authored documents | none — pins are `x-output-only` in the schema; authors write bare names and keys, exactly as now. |

Pin-stripping (the deliberate portability lever) is defined, not ad hoc:
replace each pinned label with its entry's `name` where known, drop values
whose entries carry no name (warn), then delete `pins`. One format, one
resolution rule, two emission policies — G's honest core, without the fork.

### 7.2 What each scenario gets

- **Backup/restore:** lossless including twins, unnamed entities and
  renames (pins invert). The last accepted round-trip loss class
  (`ANOMALIES.md` #6) closes.
- **Cross-account:** unchanged mechanics (names/keys + create-missing on
  import), now with an explicit dial: keep pins for fidelity, strip for
  laundering.
- **API read:** one vocabulary, revised — option names unchanged, keys
  re-spell to slugs (§7.5a-4 amends C2's letter; 153 bundled keys change
  spelling); BSON keys disappear behind slug labels; D1 becomes a warning
  instead of garbage.
- **API write:** document creates (POST) inherit pin protection; bare ops
  get the §7.4 strict default. *(PUT was the other document write path;
  it was removed — APIV2.md §8.27.)* The rename race on bare ops now fails loudly
  *before* damage instead of minting twins.
- **Small models:** see nothing new on the read/author side; on writes,
  a typo'd option name becomes a path-addressed 400 with candidates
  instead of silent garbage — strictness *helps* the small tier (§7.4).
- **Integrations/sync:** every reference obtainable as (label, identity) —
  the ownerReference shape — without per-value unions.
- **Diff/merge:** pins give the differ stable anchors; a rename diffs as
  one pin-line change, not N value changes.

### 7.3 SPEC.md changes

1. §2 envelope: `refs` becomes `pins` with four kind maps (fold decided,
   §8); canonical order and pruning rules.
2. §3: option values become **labels** under the resolution rule; the
   silent fallback clause is deleted; duplicate-name collapse leaves §11's
   normalization list (it becomes a fidelity guarantee instead).
3. §9a: rewritten as §9 "Identity and pins" — the tenet, the namespaces
   and charsets, the minting algorithm, the entry forms, the writer rules
   (§7.1 above); block labels unchanged.
4. §2a: vocabulary entries may be labels; "duplicate names are a validation
   error" becomes duplicate *labels*; twin-named options become
   expressible.
5. §6.2: filter values / customOrder reference the same rule.
6. §11: round-trip guarantee strengthened (pinned documents round-trip
   twins, unnamed options and dangling option ids byte-stably).
7. §12: pin validation — charset/length per kind, label uniqueness per
   namespace, the label-shadows-bare-term error, `x-output-only`.
8. §13: `Options` gains a `Pins: all|min|none` emission knob and the
   strip operation; §15.3 closes with a pointer here.
9. §3 **key vocabulary flips to the slug** (§7.5a-5): the "camelCase
   stored keys… so documents resolve offline" rationale is rewritten —
   offline resolution now rides the bundled derived table plus pins for
   every non-bundled (BSON) key, with the space slug lookup as the
   in-account path (§7.5's (a) decision); the well-known properties
   table re-spells (`icon_emoji`, `icon_image`, `due_date`); every
   example in SPEC and FLAT follows.
10. APIV2.md ledger edits: **C2 revised** (snake_case keys + camelCase
    envelope + recorded carve-outs, replacing "no snake_case" — §7.5a-4),
    R9's op default (§7.4), C4's `refs` → `pins`; plus the mechanical
    sweep — `schemas.go` examples, served EBNF examples, SKILL.md, §8.x
    notes, eval-harness tasks.

Package semantics stay resolver-driven (create-missing remains what the
wired resolver does — SPEC §3 is unchanged as *import* semantics); the API
chooses resolvers per verb, which is where §7.4 lives.

### 7.4 Write-side resolution: strict where a stale read exists

The review's sharpest point, accepted in full: **an absent pin on a write
was the one silent direction left in the design.** The original draft kept
R9's create-missing default and bolted on an opt-out; that reproduces D1's
failure shape (silent vocabulary invention) at the exact moment the agent
is most likely to be wrong — writing back something it read before a
rename. Two orthogonal axes replace it.

**The kind axis — what may be created implicitly: options, and nothing
else.** The principle (the human's, and it is the right one): implicit
creation is legitimate only when the referencing string *is* the complete
definition. An option's content is its name (plus an assignable color) —
a bare name defines it fully, and every create is preceded by an
exists-by-name check within the property (`resolver.go:133`; it is what
makes §8.13's retries convergent). Properties carry a format and target
types; types carry layouts and recommended-property lists; objects are
whole documents — **a bare reference can never supply that content, so a
bare reference must never create one.** They are created only from
explicit definitions: `POST /properties`, a type document's
`typeProperties` entries (which are definitions — key, name, format — not
references), `POST /objects`. This gives §8.1's shipped
create-vs-reject table its principled justification, and it is why the
option column below has a permissive mode at all while the other kinds
never do.

**The verb axis — when implicit option-creation is permitted.** Airtable's
`typecast` shows where the switch belongs: on the write request. We go one
step further and bind the *default* to the verb, because the risk boundary
is the stale-read window, and that window only exists when modifying state
previously read:

| Kind | POST (create/import — nothing was read) | PATCH (read-modify-write; PUT was the other column until APIV2.md §8.27 removed it) |
|---|---|---|
| option names in values | **create-missing** (the §8.1/R9 behavior): bounded (`v2MaxCreatedOptionsPerPatch` = 64), validated-before-created, reported in `created`, previewed by dry runs — `guardCreateMissing`, APIV2.md §8.13, all unchanged | **strict**: unknown name → 400 `unknown_option`, did-you-mean over the property's labels, and the remedy named verbatim ("retry with create:true"); `create: true` restores create-missing under the same bound and ordering |
| option names in `typeProperties` | always creates — declaring a vocabulary IS the create statement | same (editing a declared vocabulary is explicit intent) |
| **ambiguous** bare option name (twins) | **400 always**, listing the minted labels — never resolved by store order, never a third twin, with or without `create:true`. Closes D2 on the write path; reads never resolve bare names (export emits labels) | same |
| property keys in object `properties` | reject + did-you-mean (§8.1, unchanged) | same |
| type keys | reject + did-you-mean (§8.1, unchanged) | same |
| object references | **never created from a reference — any path, any flag, ever** (normative; today this is true by accident, now it is a rule). An unresolvable ref is kept verbatim with a C11 warning — never a 400 (cross-space and not-yet-synced references are legitimate), never a create | same |

Consequences, stated honestly:

- **What it costs small models: one extra turn, rarely, and only on raw
  REST.** The wrapper tier pays nothing: its planned pre-validation pass
  (APIV2.md §7.4, the A2 guard) checks option names against the live list
  before sending, and the wrapper sets `create: true` deliberately where
  creation is the intent. A raw-REST small model writing a genuinely new
  tag eats one 400 whose text names the exact remedy — the
  generate→validate→repair loop is the format's own operating premise
  (SPEC §12). In exchange the small tier gains real protection: today a
  typo'd option name silently mints garbage; now it returns candidates.
- **Import and POST keep one-shot semantics.** The discovery examples
  (`schemas.go`) — `"status": ["In progress"]` on a fresh space — still
  work first try; cross-account import still rebuilds vocabulary. The
  sets-create path (POST /sets) keeps R9's ahead-of-data option creation.
- **This supersedes R9's blanket default for ops** — an APIV2.md
  decisions-ledger edit. It is possible precisely because nothing has
  shipped; after GA it would be a breaking behavior change.
- The PATCH prewarm/lock machinery (`prewarmCreateMissing`,
  `guardCreateMissing`) is unchanged in shape — it simply runs only when
  `create: true` is present.

### 7.5 The key strategy — argued to (b), then flipped to (a) on evidence

History, kept visible: the first pass here weighed (a) — BSON internal
keys always, the caller's key living only in the slug (v1's live shape) —
against (b) — the caller's key as the internal key (v2's live shape) — and
chose (b), crediting it with retry-idempotency-via-convergence and a
cleaner namespace. A review then falsified both credits and produced two
facts the weighing had missed. Each is verified below; together they flip
the decision. The framing correction stands unchanged: the pin-map key
being a *slug* for properties/types while options use *names* is C2's own
per-kind vocabulary, not a new duality. And a second §7.5a consequence
now frames the weighing: once the surface is slug-only, the internal-key
choice is **invisible to callers**, and the resolution *mechanics* — the
§7.5a-5 chain, the label rules — are identical under either strategy.
What is **not** identical is pin population and backfill scope: (a) pins
every non-bundled key in canonical exports and needs backfill for every
pre-slug custom key, where (b) would have pinned and backfilled less — a
real cost, small and bounded, and the decision survives it. The deciding
axis is object-lifecycle semantics, where the evidence is one-sided.

**What (b) actually costs — two data problems, verified in source:**

1. **Silent format merge, with no guard and no guard possible.** The
   "sequential format-conflict guard (§2a wiring error)" this section
   previously leaned on **does not exist**: `creatingResolvers.PropertyId`
   returns the existing relation on a key hit with the declared format
   ignored (`resolver.go:357-363`), and `createRelation` validates only
   that a format is present and a valid enum value (`relation.go:24-30`) —
   never against an existing same-key relation (§2.3-5). Sequential
   *explicit* creates are stopped by the existence guard
   (`propertyKeyExists`, `schema_write.go:431`) — an existence check, not
   a format check. And the concurrent case cannot be guarded at any
   layer above the derivation: format is not part of the derivation
   payload (`{SmartBlockType, InternalKey}` — `payload.go:18-26`), so two
   members offline-creating `priority` as `select` and as `text` converge
   into **one object whose format resolves in CRDT order**. The loser's
   objects then hold select-shaped values on a text relation — silent
   data corruption, not a naming inconvenience.

2. **Delete-then-recreate is a structural dead end.** Derived objects are
   never destroyed (§2.4-5): same key ⇒ same tree, forever. Traced
   end-to-end, recreating a deleted key lands in one of three traps.
   After a **v2 delete** (archive — `schema_write.go:536`), the existence
   guard cannot see the corpse — every store query silently injects
   `isArchived != true` (`database.go:109-123`) — so the create proceeds
   to `PutTree` on the still-existing tree and dies on a raw
   `ErrTreeExists` (propagated by `objectcache/tree.go:57-59`; only the
   installer tolerates it, `installer.go:128`). After a **UI delete**
   (uninstall), nothing filters `isUninstalled` (§2.3-6), so the guard
   refuses "already exists" — steering the caller to PATCH an object the
   user deleted. The only "success" shape is the reinstall path's
   **resurrection with the old format, name and options**
   (`installer.go:210-232`). Under (b) there is no API-layer fix,
   because the caller's key *is* the derived tree.

**The two credits (b) held, withdrawn on re-examination:**

- *"Free retry idempotency via convergence"* — misattributed. §8.13's
  "convergent on retry" comes from `OptionId` doing an exists-by-name
  lookup **before** creating (`resolver.go:133`) — a lookup pattern, and
  options are BSON-keyed (`opt-<bson>`, `objectcreator/util.go:32-44`),
  never name-derived. Property-create retries are covered by C8's
  `Idempotency-Key` and by the existence guard — both of which every
  strategy needs anyway. Derived-key convergence contributes nothing at
  the API layer.
- *"Bundled keys cannot retire, so (a) hides a mixed namespace"* — an
  artifact of pre-§7.5a framing. Once the surface is slug-only, internal
  keys appear nowhere; a namespace nobody can see cannot be "mixed". The
  bundled table serves bundled slugs, the store serves the rest, and the
  caller cannot tell — which is §7.5a's whole point.

**What (a) costs, with the fix already specified:** twin slugs on
concurrent creates — a NAMING problem. The machinery is already in this
dossier: suffix-disambiguated minting (§7.1), the union collision check
at mint (§7.5a-6), ambiguity-loud lookups (400 listing candidates), and
the optional deterministic re-slug sweep (§8-OQ3). Two members
offline-creating `priority` yield two distinct, healthy relations whose
*slugs* collide; the collision is visible, loud, and repairable — no
value is ever reinterpreted.

| | (b) caller key = internal key | (a) BSON identity + slug surface |
|---|---|---|
| concurrent same-key, different intent | one object; format merges in CRDT order — **silent corruption** | two objects; twin slugs — **loud ambiguity**, repairable |
| concurrent same-key, same intent | converge (nice, rarely load-bearing) | twin slugs; repair collapses or suffixes |
| delete, then recreate the key | `ErrTreeExists` raw error / "already exists" corpse / resurrection with old content | **clean create, fresh BSON**; slug policy names it |
| retry idempotency | C8 + existence guard (convergence adds ~nothing) | C8 + existence guard — identical |
| document / pin mechanics (§7.5a-5) | identical | identical — but (a) pins more (every non-bundled key) and backfills more; small, bounded |
| failure mode | **silent, data-level** | **loud, name-level** |

**Decision: (a).** Every new type and property mints a BSON internal key
(options already do); the caller's key becomes the `apiObjectKey` slug,
snake-normalized at mint; internal keys never surface (§7.5a). By the
dossier's own tie-breaker — a design that fails loudly beats one that
fails silently — this is not close: (b)'s failures are silent and touch
data, (a)'s are loud and touch names. The irony is worth recording:
**v1 had the right identity layer all along** (`property.go:208-229`) and
lacked only slug discipline; v2's key-pinning creates — the former (b),
`resolver.go:386-401`, `schema_write.go:235-240, 455` — are what now
change.

**What (a) requires that this dossier had not yet specified:**

1. **The slug layer is identity-bearing for the API, and its integrity
   is load-bearing, not hygiene**: union uniqueness at mint (stored
   slugs + stored keys + bundled-derived slugs), ambiguity-loud lookup
   everywhere (400 listing candidates), and the §8-OQ3 repair sweep as
   the concurrency backstop.
2. **Archived and uninstalled objects vacate the slug namespace** — this
   requirement assumes §8-OQ2's lean (vacate + re-slug-on-revive) and is
   what an implementer builds unless that lean is overturned; overturning
   it edits this item and nothing else. Delete-then-recreate is (a)'s
   headline win, so the mint-time existence probe must deliberately skip
   corpses — note today's guard has it exactly backwards (blind to
   archived, blocked by uninstalled — §2.3-6/§7.5-2) — and reviving an
   archived object whose slug has been re-taken re-slugs the *revived*
   object with a suffix, loudly.
3. **v2 code changes** (the strategy-(b) remnants): stop writing
   `RelationKeyRelationKey` from the caller's key
   (`schema_write.go:455`), stop deriving type uniqueKeys from document
   keys (`schema_write.go:235-240`), and `creatingResolvers.PropertyId`
   mints a BSON and sets `apiObjectKey` from the document's key instead
   of pinning it as the relation key (`resolver.go:386-401`).
4. **Implement the format check SPEC §2a promises** at the wiring: a
   `typeProperties` entry whose declared format contradicts the resolved
   relation errors, path-addressed (§2.3-5). Under (a) it covers the
   remaining sequential-declaration case; the concurrent case no longer
   exists, because keys no longer collide.
5. Backfill for existing objects (lazy on first API touch vs migration
   sweep) — its own GO issue, and **a prerequisite for §7.5a's
   slugs-always surface over old spaces** (a pre-`apiObjectKey` BSON
   relation otherwise has no stable bare-op address; §7.5a-6). It
   **gates build step 3's bare-op surface for old accounts** — §7.6
   states the ordering. Until it runs, the exporter derives labels at
   export time (deterministic within a document, pinned, so documents
   never depend on the store having a slug).
6. Re-pointing a slug stays possible — the §8-OQ1 lean (keep mutable,
   address-only), assumed here; overturning it edits this item and
   nothing else. It is v1 compat (v1 *is* shipped, unlike everything
   else here) and document-safe by construction: documents pin to stored
   keys, so re-aiming a slug can never re-aim a stored reference — the
   slug is a branch, the key is the SHA.

### 7.5a The surface rule: the slug is the only key the API speaks (adopted)

Human decision, evaluated and **adopted with three modifications**: *API v2
addresses types and properties by the api-key slug, always — one mechanism,
snake_case, no exceptions.* `dueDate` is `due_date` on the wire; bundled,
API-created and UI-created keys are indistinguishable to a caller — nobody
has to know which kind they hold. The rule is orthogonal to the
internal-key strategy — it holds under (a) and (b) alike, with identical
resolution mechanics and differing only in pin population and backfill
scope (§7.5) — which is what let §7.5 weigh lifecycle semantics as the
deciding axis; its (a) decision now says what sits beneath the surface.
Stated once,
for a reader to act on: **every place API v2 names a type or property —
path, body, filter string, sort, field list, document key slot — it speaks
the snake_case api key and nothing else; internal keys never appear on the
wire.** The six questions the review posed, answered:

**1. The mapping is total and collision-free today — but derived, not
stored, for bundled keys.** Installed bundled relations and types *do*
receive `apiObjectKey` — installs route through
`createRelation`/`createObjectType` (`installer.go:109-134`), which call
`injectApiObjectKey` with the bundled key — **but** old spaces predate the
detail and `systemobjectreviser` does not backfill it (its revised-keys
list, `systemobjectreviser.go:30-43`, has no `ApiObjectKey`), so the
stored detail cannot be the authority for bundled keys. The authority is a
**fixed derived table in code, both directions, built from the bundle**.
Collision check, run against the real bundle with the real `strcase`
library: **194 bundled relation keys → 194 distinct snake slugs; 29
bundled type keys → 29 distinct; zero collisions even under
case-and-separator folding.** No blocker. The same run proves string
inversion is not the reverse mechanism — `mediaArtistURL` →
`media_artist_url` → `ToLowerCamel` yields `mediaArtistUrl`, and
`_score`/`_final_score` do not round-trip — so the reverse is a table
lookup, never a case transform. Visible churn: 153 of 194 relation keys
and 5 of 29 type keys (`objectType`, `relationOption`, `spaceView`,
`diaryEntry`, `chatDerived`) change spelling on the wire.

**2. Reverse lookup without a cache: one bounded query per request, never
one per reference.** `apiObjectKey` is an ordinary detail (hidden,
longtext, `source: details` — `bundle/relations.json:1725`) and is
queryable through the same details-query path every v2 listing already
uses (`ListProperties` filters on `resolvedLayout` identically,
`discovery.go:246-265`); there is no dedicated index, so per-reference
point queries would each pay a scan. The right shape is the per-request
resolver pattern the codebase already has: `storeresolver.loadRelations`
primes id↔key maps from one listing (`storeresolver.go:47-72`) — but
`model.Relation` carries no `apiObjectKey`, so the listing switches to a
details query returning `relationKey` + `apiObjectKey` + name + format,
building slug↔key maps both directions once per request, bounded by the
space's relation count (tens to low hundreds). ANOMALIES #9 already
mandates prime-from-listing with cached point-lookup fallback; this is the
same discipline with one more column.

The query-per-request shape is **provisional, pending measurement — not a
principled stance**: a v2 subscription/cache will likely arrive for
efficiency eventually (the human's expectation, recorded). Constraints
when it does: **lazy and per-space, warmed after auth** — v1's
all-spaces pre-auth warm-up sits badly with scoped keys (§8.9/§8.10 space
grants); it must **fail toward a store query, never a stale answer** — a
stale slug→key map resolves a write against the wrong property, which is
precisely the silent-failure class this dossier exists to kill; and
types/properties are the cacheable layer (small, slow-changing,
invalidated by their own object events) while objects are not.

**3. The forgiving layer is separator-insensitive and lives server-side.**
Under slugs the likely model miss is `dueDate` for `due_date` — a
separator difference, not a case one. Rule: exact match always wins; the
fallback folds (lowercase, strip `_` and `-`) and matches the folded key
set; **two keys folding together → 400 `ambiguous_input` naming both,
never a guess** (the git rule; the fold check over the bundle is clean
today, so a future collision fails loud instead of silently re-pointing).
Server-side, not wrapper-side: every tier benefits (raw REST, CLI, MCP),
it is one implementation instead of N, and C4 already blesses server-side
leniency for block suffixes. The wrapper's case-fold becomes a subset.

**4. Scope: keys are snake; the envelope stays camel — deliberately.**
"Snake_case everywhere" means the *user vocabulary*: type keys and
property keys, wherever they appear. Envelope and DTO field names stay
camelCase per C2 (`spaceId`, `etag`), with `dry_run` and `has_more` the
existing recorded carve-outs (§8.8, C10); enum *values* stay lowerCamel —
`objectType` the layout **value** coexists with `object_type` the type
**key**, and that is intended. v1 has always mixed snake keys into a camel
envelope without saying so; v2 writes it down. This **revises C2's
letter** — its cell currently reads "no id/key duality, no snake_case" —
while keeping its spirit (one vocabulary, no duality): a decisions-ledger
edit, free because nothing shipped. C2's original camelCase rested on
"the format's stored keys"; the BSON class made stored-keys-as-surface
untenable, and the uniform slug is the repair.

**5. Documents follow the surface — pin population is §7.5's to decide.**
(The earlier headline here read "and pins shrink further" — superseded by
the (a) flip, which pins every non-bundled key in canonical exports.) C2
("one vocabulary: the format's") cuts both ways: the API serves AnyBlock
documents, so key slots in the *format* carry slugs too — `"due_date"`,
`"icon_emoji"` — and SPEC §3's "camelCase stored keys" rule is overturned
(a deliberate cascade, §7.3; the biggest consequence the decision's
one-line form hides). A bare key term (a string with no pin entry — §7.1)
resolves through a four-step exact-lookup chain — no shape heuristics,
ambiguity at any step fails loud: (1) exact stored-key match (legacy
readable custom keys, the `artist` class); (2) space slug lookup over
`apiObjectKey` (every non-bundled key — under §7.5's (a) decision API-
and UI-created keys are BSON alike, and both carry a mint-time slug);
(3) the bundled derived table — offline-safe, since it ships in
`pkg/lib/bundle` with every reader; (4) miss → the §8.1 per-kind policy.
Population follows §7.1: canonical exports pin every non-bundled key
(the lossless internal identity) plus any suffixed bundled label;
bundled keys otherwise travel bare; API reads may pin-min via `?pins=`,
since chain step 2 resolves slugs in-account without pins. The
coordinator's portability reading is confirmed: slugs are *more*
portable than stored keys — meaningful in an account that never saw the
original; cross-account create-missing **mints a fresh BSON and stamps
the slug as its `apiObjectKey`** (§7.5), so the readable slug is what
crosses accounts while the BSON stays disposable plumbing — the
laundering conclusion survives with the mechanism corrected — and even a
pin-stripped document still restores in-account through chain step 2.

**6. What gets worse — named honestly.**

- **The cascade into the format is the real scope.** Every SPEC/FLAT
  example, golden file, `filterstring` example and served EBNF example
  (`due_date < currentWeek()`), `schemas.go` worked example, SKILL.md,
  §8.x note and eval-harness task re-spells its keys. Mechanical, wide,
  and only free right now.
- **Mint-time uniqueness must check a union.** A UI property named "Due
  Date" slugs to `due_date` — colliding with bundled `dueDate`'s derived
  slug. The §7.5 hardening check must test new slugs against stored
  slugs, stored keys *and* bundled-derived slugs (v1's cache check
  partially covers this; the heart-side mint checks nothing — §2.3-1,
  sharpened).
- **Backfill is promoted from follow-up to prerequisite** for old spaces:
  a pre-`apiObjectKey` custom BSON relation has no stored slug, and
  deriving one from the *name* at read time is the HTML-anchor
  anti-pattern (unstable under rename). Until backfill runs, such keys
  resolve in documents via export-time pins but have no stable bare-op
  address.
- **Debugging vocabulary diverges from the wire**: logs, store dumps and
  CRDT changes say `dueDate`/BSON where the API says `due_date`. Real but
  small — v1 callers live with exactly this today; the discovery listing
  can carry both columns.
- **Snake-at-mint now normalizes the SLUG, not the internal key.** Under
  §7.5's (a) decision the internal key is a fresh BSON; what v2 must
  normalize to snake_case at mint is the `apiObjectKey` it stamps from
  the caller's key (v1 already does — `type.go:225`, `property.go:218`),
  on *create only* — resolution never rewrites anything. Derived-key
  convergence for API creates is gone by design (§7.5); in its place,
  `dueDate2` and `due_date2` normalize to one slug, so the sequential
  second create is refused by the union uniqueness check and the
  concurrent one becomes a twin slug caught loudly — the (a) failure
  shape, names not data.

**Verdict: adopt, with the three modifications** — the derived-table
authority for bundled keys (1), the server-side folding fallback with
loud collisions (3), and snake-at-mint on v2 creates plus the
union collision check (6).

### 7.6 What remains to migrate, and the build order

**Migration inventory — nothing user-facing exists.** No exported user
documents, no third-party consumers, no wrapper installs. The complete
list: the package's golden files and `testdata/` regenerate; the round-trip
corpus reruns under `cmd/anyblockroundtrip` (expect the anomaly-#6 class to
flip from accepted-loss to pass, and the acceptance bar to move to 100% on
that class); the §7.5a re-spelling sweep (SPEC/FLAT examples, `schemas.go`,
served EBNF examples, SKILL.md, §8.x notes, eval-harness tasks); APIV2.md
takes the ledger edits (C2's key casing, C4's `refs` → `pins`, R9's
op-default); SPEC.md takes §7.3. The one store-touching item is the §7.5
requirement-5 backfill (`apiObjectKey` for pre-slug custom keys in old
accounts — the single place where "nothing shipped" does not apply,
because the *stores* exist even though the format's consumers do not).
That is all. Every choice in this dossier
gets an order of magnitude more expensive the day a third party ships a
consumer — **this is the moment to make the format right.**

Build order (status marks 2026-08-08 — APIV2.md §8.22 is the as-built
record):

1. **Kill D1** (small, self-contained): `optionName` miss → minted
   `#suffix` label + pin + warning; import side: pinned-id verbatim
   passthrough + warning. *[open — needs step 2's pins]*
2. **SPEC revision + package** (§7.3): pins, the minting algorithm, entry
   forms, the label-shadowing validation; the slug key vocabulary (§9-item
   in §7.3) with the bundled derived table (both directions) and the
   §7.5a-5 resolution chain; regenerate goldens; rerun the corpus.
   *[open — except the bundled derived table, which SHIPPED early in
   `pkg/lib/bundle/apislug.go` (both directions, collision-verified,
   fold layer included) because step 3's union check needs it]*
3. **The slug surface + the (a) identity layer** (§7.5/§7.5a): retire the
   strategy-(b) remnants — stop writing `RelationKeyRelationKey` from the
   caller's key (`schema_write.go:455`), stop deriving type uniqueKeys
   from document keys (`schema_write.go:235-240`), mint BSON + slug in
   `creatingResolvers.PropertyId` (`resolver.go:386-401`); snake-at-mint
   for the slug **with the union collision check** (stored slugs + stored
   keys + bundled-derived slugs — the check ships WITH the mint it
   guards, never after it: a custom "Due Date" colliding with bundled
   `due_date` must be impossible from the first minted slug) and
   ambiguity-loud lookups; the per-request slug↔key resolver
   (details-query listing); the server-side folding fallback with loud
   collisions; the re-spelling sweep; the §2a format check the SPEC
   already promises (§7.5 requirement 4). **Ordering:** for fresh spaces
   this step is self-contained; over old spaces its *bare-op* surface is
   gated on step 5's backfill (§7.5 requirement 5) — pre-slug custom
   keys stay reachable through documents and pins in the interim, and
   the gate is only about bare ops naming them.
   *[SHIPPED except the re-spelling sweep: the mint + union check, the
   input chain incl. fold, the §2a format check and the ambiguity-loud
   lookups are live (APIV2.md §8.22), and a five-lens review pass
   (§8.23) unified every mint and query channel onto the one chain —
   union check fold-inclusive on all paths, document-body forgery
   channel closed, search/list/set inputs canonicalized, guards
   fail-closed; outputs serve the slug only where the stored key is
   BSON and the slug round-trips — the full slugs-always output
   (bundled keys re-spelling to snake on the wire, the SPEC §3
   vocabulary flip, schemas/goldens/SKILL/eval) is the remaining sweep
   and needs its own change; view-op set channels stay stored-key-only
   until then]*
4. **Write-side defaults** (§7.4): strict on PATCH, `create: true`,
   the ambiguous-name 400; APIV2.md ledger edits; wrapper pre-validation +
   explicit create intent. *[open]*
5. **Slug lifecycle** (§7.5): the corpse policy (archived/uninstalled
   vacate the namespace; fix the inverted existence guard — §2.3-6,
   §8-OQ2) and the backfill (lazy vs sweep — its own GO issue) that
   un-gates step 3 for old accounts; the §8-OQ3 repair sweep if
   telemetry warrants it. *[corpse policy SHIPPED (vacate + both live
   defects fixed + loud ambiguity floor); open: the active
   re-slug-on-revive half of the §8-OQ2 lean (no v2 revive endpoint
   exists; the loud floor covers UI bin-restores), the backfill, the
   §8-OQ3 sweep]*

## 8. Open questions — and the ones the no-compatibility constraint closed

**Closed** (decided in this revision; recorded so the closure is visible):

- ~~Fold `refs` into `pins`?~~ **Folded.** The only argument against was
  the shipped v2 C4 read shape — which has no consumers. One legend
  concept, four kind maps.
- ~~Pin-all vs pin-min on API reads?~~ **Pin-all**, `?pins=min|none`
  opt-down. The only argument for min was token caution; clean documents
  pay ~nothing, and pin-all is what protects the PUT loop. *(Reopened by
  APIV2.md §8.27: the PUT loop is gone, so the argument for pin-all on the
  DEFAULT read is gone with it — export-shape-only, per TOKENS §6.)*
- ~~Legacy id-as-name rescue on import?~~ **Dropped.** It was
  compatibility machinery for documents that were never produced outside
  this repo; the D1 fix means they never will be. Dev artifacts
  regenerate.
- ~~Options' own slugs?~~ **Decided:** option identity in documents is
  name + pin; `apiObjectKey` on options stays a v1-only surface, v2 never
  adopts it, and the hardening covers it only for v1's sake.
- ~~The (b) convergence acceptance?~~ **Mooted by §7.5's flip to (a):**
  new creates never converge — each mints a fresh BSON; the residual
  convergence is bundled installs, where it is the intended mechanism
  (§2.4-1). The question dissolved rather than being answered.

**Open** (need a human decision):

1. **`apiObjectKey` mutability** — narrowed, not closed: v1 *is* shipped,
   so freezing breaks released behavior; the slug is address-only, so
   mutability is survivable — but §7.5a raises the stakes (the slug is
   now the *entire* key surface: re-pointing one changes what every
   subsequent request means, though never what stored documents mean,
   since they pin stored keys), and under §7.5's (a) it is also the only
   readable handle a property has. Freeze at mint (Linear) vs keep
   re-pointing with the union uniqueness check. *Lean: keep mutable,
   address-only, documented — revisit if telemetry shows re-pointing in
   the wild. §7.5 requirement 6 assumes this lean; overturning it edits
   that requirement and nothing else.*
2. **The corpse policy — archived/uninstalled objects and the slug
   namespace** (§7.5 requirement 2): corpses vacate the namespace so
   delete-then-recreate mints cleanly; reviving an archived object whose
   slug was re-taken re-slugs the revived one with a suffix, loudly.
   Alternatives: block the create and steer to unarchive (loses (a)'s
   headline win), or refuse revival into a taken slug. Also covers
   fixing today's inverted guard (blind to archived, blocked by
   uninstalled — §2.3-6). *Lean: vacate + re-slug-on-revive. §7.5
   requirement 2 assumes this lean and is what gets built unless it is
   overturned; overturning it edits that requirement and build step 5,
   nothing else.*
3. **Twin-slug repair**: is the loud-ambiguity lookup error (the floor)
   enough, or add a deterministic re-slug sweep (suffix the younger by
   internal-key order — convergent, no coordination)? Elevated by the
   (a) decision: twin slugs are now the *only* concurrency artifact
   left. *Lean: floor first, sweep if telemetry shows real collisions.*
4. **Uniform strictness for integration scopes** — narrowed by §7.4:
   PATCH is strict for everyone; the remaining question is whether
   integration-scoped keys should make POST strict too (Airtable is
   uniform-strict). Touches the API-key-scoping design, not this format.
5. **Identifier-shaped property/type labels: SHOULD or MUST?** Largely
   settled by §7.5a — slugs are identifier-shaped by construction, and
   every minted label is a slug (possibly `#`-suffixed for twins, which
   the filter grammar cannot type — the structured form is the escape
   hatch, the grammar's existing rule). Remaining question: MUST for
   hand-authored labels too? *Lean: MUST for minted, SHOULD for
   hand-authored.*
