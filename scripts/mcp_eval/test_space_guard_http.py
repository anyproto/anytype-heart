from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import unittest

from space_guard import apply_arm, inline_tool, reachable_defs, compact_bytes, OP_BRANCH

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
            result = {"protocolVersion": "2025-06-18", "capabilities": {"tools": {}}, "serverInfo": {"name": "stub", "version": "1"}}
        elif method == "tools/list":
            result = StubMCP.tools
        elif method == "tools/call" and body["params"]["name"] == "create_space":
            result = {"content": [{"type": "text", "text": json.dumps({"id": SPACE})}]}
        elif method == "tools/call" and body["params"]["name"] == "get_op_schema":
            result = {"content": [{"type": "text", "text": json.dumps(served_op(body["params"]["arguments"]["op"]))}]}
        elif method == "tools/call" and body["params"]["name"] == "get_schema":
            result = {"content": [{"type": "text", "text": json.dumps(served_kind(body["params"]["arguments"]["kind"]))}]}
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


# Synthetic served schemas with the shapes that make inlining hard: a
# definition name that means two different things on different ops
# (`block`), one every op shares (`blockRef`), a document with a `$schema`
# member, its own $defs, a cross-member allOf, and a `properties` member
# spelled differently from the shortcut body's.
NEW_CONTENT_OPS = {"insert_blocks", "insert_view"}


def served_op(op):
    block = {"type": "object", "additionalProperties": False,
             "properties": {"type": {"type": "string"}, **({} if op in NEW_CONTENT_OPS else {"id": {"type": "string"}})}}
    return {"kind": op, "example": {"op": op}, "schema": {
        "type": "object", "additionalProperties": False, "required": ["op"],
        "properties": {"op": {"const": op}, "target": {"$ref": "#/$defs/blockRef"},
                       "blocks": {"type": "array", "items": {"$ref": "#/$defs/block"}}},
        "$defs": {"block": block, "blockRef": {"type": "string", "maxLength": 64}}}}


def served_kind(kind):
    return {"kind": kind, "example": {"formatVersion": "2.0"}, "schema": {
        "type": "object", "additionalProperties": False, "required": ["formatVersion"],
        "properties": {"$schema": {"type": "string"}, "formatVersion": {"const": "2.0"},
                       "properties": {"type": "object", "additionalProperties": {"$ref": "#/$defs/anyValue"}},
                       "blocks": {"type": "array", "items": {"$ref": "#/$defs/" + kind + "Block"}}},
        "allOf": [{"if": {"required": ["kind"]}, "then": {"required": ["blocks"]}}],
        "$defs": {"anyValue": {"type": ["string", "number"]}, kind + "Block": {"type": "object"}}}}


def dangling(schema):
    refs = set()
    def walk(node):
        if isinstance(node, dict):
            if isinstance(node.get("$ref"), str):
                refs.add(node["$ref"].removeprefix("#/$defs/"))
            for v in node.values():
                walk(v)
        elif isinstance(node, list):
            for v in node:
                walk(v)
    walk(schema)
    return refs - set(schema.get("$defs", {}))


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

    def test_full_inline_fetches_each_lookup_once_and_inlines_it(self):
        responses, stats = self.run_guard("full-inline", self.conversation())
        served = {t["name"]: t for t in golden()["tools"]}
        inlined = {t["name"]: t for t in responses[1]["result"]["tools"]}
        self.assertEqual(list(served), list(inlined))
        changed = {n for n in served if served[n] != inlined[n]}
        self.assertEqual({"patch_object", "update_type", "create_object", "create_template", "create_type", "validate"}, changed)
        for name, tool in inlined.items():
            self.assertEqual(set(), dangling(tool["inputSchema"]), f"{name} has a dangling reference")
        lookups = [s["body"]["params"] for s in StubMCP.seen
                   if s["body"].get("method") == "tools/call" and s["body"]["params"]["name"] in ("get_op_schema", "get_schema")]
        ops = [l["arguments"]["op"] for l in lookups if l["name"] == "get_op_schema"]
        self.assertEqual(len(ops), len(set(ops)), "each op schema is fetched once")
        self.assertEqual(18, len(ops), "15 object ops and 7 type ops, four shared")
        self.assertEqual({"object", "template", "type_document", "document"},
                         {l["arguments"]["kind"] for l in lookups if l["name"] == "get_schema"})
        self.assertGreater(stats["forwarded_bytes"], stats["served_bytes"])
        self.assertEqual("full-inline", stats["arm"])
        wire = [json.loads(l) for l in (Path(self.tmp.name) / "wire.jsonl").read_text().splitlines()]
        self.assertTrue(any(e["direction"] == "evaluator_to_upstream" for e in wire), "lookups are evaluator traffic")
        self.assertFalse(any(e["direction"] == "to_model" and "evaluation-schema-" in json.dumps(e["payload"]) for e in wire))


class InlineToolTests(unittest.TestCase):
    def fetch(self, name, args):
        return served_op(args["op"]) if name == "get_op_schema" else served_kind(args["kind"])

    def tool(self, name):
        return next(t for t in golden()["tools"] if t["name"] == name)

    def test_ops_become_a_oneof_with_definitions_namespaced_then_merged(self):
        schema = inline_tool(self.tool("patch_object"), self.fetch)["inputSchema"]
        enum = self.tool("patch_object")["inputSchema"]["properties"]["ops"]["items"]["properties"]["op"]["enum"]
        branches = schema["properties"]["ops"]["items"]["oneOf"]
        self.assertEqual(["#/$defs/" + OP_BRANCH + op for op in enum], [b["$ref"] for b in branches])
        defs = schema["$defs"]
        self.assertIn("blockRef", defs, "identical on every op: one plain definition")
        self.assertEqual({"block__insert_blocks", "block__add_items"},
                         {k for k in defs if k.startswith("block__")}, "two shapes of block, each named after the first op (alphabetically) that has it")
        self.assertEqual(set(), dangling(schema))

    def test_an_open_document_root_is_closed_over_its_kind(self):
        schema = inline_tool(self.tool("create_template"), self.fetch)["inputSchema"]
        self.assertFalse(schema["additionalProperties"])
        self.assertIn("formatVersion", schema["properties"])
        self.assertNotIn("$schema", schema["properties"])
        self.assertIn("formatVersion", schema["required"])
        self.assertIn("space_id", schema["required"], "the path argument stays required")
        self.assertIn("anyValue", schema["$defs"])

    def test_a_document_alternative_takes_its_kind_and_conflicts_move_into_branches(self):
        served = self.tool("create_object")["inputSchema"]
        schema = inline_tool(self.tool("create_object"), self.fetch)["inputSchema"]
        shortcut, document = schema["anyOf"]
        self.assertFalse(document["additionalProperties"])
        self.assertIn("formatVersion", document["properties"])
        self.assertEqual(served["properties"]["properties"], shortcut["properties"]["properties"],
                         "the shortcut keeps its own properties schema")
        self.assertNotIn("type", schema["properties"]["properties"], "the root no longer constrains a conflicting member")
        self.assertEqual(set(), dangling(schema))

    def test_unknown_arm_and_missing_fetch_are_refused(self):
        with self.assertRaises(ValueError):
            apply_arm(golden(), "full-opaque")
        with self.assertRaises(ValueError):
            apply_arm(golden(), "full-inline")


if __name__ == "__main__":
    unittest.main()
