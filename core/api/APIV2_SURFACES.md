# API v2 — the remaining v1 surfaces (spaces, members, files, chats, auth, tails)

Status: decision v0.2 · 2026-08-06 · GO-7383 follow-on to `core/api/APIV2.md` (v0.4).
Scope: everything `/v1` serves that `/v2` does not yet. Evidence is the shipped
route table (`core/api/server/router.go`) and the v1/v2 handler+service source;
every claim below carries a file:line ref.

> **v0.2 — the completeness decision (human, 2026-08-06).** The mixed-client
> rule proposed in v0.1 (§7, Q8) is **rejected**. v2 is to be a *complete,
> self-contained API*: auth, file download, chats, streaming and the admin
> tails all get a `/v2` home, and a v2 client never types `/v1`. The stated
> reason is documentation coherence — one API that can be documented cleanly,
> end to end, for both agents and human users. §§1-8 below record the
> per-surface evidence, which is unchanged and still load-bearing; the
> *recommendations* they reach are superseded by §10, which converts them into
> a completeness plan. Where v0.1 argued "reuse, the seam is harmless", the
> counter-argument that won is that a seam is cheap to cross and expensive to
> *explain* — every exception costs a paragraph in the docs, an example that
> works differently, and a reader who now has to know which half they are in.

## Verdict

No remaining surface needs a redesign-for-agents (d) — that finding stands.
The object surface was redesigned because its *representation* was wrong for
models; the remaining surfaces are small CRUD tails whose v1 shapes are
serviceable, so the work is **translation, not redesign**. What the v0.2
decision changes is the *extent*: every surface gets a v2 home, including the
three v0.1 wanted to leave behind (auth bootstrap, byte download, SSE stream).

That makes the remaining work three phases rather than two — Phase 6 (chats),
Phase 7 (periphery), Phase 8 (completeness: auth, download, stream, admin
tails) — and it makes §6's deprecation clock meaningful for the first time: on
Phase 8 exit, `/v1` has no unique capability left, so it can be deprecated
whole rather than in pieces.

One caveat the decision inherits, stated here so it is not rediscovered later:
**completeness is not parity.** Two v1 behaviors should NOT be reproduced —
v1's `total = len(fetched)` (already banned by Phase-4 rule 4) and its
snake_case auth bodies (§10.1). "A v2 home for every capability" is the goal;
"the same shape at a new URL" is not.

| Surface | v0.1 Rec. | v0.2 (decided) | One line |
|---|---|---|---|
| Auth | (a) reuse | **issuance dropped (2026-08-07); v2 owns consumption** | No v2 minting endpoint by design — keys are issued in the app over gRPC. v2 owns the scope gate, the per-space grant and `GET /v2/auth/whoami`. |
| Spaces | (c) | **(c)** unchanged | List shipped; add GET-one/POST/PATCH — v1's list does N+1 RPCs and misses every v2 convention. |
| Members | (c) | **(c)** unchanged | List shipped; the real gap is `GET /members/me`; member admin is disabled even in v1 — nothing to port. |
| Files | (c), download stays v1 | **(c) incl. download** | Upload shipped; download gets `/v2` bytes (HTTP conventions still apply *around* the stream); the search file-layout blindness is the live bug. |
| Chats | (c) | **(c)** unchanged, incl. SSE | v1 drops chatState/message_count the RPC already returns; rows/marks are token-hostile — passthrough + compact shapes, and the stream comes too. |
| Lists (v1 `/lists`) | — | — | Superseded by Phase-4 queries/collections; nothing to do. |
| Tags admin | (a) | **(c)**, Phase 8 | Rename semantics under names-as-identity must be resolved (Q5), not dodged. |
| Templates read | (a) | **(b/c)**, Phase 8 | Trivial to port; ports for completeness rather than demonstrated demand. |

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

> **STATUS: BUILT** (Phase 7, 2026-08-06 — decisions as built in APIV2.md
> §8.8). The N+1 claim below verified. One deviation from v1's mechanics:
> the description rides the ONE WorkspaceCreate call (CreateWorkspace
> applies every detail), dropping v1's second WorkspaceSetInfo RPC. The
> get-one read is the tech-space space view (it mirrors name AND
> description), so it costs zero RPCs.

