#!/usr/bin/env python3
"""Run staged Anytype scenarios in isolated, resumable host sessions.

A host is the agent CLI that drives the model (hosts.py): Codex (`codex
exec`) or Claude Code (`claude -p`). The upstream is the bridge (a stdio
server configured in Codex) or heart's /mcp/full over HTTP, reached through
the same space guard either way."""
import argparse
import collections
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import tempfile
import time

ROOT = Path(__file__).resolve().parents[2]
SUITE = ROOT / "docs/evals/anytype-mcp-v2/scenarios.json"
GUARD = Path(__file__).with_name("space_guard.py")
DISABLED = ["shell_tool", "unified_exec", "apps", "plugins", "hooks", "multi_agent",
            "browser_use", "browser_use_external", "computer_use", "in_app_browser",
            "image_generation", "memories", "view_image"]


def read_events(path):
    result = []
    for line in path.read_text().splitlines():
        try:
            result.append(json.loads(line))
        except ValueError:
            continue
    return result


def guard_args(run_dir, label, codex, upstream, with_hooks=False, upstream_url=None, arm="full",
               trace_name="mcp-wire.jsonl"):
    """The space guard's command line (after the interpreter)."""
    args = [str(GUARD), "--state", str(run_dir / "scope.json"),
            "--trace", str(run_dir / trace_name), "--run-label", label]
    if upstream_url:
        args += ["--upstream-url", upstream_url, "--arm", arm,
                 "--stats", str(run_dir / "tools-list-stats.json")]
    else:
        args += ["--codex", codex, "--upstream", upstream]
    if with_hooks:
        args += ["--hook-config", str(run_dir / "hook-config.json")]
    return args


def configurations(run_dir, label, codex, upstream, with_hooks=False, upstream_url=None, arm="full",
                   reasoning="medium"):
    proxy_args = guard_args(run_dir, label, codex, upstream, with_hooks, upstream_url, arm)
    values = {
        "model_reasoning_effort": reasoning, "web_search": "disabled",
        "features.skip_host_skill_discovery": True,
        "mcp_servers.anytype.command": sys.executable,
        "mcp_servers.anytype.args": proxy_args,
        "mcp_servers.anytype.required": True,
        # Only this proxy is pre-authorized. Its allowlist prevents calls into
        # existing spaces; all other integrations remain disabled.
        "mcp_servers.anytype.default_tools_approval_mode": "approve",
        "mcp_servers.anytype.startup_timeout_sec": 90,
        "mcp_servers.anytype.tool_timeout_sec": 180,
    }
    if upstream_url:
        # the HTTP upstream's key reaches the guard from the environment,
        # never through argv or a config file the model could see
        values["mcp_servers.anytype.env_vars"] = ["ANYTYPE_API_KEY"]
    flags = []
    for feature in DISABLED:
        flags += ["--disable", feature]
    for key, value in values.items():
        flags += ["-c", key + "=" + json.dumps(value)]
    return flags


def command(codex, flags, model, workspace, output, session_id=None):
    cmd = [codex, "exec", "--ignore-user-config", "--json",
           "--skip-git-repo-check", "-s", "read-only", "-C", str(workspace),
           "-m", model, *flags]
    if session_id:
        cmd += ["resume", session_id]
    cmd += ["-o", str(output), "-"]
    return cmd


def execute(cmd, prompt, event_path, err_path, timeout, cwd=None, env=None):
    start = time.monotonic()
    with open(event_path, "w") as out, open(err_path, "w") as err:
        proc = subprocess.Popen(cmd, stdin=subprocess.PIPE, stdout=out, stderr=err,
                                text=True, start_new_session=True, cwd=cwd, env=env)
        (event_path.parent / "active-process.json").write_text(json.dumps({
            "pid": proc.pid, "started_at": datetime.now(timezone.utc).isoformat(),
            "event_path": str(event_path), "state": "running"}) + "\n")
        try:
            proc.communicate(prompt, timeout=timeout)
        except subprocess.TimeoutExpired:
            os.killpg(proc.pid, signal.SIGTERM)
            try:
                proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                os.killpg(proc.pid, signal.SIGKILL)
                proc.wait()
            (event_path.parent / "active-process.json").write_text(json.dumps({
                "pid": proc.pid, "state": "exited", "timeout": True,
                "exit_code": proc.returncode}) + "\n")
            return {"exit_code": proc.returncode, "timeout": True,
                    "duration_seconds": round(time.monotonic() - start, 2)}
        except BaseException:
            os.killpg(proc.pid, signal.SIGTERM)
            proc.wait(timeout=10)
            raise
    (event_path.parent / "active-process.json").write_text(json.dumps({
        "pid": proc.pid, "state": "exited", "exit_code": proc.returncode}) + "\n")
    return {"exit_code": proc.returncode, "timeout": False,
            "duration_seconds": round(time.monotonic() - start, 2)}


