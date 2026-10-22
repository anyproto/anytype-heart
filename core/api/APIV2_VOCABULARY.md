# API v2 vocabulary: slugs on the wire, names at the edge, ids in reference slots

GO-7383. Design and adoption plan for merging the format branch's raw-name
re-spell (`go-7383-anyblockjson`, 19 commits after `1ccf34e7c`, handoff:
`pkg/lib/anyblockjson/HANDOFF_API.md`) into the API v2 branch, and the
vocabulary decision the handoff left to this layer.

Sections 1-5 settle the KEY axis — how a type or a property is spelled.
Section 6 settles the REFERENCE axis — how a slot that points AT an object
is spelled — which became a live question only when the any-block bump
brought derived ids (`type-<key>`, `participant-<identity>`). Read §6 for
the map of what is stored where; §1-5 for why keys are slugs.

---

## 1. Decisions

**D1 — Document bodies default to the api slug spelling.** Every AnyBlock
body the REST surface serves (object reads, type documents, templates,
dataview fragments) spells property and type keys as the stable snake api
key. A global request parameter — `?keys=name` — opts a request into raw
display names. Default `keys=slug`.

**D2 — The parameter is parsed once, in middleware, never per endpoint.**
It rides the request context into the one place each handler composes
`anyblockjson.Options`. Alternatives considered and rejected: binding the
vocabulary to the API key / session (rejected — one key is used for
different things), an HTTP header (workable but less friendly than a query
param; a param is visible in logs, curls and examples).

**D3 — Acceptance is wider than advertising, on every channel.** Writes
and addressing accept both vocabularies everywhere: document bodies via
the format's own forgiving chain (already true), and v2's non-document
channels (PATCH op keys, `fields=`, `/properties/{key}`, view-op columns)
via a new display-name resolution step (§4.3). Unknown or ambiguous is a
loud 400 that lists candidates. **Never** auto-create a property from an
unmatched key on a record write.

**D4 — The slug is minted once, at creation, and frozen.** Unchanged
behavior, now stated as the design's load-bearing property: rename
resilience comes from *persisting* the identifier at mint time, not from
any conversion rule. The mint path (the format's grammar-safe
`MintApiSlug` family) stays as is; a rename never re-derives the slug.
The measured 4% of option keys that are not derivable from their current
name is the proof the slug carries information a name does not — and the
reason emit must always be a table lookup, never a string transform.

Considered and rejected: freezing the RAW NAME at creation as the stable
identifier. Freezing, not spelling, is what buys rename resilience — the
two options are equally stable — and the frozen name loses on everything
else: after the first rename it is a stale name masquerading as the
current one (a slug visibly signals "identifier, not label"); it drags
every name pathology into the identifier grammar forever (spaces and
arbitrary Unicode in path segments, invisible characters, trailing
spaces, and a uniqueness-over-time hole — rename "Due date", create a
new "Due date", collide — whose fix reinvents minting). The format's
transliteration criticism (`완료 → wanryo` is a wrong reading) does not
transfer here: in the format the spelling was the presentation; in the
API the slug is a mnemonic handle with the real name beside it in every
`{key, name}` listing, so a clumsy romanization at mint is harmless.
Mint-quality improvements for non-Latin names would affect only newly
created properties (existing keys are frozen) and are orthogonal.

**D5 — The tool wrapper speaks raw names.** It requests `?keys=name` and
teaches the model names. No rename resilience is needed there: a tool
call's lifecycle is seconds — ask, receive, operate — and a rename racing
that window resolves to a loud not-found/ambiguous error, which is fine.

**D6 — The format at rest is untouched.** Documents in export/backup keep
raw names with in-document legends; that decision belongs to the format
and is settled (`NAME_ADDRESSING.md`).

### Why slug-default (the research, one paragraph)

A survey of ~22 public APIs with user-defined schemas found exactly one
that keys machine-facing bodies on raw display names by default, plus one
on titles — both with open issues filed because of it. Four accept names
as an opt-in, two of which discourage it in their own reference docs. The
dominant pattern among systems that solved renames is precisely D4: a
stable key minted from the display name at creation and frozen, with the
label freely renamable above it. Well-designed name-accepting APIs fail
*loudly* on a stale name (400/422, no silent retarget, no auto-create);
the silent variants are the documented horror stories. A minted semantic
slug also satisfies model-legibility: agent-tooling failures cluster on
*opaque* identifiers, not on `due_date`.

