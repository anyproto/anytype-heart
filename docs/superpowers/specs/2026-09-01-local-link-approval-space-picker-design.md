# Local-link approval with a space picker

2026-09-01. GO-7395. Merges the approve-then-mint pairing flow into the API-key
grant work, and adds the consent surface the grant was always missing: the user
chooses which spaces a pairing key may touch, or grants all of them.

Status: accepted, review findings folded in; implementing on
`go-7395-link-approval-picker`. Supersedes branch `go-7395-link-approval` /
PR #3225, which is abandoned in favour of this — the same work, landed where
the grant model lives.

## 1. Two half-designs that fit together

**GO-7395** (`go-7395-link-approval`, one commit) reworked pairing so the 4-digit
code is minted only after a human approves. `StartNewChallenge` registers a
pending request and mints nothing; a new `LinkApprovalRequest` event names the
caller and carries no code; `AccountLocalLinkApproveChallenge` is the only place
a code is minted and returns it to the approving session alone. Denials are
remembered for the app run, pending prompts expire, and a solve against a pending
challenge does not burn the failure budget. Full detail:
`docs/LocalLinkPairingApproval.md`.

**GO-7383 / P1 of the API-key scoping spec** (`go-7383-apiv2-clean`, this branch)
built the other half: `AppLinkGrant{Spaces, Perms}` sealed into the app-link
file, `ApiGrant` carried on the request context, `ensureSpaceGrant` enforcing it
across a classified route table, and `GET /v2/auth/whoami` to introspect it. See
`docs/superpowers/specs/2026-08-06-api-key-scoping-design.md`.

