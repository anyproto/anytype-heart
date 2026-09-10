# API v2 Swagger comment review

2026-09-08. **36 consolidated findings: 18 P2 and 18 P3.**

The review standard is that an API consumer can understand the documentation with HTTP/JSON knowledge and ordinary public concepts such as objects, properties and spaces. Anytype storage formats, codecs, Go representations, design history and private documents should not be prerequisites.

Five separate max-effort agents reviewed the methods and model properties. Coverage includes all 50 operations, 59 component schemas, API/tag descriptions, inherited authentication comments and generated response prose. Each reported public quote was checked against `core/api/docs/v2/openapi.json`; relevant implementation and existing tests were read to verify behavioral claims. This is a comments review, not a general API implementation audit.

No literal internal `.md` references, section markers or C-number design references were found in emitted summaries/descriptions. Many exist in ordinary Go comments that Swagger does not publish; those are excluded. The findings instead identify unclear public language, exposed implementation concepts and a few descriptions that teach the wrong behavior. Repeated space-ID comments from three scopes are consolidated in G4.

P2 means the text can lead a caller to construct the wrong request or misinterpret behavior. P3 means a meaningful clarity or terminology improvement. Suggested directions below record the first review. A second round of five separate max-effort agents has prepared and applied concrete wording proposals in an isolated worktree; see the final section. No source comments or generated documents in the primary working tree were edited for this review.

Sources refer to the reviewed working tree, which already contained unrelated changes. JSON pointers are into the generated v2 document. A temporary generation outside the repository confirmed that the installed Swagger tool reproduces the reviewed model-field descriptions.

## Finding index

| ID | Priority | Finding |
| --- | --- | --- |
| O1 | P2 | Correct the type document's public `kind` value |
| O2 | P2 | Make the type-update field list complete and replace “typed envelope” with JSON structure |
| O3 | P2 | Replace ambiguous read-back instructions with the actual mutation formats |
| O4 | P2 | State the PATCH request shape and the actual text-selector fields |
| O5 | P2 | Clarify the deletion ownership rule instead of describing it as one credential's history |
| O6 | P3 | Connect document-format vocabulary to concrete public request definitions |
| O7 | P3 | Use language-neutral terminology for outline text truncation |
| L1 | P2 | Define the search filter and sort formats instead of merely naming them |
| L2 | P2 | Explain that omitting `view` skips saved view filters and sorting |
| L3 | P2 | Say that an unresolved placeholder removes a filter condition |
| L4 | P2 | Remove the ineffective option-creation parameter from collection creation |
| L5 | P3 | Replace “wrong-layout” with the public object kind the route requires |
| C1 | P2 | Explain the actual formatting syntax instead of “marks” |
| C2 | P2 | Replace “read watermark” with a usable mark-as-read recipe |
| C3 | P2 | Separate page traversal from the order within each message page |
| C4 | P2 | Describe delete warnings as potential attachment cleanup |
| C5 | P3 | Replace attachment “layout” with the public classification |
| C6 | P3 | Keep the stream recovery caveat without the state-ID implementation rationale |
| C7 | P3 | State the reaction toggle's actor and boolean meaning directly |
| M1 | P2 | State the permission values instead of the intended implementation technique |
| M2 | P2 | The public scope description includes a credential type that cannot reach this response |
| M3 | P3 | Explain all-spaces coverage without “boundary field” and “tech space” jargon |
| M4 | P3 | Identify the key without requiring knowledge of app-link storage |
| M5 | P3 | Describe the notice's purpose and presence condition |
| M6 | P3 | Keep request-path examples in edit receipts, remove Go names and editor jargon |
| M7 | P3 | “Identity key” obscures where a returned type/property key can be reused |
| M8 | P3 | Shared mutation-result descriptions should not call every affected object “created” |
| M9 | P3 | Pairing challenge fields should describe the value's round trip |
| G1 | P2 | Limit the ETag claim to object PATCH; chat cursors do not provide equivalent edit protection |
| G2 | P2 | Describe actual error bodies instead of promising one body and retracting it in middleware jargon |
| G3 | P2 | The published 413 descriptions refer to limits that the same generation step removes |
| G4 | P3 | Treat space IDs as opaque values; remove CID/replication-key anatomy |
| G5 | P3 | Replace the key-vocabulary monologue with concrete input/output rules |
| G6 | P3 | Reduce and structure the 1,195-word overview; move detailed streaming rules to the stream operation |
| G7 | P3 | Express whoami's three access cases directly instead of first asserting a two-case rule |
| G8 | P3 | Explain what image width selects |

## Objects, types, properties, templates and schemas

### O1 — P2 — Correct the type document's public `kind` value

- **Source:** `core/api/v2/handler/discovery.go:120`.
- **Exact quote:** `The kind:objectType AnyBlock document + etag`.
- **Operation / pointer:** `get_type`, GET `/v2/spaces/{space_id}/types/{type}`; `#/paths/~1v2~1spaces~1{space_id}~1types~1{type}/get/responses/200/description`.
- **Generated visibility:** The 200 response description contains that text verbatim; its response schema is an unconstrained object, so there is no enum nearby that corrects the spelling. The create-type operation instead says `kind "object_type"`.
- **Why it matters:** A consumer implementing a discriminator or constructing a type document from this response description would use a value the API does not emit or accept for the type kind.
- **Implementation evidence:** `core/api/v2/service/discovery.go:266` delegates to `GetObject`; `core/api/v2/service/schema_write.go:110` accepts only `object_type` when a kind is supplied. The pinned local AnyBlock implementation maps the type category to `"object_type"` at `/Users/roman/anytype/any-block_querysource/codec/anyblockjson/json.go:326`. The existing type example at `core/api/v2/service/schemas.go:51` uses that same value.
- **Suggested replacement:** `AnyBlock document with kind "object_type", including its etag.`

### O2 — P2 — Make the type-update field list complete and replace “typed envelope” with JSON structure

- **Source:** `core/api/v2/handler/create.go:181`.
- **Exact quotes:** `the icon is the typed envelope \`icon\``; `Any other key is refused.`
- **Operation / pointer:** `update_type`, PATCH `/v2/spaces/{space_id}/types/{type}`; `#/paths/~1v2~1spaces~1{space_id}~1types~1{type}/patch/description`.
- **Generated visibility:** Both phrases are emitted verbatim. The generated request body is only `{ "type": "object" }`, without an explanatory schema or example.
- **Why it matters:** The description presents its named fields as exhaustive but leaves out supported `type_settings.plural_name`. It also assumes the reader knows what a “typed envelope” is and how `icon` is represented. A consumer cannot reliably construct this patch or discover the supported plural-name update from this operation.
- **Implementation evidence:** `core/api/v2/service/schema_write.go:465` defines top-level `properties`, `type_settings`, and `icon`; lines 480–483 include `layout`, `plural_name`, and `property_definitions`; lines 565–576 apply the plural name. The documented replacement behavior for `property_definitions` is real and should remain.
- **Suggested direction:** Name the allowed JSON paths explicitly: `properties.name`, `properties.description`, `type_settings.layout`, `type_settings.plural_name`, `type_settings.property_definitions`, and top-level `icon`. Describe `icon` as an object and show a minimal example such as `{"icon":{"format":"emoji","emoji":"✅"}}`. Preserve that supplying `property_definitions` replaces the type's property lists and creates missing definitions; omitted lists are not an append operation.

### O3 — P2 — Replace ambiguous read-back instructions with the actual mutation formats

