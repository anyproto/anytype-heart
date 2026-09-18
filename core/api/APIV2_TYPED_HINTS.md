# Typed hints

Status: **implemented** on branch `typed-hints` (heart) and `typed-hints`
(anytype-mcp), 2026-09-18, then reviewed through three lenses (correctness,
served-prose truth on each surface, contract and guards) and amended — see
"Review findings" at the end. The open questions of the original brief are
answered below with the decision taken; the evidence section is kept as the
rationale.

## The problem

Every repair hint this API emitted was written in REST. `GET /v2/schemas/ops/set_properties
for the op's schema and example` is actionable if you speak HTTP. Most of our
agent callers do not: they reach the API through an MCP tool wrapper and have
no way to issue a GET at all. To act on that hint a model must first infer that
the string means `API-get-schema {"kind":"ops/set_properties"}`.

That inference correlates with model strength, so it manufactures a capability
gap that is really ours. From the six-actor benchmark in
[`docs/evals/anytype-mcp-v2/result.md`](../../docs/evals/anytype-mcp-v2/result.md):

- haiku Delta was handed that exact hint **15 times** and never followed it.
  Zeta, 10 times. Both concluded the schema was unreachable — Delta wrote, in
  its closing report, *"without direct schema access"*. It had no direct schema
  access. It had a URL.
- haiku Epsilon made the inference once. Its error rate went 70% → 18%, and it
  scored 7/10 where the other two scored 3 and 2. **Same build, same tools,
  same errors.**
- The three sonnet actors made the inference silently and the issue never
  surfaced — which is why this was originally mis-filed as a model-capability
  failure. It is not. It is an API that names an affordance the caller does not
  have.

## The inventory (what was actually converted)

Counted against `origin/develop` at `bd0f1ec86` by the walk the guard test
now performs (every string literal in `core/api/v2` outside the schema
documents that matches a method + route, an abbreviated `METHOD …/` route,
an ASCII-ellipsis route, a `/v2/` path, or a bare `?name=` parameter):
**75 literals**, several
shared through helpers and several naming two operations, so the count of
*references* is higher and the count of *call sites* lower. By kind:

| kind | example |
|---|---|
| "go read this schema" | `GET /v2/schemas/ops/{op}`, `GET /v2/schemas/filters` |
| "go list / read these" | `GET …/properties`, `…/properties/{key}/options`, `GET /v2/spaces` |
| "go write there" | `POST …/properties`, `PATCH …/types/{type}`, `DELETE …/properties/{key}` |
| "resend with this parameter" | `?create_missing_options=true`, `?dry_run=true`, `?limit=25`, `?after=` |
| route in a **message**, not a hint | `space %q not found — list spaces with GET /v2/spaces` |
| a bare read parameter | `GET the object with ?outline=true to list them`, `GET it with ?ids=full` |

Every one is now a `v2model.Ref`. The schema documents (`schemas.go`,
`schemas_ops.go`, `apiv2schema.go`) keep their `endpoint` lines in REST: they
document the HTTP surface for a reader who asked for it. Their field
descriptions do NOT spell routes (round-two eval F7: twelve reached callers
that way): a description names an operation by its op id (`update_type`) or
a schema by its kind ("schema kind filters"), and
`service/schemaprose_test.go` guards the served JSON.

## The shape

### Wire

`Issue` gained one optional member. `hint` is still prose and still carries
the route. What did change for a REST caller, beyond the ten messages
below: hints that shipped a `{space_id}` placeholder now carry the bound id
where the site knows it; placeholders follow the OpenAPI names (`{key}`,
`{type}`, not `{property_key}`, `{typeKey}`); object and chat ids are bound
too where the site knows them; abbreviated (`GET .../messages`) and
query-only read routes (`?ids=full`) are spelled out in full, while a
resend of the same request (`?dry_run=true`, `?limit=25`,
`?after=`/`?before=`, `?create_missing_options=true`) stays query-only by
design; three create refusals that were a bare route now read "create it
with …", "create a collection with …" and "upload one with …"; the empty-body
refusal's issue message is now "the body is empty" with the old text as its
hint; the view-filter type refusal's message lost its parenthesised route,
which is now its hint; the subtree-body refusal's message no longer names
`?block=`; the duplicate-key refusals no longer offer an update by a slug
that several holders answer to (the type variant's issue message now reads
"answered by several types", and the name-only property create says to
pass an explicit key rather than to use the ambiguous one); and a handful
of hints were reworded to stay
true on every surface (listed under "Review findings"). Every one of these
is prose; no status, code, path or acceptance changed. Consumers keying on `status`/`code` are unaffected;
consumers that only rendered `message` now miss the repairs that moved
into `issues`.

