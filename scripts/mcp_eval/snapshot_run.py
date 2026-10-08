#!/usr/bin/env python3
"""Read final objects and stored views through the same run's MCP guard.

This evaluator helper is never exposed to the evaluated model. It makes only
read calls and stores responses separately from the model's original trace.
"""
import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import selectors
import shutil
import subprocess
import sys

from space_guard import documents
from tool_names import capability, vocabulary_for


class Reader:
    def __init__(self, run_dir, manifest, codex, upstream, *, trace_name="reviewer-mcp-wire.jsonl",
                 stderr_name="reviewer.stderr.log"):
        self.serial = 0
        self.stderr = open(run_dir / stderr_name, "a")
        self.proc = subprocess.Popen([
            sys.executable, str(Path(__file__).with_name("space_guard.py")),
            "--state", str(run_dir / "scope.json"),
            "--trace", str(run_dir / trace_name),
            "--run-label", manifest["label"], "--codex", codex,
            "--upstream", upstream,
        ], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=self.stderr,
           text=True, bufsize=1)
        self.selector = selectors.DefaultSelector()
        self.selector.register(self.proc.stdout, selectors.EVENT_READ)
        self.request("initialize", {
            "protocolVersion": "2024-11-05", "capabilities": {},
            "clientInfo": {"name": "anytype-eval-reviewer", "version": "1"},
        })
        self.proc.stdin.write(json.dumps({"jsonrpc": "2.0",
                                         "method": "notifications/initialized"}) + "\n")
        self.proc.stdin.flush()

    def request(self, method, params):
        self.serial += 1
        self.proc.stdin.write(json.dumps({"jsonrpc": "2.0", "id": self.serial,
                                         "method": method, "params": params}) + "\n")
        self.proc.stdin.flush()
        while True:
            if not self.selector.select(timeout=120):
                raise TimeoutError("Reviewer MCP response timed out")
            line = self.proc.stdout.readline()
            if not line:
                raise RuntimeError("Reviewer MCP closed before replying")
            response = json.loads(line)
            if response.get("id") == self.serial:
                if "error" in response:
                    raise RuntimeError(response["error"])
                return response["result"]

    READ_ONLY = {"get_space", "get_object", "get_type", "search_space", "get_query_objects",
                 "get_collection_objects", "get_chat_messages", "list_property_options"}

    def call(self, tool, arguments):
        """Call a read-only tool, named in the run's surface's spelling."""
        if capability(tool) not in self.READ_ONLY:
            raise ValueError("Reviewer tool is not read-only allowlisted")
        return self.request("tools/call", {"name": tool, "arguments": arguments})

    def close(self):
        self.proc.stdin.close()
        try:
            self.proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            self.proc.terminate()
            self.proc.wait(timeout=10)
        self.selector.close()
        self.stderr.close()


