# API key scoping — P0 + P1 design

2026-08-06. Decisions: v2-only enforcement, spaces × read/readwrite,
heart-first/decoupled consent, type-writes deferred to P2, P0 fixes first.
The as-built API behavior is recorded in `core/api/APIV2.md` §8.9–8.11.

## Goals

- Close the pre-existing scope-enforcement holes (P0) so scoping is real.
- A key can be granted `{spaces × read|readwrite}`; enforcement on `/v2`;
  scoped keys rejected on `/v1`. Grants issuable and editable via the gRPC
  app-link surface today (CLI / power users), via the desktop picker when the
  reworked challenge flow lands.
- `GET /v2/auth/whoami` so any holder (and `anytype-mcp`) can introspect its
  grant.

## Non-goals

Consent UI (desktop, rides the challenge rework); type grants (P2 — the
schema reserves the field); resource classes; read-side type scoping
(declined); gateway auth (separate ticket); bot-account productization;
a committed sunset date for legacy keys (product call, see P0.6).

## Migration stance (decided 2026-08-06)

No existing key breaks. Two levers, composed:

1. **The scope gate is `/v2`-only.** `/v2` has no shipped clients, so
   gating there closes H2 on the surface that matters and changes `/v1`
   behavior not at all. Legacy keys keep working on `/v1` exactly as they
   ship today.
2. **New keys are a new kind.** From the flip, JSON-API keys are minted in
   a recognizable prefixed+checksummed format; the old format can never be
   minted again. Whether a key carries a grant is **independent of the
   format** — a new-format key minted through a pairing flow that requested
   no grant is unscoped (§7: the grant-less challenge flow behaves exactly
   as today), and a legacy-format key can be granted in place. The legacy
   population shrinks by attrition and cannot be replenished, which leaves
   a later sunset as a scheduling decision rather than a surprise break.

**Enforcement keys off the grant, not the format.** The prefix is a
provenance and secret-scanning signal for humans and scanners; "is this key
scoped" is answered by `Grant != nil`. Consequence: a grant can be attached
to an existing legacy key in place, so a user can scope a key without the
holder ever learning a new secret (the Airtable affordance, research §3.1).

---

## P0 — hygiene fixes (first PRs, in this order)

**P0.1 — H1, zero-scope pairing session.** `core/session/challenge.go:105`:
`s.StartSession(signingKey, scope)` → `s.StartSession(signingKey,
challenge.scope)`. Test: solve a `JsonAPI` challenge, assert
`ValidateSessionToken` yields `JsonAPI` and the gRPC `Authorize` interceptor
denies `ObjectSearch` for that token (today it passes as `Limited`).

**P0.2 — H2, scope-blind HTTP gate.** Extend
`RpcWalletCreateSessionResponse` (additive proto) with `accountScope` and
`appName` (the latter closes the standing TODO at
`core/api/server/middleware.go:92`), and cache `Scope` in
`ApiSessionEntry`.

**The gate is `/v2`-only** (revised 2026-08-06 — the as-built P0 put it on
both groups): keys whose scope is neither `JsonAPI` nor `Full` are refused
with 403 on `/v2`, distinct from the 401 invalid-key path, and pass on
`/v1` as they do today. Mechanically the scope check moves out of
`ensureAuthenticated` (which both groups share — `router.go:46` and the
`Auth:` dep at `router.go:64`) into its own middleware installed only on
the v2 group. `ensureAuthenticated` keeps authentication, expiry, and the
context carriers for both.

Rationale: H2 is a consent violation (a key issued for the clipper's 13
gRPC methods also opens the JSON API), not a remote escalation — the holder
already has the key. Refusing it on `/v1` would break `anytype-cli`-issued
keys with no repair path, while refusing it on the unreleased `/v2` costs
nothing. The `anytype-cli` one-liner (`Scope: model.AccountAuth_JsonAPI`)
stays worth shipping, but stops being release-blocking.

Deprecation signals for legacy keys are **P1**, not P0: they need a new
kind to migrate *to*, and in P0 every key is legacy, so the signal would
fire on every request and mean nothing.

**P0.3 — H4, revocation completeness.** `core/application/sessions.go`
tracks the **full token set** per app hash (today: one token, overwritten),
including tokens derived via `WalletCreateSession(token:)` — a reverse
`token → appHash` index populated on both mint paths. `LinkLocalRevokeApp`
closes all of them; `core/api/server.RevokeToken` drops its first-match
`return` and evicts every matching cache entry.

