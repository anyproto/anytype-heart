// Package apiv2 registers and serves the Anytype local API v2.
//
// doc.go carries nothing but the OpenAPI general-info block for the v2
// document. `make openapi` runs swag twice — once over core/api with
// core/api/v2 excluded (the v1 document, general info in core/api/service.go)
// and once over core/api/v2 with this file as -g. Keeping the two blocks in
// two packages is why a v2 reader never scrolls past a v1 endpoint.
//
//	@title							Anytype API v2
//	@version						2025-11-08
//	@description					The agent-oriented Anytype local API. An object is one AnyBlock JSON document rather than a tree of blocks, so a single GET returns a whole editable document and a single PATCH edits it.
//	@description					Everything this API names for itself is snake_case: path and query parameters, request and response fields, and the names of the PATCH ops. Things are addressed by name rather than by id: property keys, option names, and `type` as a type key. Inside an object's `blocks` and `properties` you are reading the AnyBlock format's own vocabulary, which passes through unchanged.
//	@description					Responses are compact. A list or search row carries id, name, type and the properties you asked for, and never embeds a type object. Object references are always full and inline. An object read relabels machine-minted block ids to short document-local suffixes; `?ids=full` returns the export shape, with full ids everywhere, which is the shape to store and the shape to clone from.
//	@description					Wherever a block or a view is addressed by id, a full id or a unique suffix of one is accepted. That is what lets a document read back in the compact shape be edited exactly as it came back. A suffix that matches several elements is refused, and the refusal lists the candidates.
//	@description					A read never fails on content it cannot represent. Whatever a representation cannot express is reported in `warnings` beside the result.
//	@description					Every error has one shape: {status, code, message, issues:[{path, message, hint}]}. Each issue is addressed by path and names the values that would have been accepted, so a failed call tells you how to repair it.
//	@description					Authentication is a bearer token in the Authorization header. It is never read from a query or body parameter. An unknown, revoked or expired key is a 401.
//	@description					An object read returns an `etag` in the body and an ETag header. A mutation takes that etag back in `If-Match`, where it is advisory: without the header the last write wins, and a stale one is a 409 carrying the current etag. Chats are the exception. They have no etag, because their order ids and `last_state_id` do that job.
//	@description					Every mutation accepts an `Idempotency-Key` header. The same key with the same body replays the stored response instead of repeating the write. Search is a read carried by POST and takes no key.
//	@description					Every mutation accepts `?dry_run=true`. It validates the request, reports what would have happened and writes nothing, answering 200 where the real call would answer 201. Where a dry run cannot tell the whole truth, the operation says so.
//	@description					Every list is paginated with `?offset=` and `?limit=`, 25 rows by default. The response carries `total`, `has_more` and, when it truncated, a hint for narrowing the request. Chat messages page by order-id cursor instead.
//	@description					Request bodies bind strictly: an unknown field is a 400 naming the field, never a value silently dropped. A document body is capped at 10 MiB, a structured body at 1 MiB.
//	@description					Deleting an object, a type or a property archives it: it moves to Bin, and the Anytype app can restore it. Deleting a chat message is not an archive, and neither is the attachment cleanup that can follow it.
//	@description					Schemas are discoverable at runtime, and strict enough to decode against: GET /v2/schemas lists the kinds, GET /v2/schemas/{kind} returns one, and GET /v2/schemas/ops/{op} returns the schema of a single edit op.
//	@description					A space is served by a short reference: the last six characters of the first half of its id. Every route that takes a space accepts either that short reference or the full `<cid>.<replicationKey>` id. Resolution tries an exact id first, then a unique suffix, among the spaces the key can see. An ambiguous reference is a 400 listing the candidates, and two spaces whose tails collide are both served in full.
//	@description					A short reference is an addressing convenience, not a stable identifier. It is unique only against the spaces the key can currently see, so joining a space whose tail collides retires it. `?ids=full` spells every space id in the response out in full. Use it whenever a reference will be stored outside this API: a config file, a script, a log line, another system.
//	@tag.name						Auth
//	@tag.description				What the calling key may do. Ask this before discovering the limits through 403s.
//	@tag.name						Spaces
//	@tag.description				The containers everything else lives in. Nearly every other route is scoped to one.
//	@tag.name						Objects
//	@tag.description				Read and write whole AnyBlock documents: one GET returns an editable document, one PATCH edits it.
//	@tag.name						Search
//	@tag.description				Find objects by query, filter and sort, within one space or across all of them.
//	@tag.name						Types
//	@tag.description				An object's shape: the properties it recommends and the views it opens with.
//	@tag.name						Properties
//	@tag.description				The typed key-value fields objects carry, and the option vocabularies select fields draw from.
//	@tag.name						Lists
//	@tag.description				Sets (a live query over a type) and collections (a hand-curated list), with their views.
//	@tag.name						Chat
//	@tag.description				Messages, reactions and read state. Chats store messages outside blocks, paged by order-id cursors.
//	@tag.name						Members
//	@tag.description				Who is in a space, and which of them you are.
//	@tag.name						Files
//	@tag.description				Upload bytes and get the id that file blocks and chat attachments reference.
//	@tag.name						Templates
//	@tag.description				Starting documents for a type.
//	@tag.name						Schemas
//	@tag.description				The format itself: what a valid document looks like, what each PATCH op accepts, and a validator to check one against them. Read these before writing.
//	@termsOfService					https://anytype.io/terms_of_use
//	@contact.name					Anytype Support
//	@contact.url					https://anytype.io/contact
//	@contact.email					support@anytype.io
//	@license.name					Any Source Available License 1.0
//	@license.url					https://github.com/anyproto/anytype-api/blob/main/LICENSE.md
//	@host							http://127.0.0.1:31009
//	@securitydefinitions.bearerauth	BearerAuth
//	@externalDocs.description		OpenAPI
//	@externalDocs.url				https://swagger.io/resources/open-api/
//
// The version above is deliberately the same date as v1's: it is the value of
// the `Anytype-Version` response header (server.ApiVersion), which one gin
// engine sets for both route groups (C1). Bump all three together or none.
// (Nothing below the annotation block may start a line with an at-sign —
// swag reads every comment group in this file and would take it as an
// attribute.)
package apiv2
