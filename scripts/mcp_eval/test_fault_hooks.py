from copy import deepcopy
import io
import json
from pathlib import Path
import stat
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch

from fault_hooks import FaultHooks, INVALID_BLOCK_ID, PROFILES, atomic_json, eligible
from space_guard import Guard, prepare_call, serve


WELCOME = "mcp14_welcome_timeout"
ATOMIC = "mcp27_atomic_patch_failure"
REACTION = "mcp29_reaction_timeout"


def call(name, arguments, serial=1):
    return {"jsonrpc": "2.0", "id": serial, "method": "tools/call",
            "params": {"name": name, "arguments": arguments}}


def result(*documents, serial=1, **fields):
    return {"jsonrpc": "2.0", "id": serial, "result": {
        "content": [{"type": "text", "text": json.dumps(doc)} for doc in documents], **fields}}


def welcome():
    return call("API-create-object", {"space_id": "abcdef", "body": {
        "properties": {"name": "Welcome"}, "type": "page", "blocks": []}})


def reaction():
    return call("API-toggle-chat-reaction", {"space_id": "abcdef", "chat_id": "chat",
                "message_id": "message", "emoji": "👍", "request_key": "stable-key"})


def atomic_patch():
    return call("API-patch-object", {"space_id": "abcdef", "object_id": "application",
        "create_missing_options": True, "body": {"ops": [
            {"op": "set_properties", "set": {"Stage": ["Ready"]}},
            {"op": "replace_text", "find": "Draft summary", "replace": "Final summary"}]}})


class FaultHookTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.config_path = self.root / "hook-config.json"
        self.state_path = self.root / "hook-state.json"
        self.guard = Guard(self.root / "scope.json", "eval-test")
        self.guard.register("bafytestabcdef.key")
        self.events = []

    def configure(self, profile, turn=None, **extra):
        scenario, scheduled = PROFILES[profile]
        config = {"scenario_id": scenario, "current_turn": scheduled if turn is None else turn,
                  "armed_profiles": [profile], **extra}
        atomic_json(self.config_path, config)
        return config

    def hooks(self, profile=None, **config):
        if profile:
            self.configure(profile, **config)
        return FaultHooks(self.config_path, log=lambda d, p: self.events.append((d, p)))

    def prepare(self, hooks, request):
        forwarded, refusal, ticket = prepare_call(self.guard, request, hooks)
        self.assertIsNone(refusal)
        return forwarded, ticket

    def test_no_config_is_unchanged_baseline(self):
        hooks = FaultHooks(None)
        request = welcome()
        before = deepcopy(request)
        forwarded, ticket = self.prepare(hooks, request)
        self.assertIsNone(ticket)
        self.assertEqual(request, before)
        self.assertEqual(forwarded["params"]["arguments"]["space_id"], "bafytestabcdef.key")
        response = result({"id": "welcome"})
        self.assertIs(hooks.complete(ticket, response), response)
        self.assertFalse(self.state_path.exists())

    def test_eligibility_is_exact_to_scenario_turn_and_intended_call(self):
        for profile, maker, name in [(WELCOME, welcome, "create_object"),
                                      (ATOMIC, atomic_patch, "patch_object"),
                                      (REACTION, reaction, "toggle_chat_reaction")]:
            config = self.configure(profile)
            args = maker()["params"]["arguments"]
            with self.subTest(profile=profile):
                self.assertTrue(eligible(profile, config, name, args))
                self.assertFalse(eligible(profile, {**config, "scenario_id": "MCP-01"}, name, args))
                self.assertFalse(eligible(profile, {**config, "current_turn": 1}, name, args))
                self.assertFalse(eligible(profile, config, "get_object", args))
                self.assertFalse(eligible(profile, config, name, {**args, "dry_run": True}))
        config = self.configure(WELCOME)
        self.assertFalse(eligible(WELCOME, config, "create_object", {
            "body": {"properties": {"name": "Other"}, "text": "Welcome"}}))
        config = self.configure(REACTION)
        self.assertFalse(eligible(REACTION, config, "toggle_chat_reaction", {"emoji": "❤️"}))

    def test_preview_turn_and_dry_run_are_never_modified(self):
        hooks = self.hooks(ATOMIC, turn=4)
        req = atomic_patch()
        forwarded, ticket = self.prepare(hooks, req)
        self.assertIsNone(ticket)
        self.assertEqual(forwarded["params"]["arguments"]["body"], req["params"]["arguments"]["body"])
        self.configure(ATOMIC)
        req["params"]["arguments"]["dry_run"] = True
        _, ticket = self.prepare(hooks, req)
        self.assertIsNone(ticket)
        self.assertFalse(hooks.state["hooks"][ATOMIC]["consumed"])

    def test_welcome_commit_timeout_saves_receipt_without_exposing_it(self):
        hooks = self.hooks(WELCOME)
        req = welcome()
        forwarded, ticket = self.prepare(hooks, req)
        response = result({"id": "actual-welcome"}, {"request_metadata": {"request_key": "server-key", "etag": "version"}})
        delivered = hooks.complete(ticket, response)
        self.assertNotIn("result", delivered)
        self.assertIn("transport timeout", delivered["error"]["message"])
        self.assertTrue(delivered["error"]["data"]["injected"])
        self.assertNotIn("actual-welcome", json.dumps(delivered))
        record = json.loads(self.state_path.read_text())["hooks"][WELCOME]
        self.assertEqual(record["status"], "triggered")
        self.assertEqual(record["attempts"][0]["actual_response"], response)
        self.assertEqual(record["attempts"][0]["original_request"], req)
        self.assertEqual(record["attempts"][0]["forwarded_request"], forwarded)
        self.assertTrue(self.guard.allowed_space("abcdef"))
        self.assertTrue(Guard(self.guard.path, self.guard.label).allowed_space("bafytestabcdef.key"))

    def test_reaction_replay_same_key_reaches_upstream_without_second_fault(self):
        hooks = self.hooks(REACTION)
        req = reaction()
        forwarded, ticket = self.prepare(hooks, req)
        self.assertEqual(forwarded["params"]["arguments"]["request_key"], "stable-key")
        response = result({"added": True}, {"request_metadata": {"request_key": "stable-key"}})
        self.assertIn("error", hooks.complete(ticket, response))
        restarted = self.hooks()
        replay, ticket = self.prepare(restarted, req)
        self.assertEqual(replay, forwarded)
        self.assertIsNone(ticket)
        self.assertEqual(restarted.complete(ticket, response), response)
        attempt = restarted.state["hooks"][REACTION]["attempts"][0]
        self.assertEqual(attempt["caller_request_key"], "stable-key")
        self.assertEqual(attempt["actual_response"], response)
        self.assertEqual(len(restarted.state["hooks"][REACTION]["attempts"]), 1)

    def test_actual_errors_and_missing_commit_receipts_are_not_timeouts(self):
        responses = [result({"status": 400, "code": "invalid_request"}, isError=True),
                     result({"status": 409, "code": "idempotency_conflict"}),
                     {"jsonrpc": "2.0", "id": 1, "error": {"code": -32000, "message": "Server failed"}},
                     result({"added": True, "dry_run": True}), result({"warnings": []})]
        hooks = self.hooks(REACTION)
        for response in responses:
            with self.subTest(response=response):
                _, ticket = self.prepare(hooks, reaction())
                self.assertEqual(ticket, REACTION)
                self.assertEqual(hooks.complete(ticket, response), response)
                self.assertEqual(hooks.state["hooks"][REACTION]["status"], "armed")
                self.assertFalse(hooks.state["hooks"][REACTION]["consumed"])
        _, ticket = self.prepare(hooks, reaction())
        self.assertIn("error", hooks.complete(ticket, result({"added": True})))

    def test_atomic_fault_changes_only_text_target_and_preserves_real_error(self):
        hooks = self.hooks(ATOMIC)
        request = atomic_patch()
        before = deepcopy(request)
        forwarded, ticket = self.prepare(hooks, request)
        expected = deepcopy(request)
        expected["params"]["arguments"]["space_id"] = "bafytestabcdef.key"
        expected["params"]["arguments"]["body"]["ops"][1]["id"] = INVALID_BLOCK_ID
        self.assertEqual(forwarded, expected)
        self.assertEqual(request, before)
        response = result({"status": 404, "code": "not_found", "issues": [{"path": "ops[1].id"}]},
                          {"request_metadata": {"request_key": "upstream-key"}}, isError=True)
        self.assertEqual(hooks.complete(ticket, response), response)
        self.assertEqual(hooks.state["hooks"][ATOMIC]["status"], "triggered")
        corrected, ticket = self.prepare(self.hooks(), request)
        self.assertIsNone(ticket)
        self.assertEqual(corrected["params"]["arguments"]["body"], before["params"]["arguments"]["body"])

    def test_update_block_locator_is_removed_and_actual_op_index_is_used(self):
        hooks = self.hooks(ATOMIC)
        req = atomic_patch()
        ops = req["params"]["arguments"]["body"]["ops"]
        ops[1] = {"op": "update_block", "match": "Draft summary", "set": {"text": "Final summary"}}
        ops.reverse()
        forwarded, ticket = self.prepare(hooks, req)
        self.assertEqual(ticket, ATOMIC)
        edited = forwarded["params"]["arguments"]["body"]["ops"]
        self.assertNotIn("match", edited[0])
        self.assertEqual(edited[0]["id"], INVALID_BLOCK_ID)
        self.assertEqual(edited[1], ops[1])

    def test_split_edits_and_unrelated_batches_are_not_eligible(self):
        config = self.configure(ATOMIC)
        args = atomic_patch()["params"]["arguments"]
        for op in args["body"]["ops"]:
            self.assertFalse(eligible(ATOMIC, config, "patch_object", {**args, "body": {"ops": [op]}}))
        args["body"]["ops"][0]["set"] = {"Stage": ["Review"]}
        self.assertFalse(eligible(ATOMIC, config, "patch_object", args))
        args["body"]["ops"][0]["set"] = {"unrelated": ["Ready"]}
        self.assertFalse(eligible(ATOMIC, config, "patch_object", args))
        args["body"]["ops"][0]["set"] = {"opaque-stage-id": ["Ready"]}
        self.assertTrue(eligible(ATOMIC, config, "patch_object", args))
        args["body"]["ops"][0]["set"] = {"bafyopaque": ["Ready"]}
        self.assertFalse(eligible(ATOMIC, config, "patch_object", args))
        self.assertTrue(eligible(ATOMIC, {**config, "stage_property_keys": ["bafyopaque"]}, "patch_object", args))

    def test_scope_refusal_precedes_hooks_and_never_mutates_request(self):
        hooks = self.hooks(ATOMIC)
        request = atomic_patch()
        request["params"]["arguments"]["space_id"] = "foreign"
        before = deepcopy(request)
        with patch.object(hooks, "prepare", wraps=hooks.prepare) as prepare:
            forwarded, refusal, ticket = prepare_call(self.guard, request, hooks)
            prepare.assert_not_called()
        self.assertIsNone(forwarded)
        self.assertIsNone(ticket)
        self.assertTrue(refusal["result"]["isError"])
        self.assertEqual(request, before)
        self.assertFalse(hooks.state["hooks"][ATOMIC]["consumed"])
        # FaultHooks also rejects an accidentally unguarded invocation.
        self.assertEqual(hooks.prepare(request, request, self.guard), (before, None))
        self.assertFalse(hooks.state["hooks"][ATOMIC]["consumed"])

    def test_durable_claim_on_restart_cannot_inject_twice(self):
        hooks = self.hooks(ATOMIC)
        _, ticket = self.prepare(hooks, atomic_patch())
        self.assertEqual(ticket, ATOMIC)
        resumed = self.hooks()
        record = resumed.state["hooks"][ATOMIC]
        self.assertEqual(record["status"], "fixture_unavailable")
        self.assertEqual(record["phase"], "interrupted")
        self.assertTrue(record["consumed"])
        _, ticket = self.prepare(resumed, atomic_patch())
        self.assertIsNone(ticket)
        self.assertEqual(len(record["attempts"]), 1)

    def test_atomic_state_is_private_and_leaves_no_temporary_files(self):
        hooks = self.hooks(WELCOME)
        self.prepare(hooks, welcome())
        self.assertEqual(stat.S_IMODE(self.state_path.stat().st_mode), 0o600)
        self.assertEqual(list(self.root.glob("hook-state.json.*")), [])
        self.assertTrue(json.loads(self.state_path.read_text())["hooks"][WELCOME]["consumed"])

    def test_proxy_forwards_identical_reaction_retries_and_logs_actual_results(self):
        self.configure(REACTION)
        first = reaction()
        replay = reaction()
        replay["id"] = 2
        first_response = result({"added": True}, {"request_metadata": {"request_key": "stable-key"}})
        second_response = result({"added": True}, serial=2)
        upstream = Mock()
        upstream.stdin = io.StringIO()
        upstream.stdout = io.StringIO(json.dumps(first_response) + "\n" + json.dumps(second_response) + "\n")
        capture = io.StringIO()
        args = SimpleNamespace(state=self.guard.path, run_label="eval-test", codex="fake-codex",
                               upstream="fake-upstream", trace=self.root / "wire.jsonl",
                               hook_config=self.config_path, hook_state=self.state_path)
        transport = SimpleNamespace(stdout=json.dumps({"transport": {"type": "stdio", "command": "fake"}}))
        with patch("space_guard.subprocess.run", return_value=transport), \
             patch("space_guard.subprocess.Popen", return_value=upstream), \
             patch("space_guard.sys.stdin", io.StringIO(json.dumps(first) + "\n" + json.dumps(replay) + "\n")), \
             patch("space_guard.sys.stdout", capture):
            serve(args)
        forwarded = [json.loads(line) for line in upstream.stdin.getvalue().splitlines()]
        self.assertEqual(len(forwarded), 2)
        self.assertEqual(forwarded[0]["params"], forwarded[1]["params"])
        self.assertEqual(forwarded[0]["params"]["arguments"]["request_key"], "stable-key")
        delivered = [json.loads(line) for line in capture.getvalue().splitlines()]
        self.assertIn("error", delivered[0])
        self.assertEqual(delivered[1], second_response)
        wire = [json.loads(line) for line in args.trace.read_text().splitlines()]
        self.assertEqual([e["payload"] for e in wire if e["direction"] == "from_upstream"],
                         [first_response, second_response])
        self.assertEqual(sum(e["direction"] == "hook" and e["payload"]["event"] == "triggered" for e in wire), 1)

    def test_proxy_captures_atomicity_before_model_can_retry(self):
        self.configure(ATOMIC)
        request = atomic_patch()
        replay = atomic_patch()
        replay["id"] = 2
        server_error = result({"status": 404, "code": "not_found", "issues": [{"path": "ops[1].id"}]}, isError=True)
        saved_object = {"properties": {"Stage": ["Draft"]}, "blocks": [{"id": "summary", "text": "Draft summary"}]}
        saved_options = {"data": [{"name": "Draft"}, {"name": "Review"}, {"name": "Submitted"}], "has_more": False}
        upstream = Mock()
        sent = []
        pending = []

        def respond(line):
            req = json.loads(line)
            sent.append(req)
            name = req["params"]["name"]
            if name == "API-get-object":
                response = result(saved_object, serial=req["id"])
            elif name == "API-list-property-options":
                response = result(saved_options, serial=req["id"])
            elif req["id"] == 1:
                response = server_error
            else:
                response = result({"applied": 2}, serial=req["id"])
            pending.append(json.dumps(response) + "\n")

        upstream.stdin.write.side_effect = respond
        upstream.stdout.readline.side_effect = lambda: pending.pop(0)
        capture = io.StringIO()
        args = SimpleNamespace(state=self.guard.path, run_label="eval-test", codex="fake-codex",
                               upstream="fake-upstream", trace=self.root / "wire.jsonl",
                               hook_config=self.config_path, hook_state=self.state_path)
        transport = SimpleNamespace(stdout=json.dumps({"transport": {"type": "stdio", "command": "fake"}}))
        with patch("space_guard.subprocess.run", return_value=transport), \
             patch("space_guard.subprocess.Popen", return_value=upstream), \
             patch("space_guard.sys.stdin", io.StringIO(json.dumps(request) + "\n" + json.dumps(replay) + "\n")), \
             patch("space_guard.sys.stdout", capture):
            serve(args)
        self.assertEqual([r["params"]["name"] for r in sent], ["API-get-object", "API-list-property-options",
            "API-patch-object", "API-get-object", "API-list-property-options", "API-patch-object"])
        self.assertEqual(len(set(r["id"] for r in sent)), 6)
        self.assertTrue(all(r["params"]["arguments"]["space_id"] == "bafytestabcdef.key" for r in sent))
        delivered = [json.loads(line) for line in capture.getvalue().splitlines()]
        self.assertEqual(len(delivered), 2)
        self.assertEqual(delivered[0], server_error)
        state = json.loads(self.state_path.read_text())
        attempt = state["hooks"][ATOMIC]["attempts"][0]
        self.assertEqual(attempt["actual_response"], server_error)
        for phase in ("before", "after"):
            evidence = attempt["atomicity_evidence"][phase]
            self.assertEqual(len(evidence), 2)
            self.assertEqual(json.loads(evidence[0]["response"]["result"]["content"][0]["text"]), saved_object)
            self.assertEqual(json.loads(evidence[1]["response"]["result"]["content"][0]["text"]), saved_options)
        wire = [json.loads(line) for line in args.trace.read_text().splitlines()]
        model_error_index = next(i for i, e in enumerate(wire) if e["direction"] == "to_model" and e["payload"]["id"] == 1)
        evidence_indices = [i for i, e in enumerate(wire) if e["direction"] == "evaluator_from_upstream"]
        self.assertEqual(len(evidence_indices), 4)
        self.assertTrue(all(i < model_error_index for i in evidence_indices))
        self.assertEqual(sum(e["direction"] == "to_upstream" for e in wire), 2)


if __name__ == "__main__":
    unittest.main()