**P0.4 — H5, dead `ExpireAt`.** `wallet.ReadAppLink` returns
`ErrAppLinkExpired` when `ExpireAt > 0 && now > ExpireAt` (→ 401 at HTTP,
distinct error text). `CreateApp` may set it; the challenge path keeps 0
(no default lifetime until the picker exists — product call later).

**P0.5 — H3, decided.** `LinkLocalCreateApp` rejects `Scope=Full` with
`ErrInvalidScope`, mirroring the challenge guard (`challenge.go:49-54`).
Pre-merge check: confirm no client currently calls `CreateApp` with `Full`.

Each lands as its own small PR with tests; none depends on P1.

---

## P1 — the grant

### 1. Model

```go
// core/wallet/applink.go
type AppLinkInfo struct {
    // ... existing fields unchanged ...
    Grant *AppLinkGrant `json:"grant,omitempty"`
}

type AppLinkGrant struct {
    Version int      `json:"v"`      // 1
    Spaces  []string `json:"spaces"` // space ids; must be non-empty
    Perms   string   `json:"perms"`  // "read" | "readwrite"
    // P2 reserves: Types map[string][]string — spaceId → ot-… uniqueKeys
}
```

`Grant == nil` ⇒ legacy unscoped key, behavior unchanged. A grant with
empty `Spaces` or an unknown `Perms` value is rejected at persist time.
Grants are valid **only on `JsonAPI`-scope keys** — persist rejects a
non-nil grant on `Limited`/`Full` (research §4 "no override bit": `Full`
stays a separate kind, never a modifier inside scoped semantics). A grant
may be attached to **any** `JsonAPI` key, legacy format included: that is
the in-place upgrade path (migration stance above).

### 1b. Key format — the new kind

New JSON-API keys are minted as `anytype_<body>_<crc32 as 8 lowercase hex>`,
following the GitHub 2021 token redesign (`github.blog`, 2021-04-05) and
npm's adoption of it.

**The body alphabet must be `[0-9A-Za-z]` only — NOT standard base64.**
This is the one hard constraint. `+`, `/` and `=` break `\b`-anchored
gitleaks/TruffleHog rules mid-token (defeating the entire point of the
prefix), are mangled into spaces by `application/x-www-form-urlencoded`
parsing when an agent puts the key in a query string, break
double-click-to-select in terminals and chat clients, and need shell/JSON
escaping in exactly the MCP and CLI config files these keys live in.
GitHub chose base62 plus an underscore separator for precisely these
reasons. Base62 over the 32 raw bytes is 43 chars; if avoiding a bignum
codec is preferred, lowercase unpadded base32
(`base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)`)
is stdlib, streaming, 52 chars, and equally `\b`-safe. Either is fine;
standard base64 is not.

Further constraints, each with a concrete precedent:

- **CRC32 input is `prefix + "_" + body`**, not the body alone, so a
  mangled prefix is caught too. Free.
- **Keep the explicit `_` before the checksum** (rather than GitHub's
  positional last-6) and **publish a length RANGE, never a fixed length**:
  `\banytype_[0-9A-Za-z]{40,60}_[0-9a-f]{8}\b`. GitHub spent a 2026
  changelog cycle telling integrators to stop assuming `ghs_` is 40 chars
  after installation tokens became ~520-char JWTs; GitLab's inline-versioned
  `glpat-` tokens silently defeat third-party regexes today.
- **Version by minting a new prefix namespace**, never an inline version
  field (`ghp_` → `github_pat_` is the precedent; GitLab's `.01.` segment is
  the anti-pattern). Accept every historical format forever on the read
  path; emit exactly one on the write path.
- **Encode nothing but issuer identity** — no space ids, no read/readwrite
  marker, no expiry. Anything in the string is an unauthenticated claim the
  server must ignore anyway; on a privacy-first product a leaked key
  fragment would otherwise disclose which spaces exist and what was granted;
  and it would weld the key string to the grant record, so editing a grant
  would force a re-mint. (AWS's undocumented account id inside `AKIA…` was
  recovered by third parties and can never be un-shipped.)
- **The checksum is not an authenticator.** CRC32 over public bytes is
  trivially forgeable. It defends against transcription and truncation
  errors and serves scanners as an offline plausibility filter. It must
  never gate, short-circuit, or select an authorization path, and
  "checksum valid" must never be logged in a way that reads as
  "authenticated". Conversely a distinct "checksum mismatch" error leaks
  nothing (validity is publicly computable), and constant-time comparison
  is pointless — keep the typo error, it is the whole UX payoff.
