# Anytype Local API v2 — specification

Status: **draft**
Depends on: AnyBlock JSON format version 2.0
(`github.com/anyproto/any-block/format/v2/SPEC.md`).

v1 (`/v1`) stays untouched for the deprecation window.

Design premise: agents — including small models — are the primary consumers.
Small models drive the **task-tool wrapper (§7)**, not the raw REST surface;
large models and programmatic clients use the full API. Every choice
optimizes for one-shot success, token economy, and a generate → validate →
repair loop with path-addressed errors.

Rules carry the source that enforces them, as `(path:line)`. A rule with no
citation is one nothing in the tree pins yet.

---

## 1. Conventions (apply to every endpoint)

| # | Convention | Rationale (research ref) |
|---|---|---|
| C1 | Base path `/v2`, localhost, bearer auth and `Anytype-Version` date header as in v1. `/v2` adds **no authentication surface**: it is a route group on the same server as `/v1`, behind the same Auth middleware, and the bearer-token, api-key and challenge-pairing endpoints are registered identically under both version prefixes (`core/api/server/router.go:152`). What `/v2` adds on top is the key-scope and space-grant layer of §1.5. | migration §4.8 |
| C2 | **One vocabulary, and it is the transport's: `snake_case`.** Every name **v2 itself owns** is snake_case, in every layer and both directions: path segments and path params (`/v2/spaces/{space_id}/objects/{object_id}`), query params (`dry_run`, `ids`), response and request body fields (`space_id`, `author_id`, `diff_stats`, `created_blocks`, `last_state_id`), and the PATCH op names (`set_properties`, `replace_text`, `insert_blocks`, …). Addressing is **by name, never by id**: property **keys** and option **names**, no id/key duality, and the object-type field is **`type`** (a type key) on every surface — envelope, rows, search, shortcuts. Object ids remain ids. **The version lives in the path (`/v2`), never in a name** — no `v2_` prefix on a schema, an operationId, an op or a field. **Boundary (APIV2_VOCABULARY.md):** inside `blocks`/`properties` the key VOCABULARY is selected at the codec's `Options.Keys` seam, never by re-spelling a marshaled document: the minted api slug by default (rename-stable, what the listings advertise), the format's raw display names under `?keys=name` (what the tool wrapper asks for). Everything else in the document stays the format's own (the query surface's `mimeType`/`size` field aliases follow it), and input accepts both vocabularies on every channel. The identity layer that mints, freezes and resolves those keys is §1.6. | v1's top agent trap (§2.1); ADDRESSING §7.5a-4; NAME_ADDRESSING.md |
| C3 | **Compact JSON always** (no pretty-printing) — **all the way down, not just the envelope**. `anyblockjson.Marshal` returns the format's canonical byte form, which is two-space **indented** (SPEC §4), and a v2 envelope re-embeds those bytes verbatim; `encodeEnvelope` compacts each embedded value at the **serving layer**, so the format's canonical form and its `Export ∘ Import` byte-stability are untouched. A handler that writes response bytes directly (`c.Data`) must write `encodeEnvelope` output; anything returned through the JSON writer is compacted already. Compaction is whitespace-only — it never re-escapes strings, so non-HTML-escaped characters and raw U+2028/U+2029 survive byte for byte — and a value the JSON scanner cannot read is appended verbatim rather than erroring, because the envelope encoder is a formatter and not a validator (`core/api/v2/service/service.go:297`). | 16–26% (TOKENS §1.1) |
| C4 | **Two document shapes, one id axis.** `?ids=compact` (**the default — the *edit* shape**): **machine-minted** block/row/column/view ids — 24-hex bson and view UUIDs, `isMintedLocalId` — relabel to their 5-char suffixes (legend-less, **lossy**); every id that could carry meaning (`dataview`, `title`, readable imported ids) keeps its full spelling and is **reserved**, so no label can alias a served id. `?ids=full` (**the *export* shape**): full ids everywhere — the backup/export read (§3(b)), and the read to clone from when a POST should reuse the source's real ids. Object refs are **full inline on every shape**: the `refs` legend was a measured net loss and its indirection trapped write-back, so no shape serves one — but legend **resolution on input stays total** (SPEC §9a), so a document arriving with a legend still resolves. Every write channel resolves a block/view/row/column id by exact id **or unique suffix** (`matchBlockRef`), **in payload slots as well as reference slots**, so no channel takes an id literally — that is what makes the lossy edit shape addressable. A payload id resolving to nothing is refused, not minted over; omitting it is how new content is authored. Never require echoing a full CID. `?ids=` also chooses how **space** ids are spelled (§1.4). **Outline exception (T7)**: the outline fixes the axis — short labels — and ignores `?ids=`. | ~24×/id, −89% id errors (§3.6); block labels −19…−22% on minted-id documents (TOKENS §1.2); id round-trip contract (R1) |
| C5 | Minimal rows: list/search responses carry `id, name, type` + requested property values. **Never embed type objects.** `fields=` expands. | v1's N× multiplier (§2.1) |
| C6 | Error shape everywhere: `{status, code, message, issues:[{path, message, hint}]}` — all four ENVELOPE members are always present, `issues` empty when the refusal has no path to address, so a caller never branches on a field's absence (an issue's own `path`/`hint` stay optional); path-addressed, naming allowed values. Required codes include: `validation_failed`, `version_unsupported` (surfaces SPEC §10's "produced by a newer version" verbatim, naming both versions), `idempotency_conflict` (same key, different body), `etag_mismatch`, `ambiguous_input` (e.g. both `filter` and `filters` supplied), `forbidden` (403 — an operation the caller's identity may not perform, e.g. editing another member's chat message), `request_too_large` (413, §1.1), `too_many_streams` (429, Phase 6). Error text is API surface; test it. Which refusals answer **outside** this envelope is fixed and small (§1.3). | repair loop (§3.2, §4.6); R15 |
| C7 | Every object read returns **`etag`** (short opaque token, ≤8 chars, derived from tree heads — NOT the object's `revision` property, which stays in `properties`) plus an `ETag` header. Mutations accept **`If-Match` header only** (the AnyBlock body has no envelope slot for it). **Advisory by default**: without `If-Match`, ops apply last-write-wins and `diff_stats` reports the outcome; with it, mismatch → 409 `etag_mismatch` carrying the current etag. Note: the etag advances on background sync, not only on agent edits — strict If-Match will 409 on sync noise; block-scoped preconditions are the deferred v2.x refinement. Derivation and comparison: §1.2. Two surfaces are exempt and say so at their router, service and handler: **chat** (Phase 6) and **spaces** (Phase 7). | R2; optimistic concurrency (§3.2) |
| C8 | `Idempotency-Key` honored on all mutations (POST, PATCH, DELETE) and on the data-free `POST /validate` read; replay with the same key returns the stored result; same key with a different request → 409 `idempotency_conflict`. Response always returns created ids where the operation creates them. Scope, identity and the multipart carve-out: §1.2. | agent auto-retry (§3.7); R15 |
| C9 | `?dry_run=true` on every mutation → would-be diff summary + issues, nothing committed. The response's `dry_run` echo is spelled exactly like the query parameter it answers. | highest-leverage affordance (§3.7) |
| C10 | Pagination on **every** list surface — objects, search, and the discovery lists (types, properties, **options**): default `limit=25`, `has_more`, truncation messages steer ("312 matches — narrow with filter…"). Options lists take a `prefix=` filter (tag-like properties can hold thousands of options). An out-of-range `limit` is refused in the C6 envelope like any other v2 refusal. | Linear/AXI (§3.4, §3.7); R-minor |
| C11 | Reads never fail on unknown *content*; anything a representation cannot express is listed in `warnings` (array of the C6 issue shape, warning-grade). Writes never pass through a lossy representation — enforced as a precondition on **every** PATCH (§1.3). | ADF disaster (§3.3) |
| C12 | Every endpoint documents **one worked example + its JSON Schema**, embedded in OpenAPI and fetchable (§5 discovery). | examples 72→90% (§3.4) |
| C13 | **Every schema served by discovery for generation purposes — ops, creates, properties, types, the filter string — is strict-mode-compatible**: non-recursive, `additionalProperties: false`, bounded, within vendor complexity caps. Known exception: the structured `filters` array is recursive (SPEC §12 filterNode) and therefore **not constrained-decodable** — documented as such; small models are steered to the `filter` string. | the format's own premise (Addendum A.1); R8 |

### 1.1 Request bodies: bounds, strict decoding, uploads

- **Every typed JSON body decodes strictly**: an unknown field is a 400
  naming the field in a C6 issue path, an empty body is a 400 carrying the
  shape hint, and an oversized body is a **413 `request_too_large`** naming
  the surface (`core/api/v2/handler/error.go:140`).
- **Caps.** Structured JSON bodies (search, chat, space, queries) cap at
  **1 MiB**, regardless of whether an `Idempotency-Key` is present; AnyBlock
  document bodies (create, validate) cap at **10 MiB**, which is also the
  ceiling the idempotency middleware buffers
  (`core/api/v2/model/model.go:148`). A streamed multipart upload is exempt
  from the buffering cap (§1.2).
- **Every bound a discovery schema advertises is enforced at the service
  layer from the same named constant**, so advertised and enforced cannot
  diverge: name and option-name 4096 characters, property/type key 256
  characters and `^[a-zA-Z0-9_]+$`, property options 100, option color 64,
  compact filter string 4096, query sorts 10, query views 10, collection
  items 1000 (checked before the per-item store walk), file source url 4096
  (`core/api/v2/service/schema_write.go:46`).
- **Upload failures are classified, never left to a retryable 500.** In URL
  mode, a source answering non-2xx and a fetch that never got a response
  (DNS, connection or TLS failure) are both 400s naming `/url`, with a hint
  to check the URL. Local-path staging failures and storage faults stay 500,
  because those are genuinely retryable or server-side
  (`core/api/v2/service/file.go:101`).

### 1.2 Etags and idempotency

- **Etag derivation.** The etag a read returns is the first 8 hex characters
  of sha256 over the object's sorted tree heads. `If-Match` is compared
  server-side against the **full** head-set hash and the 8-character display
  form is accepted as its prefix, so the value a GET returned can be sent
  back verbatim. Quoted (`"abc"`), weak (`W/"abc"`) and bare forms are all
  normalized before comparison (RFC 7232); an absent `If-Match` is
  last-write-wins (`core/api/v2/service/service.go:198`).
- **Idempotency scope.** An entry is scoped to `(authenticated credential,
  resolved space, key)`, so separate credentials never replay or conflict
  with one another even when they use the same key in the same space. The
  space is the id **after** reference resolution (§1.4), so a retry that
  spells the space differently lands on the same entry instead of applying
  the mutation a second time (`core/api/v2/middleware.go:90`).
- **Idempotency identity is the WHOLE request** — sha256 over method, path,
  raw query string and body. A `?dry_run=true` request therefore never
  replays as its later real twin; one key reused across two byte-identical
  PATCHes to different objects conflicts instead of silently replaying the
  first; and a retry that changes the path, the space spelling or adds
  `?ids=full` earns `idempotency_conflict` rather than a replay — a retry
  must repeat the request it is retrying (`core/api/v2/middleware.go:269`).
- **Store behaviour.** Only 2xx responses are cached; a failed or panicked
  request releases its reservation so a retry re-executes. Concurrent
  requests with the same key block on the in-flight reservation and then
  replay rather than double-executing. The store is an in-process bounded
  LRU with no persistence across restart.
- **Multipart carve-out.** For a streamed multipart body the identity is a
  bounded **64 KiB prefix plus the declared `Content-Length`**, with the
  remainder streaming straight through to the handler, so a keyed upload
  never buffers a whole file and no size ceiling appears. The trade is
  explicit and bounded to multipart: two uploads under one key that agree in
  both declared length and first 64 KiB replay instead of answering 409.
  Every other (JSON) body keeps the exact-body hash under the 10 MiB cap.
  The gate is a METHOD classifier (POST, PATCH, PUT, DELETE), so no mutation
  route is exempt; the search reads, which carry no idempotency middleware at
  all, stay the documented exception (`core/api/v2/middleware.go:42`).

### 1.3 Issue paths, warnings, and refusal craft

- **An issue's `path` addresses either the request body or a parameter.** A
  body path is a JSON pointer (`/text`, `/filters/0/value`, `/ops/1`) except
  inside the state applier, where op paths keep the receipt spelling
  (`ops[0].set.filters`, `ops[0].value[1]`, `/markdown[3]`) that
  `created_blocks` also uses. A bare name (`view`, `fields`, `offset`,
  `dry_run`, `object_id`, `chat_id`) addresses a query or path parameter
  (`core/api/v2/service/stateops.go:590`).
- **`warnings` are advisory**: warning-grade C6 issues that never require a
  retry and never signal a failed call. They ride the list envelope as well
  as object envelopes (`core/api/v2/model/model.go:169`).
- **C11's write rule is a precondition on every PATCH.** If the internal
  marshal of the object's live state reports a loss warning (an unmapped
  block type, over-deep nesting), the edit is refused **422
  `validation_failed`** carrying those warnings and the advice to edit the
  object in the app. No PATCH path and no op is exempt; four warning classes
  are, because they round-trip losslessly — `/property_internal_keys`,
  `/type_internal_keys`, the query-source classification warning and the
  raw-date fallback (`core/api/v2/service/stateops.go:245`,
  `core/api/v2/service/stateops.go:234`).
- **A key miss answers with candidates.** Every route that addresses a type
  or property BY KEY answers a miss with the known keys (capped at 15) plus a
  nearest-match did-you-mean, and the list-route steer only when the list was
  truncated with no suggestion — a candidate-bearing refusal is what a model
  repairs from, a bare one produces no retry at all. Two deliberate
  exceptions: the space 404 gets the discovery steer but never a candidate
  list (ids are opaque, and a scoped grant must not imply the full space
  list), and the object 404 is left alone (unbounded candidate set)
  (`core/api/v2/service/refs.go:199`).
- **A refusal states this API's own rule and the repair, and never cites a
  specification section**: the reader of a refusal does not have the
  document, and a citation that contradicts the refusal sends them somewhere
  that says the opposite. So the file family is refused as an `inside` target
  because clients treat those blocks as leaves and only a NEW API edit is
  refused — which the message says — while the format's own leaves are
  refused as "can never be a parent", with no scope invented for them
  (`core/api/v2/service/stateops.go:1713`).
- **Which refusals do NOT use the C6 envelope.** Pairing, authentication,
  key-scope, request-origin and the shared write rate limit answer before
  v2's own handlers, in the older shared error envelope; an unmatched route
  or an unhandled panic answers with no envelope at all. Everything v2 itself
  refuses — including an out-of-range page `limit` — answers in the C6 shape
  (`core/api/v2/markdown/api.md:202`).

### 1.4 Space references and how ids are spelled

- **A space is addressed by a short reference as well as by its full id.**
  The served short form is the last six characters of the CID half of the
  full `<cid>.<replication key>` id — the replication key is excluded
  because it distinguishes no two spaces on one account, and dropping it
  removes the dot a caller can truncate at. The census runs over the
  caller's whole visible space set **before any pagination**, so no page can
  mint a tail another page also claims; ids whose tails collide keep their
  full spelling, and an id that is not space-shaped never shortens and never
  answers to a tail. Resolution is the rule the rest of the surface uses —
  exact full id first, then a unique suffix of the CID half, and more than
  one claimant is a 400 listing the candidates — which also means a full id
  truncated at the dot resolves (`core/api/v2/service/spaceref.go:114`).
- **Resolution is one middleware over the single space route parameter, and
  it runs BEFORE the space-grant gate**, because grants are keyed by full
  space id. Its candidate set is the caller's visible spaces — live and
  intersected with the key's grant — so a short reference can never resolve
  to a space the key cannot see and is not a probe. A full, space-shaped id
  is returned untouched without reading the space list; a reference that
  resolves to nothing is passed through unchanged so the ordinary not-found
  or the grant refusal answers it, quoting the caller's own value and never
  naming the space a tail belongs to (`core/api/v2/service/spaceref.go:204`).
- **Refusals echo the caller's own space spelling.** At the single place a v2
  error becomes bytes, the resolved full space id is re-spelled back into the
  reference the caller sent, across the message, every issue message, every
  hint and the space binding of every typed reference a hint renders — and
  only that binding, since a property key or option name that happens to
  contain the id names something else. Substituting the short reference into
  a repair route keeps the route valid, because every space-addressed route
  takes either spelling (`core/api/v2/handler/error.go:41`).
- **`?ids=` is the one parameter that chooses how ids are SPELLED in a
  response, on every axis a response has**: block ids in a document and space
  ids anywhere a space is named. It is parsed once for the whole `/v2` group
  by a middleware that runs before space resolution (so an ambiguity's
  candidate list is spelled the way the request asked), and an unknown value
  is a 400 addressed at `ids` naming `compact` and `full` on every route. Its
  legal values live in one function that the object read's own validation
  also calls, so the two axes cannot drift. Under `?ids=full` the space list,
  the single-space read, the space create and update, the global-search rows'
  `space_id`, the whoami grant echo and an ambiguity's candidates all serve
  full ids, while the default serves short. Nothing else in a response is
  re-spelled: a member id that embeds a space id is an id of another object
  and keeps it whole. **Accepting is unaffected** — both spellings are
  accepted on every route, always, so a caller turns a reference it was
  handed into one it can store by re-reading with `?ids=full`
  (`core/api/v2/service/idshape.go:39`).
- **A short space reference is an addressing convenience, not an identity.**
  It is unique only against the caller's currently visible space set, so
  joining a space whose tail collides retires it. Anything that PERSISTS a
  space reference — a config file, a script, a log line, a call into another
  interface — must store the full id, which is what `?ids=full` is for
  (`core/api/v2/service/spaceref.go:154`).
- **Cross-space object links in caller text are expanded.** The platform's
  two-parameter deep link `anytype://object?objectId=<id>&spaceId=<space id>`
  has its `spaceId` expanded from a short reference to the full id on every
  path that turns caller text into marks: documents, block ops,
  `replace_text` and chat messages. Stored destinations and every read of them
  keep the FULL space id under all shapes, since a shortened destination is
  one no client could open. A reference that cannot be expanded — unknown to
  this key, or ambiguous — leaves the link exactly as sent and is reported on
  the `warnings` channel, path-addressed, once per reference; so does a
  destination that expansion would push past what the renderer can write
  back. One expander serves a whole request, so a document with many links to
  one space reads the space census once
  (`core/api/v2/service/spacelinks.go:96`).

### 1.5 Authorization — key scope and space grants

- **Scope gate.** `/v2` admits only API keys whose scope is JsonAPI or Full.
  A key with any other scope — including the web clipper's Limited and the
  zero-value Limited that legacy keys minted without a scope carry — is
  refused **403** on every `/v2` route, distinct from the 401 an invalid key
  gets. The refusal names the key and its actual scope and states the remedy
  (create a new api key with JsonAPI scope); legacy unscoped keys keep
  working unchanged on `/v1` (`core/api/server/middleware.go:316`).
- **Credentials are required on every `/v2` route except a closed exempt
  set**: the two public OpenAPI documents (`GET /v2/docs/openapi.yaml|json`)
  and the pairing endpoints (`POST /v2/auth/challenges`,
  `POST /v2/auth/api_keys`). A route not on that list must answer 401 without
  credentials (`core/api/v2/authz.go:164`).
- **Grants.** An API key may carry a grant record
  (`{spaces | all_spaces, perms: read|readwrite}`); the **grant**, never the
  key-string format, is what enforcement reads, and a key with no grant
  behaves as an unscoped legacy key. On `/v2` the grant gate runs directly
  after the scope gate: a request addressing a space the grant does not cover
  is 403 `space_not_granted`, and a write-classified route reached with a
  read-only grant is 403 `write_not_granted`. Both refusals name the actual
  grant. The tech space is denied unless explicitly granted — including under
  an all-spaces grant — because the gate runs before the service admits the
  tech space as an ordinary space id. These are v2's own middleware, so they
  answer in the C6 error shape (`core/api/v2/authz.go:218`).
- **Every `/v2` route is classified in an explicit authorization registry** —
  a verb (read or write) describing what the route does to **data**, not its
  HTTP method, plus a global class for routes carrying no space param.
  `POST /v2/search` and `POST /v2/validate` are reads; a chat `POST …/read`
  is a write, because it advances the synced read watermark. The global
  classes are `auth-exempt` (public documents and pairing), `data-free-allow`
  (`/v2/validate`, `/v2/schemas*`), `service-filtered` (`GET /v2/spaces`,
  `POST /v2/search`, `GET /v2/auth/whoami` — let through and constrained
  inside the service) and `scoped-denied` (`POST /v2/spaces`). A no-space
  route missing from the registry is refused fail-closed, and an unclassified
  route counts as a write (`core/api/v2/authz.go:110`).
- **`POST /v2/spaces` is refused to a key granted only some spaces**: a key
  that can mint spaces it then owns has escaped the boundary the user drew.
  An all-spaces grant is admitted, because it already covers spaces created
  later; a read-only all-spaces key is still refused, by the write gate, as
  `write_not_granted` (`core/api/v2/authz.go:61`).
- **The two space-fan-out surfaces** (`GET /v2/spaces` and global
  `POST /v2/search`) intersect their space set with the request's grant at
  the input, and the service layer repeats both halves of the gate as a
  backstop in its own precedence — space first, then verb — so a route that
  forgets the middleware can neither reach a non-granted space nor mutate a
  granted one with a read-only key (`core/api/v2/service/service.go:82`).
- **`has_not_granted_spaces`** rides the space-list envelope: true when at
  least one other live user space is excluded by the key's grant. It reveals
  no excluded ids, names or count, is independent of pagination (`total` and
  `has_more` count only accessible spaces), and is not set by deleted, left,
  still-joining or internal tech spaces. An agent missing a space uses it to
  ask the user for access (`core/api/v2/model/model.go:230`).
- **`WWW-Authenticate` rides every authentication and authorization
  failure**, because MCP clients parse it. A 401 sends
  `Bearer realm="anytype"` (bare when no credential was sent, with
  `error="invalid_token"` otherwise); a 403 sends
  `Bearer error="insufficient_scope"`, carrying
  `scope="space:<space_id>:<read|readwrite>"` when the request addressed
  exactly one space (`core/api/util/grant.go:192`).
- **Editing a key's grant takes effect on the very next request**: the server
  evicts that key's cached HTTP session entries when the app link is updated,
  so an in-place narrowing is never served from a stale cached grant. Because
  a sweep can only evict entries that exist, the server keeps an eviction
  generation snapshotted when the session cache is read and re-checked when
  it is written — on a mismatch the freshly minted entry serves that one
  request and is dropped rather than cached
  (`core/api/server/server.go:206`).
- **A key carrying a NARROWED grant is refused on `/v1`** with the C6 error
  `v1_not_available_for_scoped_keys`, whose message steers to the same route
  on `/v2`; grant SHAPE decides, never key format. An unrestricted grant (all
  spaces, readwrite) is honored on `/v1` by doing nothing, since it grants no
  less than an unscoped key, and a legacy nil-grant key stays served on `/v1`
  exactly as before (`core/api/server/middleware.go:350`).
- **`GET /v2/auth/whoami`** (authenticated) describes the CREDENTIAL, never
  the person: `{key: {id, name, created_at, expires_at}, scope, grant,
  api: {version}, key_status, notice?}` with RFC 3339 UTC dates. `grant`
  carries the required explicit booleans `scoped` (a grant record exists),
  `restricted` (the key reaches only the listed spaces) and `all_spaces`,
  plus `permission` and a `spaces` array of `{id, name, permission}` — a
  legacy unscoped key is `{scoped: false, spaces: [], permission: null}` and
  NEVER `spaces: null`, because consumers get the null-vs-empty test
  backwards and that failure direction is fail-open. An all-spaces grant
  reports `space_count` and lists its spaces only under `?spaces=true`;
  `?ids=` selects compact or full space-id spelling. Space names resolve
  through the same grant-intersected path `GET /v2/spaces` serves, and a
  granted space missing from the live list keeps its entry with an empty
  name. The body derives from the same request-context grant record the gate
  enforces, never from a second derivation
  (`core/api/v2/model/model.go:268`).
- **The whoami credential is read ONLY from the `Authorization` header**; a
  token supplied as a query or body parameter is never accepted, and an
  unknown or revoked key gets the shared auth middleware's plain 401. The
  route deliberately does not implement RFC 7662 introspection (no POST form,
  no `active` field), which would make it an enumeration oracle. `key.id` is
  the app link's sha256 hash — credential-adjacent: not the token and not
  invertible, but a whoami body is a full offline verifier for the credential
  and should be treated as sensitive (`core/api/v2/handler/whoami.go:13`).
- **Key-status signalling.** Every authenticated response on both route
  groups carries `Anytype-Key-Status` (`legacy` for a key with no grant,
  `scoped` otherwise), so absence never means anything and grant presence
  alone decides. For nil-grant keys of JsonAPI scope only, the response
  additionally carries `Anytype-Notice` (one printable single-line ASCII
  sentence that never interpolates user data) and
  `Link: <…authentication guide>; rel="deprecation"` — added, not set, since
  Link is list-valued. The remedial pair is withheld from Limited and Full
  credentials, which cannot act on the re-issue advice. The whoami body
  repeats the same two values as `key_status` and `notice`, because agents
  read bodies rather than headers. RFC 9745 `Deprecation`/`Sunset` are
  deliberately never emitted: they are date-bearing and resource-scoped, and
  on `/v1` they would declare `/v1` deprecated, the opposite of the
  grandfathering promise (`core/api/util/keystatus.go:27`).

### 1.6 Key identity — resolution, mint, slugs and corpses

**Resolution never guesses.** v2's request chain
(`core/api/v2/service/keys.go:226`, wired at
`core/api/v2/service/keys.go:209`): an exact live **stored key** wins
verbatim before any table is consulted; then the exact live **served slug**,
where two visible holders are refused as `ambiguous_input` listing both; then
the **built-in vocabulary** (exact key or derived slug); then a **forgiving
fold** (case, separator, spacing) that resolves a single candidate and
refuses several as `ambiguous_input`; and only last the **display name** and
its fold, on the same one-or-refuse rule. The format's own chain underneath
(`pkg/lib/anyblockjson/storeresolver/keyvocab.go:495`) has the same order but
degrades to the verbatim term wherever v2 would refuse — it never guesses
either. Ambiguity at any step lists every holder by stored key and id (the
addresses that always resolve) and never picks by store order; a full miss
falls to the unknown-key refusal with did-you-mean (§1.3). The chain is wired
into route params, search and set type scopes, document creates,
`set_properties` keys and the option pre-resolution pass, so one spelling
works at every level and no channel keeps a private resolution.

**Nothing in the chain creates.** An unknown key on a value channel is
refused; only a type's `property_definitions` may mint a property, and that
is a separate, declared step.

**One authority for the key a listing advertises and the key a document
carries.** A candidate spelling is dropped in favour of the entity's stored
key whenever another live entity holds that spelling as its own key, another
live entity also answers to it, or the built-in vocabulary resolves it to a
different entity — so the API never advertises an address that would then
refuse as ambiguous (`core/api/v2/service/keys.go:800`).

**An api key is minted once and frozen.** It is emitted by table lookup and
never re-derived from the entity's current name, so a rename never moves an
address, and a slug no longer derivable from the current name still resolves
on input (`core/api/v2/service/apikeyvocab.go:3`).

**Mint.** `POST /properties` and `POST /types` never adopt the caller's key as
stored identity: they mint an internal BSON key and store the caller's
proposed key as the snake-normalized **api slug**, which is the address every
surface advertises. The slug is union-collision-checked against bundled keys,
bundled-derived slugs, live stored keys and live stored slugs — including fold
classes, and including slugs minted earlier in the same request — before
anything is written, so a custom `Due Date`, `due_date` or `dueDate` cannot
shadow the bundled property and normalized twins refuse loudly. A slug derived
from the display name when no key is given is guarded identically. The refusal
steers to the existing holder or to an explicit different key; auto-suffixing
is deliberately not done, because explicit beats silent. Bundled keys keep
their derived install path, since convergence IS the install mechanism
(`core/api/v2/service/schema_write.go:1152`).

**A derived slug is sanitized** to the advertised `^[a-zA-Z0-9_]+$` grammar
and length cap before it becomes an address; an empty result means no
derivable slug exists and the minted internal key stays the only address.
Names like "50% done", "C++" or an emoji therefore never become
identity-bearing, unaddressable keys (`core/api/v2/service/keys.go:731`).

**Hidden properties and types own no served spelling**: they are excluded
from the vocabulary a document resolves against and from every listing, and
keep only their stored key. A hidden entity therefore cannot block a visible
entity's key, and cannot be addressed by a slug or display name
(`pkg/lib/anyblockjson/storeresolver/keyvocab.go:171`).

**Corpses.** A type or property the user deleted — archived, deleted, or
UI-uninstalled — is excluded everywhere the API addresses schema (listings,
key-addressed routes, did-you-mean candidate lists, existence guards), and it
**vacates its slug**, so a same-key create mints cleanly instead of failing on
"already exists" pointing at a corpse. Delete-then-recreate is a clean create
with a fresh identity, and DELETE of an already-deleted type or property is a
404, not a second archive (`core/api/v2/service/keys.go:56`). What a corpse
still does to a **write** is §2's key-acceptance rules.

**Option listings exclude removed options**: an option belonging to an
uninstalled property or removed on its own never appears in
`GET /v2/spaces/{space_id}/properties/{key}/options`
(`pkg/lib/localstore/objectstore/spaceindex/relations.go:239`).

**A document body is not an identity channel.** A type document supplying
`uniqueKey`, `relationKey`, `isReadonly` or `restrictions` is refused
path-addressed — export strips all four, so no legitimate round trip carries
one, and a forged unique key would occupy the id a later bundled install
converges to. System-managed details a round trip DOES legitimately carry
(`apiObjectKey`, origin, space id, archived/deleted/uninstalled flags) are
dropped in favour of the system's own values rather than rejected, so a
supplied api key can never bypass the collision check. The envelope's own
identity slots are refused by the document validator on every create, for
every kind (`core/api/v2/service/schema_write.go:391`).

**A declared format that contradicts the resolved relation is refused.** A
type-property entry whose declared format disagrees with the format of the
relation its key resolves to is a path-addressed 400 on both type create and
type update, checked before the create-missing resolver can mint anything;
entries that resolve to nothing (the creation case, where the declared format
IS the new property's format) or carry no format pass through. Formats are
compared through the served vocabulary, not the raw enum, so the guard agrees
with what a read shows (`core/api/v2/service/schema_write.go:476`).

**The identity layer fails closed.** The live type and property snapshots it
reads are primed once per request; a store error is returned, never
swallowed, because an empty-looking namespace would wave every collision
through. A key slot that resolves to nothing is refused, never minted over
(`core/api/v2/service/keys.go:96`).

## 2. Phases

Each phase ships independently and is agent-usable on its own. **[B*n*]**
marks a choice gated on a benchmark from §4. **[build]** marks a dependency
that does not exist yet and must be built (not assumed).

### Phase 0 — plumbing + eval harness

- Error contract (C6) incl. the named codes; etag plumbing (C7); idempotency
  store (C8); dry-run scaffolding (C9).
- `POST /v2/validate` — body: AnyBlock document → `{issues, warnings}`.
  **Structural + format-semantic only** (exposes `anyblockjson.Validate`;
  the package cannot see the space). Referential validation — option names,
  a type's actual property keys — is a **space-aware layer built in
  Phase 2**, not a free consequence of this endpoint (R9).
- **`/validate` answers 200 even for an invalid document.** Findings are data
  for the repair loop, never a 4xx, and both lists are always present, empty
  when the document is clean. A document produced by a newer format version
  surfaces as a `/formatVersion` issue with a hint rather than a transport
  error; `version_unsupported` is reserved for body rejection on a *write*
  (`core/api/v2/service/validate.go:21`).
- **Eval harness** (`cmd/apiv2eval`): runs task-specific model loops against
  a scratch space through the small/large wrapper tiers or the raw op
  surface. Each task has a final-state grader; the harness records outcome,
  API/tool refusals, prompt/completion tokens and turns. Its one-turn probe
  measures schema-emission behavior without a live API. `snapshotdiff` is the
  corpus round-trip comparator used by `cmd/anyblockroundtrip`, not an API
  eval metric.

### Phase 1 — read

```
GET /v2/spaces/{space_id}/objects/{object_id}
    ?include=properties,blocks      # subset; default both
    ?outline=true                   # block skeleton, see below
    ?block={blockId}                # subtree only (contiguous indent-run)
    ?ids=compact|full               # document shape (C4); default compact (edit)
                                    # object ids are always full — never compacted
    ?format=anyblock|md             # md read-only, with warnings (C11)
GET /v2/spaces/{space_id}/objects            # minimal rows (C5)
DELETE /v2/spaces/{space_id}/objects/{object_id}   # archive, OWN OUTPUT ONLY; ?permanent=true later
GET /v2/spaces                              # spaces list (read)
GET /v2/spaces/{space_id}/members            # members list (read) — agents need member ids for assignee/creator values
GET /v2/spaces/{space_id}/types              # keys + names (paginated, C10)
GET /v2/spaces/{space_id}/types/{type}       # the kind:"objectType" AnyBlock document
GET /v2/spaces/{space_id}/types/{type}/schema?flavor=json-schema|table|example   [build]
GET /v2/spaces/{space_id}/properties         # key, name, format (paginated)
GET /v2/spaces/{space_id}/properties/{key}/options    # option names (+color), paginated + prefix=
```

- **`outline=true` returns every *addressable* block** as
  `{indent, id, type, text}`. Text is capped at 80 runes and receives a
  trailing ellipsis when truncated, so the outline stays a cheap survey while
  every addressable block remains addressable for a follow-up `?block=` read
  or PATCH.
- **Block references address the entries of the document's `blocks` array and
  nothing else.** Ids a read serves from *inside* a block — a table's row and
  column ids, the ids of blocks nested in table cells, a dataview's view ids —
  are real ids but **not block references**: rows and columns are addressed
  through `set_cell`'s `row`/`col`, views through the view ops, and a block
  inside a cell is not individually addressable at all (rewrite its cell with
  `set_cell`). `outline=true` lists exactly the addressable set, and every
  not-found refusal for a block reference says so rather than promising the
  outline lists every served id (`core/api/v2/service/object.go:675`).
- **Param legality** (R12): `outline` and `block` are mutually exclusive
  with each other and with `format=md`; `outline` implies blocks (an
  accompanying `include=properties` adds the properties map; `include`
  without blocks suppresses `blocks` entirely); `ids` selects the whole
  document shape (C4); illegal combinations → 400 `ambiguous_input` naming
  the conflicting params. The outline shape uses compact block labels but
  **full object refs** (read-only shape; the C4 outline exception — `ids` is
  ignored there), which is also what the default and subtree reads emit, so a
  block id never changes spelling between an outline, a `?block=` and a
  default read.
- **`?block=` subtree reads.** Indents stay **absolute** — never rebased onto
  the anchor — so a block's id and depth are identical whether it is read in
  full or as a subtree. The reference resolves by exact id or unique suffix
  against the ids the read **serves** first and, only when that finds nothing,
  against the object's **stored** ids — mapped back to the served block by
  position, never by a second suffix scan — so both spellings of a relabeled
  block address it. An ambiguity in the served vocabulary stays a refusal and
  is never resolved against the other vocabulary; a stored id the read never
  serves (the root, table wrappers, cells) remains a 404; a suffix matching
  several served ids is a 400 steering to the full id. `?block=` implies
  blocks, and an accompanying `include=properties` adds the properties map
  rather than emptying the read (`core/api/v2/service/object.go:501`,
  `core/api/v2/service/object.go:521`).
- **`GET /types/{type}` is an object read**: it delegates to the object read
  and the `?ids=` parameter rides through, so `?ids=full` means exactly what
  it means on an object (the handler passes only `ids`; the other read
  parameters are not offered on this route). A type document therefore has an
  export shape one query parameter away, and the well-known `dataview` block
  id needs no exemption because it is not machine-minted
  (`core/api/v2/service/discovery.go:278`).
- `types/{type}/schema` **[build]**: the derived artifact (SPEC §2a
  `GenerateSchema` — *planned there, not implemented*; this endpoint is its
  first consumer). `table` flavor = prompt-ready property table with live
  option names — requires an objectstore join per select property (options
  live on option objects, not in `typeProperties`) (R6). **As shipped, the
  route is a 501 `not_implemented` stub** steering to `GET types/{type}`.
  Until it lands, the wrapper's `describe` tool assembles the type and its
  live option lists itself. The route remains the highest-leverage accuracy
  improvement (§3.4).
- **`DELETE /objects` (archive)** is built; its complete rule is under
  Phase 2's *Deleting own output*. The task-tool wrapper does not expose an
  object-archive verb.
- Implementation note: read via the **live smartblock state** → snapshot →
  `anyblockjson.Marshal` (not `ObjectShow`, whose ObjectView is the wrong
  type; not the store snapshot, which lags). Derive `etag` from that same
  state so token and content are consistent (R-note).

### Phase 2 — create (one-shot)

```
POST /v2/spaces/{space_id}/objects       # body: AnyBlock document (ids optional — id-less input, SPEC §9)
                                        # or shortcut {type, name, properties, markdown}
POST /v2/spaces/{space_id}/types         # kind:"objectType" doc — typeProperties creates missing properties
POST /v2/spaces/{space_id}/properties    # {key?, name, format, options?:[{name,color?}]}
POST /v2/spaces/{space_id}/queries       # {name, type, filters|filter, sorts?, views?}
POST /v2/spaces/{space_id}/collections   # {name, items?}
POST /v2/spaces/{space_id}/templates     # AnyBlock doc with templateFor → generic object-create path
GET  /v2/spaces/{space_id}/templates     # ?type= — the read half of the templates collection
POST /v2/spaces/{space_id}/files         # upload (multipart or URL) → file object id
PATCH/DELETE /v2/spaces/{space_id}/types/{type}        # update (type doc semantics) / archive
PATCH/DELETE /v2/spaces/{space_id}/properties/{key}    # update / archive
```

- `POST /objects` body discriminator (R7): presence of `version` or
  `blocks` ⇒ full AnyBlock document; otherwise the shortcut shape.
- **`POST /files` is load-bearing**, not an extra: file/image/video/pdf
  blocks and `iconImage` carry file object ids an agent cannot otherwise
  obtain (R11).
- **Sets** (R10): `ObjectCreateSet` accepts no filters/sorts/views. Build the
  set by constructing its **initial state with a fully-formed dataview
  block** (filters/sorts/views included) via the AnyBlock create path — one
  change set, honestly atomic. Collections are clean (the `collection_items`
  import path exists). A set created through `POST /queries` **pins its
  dataview block id to `dataview`**; any other id makes the editor add a
  second, default dataview at first open. `views` is mutually exclusive with
  a top-level `filter`/`filters`/`sorts` — supplying both is 400
  `ambiguous_input` at `/views` steering the caller to move the top-level
  filter into a view — and a request carries at most 10 views
  (`core/api/v2/service/list_create.go:67`).
- **`POST /queries` refuses a `type` filter leaf** with a targeted 400 — a
  query is already scoped to its type, so the repair is to drop the filter —
  rather than the generic unknown-property-key error
  (`core/api/v2/service/list_create.go:502`).
- **A type document carrying `blocks` is refused on create**: a type's views
  are generated for it at first open, and the refusal steers the caller to
  create the type first and then edit its views through the object PATCH
  channel, addressing the type by its object id. Silently dropping the views
  would break C11's write rule. The discovery schema for the `type_document`
  kind drops the same member from the same single definition
  (`core/api/v2/service/schema_write.go:155`).
- **`recommendedLayout` accepts the layout NAME** (`"todo"`, `"basic"`,
  `"note"`, `"profile"`) as well as the stored number on type create and
  update; an unknown name is a path-addressed 400 at
  `/properties/recommendedLayout` naming the common layouts. Exports write
  the canonical layout name (`core/api/v2/service/schema_write.go:450`).
- **Every create that takes a document accepts a body pasted straight from a
  read**: the read-envelope additions `etag` and `warnings` are stripped
  rather than refused, and a body carrying the `subtree: true` marker is
  refused by name with the repair (read the source object whole, without the
  `block` parameter). Ids shaped like a compact label — five lowercase hex
  characters — are **adopted** as the new object's real ids and reported as a
  `warnings` issue, never refused, because a clone of a document that
  genuinely owns such ids must keep working; the warning names the full-id
  read as the way to clone with the source's real ids
  (`core/api/v2/service/create.go:597`).
- **A create body carrying `type_internal_key` is refused, path-addressed.**
  Everything the endpoint decides — the type gate, the restricted-type
  refusal, the property keys, the type reported in the result and which
  template applies — reads `type`, while the format's own import lets the
  internal key win, so a body carrying both would be validated as one type
  and created as another (`core/api/v2/service/create.go:934`).
- **Templates**: no create-from-body RPC exists; `POST /templates` targets
  the generic AnyBlock create path (Template kind + `templateFor`), which the
  importer already supports (R-note).
- **Every side effect a write creates** — properties and options, real on a
  live run and would-be on a dry run — is reported under `created` in the
  response, each option named once however many ops resolved it
  (`core/api/v2/service/resolver.go:334`).
- **Everything created through `/v2` is stamped with the `api` origin** —
  objects, types, properties, options and uploaded files alike
  (`core/api/v2/service/schema_write.go:460`).
- **A create's `etag` comes from an immediate read-back and is best effort**:
  if the read-back fails the response still succeeds and carries a warning
  telling the caller to GET the object for its etag. A successful create
  never degrades to a 5xx (`core/api/v2/service/create.go:827`).
- Every kind: schema + worked example (C12/C13); `Idempotency-Key`;
  `dry_run`.

**Referential policy on writes (R9), fixed per reference kind.** Property
keys in an object's `properties` map, type keys (`type`, `templateFor`, a
set's `type`) and a set's filter/sort/view property keys are **rejected**
with a path-addressed did-you-mean; a set's keys must be among the queried
type's recommended keys plus `name` and the system allowlist (Phase 4 rule
2). `typeProperties` keys on type creates and updates are **created when
missing**. A did-you-mean ranks candidates by exact match, prefix and
containment first, then Levenshtein distance of at most 2, suggests at most
3, and an error names at most 15 of the actual keys before steering to the
listing route (`core/api/v2/service/refs.go:424`).

**Select and multi-select options are only ever created with explicit
consent**, and never as a side effect of a name that matches nothing: without
`?create_missing_options=true` an unmatched option name is **refused, not
created** (the gate defaults off and is group-wide,
`core/api/v2/middleware.go:358`). Options can be created in three places —
with the property through `POST /properties`, implicitly while writing a
value, and by declaring them on a property definition in a type write
(`POST`/`PATCH /types` and the type op channel), which reaches only a
property that already exists and refuses one the same request is minting
(`core/api/v2/service/typeoptions.go:76`). `PATCH /properties/{key}` takes
only `name`, and there is no options route on a property.

#### Key acceptance on the write channels

These rules govern every channel that takes a property or type key as *data*
— object create, `set_properties`, the type-properties list, a type's details
PATCH, the view channels, and set/query creation. The resolution chain itself
is §1.6.

- **Two spellings of one key in one body is refused**, naming the collision
  rather than letting order pick a winner: the caller asked for two values on
  one key and one of them would be dropped. The refusal is path-addressed on
  the object and type-detail channels; on the type-properties list it is a
  plain error naming both spellings
  (`core/api/v2/service/keys.go:1216`, `core/api/v2/service/resolver.go:803`,
  `core/api/v2/service/schema_write.go:719`).
- **A create whose key is already taken is refused before anything is
  written.** The uniqueness check spans the whole namespace — the space's
  live stored keys, its live served slugs, and the built-in vocabulary,
  installed or not — and the refusal names the holder and the repair. A key
  reserved by a built-in property or type says so, because neither "use the
  existing one" nor "update it" can be followed for one that is not installed
  (`core/api/v2/service/schema_write.go:1185`).
- **`POST /properties` refuses a display name the space already holds** when
  the name is all the request gave, naming the existing property's key.
  Supplying an explicit different `key` creates a second property under that
  name and returns a warning instead — a name is not an identity
  (`core/api/v2/service/schema_write.go:1216`).
- **A create response returns the key the store actually holds**, read back
  from the create, not the key the request's name would derive. The two can
  differ, because the mint walks to a free spelling when something else
  already answers to the derived one; an empty stored key is authoritative
  and falls through to the internal key, which is then the only address there
  is (`core/api/v2/service/schema_write.go:384`).
- **A removed property accepts no new data.** A property the space has
  removed — uninstalled through the app, or archived through this API's own
  DELETE — is refused, path-addressed, on document create, on
  `set_properties` for a key the document does not already hold, on the view
  channels (columns, `group_by`, filters, sorts), on set/query creation, and
  in a type's property list. The refusal names the repair and spells the key
  the way the caller sent it (`core/api/v2/service/keys.go:1016`).
- **Two tolerances survive that refusal**, both so an edit loop cannot become
  a silent deletion. A value the document already holds under a removed key
  stays editable and can be **unset** — `unset` is the only cleanup channel
  left. And a built-in property nobody ever installed is not "removed": it
  has no object at all, and naming it in a write **installs** it, which is
  the ordinary case in a fresh space
  (`core/api/v2/service/stateops.go:1363`).
- **A removed built-in TYPE is refused the same way**, on `type` and
  `template_for` and on set creation, with a refusal that says *removed* and
  steers to the live type list rather than reporting the key as unknown. A
  create cannot drop its type, so the repair differs from the property one
  (`core/api/v2/service/refs.go:344`).
- **A space-minted (non-built-in) property that was removed is treated the
  opposite way**, because its key can never be reinstalled and a value on it
  is inert: a body this API served and the caller pasted back is accepted,
  and the removed property's served slug canonicalizes to the stored key the
  value already lives under. A built-in removed key cannot round-trip this
  way, and its refusal says to drop the key from the request — the source
  object keeps its value and it reappears if the property is restored
  (`core/api/v2/service/keys.go:1170`).
- **`GET /types/{type}` serves the type's property list faithfully**,
  including entries for properties the space has removed, because the list is
  what the type actually stores and dropping those rows would make the
  documented read-modify-write loop delete every reference it touches.
  Sending such an entry back resolves to the property already referenced —
  nothing is created, nothing moves — but only when it is spelled with the
  **stored key**; the removed property's slug has vacated the namespace and
  still resolves to nothing (`core/api/v2/service/resolver.go:729`).
- **Object create tolerates a key no live property claims but some relation
  object still holds** — a UI-deleted or archived relation — so a read body
  pasted back creates a copy instead of being refused with a did-you-mean
  pointing at an unrelated live key. PATCH `set_properties` does not: it
  refuses such a key unless the object's own document already carries it,
  because it is not a paste channel. Neither tolerance is an **address** —
  nothing resolves such a key to a property object, and no listing advertises
  it — and a **built-in** relation this space removed is refused on both, as
  removed rather than as unknown (`core/api/v2/service/create.go:1162`).

#### Creating from a template

- **A create body takes `template`**: a template's store id, or the word
  `none` to start from nothing. Both body shapes accept it — the shortcut
  declares it and the full document has it lifted out before the
  discriminator runs — because it is a create **directive** like `dry_run`,
  not a document member, and no read serves it back. An EMPTY string reads as
  **absent**, not as `none`, so a schema-generated body carrying every member
  it can see cannot quietly switch a type's default template off
  (`core/api/v2/service/create.go:152`).
- **Who chose the template decides what a failure means.** A template the
  CALLER named that cannot be applied — unknown, deleted, not a template, or
  a template of another type — is a refusal addressed at `/template`. A TYPE
  default that cannot be applied creates the object without it and **warns**,
  naming the dead id and the repair, because refusing would fail every create
  of that type over a choice the caller did not make. A store error on either
  path is an error, never read as "no template"
  (`core/api/v2/service/templates.go:62`).
- **The applied template is named in the create result** as
  `template: {id, name, source}`, on dry runs too, where `source` is
  `request` or `type_default` — without it, blocks the caller never sent read
  as the server inventing a body. The object carries `blocks_added`, the
  template's own block contribution counted before layout conversion and
  excluding the header every object carries; it is **absent** (not zero) on a
  dry run, which never builds the template. `combined` says this request sent
  blocks of its own, which follow the template's, and rides dry runs because
  it is knowable from the request alone (`core/api/v2/model/model.go:427`).
- **Templates apply to `POST /objects` only.** `POST /queries` and
  `POST /collections` compose their document server-side around a generated
  dataview, so a template's blocks would blend two structures nobody asked to
  merge; `POST /templates` refuses a `template` outright, because a template
  has no template of its own. When a template applies, its blocks come first
  and the caller's document is merged on top — its blocks after, its relation
  links, its collection items, its root attributes winning — and any caller
  block whose id the template state already holds is reminted, references and
  table cells included (`core/api/objectcreateadapter.go:111`).
- **`GET /templates` is the read half of the collection `POST /templates`
  writes to**, since search excludes templates from every result by design.
  `?type=` narrows to one type's templates — including a built-in type the
  space has not installed, whose id is derived the way the create path
  derives it — and each row carries `default`, so which template a type
  starts from is visible where the templates are. The listing excludes
  uninstalled and hidden rows: offering an id the create path then refuses is
  the same broken loop pointing the other way
  (`core/api/v2/service/templates.go:279`).
- **A type's `default_template` is stored as a one-element list**, which is
  the spelling clients read, and is read back from either spelling.
  `PATCH /types/{type}` with `default_template: ""` clears it — the repair
  every stale-default warning names
  (`core/api/v2/service/schema_write.go:1017`).

#### The markdown channel on the create shortcut

- **The markdown authoring channel never fails to parse**: unknown constructs
  degrade to paragraphs, over-deep indents clamp, a line after a leaf block
  stays its sibling, and depth is bounded, so a parsed run always imports.
  Its scope is deliberately narrow: ATX headings only (a `---` line is always
  a divider), one quote level, two-space or tab list nesting, tables require
  their separator row, and images do not map to file blocks (file ids come
  from the upload route). A parsed run is capped at **256 blocks per
  `insert_blocks` op and 2048 on the create shortcut**, with the parse
  stopping early and the error naming the limit. `created_blocks` keys read
  `ops[i].markdown[j]`, j being the parsed position
  (`core/api/v2/service/schemas_ops.go:439`).
- **Validation issues from markdown-derived blocks on the create shortcut are
  readdressed** from the internal blocks path to `/markdown[j]`, because the
  caller never wrote a blocks array. Whitespace-only markdown is the same
  `markdown produced no blocks` refusal on the create shortcut as on the op
  path, never a silent empty-object success
  (`core/api/v2/service/create.go:585`).
- **A leading heading is the object's title, not the first line of its
  body.** A leading heading whose rendered text repeats the name the request
  set is dropped; with no name sent, a leading `heading_1` becomes the name
  and is dropped. A `heading_2` or `heading_3` can be a duplicate but is
  never promoted. Either outcome attaches a warning at `/markdown[0]`. The
  rule is the markdown channel's only: a full AnyBlock document is an
  authored block tree whose first block is a choice, not a convention
  (`core/api/v2/service/create.go:372`).
- **The rule reads the heading as MARKDOWN**, so `# **Title**` matches a name
  of `Title`, and it **stands down wherever acting would move a caller's
  content or invent a decision**: a heading that owns nested blocks is left
  whole, a heading carrying a link, mention, object reference or emoji is
  left alone (its rendering can equal the name while its content does not), a
  name the caller sent in any spelling is never replaced, a caller-set
  `layout` turns promotion off, and a note — whose name becomes its first
  block anyway, from the bundle for an uninstalled type too — is exempt from
  promotion while still dropping a heading that repeats its name
  (`core/api/v2/service/create.go:385`).

#### Files

**`POST /files` honours an optional `name` on its JSON (URL) form**: it is
applied after the file-derived metadata, so the caller's name wins on the
stored object, the create result and the search row. An absent or
whitespace-only name sends no details at all rather than an empty one, which
would blank the name the pipeline derived. Uploading bytes the space already
holds returns the existing object with its existing name — `name` in the
result is how a caller detects that — and the embedded file block keeps the
source-derived filename regardless (`core/api/v2/service/file.go:29`).

#### Deleting own output

- **`DELETE /objects/{object_id}` archives only objects the calling key
  created**, read from validated change storage: the tree's signed root is
  this account AND the first content change carries this account's identity
  together with the caller's app name, compared EXACTLY as raw text. Details
  are never consulted. It **fails closed** — no recorded app name, a nameless
  caller, a provenance read error or a missing provenance dependency all
  refuse with 403 `not_created_by_this_key` — so objects created in the app,
  by import, by another member, or before provenance existed are permanently
  undeletable through this route (`core/api/v2/service/delete.go:189`).
- **A positive allowlist of user content runs first**: pages (every
  layout-based object, including notes, tasks, bookmarks, sets and
  collections), templates, file objects and the store-backed chat objects.
  Everything else is refused before provenance is even read — workspaces,
  widgets, space views, participants, profiles, dates and every other system
  or derived surface, whatever their creation signature looks like. Type,
  property and option targets are steered to their own DELETE routes before
  that check (`core/api/v2/service/delete.go:147`).
- **Check order.** The space and write grants refuse before ownership is
  consulted. Deleting an already-archived object is a 200 no-op carrying an
  `already archived` warning. `?dry_run=true` is the deletability probe and
  runs every check this route owns — existence, steer, allowlist, grant,
  provenance — but **not** the archive-time restriction checks, so a
  "deletable" verdict can still meet a 403 on the real call
  (`core/api/v2/service/delete.go:106`).
- Because deletion compares app names, **key issuance requires a non-empty
  app name and rejects — never truncates — one longer than 128 bytes**, so no
  key can mint itself into permanently unattributable output. Names are
  compared byte-for-byte: a key paired as "Claude Desktop" cannot delete
  output created via "Claude/Desktop"
  (`core/domain/integrationname.go:39`).

### Phase 3 — edit

**Normative rule first (R5):** the post-op document must satisfy the format's
semantic checks (SPEC §12, V1–V5 — monotonicity, leaf containment,
row→column and column→row layout containment, bounds, id uniqueness). Any
violation rejects the **whole PATCH** with path-addressed errors (`ops[i]` +
block path). `move_block` into the moved block's own subtree is a cycle →
error. `update_block` changing a parent's type to a leaf type while
descendants exist → error naming the descendant count.

The edit API applies the editor's stricter **new-content containment policy**
to the file family (`file`, `image`, `video`, `audio`, `pdf`): these blocks
cannot be an `inside` target, and a parent with descendants cannot be changed
to one of those types. AnyBlock import/export remains intentionally lenient so
legacy documents that already contain descendants under file blocks still
round-trip without loss; API edits preserve that legacy data but cannot create
more of it.

**(a) `PATCH /v2/spaces/{space_id}/objects/{object_id}` — batched ops (default path)**

```json
{ "ops": [
    { "op": "set_properties", "set": { "status": ["Done"] }, "unset": ["oldKey"] },
    { "op": "update_block",   "id": "b5", "set": { "checked": true } },
    { "op": "replace_subtree","id": "b7", "blocks": [ { "type": "bulletedListItem", "text": "a" },
                                                     { "indent": 1, "type": "paragraph", "text": "b" } ] },
    { "op": "insert_blocks",  "after": "b3", "blocks": [ { "type": "checkbox", "text": "todo" } ] },
    { "op": "move_block",     "id": "b9", "inside": "b2", "position": "last" },
    { "op": "delete_block",   "id": "b4", "recursive": true }
  ] }
```

The op set is closed, id-addressed, atomic (one `state.Apply` per request),
with no positional/index/offset addressing anywhere (§3.1–3.2). It is:
`set_properties` · `update_block` · `replace_subtree` · `insert_blocks` ·
`move_block` · `delete_block` · `replace_text` · `set_cell` · `add_items` ·
`remove_items` · `update_view` · `insert_view` · `move_view` · `delete_view`
(`core/api/v2/service/ops.go:28`).

- **`update_block` (R4)**: THE one block-update op. Merge semantics — only the
  fields in `set` change; everything else (including `text`) is untouched;
  explicit `null` clears a field. The op for checkbox toggles, color/align
  changes, language switches, retypes and text rewrites alike.
  `replace_subtree` swaps block + descendants for the payload run.
- **Relative indent in payloads** (R3): for `after`/`before` and
  `replace_subtree`, payload `indent: 0` = the anchor's level. For `inside`,
  payload `indent: 0` = **the container's child level** (anchor + 1). Worked
  examples: sibling-insert —
  `{"op":"insert_blocks","after":"b3","blocks":[{"type":"paragraph","text":"same level as b3"}]}`;
  child-insert —
  `{"op":"insert_blocks","inside":"b3","position":"last","blocks":[{"type":"paragraph","text":"child of b3"},{"indent":1,"type":"paragraph","text":"grandchild"}]}`.
- **Targeting.** `insert_blocks` and `move_block` take **at most one** of
  `after`/`before`/`inside`; more than one is 400 `ambiguous_input` naming the
  fields supplied. `position` is legal only with `inside`, where it defaults
  to `last`, or with **no** targeting field at all, where it picks an end of
  the document; beside `after`/`before` it is refused `validation_failed` —
  the anchor already names the slot — and a value outside `first`/`last` is
  refused naming both allowed values, by one check covering both placements.
  `after` inserts past the anchor's whole subtree, and `move_block` carries
  the moved block's descendants and re-bases their indents
  (`core/api/v2/service/stateops.go:1660`,
  `core/api/v2/service/stateops.go:1671`).
- **Root targeting.** Omitting all of `after`/`before`/`inside` appends at the
  end of the document root. This is the ops-path into an empty object: SPEC §7
  keeps title/description out of the document, so a fresh object has zero
  addressable blocks. Payload `indent: 0` = the document's top level.
  `position: "first"` means the start of the **document**, not the state
  root's first child: it anchors before the first document block, so inserted
  content never lands above the title or the other structural header blocks,
  and it falls back to the append when the document has no block to sit
  before. `move_block` excludes its own subtree when it looks for that anchor,
  so moving the block that is already first is a no-op rather than a failure
  or an append at the other end (`core/api/v2/service/stateops.go:941`).
- **`delete_block`**: `recursive` defaults to false; deleting a block that has
  descendants without `recursive:true` → error naming the descendant count and
  the resolved block id (R14).
- **`match` — the id alternative on `update_block` and `delete_block`**: an
  exact substring of the block's text, which must appear in exactly ONE block
  or the op refuses — the same rule `replace_text`'s `find` follows, resolved
  per-op against the live document view under the object lock.
  `{"op":"update_block","match":"Draft timeline","set":{"checked":true}}`.
  Give `id` or `match`, **never both** — the combination is refused rather
  than ranked, and giving neither is refused too. Repeats *within* the one
  matched block are fine: `match` addresses a block, not an occurrence.
- **Each op contributes its own locator candidate set.** `replace_text`'s
  `find` scans only text-bearing blocks, because a block it could not edit
  must neither capture a match nor make a unique one ambiguous;
  `update_block`'s and `delete_block`'s `match` scans every block, because
  they address any block. **Table-cell text is never scanned by any of them**
  — cells are not entries of the blocks array and belong to `set_cell`
  (`core/api/v2/service/locator.go:55`).
- **`set_properties`** (R14): `set` writes presence — `"k": []` means
  present-but-empty (SPEC §3 presence-is-meaningful); `unset` removes
  presence. Output-only properties (SPEC §4a) are rejected with a
  path-addressed error. **A value the underlying 64-bit float model cannot
  carry is REFUSED** at its own `set`/`add`/`remove` entry rather than
  silently rounded, dropped or written as null. The favorite flag stays
  authorable and takes only `true` or `false`
  (`core/api/v2/service/stateops.go:1236`).
- **`set_properties` `add`/`remove`**: per-key list edits for list-shaped
  formats only (select, multi_select, objects, files — SPEC §3).
  `{"op":"set_properties","add":{"tags":["urgent"]},"remove":{"assignee":["bafy…"]}}`
  — `add` appends entries without duplicating existing ones; `remove` deletes
  matching entries, unsets the property when it consumes the final entry, and
  is a no-op when absent (never creates presence, never creates the option it
  names). Scalar-format keys are rejected with a path-addressed error naming
  the format. A key may appear in at most one of `set`/`unset`/`add`/`remove`
  per op. Rationale: appending one tag to a 40-entry multi_select used to
  require read → whole-array rewrite → write — the corruption pattern in
  miniature, plus a token tax.
- **`add_items` / `remove_items`** require a collection object — any other
  target is refused naming the object's type and pointing at the
  collection-create route. `add_items` de-duplicates, `remove_items` of an
  absent member is a no-op, and neither existence-checks the member ids it is
  given (`core/api/v2/service/stateops.go:2517`).
- **System-managed objects cannot be edited through the object surface**: a
  PATCH addressing a relation, a relation option, a file object or a
  participant is refused with a validation error naming the smartblock type
  and pointing at the endpoints those resources have of their own —
  properties, options, files and members
  (`core/api/v2/service/edit.go:545`).
- Block-id references accept full ids (canonical) and unique-suffix labels
  (lenient, C4).
- **Response**: new `etag`, `created_blocks` (ids the server minted, keyed by
  payload position — `ops[3].blocks[0] → "b1a2…"`), `created_views` (its twin
  for server-minted view ids, keyed by the same exact nested payload path),
  `diff_stats`, and `warnings` — the same warning-grade C6 channel reads use,
  so hazards an edit introduces (for example an unguarded date comparison
  that also matches undated objects) surface instead of passing silently. A
  client-supplied payload id resolves an existing block and preserves that
  identity, so it is absent from both maps. Both receipts follow `?ids=`:
  compact labels by default, full ids with `ids=full`
  (`core/api/v2/service/edit.go:434`, `core/api/v2/model/model.go:631`).
- **`diff_stats` is the canonical before-document versus after-document
  diff**, so import/export normalization cancels.
  `{blocks_added, blocks_removed, blocks_changed, blocks_moved,
  properties_changed}`: added/removed are set differences over block ids;
  `blocks_changed` counts ids present in both whose content differs (block
  JSON minus `indent` and `id`); `blocks_moved` counts blocks whose parent
  changed or whose nearest preceding sibling that exists in BOTH documents
  changed, so a pure insertion does not mark its followers moved;
  `properties_changed` counts keys that differ. Collection membership is
  reported separately by `items_added`/`items_removed`, present only when a
  batch changed membership (`core/api/v2/service/diff.go:93`).
- Implementation: build child state from live state, apply ops via
  `simple.Block`/`state.State` mutations, one `sb.Apply` — the pattern Block*
  RPC handlers use internally (research §2.3).

**PATCH order, preconditions and bounds**

- **A PATCH runs in a fixed order** — read the object, check the
  preconditions (system-type exclusion, then `If-Match`), then resolve any
  create-missing names, then take the object lock — so no create RPC ever
  runs while the edited object is locked, and a PATCH to a nonexistent,
  restricted or stale-etag target creates nothing. The object's per-axis
  Blocks/Details restriction verdicts come from that same locked read and are
  checked on dry runs too, so a dry run reaches the same 403 the real edit
  would (`core/api/v2/service/edit.go:176`).
- **The restriction gate is PER OP, not per request.** Each op declares which
  object-level restriction axes it touches, and a refusal addresses the first
  offending op (`/ops/i`) rather than the whole request, so a batch mixing a
  legal rename with an illegal block edit says which op is the problem. A
  restriction refusal is a **403 `forbidden`** whose issue states that the
  refusal is permanent for this object and must not be retried — never a 500,
  which sends retrying agents into loops on a refusal that can never succeed
  (`core/api/v2/service/ops.go:119`).
- **Per-op classification**: `set_properties` needs the **Details** axis;
  `update_block`, `replace_subtree`, `insert_blocks`, `move_block`,
  `delete_block`, `replace_text` and `set_cell` need the **Blocks** axis; and
  `add_items`/`remove_items` and the whole view family need **neither**. Item
  ops mutate the collection store, which no object restriction governs; view
  configuration is likewise ungated natively. Classifying either group as a
  block edit would refuse it on precisely the objects it exists to edit —
  sets, collections and object types all carry the Blocks restriction
  (`core/api/v2/service/ops.go:57`).
- **Whole-document validation of the would-be after-document runs on every
  PATCH by default and REJECTS on failure**, restoring the invariants no
  single fragment can see (row→column containment, the document-wide id
  domain, the absolute nesting bound). The refusal message is `the ops would
  produce an invalid document — no op was applied`.
  `ANYTYPE_API_V2_SKIP_EDIT_VALIDATE=1` disables it for debugging a suspected
  false rejection; there is no log-only mode
  (`core/api/v2/service/stateops.go:51`).
- **One PATCH may bring at most 64 new select/multi_select options into
  existence, and the irreversible creates go LAST.** A probe pass resolves
  without creating, so an over-cap batch is rejected for a single JSON walk
  and no RPC, with the count, the limit and the properties involved named.
  When a batch does name new options, the whole batch is first applied
  against a private state, so an op that cannot apply is found before any
  create fires and a failing batch leaves no debris. A dry run reaches the
  same verdict. Created options carry the `api` origin, and option resolution
  adopts an existing option of the same name before creating, so a retry
  converges rather than duplicating (`core/api/v2/service/edit.go:254`).
- **Batch bounds**: at most **512 ops** per PATCH, at most **256 blocks** in
  one op's block payload (the `maxItems` the `insert_blocks` and
  `replace_subtree` schemas advertise, shared by the markdown channel), and
  the batch's worst-case marshal work bounded at **2^20 block-renders**. The
  work bound counts view-rebuilding ops × (document blocks + payload blocks, a
  markdown payload counted at the per-op cap), with a dataview weighted by its
  views' columns, sorts and filters and every `insert_view` adding the
  document's heaviest per-view weight. An over-bound batch is refused whole
  before any op applies, with the numbers in the error and a hint to split the
  edit across several PATCH requests — the object is released between batches.
  The check sits on the shared apply path, so the create-missing probe, the
  dry run and the locked run reach the same verdict
  (`core/api/v2/service/edit.go:45`).
