#!/usr/bin/env python3
"""Check one exact Astra capacity failure; --execute archives it and retries once.

The default is read-only. Execution requires every other review job to be idle.
Exactly one fresh initial model call is made, without a model fallback. A successful
runtime whose JSON/citations need correction is left failed at stage=validation;
use the normal review_codex.py --retry-failed flow to repair that SAME fresh thread.
"""
import argparse
from contextlib import contextmanager
import fcntl
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

from audit_judgments import events, load, require
import review_codex as review

CAPACITY = "Selected model is at capacity. Please try a different model."
def pid_alive(pid):
    require(type(pid) is int and pid > 0, "invalid recorded subprocess PID")
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    return True


@contextmanager
def locked(directory):
    # The existing lock inode remains in the canonical directory during archival.
    with (directory / ".lock").open("r") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise ValueError("original judgment lock is held")
        yield


def check(directory):
    state = load(directory / "job.json")
    execution = state.get("execution", {})
    require(state.get("status") == "failed" and execution.get("exit_code") == 1 and execution.get("timeout") is False,
            "requires a failed, non-timeout CLI runtime with exit code 1")
    require(state.get("model") == review.MODEL and state.get("reasoning") == review.EFFORT,
            "original requested model/reasoning differs from gpt-6-astra/high")
    require(not state.get("repairs"), "capacity retry is not allowed after structural repair attempts")
    process = load(directory / "active-process.json")
    require(process.get("state") == "exited" and process.get("exit_code") == 1,
            "original subprocess is not recorded as exited with code 1")
    require(not pid_alive(process.get("pid")), "original subprocess PID is still alive")
    captured = events(directory / "judge.events.jsonl")
    threads = [event.get("thread_id") for event in captured if event.get("type") == "thread.started"]
    require(len(threads) == 1 and bool(threads[0]), "requires exactly one recorded initial thread")
    capacity_seen = False
    for event in captured:
        require("usage" not in event, "model usage exists; this is not an empty capacity failure")
        kind = event.get("type")
        require(kind in {"thread.started", "turn.started", "error", "turn.failed", "item.completed"},
                "unexpected event or successful response in failed runtime")
        if kind in {"error", "turn.failed"}:
            message = event.get("message") if kind == "error" else event.get("error", {}).get("message")
            require(message == CAPACITY, "runtime error is not the exact allowed capacity error")
            capacity_seen = True
        elif kind == "item.completed":
            item = event.get("item", {})
            require(item.get("type") == "error", "assistant response or tool activity exists")
            require(item.get("message", "").startswith(("Under-development features enabled:",
                                                         "Code Mode is unavailable because code-mode host is disabled.")),
                    "unrecognized runtime item error")
    require(capacity_seen, "exact capacity error is absent")
    metadata = load(directory / "model-metadata.json")
    require(not metadata.get("usage"), "recorded model usage exists")
    require(metadata.get("observed_model") == review.MODEL and metadata.get("observed_reasoning_effort") == review.EFFORT
            and metadata.get("thread_id") == threads[0], "failed runtime model/reasoning/thread metadata differs")
    for name in ("judge-output.json", "reviewer-result.json"):
        require(not (directory / name).exists() or not (directory / name).read_text().strip(),
                "model judgment output already exists")
    bundle, schema = load(directory / "evidence.json"), load(directory / "result.schema.json")
    prompt = (directory / "judge-prompt.txt").read_text()
    frozen_hash = review.digest((review.dump(bundle) + prompt + review.dump(schema) + review.MODEL + review.EFFORT).encode())
    require(frozen_hash == state.get("input_sha256"), "frozen input hash mismatch")
    for name, source in bundle["sources"].items():
        require(review.digest(source["text"].encode()) == source["sha256"], f"frozen source hash mismatch: {name}")
    require(bool(bundle.get("original_sha256")), "original source hash manifest is absent")
    for path, expected in bundle["original_sha256"].items():
        require(review.digest(Path(path).read_bytes()) == expected, f"original source hash mismatch: {path}")
    require((state.get("run_id"), state.get("scenario_id")) == (bundle["run_id"], bundle["scenario_id"]),
            "job/frozen input identity mismatch")
    cmd = state.get("command", [])
    require(bool(cmd) and cmd == review.command(cmd[0], Path(state["workspace"]), directory),
            "original command differs from the fixed tool-free Astra/high configuration")
    return {"eligible": True, "run_id": state["run_id"], "scenario_id": state["scenario_id"],
            "input_sha256": frozen_hash, "failed_thread_id": threads[0], "exited_child_pid": process["pid"],
            "capacity_error": CAPACITY, "original_sources_verified": len(bundle["original_sha256"])}


