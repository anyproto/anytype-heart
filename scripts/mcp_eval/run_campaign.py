#!/usr/bin/env python3
"""Run a scenario campaign: hosts × arms × scenarios × repetitions against
one throwaway heart built from this tree (spec §5,
docs/superpowers/specs/2026-10-08-mcp-eval-hosts-and-arms-design.md).

1. Build cmd/evalheart and boot it: heartboot builds this tree's heart,
   creates an empty local account and mints a key, handed over a pipe.
2. For each host × arm × scenario × repetition, run run_scenario.py in its
   own run directory: one fresh conversation, one guard instance, the
   scenario's own new space; at most --parallel at once, because every run
   shares one heart and its loopback write limiter.
3. Snapshot each run's final state through its own guard.
4. Tear the heart down (the account directory is deleted) and write
   campaign.json: heart commit, host versions, models, arms, the served
   tools/list size per arm, the scenario-set hash, and every run's outcome
   with its rate-limit refusals counted apart from model errors.

Run output stays outside the repository; --output is required.
"""
import argparse
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
import hashlib
import itertools
import json
import os
from pathlib import Path
import subprocess
import sys
import urllib.request

from space_guard import apply_arm, compact_bytes
from summarize_runs import rate_limit_refusals

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
SUITE = ROOT / "docs/evals/anytype-mcp-v2/scenarios.json"
DEFAULT_MODELS = {"claude": "claude-haiku-4-5-20251001", "codex": "gpt-5.6-luna"}


def git(*args):
    return subprocess.run(["git", "-C", str(ROOT), *args], capture_output=True, text=True).stdout.strip()


def host_version(host, executable):
    try:
        out = subprocess.run([executable or host, "--version"], capture_output=True, text=True, timeout=30)
        return out.stdout.strip() or out.stderr.strip()
    except (OSError, subprocess.TimeoutExpired) as exc:
        return f"unavailable ({exc})"


def scenario_set_hash(scenarios):
    canonical = json.dumps(scenarios, sort_keys=True, ensure_ascii=False, separators=(",", ":"))
    return hashlib.sha256(canonical.encode()).hexdigest()


class Heart:
    """cmd/evalheart as a child: ready line on stdout, teardown on stdin close."""

    def __init__(self, out_dir, keep_account=False, heart_binary=None):
        self.out_dir = out_dir
        binary = out_dir / "evalheart"
        subprocess.run(["go", "build", "-o", str(binary), "./cmd/evalheart"], cwd=ROOT, check=True)
        cmd = [str(binary)]
        if keep_account:
            cmd.append("-keep-account")
        if heart_binary:
            cmd += ["-heart-binary", heart_binary]
        self.log = open(out_dir / "evalheart.log", "w")
        self.proc = subprocess.Popen(cmd, cwd=ROOT, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                     stderr=self.log, text=True)
        line = self.proc.stdout.readline()
        if not line:
            self.proc.wait(timeout=30)
            raise RuntimeError(f"evalheart exited before it was ready; see {out_dir / 'evalheart.log'}")
        info = json.loads(line)
        self.url, self.key = info["api_url"], info["api_key"]
        self.account_id, self.data_dir = info["account_id"], info["data_dir"]

    @property
    def mcp_url(self):
        return self.url + "/mcp/full"

    def tools_list(self):
        def post(message):
            request = urllib.request.Request(self.mcp_url, data=json.dumps(message).encode(), method="POST", headers={
                "Authorization": "Bearer " + self.key, "Content-Type": "application/json", "Accept": "application/json"})
            with urllib.request.urlopen(request, timeout=60) as response:
                return json.loads(response.read())
        post({"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2025-06-18"}})
        return post({"jsonrpc": "2.0", "id": 2, "method": "tools/list"})["result"]

    def stop(self):
        try:
            self.proc.stdin.close()
            self.proc.wait(timeout=120)
        except subprocess.TimeoutExpired:
            self.proc.terminate()
            self.proc.wait(timeout=30)
        self.log.close()
        return {"exit_code": self.proc.returncode, "account_directory_removed": not Path(self.data_dir).exists()}


