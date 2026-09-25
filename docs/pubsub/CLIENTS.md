# Pub/Sub for client developers

How to use the middleware pub/sub API (`PubsubPublish`, `PubsubSubscribe`,
`PubsubUnsubscribe`, `Event.Pubsub.Message`) correctly: which topics exist,
how often to republish, and how to say goodbye. The cadence rules here follow
the established practice of Matrix (`m.typing`), XMPP chat states (XEP-0085),
Yjs awareness, and the Slack/Discord typing indicators, adapted to our
transport's guarantees.

## 1. What the transport gives you — and what it doesn't

- **Ephemeral, at-most-once, fire-and-forget.** Nothing is stored, nothing is
  retransmitted. A message published while you were not subscribed is gone
  forever; a message may be dropped under load. There is no catch-up after
  reconnect.
- **End-to-end encrypted and authenticated.** Payloads are encrypted with the
  space's current ACL read key (same key as object changes) and signed with
  the sender's account key. The `identity` on a received event is
  **verified** — trust it, never trust identity claims inside payloads.
- **Local echo.** You receive your own published messages back. Filter with a
  per-app-run `sessionId` in the payload (do NOT filter by identity — the same
  account on another device is a legitimate remote peer).
- **Rate limits (server-side, per peer):** 30 msg/s, burst 60, encrypted payload
  ≤64 KiB, ≤100 patterns per space / 1000 per stream. App payloads may contain
  at most **65,508 bytes**, leaving 28 bytes for encryption overhead. Larger
  payloads return `BAD_INPUT` before local echo or network delivery. Stay far
  below all of these limits.
- **Wildcards in subscriptions only:** `*` = exactly one segment, `>` = one or
  more trailing segments (tail only). Publish topics are always concrete.

Design consequence, the golden rule of every convention below:

> **Every message must stand alone, and absence of messages must mean absence
> of the sender.** Send full state (never diffs), refresh it on a timer, and
> let receivers expire entries by their own clock. Explicit "leave" messages
> are a latency optimization, never the mechanism of truth.

This is exactly how the reference systems behave: Matrix typing is
refresh-or-expire with a server timeout; XEP-0085 defines explicit
`composing/paused/gone` states but clients still time out stale ones; Yjs
awareness heartbeats every 15 s and peers expire entries after 30 s.

## 2. Topic registry

Topics are `/`-separated hierarchies inside a space (≤16 segments, ≤256 bytes,
no leading `/`). Anytype apps use these conventions:

| Topic | Payload | Who publishes | Purpose |
|---|---|---|---|
| `<chat_id>/status` | Optional `text` and arbitrary JSON `data` | chat clients and agents | typing and detailed activity |
| `typing/<objectId>` | Typing state (§3) | anyone with the object open | typing indicator in chat, live "typing in block" cursor in the editor |
| `presence/<objectId>` | reserved | — | future: who has the object open (viewer presence) |
| `presence` | reserved | — | future: space-level presence |
| `acc/<app-defined…>/<accountId>` | app-defined | **only** `<accountId>` | spoof-proof per-account channels; the relay and every receiver enforce that the topic's last segment equals the publisher's account id |

Rules for new topics:

- Scope by object, not by finer granularity (`typing/<objectId>`, not
  `typing/<objectId>/<blockId>`): the subscription budget is 100 patterns per
  space, and one open object should cost one subscription. Put the fine detail
  (blockId, cursor position) in the payload.
- Version by evolving the payload, not the topic. Add fields; ignore unknown
  fields on receive; never repurpose existing ones.
- Use a wildcard (`typing/*`) only when you genuinely render all objects at
  once; otherwise subscribe to the concrete topics you display.

### Chat activity: `<chat_id>/status`

API v2 exposes `POST /v2/spaces/{space_id}/chats/{chat_id}/status` for clients
and agents. Its payload is JSON with optional `text` and `data`:

```json
{"text":"Searching documentation","data":{"tool_call":"web_search"}}
```

An empty request or `{"text":""}` publishes `{}`. The receiving client
supplies its localized typing label when text is absent. `data` accepts any
JSON value; it is not restricted to objects. A nonempty `text` is plain display
text, not markup. Sender identity comes from the verified pubsub event.

Subscribe to the exact topic in the same Space. These events have no history
and do not appear in API v2's chat-message SSE stream. A successful status POST
acknowledges acceptance, not delivery. Dry runs do not publish; retries using
the same idempotency key do not refresh the status.

Recommended UI convention: keep the latest update for each verified sender,
refresh every two seconds while active, and expire it ten seconds after its
last receipt. A message from that sender may clear it earlier. Empty text
means default activity, not clear; completion stops the refresh loop. Clients
that distinguish concurrent sessions can agree on a session identifier in
`data`.

