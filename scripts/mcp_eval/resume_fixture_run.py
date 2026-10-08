#!/usr/bin/env python3
"""Inspect, or explicitly resume, a run stopped before a fixture turn.

Default is local validation only. --execute preserves the existing directory,
space, workspace, model and exact thread ID. It never creates a replacement run.
Any lock, live/uncertain PID, partial turn, transport failure or changed scenario
requires inspection outside this tool; nothing is terminated or auto-unlocked.
MCP-30 alone may continue with an explicit user-authorized unavailable-peer
receipt. Such a receipt supplies no peer identity or invented history.
"""
import argparse
from collections import Counter
from contextlib import contextmanager
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys

import run_codex as runner
from eval_fixtures import before_turn, finalize_turn
from fault_hooks import atomic_json
from snapshot_run import snapshot
from space_guard import documents


FROZEN_SUITE = runner.ROOT / "docs/evals/anytype-mcp-v2/artifacts/luna-three-runs/spec/scenarios.json"
BOUNDARIES = {("MCP-28", 7): "desktop_provenance",
              ("MCP-30", 5): "peer_history", ("MCP-30", 6): "peer_arrival"}


def load(path):
    return json.loads(path.read_text())


def events(path):
    # Unlike report extraction, resumption must not ignore a truncated event.
    return [json.loads(line) for line in path.read_text().splitlines() if line.strip()]


def require(condition, message):
    if not condition:
        raise ValueError(message)


def assert_absent(pid, label):
    require(isinstance(pid, int) and not isinstance(pid, bool) and pid > 1,
            label + " has no trustworthy PID")
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return
    except PermissionError as exc:
        raise ValueError(label + " process state is not observable") from exc
    raise ValueError(label + " PID still exists; refusing even if the PID was reused")


def process_table():
    result = subprocess.run(["ps", "-ww", "-axo", "pid=,ppid=,stat=,command="],
                            check=True, capture_output=True, text=True)
    rows = []
    for line in result.stdout.splitlines():
        fields = line.strip().split(None, 3)
        require(len(fields) == 4 and fields[0].isdigit() and fields[1].isdigit(),
                "Cannot interpret the process inventory")
        rows.append({"pid": int(fields[0]), "ppid": int(fields[1]),
                     "state": fields[2], "command": fields[3]})
    require(bool(rows), "Process inventory is unavailable")
    return rows


def check_processes(run_dir, manifest, active):
    assert_absent(manifest.get("runner_pid"), "Prior runner")
    require(active.get("state") == "exited" and active.get("exit_code") == 0 and not active.get("timeout"),
            "Last Codex process is not recorded as successfully exited")
    assert_absent(active.get("pid"), "Prior Codex")
    rows = process_table()
    by_pid = {r["pid"]: r for r in rows}
    ancestors = {os.getpid()}
    parent = os.getppid()
    while parent in by_pid and parent not in ancestors:
        ancestors.add(parent)
        parent = by_pid[parent]["ppid"]
    needles = (str(run_dir), manifest["label"], manifest["session_id"], manifest["workspace"])
    for row in rows:
        if row["pid"] in ancestors or row["state"].startswith("Z"):
            continue
        if any(value in row["command"] for value in needles):
            raise ValueError(f"Related process {row['pid']} is still live; no replacement is allowed")


def check_locks(run_dir, own_lock=None):
    candidates = list(run_dir.glob("*.lock")) + [run_dir / ".lock"]
    candidates += [parent / "campaign.lock" for parent in run_dir.parents]
    for path in candidates:
        if path.exists() and path != own_lock:
            raise ValueError(f"Existing lock requires inspection and is not removed: {path}")


def rendered(turn, label):
    return turn["user"].replace("{{run}}", label).replace(
        "{{fixture_dir}}", str(runner.ROOT / "docs/evals/anytype-mcp-v2/fixtures"))