def run_one(job, args, heart, runs_dir, env):
    host, arm, scenario, rep = job
    label = f"eval-{host}-{arm}-r{rep}-{datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')}-{scenario.lower()}"
    cmd = [sys.executable, str(HERE / "run_scenario.py"), "--host", host, "--scenario", scenario,
           "--model", args.models[host], "--output", str(runs_dir), "--label", label,
           "--upstream-url", heart.mcp_url, "--arm", arm, "--turn-timeout", str(args.turn_timeout),
           "--final-snapshot"]
    if host == "codex":
        cmd += ["--reasoning", args.codex_reasoning]
    if args.max_turns:
        cmd += ["--max-turns", str(args.max_turns)]
    executable = args.claude if host == "claude" else args.codex
    if executable:
        cmd += ["--host-executable", executable]
    with open(runs_dir / f"{label}.runner.log", "w") as log:
        exit_code = subprocess.run(cmd, stdout=log, stderr=subprocess.STDOUT, env=env).returncode
    run_dir = runs_dir / label
    manifest = json.loads((run_dir / "run.json").read_text()) if (run_dir / "run.json").exists() else {}
    return {"host": host, "arm": arm, "scenario_id": scenario, "repetition": rep, "label": label,
            "exit_code": exit_code, "status": manifest.get("status", "not_started"),
            "tool_calls": manifest.get("tool_call_count"), "tool_errors": manifest.get("tool_errors"),
            "rate_limit_refusals": rate_limit_refusals(run_dir) if run_dir.exists() else 0,
            "tools_list": manifest.get("tools_list")}


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--output", type=Path, required=True, help="campaign directory, outside the repository")
    p.add_argument("--hosts", nargs="+", choices=["claude", "codex"], default=["claude", "codex"])
    p.add_argument("--arms", nargs="+", choices=["full", "full-opaque"], default=["full", "full-opaque"])
    p.add_argument("--scenarios", nargs="+", default=["all"])
    p.add_argument("--repetitions", type=int, default=1)
    p.add_argument("--parallel", type=int, default=4)
    p.add_argument("--claude-model", default=DEFAULT_MODELS["claude"])
    p.add_argument("--codex-model", default=DEFAULT_MODELS["codex"])
    p.add_argument("--codex-reasoning", default="medium")
    p.add_argument("--claude", help="claude executable")
    p.add_argument("--codex", help="codex executable")
    p.add_argument("--turn-timeout", type=int, default=600)
    p.add_argument("--max-turns", type=int)
    p.add_argument("--keep-account", action="store_true")
    p.add_argument("--heart-binary")
    args = p.parse_args()
    args.models = {"claude": args.claude_model, "codex": args.codex_model}

    out = args.output.resolve()
    if ROOT in out.parents or out == ROOT:
        p.error("run output stays outside the repository")
    out.mkdir(parents=True, exist_ok=True)
    runs_dir = out / "runs"
    runs_dir.mkdir(exist_ok=True)
    suite = json.loads(SUITE.read_text())
    by_id = {s["id"]: s for s in suite["scenarios"]}
    selected = list(by_id) if args.scenarios == ["all"] else args.scenarios
    unknown = set(selected) - by_id.keys()
    if unknown:
        p.error("unknown scenarios: " + ", ".join(sorted(unknown)))

    manifest = {"started_at": datetime.now(timezone.utc).isoformat(),
                "heart_commit": git("rev-parse", "HEAD"), "heart_tree_dirty": bool(git("status", "--porcelain")),
                "hosts": {h: {"version": host_version(h, args.claude if h == "claude" else args.codex),
                              "model": args.models[h],
                              **({"reasoning": args.codex_reasoning} if h == "codex" else {})} for h in args.hosts},
                "arms": args.arms, "scenarios": selected, "scenario_set_sha256": scenario_set_hash([by_id[s] for s in selected]),
                "repetitions": args.repetitions, "parallel": args.parallel, "runs": []}
    heart = Heart(out, args.keep_account, args.heart_binary)
    try:
        manifest["heart"] = {"api_url": heart.url, "account_id": heart.account_id}
        served = heart.tools_list()
        manifest["tools_list"] = {arm: {"tools": len(served["tools"]), "bytes": compact_bytes(apply_arm(served, arm))}
                                  for arm in args.arms}
        env = {**os.environ, "ANYTYPE_API_KEY": heart.key}
        jobs = list(itertools.product(args.hosts, args.arms, selected, range(1, args.repetitions + 1)))
        with ThreadPoolExecutor(max_workers=max(1, args.parallel)) as pool:
            for result in pool.map(lambda job: run_one(job, args, heart, runs_dir, env), jobs):
                manifest["runs"].append(result)
                print(f"{result['host']} {result['arm']} {result['scenario_id']} r{result['repetition']}: "
                      f"{result['status']} ({result['tool_calls']} calls, {result['tool_errors']} errors, "
                      f"{result['rate_limit_refusals']} rate-limited)", flush=True)
                (out / "campaign.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
    finally:
        manifest["teardown"] = heart.stop()
        manifest["finished_at"] = datetime.now(timezone.utc).isoformat()
        (out / "campaign.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
    print(f"campaign: {out / 'campaign.json'}", flush=True)
    finished = {"conversation_completed_pending_review", "stopped_at_requested_turn_limit"}
    return 0 if all(r["status"] in finished for r in manifest["runs"]) else 1


if __name__ == "__main__":
    sys.exit(main())
