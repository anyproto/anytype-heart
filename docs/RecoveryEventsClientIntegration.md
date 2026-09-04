# Account Start-up Status (Recovery Events) — Client Integration Guide

Audience: anytype-ts / anytype-swift / anytype-kotlin developers.
Scope: the `Event.Account.Recovery.Update` stream and the `AccountRecoveryState` RPC (GO-7471),
which describe what the middleware is doing between `AccountSelect` being called and every space
having loaded — on **every** app open, not only the first sync on a device.

Engineering reference: [the design spec](superpowers/specs/2026-09-02-cold-sync-recovery-events-design.md).

---

## 1. TL;DR

- **Nothing existing changes.** `AccountSelect` blocks and returns exactly as before;
  `Event.Space.SyncStatus.Update` and `Event.P2PStatus.Update` are untouched. This surface is
  purely additive: render it or ignore it.
- **One event, one RPC.** `Event.Message.accountRecoveryUpdate` (field 206) carries typed updates
  with a `runId` and a monotonic `id`; `AccountRecoveryState` returns the folded `Snapshot` built
  from the same state, so the two never disagree.
- **A minimal client binds one label to `Snapshot.phase` / `PhaseChanged.phase`** and is done.
  A rich client renders peers and spaces from the rest of the feed.
- **`Finished` ends the run.** Nothing is emitted after it. `Finished.viewsConfirmed` says whether
  "all your spaces are here" was actually verified (§6).
- **Per-space object progress is not here.** It stays on `Event.Space.SyncStatus.Update`, which the
  user sees when they open a space.

---

## 2. The two surfaces — there is no ordering to get right

```
Event.Account.Recovery.Update { runId, id, timestampMs, oneof payload { ... } }
Rpc.Account.RecoveryState.Request {}  ->  Response { snapshot: Event.Account.Recovery.Snapshot }
```

**`AccountRecoveryState` is total.** Call it whenever you like — before `AccountSelect`, racing
it, during it, or long after — and you always get a snapshot and never an error:

- **`snapshot.runId == ""`**: no recovery run is in progress and none has left a verdict —
  either none has begun in this process, or the last one ended without a verdict (its
  `AccountSelect` was cancelled by `AccountStop`, or the account was stopped before `Done`).
  `phase` is `NotStarted`. Ignore the rest of the payload and render nothing.
- **otherwise**: this is the state of the current (or last) run. Apply subsequent events with the
  same `runId` and `id > snapshot.lastEventId`. A finished run keeps reporting itself
  (`done = true`) until the next `AccountSelect` starts a new one.

A run that ends without a verdict emits nothing to say so: the client that stopped it learns it
from its own `AccountStop` / `AccountSelect` responses, and a client attaching later sees the idle
snapshot. Reset your state on an empty `runId` and you are covered in both cases.

The RPC is lock-free with respect to `AccountSelect`, so it answers immediately while that RPC
blocks. `ACCOUNT_IS_NOT_RUNNING` exists in the response enum for wire compatibility only; it is
never returned.

