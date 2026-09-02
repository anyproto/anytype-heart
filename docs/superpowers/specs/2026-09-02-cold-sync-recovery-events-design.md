# Cold-sync recovery events (GO-7471)

Date: 2026-09-02
Status: Draft for review — revision 2 (per-space settle rule overruled, see below)
Branch: `go-7471-cold-sync-events` (based on `go-7467-quic-degradation-fallback`)
Depends on: any-sync v0.13.2 (`net/peerobserver`, `commonspace.PullObserver`) — already pinned in `go.mod`

## Problem

Signing into an existing account on a fresh device ("cold recovery") shows three animated dots
while `AccountSelect` blocks — potentially unbounded. The dominant blocker is
`space.(*service).initAccount` -> `loadTechSpace` (`space/service.go:295-327`): a 15 s deadline,
then, when no personal space exists locally, a retry with the *parent* context that never gives up.
Inside it `spaceCore.Get(techSpaceId)` misses ocache, `commonspace.NewSpace` fails
`WaitSpaceStorage` with `ErrSpaceStorageMissing`, and `getSpaceStorageFromRemote` waits for a
responsible peer and pulls. Then `techSpace.Run` -> `accountObjectExists` (15 s), then
`app.Start` finishes, then `GetInfo` may poll `getAnalyticsId` for up to 30 s. None of this emits
anything a client could render.

The legacy surfaces do not help here: `EventSpaceSyncStatusUpdate` starts 3 s after `Run` and
defaults to `Synced` (`nodestatus.ConnectionStatus` zero value is `Online`, `makeSyncEvent`
defaults to `Synced`); `EventP2PStatusUpdate` covers LAN only; both are per-space and neither
exists before the tech space is up.

This spec adds a recovery-scoped, granular, monotonic event stream plus a snapshot RPC, served
from one fold, so a client can render "looking for peers -> connecting -> fetching account ->
loading spaces -> done" (with a calm "waiting for network" resting state) from the first
millisecond of `AccountSelect` until every space has published its load result.

## Override of approved decision 1 (revision 2)

Approved decision 1 defined a finalized space as "loaded AND its first sync settled". The user
overruled it on review:

> we already have sync status per-space that shows how many objects are syncing. So when user
> switch this space they see details. So maybe on the cross-space level we should just show
> space initial progress until space state changes in spacecontroller? as far as i remember it
> includes fetching mandatory objects

**New rule: a space is finalized for recovery purposes when the space controller publishes its
load result** — `spaceloader.onLoad` setting `LocalStatusOk` (success) or `LocalStatusMissing`
(terminal failure), `space/internal/components/spaceloader/spaceloader.go:129-155`. That result
genuinely includes the mandatory objects: `clientspace` starts `go sp.mandatoryObjectsLoad(...)`
(`space/clientspace/space.go:191`), the loader awaits it through `WaitMandatoryObjects` (`:315`),
and the routine ends with `s.common.TreeSyncer().StartSync()` (`:248`). So `Loaded` means
"mandatory objects fetched, tree sync started". Object-level progress after that is already on
`EventSpaceSyncStatusUpdate` and is seen when the user enters the space; the cross-space
recovery surface must not duplicate it.

Consequences, applied throughout this revision: no per-space head-sync hook, no
`syncsubscriptions` poll loop, no quiet window, no `objectsToSync` on the wire; the state
`Synced` is renamed `Loaded` and the counter `spacesSynced` is `spacesLoaded`; the one head-sync
signal that survives is for the **tech space only**, as the SpaceView-completeness gate (R11).

## Decisions for review

Each item states the recommendation the rest of the spec is written against. Confirm or overrule;
everything else follows the approved decisions verbatim.

| # | Question | Recommendation |
|---|---|---|
| R1 | **Lazy loading and `Finished`** | One `Finished`, gating on **every** discovered space, deferred ones included. Deferred spaces are reported as `Queued` (no separate `Deferred` state). Rationale: the deferred backlog auto-releases after `preloadRemainingSpacesTimeout` (10 s, `space/service.go:80`) or on `AccountPreloadRemainingSpaces`, so deferral delays `Finished` by at most ~10 s + load, and per-space finalization is the loader's own result. `AccountReady` is the usable-before-done milestone; no third milestone. |
| R2 | **Partial failure** | `Finished` is always emitted, with `spacesTotal/Loaded/Failed`; global phase goes to `Done`. A per-space failure is a per-space `Error` state with its own `ErrorClass`. A new terminal phase `Failed` is added and reserved for **account-level** fatals only (`AccountSelect` itself fails: account deleted, account not found, incompatible version, unrecoverable start error). `Snapshot.error` carries the account-level class only. |
| R3 | **Analytics wait** | No dedicated phase. The stream's `FetchingAccount` ends at `AccountReady` (`close(techSpaceReady)`); the up-to-30 s `getAnalyticsId` window sits inside `LoadingSpaces`, which is what is actually happening. The root cause (1 s polling loop, `core/anytype/account/service.go:211-243`) is filed as a follow-up outside this spec: derive the metrics id immediately when the account object is head-synced and still has none. |
| R4 | **Space names in events** | **Omit** the name. `SpaceDiscovered` carries `spaceId` + `spaceViewId`; clients resolve names from their own SpaceView subscription (the SpaceView is what `SpaceDiscovered` is keyed off, so it exists by then). No user content on the stream, no objectstore lookup near the fold mutex, no `DisplayText` machinery. If overruled: adopt importv2's `DisplayText` (hash in logs, plaintext only in the pb builder). |
| R5 | **Coalescing policy** | 250 ms window, keyed per (kind, peerId/spaceId), last-wins, immediate publication on every phase/state **edge**; only repeats of the same state (DialFailed for a peer already failing, `SpaceStateChanged` attempt ticks, `PeerDisconnected`) coalesce. A flushed window goes out as **one** `pb.Event` carrying several ordered messages. Details in "Coalescing". |
| R6 | **Component ownership (deviation from decision 6)** | The tracker is **created once by `core/application.Service`** and passed into `Bootstrap` via the same `comps` path as the event sender, which registers it first (before `space.New()` by construction). Reason: `AccountSelect` holds `Service.lock` for the whole start and `GetApp()` takes `RLock` (`core/application/application.go:57`), so a pull RPC routed through the app would **block until `AccountSelect` returns** — the exact window it exists for. An application-owned handle is lock-free. The `peerobserver.CName` slot is still filled from `bootstrap.go` by the fan-out mux (review correction 7). |
| R7 | **`runId` on the wire** | Add `runId` (random per `Begin`) to every `Update` and to `Snapshot`. Ids restart at 1 per run; without a run id a client that pulled a snapshot before an `AccountStop`/`AccountSelect` cycle would misread the next run's ids as a gap. Cheap, and it is what makes the shape viable as an always-on surface later. |
| R8 | **`AccountNotFound` error class** | Add it to the approved taxonomy. `spacesyncproto.ErrSpaceMissing` on the tech space with no local personal space ends `AccountSelect` with `FAILED_TO_FIND_ACCOUNT_INFO` — the second most common cold-recovery failure after no-network ("wrong network or mnemonic"), and `Unexpected` would be a lie. |
| R9 | **Tech space in `spaces[]`** | Include it, with `kind: Tech`. It is `Loaded` at `AccountReady`; it never goes to `Error` (a tech-space failure is the run's `Failed`). Its head-sync diff from a responsible node is the SpaceView-completeness gate on `Finished` (R11). Clients that only want user spaces filter on `kind`. |
| R10 | **Local-only network mode** | The load-based rule needs no special case: `Loaded`/`Error` come from the loader in every mode. The only adjustment is the tech-space gate (R11): with no responsible nodes it accepts any peer's diff, since LAN peers are the only source. |
| R11 | **SpaceView-completeness gate (new; resolves what the override breaks)** | Keep **one** head-sync hook — `treesyncer.SyncAll` reporting `(spaceId, peerId, missing, responsible)` — and consume it **only for the tech space**. Gate: the latest responsible diff's `missing` set is fully resolved, where a missing id resolves when a SpaceView with that object id arrives on the SpaceView subscription (SpaceView object id == tree id) or when a later diff no longer lists it. Bounded: two consecutive responsible diffs that resolve nothing (~40 s) open the gate with `viewsConfirmed=false`, so one permanently stuck id cannot hold `Finished` forever. Chosen over the alternatives on inspection — see "Tech-space completeness gate" for what was rejected and why. |

## Goals

- A client renders something truthful from the first millisecond of `AccountSelect`, including
  the blocking window, without changing the `AccountSelect` contract.
- One coarse phase a minimal client binds to; a per-peer/per-space event log a rich client renders.
- Push and pull served from the **same** fold state, so they cannot drift.
- Monotonic ids; a gap means "re-pull the snapshot"; the log is an optimization, the snapshot is
  the truth.
- Errors are classified, tiered, and never headline raw transport strings.
- Zero effect on control flow: every producer seam is advisory, panic-contained, non-blocking.
- Emitted on every start (trivially short on warm starts) so clients have one code path.

### Scope: every app open

This stream covers **every app open**, not only the first sync on a device. A warm open with
nothing new collapses to a handful of events; an open after time away discovers the spaces that
appeared meanwhile (shared with the user, or created on another device) and runs them through
the same `Queued -> Pulling -> Loading -> Loaded` path, with `Finished` waiting for them. Same
code path, no special-casing — that is what lets a client bind one renderer to it.

## Non-goals

- Changing `AccountSelect`, `EventSpaceSyncStatusUpdate`, `EventP2PStatusUpdate`, SpaceView
  statuses, or per-object sync status. Purely additive.
- Fixing the 15 s -> unbounded `loadTechSpace` retry, the `accountObjectExists` create-if-missing
  fallback, or the `getAnalyticsId` poll. Observed and reported, not altered.
- Object-level sync progress per space. It already exists on `EventSpaceSyncStatusUpdate`
  (`syncingObjectsCounter`) and is shown when the user enters a space; this surface stops at the
  load result (override above). File sync likewise stays on the legacy surface.
- An always-on connectivity surface after `Finished`. The shape allows it later; no emission
  after terminal in this version.
- Percentages or ETAs. Remaining work is not knowable up front; we show activity and counts,
  never a fake bar.

## Wire surface

### `pb/protos/events.proto` — new `Event.Account.Recovery`

Nested under the existing `Event.Account` (line 207). Field numbers are concrete: all messages are
new, and `206` is the next free slot in the `Event.Message` oneof's `Account` block (201–205 in
use, verified).

