#!/usr/bin/env python3
"""Summarize Anytype MCP evaluation traces without contacting Anytype.

The input run directories and their raw traces are never modified. Reports contain
sanitized arguments and references to source files rather than copied responses.
"""
import argparse
from collections import Counter
import json
from pathlib import Path
import re
import sys

DEFAULT_ROOT = Path("/private/tmp/anytype-mcp-eval-runs")
SURFACE = Path(__file__).resolve().parents[2] / "docs/evals/anytype-mcp-v2/surface-snapshot.json"
SCOPED = set("""add_chat_message create_chat create_collection create_object
create_property create_query create_template create_type delete_chat_message
delete_object delete_property delete_type download_file edit_chat_message
get_chat_messages get_collection_objects get_collection_views get_member_me
get_object get_query_objects get_query_views get_space get_type get_type_schema
list_chats list_members list_objects list_properties list_property_options
list_types patch_object read_chat search_space toggle_chat_reaction update_property
update_space update_type upload_file""".split())
SECRET_KEYS = re.compile(r"(authorization|bearer|password|passwd|secret|(?:access|refresh|auth)[_-]?token|credential)", re.I)


def load_json(path):
    try:
        return json.loads(path.read_text())
    except (OSError, ValueError):
        return None


def events(path):
    try:
        lines = path.read_text().splitlines()
    except OSError:
        return []
    out = []
    for n, line in enumerate(lines, 1):
        try:
            out.append((n, json.loads(line)))
        except ValueError:
            pass
    return out


def clean(value, key=""):
    if SECRET_KEYS.search(key):
        return "[REDACTED]"
    if isinstance(value, dict):
        return {k: clean(v, k) for k, v in value.items()}
    if isinstance(value, list):
        return [clean(v, key) for v in value]
    if isinstance(value, str):
        value = re.sub(r"(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+", "Bearer [REDACTED]", value)
        return value
    return value


def normalized(name):
    return name.removeprefix("mcp__anytype__").replace("-", "_").removeprefix("API_").lower()


def text_objects(result):
    for part in (result or {}).get("content", []) or []:
        if part.get("type") == "text":
            try:
                value = json.loads(part.get("text", ""))
            except (ValueError, TypeError):
                continue
            if isinstance(value, dict):
                yield value


def error_info(call):
    result = call.get("result") or {}
    nested = next((x for x in text_objects(result) if "status" in x or "code" in x or "message" in x), {})
    err = call.get("error") or {}
    return {
        "code": nested.get("code") or err.get("code") or (nested.get("status") if nested else None),
        "message": nested.get("message") or err.get("message") or "tool call failed",
    }


def is_failed(call):
    result = call.get("result") or {}
    return call.get("status") == "failed" or bool(call.get("error")) or bool(result.get("isError"))


def successful_create(call):
    if normalized(call.get("tool", "")) != "create_space" or is_failed(call):
        return False
    if (call.get("arguments") or {}).get("dry_run") is True:
        return False
    return any(d.get("id") and not d.get("dry_run") for d in text_objects(call.get("result")))


def call_records(run):
    records = []
    for p in sorted(run.glob("turn-*.events.jsonl")):
        for line, event in events(p):
            if event.get("type") != "item.completed":
                continue
            item = event.get("item") or {}
            if item.get("type") == "mcp_tool_call":
                turn_match = re.match(r"turn-(\d+)", p.name)
                records.append({"turn": int(turn_match.group(1)) if turn_match else None,
                                "source": f"{p.name}:{line}", **item})
    # Older runner output also has a convenience trace; use it only if events are absent.
    if not records:
        for line, item in events(run / "tool-calls.jsonl"):
            if isinstance(item, dict):
                records.append({"source": f"tool-calls.jsonl:{line}", **item})
    return records


def wire_audit(run, scope):
    owned = {x.get("full_id") for x in scope.get("spaces", [])} | {x.get("compact_id") for x in scope.get("spaces", [])}
    violations = []
    forwarded = 0
    if not (run / "mcp-wire.jsonl").exists():
        return {"status": "unavailable", "forwarded_scoped_calls": 0, "violations": [], "passed": None}
    for line, e in events(run / "mcp-wire.jsonl"):
        if e.get("direction") != "to_upstream":
            continue
        payload = e.get("payload") or {}
        if payload.get("method") != "tools/call":
            continue
        params = payload.get("params") or {}
        tool = normalized(params.get("name", ""))
        args = params.get("arguments") or {}
        if tool in SCOPED:
            forwarded += 1
            if args.get("space_id") not in owned:
                violations.append({"source": f"mcp-wire.jsonl:{line}", "tool": params.get("name"),
                                   "space_id": args.get("space_id"), "reason": "forwarded scoped call is outside scope.json"})
    status = "not_exercised" if forwarded == 0 else ("pass" if not violations else "fail")
    return {"status": status, "forwarded_scoped_calls": forwarded, "violations": violations,
            "passed": not violations if forwarded else None}


