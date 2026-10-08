"""Evaluator-only fixtures. Nothing here is exposed as a model tool.

An independent client may change only the owned evaluation space. Desktop/peer
fixtures require an actual external receipt and are never simulated by editing
author properties or inventing a second participant.
"""
from datetime import datetime, timezone
import json
from pathlib import Path
import time

from snapshot_run import Reader
from space_guard import documents
from tool_names import capability, vocabulary_for


def write_json(path, value):
    temp = path.with_suffix(".tmp")
    temp.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")
    temp.replace(path)


def owned_space(manifest):
    spaces = manifest.get("owned_spaces", [])
    if len(spaces) != 1:
        raise ValueError("Fixture requires one newly created owned space")
    return spaces[0]["full_id"]


class FixtureClient(Reader):
    def __init__(self, run_dir, manifest, codex, upstream, evidence):
        self.space_id = owned_space(manifest)
        self.evidence = evidence
        self.vocab = vocabulary_for(manifest)
        super().__init__(run_dir, manifest, codex, upstream,
                         trace_name="fixture-mcp-wire.jsonl", stderr_name="fixture.stderr.log")

    ALLOWED = {"search_space", "get_object", "patch_object", "delete_object",
               "get_chat_messages", "get_member_me"}

    def call(self, cap, arguments, body=None):
        """Call capability `cap`; body members are nested or flattened as
        this run's surface takes them."""
        if arguments.get("space_id") != self.space_id:
            raise ValueError("Fixture cannot address another space")
        cap = capability(cap)
        if cap not in self.ALLOWED:
            raise ValueError("Fixture tool is not allowlisted")
        if cap == "delete_object" and arguments.get("dry_run") is not True:
            raise ValueError("Fixture may only preview object deletion")
        arguments = self.vocab.arguments(arguments, body)
        tool = self.vocab.tool(cap)
        result = self.request("tools/call", {"name": tool, "arguments": arguments})
        self.evidence.append({"tool": tool, "arguments": arguments, "result": result})
        return result


def checked_document(result):
    if result.get("isError"):
        raise RuntimeError("Fixture API call failed: " + json.dumps(result))
    return next(documents(result), {})


def exact_named_object(client, name):
    result = checked_document(client.call("search_space", {
        "space_id": client.space_id, "query": name, "limit": 100, "fields": ["name"]}))
    hits = [x for x in result.get("data", []) if x.get("name") == name or x.get("properties", {}).get("name") == name]
    if len(hits) != 1:
        raise ValueError(f"Fixture requires exactly one existing {name!r}; found {len(hits)}")
    return hits[0]["id"]


def concurrent_edit(client):
    object_id = exact_named_object(client, "Lost cursor")
    before = client.call("get_object", {"space_id": client.space_id, "object_id": object_id, "ids": "full"})
    doc = checked_document(before)
    matches = [b for b in doc.get("blocks", []) if b.get("type") == "paragraph" and
               isinstance(b.get("text"), str) and b["text"].count("Cursor stays visible") == 1]
    if len(matches) != 1:
        raise ValueError("Prepared bug lacks the unique expected-result paragraph; fixture must not repair model setup")
    etags = [d["request_metadata"]["etag"] for d in documents(before)
             if "etag" in d.get("request_metadata", {})]
    if len(etags) != 1:
        raise ValueError("Fixture needs the actual object etag")
    new_text = matches[0]["text"].replace("Cursor stays visible", "Cursor stays visible on mobile", 1)
    checked_document(client.call("patch_object", {
        "space_id": client.space_id, "object_id": object_id, "expected_etag": etags[0]},
        body={"ops": [{"op": "update_block", "id": matches[0]["id"], "set": {"text": new_text}}]}))
    after = checked_document(client.call("get_object", {"space_id": client.space_id, "object_id": object_id, "ids": "full"}))
    if not any(b.get("id") == matches[0]["id"] and b.get("text") == new_text for b in after.get("blocks", [])):
        raise ValueError("Concurrent fixture edit was not verified")