- **Sources:** `core/api/v2/handler/object.go:17` and `core/api/v2/handler/object.go:26`; related terminology at `core/api/v2/handler/discovery.go:119`.
- **Exact quotes:** `full is the export shape, with full ids everywhere, and the shape to send back`; `markdown cannot be sent back`; related type-read wording: `short labels for minted view and block ids`.
- **Operations / pointers:** `get_object`: `#/paths/~1v2~1spaces~1{space_id}~1objects~1{object_id}/get/description` and `#/paths/~1v2~1spaces~1{space_id}~1objects~1{object_id}/get/parameters/5/description`. Related `get_type`: `#/paths/~1v2~1spaces~1{space_id}~1types~1{type}/get/parameters/2/description`.
- **Generated visibility:** All quoted phrases are emitted verbatim. The object read exposes a generic JSON-object response and does not identify a supported destination for “send back.”
- **Why it matters:** “Edit shape,” “export shape,” and “minted” are unexplained format terminology. More materially, “the shape to send back” implies a full-document update path, although object updates accept an `ops` body. The blanket markdown statement also obscures the supported markdown input channels. The useful distinction is between replacing the existing object with a returned representation and using documented creation/edit inputs.
- **Implementation evidence:** `core/api/v2/router.go:314` documents and registers PATCH-only editing, with no full-document replacement route. `core/api/v2/service/edit.go:427` rejects every top-level field except `ops`. `core/api/v2/service/object.go:588` resolves full IDs or unique suffixes for targeting. Markdown is accepted by the create shortcut (`core/api/v2/service/create.go:201`) and `insert_blocks` (`core/api/v2/service/ops.go:340`).
- **Suggested direction:** Describe `compact` as returning shorter labels for generated block/view IDs that can target edit operations; describe `full` as preserving complete IDs for export or cloning. State that existing objects are updated with PATCH operations, and that the Markdown read response cannot replace an object. Preserve the subtree write restriction. Avoid claiming Markdown itself is never accepted as input.

### O4 — P2 — State the PATCH request shape and the actual text-selector fields

- **Source:** `core/api/v2/handler/edit.go:28`.
- **Exact quote:** `can address a block by its exact text instead of an id`.
- **Operation / pointer:** `patch_object`, PATCH `/v2/spaces/{space_id}/objects/{object_id}`; `#/paths/~1v2~1spaces~1{space_id}~1objects~1{object_id}/patch/description`.
- **Generated visibility:** The description discusses operations but never shows the required `{"ops":[...]}` wrapper. Its generated request-body schema is only `{ "type": "object" }`. The separate schema-operation endpoint explains an `ops` array, but this operation does not link to it or identify `match` versus `find`.
- **Why it matters:** “Exact text” can be read as whole-block equality, while matching is by an exact substring. The description also groups three operations without explaining that two use `match`, whereas `replace_text` uses `find` when `id` is omitted. Putting copied text in `id`, or giving `match` to `replace_text`, is not the described working input.
- **Implementation evidence:** `core/api/v2/service/edit.go:427` requires a nonempty `ops` array and rejects other top-level fields. `core/api/v2/service/ops.go:327`, `:367`, and `:374` define the different selectors. `core/api/v2/service/locator.go:83` uses `strings.Contains`, with zero/multiple-block matches rejected. `core/api/v2/service/stateops.go:1953` uses `find` as the locator for an ID-less `replace_text`; matching uses the text returned by the API, including inline markup.
- **Suggested direction:** Start with `Send {"ops":[...]}; each operation is documented at GET /v2/schemas/ops/{op}.` Then say that `update_block` and `delete_block` accept `match` instead of `id`, and `replace_text` uses `find` when `id` is absent. The supplied substring must occur in exactly one eligible block and must match the returned text, including markup. Retain ordered application, all-or-nothing object edits, visibility of earlier edits to later operations, and returned created IDs. The existing “none of them land” / “not guessed at” prose can be shortened while doing this.

### O5 — P2 — Clarify the deletion ownership rule instead of describing it as one credential's history

- **Sources:** `core/api/v2/handler/edit.go:61` and `:62`.
- **Exact quotes:** `Delete an object this key created`; `Only objects this key created can be deleted.`; `made before this route shipped are refused for good`.
- **Operation / pointers:** `delete_object`, DELETE `/v2/spaces/{space_id}/objects/{object_id}`; `#/paths/~1v2~1spaces~1{space_id}~1objects~1{object_id}/delete/summary` and `#/paths/~1v2~1spaces~1{space_id}~1objects~1{object_id}/delete/description`.
- **Generated visibility:** The summary and description contain these phrases verbatim. The description spends most of its space on creation-record history, without naming the identity the deletion gate actually compares.
- **Why it matters:** A consumer naturally reads “this key” as the particular bearer credential. The implementation checks the creating account and the integration's exact recorded application name, not the API key token or key ID. Relinking under the same application name is therefore materially different from changing that name. The shipping-history explanation does not communicate this distinction and forces the reader to infer storage behavior.
- **Implementation evidence:** `core/api/v2/service/delete.go:190` checks the creating-account match and recorded integration name; `:204` reads `domain.IntegrationNameFromCtx`; `:217` compares the recorded and current names exactly. Its error text at `:218` explicitly directs use of a key paired under the recorded name. `:94` performs ownership checks; `:107` returns before the actual archive call, and `:122` maps later object restrictions to 403.
- **Suggested direction:** Describe deletion as limited to objects created by the current account through an integration whose `app_name` exactly matches this key's application name. Retain that missing creator records, other members' creations, and system objects are rejected, and that dry runs do not evaluate every archive-time restriction. Replace release-history narration with the practical instruction to archive unsupported objects in Anytype. This is a documentation clarification of the existing gate, not a recommendation to change authorization behavior.

### O6 — P3 — Connect document-format vocabulary to concrete public request definitions

- **Sources:** `core/api/v2/handler/create.go:91`, `:121`, `:151`; `core/api/v2/handler/validate.go:18`.
- **Exact quotes:** `the shortcut {type, name, properties, markdown}`; `` `template_for` names the type key this template starts an object of. ``; `the type's api key, layout and plural name live in \`type_settings\``; `Validate an AnyBlock document`.
- **Operations / pointers:** `create_object`: `#/paths/~1v2~1spaces~1{space_id}~1objects/post/description`; `create_template`: `#/paths/~1v2~1spaces~1{space_id}~1templates/post/description`; `create_type`: `#/paths/~1v2~1spaces~1{space_id}~1types/post/description`; `validate`: `#/paths/~1v2~1validate/post/summary`.
- **Generated visibility:** These comments are emitted verbatim. Each corresponding generated request body has only `{ "type": "object" }`, so the operation offers no concrete JSON structure. The global description does explain runtime schema discovery in general, which mitigates this finding; it does not identify these particular kinds or make the shorthand a usable example.
- **Why it matters:** The object description begins with exceptional option behavior before explaining the two input forms; its braces list field names rather than usable JSON or required fields. The template sentence neither says it expects a document nor makes `template_for` explicitly required. Calling a type identifier an “api key” can also be confused with the authentication credential used throughout the API. Readers need public request-format guidance, not prior familiarity with AnyBlock's structure.
- **Suggested direction:** Introduce AnyBlock briefly as the JSON representation of an object's properties and blocks, then link the applicable schema/example: `/v2/schemas/object`, `/v2/schemas/shortcut`, `/v2/schemas/template`, and `/v2/schemas/type`. Put the object input forms first, retain the `formatVersion`/`blocks` discriminator and option-creation limitation, and say the shortcut requires a type key. For templates, say `template_for` is the required key of the type that objects created from the template will use. Call `type_settings.api_key` the **type key**, not an API key. Keep validation's existing structural-only and HTTP-200-findings limitations.
- **Implementation evidence:** The public schema-kind mapping and examples are at `core/api/v2/service/schemas.go:33`; the shortcut requires `type` at `core/api/v2/service/create.go:175`; templates require `template_for` at `:501`.

### O7 — P3 — Use language-neutral terminology for outline text truncation

- **Source:** `core/api/v2/handler/object.go:24`.
- **Exact quote:** `text truncated to 80 runes`.
- **Operation / pointer:** `get_object`; `#/paths/~1v2~1spaces~1{space_id}~1objects~1{object_id}/get/parameters/3/description`.
- **Generated visibility:** The `outline` parameter description contains this phrase verbatim.
- **Why it matters:** “Rune” is Go terminology and does not tell a JSON API consumer how non-ASCII text is counted. The limit is useful and should remain.
- **Implementation evidence:** `core/api/v2/service/object.go:411` converts the text to `[]rune`, keeps the first 80 code points, then appends `…` if truncated.
- **Suggested replacement:** `Return an outline with each block's indent, id, type, and the first 80 Unicode code points of its text; truncated text ends with ….`

