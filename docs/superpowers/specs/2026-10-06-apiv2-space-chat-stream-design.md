# API v2 space-wide chat stream

Status: draft for review, 2026-10-06. First Heart piece of the Hermes gateway foundation
(`../anytype-hermes/docs/MULTICHAT_RECOMMENDATION.md` §5). Builds on PR #3310 (`list_chats`
counters, `include=discussions`). Supersedes the "space-wide chat stream" parts of
`2026-10-06-apiv2-object-stream.md`, which stays the spec for the search stream.

## 1. Goal and decisions

A gateway agent watches one space over **one connection**, discovers chats and object
discussions by itself, and receives **every** new message individually, without a per-chat
SSE connection (capped at 64 process-wide) and without depending on unread counters.

Decided with the user, 2026-10-06:

- **No mention fields** on `ChatMessage` (`mention_ids`, `attention_reasons` are dropped). The
  agent detects mentions itself; the unread-mention counter already counts them.
- **No server-side replay in v1.** No event ids, cursors or `Last-Event-ID`. A fresh connection
  gets a snapshot carrying each chat's `last_state_id`; after a reconnect the gateway diffs the
  snapshot against its checkpoints and backfills changed chats through
  `GET …/messages?after=…`. The only stream-level recovery signal is `resync_required` (§5).
- **Fresh connection opens with a chat snapshot only**, no message history.
- **Backend: an observer hook on the chat state manager**, not the preview mechanism (§3).

## 2. API

```
GET /v2/spaces/{space_id}/chats/stream?include=discussions|none   # SSE
```

Read grant required; the space must be in the key's grant, checked before anything subscribes.
`include` defaults to discussions on this route (chats-only is `include=none`); any other value
is a 400 naming the parameter. Heartbeat comment lines as the per-chat stream
(`Anytype-Heartbeat-Seconds` header, same bounds).

### Events

| event | when | payload |
|---|---|---|
| `chat_added` | in the opening snapshot, and when a chat or discussion appears | the `list_chats` row (`id`, `name`, `kind`, `parent_id?`, `unread_messages`, `unread_mentions`) plus `last_state_id` |
| `chat_updated` | name or parent mapping changes | the same row |
| `chat_removed` | the chat leaves the set (deleted, archived, parent lost) | `{id}` |
| `state_updated` | a chat's counters change | `{chat_id, unread_messages, unread_mentions, last_state_id}` |
| `message_added`, `message_updated` | per message | `{chat_id, kind, parent_id?, message}` with the full v2 message DTO |
| `message_deleted` | per message | `{chat_id, kind, parent_id?, message_id}` |
| `reactions_updated`, `pinned_updated` | as the per-chat stream | same payload shapes as the per-chat stream, plus `chat_id` |
| `resync_required` | this client was dropped for reading too slowly | none; the stream then closes |

Event and payload names for message events match the per-chat stream so one decoder serves both.
No event carries an SSE `id`.

`kind` is `chat` or `discussion`. For a discussion, `id` is its own chat id (usable on every chat
route), `name` is the parent object's name, and `parent_id` the owning object.

## 3. Backend

### 3.1 Observer hook (new, in `core/block/chats/chatsubscription`)

`subscriptionManager` gains `AddObserver(id string, fn ChatObserver)` and
`RemoveObserver(id string)` on the `Manager` interface. The observer receives every change as it
happens, with no window: add, update, delete, reactions, pinned, and chat-state updates. Calls
happen inside the existing `Add`/`UpdateFull`/`UpdateReactions`/`UpdatePinned`/`Delete`/
`UpdateChatState` paths, **under the manager lock**, so an observer must only enqueue and never
block or call back into the manager. Existing windowed subscriptions and desktop previews are
untouched.

Why not the existing preview subscription: it keeps a sliding window of N messages per chat. In
`state.go`, `applyAddMessage` evicts the oldest entry when the window is full and discards its
pending add event, and drops a message whose order id is below the window's oldest. A burst
before a flush loses intermediate messages and a late-synced older message never produces an
event. That is acceptable for a preview and not for a message feed.

### 3.2 Per-space hub (new, behind a port in `apicore`)

- **Lifecycle.** Created when the first stream for a space opens, torn down when the last
  closes. Nothing is registered at startup (GO-7302).