**A client that simply subscribes never needs the RPC** in the normal flow: it sees `Started` as
`id = 1` and, on every new `ListenSessionEvents` session, receives a snapshot from the session
hook (§2.1). The RPC is for two cases only: attaching while a run is already in progress (the
hook's snapshot arrives late, see §2.1), and recovering from an `id` gap (§3).

### 2.1 Snapshot on attach

Every new `ListenSessionEvents` session receives one `Update` whose payload is `snapshot` and whose
`id` equals `snapshot.lastEventId` — unless the last run ended without a verdict (see §2), in
which case nothing is sent: there is no run to report. Treat it exactly like the RPC result. It is
an optimization: never wait for it. If the session attaches while `AccountSelect` is still
running, the hook fires only once `AccountSelect` returns (it runs behind the same lock) — call
the RPC instead.

---

## 3. `runId` and `id`

- Ids are contiguous within a `runId`, starting at `1` (`Started`). Apply an update only if
  `id == lastApplied + 1`.
- **On a gap** (`id > lastApplied + 1`): re-pull the snapshot, replace local state, then apply
  updates with `id > snapshot.lastEventId`. Do not try to reconstruct the missing ones.
- **On a new `runId`**: a new app start began (e.g. `AccountStop` + `AccountSelect`). Reset local
  state; the first update of the new run is `Started` with `id = 1`.
- **Every payload is a level, never a delta.** `attempt`, `openConnections`, a space's `state` are
  absolute values. Re-applying an update after a re-pull is harmless.
- Several updates may arrive in one `Event` (coalesced bursts). Apply them in order.

---

## 4. Phases — what the headline binds to

```
LookingForPeers -> Connecting -> FetchingAccount -> LoadingSpaces -> Done
                                                                  \-> Failed
WaitingForNetwork (overlay: calm, auto-retrying — NOT an error screen)
```

| Phase | Render as |
|---|---|
| `LookingForPeers` | "Looking for peers…" |
| `Connecting` | "Connecting…" (dials are happening; `DialFailed` underneath is normal) |
| `FetchingAccount` | "Fetching your account…" — a network node is connected and the account is being pulled |
| `LoadingSpaces` | "Loading spaces…" — the account is here; `AccountReady` was emitted |
| `WaitingForNetwork` | "Waiting for network…" with `PhaseChanged.error.class` as a hint (`NoNetwork`, `PeerUnreachable`, `IncompatibleVersion`). Retrying automatically; do not show a failure. |
| `Done` | Dismiss. See §6 for what may be claimed. |
| `Failed` | `AccountSelect` itself failed; `PhaseChanged.error` carries the class (`AccountDeleted`, `AccountNotFound`, `IncompatibleVersion`, …). The RPC error remains the authoritative signal. |

Rules:

- **Phases are not monotone. Always render the phase you were last told.** The phase is derived
  from the run's current condition, so it moves back when that condition regresses: losing the
  last node connection during the account fetch goes `FetchingAccount -> Connecting`, and the
  `WaitingForNetwork` overlay returns to the phase it interrupted (`fromPhase =
  WaitingForNetwork`). A client that drops "backward" transitions will stick on a phase the run
  has already left. Only `Done` and `Failed` are terminal.
- **Phases may be skipped.** A warm start goes `Connecting -> LoadingSpaces` without ever entering
  `FetchingAccount`. Accept any forward jump.
- `PhaseChanged.previousPhaseDurationMs` is how long the previous phase lasted, for telemetry.
- `AccountReady` is informational while `AccountSelect` is still blocking: the RPC returning is
  still what lets the UI start. After the RPC returns, treat everything as a background indicator.

---

## 5. Forward compatibility

- **Ignore unknown enum values** in every enum (`Phase`, `ErrorClass`, `SpaceState`, `PeerKind`, …).
  Render an unknown phase as the last known one. `Phase.NotStarted` appears only in the idle
  snapshot (`runId == ""`), never on the event stream.
- **Ignore unknown payload kinds** in `Update.payload`. New kinds are not a breaking change.
- `Mode.ModeUnknown` is never emitted; treat it as "unknown", never as a cold recovery.
- `transport` and `nodeTypes` are strings on purpose (`quic`, `yamux`, …; `tree`, `coordinator`,
  `file`, …). Do not enumerate them.

---

## 6. `Finished` and `viewsConfirmed`

`Finished { spacesTotal, spacesLoaded, spacesFailed, totalDurationMs, viewsConfirmed }` is the
run's verdict. The run is terminal after it: no more updates, and the snapshot is frozen.

- **`viewsConfirmed = true`**: the middleware checked the list of spaces against the network and
  every one of them is present locally. You may say **"all your spaces are here — objects inside
  them may still be syncing"**.
- **`viewsConfirmed = false`**: the completeness check could not be completed — one entry never
  arrived and the middleware gave up on it after two consecutive sync rounds, or no responsible
  sync round ever happened at all (an app opened with no connectivity loads every space from disk
  and never gets to ask the network). Render a **softer "ready"** with **no completeness claim**,
  and keep relying on your own SpaceView subscription for spaces that appear later — the run will
  not report them.

In both cases a `SpaceDiscovered` after `Finished` never happens; new spaces reach you through the
SpaceView subscription as they always did.

---

## 7. Spaces

`SpaceDiscovered { spaceId, spaceViewId, kind }` then `SpaceStateChanged { spaceId, state, fromState,
error, attempt }`.

```
Queued -> Loading -> Pulling -> Loading -> Loaded      (a space fetched from the network)
Queued -> Loaded                                       (a space already on disk from last time)
any    -> Error                                        (the loader gave up; error.class says why)
any    -> Stalled                                      (settled around it; no load result ever arrived)
any    -> Removed                                      (deleted while recovering: drop it, it is not counted)
```

- `Loaded` means the space controller finished loading it: mandatory objects fetched, sync
  started. **How many objects are still syncing inside it is on `Event.Space.SyncStatus.Update`,
  not here.**
- `Stalled` means the rest of the run settled around this space and it still never reported.
  It is **not terminal**: the load may yet complete, and you will get another `SpaceStateChanged`
  if it does. Render it as a **determinate stall with a retry** — "79 of 80 loaded, 1 stalled" —
  never as ongoing progress. `Finished` does not fire while any space is stalled, because the
  account is demonstrably not recovered.
- `kind = Tech` is the account's tech space; filter it out of user-facing lists.
- **Names are not on this stream** (no user content). Resolve them from your SpaceView
  subscription via `spaceViewId` — the SpaceView exists by the time `SpaceDiscovered` fires.
- `spaceViewId` may be empty on a first `SpaceDiscovered` and filled by a second one for the same
  `spaceId`; both are idempotent.
- `attempt` counts fetch attempts for a `Pulling` space; it is a level.

---

## 8. Peers

`DialStarted`, `PeerConnected`, `DialFailed`, `PeerDisconnected`, `PeerDiscovered` — each carries
`peerId`, `kind` (`NetworkNode` | `LocalPeer`) and `nodeTypes`.

- `PeerConnected.openConnections` / `PeerDisconnected.openConnections` are **counts**, not a
  boolean: a peer may briefly have two connections while one replaces the other.
- **`durationMs` is `0` on inbound connections (`direction = Inbound`). Suppress it; never render
  "0 ms".** `addr` is display-only and not comparable across directions.
- `DialFailed.attempt` is the number of dials since the last successful connection to that peer;
  repeated failures for one peer arrive coalesced as a single updated level.
- `LocalDiscoveryState` (`Possible` | `NoInterfaces` | `Restricted`) **may never arrive on a
  healthy network**: it is only emitted on a change, and the snapshot's default is `Possible`.
  Never wait for it.
- `WaitingForNetwork` is derived server-side (no open connection, a dial failed, ~10 s elapsed —
  or the device reported offline). Bind to the phase; do not derive your own from peer events.

### 8.1 LAN peers and the account

Connecting to a LAN peer says nothing about whether it holds *your* account: that is answered by
the space exchange right after the connection. `PeerSpaceExchange { peerId, exchanged,
hasAccountSpace, sharedSpaceCount }` reports that answer as a fact, and the middleware folds every
LAN peer's dial state and answer into one headline, `LocalPeersStateChanged { state, fromState }`
(also `Snapshot.localPeers`):

| `LocalPeersState` | Meaning |
|---|---|
| `NoLocalPeers` | nothing discovered on the LAN |
| `LocalPeersConnecting` | found a peer; dialing, or connected and waiting for its answer |
| `LocalPeersUnreachable` | every discovered peer failed to connect |
| `AccountNotOnLocalPeers` | **every** connected peer has answered and none holds the account — "looking for others" |
| `AccountOnLocalPeer` | at least one peer holds your account's tech space — "connected to a device with your account" |

The negative state never appears while a connected peer has not answered yet, and one positive
answer flips it immediately, whatever the other peers said. Bind copy to this enum, not to the
raw `PeerSpaceExchange` events.

**Caveat until GO-7492 ships:** on a cold device the exchange is sent with zero tokens (the local
store has no spaces yet), so every LAN peer answers "nothing shared" even when it holds the
account. `AccountNotOnLocalPeers` is therefore reliably reached on a fresh device today and does
**not** mean the peer lacks your data — do not surface "your account data was not found on this
device" copy yet; render `AccountNotOnLocalPeers` as neutral ("looking for peers…"). Once
GO-7492 lands, the re-exchange produces a second `PeerSpaceExchange` for the same peer
(`hasAccountSpace = true`) and the state flips to `AccountOnLocalPeer` with no client change.

---

## 9. Errors

`ErrorInfo { class, retryable, debugMessage }` appears on `PhaseChanged` (`WaitingForNetwork`,
`Failed`), `DialFailed`, `AccountFetchError`, `SpaceStateChanged` and in the snapshot.

- Headline **`class`** (`NoNetwork`, `PeerUnreachable`, `IncompatibleVersion`, `NotAuthorized`,
  `SpaceDeleted`, `AccountDeleted`, `AccountNotFound`, `RateLimited`, `Unexpected`) and honour
  `retryable`: a retryable class is a calm state, not a failure.
- **`debugMessage` is raw error text for logs and Sentry only. Never display it.**
- `AccountFetchStarted.attempt` counts pull *rounds*; a new round after `AccountFetchError` (or
  after a silent bounded wait) means "still trying", not "gave up" — the middleware never gives up
  on the account fetch by itself.

---

## 10. Minimal client, in pseudocode

```
onSessionOpen:
    snap = AccountRecoveryState()            // or wait for the snapshot Update
    state = fold(snap); last = snap.lastEventId; run = snap.runId

onEvent(u: Update):
    if u.runId != run:            state = {}; run = u.runId; last = 0
    if u.id != last + 1:          snap = AccountRecoveryState(); state = fold(snap); last = snap.lastEventId; return
    apply(state, u.payload)       // unknown payload kinds: ignore
    last = u.id

render:
    headline = label(state.phase) // unknown phase: keep the previous label
    if state.phase == Failed:     show error(state.error); stop   // done is also true here
    if state.done:                dismiss; if !state.viewsConfirmed: no "all spaces are here" claim
    stalled = count(state.spaces, s -> s.state == Stalled)
    if stalled > 0:               show "N of M loaded, K stalled" + retry, not a spinner
```

`done` is true for **both** terminals, so test `Failed` first — dismissing on it would hide the
one message the user needs (`AccountDeleted`, `IncompatibleVersion`, …).

## 11. Debugging

`ANYTYPE_LOG_LEVEL=core.recovery=debug` logs every published update as `id:Kind` (ids and kinds
only, never content). The snapshot RPC is safe to poll while `AccountSelect` blocks.
