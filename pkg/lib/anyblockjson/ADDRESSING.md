# AnyBlock JSON — addressing: one identity story for five reference kinds

Status: **design dossier** (research + recommendation, no normative force) ·
2026-08-08 · GO-7383. Companion to `SPEC.md` (v0.7) and `FLAT.md`; feeds a
SPEC revision and closes SPEC §15.3. Every claim about current behaviour is
verified against source at the cited line; prior art was researched against
vendor documentation (§6).

**Verdict, up front.** Keep readable strings in every value slot — option
*names*, type/property *keys* — and generalize the §9a refs legend into a
per-kind **pin table**: an envelope map from the readable label to the
internal identity, with the §9a total resolution rule (label in the table →
pinned identity; label absent → it *is* a name/key, resolve-or-create).
Canonical export always pins; agent-facing shapes may prune; authors never
write pins. The two "profiles" the format seems to need are then one format
with one rule and two emission policies. The complementary platform move:
harden `apiObjectKey` into a unique, frozen-at-mint, per-space slug so the
API can resolve the same labels statelessly. Everything that fails today
fails *silently*; every failure in this design is loud.

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
it (the one accepted loss in the ≥ 99.86% figure).

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

## 3. Scenario analysis

What each consumer actually needs, and what breaks when the wrong identity
is picked. "Name" below means display name for options, readable slug for
types/properties.

| Scenario | Identity that works | What breaks if you pick wrong |
|---|---|---|
| Full-account backup → restore (same space) | **id / stored key**. Everything resolves; renames since the backup are survived only by id. | Names: a rename between backup and restore mints a twin option and re-points values to it (silent); twin names collapse (D2). This is *the* case names cannot serve. |
| Export → import into another account | **name / self-contained key**. Foreign ids resolve to nothing — id-only documents are dead on arrival; create-missing (SPEC §3, §2a) rebuilds vocabulary from names/keys. | Ids: values dangle or, worse, collide with unrelated local ids. This is *the* case ids cannot serve. |
| API v2 read | name + key (C2), compact object refs (C4). | Raw BSON keys: unusable, unguessable, ~10 tokens each — v2's live state for UI-created properties. Raw ids in name slots: D1 reaches the API read surface. |
| API v2 write (agent authors JSON) | name + key, create-missing with did-you-mean (§8.1 policy). | Ids: agents mutate them in flight (−89% errors from short handles). Names alone: the rename race (§3.1) and twin ambiguity (D2). |
| Small-model authoring (wrapper tier) | names only, zero opaque tokens, no legend in sight (§7.1: "the model never sees or emits a 24-hex id"). | Anything composite or tagged: a 3B model strips suffixes and mangles unions; both forms then need accepting forever. |
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
| **Pins (recommended)** | Pin `"High": "o1"` resolves by id → correct; the label is cosmetic. Unpinned docs behave as today. | Document paths (PUT round trip) are protected — the read carried pins, the echo resolves by id. Non-document ops (`setProperties`) are *not* protected by the format — see §7.4 for the API-side answer (strict flag + `created` surfacing). Honest, not blurred. |
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
they are not the right *document value*.

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
| **E. pins** | **●** | **●** | ● (docs) ◐ (bare ops) | **●** | **●** (authors never see them) | **●** (zero when clean) | **med, additive** | **loud** |
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
  *write request*, not in the document — directly the §7.4 strict flag.
- **Kubernetes** — `metadata.name` is a reusable human handle;
  `metadata.uid` is identity; `ownerReferences` store **both** and GC
  honors the reference only when the uid matches, so a reused name can
  never re-bind a reference. Contribution: the pin table is exactly
  "store the uid next to the name"; a resolved pin whose target is gone
  must *fail*, not re-bind to a newer namesake.
- **git** — abbreviated SHAs are shortest-*unique* prefixes, checked
  against the actual object corpus, auto-lengthening as it grows, and an
  ambiguous abbreviation is a **hard error, never a guess**. Contribution:
  the label-minting rule for twins (§7.1) and the existing `matchBlockRef`
  behavior it validates.
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

**Adopt design E: readable labels in every value slot, one per-kind pin
table in the envelope, the §9a total resolution rule, loud failure for
unresolvable pins. Fold the existing `refs` legend in as the object kind of
the same concept. Complementary platform work: harden `apiObjectKey` into
the unique frozen slug that makes the same labels resolvable statelessly on
the API's non-document paths.**

### 7.1 Mechanism (normative sketch for the SPEC revision)

Envelope gains one field, `pins`, placed with `refs` before `blocks`
(legend precedes use, §2):

```json
"pins": {
  "types":      { "recipe": "6b21f0e3cda913b84c1299aa" },
  "properties": { "manual_property": "6a7663db61fab21cd4b9e745" },
  "options":    { "status": { "High": "bafyrei…o1", "High#4f2a": "6a76…o2" } },
  "objects":    { "miovm": "bafyreieqh63jv…miovm" }
}
```

