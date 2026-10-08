import json
from pathlib import Path
import tempfile
import unittest

from campaign_status import HOOK_R1_CONTROLS, build


class PrimaryRunDiscoveryTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        (self.base / "scenarios.json").write_text(json.dumps({"scenarios": [
            {"id": "MCP-30", "turns": [{"turn": 1}]}]}))
        self.run = self.base / "artifacts/luna-three-peer/round-2/runs/run-a"
        self.manifest = {"scenario_id": "MCP-30", "replicate": 2,
                         "label": "run-a", "turns": [{"turn": 1, "turn_completed": True}],
                         "status": "conversation_completed_pending_review"}
        self.write(self.run, self.manifest)

    def write(self, directory, manifest):
        directory.mkdir(parents=True)
        (directory / "run.json").write_text(json.dumps(manifest))
        (directory / "final-state.json").write_text("{}")

    def test_archived_checkpoint_is_not_a_second_conversation(self):
        archived = {**self.manifest, "status": "stopped_before_fixture_turn", "turns": []}
        self.write(self.run / "resumptions/attempt-01", archived)
        result = build(self.base)
        ready = [s for s in result["slots"] if s["status"] == "ready_for_astra_review"]
        self.assertEqual(len(ready), 1)
        self.assertEqual(ready[0]["run"]["run_dir"], str(self.run))
        self.assertEqual(len(result["excluded_runs"]), 1)

    def test_two_real_runs_for_one_slot_still_fail(self):
        self.write(self.run.with_name("run-b"), {**self.manifest, "label": "run-b"})
        with self.assertRaisesRegex(ValueError, "Two primary candidates"):
            build(self.base)

    def test_archive_name_alone_cannot_hide_a_different_run(self):
        self.write(self.run / "resumptions/attempt-01", {**self.manifest, "label": "different-run"})
        with self.assertRaisesRegex(ValueError, "Two primary candidates"):
            build(self.base)

    def test_explicit_scope_amendment_is_metadata_only(self):
        before = build(self.base)
        self.assertEqual(before["scope_amendments"], [])
        path = self.base / "artifacts/luna-three-runs/scope-amendments.json"
        path.parent.mkdir(parents=True)
        waiver = {"status": "skipped_by_user", "scenario_id": "MCP-30",
                  "verbatim": "second-account tests are not needed, we can skip them",
                  "effect": "Peer-history coverage is waived; no peer pass is claimed."}
        path.write_text(json.dumps({"source": "User message", "recorded_at": "2026-09-17",
                                    "second_account_tests": waiver}))
        after = build(self.base)
        self.assertEqual(after["slots"], before["slots"])
        self.assertEqual(after["counts"], before["counts"])
        self.assertEqual(after["expected_primary_runs"], before["expected_primary_runs"])
        self.assertEqual(len(after["scope_amendments"]), 1)
        amendment = after["scope_amendments"][0]
        self.assertEqual({key: amendment[key] for key in waiver}, waiver)
        self.assertEqual(amendment["amendment_key"], "second_account_tests")
        self.assertEqual(amendment["source_file"], str(path))
        self.assertEqual(len(amendment["source_sha256"]), 64)

    def hooked_replacement(self, hook_status="triggered"):
        document = json.loads((self.base / "scenarios.json").read_text())
        document["scenarios"].append({"id": "MCP-14", "turns": [{"turn": 1}]})
        (self.base / "scenarios.json").write_text(json.dumps(document))
        old_label = HOOK_R1_CONTROLS["MCP-14"]
        old = self.base / "artifacts/luna-20260916/runs" / old_label
        self.write(old, {**self.manifest, "label": old_label, "scenario_id": "MCP-14", "replicate": 1,
                         "hook_status": "not_exercised"})
        new = self.base / "artifacts/luna-three-hooks-r1-complete/round-1/runs/new-hooked-r1"
        self.write(new, {**self.manifest, "label": new.name, "scenario_id": "MCP-14", "replicate": 1,
                         "hook_status": hook_status})
        return old, new

    def test_new_hooked_r1_is_primary_and_exact_old_baseline_is_control(self):
        before = build(self.base)
        old, new = self.hooked_replacement()
        result = build(self.base)
        slot = next(s for s in result["slots"] if s["scenario_id"] == "MCP-14" and s["replicate"] == 1)
        self.assertEqual(slot["run"]["run_dir"], str(new))
        self.assertEqual(slot["status"], "ready_for_astra_review")
        self.assertEqual([s for s in result["slots"] if s["scenario_id"] == "MCP-30"], before["slots"])
        self.assertEqual(len(result["controls"]), 1)
        control = result["controls"][0]
        self.assertEqual(control["run"]["run_dir"], str(old))
        self.assertEqual(control["run"]["run_id"], old.name)
        self.assertEqual(control["classification"], "control")
        self.assertEqual(control["replaced_by"]["run_dir"], str(new))

    def test_untriggered_new_r1_does_not_claim_complete_hook_coverage(self):
        self.hooked_replacement(hook_status="not_triggered")
        result = build(self.base)
        slot = next(s for s in result["slots"] if s["scenario_id"] == "MCP-14" and s["replicate"] == 1)
        self.assertEqual(slot["status"], "incomplete")

    def test_unknown_old_baseline_cannot_be_silently_classified_as_control(self):
        old, _ = self.hooked_replacement()
        manifest = json.loads((old / "run.json").read_text())
        self.write(old.with_name("unapproved-baseline"), {**manifest, "label": "unapproved-baseline"})
        with self.assertRaisesRegex(ValueError, "Unapproved hooked R1 candidate"):
            build(self.base)


if __name__ == "__main__":
    unittest.main()
