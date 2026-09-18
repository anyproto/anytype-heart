# Typed hints, round two — findings and work order

Status: **measured, not fixed.** Two benchmark rounds (sonnet, haiku) against a
live build, then independent reproduction of every finding below. Each carries
the command that shows it. Companion to
[`APIV2_TYPED_HINTS.md`](APIV2_TYPED_HINTS.md), which is the design brief this
tested.

Build under test: `dffcdb5e8` (heart) + `8592284` (anytype-mcp local tarball),
server on `127.0.0.1:31009`, `Anytype-Version: 2025-11-08`.

## Verdict first

**The typed hints work, and they fixed the exact failure they were built for.**
The haiku round is the evidence; the sonnet round shows nothing because sonnet
had no headroom.

| | haiku before (`d34ca3ed6`) | haiku now (`dffcdb5e8`) |
|---|---|---|
| calls spent guessing an op's shape | **80** | **11** |
| all request-shape guessing | 80 / 212 (38%) | **25 / 130 (19%)** |
| `get_op_schema` calls | **0** | **9** |
| error responses | 119 (56%) | **29 (22%)** |
| typed-hint follow rate | n/a (0 of 15 for the URL form) | **5 / 8 = 62.5%** |
| tasks achieved | 12 / 30 | 11 / 30 ¹ |

¹ One actor (Delta) made **zero** API calls — it loaded tools via `ToolSearch`
nine times, then wrongly concluded they were "not directly callable" and gave
up. That is a harness/model failure with no API content. Excluding it, the two
runs that reached the API averaged **5.5/10 against the old round's 4.0/10**.

The mechanism's own numbers are unambiguous. No error hint anywhere reads
`GET /v2/…` any more. Epsilon's error rate across its first `get_op_schema`
call went **54.5% → 17.4%** — the Epsilon 70%→18% result from last round
reproduced, this time *caused* rather than lucky.

**But the tasks still do not come out right, for reasons that are mostly not
about hints.** 11/30 is the honest headline. The change made the API cheaper to
fight; it did not make it correct. F1–F3 below are why.

Sonnet, for completeness: **30/30, 162 calls, 11 errors** (was 27/30, 175, 2),
with `see_also` firing **once in 162 calls**. Do not read 30/30 as a win —
three sonnet actors differ by that much on temperature alone.

---

# Priority 0 — correctness and data integrity

These are not hint problems. They surfaced because the benchmark drove the API
harder than the tests do, and they outrank the hint work.

## F1 — `delete_property` leaks internal ids onto the public surface

**Severity: data integrity. Reproduced end to end.**

Deleting a property that is live on objects and referenced by a type's dataview
returns a bare `200` with no warning, and leaves the raw internal id in place of
the slug everywhere it was referenced:

```
before  defs: [alpha, beta, gamma, delta]     object: {"gamma": "VALUABLE"}
DELETE /v2/spaces/{sp}/properties/gamma  ->  200  {"id":"bafyrei…","key":"gamma"}
after   defs: [alpha, beta, 6aad3ad361fab22f055a8648, delta]
        cols: [..., alpha, beta, 6aad3ad361fab22f055a8648]
        object: {"6aad3ad361fab22f055a8648": "VALUABLE", …}
```

The value survives but becomes addressable **only by an internal hex key**,
which this API refuses as a property key elsewhere. In the benchmark this is
how an actor destroyed five objects' data: it saw the hex key on read-back,
did not recognise it, and unset it on every object.

Two distinct defects:

1. **No warning on a destructive delete.** The server knows the property is
   referenced — it rewrites those references. It should say so, or refuse
   without a force flag.
2. **Internal ids reaching the apiv2 layer at all.** The three-layer identity
   contract says internal keys belong to the backup layer. This is a leak
   across it, into `property_definitions`, into view `columns`, and into an
   object's served properties.

```bash
# full repro
curl -sX POST "$API/v2/spaces/$SP/objects" -d '{"type":"probe","name":"h","properties":{"gamma":"VALUABLE"}}'
curl -sX DELETE "$API/v2/spaces/$SP/properties/gamma"      # 200, silent
curl -s "$API/v2/spaces/$SP/types/probe"                   # hex id in defs and columns
```

## F2 — `property_definitions` replacement does not update the board; `add_property` does

