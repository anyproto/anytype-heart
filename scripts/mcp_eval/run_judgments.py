#!/usr/bin/env python3
"""Dispatch prepared primary Astra judgments with bounded concurrency.

An explicit user approval receipt is required because automatic approval review
rejected this campaign's transcript export. --check-only makes no model calls.
Per-run judge locks and immutable hashes retain the existing retry discipline.
"""
import argparse
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
import fcntl
import hashlib
import json
from pathlib import Path
from types import SimpleNamespace

from campaign_status import BASE, build
from review_codex import aggregate, judge


def plan(campaign):
    matrix = build()
    matrix_path = campaign / "matrix.json"
    matrix_path.write_text(json.dumps(matrix, indent=2) + "\n")
    authorization_path = campaign / "astra-review-authorization.json"
    authorization = json.loads(authorization_path.read_text()) if authorization_path.exists() else {"status": "pending"}
    preflight_path = campaign / "review-token-preflight.json"
    preflight = json.loads(preflight_path.read_text()) if preflight_path.exists() else {}
    token_rows = {row["run_id"]: row for row in preflight.get("runs", [])}
    jobs, errors = [], []
    for slot in matrix["slots"]:
        if slot["status"] != "ready_for_astra_review":
            continue
        run = Path(slot["run"]["run_dir"])
        output = campaign / "astra-reviews/runs" / run.name
        prompt_path = output / "judge-prompt.txt"
        tokens = token_rows.get(run.name)
        if not (output / "job.json").exists() or not prompt_path.exists():
            errors.append({"run_id": run.name, "error": "Run prepare_review_inputs.py first"})
            continue
        if not tokens or hashlib.sha256(prompt_path.read_bytes()).hexdigest() != tokens.get("prompt_sha256"):
            errors.append({"run_id": run.name, "error": "Token preflight is missing or addresses a different prompt"})
            continue
        if tokens["max_estimate"] > preflight["conservative_prompt_limit"]:
            errors.append({"run_id": run.name, "error": "Prepared prompt exceeds the conservative context budget"})
            continue
        jobs.append((run, output))
    return {"matrix": matrix_path, "authorization": authorization, "jobs": jobs, "errors": errors}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--campaign", type=Path, default=BASE / "artifacts/luna-three-runs")
    parser.add_argument("--workers", type=int, choices=range(1, 5), default=3)
    parser.add_argument("--codex", default="codex")
    parser.add_argument("--timeout", type=int, default=1800)
    parser.add_argument("--check-only", action="store_true")
    args = parser.parse_args()
    campaign = args.campaign.resolve()
    with (campaign / "judge-dispatch.lock").open("w") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        prepared = plan(campaign)
        approved = prepared["authorization"].get("status") == "approved"
        if args.check_only:
            print(json.dumps({"prepared_primary_runs": len(prepared["jobs"]),
                              "authorization_status": prepared["authorization"].get("status"),
                              "errors": prepared["errors"], "model_requests_made": 0}))
            return
        if not approved:
            raise SystemExit("Astra transcript reviews await the explicit user approval requested after automatic approval rejection")
        if prepared["errors"]:
            raise SystemExit(json.dumps(prepared["errors"]))
        judge_args = SimpleNamespace(suite=campaign / "spec/scenarios.json", prepare_only=False,
                                     retry_failed=False, max_input_bytes=12_000_000,
                                     codex=args.codex, timeout=args.timeout)

        def work(job):
            run, output = job
            try:
                status = judge(run, output, judge_args)
                result = {"run_id": run.name, "status": status}
            except Exception as error:
                result = {"run_id": run.name, "status": "failed", "error": str(error)}
            print(json.dumps(result), flush=True)
            return result

        with ThreadPoolExecutor(max_workers=args.workers) as pool:
            results = list(pool.map(work, prepared["jobs"]))
        aggregate(campaign / "astra-reviews", judge_args.suite, 3, prepared["matrix"])
        (campaign / "judge-dispatch.json").write_text(json.dumps({
            "recorded_at": datetime.now(timezone.utc).isoformat(), "results": results}, indent=2) + "\n")
        if any(result["status"] == "failed" for result in results):
            raise SystemExit(1)


if __name__ == "__main__":
    main()
