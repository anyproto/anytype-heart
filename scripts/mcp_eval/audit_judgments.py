#!/usr/bin/env python3
"""Audit completed offline judgments without changing jobs or contacting any service."""
import argparse
from collections import Counter
from contextlib import ExitStack
import fcntl
import json
from pathlib import Path

from review_codex import (MODEL, EFFORT, ROOT, digest, dump, now, save,
                         substantive_values, validate_result, validate_schema,
                         validation_feedback, verify_judge_execution)

CAMPAIGN = ROOT / "docs/evals/anytype-mcp-v2/artifacts/luna-three-runs"


class SourceIntegrityError(ValueError):
    def __init__(self, source_errors):
        super().__init__("original source provenance failed")
        self.source_errors = source_errors


def require(condition, message):
    if not condition:
        raise ValueError(message)


def load(path):
    return json.loads(path.read_text())


def events(path):
    # Unlike convenience transcript readers, an integrity audit must not skip bad JSON.
    result = [json.loads(line) for line in path.read_text().splitlines() if line.strip()]
    require(all(isinstance(event, dict) for event in result), f"non-object event in {path}")
    return result


def verify_runtime(execution, event_path, metadata_path, thread=None):
    captured_events, metadata = events(event_path), load(metadata_path)
    actual_thread = verify_judge_execution(execution, captured_events, metadata, thread)
    # Fail closed on unfamiliar item kinds: they cannot certify a tool-free judgment.
    allowed = {"agent_message", "reasoning", "error", "plan", "todo_list"}
    for event in captured_events:
        if "item" in event:
            require(event["item"].get("type") in allowed,
                    f"tool or unknown item kind: {event['item'].get('type')}")
        require("tool" not in event.get("type", "").lower(), "tool event in judge runtime")
    contexts = [entry for entry in metadata.get("rollout_metadata", []) if entry.get("type") == "turn_context"]
    require(bool(contexts), "actual model metadata lacks captured rollout turn_context")
    for context in contexts:
        require(context.get("model") == MODEL and context.get("effort", context.get("reasoning_effort")) == EFFORT,
                "captured rollout model/reasoning differs from gpt-6-astra/high")
    sessions = [entry for entry in metadata.get("rollout_metadata", []) if entry.get("type") == "session_meta"]
    require(bool(sessions) and all(entry.get("id") == actual_thread for entry in sessions),
            "captured rollout session ID differs from runtime thread")
    return actual_thread, metadata


def citation_only_failure(raw, bundle, schema):
    issues = validation_feedback(raw, bundle, schema)
    citation_only = (bool(issues) and any(issue["kind"] == "citation" for issue in issues) and
                     issues[0].get("message", "").startswith("invalid evidence reference:") and
                     not any(issue["kind"] == "consistency" for issue in issues))
    if citation_only:
        try:
            validate_schema(json.loads(raw), schema)
        except ValueError:
            return False
    return citation_only


def audit_completed(directory, state):
    bundle, schema = load(directory / "evidence.json"), load(directory / "result.schema.json")
    prompt = (directory / "judge-prompt.txt").read_text()
    frozen_hash = digest((dump(bundle) + prompt + dump(schema) + MODEL + EFFORT).encode())
    require(frozen_hash == state.get("input_sha256"), "frozen evidence/prompt/schema hash mismatch")
    for name, source in bundle["sources"].items():
        require(digest(source["text"].encode()) == source["sha256"], f"frozen source hash mismatch: {name}")
        require(len(source["text"].splitlines()) == source["line_count"], f"frozen source line count mismatch: {name}")
        included = source["included_lines"]
        require(all(type(n) is int and 1 <= n <= source["line_count"] for n in included),
                f"invalid included source line: {name}")
    originals = bundle.get("original_sha256")
    require(isinstance(originals, dict) and bool(originals), "missing original source hash manifest")
    source_errors = []
    for path, expected_hash in originals.items():
        try:
            actual_hash = digest(Path(path).read_bytes())
        except OSError as error:
            source_errors.append({"source": path, "status": "missing_or_unreadable",
                                  "expected_sha256": expected_hash, "error": str(error)})
            continue
        if actual_hash != expected_hash:
            source_errors.append({"source": path, "status": "hash_mismatch",
                                  "expected_sha256": expected_hash, "actual_sha256": actual_hash})
    if source_errors:
        raise SourceIntegrityError(source_errors)
    result = load(directory / "reviewer-result.json")
    validate_result(result, bundle, schema)
    require((state.get("run_id"), state.get("scenario_id")) == (result["run_id"], result["scenario_id"]),
            "job/result identity mismatch")
    thread, metadata = verify_runtime(state.get("execution"), directory / "judge.events.jsonl",
                                      directory / "model-metadata.json")
    original_raw = (directory / "judge-output.json").read_text()
    previous_raw = original_raw
    citation_only = citation_only_failure(original_raw, bundle, schema)
    original_values = substantive_values(json.loads(original_raw)) if citation_only else None
    unchanged = True if citation_only else None
    repairs = state.get("repairs", [])
    require([attempt["attempt"] for attempt in repairs] == list(range(1, len(repairs) + 1)) and len(repairs) <= 2,
            "repair attempts are not an ordered sequence of at most two")
    recorded_repairs = sorted((directory / "repairs").glob("*/repair.json"))
    require(len(recorded_repairs) == len(repairs), "untracked or missing repair records")
    for attempt in repairs:
        repair_dir = directory / "repairs" / f"{attempt['attempt']:02d}"
        record = load(repair_dir / "repair.json")
        require(record.get("attempt") == attempt["attempt"] and record.get("thread_id") == thread,
                "repair record attempt/thread mismatch")
        require(record.get("input_sha256") == frozen_hash, "repair frozen input hash mismatch")
        require(record.get("prompt_sha256") == digest((repair_dir / "prompt.txt").read_bytes()),
                "repair prompt hash mismatch")
        require(record.get("source_output_sha256") == digest(previous_raw.encode()),
                "repair source-output chain hash mismatch")
        command = record.get("command", [])
        require("resume" in command and command[command.index("resume") + 1:command.index("resume") + 2] == [thread],
                "repair command does not resume the exact initial thread")
        verify_runtime(record.get("execution"), repair_dir / "events.jsonl", repair_dir / "model-metadata.json", thread)
        previous_raw = (repair_dir / "output.json").read_text()
        if citation_only:
            require(substantive_values(json.loads(previous_raw)) == original_values,
                    "citation-only repair changed substantive fields")
        require(record.get("status") in {"invalid", "complete"}, "repair runtime is not terminal and successful")
    expected_output = directory / (f"repairs/{len(repairs):02d}/output.json" if repairs else "judge-output.json")
    accepted = Path(state.get("accepted_output", str(expected_output))).resolve()
    require(accepted == expected_output.resolve(), "accepted output is not the final recorded attempt")
    require(load(accepted) == result, "accepted output differs from validated reviewer-result.json")
    if repairs:
        require(load(recorded_repairs[-1]).get("status") == "complete", "accepted repair is not complete")
    return {"run_id": result["run_id"], "scenario_id": result["scenario_id"], "thread_id": thread,
            "observed_model": metadata["observed_model"], "reasoning": metadata["observed_reasoning_effort"],
            "frozen_sources_verified": len(bundle["sources"]), "original_sources_verified": len(originals),
            "repairs": len(repairs),
            "citation_only_repairs": citation_only if repairs else False,
            "substantive_fields_unchanged": unchanged,
            "schema_and_citations_valid": True, "accepted_output": str(accepted)}