def summarize(run):
    manifest = load_json(run / "run.json") or {}
    scope = load_json(run / "scope.json") or {"spaces": []}
    calls = call_records(run)
    turns = manifest.get("turns") or []
    failed = []
    counts = Counter()
    statuses = Counter()
    successful_creates = previews = 0
    env_errors = []
    guard_errors = []
    validation_issues = []
    failed_by_api = Counter()
    for c in calls:
        tool = c.get("tool", "unknown")
        counts[tool] += 1
        statuses[c.get("status", "unknown")] += 1
        if normalized(tool) == "create_space" and (c.get("arguments") or {}).get("dry_run") is True:
            previews += 1
        if successful_create(c):
            successful_creates += 1
        if normalized(tool) == "validate":
            for document in text_objects(c.get("result")):
                if document.get("issues"):
                    validation_issues.append({"turn": c.get("turn"),
                                              "result_ref": c.get("source"),
                                              "issues": document["issues"]})
        if is_failed(c):
            info = error_info(c)
            safe_args = {} if normalized(tool) == "auth_whoami" else clean(c.get("arguments") or {})
            entry = {"turn": c.get("turn"), "tool": tool, "args": safe_args,
                     "code": info["code"], "message": info["message"], "result_ref": c.get("source")}
            failed.append(entry)
            failed_by_api[f"{info['code']}: {info['message']}"] += 1
            blob = f"{info['code']} {info['message']}".lower()
            if "approval" in blob or "environment" in blob or "approval policy" in blob:
                env_errors.append(entry)
            if info["code"] == "evaluation_space_guard" or "space guard" in blob:
                guard_errors.append(entry)
    usage = next((t.get("usage") for t in reversed(turns) if t.get("usage")), None)
    completed_turns = sum(bool(t.get("turn_completed")) for t in turns)
    duration = round(sum(float(t.get("duration_seconds", 0) or 0) for t in turns), 2)
    verdict = manifest.get("independent_verdict") or manifest.get("verdict")
    reviewer_result = load_json(run / "reviewer-result.json")
    if reviewer_result is not None and not verdict:
        verdict = reviewer_result
    if not verdict:
        verdict = {"available": False}
    verification_evidence = {name: {"present": (run / name).exists()} for name in
                             ("final-state.json", "reviewer-snapshot.json", "reviewer-result.json")}
    infrastructure_failure = bool(env_errors)
    return {
        "run_id": manifest.get("label", run.name), "scenario_id": manifest.get("scenario_id"),
        "model": manifest.get("model"), "reasoning": manifest.get("reasoning"),
        "hook": manifest.get("hook"), "hook_status": manifest.get("hook_status"),
        "completion": {"status": manifest.get("status"),
                       "effective_classification": "infrastructure_failure" if infrastructure_failure else "runner_completion",
                       "turns_completed": completed_turns,
                       "turns_total": len(turns), "runner_completed": manifest.get("status") == "conversation_completed_pending_review"},
        "independent_verdict": verdict, "verification_evidence": verification_evidence,
        "turns": {"count": len(turns), "completed": completed_turns, "duration_seconds": duration},
        "usage_last_turn": clean(usage or {}), "tool_calls": {"total": len(calls), "by_tool": dict(sorted(counts.items())), "by_status": dict(statuses)},
        "create_space": {"successful_actual": successful_creates, "previews": previews},
        "failed_calls": failed, "failed_by_api": dict(failed_by_api),
        "environment_errors": env_errors, "guard_errors": guard_errors,
        "validation_issue_calls": validation_issues,
        "scope_audit": wire_audit(run, scope),
        "source": {"run_dir": str(run), "raw_traces_unchanged": True},
    }


