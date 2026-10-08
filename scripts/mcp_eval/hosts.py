"""Host drivers: the agent CLI that runs the model against the guard.

Each host runs one conversation turn at a time — start() for the first,
resume() for the rest — and writes the turn's events twice:

- `turn-NN.host-events.jsonl`: the host's own stream, verbatim (Claude
  only; for Codex the stream already is the normalized one);
- `turn-NN.events.jsonl`: the normalized stream every reader of a run
  consumes (summaries, review, resume): `thread.started`,
  `item.completed` with `mcp_tool_call` / `agent_message` items, and
  `turn.completed` with usage — Codex's own event shapes.

Usage is the host's own count. `usage` is normalized to Codex's shape
(input_tokens counts every input token, cache reads included;
cached_input_tokens is the cache-read part); `usage_detail` keeps the
host's fields.
"""
from dataclasses import dataclass, field
import json
from pathlib import Path
import subprocess
import sys
import uuid

from run_scenario import command, configurations, execute, read_events

HOST_SERVER = "anytype"


@dataclass
class TurnResult:
    session_id: str | None
    completed: bool
    exit_code: int | None
    timeout: bool
    duration_seconds: float
    final_text: str = ""
    usage: dict | None = None
    usage_detail: dict | None = None
    tool_calls: list = field(default_factory=list)
    offered_tools: list | None = None
    infrastructure_error: str | None = None


def _version(executable):
    try:
        out = subprocess.run([executable, "--version"], capture_output=True, text=True, timeout=30)
        return out.stdout.strip() or out.stderr.strip()
    except (OSError, subprocess.TimeoutExpired) as exc:
        return f"unknown ({exc})"


class CodexHost:
    """`codex exec`, as the runner always drove it."""
    name = "codex"
    isolation = "MCP scope guard; external tools disabled; read-only empty workspace"

    def __init__(self, *, run_dir, workspace, model, reasoning, codex, upstream, upstream_url, arm,
                 with_hooks, label, timeout, executable=None, **_):
        self.run_dir, self.workspace, self.model, self.timeout = run_dir, workspace, model, timeout
        self.codex = executable or codex
        self.flags = configurations(run_dir, label, codex, upstream, with_hooks, upstream_url, arm, reasoning)

    def version(self):
        return _version(self.codex)

    def start(self, prompt, turn):
        return self._turn(prompt, turn, None)

    def resume(self, session_id, prompt, turn):
        return self._turn(prompt, turn, session_id)

    def _turn(self, prompt, turn, session_id):
        events_path = self.run_dir / f"turn-{turn:02d}.events.jsonl"
        final = self.run_dir / f"turn-{turn:02d}.final.txt"
        result = execute(command(self.codex, self.flags, self.model, self.workspace, final, session_id),
                         prompt, events_path, self.run_dir / f"turn-{turn:02d}.stderr.log", self.timeout)
        events = read_events(events_path)
        for e in events:
            if e.get("type") == "thread.started":
                session_id = e["thread_id"]
        usage = next((e.get("usage") for e in reversed(events) if e.get("type") == "turn.completed"), None)
        calls = [e["item"] for e in events if e.get("type") == "item.completed"
                 and e.get("item", {}).get("type") == "mcp_tool_call"]
        infra = next(("approval refused by policy" for e in events
                      if "requires approval, but approval policy is never" in json.dumps(e)), None)
        return TurnResult(session_id=session_id, completed=any(e.get("type") == "turn.completed" for e in events),
                          exit_code=result["exit_code"], timeout=result["timeout"],
                          duration_seconds=result["duration_seconds"],
                          final_text=final.read_text() if final.exists() else "",
                          usage=usage, usage_detail=usage, tool_calls=calls, infrastructure_error=infra)


# The pinned Claude Code invocation. Each flag is load-bearing, and
# test_hosts.py proves the set against a stub Messages API on the installed
# version:
#   --tools ""                    no built-in tool at all (no Bash, file, web or agent tools)
#   --mcp-config + --strict-mcp-config  the guard is the only MCP server
#   --allowedTools mcp__anytype   the guard's tools are pre-approved...
#   --permission-mode dontAsk     ...and anything else is denied, never prompted
#   --setting-sources ""          no user, project or local settings: no hooks,
#                                 permissions, plugins or model overrides
#   --disable-slash-commands      no skills
#   --settings autoMemoryEnabled=false  no auto-memory prompt (it otherwise
#                                 names a directory under the user's home)
# (--bare would be stricter still, but accepts only ANTHROPIC_API_KEY auth.)
CLAUDE_FLAGS = ["--output-format", "stream-json", "--verbose",
                "--strict-mcp-config", "--tools", "",
                "--allowedTools", "mcp__" + HOST_SERVER,
                "--permission-mode", "dontAsk",
                "--setting-sources", "",
                "--disable-slash-commands",
                "--settings", json.dumps({"autoMemoryEnabled": False})]

# Claude's refusal text for a tool the permission mode denied.
CLAUDE_DENIED = ("haven't granted", "permission to use")


