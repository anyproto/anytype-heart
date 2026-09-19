# Round 4 — new-scenario findings and work order

Status: **measured, not fixed.** Three new eval scenarios run against build
`486d7018b`, covering surface that three previous rounds never touched. Sibling
to [`APIV2_TYPED_HINTS_ROUND2.md`](APIV2_TYPED_HINTS_ROUND2.md); the still-open
items from round 3 are carried forward at the bottom so this is the only
document a fix needs.

**Verification convention.** Findings marked **[repro]** I reproduced myself
against the live server in a clean scratch space, with the command given.
**[audit]** is an auditor's finding from the transcripts, strong but not
independently re-run. **[audit, unconfirmed]** means I tried to reproduce it and
could not — the auditor's evidence stands, mine was inconclusive.

What ran: 7 sonnet actors in parallel, 3 opus auditors.

| scenario | actors | surface, previously untouched |
|---|---|---|
| S6 spring-clean | 3 | `delete_object`, `delete_type`, `remove_property`, `move_property` |
| S2 chat | 2 | all 8 chat tools |
| S8 space/identity | 2 | `update-space`, `list-members`, `get-member-me`, `auth-whoami` |

---

# Priority 0

## R4-1 — `delete_type` orphans every surviving object behind a raw internal id **[repro]**

Deleting a type rewrites the `type` of every object that used it, from the api
slug to the type's 24-hex internal key. The objects survive carrying a label
nothing in the API resolves.

```
before   {"name":"thing one","type":"widget"}
DELETE /v2/spaces/{sp}/types/widget          -> 200
after    {"name":"thing one","type":"6aad7f9e61fab205fe53c2e8"}

GET /v2/spaces/{sp}/types/6aad7f9e61fab205fe53c2e8   -> 404
list_types                                            -> hex absent (14 types)
```

Reproduced in a clean space, and in all three S6 spaces independently
(`Archive B` gives a controlled A/B: byte-identical `list-objects` request
before and after the delete, `crm_contact` → `6aad7c0661fab205fe53c18a`).

**This is the same defect class group A just fixed for properties — it moved to
types rather than being eliminated.** The identity contract says internal keys
belong to the backup layer; this puts one on the apiv2 read surface, on the
object's most important field.

Asymmetry worth noting: the hex *is* accepted as a `?type=` filter value but
*not* by `get_type`. So the caller gets a token that works in one place and
404s in another, with nothing saying which.

Decide the intended post-delete contract. Options: keep serving the last known
slug; serve an explicit tombstone (`"type":null` plus a `type_deleted` marker);
or refuse the delete while objects reference it. Silently substituting an
unresolvable id is the one option to avoid.

## R4-2 — `delete_object` returns a hex `type` for custom types **[repro]**

Independent of R4-1: with the type alive and healthy, `delete_object`'s receipt
spells the type as a hex for **custom** types and as a slug for **bundled**
ones. `create_object` returns the slug for the same custom type.

```
DELETE object of type "gadget" (alive)  -> {"id":"bafyrei…","type":"6aad7fbf61fab205fe53c2f0"}
DELETE object of type "page"   (alive)  -> {"id":"bafyrei…","type":"page"}
POST   object of type "gadget"          -> {"type":"gadget"}
```

Nine occurrences across the three S6 actors. A response-shape bug, not a state
change — a later `get_object` on the same object still served the slug.

---

# Priority 1

## R4-3 — `message_count` counts deleted messages **[audit, unconfirmed]**

`get_chat_messages` returns a `message_count` that does not decrement on delete,
verified by the auditor four times across both S2 spaces with `has_more:false`
ruling out pagination:

| point | array | `message_count` |
|---|---|---|
| after 5 posts | 5 | 5 |
| after deleting one | **4** | **5** |
| after posting a summary | **5** | **6** |

A caller answering "how many messages are in the chat" from the field provided
for that purpose gets the wrong number. One actor got it right only by ignoring
`message_count` and counting the array.