---

## 2. The layers, their responsibilities, and the one conversion point

There is no parse-and-replace anywhere in this design, and none may be
added. Key spelling is decided exactly once, inside the codec, through an
interface that already exists.

```
Layer 1  pkg/lib/anyblockjson (codec)
         Marshal / Unmarshal / fragment entry points.
         Every key spelling, both directions, routes through
         Options.Keys (KeyVocabulary):
            emit:   PropertySlug(storedKey) → wire spelling
            accept: PropertyKey(spelling)   → storedKey
         Implementations: bundled table (default, offline),
         storeresolver.Resolvers (space-aware, names post-merge),
         apiKeyVocabulary (NEW, §4.1 — the API's slug table).

Layer 2  core/api/v2 (REST)
         Composes Options per request and calls the codec — never
         rewrites a produced document. Middleware parses ?keys once
         (§4.2) and the composition picks the vocabulary.
         Owns the non-document key channels (listings, fields=,
         PATCH keys, path params) via resolvePropertyInput /
         keycanon — these gain the name step (§4.3).
         Internal read-modify-write (stateops/viewops) pins ONE
         canonical vocabulary (slug) regardless of ?keys; the
         parameter affects only served bodies.

Layer 3  core/api/wrapper (tools)
         HTTP client of the REST surface. Transforms tool ARGUMENTS
         (folds, relative dates, @me), never bodies. Sets ?keys=name
         on its reads; serves what REST returns. Zero translation.

Layer 4  cmd/apiv2eval
         Sits on the wrapper arm (auto-tracks manifest/instructions)
         and the ops arm (auto-tracks served schemas). Only its
         GRADERS hardcode wire spellings — the known silent
         false-pass trap; they move to fold-based asserts (§4.6).
```

Consequence: "slugs on REST, names on the wrapper" costs one small
vocabulary implementation and one query parameter — not two rewrite
passes. What is genuinely doubled is *contract*, not code: two documented
spelling modes on the read surface, each with tests.

---

## 3. Merge mechanics (do this first)

Both branches sit on the same blob-purged base (`52df8b7ab`); the format
worktree's line adds 9 exporter commits plus the 19 re-spell commits. A
true `git merge go-7383-anyblockjson` is blob-safe (no purged history
returns) and previews clean except:

1. Commit the working-tree WIP first (TableColumnHeaders work in
   `pkg/lib/anyblockjson/{export.go,table.go,schema/object.schema.json,
   tableheader_test.go}` plus the core/api and apiv2eval changes).
2. Merge `go-7383-anyblockjson`. Expected conflicts and resolutions:
   - `core/api/APIV2.md` — keep this branch's API decisions.
   - `core/block/object/objectcreator/relation_option.go`, `util.go` —
     both lines routed minting through the slug helpers: reconcile,
     prefer theirs where equivalent.
   - `pkg/lib/pb/model/models.pb.go` — regenerate from the auto-merged
     `models.proto`.
   - `pkg/lib/anyblockjson/export.go`, `table.go`,
     `schema/object.schema.json` — re-apply TableColumnHeaders onto
     their reworked export (their `table.go` change is 2 lines).
3. Gates: `go build ./...`; anyblockjson + core/api + wrapper + eval unit
   suites; `gbnf_accept_test` is the deliberate tripwire for grammar
   drift (§4.5).

Riders that come with the merge and need no API work: the 13 bundled
display-name renames plus the systemobjectreviser path that applies them
to existing spaces (read-only spaces skip loudly); nothing in core/api
matches on any of the renamed names (verified by sweep).

Note on ordering: between step 2 and §4.1 landing, REST bodies would
speak names (the post-merge storeresolver default). Land §4.1 in the same
PR-sized unit so the v2 test suite — which asserts snake bodies — mostly
stays green and the wire never flips twice.

---

