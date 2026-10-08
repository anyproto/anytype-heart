# Codex-backed Anytype evaluation

The runner uses the installed Codex CLI and existing `anytype` MCP connection. It sends one user turn at a time, resumes the same thread by its exact ID, and saves both Codex events and the raw MCP wire transcript. Default model: **gpt-5.6-luna**, medium reasoning.

```sh
python3 scripts/mcp_eval/run_codex.py \
  --scenario MCP-01 \
  --model gpt-5.6-luna \
  --output /private/tmp/anytype-mcp-eval-runs
```

Use multiple scenario IDs or `--scenario all`. Each scenario gets its own thread, run directory, empty working directory, and newly created Anytype space. `--max-turns` is for setup diagnostics. The machine needs an authenticated Codex CLI and a working stdio MCP server named `anytype` (or pass `--upstream`). No new model-provider integration is required.

## Existing-space protection

The model connects only to `space_guard.py`, which launches the already-configured MCP server privately. The guard rejects every space-scoped call unless its `space_id` matches a full or compact ID returned by a successful space creation in that run. It does not learn ownership from a name, a search hit, or a model claim. The first space must use the assigned unique name prefix; additional space creations are blocked. Scope survives Codex process restarts.

Space creation requests full IDs, and accepted compact IDs are forwarded as their full owned ID. Both original and forwarded arguments are recorded. Global discovery/search reads remain available; they can see other spaces, but cannot grant write access to them. The global write-like `validate` tool is structural validation and does not save content. Unknown tools are blocked.

The September 16 baseline wrapper said “use only that space,” which could discourage the global search requested in MCP-25. The current wrapper explicitly allows global discovery/search while keeping all space-scoped calls restricted. Original baseline prompts and traces are preserved; this clarification has not been re-evaluated.

Only this guarded connection is pre-authorized for unattended MCP calls. Shell/execution tools, browsers, computer use, other connectors/plugins, image generation, subagents, and host skill discovery are disabled for the evaluated model. The empty working directory uses a read-only filesystem sandbox. The runner does not modify the user's Codex configuration or expose upstream credentials to the model. Existing exec policy rules remain enabled.

The upstream API remains responsible for verifying that object references belong to the supplied space. The guard is a scope restriction, not a replacement for server authorization. Its tests cover rejection of existing-space IDs before and after creation, exact ID matching, scope persistence, and unknown tools.

```sh
python3 -m unittest discover -s scripts/mcp_eval -p 'test_*.py' -v
```

## Outputs

Each run directory contains:

- `run.json`: model, scenario, session ID, owned spaces, per-turn runtime/usage, tool counts, and execution status.
- `scope.json`: the evaluator-owned space allowlist.
- `mcp-wire.jsonl`: complete model/upstream MCP requests and replies, including guard refusals.
- `turn-NN.prompt.txt`, `.events.jsonl`, `.stderr.log`, `.final.txt`: exact per-turn input and output.
- `tool-calls.jsonl` and `transcript.md`: extracted tool records and a readable review transcript.

Keep run directories out of version control: responses may contain account identifiers or application content. Credentials are not intentionally logged. Raw `turn.completed.usage` is retained verbatim; do not assume resumed-session counters are incremental without checking the installed CLI's behavior.

`conversation_completed_pending_review` means all scripted turns were delivered and returned, **not** that the model fulfilled them. Review final state and the checks in the scenario. Infrastructure failures such as a missing required MCP server or MCP approval misconfiguration stop the run.

After a conversation finishes, collect independent reads through its existing space guard:

```sh
python3 scripts/mcp_eval/snapshot_run.py /private/tmp/anytype-mcp-eval-runs/RUN_DIRECTORY
```

This creates `final-state.json` with the space, successfully created types and their object inventories, authored objects/templates, collection membership, and query results both without a view and for each stored view. Reviewer calls use a separate wire trace and do not count toward model tool usage. Archived objects can return expected errors. These snapshots are evidence, not automatic pass verdicts; chat and file-byte verification still need their own checks.

Generate offline trace summaries and measured tool coverage:

```sh
python3 scripts/mcp_eval/summarize_runs.py \
  --runs /private/tmp/anytype-mcp-eval-runs \
  --output /private/tmp/anytype-mcp-eval-reports
```

The summaries audit forwarded space IDs, separate infrastructure and guard errors, preserve explicit reviewer outcomes from `reviewer-result.json`, and report attempted versus successful tool coverage. They count failures from Codex's item status as well as error payloads: the CLI can omit MCP `isError` while retaining `status:failed`.

Check final query rows against the fixed expectations derived from each scenario's requested edits:

```sh
python3 scripts/mcp_eval/check_query_results.py \
  --runs /private/tmp/anytype-mcp-eval-runs \
  --output /private/tmp/anytype-mcp-query-checks
```