- **Do not plan around GitHub's secret-scanning partner program**: it
  requires a public webhook endpoint and a server-side revocation API,
  neither of which a local-first app has. Ship detection-only gitleaks and
  TruffleHog rules ourselves. (TruffleHog cannot live-verify a localhost
  credential at all, which is exactly why the offline checksum earns its
  place here more than it does for a cloud issuer.)
- Grep the tree and docs for the literal `anytype_` before committing; a
  prefix that also appears in identifiers or prose becomes permanent
  allowlist maintenance for every scanner rule that adopts it.
- If `Limited`/gRPC keys stay unprefixed, note the consequence: they are
  invisible to every scanner, and "no prefix" becomes an implicit type
  signal someone will eventually branch on. Prefer giving them their own
  prefix later over leaving them bare forever.

- Parsing stays contained: `load()` (`core/wallet/applink.go:125`) is the
  single parse point — strip the prefix and checksum and decode the body
  (legacy keys keep their base64 decode), then `sha256` the raw bytes
  exactly as today, so both kinds converge on the same raw-bytes-derived
  filename and no existing file lookup changes.
- `generate()` mints the new format; **the old format is never minted
  again** after the flip. All three issuance paths (HTTP `/v1/auth/api_keys`
  and its `/v2` twin, `CreateApp` with `JsonAPI`, and the challenge flow
  with `JsonAPI`) produce the new kind.
- `Limited` keys are unaffected: they are a gRPC/clipper credential, not a
  JSON-API key, and keep the current format. Worth renaming in the docs so
  the two stop sharing the phrase "API key".
- Format is **not** an authorization input anywhere. Enforcement reads the
  grant.

**Downgrade fail-closed.** Scoped keys are written with envelope
`Version: 2` (same v1 layout otherwise). Old heart binaries hard-check
`ver == 1` (`applink.go:309-329`), so a downgraded heart **rejects** a
scoped key instead of silently honoring it unscoped. Unscoped keys keep
ver 1. New heart reads both.

### 2. Proto surface (all additive)

- `model.AccountAuth`: `message AppGrant { repeated string spaceIds; Perm
  perm; }`, `enum Perm { Read; ReadWrite }`; `AccountAuthAppInfo` gains
  `grant` (ListApps shows it).
- `Rpc.Account.LocalLink.CreateApp`: `AppInfo` carries `grant` → persisted.
  This (Full-only) is the issuance path until the picker lands.
- New `Rpc.Account.LocalLink.UpdateApp { appHash, grant }` (Full-only):
  edit a live key's grant without reissuing. Heart applies whatever the
  Full caller sends; widen-requires-re-consent is the desktop UI's contract
  (research §3.1), not heart's — heart cannot render consent.
- `Rpc.Account.LocalLink.NewChallenge.Request` gains `requestedGrant`,
  forwarded into `EventAccountLinkChallenge` for the future picker.
  `SolveChallenge` **persists `requestedGrant` as-is when present**: grants
  only narrow, so a self-requested restriction is fail-safe by monotonicity
  — CLI users get scoped keys at pairing before any desktop UI exists. The
  picker, when it lands, displays and may further narrow it.
- `RpcWalletCreateSessionResponse` gains `grant` (next to P0.2's
  `accountScope`/`appName`) so `ensureAuthenticated` learns it.

### 3. HTTP enforcement

`ApiSessionEntry` grows `Scope` + `Grant`; `ensureAuthenticated` sets both
into the gin context **and** the request `context.Context` (services read
it from ctx — no new service constructor params).

**`ensureSpaceGrant`** — new middleware on the `/v2` group directly after
`ensureAuthenticated`:

- `Grant == nil` → pass through (legacy key).
- `space_id` param present → must be in `Grant.Spaces`, else 403
  `space_not_granted` (C6 envelope, message names the granted list). The
  tech space is denied unless explicitly granted — this middleware runs
  before `ensureSpace`'s tech-space admission.
- No `space_id` param → the route must appear in the **global-route
  registry** with an explicit class: `auth-exempt` (`/v2/auth/*`),
  `data-free-allow` (`/v2/validate`, `/v2/schemas*`),
  `service-filtered` (`GET /v2/spaces`, `POST /v2/search` — allowed
  through, constrained in the service, below). Unregistered ⇒ 403.
- Verb gate: `Perms == "read"` and the route is classified `write` → 403
  `write_not_granted`. Classification is an explicit per-route table in the
  registry. Chat `POST …/read` is classified **write** (it mutates the
  synced read watermark); revisit if agents genuinely need it read-only.

