"""Evaluator-owned, once-only faults for the raw Anytype MCP surface.

Config is reloaded for every call, and must be replaced atomically by the runner:
  {"scenario_id": "MCP-14", "current_turn": 4,
   "armed_profiles": ["mcp14_welcome_timeout"]}

The other profiles are mcp27_atomic_patch_failure (turn 6) and
mcp29_reaction_timeout (turn 4). Optional stage_property_keys supplies additional
Stage identifiers for MCP-27. Config/state must remain outside the model's
workspace. A run has one serial guard process at a time.

Claims are durable before forwarding. If the process dies before recording a
response, the next guard marks that claim fixture_unavailable and never injects
it again: the upstream outcome is unknown. This deliberately favors at-most-once
injection over falsely claiming an exactly-once fault across a process crash.
"""
from copy import deepcopy
import json
import os
from pathlib import Path
import re
import tempfile
import time


PROFILES = {
    "mcp14_welcome_timeout": ("MCP-14", 4),
    "mcp27_atomic_patch_failure": ("MCP-27", 6),
    "mcp29_reaction_timeout": ("MCP-29", 4),
}
INVALID_BLOCK_ID = "evaluationFaultMissingBlockMCP27"


def atomic_json(path, value):
    """Replace private evaluator state without leaving a partial JSON file."""
    path = Path(path)
    fd, temporary = tempfile.mkstemp(prefix=path.name + ".", dir=path.parent)
    try:
        with os.fdopen(fd, "w") as out:
            json.dump(value, out, ensure_ascii=False, indent=2)
            out.write("\n")
            out.flush()
            os.fsync(out.fileno())
        os.replace(temporary, path)
        directory = os.open(path.parent, os.O_RDONLY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def response_documents(response):
    result = response.get("result")
    if not isinstance(result, dict):
        return
    if isinstance(result.get("structuredContent"), dict):
        yield result["structuredContent"]
    content = result.get("content", [])
    for part in content if isinstance(content, list) else []:
        if not isinstance(part, dict) or part.get("type") != "text":
            continue
        try:
            value = json.loads(part["text"])
        except (ValueError, KeyError, TypeError):
            continue
        if isinstance(value, dict):
            yield value


def successful_commit(profile, response):
    """Require a mutation receipt, not merely the absence of a JSON-RPC error."""
    result = response.get("result")
    if "error" in response or not isinstance(result, dict) or result.get("isError"):
        return False
    docs = list(response_documents(response))
    for doc in docs:
        status = doc.get("status")
        if (isinstance(status, (int, float)) and status >= 400) or status == "error" or doc.get("dry_run") or doc.get("error"):
            return False
    if profile == "mcp14_welcome_timeout":
        return any(isinstance(doc.get("id"), str) and doc["id"] for doc in docs)
    if profile == "mcp29_reaction_timeout":
        return any(isinstance(doc.get("added"), bool) for doc in docs)
    return False


def edited_stage_keys(args, stage_keys=()):
    """Return Stage identifiers used for Ready, including configured opaque IDs."""
    body = args.get("body")
    ops = body.get("ops") if isinstance(body, dict) else None
    keys = []
    for op in ops if isinstance(ops, list) else []:
        if not isinstance(op, dict):
            continue
        values = op.get("set")
        if op.get("op") == "set_properties" and isinstance(values, dict):
            for key, value in values.items():
                is_stage = key in stage_keys or "stage" in re.split(r"[^a-z0-9]+", key.lower())
                if is_stage and (value == "Ready" or value == ["Ready"]):
                    keys.append(key)
    return list(dict.fromkeys(keys))


def summary_op_index(args, stage_keys=()):
    """Conservatively recognize the requested Stage+summary atomic batch."""
    body = args.get("body")
    ops = body.get("ops") if isinstance(body, dict) else None
    if not isinstance(ops, list) or len(ops) < 2 or not edited_stage_keys(args, stage_keys):
        return None
    for index, op in enumerate(ops):
        if not isinstance(op, dict):
            continue
        values = op.get("set")
        if op.get("op") == "replace_text" and op.get("replace") == "Final summary":
            return index
        elif op.get("op") == "update_block" and isinstance(values, dict) and values.get("text") == "Final summary":
            return index
    return None


def eligible(profile, config, name, args):
    if (config.get("scenario_id"), config.get("current_turn")) != PROFILES[profile]:
        return False
    if args.get("dry_run"):
        return False
    if profile == "mcp14_welcome_timeout":
        body = args.get("body")
        properties = body.get("properties") if isinstance(body, dict) else None
        return name == "create_object" and isinstance(properties, dict) and properties.get("name") == "Welcome"
    if profile == "mcp29_reaction_timeout":
        return name == "toggle_chat_reaction" and args.get("emoji") in ("👍", "👍\ufe0f")
    return name == "patch_object" and summary_op_index(args, config.get("stage_property_keys", [])) is not None


class FaultHooks:
    def __init__(self, config_path, state_path=None, log=None):
        self.config_path = Path(config_path) if config_path else None
        self.path = Path(state_path) if state_path else (self.config_path.with_name("hook-state.json") if self.config_path else None)
        self.log = log or (lambda _direction, _payload: None)
        self.state = json.loads(self.path.read_text()) if self.path and self.path.exists() else {"version": 1, "hooks": {}}
        if self.config_path:
            for profile, record in self.state["hooks"].items():
                if record.get("phase") in {"awaiting_upstream", "response_received"}:
                    record.update(status="fixture_unavailable", phase="interrupted", reason="Guard restarted during a claimed injection; delivery or upstream outcome is uncertain. Claim remains consumed.")
                    self._event(profile, "interrupted", record)
            self.refresh()

    def _save(self):
        atomic_json(self.path, self.state)

    def _event(self, profile, event, details):
        self._save()
        self.log("hook", {"event": event, "profile": profile, **deepcopy(details)})

    def refresh(self):
        if not self.config_path:
            return {"armed_profiles": []}
        config = json.loads(self.config_path.read_text())
        profiles = config.get("armed_profiles", [])
        if not isinstance(profiles, list) or any(p not in PROFILES for p in profiles):
            raise ValueError("Unknown or malformed controlled hook profile")
        for profile in profiles:
            if config.get("scenario_id") != PROFILES[profile][0]:
                raise ValueError("Controlled hook profile does not match scenario_id")
            if profile not in self.state["hooks"]:
                record = {"status": "armed", "phase": "waiting", "consumed": False,
                          "scenario_id": config["scenario_id"], "armed_turn": config.get("current_turn"),
                          "attempts": []}
                self.state["hooks"][profile] = record
                self._event(profile, "armed", record)
        return config

    def prepare(self, request, forwarded, guard):
        """Called only after scope validation; independently recheck before a claim."""
        if not self.config_path or request.get("method") != "tools/call" or "id" not in request:
            return forwarded, None
        # Keep this import out of module initialization: space_guard imports us.
        from space_guard import normalized
        params = request.get("params", {})
        args = params.get("arguments", {})
        name = normalized(params.get("name", ""))
        forwarded_args = forwarded.get("params", {}).get("arguments", {})
        if guard.check(name, args) or not guard.allowed_space(args.get("space_id")) or not guard.allowed_space(forwarded_args.get("space_id")):
            return forwarded, None
        config = self.refresh()
        for profile in config["armed_profiles"]:
            record = self.state["hooks"][profile]
            if record.get("consumed") or not eligible(profile, config, name, args):
                continue
            forwarded = deepcopy(forwarded)
            attempt = {"call_id": request["id"], "turn": config["current_turn"],
                       "at": time.time(), "original_request": deepcopy(request),
                       "caller_request_key": args.get("request_key")}
            if profile == "mcp27_atomic_patch_failure":
                index = summary_op_index(args, config.get("stage_property_keys", []))
                op = forwarded["params"]["arguments"]["body"]["ops"][index]
                op["id"] = INVALID_BLOCK_ID
                # update_block accepts id OR match, never both. replace_text's
                # find remains required text to replace, even when id is given.
                if op.get("op") == "update_block":
                    op.pop("match", None)
                attempt["mutated_op_index"] = index
                attempt["stage_property_keys"] = edited_stage_keys(args, config.get("stage_property_keys", []))
            attempt["forwarded_request"] = deepcopy(forwarded)
            record["attempts"].append(attempt)
            record.update(consumed=True, phase="awaiting_upstream")
            self._event(profile, "prepared", {"scenario_id": config["scenario_id"], **attempt})
            return forwarded, profile
        return forwarded, None

    def capture_atomicity_evidence(self, profile, phase, probe):
        """Read saved object/vocabulary before the fault and before model retry."""
        if profile != "mcp27_atomic_patch_failure" or probe is None:
            return
        attempt = self.state["hooks"][profile]["attempts"][-1]
        args = attempt["forwarded_request"]["params"]["arguments"]
        reads = [("API-get-object", {"space_id": args["space_id"], "object_id": args["object_id"], "ids": "full"})]
        reads += [("API-list-property-options", {"space_id": args["space_id"], "key": key})
                  for key in attempt["stage_property_keys"]]
        evidence = attempt.setdefault("atomicity_evidence", {}).setdefault(phase, [])
        for name, arguments in reads:
            response = probe(name, arguments)
            evidence.append({"at": time.time(), "tool": name, "arguments": arguments,
                             "response": deepcopy(response)})
            self._event(profile, "atomicity_probe", {"call_id": attempt["call_id"], "turn": attempt["turn"],
                                                    "phase": phase, "read": evidence[-1]})

    def complete(self, profile, response, probe=None):
        if profile is None:
            return response
        record = self.state["hooks"][profile]
        attempt = record["attempts"][-1]
        attempt["actual_response"] = deepcopy(response)
        if profile == "mcp27_atomic_patch_failure":
            record["phase"] = "response_received"
            self._save()
            self.capture_atomicity_evidence(profile, "after", probe)
            delivered = response
            record.update(status="triggered", phase="response_recorded")
        elif successful_commit(profile, response):
            delivered = {"jsonrpc": "2.0", "id": response["id"], "error": {
                "code": -32001,
                "message": "Injected evaluation transport timeout: the upstream response was withheld. The write outcome is unknown to the caller.",
                "data": {"code": "evaluation_injected_transport_timeout", "injected": True, "profile": profile}}}
            record.update(status="triggered", phase="response_recorded")
        else:
            delivered = response
            # An actual server refusal is not an injected timeout. A corrected
            # subsequent call may still exercise the scheduled commit fault.
            record.update(status="armed", consumed=False, phase="upstream_not_committed")
        attempt["model_response"] = deepcopy(delivered)
        self._event(profile, "triggered" if record["status"] == "triggered" else "upstream_not_committed",
                    {"scenario_id": record["scenario_id"], **attempt})
        return delivered