```json
{
  "path": "ops[0]",
  "message": "invalid set_properties op",
  "hint": "GET /v2/schemas/ops/set_properties for the op's schema and example",
  "see_also": [{"op": "get_op_schema", "params": {"op": "set_properties"}}]
}
```

`Ref` (`core/api/v2/model/ref.go`):

| member | meaning |
|---|---|
| `op` | the OpenAPI `operationId` — the key every wrapper already has, because the external MCP wrapper derives its tool names from it (`API-get-op-schema`) |
| `params` | path parameters by OpenAPI name; one left out is for the caller to fill (the prose keeps the `{name}` placeholder) |
| `query` | query parameters to send |
| no `op` | "the request that produced this issue, resent with `query`" — the `?create_missing_options=true` case |

**The prose spells each reference exactly as `Ref.String()` renders it**
(method, path with bound params substituted in one pass over the template,
`?k=v` query with keys sorted, nothing percent-encoded). That is the whole
trick: a wrapper re-spells a hint by find-and-replace of a string it can
compute, never by parsing the sentence. The rule is published in the
`see_also` description of the OpenAPI document. `TestOperationsMatchOpenAPI`
pins the operation table to the generated document, so a renamed route fails
the build rather than the hint. The space-reference echo in `RespondError`
(the caller's short spelling substituted back into every hint) is applied to
the references' parameter values too, so the contract survives it —
`TestEchoSpaceRefKeepsHintAndReferencesAligned`.

The contract is enforced by construction (`Hintf`) rather than by a check:
a site that builds a hint by concatenation, or formats a `Ref` with `%q` or
a precision verb, ships prose the wrapper cannot find. The guard catches the
concatenation case when the route is a literal; the verb case is not
guarded. A response-level assertion (every `see_also` entry's rendering
occurs in its hint) over the handler suites would close it; not done.

### Authoring

```go
v2model.Issue{Path: "/type", Message: "the shortcut needs a type key"}.
    Hintf("list keys with %s", v2model.RefListTypes(spaceId))
```

`Hintf` formats like `Sprintf`; every `Ref` argument renders in REST and is
recorded in `see_also`. Named helpers (`RefListProperties`, `RefGetOpSchema`,
…) exist for the operations hints use; `NewRef(op, "space_id", id)` for the
rest, with an empty value leaving a parameter unbound. `didYouMean` and
`unknownPropertyIssue` take a `v2model.Hint` (text + refs) instead of a string.

### Guards

- `core/api/v2/hintroutes_test.go` — no string literal (decoded, so an
  escape cannot hide a slash) in the service or handlers may contain a
  method before any path or ellipsis, an ellipsis path, a `/v2/` path or
  any bare `?name=` parameter; the schema documents, the route table and
  the operation table are the only exemptions. It does not look outside
  `core/api/v2` (the curated wrapper's own steering prose is covered by its
  own tests), it cannot see a route assembled by concatenation, and a
  whole-file exemption hides a future repair added to a schema file.
  Mutation-verified.
- `core/api/v2/model/ref_test.go` — table equals the OpenAPI document; every
  helper binds exactly its operation's path parameters; rendering cases.
- `core/api/prose` and `openapiprose_test.go` still pass on the regenerated
  document (`make openapi` was run; the `Ref` schema and `Issue.see_also` are
  in `core/api/docs/v2/openapi.{json,yaml}`).

## Answers to the brief's open questions

1. **Wire shape.** `see_also: []Ref` on `Issue`, keyed by operationId, in the
   OpenAPI document. Not on `Error`: the ten messages that carried a route
   now state the fact in `message` and carry the repair as an issue on the
   path parameter (`{"path":"space_id","message":"no space with this id is
   open on this account","hint":"list spaces with GET /v2/spaces","see_also":[…]}`).
   Fact and repair were already separated everywhere else (C6); this closes
   the exceptions. The other REST-visible changes are listed under "Wire".
2. **`Hint` stays prose.** Authoritative for humans and REST; `see_also` is
   the translation key beside it. Duplicated by design — there is no
   client-type signal on the request, and the prose costs nothing to keep.
3. **Success paths render hints now.** `warningsText` in the curated wrapper
   prints `warning: <message> — <hint>` after the same re-spell, at all seven
   renderers; the external wrapper re-spells `warnings` on success bodies.
   Mutation-verified.
4. **Sequencing against the embedded full wrapper.** The typed reference is
   the addressing key that wrapper will use: an operationId plus bound path
   and query arguments is a *partial* tool call — it names the operation and
   what is known, not a body (`create_property` still needs its fields) and
   not a header (an `If-Match` repair is not expressible). Both are additive
   if ever needed (`body`, `headers` members), and `?keys=name` is already
   expressible as a query. The curated wrapper's lookup (`toolVocab` in
   `steer.go`) and the external wrapper's (`see-also.ts`) are the two
   renderings that exist today; the embedded full wrapper adds a third row
   set, not a new mechanism.