My own attempt to reproduce this was unsound and is **not** evidence either way:
the messages array read back empty immediately after posting, so the delete had
no target. That failure is itself worth a look — see R4-9.

## R4-4 — `auth-whoami` describes an unrestricted key as "scoped", three times **[repro]**

```json
"grant":{"scoped":true,"all_spaces":true,"permission":"readwrite", …},
"key_status":"scoped"
```

`scoped:true` alongside `all_spaces:true` is self-contradictory on its face: the
caller must already know that "scoped" means *carries an explicit grant* rather
than *restricted to a subset*. Of everything in this payload it is the most
likely to make an agent under-reach and decline work it is permitted to do.

Rename, or state the reading in the field description. This matters more as the
api-key scoping work lands and genuinely-restricted keys appear alongside these.

## R4-5 — `auth-whoami` enumerates every space on the account **[repro]**

The authz probe returns the full space inventory — **15 entries** when I ran it,
including the five other concurrent actors' spaces by name. "What may I do" and
"what exists" are different questions; a permissions check should not be the
cheapest way to enumerate a workspace.

Consider omitting the per-space list unless asked (`?spaces=true`), or capping
it with a count.

---

# Priority 2

## R4-6 — `key.id` is served unlabelled **[repro]**

`auth-whoami` returns `key.id`, 64 hex characters, beside `key.name` and
`created_at`. I verified it is **not** the bearer token and not a plain
sha256/sha1 of it, so **this is not a credential leak**. But nothing in the
field or its description says whether it is a digest or an internal id, which
is exactly the ambiguity that makes a reviewer stop. Say which, in the schema.

## R4-7 — `update_space` cannot set an icon the read side advertises **[audit]**

```
PATCH space {"icon":"💠"} -> 400 'json: unknown field "icon"'
                             hint: "the update takes name and/or description — at least one"
list_spaces               -> {"id":"i5kude","name":"Get Started","icon_image":"bafybei…"}
```

Read serves `icon_image`; there is no write for it. The error is honest and the
space schema states the truth by omission (`additionalProperties:false`,
properties `name`/`description` only), but nothing says *space icons are not
writable here*, so one actor spent a call discovering it and the other silently
dropped a third of its task.

## R4-8 — `read_chat` has no observable effect **[audit]**

Returns `200 {}`; no read surface in the API reflects read state, so neither
actor could confirm task 6 ("mark everything as read"). An unverifiable write is
indistinguishable from a no-op. Either expose the read cursor on
`get_chat_messages`/`list_chats`, or have the receipt state what it moved.

## R4-9 — WITHDRAWN: my own parse error, not an API defect

**Retracted 2026-09-19.** The chat message list is served under the key
`messages`, not `data`. My probe read `data`, got nothing, and I reported an
empty array with a non-zero count. The endpoint was correct throughout. This
also means R4-3's diagnosis needed no re-sequencing — the auditor's evidence
for it was sound as filed.

## R4-10 — a space's default object type is inexpressible, expensively **[audit]**

Not supported, and nothing says so. It consumed **16 of 32 and 15 of 30 calls**
— half of each S8 run — for no result. Either support it or refuse it by name;
the current silence converts one unanswerable request into half a session.

---

# Status

Worked on branch `go-7383-typed-hints-followup` in groups, each reviewed by
three fresh reviewers before the next.

**Group E (R4-1, R4-2) — done.**

- R4-1: the post-delete contract is the one group A gave properties — the
  objects keep the type, and every read spells it by the slug the type was
  served under (`apikeyvocab.go ensure` adds removed types on the EMIT side;
  `typeKeysById` does the same for rows — all from the corpse row a delete
  leaves; the tombstone-window machinery this first carried is gone, see
  the corpse-row paragraph under Group G). Emit only: a create or a filter naming the
  slug is refused as REMOVED (`removedTypeBySpelling`), never as unknown
  with a guess, and never a hex; a live type that later takes the slug owns
  it, and the corpse reads under its stored key (the twin rule too).
  `delete_type` warns, on real and dry runs, how many objects survive it and
  what they keep, with a `list_objects?type=` reference.
