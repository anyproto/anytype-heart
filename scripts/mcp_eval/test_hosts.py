"""Host driver smoke tests: the real host CLIs against stub servers.

The Claude test points the installed `claude` at a stub Messages API
(ANTHROPIC_BASE_URL), so it spends nothing and sees exactly what the host
sends the model: the tool list, the system prompt and the messages. The
stub answers the first request with a call to one guard tool and the next
with text, so the test also proves the call is pre-approved and reaches
the stub /mcp/full through the guard. Skipped when the CLI is absent.
"""
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import shutil
import tempfile
import threading
import unittest
from unittest.mock import patch

from hosts import CLAUDE_FLAGS, ClaudeHost, normalize_claude
from run_scenario import guard_args
from test_space_guard_http import StubMCP

SMALL_TOOLS = {"tools": [
    {"name": "list_spaces", "description": "List spaces.", "inputSchema": {"type": "object", "properties": {}}},
    {"name": "create_space", "description": "Create a space.", "inputSchema": {"type": "object", "properties": {"name": {"type": "string"}}}},
    # the arm needs these two to exist; their schemas are minimal here
    {"name": "patch_object", "description": "Edit.", "inputSchema": {"type": "object", "properties": {"ops": {"type": "array", "items": {"type": "object"}}}}},
    {"name": "update_type", "description": "Edit a type.", "inputSchema": {"type": "object", "properties": {"ops": {"type": "array", "items": {"type": "object"}}}}},
]}


def sse(events):
    return "".join(f"event: {e['type']}\ndata: {json.dumps(e)}\n\n" for e in events).encode()


def message_events(blocks, stop_reason):
    events = [{"type": "message_start", "message": {
        "id": "msg_stub", "type": "message", "role": "assistant", "model": "claude-haiku-4-5-20251001",
        "content": [], "stop_reason": None, "stop_sequence": None,
        "usage": {"input_tokens": 11, "output_tokens": 1, "cache_read_input_tokens": 7, "cache_creation_input_tokens": 3}}}]
    for i, block in enumerate(blocks):
        if block["type"] == "tool_use":
            events.append({"type": "content_block_start", "index": i, "content_block": {**block, "input": {}}})
            events.append({"type": "content_block_delta", "index": i,
                           "delta": {"type": "input_json_delta", "partial_json": json.dumps(block["input"])}})
        else:
            events.append({"type": "content_block_start", "index": i, "content_block": {"type": "text", "text": ""}})
            events.append({"type": "content_block_delta", "index": i, "delta": {"type": "text_delta", "text": block["text"]}})
        events.append({"type": "content_block_stop", "index": i})
    events.append({"type": "message_delta", "delta": {"stop_reason": stop_reason, "stop_sequence": None},
                   "usage": {"output_tokens": 5}})
    events.append({"type": "message_stop"})
    return events


class StubAnthropic(BaseHTTPRequestHandler):
    requests = []

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))) or b"{}")
        StubAnthropic.requests.append({"path": self.path, "body": body})
        if not self.path.startswith("/v1/messages") or "count_tokens" in self.path:
            self._json(200, {"input_tokens": 1}) if "count_tokens" in self.path else self._json(404, {})
            return
        has_result = any(isinstance(m.get("content"), list) and any(b.get("type") == "tool_result" for b in m["content"])
                         for m in body.get("messages", []))
        if body.get("tools") and not has_result:
            blocks, stop = [{"type": "tool_use", "id": "toolu_stub1", "name": "mcp__anytype__list_spaces", "input": {}}], "tool_use"
        else:
            blocks, stop = [{"type": "text", "text": "done"}], "end_turn"
        if body.get("stream"):
            data = sse(message_events(blocks, stop))
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)
        else:
            self._json(200, {"id": "msg_stub", "type": "message", "role": "assistant", "model": body.get("model"),
                             "content": blocks, "stop_reason": stop, "stop_sequence": None,
                             "usage": {"input_tokens": 11, "output_tokens": 5}})

    def do_GET(self):
        self._json(404, {})

    def _json(self, status, value):
        data = json.dumps(value).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, *args):
        pass


