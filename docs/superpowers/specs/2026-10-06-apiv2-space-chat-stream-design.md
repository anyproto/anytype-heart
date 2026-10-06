# API v2 space-wide chat stream

Status: revision 3, 2026-10-06 (rev 1 was reviewed by four Codex lenses at xhigh; findings were
checked against the code; rev 3 applies the owner's simplifications: the API is localhost-only and
a stream never disconnects the client on its own). First Heart piece of the Hermes
gateway foundation (`../anytype-hermes/docs/MULTICHAT_RECOMMENDATION.md` §5). Builds on PR #3310
(`list_chats` counters, `include=discussions`). Replaces the chat-stream parts of
`2026-10-06-apiv2-object-stream.md`, which stays the spec for the search stream.

## 1. Goal and decisions

A gateway agent watches one space over **one connection**, discovers chats and object discussions
by itself, and receives every new message individually, without a per-chat SSE connection and
without depending on unread counters.

Decided with the user, 2026-10-06:

- **No mention fields** on messages. The agent detects mentions itself.
- **No server-side replay on the space stream.** No event ids, no `Last-Event-ID` on this route.
  Recovery uses **state-id checkpoints and the existing per-chat stream's `Last-Event-ID`** (§5).
- **A fresh connection opens with a chat snapshot only**, no history. (A chat that appears *after*
  the snapshot is the one exception, §3.4.)
- **Backend: an observer hook on the chat state manager**, not the preview mechanism (§3).
- **Observer events are published immediately**, not staged until commit. A change that rolls
  back can reach the gateway; the gateway tolerates it (§3.1).
- **Opening the stream fails if any chat in the space cannot attach** (§4).
- **A stream never disconnects its client on its own** (localhost API): no slow-reader or overflow
  closes, no write deadlines, no periodic authorization re-check, no `resync_required`/`error`
  events. Rare cases (rollback, permanent init failure, access loss mid-stream) are accepted.

## 2. API

```
GET /v2/spaces/{space_id}/chats/stream?include=discussions|none&heartbeat=30   # SSE
```

Read grant required; the space must be in the key's grant, checked before anything subscribes.
`include` defaults to discussions on this route (`none` = chats only); any other value is a 400
`validation_failed` with issue path `include` and the allowed values. `heartbeat` is the existing
v2 query parameter (1-60 s, default 30, invalid values fall back to the default). `include` is a
per-client predicate: discovery, state and message events all pass through it.

### Envelope

Same discriminated JSON envelope as the per-chat stream: SSE `event: <type>` and
`data: {"type": "<type>", ...}`. No event carries an SSE `id`. Every event carries `space_id`.

```
event: message_added
data: {"type":"message_added","space_id":"S","chat_id":"C","kind":"discussion","parent_id":"P",
       "message":{"id":"m1","state_id":"68f2…","order":"00a1","text":"…", …}}
```

| type | when | fields besides `type`, `space_id` |
|---|---|---|
| `chat_added` | each chat in the opening snapshot; any chat that appears later | `chat` = the `list_chats` row (`id`, `name`, `kind`, `parent_id?`, `unread_messages`, `unread_mentions`) plus `last_state_id` |
| `snapshot_complete` | once, after the last snapshot `chat_added`, also for an empty space | none |
| `chat_updated` | name or parent mapping changes | `chat` as above |
| `chat_removed` | the chat stops being eligible (deleted, archived, hidden, parent lost or archived) | `chat_id` |
| `state_updated` | counters change | `chat_id`, `state` = `{unread_messages, unread_mentions, last_state_id}` |
| `message_added`, `message_updated` | per message | `chat_id`, `kind`, `parent_id?`, `message` (full v2 message DTO **plus `state_id`**) |
| `message_deleted` | per message | `chat_id`, `kind`, `parent_id?`, `message_id` |
| `reactions_updated`, `pinned_updated` | as the per-chat stream | the per-chat payloads plus `chat_id` |

`state_id` is added to the v2 message DTO (REST reads and both streams): it is the value the
per-chat stream already uses as its SSE `id`.

`kind` is `chat` or `discussion`. For a discussion, `id` is its own chat id (usable on every chat
route), `name` is the parent object's name, and `parent_id` the owning object.

## 3. Backend

### 3.1 Observer hook (new, in `core/block/chats/chatsubscription`)

`Manager` gains `AddObserver(id string, fn ChatObserver)` and `RemoveObserver(id string)`. The
observer receives every committed-or-attempted change as it happens, with no window: add, update,
delete, reactions, pinned, chat-state update. It is called inside the existing `Add`, `UpdateFull`,
`UpdateReactions`, `UpdatePinned`, `Delete` and `UpdateChatState` paths, **under the manager
lock**. Contract for observers: enqueue only, never block, never call back into the manager.

