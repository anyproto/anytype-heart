from copy import deepcopy
import json
import unittest

from review_dedup import alias_snapshot_reads, read_ranges
from review_codex import prompt_for


class SnapshotAliasTest(unittest.TestCase):
    def read(self):
        return {"tool": "API-get-object", "arguments": {"space_id": "owned", "object_id": "one"},
                "result": {"content": [{"type": "text", "text": '{"name":"Café\\nnotes","blocks":[]}'},
                                       {"type": "text", "text": '{"request_metadata":{"etag":"first"}}'}]}}

    def source(self, reads, turn):
        text = json.dumps({"after_turn": turn, "recorded_at": f"observation-{turn}", "reads": reads},
                          ensure_ascii=False, indent=2) + "\n"
        return {"text": text, "included_lines": list(range(1, len(text.splitlines()) + 1)), "sha256": "test"}

    def test_repeated_read_keeps_exact_source_values_headers_and_valid_reference(self):
        sources = {"run/snapshots/turn-01.json": self.source([self.read()], 1),
                   "run/snapshots/turn-02.json": self.source([self.read(), self.read()], 2)}
        before = deepcopy(sources)
        alias_snapshot_reads(sources)
        later = sources["run/snapshots/turn-02.json"]
        self.assertEqual(len(later["identical_read_aliases"]), 2)
        for name, source in sources.items():
            self.assertEqual(source["text"], before[name]["text"])
            self.assertEqual(source["included_lines"], before[name]["included_lines"])
        original_ranges = read_ranges(sources["run/snapshots/turn-01.json"]["text"])
        for alias in later["identical_read_aliases"]:
            self.assertEqual(alias["identical_to"], {"source": "run/snapshots/turn-01.json",
                                                    "line_start": original_ranges[0][0],
                                                    "line_end": original_ranges[0][1]})
        prompt = prompt_for({"run_id": "run", "scenario_id": "MCP-01", "sources": sources})
        self.assertIn("observation-1", prompt)
        self.assertIn("observation-2", prompt)
        self.assertEqual(prompt.count("EXACT_JSON_READ_ALIAS ="), 2)
        self.assertIn('"tool": "API-get-object"', prompt)

    def test_changed_metadata_arguments_and_order_remain_explicit(self):
        first = self.read()
        changed_etag = deepcopy(first)
        changed_etag["result"]["content"][1]["text"] = '{"request_metadata":{"etag":"second"}}'
        changed_args = deepcopy(first)
        changed_args["arguments"]["object_id"] = "two"
        changed_order = deepcopy(first)
        changed_order["result"]["content"].reverse()
        sources = {"run/final-state.json": self.source([first, changed_etag, changed_args, changed_order], 4)}
        alias_snapshot_reads(sources)
        self.assertNotIn("identical_read_aliases", sources["run/final-state.json"])

    def test_only_snapshot_sources_alias(self):
        text = self.source([self.read()], 1)
        sources = {"run/snapshots/turn-01.json": text,
                   "run/fixture-evidence.json": self.source([self.read()], 2)}
        alias_snapshot_reads(sources)
        self.assertNotIn("identical_read_aliases", sources["run/fixture-evidence.json"])

    def test_unrecognized_format_is_retained_without_aliasing(self):
        text = json.dumps({"reads": [self.read(), self.read()]})
        self.assertEqual(read_ranges(text), [])
        source = {"text": text, "included_lines": [1], "sha256": "test"}
        alias_snapshot_reads({"run/final-state.json": source})
        self.assertNotIn("identical_read_aliases", source)


if __name__ == "__main__":
    unittest.main()