5. **Inlining vs referencing.** Not done here; it is orthogonal (a prose
   change to the op-decode error), and the reference now makes the lookup
   one mechanical step on every surface. Still worth doing for
   `decodeStrictOp` — the ranked list in `result.md` has it first.

## The renderers

### Curated wrapper (`core/api/wrapper/steer.go`)

`restVocab` (ten regexes against the sentence) is gone. `toolVocab` maps
operationId → spelling, and one span engine (`respellSpans`) rewrites a
string in a single pass: the spans it knows (a reference's REST rendering in
a hint; a hint as rendered, `(hint)`, in the error text) become their
replacement verbatim, longest first where two match (the outline read
extends the plain read), and the catch-all runs only over the prose between
spans — so an inserted tool spelling is never rescanned or redacted. The
issue's message is a fact and gets the catch-all only: a quoted value that
happens to look like a route is never rewritten into a tool name. Every row
is a noun phrase — the server's sentences supply the verb — and promises
only what the tool does. The catch-all `restRoute` regex stays for a server
build without references; its invariant — no route survives outside a
protected span — is still tested. A constructor-level
round trip (`v2model` constructors → `MarshalJSON` → `decodeAPIError` →
`deRest`) pins the wire shape so the hand-written fixtures cannot drift
from what the model package serializes; it does not exercise a real handler
refusal end to end.

An operation with no row renders as `an operation outside this tool set
(list_members)`: the caller learns the repair exists and where it is not,
instead of the old dead-end `the HTTP API`. `TestToolVocabularyIsRouteFree`
walks every operationId.

The space steer's `supersedes` is now an operationId (`list_spaces`) rather
than a prose fragment: when the specific repair fires, issues naming that op
lose their hint.

`ToolError` gained `Message`; `Text` is rewritten in place (executors
re-spell op paths and append what was written before the vocabulary pass, and
that must survive — the first cut re-rendered from the envelope and broke
eleven tests).

### External wrapper (`anytype-mcp`, `src/mcp/see-also.ts`)

`respellResponse` runs on every error body and on success bodies whose
declared response for the actual status is JSON (a download's content comes
back untouched; a missing or content-less declaration is treated as JSON):
each reference's
REST rendering in the hint becomes `API-get-op-schema {"op":"set_properties"}`
(tool name + arguments, unbound parameters as `<name>`, query values typed by
the operation's declared OpenAPI parameter type — a resend reference is
typed by the operation that produced the issue and renders as the arguments
alone), and the reference itself is annotated with `tool` and `args` so a
caller can act on the data without reading the prose. An op the loaded spec
does not have leaves the prose untouched. The pass is single-scan, skips
malformed references, and returns the body unchanged on any failure: it is
a courtesy over a response that is already correct and must never be why a
response fails. `restSpelling` is the Go `Ref.String` rule verbatim, with
the Go test cases copied into `see-also.test.ts`.

The hook sits in `proxy.ts`: the index is built in the constructor from
the loaded document, the success path re-spells before `compactWriteResponse`
when the response declared for the actual status is JSON, and the
structured error path re-spells unconditionally. Rebased onto the v2
discovery merge (anytype-mcp PR 152).

## Constraints that still apply

- Served prose rules (`core/api/prose/userfacing_test.go`,
  `openapiprose_test.go`): one sentence, ~200 characters, no spec references.
  Rationale lives in Go comments.
- `make openapi` after touching swagger annotations, then re-run the prose
  tests.
- Every new guard mutation-verified: remove it, watch the named test fail,
  restore it.
- Do not add a route to a served string. The guard will refuse it; name the
  operation with a `Ref`.

## Review findings (three lenses, 2026-09-18)

Confirmed and fixed:

- **Space-reference echo broke the contract** (all three lenses). The
  echo rewrote hint text to the caller's short space id but not the
  reference's `space_id`, so the rendering no longer occurred in the hint.
  Fixed in `echoRefs`, pinned by `TestEchoSpaceRefKeepsHintAndReferencesAligned`.
