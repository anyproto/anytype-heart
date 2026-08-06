# API v2 — the remaining v1 surfaces (spaces, members, files, chats, auth, tails)

Status: decision draft v0.1 · 2026-08-06 · GO-7383 follow-on to `core/api/APIV2.md` (v0.4).
Scope: everything `/v1` serves that `/v2` does not yet. Evidence is the shipped
route table (`core/api/server/router.go`) and the v1/v2 handler+service source;
every claim below carries a file:line ref.

## Verdict

No remaining surface needs a redesign-for-agents (d). The object surface was
redesigned because its *representation* was wrong for models; the remaining
surfaces are mostly small CRUD tails whose v1 shapes are serviceable. What they
need is **thin adaptation (c) where the agent loop actually runs through them**
— chats above all, where v1 silently drops the unread/state fields the
underlying RPC already returns (a correctness trap for a polling agent, not an
aesthetic) — and **plain reuse (a) for bootstrap and admin tails** (auth,
byte-stream download, tag/template/member administration). The forcing
function is §6's deprecation clock: anything the v2 *agent loop* depends on
must have a /v2 home before the CLI ships and the clock starts; the admin
tails can stay on /v1 and expire with it. Two small phases cover it: Phase 6
(chats) and Phase 7 (periphery: space create/update, `members/me`, the v2
search file-layout opt-in).

| Surface | Rec. | One line why |
|---|---|---|
| Auth | (a) | Challenge/key flow is version-neutral plumbing done once by the harness author, never by the model; the only v1-ism is the path prefix. |
| Spaces | (c) | List shipped in v2; add thin GET-one/POST/PATCH — v1's shapes are fine but its list does N+1 RPCs and misses every v2 convention. |
| Members | (c) | List shipped in v2; the one real gap is `GET /members/me` (already a named §3 build item); member admin is disabled even in v1. |
| Files | (c) | Upload shipped in v2; download stays v1 (byte stream, conventions don't apply); delete rides the pending v2 object-archive item; the real gap is v2 search can't see file objects. |
| Chats | (c) | The model (order-id cursor, message CRUD, SSE) is right; v1 drops chatState/messageCount on the floor and its rows/marks are token-hostile — passthrough + compact shapes, not a new model. |
| Lists (v1 `/lists`) | — | Superseded by Phase-4 sets/collections; nothing to do. |
| Tags admin | (a) | Rename/recolor/delete of options is rare curation; v2's names-as-identity makes rename awkward — keep v1 for the window. |
| Templates read | (a) | v1 list/get works; v2 create ships; agent demand unproven — revisit with the wrapper. |

---

## 1. Auth — (a) reuse v1 as-is

**What exists.** Two unauthenticated routes registered under `/v1`
(`router.go:329-335`): `POST /v1/auth/challenges` (app name → challenge id +
4-digit code shown in Desktop) and `POST /v1/auth/api_keys` (challenge id +
code → bearer key), backed by `AccountLocalLinkNewChallenge` /
`AccountLocalLinkSolveChallenge` with scope `AccountAuth_JsonAPI`
(`service/auth.go:19-52`). Every authenticated request — v1 and v2 alike —
passes the same `ensureAuthenticated` middleware, which exchanges the key for
a session token via `WalletCreateSession` and caches it
(`server/middleware.go:60-101`). The `/v2` group already mounts that
middleware (`router.go:85`); APIV2.md §8 declared "auth is shared with v1 —
no new auth surface" and the code matches.

**Agent workflow.** The challenge flow is inherently human-in-the-loop (the
4-digit code appears in the Desktop app) and runs once, at harness setup. No
model ever authors these two calls; token economy and C2-C13 are irrelevant
to them. Nothing v1-specific is baked into the *mechanism* — only the URL
carries `/v1`.

**Recommendation.** Reuse. Document `/v1/auth/*` in the v2 docs as the
version-neutral bootstrap. Do not mount an alias under `/v2` today: it would
put snake_case bodies (`challenge_id`, `app_key` — `model/auth.go`) inside
the C2 camelCase namespace, or fork the shape for two field names' sake. The
one commitment to record: **`/v1/auth` outlives v1 deprecation** (or moves in
its own migration) — it is the account-link protocol, not part of the v1
resource surface. See open question Q1.

## 2. Spaces — (c) thin adaptation, mostly shipped

**What v1 has.** `GET /v1/spaces` (list), `GET /v1/spaces/{id}`, `POST
/v1/spaces`, `PATCH /v1/spaces/{id}` (`router.go:536-556`). The v1 list is
expensive by construction: per space-view row it calls `WorkspaceOpen` +
`ObjectShow` (`service/space.go:88-94` → `getSpaceInfo`, `space.go:212-250`)
— N+1 RPC pairs to render name/icon/description. Create routes through
`WorkspaceCreate` (hardcoded `CHAT_SPACE` use case, random icon option —
`space.go:136-174`); update through `WorkspaceSetInfo`. The v1 `Space` model
carries `gateway_url` and `network_id` (`model/space.go:17-25`).

**What v2 has.** `GET /v2/spaces` shipped (`router.go:94`), one store query
over tech-space space views, rows `{id, name}` (`service/v2_discovery.go:26-53`,
`model/v2.go:124-128`). No get-one, no create, no update.

**Agent workflow.** (1) Orientation: "which space am I in / which spaces
exist" — the shipped list answers it. (2) A description is genuinely useful
orientation signal for multi-space accounts; today v2 has no way to read it.
(3) Creating a scratch/project space is a real, occasional agent task; two
calls into v1's shape work but miss idempotency (an auto-retried space create
duplicates a whole space — the worst possible duplicate) and the C6 error
shape. `gatewayUrl`/`networkId` are client-infrastructure fields, not agent
fields — leave them out of v2 rows (they remain reachable via v1).

