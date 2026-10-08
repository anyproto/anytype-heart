# A full wrapper inside heart — design

Status: design agreed 2026-09-18; amended 2026-10-08 after a four-lens
review (security, MCP protocol, derivation, evidence); implemented on this
branch and amended again after a second review round to match the code.
Supersedes the research brief `core/api/APIV2_WRAPPER_IN_HEART.md`.

Branch base: develop `4028f830c`. The mobile tool bridge branch
(`go-7383-apiv2-mobile-tool-bridge`, unmerged) carries an in-process HTTP
transport; this branch writes its own (§3.3) and the bridge branch adopts
it when it merges.

## 1. What is being built

An MCP server hosted by heart's local API server at `POST /mcp/{tier}`,
`tier ∈ small | large | full`, over the existing tool machinery:

- `small` and `large` serve the curated table in `core/api/wrapper`
  unchanged (14 tools today, flat arguments, handles, GBNF).
- `full` is a **second table**, derived at runtime from the embedded v2
  OpenAPI document, the served discovery schemas and the served op schemas,
  covering every operation not excluded by decision. It targets Sonnet-class
  models and replaces what the external `anytype-mcp` npm bridge serves today.

Decisions recorded from the brainstorm:

| question | decision |
|---|---|
| full tier shape | separate table, generated contract + hand-written overlay |
| generation time | runtime, lazily on first `/mcp/full` use, once per process (`full.Lazy`) |
| review artifact | golden snapshot test of the derived `tools/list` |
| tier relation | two tables; `full` is not a superset mark on curated tools |
| audience of full | Sonnet-class hosts with native HTTP MCP (Claude Code, Cursor, VS Code); Claude Desktop through a stdio shim |
| base branch | fresh develop; the in-process transport is written here |
| sessions | required for small/large once issued; `full` is sessionless |

Out of scope: growing the `large` tier for the 27B on-device target (stays
the curated table's concern); the gomobile binding (keeps calling the
curated `Host`, `ToolsManifest` keeps accepting `small|large` only); the
legacy 2024-11-05 HTTP+SSE MCP transport; browser-hosted MCP clients (no
CORS headers are served, as today); cloud-side connectors (claude.ai custom
connectors, ChatGPT connectors) — their MCP client runs on the vendor's
servers and cannot reach a loopback endpoint without a tunnel, which would
expose the local API to the internet and is a separate decision.

## 2. The full table: derivation and overlay

### 2.1 Package and inputs

New package `core/api/wrapper/full`. Entry point:

```go
func Derive(in Inputs) (*Table, error)

type Inputs struct {
    OpenAPI []byte                     // the embedded v2 document
    Ops     map[string]OpSchema        // served op schemas, by op name (GET /v2/schemas/ops/{op})
    Kinds   map[string]json.RawMessage // served discovery schemas, for the bodies still open (GET /v2/schemas/{kind})
}
type OpSchema struct { Schema json.RawMessage; Example json.RawMessage; Channels []string } // channels: object, type
```

**The document is the body contract for most operations.** swag emits a
bare `object` for every body whose shape lives in a served discovery schema
rather than a Go struct, and `core/api/v2/service/openapibodies.go`
composes those bodies from the served schemas at `make openapi` time;
`scripts/fix_openapi_v2.py` splices them into the document and
`core/api/openapibodies_test.go` pins the splice. Small kinds are spliced
whole (property, collection, query with its structured filters, widget,
the flat type body, update_property's `{name}`). Two shapes are not:

- the **ops envelopes** on `patch_object` and `update_type` publish the op
  vocabulary as an enum and leave each op's members to its own schema —
  `Ops` fills them (§2.2);
- the **AnyBlock document forms** — `create_object`'s and `create_type`'s
  document branches, `create_template`, `validate` — are open objects whose
  description names the kind, because the document schema is 19 to 50 KB
  and the OpenAPI document would carry four copies. `Kinds` fills exactly
  these, chosen by the overlay's `BodyKind` (`object`, `type_document`,
  `template`, `document`); `full.BodyKinds()` tells the caller which kinds
  to supply.

Package `api` (the composition root) supplies all three; `full` imports
only `v2model` and `wrapper`. There is no degraded mode: anything named
`full` is built from complete inputs, and a CLI `tools --tier full` is out
of scope. A test pins that no tool in the table has an opaque body.

### 2.2 One tool per operation

- **Name** = `operationId` (`patch_object`, `list_chats`). Identity with
  `v2model.Ref.Op`, so ref rendering on this tier is a lookup that returns
  its input, and a third party moving off the npm bridge recognises the
  surface.
