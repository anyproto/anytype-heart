import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from run_campaign import ROOT, scenario_set_hash
from summarize_runs import breakdown_markdown, error_category, host_arm_breakdown, rate_limit_refusals


def tool_result(doc=None, is_error=False, text=None):
    return {"content": [{"type": "text", "text": text if text is not None else json.dumps(doc)}], "isError": is_error}


def failed(doc=None, text=None):
    return {"status": "failed", "result": tool_result(doc, True, text)}


class ErrorCategoryTests(unittest.TestCase):
    def test_categories(self):
        cases = {
            "guard_refusal": failed({"status": 403, "code": "evaluation_space_guard", "message": "x", "issues": []}),
            "rate_limit": failed({"status": 429, "code": "rate_limit_exceeded", "message": "slow down"}),
            "envelope": failed({"status": 400, "code": "validation_failed", "message": "bad op",
                                "issues": [{"path": "/ops/0", "message": "unknown op \"set\""}]}),
            "locator": failed({"status": 400, "code": "validation_failed", "message": "x",
                               "issues": [{"path": "/ops/0/id", "message": "no block with id b9"}]}),
            "payload": failed({"status": 400, "code": "validation_failed", "message": "x",
                               "issues": [{"path": "/ops/0/set/due", "message": "not a date"}]}),
            "semantic": failed({"status": 404, "code": "not_found", "message": "no such object"}),
        }
        for want, call in cases.items():
            self.assertEqual(want, error_category(call), want)
        self.assertEqual("envelope", error_category(failed(text='patch_object does not take "body" — arguments: …')))
        self.assertEqual("rate_limit", error_category({"status": "failed", "error": {"data": {"http_status": 429}}, "result": {}}))

    def test_the_full_tiers_rendered_refusals_are_categorized(self):
        # verbatim shapes from the first dry run against /mcp/full
        fields = failed(text='unknown property keys\n  /fields/3: unknown property key "properties" — known property keys: active, added_date')
        include = failed(text='invalid include value\n  include: unknown value "collection_items" (allowed: properties, blocks)')
        op = failed(text='unknown op\n  /ops/0: unknown op "set" (send one of set_properties, update_block)')
        block = failed(text='edit refused\n  /ops/0/id: no block with id "b9"')
        self.assertEqual("payload", error_category(fields))
        self.assertEqual("payload", error_category(include))
        self.assertEqual("envelope", error_category(op))
        self.assertEqual("locator", error_category(block))


def write_run(root, name, host, arm, status, usage, calls, tools_list_bytes=None):
    run = root / name
    run.mkdir()
    manifest = {"host": host, "arm": arm, "scenario_id": "MCP-01", "status": status,
                "turns": [{"turn": 1, "usage": usage}]}
    if tools_list_bytes:
        manifest["tools_list"] = {"forwarded_bytes": tools_list_bytes}
    (run / "run.json").write_text(json.dumps(manifest))
    events = [{"type": "item.completed", "item": {"type": "mcp_tool_call", "tool": tool, **outcome}} for tool, outcome in calls]
    (run / "turn-01.events.jsonl").write_text("".join(json.dumps(e) + "\n" for e in events))
    return run


class BreakdownTests(unittest.TestCase):
    def test_groups_tokens_errors_and_first_attempts(self):
        ok = {"status": "completed", "result": tool_result({"id": "x"})}
        envelope_error = failed({"status": 400, "code": "validation_failed", "message": "x", "issues": [{"path": "/ops/0", "message": "unknown op"}]})
        limited = failed({"status": 429, "code": "rate_limit_exceeded", "message": "x"})
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            a = write_run(root, "a", "claude", "full", "conversation_completed_pending_review",
                          {"input_tokens": 100, "cached_input_tokens": 40, "output_tokens": 10},
                          [("patch_object", envelope_error), ("patch_object", ok), ("create_space", ok)], 254746)
            b = write_run(root, "b", "codex", "full-opaque", "runtime_failed",
                          {"input_tokens": 50, "cached_input_tokens": 0, "output_tokens": 5},
                          [("patch_object", limited)], 120000)
            rows = {(r["host"], r["arm"]): r for r in host_arm_breakdown([a, b])}
            full = rows[("claude", "full")]
            self.assertEqual({"input_tokens": 100, "output_tokens": 10, "cache_read_tokens": 40}, full["tokens"])
            self.assertEqual(full["tokens"], {k: v for k, v in full["tokens_per_completed_scenario"].items()})
            self.assertEqual(1, full["errors_by_category"]["envelope"])
            self.assertEqual({"attempts": 1, "successes": 0}, full["first_attempt_success"]["patch_object"],
                             "the retry after a failure is not a first attempt")
            self.assertEqual([254746], full["tools_list_bytes"])
            opaque = rows[("codex", "full-opaque")]
            self.assertEqual(0, opaque["completed_runs"])
            self.assertEqual({}, opaque["tokens_per_completed_scenario"])
            self.assertEqual(1, opaque["errors_by_category"]["rate_limit"])
            self.assertEqual(1, rate_limit_refusals(b))
            self.assertIn("| claude | full | 1 | 1 | 3 |", breakdown_markdown(list(rows.values())))


class CampaignTests(unittest.TestCase):
    def test_scenario_set_hash_is_order_and_spelling_stable(self):
        a = [{"id": "MCP-01", "turns": [1, 2]}]
        self.assertEqual(scenario_set_hash(a), scenario_set_hash(json.loads(json.dumps(a, indent=4))))
        self.assertNotEqual(scenario_set_hash(a), scenario_set_hash([{"id": "MCP-02", "turns": [1, 2]}]))

    def test_output_inside_the_repository_is_refused(self):
        proc = subprocess.run([sys.executable, str(Path(__file__).with_name("run_campaign.py")),
                               "--output", str(ROOT / "campaign-out")], capture_output=True, text=True)
        self.assertNotEqual(0, proc.returncode)
        self.assertIn("outside the repository", proc.stderr)
        self.assertFalse((ROOT / "campaign-out").exists())


if __name__ == "__main__":
    unittest.main()