**Deletes — recorded direction (decided 2026-08-06). SUPERSEDED on the
mechanism (2026-08-14): see `core/api/APIV2_OBJECT_DELETE.md` §4, built as
APIV2.md §8.42.** The RULE stands and shipped: delete only objects
**created via that key** — `readwrite` grants create/edit broadly but
destructive delete only of the key's own output (this also answers
Airtable's loudest complaint, research §5: write silently including
destructive delete). The MECHANISM below did not survive design review:
a device-local `appHash` detail in objectstore dies with every store
rebuild/reindex and account recovery (provenance lost, objects permanently
undeletable by their own creator) and can never serve attribution. What
shipped instead is a synced, member-signed record — the integration slug
on the object's CREATING change (`pb.Change.integrationKey`), immutable
the way `createdDate` is, read back from validated storage at delete time.
The open detail stays open: whether schema deletes (`DELETE …/types`,
`…/properties`) follow the same own-output rule or stay plain `readwrite`.

**`/v1` rejection**: one line in the `/v1` group after auth — `Grant !=
nil` → 403 `v1_not_available_for_scoped_keys` pointing at `/v2`. Note the
asymmetry this creates, and that it is intended: a *granted* key is refused
on `/v1` (its grant cannot be honored there), while a *legacy* key is
served on `/v1` exactly as today. Grant presence, not key format, decides.

**Fan-out constraint**: `V2Service.GlobalSearchObjects`/`spaceRefs` and
`ListSpaces` intersect their space set with the ctx grant. v1's
`GlobalSearch` needs nothing (scoped keys never reach `/v1`).

**Service backstop (research Option C)**: `V2Service.ensureSpace`
(`service/v2.go:52-63` — already called by all 27 space-scoped v2 entry
points) also consults the ctx grant and fails closed. Route middleware
gives the clean 403; this catches any future route that forgets the
middleware or resolves ids oddly.

**`GET /v2/auth/whoami`** (authenticated). Describes the CREDENTIAL, not the
person — there is only ever one "who" here. Body (camelCase per C2, RFC 3339
UTC dates):

```json
{
  "key":   { "id": "…", "name": "Claude Desktop", "createdAt": "…", "expiresAt": null },
  "scope": "jsonApi",
  "grant": { "scoped": true, "permission": "read",
             "spaces": [ {"id": "…", "name": "Work", "permission": "readwrite"} ] },
  "api":   { "version": "2026-08-06" }
}
```

- **`grant.scoped` is a required explicit boolean and is the load-bearing
  field.** Do NOT encode "legacy unscoped key" as `spaces: null`: every
  consumer eventually gets the null-vs-empty test backwards, and the failure
  direction is **fail-open** (the agent concludes it may touch every space).
  When `scoped` is false, `spaces` is `[]` and `permission` is `null`.
- **`spaces[]` are objects with a per-entry `permission`** even though the
  grant's `Perms` is uniform today — this is what lets P2 add per-space
  permissions and type grants without a breaking wire change. Keep the
  grant-level `permission` too, as the compact form agents string-match on.
- **Include `spaces[].name`**, derived from the same grant-intersected path
  `GET /v2/spaces` uses, so a non-granted space name cannot appear even by
  accident. It is not new disclosure (the holder can already enumerate
  exactly that set), and it is what lets an agent map "put this in Work" to
  a space id without a second call.
- **Never accept a token as a parameter** (`?key=`, `{"token":…}`). Reading
  it only from `Authorization: Bearer` is what keeps this from becoming the
  enumeration oracle RFC 7662 §4 warns about. Unknown/revoked key → bare 401,
  empty body. Do not implement RFC 7662's shape (no `active`, no POST).
- **`whoami` is discovery, not enforcement**, and must be *derived from the
  same grant record* the gate reads — never computed separately. A second
  derivation path is how the mirror and the gate drift and the mirror starts
  lying. State this in a comment at the handler.

**`WWW-Authenticate` on auth failures** — the standards-shaped channel, and
MCP clients are required to parse it (spec rev 2025-06-18), so it is free
reach into every MCP host. Alongside the C6 JSON envelope:
`401 → Bearer realm="anytype", error="invalid_token"`;
`403 → Bearer error="insufficient_scope", scope="space:<id>:readwrite"`.
The scope string is implementation-defined (RFC 6750 §3.1) — pick a shape,
document it. Do **not** stamp scopes on every 200 in a GitHub-style
`X-OAuth-Scopes` header: it cannot carry `{id, name, permission}` triples
without becoming its own serialization format, and GitHub abandoned that
header for its newer token kind anyway.

**Deprecation signals for legacy keys** (moved here from P0 — needs the new
kind to exist). Corrected 2026-08-06 after checking the RFCs:

**Do not emit `Deprecation` or `Sunset`.** An earlier draft of this spec
said `Deprecation: true`; that is wrong twice over. RFC 9745 (Standards
Track, March 2025) requires the value to be a **Date** — the boolean form
existed only in drafts and was removed before publication — and §2.2 scopes
the signal to *the resource in the response*, not to the credential
presented. Emitting it on `/v1` would tell a conforming client that `/v1` is
going away, which is the opposite of the grandfathering promise. Emitting
neither header is the spec-correct expression of "grandfathered, no sunset".

Emit instead, only when the presented credential is a legacy unscoped key:

```
Anytype-Key-Status: legacy
Anytype-Notice: This API key is a legacy unscoped key with access to every space. It will keep working. Re-issue it as a scoped key in Settings > API Keys.
Link: <https://developers.anytype.io/docs/api-keys>; rel="deprecation"; type="text/html"
```

`Anytype-Key-Status` names the *credential* (the thing that is actually
legacy) and is always present — send `scoped` for new keys so a client never
has to treat absence as meaningful. `Anytype-Notice` is npm's pattern: one
short ASCII sentence a client can print verbatim, never interpolating user
data. `Link rel="deprecation"` without a `Deprecation` header is explicitly
legal and is RFC 9745 §3.1's own worked example for "here is the policy, no
date committed"; if a date is ever set, the two headers slot in beside it.

**Channel ranking, because the headers will not do the work.** The desktop
badge on the key row is the primary channel and it is not close — the agent
holding the key *cannot* re-issue it; only the human in the desktop app can.
`ListApps` already returns enough to badge legacy keys, so no proto change.
Second is a rate-limited log line (info level, once per key per process
start), which exists mainly so we know whether anyone is still on legacy
keys before ever contemplating a sunset. The headers rank third: cheap,
correct, and realistically read by nobody until an integrator looks. Also
put the same signal in the `whoami` body — agents read bodies, not headers,
and RFC 9745 §2.2 explicitly sanctions concentrating deprecation
information on a designated resource.

### 4. Conformance test

Walk `engine.Routes()` (the shipped `v2_wrapper_routes_test.go` pattern):
every registered route either contains `:space_id` or appears in the
global-route registry, and every route appears in the read/write
classification. A new route that skips classification fails CI — the
"forgot the check" failure mode becomes a test failure, not a silent hole.

### 5. Error semantics

403 with the C6 envelope, codes `space_not_granted` / `write_not_granted` /
`v1_not_available_for_scoped_keys`, messages naming the actual grant
(error-guided self-correction is the v2 design language; enumeration
resistance is a non-goal for a localhost single-user API — research §4
"Design choices").

### 6. Testing

- Wallet: grant round-trip through the sealed envelope; ver-2 rejection by
  a ver-1-only reader (simulated); expiry; empty-spaces rejection.
- Middleware: scoped/unscoped × granted/denied space × read/readwrite ×
  global routes; v1 rejection; tech-space denial.
- Service: `ensureSpace` backstop denial; global-search and `ListSpaces`
  intersection (fixture: 3 spaces, grant covers 1).
- Conformance test as above.
- One end-to-end fixture: scoped read-only key against representative v2
  routes (list, get, search, create → 403).

### 7. Compatibility

- Existing keys: `Grant == nil`, unchanged (beyond P0.2's scope gate).
- Old clients: unknown proto fields ignored; the challenge flow without
  `requestedGrant` behaves exactly as today.
- Downgrade: scoped keys fail closed via envelope ver 2 (§1).

### 8. Sequencing within P1

**Release constraint: steps 1–2 (issuance) must not ship in a release
without steps 3–5 (enforcement).** A build that mints and displays grants
but enforces nothing hands out keys that LOOK scoped — `ListApps` badges
them, the desktop can render them — while the key has unrestricted
read-write access to every space. That is worse than no scoping at all: it
invites users to hand the key to an agent on the strength of a boundary
that does not exist, and the failure is silent (no `whoami` to contradict
the UI). Land 1–5 in one release train.

1. Wallet grant model + envelope ver 2 + tests.
2. Proto additions + protogen (`CreateApp`/`UpdateApp`/`ListApps`/
   `NewChallenge`/`WalletCreateSession`).
3. `ensureAuthenticated` carries scope+grant; `/v1` rejection.
4. `ensureSpaceGrant` + global-route registry + conformance test.
5. Fan-out constraint + `ensureSpace` backstop.
6. `whoami`.
7. Docs: developers.anytype.io key-scoping section; fix the Raycast
   "limited access" pairing copy; steer autonomous agents to `anytype-cli`
   bot accounts as the stronger boundary.