Neither is complete alone. GO-7395 asks the user "do you allow this app?" with no
way to answer "to what". The grant has issuance paths (`CreateApp`, `UpdateApp`)
that only a Full-scope caller can drive — no consent surface at all. The scoping
spec named the gap in its own non-goals ("Consent UI — desktop, rides the
challenge rework"), and the proto comment on `LinkChallenge.requestedGrant` still
reads *"the consent picker displays it and may narrow it further"*.

This spec is that picker.

### Goals

- Land GO-7395 on `go-7383-apiv2-clean`, unchanged in substance.
- The approval prompt collects a **space grant**, not just a yes/no.
- The user can grant **all spaces**, including spaces created later.
- No pairing path can mint a key without an explicit human grant decision.

### Non-goals

- Per-space permission levels (the wire shape reserves room; the grant stays
  uniform — scoping spec P2).
- Type grants, resource classes (P2).
- Changing the external app's two calls. `POST /v1/auth/challenges` and
  `/v1/auth/api_keys` keep their request and response shapes.
- Re-consent on widening an existing key's grant. `UpdateApp` remains a
  Full-scope call and the desktop's contract, not heart's.

## 2. The port

`go-7383-apiv2-clean` descends from `0b7abd765`, the direct parent of GO-7395's
commit `c57232336`, so the port is a single `git cherry-pick c57232336`.

Expected conflicts, all semantic and small:

| File | Conflict |
| --- | --- |
| `core/session/challenge.go` | GO-7395's state machine vs. this branch's `requestedGrant` plumbing and reordered budget checks |
| `core/session/service.go` | challenge map entry shape |
| `core/application/sessions.go` | `LinkLocalStartNewChallenge` / `LinkLocalSolveChallenge` signatures |
| `pb/protos/events.proto` | event rename vs. `requestedGrant` on `LinkChallenge` |
| `pb/protos/commands.proto` | `ApproveChallenge` vs. `NewChallenge.requestedGrant` |

Resolve the source files only; regenerate `pb/*.pb.go`, `pb/service`,
`clientlibrary/service` and `docs/proto.md` with `make protos` rather than
merging generated code.

`docs/LocalLinkPairingApproval.md` and `docs/LocalLinkApprovalDesktopGuide.md`
travel with the cherry-pick and are updated by §3–§8 below; they remain the
living contract, this file remains the design record.

Afterwards: close PR #3225 pointing here, and delete `go-7395-link-approval`.

> **Status: done.** The port landed as `20b42cee9` on
> `go-7395-link-approval-picker`; the conflicts were exactly the table above.

## 3. What the app may request

An external app **cannot know a space id**. Ids are opaque local identifiers with
no discovery path before pairing — the app has no key yet, so it cannot call
`GET /v2/spaces`. `requestedGrant` on the challenge path is therefore
unimplementable by the clients it exists for.

It is also, today, actively broken: `LinkLocalStartNewChallenge` validates the
requested grant with `ValidateAppLinkGrant`, which rejects an empty space list.
An app that wants to request read-only access must send space ids it cannot
obtain, or send nothing. There is no third option, and no client has ever used
the field.

**Decision: `NewChallenge.Request.requestedGrant` → `requestedPerm`**, carrying
only `model.Account.Auth.AppGrant.Perm`. The app declares what it can actually
know — whether it needs to write — and asks the user for what it cannot. The
event forwards it so the prompt can read "Claude Desktop wants read & write
access", rendered as an app-supplied claim beside the untrusted `name`, never as
a fact.

`requestedGrant` **stays valid on `CreateApp`**, whose caller is Full-scope
(desktop, `anytype-cli`) and does know space ids. That path is unchanged.

The old field number is reserved, not reused. Nothing has shipped: this branch is
unpushed and `/v2` has no released clients.

Two wire realities, accepted rather than fought:

- `Perm`'s zero value is `Read`, so "the app asked for read" and "the app asked
  for nothing" are the same bytes on the wire. Harmless by construction — both
  render the picker's permission control at its `read` default — but it means
  the prompt cannot phrase `read` as an app-supplied claim: for `read`, a claim
  is indistinguishable from silence. Only a `readwrite` request is renderable
  as "X asks for write access".
- `requestedPerm` has a gRPC carrier only. `POST /v1/auth/challenges` carries
  only the app name (`core/api/service/auth.go`) and its shape is frozen (a
  non-goal above), so every HTTP-paired app gets the `read` default; only
  native gRPC callers (`anytype-cli`) can pre-fill `readwrite`.

## 4. What the user decides

`ApproveChallenge.Request` gains the grant:

```proto
message ApproveChallenge {
    message Request {
        // both verbatim from the ClientInfo of the LinkApprovalRequest
        // event being answered; either may be empty
        string processPath = 1;
        string origin = 2;
        bool allow = 3;
        // the user's decision; required when allow is true and the
        // challenge scope is JsonAPI, forbidden otherwise
        model.Account.Auth.AppGrant grant = 4;
    }
    message Response {
        Error error = 1;
        string challenge = 2;   // the 4-digit code; empty when allow=false
        // ... error codes unchanged (BAD_INPUT covers an invalid grant)
    }
}
```

Rules:

- **Heart persists exactly what the approval sends.** `requestedPerm` only
  pre-fills the picker's permission control; it is not a ceiling. The human
  minting the credential is the authority, and the app's request is untrusted
  input.
- **Nothing is pre-selected.** No space is checked by default and "all spaces" is
  not the default. The user makes a positive choice.
- **The permission control defaults to `read`** when the app sent no
  `requestedPerm` — the safe half of the only binary choice, and the one an app
  that never asked has no claim on.
- **`allow=true` with neither `spaceIds` nor `allSpaces` → `BAD_INPUT`.** A key
  with access to nothing is a dead key that will read to its holder as a heart
  bug. The client keeps Allow disabled until a selection exists.
- **`Limited` scope carries no grant.** Grants are JsonAPI-only
  (`ValidateAppLinkGrant`); the webclipper prompt is a plain Allow/Deny. A grant
  on a `Limited` challenge is `BAD_INPUT`.
- **`allow=false` ignores the grant** entirely.

### Where the rules run

The scope-dependent rules live in `session.ApproveChallenge`, **before any
state change**. The application layer cannot host them — it does not know which
scope the pending challenge carries until the session service has looked it up
— and both answer paths mutate state (deny deletes the challenge, allow mints
the code), so a rejected grant must leave the challenge pending and the prompt
answerable. An invalid grant returns an error wrapped in
`wallet.ErrInvalidGrant` (no import cycle: `core/wallet` does not import
`core/session`), and `AccountLocalLinkApproveChallenge`'s error mapping in
`core/account.go` gains the `ErrInvalidGrant` / `ErrBadInput` → `BAD_INPUT`
lines its siblings already have. "Error codes unchanged" holds at the enum
level; the mapping lines are new.

### Pending TTL: 60s → 180s

GO-7395 set the pending TTL at 60s for a one-click decision. Approval is now a
real interaction — read the caller, open a space list, select, confirm — and 60s
is not enough for a user who has to think. The pending state holds no secret, so
a longer window costs nothing but a stale prompt; the approved TTL stays 5
minutes, measured from approval, and is unaffected by how long the picking took.

The session tests use the constant symbolically and survive the change; the
hard-coded "60s" appears twice in `docs/LocalLinkApprovalDesktopGuide.md` and
twice in `docs/LocalLinkPairingApproval.md` — §10 step 7 carries those edits.

## 5. `allSpaces`

Semantics: **dynamic**. A grant with `allSpaces` covers every space in the
account, including spaces created after the approval. This follows GitHub's
fine-grained PATs, whose "All repositories" explicitly covers current and future
repositories, and it is what the phrase means to the person reading the prompt.
The alternative — snapshotting the current list — produces a user who granted
"all spaces", creates a space, and finds their agent silently blind to it.

```go
type AppLinkGrant struct {
	Version   int      `json:"v"`
	AllSpaces bool     `json:"allSpaces,omitempty"`
	Spaces    []string `json:"spaces"`
	Perms     string   `json:"perms"`
}
```

`ValidateAppLinkGrant` requires **exactly one** of `AllSpaces` and a non-empty
`Spaces`: both set is ambiguous and rejected, neither set is the existing
empty-spaces rejection, kept verbatim. The invariant that an empty `Spaces` list
denies every space — "the loop's vacuous false is load-bearing" in
`ApiGrant.AllowsSpace` — is untouched. `allSpaces` is a separate explicit flag
precisely so that "all" can never be spelled as "empty".

Proto: `model.Account.Auth.AppGrant` gains `bool allSpaces`. `UpdateApp` accepts
it like any other grant shape.

### Grant schema version 1 → 2

`appLinkGrantVersion` bumps. Its own comment prescribes the rule and this case
sits just outside the letter of it: the rule requires a bump for any *narrowing*
dimension, because an older binary drops unknown JSON fields and would enforce
the grant wider than written. `allSpaces` widens, so an older binary reading it
sees `{spaces: [], perms: …}` and fails **closed** — but it fails with
`spaces must be non-empty`, which reads as a corrupt key rather than an old
binary. Bumping the version turns that into `unknown grant version 2`:
diagnosable, and the same fail-closed outcome.

The bump is near-free: this branch is unpushed with no PR, so no v1 grant exists
outside a local build, and the binary can accept exactly one version rather than
a set. A test key minted from a local build of this branch stops working and must
be re-paired — worth knowing before the first `make protos`, not worth a
compatibility path.

The **envelope** version stays 2 — the file layout does not change, and the
grant-version gate already refuses old readers.

One side effect worth naming: a pre-bump granted key does not just stop
authenticating — `list` degrades its entry to a hash-only row (the stored
grant no longer validates), so it loses its name in ListApps too. Re-pairing
replaces it.

### The full plumbing

`AllSpaces` rides every conversion or it silently narrows (the fail-closed
direction — a dropped flag denies everything rather than widening; still a
bug): `wallet.AppLinkGrant.Proto()` and `AppLinkGrantFromProto` (ListApps and
the `WalletCreateSession` response are the transport), `util.ApiGrantFromProto`
(the HTTP layer's ctx grant), and `util.ApiGrant.Describe()` — a 403 that
renders an all-spaces grant as `spaces []` reads as a corrupt key.
`ApiSessionEntry` is an in-memory cache (nothing marshals it), so the new field
has no persistence implications.

### /v1 and the unrestricted grant

`ensureUngrantedKey` refuses granted keys on the whole /v1 group because /v1
cannot enforce a grant. One grant needs no enforcement: **`allSpaces` with
`readwrite` grants no less than an unscoped key**, so /v1 honors it by doing
nothing. The gate therefore admits exactly that combination —
`ApiGrant.IsUnrestricted()`, fail-closed in the `CanWrite` style: only
`AllSpaces && Perms == readwrite` passes; every narrower grant (specific
spaces, or read-only) keeps today's refusal. Extending real grant enforcement
to /v1 is a **separate change, out of scope here**.

This is a design rule, not a workaround. Without it the picker would make
every newly paired key /v2-only — pairing now mints only granted keys, and
every shipped client speaks /v1.

The one asymmetry, accepted and recorded rather than discovered later: on /v1
an unrestricted key behaves exactly as an unscoped key does today, INCLUDING
whatever tech-space access /v1 allows — while the same grant on /v2 excludes
the tech space (§6). The user chose all-spaces-readwrite, the maximal consent.
The flip side belongs in the desktop guide, said plainly so the client can
warn at pick time: a user who narrows the grant — specific spaces, or
read-only — gets a key that 403s on every /v1 route.

## 6. The tech space is not "all spaces"

`ensureSpaceGrant` runs before the service's `ensureSpace`, which deliberately
admits the tech space as an ordinary space id; the gate therefore denies the tech
space unless it is explicitly listed. `allSpaces` must not quietly undo that: the
tech space holds account-level machinery (space views, profile), not user
content, and writes to it can break the account. It is never offered by the
picker and never covered by `allSpaces`; reaching it still requires listing it
explicitly, which only a Full-scope `CreateApp`/`UpdateApp` caller can do.

Implementation: `ApiGrant.AllowsSpace` stays a pure function of the grant and
returns true under `AllSpaces`. The exclusion lives in one shared helper in
`core/api/util`, consulted by both enforcement points so a future caller
cannot get the pair half-right:

- **the route gate**: `ensureSpaceGrant` takes the tech space id at
  construction (a `RouteDeps` field; the id is resolved where the server
  builds both the service and the deps, `core/api/server/server.go`).
- **the service backstop**: `ensureSpaceGranted` — NOT `ensureSpace`. It
  becomes a method (it needs `s.techSpaceId`), and it is the right spot
  because it has five callers: `ensureSpace`, `ensureSpaceWrite`,
  `ensureChatWrite`, and `GetSpace` / `UpdateSpace`, which deliberately
  bypass `ensureSpace` (their read IS the space-view lookup). Exclusion
  parked in `ensureSpace` would leave those two passing the grant check and
  failing only by the accident that the tech space has no space view.

The refusal for the tech space under `allSpaces` says so — the tech space is
never covered by an all-spaces grant and must be granted explicitly — rather
than the generic not-granted text, which would read as a heart bug to a user
who just granted "all spaces".

The third and last `AllowsSpace` caller is `liveSpaceRows`
(`core/api/v2/service/discovery.go`) — the one enumeration feeding
`GET /v2/spaces`, whoami's names, the global-search fan-out and the §8.35
short-reference census. It needs no exclusion: its source is the tech space's
space views, and the tech space has no space view of itself (which is why
`ensureSpace` special-cases it before `GetSpaceViewDetails`). Under
`allSpaces` the filter admits every enumerated row — every live user space,
never the tech space.

## 7. The grant is never an input on the solve path

`SolveChallenge` is the call the **external app** makes. It must never accept,
read, or merge a grant from its request: the grant is fixed at approval, stored
in the challenge record beside the code, and read out at solve.

This is the same shape as the code itself — minted at approve, never influenced
by the solver — and it is the property that makes the picker meaningful. Without
it, an app could pair with the user's narrow grant and then hand itself a wider
one. It gets an explicit test (§9), not just a comment.

## 8. `whoami` and `ListApps`

Under `allSpaces`, `whoami` returns `all_spaces: true` **and** enumerates the
current spaces in `spaces[]` (snake_case on the REST body — C2 is one
vocabulary, and the conformance test enforces it):

```json
"grant": { "scoped": true, "all_spaces": true, "permission": "readwrite",
           "spaces": [ {"id": "…", "name": "Work", "permission": "readwrite"} ] }
```

`all_spaces` is the boundary field; `spaces[]` is informational and is what lets
an agent map "put this in Work" to an id. Enumerating is not new disclosure — it
is the same grant-intersected path `GET /v2/spaces` uses, and under `allSpaces`
that is every space. The scoping spec's warning applies unchanged: no consumer
may infer the boundary from the emptiness of `spaces[]`.

`ListApps` returns the grant as-is, so the desktop key list can badge an
all-spaces key distinctly from a narrow one.

Implementation: `Whoami` loops over `grant.Spaces` and
`resolveGrantedSpaceNames` short-circuits on an empty list — under `allSpaces`
the spaces come from `liveSpaceRows` instead: the same grant-intersected
enumeration, which under `allSpaces` is every live user space and never the
tech space (§6). `v2model.WhoamiGrant` gains the always-present `allSpaces`
boolean, and the hand-maintained contract files `core/api/docs/v2/openapi.yaml`
/ `.json` gain the field beside it.

## 9. Test plan

Carried over from GO-7395 unchanged: the auth-map absence assertions, the
`Origin`-bearing caller refusal, "no code exists while pending" across all 10^4
answers, pending solves not burning the failure budget, deny memory and its
unattributable-bucket exemption, one-pending-per-caller without id sharing, TTL
expiry.

New:

- **Grant round-trip**: approve with `spaceIds`, solve, read the app link back —
  the persisted grant is exactly what the approval sent.
- **`allSpaces` round-trip** through the sealed envelope; a v1-only grant reader
  refuses a v2 grant (simulated).
- **Validation matrix**: `allSpaces` + non-empty `spaces` → rejected; neither →
  rejected; `allSpaces` on a `Limited` challenge → `BAD_INPUT`; `allow=true` with
  an empty grant → `BAD_INPUT`.
- **The solve path cannot influence the grant**: the persisted grant equals the
  approved one for every solve, and `SolveChallenge` reads the grant only from
  the stored challenge record — no parameter, no merge, no default.
- **`allSpaces` covers a space created after approval** — the dynamic semantics,
  as a behavioural test, not a comment.
- **The tech space is denied under `allSpaces`** at the gate and at the
  `ensureSpaceGranted` backstop; still reachable when explicitly listed.
- **`IsUnrestricted` matrix**: only `allSpaces` + `readwrite` passes /v1;
  `allSpaces` + `read` and narrow grants keep refusing; a nil grant stays the
  legacy pass-through.
- **`requestedPerm` never becomes a ceiling**: app requests `read`, user approves
  `readwrite`, the key can write.
- **`whoami` under `allSpaces`**: `allSpaces` true, `spaces[]` non-empty, and the
  body derived from the same grant record the gate reads.

Rewritten or removed — the change deletes their subjects:

- `TestSolveChallenge_CarriesRequestedGrant` (`core/session`): the record no
  longer carries a requested grant; replaced by the approved-grant round-trip.
- `TestChallengeFlowGrant` (`core/application`): "solve persists the requested
  grant" becomes "solve persists the approved grant"; "challenge without grant
  persists an unscoped key" is now impossible for JsonAPI and flips into the
  `BAD_INPUT` assertion; the two requested-grant refusal subtests move to the
  approve path.
- The `Version: 1` literals in `core/wallet/applink_test.go`'s validation
  table follow the bump; `readV1OnlyEnvelopeVersion` stays as-is (it simulates
  envelope-v1-only readers — a different axis from the grant schema).
- `TestWhoami` / `TestWhoamiAgreesWithTheGate` (`core/api/server`) gain
  `allSpaces` cases.
- /v1's granted-key refusal tests split: an unrestricted grant passes, every
  narrower grant keeps refusing.

## 10. Implementation order

1. Cherry-pick `c57232336`; resolve; `make protos`; whole suite green. No
   behaviour change beyond GO-7395 as designed. **Done: `20b42cee9`.**
2. Wallet: `AllSpaces`, validation, version bump, envelope tests.
3. Proto: `AppGrant.allSpaces`, `NewChallenge.requestedPerm`,
   `ApproveChallenge.grant`; `make protos`.
4. Session: the challenge record carries the approved grant; `ApproveChallenge`
   validates and stores it; `SolveChallenge` returns it; `sessions.go` persists
   it. Pending TTL to 180s.
5. Enforcement: `ApiGrant.AllSpaces` + `IsUnrestricted` + `Describe`, the
   shared tech-space helper in the gate (`RouteDeps` carries the id) and the
   `ensureSpaceGranted` backstop, the /v1 gate admitting unrestricted grants,
   and the remaining conversions (§5 "The full plumbing").
6. `whoami` + `ListApps` rendering, and the openapi contract files.
7. Docs: fold §3–§8 into `docs/LocalLinkPairingApproval.md`; rewrite the picker
   contract in `docs/LocalLinkApprovalDesktopGuide.md`, including the four
   hard-coded 60s mentions, the narrow-grant-/v1 warning, and retracting its
   "no client needs updating" claim.

Steps 2–6 land together: a build that collects a grant it does not enforce hands
out keys that look scoped and are not — the failure the scoping spec's §8 release
constraint exists to prevent.

## 11. Open items

- **Desktop copy** for the all-spaces option. It must say the dynamic part out
  loud ("all spaces, including ones you create later"); a prompt that says only
  "All spaces" understates what is being granted.
- **Whether `UpdateApp` widening to `allSpaces` needs re-consent** in the desktop
  UI. Out of scope here (heart applies what a Full caller sends), but it is the
  same question the scoping spec left with the client.