## Search and lists

### L1 — P2 — Define the search filter and sort formats instead of merely naming them

Source: `core/api/v2/handler/search.go:87`, with the referenced fields at `core/api/v2/model/model.go:480`, `:481`, and `:482`.

Exact quote: “the compact string and the structured array”. The `Sorts` declaration is `Sorts   []map[string]any` and has no field description.

Published locations: `/paths/~1v2~1spaces~1{space_id}~1search/post/description`; `/components/schemas/SearchRequestDoc/properties/filter`; `/components/schemas/SearchRequestDoc/properties/filters`; `/components/schemas/SearchRequestDoc/properties/sorts`. Both search operations' request bodies reference `#/components/schemas/SearchRequestDoc`.

Visibility evidence: the operation description contains the quote verbatim. The schema supplies only `string` for `filter` and arrays of unrestricted objects for `filters` and `sorts`; none has a description or example. There is no emitted definition of the expression grammar, filter-node keys, or sort-object keys at these locations.

Impact: knowing JSON does not tell callers whether to write SQL-like expressions, which names represent operators, or how to express sorting. Saying the forms are “two spellings of the same thing” also fails to explain their different date representations. Callers must guess, inspect implementation code, or discover another endpoint independently.

Behavior checked: `service/search.go:301` parses the compact grammar; `:382` decodes sort objects; `:736` rejects date strings in structured filters, which use Unix seconds. `service/schemas.go:92` already provides a search schema/example; `:147` and `:220` provide the structured filters schema and compact grammar through the public schema endpoint.

Suggested direction: add consumer-facing field descriptions and one example of each input form, and directly point to `GET /v2/schemas/search` and `GET /v2/schemas/filters` for the complete contract. For example: `filter: "done = false"`; `filters: [{"property":"done","condition":"equal","value":false}]`; `sorts: [{"property":"name","direction":"asc"}]`. State that the two filter fields are mutually exclusive and explain the date representation difference. This finding concerns undefined public formats, not the non-emitted type comment.

### L2 — P2 — Explain that omitting `view` skips saved view filters and sorting

Source: `core/api/v2/handler/list_read.go:40` (also `:104`); related summary at `:33` and create-query description at `core/api/v2/handler/create.go:320`.

Exact quotes: “Stored view id (exact or unique suffix)” and “Run a query and list what it matches”.

Published locations: `/paths/~1v2~1spaces~1{space_id}~1queries~1{query_id}~1objects/get/parameters/2/description` and `/paths/~1v2~1spaces~1{space_id}~1collections~1{collection_id}~1objects/get/parameters/2/description`.

Visibility evidence: both optional `view` parameters contain the same exact description, with no documented default or explanation of what selecting a view changes. The query endpoint description discusses dynamic placeholders but does not explain this behavior.

Impact: a caller can create a query with `filter: "done = false"`, then reasonably expect its `/objects` endpoint to return the saved filtered result. The filter is actually stored in a view, so omitting `view` returns results matching the query's underlying type/property criteria without that filter. Understanding this currently depends on Anytype's query/view model.

Behavior checked: `service/list_create.go:445`–`:455` puts top-level filters/sorts in an automatically created view; `service/list_read.go:207`–`:216` applies view filters/sorts only when a view reference is supplied. The underlying query criteria still apply at `:221`–`:225`. Collection order stays intact unless the selected view specifies sorts (`:243`).

Suggested wording for queries: “Optional view ID from GET …/views, accepted in full or as a unique suffix. Applies that view's filters and sorting. If omitted, only the query's type/property criteria apply; saved view filters and sorting are not applied.” Explain the same opt-in behavior for collections and say that collection order changes only when the selected view defines sorting.

### L3 — P2 — Say that an unresolved placeholder removes a filter condition

Source: `core/api/v2/handler/list_read.go:34`.

Exact quote: “One that cannot be resolved becomes a warning rather than a silently empty result.”

Published location: `/paths/~1v2~1spaces~1{space_id}~1queries~1{query_id}~1objects/get/description`.

Visibility evidence: the quote is the operation's current description; it says nothing about ignoring a condition.

Impact: this describes the warning but omits the consequence for the returned objects. A caller may treat the warning as diagnostic while assuming all saved conditions still constrain the results. Dropping an unresolved condition can return additional objects. This is distinct from finding 2: it happens even when the caller explicitly selects a view.

Behavior checked: `service/list_read.go:501`–`:516` records a warning and marks unresolved placeholders for removal; `:522`–`:568` omits the entire affected filter node. The existing test at `service/list_read_test.go:125`–`:146` verifies that a formerly restrictive condition is dropped and both objects return.

Suggested wording: “Dynamic values in the selected view are resolved when the query runs. If a value cannot be resolved, its filter condition is ignored and a warning is returned; the results may include objects that would otherwise be excluded.”

### L4 — P2 — Remove the ineffective option-creation parameter from collection creation

Source: `core/api/v2/handler/create.go:360`.

Exact quote: “Create select options for names the property does not hold yet (default false: an unmatched name is refused)”.

Published location: `/paths/~1v2~1spaces~1{space_id}~1collections/post/parameters/2/description`, on the `create_missing_options` parameter.

Visibility evidence: both the parameter and quoted behavior appear in the generated operation. This is an inaccurate public contract, not just awkward phrasing.

Impact: the operation advertises a way to create missing select options and implies the collection request can contain values requiring that behavior. Its body supports only a name and member IDs, and setting the parameter cannot do what this description promises.

Behavior checked: `handler/create.go:374` invokes `CreateCollection` without reading `mayCreateMissingOptions`; `service/list_create.go:167` takes no such flag; `:204`–`:226` builds only a collection name and membership list and explicitly states that no select name reaches the option resolver. The corresponding parameter is meaningful on create-query and should remain documented there.

Suggested direction: remove the collection-only `@Param create_missing_options` annotation, or explicitly mark it ignored if retaining it is a compatibility requirement. No implementation change is needed to correct this description.

### L5 — P3 — Replace “wrong-layout” with the public object kind the route requires

Source: `core/api/v2/handler/list_read.go:45`, `:76`, `:109`, and `:140`.

Exact quotes: “Wrong-layout target or invalid params” and “Wrong-layout target”.

Published locations: the `/responses/400/description` entries below each of these operation pointers:

- `/paths/~1v2~1spaces~1{space_id}~1queries~1{query_id}~1objects/get`
- `/paths/~1v2~1spaces~1{space_id}~1queries~1{query_id}~1views/get`
- `/paths/~1v2~1spaces~1{space_id}~1collections~1{collection_id}~1objects/get`
- `/paths/~1v2~1spaces~1{space_id}~1collections~1{collection_id}~1views/get`

Visibility evidence: all four descriptions retain these phrases verbatim in the generated JSON.

Impact: “layout” names the implementation's classification check and can be mistaken for the appearance of the view. The useful information is that the supplied object is not a query/collection.

Behavior checked: `service/list_read.go:98`–`:117` checks the stored layout but reports errors in public terms (“is a collection, not a query”, and vice versa).

Suggested wording: “The object is not a query, or the request parameters are invalid”; for query views, “The object is not a query”. Use “collection” for the collection routes.

## Chat

### C1 — P2 — Explain the actual formatting syntax instead of “marks”

**Source:** `core/api/v2/handler/chat.go:155` and `:188`; corresponding text fields at `core/api/v2/model/chat.go:45`, `:109`, and `:117`.

**Exact quotes:** “`*`, `[` and a mention tag mint real marks”; “Every mark is re-derived from the text you send”.

**Published locations:** `/paths/~1v2~1spaces~1{space_id}~1chats~1{chat_id}~1messages/post/description` (`add_chat_message`) and `/paths/~1v2~1spaces~1{space_id}~1chats~1{chat_id}~1messages~1{message_id}/patch/description` (`edit_chat_message`). Both quotes are present verbatim in JSON. The operations reference `/components/schemas/AddChatMessageRequest/properties/text` and `/components/schemas/EditChatMessageRequest/properties/text`, which only say `type: string`; `/components/schemas/ChatMessage/properties/text` is equally unexplained.

