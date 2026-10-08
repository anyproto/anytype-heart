import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from eval_fixtures import FixtureClient, concurrent_edit
from snapshot_run import snapshot


def result(doc, etag=None):
    value = {"content": [{"type": "text", "text": json.dumps(doc)}]}
    if etag:
        value["content"].append({"type": "text", "text": json.dumps({"request_metadata": {"etag": etag}})})
    return value


class FixtureTests(unittest.TestCase):
    def test_fixture_rejects_other_space_before_request(self):
        client = FixtureClient.__new__(FixtureClient)
        client.space_id = "owned"
        client.evidence = []
        client.request = lambda *a: self.fail("must reject before sending")
        with self.assertRaises(ValueError):
            client.call("API-patch-object", {"space_id": "other", "body": {"ops": []}})
        with self.assertRaises(ValueError):
            client.call("API-delete-object", {"space_id": "owned", "object_id": "x"})

    def test_missing_prepared_content_is_not_silently_repaired(self):
        class Client:
            space_id = "owned"
            def call(inner, tool, args):
                if tool == "API-search-space":
                    return result({"data": [{"id": "bug", "name": "Lost cursor"}]})
                if tool == "API-get-object":
                    return result({"blocks": []}, '"etag"')
                self.fail("fixture must not repair missing model content")
        with self.assertRaises(ValueError):
            concurrent_edit(Client())

    def test_concurrent_edit_changes_only_expected_paragraph(self):
        calls = []
        class Client:
            space_id = "owned"
            edited = False
            def call(inner, tool, args):
                calls.append((tool, args))
                if tool == "API-search-space":
                    return result({"data": [{"id": "bug", "name": "Lost cursor"}]})
                if tool == "API-patch-object":
                    inner.edited = True
                    return result({"etag": "new"})
                return result({"blocks": [{"id": "target", "type": "paragraph", "text":
                    "Cursor stays visible on mobile" if inner.edited else "Cursor stays visible"},
                    {"id": "keep", "type": "paragraph", "text": "Keep this"}]}, '"old"')
        concurrent_edit(Client())
        mutation = next(args for tool, args in calls if tool == "API-patch-object")
        self.assertEqual(mutation["expected_etag"], '"old"')
        self.assertEqual(mutation["body"]["ops"], [{"op": "update_block", "id": "target",
                                                   "set": {"text": "Cursor stays visible on mobile"}}])

    def test_snapshot_captures_creation_hidden_by_timeout(self):
        calls = []
        class Client:
            def __init__(inner, *a, **kw):
                pass
            def call(inner, tool, args):
                calls.append((tool, args))
                return result({"id": args.get("object_id", "owned"), "type": "page"})
            def close(inner):
                pass
        with tempfile.TemporaryDirectory() as tmp:
            run = Path(tmp)
            (run / "run.json").write_text(json.dumps({"scenario_id": "MCP-14", "label": "eval",
                "status": "running", "owned_spaces": [{"full_id": "owned"}], "turns": [{"turn": 4}]}))
            (run / "tool-calls.jsonl").write_text("")
            wire = [{"direction": "to_upstream", "payload": {"id": 7, "method": "tools/call", "params": {
                "name": "API-create-object", "arguments": {"space_id": "owned", "body": {"name": "Welcome"}}}}},
                {"direction": "from_upstream", "payload": {"id": 7, "result": result({"id": "hidden-created-id"})}}]
            (run / "mcp-wire.jsonl").write_text("\n".join(json.dumps(x) for x in wire))
            with patch("snapshot_run.Reader", Client):
                snapshot(run, "codex", "anytype", allow_running=True, output_path=run / "snapshots/turn-04.json")
            self.assertIn(("API-get-object", {"space_id": "owned", "object_id": "hidden-created-id", "ids": "full"}), calls)
            captured = json.loads((run / "snapshots/turn-04.json").read_text())
            self.assertEqual(captured["capture_status"], "complete")


if __name__ == "__main__":
    unittest.main()
