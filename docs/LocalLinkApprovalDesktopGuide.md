# Desktop integration: local-link approval prompt

What the desktop client must implement so external apps can pair with the local
API. Companion to `LocalLinkPairingApproval.md` (the design); this is the
client-facing contract.

## What changed

Before, when an app requested a pairing code, heart minted a 4-digit code and
broadcast it to every session; the client just displayed it. Now **no code is
minted until the user approves**. The client has to show an approve/deny prompt
first, then ask heart for the code.

If the client does nothing, **pairing cannot complete** — there is no fallback
that hands out a code without approval.

## The flow

```
external app          heart                         desktop client
     │                                                    │
     ├─ POST /v1/auth/challenges ─▶ (broadcast) ─────────▶│  Event.Account.LinkApprovalRequest
     │  or AccountLocalLinkNewChallenge                    │  → show "X wants to connect" [Allow][Deny]
     │                                                     │
     │                            ◀── AccountLocalLinkApproveChallenge ──┤  user pressed Allow
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
| `scope` | the access level being requested (`JsonAPI` or `Limited`). |

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
)
```

Pass `processPath` and `origin` back **exactly** as they arrived in the event —
together they identify which pending request you are answering. Do not
normalize, lowercase, or trim them.

Response:

| outcome | response |
| --- | --- |
| `allow=true`, success | `challenge` = the 4-digit code. Display it for the user to type into the external app. |
| `allow=false` | `challenge` empty. The request is dropped. |
| `error.code = NO_PENDING_CHALLENGE` | nothing was pending for that caller: it expired (60s), was already answered, or never existed. Dismiss the prompt. |
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

## Edge cases

- **Timeouts.** An unanswered prompt expires after 60s; a displayed code expires
  5 minutes after approval. In both cases a `LinkApprovalHide` arrives — drive
  dismissal off that, not off your own timer.
- **One prompt per caller.** A caller with a prompt already open cannot open a
  second; repeat requests are refused by heart. No client-side dedup needed, but
  don't assume one request per app lifetime.
- **Mobile / no process info.** `processPath` is empty on mobile and for browser
  callers. Fall back to `origin`, then `name`. Always have a non-empty label.
- **Reconnect.** Pending prompts live in heart, not in the event log. A client
  that connects mid-flight won't receive the original `LinkApprovalRequest`;
  the caller must re-request. Don't try to reconstruct pending prompts from
  history.

## What has NOT changed

The external app's side is identical to before — `POST /v1/auth/challenges`
then `POST /v1/auth/api_keys` (or the gRPC equivalents), same requests, same
responses. Only the desktop client gains the approval step. No SDK or extension
needs updating.