**Problem:** “Marks” is an internal representation, and “a mention tag” does not tell a caller what to send. The warning names punctuation without showing complete formatting or mention syntax, so someone can neither confidently compose formatted text nor preserve it while editing. The required `object_id` attribute of a mention is not documented in the published operation/schema text. The global overview does not supply an inline-formatting reference either.

**Direction:** Describe `text` as formatted text in the API's inline markup format, with a short public syntax reference or examples such as `**bold**`, `[label](https://example.com)`, and `<mention object_id="…">Name</mention>`. Define where a mention target ID comes from. Show a literal escape in a JSON example so callers see both markup and JSON escaping. For edit, use: “Send the complete replacement text, including any formatting and mentions to retain. Other message content, including attachments, reply target, and message style, is preserved. Editing another member's message returns 403.” Preserve the 8000 UTF-16-unit limit and specify that it applies to text after markup is interpreted, along with the emoji caveat and attachment limit.

**Behavior checked:** `core/api/v2/service/chat.go:219` and `:262` parse the same inline format for create/edit; `:224` and `:267` validate parsed text. `core/api/v2/model/chat.go:217` returns formatted source. Existing tests show bold at `core/api/v2/model/chat_test.go:50` and the full mention syntax at `:93`; the locally referenced parser rejects `<mention>` without `object_id`. This is a published explanation gap, not a claim that the private SPEC/codec comments are visible.

### C2 — P2 — Replace “read watermark” with a usable mark-as-read recipe

**Source:** `core/api/v2/handler/chat.go:282`, `:283`, `:292`, `:293`; related fields `core/api/v2/model/chat.go:153`–`:155`.

**Exact quotes:** “Move a chat's read watermark”; “the newest message's order, and the state's own id”; “An empty value for either would silently mark nothing, so it is refused.”

**Published locations:** `/paths/~1v2~1spaces~1{space_id}~1chats~1{chat_id}~1read/post/summary`, `/description`, `/requestBody/description`, and `/responses/200/description` (`read_chat`). The generated description is verbatim. Its request schema `/components/schemas/ChatReadRequest/properties/scope` is just an unconstrained string with no description, default, or enum; `up_to` and `last_state_id` also have no descriptions.

**Problem:** The operation leads with implementation vocabulary and a rationale for rejecting empty values, while leaving out the supported scopes, the default, and an exact field mapping. “The state's own id” is especially unhelpful when the response field is named `state.last_state_id`, not `state.id`. Callers cannot discover the `mentions` mode from this operation or schema, or confidently determine which fields are required for each scope.

**Direction:** Summary: “Mark chat messages, mentions, or reactions as read”. Description: “`scope` is `messages` (default), `mentions`, or `reactions`. For messages or mentions, set `up_to` to the last message's `order` you want to mark read, inclusive, and `last_state_id` to `state.last_state_id` from the same GET messages response. Both values are required and must be nonempty. Messages received after that response remain unread. For reactions, omit both fields; all unread reactions are marked read.” Change request/response labels to “Read scope and boundary” / “Marked as read”. A `limit=1` example can illustrate marking all currently received messages without describing backend matching behavior.

**Behavior checked:** `core/api/v2/service/chat.go:408`–`:466` defaults empty scope to messages, accepts mentions, requires both values in those modes, and rejects either value for reactions. The long model type comment already describes this but is not emitted to Swagger.

### C3 — P2 — Separate page traversal from the order within each message page

**Source:** `core/api/v2/handler/chat.go:106`, `:112`, `:113`; related `core/api/v2/model/chat.go:40`, `:87`, `:88`.

**Exact quote:** “Every other query, including `after` together with `before`, is anchored at the newest end of the range and walks backward from `next_before`.”

**Published location:** `/paths/~1v2~1spaces~1{space_id}~1chats~1{chat_id}~1messages/get/description` (`get_chat_messages`), verbatim. `/parameters/2/description` and `/parameters/3/description` say “order id” without naming `ChatMessage.order`. `/components/schemas/ChatMessage/properties/order` and `/components/schemas/ChatMessagesResponse/properties/messages` have no descriptions supplying the missing distinction.

**Problem:** Only the forward branch says “oldest first”; the other branch says “walks backward”. A reader can reasonably infer that a normal response is newest-first, although returned messages are always ascending by `order`. That can cause reversed presentation or an incorrect choice of the newest message when marking a page read. The unfamiliar “order id” is also not explicitly connected to the returned `order` field, as distinct from message `id` or `last_state_id`.

**Direction:** Start with “Each response lists messages from oldest to newest by `order`.” Then state that `after` alone selects the next oldest page and continues with `after=next_after`, while other queries select the newest page within the bounds and continue with `before=next_before`. Explain that `after`/`before` take a returned message's `order`, and preserve any opposite bound while paging. Retain exclusive bounds, the lifetime meaning of `message_count`, and the rejection of offset pagination.

**Behavior checked:** `core/api/v2/service/chat.go:161`–`:198` chooses which end to trim and returns the appropriate boundary from the ascending result. `core/api/v2/service/chat_test.go:269`–`:273` and `:297`–`:301` explicitly assert ascending results and the distinct cursors for both directions.

### C4 — P2 — Describe delete warnings as potential attachment cleanup

**Source:** `core/api/v2/handler/chat.go:222`.

**Exact quote:** “The response names those ids in `warnings`, and a dry run reports the same list without deleting anything.”

**Published location:** `/paths/~1v2~1spaces~1{space_id}~1chats~1{chat_id}~1messages~1{message_id}/delete/description` (`delete_chat_message`), verbatim. The immediately preceding sentence identifies attachments whose only reference was the message as permanently erased.

**Problem:** In context, “those ids” and “the same list” imply a confirmed list of orphaned objects that will be erased, including in dry run. The implementation does not determine which attachments have other references before responding: it names every attachment and warns that it *may* be permanently deleted. This difference matters when a consumer presents a deletion preview.

**Suggested wording:** “Deleting a message can later permanently delete attached objects that have no other references, bypassing Bin. `warnings` lists potentially affected attachment IDs. A dry run reports the same potential effects without deleting anything. A missing message returns 404, including on a dry run.”

**Behavior checked:** `core/api/v2/service/chat.go:320` computes the same warnings for real/dry-run paths. `v2ChatDeleteWarnings` at `:606`–`:620` gathers all nonempty attachment targets and emits “may PERMANENTLY delete”; it does not check reference counts. Retain the permanent-deletion and later-cleanup warning rather than trimming it away as internal detail.

### C5 — P3 — Replace attachment “layout” with the public classification

**Source:** `core/api/v2/handler/chat.go:155`; related fields `core/api/v2/model/chat.go:58`, `:111`.

**Exact quote:** “Attachments are object ids, at most 32, and each one's kind is taken from the target's layout.”

**Published location:** `/paths/~1v2~1spaces~1{space_id}~1chats~1{chat_id}~1messages/post/description` (`add_chat_message`), verbatim. The generated `/components/schemas/AddChatMessageRequest/properties/attachments` only describes an array of strings; `/components/schemas/ChatAttachment/properties/type` is a bare string. Neither defines “layout” or the resulting kinds.

**Problem:** A caller needs to know which IDs to supply and what attachment types the API returns, not the internal property used to classify them. “Layout” does not supply either answer and makes the simple automatic classification sound like a concept the caller must understand or configure.

**Suggested wording:** “Attach up to 32 existing object IDs from this space. Images are returned as `image`, other files as `file`, and other objects as `link`. Upload a file before attaching its object ID.” Place the values in the `ChatAttachment.type` field description as well. This is separate from the inline-text issue because it explains a different input and output.

**Behavior checked:** `core/api/v2/service/chat.go:546`–`:564` resolves objects in the chat's space and applies exactly that image/file/link mapping; its missing-target error already suggests uploading first.

### C6 — P3 — Keep the stream recovery caveat without the state-ID implementation rationale