```proto
message Account {
  // ... existing Show/Details/Config/Update/LinkChallenge ...
  message Recovery {
    enum Mode  { ColdRecovery = 0; WarmStart = 1; NewAccount = 2; }
    // Monotone except WaitingForNetwork (an overlay, see PhaseChanged) and the two terminals.
    // Phases may be skipped (a warm start never enters FetchingAccount); clients must accept
    // any forward jump.
    enum Phase { LookingForPeers = 0; Connecting = 1; FetchingAccount = 2; LoadingSpaces = 3;
                 Done = 4; WaitingForNetwork = 5; Failed = 6; }
    enum ErrorClass { None = 0; NoNetwork = 1; PeerUnreachable = 2; IncompatibleVersion = 3;
                      NotAuthorized = 4; SpaceDeleted = 5; AccountDeleted = 6;
                      AccountNotFound = 7; RateLimited = 8; StorageLimit = 9; Unexpected = 10; }
    enum PeerKind       { LocalPeer = 0; NetworkNode = 1; }
    enum Direction      { Outbound = 0; Inbound = 1; }
    enum SpaceKind      { Regular = 0; Tech = 1; }
    // Loaded = the space controller published LocalStatusOk: mandatory objects fetched, tree
    // sync started. Object-level progress after that is on EventSpaceSyncStatusUpdate.
    enum SpaceState     { Queued = 0; Pulling = 1; Loading = 2; Loaded = 3; Error = 4; }
    enum DiscoveryState { Possible = 0; NoInterfaces = 1; Restricted = 2; }

    // ErrorInfo, not Error: enum values and nested messages share the enclosing scope in
    // protobuf, and SpaceState.Error already takes that name (found by protoc in phase 1)
    message ErrorInfo {
      ErrorClass class = 1;
      bool retryable = 2;
      // raw error text for logs/diagnostics; clients may log it, must never headline it
      string debugMessage = 3;
    }

    message Update {
      string runId = 1;      // random per run (R7); a change means "new run, reset"
      int64 id = 2;          // monotonic within runId, starts at 1; gap => AccountRecoveryState
      int64 timestampMs = 3; // unix ms
      oneof payload {
        Started             started             = 10;
        PhaseChanged        phaseChanged        = 11;
        LocalDiscoveryState localDiscoveryState = 12;
        PeerDiscovered      peerDiscovered      = 13;
        DialStarted         dialStarted         = 14;
        PeerConnected       peerConnected       = 15;
        DialFailed          dialFailed          = 16;
        PeerDisconnected    peerDisconnected    = 17;
        AccountFetchStarted accountFetchStarted = 18;
        AccountFetchError   accountFetchError   = 19;
        AccountReady        accountReady        = 20;
        SpaceDiscovered     spaceDiscovered     = 21;
        SpaceStateChanged   spaceStateChanged   = 22;
        Finished            finished            = 23;
        // sent only to a newly attached session (session hook): "snapshot-on-subscribe"
        Snapshot            snapshot            = 24;
      }
    }

    message Started      { Mode mode = 1; string networkId = 2; }
    message PhaseChanged { Phase phase = 1; Phase fromPhase = 2; int64 previousPhaseDurationMs = 3;
                           ErrorInfo error = 4; /* set for WaitingForNetwork and Failed */ }
    message LocalDiscoveryState { DiscoveryState state = 1; }
    message PeerDiscovered { string peerId = 1; repeated string addrs = 2; }
    message DialStarted    { string peerId = 1; PeerKind kind = 2; repeated string nodeTypes = 3;
                             int32 addrsCount = 4; }
    message PeerConnected  { string peerId = 1; PeerKind kind = 2; repeated string nodeTypes = 3;
                             Direction direction = 4; string addr = 5; string transport = 6;
                             uint32 protoVersion = 7; int64 durationMs = 8; /* 0 = unknown (inbound) */
                             int32 openConnections = 9; }
    message DialFailed     { string peerId = 1; PeerKind kind = 2; repeated string nodeTypes = 3;
                             ErrorInfo error = 4; int32 attempt = 5; /* dials observed since last Connected */
                             int64 durationMs = 6; }
    message PeerDisconnected { string peerId = 1; PeerKind kind = 2; int32 openConnections = 3; }
    message AccountFetchStarted { string spaceId = 1; string peerId = 2; /* empty = waiting for a peer */ }
    message AccountFetchError   { string peerId = 1; ErrorInfo error = 2; int32 attempt = 3; }
    message AccountReady        { int64 durationMs = 1; /* since Started */ }
    message SpaceDiscovered     { string spaceId = 1; string spaceViewId = 2; SpaceKind kind = 3; }
    message SpaceStateChanged   { string spaceId = 1; SpaceState state = 2; SpaceState fromState = 3;
                                  ErrorInfo error = 4; int32 attempt = 5; /* pull attempts, a level */ }
    message Finished { int32 spacesTotal = 1; int32 spacesLoaded = 2; int32 spacesFailed = 3;
                       int64 totalDurationMs = 4;
                       // true: the SpaceView set was confirmed against the network ("no more
                       // spaces to download"); false: the completeness gate opened on its stall
                       // bound, no completeness claim may be made
                       bool viewsConfirmed = 5; }

    // Folded state. Served by Rpc.Account.RecoveryState and pushed once per new session.
    message Snapshot {
      string runId = 1; int64 lastEventId = 2; Mode mode = 3; string networkId = 4;
      int64 startedAtMs = 5;
      Phase phase = 6; int64 phaseStartedAtMs = 7; bool done = 8; ErrorInfo error = 9;
      DiscoveryState discovery = 10;
      bool accountFetchStarted = 11; bool accountReady = 12;
      repeated Peer peers = 13; repeated Space spaces = 14;
      int32 spacesTotal = 15; int32 spacesLoaded = 16; int32 spacesFailed = 17;
      bool viewsConfirmed = 18; // meaningful once done; see Finished
      message Peer  { string peerId = 1; PeerKind kind = 2; repeated string nodeTypes = 3;
                      int32 openConnections = 4; string transport = 5; uint32 protoVersion = 6;
                      int32 dialAttempts = 7; ErrorInfo lastError = 8; bool discoveredLocally = 9; }
      message Space { string spaceId = 1; string spaceViewId = 2; SpaceKind kind = 3;
                      SpaceState state = 4; ErrorInfo error = 5; int32 attempt = 6; }
    }
  }
}
```