This checks the named view when specified, otherwise the first stored view. It ignores archived query attempts, combines complete pages, checks multiplicity and requested ordering (including tied groups), and distinguishes missing evidence from wrong rows. It does not grade the whole scenario. Expectations are evaluator-only and must be reviewed against the final user request rather than inferred from a model's output.

Without `--with-hooks`, runs retain the original baseline behavior: hooks are `not_exercised` and the foreign-provenance turn in MCP-28 is skipped explicitly. With that flag, controlled response faults run for MCP-14/27/29 and an independent-client edit runs for MCP-18. MCP-28 pauses for a verified desktop-created note; MCP-30 pauses for real second-member receipts. Missing peers are never simulated by editing author fields. No scenario receives a full end-to-end pass solely from this runner's exit status.

The evaluated model has no general file inspection tool. In MCP-22 it can use MCP upload/download paths, but byte hashes must be verified by the reviewer; inability to inspect bytes locally is a harness limitation. Use `--snapshot-after-turn` for independent reads after every completed turn and a final snapshot. Captures include actual upstream creation receipts hidden by a lost-response fault and paginated chat reads. The original September 16 baseline had only final snapshots.

The guard intentionally changes the transport context from the original specification: new spaces are isolated within the connected account rather than restoring a disposable account snapshot. Record that shared-account profile when comparing results.

Codex's [non-interactive execution documentation](https://learn.chatgpt.com/docs/non-interactive-mode) describes JSONL events and session resumption; the [configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference) documents the per-server MCP approval setting.

## Independent offline Astra judgments

`review_codex.py` uses **gpt-6-astra**, **high** reasoning to judge each captured run in a fresh Codex session. It sends local evidence to the authenticated Codex model service; “offline” means the judge cannot access Anytype or perform live verification. The judge has an empty read-only workspace, no MCP connections, disabled tools/plugins/hooks/host skills, and `--ignore-user-config`. The script never modifies execution evidence or Anytype state.

Prepare inspectable input without starting a model request:

```sh
python3 scripts/mcp_eval/review_codex.py \
  --runs /private/tmp/anytype-mcp-eval-runs \
  --output /private/tmp/anytype-astra-reviews --prepare-only
```

Run judgments by repeating that command without `--prepare-only`. Explicit run directory paths can replace `--runs`. All discovered runs are reviewed, so point it at the intended benchmark batch rather than diagnostic launches. To judge all 90 executions, supply the directory holding the 30 scenarios × 3 runs; the script does not manufacture missing runs. It processes jobs sequentially and continues after a failed judgment, returning nonzero if any job failed.

Inputs include exact executed user prompts, complete completed tool arguments/results and assistant messages, runtime errors, raw independent final reads, `scope.json`, and relevant captured tool/schema declarations. All content blocks, warnings, issue details, and request metadata survive. Turn-event start/update duplicates and identical transport responses are omitted from the prompt deterministically; wire differences between original/forwarded arguments and distinct server/model replies remain. Matching ignores Codex's null `structured_content` transport field. Incomplete tool events remain. Repeated, identical snapshot reads use explicit source/line aliases: the complete JSON value (tool, arguments, result, and metadata) must match, while each snapshot's timestamp and turn remain visible. Changed values never alias. Original alias line ranges remain valid citations, and all raw source text is retained in `evidence.json`. Every selected source is stored with original line numbers and SHA-256 hashes; original files remain untouched locally. Non-owned global discovery/search record strings and `auth_whoami.key` metadata are replaced with consistent placeholders across replies, wire copies, prompts, and assistant echoes. Counts and grant/scope capabilities remain. The audit records original and transformed hashes and each replacement’s category and source-value hash. No selected record is truncated: the explicit `--max-input-bytes` limit fails before launch, and model context overflow is a failed judge job, not a scenario failure.

Additional raw evidence is picked up from `hook-state.json`, `hook-config.json`, `fixture-evidence.json`, `chat-verification.json`, `file-byte-verification.json`, `snapshots/`, and `evidence/`. These directories must contain raw UTF-8 evidence, not previous assessments. Inspect the prepared prompt before a first launch with a new evidence format: the privacy transformation recognizes the captured discovery/auth response shapes, not arbitrary secrets embedded in free text. `reviewer-snapshot.json` is included for independent reads and documented evaluator cleanup. Previous `reviewer-result.json`, findings, and derived summaries are excluded; `run.json` uses an explicit metadata projection to avoid importing embedded prior verdicts. The executed prompts take precedence over newer scenario wording.

For the three-repetition campaign, `prepare_review_inputs.py` selects only complete primary runs from the explicit 90-slot matrix and uses the frozen specification under `artifacts/luna-three-runs/spec/`. It captures missing MCP-22 byte evidence before freezing that run's input. Preparation is strictly local and cannot launch a judge:

```sh
python3 scripts/mcp_eval/prepare_review_inputs.py
python3 scripts/mcp_eval/audit_campaign.py
```

