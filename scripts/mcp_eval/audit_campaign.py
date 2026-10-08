#!/usr/bin/env python3
"""Audit local actor metadata, snapshots, and all captured space-scoped traffic.

This is execution evidence, not a judgment of scenario success. It makes no
network requests and never changes original run files.
"""
import argparse
from collections import Counter
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path

from campaign_status import BASE, build
from summarize_runs import SCOPED, normalized


def rows(path):
    for number, line in enumerate(path.read_text().splitlines(), 1):
        if line.strip():
            yield number, json.loads(line)


def audit(run):
    manifest = json.loads((run / "run.json").read_text())
    scope = json.loads((run / "scope.json").read_text())
    owned = {space[key] for space in scope.get("spaces", [])
             for key in ("full_id", "compact_id") if space.get(key)}
    traffic = Counter()
    violations = []
    for path in sorted(run.glob("*mcp-wire.jsonl")):
        for line, event in rows(path):
            if event.get("direction") not in {"to_upstream", "evaluator_to_upstream"}:
                continue
            payload = event.get("payload", {})
            if payload.get("method") != "tools/call":
                continue
            params = payload.get("params", {})
            if normalized(params.get("name", "")) not in SCOPED:
                continue
            traffic[path.name] += 1
            if params.get("arguments", {}).get("space_id") not in owned:
                violations.append({"source": str(path), "line": line,
                                   "tool": params.get("name"), "reason": "outside owned space"})

    threads = set()
    for path in run.glob("turn-*.events.jsonl"):
        threads.update(event["thread_id"] for _, event in rows(path)
                       if event.get("type") == "thread.started")
    observed = []
    sources = []
    codex_dir = Path(os.environ.get("CODEX_HOME", str(Path.home() / ".codex")))
    for thread in sorted(threads):
        for path in (codex_dir / "sessions").glob(f"*/*/*/*{thread}*.jsonl"):
            sources.append({"path": str(path), "sha256": hashlib.sha256(path.read_bytes()).hexdigest()})
            for line, event in rows(path):
                if event.get("type") != "turn_context":
                    continue
                payload = event.get("payload", {})
                observed.append({"source": str(path), "line": line,
                                 "model": payload.get("model"),
                                 "reasoning": payload.get("effort", payload.get("reasoning_effort"))})
    completed_turns = sum(bool(t.get("turn_completed")) for t in manifest.get("turns", []))
    model_verified = (len(threads) == 1 and len(observed) >= completed_turns > 0 and
                      all(r["model"] == "gpt-5.6-luna" and r["reasoning"] == "medium" for r in observed))
    snapshots = []
    for path in sorted(run.glob("snapshots/*.json")):
        data = json.loads(path.read_text())
        snapshots.append({"source": str(path), "after_turn": data.get("after_turn"),
                          "capture_status": data.get("capture_status")})
    final_path = run / "final-state.json"
    final = json.loads(final_path.read_text()) if final_path.exists() else {}
    return {"run_id": manifest["label"], "scenario_id": manifest["scenario_id"],
            "model_verified": model_verified, "observed_contexts": observed,
            "metadata_sources": sources, "completed_turns": completed_turns,
            "forwarded_scoped_calls": dict(traffic), "scope_violations": violations,
            "scope_verified": bool(traffic) and not violations,
            "snapshots": snapshots, "final_state_present": final_path.exists(),
            "final_capture_status": final.get("capture_status", "legacy_unlabelled" if final else "missing")}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=BASE / "artifacts/luna-three-runs/execution-audit.json")
    args = parser.parse_args()
    results, errors = [], []
    for slot in build()["slots"]:
        if slot["status"] != "ready_for_astra_review":
            continue
        run = Path(slot["run"]["run_dir"])
        try:
            results.append({"replicate": slot["replicate"], **audit(run)})
        except Exception as error:
            errors.append({"run": str(run), "error": str(error)})
    summary = {"audited_runs": len(results), "model_verified": sum(r["model_verified"] for r in results),
               "scope_verified": sum(r["scope_verified"] for r in results),
               "scope_violations": sum(len(r["scope_violations"]) for r in results),
               "scoped_calls": sum(sum(r["forwarded_scoped_calls"].values()) for r in results),
               "audit_errors": len(errors)}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps({"recorded_at": datetime.now(timezone.utc).isoformat(),
                                      "summary": summary, "runs": results, "errors": errors}, indent=2) + "\n")
    print(json.dumps(summary))


if __name__ == "__main__":
    main()