**Severity: silent inconsistency. A/B verified in one space.**

Both paths return `200`. Only one keeps the type's own dataview consistent:

```
add Gamma via  {"ops":[{"op":"add_property",...}]}        -> defs +gamma, columns +gamma  ✓
add Delta via  {"property_definitions":[... , Delta]}     -> defs +delta, columns UNCHANGED ✗

final defs:    [alpha, beta, gamma, delta]
final columns: [name, created_date, ..., alpha, beta, gamma]
```

This resolves an apparent contradiction between the two rounds: sonnet reported
that a new property *did* appear as a board column without reopening the type;
haiku reported that it did **not**. Both were right. Sonnet used the op channel;
haiku resent the whole list.

So the op channel is not merely safer — it is the **only** path that maintains
the invariant. Meanwhile (F3) it is undiscoverable, so the actors that most
need it never find it.

Fix: make full replacement reconcile the dataview too, or refuse it. A write
that half-updates a type and returns 200 is the worst option.

## F3 — `add_property` / `remove_property` / `move_property` are undiscoverable

**Zero uses across 130 haiku calls.** Both actors mutated type fields by
resending the entire `property_definitions` list — the operation the schema
itself warns is a full replacement, and the one that breaks F2's invariant.
Their lists survived by luck; one slip and four fields vanish with a 200.

(Sonnet found the ops — 4 uses, zero `property_definitions` re-sends. The gap
is exactly the model class the hints were built for.)

The only place `add_property` is named to a caller is inside `schemas/type`'s
`property_definitions` description — **next to two raw HTTP routes** (F5). A
haiku actor read that string and then performed the rename by hand, destroying
a live property (F1).

## F4 — `dry_run` is implemented everywhere, declared on 18 of 26 mutators

**Severity: a rehearsal that commits. Verified both directions.**

C9 honours `?dry_run=true` globally — verified: `delete_type?dry_run=true`
returns `"dry_run":true` and the type survives. The OpenAPI omits the parameter
on three operations that implement it:

- `update_property` (PATCH)
- `delete_property` (DELETE) — the F1 endpoint
- `delete_type` (DELETE)

The generated wrapper mirrors the declaration, so it **drops the argument before
sending** and the write commits. The response is shape-identical to a real
write — no `"dry_run":true` echo — so the caller cannot tell.

This is not hypothetical: a sonnet actor's task-10 rename committed this way and
got the right answer only by luck.

Fix: add the annotation to those three, `make openapi`, re-run prose tests. Then
the general rule — **never silently drop an argument a caller sent.** An
undeclared argument should 400.

---

# Priority 0 — the hint mechanism's own ceiling

## F5 — `see_also` names a tool the caller has not loaded

**This is the round's most important hint finding, and it is new.**

The tool surface is loaded on demand through `ToolSearch`. A `see_also` that
names a tool the caller has not yet loaded requires a step the caller cannot
take from where it is standing — the same *shape* as the URL problem, milder
only because the tool is fetchable at all.

Cross-referencing each hint against what that actor had loaded:

| hint named a tool… | followed | rate |
|---|---|---|
| already loaded | 3 of 3 | **100%** |
| not loaded | 2 of 5 | **40%** |

**All three ignored hints named a tool the actor did not have**
(`API-get-schema` ×2, `API-list-properties` ×1). Both recoveries cost an extra
`ToolSearch` round trip.

So the 62.5% follow rate is not the mechanism's ceiling — it is the ceiling
imposed by tool residency. Two candidate fixes, pick deliberately:

- keep the small discovery set (`get_schema`, `get_op_schema`,
  `list_properties`) always resident in the wrapper; or
- have `see_also` carry a `load_with` field naming how to obtain the tool.

The first is simpler and removes the failure entirely. This belongs in the
[embedded-wrapper](APIV2_WRAPPER_IN_HEART.md) decision — it is a concrete,
measured argument for a curated resident tier.

## F6 — `see_also` is absent from most errors that actually happen

The design assumed the reference would ride on repair-shaped errors. It rides on
some. Every error below is one a real actor hit, and none carried `see_also`:

| error | seen | referent it should carry |
|---|---|---|
| `formatVersion is required` | 6× sonnet, 4 calls haiku | `get_schema {"kind":"object"}` |
| `did you mean is_favorite?` | 1× | `list_properties` — **same class as one that does** |
| `unknown key "properties" — the PATCH body carries only ops` | 3× haiku, 1× sonnet | `get_op_schema {"op":"set_properties"}` |
| `"key" is not allowed` / `"type" is not allowed` on create-type | 1× | `get_schema {"kind":"type"}` |
| filter `parse error at offset 19` | 1× | `get_schema {"kind":"filters"}` |
| `views` rejected in create-query | 3× | `get_op_schema {"op":"insert_view"}` |

The second row is the sharpest: `unknown property key` **attaches** `see_also`
on the list-all branch and **drops it** on the did-you-mean branch — the branch
a confused caller hits more often.

What already works, and is worth preserving as the model: the
`create_missing_options` error emits **two** reference kinds, including the
param kind the brief doubted —
`[{"op":"list_property_options",…},{"query":{"create_missing_options":"true"}}]`.

## F7 — twelve raw HTTP routes still served, through the schema-prose channel

The guard test covers `Hint:` strings and holds there. It does not cover schema
`description:` strings, which carry the same repair advice to the same callers —
and which is where a route-blind caller goes to recover. Live inventory (7
schema kinds + 17 op schemas):

| source | field | route |
|---|---|---|
| `schemas/type` | `property_definitions` | `PATCH …/types/{type}`, `PATCH …/properties/{key}` |
| `ops/add_property` | `property` | `GET …/properties`, `PATCH …/properties/{key}` |
| `ops/remove_property`, `ops/move_property` | `property` | `GET …/properties` |
| `ops/insert_view`, `ops/update_view` | `set.filter`, `set.filters` | `GET /v2/schemas/filters` (×4) |
| `ops/insert_blocks`, `update_block`, `replace_subtree`, `set_cell` | `$defs/block` | `GET /v2/schemas/object` (×4) |

Row one is the costliest string in the API: it is the only documentation of the
rename and add-field paths, it names two routes an MCP caller cannot issue, and
a haiku actor followed it by hand into the F1 data loss.

Extend the guard test to the description channel in the same change, or it
regrows.

---

# Priority 1 — messages that cost calls or corrupt state

## F8 — the op-schema `example` contradicts its own `description`

`get_op_schema {"op":"set_properties"}` returns:

```
description: "one entry of the ops array. The request body wraps entries: {"ops":[ this, ... ]} …"
example:     {"op":"set_properties","set":{"status":["Done"]},…}      ← unwrapped
```

An actor read this schema **before** erroring, copied the example to the body
root, and sent the same wrong envelope five times. **The API punishes the caller
who reads ahead** — and that is precisely the caller the hints are trying to
create. This is the direct cause of one actor's error rate *rising* after its
first schema fetch.

Fix: make `example` the full body, or add `example_body`.

Secondary: the response is 4,249 bytes, most of it a recursive `anyValue`
`$defs` blob ahead of the one-line description. For a small model that is the
whole signal budget spent on a union it never needs.

## F8b — every op schema's top-level `description` is byte-identical

All 17 op schemas return the **same** 181-character string (sha1 `9087ecd503a1`):

> `"one entry of the ops array. The request body wraps entries: {"ops":[ this, ... ]}, 1 to 512 of them, applied in order as one edit: if any one op is refused, none of them is applied."`

`get_op_schema {"op":"set_cell"}` says nothing about `set_cell`. Only the
`example` and the nested property descriptions distinguish one op from another —
and the example is the one that misleads (F8).

**This matters more than it looks.** The typed hints' headline success is that
callers now *reach* `get_op_schema` (0 → 9 uses). We fixed the routing; the
destination is thin. Every op should describe itself in its own first sentence,
with the envelope boilerplate stated once somewhere else.

```bash
for op in set_properties insert_blocks move_view add_items set_cell; do
  curl -s "$API/v2/schemas/ops/$op" | jq -r '.schema.description' | shasum | cut -c1-12
done   # five identical hashes
```

## F8c — served examples use the benchmark's own vocabulary

`add_property` and `move_property` examples name `harvest_season`;
`remove_property` names `sun_needs`. Those are plant-benchmark terms.

Two consequences, one minor and one not:

- The plant benchmark partly measures its own leaked examples, so its task-3
  result is inflated. (Known; recorded here with proof.)
