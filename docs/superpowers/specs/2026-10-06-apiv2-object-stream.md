# API v2 object stream (SSE live search)

Status: draft for discussion, 2026-10-06. Motivated by one agent use case: **discover every
discussion in which the agent was mentioned, including discussions that appear later**,
without polling and without opening every chat.

## 1. Problem

v2 has a per-chat SSE stream (`GET /v2/spaces/{space_id}/chats/{chat_id}/messages/stream`)
and nothing broader. `APIV2.md` lists "events/subscriptions beyond the chat stream" as
Deferred. An agent that wants "where was I mentioned" today has to:

1. `list_chats` per space (space chats only, no discussions),
2. read every object's `discussion` id to find discussions,
3. `GET …/messages?limit=1` per chat for `state.unread_mentions`,
4. repeat on a timer.

That is N+1 calls per poll, and the discovery of *new* discussions is a full re-scan.

## 2. What already exists (verified in code)

- **Counters live on the parent object, for discussions only.**
  `chatobject.writeUnreadCountersToParent` (`core/block/editor/chatobject/chatobject.go:416`)
  writes `unreadMessageCount` and `unreadMentionCount` as local details onto the discussion's
  parent object. `triggerParentUnreadUpdate` returns early unless the chat's type key is
  `discussion` (`:388`): **space chats (`chatDerived`) do not project counters anywhere**.
  The relations are hidden, read-only, `derived` (`pkg/lib/bundle/relations.json:1908`).
- **The subscription engine can stream a filtered set.** `subscription.SubscribeRequest`
  (`core/subscription/service.go:41`) takes `SpaceId`, `Filters`, `Sorts`, `Keys`, `Limit`,
  and `Internal` + a caller `InternalQueue` that receives `pb.EventMessage` values instead of
  the global client event bus. That queue is the SSE sink.
- **The engine accepts the key.** Filter formats for bundled keys resolve from the bundle
  (`spaceindex/relations.go:204`), so a number filter on `unreadMentionCount` needs no relation
  object in the space.
- **The chat stream is the transport template**: process-wide slot cap and
  `too_many_streams` (`v2/service/chat_stream.go:46`), sink buffer, heartbeat,
  `resync_required` for a dropped slow reader, grant check before subscribing.

## 3. Verification results

### Check 1 — can v2 search express `unreadMentionCount > 0`? **No, not as built.**

v2 validates filter, field and sort keys against `acceptKeys` = the space's *live relation
objects* (`liveProperties`, `service/keys.go:107`) + `v2SystemQueryKeys`
(`createdDate, lastModifiedDate, creator, lastOpenedDate`, `service/search.go:52`).
`unreadMentionCount` is in neither: it is not in any type's recommended relations and not in
the system-relation set installed into every space, and `SetLocalDetail` does not create a
relation object. **Confirmed by a throwaway test** (deleted) on the search fixture, with an object carrying
`unreadMentionCount=2`: both `unreadMentionCount` and `unread_mention_count` are refused with
`unknown property key … known property keys: created_date, creator, last_modified_date,
last_opened_date, mimeType, severity, size, type`, while `lastModifiedDate` with the same
condition is accepted. The fixture has no relation object for the key. Whether a *real* space
happens to hold an installed copy is not guaranteed by code (the key is in neither
`systemRelations` nor `internalRelations`, so it is not created with the space), so the fix
below does not depend on it.

The engine side is fine (above), so the gap is the v2 validation vocabulary only.

### Check 2 — does a discussion that syncs in from another member get its counter written? **Yes, with two bounded gaps.**

- The counter is written from `storeObject.onInit` and `onUpdate`, i.e. only when the
  discussion object is *loaded*.
- Sync loads it: the tree syncer's `requestTree` (missing trees) and `updateTree` (existing
  ones) call `treeManager.GetTree`, which is `space.GetObject`, a full smartblock load
  (`core/block/object/treemanager/treemanager.go:69`, `treesyncer.go:319`). Head-sync
  therefore populates counters for discussions nobody opened. The values persist in the
  store after the object is evicted from the cache.