- **Sequential replacement rescanned inserted text**, and **placeholder
  substitution iterated a map** (nondeterministic when a bound value looked
  like a placeholder). Both renderers now substitute in one pass.
- **Messages were rewritten by the hint's references**; a quoted value
  spelling a route would have become a tool name. Messages now get the
  catch-all only.
- **Five query-parameter hints escaped the guard** (`?ids=full`,
  `?dry_run=true`, `?offset=&limit=`, `?after=`/`?before=`, `?block=`).
  Converted; the guard now matches any `?name=`.
- **Curated renderings that were false or broken**: `find` presented as a
  type listing (it is not; the row now says types are visible on `find`
  results and that there is no listing); `describe` "for its id" (it serves
  no id; the sentence no longer claims a source for it); the
  `list_properties` row carried its own verb and collided with three
  sentences (now a noun phrase); query and collection reads were called
  "outside this tool set" (they are `read` on the list object); "page one
  space with `find`" (`find` cannot page; the sentence now says "search one
  space at a time"); "at the editing tools" (now "through"); the
  block-not-found repair pointed at the outline read where the old wrapper
  deliberately steered to the full read (the server now names the plain
  read, which lists the same blocks with their full text).
- **The shared RPC-error conversion** put `path: "space_id"` on space
  creation, which has no such parameter, and asserted an account status a
  missing store does not establish. Now path-less with a neutral fact.
- **External wrapper**: a `see_also` entry that was `null`, or an op id that
  named an `Object.prototype` member, threw; query arguments were typed by
  spelling rather than schema (`limit: "5"`). All fixed and tested.

Round two (the same three sessions, resumed against the amended tree):

- **The round-one echo fix over-reached**: it shortened any reference value
  containing the space id, so a property key or option name that happened
  to contain it was re-addressed. The echo now re-spells only a value that
  IS the space id, and the hint span-wise (each rendering follows its
  reference; the prose between renderings gets the plain substitution), so
  the hint and its references keep agreeing even for such a key. Pinned by
  a subtest of `TestEchoSpaceRefKeepsHintAndReferencesAligned`.
- **The "message is a fact" rule did not hold in the served text**: the
  wrapper's `Text` was still rewritten by references, message part
  included. `deRest` now substitutes each hint as a whole span of the text
  (old hint → re-spelled hint), so an executor's edits around it survive
  and the message is never touched by a reference; both `Text` and
  `Message` are asserted.
- **The catch-all rescanned inserted tool spellings**: a bound value that
  looked like a route was redacted into "the HTTP API" after the reference
  had been rendered. The catch-all now sees only the prose between
  renderings.
- **External wrapper**: JSON file downloads were rewritten when their
  content looked like an envelope (now skipped for any operation whose
  success response is not JSON, with a proxy test); `$ref` parameters and
  schemas were typed as strings (now resolved through the document's
  components); malformed references were annotated (now passed through).
- **Three chat hints spelled the route with an ASCII ellipsis**
  (`GET .../messages`) and escaped both the original and the widened guard;
  converted to bound `get_chat_messages` references (the first with
  `limit=1`), and the guard now matches `.../` too.
- **Curated wording, second pass**: the type-listing row read as an
  instruction to list keys "with the types find results show"; it is now
  "a type listing (not in this tool set; `find` results show each object's
  type)". A read with `?ids=full` rendered as plain `read`, which serves
  compact labels and cannot satisfy the clone repair; it now renders as
  "a full-id read (not in this tool set)". The type-views hint named
  `insert_view`, which the curated wrapper does not offer; it now says
  "edit its views through" the patch operation without naming ops.
- The inventory count is 75 under the final rule (66 under the original
  guard, 72 before the ASCII-ellipsis widening), and the round-trip test
  is described as constructor-level, not end to end.

Round three (the same sessions, resumed once more):

- **The round-two echo still over-reached**: a value that EQUALS the space
  id was echoed whether or not it was a space id. Only the `space_id`
  binding is echoed now; a property key equal to the id stays a key.
- **The round-two text rewrite still had two holes**: the final catch-all
  ran over the whole text and redacted the tool spellings the span engine
  had just protected, and substituting the hint text wherever it occurred
  also rewrote a message that quoted it. The text now goes through the same
  span engine keyed on each hint as rendered — the parenthesised `(hint)`
  form, which a quoted value in a message does not take — with the
  catch-all confined to everything outside those spans. A message whose
  own text is `x (hint)` is the remaining, accepted, false positive.