- More usefully: examples drawn from a real domain are *better* than abstract
  ones, so the fix is not to genericise them — it is to stop reusing the same
  domain in the eval. The replacement scenarios in
  `docs/evals/anytype-mcp-v2/scenarios-v2.md` avoid all served vocabulary.

## F9 — `formatVersion` takes three round trips to learn one literal

```
"formatVersion is required"
"formatVersion must be a string in major.minor form"     ← still doesn't say which
"document formatVersion 3.0 is newer than the supported formatVersion 2.0"
```

6 of sonnet's 11 errors; 4 haiku calls. **The correct text already exists one
branch over**, on the same endpoint:

> `"to send a full AnyBlock document, include \"formatVersion\":\"2.0\""`

Copy it onto the required-field branch. `formatVersion` has exactly one legal
value; an error that withholds it is a riddle.

## F10 — the create-type error never says `format`, and its prose self-cancels

```
property "key" is not allowed — the member that names a property is spelled "property" in every
structure: a dataview's properties[] entry and the property block spelled it "key" earlier, twelve
lines from view columns, sorts and filters that spelled "property". Rename the member and keep its value
property "type" is not allowed
```

"spelled `property` in every structure … spelled it `key` earlier" cancels
itself, and "twelve lines from view columns" is not parseable. Critically
`"type" is not allowed` **never says the member is `format`** — so the actor
dropped the member entirely and **every field silently became `text`** where it
wanted `select`. The message cost that run its typed fields on task 1, and
everything downstream.

## F11 — errors report hex ids the caller never sent

```
"type \"6aad367a61fab22f055a85a3\" has no property \"6aad36b761fab22f055a85bd\" —
 known property keys of the type: condition, location, sun, watering_frequency"
```

The caller sent `"type":"plant"` and `"property":"harvest_season"`. It is told
about two strings it has never seen, **and the `see_also` args echo the hex**,
so following the reference propagates the unusable spelling. The useful half —
the known-key list — is buried behind them. Same identity leak as F1.

## F12 — `see_also.args` are not always sufficient for the named tool

Sonnet's single `see_also` named `API-create-property` with
`{"space_id":"h7ii3i"}`; that tool also requires `body`. Passed verbatim it is
rejected. Four of haiku's five followed hints *were* verbatim-usable, so the
contract mostly holds — but it must hold always, or the reference invites a
failing call. Decide whether `args` are send-ready and state it in the `Ref`
schema description, which currently promises neither.

## F13 — served prose names affordances the caller does not have

`list_properties` declares exactly one parameter, `space_id`, yet its own
success message says:

> `"43 matches — showing 25 from offset 0; request the next offset"`

There is no declared `offset`. The server honours one anyway (verified), so the
wrapper cannot expose it and a sonnet actor guessed it. The F12 hint compounds
this by saying *"list all with API-list-properties"* — it cannot list all.

Fix: declare `limit`/`offset` here and audit the other list endpoints.

## F14 — every write tool declares `body` with no schema and no description

`{"type":"object","additionalProperties":true}`, on every mutating tool.
Consequence: **34 of 162 sonnet calls (21%) were schema fetches** before any
work happened, plus the F9 failures.

Strongest input to the [embedded-wrapper](APIV2_WRAPPER_IN_HEART.md) research: a
curated tool with flat arguments has no envelope to get wrong and no schema to
fetch.

---

# Priority 2

## F15 — the If-Match hint misfires on every unknown PATCH body key

```json
{"path":"/properties","message":"unknown key \"properties\" — the PATCH body carries only ops",
 "hint":"the If-Match precondition is a header, not a body field"}
```

The message is right; the hint is about something the caller never sent. Fires
identically for `type_settings`, `blocks` and `properties`. Delete it, or fire
it only when the key is `If-Match` — and give the error F6's `see_also`.

## F16 — writes that accept impossible state

- **`{"default_view":"kanban"}` accepted** against a type with no kanban view
  and no group-by. 200, no warning, no view created.
