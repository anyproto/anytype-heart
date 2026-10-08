from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import unittest

from space_guard import apply_arm, opaque_ops, reachable_defs, compact_bytes

HERE = Path(__file__).resolve().parent
GOLDEN = HERE.parents[1] / "core/api/wrapper/full/testdata/tools_list.golden.json"
SPACE = "bafyreiguardtest123456.k"


class StubMCP(BaseHTTPRequestHandler):
    """A stub /mcp/full: records each request's headers and answers like
    heart does — JSON for requests, 202 for notifications."""
    seen = []
    tools = None

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        StubMCP.seen.append({"path": self.path, "headers": dict(self.headers), "body": body})
        if "id" not in body:
            self.send_response(202)
            self.end_headers()
            return
        method = body.get("method")
        if method == "initialize":
            result = {"protocolVersion": "2025-06-18", "capabilities": {"tools": {}}, "serverInfo": {"name": "stub"}}
        elif method == "tools/list":
            result = StubMCP.tools
        elif method == "tools/call" and body["params"]["name"] == "create_space":
            result = {"content": [{"type": "text", "text": json.dumps({"id": SPACE})}]}
        else:
            result = {"content": [{"type": "text", "text": json.dumps({"echo": body["params"]})}]}
        data = json.dumps({"jsonrpc": "2.0", "id": body["id"], "result": result}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, *args):
        pass


def golden():
    return json.loads(GOLDEN.read_text())


class HttpUpstreamTests(unittest.TestCase):
    def setUp(self):
        StubMCP.seen = []
        StubMCP.tools = golden()
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), StubMCP)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        self.tmp = tempfile.TemporaryDirectory()

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.tmp.cleanup()

    def run_guard(self, arm, messages):
        tmp = Path(self.tmp.name)
        url = f"http://127.0.0.1:{self.server.server_address[1]}/mcp/full"
        env = {**os.environ, "ANYTYPE_API_KEY": "eval-key"}
        proc = subprocess.run([sys.executable, str(HERE / "space_guard.py"), "--state", str(tmp / "scope.json"),
                               "--trace", str(tmp / "wire.jsonl"), "--run-label", "eval-run",
                               "--upstream-url", url, "--arm", arm, "--stats", str(tmp / "stats.json")],
                              input="".join(json.dumps(m) + "\n" for m in messages),
                              capture_output=True, text=True, env=env, timeout=60)
        self.assertEqual(0, proc.returncode, proc.stderr)
        return [json.loads(line) for line in proc.stdout.splitlines()], json.loads((tmp / "stats.json").read_text())

    def conversation(self):
        return [
            {"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2025-06-18"}},
            {"jsonrpc": "2.0", "method": "notifications/initialized"},
            {"jsonrpc": "2.0", "id": 2, "method": "tools/list"},
            {"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "create_space", "arguments": {"name": "eval-run — Garden"}}},
            {"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "patch_object", "arguments": {
                "space_id": SPACE[-8:-2], "object_id": "o", "ops": [{"op": "delete_block", "id": "b"}]}}},
        ]

    def test_bearer_json_and_no_session_header(self):
        responses, stats = self.run_guard("full", self.conversation())
        self.assertEqual([1, 2, 3, 4], [r["id"] for r in responses])
        self.assertEqual(5, len(StubMCP.seen), "every message, the notification included, reached the upstream")
        for request in StubMCP.seen:
            headers = {k.lower(): v for k, v in request["headers"].items()}
            self.assertEqual("/mcp/full", request["path"])
            self.assertEqual("Bearer eval-key", headers["authorization"])
            self.assertEqual("application/json", headers["content-type"])
            self.assertEqual("application/json", headers["accept"])
            self.assertNotIn("mcp-session-id", headers)
        patch = StubMCP.seen[-1]["body"]["params"]["arguments"]
        self.assertEqual(SPACE, patch["space_id"], "the compact space id was pinned to the full one")
        self.assertEqual([{"op": "delete_block", "id": "b"}], patch["ops"], "flat full-tier arguments pass through")
        self.assertEqual(golden(), responses[1]["result"], "the full arm forwards tools/list as served")
        self.assertEqual({"arm": "full", "tool_count": len(golden()["tools"]), "served_bytes": compact_bytes(golden()),
                          "forwarded_bytes": compact_bytes(golden())}, stats)

    def test_full_opaque_rewrites_only_the_two_envelopes(self):
        responses, stats = self.run_guard("full-opaque", self.conversation())
        served = {t["name"]: t for t in golden()["tools"]}
        forwarded = {t["name"]: t for t in responses[1]["result"]["tools"]}
        self.assertEqual(list(served), list(forwarded))
        for name, tool in served.items():
            if name in ("patch_object", "update_type"):
                continue
            self.assertEqual(tool, forwarded[name], f"{name} is untouched")
        self.assertLess(stats["forwarded_bytes"], stats["served_bytes"])
        self.assertEqual("full-opaque", stats["arm"])


class OpaqueRewriteTests(unittest.TestCase):
    def test_only_ops_items_change_and_dropped_defs_are_exactly_the_unreferenced(self):
        for tool in golden()["tools"]:
            if tool["name"] not in ("patch_object", "update_type"):
                continue
            before = tool["inputSchema"]
            after = opaque_ops(before)
            # only the ops items differ outside $defs
            strip = lambda s: {k: v for k, v in s.items() if k != "$defs"}
            b, a = json.loads(json.dumps(strip(before))), json.loads(json.dumps(strip(after)))
            for holder in [b, *b.get("anyOf", [])]:
                if "items" in holder.get("properties", {}).get("ops", {}):
                    holder["properties"]["ops"]["items"] = {"type": "object"}
            self.assertEqual(b, a, tool["name"])
            # kept defs are exactly those still reachable, and every one of them is kept as served
            kept = set(after.get("$defs", {}))
            self.assertEqual(reachable_defs(after), kept)
            self.assertTrue(kept < set(before["$defs"]), "the op definitions are gone")
            for name in kept:
                self.assertEqual(before["$defs"][name], after["$defs"][name])
            self.assertFalse(any(n.startswith("op__") for n in kept))

    def test_an_arm_without_its_tools_is_refused(self):
        with self.assertRaises(ValueError):
            apply_arm({"tools": [{"name": "list_spaces", "inputSchema": {}}]}, "full-opaque")
        with self.assertRaises(ValueError):
            apply_arm(golden(), "bridge")


if __name__ == "__main__":
    unittest.main()