def check_completed_events(path, session_id):
    records = events(path)
    require(records and records[-1].get("type") == "turn.completed", f"Partial turn: {path}")
    require(sum(e.get("type") == "turn.completed" for e in records) == 1,
            f"Ambiguous turn completion: {path}")
    identities = [e.get("thread_id") for e in records if e.get("type") == "thread.started"]
    require(identities == [session_id], f"Thread identity differs or is missing: {path}")
    require(not any(e.get("type") in {"error", "turn.failed"} for e in records), f"Failed turn: {path}")
    pending = set()
    calls = []
    for event in records:
        item = event.get("item", {})
        if item.get("type") != "mcp_tool_call":
            continue
        if event.get("type") in {"item.started", "item.updated"}:
            pending.add(item["id"])
        elif event.get("type") == "item.completed":
            pending.discard(item["id"])
            require(not item.get("error"), f"Tool transport outcome is uncertain: {path}")
            if item.get("status") != "completed":
                # A recorded 4xx tool refusal is a known response, unlike an
                # unfinished call, lost transport response or possible 5xx write.
                failures = list(documents(item.get("result") or {}))
                known_refusal = any(isinstance(d.get("status"), int) and 400 <= d["status"] < 500
                                    and d.get("code") for d in failures)
                # This captured capability refusal is read-only and explicit;
                # it cannot hide a committed write. Do not generalize to 5xx
                # responses from writes, unknown tools, or transport errors.
                schema_unavailable = item.get("tool") == "API-get-type-schema" and any(
                    d.get("status") == 501 and d.get("code") == "not_implemented" for d in failures)
                require(known_refusal or schema_unavailable, f"Failed tool outcome is uncertain: {path}")
            calls.append((item["tool"], item.get("arguments", {})))
    require(not pending, f"Unfinished tool call: {path}")
    return calls


def validate(run_dir, suite_path=FROZEN_SUITE, *, own_lock=None):
    run_dir = Path(run_dir).resolve(strict=True)
    check_locks(run_dir, own_lock)
    manifest = load(run_dir / "run.json")
    sid = manifest.get("scenario_id")
    require(sid in {"MCP-28", "MCP-30"}, "Only MCP-28 and MCP-30 fixture boundaries may resume")
    require(manifest.get("status") in {"running", "stopped_before_fixture_turn"},
            "Run did not stop at an eligible fixture boundary")
    require(manifest.get("session_id") and isinstance(manifest["session_id"], str), "Exact thread ID is required")
    require(manifest.get("reasoning") == "medium", "Stored reasoning differs from the runner configuration")
    require(run_dir.name == manifest.get("label"), "Run directory and original label disagree")
    require(Path(manifest.get("workspace", "")).is_dir(), "Original workspace is unavailable; it will not be replaced")
    check_processes(run_dir, manifest, load(run_dir / "active-process.json"))
    suite_path = Path(suite_path).resolve(strict=True)
    suite = load(suite_path)
    scenario, = [s for s in suite["scenarios"] if s["id"] == sid]
    current, = [s for s in load(runner.SUITE)["scenarios"] if s["id"] == sid]
    require(scenario == current, "Frozen scenario differs from the current scenario; refusing a mixed run")
    turns = manifest.get("turns", [])
    next_turn = len(turns) + 1
    kind = BOUNDARIES.get((sid, next_turn))
    require(kind is not None and not manifest.get("skipped_turns"), "Completed prefix does not end immediately before a fixture turn")
    scope = load(run_dir / "scope.json")
    require(len(scope.get("spaces", [])) == 1 and scope["spaces"] == manifest.get("owned_spaces"),
            "Original owned-space record is missing or inconsistent")
    request = load(run_dir / "fixture-request.json")
    require(all(request.get(key) == value for key, value in {
        "scenario_id": sid, "before_turn": next_turn, "kind": kind,
        "run_label": manifest["label"], "space_id": scope["spaces"][0]["full_id"],
        "status": "waiting_for_real_fixture"}.items()), "Fixture request does not match this exact boundary and run")
    fixture_records = load(run_dir / "fixture-evidence.json").get("fixtures", [])
    require(any(e.get("kind") == kind and e.get("before_turn") == next_turn for e in fixture_records),
            "No recorded pre-turn fixture attempt exists")
    completed_calls = []
    protected = []
    for index, record in enumerate(turns, 1):
        require(record.get("turn") == index and record.get("turn_completed") is True
                and record.get("exit_code") == 0 and record.get("timeout") is False,
                "Completed turn prefix contains a partial, failed or missing turn")
        prompt_path = run_dir / f"turn-{index:02d}.prompt.txt"
        prompt = prompt_path.read_text()
        expected = rendered(scenario["turns"][index-1], manifest["label"])
        require(prompt == record["user"] and (prompt == expected or index == 1 and prompt.endswith("\n\n" + expected)),
                "Completed prompt differs from its recorded/frozen scenario")
        completed_calls += check_completed_events(run_dir / f"turn-{index:02d}.events.jsonl", manifest["session_id"])
        for suffix in ("prompt.txt", "events.jsonl", "stderr.log", "final.txt"):
            path = run_dir / f"turn-{index:02d}.{suffix}"
            require(path.is_file(), f"Completed turn evidence is missing: {path}")
            protected.append(path)
    for path in list(run_dir.glob("turn-*")) + list((run_dir / "snapshots").glob("turn-*")):
        match = re.match(r"turn-(\d+)\.", path.name)
        require(match is not None and int(match[1]) < next_turn,
                f"Pending/later turn already has artifacts and may have committed: {path}")
        if path.is_file() and path not in protected:
            protected.append(path)
    # Wire calls must all belong to the completed event prefix and have replies.
    wire = events(run_dir / "mcp-wire.jsonl")
    pending = {}
    observed = []
    created_owned_space = False
    for event in wire:
        payload = event["payload"]
        direction = event["direction"]
        if direction == "from_model" and payload.get("method") == "tools/call":
            require(payload["id"] not in pending, "Overlapping wire call IDs prevent authoritative reconstruction")
            pending[payload["id"]] = payload
            observed.append((payload["params"]["name"], payload["params"].get("arguments", {})))
        elif direction == "from_upstream" and payload.get("id") in pending:
            original = pending[payload["id"]]
            if original["params"]["name"] == "API-create-space":
                created_owned_space |= any(d.get("id") == scope["spaces"][0]["full_id"]
                                           for d in documents(payload.get("result", {})))
        elif direction == "to_model":
            pending.pop(payload.get("id"), None)
    canonical = lambda values: Counter(json.dumps(v, sort_keys=True, ensure_ascii=False) for v in values)
    require(not pending and canonical(observed) == canonical(completed_calls),
            "Wire contains unfinished calls or calls outside the completed turn prefix")
    require(created_owned_space, "Owned space lacks its original upstream creation receipt")
    receipt_path = run_dir / f"fixture-{kind}-receipt.json"
    require(receipt_path.is_file(), "Supply the real fixture receipt before resuming")
    receipt = load(receipt_path)
    require(receipt.get("space_id") == request["space_id"], "Fixture receipt addresses another space")
    if receipt.get("status") == "fixture_unavailable":
        require(sid == "MCP-30" and kind in {"peer_history", "peer_arrival"}
                and receipt.get("user_authorized_skip") is True
                and isinstance(receipt.get("reason"), str) and receipt["reason"].strip()
                and receipt.get("scenario_id") == sid and receipt.get("kind") == kind
                and receipt.get("before_turn") == next_turn and receipt.get("run_label") == manifest["label"]
                and not receipt.get("peer_id"),
                "Unavailable fixture requires an explicit matching user-authorized MCP-30 peer skip, without a peer identity")
    else:
        required = ("object_id",) if kind == "desktop_provenance" else ("chat_id", "peer_id")
        require(all(isinstance(receipt.get(k), str) and receipt[k] for k in required), "Fixture receipt lacks real object/peer identifiers")
    return {"run_dir": run_dir, "manifest": manifest, "scenario": scenario, "next_turn": next_turn,
            "kind": kind, "suite_path": suite_path, "protected": protected}