class ClaudeHost:
    """`claude -p`, one process per turn; later turns resume the session."""
    name = "claude"
    isolation = ("MCP scope guard; no built-in tools (--tools \"\"), only the guard's MCP server, "
                 "no user/project settings, hooks or skills; empty workspace")

    def __init__(self, *, run_dir, workspace, model, guard, timeout, executable=None, **_):
        self.run_dir, self.workspace, self.model, self.timeout = run_dir, workspace, model, timeout
        self.claude = executable or "claude"
        self.mcp_config = run_dir / "claude-mcp.json"
        # no credential here: the guard inherits ANYTYPE_API_KEY from the
        # environment of the claude process
        self.mcp_config.write_text(json.dumps({"mcpServers": {HOST_SERVER: {
            "type": "stdio", "command": sys.executable, "args": guard}}}, indent=2) + "\n")

    def version(self):
        return _version(self.claude)

    def command(self, session_id, resume):
        cmd = [self.claude, "-p", *CLAUDE_FLAGS, "--model", self.model, "--mcp-config", str(self.mcp_config)]
        cmd += ["--resume", session_id] if resume else ["--session-id", session_id]
        return cmd

    def start(self, prompt, turn):
        return self._turn(prompt, turn, str(uuid.uuid4()), False)

    def resume(self, session_id, prompt, turn):
        return self._turn(prompt, turn, session_id, True)

    def _turn(self, prompt, turn, session_id, resume):
        raw_path = self.run_dir / f"turn-{turn:02d}.host-events.jsonl"
        result = execute(self.command(session_id, resume), prompt, raw_path,
                         self.run_dir / f"turn-{turn:02d}.stderr.log", self.timeout, cwd=self.workspace)
        normalized, turn_result = normalize_claude(read_events(raw_path), session_id)
        (self.run_dir / f"turn-{turn:02d}.events.jsonl").write_text(
            "".join(json.dumps(e, ensure_ascii=False) + "\n" for e in normalized))
        (self.run_dir / f"turn-{turn:02d}.final.txt").write_text(turn_result.final_text)
        turn_result.exit_code = result["exit_code"]
        turn_result.timeout = result["timeout"]
        turn_result.duration_seconds = result["duration_seconds"]
        return turn_result


def strip_host(name):
    return name.removeprefix("mcp__" + HOST_SERVER + "__")


def normalize_claude(events, session_id):
    """Claude's stream-json events as Codex-shaped normalized events."""
    out = [{"type": "thread.started", "thread_id": session_id}]
    uses, offered, final, usage_detail, completed, failed = {}, None, "", None, False, None
    infra = None
    for e in events:
        kind = e.get("type")
        if kind == "system" and e.get("subtype") == "init":
            session_id = e.get("session_id", session_id)
            out[0]["thread_id"] = session_id
            offered = e.get("tools", [])
        elif kind == "assistant":
            for block in e.get("message", {}).get("content", []):
                if block.get("type") == "tool_use":
                    uses[block["id"]] = block
                elif block.get("type") == "text" and block.get("text"):
                    out.append({"type": "item.completed", "item": {"type": "agent_message", "text": block["text"]}})
        elif kind == "user":
            content = e.get("message", {}).get("content", [])
            for block in content if isinstance(content, list) else []:
                if block.get("type") != "tool_result":
                    continue
                use = uses.pop(block.get("tool_use_id"), {})
                parts = block.get("content")
                if isinstance(parts, str):
                    parts = [{"type": "text", "text": parts}]
                is_error = bool(block.get("is_error"))
                text = " ".join(p.get("text", "") for p in parts or [] if isinstance(p, dict))
                if is_error and any(marker in text for marker in CLAUDE_DENIED):
                    infra = "the host denied a tool: " + text[:300]
                out.append({"type": "item.completed", "item": {
                    "type": "mcp_tool_call", "server": HOST_SERVER, "tool": strip_host(use.get("name", "")),
                    "arguments": use.get("input", {}), "result": {"content": parts or [], "isError": is_error},
                    "error": None, "status": "failed" if is_error else "completed"}})
        elif kind == "result":
            final = e.get("result") or ""
            usage_detail = e.get("usage")
            if e.get("is_error") or e.get("subtype") != "success":
                failed = e
            else:
                completed = True
    usage = None
    if usage_detail:
        read = usage_detail.get("cache_read_input_tokens", 0) or 0
        write = usage_detail.get("cache_creation_input_tokens", 0) or 0
        usage = {"input_tokens": (usage_detail.get("input_tokens", 0) or 0) + read + write,
                 "cached_input_tokens": read, "output_tokens": usage_detail.get("output_tokens", 0) or 0}
    if completed:
        out.append({"type": "turn.completed", "usage": usage})
    elif failed is not None:
        out.append({"type": "turn.failed", "error": failed})
    calls = [e["item"] for e in out if e.get("item", {}).get("type") == "mcp_tool_call"]
    return out, TurnResult(session_id=session_id, completed=completed, exit_code=None, timeout=False,
                           duration_seconds=0.0, final_text=final, usage=usage, usage_detail=usage_detail,
                           tool_calls=calls, offered_tools=offered, infrastructure_error=infra)


HOSTS = {"codex": CodexHost, "claude": ClaudeHost}


def make_host(name, **kwargs):
    return HOSTS[name](**kwargs)
