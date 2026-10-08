#!/usr/bin/env python3
"""Stdio MCP proxy: permit scoped calls only in this run's newly created space.

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
import uuid

from fault_hooks import FaultHooks


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
    return name.removeprefix("mcp__anytype__").replace("-", "_").removeprefix("API_")


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


def serve(args):
    guard = Guard(args.state, args.run_label)
    config = subprocess.run([args.codex, "mcp", "get", args.upstream, "--json"],
                            capture_output=True, text=True, check=True)
    transport = json.loads(config.stdout)["transport"]
    if transport["type"] != "stdio":
        raise ValueError("This runner currently requires an existing stdio MCP configuration")
    env = os.environ.copy()
    env.update(transport.get("env") or {})
    for name in transport.get("env_vars") or []:
        if name in os.environ:
            env[name] = os.environ[name]
    upstream = subprocess.Popen([transport["command"], *transport.get("args", [])],
                                stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                stderr=sys.stderr, text=True, bufsize=1,
                                env=env, cwd=transport.get("cwd") or None)
    trace = open(args.trace, "a", buffering=1)
    os.chmod(args.trace, 0o600)

    def log(direction, payload):
        trace.write(json.dumps({"at": time.time(), "direction": direction,
                                "payload": payload}, ensure_ascii=False) + "\n")

    def send(payload):
        log("to_model", payload)
        print(json.dumps(payload, ensure_ascii=False), flush=True)

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
        upstream.stdin.write(json.dumps(request) + "\n")
        upstream.stdin.flush()
        while True:
            incoming = upstream.stdout.readline()
            if not incoming:
                raise RuntimeError("Upstream MCP closed during evaluator probe")
            response = json.loads(incoming)
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
            upstream.stdin.write(json.dumps(forwarded) + "\n")
            upstream.stdin.flush()
            if "id" not in request:
                continue
            while True:
                incoming = upstream.stdout.readline()
                if not incoming:
                    raise RuntimeError("Upstream MCP closed before replying")
                response = json.loads(incoming)
                log("from_upstream", response)
                if response.get("id") == request["id"]:
                    result = response.get("result", {})
                    if name == "create_space" and not call_args.get("dry_run") and not result.get("isError"):
                        created = next((d.get("id") for d in documents(result) if d.get("id") and not d.get("dry_run")), None)
                        if created:
                            guard.register(created)
                    send(hooks.complete(hook, response, evaluator_probe))
                    break
                send(response)
    finally:
        trace.close()
        upstream.terminate()
        try:
            upstream.wait(timeout=5)
        except subprocess.TimeoutExpired:
            upstream.kill()
            upstream.wait()


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--state", required=True)
    p.add_argument("--trace", required=True)
    p.add_argument("--run-label", required=True)
    p.add_argument("--upstream", default="anytype")
    p.add_argument("--codex", default="codex")
    p.add_argument("--hook-config", help="Evaluator-owned fault config; absent preserves baseline behavior")
    p.add_argument("--hook-state", help="Durable once-only hook state; defaults to hook-state.json beside config")
    serve(p.parse_args())


if __name__ == "__main__":
    main()