- **`create_query` with no filter** accepted under predicate names ("Plants
  needing frequent watering") — three such objects created, each matching
  everything. The same server elsewhere volunteers *"a node with both is read as
  an empty group, which matches everything"*, so the detection exists.
- **`set_properties` writes a property the object's type does not declare**,
  returns `properties_changed:1`, and reads back cleanly — while remaining
  invisible to `fields`, to filters, and to `get_type`.

## F17 — `views` in the query schema is `anyValue` with no structure

Typed `{"$ref":"#/$defs/anyValue"}`, described only as "full view objects". An
actor guessed `groupBy`, then `groupProperty`, then abandoned creating the board
at create time. A `see_also` to `get_op_schema {"op":"insert_view"}` on the
rejection saves three calls.

## F18 — warnings are noise, and carry no `see_also`

22 instances across the haiku round, 20 across sonnet, `see_also` on **zero**.
Every `get_type` carries variants of:

> `"legend value: \"backlinks\" is internal: export strips it, so import does not accept it — so no legend entry is written for \"backlinks\"; the term is spelled verbatim"`

The caller never mentioned backlinks and cannot act on it. This is
export-pipeline vocabulary leaking into a read API — same layer violation as F1
and F11. The brief already noted hints on warnings are never *rendered* on the
curated wrapper; they are also not worth rendering in their current form.

## F19 — writes echo nothing a caller can check

`patch_object` returns `diff_stats`, and it is the best affordance in this API:
"change ONLY this sentence" was clean in **all five** runs across both rounds
because the server proved the constraint (`blocks_changed:1, added:0,
removed:0`). Nothing else does this. `update_property` → `{"id","key"}`.
`create_collection` → `{"id","type","etag"}`, no item count — and no run in
either round ever verified its collection's membership.

## F20 — `is_favorite` is a dead flag that accepts writes

No favourite/pin mechanism exists; `set_properties` accepts `is_favorite` and
`get_object` echoes it, but it appears in no schema and no `list_properties`.
Writing it and reading back `true` proves persistence, not function.

Improvement worth recording: **no haiku actor wrote it this round** (0
occurrences). One attempted a real substitute and was refused with

> `403 … "this refusal is permanent for this object — do not retry the same request"`

which stopped a retry loop dead. **That message is the model to copy** for every
permanent refusal.

Either implement the flag, surface it, or reject it on write. Silently accepting
a write that does nothing is the worst of the three.

---

# Status

Worked on branch `go-7383-typed-hints-followup` in groups, each group
reviewed by three fresh reviewers before the next.

**Group A (F1, F2, F3 hint, F4, F13) — done.**

- F1(a): `delete_property` returns warnings naming how many objects hold a
  value and which types list the property, on real and dry runs.
- F1(b): the corpse policy is REVERSED. A removed property's values, type
  list entries and view columns now spell its api slug, on every read
  surface and in every store shape — including the post-delete tombstone
  window, because `spaceindex.SnapshotOnDelete` now keeps `uniqueKey`,
  `relationKey` and `apiObjectKey` inside the tombstone's unindexed
  snapshot (a derived object is uninstalled, never removed from the tree;
  a cold recovery carried those keys anyway, so the local index carrying
  them too closes an asymmetry, not a contract). The accept side inverts
  only the corpse slugs a vocabulary itself emitted, so a document served
  with the slug re-imports onto the stored key (in-document edits and
  unsets work by the slug), a write to it off-document is refused as
  REMOVED, and a type definition or a create naming the slug mints anew.
  Create keeps carrying a pasted read body's values (§8.29: "a pasted read
  body creates a copy"), slug or stored key alike; the closed channel for
  a removed property is `set_properties` off the document. The stored bson
  key is never served any more. Recorded in the header of
  `corpse_addressability_test.go`.

  Reviewed by three fresh reviewers; their blockers and should-fixes are
  in: a definition echoing the slug a type's read served resolves to that
  corpse (the type's referenced corpses are remembered before resolving —
  `referencedCorpses`, `rememberCorpses`), the applier renders and
  re-imports through ONE vocabulary so a view edit keeps a corpse column
  on its stored key, a served corpse slug on a document is resolved before
  the forgiving name chain can fold it onto a live namesake, twin corpses
  sharing a slug are both demoted, tombstone emission passes the same
  guards as a listed entry, a replacement prunes the columns of detached
  removed properties too, dry runs report the prune, prune warnings spell
  served keys, the holder count is presence-based and includes archived
  objects, reconciliation errors propagate, `list_property_options`
  declares its pagination, and the delete warnings say what actually
  stays (the type's entry until taken off with `remove_property`).
  A second pass of three fresh reviewers found the remaining namesake
  paths and they are closed too: scoped resolution
  (`PropertyKeyCandidates`, the codec's first stop on a fragment
  re-import) returns the emitted corpse first, so a view rename beside a
  live property NAMED like the corpse keeps the column; a pasted read body
  beside such a namesake lands on the corpse (`canonicalizeDocumentKeys`
  and `keyCanon` resolve an exact removed slug before the display-name
  fold, unless a live key or slug takes it); tombstone emission registers
  its claim so twin tombstones never both serve one slug; the type-op
  planner carries the type's referenced corpses (`typePropertyList`), so
  `remove_property` accepts the served slug the delete warning pointed at
  and its prune warning spells it; prune warnings and delete warnings
  spell the SERVED key through the api vocabulary (a delete by display
  name promises the slug, not the name); the removed-property refusal
  says what is actually closed ("set_properties gives no object that does
  not already hold a value of it one") since a create still carries a
  pasted value.
  Accepted: in the post-delete tombstone window an off-document write by
  the slug is refused as unknown rather than removed, and a create by the
  slug is a plain 400; `?keys=name` renders a corpse under its stored key;
  tombstone slugs are not served by a service built without a creator;
  `fields=`, list and unscoped search filters validate against live
  properties and refuse a corpse slug as unknown (a canonicalized removed
  slug can surface as its stored key in that error — F11 territory, group
  D).
- F2: a flat `property_definitions` replacement now reconciles the type's
  dataview like the op channel: columns pruned for detached properties
  (with the same "columns dropped" warning), added for newly listed ones.
- F3: the replacement warning names the ops with a `see_also`.
- F4/F13: `dry_run` declared on `update_property`, `delete_property`,
  `delete_type`; `offset`/`limit` on the four list endpoints.

# Ranked work order

1. **F1** — warn or refuse on destructive `delete_property`; stop leaking
   internal ids into `property_definitions`, view columns and object properties.
2. **F2** — make `property_definitions` replacement reconcile the dataview, or
   refuse it.
3. **F4** — declare `dry_run` on the three mutators; adopt "never silently drop
   an argument".
4. **F5** — make the discovery tools resident, or add `load_with`. This is the
   difference between a 62.5% and a ~100% follow rate.
5. **F6** — attach `see_also` to the six branches that actually fire, starting
   with did-you-mean.
6. **F7** — structured referents for the 12 schema-prose routes; extend the
   guard test to the description channel.
7. **F8 + F8b** — fix the op-schema example, and give each op a description of
   its own. The hints now successfully route callers here; the payload they
   arrive at penalises reading ahead (F8) and does not say what the op does.
8. **F9** — copy the existing `formatVersion` hint one branch over.
9. **F10** — say `format`; rewrite the self-cancelling sentence.
10. **F3, F11–F20** — as scoped above.

## The guard test

Holds for `Hint` strings. Does not cover:

1. schema `description` fields (F7 — 12 live leaks)
2. success-path `message` fields (F13)
3. whether a `see_also`'s `args` suffice for the named tool (F12)
4. whether the named tool is reachable by the caller (F5)

Extend to all four together, or each regrows independently.

## What held up

- **The op-shape failure is fixed.** 80 guessing calls → 11; `get_op_schema`
  0 → 9 uses; one actor's error rate 54.5% → 17.4% across its first call.
- **No error hint contains a URL.** The re-spell works: `list keys with
  API-list-properties {"space_id":"…"}`.
- **The param-kind reference works** (`create_missing_options`).
- **`diff_stats`** (F19) and the **permanent-403 wording** (F20) are the two
  affordances worth copying everywhere.

## Caveat on the evidence

Sonnet's baseline was 27/30, so that round could not show an improvement and
did not; its value is the defect list, not the score. The haiku round is the
discriminating test and n=2, because one actor never reached the API. Treat the
per-task scores as indicative and the call/error/follow-rate counts as solid —
those come from every tool call in the transcripts, and the parse was validated
by reproducing the sonnet audit's totals exactly.

A scratch space `ZZ Scratch Probe` (`ksfgk4`) and the six Garden spaces remain
in the dev account from these runs; delete at will.
