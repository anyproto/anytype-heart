import argparse
from copy import deepcopy
import fcntl
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import review_codex as review


class ReviewTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.run = self.root / "run-1"
        self.run.mkdir()
        self.suite = self.root / "scenarios.json"
        self.scenario = {"id": "MCP-01", "title": "Test", "checks": ["One record"],
                         "turns": [{"turn": 1, "user": "Create one record"}]}
        self.suite.write_text(json.dumps({"scenarios": [self.scenario]}))
        self.suite.with_name("HARNESS.md").write_text("Score independently\n")
        self.suite.with_name("surface-snapshot.json").write_text('{"tools": []}\n')
        (self.run / "run.json").write_text(json.dumps({"scenario_id": "MCP-01", "label": "run-1",
            "turns": [{"turn": 1, "user": "Create one record"}], "independent_verdict": "SECRET PRIOR VERDICT"}))
        (self.run / "turn-01.prompt.txt").write_text("Create one record\n")
        (self.run / "turn-01.events.jsonl").write_text('{"type":"item.completed","item":{"type":"mcp_tool_call","error":"real failure"}}\n')
        self.bundle = review.evidence_bundle(self.run, self.suite)
        self.schema = json.loads(review.SCHEMA.read_text())
        self.ref = {"source": "run/turn-01.events.jsonl", "line_start": 1, "line_end": 1}
        score = {"score": 2, "reason": "Partial", "evidence": [self.ref]}
        self.result = {"schema_version": 1, "run_id": "run-1", "scenario_id": "MCP-01",
            "outcome": "partial", "summary": "Incomplete", "full_pass": False,
            "integrity_failure": False, "hook_status": "not_applicable",
            "scores": {k: deepcopy(score) for k in self.schema["properties"]["scores"]["properties"]},
            "checks": [{"check_index": 1, "requirement": "One record", "status": "unverified",
                        "reason": "No final read", "evidence": [self.ref]}],
            "findings": [], "evidence_gaps": [], "additional_requirements": []}

    def test_evidence_excludes_prior_judgment_preserves_raw_failures_and_extra_reads(self):
        (self.run / "reviewer-result.json").write_text('{"summary": "SECRET PRIOR VERDICT"}')
        (self.run / "hook-state.json").write_text('{"original": "valid", "forwarded": "invalid", "triggered": true}')
        (self.run / "snapshots").mkdir()
        (self.run / "snapshots/turn-01.json").write_text('{"objects": []}')
        bundle = review.evidence_bundle(self.run, self.suite)
        prompt = review.prompt_for(bundle)
        self.assertNotIn("SECRET PRIOR VERDICT", prompt)
        self.assertIn('"error":"real failure"', prompt)
        self.assertIn('"forwarded": "invalid"', prompt)
        self.assertIn("run/snapshots/turn-01.json", bundle["sources"])
        self.assertEqual(bundle["sources"]["run/turn-01.events.jsonl"]["text"], (self.run / "turn-01.events.jsonl").read_text())

    def test_strict_results_and_references(self):
        review.validate_result(self.result, self.bundle, self.schema)
        mutations = [lambda r: r.update(extra="invalid"),
                     lambda r: r.update(scenario_id="MCP-02"),
                     lambda r: r["scores"]["data_integrity"].update(score=True),
                     lambda r: r["scores"]["data_integrity"].update(score=5),
                     lambda r: r["checks"].append(deepcopy(r["checks"][0])),
                     lambda r: r["checks"][0]["evidence"][0].update(source="reviewer-result.json"),
                     lambda r: r["checks"][0]["evidence"][0].update(line_end=100),
                     lambda r: r.update(full_pass=True)]
        for mutation in mutations:
            with self.subTest(mutation=mutation):
                value = deepcopy(self.result)
                mutation(value)
                with self.assertRaises(ValueError):
                    review.validate_result(value, self.bundle, self.schema)

    def test_prepare_never_launches_and_changed_inputs_require_new_output(self):
        args = argparse.Namespace(suite=self.suite, prepare_only=True, retry_failed=False,
                                  max_input_bytes=1_000_000, codex="codex", timeout=10)
        output = self.root / "judgment"
        with patch.object(review, "execute", side_effect=AssertionError("must not launch")):
            self.assertEqual(review.judge(self.run, output, args), "pending")
            state = json.loads((output / "job.json").read_text())
            self.assertEqual(state["status"], "pending")
            (self.run / "turn-01.prompt.txt").write_text("Changed input\n")
            with self.assertRaisesRegex(ValueError, "inputs changed"):
                review.judge(self.run, output, args)

    def test_oversize_input_fails_without_truncation(self):
        args = argparse.Namespace(suite=self.suite, prepare_only=True, retry_failed=False,
                                  max_input_bytes=1, codex="codex", timeout=10)
        output = self.root / "judgment"
        with self.assertRaisesRegex(ValueError, "nothing truncated"):
            review.judge(self.run, output, args)
        self.assertIn("real failure", (output / "judge-prompt.txt").read_text())

    def test_command_has_offline_boundary_and_fixed_model(self):
        cmd = review.command("codex", self.root, self.root)
        self.assertIn("--ignore-user-config", cmd)
        self.assertEqual(cmd[cmd.index("-s") + 1], "read-only")
        self.assertEqual(cmd[cmd.index("-m") + 1], "gpt-6-astra")
        self.assertIn('model_reasoning_effort="high"', cmd)
        self.assertIn("mcp_servers={}", cmd)
        self.assertIn("project_doc_max_bytes=0", cmd)
        self.assertNotIn("resume", cmd)

    def test_wire_selection_keeps_injected_difference_and_all_result_blocks(self):
        def wire(direction, payload):
            return json.dumps({"direction": direction, "payload": payload}) + "\n"
        original = {"id": 2, "method": "tools/call", "params": {"name": "API-patch-object", "arguments": {"block_id": "valid"}}}
        injected = deepcopy(original)
        injected["params"]["arguments"]["block_id"] = "invalid"
        reply = {"id": 2, "result": {"isError": True, "content": [
            {"type": "text", "text": '{"code":"invalid_block","issues":["bad block"]}'},
            {"type": "text", "text": '{"request_metadata":{"etag":"abc"}}'}]}}
        text = wire("from_model", original) + wire("to_upstream", injected) + wire("from_upstream", reply) + wire("to_model", reply)
        (self.run / "mcp-wire.jsonl").write_text(text)
        bundle = review.evidence_bundle(self.run, self.suite)
        source = bundle["sources"]["run/mcp-wire.jsonl"]
        self.assertEqual(source["included_lines"], [1, 2, 3])
        self.assertEqual(source["text"], text)
        self.assertIn("request_metadata", review.prompt_for(bundle))
        self.assertIn("invalid_block", review.prompt_for(bundle))

    def test_active_lock_prevents_duplicate_launch(self):
        args = argparse.Namespace(suite=self.suite, prepare_only=True, retry_failed=False,
                                  max_input_bytes=1_000_000, codex="codex", timeout=10)
        output = self.root / "judgment"
        output.mkdir()
        with (output / ".lock").open("w") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            with self.assertRaisesRegex(ValueError, "already running"):
                review.judge(self.run, output, args)

    def test_null_transport_slot_deduplicates_wire_without_losing_distinct_content(self):
        reply = {"content": [{"type": "text", "text": '{"schema":{"type":"object"}}'},
                             {"type": "text", "text": '{"request_metadata":{"etag":"one"}}'}]}
        item = {"type": "item.completed", "item": {"type": "mcp_tool_call",
                "tool": "API-get-schema", "result": {**reply, "structured_content": None}}}
        (self.run / "turn-01.events.jsonl").write_text(json.dumps(item) + "\n")
        changed = deepcopy(reply)
        changed["content"][1]["text"] = '{"request_metadata":{"etag":"two"}}'
        structured = {**reply, "structuredContent": {"extra": "must retain"}}
        failed = {**reply, "isError": True}
        wire = "".join(json.dumps({"direction": "from_upstream", "payload": {"id": i, "result": r}}) + "\n"
                       for i, r in enumerate([reply, changed, structured, failed], 1))
        (self.run / "mcp-wire.jsonl").write_text(wire)
        bundle = review.evidence_bundle(self.run, self.suite)
        source = bundle["sources"]["run/mcp-wire.jsonl"]
        self.assertEqual(source["included_lines"], [2, 3, 4])
        self.assertEqual(source["text"], wire)
        prompt = review.prompt_for(bundle)
        self.assertIn("must retain", prompt)
        self.assertIn("isError", prompt)
        self.assertIn("schema", prompt)

    def test_private_discovery_redacted_in_replies_wire_and_assistant_echoes(self):
        (self.run / "scope.json").write_text('{"spaces":[{"full_id":"owned-space","compact_id":"own"}]}')
        records = [{"id": "private-space-id", "name": "Garden Delta", "description": "Private project"},
                   {"id": "owned-space", "name": "My evaluation", "description": "Keep this"}]
        def event(tool, document):
            return json.dumps({"type": "item.completed", "item": {"type": "mcp_tool_call", "tool": tool,
                "result": {"content": [{"type": "text", "text": json.dumps(document)}]}}}) + "\n"
        original = event("API-list-spaces", {"data": records, "total": 2})
        original += event("API-auth-whoami", {"key": {"id": "private-key-id", "name": "Private key"},
                                             "scope": "all_spaces", "grant": {"can_create_spaces": True}})
        original += json.dumps({"type": "item.completed", "item": {"type": "agent_message", "text": "Garden Delta has id private-space-id. Owned email user@example.test."}}) + "\n"
        (self.run / "turn-01.events.jsonl").write_text(original)
        (self.run / "turn-01.final.txt").write_text("Garden Delta has id private-space-id.")
        bundle = review.evidence_bundle(self.run, self.suite)
        prompt = review.prompt_for(bundle)
        for secret in ("Garden Delta", "Private project", "private-space-id", "private-key-id", "Private key"):
            self.assertNotIn(secret, prompt)
        for retained in ("owned-space", "My evaluation", "Keep this", "all_spaces", "can_create_spaces", "user@example.test"):
            self.assertIn(retained, prompt)
        source = bundle["sources"]["run/turn-01.events.jsonl"]
        self.assertEqual(source["original_text_sha256"], review.digest(original.encode()))
        self.assertNotEqual(source["sha256"], source["original_text_sha256"])
        self.assertTrue(bundle["redactions"]["values"])

    def test_aggregate_requires_three_distinct_validated_runs(self):
        output = self.root / "reviews"
        directory = output / "runs/run-1"
        directory.mkdir(parents=True)
        review.save(directory / "job.json", {"status": "complete"})
        review.save(directory / "evidence.json", self.bundle)
        review.save(directory / "result.schema.json", self.schema)
        review.save(directory / "reviewer-result.json", self.result)
        review.aggregate(output, self.suite, 3)
        summary = json.loads((output / "summary.json").read_text())
        self.assertEqual(summary, {"reviewed": 1, "expected": 3, "complete": False})
        data = json.loads((output / "scenarios/MCP-01.json").read_text())
        self.assertEqual(data["outcomes"], {"partial": 1})
        self.assertFalse(data["coverage_complete"])

    def matrix_reviews(self, replicates, scenario_id="MCP-01"):
        output = self.root / "matrix-reviews"
        slots = [{"scenario_id": scenario_id, "replicate": number, "status": "not_started", "run": None}
                 for number in (1, 2, 3)]
        for number in replicates:
            # Deliberately avoid repetition-looking labels; mapping must come from the matrix.
            run_id = "baseline" if number == 1 else f"captured-{chr(64 + number)}"
            run_dir = self.root / run_id
            directory = output / "runs" / run_id
            directory.mkdir(parents=True)
            bundle, result = deepcopy(self.bundle), deepcopy(self.result)
            bundle["run_id"] = result["run_id"] = run_id
            bundle["scenario_id"] = result["scenario_id"] = scenario_id
            bundle["scenario"]["id"] = scenario_id
            bundle["sources"]["run/run.json"]["original"] = str(run_dir / "run.json")
            review.save(directory / "job.json", {"status": "complete", "run_id": run_id,
                "scenario_id": scenario_id, "run": str(run_dir)})
            review.save(directory / "evidence.json", bundle)
            review.save(directory / "result.schema.json", self.schema)
            review.save(directory / "reviewer-result.json", result)
            slots[number - 1].update(status="ready_for_astra_review", run={"run_id": run_id, "run_dir": str(run_dir)})
        matrix = self.root / "matrix.json"
        review.save(matrix, {"slots": slots})
        return output, matrix

    def test_scope_waiver_is_reported_only_for_matching_scenario_without_regrading(self):
        peer = {**self.scenario, "id": "MCP-30", "title": "Peer chat"}
        self.suite.write_text(json.dumps({"scenarios": [self.scenario, peer]}))
        output, matrix = self.matrix_reviews([1], scenario_id="MCP-30")
        result_path = output / "runs/baseline/reviewer-result.json"
        result = json.loads(result_path.read_text())
        result["evidence_gaps"] = [{"category": "fixture_missing", "description": "Peer history unavailable",
                                    "affected_checks": [1], "evidence": [self.ref]}]
        review.save(result_path, result)
        raw_before = result_path.read_bytes()
        review.aggregate(output, self.suite, 3, matrix)
        before = json.loads((output / "scenarios/MCP-30.json").read_text())
        waiver = {"amendment_key": "second_account_tests", "scenario_id": "MCP-30", "status": "skipped_by_user",
                  "verbatim": "second-account tests are not needed, we can skip them",
                  "effect": "Peer-authored pagination and unread-arrival race coverage is waived.",
                  "source": "User message", "source_file": "scope-amendments.json"}
        document = json.loads(matrix.read_text())
        document["scope_amendments"] = [waiver]
        review.save(matrix, document)
        review.aggregate(output, self.suite, 3, matrix)
        after = json.loads((output / "scenarios/MCP-30.json").read_text())
        self.assertEqual(after["scope_amendments"], [waiver])
        self.assertIn("Reporting only", after["scope_amendment_policy"])
        self.assertEqual({k: v for k, v in after.items() if not k.startswith("scope_amendment")}, before)
        self.assertEqual(after["expected_repetitions"], 3)
        self.assertFalse(after["coverage_complete"])
        self.assertEqual(after["runs"][0]["result"]["evidence_gaps"], result["evidence_gaps"])
        self.assertEqual(result_path.read_bytes(), raw_before)
        other = json.loads((output / "scenarios/MCP-01.json").read_text())
        self.assertNotIn("scope_amendments", other)
        markdown = (output / "FINDINGS.md").read_text()
        self.assertIn(waiver["verbatim"], markdown)
        self.assertIn("Unavailable peer evidence does not become a pass", markdown)

    def test_generic_aggregate_reads_explicit_scope_document_without_matrix(self):
        output, _ = self.matrix_reviews([1])
        path = self.root / "artifacts/luna-three-runs/scope-amendments.json"
        path.parent.mkdir(parents=True)
        path.write_text(json.dumps({"source": "User message", "specific_waiver": {
            "scenario_id": "MCP-01", "status": "skipped_by_user", "verbatim": "Skip this fixture",
            "effect": "Fixture omitted from requested reporting scope"}}))
        review.aggregate(output, self.suite, 3)
        data = json.loads((output / "scenarios/MCP-01.json").read_text())
        self.assertEqual(data["scope_amendments"][0]["amendment_key"], "specific_waiver")
        self.assertFalse(data["coverage_complete"])
        self.assertEqual(data["expected_repetitions"], 3)

    def test_evaluator_corrections_match_run_and_scenario_without_grade_overrides(self):
        other = {**self.scenario, "id": "MCP-02", "title": "Other scenario"}
        self.suite.write_text(json.dumps({"scenarios": [self.scenario, other]}))
        output, matrix = self.matrix_reviews([1])
        raw_path = output / "runs/baseline/reviewer-result.json"
        raw_before = raw_path.read_bytes()
        review.aggregate(output, self.suite, 3, matrix)
        before = json.loads((output / "scenarios/MCP-01.json").read_text())
        matching = {"id": "type-etag-baseline", "scenario_id": "MCP-01", "run_id": "baseline",
                    "kind": "runtime_argument_declaration", "status": "verified_evaluator_correction",
                    "correction": {"contract": "The runtime manifest declared expected_etag."}, "evidence": []}
        document = {"schema_version": 1, "authority": "Root evaluator", "policy": "Raw scores remain unchanged",
                    "source": "surface-contract-audit.json", "source_sha256": "a" * 64,
                    "entries": [matching, {**matching, "id": "wrong-run", "run_id": "not-reviewed"},
                                {**matching, "id": "wrong-scenario", "scenario_id": "MCP-02"}]}
        path = matrix.parent / "evaluator-corrections.json"
        review.save(path, document)
        review.aggregate(output, self.suite, 3, matrix)
        after = json.loads((output / "scenarios/MCP-01.json").read_text())
        self.assertEqual(after["evaluator_corrections"], [matching])
        provenance = after["evaluator_corrections_provenance"]
        self.assertEqual(provenance["authority"], document["authority"])
        self.assertEqual(provenance["policy"], document["policy"])
        self.assertEqual(provenance["source_sha256"], document["source_sha256"])
        self.assertEqual(provenance["corrections_file_sha256"], review.digest(path.read_bytes()))
        self.assertEqual({k: v for k, v in after.items() if not k.startswith("evaluator_corrections")}, before)
        self.assertEqual(raw_path.read_bytes(), raw_before)
        unrelated = json.loads((output / "scenarios/MCP-02.json").read_text())
        self.assertNotIn("evaluator_corrections", unrelated)
        self.assertIn("Raw judgments and grades remain unchanged", (output / "FINDINGS.md").read_text())

    def add_control_review(self, output, matrix):
        original = output / "runs/baseline"
        control_dir = output / "runs/unhooked-control"
        control_dir.mkdir()
        for path in original.iterdir():
            (control_dir / path.name).write_bytes(path.read_bytes())
        run_dir = self.root / "unhooked-control-evidence"
        job = json.loads((control_dir / "job.json").read_text())
        job.update(run_id="unhooked-control", run=str(run_dir))
        review.save(control_dir / "job.json", job)
        bundle = json.loads((control_dir / "evidence.json").read_text())
        bundle["run_id"] = "unhooked-control"
        bundle["sources"]["run/run.json"]["original"] = str(run_dir / "run.json")
        review.save(control_dir / "evidence.json", bundle)
        result = json.loads((control_dir / "reviewer-result.json").read_text())
        result.update(run_id="unhooked-control", outcome="model_failed")
        for score in result["scores"].values():
            score["score"] = 0
        review.save(control_dir / "reviewer-result.json", result)
        document = json.loads(matrix.read_text())
        declaration = {"scenario_id": "MCP-01", "classification": "control", "former_replicate": 1,
                       "reason": "Original baseline did not exercise the hook", "run": {
                           "run_id": "unhooked-control", "run_dir": str(run_dir)}}
        document["controls"] = [declaration]
        review.save(matrix, document)
        return control_dir

    def test_explicit_controls_preserved_but_excluded_from_primary_coverage_and_scores(self):
        output, matrix = self.matrix_reviews([1, 2, 3])
        review.aggregate(output, self.suite, 3, matrix)
        before = json.loads((output / "scenarios/MCP-01.json").read_text())
        control_dir = self.add_control_review(output, matrix)
        control_bytes = {path.name: path.read_bytes() for path in control_dir.iterdir()}
        review.aggregate(output, self.suite, 3, matrix)
        after = json.loads((output / "scenarios/MCP-01.json").read_text())
        self.assertEqual({k: v for k, v in after.items() if k != "control_runs"}, before)
        self.assertEqual(after["control_runs"][0]["result"]["outcome"], "model_failed")
        self.assertEqual(after["reviewed_replicates"], [1, 2, 3])
        summary = json.loads((output / "summary.json").read_text())
        self.assertEqual(summary["reviewed"], 3)
        self.assertEqual(summary["reviewed_controls"], 1)
        self.assertTrue(summary["complete"])
        self.assertEqual(json.loads((output / "controls.json").read_text())["reviewed_controls"], 1)
        self.assertEqual(control_bytes, {path.name: path.read_bytes() for path in control_dir.iterdir()})
        self.assertIn("Control run — excluded from primary scores and coverage", (output / "FINDINGS.md").read_text())

    def test_control_cannot_fill_missing_primary_slot_and_nonmatching_control_is_rejected(self):
        output, matrix = self.matrix_reviews([1, 2])
        control_dir = self.add_control_review(output, matrix)
        review.aggregate(output, self.suite, 3, matrix)
        summary = json.loads((output / "summary.json").read_text())
        self.assertEqual(summary["reviewed"], 2)
        self.assertFalse(summary["complete"])
        document = json.loads(matrix.read_text())
        for field, value in (("run_id", "different-control"), ("run_dir", str(self.root / "different-control-path"))):
            changed = deepcopy(document)
            changed["controls"][0]["run"][field] = value
            review.save(matrix, changed)
            with self.assertRaisesRegex(ValueError, "not an exact primary run or explicit control"):
                review.aggregate(output, self.suite, 3, matrix)
        review.save(matrix, {**document, "controls": []})
        with self.assertRaisesRegex(ValueError, "not an exact primary run or explicit control"):
            review.aggregate(output, self.suite, 3, matrix)

    def test_duplicate_control_or_primary_control_overlap_is_rejected(self):
        output, matrix = self.matrix_reviews([1])
        self.add_control_review(output, matrix)
        document = json.loads(matrix.read_text())
        document["controls"].append(deepcopy(document["controls"][0]))
        review.save(matrix, document)
        with self.assertRaisesRegex(ValueError, "control overlaps"):
            review.aggregate(output, self.suite, 3, matrix)
        document["controls"] = [{**document["controls"][0], "run": document["slots"][0]["run"]}]
        review.save(matrix, document)
        with self.assertRaisesRegex(ValueError, "control overlaps"):
            review.aggregate(output, self.suite, 3, matrix)

    def test_matrix_retains_explicit_baseline_slot_and_requires_all_three(self):
        output, matrix = self.matrix_reviews([1, 3])
        review.aggregate(output, self.suite, 3, matrix)
        data = json.loads((output / "scenarios/MCP-01.json").read_text())
        self.assertEqual(data["reviewed_replicates"], [1, 3])
        self.assertEqual([(e["replicate"], e["result"]["run_id"]) for e in data["runs"]], [(1, "baseline"), (3, "captured-C")])
        self.assertFalse(data["coverage_complete"])
        self.assertFalse(json.loads((output / "summary.json").read_text())["complete"])

    def test_matrix_all_three_valid_judgments_complete(self):
        output, matrix = self.matrix_reviews([3, 1, 2])
        review.aggregate(output, self.suite, 3, matrix)
        self.assertTrue(json.loads((output / "summary.json").read_text())["complete"])
        data = json.loads((output / "scenarios/MCP-01.json").read_text())
        self.assertEqual(data["reviewed_replicates"], [1, 2, 3])
        self.assertTrue(data["coverage_complete"])

    def test_matrix_rejects_unlisted_or_mismatched_primary_identity(self):
        output, matrix = self.matrix_reviews([1])
        path = output / "runs/baseline/job.json"
        original = json.loads(path.read_text())
        for field, value in (("run", str(self.root / "diagnostic")), ("run_id", "diagnostic"), ("scenario_id", "MCP-02")):
            with self.subTest(field=field):
                review.save(path, {**original, field: value})
                with self.assertRaisesRegex(ValueError, "identity mismatch|not an exact primary"):
                    review.aggregate(output, self.suite, 3, matrix)
        review.save(path, original)
        evidence_path = output / "runs/baseline/evidence.json"
        evidence = json.loads(evidence_path.read_text())
        evidence["sources"]["run/run.json"]["original"] = str(self.root / "diagnostic/run.json")
        review.save(evidence_path, evidence)
        with self.assertRaisesRegex(ValueError, "evidence directory differs"):
            review.aggregate(output, self.suite, 3, matrix)

    def test_matrix_rejects_duplicate_slots_and_duplicate_judgments(self):
        output, matrix = self.matrix_reviews([1])
        original = json.loads(matrix.read_text())
        review.save(matrix, {"slots": original["slots"] + [original["slots"][0]]})
        with self.assertRaisesRegex(ValueError, "duplicate replicate slot in matrix"):
            review.aggregate(output, self.suite, 3, matrix)
        review.save(matrix, original)
        duplicate = output / "runs/second-review-of-baseline"
        duplicate.mkdir()
        for path in (output / "runs/baseline").iterdir():
            (duplicate / path.name).write_bytes(path.read_bytes())
        with self.assertRaisesRegex(ValueError, "duplicate replicate slot in judgments"):
            review.aggregate(output, self.suite, 3, matrix)

    def test_matrix_rejects_invalid_judgment_and_non_three_configuration(self):
        output, matrix = self.matrix_reviews([1, 2, 3])
        with self.assertRaisesRegex(ValueError, "exactly three"):
            review.aggregate(output, self.suite, 2, matrix)
        result_path = output / "runs/baseline/reviewer-result.json"
        result = json.loads(result_path.read_text())
        result["checks"] = []
        review.save(result_path, result)
        with self.assertRaises(ValueError):
            review.aggregate(output, self.suite, 3, matrix)

    def failed_structural_judge(self):
        output = self.root / "repair-review"
        output.mkdir()
        bundle = deepcopy(self.bundle)
        source = bundle["sources"][self.ref["source"]]
        source.update(text='{"visible":1}\n{"omitted":2}\n{"visible":3}\n', line_count=3, included_lines=[1, 3])
        prompt = review.prompt_for(bundle)
        invalid = deepcopy(self.result)
        invalid["checks"][0]["evidence"] = [{**self.ref, "line_end": 3}]
        review.save(output / "evidence.json", bundle)
        review.save(output / "result.schema.json", self.schema)
        (output / "judge-prompt.txt").write_text(prompt)
        review.save(output / "judge-output.json", invalid)
        events = [{"type": "thread.started", "thread_id": "original-thread"}, {"type": "turn.completed"}]
        (output / "judge.events.jsonl").write_text("".join(json.dumps(e) + "\n" for e in events))
        metadata = {"observed_model": review.MODEL, "observed_reasoning_effort": review.EFFORT, "thread_id": "original-thread"}
        review.save(output / "model-metadata.json", metadata)
        workspace = self.root / "empty-judge-workspace"
        workspace.mkdir()
        state = {"status": "failed", "stage": "validation", "workspace": str(workspace),
                 "execution": {"exit_code": 0, "timeout": False},
                 "input_sha256": review.digest((review.dump(bundle) + prompt + review.dump(self.schema) + review.MODEL + review.EFFORT).encode())}
        review.save(output / "job.json", state)
        args = argparse.Namespace(retry_failed=True, prepare_only=False, suite=self.suite,
                                  max_repairs=2, codex="codex", timeout=5)
        return output, args, metadata, invalid

    def fake_repair_execute(self, result):
        def execute(cmd, prompt, events, stderr, timeout):
            Path(cmd[cmd.index("-o") + 1]).write_text(review.dump(result))
            events.write_text(json.dumps({"type": "thread.started", "thread_id": "original-thread"}) + "\n" +
                              json.dumps({"type": "turn.completed"}) + "\n")
            stderr.write_text("")
            return {"exit_code": 0, "timeout": False}
        return execute

    def test_structural_retry_resumes_exact_thread_and_keeps_frozen_originals(self):
        output, args, metadata, invalid = self.failed_structural_judge()
        immutable = {name: (output / name).read_bytes() for name in
                     ("evidence.json", "result.schema.json", "judge-prompt.txt", "judge-output.json", "judge.events.jsonl", "model-metadata.json")}
        # Repair must not rebuild evidence even if live source files changed.
        (self.run / "turn-01.prompt.txt").write_text("Unrelated subsequent change")
        with patch.object(review, "evidence_bundle", side_effect=AssertionError("must use frozen evidence")), \
             patch.object(review.shutil, "which", return_value="/bin/codex"), \
             patch.object(review, "model_metadata", return_value=metadata), \
             patch.object(review, "execute", side_effect=self.fake_repair_execute(self.result)) as execute:
            self.assertEqual(review.judge(self.run, output, args), "complete")
        cmd, prompt = execute.call_args.args[:2]
        self.assertEqual(cmd[cmd.index("resume") + 1], "original-thread")
        self.assertIn("--ignore-user-config", cmd)
        self.assertEqual(cmd[cmd.index("-s") + 1], "read-only")
        self.assertIn("allowed_contiguous_ranges", prompt)
        self.assertNotIn("=== SOURCE", prompt)
        self.assertEqual(json.loads((output / "reviewer-result.json").read_text()), self.result)
        for name, data in immutable.items():
            self.assertEqual((output / name).read_bytes(), data, name)
        self.assertTrue((output / "repairs/01/prompt.txt").exists())
        self.assertTrue((output / "repairs/01/model-metadata.json").exists())

    def test_feedback_collects_all_invalid_references_and_keeps_strict_validation(self):
        output, _, _, invalid = self.failed_structural_judge()
        invalid["scores"]["data_integrity"]["evidence"] = [{**self.ref, "line_start": 2, "line_end": 2}]
        bundle = json.loads((output / "evidence.json").read_text())
        issues = review.validation_feedback(review.dump(invalid), bundle, self.schema)
        citations = [issue for issue in issues if issue["kind"] == "citation"]
        self.assertEqual(len(citations), 2)
        self.assertEqual(citations[0]["allowed_contiguous_ranges"], [[1, 1], [3, 3]])
        with self.assertRaises(ValueError):
            review.validate_result(invalid, bundle, self.schema)

    def test_structural_repair_limit_survives_explicit_retries(self):
        output, args, metadata, invalid = self.failed_structural_judge()
        with patch.object(review.shutil, "which", return_value="/bin/codex"), \
             patch.object(review, "model_metadata", return_value=metadata), \
             patch.object(review, "execute", side_effect=self.fake_repair_execute(invalid)) as execute:
            with self.assertRaisesRegex(ValueError, "limit exhausted"):
                review.judge(self.run, output, args)
            self.assertEqual(execute.call_count, 2)
            with self.assertRaisesRegex(ValueError, "limit exhausted"):
                review.judge(self.run, output, args)
            self.assertEqual(execute.call_count, 2)
        self.assertTrue((output / "repairs/02/output.json").exists())
        self.assertFalse((output / "reviewer-result.json").exists())

    def test_repair_refuses_runtime_boundary_model_and_frozen_input_failures(self):
        output, args, metadata, _ = self.failed_structural_judge()
        state = json.loads((output / "job.json").read_text())
        original_events = (output / "judge.events.jsonl").read_text()
        cases = [lambda: review.save(output / "job.json", {**state, "execution": {"exit_code": 1}}),
                 lambda: review.save(output / "model-metadata.json", {**metadata, "observed_model": "wrong-model"}),
                 lambda: review.save(output / "model-metadata.json", {**metadata, "thread_id": "different-thread"}),
                 lambda: (output / "judge.events.jsonl").write_text(original_events + '{"type":"item.completed","item":{"type":"mcp_tool_call"}}\n')]
        with patch.object(review, "execute", side_effect=AssertionError("must refuse launch")):
            for corrupt in cases:
                review.save(output / "job.json", state)
                review.save(output / "model-metadata.json", metadata)
                (output / "judge.events.jsonl").write_text(original_events)
                corrupt()
                with self.assertRaisesRegex(ValueError, "repair refused"):
                    review.judge(self.run, output, args)
            review.save(output / "job.json", state)
            (output / "judge-prompt.txt").write_text("Changed frozen prompt")
            with self.assertRaisesRegex(ValueError, "hash mismatch"):
                review.judge(self.run, output, args)

    def test_citation_only_repair_cannot_change_substantive_verdict(self):
        output, args, metadata, _ = self.failed_structural_judge()
        args.max_repairs = 1
        changed = deepcopy(self.result)
        changed["summary"] = "A different substantive conclusion"
        with patch.object(review.shutil, "which", return_value="/bin/codex"), \
             patch.object(review, "model_metadata", return_value=metadata), \
             patch.object(review, "execute", side_effect=self.fake_repair_execute(changed)):
            with self.assertRaisesRegex(ValueError, "limit exhausted"):
                review.judge(self.run, output, args)
        issues = json.loads((output / "repairs/01/validation-after.json").read_text())
        self.assertIn("substantive", issues[0]["message"])
        self.assertFalse((output / "reviewer-result.json").exists())


