#!/usr/bin/env python3
"""Estimate prepared review input size locally with cached tiktoken vocabularies.

Install tiktoken in a separate environment and cache its standard vocabularies
before using this script. It never sends transcripts to a model. These standard
encodings are estimates; they do not claim to expose Astra's private tokenizer.
"""
import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path

import tiktoken

from campaign_status import BASE


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--campaign", type=Path, default=BASE / "artifacts/luna-three-runs")
    parser.add_argument("--context-window", type=int, default=272000)
    parser.add_argument("--effective-percent", type=int, default=95)
    parser.add_argument("--reserve", type=int, default=24000)
    args = parser.parse_args()
    encoders = {name: tiktoken.get_encoding(name) for name in ("o200k_base", "cl100k_base")}
    limit = args.context_window * args.effective_percent // 100 - args.reserve
    rows = []
    for path in sorted((args.campaign / "astra-reviews/runs").glob("*/judge-prompt.txt")):
        text = path.read_text()
        counts = {name: len(encoder.encode(text, disallowed_special=())) for name, encoder in encoders.items()}
        rows.append({"run_id": path.parent.name, "bytes": len(text.encode()),
                     "prompt_sha256": hashlib.sha256(text.encode()).hexdigest(),
                     "token_estimates": counts, "max_estimate": max(counts.values())})
    report = {"recorded_at": datetime.now(timezone.utc).isoformat(),
              "method": f"Local tiktoken {tiktoken.__version__} counts with two standard encodings; private Astra tokenizer may differ.",
              "context_window": args.context_window, "effective_percent": args.effective_percent,
              "reserved_tokens_for_output_and_runtime": args.reserve,
              "conservative_prompt_limit": limit, "runs": rows}
    (args.campaign / "review-token-preflight.json").write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({"count": len(rows), "largest": sorted(rows, key=lambda r: r["max_estimate"], reverse=True)[:5],
                      "above_conservative_limit": [row["run_id"] for row in rows if row["max_estimate"] > limit]}, indent=2))


if __name__ == "__main__":
    main()