def audit(reviews):
    runs, errors, skipped = [], [], []
    completed_count = 0
    for job_path in sorted((reviews / "runs").glob("*/job.json")):
        try:
            with ExitStack() as stack:
                lock_path = job_path.parent / ".lock"
                if lock_path.exists():
                    lock = stack.enter_context(lock_path.open("r"))
                    try:
                        fcntl.flock(lock, fcntl.LOCK_SH | fcntl.LOCK_NB)
                    except BlockingIOError:
                        skipped.append({"job": str(job_path), "status": "active", "reason": "judge holds exclusive lock"})
                        continue
                before = job_path.read_bytes()
                state = json.loads(before)
                if state.get("status") != "complete":
                    skipped.append({"job": str(job_path), "status": state.get("status", "unknown"),
                                    "reason": "only completed jobs are audited"})
                    continue
                completed_count += 1
                result = audit_completed(job_path.parent, state)
                require(job_path.read_bytes() == before, "job changed during audit")
                runs.append(result)
        except (ValueError, OSError, KeyError, TypeError, IndexError) as error:
            entry = {"job": str(job_path), "error": str(error)}
            if isinstance(error, SourceIntegrityError):
                entry["source_errors"] = error.source_errors
            errors.append(entry)
    threads = Counter(run["thread_id"] for run in runs)
    for thread, count in threads.items():
        if count > 1:
            errors.append({"thread_id": thread, "error": "initial thread reused across completed judgments",
                           "run_ids": [run["run_id"] for run in runs if run["thread_id"] == thread]})
    return {"recorded_at": now(), "reviews": str(reviews.resolve()),
            "basis": "Frozen embedded source bytes, saved input hash, raw Codex events, and captured rollout session/turn_context metadata. Original local source files are separately checked against original_sha256 for provenance; their current content never replaces the frozen adjudication evidence.",
            "summary": {"audited_completed_reviews": completed_count, "verified_reviews": len(runs),
                        "errors": len(errors), "distinct_initial_threads": len(threads),
                        "repairs": sum(run["repairs"] for run in runs), "skipped_jobs": len(skipped)},
            "runs": runs, "errors": errors, "skipped": skipped}


def validate_output_path(reviews, output):
    """Prevent the report or its atomic-write temporary file overwriting inputs."""
    targets = [output.resolve(), output.with_suffix(output.suffix + ".tmp").resolve()]
    protected_dirs = [reviews.resolve()]
    protected_files = set()
    for job_path in (reviews / "runs").glob("*/job.json"):
        state = load(job_path)
        if state.get("run"):
            protected_dirs.append(Path(state["run"]).resolve())
        evidence_path = job_path.parent / "evidence.json"
        if evidence_path.exists():
            protected_files.update(Path(path).resolve() for path in load(evidence_path).get("original_sha256", {}))
    require(not any(target in protected_files or any(target.is_relative_to(directory) for directory in protected_dirs)
                    for target in targets), "audit output must not overwrite review artifacts or original evidence")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--reviews", type=Path, default=CAMPAIGN / "astra-reviews")
    parser.add_argument("--output", type=Path, help="Default: judgment-audit.json beside the reviews directory")
    args = parser.parse_args()
    output = (args.output or args.reviews.parent / "judgment-audit.json").resolve()
    try:
        validate_output_path(args.reviews, output)
    except ValueError as error:
        parser.error(str(error))
    report = audit(args.reviews)
    output.parent.mkdir(parents=True, exist_ok=True)
    save(output, report)
    print(json.dumps(report["summary"]))
    return 1 if report["errors"] else 0


if __name__ == "__main__":
    raise SystemExit(main())
