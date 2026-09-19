# Round 5 — scenario findings and work order

Status: **measured, not fixed.** Four scenarios against build `78a737bbb`
(groups E, F, G plus migration/receipt fixes). Sibling to
[`APIV2_ROUND4_SCENARIOS.md`](APIV2_ROUND4_SCENARIOS.md), whose findings this
round was built to verify.

**[repro]** = I reproduced it myself against the live server, command given.
**[audit]** = an auditor's finding from the transcripts, not independently re-run.

9 sonnet actors, 4 opus auditors. Zero timeouts and zero 5xx in any scenario,
so nothing here is a contention artifact.

## Verdict on round 4

| # | verdict | basis |
|---|---|---|
| R4-1 `delete_type` orphaning | **FIXED** [repro] | object keeps `"widget"`; was a raw hex |
| R4-2 `delete_object` hex receipt | **FIXED** [audit 9/9] | slug on every custom-type delete |
| R4-3 `message_count` counted deleted | **FIXED** [repro] | `message_count` live + `lifetime_message_count` |
| R4-4 self-contradictory scope | **FIXED, residue** [repro] | `grant.restricted` added; `scoped`/`key_status` still say "scoped" |
| R4-5 space enumeration in authz probe | **FIXED** [repro] | `spaces:[]` + `space_count` by default |
| R4-6 `key.id` unlabelled | **NOT FIXED** [audit] | no served prose describes it; no `auth` schema kind exists |
| R4-7 icon not writable | **FIXED as legibility** [audit] | three served strings now say so; capability still absent |
| R4-8 `read_chat` invisible effect | **FIXED** [audit] | receipt carries state after the move |
| R4-10 default object type | **FIXED as legibility** [audit] | same hint; cost fell from ~15 calls to 1-2 |
| R4-9 empty message array | **WITHDRAWN** | my parse error — the key is `messages`, not `data` |

R4-1 was round 4's headline and it is properly fixed, with a dry-run warning
that counts the affected objects first:

```
DELETE /v2/spaces/{sp}/types/widget?dry_run=true
  -> '1 object is of type "widget"; they keep it, and reads still spell it
      "widget", but nothing new is created in it and it is not listed or
      filterable, until this same type is restored in the app'
DELETE (real) -> 200 ;  object still reads {"name":"thing one","type":"widget"}
```

Also clean this round: **zero hex ids where a slug belongs**, across all three
S6 transcripts. The only 24-hex strings served are in explicitly-labelled
`internal_key` fields and one schema doc example.

---

# Priority 0 — new

## R5-1 — an empty `<mention>` is accepted and then silently erased **[repro]**

Chat message text containing `<mention object_id="…"></mention>` with **no
body** returns 201 and is stored with the element removed entirely.

```
sent:  A empty-mention <mention object_id="totally-bogus-id-xyz"></mention> tail
read:  "A empty-mention  tail"

sent:  B bodied-mention <mention object_id="totally-bogus-id-xyz">Label</mention> tail
read:  "B bodied-mention <mention object_id="totally-bogus-id-xyz">Label</mention> tail"
```

A 201 receipt for content the server did not keep. The bodied form round-trips,
so the mint path works — the empty case is a data-loss hole, not a parse
rejection.

In the wild this produced a **false success claim**: an S4 actor posted its
task-8 summary with an empty mention, read back `"… Full thread in ."`, and
closed with "All tasks complete." Three occurrences in one transcript.

Refuse it, or fill the body from the target's name. Do not accept and drop.

## R5-2 — a mention's `object_id` is never validated **[repro]**

`totally-bogus-id-xyz` is accepted and persisted, including under `dry_run`.
`attachments` on the same body **does** resolve its targets and 404s correctly —
two adjacent fields, opposite strictness. Validate the target, or say in the
schema that mentions are not resolved on write.

---

# Priority 1 — new

## R5-3 — `move_property` cannot express "move to the end of the field list" **[audit 3/3]**

Round 4's reorder was a no-op because the target was already first. I rewrote
the task to move a field to the *end*. **It is still a no-op, for a new reason.**

All three actors sent `{"op":"move_property","property":"last_contacted","position":"last"}`
and got an empty ack with **the etag unchanged**:

| space | etag after `remove_property` | after `move_property` |
|---|---|---|
| Archive D | `76c7d30e` | `76c7d30e` |
| Archive E | `5566cd6d` | `5566cd6d` |
| Archive F | `8d0d9335` | `8d0d9335` |

`move_property` reorders **within a section**, as its schema says.
`last_contacted` was already last *in its section*, while the user's "end of the
field list" means past `created_date`, `creator` and `links` in the unsectioned
tail — which no operation can express.

Four separate problems, in order of severity:
1. There is no way to do what was asked.
2. The op returned success having written nothing.
3. The etag did not roll, so a caller diffing state sees no change and cannot tell
   whether that means "no-op" or "not applied".
4. No warning said the move was vacuous. All three actors read the type back and
   none noticed; two reported the task complete.

**At minimum, warn when a move changes nothing.** That alone converts a silent
wrong answer into a correct one.

## R5-4 — destructive warnings count archived objects as live **[audit 3/3]**

Both `delete_type` and `delete_property` report **6 objects** in spaces where
the user had already deleted three in task 2. Those three are archived
(`is_archived:true`), not erased.