- **Arguments**, all top-level, flat:
  - path parameters (`space_id`, `object_id`, …), required;
  - query parameters (`dry_run`, `ids`, `create_missing_options`, `limit`, …);
  - header parameters in snake_case (`if_match`); `Anytype-Version` occurs
    only on excluded operations;
  - **`idempotency_key` is reserved on every non-GET tool**, documented on
    the route or not: a header argument bounded like the server bounds the
    header (1–255 visible ASCII), never a body member;
  - the request body's top-level properties, required per the source schema.
    A member whose name is not a legal tool-argument key
    (`^[a-zA-Z0-9_.-]{1,64}$`, the Claude API's rule) is not offered — the
    documents' `$schema`.
  - Verified against the current document: no body property collides with
    a path/query/header name on any operation. A collision fails `Derive`
    with a message naming the overlay rename that fixes it
    (`RenameBodyMember`; the executor sends a renamed member under its wire
    name).
  - A body's `minProperties: 1` becomes "one of the body members is
    present": flattened, the path arguments would satisfy it alone.
- **Path arguments are one segment**: gin routes on the decoded path, so a
  path argument that is empty, `.`, `..` or contains `/` is refused before
  the call — an escaped slash would otherwise reach another route,
  excluded ones included.
- **Body content type**: when an operation offers both `application/json`
  and `multipart/form-data`, derive from the JSON body and drop multipart.
  `upload_file` therefore takes `url` (required) and `name`.
- **Schemas**: component schemas a tool references are copied into the tool
  schema's `$defs` with `$ref` rewritten to `#/$defs/<name>`. Served op and
  kind schemas carry their own `$defs` (`block`, `blockRef`, `anyValue`…)
  and **the same name means different shapes on different ops**
  (`insert_blocks` vs `replace_subtree` `block`). Each embedded schema's
  defs are therefore namespaced (`<op>__block`) and every local ref
  rewritten, then defs that are structurally identical are merged back under
  one name, to a fixpoint (a definition merges once the ones it references
  have). The same holds for embedded kinds. Within one tool only: each tool
  carries its own `$defs`.
- **Typed ops envelope**: `ops` is an array (1–512 items, the server's
  bound) whose items are a `oneOf` over the channel's op schemas; `op` is a
  required `const` per branch, which is a sufficient discriminator in JSON
  Schema. `additionalProperties:false` and `required` are preserved per
  branch. The object channel has 15 ops (including `set_type`), the type
  channel 7, four view ops on both — 18 distinct.
- **Alternative bodies** (`create_object`, `create_type`, `update_type`)
  stay root-level `anyOf` constraints over the same flat arguments. A
  member every branch spells alike lives once on the root; one two branches
  spell differently (the shortcut's `properties` against the document's)
  gets an unconstrained root entry and its real schema per branch. Each
  branch keeps its `required` and `additionalProperties:false`, widened
  with the parameter names, so `update_type`'s flat patch and ops envelope
  stay mutually exclusive.
- **Description**: the operation's `summary`, then `description` if present.
  The document's prose is already under `core/api/openapiprose_test.go`.
- **Annotations by semantics, not method**: `readOnlyHint` on GETs and on
  `search_space`, `search_global`, `validate`; `destructiveHint` on DELETEs
  and on `patch_object`/`update_type` (they can delete blocks and
  properties). Hints are advisory; nothing enforces on them.
- **Stateless**: no handles, no session store, no GBNF, no examples. The
  curated machinery is untouched.
- **Operation inventory**: the conformance test diffs the derived names
  against `v2model.OperationIds()`, the operation table `ref.go` pins to
  the document, so a route added to the document without a `Ref` constant
  fails there first.

### 2.3 The overlay

A hand-written Go table keyed by operationId. Per entry it may:

- **exclude** with a reason (the entry is the only way an operation can be
  absent);
- **rewrite** the description;
- **rename**, **drop** or **add** an argument;
- **replace** an argument's schema;
- **set** annotations.

Launch contents:

| operationId | action | reason |
|---|---|---|
| `auth_whoami`, `create_auth_challenge`, `create_api_key` | exclude | pairing/identity flows are the host's, not the model's |
| `stream_chat_messages`, `stream_space_chats`, `stream_space_search` | exclude | SSE; the in-process transport buffers whole responses |
| `head_file`, `download_file` | exclude | binary/range responses have no text rendering; revisit with a use case |
| `search_space`, `search_global`, `validate` | annotate read-only | POST reads |
| `patch_object`, `update_type` | annotate destructive; type the ops envelope | ops can delete |
| `create_object`, `create_type`, `create_template`, `validate` | `BodyKind` | embed the document form (§2.1) |

58 operations in the document, 8 excluded, 50 tools.

Descriptions inside the `oneOf` branches come from the served op schemas
verbatim at first; the size ceiling (§5) decides whether they are trimmed.

**Document bugs are fixed in the document, not the overlay.** The review
found places where the OpenAPI document disagrees with the server or the
discovery schema; those are swagger fixes that land before or with this
work, so the derived table inherits them: `name` required on
`create_space`/`create_chat`, `emoji` on `toggle_chat_reaction`, `text` on
`edit_chat_message` (and the handler refuses an absent text with a C6
issue on `/text`, so the document's word is true), `read_chat`'s scope
enum published. The overlay is for MCP-specific shape only.

### 2.4 Build and lifetime

Built once per process on first `/mcp/full` use (`full.Lazy`); the error,
if any, is retained and returned as a 500 naming it — a server built
without the table answers 503. Only the
immutable inputs, the derived table and that error live in the `Once`;
clients, credentials and engine lookup stay per request. The inputs are
constant, so a failure is a build defect the tests in §5 already catch, not
a runtime condition to retry.

## 3. The MCP handler

### 3.1 Route and middleware

`POST /mcp/:tier` registered in `core/api/server/router.go` beside `/v1`
and `/v2`, under the same trusted-origin policy and `ensureAuthenticated`.
Outer authentication failures are HTTP 401/403 to the host, never an
in-band tool error — the model cannot repair a key. `GET` → 405 (no server
push). `DELETE` with a session header ends that session. Unknown tier → 404
naming the three.

**Admission before work.** In this order: the protocol header is checked
(on POST, GET and DELETE alike); one of the key's call slots is taken
(at most 8 in flight per key, across tiers — the full tier included);
the body is read under an 8 MiB cap and a 10 s deadline; the message is
classified; only then is anything built. A call deadline of 60 s bounds
the rest. The in-process response buffer is capped at 32 MiB. The slot is
released through a call lease that the in-process transport also holds for
every inner handler it starts, so a caller that gives up on its deadline
does not free its slot while its handler still runs. These are resource
limits, distinct from model-facing result budgets.

### 3.2 Transport: Streamable HTTP, JSON only

- Protocol revisions on HTTP: `2025-06-18` and `2025-11-25`. Chosen
  explicitly for this delivery, not reused from the stdio map, because the
  HTTP profile below refuses batches, which `2025-03-26` allowed. An
  unsupported `MCP-Protocol-Version` header → 400. Newer revisions are
  added only once their transport changes are reviewed.
- One JSON-RPC message per POST, answered as `application/json`.
- **Classify before execute.** An exported entry point validates the
  envelope (`jsonrpc`, id shape, method, params an object or array, a
  response carrying result or error but not both) and classifies request /
  notification / response before anything runs; valid JSON of the wrong
  shape is an invalid request answered under its own id. Notifications and
  responses → 202, empty body; an id-less `tools/call` is a notification
  and is **not** executed. Malformed envelopes → JSON-RPC error. The stdio
  server moves onto the same entry point.
- JSON arrays (batches) → `-32600`.
- `MCPServer` holds an executor interface — `Tools() []ToolDef`,
  `Instructions() string`, `Run(ctx, name, args) (*Result, error)` —
  satisfied by the curated Runner (per tier) and by the full table's
  generic executor. One protocol loop, two tables. `tools/call` results
  carry `structuredContent` beside the text on every tier, always an
  object (a non-object shape is wrapped as `{"result": …}`).
- Bad `initialize` params and any `tools/list` cursor (the list is never
  paginated) are `-32602`. A request whose context has ended is answered
  and not run.

### 3.3 Caller's key, never the internal key

Each session owns a `wrapper.Client` whose transport is **in-process**: an
`http.RoundTripper` (`core/api/server/inproc.go`) that clones the request,
stamps `RemoteAddr` as loopback, and calls the gin engine's `ServeHTTP`
into a buffered response — no listener, no socket, every middleware runs
unchanged. Its resolver is **engine-only** and returns the current engine
per request (a rebuilt engine after `ReassignAddress` is picked up without
rebuilding the transport); it never supplies a bearer. The client sends the
bearer taken from the `/mcp` request, so every inner `/v2` call is
authenticated, scoped and grant-gated as the caller. The write rate limiter
keys on the stamped loopback address, shared with real loopback callers by
design. The engine runs on its own goroutine so the request context's end
ends the wait; the runner's turn is a channel a waiter can abandon. The
mobile bridge branch's variant of this transport forces a
per-process internal key; when it merges it builds on this file and keeps
that forcing as a second constructor, never wired into `/mcp`.

### 3.4 Sessions

- `initialize` on `small` or `large` mints an `Mcp-Session-Id`: 128 bits
  from `crypto/rand`, bound to the API key that created it and to the tier.
  Conforming clients echo it on every later request (the transport spec
  requires it, and every SDK-based host does).
- A session is reserved against the caps before its runner is built, and
  only for an `initialize` **request**: an initialize notification or a
  malformed one allocates nothing.
- After `initialize`, a curated-tier request without the header → 400; an
  unknown, expired, foreign-key or wrong-tier id → 404 on every method
  (GET answers it before its 405), and the client re-initialises.
  Ownership mismatches are not distinguished from expiry.
- `full` issues no session id and keeps no state; every call is
  independent.
- One session = one `MCPServer` (its Runner + `MemoryStore`). Handles per
  session are capped at 4,096 and keep at most 256 runes of name. A call
  that needs room evicts the oldest handles **before** it registers any,
  so nothing it advertises — its own list handle included — is dropped by
  its own registrations, and its result names the dropped range. Idle expiry, a global session cap and a
  per-key cap are constants with tests; admission is counted before
  allocation; overflow refuses the new session with 429.
- `DELETE` invalidates exactly that session; a concurrent call on it
  finishes or fails, it never resurrects the session.

### 3.5 Instructions and budget

`small`/`large` serve `tierInstructions(tier)` as today. `full` serves its
own short text: edits are op envelopes on `patch_object`/`update_type`,
`get_op_schema` and `get_schema` exist for lookups, `dry_run` previews. No
result budget on this delivery (same as stdio; `DefaultResultChars` = 0).

### 3.6 Host attachment

- **Claude Code, Cursor, VS Code**: native HTTP transport to
  `http://127.0.0.1:<port>/mcp/full` with an `Authorization: Bearer` header.
  A recipe per host is documented and tested against pinned host versions.
- **Claude Desktop**: its local config is stdio-only and its connectors run
  in Anthropic's cloud. Desktop attaches through a stdio shim that forwards
  JSON-RPC to `/mcp/full`: `mcp-remote` with no code of ours, or
  `anytype-mcp` reduced to that shim (it then stops generating tools from
  the OpenAPI document). Which one ships is a packaging decision outside
  this spec.
- **ChatGPT and claude.ai connectors**: out of scope (see §1).

## 4. Results and errors on the full tier

- **Results pass through**: response body as text content and as
  `structuredContent`, byte for byte except the top-level `warnings`: a
  warning's message and hint are re-spelled through its `see_also` like an
  error's (below), so a success-path repair names a tool, not a route.
- **Idempotency**: the executor mints an `Idempotency-Key` for every
  mutating call that did not supply `idempotency_key`, and the same key is
  reused across the client's transport retries, as the curated Runner does
  today. An explicit key is sent as given, after the same 1–255 visible
  ASCII bound the v2 middleware now enforces for every REST caller.
- **Errors are in-band** (`isError: true`) carrying the C6 envelope's
  message, issues and hints. Transport failure of the in-process call (no
  account running) renders the "ask the user" tip `mcp.go` already has.
- **Hint rendering by lookup, no regex table**: each issue's `SeeAlso` ref
  renders as the tool name plus bound arguments; the ref's REST spelling
  (`Ref.String()`, which the prose contains verbatim by contract) is
  replaced in the prose by that rendering. A resend ref (no `op`) renders as
  "call this tool again with `<param>: <value>`".
- **One lookup, two vocabularies**: `steer.go`'s `deRestText` respells refs
  through the package-level `toolVocab`. It is extracted into a function
  that takes the vocabulary as an argument; the curated tiers pass
  `toolVocab`, the full tier passes the identity spelling (operationId is
  the tool name, so every ref has a row and the "not on this surface"
  fallback never fires). The `restRoute` catch-all stays as the last pass
  on both, and its "nothing route-shaped survives" test runs over the full
  tier's output too.

## 5. Tests and measurement

**Conformance** (`core/api/wrapper/full`):
- every operationId in the document is derived or excluded with a reason;
- no tool has an opaque body; each embedded kind's served example validates
  in the assembled tool schema and an unknown member is refused;
- every reference in every tool resolves in its own `$defs`; a dangling
  transitive component reference fails `Derive`;
- every argument name is a legal tool-argument key; every write takes the
  reserved retry key;
- golden snapshot of the full `tools/list` (compact JSON of the result
  object, no JSON-RPC framing) checked in, `-update` refreshes; the golden
  is built from the **served** kind and op schemas, so a service-side schema
  change without a document change still shows;
- the ops `oneOf` has exactly the channel's op set (object 15, type 7) and
  every branch validates the op's own served example **after embedding**,
  i.e. against the assembled tool schema with rewritten refs; a negative
  fixture per channel (unknown op, extra field, missing `op`) is rejected;
- a body/path/query name collision fails `Derive` with the fixing rename
  named;
- every overlay target exists (an overlay row for a vanished argument or
  operation fails).

**Handler** (`core/api/server`), table tests over the gin engine: auth
required and 401 not in-band; tier routing; notification and id-less call
→ 202 with nothing executed; batch refused; unsupported protocol version
→ 400; session required after initialize on curated tiers, foreign-key and
wrong-tier ids → 404, `full` sessionless; expiry, per-key and global caps,
DELETE semantics; body limit; inner calls carry the caller's bearer and not
the internal key — mutation-verified by forcing the internal key in the
variant constructor and watching the named test fail.

**Rendering**: served error envelopes with `see_also` (the schema-lookup,
list and resend forms), one legacy envelope without it, and a success body
with warnings; each asserts the model-visible text, and the route-shape
guard runs over every rendered string. Request-assembly tests: path
escaping, query vs body separation, `if_match` as a header, minted
idempotency key stable across a retried call.

**Size**: the artifact is the compact `tools/list` result object. Measured
on this branch: small 8,471 B, large 15,470 B. The npm bridge's v2 surface
measured 45 tools / 59,269 B with an **untyped** `patch-object` body. The
full tier measures **50 tools, 257,156 B**: 110,938 B before the four
document bodies were embedded, so those four account for well over half.
Each tool carries its own `$defs`, so the document schema is paid per
tool; a host that loads tools on demand pays it only for the tool it uses.
The ceiling (280 KiB) is a regression guard, not evidence of
acceptability; acceptability is the benchmark's job, and the four document
bodies are the first place to look if the measured cost is too high.

**Benchmark** (acceptance, after implementation): the plant benchmark
(`docs/evals/anytype-mcp-v2`) with a task list moved off the served example
values, sonnet and haiku, paired runs on the same build and fresh fixtures,
isolated rate-limit state per actor. Arms:

1. npm bridge (baseline, untyped `patch-object`);
2. `/mcp/full` as specified;
3. `/mcp/full` with the ops body made opaque — the ablation that isolates
   the typed envelope, which is the central design claim;
4. optionally per-op tools, which `cmd/apiv2eval`'s ops arm already builds.

Pre-registered before the runs: the primary score, the per-category error
counts (envelope/discriminator, locator, payload, semantic), first-attempt
success per op, calls and tokens per completed task, time to first
successful mutation. The typed body is expected to remove envelope and
discriminator errors; it is not expected to remove locator or semantic
errors, since the op schemas admit calls the server rejects (e.g.
`update_block` without a locator). Three actors per arm is a pilot; the
result stands only if the difference exceeds the run-to-run spread.

## 6. Evidence corrections to the research brief

- The brief's "generated surface is what haiku met" was measured against a
  bridge whose `patch-object` input schema is
  `{"type":"object","additionalProperties":true}` — the tool schema carried
  no op shape. That is the opaque-body case, not the generated-with-typed-
  body case this design builds. What it does not prove: that an inline
  typed body beats per-op tools or a lookup; arm 3 and 4 above test that.
- The mobile "C bridge" (unmerged branch) is a gomobile binding over an
  in-process HTTP transport; it bypasses the socket and the user-managed
  key, not the API middleware. Its internal key is full-scope and must not
  back `/mcp`.
- The OpenAPI document is not the body contract for ten operations; the
  discovery catalog is. Any derivation that reads only the document
  produces tools without bodies for exactly the operations that matter.

## 7. Constraints carried over

- Served prose rules (`core/api/prose`), `make openapi` after swagger
  changes, every new guard mutation-verified, error wrapping per CLAUDE.md.
- The curated wrapper's `deRest` invariant (nothing route-shaped survives)
  is unchanged and now also asserted over the full tier's renderer.
- `Manifest` and `ToolsManifest(tier)` are a shipped mobile ABI: fields may
  be added, not renamed; `small`/`large` strings are fixed.
