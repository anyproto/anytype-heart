# API key permission scoping — research

Status: research only, no implementation. 2026-08-06, branch `go-7383-apiv2-phase0`.

Question: can an API key be limited to a single space? And beyond that, to a
type ("only read/edit/create objects of type Task")? What would each cost,
what would each actually guarantee, and in what order do they make sense?

Demand is on the record: the community thread "API/MCP Key Scope Settings"
(community.anytype.io, Apr 2026, staff-acknowledged and added to the
tracker) asks for exactly this — keys restricted to selected spaces plus a
read-only mode, with "my MCP server can reach my personal spaces" as the
stated worry. It also asks for object-level control; §6 is the case for
declining that half.

Evidence base: code survey of the key lifecycle (`core/wallet/applink.go`,
`core/session`, `core/application/sessions.go`, `core/auth.go`), the full
v1+v2 route surface (`core/api/server/router.go`), the type/restriction
machinery (`core/block/restriction`, `pkg/lib/database`, `core/api/service`),
and a web survey of industry practice (§3; sources at the end — claims
marked [K] are background knowledge not re-verified against a primary
source, everything else was checked against primary docs during the survey).
File:line references are as of this branch.

---

## 1. What scoping can and cannot be — the threat model

Scoping here is **policy enforcement inside heart's HTTP layer**, not a
cryptographic or process boundary. Three facts bound what any design can
promise:

1. **Heart holds everything.** All spaces' data sits in one process and one
   unencrypted-at-rest store; every API call already runs in-process as the
   full account (no principal threading below the gin layer). A scoped key
   constrains what a *key-holder* can request, not what the process can do.
2. **The file gateway is unauthenticated.** `pkg/lib/gateway/gateway.go:76,
   107-108` binds 127.0.0.1 and serves `/file/` and `/image/` by CID with no
   auth. Any local process that can reach the API port can fetch any file's
   bytes without any key. File *content* confidentiality against local
   processes does not exist today, regardless of API scoping. (And
   localhost-bound servers are reachable from hostile *web pages* too, not
   just local processes — the new `ensureTrustedOrigin` middleware covers the
   API; the gateway has nothing.) The general rule behind this: **a key's
   scope is an upper bound only while every reachable operation consults the
   key.** A surface that answers without a key voids the guarantee for
   whatever it serves; here, that is file bytes — which no amount of key
   scoping constrains.
3. **Local malware is out of scope.** A process that can read
   `~/.../<account>/` gets the store directly. Scoping does not defend
   against that and should never be described as if it did. Local consent
   enforcement is a hard problem even for platform owners who control the
   whole stack; an HTTP layer is not where it gets solved.

What scoping *does* defend, and it is worth defending:

- **Prompt-injected agents** (already named as the motivating threat in
  `docs/AgentApiV2Research.md` §4.6; the agent-security practice surveyed in
  §3.4 is uniformly shaped around this). The agent acts only through its tool
  surface — the API with its key. Least privilege directly caps the blast
  radius of a hijacked agent.
- **Buggy or over-curious integrations** — the Raycast/MCP/script class.
- **Remote or sandboxed key-holders** that genuinely have nothing but the
  key and the API port. For these, the policy layer *is* the boundary.

This is the honest framing: blast-radius reduction and consent, not
isolation. It's also exactly what the rest of the industry ships (§3).

---

## 2. Current state — the facts that shape the design

### 2.1 Key anatomy: the grant slot already exists

An app key is a raw random AES-256 key, base64-encoded; the bearer token on
every HTTP request *is* the key (`core/api/server/middleware.go:75`). Server
side, each key has one metadata record:

```go
// core/wallet/applink.go:32-39
type AppLinkInfo struct {
    AppHash   string `json:"-"`
    AppKey    string `json:"app_key"`
    AppName   string `json:"app_name"`
    CreatedAt int64  `json:"created_at"`
    ExpireAt  int64  `json:"expire_at"`  // never set, never checked
    Scope     int    `json:"scope"`
}
```

stored one file per key under `<repo>/auth/<sha256(key)>.json`, X25519-sealed
to the account pubkey, HMAC-bound to the app key, Ed25519-signed by the
account key (`applink.go:255-329`). Two useful properties fall out for free:

- **Additive fields are compatible.** Old files unmarshal with nil new
  fields (⇒ "unscoped, legacy"); old code ignores unknown JSON fields.
- **Grants would be tamper-proof.** The signature means nothing short of the
  account key can forge or widen a grant record on disk.

List/revoke exist (`AccountLocalLinkListApps/RevokeApp`, Full-scope gRPC
only). Revocation deletes the file.

### 2.2 Scope enforcement today: one good wall, and holes in it

The scope enum is `Limited / JsonAPI / Full`
(`pkg/lib/pb/model/protos/models.proto:738-742`). Enforcement:

- **gRPC interceptor** (`core/auth.go:56-90`): `JsonAPI` sessions are denied
  *every* gRPC method (deny-all `default` branch); `Limited` gets a
  hardcoded 13-method allowlist. This is the wall that makes the gin layer
  the sole surface a JSON-API key can reach — the precondition for HTTP-layer
  scoping being sound at all.
- **HTTP layer**: `ensureAuthenticated` (`middleware.go:61-106`) exchanges
  key → session token and **never consults scope**. Downstream, the token is
  set into the gin context and never read again; all service calls run as
  the account.

Holes found, all of which must close before any new scoping is credible —
otherwise space scoping is partially theater:

| # | Hole | Where | Effect |
|---|---|---|---|
| H1 | `SolveChallenge` mints the pairing session with the **zero-valued named return** (`Limited`) instead of `challenge.scope` | `core/session/challenge.go:105` vs `:113` | Every key pairing done over gRPC hands the client a live `Limited` gRPC session — ObjectSearch/Show/Create **in any space** — precisely what `JsonAPI` scope exists to forbid. (The HTTP auth endpoints happen to drop this token, `core/api/service/auth.go:42-51`, so HTTP-paired clients don't see it; gRPC-paired ones do. In-memory sessions die on restart, which caps but doesn't excuse it.) |
| H2 | HTTP auth accepts **any** valid key, including `Limited` web-clipper keys | `middleware.go:61-106` | A key issued for the clipper's 13 gRPC methods silently also grants the entire JSON API. Scope should be checked at the gin gate (`JsonAPI` or `Full` only). |
| H3 | `AccountLocalLinkCreateApp` persists **any** requested scope incl. `Full` unvalidated | `core/application/sessions.go:152-160` | Full-only method, so escalation requires Full already — but it mints *permanent* Full app keys, and the challenge path deliberately refuses Full (`challenge.go:49-54`). Inconsistent by accident, not by decision. |
| H4 | Revocation is incomplete | `sessions.go:194-219`, `core/api/server/server.go:106-115` | Only the latest session per app-hash is closed; tokens re-minted via `WalletCreateSession(token:)` (a no-auth method) survive; the API key-cache eviction deletes exactly one entry. |
| H5 | `ExpireAt` is dead | `applink.go:37` | Keys are immortal; the field is stored, listed, and never enforced. Industry trajectory runs the other way: Figma removed non-expiring PATs outright, GitHub defaults org tokens to ≤366-day lifetimes, Linear moved OAuth to short-lived tokens in 2025. Airtable's no-expiry PATs are now the noted outlier. |

Also relevant: `AccountAuthAppInfo` returns the **live plaintext key** of
every paired app to any Full caller (`applink.go:194-201`) — fine for the
desktop's own management UI, worth remembering when reasoning about what
"Full" means.

### 2.3 Route surface: structurally ready for a space gate

- `space_id` is **uniformly a URL path param** in both v1 and v2 — no
  request DTO carries a space field. Group middleware sees `c.Param
  ("space_id")` before handlers run; two shipped middlewares already read it
  there (`ensureFilters`, `ensureIdempotency`).
- The global (no-space) routes are a short, closed list: `/v1/auth/*`,
  `GET|POST /v1/spaces`, `POST /v1/search`, docs routes; `/v2/validate`,
  `GET /v2/spaces`, `POST /v2/search`, `/v2/schemas*`.
- The two global searches fan out **inside the service layer**
  (`service/search.go:27-76` over `GetAllSpaceIds`; `service/v2_search.go:
  692-776` over `spaceRefs()`), so a path-param middleware cannot constrain
  them — they need the grant threaded into the space-set selection.
- `ensureSpace` (v2) **explicitly admits the tech space** as an ordinary
  space id (`service/v2.go:56-58`).
- The route inventory is already machine-checkable — `v2_wrapper_routes_test.go`
  walks `engine.Routes()`; the same pattern can pin "every route is either
  space-scoped or explicitly classified global."

v1-specific space-confusion bugs a scoped key would inherit (independently
worth fixing, but they triple the audit surface if scoped keys must honor
v1):

- **File download ignores `space_id` entirely** — `handler/file.go:38-61` /
  `service/file.go:68-110` resolve by object id *or raw CID*, any space.
- **All v1 chat mutations and the SSE stream key on `chatId` alone**
  (`service/chat.go:128-254`, `chat_stream.go:79`); the path `space_id` is
  decorative. v2 closed this class with `ensureChat(spaceId, chatId)`
  (`service/v2_chat.go:389-408`).
- Member lookup accepts a **raw identity** as well as a participant id
  (`member.go:107-110`).

### 2.4 Type machinery: identity is the hard part

