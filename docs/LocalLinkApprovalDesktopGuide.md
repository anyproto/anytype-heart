# Desktop integration: local-link approval prompt

What the desktop client must implement so external apps can pair with the local
API. Companion to `LocalLinkPairingApproval.md` (the design); this is the
client-facing contract.

## What changed

Before, when an app requested a pairing code, heart minted a 4-digit code and
broadcast it to every session; the client just displayed it. Now **no code is
minted until the user approves**, and for API keys (`JsonAPI` scope) the
approval is a **space picker**, not a yes/no: the user chooses which spaces
the key may touch — specific spaces, or all of them — and whether it may
write. Heart persists exactly what the approval sends.

If the client does nothing, **pairing cannot complete** — there is no fallback
that hands out a code without approval, and no pairing path that mints a key
without an explicit grant decision.

## The flow

API v2 clients use `POST /v2/auth/challenges` and `POST /v2/auth/api_keys`.
The `/v1/auth/*` paths shown below remain aliases of the same handlers, with
the same request and response bodies. Neither version requires an existing key
for pairing; Desktop approval is required before the code can be exchanged.

```
external app          heart                         desktop client
     │                                                    │
     ├─ POST /v1/auth/challenges ─▶ (broadcast) ─────────▶│  Event.Account.LinkApprovalRequest
     │  or AccountLocalLinkNewChallenge                    │  → show "X wants to connect" [Allow][Deny]
     │                                                     │
     │                            ◀── AccountLocalLinkApproveChallenge ──┤  user picked spaces +
     │                              (request: allow, grant)│  permission, pressed Allow
     │                              (response: challenge)  │  → show the 4-digit code
     │                                                     │
     │◀ user types the code into the external app          │
     │                                                     │
     ├─ POST /v1/auth/api_keys ──▶ mints app key           │  (broadcast) LinkApprovalHide
     │  or AccountLocalLinkSolveChallenge                  │  → dismiss the prompt/code UI
```

## 1. Listen for the request event

`Event.Account.LinkApprovalRequest` (oneof field `accountLinkApprovalRequest`):

| field | use |
| --- | --- |
| `clientInfo.name` | app-supplied label. **Untrusted** — the caller chose it. |
| `clientInfo.origin` | browser origin, e.g. `chrome-extension://<id>`. Empty for native callers. Set by the browser, not forgeable by a page. |
| `clientInfo.processName` / `processPath` | resolved OS process for native callers. Empty for browsers and on mobile. |
| `clientInfo.signatureVerified` | **always false today** — not implemented. Do not show a "verified" badge from it. |
| `scope` | the access level being requested (`JsonAPI` or `Limited`). `JsonAPI` prompts are the space picker; `Limited` (webclipper) prompts stay a plain Allow/Deny. |
| `requestedPerm` | the permission the app claims to need (`Read`/`ReadWrite`). Pre-fill the permission control with it, nothing more — it is app-supplied and never a ceiling. `Read` is the wire default, so "asked for read" and "asked for nothing" look identical; treat both as the read default. |

Show `origin` and/or `processPath` as the identity — those are attributable.
Treat `name` as a hint, not a fact; render it clearly as caller-supplied.

There is no code in this event and no `needApprove` flag. The event's arrival
**is** the request to approve.

## 2. Prompt the user, then call approve

On Allow or Deny, call the new RPC:

```
AccountLocalLinkApproveChallenge(
    processPath: <clientInfo.processPath, verbatim>,
    origin:      <clientInfo.origin, verbatim>,
    allow:       true | false,
    grant:       { spaceIds: [...] | allSpaces: true, perm: Read | ReadWrite },
)
```

Pass `processPath` and `origin` back **exactly** as they arrived in the event —
together they identify which pending request you are answering. Do not
normalize, lowercase, or trim them.

The grant is the picker's result and is **required when `allow=true` on a
`JsonAPI` challenge**, forbidden on a `Limited` one, ignored on Deny. Picker
rules, all enforced by heart with `BAD_INPUT`:

- exactly one of a non-empty `spaceIds` and `allSpaces: true`;
- **nothing is pre-selected** — no space checked by default, "all spaces" not
  the default; keep Allow disabled until a selection exists;
- the permission control defaults to read when the app sent no
  `requestedPerm`;
- the all-spaces option must say the dynamic part out loud: **"all spaces,
  including ones you create later"** — a label that says only "All spaces"
  understates the grant;
- the tech space is never offered: it is not a user space, `allSpaces` never
  covers it, and only Full-scope `CreateApp`/`UpdateApp` can grant it.

