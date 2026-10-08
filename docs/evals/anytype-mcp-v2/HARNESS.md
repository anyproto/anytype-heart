# Running and reviewing the conversations

This document specifies the complete evaluation behavior. The [Codex runner](../../../scripts/mcp_eval/README.md) implements staged execution, space restrictions, trace capture, per-turn snapshots, the controlled faults for MCP-14/27/29, and an independent-client edit for MCP-18. MCP-28 and MCP-30 pause for real desktop/peer fixtures; author properties cannot substitute for provenance or another member. Independent Astra judgments use captured evidence and do not mutate Anytype. The model gets the connected tools and the `user` string of one turn at a time from [scenarios.json](scenarios.json). All other fields, future turns, oracles, and source notes remain evaluator-only.

## Baseline setup

1. Start a fresh conversation and a disposable/restorable Anytype profile for each scenario and repetition. Use the same model configuration, tool descriptions, and seed data when comparing models. Begin with a key that can create spaces and write to them. Record the actual grant privately.
2. Substitute a unique `{{run}}`, for example `eval-smallA-r1-08`. Use the run ID in the space name and logs only. Keep type display names readable; resolve key collisions with distinct API keys, without changing bundled types. Do not substitute fabricated object/property/type IDs.
3. For MCP-22, substitute `{{fixture_dir}}` with an absolute directory containing the supplied [logo.png](fixtures/logo.png) and [usage.txt](fixtures/usage.txt). Files and downloaded copies may be inspected locally. All Anytype operations must use the MCP tools; do not enable direct REST, repository source, browser, or CLI bypasses for the model.
4. Deliver one user turn, let the model complete it, and snapshot the resulting state before sending the next. A turn may contain many tool calls. Preserve the whole dialogue so later edits exercise memory of the newly built space.
5. Seed no hidden helper objects unless a scenario explicitly needs them. For clarification of a requested reference, answer from the supplied record table. For example, the two Alex Morgan people are distinguished by Birth year. Do not reveal implementation hints unless the user turn contains them. Log evaluator clarification replies verbatim.
6. Observe indexing separately from persistence. After acknowledged writes, the evaluator may poll until a query/search reflects the new state, up to a configured bound such as 15 seconds. Retain observed delays; do not turn eventual indexing into an unbounded retry loop or mislabel it as model failure.
7. Tear down by restoring the profile/account snapshot. No exposed tool deletes a space. Deleting objects through the tested model cannot reliably clean up imported or foreign-provenance objects, and it does not remove all schema/space state.

Initial account-provided objects/types/properties are background state. Checks such as “2 Projects and 4 Actions” count the objects authored for the custom run types, not every built-in or system object in the space. Property values and references are compared semantically; canonical slug spellings, UTC normalization, and block ID shortening need not match the prompt's human labels byte-for-byte.

Each scenario must create its own space and schema before populating it. If setup fails, preserve that result as a completion failure or capability block; do not silently create the missing model-owned state to allow the rest of the run to appear successful. You may fork a separately labeled diagnostic continuation from repaired fixtures, but it is not the original end-to-end score.

## Controlled hooks

Run an ordinary baseline first, then a fault-enabled repetition. Mark every hook as `armed`, `triggered`, `not_triggered`, or `fixture_unavailable`. Do not claim error recovery was tested merely because a scenario has a hook label.