def save_report(run_dir, manifest):
    messages = [f"# {manifest.get('host', 'codex').capitalize()} evaluation: " + manifest["scenario_id"], "",
                "Model: `" + manifest["model"] + "`; reasoning: `" + str(manifest.get("reasoning")) + "`.", "",
                "This transcript is execution evidence, not an independent pass verdict.", ""]
    calls = []
    for turn in manifest["turns"]:
        number = turn["turn"]
        messages += [f"## User turn {number}", "", turn["user"], ""]
        events = read_events(run_dir / f"turn-{number:02d}.events.jsonl")
        for e in events:
            if e.get("type") == "item.completed":
                item = e.get("item", {})
                if item.get("type") == "mcp_tool_call":
                    calls.append({"turn": number, **item})
                    messages += ["### Tool: " + item.get("tool", "unknown"), "", "```json",
                                 json.dumps(item, ensure_ascii=False, indent=2), "```", ""]
                elif item.get("type") == "agent_message":
                    messages += ["### Assistant", "", item.get("text", ""), ""]
            elif e.get("type") in {"error", "turn.failed"}:
                messages += ["### Runtime error", "", "```json", json.dumps(e), "```", ""]
    (run_dir / "transcript.md").write_text("\n".join(messages))
    (run_dir / "tool-calls.jsonl").write_text("".join(json.dumps(c, ensure_ascii=False) + "\n" for c in calls))
    counts = collections.Counter(c.get("tool") for c in calls)
    manifest["tool_call_count"] = len(calls)
    manifest["tool_counts"] = dict(counts)
    manifest["tool_errors"] = sum(bool(c.get("error")) or c.get("status") == "failed" or bool((c.get("result") or {}).get("isError")) for c in calls)
    scope = run_dir / "scope.json"
    manifest["owned_spaces"] = json.loads(scope.read_text())["spaces"] if scope.exists() else []
    (run_dir / "run.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")