- **Discovery.** Two internal subscriptions through `subscription.Service.Search` with an
  `Internal` unbounded queue:
  1. objects whose layout is `chatDerived` or `discussion` (the chat set);
  2. objects with a non-empty `discussionId` (parent id and parent name per discussion).
  A discussion object carries no parent or name detail of its own (verified in a real store:
  only the derived `backlinks`), so the second subscription is required.
- **Attach.** For each chat the hub calls `GetManager` and registers an observer, creating the
  manager on first use. A chat whose manager cannot be created is skipped and logged; the stream
  continues without it. The same applies when a chat appears later.
- **Fan-out.** One goroutine drains the hub queue, converts events (message DTO conversion and
  participant-name resolution once per space, as `ChatStreamMessageOptions` does), and pushes
  each event into every client's bounded buffer (1,024 events).
- **Ordering.** Per chat, events keep manager order. No ordering is promised across chats.
- **Reach of events.** A chat manager only sees changes while the chat object applies them. New
  messages arriving by sync load the object through the tree syncer, so they reach the observer.
  A late-synced older message is delivered as `message_added` (no window filtering).

### 3.3 Snapshot and the race

1. Register the client's buffer on the hub (events start buffering).
2. Build the snapshot: chat set from the hub's discovery state, counters and `last_state_id` from
   each manager's `GetChatState`.
3. Send the snapshot, then drain the buffer.

A chat or message may appear in both the snapshot and the buffer. The gateway dedupes by
account, space, chat and message id.

## 4. Limits and errors

- Read grant and space grant are checked first; refusals are C6 envelopes before the first byte.
- Separate process-wide cap, `maxConcurrentSpaceChatStreams = 16`, same `429 too_many_streams`
  envelope, message naming the kind of stream. The cap is independent of the per-chat stream cap.
- An unreadable chat state skips that chat's counters and logs; it does not fail the stream.
- A slow client: `resync_required`, then close. The gateway reconnects.
- `include=discussions` on a space with many discussions creates one manager and one observer per
  discussion on first stream; measured cost of a first-time manager is 0.4–18 ms (see the review
  of PR #3310), so opening a stream on a space with hundreds of discussions can take seconds
  before the snapshot starts. The stream sends heartbeats during that time.

## 5. Gateway contract (informative)

1. Open the stream; apply each `chat_added`.
2. For each chat whose `last_state_id` is ahead of its checkpoint, backfill with
   `GET …/messages?after=<checkpoint>`.
3. Process live events; dedupe on message id; persist before acknowledging.
4. On `resync_required` or any disconnect, reconnect and repeat from step 1.

Edits, deletions and reactions during an outage are not replayed; a stale local copy persists
until the message is re-read. The per-chat stream has the same limitation.

## 6. Testing

Fixture pattern and `want` structs per `CLAUDE.md`.

- **Manager observer:** every mutation reaches the observer; a late-synced older message is
  delivered; `RemoveObserver` stops delivery; an observer that is slow cannot block the manager
  (enqueue only).
- **Hub:** snapshot then live events; a burst of N messages yields N `message_added` events; a
  chat added after open produces `chat_added` and starts delivering; teardown on last client;
  second client on the same space shares one set of observers.
- **Race:** a message added between observer registration and snapshot is delivered at least once.
- **Handler:** SSE framing, heartbeat, grant-first refusal, `include` validation, 429 cap
  independence from the per-chat cap, `resync_required` then close for a slow reader.
- **Integration (local API loop):** two accounts in a shared space; a discussion on a page; a
  mention from account B reaches account A's open stream without any per-chat connection.

## 7. Out of scope

- The search stream (`2026-10-06-apiv2-object-stream.md`) and the spaces stream.
- Server-side replay, cursors, a durable journal.
- Mention fields on messages.
- Counters for discussions that never loaded on this device beyond what the manager's repository
  holds (spec gaps A and B of the object stream spec stay accepted).

## 8. Risks

- Observers run under the manager lock: a slow or blocking observer would stall a chat. The
  contract is "enqueue only", enforced by a test and by the hub using an unbounded queue.
- The first stream on a large space pays the manager-creation cost up front.
- Managers and repositories are never evicted (existing behaviour); a hub that attaches to every
  discussion pins one per discussion for the process lifetime.