def snapshot(run_dir, codex, upstream, *, output_path=None, allow_running=False):
    manifest = json.loads((run_dir / "run.json").read_text())
    if manifest["status"] == "running" and not allow_running:
        raise ValueError("Wait for this conversation to finish before snapshotting")
    spaces = manifest.get("owned_spaces", [])
    if len(spaces) != 1:
        raise ValueError("A snapshot requires exactly one owned space")
    space_id = spaces[0]["full_id"]
    output = {"scenario_id": manifest["scenario_id"], "label": manifest["label"],
              "recorded_at": datetime.now(timezone.utc).isoformat(),
              "purpose": "Independent final reads; not an automatic semantic verdict",
              "after_turn": manifest.get("turns", [{}])[-1].get("turn"),
              "capture_status": "incomplete", "reads": []}
    output_path = output_path or run_dir / "final-state.json"
    output_path.parent.mkdir(parents=True, exist_ok=True)
    client = Reader(run_dir, manifest, codex, upstream)

    vocab = vocabulary_for(manifest)

    def read(cap, **args):
        args["space_id"] = space_id
        tool = vocab.tool(cap)
        response = client.call(tool, args)
        output["reads"].append({"tool": tool, "arguments": args, "result": response})
        return next(documents(response), {})

    try:
        read("get_space", ids="full")
        authored = {}
        types = set()
        calls = [json.loads(line) for line in (run_dir / "tool-calls.jsonl").read_text().splitlines()]
        # An injected lost-response fault can hide a successful creation from
        # the model trace. Keep its actual server receipt in evaluator coverage.
        wire = run_dir / "mcp-wire.jsonl"
        pending = {}
        if wire.exists():
            for line in wire.read_text().splitlines():
                event = json.loads(line)
                payload = event.get("payload", {})
                if event.get("direction") == "to_upstream" and payload.get("method") == "tools/call":
                    pending[payload["id"]] = payload.get("params", {})
                elif event.get("direction") == "from_upstream" and payload.get("id") in pending:
                    params = pending.pop(payload["id"])
                    result = payload.get("result", {})
                    if not payload.get("error") and not result.get("isError"):
                        calls.append({"tool": params.get("name"), "arguments": params.get("arguments", {}),
                                      "status": "completed", "result": result})
        chats = set()
        for call in calls:
            if call.get("status") != "completed" or call.get("arguments", {}).get("dry_run"):
                continue
            for receipt in documents(call.get("result") or {}):
                object_id = receipt.get("id")
                cap = capability(call["tool"])
                if cap == "create_type" and receipt.get("key"):
                    types.add(receipt["key"])
                if object_id and cap == "create_chat":
                    chats.add(object_id)
                if object_id and cap in {"create_object", "create_template",
                                         "create_query", "create_collection"}:
                    authored[object_id] = call["tool"]
        for type_key in sorted(types):
            read("get_type", type=type_key, ids="full")
            offset = 0
            while True:
                page = read("search_space", type=type_key, limit=100, offset=offset)
                if not page.get("has_more"):
                    break
                rows = page.get("data", [])
                if not rows:
                    raise ValueError("Type inventory pagination reports more data without rows")
                offset += len(rows)
        for object_id, creation in authored.items():
            doc = read("get_object", object_id=object_id, ids="full")
            # Generic create_object can also author a query/collection document.
            if doc.get("type") not in {"query", "collection"}:
                continue
            is_query = doc.get("type") == "query"
            tool = "get_query_objects" if is_query else "get_collection_objects"
            id_arg = "query_id" if is_query else "collection_id"
            views = [v for b in doc.get("blocks", []) for v in b.get("views", [])]
            # Membership/source results and every stored view are separate facts.
            for view in [None, *[v["id"] for v in views]]:
                offset = 0
                while True:
                    args = {id_arg: object_id, "limit": 100, "offset": offset}
                    if view:
                        args["view"] = view
                    page = read(tool, **args)
                    if not page.get("has_more"):
                        break
                    rows = page.get("data", [])
                    if not rows:
                        raise ValueError("Pagination reports more data without rows")
                    offset += len(rows)
        for chat_id in sorted(chats):
            before = None
            seen = set()
            while True:
                args = {"chat_id": chat_id, "limit": 100, "reactions": "full"}
                if before:
                    args["before"] = before
                page = read("get_chat_messages", **args)
                if not page.get("has_more"):
                    break
                cursor = page.get("next_before")
                if not cursor or cursor in seen:
                    raise ValueError("Chat pagination has no new order cursor")
                seen.add(cursor)
                before = cursor
        output["capture_status"] = "complete"
    finally:
        client.close()
        output_path.write_text(json.dumps(output, ensure_ascii=False, indent=2) + "\n")
    return len(output["reads"])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("run_dir", type=Path, nargs="+")
    parser.add_argument("--codex", default="codex")
    parser.add_argument("--upstream", default="anytype")
    args = parser.parse_args()
    codex = shutil.which(args.codex)
    if not codex:
        parser.error("Codex executable not found")
    for run_dir in args.run_dir:
        count = snapshot(run_dir, codex, args.upstream)
        print(f"{run_dir.name}: {count} independent reads saved", flush=True)


if __name__ == "__main__":
    main()