`Event.Message` oneof: `Account.Recovery.Update accountRecoveryUpdate = 206;`.

`transport` is the scheme string as any-sync reports it (`quic`, `yamux`, `iroh`, ...), not an
enum: a new transport must not require a proto change. `nodeTypes` are `nodeconf.NodeType`
strings (`coordinator`, `tree`, `file`, ...) for the same reason.

### `pb/protos/commands.proto` — new `Rpc.Account.RecoveryState`

Next to `Rpc.Account.PreloadRemainingSpaces` (line 968). `commands.proto` already imports
`events.proto`, so the snapshot type is shared, as `Rpc.Object.ImportRunStatus.Run.status`
shares `Event.Import.Statistic` on the importv2 branch.

```proto
message RecoveryState {
  message Request {}
  message Response {
    Error error = 1;
    anytype.Event.Account.Recovery.Snapshot snapshot = 2;
    message Error {
      Code code = 1; string description = 2;
      enum Code { NULL = 0; UNKNOWN_ERROR = 1; BAD_INPUT = 2; ACCOUNT_IS_NOT_RUNNING = 101; }
    }
  }
}
```

`pb/protos/service/service.proto`: `rpc AccountRecoveryState (anytype.Rpc.Account.RecoveryState.Request) returns (anytype.Rpc.Account.RecoveryState.Response);` in the Account block (line ~49).

### Codegen and files touched

`make protos` (`makefiles/protos.mk`: `protos-go` + `protos-server` + `protos-docs`; needs the
anyproto `protoc`, `make setup-protoc`). Regenerates `pb/*.pb.go`, `pb/service/*.pb.go`,
`clientlibrary/service/*.pb.go`, `docs/proto.md`. Handler: `core/account.go`
`AccountRecoveryState` following the `AccountPreloadRemainingSpaces` shape (`mapErrorCode` +
`errToCode(application.ErrApplicationIsNotRunning, ..._ACCOUNT_IS_NOT_RUNNING)`).

Client contract, stated once: on attach call `AccountRecoveryState` (or wait for the session-hook
`Snapshot`), then apply every `Update` with the same `runId` and `id == lastApplied+1`; on a gap
or a new `runId`, re-pull. Every payload is a **level**, never a delta (`attempt`,
`openConnections` are absolute), so re-applying after a re-pull is idempotent.

`Finished` is the app telling the user "we checked, and there are no more spaces to download —
only objects inside them may still be syncing, which you see when you open one". The
tech-space completeness gate (R11) is what licenses that claim, so `viewsConfirmed` says
whether it was earned: `true` supports "all spaces are here"; `false` (the gate opened on its
stall bound) must render as a softer "ready" with **no** completeness claim. In both cases the
run is terminal and reports no later `SpaceDiscovered`; with `viewsConfirmed=false` the client
must keep relying on its own SpaceView subscription for spaces that appear afterwards.

## Component design: `core/recovery`

Package `core/recovery`, type `Tracker`, CName `core.recovery`. One mutex, one builder, modelled
on `core/block/importv2/adapter/statistic.go` (branch `go-7349-import-llm`).

### Ownership and lifetime (R6)

- `application.New()` creates `s.recovery = recovery.New()` once for the process. State is built
  in `New()` (correction 9): every map, the timer slot, the clock; nothing waits for `Init`.
- Each `anytype.StartNewApp` site (`account_select.go:166`, `account_create.go:82`,
  `account_create_from_export.go:143`, and `account_stop.go:119` via `start`) does
  `s.recovery.Begin(recovery.Run{Mode, Sender: s.eventSender})` immediately before the call and
  appends `s.recovery` to `comps`. Mode: `ColdRecovery` when `repoWasMissing`, `WarmStart`
  otherwise, `NewAccount` for the create paths. A small `s.startApp(ctx, mode, comps...)` helper
  keeps the three sites identical. On `StartNewApp` error the same helper calls
  `s.recovery.Fail(err)`.
- `Init(a)` binds: `event.Sender` (re-bound; `Begin` already had it), `nodeconf.Service`,
  `net/peerobservermux.Mux` (registers itself as an observer), `localdiscovery` possibility hook
  via the `peerstatus.LocalDiscoveryHook`-shaped interface (`RegisterDiscoveryPossibilityHook`),
  `session.HookRunner` (registers the snapshot-on-attach hook), and, optionally by name
  (`"networkState"`, as `space/spacecore/peermanager/provider.go` does), the device network state
  for `IsOffline()` + `RegisterConnectivityHook`. All are registered before `Start`, so lookup
  order is irrelevant; because the tracker is registered first its `Init` runs first and no dial
  can precede its mux registration.
- `Close`: stops the coalescing timer, marks the run `closed`. A run closed before terminal emits
  nothing further; the snapshot keeps its last non-terminal phase. The next `Begin` resets.
- `Begin` on a tracker whose previous run is not terminal is legal (an `AccountStop` +
  `AccountSelect` cycle) and simply starts a new `runId`.

### State (all under `mu`)