- **`replace_text` is exempt from the render-work bound**: it maintains the
  document view in place instead of forcing a whole-document re-marshal,
  changing exactly one exported field of one block and writing the
  **canonical** rendering a re-marshal would emit, so the next op's `find`
  matches what a caller would read back. Every other op invalidates the view
  and is counted by the bound (`core/api/v2/service/ops.go:151`).

**Payload ids**

- **Every empty id slot in a PATCH payload is minted by the API, not by the
  format importer**, and reported under that slot's own payload path — nested
  `rows`/`columns` entries, blocks inside cells and views included, for
  example `ops[0].blocks[0].rows[1]`, `ops[0].value[1]`,
  `ops[0].set.views[2]`. A minted view id is reported in `created_views`;
  everything else in `created_blocks`. This is what makes the refusals'
  promise ("omit the id and the server returns what it minted") true for
  every slot they fire on (`core/api/v2/service/payloadids.go:319`).
- **Reference slots**: an id matching no id is a 404 naming the addressable
  blocks and the outline read; an id that is a suffix of several ids is a 400
  `ambiguous_input` asking for the full id (reference slots do not enumerate
  the candidates). **Payload slots** answer the same two refusals, where the
  ambiguity additionally lists the matching ids (bounded at 8), an id that
  resolves to nothing is told to omit the id so the server mints one and
  reports it, and an id that resolves to an element the op may not reuse is
  refused as a duplicate — naming both the spelling sent and the id it
  resolved to when a compact label was used
  (`core/api/v2/service/payloadids.go:382`).