**Source:** `core/api/v2/handler/chat_stream.go:50` and `:58`.

**Exact quote:** “because none of those restamp the state id a resume is measured against.”

**Published location:** `/paths/~1v2~1spaces~1{space_id}~1chats~1{chat_id}~1messages~1stream/get/description` (`stream_chat_messages`), verbatim; the `Last-Event-ID` description is `/parameters/4/description`.

**Problem:** “Restamp” explains a backend mechanism instead of the caller's recovery steps and encourages readers to connect several different IDs used by the Chat API. The current header description says only “Resume from this chat state id”, although the actionable instruction is to return the last SSE `id` value. The limited replay guarantee is useful and must remain.

**Suggested wording:** “Reconnect with the last SSE `id` value in `Last-Event-ID`. Resuming replays only newly added messages. Re-fetch messages with GET /v2/spaces/{space_id}/chats/{chat_id}/messages to refresh edits, deletions, pins, and reactions missed while disconnected.” Header: “Last SSE `id` value received from this stream.” Keep the separate published overview's window/resync and same-node limitations when polishing that overview; this finding does not propose removing them.

**Behavior checked:** `core/api/v2/handler/chat_stream.go:96`–`:111`, `:154`–`:178` emits addition IDs and an id-only cursor frame; `core/api/v2/model/chat_event.go:75`–`:122` withholds resumable IDs for other changes. This recommendation concerns the emitted route description; the lengthy private `ChatEvent.Id` comment is not currently published.

### C7 — P3 — State the reaction toggle's actor and boolean meaning directly

**Source:** `core/api/v2/handler/chat.go:249`; related result field `core/api/v2/model/chat.go:138`.

**Exact quotes:** “`added` says which way the toggle went”; “when there is no account identity to predict with”.

**Published location:** `/paths/~1v2~1spaces~1{space_id}~1chats~1{chat_id}~1messages~1{message_id}~1reactions/post/description` (`toggle_chat_reaction`), verbatim. `/components/schemas/ChatReactionResult/properties/added` has no description.

**Problem:** The prose is conversational but never explicitly says whose reaction is toggled or what true/false means. The backend's ability to resolve an account identity is incidental; the relevant caveat is that a dry run can omit the prediction and provide a warning.

**Suggested wording:** “Adds or removes the current user's reaction. `added` is true when the reaction was added and false when removed. A dry run reports the expected result; if it cannot determine the result, `added` is omitted and `warnings` explains why.”

**Behavior checked:** `core/api/v2/service/chat.go:353`–`:383` checks the account's existing reaction, reports the inverse, and omits `added` with a warning if it cannot predict.

## Model properties

### M1 — P2: State the permission values instead of the intended implementation technique

- **Source:** `core/api/v2/model/model.go:284`
- **Exact quote:** “the compact form agents string-match on”
- **JSON pointer:** `/components/schemas/WhoamiGrant/properties/permission/description`
- **Why it matters:** This is the description of the value clients must inspect before attempting writes, but it never says what strings occur or what they allow. There is no enum or example elsewhere in this property schema. A client cannot tell whether the write-capable value is `write`, `read_write`, or `readwrite`; the description also fails to explain the `null` value returned for a legacy key. “String-match” describes an implementation choice, not this API's permission contract.
- **Behavior checked:** `core/api/util/grant.go:21` defines `read` and `readwrite`; `ApiGrant.CanWrite` at line 123 admits writes only for `readwrite`. `core/api/v2/service/whoami.go:48` initializes the legacy response with a nil permission, then lines 63–66 copy the grant permission for scoped keys.
- **Replacement direction:** Define `read` as read-only access and `readwrite` as read/write access within the granted spaces, and state that the field is `null` when `grant.scoped` is false. For example: “Access within the granted spaces: `read` or `readwrite`. Null for a legacy key with no space grant.” This is a prose finding; schema nullability changes are outside this review.

### M2 — P2: The public scope description includes a credential type that cannot reach this response

- **Source:** `core/api/v2/model/model.go:259`
- **Exact quote:** `"jsonApi" | "full" | "limited"`
- **JSON pointer:** `/components/schemas/WhoamiResponse/properties/scope/description`
- **Why it matters:** The bare internal enum gives no distinction between credential scope and the separate `grant.permission`/`grant.spaces` fields. More concretely, it presents `limited` as a possible successful whoami response even though v2 rejects those credentials before the handler. A consumer could reasonably expect whoami to be the way to inspect a limited key and interpret the resulting 403 as an unexpected failure.
- **Behavior checked:** `core/api/server/middleware.go:323` admits only `AccountAuth_JsonAPI` and `AccountAuth_Full`. `core/api/server/router.go:73` installs this gate; `core/api/v2/router.go:102` applies it to the group containing whoami at line 138. `scopeName` in `core/api/v2/service/whoami.go:156` can stringify `limited` for direct service callers, but that does not make it a reachable HTTP response. `pkg/lib/pb/model/protos/models.proto:740` distinguishes JSON API, limited WebClipper, and full local API scopes.
- **Replacement direction:** Explain that this is the credential's API scope and name the values accepted by this endpoint (`jsonApi`, `full`), with space/write permissions documented under `grant`. Do not change runtime behavior to make the present description true.

### M3 — P3: Explain all-spaces coverage without “boundary field” and “tech space” jargon

- **Source:** `core/api/v2/model/model.go:283`
- **Exact quotes:** “the boundary field of an all-spaces grant”; “the tech space excepted”; “Never infer the boundary from it”
- **JSON pointer:** `/components/schemas/WhoamiGrant/properties/all_spaces/description`
- **Why it matters:** The useful rule is present—future spaces are included and the current list is informational—but it is surrounded by abstract “boundary” terminology and an unexplained internal space type. A consumer should not need to understand Anytype's account-storage design to interpret a boolean. The description should also anchor this rule to `scoped: true`, because legacy responses return `all_spaces: false` while remaining unrestricted.
- **Behavior checked:** `core/api/v2/service/whoami.go:48` leaves `all_spaces` false for legacy keys; lines 64–95 set it and enumerate currently available spaces for an all-spaces grant. `core/api/util/grant.go:107` excludes the internal account-data space from such a grant.
- **Replacement direction:** Say that, for a scoped key, true covers all current and future user spaces; `spaces` lists currently available spaces and does not limit that access. Replace the unexplained “tech space” parenthesis with a short “Internal account data is excluded” explanation if the exception remains in this field.

### M4 — P3: Identify the key without requiring knowledge of app-link storage

- **Source:** `core/api/v2/model/model.go:269`
- **Exact quote:** “the app link's hash, which is the id the key list shows”
- **JSON pointer:** `/components/schemas/WhoamiKey/properties/id/description`
- **Why it matters:** Neither an “app link” nor “the key list” is defined by the v2 schema or a v2 key-listing operation. This sends readers looking for a hash algorithm or another resource instead of telling them that the value identifies the authenticated key. The storage provenance adds no necessary instruction for using this response.
- **Behavior checked:** `core/api/v2/service/whoami.go:40` returns the authenticated session's ID unchanged; `core/api/server/middleware.go:229` obtains it from `apiSession.KeyId`. The SHA-256 app-link representation is created internally in `core/wallet/applink.go:313`.
- **Replacement direction:** “Identifier of the authenticated API key.” Mention a management interface only if that interface is named explicitly and is useful to API consumers; do not require a client to reproduce the hash.

### M5 — P3: Describe the notice's purpose and presence condition

- **Source:** `core/api/v2/model/model.go:263`
- **Exact quote:** “the legacy sentence, verbatim printable”
- **JSON pointer:** `/components/schemas/WhoamiResponse/properties/notice/description`
- **Why it matters:** “The legacy sentence” refers to a specific piece of implementation text the reader has not seen. It does not say what the message is for or when the optional field is present. The useful part is that it is already suitable for display.
- **Behavior checked:** `core/api/v2/service/whoami.go:52` emits the notice only for unscoped `JsonAPI` keys; it is absent for scoped keys and for `Full` credentials. `core/api/util/keystatus.go:40` is user-facing guidance about replacing an unscoped key in Settings > API Keys.
- **Replacement direction:** “User-facing guidance for legacy unscoped JSON API keys; omitted for other keys. May be displayed as returned.” Avoid promising that every `key_status: legacy` response has a notice, since full-scope credentials do not.