def run(args):
    from hosts import make_host
    suite = json.loads(SUITE.read_text())
    scenarios = {c["id"]: c for c in suite["scenarios"]}
    selected = list(scenarios) if args.scenario == ["all"] else args.scenario
    unknown = set(selected) - scenarios.keys()
    if unknown:
        raise ValueError("Unknown scenarios: " + ", ".join(sorted(unknown)))
    codex = shutil.which(args.codex)
    if not codex and (args.host == "codex" or not args.upstream_url):
        raise ValueError("Codex executable not found")
    if args.upstream_url and not os.environ.get("ANYTYPE_API_KEY"):
        raise ValueError("--upstream-url needs the API key in ANYTYPE_API_KEY")
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    status = 0
    for scenario_id in selected:
        scenario = scenarios[scenario_id]
        replicate = f"r{args.replicate}-" if args.replicate else ""
        label = args.label or f"eval-{args.host}-{args.arm if args.upstream_url else 'bridge'}-{replicate}{stamp}-{scenario_id.lower()}"
        run_dir = args.output.resolve() / label
        run_dir.mkdir(parents=True, mode=0o700)
        workspace = Path(tempfile.mkdtemp(prefix=label + "-"))
        guard = guard_args(run_dir, label, codex, args.upstream, args.with_hooks, args.upstream_url, args.arm)
        host = make_host(args.host, run_dir=run_dir, workspace=workspace, model=args.model,
                         reasoning=args.reasoning, guard=guard, codex=codex, upstream=args.upstream,
                         upstream_url=args.upstream_url, arm=args.arm, with_hooks=args.with_hooks,
                         label=label, timeout=args.turn_timeout, executable=args.host_executable)
        manifest = {"scenario_id": scenario_id, "host": args.host, "host_version": host.version(),
                    "model": args.model, "reasoning": args.reasoning if args.host == "codex" else None,
                    "surface": "full" if args.upstream_url else "bridge",
                    "arm": args.arm if args.upstream_url else None,
                    "upstream_url": args.upstream_url,
                    "label": label, "workspace": str(workspace),
                    "started_at": stamp, "turns": [], "status": "running",
                    "replicate": args.replicate, "runner_pid": os.getpid(),
                    "hook": scenario.get("harness"),
                    "hook_status": "not_exercised" if scenario.get("harness") else "not_applicable",
                    "isolation": host.isolation,
                    "surface_variation": "Guard requests full IDs for space creation and canonicalizes accepted scope IDs; original/forwarded arguments are both logged."}
        session_id = None
        save_report(run_dir, manifest)
        for turn in scenario["turns"]:
            if args.max_turns and turn["turn"] > args.max_turns:
                manifest["status"] = "stopped_at_requested_turn_limit"
                break
            # These user turns require state injected by an independent client.
            # Baseline mode does not pretend that those fixtures were created.
            if scenario_id == "MCP-28" and "Original lease notes" in turn["user"] and not args.with_hooks:
                manifest.setdefault("skipped_turns", []).append({"turn": turn["turn"], "reason": "foreign-provenance fixture unavailable"})
                continue
            if args.with_hooks:
                from eval_fixtures import before_turn
                before_turn(run_dir, manifest, turn["turn"], codex, args.upstream)
                profiles = {"MCP-14": "mcp14_welcome_timeout", "MCP-27": "mcp27_atomic_patch_failure",
                            "MCP-29": "mcp29_reaction_timeout"}
                config = {"scenario_id": scenario_id, "current_turn": turn["turn"],
                          "armed_profiles": [profiles[scenario_id]] if scenario_id in profiles else []}
                (run_dir / "hook-config.json").write_text(json.dumps(config) + "\n")
            prompt = turn["user"].replace("{{run}}", label).replace("{{fixture_dir}}", str(ROOT / "docs/evals/anytype-mcp-v2/fixtures"))
            if not session_id:
                prompt = ("Use only the Anytype MCP tools for this conversation. Create a new space for this run and use it for every space-scoped call and write. Read-only global discovery and global search are allowed, including searches requested across spaces. Never edit, delete, or otherwise modify an existing space. The MCP guard enforces that boundary. Do not use shell, filesystem, browser, other connectors, or source-code inspection. Do not read evaluation files or future turns. Complete only the current user turn, then stop.\n\n" + prompt)
            n = turn["turn"]
            (run_dir / f"turn-{n:02d}.prompt.txt").write_text(prompt)
            print(f"{scenario_id} turn {n}/{len(scenario['turns'])} started", flush=True)
            result = host.resume(session_id, prompt, n) if session_id else host.start(prompt, n)
            session_id = result.session_id or session_id
            record = {"turn": n, "user": prompt, "exit_code": result.exit_code, "timeout": result.timeout,
                      "duration_seconds": result.duration_seconds, "turn_completed": result.completed,
                      "usage": result.usage, "usage_detail": result.usage_detail}
            if result.offered_tools is not None:
                record["offered_tools"] = result.offered_tools
            manifest["turns"].append(record)
            manifest["session_id"] = session_id
            stats = run_dir / "tools-list-stats.json"
            if stats.exists():
                manifest["tools_list"] = json.loads(stats.read_text())
            if args.with_hooks:
                from eval_fixtures import finalize_turn
                finalize_turn(run_dir, manifest, n)
            save_report(run_dir, manifest)
            print(f"{scenario_id} turn {n} {'completed' if result.completed else 'failed'}; cumulative MCP calls={manifest['tool_call_count']}", flush=True)
            if result.exit_code or not result.completed or not session_id or result.infrastructure_error:
                manifest["status"] = "runtime_failed"
                if result.infrastructure_error:
                    manifest["infrastructure_error"] = result.infrastructure_error
                break
            if args.snapshot_after_turn and len(manifest["owned_spaces"]) == 1:
                from snapshot_run import snapshot
                try:
                    count = snapshot(run_dir, codex, args.upstream, allow_running=True,
                                     output_path=run_dir / "snapshots" / f"turn-{n:02d}.json")
                    record["independent_snapshot_reads"] = count
                except Exception as exc:
                    record["independent_snapshot_error"] = str(exc)
                save_report(run_dir, manifest)
        else:
            manifest["status"] = "conversation_completed_pending_review"
        save_report(run_dir, manifest)
        if args.snapshot_after_turn and len(manifest["owned_spaces"]) == 1:
            from snapshot_run import snapshot
            try:
                snapshot(run_dir, codex, args.upstream)
            except Exception as exc:
                manifest["final_snapshot_error"] = str(exc)
                save_report(run_dir, manifest)
        print(f"{scenario_id}: {manifest['status']} — {run_dir}", flush=True)
        if manifest["status"] == "runtime_failed":
            status = 1
            break
    return status


def parser(default_host="codex"):
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--host", choices=["codex", "claude"], default=default_host)
    p.add_argument("--host-executable", help="path to the host CLI (default: found on PATH)")
    p.add_argument("--scenario", nargs="+", default=["MCP-01"])
    p.add_argument("--model", default="gpt-5.6-luna")
    p.add_argument("--reasoning", default="medium", help="Codex reasoning effort")
    p.add_argument("--output", type=Path, required=True)
    p.add_argument("--label", help="run label (default: derived from host, arm, time and scenario)")
    p.add_argument("--codex", default="codex")
    p.add_argument("--upstream", default="anytype", help="the bridge: a stdio server configured in Codex")
    p.add_argument("--upstream-url", help="heart's /mcp/full instead of the bridge; key in ANYTYPE_API_KEY")
    p.add_argument("--arm", choices=["full", "full-opaque"], default="full")
    p.add_argument("--turn-timeout", type=int, default=600)
    p.add_argument("--max-turns", type=int)
    p.add_argument("--replicate", type=int, choices=[1, 2, 3])
    p.add_argument("--snapshot-after-turn", action="store_true")
    p.add_argument("--with-hooks", action="store_true")
    return p


def main(default_host="codex"):
    def interrupted(_signum, _frame):
        raise KeyboardInterrupt
    signal.signal(signal.SIGTERM, interrupted)
    sys.exit(run(parser(default_host).parse_args()))


if __name__ == "__main__":
    main()
