#!/usr/bin/env python3
"""Build the explicit 30 x 3 review matrix; never infer completion from file count."""
import argparse
from collections import Counter
import hashlib
import json
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
BASE = ROOT / "docs/evals/anytype-mcp-v2"
HOOK_R1_CONTROLS = {
    "MCP-14": "eval-luna-20260916T214508Z-mcp-14",
    "MCP-18": "eval-luna-20260916T214530Z-mcp-18",
    "MCP-27": "eval-luna-20260916T214530Z-mcp-27",
    "MCP-29": "eval-luna-20260916T214508Z-mcp-29",
}


def scope_amendments(base=BASE):
    """Read explicit user amendments as reporting metadata, never grading overrides."""
    path = base / "artifacts/luna-three-runs/scope-amendments.json"
    if not path.exists():
        return []
    raw = path.read_bytes()
    document = json.loads(raw)
    return [{**amendment, "amendment_key": key, "recorded_at": document.get("recorded_at"),
             "source": document.get("source"), "source_file": str(path),
             "source_sha256": hashlib.sha256(raw).hexdigest()}
            for key, amendment in document.items()
            if isinstance(amendment, dict) and amendment.get("scenario_id")]


def build(base=BASE):
    suite = json.loads((base / "scenarios.json").read_text())
    scenarios = {s["id"]: s for s in suite["scenarios"]}
    baseline_root = base / "artifacts/luna-20260916/runs"
    hooked_r1_root = base / "artifacts/luna-three-hooks-r1-complete"
    roots = [baseline_root,
             base / "artifacts/luna-three-runs", base / "artifacts/luna-three-hooks",
             base / "artifacts/luna-three-fixtures", base / "artifacts/luna-three-peer", hooked_r1_root]
    candidates = {}
    excluded = []
    controls = []
    for root in roots:
        if not root.exists():
            continue
        for path in root.rglob("run.json"):
            run = json.loads(path.read_text())
            # A resumption preserves its old manifest beneath the same run;
            # that checkpoint is evidence, not another primary conversation.
            if (path.parent.parent.name == "resumptions" and
                    path.parent.name.startswith("attempt-") and
                    path.parent.parent.parent.name == run.get("label")):
                excluded.append({"run": str(path.parent),
                                 "reason": "Archived pre-resumption metadata for the same primary conversation"})
                continue
            sid = run.get("scenario_id")
            if sid not in scenarios:
                continue
            repetition = run.get("replicate") or (1 if root.name == "runs" else None)
            if repetition not in (1, 2, 3):
                continue
            if root == hooked_r1_root and (sid not in HOOK_R1_CONTROLS or repetition != 1):
                raise ValueError(f"Unexpected scenario/replicate in hooked R1 replacement root: {sid}, {repetition}")
            expected = [t["turn"] for t in scenarios[sid]["turns"]]
            delivered = [t["turn"] for t in run.get("turns", []) if t.get("turn_completed")]
            if repetition == 1 and sid == "MCP-28" and run.get("skipped_turns"):
                excluded.append({"run": str(path.parent), "reason": "Original baseline skipped the foreign-provenance user turn; a full replacement is required"})
                continue
            key = (sid, repetition)
            final = path.parent / "final-state.json"
            candidate = {
                "run_id": run["label"], "run_dir": str(path.parent), "runner_status": run.get("status"),
                "all_turns_delivered": delivered == expected and not run.get("skipped_turns"),
                "delivered_turns": len(delivered), "expected_turns": len(expected),
                "final_state_present": final.exists(), "hook_status": run.get("hook_status"),
                "snapshot_errors": [t.get("independent_snapshot_error") for t in run.get("turns", []) if t.get("independent_snapshot_error")],
                "runner_reported_model": run.get("model"), "runner_reported_reasoning": run.get("reasoning")}
            original_label = HOOK_R1_CONTROLS.get(sid)
            if (root == baseline_root and repetition == 1 and original_label and
                    run.get("label") == original_label and path.parent == baseline_root / original_label):
                if run.get("hook_status") != "not_exercised":
                    raise ValueError(f"Known unhooked control has unexpected hook status: {path}")
                controls.append({"scenario_id": sid, "former_replicate": 1, "classification": "control",
                                 "reason": "Original R1 did not exercise the controlled hook. Preserved as an unhooked control; the new hooked R1 supplies the primary replicate. Control judgments do not count toward the three primary repetitions.",
                                 "run": candidate})
                continue
            if sid in HOOK_R1_CONTROLS and repetition == 1 and root != hooked_r1_root:
                raise ValueError(f"Unapproved hooked R1 candidate outside replacement root: {path}")
            if key in candidates:
                raise ValueError(f"Two primary candidates for {key}; resolve explicitly, never silently select a passing attempt")
            if root == hooked_r1_root:
                candidate["required_hook_triggered"] = run.get("hook_status") == "triggered"
            candidates[key] = candidate
    slots = []
    for sid in scenarios:
        for repetition in (1, 2, 3):
            c = candidates.get((sid, repetition))
            status = "not_started" if c is None else ("ready_for_astra_review" if
                c["all_turns_delivered"] and c["final_state_present"] and
                c.get("required_hook_triggered", True) and
                c["runner_status"] == "conversation_completed_pending_review" else "incomplete")
            slots.append({"scenario_id": sid, "replicate": repetition, "status": status, "run": c})
    for control in controls:
        replacement = candidates.get((control["scenario_id"], 1))
        control["replaced_by"] = ({"run_id": replacement["run_id"], "run_dir": replacement["run_dir"]}
                                  if replacement else None)
    approval_path = base / "artifacts/luna-three-runs/astra-review-authorization.json"
    approval = json.loads(approval_path.read_text()) if approval_path.exists() else {"status": "pending",
        "reason": "Automatic approval review rejected original and sanitized requests; explicit user confirmation was requested"}
    return {"objective": "30 scenarios, three complete conversations each, Astra high review of each transcript, per-scenario JSON and final Markdown",
            "recorded_at": datetime.now(timezone.utc).isoformat(),
            "actor_model": "gpt-5.6-luna", "actor_reasoning": "medium", "judge_model": "gpt-6-astra",
            "judge_reasoning": "high", "expected_primary_runs": 90, "counts": dict(Counter(x["status"] for x in slots)),
            "review_authorization": approval,
            "scope_amendments": scope_amendments(base),
            "slots": slots, "controls": controls, "excluded_runs": excluded}


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--output", type=Path, default=BASE / "artifacts/luna-three-runs/matrix.json")
    args = p.parse_args()
    result = build()
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps(result["counts"]))


if __name__ == "__main__":
    main()
