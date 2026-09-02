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

## 2. The two surfaces and when each is needed

```
Event.Account.Recovery.Update { runId, id, timestampMs, oneof payload { ... } }
Rpc.Account.RecoveryState.Request {}  ->  Response { snapshot: Event.Account.Recovery.Snapshot }
```

| Situation | What to do |
|---|---|
| Session already listening when `AccountSelect` is called (the normal flow) | Just apply events. `Started` arrives as `id = 1`. |
| Session attaches while `AccountSelect` is still running | Call `AccountRecoveryState` once. The session-hook snapshot (§2.1) arrives only after `AccountSelect` returns, because the hook runs behind the same lock. |
| Session attaches after the run finished | Call `AccountRecoveryState`; `snapshot.done = true`. |
| An `id` gap, or an unexpected `runId` (§3) | Call `AccountRecoveryState`, replace local state, continue from `snapshot.lastEventId`. |

`AccountRecoveryState` is lock-free with respect to `AccountSelect`: it answers immediately while
that RPC blocks. It returns `ACCOUNT_IS_NOT_RUNNING` only before the first start of the process.

### 2.1 Snapshot on attach

Every new `ListenSessionEvents` session receives one `Update` whose payload is `snapshot` and whose
`id` equals `snapshot.lastEventId`. Treat it exactly like the RPC result. It is an optimization:
never wait for it.

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

- Phases are **monotone except** the `WaitingForNetwork` overlay (which returns to the phase it
  interrupted, `fromPhase = WaitingForNetwork`) and the two terminals.
- **Phases may be skipped.** A warm start goes `Connecting -> LoadingSpaces` without ever entering
  `FetchingAccount`. Accept any forward jump.
- `PhaseChanged.previousPhaseDurationMs` is how long the previous phase lasted, for telemetry.
- `AccountReady` is informational while `AccountSelect` is still blocking: the RPC returning is
  still what lets the UI start. After the RPC returns, treat everything as a background indicator.

---

## 5. Forward compatibility

- **Ignore unknown enum values** in every enum (`Phase`, `ErrorClass`, `SpaceState`, `PeerKind`, …).
  Render an unknown phase as the last known one.
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
- **`viewsConfirmed = false`**: the completeness check could not be completed (one entry never
  arrived; the middleware gave up on it after two consecutive sync rounds). Render a **softer
  "ready"** with **no completeness claim**, and keep relying on your own SpaceView subscription
  for spaces that appear later — the run will not report them.

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
any    -> Removed                                      (deleted while recovering: drop it, it is not counted)
```

- `Loaded` means the space controller finished loading it: mandatory objects fetched, sync
  started. **How many objects are still syncing inside it is on `Event.Space.SyncStatus.Update`,
  not here.**
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
    if state.done:                dismiss; if !state.viewsConfirmed: no "all spaces are here" claim
```

## 11. Debugging

`ANYTYPE_LOG_LEVEL=core.recovery=debug` logs every published update as `id:Kind` (ids and kinds
only, never content). The snapshot RPC is safe to poll while `AccountSelect` blocks.
