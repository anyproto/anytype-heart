from copy import deepcopy
import fcntl
import json
from pathlib import Path
import unittest
from unittest.mock import patch

import retry_capacity_judgment as retry
import review_codex as review
import test_audit_judgments as fixtures


class CapacityRetryTest(unittest.TestCase):
    def setUp(self):
        self.fixture = fixtures.JudgmentAuditTest()
        self.fixture.setUp()
        self.addCleanup(self.fixture.doCleanups)
        self.directory = self.fixture.completed(repaired=False).resolve()
        self.valid_result = retry.load(self.directory / "reviewer-result.json")
        (self.directory / "reviewer-result.json").unlink()
        (self.directory / "judge-output.json").write_text("")
        (self.directory / ".lock").touch()
        state = retry.load(self.directory / "job.json")
        workspace = self.directory.parent.parent / "empty-workspace"
        workspace.mkdir()
        state.update(status="failed", execution={"exit_code": 1, "timeout": False},
                     workspace=str(workspace), model=review.MODEL, reasoning=review.EFFORT,
                     run=str(self.directory.parent.parent / "actor-run"), prompt_bytes=24,
                     command=review.command("codex", workspace, self.directory))
        review.save(self.directory / "job.json", state)
        review.save(self.directory / "active-process.json", {"state": "exited", "pid": 999999999, "exit_code": 1})
        self.failed_events = [{"type": "thread.started", "thread_id": "thread-one"},
                              {"type": "turn.started"}, {"type": "error", "message": retry.CAPACITY},
                              {"type": "turn.failed", "error": {"message": retry.CAPACITY}}]
        self.write_events(self.failed_events)

    def write_events(self, values):
        (self.directory / "judge.events.jsonl").write_text("".join(json.dumps(value) + "\n" for value in values))

    def test_check_only_is_read_only_and_qualifies_exact_capacity_failure(self):
        before = retry.file_hashes(self.directory)
        with patch.object(review, "execute", side_effect=AssertionError("must not launch")):
            result = retry.run(self.directory)
        self.assertTrue(result["eligible"])
        self.assertEqual(result["mode"], "check_only")
        self.assertEqual(retry.file_hashes(self.directory), before)
        self.assertFalse((self.directory.parent.parent / "infrastructure-failures").exists())

    def test_rejects_other_errors_success_usage_tools_and_live_pid(self):
        variants = [self.failed_events + [{"type": "turn.completed"}],
                    self.failed_events + [{"type": "item.completed", "item": {"type": "agent_message", "text": "Review"}}],
                    self.failed_events + [{"type": "item.completed", "item": {"type": "mcp_tool_call"}}],
                    self.failed_events + [{"type": "turn.failed", "usage": {"input_tokens": 1}, "error": {"message": retry.CAPACITY}}],
                    [{**event, "message": "Authentication failed"} if event["type"] == "error" else event for event in self.failed_events]]
        with patch.object(review, "execute", side_effect=AssertionError("must refuse")):
            for values in variants:
                self.write_events(values)
                with self.assertRaises(ValueError):
                    retry.run(self.directory)
            self.write_events(self.failed_events)
            with patch.object(retry, "pid_alive", return_value=True), self.assertRaisesRegex(ValueError, "PID is still alive"):
                retry.run(self.directory)

    def test_rejects_changed_frozen_inputs_or_original_sources(self):
        prompt = self.directory / "judge-prompt.txt"
        old = prompt.read_text()
        prompt.write_text("Changed")
        with self.assertRaisesRegex(ValueError, "frozen input hash"):
            retry.run(self.directory)
        prompt.write_text(old)
        source = Path(next(iter(retry.load(self.directory / "evidence.json")["original_sha256"])))
        source.write_text("Changed original")
        with self.assertRaisesRegex(ValueError, "original source hash"):
            retry.run(self.directory)

    def test_locked_target_or_other_active_job_cannot_execute(self):
        with (self.directory / ".lock").open("r") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            with self.assertRaisesRegex(ValueError, "lock is held"):
                retry.run(self.directory)
        other = self.directory.parent / "another-review"
        other.mkdir()
        review.save(other / "job.json", {"status": "running"})
        self.assertEqual(retry.run(self.directory)["execution_blockers"], [str(other)])
        with patch.object(review, "execute", side_effect=AssertionError("must refuse")), \
             self.assertRaisesRegex(ValueError, "other review jobs are active"):
            retry.run(self.directory, execute=True)
        self.assertFalse((self.directory.parent.parent / "infrastructure-failures").exists())

    def fake_execute(self, result):
        def execute(cmd, prompt, event_path, stderr_path, timeout):
            review.save(Path(cmd[cmd.index("-o") + 1]), result)
            self.fixture.write_runtime(self.directory, "judge.events.jsonl", "model-metadata.json", "fresh-thread")
            stderr_path.write_text("")
            review.save(self.directory / "active-process.json", {"state": "exited", "pid": 999999998, "exit_code": 0})
            return {"exit_code": 0, "timeout": False}
        return execute

    def test_execute_archives_entire_failure_and_makes_exactly_one_fresh_attempt(self):
        before = retry.file_hashes(self.directory)
        prompt = (self.directory / "judge-prompt.txt").read_text()
        with patch.object(retry.shutil, "which", return_value="/bin/codex"), \
             patch.object(retry.subprocess, "check_output", return_value="codex test"), \
             patch.object(review, "model_metadata", side_effect=lambda _: retry.load(self.directory / "model-metadata.json")), \
             patch.object(review, "execute", side_effect=self.fake_execute(self.valid_result)) as execute:
            result = retry.run(self.directory, execute=True)
        self.assertEqual(execute.call_count, 1)
        cmd, sent_prompt = execute.call_args.args[:2]
        self.assertEqual(sent_prompt, prompt)
        self.assertNotIn("resume", cmd)
        self.assertEqual(cmd[cmd.index("-m") + 1], review.MODEL)
        self.assertIn('model_reasoning_effort="high"', cmd)
        archive = Path(result["archive"])
        archived_hashes = retry.file_hashes(archive)
        archived_hashes.pop("failure-receipt.json")
        self.assertEqual(archived_hashes, before)
        self.assertFalse(archive.is_relative_to(self.directory.parent))
        for name in ("evidence.json", "judge-prompt.txt", "result.schema.json"):
            self.assertEqual(review.digest((self.directory / name).read_bytes()), before[name])
        self.assertEqual(retry.load(self.directory / "job.json")["status"], "complete")

    def test_invalid_new_json_requires_separate_structural_repair_not_another_fresh_attempt(self):
        invalid = deepcopy(self.valid_result)
        invalid["checks"][0]["evidence"] = [{"source": "run/events.jsonl", "line_start": 1, "line_end": 3}]
        with patch.object(retry.shutil, "which", return_value="/bin/codex"), \
             patch.object(retry.subprocess, "check_output", return_value="codex test"), \
             patch.object(review, "model_metadata", side_effect=lambda _: retry.load(self.directory / "model-metadata.json")), \
             patch.object(review, "execute", side_effect=self.fake_execute(invalid)) as execute:
            with self.assertRaisesRegex(ValueError, "same-thread structural repair"):
                retry.run(self.directory, execute=True)
        self.assertEqual(execute.call_count, 1)
        state = retry.load(self.directory / "job.json")
        self.assertEqual(state["status"], "failed")
        self.assertEqual(state["stage"], "validation")
        with self.assertRaisesRegex(ValueError, "exit code 1"):
            retry.run(self.directory)


if __name__ == "__main__":
    unittest.main()