**v2 additions** (C6/C8/C9/C10 semantics throughout):

```
GET   /v2/spaces/{spaceId}            → {"id","name","description"}
POST  /v2/spaces                      body {"name", "description"?} → the same shape
PATCH /v2/spaces/{spaceId}            body {"name"?, "description"?} → the same shape
```

Create/patch are thin over `WorkspaceCreate`/`WorkspaceSetInfo` exactly as
v1; `Idempotency-Key` honored (C8), `dry_run` validates the body only (C9 —
a space create cannot be simulated). No v2 space delete: v1 has none either,
and space deletion is an account-level operation the local API should not
casually offer. The "space orientation one-shot" is deliberately **not**
specced here — it is a real fork, see Q2.

## 3. Members — (c) thin adaptation, one endpoint of substance

**What v1 has.** `GET /v1/spaces/{id}/members` (active + joining, two
sequential ObjectSearches — `service/member.go:19-103`), `GET
…/members/{member_id}` resolving by participant id *or* raw identity
(`member.go:106-143`). The write path (approve/decline/remove/role) exists in
service code (`member.go:146-208`) but its route is **commented out** pending
granular permissions (`router.go:459-464`) — there is no live member-write
surface in v1 today.

**What v2 has.** `GET /v2/spaces/{spaceId}/members` shipped
(`router.go:106`): active participants, minimal rows `{id, name, role,
identity}` (`v2_discovery.go:57-99`).

**Agent workflow.** Members exist for one reason in the agent loop: object
property values (`assignee`, `creator` hold participant ids). The shipped
list serves that. The missing piece is self-identity — `@me` in filters and
"assign to me" — which only the server knows; APIV2.md §3 already names
**`GET /v2/spaces/{spaceId}/members/me`** as a Phase-5 build item (the same
identity Phase 4's placeholder substitution uses, `v2_list_read.go:453-467`
via `V2Deps.AccountId`). Shape:

```
GET /v2/spaces/{spaceId}/members/me   → {"id","name","role","identity"}
```

Not building: get-one-member (the paginated list covers realistic space
sizes; a compact row is ~90 tokens), and member administration — v1 itself
keeps it disabled, and approving join requests is a human trust decision, not
an agent task. If the v1 route is ever re-enabled, agents can be pointed at
it; do not pre-build a v2 twin of a surface v1 doesn't trust yet.

## 4. Files — (c) thin, and mostly already decided

