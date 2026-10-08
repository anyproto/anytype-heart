#!/usr/bin/env python3
"""Run distinct Luna repetitions with a durable, explicitly bounded job queue.

Existing terminal jobs are never rerun. Existing running/failed jobs require
inspection; observation timeouts never cause a replacement process.
"""
import argparse
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import subprocess
import sys
import threading


def now():
    return datetime.now(timezone.utc).isoformat()


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--campaign", type=Path, required=True)
    p.add_argument("--rounds", nargs="+", type=int, choices=[1, 2, 3], default=[2, 3])
    p.add_argument("--scenarios", nargs="+", required=True)
    p.add_argument("--workers", type=int, default=3)
    p.add_argument("--with-hooks", action="store_true")
    p.add_argument("--codex", default="codex")
    args = p.parse_args()
    root = args.campaign.resolve()
    root.mkdir(parents=True, exist_ok=True)
    path = root / "campaign.json"
    lock_path = root / "campaign.lock"
    lock = threading.Lock()
    # Exclusive lock is released normally. A stale lock is evidence to inspect,
    # not permission to launch duplicate work.
    with open(lock_path, "x") as f:
        json.dump({"pid": os.getpid(), "started_at": now()}, f)
    state = json.loads(path.read_text()) if path.exists() else {"version": 1, "jobs": []}
    known = {(j["replicate"], j["scenario_id"]) for j in state["jobs"]}
    for replicate in args.rounds:
        for sid in args.scenarios:
            if (replicate, sid) not in known:
                state["jobs"].append({"replicate": replicate, "scenario_id": sid,
                                      "with_hooks": args.with_hooks, "status": "pending"})
                known.add((replicate, sid))

    def save():
        temp = path.with_suffix(".tmp")
        temp.write_text(json.dumps(state, indent=2) + "\n")
        temp.replace(path)

    def work(job):
        rid, sid = job["replicate"], job["scenario_id"]
        output = root / f"round-{rid}" / "runs"
        logdir = root / "logs"
        logdir.mkdir(exist_ok=True)
        command = [sys.executable, str(Path(__file__).with_name("run_codex.py")),
                   "--scenario", sid, "--replicate", str(rid), "--model", "gpt-5.6-luna",
                   "--codex", args.codex, "--snapshot-after-turn", "--output", str(output)]
        if job["with_hooks"]:
            command.append("--with-hooks")
        with open(logdir / f"r{rid}-{sid}.log", "a", buffering=1) as logfile:
            with lock:
                job.update(status="starting", command=command, started_at=now())
                save()
            proc = subprocess.Popen(command, stdout=logfile, stderr=subprocess.STDOUT,
                                    start_new_session=True)
            with lock:
                job.update(status="running", pid=proc.pid)
                save()
            print(f"r{rid} {sid} running pid={proc.pid}", flush=True)
            code = proc.wait()
        manifests = list(output.glob(f"*-{sid.lower()}/run.json"))
        run = json.loads(manifests[0].read_text()) if len(manifests) == 1 else {}
        completed = code == 0 and run.get("status") == "conversation_completed_pending_review"
        with lock:
            job.update(status="completed" if completed else "failed", exit_code=code,
                       ended_at=now(), run_dir=str(manifests[0].parent) if len(manifests) == 1 else None)
            save()
        print(f"r{rid} {sid} {job['status']}", flush=True)

    try:
        if any(j["status"] in {"starting", "running"} for j in state["jobs"]):
            raise RuntimeError("Existing active job records require checking their actual processes before resuming")
        save()
        pending = [j for j in state["jobs"] if j["status"] == "pending"]
        with ThreadPoolExecutor(max_workers=args.workers) as pool:
            list(pool.map(work, pending))
    finally:
        lock_path.unlink(missing_ok=True)


if __name__ == "__main__":
    main()
