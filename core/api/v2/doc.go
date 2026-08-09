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
//	@description					The agent-oriented Anytype local API. Objects are AnyBlock JSON documents (pkg/lib/anyblockjson/SPEC.md) rather than block trees, so one read returns a whole editable document and one PATCH edits it.
//	@description					One vocabulary throughout (C2): camelCase, property keys, option names, and `type` as a type key — never an id, never snake_case. Responses are compact JSON (C3); object refs are always full inline, and the default object read relabels machine-minted block ids to short doc-local suffixes — `?ids=full` returns the export shape with full ids everywhere, the shape that PUTs back (C4); list and search rows carry id, name, type and the properties you asked for, and never embed type objects (C5).
//	@description					Errors share one shape (C6): {status, code, message, issues:[{path, message, hint}]}, path-addressed and naming the allowed values, so a failed call tells an agent how to repair it. Reads never fail on unknown content — anything a representation cannot express is reported in `warnings` (C11).
//	@description					Every object read returns an `etag` derived from the tree heads plus an ETag header; mutations take `If-Match` and are advisory by default (C7). Every mutation honours `Idempotency-Key` (C8) and `?dry_run=true` (C9). Every list surface is paginated with limit=25 by default and steers you when it truncates (C10).
//	@description					Schemas are discoverable at runtime and strict-mode-compatible for constrained decoding (C12, C13): GET /v2/schemas lists the kinds, GET /v2/schemas/{kind} returns one, and GET /v2/schemas/ops/{op} returns the schema of a single PATCH op.
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