@contextmanager
def exclusive_resume(run_dir):
    path = run_dir / "resume.lock"
    with path.open("x") as out:
        json.dump({"pid": os.getpid(), "started_at": datetime.now(timezone.utc).isoformat()}, out)
    try:
        yield path
    finally:
        path.unlink()


def execute_resume(plan, *, codex, upstream="anytype", turn_timeout=600):
    run_dir = plan["run_dir"]
    with exclusive_resume(run_dir) as lock:
        # Repeat all authoritative checks after claiming the exclusive lock.
        plan = validate(run_dir, plan["suite_path"], own_lock=lock)
        manifest = plan["manifest"]
        session_id = manifest["session_id"]
        protected = {p: p.read_bytes() for p in plan["protected"]}
        trace_prefixes = {p: p.read_bytes() for p in run_dir.glob("*-wire.jsonl")}
        history = manifest.setdefault("fixture_resumptions", [])
        archive = run_dir / "resumptions" / f"attempt-{len(history)+1:02d}"
        archive.mkdir(parents=True, exist_ok=False)
        for name in ("run.json", "active-process.json", "fixture-request.json", "fixture-evidence.json"):
            shutil.copyfile(run_dir / name, archive / name)
        history.append({"started_at": datetime.now(timezone.utc).isoformat(), "runner_pid": os.getpid(),
                        "from_turn": plan["next_turn"], "session_id": session_id,
                        "suite_path": str(plan["suite_path"]),
                        "suite_sha256": hashlib.sha256(plan["suite_path"].read_bytes()).hexdigest(),
                        "prior_metadata_archive": str(archive)})
        manifest.update(runner_pid=os.getpid(), status="running")
        runner.save_report(run_dir, manifest)
        flags = runner.configurations(run_dir, manifest["label"], codex, upstream, with_hooks=True)
        for turn in plan["scenario"]["turns"][plan["next_turn"]-1:]:
            number = turn["turn"]
            try:
                before_turn(run_dir, manifest, number, codex, upstream)
            except Exception as exc:
                manifest.update(status="stopped_before_fixture_turn", fixture_resume_error=str(exc))
                runner.save_report(run_dir, manifest)
                raise
            atomic_json(run_dir / "hook-config.json", {"scenario_id": manifest["scenario_id"],
                        "current_turn": number, "armed_profiles": []})
            prompt = rendered(turn, manifest["label"])
            (run_dir / f"turn-{number:02d}.prompt.txt").write_text(prompt)
            result = runner.execute(runner.command(codex, flags, manifest["model"], manifest["workspace"],
                                    run_dir / f"turn-{number:02d}.final.txt", session_id),
                                    prompt, run_dir / f"turn-{number:02d}.events.jsonl",
                                    run_dir / f"turn-{number:02d}.stderr.log", turn_timeout)
            recorded = events(run_dir / f"turn-{number:02d}.events.jsonl")
            ids = [e.get("thread_id") for e in recorded if e.get("type") == "thread.started"]
            completed = any(e.get("type") == "turn.completed" for e in recorded)
            record = {"turn": number, "user": prompt, **result, "turn_completed": completed,
                      "usage": next((e.get("usage") for e in reversed(recorded) if e.get("type") == "turn.completed"), None)}
            manifest["turns"].append(record)
            finalize_turn(run_dir, manifest, number)
            valid = result["exit_code"] == 0 and not result["timeout"] and completed and ids == [session_id]
            if not valid:
                manifest["status"] = "runtime_failed"
                if ids != [session_id]:
                    record["thread_identity_error"] = {"expected": session_id, "observed": ids}
            runner.save_report(run_dir, manifest)
            if not valid:
                return 1
            try:
                record["independent_snapshot_reads"] = snapshot(run_dir, codex, upstream, allow_running=True,
                    output_path=run_dir / "snapshots" / f"turn-{number:02d}.json")
            except Exception as exc:
                record["independent_snapshot_error"] = str(exc)
            require(all(p.read_bytes() == data for p, data in protected.items()), "Completed evidence changed during resumption")
            require(all(p.read_bytes().startswith(data) for p, data in trace_prefixes.items()), "A prior wire trace was overwritten")
            runner.save_report(run_dir, manifest)
        manifest["status"] = "conversation_completed_pending_review"
        runner.save_report(run_dir, manifest)
        if (run_dir / "final-state.json").exists():
            shutil.copyfile(run_dir / "final-state.json", archive / "final-state.json")
        try:
            snapshot(run_dir, codex, upstream)
        except Exception as exc:
            manifest["final_snapshot_error"] = str(exc)
            runner.save_report(run_dir, manifest)
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("run_dir", type=Path)
    parser.add_argument("--suite", type=Path, default=FROZEN_SUITE)
    parser.add_argument("--execute", action="store_true", help="Resume after local validation; default only inspects")
    parser.add_argument("--codex", default="codex")
    parser.add_argument("--upstream", default="anytype")
    parser.add_argument("--turn-timeout", type=int, default=600)
    args = parser.parse_args()
    plan = validate(args.run_dir, args.suite)
    print(json.dumps({"run_dir": str(plan["run_dir"]), "scenario_id": plan["manifest"]["scenario_id"],
                      "session_id": plan["manifest"]["session_id"], "resume_before_turn": plan["next_turn"],
                      "fixture_kind": plan["kind"], "mode": "execute" if args.execute else "validated_only"}))
    if args.execute:
        codex = shutil.which(args.codex)
        require(codex is not None, "Codex executable not found")
        return execute_resume(plan, codex=codex, upstream=args.upstream, turn_timeout=args.turn_timeout)
    return 0


if __name__ == "__main__":
    sys.exit(main())