**What v1 has** (`router.go:403-423`): `POST /files` (multipart, staged to a
temp path — `handler/file.go:157-229`), `GET`/`HEAD /files/{id}` streaming
bytes with range/conditional support and image width variants + SVG
sanitization (`handler/file.go:38-76`, `service/file.go:68-162`), `DELETE
/files/{id}` = archive-or-purge of the file *object*
(`ObjectSetIsArchived`/`ObjectListDelete`, `service/file.go:189-208`).

**What v2 has.** `POST /v2/spaces/{spaceId}/files` shipped — multipart *or*
`{"url": …}`, returns `{id, name, mimeType, size}`, stamps `origin: api`
(`service/v2_file.go:21-50`, route `router.go:264`). APIV2.md already calls
it load-bearing (R11): file/image blocks and `iconImage` need the id.

**Agent workflow, honestly assessed.** Agents deal in text; what they do with
files is (1) mint an id to place in a block or icon — shipped; (2) attach an
*existing* file — requires finding it; (3) hand bytes to the harness (save an
attachment, show an image to the user) — a tooling concern, not a model
concern; (4) "read this PDF" — **no server capability exists**: the FT
indexer treats file layouts specially but indexes relation values, not
content (`core/indexer/fulltext.go:52-57`), and the `FileObjectService` port
serves bytes only (`core/api/core/core.go:35-39`). Do not invent (d) scope
for a capability the middleware does not have (Q7).

**Decisions.**
- **Download: reuse v1 (a).** It is a byte stream with HTTP-native semantics
  (ranges, conditional GET); none of C2-C13 applies to it. Document
  `GET /v1/spaces/{id}/files/{fileId}?width=` as the transport endpoint.
- **Delete: no v2 file route.** A file object is an object; the pending
  `DELETE /v2/spaces/{spaceId}/objects/{objectId}` archive (§3 build item,
  due before Phase 5) covers it — v1's own DeleteFile is exactly that RPC.
- **The real gap — discovery (build item).** v2 search and ListObjects scope
  rows to `util.ObjectLayouts`, which excludes file layouts
  (`service/v2_search.go:353`, `service/v2_object.go:426`,
  `util/constant.go:9-18`); v1 search has an explicit opt-in
  (`prepareBaseFilters(includeFileLayouts)`, `service/search.go:178-186`).
  So a pure-v2 agent can upload an image but can never find it again.
  Fix inside the existing Phase-4 grammar: naming a file type in the type
  channel (`type = "image"`, top-level `type: "file"`, …) widens the layout
  scope to `ObjectAndFileLayouts` for that query — the v1 opt-in reproduced
  without a new parameter. Rows come back C5-minimal with `mimeType`/`size`
  available via `fields=`.

## 5. Chats — (c) thin adaptation with three real reshapes

The one surface where "thin" still means design work. Read the machinery
first; the conclusion is that **v1's chat *model* is right and its chat
*shapes* leak or drop exactly the fields an agent needs**.

**What v1 has** (`router.go:338-400`, 13 routes): list/create chats;
get-messages with order-id cursor (`before_order_id`/`after_order_id`,
limit ≤1000 — `handler/chat.go:126-153`); get-one; add/edit/delete message;
toggle reaction; read-all / read-range / read-reactions; per-chat FT search
(`ChatSearch`); an SSE stream (last N + live `message_added/updated/deleted`,
`reactions_updated`, heartbeats — `handler/chat_stream.go:63-135`).

**What the middleware can do that v1 hides.** `ChatGetMessages` returns
`chatState` — unread messages *and* mentions `{oldestOrderId, counter}`,
`unreadReactionOrderId`, `lastStateId` — plus `messageCount`
(`pb/protos/commands.proto:9179-9205`;
`pkg/lib/pb/model/protos/models.proto:1638-1648`). The v1 service **drops
both** and returns bare messages (`service/chat.go:88-103`). Three concrete
consequences for a polling agent:

1. **"Anything new?" costs a page, not a peek.** With no counters on any read
   shape, the agent must fetch messages and diff to learn there is nothing.
2. **Mark-read has a race v1 makes unavoidable.** `ReadChatMessagesRequest`
   accepts `last_state_id` precisely to prevent marking messages the client
   has not seen (`model/chat.go:64-69`, proto comment `commands.proto:9344`)
   — but **no v1 response ever carries a state id** (the message DTO omits
   `stateId`; the SSE converter drops `ChatStateUpdate` events —
   `model/chat.go:279-314`). The correctness affordance exists server-side
   and is unreachable through the API.
