#!/usr/bin/env python3
"""Measure completed primary actor traces locally, without importing prior grades."""
from collections import Counter
from datetime import datetime, timezone
import json
from pathlib import Path

from campaign_status import BASE, build
from summarize_runs import call_records, error_info, is_failed, normalized


def main():
    campaign = BASE / "artifacts/luna-three-runs"
    surface = json.loads((campaign / "spec/surface-snapshot.json").read_text())
    tool_counts, accepted, schema_calls, op_counts, op_accepted = (Counter() for _ in range(5))
    failures, runs = [], []
    for slot in build()["slots"]:
        if slot["status"] != "ready_for_astra_review":
            continue
        run = Path(slot["run"]["run_dir"])
        calls = call_records(run)
        run_failures = 0
        for call in calls:
            tool = normalized(call.get("tool", ""))
            failed = is_failed(call)
            args = call.get("arguments") or {}
            tool_counts[tool] += 1
            accepted[tool] += not failed
            if tool == "get_schema":
                schema_calls[args.get("kind", "unknown")] += 1
            if tool == "get_op_schema":
                schema_calls["ops/" + args.get("op", "unknown")] += 1
            if tool == "patch_object":
                body = args.get("body") or {}
                for operation in body.get("ops", []) if isinstance(body, dict) else []:
                    if isinstance(operation, dict):
                        name = operation.get("op", "unknown")
                        op_counts[name] += 1
                        op_accepted[name] += not failed
            if failed:
                run_failures += 1
                failures.append({"run_id": run.name, "scenario_id": slot["scenario_id"],
                                 "replicate": slot["replicate"], "turn": call.get("turn"),
                                 "tool": tool, "result_ref": call["source"], **error_info(call)})
        runs.append({"run_id": run.name, "scenario_id": slot["scenario_id"], "replicate": slot["replicate"],
                     "actor_calls": len(calls), "failed_calls": run_failures, "run_dir": str(run)})
    declared = [normalized(tool["name"]) for tool in surface["tools"]]
    known_ops = [key.removeprefix("ops/") for key in surface["schemas"] if key.startswith("ops/")]
    report = {"recorded_at": datetime.now(timezone.utc).isoformat(),
              "scope": "Completed primary actor calls only; evaluator probes and all prior model grades excluded. A failed call may be an expected negative test, missing capability, injected fault, or model/server error. Acceptance is not semantic success; operation counts include previews and no-ops.",
              "summary": {"completed_primary_runs": len(runs), "actor_calls": sum(tool_counts.values()),
                          "failed_calls": len(failures), "declared_tools": len(declared),
                          "attempted_tools": sum(tool_counts[tool] > 0 for tool in declared),
                          "tools_with_accepted_calls": sum(accepted[tool] > 0 for tool in declared)},
              "tools": {tool: {"attempted": tool_counts[tool], "accepted": accepted[tool]} for tool in declared},
              # Retain reference keys (including zeros) and runtime-only schemas.
              # Type-update schemas share discovery with object PATCH schemas,
              # but do not expand the object operation inventory below.
              "schema_discovery_calls": {key: schema_calls[key] for key in
                                         list(surface["schemas"]) + sorted(set(schema_calls) - set(surface["schemas"]))},
              "patch_operations": {name: {"attempted_in_calls": op_counts[name],
                                          "included_in_accepted_calls": op_accepted[name]} for name in known_ops},
              "runs": runs, "failed_calls": failures}
    (campaign / "measurements.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(report["summary"]))


if __name__ == "__main__":
    main()
