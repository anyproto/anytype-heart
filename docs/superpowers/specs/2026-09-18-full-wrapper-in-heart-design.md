# A full wrapper inside heart — design

Status: design agreed in conversation on 2026-09-18, not yet implemented.
Supersedes the research brief `core/api/APIV2_WRAPPER_IN_HEART.md`.

Branch base: `go-7383-apiv2-mobile-tool-bridge` (on develop `feb1ee1ad`).

## 1. What is being built

An MCP server hosted by heart's local API server at `POST /mcp/{tier}`,
`tier ∈ small | large | full`, over the existing tool machinery:

- `small` and `large` serve the curated table in `core/api/wrapper`
  unchanged (14 tools today, flat arguments, handles, GBNF).
- `full` is a **second table**, derived at runtime from the embedded v2
  OpenAPI document plus the 17 served op schemas, covering every operation
  not excluded by decision. It targets Sonnet-class models and replaces
  what the external `anytype-mcp` npm bridge serves today.

Decisions recorded from the brainstorm:

| question | decision |
|---|---|
| full tier shape | separate table, generated contract + hand-written overlay |
| generation time | runtime, lazily on first `/mcp/full` use, `sync.Once` |
| review artifact | golden snapshot test of the derived `tools/list` |
| tier relation | two tables; `full` is not a superset mark on curated tools |
| audience of full | Sonnet-class hosts (Claude Desktop, Claude Code, Cursor, VS Code) |
| base branch | mobile bridge branch, for `inproc.go` and `Host` |

Out of scope: growing the `large` tier for the 27B on-device target (stays
the curated table's concern); the gomobile binding (keeps calling the
curated `Host`, `ToolsManifest` keeps accepting `small|large` only); the
legacy 2024-11-05 HTTP+SSE MCP transport.

## 2. The full table: derivation and overlay

### 2.1 Package and inputs

New package `core/api/wrapper/full`. Entry point:

```go
func Derive(doc []byte, opSchemas map[string]json.RawMessage) (*Table, error)
```

`doc` is the embedded v2 OpenAPI JSON (`openapiV2JSON` in package `api`),
`opSchemas` the 17 served op schemas keyed by op name, obtained from
`v2service.Service.SchemaOp`. Package `api` (the composition root) supplies
both; `full` imports nothing heavier than `v2model`. Printing the full
manifest from the CLI (`anytype tools --tier full`) is not required at
launch; if added, it reads the document from disk and passes an empty op
map, and the typed body then degrades to the document's own shape.

### 2.2 One tool per operation

- **Name** = `operationId` (`patch_object`, `list_chats`). Identity with the
  `v2model.Ref.Op`, so ref rendering on this tier is a lookup that
  returns its input, and a third party moving off the npm bridge recognises
  the surface.
- **Arguments**, all top-level, flat:
  - path parameters (`space_id`, `object_id`, …), required;
  - query parameters (`dry_run`, `ids`, `create_missing_options`, `limit`, …);
  - header parameters in snake_case (`idempotency_key`, `if_match`);
    `Anytype-Version` is dropped by the overlay (server-filled);
  - the request body's top-level properties, required per the document.
  - Verified against the current document: no body property collides with
    a path/query name on any operation. A collision fails `Derive` with a
    message naming the overlay rename that fixes it.
- **Body content type**: when an operation offers both `application/json`
  and `multipart/form-data`, derive from the JSON body and drop multipart.
  `upload_file` therefore takes `url` (required) and `name`.
- **Schemas**: component schemas a tool references are copied into the tool
  schema's `$defs` with `$ref` rewritten to `#/$defs/<name>`. No inlining,
  no cycle handling needed (block schemas are recursive).
- **Description**: the operation's `summary`, then `description` if present.
  The document's prose is already under `core/api/openapiprose_test.go`.
- **Annotations**: `readOnlyHint` for GET operations; `destructiveHint` for
  DELETE.
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
- **rename** or **drop** an argument;
- **replace** an argument's schema.

Launch contents:

| operationId | action | reason |
|---|---|---|
| `auth_whoami`, `create_auth_challenge`, `create_api_key` | exclude | pairing/identity flows are the host's, not the model's |
| `stream_chat_messages` | exclude | SSE; the in-process transport buffers whole responses |
| `head_file`, `download_file` | exclude | binary/range responses have no text rendering; revisit with a use case |
| `patch_object` | replace `ops.items` | typed `oneOf` over the 14 object op schemas, `op` as discriminator |
| `update_type` | replace `ops.items` | typed `oneOf` over the type op schemas (add/remove/move_property + the view family) |
| every op with `Anytype-Version` header | drop argument | server-filled |

Descriptions inside the `oneOf` branches come from the served op schemas
verbatim at first; the size ceiling (§5) decides whether they are trimmed.

### 2.4 Build and lifetime

Built once per process on first `/mcp/full` use behind `sync.Once`; the
error, if any, is retained and returned as a 500 naming it. The input is
constant, so a failure is a build defect the tests in §5 already catch, not
a runtime condition to retry.

## 3. The MCP handler

### 3.1 Route and middleware

`POST /mcp/:tier` registered in `core/api/server/router.go` beside `/v1`
and `/v2`, under the same trusted-origin policy, `ensureAuthenticated`, and
the v2 JSON-API scope gate. `GET` → 405 (no server push). `DELETE` with a
session header ends that session. Unknown tier → 404 naming the three.