- R4-2: `delete_object`'s receipt spells the type through the api
  vocabulary, as every other route does.

  Reviewed by three fresh reviewers (one blocker: a removed type's emitted
  slug folded onto a live type NAMED that way, so a search, a create and a
  delete by that slug went to the wrong type — the type resolution chain
  now stops at an exact removed spelling before its fold and name steps);
  their should-fixes are in: the removal lookup saw tombstones too (since
  removed with the tombstone shape itself, Group G), the markdown envelope
  spells the slug, a
  read of an object whose type was removed carries a `/type` warning
  saying so (the served marker the reviewers asked for), GET types by a
  removed spelling is a 404 that says removed, the delete warning spells
  the served slug however the type was addressed and says when a twin
  will demote it, its hint no longer references a `list_objects?type=`
  filter that does not exist, and the refusal for the old type's key names
  the live type that took its slug. Accepted: `?keys=name` bypasses the
  vocabulary.

**Group F (R4-3, R4-4, R4-5, R4-6, R4-7, R4-8, R4-9, R4-10) — done.**

- R4-3: `message_count` is the number of messages the chat HOLDS now (the
  RPC's live count — a deleted message leaves it); the total ever posted
  rides beside it as `lifetime_message_count`. The handler's description
  said "total since it began" and served exactly that; the field a caller
  answers "how many messages" from now answers it.
- R4-9: investigated. The chat repository sorts every read ascending by
  order id, so the served order was not the cause; the served order is now
  enforced in the service too, independent of the RPC. An empty array
  beside a non-zero count is consistent with the OLD `message_count` (the
  lifetime index) after deletes or during a history replay; it could not
  be reproduced offline. If it recurs with the live count, it is a chat
  store question, not an API one.
- R4-8: `read_chat`'s receipt carries the chat's `state` after the move
  (`unread_messages`, `unread_mentions`, `last_state_id`), best effort;
  the messages read's `state.unread_messages` reflects it too, and both
  descriptions say so.
- R4-4: `grant.restricted` is the boolean to branch on — true iff the key
  reaches only the spaces listed; false for an all-spaces grant and for a
  legacy key alike. `grant.scoped` keeps meaning "a grant record exists",
  and its description now says so instead of implying a subset.
- R4-5: an all-spaces grant reports `space_count` and lists the spaces
  only with `?spaces=true`; a restricted grant's list is its boundary and
  is always served. The gate-agreement test derives the boundary from
  `all_spaces`, never from the list.
- R4-6: `key.id` is described: the hex sha256 of the key's raw bytes, the
  id the key list in Settings shows, never the token itself.
- R4-7 / R4-10: the space update's refusals, the `space` schema kind and
  the PATCH description say that the space icon and the default object
  type are not writable through this API, so neither costs a session.

  Reviewed (together with the group E review fixes) by three fresh
  reviewers. One blocker: the removed-spelling stop in the type resolution
  chain relied on a BOUNDED scan of the deleted rows for a tombstone, so a
  tombstone past the bound let the spelling fall through to a live type
  named that way. Closed store-side at the time (superseded — see the
  corpse-row paragraph under Group G): the tombstone of a deleted type or
  property kept its layout top level as `deletedLayout` (a local bundled
  relation, sparse-indexed), so "every deleted type" was one exact query;
  the resolution chain fails CLOSED when a lookup errors, which stays. Their should-fixes are in: a tombstone demoted behind a
  visible corpse still reads as removed, the markdown envelope carries the
  removed-type warning, the reactions scope's receipt carries the state,
  the removed-type 404 states the diagnosis once, the live owner of a
  removed slug is checked for unambiguous visible ownership, the twin
  sentence describes both corpses, `space_count` is reported for every
  all-spaces grant (zero included), the `spaces` parameter refuses values
  other than true and false, `key_status` is explained beside
  `grant.scoped`, and the OpenAPI descriptions fit the prose guard again
  (the regenerated document had broken it). The external MCP wrapper keeps
  an operation's description beside its summary for v2 tools, so the
  update_space and chat rules reach that surface. Accepted: the resolution
  chain now pays the removed-type queries on a display-name-addressed
  input (two queries and a derived-id probe); `?keys=name` bypasses the
  read marker; list and search rows carry no removal marker.

**Group G (R3-a, R3-b, R3-c, R3-d, R3-f) — done.**

- R3-a: the views/top-level ambiguity carries a `see_also` to the query
  schema and says what to move where.
- R3-b: a view the TYPE channel minted is spelled in its receipt as reads
  spell it (compact by default, full with `?ids=full`) — the object
  channel's rule, through the same receipt compaction. The two channels
  keep their own receipt key spelling (`ops[i]` on objects, `/ops/i` on
  types), each matching its issue paths.
- R3-c: a created option's `property` in `created.options[]` is the served
  key, not the stored one, on every receipt (`creatingResolvers.created`).
- R3-d: `create_property` under a display name another property already
  carries is accepted (a name is not identity) with a warning naming the
  existing property's key and the repair. Type definitions and
  `add_property` resolve a display name to the existing property before
  minting, so they do not make twins.
- R3-f: the system query keys (`created_date`, `last_modified_date`,
  `creator`, `last_opened_date`) are accepted in both spellings and listed
  in the served one, as `list_properties` serves them.
- R3-e: `default_view` stays as documented under F16(a) (it governs how
  sets and collections of the type open).

  Reviewed (with the E/F review fixes) by three fresh reviewers. One
  blocker: tombstones written before the `deletedLayout` marker existed
  were invisible to the slug lookup, so the resolution stop could still
  miss one. Closed at the time (superseded — see below): the marker was
  derived from the tombstone's RETAINED snapshot, and the deleted-objects
  reindex counter was bumped so every space would backfill its tombstones
  on its next load. Found on the way: the index helper aliased
  its input slice, so adding one index dropped the first existing one on
  an upgraded space — fixed. Their should-fixes are in: a lookup error is
  an outcome of its own through the whole resolution chain (a 500 "could
  not verify the type — retry", never "unknown" with a guess, and never a
  free slug for a mint), receipts spell options on a property this same
  request minted and on a type op that imported nothing, `list_objects`
  lists the system keys as served, a name-only `create_property` under an
  existing display name is refused (an explicit key creates another, with
  a warning), the `spaces` parameter refuses an empty value, and the
  count wording is "the number of messages".

  A further fresh round found the migration wrong twice over (a deleted
  type or property keeps its tree, so the deleted-tree reindex the counter
  bump triggered never reached their tombstones, and a failed run recorded
  itself complete), and the replacement — a targeted backfill with a
  completion marker and a resolution gate that answered 500 until it had
  run — was then found to guard an empty set. Checked on real stores
  (every space of a 126-space account, pre- and post-wipe, and a prod
  space after an uninstall and a restart): a deleted type or property
  keeps its tree, its tombstone drops the indexed heads hash, and the next
  space load's outdated-object reindex rebuilds the full row from the tree
  — name, key and slug included, flagged uninstalled and deleted — which
  the removedTypes / removedProperties queries serve by row. A tombstone
  of a derived object therefore lives only from the uninstall to the next
  load on the uninstalling device; tombstones from before this branch
  carry no slug to match anyway and are gone after the first load. So:
  NO migration; the backfill, its completion marker, the gate, the
  tech-space call and the fixture override were removed again. A
  synchronous rebuild on load was tried and reverted (the personal
  space's types and properties are not flagged derived in head storage,
  and the API admits requests to a space that is still loading).

  The last step removed the second shape itself. A derived object's
  delete no longer strips its index row: `core/block/delete.go
  beforeDeleteDerived` closes the sessions and marks the smartblock
  deleted in memory, but does not call the store's `DeleteObject`, so the
  row stays the full corpse the `isUninstalled` Apply just indexed — the
  same row every other device holds and the same row this device would
  hold again after a restart. One shape, before and after a restart;
  `removedTypes` / `removedProperties` read it by row, and the
  tombstone-reading machinery THIS BRANCH had added went with the window:
  the `deletedLayout` relation and its index, the identity keys in
  `SnapshotOnDelete`, the tombstone lookups by slug and by key, the
  vocabulary's and the row builder's tombstone probes (develop's §8.41
  derived-id probes stay, for the rows below). The store's `DeleteObject`
  is unchanged — a deleted tree, an import's kept id, the marketplace's
  stale bundled templates — and the rows an OLDER build left for its
  deleted derived objects stay as they are until the next space load
  rebuilds them from the tree (the outdated-object reindex, a goroutine
  behind the reindex limiter, since that tombstone dropped the heads
  hash). Until then such a row has three faces, all accepted: an object
  read serves the stored key (a 24-hex for a space-minted type or
  property) with no removal warning, a list or search row serves an
  empty `type`, and a write naming either spelling is refused as unknown
  with a did-you-mean, name step included, never as removed. Full-text
  removal and subscriptions follow the Apply path, as they always did on
  every other device: the indexer returns no docs for a row flagged
  deleted and the queue consumer deletes the existing ones; the links
  row is no longer erased on the deleting device, which also matches
  every other device. Two side effects, both improvements: the deletion
  audit materialises a derived delete at once (the tombstone carried no
  `isUninstalled` for it to find, so "recently deleted" missed a type
  until the next load), and the row write now rides the indexer — a
  store-write failure during the delete leaves the object live in the
  index until the next load, where the unsaved heads hash rebuilds it.
  One backstop moved: `deleteRelationOptions` no longer stops at the
  first failing option (nothing re-runs it on the next load now that the
  relation keeps its heads hash).

  Also in from those rounds: a mint that suffixed or emptied its slug
  reports the stored slug (or the minted key) on the property row and its
  options, and creates the option against it; global search no longer
  files a server error under "space skipped"; `create_property` loads its
  property snapshot once, fails closed when it cannot, spells the
  twin-name holder from that snapshot, and refuses a name-only twin (an
  explicit key creates another, with a warning); its operation
  description carries the name rule into OpenAPI; the removal-lookup
  error is typed at the resolution boundary on every type-resolution path
  and on the bundled-type removal gate (the cause is logged, not served;
  a failure to load the live type list itself still serves its own
  message); the `property` kind states that names are not
  identities; and the curated `create_type` pre-flight prefixes "check
  the type name and formats" on a decoded validation refusal alone — a
  grant refusal, a rate limit, a 5xx, a transport failure or an
  undecodable reply of any status gets "the pre-flight failed" (the
  caller appends "nothing was created" itself). Open on that surface,
  carried forward: a transport or decode failure still serves the raw
  route inside the cause (`call POST /v2/...`), which the tool vocabulary
  pass does not render. Also found on the way and kept:
  `anystorehelper.AddIndexes` aliased its input (`indexes[:0]`), so any
  upgrade that added one index dropped the `uniqueKey` index.
  Accepted: the resolution stop's queries on display-name inputs are not
  memoised per request.

# What the fixes did achieve

Worth recording so nobody re-opens them:

- **`remove_property` is clean [audit, 3/3].** Values survive, remain queryable
  by slug, and `property_definitions` and view `columns` stay in slugs. The
  round-2 corruption on this path did not reproduce. Task 4 of S6 — "take it off
  the type but keep the data" — passed in every run.
- **Permission discovery works [audit, 2/2].** Both S8 actors learned their own
  scope from `auth-whoami` *before* writing anything; neither hit a 403 all run.
  The old "the only way to learn your scope is to fail a write" is fixed.
- **Six of eight chat tools verified working end to end [audit]**, including
  edit, delete, and reactions with `reacted_by`. `create-chat`,
  `add-chat-message`, `get-chat-messages`, `edit-chat-message`,
  `delete-chat-message`, `toggle-chat-reaction` all confirmed by read-back.
- **The identity surface works on first call [audit]** — all four tools, no
  warm-up, no malformed output.

---

# Carried forward — still open from round 3 and the surface audit

Not re-measured here; listed so this is the only document a fix needs.

| # | finding |
|---|---|
| R3-a | New bare error branch `400 ambiguous_input: provide views or top-level filter/filters/sorts, not both` — no hint, no `see_also` |
| R3-b | `created_views` returns a 24-hex id the read surface never serves (`5202192082156654fc70d2e5` → `0d2e5`); object channel returns the short id, and spells the key `ops[0]` vs `/ops/0` |
| R3-c | 53 hex ids in `create_type`/`update_type` receipts (`created.options[].property`) where `create_property` correctly returns the slug |
| R3-d | Two properties with the same display name accepted silently; 3 of 6 runs ended with two fields both called "Condition" |
| R3-e | `{"default_view":"kanban"}` accepted against a type with no such view, 200, no warning |
| R3-f | `lastOpenedDate` served in camelCase on types while `list_properties` serves `last_opened_date`; copying the served name into a filter is refused |
| R3-g | F5 tool residency unfixed — `see_also` still names tools the caller has not loaded (67% follow when loaded, 35% when not) |
| SA-a | 116 HTTP routes reach the caller, **100% in structured `endpoint`/`url` fields**, which the prose guard does not inspect |
| SA-b | Routes in prose were rewritten to bare server op ids (`list_properties`), a second vocabulary the MCP caller also lacks |
| SA-c | `schemas/object`, `schemas/template`, `schemas/type_document` return a byte-identical 46,950-byte schema — ~37% of the served surface is duplication |
| SA-d | 540 bytes of opaque `body:{additionalProperties:true}` on 10 tools commits the caller to a 45,314-byte schema fetch — **84:1** |

---

# Ranked work order

1. **R4-1** — `delete_type` orphaning. Data integrity, and the same class as the
   property bug just fixed. Decide the post-delete contract.
2. **R4-2** — `delete_object`'s hex receipt on custom types. Same family, small diff.
3. **R4-9 then R4-3** — settle whether fresh messages are immediately readable,
   then fix `message_count`. Order matters; R4-9 may change the diagnosis.
4. **R4-4** — the `scoped`/`all_spaces` contradiction, before scoped keys ship.
5. **R4-5** — stop enumerating every space in an authz probe.
6. **R3-b, R3-c, R4-6** — one pass over write receipts and identity spellings.
7. **R4-7, R4-8, R4-10** — say no honestly where the surface does not exist.

---

# Coverage after this round

Still never exercised by any round: `delete_property` (see below),
`create-template`, `upload-file`, `download-file`, `get-collection-objects`,
`get-collection-views`, `search-global`, `validate`, and the ops
`replace_subtree`, `move_block`, `delete_block`, `set_cell`, `move_view`,
`delete_view`, `add_items`, `remove_items`, `move_property`.

`scenarios-v2.md` S1, S3, S4, S5, S7, S9, S10 cover all of these and have not
been run.

**Two scenario bugs found by running S6, already patched in `scenarios-v2.md`:**

- **`delete_property` is unreachable as S6 was written.** All three actors
  loaded it and correctly chose `remove_property`, because task 4 says "keep the
  data" and the schema promises exactly that. Its shipped warning fix is
  **still unverified after four rounds.** S6 now carries a task demanding total
  removal.
- **`move_property` was a no-op** in all three runs — actors author
  `property_definitions` with company already first, so `{"position":"first"}`
  wrote nothing (empty ack, unchanged etag, 3/3). The task now requires a
  reorder that is not already satisfied.