- `pins.types` — label → internal type key. Covers `type`, `templateFor`,
  the envelope `key` of a type document, `typeProperties[].objectTypes`.
- `pins.properties` — label → stored relation key. Covers `properties` map
  keys, `typeProperties[].key`, dataview `properties[].key` / `groupBy` /
  column `property` / sort and filter `property`, the `property` block's
  `key`, link-block `properties` entries.
- `pins.options` — per property key: label → option object id. Covers
  select/multiSelect values in `properties`, filter `value`s, sort
  `customOrder` entries, and §2a vocabulary entries.
- `pins.objects` — the current `refs`, renamed into the family (see Open
  Question 1). Same charset, same rules.

**Resolution rule (total, per kind — verbatim §9a generalized):** where a
value of kind K is expected, if the string is a key in the kind's pin map it
resolves to the pinned identity; otherwise it *is* what the slot says — an
option name, a property key, a type key — with today's resolve-or-create
semantics (SPEC §3, §2a; APIV2 §8.1 policy). No shape heuristics, ever. An
unused pin is pruned by export; import ignores it.

**Label minting (export):** the label is the display name (options) or the
readable slug (types/properties) whenever that string is unique among the
document's emitted labels of that kind *and* does not collide with another
real name in the same scope. A twin gets `name + "#" + shortest unique
suffix (≥4) of its internal id`, lengthened until unique against both the
sibling labels and every real name present (the git rule; the §9a
key-vs-full-id collision rule). Because labels are opaque map keys, `#`
needs no escaping and no parser — a real option named `High#2` simply
forces a longer suffix on the minted label.

**Pin misses fail loudly, by kind:**

- `pins.options` label whose pinned id resolves to no option → import
  stores the pinned id **verbatim** in the value (the snapshot's own
  dangling state round-trips instead of laundering into a fake option) and
  reports a warning-grade issue naming property, label and id. **Never
  create an option named by an id** — D1 dies here.
- `pins.properties` / `pins.types` miss → create with the **pinned** key
  (restore fidelity wins; the created property carries the label as its
  display name). Stripping pins is the deliberate cross-account lever: a
  pin-stripped document creates the *readable* key — BSON keys launder into
  slugs exactly when the author chooses portability over fidelity.
- `pins.objects` — unchanged §9a.

**The D1 fix on export:** `optionName` misses stop emitting the raw id as a
name. The value becomes a minted label (`"#4f2a"`-style, no name half) with
a pin to the raw id, plus a C11 warning on API reads. The mix of names and
ids in one slot becomes *representable*, so it stops being invisible.

**Emission policies (this is the whole "profiles" story):**

| Shape | Pins |
|---|---|
| Canonical export / backup (`Marshal` default) | **all** — every option value, every non-slug key. Lossless; anomaly #6 becomes round-trippable; a backup restored after a rename re-points correctly. |
| API v2 default read | all (the PUT round trip is a document path and inherits the protection; cost is near zero on clean documents — see §7.4); `?pins=min|none` opts down. |
| Outline / prompt / example shapes | none (matches `OmitIds`+labels today). |
| Agent-authored documents | none — pins are `x-output-only` in the schema; authors write bare names and keys, exactly as now. |

One format, one resolution rule, two emission policies. G's honest core,
without the fork.

### 7.2 What each scenario gets