3. **Token-hostile shapes.** `ListChats` rows are the full v1 Object — the
   embedded `*Type` object and complete properties array
   (`service/chat.go:72-74` → `object.go:415-431`), the C5-banned
   multiplier. Message reads carry a ~120-char participant id per message
   plus identity-list reaction maps; marks are offset-based
   (`model/chat.go:28-33`) — the exact trap (offset arithmetic in model
   space) the block surface eliminated via §8 inline markup.

Sending is fine: one round trip, returns `message_id`
(`service/chat.go:128-144`). The SSE stream is fine for harnesses that hold
a connection; a stateless polling agent needs the counters instead.

**Not (d), because:** the primitives (order-id cursor pagination, id-addressed
message CRUD, toggle reactions, read watermarks) are already agent-shaped —
they map 1:1 onto what a redesign would specify. Everything wrong is at the
DTO layer. That is the definition of (c).

**v2 chat surface** (all C6 errors; C8 `Idempotency-Key` on every mutation —
a double-sent chat message is user-visible damage; C9 `dry_run` =
validate-only; C7 etag/If-Match **does not apply** — order ids and
`lastStateId` are the stream's native concurrency vocabulary, documented as
a deliberate exemption like search's C8/C9 one):

```
GET    /v2/spaces/{spaceId}/chats                       # C5 rows {id,name} — store query, no chat opens
POST   /v2/spaces/{spaceId}/chats                       # {name} → row (thin over ObjectCreate, v1 parity)
GET    /v2/spaces/{spaceId}/chats/{chatId}/messages     # ?after=&before=&limit=25
POST   /v2/spaces/{spaceId}/chats/{chatId}/messages     # {text, replyTo?, attachments?:[fileId…]} → {id}
PATCH  /v2/spaces/{spaceId}/chats/{chatId}/messages/{messageId}   # {text}
DELETE /v2/spaces/{spaceId}/chats/{chatId}/messages/{messageId}
POST   /v2/spaces/{spaceId}/chats/{chatId}/messages/{messageId}/reactions   # {emoji} → {added:bool}
POST   /v2/spaces/{spaceId}/chats/{chatId}/read         # {upTo, lastStateId?, scope?:"messages"|"mentions"}
```

GET messages response (compact JSON, C3):

```json
{"messages":[
   {"id":"…","order":"…","author":"Alice","authorId":"_participant_…",
    "at":1717405200,"text":"can you **check** the doc?","replyTo":"…",
    "reactions":{"👍":2},"attachments":[{"id":"bafy…","type":"image"}]}],
 "state":{"unreadMessages":3,"unreadMentions":1,"oldestUnreadOrder":"…","lastStateId":"…"},
 "messageCount":812}
```

The three reshapes that carry the phase:

- **State passthrough.** `state` + `messageCount` on every messages read —
  zero extra RPC cost (the fields are already in the response the service
  throws away). This closes both the peek problem (poll = `limit=1` read)
  and the mark-read race (`lastStateId` finally reaches the client;
  `POST …/read` forwards it).
- **Text is §8 inline markup, both directions.** Read: marks render into the
  text via the anyblockjson inline codec (the same serialization block text
  uses — one vocabulary, C2); write: `text` parses as markdown source
  exactly like `replaceText`/`insertBlocks` payloads (the D′1 caveat applies
  verbatim and is documented on the endpoint). Offset mark arrays never
  cross the API. `style` is dropped from the default read (it is
  `"paragraph"` in practice) and not accepted on write for now.
- **C5 rows and compact reactions.** Chat rows are `{id,name}` (the chat
  *object* remains visible in object search — `chatDerived` is in
  `util.ObjectLayouts` — but its document body is empty: messages live in
  the chat store, not blocks). Reactions default to counts
  (`{"👍":2}`); `?reactions=full` restores identity lists.

**Reused from v1 during the window (a):** the SSE stream (an event channel
has no v2-convention delta worth re-mounting; harness-level consumers only)
and per-chat FT search (`…/messages/search`). Both are candidates to move
under /v2 unchanged (b) before the deprecation clock if the mixed-client rule
(Q8) demands it. Deliberately absent: per-chat unread counters on the *list*
— computing them means opening every chat, the exact cost GO-7302 removed
from startup; see Q3.

## 6. Residual v1 surfaces — nothing to build

- **Lists** (`/v1/spaces/{id}/lists/*`, `router.go:426-446`): superseded by
  the shipped Phase-4 sets/collections reads and the Phase-3
  `addItems`/`removeItems` ops. v1 keeps serving legacy clients until
  deprecation. Done.
- **Tags** (`/v1/…/properties/{id}/tags/*`, `router.go:559-584`): v2 reads
  options (`GET /properties/{key}/options`) and creates them by name
  (create-missing, R9). What v1 alone offers is id-addressed rename /
  recolor / delete (`service/tag.go:103-208`). That is curation, not an
  agent loop; and v2's names-as-identity makes "rename" a genuinely
  different operation (the identity changes under every object that carries
  it). Reuse v1 (a); revisit only on demand (Q5).
- **Templates** (`/v1/…/types/{id}/templates`, `router.go:587-597`): v2 can
  create templates (`POST /v2/…/templates`) and read one by id (GET object),
  but cannot list them — v2 search excludes template rows by design (§8.4
  base scope). v1's list/get stand (a). If the wrapper ever grows a
  create-from-template flow, a `GET /v2/…/types/{type}/templates` list is a
  half-day build over the same store query v1 uses (Q6).

## 7. Cross-cutting: the mixed /v1+/v2 client, stated plainly

A v2 client that calls /v1 for auth, byte downloads, and admin tails is
**acceptable — with a rule**. The coherent-API story agents actually need is
"one OpenAPI document whose examples all work"; that is satisfied by the v2
spec *documenting* its three /v1 dependencies (auth bootstrap, file
download, SSE stream) as named, versioned exceptions. What would break the
story is the agent *loop* straddling versions — different error shapes,
pagination, and vocabulary inside one task. Hence the rule this document is
built around: **everything a task loop touches (objects, query, chats core,
file upload+discovery, space/member orientation) lives under /v2 before the
CLI ships and §6's deprecation clock starts; what stays on /v1 is only
bootstrap, byte transport, and administration** — surfaces where either HTTP
itself is the convention or a human is in the loop. When v1 is eventually
removed, the survivors (auth, download, stream) are re-homed as their own
small migration, not blockers today.

**Phase-5 relevance.** The wrapper's ~10 tools (§7.2) touch none of the
surfaces above except `GET /v2/spaces` (shipped) and the `@me`/`members/me`
identity (already a named Phase-5 item — this document just confirms its
shape). Chats and files do not appear in the tool set, so nothing in these
recommendations changes the wrapper's design. If chat tools are wanted later
(`chat_read`/`chat_post` are the obvious pair, and the Phase-6 shapes above
are deliberately flat enough to back them), they belong in a separate
profile/manifest — the >15-tool cliff (§7) rules out widening the core set.

## 8. Phase plan

### Phase 6 — chats for agents (sized like Phase 4: one surface, one design decision, mostly translation)

1. **[build]** v2 chat DTOs + inline-markup bridge: marks ↔ §8 markup text
   via the anyblockjson inline codec, both directions; reactions compaction;
   author name enrichment reusing the participant cache
   (`service/chat.go:111-126`).
2. **[build]** `GET /chats` C5 rows (store query over `ChatLayouts`, no chat
   opens) + `POST /chats` (thin `ObjectCreate` with the `chatDerived` type —
   NOT the Phase-2 snapshot path, which has never been exercised for
   store-backed smartblocks).
3. **[build]** `GET /messages` with `state`+`messageCount` passthrough — the
   port already returns them (`core.go:159`); only the service/DTO layer
   changes.
4. **[build]** `POST`/`PATCH`/`DELETE` message + reactions toggle, with C8
   idempotency middleware on the routes and C9 validate-only dry runs.
5. **[build]** `POST /read` forwarding `{upTo, lastStateId, scope}` to
   `ChatReadMessages`/`ChatReadReactions`.
6. Docs: C7 exemption stated on every chat endpoint; D′1 markup caveat on
   `POST`/`PATCH` message; v1 SSE stream + chat search documented as the
   named exceptions.

Exit criterion (harness): a polling agent completes "summarize what's new in
this chat and mark it read" in ≤2 calls with zero message re-reads, at lower
token cost than the v1 flow, and a double-send retry is absorbed by C8.

### Phase 7 — periphery (small; half the size of Phase 6)

1. **[build]** Spaces: `GET /v2/spaces/{spaceId}`, `POST /v2/spaces`,
   `PATCH /v2/spaces/{spaceId}` (§2 shapes; C8 on both mutations).
2. **[build]** `GET /v2/spaces/{spaceId}/members/me` (§3 shape) — subsumes
   the §3/Phase-5 named item; land it here if Phase 5 has not already.
3. **[build]** v2 search/list file-layout opt-in keyed off the type channel
   (§4) + `mimeType`/`size` in the `fields=` vocabulary.
4. Restated, not re-budgeted (already §3 build items): `DELETE /v2/objects`
   archive (covers file delete), `GenerateSchema`.

## 9. Open questions — decisions needed from a human

- **Q1 · Auth path.** Keep `/v1/auth/*` as the documented version-neutral
  bootstrap (recommended: zero code, one ugly URL) — or alias it under
  `/v2/auth` unchanged, accepting snake_case bodies inside the v2 namespace?
  The alias buys nothing functional; it buys the "client never types /v1"
  aesthetic. Deciding factor: whether the eventual v1 removal plan treats
  auth as its own protocol (then keep /v1/auth forever) or not.
- **Q2 · Space orientation one-shot.** A `GET /v2/spaces/{spaceId}/context`
  returning `{space, me, types[], propertyKeys[]}` would collapse the
  cold-start 3-4 calls into one (~1 s of round trips, a few hundred tokens).
  Against it: it duplicates three shipped discovery lists behind a second
  cache-staleness surface, and the wrapper's `describe` flow already covers
  the per-type half. Recommendation: defer until the Phase-0 harness measures
  the wrapper's cold-start; build only if orientation calls dominate turns.
  This is the one candidate where "agent-shaped addition" is plausibly real.
- **Q3 · Unread counters on the chat list.** Options: (i) list stays
  counter-free; agents poll the chats they care about (recommended — per-chat
  state is free on the messages read, and computing list-wide counters means
  opening every chat, the GO-7302 startup cost, on every poll); (ii) opt-in
  `?state=true` that pays the opens explicitly; (iii) a new middleware
  aggregate keeping counters store-side. (iii) is the only *good* UX and the
  only expensive one — it is middleware work, not API work.
- **Q4 · Reactions default.** Counts-by-default (`{"👍":2}`,
  `?reactions=full` for identities) — or identity lists for v1 parity?
  Recommended: counts; an agent almost never needs *who* reacted, and
  identity lists are the single largest token line in a busy chat read.
- **Q5 · Tag/option administration.** Leave rename/recolor/delete on v1
  until deprecation (recommended), or spec
  `PATCH/DELETE /v2/…/properties/{key}/options/{name}` now and resolve the
  rename-changes-identity semantics (rename = create+migrate+delete under
  names-as-identity, or an id-addressed escape hatch that reintroduces the
  id/name duality C2 banned)?
- **Q6 · Template listing under v2.** Build `GET /v2/…/types/{type}/templates`
  (trivial) — or wait for a demonstrated create-from-template agent flow?
  Recommended: wait; v2 create takes full bodies, so templates currently buy
  an agent nothing it cannot inline.
- **Q7 · File content extraction.** "Read this PDF" has no backing
  capability anywhere in the middleware (FT indexes relation values only).
  Building extraction is a middleware project with its own owner — decide
  whether it enters the v2 roadmap at all; the API-shaped part
  (`GET /files/{id}/text` with C11 warnings) is trivial once a service
  exists. Until then the honest answer is "download the bytes via v1 and
  extract harness-side".
- **Q8 · The mixed-client rule.** §7's position — /v1 allowed for bootstrap,
  byte transport, and admin only; everything in the task loop on /v2 before
  the CLI ships — needs sign-off, because it is what makes Phase 6/7 the
  *complete* remaining scope rather than an installment.