> `"6 objects are of type \"crm_contact\"; they keep it…"`

A user who just deleted half the contacts and is then told six are affected has
been given a number they cannot reconcile. Count live objects, or say
"6 (3 archived)".

## R5-5 — `delete_object`'s receipt does not say what kind of delete happened **[audit]**

Returns `{"id","type"}`. All three S6 actors' deletes were **archive**, not
erase, and nothing said so; two actors spent an extra `get_object` to find out.
Task 9 asks the actor to report what it destroyed — this is the receipt it must
answer from, and it is silent on the one fact the question needs.

## R5-6 — `delete_property` warns with a count but no identities **[audit 3/3]**

Reached for the first time in five rounds, and it does warn, identically on dry
and real runs:

> `"6 objects hold a value of \"relationship_strength\"; those values stay readable and editable in place under that key, but set_properties gives no other object one, until this same property is restored in the app"`

Good. Two gaps:
- It **counts** but does not **name** the objects. Survivable on a toy space;
  on a real one the caller cannot act without a separate query.
- The "names the types listing the property" half is **NOT EXERCISED** — by
  task 6 no type still listed it, because task 3 had already removed it. A task
  that purges a property still attached to a live type is needed.

Also worth a product decision: after `delete_property`, every object still
serves the value under its old key, so "purge it entirely" is **not achievable**
through this API. The warning discloses this plainly, which is right, but there
is no operation that strips the values.

---

# Priority 2 — new

## R5-7 — `groups` is declared output-only and never served **[audit 2/2]**

`get-collection-views` returns no `groups` array even for a kanban over three
distinct topic values. The object schema marks `groups` `"x-output-only":true`,
so a caller reasonably expects it on a read. The board's buckets are therefore
unverifiable through the API; only the fact that `group_by` was recorded.

## R5-8 — a view's sort `direction` is not echoed on read **[audit 2/2]**

Sent `{"property":"article_author","direction":"asc"}`; read back
`{"property":"article_author"}`. The **default** kanban sort in the same
response does carry `"direction":"desc"`, so the omission is asymmetric and a
caller diffing intent against state cannot confirm what was stored.

## R5-9 — views omit the property they are organised by **[audit 2/2]**

A list view sorted by author does not include author in its columns; a kanban
grouped by topic does not include topic. Both are stored correctly; both render
without the field that gives them meaning.

## R5-10 — `lastOpenedDate` still served in camelCase **[audit]**

Carried forward from R3-f and still live, now observed in collection view
columns beside `created_date`, `last_modified_by`, `backlinks`.
`list_properties` serves `last_opened_date`. Copying the served spelling into a
filter is refused.

---

# What works, verified

Worth recording so it is not re-litigated:

- **S4 is a clean scenario.** `add_items`, `remove_items`,
  `get-collection-objects` and `get-collection-views` all did exactly what they
  claim, every one confirmed by read-back rather than receipt. First contact,
  no defects in the core paths.
- **Task 7 destroyed nothing** — "empty the collection without deleting the
  articles" left all eight objects intact in both spaces, and neither actor was
  tempted toward `delete_object` by anything in the surface.
- **`get-collection-objects` returns membership, not a view-rendered subset.**
  Proven by order: the default view sorted by name, the served order was
  insertion order. *Caveat: no view in this scenario carried a **filter**, so
  the dangerous case is still untested.*
- **Collection writes count their members** — `create_collection` returns
  `"items":3`; item ops return `items_added`/`items_removed`.
- **Atomic multi-op edits work** — both actors did task 3's swap as one patch
  with `remove_items` + `add_items` in a single `ops` array.

---

# Regression noticed

**S8's round-4 result "both actors learned their scope before writing" no
longer holds** [audit]. Both round-5 actors wrote first and called
`auth-whoami` later (C: write at call 3, whoami at 18; D: write at 4, whoami at
12). No 403 occurred, but only because the grant is all-spaces readwrite. The
capability is intact — nothing steers a caller toward using it, and the default
`space_count`-only response is not advertised anywhere a caller looks first.

---

# Ranked work order

1. **R5-1** — refuse or fill an empty `<mention>`; never accept and drop.
2. **R5-2** — validate a mention's target, or document that it is not resolved.
3. **R5-3** — warn when `move_property` changes nothing; decide whether
   cross-section moves need an operation at all.
4. **R5-4, R5-5, R5-6** — one pass over destructive receipts and warnings:
   exclude or label archived objects, say archive-vs-erase, name what is affected.
5. **R4-6** — describe `key.id`, the one round-4 finding still untouched.
6. **R5-7, R5-8, R5-9** — view read fidelity.
7. **R5-10** — the camelCase spelling, carried since round 3.

# Coverage after five rounds

Still never exercised: `create-template`, `upload-file`, `download-file`,
`search-global`, `validate`, and the ops `replace_subtree`, `move_block`,
`delete_block`, `set_cell`, `move_view`, `delete_view`.

Covered by `scenarios-v2.md` S1, S3, S5, S7, S9, S10 — none run yet. **S3 needs
its prompt carve-out applied** (the actor must be allowed to create local files,
since `upload-file` takes an absolute path).

Two sub-claims from this round that need a task to reach them:
- a filtered collection view asked for membership (R5-x caveat above);
- `delete_property` on a property still attached to a live type (R5-6).