- **Gap A, latency.** The counter appears when diffsync reaches the discussion. Priority
  ordering puts `chatDerived` before `discussion` (`core/block/service.go:416`), so a new
  discussion is the last class of tree to be pulled. "Newly appeared" means "after the next
  sync cycle", not instant. The spec states this on the route.
- **Gap B, parent not yet local.** `writeUnreadCountersToParent` does
  `Space().Do(parentId, …)` and only logs on failure (`logParentUnreadError`). If the
  discussion loads before its parent has synced, that write fails and is retried only by the
  next message or read in that discussion. The parent then shows no counter until then.
  This is a pre-existing behaviour; the stream inherits it and does not fix it (see §8).

Check 2 was not exercised against a running node; it rests on reading the code paths above.
Gaps A and B are accepted as-is (decided 2026-10-06).

## 4. Design

### 4.1 Route

```
GET /v2/spaces/{space_id}/objects/stream
    ?filter=…  |  ?filters=…     same grammar, validation and canonicalisation as v2 search
    &fields=…                    property keys to carry on each row (as search)
    &limit=…                     size of the opening snapshot (shared pagination bounds)
```

One stream per space (the engine subscribes per space). Same bearer auth and read grant as
search; the space must be in the key's grant before anything subscribes.

### 4.2 Events

| event            | when                                                          | payload |
|------------------|---------------------------------------------------------------|---------|
| `object_added`   | in the opening snapshot, and when an object enters the set    | the search row shape (`ObjectRow`), `fields` applied |
| `object_updated` | a carried field of a member changes                           | row |
| `object_removed` | an object leaves the set (e.g. mentions read → count 0)       | `{id}` |
| `resync_required`| producer dropped this reader for being slow                   | none |
| heartbeat        | idle, as the chat stream                                       | SSE comment |

Rows reuse the search renderer so a client parses one object shape for search and stream.
`object_removed` means "left the set", not "was deleted"; the doc says so.

### 4.3 No resume cursor

Chat adds are resumable because chat state ids exist. A subscription has no equivalent.
**A reconnect is always a fresh snapshot.** The route ignores `Last-Event-ID` and the doc
says so in one sentence. Clients diff the snapshot against what they hold.

### 4.4 Service shape

Mirror `ChatStream`: `OpenObjectStream(ctx, spaceId, req) (*ObjectStream, error)` with
`Initial`, `Events`, `Close()` (idempotent, `sync.Once`). Slot acquired before subscribing and
released through `Close`, with the same panic-safe handoff pattern as `OpenChatStream`.