- **"Already exists in this object" has one definition on the edit path**,
  used by both the payload id resolver and the collision guard: every state
  block (including blocks the served document does not carry, such as the root
  and header) together with every doc-local id of the pre-op document that is
  not a block — a dataview's view ids. A payload may therefore not adopt a
  live view's id, and two views may not be stored under one id. The ids an op
  may reuse are the ids of the subtree it replaces, view ids included, so
  echoing a dataview block back keeps its views
  (`core/api/v2/service/payloadids.go:121`).
- **A view id is unique within its dataview block, not document-wide**: the
  intra-payload duplicate scan for views runs per dataview block while block,
  row, column and cell ids share one document-wide scan — so one payload may
  author two dataviews carrying the same view id. The existence guard is still
  document-wide, so a payload may not adopt a view id another dataview already
  holds (`core/api/v2/service/stateops.go:803`).
- **A field in which every value would be an error does not appear in that
  op's published schema**; the runtime refusal is a backstop for the caller
  who ignored the schema, never the mechanism. Applied to payload ids: the
  new-content payload block (`insert_blocks`) publishes no `id` — not on the
  block and not on its `rows`/`columns` entries, which are themselves strict
  typed defs — while the existing-content payload block (`replace_subtree`)
  publishes it, because naming the block being replaced is what makes echoing
  a read back a no-op instead of a rename. An id sent to a new-content payload
  is refused as **not part of that op**, rather than reported as a duplicate
  or an unresolvable reference. The set of new-content ops is stated once and
  read by both the schema builder and the applier
  (`core/api/v2/service/schemas_ops.go:159`).