def markdown(s):
    run_dir = Path(s["source"]["run_dir"])
    evidence_links = [f"[{name}]({run_dir / name})" for name in
                      ["transcript.md", "tool-calls.jsonl", "final-state.json", "reviewer-result.json"]
                      if (run_dir / name).exists()]
    lines = [f"# Anytype MCP trace summary: {s['run_id']}", "", f"Model: `{s['model']}`; reasoning: `{s['reasoning']}`.", "",
             "Evidence: " + ", ".join(evidence_links) + ".", "",
             f"Runner completion: `{s['completion']['status']}` ({s['completion']['turns_completed']}/{s['completion']['turns_total']} turns).",
             f"Independent verdict evidence: `{json.dumps(s['independent_verdict'], ensure_ascii=False)}`.", "",
             f"Calls: {s['tool_calls']['total']}; duration: {s['turns']['duration_seconds']}s; successful create_space: {s['create_space']['successful_actual']}; previews: {s['create_space']['previews']}.", "",
             "## Tool counts", "", "| Tool | Count |", "| --- | ---: |"]
    lines += [f"| `{k}` | {v} |" for k, v in s["tool_calls"]["by_tool"].items()]
    lines += ["", f"Scope audit: `{s['scope_audit']['status']}`; forwarded scoped calls: {s['scope_audit']['forwarded_scoped_calls']}.",
              f"Errors: {len(s['failed_calls'])}; environment: {len(s['environment_errors'])}; guard: {len(s['guard_errors'])}.", "",
              "## Failed calls", "", "| Turn | Tool | Code | Message | Result ref |", "| ---: | --- | --- | --- | --- |"]
    for e in s["failed_calls"]:
        lines.append(f"| {e['turn']} | `{e['tool']}` | `{e['code']}` | {str(e['message']).replace('|','\\|')} | `{e['result_ref']}` |")
    if not s["failed_calls"]:
        lines.append("| — | — | — | none | — |")
    lines += ["", "Semantic success is not inferred from completion; independent verification is reported separately.", ""]
    return "\n".join(lines)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--runs", type=Path, default=DEFAULT_ROOT)
    p.add_argument("--output", type=Path, required=True)
    p.add_argument("run", nargs="*", help="run directory names (default: all run-* directories)")
    a = p.parse_args()
    dirs = [a.runs / x for x in a.run] if a.run else sorted(x for x in a.runs.iterdir() if x.is_dir() and (x / "run.json").exists())
    a.output.mkdir(parents=True, exist_ok=True)
    summaries = []
    for d in dirs:
        if not d.is_dir():
            raise SystemExit(f"run directory not found: {d}")
        s = summarize(d); summaries.append(s)
        (a.output / f"{s['run_id']}.json").write_text(json.dumps(s, ensure_ascii=False, indent=2) + "\n")
        (a.output / f"{s['run_id']}.md").write_text(markdown(s))
    declared = load_json(SURFACE) or {}
    declared_tools = [x.get("name") for x in declared.get("tools", []) if x.get("name")]
    attempted_counts = Counter()
    successful_counts = Counter()
    for run_dir in dirs:
        for call in call_records(run_dir):
            name = normalized(call.get("tool", ""))
            attempted_counts[name] += 1
            if not is_failed(call):
                successful_counts[name] += 1
    coverage = {}
    for name in declared_tools:
        coverage[name] = {"attempted": attempted_counts[normalized(name)],
                          "successful": successful_counts[normalized(name)]}
    coverage_summary = {"declared_tools": len(declared_tools), "attempted_tools": sum(v["attempted"] > 0 for v in coverage.values()),
                        "successful_tools": sum(v["successful"] > 0 for v in coverage.values()), "tools": coverage,
                        "source": str(SURFACE)}
    (a.output / "summary.json").write_text(json.dumps({"runs": summaries, "tool_coverage": coverage_summary}, ensure_ascii=False, indent=2) + "\n")
    index = ["# Anytype MCP trace summaries", "", "Runner status records delivery of the conversation; the review outcome assesses its result. This index includes diagnostic launches, which must be excluded from benchmark totals.", "", "| Run | Review outcome | Runner status | Hook | Calls | Errors | Scope audit |", "| --- | --- | --- | --- | ---: | ---: | --- |"]
    index += [f"| [{s['run_id']}]({s['run_id']}.md) | `{s['independent_verdict'].get('outcome', 'pending')}` | `{s['completion']['status']}` | `{s['hook_status'] or 'none'}` | {s['tool_calls']['total']} | {len(s['failed_calls'])} | `{s['scope_audit']['status']}` |" for s in summaries]
    index += ["", f"Measured API coverage ({coverage_summary['declared_tools']} declared): {coverage_summary['attempted_tools']} attempted; {coverage_summary['successful_tools']} successful.", "", "| API tool | Attempted | Successful |", "| --- | ---: | ---: |"]
    index += [f"| `{name}` | {v['attempted']} | {v['successful']} |" for name, v in coverage.items()]
    (a.output / "index.md").write_text("\n".join(index) + "\n")
    print(f"summarized {len(summaries)} run(s) into {a.output}")


if __name__ == "__main__":
    main()