The naming follows Hermes' `set_status_text` adapter abstraction: tool starts
set a phrase and tool completion restores the default indicator. Platform
precedents also treat typing as ephemeral: [Matrix](https://spec.matrix.org/latest/client-server-api/#typing-notifications)
supports a timeout and explicit stop, [Discord](https://docs.discord.com/developers/resources/channel#trigger-typing-indicator)
expires typing after ten seconds, and [Slack assistant status](https://docs.slack.dev/reference/methods/assistant.threads.setStatus/)
supports custom text. The refresh/expiry values above are an Anytype client
convention, not server-side timers.

## 3. Typing topic: `typing/<objectId>`

JSON payload (UTF-8):

```json
{ "sessionId": "9f2c1a0e", "blockId": "bafy...", "active": true }
```

- `sessionId` — random id generated once per app run (e.g. 8 hex bytes). Keys
  the entry together with the verified `identity`; also your echo filter. A
  fresh id per run means a crashed session never blocks its successor — the
  stale entry just expires (Yjs regenerates `clientID` per page load for the
  same reason).
- `blockId` — the editor block the sender is typing in / holds focus in;
  empty/absent means the chat input of that object. Receivers may treat a
  block-scoped entry as a soft lock on that block (render a "X is editing"
  chip and make the block read-only locally) — remember the transport is
  at-most-once, so the lock is advisory UX, not mutual exclusion.
- `active` — `true` = typing/holding focus (refresh), `false` = stopped/left
  (clear me).

Receiver state: `objectId → (identity, sessionId) → {blockId, lastSeen}` where
`lastSeen` is **receiver-local receipt time** (immune to sender clock skew —
none of the reference systems trust sender clocks for expiry).

## 4. Republish cadence (how often, when, and why)

| Signal | Rule | Rationale / precedent |
|---|---|---|
| Start (chat input) | publish `active:true` on the first input event, immediately | all platforms; latency is the whole feature |
| Refresh while typing | every **2 s** while input continues; never faster | Slack emits typing every ~3 s; Matrix clients re-PUT before their timeout; more often is pure waste — the indicator can't get "more on" |
| Editor block focus | publish `active:true` with `blockId` **on the focus edge**, heartbeat every **2 s** while focus is held (independent of keystrokes), publish `active:false` **on the blur edge** | the editor signal is focus presence (a soft lock), not keystrokes: a paused writer still holds the block; focus/blur edges are what makes the lock feel snappy |
| Block/context switch | on `blockId` change publish immediately, bypassing the throttle | the payload is full-state; a stale block marker is worse than an extra message |
| Idle stop (chat input) | after **3 s** without input publish `active:false` | XEP-0085 `paused` after a short pause; matches user perception of "stopped typing" |
| Receiver expiry (TTL) | drop an entry after **5 s** without refresh | ≥2× refresh interval so a single lost message (at-most-once!) doesn't flicker the indicator, and short enough that a crashed peer's block lock clears fast |
| Message sent (chat) | publish `active:false` immediately; receivers also clear the sender's entry when the actual chat message arrives | every chat platform clears typing on message delivery; the data beats the signal |
| Heartbeat jitter | add ±10% randomness to refresh timers | avoids synchronized bursts when many clients react to the same event (thundering-herd hygiene, standard gossip practice) |

For future continuous signals (cursor position, selection): coalesce to
≤10 Hz and consider a coarser topic cadence — Yjs awareness's guidance —
and for slow presence ("who's here") use heartbeat 15 s / TTL 30 s.

## 5. Saying goodbye: closing messages

When the user leaves the context — closes the object, switches away, quits the
app — others should not wait out the TTL to see them go. Publish a **closing
message**, then unsubscribe:

1. Publish `{ "sessionId": "...", "active": false }` on the object's topic
   (blockId omitted ⇒ clears everything from this session).
2. Then call `PubsubUnsubscribe` for that object's subId. Order matters:
   publish is fire-and-forget so nothing waits, but unsubscribing first would
   still deliver your own goodbye to nobody locally and looks like a bug in
   traces.

Best-practice framing (XEP-0085 `gone`, Yjs `state:null` on
`window.beforeunload`): the goodbye is **best-effort only**. It will be lost
sometimes — killed process, dropped connection, at-most-once transport. That
is fine *because* receivers expire by TTL anyway; the goodbye only shortens
the ghost period from TTL (8 s) to near zero in the common case.

Send closing messages on:
- object/chat close, navigation away, window/tab close (`beforeunload` —
  best-effort by nature, don't block on it),
- input blur if you sent `active:true` and the user walks away,
- before app quit for every object with an active `true` state.

Never send a goodbye you don't owe: if your last published state was already
`active:false` (or expired long ago), stay silent — goodbye storms on app
quit are noise.

## 6. Subscribe lifecycle

- Subscribe when the surface that renders the signal appears (object opened),
  unsubscribe when it disappears. Use a deterministic subId
  (`typing-<objectId>`) so re-subscribing replaces rather than duplicates —
  the middleware replaces the pattern set of an existing subId.
- **Subscribe before you announce yourself** (publish your own first state
  after subscribing). A stateless channel cannot replay the room to you: you
  are blind to others until their next refresh — at most one refresh interval
  (3 s for typing). For typing indicators that blindness is acceptable; don't
  try to fix it with request/response chatter.
- Middleware subscriptions are app-global, not per-window: multiplex per-window
  interest in the client and unsubscribe only when the last consumer is gone.

## 7. Receiving

On `Event.Pubsub.Message`:

1. Parse payload; ignore messages whose `sessionId` equals yours (echo).
2. Upsert `(identity, sessionId)` with receiver-local `lastSeen`; on
   `active:false` remove the entry.
3. Run an expiry sweep (a 1 s ticker is plenty) dropping entries older than
   the TTL.
4. Resolve display name/avatar from the participant object of the **verified**
   `identity` — never from the payload.
5. Render defensively: unknown identity (participant not yet synced) ⇒ skip;
   unknown blockId ⇒ treat as chat-input typing.

Publish errors (`BAD_INPUT` aside) are non-fatal: drop the signal and let the
next refresh repair state. Never queue or retry ephemeral signals — a late
typing indicator is worse than none.