**What v1 has.** `GET /v1/spaces` (list), `GET /v1/spaces/{id}`, `POST
/v1/spaces`, `PATCH /v1/spaces/{id}` (`router.go:536-556`). The v1 list is
expensive by construction: per space-view row it calls `WorkspaceOpen` +
`ObjectShow` (`service/space.go:88-94` → `getSpaceInfo`, `space.go:212-250`)
— N+1 RPC pairs to render name/icon/description. Create routes through
`WorkspaceCreate` (hardcoded `CHAT_SPACE` use case, random icon option —
`space.go:136-174`); update through `WorkspaceSetInfo`. The v1 `Space` model
carries `gateway_url` and `network_id` (`model/space.go:17-25`).

**What v2 has.** `GET /v2/spaces` shipped (`router.go:94`), one store query
over tech-space space views, rows `{id, name}` (`v2/service/discovery.go:26-53`,
`v2/model/model.go:124-128`). No get-one, no create, no update.

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
GET   /v2/spaces/{space_id}            → {"id","name","description"}
POST  /v2/spaces                      body {"name", "description"?} → the same shape
PATCH /v2/spaces/{space_id}            body {"name"?, "description"?} → the same shape
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

**What v2 has.** `GET /v2/spaces/{space_id}/members` shipped
(`router.go:106`): active participants, minimal rows `{id, name, role,
identity}` (`v2_discovery.go:57-99`).

**Agent workflow.** Members exist for one reason in the agent loop: object
property values (`assignee`, `creator` hold participant ids). The shipped
list serves that. The missing piece is self-identity — `@me` in filters and
"assign to me" — which only the server knows; APIV2.md §3 already names
**`GET /v2/spaces/{space_id}/members/me`** as a Phase-5 build item (the same
identity Phase 4's placeholder substitution uses, `v2_list_read.go:453-467`
via `V2Deps.AccountId`). Shape:

```
GET /v2/spaces/{space_id}/members/me   → {"id","name","role","identity"}
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

**What v2 has.** `POST /v2/spaces/{space_id}/files` shipped — multipart *or*
`{"url": …}`, returns `{id, name, mimeType, size}`, stamps `origin: api`
(`v2/service/file.go:21-50`, route `router.go:264`). APIV2.md already calls
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
- **Delete: no dedicated v2 file route.** A file object is an object; the
  built `DELETE /v2/spaces/{space_id}/objects/{object_id}` archive covers
  API-key-created files under its own-output-only provenance rule. v1's own
  DeleteFile uses the same archive RPC.
- **The real gap — discovery (BUILT, Phase 7 — APIV2.md §8.8).** The gap
  was: the v2 query surface scoped rows to `util.ObjectLayouts`, which
  excludes file layouts (`util/constant.go:9-18`), while v1 search has an
  explicit opt-in (`prepareBaseFilters(includeFileLayouts)`,
  `service/search.go:178-186`) — so a pure-v2 agent could upload an image
  and never find it again. What was widened is **search only**
  (`v2/service/search.go appendBaseRowScope`): naming a file type in the
  type channel (`type = "image"`, top-level `type: "file"`, …) widens the
  layout scope to `ObjectAndFileLayouts` for that query — the v1 opt-in
  reproduced without a new parameter. **ListObjects keeps the narrow
  `ObjectLayouts` scope by design** (`v2/service/object.go` — it has no
  type channel and deliberately gains none; file discovery is search's
  job), and the queries/collections reads never had the layout scope at all,
  so a set over a file type already returned its rows. Rows come back
  C5-minimal with `mimeType`/`size` available via `fields=` (and, post
  review, as filter/sort keys — APIV2.md §8.8).

## 5. Chats — (c) thin adaptation with three real reshapes

> **STATUS: BUILT** (Phase 6, 2026-08-06 — decisions as built in APIV2.md
> §8.7). The evidence below held under re-verification, with two backend
> facts the plan could not see: `ChatReadReactions` ignores its order id
> (`core/chats.go:325`) so the reactions read scope is all-or-nothing, and
> the edit RPC replaces the whole message content so PATCH is a read-merge.
> Q3 resolved (i) counter-free list; Q4 resolved counts-by-default. The SSE
> stream and per-chat FT search remain on v1 until Phase 8, as planned.

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
`unreadReactionOrderId`, `last_state_id` — plus `message_count`
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
`last_state_id` are the stream's native concurrency vocabulary, documented as
a deliberate exemption like search's C8/C9 one):

```
GET    /v2/spaces/{space_id}/chats                       # C5 rows {id,name} — store query, no chat opens
POST   /v2/spaces/{space_id}/chats                       # {name} → row (thin over ObjectCreate, v1 parity)
GET    /v2/spaces/{space_id}/chats/{chat_id}/messages     # ?after=&before=&limit=25
POST   /v2/spaces/{space_id}/chats/{chat_id}/messages     # {text, reply_to?, attachments?:[fileId…]} → {id}
PATCH  /v2/spaces/{space_id}/chats/{chat_id}/messages/{message_id}   # {text}
DELETE /v2/spaces/{space_id}/chats/{chat_id}/messages/{message_id}
POST   /v2/spaces/{space_id}/chats/{chat_id}/messages/{message_id}/reactions   # {emoji} → {added:bool}
POST   /v2/spaces/{space_id}/chats/{chat_id}/read         # {up_to, last_state_id?, scope?:"messages"|"mentions"}
```

GET messages response (compact JSON, C3):

```json
{"messages":[
   {"id":"…","order":"…","author":"Alice","author_id":"_participant_…",
    "at":1717405200,"text":"can you **check** the doc?","reply_to":"…",
    "reactions":{"👍":2},"attachments":[{"id":"bafy…","type":"image"}]}],
 "state":{"unread_messages":3,"unread_mentions":1,"oldest_unread_order":"…","last_state_id":"…"},
 "message_count":812}
