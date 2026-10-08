#!/usr/bin/env python3
"""Stdio MCP proxy: permit scoped calls only in this run's newly created space.

The upstream is a stdio MCP server (the bridge, configured in Codex) or
heart's /mcp/full over Streamable HTTP; for the latter the arm decides
whether tools/list is forwarded as served or with the ops items untyped.

The model has no access to this process's credential configuration. Credentials
are read from the existing Codex MCP configuration and passed only to upstream.
Run state and wire traces are evaluator-owned, outside the model's working dir.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import urllib.error
import urllib.request
import uuid

from fault_hooks import FaultHooks
from tool_names import capability


SAFE_GLOBAL = {"auth_whoami", "list_spaces", "list_schemas", "get_schema",
               "get_op_schema", "validate", "search_global"}
SCOPED = set("""add_chat_message create_chat create_collection create_object
create_property create_query create_template create_type delete_chat_message
delete_object delete_property delete_type download_file edit_chat_message
get_chat_messages get_collection_objects get_collection_views get_member_me
get_object get_query_objects get_query_views get_space get_type get_type_schema
list_chats list_members list_objects list_properties list_property_options
list_types patch_object read_chat search_space toggle_chat_reaction
update_property update_space update_type upload_file""".split())


def normalized(name):
    return capability(name)


def documents(result):
    for part in result.get("content", []):
        if part.get("type") == "text":
            try:
                value = json.loads(part["text"])
            except (ValueError, KeyError):
                continue
            if isinstance(value, dict):
                yield value


class Guard:
    def __init__(self, state_path, run_label):
        self.path = Path(state_path)
        self.label = run_label
        self.state = json.loads(self.path.read_text()) if self.path.exists() else {"spaces": []}

    def allowed_space(self, value):
        # No arbitrary suffix guessing. Only full IDs and the observed six-char
        # compact spelling of an owned space are accepted.
        return isinstance(value, str) and any(value in {s["full_id"], s["compact_id"]} for s in self.state["spaces"])

    def check(self, name, args):
        name = normalized(name)
        if not isinstance(args, dict):
            return "Tool arguments must be an object."
        if name == "create_space":
            if not isinstance(args.get("name"), str) or not args["name"].startswith(self.label + " — "):
                return "New spaces must use this run's assigned name prefix."
            if self.state["spaces"]:
                return "This run already owns a space. Reuse its returned ID; do not create another."
        elif name in SCOPED:
            if not self.allowed_space(args.get("space_id")):
                return "Space is outside this run. Only the space created by this run may be read or modified."
        elif name not in SAFE_GLOBAL:
            return "Tool is not on the evaluation allowlist."
        return None

    def register(self, full_id):
        if not isinstance(full_id, str) or "." not in full_id:
            raise ValueError("create_space did not return a full space ID")
        compact = full_id.split(".", 1)[0][-6:]
        if not self.allowed_space(full_id):
            self.state["spaces"].append({"full_id": full_id, "compact_id": compact})
            tmp = self.path.with_suffix(".tmp")
            tmp.write_text(json.dumps(self.state, indent=2) + "\n")
            os.chmod(tmp, 0o600)
            tmp.replace(self.path)


def error_response(request, message):
    return {"jsonrpc": "2.0", "id": request["id"], "result": {
        "isError": True, "content": [{"type": "text", "text": json.dumps({
            "status": 403, "code": "evaluation_space_guard", "message": message,
            "issues": []})}]}}


def prepare_call(guard, request, hooks):
    """Validate ownership before canonicalizing or considering a fault."""
    params = request.get("params", {})
    args = params.get("arguments", {})
    name = normalized(params.get("name", ""))
    refusal = guard.check(name, args)
    if refusal:
        return None, error_response(request, refusal), None
    forwarded = json.loads(json.dumps(request))
    # Obtain stable full identity before allowing any subsequent write. Both
    # original and forwarded requests are retained in the evaluator wire log.
    if name == "create_space":
        forwarded["params"].setdefault("arguments", {})["ids"] = "full"
    elif name in SCOPED:
        supplied = args["space_id"]
        owned = next(s for s in guard.state["spaces"]
                     if supplied in {s["full_id"], s["compact_id"]})
        forwarded["params"]["arguments"]["space_id"] = owned["full_id"]
    forwarded, hook = hooks.prepare(request, forwarded, guard)
    return forwarded, None, hook


class StdioUpstream:
    """An upstream MCP server launched as a child process (the bridge)."""

    def __init__(self, command, args, env, cwd):
        self.proc = subprocess.Popen([command, *args], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                     stderr=sys.stderr, text=True, bufsize=1, env=env, cwd=cwd or None)

    def post(self, payload):
        self.proc.stdin.write(json.dumps(payload) + "\n")
        self.proc.stdin.flush()

    def next_message(self):
        line = self.proc.stdout.readline()
        return json.loads(line) if line else None

    def close(self):
        self.proc.terminate()
        try:
            self.proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            self.proc.kill()
            self.proc.wait()


class HttpUpstream:
    """Streamable HTTP to heart's /mcp/full: one JSON-RPC message per POST,
    the caller's bearer, JSON responses only, and no session header — the
    full tier is sessionless. A notification is answered 202 with no body."""

    def __init__(self, url, key, timeout=120):
        self.url = url
        self.key = key
        self.timeout = timeout
        self.queue = []

    def post(self, payload):
        data = json.dumps(payload).encode()
        request = urllib.request.Request(self.url, data=data, method="POST", headers={
            "Authorization": "Bearer " + self.key,
            "Content-Type": "application/json",
            "Accept": "application/json",
        })
        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as response:
                status, body = response.status, response.read()
        except urllib.error.HTTPError as error:
            status, body = error.code, error.read()
        if status == 202 or "id" not in payload:
            return
        try:
            message = json.loads(body)
        except ValueError:
            message = None
        if not isinstance(message, dict) or message.get("id") != payload["id"]:
            # an HTTP-level refusal (401, 429, 5xx) carries no JSON-RPC answer;
            # the model gets one, and the wire keeps the status and body
            text = body.decode(errors="replace")[:2000]
            message = {"jsonrpc": "2.0", "id": payload["id"], "error": {
                "code": -32000, "message": f"upstream answered HTTP {status}: {text}",
                "data": {"http_status": status}}}
        self.queue.append(message)

    def next_message(self):
        return self.queue.pop(0) if self.queue else None

    def close(self):
        pass


def make_upstream(args):
    if getattr(args, "upstream_url", None):
        key = os.environ.get(args.upstream_key_env or "ANYTYPE_API_KEY", "")
        if not key:
            raise ValueError(f"the HTTP upstream needs a key in ${args.upstream_key_env or 'ANYTYPE_API_KEY'}")
        return HttpUpstream(args.upstream_url, key)
    config = subprocess.run([args.codex, "mcp", "get", args.upstream, "--json"],
                            capture_output=True, text=True, check=True)
    transport = json.loads(config.stdout)["transport"]
    if transport["type"] != "stdio":
        raise ValueError("A configured upstream must be stdio; use --upstream-url for /mcp/full")
    env = os.environ.copy()
    env.update(transport.get("env") or {})
    for name in transport.get("env_vars") or []:
        if name in os.environ:
            env[name] = os.environ[name]
    return StdioUpstream(transport["command"], transport.get("args", []), env, transport.get("cwd"))


ARMS = ("full", "full-opaque")
OPAQUE_TOOLS = ("patch_object", "update_type")


def _refs(node, out):
    if isinstance(node, dict):
        ref = node.get("$ref")
        if isinstance(ref, str) and ref.startswith("#/$defs/"):
            out.add(ref[len("#/$defs/"):])
        for key, value in node.items():
            if key != "$defs":
                _refs(value, out)
    elif isinstance(node, list):
        for value in node:
            _refs(value, out)


def reachable_defs(schema):
    """The $defs names a schema reaches from outside $defs, transitively."""
    defs = schema.get("$defs", {})
    seen, frontier = set(), set()
    _refs(schema, frontier)
    while frontier:
        name = frontier.pop()
        if name in seen or name not in defs:
            continue
        seen.add(name)
        _refs(defs[name], frontier)
    return seen


def opaque_ops(schema):
    """The full-opaque arm: every ops.items becomes {"type": "object"} and
    the $defs only those items reached are dropped. Nothing else changes."""
    schema = json.loads(json.dumps(schema))
    holders = [schema, *[b for b in schema.get("anyOf", []) if isinstance(b, dict)]]
    changed = False
    for holder in holders:
        ops = holder.get("properties", {}).get("ops")
        if isinstance(ops, dict) and "items" in ops:
            ops["items"] = {"type": "object"}
            changed = True
    if not changed:
        raise ValueError("no ops.items to make opaque")
    if "$defs" in schema:
        keep = reachable_defs(schema)
        schema["$defs"] = {k: v for k, v in schema["$defs"].items() if k in keep}
        if not schema["$defs"]:
            del schema["$defs"]
    return schema


def apply_arm(result, arm):
    """Rewrite a tools/list result for the arm."""
    if arm == "full":
        return result
    if arm != "full-opaque":
        raise ValueError(f"unknown arm {arm!r}; arms: {', '.join(ARMS)}")
    result = json.loads(json.dumps(result))
    seen = set()
    for tool in result.get("tools", []):
        if tool.get("name") in OPAQUE_TOOLS:
            tool["inputSchema"] = opaque_ops(tool["inputSchema"])
            seen.add(tool["name"])
    if seen != set(OPAQUE_TOOLS):
        raise ValueError(f"full-opaque needs {OPAQUE_TOOLS} in tools/list; found {sorted(seen)}")
    return result


def compact_bytes(value):
    return len(json.dumps(value, ensure_ascii=False, separators=(",", ":")).encode())


def serve(args):
    guard = Guard(args.state, args.run_label)
    arm = getattr(args, "arm", None) or "full"
    upstream = make_upstream(args)
    trace = open(args.trace, "a", buffering=1)
    os.chmod(args.trace, 0o600)

    def log(direction, payload):
        trace.write(json.dumps({"at": time.time(), "direction": direction,
                                "payload": payload}, ensure_ascii=False) + "\n")

    def send(payload):
        log("to_model", payload)
        print(json.dumps(payload, ensure_ascii=False), flush=True)

    def record_tools_list(served, forwarded):
        stats = {"arm": arm, "tool_count": len(served.get("tools", [])),
                 "served_bytes": compact_bytes(served), "forwarded_bytes": compact_bytes(forwarded)}
        log("tools_list_stats", stats)
        if getattr(args, "stats", None):
            Path(args.stats).write_text(json.dumps(stats, indent=2) + "\n")

    def evaluator_probe(name, arguments):
        # These reads are evaluator-only evidence, never model calls. Recheck
        # scope at this boundary rather than inheriting authority from the hook.
        if normalized(name) not in {"get_object", "list_property_options"}:
            raise ValueError("Hook probe is not read-only allowlisted")
        refusal = guard.check(name, arguments)
        if refusal:
            raise ValueError("Hook probe refused: " + refusal)
        request = {"jsonrpc": "2.0", "id": "evaluation-probe-" + uuid.uuid4().hex,
                   "method": "tools/call", "params": {"name": name, "arguments": arguments}}
        log("evaluator_to_upstream", request)
        upstream.post(request)
        while True:
            response = upstream.next_message()
            if response is None:
                raise RuntimeError("Upstream MCP closed during evaluator probe")
            log("evaluator_from_upstream", response)
            if response.get("id") == request["id"]:
                return response
            # Preserve unsolicited upstream notifications without presenting
            # the evaluator's own request or response as a model tool call.
            send(response)

    try:
        hooks = FaultHooks(getattr(args, "hook_config", None),
                           getattr(args, "hook_state", None), log)
        for line in sys.stdin:
            request = json.loads(line)
            log("from_model", request)
            is_call = request.get("method") == "tools/call"
            params = request.get("params", {})
            name = normalized(params.get("name", "")) if is_call else ""
            call_args = params.get("arguments", {})
            hook = None
            if is_call:
                forwarded, refusal, hook = prepare_call(guard, request, hooks)
                if refusal:
                    send(refusal)
                    continue
            elif request.get("method") not in {"initialize", "ping", "tools/list", "notifications/initialized", "notifications/cancelled"}:
                if "id" in request:
                    send({"jsonrpc": "2.0", "id": request["id"], "error": {"code": -32601, "message": "Method disabled by evaluation guard"}})
                continue
            if not is_call:
                forwarded = json.loads(json.dumps(request))
            hooks.capture_atomicity_evidence(hook, "before", evaluator_probe)
            log("to_upstream", forwarded)
            upstream.post(forwarded)
            if "id" not in request:
                continue
            while True:
                response = upstream.next_message()
                if response is None:
                    raise RuntimeError("Upstream MCP closed before replying")
                log("from_upstream", response)
                if response.get("id") == request["id"]:
                    result = response.get("result", {})
                    if request.get("method") == "tools/list" and isinstance(result, dict) and "tools" in result:
                        rewritten = apply_arm(result, arm)
                        record_tools_list(result, rewritten)
                        response = {**response, "result": rewritten}
                    if name == "create_space" and not call_args.get("dry_run") and not result.get("isError"):
                        created = next((d.get("id") for d in documents(result) if d.get("id") and not d.get("dry_run")), None)
                        if created:
                            guard.register(created)
                    send(hooks.complete(hook, response, evaluator_probe))
                    break
                send(response)
    finally:
        trace.close()
        upstream.close()


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--state", required=True)
    p.add_argument("--trace", required=True)
    p.add_argument("--run-label", required=True)
    p.add_argument("--upstream", default="anytype", help="a stdio MCP server configured in Codex (the bridge)")
    p.add_argument("--upstream-url", help="Streamable HTTP upstream instead, e.g. http://127.0.0.1:31009/mcp/full")
    p.add_argument("--upstream-key-env", default="ANYTYPE_API_KEY",
                   help="environment variable holding the HTTP upstream's bearer key (never on argv)")
    p.add_argument("--arm", choices=ARMS, default="full", help="tools/list as served, or with ops items untyped")
    p.add_argument("--stats", help="write the served tools/list size and tool count here")
    p.add_argument("--codex", default="codex")
    p.add_argument("--hook-config", help="Evaluator-owned fault config; absent preserves baseline behavior")
    p.add_argument("--hook-state", help="Durable once-only hook state; defaults to hook-state.json beside config")
    serve(p.parse_args())


if __name__ == "__main__":
    main()