`review_token_preflight.py` uses a separately installed `tiktoken` and cached standard vocabularies to estimate input size. These are conservative estimates, not a claim about Astra's private tokenizer. The current defaults reflect the installed model catalog's 272,000-token window and reserve room for reasoning/output and runtime instructions. A byte limit alone does not prove that a prompt fits. Preserve old prepared inputs in a separate archive if the representation changes; never silently replace inputs of an active or completed judgment.

`run_judgments.py --check-only` validates prepared primary runs against their token-preflight hashes without launching a model. The dispatcher can then run up to four independent judgments concurrently, retaining per-run locks and refusing automatic retries. This campaign's transcript export was rejected by automatic approval review, so actual dispatch also requires `astra-review-authorization.json` with `status: approved`, recorded only after the requested explicit user confirmation. The presence of a prepared prompt is not approval. No transcript judgment has been launched as part of local preparation.

Each `runs/RUN_DIRECTORY/` output contains the exact `judge-prompt.txt`, `evidence.json`, `result.schema.json`, raw `judge.events.jsonl`, `judge.stderr.log`, `judge-output.json`, validated `reviewer-result.json`, `model-metadata.json`, and `job.json`. Metadata separates requested model/reasoning from observed CLI rollout metadata; unavailable observations remain null. Input hashes, an exclusive lock, and pending/running/complete/failed states prevent concurrent duplicate launches and permit reuse of exact completed inputs. Changed inputs require a new output directory. Do not retry a job whose original process is still running.

Successfully completed Astra responses that fail structural JSON, schema, citation, or consistency validation may receive at most two correction turns in the same original thread. The original response and frozen inputs remain unchanged; `repairs/01/` and `repairs/02/` retain correction prompts, responses, events, observed model metadata, and validation reports. Citation-only corrections must preserve all non-evidence fields exactly. `--max-repairs 0` disables correction. After inspecting an older failed job, `--retry-failed` can invoke this same bounded correction path; it does not rerun the original judgment. Runtime, tool-boundary, model, thread, or frozen-hash mismatches are refused and require investigation.

The strict [result schema](review-result.schema.json) requires evidence references, every scenario check, five scoring dimensions, explicit integrity failures, additional user requirements, and separate model/API/injected-fault/fixture/evidence findings. Local validation rejects unknown keys, invalid scores, nonexistent source lines, missing/duplicate checks, and contradictory full-pass claims. Successful JSON validation verifies structure and citation locations; it does not prove the model interpreted evidence correctly.

Validated results are grouped in `scenarios/MCP-NN.json`, retaining each run's complete findings. `FINDINGS.md` mechanically synthesizes those independent Astra judgments; it is not an additional model vote. `summary.json` reports reviewed versus expected counts (90 by default), and `errors.json` reports job failures. Regenerate the aggregate without any model calls:

```sh
python3 scripts/mcp_eval/review_codex.py \
  --output /private/tmp/anytype-astra-reviews --aggregate-only
python3 -m unittest discover -s scripts/mcp_eval -p 'test_review_codex.py' -v
```

For the campaign benchmark, bind aggregation to the explicit primary-run matrix produced by `campaign_status.py`:

```sh
python3 scripts/mcp_eval/review_codex.py \
  --output /private/tmp/anytype-astra-reviews --aggregate-only \
  --matrix docs/evals/anytype-mcp-v2/artifacts/luna-three-runs/matrix.json
```

Matrix-bound aggregation accepts only completed, valid judgments whose run ID, scenario, and original run directory match a primary matrix entry. It rejects duplicate replicate slots, mismatched or diagnostic runs, and reused primary identities. Per-run JSON entries and Markdown retain the matrix's explicit replicate number, including the baseline mapping. Per-scenario and overall coverage require exactly replicas 1, 2, and 3; three arbitrary judgment files do not establish coverage. The summary records the matrix path and SHA-256. Without `--matrix`, the existing generic count-based aggregation remains available.

`audit_judgments.py` independently checks completed jobs without contacting a model or Anytype. It verifies frozen inputs, embedded sources and original source-file hashes, schema/citations, accepted output, observed model/reasoning and thread identities, tool boundaries, and same-thread correction provenance. Active/pending jobs are reported as skipped. Missing or changed original files are explicit integrity errors. By default it writes the campaign's `judgment-audit.json`:

```sh
python3 scripts/mcp_eval/audit_judgments.py
```

Reporting may attach `evaluator-corrections.json` from beside the matrix or review output. Entries must match both the scenario and a reviewed run ID; their authority, policy, source hashes, and addendum hash remain visible. This does not rewrite raw judgments, grades, pass flags, or coverage. The current campaign uses this addendum because its frozen reference catalogue omitted `update_type.expected_etag` and `update_type.dry_run`, although all actor-visible manifests exposed them. Future campaigns should compare the reference catalogue with runtime manifests before freezing judge inputs. Do not regenerate this campaign's active or completed inputs to hide that discrepancy.
