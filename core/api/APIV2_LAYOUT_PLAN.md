# API v2 — package split and two OpenAPI documents

Status: plan · 2026-08-06 · GO-7383 · companion to `APIV2.md` (v0.4) and
`APIV2_SURFACES.md` (v0.2).

Two changes, done together because they are entangled: move v2 into its own
package tree, and generate one OpenAPI document per API version instead of one
mixed document. The second is the reason the first is worth doing now — the
"clean documentation for agents and users" goal (`APIV2_SURFACES.md` §11) is
not reachable while a v2 reader has to scroll past v1 endpoints, and the
schema-name collision that blocks de-prefixing v2's models only disappears
once the documents are separate.

## 0. Why now, and why not just tidy later

**The invariant is the point, not the tidiness.** v2 already shares *nothing*
with v1 at the type level: `V2Service` is its own struct with its own
dependencies, and no `v2_*.go` service file references the v1 `*Service` or
its cached getters. That rule lives only in prose today — same package, so
nothing stops the next file from reaching across. A separate package makes it
a compile error. This is the pattern that has worked repeatedly on this
project (the route conformance test, the one-Go-table tool pin, the GBNF
acceptance suite): rules that are only written down decay; rules the build
enforces do not.

**Scale justifies it.** v2 is ~10k LOC across 31 non-test files.
`core/api/service/` is 19 v2 files interleaved with 19 v1 files.

**Before Phase 8, not after.** Phase 8 adds auth, file download, the chat
stream, the tag/template tails and the conformance test. Landing those in the
new layout costs nothing; moving them afterwards means doing the move twice.

**Not while a workflow runs.** This touches nearly every v2 file. Any agent
holding a stale path will conflict. The tree must be clean and no workflow
in flight.

## 1. Target layout

```
core/api/
  core/            apicore ports              — SHARED, unchanged
  util/            error helpers              — SHARED, unchanged
  pagination/      offset/limit plumbing      — SHARED, unchanged
  server/          gin engine, auth middleware, route registration — SHARED
  docs/
    v1/            generated: docs.go, openapi.{json,yaml}
    v2/            generated: docs.go, openapi.{json,yaml}
  handler/  service/  model/                  — v1, unchanged
  v2/
    handler/       from core/api/handler/v2_*.go
    service/       from core/api/service/v2_*.go
    model/         from the V2* types in core/api/model/v2.go
    doc.go         the v2 swagger general-info block
    router.go      v2 route registration, called by server/
  wrapper/         the task-tool wrapper      — already separate, unchanged
  eval/            harness                    — unchanged
```

What deliberately stays shared: the apicore ports (both versions consume the
same middleware surface), `util`/`pagination` (no version semantics), and
`server` — one gin engine and one `ensureAuthenticated` serve both versions,
and splitting that would mean two auth paths, which is the opposite of the
goal.

## 2. Step 1 — the mechanical move (one commit, zero behavior change)

1. `git mv` the 31 non-test files + their tests into `core/api/v2/{handler,
   service,model}`; drop the now-redundant `v2_` filename prefix
   (`v2_search.go` → `search.go`).
2. Rename packages; fix imports. v2 packages: `v2handler`, `v2service`,
   `v2model` (import aliases keep call sites readable).
3. Move v2 route registration out of `server/router.go` into
   `core/api/v2/router.go`, exporting one `RegisterRoutes(engine, deps)` that
   `server` calls. The shared middleware stack stays in `server`.
4. **Do not rename any exported type in this commit.** Type de-prefixing is
   step 5; keeping it separate is what makes this diff reviewable as a pure
   move.

**Verification.** `go build ./...`, the full suites, gofmt, and
`git diff --stat -M` showing renames rather than delete+add. The existing
route-table test must pass untouched — it walks `engine.Routes()`, so it
proves the move did not change a single registered path.

## 3. Step 2 — a general-info block per document

v1 keeps the existing block in `core/api/service.go` (`@title Anytype API`,
`@version 2025-11-08`, `@host`, `@securitydefinitions.bearerauth`).

v2 gets its own in `core/api/v2/doc.go` — same contact/license/host, its own
`@title` and `@version`, and a `@description` that says what v2 *is* (the
agent-oriented API, the C1-C13 conventions, where the discovery endpoints
live) rather than repeating v1's sentence.

