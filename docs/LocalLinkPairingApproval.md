# Local-link pairing: approve before minting the code

Spec for GO-7395. Supersedes the pairing-hardening items of GO-7394.

Status: implemented in heart. The desktop client still has to build the prompt
and call the new RPC — until it does, nothing can pair (§8.2).

## 1. Goal

Today the 4-digit pairing code exists from the moment a challenge is requested,
before any human has agreed to anything. Everything protecting it is a counter.

This spec moves the code's creation behind an explicit user approval, so an
unapproved request has no secret associated with it and there is nothing to
guess. The requesting client's API is unchanged.

### Non-goals

- Changing `POST /v1/auth/challenges` or `POST /v1/auth/api_keys` request or
  response shapes. Existing integrations must keep working untouched.
- Replacing the 4-digit code with one-click authorization (see §11).
- The Origin/CORS work in GO-7394, which ships independently.

## 2. Current flow

```
client                  heart                          desktop UI
  │
  ├─ NewChallenge ──────▶ StartNewChallenge
  │                       └─ mints code immediately
  │                       └─ Broadcast(LinkChallenge{challenge: "4821"}) ──▶ shows "4821"
  ◀── challengeId ───────┤
  │
  │  (user reads 4821 off the screen, types it into the client)
  │
  ├─ SolveChallenge ────▶ compares, mints app key
  ◀── appKey ────────────┤
                          └─ Broadcast(LinkChallengeHide{challenge: "4821"})
```

Relevant code:

| Concern | Location |
| --- | --- |
| Code minting, per-challenge and per-run budgets | `core/session/challenge.go` |
| Challenge state map | `core/session/service.go` |
| Event broadcast, app-key persistence | `core/application/sessions.go:102-150` |
| RPC entry points, `ClientInfo` assembly | `core/account.go:263-300`, `:346` |
| Method authorization | `core/auth.go:20-54` |
| JSON API surface | `core/api/service/auth.go`, `core/api/server/router.go:120-126` |

## 3. New flow

```
client                  heart                          desktop UI
  │
  ├─ NewChallenge ──────▶ StartNewChallenge
  │                       └─ state=pending, NO code
  │                       └─ Broadcast(LinkChallenge{
  ◀── challengeId ───────┤      clientInfo, needApprove: true })  ──▶ "Google Chrome
  │                       │                                          chrome-extension://abc…
  │                       │                                          wants to connect"
  │                       │                                          [Allow] [Deny]
  │                       │
  │                       ◀── ApproveChallenge(processPath, origin, allow: true) ──┤
  │                       └─ state=approved, mints code
  │                       ├── challenge: "4821" ─────────────────▶ shows "4821"
  │
  │  (user types 4821 into the client)
  │
  ├─ SolveChallenge ────▶ requires state=approved
  ◀── appKey ────────────┤
                          └─ Broadcast(LinkChallengeHide{clientInfo})
```

The client's two calls are byte-identical to today. The only observable
difference is that the code appears later.

## 4. State machine

```
                    ApproveChallenge(allow=true)
   ┌─────────┐  ────────────────────────────────▶  ┌──────────┐
   │ pending │                                     │ approved │
   └─────────┘  ◀──── (no path back) ───────────   └──────────┘
        │                                                │
        │ ApproveChallenge(allow=false)                   │ SolveChallenge OK
        │ or TTL expiry                                   │ or tries exhausted
        ▼                                                ▼
   ┌─────────┐                                      ┌──────────┐
   │ dropped │                                      │ finished │
   └─────────┘                                      └──────────┘
```

- `pending` — no code exists. `SolveChallenge` fails regardless of the answer.
- `approved` — code exists, `challengeMaxTries` applies as today.
- Both terminal states delete the entry and broadcast `LinkChallengeHide`.

Invariant: **a challenge value is never generated in the `pending` state, and
never leaves the process except as the `ApproveChallenge` response.**

## 5. Proto changes

`pb/protos/events.proto` — the former `LinkChallenge`/`LinkChallengeHide`
events are **replaced** by a new, cleanly-named pair. A rename rather than an
edit-in-place: the old events broadcast the code, the new ones never do, and a
distinct name means a client cannot subscribe to the old shape by accident.
Events are not persisted, so the oneof field numbers (204/205) are reused.

```proto
// asks the user to approve a pairing; names the caller, carries no code
message LinkApprovalRequest {
  message ClientInfo { ... }              // unchanged; `origin` already added
  ClientInfo clientInfo = 2;
  model.Account.Auth.LocalApiScope scope = 3;
}

// take the prompt off screen once answered, denied or expired
message LinkApprovalHide {
  LinkApprovalRequest.ClientInfo clientInfo = 2;
}
```