- **Owned payloads.** The manager deep-copies the message (`chatmodel.Message.Clone`), reactions
  and state **while locked, before handing off**: the window state keeps mutating the same
  pointers (reactions, pin, read and sync fields), and conversion happens later on another
  goroutine.
- **Delivery gate.** Observer delivery is independent of `canSend()` (which requires a windowed
  subscription or a session context) and does **not** change `IsActive()`, whose meaning is
  object retention (`TryClose`). Counting observers there would stop synced chats from ever being
  evicted.
- **No staging until commit (decided).** Observers fire from the same mutation paths that run
  before the insert (`chathandler.go` `BeforeCreate` updates state and calls `Add` before the
  document is inserted, `storestate/state.go`), and a failed apply rolls back
  (`sourceimpl/store.go`). A rolled-back change can therefore reach the gateway as an event for a
  message that does not exist. The existing manager already tolerates equivalent drift
  (`ReconcileChatState`). The gateway must tolerate it: treat message events as at-least-once
  hints, and before acting on a message it was not already holding, it may confirm the message
  through the REST read. Checkpoints advance from message events only; `state_updated` and
  `chat_added.last_state_id` never advance a checkpoint, because `UpdateChatState` runs before
  `Add`.
- **What is not covered.** Per-message read and mention-read notifications, reaction-read and
  `UpdateSyncStatus` have separate paths and are outside the observer contract (the v2 converter
  already omits them). Aggregate read state reaches `UpdateChatState` and is covered.

Why not the existing preview subscription: it keeps a sliding window of N messages per chat. In
`state.go`, `applyAddMessage` evicts the oldest entry when the window is full and discards its
pending add event, and drops a message whose order id is below the window's oldest. That loses
bursts and late-synced messages. Acceptable for a preview, not for a feed.

### 3.2 Per-space hub

- **Lifecycle.** Created by the first stream for a space, torn down by the last, under one
  per-space mutex, so concurrent open and close are serialized and each hub has a generation.
  Observer and subscription ids include the generation, so an old teardown cannot remove a new
  registration. Teardown: refuse new attaches, remove observers, unsubscribe both searches,
  close the owned queues (unsubscribe does not close them), cancel and join the workers. Lock
  order: a manager's callback takes only the hub's queue lock; the hub never holds its own locks
  while taking a manager lock or waiting on a worker. Initialization is cancelled with the
  stream's context, so a client that leaves during a slow start does not leave a hub behind.
- **Discovery.** Two internal subscriptions through `subscription.Service.Search` with an
  `Internal` unbounded queue and **no limit** (an unsorted limit truncates the snapshot while the
  engine tracks the whole set):
  1. objects with layout `chatDerived` or `discussion`, excluding hidden;
  2. objects with a non-empty `discussionId` (parent id and parent name per discussion; a
     discussion object has no parent or name detail of its own, only the derived `backlinks`).
  Archived and deleted rows are excluded by the engine's implicit filters.
- **Eligibility.** A discussion is eligible only when its parent is eligible (present in
  subscription 2, not archived or deleted). Losing either side emits `chat_removed`, detaches the
  observer, and invalidates events queued for that membership generation. Eligibility is checked
  again at fan-out, so no event for an ineligible chat is delivered.
- **Attach.** For each eligible chat: `GetManager`, `AddObserver`. Counters and `last_state_id`
  are read under the manager lock (`GetChatState` itself does not lock).
- **Fan-out.** One goroutine drains the hub queue, converts once (message DTO and participant-name
  lookups cached per space), applies each client's `include` predicate, and appends to the client's
  own unbounded queue. A slow reader only costs memory; it is never disconnected (localhost).
  The writer goroutine of each client does the socket writes.
- **Ordering.** Per chat, events keep manager order. None across chats.
- **Reach.** A manager sees changes only while its chat object applies them. Messages arriving by
  sync load the object through the tree syncer (`GetTree` → `GetObject`), so they reach the
  observer. A late-synced older message is delivered as `message_added`.

### 3.3 Snapshot and the race

1. Register the client's buffer on the hub (events start buffering).
2. Build the snapshot from the hub's discovery state, with counters and `last_state_id` read under
   each manager's lock.
3. Send `chat_added` for each chat, then `snapshot_complete`, then drain the buffer.

A chat or message may appear in both. The gateway dedupes by account, space, chat and message id
and must still apply `message_updated` and `message_deleted` for a message it already holds:
"message seen" and "input processed" are different states.

### 3.4 A chat that appears after the snapshot

A new discussion can apply its first message before discovery reaches the hub, and `GetManager`
does not replay messages. So for every chat discovered **after** the snapshot, the hub attaches
the observer first, then reads the chat's newest 50 messages from its repository and emits them as
`message_added` right after the `chat_added`. This preserves the first comment under an
agent-created object (research §6.5). Messages applied after attach arrive through the observer;
an overlap is a duplicate the gateway dedupes. Chats in the opening snapshot are not backfilled
(§1).

## 4. Limits and errors