- An object's type is cheap to resolve without opening it: details carry
  `type` (the type object's per-space id); `type.uniqueKey` nested filters
  already work in the query engine and both APIs already use them.
- **Type is mutable post-creation** (v1 `PATCH` `type_key` →
  `ObjectSetObjectType`; v2 `PUT` document `type` is authoritative; v2
  `PATCH` refuses it).
- **Identity across spaces only exists for bundled types** (`ot-task` is
  universal). Custom types get random per-space unique keys
  (`objectcreator/util.go:32-44`). v1's client-facing `apiObjectKey` is
  **user-settable after the fact** (`service/type.go:224-230, 297`) — a
  grant pinned to an api key string can be silently re-pointed to a
  different type. Grants must pin the immutable `uniqueKey` (plus space),
  never the display key. Consequence: **type grants only make sense inside a
  space grant** — the grant model nests, which independently argues for
  building space scoping first.
- There is **no existing per-type or per-object permission machinery** to
  build on. `core/block/restriction` is a pure function of
  (sbType, layout, uniqueKey); the one per-type flag that exists
  (`CreateObjectOfThisType`) is set and never checked — a client-side hint.
  ACL enforcement happens in any-sync (`ObjectTree.AddContent`), per
  participant, not per key.

---

## 3. Prior art — what the industry ships (web survey)

Surveyed: the fine-grained-key generation (GitHub fine-grained PATs, Stripe
restricted keys, Airtable PATs, Linear, Figma, Slack, Discord, OpenAI,
Notion), the per-type counterexamples (Salesforce, Strapi, Directus,
PostgREST/Supabase, Firebase, Microsoft Graph), token technology
(macaroons/biscuits/RAR), local-first peers (Obsidian, Home Assistant), and
the 2024–2026 MCP/agent security-practice guidance.

### 3.1 The convergent pattern

Every product that took a second pass at API keys landed on the same shape:

| Product | Container axis | Verb axis | Notes |
|---|---|---|---|
| Airtable PATs (2023) | base/workspace allowlist | small closed scope set (records/schema × read/write) | **closest structural analog**; per-*table* scoping requested by users, never shipped; no expiry (now the outlier) |
| GitHub fine-grained PATs (2022→GA 2025) | single owner + repo allowlist | ~50 resource-class perms × none/read/write | 2.5-year beta; per-endpoint→permission reference doc |
| Stripe restricted keys | (account-mode only) | closed resource × none/read/write, default **none**, write⊃read | docs now say: use RAKs "especially when you provide a key to an AI agent"; 403s name the missing permission |
| Linear (Apr 2025) | team allowlist (incl. private teams) | read/write/admin/create-issues/create-comments | converged late, shipped as a simple settings UI |
| Notion internal integrations | page/database connect (subtree-inheriting) | integration-wide capabilities (read/update/insert content, comments, user info) | consent is *per-container in the sharing UI*, capabilities fixed per integration |
| Microsoft Graph `Sites.Selected` (2021+) | per-site admin grant, zero access by default | scope = capability class | their stated rationale: the scopes model couldn't express container grants, so a separate grant surface was bolted on |
| OpenAI (2024) | project-scoped keys | all/read-only/restricted per endpoint class | plus per-project model allowlists |
| Figma (2023) | **none** | ~25 operation scopes | the counter-datapoint: verbs with no container axis; also removed non-expiring PATs |
| Slack (2020) | none in the token — channel **membership** is the container consent | granular scope strings | published rationale: coarse grants suppressed install approval — consent economics |
| Discord | guild → channel overwrites | 53-bit verb bitfield | the `ADMINISTRATOR` bit overrides every per-channel deny by design — the canonical override footgun |

Convergent facts beyond the table:

- **Opaque token + server-side grant record, unanimously.** Nobody ships
  self-describing tokens for user-facing keys; Supabase's 2025 key rework
  moved *away* from JWT-shaped keys to opaque `sb_secret_…` precisely to get
  instant revocation and rotation back.
- **Consent-time container picking is the direction of travel.** Google
  `drive.file` + Picker (app sees only what the human pointed at, while the
  broad `drive` scope was made bureaucratically expensive), Google Photos'
  2025 *removal* of library-wide scopes in favor of a Picker API, Notion's
  page-connect, Home Assistant's per-entity "expose to Assist" allowlist [K]
  — when the consumer is an agent, the human points at containers.
- **Mandatory expiry without a rotation story nearly killed GitHub's
  adoption** (reverted for personal tokens, Oct 2024); Stripe instead ships
  no lifetimes but a 7-day dual-validity rotation window. Lesson: enforce
  expiry, but only alongside a usable rotation/re-pair flow.
- **Two model-level design lessons worth heeding:** Discord's
  `ADMINISTRATOR` bit overriding every per-container deny (don't ever add an
  override bit to the grant model — `Full` keys should be a different kind,
  not a bit inside scoped semantics); and the architectural point that a
  *retrieval layer* holding cross-container privilege can quietly void
  per-container scoping — if a future Anytype AI feature reads across spaces
  on behalf of a scoped consumer, the key's space grant buys nothing at that
  layer. Scope the retrieval context, not just the key.
- **Consent economics are real** (Slack's published rationale for granular
  scopes): a smaller, legible consent screen increases approval. Argues for
  keeping the grant dialog to *spaces × read/write* and resisting
  checkbox-matrix creep.
- **Make keys introspectable from day one.** GitHub's fine-grained PATs are
  not — no header or endpoint reveals a token's grant — and the direct
  consequence is that GitHub's *own* MCP server cannot scope-filter its
  tool surface for them (it can for classic PATs via `X-OAuth-Scopes`). A
  `GET /v2/auth/whoami` returning `{name, spaces, perms, expiresAt}` is
  cheap now and unretrofittable later.
- **Let the container list be edited after issuance without reissuing the
  key.** Airtable does ("users can add or revoke resources for an access
  token at any time"); 1Password's immutable service accounts are the
  anti-model (widening = recreate the credential). But guard the edit path:
  a re-consent flow that silently defaults to the *widest* container set on
  reopen would let routine maintenance (e.g. bumping expiry) accidentally
  broaden the grant — GitHub's token-edit form is a known example of exactly
  that footgun. Widening must round-trip the stored grant and demand
  explicit re-consent.

### 3.2 Per-type scoping: the counterexamples, and what joins cost them

The hunt for systems that DID ship per-type API scoping found them — and
they testify against doing it as a consumer key surface:

- **Salesforce**: object-level CRUD + field-level security apply to API
  principals (least-privilege Integration User licenses since 2023). The
  reference graph surfaces as `INSUFFICIENT_ACCESS_ON_CROSS_REFERENCE_ENTITY`
  and "No such column" errors — documented integration-support staples that
  a paid admin class exists to debug.
- **PostgREST/Supabase**: table/column grants + RLS flow straight through
  the API; embedding (joins) fails unless grants cover the entire join
  graph. The policy lives in the database per *principal*; the key only
  selects the principal — even Supabase didn't put per-table scoping on the
  key.
- **Strapi** (closest to Anytype: user-defined content types, and the only
  system found that put per-type CRUD **on the token itself**, v4.4+):
  relation `populate` requires find permission on the *related* type;
  failures surface as misleading "invalid" errors (open issue, Feb 2025),
  and the community answer is "grant broad find" — the matrix erodes under
  join pressure. And the deeper problem is structural: because `populate`
  reaches related types, a per-type token boundary is only ever as tight as
  the policy on *every reachable type* — the one type-on-token precedent is
  a live illustration of exactly the join-graph erosion §6.2 predicts.
- **Sanity** (user-defined document types; Enterprise custom roles):
  document-level grants with GROQ filters like `_type == "article"` — and
  the docs state plainly that **"joins or reference traversal in filter
  GROQ" cannot be expressed**. The cleanest vendor admission on record that
  type filters and reference graphs don't compose.
- **Directus**: the most complete implementation (collections × actions ×
  fields × row filters) — an admin-configured policy system, not
  issuance-time consent.
- **Firebase security rules**: expressive per-resource policy, but
  expressiveness in non-expert hands is a well-known source of
  misconfiguration at scale. Policy expressiveness is not safety.

Pattern: per-type scoping exists only as **admin-configured policy in
schema-centric systems where someone is paid to maintain it**, never as a
key-holder consent surface; and every instance chose one of three join
semantics — hard error, silent omission, or undereferenceable ids — all
documented sources of integrator misery. None of the consumer-grade key
systems (Airtable, Linear, Figma, Notion, GitHub, Stripe) chose to enter
that territory at all — every one of them stops at the container. Field
confirmation of the usability wall from the closest analog: Notion relation
properties pointing at unshared containers come back as bare ids that 404
on dereference, and the community's standing answer is "share the related
database too."

### 3.3 Token technology — surveyed, and why we stay boring

- **Macaroons** (offline attenuation, Google 2014): real adopters are lnd
  (Lightning's `bakemacaroon`, an `entity:action` caveat vocabulary) and
  Fly.io. The one feature a signed self-describing token buys over our
  existing per-key record is a holder minting a *narrower* token offline,
  with no server round-trip. Everything else argues against — revocation
  still needs server state (erasing the statelessness win), and there is no
  standard caveat language. Park it as a possible future *key kind* if
  sub-agent delegation ever becomes a real workflow. (If a mint-narrower-
  keys scope ever ships, learn from lnd's `bakemacaroon` design: a "mint
  further keys" permission is de-facto root unless baking is explicitly
  constrained to a subset of the baker's own rights.)
- **Biscuit / Zanzibar-style ReBAC** (SpiceDB, OpenFGA): biscuit is niche;
  ReBAC solves globally-consistent authorization over a *shared*
  relationship graph across many services and users. A single-user local
  process has no sharing graph and one decision point — it's a query engine
  for a problem we don't have. Skip.
- **OAuth 2.0 Rich Authorization Requests** (RFC 9396, Jan 2023): near-zero
  consumer-SaaS adoption, but its `authorization_details` object —
  `{type, locations, actions, datatypes}` — maps almost exactly onto
  `{spaces, verbs, typeKeys}`. Worth borrowing as the *shape* of the grant
  JSON (self-documenting, extensible) without adopting any OAuth machinery.

### 3.4 The agent threat, 2024–2026

The dominant failure mode the community documents is one confused-deputy
shape — an over-privileged key + untrusted content reaching the agent + an
outbound channel it can write to (Simon Willison's "lethal trifecta," Jun
2025). The recurring pattern across published write-ups is the same each
time: an agent holds a credential far broader than its task, ingests
attacker-controlled text, and is steered into moving privileged data to a
place the attacker can read. None of these turn on a product defect — the
common root is over-privilege, and the standard mitigations that vendors
shipped in response are exactly **read-only modes and container (project)
scoping**. Scoping is the practical way to shrink the trifecta's
"private data" leg per integration.

MCP's own spec (rev 2025-06-18) makes servers OAuth resource servers but
**deliberately defines no tool-level authorization**, and marks tool
annotations like `readOnlyHint` as untrusted. So real enforcement has to
live in the API the MCP server calls — i.e. in heart's key, exactly where
this design puts it. The convergent agent-facing practice is: read-only by
default, container-narrow at connect time, human-in-the-loop for writes at
the client, and short-lived per-session credentials. The recommendations
below are that practice.

Two agent-era additions with three-vendor convergence (2025-26):

- **The tool surface should be a function of the grant.** Supabase MCP
  (`read_only`, `project_ref`, `features=`), GitHub MCP (`--read-only`
  overrides `--toolsets`; OAuth scope filtering hides tools the token
  can't use), Stripe MCP (tool availability follows the restricted key's
  matrix). For us this is nearly free: `anytype-mcp` generates its tools
  from the OpenAPI spec, so it can call `whoami` and emit only what the
  grant permits — GitHub reports 60-90% context reduction as the side
  benefit.
- **Read-only is necessary, not sufficient — say so in the docs.**
  Read-only removes the direct write-back channel, but an agent that can
  still emit text to any place a third party can read has an outbound path
  regardless; and a container boundary enforced at the API layer buys
  nothing if a higher AI/retrieval layer re-aggregates across containers.
  Scoping shrinks the trifecta's private-data leg; it never removes the
  other two. (Relatedly: the current Raycast pairing copy — "limited access
  to your account" — already overclaims for what is an account-wide grant;
  fix the copy regardless of what ships here.)

---

## 4. Axis 1 — space-scoped keys

This is the industry-consensus cell (§3.1): Airtable bases × scopes, Linear
teams × verbs, Microsoft `Sites.Selected`, OpenAI project keys. No
local-first peer (Obsidian, Home Assistant) ships even this much — building
it puts Anytype ahead of the field.

### Option A (recommended): grant record on the app key + HTTP-layer enforcement

**Model.** Extend `AppLinkInfo` with a versioned grant, e.g.
`{spaces: []string|null, perms: "read"|"readwrite"}` (RAR-shaped, §3.3); nil
= legacy unscoped. Wire format is additive (§2.1); the signed envelope gives
integrity for free. Surface it through `WalletCreateSession`'s response (or a
dedicated `ValidateApiKey` on the in-process middleware surface) so
`ensureAuthenticated` can cache `{token, scope, grant}` in `ApiSessionEntry`
instead of dropping everything but the token.

**Enforcement.** One group middleware after `ensureAuthenticated`:
`space_id` param present → allowlist check (403 on miss, tech space denied
unless granted). Plus the closed list of global routes, each explicitly
classified: auth endpoints exempt; `GET /spaces` filters output to the
grant; global search passes the grant into `GetAllSpaceIds`/`spaceRefs` as
the candidate set; `/v2/validate` and `/v2/schemas*` are data-free, allow.
A router-walking conformance test (the existing `engine.Routes()` pattern)
pins fail-closed: any new route must be space-param'd or classified, or CI
fails. That converts the classic "new route forgets the check" failure mode
from a silent hole into a test failure.

**The v2-only lever.** Scoped keys could be **rejected on `/v1` routes**
(401/403 with a pointer to v2). This shrinks the audited surface to the API
that already has `ensureSpace`/`ensureChat` discipline and dodges v1's
space-confusion holes (§2.3) as *scoping* prerequisites — they remain plain
bugs to fix on their own schedule. Cost: existing v1 integrations can't
adopt scoped keys until they move to v2. Given v2 is explicitly headed for
"complete, self-contained API" with a v1-parity conformance gate (Phase 8),
this is alignment, not a detour. (Google Photos' 2025 retroactive scope
removal is the cautionary tale for the *other* path — narrowing an existing
surface with no migration burned an ecosystem; making scoping opt-in on a
new surface avoids that entirely.)

**Consent UX.** The challenge request would carry the requested grant, and
the desktop approval dialog shows it — the picker-at-pairing-time pattern
that the whole industry is converging on (§3.1: Google Picker/`drive.file`,
Notion page-connect, Home Assistant Assist exposure). Either way this is
cross-repo work (proto change to `NewChallenge.Request` /
`EventAccountLinkChallenge`, desktop UI). Critically: `APIV2_SURFACES.md`
Phase 8 records that **a reworked challenge flow is already landing
separately across heart and the clients** — that rework is the natural (and
probably only) vehicle for adding grant selection without doing the
consent-UI work twice. Sequencing with that project is the main external
dependency of this whole effort.

**Pros.** Grant lives where the key already lives; no new token format
(Supabase's 2025 *retreat* from JWT keys to opaque records, §3.1, is the
fresh confirmation this is the right pole); no crypto changes; revocation
story unchanged; enforcement at a real chokepoint with a mechanical
completeness check; backward compatible.

**Cons / risks.** Enforcement completeness is a discipline problem —
mitigated but not eliminated by the conformance test (response *content*
still needs eyes: e.g. v2 global-search rows carry `spaceId`, fine once the
fan-out set is constrained). The grant is invisible in the key itself
(server-side lookup only) — irrelevant for a localhost API. Pre-auth
all-space cache warm-up (`ensureCacheInitialized` runs before auth and
subscribes across all spaces) is internal state, not exposure, but worth
knowing when reasoning about "what did this key touch."

### Option B: self-describing signed tokens (JWT / macaroon / biscuit)

Embed `{spaces, perms, exp}` in a token signed by the account key;
optionally macaroon-style offline attenuation (a holder mints a narrower
key for a sub-agent without a server round-trip).

**Verdict: rejected for now.** Statelessness buys nothing on a
single-account localhost server that already has a per-key store; revocation
would *newly* require a revocation list (today: delete the file); it
obsoletes every issued key and both pairing flows; and it adds a token
format to get wrong. Supabase walking this exact path backwards in 2025 —
JWT keys → opaque `sb_secret_…` *specifically to recover instant revocation
and rotation* (§3.1) — is the strongest recent evidence against. The one
genuinely attractive property, offline attenuation for agent trees (§3.3),
is speculative today and could be added later as a separate key *kind*
without rearchitecting Option A. If sub-agent delegation becomes real,
revisit.

### Option C: enforcement at the dependency boundary (defense in depth)

Instead of (or under) route middleware: wrap what the API services hold —
the `apicore.ClientCommands` in-process client for v1, and for v2 the
`objectstore.ObjectStore` handle, so `SpaceIndex(spaceId)` itself checks the
grant carried in `ctx`. Most pb requests carry a `SpaceId` field that could
be checked mechanically (protoreflect/codegen), failing closed on requests
without one — which would have *caught* the v1 chat/file holes, since those
RPCs carry no space at all.

**Verdict: not a standalone answer** — v2 reads bypass `ClientCommands`
entirely (direct store queries), services are singletons so the grant must
ride `ctx` anyway, and a pure dep-layer check produces worse errors (500-ish
from deep inside vs a clean 403). But as a **second layer under Option A**
— specifically wrapping `SpaceIndex()` for v2 and the space-set selection in
the two global searches — it's cheap and catches exactly the class of bug
the route layer can miss. Recommended as belt-and-braces, not foundation.
The design lesson behind the second layer: a container boundary enforced
only at the route/path layer — string-matching a URL prefix — is fragile,
because path handling and route matching can disagree about what a request
actually addresses. The check belongs at object-id resolution in the
service layer, which is exactly what wrapping `SpaceIndex()` gives.

### Option D: a scoped principal — the bot account (any-sync ACL)

The mechanism Anytype *already ships headless*: `anytype-cli` creates a
dedicated bot account whose reach is the spaces it is explicitly invited
into — the README's own words: "The bot account only has access to spaces
it explicitly joins … easily revoke its access at any time from the
desktop app." That is scoping by **principal**, enforced by any-sync's ACL
and read-key distribution rather than by gin middleware: the bot never
holds keys for non-granted spaces, so the boundary is cryptographic, not
policy. Read-only is expressible as inviting the bot as ACL Reader. Same
pattern as Salesforce's integration user, Dropbox's App Folder, and
1Password service accounts — and it composes with Option A (Notion's rule,
"a connection's capabilities never supersede the user's," becomes: the
key's grant narrows what the principal can already reach, never widens).

**Costs, honestly:** it is a second account — on desktop that means either
a second heart instance (what the CLI runs) or multi-account support in
one process (large); granted spaces replicate into the bot's own storage
(double local storage for those spaces); the bot is visible as a
participant to every space member; invite/join needs UX. And it answers
none of the per-type, tech-space, or global-search questions — those
remain Option A's job.

**Verdict: not a competitor to Option A — the high-assurance tier beside
it.** Option A is the cheap consent surface for "my Raycast/MCP key
shouldn't see my personal spaces"; Option D is the real boundary for "an
autonomous agent works inside this one space." Document both with their
distinct guarantees; steer autonomous-agent setups to the CLI's bot
accounts today; weigh a desktop "create agent account" story later. It
should not gate P1.

### Design choices inside Option A

- **Single space vs space list.** Store a list, even if the consent UI only
  offers "one space" initially — "N keys for N spaces" is a fine v1 UX and
  the model shouldn't have to migrate to widen it.
- **Tech space:** deny unless explicitly granted. `GET /spaces` filtered.
- **Space creation** (`POST /spaces`): deny for scoped keys — a key that
  can mint spaces it then owns isn't meaningfully scoped.
- **No override bit.** `Full` keys stay a separate *kind*, never a bit
  inside scoped semantics — Discord's `ADMINISTRATOR` bypass (§3.1) is the
  standing lesson that an override bit silently voids every container deny.
- **Grant list mutable post-issuance, widen-guarded** (§3.1): editing the
  space list on a live key beats reissuing; widening requires explicit
  re-consent, and the edit UI must round-trip the stored grant (GitHub's
  re-widening form bug is the cautionary precedent).
- **Error semantics, decided not defaulted.** For an out-of-scope space,
  Notion deliberately 404s to close the enumeration oracle and pays for it
  in support load; Stripe 403s and *names the missing permission*. Our
  threat model is a hijacked agent, not an attacker probing for space ids,
  and error-guided self-correction is the v2 API's design language — so:
  **403 naming the grant** ("key not granted space X; granted: […]"), with
  `whoami` for orientation.
- **Key format: recognizable prefix + checksum** (the GitHub 2021 token
  redesign): costs nothing now, makes leaked keys detectable by
  gitleaks/secret-scanning later, and cannot be retrofitted cheaply.

---

## 5. Axis 2 — read vs read-write (do this regardless)

The route method is almost sufficient: GET/HEAD = read; everything else =
write; explicit exceptions classified per route (`POST /search` and
`/validate` are reads; chat `POST .../read` mutates only the caller's
watermark — classify deliberately). The v2 router already visually separates
read and mutation registration (mutations carry the rate-limit + idempotency
middlewares), so the classification exists implicitly; making it explicit is
small. `docs/AgentApiV2Research.md` §4.6 already recommends read-only
default keys (Stripe trust-boundary pattern). This is the cheapest
meaningful cut of least privilege and belongs in the same grant record and
the same consent dialog as spaces.

Prior art converges hard here and is worth taking seriously: GitHub
fine-grained PATs, Stripe restricted keys (write⊃read, default none), Notion
capabilities, and Linear all express access as *container × read/write*, and
read-only is the flagship cut every 2025 MCP mitigation reached for first
(§3.4). Stripe's docs now say to use restricted keys "especially when you
provide a key to an AI agent." "Container × verb" for Anytype is exactly
*spaces × read/write*. Slack's published rationale for granularity — coarse
grants *suppress* consent approval (§3.1) — argues for keeping this dialog
legible: spaces × read/write, resisting checkbox-matrix creep.

Two refinements from the survey: **don't bundle delete into write** if the
grant language can afford three verbs — Airtable's single loudest complaint
is that `data.records:write` includes destructive delete; and consider a
**read-only endpoint alias** à la Linear's `mcp.linear.app/mcp/readonly` —
read-only expressed in the URL a client is pointed at, so a misconfigured
client cannot write even holding a write-capable key.

---

## 6. Axis 3 — type scoping ("only Task objects")

The honest answer: **write-side type scoping is feasible and useful;
read-side type scoping is a tarpit** and should not be promised. The
counterexample hunt (§3.2) is the external evidence: per-type scoping exists
only as admin-configured policy in schema-centric systems (Salesforce,
Strapi, Directus, PostgREST/RLS), never as a consumer key-consent surface,
and every one pays a documented tax on the reference graph.

### 6.1 Write-side: "may only create/edit/archive Tasks" — feasible

The write chokepoints are few, already resolve type (or can, from the store,
for pennies), and are the same places the current restriction checks live:

| Path | Chokepoint |
|---|---|
| v1 create | `service/object.go:117` (after `ResolveTypeApiKey`) |
| v1 retype | `service/object.go:182-191` |
| v1 delete/archive | `service/object.go:218` |
| v2 create | `v2_create.go:328-368` (validate) / `objectcreateadapter.go:45` |
| v2 patch | `objectmutateadapter.go:48` + the `checkObjectEditable` pattern at `:132-144` — a `checkTypeInScope(sb)` twin runs under the same open-smartblock lock, closing the check-to-write race |
| v2 put (incl. retype) | `v2_edit.go:273-292` |

Semantics that must be decided, all decidable:

- **Retype = escape.** Changing a Task to another type, or another object to
  Task, is denied for a type-scoped key (v2 PATCH already refuses `type`;
  the PUT and v1 paths gain the check).
- **Enforce at API-operation level, not storage-write level.** Editing a
  Task may internally create relation-option objects, bump counters, touch
  the archive — those are side effects of a permitted operation, running as
  the account like everything else. Trying to police internal writes means
  threading a principal through the smartblock layer: a huge refactor for no
  real threat-model gain (the key-holder never addresses those writes).
- **Pin by `uniqueKey` within the granted space** (§2.4). Never by
  `apiObjectKey` (user-settable ⇒ grant re-pointable), never by name. The
  consent UI shows the name; the grant stores `(spaceId, ot-…)`.

Cost: moderate — a handful of checks at listed sites plus grant plumbing
that Axis 1 already built. Value: real — "the agent can manage tasks but
cannot touch my notes" is the concrete ask behind this research. Strapi and
Directus prove per-user-defined-type CRUD matrices are buildable; their join
pain (§3.2) is a read-side problem, so a *write-only* type limit sidesteps
it — you are gating which types the key may author, not which it may see.

### 6.2 Read-side: "may only read Tasks" — advise against

Enforcing GET-by-id and injecting `type IN (...)` into the API's list/search
query builders is easy (v2 even has per-request type filters already:
`v2_search.go:189-200`). One caveat even there: injection must live in the
**API's** query construction, not `database.NewFilters`' default-filter
injection — that chokepoint is process-wide and shared with the desktop
client's own subscriptions. That's the easy 20%. The remaining 80% is a
leak inventory that keeps going:

- **Markdown resolves neighbors.** Every v1 object GET attaches markdown;
  the exporter resolves linked/mentioned/collection-member objects' names
  through a resolver with **no filters at all**
  (`export/objectresolver.go:169-181`, `converter/md/md.go:822-830`). The
  v2 refs legend does the same job by design.
- **`ObjectShow` computes full dependent-object details** (all link/mention
  targets, relation values, types — `smartblock.go:460-523`). Today v1 DTOs
  serialize only `Details[0]` (with one exception in `list.go:151-157`), so
  the leak is latent in the middleware response, one refactor away from the
  wire. A confidentiality guarantee resting on "we happen not to serialize
  it" is not a guarantee.
- **Sets/collections/views** return objects of whatever type the set
  targets, and view definitions expose filters over other types.
- **Space-wide schema**: types, properties, tag options (which carry
  content-ish text) are catalog endpoints.
- **Chat** messages embed object mentions; files are their own types.

And even a perfectly sealed version has a **usability wall**: a Task
references an assignee (participant object), a project, attachments (file
objects), tag options. A strict Task-only reader receives ids it can never
dereference; every real integration immediately needs "Task + participants +
attached files + options + …" and the grant language turns into a policy
over the object reference graph — the Postgres-RLS lesson (§3.2: policies
must cover the join graph or they strangle the application) in a system
where the graph is user-shaped and the type vocabulary is open. The
counterexamples confirm the three available join semantics — hard error,
silent omission, undereferenceable ids — are all integrator misery, and
every consumer-grade key system declined to enter that territory.

**If list-narrowing is ever wanted, reframe it**: a per-key *default search
filter* (key sees only Tasks in list/search responses; GET-by-id stays
space-wide) — a capability-shaping and token-budget feature, explicitly
**not** a confidentiality claim. Cheap, honest, and probably what an
integration author actually wants. Stripe's restricted keys — the one real
type×verb precedent among consumer keys — work only because Stripe's
resource vocabulary is closed, fixed, and reference-free; none of those hold
here.

---

## 7. Recommended sequencing

- **P0 — hygiene, prerequisite, valuable standalone:** fix H1 (one
  identifier: `challenge.scope`), H2 (check scope at the gin gate), H4
  (revocation completeness), H5 (enforce `ExpireAt` — the industry is
  actively retiring immortal keys, §2.2/§3.1); decide H3. Without P0, any
  scoping above it is partially decorative.
- **P1 — space × read/write grants (Options A + C-as-backstop):**
  `AppLinkInfo` grant record; `ensureAuthenticated` carries it; space-gate
  middleware + classified globals + fan-out constraint + route conformance
  test; scoped keys v2-only; tech space denied; consent via the reworked
  challenge flow (coordinate — external dependency); grants shown in
  ListApps/management UI and editable post-issuance (widen ⇒ re-consent);
  `GET /v2/auth/whoami`; key prefix + checksum.
- **P2 — type-scoped writes**, pinned by `(spaceId, uniqueKey)`, enforced at
  the six chokepoints in §6.1. Optionally resource-class perms
  (objects/schema/files/chats) if the consent UI can carry them — chats
  especially are worth a separate switch given their sensitivity. GitHub's
  ~50-permission precedent says resource classes are worth it eventually,
  Slack's consent-economics rationale says not necessarily first. Same
  tier: shape `anytype-mcp`'s tool surface from the grant (§3.4); an
  attribution axis (mark agent-authored objects — Linear's `actor=app`); a
  per-key request log (Stripe's audit-next-to-scope pattern is how
  integration authors actually discover their minimal grant).
- **In parallel, at product pace — Option D:** document the bot-account
  path as the high-assurance tier (it exists today via `anytype-cli`) and
  steer autonomous-agent setups there; weigh a desktop "agent account"
  story on its own schedule.
- **P3 — type-scoped reads: don't.** If demand materializes, ship the
  default-filter reframing from §6.2 and say what it is.

## 8. Open questions

1. Is **v2-only** for scoped keys acceptable product-wise (v1 integrations
   would need to migrate to benefit)? Recommended yes; the alternative adds
   the v1 space-confusion fixes (§2.3) to the critical path.
2. Who owns the **consent UI**, and can the grant picker ride the already-
   in-flight challenge-flow rework? This is the schedule-defining
   dependency, not the heart-side enforcement.
3. Does the grant carry **resource classes** (objects/schema/chats/files) in
   v1 of the feature, or only spaces × read/write? Each class multiplies
   consent-UI complexity; GitHub's precedent says classes are worth it
   eventually, not necessarily first.
4. Should the **unauthenticated gateway** get a ticket of its own? It
   undercuts any file-confidentiality story the API might imply, scoped keys
   or not — and the browser→localhost reachability class (§1) makes it a
   live exposure, not a hypothetical.
5. Does the **bot-account tier** (Option D) get productized on desktop, and
   when? Until then, do the docs steer autonomous-agent setups to
   `anytype-cli` bot accounts as the stronger boundary, with scoped keys as
   the desktop-convenience tier?

---

## 9. Decisions (2026-08-06)

Approach decided with Roman after review; the design spec lives at
`docs/superpowers/specs/2026-08-06-api-key-scoping-design.md`.

1. **Surface: v2-only.** Scoped keys are rejected on `/v1` (403 pointing at
   v2). The v1 space-confusion fixes (§2.3) stay off the critical path as
   ordinary bugs. (Open question 1 → closed.)
2. **Grant axes: spaces × read/readwrite.** Two verbs, container allowlist —
   the industry-consensus cell. The grant schema reserves room for types and
   resource classes without migration. (Open question 3 → closed: no
   resource classes in v1 of the feature.)
3. **Consent path: heart-first, decoupled.** Grant model, enforcement, proto
   fields, and `whoami` land now; grants are settable via the gRPC app-link
   surface (CLI / power users) immediately. The desktop grant picker rides
   the reworked challenge flow whenever that lands — heart does not block on
   it. (Open question 2 → closed for heart's part.)
4. **Type scoping: writes stay on the roadmap as P2**, pinned by
   `(spaceId, uniqueKey)`, built after P1 proves out. Read-side type scoping
   is declined per §6.2.
5. **P0 fixes (H1/H2/H4/H5, decide H3) are the first PRs of this effort** —
   landed before any grant code, each small and independently shippable.
6. **Deletes: own-output only.** Scoped keys will be allowed to delete only
   objects created via that key (likely mechanism: record the issuing key's
   `appHash` as a device-local objectstore detail at create time). Recorded
   in the spec for later implementation — v2 has no object-delete route yet.
7. **No existing key breaks (migration stance).** The scope gate is
   `/v2`-only — `/v2` has no shipped clients, so H2 closes where it matters
   and `/v1` behaves exactly as today. Legacy keys are grandfathered with
   deprecation signals (RFC 9745 `Deprecation` header, log line, desktop
   badge); no sunset date committed yet.
8. **New keys are a new kind.** From the flip, JSON-API keys mint in a
   prefixed+checksummed format (secret-scannable) and the old format is
   never mintable again, so the legacy population shrinks by attrition.
   Crucially, **enforcement keys off the grant, not the format** — so a
   grant can be attached to an existing legacy key in place, scoping it
   without redistributing a secret. `Limited` keys are out of scope of this:
   they are a gRPC/clipper credential, not a JSON-API key.
9. Standing recommendations not otherwise decided: file the unauthenticated
   gateway as its own ticket (open question 4); docs steer autonomous-agent
   setups to `anytype-cli` bot accounts as the stronger boundary (open
   question 5), desktop productization at product pace.

## 10. Sources

Primary docs checked during the survey (claims tagged [K] above were *not*
re-verified against these). Grouped by subject; dates are publication dates
where stated.

**Fine-grained keys.** GitHub fine-grained PATs: the Oct 18 2022 launch
post + changelog, the Oct 18 2024 rotation-policy/optional-expiration
changelog, the Mar 18 2025 GA changelog, the
`permissions-required-for-fine-grained-personal-access-tokens` reference,
and community discussion #36441 (feedback). Stripe restricted keys:
`docs.stripe.com/keys/restricted-api-keys`, `docs.stripe.com/keys`.
Airtable PATs: `support.airtable.com` create-PAT doc,
`airtable.com/developers/web/api/scopes` + `/authentication`, and the
Feb 1 2024 legacy-API-key shutoff announcement. Linear: the scoped-key
changelog (attributed Apr 10 2025), OAuth + actor-authorization docs. Figma:
`developers.figma.com/docs/rest-api/scopes/` + `/authentication/`. Slack:
"More precision, fewer restrictions" (Jan 21 2020) + token/rotation docs.
Discord: `discord.com/developers/topics/permissions`. OpenAI:
`help.openai.com` API-key-permissions + projects articles.

**Per-type counterexamples.** Salesforce: least-privilege Integration User
release note + "Best Practices for Configuring Your Integration User"
(2023), and support threads on `INSUFFICIENT_ACCESS_ON_CROSS_REFERENCE_ENTITY`
/ "No such column." PostgREST/Supabase:
`supabase.com/changelog/29260` (2025 API-key rework),
`supabase.com/docs/guides/getting-started/api-keys`, PostgREST resource-
embedding docs. Strapi: `docs.strapi.io/cms/features/api-tokens`, the v4.4
custom-token announcement (2022), issue #22839 (Feb 2025). Directus:
permissions + tokens docs. **Sanity**: content-lake roles doc (the "joins
or reference traversal in filter GROQ … cannot be expressed" limitation).
**Kubernetes RBAC**: `resourceNames` + the list/watch field-selector caveat
(type scoping is clean only because resources don't join). Firebase:
security-rules docs (expressive per-resource policy; misconfiguration at
scale is a well-known operational hazard). Microsoft Graph:
`learn.microsoft.com/graph/permissions-selected-overview` (`Sites.Selected`
rationale quote). **Notion**: relation-to-unshared-container returns bare
undereferenceable ids (community reports); capabilities-are-a-ceiling +
403 `restricted_resource` / 404 `object_not_found` docs.

**Token tech.** RFC 9396 (RAR, Jan 2023); Fly.io macaroon posts + lnd
`bakemacaroon` docs; biscuitsec.org; SpiceDB/OpenFGA docs.

**Local-first peers.** Obsidian Local REST API plugin (all-or-nothing vault
key; path-handling has been a documented weak point); Home Assistant
long-lived tokens (architecture#67 non-goal), per-entity Assist exposure,
and the MCP-server integration reusing it; Raycast extensions security doc
+ AI-extension per-tool consent; macOS TCC / security-scoped bookmarks +
Powerbox [K]. `anyproto/anytype-cli` README (bot accounts) and
`anyproto/anytype-mcp`; developers.anytype.io auth docs + changelog; the
community.anytype.io key-scope feature thread (Apr 2026).

**Agent guidance & practice.** Simon Willison "lethal trifecta" (Jun 16
2025) and his MCP write-ups; Invariant Labs and General Analysis MCP
security posts (2025); Supabase "Defense in Depth for MCP Servers" (Sep 16
2025, documenting read-only mode + project scoping + feature groups); the
documented browser→localhost reachability class (2024); GitHub community
#36441 and the read-only-vs-reachable-surface behavior reports; MCP spec
authorization revisions 2025-06-18 / 2025-11-25 /
2026-07-28 incl. the Scope Minimization + token-passthrough-forbidden
sections (`modelcontextprotocol.io`); OWASP LLM06:2025 Excessive Agency +
the Agentic-Applications guide; CaMeL (arXiv 2503.18813) and "Design
Patterns for Securing LLM Agents" (arXiv 2506.08837); Anthropic "How we
contain Claude" (May 2026) + Claude Code security docs; GitHub MCP server
`scope-filtering.md`; Linear `actor=app` + readonly MCP endpoint; Stripe
MCP restricted-key guidance. 