```

The three reshapes that carry the phase:

- **State passthrough.** `state` + `message_count` on every messages read —
  zero extra RPC cost (the fields are already in the response the service
  throws away). This closes both the peek problem (poll = `limit=1` read)
  and the mark-read race (`last_state_id` finally reaches the client;
  `POST …/read` forwards it).
- **Text is §8 inline markup, both directions.** Read: marks render into the
  text via the anyblockjson inline codec (the same serialization block text
  uses — one vocabulary, C2); write: `text` parses as markdown source
  exactly like `replace_text`/`insert_blocks` payloads (the D′1 caveat applies
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
  the shipped Phase-4 queries/collections reads and the Phase-3
  `add_items`/`remove_items` ops. v1 keeps serving legacy clients until
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

### Phase 6 — chats for agents (BUILT 2026-08-06 — APIV2.md §8.7)

1. **[built]** v2 chat DTOs + inline-markup bridge (`v2/model/chat.go`):
   marks ↔ §8 markup text via the anyblockjson inline codec, both
   directions; reactions compaction (counts default, `?reactions=full` as
   participant ids); author name enrichment — store-backed via the
   deterministic participant id, NOT the v1 cross-space subscription cache
   (that cache is the v1 service's; the deviation is recorded in §8.7).
2. **[built]** `GET /chats` C5 rows (store query over `ChatLayouts`, no chat
   opens — the test fails on any RPC) + `POST /chats` (thin `ObjectCreate`
   with the `chatDerived` type; non-empty name required).
3. **[built]** `GET /messages` with `state`+`message_count` passthrough
   (`v2/service/chat.go`).
4. **[built]** `POST`/`PATCH`/`DELETE` message + reactions toggle, C8 on all
   (the middleware's method set widened to DELETE for the chat delete
   route), C9 dry runs (PATCH is a read-merge — the edit RPC replaces the
   whole content, a naive text forward would wipe attachments).
5. **[built]** `POST /read` forwarding `{up_to, last_state_id, scope}`; up_to
   required+inclusive for messages/mentions (an empty bound silently marks
   nothing); the reactions scope is all-or-nothing (the backend ignores its
   order id) and rejects bounds.
6. **[done]** Docs: C7 exemption on every chat endpoint; D′1 caveat on
   POST/PATCH message; v1 SSE stream + chat search named as exceptions;
   `chat`/`chatMessage`/`chatRead` discovery kinds (§5 rule: an authoring
   surface needs a schema kind).

Exit criterion (harness): a polling agent completes "summarize what's new in
this chat and mark it read" in ≤2 calls with zero message re-reads, at lower
token cost than the v1 flow, and a double-send retry is absorbed by C8.

### Phase 7 — periphery (BUILT 2026-08-06 — APIV2.md §8.8)

1. **[built]** Spaces: `GET /v2/spaces/{space_id}`, `POST /v2/spaces`,
   `PATCH /v2/spaces/{space_id}` (§2 shapes; C8 on both mutations — the
   router test pins both; PATCH additionally requires at least one field
   and a non-empty name).
2. **[already shipped]** `GET /v2/spaces/{space_id}/members/me` — verified
   Phase 5 landed it (route + `GetMemberMe` + tests); nothing rebuilt.
3. **[built]** the search file-layout opt-in keyed off the type channel
   (§4): top-level file `type` or a positive (`=`/`IN`) `type` filter
   leaf widens the row scope to `ObjectAndFileLayouts`, both request
   forms; negated leaves do not. `mimeType`/`size` joined the `fields=`
   vocabulary as display-only aliases of fileMimeType/sizeInBytes
   (search AND the queries/collections `?fields=`). Scoped to search:
   ListObjects has no type channel (§4's "without a new parameter"),
   and the queries/collections reads never had the layout scope, so queries
   over file types already worked.
4. Restated, not re-budgeted (already §3 build items): `DELETE /v2/objects`
   archive (covers file delete), `GenerateSchema`.

## 9. Open questions — decisions needed from a human

- **Q1 · Auth path — DECIDED, twice.** v0.2 (2026-08-06) moved the
  challenge/key pair to `/v2/auth/*`. **2026-08-07 supersedes it: v2 mints
  nothing.** Keys are issued in the app over gRPC; the HTTP API only consumes
  a bearer token, and v2 owns everything downstream of it — the scope gate,
  the grant, `whoami` (§10.1 item 1). The v0.1 instinct that auth was
  "version-neutral plumbing" turns out to have been half right: not because
  the URL prefix is harmless, but because *issuance does not belong to the
  HTTP API at all*.
- **Q2 · Space orientation one-shot.** A `GET /v2/spaces/{space_id}/context`
  returning `{space, me, types[], propertyKeys[]}` would collapse the
  cold-start 3-4 calls into one (~1 s of round trips, a few hundred tokens).
  Against it: it duplicates three shipped discovery lists behind a second
  cache-staleness surface, and the wrapper's `describe` flow already covers
  the per-type half. Recommendation: defer until the Phase-0 harness measures
  the wrapper's cold-start; build only if orientation calls dominate turns.
  This is the one candidate where "agent-shaped addition" is plausibly real.
- **Q3 · Unread counters on the chat list — DECIDED (Phase 6, as
  recommended): (i)**, the list stays counter-free; per-chat state is free
  on the messages read (a `limit=1` poll), and computing list-wide counters
  means opening every chat — the GO-7302 startup cost — on every poll.
  (iii), the store-side counter aggregate, remains the only good-UX option
  if list counters are ever demanded; it is middleware work, not API work.
- **Q4 · Reactions default — DECIDED (Phase 6, as recommended):
  counts-by-default** (`{"👍":2}`); `?reactions=full` restores identity
  lists, carrying participant ids (one vocabulary with `author_id`, C2),
  never raw identities.
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
- **Q8 · The mixed-client rule — DECIDED (v0.2): rejected.** v2 is complete
  and self-contained; a v2 client never types `/v1`. See the header note and
  §10. Q5 (tag rename semantics) and Q6 (templates) are consequently no longer
  "wait for demand" — they are Phase-8 build items, and Q5's rename semantics
  must actually be resolved. Q7 (file text extraction) is unaffected: it is a
  missing *capability*, not a missing endpoint, so completeness does not
  conjure it — `/v2` exposes the bytes and says so plainly.

## 10. The completeness plan (v0.2)

The three phases below replace §8's two. Phases 6 and 7 are unchanged in
content; Phase 8 is new and exists only because of the v0.2 decision.

### 10.1 Phase 8 — completeness: the surfaces v0.1 wanted to leave on v1

1. **~~Auth endpoints~~ — DROPPED (human decision, 2026-08-07). v2 does not
   mint keys at all.** Neither `POST /v2/auth/challenges` nor
   `POST /v2/auth/api_keys` is built.

   **Key issuance is not an API surface.** A user creates a key in the app,
   over gRPC; the HTTP API only ever *consumes* an already-issued bearer
   token. That is a boundary, not a gap: issuance is an in-app, human,
   consent-bearing action, and the reworked challenge flow landing separately
   across heart and the clients is where it belongs. Building either endpoint
   now would mean shipping a v2 surface we already know is changing.

   What v2 *does* own is everything downstream of the token: the scope gate,
   the per-space grant, and `GET /v2/auth/whoami` (shipped with scoped keys —
   it derives from the same grant record the gate reads, so a client can ask
   what its key is allowed to do without guessing).

   **Consequence for the completeness rule.** `/v1/auth/*` is now an
   *architectural* exception rather than a temporary one — the whole issuance
   flow lives outside the HTTP API, so a v2 client that already holds a token
   never types `/v1`, and one that does not cannot get a token from any HTTP
   call, v1 or v2. The docs must say this plainly: **v2 has no minting
   endpoint by design; obtain a key in the app.** Revisit only if headless
   issuance ever becomes a real requirement.
2. **[build] `GET /v2/spaces/{space_id}/files/{fileId}/content`** — the byte
   stream. HTTP is the convention *inside* the response (Content-Type,
   Content-Length, Range, ETag as a real validator), but everything around it
   is v2: path shape, C6 errors on the failure paths, and the 404/403
   vocabulary. Pairs with the shipped upload and with `GET .../files/{id}`
   metadata.
3. **[build] the chat SSE stream under `/v2`** — carried by Phase 6's DTOs
   rather than v1's, so the stream and the polling read agree field for field.
   This is the one item where a straddling client would have been genuinely
   incoherent: the same message in two shapes depending on how it arrived.
4. **[build] tag/option administration** (`PATCH`/`DELETE` on
   `.../properties/{key}/options/{name}`) — requires resolving Q5's rename
   semantics under names-as-identity. Recommended resolution:
   rename = create + migrate + delete, performed server-side as one operation,
   with the id-addressed escape hatch explicitly NOT reintroduced (C2).
5. **[build] template reads** (`GET /v2/…/types/{type}/templates`, `GET` one)
   — trivial passthrough; ships for completeness.
6. **Exit criterion.** A conformance test asserts that every capability
   reachable under `/v1` has a `/v2` route, with an explicit, reviewed
   allowlist of the exceptions. This is the test that makes "complete"
   checkable instead of asserted — without it, completeness decays silently
   the next time a v1 route is added. The allowlist today is exactly
   `/v1/auth/*` (both routes, item 1): key issuance is deliberately not an
   HTTP-API surface. Each entry carries a reason and an owner, so the list
   reads as a set of decisions rather than a carve-out, and adding an entry
   for any other reason has to be argued for in review.

   Note the harness already exists: scoped API keys shipped a fail-closed
   route-walking registry over the `/v2` group (verb × global class). It
   answers a *different* question — "is every v2 route classified for
   grants?" — but the walk is reusable, and this test is the second
   assertion over it.

### 10.2 What "complete" does not mean

- **Not shape parity.** See the Verdict caveat: `total = len(fetched)` and
  snake_case bodies are not ported.
- **Not capability invention.** File text extraction (Q7) stays absent because
  the middleware cannot do it; the docs say so rather than implying a gap.
- **Not v1 removal.** Phase 8 makes `/v1` *deprecable*, and §6's clock can
  then start. Removal is its own migration with its own notice period.

### 10.3 Phase 9 — space-optional object routes (decided 2026-08-09 · **RETIRED 2026-08-11**)

> **Retired — superseded by the short space reference (APIV2.md §8.35).**
> `/v2` now serves spaces by a six-character reference off the tail of the
> CID half and accepts either spelling on every route that takes a space.
> That removes the measured failure — a model cannot truncate a value with
> no dot in it, and the truncated form resolves anyway — without a new route
> class, a new grant class, or a resolver dependency. The section below is
> kept for the reasoning it records; three of its claims are corrected here.
>
> **Correction 1 — the tools it was said to unburden already take `object`
> alone.** "For the wrapper's small tier that removes a required argument
> from half the tools" is wrong as written: `read`, `set_properties`,
> `add_blocks`, `edit_text`, `check_item`, `move_block`, `delete_block` and
> `set_cell` take `object` (a handle number or an id) and resolve the space
> from `session.Space`, set by the last `find` (`runner.go` `resolveObject`).
> Phase 9 would have removed a `space` argument from the ROUTES, which the
> wrapper fills itself. The three tools that ask a model for a space —
> `find`, `describe`, `create` — are exactly the three Phase 9 says keep it.
>
> **Correction 2 — what Phase 9 uniquely solved, and how often it happens.**
> The one case the wrapper's own space memory cannot cover is a **cold-pasted
> object id with no prior `find`**. That case has **never appeared in an
> eval**: every measured trace reaches an object through `find`, which is
> also the only thing that mints the handles the tools take. It is a real
> gap and an unobserved one.
>
> **Correction 3 — `set_properties` needs a space regardless.** Even with a
> space-less route, the wrapper cannot drop the space for it: `propertyFormats`
> (the key → format index), the option-name guard, `@me` resolution and
> relative-date resolution are all space-scoped calls (`values.go`). A
> space-optional route would have moved the lookup, not removed it.
>
> **A defect found while scoping it, filed here rather than fixed:**
> `ResolveSpaceIdWithRetry` (`core/block/object/idresolver/resolver.go:98`)
> is `retry.Attempts(0)` — **infinite**, bounded only by the caller's
> context. Build item 2 below mandates using it, so an unresolvable object
> id would have spun until the request deadline instead of answering 404.
> Anything that revives this must bound the retry first.
>
> **Decision D2 is moot**, not decided: it asked what to answer for an object
> in a space the key does not hold, which only arises on a space-less route.

Not completeness and not a token knob — a surface simplification that
happens to save tokens. **Object ids are content-addressed (the CID of the
object header), so they are unique across spaces**; the `space_id` in
`/v2/spaces/{space_id}/objects/{object_id}` is redundant whenever the object
id is known.

The binding already exists and is a keyed point lookup, not a scan:
`spaceresolverstore.GetSpaceId(objectId)` (`FindId` on the primary key,
`pkg/lib/localstore/objectstore/spaceresolverstore/store.go:44`), exposed
as the `idresolver.Resolver` component (`ResolveSpaceID` /
`ResolveSpaceIdWithRetry`, `core/block/object/idresolver/resolver.go:32`)
and already consumed by `core/block/service.go:215` and `fileobject`. This
is exposing existing machinery, not building it.

**What it buys.** `space` disappears from every object-addressed route and
tool — `read`, `set_properties`, `add_blocks`, `edit_text`, and
`check_item`/`move_block`/`delete_block`/`set_cell` in the large tier. It
stays where it is genuinely part of the intent: `find`, `describe`,
`create` (you search *in* a space, create *in* a space). For the wrapper's
small tier that removes a required argument from half the tools — and it
is the argument a model is most likely to omit or invent, because it never
appears in the user's request.

**Measured, 2026-08-11 (APIV2.md §8.34).** The prediction above is right
about *which* argument and wrong about the failure mode: `space_id` is the
argument a small model most often **mangles**, not the one it omits. A space
id is two dot-joined parts (`bafyrei….28y6mgnwgodt7` — CID plus base36
replication key, and the suffix is load-bearing: it is what
`nodeconf.ReplKey` hashes to pick the responsible nodes). `gemma4:e4b`
truncates it at the dot, plausibly reading the suffix as a file extension:
**83 of 93 `find` calls** across its wrapper attempts in run
`20260810-235748`, **zero** mangles in any other argument of any other tool,
and 2/12 passed on `wrapper/large` against 8/10 on an ops arm whose tools
take **no** space id at all. §8.34 repairs the refusal so the mistake is
recoverable; Phase 9 removes the argument from the routes that do not need
it, which is the only version of this that also removes the mistake. Note
the scope limit: the routes that keep `space` (`find` above all — the very
call that produced these numbers) still take the composite id, so Phase 9
narrows the exposure rather than closing it.

**That scope limit is what retired it (2026-08-11).** `find` is where the
numbers came from and Phase 9 does not touch `find`. §8.35 does: it changes
what a space id *is* on the wire, so the value the model copies has no dot
to cut, on `find`, `describe`, `create` and every path param at once — and
it needs no resolver, no new route class and no D2 decision.

**Build items:**

1. **[build]** an `apicore` port for the resolver, carried on `V2Deps` (the
   same shape as the existing object adapters).
2. **[build]** space-optional routes (`GET /v2/objects/{object_id}` and the
   object-addressed mutations), resolving the space before anything else.
   **Use `ResolveSpaceIdWithRetry`** — the binding is eventual, so a plain
   resolve immediately after a create will intermittently 404, which is the
   worst class of bug to ship on the commonest agent flow.
3. **[build]** grant enforcement **after** resolution. `ensureSpaceGrant`
   reads the space from the path param today; a space-less route needs a
   new class in the fail-closed route registry (§8.10), or the conformance
   walk refuses it outright — which is the M1 trap in a new form: an
   unregistered route class is exactly what that registry exists to catch.
4. **[decide]** the error for an object in a space the key does not hold:
   403 naming the space confirms the object exists somewhere; 404 hides it.
   Enumeration is not a real threat against 59-char CIDs, so the lean is
   403 with the grant message — but it should be a deliberate call.

**Sequencing note.** A free partial exists first: the wrapper's `find`
already returns handles, so it can remember the space each handle came from
and fill the path itself — no API change, covers the dominant
find → read → edit flow, fails only on a cold-pasted id. Do it in the API
anyway, for the same reason locators belong there: one implementation
serves the CLI, both MCP tiers, raw HTTP and third-party SDKs, and the
wrapper-side version helps nobody who is not using the wrapper.

## 11. Documentation architecture — the actual deliverable

The completeness decision was made *for* the documentation, so the doc plan is
part of the spec, not an afterthought. Three audiences, three artifacts, one
source of truth each — and every artifact must be generated or test-pinned,
because this project has already been bitten twice by hand-maintained
artifacts drifting from the code they describe (the Phase-5 GBNF accepted
strings its own parser rejected; nine of eleven served examples were
ungeneratable under their own served grammar).

| Audience | Artifact | Source of truth | Anti-drift mechanism |
|---|---|---|---|
| Humans (developers, integrators) | OpenAPI document + a narrative guide | Swagger annotations on the v2 handlers (already present on all seven v2 handler files) | `make openapi` in CI; the conformance test of §10.1(6) |
| Agents at runtime | `GET /v2/schemas/{kind}` — the nine discovery kinds, the filter grammar, per-kind examples | The Go types and the shipped validators | The Phase-5 pattern: every served example is asserted to be accepted by its own served schema/grammar |
| Agents via the CLI | `anytype tools` manifest + `SKILL.md` | The one Go tool table (`wrapper.Tools()`) | `TestToolCount`, `TestOneDefinition`, the GBNF acceptance suite |

Two rules follow, and they are the ones worth enforcing in review:

1. **No artifact describes the API from memory.** If a document states a
   behavior, either it is generated from the code that implements it, or a
   test fails when the two disagree. Prose that cannot be pinned should
   describe *intent* (why a surface is shaped this way), never *contract*.
2. **One vocabulary across all three.** The same concept keeps the same name
   in the OpenAPI document, the discovery kinds, and the tool manifest —
   `property`, not `relation`; `type`, not `objectType`; option *names*, not
   ids. A reader moving between artifacts should never have to translate.

Open build item: the narrative guide has no home yet. The candidates are a
generated docs site (consistent, unloved) or a hand-written `README`-style
guide under `core/api/` that the conformance test keeps honest about routes
but not about tone. Recommendation: the latter, kept deliberately short — the
OpenAPI document is the reference, and the guide's job is orientation.