def serve(handler):
    server = ThreadingHTTPServer(("127.0.0.1", 0), handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    return server


@unittest.skipUnless(shutil.which("claude"), "claude CLI not installed")
class ClaudeHostSmokeTest(unittest.TestCase):
    def test_only_guard_tools_offered_preapproved_and_no_user_config(self):
        StubAnthropic.requests, StubMCP.seen, StubMCP.tools = [], [], SMALL_TOOLS
        anthropic, mcp = serve(StubAnthropic), serve(StubMCP)
        self.addCleanup(anthropic.shutdown)
        self.addCleanup(mcp.shutdown)
        with tempfile.TemporaryDirectory() as tmp:
            run_dir, workspace = Path(tmp) / "run", Path(tmp) / "ws"
            run_dir.mkdir()
            workspace.mkdir()
            guard = guard_args(run_dir, "eval-smoke", None, None, upstream_url=f"http://127.0.0.1:{mcp.server_address[1]}/mcp/full")
            host = ClaudeHost(run_dir=run_dir, workspace=workspace, model="claude-haiku-4-5-20251001", guard=guard, timeout=180)
            env = {"ANTHROPIC_BASE_URL": f"http://127.0.0.1:{anthropic.server_address[1]}",
                   "ANTHROPIC_API_KEY": "sk-ant-stub", "ANYTYPE_API_KEY": "eval-key",
                   "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"}
            with patch.dict(os.environ, env):
                result = host.start("List the spaces, then say done.", 1)
            main = [r["body"] for r in StubAnthropic.requests if r["body"].get("tools")]
            self.assertTrue(main, f"the host sent no tool-bearing request: {[r['path'] for r in StubAnthropic.requests]}; "
                                  f"stderr: {(run_dir / 'turn-01.stderr.log').read_text()[-2000:]}")
            offered = sorted({t["name"] for body in main for t in body["tools"]})
            self.assertEqual(sorted("mcp__anytype__" + t["name"] for t in SMALL_TOOLS["tools"]), offered,
                             "the model is offered the guard's tools and nothing else")
            everything = json.dumps(main)
            # what only the user's own configuration would put there: a plugin's
            # SessionStart hook, a skill listing, auto-memory, any path under home
            for marker in ("superpowers", "EXTREMELY_IMPORTANT", "skills are available", "# auto memory",
                           str(Path.home()), "Contents of /"):
                self.assertNotIn(marker, everything, f"user configuration leaked into the request: {marker}")
            self.assertTrue(result.completed, result)
            self.assertEqual(["list_spaces"], [c["tool"] for c in result.tool_calls])
            self.assertEqual("completed", result.tool_calls[0]["status"], "the call was pre-approved, not denied")
            self.assertIsNone(result.infrastructure_error)
            calls = [s for s in StubMCP.seen if s["body"].get("method") == "tools/call"]
            self.assertEqual(["list_spaces"], [c["body"]["params"]["name"] for c in calls], "the call reached /mcp/full")
            self.assertEqual("Bearer eval-key", {k.lower(): v for k, v in calls[0]["headers"].items()}["authorization"],
                             "the guard inherited the key from the host's environment")
            self.assertTrue(result.usage and result.usage["output_tokens"] > 0, result.usage)
            self.assertTrue(all(t.startswith("mcp__anytype__") for t in result.offered_tools), result.offered_tools)
            normalized = [json.loads(l) for l in (run_dir / "turn-01.events.jsonl").read_text().splitlines()]
            self.assertEqual("thread.started", normalized[0]["type"])
            self.assertEqual("turn.completed", normalized[-1]["type"])
            print("\nclaude smoke evidence:", json.dumps({
                "version": host.version(), "flags": CLAUDE_FLAGS, "offered_to_model": offered,
                "requests_to_model": len(main), "system_prompt_chars": len(json.dumps(main[0].get("system"))),
                "usage": result.usage}, indent=1))


class NormalizeClaudeTests(unittest.TestCase):
    def test_a_denied_tool_is_an_infrastructure_error(self):
        events = [{"type": "system", "subtype": "init", "session_id": "s", "tools": ["mcp__anytype__list_spaces"]},
                  {"type": "assistant", "message": {"content": [{"type": "tool_use", "id": "t", "name": "mcp__anytype__list_spaces", "input": {}}]}},
                  {"type": "user", "message": {"content": [{"type": "tool_result", "tool_use_id": "t", "is_error": True,
                                                            "content": "Claude requested permissions to use mcp__anytype__list_spaces, but you haven't granted it yet."}]}},
                  {"type": "result", "subtype": "success", "is_error": False, "result": "x",
                   "usage": {"input_tokens": 1, "output_tokens": 2, "cache_read_input_tokens": 3, "cache_creation_input_tokens": 4}}]
        _, result = normalize_claude(events, "s")
        self.assertIsNotNone(result.infrastructure_error)
        self.assertEqual({"input_tokens": 8, "cached_input_tokens": 3, "output_tokens": 2}, result.usage)




class CodexHostConfigTests(unittest.TestCase):
    def test_the_http_upstream_reaches_the_guard_without_its_key_on_argv(self):
        from hosts import CodexHost
        with tempfile.TemporaryDirectory() as tmp:
            run_dir = Path(tmp)
            host = CodexHost(run_dir=run_dir, workspace=run_dir, model="gpt-5.6-luna", reasoning="medium",
                             codex="codex", upstream="anytype", upstream_url="http://127.0.0.1:1/mcp/full",
                             arm="full-opaque", with_hooks=False, label="eval-x", timeout=10)
            flags = " ".join(host.flags)
            self.assertIn('mcp_servers.anytype.env_vars=["ANYTYPE_API_KEY"]', flags)
            self.assertIn("--upstream-url", flags)
            self.assertIn('"full-opaque"', flags)
            self.assertNotIn("--codex", flags, "the HTTP upstream needs no Codex MCP config")
            self.assertIn('model_reasoning_effort="medium"', flags)


if __name__ == "__main__":
    unittest.main()
