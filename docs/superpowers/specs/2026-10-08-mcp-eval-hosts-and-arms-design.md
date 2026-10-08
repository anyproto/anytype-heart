# MCP evaluation across hosts and surfaces — design

Status: design agreed 2026-10-08, not implemented.
Purpose: measure `/mcp/full` before optimizing it (the first optimization
in line is its 257 KB `tools/list`).

## 1. Decisions

| question | decision |
|---|---|
| hosts | Claude Code headless (`claude -p`) and Codex CLI (`codex exec`) — real hosts, so host behaviour such as deferred tool loading is part of what is measured |
| scoring | the existing scenarios (32, incl. MCP-31 `set_type`, MCP-32 discussions) with the existing LLM review pipeline |
| arms | `full` (`/mcp/full` as built: the large schemas are lookups) and `full-inline` (the same table with every lookup fetched and inlined by the guard) |
| heart | one headless heart per campaign via `cmd/apiv2eval/heartboot`, built from this branch, throwaway account |
| home | extend `scripts/mcp_eval` — it already has turn delivery, the space guard, wire logs, snapshots and review |

Rejected: porting the scenarios into `cmd/apiv2eval` (rebuilds the runner,
guard, snapshots and review); workflow-driven Claude agents (not a real
host). Out of scope: the npm bridge and the curated tiers as arms, local
models, the deterministic compact suite.

## 2. What moves into this branch

- `scripts/mcp_eval/*.py` and its tests, from the main checkout (untracked
  there), without `__pycache__`.
- `docs/evals/anytype-mcp-v2/{scenarios.json,SCENARIOS.md,HARNESS.md}` in
  their 32-scenario form from branch `go-0000-mcp-31-32-scenarios`.
- Not moved: `docs/evals/anytype-mcp-v2/artifacts` (672 MB), the
  per-campaign result documents, and that branch's `run_haiku_scenarios.py`
  / `run_all_haiku_parallel.sh`, which pass a Claude model id to the Codex
  CLI and so never drove Claude. Run output stays outside the repo, as the
  runner README already requires.

## 3. Host drivers

`run_codex.py` becomes `run_scenario.py` over a host interface:

```python
class Host:
    def start(self, prompt, mcp_config, workdir) -> TurnResult   # first turn, returns session id
    def resume(self, session_id, prompt) -> TurnResult           # later turns, same conversation
# TurnResult: final text, raw events path, usage (input/output/cache tokens), tool calls, exit status
```

- **codex**: today's `codex exec` invocation, unchanged in behaviour;
  `run_codex.py` stays as a thin alias so existing commands keep working.
- **claude**: `claude -p --output-format stream-json --verbose
  --model <id> --mcp-config <guard.json> --strict-mcp-config
  --allowedTools 'mcp__anytype__*' --permission-mode dontAsk` (or the
  equivalent that pre-approves only those tools), first turn with a fixed
  `--session-id`, later turns with `--resume <id>`. Built-in tools that
  run commands, edit files, browse or spawn agents are disallowed; the
  working directory is an empty temp dir; user settings, hooks, CLAUDE.md
  and skills are not loaded (`--bare` or the setting-sources equivalent,
  whichever the installed version honours — pinned in a smoke test).
- Both drivers record the host CLI version and model id in `run.json`, and
  usage is taken from the host's own events. A smoke test per driver runs
  one turn against a stub MCP server and asserts that only `anytype` tools
  were offered and called.

## 4. The guard, upstreams and arms

`space_guard.py` keeps its role (pins every space-scoped call to spaces the
run created, logs the wire) and gains a second upstream kind:

- **stdio** (today): launches a configured stdio server.
- **http**: Streamable HTTP to `<base>/mcp/full` with
  `Authorization: Bearer <key>`, JSON-only responses, no session header
  (the full tier is sessionless). This is also how Codex reaches
  `/mcp/full`, since the guard is a stdio server.

Arms are guard settings, not heart settings:

- `full`: `tools/list` forwarded as served — 50 tools, 68,280 bytes, the
  large schemas as `get_op_schema` / `get_schema` lookups (the npm bridge's
  shape, chosen on its 180-actor experiment: opaque 6/10 blind invalid
  writes, pointer 9/10 fetching first, typed 10/10).
- `full-inline`: when `tools/list` arrives, the guard calls `get_op_schema`
  for every op in an envelope's enum and `get_schema` for every kind an
  open body points at — through the same upstream, as evaluator traffic on
  the wire log, each fetched once — and inlines them: the envelope items
  become a `oneOf` over the ops' schemas, an open document form takes its
  kind's schema and closes, a member two alternatives spell differently
  moves into the alternatives, and definitions are namespaced then merged.
  Heart ships no switch; the lookups are what it serves.

The guard records the served `tools/list` bytes and tool count per run.

**Tool-name vocabulary.** The guard, `snapshot_run.py`, `fault_hooks.py`,
`eval_fixtures.py`, `check_query_results.py`, `summarize_runs.py`,
`review_codex.py` and three more files hardcode bridge tool names
(`API-patch-object`, `mcp__anytype__API_*`; 36 sites). They move to one
table mapping a capability to its tool name per surface (`create_space` →
`API-create-space` on the bridge, `create_space` on full), and every site
reads it. A test fails if any runner file still contains a bridge-shaped
literal outside that table. Argument names differ too (the bridge nests a
`body`; full is flat), so the guard's space-id extraction and the fault
hooks' request matching read arguments through the same table.

## 5. Campaign script

`run_campaign.py`:

1. `go build` the branch's heart and boot it with `heartboot` on a throwaway
   account; mint an API key.
2. For each host × arm × scenario × repetition: one run directory, one
   fresh conversation, one guard instance, the scenario's own new space.
   Bounded parallelism (default 4) because both hosts share one heart and
   its loopback write limiter; runs record the limiter's 429s separately
   from model errors.
3. Snapshot each run's final state through its guard.
4. Tear down heart and delete the account directory.

A campaign manifest records the heart commit, both host versions, model
ids, arm settings, served `tools/list` bytes per arm, and the scenario set
hash.

## 6. Review and reporting

The existing review pipeline scores each run unchanged in method; its
prompts read tool names through the §4 table. `summarize_runs.py` gains a
host × arm breakdown:

- review outcome per scenario dimension;
- tokens per turn and per completed scenario (input, output, cache read);
- calls, error responses by category (envelope/discriminator, locator,
  payload, semantic, guard refusal, rate limit);
- first-attempt success per tool;
- served `tools/list` bytes.

The inline arm prices the lookups: against a real heart it forwards
241,524 bytes for the same 50 tools (all 50 inlined schemas compile, and
every op and document example the service serves validates in place). It
is expected to remove the envelope and payload-shape errors a skipped
lookup causes, and nothing else; the question is whether that is worth
3.5 times the bytes.

## 7. Tests

- Existing `test_*.py` stay green.
- Host driver smoke tests (§3) against a stub MCP server, skipped when the
  CLI is not installed.
- Guard: HTTP upstream against a stub HTTP MCP server (bearer sent, no
  session header, JSON response), inline rewrite (only the six lookup tools
  change; dropped defs are exactly the unreferenced ones), space pinning
  with full-tier flat arguments.
- Vocabulary table: no bridge-shaped literal outside it.
- One end-to-end dry campaign: one scenario, one host, one arm, against a
  real headless heart, run before any real campaign.

## 8. Risks

- **Review drift.** The review pipeline was calibrated on bridge
  transcripts; a reviewer that has seen only `API-*` names may misread a
  full-tier transcript. Mitigation: the §4 table in prompts, and a manual
  spot check of the first campaign's reviews.
- **Host change.** Host CLIs update often and change flags and event
  shapes; versions are pinned in the manifest and the smoke tests catch
  drift.
- **Cost.** 32 scenarios × 2 hosts × 2 arms × repetitions, each multi-turn,
  plus review. The first campaign should be one repetition on a subset
  chosen to exercise edits (`patch_object`, `update_type`).