```go
type Tracker struct {
    mu sync.Mutex
    run     runState          // runId, mode, networkId, startedAt, sender, closed
    nextId  int64             // next event id to assign
    phase   phaseState        // base phase, waitingForNetwork bool, phaseStartedAt, failed *Error
    net     netState          // per-peer map + totals (below)
    account accountState      // fetchStarted, fetchPeer, attempt, lastError, ready, readyAt, techSpaceId
    spaces  map[string]*spaceState
    views   viewGate          // tech-space SpaceView-completeness gate (R11)
    pending map[coalesceKey]*pb.EventAccountRecoveryUpdate   // coalesced, unpublished
    timer   *time.Timer
    now     func() time.Time  // injectable clock
    deps    deps              // nodeconf, sender (bound in Init/Begin)
}
type peerState struct {
    kind          PeerKind; nodeTypes []string
    open          int        // open connections, clamped at 0 (never a latched bool)
    dialAttempts  int        // DialFailed count since last Connected; reset on Connected
    lastError     *Error; lastTransport string; lastProto uint32
    discoveredLocally bool
}
type netState struct {
    peers            map[string]*peerState
    openTotal        int        // sum of open over peers
    openNodes        int        // sum of open over NetworkNode peers
    dialsStarted     bool
    outageSince      time.Time  // zero when no outage; set on first DialFailed while openTotal==0
    incompatibleLatched bool    // first DialFailed with ErrIncompatibleVersion; never cleared
    discovery        DiscoveryState
    deviceOffline    bool
}
type spaceState struct {
    spaceViewId string; kind SpaceKind
    state       SpaceState; from SpaceState
    attempt     int        // pull attempts (PullEventResult with err), reset on success
    lastError   *Error
}
type viewGate struct {
    diffSeen     bool                // a qualifying tech-space SyncAll has been observed
    unresolved   map[string]struct{} // tree ids the latest diff reported missing, not yet arrived
    stalledDiffs int                 // consecutive qualifying diffs in which nothing resolved
}
```

### Mutex discipline

- Every producer entry point: `defer containTelemetry("name")` (recover + log), `mu.Lock()`,
  mutate, `mark(...)`, `mu.Unlock()`. Nothing else.
- **Never under `mu`**: pool calls (never at all, anywhere in the package), objectstore or
  subscription access, space-view lookups, logging of anything larger than ids.
- Allowed under `mu`: `nodeconf.NodeTypes` (RLock, no I/O), building pb structs,
  `event.Sender.Broadcast`/`SendToSession` (verified non-blocking: `core/event/event_grpc.go`
  `sendEvent` enqueues onto a bounded per-session queue and closes overflowing sessions on its
  own goroutine), `time.AfterFunc`.
- Sends happen **under** `mu` so ids and delivery order can never disagree (importv2's `mark`
  rule). The send never blocks, so no client can hold the lock.
- Reentrancy: the fold never calls into any-sync from a callback, so the "pool call re-enters
  the observer on the same goroutine" hazard cannot arise. The mux fan-out is synchronous and
  holds no lock while calling observers.
- The `localdiscovery` possibility hook runs under `localDiscovery.hookMu`
  (`space/spacecore/localdiscovery/common.go:131-143`): the tracker's handler is lock-mutate-
  mark-unlock only, same as the dial path.
- The tracker owns **no goroutine** besides the coalescing timer callback.

### Event ids and emission

`mark(ev, immediate)` mirrors importv2: assign `id = nextId++` **at publication**, not at arrival.
Immediate edges publish now (together with any pending window contents, so ordering stays
causal); coalesced events go into `pending` keyed by `(kind, peerId|spaceId)` where a later
arrival replaces the earlier one (levels, so last-wins is correct). The trailing-edge timer
flushes `pending` as one `pb.Event` with N messages in key insertion order. Publication under
`mu`. A closed run drops everything.

`Snapshot()` locks, calls `buildSnapshotLocked()`, unlocks. `lastEventId = nextId-1`. Pending
(unpublished) state is already folded into the snapshot; when it later publishes with a higher id
the client re-applies an identical level. No drift is possible because push events are emitted
from the same mutated state the snapshot is built from.

### Coalescing (R5)

| Event | Policy |
|---|---|
| `Started`, `PhaseChanged`, `AccountFetchStarted`, `AccountFetchError`, `AccountReady`, `Finished`, `LocalDiscoveryState` | immediate |
| `PeerDiscovered` | immediate (first per peer); repeats for a known peer are dropped |
| `DialStarted` | immediate for a peer with `dialAttempts == 0`; else coalesced (per peer) |
| `PeerConnected` | immediate (it is a state edge: `open` 0 -> 1, or a transport upgrade) |
| `DialFailed` | immediate on the first failure per peer; subsequent failures coalesced per peer, `attempt` as a level |
| `PeerDisconnected` | coalesced per peer (an idle-TTL close followed by a redial is noise) |
| `SpaceDiscovered` | immediate |
| `SpaceStateChanged` | immediate when `state != fromState` or `error` changes; coalesced per space when only `attempt` changed (repeated pull failures) |

Window 250 ms, trailing-edge timer, same constants as importv2 (`statWindow`).

### Phase derivation

Base phase is a monotone max over milestones; `WaitingForNetwork` is an overlay; `Failed` is
terminal. `wirePhase()`:

```
if failed                       -> Failed
if finished                     -> Done
if waitingForNetwork            -> WaitingForNetwork
if accountReady                 -> LoadingSpaces
if account.fetchStarted && openNodes > 0 -> FetchingAccount
if dialsStarted                 -> Connecting
else                            -> LookingForPeers
```

- `FetchingAccount` requires an open **NetworkNode** connection (correction 8): before one exists
  the pull loop is merely waiting in `GetResponsiblePeers`, and the honest headline is
  "connecting". `AccountFetchStarted{peerId:""}` is still emitted at `PullEventWaiting` so a rich
  client can show "will fetch account" under the Connecting headline.
- A warm start jumps `Connecting -> LoadingSpaces` (tech space from disk); clients accept jumps.
- `PhaseChanged` is emitted whenever `wirePhase()` changes; `previousPhaseDurationMs` is measured
  from the previous edge. Entering `WaitingForNetwork` carries `error` (class below); leaving it
  returns to the current base phase with `fromPhase = WaitingForNetwork`.

**WaitingForNetwork rule (correction 2)** — all three, re-evaluated on every net event and by a
one-shot timer armed at `outageSince + 10s`:

1. `openTotal == 0`, and
2. at least one `DialFailed` since the last `PeerConnected` (`outageSince` non-zero), and
3. `now - outageSince >= 10s`.

`outageSince` is set on the first `DialFailed` observed while `openTotal == 0`, cleared on any
`PeerConnected`. An idle-TTL `Closed` alone never satisfies (2), so it cannot fake an outage.
Additionally, if the device reports offline (`networkState.IsOffline()` at `Begin`, or the
connectivity hook with `online=false`) the overlay is entered immediately with class
`NoNetwork`; the hook with `online=true` clears `deviceOffline` but the overlay only lifts on a
real `PeerConnected`. Class while waiting: `NoNetwork` if `deviceOffline || discovery ==
NoInterfaces`, else `IncompatibleVersion` if `incompatibleLatched`, else `PeerUnreachable`.
`retryable = class != IncompatibleVersion`.

`Failed` is set only by `Tracker.Fail(err)` from the application start path, with the class from
the mapping table; `Finished` is never emitted after `Failed`.

### Peer folding (corrections 1, 8, 12)

- `KindDialStarted`: ensure `peers[id]`; resolve kind once: `nodeconf.NodeTypes(id)` non-empty
  => `NetworkNode` + types, else `LocalPeer`; `dialsStarted = true`.