- **Backup/restore:** lossless including twins and renames (pins invert).
  The last accepted round-trip loss class (`ANOMALIES.md` #6) closes.
- **Cross-account:** unchanged mechanics (names/keys + create-missing),
  now with an explicit dial: keep pins for fidelity, strip for laundering.
- **API read:** unchanged vocabulary (C2 intact); BSON keys disappear
  behind slug labels; D1 becomes a warning instead of garbage.
- **API write:** documents (POST/PUT) inherit pin protection; bare ops get
  the §7.4 loudness levers. Stated plainly: **the format cannot rename-proof
  a bare `setProperties` write, and this design does not pretend to.**
- **Small models:** see nothing new. The wrapper already never shows ids.
- **Integrations/sync:** every reference obtainable as (label, id) — the
  ownerReference shape — without per-value unions.
- **Diff/merge:** pins give the differ stable anchors; a rename diffs as
  one pin-line change, not N value changes.

### 7.3 SPEC.md changes

1. §2 envelope: add `pins` (after `refs` or replacing it — OQ1); canonical
   order and pruning rules.
2. §3: option values become **labels** under the resolution rule; the
   silent fallback clause is deleted; duplicate-name collapse leaves §11's
   normalization list (it becomes a fidelity guarantee instead).
3. §9a: rewritten as §9 "Identity and pins", one rule for four kinds;
   block labels unchanged.
4. §2a: vocabulary entries may be labels; "duplicate names are a validation
   error" becomes duplicate *labels*; twin-named options become
   expressible.
5. §6.2: filter values / customOrder reference the same rule.
6. §11: round-trip guarantee strengthened (pinned documents round-trip
   twins and dangling option ids byte-stably).
7. §12: pin validation — charset per §9a, label uniqueness per kind,
   options sub-maps keyed by known property labels/keys, `x-output-only`.
8. §13: `Options` gains `PinOptions`/`AliasKeys` (or a single
   `Pins: all|min|none`); §15.3 closes with a pointer here.

The format is still a **draft with no external consumers** (§10 explicitly
licenses breaking changes) — this is the cheapest this change will ever be;
after freeze it becomes a version-2 conversation.

### 7.4 API surface cost (the C2/C13 audit)

- **C2 (one concept, one slot): respected.** Value slots keep exactly one
  vocabulary — the readable one, both directions. Identity occupies exactly
  one envelope slot, and it is a slot the surface already has (the C4 refs
  legend); folding `refs` in means the concept count goes *down*, not up.
- **C13 (strict schemas): improved relative to the alternatives.** No
  unions anywhere; `pins` is output-only and absent from authoring schemas.
- **Token cost:** zero on documents whose labels are all unique readable
  strings (the overwhelmingly common case — pins are only *required* for
  twins, BSON keys, and danglings; pin-all on reads adds one short line per
  distinct select value and is opt-out).
- **Write-path loudness (build items):** (a) surface the existing `created`
  side-effect array in the wrapper/SKILL so a minted option is *seen*;
  (b) an Airtable-style strict switch — `?create_options=false` (or
  `strict=true`) turning unknown option names into a 400 with did-you-mean,
  for integrators who prefer loud to permissive. R9's create-missing stays
  the default; the switch is the escape hatch the rename race needs on bare
  ops.
- **Non-document key resolution** (filter strings, sort keys, `fields=`,
  `setProperties` keys): aliases must resolve statelessly → requires §7.5.

### 7.5 The platform prong: harden `apiObjectKey`

The document story above needs nothing from the store. The *API* story —
readable keys on bare ops, v1 parity, one vocabulary — needs a per-space
slug that is **unique at mint and frozen by default**:

1. `injectApiObjectKey` checks the space index and disambiguates
   (`manual_property`, `manual_property_2`, …) instead of blindly stamping
   (fixes §2.3-1; also fix the un-snake-cased option branch, §2.3-3).
2. Backfill decision for existing objects (lazy on first API touch vs
   migration sweep) — needs its own GO issue; until then the exporter
   derives labels at export time (deterministic within a document, pinned,
   so nothing depends on the store having a slug).
3. Re-pointing stays possible (v1 compat) but is document-safe by
   construction: documents pin to *stored keys*, so re-aiming an alias can
   never re-aim a stored reference — the alias is a branch, the key is the
   SHA.
4. v2 emits the slug as the label wherever the stored key is a BSON;
   the interim "v2 reads apiObjectKey" fix converges into this.

### 7.6 Build order

1. **Kill D1** (small, self-contained): `optionName` miss → minted label +
   pin + warning; import side: pinned-id verbatim passthrough + warning.
   Ships value even before the rest.
2. **SPEC revision + package** (§7.3): pins emission/resolution, label
   minting, `Options` knob; round-trip tests over the anomaly-#6 corpus —
   acceptance target moves from 99.86% to 100% on that class.
3. **v2 read aliasing** for BSON keys via `pins.properties`/`pins.types`
   (subsumes the interim apiObjectKey read fix).
4. **Write loudness**: strict switch + wrapper surfacing of `created`.
5. **`apiObjectKey` hardening + backfill** (own issue, heart-wide).

## 8. Open questions (need a human decision)

1. **Fold `refs` into `pins.objects`, or keep both fields?** Folding gives
   one concept (C2's spirit) but renames a field the shipped v2 read
   surface already emits (C4). Pre-GA this is cheap; post-GA it is a
   compatibility carve-out. *Lean: fold now.*
2. **Pin-all vs pin-min as the API read default.** Pin-all protects the
   PUT round trip against the rename race; pin-min is leaner on token-
   sensitive readers. *Lean: pin-all, `?pins=min` opt-down.*
3. **`apiObjectKey` mutability**: freeze after mint (Linear) vs keep v1's
   re-pointing with a uniqueness check. Freezing breaks a documented v1
   behavior. *Lean: keep mutable as address-only, document that stored
   references never use it.*
4. **Legacy id-as-name rescue**: should import wiring resolve a bare value
   that exactly equals an existing option id of that property (a store
   lookup, not a shape heuristic — the §9a wiring allowance)? Rescues
   documents already polluted by D1. *Lean: yes, wiring-level, warn.*
5. **Options' own slugs**: options carry `apiObjectKey` today (v1 tags use
   it; format and v2 use names). Drop it from options, or harden it with
   the rest? *Lean: harden with the rest but keep it out of the format —
   option labels are display names.*
6. **Strict-by-default for integrations**: should some auth scopes (e.g.
   future read-write integration keys) default to `strict=true` on option
   creation, Airtable-style? Touches the API-key-scoping design, not this
   format.