No `@BasePath` juggling is needed: all 44 v2 `@Router` annotations already
carry absolute `/v2/...` paths, and v1's carry `/v1/...`.

## 4. Step 3 — two generated document sets

`makefiles/tools.mk` grows a second invocation. Sketch, to be verified by an
actual run:

```make
openapi: setup-swag
	@deps/swag init --v3.1 -q -d core/api -g service.go \
		--exclude core/api/v2 --instanceName v1 -o $(OPENAPI_DOCS_DIR)/v1
	@deps/swag init --v3.1 -q -d core/api/v2,core/api/util,core/api/pagination \
		-g doc.go --instanceName v2 -o $(OPENAPI_DOCS_DIR)/v2
	# then the per-document prefix-strip pass (jq + sed), once per output dir
```

Notes that will bite if forgotten:

- `-d` is comma-separated and **the general-info file must live in the first
  directory** — hence `core/api/v2` first for the v2 doc.
- The strip rules (`apimodel.` → ``, `pagination.` → ``, `util.` → ``) are
  per-document and must be extended with the new v2 model package name.
  A missed rule leaves package prefixes in schema names — visible, not
  silent.
- Shared packages get parsed into both documents. That is intended: each
  document must stand alone.
- Fallback if the package split were ever abandoned: `swag --tags` filters by
  tag, including negation (`!v2`). Directory exclusion is cleaner and is what
  this plan uses.

## 5. Step 4 — serve both documents

`server/router.go` currently serves `/docs/openapi.yaml` and
`/docs/openapi.json` from the single generated package.

- Add `/v1/docs/openapi.{yaml,json}` and `/v2/docs/openapi.{yaml,json}`.
- **Keep `/docs/*` serving v1 unchanged.** It is the path
  developers.anytype.io and existing integrations use; silently repointing it
  at v2 would break them, and repointing it at *nothing* is worse.
- Both `docs/v1` and `docs/v2` packages get blank-imported so their templates
  register under their instance names.

## 6. Step 5 — de-prefix the v2 model types (the payoff, and optional)

Once the documents are separate, `V2SearchRequest` → `SearchRequest`,
`V2ObjectRow` → `ObjectRow`, and so on: the collision that forced the `V2`
prefixes only existed inside one shared components map. Mechanical rename,
own commit, and the v2 document's schema names come out clean — which is what
an agent or a human reads.

This step is genuinely optional and can be deferred; steps 1-4 stand on their
own. But it is cheapest immediately after the move, before Phase 8 adds more
`V2*` names to rename later.

## 7. Verification gate

1. `go build ./...`, `go test ./core/api/... ./pkg/lib/anyblockjson/...
   ./cmd/anytype/...`, gofmt clean, tree clean.
2. The route-table test passes unchanged (proves no path moved).
3. **`make openapi`, run by a human** — agents on this project are instructed
   not to run it. Then the decisive check: **the regenerated v1 document must
   be identical to today's, modulo the instance name and output path.** A
   clean diff is the proof that splitting the packages did not alter v1's
   published contract. Any real difference is a bug in the move, not a
   cosmetic artifact, and should be treated that way.
4. The v2 document contains every registered `/v2` route. Worth pinning with
   the same route-walking pattern the conformance test uses, so a new route
   without annotations fails CI instead of quietly missing from the docs.

## 8. Risks

| Risk | Mitigation |
|---|---|
| Huge diff obscures a real change | Pure-move commit with no renames; verify with `-M` and the unchanged route test |
| v1 document silently changes | The byte-diff gate in §7.3 — this is the whole reason that gate exists |
| Strip rules missed for the new package | Visible in schema names on first generation; check both documents |
| Import cycles (`server` → `v2` → `server`) | v2 exposes `RegisterRoutes`; dependencies point one way, server → v2 |
| Concurrent agent work conflicts | Do not run during a workflow; tree clean before starting |

## 9. Sequencing

1. This plan (steps 1-5) — before Phase 8.
2. Phase 8 — lands directly in `core/api/v2/`, and its conformance test gains
   the doc-coverage assertion from §7.4.
3. The auth work (`ApiKeyScopingResearch.md`) is independent of the layout and
   can proceed on its own schedule.