- `KindConnected`: `open++`, `openTotal++`, `openNodes++` if node; `dialAttempts = 0`,
  `lastError = nil`, `outageSince = zero`; `durationMs = 0` when `Inbound` (never render 0 ms —
  the field is documented as unknown). `Addr` is display-only; never compared.
- `KindDialFailed`: `dialAttempts++`, `lastError = classify(err)`; if `openTotal == 0 &&
  outageSince.IsZero()` set `outageSince = now` and arm the 10 s timer; if
  `errors.Is(err, handshake.ErrIncompatibleVersion)` set `incompatibleLatched` (the first failure
  is observed; the pool suppresses re-dials for up to 20 min after).
- `KindClosed`: `open--` clamped at 0; totals follow. No reason is available; nothing else is
  inferred (an idle-TTL close looks identical to a failure by design).
- `attempt` on `DialFailed` is *dials observed* (correction 1); there is no `nextRetryMs`.
- `PeerDiscovered` (LAN): `discoveredLocally = true`, kind `LocalPeer` if not yet a node.

### Account folding

- `PullEventWaiting{techSpaceId}` -> `account.fetchStarted`, `AccountFetchStarted{peerId:""}`.
- `PullEventAttempt{techSpaceId, peer}` -> `AccountFetchStarted{peerId}`.
- `PullEventResult{techSpaceId, err != nil}` -> `attempt++`, `AccountFetchError{classify(err)}`.
- `OnAccountReady(techSpaceId)` (from `space/init.go`, both `loadTechSpace` and
  `createTechSpace`) -> `account.ready`, `AccountReady{durationMs: now - startedAt}`, the tech
  space entry is created (or promoted) as `kind: Tech, state: Loaded`, phase -> `LoadingSpaces`.
- Pull events for other space ids drive that space's `Pulling` state instead (below). The tech
  space id is derived in `space.Init`; the tracker learns it from the first producer call that
  carries it (`OnAccountReady`, `OnSpaceViewStatus`). Until then a `PullEvent` for an unknown id
  is folded as a regular space and re-labelled `Tech` at `OnAccountReady` (harmless: the tech
  space is in `spaces[]` anyway, R9).

### Space state machine (corrections 3, 5; override of decision 1)

`Queued -> Pulling -> Loading -> Loaded | Error`. Inputs and transitions per `spaceId`:

| Input | From | To / effect |
|---|---|---|
| `OnSpaceViewStatus` first seen, accountStatus not Deleted/Removing, remote not Deleted | — | create as `Queued`; emit `SpaceDiscovered`; resolve the view id in the tech-space gate (below) |
| `OnSpaceViewStatus` with Deleted/Removing/RemoteDeleted | any | not tracked (emit `SpaceStateChanged{Error, SpaceDeleted}` if it was, then drop; counters exclude it); still resolves the gate |
| `PullEventWaiting/Attempt` | `Queued` | `Pulling` |
| `PullEventResult{err}` | `Pulling` | stay, `attempt++`, `error = classify(err)` (the retry is `loadingSpace.loadRetry`'s, `loadingspace.go:74-104`: 1 s × 1.5 backoff capped at `retryTimeout`, exits only on success, a non-retryable error or ctx cancel) |
| `PullEventResult{nil}` | `Pulling` | `Loading` |
| `OnSpaceLoadStarted(optimistic=false)` | `Queued`/`Pulling` | `Loading` |
| `OnSpaceLoadStarted(optimistic=true)` | `Queued` | `Loaded` directly — the optimistic fast path in `spaceloader.startLoad` never publishes `Loading`; "no transition seen" means already loaded (correction 5) |
| `OnSpaceLoaded(nil)` | `Loading`/`Loaded` | `Loaded` (idempotent for the optimistic path); `error = nil` |
| `OnSpaceLoaded(err)`, `err` is `context.Canceled`/`DeadlineExceeded` | any | **no transition** — `onLoad` deliberately leaves the persisted status untouched on shutdown (`spaceloader.go:145-150`); the fold mirrors that |
| `OnSpaceLoaded(err)`, any other error | any | `Error` with `classify(err)`; terminal for the space |

`Loaded` is exactly `LocalStatusOk` from `spaceloader.onLoad`; `Error` is exactly
`LocalStatusMissing`. There is no state after `Loaded` on this surface. During a genuine outage
`loadRetry` never returns, so a space stays `Loading` and the run stays open — with
`WaitingForNetwork` as the headline, which is the truthful rendering.

The tech space is `Loaded` at `OnAccountReady` and has no `Error` (a tech-space failure is the
run's `Failed`).

### Tech-space completeness gate (R11)

**What it protects.** `Finished` must not fire on a partial SpaceView set. On a fresh device the
tech space is pulled with its ACL and settings only; the SpaceView trees arrive through the tech
space's head-sync. Tech-space `LocalStatusOk` (i.e. `AccountReady`) happens before any of that,
so it cannot serve as "all spaces are known".

**Signal chosen.** heart's `treesyncer.SyncAll(ctx, p, existing, missing)`
(`core/block/object/treesyncer/treesyncer.go:196-248`) is the head-sync outcome per space; it
already computes `isResponsible := slices.Contains(t.nodeConf.NodeIds(t.spaceId), peerId)`, and
`missing` is the list of tree ids the peer has that we do not. The treesyncer gains an optional
`HeadSyncObserver` resolved like `PriorityProvider` (`app.GetComponent`, parent-chain lookup
works from the space child app), called after `sendSyncEvents` outside `t.Lock`, for every space
— the fold ignores every `spaceId` except the tech space. Gate rule, all under `mu`:

- `OnHeadSync(techSpaceId, peer, missing, responsible=true)`: `diffSeen = true`;
  `unresolved = set(missing) \ {ids already seen as SpaceViews}`. A later diff **replaces** the
  set (a tree no longer reported missing is resolved).
- `OnSpaceViewStatus(spaceViewId)`: `delete(unresolved, spaceViewId)`. SpaceView object id ==
  tree id (SpaceViews are unique-key-derived objects, `techspace.go:277-290`; the subscription's
  `spaceViewId` is `RelationKeyId`, `space/spacesub.go`).
- `stalledDiffs`: incremented on each qualifying diff in which `unresolved` did not shrink (no id
  resolved since the previous diff); reset to 0 whenever any id resolves.
- `viewsConfirmed := diffSeen && len(unresolved) == 0` — the gate opened properly.
- `viewsStalled := diffSeen && stalledDiffs >= 2` — the **stall bound** (~40 s at `SyncPeriod`
  20): a tree id that no diff ever resolves (a node not serving it, a persistent fetch failure)
  must not hold `Finished` forever. It is self-limiting: with the network down no diffs arrive,
  the counter cannot advance, and the spaces themselves block `Finished` anyway. It fires only
  in the target case — diffs flowing, everything else healthy, one id permanently stuck.
- `gateOpen := viewsConfirmed || viewsStalled`; `Finished.viewsConfirmed` reports which.
- In `RpcAccount_LocalOnly` mode (R10) `responsible` is not required: there are no responsible
  nodes, LAN peers are the only source.

**Why this and not the alternatives inspected.**
- *Tech-space `LocalStatusOk`* — too early, as above.
- *First responsible diff alone* — proves the set is **known**, not present; `Finished` could
  fire while the missing SpaceViews are still downloading, exactly the partial set to avoid.
- *`syncsubscriptions.SyncingObjectsCount` for the tech space only* — correct but a 1 s poll
  goroutine plus objectstore subscription access for one gate; the diff + arrival rule is
  event-driven and needs neither.
- *A "missing trees fetched" completion hook in the treesyncer request pool* — precise but
  invasive (per-`SyncAll` wait groups around `requestTree`); the arrival of the SpaceView on the
  subscription we already consume is the same fact one hop later.
- *Any-sync `headsync`* — exposes only `DiffSync(ctx) error`; nothing narrower exists.

**Latency.** Non-SpaceView tech objects (account object, personal favorites) are fetched or
created in `techSpace.Run` before `AccountReady`, so on cold recovery the first diff's `missing`
is normally SpaceViews only and the gate resolves the moment the last one arrives. If a
non-SpaceView tree is ever in `missing` (a future tech-space object type) and it *does* arrive,
it resolves on the next diff, bounded by `SyncPeriod` (20 s, `core/anytype/config/config.go:542`).
If an id never arrives, the stall bound opens the gate after two stalled diffs with
`viewsConfirmed=false`. Both paths are event-driven; no poll.

### Finalization

After any space transition or gate update: if `account.ready && gateOpen && every tracked
space is Loaded or Error` then emit `Finished{spacesTotal, spacesLoaded, spacesFailed,
totalDurationMs, viewsConfirmed}` and `PhaseChanged -> Done`, mark `finished`. `spacesTotal`
counts tracked spaces including the tech space (R9). A brand-new account (`NewAccount`) has a
trivially confirmed gate (its first diff has nothing missing) and finishes when its first created
space loads. Nothing is emitted after `Done`; a `SpaceDiscovered` after `Finished` is ignored —
the run is terminal. With `viewsConfirmed=true` that is normal operation covered by the legacy
surfaces; with `viewsConfirmed=false` the client must not treat the run's space list as
complete and keeps relying on its own SpaceView subscription for spaces that appear later.

There is deliberately **no stall cap**: the only per-space input is the loader's own result, and
the loader retries forever through a real outage by design. The run staying open under
`WaitingForNetwork` is the correct rendering of that; a cap would manufacture a `Done` the
loader never reported. (The failure mode that would have justified one — objects trickling in
forever while everything else looked fine — existed only under the settle heuristic this revision
removed.)

### Error-class mapping

`classify(err) Error` is one function, table-tested. `errors.Is` throughout; joined dial errors
(`errors.Join`, unwrap via `interface{ Unwrap() []error }`) are classified by the **best** member
in the order below (a version mismatch on one address outranks a timeout on another).

| Source error | ErrorClass | retryable |
|---|---|---|
| `handshake.ErrIncompatibleVersion`, `ErrIncompatibleProto`, `ErrRemoteIncompatibleProto`; `nodeconf.NetworkCompatibilityStatusIncompatible` | `IncompatibleVersion` | false |
| `handshake.ErrInvalidCredentials`, `ErrPeerDeclinedCredentials`, `ErrSkipVerifyNotAllowed`; `coordinatorproto.ErrForbidden` | `NotAuthorized` | false |
| `coordinatorproto.ErrAccountIsDeleted`; `spacesyncproto.ErrSpaceIsDeleted` / `coordinatorproto.ErrSpaceIsDeleted` / `spacecore.ErrSpaceIsDeleted` **for the tech space** | `AccountDeleted` | false |
| `spacesyncproto.ErrSpaceIsDeleted`, `coordinatorproto.ErrSpaceIsDeleted`, `ErrSpaceDeletionPending`, `spacecore.ErrSpaceIsDeleted`, `spacecore.ErrSpaceDeletionPending`, `spaceloader.ErrSpaceDeleted` | `SpaceDeleted` | false |
| `spacesyncproto.ErrSpaceMissing` for the tech space; `space.ErrSpaceNotExists`; `application.ErrFailedToFindAccountInfo` | `AccountNotFound` | false |
| `spacesyncproto.ErrTooManyRequestsFromPeer`, `ErrDuplicateRequest` | `RateLimited` | true |
| `net.ErrUnableToConnect`, `peerservice.ErrAddrsNotFound`, `handshake.ErrDeadlineExceeded`, `context.DeadlineExceeded`, `*net.OpError`, any `net.Error` with `Timeout() == true` (covers quic-go handshake/idle timeouts without a direct quic-go import), `peermanager.ErrPeerFindDeadlineExceeded`, `spacesyncproto.ErrPeerIsNotResponsible` (try another peer) | `PeerUnreachable` | true |
| device offline / `DiscoveryNoInterfaces` with zero connections (derived, not from an error) | `NoNetwork` | true |
| `objecttree.ErrHasInvalidChanges`, `spacedomain.ErrUnexpectedSpaceType`, `anystore.ErrCollectionNotFound`, `spacestorage` persist errors on `PullEventResult` | `Unexpected` | false |
| `context.Canceled` | not classified — dropped (shutdown, never an error) | — |
| anything else | `Unexpected` | true |
| *(reserved)* `StorageLimit` | no producer in this version; kept for the file surface | — |

`debugMessage = err.Error()` truncated to 256 bytes. Per-space `Error` state uses the same table;
a space in `Error` with `retryable=true` is still counted as `spacesFailed` at `Finished` — the
loader gave up, so the run's verdict is honest.

### Snapshot-on-attach

`Init` registers a `session.HookRunner` hook that `SendToSession(token, Update{Snapshot})`.
Known limitation: `ListenSessionEvents` calls `mw.GetApp()` *after* registering the stream
(`core/grpc_events.go:38-49`), and `GetApp()` blocks on `Service.lock` while `AccountSelect` runs,
so a session attaching mid-start receives live broadcasts from attach time and its `Snapshot`
when `AccountSelect` returns. `AccountRecoveryState` is lock-free and serves the gap. Sessions
that attach before `AccountSelect` (the normal client flow) see `Started` as id 1.

## Producer wiring

| Producer | Exact site | Mechanism | Tracker method -> event |
|---|---|---|---|
| Application start | `core/application/account_select.go` `start()` (and the two create sites, `account_stop.go` restart) via a shared `startApp` helper | direct handle, before `StartNewApp`; on error | `Begin` -> `Started`; `Fail(err)` -> `PhaseChanged{Failed}` |
| Peer lifecycle | `net/peerobservermux` (new) registered in `core/anytype/bootstrap.go` right after `peerstore.New()`, before `pool.New()`/`peerservice.New()`; `Name()` returns `peerobserver.CName` (correction 7) | tracker `Init` -> `mux.Add(t)`; mux fans out synchronously with per-observer panic containment | `ObservePeerEvent` -> `DialStarted`, `PeerConnected`, `DialFailed`, `PeerDisconnected` |
| LAN discovery possibility | `localdiscovery.RegisterDiscoveryPossibilityHook` (`common.go:145`) | tracker `Init`, same interface `peerstatus` uses | `OnDiscoveryPossibility` -> `LocalDiscoveryState` |
| LAN peer found | `space/spacecore/peer.go` `PeerDiscovered` — first line, before the pool `Get` (correction 6; Android's `SetNotifierProvider` is upstream of this point, so one seam covers all backends) | `spacecore.Init`: `a.Component("core.recovery")` type-asserted to a `PeerDiscoveryObserver` interface declared in `spacecore`; nil-safe | `OnLocalPeerDiscovered` -> `PeerDiscovered` |
| Remote pull | `space/spacecore/service.go` `loadSpace`: `deps.PullObserver = s.recovery` (the single `commonspace.Deps` literal; covers tech/personal/shareable/streamable/one-to-one; correction 13) | same optional lookup, asserted to `commonspace.PullObserver` | `ObservePullEvent` -> `AccountFetchStarted/Error` or `SpaceStateChanged{Pulling}` |
| Tech space ready | `space/init.go` `loadTechSpace` and `createTechSpace`, immediately after `close(s.techSpaceReady)` | `space.Init`: optional lookup by name (precedent `BlockServiceCName`), narrow `recoveryObserver` interface declared in `space` | `OnAccountReady(techSpaceId)` -> `AccountReady`, tech space `Loaded` |
| Space discovery | `space/service.go` `onSpaceStatusUpdated`, first statement inside the goroutine (before the deleted/deferred branching) | same interface | `OnSpaceViewStatus(spaceId, spaceViewId, local, account, remote)` -> `SpaceDiscovered`; resolves the view gate |
| Space load lifecycle | `space/internal/components/spaceloader/spaceloader.go` `startLoad` (after the `onDiskAndOk` decision) and `onLoad` (after the switch, passing the raw `loadErr`) | `app.GetComponent[LoadObserver]` optional in `Init` (child app -> parent chain, `app.Component` walks `parent`) | `OnSpaceLoadStarted(id, optimistic)`, `OnSpaceLoaded(id, err)` -> `SpaceStateChanged{Loading/Loaded/Error}` |
| Tech-space completeness gate | `core/block/object/treesyncer/treesyncer.go` `SyncAll`, after `sendSyncEvents`, outside `t.Lock` | `app.GetComponent[HeadSyncObserver]` optional in `Init` (same as `PriorityProvider`); called for every space, fold filters on the tech space | `OnHeadSync(spaceId, peerId, missing, responsible)` -> gate only, no event |
| Device connectivity | `core/device` `NetworkState.IsOffline()` at `Begin`/`Init`, `RegisterConnectivityHook(func(online bool))` | optional lookup by name `"networkState"` (peermanager precedent) | `OnNetworkOnline` -> `WaitingForNetwork{NoNetwork}` overlay |
| New session | `session.HookRunner.RegisterHook` | tracker `Init` | `SendToSession(Snapshot)` |

Import-cycle audit (verified with `go list -deps`): `core/recovery` imports any-sync
(`app`, `commonspace`, `net/peerobserver`, `nodeconf`, `net/secureservice/handshake`, proto
error packages), `pb`, `core/event`, `core/session`, `space/spaceinfo`. `spaceinfo` reaches
`core/block/editor/state` but not `core/recovery`, and every producer in `space/...`,
`spaceloader`, `treesyncer`, `spacecore` resolves the tracker by **name or interface**, never by
import. Only `core/application` and `core/anytype` (bootstrap, for the mux) import the new
packages. Implementation checkpoint: `go build ./...` after each producer.

### `net/peerobservermux`

```go
type Mux struct { mu sync.RWMutex; observers []peerobserver.Observer }
func New() *Mux; func (m *Mux) Name() string { return peerobserver.CName }
func (m *Mux) Init(*app.App) error; func (m *Mux) Add(o peerobserver.Observer)
func (m *Mux) ObservePeerEvent(ev peerobserver.Event) // RLock, copy slice, unlock, then call each under peerobserver.Notify-style recover
```

Diagnostics (`core/debug`, transport penalties) can `Add` later without touching the slot.

## Warm start behavior

`WarmStart`: `Started` -> `LocalDiscoveryState` -> dials begin in the background
(`Connecting`) -> tech space from disk -> `AccountReady` within ~100 ms–1 s -> `LoadingSpaces`
-> spaces discovered from local SpaceViews at once (`Queued`); optimistic loads jump straight to
`Loaded`, the rest go `Loading -> Loaded` as their background builds finish -> the tech space's
first responsible diff (normally nothing missing) completes the gate -> `Finished`. With a
working network this is a few seconds and a few dozen events; the gate is the only network-
dependent step. Offline: `WaitingForNetwork{NoNetwork|PeerUnreachable}` after 10 s (or
immediately if the device says offline), on-disk spaces still reach `Loaded`, the gate never
completes, `Finished` never fires — honest, calm, and identical in shape to cold recovery. The
stream is recovery-scoped, so an offline warm start simply ends without `Finished` when the app
stops; there is no fake `Done`.

`NewAccount`: `Started{NewAccount}`; no `AccountFetch*`; `AccountReady` at `createTechSpace`;
the first created space loads; `Finished`.

## Interaction with existing surfaces

- `EventSpaceSyncStatusUpdate`, `EventP2PStatusUpdate`, SpaceView `LocalStatus/RemoteStatus/
  AccountStatus`, per-object status: untouched and unread. The tracker does not consult
  `nodestatus` (whose zero value is `Online`) — connectivity comes from the peer observer. This
  surface makes **no claim about object sync**; "how many objects are still syncing in this
  space" stays exactly where it is today, on `EventSpaceSyncStatusUpdate`.
- `peerstatus` keeps `RegisterDiscoveryPossibilityHook`; hooks are a slice, so two registrants
  coexist. `spacecore.SetNotifier` stays single-slot; the forward is inside spacecore.
- `AccountSelect`, `AccountPreloadRemainingSpaces`, lazy loading: unchanged. `PhaseChanged`
  and `AccountReady` are informational during the blocking window; the RPC result remains the
  client's "UI may start" trigger (R3).
- The QUIC-degradation work on the base branch changes dial *ordering*, not the observer
  contract; `transport` on `PeerConnected` is how a client sees which scheme won.

## Testing

Conventions: fixture pattern, `// given / when / then`, `want` structs, testify, mockery mocks
regenerated with `make test-deps` (`.mockery.yaml` gains `core/recovery` interfaces:
`HeadSyncObserver`, `LoadObserver`, `PeerDiscoveryObserver`, `recoveryObserver`, plus the
existing `core/event` `Sender` mock).

### `core/recovery` (unit, no app)

```go
type fixture struct {
    *Tracker
    sender   *mock_event.MockSender   // or a recording sender collecting []*pb.Event
    clock    *fakeClock                // now func() + manual timer firing
    nodeConf *mock_nodeconf.MockService
}
```

- Ids: strictly `1..n` per run; `Begin` resets and changes `runId`; nothing after `Done`/`Failed`
  /`Close`.
- Phase table: for each scripted input sequence, `want []Phase` edges — cold happy path
  (`LookingForPeers, Connecting, FetchingAccount, LoadingSpaces, Done`), warm path skipping
  `FetchingAccount`, `FetchingAccount` withheld until a node connection is open, `Failed`.
- WaitingForNetwork: idle-TTL `Closed` with no `DialFailed` never enters; `DialFailed` at t=0
  enters at t=10 s (timer), not at 9.9 s; `Connected` at t=5 s cancels; device-offline enters
  immediately with `NoNetwork`; leaving returns `fromPhase = WaitingForNetwork`.
- Peer counting: `Connected(A), Connected(A'), Closed(A)` leaves `open == 1`; `Closed` below zero
  clamps; `attempt` increments per `DialFailed` and resets on `Connected`; incompatible-version
  latch survives later failures; inbound `durationMs == 0`; kind from `NodeTypes`.
- Spaces: every row of the state table; optimistic `Queued -> Loaded`; `OnSpaceLoaded` with
  `context.Canceled`/`DeadlineExceeded` leaves the state untouched; deleted views untracked;
  `SpaceDiscovered` after `Finished` ignored; `Finished` counters; tech space `Loaded` at
  `AccountReady` and never `Error`.
- Tech-space gate: no diff -> no `Finished` even with all spaces `Loaded`; diff with
  `missing = {v1, v2}` -> `Finished` only after both arrive via `OnSpaceViewStatus`; a second
  diff with `missing = {}` resolves a leftover non-view id; a deleted view still resolves;
  non-responsible diff ignored outside LocalOnly and accepted in LocalOnly; a diff for a regular
  space id is ignored. Stall bound: `missing = {stuck}` on three consecutive diffs -> `Finished{
  viewsConfirmed: false}` after the third (two stalled), never after the second; a diff that
  resolves one of two ids resets the counter; `viewsConfirmed: true` on the normal path.
- Coalescing: 5 `DialFailed` for one peer in 100 ms -> first immediate, one trailing flush with
  `attempt == 5`, one `pb.Event`; a `PhaseChanged` mid-window flushes pending first (ordering).
- **Replay property**: fold every emitted `Update` into a client-side model; `assert.Equal(t,
  want=fx.Snapshot(), got=model)` after each step. This is the "push and pull cannot drift" test.
- `classify`: table test, one row per line of the mapping table, including `errors.Join` shapes.
- Panic containment: a sender that panics does not propagate out of `ObservePeerEvent`.
- Snapshot after `Close` mid-run keeps the last phase; `Begin` after `Close` starts clean.

### Producers

- `net/peerobservermux`: fan-out order, an observer added mid-dispatch is not called for the
  in-flight event, a panicking observer does not starve the others.
- `space/spacecore`: `loadSpace` sets `deps.PullObserver` when `core.recovery` is registered,
  leaves it nil otherwise (existing `TestService` fixture, `Register(testutil.PrepareMock(...))`);
  `PeerDiscovered` forwards before dialing.
- `spaceloader`: `startLoad` forwards `optimistic=true` on the on-disk-Ok path and `false`
  otherwise; `onLoad` forwards the raw error on every branch, including the canceled one; no
  observer -> no call (existing loader tests + mock).
- `treesyncer`: `SyncAll` forwards `responsible` consistent with `nodeConf.NodeIds` and the
  `missing` slice unchanged (existing fixture at `treesyncer_test.go:41`).
- `space`: `onSpaceStatusUpdated` forwards once per status; `loadTechSpace` calls
  `OnAccountReady` after `techSpaceReady` is closed (existing `newFixture` in `service_test.go`).
- `core/application`: `startApp` emits `Started{ColdRecovery}` when the repo was missing,
  `WarmStart` otherwise, `Fail` on `StartNewApp` error; `AccountRecoveryState` returns
  `ACCOUNT_IS_NOT_RUNNING` before any `Begin`.

### Manual / integration checkpoint

Cold recovery of a real multi-space account on the dev network with `ANYTYPE_LOG_LEVEL=core.
recovery=debug` (the tracker logs each published id and kind — ids and codes only, never
content). Expected: `Started` before any other log line of `app start`, `AccountReady` within
the same second as `techSpaceReady`, `Finished` after the last space's `Loaded` and no earlier
than the last SpaceView's arrival. Repeat with Wi-Fi off after `Started`: `WaitingForNetwork` at
+10 s, no `Finished`, no error-level logs.

## Implementation plan

Each phase compiles, passes `go test ./core/recovery/... ./net/peerobservermux/...` and the
touched packages, and is a reviewable commit (`GO-7471 ...`).

1. **Proto + RPC skeleton.** Add the messages, `make protos`, `AccountRecoveryState` handler
   returning `ACCOUNT_IS_NOT_RUNNING`. Checkpoint: `go build ./...`, generated code committed.
2. **Fold.** `core/recovery`: state, `Begin/Fail/Close/Snapshot`, phase derivation, coalescing,
   ids, `classify`, session hook, application ownership + `startApp` helper, `Started`/`Failed`
   end to end. Checkpoint: unit suite incl. the replay property; a client sees `Started` and
   `Snapshot` on a warm start.
3. **Peers.** `net/peerobservermux`, bootstrap registration, tracker as observer, LAN
   possibility hook, `PeerDiscovered` forward, device-offline overlay, WaitingForNetwork.
   Checkpoint: cold start with Wi-Fi off reaches `WaitingForNetwork{NoNetwork}` at +10 s.
4. **Account.** `PullObserver` injection in `loadSpace`, `AccountReady` at `techSpaceReady`.
   Checkpoint: cold recovery shows `FetchingAccount` only after a node connection, then
   `AccountReady` before `AccountSelect` returns.
5. **Spaces.** Discovery forward, spaceloader forward (`startLoad` + `onLoad`), treesyncer
   forward for the tech-space gate, finalization, `Finished`. Checkpoint: `Finished` counters
   match the SpaceView set; on a fresh device `Finished` never precedes the last SpaceView's
   arrival.
6. **Hardening + docs.** Coalescing tuning from a real event log, `docs/Flow.md` cross-link,
   Linear follow-ups: analytics-id poll (R3), `accountObjectExists` create-if-missing after a
   15 s timeout (observed, out of scope), `debugMessage` sizing.

## Risks and open issues

- **`FetchingAccount` gate on a node connection** hides the case where the coordinator is
  reachable but tree nodes are not: the headline stays `Connecting` with `DialFailed` events for
  the tree nodes underneath. Acceptable and honest; a rich client can render "reached
  coordinator, tree node unreachable" from `nodeTypes`.
- **No stall cap, by decision.** A space in `Loading` through a real outage keeps the run open
  indefinitely; `WaitingForNetwork` is the headline for exactly that case. A cap was considered
  and rejected: it would emit a `Done` the loader never reported, and the failure mode that
  motivated one (objects trickling forever while everything else looked settled) cannot occur
  now that the only per-space input is the loader's own result.
- **Tech-space gate: arrival latency vs. permanent stall.** A missing id that *does* arrive
  costs at most one extra diff (≤ `SyncPeriod`, 20 s) when it is not a SpaceView; today the
  account object and personal favorites are fetched before `AccountReady`, so this does not
  happen. An id that *never* arrives (node not serving it, persistent fetch failure) would hold
  the gate forever — every later diff keeps reporting it — hence the stall bound: two
  consecutive resolving-nothing diffs (~40 s) open the gate with `viewsConfirmed=false`, and the
  client is told the completeness claim was not earned. The bound cannot fire while the network
  is down (no diffs), so it never converts an outage into a fake `Done`.
- **Deleted-while-recovering** (`RemoteStatusDeleted` after `SpaceDiscovered`): the space is
  dropped from tracking; a `SpaceStateChanged{Error, SpaceDeleted}` is emitted first so the
  client's list can reconcile. Counters exclude it.
- **`accountObjectExists` fallback** (`space/techspace/techspace.go:159-170`): after a 15 s
  `GetObject` timeout the tech space *creates* a new account object; `AccountReady` fires after
  that regardless, so a slow network can produce a misleading "ready". Pre-existing behavior;
  reported, not fixed here.
- **Mid-start session attach** gets its `Snapshot` late (see "Snapshot-on-attach"). Clients that
  attach after `AccountSelect` is already in flight must call the RPC.
- **Field numbers** are assigned here; if a concurrent branch claims `206` in the
  `Event.Message` oneof, renumber at merge — nothing else in this design depends on the value.
