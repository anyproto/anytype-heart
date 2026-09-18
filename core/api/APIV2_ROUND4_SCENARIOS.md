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

## R4-9 — `get_chat_messages` read an empty array with a non-zero count **[repro, unexplained]**

In my own probe, messages posted successfully (200, ids returned) read back as
`array=0` while `message_count` showed them, then a later read of a different
chat showed 3 and a subsequent identical read showed 0. I could not explain this
and it may be my misuse of the endpoint (no ordering/window parameters passed).
Flagging it because if a freshly-posted message is not immediately readable, it
would also explain R4-3's symptoms differently. **Worth a deliberate check
before acting on R4-3.**

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
  `typeKeysById` and the row builder's tombstone fallback do the same for
  rows; the tombstone window works because `SnapshotOnDelete` keeps
  `uniqueKey` and `apiObjectKey`). Emit only: a create or a filter naming the
  slug is refused as REMOVED (`removedTypeBySpelling`), never as unknown
  with a guess, and never a hex; a live type that later takes the slug owns
  it, and the corpse reads under its stored key (the twin rule too).
  `delete_type` warns, on real and dry runs, how many objects survive it and
  what they keep, with a `list_objects?type=` reference.
- R4-2: `delete_object`'s receipt spells the type through the api
  vocabulary, as every other route does.

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
