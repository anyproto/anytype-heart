#!/usr/bin/env python3
"""Prepare completed primary runs for Astra locally; never start a model request."""
import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
from types import SimpleNamespace

from campaign_status import BASE, build
from review_codex import judge
from verify_file_bytes import check_run as check_file_bytes


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--campaign", type=Path, default=BASE / "artifacts/luna-three-runs")
    args = parser.parse_args()
    campaign = args.campaign.resolve()
    matrix = build()
    (campaign / "matrix.json").write_text(json.dumps(matrix, indent=2) + "\n")
    judge_args = SimpleNamespace(suite=campaign / "spec/scenarios.json", prepare_only=True,
                                retry_failed=False, max_input_bytes=12_000_000)
    results = []
    for slot in matrix["slots"]:
        if slot["status"] != "ready_for_astra_review":
            continue
        run = Path(slot["run"]["run_dir"])
        output = campaign / "astra-reviews/runs" / run.name
        row = {"scenario_id": slot["scenario_id"], "replicate": slot["replicate"], "run_id": run.name}
        try:
            # Capture byte evidence before freezing the review input. Preserve
            # previous observations: rehashing later could see expired downloads.
            file_evidence = run / "file-byte-verification.json"
            if slot["scenario_id"] == "MCP-22" and not file_evidence.exists():
                if (output / "job.json").exists():
                    raise ValueError("File evidence was missing at preparation; inspect before changing frozen inputs")
                file_evidence.write_text(json.dumps(check_file_bytes(run), ensure_ascii=False, indent=2) + "\n")
            row["status"] = judge(run, output, judge_args)
            row["prompt_bytes"] = json.loads((output / "job.json").read_text())["prompt_bytes"]
        except Exception as error:
            row["error"] = str(error)
        results.append(row)
    summary = {"prepared_or_cached": sum("error" not in row for row in results),
               "errors": [row for row in results if "error" in row]}
    (campaign / "review-preparation.json").write_text(json.dumps({
        "recorded_at": datetime.now(timezone.utc).isoformat(), "model_requests_made": 0,
        "summary": summary, "runs": results}, indent=2) + "\n")
    print(json.dumps(summary))


if __name__ == "__main__":
    main()