## 4. Design details

### 4.1 `apiKeyVocabulary`

An API-owned `anyblockjson.KeyVocabulary` in `core/api/v2/service`,
built per request from the same property/type entries the listings serve
(`liveProperties` — stored key, minted slug, name, hidden), embedding the
space's `storeresolver.Resolvers`:

- **Emit** (`PropertySlug`, `TypeSlug`): answer the served api key from
  the entry table (bundled table for bundled keys). Table lookup only —
  never derived from the name (D4).
- **Accept** (`PropertyKey`, `TypeKey`): consult the slug table first
  (exact slug → stored key — required because the format's own chain no
  longer reads `apiObjectKey`, so the 4% non-fold-derivable slugs would
  not invert through the embedded resolver), then delegate to the
  resolver (stored keys, names, folds, old spellings).

Interface obligations to pin with tests, mirroring the format's:
inversion (everything emitted must invert — the slug-table-first accept
step is what makes this hold), no shadowing of the bundled table, live
stored key outranks a name binding. Legend behavior under slug emit
(verified): a SPACE-minted slug still writes a `property_internal_keys`
entry — the legend is owed to a package-only reader holding just the
bundled table, which cannot invert a space slug — while bundled-derived
slugs need none. So a slug-shaped body self-describes exactly as the
raw-name shape does; the test pins the entry's content, not its absence.

### 4.2 The `?keys` parameter

- Values: `slug` (default) | `name`. "Slug" is the industry word for a
  stable, URL-safe, human-readable identifier and matches
  `bundle.ApiSlug` internally; serving field names (`key`,
  `type_settings.api_key`) do not change.
- Parsed once in v2 middleware alongside pagination; invalid value is a
  400. Carried in the request context; the Options composition points
  (`GetObject`, type/template reads, `list_read`'s dataview fragment,
  outline) pick the vocabulary from it.
- Applies to READS (and to response bodies of writes that echo
  documents). The write/accept direction ignores it — both vocabularies
  always resolve (D3).
- Internal RMW marshals in stateops/viewops always use the slug
  vocabulary (one canonical internal spelling; fixes the
  `canonicalViewKey`-vs-document mismatch class by construction).
- Row `properties` in list/search responses keep echoing the caller's
  requested `fields=` spelling (already the contract); `row.Type` keeps
  the type key.

### 4.3 Name acceptance on v2's own channels (chain step 5)

`resolvePropertyInput` / `resolveTypeInput` gain, after the existing
stored-key → live-slug → bundled → `FoldApiKey` steps:

5. exact NFC display name (visible entries; two holders → loud
   ambiguity), then `anyblockjson.FoldKeyTerm` fold-class (the format's
   fold — case, `_`/`-`, whitespace, invisibles) with the same
   single-candidate-resolves / several-refuse rule.

Live stored key and live slug continue to outrank a name (a name may
never shadow an address). Did-you-mean candidate lists spell suggestions
in the requesting surface's vocabulary. This makes the read-modify-write
loop closed for a caller holding names: PATCH set/unset, `fields=`,
`/properties/{key}`, `/types/{key}`, view-op column keys all resolve.

### 4.4 Wrapper adaptation (names at the edge)

- Reads request `?keys=name`; `read`, `describe`, `find` and the view
  tools therefore show the model one vocabulary: names.
- `describe`: dedup its two sources by stored key (the type document
  half and the `/properties` rows both carry enough to resolve), fix
  `IsOutputOnlyProperty` to resolve the input through the chain before
  the predicate, and route option lookups through the resolved key so
  `properties/{key}/options` keeps working.
- Fold conveniences (`foldPropertyKey`, `foldTypeArg`, the formats map):
  index by fold-class of BOTH name and slug using `FoldKeyTerm`, so a
  model echoing either spelling — or the user's words — lands. Loud
  refusal when two keys share a fold class.
- `parseSortArg`: last token is the direction when it parses as
  `asc`/`desc`; everything before it is the key — `"Due date desc"`
  parses. Same treatment for `columns` splitting where needed.