There is no `needApprove` flag: the event exists only to request approval, so
its arrival is the signal. There is no `challenge` field: no path broadcasts a
code, so no field should invite reading one. The hide event is keyed on the
caller, since a request that was never approved never had a code.

Generated Go names follow: `EventAccountLinkApprovalRequest`,
`EventAccountLinkApprovalRequestClientInfo`, `EventAccountLinkApprovalHide`.

`pb/protos/commands.proto` — new `Rpc.Account.LocalLink.ApproveChallenge`:

```proto
message ApproveChallenge {
    message Request {
        string processPath = 1;   // both verbatim from the event's ClientInfo
        string origin = 2;
        bool allow = 3;
    }
    message Response {
        Error error = 1;
        string challenge = 2;   // the 4-digit code; empty when allow=false
        message Error {
            Code code = 1;
            string description = 2;
            enum Code {
                NULL = 0;
                UNKNOWN_ERROR = 1;
                BAD_INPUT = 2;             // includes a browser-origin caller
                ACCOUNT_IS_NOT_RUNNING = 101;
                NO_PENDING_CHALLENGE = 102;  // never asked, decided, or expired
            }
        }
    }
}
```

The pending challenge is addressed by the caller the prompt displayed, not by a
challenge id. Only one challenge can be pending per caller (§7.1), so the pair
is unambiguous, and it is the durable identity a future "always allow this
origin" would key on — a challenge id is discarded minutes later.

`pb/protos/service/service.proto` — register
`AccountLocalLinkApproveChallenge` alongside the existing LocalLink RPCs
(`service.proto:18-22`).

`Rpc.Account.LocalLink.SolveChallenge.Response.Error.Code` — add:

```proto
CHALLENGE_NOT_APPROVED = 105;
```

Regenerate with `make protos` (or the `protos-go` recipe plus
`--doc_out` for `docs/proto.md`).

## 6. Authorization — the load-bearing requirement

`AccountLocalLinkApproveChallenge` **must not be added to `noAuthMethods` or
`limitedScopeMethods`** in `core/auth.go`. Falling through both maps means
`Authorize` requires a valid session token and admits only
`model.AccountAuth_Full` (`core/auth.go:77-86`) — exactly the desktop UI, and
nothing else. A `JsonAPI`- or `Limited`-scoped token hits the `default` branch
and is refused.

If it ever lands in `noAuthMethods`, any local process approves its own pairing
and this design is strictly weaker than what ships today. Guard this with a test
(§10).

Additionally:

- **No JSON API route.** Do not register it in `registerAuthRoutes`.
- **Reject any call carrying an `Origin` header.** The gRPC-Web proxy trusts the
  Webclipper's `chrome-extension://` origins (`cmd/grpcserver/proxy.go:37-43`),
  so a browser context can reach the RPC surface. Approval is a desktop-UI-only
  action; a request with any `Origin` must be refused even with a valid token.

## 7. Anti-spam

The scarce resource is now the user's attention: anyone who can reach the
endpoint can make a prompt appear, and a prompt interrupts where a counter did
not.

### 7.1 One pending approval per caller

While a caller has a challenge in `pending`, a second request from the same
`callerKey` (`core/session/challenge.go`, origin → process path → unattributable)
is refused with `TOO_MANY_REQUESTS` and raises no second prompt.

**Do not implement this by returning the existing `challengeId` to the second
caller.** Two different processes can share a `callerKey` — every caller with
neither an origin nor a resolvable process shares the unattributable bucket — so
handing back an existing id would let caller B solve a challenge the user
approved for caller A. Refusing is safe; sharing an id is a privilege
escalation.

### 7.2 Denials are remembered for the app run

After `allow=false`, further requests from that `callerKey` fail with
`TOO_MANY_REQUESTS` and raise no prompt, until the process restarts. Without
this, Deny is a button the user presses forever.

**Exempt the unattributable bucket** for the same reason as §7.1: one denied
native client must not block every other native client. Deny-memory applies only
when the key is an origin or a process path.

### 7.3 TTL

`pending` entries expire after **60s**; `approved` entries after **5 min**. On
expiry: delete, broadcast `LinkChallengeHide{challengeId}`. A sweep on each
`StartNewChallenge`/`SolveChallenge` call is sufficient — no timer goroutine.
Today entries are removed only on a successful solve and otherwise live for the
process lifetime.

### 7.4 Existing budgets stay

`maxChallengesRequests` (30/run), `maxChallengesRequestsPerCaller` (10/caller),
`challengeMaxTries` (5) and `maxFailedChallengeSolves` (20/run) all remain, as a
backstop against prompt churn rather than as the security boundary.

### 7.5 A pending solve must not burn the failure budget