### M6 — P3: Keep request-path examples in edit receipts, remove Go names and editor jargon

- **Source:** `core/api/v2/model/model.go:517` and `core/api/v2/model/model.go:524`
- **Exact quotes:** “CreatedBlocks maps each payload position”; “top-level run positions”; “cell run”; “CreatedViews maps each payload position that created a dataview view”; “view slot of an update_block set channel”; “reported here rather than in CreatedBlocks”
- **JSON pointers:**
  - `/components/schemas/EditResult/properties/created_blocks/description`
  - `/components/schemas/EditResult/properties/created_views/description`
- **Why it matters:** The request-path examples and the exclusion of reused IDs are valuable public behavior. “Run,” “slot,” “dataview,” and “set channel” add undocumented editor terminology, while `CreatedBlocks`/`CreatedViews` name Go fields that are not present in JSON. The final explanation of why view IDs use another field repeats information already conveyed by the schema.
- **Behavior checked:** `core/api/v2/service/edit.go:416` copies the two maps into the response. Existing tests in `core/api/v2/service/payloadids_test.go:528` cover newly created table rows/columns, a nested cell block at `ops[0].value[1]`, and a view at `ops[0].set.views[1]`; `core/api/v2/service/viewops_test.go:671` covers `insert_view` at `ops[0]`. These tests were read, not executed.
- **Replacement direction:** Describe a map from request paths to IDs of newly created blocks/views. Keep representative paths, including nested/table positions, and the rule that reused IDs are omitted. Describe `insert_view` and the `views` array in an `update_block` operation directly. Use the JSON names if a cross-reference is necessary. Do not remove nested receipt behavior merely to shorten the text.

### M7 — P3: “Identity key” obscures where a returned type/property key can be reused

- **Source:** `core/api/v2/model/model.go:364`
- **Exact quote:** “identity key (types, properties)”
- **JSON pointer:** `/components/schemas/CreateResult/properties/key/description`
- **Why it matters:** The response already has an `id`, and “identity key” introduces an undefined second identifier category. What a client needs is that this is the type or property key accepted by corresponding type/property parameters and definitions. The parenthetical names the resource classes but not that use.
- **Behavior checked:** `core/api/v2/service/schema_write.go:224` and line 820 return type/property API slugs on creation; line 607 and lines 899/933 return the addressed key on updates/deletes. This is distinct from the object ID also carried by `CreateResult`.
- **Replacement direction:** “Type or property key used by the corresponding API endpoints.” An example such as a type key or property key can help, but avoid adding internal stored-key, BSON, or slug-conversion details.

### M8 — P3: Shared mutation-result descriptions should not call every affected object “created”

- **Source:** `core/api/v2/model/model.go:363` and `core/api/v2/model/model.go:365`
- **Exact quotes:** “type key of the created object”; “etag of the created object”
- **JSON pointers:**
  - `/components/schemas/CreateResult/properties/type/description`
  - `/components/schemas/CreateResult/properties/etag/description`
- **Why it matters:** This schema also documents update and delete receipts. On object DELETE, `type` describes the existing object being archived; on type PATCH, `etag` describes the updated object version. Calling those objects “created” makes the shared response documentation misleading even though the field values are correct.
- **Behavior checked:** The generated document references `CreateResult` from object DELETE and type/property PATCH/DELETE as well as POST. `core/api/v2/service/delete.go:98` returns the deleted object's type; `core/api/v2/service/schema_write.go:607` and line 619 return the existing type ID/key and its updated ETag.
- **Replacement direction:** Use “Type key of the affected object” and “ETag of the resulting object version.” Keep conditional field presence as it is; this does not require renaming the Go schema or changing the wire shape.

### M9 — P3: Pairing challenge fields should describe the value's round trip

- **Source:** `core/api/model/auth.go:18` and `core/api/model/auth.go:22`
- **Exact quotes:** “needed to solve the challenge for api_key”; “The challenge id associated with the previously displayed code”
- **JSON pointers:**
  - `/components/schemas/CreateChallengeResponse/properties/challenge_id/description`
  - `/components/schemas/CreateApiKeyRequest/properties/challenge_id/description`
- **Why it matters:** “Solve the challenge for api_key” repeats the internal RPC metaphor instead of telling the reader to return this value with the approval code. Both comments describe the ID through a code being displayed, while the current pairing flow returns the challenge ID before the user approves and reveals the code. The response and request fields should make their direct connection clear.
- **Behavior checked:** `core/api/service/auth.go:33` returns `resp.ChallengeId`; lines 42–45 pass the submitted `challenge_id` and code together to the exchange. The emitted `/v2/auth/challenges` operation description documents the approval-before-code flow.
- **Replacement direction:** Response: “Identifier for this pairing request. Submit it with the approval code when creating an API key.” Request: “The `challenge_id` returned when the pairing request was created.” These models are shared with v1, so a later edit should keep the wording version-neutral unless both documents are deliberately updated.

## Overview, authentication, spaces, members and files

### G1 — P2: Limit the ETag claim to object PATCH; chat cursors do not provide equivalent edit protection

- Exact public quote: “A mutation takes that etag back in `If-Match`” and “They have no etag, because their order ids and `last_state_id` do that job.”
- Source: `core/api/v2/doc.go:21`.
- Public pointer: `/info/description`. Both quoted sentences occur verbatim in the generated info description shown above the operations.
- Impact: The text teaches that mutations generally accept a stale-write precondition and that chats provide the same protection through their IDs. An integration can consequently expect a conflicting edit to be rejected on a chat or another mutation that does not read `If-Match`.
- Behavior checked: `core/api/v2/handler/edit.go:50` passes the header into object PATCH. Chat edit takes only the message request (`core/api/v2/service/chat.go:258`) and submits the replacement content without a version comparison (`:292`). `last_state_id` is used with `up_to` when marking messages read (`:429`); order IDs also serve pagination, and state IDs serve stream resumption.
- Direction: Explicitly scope `If-Match` to object PATCH. Explain that it is optional, with HTTP 409 on a stale value. Say chats have no ETag precondition; describe their IDs only as pagination/read-state/resume values. Do not imply they prevent concurrent message overwrites.

### G2 — P2: Describe actual error bodies instead of promising one body and retracting it in middleware jargon

- Exact public quotes: “Every error has one shape”; “Each issue is addressed by path and names the values that would have been accepted”; “the older shared envelope”; “The shared key-scope gate uses the legacy envelope”; “Shared write rate limit; legacy error envelope”.
- Sources: `core/api/v2/doc.go:17` (also “shared authentication error envelope” at `:19`); `scripts/fix_openapi_v2.py:154`, `:158`, `:177`.
- Public pointers: `/info/description`; `/components/responses/Unauthorized/description`; `/components/responses/Forbidden/description`; `/components/responses/RateLimited/description`.
- Visibility proof: The response components are referenced from 48, 48 and 21 operations respectively. Examples are `/paths/~1v2~1auth~1whoami/get/responses/401`, `/paths/~1v2~1spaces/post/responses/403`, and `/paths/~1v2~1spaces/post/responses/429`. The generated references resolve to those descriptions, so these are operation-visible response descriptions, not dead helper strings.
- Impact: A new consumer cannot infer what “shared”, “legacy”, or “gate” means from API v2. The leading universal promise encourages a single decoder even though the paragraph and response schemas later describe two bodies. It also overstates the issue detail: an issue can have no `path` or accepted-value list.
- Behavior checked: The v2 `Error` schema requires status/code/message/issues, whereas `Issue` requires only message. Shared errors have object/status/code/message (`core/api/util/error.go`). The generated 403 response explicitly permits both `ForbiddenError` and `Error` through `anyOf`.
- Direction: State the two concrete JSON body shapes and when clients may receive them; identify fields of an issue as required or optional. Describe 401 as invalid/missing credentials, 403 as insufficient access, and 429 as too many write requests, with the applicable documented body shape. Mention non-JSON unmatched-route/unhandled-failure responses if retaining that global caveat. Remove handler-order, gate, shared, older and legacy terminology.
- Exclusion: `core/api/v2/handler/whoami.go:33-34` contains similar annotation prose, but those exact response descriptions are overwritten by the postprocessor and are not independent visibility findings.