if __name__ == "__main__":
    unittest.main()


class FullTierReviewInputsTest(unittest.TestCase):
    """A run against /mcp/full is reviewed against the surface its arm was
    served — the campaign's snapshot — not the bridge's."""

    def test_review_inputs_carry_full_tier_names_and_no_bridge_names(self):
        golden = Path(__file__).resolve().parents[2] / "core/api/wrapper/full/testdata/tools_list.golden.json"
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            suite = root / "scenarios.json"
            suite.write_text(json.dumps({"scenarios": [{"id": "MCP-01", "title": "T", "checks": ["x"],
                                                        "tools": ["create_space"], "ops": ["insert_blocks"],
                                                        "turns": [{"turn": 1, "user": "u"}]}]}))
            suite.with_name("HARNESS.md").write_text("h\n")
            # the bridge's snapshot is there too, and must not be read
            suite.with_name("surface-snapshot.json").write_text(json.dumps({"tools": [
                {"name": "mcp__anytype__API_patch_object", "description": "bridge"}], "schemas": {}}))
            snapshot = root / "surface-snapshot-full.json"
            snapshot.write_text(json.dumps({"surface": "full", "arm": "full",
                                            "tools": json.loads(golden.read_text())["tools"],
                                            "schemas": {"ops/insert_blocks": {"kind": "insert_blocks", "schema": {}},
                                                        "object": {"kind": "object", "schema": {}}}}))
            run = root / "run-full"
            run.mkdir()
            (run / "run.json").write_text(json.dumps({
                "scenario_id": "MCP-01", "label": "run-full", "host": "claude", "surface": "full", "arm": "full",
                "surface_snapshot": str(snapshot), "turns": [{"turn": 1, "user": "u"}]}))
            (run / "turn-01.prompt.txt").write_text("u\n")
            events = [{"type": "item.completed", "item": {"type": "mcp_tool_call", "tool": tool, "arguments": args,
                                                           "result": {"content": [{"type": "text", "text": "{}"}]}}}
                      for tool, args in [("patch_object", {"space_id": "s", "object_id": "o", "ops": []}),
                                         ("get_schema", {"kind": "object"})]]
            (run / "turn-01.events.jsonl").write_text("".join(json.dumps(e) + "\n" for e in events))

            bundle = review.evidence_bundle(run, suite)

            surface = json.loads(bundle["sources"]["spec/selected-surface.json"]["text"])
            names = sorted(t["name"] for t in surface["tools"])
            self.assertEqual(["create_space", "get_schema", "patch_object"], names)
            self.assertEqual({"ops/insert_blocks", "object"}, set(surface["schemas"]))
            self.assertEqual(str(snapshot), bundle["sources"]["spec/selected-surface.json"]["original"])
            text = json.dumps(bundle)
            self.assertNotRegex(text, r"API[-_][a-z]", "no bridge-spelled tool name in the review inputs")
            run_fields = json.loads(bundle["sources"]["run/run.json"]["text"])
            self.assertEqual(("claude", "full", "full"), (run_fields["host"], run_fields["surface"], run_fields["arm"]))