def wait_for_receipt(run_dir, manifest, kind, turn):
    request = {"kind": kind, "scenario_id": manifest["scenario_id"], "before_turn": turn,
               "space_id": owned_space(manifest), "run_label": manifest["label"],
               "requested_at": datetime.now(timezone.utc).isoformat(), "status": "waiting_for_real_fixture"}
    path = run_dir / f"fixture-{kind}-receipt.json"
    write_json(run_dir / "fixture-request.json", request)
    # The supervising agent services this explicit request. A timeout does not
    # create a fake fixture or tell the model it exists.
    deadline = time.monotonic() + 6 * 3600
    while time.monotonic() < deadline:
        if path.exists():
            receipt = json.loads(path.read_text())
            if receipt.get("space_id") != request["space_id"]:
                raise ValueError("Fixture receipt addresses another space")
            return receipt
        time.sleep(2)
    raise TimeoutError("Real fixture was not supplied; retained run may be resumed after inspection")


def before_turn(run_dir, manifest, turn, codex, upstream):
    sid = manifest["scenario_id"]
    kind = {("MCP-18", 6): "concurrent_edit", ("MCP-28", 7): "desktop_provenance",
            ("MCP-30", 5): "peer_history", ("MCP-30", 6): "peer_arrival"}.get((sid, turn))
    if kind is None:
        return
    path = run_dir / "fixture-evidence.json"
    evidence = json.loads(path.read_text()) if path.exists() else {"fixtures": []}
    if any(x.get("kind") == kind and x.get("status") == "triggered" for x in evidence["fixtures"]):
        return
    entry = {"kind": kind, "before_turn": turn, "status": "prepared", "calls": []}
    evidence["fixtures"].append(entry)
    write_json(path, evidence)
    client = FixtureClient(run_dir, manifest, codex, upstream, entry["calls"])
    try:
        if kind == "concurrent_edit":
            concurrent_edit(client)
        else:
            receipt = wait_for_receipt(run_dir, manifest, kind, turn)
            entry["receipt"] = receipt
            if receipt.get("status") == "fixture_unavailable":
                if kind == "desktop_provenance":
                    raise ValueError("Cannot deliver the imported-note turn without a real foreign-provenance note")
                entry["status"] = "fixture_unavailable"
                manifest["hook_status"] = "fixture_unavailable"
                return
            if kind == "desktop_provenance":
                object_id = exact_named_object(client, "Original lease notes")
                if object_id != receipt.get("object_id"):
                    raise ValueError("Desktop fixture receipt does not match the observed object")
                refusal = client.call("delete_object", {"space_id": client.space_id, "object_id": object_id, "dry_run": True})
                if not any(d.get("code") == "not_created_by_this_key" for d in documents(refusal)):
                    raise ValueError("Desktop object provenance refusal was not verified")
            else:
                # Actual peer identity and unread invariants are reviewed from
                # the supplied raw peer receipts plus this independent read.
                current = checked_document(client.call("get_member_me", {"space_id": client.space_id}))
                if not receipt.get("peer_id") or receipt["peer_id"] == current.get("id"):
                    raise ValueError("A distinct actual peer identity is required")
                page = checked_document(client.call("get_chat_messages", {
                    "space_id": client.space_id, "chat_id": receipt["chat_id"], "limit": 100, "reactions": "full"}))
                peers = [m for m in page.get("messages", []) if m.get("author_id") == receipt["peer_id"]]
                expected = "Update 27" if kind == "peer_history" else "Update 28"
                if not any(m.get("text") == expected for m in peers):
                    raise ValueError("Peer authored history/arrival not observed")
        entry["status"] = "triggered"
        manifest["hook_status"] = "triggered"
    except Exception as exc:
        entry.update(status="fixture_failed", error=str(exc))
        raise
    finally:
        client.close()
        write_json(path, evidence)


def finalize_turn(run_dir, manifest, turn):
    profiles = {"MCP-14": ("mcp14_welcome_timeout", 4),
                "MCP-27": ("mcp27_atomic_patch_failure", 6),
                "MCP-29": ("mcp29_reaction_timeout", 4)}
    selected = profiles.get(manifest["scenario_id"])
    if not selected or turn < selected[1]:
        return
    path = run_dir / "hook-state.json"
    state = json.loads(path.read_text()) if path.exists() else {"version": 1, "hooks": {}}
    record = state["hooks"].setdefault(selected[0], {"status": "not_triggered", "attempts": []})
    if record["status"] == "armed":
        record["status"] = "not_triggered"
        record["reason"] = "No eligible call committed during the scheduled model turn"
    manifest["hook_status"] = record["status"]
    write_json(path, state)