### 3.2 Transport: Streamable HTTP, JSON only

- One JSON-RPC message per POST, answered as `application/json`.
- Notifications (`notifications/initialized`, …) → 202, empty body.
- JSON arrays (batches) → `-32600`; the current protocol revision removed
  batching and the stdio server never accepted it.
- `MCP-Protocol-Version` request header accepted; version negotiation as in
  `mcp.go` (`mcpSupportedVersions`).
- The existing `MCPServer.dispatch` is transport-agnostic; the handler calls
  it. No second protocol implementation.
- `MCPServer` today holds a `*Runner`. It changes to hold an executor
  interface — `Tools() []ToolDef` and `Run(ctx, name, args) (*Result, error)`
  — satisfied by the curated Runner (per tier) and by the full table's
  generic executor. One protocol loop, two tables.

### 3.3 Caller's key, never the internal key

Each session owns a `wrapper.Client` whose transport is the in-process one
with the bearer taken from the `/mcp` request, so every inner `/v2` call is
authenticated, scoped and grant-gated as the caller. `inproc.go` gains a
constructor variant that leaves `Authorization` to the client; the mobile
bridge keeps the forcing variant. The write rate limiter still keys on the
stamped loopback address, shared with real loopback callers by design.

### 3.4 Sessions

- `initialize` mints an `Mcp-Session-Id` and returns it in the header.
- The header is **never required**. Resolution order: header if present,
  else the caller's API key. A host that does not echo the header still
  gets stable handles (one key belongs to one app); conforming hosts get
  isolation between two hosts on one key.
- One session = one `MCPServer` (its Runner + `MemoryStore`, tier fixed at
  creation). Curated-tier handles behave exactly as over stdio. Full-tier
  sessions hold no state but follow the same protocol.
- Idle expiry and a session cap are constants with tests; an expired id
  → 404 per spec, and the client re-initialises.

### 3.5 Instructions and budget

`small`/`large` serve `tierInstructions(tier)` as today. `full` serves its
own short text: edits are op envelopes on `patch_object`/`update_type`,
`get_op_schema` and `get_schema` exist for lookups, `dry_run` previews. No
result budget on this delivery (same as stdio; `DefaultResultChars` = 0).

## 4. Results and errors on the full tier

- **Results pass through**: response body verbatim as text content and as
  `structuredContent`. Success-path `warnings` and their hints reach the
  model unchanged.
- **Errors are in-band** (`isError: true`) carrying the C6 envelope's
  message, issues and hints.
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
- Transport failure of the in-process call (no account running) and a
  rejected key render as the two "ask the user" tips `mcp.go` already has.

## 5. Tests and measurement

**Conformance** (`core/api/wrapper/full`):
- every operationId in the document is derived or excluded with a reason;
- golden snapshot of the full `tools/list` checked in, `-update` refreshes;
- every `oneOf` branch of the typed ops body validates the op's own served
  example;
- a body/path/query name collision fails `Derive` with the fixing rename
  named.

**Handler** (`core/api/server`), table tests over the gin engine: auth
required; scope gate applied; tier routing; notification → 202; batch
refused; session by header and by key fallback; expiry and cap; inner calls
carry the caller's bearer and not the internal key — mutation-verified by
forcing the internal key in the variant constructor and watching the named
test fail.

**Rendering**: served error envelopes with `see_also` (the schema-lookup,
list and resend forms), one legacy envelope without it, and a success body
with warnings; each asserts the model-visible text, and the route-shape
guard runs over every rendered string.

**Size**: pinned ceiling on `tools/list` bytes per tier, set from the first
measurement. Baselines today: small 8,268 B, large 15,300 B; the npm
bridge's v2 surface is 45 tools / 59,269 B with an **untyped** `patch-object`
body. Expect full near 70 KB with the typed body.

**Benchmark** (acceptance, after implementation): the plant benchmark
(`docs/evals/anytype-mcp-v2`) on `/mcp/full` and on the npm bridge against
the same build, sonnet and haiku, three actors each, task list moved off the
served example values. Prediction: full ≥ bridge on score; haiku's op-shape
errors collapse once the op shape is in the tool schema rather than behind a
lookup.

## 6. Evidence corrections to the research brief

- The brief's "generated surface is what haiku met" was measured against a
  bridge whose `patch-object` body is `{"type":"object","additionalProperties":true}`
  — the model saw **no op shape at all**. That is the opaque-body case, not
  the generated-with-typed-body case this design builds.
- The mobile "C bridge" is a gomobile binding over an in-process HTTP
  transport; it bypasses the socket and the user-managed key, not the API
  middleware. Its internal key is full-scope and must not back `/mcp`.

## 7. Constraints carried over

- Served prose rules (`core/api/prose`), `make openapi` after swagger
  changes, every new guard mutation-verified, error wrapping per CLAUDE.md.
- The curated wrapper's `deRest` invariant (nothing route-shaped survives)
  is unchanged and now also asserted over the full tier's renderer.
- `Manifest` and `ToolsManifest(tier)` are a shipped mobile ABI: fields may
  be added, not renamed; `small`/`large` strings are fixed.