**(b) There is no full-document replace**

There is no `PUT`. **PATCH is the edit surface**, and the principle is:
*snapshots are for creates, edits are ops.* A `?block=` subtree read is
marked `"subtree": true` and no write path accepts it (create names it by
path and refuses it, Phase 2). `?ids=full` is the **backup/export shape**
(C4) and the id vocabulary a clone-from-read POST should use.

**(c) `replace_text` — str_replace scoped to one block's `text`**

```json
{ "op": "replace_text", "id": "b2", "find": "Q3", "replace": "Q4" }
{ "op": "replace_text", "find": "Q3 report", "replace": "Q4 report" }
```

Exact-match within one block, must match exactly once, Anthropic-style errors
("found 2 matches — provide more context"), `replace_all` escape hatch. It is
the `edit_text` wrapper tool's backing primitive (§7); deferring it forces the
commonest edit (change-one-word) through whole-block verbatim reproduction —
the documented 3B collapse mode. **It applies only to text-bearing blocks** —
a non-text target is refused path-addressed — and its `find` must be
non-empty (`core/api/v2/service/stateops.go:2121`).

**`id` is optional: `find` doubles as the locator.** Omitted, the find text
must appear in exactly ONE text-bearing block or the op refuses — zero
matches 404 with the outline steer, several matching blocks are
`ambiguous_input` listing ≤8 candidate block ids with context, several
occurrences within the one matched block get the more-context refusal.
Resolution runs per-op against the applier's live document view under the
object lock, so op *i* locates against op *i−1*'s edits, and a dry run
resolves identically (C9, advisory). **`replace_all` composes with the
locator rather than widening it**: the block is resolved first, from the op's
own candidate set, so a find text appearing in two blocks still refuses —
`replace_all` only governs repeats inside the one matched block
(`core/api/v2/service/stateops.go:2099`).