`SolveChallenge` against a `pending` challenge returns
`CHALLENGE_NOT_APPROVED` and **must not** increment `failedChallengeSolves`. It
is not a wrong guess — there is nothing to guess — and counting it would let a
caller lock pairing for everyone by solving its own unapproved challenge 20
times.

## 8. Compatibility

### 8.1 Headless and self-hosted

anytype-heart in Docker with the gRPC server exposed has no UI to approve a
prompt, so it cannot use the challenge flow at all. This is fine: it was never
the right path for headless. `AccountLocalLinkCreateApp` mints an app key
directly, with no challenge and no prompt, and is the supported way to obtain a
key without a human at a screen. No approval escape hatch is added — one would
be a standing bypass of the protection this spec exists to add, guarding a case
that already has a clean answer.

### 8.2 Old desktop clients — release coordination, no fallback

Every challenge now requires approval; there is no path that mints a code
without one. A desktop build that does not know `ApproveChallenge` therefore
cannot pair at all — it will show a prompt it cannot answer.

A capability flag on `Initial.SetParameters` with a mint-immediately fallback
was considered and rejected: it would leave the pre-approval flow reachable by
any client that simply declines to set the flag, which is the whole attack it
exists to stop. The cost is that heart and desktop must ship together.

### 8.3 Mobile

`grpcprocess_mobile.go` is a no-op by design, so `processPath` is always empty
there. The prompt must read sensibly with only `origin` and `name`.

### 8.4 signatureVerified

`ClientInfo.signatureVerified` is declared in the proto but written by nothing
in the codebase — it is always `false`. Do not render it as a trust signal in
the new prompt without implementing it first.

## 9. Implementation checklist

| # | Change | Files |
| --- | --- | --- |
| 1 | Proto: drop `challenge` from both events, `needApprove`, `LinkChallengeHide.clientInfo`, `ApproveChallenge`, `CHALLENGE_NOT_APPROVED` | `pb/protos/events.proto`, `pb/protos/commands.proto`, `pb/protos/service/service.proto` |
| 2 | `challenge` gains `state`/`stateSince`; `StartNewChallenge` stops minting and returns only an id; new `ApproveChallenge`; `SolveChallenge` gates on state; `SweepExpired`; pending/deny bookkeeping; injectable clock | `core/session/challenge.go`, `core/session/service.go` |
| 3 | `LinkLocalApproveChallenge`; broadcast `needApprove`; hide on deny/expiry/solve | `core/application/sessions.go` |
| 4 | RPC handler, error mapping, browser-origin rejection | `core/account.go` |
| 5 | Leave both auth maps untouched; add the regression test | `core/auth.go`, `core/auth_test.go` |
| 6 | Regenerate `pb/*.pb.go`, `pb/service`, `clientlibrary/service`, `docs/proto.md` | `make protos` |

No changes to `core/api/*`: the JSON API keeps calling
`AccountLocalLinkNewChallenge` and `AccountLocalLinkSolveChallenge` exactly as
it does today.

## 10. Test plan

Security:

- `ApproveChallenge` is absent from `noAuthMethods` **and**
  `limitedScopeMethods` — assert on the maps directly, so a future edit fails
  the build rather than the threat model.
- `Limited` and `JsonAPI` scoped tokens are refused; `Full` is admitted.
- A call carrying an `Origin` header is refused even with a `Full` token.

Behaviour:

- `SolveChallenge` on a `pending` challenge fails with
  `CHALLENGE_NOT_APPROVED` for every one of the 10^4 possible answers —
  the concrete form of "there is nothing to brute-force".
- A pending solve does not increment `failedChallengeSolves`.
- The `LinkChallenge` event carries no `challenge` value while
  `needApprove` is true — assert on the broadcast payload, not just the API
  response.
- `allow=false` deletes the challenge, broadcasts hide, and suppresses the next
  request from that caller; the unattributable bucket is *not* suppressed.
- A second request from a caller with one pending is refused and does **not**
  return the first challenge's id.
- A caller asking again supersedes its own approved-but-unsolved challenge, so
  the old code dies rather than lingering as a second way in.
- Pending expires after 60s; approved after 5 min (inject a clock).
- A denial survives an unrelated successful pairing: it is the user's decision,
  not a rate limit that a success resets.

Regression: the whole existing `core/session` suite, plus
`TestRouter_ChallengeCarriesOrigin` and the per-caller budget tests added for
GO-7394.

## 11. Open decision

Once a human has approved a named, id-addressed request, the 4-digit code's
remaining job is proving the person at the keyboard is the one driving the
requesting app. `ApproveChallenge` could return the app key directly and drop a
step.

Recommendation: **keep the code.** Dropping it is the one change that would
break the client contract, which is the most valuable property of this design —
every existing integration keeps working without a line of change.