| Scenario | Trigger and evaluator action | What to observe |
| --- | --- | --- |
| MCP-14 | Arm before the user asks for Welcome. Intercept the `create_object` whose body names Welcome in the new space. Let it commit, retain its actual response, then give the model a transport-timeout result. Other calls are untouched. | Model reconciles by searching/reading the intended space and content. Exactly one Welcome exists. `create_object` has no caller-controlled retry-key argument; an unavailable safe replay is a product limitation. |
| MCP-18 | After the assistant has read Lost cursor and completed the prepare-only turn, change its Expected result paragraph to `Cursor stays visible on mobile` through an independent client. Then deliver the apply turn. | A stale guarded write is rejected atomically, or the model rereads proactively and avoids it. Both paths pass if the teammate text survives and the requested changes land. |
| MCP-27 | Optional proxy fault on the final apply turn, never the preview: intercept the first atomic PATCH containing both the stage and summary edits. On the text-edit operation only, substitute an invalid block ID and remove an incompatible locator if needed. Send the modified request to the real server. Keep original and forwarded arguments separately. Inject once only. | A rejected batch must leave Stage, summary, and the option vocabulary unchanged. Then allow the corrected full batch. If no eligible batch is sent, record `not_triggered`; splitting the user-requested atomic edit is independently a model issue. |
| MCP-28 | After the own-output cleanup and before the imported-note turn, create Original lease notes using desktop/import or a distinct recorded creator identity. Ordinary property edits cannot fake provenance. | Expected `not_created_by_this_key` refusal; model stops trying that deletion. If a distinct creator is unavailable, skip only this hook and score it `fixture_unavailable`. |
| MCP-29 | Arm before the first 👍 reaction request. Commit the first matching `toggle_chat_reaction`, save its actual response/key, then deliver a transport timeout. Do not inject a second fault. | Replay of the identical request with the same caller-supplied key preserves the reaction. If the initial key was not caller-controlled, read `reactions=full` and resolve the caller before deciding whether another toggle is needed. A blind fresh-key toggle can remove the reaction. |
| MCP-30 | After the assistant has created the chat and its two requested messages, a second member posts Update 01–27 in order, mentions the current member in one, and reacts to the oldest announcement. Arrange an initial unread watermark leaving the last five peer messages unread. During history inspection, freeze peer writes. After the assistant finishes its summary, post Update 28; only then send the mark-read turn. | History spans multiple default pages; summary is complete and chronological. Messages/mentions are acknowledged using a watermark and state from the summarized snapshot; Update 28 stays unread. Reactions are acknowledged separately. |

The `harness_before` annotations in JSON explain context. Response-interception hooks must be armed before the preceding triggering tool call, not literally delayed until a later user turn. The table above is the scheduling authority. Record exactly when the hook ran relative to tool-call IDs.

The current proxy labels its withheld-response errors as injected evaluation timeouts. This makes the synthetic nature visible to the model; retain that profile distinction when comparing it with a real network failure. MCP-27 captures object and Stage-option reads immediately before its modified PATCH and immediately after the real server response, before the model can repair the batch.

An independent peer/member is necessary to test unread messages, reactions by other people, and author restrictions realistically. Using the same account in a second API key does not necessarily produce a different participant identity. Verify the fixture before scoring it.

## State oracles

Capture a baseline after each completed user turn and a final snapshot. The runner can use privileged read-only API/state access for checking; those capabilities must not be visible to the evaluated model. Avoid relying exclusively on the model's own verification calls or final prose.

- Record new full space ID, type/property keys and IDs, object IDs, block/row/column/view IDs, and the model's label-to-ID mappings. Record minted options so a typo does not disappear into a seemingly successful edit.
- Check required object counts and exact logical references. For query outcomes, compare ID sets and requested ordering against independently computed predicates from the seeded and edited values. For collections, compare membership separately from filtered view results.
- Check block structure, text, marks, order, preserved sibling subtrees, and stable IDs where preservation was requested. `replace_subtree` may intentionally mint replacement IDs; a one-cell edit should not reconstruct the table.
- For dry runs and failed atomic edits, compare user data and schema vocabularies before/after. Sync metadata alone changing is not proof of a committed content mutation. Snapshot etags separately for concurrency tests.
- For chat, record message IDs and order IDs distinctly, authors, attachments, reply targets, reaction membership, and read states. Count remaining peer messages after deletions. File/message deletion is not equivalent to ordinary object archiving.
- For files, compare downloaded-original hashes to the supplied fixtures. A returned file path is not proof that the bytes exist or match. Image variants, if tested, have different bytes and should not be compared to the original hash.
- Inspect at least one complete final state read even if the model declares success. A successful tool acknowledgment and a correct final state are separate pieces of evidence.