class ArmNeutralBundleTest(unittest.TestCase):
    """The review compares behaviour across arms, so the inline arm's larger
    tools/list and the guard's schema lookups must not reach the bundle; both
    arms carry the same placeholder fields and the same served declarations."""

    def make_run(self, root, arm, served, inlined, fetches):
        snapshot = root / f"surface-snapshot-{arm}.json"
        snapshot.write_text(json.dumps({"surface": "full", "arm": arm, "tools": inlined if arm == "full-inline" else served,
                                        "served_tools": served, "schemas": {"ops/insert_blocks": {"kind": "insert_blocks"}}}))
        run = root / f"run-{arm}"
        run.mkdir()
        (run / "run.json").write_text(json.dumps({
            "scenario_id": "MCP-01", "label": run.name, "host": "codex", "surface": "full", "arm": arm,
            "surface_snapshot": str(snapshot), "turns": [{"turn": 1, "user": "u"}],
            "tools_list": {"arm": arm, "tool_count": 2, "served_bytes": 100,
                           "forwarded_bytes": 900 if arm == "full-inline" else 100}}))
        (run / "turn-01.prompt.txt").write_text("u\n")
        call = {"type": "item.completed", "item": {"type": "mcp_tool_call", "tool": "patch_object",
                "arguments": {"ops": []}, "result": {"content": [{"type": "text", "text": "{\"etag\":\"e\"}"}]}}}
        (run / "turn-01.events.jsonl").write_text(json.dumps(call) + "\n")
        wire = [{"direction": "to_model", "payload": {"jsonrpc": "2.0", "id": 2, "result": {"tools": inlined}}}]
        for i in range(fetches):
            rid = f"evaluation-schema-{i}"
            wire.append({"direction": "evaluator_to_upstream", "payload": {"jsonrpc": "2.0", "id": rid, "method": "tools/call",
                         "params": {"name": "get_op_schema", "arguments": {"op": f"op{i}"}}}})
            wire.append({"direction": "evaluator_from_upstream", "payload": {"jsonrpc": "2.0", "id": rid,
                         "result": {"content": [{"type": "text", "text": "x" * 5000}]}}})
        (run / "mcp-wire.jsonl").write_text("".join(json.dumps(e) + "\n" for e in wire))
        return run

    def test_both_arms_get_the_same_shape_of_bundle(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            suite = root / "scenarios.json"
            suite.write_text(json.dumps({"scenarios": [{"id": "MCP-01", "title": "T", "checks": ["x"], "tools": ["patch_object"],
                                                        "ops": ["insert_blocks"], "turns": [{"turn": 1, "user": "u"}]}]}))
            suite.with_name("HARNESS.md").write_text("h\n")
            served = [{"name": "patch_object", "description": "d", "inputSchema": {"type": "object"}}]
            inlined = [{"name": "patch_object", "description": "d", "inputSchema": {"type": "object", "pad": "y" * 20000}}]
            full = review.evidence_bundle(self.make_run(root, "full", served, served, 0), suite)
            inline = review.evidence_bundle(self.make_run(root, "full-inline", served, inlined, 6), suite)

            pf = json.loads(full["sources"]["run/served-tools-list.json"]["text"])
            pi = json.loads(inline["sources"]["run/served-tools-list.json"]["text"])
            self.assertEqual(set(pf), set(pi), "the same placeholder fields for both arms")
            self.assertEqual(("full", 100, 0), (pf["arm"], pf["forwarded_bytes"], pf["schema_lookups_inlined_by_guard"]))
            self.assertEqual(("full-inline", 900, 6), (pi["arm"], pi["forwarded_bytes"], pi["schema_lookups_inlined_by_guard"]))

            wire = inline["sources"]["run/mcp-wire.jsonl"]
            kept = [wire["text"].splitlines()[n - 1] for n in wire["included_lines"]]
            self.assertFalse(any("evaluation-schema-" in line for line in kept), "the guard's lookups are not evidence")
            self.assertFalse(any('"tools"' in line for line in kept), "nor is the tools/list result")

            sf = json.loads(full["sources"]["spec/selected-surface.json"]["text"])
            si = json.loads(inline["sources"]["spec/selected-surface.json"]["text"])
            self.assertEqual(sf["tools"], si["tools"], "both arms are judged on the served declarations")
            self.assertIn("arm_note", si)
            self.assertLess(len(review.prompt_for(inline)) - len(review.prompt_for(full)), 2000,
                            "the arms' prompts differ by the arm fields, not by payload")