**(d) `set_cell` — scoped table-cell write**

```json
{ "op": "set_cell", "table_id": "t1", "row": "r2", "col": "c1", "value": "done" }
```

`value` takes a string (paragraph-cell shorthand), `null` (clear), a block
object (SPEC §6.1 cell forms), or a flat array whose first element is the cell
block; a cell block takes no id. Row and column references resolve by exact id
or unique suffix like block references, and additionally by the row's
first-cell text or the column's header text (case-insensitive, one match or
refuse); an invalid inner shape is refused at the op's `value` path
(`core/api/v2/service/schemas_ops.go:473`). Flat and non-recursive — trivially
grammar-constrainable.

**(e) The view family — `update_view`, `insert_view`, `move_view`, `delete_view`**

Views are part of the object's document, so the edit slot is
`PATCH …/objects/{id}` — which reaches sets, collections, object types (by
the id `GET …/types/{key}` returns) and inline dataviews alike. The four ops
introduce no new grammar: they are the block family's verbs, view-scoped,
sharing `update_view`'s `set` and `columns` channels and its block/view
addressing, and `insert_blocks`/`move_block`'s targeting words. The names are
singular — a view has no internal structure, and several views are several ops
in the already-atomic batch.

- **`update_view`** — `{op, block?, view?, set?, columns?}` with at least one
  of `set`/`columns`. `block` defaults to the object's only dataview and
  `view` to the only view; both resolve by full id or unique suffix. `set`
  merges view-level fields with `update_block` semantics (named fields change,
  explicit null clears one), `sorts` and `filters` replace whole, and `filter`
  is the compact-string alternative to `filters` (supplying both is
  `ambiguous_input`). `columns` merges **per column** keyed by property key — a
  patch merges `{hidden, width, align, aggregation}`, a key with no column
  appends one, and null removes one (removal is deliberately not
  key-validated, so a stale column for a deleted property stays removable).
  `id` is immutable, `set.columns` is steered to the columns channel, and
  `groups`/`object_orders` are rejected as output-only yet survive the edit
  untouched. All validation runs against a private copy first, so a failing op
  leaves the state and the view unchanged (`core/api/v2/service/viewops.go:109`).