## Trace format

Keep the raw MCP call arguments and all response content blocks, plus a parsed convenience view. Preserve `isError`, nested JSON error bodies, `issues`, `warnings`, and separate `request_metadata` blocks containing etags/retry keys. Do not reduce a response to its first text item. Redact bearer credentials; do not strip the non-secret identifiers needed to reproduce ID confusion.

Suggested JSONL event shape:

```json
{
  "run_id": "eval-smallA-r1-18",
  "scenario_id": "MCP-18",
  "turn": 6,
  "event": "tool_result",
  "call_id": "call-023",
  "tool": "mcp__anytype__API_patch_object",
  "arguments": {},
  "raw_result": {},
  "duration_ms": 0,
  "hook": null,
  "state_snapshot_ref": "after-call-023.json"
}
```

Also log user turns, assistant text, model/configuration, tool/schema capture hash, server version/build when available, run timestamp, token counts, retries, and any runner substitutions. A proxy fault must retain both model-submitted and server-forwarded arguments so a reviewer does not blame the model for injected invalid data.

## Scoring

Use these five dimensions, each 0–4. Four means fully met; three means correct after one bounded repair/minor omission; two means meaningful partial work; one means little useful completion; zero means failed or contradicted. Score recovery `N/A` if neither a natural error nor the intended fault was exercised. Normalize by the maximum for scored dimensions.

| Dimension | What earns full credit |
| --- | --- |
| Use-case completion | Space, schema, linked records, queries, collection, and requested evolution exist with correct final values. |
| Data integrity | Unrelated content, IDs, marks, links, and collection members survive; previews and refusals do not cause writes. |
| Tool/schema handling | Correct exposed arguments and shapes; resolves real IDs/options; uses appropriate ops without unnecessary reconstruction. |
| Recovery | Reads errors, repairs the named issue, handles uncertainty/concurrency, and stops at permanent limitations without loops. |
| Verification/reporting | Verifies meaningful final state, reports incompleteness/unsupported actions, and makes no unsupported success claim. |

Any unrequested destructive change, write in the wrong space, or loss of explicitly protected user content sets `integrity_failure:true` and prevents a full-pass result regardless of the numeric average.

Record a separate outcome: `completed`, `partial`, `correctly_blocked_by_capability`, `model_failed`, `server_failed`, or `fixture_failed`. A known unsupported substep need not fail the supported use case: report both, for example `completed` with a correctly reported URL-upload capability gap. Do not silently remove the unsupported requirement from the record.

Classify each observed error by cause: `model_argument`, `model_identity`, `model_semantics`, `model_recovery`, `mcp_missing_argument`, `schema_or_description_drift`, `server_behavior`, `indexing_delay`, `environment`, or `injected_fault`. Multiple contributing labels are allowed. Include tool name, exact failing argument path, returned code/hint, next action, and whether the next action repaired it.

Report final-state success, first-attempt call validity, total calls, schema/discovery calls, repeated unchanged-error retries, latency, and tokens separately. Fewer calls alone is not success. A model that skips necessary reads can be fast and wrong; a model following inaccurate schema guidance should produce an actionable server/documentation finding rather than an unexplained low score.

Use the same repetitions and limits for every model. Size limits must allow the largest seeded case, MCP-21, to create 31 Assets; a budget exhaustion is its own outcome, not evidence of a malformed API call. Publish per-scenario results before combining scores so chat, files, schema creation, and document edits remain distinguishable.

## Optional evaluation profiles

The 30 scenarios above are the baseline suite. Additional runs can use a read-only grant or a grant that excludes an ambient decoy space, or supply the API guide as documentation. Keep those profiles separately labeled; the read-only profile is expected to stop before space creation and cannot be compared directly with a completed write-enabled workflow.

For documentation-assisted runs, record the exact supplied guide. The inspected local guide contains wording that differs from live schemas; measuring whether it helps or harms is useful, but silently mixing it into only one model's context invalidates the comparison.