### G3 — P2: The published 413 descriptions refer to limits that the same generation step removes

- Exact public quote: “Request body exceeds this operation's documented cap”. Related overview quote: “A document body is capped at 10 MiB, a structured body at 1 MiB.”
- Sources: `scripts/fix_openapi_v2.py:173` and its replacement at `:224-225`; `core/api/v2/doc.go:28`.
- Public pointers: `/components/responses/RequestTooLarge/description`; `/info/description`. Example operation references: `/paths/~1v2~1spaces/post/responses/413`, `/paths/~1v2~1spaces~1{space_id}/patch/responses/413`, `/paths/~1v2~1spaces~1{space_id}~1files/post/responses/413`.
- Visibility proof: Thirteen generated operations reference this component. The upload annotation's more useful “JSON request body exceeds the 1 MiB cap” (`core/api/v2/handler/create.go:397`) is absent from the generated operation because the replacement is unconditional.
- Impact: A user receiving 413 cannot find the promised per-operation number. “Document” versus “structured” does not distinguish two JSON bodies without learning the implementation's categories, and the upload endpoint has both JSON and multipart bodies with different handling.
- Behavior checked: Space create/update decode at 1 MiB (`core/api/v2/handler/space.go:21,66,105`); URL upload JSON decodes at 1 MiB (`core/api/v2/handler/create.go:423-425`). Multipart follows a separate staging path. Type update reads the 10 MiB AnyBlock body path (`create.go:194`), so assigning 1 MiB to every current shared-component reference would be inaccurate.
- Direction: Publish the actual numeric limit in each affected operation's response or request-body description, and identify the media type when needed. Preserve useful numeric annotations rather than replacing them with a circular reference. Use user-facing categories such as “AnyBlock JSON document” and “JSON URL-upload request” if a global limits section remains.

### G4 — P3: Treat space IDs as opaque values; remove CID/replication-key anatomy

- Exact public quotes: “the last six characters of the first half of its id”; “the whole <cid>.<replicationKey> id”; “two spaces whose tails collide”.
- Sources: `core/api/v2/doc.go:31-32`; `core/api/v2/handler/whoami.go:31`; `core/api/v2/handler/discovery.go:20`; `core/api/v2/handler/space.go:31,56,93`. Consolidated cross-scope occurrences, requested by root: `core/api/v2/handler/search.go:131` and `core/api/v2/model/model.go:211-215`.
- Public pointers: `/info/description`; `/paths/~1v2~1auth~1whoami/get/parameters/0/description`; `/paths/~1v2~1spaces/get/parameters/0/description`; `/paths/~1v2~1spaces/post/parameters/1/description`; `/paths/~1v2~1spaces~1{space_id}/get/parameters/1/description`; `/paths/~1v2~1spaces~1{space_id}/patch/parameters/2/description`; `/paths/~1v2~1search/post/parameters/2/description`; `/components/schemas/SpaceRow/properties/id/description`.
- Visibility proof: A recursive scan of emitted summary/description strings found precisely those locations for “first half” or “<cid>”. The model-field occurrence is included only to consolidate the other reviewer's same finding.
- Impact: CID, replication keys, halves and tails assume storage-format knowledge and invite clients to manufacture or parse IDs. The public action is simply choosing compact display references versus full persistent IDs.
- Direction: Explain `compact` as the default short space reference, with a full ID returned if it would be ambiguous. Explain `full` as the stable ID to save and reuse. State that either returned value is accepted in `space_id`, short references may become ambiguous as accessible spaces change, and ambiguous input returns HTTP 400 with candidates. Clients should reuse returned IDs without needing their construction algorithm.

### G5 — P3: Replace the key-vocabulary monologue with concrete input/output rules

- Exact public quotes: “Things are addressed by name rather than by id”; “a served body picks one”; “the forgiving fold between them”; “dueDate, due-date, "Due date" and Дата выполнения all address the property they name”.
- Sources: `core/api/v2/doc.go:12-13`.
- Public pointer: `/info/description`; the complete paragraph is emitted verbatim.
- Impact: The preceding “by name” claim conflates stable API keys and changeable display names. “Spelling”, “vocabulary”, “minted”, “frozen” and “fold” make a straightforward choice harder to follow. The example groups normalized English variants with a Russian display name without saying that a property must actually have that display name; it can read as automatic translation.
- Behavior checked: Resolution accepts keys and names and normalizes variants, but exact keys take precedence (`core/api/v2/service/keys.go:159-165`); live display names are matched against stored names (`:238-242`). The fold removes punctuation/case distinctions, not language differences (`pkg/lib/bundle/apislug.go:155-170`). The externally replaced AnyBlock module's bundled-name lookup uses bundled names and does not make the Russian example an automatic translation.
- Direction: State that responses use stable property/type keys by default and `keys=name` requests display names. Recommend stable keys for saved integrations. Explain accepted inputs with a single ordinary normalization example, then separately say current display names in any language are accepted. Preserve ambiguity and unknown-name errors, with key precedence if documenting matching details. Avoid implying that all strings matching some key/name are rejected when an exact key has priority.

### G6 — P3: Reduce and structure the 1,195-word overview; move detailed streaming rules to the stream operation

- Exact public excerpts: “The agent-oriented Anytype local API”; representative excessive detail is “the opening window is followed by one id-only frame carrying the highest such id” and “It is a warning, not a guarantee of the converse”.
- Source: `core/api/v2/doc.go:11-32`, especially `:24-26`.
- Public pointer: `/info/description`.
- Visibility proof: The generated description contains 1,195 whitespace-delimited words, 21 single newlines and zero blank paragraph separators. The entire text precedes the endpoint reference.
- Impact: Authentication, vocabulary, object editing, binary downloads, retrying, dry runs, stream event semantics, pagination, limits and ID storage all occupy the same opening description. Readers looking for the first authenticated call must process endpoint-level implementation qualifications, and duplicated rules can drift between overview and operation comments.
- Direction: Keep a concise introduction and a small, clearly grouped set of common conventions. Put the detailed event types/replay/resync semantics on the chat-stream operation, preserving the important limitations there; link or direct readers to that operation. Likewise make endpoint descriptions own the detailed limits and exceptional behavior. This is a layout/duplication finding separate from the inaccurate sentences in OV1-OV5.
- Related introductory wording: `core/api/v2/doc.go:48`, `/tags/7/description`, says “Chats store messages outside blocks”. Describe the dedicated message endpoints instead; callers do not need the storage organization.

### G7 — P3: Express whoami's three access cases directly instead of first asserting a two-case rule

- Exact public quote: “Branch on `grant.scoped`”; “True means the key reaches exactly the spaces listed”; followed by “When `grant.all_spaces` is true … `spaces` is informational only.”
- Source: `core/api/v2/handler/whoami.go:27`.
- Public pointer: `/paths/~1v2~1auth~1whoami/get/description`.
- Visibility proof: The complete annotation is the generated operation description.
- Impact: The imperative teaches `grant.scoped` as sufficient for a branch, then makes its true-case rule conditional on a second flag. Consumers using the listed spaces to build an interface need a direct account of whether that list is an access boundary or a current inventory.
- Behavior checked: `core/api/v2/service/whoami.go:48-60` returns the unscoped case; `:64-95` handles scoped all-spaces grants with a current-space enumeration; `:103-120` returns the explicit grant list otherwise.
- Direction: Present the three combinations in order: unscoped has no space restriction and an empty list; scoped plus all_spaces covers current/future spaces and the list is informational; scoped without all_spaces covers the listed spaces. Keep the useful statement that the response describes the authenticated key. Field-specific hash/scope/permission/legacy-notice prose belongs to the model review, not this finding.

