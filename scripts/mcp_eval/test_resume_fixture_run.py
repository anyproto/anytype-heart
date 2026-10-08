from copy import deepcopy
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import resume_fixture_run as resume


def write(path, data):
    path.write_text(json.dumps(data) + "\n")


class ResumeFixtureTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.suite = self.root / "scenarios.json"
        self.suite.write_bytes(resume.runner.SUITE.read_bytes())
        self.session_id = "01a0ac7e-627e-7ce2-be08-584440b42338"
        self.space = {"full_id": "bafyownedabcdef.key", "compact_id": "abcdef"}
        self.run_dir = self.root / "eval-preserved-run"
        self.run_dir.mkdir()
        self.workspace = self.root / "original-workspace"
        self.workspace.mkdir()
        self.pid_check = patch.object(resume, "assert_absent")
        self.pid_check.start()
        self.addCleanup(self.pid_check.stop)
        self.processes = patch.object(resume, "process_table", return_value=[])
        self.processes.start()
        self.addCleanup(self.processes.stop)
        self.make_run()

    def make_run(self, scenario_id="MCP-28", boundary=7):
        scenario = next(s for s in json.loads(self.suite.read_text())["scenarios"] if s["id"] == scenario_id)
        self.manifest = {"scenario_id": scenario_id, "model": "gpt-5.6-luna", "reasoning": "medium",
                         "label": self.run_dir.name, "workspace": str(self.workspace), "runner_pid": 987654,
                         "status": "running", "session_id": self.session_id, "owned_spaces": [self.space], "turns": []}
        create_args = {"name": self.run_dir.name + " — Test space"}
        for turn in scenario["turns"][:boundary-1]:
            n = turn["turn"]
            prompt = resume.rendered(turn, self.run_dir.name)
            self.manifest["turns"].append({"turn": n, "user": prompt, "exit_code": 0,
                                           "timeout": False, "turn_completed": True, "usage": {"input_tokens": n}})
            es = [{"type": "thread.started", "thread_id": self.session_id}, {"type": "turn.started"}]
            if n == 1:
                es.append({"type": "item.completed", "item": {"id": "item_1", "type": "mcp_tool_call",
                           "tool": "API-create-space", "arguments": create_args, "status": "completed",
                           "error": None, "result": {"content": [{"type": "text", "text": json.dumps({"id": self.space["full_id"]})}]}}})
            es.append({"type": "turn.completed", "usage": {"input_tokens": n}})
            (self.run_dir / f"turn-{n:02d}.events.jsonl").write_text("".join(json.dumps(e)+"\n" for e in es))
            (self.run_dir / f"turn-{n:02d}.prompt.txt").write_text(prompt)
            (self.run_dir / f"turn-{n:02d}.final.txt").write_text(f"Existing final {n}\n")
            (self.run_dir / f"turn-{n:02d}.stderr.log").write_text(f"Existing stderr {n}\n")
        request = {"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "API-create-space", "arguments": create_args}}
        response = {"jsonrpc": "2.0", "id": 1, "result": {"content": [{"type": "text", "text": json.dumps({"id": self.space["full_id"]})}]}}
        wire = [{"direction": d, "payload": p} for d,p in [("from_model", request), ("to_upstream", request),
                                                                       ("from_upstream", response), ("to_model", response)]]
        (self.run_dir / "mcp-wire.jsonl").write_text("".join(json.dumps(e)+"\n" for e in wire))
        write(self.run_dir / "run.json", self.manifest)
        write(self.run_dir / "scope.json", {"spaces": [self.space]})
        write(self.run_dir / "active-process.json", {"pid": 987655, "state": "exited", "exit_code": 0})
        kind = resume.BOUNDARIES[(scenario_id, boundary)]
        write(self.run_dir / "fixture-request.json", {"kind": kind, "scenario_id": scenario_id,
              "before_turn": boundary, "space_id": self.space["full_id"], "run_label": self.run_dir.name,
              "status": "waiting_for_real_fixture"})
        write(self.run_dir / "fixture-evidence.json", {"fixtures": [{"kind": kind, "before_turn": boundary,
              "status": "fixture_failed", "error": "Real fixture was not supplied; retained run may be resumed after inspection", "calls": []}]})
        write(self.run_dir / f"fixture-{kind}-receipt.json", {"space_id": self.space["full_id"],
              "object_id": "real-desktop-object", "chat_id": "real-chat", "peer_id": "real-peer", "status": "ready"})

    def validate(self):
        return resume.validate(self.run_dir, self.suite)

    def test_validation_preserves_exact_session_and_completed_prefix_without_writes(self):
        before = {p: p.read_bytes() for p in self.run_dir.iterdir()}
        plan = self.validate()
        self.assertEqual(plan["manifest"]["session_id"], self.session_id)
        self.assertEqual(plan["manifest"]["turns"], self.manifest["turns"])
        self.assertEqual(plan["next_turn"], 7)
        self.assertEqual(plan["run_dir"], self.run_dir)
        self.assertEqual(before, {p: p.read_bytes() for p in self.run_dir.iterdir()})

    def test_live_recorded_process_is_refused(self):
        with patch.object(resume, "assert_absent", side_effect=ValueError("Prior runner PID still exists")):
            with self.assertRaisesRegex(ValueError, "still exists"):
                self.validate()

    def test_live_related_codex_even_with_dead_recorded_pids_is_refused(self):
        with patch.object(resume, "process_table", return_value=[{"pid": 989999, "ppid": 1, "state": "S",
                          "command": "codex exec resume " + self.session_id}]):
            with self.assertRaisesRegex(ValueError, "still live"):
                self.validate()

    def test_nonterminal_codex_record_is_refused(self):
        write(self.run_dir / "active-process.json", {"pid": 987655, "state": "running"})
        with self.assertRaisesRegex(ValueError, "successfully exited"):
            self.validate()

    def test_campaign_or_run_lock_is_never_removed(self):
        for path in (self.root / "campaign.lock", self.run_dir / "resume.lock"):
            with self.subTest(path=path):
                path.write_text("stale or live; inspect separately")
                with self.assertRaisesRegex(ValueError, "Existing lock"):
                    self.validate()
                self.assertTrue(path.exists())
                path.unlink()

    def test_failed_turn_or_any_pending_turn_file_is_refused(self):
        self.manifest["turns"][-1]["exit_code"] = 1
        write(self.run_dir / "run.json", self.manifest)
        with self.assertRaisesRegex(ValueError, "partial, failed"):
            self.validate()
        self.manifest["turns"][-1]["exit_code"] = 0
        write(self.run_dir / "run.json", self.manifest)
        (self.run_dir / "turn-07.events.jsonl").write_text("")
        with self.assertRaisesRegex(ValueError, "may have committed"):
            self.validate()

    def test_unfinished_tool_despite_turn_completed_is_refused(self):
        path = self.run_dir / "turn-06.events.jsonl"
        es = resume.events(path)
        es.insert(-1, {"type": "item.started", "item": {"id": "pending", "type": "mcp_tool_call"}})
        path.write_text("".join(json.dumps(e)+"\n" for e in es))
        with self.assertRaisesRegex(ValueError, "Unfinished tool"):
            self.validate()

    def test_tool_transport_failure_is_refused(self):
        path = self.run_dir / "turn-06.events.jsonl"
        es = resume.events(path)
        es.insert(-1, {"type": "item.completed", "item": {"id": "lost", "type": "mcp_tool_call",
                      "tool": "API-delete-object", "status": "failed", "error": "transport timeout"}})
        path.write_text("".join(json.dumps(e)+"\n" for e in es))
        with self.assertRaisesRegex(ValueError, "transport outcome"):
            self.validate()

    def test_explicit_read_only_schema_501_is_safe_but_other_5xx_are_not(self):
        path = self.run_dir / "turn-06.events.jsonl"
        original_events = resume.events(path)
        wire_path = self.run_dir / "mcp-wire.jsonl"
        original_wire = wire_path.read_text()
        for tool, status, code, allowed in [
                ("API-get-type-schema", 501, "not_implemented", True),
                ("API-get-type-schema", 500, "internal_error", False),
                ("API-patch-object", 501, "not_implemented", False),
                ("API-unknown-tool", 501, "not_implemented", False)]:
            with self.subTest(tool=tool, status=status):
                arguments = {"space_id": self.space["full_id"], "type": "room"}
                result = {"content": [{"type": "text", "text": json.dumps({"status": status, "code": code})}]}
                es = deepcopy(original_events)
                es.insert(-1, {"type": "item.completed", "item": {"id": "capability", "type": "mcp_tool_call",
                              "tool": tool, "arguments": arguments, "status": "failed", "error": None, "result": result}})
                path.write_text("".join(json.dumps(e)+"\n" for e in es))
                request = {"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": tool, "arguments": arguments}}
                response = {"jsonrpc": "2.0", "id": 2, "result": result}
                extra_wire = [{"direction": d, "payload": p} for d,p in [("from_model", request),
                              ("to_upstream", request), ("from_upstream", response), ("to_model", response)]]
                wire_path.write_text(original_wire+"".join(json.dumps(e)+"\n" for e in extra_wire))
                if allowed:
                    self.assertEqual(self.validate()["next_turn"], 7)
                else:
                    with self.assertRaisesRegex(ValueError, "Failed tool outcome"):
                        self.validate()

    def test_wrong_scenario_or_thread_is_refused(self):
        original = deepcopy(self.manifest)
        self.manifest["scenario_id"] = "MCP-14"
        write(self.run_dir / "run.json", self.manifest)
        with self.assertRaisesRegex(ValueError, "Only MCP-28"):
            self.validate()
        write(self.run_dir / "run.json", original)
        path = self.run_dir / "turn-06.events.jsonl"
        path.write_text(path.read_text().replace(self.session_id, "replacement-thread"))
        with self.assertRaisesRegex(ValueError, "Thread identity"):
            self.validate()

    def test_changed_frozen_scenario_is_refused(self):
        suite = json.loads(self.suite.read_text())
        next(s for s in suite["scenarios"] if s["id"] == "MCP-28")["turns"][-1]["user"] += " changed"
        write(self.suite, suite)
        with self.assertRaisesRegex(ValueError, "Frozen scenario differs"):
            self.validate()

    def test_missing_receipt_or_space_mismatch_is_refused(self):
        path = self.run_dir / "fixture-desktop_provenance-receipt.json"
        path.unlink()
        with self.assertRaisesRegex(ValueError, "Supply the real fixture"):
            self.validate()
        write(path, {"space_id": "another-space", "object_id": "foreign"})
        with self.assertRaisesRegex(ValueError, "another space"):
            self.validate()

    def test_both_peer_boundaries_are_eligible(self):
        # Use fresh directories so no completed later-turn evidence is removed.
        for boundary in (5, 6):
            self.run_dir = self.root / f"peer-boundary-{boundary}"
            self.run_dir.mkdir()
            self.make_run("MCP-30", boundary)
            self.assertEqual(self.validate()["next_turn"], boundary)

    def test_unavailable_peer_requires_truthful_matching_user_authorization(self):
        for boundary in (5, 6):
            self.run_dir = self.root / f"unavailable-peer-{boundary}"
            self.run_dir.mkdir()
            self.make_run("MCP-30", boundary)
            kind = resume.BOUNDARIES[("MCP-30", boundary)]
            path = self.run_dir / f"fixture-{kind}-receipt.json"
            receipt = {"space_id": self.space["full_id"], "status": "fixture_unavailable",
                       "kind": kind, "before_turn": boundary, "run_label": self.run_dir.name,
                       "scenario_id": "MCP-30", "reason": "User excluded second-account tests.",
                       "user_authorized_skip": True}
            write(path, receipt)
            self.assertEqual(self.validate()["next_turn"], boundary)
            for extra in ({"user_authorized_skip": False}, {"peer_id": "invented-peer"},
                          {"before_turn": 99}, {"reason": ""}):
                with self.subTest(boundary=boundary, invalid=extra):
                    write(path, {**receipt, **extra})
                    with self.assertRaisesRegex(ValueError, "user-authorized MCP-30 peer skip"):
                        self.validate()

    def test_unavailable_desktop_fixture_still_cannot_resume_imported_note_turn(self):
        write(self.run_dir / "fixture-desktop_provenance-receipt.json", {
            "space_id": self.space["full_id"], "status": "fixture_unavailable",
            "user_authorized_skip": True, "reason": "No desktop fixture"})
        with self.assertRaisesRegex(ValueError, "user-authorized MCP-30 peer skip"):
            self.validate()

    def test_execute_continues_exact_thread_and_preserves_completed_artifacts(self):
        plan = self.validate()
        original = {p: p.read_bytes() for p in plan["protected"]}
        prefix = deepcopy(self.manifest["turns"])
        wire = (self.run_dir / "mcp-wire.jsonl").read_bytes()
        commands = []

        def fake_execute(cmd, prompt, event_path, err_path, timeout):
            commands.append(cmd)
            self.assertEqual(cmd[cmd.index("resume")+1], self.session_id)
            self.assertEqual(cmd[cmd.index("-C")+1], str(self.workspace))
            event_path.write_text(json.dumps({"type": "thread.started", "thread_id": self.session_id})+"\n"+
                                  json.dumps({"type": "turn.completed", "usage": {}})+"\n")
            err_path.write_text("")
            Path(cmd[cmd.index("-o")+1]).write_text("resumed final")
            return {"exit_code": 0, "timeout": False, "duration_seconds": 1}

        with patch.object(resume, "before_turn"), patch.object(resume, "snapshot", return_value=2), \
             patch.object(resume.runner, "execute", side_effect=fake_execute):
            self.assertEqual(resume.execute_resume(plan, codex="fake-codex"), 0)
        manifest = resume.load(self.run_dir / "run.json")
        self.assertEqual(len(commands), 2)
        self.assertEqual(manifest["turns"][:6], prefix)
        self.assertEqual(manifest["session_id"], self.session_id)
        self.assertEqual(manifest["label"], self.run_dir.name)
        self.assertEqual(manifest["owned_spaces"], [self.space])
        self.assertEqual(manifest["status"], "conversation_completed_pending_review")
        self.assertEqual(original, {p:p.read_bytes() for p in original})
        self.assertEqual((self.run_dir / "mcp-wire.jsonl").read_bytes(), wire)
        self.assertFalse((self.run_dir / "resume.lock").exists())
        self.assertEqual(resume.load(self.run_dir / "resumptions/attempt-01/run.json")["session_id"], self.session_id)

    def test_unexpected_new_thread_stops_without_adopting_it(self):
        plan = self.validate()
        def fake_execute(cmd, prompt, event_path, err_path, timeout):
            event_path.write_text(json.dumps({"type":"thread.started","thread_id":"replacement"})+"\n"+
                                  json.dumps({"type":"turn.completed"})+"\n")
            return {"exit_code":0,"timeout":False,"duration_seconds":1}
        with patch.object(resume, "before_turn"), patch.object(resume, "snapshot") as snapshot, \
             patch.object(resume.runner, "execute", side_effect=fake_execute) as execute:
            self.assertEqual(resume.execute_resume(plan, codex="fake-codex"), 1)
            self.assertEqual(execute.call_count, 1)
            snapshot.assert_not_called()
        manifest = resume.load(self.run_dir / "run.json")
        self.assertEqual(manifest["session_id"], self.session_id)
        self.assertEqual(manifest["status"], "runtime_failed")


class PIDTests(unittest.TestCase):
    def test_own_live_pid_is_refused(self):
        with self.assertRaisesRegex(ValueError, "still exists"):
            resume.assert_absent(os.getpid(), "Test")

    def test_permission_error_is_not_treated_as_absence(self):
        with patch.object(resume.os, "kill", side_effect=PermissionError):
            with self.assertRaisesRegex(ValueError, "not observable"):
                resume.assert_absent(999999, "Test")


if __name__ == "__main__":
    unittest.main()