- **Vocabularies and key membership.** View-op values validate against
  vocabularies the format package exports (view types, card and list sizes,
  column align and aggregation) rather than against the codec's silent enum
  defaults, and sorts and filters validate through the exported read-only
  fragment codec with issues rebased onto the op's own path. Property keys a
  view patch introduces — columns, sorts, filter leaves, `group_by`,
  `cover_property`, `end_property` — must be known to the dataview (its
  properties list plus every view's columns, taken pre-merge) or to the space,
  and are otherwise refused with did-you-mean; a resolvable key is appended to
  the dataview's properties list with its format so formats rehydrate. This
  membership rule is deliberately looser than `POST /queries`'
  type-recommended-keys rule, because generated views already carry columns
  outside that set and an edit surface must not reject what the surface
  already shows (`core/api/v2/service/viewops.go:875`).
- **Bounds**, all advertised by the served op schemas and enforced: columns
  ≤ 64, sorts ≤ 10, a sort's `custom_order` ≤ 128 entries, `set.filters` ≤ 32
  top-level nodes (nesting stays the documented recursive exception), compact
  filter string ≤ 4096, `page_size` ≤ 1000, column width ≤ 10000 px, name
  ≤ 4096, keys and ids ≤ 256 (`core/api/v2/service/viewops.go:39`).
- **`insert_view`** is `update_view` aimed at a fresh view: the base is either
  sensible defaults or a `copy_from` duplicate of another view of the same
  dataview, and `set`/`columns` then merge on top through the same code paths,
  so vocabulary, filter gates, key validation, warnings and option
  pre-resolution all hold for create. `name` is required on the op (≤ 4096)
  and `set.name` is rejected with a steer, since two name slots would silently
  override each other. View ids are always server-minted — the payload has no
  id slot and `set.id` is rejected — and each minted id is returned in
  `created_views` keyed by the op's payload position. The bare default is a
  view someone can look at: one column per property the dataview lists, ALL
  visible, sorted by last-modified descending; `copy_from` duplicates columns,
  sorts, filters, type, `group_by`, card options and per-view editor state,
  everything but id and name (`core/api/v2/service/viewops.go:1202`).
- **`move_view`** takes `view` plus `after`/`before`/`position: first|last` —
  the `move_block` vocabulary minus `inside`, views being a flat list — and
  **requires** a destination: the front of the list is what a fresh client
  opens (active view is per-device local state), so a target-less move is a
  forgotten field rather than an intent, and `position: "first"` is the
  documented "make this the default tab" verb. The splice adjusts the target
  index across the removal, and moving a view relative to itself is a no-op
  rather than an error (`core/api/v2/service/viewops.go:1317`).
- **`delete_view`** refuses to delete a dataview's last view with a 400 naming
  the invariant and hinting at the repair; the guard counts the BATCH's state,
  not the original document, so insert-then-delete in one PATCH is legal and
  is how a type's default view is replaced atomically. Deleting a view some
  client had active is deliberately unhandled server-side — that client falls
  back to the first view — and per-view editor state vanishes with its view,
  so orphaned group or object orders are structurally impossible
  (`core/api/v2/service/viewops.go:1368`).
- **Unauthored views are restored, not round-tripped.** A view op re-imports
  the edited dataview block with a **no-create** option resolver and then
  restores from the live document everything the op did not author. Exported
  view JSON writes option values as NAMES, so a naive round trip would
  re-resolve every untouched view's filters and custom sort orders: a dangling
  reference would mint a brand-new option named after the raw id, twins sharing
  a name would repoint by store listing order, and those creates would fire
  under the object lock outside the create-missing bound. `move_view` and
  `delete_view` author nothing, so every surviving view is restored
  byte-for-byte and only order and membership come from the splice;
  `update_view` and `insert_view` author one view, whose sorts and filters
  restore from the live (or copy-source) document unless the op's `set`
  actually named them. Option names the op itself authors are created by the
  pre-lock pass, so they resolve; anything the pre-lock pass cannot see is
  content the op has no business minting for and passes through verbatim
  (`core/api/v2/service/viewops.go:191`).
- **A view's `sorts[].id` is a different id domain** from block and view ids:
  output-only on reads and accepted back unchanged, so a read of a view
  round-trips through `update_view`/`insert_view`. The compact filter string
  validates its keys against the same membership the structured form uses, so
  the recommended input form never rejects a key the structured form accepts
  (`core/api/v2/service/schemas_ops.go:315`).
- **Removing a filter** is done by sending the plural key empty —
  `{"filters": null}` or `{"filters": []}` clears it. The compact singular
  channel cannot clear: `{"filter": null}` and `{"filter": ""}` are both 400s,
  because an empty compact filter is not an expression. (In the task-tool
  wrapper the words `none`, `all`, `clear` and `any`, any case, mean "no
  filter" for the same argument.) (`core/api/v2/service/viewops.go:348`)

**(f) Deferred, additive later** (closed op set, versioned):
`replaceProperties` full-map swap · cross-object batch · block-scoped
preconditions (C7 note).

### Phase 4 — query

```
POST /v2/spaces/{space_id}/search        (+ POST /v2/search global)
GET  /v2/spaces/{space_id}/queries/{query_id}/objects?view={viewId}&fields=…
GET  /v2/spaces/{space_id}/queries/{query_id}/views
GET  /v2/spaces/{space_id}/collections/{collection_id}/objects?fields=…
GET  /v2/spaces/{space_id}/collections/{collection_id}/views
```

Primary worked example (single-filter form — the small-model form, C12):

```json
{ "query": "report", "type": "task",
  "filter": "done = false AND (dueDate < currentWeek() OR dueDate IS EMPTY)",
  "sorts": [ { "property": "due_date", "direction": "asc" } ],
  "fields": ["name", "dueDate", "status"] }
```

Secondary example — programmatic composition (the structured array):

```json
{ "type": "task",
  "filters": [ { "property": "done", "condition": "equal", "value": false } ],
  "sorts": [ { "property": "due_date", "direction": "asc" } ] }
```

- `filter` (compact string) and `filters` (structured array) are mutually
  exclusive; **both supplied → 400 `ambiguous_input`** (R15). Both forms land
  on **one internal tree** (the SPEC §6.2 filter node).
- **Request-shape conventions**: the sort field is **`sorts`** — the SPEC §6.2
  view name and the shipped `POST /queries` name; one concept two names is
  exactly the duality C2 bans. Pagination is the **C10 query params**
  (`offset`/`limit`, default 25, `has_more`) like every shipped v2 list — no
  body `limit`, which the strict request schema rejects as an unknown field.
- **Search is a read** (POST only because the request needs a body): exempt
  from `Idempotency-Key` (registration is per-route, so the middleware is
  simply not attached) and from `dry_run` — a supplied `dry_run` is
  **ignored** (a read is its own dry run; erroring would punish a harmless
  habit).
- **The filter-string parser** (R6) is the SPEC §6.2.1 grammar shipped as a
  library; it parses to the §6.2 structured tree with offset-addressed parse
  errors naming the offending token and position, with did-you-mean. The
  string uses RFC 3339 dates / preset functions; the structured form uses unix
  numbers — the §6.2.1 mapping applies — except where the filter is STORED (a
  query's views, `update_view`), which also takes an RFC 3339 or `YYYY-MM-DD`
  string and stores the seconds (R6-1).
- **Date values by channel.** On the SEARCH path the structured `filters` form
  takes **unix seconds** for a date-formatted property: a string value is
  refused path-addressed at `/filters/{i}/value`, spelling out the number to
  send and steering to the filter string or a date preset. Where a filter is
  STORED, an RFC 3339 or `YYYY-MM-DD` string is accepted and converted on
  write, because a stored string would silently compare string against int64
  for good. Presence predicates and counting presets keep their values
  untouched (`core/api/v2/service/search.go:790`).
- **The structured `filters` array is shape-validated before it reaches the
  store**, because three malformed shapes otherwise degrade silently into
  MATCH EVERYTHING: a node carrying both the group and the leaf arms, a group
  with an empty `filters` array, and a leaf with no `condition` (which a
  misspelled key also produces). Each is a path-addressed 400 whose hint
  states that the shape matches every object. The gate runs on the query path
  AND on `POST /queries` and on a view op's `set.filters`, because a persisted
  match-everything filter is a saved query that quietly contains the whole
  space. The served schema matches the enforcement: the leaf arm requires
  `condition` and the group arm's `filters` carries `minItems: 1`. The shared
  document codec is deliberately untouched and keeps accepting condition-less
  leaves, since stored dataviews legitimately carry them
  (`core/api/v2/service/search.go:706`).
- **Validation & resolution rules:**
  1. **Key scope.** With a top-level `type`, filter/sort/field keys validate
     against the type's recommended keys + `name` (`typePropertyKeys`) plus
     the system allowlist below. Without `type`, and on global search, keys
     validate against the **space's property keys** (per space, for global).
     Unknown keys → path-addressed did-you-mean.
  2. **System-key allowlist.** `createdDate`, `lastModifiedDate`, `creator`,
     `lastOpenedDate` — output-only/system keys that appear in no type's
     recommended lists yet back bread-and-butter queries. Always part of the
     query-surface reference set — for search AND for set filters/sorts.
  3. **Option names resolve READ-ONLY on the query path.** SPEC §3's
     create-missing is write/import behavior; a query must never mint the very
     option it names. Unresolved names → did-you-mean error, never a silent
     no-match.
  4. **Global search resolves per space.** Type keys and option names resolve
     inside each space's loop iteration; a name that resolves in only some
     spaces queries those spaces and carries a C6 warning naming the spaces
     where it did not. Queries use the one-shot `ObjectCrossSpaceSearch`
     method over loaded, granted spaces, grouping identical resolved filters
     and sorts. The result includes one lookahead row: `total` is a lower
     bound when clipped, and `has_more` reports whether another page is
     available.
  5. **Empty-date hazard surfaces.** SPEC §6.2: an unguarded `less`/
     `lessOrEqual` date comparison matches undated objects. Document import
     warns; the search path does too — the same warning text rides the C6
     `warnings` channel.
  6. **`type` is a filterable pseudo-key.** The top-level `type` stays a
     single type key (the small-model form, C2). Multi-type queries use the
     filter channel: `type IN ("task", "bug")` — resolved key→id server-side
     like any reference. A top-level `type` and a `type` filter compose by AND.
- **The query channels accept what the listings advertise.** Search `fields`,
  structured filters, the compact filter string and sorts, list `?fields=`,
  and a set's persisted filters/sorts/views are all canonicalized from the
  served spelling to the **stored key** before validation and before
  persistence — a served slug written verbatim into a stored dataview filter
  would become a permanently dead filter. Membership acceptance is WIDER than
  advertising (stored keys, served spellings and bundled derived slugs all
  resolve), while candidate lists and did-you-mean speak served spellings
  only, so an error never advertises a spelling the channels reject. A `type`
  filter leaf resolves through the same chain and is corpse-aware: a deleted
  type is not a usable query scope (`core/api/v2/service/keycanon.go:46`).
- **Effective sort list**: explicit `sorts` win; a full-text query without a
  relevance sort gets `_final_score desc` appended as a tiebreak (which also
  stops the engine prepending its own score sort, keeping explicit sorts
  primary); a full-text query with no sorts is pure relevance; neither query
  nor sorts falls back to `lastModifiedDate desc`, the list-objects order. The
  same list drives the global merge comparator
  (`core/api/v2/service/search.go:435`). A sort on a date-formatted property
  that omits `includeTime` defaults to **second** granularity, matching the
  default `lastModifiedDate` sort, so ordering does not change with the
  presence of the full-text tiebreak; an explicit `includeTime: false` is
  honored (`core/api/v2/service/search.go:415`).
- **Row scope** mirrors the object list: object layouts only, no templates, no
  hidden objects, with archived and deleted excluded by the store's defaults —
  widened to object-and-file layouts only by the file-layout opt-in a file type
  in the `type` channel triggers (`core/api/v2/service/search.go:461`). The
  opt-in is keyed off the type channel on both request forms — positive type
  leaves and the top-level type — with a negation-scoped condition set, and it
  brings the `mimeType`/`size` field aliases, which live in filters and sorts
  too and are shadowed per space by a real property of the same name.
- **A full-text search pushes `Limit = offset + limit + 1`** into the store so
  the engine's candidate-budget escalation sees the requested page. The extra
  record makes `total` exact when the store held fewer matches and a lower
  bound with `has_more: true` when it clipped; truncation beyond the engine's
  candidate hard limit remains a documented approximation
  (`core/api/v2/service/search.go:122`).
- **Global search pages at most 2000 rows deep**: a larger `offset` is a 400
  steering to the space-scoped search, which pushes offset and limit into the
  store. Each global row carries `space_id` as addressing information for the
  follow-up read. A space whose plan fails to compile is skipped with a
  `space "X" was skipped: …` warning and stays out of the results; only when
  NO space resolves does the first per-space error become the response. A
  space whose store is not loaded produces an incomplete-results warning
  rather than an error (`core/api/v2/service/search.go:1031`).
- **An unknown entry in `fields` is handled differently by scope**, because
  fields are display and never scope: on the space-scoped search it is a 400
  with did-you-mean, while on the global fan-out it degrades to a per-space
  warning naming the omitted column and the space stays in the results and the
  total. Unknown filter and sort keys keep the skip-with-warning behaviour on
  the global path (`core/api/v2/service/search.go:178`).
- **Stored-view execution (`?view=`) substitutes dynamic placeholders.**
  SPEC §6.2's `_filter_template_<n>_` values are client-substituted and opaque
  to the middleware — a query evaluated server-side against the literal string
  matches nothing. The handler substitutes `_filter_template_2_` → the
  caller's participant id (`_participant_<space>_<account>`) and
  `_filter_template_1_` → the hosting object id before building the store
  query; any other placeholder degrades to a C6 warning, never a silent
  no-match.
- **On the query and collection read routes**, `?view=` resolves by exact view
  id or unique suffix — zero matches lists the stored view ids, several steer
  to the full id — and `?fields=` is validated like search's, with a 400 and
  did-you-mean at path `fields` so a typoed key never degrades to rows that
  silently carry no properties. A query whose `setOf` source is empty or
  unresolvable is an explicit 400 saying the query matches nothing, never an
  unscoped full-space read. A collection with no stored-view sorts reads in
  store-slice order, and a stored view's sorts override membership order
  (`core/api/v2/service/list_read.go:469`).
- **Sets AND collections both get a read path.** `queries/{id}/*` requires a
  query (its dataview source drives it), `collections/{id}/*` requires a
  collection (membership rows = the store slice, in its order); one handler
  branches on layout, and a wrong-layout target is a 400 naming the other
  route. Rows follow C5.
- **Small-model form is settled** (C13): the structured `filters` array is
  recursive and not constrained-decodable, so the string is the documented
  default for small models; the array serves round-trip and programmatic
  composition (both ship — B2 only tunes steering, §4).
- Sort by any property key. **[B3]** `resultFormat=rows` stays gated.

### Phase 5 — the task-tool wrapper (CLI + skill + on-device manifest)

Built — `core/api/wrapper` (the tool table + manifest + runner) and
`cmd/anytype` (the verb-set) + `cmd/anytype/SKILL.md`. The contract is §7.

The Phase-5 deliverable is the **task-tool wrapper**: the curated tool layer
over `/v2`, delivered as (a) CLI verbs with AXI/Chow output conventions and
SKILL.md three-tier packaging for coding-agent harnesses, and (b) a
function-calling/MCP manifest of the same tools for on-device small models.
Both are thin over the same server primitives; bulk work via scripts.

- **Dependency status.** Phase-4 search and the filter-string parser back
  `find`, and the markdown→flat-blocks parser backs `add_blocks` and the
  single-change-set create shortcut. `GenerateSchema` plus the store-backed
  option join remains a 501 stub; `describe` therefore uses the sanctioned
  degraded wrapper-side composition from the type document and property
  option lists.
- **CLI verb naming**: `move-block`/`delete-block`, matching the tool names —
  a plain `delete` would be read by coding agents as *object* deletion. The
  REST object-archive route is built, but the wrapper does not expose it; a
  future wrapper verb should be named `archive`.

### Phase 6 — chat

```
GET/POST   /v2/spaces/{space_id}/chats
GET/POST   /v2/spaces/{space_id}/chats/{chat_id}/messages
GET        /v2/spaces/{space_id}/chats/{chat_id}/messages/stream   # SSE, when enabled
PATCH/DELETE /v2/spaces/{space_id}/chats/{chat_id}/messages/{message_id}
POST       /v2/spaces/{space_id}/chats/{chat_id}/messages/{message_id}/reactions
POST       /v2/spaces/{space_id}/chats/{chat_id}/read
```

- Every chat-scoped route first resolves the chat id in the store: an unknown
  id is a 404 steering to the chats list and a non-chat layout is a targeted
  400. `GET /chats` is a pure store query over the chat layouts — it opens no
  chat — and rows are the C5 pair `{id, name}`. `POST /chats` requires a
  non-empty name, because an unnamed chat is unaddressable in that row
  (`core/api/v2/service/chat.go:56`).
- **The messages read** returns `{messages, state, message_count,
  lifetime_message_count, has_more, next_after?, next_before?}`.
  `message_count` is how many messages the chat HOLDS now (a deleted one
  leaves it) and `lifetime_message_count` counts every message ever posted;
  neither is the size of the requested range. `has_more` says more messages
  exist inside the requested bounds and is detected by fetching limit+1.
  Messages are always served ascending by order id whichever way the range was
  walked. **Paging is by cursor only**: `?offset=` is refused with a 400
  steering to the cursors, `after` alone walks forward and continues from
  `next_after`, and every other query anchors at the newest end and walks
  backward from `next_before` — `after` together with `before` does not
  advance a forward cursor (`core/api/v2/model/chat.go:77`).
- **Message text crosses the API as inline MARKUP SOURCE in both directions**
  — rendered on read, parsed on write, with mentions as
  `<mention object_id="…">` tags — and offset mark arrays never appear on the
  wire. `style` is dropped on read and not accepted on write: a new message is
  always a paragraph and an edit preserves the stored style. Block-composed
  content (quotes, rich pastes) surfaces read-only as `blocks_text`, the
  text-bearing blocks rendered as markup and newline-joined; link and embed
  blocks stay invisible (`core/api/v2/model/chat.go:200`).
- **`reactions` is ALWAYS the counts map** (`{"👍": 2}`); `?reactions=full`
  adds `reacted_by`, a separate slot carrying participant-id lists — the same
  vocabulary as `author_id`, never raw identities — so neither field ever
  changes type. `at` and `edited_at` are RFC 3339 UTC strings, `edited_at`
  present only when the message was edited, and sync/read flags are
  deliberately absent from the DTO (`core/api/v2/model/chat.go:34`).
- **The chat surface is exempt from C7**: no chat response carries an `etag`
  and no chat mutation reads `If-Match`. Order ids and `last_state_id` are the
  chat's native concurrency vocabulary, and the exemption is documented at the
  chat router, service and handler headers (`core/api/v2/router.go:259`).
- **`POST /chats/{chat_id}/read` requires BOTH `up_to`** (the inclusive order
  id) **and `last_state_id`** (the race guard) for the `messages` and
  `mentions` scopes — an empty value for either selects nothing and would
  succeed while marking nothing — and a missing field is a 400 addressed to
  that field. Both values ride the same messages response, so requiring them
  costs no extra call. The server never fills the guard itself: a guard
  resolved at write time is newer than the caller's read and would mark
  late-arriving unseen messages as read. The `reactions` scope is
  all-or-nothing and REJECTS `up_to` and `last_state_id` path-addressed rather
  than pretending a bound exists. There is deliberately no mark-all form
  (`core/api/v2/service/chat.go:424`).
- **Chat RPC failures are classified into the C6 shape** rather than surfaced
  as retry-looping 500s: a `validate:` description becomes 400
  `validation_failed`, a `not found` description becomes 404, and the
  foreign-message refusals — matched through the producers' exported sentinels
  so a rewording breaks the build — become 403 `forbidden`. Anything else
  stays 500 carrying the description. Before any RPC runs, v2 enforces what it
  owns: parsed text longer than **8000 UTF-16 code units** is a 400 at
  `/text`, and more than **32 attachments** is a 400 at `/attachments`
  (`core/api/v2/service/chat.go:653`).
- **`PATCH` on a message is a read-merge, not a replace**: the service reads
  the message first and carries its style, attachments, reply target and
  blocks through unchanged, so a text edit never wipes attachments. An emoji
  MARK is materialized into its literal emoji on read and the re-parse does
  not re-mint it, so a read-then-PATCH round trip leaves the emoji as plain
  text (`core/api/v2/service/chat.go:264`).
- **Attachments are bare object ids**, at most 32 per message, enforced before
  any store lookup. The kind is inferred from the target's layout — an image
  layout gives an image, other file layouts a file, anything else a link — and
  an id that resolves to nothing is a path-addressed 400 steering to the file
  upload route rather than a silently broken message
  (`core/api/v2/service/chat.go:43`).
- **Deleting a message permanently deletes — with no bin — the attachment and
  link targets the delete orphans**, asynchronously and after the API has
  replied. Both the dry run and the real receipt carry C6 warnings naming the
  ids at risk. Delete and the reaction toggle run their existence read on the
  committing path as well as the dry run, so a missing message 404s
  identically either way rather than answering 200 for a deletion that never
  happened (`core/api/v2/service/chat.go:324`).
- **Author names** are resolved from the space index by the participant's
  deterministic id, memoized per read; an unindexed participant degrades to an
  empty name. The reaction toggle's predicted `added` is optional: with no
  account identity wired it is omitted and a warning takes its place, because
  nothing would match the stored reactions and the prediction would be a coin
  flip (`core/api/v2/service/chat.go:524`).

**Streaming (`GET …/messages/stream`)**

- The route opens a **Server-Sent Events** connection: it begins with the last
  `limit` messages as `message_added` events, then delivers `message_added`,
  `message_updated`, `message_deleted`, `reactions_updated`, `state_updated`
  and `pinned_updated` as they happen, holding idle connections open with
  comment lines. It is a **READ**: the chat is verified to live in the named
  space, `limit` comes from the shared pagination bounds like every other
  list, and the key's read grant is required
  (`core/api/v2/model/chat_event.go:11`).
- **Only ADDITIONS are resumable.** Each added message carries its chat state
  id as the SSE event id and `Last-Event-ID` replays what is newer; an edit, a
  deletion, a pin or a reaction carries **no id**, because a state id is
  stamped once at creation and re-emitting it would drag a client's cursor
  backwards. A client that reconnects therefore keeps its stale copy of
  anything edited, deleted, pinned or reacted to during the gap until it reads
  the messages again. The opening window is ordered by order id rather than by
  state id, so it is followed by one id-only frame carrying the highest state
  id in it, withheld when that would move the client backwards. A cursor is
  meaningful only against the node that issued it
  (`core/api/v2/handler/chat_stream.go:100`).
- **`resync_required`** says the retained messages could not be shown to cover
  the gap since the client's cursor: the events that follow are a fresh window
  and anything held outside it is unverified. It is a warning and **not** the
  converse guarantee — a message that arrived out of order, or a chat that was
  reindexed, can leave a gap the test cannot detect — so a client that needs
  certainty re-reads. The same event is sent when the producer drops a
  subscriber for reading too slowly, in place of a silent close
  (`core/api/v2/service/chat_stream.go:200`).
- **Open streams are capped process-wide**, and exhausting the cap is **429
  `too_many_streams`** — deliberately not the shared limiter's
  `rate_limit_exceeded`, because retrying cannot succeed and the caller has to
  close something instead. `pinned_updated` carries only the message id and
  the flag, never a message body, since the pinned message is commonly outside
  the subscription's window (`core/api/v2/service/chat_stream.go:128`).

### Phase 7 — spaces

```
GET   /v2/spaces/{space_id}      # RPC-free tech-space-view read
POST  /v2/spaces                 # ONE WorkspaceCreate call
PATCH /v2/spaces/{space_id}
```

- **`PATCH /v2/spaces/{space_id}` requires at least one of
  `name`/`description`** — an accepted empty update would let a caller believe
  it renamed something — and omitted fields stay unchanged. A present-but-empty
  `name` is rejected while `description: ""` clears. An unknown space 404s
  before body validation. The response overlays the patch onto the current
  view row instead of re-reading, because the view sync would race an immediate
  read-back. **The space surface is exempt from C7**: it carries no etag and
  ignores `If-Match`, so concurrent renames are last-write-wins
  (`core/api/v2/service/space.go:246`).
- **Every space-addressing surface serves LIVE spaces only**, judged on the
  two-axis view predicate (local status and account status): the single-space
  read, the spaces list, the global-search fan-out and every space-scoped
  route. A retained view for a deleted, left or still-joining space 404s or is
  filtered out rather than exposing stale nested data. The one asymmetry is
  that internal space-scoped calls may address the tech space, which the
  single-space read never advertises (`core/api/v2/service/space.go:100`).
- **Workspace RPC failures are classified on their description**, like chat
  failures: the space-not-exists, space-deleted and storage-missing sentinels
  become 404 `not_found` steering to the spaces list, the restriction sentinel
  becomes 403 `forbidden`, a BAD_INPUT code becomes 400 `validation_failed`
  carrying the description, and everything else stays 500. The matched strings
  come from the producers' exported sentinels so a rewording updates the
  matcher at compile time (`core/api/v2/service/space.go:286`).

## 3. Decisions ledger

**Decided**: id-addressed closed op set, no RFC 6902, no index/offset
addressing · `update_block` merge op + `replace_text` + `set_cell` in the
launch op set (they back wrapper tools — §7/S1) · the four view ops on the
same PATCH channel · **no full-document replace at all** (*snapshots are for
creates, edits are ops*), compact edit reads with an explicit full-id export
shape, `diff_stats` · flat AnyBlock as the primary content representation on
the REST write path, plus **one markdown-in alternative: an `insert_blocks`
`markdown` payload** (mutually exclusive with `blocks`, same targeting incl.
root-append — the server parses; it backs the wrapper's `add_blocks` channel
and the create shortcut); markdown read-only otherwise · compact object ids +
full block ids on REST reads / **short handles on wrapper reads** · one
vocabulary incl. `type` (C2) · per-endpoint schema + worked example,
strict-mode-compatible (C12/C13) · path-addressed errors + `/validate` +
`dry_run` + idempotency · etag advisory by default, with chat and spaces
exempt · filter string as the small-model filter form (SPEC §6.2.1 scope
split) · atomic composite creates (sets via initial-state dataview) ·
**search is a read** — exempt from `Idempotency-Key` and `dry_run` ·
**`type` as a filter pseudo-key**; top-level `type` stays a single key ·
stored-view execution substitutes the §6.2 dynamic placeholders ·
`@me` identity served by `GET /members/me` server-side; sentinel +
relative-date math in the wrapper handler (§7.3) · **option creation is
opt-in on every channel** (`?create_missing_options=true`; without it an
unmatched name is refused — Phase 2), and the wrapper additionally
pre-validates names for the small tier · **key scope and space grants gate
`/v2`** (§1.5) · **api keys are minted, frozen and resolved by one identity
layer** (§1.6) · **object DELETE reaches own output only**, fail-closed
(Phase 2) · **the small-model contract is the task-tool wrapper (§7), not a
REST mode**.

**Benchmark-gated**: B1 — whether *large*-model docs/steering prefer
`replace_text`/`set_cell` over `update_block` (all are launch ops; nothing
ships or unships on B1) · B2 — which filter form the docs/steering recommend
per tier (both forms ship regardless; small-model primacy of the string is
settled — R8) · B3 tabular result format · B4 wrapper-tool prompt/skill
guidance.

**Deferred**: cross-object batch · block-scoped preconditions · conflict
rebase · events/subscriptions beyond the chat stream · core-profile strict
schema · `?permanent=true` hard delete · individually addressable blocks
inside table cells.

**Named build items** (open today; budget them):

- **`resultFormat=rows` encoder**. [B3-gated]
- **`GenerateSchema` + store-backed option join** — the `types/{type}/schema`
  route is a 501 stub today. The wrapper ships `describe` in its sanctioned
  degraded form, so this item no longer blocks the wrapper — landing it
  collapses the wrapper's composition to one GET.
- **md-export loss detector** (converter/md has no warning channel).
- **The output-only set is narrower than the unwritable set.**
  `set_properties` still accepts derived, bundle-readonly keys (`backlinks`,
  `links`, `lastModifiedBy`, `lastOpenedDate`) and answers 200 with
  `properties_changed: 0`: the service checks only the hand list
  (`core/api/v2/service/stateops.go:1093` via
  `v2model.IsOutputOnlyProperty`), while the wider predicate
  `v2model.IsUnwritableProperty` (`core/api/v2/model/model.go:699`) is
  consumed only by the wrapper's `describe`. The wrapper half is honest; the
  server half is not.

## 4. Benchmark program

`cmd/apiv2eval` uses task-specific final-state graders and records outcomes,
refusals, output tokens and turns per model tier. Its small-tier wrapper is
grammar-constrained.

- **B1 — edit-primitive steering** (tunes documentation; ships nothing —
  `replace_text`/`set_cell` are launch ops): arms = **update_block-only**
  (`replace_text`/`set_cell` withheld from the prompt) · **full launch op
  set**. Decision rule: whether the REST docs and B4 guidance point *large*
  models at `replace_text`/`set_cell` for text/cell edits or leave them on
  `update_block` (small-model steering is settled — the wrapper channels).
- **B2 — filter-form steering**: both forms ship regardless (the array is
  load-bearing for sets creation and round-trip; the string is the settled
  small-model form — R8), so B2 decides which form the docs/steering
  recommend for mid/frontier programmatic flows. Scored by execution
  semantics, never string equality.
- **B3 — result format** (gates `resultFormat=rows`): reading-accuracy +
  tokens, compact JSON vs rows over 10/100/1000-row results.
- **B4 — creation guidance** (tunes prompt/SKILL.md guidance — *not* the C12
  endpoint docs, which always ship schema + example): which in-context
  combination (example-only / schema+example / constrained core-profile)
  maximizes one-shot validity per tier.

## 5. Discovery

```
GET /v2/schemas                     # index: kinds, endpoints, examples, ops list
GET /v2/schemas/{kind}              # JSON Schema + worked example
                                    #   kinds: object · shortcut · type · template ·
                                    #   property · set · collection · file · filters ·
                                    #   search · space · chat · chatMessage ·
                                    #   chatMessageEdit · chatReaction · chatRead
GET /v2/schemas/ops/{op}            # per-op tiny strict schema + single-op minimal example
```

Per-op fetch keeps the smallest consumers at the smallest schema surface
(§3.5 capability cliff); the multi-op composite example (§2a) remains as a
secondary "multiple ops in one request" illustration. All generation-facing
schemas follow C13.

- **The compact filter-string grammar is served on the `filters` kind** (one
  concept, one slot, C2): that kind's response carries the structured-array
  schema AND the string grammar (EBNF + examples), the same artifact the §7
  GBNF conversion consumes.
- **The `search` and `query` kinds do NOT embed the recursive structured
  `filters` array** — an array without `items` breaks every constrained
  decoder — so they describe only the `filter` string and point at the
  `filters` kind for the structured escape hatch, which the endpoints still
  accept (`core/api/v2/service/schemas.go:190`).
- **Five chat kinds**, each strict and each with a worked example.
  `chatMessage` is the authoring surface (markup-source text, bare-id
  attachments) and its `text.maxLength` is 8000, the store's own cap in
  UTF-16 code units, pinned to the constant so the schema cannot out-promise
  the endpoint (`core/api/v2/service/schemas.go:215`).
- **A per-op schema describes ONE entry of the `ops` array, and its `example`
  is a bare op object** — an instance of the schema served beside it — while
  prose and a separate `example_body` show the wrapping request. A copyable
  example that carried the wrapper cost small models the inner `op` field, and
  a new op cannot land with an example its own schema rejects
  (`core/api/v2/service/schemas_ops.go:203`,
  `core/api/v2/service/schemas_ops.go:9`).
- **The payload block's `type` publishes the block-type vocabulary as an
  enum** on both payload-block shapes, derived from the format's own block
  schema minus the structural types the document does not carry (`title`,
  `description`, `featured_properties`) and minus the §7a transparent
  containers (`group`) — never a list hand-copied into the API. An enum must
  not offer a value no caller can succeed with, which is the "no field whose
  every value errors" rule applied one level down, to a value
  (`core/api/v2/service/schemas_ops.go:76`).
- **Neither payload-block shape publishes a `views` property**, so a payload
  block cannot name a dataview view through `insert_blocks` or
  `replace_subtree` at all; views are authored by the view-family ops and by
  `update_block`'s `set`. `update_block`'s `set` and `set_cell`'s `value`
  publish their block-shaped payloads deliberately **untyped** (a cell run's
  interior is recursive and a strict recursive def is a real cost to a
  constrained decoder), so on those ops the runtime guard is the instrument
  and the published block def is documentation rather than machinery
  (`core/api/v2/service/schemas_ops.go:185`).
- **Every operation whose path carries `{space_id}` can answer 404**: a
  well-shaped id for a space that does not exist is a 404 on all of them,
  object creation and file upload included. The rule is derived from the path
  rather than annotated per route, so a route added later is covered by
  construction (`scripts/fix_openapi_v2.py:250`).
- The per-type artifact's route (`types/{type}/schema`) shipped as a 501 stub;
  the `GenerateSchema` artifact is an open §3 build item.

## 6. Rollout

1. As built: `/v2` ships **ungated** alongside v1 on the same localhost
   server — no experimental flag exists (`V2Deps` is always fully populated;
   the only gating is nil-dependency skips for degraded test construction).
   Whether a flag is wanted before the first public release is an **open
   rollout task**, not a shipped fact. OpenAPI generated from day one.
2. Phase 2, then 3a/3b — shipped, with the Phase 1–2 R11 additions: files,
   spaces and members, plus object archive (own-output-only).
3. Benchmarks run once the harness + Phase 3a exist; gated items land in minor
   releases (additive to the closed op set).
4. Phase 4, Phase 5 — shipped (`cmd/anytype` is the CLI). The v1 deprecation
   clock starts when the CLI ships in a release build.

Each phase's exit criterion: its endpoints pass the harness's task set at
parity-or-better vs the v1 baseline flow for the same task (fewer calls,
fewer tokens, ≥ success rate), and every error message a failing run produced
has been reviewed as "actionable for a model" (C6).

## 7. Small-model surface: the task-tool wrapper

The full REST surface (§1–§6) is for large models and programmatic clients.
**Small models (3–4B on-device: Gemma 3n E4B and peers) never touch it
directly** — they drive a curated, task-shaped **tool-calling wrapper** over
the same primitives. A model-specific "mode" threaded through the REST
surface would preserve too many sharp edges; the wrapper instead reduces the
tool count and gives each tool the appropriate authoring, editing or
reference channel.

**One artifact, two deliveries.** The wrapper is a single tool manifest
mapping the task tools to `/v2` primitives; it is exposed as **CLI verbs**
(coding-agent harnesses) and as an **on-device function-calling / MCP
manifest**. Full REST + wrapper = two surfaces sharing one AnyBlock format,
one validation contract, one error contract. The two deliveries share the
manifest but NOT a state story: `find`'s enumerated handles outlive a tool
call, which a long-lived MCP process holds in memory while the
process-per-invocation CLI must persist — §7.4 states where handle state
lives.

### 7.1 The three channels that close the review's structural findings

- **Authoring channel = markdown** (not AnyBlock JSON). `add_blocks` takes a
  markdown string; the parser's home is an `insert_blocks` `markdown` payload
  alternative — mutually exclusive with `blocks`, same targeting incl.
  root-append — so markdown-in rides the whole op pipeline (validation,
  `created_blocks`, `diff_stats`, dry-run, idempotency) and the CLI vendors
  nothing. Removes inline-markup-in-JSON escaping (the top 3B failure) and the
  relative-indent arithmetic. [closes S2]
- **Editing channel = anchor + deterministic server edit.** `edit_text` takes
  `find`/`replace`; the server does the string replace in code and applies it
  via the `replace_text` primitive. The model supplies a short anchor, never
  reproduces the block. [closes S1's change-one-word collapse] `replace_text`
  matches the block's markup source, but replacement text is literal: the
  server parses a context-safe placeholder, then replaces it in the
  text-and-marks model, so a replacement cannot mint marks or break marks
  enclosing the match. A `find` confined to markup metadata (a tag attribute
  or link destination rather than rendered prose) is rejected path-addressed;
  use the relevant structured block operation instead. Dense `replace_all`
  output is size-checked before allocation against the block text's 1,048,576
  UTF-16-unit limit. The tool deliberately omits `replace_all` — single-match
  only for the small tier; the CLI may expose `--all` for larger consumers.
- **Reference channel = enumerated handles.** `find` returns `1,2,3`; both
  `read` modes expose the server's short block labels. Labels are unique
  suffixes, and every write path resolves suffixes via `matchBlockRef`, so the
  wrapper passes them through unchanged. It resolves object handles and
  retries one stale block reference after a re-read as described in §7.4. The
  model never sees or emits a 24-hex id. [closes S3]

### 7.2 Tool set (14; flat, grammar-constrainable args)

| Tool | Args (flat) | Backing primitive | Channel notes |
|---|---|---|---|
| `spaces` | `limit?` | `GET /v2/spaces` | the bootstrap tool: every trace needs a space id and nothing else in the set could produce one — `name — id` rows, no handles |
| `find` | `space, query?, type?, filter?, limit?` | Phase 4 search | filter = string form; results are enumerated handles + minimal fields. With none of `query`/`type`/`filter` the call would match nothing, so it LISTS the space instead — unnumbered, assigning no handles, because handle 1 of a listing is not the object anyone asked for |
| `read` | `object, mode=full\|outline` | Phase 1 read | both modes use server-issued short block labels; `full` returns complete block text, `outline` caps each block's text at 80 runes |
| `describe` | `space, type, options?, starting_with?` | `types/{type}/schema?flavor=table` **[build — 501 stub today]** | the accuracy lever, **called before create/set**; interim degraded form assembled wrapper-side; every backing GET is space-scoped, so the tool takes `space` too. It reports what is SETTABLE. Its `options` argument lists ONE select property's options in full — the only route to the options of a select the type does not name; `starting_with` narrows through the route's own prefix search |
| `create` | `space, type, name, properties?, markdown?` | Phase 2 create | type and property keys validated with did-you-mean; **an unmatched select option name is refused, not created** — the tool pre-validates names wrapper-side and sends the consent flag only where the caller asked for creation; markdown is parsed and folded into the single create snapshot |
| `set_properties` | `object, set?{key: value}, add?{key: […]}, remove?{key: […]}` | `set_properties` op incl. per-key `add`/`remove` | mirrors the op so a one-tag append never rewrites the whole array (reintroducing the read→rewrite→write trap at the wrapper layer would defeat it); `add` on a non-empty select errors, steering to `set`; scalar→array coercion is server-side |
| `check_item` | `object, block, checked` | `update_block` op | the one block-field tool: checkbox **blocks** are a common note shape; other block-field updates (color/align/language/retype) stay excluded — SKILL.md steers task completion to properties |
| `add_blocks` | `object, after?\|under?, markdown` | `insert_blocks` op, `markdown` payload | **markdown channel**; server parses → flat blocks (§7.1) |
| `edit_text` | `object, block?, find, replace` | `replace_text` op | **anchor channel**; deterministic server replace; `find` copies the served inline source and `replace` is literal prose; an EMPTY `replace` deletes the found text (Required means present, not non-empty). `block` is OPTIONAL — see below |
| `set_cell` | `object, table, row, col, value` | `set_cell` op | flat cell write (the tool takes `object` too — the REST op addresses a table within one object, and a table-only reference would need a hidden cross-object table registry); row/col take the labels a full read mints; an EMPTY `value` clears the cell (null on the wire) |
| `move_block` / `delete_block` | `object, block, after?\|under?` / `object, block(s), recursive?` | `move_block`/`delete_block` ops | handle-addressed; `delete_block`'s `block` takes ONE reference or a comma-separated list — the list rides one atomic PATCH (all deleted or none), added for the measured chain-length cliff |
| `update_view` | `object, space?, block?, view?, filter?, sort?, columns?` | `update_view` op | ONE tool for the three view channels rather than three tools — they share every line of targeting. At least one of `filter`/`sort`/`columns`. `filter` reuses the compact syntax `find` already publishes, and `none`/`all`/`clear`/`any` (any case) REMOVE the filter; `sort` is ORDER BY's grammar (`"Due date desc"`, comma-separated) and replaces the view's sorts; `columns` is the comma-separated list to show, and every other column is hidden. `block` and `view` are optional when the object has exactly one of each |
| `create_type` | `space, name, properties?` | `POST /types` (+ `POST /properties` for option-bearing selects) | the schema-authoring tool: `properties` is a flat paren-aware DDL, `Name: format(options)` — the same form `describe` prints per row. Refuses before writing anything: unknown formats, options on a non-select, a format that conflicts with an existing property, and (through a `dry_run=true` pre-flight) a taken or bundled-reserved type name |

Excluded from the wrapper: whole-document replace (excluded from the REST
surface too), multi-op batch (single tool call per intent — `delete_block`'s
comma-list is not the exception but the rule restated: N references, one
intent, one atomic PATCH), the structured `filters` array (recursive, not
constrained-decodable — string only), relative-indent authoring (the markdown
channel replaces it), block-field updates beyond `checked` (deliberate
curation), object archive (the REST route exists but is not a wrapper tool),
query building (`POST /queries` is the REST path).

**`edit_text`'s `block` is OPTIONAL.** With it omitted, the `find` snippet
locates the block and must identify exactly ONE block (and occur exactly once
within it). Zero matches refuse with a steer to `read mode=outline`; several
matching blocks refuse listing the candidate block labels with surrounding
context so the retry can pass `block` explicitly; several occurrences inside
the one matched block get the more-context refusal during the locate, before
any write is attempted. A required block id is unknowable on a model's first
turn, and a tool that requires a prior call is a tool a small model routes
around — into a silent wrong action (`core/api/wrapper/manifest.go:283`).

**Authoring a schema is a sequence whose ORDER is the safety contract**: read
the space's properties, pre-flight the type with `?dry_run=true`, create each
option-bearing property through `POST /properties`, and create the type LAST
with exactly one request. No failure can then leave a half-made type; the
residue of a mid-sequence failure is properties, which a re-run reuses rather
than duplicates. The pre-flight exists because only the server can answer
whether a type name is free (`core/api/wrapper/tools_type.go:34`). **A dozen
everyday type names** (Recipe, Book, Movie, Project, Contact and peers) are
reserved by built-in types that appear in no listing until something uses
them — creating an object of that type installs the built-in on first use, so
the name-taken refusal is the only place a caller can discover such a type
exists, and it says so (`core/api/v2/service/refs.go:104`).

**Whether an option exists is always established through the options route's
`prefix` search, never by reading a page of the listing.** The listing is
name-sorted and capped, so a property holding more options than one page
answers only for the alphabetically first of them, and a check built on a page
reports an option that exists as missing
(`core/api/wrapper/tools_type.go:372`).

**What a tool PRINTS must be accepted as what a tool TAKES.** `describe`
prints one property per row in the exact `Name: format(options)` form
`create_type` parses, so a describe output transcribes straight back. Nothing
that is not part of a property spec may appear in a row and nothing that is
not an option name may appear in the parentheses — truncation markers and
caveats go in `note:` lines after the rows
(`core/api/wrapper/describe.go:448`). A property whose option listing FAILED
is marked "could not be listed" rather than printed as an optionless select
that invites an invented name (`core/api/wrapper/describe.go:415`).

**`describe` reports what is SETTABLE, in one array whose rows carry their
section as flags**: the type's own properties first (marked as on-type — real
curation, but a subset of the settable set and not a bound on it), then the
rest of the space's property index, then the keys that can never be set,
marked read-only rather than dropped, so a caller who saw one in a read does
not conclude the tool is incomplete. `name` is always included, since neither
source produces it. The unwritable set is defined once and shared, and covers
both the output-only keys a write is refused for and the derived relations a
write would silently ignore (`core/api/wrapper/describe.go:52`).

### 7.3 What the wrapper does NOT let us skip

1. **The bounded server primitives must exist** — `edit_text`/`set_cell` are
   safe only because `replace_text`/`set_cell` are real server-side scoped
   ops. A wrapper that implemented them as GET+regenerate+write-the-whole-
   document would reintroduce corruption — the same argument that removed the
   REST PUT.
2. **Constrained decoding still required, now tractable** — on-device
   function-calling (Ollama/llama.cpp GBNF) needs the *tool* schemas
   grammar-emittable; C13 applies to the small flat tool args here instead of
   the recursive block tree. The wrapper serves a GBNF/CFG artifact per tool,
   including the filter-string grammar for `find`, and tests that its examples
   are accepted by their own grammars.
3. **Conveniences, placed** — scalar→array coercion is **already served**
   (`anyblockjson.UnmarshalPropertyValue` wraps scalars of list-shaped
   formats; every write path routes through it — not wrapper work).
   `GET /v2/spaces/{space_id}/members/me` supplies the server-known identity.
   **The wrapper resolves `@me` and relative dates itself while the REST value
   path stays literal**: `@me` substitutes textually inside a quoted filter
   value, and in property VALUES only on object-format keys, so a description
   containing the literal text is data; relative dates resolve only on
   date-format keys — `today`, `tomorrow`, `yesterday`, weekday names (the
   next occurrence, today included) and `±Nd`, all to RFC 3339 local midnight
   — and anything else passes through literally for the server to judge. The
   property formats are fetched once per tool call
   (`core/api/wrapper/values.go:739`). The wrapper also pre-validates option
   names for the small tier; on REST, creation is opt-in per request
   (Phase 2). If-Match/Idempotency-Key management stays wrapper-owned (the
   model authors none of these).

### 7.4 Delivery, transport and session state

- **The tools call `/v2` over localhost HTTP rather than in-process**, so
  every request passes the server's one enforcement point — auth, the write
  rate limit and the C8 idempotency store all live in server middleware that
  in-process calls would bypass. The tool table is Go data that the CLI
  imports, so the verb set equals the tool set by construction, and
  `anytype tools` prints the machine-readable manifest (name, description,
  strict parameter schema, example, GBNF per tool, plus the filter-string
  grammar artifact) (`core/api/wrapper/client.go:29`).
- **Two model tiers over one tool table**; a tier is a field on the existing
  definitions, never a second list, and every tool must declare one. The
  SMALL tier (~8B on-device models) serves exactly `spaces, find, read,
  describe, create, set_properties, add_blocks, edit_text`; every omission is
  deliberate, on the principle that for a small model a missing capability
  beats a misused surface. The LARGE tier serves the whole table. Arguments
  are not tiered, and the CLI verb set is not tiered — coding-agent harnesses
  drive large models (`core/api/wrapper/tier.go:45`).
- **`anytype mcp --tier small|large`** (default large) serves the same tool
  table over MCP stdio: newline-delimited JSON-RPC 2.0 with `initialize`
  (version negotiation, answering with ours for an unknown version, plus
  tier-aware `instructions`), `tools/list` (the strict tool schemas as
  `inputSchema`, with `readOnlyHint` on the non-mutating tools), `tools/call`
  and `ping`; notifications are acknowledged by silence and batching is
  refused with steering. The MCP delivery holds handle state in memory for the
  life of the process and never shares the CLI's session file
  (`core/api/wrapper/mcp.go:180`).
- **Tool failures are returned IN BAND** (`isError: true` plus text) so the
  model reads the repair tip; only malformed JSON-RPC and a tool name outside
  the served tier are protocol errors, and the unknown-tool message still
  lists the tier's tools. The two conditions whose fix is outside the model's
  reach — the API unreachable and the key rejected — end with a statement that
  no change to the call will help, so a small model stops retrying
  (`core/api/wrapper/mcp.go:16`).
- **Idempotency.** The wrapper mints a key per mutation whose reuse identity
  is the RESOLVED request — method, path, encoded query and marshalled body,
  the server's own C8 identity — not the tool name and raw arguments, so a dry
  run and its real twin never share a key. An identical resolved request
  repeated within 60 seconds reuses the previous key, including after a
  failure, so a harness re-run replays instead of re-applying; after the window
  an identical request is presumed intentional and applies fresh. Transport
  errors and 429/502/503/504 resend the same body under the same key, at most
  three attempts with 1s then 2s backoff (the server's sustained write budget
  is one request per second), and an exhausted retryable status surfaces the
  server's last error body rather than a bare status. The task tools never
  send `If-Match`; the CLI exposes it as an advanced flag for scripts
  (`core/api/wrapper/session.go:52`).
- **Handle state.** CLI handle state lives in a session file at
  `os.UserCacheDir()/anytype-cli/session.json`, overridden by
  `ANYTYPE_CLI_SESSION`; saves are atomic through a temp-file rename and a
  corrupt file starts fresh rather than bricking the CLI. A long-lived host
  keeps the same table in memory behind the same store interface, hands out
  deep copies and serializes tool calls, so concurrent calls cannot race the
  session maps (`core/api/wrapper/session.go:166`). Handles are written by
  `find`, read by the id-taking verbs, and invalidated/renumbered by each new
  `find`.
- **Block references pass through verbatim** — the server labels reads itself,
  so the wrapper does no relabeling; unique-suffix pass-through is the only
  write mechanism. **The ambiguity retry is scoped honestly**: a 400
  `ambiguous_input` means the ref did not resolve against the document the
  server saw, so the wrapper re-reads the object and retries once when the ref
  uniquely tails one of the re-read's own SERVED ids, which self-heals exactly
  the concurrent-modification race. A persistent ambiguity is unresolvable in
  principle — the wrapper cannot know which block the model meant — and
  surfaces the server's error. Because the rewrite lands after the
  Idempotency-Key is minted, `LastWrite` records it (`PriorHash`+`Rewrites`)
  so an identical re-run replays under the same key instead of re-applying.
- **Key folding lives in the WRAPPER, never in the REST surface**: a key is a
  key over REST, and two keys differing only by case must stay distinguishable
  for programmatic clients, while the wrapper is the layer built to be
  forgiving for small models. The hard rule either way is that if two entries
  answer to one fold class the call refuses naming both, and never picks one.
  Property keys fold before the format lookup, so a folded key still gets its
  date resolution and option guard; type keys fold on the ERROR path only,
  retrying once with the unique variant, so a correct key never pays for a
  type listing, and a folded retry re-derives its idempotency key because a
  different resolved request must not reuse the failed body's. Select option
  NAMES are never folded — they are user data, and two casings can
  legitimately coexist (`core/api/wrapper/values.go:247`).
- **Per-tool GBNF/CFG artifacts** are generated from the served tool schemas,
  with grammar/example agreement pinned by tests to keep C13 honest.

### 7.5 Refusals the caller can act on

- **A refusal that is correct and unactionable is a defect**: when a rejected
  value is wrong in a recognisable way, the refusal must name the repair, in
  the caller's own vocabulary. On the wrapper surface this is one table keyed
  by ARGUMENT NAME, evaluated on the runner's error path, so every tool taking
  that argument is covered and a new tool inherits it. The repair is APPENDED
  to the server's refusal (the server states which value failed; the hint
  states the fix), fires only after the server has answered not-found AND the
  server's message quotes the caller's own value, and a specific repair
  supersedes the generic one it competes with rather than being printed
  beside it. Recognised shapes include a block reference or a space id sent
  where an object handle belongs, and a space id truncated at its dot
  (`core/api/wrapper/steer.go:53`).
- **The wrapper never hands a tool caller a REST repair.** Every server
  message, issue and hint is re-spelled into the tool vocabulary before it
  reaches the caller: `inside` becomes `under`, `id` becomes `block`,
  `table_id` becomes `table`, the op-path prefix is stripped, a view op's
  filter and sort paths become the tool's own slots, and hints naming
  arguments the tool does not expose — such as `replace_all` — are removed
  (`core/api/wrapper/runner.go:628`). Anything still shaped like a route
  (`METHOD /vN/…`) degrades to "the HTTP API" and a bare `?param=` to "a
  parameter these tools do not take", so a hint added server-side later can
  lose specificity but can never name something the caller cannot do
  (`core/api/wrapper/steer.go:336`). The raw HTTP surface keeps its routes,
  where naming them is correct.

### 7.6 What the agent-facing guides may say

The SKILL files do **not** explain the id-compaction mechanism or the
short-versus-full space spelling. They state one instruction and one
recovery — use block ids exactly as a read served them, and if one is
rejected as unknown, re-read and use the fresh ids — and they frame
`?ids=full` only as the backup/export shape. The mechanisms belong in this
reference, not in a guide whose readers are language models: every extra
sentence is a concept the model must reason about in exchange for a case it
cannot act on differently (`core/api/v2/SKILL.md:96`).