**Warn on narrow grants.** A key granted specific spaces, or read-only, works
on `/v2` only — every `/v1` route refuses it with
`v1_not_available_for_scoped_keys`. Only the maximal choice (all spaces +
read & write) is also served on `/v1`, where it behaves exactly like a legacy
unscoped key. Say this at pick time: a user pairing an app that speaks `/v1`
and narrowing the grant gets a key that appears broken.

Response:

| outcome | response |
| --- | --- |
| `allow=true`, success | `challenge` = the 4-digit code. Display it for the user to type into the external app. |
| `allow=false` | `challenge` empty. The request is dropped. |
| `error.code = BAD_INPUT` | the grant violated a rule above. The challenge is **still pending** — fix the picker result and call again; the prompt stays answerable. |
| `error.code = NO_PENDING_CHALLENGE` | nothing was pending for that caller: it expired (180s), was already answered, or never existed. Dismiss the prompt. |
| `error.code = BAD_INPUT` | the grant is unusable: absent on a JsonAPI approval, present on a `Limited` one, or neither `spaceIds` nor `allSpaces` set. Keep Allow disabled until the user has chosen. |
| `error.code = ACCOUNT_IS_NOT_RUNNING` | no account loaded. |

This RPC requires a **full-scope** session — the desktop client's own. It is
rejected for JsonAPI/Limited tokens and for any caller sending an `Origin`
header, so it can only be driven from the app itself, never from a paired app or
a browser.

## 3. Hide the prompt on `LinkApprovalHide`

`Event.Account.LinkApprovalHide` (field `accountLinkApprovalHide`) fires when a
prompt should come down: the user solved it, denied it, or it expired. Match on
`clientInfo` (same identity as the request) and dismiss the corresponding UI —
both the pending prompt and a displayed code.

Deny is remembered for the app run: a denied caller is refused silently and
raises no new prompt until restart. You do not need to track this yourself.

**Approving does NOT broadcast a hide**, and that is deliberate: this event
means "take down the prompt and any code shown for it", and at the moment of
approval the approving session is displaying the code it just received in the
response. It dismisses its own prompt locally and keeps showing the code.

The consequence, if you run more than one window: a second window keeps a live
Allow/Deny for a request that has already been answered, and pressing Deny
there returns `NO_PENDING_CHALLENGE` rather than revoking anything. Closing
that needs either an "answered" event distinct from this one, or a broadcast
that skips the approving session — raise it if multi-window matters to you and
we will add the event rather than have you work around it.

A repeat request from the same caller DOES broadcast a hide before the new
`LinkApprovalRequest` arrives: asking again destroys that caller's previous
prompt, including a code it had already been granted, which stops working at
that moment. Dismiss on the hide, then render the new request.

## What the requesting app sees

Relevant if you also maintain a pairing client, and to know what is NOT a bug:

| outcome | code on `SolveChallenge` / `NewChallenge` |
| --- | --- |
| the user has not answered the prompt yet | `CHALLENGE_NOT_APPROVED` — wait and retry; it is not a wrong code |
| this caller already has a prompt on screen | `TOO_MANY_REQUESTS` — answer the open one, do not ask again |
| the user denied this caller earlier this run | `TOO_MANY_REQUESTS` — it will not prompt again until heart restarts |

All three answered `UNKNOWN_ERROR` before, so a client could only retry blindly
— which burned the request budget and re-prompted the user.

## Edge cases

- **Timeouts.** An unanswered prompt expires after 180s — enough to read the
  caller, open the space list, and think; a displayed code expires 5 minutes
  after approval, however long the picking took. In both cases a
  `LinkApprovalHide` arrives — drive dismissal off that, not off your own
  timer.
- **One prompt per caller.** A caller with a prompt already open cannot open a
  second; repeat requests are refused by heart. No client-side dedup needed, but
  don't assume one request per app lifetime.
- **Mobile / no process info.** `processPath` is empty on mobile and for browser
  callers. Fall back to `origin`, then `name`. Always have a non-empty label.
- **Reconnect.** Pending prompts live in heart, not in the event log. A client
  that connects mid-flight won't receive the original `LinkApprovalRequest`;
  the caller must re-request. Don't try to reconstruct pending prompts from
  history.

## What has NOT changed — and what has

The external app's pairing calls are byte-identical to before —
`POST /v1/auth/challenges` then `POST /v1/auth/api_keys` (or the gRPC
equivalents), same requests, same responses. The same endpoints are also
available under `/v2/auth/*`, so v2 clients can complete pairing within `/v2`.

What HAS changed is the key that comes out: it carries the grant the user
picked. An app whose key was granted all spaces with read & write works
everywhere, `/v1` included, exactly as an unscoped key did. Any narrower key
works on `/v2` only and is refused on every `/v1` route. An SDK or extension
that speaks `/v1` therefore keeps working only when the user grants
everything — which is why the picker must warn on narrow grants (see above).