- `subscription.Service.Search` with `Internal: true`, a caller-provided **unbounded**
  `mb.New(0)` queue (the engine's contract: a full bounded queue stalls the space worker),
  `NoDepSubscription: true`, `Keys` = the requested fields plus `id`.
- A forwarder goroutine drains the queue, translates
  `ObjectDetailsSet/Amend/Unset` and `SubscriptionAdd/Remove` into the events above, and
  pushes into a **bounded** per-stream channel. If that channel fills, close the stream
  after sending `resync_required` (same policy as the chat producer). The unbounded queue
  absorbs bursts; the bounded channel is the one that can shed a slow client.
- `Close` calls `Unsubscribe(subId)` and logs a failure, as `ChatStream.Close` does.

### 4.5 Stream cap

Reuse `maxConcurrentChatStreams` accounting but with a **separate counter** (`objectStreams`)
so an agent holding many chat streams cannot starve discovery, and the reverse. Suggested
cap: 64 objects streams process-wide, same `429 too_many_streams` envelope. The cap error
text names which kind of stream is full.

### 4.6 Vocabulary change (the fix for check 1)

No relation objects are created for hidden system properties. v2 already falls back to the
bundle in several places (`bundle.GetRelation` for formats in `list_create.go:626`, for
read-only checks in `model.go:719`, for served spellings in `search.go:1220`); only the key
*membership* check lacks it. Add one accept-only allowlist beside `v2SystemQueryKeys`
(`service/search.go:52`):

```go
// v2BundledQueryKeys are hidden bundled relations that queries may name although no relation
// object exists for them in the space. Accepted in filters, fields and sorts, in both
// spellings, but NOT advertised in refusals' "known keys" or list_properties.
var v2BundledQueryKeys = []string{"unreadMentionCount", "unreadMessageCount"}
```

- Membership: `acceptKeys` (search `buildSearchPlan`, set views in `list_read.go`/`list_create.go`)
  appends `withServedSpellings(v2BundledQueryKeys)`. `refKeys` does **not**, so did-you-mean
  and listings stay free of hidden keys (unlike `v2SystemQueryKeys`, which is advertised).
- Format: already resolved from the bundle, so no resolver change.
- Serving: on a row only when listed in `fields`, as a number; absent from default rows (C5).
  To verify in the plan: that the row renderer serves a hidden bundled key from the details
  without a relation entry. If it does not, serve it through the same bundle fallback.
- The list is deliberately explicit, not "any bundled key": a blanket fallback would expose
  every internal relation to filters and sorts. Adding a key is a one-line, reviewed change.
- Documented as **discussion-parent counters, current account, local**: meaningful only for
  objects that have a discussion; absent otherwise; never synced to other members.

### 4.7 Agent recipe the docs will carry

```
GET /v2/spaces/{space_id}/objects/stream?filter=unread_mention_count>0&fields=name,discussion
```

The snapshot is every object with an unread mention; later `object_added` events are newly
mentioned discussions; `object_removed` fires when the agent reads the mention
(`POST …/chats/{discussion_id}/read` with `scope:"mentions"`). Account-wide coverage is one
such stream per space.

## 5. Out of scope

- An account-wide stream over all spaces (engine is per-space; see §7 fork 1).
- Counters for **space chats** in the stream (they are not projected today; §2). A mentions
  inbox that includes space chats still needs a per-chat read, or a follow-up that projects
  chat counters onto the chat object itself.
- Resume cursor (§4.3).
- Any change to which objects the sync loads or in what priority (§3 gaps A, B).

## 6. Test plan

Fixture pattern per `CLAUDE.md`; `want` structs; testify.

1. **Vocabulary**: `filter=unread_mention_count>0` accepted on search and stream for a space
   with no relation object for it; also rejected spelling still errors with a did-you-mean.
   Written first and run against current code to confirm check 1.
2. **Service**: opening snapshot equals the matching rows; an update that crosses 0→1 emits
   `object_added`; 1→0 emits `object_removed`; a field change on a member emits
   `object_updated` with `fields` applied.
3. **Lifecycle**: `Close` twice is safe and unsubscribes once; slot is returned on every
   exit path including a panic in the opening read; the cap returns `too_many_streams` and
   the object and chat counters are independent.
4. **Backpressure**: a stalled reader gets `resync_required` then the stream closes; the
   engine queue never blocks (unbounded queue contract).
5. **Handler**: refusals are JSON before the first byte; SSE framing; heartbeat; grant check
   precedes subscribing; `Last-Event-ID` ignored.
6. **Integration (local API loop, as in the GO-7558 verification)**: two accounts in a shared
   space; account B creates a discussion on a page and mentions A; A's open stream receives
   `object_added` within one sync cycle; A reads the mention and receives `object_removed`.
   This is also where gaps A and B are measured, not assumed.

## 7. Decisions needed

1. **Scope**: per-space stream now (recommended), or account-wide fan-out in the same change.
   Account-wide means N engine subscriptions behind one connection and a space id on every
   event; worth doing only if agents commonly sit in many spaces.
2. **Generic vs narrow**: **decided, generic live search** (also serves "new objects of type X").
3. **Counter names**: **decided, accept-only `v2BundledQueryKeys` allowlist** (§4.6); no relation
   objects are created.
4. **Gap B**: **decided, accept**; no change to sync priority or the parent write.

## 8. Risks

- The mentions signal is **unread mentions of the current account**. Once the agent reads, the
  object leaves the set. That is the intended inbox semantic but means the stream is not a
  history of mentions.
- Latency is bounded by sync priority (gap A). If sub-minute discovery matters, the fix is in
  the sync priority list, not this route.
- Counters are local details, so an object that never loaded its discussion on this device has
  no value. Absent is treated as 0 by the filter, so such objects never match.