- Read grant and space grant are checked first. Refusals are C6 envelopes before the first byte.
- Separate process-wide cap, `maxConcurrentSpaceChatStreams = 16`, the same `429 too_many_streams`
  envelope, message naming the kind. Independent of the per-chat cap. The cap bounds concurrent
  watches, not retained managers: managers and repositories are not evicted, so visiting many
  spaces over time accumulates them (existing behaviour).
- **Opening fails if any eligible chat cannot attach (decided).** A chat whose manager failed
  initialization fails the open with a C6 `500` naming the chat id. A failed initialization is
  cached by `chatsubscription` until restart, so one bad chat blocks the stream for that space
  until then; this is accepted. A chat that becomes eligible after opening and fails to attach
  is logged and skipped; the stream continues without it.
- **Counters belong to the account Heart runs as.** Managers are initialized with the service's
  account identity, so unread counters are those of that account. Fine for a gateway running as
  the agent's own account; not for a multi-member engine. Multi-member counters are out of scope.
- Opening on a space with many discussions creates a manager per chat on first use: two collection
  opens or creates, four indexes, a message-history migration for old chats, state and count
  queries, an expiring cache and its goroutine. The cost depends on migration status and history
  size and is not a fixed bound (the 0.4-18 ms measured in the PR #3310 review is one data point).
  Heartbeats are sent while the snapshot builds, and the stream's context cancels the work.

## 5. Gateway contract (informative)

1. Keep one checkpoint per chat: the `state_id` of the newest message event it processed.
2. Open the stream and apply each `chat_added` until `snapshot_complete`. Chats it holds that did
   not appear in the snapshot are gone: stop watching them.
3. For each chat whose `last_state_id` is ahead of its checkpoint, catch up with a one-shot
   per-chat stream: `GET …/chats/{chat_id}/messages/stream` with `Last-Event-ID: <checkpoint>`,
   drain the replayed `message_added` events and close. State ids are generated when a message is
   materialized locally and sort in that order, so a late-synced older message has a state id above
   the checkpoint and is replayed. `resync_required` on that per-chat stream (an existing event of that route) means the checkpoint is older
   than the replay window; re-read the chat's history instead. Catch-up connections count against
   the per-chat cap (64), so run them sequentially.
4. Process live events; dedupe on message id; persist the input before advancing the checkpoint.
5. On any disconnect, reconnect and repeat from step 2.

Not recoverable after an outage: an **edit** (a first-mention edit leaves the state id unchanged,
so step 3 does not trigger), a **deletion**, a **reaction** change, and a message that existed and
was deleted during the gap. The per-chat stream has the same limits. A gateway that must not miss a
first-mention edit re-reads the recent messages of every watched chat after a gap.

## 6. Testing

Fixture pattern and `want` structs per `CLAUDE.md`.

- **Observer:** every mutation path reaches the observer; the payload is an owned copy (mutating the
  window afterwards does not change a queued event); delivery works with no windowed subscription
  and a nil session context; `IsActive()` is unchanged by observers and an observed chat is still
  evictable; `RemoveObserver` stops delivery; a stalled observer queue never blocks the manager.
- **Hub:** snapshot then `snapshot_complete` (also for an empty space); a burst of N messages
  yields N events; a late-synced older message is delivered; a chat appearing after open yields
  `chat_added` then its newest messages, including a first message applied **before** the observer
  was attached; eligibility join (archived parent removes the discussion, hidden chat excluded);
  per-client `include`; concurrent open and close, double release, generation ids, teardown joins
  workers and closes queues.
- **Failure:** a manager that fails initialization fails the open with a C6 500 naming the chat;
  a chat that fails to attach after open is skipped and logged.
- **Handler:** SSE framing and envelope, heartbeat query parameter, grant-first refusal, `include`
  validation, 429 cap independent of the per-chat cap.
- **Integration (local API loop):** two accounts in a shared space; a mention from account B in a
  discussion reaches account A's open stream; a gateway restart catches up through the per-chat
  `Last-Event-ID` path, including a late-synced older message.

## 7. Out of scope

- The search stream and the spaces stream.
- Server-side replay, a space-level cursor, a durable journal.
- Mention fields.
- Committed-only observer delivery, retrying failed manager initialization.
- Multi-member (hosted) counters.
- Sync-priority guarantees: discussions are head-synced after ordinary chats and before other
  opened objects; delivery follows successful local application, with no promised bound.

## 8. Risks

- A rolled-back change can reach the gateway as an event (decided, §3.1).
- A failed manager initialization blocks the stream for that space until restart (decided, §4).
- Observers run under the manager lock; a slow observer would stall a chat. Enforced by the
  enqueue-only contract and a test.
- The first stream on a large space pays manager-creation cost up front; managers are never
  evicted.
- Access loss, a slow reader and a stalled converter are not handled by disconnecting (decided);
  a stalled reader costs memory.
