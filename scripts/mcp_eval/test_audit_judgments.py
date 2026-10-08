from copy import deepcopy
import fcntl
import json
from pathlib import Path
import tempfile
import unittest

import audit_judgments as audit
import review_codex as review


class JudgmentAuditTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.reviews = Path(self.temp.name) / "reviews"
        self.schema = json.loads(review.SCHEMA.read_text())

    def write_runtime(self, directory, event_name, metadata_name, thread):
        events = [{"type": "thread.started", "thread_id": thread},
                  {"type": "item.completed", "item": {"type": "agent_message", "text": "captured JSON"}},
                  {"type": "turn.completed"}]
        (directory / event_name).write_text("".join(json.dumps(event) + "\n" for event in events))
        review.save(directory / metadata_name, {"observed_model": review.MODEL,
            "observed_reasoning_effort": review.EFFORT, "thread_id": thread,
            "rollout_metadata": [{"type": "session_meta", "id": thread},
                                 {"type": "turn_context", "model": review.MODEL, "effort": review.EFFORT}]})

    def completed(self, label="run-one", thread="thread-one", repaired=True):
        directory = self.reviews / "runs" / label
        directory.mkdir(parents=True)
        text = '{"visible":1}\n{"duplicate":true}\n{"visible":3}\n'
        original = self.reviews.parent / (label + "-original.jsonl")
        original.write_text(text)
        bundle = {"run_id": label, "scenario_id": "MCP-01", "scenario": {"checks": ["One object"]},
                  "original_sha256": {str(original): review.digest(original.read_bytes())},
                  "sources": {"run/events.jsonl": {"text": text, "sha256": review.digest(text.encode()),
                              "line_count": 3, "included_lines": [1, 3]}}}
        ref = {"source": "run/events.jsonl", "line_start": 1, "line_end": 1}
        score = {"score": 2, "reason": "Partial", "evidence": [ref]}
        result = {"schema_version": 1, "run_id": label, "scenario_id": "MCP-01", "outcome": "partial",
                  "summary": "Partly complete", "full_pass": False, "integrity_failure": False,
                  "hook_status": "not_applicable", "scores": {k: deepcopy(score) for k in self.schema["properties"]["scores"]["properties"]},
                  "checks": [{"check_index": 1, "requirement": "One object", "status": "unverified",
                              "reason": "Missing final evidence", "evidence": [ref]}],
                  "findings": [], "evidence_gaps": [], "additional_requirements": []}
        initial = deepcopy(result)
        if repaired:
            initial["checks"][0]["evidence"] = [{**ref, "line_end": 3}]
        review.save(directory / "judge-output.json", initial)
        review.save(directory / "reviewer-result.json", result)
        review.save(directory / "evidence.json", bundle)
        review.save(directory / "result.schema.json", self.schema)
        prompt = "Frozen evaluation input\n"
        (directory / "judge-prompt.txt").write_text(prompt)
        frozen_hash = review.digest((review.dump(bundle) + prompt + review.dump(self.schema) + review.MODEL + review.EFFORT).encode())
        state = {"status": "complete", "run_id": label, "scenario_id": "MCP-01", "input_sha256": frozen_hash,
                 "execution": {"exit_code": 0, "timeout": False}}
        self.write_runtime(directory, "judge.events.jsonl", "model-metadata.json", thread)
        if repaired:
            repair = directory / "repairs/01"
            repair.mkdir(parents=True)
            (repair / "prompt.txt").write_text("Repair invalid citation only\n")
            review.save(repair / "output.json", result)
            self.write_runtime(repair, "events.jsonl", "model-metadata.json", thread)
            review.save(repair / "repair.json", {"attempt": 1, "status": "complete", "thread_id": thread,
                "input_sha256": frozen_hash, "prompt_sha256": review.digest((repair / "prompt.txt").read_bytes()),
                "source_output_sha256": review.digest((directory / "judge-output.json").read_bytes()),
                "execution": {"exit_code": 0, "timeout": False}, "command": ["codex", "exec", "resume", thread, "-"]})
            state.update(repairs=[{"attempt": 1}], accepted_output=str(repair / "output.json"))
        review.save(directory / "job.json", state)
        return directory

    def assert_error(self, phrase):
        report = audit.audit(self.reviews)
        self.assertGreater(report["summary"]["errors"], 0)
        self.assertTrue(any(phrase in error["error"] for error in report["errors"]), report["errors"])

    def test_valid_original_and_repaired_judgments_are_read_only(self):
        self.completed()
        self.completed("run-two", "thread-two", repaired=False)
        before = {str(path): path.read_bytes() for path in self.reviews.rglob("*") if path.is_file()}
        report = audit.audit(self.reviews)
        self.assertEqual(report["errors"], [])
        self.assertEqual(report["summary"]["audited_completed_reviews"], 2)
        self.assertEqual(report["summary"]["distinct_initial_threads"], 2)
        self.assertEqual(report["summary"]["repairs"], 1)
        self.assertTrue(report["runs"][0]["substantive_fields_unchanged"])
        self.assertEqual(before, {str(path): path.read_bytes() for path in self.reviews.rglob("*") if path.is_file()})

    def test_changed_frozen_prompt_or_source_hash_is_rejected(self):
        directory = self.completed()
        prompt_path = directory / "judge-prompt.txt"
        original_prompt = prompt_path.read_text()
        prompt_path.write_text("Tampered prompt")
        self.assert_error("frozen evidence/prompt/schema hash mismatch")
        prompt_path.write_text(original_prompt)
        bundle = audit.load(directory / "evidence.json")
        bundle["sources"]["run/events.jsonl"]["sha256"] = "0" * 64
        review.save(directory / "evidence.json", bundle)
        state = audit.load(directory / "job.json")
        state["input_sha256"] = review.digest((review.dump(bundle) + original_prompt + review.dump(self.schema) + review.MODEL + review.EFFORT).encode())
        review.save(directory / "job.json", state)
        self.assert_error("frozen source hash mismatch")

    def test_accepted_output_must_equal_validated_result(self):
        directory = self.completed(repaired=False)
        initial = audit.load(directory / "judge-output.json")
        initial["summary"] = "Different accepted result"
        review.save(directory / "judge-output.json", initial)
        self.assert_error("accepted output differs")

    def test_actual_rollout_model_and_no_tool_boundary_are_verified(self):
        directory = self.completed()
        metadata_path = directory / "model-metadata.json"
        original = audit.load(metadata_path)
        wrong = deepcopy(original)
        wrong["rollout_metadata"][1]["model"] = "wrong-model"
        review.save(metadata_path, wrong)
        self.assert_error("captured rollout model/reasoning differs")
        review.save(metadata_path, original)
        events = directory / "judge.events.jsonl"
        events.write_text(events.read_text() + '{"type":"item.completed","item":{"type":"unknown_tool_call"}}\n')
        self.assert_error("tool or unknown item kind")

    def test_repair_requires_same_thread_and_substantive_fields(self):
        directory = self.completed()
        repair = directory / "repairs/01"
        self.write_runtime(repair, "events.jsonl", "model-metadata.json", "different-thread")
        self.assert_error("mismatched judge thread")
        self.write_runtime(repair, "events.jsonl", "model-metadata.json", "thread-one")
        changed = audit.load(repair / "output.json")
        changed["summary"] = "Changed grading conclusion"
        review.save(repair / "output.json", changed)
        review.save(directory / "reviewer-result.json", changed)
        self.assert_error("citation-only repair changed substantive fields")

    def test_duplicate_initial_threads_are_reported(self):
        self.completed("run-one", "shared-thread")
        self.completed("run-two", "shared-thread")
        self.assert_error("initial thread reused")

    def test_pending_and_locked_active_jobs_are_skipped(self):
        directory = self.completed()
        pending = self.reviews / "runs/pending"
        pending.mkdir()
        review.save(pending / "job.json", {"status": "pending"})
        with (directory / ".lock").open("w") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            report = audit.audit(self.reviews)
        self.assertEqual(report["errors"], [])
        self.assertEqual(report["summary"]["audited_completed_reviews"], 0)
        self.assertEqual({entry["status"] for entry in report["skipped"]}, {"active", "pending"})

    def test_malformed_runtime_evidence_is_not_silently_skipped(self):
        directory = self.completed()
        events = directory / "judge.events.jsonl"
        events.write_text(events.read_text() + "NOT JSON\n")
        self.assert_error("Expecting value")

    def test_original_source_tampering_and_missing_file_report_exact_paths(self):
        directory = self.completed()
        source = Path(next(iter(audit.load(directory / "evidence.json")["original_sha256"])))
        source.write_text("Changed original evidence")
        report = audit.audit(self.reviews)
        error = report["errors"][0]["source_errors"][0]
        self.assertEqual(error["source"], str(source))
        self.assertEqual(error["status"], "hash_mismatch")
        source.unlink()
        report = audit.audit(self.reviews)
        error = report["errors"][0]["source_errors"][0]
        self.assertEqual(error["source"], str(source))
        self.assertEqual(error["status"], "missing_or_unreadable")

    def test_audit_output_cannot_overwrite_originals_or_review_artifacts(self):
        directory = self.completed()
        source = Path(next(iter(audit.load(directory / "evidence.json")["original_sha256"])))
        for target in (source, self.reviews / "summary.json", directory / "job.json"):
            with self.subTest(target=target), self.assertRaisesRegex(ValueError, "must not overwrite"):
                audit.validate_output_path(self.reviews, target)
        audit.validate_output_path(self.reviews, self.reviews.parent / "judgment-audit.json")


if __name__ == "__main__":
    unittest.main()
