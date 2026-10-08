import tempfile
import unittest
from pathlib import Path

from space_guard import Guard, documents


class GuardTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.path = Path(self.tmp.name) / "state.json"
        self.guard = Guard(self.path, "eval-luna-01")

    def test_existing_spaces_are_denied_even_before_creation(self):
        for name in ["API-patch-object", "API-delete-object", "API-update-type", "API-add-chat-message", "API-get-object"]:
            self.assertIsNotNone(self.guard.check(name, {"space_id": "7y7i74"}))

    def test_only_assigned_name_can_create(self):
        self.assertIsNone(self.guard.check("API-create-space", {"name": "eval-luna-01 — Planner", "dry_run": True}))
        self.assertIsNotNone(self.guard.check("API-create-space", {"name": "test"}))

    def test_only_exact_owned_full_or_compact_id_is_allowed(self):
        self.guard.register("bafytestabcdef.key")
        for value in ["bafytestabcdef.key", "abcdef"]:
            self.assertIsNone(self.guard.check("API-patch-object", {"space_id": value}))
        for value in ["def", "7y7i74", "bafyotherabcdef.key", None, "", ["abcdef"]]:
            self.assertIsNotNone(self.guard.check("API-patch-object", {"space_id": value}))

    def test_resume_loads_scope_and_blocks_extra_creates(self):
        self.guard.register("bafytestabcdef.key")
        resumed = Guard(self.path, "eval-luna-01")
        self.assertTrue(resumed.allowed_space("abcdef"))
        self.assertIsNotNone(resumed.check("API-create-space", {"name": "eval-luna-01 — Duplicate"}))

    def test_unknown_tools_and_missing_scope_fail_closed(self):
        self.assertIsNotNone(self.guard.check("API-delete-space", {}))
        self.assertIsNotNone(self.guard.check("API-patch-object", {}))
        self.assertIsNone(self.guard.check("API-list-schemas", {}))

    def test_result_parser_handles_all_text_content_blocks(self):
        result = {"content": [{"type": "text", "text": "not json"}, {"type": "text", "text": '{"id":"created"}'}, {"type": "text", "text": '{"request_metadata":{}}'}]}
        self.assertEqual(list(documents(result))[0]["id"], "created")


if __name__ == "__main__":
    unittest.main()