### G8 — P3: Explain what image width selects

- Exact public quotes: “Images support width variants”; “Image variant width; zero selects the original”.
- Sources: `core/api/v2/handler/file_download.go:17,23,86`.
- Public pointers: `/paths/~1v2~1spaces~1{space_id}~1files~1{file_id}~1content/get/description`; the GET and HEAD `/parameters/2/description` at that path.
- Visibility proof: Both generated operations expose the width query parameter with that text.
- Impact: “Variant width” assumes familiarity with Anytype's stored thumbnails. It does not state pixels or tell a client whether it requests exact resizing or chooses an available image size.
- Behavior checked: Positive widths select a stored variant (`core/api/filecontent/content.go:121-127`): the first available width at least as large as requested, or the largest available (`core/files/image.go:85-96`). SVG uses its own processing and ignores width (`core/api/filecontent/content.go:106-118`); non-images also ignore width (`:68-87`).
- Direction: Describe a preferred image width in pixels, selecting an available size rather than guaranteeing exact dimensions; zero/omitted selects the original. A short note that the parameter applies to raster images is sufficient. The exact stored-variant algorithm need not be exposed unless that is intended as a durable contract.

## Exclusions and verification notes

- Ordinary type/package comments containing `APIV2.md`, `SPEC`, `§`, C-number constraints, codec names, RPC history and implementation rationale were excluded where they do not emit. This includes the long comments above Error, ListResponse, SearchRequestDoc, ViewObject and the chat DTOs. ChatEvent is absent from the generated component schemas.
- The `@Failure` prose at `v2/handler/validate.go:26` and `v2/handler/whoami.go:33-34` mentions shared authentication internals, but the generator replaces those descriptions with response-component references. They are source cleanup candidates, not additional published findings; G2 covers the text readers actually receive.
- The API-key prefix/regex description is retained as a useful public secret-scanner contract, with compatibility information for older credentials. It is not an internal-document leak. Standard HTTP header terminology, public schema endpoints, concise icon-download instructions and necessary destructive-operation/replay limitations are also not findings.
- No source-generation drift is claimed from files changing during the review. Current public text was rechecked. JSON and YAML prose was compared through parsers; at that check the only difference was a JSON-only optional upload `name` description. This is recorded as a verification note, not an additional comment finding.
- No implementation tests were added or run in the first round. The completed second round ran the existing Swagger prose and response-policy checks, as detailed below.

## Second-round wording proposals

Completed with five separate max-effort agents. All 36 findings have proposed documentation fixes, applied in an isolated worktree and verified against regenerated Swagger output.

- **Review patch:** [APIV2_SWAGGER_COMMENTS_PROPOSAL.patch](APIV2_SWAGGER_COMMENTS_PROPOSAL.patch). It includes source comments, the small prose changes in the document postprocessor, and regenerated JSON/YAML documents.
- **Isolated worktree:** `/private/tmp/anytype-heart-apiv2-comments-20260908`.
- **Baseline:** `a6610ac7a803a15d0c3cb69bffdbbaa58dbf8326`, a local snapshot of the primary working tree's existing changes before the proposals. The patch contains only changes after that snapshot.
- **Primary working tree:** source and generated documents remain unchanged by this task. Only the review Markdown and proposal patch were added. Source hashes still match the snapshot, and `git apply --check` confirms that the proposal patch applies cleanly to the current primary working tree. It has not been applied there.

### Coverage of proposed fixes

| Scope | Findings addressed | Concrete changes |
| --- | --- | --- |
| Object, type, property, template and schema methods | O1–O7; L4; related L2/G3/G4/G6/G8 text | Correct `object_type`; explain allowed type updates, actual PATCH selectors and request bodies, integration-name deletion rules, schema discovery, Unicode truncation and image width. Remove the ineffective collection option-creation annotation. |
| Search and lists | L1–L3, L5; search occurrence of G4 | Add usable filter/sort guidance and public schema links; explain that saved views must be selected, what omitted views mean, and how unresolved conditions affect results. Replace internal layout terminology. |
| Chat | C1–C7; streaming part of G6 and chat limits in G3 | Show formatting/mention syntax and literal escapes; give a mark-as-read recipe; distinguish page order from traversal; explain potential attachment cleanup, attachment classes and reaction booleans. Put stream recovery details beside its endpoint. |
| Model properties | M1–M9; model portions of L1/G4 | Define permission values and reachable credential scopes, key identity, notices, all-spaces access, request-path receipt maps, affected-object fields and the pairing round trip. Add filter/sort examples and date representations. |
| Overview and shared descriptions | G1–G7 | Limit If-Match guidance to object PATCH; describe both actual error bodies; publish numeric body limits; simplify keys/space references and whoami's three cases. Reduce the overview and remove storage/history explanations. |

The overview now has **568 words and nine blank paragraph separators**, down from 1,195 words without paragraph breaks. Detailed stream behavior remains documented: opening snapshots, live event names, id-only checkpoints, addition-only replay, missed-update refreshes, resync limitations, same-instance resume tokens and keepalive comments.

### Examples of the final wording

These are excerpts from the applied proposals; the patch contains every replacement in context.

| Location | Proposed wording |
| --- | --- |
| `WhoamiGrant.permission` | “The key's access within granted spaces: `read` for read-only access or `readwrite` for read/write access. Returns `null` when `scoped` is false.” |
| `WhoamiKey.id` | “Identifier of the authenticated API key.” |
| `CreateChallengeResponse.challenge_id` | “Identifier for this pairing request. Submit it with the approval code when creating an API key.” |
| `CreateResult.key` | “Type or property key used by the corresponding API paths and request fields.” |
| `CreateResult.etag` | “ETag of the resulting object version.” |
| `ChatReadRequest.scope` | “What to mark read: messages (default), mentions, or reactions. Messages and mentions require up_to and last_state_id; reactions marks all unread reactions and accepts neither field.” |
| `ChatMessagesResponse.messages` | “Requested page, always ordered from oldest to newest by each message's order.” |
| File download `width` | “Preferred raster image width in pixels. Selects an available size, so the result may not have this exact width. Zero or omitted selects the original; other file types ignore width.” |
| Object deletion | “Archives objects created by the current account through an integration whose recorded `app_name` exactly matches this API key's application name.” |
| PATCH input | “JSON body with only an ops array of 1 to 512 operations, at most 10 MiB. update_block and delete_block accept match instead of id; replace_text uses find when id is omitted.” |

### Validation

- Regenerated both documents with the repository-pinned Swagger `v2.0.0-rc4`, matching the normal package-prefix normalization and `scripts/fix_openapi_v2.py`. A temporary Go cache allowed generation in the isolated worktree without changing the user's cache or source.
- Ran the existing `TestV2DocumentProse`, `TestV2UploadRequestBodyForms`, `TestV2OpenAPIResponsePolicies` and `TestV2OpenAPIQueryPaths`: **all passed**. The existing test files were executed as focused file-based Go tests; a temporary fixture loader supplied the generated document for the prose tests. No assertions were changed, and the full application test suite was not needed for comment-only Go changes.
- Compared all emitted v2 summaries/descriptions in JSON and YAML: **482 entries in each, with zero differences**. The longest non-overview description is **378 UTF-8 bytes**, within the unchanged 400-byte guard.
- Confirmed **50 operations, 59 component schemas and 318 operation/status pairs** remain intact. All 13 operations using the shared 413 response now expose their numeric limit beside the request body: 10 MiB for type updates, 1 MiB for the other 12. The upload description identifies the JSON form specifically.
- Compared generated documents with the snapshot after excluding prose: v1 is structurally unchanged; v2 differs only by removal of the ineffective `create_missing_options` query parameter from collection creation. Request schemas, requiredness, response references and status inventory are otherwise unchanged. Shared authentication field comments intentionally update both versions' generated prose.
- Compared Go lexical tokens with comments excluded across all **16 changed Go files**: identical to the baseline. `git diff --check` passes. The postprocessor changes only emitted prose.

The isolated proposal has no unresolved findings. The original findings above remain as the review record for the primary working tree, where the proposals have not been applied.
