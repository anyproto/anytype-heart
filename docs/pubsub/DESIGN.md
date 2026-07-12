# Client Pub/Sub — anytype-heart design

Status: DESIGN + implementation notes. Builds on the any-sync engine
(`any-sync/commonspace/pubsub`, see `any-sync/docs/stateless-pubsub/DESIGN.md`).

## 1. Goal

Expose the any-sync ephemeral pub/sub channel to app clients (anytype-ts et al.)
as a small, abstract middleware protocol:

- **Publish** a payload to a fully-qualified topic in a space.
- **Subscribe** to topic patterns (NATS-style wildcards: `*` = one segment,
  `>` = one-or-more trailing segments) under a client-chosen subscription id.
- **Receive** messages as regular `pb.Event`s on the existing event stream.

Semantics inherited from the engine: ephemeral, fire-and-forget, at-most-once,
no persistence, no replay. Payloads are end-to-end encrypted with the space's
**current ACL read key** (the same key that encrypts object-tree changes) and
signed with the sender's account key; heart verifies + decrypts before emitting
events, so clients only ever see plaintext payloads with a **verified sender
identity**.

## 2. Middleware protocol

### Commands (`pb/protos/commands.proto`, `service.proto`)

```proto
Rpc.Pubsub.Publish.Request   { spaceId, topic, payload bytes }
Rpc.Pubsub.Publish.Response  { error }

Rpc.Pubsub.Subscribe.Request  { spaceId, topics[] /* patterns allowed */, subId }
Rpc.Pubsub.Subscribe.Response { error, subId /* generated when request.subId == "" */ }

Rpc.Pubsub.Unsubscribe.Request  { subId }
Rpc.Pubsub.Unsubscribe.Response { error }
```

Topic constraints (validated by the engine, surfaced as `BAD_INPUT`):
UTF-8, ≤256 bytes, ≤16 `/`-separated non-empty segments, no leading `/`;
publish topics must be concrete (no wildcards); `acc/…/<accountId>` topics are
publishable only by that account.

### Event (`pb/protos/events.proto`)

```proto
Event.Pubsub.Message {
  string topic    = 1;  // concrete topic the message was published to
  bytes  payload  = 2;  // decrypted app payload
  string identity = 3;  // signature-verified sender account id
  repeated string subIds = 4; // local subscriptions whose pattern matched
}
// Event.Message oneof: pubsubMessage, spaceId carried by Event.Message.spaceId
```

Events are `Broadcast` to all sessions. A message matching several distinct
patterns may be emitted once **per matched pattern** (subIds of that pattern);
subscribers sharing one pattern are coalesced into a single event. Publishers
receive their own messages back (local echo) — apps filter by their own
session/identity if they don't want them.

## 3. Heart component (`core/pubsub`)

Two components registered in `core/anytype/bootstrap.go`:

1. `core/pubsub.New()` — CName `client.pubsub`. Implements the engine deps
   (`Crypto`, `MembershipChecker`, `PeerProvider`, `StatusHandler`) plus the
   middleware-facing `Service` (Publish/Subscribe/Unsubscribe) and the
   subscription registry.
2. the any-sync engine `pubsub.New(deps)` wired to (1). Registered after (1);
   both resolve each other at `Init` (registration completes before init, so
   mutual runtime lookup is safe; Go import cycle avoided because only
   `core/pubsub` imports the engine).

### Dep implementations

- **Crypto** — `spacecore.Get(spaceId).Acl()`: under `RLock`,
  `AclState().CurrentReadKeyId()` + `CurrentReadKey()` to encrypt,
  `AclState().Keys()[keyId].ReadKey` to decrypt. This is exactly the read key
  used for change encryption; key rotation on member removal applies
  automatically. Keyless spaces: `ErrNoReadKey` → engine falls back is NOT
  allowed (we return the error; plaintext only if `Crypto` is nil, which we
  never set client-side).
- **MembershipChecker** (gates inbound LAN subscribes/publishes we serve) —
  space must already be in the spacecore cache (`Pick`, no load on behalf of
  LAN peers); `AclState().Permissions(identity).NoPermissions()` → reject.
- **PeerProvider** — mirror of `clientPeerManager.getStreamResponsiblePeers`:
  one responsible node via `pool.GetOneOf(peerStore.ResponsibleNodeIds(spaceId))`
  plus all `peerStore.LocalPeerIds(spaceId)` LAN peers.
- **LAN serving** — `pubsubproto.DRPCRegisterPubSub` on the shared
  `server.DRPCServer` (same mux spacesync registers on), handler delegates to
  `engine.HandleStream`.

### Subscription registry

```
subs:     subId -> {spaceId, patterns}
patterns: spaceId -> pattern -> {subIds set, engine unsubscribe func}
```

- `Subscribe(spaceId, topics, subId)`: adds subId to each pattern entry;
  first subId on a pattern creates the engine subscription whose handler
  emits `Event.Pubsub.Message` with that pattern's current subIds.
- Re-subscribing an existing subId replaces its pattern set (idempotent).
- `Unsubscribe(subId)`: removes subId everywhere; last subId on a pattern
  tears down the engine subscription.
- `CloseSpace(spaceId)` (invoked from spacecore space close/eviction via a
  narrow local interface — no import cycle): drops all registry state for the
  space and calls `engine.CloseSpace`.

Subscriptions are app-global (not session-scoped), matching the object-search
subscription model: clients own subId lifecycles and unsubscribe explicitly.

## 4. Non-goals (v1)

Per-session subscription GC; delivery guarantees beyond at-most-once;
persistence/replay; plaintext publish; exposing keyId/msgId to clients.

## 5. App pattern: typing / live-cursor presence (anytype-ts)

The generic protocol stays app-agnostic; topic conventions, payload schema,
republish cadence, and closing-message rules for client developers live in
[CLIENTS.md](./CLIENTS.md). Summary: topic `typing/<objectId>`, JSON payload
`{sessionId, blockId, active}`, published on focus/input edges and refreshed
every 2s while held, `active:false` on blur/idle (3s) / leave, receiver-side
TTL 5s by local clock, identity always from the verified event, own sessionId
filtered (local echo). Editor blocks with a remote holder render a participant
chip and soft-lock locally.
