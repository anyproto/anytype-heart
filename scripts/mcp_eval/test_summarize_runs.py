import json
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).parent))
from summarize_runs import summarize


class SummarizeRunsTest(unittest.TestCase):
    def make_run(self, manifest=None, calls=(), scope=None, files=()):
        root = Path(tempfile.mkdtemp())
        (root / "run.json").write_text(json.dumps(manifest or {"label": "synthetic", "status": "conversation_completed_pending_review", "turns": []}))
        if scope is not None:
            (root / "scope.json").write_text(json.dumps(scope))
        if calls:
            event_dir = root / "turn-01.events.jsonl"
            event_dir.write_text("\n".join(json.dumps({"type": "item.completed", "item": c}) for c in calls) + "\n")
        for name, value in files:
            (root / name).write_text(json.dumps(value))
        return root

    def test_snapshot_is_evidence_and_wire_unavailable(self):
        root = self.make_run(files=[("reviewer-snapshot.json", {"score": 4})])
        result = summarize(root)
        self.assertFalse(result["independent_verdict"]["available"])
        self.assertTrue(result["verification_evidence"]["reviewer-snapshot.json"]["present"])
        self.assertEqual(result["scope_audit"]["status"], "unavailable")

    def test_zero_forwarded_scoped_calls_not_exercised(self):
        root = self.make_run(scope={"spaces": []})
        (root / "mcp-wire.jsonl").write_text(json.dumps({"direction": "to_upstream", "payload": {"method": "tools/list"}}) + "\n")
        self.assertEqual(summarize(root)["scope_audit"]["status"], "not_exercised")

    def test_explicit_verdict_and_infrastructure_classification(self):
        call = {"id": "i", "type": "mcp_tool_call", "tool": "API-auth-whoami", "arguments": {"api_key": "secret"},
                "result": None, "error": {"message": "MCP tool call requires approval, but approval policy is never"}, "status": "failed"}
        root = self.make_run(manifest={"label": "synthetic", "status": "conversation_completed_pending_review", "turns": []}, calls=[call])
        (root / "reviewer-result.json").write_text(json.dumps({"outcome": "environment"}))
        result = summarize(root)
        self.assertEqual(result["independent_verdict"]["outcome"], "environment")
        self.assertEqual(result["completion"]["effective_classification"], "infrastructure_failure")
        self.assertEqual(result["failed_calls"][0]["args"], {})


if __name__ == "__main__":
    unittest.main()