def active_jobs(directory):
    blockers = []
    for path in sorted(directory.parent.glob("*/job.json")):
        if path.parent == directory:
            continue
        state = load(path)
        process_path = path.parent / "active-process.json"
        process = load(process_path) if process_path.exists() else {}
        busy = state.get("status") == "running"
        if process.get("state") == "running" and process.get("pid"):
            busy = busy or pid_alive(process["pid"])
        lock_path = path.parent / ".lock"
        if lock_path.exists():
            with lock_path.open("r") as lock:
                try:
                    fcntl.flock(lock, fcntl.LOCK_SH | fcntl.LOCK_NB)
                except BlockingIOError:
                    busy = True
        if busy:
            blockers.append(str(path.parent))
    return blockers


def file_hashes(directory):
    result = {}
    for path in sorted(directory.rglob("*")):
        require(not path.is_symlink(), f"unexpected symlink in failed job: {path}")
        if path.is_file():
            result[str(path.relative_to(directory))] = review.digest(path.read_bytes())
    return result


def run(directory, execute=False, codex="codex", timeout=1800):
    directory = directory.resolve()
    require(directory.parent.name == "runs", "expected canonical astra-reviews/runs/RUN directory")
    with locked(directory):
        receipt = check(directory)
        blockers = active_jobs(directory)
        receipt["execution_blockers"] = blockers
        if not execute:
            return {"mode": "check_only", **receipt}
        require(not blockers, "other review jobs are active; wait for the dispatcher to finish before --execute")
        binary = shutil.which(codex)
        require(binary is not None, "Codex executable not found")
        before = file_hashes(directory)
        archive_root = directory.parent.parent / "infrastructure-failures" / directory.name
        archive_root.mkdir(parents=True, exist_ok=True)
        index = 1
        while (archive_root / f"attempt{index:02d}").exists():
            index += 1
        archive = archive_root / f"attempt{index:02d}"
        shutil.copytree(directory, archive)
        require(file_hashes(archive) == before and file_hashes(directory) == before,
                "failed job changed or archive verification failed")
        receipt.update(recorded_at=review.now(), archive=str(archive), failed_job_file_sha256=before)
        review.save(archive / "failure-receipt.json", receipt)
        old = load(directory / "job.json")
        bundle, schema = load(directory / "evidence.json"), load(directory / "result.schema.json")
        prompt = (directory / "judge-prompt.txt").read_text()
        workspace = Path(tempfile.mkdtemp(prefix="anytype-capacity-retry-"))
        cmd = review.command(binary, workspace, directory)
        state = {key: old[key] for key in ("run", "run_id", "scenario_id", "input_sha256", "model", "reasoning", "prompt_bytes")}
        state.update(status="running", prepared_at=old.get("prepared_at"), started_at=review.now(), pid=os.getpid(),
                     workspace=str(workspace), command=cmd,
                     codex_version=subprocess.check_output([binary, "--version"], text=True).strip(),
                     infrastructure_retry={"failure_receipt": str(archive / "failure-receipt.json"),
                                           "failed_thread_id": receipt["failed_thread_id"], "reason": CAPACITY})
        review.save(directory / "job.json", state)
        try:
            execution = review.execute(cmd, prompt, directory / "judge.events.jsonl", directory / "judge.stderr.log", timeout)
            state["execution"] = execution
            captured = events(directory / "judge.events.jsonl")
            metadata = review.model_metadata(captured)
            review.save(directory / "model-metadata.json", metadata)
            thread = review.verify_judge_execution(execution, captured, metadata)
            require(thread != receipt["failed_thread_id"], "capacity retry reused the failed initial thread")
            raw = (directory / "judge-output.json").read_text()
            issues = review.validation_feedback(raw, bundle, schema)
            if issues:
                review.save(directory / "validation-errors.json", issues)
                state["stage"] = "validation"
                raise ValueError("new judgment requires same-thread structural repair; use review_codex.py --retry-failed")
            result = json.loads(raw)
            review.validate_result(result, bundle, schema)
            review.save(directory / "reviewer-result.json", result)
            state.update(status="complete", completed_at=review.now())
        except BaseException as error:
            state.update(status="failed", error=str(error), failed_at=review.now())
            raise
        finally:
            review.save(directory / "job.json", state)
        return {"mode": "executed_once", "status": state["status"], **receipt}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    parser.add_argument("--execute", action="store_true", help="Explicitly archive and make one fresh review attempt after the dispatcher finishes")
    parser.add_argument("--codex", default="codex")
    parser.add_argument("--timeout", type=int, default=1800)
    args = parser.parse_args()
    try:
        print(json.dumps(run(args.directory, args.execute, args.codex, args.timeout), indent=2))
    except (ValueError, OSError, KeyError) as error:
        parser.exit(1, str(error) + "\n")


if __name__ == "__main__":
    main()