- **`find` serves handles, not full ids**, so the collection-items repair
  said so falsely on the curated surface; the row now says it.
- **External wrapper**: response `$ref`s and `2XX` range declarations
  bypassed the download gate (resolved and matched by actual status now);
  schema reference chains typed as string (resolved with cycle protection);
  arrays were accepted as parameter maps (rejected).

Round four (merge review, all three lenses): **no merge-blocking finding.**
Nits fixed: bound path values in the external wrapper are now typed by
their declared parameter like query values; the view-filter type refusal's
message-to-hint move is listed under "Wire"; the malformed-reference test
returned early on a non-string message and did not exercise the list it
supplied; the type tool's duplicate-name cleanup still matched the retired
"the HTTP API" text and now drops the `update_type` hint by operation; the
test-only `spacesListRepair` constant moved into the test file; overview
prose that still described `strings.Replacer` and unconditional success
rewriting was corrected. Pre-existing and untouched: the upload-file
request schema differs between the generated JSON and YAML documents.

Round five (three FRESH reviewers, no prior context, merge-review framing):
no merge-blocking finding. Fixed: the curated resend spelling offered a
parameter no tool takes (option-creation consent and dry runs are host
settings; it now says so); the block-listing repair named the plain read,
whose curated form serves ROWS for a query or collection, not blocks — it
names the outline read again, which lists every object's blocks, while the
locator's full-text repair keeps the plain read (this reverses a round-one
change, with the reason recorded here); the `list_properties` row promised
an exhaustive list where `describe` caps at 120 names; the duplicate-key
hints bound an update to a slug several holders answer to, which that
update would refuse as ambiguous; the published `Ref` descriptions now
state the rendering rule (verbatim substitution, no percent-encoding, kept
placeholders, sorted query, query-only rendering for a resend); the guard
decodes literals and matches versionless and ellipsis routes; the doc's
"Wire" section and both PR descriptions were completed.

Round six (three fresh reviewers on the MERGED commits, develop feb1ee1ad
and main c6f069f): nothing needing a must-fix follow-up. Fixed in the
follow-up: the name-only property create overwrote the ambiguous-slug
repair with "use the existing property <slug>", which cannot be followed
(now "pass an explicit different key", tested for both request shapes);
the three list-read warnings that said "pass view=<id>" carried no
reference and escaped the guard — on the curated wrapper, whose `read`
takes no view argument, they now render as a parameter these tools do not
take (the guard matches `name=<placeholder>` mentions too); the
`list_properties` row overstated `describe`'s cap (only the off-type
section is capped; it now says the listing may be truncated); the Wire
section gained the three create refusals' rewording and the several-types
message. Nothing that landed on develop meanwhile touched `core/api`.

Round seven (three fresh reviewers on the merged commits plus the
follow-up): nothing must-fix. Fixed in the follow-up: a key collision with
a built-in property (installed or not) offered an update that the update
route refuses as read-only or 404s — the key is now called reserved, with
no reference; the curated `describe … options` refused a hidden property's
exact key because its index comes from the listing, which hides such
properties, while the consent refusal names that key and points there —
it now tries the options read by the key as given and only refuses on the
server's own 404; a server build that predates the references spells the
block hint as "GET the object with ?outline=true", which neither the
references nor the route catch-all touched — a bare `?name=value` is now
redacted too; the external wrapper's response gate rejected vendor JSON
media types with digits in the subtype (`application/vnd.anytype.v2+json`).

Accepted, not fixed (added in round five):

- A server hint that names a tool (`set_cell` in the nested-block repair)
  cannot know the caller's tier; on the small tier that tool is absent.
- The locator's full-text repair names the plain read, whose curated form
  serves rows for a query or collection; text edits on a list object's own
  blocks are rare.

Accepted, not fixed:

- The `hint == Ref.String()` contract is by construction, not checked at
  response time (see "The shape").
- `describe` on the curated wrapper injects the hidden `Name` property, so
  "lists user-visible properties only" is very slightly off on that surface
  for the one over-15-keys property refusal.
- The guard exempts whole schema-document files.

## Not in scope

The ranked fix list in `result.md` also has: inlining the op envelope in
`decodeStrictOp`'s error; the object-channel unknown-op error pointing at the
type surface; naming every offending key instead of one; deleting the
irrelevant If-Match line from unknown-field errors. Independent and cheaper;
none needed this design.