- Compact filter strings stay identifier-only (the format's settled
  decision): teach the model that multi-word names are written with
  underscores (`Due_date`, `Дата_выполнения` — a mechanical join, not a
  derivation; the fold resolves it), and that names no identifier folds
  onto (`C++`, `50% done`) go to the structured filters array. The
  manifest text and examples say so explicitly.
- Re-spell every manifest/steer/tool example and description to names;
  delete the "served vocabulary is snake_case now" comments.

### 4.5 Grammar sync

The wrapper's GBNF `key` production is ASCII-only while the format's
EBNF identifier is Unicode (UAX #31). Post-merge, sync the GBNF to the
served EBNF (underscore-joined Unicode identifiers must be emittable);
`gbnf_accept_test` is the tripwire and must stay green.

### 4.6 Eval harness

- Graders stop exact-matching wire spellings: assert through
  `FoldKeyTerm` equality (or resolve through the codec), so they are
  correct on both arms and resilient to future vocabulary moves.
- Expected values per arm: wrapper arm sees names, ops arm sees the slug
  default. The `set-multiword-property` task stays the designated
  tripwire — its expected value moves to the name spelling on the
  wrapper arm.
- Update the stale "both surfaces spell snake_case" comments.
- Test files listed in the sweep (`tasks_test.go`, wrapper `*_test.go`
  suite) get the same treatment.

### 4.7 Documentation

- `APIV2.md`: C2 gains the boundary note (v2's own names stay
  snake_case; document bodies are vocabulary-selectable with slug
  default; the format at rest speaks names); §8.46's grammar-safety
  paragraph updated (this rename is exactly the case it said could not
  happen); record D1–D6 with a pointer to this file.
- `core/api/v2/SKILL.md`: the "so do documents" paragraph rewritten for
  the two modes; fold examples updated.
- `cmd/anytype/SKILL.md`: fold/forgiveness section updated (names now
  resolve; multi-word rules; underscore-join for filter strings).
- `core/api/v2/doc.go` + `make openapi`: state the two vocabularies and
  the parameter explicitly.
- Served examples in `schemas.go` / `schemas_ops.go` stay slug-spelled
  (they document the default) — add the `?keys=name` mention where the
  schema index is described.

### 4.8 Out of scope, recorded

- Option VALUES are addressed by name with create-missing semantics
  (R9/§8.1) — rename-fragile by design, guarded wrapper-side; revisit
  separately if it bites.
- v1 is untouched.
- A future `?keys=name` for row `properties` maps (today: caller's
  `fields=` spelling) — only if a consumer asks.

---

## 5. Work plan

Phase 0 — commit the working-tree WIP (its own commits, user's slicing).

Phase 1 — merge `go-7383-anyblockjson` per §3; resolve; gates green
except the known-to-move v2/wrapper/eval assertions.

Phase 2 — `apiKeyVocabulary` (§4.1) wired as the REST default; v2 suite
back to green with snake bodies; legend/inversion/shadowing tests.

Phase 3 — `?keys` middleware + Options plumbing (§4.2); internal RMW
pinned to slug; name-mode read tests (object, type, fragment, outline).

Phase 4 — chain step 5 on v2 channels (§4.3) + did-you-mean spelling per
surface; round-trip tests: read names → PATCH by name → read back.

Phase 5 — wrapper (§4.4): `?keys=name`, describe dedup +
output-only fix, folds via `FoldKeyTerm`, sort parser, manifest/steer
re-spell; GBNF sync (§4.5).

Phase 6 — eval graders (§4.6); run the eval to confirm no silent
pass/fail drift.

Phase 7 — docs + openapi (§4.7).

---

## 6. The reference axis: what is stored where

Added 2026-09-05, when the any-block bump (`37ca00d`) brought derived ids.
Status markers: **✅** served today (heart `acf339579`, any-block
`57f4add`) · **⏳** decided, not yet built.

### 6.1 Three namespaces, three questions

| namespace | answers | minted by |
|---|---|---|
| **key** | "what *kind* is this?" | `bundle.ApiSlug` for a bundled key, `MintApiSlugFromName` at create for a custom one — then **persisted** as the `apiObjectKey` detail (D4) |
| **reference** | "*which object* is this?" | the store, at object creation |
| **name** | "what does a human call it?" | the user |

### 6.2 The rule

> A slot that names a **kind** carries a key. A slot that points at an
> **object** carries an id. A name is a caption — never resolved, never an
> address.

The one deliberate exception is the participant, and it is the exception
that explains the rule. A type and a participant are both space-invariant,
but their invariant HANDLES differ: a type's is its key, a participant's is
its identity. So a type goes to a key where it names a kind and to an id
where it names an object, while a participant folds to
`participant-<identity>` everywhere — one string for one human across every
space, which a per-space composite could never be.

Rejected: spelling type references `type-<internal_key>` as the format
does. For a custom type the remainder is the bson stored key, so
`type-68f1a9c…` names a type that no `/types` route can address and no
`/objects` route can open — a third spelling beside the api key and the
store id. See `apiRefSpelling` (apikeyvocab.go) for the enforcement point.

### 6.3 The forms one type has

A custom type displayed as "Bug":

| form | value | lives in |
|---|---|---|
| store object id | `bafyrei…t8c` | the object's `id` |
| uniqueKey | `ot-68f1a9c…` | `uniqueKey` detail |
| stored internal key | `68f1a9c…` | `state.UniqueKeyInternal`; the uniqueKey's tail |
| **api key** | `bug` | `apiObjectKey` detail — persisted, minted from the name |
| display name | `Bug` | `name` detail |
| derived id | `type-68f1a9c…` | the FORMAT only, never the store |

A bundled type has the same shape with a readable stored key (`task`) whose
api key derives by `strcase.ToSnake`.

### 6.4 Where each form is served

| slot | store holds | API v2 serves | file bundle writes |
|---|---|---|---|
| envelope `type` | type key | ✅ api key `bug` | display name |
| envelope `type_internal_key` | — | ✅ stored key (every typed doc) | same |
| `template_for` | `ObjectTypes[1]` | ✅ api key | `type-<key>` |
| `object_types` (property doc) | type ids | ✅ api key | `type-<key>` |
| type doc envelope `id` | store id | ✅ store id | `type-<key>` |
| `query_source.types` | type **ids** in `setOf` | ✅ api key `bug` | `type-<key>` |
| `query_source.properties` | property **ids** in `setOf` | ✅ **bare stored key** | same |
| `default_type_id`, `default_template_id` | store id | ✅ store id | `type-<key>` |
| mention / link target | store id | ✅ store id | `type-<key>` if a type |
| property KEYS in `properties` | stored key | ✅ api slug | display name |
| property values, object format | store ids | ✅ store ids | store ids |
| property values, select / multiSelect | option ids | ✅ option **names** | option names |
| participant, anywhere | `_participant_<space>_<identity>` | ✅ `participant-<identity>` | same |
| `created_by` / `last_modified_by` | composite | ✅ `participant-<identity>#Alice` | same |
| file / image refs | store id | ✅ store id | store id |
| space refs | `<cid>.<key>` | ✅ short ref, full on `?ids=full` | n/a |

`setOf` is the slot that settled the argument, and the format has since acted
on it. It held a type id **or a property id** under one grammar — heart's own
refusal said so in its error text — so no schema could describe it and no
reader could tell an entry's kind. It is now promoted out of `properties`
onto the root as `query_source`, two typed lists, and the flat spelling is
refused. The list a value sits in is the marker; the entries need none.

The two lists take different spellings, and that is not an oversight. `types`
is a type-KEY slot, so it follows every other one (`template_for`,
`object_types`) — the derived id normally, the api key under our mode.
`properties` is the bare stored key, because a property has no address to
derive one from: a bundle carries no property documents, so there is nothing
for a `property-<key>` to be the address OF.

**That leaves one asymmetry a consumer will notice**: for a v2-minted
property the stored key is a bson id, so `query_source.properties` can show
`6a83296f…` where every other property slot in the same document shows
`due_date`. Neither vocabulary reaches it — not the api-slug default, not
`?keys=name`. There is no heart-side seam; the fix, if we want one, is
upstream in `queryPropertyKey`, and it has a real cost there: the dictionary's
used-key census counts this entry as the one place a property key appears
with no spelling anywhere in the document.

### 6.5 Endpoints

| endpoint | takes |
|---|---|
| `/spaces/{space_id}` | full space id or short ref |
| `/objects/{object_id}` | **store id** |
| `/types/{type}` | **api key** (also accepts stored key and display name) |
| `/properties/{key}` | **api slug** |
| `POST /search` — `type`, and a `type` filter leaf | **api key**, resolved to type ids server-side |
| filter `property` | api slug |
| filter value naming a participant | identity **or** composite; both accepted |
| `POST /queries` — `type` | api key, exactly one; the stored document states it as `query_source` |

### 6.6 The switches

| switch | moves | leaves alone |
|---|---|---|
| `?keys=name` | property + type keys → display names | every reference |
| `?ids=full` | block labels, space refs | object refs; participants ⏳ |
| `NoDerivedTypeIds` (server-side, always on for v2) | type references → store ids; type KEY slots → api key | participants |

`NoDerivedTypeIds` is set by `apiRefSpelling` at the four surfaces whose
bytes reach a caller: the object read, list and search rows, the views
fragment, and the applier's own marshal — that last one because its
after-documents are COMPARED against what the read served, so a
disagreement would diff a view op against a shape no read emits. The file
exporter deliberately does not set it: portable documents want the derived
id, which is why it cannot live in `storeresolver`.

### 6.7 Not yet built

Shipped since this section was written: the write-path P0, `query_source`,
the `set` → `query` rename, and the `NotEmpty` → `Exists` fix. What the
rename actually needed is recorded in §6.9, because the plan here was wrong
about the migration.

- Member-by-id route (v1 has one that already accepts both spellings);
  `?ids=full` honouring participants, or dropping the claim that it spells
  everything in full.
- Export `resolveSources` from package `subscription` so `querySourceFilters`
  stops owning a second copy. The copy is what let the GO-7404 regression be
  re-derived five weeks after its fix; the fix is in, the duplication is not.
- `query_source.properties` spells the bare stored key (§6.4). Decide whether
  to raise it upstream.

### 6.9 The `set` → `query` rename, and why it needs no migration

This section is here because §6.7 predicted a per-space migration and that
was wrong — and wrong in the dangerous direction, since running one is what
would have changed API v1.

Three facts settle it, and each was verified rather than assumed:

1. **v1 never calls `bundle.ApiSlug` on its serve path.** It derives its own
   spelling in `util.ToTypeApiKey` (`TrimPrefix("ot-")` then
   `strcase.ToSnake`). So the derived table can be repointed without v1
   observing anything.
2. **v2 discards the stored `apiObjectKey` for a bundled key** and derives in
   code (`servedKeyOf`). So old and new spaces answer alike from the first
   read, with nothing to migrate.
3. **v1 DOES serve the stored `apiObjectKey`** when one is present. Which is
   why repointing the detail — the migration §6.7 wanted — is precisely the
   action that would have renamed the type in v1 too.

So the override lives in `bundle.TypeApiSlug`, which the type table and its
injectivity guard read, and `ApiSlug`/`MintApiSlug` do not. The mint still
derives `set`, every space still STORES `set`, and only a v2 read says
`query`. Accept takes both spellings.

Two consequences worth stating rather than discovering:

- `type_settings.api_key` on the set type's own document still reads `set`.
  The codec emits that member verbatim from the stored detail, bypassing the
  vocabulary — so the one place a v2 document still says the old word is the
  slot whose whole job is to report what is stored. Arguably correct.
- One v1 delta is unavoidable: `PATCH /v1/…/types/{id}` renaming a custom
  type's key to `query` flips from allowed to refused, because such a type
  would now shadow the bundled one on v2. The refusal is the right answer.

### 6.8 Upstream behaviour worth questioning

In `object_types`, a target the space cannot resolve is written
`type-<the stored id>` — a derived id asserting a type key that never
existed, where verbatim passthrough would say "an address I could not
translate". It reaches the file exporter only; under `NoDerivedTypeIds`
the slot falls back to the vocabulary spelling, which leaves the id alone.
